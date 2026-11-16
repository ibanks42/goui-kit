// app: the protocol tests of the app family (docs/PROTOCOLS.md section 11): actions and key
// bindings dispatched to Odin handlers, timers between frames, several windows, the clipboard.
// Each mode runs offscreen (the diag library) under a gk-meter script and prints lines that
// core/tests/run-app.sh checks, with the exit code.
//
//   GK_PROTO=actions | timers | windows | clipboard | ambient | builtin-app-listener | bad-keys |
//            builtin-on-action | close-veto | quit-hook | reduce-motion | window-icon | bad-icon
package app_test

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

App :: struct {
	mode:      string,
	root:      gk.View,
	focus:     gk.Focus_Handle,
	save:      gk.Action,
	other:     gk.Action,
	cancel:    gk.Action,
	up:        gk.Action,
	global:    gk.Action_Listener,
	saves:     int,
	others:    int,
	cancels:   int,
	// timers
	once:      int,
	ticks:     int,
	never:     int,
	ticker:    gk.Timer,
	described: int,
	// windows
	second:    gk.Window,
	second_v:  gk.View,
	closed:    int,
	frame:     int,
	// window close, quit, motion, icon and app id (PROTOCOLS.md 11.38)
	allow:     bool,
	asked:     int,
	veto:      gk.Action_Listener,
	quits:     int,
	motion:    [3]bool,
}

// A 2 x 2 PNG (red and green), a window's icon.
ICON_PNG := [?]u8{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 2, 0, 0, 0, 2, 8, 6, 0, 0, 0, 114, 182, 13, 36, 0, 0, 0, 16, 73, 68, 65, 84, 120, 156, 99, 248, 207, 192, 240, 31, 10, 33, 20, 0, 69, 204, 7, 249, 102, 203, 255, 57, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130}

// --- actions ------------------------------------------------------------------------------------

saved :: proc(a: ^App, cx: ^gk.Cx, act: gk.Action) {
	a.saves += 1
	keys, has := gk.keystrokes(cx, a.save)
	fmt.printf("app saved n=%d action_ok=%v keys=%q has_keys=%v available=%v\n", a.saves, act == a.save, keys, has,
		gk.action_available(cx, a.save))
	if a.saves == 1 {
		gk.dispatch_action(cx, a.other) // after this handler, as if ctrl-o had been pressed
	}
}

other_global :: proc(a: ^App, cx: ^gk.Cx, act: gk.Action) {
	a.others += 1
	fmt.printf("app other n=%d action_ok=%v\n", a.others, act == a.other)
	if a.others == 2 {
		gk.release(cx, a.global) // the next ctrl-o reaches nobody
		fmt.println("app other released")
	}
}

cancelled :: proc(a: ^App, cx: ^gk.Cx, act: gk.Action) {
	a.cancels += 1
	fmt.printf("app cancel n=%d builtin=%v same=%v\n", a.cancels, gk.is_builtin(act), act == a.cancel)
}

captured :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.println("app capture save")
}

actions_init :: proc(a: ^App, cx: ^gk.Cx) {
	a.save = gk.action(cx, "app::Save")
	a.other = gk.action(cx, "app::Other")
	a.cancel = gk.action(cx, "ui::Cancel")
	a.up = gk.action(cx, "ui::SelectUp")
	again := gk.action(cx, "app::Save")
	fmt.printf("app actions save_builtin=%v cancel_builtin=%v same_name_same_id=%v distinct=%v\n",
		gk.is_builtin(a.save), gk.is_builtin(a.cancel), again == a.save, a.save != a.other)
	switch a.mode {
	case "builtin-app-listener":
		gk.on_app_action(cx, a.cancel, gk.on(cancelled)) // stops: app-wide listeners take app actions
	case "bad-keys":
		gk.bind_keys(cx, {"ctrl-b", a.save, "Editor &&"}) // a context that does not parse: stops when applied
	}
	gk.bind_keys(cx, {"ctrl-s", a.save, "Editor"}, {"ctrl-o", a.other, ""}, {"ctrl-x", a.cancel, ""})
	a.global = gk.on_app_action(cx, a.other, gk.on(other_global))
	a.focus = gk.focus_handle(cx)
	gk.focus(cx, a.focus)
}

actions_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	root := gk.v_flex(.Size_Full, .P_4)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		key_context("Editor")->track_focus(a.focus)->
		capture_action(gk.act(a.save, gk.quiet(gk.on(captured))))->
		on_action(gk.act(a.save, gk.on(saved)))->
		on_boxed_action(a.cancel, gk.on(cancelled))->
		child(gk.label(fmt.tprintf("saves %d others %d cancels %d", a.saves, a.others, a.cancels)))
	if a.mode == "builtin-on-action" {
		root = root->on_action(gk.act(a.cancel, gk.on(saved))) // stops: on_action takes app actions
	}
	return root
}

// --- timers -------------------------------------------------------------------------------------

once :: proc(a: ^App, cx: ^gk.Cx) {
	a.once += 1
	fmt.printf("app timer once n=%d\n", a.once)
	gk.release(cx, gk.Timer{}) // nothing: a zero timer is no timer
}

tick :: proc(a: ^App, cx: ^gk.Cx) {
	a.ticks += 1
	fmt.printf("app timer tick n=%d\n", a.ticks)
	if a.ticks == 3 {
		gk.release(cx, a.ticker)
		fmt.println("app timer ticker released")
	}
}

never :: proc(a: ^App, cx: ^gk.Cx) {
	a.never += 1
	fmt.println("app timer never fired (wrong)")
}

timers_init :: proc(a: ^App, cx: ^gk.Cx) {
	t1 := gk.after(cx, 30, gk.on(once))
	a.ticker = gk.every(cx, 20, gk.on(tick))
	t3 := gk.after(cx, 40, gk.on(never))
	gk.release(cx, t3) // cancelled before it fires
	_ = t1
}

timers_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.described += 1
	fmt.printf("app timers view ticks=%d once=%d\n", a.ticks, a.once)
	return gk.div()->child(gk.label(fmt.tprintf("ticks %d", a.ticks)))
}

// --- windows ------------------------------------------------------------------------------------

second_closed :: proc(a: ^App, cx: ^gk.Cx, w: gk.Window) {
	a.closed += 1
	fmt.printf("app window closed n=%d same=%v\n", a.closed, w == a.second)
}

close_second :: proc(a: ^App, cx: ^gk.Cx) {
	info := gk.window_info(cx, a.second)
	fmt.printf("app second info title=%q w=%.0f h=%.0f windows=%d\n", info.title, info.width, info.height, len(gk.windows(cx)))
	gk.close_window(cx, a.second)
}

read_first :: proc(a: ^App, cx: ^gk.Cx) {
	ws := gk.windows(cx)
	fmt.printf("app windows after close n=%d\n", len(ws))
	if len(ws) > 0 {
		gk.set_window_title(cx, ws[0], "Renamed")
	}
}

read_title :: proc(a: ^App, cx: ^gk.Cx) {
	ws := gk.windows(cx)
	info := gk.window_info(cx, ws[0])
	fmt.printf("app first title=%q\n", info.title)
}

second_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	fmt.println("app second view described")
	return gk.div(.Size_Full)->child(gk.label("second window"))
}

open_second :: proc(a: ^App, cx: ^gk.Cx) {
	a.second_v = gk.view(a, nil, second_view)
	// the kit's title bar options (TitleBar::window_options, PROTOCOLS.md 13.21) under the title the app gives
	a.second = gk.open_window_with(cx, {width = 300, height = 200, title = "Second", min_width = 100, min_height = 80, kit_title_bar = true}, a.second_v)
	gk.on_window_closed(cx, a.second, gk.on(second_closed))
	fmt.printf("app windows opened n=%d\n", len(gk.windows(cx)))
}

windows_init :: proc(a: ^App, cx: ^gk.Cx) {
	gk.after(cx, 10, gk.on(open_second))
	gk.after(cx, 70, gk.on(close_second))
	gk.after(cx, 160, gk.on(read_first))
	gk.after(cx, 260, gk.on(read_title))
}

// --- the clipboard ---------------------------------------------------------------------------------

clip_write :: proc(a: ^App, cx: ^gk.Cx) {
	gk.clipboard_write(cx, "hello gk")
}

clip_read :: proc(a: ^App, cx: ^gk.Cx) {
	text, ok := gk.clipboard_read(cx)
	fmt.printf("app clipboard read ok=%v text=%q\n", ok, text)
}

clipboard_init :: proc(a: ^App, cx: ^gk.Cx) {
	gk.after(cx, 10, gk.on(clip_write))
	gk.after(cx, 60, gk.on(clip_read))
}

// --- ambient calls -------------------------------------------------------------------------------

run_ambient :: proc(a: ^App, cx: ^gk.Cx) {
	n := ambient_all(a, cx) // gen_ambient.odin: every ambient call once
	fmt.printf("app ambient called=%d want=%d dialog=%v\n", n, AMBIENT_CALLS, gk.has_active_dialog(cx))
}

ambient_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	c := gk.blue(cx, 500) // a call that returns a value may run in a view procedure
	fmt.printf("app ambient view blue=%v\n", c)
	return gk.div(.Size_Full)->child(gk.label("ambient"))
}

// --- PROTOCOLS.md 11.38: a window's close asked, the app's quit, reduced motion, a
// window's icon and app id ----------------------------------------------------------------------------

may_close :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	a.asked += 1
	fmt.printf("app asked may close n=%d answer=%v\n", a.asked, a.allow)
	return a.allow
}

says_nothing :: proc(a: ^App, cx: ^gk.Cx) {
	a.asked += 1
	fmt.printf("app asked by a handler without an answer n=%d\n", a.asked)
}

veto_check :: proc(a: ^App, cx: ^gk.Cx) {
	w := gk.windows(cx)[0]
	a.veto = gk.on_window_should_close(cx, w, gk.on(may_close))
	first := gk.test_should_close(cx, w) // the handler says no
	a.allow = true
	second := gk.test_should_close(cx, w) // now yes
	gk.release(cx, a.veto)
	third := gk.test_should_close(cx, w) // no handler: it closes
	a.veto = gk.on_window_should_close(cx, {}, gk.on(says_nothing)) // the turn's window; no answer: it closes
	fourth := gk.test_should_close(cx, w)
	fmt.printf("app should close first=%v second=%v third=%v fourth=%v asked=%d windows=%d\n", first, second, third, fourth,
		a.asked, len(gk.windows(cx)))
}

quit_hook :: proc(a: ^App, cx: ^gk.Cx) {
	a.quits += 1
	fmt.printf("app quit hook ran n=%d windows=%d\n", a.quits, len(gk.windows(cx)))
}

quit_released :: proc(a: ^App, cx: ^gk.Cx) {
	a.quits += 1 // counted, so "quits=1" shows it never ran
	fmt.println("app quit hook ran after its release (wrong)")
}

motion_read :: proc(a: ^App, cx: ^gk.Cx) {
	a.motion[1] = gk.reduce_motion(cx)
	gk.set_reduce_motion(cx, false)
	gk.after(cx, 20, gk.on(motion_read_again))
}

motion_read_again :: proc(a: ^App, cx: ^gk.Cx) {
	a.motion[2] = gk.reduce_motion(cx)
	fmt.printf("app reduce motion start=%v set=%v cleared=%v\n", a.motion[0], a.motion[1], a.motion[2])
}

motion_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	fmt.printf("app reduce motion in view=%v\n", gk.reduce_motion(cx)) // a read a view procedure may make
	return gk.div(.Size_Full)->child(gk.label("motion"))
}

open_icon_window :: proc(a: ^App, cx: ^gk.Cx) {
	a.second_v = gk.view(a, nil, second_view)
	icon := ICON_PNG
	a.second = gk.open_window_with(cx, {width = 300, height = 200, title = "Icon", app_id = "gk-test", icon = icon[:]}, a.second_v)
	fmt.printf("app icon window opened n=%d\n", len(gk.windows(cx)))
}

open_bad_icon :: proc(a: ^App, cx: ^gk.Cx) {
	a.second_v = gk.view(a, nil, second_view)
	bad := [4]u8{1, 2, 3, 4}
	gk.open_window_with(cx, {title = "Bad", icon = bad[:]}, a.second_v) // stops: not a PNG
	fmt.println("app bad icon accepted (wrong)")
}

port_init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "close-veto":
		gk.after(cx, 10, gk.on(veto_check))
	case "quit-hook":
		gk.on_app_quit(cx, gk.on(quit_hook))
		gone := gk.on_app_quit(cx, gk.on(quit_released))
		gk.release(cx, gone)
	case "reduce-motion":
		a.motion[0] = gk.reduce_motion(cx)
		gk.set_reduce_motion(cx, true)
		gk.after(cx, 10, gk.on(motion_read))
	case "window-icon":
		gk.after(cx, 10, gk.on(open_icon_window))
	case "bad-icon":
		gk.after(cx, 10, gk.on(open_bad_icon))
	}
}

// --- the app ------------------------------------------------------------------------------------

plain_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->child(gk.label("app"))
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "timers":
		timers_init(a, cx)
	case "windows":
		windows_init(a, cx)
	case "clipboard":
		clipboard_init(a, cx)
	case "ambient":
		gk.after(cx, 10, gk.on(run_ambient))
	case "close-veto", "quit-hook", "reduce-motion", "window-icon", "bad-icon":
		port_init(a, cx)
	case:
		actions_init(a, cx)
	}
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	render := actions_view
	switch a.mode {
	case "timers":
		render = timers_view
	case "windows", "clipboard":
		render = plain_view
	case "ambient":
		render = ambient_view
	case "reduce-motion":
		render = motion_view
	case "close-veto", "quit-hook", "window-icon", "bad-icon":
		render = plain_view
	}
	a.root = gk.view(a, init, render)
	gk.open_window(cx, "app", {640, 480}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	script := "size 640 480\nframe\n"
	switch app.mode {
	case "actions", "builtin-on-action":
		// ctrl-s twice (the first dispatches app::Other), ctrl-o twice (the second after the
		// app-wide listener was released: nobody), ctrl-x the built-in ui::Cancel
		script = fmt.aprint(script, "key ctrl-s\n", "key ctrl-o\n", "key ctrl-s\n", "key ctrl-o\n", "key ctrl-x\n", "frame\nstat\n", sep = "")
	case "timers", "clipboard", "ambient", "close-veto", "reduce-motion", "window-icon", "bad-icon":
		// the offscreen clock is simulated: advance moves it on, and due timers fire then
		script = fmt.aprint(script, "advance 25\nadvance 25\nadvance 25\nadvance 25\nadvance 25\nadvance 25\nframe\nstat\n", sep = "")
	case "windows":
		script = fmt.aprint(script, "advance 30\nwindows\nwindow 1\nframe\nwindow 0\nadvance 60\nwindows\nadvance 100\nadvance 120\nframe\nstat\n", sep = "")
	case:
		script = fmt.aprint(script, "frame\nframe\n", sep = "")
	}
	gk.run_headless(&app, script, start)
	if app.mode == "quit-hook" {
		fmt.printf("app after the run quits=%d\n", app.quits) // the hook ran as the offscreen app shut down
	}
	fmt.printf("app done mode=%s saves=%d others=%d cancels=%d once=%d ticks=%d never=%d closed=%d\n",
		app.mode, app.saves, app.others, app.cancels, app.once, app.ticks, app.never, app.closed)
}
