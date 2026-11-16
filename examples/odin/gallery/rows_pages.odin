// The rows family's gallery pages, the Odin twin of ref/src/bin/gallery/rows_pages.rs, call
// for call: a list of 10,000 rows, a table of 10,000 rows with a sortable column and a tree,
// over row sources (docs/PROTOCOLS.md section 9.5). Each row or cell is a view described once;
// the steps of ref/gallery/pages.txt click, press keys and scroll (a far scroll and a jump to
// the last row draw placeholders for one frame, and the shot follows the next frame).
package gallery

import "core:fmt"
import "core:strings"
import gk "gpuikit:gk"

LIST_ROWS :: 10_000
TABLE_ROWS :: 10_000

Rows_Pages :: struct {
	list:      gk.List_State,
	table:     gk.Table_State,
	cells:     gk.Row_Source, // the table's
	tree:      gk.Tree_State,
	tree_rows: gk.Row_Source,
	desc:      bool, // the table's rows in descending order of their id
}

ROWS_PAGES := [?]Page{{"rows-list", rows_list}, {"rows-table", rows_table}, {"rows-tree", rows_tree}}

ROWS_TREE := [?]gk.Tree_Node {
	{id = "docs", label = "docs", depth = 0},
	{id = "docs/guide", label = "guide.md", depth = 1},
	{id = "docs/api", label = "api.md", depth = 1},
	{id = "src", label = "src", depth = 0, expanded = true},
	{id = "src/a", label = "a.odin", depth = 1},
	{id = "src/b", label = "b.odin", depth = 1},
	{id = "readme", label = "README", depth = 0},
}

rows_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	p := &a.rows_family
	p.list = gk.list_state(cx, gk.list_source(cx, a, LIST_ROWS, rows_list_row))->build()
	columns := [?]gk.Table_Column {
		gk.table_column("id", "Id", sort = gk.Column_Sort.Default),
		gk.table_column("a", "A"),
		gk.table_column("b", "B"),
		gk.table_column("c", "C"),
	}
	p.cells = gk.table_source(cx, a, TABLE_ROWS, columns[:], rows_cell, gk.on(rows_table_event))
	p.table = gk.table_state(cx, p.cells)->build()
	p.tree = gk.tree_state(cx)->build()
	gk.tree_set_items(cx, p.tree, ROWS_TREE[:])
	p.tree_rows = gk.tree_source(cx, a, rows_tree_row)
}

rows_list_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	return gk.list_item(gk.id("item", at.row))->child(gk.label(fmt.tprintf("row %d", at.row)))
}

rows_cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	row := a.rows_family.desc ? TABLE_ROWS - 1 - at.row : at.row
	return gk.label(fmt.tprintf("%d:%d", row, at.col))
}

rows_table_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Table_Delegate_Event) {
	if e.kind == .Sort {
		a.rows_family.desc = e.sort == .Descending
		gk.table_invalidate(cx, a.rows_family.cells, 0, TABLE_ROWS)
	}
}

rows_tree_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	text := ""
	if e, ok := gk.tree_entry(cx, a.rows_family.tree, at.row); ok {
		b := strings.builder_make(context.temp_allocator)
		for _ in 0 ..< e.depth {
			strings.write_string(&b, "  ")
		}
		strings.write_string(&b, string(e.label))
		text = strings.to_string(b)
	}
	return gk.list_item(gk.id("tree", at.row))->child(gk.label(text))
}

rows_list :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(300))->h(gk.px(240))->child(gk.list(a.rows_family.list)->scrollbar_visible(false))
}

rows_table :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(440))->h(gk.px(240))->child(gk.data_table(a.rows_family.table)->scrollbar_visible(false, false))
}

rows_tree :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(300))->h(gk.px(240))->child(gk.tree(a.rows_family.tree, a.rows_family.tree_rows))
}
