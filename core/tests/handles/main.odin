// handles: the protocol tests of library objects behind handles (docs/PROTOCOLS.md section 13):
// kit values that are not entities, held by the library and named by a handle of kind OBJ.
// Each mode runs offscreen (the diag library) under a gk-meter script and prints lines that
// core/tests/run-handles.sh checks, with the exit code.
//
//   GK_PROTO=obj-objects | obj-temps | obj-stale-temp | obj-global | obj-dialog | obj-list |
//            obj-given-away | obj-values | obj-stale-value | obj-columns
package handles_test

import "core:fmt"
import "core:os"
import "core:time"
import gk "gpuikit:gk"

ROWS :: 200

App :: struct {
	mode:    string,
	root:    gk.View,
	clicks:  int,
	// temps
	input:   gk.Input_State,
	temp:    gk.Input_Presentation,
	kept:    gk.Input_Presentation,
	// list
	src:     gk.Row_Source,
	list:    gk.List_State,
	scroll:  gk.Virtual_List_Scroll_Handle,
	events:  int,
	// dialog
	dialog:  gk.Dialog_Handle,
	// values
	tree:    gk.Tree_State,
	grid:    gk.Grid_Layout_State,
	kept_v:  gk.Tree_Item,
	// columns
	tsrc:    gk.Row_Source,
	table:   gk.Table_State,
}

counts :: proc(cx: ^gk.Cx, what: string) {
	temps, live := gk.test_obj_counts(cx)
	fmt.printf("handles counts %s temps=%d live=%d\n", what, temps, live)
}

// Owned objects: made by a constructor (kept), read at once, written (buffered: the next read
// sees it), a call that returns a value; released.
objects :: proc(a: ^App, cx: ^gk.Cx) {
	counts(cx, "before")
	d := gk.dock(cx, 240)
	fmt.printf("handles dock open=%v size=%.0f\n", gk.is_open(d, cx), gk.size(d, cx))
	gk.set_open(d, cx, false)
	gk.set_size(d, cx, 300)
	gk.set_resizing(d, cx, true)
	fmt.printf("handles dock written open=%v size=%.0f resizing=%v\n", gk.is_open(d, cx), gk.size(d, cx), gk.is_resizing(d, cx))
	p := gk.diff_line_position(cx, "src/a.rs", .Modified, 7)
	fmt.printf("handles position path=%s side=%v line=%d\n", gk.path(p, cx), gk.side(p, cx), gk.line(p, cx))
	m := gk.search_matcher(cx)
	gk.update_query(m, cx, "needle", true)
	fmt.printf("handles matcher len=%d empty=%v label=%q\n", gk.len_(m, cx), gk.is_empty(m, cx), gk.search_matcher_label(m, cx))
	counts(cx, "made")
	gk.release(cx, d)
	gk.release(cx, p)
	gk.release(cx, m)
}

// A read that hands out an object gives a temporary handle: usable in its turn, swept when
// the next frame begins; gk.retain keeps one.
temps :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.clicks {
	case 1:
		a.input = gk.input_state(cx)->placeholder("type here")->build()
		counts(cx, "before")
		a.temp = gk.presentation(a.input, cx)
		a.kept = gk.retain(cx, gk.presentation(a.input, cx))
		fmt.printf("handles temp placeholder=%q editable=%v multi_line=%v\n", gk.placeholder(a.temp, cx), gk.is_editable(a.temp, cx), gk.is_multi_line(a.temp, cx))
		counts(cx, "handed out")
	case 2:
		counts(cx, "next frame")
		fmt.printf("handles kept placeholder=%q\n", gk.placeholder(a.kept, cx))
		gk.release(cx, a.kept)
		if a.mode == "obj-stale-temp" {
			fmt.println("handles reading the swept temporary")
			fmt.println(gk.placeholder(a.temp, cx)) // stops: it was temporary
		}
	}
}

// The kit's theme global: one handle, reads in place; a read in a view procedure is allowed.
global :: proc(a: ^App, cx: ^gk.Cx) {
	th := gk.theme_global(cx)
	again := gk.theme_global_mut(cx)
	fmt.printf("handles theme same=%v dark=%v name=%q radius_full=%.0f\n", th.h == again.h, gk.is_dark(th, cx), gk.theme_name(th, cx), gk.radius_full(th, cx))
	reg := gk.theme_registry_global(cx)
	fmt.printf("handles registry ok=%v\n", reg.h != 0)
	counts(cx, "globals")
}

// A dialog's handle opens and closes it from code (writes with the window).
dialog :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.clicks {
	case 1:
		a.dialog = gk.dialog_handle(cx, false)
		fmt.printf("handles dialog open=%v\n", gk.is_open(a.dialog, cx))
		gk.open(a.dialog, cx)
	case 2:
		fmt.printf("handles dialog open=%v\n", gk.is_open(a.dialog, cx))
		gk.close(a.dialog, cx)
		fmt.printf("handles dialog closed open=%v\n", gk.is_open(a.dialog, cx))
	}
}

list_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	return gk.list_item(gk.id("row", at.row))->child(gk.text(fmt.tprintf("row %d", at.row)))
}

list_event :: proc(a: ^App, cx: ^gk.Cx) {
	a.events += 1
}

// A list selected and scrolled from code: an IndexPath travels as its record, and the list's
// scroll handle is a library object.
list :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.clicks {
	case 1:
		// a row source needs a window: made in a handler
		a.src = gk.list_source(cx, a, ROWS, list_row, gk.on(list_event), initial = 20)
		a.list = gk.list_state(cx, a.src)->build()
	case 2:
		gk.set_selected_index(a.list, cx, 120)
		gk.scroll_to_item(a.list, cx, 120, .Top)
		a.scroll = gk.retain(cx, gk.list_state_scroll_handle(a.list, cx))
	case 3:
		sel, ok := gk.selected_index(a.list, cx).?
		fmt.printf("handles list selected ok=%v row=%d\n", ok, sel.row)
		gk.scroll_to_item(a.scroll, cx, 3, .Top)
		gk.set_selected_index(a.list, cx, nil)
	case 4:
		_, ok := gk.selected_index(a.list, cx).?
		fmt.printf("handles list cleared selected=%v\n", ok)
	}
}

// An object a method takes by value and cannot copy is given away: its handle is released.
given_away :: proc(a: ^App, cx: ^gk.Cx) {
	t := gk.tooltip_defaults(cx)->with_show_delay(250 * time.Millisecond)->build()
	gk.install(t, cx) // TooltipDefaults::install(self, cx): copied (it is Clone), the handle stays
	gk.install(t, cx)
	counts(cx, "installed twice")
	gk.release(cx, t)
}

// Builder values made in a handler, outside any view, given to call ops: each travels as a
// tape of its own and is decoded by the value's own decoder (a tree's items, a grid's areas).
values :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.clicks {
	case 1:
		a.tree = gk.tree_state(cx)->build()
		src := gk.tree_item("src", "src")->expanded(true)->child(gk.tree_item("src/a.rs", "a.rs"))->child(gk.tree_item("src/b.rs", "b.rs"))
		gk.set_items(a.tree, cx, {src, gk.tree_item("README", "README")})
		a.grid = gk.grid_layout_state(cx)->build()
		gk.set_areas(a.grid, cx, {gk.grid_area("one")->col(1)->row(2)->col_span(3), gk.grid_area("two")})
		a.kept_v = gk.tree_item("late", "late")
	case 2:
		n := 0
		for i in 0 ..< 8 {
			e, ok := gk.entry(a.tree, cx, i).?
			if !ok {break}
			it := gk.item(e, cx)
			fmt.printf("handles tree entry %d id=%s depth=%d folder=%v\n", i, it.id, gk.depth(e, cx), gk.is_folder(e, cx))
			n += 1
		}
		fmt.printf("handles tree entries=%d\n", n)
		areas := gk.areas(a.grid, cx)
		for ar in areas {
			fmt.printf("handles grid area id=%s col=%d row=%d col_span=%d\n", ar.id, ar.col, ar.row, ar.col_span)
		}
		if a.mode == "obj-stale-value" {
			fmt.println("handles giving a value of an earlier turn")
			gk.set_items(a.tree, cx, {a.kept_v}) // stops: a value lives for the turn that made it
		}
	}
}

cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	return gk.text(fmt.tprintf("%d.%d", at.row, at.col))
}

// A table's columns as the kit's own Column values (gk.column(..)->..) instead of gk.Table_Column.
columns :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.clicks {
	case 1:
		a.tsrc = gk.table_source(cx, a, 40, nil, cell)
		gk.table_columns(cx, a.tsrc, {gk.column("name", "Name")->width(140)->sortable(), gk.column("size", "Size")->width(60)->text_right()})
		a.table = gk.table_state(cx, a.tsrc)->build()
	case 2:
		hs := gk.headers(a.table, cx)
		w0, _ := gk.col_width(a.table, cx, 0).?
		fmt.printf("handles columns n=%d first=%s second=%s width0=%.0f\n", len(hs), hs[0] if len(hs) > 0 else "", hs[1] if len(hs) > 1 else "", w0)
		gk.table_columns(cx, a.tsrc, {gk.column("name", "Name"), gk.column("size", "Size"), gk.column("kind", "Kind")}, a.table)
	case 3:
		fmt.printf("handles columns replaced n=%d\n", len(gk.headers(a.table, cx)))
	}
}

clicked :: proc(a: ^App, cx: ^gk.Cx) {
	a.clicks += 1
	switch a.mode {
	case "obj-objects":
		if a.clicks == 1 {objects(a, cx)}
	case "obj-temps", "obj-stale-temp":
		temps(a, cx)
	case "obj-global":
		if a.clicks == 1 {global(a, cx)}
	case "obj-dialog":
		dialog(a, cx)
	case "obj-list":
		list(a, cx)
	case "obj-values", "obj-stale-value":
		values(a, cx)
	case "obj-columns":
		columns(a, cx)
	case "obj-given-away":
		if a.clicks == 1 {given_away(a, cx)}
	}
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	body := gk.div(.Size_Full)->id("all")->on_click(gk.on(clicked))
	if a.mode == "obj-global" {
		// a read of a global in a view procedure: a read, allowed
		body = body->child(gk.text(fmt.tprintf("dark %v", gk.is_dark(gk.theme_global(cx), cx))))
	}
	if a.mode == "obj-columns" && a.table.h != 0 {
		body = body->child(gk.div()->w(gk.px(300))->h(gk.px(240))->child(gk.data_table(a.table)))
	}
	if a.mode == "obj-list" && a.list.h != 0 {
		body = body->child(gk.div()->w(gk.px(300))->h(gk.px(240))->child(gk.list(a.list, .Size_Full)))
	}
	return body
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, nil, root_view)
	gk.open_window(cx, "handles", {640, 480}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	script := "size 640 480\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nstat\n"
	gk.run_headless(&app, script, start)
	fmt.printf("handles done mode=%s clicks=%d\n", app.mode, app.clicks)
}
