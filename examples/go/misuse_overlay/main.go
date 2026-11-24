// misuse_overlay: mistakes against view-built closures (docs/PROTOCOLS.md section 11.6) that
// must stop with a message, never draw a dialog from the wrong view. Run
// offscreen with the diag library by gates/go-misuse.sh, one
// mode per case:
//
//	GK_NEG=built-ok    a dialog opened from a handler and drawn: correct (exit 0)
//	GK_NEG=built-freed the view of an open dialog is released: its next build must stop
//	GK_NEG=built-view  gk.NewBuilt in a view function: must stop at its line
//	GK_NEG=built-none  gk.OpenDialog given a zero gk.Built (no view): must stop at its line
//	GK_NEG=dock-node     a dock call given a node id the dock never handed out: must stop
//	                     (docs/PROTOCOLS.md 11.8)
//	GK_NEG=dock-register gk.RegisterPanel in a view function: must stop at its line
//
// A built root (a function returning a node that is not a Dialog) does not compile in Go:
// examples/go/misuse_compile/built_root.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"gpuikit.dev/gk"
	gkdock "gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
)

type App struct {
	mode  string
	root  gk.View
	dlg   gk.Built[overlay.DialogEl]
	dock  gkdock.Area
	built int
}

func (a *App) good(cx *gk.Cx) overlay.DialogEl {
	a.built++
	return overlay.Dialog().Title(kit.Label("Fine"))
}

func (a *App) open(cx *gk.Cx) {
	switch a.mode {
	case "built-none":
		overlay.OpenDialog(cx, gk.Built[overlay.DialogEl]{})
	case "dock-node":
		a.dock = gkdock.NewAreaSkinned(cx, "neg")
		a.dock.SetZoomedIn(cx, gkdock.NodeID(424242)) // made up: no read handed it out
	default:
		a.dlg = gk.NewBuilt(cx, a.good)
		overlay.OpenDialog(cx, a.dlg)
	}
}

func (a *App) later(cx *gk.Cx) {
	switch a.mode {
	case "built-freed":
		gk.Release(cx, a.dlg.View)
		overlay.OpenAlertDialog(cx, gk.NewBuilt(cx, func(cx *gk.Cx) overlay.AlertDialogEl { return overlay.AlertDialog() }))
	default:
		fmt.Printf("gk-neg done mode=%s built=%d active=%v\n", a.mode, a.built, overlay.HasActiveDialog(cx))
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	if a.mode == "built-view" {
		gk.NewBuilt(cx, a.good) // a view function may not make views
	}
	if a.mode == "dock-register" {
		gkdock.RegisterPanel(cx, "x", func(cx *gk.Cx, b gkdock.PanelBuild) {}) // only in init or a handler
	}
	return gk.Div(s.SizeFull).Child(kit.Label("misuse overlay"))
}

func (a *App) init(cx *gk.Cx) {
	gk.After(cx, 10*time.Millisecond, a.open)
	gk.After(cx, 30*time.Millisecond, a.later)
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	if err := gk.RunHeadless("size 640 480\nframe\nadvance 10\nframe\nframe\nadvance 20\nframe\nframe\nadvance 20\nframe\nstat\n", func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "misuse_overlay", 640, 480, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}
