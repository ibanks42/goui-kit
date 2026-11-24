// Command charts runs headless protocol tests for chart layout helpers (`pie`, `stack`).
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
)

type App struct {
	mode string
	root gk.View
}

// modes maps GK_PROTO values to chart layout test functions.
var modes = map[string]func(a *App, cx *gk.Cx){}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(kit.Label("charts"))
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
	if err := gk.RunHeadless("size 320 240\nframe\nadvance 10\nframe\nframe\nadvance 20\nframe\nstat\n", func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.rootView)
		gk.OpenWindow(cx, "charts", 320, 240, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}
