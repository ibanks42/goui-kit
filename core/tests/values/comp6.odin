// The component protocol tests (docs/PROTOCOLS.md 11.18 to 11.26), in the values
// test app (main.odin calls comp6_click, comp6_start and comp6_view):
//
//   c6-background   try_parse_background read back as gk.Background_Value (11.23)
//   c6-matcher      a calendar's disabled matcher of every variant, written and read back (11.21)
//   c6-q-rules      a questionnaire item's validator as rules, checked by the library (11.26)
//   c6-resize       a resizable group whose handles the app draws: the snapshot of the kit's context
//                   (11.18) read by the row, described again when the pointer moves over the handle
//   c6-chains       colours and value nodes in a constructor's builder chain (11.27, the editor
//                   family's rule of 9.31): a colour picker's default value, a grid's default areas,
//                   a tree's items, an ordinal scale's colours and its colour for unknown values
package values_test

import "core:fmt"
import gk "gpuikit:gk"

C6 :: struct {
	handles:  gk.Row_Source, // c6-resize: one row per handle the group draws
	resizer:  gk.Resizable_State,
}

c6: C6

// The root view's init procedure (a row source needs the window, which a view's init has).
comp6_init :: proc(a: ^App, cx: ^gk.Cx) {
	if a.mode == "c6-resize" {
		c6.handles = gk.row_source(cx, a, 0, handle_row)
		c6.resizer = gk.resizable_state(cx)
	}
}

// The handle's visible part, drawn from the snapshot of the context the kit hands the renderer.
handle_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	ctx := gk.row_context(cx, c6.handles, at, gk.Resize_Handle_Context)
	edge, has_edge := ctx.edge.?
	fmt.printf("values c6 handle row=%d axis=%v edge=%v has_edge=%v active=%v state=%v\n", at.row, ctx.axis, edge,
		has_edge, ctx.is_active, ctx.state)
	color := gk.theme(.Primary) if ctx.is_active || ctx.state == .Hovered else gk.theme(.Border)
	return gk.div(.H_Full)->w(gk.px(1))->bg(color)
}

comp6_view :: proc(a: ^App, cx: ^gk.Cx) -> (gk.Element, bool) {
	if a.mode != "c6-resize" || c6.handles.h == 0 {
		return {}, false
	}
	left := gk.resizable_panel()->size(200)->child(gk.text("left"))
	right := gk.resizable_panel()->child(gk.text("right"))
	group := gk.h_resizable(gk.Id("g"))->with_state(c6.resizer)->with_handle_appearance(c6.handles)->child(left)->child(right)
	return gk.div(.Size_Full)->child(group), true
}

comp6_click :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "c6-background":
		if a.clicks == 1 {background(a, cx)}
	case "c6-matcher":
		if a.clicks == 1 {matcher(a, cx)}
	case "c6-q-rules":
		if a.clicks == 1 {q_rules(a, cx)}
	case "c6-chains":
		if a.clicks == 1 {chains(a, cx)}
	}
}

background :: proc(a: ^App, cx: ^gk.Cx) {
	solid := gk.try_parse_background(cx, "#ff0000")
	s, sok := solid.value.?
	fmt.printf("values c6 background solid ok=%v kind=%v l=%.2f a=%.2f\n", solid.ok, s.kind, s.solid.l, s.solid.a)
	if !sok {fmt.println("values c6 background solid has no value")}
	grad := gk.try_parse_background(cx, "linear-gradient(90deg, #ff0000 0%, #0000ff 100%)")
	g, _ := grad.value.?
	fmt.printf("values c6 background gradient ok=%v kind=%v angle=%.0f from_h=%.2f to_h=%.3f to_pct=%.2f\n", grad.ok, g.kind,
		g.angle_or_height, g.stops[0].color.h, g.stops[1].color.h, g.stops[1].percentage)
	bad := gk.try_parse_background(cx, "not a background")
	_, has := bad.value.?
	fmt.printf("values c6 background bad ok=%v has=%v err=%v\n", bad.ok, has, len(bad.err) > 0)
}

matcher :: proc(a: ^App, cx: ^gk.Cx) {
	d := gk.Naive_Date{2026, 11, 12}
	cal := gk.calendar_state(cx)->disabled_matcher(gk.Matcher_Interval{gk.Interval_Matcher{before = d, after = nil}})->build()
	print_matcher("interval", cal->disabled_matcher_ref(cx))
	cal->set_disabled_matcher(cx, gk.Matcher_Range{gk.Range_Matcher{from = gk.Naive_Date{2026, 1, 1}, to = gk.Naive_Date{2026, 1, 31}}})
	print_matcher("range", cal->disabled_matcher_ref(cx))
	days := []gk.Naive_Date{{2026, 12, 24}, {2026, 12, 25}}
	cal->set_disabled_matcher(cx, gk.Matcher_Custom{days})
	print_matcher("custom", cal->disabled_matcher_ref(cx))
	cal->set_disabled_matcher(cx, []u32{0, 6})
	print_matcher("weekdays", cal->disabled_matcher_ref(cx))
}

print_matcher :: proc(what: string, m: Maybe(gk.Matcher)) {
	v, ok := m.?
	if !ok {
		fmt.printf("values c6 matcher %s none\n", what)
		return
	}
	switch x in v {
	case gk.Matcher_Interval:
		b, hb := x.v0.before.?
		_, ha := x.v0.after.?
		fmt.printf("values c6 matcher %s interval before=%d-%d-%d has_before=%v has_after=%v\n", what, b.year, b.month, b.day, hb, ha)
	case gk.Matcher_Range:
		f, _ := x.v0.from.?
		t, _ := x.v0.to.?
		fmt.printf("values c6 matcher %s range from=%d-%d-%d to=%d-%d-%d\n", what, f.year, f.month, f.day, t.year, t.month, t.day)
	case gk.Matcher_Custom:
		fmt.printf("values c6 matcher %s custom n=%d first=%d-%d-%d\n", what, len(x.v0), x.v0[0].year if len(x.v0) > 0 else 0,
			x.v0[0].month if len(x.v0) > 0 else 0, x.v0[0].day if len(x.v0) > 0 else 0)
	case gk.Matcher_Day_Of_Week:
		fmt.printf("values c6 matcher %s days n=%d\n", what, len(x.v0))
	}
}

q_rules :: proc(a: ^App, cx: ^gk.Cx) {
	color := gk.questionnaire_item_definition("color", "Favourite colour")->with_multiple(true)->
		with_choice(gk.questionnaire_choice_definition("red", "Red"))->with_choice(gk.questionnaire_choice_definition("blue", "Blue"))->
		with_validator(gk.Questionnaire_Rules{max_choices = 1, message = "{item}: one only"})
	plain := gk.questionnaire_item_definition("plain", "No rules")->
		with_choice(gk.questionnaire_choice_definition("x", "X"))
	q := gk.questionnaire_state(cx, {color, plain})->build()
	_ = q->set_answer(cx, "color", gk.questionnaire_answer()->with_choices({"red", "blue"}))
	_ = q->go_next(cx)
	print_error(q, cx, "two")
	_ = q->set_answer(cx, "color", gk.questionnaire_answer()->with_choices({"blue"}))
	_ = q->go_next(cx)
	print_error(q, cx, "one")
	has, phas := false, false
	if def, dok := q->item_definition(cx, "color").?; dok {
		_, has = def->validator(cx).?
	}
	if pdef, pok := q->item_definition(cx, "plain").?; pok {
		_, phas = pdef->validator(cx).?
	}
	fmt.printf("values c6 rules validator color=%v plain=%v\n", has, phas)
}

print_error :: proc(q: gk.Questionnaire_State, cx: ^gk.Cx, what: string) {
	e, ok := q->error(cx, "color").?
	if !ok {
		fmt.printf("values c6 rules %s valid\n", what)
		return
	}
	#partial switch x in e {
	case gk.Questionnaire_Validation_Error_Message:
		fmt.printf("values c6 rules %s message=%q\n", what, x.v0)
	case:
		fmt.printf("values c6 rules %s other=%v\n", what, e)
	}
}

chains :: proc(a: ^App, cx: ^gk.Cx) {
	picker := gk.color_picker_state(cx)->default_value(gk.hsla(0.5, 1, 0.5, 1))->build()
	v, has := picker->value(cx).?
	fmt.printf("values c6 chains picker has=%v h=%.2f\n", has, v.h)
	grid := gk.grid_layout_state(cx)->default_areas({gk.grid_area("a"), gk.grid_area("b")})->build()
	fmt.printf("values c6 chains grid areas=%d\n", len(grid->areas(cx)))
	tree := gk.tree_state(cx)->items({gk.tree_item("r", "root")->expanded(true)->child(gk.tree_item("c", "child"))})->build()
	_, e0 := tree->entry(cx, 0).?
	_, e1 := tree->entry(cx, 1).?
	_, e2 := tree->entry(cx, 2).?
	fmt.printf("values c6 chains tree entries=%v,%v,%v\n", e0, e1, e2)
	scale := gk.scale_ordinal(cx, {"a", "b"}, {gk.hsla(0, 1, 0.5, 1), gk.hsla(0.5, 1, 0.5, 1)})->
		range({gk.hsla(0.25, 1, 0.5, 1), gk.hsla(0.75, 1, 0.5, 1)})->unknown(gk.hsla(0, 0, 0.5, 1))->build()
	b, _ := scale->map_(cx, "b").?
	z, hz := scale->map_(cx, "zz").?
	fmt.printf("values c6 chains scale b_h=%.2f unknown=%v unknown_l=%.2f unknown_s=%.2f\n", b.h, hz, z.l, z.s)
}
