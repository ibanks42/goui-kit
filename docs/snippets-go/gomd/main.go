// The code blocks of docs/GO.md, collected into one program that compiles, so the page cannot
// drift from the package (gates/docs-snippets.py checks each block against this file). Section 3
// is an app of its own (run by gates/docs-go.sh); section 7's examples are the bodies of the
// Bands methods below. Not an app to read for itself: the tutorial in docs/tutorial-go is.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/chart"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	count int
	name  editor.InputState // a handle: the kit's InputState lives in the library
	root  gk.View
}

func (a *App) init(cx *gk.Cx) { a.name = editor.NewInputState(cx).Placeholder("Your name").Build() }

func (a *App) add(cx *gk.Cx, _ gk.ClickEvent) { a.count++ } // marks its view for redraw on return

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(kit.Button("add").Primary().Label("Add").OnClick(a.add)).
			Child(kit.Label(fmt.Sprintf("clicked %d times", a.count)))).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))).
		Child(kit.Label("hello " + a.name.Value(cx))) // a read in a view: redrawn when it changes
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "Hello", 480, 320, a.root)
	})
}

// Bands holds what section 7's examples use: a draggable band, a plot with a gradient line, and
// asking before a window closes.
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
	OnDrag(gk.DragValue{ID: uint64(i), Text: name}, a.bandPreview).   // a.bandPreview: a gk.View made in init
	CanDrop(gk.DropAccept{IDs: a.bandIDs()}).
	DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent)).
	OnDrop(func(cx *gk.Cx, d gk.DragValue) { a.moveBand(int(d.ID), i) })

chart.Plot(chart.NewGrid().X(xs).Y(ys).Stroke(theme.Border),
	chart.NewPlotArea(area, gk.HSLA(0.6, 0.8, 0.7, 0.12)),
	chart.NewPlotLine(curve, 2.5, gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1))))

gk.OnWindowShouldClose(cx, a.win, func(cx *gk.Cx) bool {
	if !a.dirty { return true }
	overlay.OpenAlertDialog(cx, a.askClose) // its OK calls gk.CloseWindow(cx, a.win), which does not ask again
	return false
})
}
