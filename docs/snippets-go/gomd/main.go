// Compilable snippet host for docs/GO.md.
// gates/docs-go.sh builds and runs this program to keep guide excerpts verified.
package main

import (
	"fmt"
	"log"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/chart"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type App struct {
	count int
	name  editor.InputState
	root  gk.View
}

func (a *App) init(cx *gk.Cx) { a.name = editor.NewInputState(cx).Placeholder("Your name").Build() }

func (a *App) add(cx *gk.Cx) { a.count++ }

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(kit.Button("add").Primary().Label("Add").OnClickDo(a.add)).
			Child(kit.Label(fmt.Sprintf("clicked %d times", a.count)))).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))).
		Child(kit.Label("hello " + a.name.Value(cx)))
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "Hello", 480, 320, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}

type Bands struct {
	bandPreview gk.View
	names       []string
	win         gk.Window
	dirty       bool
	askClose    gk.Built[overlay.AlertDialogEl]
}

func (a *Bands) bandIDs() []uint64 {
	ids := make([]uint64, len(a.names))
	for i := range ids {
		ids[i] = uint64(i)
	}
	return ids
}

func (a *Bands) moveBand(from, to int) { a.names[from], a.names[to] = a.names[to], a.names[from] }

func (a *Bands) examples(cx *gk.Cx, i int, name string, xs, ys []float32, area, curve [][2]float32, cyan, violet gk.Color) {
	gk.Div().ID(gk.Key("band", i)).
		OnDrag(gk.DragValue{ID: uint64(i), Text: name}, a.bandPreview).
		CanDrop(gk.DropAccept{IDs: a.bandIDs()}).
		DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent)).
		OnDrop(func(cx *gk.Cx, d gk.DragValue) { a.moveBand(int(d.ID), i) })

	chart.Plot(chart.NewGrid().X(xs).Y(ys).Stroke(theme.Border),
		chart.NewPlotArea(area, gk.HSLA(0.6, 0.8, 0.7, 0.12)),
		chart.NewPlotLine(curve, 2.5, gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1))))

	gk.OnWindowShouldClose(cx, a.win, func(cx *gk.Cx) bool {
		if !a.dirty {
			return true
		}
		overlay.OpenAlertDialog(cx, a.askClose)
		return false
	})
}
