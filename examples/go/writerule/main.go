// writerule: the write-rule case in Go, the twin of ref/src/bin/writerule.rs (DESIGN.md
// section 7, "When a write lands"; docs/PROTOCOLS.md section 3). A per-frame handler writes
// the frame's number into an input's state; a second view echoes the input's value. The rule
// says the write is in the pixels of the frame the handler ran in, so every image must show
// the same number in the input and in the echo, as the Rust reference does.
//
// The handler does not notify the echo view: that view is described again only because it
// read the state that was written. An app that notified it by hand would hide a broken rule.
//
//	GK_SCRIPT=file writerule-go   offscreen under a gk-meter script (the diag library)
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	frame      int
	name       gk.InputState
	root, echo gk.View
}

func (a *App) init(cx *gk.Cx) {
	a.name = gk.NewInputState(cx).Placeholder("Name").Build()
	gk.OnFrame(cx, a.tick)
}

func (a *App) tick(cx *gk.Cx) {
	a.frame++
	a.name.SetValue(cx, fmt.Sprintf("frame %d", a.frame))
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Bg(theme.Background).
		TextColor(theme.Foreground).
		Child(gk.Div().W(gk.Px(240)).Child(gk.Input(a.name))).
		ChildView(a.echo)
}

func (a *App) echoView(cx *gk.Cx) gk.Element {
	return gk.HFlex().Child(gk.Label("echo " + a.name.Value(cx)))
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		a.echo = gk.NewView(nil, a.echoView)
		a.root = gk.NewView(a.init, a.rootView)
		gk.OpenWindow(cx, "Write rule", 480, 200, a.root)
	})
}
