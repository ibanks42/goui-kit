// misuse_values: mistakes against the values family's protocols (docs/PROTOCOLS.md sections
// 13.8 to 13.12, 11.17 and 11.25) that must stop with a message and the app's line, never decode
// a value inside itself until the library's stack runs out. Run
// offscreen with the diag library by gates/go-misuse.sh, one
// mode per case:
//
//	GK_NEG=val-ok           a scale made in a handler and read in the view: correct (exit 0)
//	GK_NEG=val-self         a value made in a handler given to its own builder: must stop
//	GK_NEG=val-self-view    the same in a view function: must stop
//	GK_NEG=val-write-view   a theme field written in a view function: must stop
//	GK_NEG=val-scroll-freed a released ScrollHandle tracked by a view: must stop
//	GK_NEG=c6-tooltip-kept  a tooltip value made in a handler and given to a view later: must stop
//
// val-action-view, val-bad-time, c6-rules-source and c6-rules-limits are in values13.go.
package main

import (
	"fmt"
	"os"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

type App struct {
	mode    string
	root    gk.View
	band    gk.ScaleBand
	hasBand bool
	scroll  gk.ScrollHandle
	clicks  int
	drawn   int
	tip     gk.KitTooltip // c6-tooltip-kept: made in a handler, kept past its turn
}

// The modes of values13.go add their steps here.
var (
	pendingClick = map[string]func(a *App, cx *gk.Cx){}
	pendingView  = map[string]func(a *App, cx *gk.Cx){}
)

func (a *App) clicked(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	switch a.mode {
	case "val-ok":
		if !a.hasBand {
			a.band = gk.NewScaleBand(cx, []string{"x", "y"}, [2]float32{0, 100}).Build()
			a.hasBand = true
		}
	case "val-self":
		t := gk.NewTreeItem("a", "A")
		t = t.Child(t)
	case "val-scroll-freed":
		gk.Release(cx, a.scroll)
	case "c6-tooltip-kept":
		a.tip = gk.NewKitTooltip("made in a handler")
	default:
		if f := pendingClick[a.mode]; f != nil {
			f(a, cx)
		}
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	a.drawn++
	body := gk.Div(s.SizeFull).ID("all").OnClick(a.clicked)
	switch a.mode {
	case "val-ok":
		if a.hasBand {
			x, _ := a.band.Tick(cx, "y").Get()
			body = body.Child(gk.Text(fmt.Sprintf("y at %.0f", x)))
		}
	case "val-self-view":
		t := gk.NewTreeItem("a", "A")
		t = t.Child(t)
	case "val-write-view":
		gk.ThemeGlobal(cx).SetFontSize(cx, 20)
	case "val-scroll-freed":
		body = body.Child(gk.Div(s.H32).ID("list").OverflowYScroll().TrackScroll(a.scroll))
	case "c6-tooltip-kept":
		if a.clicks > 0 {
			body = body.Child(gk.Div().ID("tip").W(gk.Px(80)).H(gk.Px(30)).Tooltip(a.tip))
		}
	default:
		if f := pendingView[a.mode]; f != nil {
			f(a, cx)
		}
	}
	return body
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	gk.RunHeadless("size 400 300\nframe\nclick 200 150\nframe\nframe\nclick 200 150\nframe\nframe\nstat\n", func(cx *gk.Cx) {
		if a.mode == "val-scroll-freed" {
			a.scroll = gk.NewScrollHandle(cx)
		}
		a.root = gk.NewView(nil, a.view)
		gk.OpenWindow(cx, "misuse_values", 400, 300, a.root)
	})
	fmt.Printf("gk-neg done mode=%s drawn=%d clicks=%d\n", a.mode, a.drawn, a.clicks)
}
