// rows: the protocol tests of the rows family (docs/PROTOCOLS.md section 9.5): the kit's
// list, table and tree over row sources, offscreen under a gk-meter script (the diag
// library). Each mode prints lines core/tests/run-rows.sh checks, with the exit code.
//
//   GK_PROTO=list | list-search | list-sections | list-states | table | table-scroll | tree | released
package rows

import "core:fmt"
import "core:os"
import "core:strings"
import gk "gpuikit:gk"

ROWS :: 10_000
TABLE_ROWS :: 100_000

App :: struct {
	mode:      string,
	root:      gk.View,
	list:      gk.List_State,
	src:       gk.Row_Source, // the list's or table's row source
	table:     gk.Table_State,
	sub:       gk.Subscription,
	tree:      gk.Tree_State,
	tree_rows: gk.Row_Source,
	empty:     gk.View,
	loading:   gk.View,
	clicks:    int,
	events:    int,
	query:     string,
	matches:   [dynamic]int, // list-search: the rows the query matches
	sorted:    bool, // table: rows in descending order
	described: map[u64]int, // descriptions per row or cell key
	empties:   int, // descriptions of the empty view
	loadings:  int,
}

key :: proc(section, row: int) -> u64 {
	return u64(section) << 32 | u64(u32(row))
}

// --- lists ----------------------------------------------------------------------------------

list_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	a.described[key(at.section, at.row)] += 1
	text: string
	switch {
	case at.row == gk.HEADER:
		text = fmt.tprintf("section %d", at.section)
		fmt.printf("rows header section=%d\n", at.section)
	case at.row == gk.FOOTER:
		text = fmt.tprintf("end of %d", at.section)
		fmt.printf("rows footer section=%d\n", at.section)
	case a.mode == "list-search" && at.row < len(a.matches):
		text = fmt.tprintf("item %d", a.matches[at.row])
	case:
		text = fmt.tprintf("item %d.%d", at.section, at.row)
	}
	return gk.list_item(gk.id("item", at.row))->child(gk.label(text))
}

list_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Delegate_Event) {
	a.events += 1
	ix, has := e.index.?
	fmt.printf("rows list %v has=%v section=%d row=%d secondary=%v id=%d query=%q\n", e.kind, has, ix.section, ix.row, e.secondary, e.id, e.query)
	if e.kind == .Confirm {
		ix, ok := gk.selected_index(a.list, cx).?
		fmt.printf("rows list-state read selected_index ok=%v row=%d\n", ok, ix.row)
	}
	if e.kind == .Search {
		// Filter, set the count, then answer: the list's search task completes after it.
		a.query = strings.clone(string(e.query))
		clear(&a.matches)
		for i in 0 ..< ROWS {
			if strings.contains(fmt.tprintf("item %d", i), a.query) {
				append(&a.matches, i)
			}
		}
		gk.set_count(cx, a.src, len(a.matches))
		gk.invalidate(cx, a.src, 0, len(a.matches))
		gk.answer(cx, a.src, e.id, true)
		fmt.printf("rows search answered query=%q matches=%d\n", a.query, len(a.matches))
	}
}

// The list state's own events (the generated subscription).
list_state_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Event) {
	fmt.printf("rows list-state %v\n", e)
}

// list-states: a button walks the states: empty (count 0), loading, more rows with has_more.
list_step :: proc(a: ^App, cx: ^gk.Cx) {
	a.clicks += 1
	if a.mode == "table" {
		gk.scroll_to_row(a.table, cx, 50_000) // a far jump through the generated write
		fmt.printf("rows table-state scroll_to_row 50000\n")
		return
	}
	d := gk.default_delegate_data()
	d.empty = a.empty
	d.loading_view = a.loading
	switch a.clicks {
	case 1:
		gk.set_count(cx, a.src, 0)
		gk.delegate_data(cx, a.list, d)
	case 2:
		d.loading = true
		gk.set_count(cx, a.src, 50)
		gk.delegate_data(cx, a.list, d)
	case 3:
		d.has_more = true
		d.threshold = 10
		gk.delegate_data(cx, a.list, d)
	}
	fmt.printf("rows step=%d\n", a.clicks)
}

empty_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.empties += 1
	fmt.printf("rows empty view described n=%d\n", a.empties)
	return gk.label("nothing here")
}

initial_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	fmt.printf("rows initial view described\n")
	return gk.label("type to search")
}

loading_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.loadings += 1
	fmt.printf("rows loading view described n=%d\n", a.loadings)
	return gk.label("loading")
}

// --- tables -----------------------------------------------------------------------------------

COLUMNS := [?]gk.Table_Column {
	{key = "id", name = "Id", width = 80, sort = gk.Column_Sort.Default, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
	{key = "name", name = "Name", width = 120, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
	{key = "a", name = "A", width = 60, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
	{key = "b", name = "B", width = 60, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
	{key = "c", name = "C", width = 60, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
}

cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	a.described[key(at.col, at.row)] += 1
	row := a.sorted ? TABLE_ROWS - 1 - at.row : at.row
	return gk.label(fmt.tprintf("%d:%d", row, at.col))
}

table_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Table_Delegate_Event) {
	a.events += 1
	fmt.printf("rows table %v row=%d col=%d to=%d sort=%v\n", e.kind, e.row, e.col, e.to, e.sort)
	if e.kind == .Sort {
		a.sorted = e.sort == .Descending
		gk.table_invalidate(cx, a.src, 0, TABLE_ROWS)
		row, ok := gk.selected_row(a.table, cx).?
		fmt.printf("rows table-state read selected_row ok=%v row=%d\n", ok, row)
	}
}

// The table state's own events (the generated subscription): selection, double clicks, widths.
table_state_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Table_Event) {
	a.events += 1
	#partial switch v in e {
	case gk.Table_Event_Select_Row:
		fmt.printf("rows table Select_Row row=%d\n", v.v0)
	case gk.Table_Event_Double_Clicked_Row:
		fmt.printf("rows table Double_Clicked_Row row=%d\n", v.v0)
	case gk.Table_Event_Column_Widths_Changed:
		fmt.printf("rows table Column_Widths widths=%v\n", v.v0)
	case gk.Table_Event_Move_Column:
		fmt.printf("rows table state Move_Column from=%d to=%d\n", v.v0, v.v1)
	case:
		fmt.printf("rows table state %v\n", e)
	}
}

// --- trees ------------------------------------------------------------------------------------

TREE := [?]gk.Tree_Node {
	{id = "src", label = "src", depth = 0, expanded = true},
	{id = "src/a", label = "a.odin", depth = 1},
	{id = "src/b", label = "b.odin", depth = 1},
	{id = "docs", label = "docs", depth = 0},
	{id = "docs/c", label = "c.md", depth = 1},
	{id = "d", label = "README", depth = 0},
}

tree_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	a.described[key(0, at.row)] += 1
	e, ok := gk.tree_entry(cx, a.tree, at.row)
	if ok {
		fmt.printf("rows tree row=%d id=%s depth=%d folder=%v expanded=%v\n", at.row, e.id, e.depth, e.folder, e.expanded)
	}
	text := ""
	if ok {
		b := strings.builder_make(context.temp_allocator)
		for _ in 0 ..< e.depth {
			strings.write_string(&b, "  ")
		}
		strings.write_string(&b, string(e.label))
		text = strings.to_string(b)
	}
	return gk.list_item(gk.id("tree", at.row))->child(gk.label(text))
}

tree_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Tree_Event) {
	ix, ok := gk.selected_index(a.tree, cx).?
	fmt.printf("rows tree-state %v selected ok=%v ix=%d\n", e, ok, ix)
}

// --- released ---------------------------------------------------------------------------------

release_source :: proc(a: ^App, cx: ^gk.Cx) {
	gk.release(cx, a.src)
	fmt.printf("rows released source\n")
}

// --- the views --------------------------------------------------------------------------------

init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "list", "list-sections", "released":
		a.src = gk.list_source(cx, a, ROWS, list_row, gk.on(list_event), initial = 20)
		a.list = gk.list_state(cx, a.src)->build()
		a.sub = gk.on_list_event(a.list, cx, gk.on(list_state_event))
		d := gk.default_delegate_data()
		d.placeholder_height = 32
		if a.mode == "list-sections" {
			gk.set_count(cx, a.src, 5, 0)
			gk.set_count(cx, a.src, 3, 1)
			d.headers, d.footers = true, true
		}
		gk.delegate_data(cx, a.list, d)
	case "list-search":
		for i in 0 ..< ROWS {
			append(&a.matches, i)
		}
		a.src = gk.list_source(cx, a, ROWS, list_row, gk.on(list_event), initial = 20)
		a.list = gk.list_state(cx, a.src)->searchable(true)->build()
		// The view a searchable list shows while its query is empty.
		a.empty = gk.view(a, nil, initial_view)
		d := gk.default_delegate_data()
		d.initial = a.empty
		gk.delegate_data(cx, a.list, d)
	case "list-states":
		a.empty = gk.view(a, nil, empty_view)
		a.loading = gk.view(a, nil, loading_view)
		a.src = gk.list_source(cx, a, 30, list_row, gk.on(list_event), initial = 20)
		a.list = gk.list_state(cx, a.src)->build()
	case "table", "table-scroll":
		a.src = gk.table_source(cx, a, TABLE_ROWS, COLUMNS[:], cell, gk.on(table_event), initial_rows = 12)
		a.table = gk.table_state(cx, a.src)->build()
		a.sub = gk.on_table_event(a.table, cx, gk.on(table_state_event))
		d := gk.default_delegate_data()
		d.visible_events = true
		gk.delegate_data(cx, a.table, d)
	case "tree":
		a.tree = gk.tree_state(cx)->build()
		gk.tree_set_items(cx, a.tree, TREE[:])
		a.sub = gk.on_tree_event(a.tree, cx, gk.on(tree_event))
		a.tree_rows = gk.tree_source(cx, a, tree_row, initial = 8)
	}
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	body: gk.Element
	switch a.mode {
	case "table", "table-scroll":
		body = gk.div()->w(gk.px(440))->h(gk.px(240))->child(gk.data_table(a.table))
	case "tree":
		body = gk.div()->w(gk.px(300))->h(gk.px(240))->child(gk.tree(a.tree, a.tree_rows, .Size_Full))
	case:
		body = gk.div()->w(gk.px(300))->h(gk.px(240))->child(gk.list(a.list, .Size_Full))
	}
	return gk.v_flex(.Size_Full, .P_4, .Gap_2)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.h_flex(.Gap_2)->
			child(gk.button("step")->label("step")->on_click(gk.on(list_step)))->
			child(gk.button("release")->label("release")->on_click(gk.on(release_source))))->
		child(body)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, root_view)
	gk.open_window(cx, "rows", {480, 320}, a.root)
}

// Positions: the step button at (36, 30), the release button at (100, 30); the list, table or
// tree from (16, 56), 240 high.
STEP :: "click 36 30\n"
RELEASE :: "click 100 30\n"
ROW0 :: "click 60 74\n"

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	s := strings.builder_make()
	strings.write_string(&s, "size 480 320\nframe\nstat\n")
	switch app.mode {
	case "list":
		// a click on row 0 (select, confirm), down, a right click, escape, a far scroll (one
		// placeholder frame), back to the top (rows kept, nothing described again).
		strings.write_string(&s, ROW0)
		strings.write_string(&s, "key down\nrclick 60 140\nkey escape\nscroll 160 200 -200000\nframe\nstat\nscroll 160 200 400000\nframe\n")
	case "list-search":
		// The search input is the list's first row: click it, type, the request is answered.
		strings.write_string(&s, "click 120 72\ntype 77\nsleep 200\nframe\nsleep 200\nframe\nframe\n")
	case "list-sections":
		strings.write_string(&s, "scroll 160 200 -400\nframe\n")
	case "list-states":
		strings.write_string(&s, STEP)
		strings.write_string(&s, "frame\n")
		strings.write_string(&s, STEP)
		strings.write_string(&s, "frame\n")
		strings.write_string(&s, STEP)
		strings.write_string(&s, "frame\nscroll 160 200 -2000\nframe\nframe\n")
	case "table":
		// a click on row 0, down, End (a far jump: one placeholder frame), a sort, a double
		// click, a column moved and one resized, Home.
		strings.write_string(&s, "click 120 110\nkey down\nkey end\nframe\nstat\n")
		strings.write_string(&s, STEP)
		strings.write_string(&s, "frame\nclick 80 74\nframe\ndblclick 120 110\ndrag 160 74 330 74\ndrag 336 74 376 74\nkey home\nframe\n")
	case "table-scroll":
		// A steady scroll of 24 px before every frame, as the frame-cost scene does: rows are
		// bound ahead in batches, so no frame draws a placeholder.
		for _ in 0 ..< 120 {
			strings.write_string(&s, "scroll 220 200 -24\n")
		}
	case "tree":
		strings.write_string(&s, "click 60 152\nframe\nkey down\nframe\n")
	case "released":
		strings.write_string(&s, RELEASE)
		strings.write_string(&s, "frame\n")
	}
	strings.write_string(&s, "frame\nstat\n")
	gk.run_headless(&app, strings.to_string(s), start)
	again := 0
	for _, n in app.described {
		if n > 1 {
			again += 1
		}
	}
	fmt.printf("rows done mode=%s events=%d described=%d again=%d\n", app.mode, app.events, len(app.described), again)
}
