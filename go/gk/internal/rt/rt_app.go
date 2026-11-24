package rt

import (
	"fmt"
	rdebug "runtime/debug"
	"sync"
	"time"
)

// High bit set by the Rust core on built-in kit actions to distinguish them
// from app-defined actions created with NewAction.
const actionBuiltin = 0x8000_0000

// Action identifies an application or built-in kit action registered with GPUI.
type Action struct{ id uint32 }

// IsBuiltin reports whether a refers to a built-in kit action rather than an
// app-defined action.
func (a Action) IsBuiltin() bool { return a.id&actionBuiltin != 0 }

// PutAction encodes an Action ID onto o.
func PutAction(o *Tape, a Action) { o.U32(a.id) }

// NewAction registers a new named app action with optional payload data and
// returns its Action handle.
func NewAction(cx *Cx, name string, data ...string) Action {
	CheckCx(cx).Flush()
	o := NewReq(CActionNew)
	o.Str(name)
	d := ""
	if len(data) > 0 {
		d = data[0]
	}
	o.Str(d)
	r := Reader{B: MustCall(*o)}
	return Action{r.U32()}
}

// KitActionValue is implemented by generated kit action value types accepted by
// ActionOf.
type KitActionValue interface{ valueNode() *Node }

// ActionOf converts a kit action value into an Action handle outside a view
// function so it can be bound to keys or dispatched.
func ActionOf(cx *Cx, v KitActionValue) Action {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("gk.ActionOf in a view function: name the action in init or a handler and keep the gk.Action")
	}
	t.Flush()
	o := NewReq(CValueAction)
	PutVtape(o, v.valueNode())
	r := Reader{B: MustCall(*o)}
	return Action{r.U32()}
}

// ActionHandler pairs an Action with the callback invoked when it fires on an
// element.
type ActionHandler struct {
	action Action
	fn     func(cx *Cx, a Action)
}

// Act constructs an ActionHandler binding a to f.
func Act(a Action, f func(cx *Cx, a Action)) ActionHandler { return ActionHandler{a, f} }

// PutActionHandler encodes an element-level ActionHandler onto o and registers
// its callback in the active view build's handler table.
func PutActionHandler(o *Tape, a ActionHandler) {
	if a.action.id == 0 {
		Stop("OnAction: no action given (make one with gk.NewAction)")
	}
	if a.action.IsBuiltin() {
		Stop("OnAction listens for the app's own actions; listen to a built-in action with OnBoxedAction")
	}
	o.U32(a.action.id)
	o.U32(Register(a.fn, ShapeAction))
}

// KeyBinding maps a keystroke string to an Action, optionally restricted to a
// GPUI key context expression in When.
type KeyBinding struct {
	Keys   string
	Action Action
	When   string
}

// BindKeys registers one or more key bindings with the application.
func BindKeys(cx *Cx, bindings ...KeyBinding) {
	t := CheckCx(cx)
	o := NewReq(CBindKeys)
	o.U32(uint32(len(bindings)))
	for _, b := range bindings {
		if b.Action.id == 0 {
			Stop("BindKeys: the binding %q has no action (make one with gk.NewAction)", b.Keys)
		}
		o.Str(b.Keys)
		o.U32(b.Action.id)
		o.Str(b.When)
	}
	t.DeferOp(*o)
}

// ClearKeyBindings removes all key bindings registered by the application.
func ClearKeyBindings(cx *Cx) { CheckCx(cx).DeferOp(*NewReq(CClearKeyBindings)) }

// ActionListener is a releasable handle to a global action, window-close, or
// app-quit callback.
type ActionListener struct {
	h    uint64
	slot uint32
}

// OnAppAction registers a global handler for app-defined action a.
func OnAppAction(cx *Cx, a Action, f func(cx *Cx, a Action)) ActionListener {
	t := CheckCx(cx)
	if a.IsBuiltin() {
		Stop("OnAppAction takes the app's own actions; listen to a built-in action on an element with OnBoxedAction")
	}
	t.Flush()
	slot, gen := KeepHandler(cx, f, ShapeAction)
	o := NewReq(COnAction)
	o.U32(a.id)
	o.U32(slot)
	o.U32(gen)
	r := Reader{B: MustCall(*o)}
	return ActionListener{r.U64(), slot}
}

// StopPropagation prevents the current event from bubbling to ancestor elements.
func StopPropagation(cx *Cx) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("gk.StopPropagation in a view function: call it in the handler of the event")
	}
	t.Flush()
	MustCall(*NewReq(CStopPropagation))
}

// ShortcutOption configures optional settings on Shortcut.
type ShortcutOption func(*ShortcutOpts)

// ShortcutOpts holds options applied when registering a keyboard shortcut.
type ShortcutOpts struct{ When string }

// WhenIn restricts a Shortcut to the given GPUI key context expression.
func WhenIn(ctx string) ShortcutOption { return func(o *ShortcutOpts) { o.When = ctx } }

var shortcutN int

// ShortcutHandler constrains Shortcut callbacks to plain or click handlers.
type ShortcutHandler interface {
	func(cx *Cx) | func(cx *Cx, e ClickEvent)
}

// Shortcut creates a unique Action, registers f as its global handler, and
// binds keys (when non-empty) to that Action.
func Shortcut[F ShortcutHandler](cx *Cx, keys string, f F, opts ...ShortcutOption) Action {
	CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not make shortcuts: make them in init or in a handler")
	}
	var run func(cx *Cx)
	switch h := any(f).(type) {
	case func(*Cx):
		run = h
	case func(*Cx, ClickEvent):
		if h != nil {
			run = func(cx *Cx) { h(cx, ClickEvent{Button: ButtonNone, Kind: ClickKeyboard}) }
		}
	}
	if run == nil {
		Stop("gk.Shortcut was given a nil handler: give a func(cx *gk.Cx) or a click handler, as in gk.Shortcut(cx, \"ctrl-s\", a.save)")
	}
	var so ShortcutOpts
	for _, o := range opts {
		o(&so)
	}
	shortcutN++
	a := NewAction(cx, fmt.Sprintf("gk::shortcut::go%d", shortcutN))
	OnAppAction(cx, a, func(cx *Cx, _ Action) { run(cx) })
	if keys != "" {
		BindKeys(cx, KeyBinding{keys, a, so.When})
	}
	return a
}

// DispatchAction dispatches action a to the currently focused element chain.
func DispatchAction(cx *Cx, a Action) {
	o := NewReq(CDispatchAction)
	o.U32(a.id)
	CheckCx(cx).DeferOp(*o)
}

// ActionAvailable reports whether any focused element or global listener can
// handle action a.
func ActionAvailable(cx *Cx, a Action) bool {
	CheckCx(cx).Flush()
	o := NewReq(CActionAvailable)
	o.U32(a.id)
	r := Reader{B: MustCall(*o)}
	return r.U8() != 0
}

// Keystrokes returns the primary keystroke string bound to action a in
// keyContext, and whether a binding exists.
func Keystrokes(cx *Cx, a Action, keyContext string) (string, bool) {
	CheckCx(cx).Flush()
	o := NewReq(CActionKeystrokes)
	o.U32(a.id)
	o.Str(keyContext)
	r := Reader{B: MustCall(*o)}
	if r.U8() == 0 {
		return "", false
	}
	return r.Str(), true
}

// Timer is a releasable handle to a one-shot or repeating UI-thread timer.
type Timer struct {
	h    uint64
	slot uint32
}

// timersOnce tracks one-shot timers by handler slot so timerFired can drop the
// slot automatically after the callback runs. timersLive maps all active timer
// handles to their handler slots for cancellation via Release.
var (
	timersOnce = map[uint32]uint64{}
	timersLive = map[uint64]uint32{}
)

// After schedules f to run once on the UI thread after duration d.
func After(cx *Cx, d time.Duration, f func(cx *Cx)) Timer { return timerNew(cx, d, false, f) }

// Every schedules f to run repeatedly on the UI thread at interval d (at least 1 ms).
func Every(cx *Cx, d time.Duration, f func(cx *Cx)) Timer { return timerNew(cx, d, true, f) }

func timerNew(cx *Cx, d time.Duration, repeat bool, f func(cx *Cx)) Timer {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not start timers: start them in init or in a handler")
	}
	ms := d.Milliseconds()
	if ms < 0 || ms > int64(^uint32(0)) || repeat && ms == 0 {
		Stop("a timer of %v: give 1 ms to 49 days (gk.After may also take 0)", d)
	}
	t.Flush()
	slot, gen := KeepHandler(cx, f, ShapeUnit)
	o := NewReq(CTimerNew)
	o.U32(uint32(ms))
	o.Bool(repeat)
	o.U32(slot)
	o.U32(gen)
	r := Reader{B: MustCall(*o)}
	tm := Timer{r.U64(), slot}
	timersLive[tm.h] = slot
	if !repeat {
		timersOnce[slot] = tm.h
	}
	return tm
}

func timerOfSlot(slot uint32) (uint64, bool) {
	for th, s := range timersLive {
		if s == slot {
			return th, true
		}
	}
	return 0, false
}

// timerFired drops the kept handler slot for a one-shot timer once it has run.
func timerFired(slot uint32) {
	if th, ok := timersOnce[slot]; ok {
		delete(timersOnce, slot)
		delete(timersLive, th)
		dropHandler(slot)
	}
}

func releaseTimer(t *turn, tm Timer) {
	slot, live := timersLive[tm.h]
	if !live {
		return
	}
	delete(timersLive, tm.h)
	delete(timersOnce, slot)
	dropHandler(slot)
	releaseHandle(t, tm.h)
}

// ClipboardWrite writes text to the system clipboard.
func ClipboardWrite(cx *Cx, text string) {
	o := NewReq(CClipboardWrite)
	o.Str(text)
	CheckCx(cx).DeferOp(*o)
}

// ClipboardRead reads text from the system clipboard, returning false when the
// clipboard has no text content.
func ClipboardRead(cx *Cx) (string, bool) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CClipboardRead))}
	if r.U8() == 0 {
		return "", false
	}
	return r.Str(), true
}

// Thread-safe queue of callbacks submitted from any goroutine via Post.
var (
	postMu    sync.Mutex
	postQueue []func(*Cx)
)

// Post schedules f to run on the UI thread during the next KWake turn.
// Post is safe to call from any goroutine; it wakes the event loop only on
// the transition from an empty queue to a non-empty queue.
func Post(f func(cx *Cx)) {
	if f == nil {
		return
	}
	postMu.Lock()
	first := len(postQueue) == 0
	postQueue = append(postQueue, f)
	postMu.Unlock()
	if first {
		postWake()
	}
}

// Go runs work in a background goroutine and posts done back to the UI thread
// with its result. Panics in work are captured into a *PanicError, latched on
// the runtime, and delivered by waking the UI loop to exit cleanly.
func Go[T any](cx *Cx, work func() T, done func(cx *Cx, v T)) {
	CheckCx(cx)
	if work == nil || done == nil {
		Stop("gk.Go needs a work function and a done function: gk.Go(cx, func() T { ... }, func(cx *gk.Cx, v T) { ... })")
	}
	file, line, _ := appFrame(2)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				p := &PanicError{Value: r, Where: fmt.Sprintf("the work function of the gk.Go at %s:%d", file, line), Stack: rdebug.Stack()}
				p.File, p.Line, p.Func = appFrame(3)
				fail(p)
				postWake()
			}
		}()
		v := work()
		Post(func(cx *Cx) { done(cx, v) })
	}()
}

func postsQueued() int {
	postMu.Lock()
	defer postMu.Unlock()
	return len(postQueue)
}

// drainPosts executes up to limit queued Post callbacks on the UI thread (or
// all queued callbacks when limit < 0). If items remain, it wakes the loop again.
func drainPosts(cx *Cx, limit int) {
	if limit == 0 {
		return
	}
	postMu.Lock()
	queue := postQueue
	postQueue = nil
	left := 0
	if limit > 0 && limit < len(queue) {
		postQueue = append(postQueue, queue[limit:]...)
		queue = queue[:limit]
		left = len(postQueue)
	}
	postMu.Unlock()
	if left > 0 {
		postWake()
	}
	for _, f := range queue {
		f(cx)
	}
}

// DoneStatus reports the outcome of an asynchronous platform dialog request.
type DoneStatus uint8

// Outcomes returned in PathsDone.Status and PromptDone.Status.
const (
	DoneAnswered  DoneStatus = 0
	DoneCancelled DoneStatus = 1
	DoneFailed    DoneStatus = 2
)

// PathsDone is the result passed to PromptForPaths and PromptForNewPath callbacks.
type PathsDone struct {
	Status DoneStatus
	Paths  []string
	Error  string
}

// PromptDone is the result passed to Prompt callbacks, with Answer indexing
// into the answers slice supplied to Prompt.
type PromptDone struct {
	Status DoneStatus
	Answer uint32
}

// Completion represents an in-flight asynchronous platform dialog request that
// can be canceled by passing it to Release.
type Completion struct {
	h    uint64
	slot uint32
}

type completionState uint8

const (
	completionLive completionState = iota
	completionDone
	completionReleased
)

var (
	completions  = map[uint64]completionState{}
	completionOf = map[uint32]uint64{}
)

func completionBegin(cx *Cx, code uint16, h any, shape uint16) (*Tape, uint32, uint32) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not start a request: start it in init or in a handler")
	}
	t.Flush()
	slot, gen := KeepHandler(cx, h, shape)
	return NewReq(code), slot, gen
}

func completionEnd(o *Tape, slot, gen uint32) Completion {
	o.U32(slot)
	o.U32(gen)
	r := Reader{B: MustCall(*o)}
	c := Completion{r.U64(), slot}
	completions[c.h] = completionLive
	completionOf[slot] = c.h
	return c
}

func putOptStr(o *Tape, s string) {
	if s == "" {
		o.U8(0)
	} else {
		o.U8(1)
		o.Str(s)
	}
}

// PathPrompt configures a native open-file or open-directory dialog.
type PathPrompt struct {
	Files, Directories, Multiple bool
	Prompt                       string
}

// PromptForPaths opens a native file or directory picker and invokes f when
// the user finishes or cancels.
func PromptForPaths(cx *Cx, p PathPrompt, f func(cx *Cx, d PathsDone)) Completion {
	if !p.Files && !p.Directories {
		Stop("gk.PromptForPaths was given a dialog that may choose neither files nor directories: set Files or Directories in the gk.PathPrompt")
	}
	o, slot, gen := completionBegin(cx, CPromptPaths, f, ShapePaths)
	o.Bool(p.Files)
	o.Bool(p.Directories)
	o.Bool(p.Multiple)
	putOptStr(o, p.Prompt)
	return completionEnd(o, slot, gen)
}

// PromptForNewPath opens a native save-file dialog starting in directory with
// an optional suggestedName and invokes f when complete.
func PromptForNewPath(cx *Cx, directory, suggestedName string, f func(cx *Cx, d PathsDone)) Completion {
	o, slot, gen := completionBegin(cx, CPromptNewPath, f, ShapePaths)
	o.Str(directory)
	putOptStr(o, suggestedName)
	return completionEnd(o, slot, gen)
}

// PromptLevel sets the severity icon of a native alert dialog.
type PromptLevel uint8

// Severity levels accepted by Prompt.
const (
	PromptInfo     PromptLevel = 0
	PromptWarning  PromptLevel = 1
	PromptCritical PromptLevel = 2
)

// PromptButtonKind classifies a button in a native alert dialog.
type PromptButtonKind uint8

// Button kinds accepted in PromptButton.Kind.
const (
	ButtonOther  PromptButtonKind = 0
	ButtonOk     PromptButtonKind = 1
	ButtonCancel PromptButtonKind = 2
)

// PromptButton defines one button shown by Prompt.
type PromptButton struct {
	Kind  PromptButtonKind
	Label string
}

// Prompt displays a native modal alert dialog with the given buttons and
// invokes f with the chosen button index or cancellation status.
func Prompt(cx *Cx, level PromptLevel, message, detail string, answers []PromptButton, f func(cx *Cx, d PromptDone)) Completion {
	if len(answers) == 0 {
		Stop("gk.Prompt was given no answers: give at least one button, as in []gk.PromptButton{{Label: \"OK\"}}")
	}
	o, slot, gen := completionBegin(cx, CWindowPrompt, f, ShapePrompt)
	o.U64(0)
	o.U8(uint8(level))
	o.Str(message)
	putOptStr(o, detail)
	o.U32(uint32(len(answers)))
	for _, a := range answers {
		o.U8(uint8(a.Kind))
		o.Str(a.Label)
	}
	return completionEnd(o, slot, gen)
}

func releaseCompletion(t *turn, c Completion) {
	state, ok := completions[c.h]
	if !ok {
		Stop("this completion is not one the app holds (released twice, or not made by a request): release the gk.Completion a request returned, once")
	}
	switch state {
	case completionLive:
		completions[c.h] = completionReleased
		delete(completionOf, c.slot)
		dropHandler(c.slot)
		releaseHandle(t, c.h)
	case completionDone:
		delete(completions, c.h)
	case completionReleased:
		Stop("this completion was released already (a request is released once)")
	}
}

func completionDropKept(t *turn, slot uint32) bool {
	h, ok := completionOf[slot]
	if !ok {
		return false
	}
	releaseCompletion(t, Completion{h, slot})
	return true
}

// doneTurn delivers a KDone completion callback and transitions the completion
// handle to completionDone so a subsequent explicit Release is harmless.
func doneTurn(cx *Cx, r *Reader) {
	peek := *r
	peek.U64()
	peek.U32()
	slot := peek.U32()
	event(cx, r)
	if h, ok := completionOf[slot]; ok {
		delete(completionOf, slot)
		completions[h] = completionDone
		dropHandler(slot)
		releaseHandle(cx.t, h)
	}
}

// OpenURL opens url in the system's default handler.
func OpenURL(cx *Cx, url string) {
	o := NewReq(COpenUrl)
	o.Str(url)
	CheckCx(cx).DeferOp(*o)
}

// PlatformAnswerPaths supplies a scripted answer to an open or save path dialog
// in headless diagnostic tests.
func PlatformAnswerPaths(cx *Cx, status DoneStatus, paths []string, newPath bool, errText string) bool {
	CheckCx(cx).Flush()
	o := NewReq(CPlatformAnswer)
	if newPath {
		o.U8(2)
	} else {
		o.U8(1)
	}
	o.U8(uint8(status))
	o.U32(uint32(len(paths)))
	for _, p := range paths {
		o.Str(p)
	}
	o.Str(errText)
	r := Reader{B: MustCall(*o)}
	return r.U8() != 0
}

// PlatformAnswerPrompt supplies a scripted button answer to a modal prompt in
// headless diagnostic tests.
func PlatformAnswerPrompt(cx *Cx, status DoneStatus, answer int) bool {
	CheckCx(cx).Flush()
	o := NewReq(CPlatformAnswer)
	o.U8(3)
	o.U8(uint8(status))
	o.U32(uint32(answer))
	r := Reader{B: MustCall(*o)}
	return r.U8() != 0
}

// MkAction constructs an Action from a raw action ID returned by the library.
func MkAction(id uint32) Action { return Action{id} }
