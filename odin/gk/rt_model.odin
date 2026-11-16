package gk

// App state lives in Odin and the library never sees it. A Model is the bookkeeping a Rust
// app gets from an Entity: it knows which views read it, and changing it marks exactly those
// views for redraw. It is optional: a handler already marks the view that registered it.
// No crossing is involved; the marks travel in the turn's reply.
//
//   App :: struct { count: gk.Model(int) }
//   gk.get(&a.count)             in a view procedure: records that this view shows it
//   gk.set(cx, &a.count, 3)      in a handler, a post, a frame handler: marks those views
//   gk.update(cx, &a.count, proc(v: ^int) { v^ += 1 })
//   gk.observe(cx, &a.count, a, proc(a: ^App, cx: ^gk.Cx, v: int) {..})   runs on every change

import "base:runtime"

@(private)
Model_Reader :: struct {
	view:     ^View_Rec,
	build_no: u32,
}

@(private)
Model_Observer :: struct {
	call:  proc(o: ^Model_Observer, cx: ^Cx, value: rawptr),
	fn:    rawptr,
	state: rawptr,
}

@(private)
Model_Base :: struct {
	readers:   [dynamic]Model_Reader,
	observers: [dynamic]Model_Observer,
	changing:  bool,
}

Model :: struct($T: typeid) {
	using base: Model_Base,
	value:      T,
}

// get reads the value. In a view procedure it also records that the view shows it.
get :: proc(m: ^Model($T)) -> T {
	model_read(&m.base)
	return m.value
}

// peek reads the value without recording anything.
peek :: proc(m: ^Model($T)) -> T {
	return m.value
}

// set stores a value and marks the views that read the model in their latest build.
set :: proc(cx: ^Cx, m: ^Model($T), value: T, loc := #caller_location) {
	t := model_write(cx, loc)
	m.value = value
	model_changed(t, &m.base, &m.value)
}

// update changes the value in place and marks the views that read the model.
update :: proc(cx: ^Cx, m: ^Model($T), change: proc(v: ^T), loc := #caller_location) {
	t := model_write(cx, loc)
	change(&m.value)
	model_changed(t, &m.base, &m.value)
}

// touch marks the model's readers after its value was changed through a pointer.
touch :: proc(cx: ^Cx, m: ^Model($T), loc := #caller_location) {
	model_changed(model_write(cx, loc), &m.base, &m.value)
}

// observe runs `fn` with `state` after every change of the model (GPUI's cx.observe).
observe :: proc(cx: ^Cx, m: ^Model($T), state: ^$S, fn: proc(s: ^S, cx: ^Cx, value: T), loc := #caller_location) {
	check_cx(cx, loc)
	tramp :: proc(o: ^Model_Observer, cx: ^Cx, value: rawptr) {
		(cast(proc(s: ^S, cx: ^Cx, value: T))o.fn)(cast(^S)o.state, cx, (cast(^T)value)^)
	}
	if m.observers == nil {
		m.observers = make([dynamic]Model_Observer, 0, 2, runtime.heap_allocator())
	}
	append(&m.observers, Model_Observer{tramp, rawptr(fn), state})
}

@(private)
model_read :: proc(m: ^Model_Base) {
	if !build.active {
		return
	}
	v := build.view
	if m.readers == nil {
		m.readers = make([dynamic]Model_Reader, 0, 4, runtime.heap_allocator())
	}
	for &r in m.readers {
		if r.view == v {
			r.build_no = v.build_no
			return
		}
	}
	append(&m.readers, Model_Reader{v, v.build_no})
}

@(private)
model_write :: proc(cx: ^Cx, loc: runtime.Source_Code_Location) -> ^Turn {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not change app state: it only describes. Change state in a handler, in init or in a frame handler")
	}
	return t
}

@(private)
model_changed :: proc(t: ^Turn, m: ^Model_Base, value: rawptr) {
	// Readers whose view has been described again without reading the model are dropped.
	for i := 0; i < len(m.readers); {
		r := m.readers[i]
		if r.view.dead || r.view.build_no != r.build_no {
			unordered_remove(&m.readers, i)
			continue
		}
		mark_dirty(t, r.view.h)
		i += 1
	}
	if m.changing {
		return // an observer changed the model it observes: its readers are marked, no recursion
	}
	m.changing = true
	for i := 0; i < len(m.observers); i += 1 {
		o := m.observers[i]
		o.call(&o, &t.cx, value)
	}
	m.changing = false
}
