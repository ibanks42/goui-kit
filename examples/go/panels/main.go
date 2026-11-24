// Panels scene: renders N child views of M cells each and marks K views dirty per frame
// via gk.OnFrame. Used by pixel, memory, and frame-cost gates against ref-panels.
// Usage: panels [panelCount] [cellsPerPanel] [changingPerFrame]
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/s"
)

const labels = 97

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

// render describes a single panel view. Because each panel is its own gk.View,
// dirtying 2 panels per frame leaves the other 58 panels retained by gpui-fast.
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

// frame runs in the pre-frame hook (K_FRAME) and dirties a.changing panels per frame;
// when there are no deferred state writes, Go inline-describes those panels in the same reply.
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
	if err := gk.Run(func(cx *gk.Cx) {
		for i := range a.panels {
			p := &Panel{app: a, index: i, tick: i}
			p.view = gk.NewView(nil, p.render)
			a.panels[i] = p
		}
		a.root = gk.NewView(a.rootInit, a.rootView)
		gk.OpenWindow(cx, "Panels", 1400, 900, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}
