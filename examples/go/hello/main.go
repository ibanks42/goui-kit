// hello: the README's first app in Go (examples/odin/hello is the same app in Odin).
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	count int
	name  gk.InputState // a handle to the kit's InputState, which lives in the library
	root  gk.View
}

func (a *App) init(cx *gk.Cx) { a.name = gk.NewInputState(cx).Placeholder("Your name").Build() }

func (a *App) add(cx *gk.Cx, _ gk.ClickEvent) { a.count++ } // marks its view for redraw

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(gk.Button("add").Primary().Label("Add").OnClick(a.add)).
			Child(gk.Label(fmt.Sprintf("clicked %d times", a.count)))).
		Child(gk.Div().W(gk.Px(240)).Child(gk.Input(a.name))).
		Child(gk.Label("hello " + a.name.Value(cx))) // a read: redrawn when it changes
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "Hello", 480, 320, a.root)
	})
}
