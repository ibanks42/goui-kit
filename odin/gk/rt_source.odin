package gk

// Row sources: the shared core of the delegate protocol (docs/PROTOCOLS.md section 9). A row
// source holds the rows of a list, table or tree in the library; each row is a view of its
// own, described by the app's row procedure for that row, once, and retained like any view
// until it is invalidated or evicted. The library binds row views to rows before a frame (the
// ROWS turn): a row drawn as a placeholder in one frame is described before the next. App
// code never runs in the middle of a frame.
//
//   src := gk.row_source(cx, a, len(a.items), item_row)        in init or in a handler
//   item_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element { .. a.items[at.row] .. }
//   gk.set_count(cx, src, n)    gk.invalidate(cx, src, lo, hi)    gk.release(cx, src)

import "base:runtime"

// Row_At is the row a row view describes: its section (0 without sections) and its row.
Row_At :: struct {
	section, row: int,
}

// Row_Source is a handle to a row source, which lives in the library.
Row_Source :: struct {
	h: u64,
}

// Source_Test is the event of the row sources' own test request (SHAPE_SOURCE_TEST): a
// request id and a text, answered with gk.answer.
Source_Test :: struct {
	id:   u64,
	text: Temp_String,
}

@(init, private)
source_shapes :: proc "contextless" () {
	register_shape(SHAPE_SOURCE_TEST, Source_Test, nil)
}

@(private)
Source_Rec :: struct {
	h:          u64,
	state:      rawptr,
	state_type: typeid,
	row_fn:     rawptr,
	row_call:   proc(fn, state: rawptr, cx: ^Cx, at: Row_At) -> Element,
	rows:       map[u64]u64, // row key to row view
	events:     u32, // kept handler slot + 1; 0 for none
	items_kind: int, // gk.items: the kind of item procedure (gen_items.odin), 0 for any other source
	count:      int, // section 0's count as the binding last set it
	count_fn:   rawptr, // a count procedure (gk.list_source given one): gk.refresh reads it
	count_call: proc(fn, state: rawptr) -> int,
}

@(private)
sources: map[u64]^Source_Rec

// View records of rows that were evicted or dropped: freed when the outermost turn has
// ended, since a handler of one of them may be running.
@(private)
dead_rows: [dynamic]^View_Rec

// MAX_ROWS is the most rows a section of a row source holds (a table's count is its cells:
// rows times columns). A row's number is 32 bits and the two highest are a section's header
// and footer; a larger count made the rows past it show the views of other rows.
MAX_ROWS :: 0xffff_fffe

@(private)
check_count :: proc(count: int, what: string, loc: runtime.Source_Code_Location) {
	if count < 0 {
		stop(loc, "%s was given a negative count (%d)", what, count)
	}
	if count > MAX_ROWS {
		stop(loc, "%s was given %d rows; a row source holds at most %d in a section (a table: rows times columns)", what, count, MAX_ROWS)
	}
}

@(private)
row_key :: proc(at: Row_At) -> u64 {
	return u64(at.section) << 32 | u64(u32(at.row))
}

// row_source makes a source of `count` rows (one section) whose rows `row` describes, with
// the state `state`. `events`, if given, receives the source's events (a delegate family's
// event shape, `events_shape`; the families of docs/PROTOCOLS.md section 9 pass their own).
// The first `initial` rows get their views at once, so the first frame that draws the source
// is complete; at most `capacity` row views are kept, far ones are evicted. Needs a window:
// call it in a view's init procedure or in a handler.
row_source :: proc(
	cx: ^Cx,
	state: ^$S,
	count: int,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element,
	events: Handler = {},
	events_shape: u16 = 0,
	initial := 64,
	capacity := 512,
	loc := #caller_location,
) -> Row_Source {
	if build.active {
		_ = check_cx(cx, loc)
		stop(loc, "a view procedure may not create row sources: create them in init or in a handler and keep the gk.Row_Source")
	}
	return new_source(cx, state, count, row, events, events_shape, initial, capacity, loc)
}

// new_source is row_source without its rule against a view procedure: gk.items makes its source of
// no rows on first use, in a view too (rt_items.odin).
@(private)
new_source :: proc(
	cx: ^Cx,
	state: ^$S,
	count: int,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element,
	events: Handler,
	events_shape: u16,
	initial, capacity: int,
	loc: runtime.Source_Code_Location,
) -> Row_Source {
	t := check_cx(cx, loc)
	if count < 0 || initial < 0 || capacity < 0 {
		stop(loc, "a negative number was given to gk.row_source (count %d, initial %d, capacity %d)", count, initial, capacity)
	}
	check_count(count, "gk.row_source", loc)
	tramp :: proc(fn, state: rawptr, cx: ^Cx, at: Row_At) -> Element {
		return (cast(proc(s: ^S, cx: ^Cx, at: Row_At) -> Element)fn)(cast(^S)state, cx, at)
	}
	slot, gen: u32
	has := events.call != nil
	if has {
		slot, gen = keep_handler(cx, events, events_shape, loc)
	}
	flush(t, loc)
	req := make(Tape, 0, 40, context.temp_allocator)
	put_u16(&req, C_SOURCE_NEW)
	put_u64(&req, u64(count))
	put_u32(&req, u32(initial))
	put_u32(&req, u32(capacity))
	put_bool(&req, has)
	put_u32(&req, slot)
	put_u32(&req, gen)
	r := Reader{must_call(req[:], loc), 0}
	rec := new(Source_Rec, runtime.heap_allocator())
	rec.h = rd_u64(&r)
	rec.state = state
	rec.state_type = typeid_of(S)
	rec.row_fn = rawptr(row)
	rec.row_call = tramp
	rec.rows = make(map[u64]u64, 64, runtime.heap_allocator())
	rec.events = has ? slot + 1 : 0
	rec.count = count
	if sources == nil {
		sources = make(map[u64]^Source_Rec, 8, runtime.heap_allocator())
	}
	sources[rec.h] = rec
	n := rd_u32(&r) // read once: a range's end is evaluated on every pass
	for _ in 0 ..< n {
		key := rd_u64(&r)
		bind_row(rec, key, rd_u64(&r))
	}
	return {rec.h}
}

// set_count changes the number of rows of a section. Rows at and past the new count are
// dropped; rows below it keep their views (invalidate them if their content moved).
set_count :: proc(cx: ^Cx, src: Row_Source, count: int, section := 0, loc := #caller_location) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not write: it only describes (gk.set_count). Change rows in a handler, in init or in a frame handler")
	}
	rec := source_rec(src, loc)
	check_count(count, "gk.set_count", loc)
	if section == 0 {
		rec.count = count
	}
	lo := row_key({section, count})
	hi := u64(section + 1) << 32
	gone := make([dynamic]u64, 0, 16, context.temp_allocator)
	for key, view in rec.rows {
		if key >= lo && key < hi {
			append(&gone, view)
		}
	}
	for view in gone {
		unbind_row(view)
	}
	req := make(Tape, 0, 24, context.temp_allocator)
	put_u16(&req, C_SOURCE_SET_COUNT)
	put_u64(&req, src.h)
	put_u32(&req, u32(section))
	put_u64(&req, u64(count))
	defer_op(t, req[:], loc)
}

// invalidate has the rows lo ..< hi of a section described again before the next frame (the
// rows that have a view; the others are described when they are drawn).
invalidate :: proc(cx: ^Cx, src: Row_Source, lo, hi: int, section := 0, loc := #caller_location) {
	t := check_cx(cx, loc)
	rec := source_rec(src, loc)
	if build.active {
		stop(loc, "gk.invalidate in a view procedure: a view procedure only describes. Call it in the handler that changed the rows (or call gk.refresh there)")
	}
	a, b := row_key({section, max(lo, 0)}), row_key({section, max(hi, 0)})
	for key, view in rec.rows {
		if key >= a && key < b {
			mark_dirty(t, view)
		}
	}
}

// refresh is for after the app's data changed: every view is described again before the next
// frame, and so is every row a list, table or tree shows; a list made with a count procedure
// (gk.list_source(cx, a, task_count, task_row)) first takes its number of rows from it. The app
// needs no handle: not the views that show the data, not the sources, not the rows. A description
// that did not change costs a compare. In a big app, gk.notify and gk.invalidate redraw less.
//   toggle :: proc(a: ^App, cx: ^gk.Cx, task: int) {a.tasks[task].done = !a.tasks[task].done; gk.refresh(cx)}
refresh :: proc(cx: ^Cx, loc := #caller_location) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.refresh in a view procedure: a view procedure only describes. Call it at the end of the handler that changed the data")
	}
	for h, rec in sources {
		if rec.count_call != nil {
			if n := rec.count_call(rec.count_fn, rec.state); n != rec.count {
				set_count(cx, Row_Source{h}, n, loc = loc)
			}
		}
	}
	for h, v in views {
		if !v.dead {
			mark_dirty(t, h)
		}
	}
}

// counted gives a source a count procedure, which gk.refresh reads.
@(private)
counted :: proc(src: Row_Source, count: proc(s: ^$S) -> int) {
	tramp :: proc(fn, state: rawptr) -> int {
		return (cast(proc(s: ^S) -> int)fn)(cast(^S)state)
	}
	rec := sources[src.h]
	rec.count_fn = rawptr(count)
	rec.count_call = tramp
}

// answer answers a request a row source made (a provider method that returns a task upstream):
// `value` in the wire type the request's family fixed. A buffered write.
answer :: proc(cx: ^Cx, src: Row_Source, id: u64, value: $T, loc := #caller_location) {
	t := check_cx(cx, loc)
	v := value
	body := make(Tape, 0, 32, context.temp_allocator)
	put_value(&body, &v, type_info_of(T))
	req := make(Tape, 0, 24 + len(body), context.temp_allocator)
	put_u16(&req, C_SOURCE_ANSWER)
	put_u64(&req, src.h)
	put_u64(&req, id)
	put_u32(&req, u32(len(body)))
	append(&req, ..body[:])
	defer_op(t, req[:], loc)
}

// source_list is the shared core's own list element: a uniform list over a row source whose
// rows are `row_height` high. The delegate families draw sources with the kit's own lists,
// tables and trees; this one is for the runtime's tests and for plain lists.
source_list :: proc(id: string, src: Row_Source, row_height: f32, flags: ..Style_Flag, loc := #caller_location) -> Uniform_List {
	e := Uniform_List{new_node(N_SOURCE_LIST, loc), &uniform_list_vt}
	put_str(&build.ops, id)
	put_u64(&build.ops, src.h)
	put_f32(&build.ops, row_height)
	tokens(e.el, flags)
	return e
}

// scroll_to scrolls the core list of a source so that `row` is at the top. A buffered write.
scroll_to :: proc(cx: ^Cx, src: Row_Source, row: int, loc := #caller_location) {
	t := check_cx(cx, loc)
	req := make(Tape, 0, 18, context.temp_allocator)
	put_u16(&req, C_SOURCE_SCROLL_TO)
	put_u64(&req, src.h)
	put_u64(&req, u64(max(row, 0)))
	defer_op(t, req[:], loc)
}

// --- the runtime's side ------------------------------------------------------------------------

@(private)
source_rec :: proc(src: Row_Source, loc: runtime.Source_Code_Location) -> ^Source_Rec {
	rec, ok := sources[src.h]
	if !ok {
		stop(loc, "this row source was released: release a list's source only after nothing shows it, or make a new one (gk.list_source, gk.row_source)")
	}
	return rec
}

// bind_row makes `view` (a view the library made) the row `key` of a source.
@(private)
bind_row :: proc(rec: ^Source_Rec, key: u64, view: u64) {
	v := new(View_Rec, runtime.heap_allocator())
	v.h = view
	v.state = rec.state
	v.state_type = rec.state_type
	v.view_fn = rec.row_fn
	v.row_call = rec.row_call
	v.row_at = Row_At{int(key >> 32), int(key & 0xffff_ffff)}
	v.src = rec.h
	v.last = make([dynamic]u8, 0, 0, runtime.heap_allocator())
	v.cur = make([dynamic]Handler, 0, 0, runtime.heap_allocator())
	if views == nil {
		views = make(map[u64]^View_Rec, 64, runtime.heap_allocator())
	}
	views[view] = v
	rec.rows[key] = view
}

// unbind_row forgets a row view: its handler data is freed and its record freed when the
// outermost turn has ended. The library releases the view handle (it evicted the row, or it
// applies the count or the release that dropped it).
@(private)
unbind_row :: proc(view: u64) {
	v, ok := views[view]
	if !ok {
		return
	}
	delete_key(&views, view)
	if rec, ok2 := sources[v.src]; ok2 {
		delete_key(&rec.rows, row_key(v.row_at))
	}
	free_handlers(v.cur[:])
	free_old_tables(v)
	v.dead = true
	if dead_rows == nil {
		dead_rows = make([dynamic]^View_Rec, 0, 64, runtime.heap_allocator())
	}
	append(&dead_rows, v)
}

// rows_turn is the ROWS turn: bind the rows the library asks for, forget the ones it evicted.
// No app code runs in it; the new rows are described in the VIEWS turn that follows.
@(private)
rows_turn :: proc(r: ^Reader) {
	// Counts are read into variables: a range's end is evaluated on every pass.
	n_sources := rd_u32(r)
	for _ in 0 ..< n_sources {
		rec := sources[rd_u64(r)] // nil if released meanwhile: then its rows are dropped
		n_new := rd_u32(r)
		for _ in 0 ..< n_new {
			key := rd_u64(r)
			view := rd_u64(r)
			if rec != nil {
				bind_row(rec, key, view)
			}
		}
		n_gone := rd_u32(r)
		for _ in 0 ..< n_gone {
			unbind_row(rd_u64(r))
		}
	}
}

@(private)
release_source :: proc(t: ^Turn, src: Row_Source, loc: runtime.Source_Code_Location) {
	rec := source_rec(src, loc)
	views_of := make([dynamic]u64, 0, len(rec.rows), context.temp_allocator)
	for _, view in rec.rows {
		append(&views_of, view)
	}
	for view in views_of {
		unbind_row(view)
	}
	if rec.events != 0 {
		drop_handler(rec.events - 1)
	}
	delete_key(&sources, src.h)
	delete(rec.rows)
	free(rec, runtime.heap_allocator())
	release_handle(t, src.h, loc)
}

// sweep_rows frees the records of dropped rows when no turn is running.
@(private)
sweep_rows :: proc() {
	for v in dead_rows {
		delete(v.cur)
		delete(v.last)
		free(v, runtime.heap_allocator())
	}
	clear(&dead_rows)
}

// --- test hooks (core/tests) --------------------------------------------------------------------

// test_source_request has the library ask the source's events handler a question (Source_Test),
// as a provider would; test_source_take returns the answer once it has arrived.
test_source_request :: proc(cx: ^Cx, src: Row_Source, text: string, loc := #caller_location) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 16 + len(text), context.temp_allocator)
	put_u16(&req, C_TEST_SOURCE_REQUEST)
	put_u64(&req, src.h)
	put_str(&req, text)
	must_call(req[:], loc)
}

test_source_take :: proc(cx: ^Cx, src: Row_Source, loc := #caller_location) -> (text: Temp_String, ok: bool) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_TEST_SOURCE_TAKE)
	put_u64(&req, src.h)
	r := Reader{must_call(req[:], loc), 0}
	if !rd_bool(&r) {
		return "", false
	}
	return rd_temp(&r), true
}

// test_source_rows is the number of rows of a source that have a view now.
test_source_rows :: proc(src: Row_Source) -> int {
	rec, ok := sources[src.h]
	return ok ? len(rec.rows) : 0
}

// Element closures over a context (docs/PROTOCOLS.md 9.16). Upstream draws some parts of
// an element through a closure it hands values: a token's context (`Input::token`), a diff file
// (`Diff::render_header`), an editor's scrollbar geometry (`set_scrollbar_renderer`). The app
// passes a row source instead; each call of the closure in a frame is a row of it (in the order
// of the calls, or the index when the closure's only value is one), and the row's procedure reads
// what the kit handed it:
//
//   a.tokens = gk.row_source(cx, a, 0, token_row, initial = 0)       in init
//   gk.input(a.name)->token(a.tokens)
//   token_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
//       t := gk.row_context(cx, a.tokens, at, gk.Inline_Token_Context)
//       return gk.label(t->is_selected(cx) ? "[x]" : "[ ]")
//   }
//
// A library object comes as a copy behind a temporary handle (valid while the row is described);
// several values as a struct of them in order. The row is described again when the kit hands it
// other values; that frame still shows the old row, and a new row shows nothing for one frame.

// row_context is what the element closure over a context last handed row `at` of `src`, read as
// T: the closure's value (an object type, a record, a number..) or a struct of its values in
// order. Allowed in a view procedure (it is how a row's view reads it).
row_context :: proc(cx: ^Cx, src: Row_Source, at: Row_At, $T: typeid, loc := #caller_location) -> (v: T) {
	_ = check_cx(cx, loc)
	req := make(Tape, 0, 18, context.temp_allocator)
	put_u16(&req, C_CTX_GET)
	put_u64(&req, src.h)
	put_u32(&req, u32(at.section))
	put_u32(&req, u32(at.row))
	r := Reader{must_call(req[:], loc), 0}
	rd_value(&r, &v, type_info_of(T))
	return
}

// Virtual_List is gpui-kit's virtual list over a row source (docs/PROTOCOLS.md 13.31): rows of
// different sizes, each a view described by the source's row procedure, only those in view drawn.
Virtual_List :: struct {
	using el: Element,
	using vt: ^Virtual_List_VT,
}

Virtual_List_VT :: struct {
	using t_styled: ^Styled_Trait(Virtual_List),
}

@(private)
virtual_list_vt := Virtual_List_VT {
	t_styled = cast(^Styled_Trait(Virtual_List))&styled_any,
}

// v_virtual_list lays a source's rows out top to bottom, row i `sizes[i].y` high (the width is the
// list's). h_virtual_list lays them out left to right, row i `sizes[i].x` wide. One size per row:
// rows beyond the sizes are not drawn, sizes beyond the rows neither. A row without a view yet is a
// gap of its size for one frame (the rows protocol). Style the list with tokens (.Size_Full, .H_64).
v_virtual_list :: proc(id: string, src: Row_Source, sizes: [][2]f32, flags: ..Style_Flag, loc := #caller_location) -> Virtual_List {
	return virtual_list(id, src, 0, sizes, flags, loc)
}

h_virtual_list :: proc(id: string, src: Row_Source, sizes: [][2]f32, flags: ..Style_Flag, loc := #caller_location) -> Virtual_List {
	return virtual_list(id, src, 1, sizes, flags, loc)
}

@(private)
virtual_list :: proc(id: string, src: Row_Source, axis: u8, sizes: [][2]f32, flags: []Style_Flag, loc: runtime.Source_Code_Location) -> Virtual_List {
	if src.h == 0 {
		stop(loc, "a virtual list needs a row source (gk.row_source)")
	}
	e := Virtual_List{new_node(N_VIRTUAL_LIST, loc), &virtual_list_vt}
	put_str(&build.ops, id)
	put_u64(&build.ops, src.h)
	put_u8(&build.ops, axis)
	put_u32(&build.ops, u32(len(sizes)))
	for s in sizes {
		put_f32(&build.ops, s.x)
		put_f32(&build.ops, s.y)
	}
	tokens(e.el, flags)
	return e
}
