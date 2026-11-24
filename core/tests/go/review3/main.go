// Command review3 runs regression tests for lifetime and reentrancy edge cases:
// timers/listeners attached to released views, closing a window from its own
// action handler, releasing a docked panel's view, reentrant action dispatch,
// and retaining derived dock handles past their owner's release.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	gkdock "github.com/ibanks42/goui-kit/gk/dock"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
)

const ms = time.Millisecond

// Child tracks callbacks owned by a secondary view in `timer-after-view` to
// verify none fire once `dead` is set on release.
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

	child  Child
	childV gk.View
	win    gk.Window
	final  int

	close   gk.Action
	ping    gk.Action
	focus2  gk.FocusHandle
	second  gk.Window
	secondV gk.View
	closes  int
	closed  int
	pings   int

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

	l1, l2 gk.ActionListener
	n1, n2 int
	depth  int
	nested int
	phase  int
	limit  int
}

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
	gk.DispatchAction(cx, a.child.ping)
}

func (a *App) finalFired(cx *gk.Cx) {
	a.final++
}

func (a *App) reportChild(cx *gk.Cx) {

	gk.Release(cx, a.child.listener)
	gk.Release(cx, a.child.timer)
	gk.After(cx, 10*ms, a.finalFired)
	gk.DispatchAction(cx, a.child.ping)
	fmt.Printf("review3 timer-after-view ticks=%d pings=%d wrong=%d windows=%d\n", a.child.ticks, a.child.pings,
		a.child.wrong, len(gk.Windows(cx)))
}

// timerAfterViewInit proves closing a child window and releasing its view stops
// the child's repeating timer and action listener from firing (wrong=0, pings
// stays at 1) while later timers on the surviving root window still run (final=1).
func (a *App) timerAfterViewInit(cx *gk.Cx) {
	gk.After(cx, 10*ms, a.openChild)
	gk.After(cx, 60*ms, a.pingChild)
	gk.After(cx, 120*ms, a.dropChild)
	gk.After(cx, 220*ms, a.reportChild)
}

func (a *App) closeSelf(cx *gk.Cx, _ gk.Action) {
	a.closes++
	gk.CloseWindow(cx, gk.Window{})
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

// closeOwnInit proves an action handler running inside a secondary window can
// close its own window via gk.CloseWindow(cx, gk.Window{}) and dispatch another
// action without deadlocking, firing OnWindowClosed once and returning windows=1.
func (a *App) closeOwnInit(cx *gk.Cx) {
	a.close = gk.NewAction(cx, "review::CloseWindow")
	a.ping = gk.NewAction(cx, "review::Ping")
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-w", Action: a.close, When: "Second"})
	gk.OnAppAction(cx, a.ping, a.pinged)
	gk.After(cx, 10*ms, a.openSecond)
	gk.After(cx, 200*ms, a.reportClose)
}

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
	gk.Release(cx, a.paV)
	a.freed = true
	gk.Notify(cx, a.root)
	fmt.Printf("review3 panel view released drawn=%d\n", a.aDrawn)
}

func (a *App) relayout(cx *gk.Cx) {
	gk.Release(cx, a.pa)
	a.dock.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(a.pb))
	gk.Notify(cx, a.root)
}

func (a *App) reportDock(cx *gk.Cx) {
	fmt.Printf("review3 dock-panel-freed described_after=%d frames=%d\n", a.aAfter, a.frame)
}

// dockFreedInit proves releasing a docked panel's backing view stops any future
// view describes or click deliveries to that panel (described_after=0) while
// allowing the dock to relayout around the remaining panel cleanly.
func (a *App) dockFreedInit(cx *gk.Cx) {
	a.dock = gkdock.NewAreaSkinned(cx, "review3")
	a.makePanels(cx)
	a.dock.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(a.pa).PanelView(a.pb))
	gk.After(cx, 30*ms, a.freePanelView)
	gk.After(cx, 90*ms, a.relayout)
	gk.After(cx, 150*ms, a.reportDock)
}

func (a *App) ping1(cx *gk.Cx, _ gk.Action) {
	a.depth++
	if a.depth > 1 {
		a.nested++
	}
	a.n1++
	if a.n1 == 2 {
		gk.Release(cx, a.l1)
		a.l2 = gk.OnAppAction(cx, a.ping, a.ping2)
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

	a.limit = 12
	gk.DispatchAction(cx, a.ping)
}

func (a *App) reportReentrant(cx *gk.Cx) {
	fmt.Printf("review3 reentrant-action n1=%d n2=%d nested=%d\n", a.n1, a.n2, a.nested)
}

// reentrantInit proves dispatching an action from inside its own listener queues
// iteratively rather than recursing (nested=0), swaps listeners cleanly mid-chain
// (l1 runs twice then hands off to l2), and works both in a window and timer turn.
func (a *App) reentrantInit(cx *gk.Cx) {
	a.ping = gk.NewAction(cx, "review::Ping")
	a.limit = 6
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-p", Action: a.ping})
	a.l1 = gk.OnAppAction(cx, a.ping, a.ping1)
	gk.After(cx, 100*ms, a.reportWindowPhase)
	gk.After(cx, 400*ms, a.reportReentrant)
}

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
		gk.After(cx, 200*ms, a.noWindowPing)
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
	if err := gk.RunHeadless(script, a.start); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("review3 done mode=%s frames=%d final=%d\n", a.mode, a.frame, a.final)
}
