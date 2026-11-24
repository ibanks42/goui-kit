package rt

// stateMake runs a constructor call op and returns the new handle.
func StateMake(cx *Cx, o *Tape) uint64 {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not create kit state (it runs on every redraw): create it once in init or in a handler and keep the handle in your state struct, as in a.search = editor.NewInputState(cx).Build()")
	}
	t.Flush()
	r := Reader{B: MustCall(*o)}
	h := r.U64()
	keptAttach(h)
	return h
}

func readResult(rc int32, res []byte) *Reader {
	switch rc {
	case 0:
		return &Reader{B: res}
	case ErrPoisoned:
		libraryPanic(lastError())
	}
	stopCall(rc)
	return nil
}

// stateTry runs a read call op (a `&self` method). Buffered writes are sent first, so the read
// sees them. In a view build the state is recorded as a dependency. ok is false when the state
// is being updated by the component that called the handler.
func StateTry(h uint64, cx *Cx, o *Tape) (*Reader, bool) {
	CheckCx(cx).Flush()
	dep(h)
	rc, res := call(*o)
	if rc == ErrBusy {
		return &Reader{}, false
	}
	return readResult(rc, res), true
}

// stateGet is stateTry for the usual case: a busy state stops with the library's message.
func StateGet(h uint64, cx *Cx, o *Tape) *Reader {
	r, ok := StateTry(h, cx, o)
	if !ok {
		Stop("%s", lastError())
	}
	return r
}

// stateCall runs an update that returns a value at once. A view function may not call it.
func StateCall(h uint64, cx *Cx, o *Tape) *Reader {
	if Vb.Active {
		Stop("a view function may not change kit state (it only describes): change it in a handler or in init")
	}
	CheckCx(cx).Flush()
	keptAttachReq(*o)
	rc, res := call(*o)
	return readResult(rc, res)
}

// stateSet buffers a write call op (a `&mut self` method).
func StateSet(cx *Cx, o *Tape) {
	t := CheckCx(cx)
	keptAttachReq(*o)
	t.DeferOp(*o)
}

// objGet runs a read of a library object (or a global accessor, h = 0). An object does not
// notify, so no dependency is recorded.
func ObjGet(h uint64, cx *Cx, o *Tape) *Reader {
	CheckCx(cx).Flush()
	rc, res := call(*o)
	return readResult(rc, res)
}

// ambientCall runs one generated call of the ambient context at once, with the turn's window
// and app. One that returns nothing changes the app or a window, so a view function may not
// make it; one that returns a value may.
func AmbientCall(cx *Cx, o *Tape, returns bool) *Reader {
	t := CheckCx(cx)
	if !returns && Vb.Active {
		Stop("a view function may not change the app or a window: make this call in a handler or in init")
	}
	t.Flush()
	return &Reader{B: MustCall(*o)}
}

// stateBuilder is a state constructor's chain of `self -> Self` methods: one crossing in all.
type StateBuilder struct {
	cx      *Cx
	o       *Tape
	countAt int
}

func NewStateBuilder(cx *Cx, o *Tape) *StateBuilder {
	at := len(*o)
	o.U16(0) // how many builder ops follow
	return &StateBuilder{cx, o, at}
}

func (b *StateBuilder) Op(code uint16) *Tape {
	if b.o == nil {
		Stop("a builder chain was used after Build: start a new one with its constructor")
	}
	n := le.Uint16((*b.o)[b.countAt:])
	le.PutUint16((*b.o)[b.countAt:], n+1)
	b.o.U16(code)
	return b.o
}

func (b *StateBuilder) Build() uint64 {
	if b.o == nil {
		Stop("Build was called twice on one builder chain: each chain makes one")
	}
	o := b.o
	b.o = nil
	return StateMake(b.cx, o)
}

// Window is a handle to a window.
type Window struct{ HCore }

// WindowStyle is how a window sits among others.
type WindowStyle uint8

const (
	WindowNormal   WindowStyle = 0
	WindowPopup    WindowStyle = 1 // above all other windows
	WindowFloating WindowStyle = 2 // above its parent window
)

// WindowConfig is every option OpenWindowWith takes; zero values are GPUI's defaults.
type WindowConfig struct {
	Width, Height       float32
	Title               string
	MinWidth, MinHeight float32 // 0: no minimum
	Style               WindowStyle
	Position            Option[[2]float32] // none: centred
	NotResizable        bool
	NotMinimizable      bool
	NotMovable          bool
	NoFocus             bool
	Hidden              bool
	KitTitleBar         bool // the kit's title bar options (TitleBar::window_options)
	// AppID names the app to the desktop, which matches the window to the app's desktop entry and
	// its icon by it (Wayland's app_id, X11's WM_CLASS): "myapp" for myapp.desktop. "": none.
	AppID string
	// Icon is the window's icon as the bytes of a PNG (X11 and Windows show it; Wayland takes the
	// icon of the desktop entry AppID names). nil: none. One the library cannot read stops.
	Icon []byte
}

// OpenWindow opens a window titled title, width by height logical pixels, whose root (inside
// the kit's Root) is the view.
//
//	gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(a.init, a.view))
func OpenWindow(cx *Cx, title string, width, height float32, root View) Window {
	return OpenWindowWith(cx, WindowConfig{Width: width, Height: height, Title: title}, root)
}

// OpenWindowWith opens a window with every option (a minimum size, a position, a style, ...).
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

// CloseWindow closes a window (after the handler); a zero Window is the turn's own.
func CloseWindow(cx *Cx, w Window) { windowOp(cx, CWindowClose, w, "") }

// SetWindowTitle changes a window's title.
func SetWindowTitle(cx *Cx, w Window, title string) { windowOp(cx, CWindowSetTitle, w, title) }

// ActivateWindow brings a window to the front.
func ActivateWindow(cx *Cx, w Window) { windowOp(cx, CWindowActivate, w, "") }

// QuitOnLastWindow sets whether closing the last window quits the app (the default).
func QuitOnLastWindow(cx *Cx, yes bool) {
	o := NewReq(CQuitOnLastWindow)
	o.Bool(yes)
	CheckCx(cx).DeferOp(*o)
}

// WindowInfo is a window's title, bounds and whether it is the active one.
type WindowInfo struct {
	Title               string
	X, Y, Width, Height float32
	Active              bool
}

// GetWindowInfo reads a window's title and bounds; a zero Window is the turn's own.
func GetWindowInfo(cx *Cx, w Window) WindowInfo {
	CheckCx(cx).Flush()
	o := NewReq(CWindowInfo)
	o.U64(w.h)
	r := Reader{B: MustCall(*o)}
	return WindowInfo{r.Str(), r.F32(), r.F32(), r.F32(), r.F32(), r.Bool()}
}

// Windows lists the open windows, in the order they were opened.
func Windows(cx *Cx) []Window {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CWindows))}
	return RdList(&r, func(r *Reader) Window { return Window{MkH(r.U64())} })
}

// OnWindowClosed runs f when the window has closed; gk.Release ends it.
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

// OnWindowShouldClose asks f before the user closes the window (its close button, Alt-F4, the
// taskbar; Window::on_window_should_close): f returns false to keep it open, say to ask about
// unsaved edits first, and closes it later with gk.CloseWindow, which does not ask again. Several
// handlers are asked in the order given; the first false keeps the window. A zero Window is the
// turn's own. gk.Release ends it. PROTOCOLS.md 11.38.
//
//	gk.OnWindowShouldClose(cx, gk.Window{}, func(cx *gk.Cx) bool {
//		if !a.dirty { return true }
//		gk.OpenDialog(cx, a.confirmClose) // its OK calls gk.CloseWindow(cx, a.win)
//		return false
//	})
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

// OnAppQuit runs f once as the app quits (App::on_app_quit), however it quits: gk.Quit, the last
// window closed, the platform's quit (Cmd-Q on macOS). It runs with no window and must not wait
// long: gpui gives quit handlers a moment, then exits. Start what must outlive the app (a
// detached process) from here. gk.Release ends it. PROTOCOLS.md 11.38.
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

// ReduceMotion reports whether the app should draw non-essential motion still (gpui's
// App::reduce_motion: the system's setting once gk.BaseApplySystemReduceMotion follows it, or what
// gk.SetReduceMotion set). A view function may read it.
func ReduceMotion(cx *Cx) bool {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CReduceMotion))}
	return r.Bool()
}

// SetReduceMotion sets the reduce-motion flag (App::set_reduce_motion); every window redraws. It
// runs after the handler.
func SetReduceMotion(cx *Cx, reduce bool) {
	o := NewReq(CSetReduceMotion)
	o.Bool(reduce)
	CheckCx(cx).DeferOp(*o)
}

// TestShouldClose asks the window's should-close handlers as the platform does when the user closes
// it, and reports whether it may close. For tests (offscreen there is no close button to press).
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

// OnFrame asks for a FRAME turn before every frame of the turn's window: the only way app code
// runs on every frame, and it is opt-in. What f writes is in place before the views are
// described for that same frame.
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

// Quit ends the app.
func Quit(cx *Cx) { CheckCx(cx).DeferOp(*NewReq(CQuit)) }

// handle makes a focus handle something gk.Release takes.
func (f FocusHandle) handle() uint64 { return f.h }

// NewFocusHandle makes a GPUI FocusHandle: give it to TrackFocus on an element, and move the
// keyboard focus there with gk.Focus.
func NewFocusHandle(cx *Cx) FocusHandle {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not create focus handles: create them in init or in a handler and keep the handle")
	}
	t.Flush()
	r := Reader{B: MustCall(*NewReq(CFocusHandleNew))}
	return FocusHandle{MkH(r.U64())}
}

// Focus moves keyboard focus to the element that tracks the handle.
func Focus(cx *Cx, f FocusHandle) {
	o := NewReq(CFocus)
	o.U64(f.h)
	CheckCx(cx).DeferOp(*o)
}

// Subscription is a handler subscribed to the events of kit state; gk.Release ends it.
type Subscription struct {
	h    uint64
	slot uint32
}

func releaseHandle(t *turn, h uint64) {
	o := NewReq(CRelease)
	o.U64(h)
	t.DeferOp(*o)
}

type Handled interface{ handle() uint64 }

// Release gives back anything held by handle: kit state, a library object, a subscription, a
// focus handle, a view, a row source, a timer, a request that waits, a listener. The handle is
// stale from then on; using it stops with a message, never worse.
func Release(cx *Cx, x any) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not write: it only describes (gk.Release). Release in a handler, in init or in a frame handler")
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
		keptRelease(v.handle())
		releaseHandle(t, v.handle())
	default:
		Stop("gk.Release takes a handle (kit state, a view, a subscription, a row source, a timer, ...), not a %T", x)
	}
}

// Retain keeps a temporary library object (one a read, a call or an event handed out) until
// gk.Release. Retain(cx, obj) returns nothing: the handle is the same.
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

// TestFire has the library fire a kept handler synchronously and returns what it received.
func TestFire(cx *Cx, slot uint32) int32 {
	CheckCx(cx).Flush()
	o := NewReq(CTestFire)
	putKept(o, slot)
	r := Reader{B: MustCall(*o)}
	return int32(r.U32())
}

// TestFireValue fires a handler of the test shape with (text, n) through the library and
// returns the string that came back.
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

// TestFireNoValue fires a handler that returns nothing where the test shape expects a string:
// the library gets the default.
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

// TestFireInUpdate fires a kept handler from inside a state's own update, as a delegate would.
func TestFireInUpdate(cx *Cx, s Handled, slot uint32) int32 {
	CheckCx(cx).Flush()
	o := NewReq(CTestFireInUpdate)
	o.U64(s.handle())
	putKept(o, slot)
	r := Reader{B: MustCall(*o)}
	return int32(r.U32())
}

// TestFireQueued raises an event with payload row from inside a state's own update; the handler
// runs when that update has ended.
func TestFireQueued(cx *Cx, s Handled, h func(cx *Cx, row int), row int) {
	CheckCx(cx).Flush()
	slot, _ := KeepHandler(cx, h, ShapeUsize)
	o := NewReq(CTestFireQueued)
	o.U64(s.handle())
	putKept(o, slot)
	o.U64(uint64(row))
	MustCall(*o)
}

// TestPanic makes the library panic: at once, inside the call, or later, when buffered writes
// are applied outside any call.
func TestPanic(cx *Cx, later bool) {
	t := CheckCx(cx)
	o := NewReq(CTestPanic)
	if later && !debug {
		t.DeferOp(*o)
	} else {
		MustCall(*o)
	}
}

// TestFireShape fires a handler of shape with a payload already on the wire, through the
// library, and returns whether the library decoded it into its Rust type and what the handler
// returned, encoded (the generated shape test).
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

// TestInputState makes an InputState of any handle (tests only: one of another kind shows the
// library's type check).

// Handle returns the raw handle of x (tests only: a fake of another kind shows the check).
func Handle(x Handled) uint64 { return x.handle() }

// MkSubscription is a subscription the library made (generated state events).
func MkSubscription(h uint64, slot uint32) Subscription { return Subscription{h, slot} }
