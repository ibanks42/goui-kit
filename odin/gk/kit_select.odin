package gk

// The searchable-list family of the delegate protocol (docs/PROTOCOLS.md section 9.9): the kit's
// Select and Combobox, whose states are generic over a delegate the app implements upstream. The
// binding's delegate lives in the library (core/src/shim_select.rs) over a row source; the app
// gives it items as data, and optionally a row procedure that draws each item, header rows for
// groups, and a handler that answers searches. The states and elements are generated
// (gen_state.odin, gen_component_select.odin, gen_component_combobox.odin):
//
//   src := gk.search_source(cx, a, ITEMS[:])                       init or a handler
//   a.fruit = gk.select_state(cx, src, nil)->searchable(true)->build()   nil: nothing chosen; 0: the first item
//   view: gk.select(a.fruit)->placeholder("Fruit")->w(gk.px(200))
//   a.fruit->selected_value(cx)                                     a Maybe(Temp_String): the item's value
//
//   src := gk.search_source_groups(cx, a, GROUPS[:], row = fruit_row, headers = true)
//   a.tags = gk.combobox_state(cx, src, {})->multiple(true)->build()      []int{0, 2}: those items chosen
//   fruit_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element  at: the group and the item's index in it
//
// Searching filters the items in the library as upstream's SearchableVec does (the title or
// the keywords contain the query, ignoring case; a group stays while an item or its title
// matches). With `requests`, the source asks the app instead: the handler gets a Search_Request,
// pushes the items it found (gk.search_items), sets them on the state (->set_items(cx, src))
// and answers (gk.answer(cx, src, e.id, true)); until then the menu shows the items it had.

import "base:runtime"

// Search_Item is one item of a select or combobox (upstream's SearchableListItem as data).
Search_Item :: struct {
	value:    string, // identifies the item: what selected_value and the events report
	title:    string, // shown in the menu and, unless display is set, in the trigger
	display:  Maybe(string), // shown in the trigger instead of the title
	keywords: string, // searched besides the title
	disabled: bool, // shown, but cannot be chosen
}

// Search_Group is a titled group of items (upstream's SearchableGroup).
Search_Group :: struct {
	title: string,
	items: []Search_Item,
}

// Search_Request is the event of a source made with `requests`: the query changed.
Search_Request :: struct {
	id:    u64,
	query: Temp_String,
}

@(init, private)
select_shapes :: proc "contextless" () {
	register_shape(SHAPE_SEARCH_EVENT, Search_Request, nil)
}

// search_item is an item whose title is its value unless a title is given.
search_item :: proc(value: string, title := "", disabled := false, keywords := "") -> Search_Item {
	return {value = value, title = title == "" ? value : title, keywords = keywords, disabled = disabled}
}

@(private = "file")
Search_Rec :: struct {
	flags: u8, // 1 rows draw the items, 2 searches are requests, 4 header rows
}

@(private = "file")
searches: map[u64]Search_Rec

// search_source makes the row source of a select or combobox over a flat list of items. `row`,
// if given, describes the row of item at.row (the kit then draws it instead of the title);
// `requests` makes searches requests to the app. Needs a window: init or a handler.
search_source :: proc(
	cx: ^Cx,
	state: ^$S,
	items: []Search_Item,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element = nil,
	requests: Handler = {},
	loc := #caller_location,
) -> Row_Source {
	one := [1]Search_Group{{items = items}}
	return search_source_of(cx, state, one[:], false, row, false, requests, loc)
}

// search_source_groups is search_source over titled groups. With `headers`, the row procedure
// also describes each group's header row (at.row == gk.HEADER).
search_source_groups :: proc(
	cx: ^Cx,
	state: ^$S,
	groups: []Search_Group,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element = nil,
	headers := false,
	requests: Handler = {},
	loc := #caller_location,
) -> Row_Source {
	return search_source_of(cx, state, groups, true, row, headers, requests, loc)
}

@(private = "file")
search_source_of :: proc(
	cx: ^Cx,
	state: ^$S,
	groups: []Search_Group,
	grouped: bool,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element,
	headers: bool,
	requests: Handler,
	loc: runtime.Source_Code_Location,
) -> Row_Source {
	if headers && row == nil {
		stop(loc, "gk.search_source_groups: header rows need a row procedure")
	}
	placeholder :: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element {return {}}
	first := len(groups) > 0 ? len(groups[0].items) : 0
	src := row_source(cx, state, row != nil ? first : 0, row != nil ? row : placeholder, requests, SHAPE_SEARCH_EVENT,
		initial = row != nil ? 64 : 0, loc = loc)
	flags: u8
	if row != nil {flags |= 1}
	if requests.call != nil {flags |= 2}
	if headers {flags |= 4}
	if searches == nil {
		searches = make(map[u64]Search_Rec, 8, runtime.heap_allocator())
	}
	searches[src.h] = {flags}
	put_search(cx, src, groups, grouped, loc)
	return src
}

// search_items replaces the items of a select's or combobox's source (a buffered write). A state
// made over it before shows them after ->set_items(cx, src).
search_items :: proc(cx: ^Cx, src: Row_Source, items: []Search_Item, loc := #caller_location) {
	one := [1]Search_Group{{items = items}}
	put_search(cx, src, one[:], false, loc)
}

// search_groups is search_items for titled groups.
search_groups :: proc(cx: ^Cx, src: Row_Source, groups: []Search_Group, loc := #caller_location) {
	put_search(cx, src, groups, true, loc)
}

@(private = "file")
put_search :: proc(cx: ^Cx, src: Row_Source, groups: []Search_Group, grouped: bool, loc: runtime.Source_Code_Location) {
	t := check_cx(cx, loc)
	rec, ok := searches[src.h]
	if !ok {
		stop(loc, "gk.search_items was given a row source gk.search_source did not make")
	}
	if rec.flags & 1 != 0 {
		// Rows are keyed by (group, the item's index in it): every group needs its count.
		for g, s in groups {
			set_count(cx, src, len(g.items), s, loc)
		}
	}
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_SEARCH_ITEMS)
	put_u64(&req, src.h)
	put_u8(&req, rec.flags)
	put_u32(&req, u32(len(groups)))
	for g in groups {
		put_u8(&req, grouped ? 1 : 0)
		if grouped {
			put_str(&req, g.title)
		}
		put_u32(&req, u32(len(g.items)))
		for it in g.items {
			put_search_item(&req, it)
		}
	}
	defer_op(t, req[:], loc)
}

@(private = "file")
put_search_item :: proc(b: ^Tape, it: Search_Item) {
	v := it
	put_value(b, &v, type_info_of(Search_Item))
}

// put_view_arg writes a view given to a closure that draws it (the generator's `viewfn` rule,
// `select(..)->empty(view)`): its handle, and the view is described like a CHILD_VIEW's.
@(private)
put_view_arg :: proc(o: ^Tape, v: View) {
	put_u64(o, v.h)
	if v.h == 0 {
		return
	}
	append(&build.slots, v.h)
	if rec, ok := views[v.h]; ok && !rec.described && cur != nil {
		append(&cur.pending, rec)
	}
}

// --- rules for a selection change (docs/PROTOCOLS.md 11.25) -------------------------------------

// Search_Rules is what a select's or combobox's selection may become, given in advance: upstream
// asks its delegate (SearchableListDelegate::on_will_change) while the list handles a click, where
// app code may not run, so the library applies upstream's change and then these rules. `blocked`
// values are never added; `locked` values are never taken out once selected; a change that would
// leave fewer than `min` or more than `max` values selected (0: no limit) is refused whole.
Search_Rules :: struct {
	min:     int,
	max:     int,
	locked:  []string,
	blocked: []string,
}

// search_will_change gives a select's or combobox's source its rules (buffered; replaces the
// source's rules; the zero Search_Rules is upstream's default alone).
search_will_change :: proc(cx: ^Cx, src: Row_Source, rules: Search_Rules, loc := #caller_location) {
	t := check_cx(cx, loc)
	if _, ok := searches[src.h]; !ok {
		stop(loc, "gk.search_will_change was given a row source gk.search_source did not make")
	}
	if rules.min < 0 || rules.max < 0 || (rules.max > 0 && rules.min > rules.max) {
		stop(loc, "gk.search_will_change: min %d and max %d cannot both hold: min must not be greater than max", rules.min, rules.max)
	}
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_SEARCH_RULES)
	put_u64(&req, src.h)
	put_u32(&req, u32(rules.min))
	put_u32(&req, u32(rules.max))
	lists := [2][]string{rules.locked, rules.blocked}
	for list in lists {
		put_u32(&req, u32(len(list)))
		for v in list {
			put_str(&req, v)
		}
	}
	defer_op(t, req[:], loc)
}
