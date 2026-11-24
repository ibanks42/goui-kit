package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/dock"
)

var renderer dock.Renderer

// askedRender retains the dock context handle on the first Dock hook callback
// so `handle-after-owner` can exercise it after the owning DockArea is released.
func (a *App) askedRender(cx *gk.Cx, q dock.Render) {
	switch q.Hook {
	case dock.HookDock:
		if !a.hasCtx {
			gk.Retain(cx, q.Dock)
			a.dockCtx = q.Dock
			a.hasCtx = true
		}
	}
	dock.RenderAnswer(cx, q.Renderer, q.Hook, q.Key, gk.View{})
}

func (a *App) keepAndDrop(cx *gk.Cx) {
	tree, ok := a.dock.Layout(cx, dock.PlacementLeft).Get()
	if ok {
		gk.Retain(cx, tree)
		a.tree = tree
	}
	n := -1
	if ok {
		n = len(a.tree.Panels(cx))
	}
	fmt.Printf("review3 owner kept tree=%v ctx=%v panels=%d\n", ok, a.hasCtx, n)
	a.dockGone = true
	gk.Notify(cx, a.root)
	gk.Release(cx, a.dock)
}

func (a *App) useAfterOwner(cx *gk.Cx) {
	n := -1
	if a.tree != (dock.PaneTree{}) {
		n = len(a.tree.Panels(cx))
	}
	open := false
	if a.hasCtx {
		open = a.dockCtx.IsOpen(cx)
		a.dockCtx.Toggle(cx)
	}
	fmt.Printf("review3 handle-after-owner panels=%d ctx_open=%v\n", n, open)
}

// handleOwnerInit proves that a PaneTree and Dock Context promoted with
// gk.Retain remain valid and readable (panels=1, IsOpen/Toggle succeed) even
// after the owning DockArea handle is removed from the tree and released.
func (a *App) handleOwnerInit(cx *gk.Cx) {
	a.makePanels(cx)
	renderer = dock.NewRenderer(cx, dock.Hooks(dock.HookDock), a.askedRender)
	a.dock = dock.NewArea(cx, "review3-owner", gk.Option[int]{}).WithRenderer(renderer).Build()
	a.dock.SetCenter(cx, dock.NewLayoutTabs().Panel(a.pb))
	a.dock.SetDock(cx, dock.PlacementLeft, dock.NewLayoutTabs().Panel(a.pa))
	gk.After(cx, 40*ms, a.keepAndDrop)
	gk.After(cx, 100*ms, a.useAfterOwner)
}
