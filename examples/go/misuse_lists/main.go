// Runtime misuse cases for searchable lists, selects, zero-disallowed counts,
// and writes or kept callbacks triggered inside view functions (GK_NEG).
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/motion"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/table"
)

type App struct {
	mode  string
	root  gk.View
	src   gk.RowSource
	sel   table.SelectState
	inp   editor.InputState
	drawn int
}

var items = []gk.SearchItem{{Value: "a", Title: "A"}, {Value: "b", Title: "B"}}

var pendingInit = map[string]func(a *App, cx *gk.Cx){}

func (a *App) init(cx *gk.Cx) {
	switch a.mode {
	case "sel-ok", "sel-view":
		a.src = gk.NewSearchSource(cx, items)
		a.sel = table.NewSelectState(cx, a.src, table.NoRow).Build()
		a.sel.SetSelectedValue(cx, "b")
	case "sel-foreign":
		other := gk.NewRowSource(cx, 1, func(cx *gk.Cx, at gk.RowAt) gk.Element { return kit.Label("x") })
		gk.SetSearchItems(cx, other, items)
	case "kept-view":
		a.inp = editor.NewInputState(cx).Build()
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
		body = body.Child(table.Select(a.sel))
	case "sel-view":
		gk.SetSearchItems(cx, a.src, items)
	case "nonzero":
		_ = motion.NewTransition(200 * time.Millisecond).Easing(gk.EasingSteps{Count: 0, Position: gk.StepPositionJumpEnd})
	case "kept-view":
		a.inp.SetValidator(cx, func(cx *gk.Cx, text string) bool { return true })
	}
	return body
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	if err := gk.RunHeadless("size 400 300\nframe\nframe\n", func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "misuse_lists", 400, 300, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gk-neg done mode=%s drawn=%d\n", a.mode, a.drawn)
}
