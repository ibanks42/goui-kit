package gk

// The emitter's interface: everything generated Odin (gen_*.odin) may call
// (docs/PROTOCOLS.md section 7). Together with the writers and readers of rt_wire.odin,
// register_shape and keep_handler of rt_handler.odin, and the types Handler, Temp_String and
// Cx, this is the whole surface. Nothing else in rt_*.odin is for generated code.
//
//   elements:  Element  new_node  op  attach  slot_view  tokens  register  put_color
//   state:     State  state_make  state_get  state_try  state_set  state_call  dep  focus_new  focus_set
//   wire:      put_u8 put_bool put_u16 put_u32 put_u64 put_i32 put_i64 put_f32 put_f64 put_str
//              Reader rd_u8 rd_bool rd_u16 rd_u32 rd_u64 rd_i32 rd_i64 rd_f32 rd_f64 rd_temp
//              rd_into (any wire type, by reflection)  Hsla  Color  hsla  put_color
//
// The generated gen_support.odin has take, take_value and node_args, which reach into the
// build in progress: the fields of Build and Node_Rec below are part of this interface too
// (ops, nodes, serial, active; typ, args, first, last, attached).
//
// Names this runtime leaves to the generated package: Any_El, Value, Length, px, rems,
// relative, auto, Id, id, Range, Handle, Action, theme, palette, Focus_Handle, focus_handle,
// focus, Modifiers, Mouse_Button, Style_Flag.

import "base:intrinsics"
import "base:runtime"

// Element is an element being described: a node of the view build in progress.
Element :: struct {
	node:   u32,
	serial: u32,
}

@(private)
Node_Rec :: struct {
	typ:               u16,
	args, first, last: u32,
	attached:          bool,
}

// Build is the tape of one view build: an op stream and a node table. A builder call appends
// one op and patches the previous op's link; attaching a child writes its node index.
@(private)
Build :: struct {
	ops:      Tape,
	nodes:    [dynamic]Node_Rec,
	locs:     [dynamic]runtime.Source_Code_Location, // GK_DEBUG=1: where each node was made
	handlers: [dynamic]Handler,
	deps:     [dynamic]u64,
	slots:    [dynamic]u64,
	view:     ^View_Rec,
	serial:   u32,
	active:   bool,
	cur:      u32, // the node whose op or constructor arguments are being written
}

@(private)
build: Build
@(private)
build_serial: u32

@(private)
TAPE_MAGIC :: 0x31544b47

@(private)
finish :: proc(root: Element) -> Tape {
	b := &build
	if !b.active || root.serial != b.serial {
		stop({}, "a view procedure returned an element it did not make in this call (one kept from an earlier call, or made by another view): return an element the procedure built just now%s", element_origin(0, false))
	}
	b.active = false
	out := make(Tape, 0, 32 + len(b.ops) + len(b.nodes) * 10 + (len(b.deps) + len(b.slots)) * 8, context.temp_allocator)
	put_u32(&out, TAPE_MAGIC)
	put_u32(&out, u32(len(b.nodes)))
	put_u32(&out, root.node)
	put_u32(&out, u32(len(b.handlers)))
	put_u16(&out, u16(len(b.deps)))
	for d in b.deps {
		put_u64(&out, d)
	}
	put_u16(&out, u16(len(b.slots)))
	for s in b.slots {
		put_u64(&out, s)
	}
	put_u32(&out, u32(len(b.ops)))
	append(&out, ..b.ops[:])
	for n in b.nodes {
		put_u16(&out, n.typ)
		put_u32(&out, n.args)
		put_u32(&out, n.first)
	}
	return out
}

// new_node starts an element of a node type. The constructor's arguments are appended to
// build.ops right after it, before any op.
@(private)
new_node :: proc(typ: u16, loc := #caller_location) -> Element {
	b := &build
	if !b.active {
		stop(loc, "an element was made outside a view procedure: gk.button, gk.div, gk.label and the rest describe what a view shows, in the procedure given to gk.view. A handler changes the app's state; the view procedure then describes it again")
	}
	append(&b.nodes, Node_Rec{typ = typ, args = u32(len(b.ops))})
	if debug {
		append(&b.locs, loc)
	}
	b.cur = u32(len(b.nodes) - 1)
	return {b.cur, b.serial}
}

// op starts an op of this element and returns the op stream to append its arguments to.
// (No caller location here or in attach, tokens and slot_view: they are called from the
// generated method tables, whose own lines mean nothing to the app, and they run hundreds
// of times per described view.)
@(private)
op :: proc(e: Element, code: u16) -> ^Tape {
	b := &build
	if (!b.active || b.serial != e.serial) && e.serial == vbuild.serial && vbuild.serial != 0 {
		return value_op(e, code) // a value made outside a view (rt_handles.odin)
	}
	if !b.active || b.serial != e.serial {
		stop({}, "an element was used after the view procedure that made it returned: an element lives for one call of the view procedure. Make it again in each call (a procedure that returns the element is how a piece of a view is reused)%s", element_origin(e.node, false))
	}
	n := &b.nodes[e.node]
	if n.attached {
		stop({}, "an element was changed after it was given to its parent (->child or another builder): call its builders first, then give it, as in row->child(gk.label(..)->text_color(..))%s", element_origin(e.node, true))
	}
	at := u32(len(b.ops))
	p := grow(&b.ops, 6) // u16 opcode, u32 link to the node's next op (0: none yet)
	if n.last == 0 {
		n.first = at
	} else {
		intrinsics.unaligned_store(cast(^u32)&b.ops[n.last + 2], at)
	}
	n.last = at
	b.cur = e.node
	intrinsics.unaligned_store(cast(^u16)p, code)
	intrinsics.unaligned_store(cast(^u32)&p[2], u32(0))
	return &b.ops
}

// attach makes `child` a child of `e` (the CHILD op). A node is attached once.
@(private)
attach :: proc(e, child: Element) {
	if child.serial != build.serial || !build.active {
		stop({}, "an element was used after the view procedure that made it returned: an element lives for one call of the view procedure. Make it again in each call (a procedure that returns the element is how a piece of a view is reused)%s", element_origin(child.node, false))
	}
	if build.nodes[child.node].attached {
		stop({}, "an element was given to a second parent: an element has one place in a view. Make another one for the second place (a procedure that returns it, called twice)%s", element_origin(child.node, true))
	}
	if child.node == e.node {
		// The library would decode the node inside itself until its stack ran out.
		stop({}, "an element was made a child of itself (x->child(x))%s", element_origin(e.node, true))
	}
	o := op(e, E_CHILD)
	build.nodes[child.node].attached = true
	put_u32(o, child.node)
}

// slot_view embeds a view in `e` (the CHILD_VIEW op). A child's change does not alter the
// parent's tape, so the parent stays retained when the child is described again.
@(private)
slot_view :: proc(e: Element, v: View) {
	put_u64(op(e, E_CHILD_VIEW), v.h)
	append(&build.slots, v.h)
	if rec, ok := views[v.h]; ok && !rec.described && cur != nil {
		append(&cur.pending, rec)
	}
}

// tokens writes style tokens (the STYLE op).
@(private)
tokens :: proc(e: Element, flags: []Style_Flag) {
	if len(flags) == 0 {
		return
	}
	o := op(e, E_STYLE)
	p := grow(o, 2 + 2 * len(flags))
	intrinsics.unaligned_store(cast(^u16)p, u16(len(flags)))
	for f, i in flags {
		intrinsics.unaligned_store(cast(^u16)&p[2 + 2 * i], u16(f))
	}
}

// register puts a handler into the table of the view being built and returns its id, which
// the builder writes into the op. The handler's state type, event type and return type are
// checked here, against the view and the closure's shape; a mismatch stops at the line of
// the gk.on call.
@(private)
register :: proc(h: Handler, shape: u16) -> u32 {
	if !build.active {
		stop(h.loc, "a handler for an element was made outside a view procedure: give gk.on(..) to an element's builder (->on_click) while the view procedure describes it")
	}
	check_shape(h, shape)
	if h.state_type != build.view.state_type {
		stop(h.loc, "a handler takes ^%v but its view's state is %v: a handler given in a view procedure takes the state that view was made with (gk.view(state, ..)), as in proc(s: ^%v, cx: ^gk.Cx)", h.state_type, build.view.state_type, build.view.state_type)
	}
	take_owned(h)
	append(&build.handlers, h)
	return u32(len(build.handlers) - 1)
}

// --- kit state behind handles -----------------------------------------------------------------

// State is a handle to a kit entity that lives in the library. A generated state type is
// struct {using state: State}, with procedures named after it (gk.input_state_value(s, cx)).
State :: struct {
	h: u64,
}

// dep records that the view being built read this state: the library observes it and has
// the view described again when it changes.
@(private)
dep :: proc(h: u64) {
	if !build.active {
		return
	}
	for d in build.deps {
		if d == h {
			return
		}
	}
	append(&build.deps, h)
}

// state_make runs a constructor call op and returns the new handle.
@(private)
state_make :: proc(cx: ^Cx, req: []u8, loc := #caller_location) -> u64 {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not create kit state (it runs on every redraw): create it once in init or in a handler and keep the handle in your state struct, as in a.search = gk.input_state(cx)->build()")
	}
	flush(t, loc)
	r := Reader{must_call(req, loc), 0}
	h := rd_u64(&r)
	kept_attach(h) // handlers given to its builders belong to it (kit_editor.odin)
	return h
}

// state_try runs a read call op (a `&self` method). Buffered writes are sent first, so the
// read sees them. Inside a view build the state is recorded as a dependency. ok is false
// when the state is being updated by the component that called the handler.
@(private)
state_try :: proc(h: u64, cx: ^Cx, req: []u8, loc := #caller_location) -> (r: Reader, ok: bool) {
	flush(check_cx(cx, loc), loc)
	dep(h)
	rc, res := call(req)
	switch rc {
	case 0:
		return Reader{res, 0}, true
	case ERR_BUSY:
		return {}, false
	case ERR_POISONED:
		library_panic(last_error(), loc)
	}
	stop_call(loc, rc)
}

// state_get is state_try for the usual case: a busy state stops with the library's message.
@(private)
state_get :: proc(h: u64, cx: ^Cx, req: []u8, loc := #caller_location) -> Reader {
	r, ok := state_try(h, cx, req, loc)
	if !ok {
		stop(loc, "%s", last_error())
	}
	return r
}

// state_call runs an update that returns a value (a `&mut self` method with a result) at
// once: buffered writes are sent first, the state is updated, the result comes back like a
// read's. A view procedure may not call it (it writes); a busy state stops with the
// library's message.
@(private)
state_call :: proc(h: u64, cx: ^Cx, req: []u8, loc := #caller_location) -> Reader {
	if build.active {
		stop(loc, "a view procedure may not change kit state (it only describes): change it in a handler or in init")
	}
	flush(check_cx(cx, loc), loc)
	kept_attach_req(req) // handlers given to the call belong to its state (kit_editor.odin)
	rc, res := call(req)
	switch rc {
	case 0:
		return Reader{res, 0}
	case ERR_POISONED:
		library_panic(last_error(), loc)
	}
	stop_call(loc, rc)
}

// state_set buffers a write call op (a `&mut self` method).
@(private)
state_set :: proc(cx: ^Cx, req: []u8, loc := #caller_location) {
	kept_attach_req(req) // handlers given to the write belong to its state (kit_editor.odin)
	defer_op(check_cx(cx, loc), req, loc)
}

// rd_into reads a value of any wire type by the reflection that reads event payloads: the
// result of a state read can be a record, an option or a list.
@(private)
rd_into :: proc(r: ^Reader, v: ^$T) {
	rd_value(r, v, type_info_of(T))
}

// --- colours ----------------------------------------------------------------------------------

// Hsla is GPUI's Hsla as a value (in an event, in a read's result).
Hsla :: struct {
	h, s, l, a: f32,
}

// Color is an HSLA literal or a theme token, resolved by the library when it decodes.
Color :: struct {
	theme:      u16,
	h, s, l, a: f32,
}

hsla :: proc(h, s, l, a: f32) -> Color {
	return {0, h, s, l, a}
}

@(private)
put_color :: proc(b: ^Tape, c: Color) {
	if c.theme != 0 {
		append(b, COLOR_THEME)
		put_u16(b, c.theme)
		return
	}
	append(b, COLOR_HSLA)
	put_f32(b, c.h)
	put_f32(b, c.s)
	put_f32(b, c.l)
	put_f32(b, c.a)
}

// linear_color_stop is gpui's linear_color_stop: c at percentage (0 to 1) of a gradient
// (gk.Linear_Color_Stop is the kit's record of it).
linear_color_stop :: proc(c: Color, percentage: f32) -> Linear_Color_Stop {
	return {c, percentage}
}

// Linear_Gradient is gpui's linear_gradient: from one stop to the other along angle (degrees; 0 is
// up, 90 left to right, increasing clockwise), mixed in sRGB or, with oklab, in the Oklab space.
Linear_Gradient :: struct {
	angle:    f32,
	from, to: Linear_Color_Stop,
	oklab:    bool,
}

// linear_gradient is gpui's linear_gradient(angle, from, to):
//   ->bg(gk.linear_gradient(90, gk.linear_color_stop(cyan, 0), gk.linear_color_stop(violet, 1)))
linear_gradient :: proc(angle: f32, from, to: Linear_Color_Stop, oklab := false) -> Linear_Gradient {
	return {angle, from, to, oklab}
}

// Fill is what bg and the plot's paths and quads take (gpui's Fill and Background): a Color (it
// converts by itself, so ->bg(gk.theme(.Background)) compiles as before) or a Linear_Gradient.
// PROTOCOLS.md 11.38.
Fill :: union #no_nil {
	Color,
	Linear_Gradient,
}

@(private)
put_fill :: proc(b: ^Tape, f: Fill) {
	switch x in f {
	case Color:
		put_color(b, x)
	case Linear_Gradient:
		append(b, COLOR_GRADIENT)
		put_f32(b, x.angle)
		put_u8(b, x.oklab ? 1 : 0)
		put_color(b, x.from.color)
		put_f32(b, x.from.percentage)
		put_color(b, x.to.color)
		put_f32(b, x.to.percentage)
	}
}
