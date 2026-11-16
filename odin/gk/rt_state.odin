package gk

// What the runtime itself offers through call ops: windows, the per-frame handler, quit,
// focus handles, releasing handles, and the test hooks of the runtime's own tests.

import "base:runtime"

Window :: struct {
	h: u64,
}

// open_window opens a window titled `title`, `size` = {width, height} in logical pixels, whose
// root (inside the kit's Root) is the view:
//   gk.open_window(cx, "Groceries", {520, 560}, gk.view(a, init, root_view))
// open_window_with (rt_app.odin) takes every other option (a minimum size, a position, ...).
open_window :: proc(cx: ^Cx, title: string, size: [2]f32, root: View, loc := #caller_location) -> Window {
	return open_window_with(cx, {width = size[0], height = size[1], title = title}, root, loc)
}

@(private)
Frame_Handler :: struct {
	call:  proc(fn, state: rawptr, cx: ^Cx),
	fn:    rawptr,
	state: rawptr,
}

@(private)
frame_handlers: map[u64]Frame_Handler

// on_frame asks for a FRAME turn before every frame of the turn's window. It is the only way
// app code runs on every frame, and it is opt-in. What the procedure writes to kit state is
// in place before the views are described for that same frame.
on_frame :: proc(cx: ^Cx, fn: proc(s: ^$S, cx: ^Cx), loc := #caller_location) {
	t := check_cx(cx, loc)
	state: rawptr
	if t.owner != nil && t.owner.state_type == typeid_of(S) {
		state = t.owner.state
	} else if typeid_of(S) == app_state_type {
		state = app_state
	} else {
		stop(loc, "a frame handler takes ^%v, which is neither the state of the view whose init or handler calls gk.on_frame nor the app's (%v): take one of those", typeid_of(S), app_state_type)
	}
	if t.window == 0 {
		stop(loc, "on_frame needs a window: call it in a view's init procedure or in a handler")
	}
	tramp :: proc(fn, state: rawptr, cx: ^Cx) {
		(cast(proc(s: ^S, cx: ^Cx))fn)(cast(^S)state, cx)
	}
	if frame_handlers == nil {
		frame_handlers = make(map[u64]Frame_Handler, 4, runtime.heap_allocator())
	}
	frame_handlers[t.window] = Frame_Handler{tramp, rawptr(fn), state}
	req := make(Tape, 0, 12, context.temp_allocator)
	put_u16(&req, C_ON_FRAME)
	put_u64(&req, t.window)
	append(&req, 1)
	defer_op(t, req[:], loc)
}

// quit ends the app.
quit :: proc(cx: ^Cx, loc := #caller_location) {
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_QUIT)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// focus_new makes a GPUI FocusHandle in the library and returns its handle (kind FOCUS);
// focus_set moves keyboard focus to the element that tracks it. The typed procedures
// (gk.focus_handle, gk.focus) sit beside the Focus_Handle type, which is the generated
// package's (elements.odin has the sample's).
@(private)
focus_new :: proc(cx: ^Cx, loc := #caller_location) -> u64 {
	if build.active {
		stop(loc, "a view procedure may not create focus handles: create them in init or in a handler and keep the handle")
	}
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_FOCUS_HANDLE_NEW)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u64(&r)
}

@(private)
focus_set :: proc(cx: ^Cx, h: u64, loc := #caller_location) {
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_FOCUS)
	put_u64(&req, h)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// Subscription is a handler subscribed to the events of kit state. Releasing it, or the
// state, ends it.
Subscription :: struct {
	h:    u64,
	slot: u32,
}

@(private)
release_handle :: proc(t: ^Turn, h: u64, loc: runtime.Source_Code_Location) {
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_RELEASE)
	put_u64(&req, h)
	defer_op(t, req[:], loc)
}

// release gives back anything held by handle: kit state, a subscription, a focus handle, a
// view. The handle is stale from then on; using it stops with a message, never worse.
release :: proc(cx: ^Cx, x: $T, loc := #caller_location) {
	t := check_cx(cx, loc)
	if build.active {
		// Before anything is forgotten: a row procedure releasing its own row source would
		// otherwise free the record of the row being described.
		stop(loc, "a view procedure may not write: it only describes (gk.release). Release in a handler, in init or in a frame handler")
	}
	when T == View {
		release_view(t, x, loc)
	} else when T == Subscription {
		drop_handler(x.slot)
		release_handle(t, x.h, loc)
	} else when T == Row_Source {
		release_source(t, x, loc)
	} else when T == Timer {
		release_timer(t, x, loc)
	} else when T == Completion {
		release_completion(t, x, loc) // cancels a request that waits (rt_platform.odin)
	} else when T == Action_Listener {
		drop_handler(x.slot)
		release_handle(t, x.h, loc)
	} else {
		kept_release(x.h) // handlers given to its calls (kit_editor.odin)
		release_handle(t, x.h, loc)
	}
}

// --- test hooks (used by core/tests and the misuse gate) ----------------------------------------

@(private)
put_kept :: proc(req: ^Tape, slot: u32, loc: runtime.Source_Code_Location) {
	if int(slot) >= len(kept) || !kept[slot].live {
		stop(loc, "this handler slot is not in use")
	}
	put_u64(req, 0)
	put_u32(req, kept[slot].gen)
	put_u32(req, slot)
}

// test_fire has the library fire a kept handler synchronously and returns the value the
// library received from it.
test_fire :: proc(cx: ^Cx, slot: u32, loc := #caller_location) -> i32 {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 20, context.temp_allocator)
	put_u16(&req, C_TEST_FIRE)
	put_kept(&req, slot, loc)
	r := Reader{must_call(req[:], loc), 0}
	return rd_i32(&r)
}

// test_fire_value fires a kept handler of SHAPE_TEST with (text, n) and returns the string
// that came back through the library.
test_fire_value :: proc(cx: ^Cx, h: Handler, text: string, n: u32, loc := #caller_location) -> Temp_String {
	flush(check_cx(cx, loc), loc)
	slot, _ := keep_handler(cx, h, SHAPE_TEST, loc)
	req := make(Tape, 0, 32 + len(text), context.temp_allocator)
	put_u16(&req, C_TEST_FIRE_VALUE)
	put_kept(&req, slot, loc)
	put_str(&req, text)
	put_u32(&req, n)
	r := Reader{must_call(req[:], loc), 0}
	out := rd_temp(&r)
	drop_handler(slot)
	return out
}

// test_fire_shape fires a handler of `shape` with a payload already on the wire, through the
// library (call op TEST_FIRE_SHAPE: decoded into the Rust payload types and fired through their
// Wire impls when the generated codecs can build them, typed = true), and returns what the
// handler returned, encoded. The generated shape test (core/tests/gen) calls it.
test_fire_shape :: proc(cx: ^Cx, h: Handler, shape: u16, payload: []u8, loc := #caller_location) -> (typed: bool, ret: []u8) {
	flush(check_cx(cx, loc), loc)
	slot, _ := keep_handler(cx, h, shape, loc)
	req := make(Tape, 0, 32 + len(payload), context.temp_allocator)
	put_u16(&req, C_TEST_FIRE_SHAPE)
	put_kept(&req, slot, loc)
	put_u16(&req, shape)
	put_u32(&req, u32(len(payload)))
	append(&req, ..payload)
	r := Reader{must_call(req[:], loc), 0}
	typed = rd_u8(&r) != 0
	n := int(rd_u32(&r))
	ret = make([]u8, n, context.temp_allocator)
	copy(ret, r.b[r.i:r.i + n])
	drop_handler(slot)
	return
}

// test_wire encodes a value as a handler's payload or return value travels (the runtime's
// reflection), for the generated shape test to compare with the bytes it sent.
test_wire :: proc(v: $T) -> []u8 {
	v := v
	b := make(Tape, 0, 64, context.temp_allocator)
	put_value(&b, &v, type_info_of(T))
	return b[:]
}

// test_fire_in_update fires a kept handler from inside a state's own update, as a delegate
// method would.
test_fire_in_update :: proc(cx: ^Cx, s: $S, slot: u32, loc := #caller_location) -> i32 {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 28, context.temp_allocator)
	put_u16(&req, C_TEST_FIRE_IN_UPDATE)
	put_u64(&req, s.h)
	put_kept(&req, slot, loc)
	r := Reader{must_call(req[:], loc), 0}
	return rd_i32(&r)
}

// test_fire_queued raises an event with payload `row` from inside a state's own update; the
// handler (SHAPE_USIZE) runs when that update has ended.
test_fire_queued :: proc(cx: ^Cx, s: $S, h: Handler, row: int, loc := #caller_location) {
	flush(check_cx(cx, loc), loc)
	slot, _ := keep_handler(cx, h, SHAPE_USIZE, loc)
	req := make(Tape, 0, 36, context.temp_allocator)
	put_u16(&req, C_TEST_FIRE_QUEUED)
	put_u64(&req, s.h)
	put_kept(&req, slot, loc)
	put_u64(&req, u64(row))
	must_call(req[:], loc)
}

// test_panic makes the library panic: at once, inside the call, or later, when buffered
// writes are applied outside any call.
test_panic :: proc(cx: ^Cx, later: bool, loc := #caller_location) {
	t := check_cx(cx, loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_TEST_PANIC)
	if later && !debug {
		defer_op(t, req[:], loc)
	} else {
		must_call(req[:], loc)
	}
}
