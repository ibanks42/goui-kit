package gk

// Background work: any thread may post a procedure to run on the UI thread. The queue is
// the binding's own; only a wake crosses to the library, and only when the queue goes from
// empty to non-empty. However fast a thread posts, the library makes at most one WAKE turn
// per turn of its main loop, and the views the posts mark are described once, in the next
// pre-frame hook.

import "base:runtime"
import "core:sync"

@(private)
Post :: struct {
	call:       proc(fn, state, data: rawptr, cx: ^Cx),
	fn:         rawptr,
	data:       rawptr,
	ti:         ^runtime.Type_Info, // of data
	state_type: typeid,
	loc:        runtime.Source_Code_Location,
}

@(private)
post_mu: sync.Mutex
@(private)
post_queue: [dynamic]Post

// post runs a procedure on the UI thread, in a WAKE turn, with the app's state. It may be
// called from any thread. The value is copied when post is called, under the rules of handler
// data except that slices and dynamic arrays are allowed (a thread's list of results): they are
// copied with their elements, so what the posting thread allocated stays its own. The message
// is then the procedure's: its strings, slices and dynamic arrays are allocated with the app's
// allocator (context.allocator of gk.run); keep them, or delete them when done.
//   gk.post(tick)            tick :: proc(s: ^App, cx: ^gk.Cx)
//   gk.post(on_quote, q)     on_quote :: proc(s: ^App, cx: ^gk.Cx, q: Quote)
post :: proc {
	post_plain,
	post_data,
}

post_plain :: proc(fn: proc(s: ^$S, cx: ^Cx), loc := #caller_location) {
	tramp :: proc(fn, state, data: rawptr, cx: ^Cx) {
		(cast(proc(s: ^S, cx: ^Cx))fn)(cast(^S)state, cx)
	}
	push_post(Post{call = tramp, fn = rawptr(fn), state_type = typeid_of(S), loc = loc})
}

post_data :: proc(fn: proc(s: ^$S, cx: ^Cx, m: $M), msg: M, loc := #caller_location) {
	tramp :: proc(fn, state, data: rawptr, cx: ^Cx) {
		(cast(proc(s: ^S, cx: ^Cx, m: M))fn)(cast(^S)state, cx, (cast(^M)data)^)
	}
	push_post(Post{call = tramp, fn = rawptr(fn), data = own(msg, loc, lists = true), ti = type_info_of(M), state_type = typeid_of(S), loc = loc})
}

@(private)
push_post :: proc(p: Post) {
	// The state type is checked here when the app is running, and when the queue is drained
	// for a post made before gk.run.
	if app_state_type != nil && p.state_type != app_state_type {
		stop(p.loc, "a posted procedure takes ^%v but the app's state is %v: it takes the state given to gk.run, as in proc(a: ^%v, cx: ^gk.Cx, m: Message)", p.state_type, app_state_type, app_state_type)
	}
	sync.mutex_lock(&post_mu)
	if post_queue == nil {
		post_queue = make([dynamic]Post, 0, 64, runtime.heap_allocator())
	}
	first := len(post_queue) == 0
	append(&post_queue, p)
	sync.mutex_unlock(&post_mu)
	if first {
		// Before gk.run has started the library's loop this fails and is ignored: the INIT
		// turn drains the queue.
		gk_post(0)
	}
}

@(private)
posts_queued :: proc() -> int {
	sync.mutex_lock(&post_mu)
	defer sync.mutex_unlock(&post_mu)
	return len(post_queue)
}

// drain_posts runs the queued posts, or the first `limit` of them (the rest stay queued).
@(private)
drain_posts :: proc(cx: ^Cx, limit := -1) {
	// The queue is taken whole (it may hold more than a turn's arena: 300,000 posts are
	// 17 MB of records) and a new one starts.
	if limit == 0 {
		return
	}
	sync.mutex_lock(&post_mu)
	queue := post_queue
	post_queue = make([dynamic]Post, 0, 64, runtime.heap_allocator())
	left := 0
	if limit > 0 && limit < len(queue) {
		append(&post_queue, ..queue[limit:])
		resize(&queue, limit)
		left = len(post_queue)
	}
	sync.mutex_unlock(&post_mu)
	if left > 0 {
		gk_post(0) // the posts left behind may not have asked for a wake (the queue was not empty)
	}
	defer delete(queue)
	for p in queue {
		if p.state_type != app_state_type {
			stop(p.loc, "a posted procedure takes ^%v but the app's state is %v: it takes the state given to gk.run, as in proc(a: ^%v, cx: ^gk.Cx, m: Message)", p.state_type, app_state_type, app_state_type)
		}
		if p.data != nil {
			give(p.data, p.ti) // the strings are the procedure's from here on
		}
		p.call(p.fn, app_state, p.data, cx)
		if p.data != nil {
			disown(p.data, p.ti, true)
		}
	}
}
