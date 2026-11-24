// counter: a small app in Go, the same app as ref/src/bin/counter.rs:
// a kit Button, a Label with the count, an Input echoed into a second
// Label, and a list of 10,000 rows, in three views inside a root view.
//
//	counter-go                 a real window
//	GK_SCRIPT=file counter-go  offscreen under a gk-meter script (the diag library)
//	GK_POSTS=rate,seconds      post from a goroutine at that rate
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

const rows = 10_000

type App struct {
	count, ticks               int
	name                       editor.InputState // handles to kit state that lives in the library
	rows                       gk.Rows
	root, controls, echo, list gk.View
}

func (a *App) init(cx *gk.Cx) {
	a.name = editor.NewInputState(cx).Placeholder("Name").Build()
	a.rows = gk.NewRows(cx)
	all := make([]string, rows)
	for i := range all {
		all[i] = fmt.Sprintf("row %d", i)
	}
	a.rows.Push(cx, all...) // one crossing for all rows
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Bg(theme.Background).
		TextColor(theme.Foreground).
		ChildView(a.controls).
		ChildView(a.echo).
		Child(gk.Div(s.WFull).H(gk.Px(160)).ChildView(a.list))
}

func (a *App) controlsView(cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(kit.Button("add").Primary().Label("Add").OnClick(a.add)).
		Child(kit.Label(fmt.Sprintf("count %d", a.count+a.ticks))).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name)))
}

// echoView reads kit state through its handle: the library observes the input and has this
// view described again when it changes.
func (a *App) echoView(cx *gk.Cx) gk.Element {
	return gk.HFlex().Child(kit.Label("hello " + a.name.Value(cx)))
}

func (a *App) listView(cx *gk.Cx) gk.Element {
	return gk.UniformList("rows", a.rows).Style(s.SizeFull)
}

func (a *App) add(cx *gk.Cx, _ gk.ClickEvent) {
	a.count++
	a.rows.Push(cx, fmt.Sprintf("added %d", a.count))
} // the controls view, which registered this handler, is marked dirty on return

func (a *App) tick(cx *gk.Cx) {
	a.ticks++
	gk.Notify(cx, a.controls)
}

var sent atomic.Int64

func (a *App) poster(rate, secs int) {
	time.Sleep(time.Second) // let the window come up
	end := time.Now().Add(time.Duration(secs) * time.Second)
	for time.Now().Before(end) {
		for range rate / 1000 {
			gk.Post(a.tick)
			sent.Add(1)
		}
		time.Sleep(time.Millisecond)
	}
}

func main() {
	a := &App{}
	if posts := os.Getenv("GK_POSTS"); posts != "" {
		if parts := strings.Split(posts, ","); len(parts) == 2 {
			rate, _ := strconv.Atoi(parts[0])
			secs, _ := strconv.Atoi(parts[1])
			go a.poster(rate, secs)
		}
	}
	if err := gk.Run(func(cx *gk.Cx) {
		a.controls = gk.NewView(nil, a.controlsView)
		a.echo = gk.NewView(nil, a.echoView)
		a.list = gk.NewView(nil, a.listView)
		a.root = gk.NewView(a.init, a.rootView)
		gk.OpenWindow(cx, "Counter", 640, 480, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gk-app counter-go count=%d ticks=%d posts_sent=%d\n", a.count, a.ticks, sent.Load())
}
