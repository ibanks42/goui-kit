package gk

// The app-level protocols (docs/PROTOCOLS.md section 11): actions and key bindings, timers,
// windows and the clipboard. The library side is core/src/app.rs.
//
//   save := gk.action(cx, "editor::Save")                  // an app action (any name GPUI does not know)
//   gk.bind_keys(cx, {"ctrl-s", save, "Editor"})            // context "" = everywhere
//   gk.div()->key_context("Editor")->on_action(gk.act(save, gk.on(do_save)))   // on the focus path
//   gk.on_app_action(cx, save, gk.on(do_save))              // app-wide, at the end of the dispatch
//   gk.dispatch_action(cx, save)                             // as if its keys had been pressed
//   t := gk.every(cx, 500, gk.on(tick))                      // between frames; gk.release(cx, t) stops it
//   w := gk.open_window(cx, "Two", {400, 300}, view)
//   w := gk.open_window_with(cx, {width = 400, height = 300, title = "Two", min_width = 200}, view)
//
// A name GPUI's action registry knows ("menu::Confirm", the kit's actions) is that built-in
// action: key bindings, menus and dispatch take it, an element listens to it with
// on_boxed_action, and its id has ACTION_BUILTIN set.

import "base:runtime"
import "core:fmt"

ACTION_BUILTIN :: 0x8000_0000

@(init, private)
app_shapes :: proc "contextless" () {
	register_shape(SHAPE_ACTION, Action, nil)
	register_shape(SHAPE_WINDOW, Window, nil)
}

// action names an action and returns its id: the same name (and data) gives the same id.
// data is the JSON a built-in action with fields takes ("" for none).
action :: proc(cx: ^Cx, name: string, data := "", loc := #caller_location) -> Action {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 16 + len(name) + len(data), context.temp_allocator)
	put_u16(&req, C_ACTION_NEW)
	put_str(&req, name)
	put_str(&req, data)
	r := Reader{must_call(req[:], loc), 0}
	return Action{rd_u32(&r)}
}

// is_builtin reports whether an action is GPUI's or the kit's rather than the app's.
is_builtin :: proc(a: Action) -> bool {
	return a.id & ACTION_BUILTIN != 0
}

// Action_Handler is what an element's on_action and capture_action take: the action to listen
// for and the handler (which may take the Action). Made with gk.act.
//   gk.div()->key_context("Editor")->on_action(gk.act(save, gk.on(do_save)))
Action_Handler :: struct {
	action:  Action,
	handler: Handler,
}

act :: proc(a: Action, h: Handler) -> Action_Handler {
	return {a, h}
}

@(private)
put_action_handler :: proc(b: ^Tape, a: Action_Handler) {
	if a.action.id == 0 {
		stop(a.handler.loc, "on_action: no action given (make one with gk.action)")
	}
	if is_builtin(a.action) {
		stop(a.handler.loc, "on_action listens for the app's own actions; listen to a built-in action with on_boxed_action")
	}
	put_u32(b, a.action.id)
	put_u32(b, register(a.handler, SHAPE_ACTION))
}

// Key_Binding: keystrokes ("ctrl-s", "ctrl-k ctrl-c" for a sequence), the action, and the key
// context it applies in, when_in (a predicate: "Editor", "Editor && !Readonly"; "" for everywhere).
Key_Binding :: struct {
	keys:    string,
	action:  Action,
	when_in: string,
}

// bind_keys adds key bindings to the app's keymap. A later binding of the same keys wins.
bind_keys :: proc(cx: ^Cx, bindings: ..Key_Binding, loc := #caller_location) {
	t := check_cx(cx, loc)
	n := 8
	for b in bindings {
		n += 16 + len(b.keys) + len(b.when_in)
		if b.action.id == 0 {
			stop(loc, "bind_keys: the binding %q has no action (make one with gk.action)", b.keys)
		}
	}
	req := make(Tape, 0, n, context.temp_allocator)
	put_u16(&req, C_BIND_KEYS)
	put_u32(&req, u32(len(bindings)))
	for b in bindings {
		put_str(&req, b.keys)
		put_u32(&req, b.action.id)
		put_str(&req, b.when_in)
	}
	defer_op(t, req[:], loc)
}

// clear_key_bindings removes every key binding, the kit's included.
clear_key_bindings :: proc(cx: ^Cx, loc := #caller_location) {
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_CLEAR_KEY_BINDINGS)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// Action_Listener is an app-wide action listener; gk.release ends it.
Action_Listener :: struct {
	h:    u64,
	slot: u32,
}

// on_app_action runs a handler whenever the app action is dispatched and no element on the
// focus path took it (GPUI's global action listeners). The handler may take the Action.
on_app_action :: proc(cx: ^Cx, a: Action, h: Handler, loc := #caller_location) -> Action_Listener {
	t := check_cx(cx, loc)
	if is_builtin(a) {
		stop(loc, "on_app_action takes the app's own actions; listen to a built-in action on an element with on_boxed_action")
	}
	flush(t, loc)
	slot, gen := keep_handler(cx, h, SHAPE_ACTION, loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_ON_ACTION)
	put_u32(&req, a.id)
	put_u32(&req, slot)
	put_u32(&req, gen)
	r := Reader{must_call(req[:], loc), 0}
	return Action_Listener{rd_u64(&r), slot}
}

// stop_propagation, in the handler of a click, a mouse or a key event, keeps the event from the
// elements around the one whose handler runs (GPUI's cx.stop_propagation()): a click on a checkbox
// or a button in a list row then does not also select the row.
//   tick :: proc(a: ^App, cx: ^gk.Cx, task: int) {gk.stop_propagation(cx); a.tasks[task].done ~= true}
stop_propagation :: proc(cx: ^Cx, loc := #caller_location) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.stop_propagation in a view procedure: call it in the handler of the event")
	}
	flush(t, loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_STOP_PROPAGATION)
	_ = must_call(req[:], loc)
}

// shortcut is a keyboard shortcut in one call: it names an app action, binds `keys` to it and
// runs the handler when they are pressed, wherever the focus is (an app-wide listener, so it keeps
// working after a dialog or a click has moved the focus). It returns the action, for a menu item
// (the menu shows the keys). when_in limits the keys to a key context: "Tasks" only while the
// focus is inside an element with key_context("Tasks"). The same procedure given again adds keys
// to its action. Given in a view's init procedure, the handler redraws that view.
//   a.new_task = gk.shortcut(cx, "ctrl-n", gk.on(new_task))
//   gk.shortcut(cx, "delete", gk.on(delete_task), when_in = "Tasks")
//   gk.popup_menu()->menu("New task", a.new_task)            // shows "Ctrl+N"
// keys "" makes the action and its listener without keys (a menu item no key reaches).
shortcut :: proc(cx: ^Cx, keys: string, h: Handler, when_in := "", loc := #caller_location) -> Action {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not make shortcuts: make them in init or in a handler")
	}
	if h.call == nil {
		stop(loc, "gk.shortcut: no handler given: write gk.shortcut(cx, \"ctrl-n\", gk.on(procedure))")
	}
	a, known := shortcut_actions[h.fn]
	if !known {
		// One action per procedure: its name only has to differ from every other action's.
		a = action(cx, fmt.tprintf("gk::shortcut::%x", uintptr(h.fn)), "", loc)
		on_app_action(cx, a, h, loc)
		if shortcut_actions == nil {
			shortcut_actions = make(map[rawptr]Action, 16, runtime.heap_allocator())
		}
		shortcut_actions[h.fn] = a
	}
	if keys != "" {
		bind_keys(cx, {keys, a, when_in}, loc = loc)
	}
	_ = t
	return a
}

@(private)
shortcut_actions: map[rawptr]Action

// dispatch_action dispatches an action to the focused element of the turn's window (as its
// key binding would), or app-wide in a turn without a window. It runs after the handler.
dispatch_action :: proc(cx: ^Cx, a: Action, loc := #caller_location) {
	req := make(Tape, 0, 8, context.temp_allocator)
	put_u16(&req, C_DISPATCH_ACTION)
	put_u32(&req, a.id)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// action_available reports whether something on the focus path, or app-wide, takes the action.
// App actions are one Rust type, so this is true for every app action when any listener of an
// app action is on the path (DESIGN.md section 8).
action_available :: proc(cx: ^Cx, a: Action, loc := #caller_location) -> bool {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 8, context.temp_allocator)
	put_u16(&req, C_ACTION_AVAILABLE)
	put_u32(&req, a.id)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u8(&r) != 0
}

// keystrokes gives the keys of the action's binding that wins ("ctrl-s"), as the kit's Kbd
// shows them, and false when it has none. key_context: only bindings of that context.
keystrokes :: proc(cx: ^Cx, a: Action, key_context := "", loc := #caller_location) -> (Temp_String, bool) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 12 + len(key_context), context.temp_allocator)
	put_u16(&req, C_ACTION_KEYSTROKES)
	put_u32(&req, a.id)
	put_str(&req, key_context)
	r := Reader{must_call(req[:], loc), 0}
	if rd_u8(&r) == 0 {
		return "", false
	}
	return rd_temp(&r), true
}

// --- timers ---------------------------------------------------------------------------------------

// Timer fires a handler between frames, never in the middle of one. gk.release cancels it; a
// one-shot timer that has fired is gone already, and releasing it then does nothing.
Timer :: struct {
	h:    u64,
	slot: u32,
}

@(private)
timers_once: map[u32]u64 // kept slot -> timer, for one-shot timers that have not fired
@(private)
timers_live: map[u64]u32 // timer -> kept slot

// after runs the handler once, ms milliseconds from now.
after :: proc(cx: ^Cx, ms: int, h: Handler, loc := #caller_location) -> Timer {
	return timer_new(cx, ms, false, h, loc)
}

// every runs the handler every ms milliseconds until the timer is released.
every :: proc(cx: ^Cx, ms: int, h: Handler, loc := #caller_location) -> Timer {
	return timer_new(cx, ms, true, h, loc)
}

@(private)
timer_new :: proc(cx: ^Cx, ms: int, repeat: bool, h: Handler, loc: runtime.Source_Code_Location) -> Timer {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not start timers: start them in init or in a handler")
	}
	if ms < 0 || ms > int(max(u32)) || repeat && ms == 0 {
		stop(loc, "a %s of %d ms: give %d to %d ms", "repeating timer (gk.every)" if repeat else "timer (gk.after)", ms, 1 if repeat else 0, max(u32))
	}
	flush(t, loc)
	slot, gen := keep_handler(cx, h, SHAPE_UNIT, loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_TIMER_NEW)
	put_u32(&req, u32(ms))
	put_u8(&req, repeat ? 1 : 0)
	put_u32(&req, slot)
	put_u32(&req, gen)
	r := Reader{must_call(req[:], loc), 0}
	tm := Timer{rd_u64(&r), slot}
	if timers_live == nil {
		timers_live = make(map[u64]u32, 8, runtime.heap_allocator())
		timers_once = make(map[u32]u64, 8, runtime.heap_allocator())
	}
	timers_live[tm.h] = slot
	if !repeat {
		timers_once[slot] = tm.h
	}
	return tm
}

// timer_fired: after a kept handler ran (rt_handler.odin, event); a one-shot timer's handler
// is forgotten here, as the library forgets the timer.
@(private)
timer_fired :: proc(slot: u32) {
	if th, ok := timers_once[slot]; ok {
		delete_key(&timers_once, slot)
		delete_key(&timers_live, th)
		drop_handler(slot)
	}
}

@(private)
release_timer :: proc(t: ^Turn, tm: Timer, loc: runtime.Source_Code_Location) {
	slot, live := timers_live[tm.h]
	if !live {
		return // a one-shot timer that has fired, or one released before
	}
	delete_key(&timers_live, tm.h)
	delete_key(&timers_once, slot)
	drop_handler(slot)
	release_handle(t, tm.h, loc)
}

// --- windows --------------------------------------------------------------------------------------

Window_Style :: enum u8 {
	Normal   = 0,
	Popup    = 1, // above all other windows
	Floating = 2, // above its parent window
}

// Window_Config is every option open_window_with takes; zero values are GPUI's defaults.
Window_Config :: struct {
	width, height:         f32,
	title:                 string,
	min_width, min_height: f32, // 0: no minimum
	style:                 Window_Style,
	position:              Maybe([2]f32), // nil: centred
	not_resizable:         bool,
	not_minimizable:       bool,
	not_movable:           bool,
	no_focus:              bool,
	hidden:                bool,
	kit_title_bar:         bool, // the kit's title bar options (TitleBar::window_options, PROTOCOLS.md 13.21)
	// app_id names the app to the desktop, which matches the window to the app's desktop entry and
	// its icon by it (Wayland's app_id, X11's WM_CLASS): "myapp" for myapp.desktop. "": none.
	app_id:                string,
	// icon is the window's icon as the bytes of a PNG (X11 and Windows show it; Wayland takes the
	// icon of the desktop entry app_id names). nil: none. One the library cannot read stops.
	icon:                  []u8,
}

// open_window_with opens a window with options: gk.open_window (a title and a size) plus a
// minimum size, a position, a style and the rest.
open_window_with :: proc(cx: ^Cx, o: Window_Config, root: View, loc := #caller_location) -> Window {
	if build.active {
		stop(loc, "a view procedure may not open windows: open them in init or in a handler")
	}
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 56 + len(o.title) + len(o.app_id) + len(o.icon), context.temp_allocator)
	put_u16(&req, C_WINDOW_OPEN)
	put_f32(&req, o.width > 0 ? o.width : 1024)
	put_f32(&req, o.height > 0 ? o.height : 700)
	put_u64(&req, root.h)
	put_str(&req, o.title)
	flags: u8
	if o.not_resizable { flags |= 1 }
	if o.not_minimizable { flags |= 2 }
	if o.not_movable { flags |= 4 }
	if o.no_focus { flags |= 8 }
	if o.hidden { flags |= 16 }
	if o.kit_title_bar { flags |= 32 }
	put_u8(&req, flags)
	put_f32(&req, o.min_width)
	put_f32(&req, o.min_height)
	put_u8(&req, u8(o.style))
	p, has := o.position.?
	put_u8(&req, has ? 1 : 0)
	put_f32(&req, p[0])
	put_f32(&req, p[1])
	put_str(&req, o.app_id)
	put_u32(&req, u32(len(o.icon)))
	append(&req, ..o.icon)
	r := Reader{must_call(req[:], loc), 0}
	return Window{rd_u64(&r)}
}

@(private)
window_op :: proc(cx: ^Cx, op: u16, w: Window, text: string, loc: runtime.Source_Code_Location) {
	req := make(Tape, 0, 16 + len(text), context.temp_allocator)
	put_u16(&req, op)
	put_u64(&req, w.h)
	if op == C_WINDOW_SET_TITLE {
		put_str(&req, text)
	}
	defer_op(check_cx(cx, loc), req[:], loc)
}

// close_window closes a window (after the handler). Its handle is stale from then on, its
// on_window_closed handlers run, and when it was the last window the app quits (unless
// quit_on_last_window(cx, false)). A zero Window is the turn's own.
close_window :: proc(cx: ^Cx, w: Window = {}, loc := #caller_location) {
	window_op(cx, C_WINDOW_CLOSE, w, "", loc)
}

set_window_title :: proc(cx: ^Cx, w: Window, title: string, loc := #caller_location) {
	window_op(cx, C_WINDOW_SET_TITLE, w, title, loc)
}

activate_window :: proc(cx: ^Cx, w: Window, loc := #caller_location) {
	window_op(cx, C_WINDOW_ACTIVATE, w, "", loc)
}

// quit_on_last_window: whether closing the last window quits the app (the default).
quit_on_last_window :: proc(cx: ^Cx, yes: bool, loc := #caller_location) {
	req := make(Tape, 0, 4, context.temp_allocator)
	put_u16(&req, C_QUIT_ON_LAST_WINDOW)
	put_u8(&req, yes ? 1 : 0)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// on_window_closed runs a handler when the window has closed (the handler may take the
// Window, whose handle is stale by then). gk.release(cx, listener) ends it.
on_window_closed :: proc(cx: ^Cx, w: Window, h: Handler, loc := #caller_location) -> Action_Listener {
	t := check_cx(cx, loc)
	flush(t, loc)
	slot, gen := keep_handler(cx, h, SHAPE_WINDOW, loc)
	req := make(Tape, 0, 20, context.temp_allocator)
	put_u16(&req, C_ON_WINDOW_CLOSED)
	put_u64(&req, w.h)
	put_u32(&req, slot)
	put_u32(&req, gen)
	r := Reader{must_call(req[:], loc), 0}
	return Action_Listener{rd_u64(&r), slot}
}

// on_window_should_close asks a handler before the user closes the window (its close button,
// Alt-F4, the taskbar; Window::on_window_should_close): the handler returns false to keep it open,
// say to ask about unsaved edits first, and closes it later with gk.close_window, which does not ask
// again. Several handlers are asked in the order given; the first false keeps the window. A zero
// Window is the turn's own. gk.release(cx, listener) ends it. PROTOCOLS.md 11.38.
//   ask :: proc(a: ^App, cx: ^gk.Cx) -> bool {if !a.dirty {return true}; gk.open_dialog(cx, a.confirm); return false}
//   gk.on_window_should_close(cx, {}, gk.on(ask))
on_window_should_close :: proc(cx: ^Cx, w: Window, h: Handler, loc := #caller_location) -> Action_Listener {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not listen for a window's close: do it in init or in a handler")
	}
	flush(t, loc)
	slot, gen := keep_handler(cx, h, SHAPE_UNIT_BOOL, loc)
	req := make(Tape, 0, 20, context.temp_allocator)
	put_u16(&req, C_ON_WINDOW_SHOULD_CLOSE)
	put_u64(&req, w.h)
	put_u32(&req, slot)
	put_u32(&req, gen)
	r := Reader{must_call(req[:], loc), 0}
	return Action_Listener{rd_u64(&r), slot}
}

// on_app_quit runs a handler once as the app quits (App::on_app_quit), however it quits: gk.quit,
// the last window closed, the platform's quit (Cmd-Q on macOS). It runs with no window and must not
// wait long: gpui gives quit handlers a moment, then exits. gk.release(cx, listener) ends it.
on_app_quit :: proc(cx: ^Cx, h: Handler, loc := #caller_location) -> Action_Listener {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not set a quit handler: do it in init or in a handler")
	}
	flush(t, loc)
	slot, gen := keep_handler(cx, h, SHAPE_UNIT, loc)
	req := make(Tape, 0, 12, context.temp_allocator)
	put_u16(&req, C_ON_APP_QUIT)
	put_u32(&req, slot)
	put_u32(&req, gen)
	r := Reader{must_call(req[:], loc), 0}
	return Action_Listener{rd_u64(&r), slot}
}

// reduce_motion reports whether the app should draw non-essential motion still (gpui's
// App::reduce_motion: the system's setting once gk.base_apply_system_reduce_motion follows it, or
// what set_reduce_motion set). A view procedure may read it.
reduce_motion :: proc(cx: ^Cx, loc := #caller_location) -> bool {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_REDUCE_MOTION)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u8(&r) != 0
}

// set_reduce_motion sets the reduce-motion flag (App::set_reduce_motion); every window redraws. It
// runs after the handler.
set_reduce_motion :: proc(cx: ^Cx, reduce: bool, loc := #caller_location) {
	req := make(Tape, 0, 4, context.temp_allocator)
	put_u16(&req, C_SET_REDUCE_MOTION)
	put_u8(&req, reduce ? 1 : 0)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// test_should_close asks the window's should-close handlers as the platform does when the user
// closes it, and reports whether it may close. For tests (offscreen there is no close button).
test_should_close :: proc(cx: ^Cx, w: Window, loc := #caller_location) -> bool {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 12, context.temp_allocator)
	put_u16(&req, C_PLATFORM_TEST)
	put_u8(&req, 6)
	put_u64(&req, w.h)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u8(&r) != 0
}

Window_Info :: struct {
	title:                Temp_String,
	x, y, width, height: f32,
	active:               bool,
}

// window_info reads a window's title, bounds and whether it is the active one. A zero Window
// is the turn's own.
window_info :: proc(cx: ^Cx, w: Window = {}, loc := #caller_location) -> Window_Info {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_WINDOW_INFO)
	put_u64(&req, w.h)
	r := Reader{must_call(req[:], loc), 0}
	i: Window_Info
	i.title = rd_temp(&r)
	i.x, i.y, i.width, i.height = rd_f32(&r), rd_f32(&r), rd_f32(&r), rd_f32(&r)
	i.active = rd_u8(&r) != 0
	return i
}

// windows lists the open windows, in the order they were opened (in the turn's allocator).
windows :: proc(cx: ^Cx, loc := #caller_location) -> []Window {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_WINDOWS)
	r := Reader{must_call(req[:], loc), 0}
	n := int(rd_u32(&r))
	out := make([]Window, n, context.temp_allocator)
	for i in 0 ..< n {
		out[i] = Window{rd_u64(&r)}
	}
	return out
}

// --- the clipboard ----------------------------------------------------------------------------------

clipboard_write :: proc(cx: ^Cx, text: string, loc := #caller_location) {
	req := make(Tape, 0, 8 + len(text), context.temp_allocator)
	put_u16(&req, C_CLIPBOARD_WRITE)
	put_str(&req, text)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// clipboard_read gives the clipboard's text, and false when it holds none.
clipboard_read :: proc(cx: ^Cx, loc := #caller_location) -> (Temp_String, bool) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_CLIPBOARD_READ)
	r := Reader{must_call(req[:], loc), 0}
	if rd_u8(&r) == 0 {
		return "", false
	}
	return rd_temp(&r), true
}

// --- ambient calls -----------------------------------------------------------------------------------

// ambient_call runs one generated call of the ambient context (gen_ambient.odin, PROTOCOLS.md
// section 11.5) at once, with the turn's window and app. One that returns nothing changes the
// app or a window, so a view procedure may not make it; one that returns a value (a colour of
// the palette) may.
@(private)
ambient_call :: proc(cx: ^Cx, req: []u8, returns: bool, loc := #caller_location) -> Reader {
	t := check_cx(cx, loc)
	if !returns && build.active {
		stop(loc, "a view procedure may not change the app or a window: make this call in a handler or in init")
	}
	flush(t, loc)
	return Reader{must_call(req, loc), 0}
}
