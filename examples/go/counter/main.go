// Counter example: splits the window into independent child views (controls, echo, list)
// so clicking Add or receiving background gk.Post ticks redraws only the affected subtree.
// Set GK_POSTS="rate,seconds" to stress-test background posting.
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

const rows = 10_000

type App struct {
	count, ticks               int
	name                       editor.InputState
	rows                       gk.Rows
	root, controls, echo, list gk.View
}

// init runs once when the window mounts the root view, creating window-backed state
// and preloading 10,000 rows into the library-owned Rows store.
func (a *App) init(cx *gk.Cx) {
	a.name = editor.NewInputState(cx).Placeholder("Name").Build()
	a.rows = gk.NewRows(cx)
	all := make([]string, rows)
	for i := range all {
		all[i] = fmt.Sprintf("row %d", i)
	}
	a.rows.Push(cx, all...)
}

// rootView mounts three independent child views so updates in controls or echo
// do not re-describe sibling views (gpui-fast retains unchanged views).
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

// echoView reads a.name.Value(cx), recording a state dependency so only echoView
// redraws when the user types into the input.
func (a *App) echoView(cx *gk.Cx) gk.Element {
	return gk.HFlex().Child(kit.Label("hello " + a.name.Value(cx)))
}

func (a *App) listView(cx *gk.Cx) gk.Element {
	return gk.UniformList("rows", a.rows).Style(s.SizeFull)
}

// add mutates Go state and buffers a row append; controlsView redraws automatically.
func (a *App) add(cx *gk.Cx, _ gk.ClickEvent) {
	a.count++
	a.rows.Push(cx, fmt.Sprintf("added %d", a.count))
}

// tick runs on the UI thread inside a coalesced WAKE turn from poster() and dirties
// only a.controls so a.echo and a.list stay untouched.
func (a *App) tick(cx *gk.Cx) {
	a.ticks++
	gk.Notify(cx, a.controls)
}

var sent atomic.Int64

func (a *App) poster(rate, secs int) {
	time.Sleep(time.Second)
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
