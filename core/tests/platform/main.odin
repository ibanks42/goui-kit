// platform: the protocol tests of the platform and motion families (docs/PROTOCOLS.md 11.30): requests
// that answer later (completions delivered by a DONE turn), file dialogs, a window's prompt, the
// menu bar and open_url. Offscreen the platform is scripted: the test answers its own dialogs with
// gk.platform_answer_*. Each mode prints lines that core/tests/run-platform.sh checks, with the exit
// code.
//
//   GK_PROTO=dialogs | cancel | race | view-released | menus | release-twice | request-in-view |
//            wrong-payload | menu-no-action
//            PROTOCOLS.md 13.31: elements | drag | vlist | cache-in-view | scope-zero
//            PROTOCOLS.md 11.32: real-dialogs | real-url | real-menus (a real window, the
//            platform asked: run by bin/platform-real under gates/display.sh, GK_SCRIPTED_PLATFORM
//            unset); context-menu | context-menu-kit (offscreen: an input's right click)
//   GK_PLATFORM_SCRIPT=FILE   a gk-meter script instead of the mode's (to take shots of what a mode draws)
package platform_test

import "core:fmt"
import "core:os"
import "core:strings"
import "core:time"
import gk "gpuikit:gk"

App :: struct {
	mode:      string,
	root:      gk.View,
	child_v:   gk.View,
	child_w:   gk.Window,
	c1, c2:    gk.Completion,
	c3, c4:    gk.Completion,
	done:      int,
	described: int,
	last:      string,
	open:      gk.Action,
	save:      gk.Action,
	// PROTOCOLS.md 13.31
	cache:     gk.Image_Cache,
	scroll:    gk.Scroll_Handle,
	anchor:    gk.Scroll_Anchor,
	scope:     gk.Text_Selection_Scope,
	rows:      gk.Row_Source,
	drops:     int,
	moves:     int,
	row_views: int,
	// PROTOCOLS.md 11.32
	asked_at:  time.Tick,
	polls:     int,
	menubar:   gk.App_Menu_Bar,
	bar_seen:  bool,
	inp:       gk.Input_State,
	fired:     int,
}

// --- dialogs: answered later, answered before, a prompt, a cancel --------------------------------

opened :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) {
	a.done += 1
	a.last = strings.join(d.paths, ",", context.allocator)
	fmt.printf("platform opened n=%d status=%v paths=%s error=%q\n", a.done, d.status, a.last, d.error)
}

saved_to :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) {
	a.done += 1
	fmt.printf("platform saved n=%d status=%v paths=%s\n", a.done, d.status, strings.join(d.paths, ",", context.temp_allocator))
	gk.release(cx, a.c2) // answered: releasing it does nothing
}

answered :: proc(a: ^App, cx: ^gk.Cx, d: gk.Prompt_Done) {
	a.done += 1
	fmt.printf("platform prompt n=%d status=%v answer=%d\n", a.done, d.status, d.answer)
}

dismissed :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) {
	a.done += 1
	fmt.printf("platform dismissed n=%d status=%v paths=%d\n", a.done, d.status, len(d.paths))
}

ask_open :: proc(a: ^App, cx: ^gk.Cx) {
	a.c1 = gk.prompt_for_paths(cx, {files = true, multiple = true, prompt = "Open"}, gk.on(opened))
	// the save dialog's answer was given before it was asked: it is taken at once
	took := gk.platform_answer_paths(cx, .Answered, {"/tmp/new.txt"}, new_path = true)
	fmt.printf("platform early answer taken=%v\n", took)
	a.c2 = gk.prompt_for_new_path(cx, "/tmp", gk.on(saved_to), "new.txt")
	a.c3 = gk.prompt(cx, .Warning, "Replace it?", {{label = "Replace"}, {.Cancel, "Keep"}}, gk.on(answered), detail = "It exists")
	a.c4 = gk.prompt_for_paths(cx, {directories = true}, gk.on(dismissed))
}

answer_all :: proc(a: ^App, cx: ^gk.Cx) {
	t1 := gk.platform_answer_paths(cx, .Answered, {"/tmp/a.txt", "/tmp/b.txt"})
	t2 := gk.platform_answer_prompt(cx, .Answered, 1)
	t3 := gk.platform_answer_paths(cx, .Cancelled)
	fmt.printf("platform answers taken=%v,%v,%v\n", t1, t2, t3)
}

check_log :: proc(a: ^App, cx: ^gk.Cx) {
	l := gk.platform_log(cx)
	fmt.printf("platform log requests=%d delivered=%d discarded=%d waiting=%d scripted=%v done=%d\n",
		l.requests, l.delivered, l.discarded, l.waiting, l.scripted, a.done)
	gk.release(cx, a.c1) // answered already: nothing
}

// --- cancel: released before its answer ----------------------------------------------------------

never :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) {
	a.done += 1
	fmt.println("platform cancelled request answered (wrong)")
}

ask_then_release :: proc(a: ^App, cx: ^gk.Cx) {
	a.c1 = gk.prompt_for_paths(cx, {files = true}, gk.on(never))
}

release_it :: proc(a: ^App, cx: ^gk.Cx) {
	gk.release(cx, a.c1)
}

answer_late :: proc(a: ^App, cx: ^gk.Cx) {
	took := gk.platform_answer_paths(cx, .Answered, {"/tmp/late.txt"})
	fmt.printf("platform late answer taken=%v\n", took)
}

// --- race: released and answered in the same turn (the answer arrives before the release lands) ----

race :: proc(a: ^App, cx: ^gk.Cx) {
	took := gk.platform_answer_paths(cx, .Answered, {"/tmp/race.txt"}) // taken: delivered after this turn
	gk.release(cx, a.c1) // buffered: lands after this turn, after the answer was delivered
	fmt.printf("platform race answer taken=%v\n", took)
}

// --- view-released: a request of a child view goes with the view ---------------------------------

child_asked :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) {
	a.done += 1
	fmt.println("platform child's request answered after its view was released (wrong)")
}

child_init :: proc(a: ^App, cx: ^gk.Cx) {
	gk.prompt_for_paths(cx, {files = true}, gk.on(child_asked))
	fmt.println("platform child asked")
}

child_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.label("child")
}

open_child :: proc(a: ^App, cx: ^gk.Cx) {
	// a window's root view runs its init when the window opens
	a.child_v = gk.view(a, child_init, child_view)
	a.child_w = gk.open_window(cx, "child", {200, 100}, a.child_v)
}

release_child :: proc(a: ^App, cx: ^gk.Cx) {
	gk.close_window(cx, a.child_w)
	gk.release(cx, a.child_v)
	a.child_v = {}
	fmt.println("platform child released")
}

// --- menus and URLs -----------------------------------------------------------------------------

set_bar :: proc(a: ^App, cx: ^gk.Cx) {
	recent := []gk.Menu_Item{gk.Menu_Action{name = "notes.txt", action = a.open}}
	file := []gk.Menu_Item{
		gk.Menu_Action{name = "Open", action = a.open},
		gk.Menu_Separator{},
		gk.Menu_Submenu{gk.Menu{name = "Recent", items = recent}},
		gk.Menu_Services{"Services"},
	}
	edit := []gk.Menu_Item{gk.Menu_Action{name = "Save", action = a.save, os_action = .Copy, checked = true, disabled = true}}
	gk.set_menus(cx, {name = "File", items = file}, {name = "Edit", items = edit, disabled = true})
	gk.open_url(cx, "https://example.com/gk")
}

read_bar :: proc(a: ^App, cx: ^gk.Cx) {
	ms := gk.menus(cx)
	fmt.printf("platform menus n=%d\n", len(ms))
	for m in ms {
		fmt.printf("platform menu %q disabled=%v items=%d\n", m.name, m.disabled, len(m.items))
		for it in m.items {
			switch x in it {
			case gk.Menu_Separator:
				fmt.println("platform item separator")
			case gk.Menu_Action:
				fmt.printf("platform item action %q same=%v os=%v checked=%v disabled=%v\n", x.name,
					x.action == a.open || x.action == a.save, x.os_action, x.checked, x.disabled)
			case gk.Menu_Submenu:
				fmt.printf("platform item submenu %q items=%d\n", x.menu.name, len(x.menu.items))
			case gk.Menu_Services:
				fmt.printf("platform item services %q\n", x.name)
			}
		}
	}
	l := gk.platform_log(cx)
	fmt.printf("platform urls n=%d first=%q\n", len(l.urls), l.urls[0] if len(l.urls) > 0 else "")
}

// --- gpui's element leftovers (PROTOCOLS.md 13.31) ---------------------------------------

// elements: the decoders generated code uses, run by the library on the bytes the package wrote;
// the objects the library makes for the elements, made and released.
elements :: proc(a: ^App, cx: ^gk.Cx) {
	v1, f1 := gk.test_drop_accept(cx, {ids = {7, 9}}, 7)
	v2, _ := gk.test_drop_accept(cx, {ids = {7, 9}}, 8)
	v3, f3 := gk.test_drop_accept(cx, {values = true, paths = true}, 8)
	v4, _ := gk.test_drop_accept(cx, {}, 7)
	fmt.printf("platform drop accept 7=%v 8=%v any=%v none=%v files=%v,%v\n", v1, v2, v3, v4, f1, f3)
	fs := gk.test_drag_payload(cx, {files = {{path = "/tmp/a.txt"}, {path = "/tmp/dir", directory = true}}})
	fmt.printf("platform drag payload n=%d first=%s dir=%v,%v\n", len(fs), fs[0].path, fs[0].directory, fs[1].directory)
	none := gk.test_drag_payload(cx, {})
	fmt.printf("platform drag payload none n=%d\n", len(none))
	order := gk.test_prepaint_order(cx, {2, 0, 1})
	fmt.printf("platform prepaint order=%v\n", order)
	red := []u8{255, 0, 0, 255, 0, 0, 255, 255}
	w, h, first := gk.test_pixel_image(cx, {2, 1, red})
	cw, ch, cfirst := gk.test_pixel_image(cx, {2, 1, red}, custom = true)
	fmt.printf("platform pixel image %dx%d first=%08x custom %dx%d first=%08x\n", w, h, first, cw, ch, cfirst)
	if e, ok := gk.test_file_drop_entered(cx).(gk.File_Drop_Event_Entered); ok {
		fmt.printf("platform file drop entered at=%.0f,%.0f paths=%s\n", e.position.x, e.position.y,
			strings.join(transmute([]string)e.paths, ",", context.temp_allocator))
	}
	// Sequence::with_step: a Transition node in a call op's builder chain (the rule of 9.31)
	seq := gk.sequence(cx, "seq", 10)->with_step(40, gk.transition(200 * time.Millisecond))->with_step(80, gk.transition(100 * time.Millisecond))->build()
	steps := seq->steps(cx)
	fmt.printf("platform sequence steps=%d from=%.0f targets=%.0f,%.0f\n", len(steps), seq->from(cx), steps[0]->target(cx), steps[1]->target(cx))
	gk.release(cx, seq)
	a.cache = gk.image_cache(cx)
	a.scroll = gk.scroll_handle(cx)
	a.anchor = gk.scroll_anchor(cx, a.scroll)
	a.scope = gk.text_selection_scope_id(cx)
	fmt.printf("platform objects cache=%v anchor=%v scope=%v\n", a.cache.h != 0, a.anchor.h != 0, a.scope.h != 0)
}

elements_drawn :: proc(a: ^App, cx: ^gk.Cx) {
	gk.scroll_anchor_to(cx, a.anchor)
	gk.release(cx, a.anchor)
	gk.release(cx, a.cache)
	gk.release(cx, a.scope)
	fmt.printf("platform elements released described=%d\n", a.described)
}

// The elements of 13.31 drawn (a mode's view): a scope, a cache on a div and an img, an anchored
// scrolled child, synthetic accessibility nodes, a prepaint order, a drag's files.
elements_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if a.cache.h == 0 {
		return gk.label("waiting")
	}
	px := gk.Pixel_Image{2, 2, {255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255}}
	pic := gk.img(gk.Image_Source_Render{px})->w(gk.px(20))->h(gk.px(20))->image_cache(a.cache)
	inner := gk.div()->id("anchored")->anchor_scroll(a.anchor)->a11y_synthetic_children({{"q1", .List_Item, "first"}, {"q2", .List_Item, "second"}})->
		child(gk.label("anchored"))
	box := gk.div()->image_cache(a.cache)->with_dynamic_prepaint_order({1, 0})->child(pic)->child(inner)
	files := gk.div()->id("files")->on_drag(gk.Drag_Value{3, "three"}, {})->external_drag_payload({files = {{path = "/tmp/a.txt"}}})->child(gk.label("files"))
	return gk.v_flex(.Size_Full)->child(gk.text_selection_scope(box, a.scope))->child(files)
}

// drag: a drag of value 7 onto a target that takes 7 (its drop runs, its drag-move handler sees the
// value), then onto one that takes only 8 (no drop); a group's drag-over style.
dropped7 :: proc(a: ^App, cx: ^gk.Cx, d: gk.Drag_Value) {
	a.drops += 1
	fmt.printf("platform drop on seven id=%d text=%s drops=%d\n", d.id, d.text, a.drops)
}

dropped8 :: proc(a: ^App, cx: ^gk.Cx, d: gk.Drag_Value) {
	a.drops += 1
	fmt.printf("platform drop on eight id=%d (wrong: it takes only 8)\n", d.id)
}

moved :: proc(a: ^App, cx: ^gk.Cx, e: gk.Drag_Move_Event) {
	a.moves += 1
	if a.moves == 1 {
		fmt.printf("platform drag move id=%d text=%s bounds=%.0fx%.0f at x>0=%v\n", e.value.id, e.value.text,
			e.bounds.size.width, e.bounds.size.height, e.position.x > 0)
	}
}

drag_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	src := gk.div()->w(gk.px(100))->h(gk.px(100))->id("src")->on_drag(gk.Drag_Value{7, "seven"}, {})->child(gk.label("seven"))
	t7 := gk.div()->w(gk.px(100))->h(gk.px(100))->id("t7")->group("targets")->can_drop({ids = {7}})->on_drop(gk.on(dropped7))->
		on_drag_move(gk.on(moved))->group_drag_over("targets", gk.style_refinement_default()->bg(gk.theme(.Accent)))
	t8 := gk.div()->w(gk.px(100))->h(gk.px(100))->id("t8")->can_drop({ids = {8}})->on_drop(gk.on(dropped8))
	return gk.h_flex()->child(src)->child(t7)->child(t8)
}

drag_done :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("platform drag done drops=%d moved=%v\n", a.drops, a.moves > 0)
}

// vlist: a vertical and a horizontal virtual list over a row source: rows drawn as placeholders for
// one frame, then described (only those in view).
vlist_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.row_views += 1
	return gk.label(fmt.tprintf("row %d", at.row))
}

vlist_init :: proc(a: ^App, cx: ^gk.Cx) {
	a.rows = gk.row_source(cx, a, 1000, vlist_row, initial = 0)
}

vlist_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	sizes := make([][2]f32, 1000, context.temp_allocator)
	for &s, i in sizes {
		s = {80, f32(20 + (i % 3) * 10)}
	}
	return gk.v_flex(.Size_Full)->
		child(gk.v_virtual_list("v", a.rows, sizes)->h(gk.px(200))) ->
		child(gk.h_virtual_list("h", a.rows, sizes[:10])->h(gk.px(40)))
}

vlist_check :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("platform vlist rows described=%v of 1000 some=%v\n", a.row_views < 1000, a.row_views > 0)
}

// --- the real platform, not the script (PROTOCOLS.md 11.32) -------------------------------
//
// Run on a real window (gk.run) under gates/display.sh: a private X server or compositor with its own
// D-Bus session bus that has no service files, so no xdg-desktop-portal answers; bin/platform-real puts
// a fake xdg-open first on PATH that writes its arguments to $GK_URL_LOG. A guard timer quits after
// 15 s, so a request that never answers fails the check instead of hanging the run.

real_guard :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("platform real guard: still waiting after 15 s (done=%d)\n", a.done)
	gk.quit(cx)
}

real_ask :: proc(a: ^App, cx: ^gk.Cx) {
	a.asked_at = time.tick_now()
	a.c1 = gk.prompt_for_paths(cx, {files = true, prompt = "Open"}, gk.on(real_answered))
	a.c2 = gk.prompt_for_new_path(cx, "/tmp", gk.on(real_answered), "new.txt")
	fmt.println("platform real asked")
}

real_answered :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) {
	a.done += 1
	ms := time.duration_milliseconds(time.tick_since(a.asked_at))
	fmt.printf("platform real answer n=%d status=%v paths=%d error=%q in_time=%v\n", a.done, d.status, len(d.paths), d.error, ms < 10000)
	if a.done == 2 {
		l := gk.platform_log(cx)
		fmt.printf("platform log requests=%d delivered=%d discarded=%d waiting=%d scripted=%v done=%d\n",
			l.requests, l.delivered, l.discarded, l.waiting, l.scripted, a.done)
		gk.quit(cx)
	}
}

real_url :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_url(cx, "https://example.com/gk-real?q=1")
	gk.after(cx, 100, gk.on(real_url_poll))
}

// The system opener runs on gpui's background executor: the app looks for what it wrote.
real_url_poll :: proc(a: ^App, cx: ^gk.Cx) {
	a.polls += 1
	path, _ := os.lookup_env("GK_URL_LOG", context.temp_allocator)
	data, err := os.read_entire_file(path, context.temp_allocator)
	if err == nil && len(data) > 0 {
		l := gk.platform_log(cx)
		fmt.printf("platform real url opener got %s", string(data))
		fmt.printf("platform real url recorded=%d scripted=%v\n", len(l.urls), l.scripted)
		gk.quit(cx)
		return
	}
	if a.polls < 50 {
		gk.after(cx, 100, gk.on(real_url_poll))
	} else {
		fmt.printf("platform real url: the opener wrote nothing to %q in 5 s\n", path)
		gk.quit(cx)
	}
}

real_menus_init :: proc(a: ^App, cx: ^gk.Cx) {
	file := []gk.Menu_Item{gk.Menu_Action{name = "Open", action = a.open}, gk.Menu_Separator{}, gk.Menu_Action{name = "Save", action = a.save}}
	help := []gk.Menu_Item{gk.Menu_Action{name = "About", action = a.open}}
	gk.set_menus(cx, {name = "File", items = file}, {name = "Help", items = help})
	a.menubar = gk.app_menu_bar(cx)
}

// The menu bar's bounds when it is laid out in a real window's frame (on_children_prepainted of the
// div that holds it): it is drawn though the platform has no menu bar of its own.
real_bar_bounds :: proc(a: ^App, cx: ^gk.Cx, bs: []gk.Bounds) {
	if a.bar_seen || len(bs) == 0 {
		return
	}
	a.bar_seen = true
	ms := gk.menus(cx)
	fmt.printf("platform real menubar drawn w>100=%v h>10=%v menus=%d first=%q\n", bs[0].size.width > 100, bs[0].size.height > 10,
		len(ms), ms[0].name if len(ms) > 0 else "")
	gk.after(cx, 50, gk.on(real_quit))
}

real_quit :: proc(a: ^App, cx: ^gk.Cx) {
	gk.quit(cx)
}

// --- an input's context menu under the offscreen driver (PROTOCOLS.md 11.32) -------------
//
// InputState::on_context_menu (upstream: "the right-click context menu" handler of the state) set in
// init. Drawn by the base element (gk.base_input, gpui-base's Input) the state keeps it: a right click
// runs it, after the release, with upstream's empty NativeMenu, the click's position and the input's
// capabilities (mode context-menu). Drawn by the kit's element (gk.input, gpui-component's Input) it
// is replaced each time the element draws: Input::render installs the kit's own handler (its
// Cut/Copy/Paste/Select All menu, or Input::context_menu's), so the app's never runs (mode
// context-menu-kit). Either way the right press reaches the state: its cursor moves into the text.

menu_fired :: proc(a: ^App, cx: ^gk.Cx, e: gk.Event_114) {
	a.fired += 1
	copy := gk.action(cx, "input::Copy")
	actions := 0
	for it in e.a0.items {
		if x, ok := it.(gk.Native_Menu_Item_Action); ok && x.action.id == copy.id {
			actions += 1
		}
	}
	fmt.printf("platform context-menu fired items=%d copy=%d at=%.0f,%.0f editable=%v disabled=%v cursor=%d\n", len(e.a0.items), actions,
		e.a2.x, e.a2.y, e.a1->is_editable(cx), e.a1->is_disabled(cx), a.inp->cursor(cx))
}

context_menu_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	box := gk.div()->w(gk.px(220))->h(gk.px(32))
	if a.mode == "context-menu" {
		box = box->child(gk.base_input(a.inp))
	} else {
		box = box->child(gk.input(a.inp))
	}
	return gk.v_flex(.Size_Full)->child(box)
}

context_menu_check :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("platform context-menu check mode=%s fired=%d cursor=%d\n", a.mode, a.fired, a.inp->cursor(cx))
}

// --- misuse ---------------------------------------------------------------------------------------

Other :: struct {
	n: int,
}

wrong :: proc(a: ^App, cx: ^gk.Cx, w: gk.Window) {}

misuse :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "release-twice":
		c := gk.prompt_for_paths(cx, {files = true}, gk.on(never))
		gk.release(cx, c)
		gk.release(cx, c) // stops: released already
	case "wrong-payload":
		gk.prompt_for_paths(cx, {files = true}, gk.on(wrong)) // stops: the handler takes a gk.Window
	case "menu-no-action":
		gk.set_menus(cx, {name = "File", items = {gk.Menu_Action{name = "Open"}}}) // stops: no action
	}
}

// --- the app ------------------------------------------------------------------------------------

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.described += 1
	fmt.printf("platform view described=%d done=%d\n", a.described, a.done)
	if a.mode == "request-in-view" {
		gk.prompt_for_paths(cx, {files = true}, gk.on(never)) // stops: a view procedure may not start a request
	}
	if a.mode == "cache-in-view" {
		gk.image_cache(cx) // stops: a view procedure may not make an image cache
	}
	switch a.mode {
	case "elements":
		return elements_view(a, cx)
	case "drag":
		return drag_view(a, cx)
	case "vlist":
		return vlist_view(a, cx)
	case "scope-zero":
		return gk.text_selection_scope(gk.div(), gk.Text_Selection_Scope{}) // stops: no scope
	case "real-menus":
		return gk.v_flex(.Size_Full)->child(gk.div()->h(gk.px(32))->on_children_prepainted(gk.on(real_bar_bounds))->
			child(gk.app_menu_bar_view(a.menubar)))
	case "context-menu", "context-menu-kit":
		return context_menu_view(a, cx)
	}
	root := gk.v_flex(.Size_Full)->child(gk.label(fmt.tprintf("done %d %s", a.done, a.last)))
	return root
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.open = gk.action(cx, "platform::Open")
	a.save = gk.action(cx, "platform::Save")
	switch a.mode {
	case "dialogs":
		gk.after(cx, 10, gk.on(ask_open))
		gk.after(cx, 30, gk.on(answer_all))
		gk.after(cx, 60, gk.on(check_log))
	case "cancel":
		gk.after(cx, 10, gk.on(ask_then_release))
		gk.after(cx, 20, gk.on(release_it))
		gk.after(cx, 30, gk.on(answer_late))
		gk.after(cx, 60, gk.on(check_log_cancel))
	case "race":
		gk.after(cx, 10, gk.on(ask_then_release))
		gk.after(cx, 20, gk.on(race))
		gk.after(cx, 60, gk.on(check_log_cancel))
	case "view-released":
		gk.after(cx, 5, gk.on(open_child))
		gk.after(cx, 20, gk.on(release_child))
		gk.after(cx, 30, gk.on(answer_late))
		gk.after(cx, 60, gk.on(check_log_cancel))
	case "menus":
		gk.after(cx, 10, gk.on(set_bar))
		gk.after(cx, 30, gk.on(read_bar))
	case "release-twice", "wrong-payload", "menu-no-action":
		gk.after(cx, 10, gk.on(misuse))
	case "elements":
		gk.after(cx, 10, gk.on(elements))
		gk.after(cx, 40, gk.on(elements_drawn))
	case "drag":
		gk.after(cx, 60, gk.on(drag_done))
	case "vlist":
		vlist_init(a, cx)
		gk.after(cx, 60, gk.on(vlist_check))
	case "cache-in-view":
		// misuse checked in view below
	case "real-dialogs":
		gk.after(cx, 100, gk.on(real_ask))
		gk.after(cx, 15000, gk.on(real_guard))
	case "real-url":
		gk.after(cx, 100, gk.on(real_url))
		gk.after(cx, 15000, gk.on(real_guard))
	case "real-menus":
		real_menus_init(a, cx)
		gk.after(cx, 15000, gk.on(real_guard))
	case "context-menu", "context-menu-kit":
		a.inp = gk.input_state(cx)->default_value("menu text")->build()
		a.inp->on_context_menu(cx, gk.on(menu_fired))
		gk.after(cx, 10, gk.on(context_menu_check))
	}
}

check_log_cancel :: proc(a: ^App, cx: ^gk.Cx) {
	l := gk.platform_log(cx)
	fmt.printf("platform log requests=%d delivered=%d discarded=%d waiting=%d scripted=%v done=%d\n",
		l.requests, l.delivered, l.discarded, l.waiting, l.scripted, a.done)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "platform", {400, 300}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	script := "size 400 300\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nstat\n"
	if strings.has_prefix(app.mode, "real-") {
		// a real window and the platform's own services (PROTOCOLS.md 11.32)
		gk.run(&app, start)
		fmt.printf("platform done mode=%s done=%d described=%d\n", app.mode, app.done, app.described)
		return
	}
	if app.mode == "context-menu" || app.mode == "context-menu-kit" {
		// a right click on the input's text, inside it (the cursor was at its end)
		script = "size 400 300\nframe\nframe\nrclick 40 16\nframe\nadvance 10\nframe\nstat\n"
	}
	if path, ok := os.lookup_env("GK_PLATFORM_SCRIPT", context.allocator); ok {
		// a test's own script instead of the mode's (diagnosis: shots of what a mode draws)
		data, err := os.read_entire_file(path, context.allocator)
		if err == nil {
			script = string(data)
		}
	}
	if app.mode == "drag" {
		// onto the target that takes 7, then from the source onto the one that takes only 8
		script = "size 400 300\nframe\nframe\ndrag 50 50 150 50\nframe\ndrag 50 50 250 50\nframe\nadvance 30\nframe\nadvance 40\nframe\nstat\n"
	}
	gk.run_headless(&app, script, start)
	fmt.printf("platform done mode=%s done=%d described=%d\n", app.mode, app.done, app.described)
}
