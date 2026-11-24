package rt

import (
	"fmt"
	"os"
)

// turn holds the per-callback state for one active call from the Rust core into
// Go. Turns can nest (for example, when a gk_call triggers a re-entrant callback),
// forming a stack linked by parent.
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
	owner         *viewRec
	quiet         bool
	skipInline    bool
	inInlineEvent bool
	cx            *Cx
}

// abortInline is a sentinel panic raised by call/Flush when a view being
// re-described inline after an event attempts a library read; tryDescribe
// catches it, marks the view readsLib, and leaves the view dirty for a full turn.
type abortInline struct{}

// Cx is the UI-thread execution context passed to init, view, frame, and event
// callbacks. Its serial is invalidated as soon as the callback returns so stale
// contexts stored by user code are rejected by CheckCx.
type Cx struct {
	t      *turn
	serial uint64
}

// UI-thread turn stack, reusable buffers, and global run state.
var (
	cur        *turn
	turnPool   []*turn
	depth      int
	turnSerial uint64
	uiG        uintptr
	debug      = os.Getenv("GK_DEBUG") != ""
	dropped    uint64
	appStart   func(*Cx)
	tapeBuf    Tape
	cxSlab     []Cx
	viewList   []uint64
)

// runtimeInit records the UI goroutine/thread identity before entering gk_run.
func runtimeInit() {
	uiG = curG()
}

// CheckCx verifies that the caller is executing on the locked UI thread and
// that cx belongs to the currently active turn. Calls from another goroutine
// latch an error and exit that goroutine cleanly via failOffThread.
func CheckCx(cx *Cx) *turn {
	if curG() != uiG {
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

// DeferOp queues a write-only call tape to be sent in the turn reply instead of
// making an immediate FFI call. Under GK_DEBUG=1 it calls immediately so errors
// point directly at the offending call site.
func (t *turn) DeferOp(op []byte) {
	if Vb.Active {
		Stop("a view function may not write: it only describes what the view shows. Change kit state in a handler, in init or in a frame handler")
	}
	if debug {
		MustCall(op)
		return
	}
	t.deferred.U32(uint32(len(op)))
	t.deferred = append(t.deferred, op...)
	t.nDefer++
}

// Flush immediately executes every deferred write op queued on t so subsequent
// synchronous reads observe prior writes in order.
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

// dispatch pushes a turn frame, handles one callback kind from the Rust core,
// and encodes any return value, dirty views, deferred writes, and view tapes
// into the turn reply buffer.
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
		// Capture the count of posts queued before start ran so posts queued
		// during start wait for a normal KWake turn.
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
		_ = r.U64()
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
		// Piggyback view descriptions onto the KFrame reply to avoid a second
		// round trip when no deferred writes need to run first.
		if !failed.Load() && !t.skipInline && depth == 1 && t.nDefer == 0 && len(t.dirty) > 0 {
			describeInline(t, false)
		}
	case KViews:
		t.window = r.U64()
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
		// When the library allows inline replies (flags bit 0), re-describe pure
		// views immediately in the event reply rather than waiting for KViews.
		if !failed.Load() && flags&1 != 0 && depth == 1 && t.window != 0 && !t.skipInline && len(t.dirty) > 0 {
			describeInline(t, true)
		}
	case KWake:
		releaseCollected(t)
		drainPosts(cx, -1)
	case KDone:
		doneTurn(cx, &r)
	default:
		Stop("the library called the app in a way this package does not know (callback kind %d): the library is newer than the package. Build the app against the go/ folder of the download its library came from", kind)
	}
	if len(t.ret) == 0 && len(t.dirty) == 0 && t.nDefer == 0 && t.nTapes == 0 {
		return nil
	}
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

func retireAll(r *Reader) {
	for range r.U32() {
		view := r.U64()
		retireTable(view, r.U32())
	}
}

// rotateCx invalidates the previous Cx issued on t and allocates a fresh one
// before transitioning from a frame/event handler into inline view building.
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

// describePending describes child views discovered via SlotView/PutViewArg
// during the current turn, or marks them dirty when inline describing is off.
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

// tryDescribe runs describe under inline-event restrictions, recovering
// abortInline if the view attempts a library call so it can be retried in KViews.
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

// describeInline re-describes dirty views belonging to t.window during a KFrame
// or KEvent turn. When pureOnly is true, only already-described views that do
// not read from the library are attempted inline.
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

// endTurn pops the current turn, invalidates its Cx, and runs top-level cleanup
// hooks (row-source sweeps and fresh-handle unpinning) when depth reaches zero.
func endTurn() {
	t := cur
	t.cx.serial = 0
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

// Window returns the handle of the window associated with the current turn, or
// a zero Window in windowless turns such as KInit or KWake.
func (cx *Cx) Window() Window { return Window{MkH(CheckCx(cx).window)} }
