// handle-after-owner: a dock's layout and a dock context (retained from a render request of the
// dock renderer) used after the dock was released.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
)

// --- handle-after-owner -------------------------------------------------------------------------

var renderer gk.DockRenderer

func (a *App) askedRender(cx *gk.Cx, q gk.DockRender) {
	switch q.Hook {
	case gk.HookDock:
		if !a.hasCtx {
			gk.Retain(cx, q.Dock) // a temporary handed with the request: kept past it
			a.dockCtx = q.Dock
			a.hasCtx = true
		}
	}
	gk.DockRenderAnswer(cx, q.Renderer, q.Hook, q.Key, gk.View{}) // upstream's own drawing
}

func (a *App) keepAndDrop(cx *gk.Cx) {
	tree, ok := a.dock.Layout(cx, gk.DockPlacementLeft).Get()
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
	gk.Notify(cx, a.root) // the view stops showing the dock before it is released
	gk.Release(cx, a.dock)
}

func (a *App) useAfterOwner(cx *gk.Cx) {
	n := -1
	if a.tree != (gk.PaneTree{}) {
		n = len(a.tree.Panels(cx))
	}
	open := false
	if a.hasCtx {
		open = a.dockCtx.IsOpen(cx)
		a.dockCtx.Toggle(cx) // the dock it toggles is gone
	}
	fmt.Printf("review3 handle-after-owner panels=%d ctx_open=%v\n", n, open)
}

func (a *App) handleOwnerInit(cx *gk.Cx) {
	a.makePanels(cx)
	renderer = gk.NewDockRenderer(cx, gk.DockHooks(gk.HookDock), a.askedRender)
	a.dock = gk.NewDockArea(cx, "review3-owner", gk.Option[int]{}).WithRenderer(renderer).Build()
	a.dock.SetCenter(cx, gk.NewDockLayoutTabs().Panel(a.pb))
	a.dock.SetDock(cx, gk.DockPlacementLeft, gk.NewDockLayoutTabs().Panel(a.pa))
	gk.After(cx, 40*ms, a.keepAndDrop)
	gk.After(cx, 100*ms, a.useAfterOwner)
}
