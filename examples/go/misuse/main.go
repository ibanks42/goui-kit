// misuse: the mistakes the Go API must make impossible or loud (docs/GO.md section 5).
// Each mode is one mistake; gates/go-misuse.sh holds the
// expected outcome. Runs offscreen under a script that clicks the first button a few times.
// The mistakes Go's compiler catches are in examples/go/misuse_compile.
//
//	GK_NEG=rune58    a string made during a view is captured by a handler that stores it:
//	                 must show the right content
//	GK_NEG=keep      event text stored: correct (Go strings are copies)
//	GK_NEG=cx        a *gk.Cx stored and used in a later turn: must stop
//	GK_NEG=element   an element kept across view builds: must stop
//	GK_NEG=twice     an element given to two parents: must stop
//	GK_NEG=self      an element made a child of itself: must stop
//	GK_NEG=view      a view made inside a view function: must stop
//	GK_NEG=usize     a negative int where the kit takes a count: must stop
//	GK_NEG=nil       a nil function as a handler: must stop
//	GK_NEG=zero      a state handle that was never made: must stop, saying so
//	GK_NEG=built     a state builder chain used after Build: must stop
//	GK_NEG=utf8      a string that is not UTF-8: must stop
//	GK_NEG=nul       a string with a NUL inside: correct
//	GK_NEG=goroutine a UI call from another goroutine: must stop
//	GK_NEG=release   kit state released while a view still shows it: must stop when drawn
//	GK_NEG=row-write a row function that writes: must stop
//	GK_NEG=nil-child a nil element given to Child: must stop
//	GK_NEG=data-column a chart names a column its rows do not have: must stop, listing them
//	GK_NEG=panel-name a dock panel without a name: must stop
//	GK_NEG=app-panic an index out of range in a handler (the app's own bug): must stop, naming the line
//	GK_NEG=go-panic  a panic in gk.Go's work function, on its goroutine: must stop, naming the gk.Go
package main

import (
	"fmt"
	"os"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	mode     string
	name     gk.InputState
	zero     gk.InputState
	selected string // stored by a handler
	kept     string
	clicks   int
	cx       *gk.Cx
	old      gk.DivEl
	root     gk.View
	src      gk.RowSource
}

func (a *App) init(cx *gk.Cx) {
	b := gk.NewInputState(cx).DefaultValue("typed text")
	a.name = b.Build()
	switch a.mode {
	case "built":
		b.Placeholder("again") // the chain was used up by Build
	case "panel-name":
		gk.NewPanel(cx, gk.NewView(nil, a.view), gk.PanelData{Title: "Files"}) // no Name
	case "row-write":
		a.src = gk.NewRowSource(cx, 3, func(cx *gk.Cx, at gk.RowAt) gk.Element {
			a.name.SetValue(cx, "written by a row") // a row function only describes
			return gk.Label("row")
		})
	}
}

func (a *App) store(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	text := a.name.Value(cx)
	switch a.mode {
	case "keep":
		a.kept = text // a copy: always safe to keep
	case "cx":
		a.cx = cx
	case "goroutine":
		done := make(chan struct{})
		go func() { defer close(done); gk.Notify(cx, a.root) }()
		<-done
	case "release":
		gk.Release(cx, a.name) // the view still shows the input
	case "zero":
		fmt.Println(a.zero.Value(cx))
	case "app-panic":
		var items []string
		fmt.Println(items[a.clicks+2]) // out of range
	case "go-panic":
		gk.Go(cx, func() int {
			var m map[string]int
			m["x"] = 1 // a nil map
			return 0
		}, func(cx *gk.Cx, n int) {})
		select {} // the goroutine ends the program
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
			// A different string in every build, so a stale one would show.
			title := fmt.Sprintf("item %d of build after %d clicks", i, a.clicks)
			row.Child(gk.Button(gk.Key("item", i)).Label("item").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
				a.clicks++
				a.selected = title
			}))
		}
	case "cx":
		if a.cx != nil {
			gk.Notify(a.cx, a.root) // a Cx from an earlier turn
		}
		row.Child(gk.Button("b").Label("store").OnClick(a.store))
	case "element":
		if a.clicks > 0 {
			a.old.Text("again") // an element of an earlier build
		}
		a.old = gk.Div()
		row.Child(gk.Button("b").Label("store").OnClick(a.store))
	case "twice":
		l := gk.Label("once")
		row.Child(l)
		row.Child(l)
	case "self":
		d := gk.Div().W(gk.Px(10))
		d.Child(d)
		return d
	case "view":
		row.ChildView(gk.NewView(nil, a.view))
	case "usize":
		row.Child(gk.Badge().Count(a.clicks - 1).Child(gk.Label("b")))
	case "nil":
		var f func(*gk.Cx, gk.ClickEvent)
		row.Child(gk.Button("b").Label("nil").OnClick(f))
	case "nil-child":
		var e gk.Element
		row.Child(e)
	case "data-column":
		type sale struct {
			Month string
			Total float64
		}
		sales := []sale{{"Jan", 3}, {"Feb", 5}}
		row.Child(gk.BarChart(gk.NewData(sales)).Band("month").Value("Total")) // the field is Month
	case "row-write":
		row.Child(gk.SourceList("rows", a.src, 20).H(gk.Px(60)))
	default:
		row.Child(gk.Button("b").Label("store").OnClick(a.store))
	}
	fmt.Printf("gk-neg view clicks=%d selected=%q kept=%q\n", a.clicks, a.selected, a.kept)
	col := gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(row).
		Child(gk.Label(fmt.Sprintf("selected %s kept %s", a.selected, a.kept)))
	if a.mode == "release" {
		col.Child(gk.Div().W(gk.Px(200)).Child(gk.Input(a.name)))
	}
	return col
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	script := "size 480 200\nframe\nclick 40 32\nframe\nclick 40 32\nframe\nclick 40 32\nframe\nframe\n"
	gk.RunHeadless(script, func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "misuse", 480, 200, a.root)
	})
	fmt.Printf("gk-neg done mode=%s clicks=%d selected=%q kept=%q\n", a.mode, a.clicks, a.selected, a.kept)
}
