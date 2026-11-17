package gk

// Turns: all app code runs in one callback at a time, on the UI thread, between frames
// (docs/PROTOCOLS.md section 3). A turn collects the handler's return value, dirty marks,
// buffered writes and changed tapes, and they travel back in one reply.

import "base:runtime"
import "core:os"
import "core:strings"
import "core:sync"

@(private)
Turn :: struct {
	kind:     u32,
	parent:   ^Turn,
	serial:   u64,
	window:   u64,
	ret:      Tape, // the handler's return value, in the shape's wire type
	dirty:    [dynamic]u64,
	deferred: Tape, // (u32 len, op bytes)*
	n_defer:  u32,
	tapes:    Tape, // (u64 view, u32 generation, u32 len, tape)*
	n_tapes:  u32,
	pending:  [dynamic]^View_Rec,
	// The view whose init procedure or handler this turn runs: handlers kept during the
	// turn belong to it.
	owner:    ^View_Rec,
	slot:     ^Turn_Slot,
	cx:       Cx,
}

// Cx is the context of one turn. It lives in the turn's arena and is valid until the turn's
// callback returns; a ^Cx kept longer stops the program on use.
Cx :: struct {
	turn:   ^Turn,
	serial: u64,
}

// Temp_String is text lent for the current turn (a getter's result, text in an event). It
// does not assign to a `string`; gk.keep clones it. Passing it to a builder is fine: builders
// copy into the tape.
Temp_String :: distinct string

// keep copies a lent string so it can be stored.
keep :: proc(s: Temp_String, allocator := context.allocator) -> string {
	return strings.clone(string(s), allocator)
}

@(private)
ctx: runtime.Context
@(private)
cur: ^Turn
@(private)
turn_serial: u64
@(private)
ui_thread: int
@(private)
debug: bool
// lib_diag: the library is the diagnostic one (tests), where a binding fault stops loudly.
lib_diag: bool
@(private)
dropped: u64

@(private)
app_state: rawptr
@(private)
app_state_type: typeid
@(private)
app_init: rawptr
@(private)
app_init_call: proc(fn, state: rawptr, cx: ^Cx)

@(private)
runtime_init :: proc() {
	ui_thread = sync.current_thread_id()
	_, debug = os.lookup_env("GK_DEBUG", context.allocator)
	arena_init()
	builtin_shapes()
}

@(private)
check_cx :: proc(cx: ^Cx, loc := #caller_location) -> ^Turn {
	if sync.current_thread_id() != ui_thread {
		stop(loc, "the gk package was called from a thread that is not the UI thread: only gk.post may be called there. Post what the thread computed, as in gk.post(loaded, Loaded{..}); the posted procedure runs on the UI thread with a cx")
	}
	if cx == nil || cur == nil || cx.turn != cur || cx.serial != cur.serial {
		stop(loc, "a ^gk.Cx was used after the procedure it was given to returned: a cx is valid only during that call. Do not store it; use the cx your handler, init or view procedure is given")
	}
	return cur
}

@(private)
mark_dirty :: proc(t: ^Turn, view: u64) {
	for v in t.dirty {
		if v == view {
			return
		}
	}
	append(&t.dirty, view)
}

// defer_op buffers a call op that returns nothing; it travels in the turn's reply. The
// library applies it when the turn's callback has returned (after the outermost entity
// update, for an EVENT turn), before the next draw. A view procedure may not write.
@(private)
defer_op :: proc(t: ^Turn, op: []u8, loc := #caller_location) {
	if build.active {
		stop(loc, "a view procedure may not write: it only describes what the view shows. Change kit state in a handler, in init or in a frame handler")
	}
	if debug {
		must_call(op, loc) // GK_DEBUG: every op at once, so an error points at its own line
		return
	}
	put_u32(&t.deferred, u32(len(op)))
	append(&t.deferred, ..op)
	t.n_defer += 1
}

// flush sends buffered writes now, before a call that returns a value, so order is kept.
@(private)
flush :: proc(t: ^Turn, loc := #caller_location) {
	if t.n_defer == 0 {
		return
	}
	r := Reader{t.deferred[:], 0}
	for _ in 0 ..< t.n_defer {
		n := int(rd_u32(&r))
		must_call(r.b[r.i:r.i + n], loc)
		r.i += n
	}
	clear(&t.deferred)
	t.n_defer = 0
}

@(private)
dispatch :: proc(kind: u32, input: []u8) -> []u8 {
	slot := begin_slot(kind)
	alloc := runtime.Allocator{turn_allocator_proc, slot}
	context.temp_allocator = alloc
	t := new(Turn, alloc)
	t.kind = kind
	t.parent = cur
	t.serial = slot.turn_no
	t.slot = slot
	t.ret = make(Tape, 0, 16, alloc)
	t.dirty = make([dynamic]u64, 0, 8, alloc)
	t.deferred = make(Tape, 0, 64, alloc)
	t.tapes = make(Tape, 0, 64, alloc)
	t.pending = make([dynamic]^View_Rec, 0, 8, alloc)
	t.cx = Cx{t, t.serial}
	cur = t
	cx := &t.cx
	r := Reader{input, 0}
	switch kind {
	case K_INIT:
		// Posts made before the library's loop existed had no wake to ask for: they run here,
		// after the start procedure. Posts made while it runs (a thread a view's init started)
		// asked for a WAKE turn, which runs them with the app's first window open.
		early := posts_queued()
		app_init_call(app_init, app_state, cx)
		drain_posts(cx, early)
	case K_WINDOW_INIT:
		t.window = rd_u64(&r)
		if v, ok := views[rd_u64(&r)]; ok && v.init_fn != nil {
			t.owner = v
			v.init_call(v.init_fn, v.state, cx)
		}
	case K_FRAME:
		t.window = rd_u64(&r)
		if f, ok := frame_handlers[t.window]; ok {
			f.call(f.fn, f.state, cx)
		}
	case K_VIEWS:
		t.window = rd_u64(&r)
		list := make([]u64, int(rd_u32(&r)), context.temp_allocator)
		for &h in list {
			h = rd_u64(&r)
		}
		// The descriptions the library dropped since the last VIEWS turn: their tables go.
		n_retired := int(rd_u32(&r))
		for _ in 0 ..< n_retired {
			view := rd_u64(&r)
			retire_table(view, rd_u32(&r))
		}
		for h in list {
			if v, ok := views[h]; ok {
				describe(cx, v)
			}
		}
		for i := 0; i < len(t.pending); i += 1 {
			if v := t.pending[i]; !v.described {
				describe(cx, v)
			}
		}
	case K_ROWS:
		t.window = rd_u64(&r)
		rows_turn(&r)
	case K_EVENT:
		event(cx, &r)
	case K_WAKE:
		drain_posts(cx)
	case K_DONE:
		done_turn(cx, &r) // a request's answer (rt_platform.odin, PROTOCOLS.md 11.30)
	case:
		stop({}, "the library called the app in a way this package does not know (callback kind %d): the library is newer than the package. Build the app against the odin/ folder of the download its library came from", kind)
	}
	if len(t.ret) == 0 && len(t.dirty) == 0 && t.n_defer == 0 && t.n_tapes == 0 {
		return nil
	}
	out := make(Tape, 0, 32 + len(t.ret) + len(t.dirty) * 8 + len(t.deferred) + len(t.tapes), alloc)
	put_u32(&out, u32(len(t.ret)))
	append(&out, ..t.ret[:])
	put_u32(&out, u32(len(t.dirty)))
	for v in t.dirty {
		put_u64(&out, v)
	}
	put_u32(&out, t.n_defer)
	append(&out, ..t.deferred[:])
	put_u32(&out, t.n_tapes)
	append(&out, ..t.tapes[:])
	return out[:]
}

// end_turn retires the turn's arena. The reply has been copied by gk_reply.
@(private)
end_turn :: proc() {
	t := cur
	slot := t.slot
	cur = t.parent
	retire_slot(slot)
}

@(private)
report :: proc() {
	if debug {
		eprintf_report()
	}
}
