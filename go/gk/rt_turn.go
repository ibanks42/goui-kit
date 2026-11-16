package gk

// Turns (docs/PROTOCOLS.md section 3): all app code runs in one callback at a time, on the UI
// thread, between frames. A turn collects the handler's return value, dirty marks, buffered
// writes and changed tapes, and they travel back in one reply.

import (
	"fmt"
	"os"
)

type turn struct {
	kind     uint32
	parent   *turn
	serial   uint64
	window   uint64
	ret      tape
	dirty    []uint64
	deferred tape
	nDefer   uint32
	tapes    tape
	nTapes   uint32
	pending  []*viewRec
	owner    *viewRec // the view whose init or handler runs: handlers kept now belong to it
	quiet    bool     // gk.Quiet was called in the handler running
	out      tape
	cx       *Cx
}

// Cx is the context of one turn, given to every init function, handler, view function and
// posted function. It is valid until that call returns; one kept longer stops the program on use.
type Cx struct {
	t      *turn
	serial uint64
}

var (
	cur        *turn
	turnPool   []*turn // by nesting depth, reused
	depth      int
	turnSerial uint64
	uiThread   int
	debug      = os.Getenv("GK_DEBUG") != ""
	dropped    uint64
	appStart   func(*Cx)
	tapeBuf    tape
)

func runtimeInit() {
	uiThread = osThread()
}

// checkCx checks that cx is the current turn's, on the UI thread, and returns the turn.
func checkCx(cx *Cx) *turn {
	if osThread() != uiThread {
		// No callback boundary is below another goroutine to recover a panic: print the stop the
		// way the boundary does (message and the app's line, exit 2), without Go's goroutine dump.
		e := &Error{Msg: "the gk package was called from a goroutine that is not on the UI thread: only gk.Post may be called there. Post what the goroutine computed, as in gk.Post(func(cx *gk.Cx) { a.items = items }); the posted function runs on the UI thread with a cx"}
		e.File, e.Line, e.Func = appFrame(2)
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(2)
	}
	if cx == nil || cur == nil || cx.t != cur || cx.serial != cur.serial {
		stop("a *gk.Cx was used after the function it was given to returned: a cx is valid only during that call. Do not store it; use the cx your handler, init or view function is given")
	}
	return cur
}

func (t *turn) markDirty(view uint64) {
	for _, v := range t.dirty {
		if v == view {
			return
		}
	}
	t.dirty = append(t.dirty, view)
}

// deferOp buffers a call op that returns nothing; it travels in the turn's reply and the library
// applies it when the callback has returned (after the outermost update, for an EVENT turn),
// before the next draw. A view function may not write.
func (t *turn) deferOp(op []byte) {
	if vb.active {
		stop("a view function may not write: it only describes what the view shows. Change kit state in a handler, in init or in a frame handler")
	}
	if debug {
		mustCall(op) // GK_DEBUG: every op at once, so an error points at its own line
		return
	}
	t.deferred.u32(uint32(len(op)))
	t.deferred = append(t.deferred, op...)
	t.nDefer++
}

// flush sends buffered writes now, before a call that returns a value, so order is kept.
func (t *turn) flush() {
	if t.nDefer == 0 {
		return
	}
	r := reader{b: t.deferred}
	for range t.nDefer {
		n := int(r.u32())
		mustCall(r.b[r.i : r.i+n])
		r.i += n
	}
	t.deferred, t.nDefer = t.deferred[:0], 0
}

func dispatch(kind uint32, in []byte) []byte {
	if depth == len(turnPool) {
		turnPool = append(turnPool, &turn{})
	}
	t := turnPool[depth]
	depth++
	turnSerial++
	*t = turn{kind: kind, parent: cur, serial: turnSerial, ret: t.ret[:0], dirty: t.dirty[:0],
		deferred: t.deferred[:0], tapes: t.tapes[:0], pending: t.pending[:0], out: t.out[:0]}
	t.cx = &Cx{t, t.serial} // a new one per turn: a kept *Cx keeps its old serial
	cur = t
	cx := t.cx
	r := reader{b: in}
	switch kind {
	case kInit:
		// Posts made before the library's loop existed had no wake to ask for: they run here,
		// after the start function.
		early := postsQueued()
		appStart(cx)
		drainPosts(cx, early)
	case kWindowInit:
		t.window = r.u64()
		if v := views[r.u64()]; v != nil && v.init != nil {
			t.owner = v
			v.init(cx)
		}
	case kFrame:
		t.window = r.u64()
		if f := frameHandlers[t.window]; f != nil {
			t.owner = frameOwners[t.window]
			f(cx)
		}
	case kViews:
		t.window = r.u64()
		n := int(r.u32())
		for range n {
			if v := views[r.u64()]; v != nil {
				describe(cx, v)
			}
		}
		for i := 0; i < len(t.pending); i++ {
			if v := t.pending[i]; !v.described && !v.dead {
				describe(cx, v)
			}
		}
	case kRows:
		t.window = r.u64()
		rowsTurn(&r)
	case kEvent:
		event(cx, &r)
	case kWake:
		drainPosts(cx, -1)
	case kDone:
		doneTurn(cx, &r)
	default:
		stop("the library called the app in a way this package does not know (callback kind %d): the library is newer than the package. Build the app against the go/ folder of the download its library came from", kind)
	}
	if len(t.ret) == 0 && len(t.dirty) == 0 && t.nDefer == 0 && t.nTapes == 0 {
		return nil
	}
	o := t.out[:0]
	o.u32(uint32(len(t.ret)))
	o = append(o, t.ret...)
	o.u32(uint32(len(t.dirty)))
	for _, v := range t.dirty {
		o.u64(v)
	}
	o.u32(t.nDefer)
	o = append(o, t.deferred...)
	o.u32(t.nTapes)
	o = append(o, t.tapes...)
	t.out = o
	return o
}

// endTurn ends the turn; the reply has been copied by gk_reply.
func endTurn() {
	t := cur
	t.cx.serial = 0 // a kept *Cx is stale from here on, even if the turn record is reused
	cur = t.parent
	depth--
	if cur == nil {
		sweepRows()
	}
}

func report() {
	if debug {
		fmt.Fprintf(os.Stderr, "gk: dropped events %d, turns %d, kept handlers %d\n", dropped, turnSerial, keptLive())
	}
}

// Window is the turn's window (zero in the first turn and in posted functions without one).
func (cx *Cx) Window() Window { return Window{checkCx(cx).window} }
