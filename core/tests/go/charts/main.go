// charts: the protocol tests of the charts' calculators over the app's rows (docs/PROTOCOLS.md
// 12.15), the port of core/tests/charts: the pie and stack layouts as library objects instantiated
// for a row of a data table, their value closures answered from a column (or, for a stack, from the
// column each key names), their results read back with each row as its index in the rows the app
// passed. Each mode runs offscreen (the diag library) and prints lines core/tests/go/run-charts.sh
// checks.
//
//	GK_PROTO=pie | stack
//
// Both modes are in arcs.go.
package main

import (
	"fmt"
	"os"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

type App struct {
	mode string
	root gk.View
}

// modes: what a timer runs 10 ms in, by mode. They register from arcs.go.
var modes = map[string]func(a *App, cx *gk.Cx){}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(gk.Label("charts"))
}

func (a *App) init(cx *gk.Cx) {
	if f, ok := modes[a.mode]; ok {
		gk.After(cx, 10*time.Millisecond, func(cx *gk.Cx) { f(a, cx) })
	}
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	if _, ok := modes[a.mode]; !ok {
		fmt.Printf("charts: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	gk.RunHeadless("size 320 240\nframe\nadvance 10\nframe\nframe\nadvance 20\nframe\nstat\n", func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.rootView)
		gk.OpenWindow(cx, "charts", 320, 240, a.root)
	})
}
