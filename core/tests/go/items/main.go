// items: item functions (docs/PROTOCOLS.md 9.41), the port of core/tests/items: one mode per kind
// of value the kit hands one, run offscreen (the diag library) under a gk-meter script;
// core/tests/go/run-items.sh checks the lines.
//
//	GK_PROTO=items-calendar    an element the kit built (a calendar's day) decorated by the app, with
//	                           the record of its state; the rows are described once and kept
//	GK_PROTO=items-time-field  the same over a time field's segments (the segment's kind and value)
//	GK_PROTO=items-tree        values 9.16 reads (the entry's index as the row's key) beside records
//	                           of a value's reads (TreeEntryReads, BaseTreeEntryState)
//	GK_PROTO=items-popover     kit state handed by reference with its own context (a popover's
//	                           content reads PopoverStateReads)
//	GK_PROTO=items-command     kit state handed by reference with the app's context (a command
//	                           palette's header and footer)
//	GK_PROTO=items-hover       a closure that returns one element type (a base hover card's content
//	                           is a StatefulDivEl), opened by the pointer
//	GK_PROTO=items-close       FnOnce(Button) -> E: the dialog's close button built from the app's ops
//	GK_PROTO=items-combobox    a generic context (ComboboxTriggerContext with the binding's delegate) as
//	                           a record: the trigger reads the selection and the placeholder
//	GK_PROTO=items-nav         an element handed with the record of its own reads (a nav stack's page)
//	GK_PROTO=items-wrong-kind  a time field's function given to a calendar: stops where it is given
//
// Every mode but items-close (gk.Items and the builders that take its source) is in items.go.
package main

import (
	"fmt"
	"os"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	mode      string
	root      gk.View
	cal       gk.CalendarState
	tf        gk.TimeFieldState
	tree      gk.TreeState
	cmd       gk.CommandState
	combo     gk.ComboboxState
	nav       gk.NavStackState
	page      gk.View
	builds    int
	described int // item rows described
	kinds     [gk.CalendarItemKindYear + 1]int
	segments  [gk.TimeSegmentPeriod + 1]int
	active    int
	after     int // rows described after the first three frames
	frames    int
}

// views: what a mode's view adds to the root, by mode; the others register from items.go.
var views = map[string]func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element{
	"items-close": closeView,
}

// modeInit: what a mode's init does, by mode (items.go).
var modeInit = map[string]func(a *App, cx *gk.Cx){}

var fruit = []gk.SearchItem{{Value: "apple", Title: "Apple"}, {Value: "pear", Title: "Pear"}}

// Go: the Go package's enums print as numbers; these print them as Odin's %v does.
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

// --- the view -------------------------------------------------------------------------------------

func (a *App) init(cx *gk.Cx) {
	gk.OnFrame(cx, a.onFrame)
	switch a.mode {
	case "items-calendar", "items-wrong-kind":
		a.cal = gk.NewCalendarState(cx).Build()
	case "items-time-field":
		a.tf = gk.NewTimeFieldState(cx).Build()
	case "items-tree":
		a.tree = gk.NewTreeState(cx).Items([]gk.TreeItem{gk.NewTreeItem("a", "Alpha"), gk.NewTreeItem("b", "Beta")}).Build()
	case "items-command":
		a.cmd = gk.NewCommandState(cx)
	case "items-combobox":
		a.combo = gk.NewComboboxState(cx, gk.NewSearchSource(cx, fruit), []gk.BaseIndexPath{{Row: 1}}).Build() // Pear
	case "items-nav":
		a.nav = gk.NewNavStackState(cx)
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
	close := gk.BaseDialogClose().Trigger(gk.BaseButton("x").Child(gk.Text("Close")))
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
	gk.RunHeadless(script, a.start)
	switch a.mode {
	case "items-calendar":
		fmt.Printf("items calendar described=%d days=%d weekdays=%d active=%d after=%d\n", a.described,
			a.kinds[gk.CalendarItemKindDay], a.kinds[gk.CalendarItemKindWeekday], a.active, a.after)
	case "items-time-field":
		fmt.Printf("items time field described=%d hours=%d minutes=%d\n", a.described, a.segments[gk.TimeSegmentHour],
			a.segments[gk.TimeSegmentMinute])
	}
	fmt.Printf("items done mode=%s described=%d builds=%d frames=%d\n", a.mode, a.described, a.builds, a.frames)
}
