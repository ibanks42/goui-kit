package gk

// Data closures answered between frames (docs/PROTOCOLS.md 9.14). A closure upstream calls while it draws and that returns a value (a code block's
// highlights, an image for a URI, a calendar label) is an *ask*: the app gives a procedure once,
//
//   a.hl = gk.ask(cx, a, highlight)    highlight :: proc(a: ^App, cx: ^gk.Cx, b: gk.Code_Block) -> []gk.Highlight_Span
//   gk.text_view(..)->code_block_highlighter(a.hl)
//
// and the library answers upstream from a cache keyed by the arguments. A key it has not got is
// drawn with the type's default for a frame or two while the library asks the app between frames
// (the procedure runs as a handler of the ask's state; what it returns is cached for that key).
// The procedure's argument and result types are the closure's, checked by the compiler: an ask
// of the wrong types does not fit the builder. Release it with gk.release(cx, a.hl.src).

// Ask is a data closure's procedure, kept by the library with its cache of answers: K the
// closure's arguments (one argument, or a Tuple_ of them), V its result.
Ask :: struct($K, $V: typeid) {
	src: Row_Source,
}

// Code_Block is a fenced code block as a highlighter gets it: its language (the word after the
// fence), its code, and the rest of the fence's info string.
Code_Block :: struct {
	lang: Maybe(Temp_String),
	code: Temp_String,
	meta: Maybe(Temp_String),
}

@(init, private)
text_shapes :: proc "contextless" () {
	register_shape(SHAPE_ASK_EVENT, nil, nil)
}

// ask keeps `procedure` as a data closure's procedure with the state `state`: the library asks it
// for each key it draws without an answer, between frames, and caches what it returns. Needs a
// window: call it in init or in a handler, and keep the gk.Ask.
ask :: proc(cx: ^Cx, state: ^$S, procedure: proc(s: ^S, cx: ^Cx, key: $K) -> $V, loc := #caller_location) -> Ask(K, V) {
	if build.active {
		stop(loc, "a view procedure may not make an ask: make it in init or in a handler and keep the gk.Ask")
	}
	call_it :: proc(h: ^Handler, state: rawptr, cx: ^Cx, r: ^Reader) {
		id := rd_u64(r)
		src := Row_Source{rd_u64(r)}
		k: K
		rd_value(r, &k, type_info_of(K))
		v := (cast(proc(s: ^S, cx: ^Cx, key: K) -> V)h.fn)(cast(^S)state, cx, k)
		answer(cx, src, id, v)
	}
	none :: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element {
		return {}
	}
	// No redraw of the view that made it: the library redraws the windows when the answer is in.
	h := Handler{call = call_it, fn = rawptr(procedure), state_type = typeid_of(S), quiet = true, loc = loc}
	src := row_source(cx, state, 0, none, h, SHAPE_ASK_EVENT, initial = 0, capacity = 16, loc = loc)
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_ASK_NEW)
	put_u64(&req, src.h)
	must_call(req[:], loc)
	return {src = src}
}

// test_asks: the library's counts of data closure answers (gets, keys asked, answers cached).
test_asks :: proc(cx: ^Cx, loc := #caller_location) -> (gets, asked, answered: int) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_ASK_TEST)
	r := Reader{must_call(req[:], loc), 0}
	return int(rd_u64(&r)), int(rd_u64(&r)), int(rd_u64(&r))
}

// Lsp_Position is an LSP position: a line and a character (UTF-16 units) in it, from 0
// (`cursor_position`, PROTOCOLS.md 9.13).
Lsp_Position :: struct {
	line:      u32,
	character: u32,
}
