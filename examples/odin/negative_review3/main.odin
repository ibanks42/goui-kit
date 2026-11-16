// negative_review3: mistakes against the app protocols (actions, timers, windows: PROTOCOLS.md
// section 11), the dock's panels (11.7) and library objects (13.2), written by the review of
// 12 November 2026. Each must stop with a message and the app's line (exit 2) or behave
// correctly (exit 0); exit 0 with wrong content fails. Run offscreen with the diag library by
// gates/misuse.sh, one mode per case:
//
//   GK_NEG=rv-ok             timers, a second window opened and closed, an action, a panel: correct (exit 0)
//   GK_NEG=rv-timer-view     gk.after in a view procedure: must stop
//   GK_NEG=rv-timer-zero     a repeating timer of 0 ms: must stop
//   GK_NEG=rv-window-stale   the title of a window that was closed: must stop
//   GK_NEG=rv-keys-view      key bindings in a view procedure: must stop
//   GK_NEG=rv-action-forged  an action id the library never made dispatched: must stop
//   GK_NEG=rv-temp-later     a temporary library object (a dock's layout) used a frame later: must stop
//   GK_NEG=rv-panel-released a dock panel released and then changed: must stop
//   GK_NEG=rv-timer-state    a timer whose handler takes another state than the view's: must stop
package negative_review3

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

Other :: struct {
	n: int,
}

App :: struct {
	mode:    string,
	root:    gk.View,
	clicks:  int,
	fired:   int,
	drawn:   int,
	second:  gk.Window,
	second_v: gk.View,
	closed:  int,
	act:     gk.Action,
	acted:   int,
	dock:    gk.Dock_Area,
	panel:   gk.Dock_Panel,
	panel_v: gk.View,
	tree:    gk.Pane_Tree,
	other:   Other,
}

fired :: proc(a: ^App, cx: ^gk.Cx) {
	a.fired += 1
}

other_fired :: proc(o: ^Other, cx: ^gk.Cx) {
	o.n += 1
}

acted :: proc(a: ^App, cx: ^gk.Cx) {
	a.acted += 1
}

second_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->child(gk.label("second"))
}

panel_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->child(gk.label("panel"))
}

second_closed :: proc(a: ^App, cx: ^gk.Cx, w: gk.Window) {
	a.closed += 1
}

close_second :: proc(a: ^App, cx: ^gk.Cx) {
	gk.close_window(cx, a.second)
}

clicked :: proc(a: ^App, cx: ^gk.Cx) {
	a.clicks += 1
	switch a.mode {
	case "rv-ok":
		if a.clicks == 1 {
			gk.after(cx, 10, gk.on(fired))
			a.second_v = gk.view(a, nil, second_view)
			a.second = gk.open_window(cx, "second", {200, 100}, a.second_v)
			gk.on_window_closed(cx, a.second, gk.on(second_closed))
			gk.after(cx, 20, gk.on(close_second))
			gk.dispatch_action(cx, a.act)
			gk.panel_set(cx, a.panel, {name = "p", title = "Renamed"})
		}
	case "rv-timer-zero":
		gk.every(cx, 0, gk.on(fired))
	case "rv-window-stale":
		if a.clicks == 1 {
			a.second_v = gk.view(a, nil, second_view)
			a.second = gk.open_window(cx, "second", {200, 100}, a.second_v)
			gk.after(cx, 20, gk.on(close_second))
		} else {
			gk.set_window_title(cx, a.second, "gone") // the window closed a frame ago
		}
	case "rv-action-forged":
		gk.dispatch_action(cx, gk.Action{id = 4242})
	case "rv-temp-later":
		if a.clicks == 1 {
			a.tree, _ = a.dock->layout(cx, .Center).? // temporary: not retained
		} else {
			_ = a.tree->panels(cx)
		}
	case "rv-panel-released":
		gk.release(cx, a.panel)
		gk.panel_set(cx, a.panel, {name = "p", title = "after release"})
	case "rv-timer-state":
		gk.after(cx, 10, gk.on(other_fired)) // this handler takes ^Other; the view's state is ^App
	}
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.drawn += 1
	switch a.mode {
	case "rv-timer-view":
		gk.after(cx, 10, gk.on(fired))
	case "rv-keys-view":
		gk.bind_keys(cx, {"ctrl-k", a.act, ""})
	}
	body := gk.v_flex(.Size_Full)->id("all")->on_click(gk.on(clicked))->
		on_action(gk.act(a.act, gk.on(acted)))->
		child(gk.label(fmt.tprintf("clicks %d", a.clicks)))
	if a.dock.h != 0 {
		body = body->child(gk.div(.Flex_1, .W_Full)->child(gk.dock_view(a.dock)))
	}
	return body
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.act = gk.action(cx, "review::Act")
	_ = gk.on_app_action(cx, a.act, gk.on(acted))
	switch a.mode {
	case "rv-ok", "rv-temp-later", "rv-panel-released":
		a.dock = gk.dock_area_skinned(cx, "rv")
		a.panel_v = gk.view(a, nil, panel_view)
		a.panel = gk.panel(cx, a.panel_v, {name = "p", title = "P"})
		a.dock->set_center(cx, gk.dock_layout_tabs()->panel_view(a.panel))
	}
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "negative_review3", {400, 300}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_NEG", context.allocator)
	gk.run_headless(&app, "size 400 300\nframe\nclick 200 20\nframe\nadvance 30\nframe\nadvance 30\nframe\nclick 200 20\nframe\nadvance 30\nframe\nstat\n", start)
	fmt.printf("gk-neg done mode=%s drawn=%d clicks=%d fired=%d closed=%d acted=%d other=%d\n", app.mode, app.drawn,
		app.clicks, app.fired, app.closed, app.acted, app.other.n)
}
