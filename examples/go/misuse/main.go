// Core runtime misuse suite driven by gates/go-misuse.sh via GK_NEG.
// Exercises stale contexts, stale or double-attached elements, writes inside views,
// off-thread calls, invalid UTF-8, embedded NUL bytes, nil handlers, and app/gk.Go panics.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/chart"
	"gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	mode     string
	name     editor.InputState
	zero     editor.InputState
	selected string
	kept     string
	clicks   int
	cx       *gk.Cx
	old      gk.DivEl
	root     gk.View
	src      gk.RowSource
}

func (a *App) init(cx *gk.Cx) {
	b := editor.NewInputState(cx).DefaultValue("typed text")
	a.name = b.Build()
	switch a.mode {
	case "built":
		b.Placeholder("again")
	case "panel-name":
		dock.NewPanel(cx, gk.NewView(nil, a.view), dock.PanelData{Title: "Files"})
	case "row-write":
		a.src = gk.NewRowSource(cx, 3, func(cx *gk.Cx, at gk.RowAt) gk.Element {
			a.name.SetValue(cx, "written by a row")
			return kit.Label("row")
		})
	}
}

func (a *App) store(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	text := a.name.Value(cx)
	switch a.mode {
	case "keep":
		a.kept = text
	case "cx":
		a.cx = cx
	case "goroutine":
		done := make(chan struct{})
		go func() { defer close(done); gk.Notify(cx, a.root) }()
		<-done
	case "release":
		gk.Release(cx, a.name)
	case "zero":
		fmt.Println(a.zero.Value(cx))
	case "app-panic":
		var items []string
		fmt.Println(items[a.clicks+2])
	case "go-panic":
		gk.Go(cx, func() int {
			var m map[string]int
			m["x"] = 1
			return 0
		}, func(cx *gk.Cx, n int) {})
		time.Sleep(200 * time.Millisecond)
	case "utf8":
		a.name.SetValue(cx, "ab\xff\xfe")
	case "nul":
		a.name.SetValue(cx, "a\x00b")
		v := a.name.Value(cx)
		fmt.Printf("gk-neg nul len=%d same=%v\n", len(v), v == "a\x00b")
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	row := gk.HFlex(s.Gap2)
	switch a.mode {
	case "rune58":
		for i := range 3 {
			title := fmt.Sprintf("item %d of build after %d clicks", i, a.clicks)
			row.Child(kit.Button(gk.Key("item", i)).Label("item").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
				a.clicks++
				a.selected = title
			}))
		}
	case "cx":
		if a.cx != nil {
			gk.Notify(a.cx, a.root)
		}
		row.Child(kit.Button("b").Label("store").OnClick(a.store))
	case "element":
		if a.clicks > 0 {
			a.old.Text("again")
		}
		a.old = gk.Div()
		row.Child(kit.Button("b").Label("store").OnClick(a.store))
	case "twice":
		l := kit.Label("once")
		row.Child(l)
		row.Child(l)
	case "self":
		d := gk.Div().W(gk.Px(10))
		d.Child(d)
		return d
	case "view":
		row.ChildView(gk.NewView(nil, a.view))
	case "usize":
		row.Child(kit.Badge().Count(a.clicks - 1).Child(kit.Label("b")))
	case "nil":
		var f func(*gk.Cx, gk.ClickEvent)
		row.Child(kit.Button("b").Label("nil").OnClick(f))
	case "nil-child":
		var e gk.Element
		row.Child(e)
	case "data-column":
		type sale struct {
			Month string
			Total float64
		}
		sales := []sale{{"Jan", 3}, {"Feb", 5}}
		row.Child(chart.BarChart(chart.NewData(sales)).Band("month").Value("Total"))
	case "row-write":
		row.Child(gk.SourceList("rows", a.src, 20).H(gk.Px(60)))
	default:
		row.Child(kit.Button("b").Label("store").OnClick(a.store))
	}
	fmt.Printf("gk-neg view clicks=%d selected=%q kept=%q\n", a.clicks, a.selected, a.kept)
	col := gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(row).
		Child(kit.Label(fmt.Sprintf("selected %s kept %s", a.selected, a.kept)))
	if a.mode == "release" {
		col.Child(gk.Div().W(gk.Px(200)).Child(editor.Input(a.name)))
	}
	return col
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	script := "size 480 200\nframe\nclick 40 32\nframe\nclick 40 32\nframe\nclick 40 32\nframe\nframe\n"
	if err := gk.RunHeadless(script, func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "misuse", 480, 200, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gk-neg done mode=%s clicks=%d selected=%q kept=%q\n", a.mode, a.clicks, a.selected, a.kept)
}
