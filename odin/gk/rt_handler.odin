package gk

// Handlers (docs/PROTOCOLS.md section 4). A handler is a typed procedure, the trampoline
// that knows its type, and at most one value, copied and owned by the binding. Its shape
// (the event it takes and the value it returns) is checked against the builder's when it is
// registered. There are no closures: nothing a view made can be captured by accident.

import "base:runtime"
import "core:fmt"
import "core:os"
import "core:strings"
import "core:sync"

Handler :: struct {
	call:       proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader),
	fn:         rawptr,
	data:       rawptr,
	state_type: typeid,
	event_type: typeid, // nil: takes no event, so it fits every shape
	ret_type:   typeid, // nil: returns nothing
	quiet:      bool,
	loc:        runtime.Source_Code_Location,
	// Which owned block `data` is (slot + 1 and the slot's generation; 0 for none). A Handler
	// is a value and can be copied, so the block itself knows whether a builder has it.
	owned:      u32,
	owned_gen:  u32,
}

// --- the events of the built-in shapes --------------------------------------------------------

// The built-in events are the runtime's own records, with their own small types: the names
// Modifiers and Mouse_Button belong to the generated package (GPUI's types of those names).

Key_Mod :: enum u8 {
	Control,
	Alt,
	Shift,
	Platform,
	Function,
}
Key_Mods :: bit_set[Key_Mod;u8]

Pointer_Button :: enum u8 {
	Left    = 0,
	Right   = 1,
	Middle  = 2,
	Back    = 3,
	Forward = 4,
	None    = 255,
}

Click_Kind :: enum u8 {
	Mouse,
	Keyboard,
	Touch,
}

// Click_Event is GPUI's ClickEvent.
Click_Event :: struct {
	position:    [2]f32,
	button:      Pointer_Button,
	modifiers:   Key_Mods,
	click_count: u32,
	kind:        Click_Kind,
}

// Key_Event is GPUI's KeyDownEvent or KeyUpEvent. Its strings are lent for the turn.
Key_Event :: struct {
	key:       Temp_String,
	key_char:  Maybe(Temp_String),
	modifiers: Key_Mods,
	is_held:   bool,
}

// Mouse_Event is GPUI's MouseDownEvent, MouseUpEvent or MouseMoveEvent.
Mouse_Event :: struct {
	position:    [2]f32,
	button:      Pointer_Button,
	modifiers:   Key_Mods,
	click_count: u32,
}

// Test_Payload is the payload of SHAPE_TEST, which only the runtime's own tests fire.
Test_Payload :: struct {
	text: Temp_String,
	n:    u32,
}

// --- shapes -----------------------------------------------------------------------------------

@(private)
Shape_Info :: struct {
	event, ret: typeid,
	known:      bool,
}

@(private)
MAX_SHAPES :: 1024
@(private)
shapes: [MAX_SHAPES]Shape_Info

// register_shape tells the runtime what a shape number means in Odin: the type of the event
// a handler of that shape receives and the type it returns (nil for none). Generated code
// calls it from an @(init) procedure, once per generated shape.
@(private)
register_shape :: proc "contextless" (shape: u16, event: typeid, ret: typeid) {
	if int(shape) < MAX_SHAPES {
		shapes[shape] = {event, ret, true}
	}
}

@(private)
builtin_shapes :: proc() {
	register_shape(SHAPE_CLICK, Click_Event, nil)
	register_shape(SHAPE_UNIT_BOOL, nil, bool)
	register_shape(SHAPE_UNIT, nil, nil)
	register_shape(SHAPE_BOOL, bool, nil)
	register_shape(SHAPE_USIZE, int, nil)
	register_shape(SHAPE_STR, Temp_String, nil)
	register_shape(SHAPE_KEY, Key_Event, nil)
	register_shape(SHAPE_CLICK_BOOL, Click_Event, bool)
	register_shape(SHAPE_MOUSE, Mouse_Event, nil)
	register_shape(SHAPE_F64, f64, nil)
	register_shape(SHAPE_TEST, Test_Payload, string)
}

// check_shape stops the program, at the line of the gk.on call, when a handler does not fit
// the closure it is given to.
@(private)
check_shape :: proc(h: Handler, shape: u16) {
	if int(shape) >= MAX_SHAPES || !shapes[shape].known {
		stop(h.loc, "this builder's handler kind (%d) is unknown to the package: the package's generated files are incomplete or mixed from two downloads; use one download's odin/ folder", shape)
	}
	info := shapes[shape]
	if h.event_type != nil && h.event_type != info.event {
		if info.event == nil {
			stop(h.loc, "this handler takes a %v, but the event of the builder it is given to carries nothing: write proc(s: ^S, cx: ^gk.Cx) (with a value: proc(s, cx, value)); the builder's entry in the generated file shows the procedure to write", h.event_type)
		}
		stop(h.loc, "this handler takes a %v, but the event of the builder it is given to is a %v: write proc(s: ^S, cx: ^gk.Cx, e: gk.%v)", h.event_type, info.event, info.event)
	}
	if h.ret_type != nil && h.ret_type != info.ret {
		if info.ret == nil {
			stop(h.loc, "this handler returns a %v, but the builder it is given to expects no result: drop the result", h.ret_type)
		}
		stop(h.loc, "this handler returns a %v, but the builder it is given to expects a %v (a dialog's on_ok returns a bool: true closes it)", h.ret_type, info.ret)
	}
}

// --- gk.on ------------------------------------------------------------------------------------

// on makes a handler from a procedure, or a procedure and one value. The procedure takes the
// view's state and the Cx, then optionally the event, then optionally the value; it may
// return what the event expects back.
//   gk.on(add_row)                 add_row :: proc(s: ^App, cx: ^gk.Cx)
//   gk.on(toggled)                 toggled :: proc(s: ^App, cx: ^gk.Cx, checked: bool)
//   gk.on(select_row, row_id)      select_row :: proc(s: ^App, cx: ^gk.Cx, id: Row_Id)
//   gk.on(clicked_row, row_id)     clicked_row :: proc(s: ^App, cx: ^gk.Cx, e: gk.Click_Event, id: Row_Id)
//   gk.on(confirm)                 confirm :: proc(s: ^App, cx: ^gk.Cx, e: gk.Click_Event) -> bool
// The value is copied when the handler is made and owned by the binding, strings included; a
// pointer, slice, map or dynamic array inside it is refused with this line. It is freed with
// the handler; the strings of a value whose handler ran are kept, so a handler may store them. The view that registers the handler is marked for redraw when
// the handler returns (gk.quiet opts out).
on :: proc {
	on_plain,
	on_event,
	on_data,
	on_event_data,
	on_ret,
	on_event_ret,
	on_data_ret,
	on_event_data_ret,
}

on_plain :: proc(fn: proc(s: ^$S, cx: ^Cx), loc := #caller_location) -> Handler {
	tramp :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		(cast(proc(s: ^S, cx: ^Cx))h.fn)(cast(^S)state, cx)
	}
	return {call = tramp, fn = rawptr(fn), state_type = typeid_of(S), loc = loc}
}

on_event :: proc(fn: proc(s: ^$S, cx: ^Cx, e: $E), loc := #caller_location) -> Handler {
	tramp :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		e: E
		rd_value(r, &e, type_info_of(E))
		(cast(proc(s: ^S, cx: ^Cx, e: E))h.fn)(cast(^S)state, cx, e)
	}
	return {call = tramp, fn = rawptr(fn), state_type = typeid_of(S), event_type = typeid_of(E), loc = loc}
}

on_data :: proc(fn: proc(s: ^$S, cx: ^Cx, d: $D), data: D, loc := #caller_location) -> Handler {
	tramp :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		(cast(proc(s: ^S, cx: ^Cx, d: D))h.fn)(cast(^S)state, cx, (cast(^D)h.data)^)
	}
	return with_data(Handler{call = tramp, fn = rawptr(fn), state_type = typeid_of(S), loc = loc}, data, loc)
}

on_event_data :: proc(fn: proc(s: ^$S, cx: ^Cx, e: $E, d: $D), data: D, loc := #caller_location) -> Handler {
	tramp :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		e: E
		rd_value(r, &e, type_info_of(E))
		(cast(proc(s: ^S, cx: ^Cx, e: E, d: D))h.fn)(cast(^S)state, cx, e, (cast(^D)h.data)^)
	}
	return with_data(Handler{call = tramp, fn = rawptr(fn), state_type = typeid_of(S), event_type = typeid_of(E), loc = loc}, data, loc)
}

on_ret :: proc(fn: proc(s: ^$S, cx: ^Cx) -> $R, loc := #caller_location) -> Handler {
	tramp :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		ret := (cast(proc(s: ^S, cx: ^Cx) -> R)h.fn)(cast(^S)state, cx)
		put_value(&cx.turn.ret, &ret, type_info_of(R))
	}
	return {call = tramp, fn = rawptr(fn), state_type = typeid_of(S), ret_type = typeid_of(R), loc = loc}
}

on_event_ret :: proc(fn: proc(s: ^$S, cx: ^Cx, e: $E) -> $R, loc := #caller_location) -> Handler {
	tramp :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		e: E
		rd_value(r, &e, type_info_of(E))
		ret := (cast(proc(s: ^S, cx: ^Cx, e: E) -> R)h.fn)(cast(^S)state, cx, e)
		put_value(&cx.turn.ret, &ret, type_info_of(R))
	}
	return {
		call = tramp,
		fn = rawptr(fn),
		state_type = typeid_of(S),
		event_type = typeid_of(E),
		ret_type = typeid_of(R),
		loc = loc,
	}
}

on_data_ret :: proc(fn: proc(s: ^$S, cx: ^Cx, d: $D) -> $R, data: D, loc := #caller_location) -> Handler {
	tramp :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		ret := (cast(proc(s: ^S, cx: ^Cx, d: D) -> R)h.fn)(cast(^S)state, cx, (cast(^D)h.data)^)
		put_value(&cx.turn.ret, &ret, type_info_of(R))
	}
	return with_data(Handler{call = tramp, fn = rawptr(fn), state_type = typeid_of(S), ret_type = typeid_of(R), loc = loc}, data, loc)
}

on_event_data_ret :: proc(fn: proc(s: ^$S, cx: ^Cx, e: $E, d: $D) -> $R, data: D, loc := #caller_location) -> Handler {
	tramp :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		e: E
		rd_value(r, &e, type_info_of(E))
		ret := (cast(proc(s: ^S, cx: ^Cx, e: E, d: D) -> R)h.fn)(cast(^S)state, cx, e, (cast(^D)h.data)^)
		put_value(&cx.turn.ret, &ret, type_info_of(R))
	}
	return with_data(Handler{
		call = tramp,
		fn = rawptr(fn),
		state_type = typeid_of(S),
		event_type = typeid_of(E),
		ret_type = typeid_of(R),
		loc = loc,
	}, data, loc)
}

// quiet makes a handler that does not mark its view dirty when it returns.
quiet :: proc(h: Handler) -> Handler {
	h := h
	h.quiet = true
	return h
}

// --- values owned by the binding: handler data and posted messages ----------------------------

// own copies a handler's or a post's value into memory the binding owns: one block for the
// value and one allocation per string inside it. A pointer, map, any, union or procedure inside
// it is refused with the caller's line, and so are a slice and a dynamic array in handler data;
// a posted message (lists) may hold slices and dynamic arrays, which are copied with their
// elements (a thread's result handed to the UI thread). The block is given back by disown
// (handler data) or give (a posted message); live bytes are counted in owned_bytes.
@(private)
own :: proc(data: $D, loc: runtime.Source_Code_Location, lists := false) -> rawptr {
	p := new(D, runtime.heap_allocator())
	p^ = data
	n := own_walk(p, type_info_of(D), type_info_of(D), "", loc, lists)
	account(size_of(D) + n)
	return p
}

// own_walk checks a value's type and copies every string (and, with lists, every slice and
// dynamic array) in it; it returns the bytes copied.
@(private)
own_walk :: proc(p: rawptr, ti, root: ^runtime.Type_Info, field: string, loc: runtime.Source_Code_Location, lists := false) -> (n: int) {
	refuse :: proc(what: string, ti, root: ^runtime.Type_Info, field: string, loc: runtime.Source_Code_Location, lists: bool) -> ! {
		stop(
			loc,
			"%s of type %v holds a %s (%v%s%s), which the binding cannot own; pass plain values, strings%s, or an id",
			lists ? "a posted message" : "handler data", root, what, ti, field != "" ? " in field " : "", field,
			lists ? ", slices and dynamic arrays" : "",
		)
	}
	// copy_elems copies n elements at data into a block of the binding's and owns what each holds.
	copy_elems :: proc(data: rawptr, count: int, elem, root: ^runtime.Type_Info, field: string, loc: runtime.Source_Code_Location) -> (rawptr, int) {
		if count == 0 {
			return nil, 0
		}
		size := count * elem.size
		mem, _ := runtime.mem_alloc(size, elem.align, runtime.heap_allocator())
		runtime.mem_copy(raw_data(mem), data, size)
		n := size
		for i in 0 ..< count {
			n += own_walk(rawptr(uintptr(raw_data(mem)) + uintptr(i * elem.size)), elem, root, field, loc, true)
		}
		return raw_data(mem), n
	}
	#partial switch v in runtime.type_info_base(ti).variant {
	case runtime.Type_Info_String:
		if v.is_cstring {
			refuse("cstring", ti, root, field, loc, lists)
		}
		s := cast(^string)p
		if len(s^) > 0 {
			s^ = strings.clone(s^, runtime.heap_allocator())
			n = len(s^)
		} else {
			s^ = ""
		}
	case runtime.Type_Info_Struct:
		for i in 0 ..< int(v.field_count) {
			n += own_walk(rawptr(uintptr(p) + v.offsets[i]), v.types[i], root, v.names[i], loc, lists)
		}
	case runtime.Type_Info_Array:
		for i in 0 ..< v.count {
			n += own_walk(rawptr(uintptr(p) + uintptr(i * v.elem_size)), v.elem, root, field, loc, lists)
		}
	case runtime.Type_Info_Pointer, runtime.Type_Info_Multi_Pointer:
		refuse("pointer", ti, root, field, loc, lists)
	case runtime.Type_Info_Slice:
		if !lists {
			refuse("slice", ti, root, field, loc, lists)
		}
		s := cast(^runtime.Raw_Slice)p
		data, m := copy_elems(s.data, s.len, v.elem, root, field, loc)
		s.data = data
		n += m
	case runtime.Type_Info_Dynamic_Array:
		if !lists {
			refuse("dynamic array", ti, root, field, loc, lists)
		}
		d := cast(^runtime.Raw_Dynamic_Array)p
		data, m := copy_elems(d.data, d.len, v.elem, root, field, loc)
		d^ = {data = data, len = d.len, cap = d.len, allocator = runtime.heap_allocator()}
		n += m
	case runtime.Type_Info_Map:
		refuse("map", ti, root, field, loc, lists)
	case runtime.Type_Info_Any:
		refuse("any", ti, root, field, loc, lists)
	case runtime.Type_Info_Union:
		refuse("union", ti, root, field, loc, lists)
	case runtime.Type_Info_Procedure:
		refuse("procedure", ti, root, field, loc, lists)
	}
	return
}

@(private)
Str_Op :: enum {
	Free, // free each string, slice and dynamic array (the binding's copy)
	Give, // replace each by a copy in context.allocator and free the binding's copy
}

// strings_walk applies op to every string, slice and dynamic array of a value own_walk accepted;
// it returns their bytes.
@(private)
strings_walk :: proc(p: rawptr, ti: ^runtime.Type_Info, op: Str_Op) -> (n: int) {
	// elems applies op to count elements at data (a block of the binding's) and returns the block
	// that holds them now: a copy in context.allocator for Give, nil for Free.
	elems :: proc(data: rawptr, count: int, elem: ^runtime.Type_Info, op: Str_Op) -> (rawptr, int) {
		if data == nil || count == 0 {
			return nil, 0
		}
		size := count * elem.size
		to := rawptr(nil)
		if op == .Give {
			mem, _ := runtime.mem_alloc(size, elem.align, context.allocator)
			runtime.mem_copy(raw_data(mem), data, size)
			to = raw_data(mem)
		}
		n := size
		at := to if op == .Give else data
		for i in 0 ..< count {
			n += strings_walk(rawptr(uintptr(at) + uintptr(i * elem.size)), elem, op)
		}
		runtime.mem_free_with_size(data, size, runtime.heap_allocator())
		return to, n
	}
	#partial switch v in runtime.type_info_base(ti).variant {
	case runtime.Type_Info_String:
		s := cast(^string)p
		if len(s^) == 0 {
			return 0
		}
		n = len(s^)
		old := s^
		if op == .Give {
			s^ = strings.clone(old, context.allocator)
		}
		delete(old, runtime.heap_allocator())
	case runtime.Type_Info_Struct:
		for i in 0 ..< int(v.field_count) {
			n += strings_walk(rawptr(uintptr(p) + v.offsets[i]), v.types[i], op)
		}
	case runtime.Type_Info_Array:
		for i in 0 ..< v.count {
			n += strings_walk(rawptr(uintptr(p) + uintptr(i * v.elem_size)), v.elem, op)
		}
	case runtime.Type_Info_Slice:
		s := cast(^runtime.Raw_Slice)p
		data, m := elems(s.data, s.len, v.elem, op)
		s.data = data
		n += m
	case runtime.Type_Info_Dynamic_Array:
		d := cast(^runtime.Raw_Dynamic_Array)p
		data, m := elems(d.data, d.len, v.elem, op)
		d^ = {data = data, len = d.len if data != nil else 0, cap = d.len if data != nil else 0, allocator = context.allocator}
		n += m
	}
	return
}

// disown frees a block made by own. keep_strings leaves its strings allocated (and counted):
// a handler that has run may have stored one of them in its state.
@(private)
disown :: proc(p: rawptr, ti: ^runtime.Type_Info, keep_strings: bool) {
	n := ti.size
	if !keep_strings {
		n += strings_walk(p, ti, .Free)
	}
	free(p, runtime.heap_allocator())
	account(-n)
}

// give hands a posted message to the procedure it is posted to: its strings become copies in
// the app's own allocator (context.allocator of gk.run), which the procedure owns from then on,
// and the binding's copies are freed.
@(private)
give :: proc(p: rawptr, ti: ^runtime.Type_Info) {
	account(-strings_walk(p, ti, .Give))
}

// owned_bytes is what the binding holds for the app: handler values, posted messages not yet
// delivered, and the strings of handler values that ran. Past 16 MB it warns, and again at
// every doubling: values given to gk.on in a view described often, posts made faster than the
// UI thread takes them, or handlers that kept strings of values that are gone.
@(private)
owned_bytes: int
@(private)
owned_warn_at: int = 16 << 20

@(private)
account :: proc(delta: int) {
	now := sync.atomic_add(&owned_bytes, delta) + delta
	at := sync.atomic_load(&owned_warn_at)
	if delta > 0 && now >= at {
		if _, ok := sync.atomic_compare_exchange_strong(&owned_warn_at, at, at * 2); ok {
			fmt.eprintf(
				"gk: warning: the binding holds %d MB of values given to gk.on and gk.post (handler data of views described with new values, posts not yet delivered, and the strings of handler values that ran); it warns again at %d MB\n",
				now >> 20, (at * 2) >> 20,
			)
		}
	}
}

// test_owned_bytes is what the binding holds for the app now (the runtime's tests read it).
test_owned_bytes :: proc() -> int {
	return sync.atomic_load(&owned_bytes)
}

// --- owned blocks of handler data -----------------------------------------------------------------

// A Handler is a plain value: the app can copy it, give one to two builders, or keep it in
// its state past the view build that made it. The block of data behind it is therefore
// tracked here, by slot and generation, and every use is checked:
//   - a builder (or a subscription) takes a block once; a second taker stops the program;
//   - a block released with its handler table makes every remaining copy stale, and giving
//     such a copy to a builder stops the program instead of reading freed memory;
//   - a block released while a handler is running (a handler that releases its own view or
//     subscription, or whose view is described again inside it) is freed when the outermost
//     handler has returned, so the running handler's value stays readable;
//   - a block no builder took is freed when the turn that made it ends;
//   - the strings of a block whose handler ran stay allocated when the block is freed: the
//     handler may have stored one (the kept view string, DESIGN.md section 14), and nothing tells
//     the binding when the app lets go of it. That is bounded by the handlers that ran, and
//     counted in owned_bytes.
@(private)
Owned :: struct {
	p:     rawptr,
	ti:    ^runtime.Type_Info,
	gen:   u32,
	taken: bool,
	fired: bool, // the handler ran: its strings may be in the app's state
}

@(private)
Late :: struct {
	p:     rawptr,
	ti:    ^runtime.Type_Info,
	fired: bool,
}

@(private)
owned_mu: sync.Mutex
@(private)
owned_tab: [dynamic]Owned
@(private)
owned_free: [dynamic]u32
@(private)
owned_fresh: [dynamic]u32 // made in this turn; freed at its end unless taken
@(private)
owned_late: [dynamic]Late // released under a running handler; freed when it has returned
@(private)
firing: int

@(private)
with_data :: proc(h: Handler, data: $D, loc: runtime.Source_Code_Location) -> Handler {
	h := h
	h.data = own(data, loc)
	ti := type_info_of(D)
	sync.mutex_lock(&owned_mu)
	defer sync.mutex_unlock(&owned_mu)
	if owned_tab == nil {
		a := runtime.heap_allocator()
		owned_tab = make([dynamic]Owned, 0, 64, a)
		owned_free = make([dynamic]u32, 0, 64, a)
		owned_fresh = make([dynamic]u32, 0, 64, a)
		owned_late = make([dynamic]Late, 0, 8, a)
	}
	slot: u32
	if n := len(owned_free); n > 0 {
		slot = owned_free[n - 1]
		pop(&owned_free)
		owned_tab[slot].p = h.data
		owned_tab[slot].ti = ti
	} else {
		append(&owned_tab, Owned{p = h.data, ti = ti, gen = 1})
		slot = u32(len(owned_tab) - 1)
	}
	append(&owned_fresh, slot)
	h.owned = slot + 1
	h.owned_gen = owned_tab[slot].gen
	return h
}

// take_owned gives a handler's block to the builder or subscription that registers it.
@(private)
take_owned :: proc(h: Handler) {
	if h.owned == 0 {
		return
	}
	sync.mutex_lock(&owned_mu)
	o := &owned_tab[h.owned - 1]
	stale := o.gen != h.owned_gen
	twice := !stale && o.taken
	o.taken = o.taken || !stale
	sync.mutex_unlock(&owned_mu)
	if stale {
		stop(h.loc, "this handler was kept past the call that made it (a view procedure, init or a handler), and its value has been released: call gk.on again each time a handler is given to a builder, instead of storing the gk.Handler")
	}
	if twice {
		stop(h.loc, "one handler was given to two builders; a handler made by gk.on with a value belongs to one builder: call gk.on for each")
	}
}

// drop_owned releases a handler's block. Copies of the handler are stale from here on.
@(private)
drop_owned :: proc(h: Handler) {
	if h.owned == 0 {
		return
	}
	sync.mutex_lock(&owned_mu)
	defer sync.mutex_unlock(&owned_mu)
	o := &owned_tab[h.owned - 1]
	if o.gen != h.owned_gen || o.p == nil {
		return
	}
	if firing > 0 {
		append(&owned_late, Late{o.p, o.ti, o.fired})
	} else {
		disown(o.p, o.ti, o.fired)
	}
	o.p = nil
	o.taken = false
	o.fired = false
	o.gen += 1
	append(&owned_free, h.owned - 1)
}

// sweep_owned runs when the outermost turn has ended: blocks made in it that no builder took
// are freed (gk.on whose result was dropped), and so are blocks whose release had to wait.
@(private)
sweep_owned :: proc() {
	sync.mutex_lock(&owned_mu)
	defer sync.mutex_unlock(&owned_mu)
	for slot in owned_fresh {
		o := &owned_tab[slot]
		if o.p != nil && !o.taken {
			disown(o.p, o.ti, false)
			o.p = nil
			o.gen += 1
			append(&owned_free, slot)
		}
	}
	clear(&owned_fresh)
	if firing == 0 {
		for l in owned_late {
			disown(l.p, l.ti, l.fired)
		}
		clear(&owned_late)
	}
}

@(private)
free_handlers :: proc(hs: []Handler) {
	for h in hs {
		drop_owned(h)
	}
}

// --- kept handlers: subscriptions and other handlers that outlive a view build ----------------

@(private)
Kept :: struct {
	h:     Handler,
	gen:   u32,
	live:  bool,
	state: rawptr,
	owner: u64, // the view marked dirty when the handler returns; 0 for none
	// dropped with its view while the app still holds its handle (a subscription, a listener):
	// the slot is reused only when the app releases that handle, so the release cannot drop a
	// handler kept later in the same slot
	retired: bool,
}

// drop_kept_of drops the kept handlers that belong to a view being released (started in its
// init or its handlers: timers, subscriptions, app-wide and window listeners, panel events),
// so none of them runs again with the state of a view that is gone. A timer is cancelled.
@(private)
drop_kept_of :: proc(t: ^Turn, view: u64, loc: runtime.Source_Code_Location) {
	for i in 0 ..< len(kept) {
		k := &kept[i]
		if !k.live || k.owner != view {
			continue
		}
		slot := u32(i)
		timer: u64
		for th, s in timers_live {
			if s == slot {
				timer = th
				break
			}
		}
		if timer != 0 {
			release_timer(t, Timer{timer, slot}, loc)
			continue
		}
		if completion_drop_kept(t, slot, loc) {
			continue // a request of the view, cancelled with it (rt_platform.odin)
		}
		drop_owned(k.h)
		k.h = {}
		k.live = false
		k.gen += 1
		k.retired = true
	}
}

@(private)
kept: [dynamic]Kept
@(private)
kept_free: [dynamic]u32

// keep_handler stores a handler until drop_handler: for subscriptions and anything else the
// library calls back outside a view's tape. It belongs to the view whose init procedure or
// handler is running (that view's state, and that view is marked dirty after the handler);
// kept in any other turn, it takes the app's state and marks nothing.
@(private)
keep_handler :: proc(cx: ^Cx, h: Handler, shape: u16, loc := #caller_location) -> (slot, gen: u32) {
	t := check_cx(cx, loc)
	check_shape(h, shape)
	k := Kept{h = h, live = true, state = app_state}
	want := app_state_type
	if t.owner != nil {
		k.state = t.owner.state
		k.owner = t.owner.h
		want = t.owner.state_type
	}
	if h.state_type != want {
		stop(h.loc, "a handler takes ^%v but the state here is %v: a handler's first parameter is the state of the view (or, for app-wide handlers, the state given to gk.run) it belongs to", h.state_type, want)
	}
	take_owned(h)
	if kept == nil {
		kept = make([dynamic]Kept, 0, 16, runtime.heap_allocator())
		kept_free = make([dynamic]u32, 0, 16, runtime.heap_allocator())
	}
	if n := len(kept_free); n > 0 {
		slot = kept_free[n - 1]
		pop(&kept_free)
		k.gen = kept[slot].gen
		kept[slot] = k
	} else {
		k.gen = 1
		append(&kept, k)
		slot = u32(len(kept) - 1)
	}
	return slot, k.gen
}

@(private)
drop_handler :: proc(slot: u32) {
	if int(slot) >= len(kept) {
		return
	}
	if !kept[slot].live {
		if kept[slot].retired {
			// its view was released first (drop_kept_of); the slot is free from now on
			kept[slot].retired = false
			append(&kept_free, slot)
		}
		return
	}
	k := &kept[slot]
	drop_owned(k.h)
	k.h = {}
	k.live = false
	k.gen += 1
	append(&kept_free, slot)
}

// --- delivering an event ----------------------------------------------------------------------

@(private)
event :: proc(cx: ^Cx, r: ^Reader) {
	view := rd_u64(r)
	gen := rd_u32(r)
	id := rd_u32(r)
	_ = rd_u16(r) // the shape: checked when the handler was registered
	h: Handler
	found: bool
	state: rawptr
	owner: u64
	if view == 0 {
		if int(id) < len(kept) && kept[id].live && kept[id].gen == gen {
			k := kept[id]
			h, found, state, owner = k.h, true, k.state, k.owner
			if k.owner != 0 {
				cx.turn.owner = views[k.owner]
			}
		}
	} else if v, ok := views[view]; ok {
		table: []Handler
		has := gen == v.gen
		if has {
			table = v.cur[:]
		} else if t, in_old := v.old[gen]; in_old {
			table, has = t[:], true
		}
		if has && int(id) < len(table) {
			h, found, state, owner = table[id], true, v.state, view
			cx.turn.owner = v
		} else if lib_diag {
			// Every description the library holds keeps its table (retire_table), so an event for
			// a live view always finds its handler: one that does not is a bug in the binding,
			// which would otherwise drop a click without a word.
			fmt.eprintf("gk: an event came for handler %d of description %d of view %x, whose handler table is gone (the view's description is %d): the library and the package disagree about which descriptions are alive. This is a bug in the binding; please report it\n", id, gen, view, v.gen)
			os.exit(1)
		}
	}
	if !found {
		// The view was released, or the subscription or timer that named this handler. The
		// library falls back to the default return value.
		dropped += 1
		if debug {
			fmt.eprintf("gk: dropped event for view %x generation %d handler %d\n", view, gen, id)
		}
		return
	}
	// The handler may release what it belongs to (its own view, its own subscription): its
	// value stays readable until it has returned.
	if h.owned != 0 {
		sync.mutex_lock(&owned_mu)
		if o := &owned_tab[h.owned - 1]; o.gen == h.owned_gen {
			o.fired = true
		}
		sync.mutex_unlock(&owned_mu)
	}
	firing += 1
	h.call(&h, state, cx, r)
	firing -= 1
	if view == 0 {
		timer_fired(id) // a one-shot timer's handler is forgotten once it has run (rt_app.odin)
	}
	// The view that registered the handler is marked dirty when the handler returns; an
	// unchanged tape costs one view procedure and a compare.
	if owner != 0 && !h.quiet {
		mark_dirty(cx.turn, owner)
	}
}

// handler keeps a handler that returns bool and belongs to no tape, and returns its slot
// (the runtime's tests fire it with test_fire).
handler :: proc(fn: proc(s: ^$S, cx: ^Cx) -> bool, loc := #caller_location) -> u32 {
	if cur == nil {
		stop(loc, "gk.handler was called before the app started: make handlers in init, a handler or a view procedure")
	}
	slot, _ := keep_handler(&cur.cx, quiet(on_ret(fn, loc)), SHAPE_UNIT_BOOL, loc)
	return slot
}

@(private)
eprintf_report :: proc() {
	fmt.eprintf("gk: dropped events %d, turns %d, bytes held for the app %d, kept handlers %d\n", dropped, turn_serial, sync.atomic_load(&owned_bytes), len(kept) - len(kept_free))
}
