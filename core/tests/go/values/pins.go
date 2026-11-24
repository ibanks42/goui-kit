package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
)

func init() {
	// Register value-pin retention tests across theme changes and view re-renders.
	modes["val-pins"] = &mode{click: pins, start: pinsStart,
		body: func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl { return body.ChildView(a.child) }}
}

func pinsStart(a *App, cx *gk.Cx) {
	a.diff = editor.NewDiffState(cx, nil).Build()
	a.ann = editor.NewDiffAnnotationLine(cx, "a", editor.NewDiffLinePosition(cx, "a.rs", editor.DiffSideModified, 1))
	a.child = gk.NewView(nil, a.childView)
}

// pins proves view-scoped value pins stay live across a theme-only redraw
// (click 2: builds=1, pinned=1, later=0), queue the old pin for deferred
// retirement when childView rebuilds (click 3: builds=2, pinned=1, later=1),
// and free the retired pin on the next frame (click 4: later=0).
func pins(a *App, cx *gk.Cx) {
	pinned, later := gk.TestValuePins(cx)
	fmt.Printf("values pins click=%d builds=%d pinned=%d later=%d\n", a.clicks, a.builds, pinned, later)
	switch a.clicks {
	case 1:
		gk.ThemeChange(cx, gk.ThemeModeDark)
	case 2:
		gk.Notify(cx, a.child)
	}
}

func (a *App) childView(cx *gk.Cx) gk.Element {
	a.builds++
	body := gk.Div().W(gk.Px(300)).H(gk.Px(200))
	if p, ok := a.ann.Position(cx).Get(); ok {
		body = body.Child(editor.Diff(a.diff).GutterMarkers([]editor.DiffGutterMarker{editor.NewDiffGutterMarker("m", p)}))
	}
	return body
}
