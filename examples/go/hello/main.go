// Minimal GPUI-kit example: a button counter and a text input echoed live.
// README.md embeds this file verbatim.
package main

import (
	"fmt"
	"log"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	count int
	name  editor.InputState
	root  gk.View
}

// init runs once when the window mounts the root view, which is when window-backed
// kit state (such as InputState) can be created.
func (a *App) init(cx *gk.Cx) { a.name = editor.NewInputState(cx).Placeholder("Your name").Build() }

// add is an event handler. After it returns, the view is marked dirty and re-described.
func (a *App) add(cx *gk.Cx) { a.count++ }

// view describes the current UI tree. Reading a.name.Value records a dependency so
// typing into the input automatically triggers a redraw of this view.
func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Bg(theme.Background).TextColor(theme.Foreground).
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
