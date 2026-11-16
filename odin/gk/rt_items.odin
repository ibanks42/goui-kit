package gk

// Item procedures (docs/PROTOCOLS.md 9.41). Some components draw one part per item through a
// closure upstream calls while it draws: a calendar's days, a time field's segments, a tree's
// entries, a nav stack's pages, a popover's content, a notification's action button. The app
// gives a procedure that describes one item from the values the kit hands it, typed:
//
//   day :: proc(a: ^App, cx: ^gk.Cx, item: gk.Calendar_Item, st: gk.Calendar_Item_State) -> gk.Element {
//       return item->rounded_full()->bg(gk.theme(.Primary) if st.is_active else gk.theme(.Background))
//   }
//   gk.base_calendar(a.cal)->item(gk.items(cx, a, day))          in a view procedure
//
// The procedure runs between frames, like a row procedure: each item drawn in a frame is a row,
// described when it is first drawn and again when the kit hands it other values (that frame still
// shows the row as it was; a new row shows upstream's own drawing for one frame). An element the
// kit built for the item (gk.Calendar_Item above) is the procedure's to style, fill or place, once.
//
// gk.items is a procedure group (gen_items.odin): its members differ by what the procedure takes,
// so a procedure that fits no closure of the kit does not compile, and one given to a closure of
// another kind stops the app where it is given.

import "base:runtime"

@(private)
Items_Key :: struct {
	fn, state: rawptr,
}

@(private)
items_sources: map[Items_Key]Row_Source

// items_source is the row source of one (procedure, state) pair, made on first use: no rows, a
// procedure that reads the item's values (gen_items.odin's trampoline of `kind`).
@(private)
items_source :: proc(
	cx: ^Cx,
	state: ^$S,
	fn: rawptr,
	tramp: proc(fn, state: rawptr, cx: ^Cx, at: Row_At) -> Element,
	kind: int,
	loc: runtime.Source_Code_Location,
) -> Row_Source {
	_ = check_cx(cx, loc)
	key := Items_Key{fn, state}
	if src, ok := items_sources[key]; ok {
		if _, alive := sources[src.h]; alive {
			return src
		}
	}
	none :: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element {
		return {}
	}
	src := new_source(cx, state, 0, none, {}, 0, 0, 512, loc)
	rec := sources[src.h]
	rec.row_fn = fn
	rec.row_call = tramp
	rec.items_kind = kind
	if items_sources == nil {
		items_sources = make(map[Items_Key]Row_Source, 8, runtime.heap_allocator())
	}
	items_sources[key] = src
	return src
}

// items_check: the source a builder of an item closure is given, checked against the kind of
// procedure that closure calls. A source gk.row_source made passes (9.16's gk.row_context reads).
@(private)
items_check :: proc(src: Row_Source, kind: int) -> u64 {
	if rec, ok := sources[src.h]; ok && rec.items_kind != 0 && rec.items_kind != kind {
		stop({}, "gk.items was given a procedure of the kind %s, but this closure calls one of the kind %s",
			ITEMS_KINDS[rec.items_kind], ITEMS_KINDS[kind])
	}
	return src.h
}

// Kit_View is a view the kit hands an item procedure (a tooltip's content): place it as a child,
// once, in the element the procedure returns.
Kit_View :: struct {
	using el: Element,
}

@(private)
kit_view :: proc(index: int, loc := #caller_location) -> Kit_View {
	e := Kit_View{new_node(N_KIT_VIEW, loc)}
	put_u8(&build.ops, u8(index))
	return e
}
