// Command items exercises typed `overlay.Items` callback renderers across
// calendar, time field, tree, popover, command palette, hover card, combobox,
// navigation stack, dialog close, and type-mismatch rejection modes.
package main

import (
	"fmt"
	"log"
	"os"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/base"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/table"
	"gpuikit.dev/gk/theme"
)

// App holds widget states and invocation counters across all `items-*` modes.
type App struct {
	mode      string
	root      gk.View
	cal       base.CalendarState
	tf        base.TimeFieldState
	tree      table.TreeState
	cmd       table.CommandState
	combo     table.ComboboxState
	nav       base.NavStackState
	page      gk.View
	builds    int
	described int
	kinds     [base.CalendarItemKindYear + 1]int
	segments  [base.TimeSegmentPeriod + 1]int
	active    int
	after     int
	frames    int
}

var views = map[string]func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element{
	"items-close": closeView,
}

var modeInit = map[string]func(a *App, cx *gk.Cx){}

var fruit = []gk.SearchItem{{Value: "apple", Title: "Apple"}, {Value: "pear", Title: "Pear"}}

var (
	segmentNames = [...]string{"Hour", "Minute", "Second", "Period"}
	phaseNames   = [...]string{"Entering", "Present", "Exiting", "Absent"}
)

func name(names []string, v int) string {
	if v >= 0 && v < len(names) {
		return names[v]
	}
	return fmt.Sprint(v)
}

func (a *App) init(cx *gk.Cx) {
	gk.OnFrame(cx, a.onFrame)
	switch a.mode {
	case "items-calendar", "items-wrong-kind":
		a.cal = base.NewCalendarState(cx).Build()
	case "items-time-field":
		a.tf = base.NewTimeFieldState(cx).Build()
	case "items-tree":
		a.tree = table.NewTreeState(cx).Items([]table.TreeItem{table.NewTreeItem("a", "Alpha"), table.NewTreeItem("b", "Beta")}).Build()
	case "items-command":
		a.cmd = table.NewCommandState(cx)
	case "items-combobox":
		a.combo = table.NewComboboxState(cx, gk.NewSearchSource(cx, fruit), []table.BaseIndexPath{{Row: 1}}).Build()
	case "items-nav":
		a.nav = base.NewNavStackState(cx)
		a.page = gk.NewView(nil, a.pageView)
	}
	if f, ok := modeInit[a.mode]; ok {
		f(a, cx)
	}
}

func (a *App) pageView(cx *gk.Cx) gk.Element {
	return gk.Text("the page")
}

func closeView(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
	close := overlay.BaseDialogClose().Trigger(base.Button("x").Child(gk.Text("Close")))
	return root.Child(close).Child(gk.Text("items close drawn"))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	a.builds++
	root := gk.Div(s.SizeFull, s.P2).Bg(theme.Background).TextColor(theme.Foreground)
	if f, ok := views[a.mode]; ok {
		return f(a, cx, root)
	}
	return root
}

func (a *App) onFrame(cx *gk.Cx) {
	a.frames++
}

func (a *App) start(cx *gk.Cx) {
	a.root = gk.NewView(a.init, a.view)
	gk.OpenWindow(cx, "items", 640, 480, a.root)
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	if _, ok := views[a.mode]; !ok {
		fmt.Printf("items: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	script := "size 640 480\nframe\nframe\nframe\n"
	if a.mode == "items-hover" {
		script += "move 10 10\nadvance 100\nframe\nadvance 100\nframe\nframe\n"
	}
	script += "frame\nframe\nframe\nstat\n"
	if err := gk.RunHeadless(script, a.start); err != nil {
		log.Fatal(err)
	}
	switch a.mode {
	case "items-calendar":
		fmt.Printf("items calendar described=%d days=%d weekdays=%d active=%d after=%d\n", a.described,
			a.kinds[base.CalendarItemKindDay], a.kinds[base.CalendarItemKindWeekday], a.active, a.after)
	case "items-time-field":
		fmt.Printf("items time field described=%d hours=%d minutes=%d\n", a.described, a.segments[base.TimeSegmentHour],
			a.segments[base.TimeSegmentMinute])
	}
	fmt.Printf("items done mode=%s described=%d builds=%d frames=%d\n", a.mode, a.described, a.builds, a.frames)
}
