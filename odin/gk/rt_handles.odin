package gk

// Library objects behind handles (docs/PROTOCOLS.md section 13): kit values that are not
// entities (a dialog's handle, a scroll handle, a search matcher, the theme). A generated
// object type is struct {using state: State, using vt: ^T_VT}, like kit state, with a handle of
// kind OBJ. What generated code uses from here: obj_get, register_obj. What the app uses:
// gk.retain.
//
// An object a constructor makes is kept until gk.release. One that a read, a call or an event
// hands out is temporary: it lives until the next frame begins, so it can be used in the turn
// that got it and by the writes that turn buffered. gk.retain keeps it.

import "base:intrinsics"
import "base:runtime"

@(private)
MAX_OBJ_TYPES :: 512

@(private)
Obj_Type :: struct {
	id: typeid,
	vt: rawptr,
}

@(private)
obj_types: [MAX_OBJ_TYPES]Obj_Type
@(private)
n_obj_types: int

// register_obj records the method table of a generated object type, so that a value of it
// read by reflection (an event's payload, a read's result) gets its table: on the wire it is
// only the handle.
@(private)
register_obj :: proc "contextless" (id: typeid, vt: rawptr) {
	if n_obj_types < MAX_OBJ_TYPES {
		obj_types[n_obj_types] = {id, vt}
		n_obj_types += 1
	}
}

// read_obj reads an object handle into a value of a generated object type and gives it its
// method table. False for any other type. Called by rd_value for every struct.
@(private)
read_obj :: proc(r: ^Reader, p: rawptr, ti: ^runtime.Type_Info) -> bool {
	s, ok := runtime.type_info_base(ti).variant.(runtime.Type_Info_Struct)
	if !ok || s.field_count != 2 || s.names[0] != "state" || s.types[0].id != State {
		return false
	}
	for i in 0 ..< n_obj_types {
		if obj_types[i].id == ti.id {
			(cast(^u64)p)^ = rd_u64(r)
			intrinsics.unaligned_store(cast(^rawptr)(uintptr(p) + s.offsets[1]), obj_types[i].vt)
			return true
		}
	}
	return false
}

// obj_get runs a read call op of a library object (or of none: a global accessor). Buffered
// writes are sent first, so the read sees them. Unlike a read of kit state it records no
// dependency: an object does not notify; the view that shows one is described again when a
// handler marks it (a handler marks its own view).
@(private)
obj_get :: proc(h: u64, cx: ^Cx, req: []u8, loc := #caller_location) -> Reader {
	flush(check_cx(cx, loc), loc)
	rc, res := call(req)
	switch rc {
	case 0:
		return Reader{res, 0}
	case ERR_POISONED:
		library_panic(last_error(), loc)
	}
	stop_call(loc, rc)
}

// retain keeps a temporary library object (one a read, a call or an event handed out) until
// gk.release. A kept object or a global is unchanged.
retain :: proc(cx: ^Cx, x: $T, loc := #caller_location) -> T where intrinsics.type_has_field(T, "state") {
	t := check_cx(cx, loc)
	flush(t, loc)
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_OBJ_RETAIN)
	put_u64(&req, x.h)
	must_call(req[:], loc)
	return x
}

// test_obj_counts: temporary handles waiting for the next frame, and live library objects
// (core/tests/handles).
test_obj_counts :: proc(cx: ^Cx, loc := #caller_location) -> (temps, live: int) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_OBJ_TEST_TEMPS)
	r := Reader{must_call(req[:], loc), 0}
	return int(rd_u32(&r)), int(rd_u32(&r))
}

// --- builder values made outside a view (docs/PROTOCOLS.md section 13.6) ------------------------
//
// A builder value that is not an element (a grid area, a tree item, an editor decoration) made
// in a handler or in init, where no view is being built, goes into this value build. A call op
// that takes one carries it as a small tape of its own (put_vtape), which the library decodes
// with the value's generated decoder. The values of a turn live until the outermost turn ends:
// one kept longer stops when it is used.

@(private)
vbuild: Build
@(private)
vbuild_turn: u64

@(private)
outer_turn :: proc() -> u64 {
	t := cur
	if t == nil {
		return 0
	}
	for t.parent != nil {
		t = t.parent
	}
	return t.serial
}

// value_new starts a value node in the value build (new_value when no view is being built).
@(private)
value_new :: proc(typ: u16, loc := #caller_location) -> Value {
	if cur == nil {
		stop(loc, "a kit value was made outside the app's procedures: make it in init, a handler or a view procedure (after gk.run has started the app)")
	}
	if outer := outer_turn(); vbuild_turn != outer || vbuild.serial == 0 {
		vbuild_turn = outer
		clear(&vbuild.ops)
		clear(&vbuild.nodes)
		build_serial += 1
		vbuild.serial = build_serial
		append(&vbuild.ops, 0) // offset 0 means "no op": nothing may live there
	}
	append(&vbuild.nodes, Node_Rec{typ = typ, args = u32(len(vbuild.ops))})
	vbuild.cur = u32(len(vbuild.nodes) - 1)
	return {vbuild.cur, vbuild.serial}
}

// value_op is op() for a node of the value build.
@(private)
value_op :: proc(e: Element, code: u16) -> ^Tape {
	b := &vbuild
	if b.serial != e.serial || int(e.node) >= len(b.nodes) || vbuild_turn != outer_turn() {
		stop({}, "a kit value (a column, a menu item, a style) was used after the procedure that made it returned: values live for one call of init, a handler or a view procedure. Make it again where it is used")
	}
	n := &b.nodes[e.node]
	if n.attached {
		stop({}, "a kit value was changed after it was given to a builder or a call: finish its builders first, then give it")
	}
	at := u32(len(b.ops))
	p := grow(&b.ops, 6)
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

// value_take marks a value of the value build used (an argument of another value or a call).
@(private)
value_take :: proc(v: Value) -> u32 {
	if v.serial != vbuild.serial || int(v.node) >= len(vbuild.nodes) || vbuild_turn != outer_turn() {
		stop({}, "a kit value was used after the procedure that made it returned: values live for one call of init, a handler or a view procedure. Make it again where it is used")
	}
	if v.node == vbuild.cur && !vtape_taking {
		// v->op(v): the library would decode the value inside itself until its stack ran out.
		stop({}, "a value was given as an argument to its own builder")
	}
	vbuild.nodes[v.node].attached = true
	return v.node
}

// True while put_vtape takes the value it writes (a call's argument, not a builder's).
@(private)
vtape_taking: bool

// value_args is node_args for the value build.
@(private)
value_args :: proc() -> ^Tape {
	vbuild.nodes[len(vbuild.nodes) - 1].args = u32(len(vbuild.ops))
	return &vbuild.ops
}

// put_vtape writes a value as a call op's argument: u32 length, then a tape whose root is the
// value (the value build so far, without handlers, dependencies or slots).
@(private)
put_vtape :: proc(o: ^Tape, v: Value) {
	if build.active && v.serial == build.serial {
		stop({}, "a value made in a view procedure was given to a call: make it in the handler that calls")
	}
	vtape_taking = true
	node := value_take(v)
	vtape_taking = false
	b := &vbuild
	put_u32(o, u32(4 * 4 + 2 + 2 + 4 + len(b.ops) + len(b.nodes) * 10))
	put_u32(o, TAPE_MAGIC)
	put_u32(o, u32(len(b.nodes)))
	put_u32(o, node)
	put_u32(o, 0)
	put_u16(o, 0)
	put_u16(o, 0)
	put_u32(o, u32(len(b.ops)))
	append(o, ..b.ops[:])
	for nd in b.nodes {
		put_u16(o, nd.typ)
		put_u32(o, nd.args)
		put_u32(o, nd.first)
	}
}
