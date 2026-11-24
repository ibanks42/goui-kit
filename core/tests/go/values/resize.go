package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/base"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

// c6 holds the custom handle RowSource and ResizableState for `c6-resize`.
var c6 struct {
	handles gk.RowSource
	resizer base.ResizableState
	made    bool
}

func init() {
	modes["c6-resize"] = &mode{init: comp6Init, view: comp6View,
		// Move the pointer onto the horizontal handle at x=200 and then away to observe Idle -> Hovered transitions.
		script: "size 640 480\nframe\nframe\nmove 200 100\nframe\nframe\nframe\nmove 500 300\nframe\nframe\nframe\nstat\n"}
}

func comp6Init(a *App, cx *gk.Cx) {
	c6.handles = overlay.Items(cx, a.handleRow)
	c6.resizer = base.NewResizableState(cx)
	c6.made = true
}

func axisName(x gk.Axis) string {
	if x == gk.AxisHorizontal {
		return "Horizontal"
	}
	return "Vertical"
}

func edgeName(x base.HandleEdge) string {
	if x == base.HandleEdgeTrailing {
		return "Trailing"
	}
	return "Leading"
}

func stateName(x base.ResizeHandleState) string {
	return [...]string{"Idle", "Hovered", "Pressed", "Dragging"}[x]
}

// handleRow proves HResizable's custom handle appearance callback receives
// Horizontal axis, no outer edge (has_edge=false), and transitions from State
// Idle to Hovered when the pointer moves onto the divider at x=200.
func (a *App) handleRow(cx *gk.Cx, ctx base.ResizeHandleContext) gk.Element {
	edge, hasEdge := ctx.Edge.Get()
	fmt.Printf("values c6 handle row=%d axis=%s edge=%s has_edge=%v active=%v state=%s\n", 0, axisName(ctx.Axis), edgeName(edge),
		hasEdge, ctx.IsActive, stateName(ctx.State))
	color := theme.Border
	if ctx.IsActive || ctx.State == base.ResizeHandleStateHovered {
		color = theme.Primary
	}
	return gk.Div(s.HFull).W(gk.Px(1)).Bg(color)
}

func comp6View(a *App, cx *gk.Cx) (gk.Element, bool) {
	if !c6.made {
		return nil, false
	}
	left := base.ResizablePanel().Size(200).Child(gk.Text("left"))
	right := base.ResizablePanel().Child(gk.Text("right"))
	group := base.HResizable("g").WithState(c6.resizer).WithHandleAppearance(c6.handles).Child(left).Child(right)
	return gk.Div(s.SizeFull).Child(group), true
}
