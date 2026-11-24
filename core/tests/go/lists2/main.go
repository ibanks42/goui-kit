// lists2: the protocol tests of the lists and editor families' searchable lists and table delegate
// data (docs/PROTOCOLS.md sections 9.9 and 9.10), the port of core/tests/lists2: the kit's Select
// and Combobox over the library's search delegate (items as data, rows, header rows, search
// requests), and a table's texts, row labels and looks. Offscreen under a gk-meter script (the
// diag library). Each mode prints lines core/tests/go/run-lists2.sh checks, with the exit code.
//
//	GK_PROTO=select | combobox | search | search-requests | rows | table | bad-source | bad-headers | released
//	         | mask | validate | text | display-map | providers | expr | bad-expr
//	         | drive | kept | status-colors | ask | diff   (docs/PROTOCOLS.md 9.13 and 9.14)
//	         | will-change                                 (docs/PROTOCOLS.md 11.25)
//	         | ctx-token | lend | highlight | languages | bad-lend | small   (round4.go)
//
// Most modes are in modes.go and round4.go, which register their init, step and view from init().
// There is no bad-ctx mode (a row's context read for a row no closure drew): an item function is
// handed its context's values (gk.Items), so there is no read of a context to misuse.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	gktable "gpuikit.dev/gk/table"
	"gpuikit.dev/gk/theme"
)

var fruit = []gk.SearchItem{
	{Value: "apple", Title: "Apple"},
	{Value: "banana", Title: "Banana", Keywords: "yellow"},
	{Value: "cherry", Title: "Cherry"},
	{Value: "grape", Title: "Grape", Disabled: true},
	{Value: "mango", Title: "Mango", Display: gk.Some("Mango (ripe)")},
}

var veg = []gk.SearchItem{{Value: "kale", Title: "Kale"}, {Value: "leek", Title: "Leek"}}

type App struct {
	mode     string
	root     gk.View
	src      gk.RowSource
	sel      gktable.SelectState
	combo    gktable.ComboboxState
	table    gktable.State
	cells    gk.RowSource
	group    gk.View
	sub      gk.Subscription
	step     int
	events   int
	rows     map[uint64]int
	requests int
	in1, in2 editor.InputState
	checks   int // validator calls
	ed       editor.State
	prov     gk.RowSource // the editor's providers' requests
	asked    map[string]int
	// drive: a list, a table and a tree selected and scrolled from code
	list     gktable.ListState
	tree     gktable.TreeState
	treeRows gk.RowSource
	drawn    map[string]int // "list 500", "table 700", "tree 150": rows described
	kept0    int
}

// What init, the step button and the view do for a mode of modes.go and round4.go.
var (
	modeInit = map[string]func(a *App, cx *gk.Cx){}
	modeStep = map[string]func(a *App, cx *gk.Cx){}
	modeView = map[string]func(a *App, cx *gk.Cx) gk.Element{}
)

// The modes whose code is all here; the others register.
var ready = map[string]bool{
	"select": true, "search": true, "search-requests": true, "rows": true, "bad-source": true, "bad-headers": true,
	"released": true, "mask": true, "validate": true, "text": true, "status-colors": true,
}

func key(section, row int) uint64 {
	return uint64(section)<<32 | uint64(uint32(row))
}

// strs prints strings in the form the checks expect: ["a", "b"].
func strs(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = fmt.Sprintf("%q", v)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// --- events -------------------------------------------------------------------------------------

func (a *App) selectEvent(cx *gk.Cx, e gktable.SelectEvent) {
	a.events++
	c, _ := e.(gktable.SelectEventConfirm)
	v, has := c.Value.Get()
	now, ok := a.sel.SelectedValue(cx).Get()
	fmt.Printf("lists2 select confirm has=%v value=%q now=%q same=%v\n", has, v, now, ok && now == v)
}

func (a *App) comboEvent(cx *gk.Cx, e gktable.ComboboxEvent) {
	a.events++
	switch v := e.(type) {
	case gktable.ComboboxEventChange:
		fmt.Printf("lists2 combobox change values=%s\n", strs(v.Value))
	case gktable.ComboboxEventConfirm:
		fmt.Printf("lists2 combobox confirm values=%s\n", strs(v.Value))
	}
}

// search-requests: the app answers a query with items of its own.
func (a *App) searchRequest(cx *gk.Cx, e gk.SearchRequest) {
	a.requests++
	fmt.Printf("lists2 search request id=%d query=%q\n", e.ID, e.Query)
	found := []gk.SearchItem{{Value: "xenon", Title: "Xenon"}, {Value: "xylophone", Title: "Xylophone"}}
	gk.SetSearchItems(cx, a.src, found)
	a.sel.SetItems(cx, a.src)
	gk.AnswerSearch(cx, a.src, e.ID)
}

// rows: an item's row and a group's header row, each a view described once.
func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) gk.Element {
	a.rows[key(at.Section, at.Row)]++
	if at.Row == gktable.HeaderRow {
		fmt.Printf("lists2 header group=%d\n", at.Section)
		if at.Section == 0 {
			return kit.Label("== fruit ==")
		}
		return kit.Label("== veg ==")
	}
	items := fruit
	if at.Section != 0 {
		items = veg
	}
	fmt.Printf("lists2 row group=%d row=%d value=%s\n", at.Section, at.Row, items[at.Row].Value)
	return kit.Label(fmt.Sprintf("> %s", items[at.Row].Title))
}

// drive: rows that say which of them were described (drawn after a scroll from code).
func (a *App) driveListRow(cx *gk.Cx, at gk.RowAt) gktable.ListItemEl {
	a.drawn[fmt.Sprintf("list %d", at.Row)]++
	return gktable.ListItem(gk.Key("item", at.Row)).Child(kit.Label(fmt.Sprintf("item %d", at.Row)))
}

func (a *App) drawnRow(what string, row int) bool {
	return a.drawn[fmt.Sprintf("%s %d", what, row)] > 0
}

// --- steps (the step button) ------------------------------------------------------------------

func (a *App) stepClicked(cx *gk.Cx, _ gk.ClickEvent) {
	a.step++
	if f, ok := modeStep[a.mode]; ok {
		f(a, cx)
		return
	}
	switch a.mode {
	case "select":
		switch a.step {
		case 1:
			a.sel.SetSelectedValue(cx, "cherry")
		case 2:
			v, ok := a.sel.SelectedValue(cx).Get()
			ix, iok := a.sel.SelectedIndex(cx).Get()
			fmt.Printf("lists2 select set_selected_value value=%q ok=%v row=%d index_ok=%v\n", v, ok, ix.Row, iok)
			a.sel.SetSelectedIndex(cx, gk.Some(gktable.Row(4)))
		case 3:
			v, _ := a.sel.SelectedValue(cx).Get()
			fmt.Printf("lists2 select set_selected_index value=%q\n", v)
			// New items: mango stays (by value), the others go.
			items := []gk.SearchItem{{Value: "mango", Title: "Mango"}, {Value: "plum", Title: "Plum"}}
			gk.SetSearchItems(cx, a.src, items)
			a.sel.SetItems(cx, a.src)
		case 4:
			v, ok := a.sel.SelectedValue(cx).Get()
			ix, _ := a.sel.SelectedIndex(cx).Get()
			fmt.Printf("lists2 select set_items value=%q ok=%v row=%d\n", v, ok, ix.Row)
			a.sel.SetSelectedIndex(cx, gktable.NoRow)
		case 5:
			_, ok := a.sel.SelectedValue(cx).Get()
			fmt.Printf("lists2 select cleared has=%v\n", ok)
		}
	case "released":
		gk.Release(cx, a.src)
		fmt.Printf("lists2 released source\n")
	case "status-colors":
		// A library object with no constructor but Default (PROTOCOLS.md 9.13): its reads.
		sc := editor.NewStatusColorsDefault(cx)
		e := sc.Error(cx)
		fmt.Printf("lists2 status error=%+v warning=%+v\n", e, sc.Warning(cx))
		fmt.Printf("lists2 status error_opaque=%v error_not_warning=%v created_is_success=%v deleted_is_error=%v border_is_error=%v\n",
			e.A == 1, e != sc.Warning(cx), sc.Created(cx) == sc.Success(cx), sc.Deleted(cx) == e, sc.ErrorBorder(cx) == e)
		syn := editor.NewSyntaxColorsDefault(cx)
		_, has := syn.Style(cx, "keyword").Get()
		_, has0 := syn.StyleForIndex(cx, 0).Get()
		fmt.Printf("lists2 status syntax keyword=%v index0=%v\n", has, has0)
		gk.Release(cx, sc)
		gk.Release(cx, syn)
	case "mask":
		switch a.step {
		case 1:
			// A mask the binding could not set before: the Number variant (a char separator).
			a.in2.SetMaskPattern(cx, editor.MaskPatternNumber{Separator: gk.Some(','), Fraction: gk.Some(2)})
		case 2:
			fmt.Printf("lists2 mask pattern=%q number=%q\n", a.in1.Value(cx), a.in2.Value(cx))
		}
	case "validate":
		fmt.Printf("lists2 validate value=%q calls=%d\n", a.in1.Value(cx), a.checks)
	case "text":
		a.in1.SetSelectedRange(cx, gk.Range{Lo: 6, Hi: 11})
		fmt.Printf("lists2 text text=%q selected=%q\n", a.in1.Text(cx), a.in1.SelectedText(cx))
	}
}

// validate: refuses any text with an x in it (a handler kept by the state's builder).
func (a *App) noX(cx *gk.Cx, text string) bool {
	a.checks++
	return !strings.Contains(text, "x")
}

// --- the views ----------------------------------------------------------------------------------

func (a *App) init(cx *gk.Cx) {
	if f, ok := modeInit[a.mode]; ok {
		f(a, cx)
		return
	}
	switch a.mode {
	case "select":
		a.src = gk.NewSearchSource(cx, fruit)
		a.sel = gktable.NewSelectState(cx, a.src, gktable.NoRow).Build()
		a.sub = a.sel.OnSelectEvent(cx, a.selectEvent)
	case "search":
		a.src = gk.NewSearchSource(cx, fruit)
		a.sel = gktable.NewSelectState(cx, a.src, gktable.NoRow).Searchable(true).Build()
		a.sub = a.sel.OnSelectEvent(cx, a.selectEvent)
	case "search-requests":
		a.src = gk.NewSearchSource(cx, fruit, gk.SearchRequests(a.searchRequest))
		a.sel = gktable.NewSelectState(cx, a.src, gktable.NoRow).Searchable(true).Build()
		a.sub = a.sel.OnSelectEvent(cx, a.selectEvent)
	case "rows", "released":
		groups := []gk.SearchGroup{{Title: "Fruit", Items: fruit}, {Title: "Veg", Items: veg}}
		a.src = gk.NewSearchSourceGroups(cx, groups, gk.SearchRow(a.itemRow), gk.SearchHeaders())
		a.sel = gktable.NewSelectState(cx, a.src, gktable.NoRow).Build()
		a.sub = a.sel.OnSelectEvent(cx, a.selectEvent)
	case "bad-source":
		other := gk.NewRowSource(cx, 3, func(cx *gk.Cx, at gk.RowAt) gk.Element { return kit.Label("x") })
		gk.SetSearchItems(cx, other, fruit)
	case "bad-headers":
		groups := []gk.SearchGroup{{Title: "Fruit", Items: fruit}}
		a.src = gk.NewSearchSourceGroups(cx, groups, gk.SearchHeaders())
	case "mask":
		a.in1 = editor.NewInputState(cx).MaskPattern("999-999").Build()
		a.in2 = editor.NewInputState(cx).Build()
	case "validate":
		a.in1 = editor.NewInputState(cx).Validate(a.noX).Build()
	case "text":
		a.in1 = editor.NewInputState(cx).DefaultValue("hello world").Build()
	}
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	var body gk.Element
	switch a.mode {
	case "combobox", "will-change":
		body = gktable.Combobox(a.combo).Placeholder("Fruit").W(gk.Px(220))
	case "table":
		body = gk.Div().W(gk.Px(440)).H(gk.Px(200)).Child(gktable.DataTable(a.table))
	case "bad-source", "bad-headers", "display-map", "status-colors", "diff", "kept-gone":
		body = kit.Label("-")
	case "drive":
		body = gk.HFlex(s.Gap2).
			Child(gk.Div().W(gk.Px(120)).H(gk.Px(200)).Child(gktable.List(a.list, s.SizeFull))).
			Child(gk.Div().W(gk.Px(200)).H(gk.Px(200)).Child(gktable.DataTable(a.table))).
			Child(gk.Div().W(gk.Px(120)).H(gk.Px(200)).Child(gktable.Tree(a.tree, a.treeRows, s.SizeFull)))
	case "expr":
		body = gk.VFlex(s.Gap2).W(gk.Px(200)).Child(editor.NumberInput(a.in1)).Child(editor.NumberInput(a.in2))
	case "bad-expr":
		body = kit.Label("-")
	case "mask", "validate", "text", "kept":
		body = gk.VFlex(s.Gap2).W(gk.Px(220)).Child(editor.Input(a.in1))
		if a.mode == "mask" {
			body = gk.VFlex(s.Gap2).W(gk.Px(220)).Child(editor.Input(a.in1)).Child(editor.Input(a.in2))
		}
	default:
		if f, ok := modeView[a.mode]; ok {
			body = f(a, cx) // ask, providers and the round-4 modes
		} else {
			body = gktable.Select(a.sel).Placeholder("Fruit").W(gk.Px(220))
		}
	}
	return gk.VFlex(s.SizeFull, s.P4, s.Gap2).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2).Child(kit.Button("step").Label("step").OnClick(a.stepClicked))).
		Child(body)
}

func (a *App) start(cx *gk.Cx) {
	a.root = gk.NewView(a.init, a.rootView)
	gk.OpenWindow(cx, "lists2", 480, 360, a.root)
}

// Positions: the step button at (36, 30); the select or combobox trigger from (16, 56), 220 wide.
const (
	stepClick = "click 36 30\nframe\n"
	openMenu  = "click 100 72\nframe\nframe\n"
)

// modeScript: the scripts of the round-4 modes (round4.go).
var modeScript = map[string]func(a *App, sb *strings.Builder){}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO"), rows: map[uint64]int{}, asked: map[string]int{}, drawn: map[string]int{}}
	_, registered := modeInit[a.mode]
	if _, ok := modeStep[a.mode]; ok {
		registered = true
	}
	if !ready[a.mode] && !registered {
		fmt.Printf("lists2: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	var sb strings.Builder
	sb.WriteString("size 480 360\nframe\n")
	switch a.mode {
	case "select":
		// Open the menu, Down to the first item, Enter confirms it; then the generated writes.
		sb.WriteString(openMenu)
		sb.WriteString("key down\nkey enter\nframe\n")
		for range 5 {
			sb.WriteString(stepClick)
		}
	case "search":
		// Searchable: the query input has the focus; "an" leaves Banana and Mango, the first one
		// under the cursor; Down and Enter take Mango.
		sb.WriteString(openMenu)
		sb.WriteString("type an\nframe\nframe\nkey down\nkey enter\nframe\n")
	case "search-requests":
		sb.WriteString(openMenu)
		sb.WriteString("type x\nsleep 100\nframe\nframe\nframe\nkey down\nkey enter\nframe\n")
	case "rows":
		sb.WriteString(openMenu)
		sb.WriteString("frame\nkey down\nkey enter\nframe\n")
	case "combobox":
		sb.WriteString(openMenu)
		sb.WriteString("key down\nkey enter\nframe\nkey escape\nframe\n")
		for range 3 {
			sb.WriteString(stepClick)
		}
	case "will-change":
		sb.WriteString(openMenu)
		sb.WriteString("key down\nkey enter\nframe\nkey escape\nframe\n")
		sb.WriteString(stepClick)
	case "table":
		sb.WriteString(stepClick)
		sb.WriteString("frame\n")
		sb.WriteString(stepClick)
	case "mask":
		sb.WriteString("click 100 72\ntype 123456\nframe\n")
		sb.WriteString(stepClick)
		sb.WriteString("click 100 112\ntype 1234567\nframe\n")
		sb.WriteString(stepClick)
	case "validate":
		sb.WriteString("click 100 72\ntype abxc\nframe\n")
		sb.WriteString(stepClick)
	case "text", "display-map", "status-colors", "diff":
		sb.WriteString(stepClick)
	case "kept":
		sb.WriteString(stepClick)
		sb.WriteString(stepClick)
		sb.WriteString(stepClick)
	case "ask":
		sb.WriteString("frame\nadvance 100\nframe\nadvance 100\nframe\nframe\n")
		sb.WriteString(stepClick)
	case "drive":
		for range 3 {
			sb.WriteString(stepClick)
			sb.WriteString("frame\nframe\nframe\n")
		}
	case "expr":
		// The increment buttons of both inputs, twice each.
		sb.WriteString("click 183 72\nframe\nclick 183 72\nframe\nclick 183 112\nframe\nclick 183 112\nframe\n")
		sb.WriteString(stepClick)
	case "providers":
		// Type, a trigger character, wait for the debounces, Enter takes the completion, ctrl-.
		// asks for code actions and Enter performs the first.
		sb.WriteString("click 200 100\ntype a\nframe\nadvance 500\nframe\ntype .\nframe\nframe\nadvance 500\nframe\nframe\n")
		sb.WriteString("key enter\nframe\nkey ctrl-.\nframe\nframe\nadvance 200\nframe\nkey enter\nframe\nframe\nadvance 500\nframe\n")
	case "ctx-token", "lend", "highlight", "languages", "bad-lend", "small":
		modeScript[a.mode](a, &sb)
	case "released":
		// The source released, then the menu drawn: its rows cannot be drawn (exit 2).
		sb.WriteString(stepClick)
		sb.WriteString(openMenu)
	}
	sb.WriteString("frame\n")
	if err := gk.RunHeadless(sb.String(), a.start); err != nil {
		log.Fatal(err)
	}
	again := 0
	for _, n := range a.rows {
		if n > 1 {
			again++
		}
	}
	if a.mode == "providers" {
		fmt.Printf("lists2 providers asked=%v value=%q\n", a.asked, "")
	}
	fmt.Printf("lists2 done mode=%s events=%d requests=%d rows=%d again=%d\n", a.mode, a.events, a.requests, len(a.rows), again)
}
