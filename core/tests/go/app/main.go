// app: the protocol tests of the app family (docs/PROTOCOLS.md section 11), the port of
// core/tests/app: actions and key bindings dispatched to Go handlers, timers between frames,
// several windows, the clipboard. Each mode runs offscreen (the diag library) under a gk-meter
// script and prints lines that core/tests/go/run-app.sh checks, with the exit code.
//
//	GK_PROTO=actions | timers | windows | clipboard | ambient | builtin-app-listener | bad-keys |
//	         builtin-on-action | close-veto | quit-hook | reduce-motion | window-icon | bad-icon
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

const ms = time.Millisecond

type App struct {
	mode    string
	root    gk.View
	focus   gk.FocusHandle
	save    gk.Action
	other   gk.Action
	cancel  gk.Action
	up      gk.Action
	global  gk.ActionListener
	saves   int
	others  int
	cancels int
	// timers
	once      int
	ticks     int
	never     int
	ticker    gk.Timer
	described int
	// windows
	second  gk.Window
	secondV gk.View
	closed  int
	frame   int
	// window close, quit, motion, icon and app id (PROTOCOLS.md 11.38)
	allow  bool
	asked  int
	veto   gk.ActionListener
	quits  int
	motion [3]bool
}

// A 2 x 2 PNG (red and green), a window's icon.
var iconPNG = []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 2, 0, 0, 0, 2, 8, 6, 0, 0, 0, 114, 182, 13, 36, 0, 0, 0, 16, 73, 68, 65, 84, 120, 156, 99, 248, 207, 192, 240, 31, 10, 33, 20, 0, 69, 204, 7, 249, 102, 203, 255, 57, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130}

// --- actions ------------------------------------------------------------------------------------

func (a *App) saved(cx *gk.Cx, act gk.Action) {
	a.saves++
	keys, has := gk.Keystrokes(cx, a.save, "")
	fmt.Printf("app saved n=%d action_ok=%v keys=%q has_keys=%v available=%v\n", a.saves, act == a.save, keys, has,
		gk.ActionAvailable(cx, a.save))
	if a.saves == 1 {
		gk.DispatchAction(cx, a.other) // after this handler, as if ctrl-o had been pressed
	}
}

func (a *App) otherGlobal(cx *gk.Cx, act gk.Action) {
	a.others++
	fmt.Printf("app other n=%d action_ok=%v\n", a.others, act == a.other)
	if a.others == 2 {
		gk.Release(cx, a.global) // the next ctrl-o reaches nobody
		fmt.Println("app other released")
	}
}

func (a *App) cancelled(cx *gk.Cx, act gk.Action) {
	a.cancels++
	fmt.Printf("app cancel n=%d builtin=%v same=%v\n", a.cancels, act.IsBuiltin(), act == a.cancel)
}

func (a *App) captured(cx *gk.Cx, _ gk.Action) {
	gk.Quiet(cx)
	fmt.Println("app capture save")
}

func (a *App) actionsInit(cx *gk.Cx) {
	a.save = gk.NewAction(cx, "app::Save")
	a.other = gk.NewAction(cx, "app::Other")
	a.cancel = gk.NewAction(cx, "ui::Cancel")
	a.up = gk.NewAction(cx, "ui::SelectUp")
	again := gk.NewAction(cx, "app::Save")
	fmt.Printf("app actions save_builtin=%v cancel_builtin=%v same_name_same_id=%v distinct=%v\n",
		a.save.IsBuiltin(), a.cancel.IsBuiltin(), again == a.save, a.save != a.other)
	switch a.mode {
	case "builtin-app-listener":
		gk.OnAppAction(cx, a.cancel, a.cancelled) // stops: app-wide listeners take app actions
	case "bad-keys":
		gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-b", Action: a.save, When: "Editor &&"}) // a context that does not parse: stops when applied
	}
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-s", Action: a.save, When: "Editor"}, gk.KeyBinding{Keys: "ctrl-o", Action: a.other},
		gk.KeyBinding{Keys: "ctrl-x", Action: a.cancel})
	a.global = gk.OnAppAction(cx, a.other, a.otherGlobal)
	a.focus = gk.NewFocusHandle(cx)
	gk.Focus(cx, a.focus)
}

func (a *App) actionsView(cx *gk.Cx) gk.Element {
	root := gk.VFlex(s.SizeFull, s.P4).Bg(theme.Background).TextColor(theme.Foreground).
		KeyContext("Editor").TrackFocus(a.focus).
		CaptureAction(gk.Act(a.save, a.captured)).
		OnAction(gk.Act(a.save, a.saved)).
		OnBoxedAction(a.cancel, a.cancelled).
		Child(kit.Label(fmt.Sprintf("saves %d others %d cancels %d", a.saves, a.others, a.cancels)))
	if a.mode == "builtin-on-action" {
		root = root.OnAction(gk.Act(a.cancel, a.saved)) // stops: OnAction takes app actions
	}
	return root
}

// --- timers -------------------------------------------------------------------------------------

func (a *App) onceFired(cx *gk.Cx) {
	a.once++
	fmt.Printf("app timer once n=%d\n", a.once)
	gk.Release(cx, gk.Timer{}) // nothing: a zero timer is no timer
}

func (a *App) tick(cx *gk.Cx) {
	a.ticks++
	fmt.Printf("app timer tick n=%d\n", a.ticks)
	if a.ticks == 3 {
		gk.Release(cx, a.ticker)
		fmt.Println("app timer ticker released")
	}
}

func (a *App) neverFired(cx *gk.Cx) {
	a.never++
	fmt.Println("app timer never fired (wrong)")
}

func (a *App) timersInit(cx *gk.Cx) {
	gk.After(cx, 30*ms, a.onceFired)
	a.ticker = gk.Every(cx, 20*ms, a.tick)
	t3 := gk.After(cx, 40*ms, a.neverFired)
	gk.Release(cx, t3) // cancelled before it fires
}

func (a *App) timersView(cx *gk.Cx) gk.Element {
	a.described++
	fmt.Printf("app timers view ticks=%d once=%d\n", a.ticks, a.once)
	return gk.Div().Child(kit.Label(fmt.Sprintf("ticks %d", a.ticks)))
}

// --- windows ------------------------------------------------------------------------------------

func (a *App) secondClosed(cx *gk.Cx, w gk.Window) {
	a.closed++
	fmt.Printf("app window closed n=%d same=%v\n", a.closed, w == a.second)
}

func (a *App) closeSecond(cx *gk.Cx) {
	info := gk.GetWindowInfo(cx, a.second)
	fmt.Printf("app second info title=%q w=%.0f h=%.0f windows=%d\n", info.Title, info.Width, info.Height, len(gk.Windows(cx)))
	gk.CloseWindow(cx, a.second)
}

func (a *App) readFirst(cx *gk.Cx) {
	ws := gk.Windows(cx)
	fmt.Printf("app windows after close n=%d\n", len(ws))
	if len(ws) > 0 {
		gk.SetWindowTitle(cx, ws[0], "Renamed")
	}
}

func (a *App) readTitle(cx *gk.Cx) {
	ws := gk.Windows(cx)
	info := gk.GetWindowInfo(cx, ws[0])
	fmt.Printf("app first title=%q\n", info.Title)
}

func (a *App) secondView(cx *gk.Cx) gk.Element {
	fmt.Println("app second view described")
	return gk.Div(s.SizeFull).Child(kit.Label("second window"))
}

func (a *App) openSecond(cx *gk.Cx) {
	a.secondV = gk.NewView(nil, a.secondView)
	// the kit's title bar options (TitleBar::window_options, PROTOCOLS.md 13.21) under the title the app gives
	a.second = gk.OpenWindowWith(cx, gk.WindowConfig{Width: 300, Height: 200, Title: "Second", MinWidth: 100, MinHeight: 80, KitTitleBar: true}, a.secondV)
	gk.OnWindowClosed(cx, a.second, a.secondClosed)
	fmt.Printf("app windows opened n=%d\n", len(gk.Windows(cx)))
}

func (a *App) windowsInit(cx *gk.Cx) {
	gk.After(cx, 10*ms, a.openSecond)
	gk.After(cx, 70*ms, a.closeSecond)
	gk.After(cx, 160*ms, a.readFirst)
	gk.After(cx, 260*ms, a.readTitle)
}

// --- the clipboard ---------------------------------------------------------------------------------

func (a *App) clipWrite(cx *gk.Cx) {
	gk.ClipboardWrite(cx, "hello gk")
}

func (a *App) clipRead(cx *gk.Cx) {
	text, ok := gk.ClipboardRead(cx)
	fmt.Printf("app clipboard read ok=%v text=%q\n", ok, text)
}

func (a *App) clipboardInit(cx *gk.Cx) {
	gk.After(cx, 10*ms, a.clipWrite)
	gk.After(cx, 60*ms, a.clipRead)
}

// --- ambient calls -------------------------------------------------------------------------------

func (a *App) runAmbient(cx *gk.Cx) {
	n := ambientAll(a, cx) // gen_ambient.go: every ambient call once
	fmt.Printf("app ambient called=%d want=%d dialog=%v\n", n, ambientCalls, overlay.HasActiveDialog(cx))
}

func (a *App) ambientView(cx *gk.Cx) gk.Element {
	c := gk.Blue(cx, 500) // a call that returns a value may run in a view function
	fmt.Printf("app ambient view blue=%v\n", c)
	return gk.Div(s.SizeFull).Child(kit.Label("ambient"))
}

// --- PROTOCOLS.md 11.38: a window's close asked, the app's quit, reduced motion, a
// window's icon and app id ----------------------------------------------------------------------------

func (a *App) mayClose(cx *gk.Cx) bool {
	a.asked++
	fmt.Printf("app asked may close n=%d answer=%v\n", a.asked, a.allow)
	return a.allow
}

// saysYes: Go's types leave no handler without an answer; this one answers yes, which lets the
// window close.
func (a *App) saysYes(cx *gk.Cx) bool {
	a.asked++
	fmt.Printf("app asked by a second handler n=%d\n", a.asked)
	return true
}

func (a *App) vetoCheck(cx *gk.Cx) {
	w := gk.Windows(cx)[0]
	a.veto = gk.OnWindowShouldClose(cx, w, a.mayClose)
	first := gk.TestShouldClose(cx, w) // the handler says no
	a.allow = true
	second := gk.TestShouldClose(cx, w) // now yes
	gk.Release(cx, a.veto)
	third := gk.TestShouldClose(cx, w)                          // no handler: it closes
	a.veto = gk.OnWindowShouldClose(cx, gk.Window{}, a.saysYes) // the turn's window
	fourth := gk.TestShouldClose(cx, w)
	fmt.Printf("app should close first=%v second=%v third=%v fourth=%v asked=%d windows=%d\n", first, second, third, fourth,
		a.asked, len(gk.Windows(cx)))
}

func (a *App) quitHook(cx *gk.Cx) {
	a.quits++
	fmt.Printf("app quit hook ran n=%d windows=%d\n", a.quits, len(gk.Windows(cx)))
}

func (a *App) quitReleased(cx *gk.Cx) {
	a.quits++ // counted, so "quits=1" shows it never ran
	fmt.Println("app quit hook ran after its release (wrong)")
}

func (a *App) motionRead(cx *gk.Cx) {
	a.motion[1] = gk.ReduceMotion(cx)
	gk.SetReduceMotion(cx, false)
	gk.After(cx, 20*ms, a.motionReadAgain)
}

func (a *App) motionReadAgain(cx *gk.Cx) {
	a.motion[2] = gk.ReduceMotion(cx)
	fmt.Printf("app reduce motion start=%v set=%v cleared=%v\n", a.motion[0], a.motion[1], a.motion[2])
}

func (a *App) motionView(cx *gk.Cx) gk.Element {
	fmt.Printf("app reduce motion in view=%v\n", gk.ReduceMotion(cx)) // a read a view function may make
	return gk.Div(s.SizeFull).Child(kit.Label("motion"))
}

func (a *App) openIconWindow(cx *gk.Cx) {
	a.secondV = gk.NewView(nil, a.secondView)
	a.second = gk.OpenWindowWith(cx, gk.WindowConfig{Width: 300, Height: 200, Title: "Icon", AppID: "gk-test", Icon: iconPNG}, a.secondV)
	fmt.Printf("app icon window opened n=%d\n", len(gk.Windows(cx)))
}

func (a *App) openBadIcon(cx *gk.Cx) {
	a.secondV = gk.NewView(nil, a.secondView)
	gk.OpenWindowWith(cx, gk.WindowConfig{Title: "Bad", Icon: []byte{1, 2, 3, 4}}, a.secondV) // stops: not a PNG
	fmt.Println("app bad icon accepted (wrong)")
}

func (a *App) portInit(cx *gk.Cx) {
	switch a.mode {
	case "close-veto":
		gk.After(cx, 10*ms, a.vetoCheck)
	case "quit-hook":
		gk.OnAppQuit(cx, a.quitHook)
		gone := gk.OnAppQuit(cx, a.quitReleased)
		gk.Release(cx, gone)
	case "reduce-motion":
		a.motion[0] = gk.ReduceMotion(cx)
		gk.SetReduceMotion(cx, true)
		gk.After(cx, 10*ms, a.motionRead)
	case "window-icon":
		gk.After(cx, 10*ms, a.openIconWindow)
	case "bad-icon":
		gk.After(cx, 10*ms, a.openBadIcon)
	}
}

// --- the app ------------------------------------------------------------------------------------

func (a *App) plainView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(kit.Label("app"))
}

func (a *App) init(cx *gk.Cx) {
	switch a.mode {
	case "timers":
		a.timersInit(cx)
	case "windows":
		a.windowsInit(cx)
	case "clipboard":
		a.clipboardInit(cx)
	case "ambient":
		gk.After(cx, 10*ms, a.runAmbient)
	case "close-veto", "quit-hook", "reduce-motion", "window-icon", "bad-icon":
		a.portInit(cx)
	default:
		a.actionsInit(cx)
	}
}

func (a *App) start(cx *gk.Cx) {
	render := a.actionsView
	switch a.mode {
	case "timers":
		render = a.timersView
	case "windows", "clipboard":
		render = a.plainView
	case "ambient":
		render = a.ambientView
	case "reduce-motion":
		render = a.motionView
	case "close-veto", "quit-hook", "window-icon", "bad-icon":
		render = a.plainView
	}
	a.root = gk.NewView(a.init, render)
	gk.OpenWindow(cx, "app", 640, 480, a.root)
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	script := "size 640 480\nframe\n"
	switch a.mode {
	case "actions", "builtin-on-action":
		// ctrl-s twice (the first dispatches app::Other), ctrl-o twice (the second after the
		// app-wide listener was released: nobody), ctrl-x the built-in ui::Cancel
		script += "key ctrl-s\nkey ctrl-o\nkey ctrl-s\nkey ctrl-o\nkey ctrl-x\nframe\nstat\n"
	case "timers", "clipboard", "ambient", "close-veto", "reduce-motion", "window-icon", "bad-icon":
		// the offscreen clock is simulated: advance moves it on, and due timers fire then
		script += strings.Repeat("advance 25\n", 6) + "frame\nstat\n"
	case "windows":
		script += "advance 30\nwindows\nwindow 1\nframe\nwindow 0\nadvance 60\nwindows\nadvance 100\nadvance 120\nframe\nstat\n"
	default:
		script += "frame\nframe\n"
	}
	gk.RunHeadless(script, a.start)
	if a.mode == "quit-hook" {
		fmt.Printf("app after the run quits=%d\n", a.quits) // the hook ran as the offscreen app shut down
	}
	fmt.Printf("app done mode=%s saves=%d others=%d cancels=%d once=%d ticks=%d never=%d closed=%d\n",
		a.mode, a.saves, a.others, a.cancels, a.once, a.ticks, a.never, a.closed)
}
