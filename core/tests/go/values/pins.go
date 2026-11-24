// val-pins: the temporary handles pinned to views, now and released later (gk.TestValuePins;
// Odin's gk.test_value_pins).
package main

import (
	"fmt"

	"gpuikit.dev/gk"
)

func init() {
	modes["val-pins"] = &mode{click: pins, start: pinsStart,
		body: func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl { return body.ChildView(a.child) }}
}

func pinsStart(a *App, cx *gk.Cx) {
	a.diff = gk.NewDiffState(cx, nil).Build()
	a.ann = gk.NewDiffAnnotationLine(cx, "a", gk.NewDiffLinePosition(cx, "a.rs", gk.DiffSideModified, 1))
	a.child = gk.NewView(nil, a.childView)
}

// A temporary handle (a read's result) placed in a view's elements: pinned to that view, so a redraw
// of the same tape frames later still finds it (Theme::change redraws every window without describing
// the view again); a newer build releases the old one at the next sweep.
func pins(a *App, cx *gk.Cx) {
	pinned, later := gk.TestValuePins(cx)
	fmt.Printf("values pins click=%d builds=%d pinned=%d later=%d\n", a.clicks, a.builds, pinned, later)
	switch a.clicks {
	case 1:
		gk.ThemeChange(cx, gk.ThemeModeDark) // every window drawn again, the child's tape unchanged
	case 2:
		gk.Notify(cx, a.child) // the child described again: a new temporary handle, the old one released later
	}
}

func (a *App) childView(cx *gk.Cx) gk.Element {
	a.builds++
	body := gk.Div().W(gk.Px(300)).H(gk.Px(200))
	if p, ok := a.ann.Position(cx).Get(); ok {
		body = body.Child(gk.Diff(a.diff).GutterMarkers([]gk.DiffGutterMarker{gk.NewDiffGutterMarker("m", p)}))
	}
	return body
}
