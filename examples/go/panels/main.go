// panels: the panels scene in Go, the same dashboard as ref/src/bin/panels.rs and
// examples/odin/panels (a port of gpui-fast's views_frames benchmark). 60 panels of 64 labels,
// each panel a view of its own; `changing` of them change before every frame.
//
//	panels-go [panels] [labels] [changing]      (default 60 64 2)
package main

import (
	"fmt"
	"os"
	"strconv"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

const labels = 97 // distinct labels a cell can show

type Panel struct {
	app         *App
	index, tick int
	view        gk.View
}

type App struct {
	panels          []*Panel
	labels          [labels]string
	count, changing int
	tick            int
	root            gk.View
}

func (p *Panel) render(cx *gk.Cx) gk.Element {
	hover := gk.NewStyleRefinementDefault().Bg(gk.HSLA(0.1, 0.6, 0.4, 1))
	cells := gk.Div(s.Flex, s.FlexRow, s.FlexWrap)
	for cell := range p.app.count {
		cells.Child(gk.Div().W(gk.Px(26)).H(gk.Px(14)).Hover(hover).Text(p.app.labels[(cell+p.tick)%labels]))
	}
	return gk.Div(s.Flex, s.FlexCol).W(gk.Px(220)).Style(s.P1, s.Gap1, s.RoundedSm).
		Bg(gk.HSLA(0.6, 0.2, 0.18, 1)).
		Text(fmt.Sprintf("panel %d · %d", p.index, p.tick)).
		Child(cells)
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	d := gk.Div(s.SizeFull, s.Flex, s.FlexRow, s.FlexWrap, s.Gap1).
		Bg(gk.HSLA(0, 0, 0.12, 1)).
		TextColor(gk.HSLA(0, 0, 0.85, 1)).
		Style(s.TextXs)
	for _, p := range a.panels {
		d.ChildView(p.view)
	}
	return d
}

// frame runs before every frame (only because init asked for it): change a few panels and mark
// only them, as a timer or a message from elsewhere would.
func (a *App) frame(cx *gk.Cx) {
	a.tick++
	for n := range a.changing {
		p := a.panels[(a.tick*7+n*13)%len(a.panels)]
		p.tick++
		gk.Notify(cx, p.view)
	}
}

func (a *App) rootInit(cx *gk.Cx) {
	if a.changing > 0 && len(a.panels) > 0 {
		gk.OnFrame(cx, a.frame)
	}
}

func arg(i, def int) int {
	if len(os.Args) > i {
		if v, err := strconv.Atoi(os.Args[i]); err == nil {
			return v
		}
	}
	return def
}

func main() {
	a := &App{count: arg(2, 64), changing: arg(3, 2)}
	for i := range a.labels {
		a.labels[i] = fmt.Sprintf("%02d", i)
	}
	a.panels = make([]*Panel, arg(1, 60))
	gk.Run(func(cx *gk.Cx) {
		for i := range a.panels {
			p := &Panel{app: a, index: i, tick: i}
			p.view = gk.NewView(nil, p.render)
			a.panels[i] = p
		}
		a.root = gk.NewView(a.rootInit, a.rootView)
		gk.OpenWindow(cx, "Panels", 1400, 900, a.root)
	})
}
