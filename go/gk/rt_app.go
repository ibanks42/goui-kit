package gk

// The app-level protocols (docs/PROTOCOLS.md section 11): actions and key bindings, timers, the
// clipboard, background posts and requests that answer later. The library side is
// core/src/app.rs and core/src/platform.rs.
//
//	save := gk.NewAction(cx, "editor::Save")
//	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-s", Action: save, When: "Editor"})
//	gk.Div().KeyContext("Editor").OnAction(gk.Act(save, a.save))
//	gk.Shortcut(cx, "ctrl-n", a.newTask)
//	t := gk.Every(cx, 500*time.Millisecond, a.tick)

import (
	"fmt"
	"os"
	rdebug "runtime/debug"
	"sync"
	"time"
)

const actionBuiltin = 0x8000_0000

// Action is an action of the app's action table, or a built-in one of GPUI or the kit.
type Action struct{ id uint32 }

// IsBuiltin reports whether an action is GPUI's or the kit's rather than the app's.
func (a Action) IsBuiltin() bool { return a.id&actionBuiltin != 0 }

func putAction(o *tape, a Action) { o.u32(a.id) }

// NewAction names an action and returns it: the same name (and data) gives the same action.
// data is the JSON a built-in action with fields takes ("" for none).
func NewAction(cx *Cx, name string, data ...string) Action {
	checkCx(cx).flush()
	o := newReq(cActionNew)
	o.str(name)
	d := ""
	if len(data) > 0 {
		d = data[0]
	}
	o.str(d)
	r := reader{b: mustCall(*o)}
	return Action{r.u32()}
}

// ActionHandler is what an element's OnAction takes: the action to listen for and the handler.
type ActionHandler struct {
	action Action
	fn     func(cx *Cx, a Action)
}

// Act pairs an action with its handler: gk.Div().OnAction(gk.Act(save, a.save)).
func Act(a Action, f func(cx *Cx, a Action)) ActionHandler { return ActionHandler{a, f} }

func putActionHandler(o *tape, a ActionHandler) {
	if a.action.id == 0 {
		stop("OnAction: no action given (make one with gk.NewAction)")
	}
	if a.action.IsBuiltin() {
		stop("OnAction listens for the app's own actions; listen to a built-in action with OnBoxedAction")
	}
	o.u32(a.action.id)
	o.u32(register(a.fn, shapeAction))
}

// KeyBinding is keystrokes ("ctrl-s", "ctrl-k ctrl-c"), the action, and the key context it
// applies in (When: "Editor", "Editor && !Readonly"; "" for everywhere).
type KeyBinding struct {
	Keys   string
	Action Action
	When   string
}

// BindKeys adds key bindings to the app's keymap. A later binding of the same keys wins.
func BindKeys(cx *Cx, bindings ...KeyBinding) {
	t := checkCx(cx)
	o := newReq(cBindKeys)
	o.u32(uint32(len(bindings)))
	for _, b := range bindings {
		if b.Action.id == 0 {
			stop("BindKeys: the binding %q has no action (make one with gk.NewAction)", b.Keys)
		}
		o.str(b.Keys)
		o.u32(b.Action.id)
		o.str(b.When)
	}
	t.deferOp(*o)
}

// ClearKeyBindings removes every key binding, the kit's included.
func ClearKeyBindings(cx *Cx) { checkCx(cx).deferOp(*newReq(cClearKeyBindings)) }

// ActionListener is an app-wide action listener (or a window-closed one); gk.Release ends it.
type ActionListener struct {
	h    uint64
	slot uint32
}

// OnAppAction runs f whenever the app action is dispatched and no element on the focus path
// took it (GPUI's global action listeners).
func OnAppAction(cx *Cx, a Action, f func(cx *Cx, a Action)) ActionListener {
	t := checkCx(cx)
	if a.IsBuiltin() {
		stop("OnAppAction takes the app's own actions; listen to a built-in action on an element with OnBoxedAction")
	}
	t.flush()
	slot, gen := keepHandler(cx, f, shapeAction)
	o := newReq(cOnAction)
	o.u32(a.id)
	o.u32(slot)
	o.u32(gen)
	r := reader{b: mustCall(*o)}
	return ActionListener{r.u64(), slot}
}

// StopPropagation, in the handler of a click, a mouse or a key event, keeps the event from the
// elements around the one whose handler runs: a click on a checkbox in a list row then does
// not also select the row.
func StopPropagation(cx *Cx) {
	t := checkCx(cx)
	if vb.active {
		stop("gk.StopPropagation in a view function: call it in the handler of the event")
	}
	t.flush()
	mustCall(*newReq(cStopPropagation))
}

// ShortcutOption changes a Shortcut.
type ShortcutOption func(*shortcutOpts)

type shortcutOpts struct{ when string }

// WhenIn limits a shortcut's keys to a key context: only while the focus is inside an element
// with .KeyContext(ctx).
func WhenIn(ctx string) ShortcutOption { return func(o *shortcutOpts) { o.when = ctx } }

var shortcutN int

// ShortcutHandler is what gk.Shortcut runs: a func(cx), or a click handler (the method a
// button's OnClick takes, so one method serves the button and its keys; it gets a ClickEvent of
// Kind ClickKeyboard).
type ShortcutHandler interface {
	func(cx *Cx) | func(cx *Cx, e ClickEvent)
}

// Shortcut is a keyboard shortcut in one call: it makes an app action, binds keys to it and runs
// f when they are pressed, wherever the focus is (an app-wide listener, so it keeps working
// after a dialog or a click moved the focus). It returns the action, for a menu item (the menu
// shows the keys) or for more keys (gk.BindKeys). Given in a view's init, f redraws that view.
//
//	a.newTask = gk.Shortcut(cx, "ctrl-n", a.addTask)              // func (a *App) addTask(cx *gk.Cx)
//	gk.Shortcut(cx, "ctrl-t", a.toggleTheme)                       // also a button's OnClick
//	gk.Shortcut(cx, "delete", a.deleteTask, gk.WhenIn("Tasks"))
func Shortcut[F ShortcutHandler](cx *Cx, keys string, f F, opts ...ShortcutOption) Action {
	checkCx(cx)
	if vb.active {
		stop("a view function may not make shortcuts: make them in init or in a handler")
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
		stop("gk.Shortcut was given a nil handler: give a func(cx *gk.Cx) or a click handler, as in gk.Shortcut(cx, \"ctrl-s\", a.save)")
	}
	var so shortcutOpts
	for _, o := range opts {
		o(&so)
	}
	shortcutN++
	a := NewAction(cx, fmt.Sprintf("gk::shortcut::go%d", shortcutN))
	OnAppAction(cx, a, func(cx *Cx, _ Action) { run(cx) })
	if keys != "" {
		BindKeys(cx, KeyBinding{keys, a, so.when})
	}
	return a
}

// DispatchAction dispatches an action to the focused element of the turn's window (as its key
// binding would), or app-wide in a turn without a window. It runs after the handler.
func DispatchAction(cx *Cx, a Action) {
	o := newReq(cDispatchAction)
	o.u32(a.id)
	checkCx(cx).deferOp(*o)
}

// ActionAvailable reports whether something on the focus path, or app-wide, takes the action.
func ActionAvailable(cx *Cx, a Action) bool {
	checkCx(cx).flush()
	o := newReq(cActionAvailable)
	o.u32(a.id)
	r := reader{b: mustCall(*o)}
	return r.u8() != 0
}

// Keystrokes gives the keys of the action's binding that wins ("ctrl-s"), and false when it has
// none. keyContext: only bindings of that context ("" for any).
func Keystrokes(cx *Cx, a Action, keyContext string) (string, bool) {
	checkCx(cx).flush()
	o := newReq(cActionKeystrokes)
	o.u32(a.id)
	o.str(keyContext)
	r := reader{b: mustCall(*o)}
	if r.u8() == 0 {
		return "", false
	}
	return r.str(), true
}

// --- timers ---------------------------------------------------------------------------------------

// Timer fires a handler between frames, never in the middle of one. gk.Release cancels it.
type Timer struct {
	h    uint64
	slot uint32
}

var (
	timersOnce = map[uint32]uint64{} // kept slot -> timer, one-shot timers that have not fired
	timersLive = map[uint64]uint32{} // timer -> kept slot
)

// After runs f once, d from now.
func After(cx *Cx, d time.Duration, f func(cx *Cx)) Timer { return timerNew(cx, d, false, f) }

// Every runs f every d until the timer is released.
func Every(cx *Cx, d time.Duration, f func(cx *Cx)) Timer { return timerNew(cx, d, true, f) }

func timerNew(cx *Cx, d time.Duration, repeat bool, f func(cx *Cx)) Timer {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not start timers: start them in init or in a handler")
	}
	ms := d.Milliseconds()
	if ms < 0 || ms > int64(^uint32(0)) || repeat && ms == 0 {
		stop("a timer of %v: give 1 ms to 49 days (gk.After may also take 0)", d)
	}
	t.flush()
	slot, gen := keepHandler(cx, f, shapeUnit)
	o := newReq(cTimerNew)
	o.u32(uint32(ms))
	o.bool(repeat)
	o.u32(slot)
	o.u32(gen)
	r := reader{b: mustCall(*o)}
	tm := Timer{r.u64(), slot}
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
		return // a one-shot timer that has fired, or one released before
	}
	delete(timersLive, tm.h)
	delete(timersOnce, slot)
	dropHandler(slot)
	releaseHandle(t, tm.h)
}

// --- the clipboard ------------------------------------------------------------------------------------

// ClipboardWrite puts text on the clipboard.
func ClipboardWrite(cx *Cx, text string) {
	o := newReq(cClipboardWrite)
	o.str(text)
	checkCx(cx).deferOp(*o)
}

// ClipboardRead gives the clipboard's text, and false when it holds none.
func ClipboardRead(cx *Cx) (string, bool) {
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cClipboardRead))}
	if r.u8() == 0 {
		return "", false
	}
	return r.str(), true
}

// --- background work ------------------------------------------------------------------------------

var (
	postMu    sync.Mutex
	postQueue []func(*Cx)
)

// Post runs f on the UI thread, in a WAKE turn, with a cx. Any goroutine may call it; it is the
// only function of this package that may be called off the UI thread. However fast posts come,
// the library makes at most one WAKE turn per turn of its main loop, and the views they mark are
// described once, before the next frame.
//
//	go func() { items := load(path); gk.Post(func(cx *gk.Cx) { a.items = items; gk.Refresh(cx) }) }()
func Post(f func(cx *Cx)) {
	if f == nil {
		return
	}
	postMu.Lock()
	first := len(postQueue) == 0
	postQueue = append(postQueue, f)
	postMu.Unlock()
	if first {
		// Before gk.Run has started the library's loop this fails and is ignored: the first
		// turn drains the queue.
		postWake()
	}
}

// Go runs work on a new goroutine and done with its result on the UI thread.
//
//	gk.Go(cx, func() []Article { return parse(path) }, func(cx *gk.Cx, as []Article) { a.articles = as; gk.Refresh(cx) })
func Go[T any](cx *Cx, work func() T, done func(cx *Cx, v T)) {
	checkCx(cx)
	if work == nil || done == nil {
		stop("gk.Go needs a work function and a done function: gk.Go(cx, func() T { ... }, func(cx *gk.Cx, v T) { ... })")
	}
	file, line, _ := appFrame(2)
	go func() {
		// A panic in work would end the program with Go's goroutine dump; say whose it is first.
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "gk: panic in the work function of the gk.Go at %s:%d: %v\n", file, line, r)
				if f, l, fn := appFrame(3); l != 0 {
					fmt.Fprintf(os.Stderr, "    at %s:%d (%s)\n", f, l, fn)
				}
				fmt.Fprintf(os.Stderr, "    The app stops here (exit 2). Go's stack follows.\n%s", rdebug.Stack())
				os.Exit(2)
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

// drainPosts runs the queued posts, or the first limit of them (the rest stay queued).
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
		postWake() // the posts left behind may not have asked for a wake
	}
	for _, f := range queue {
		f(cx)
	}
}

// --- requests that answer later (PROTOCOLS.md 11.30) -----------------------------------------------

// DoneStatus says what happened to a request.
type DoneStatus uint8

const (
	DoneAnswered  DoneStatus = 0 // the user chose (paths, a button)
	DoneCancelled DoneStatus = 1 // the user dismissed it
	DoneFailed    DoneStatus = 2 // the platform could not show it: Error says why
)

// PathsDone is what a file dialog answers.
type PathsDone struct {
	Status DoneStatus
	Paths  []string
	Error  string
}

// PromptDone is what a prompt answers: the index of the button chosen.
type PromptDone struct {
	Status DoneStatus
	Answer uint32
}

// Completion is a request that answers later. gk.Release cancels it while it waits.
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

func completionBegin(cx *Cx, code uint16, h any, shape uint16) (*tape, uint32, uint32) {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not start a request: start it in init or in a handler")
	}
	t.flush()
	slot, gen := keepHandler(cx, h, shape)
	return newReq(code), slot, gen
}

func completionEnd(o *tape, slot, gen uint32) Completion {
	o.u32(slot)
	o.u32(gen)
	r := reader{b: mustCall(*o)}
	c := Completion{r.u64(), slot}
	completions[c.h] = completionLive
	completionOf[slot] = c.h
	return c
}

func putOptStr(o *tape, s string) {
	if s == "" {
		o.u8(0)
	} else {
		o.u8(1)
		o.str(s)
	}
}

// PathPrompt is what a file dialog may choose. Prompt: the text of its button, "" for the platform's.
type PathPrompt struct {
	Files, Directories, Multiple bool
	Prompt                       string
}

// PromptForPaths shows the platform's dialog for opening files or directories; f gets the answer.
func PromptForPaths(cx *Cx, p PathPrompt, f func(cx *Cx, d PathsDone)) Completion {
	if !p.Files && !p.Directories {
		stop("gk.PromptForPaths was given a dialog that may choose neither files nor directories: set Files or Directories in the gk.PathPrompt")
	}
	o, slot, gen := completionBegin(cx, cPromptPaths, f, shapePaths)
	o.bool(p.Files)
	o.bool(p.Directories)
	o.bool(p.Multiple)
	putOptStr(o, p.Prompt)
	return completionEnd(o, slot, gen)
}

// PromptForNewPath shows the platform's dialog for saving a new file, starting in directory.
func PromptForNewPath(cx *Cx, directory, suggestedName string, f func(cx *Cx, d PathsDone)) Completion {
	o, slot, gen := completionBegin(cx, cPromptNewPath, f, shapePaths)
	o.str(directory)
	putOptStr(o, suggestedName)
	return completionEnd(o, slot, gen)
}

// PromptLevel is how serious a prompt is.
type PromptLevel uint8

const (
	PromptInfo     PromptLevel = 0
	PromptWarning  PromptLevel = 1
	PromptCritical PromptLevel = 2
)

// PromptButtonKind marks a prompt's button as the OK or Cancel one.
type PromptButtonKind uint8

const (
	ButtonOther  PromptButtonKind = 0
	ButtonOk     PromptButtonKind = 1
	ButtonCancel PromptButtonKind = 2
)

// PromptButton is one answer of a prompt.
type PromptButton struct {
	Kind  PromptButtonKind
	Label string
}

// Prompt asks a question in the turn's window with the platform's dialog where it has one and
// GPUI's own otherwise; f gets the index of the button chosen.
func Prompt(cx *Cx, level PromptLevel, message, detail string, answers []PromptButton, f func(cx *Cx, d PromptDone)) Completion {
	if len(answers) == 0 {
		stop("gk.Prompt was given no answers: give at least one button, as in []gk.PromptButton{{Label: \"OK\"}}")
	}
	o, slot, gen := completionBegin(cx, cWindowPrompt, f, shapePrompt)
	o.u64(0)
	o.u8(uint8(level))
	o.str(message)
	putOptStr(o, detail)
	o.u32(uint32(len(answers)))
	for _, a := range answers {
		o.u8(uint8(a.Kind))
		o.str(a.Label)
	}
	return completionEnd(o, slot, gen)
}

func releaseCompletion(t *turn, c Completion) {
	state, ok := completions[c.h]
	if !ok {
		stop("this completion is not one the app holds (released twice, or not made by a request): release the gk.Completion a request returned, once")
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
		stop("this completion was released already (a request is released once)")
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

// doneTurn delivers an answer: the kept handler runs as for an event, then the completion is done.
func doneTurn(cx *Cx, r *reader) {
	peek := *r
	peek.u64()
	peek.u32()
	slot := peek.u32()
	event(cx, r)
	if h, ok := completionOf[slot]; ok {
		delete(completionOf, slot)
		completions[h] = completionDone
		dropHandler(slot)
		releaseHandle(cx.t, h)
	}
}

// OpenURL has the platform open a URL with the user's browser. It runs after the handler.
func OpenURL(cx *Cx, url string) {
	o := newReq(cOpenUrl)
	o.str(url)
	checkCx(cx).deferOp(*o)
}

// PlatformAnswerPaths answers the oldest waiting file dialog (offscreen, or with
// GK_SCRIPTED_PLATFORM=1: tests). It reports whether one was waiting.
func PlatformAnswerPaths(cx *Cx, status DoneStatus, paths []string, newPath bool, errText string) bool {
	checkCx(cx).flush()
	o := newReq(cPlatformAnswer)
	if newPath {
		o.u8(2)
	} else {
		o.u8(1)
	}
	o.u8(uint8(status))
	o.u32(uint32(len(paths)))
	for _, p := range paths {
		o.str(p)
	}
	o.str(errText)
	r := reader{b: mustCall(*o)}
	return r.u8() != 0
}

// PlatformAnswerPrompt answers the oldest waiting prompt with the index of a button.
func PlatformAnswerPrompt(cx *Cx, status DoneStatus, answer int) bool {
	checkCx(cx).flush()
	o := newReq(cPlatformAnswer)
	o.u8(3)
	o.u8(uint8(status))
	o.u32(uint32(answer))
	r := reader{b: mustCall(*o)}
	return r.u8() != 0
}
