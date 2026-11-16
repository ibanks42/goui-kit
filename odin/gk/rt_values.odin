package gk

// The values family (docs/PROTOCOLS.md sections 13.8 to 13.12): results read
// back, kit action values as actions, library objects placed in a view's elements.
//
// What generated code uses from here: Fallible (the Odin type of Result<T, E>), put_obj (an
// object argument of an element). What the app uses: gk.action_of, gk.Fallible.

import "base:intrinsics"

// Fallible is a Result<T, E> of the kit read back: ok, the value when ok, the error's text when
// not (core/src/values.rs Fallible). Result<(), E> reads as a Maybe(Temp_String), the error
// (PROTOCOLS.md 11.6).
Fallible :: struct($T: typeid) {
	ok:    bool,
	value: Maybe(T),
	err:   Temp_String,
}

// action_of names one of the kit's own action values (gk.base_move_down(), gk.close_panel())
// as an Action, which menus, key bindings and Kbd take (PROTOCOLS.md 13.11). Make the value in
// the handler or init that calls; the same action value always gives the same Action.
action_of :: proc(cx: ^Cx, v: $T, loc := #caller_location) -> Action where intrinsics.type_has_field(T, "nd") {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.action_of in a view procedure: name the action in init or a handler and keep the gk.Action")
	}
	flush(t, loc)
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_VALUE_ACTION)
	put_vtape(&req, v.nd)
	r := Reader{must_call(req[:], loc), 0}
	return Action{rd_u32(&r)}
}

// put_obj writes a library object's handle as an element's argument. A temporary handle (a
// read's result, section 13.2) placed in a view's elements is pinned to that view: it lives as
// long as a build of the view names it, so a later redraw of the same tape finds it, and it is
// released at the first frame after a newer build of the view stops naming it.
@(private)
put_obj :: proc(o: ^Tape, h: u64) {
	put_u64(o, h)
	if build.active && build.view != nil && h != 0 {
		req := make(Tape, 0, 24, context.temp_allocator)
		put_u16(&req, C_OBJ_PIN)
		put_u64(&req, build.view.h)
		put_u32(&req, build.view.build_no)
		put_u64(&req, h)
		call(req[:]) // a stale handle is reported where the element is drawn (obj_shown)
	}
}

// test_value_pins: handles pinned by views, and handles waiting for the next sweep (tests).
test_value_pins :: proc(cx: ^Cx, loc := #caller_location) -> (pinned, later: int) {
	t := check_cx(cx, loc)
	flush(t, loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_VALUES_TEST)
	r := Reader{must_call(req[:], loc), 0}
	return int(rd_u32(&r)), int(rd_u32(&r))
}

// chrono's calendar values (docs/PROTOCOLS.md 13.13), plain records both ways: a date picker's
// and a calendar's dates, a time field's time, the first day of a week. A date or a time that
// does not exist (February 30, 25:00) stops where it is written.
Naive_Date :: struct {
	year:  i32,
	month: u32, // 1 to 12
	day:   u32, // 1 to the month's last day
}

Naive_Time :: struct {
	hour, minute, second: u32,
	nanosecond:           u32, // up to 1_999_999_999 (a leap second, as chrono allows)
}

Naive_Date_Time :: struct {
	date: Naive_Date,
	time: Naive_Time,
}

Weekday :: enum u8 {
	Mon,
	Tue,
	Wed,
	Thu,
	Fri,
	Sat,
	Sun,
}

// days_in_month: 28 to 31 for a month 1 to 12 (the proleptic Gregorian calendar, as chrono's), 0
// for another month.
days_in_month :: proc(year: i32, month: u32) -> u32 {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		leap := year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
		return 29 if leap else 28
	}
	return 0
}

@(private)
put_naive_date :: proc(o: ^Tape, d: Naive_Date) {
	if d.year < -262_143 || d.year > 262_142 {
		stop({}, "the date %d-%d-%d is outside the years -262143 to 262142 a date can hold", d.year, d.month, d.day)
	}
	if d.day < 1 || d.day > days_in_month(d.year, d.month) {
		stop({}, "the date %d-%02d-%02d does not exist (month 1 to 12, day 1 to the last day of the month)", d.year, d.month, d.day)
	}
	put_u32(o, u32(d.year))
	put_u32(o, d.month)
	put_u32(o, d.day)
}

@(private)
put_naive_time :: proc(o: ^Tape, t: Naive_Time) {
	if t.hour > 23 || t.minute > 59 || t.second > 59 || t.nanosecond > 1_999_999_999 ||
	   (t.nanosecond > 999_999_999 && t.second != 59) {
		stop({}, "the time %02d:%02d:%02d.%09d does not exist (hour 0 to 23, minute and second 0 to 59)", t.hour, t.minute, t.second, t.nanosecond)
	}
	put_u32(o, t.hour)
	put_u32(o, t.minute)
	put_u32(o, t.second)
	put_u32(o, t.nanosecond)
}

@(private)
put_naive_date_time :: proc(o: ^Tape, v: Naive_Date_Time) {
	put_naive_date(o, v.date)
	put_naive_time(o, v.time)
}

@(private)
put_weekday :: proc(o: ^Tape, w: Weekday) {
	if u8(w) > 6 {
		stop({}, "gk.Weekday(%d) is not a day of the week", u8(w))
	}
	put_u8(o, u8(w))
}

// --- value cells (docs/PROTOCOLS.md 13.19) ----------------------------------------------------

// Cell is a value the library holds for the kit to read while it draws: a settings field's
// value (`gk.setting_field_bool_switch(dark, gk.on(set_dark))`), a field's dirty test. The page
// reads the cell each time it draws; the app writes it between frames with gk.cell_set (in the
// field's own handler, a timer, init) and every window redraws. T is the value's type as the
// field has it: bool, string, f64. A kept handle: gk.release frees it.
Cell :: struct($T: typeid) {
	h: u64,
}

// cell makes a value cell holding `value` (immediate; not in a view procedure).
cell :: proc(cx: ^Cx, value: $T, loc := #caller_location) -> Cell(T) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.cell in a view procedure: make the cell in init or a handler and keep it")
	}
	flush(t, loc)
	req := make(Tape, 0, 32, context.temp_allocator)
	put_u16(&req, C_CELL_NEW)
	put_cell_value(&req, value)
	r := Reader{must_call(req[:], loc), 0}
	return Cell(T){rd_u64(&r)}
}

// cell_set writes the cell (buffered, with the turn's writes; every window redraws then).
cell_set :: proc(cx: ^Cx, c: Cell($T), value: T, loc := #caller_location) {
	if c.h == 0 {
		stop(loc, "gk.cell_set on a zero Cell (make it with gk.cell)")
	}
	req := make(Tape, 0, 32, context.temp_allocator)
	put_u16(&req, C_CELL_SET)
	put_u64(&req, c.h)
	put_cell_value(&req, value)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// cell_get reads the cell's value now (after the turn's writes so far; allowed in a view
// procedure). A string is lent for the turn.
cell_get :: proc(cx: ^Cx, c: Cell($T), loc := #caller_location) -> (v: T) {
	t := check_cx(cx, loc)
	flush(t, loc)
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_CELL_GET)
	put_u64(&req, c.h)
	r := Reader{must_call(req[:], loc), 0}
	n := int(rd_u32(&r))
	inner := Reader{r.b[r.i:r.i + n], 0}
	rd_into(&inner, &v)
	return
}

@(private)
put_cell_value :: proc(o: ^Tape, value: $T) {
	body := make(Tape, 0, 16, context.temp_allocator)
	v := value
	put_value(&body, &v, type_info_of(T))
	put_u32(o, u32(len(body)))
	append(o, ..body[:])
}

// --- drag and drop (docs/PROTOCOLS.md 13.20) ------------------------------------------------------

// Drag_Value is what an app drags: gpui's drag methods take it (`div->id("row")->on_drag(v,
// preview_view)`), a drop's handler gets it back (`->on_drop(gk.on(proc(a: ^App, cx: ^gk.Cx,
// d: gk.Drag_Value) {..}))`), and a drag-over style (`->drag_over(gk.style()->bg(..))`) applies
// while one is over the element. `text` read back is lent for the turn.
Drag_Value :: struct {
	id:   u64,
	text: string,
}

// test_entity_handles: one entity read back through the library's two paths (core/tests/values).
test_entity_handles :: proc(cx: ^Cx, loc := #caller_location) -> (values_path, dock_path, again, handles: u64) {
	t := check_cx(cx, loc)
	flush(t, loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_HANDLES_TEST)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u64(&r), rd_u64(&r), rd_u64(&r), rd_u64(&r)
}

// --- the components family, docs/PROTOCOLS.md 11.21 to 11.23 --------------------------

// gpui's Background read back (try_parse_background, PROTOCOLS.md 11.23): its kind, colour space,
// solid colour (solid, slash pattern, checkerboard), the gradient's angle in degrees or the
// pattern's height, and a linear gradient's two stops. Written by the library's Wire impl
// (core/src/values.rs), read by reflection.
Background_Kind :: enum u8 {
	Solid,
	Linear_Gradient,
	Pattern_Slash,
	Checkerboard,
}
Background_Color_Space :: enum u8 {
	Srgb,
	Oklab,
}
Background_Stop :: struct {
	color:      Hsla,
	percentage: f32, // 0 to 1
}
Background_Value :: struct {
	kind:            Background_Kind,
	color_space:     Background_Color_Space,
	solid:           Hsla,
	angle_or_height: f32,
	stops:           [2]Background_Stop,
}

// A calendar matcher's parts (PROTOCOLS.md 11.21), both ways: Matcher_Interval{v0 = {before, after}}
// disables the days before `before` and after `after`; Matcher_Range{v0 = {from, to}} the days from
// `from` to `to`; nil is no bound. Matcher_Custom{v0 = []Naive_Date} disables exactly those days.
Interval_Matcher :: struct {
	before: Maybe(Naive_Date),
	after:  Maybe(Naive_Date),
}
Range_Matcher :: struct {
	from: Maybe(Naive_Date),
	to:   Maybe(Naive_Date),
}

// A questionnaire item's validator as rules (QuestionnaireItemDefinition::with_validator, PROTOCOLS.md
// 11.26), checked by the library while the questionnaire updates: a limit of 0 is none; a length counts
// the free answer's characters; one_of / not_one_of (empty: any) the free answer must or must not be;
// same_as / differs_from name another item whose answer this one must equal or differ from; message
// replaces the error ("{item}" is the item's name).
Questionnaire_Rules :: struct {
	min_choices:  u32,
	max_choices:  u32,
	min_length:   u32,
	max_length:   u32,
	one_of:       []string,
	not_one_of:   []string,
	same_as:      string,
	differs_from: string,
	message:      string,
}

// QuestionnaireItemDefinition::validator read back: present when the item has one.
Questionnaire_Validator :: struct {}
