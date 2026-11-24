// handle-after-owner: a dock's layout and a dock context (retained from a render request of the
// dock renderer) used after the dock was released.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/dock"
)

// --- handle-after-owner -------------------------------------------------------------------------

var renderer dock.Renderer

func (a *App) askedRender(cx *gk.Cx, q dock.Render) {
	switch q.Hook {
	case dock.HookDock:
		if !a.hasCtx {
			gk.Retain(cx, q.Dock) // a temporary handed with the request: kept past it
			a.dockCtx = q.Dock
			a.hasCtx = true
		}
	}
	dock.RenderAnswer(cx, q.Renderer, q.Hook, q.Key, gk.View{}) // upstream's own drawing
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
	gk.Notify(cx, a.root) // the view stops showing the dock before it is released
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
		a.dockCtx.Toggle(cx) // the dock it toggles is gone
	}
	fmt.Printf("review3 handle-after-owner panels=%d ctx_open=%v\n", n, open)
}

func (a *App) handleOwnerInit(cx *gk.Cx) {
	a.makePanels(cx)
	renderer = dock.NewRenderer(cx, dock.Hooks(dock.HookDock), a.askedRender)
	a.dock = dock.NewArea(cx, "review3-owner", gk.Option[int]{}).WithRenderer(renderer).Build()
	a.dock.SetCenter(cx, dock.NewLayoutTabs().Panel(a.pb))
	a.dock.SetDock(cx, dock.PlacementLeft, dock.NewLayoutTabs().Panel(a.pa))
	gk.After(cx, 40*ms, a.keepAndDrop)
	gk.After(cx, 100*ms, a.useAfterOwner)
}
