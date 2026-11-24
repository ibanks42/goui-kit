// Kit state calls, window lifecycle, focus handles, and unified handle Release.
//
// Reads flush pending deferred writes first so callers observe their own prior
// mutations in order; reads on kit states also record an entity dependency via
// dep(h) during view rendering so GPUI re-renders the view when that state
// changes. Write-only operations queue onto the active turn's deferred buffer
// (except under GK_DEBUG=1, where DeferOp executes immediately).

package rt

// StateMake executes a state-creation request tape, attaches any pending kept
// handlers to the returned handle, registers GC tracking, and links ownership
// to the current turn's view. It must be called outside a view function.
func StateMake(cx *Cx, o *Tape) uint64 {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not create kit state (it runs on every redraw): create it once in init or in a handler and keep the handle in your state struct, as in a.search = editor.NewInputState(cx).Build()")
	}
	t.Flush()
	r := Reader{B: MustCall(*o)}
	h := r.U64()
	keptAttach(h)
	track(h)
	own(h, ownState)
	return h
}

func readResult(rc int32, res []byte) *Reader {
	if failed.Load() {
		panic(abortTurn{})
	}
	switch rc {
	case 0:
		return &Reader{B: res}
	case ErrPoisoned:
		libraryPanic(lastError())
	}
	stopCall(rc)
	return nil
}

// StateTry flushes pending writes, records state handle h as a view dependency
// if inside a view build, and runs a read request. It returns ok=false when the
// state is currently borrowed by an active component update (ErrBusy).
func StateTry(h uint64, cx *Cx, o *Tape) (*Reader, bool) {
	CheckCx(cx).Flush()
	dep(h)
	rc, res := call(*o)
	if rc == ErrBusy {
		return &Reader{}, false
	}
	return readResult(rc, res), true
}

// StateGet performs a synchronous state read like StateTry, stopping if the
// state is busy.
func StateGet(h uint64, cx *Cx, o *Tape) *Reader {
	r, ok := StateTry(h, cx, o)
	if !ok {
		Stop("%s", lastError())
	}
	return r
}

// StateCall executes a synchronous mutating call that returns a result on a
// kit state handle outside a view function.
func StateCall(h uint64, cx *Cx, o *Tape) *Reader {
	if Vb.Active {
		Stop("a view function may not change kit state (it only describes): change it in a handler or in init")
	}
	CheckCx(cx).Flush()
	keptAttachReq(*o)
	rc, res := call(*o)
	return readResult(rc, res)
}

// StateSet queues a write-only state mutation onto the current turn's deferred
// op buffer.
func StateSet(cx *Cx, o *Tape) {
	t := CheckCx(cx)
	keptAttachReq(*o)
	t.DeferOp(*o)
}

// ObjGet performs a synchronous read on an object handle without recording a
// GPUI entity dependency.
func ObjGet(h uint64, cx *Cx, o *Tape) *Reader {
	CheckCx(cx).Flush()
	rc, res := call(*o)
	return readResult(rc, res)
}

// AmbientCall invokes an app-level or window-level C ABI call. Calls that do
// not return a value (returns == false) are treated as mutations and rejected
// inside view functions.
func AmbientCall(cx *Cx, o *Tape, returns bool) *Reader {
	t := CheckCx(cx)
	if !returns && Vb.Active {
		Stop("a view function may not change the app or a window: make this call in a handler or in init")
	}
	t.Flush()
	return &Reader{B: MustCall(*o)}
}

// StateBuilder accumulates builder options into a state-creation tape before
// Build submits the request via StateMake.
type StateBuilder struct {
	cx      *Cx
	o       *Tape
	countAt int
}

// NewStateBuilder reserves a uint16 option count at the current end of o and
// returns a single-use StateBuilder.
func NewStateBuilder(cx *Cx, o *Tape) *StateBuilder {
	at := len(*o)
	o.U16(0)
	return &StateBuilder{cx, o, at}
}

// Op increments the builder option count, appends opcode code, and returns the
// tape for writing option arguments.
func (b *StateBuilder) Op(code uint16) *Tape {
	if b.o == nil {
		Stop("a builder chain was used after Build: start a new one with its constructor")
	}
	n := le.Uint16((*b.o)[b.countAt:])
	le.PutUint16((*b.o)[b.countAt:], n+1)
	b.o.U16(code)
	return b.o
}

// Build consumes the builder chain, executes StateMake, and returns the new
// state handle.
func (b *StateBuilder) Build() uint64 {
	if b.o == nil {
		Stop("Build was called twice on one builder chain: each chain makes one")
	}
	o := b.o
	b.o = nil
	return StateMake(b.cx, o)
}

// Window is a handle to an open GPUI window.
type Window struct{ HCore }

// WindowStyle selects the native window chrome and floating behavior.
type WindowStyle uint8

// Window presentation styles for WindowConfig.Style.
const (
	WindowNormal   WindowStyle = 0
	WindowPopup    WindowStyle = 1
	WindowFloating WindowStyle = 2
)

// WindowConfig configures size, title, style, position, flags, AppID, and icon
// when opening a window with OpenWindowWith.
type WindowConfig struct {
	Width, Height       float32
	Title               string
	MinWidth, MinHeight float32
	Style               WindowStyle
	Position            Option[[2]float32]
	NotResizable        bool
	NotMinimizable      bool
	NotMovable          bool
	NoFocus             bool
	Hidden              bool
	KitTitleBar         bool
	AppID               string
	Icon                []byte
}

// OpenWindow opens a standard window displaying root with the given title and dimensions.
func OpenWindow(cx *Cx, title string, width, height float32, root View) Window {
	return OpenWindowWith(cx, WindowConfig{Width: width, Height: height, Title: title}, root)
}

// OpenWindowWith opens a window configured by c displaying root.
func OpenWindowWith(cx *Cx, c WindowConfig, root View) Window {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not open windows: open them in init or in a handler")
	}
	t.Flush()
	o := NewReq(CWindowOpen)
	o.F32(orDefault(c.Width, 1024))
	o.F32(orDefault(c.Height, 700))
	o.U64(root.h)
	o.Str(c.Title)
	var flags uint8
	for i, b := range []bool{c.NotResizable, c.NotMinimizable, c.NotMovable, c.NoFocus, c.Hidden, c.KitTitleBar} {
		if b {
			flags |= 1 << i
		}
	}
	o.U8(flags)
	o.F32(c.MinWidth)
	o.F32(c.MinHeight)
	o.U8(uint8(c.Style))
	p, has := c.Position.Get()
	o.Bool(has)
	o.F32(p[0])
	o.F32(p[1])
	o.Str(c.AppID)
	o.Bytes(c.Icon)
	r := Reader{B: MustCall(*o)}
	return Window{MkH(r.U64())}
}

func orDefault(v, d float32) float32 {
	if v > 0 {
		return v
	}
	return d
}

func windowOp(cx *Cx, code uint16, w Window, text string) {
	o := NewReq(code)
	o.U64(w.h)
	if code == CWindowSetTitle {
		o.Str(text)
	}
	CheckCx(cx).DeferOp(*o)
}

// CloseWindow closes window w and disables inline re-description for the turn.
func CloseWindow(cx *Cx, w Window) {
	CheckCx(cx).skipInline = true
	windowOp(cx, CWindowClose, w, "")
}

// SetWindowTitle updates the title of window w.
func SetWindowTitle(cx *Cx, w Window, title string) { windowOp(cx, CWindowSetTitle, w, title) }

// ActivateWindow raises and focuses window w.
func ActivateWindow(cx *Cx, w Window) { windowOp(cx, CWindowActivate, w, "") }

// QuitOnLastWindow controls whether the application exits automatically when
// its last open window closes.
func QuitOnLastWindow(cx *Cx, yes bool) {
	o := NewReq(CQuitOnLastWindow)
	o.Bool(yes)
	CheckCx(cx).DeferOp(*o)
}

// WindowInfo describes a window's current title, bounds, and active status.
type WindowInfo struct {
	Title               string
	X, Y, Width, Height float32
	Active              bool
}

// GetWindowInfo queries current geometry and title metadata for window w.
func GetWindowInfo(cx *Cx, w Window) WindowInfo {
	CheckCx(cx).Flush()
	o := NewReq(CWindowInfo)
	o.U64(w.h)
	r := Reader{B: MustCall(*o)}
	return WindowInfo{r.Str(), r.F32(), r.F32(), r.F32(), r.F32(), r.Bool()}
}

// Windows returns handles for all currently open application windows.
func Windows(cx *Cx) []Window {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CWindows))}
	return RdList(&r, func(r *Reader) Window { return Window{MkH(r.U64())} })
}

// OnWindowClosed registers f to run after window w closes.
func OnWindowClosed(cx *Cx, w Window, f func(cx *Cx, w Window)) ActionListener {
	CheckCx(cx).Flush()
	slot, gen := KeepHandler(cx, f, ShapeWindow)
	o := NewReq(COnWindowClosed)
	o.U64(w.h)
	o.U32(slot)
	o.U32(gen)
	r := Reader{B: MustCall(*o)}
	return ActionListener{r.U64(), slot}
}

// OnWindowShouldClose registers a veto callback invoked before window w closes;
// returning false cancels the close.
func OnWindowShouldClose(cx *Cx, w Window, f func(cx *Cx) bool) ActionListener {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not listen for a window's close: do it in init or in a handler")
	}
	t.Flush()
	slot, gen := KeepHandler(cx, f, ShapeUnitBool)
	o := NewReq(COnWindowShouldClose)
	o.U64(w.h)
	o.U32(slot)
	o.U32(gen)
	r := Reader{B: MustCall(*o)}
	return ActionListener{r.U64(), slot}
}

// OnAppQuit registers f to run when the application is quitting.
func OnAppQuit(cx *Cx, f func(cx *Cx)) ActionListener {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not set a quit handler: do it in init or in a handler")
	}
	t.Flush()
	slot, gen := KeepHandler(cx, f, ShapeUnit)
	o := NewReq(COnAppQuit)
	o.U32(slot)
	o.U32(gen)
	r := Reader{B: MustCall(*o)}
	return ActionListener{r.U64(), slot}
}

// ReduceMotion reports whether the reduced-motion accessibility flag is active.
func ReduceMotion(cx *Cx) bool {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CReduceMotion))}
	return r.Bool()
}

// SetReduceMotion overrides the reduced-motion accessibility flag.
func SetReduceMotion(cx *Cx, reduce bool) {
	o := NewReq(CSetReduceMotion)
	o.Bool(reduce)
	CheckCx(cx).DeferOp(*o)
}

// TestShouldClose triggers window w's OnWindowShouldClose check in tests.
func TestShouldClose(cx *Cx, w Window) bool {
	CheckCx(cx).Flush()
	o := NewReq(CPlatformTest)
	o.U8(6)
	o.U64(w.h)
	r := Reader{B: MustCall(*o)}
	return r.Bool()
}

var (
	frameHandlers = map[uint64]func(*Cx){}
	frameOwners   = map[uint64]*viewRec{}
)

// OnFrame installs or clears (when f is nil) a per-frame callback for the
// current turn's window.
func OnFrame(cx *Cx, f func(cx *Cx)) {
	t := CheckCx(cx)
	if t.window == 0 {
		Stop("gk.OnFrame needs a window: call it in a view's init function or in a handler")
	}
	frameHandlers[t.window] = f
	frameOwners[t.window] = t.owner
	o := NewReq(COnFrame)
	o.U64(t.window)
	o.Bool(f != nil)
	t.DeferOp(*o)
}

// Quit requests a clean shutdown of the application event loop.
func Quit(cx *Cx) {
	t := CheckCx(cx)
	t.skipInline = true
	t.DeferOp(*NewReq(CQuit))
}

func (f FocusHandle) handle() uint64 { return f.h }

// NewFocusHandle allocates a GPUI focus handle tracked for GC cleanup and owned
// by the current view.
func NewFocusHandle(cx *Cx) FocusHandle {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not create focus handles: create them in init or in a handler and keep the handle")
	}
	t.Flush()
	r := Reader{B: MustCall(*NewReq(CFocusHandleNew))}
	h := r.U64()
	track(h)
	own(h, ownState)
	return FocusHandle{MkH(h)}
}

// Focus moves keyboard focus to f.
func Focus(cx *Cx, f FocusHandle) {
	o := NewReq(CFocus)
	o.U64(f.h)
	CheckCx(cx).DeferOp(*o)
}

// Subscription is a releasable handle to a kit state event subscription.
type Subscription struct {
	h    uint64
	slot uint32
}

func releaseHandle(t *turn, h uint64) {
	o := NewReq(CRelease)
	o.U64(h)
	t.DeferOp(*o)
}

// Handled is implemented by every Go type wrapping an opaque Rust handle.
type Handled interface{ handle() uint64 }

// Release explicitly frees a handle (View, RowSource, Timer, Completion,
// Subscription, ActionListener, or any kit state handle), cascading to owned
// children and dropping associated kept handlers.
func Release(cx *Cx, x any) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not write: it only describes (gk.Release). Release in a handler, in init or in a frame handler")
	}
	t.skipInline = true
	if hd, ok := x.(Handled); ok {
		checkNotGone(hd.handle())
		disown(hd.handle())
		forget(hd.handle())
	}
	switch v := x.(type) {
	case View:
		releaseView(t, v)
	case Subscription:
		dropHandler(v.slot)
		releaseHandle(t, v.h)
	case RowSource:
		releaseSource(t, v)
	case Timer:
		releaseTimer(t, v)
	case Completion:
		releaseCompletion(t, v)
	case ActionListener:
		dropHandler(v.slot)
		releaseHandle(t, v.h)
	case Handled:
		releaseOwned(t, v.handle())
		keptRelease(v.handle())
		releaseHandle(t, v.handle())
	default:
		Stop("gk.Release takes a handle (kit state, a view, a subscription, a row source, a timer, ...), not a %T", x)
	}
}

// Retain promotes a frame-temporary handle to a long-lived handle on the Rust side.
func Retain(cx *Cx, x Handled) {
	CheckCx(cx).Flush()
	o := NewReq(CObjRetain)
	o.U64(x.handle())
	MustCall(*o)
}

func putKept(o *Tape, slot uint32) {
	if int(slot) >= len(keptTab) || !keptTab[slot].live {
		Stop("this handler slot is not in use")
	}
	o.U64(0)
	o.U32(keptTab[slot].gen)
	o.U32(slot)
}

// TestFire invokes kept handler slot from Rust and returns its integer status.
func TestFire(cx *Cx, slot uint32) int32 {
	CheckCx(cx).Flush()
	o := NewReq(CTestFire)
	putKept(o, slot)
	r := Reader{B: MustCall(*o)}
	return int32(r.U32())
}

// TestFireValue fires a ShapeTest handler round-trip through Rust and returns its string reply.
func TestFireValue(cx *Cx, h func(cx *Cx, p TestPayload) string, text string, n uint32) string {
	CheckCx(cx).Flush()
	slot, _ := KeepHandler(cx, h, ShapeTest)
	defer dropHandler(slot)
	o := NewReq(CTestFireValue)
	putKept(o, slot)
	o.Str(text)
	o.U32(n)
	r := Reader{B: MustCall(*o)}
	return r.Str()
}

// TestFireNoValue fires a ShapeUnit handler through the value-returning test hook.
func TestFireNoValue(cx *Cx, h func(cx *Cx), text string, n uint32) string {
	CheckCx(cx).Flush()
	slot, _ := KeepHandler(cx, h, ShapeUnit)
	defer dropHandler(slot)
	o := NewReq(CTestFireValue)
	putKept(o, slot)
	o.Str(text)
	o.U32(n)
	r := Reader{B: MustCall(*o)}
	return r.Str()
}

// TestFireInUpdate invokes slot while state handle s is actively borrowed by Rust
// to exercise ErrBusy handling.
func TestFireInUpdate(cx *Cx, s Handled, slot uint32) int32 {
	CheckCx(cx).Flush()
	o := NewReq(CTestFireInUpdate)
	o.U64(s.handle())
	putKept(o, slot)
	r := Reader{B: MustCall(*o)}
	return int32(r.U32())
}

// TestFireQueued schedules a queued ShapeUsize callback on state s in tests.
func TestFireQueued(cx *Cx, s Handled, h func(cx *Cx, row int), row int) {
	CheckCx(cx).Flush()
	slot, _ := KeepHandler(cx, h, ShapeUsize)
	o := NewReq(CTestFireQueued)
	o.U64(s.handle())
	putKept(o, slot)
	o.U64(uint64(row))
	MustCall(*o)
}

// TestPanic triggers a deliberate Rust panic (either immediately or deferred)
// to test LibraryError recovery.
func TestPanic(cx *Cx, later bool) {
	t := CheckCx(cx)
	t.skipInline = true
	o := NewReq(CTestPanic)
	if later && !debug {
		t.DeferOp(*o)
	} else {
		MustCall(*o)
	}
}

// TestFireShape dispatches an arbitrary wire payload for shape through Rust and
// returns whether the typed handler matched and any reply bytes produced.
func TestFireShape(cx *Cx, h any, shape uint16, payload []byte) (typed bool, ret []byte) {
	CheckCx(cx).Flush()
	slot, _ := KeepHandler(cx, h, shape)
	defer dropHandler(slot)
	o := NewReq(CTestFireShape)
	putKept(o, slot)
	o.U16(shape)
	o.Bytes(payload)
	r := Reader{B: MustCall(*o)}
	typed = r.U8() != 0
	n := int(r.U32())
	return typed, append([]byte(nil), r.B[r.i:r.i+n]...)
}

// Handle extracts the raw 64-bit Rust handle from x.
func Handle(x Handled) uint64 { return x.handle() }

// MkSubscription constructs a Subscription from a Rust handle and kept slot.
func MkSubscription(h uint64, slot uint32) Subscription { return Subscription{h, slot} }
