// Runtime misuse cases for timers, windows, key bindings, temporary dock handles,
// and released dock panels (GK_NEG).
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	gkdock "github.com/ibanks42/goui-kit/gk/dock"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
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
			gk.SetWindowTitle(cx, a.second, "gone")
		}
	case "rv-temp-later":
		if a.clicks == 1 {
			a.tree, _ = a.dock.Layout(cx, gkdock.PlacementCenter).Get()
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
