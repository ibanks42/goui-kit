// overlay: the protocol tests of the overlays family (docs/PROTOCOLS.md section 11.6): a dialog, an
// alert and a sheet opened from handlers with view-built procedures (gk.built), described again
// while open, closed by the keyboard and by the app, opened again on the same view; a
// questionnaire (a state whose constructor and updates can fail). Each mode runs offscreen (the
// diag library) under a gk-meter script and prints lines core/tests/run-overlay.sh checks.
//
//   GK_PROTO=dialog | alert | sheet | questionnaire | release | wrong-root | questionnaire-dup
//   (and the further modes below: dock, ..., popover, observe, values)
package overlay_test

import "core:fmt"
import "core:os"
import "core:time"
import gk "gpuikit:gk"

App :: struct {
	mode:      string,
	root:      gk.View,
	count:     int, // shown in the dialog; bumped while it is open
	described: int, // runs of the dialog's procedure
	cancels:   int,
	oks:       int,
	closes:    int,
	first:     gk.Built(gk.Dialog),
	sheet:     gk.Built(gk.Sheet),
	q:         gk.Questionnaire_State,
	// the dock
	dock:      gk.Dock_Area,
	panels:    [2]gk.Dock_Panel,
	bodies:    [2]Body,
	events:    [dynamic]string,
	// ids, saved layouts, the nav stack
	node:      gk.Node_Id,
	saved:     string, // the first dump, printed
	builds:    int,
	nav:       gk.Nav_Stack_State,
	pages:     [2]Body,
	page_views: [2]gk.View,
	bar:       gk.App_Menu_Bar,
	// a renderer, the toast manager
	renderer:  gk.Dock_Renderer,
	asks:      [gk.Dock_Hook]int,
	chrome:    Chrome,
	chrome_v:  gk.View,
	tabbar_v:  gk.View,
	frame_v:   gk.View,
	dock_ctx:  gk.Dock_Context,
	tabbars:   int,
	frames:    int,
	toasts:    gk.Toast_Manager,
	popover_open: bool,
	popover_changes: int,
	toggles:   int,
	toggle_sub: gk.Subscription,
	// views family: observations, value calls
	obs_x:     f32,
	positions: int,
	last_pos:  gk.Resolved_Position,
	pops:      int,
	last_pop:  gk.Event_117,
	frames5:   int,
}

Chrome :: struct {
	placement: gk.Dock_Placement,
	open:      bool,
	described: int,
}

Body :: struct {
	text:      string,
	described: int,
}

// --- the dialog --------------------------------------------------------------------------------

cancelled :: proc(a: ^App, cx: ^gk.Cx, seen: int) -> bool {
	a.cancels += 1
	fmt.printf("overlay dialog cancel n=%d seen=%d count=%d\n", a.cancels, seen, a.count)
	return true // closes it
}

confirm_dialog :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	a.described += 1
	fmt.printf("overlay dialog described n=%d count=%d\n", a.described, a.count)
	return gk.dialog()->
		title(gk.label("Delete?"))->
		child(gk.label(fmt.tprintf("count %d", a.count)))->
		on_cancel(gk.on(cancelled, a.count)) // the count of this description: which tape the kit used
}

open_first :: proc(a: ^App, cx: ^gk.Cx) {
	a.first = gk.built(cx, a, confirm_dialog)
	gk.open_dialog(cx, a.first)
	fmt.println("overlay dialog opened")
}

bump :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay dialog active=%v\n", gk.has_active_dialog(cx))
	a.count += 1
	gk.notify(cx, a.first.view) // described again while open
}

reopen :: proc(a: ^App, cx: ^gk.Cx) {
	again := gk.built(cx, a, confirm_dialog)
	fmt.printf("overlay dialog closed active=%v; again same=%v\n", gk.has_active_dialog(cx), again.h == a.first.h)
	gk.open_dialog(cx, again)
}

close_by_app :: proc(a: ^App, cx: ^gk.Cx) {
	gk.close_dialog(cx)
}

dialog_done :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay done mode=dialog active=%v described=%d cancels=%d count=%d\n",
		gk.has_active_dialog(cx), a.described, a.cancels, a.count)
}

// --- the alert ---------------------------------------------------------------------------------

alert_ok :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	a.oks += 1
	fmt.printf("overlay alert ok n=%d\n", a.oks)
	return true
}

alert :: proc(a: ^App, cx: ^gk.Cx) -> gk.Alert_Dialog {
	return gk.alert_dialog()->title(gk.label("Unsaved"))->description(gk.label("Leave?"))->show_cancel(true)->
		on_ok(gk.on(alert_ok))
}

open_alert :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_alert_dialog(cx, gk.built(cx, a, alert))
	fmt.println("overlay alert opened")
}

alert_done :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay done mode=alert active=%v oks=%d\n", gk.has_active_dialog(cx), a.oks)
}

// --- the sheet ---------------------------------------------------------------------------------

sheet_closed :: proc(a: ^App, cx: ^gk.Cx) {
	a.closes += 1
	fmt.printf("overlay sheet on_close n=%d\n", a.closes)
}

settings :: proc(a: ^App, cx: ^gk.Cx) -> gk.Sheet {
	fmt.println("overlay sheet described")
	return gk.sheet()->title(gk.label("Settings"))->child(gk.label("body"))->on_close(gk.on(sheet_closed))
}

open_sheet :: proc(a: ^App, cx: ^gk.Cx) {
	a.sheet = gk.built(cx, a, settings)
	gk.open_sheet_at(cx, .Left, a.sheet)
}

sheet_check :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay sheet active=%v\n", gk.has_active_sheet(cx))
	gk.close_sheet(cx)
}

sheet_done :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay done mode=sheet active=%v\n", gk.has_active_sheet(cx))
}

// --- misuse: a released view, a procedure that returns another node ----------------------------

release_first :: proc(a: ^App, cx: ^gk.Cx) {
	gk.release(cx, a.first.view) // the dialog is still open: its next build stops
	fmt.println("overlay released the open dialog's view")
	gk.open_sheet(cx, gk.built(cx, a, settings)) // the window's layers are built again
}

wrong :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	return gk.Dialog{el = gk.div()->child(gk.label("not a dialog")).el} // compiles; the library refuses it
}

open_wrong :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_dialog(cx, gk.built(cx, a, wrong))
}

// --- the questionnaire ---------------------------------------------------------------------------

items :: proc(dup: bool) -> []gk.Questionnaire_Item_Definition {
	color := gk.questionnaire_item_definition("color", "Favourite colour")->
		with_choice(gk.questionnaire_choice_definition("red", "Red"))->
		with_choice(gk.questionnaire_choice_definition("blue", "Blue"))
	size := gk.questionnaire_item_definition(dup ? "color" : "size", "Size")->
		with_choice(gk.questionnaire_choice_definition("s", "Small"))->
		with_choice(gk.questionnaire_choice_definition("l", "Large"))
	out := make([]gk.Questionnaire_Item_Definition, 2, context.temp_allocator)
	out[0], out[1] = color, size
	return out
}

questionnaire :: proc(a: ^App, cx: ^gk.Cx) {
	a.q = gk.questionnaire_state(cx, items(a.mode == "questionnaire-dup"))->build()
	cur, _ := a.q->current_item(cx).?
	fmt.printf("overlay q made total=%d current=%s\n", a.q->total(cx), cur)
	err, failed := a.q->choose(cx, "color", "green").?
	fmt.printf("overlay q choose unknown failed=%v err=%s\n", failed, err)
	_, failed2 := a.q->choose(cx, "color", "blue").?
	fmt.printf("overlay q choose blue failed=%v\n", failed2)
	moved := a.q->go_next(cx)
	cur2, _ := a.q->current_item(cx).?
	fmt.printf("overlay q next moved=%v current=%s\n", moved, cur2)
	_, failed3 := a.q->set_current_item(cx, "nope").?
	fmt.printf("overlay q set_current_item unknown failed=%v\n", failed3)
	fmt.printf("overlay done mode=questionnaire complete=%v\n", a.q->is_complete(cx))
}

// --- the dock: two Odin views as panels ---------------------------------------------------------

body_view :: proc(b: ^Body, cx: ^gk.Cx) -> gk.Element {
	b.described += 1
	return gk.label(b.text)
}

panel_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Panel_Event, which: int) {
	fmt.printf("overlay panel %d event %v %v\n", which, e.kind, e.flag)
}

dock_make :: proc(a: ^App, cx: ^gk.Cx) {
	a.bodies = {{text = "first"}, {text = "second"}}
	a.dock = gk.dock_area_skinned(cx, "test")
	for i in 0 ..< 2 {
		v := gk.view(&a.bodies[i], nil, body_view)
		a.panels[i] = gk.panel(cx, v, {name = i == 0 ? "first" : "second", title = a.bodies[i].text}, gk.on(panel_event, i))
	}
	a.dock->set_center(cx, gk.dock_layout_tabs()->panel(a.panels[0])->panel(a.panels[1]))
	fmt.println("overlay dock made")
}

dock_check :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay dock described first=%d second=%d empty=%v\n", a.bodies[0].described, a.bodies[1].described,
		a.dock->is_empty(cx, .Center))
	gk.panel_set(cx, a.panels[0], {name = "first", title = "renamed", not_closable = true})
	gk.panel_emit(cx, a.panels[0], .Zoom_In)
}

dock_zoomed :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay dock zoomed=%v\n", a.dock->is_zoomed(cx))
	a.dock->remove_panel(cx, a.panels[1])
}

dock_done :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay done mode=dock zoomed=%v\n", a.dock->is_zoomed(cx))
}

dock_root :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if a.dock.h == 0 {
		return gk.div(.Size_Full)
	}
	return gk.div(.Size_Full)->child(gk.dock_view(a.dock))
}

// --- PROTOCOLS.md 11.8: node ids, the tab group, a split, zoom by node -----------------

dock_ids :: proc(a: ^App, cx: ^gk.Cx) {
	g, ok := gk.panel_group(cx, a.panels[0])
	fmt.printf("overlay ids group=%v\n", ok)
	if !ok {return}
	a.node = g->node(cx)
	ps := g->panels(cx)
	ctx := g->context_(cx)
	fmt.printf("overlay ids panels=%d same=%v,%v active=%d ctx_node_same=%v ctx_panels=%d closable=%v\n", len(ps),
		len(ps) == 2 && ps[0].h == a.panels[0].h, len(ps) == 2 && ps[1].h == a.panels[1].h,
		g->active_ix(cx), ctx->node(cx) == a.node, len(ctx->panels(cx)), ctx->is_panel_closable(cx, gk.panel_id(cx, a.panels[0])))
	p, found := a.dock->panel(cx, gk.panel_id(cx, a.panels[1])).?
	fmt.printf("overlay ids dock panel found=%v same=%v\n", found, found && p.h == a.panels[1].h)
	// split the group: the second panel goes to the right of it, in a new group
	a.dock->split_at(cx, a.node, gk.panel_id(cx, a.panels[1]), .Right)
}

dock_ids_split :: proc(a: ^App, cx: ^gk.Cx) {
	tree, ok := a.dock->layout(cx, .Center).?
	ids := tree->node_ids(cx)
	n1, _ := tree->find_panel_node(cx, gk.panel_id(cx, a.panels[0])).?
	n2, _ := tree->find_panel_node(cx, gk.panel_id(cx, a.panels[1])).?
	root := tree->root(cx)->id(cx)
	fmt.printf("overlay ids split tree=%v nodes=%d first_kept=%v second_moved=%v\n", ok, len(ids), n1 == a.node, n2 != a.node)
	kind := gk.pane_node_kind(cx, tree->root(cx))
	sp, is_split := kind.(gk.Pane_Split)
	walked := gk.pane_node_walk(cx, tree->root(cx))
	fmt.printf("overlay ids kind split=%v children=%d walk=%d\n", is_split, len(sp.children), len(walked))
	if len(sp.children) > 0 {
		tabs, is_tabs := gk.pane_node_kind(cx, sp.children[0]).(gk.Pane_Tabs)
		fmt.printf("overlay ids first child tabs=%v panels=%d\n", is_tabs, len(tabs.panels))
	}
	a.dock->set_split_sizes(cx, root, {200, 440})
	a.dock->set_zoomed_in(cx, n2)
}

dock_ids_zoom :: proc(a: ^App, cx: ^gk.Cx) {
	z, zoomed := a.dock->zoomed_group(cx).?
	tree, _ := a.dock->layout(cx, .Center).?
	n2, _ := tree->find_panel_node(cx, gk.panel_id(cx, a.panels[1])).?
	fmt.printf("overlay ids zoomed=%v group=%v\n", zoomed, z == n2)
	a.dock->set_zoomed_out(cx)
	gk.dock_skin_set_panel_style(cx, a.dock, .Tab_Bar)
	gk.dock_skin_set_toggle_button_visible(cx, a.dock, false)
	fmt.printf("overlay ids skin style=%v toggle=%v\n", gk.dock_skin_panel_style(cx, a.dock), gk.dock_skin_toggle_button_visible(cx, a.dock))
	saved := a.dock->dump(cx)
	sizes: []f32
	if s, ok := saved.center.info.(gk.Panel_Info_Stack); ok {sizes = s.sizes}
	fmt.printf("overlay done mode=dock-ids children=%d sizes=%v\n", len(saved.center.children), sizes)
}

bad_node :: proc(a: ^App, cx: ^gk.Cx) {
	a.dock->set_zoomed_in(cx, gk.Node_Id(987654321)) // never handed out
}

// --- a saved layout restored through the registry --------------------------------------

restore :: proc(a: ^App, cx: ^gk.Cx, b: gk.Panel_Build) {
	a.builds += 1
	i := b.name == "first" ? 0 : 1
	fmt.printf("overlay layout build %s n=%d dock=%v\n", b.name, a.builds, b.dock_handle == a.dock.h)
	a.panels[i] = b.panel
	a.bodies[i].text = fmt.aprintf("restored %s", b.name)
	info := ""
	if pi, ok := b.state.info.(gk.Panel_Info_Panel); ok {info = pi.json} // the panel's own saved data
	gk.panel_fill(cx, b.panel, gk.view(&a.bodies[i], nil, body_view), {name = b.name, title = b.name, info = info}, gk.on(panel_event, i))
}

print_state :: proc(s: gk.Panel_State, depth := 0) -> string {
	kind := "panel"
	switch v in s.info {
	case gk.Panel_Info_Stack: kind = fmt.tprintf("stack%d", len(v.sizes))
	case gk.Panel_Info_Tabs: kind = fmt.tprintf("tabs@%d", v.active_index)
	case gk.Panel_Info_Panel: kind = fmt.tprintf("panel'%s'", v.json)
	}
	out := fmt.tprintf("%s:%s(", s.panel_name, kind)
	for c in s.children {out = fmt.tprintf("%s%s ", out, print_state(c, depth + 1))}
	return fmt.tprintf("%s)", out)
}

layout_save :: proc(a: ^App, cx: ^gk.Cx) {
	gk.panel_set(cx, a.panels[0], {name = "first", title = "first", info = `{"file":"a.odin"}`})
}

layout_load :: proc(a: ^App, cx: ^gk.Cx) {
	saved := a.dock->dump(cx)
	a.saved = fmt.aprint(print_state(saved.center))
	fmt.printf("overlay layout saved %s\n", a.saved)
	gk.register_panel(cx, "first", gk.on(restore))
	gk.register_panel(cx, "second", gk.on(restore))
	a.builds = 0
	err, failed := a.dock->load(cx, saved).?
	fmt.printf("overlay layout loaded failed=%v err=%s\n", failed, err)
}

layout_check :: proc(a: ^App, cx: ^gk.Cx) {
	again := fmt.tprint(print_state(a.dock->dump(cx).center))
	fmt.printf("overlay layout again same=%v described=%d,%d\n", again == a.saved, a.bodies[0].described, a.bodies[1].described)
	tree, _ := a.dock->layout(cx, .Center).?
	st := gk.pane_tree_to_state(cx, tree, a.dock)
	t2 := gk.pane_tree_from_state(cx, a.dock, st, .Split)
	fmt.printf("overlay layout tree to_state=%s from_state_panels=%d\n", print_state(st), len(t2->panels(cx)))
	p, ok := gk.build_panel(cx, a.dock, {panel_name = "second", info = gk.Panel_Info_Panel{}})
	q, ok2 := gk.build_panel(cx, a.dock, {panel_name = "nobody", info = gk.Panel_Info_Panel{}})
	fmt.printf("overlay layout build_panel known=%v unknown=%v\n", ok && p.h != 0, ok2)
}

layout_done :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay done mode=dock-layout builds=%d\n", a.builds)
}

// --- the nav stack takes and gives back app views -------------------------------------

nav_make :: proc(a: ^App, cx: ^gk.Cx) {
	a.pages = {{text = "home"}, {text = "details"}}
	for i in 0 ..< 2 {a.page_views[i] = gk.view(&a.pages[i], nil, body_view)}
	a.nav = gk.nav_stack_state(cx)
	a.nav->push(cx, a.page_views[0], .Immediate)
	a.nav->push(cx, a.page_views[1], .Immediate)
}

nav_check :: proc(a: ^App, cx: ^gk.Cx) {
	cur, ok := a.nav->current(cx).?
	vs := a.nav->views(cx)
	fmt.printf("overlay nav depth=%d current=%v views=%d,%v\n", a.nav->depth(cx), ok && cur.h == a.page_views[1].h,
		len(vs), len(vs) == 2 && vs[0].h == a.page_views[0].h)
	popped, pok := a.nav->pop(cx, .Immediate).?
	cur2, _ := a.nav->current(cx).?
	fmt.printf("overlay nav popped=%v now=%v forward=%d\n", pok && popped.h == a.page_views[1].h, cur2.h == a.page_views[0].h,
		len(a.nav->forward_views(cx)))
	bar := gk.app_menu_bar(cx)
	bar->reload(cx)
	a.bar = bar
	r := gk.resizable_state(cx)
	fmt.printf("overlay done mode=nav sizes=%d container=%v\n", len(r->sizes(cx)), r->container_size(cx))
}
nav_root :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if a.nav.h == 0 {return gk.div(.Size_Full)}
	root := gk.div(.Size_Full)->child(gk.nav_stack(a.nav))
	if a.bar.h != 0 {root = root->child(gk.app_menu_bar_view(a.bar))}
	return root
}

// --- a real window on a display (an X server such as Xvfb): title and size ------------------------
// Offscreen every window has the meter's size and the headless platform keeps no title, so the
// app tests could only read them; here the X server holds them. GPUI reads a window's title back
// on macOS only (gpui-fast's get_title is empty elsewhere), so run-overlay.sh asks the X server
// (xwininfo) while the app runs; the size comes back through gk.window_info.

real_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->child(gk.label("a real window"))
}

real_read :: proc(a: ^App, cx: ^gk.Cx) {
	i := gk.window_info(cx, gk.windows(cx)[0])
	fmt.printf("overlay window title=%q w=%d h=%d\n", i.title, int(i.width), int(i.height))
	gk.set_window_title(cx, gk.windows(cx)[0], "Renamed by the app")
}

real_again :: proc(a: ^App, cx: ^gk.Cx) {
	i := gk.window_info(cx, gk.windows(cx)[0])
	fmt.printf("overlay window again title=%q w=%d h=%d\n", i.title, int(i.width), int(i.height))
	gk.quit(cx)
}

real_init :: proc(a: ^App, cx: ^gk.Cx) {
	gk.after(cx, 1000, gk.on(real_read))
	gk.after(cx, 2000, gk.on(real_again))
}

real_start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, real_init, real_view)
	gk.open_window(cx, "Odin on the kit", {420, 300}, a.root)
}

// --- PROTOCOLS.md 11.9: a dock drawn by the app's renderer -------------------------------

chrome_view :: proc(c: ^Chrome, cx: ^gk.Cx) -> gk.Element {
	c.described += 1
	return gk.v_flex(.Size_Full)->child(gk.label(fmt.tprintf("%v open=%v", c.placement, c.open)))->child(gk.dock_slot())
}

tabbar_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.tabbars += 1
	return gk.h_flex(.W_Full)->child(gk.label("tabs of the app"))
}

frame_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.frames += 1
	if a.mode == "render-bad-frame" {
		return gk.label("a frame must be a div") // misuse: a frame hook's view is a gk.div()
	}
	return gk.h_flex(.Size_Full, .P_1)
}

asked :: proc(a: ^App, cx: ^gk.Cx, q: gk.Dock_Render) {
	a.asks[q.hook] += 1
	#partial switch q.hook {
	case .Frame:
		gk.dock_render_answer(cx, q.renderer, q.hook, q.key, a.frame_v)
	case .Dock:
		a.chrome.placement = q.dock->placement(cx)
		a.chrome.open = q.dock->is_open(cx)
		a.dock_ctx = q.dock
		fmt.printf("overlay render asked Dock placement=%v open=%v key=%d same=%v\n", q.placement, a.chrome.open, q.key,
			q.placement == a.chrome.placement)
		gk.dock_render_answer(cx, q.renderer, q.hook, q.key, a.chrome_v)
	case .Tab_Bar:
		fmt.printf("overlay render asked Tab_Bar panels=%d\n", len(q.group->panels(cx)))
		gk.dock_render_answer(cx, q.renderer, q.hook, q.key, a.tabbar_v)
	}
}

render_make :: proc(a: ^App, cx: ^gk.Cx) {
	a.bodies = {{text = "first"}, {text = "second"}}
	a.chrome_v = gk.view(&a.chrome, nil, chrome_view)
	a.tabbar_v = gk.view(a, nil, tabbar_view)
	a.frame_v = gk.view(a, nil, frame_view)
	a.renderer = gk.dock_renderer(cx, {.Frame, .Dock, .Tab_Bar}, gk.on(asked))
	a.dock = gk.dock_area(cx, "rendered", nil)->with_renderer(a.renderer)->build()
	for i in 0 ..< 2 {
		v := gk.view(&a.bodies[i], nil, body_view)
		a.panels[i] = gk.panel(cx, v, {name = i == 0 ? "first" : "second", title = a.bodies[i].text})
	}
	a.dock->set_center(cx, gk.dock_layout_tabs()->panel(a.panels[0]))
	a.dock->set_dock(cx, .Left, gk.dock_layout_tabs()->panel(a.panels[1]))
	a.toggle_sub = gk.on_dock_toggle(cx, a.dock, gk.on(proc(a: ^App, cx: ^gk.Cx, t: gk.Dock_Toggle) { a.toggles += 1 }))
	fmt.println("overlay render made")
}

render_check :: proc(a: ^App, cx: ^gk.Cx) {
	sent, answers, drawn := gk.test_dock_render(cx, a.renderer)
	fmt.printf("overlay render frame=%d dock=%d tab_bar=%d answers=%d drawn_some=%v\n", a.asks[.Frame], a.asks[.Dock],
		a.asks[.Tab_Bar], answers, drawn > 0)
	fmt.printf("overlay render described chrome=%v tabbar=%v frame=%v bodies=%v\n", a.chrome.described > 0, a.tabbars > 0,
		a.frames > 0, a.bodies[0].described > 0 && a.bodies[1].described > 0)
	_ = sent
	a.dock_ctx->toggle(cx) // the chrome's own call: the dock closes, its context changes, it is asked again
}

render_done :: proc(a: ^App, cx: ^gk.Cx) {
	// the kit reports a toggle of the user's (the skin's buttons) only: the chrome's call is the app's
	fmt.printf("overlay done mode=render open=%v dock_asks=%d chrome_open=%v toggles=%d sub=%v\n", a.dock->is_dock_open(cx, .Left),
		a.asks[.Dock], a.chrome.open, a.toggles, a.toggle_sub.h != 0)
}

// --- PROTOCOLS.md 11.10: the toast manager on the library clock ---------------------------

toast_run :: proc(a: ^App, cx: ^gk.Cx) {
	a.toasts = gk.toast_manager(cx, gk.Toast_Motion{duration = 100 * time.Millisecond, exit_duration = 50 * time.Millisecond})
	t0 := gk.clock_now(cx)
	a.toasts->push(cx, 1, 100, gk.Toast_Options{timeout = 300 * time.Millisecond}, t0)
	a.toasts->push(cx, 2, 200, gk.Toast_Options{}, t0)
	replaced := a.toasts->push(cx, 2, 201, gk.Toast_Options{}, t0)
	v2, ok2 := a.toasts->get(cx, 2).?
	fmt.printf("overlay toast len=%d replaced=%v get2=%v\n", a.toasts->len_(cx), replaced, v2)
	_ = ok2
	adv := a.toasts->advance(cx, t0 + 150 * time.Millisecond, false)
	fmt.printf("overlay toast presented=%v changed=%v\n", adv.presented, adv.changed)
	adv = a.toasts->advance(cx, t0 + 500 * time.Millisecond, false)
	fmt.printf("overlay toast ending=%v\n", adv.ending)
	adv = a.toasts->advance(cx, t0 + 600 * time.Millisecond, false)
	removed := len(adv.removed) > 0 ? adv.removed[0] : gk.Tuple_U64_U64{}
	fmt.printf("overlay toast removed=%d value=%d left=%d\n", removed.v0, removed.v1, a.toasts->len_(cx))
	dismissed := a.toasts->dismiss(cx, 2, t0 + 600 * time.Millisecond)
	visible := a.toasts->visible(cx, 5)
	fmt.printf("overlay toast dismissed=%v visible=%d status=%v empty=%v\n", dismissed, len(visible), len(visible) > 0 ? visible[0].v2 : gk.Toast_Transition_Status.Starting, a.toasts->is_empty(cx))
	a.toasts->push(cx, 3, 300, gk.Toast_Options{}, t0 + 600 * time.Millisecond)
	all := a.toasts->iter(cx)
	gone := a.toasts->dismiss_all(cx, t0 + 700 * time.Millisecond)
	later := gk.clock_now(cx)
	fmt.printf("overlay done mode=toast iter=%d dismissed_all=%v clock_moves=%v\n", len(all), gone, later >= t0)
}

// --- PROTOCOLS.md 11.12: a popover opened and closed from code ----------------------------

popover_changed :: proc(a: ^App, cx: ^gk.Cx, open: bool) {
	a.popover_changes += 1
	a.popover_open = open
}

popover_root :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	p := gk.popover("pop")->trigger(gk.button("trigger")->label("Open"))->open(a.popover_open)->
		on_open_change(gk.on(popover_changed))->child(gk.label("inside the popover"))
	return gk.div(.Size_Full)->child(p)
}

popover_open :: proc(a: ^App, cx: ^gk.Cx) {
	a.popover_open = true
	fmt.println("overlay popover opened by the app")
}

popover_close :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay popover open=%v\n", a.popover_open)
	a.popover_open = false
}

popover_done :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay done mode=popover open=%v changes=%d\n", a.popover_open, a.popover_changes)
}

// --- views family, PROTOCOLS.md 11.14, 11.15: observations, value calls --------------------

// A positioner at a corner the app moves, and a popup (always open), each reporting where it was laid
// out (the popup also its trigger's bounds): the reports come after the frame, once per change, never again for the same values.
positioned :: proc(a: ^App, cx: ^gk.Cx, p: gk.Resolved_Position) {
	a.positions += 1
	a.last_pos = p
}

popped :: proc(a: ^App, cx: ^gk.Cx, e: gk.Event_117) {
	a.pops += 1
	a.last_pop = e
}

observe_root :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.frames5 += 1
	pos := gk.positioner_corner(.Top_Left, {a.obs_x, 50})->on_position(gk.on(positioned))->child(gk.label("placed"))
	pop := gk.popup("pp", gk.button("bt")->label("Trigger"))->content(gk.label("inside"))->on_position(gk.on(popped))
	return gk.div(.Size_Full)->child(pos)->child(gk.div(.P_4)->child(pop))
}

observe_first :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay observe first positions=%d x=%.0f y=%.0f placement=%v pops=%d pop_w=%v\n", a.positions,
		a.last_pos.bounds.origin.x, a.last_pos.bounds.origin.y, a.last_pos.placement, a.pops, a.last_pop.a1.size.width > 0)
	a.obs_x = 120 // the positioner moves: one more report
}

observe_moved :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay observe moved positions=%d x=%.0f pops=%d\n", a.positions, a.last_pos.bounds.origin.x, a.pops)
}

observe_done :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("overlay done mode=observe positions=%d pops=%d frames_seen=%v\n", a.positions, a.pops, a.frames5 >= 3)
}

// Value calls: computations over a value the app built (DockSizing, Sankey), run as ambient calls.
value_calls :: proc(a: ^App, cx: ^gk.Cx) {
	sizing := gk.dock_sizing(.Left)->with_area_width(1000)->with_opposite_dock_size(200)
	big := gk.base_dock_sizing_clamp(cx, sizing, 5000)
	small := gk.base_dock_sizing_clamp(cx, gk.dock_sizing(.Left)->with_area_width(1000)->with_opposite_dock_size(200), 10)
	from := gk.base_dock_sizing_size_from_pointer(cx, gk.dock_sizing(.Left)->with_area_width(1000), {300, 10})
	fmt.printf("overlay values clamp=%.0f,%.0f pointer=%.0f\n", big, small, from)
	links := [?]gk.Sankey_Link{gk.sankey_link(0, 1, 5), gk.sankey_link(1, 2, 5)}
	g := gk.base_sankey_layout(cx, gk.sankey()->size(300, 200), 3, links[:])
	layers := -1
	if graph, ok := g.value.?; ok {
		layers = graph->layer_count(cx)
	}
	bad := gk.base_sankey_layout(cx, gk.sankey(), 1, links[:]) // a link to a node that is not there
	fmt.printf("overlay done mode=values ok=%v layers=%d bad_ok=%v bad_err=%v\n", g.ok, layers, bad.ok, len(bad.err) > 0)
}

// --- the app -----------------------------------------------------------------------------------

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->child(gk.label("overlay"))
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "dialog":
		gk.after(cx, 10, gk.on(open_first))
		gk.after(cx, 30, gk.on(bump))
		gk.after(cx, 50, gk.on(reopen))
		gk.after(cx, 70, gk.on(close_by_app))
		gk.after(cx, 90, gk.on(dialog_done))
	case "alert":
		gk.after(cx, 10, gk.on(open_alert))
		gk.after(cx, 50, gk.on(alert_done))
	case "sheet":
		gk.after(cx, 10, gk.on(open_sheet))
		gk.after(cx, 30, gk.on(sheet_check))
		gk.after(cx, 50, gk.on(sheet_done))
	case "release":
		gk.after(cx, 10, gk.on(open_first))
		gk.after(cx, 30, gk.on(release_first))
	case "wrong-root":
		gk.after(cx, 10, gk.on(open_wrong))
	case "dock":
		gk.after(cx, 10, gk.on(dock_make))
		gk.after(cx, 30, gk.on(dock_check))
		gk.after(cx, 50, gk.on(dock_zoomed))
		gk.after(cx, 70, gk.on(dock_done))
	case "dock-ids":
		gk.after(cx, 10, gk.on(dock_make))
		gk.after(cx, 30, gk.on(dock_ids))
		gk.after(cx, 50, gk.on(dock_ids_split))
		gk.after(cx, 70, gk.on(dock_ids_zoom))
	case "dock-bad-node":
		gk.after(cx, 10, gk.on(dock_make))
		gk.after(cx, 30, gk.on(bad_node))
	case "dock-layout":
		gk.after(cx, 10, gk.on(dock_make))
		gk.after(cx, 30, gk.on(layout_save))
		gk.after(cx, 50, gk.on(layout_load))
		gk.after(cx, 70, gk.on(layout_check))
		gk.after(cx, 90, gk.on(layout_done))
	case "nav":
		gk.after(cx, 10, gk.on(nav_make))
		gk.after(cx, 30, gk.on(nav_check))
	case "questionnaire", "questionnaire-dup":
		gk.after(cx, 10, gk.on(questionnaire))
	case "render-bad-frame":
		gk.after(cx, 10, gk.on(render_make))
	case "render":
		gk.after(cx, 10, gk.on(render_make))
		gk.after(cx, 50, gk.on(render_check))
		gk.after(cx, 90, gk.on(render_done))
	case "toast":
		gk.after(cx, 10, gk.on(toast_run))
	case "observe":
		gk.after(cx, 10, gk.on(observe_first))
		gk.after(cx, 30, gk.on(observe_moved))
		gk.after(cx, 70, gk.on(observe_done))
	case "values":
		gk.after(cx, 10, gk.on(value_calls))
	case "popover":
		gk.after(cx, 10, gk.on(popover_open))
		gk.after(cx, 30, gk.on(popover_close))
		gk.after(cx, 50, gk.on(popover_done))
	}
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	root := root_view
	switch a.mode {
	case "dock", "dock-ids", "dock-bad-node", "dock-layout", "render", "render-bad-frame": root = dock_root
	case "nav": root = nav_root
	case "popover": root = popover_root
	case "observe": root = observe_root
	}
	a.root = gk.view(a, init, root)
	gk.open_window(cx, "overlay", {640, 480}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	step := "frame\nframe\n"
	script := "size 640 480\nframe\n"
	switch app.mode {
	case "dialog":
		// open at 10; bumped at 30 while open; escape (on_cancel of the second description); reopened
		// at 50 on the same view; closed by the app at 70; done at 90
		script = fmt.aprint(script, "advance 10\n", step, "advance 20\n", step, "key escape\n", step,
			"advance 20\n", step, "advance 20\n", step, "advance 20\n", step, "stat\n", sep = "")
	case "alert":
		script = fmt.aprint(script, "advance 10\n", step, "key enter\n", step, "advance 40\n", step, "stat\n", sep = "")
	case "dock-layout", "render":
		script = fmt.aprint(script, "advance 10\n", step, "advance 20\n", step, "advance 20\n", step, "advance 20\n", step,
			"advance 20\n", step, "stat\n", sep = "")
	case:
		script = fmt.aprint(script, "advance 10\n", step, "advance 20\n", step, "advance 20\n", step, "advance 20\n", step, "stat\n", sep = "")
	}
	if app.mode == "window-real" {
		gk.run(&app, real_start)
		return
	}
	gk.run_headless(&app, script, start)
}
