package gk

// Views: a state pointer and a procedure that describes its elements. gpui-fast retains each
// as a view of its own, and the binding runs the procedure only when the view is dirty.

import "base:runtime"
import "core:bytes"

View :: struct {
	h: u64,
}

@(private)
View_Rec :: struct {
	h:          u64,
	state:      rawptr,
	state_type: typeid,
	init_fn:    rawptr,
	view_fn:    rawptr,
	init_call:  proc(fn, state: rawptr, cx: ^Cx),
	view_call:  proc(fn, state: rawptr, cx: ^Cx) -> Element,
	last:       [dynamic]u8, // the accepted tape
	gen:        u32, // generation of `last`
	cur:        [dynamic]Handler, // the handler table of `gen`
	// The handler tables of older generations the library still holds (an open menu made from one,
	// a closure upstream keeps): each goes when the library reports its description dropped
	// (retire_table, from a VIEWS turn).
	old:        map[u32][dynamic]Handler,
	cur_retired: bool, // the library dropped gen's description: its table is not kept when gen moves on
	build_no:   u32, // counts runs of the view procedure (what a Model's reader list refers to)
	described:  bool,
	dead:       bool, // released; the record stays, so a stale pointer to it is harmless
	// A row view (rt_source.odin): the source's row procedure for one row.
	row_call:   proc(fn, state: rawptr, cx: ^Cx, at: Row_At) -> Element,
	row_at:     Row_At,
	src:        u64,
	loc:        runtime.Source_Code_Location, // where gk.view made it (for messages)
}

@(private)
views: map[u64]^View_Rec

// view makes a view over `state`. `init`, if not nil, runs once when a window builds this
// view as its root (kit state that needs a window is created there).
view :: proc(
	state: ^$S,
	init: proc(s: ^S, cx: ^Cx),
	render: proc(s: ^S, cx: ^Cx) -> Element,
	loc := #caller_location,
) -> View {
	if cur == nil {
		stop(loc, "gk.view was called before the app started: make views in the procedure given to gk.run, in init or in a handler")
	}
	if build.active {
		// A view made by a view procedure is a new view on every build, kept until the app ends.
		stop(loc, "a view procedure may not create views (it runs on every redraw): create them in init or in a handler and keep the gk.View in your state struct")
	}
	init_tramp :: proc(fn, state: rawptr, cx: ^Cx) {
		(cast(proc(s: ^S, cx: ^Cx))fn)(cast(^S)state, cx)
	}
	view_tramp :: proc(fn, state: rawptr, cx: ^Cx) -> Element {
		return (cast(proc(s: ^S, cx: ^Cx) -> Element)fn)(cast(^S)state, cx)
	}
	check_cx(&cur.cx, loc)
	flush(cur, loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_VIEW_NEW)
	r := Reader{must_call(req[:], loc), 0}
	rec := new(View_Rec, runtime.heap_allocator())
	rec.h = rd_u64(&r)
	rec.state = state
	rec.state_type = typeid_of(S)
	rec.init_fn = rawptr(init)
	rec.view_fn = rawptr(render)
	rec.init_call = init_tramp
	rec.view_call = view_tramp
	rec.loc = loc
	rec.last = make([dynamic]u8, 0, 0, runtime.heap_allocator())
	rec.cur = make([dynamic]Handler, 0, 0, runtime.heap_allocator())
	if views == nil {
		views = make(map[u64]^View_Rec, 64, runtime.heap_allocator())
	}
	views[rec.h] = rec
	return View{rec.h}
}

// notify marks a view dirty: it is described again before the next frame. An unchanged
// description costs no redraw.
notify :: proc(cx: ^Cx, v: View, loc := #caller_location) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.notify in a view procedure: a view procedure only describes. Call it in the handler that changed the data (or call gk.refresh there)")
	}
	mark_dirty(t, v.h)
}

// release_view frees a view: its handle goes stale in the library and its handler data is
// freed. A parent that still embeds it draws nothing in its place.
@(private)
release_view :: proc(t: ^Turn, v: View, loc: runtime.Source_Code_Location) {
	rec, ok := views[v.h]
	if !ok {
		stop(loc, "this gk.View was released already: release a view once")
	}
	delete_key(&views, v.h)
	drop_kept_of(t, v.h, loc)
	free_handlers(rec.cur[:])
	free_old_tables(rec)
	delete(rec.cur)
	delete(rec.last)
	rec.cur, rec.last = nil, nil
	rec.dead = true
	release_handle(t, v.h, loc)
}

// retire_table forgets the handler table of a view's description the library has dropped (no
// element, open menu or closure upstream keeps holds it): none of its handlers can fire again, and
// their data is freed.
@(private)
retire_table :: proc(view: u64, gen: u32) {
	v, ok := views[view]
	if !ok {
		return
	}
	if gen == v.gen {
		v.cur_retired = true
		return
	}
	if t, has := v.old[gen]; has {
		free_handlers(t[:])
		delete(t)
		delete_key(&v.old, gen)
	}
}

// free_old_tables frees every older handler table of a view that goes.
@(private)
free_old_tables :: proc(v: ^View_Rec) {
	for _, t in v.old {
		free_handlers(t[:])
		delete(t)
	}
	delete(v.old)
	v.old = nil
}

@(private)
describe :: proc(cx: ^Cx, v: ^View_Rec) {
	t := cx.turn
	alloc := context.temp_allocator
	build_serial += 1
	v.build_no += 1
	build = Build {
		ops      = make(Tape, 4, 1024, alloc), // offset 0 means "no op"
		nodes    = make([dynamic]Node_Rec, 0, 64, alloc),
		locs     = make([dynamic]runtime.Source_Code_Location, 0, 0, alloc),
		handlers = make([dynamic]Handler, 0, 8, alloc),
		deps     = make([dynamic]u64, 0, 4, alloc),
		slots    = make([dynamic]u64, 0, 8, alloc),
		view     = v,
		serial   = build_serial,
		active   = true,
	}
	root: Element
	if v.row_call != nil {
		root = v.row_call(v.view_fn, v.state, cx, v.row_at)
	} else {
		root = v.view_call(v.view_fn, v.state, cx)
	}
	tape := finish(root)
	v.described = true
	if v.gen != 0 && bytes.equal(tape[:], v.last[:]) {
		// An identical tape is dropped: nothing is notified and the generation stays. The
		// handler table is still replaced, in place: an identical tape has the same handler
		// ids, but the data behind them may be new.
		free_handlers(v.cur[:])
		clear(&v.cur)
		append(&v.cur, ..build.handlers[:])
		return
	}
	resize(&v.last, len(tape))
	copy(v.last[:], tape[:])
	if v.gen != 0 && !v.cur_retired {
		if v.old == nil {
			v.old = make(map[u32][dynamic]Handler, 4, runtime.heap_allocator())
		}
		v.old[v.gen] = v.cur
		v.cur = make([dynamic]Handler, 0, len(build.handlers), runtime.heap_allocator())
	} else {
		free_handlers(v.cur[:])
		clear(&v.cur)
	}
	v.cur_retired = false
	v.gen += 1
	append(&v.cur, ..build.handlers[:])
	put_u64(&t.tapes, v.h)
	put_u32(&t.tapes, v.gen)
	put_u32(&t.tapes, u32(len(tape)))
	append(&t.tapes, ..tape[:])
	t.n_tapes += 1
}
