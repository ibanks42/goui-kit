// review3: five attacks on the protocols built since milestone 2 (timers, windows, actions, dock
// panels, library objects behind handles), written by the review of 12 November 2026. Each runs
// offscreen (the diag library) under a gk-meter script; core/tests/run-review3.sh checks the
// lines and the exit code. None may crash, hang, or run app code against a view that is gone.
//
//   GK_PROTO=timer-after-view  a second window's view starts a repeating timer and an app-wide
//                              listener in its init; the window closes and the view is released:
//                              neither may run again with the released view's state
//   GK_PROTO=close-own-window  a key binding's handler in a second window closes that window and
//                              dispatches another action in the same turn
//   GK_PROTO=dock-panel-freed  the view of a docked panel is released while the dock shows it; the
//                              dock draws, is clicked, and is laid out again without the panel
//   GK_PROTO=reentrant-action  an app-wide listener dispatches its own action again, releases
//                              itself and adds another listener while it runs, in a turn with a
//                              window (a key) and in one without (a timer started before any window)
//   GK_PROTO=handle-after-owner a dock's layout and a dock context kept past the release of the dock
package review3_test

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

Child :: struct {
	ticks:     int,
	wrong:     int,
	pings:     int,
	dead:      bool,
	timer:     gk.Timer,
	listener:  gk.Action_Listener,
	ping:      gk.Action,
}

App :: struct {
	mode:      string,
	root:      gk.View,
	frame:     int,
	// timer-after-view
	child:     Child,
	child_v:   gk.View,
	win:       gk.Window,
	final:     int,
	// close-own-window
	close:     gk.Action,
	ping:      gk.Action,
	focus2:    gk.Focus_Handle,
	second:    gk.Window,
	second_v:  gk.View,
	closes:    int,
	closed:    int,
	pings:     int,
	// dock-panel-freed, handle-after-owner
	dock:      gk.Dock_Area,
	pa, pb:    gk.Dock_Panel,
	pa_v, pb_v: gk.View,
	a_drawn:   int,
	a_after:   int,
	freed:     bool,
	dock_gone: bool,
	tree:      gk.Pane_Tree,
	renderer:  gk.Dock_Renderer,
	dock_ctx:  gk.Dock_Context,
	has_ctx:   bool,
	// reentrant-action
	l1, l2:    gk.Action_Listener,
	n1, n2:    int,
	depth:     int,
	nested:    int,
	phase:     int,
	limit:     int,
}

// --- timer-after-view ---------------------------------------------------------------------------

child_tick :: proc(c: ^Child, cx: ^gk.Cx) {
	if c.dead {
		c.wrong += 1
		fmt.println("review3 WRONG: a timer of a released view ran")
		return
	}
	c.ticks += 1
}

child_ping :: proc(c: ^Child, cx: ^gk.Cx) {
	if c.dead {
		c.wrong += 1
		fmt.println("review3 WRONG: a listener of a released view ran")
		return
	}
	c.pings += 1
}

child_init :: proc(c: ^Child, cx: ^gk.Cx) {
	c.timer = gk.every(cx, 20, gk.on(child_tick))
	c.ping = gk.action(cx, "review::Ping")
	c.listener = gk.on_app_action(cx, c.ping, gk.on(child_ping))
}

child_view :: proc(c: ^Child, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->child(gk.label(fmt.tprintf("child ticks %d", c.ticks)))
}

open_child :: proc(a: ^App, cx: ^gk.Cx) {
	a.child_v = gk.view(&a.child, child_init, child_view)
	a.win = gk.open_window(cx, "child", {300, 200}, a.child_v)
	fmt.printf("review3 child opened windows=%d\n", len(gk.windows(cx)))
}

ping_child :: proc(a: ^App, cx: ^gk.Cx) {
	gk.dispatch_action(cx, a.child.ping)
}

drop_child :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("review3 child before release ticks=%d pings=%d\n", a.child.ticks, a.child.pings)
	gk.close_window(cx, a.win)
	gk.release(cx, a.child_v)
	a.child.dead = true
	gk.dispatch_action(cx, a.child.ping) // the released view's listener must not hear it
}

final_fired :: proc(a: ^App, cx: ^gk.Cx) {
	a.final += 1
}

report_child :: proc(a: ^App, cx: ^gk.Cx) {
	// the app still holds the listener's and the timer's handles: releasing them now does nothing
	// wrong, and a handler kept afterwards (in a slot the view's handlers had) still fires
	gk.release(cx, a.child.listener)
	gk.release(cx, a.child.timer)
	gk.after(cx, 10, gk.on(final_fired))
	gk.dispatch_action(cx, a.child.ping)
	fmt.printf("review3 timer-after-view ticks=%d pings=%d wrong=%d windows=%d\n", a.child.ticks, a.child.pings,
		a.child.wrong, len(gk.windows(cx)))
}

timer_after_view_init :: proc(a: ^App, cx: ^gk.Cx) {
	gk.after(cx, 10, gk.on(open_child))
	gk.after(cx, 60, gk.on(ping_child))
	gk.after(cx, 120, gk.on(drop_child))
	gk.after(cx, 220, gk.on(report_child))
}

// --- close-own-window ---------------------------------------------------------------------------

close_self :: proc(a: ^App, cx: ^gk.Cx) {
	a.closes += 1
	gk.close_window(cx) // the turn's window: the one this handler's key press went to
	gk.dispatch_action(cx, a.ping)
	fmt.printf("review3 close-self n=%d\n", a.closes)
}

pinged :: proc(a: ^App, cx: ^gk.Cx) {
	a.pings += 1
}

second_closed :: proc(a: ^App, cx: ^gk.Cx, w: gk.Window) {
	a.closed += 1
	fmt.printf("review3 second closed n=%d same=%v\n", a.closed, w == a.second)
}

second_init :: proc(a: ^App, cx: ^gk.Cx) {
	a.focus2 = gk.focus_handle(cx)
	gk.focus(cx, a.focus2)
}

second_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->key_context("Second")->track_focus(a.focus2)->
		on_action(gk.act(a.close, gk.on(close_self)))->child(gk.label("second"))
}

open_second :: proc(a: ^App, cx: ^gk.Cx) {
	a.second_v = gk.view(a, second_init, second_view)
	a.second = gk.open_window(cx, "second", {300, 200}, a.second_v)
	gk.on_window_closed(cx, a.second, gk.on(second_closed))
	fmt.printf("review3 second opened windows=%d\n", len(gk.windows(cx)))
}

report_close :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("review3 close-own-window closes=%d closed=%d pings=%d windows=%d\n", a.closes, a.closed, a.pings,
		len(gk.windows(cx)))
}

close_own_init :: proc(a: ^App, cx: ^gk.Cx) {
	a.close = gk.action(cx, "review::CloseWindow")
	a.ping = gk.action(cx, "review::Ping")
	gk.bind_keys(cx, {"ctrl-w", a.close, "Second"})
	_ = gk.on_app_action(cx, a.ping, gk.on(pinged))
	gk.after(cx, 10, gk.on(open_second))
	gk.after(cx, 200, gk.on(report_close))
}

// --- dock-panel-freed ---------------------------------------------------------------------------

pa_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.a_drawn += 1
	if a.freed {
		a.a_after += 1
	}
	return gk.div(.Size_Full)->id("pa")->on_click(gk.on(pa_clicked))->child(gk.label("panel a"))
}

pa_clicked :: proc(a: ^App, cx: ^gk.Cx) {
	if a.freed {
		fmt.println("review3 WRONG: a click reached a released panel view")
	}
}

pb_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->id("pb")->child(gk.label("panel b"))
}

make_panels :: proc(a: ^App, cx: ^gk.Cx) {
	a.pa_v = gk.view(a, nil, pa_view)
	a.pb_v = gk.view(a, nil, pb_view)
	a.pa = gk.panel(cx, a.pa_v, {name = "a", title = "A"})
	a.pb = gk.panel(cx, a.pb_v, {name = "b", title = "B"})
}

free_panel_view :: proc(a: ^App, cx: ^gk.Cx) {
	gk.release(cx, a.pa_v) // still docked, and the active tab
	a.freed = true
	gk.notify(cx, a.root)
	fmt.printf("review3 panel view released drawn=%d\n", a.a_drawn)
}

relayout :: proc(a: ^App, cx: ^gk.Cx) {
	gk.release(cx, a.pa) // the panel itself too, while the dock still holds it
	gk.set_center(a.dock, cx, gk.dock_layout_tabs()->panel_view(a.pb))
	gk.notify(cx, a.root)
}

report_dock :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("review3 dock-panel-freed described_after=%d frames=%d\n", a.a_after, a.frame)
}

dock_freed_init :: proc(a: ^App, cx: ^gk.Cx) {
	a.dock = gk.dock_area_skinned(cx, "review3")
	make_panels(a, cx)
	gk.set_center(a.dock, cx, gk.dock_layout_tabs()->panel_view(a.pa)->panel_view(a.pb))
	gk.after(cx, 30, gk.on(free_panel_view))
	gk.after(cx, 90, gk.on(relayout))
	gk.after(cx, 150, gk.on(report_dock))
}

// --- reentrant-action ---------------------------------------------------------------------------

ping1 :: proc(a: ^App, cx: ^gk.Cx) {
	a.depth += 1
	if a.depth > 1 {
		a.nested += 1
	}
	a.n1 += 1
	if a.n1 == 2 {
		gk.release(cx, a.l1) // itself, while it runs
		a.l2 = gk.on_app_action(cx, a.ping, gk.on(ping2)) // another one, while the dispatch runs
	}
	if a.n1 + a.n2 < a.limit {
		gk.dispatch_action(cx, a.ping)
	}
	a.depth -= 1
}

ping2 :: proc(a: ^App, cx: ^gk.Cx) {
	a.depth += 1
	if a.depth > 1 {
		a.nested += 1
	}
	a.n2 += 1
	if a.n1 + a.n2 < a.limit {
		gk.dispatch_action(cx, a.ping)
	}
	a.depth -= 1
}

report_window_phase :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("review3 reentrant window n1=%d n2=%d nested=%d\n", a.n1, a.n2, a.nested)
}

no_window_ping :: proc(a: ^App, cx: ^gk.Cx) {
	// kept in the start turn, before any window: fires without one, so the dispatch has none
	a.limit = 12
	gk.dispatch_action(cx, a.ping)
}

report_reentrant :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("review3 reentrant-action n1=%d n2=%d nested=%d\n", a.n1, a.n2, a.nested)
}

reentrant_init :: proc(a: ^App, cx: ^gk.Cx) {
	a.ping = gk.action(cx, "review::Ping")
	a.limit = 6
	gk.bind_keys(cx, {"ctrl-p", a.ping, ""})
	a.l1 = gk.on_app_action(cx, a.ping, gk.on(ping1))
	gk.after(cx, 100, gk.on(report_window_phase))
	gk.after(cx, 400, gk.on(report_reentrant))
}

// --- handle-after-owner -------------------------------------------------------------------------

asked :: proc(a: ^App, cx: ^gk.Cx, q: gk.Dock_Render) {
	#partial switch q.hook {
	case .Dock:
		if !a.has_ctx {
			a.dock_ctx = gk.retain(cx, q.dock)
			a.has_ctx = true
		}
	}
	gk.dock_render_answer(cx, q.renderer, q.hook, q.key, {}) // upstream's own drawing
}

keep_and_drop :: proc(a: ^App, cx: ^gk.Cx) {
	tree, ok := gk.layout(a.dock, cx, .Left).?
	if ok {
		a.tree = gk.retain(cx, tree)
	}
	fmt.printf("review3 owner kept tree=%v ctx=%v panels=%d\n", ok, a.has_ctx, ok ? len(gk.panels(a.tree, cx)) : -1)
	a.dock_gone = true
	gk.notify(cx, a.root) // the view stops showing the dock before it is released
	gk.release(cx, a.dock)
}

use_after_owner :: proc(a: ^App, cx: ^gk.Cx) {
	n := -1
	if a.tree.h != 0 {
		n = len(gk.panels(a.tree, cx))
	}
	open := false
	if a.has_ctx {
		open = gk.is_open(a.dock_ctx, cx)
		gk.dock_context_toggle(a.dock_ctx, cx) // the dock it toggles is gone
	}
	fmt.printf("review3 handle-after-owner panels=%d ctx_open=%v\n", n, open)
}

handle_owner_init :: proc(a: ^App, cx: ^gk.Cx) {
	make_panels(a, cx)
	a.renderer = gk.dock_renderer(cx, {.Dock}, gk.on(asked))
	a.dock = gk.dock_area(cx, "review3-owner", nil)->with_renderer(a.renderer)->build()
	gk.set_center(a.dock, cx, gk.dock_layout_tabs()->panel(a.pb))
	gk.set_dock(a.dock, cx, .Left, gk.dock_layout_tabs()->panel(a.pa))
	gk.after(cx, 40, gk.on(keep_and_drop))
	gk.after(cx, 100, gk.on(use_after_owner))
}

// --- the app ------------------------------------------------------------------------------------

init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "timer-after-view":
		timer_after_view_init(a, cx)
	case "close-own-window":
		close_own_init(a, cx)
	case "dock-panel-freed":
		dock_freed_init(a, cx)
	case "reentrant-action":
		reentrant_init(a, cx)
	case "handle-after-owner":
		handle_owner_init(a, cx)
	}
}

on_frame :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.frame += 1
	root := gk.v_flex(.Size_Full)->child(gk.label(fmt.tprintf("review3 %s", a.mode)))
	if a.dock.h != 0 && !a.dock_gone {
		root = root->child(gk.div(.Flex_1, .W_Full)->child(gk.dock_view(a.dock)))
	}
	return root
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	if a.mode == "reentrant-action" {
		gk.after(cx, 200, gk.on(no_window_ping)) // kept in the start turn: no window, the app's state
	}
	a.root = gk.view(a, init, on_frame)
	gk.open_window(cx, "review3", {640, 480}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	script := "size 640 480\nframe\n"
	adv := "advance 25\nframe\n"
	switch app.mode {
	case "close-own-window":
		script = fmt.aprint(script, adv, adv, "windows\nwindow 1\nframe\nkey ctrl-w\nwindow 0\n", adv, adv, adv, adv,
			adv, adv, adv, adv, "windows\nframe\nstat\n", sep = "")
	case "dock-panel-freed":
		script = fmt.aprint(script, adv, adv, "click 320 240\n", adv, adv, "click 320 240\n", adv, adv, adv, adv, "frame\nstat\n", sep = "")
	case "reentrant-action":
		script = fmt.aprint(script, "key ctrl-p\nframe\nframe\n", sep = "")
		for _ in 0 ..< 18 {
			script = fmt.aprint(script, adv, sep = "")
		}
		script = fmt.aprint(script, "stat\n", sep = "")
	case:
		for _ in 0 ..< 14 {
			script = fmt.aprint(script, adv, sep = "")
		}
		script = fmt.aprint(script, "stat\n", sep = "")
	}
	gk.run_headless(&app, script, start)
	fmt.printf("review3 done mode=%s frames=%d final=%d\n", app.mode, app.frame, app.final)
}
