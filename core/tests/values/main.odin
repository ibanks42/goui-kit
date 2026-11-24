// values: the values family's protocol tests (docs/PROTOCOLS.md sections 13.8
// to 13.12): generic calculators instantiated as library objects (the plot scales, the
// motion values), results read back (Result<T, E>, maps), the theme's fields, registry and mode,
// kit action values as actions, temporary handles placed in a view, and the misuse of a value
// given to its own builder. Each mode runs offscreen (the diag library) under a gk-meter script
// and prints lines that core/tests/run-values.sh checks, with the exit code.
//
//   GK_PROTO=val-scales | val-motion | val-theme | val-actions | val-pins | val-self-arg | val-action-in-view |
//            val-clipboard | val-dates | val-bad-date | val-scroll | val-readback | val-handed | val-ambient
//            (the last six: PROTOCOLS.md 13.13 to 13.16) | val-cells | val-one-handle |
//            val-box-shadow | val-drag | val-settings | val-quest-events (13.19 and 13.20)
package values_test

import "core:fmt"
import "core:os"
import "core:time"
import gk "gpuikit:gk"

App :: struct {
	mode:   string,
	root:   gk.View,
	child:  gk.View,
	clicks: int,
	// pins
	diff:   gk.Diff_State,
	ann:    gk.Diff_Annotation,
	builds: int,
	// PROTOCOLS.md 13.13 to 13.16
	scroll: gk.Scroll_Handle,
	quest:  gk.Questionnaire_State,
	picker: gk.Color_Picker_State,
	events: int,
	tip:    gk.View,
	tips:   int,
	// PROTOCOLS.md 13.19, 13.20
	cb:      gk.Cell(bool),
	cs:      gk.Cell(string),
	cf:      gk.Cell(f64),
	dark:    gk.Cell(bool),
	dirty:   gk.Cell(bool),
	preview: gk.View,
	drops:   int,
	shadow:  gk.Box_Shadow,
	answers: int,
	submits: int,
}

// The plot scales over text bands and numbers (PROTOCOLS.md 13.8): made with their builder
// chain in one crossing, read at once; Scale::tick and nearest_index through the trait.
scales :: proc(a: ^App, cx: ^gk.Cx) {
	band := gk.scale_band(cx, {"a", "b", "c"}, {0, 300})->padding_inner(0.1)->build()
	bw, st := gk.band_width(band, cx), gk.step(band, cx)
	ta, _ := gk.tick(band, cx, "a").?
	tc, _ := gk.tick(band, cx, "c").?
	_, missing := gk.tick(band, cx, "zz").?
	fmt.printf("values band width_ok=%v step_ok=%v order=%v missing=%v nearest=%d\n",
		bw > 0 && bw < 100, st >= bw, ta < tc, !missing, gk.nearest_index(band, cx, tc))
	lin := gk.scale_linear(cx, {0, 100}, {0, 200})
	t0, _ := gk.tick(lin, cx, 0).?
	t50, _ := gk.tick(lin, cx, 50).?
	t100, _ := gk.tick(lin, cx, 100).?
	fmt.printf("values linear t0=%.0f t50=%.0f t100=%.0f\n", t0, t50, t100)
	pt := gk.scale_point(cx, {"x", "y", "z"}, {0, 100})
	p0, _ := gk.tick_at(pt, cx, 0).?
	p2, _ := gk.tick_at(pt, cx, 2).?
	_, none := gk.tick_at(pt, cx, 9).?
	fmt.printf("values point first=%.0f last=%.0f out_of_range=%v\n", p0, p2, !none)
	ord := gk.scale_ordinal(cx, {"red", "blue"}, {gk.hsla(0, 1, 0.5, 1), gk.hsla(0.66, 1, 0.5, 1)})->build()
	c, ok := gk.map_(ord, cx, "blue").?
	fmt.printf("values ordinal found=%v hue=%.2f\n", ok, c.h)
	gk.release(cx, band)
	gk.release(cx, lin)
	gk.release(cx, pt)
	gk.release(cx, ord)
}

// Motion values over f32: keyframes (a fallible constructor), a discrete switch, a sequence.
motion :: proc(a: ^App, cx: ^gk.Cx) {
	kf := gk.keyframes_try_new(cx, {{offset = 0, value = 0, easing = gk.Easing_Linear{}}, {offset = 1, value = 10, easing = gk.Easing_Linear{}}})
	k, kok := kf.value.?
	fmt.printf("values keyframes ok=%v len=%d mid=%.1f\n", kf.ok, gk.len_(k, cx) if kok else -1, gk.sample(k, cx, 0.5) if kok else -1)
	bad := gk.keyframes_try_new(cx, {})
	fmt.printf("values keyframes empty ok=%v err=%v\n", bad.ok, len(bad.err) > 0)
	d := gk.discrete(cx, 1, 2)
	fmt.printf("values discrete before=%.0f after=%.0f\n", gk.sample(d, cx, 0.2), gk.sample(d, cx, 0.9))
	sw := gk.switch_at(d, cx, 0.1)
	fmt.printf("values discrete switch ok=%v\n", sw.ok)
	step := gk.sequence_step(cx, 5, gk.transition(10 * time.Millisecond))
	seq := gk.sequence(cx, gk.Id("seq"), 1)->with_steps({step})->build()
	fmt.printf("values sequence from=%.0f steps=%d\n", gk.from(seq, cx), len(gk.steps(seq, cx)))
}

// The theme: the registry's themes (a map, read back sorted), a config applied, the mode
// changed (a static taking Option<&mut Window>), fields read and written, results read back.
theme :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.clicks {
	case 1:
		reg := gk.theme_registry_global(cx)
		themes := gk.themes(reg, cx)
		light := gk.default_light_theme(reg, cx)
		th := gk.theme_global(cx)
		gk.apply_config(th, cx, light)
		fmt.printf("values registry themes=%v dark=%v\n", len(themes) > 0, gk.is_dark(th, cx))
		gk.theme_change(cx, gk.Theme_Mode.Dark)
		gk.set_font_size(th, cx, 17)
		err := gk.load_themes_from_str(reg, cx, "not json")
		_, failed := err.?
		fmt.printf("values registry bad json refused=%v\n", failed)
		ok := gk.try_parse_color(cx, "#ff0000")
		no := gk.try_parse_color(cx, "not a colour")
		col, _ := ok.value.?
		fmt.printf("values parse ok=%v red=%v bad_ok=%v bad_err=%v\n", ok.ok, col.l > 0.4 && col.l < 0.6, no.ok, len(no.err) > 0)
		cfg := gk.theme_config_rc(cx, gk.theme_config_default())
		fmt.printf("values config made=%v\n", cfg.h != 0)
	case 2:
		th := gk.theme_global(cx)
		fmt.printf("values theme dark=%v font_size=%.0f\n", gk.is_dark(th, cx), gk.font_size(th, cx))
		gk.theme_change(cx, gk.Theme_Mode.Light)
	case 3:
		fmt.printf("values theme light again dark=%v\n", gk.is_dark(gk.theme_global(cx), cx))
	}
}

// The kit's own action values as actions: the same value gives the same id, a built-in one.
actions :: proc(a: ^App, cx: ^gk.Cx) {
	x := gk.action_of(cx, gk.cancel_default())
	y := gk.action_of(cx, gk.cancel_default())
	z := gk.action_of(cx, gk.select_down_default())
	fmt.printf("values actions same=%v builtin=%v other=%v\n", x == y, gk.is_builtin(x), x != z)
}

// A temporary handle (a read's result) placed in a view's elements: pinned to that view, so a
// redraw of the same tape frames later still finds it (Theme::change redraws every window
// without describing the view again); a newer build releases the old one at the next sweep.
pins :: proc(a: ^App, cx: ^gk.Cx) {
	pinned, later := gk.test_value_pins(cx)
	fmt.printf("values pins click=%d builds=%d pinned=%d later=%d\n", a.clicks, a.builds, pinned, later)
	switch a.clicks {
	case 1:
		gk.theme_change(cx, gk.Theme_Mode.Dark) // every window drawn again, the child's tape unchanged
	case 2:
		gk.notify(cx, a.child) // the child described again: a new temporary handle, the old one released later
	}
}

child_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.builds += 1
	body := gk.div()->w(gk.px(300))->h(gk.px(200))
	if p, ok := gk.position(a.ann, cx).?; ok {
		body = body->child(gk.diff(a.diff)->gutter_markers({gk.diff_gutter_marker(gk.Id("m"), p)}))
	}
	return body
}

// Misuse: a value made in a handler given as an argument to its own builder (exit 2).
self_arg :: proc(a: ^App, cx: ^gk.Cx) {
	t := gk.tree_item("a", "A")
	fmt.println("values giving a value to its own builder")
	t = t->child(t)
	fmt.println("values not stopped")
}

// The clipboard's value closure: a handler that returns text, run when the copy button is
// clicked (an event, never while the kit draws).
copy_value :: proc(a: ^App, cx: ^gk.Cx) -> string {
	fmt.println("values clipboard value asked")
	return "copied by the app"
}

// jiff's calendar values (PROTOCOLS.md 13.13): a calendar's date set and read back, its today and
// weeks, a date picker's date and date with time, a time field's time, a preset of a range.
dates :: proc(a: ^App, cx: ^gk.Cx) {
	cal := gk.calendar_state(cx)->build()
	gk.set_date(cal, cx, gk.Date_Single{gk.Civil_Date{2024, 2, 29}})
	got := gk.date(cal, cx)
	d, _ := got.(gk.Date_Single).v0.?
	today := gk.today(cal, cx)
	weeks := gk.days(cal, cx)
	fmt.printf("values calendar date=%d-%02d-%02d today_ok=%v weeks=%d week_len=%d\n", d.year, d.month, d.day,
		today.year >= 2024 && today.month >= 1 && today.month <= 12, len(weeks), len(weeks[0]) if len(weeks) > 0 else 0)
	ok := gk.apply_date(cal, cx, gk.Date_Range{gk.Civil_Date{2026, 1, 5}, gk.Civil_Date{2026, 1, 9}})
	r, _ := gk.date(cal, cx).(gk.Date_Range)
	s, _ := r.v0.?
	e, _ := r.v1.?
	fmt.printf("values calendar range applied=%v start=%d-%02d-%02d end=%d\n", ok, s.year, s.month, s.day, e.day)
	dp := gk.date_picker_state(cx)->build()
	gk.set_date_time(dp, cx, gk.Date_Time_Single{gk.Civil_Date_Time{{2026, 11, 11}, {9, 30, 15, 0}}})
	dt, _ := gk.date_time(dp, cx).(gk.Date_Time_Single).v0.?
	fmt.printf("values picker date_time=%d-%02d-%02d %02d:%02d:%02d\n", dt.date.year, dt.date.month, dt.date.day,
		dt.time.hour, dt.time.minute, dt.time.second)
	tf := gk.time_field_state(cx)->build()
	gk.set_time(tf, cx, gk.Civil_Time{23, 59, 0, 0}) // the field keeps minutes by default
	t := gk.time(tf, cx)
	fmt.printf("values time %02d:%02d:%02d\n", t.hour, t.minute, t.second)
	_ = gk.date_range_preset_date_time("Launch", gk.Date_Time_Single{gk.Civil_Date_Time{{2026, 11, 11}, {12, 0, 0, 0}}})
	gk.release(cx, cal)
	gk.release(cx, dp)
	gk.release(cx, tf)
}

// Misuse: a date that does not exist stops where it is written (exit 2).
bad_date :: proc(a: ^App, cx: ^gk.Cx) {
	cal := gk.calendar_state(cx)->build()
	fmt.println("values writing February 30")
	gk.set_date(cal, cx, gk.Date_Single{gk.Civil_Date{2026, 2, 30}})
	fmt.println("values not stopped")
}

// gpui's ScrollHandle as a library object (PROTOCOLS.md 13.14): tracked by a scrolling div, its
// ScrollbarHandle methods (sizes read back, the offset written then read), a scrollbar over it.
scroll :: proc(a: ^App, cx: ^gk.Cx) {
	h := a.scroll
	switch a.clicks {
	case 1:
		c := gk.content_size(h, cx)
		v := gk.viewport_bounds(h, cx)
		fmt.printf("values scroll content_taller=%v viewport_h=%.0f offset=%.0f\n", c.height > v.size.height, v.size.height, gk.offset(h, cx).y)
		gk.set_offset(h, cx, {0, -200})
	case 2:
		fmt.printf("values scroll offset=%.0f\n", gk.offset(h, cx).y)
	}
}

// Kit values read back (PROTOCOLS.md 13.15): a questionnaire's answer, its item definitions and
// the answers object, each a copy behind a temporary handle with the type's reads.
readback :: proc(a: ^App, cx: ^gk.Cx) {
	color := gk.questionnaire_item_definition("color", "Favourite colour")->with_required(true)->
		with_choice(gk.questionnaire_choice_definition("red", "Red"))->with_choice(gk.questionnaire_choice_definition("blue", "Blue"))
	a.quest = gk.questionnaire_state(cx, {color})->build()
	_ = gk.on_questionnaire_event(a.quest, cx, gk.on(quest_event))
	q := a.quest
	err := gk.set_answer(q, cx, "color", gk.questionnaire_answer()->with_choices({"blue"}))
	e, failed := err.?
	ans, ok := gk.questionnaire_state_answer(q, cx, "color").?
	ch := gk.choices(ans, cx) if ok else nil
	fmt.printf("values answer set_failed=%v ok=%v choices=%d first=%s empty=%v err=%s\n", failed, ok, len(ch),
		ch[0] if len(ch) > 0 else "", gk.is_empty(ans, cx) if ok else true, e)
	def, dok := gk.item_definition(q, cx, "color").?
	if dok {
		cs := gk.choices(def, cx)
		fmt.printf("values definition name=%s required=%v choices=%d second=%s\n", gk.name(def, cx), gk.is_required(def, cx),
			len(cs), gk.value(cs[1], cx) if len(cs) > 1 else "")
	}
	all := gk.answers(q, cx)
	got, gok := gk.questionnaire_answers_get(all, cx, "color").?
	fmt.printf("values answers len=%d get=%v first=%s\n", gk.len_(all, cx), gok, gk.choices(got, cx)[0] if gok else "")
}

// The questionnaire's event: an answer change hands out a QuestionnaireAnswerChange object.
quest_event :: proc(a: ^App, cx: ^gk.Cx, ev: gk.Questionnaire_Event) {
	#partial switch e in ev {
	case gk.Questionnaire_Event_Answer_Changed:
		a.events += 1
		ch := gk.choices(gk.questionnaire_answer_change_answer(e.v0, cx), cx)
		fmt.printf("values event answer_changed item=%s choices=%d\n", gk.item(e.v0, cx), len(ch))
	}
}

// Kit state a read hands out (PROTOCOLS.md 13.16): a colour picker's sliders, each a SliderState.
handed :: proc(a: ^App, cx: ^gk.Cx) {
	a.picker = gk.color_picker_state(cx)->build()
	sl := gk.sliders(a.picker, cx)
	hue := gk.hue(sl, cx)
	again := gk.hue(sl, cx)
	v := gk.value(hue, cx)
	fmt.printf("values handed hue_max=%.0f same_handle=%v value_kind=%v\n", gk.max_value(hue, cx), hue.h == again.h, v)
	gk.release(cx, hue)
}

// Ambient reads (PROTOCOLS.md 11.5): the locale, the theme through ActiveTheme.
ambient :: proc(a: ^App, cx: ^gk.Cx) {
	th := gk.kit_theme(cx)
	fmt.printf("values ambient locale=%s theme_font=%v\n", gk.locale(cx), gk.font_size(th, cx) > 0)
}

// A tooltip that is the app's view (PROTOCOLS.md 13.18): the closure hands the view back.
tip_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.tips += 1
	fmt.printf("values tooltip view described n=%d\n", a.tips)
	return gk.div()->child(gk.label("a tooltip from Odin"))
}

// --- PROTOCOLS.md 13.19, 13.20 ------------------------------------------------------

// Value cells: made, read, written between frames (the write lands with the turn's writes), read
// again in the next turn, released.
cells :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.clicks {
	case 1:
		a.cb = gk.cell(cx, true)
		a.cs = gk.cell(cx, "first")
		a.cf = gk.cell(cx, 1.5)
		fmt.printf("values cells made b=%v s=%s f=%.1f\n", gk.cell_get(cx, a.cb), gk.cell_get(cx, a.cs), gk.cell_get(cx, a.cf))
		gk.cell_set(cx, a.cb, false)
		gk.cell_set(cx, a.cs, "second")
		gk.cell_set(cx, a.cf, 2.25)
	case 2:
		fmt.printf("values cells again b=%v s=%s f=%.2f\n", gk.cell_get(cx, a.cb), gk.cell_get(cx, a.cs), gk.cell_get(cx, a.cf))
		gk.release(cx, a.cb)
		gk.release(cx, a.cs)
		gk.release(cx, a.cf)
	}
}

// One table of handed-out entities: an entity read back through the values path and the dock's
// path gets the same handle (PROTOCOLS.md 13.16).
one_handle :: proc(a: ^App, cx: ^gk.Cx) {
	v, d, again, n := gk.test_entity_handles(cx)
	fmt.printf("values one handle same=%v again=%v distinct=%d nonzero=%v\n", v == d, v == again, n, v != 0)
}

// box_shadow read back as a record whose colour is a gk.Color (PROTOCOLS.md 13.20), then drawn.
box_shadow :: proc(a: ^App, cx: ^gk.Cx) {
	a.shadow = gk.base_box_shadow(cx, 1, 2, 3, 4, gk.hsla(0.5, 1, 0.25, 1))
	bs := a.shadow
	fmt.printf("values box shadow theme=%d h=%.2f l=%.2f x=%.0f y=%.0f blur=%.0f spread=%.0f\n", bs.color.theme,
		bs.color.h, bs.color.l, bs.offset.x, bs.offset.y, bs.blur_radius, bs.spread_radius)
}

dropped :: proc(a: ^App, cx: ^gk.Cx, d: gk.Drag_Value) {
	a.drops += 1
	fmt.printf("values drop id=%d text=%s\n", d.id, d.text)
}

preview_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	fmt.println("values drag preview described")
	return gk.div()->w(gk.px(60))->h(gk.px(20))->bg(gk.theme(.Primary))
}

set_dark :: proc(a: ^App, cx: ^gk.Cx, v: bool) {
	fmt.printf("values settings switch set=%v was=%v\n", v, gk.cell_get(cx, a.dark))
	gk.cell_set(cx, a.dark, v)
	gk.cell_set(cx, a.dirty, true)
}

frag_click :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.printf("values settings fragment click dark=%v\n", gk.cell_get(cx, a.dark))
}

reset_dark :: proc(a: ^App, cx: ^gk.Cx) {
	fmt.println("values settings reset")
	gk.cell_set(cx, a.dark, false)
	gk.cell_set(cx, a.dirty, false)
}

settings_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	sw := gk.setting_field_bool_switch(a.dark, gk.on(set_dark))->on_reset(a.dirty, gk.on(reset_dark))
	frag := gk.div()->id("frag")->w(gk.px(200))->h(gk.px(40))->on_click(gk.on(frag_click))->child(gk.text("fragment"))
	group := gk.setting_group()->title("Look")->item(gk.setting_item("Dark", sw))->item(gk.setting_item_render(frag))
	return gk.div(.Size_Full)->child(gk.settings(gk.Id("s"))->sidebar_size_range({100, 300})->
		page(gk.setting_page("General")->group(group)))
}

// The questionnaire driven by simulated input: a choice clicked (AnswerChanged), submitted (Submit).
quest_events_start :: proc(a: ^App, cx: ^gk.Cx) {
	color := gk.questionnaire_item_definition("color", "Favourite colour")->
		with_choice(gk.questionnaire_choice_definition("red", "Red"))->with_choice(gk.questionnaire_choice_definition("blue", "Blue"))
	a.quest = gk.questionnaire_state(cx, {color})->build()
	_ = gk.on_questionnaire_event(a.quest, cx, gk.on(quest_event2))
}

quest_event2 :: proc(a: ^App, cx: ^gk.Cx, ev: gk.Questionnaire_Event) {
	#partial switch e in ev {
	case gk.Questionnaire_Event_Answer_Changed:
		a.answers += 1
		ch := gk.choices(gk.questionnaire_answer_change_answer(e.v0, cx), cx)
		fmt.printf("values quest answer_changed item=%s choice=%s\n", gk.item(e.v0, cx), ch[0] if len(ch) > 0 else "")
	case gk.Questionnaire_Event_Submit:
		a.submits += 1
		fmt.printf("values quest submit\n")
	}
}

quest_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	q := a.quest
	if q.h == 0 {
		return gk.div() // the first description, inside open_window: the state needs the window first
	}
	choices := gk.questionnaire_choices(q, "color")->
		child(gk.questionnaire_choice(q, "color", "red")->w(gk.px(200))->h(gk.px(40))->child(gk.text("Red")))->
		child(gk.questionnaire_choice(q, "color", "blue")->w(gk.px(200))->h(gk.px(40))->child(gk.text("Blue")))
	return gk.div(.Size_Full)->child(gk.questionnaire(q)->child(gk.questionnaire_item(q, "color")->child(choices))->
		child(gk.questionnaire_submit(q)->w(gk.px(200))->h(gk.px(40))->child(gk.text("Submit"))))
}

clicked :: proc(a: ^App, cx: ^gk.Cx) {
	a.clicks += 1
	switch a.mode {
	case "val-clipboard":
		text, ok := gk.clipboard_read(cx)
		fmt.printf("values clipboard click=%d ok=%v text=%q\n", a.clicks, ok, text)
	case "val-scales":
		if a.clicks == 1 {scales(a, cx)}
	case "val-motion":
		if a.clicks == 1 {motion(a, cx)}
	case "val-theme":
		theme(a, cx)
	case "val-actions":
		if a.clicks == 1 {actions(a, cx)}
	case "val-pins":
		pins(a, cx)
	case "val-self-arg":
		if a.clicks == 1 {self_arg(a, cx)}
	case "val-dates":
		if a.clicks == 1 {dates(a, cx)}
	case "val-bad-date":
		if a.clicks == 1 {bad_date(a, cx)}
	case "val-scroll":
		scroll(a, cx)
	case "val-readback":
		if a.clicks == 1 {readback(a, cx)}
	case "val-handed":
		if a.clicks == 1 {handed(a, cx)}
	case "val-ambient":
		if a.clicks == 1 {ambient(a, cx)}
	case "val-cells":
		cells(a, cx)
	case "val-one-handle":
		if a.clicks == 1 {one_handle(a, cx)}
	case "val-box-shadow":
		if a.clicks == 1 {box_shadow(a, cx)}
	case:
		comp6_click(a, cx) // the components family (comp6.odin)
	}
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if el, ok := comp6_view(a, cx); ok {
		return el // the components family (comp6.odin)
	}
	switch a.mode {
	case "val-drag":
		src := gk.div(.Absolute)->left(gk.px(10))->top(gk.px(10))->w(gk.px(100))->h(gk.px(40))->bg(gk.theme(.Muted))->
			id("src")->on_drag(gk.Drag_Value{7, "row 7"}, a.preview)->child(gk.text("drag me"))
		dst := gk.div(.Absolute)->left(gk.px(300))->top(gk.px(10))->w(gk.px(120))->h(gk.px(120))->bg(gk.theme(.Secondary))->
			drag_over(gk.style_refinement_default()->bg(gk.theme(.Accent)))->on_drop(gk.on(dropped))
		return gk.div(.Size_Full, .Relative)->child(src)->child(dst)
	case "val-settings":
		return settings_page(a, cx)
	case "val-quest-events":
		return quest_page(a, cx)
	}
	body := gk.div(.Size_Full)->id("all")->on_click(gk.on(clicked))
	if a.mode == "val-pins" {
		body = body->child_view(a.child)
	}
	if a.mode == "val-clipboard" {
		body = body->child(gk.clipboard("copy")->value_fn(gk.on(copy_value)))
	}
	if a.mode == "val-scroll" {
		list := gk.div(.W_Full, .H_64)->id("list")->overflow_y_scroll()->track_scroll(a.scroll)
		for i in 0 ..< 40 {
			list = list->child(gk.div(.H_10)->child(gk.text(fmt.tprintf("row %d", i))))
		}
		body = gk.div(.Size_Full)->id("all")->on_click(gk.on(clicked))->child(list)->child(gk.scrollbar_vertical(a.scroll))
	}
	if a.mode == "val-tooltip" {
		body = body->child(gk.div()->id("tip")->w(gk.px(120))->h(gk.px(40))->tooltip(a.tip)->child(gk.text("hover me")))
	}
	if a.mode == "val-box-shadow" && a.clicks > 0 {
		body = body->child(gk.div()->w(gk.px(80))->h(gk.px(40))->bg(gk.theme(.Background))->shadow({a.shadow}))
	}
	if a.mode == "val-action-in-view" {
		fmt.println("values naming an action in a view")
		_ = gk.action_of(cx, gk.cancel_default())
	}
	return body
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	if a.mode == "val-pins" {
		a.diff = gk.diff_state(cx, {})->build()
		a.ann = gk.diff_annotation_line(cx, gk.Id("a"), gk.diff_line_position(cx, "a.rs", .Modified, 1))
		a.child = gk.view(a, nil, child_view)
	}
	switch a.mode {
	case "val-scroll":
		a.scroll = gk.scroll_handle(cx)
	case "val-tooltip":
		a.tip = gk.view(a, nil, tip_view)
	case "val-drag":
		a.preview = gk.view(a, nil, preview_view)
	case "val-settings":
		a.dark = gk.cell(cx, false)
		a.dirty = gk.cell(cx, false)
	}
	a.root = gk.view(a, comp6_init, root_view) // comp6_init: the components family (comp6.odin)
	gk.open_window(cx, "values", {640, 480}, a.root)
	if a.mode == "val-quest-events" {
		quest_events_start(a, cx) // its state needs the window
		gk.notify(cx, a.root)
	}
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	script := "size 640 480\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nstat\n"
	if app.mode == "val-tooltip" {
		script = "size 640 480\nframe\nmove 20 20\nframe\nadvance 1500\nframe\nframe\nmove 30 25\nadvance 1500\nframe\nstat\n"
	}
	if app.mode == "val-clipboard" {
		// the copy button first (top left), then the page
		script = "size 640 480\nframe\nclick 10 10\nframe\nframe\nclick 600 400\nframe\nframe\nstat\n"
	}
	if app.mode == "val-settings" {
		// the switch of the Dark item, then the app's fragment below it (the kit's layout at 640 x 480)
		script = "size 640 480\nframe\nclick 284 153\nframe\nframe\nclick 300 192\nframe\nframe\nstat\n"
	}
	if app.mode == "val-quest-events" {
		// the Blue choice, then Submit (the kit's layout at 640 x 480)
		script = "size 640 480\nframe\nframe\nclick 100 74\nframe\nframe\nclick 100 132\nframe\nframe\nstat\n"
	}
	if app.mode == "val-drag" {
		script = "size 640 480\nframe\ndrag 50 30 350 60\nframe\nframe\nstat\n"
	}
	if app.mode == "c6-resize" {
		// the pointer over the divider between the panels (the left one is 200 wide), then away
		script = "size 640 480\nframe\nframe\nmove 200 100\nframe\nframe\nframe\nmove 500 300\nframe\nframe\nframe\nstat\n"
	}
	if s, ok := os.lookup_env("GK_VALUES_SCRIPT", context.allocator); ok {
		script = s // a mode's clicks given by run-values.sh (positions in the kit's layout)
	}
	gk.run_headless(&app, script, start)
	fmt.printf("values done mode=%s clicks=%d builds=%d\n", app.mode, app.clicks, app.builds)
}
