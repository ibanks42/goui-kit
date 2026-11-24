// review3: five attacks on the protocols built since milestone 2 (timers, windows, actions, dock
// panels, library objects behind handles), written by the review of 12 November 2026; the port of
// core/tests/review3. Each runs offscreen (the diag library) under a gk-meter script;
// core/tests/go/run-review3.sh checks the lines and the exit code. None may crash, hang, or run
// app code against a view that is gone.
//
//	GK_PROTO=timer-after-view  a second window's view starts a repeating timer and an app-wide
//	                           listener in its init; the window closes and the view is released:
//	                           neither may run again with the released view's state
//	GK_PROTO=close-own-window  a key binding's handler in a second window closes that window and
//	                           dispatches another action in the same turn
//	GK_PROTO=dock-panel-freed  the view of a docked panel is released while the dock shows it; the
//	                           dock draws, is clicked, and is laid out again without the panel
//	GK_PROTO=reentrant-action  an app-wide listener dispatches its own action again, releases
//	                           itself and adds another listener while it runs, in a turn with a
//	                           window (a key) and in one without (a timer started before any window)
//	GK_PROTO=handle-after-owner a dock's layout and a dock context kept past the release of the dock
//
// handle-after-owner is in owner.go.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gpuikit.dev/gk"
	gkdock "gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
)

const ms = time.Millisecond

type Child struct {
	ticks    int
	wrong    int
	pings    int
	dead     bool
	timer    gk.Timer
	listener gk.ActionListener
	ping     gk.Action
}

type App struct {
	mode  string
	root  gk.View
	frame int
	// timer-after-view
	child  Child
	childV gk.View
	win    gk.Window
	final  int
	// close-own-window
	close   gk.Action
	ping    gk.Action
	focus2  gk.FocusHandle
	second  gk.Window
	secondV gk.View
	closes  int
	closed  int
	pings   int
	// dock-panel-freed, handle-after-owner
	dock     gkdock.Area
	pa, pb   gkdock.Panel
	paV, pbV gk.View
	aDrawn   int
	aAfter   int
	freed    bool
	dockGone bool
	tree     gkdock.PaneTree
	dockCtx  gkdock.Context
	hasCtx   bool
	// reentrant-action
	l1, l2 gk.ActionListener
	n1, n2 int
	depth  int
	nested int
	phase  int
	limit  int
}

// --- timer-after-view ---------------------------------------------------------------------------

func (c *Child) tick(cx *gk.Cx) {
	if c.dead {
		c.wrong++
		fmt.Println("review3 WRONG: a timer of a released view ran")
		return
	}
	c.ticks++
}

func (c *Child) pinged(cx *gk.Cx, _ gk.Action) {
	if c.dead {
		c.wrong++
		fmt.Println("review3 WRONG: a listener of a released view ran")
		return
	}
	c.pings++
}

func (c *Child) init(cx *gk.Cx) {
	c.timer = gk.Every(cx, 20*ms, c.tick)
	c.ping = gk.NewAction(cx, "review::Ping")
	c.listener = gk.OnAppAction(cx, c.ping, c.pinged)
}

func (c *Child) view(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(kit.Label(fmt.Sprintf("child ticks %d", c.ticks)))
}

func (a *App) openChild(cx *gk.Cx) {
	a.childV = gk.NewView(a.child.init, a.child.view)
	a.win = gk.OpenWindow(cx, "child", 300, 200, a.childV)
	fmt.Printf("review3 child opened windows=%d\n", len(gk.Windows(cx)))
}

func (a *App) pingChild(cx *gk.Cx) {
	gk.DispatchAction(cx, a.child.ping)
}

func (a *App) dropChild(cx *gk.Cx) {
	fmt.Printf("review3 child before release ticks=%d pings=%d\n", a.child.ticks, a.child.pings)
	gk.CloseWindow(cx, a.win)
	gk.Release(cx, a.childV)
	a.child.dead = true
	gk.DispatchAction(cx, a.child.ping) // the released view's listener must not hear it
}

func (a *App) finalFired(cx *gk.Cx) {
	a.final++
}

func (a *App) reportChild(cx *gk.Cx) {
	// the app still holds the listener's and the timer's handles: releasing them now does nothing
	// wrong, and a handler kept afterwards (in a slot the view's handlers had) still fires
	gk.Release(cx, a.child.listener)
	gk.Release(cx, a.child.timer)
	gk.After(cx, 10*ms, a.finalFired)
	gk.DispatchAction(cx, a.child.ping)
	fmt.Printf("review3 timer-after-view ticks=%d pings=%d wrong=%d windows=%d\n", a.child.ticks, a.child.pings,
		a.child.wrong, len(gk.Windows(cx)))
}

func (a *App) timerAfterViewInit(cx *gk.Cx) {
	gk.After(cx, 10*ms, a.openChild)
	gk.After(cx, 60*ms, a.pingChild)
	gk.After(cx, 120*ms, a.dropChild)
	gk.After(cx, 220*ms, a.reportChild)
}

// --- close-own-window ---------------------------------------------------------------------------

func (a *App) closeSelf(cx *gk.Cx, _ gk.Action) {
	a.closes++
	gk.CloseWindow(cx, gk.Window{}) // the turn's window: the one this handler's key press went to
	gk.DispatchAction(cx, a.ping)
	fmt.Printf("review3 close-self n=%d\n", a.closes)
}

func (a *App) pinged(cx *gk.Cx, _ gk.Action) {
	a.pings++
}

func (a *App) secondClosed(cx *gk.Cx, w gk.Window) {
	a.closed++
	fmt.Printf("review3 second closed n=%d same=%v\n", a.closed, w == a.second)
}

func (a *App) secondInit(cx *gk.Cx) {
	a.focus2 = gk.NewFocusHandle(cx)
	gk.Focus(cx, a.focus2)
}

func (a *App) secondView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).KeyContext("Second").TrackFocus(a.focus2).
		OnAction(gk.Act(a.close, a.closeSelf)).Child(kit.Label("second"))
}

func (a *App) openSecond(cx *gk.Cx) {
	a.secondV = gk.NewView(a.secondInit, a.secondView)
	a.second = gk.OpenWindow(cx, "second", 300, 200, a.secondV)
	gk.OnWindowClosed(cx, a.second, a.secondClosed)
	fmt.Printf("review3 second opened windows=%d\n", len(gk.Windows(cx)))
}

func (a *App) reportClose(cx *gk.Cx) {
	fmt.Printf("review3 close-own-window closes=%d closed=%d pings=%d windows=%d\n", a.closes, a.closed, a.pings,
		len(gk.Windows(cx)))
}

func (a *App) closeOwnInit(cx *gk.Cx) {
	a.close = gk.NewAction(cx, "review::CloseWindow")
	a.ping = gk.NewAction(cx, "review::Ping")
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-w", Action: a.close, When: "Second"})
	gk.OnAppAction(cx, a.ping, a.pinged)
	gk.After(cx, 10*ms, a.openSecond)
	gk.After(cx, 200*ms, a.reportClose)
}

// --- dock-panel-freed ---------------------------------------------------------------------------

func (a *App) paView(cx *gk.Cx) gk.Element {
	a.aDrawn++
	if a.freed {
		a.aAfter++
	}
	return gk.Div(s.SizeFull).ID("pa").OnClick(a.paClicked).Child(kit.Label("panel a"))
}

func (a *App) paClicked(cx *gk.Cx, _ gk.ClickEvent) {
	if a.freed {
		fmt.Println("review3 WRONG: a click reached a released panel view")
	}
}

func (a *App) pbView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).ID("pb").Child(kit.Label("panel b"))
}

func (a *App) makePanels(cx *gk.Cx) {
	a.paV = gk.NewView(nil, a.paView)
	a.pbV = gk.NewView(nil, a.pbView)
	a.pa = gkdock.NewPanel(cx, a.paV, gkdock.PanelData{Name: "a", Title: "A"})
	a.pb = gkdock.NewPanel(cx, a.pbV, gkdock.PanelData{Name: "b", Title: "B"})
}

func (a *App) freePanelView(cx *gk.Cx) {
	gk.Release(cx, a.paV) // still docked, and the active tab
	a.freed = true
	gk.Notify(cx, a.root)
	fmt.Printf("review3 panel view released drawn=%d\n", a.aDrawn)
}

func (a *App) relayout(cx *gk.Cx) {
	gk.Release(cx, a.pa) // the panel itself too, while the dock still holds it
	a.dock.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(a.pb))
	gk.Notify(cx, a.root)
}

func (a *App) reportDock(cx *gk.Cx) {
	fmt.Printf("review3 dock-panel-freed described_after=%d frames=%d\n", a.aAfter, a.frame)
}

func (a *App) dockFreedInit(cx *gk.Cx) {
	a.dock = gkdock.NewAreaSkinned(cx, "review3")
	a.makePanels(cx)
	a.dock.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(a.pa).PanelView(a.pb))
	gk.After(cx, 30*ms, a.freePanelView)
	gk.After(cx, 90*ms, a.relayout)
	gk.After(cx, 150*ms, a.reportDock)
}

// --- reentrant-action ---------------------------------------------------------------------------

func (a *App) ping1(cx *gk.Cx, _ gk.Action) {
	a.depth++
	if a.depth > 1 {
		a.nested++
	}
	a.n1++
	if a.n1 == 2 {
		gk.Release(cx, a.l1)                       // itself, while it runs
		a.l2 = gk.OnAppAction(cx, a.ping, a.ping2) // another one, while the dispatch runs
	}
	if a.n1+a.n2 < a.limit {
		gk.DispatchAction(cx, a.ping)
	}
	a.depth--
}

func (a *App) ping2(cx *gk.Cx, _ gk.Action) {
	a.depth++
	if a.depth > 1 {
		a.nested++
	}
	a.n2++
	if a.n1+a.n2 < a.limit {
		gk.DispatchAction(cx, a.ping)
	}
	a.depth--
}

func (a *App) reportWindowPhase(cx *gk.Cx) {
	fmt.Printf("review3 reentrant window n1=%d n2=%d nested=%d\n", a.n1, a.n2, a.nested)
}

func (a *App) noWindowPing(cx *gk.Cx) {
	// kept in the start turn, before any window: fires without one, so the dispatch has none
	a.limit = 12
	gk.DispatchAction(cx, a.ping)
}

func (a *App) reportReentrant(cx *gk.Cx) {
	fmt.Printf("review3 reentrant-action n1=%d n2=%d nested=%d\n", a.n1, a.n2, a.nested)
}

func (a *App) reentrantInit(cx *gk.Cx) {
	a.ping = gk.NewAction(cx, "review::Ping")
	a.limit = 6
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-p", Action: a.ping})
	a.l1 = gk.OnAppAction(cx, a.ping, a.ping1)
	gk.After(cx, 100*ms, a.reportWindowPhase)
	gk.After(cx, 400*ms, a.reportReentrant)
}

// --- the app ------------------------------------------------------------------------------------

func (a *App) init(cx *gk.Cx) {
	switch a.mode {
	case "timer-after-view":
		a.timerAfterViewInit(cx)
	case "close-own-window":
		a.closeOwnInit(cx)
	case "dock-panel-freed":
		a.dockFreedInit(cx)
	case "reentrant-action":
		a.reentrantInit(cx)
	case "handle-after-owner":
		a.handleOwnerInit(cx)
	}
}

func (a *App) onFrame(cx *gk.Cx) gk.Element {
	a.frame++
	root := gk.VFlex(s.SizeFull).Child(kit.Label(fmt.Sprintf("review3 %s", a.mode)))
	if a.dock != (gkdock.Area{}) && !a.dockGone {
		root = root.Child(gk.Div(s.Flex1, s.WFull).Child(gkdock.View(a.dock)))
	}
	return root
}

func (a *App) start(cx *gk.Cx) {
	if a.mode == "reentrant-action" {
		gk.After(cx, 200*ms, a.noWindowPing) // kept in the start turn: no window, the app's state
	}
	a.root = gk.NewView(a.init, a.onFrame)
	gk.OpenWindow(cx, "review3", 640, 480, a.root)
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	switch a.mode {
	case "timer-after-view", "close-own-window", "dock-panel-freed", "reentrant-action", "handle-after-owner":
	default:
		fmt.Printf("review3: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	script := "size 640 480\nframe\n"
	adv := "advance 25\nframe\n"
	switch a.mode {
	case "close-own-window":
		script += adv + adv + "windows\nwindow 1\nframe\nkey ctrl-w\nwindow 0\n" + strings.Repeat(adv, 8) + "windows\nframe\nstat\n"
	case "dock-panel-freed":
		script += adv + adv + "click 320 240\n" + adv + adv + "click 320 240\n" + strings.Repeat(adv, 4) + "frame\nstat\n"
	case "reentrant-action":
		script += "key ctrl-p\nframe\nframe\n" + strings.Repeat(adv, 18) + "stat\n"
	default:
		script += strings.Repeat(adv, 14) + "stat\n"
	}
	gk.RunHeadless(script, a.start)
	fmt.Printf("review3 done mode=%s frames=%d final=%d\n", a.mode, a.frame, a.final)
}
