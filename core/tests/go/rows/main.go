// rows: the protocol tests of the rows family (docs/PROTOCOLS.md section 9.5), the port of
// core/tests/rows: the kit's list, table and tree over row sources, offscreen under a gk-meter
// script (the diag library). Each mode prints lines core/tests/go/run-rows.sh checks, with the
// exit code.
//
//	GK_PROTO=list | list-search | list-sections | list-states | table | table-scroll | tree | released
//
// The table and tree modes are in tables.go.
package main

import (
	"fmt"
	"os"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

const (
	rowCount   = 10_000
	tableCount = 100_000
)

type App struct {
	mode      string
	root      gk.View
	list      gk.ListState
	src       gk.RowSource // the list's or table's row source
	table     gk.TableState
	sub       gk.Subscription
	tree      gk.TreeState
	treeRows  gk.RowSource
	empty     gk.View
	loading   gk.View
	clicks    int
	events    int
	query     string
	matches   []int          // list-search: the rows the query matches
	sorted    bool           // table: rows in descending order
	described map[uint64]int // descriptions per row or cell key
	empties   int            // descriptions of the empty view
	loadings  int
}

func key(section, row int) uint64 {
	return uint64(section)<<32 | uint64(uint32(row))
}

// variant names an event's variant as Odin's %v does (Go: the type's name and its fields).
func variant(e any) string {
	return strings.ReplaceAll(fmt.Sprintf("%#v", e), "gk.", "")
}

// --- lists ----------------------------------------------------------------------------------

func (a *App) listRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	a.described[key(at.Section, at.Row)]++
	var text string
	switch {
	case at.Row == gk.HeaderRow:
		text = fmt.Sprintf("section %d", at.Section)
		fmt.Printf("rows header section=%d\n", at.Section)
	case at.Row == gk.FooterRow:
		text = fmt.Sprintf("end of %d", at.Section)
		fmt.Printf("rows footer section=%d\n", at.Section)
	case a.mode == "list-search" && at.Row < len(a.matches):
		text = fmt.Sprintf("item %d", a.matches[at.Row])
	default:
		text = fmt.Sprintf("item %d.%d", at.Section, at.Row)
	}
	return gk.ListItem(gk.Key("item", at.Row)).Child(gk.Label(text))
}

func (a *App) listEvent(cx *gk.Cx, e gk.ListDelegateEvent) {
	a.events++
	ix, has := e.Index.Get()
	fmt.Printf("rows list %v has=%v section=%d row=%d secondary=%v id=%d query=%q\n", e.Kind, has, ix.Section, ix.Row, e.Secondary, e.ID, e.Query)
	if e.Kind == gk.ListConfirm {
		sel, ok := a.list.SelectedIndex(cx).Get()
		fmt.Printf("rows list-state read selected_index ok=%v row=%d\n", ok, sel.Row)
	}
	if e.Kind == gk.ListSearch {
		// Filter, set the count, then answer: the list's search task completes after it.
		a.query = e.Query
		a.matches = a.matches[:0]
		for i := range rowCount {
			if strings.Contains(fmt.Sprintf("item %d", i), a.query) {
				a.matches = append(a.matches, i)
			}
		}
		gk.SetCount(cx, a.src, len(a.matches))
		gk.Invalidate(cx, a.src, 0, len(a.matches))
		gk.AnswerSearch(cx, a.src, e.ID) // Odin: gk.answer(cx, a.src, e.id, true)
		fmt.Printf("rows search answered query=%q matches=%d\n", a.query, len(a.matches))
	}
}

// The list state's own events (the generated subscription).
func (a *App) listStateEvent(cx *gk.Cx, e gk.ListEvent) {
	fmt.Printf("rows list-state %s\n", variant(e))
}

// list-states: a button walks the states: empty (count 0), loading, more rows with has_more.
func (a *App) listStep(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	if a.mode == "table" {
		a.table.ScrollToRow(cx, 50_000) // a far jump through the generated write
		fmt.Printf("rows table-state scroll_to_row 50000\n")
		return
	}
	d := gk.DelegateData{Threshold: 20} // Odin: gk.default_delegate_data()
	d.Empty = a.empty
	d.LoadingView = a.loading
	switch a.clicks {
	case 1:
		gk.SetCount(cx, a.src, 0)
		gk.SetDelegateData(cx, a.list, d)
	case 2:
		d.Loading = true
		gk.SetCount(cx, a.src, 50)
		gk.SetDelegateData(cx, a.list, d)
	case 3:
		d.HasMore = true
		d.Threshold = 10
		gk.SetDelegateData(cx, a.list, d)
	}
	fmt.Printf("rows step=%d\n", a.clicks)
}

func (a *App) emptyView(cx *gk.Cx) gk.Element {
	a.empties++
	fmt.Printf("rows empty view described n=%d\n", a.empties)
	return gk.Label("nothing here")
}

func (a *App) initialView(cx *gk.Cx) gk.Element {
	fmt.Printf("rows initial view described\n")
	return gk.Label("type to search")
}

func (a *App) loadingView(cx *gk.Cx) gk.Element {
	a.loadings++
	fmt.Printf("rows loading view described n=%d\n", a.loadings)
	return gk.Label("loading")
}

// --- released ---------------------------------------------------------------------------------

func (a *App) releaseSource(cx *gk.Cx, _ gk.ClickEvent) {
	gk.Release(cx, a.src)
	fmt.Printf("rows released source\n")
}

// --- the views --------------------------------------------------------------------------------

func (a *App) init(cx *gk.Cx) {
	switch a.mode {
	case "list", "list-sections", "released":
		a.src = gk.NewListSource(cx, func() int { return rowCount }, a.listRow, gk.ListEvents(a.listEvent), gk.Initial(20))
		a.list = gk.NewListState(cx, a.src).Build()
		a.sub = a.list.OnListEvent(cx, a.listStateEvent)
		d := gk.DelegateData{Threshold: 20}
		d.PlaceholderHeight = 32
		if a.mode == "list-sections" {
			gk.SetSectionCount(cx, a.src, 0, 5)
			gk.SetSectionCount(cx, a.src, 1, 3)
			d.Headers, d.Footers = true, true
		}
		gk.SetDelegateData(cx, a.list, d)
	case "list-search":
		for i := range rowCount {
			a.matches = append(a.matches, i)
		}
		a.src = gk.NewListSource(cx, func() int { return rowCount }, a.listRow, gk.ListEvents(a.listEvent), gk.Initial(20))
		a.list = gk.NewListState(cx, a.src).Searchable(true).Build()
		// The view a searchable list shows while its query is empty.
		a.empty = gk.NewView(nil, a.initialView)
		gk.SetDelegateData(cx, a.list, gk.DelegateData{Threshold: 20, Initial: a.empty})
	case "list-states":
		a.empty = gk.NewView(nil, a.emptyView)
		a.loading = gk.NewView(nil, a.loadingView)
		a.src = gk.NewListSource(cx, func() int { return 30 }, a.listRow, gk.ListEvents(a.listEvent), gk.Initial(20))
		a.list = gk.NewListState(cx, a.src).Build()
	case "table", "table-scroll":
		a.tableInit(cx)
	case "tree":
		a.treeInit(cx)
	}
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	var body gk.Element
	switch a.mode {
	case "table", "table-scroll":
		body = gk.Div().W(gk.Px(440)).H(gk.Px(240)).Child(gk.DataTable(a.table))
	case "tree":
		body = gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(gk.Tree(a.tree, a.treeRows, s.SizeFull))
	default:
		body = gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(gk.List(a.list, s.SizeFull))
	}
	return gk.VFlex(s.SizeFull, s.P4, s.Gap2).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2).
			Child(gk.Button("step").Label("step").OnClick(a.listStep)).
			Child(gk.Button("release").Label("release").OnClick(a.releaseSource))).
		Child(body)
}

func (a *App) start(cx *gk.Cx) {
	a.root = gk.NewView(a.init, a.rootView)
	gk.OpenWindow(cx, "rows", 480, 320, a.root)
}

// Positions: the step button at (36, 30), the release button at (100, 30); the list, table or
// tree from (16, 56), 240 high.
const (
	stepClick    = "click 36 30\n"
	releaseClick = "click 100 30\n"
	row0         = "click 60 74\n"
)

func main() {
	a := &App{mode: os.Getenv("GK_PROTO"), described: map[uint64]int{}}
	switch a.mode {
	case "list", "list-search", "list-sections", "list-states", "released", "table", "table-scroll", "tree":
	default:
		fmt.Printf("rows: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	var sb strings.Builder
	sb.WriteString("size 480 320\nframe\nstat\n")
	switch a.mode {
	case "list":
		// a click on row 0 (select, confirm), down, a right click, escape, a far scroll (one
		// placeholder frame), back to the top (rows kept, nothing described again).
		sb.WriteString(row0)
		sb.WriteString("key down\nrclick 60 140\nkey escape\nscroll 160 200 -200000\nframe\nstat\nscroll 160 200 400000\nframe\n")
	case "list-search":
		// The search input is the list's first row: click it, type, the request is answered.
		sb.WriteString("click 120 72\ntype 77\nsleep 200\nframe\nsleep 200\nframe\nframe\n")
	case "list-sections":
		sb.WriteString("scroll 160 200 -400\nframe\n")
	case "list-states":
		sb.WriteString(stepClick)
		sb.WriteString("frame\n")
		sb.WriteString(stepClick)
		sb.WriteString("frame\n")
		sb.WriteString(stepClick)
		sb.WriteString("frame\nscroll 160 200 -2000\nframe\nframe\n")
	case "table":
		// a click on row 0, down, End (a far jump: one placeholder frame), a sort, a double
		// click, a column moved and one resized, Home.
		sb.WriteString("click 120 110\nkey down\nkey end\nframe\nstat\n")
		sb.WriteString(stepClick)
		sb.WriteString("frame\nclick 80 74\nframe\ndblclick 120 110\ndrag 160 74 330 74\ndrag 336 74 376 74\nkey home\nframe\n")
	case "table-scroll":
		// A steady scroll of 24 px before every frame, as the frame-cost scene does: rows are
		// bound ahead in batches, so no frame draws a placeholder.
		for range 120 {
			sb.WriteString("scroll 220 200 -24\n")
		}
	case "tree":
		sb.WriteString("click 60 152\nframe\nkey down\nframe\n")
	case "released":
		sb.WriteString(releaseClick)
		sb.WriteString("frame\n")
	}
	sb.WriteString("frame\nstat\n")
	gk.RunHeadless(sb.String(), a.start)
	again := 0
	for _, n := range a.described {
		if n > 1 {
			again++
		}
	}
	fmt.Printf("rows done mode=%s events=%d described=%d again=%d\n", a.mode, a.events, len(a.described), again)
}
