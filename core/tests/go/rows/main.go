// Command rows runs headless protocol tests for virtualized List, DataTable,
// and Tree row sources: row paging, search requests, section headers/footers,
// empty/loading/load-more states, sorting, column moves, and released sources.
package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	gktable "github.com/ibanks42/goui-kit/gk/table"
	"github.com/ibanks42/goui-kit/gk/theme"
)

const (
	rowCount   = 10_000
	tableCount = 100_000
)

// App holds list, table, and tree handles plus render/event counters for GK_PROTO.
type App struct {
	mode      string
	root      gk.View
	list      gktable.ListState
	src       gk.RowSource
	table     gktable.State
	sub       gk.Subscription
	tree      gktable.TreeState
	treeRows  gk.RowSource
	empty     gk.View
	loading   gk.View
	clicks    int
	events    int
	query     string
	matches   []int
	sorted    bool
	described map[uint64]int
	empties   int
	loadings  int
}

func key(section, row int) uint64 {
	return uint64(section)<<32 | uint64(uint32(row))
}

var pkgQual = regexp.MustCompile(`\b(gk|rt|table|kit|editor|base|chart|overlay|dock|motion)\.`)

func variant(e any) string {
	return pkgQual.ReplaceAllString(fmt.Sprintf("%#v", e), "")
}

// listRow builds list items as well as section header/footer sentinel rows.
func (a *App) listRow(cx *gk.Cx, at gk.RowAt) gktable.ListItemEl {
	a.described[key(at.Section, at.Row)]++
	var text string
	switch {
	case at.Row == gktable.HeaderRow:
		text = fmt.Sprintf("section %d", at.Section)
		fmt.Printf("rows header section=%d\n", at.Section)
	case at.Row == gktable.FooterRow:
		text = fmt.Sprintf("end of %d", at.Section)
		fmt.Printf("rows footer section=%d\n", at.Section)
	case a.mode == "list-search" && at.Row < len(a.matches):
		text = fmt.Sprintf("item %d", a.matches[at.Row])
	default:
		text = fmt.Sprintf("item %d.%d", at.Section, at.Row)
	}
	return gktable.ListItem(gk.Key("item", at.Row)).Child(kit.Label(text))
}

// listEvent logs list delegate callbacks and answers asynchronous ListSearch requests.
func (a *App) listEvent(cx *gk.Cx, e gktable.ListDelegateEvent) {
	a.events++
	ix, has := e.Index.Get()
	fmt.Printf("rows list %v has=%v section=%d row=%d secondary=%v id=%d query=%q\n", e.Kind, has, ix.Section, ix.Row, e.Secondary, e.ID, e.Query)
	if e.Kind == gktable.ListConfirm {
		sel, ok := a.list.SelectedIndex(cx).Get()
		fmt.Printf("rows list-state read selected_index ok=%v row=%d\n", ok, sel.Row)
	}
	if e.Kind == gktable.ListSearch {

		a.query = e.Query
		a.matches = a.matches[:0]
		for i := range rowCount {
			if strings.Contains(fmt.Sprintf("item %d", i), a.query) {
				a.matches = append(a.matches, i)
			}
		}
		gk.SetCount(cx, a.src, len(a.matches))
		gk.Invalidate(cx, a.src, 0, len(a.matches))
		gk.AnswerSearch(cx, a.src, e.ID)
		fmt.Printf("rows search answered query=%q matches=%d\n", a.query, len(a.matches))
	}
}

func (a *App) listStateEvent(cx *gk.Cx, e gktable.ListEvent) {
	fmt.Printf("rows list-state %s\n", variant(e))
}

// listStep advances the empty -> loading -> load-more sequence (or scrolls the table in `table` mode).
func (a *App) listStep(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	if a.mode == "table" {
		a.table.ScrollToRow(cx, 50_000)
		fmt.Printf("rows table-state scroll_to_row 50000\n")
		return
	}
	d := gktable.DelegateData{Threshold: 20}
	d.Empty = a.empty
	d.LoadingView = a.loading
	switch a.clicks {
	case 1:
		gk.SetCount(cx, a.src, 0)
		gktable.SetDelegateData(cx, a.list, d)
	case 2:
		d.Loading = true
		gk.SetCount(cx, a.src, 50)
		gktable.SetDelegateData(cx, a.list, d)
	case 3:
		d.HasMore = true
		d.Threshold = 10
		gktable.SetDelegateData(cx, a.list, d)
	}
	fmt.Printf("rows step=%d\n", a.clicks)
}

func (a *App) emptyView(cx *gk.Cx) gk.Element {
	a.empties++
	fmt.Printf("rows empty view described n=%d\n", a.empties)
	return kit.Label("nothing here")
}

func (a *App) initialView(cx *gk.Cx) gk.Element {
	fmt.Printf("rows initial view described\n")
	return kit.Label("type to search")
}

func (a *App) loadingView(cx *gk.Cx) gk.Element {
	a.loadings++
	fmt.Printf("rows loading view described n=%d\n", a.loadings)
	return kit.Label("loading")
}

func (a *App) releaseSource(cx *gk.Cx, _ gk.ClickEvent) {
	gk.Release(cx, a.src)
	fmt.Printf("rows released source\n")
}

// init builds the row source and state for the active GK_PROTO mode:
//   - list / list-sections / released: 10k-row List verifying selection/confirm/
//     right-click/cancel events, multi-section headers/footers, or released-source guard.
//   - list-search: Searchable List proving async search filters 10k rows to 280 "77" matches.
//   - list-states: List stepping through empty (count=0), loading, and LoadMore states.
//   - table / table-scroll: 100k-row DataTable verifying sorting, jumps, column moves, and prefetch.
//   - tree: Hierarchical Tree verifying folder expansion and selection events.
func (a *App) init(cx *gk.Cx) {
	switch a.mode {
	case "list", "list-sections", "released":
		a.src = gktable.NewListSource(cx, func() int { return rowCount }, a.listRow, gktable.ListEvents(a.listEvent), gk.Initial(20))
		a.list = gktable.NewListState(cx, a.src).Build()
		a.sub = a.list.OnListEvent(cx, a.listStateEvent)
		d := gktable.DelegateData{Threshold: 20}
		d.PlaceholderHeight = 32
		if a.mode == "list-sections" {
			gk.SetSectionCount(cx, a.src, 0, 5)
			gk.SetSectionCount(cx, a.src, 1, 3)
			d.Headers, d.Footers = true, true
		}
		gktable.SetDelegateData(cx, a.list, d)
	case "list-search":
		for i := range rowCount {
			a.matches = append(a.matches, i)
		}
		a.src = gktable.NewListSource(cx, func() int { return rowCount }, a.listRow, gktable.ListEvents(a.listEvent), gk.Initial(20))
		a.list = gktable.NewListState(cx, a.src).Searchable(true).Build()

		a.empty = gk.NewView(nil, a.initialView)
		gktable.SetDelegateData(cx, a.list, gktable.DelegateData{Threshold: 20, Initial: a.empty})
	case "list-states":
		a.empty = gk.NewView(nil, a.emptyView)
		a.loading = gk.NewView(nil, a.loadingView)
		a.src = gktable.NewListSource(cx, func() int { return 30 }, a.listRow, gktable.ListEvents(a.listEvent), gk.Initial(20))
		a.list = gktable.NewListState(cx, a.src).Build()
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
		body = gk.Div().W(gk.Px(440)).H(gk.Px(240)).Child(gktable.DataTable(a.table))
	case "tree":
		body = gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(gktable.Tree(a.tree, a.treeRows, s.SizeFull))
	default:
		body = gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(gktable.List(a.list, s.SizeFull))
	}
	return gk.VFlex(s.SizeFull, s.P4, s.Gap2).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2).
			Child(kit.Button("step").Label("step").OnClick(a.listStep)).
			Child(kit.Button("release").Label("release").OnClick(a.releaseSource))).
		Child(body)
}

func (a *App) start(cx *gk.Cx) {
	a.root = gk.NewView(a.init, a.rootView)
	gk.OpenWindow(cx, "rows", 480, 320, a.root)
}

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

		sb.WriteString(row0)
		sb.WriteString("key down\nrclick 60 140\nkey escape\nscroll 160 200 -200000\nframe\nstat\nscroll 160 200 400000\nframe\n")
	case "list-search":

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

		sb.WriteString("click 120 110\nkey down\nkey end\nframe\nstat\n")
		sb.WriteString(stepClick)
		sb.WriteString("frame\nclick 80 74\nframe\ndblclick 120 110\ndrag 160 74 330 74\ndrag 336 74 376 74\nkey home\nframe\n")
	case "table-scroll":

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
	if err := gk.RunHeadless(sb.String(), a.start); err != nil {
		log.Fatal(err)
	}
	again := 0
	for _, n := range a.described {
		if n > 1 {
			again++
		}
	}
	fmt.Printf("rows done mode=%s events=%d described=%d again=%d\n", a.mode, a.events, len(a.described), again)
}
