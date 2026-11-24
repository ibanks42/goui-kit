// The lifetimes of handles (docs/GO.md section 4): what a view makes is released with it, a
// closed dock panel releases its view and what that made, and a handle the app can no longer
// reach is released after the collector finds it.
package main

import (
	"fmt"
	"runtime"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/s"
)

// lifeChild is a view that makes kit state, a focus handle, a row source and a view of its own
// in its first click's handler (a view's turn: its handlers, and init for a window's root): all
// of it is the view's. It fills its parent, so the script's click reaches it first.
type lifeChild struct {
	name  string
	idle  bool // makes nothing
	input editor.InputState
	focus gk.FocusHandle
	src   gk.RowSource
	inner gk.View
	made  bool
}

func (c *lifeChild) click(cx *gk.Cx) {
	if c.made || c.idle {
		return
	}
	c.input = editor.NewInputState(cx).Placeholder("child").Build()
	c.focus = gk.NewFocusHandle(cx)
	c.src = gk.NewRowSource(cx, 3, func(cx *gk.Cx, at gk.RowAt) gk.Element { return gk.Text("row") })
	c.inner = gk.NewView(nil, func(cx *gk.Cx) gk.Element { return gk.Text("inner") })
	c.made = true
	fmt.Printf("handles life %s made its state\n", c.name)
}

func (c *lifeChild) view(cx *gk.Cx) gk.Element {
	d := gk.Div(s.SizeFull).ID("child").OnClickDo(c.click)
	if c.made {
		d.Child(editor.Input(c.input)).ChildView(c.inner)
	}
	return d
}

func liveCounts(cx *gk.Cx, what string) {
	st, vw, fh := gk.TestLiveCounts(cx)
	_, live := gk.TestObjCounts(cx)
	fmt.Printf("handles life %s states=%d views=%d focus=%d objects=%d\n", what, st, vw, fh, live)
}

var lifeKid *lifeChild
var lifeKidView gk.View

// life-view: a view and what it made, released together; life-view-again: then the app releases
// the view's state itself, which its view released already (stops: was released).
func lifeView(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		liveCounts(cx, "before")
		lifeKid = &lifeChild{name: "child"}
		lifeKidView = gk.NewView(nil, lifeKid.view)
	case 2: // the child's handler ran first and made its state
		liveCounts(cx, "made")
	case 3:
		gk.Release(cx, lifeKidView)
		lifeKidView = gk.View{}
	case 4:
		liveCounts(cx, "released")
		if a.mode == "life-view-again" {
			fmt.Println("handles life releasing the child's state again")
			gk.Release(cx, lifeKid.input) // stops: released with its view
		}
	}
}

// life-gc: kit state and library objects the app drops are released once the collector finds
// them; one the app keeps, and one handed to the library (a panel given to a dock), stay.
func lifeGC(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		liveCounts(cx, "before")
		for range 3 {
			_ = editor.NewInputState(cx).Placeholder("dropped").Build()
			_ = gk.NewFocusHandle(cx)
		}
		a.input = editor.NewInputState(cx).DefaultValue("kept").Build()
		liveCounts(cx, "made")
	case 2:
		for range 3 {
			runtime.GC()
			time.Sleep(20 * time.Millisecond) // the cleanups run on their own goroutine
		}
	case 3:
		released, _ := gk.TestLife()
		liveCounts(cx, "collected")
		fmt.Printf("handles life gc released=%d kept=%q\n", released, a.input.Value(cx))
	}
}

// life-panel: a dock panel the dock removed (as when the user closes its tab) is released with
// its view and what the view made; the other panel stays.
var lifePanels [2]dock.Panel
var lifeDock dock.Area

func lifePanel(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		liveCounts(cx, "before")
		lifeDock = dock.NewAreaSkinned(cx, "life")
		tabs := dock.NewLayoutTabs()
		for i := range 2 {
			c := &lifeChild{name: fmt.Sprint("panel ", i), idle: i == 1}
			v := gk.NewView(nil, c.view)
			lifePanels[i] = dock.NewPanel(cx, v, dock.PanelData{Name: fmt.Sprint("p", i), Title: fmt.Sprint("panel ", i)})
			tabs = tabs.PanelView(lifePanels[i])
		}
		lifeDock.SetCenter(cx, tabs)
	case 2: // the panel shown made its state
		liveCounts(cx, "docked")
		lifeDock.RemovePanel(cx, lifePanels[0])
	case 3:
		liveCounts(cx, "closed")
		fmt.Printf("handles life dock empty=%v\n", lifeDock.IsEmpty(cx, dock.PlacementCenter))
	}
}

func init() {
	modes["life-view"] = lifeView
	modes["life-view-again"] = lifeView
	modes["life-gc"] = lifeGC
	modes["life-panel"] = lifePanel
	views["life-view"] = func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
		if lifeKidView == (gk.View{}) {
			return body
		}
		return body.Child(gk.Div(s.SizeFull).ChildView(lifeKidView))
	}
	views["life-view-again"] = views["life-view"]
	views["life-panel"] = func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
		if lifeDock == (dock.Area{}) {
			return body
		}
		return body.Child(gk.Div(s.SizeFull).Child(dock.View(lifeDock)))
	}
}
