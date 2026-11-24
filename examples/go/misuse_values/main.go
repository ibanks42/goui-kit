// Runtime misuse cases for builder values, global theme writes in views,
// released scroll handles, and tooltips kept across turns (GK_NEG).
package main

import (
	"fmt"
	"log"
	"os"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/table"
)

type App struct {
	mode    string
	root    gk.View
	band    gk.ScaleBand
	hasBand bool
	scroll  gk.ScrollHandle
	clicks  int
	drawn   int
	tip     overlay.KitTooltip
}

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
		t := table.NewTreeItem("a", "A")
		t = t.Child(t)
	case "val-scroll-freed":
		gk.Release(cx, a.scroll)
	case "c6-tooltip-kept":
		a.tip = overlay.NewKitTooltip("made in a handler")
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
		t := table.NewTreeItem("a", "A")
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
	if err := gk.RunHeadless("size 400 300\nframe\nclick 200 150\nframe\nframe\nclick 200 150\nframe\nframe\nstat\n", func(cx *gk.Cx) {
		if a.mode == "val-scroll-freed" {
			a.scroll = gk.NewScrollHandle(cx)
		}
		a.root = gk.NewView(nil, a.view)
		gk.OpenWindow(cx, "misuse_values", 400, 300, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gk-neg done mode=%s drawn=%d clicks=%d\n", a.mode, a.drawn, a.clicks)
}
