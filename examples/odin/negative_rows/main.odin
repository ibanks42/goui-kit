// negative_rows: mistakes against the delegate protocol (row sources, the kit's table) and
// InputState that must behave correctly or stop with a message, never show wrong content or
// crash (the second review of 11 November 2026). Run offscreen with the diag library by
// gates/misuse.sh, one mode per case:
//
//   GK_NEG=table-freed      the row source a table draws (its delegate's data) is released
//                           while the table is shown: must stop (exit 2) when it is drawn
//   GK_NEG=row-reenter      a cell procedure releases its own row source while the library
//                           asks it for the cell: must stop, naming the line
//   GK_NEG=row-handler      a row's click handler drops its own row (set_count 0) and then
//                           reads the value it was given: correct (exit 0), the value intact
//   GK_NEG=huge-count       a table of 3,000,000,000 rows of 2 columns: the cells' keys do not
//                           fit the row source; must stop at table_source rather than show
//                           the cells of another row
//   GK_NEG=nul              text with a NUL byte through InputState: arrives byte for byte
//   GK_NEG=utf8             text that is not UTF-8 through InputState: must stop
//   GK_NEG=input-release    a change handler of an input releases that input while the view
//                           still shows it: must stop when the view is drawn
package negative_rows

import "core:fmt"
import "core:os"
import "core:strings"
import gk "gpuikit:gk"

App :: struct {
	mode:      string,
	root:      gk.View,
	src:       gk.Row_Source,
	table:     gk.Table_State,
	list:      gk.List_State,
	name:      gk.Input_State,
	sub:       gk.Subscription,
	clicks:    int,
	max_row:   int, // the highest row a cell procedure was asked for
	described: int,
	got:       string,
}

COLUMNS := [?]gk.Table_Column {
	{key = "a", name = "A", width = 100, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
	{key = "b", name = "B", width = 100, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
}

cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	a.described += 1
	a.max_row = max(a.max_row, at.row)
	if a.mode == "row-reenter" && a.described == 3 {
		gk.release(cx, a.src) // re-enters the runtime from a cell procedure: must stop here
		fmt.println("gk-neg row-reenter went on after releasing its own source")
	}
	return gk.label(fmt.tprintf("%d:%d", at.row, at.col))
}

row_clicked :: proc(a: ^App, cx: ^gk.Cx, e: gk.Click_Event, text: string) {
	gk.set_count(cx, a.src, 0) // drops this row's view while its handler runs
	a.got = strings.clone(text) // the handler's value must still be intact
	fmt.printf("gk-neg row-handler clicked text=%q\n", a.got)
}

list_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	text := fmt.tprintf("row %d of a list", at.row)
	return gk.list_item(gk.id("r", at.row))->child(gk.label(text))->on_click(gk.on(row_clicked, text))
}

act :: proc(a: ^App, cx: ^gk.Cx) {
	a.clicks += 1
	switch a.mode {
	case "table-freed":
		gk.release(cx, a.src)
		fmt.println("gk-neg table-freed released the source")
	case "huge-count":
		a.table->scroll_to_row(cx, 2_147_483_700)
	case "nul":
		in_ := "a\x00b"
		a.name->set_value(cx, in_)
		v := a.name->value(cx)
		fmt.printf("gk-neg nul len=%d same=%v\n", len(v), string(v) == in_)
	case "utf8":
		bad := [?]u8{'o', 'k', 0xff, 'x'}
		a.name->set_value(cx, string(bad[:]))
		v := a.name->value(cx)
		fmt.printf("gk-neg utf8 accepted value=%q len=%d\n", v, len(v))
	}
}

changed :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	if _, ok := e.(gk.Input_Event_Change); ok {
		v := gk.keep(a.name->value(cx))
		gk.release(cx, a.name) // its own input, which the view still shows
		fmt.printf("gk-neg input-release released in its handler value=%q\n", v)
	}
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "table-freed", "row-reenter":
		a.src = gk.table_source(cx, a, 1000, COLUMNS[:], cell, initial_rows = 8)
		a.table = gk.table_state(cx, a.src)->build()
	case "huge-count":
		a.src = gk.table_source(cx, a, 3_000_000_000, COLUMNS[:], cell, initial_rows = 8)
		a.table = gk.table_state(cx, a.src)->build()
	case "row-handler":
		a.src = gk.list_source(cx, a, 20, list_row, initial = 20)
		a.list = gk.list_state(cx, a.src)->build()
	case "nul", "utf8", "input-release":
		a.name = gk.input_state(cx)->build()
		if a.mode == "input-release" {
			a.sub = a.name->on_input_event(cx, gk.on(changed))
		}
	}
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	body: gk.Element
	switch a.mode {
	case "table-freed", "row-reenter", "huge-count":
		body = gk.div()->w(gk.px(300))->h(gk.px(200))->child(gk.data_table(a.table))
	case "row-handler":
		body = gk.div()->w(gk.px(300))->h(gk.px(200))->child(gk.list(a.list, .Size_Full))
	case:
		body = gk.div()->w(gk.px(240))->child(gk.input(a.name))
	}
	return gk.v_flex(.Size_Full, .P_4, .Gap_2)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.h_flex(.Gap_2)->child(gk.button("act")->label("act")->on_click(gk.on(act))))->
		child(body)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "negative_rows", {400, 300}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_NEG", context.allocator)
	// The act button at (36, 30); the table, list or input from (16, 56).
	script := "size 400 300\nframe\nclick 36 30\nframe\nframe\n"
	switch app.mode {
	case "row-handler":
		script = "size 400 300\nframe\nclick 60 74\nframe\nframe\n"
	case "input-release":
		script = "size 400 300\nframe\nclick 60 72\ntype x\nframe\nframe\n"
	}
	gk.run_headless(&app, script, start)
	fmt.printf("gk-neg done mode=%s clicks=%d described=%d max_row=%d got=%q\n", app.mode, app.clicks, app.described, app.max_row, app.got)
}
