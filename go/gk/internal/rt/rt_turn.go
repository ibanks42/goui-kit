package rt

import (
	"fmt"
	"os"
)

type turn struct {
	kind          uint32
	parent        *turn
	serial        uint64
	window        uint64
	ret           Tape
	dirty         []uint64
	deferred      Tape
	nDefer        uint32
	tapes         Tape
	nTapes        uint32
	pending       []*viewRec
	owner         *viewRec // the view whose init or handler runs: handlers kept now belong to it
	quiet         bool     // gk.Quiet was called in the handler running
	skipInline    bool     // do not describe dirty views inline (Quit, CloseWindow, Release, TestPanic)
	inInlineEvent bool     // describing pure-Go views inline at the end of an EVENT turn
	cx            *Cx
}

// abortInline unwinds an inline EVENT-turn view description if the view calls into the library
// (so buffered writes are never flushed early and leased kit state is never read inside EVENT).
type abortInline struct{}

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
	uiG        uintptr
	debug      = os.Getenv("GK_DEBUG") != ""
	dropped    uint64
	appStart   func(*Cx)
	tapeBuf    Tape
	cxSlab     []Cx     // the Cx of each turn, from blocks of 64 (one allocation per 64 turns)
	viewList   []uint64 // the VIEWS turn's list, reused
)

func runtimeInit() {
	uiG = curG()
}

// checkCx checks that cx is the current turn's, on the UI thread, and returns the turn.
func CheckCx(cx *Cx) *turn {
	if curG() != uiG {
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
	if Vb.Active && t.inInlineEvent {
		panic(abortInline{})
	}
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
	t.kind = kind
	t.parent = cur
	t.serial = turnSerial
	t.window = 0
	t.ret = t.ret[:0]
	t.dirty = t.dirty[:0]
	t.deferred = t.deferred[:0]
	t.nDefer = 0
	t.tapes = t.tapes[:0]
	t.nTapes = 0
	t.pending = t.pending[:0]
	t.owner = nil
	t.quiet = false
	t.skipInline = false
	t.inInlineEvent = false
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
		if v := views.get(r.U64()); v != nil {
			v.window = t.window
			if v.init != nil {
				t.owner = v
				v.init(cx)
			}
		}
	case KFrame:
		t.window = r.U64()
		_ = r.U64() // frame number
		for range r.U32() {
			h := r.U64()
			if v := views.get(h); v != nil {
				v.window = t.window
			}
			t.markDirty(h)
		}
		retireAll(&r)
		if f := frameHandlers[t.window]; f != nil {
			t.owner = frameOwners[t.window]
			f(cx)
		}
		if !failed.Load() && !t.skipInline && depth == 1 && t.nDefer == 0 && len(t.dirty) > 0 {
			describeInline(t, false)
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
		retireAll(&r)
		for _, h := range list {
			if v := views.get(h); v != nil {
				v.window = t.window
				describe(cx, v)
			}
		}
		describePending(cx, t, true)
	case KRows:
		t.window = r.U64()
		fresh := rowsTurn(&r, t.window)
		var list []uint64
		if depth == 1 {
			list = viewList[:0]
		}
		for range r.U32() {
			list = append(list, r.U64())
		}
		if depth == 1 {
			viewList = list
		}
		retireAll(&r)
		for _, h := range list {
			if v := views.get(h); v != nil {
				v.window = t.window
				describe(cx, v)
			}
		}
		for _, h := range fresh {
			if v := views.get(h); v != nil {
				v.window = t.window
				describe(cx, v)
			}
		}
		describePending(cx, t, true)
	case KEvent:
		t.window = r.U64()
		flags := r.U8()
		retireAll(&r)
		event(cx, &r)
		if !failed.Load() && flags&1 != 0 && depth == 1 && t.window != 0 && !t.skipInline && len(t.dirty) > 0 {
			describeInline(t, true)
		}
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

// retireAll forgets the handler tables of descriptions the library dropped since the last turn.
func retireAll(r *Reader) {
	for range r.U32() {
		view := r.U64()
		retireTable(view, r.U32())
	}
}

// rotateCx ends the handler's Cx (so a kept *Cx becomes stale before view functions run) and
// hands the inline describe phase a fresh Cx.
func rotateCx(t *turn) *Cx {
	t.cx.serial = 0
	t.owner = nil
	turnSerial++
	t.serial = turnSerial
	if len(cxSlab) == cap(cxSlab) {
		cxSlab = make([]Cx, 0, 64)
	}
	cxSlab = append(cxSlab, Cx{t, t.serial})
	t.cx = &cxSlab[len(cxSlab)-1]
	return t.cx
}

func describePending(cx *Cx, t *turn, allowInline bool) {
	if !allowInline {
		for _, v := range t.pending {
			if !v.described && !v.dead {
				t.markDirty(v.h)
			}
		}
		return
	}
	for i := 0; i < len(t.pending); i++ {
		if v := t.pending[i]; !v.described && !v.dead {
			describe(cx, v)
		}
	}
}

// tryDescribe describes v during an inline EVENT pass, aborting cleanly if v calls into the
// library (so v falls back to the pre-frame VIEWS turn after deferred writes and updates land).
func tryDescribe(cx *Cx, v *viewRec) (ok bool) {
	t := cx.t
	pendingLen := len(t.pending)
	ok = true
	defer func() {
		if r := recover(); r != nil {
			if _, is := r.(abortInline); is {
				Vb.Active = false
				v.readsLib = true
				t.pending = t.pending[:pendingLen]
				ok = false
				return
			}
			panic(r)
		}
	}()
	describe(cx, v)
	return true
}

// describeInline describes eligible dirty mounted views of t.window at the end of an outermost
// FRAME or EVENT turn, leaving ineligible views in t.dirty for the library's pre-frame VIEWS turn.
// When pureOnly is true (EVENT), only previously described views that made no library calls (and
// therefore cannot flush deferred writes or hit leased state) are described inline.
func describeInline(t *turn, pureOnly bool) {
	if len(t.dirty) == 1 {
		h := t.dirty[0]
		v := views.get(h)
		if v == nil || v.dead || v.window != t.window || (pureOnly && (!v.described || v.readsLib)) {
			return
		}
		cx := rotateCx(t)
		t.dirty = t.dirty[:0]
		if pureOnly {
			t.inInlineEvent = true
			if !tryDescribe(cx, v) {
				t.dirty = append(t.dirty, h)
			}
			t.inInlineEvent = false
		} else {
			describe(cx, v)
		}
		describePending(cx, t, !pureOnly && t.nDefer == 0)
		return
	}
	any := false
	for _, h := range t.dirty {
		if v := views.get(h); v != nil && !v.dead && v.window == t.window && (!pureOnly || (v.described && !v.readsLib)) {
			any = true
			break
		}
	}
	if !any {
		return
	}
	cx := rotateCx(t)
	list := viewList[:0]
	list = append(list, t.dirty...)
	viewList = list
	t.dirty = t.dirty[:0]
	if pureOnly {
		t.inInlineEvent = true
	}
	for _, h := range list {
		v := views.get(h)
		if v != nil && !v.dead && v.window == t.window && (!pureOnly || (v.described && !v.readsLib)) {
			if pureOnly {
				if !tryDescribe(cx, v) {
					t.dirty = append(t.dirty, h)
				}
			} else {
				describe(cx, v)
			}
		} else {
			t.dirty = append(t.dirty, h)
		}
	}
	t.inInlineEvent = false
	describePending(cx, t, !pureOnly && t.nDefer == 0)
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
