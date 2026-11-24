// misuse_lists: mistakes against the lists and editor families' protocols (docs/PROTOCOLS.md
// sections 9.9 to 9.12) that must stop with a message, never draw wrong content with exit 0. The
// port of examples/odin/negative_lists2; run offscreen with the diag library by
// gates/go-misuse.sh, one mode per case:
//
//	GK_NEG=sel-ok      a select over items, its value set and read back: correct (exit 0)
//	GK_NEG=sel-foreign gk.SetSearchItems on a row source that is not a select's: must stop
//	GK_NEG=sel-view    gk.SetSearchItems in a view function (a write): must stop
//	GK_NEG=nonzero     Easing's step count given 0 (upstream's NonZeroU32): must stop
//	GK_NEG=kept-view   a validator given to InputState.SetValidator in a view function (it
//	                   would be kept per build): must stop
//
// expr-syntax, expr-arg and expr-func (an expression closure given to StepBy) are in expr.go.
package main

import (
	"fmt"
	"os"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

type App struct {
	mode  string
	root  gk.View
	src   gk.RowSource
	sel   gk.SelectState
	inp   gk.InputState
	drawn int
}

var items = []gk.SearchItem{{Value: "a", Title: "A"}, {Value: "b", Title: "B"}}

// The modes of expr.go add their steps here.
var pendingInit = map[string]func(a *App, cx *gk.Cx){}

func (a *App) init(cx *gk.Cx) {
	switch a.mode {
	case "sel-ok", "sel-view":
		a.src = gk.NewSearchSource(cx, items)
		a.sel = gk.NewSelectState(cx, a.src, gk.NoRow).Build()
		a.sel.SetSelectedValue(cx, "b")
	case "sel-foreign":
		other := gk.NewRowSource(cx, 1, func(cx *gk.Cx, at gk.RowAt) gk.Element { return gk.Label("x") })
		gk.SetSearchItems(cx, other, items)
	case "kept-view":
		a.inp = gk.NewInputState(cx).Build()
	default:
		if f := pendingInit[a.mode]; f != nil {
			f(a, cx)
		}
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	a.drawn++
	body := gk.VFlex(s.SizeFull)
	switch a.mode {
	case "sel-ok":
		v, ok := a.sel.SelectedValue(cx).Get()
		fmt.Printf("gk-neg sel-ok value=%q ok=%v\n", v, ok)
		body = body.Child(gk.Select(a.sel))
	case "sel-view":
		gk.SetSearchItems(cx, a.src, items)
	case "nonzero":
		_ = gk.NewTransition(200 * time.Millisecond).Easing(gk.EasingSteps{Count: 0, Position: gk.StepPositionJumpEnd})
	case "kept-view":
		a.inp.SetValidator(cx, func(cx *gk.Cx, text string) bool { return true })
	}
	return body
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	gk.RunHeadless("size 400 300\nframe\nframe\n", func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "misuse_lists", 400, 300, a.root)
	})
	fmt.Printf("gk-neg done mode=%s drawn=%d\n", a.mode, a.drawn)
}
