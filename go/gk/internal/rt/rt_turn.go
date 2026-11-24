package rt

import (
	"fmt"
	"os"
)

type turn struct {
	kind     uint32
	parent   *turn
	serial   uint64
	window   uint64
	ret      Tape
	dirty    []uint64
	deferred Tape
	nDefer   uint32
	tapes    Tape
	nTapes   uint32
	pending  []*viewRec
	owner    *viewRec // the view whose init or handler runs: handlers kept now belong to it
	quiet    bool     // gk.Quiet was called in the handler running
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
	tapeBuf    Tape
	cxSlab     []Cx     // the Cx of each turn, from blocks of 64 (one allocation per 64 turns)
	viewList   []uint64 // the VIEWS turn's list, reused
)

func runtimeInit() {
	uiThread = osThread()
}

// checkCx checks that cx is the current turn's, on the UI thread, and returns the turn.
func CheckCx(cx *Cx) *turn {
	if osThread() != uiThread {
		// No callback boundary is below another goroutine to recover a panic: the stop is
		// recorded, the UI thread asks the library to quit, and this goroutine ends (rt_fail.go).
		e := &Error{Msg: "the gk package was called from a goroutine that is not on the UI thread: only gk.Post may be called there. Post what the goroutine computed, as in gk.Post(func(cx *gk.Cx) { a.items = items }); the posted function runs on the UI thread with a cx"}
		e.File, e.Line, e.Func = appFrame(2)
		failOffThread(e)
	}
	if cx == nil || cur == nil || cx.t != cur || cx.serial != cur.serial {
		Stop("a *gk.Cx was used after the function it was given to returned: a cx is valid only during that call. Do not store it; use the cx your handler, init or view function is given")
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
func (t *turn) DeferOp(op []byte) {
	if Vb.Active {
		Stop("a view function may not write: it only describes what the view shows. Change kit state in a handler, in init or in a frame handler")
	}
	if debug {
		MustCall(op) // GK_DEBUG: every op at once, so an error points at its own line
		return
	}
	t.deferred.U32(uint32(len(op)))
	t.deferred = append(t.deferred, op...)
	t.nDefer++
}

// flush sends buffered writes now, before a call that returns a value, so order is kept.
func (t *turn) Flush() {
	if t.nDefer == 0 {
		return
	}
	r := Reader{B: t.deferred}
	for range t.nDefer {
		n := int(r.U32())
		MustCall(r.B[r.i : r.i+n])
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
		deferred: t.deferred[:0], tapes: t.tapes[:0], pending: t.pending[:0]}
	// A new one per turn, so a kept *Cx keeps its old serial; taken from a block, never reused.
	if len(cxSlab) == cap(cxSlab) {
		cxSlab = make([]Cx, 0, 64)
	}
	cxSlab = append(cxSlab, Cx{t, t.serial})
	t.cx = &cxSlab[len(cxSlab)-1]
	cur = t
	cx := t.cx
	r := Reader{B: in}
	switch kind {
	case KInit:
		// Posts made before the library's loop existed had no wake to ask for: they run here,
		// after the start function.
		early := postsQueued()
		appStart(cx)
		drainPosts(cx, early)
	case KWindowInit:
		t.window = r.U64()
		if v := views.get(r.U64()); v != nil && v.init != nil {
			t.owner = v
			v.init(cx)
		}
	case KFrame:
		t.window = r.U64()
		if f := frameHandlers[t.window]; f != nil {
			t.owner = frameOwners[t.window]
			f(cx)
		}
	case KViews:
		t.window = r.U64()
		var list []uint64
		if depth == 1 {
			list = viewList[:0] // reused by the outermost turn only: a nested one keeps its own
		}
		for range r.U32() {
			list = append(list, r.U64())
		}
		if depth == 1 {
			viewList = list
		}
		// The descriptions the library dropped since the last VIEWS turn: their tables go.
		for range r.U32() {
			view := r.U64()
			retireTable(view, r.U32())
		}
		for _, h := range list {
			if v := views.get(h); v != nil {
				describe(cx, v)
			}
		}
		for i := 0; i < len(t.pending); i++ {
			if v := t.pending[i]; !v.described && !v.dead {
				describe(cx, v)
			}
		}
	case KRows:
		t.window = r.U64()
		rowsTurn(&r)
	case KEvent:
		event(cx, &r)
	case KWake:
		releaseCollected(t) // what the collector found unreachable (rt_life.go)
		drainPosts(cx, -1)
	case KDone:
		doneTurn(cx, &r)
	default:
		Stop("the library called the app in a way this package does not know (callback kind %d): the library is newer than the package. Build the app against the go/ folder of the download its library came from", kind)
	}
	if len(t.ret) == 0 && len(t.dirty) == 0 && t.nDefer == 0 && t.nTapes == 0 {
		return nil
	}
	// Written straight into C memory, which the C side hands to gk_reply (rt_capi.go).
	o := replyBuf(16 + len(t.ret) + 8*len(t.dirty) + len(t.deferred) + len(t.tapes))
	o.U32(uint32(len(t.ret)))
	o = append(o, t.ret...)
	o.U32(uint32(len(t.dirty)))
	for _, v := range t.dirty {
		o.U64(v)
	}
	o.U32(t.nDefer)
	o = append(o, t.deferred...)
	o.U32(t.nTapes)
	o = append(o, t.tapes...)
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
		endTurnLife()
	}
}

func report() {
	if debug {
		fmt.Fprintf(os.Stderr, "gk: dropped events %d, turns %d, kept handlers %d\n", dropped, turnSerial, keptLive())
	}
}

// Window is the turn's window (zero in the first turn and in posted functions without one).
func (cx *Cx) Window() Window { return Window{MkH(CheckCx(cx).window)} }
