// table: the `table` scene in Odin, the same table as ref/src/bin/table.rs: the kit's
// DataTable with `rows` rows of `cols` columns over a row source whose rows are the cells
// (docs/PROTOCOLS.md section 9.5). Each cell is a view described once, when it is first
// drawn near the visible rows, and kept while the table scrolls; the frame-cost gate scrolls
// it before every frame with GK_SCROLL (gates/fps.sh table).
//
//   table-odin [rows] [cols]      (default 100000 6)
package table

import "core:fmt"
import "core:os"
import "core:strconv"
import gk "gpuikit:gk"

App :: struct {
	rows, cols: int,
	src:        gk.Row_Source,
	table:      gk.Table_State,
	root:       gk.View,
}

cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	return gk.label(fmt.tprintf("%d:%d", at.row, at.col))
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	columns := make([]gk.Table_Column, a.cols, context.temp_allocator)
	for &c, i in columns {
		c = gk.table_column(fmt.tprintf("c%d", i), fmt.tprintf("Column %d", i))
	}
	if _, trace := os.lookup_env("TABLE_TRACE", context.temp_allocator); trace {
		// Diagnostics: print the visible rows on each change (an event per change).
		a.src = gk.table_source(cx, a, a.rows, columns, cell, gk.on(visible))
		d := gk.default_delegate_data()
		d.visible_events = true
		a.table = gk.table_state(cx, a.src)->build()
		gk.delegate_data(cx, a.table, d)
		return
	}
	a.src = gk.table_source(cx, a, a.rows, columns, cell)
	a.table = gk.table_state(cx, a.src)->build()
}

visible :: proc(a: ^App, cx: ^gk.Cx, e: gk.Table_Delegate_Event) {
	if e.kind == .Visible_Rows {
		fmt.printf("table visible %d ..< %d\n", e.row, e.to)
	}
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.data_table(a.table))
}

arg :: proc(i, def: int) -> int {
	if len(os.args) > i {
		if v, ok := strconv.parse_int(os.args[i]); ok {
			return v
		}
	}
	return def
}

main :: proc() {
	app := App{rows = arg(1, 100_000), cols = arg(2, 6)}
	gk.run(&app, proc(a: ^App, cx: ^gk.Cx) {
		a.root = gk.view(a, init, root_view)
		gk.open_window(cx, "Table", {900, 600}, a.root)
	})
}
