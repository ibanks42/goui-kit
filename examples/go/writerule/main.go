// Write-rule example: updates an InputState inside gk.OnFrame and echoes its value in a
// child view, verifying that pre-frame writes become visible on the very same frame.
package main

import (
	"fmt"
	"log"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type App struct {
	frame      int
	name       editor.InputState
	root, echo gk.View
}

func (a *App) init(cx *gk.Cx) {
	a.name = editor.NewInputState(cx).Placeholder("Name").Build()
	gk.OnFrame(cx, a.tick)
}

// tick mutates InputState during K_FRAME; because the write lands before K_VIEWS runs,
// echoView observes the updated text on the very same frame (no one-frame lag).
func (a *App) tick(cx *gk.Cx) {
	a.frame++
	a.name.SetValue(cx, fmt.Sprintf("frame %d", a.frame))
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Bg(theme.Background).
		TextColor(theme.Foreground).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))).
		ChildView(a.echo)
}

// echoView reads a.name.Value(cx), subscribing this child view to InputState changes.
func (a *App) echoView(cx *gk.Cx) gk.Element {
	return gk.HFlex().Child(kit.Label("echo " + a.name.Value(cx)))
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		a.echo = gk.NewView(nil, a.echoView)
		a.root = gk.NewView(a.init, a.rootView)
		gk.OpenWindow(cx, "Write rule", 480, 200, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}
