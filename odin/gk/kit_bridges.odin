package gk

// The hand-written bridges: what the generator does not emit yet and the examples need,
// written as generated code is, over the emitter's interface (rt_emit.odin). The Rust side
// is core/src/hand.rs.
//
// Rows and uniform_list: a list whose rows are kept in the library (DESIGN.md section 9).

// --- UniformList and its rows -------------------------------------------------------------------

// Rows is a handle to the rows of a list, kept in the library.
Rows :: struct {
	h:        u64,
	using vt: ^Rows_VT,
}

Rows_VT :: struct {
	// push appends rows: one buffered op however many rows.
	push: proc(r: Rows, cx: ^Cx, rows: ..string),
}

rows :: proc(cx: ^Cx, loc := #caller_location) -> Rows {
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_ROWS_NEW)
	return {state_make(cx, req[:], loc), &rows_vt}
}

@(private)
rows_vt := Rows_VT {
	push = proc(r: Rows, cx: ^Cx, rows: ..string) {
		req := make(Tape, 0, 64, context.temp_allocator)
		put_u16(&req, C_ROWS_PUSH)
		put_u64(&req, r.h)
		put_u32(&req, u32(len(rows)))
		for s in rows {
			put_str(&req, s)
		}
		state_set(cx, req[:])
	},
}

// Uniform_List is GPUI's uniform_list over rows kept in the library.
Uniform_List :: struct {
	using el: Element,
	using vt: ^Uniform_List_VT,
}

Uniform_List_VT :: struct {
	using t_styled: ^Styled_Trait(Uniform_List),
}

uniform_list :: proc(id: string, rows: Rows, flags: ..Style_Flag, loc := #caller_location) -> Uniform_List {
	e := Uniform_List{new_node(N_UNIFORM_LIST, loc), &uniform_list_vt}
	put_str(&build.ops, id)
	put_u64(&build.ops, rows.h)
	tokens(e.el, flags)
	return e
}

@(private)
uniform_list_vt := Uniform_List_VT {
	t_styled = cast(^Styled_Trait(Uniform_List))&styled_any,
}

// text is a string as an element, for a builder that takes any element where a Rust app
// would pass a string (GroupBox::title("Title")): gk.group_box()->title(gk.text("Title")).
// For a child, the parent's own text method is shorter: gk.div()->text("hello").
text :: proc(s: string, loc := #caller_location) -> Element {
	e := new_node(N_TEXT, loc)
	put_str(&build.ops, s)
	return e
}
