// misuse_review: mistakes against the app protocols (actions, timers, windows: PROTOCOLS.md
// section 11), the dock's panels (11.7) and library objects (13.2). Each must stop with a
// message and the app's line (exit 2) or behave correctly (exit 0); exit 0 with wrong content
// fails. Run offscreen with the diag library by
// gates/go-misuse.sh, one mode per case:
//
//	GK_NEG=rv-ok             timers, a second window opened and closed, an action, a panel: correct (exit 0)
//	GK_NEG=rv-timer-view     gk.After in a view function: must stop
//	GK_NEG=rv-timer-zero     a repeating timer of 0: must stop
//	GK_NEG=rv-window-stale   the title of a window that was closed: must stop
//	GK_NEG=rv-keys-view      key bindings in a view function: must stop
//	GK_NEG=rv-temp-later     a temporary library object (a dock's layout) used a frame later: must stop
//	GK_NEG=rv-panel-released a dock panel released and then changed: must stop
//
// A forged action (an action id the library never made) does not compile in Go
// (examples/go/misuse_compile/action_forged); rv-timer-state (a timer handler of another state
// type) cannot be written: a Go handler is a closure and carries no state type.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"gpuikit.dev/gk"
	gkdock "gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
)

type App struct {
	mode    string
	root    gk.View
	clicks  int
	fired   int
	drawn   int
	second  gk.Window
	secondV gk.View
	closed  int
	act     gk.Action
	acted   int
	dock    gkdock.Area
	hasDock bool
	panel   gkdock.Panel
	panelV  gk.View
	tree    gkdock.PaneTree
}

func (a *App) fire(cx *gk.Cx)                      { a.fired++ }
func (a *App) actOn(cx *gk.Cx, _ gk.Action)        { a.acted++ }
func (a *App) closeSecond(cx *gk.Cx)               { gk.CloseWindow(cx, a.second) }
func (a *App) secondClosed(cx *gk.Cx, _ gk.Window) { a.closed++ }

func label(text string) func(cx *gk.Cx) gk.Element {
	return func(cx *gk.Cx) gk.Element { return gk.Div(s.SizeFull).Child(kit.Label(text)) }
}

func (a *App) clicked(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	switch a.mode {
	case "rv-ok":
		if a.clicks == 1 {
			gk.After(cx, 10*time.Millisecond, a.fire)
			a.secondV = gk.NewView(nil, label("second"))
			a.second = gk.OpenWindow(cx, "second", 200, 100, a.secondV)
			gk.OnWindowClosed(cx, a.second, a.secondClosed)
			gk.After(cx, 20*time.Millisecond, a.closeSecond)
			gk.DispatchAction(cx, a.act)
			gkdock.SetPanel(cx, a.panel, gkdock.PanelData{Name: "p", Title: "Renamed"})
		}
	case "rv-timer-zero":
		gk.Every(cx, 0, a.fire)
	case "rv-window-stale":
		if a.clicks == 1 {
			a.secondV = gk.NewView(nil, label("second"))
			a.second = gk.OpenWindow(cx, "second", 200, 100, a.secondV)
			gk.After(cx, 20*time.Millisecond, a.closeSecond)
		} else {
			gk.SetWindowTitle(cx, a.second, "gone") // the window closed a frame ago
		}
	case "rv-temp-later":
		if a.clicks == 1 {
			a.tree, _ = a.dock.Layout(cx, gkdock.PlacementCenter).Get() // temporary: not retained
		} else {
			_ = a.tree.RootKind(cx)
		}
	case "rv-panel-released":
		gk.Release(cx, a.panel)
		gkdock.SetPanel(cx, a.panel, gkdock.PanelData{Name: "p", Title: "after release"})
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	a.drawn++
	switch a.mode {
	case "rv-timer-view":
		gk.After(cx, 10*time.Millisecond, a.fire)
	case "rv-keys-view":
		gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-k", Action: a.act})
	}
	body := gk.VFlex(s.SizeFull).ID("all").OnClick(a.clicked).
		OnAction(gk.Act(a.act, a.actOn)).
		Child(kit.Label(fmt.Sprintf("clicks %d", a.clicks)))
	if a.hasDock {
		body = body.Child(gk.Div(s.Flex1, s.WFull).Child(gkdock.View(a.dock)))
	}
	return body
}

func (a *App) init(cx *gk.Cx) {
	a.act = gk.NewAction(cx, "review::Act")
	gk.OnAppAction(cx, a.act, a.actOn)
	switch a.mode {
	case "rv-ok", "rv-temp-later", "rv-panel-released":
		a.dock = gkdock.NewAreaSkinned(cx, "rv")
		a.hasDock = true
		a.panelV = gk.NewView(nil, label("panel"))
		a.panel = gkdock.NewPanel(cx, a.panelV, gkdock.PanelData{Name: "p", Title: "P"})
		a.dock.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(a.panel))
	}
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	if err := gk.RunHeadless("size 400 300\nframe\nclick 200 20\nframe\nadvance 30\nframe\nadvance 30\nframe\nclick 200 20\nframe\nadvance 30\nframe\nstat\n", func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "misuse_review", 400, 300, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gk-neg done mode=%s drawn=%d clicks=%d fired=%d closed=%d acted=%d\n", a.mode, a.drawn, a.clicks, a.fired, a.closed, a.acted)
}
