package main

import (
	"fmt"
	"runtime"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/dock"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/s"
)

// lifeChild owns nested state, focus, row-source, and child-view handles so
// life-view and life-panel can verify cascading release when the owner is freed.
type lifeChild struct {
	name  string
	idle  bool
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

// lifeView proves releasing lifeKidView cascades release to every state, focus,
// row-source, and inner view handle created inside it (counts return to
// states=0 views=1 focus=0). In `life-view-again`, releasing lifeKid.input
// afterward proves double-release detection names the parent view cascade.
func lifeView(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		liveCounts(cx, "before")
		lifeKid = &lifeChild{name: "child"}
		lifeKidView = gk.NewView(nil, lifeKid.view)
	case 2:
		liveCounts(cx, "made")
	case 3:
		gk.Release(cx, lifeKidView)
		lifeKidView = gk.View{}
	case 4:
		liveCounts(cx, "released")
		if a.mode == "life-view-again" {
			fmt.Println("handles life releasing the child's state again")
			gk.Release(cx, lifeKid.input)
		}
	}
}

// lifeGC proves Go GC finalizers reclaim 3 unreachable InputState handles and 3
// FocusHandle handles (6 total released, focus 3->0, states 4->1) while keeping
// the live referenced `a.input` intact ("kept").
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
			time.Sleep(20 * time.Millisecond)
		}
	case 3:
		released, _ := gk.TestLife()
		liveCounts(cx, "collected")
		fmt.Printf("handles life gc released=%d kept=%q\n", released, a.input.Value(cx))
	}
}

var lifePanels [2]dock.Panel
var lifeDock dock.Area

// lifePanel proves removing panel 0 from a DockArea cascades release to all
// child states, views, and focus handles created inside that panel while
// leaving panel 1 docked (empty=false).
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
	case 2:
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
