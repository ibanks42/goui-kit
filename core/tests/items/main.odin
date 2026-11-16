// items: item procedures (docs/PROTOCOLS.md 9.41), one mode per kind of value the kit hands one, run
// offscreen (the diag library) under a gk-meter script; core/tests/run-items.sh checks the lines.
//
//   GK_PROTO=items-calendar    an element the kit built (a calendar's day) decorated by the app, with
//                              the record of its state; the rows are described once and kept
//   GK_PROTO=items-time-field  the same over a time field's segments (the segment's kind and value)
//   GK_PROTO=items-tree        values 9.16 reads (the entry's index as the row's key) beside records
//                              of a value's reads (Tree_Entry_Reads, Base_Tree_Entry_State)
//   GK_PROTO=items-popover     kit state handed by reference with its own context (a popover's
//                              content reads Popover_State_Reads)
//   GK_PROTO=items-command     kit state handed by reference with the app's context (a command
//                              palette's header and footer)
//   GK_PROTO=items-hover       a closure that returns one element type (a base hover card's content
//                              is a Stateful_Div), opened by the pointer
//   GK_PROTO=items-close       FnOnce(Button) -> E: the dialog's close button built from the app's ops
//   GK_PROTO=items-combobox    a generic context (ComboboxTriggerContext with the binding's delegate) as
//                              a record: the trigger reads the selection and the placeholder
//   GK_PROTO=items-nav         an element handed with the record of its own reads (a nav stack's page)
//   GK_PROTO=items-wrong-kind  a calendar's procedure given to a time field: stops where it is given
package items_test

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

App :: struct {
	mode:      string,
	root:      gk.View,
	cal:       gk.Calendar_State,
	tf:        gk.Time_Field_State,
	tree:      gk.Tree_State,
	cmd:       gk.Command_State,
	combo:     gk.Combobox_State,
	nav:       gk.Nav_Stack_State,
	page:      gk.View,
	builds:    int,
	described: int, // item rows described
	kinds:     [gk.Calendar_Item_Kind]int,
	segments:  [gk.Time_Segment]int,
	active:    int,
	after:     int, // rows described after the first three frames
	frames:    int,
}

// --- the procedures -------------------------------------------------------------------------------

day :: proc(a: ^App, cx: ^gk.Cx, item: gk.Calendar_Item, st: gk.Base_Calendar_Item_State) -> gk.Element {
	a.described += 1
	a.kinds[st.kind] += 1
	if st.is_active {
		a.active += 1
	}
	if a.frames >= 3 {
		a.after += 1
	}
	if st.kind != .Day {
		return item
	}
	return item->style(.Rounded_Full)->bg(gk.theme(.Primary) if st.is_today else gk.theme(.Background))
}

segment :: proc(a: ^App, cx: ^gk.Cx, seg: gk.Time_Field_Segment, st: gk.Time_Field_Segment_State) -> gk.Element {
	a.described += 1
	a.segments[st.segment] += 1
	fmt.printf("items segment=%v value=%d selected=%v\n", st.segment, st.value, st.is_selected)
	return gk.div(.Px_1)->bg(gk.theme(.Muted))->child(seg)
}

entry :: proc(a: ^App, cx: ^gk.Cx, ix: int, e: gk.Tree_Entry_Reads, st: gk.Base_Tree_Entry_State) -> gk.Element {
	a.described += 1
	fmt.printf("items tree row ix=%d label=%s depth=%d folder=%v selected=%v\n", ix, e.item.label, e.depth, e.is_folder,
		st.is_selected)
	return gk.div(.H_6, .Px_2)->child(gk.text(fmt.tprintf("%s (%d)", e.item.label, ix)))
}

pop_content :: proc(a: ^App, cx: ^gk.Cx, st: gk.Popover_State_Reads) -> gk.Element {
	a.described += 1
	fmt.printf("items popover content is_open=%v\n", st.is_open)
	return gk.div(.P_2)->child(gk.text("inside the popover"))
}

cmd_header :: proc(a: ^App, cx: ^gk.Cx, st: gk.Command_State_Reads) -> gk.Element {
	a.described += 1
	_, has := st.selected_index.?
	fmt.printf("items command header matched=%d loading=%v selected=%v\n", st.matched_count, st.is_loading, has)
	return gk.text(fmt.tprintf("%d matches", st.matched_count))
}

hover_content :: proc(a: ^App, cx: ^gk.Cx, st: gk.Hover_Card_State_Reads) -> gk.Stateful_Div {
	a.described += 1
	fmt.printf("items hover content is_open=%v\n", st.is_open)
	return gk.div(.P_2)->id("card")->child(gk.text("card"))
}

trigger :: proc(a: ^App, cx: ^gk.Cx, t: gk.Combobox_Trigger_Context) -> gk.Element {
	a.described += 1
	ph, _ := t.placeholder.?
	first := t.selection[0].v1.title if len(t.selection) > 0 else ""
	fmt.printf("items combobox trigger selected=%d first=%s placeholder=%s open=%v\n", len(t.selection), first, ph, t.is_open)
	return gk.div(.P_1)->child(gk.text(fmt.tprintf("%d chosen", len(t.selection))))
}

page :: proc(a: ^App, cx: ^gk.Cx, pg: gk.Nav_Page, r: gk.Nav_Page_Reads) -> gk.Element {
	a.described += 1
	fmt.printf("items nav page index=%d phase=%v progress=%.1f\n", r.index, r.phase, r.progress)
	return pg->opacity(r.progress)
}

page_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.text("the page")
}

FRUIT := [?]gk.Search_Item{{value = "apple", title = "Apple"}, {value = "pear", title = "Pear"}}

// --- the view -------------------------------------------------------------------------------------

init :: proc(a: ^App, cx: ^gk.Cx) {
	gk.on_frame(cx, on_frame)
	switch a.mode {
	case "items-calendar", "items-wrong-kind":
		a.cal = gk.calendar_state(cx)->build()
	case "items-time-field":
		a.tf = gk.time_field_state(cx)->build()
	case "items-tree":
		a.tree = gk.tree_state(cx)->items([]gk.Tree_Item{gk.tree_item("a", "Alpha"), gk.tree_item("b", "Beta")})->build()
	case "items-command":
		a.cmd = gk.command_state(cx)
	case "items-combobox":
		a.combo = gk.combobox_state(cx, gk.search_source(cx, a, FRUIT[:]), []int{1})->build() // Pear
	case "items-nav":
		a.nav = gk.nav_stack_state(cx)
		a.page = gk.view(a, nil, page_view)
		a.nav->push(cx, a.page, .Immediate)
	}
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.builds += 1
	root := gk.div(.Size_Full, .P_2)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))
	switch a.mode {
	case "items-calendar":
		return root->child(gk.base_calendar(gk.Id("cal"), a.cal)->item(gk.items(cx, a, day)))
	case "items-wrong-kind":
		return root->child(gk.base_calendar(gk.Id("cal"), a.cal)->item(gk.items(cx, a, segment)))
	case "items-time-field":
		return root->child(gk.base_time_field(gk.Id("tf"), a.tf)->render_segment(gk.items(cx, a, segment)))
	case "items-tree":
		tree := gk.base_tree(a.tree, .Size_Full)->list_style(gk.style_refinement_default(.Size_Full))
		return root->child(gk.div()->h(gk.px(200))->child(tree->item(gk.items(cx, a, entry))))
	case "items-popover":
		pop := gk.base_popover(gk.Id("pop"))->open(true)->trigger(gk.button("t")->label("Open"))
		return root->child(pop->content(gk.items(cx, a, pop_content)))
	case "items-command":
		return root->child(gk.command(a.cmd)->header(gk.items(cx, a, cmd_header))->footer(gk.items(cx, a, cmd_header)))
	case "items-hover":
		card := gk.base_hover_card(gk.Id("card"))->open_delay(0)->trigger(gk.div(.W_20, .H_10)->bg(gk.theme(.Primary)))
		return root->child(card->content(gk.items(cx, a, hover_content)))
	case "items-combobox":
		return root->child(gk.combobox(a.combo)->placeholder("Fruit")->render_trigger(gk.items(cx, a, trigger)))
	case "items-nav":
		return root->child(gk.nav_stack(a.nav, .Size_Full)->item(gk.items(cx, a, page)))
	case "items-close":
		close := gk.base_dialog_close()->trigger(gk.base_button(gk.Id("x"))->child(gk.text("Close")))
		return root->child(close)->child(gk.text("items close drawn"))
	}
	return root
}

on_frame :: proc(a: ^App, cx: ^gk.Cx) {
	a.frames += 1
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "items", {640, 480}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	script := "size 640 480\nframe\nframe\nframe\n"
	if app.mode == "items-hover" {
		script = fmt.aprint(script, "move 10 10\nadvance 100\nframe\nadvance 100\nframe\nframe\n", sep = "")
	}
	script = fmt.aprint(script, "frame\nframe\nframe\nstat\n", sep = "")
	gk.run_headless(&app, script, start)
	switch app.mode {
	case "items-calendar":
		fmt.printf("items calendar described=%d days=%d weekdays=%d active=%d after=%d\n", app.described,
			app.kinds[.Day], app.kinds[.Weekday], app.active, app.after)
	case "items-time-field":
		fmt.printf("items time field described=%d hours=%d minutes=%d\n", app.described, app.segments[.Hour],
			app.segments[.Minute])
	}
	fmt.printf("items done mode=%s described=%d builds=%d frames=%d\n", app.mode, app.described, app.builds, app.frames)
}
