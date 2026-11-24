// Runtime misuse cases for view-built overlays (gk.NewBuilt) and dock operations (GK_NEG):
// zero or released Built views, view-time creation, unknown dock node IDs, and panel registration.
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
		a.dock.SetZoomedIn(cx, gkdock.NodeID(424242))
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
		gk.NewBuilt(cx, a.good)
	}
	if a.mode == "dock-register" {
		gkdock.RegisterPanel(cx, "x", func(cx *gk.Cx, b gkdock.PanelBuild) {})
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
