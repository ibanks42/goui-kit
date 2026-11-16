// editor2: the protocol tests of docs/PROTOCOLS.md 9.30 to 9.37 (main.odin dispatches the
// modes it does not know here; those this file does not know go on to round7.odin).
//
//   GK_PROTO=host        the editor host: the app's blocks (a heading, a paragraph of inline nodes,
//                        a list of list items, a code block: recursive variants) and Markdown parsed
//                        by gk.base_parse_markdown (held values, kept by gk.retain) drawn by
//                        gk.block_host with a flow, a render context with Markdown extensions and a
//                        decoration; each keyed leaf's geometry after the frame, read back (bounds,
//                        lengths, a position and the index it maps back to, a caret); delivered
//                        once while nothing changes; a parsed table's rows read back
//   chain                a text view state whose builder chain holds a value node (motion), and the
//                        text view defaults with a style in theirs: decoded with the call's context
//   decorate             a number input's buttons decorated: a style and a row each, the rows
//                        described when the buttons are laid out (no window where upstream calls)
//   lsp                  the editor's Lsp edited in place (lsp_mut) with a completion highlighter
//   diagnostic           a diagnostic presented on the editor (a record holding a value node)
//   overlay-keys         the overlay actions the app takes, given in advance; routed with none open
//   edits                an input's InputEditsEvent: the edits and the text after each, typed in
//   context-menu         an input's context menu read back by its handler: the items' actions as
//                        the app's action ids
package editor2

import "core:fmt"
import "core:time"
import gk "gpuikit:gk"

Round6 :: struct {
	ext:      gk.Markdown_Extensions_Arc,
	held:     [dynamic]gk.Block_Node_Held,
	events:   int,
	plus:     gk.Row_Source,
	minus:    gk.Row_Source,
	described: int,
	hl:       gk.Ask(gk.Tuple_Temp_String_Completion_Item, []gk.Range),
	tvs:      gk.Text_View_State,
	edits:    gk.Subscription,
}

PARSED_MD :: "## Parsed\n\n- one\n- two\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"

init6 :: proc(a: ^App, cx: ^gk.Cx) {
	r := &a.r6
	switch a.mode {
	case "host":
		r.ext = gk.markdown_extensions_arc(cx, gk.markdown_extensions_default())
		parsed := gk.base_parse_markdown(cx, PARSED_MD, r.ext)
		blocks, ok := parsed.value.?
		fmt.printf("editor2 host parsed ok=%v n=%d\n", ok, len(blocks))
		for b in blocks {
			append(&r.held, gk.retain(cx, b)) // temporary until retained (13.2)
		}
	case "chain":
		r.tvs = gk.text_view_state_markdown(cx, "Chain **text**")->
			motion(gk.text_view_motion_default()->with_stream_fade(200 * time.Millisecond))->build()
		d := gk.text_view_defaults(cx)->with_style(gk.base_text_view_style_default()->with_paragraph_gap(6))->
			with_inherit_text_color(true)->build()
		fmt.printf("editor2 chain inherit=%v\n", gk.inherit_text_color(d, cx))
	case "decorate":
		r.plus = gk.row_source(cx, a, 0, plus_row, initial = 1)
		r.minus = gk.row_source(cx, a, 0, minus_row, initial = 1)
		a.inp = gk.input_state(cx)->default_value("4")->build()
	case "lsp":
		r.hl = gk.ask(cx, a, highlight)
		a.ed = gk.editor_state(cx)->default_value("let x = 1\n")->build()
		gk.lsp_mut(a.ed, cx, gk.lsp_default()->with_completion_highlighter(r.hl))
	case "diagnostic":
		a.ed = gk.editor_state(cx)->default_value("let x = 1\n")->build()
		gk.present_diagnostic(a.ed, cx, {0, 3}, gk.diagnostic({{0, 0}, {0, 3}}, "unused")->with_severity(.Warning)->with_code("W1"))
	case "overlay-keys":
		a.ed = gk.editor_state(cx)->default_value("let x = 1\n")->build()
		gk.set_overlay_action_handler(a.ed, cx, {{.Completion, "input::Tab"}, {.Code_Action, "input::Escape"}})
	case "edits":
		a.inp = gk.input_state(cx)->default_value("ab")->build()
		r.edits = gk.on_input_edits_event(a.inp, cx, gk.on(on_edits))
	case "context-menu":
		a.inp = gk.input_state(cx)->default_value("menu text")->build()
		gk.on_context_menu(a.inp, cx, gk.on(on_menu))
	case:
		init7(a, cx) // round7.odin
	}
}

plus_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.r6.described += 1
	return gk.label("+")
}

minus_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.r6.described += 1
	return gk.label("-")
}

highlight :: proc(a: ^App, cx: ^gk.Cx, key: gk.Tuple_Temp_String_Completion_Item) -> []gk.Range {
	out := make([]gk.Range, 1, context.temp_allocator)
	out[0] = {0, len(key.v0)}
	return out
}

on_edits :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Edits_Event) {
	for x in e.edits {
		fmt.printf("editor2 edits start=%d old_end=%d new_end=%d text=%q\n", x.v0.start_byte, x.v0.old_end_byte, x.v0.new_end_byte, x.v1)
	}
}

on_menu :: proc(a: ^App, cx: ^gk.Cx, e: gk.Event_114) {
	copy := gk.action(cx, "input::Copy")
	actions, same := 0, 0
	for it in e.a0.items {
		if x, ok := it.(gk.Native_Menu_Item_Action); ok {
			actions += 1
			if x.action.id == copy.id {
				same += 1
				fmt.printf("editor2 context-menu copy label=%q disabled=%v\n", x.label, x.disabled)
			}
		}
	}
	a.r6.events += 1
	fmt.printf("editor2 context-menu items=%d actions=%d copy=%d at_x=%v\n", len(e.a0.items), actions, same, e.a2.x > 0)
}

// The host's blocks: a heading, a paragraph of two inline nodes (the first marked bold), a list of
// two items (variants holding blocks), a code block, then the parsed ones.
host_blocks :: proc(a: ^App) -> []gk.Block_Node {
	bs := make([dynamic]gk.Block_Node, context.temp_allocator)
	append(&bs, gk.Block_Node_Heading{level = 2, children = gk.paragraph_default()->push(gk.inline_node("Host"))->with_leaf_key(1)})
	append(&bs, gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->
		push(gk.inline_node("Hello ")->marks({{gk.Range{0, 5}, gk.text_mark_default()->bold()}}))->
		push(gk.inline_node("world"))->with_leaf_key(2)})
	items := make([]gk.Block_Node, 2, context.temp_allocator)
	one := make([]gk.Block_Node, 1, context.temp_allocator)
	one[0] = gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->push(gk.inline_node("first"))->with_leaf_key(3)}
	two := make([]gk.Block_Node, 1, context.temp_allocator)
	two[0] = gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->push(gk.inline_node("second"))}
	items[0] = gk.Block_Node_List_Item{children = one}
	items[1] = gk.Block_Node_List_Item{children = two, checked = true}
	append(&bs, gk.Block_Node_List{children = items})
	append(&bs, gk.Block_Node_Code_Block{v0 = gk.base_code_block_from_code("x := 1", "odin")->with_leaf_key(4)})
	for h in a.r6.held {
		append(&bs, h)
	}
	return bs[:]
}

view6 :: proc(a: ^App, cx: ^gk.Cx) -> (gk.Element, bool) {
	r := &a.r6
	switch a.mode {
	case "host":
		blocks := host_blocks(a)
		flows := make([]Maybe(gk.Flow_Position), len(blocks), context.temp_allocator)
		flows[0] = gk.flow_position_at(blocks, 0, false)->first(true)
		rc := gk.render_context_from_app()->with_markdown_extensions(r.ext)->with_body_font_size(14)
		decs := []gk.Leaf_Decoration{{key = 2, ranges = {{gk.Range{0, 5}, gk.theme(.Selection)}}}}
		return gk.div()->w(gk.px(420))->child(gk.block_host("doc", blocks, flows, rc, decs, gk.on(on_leaves))), true
	case "chain":
		return gk.div()->w(gk.px(300))->child(gk.text_view(r.tvs)), true
	case "decorate":
		return gk.div()->w(gk.px(200))->child(gk.base_number_input(a.inp)->
			increment_button(gk.Decorate{style = gk.style_refinement_default()->bg(gk.theme(.Primary)), content = r.plus})->
			decrement_button(gk.Decorate{content = r.minus})), true
	case "lsp", "diagnostic", "overlay-keys":
		return gk.div()->w(gk.px(420))->h(gk.px(120))->child(gk.editor(a.ed)), true
	case "edits", "context-menu":
		return gk.div()->w(gk.px(220))->child(gk.input(a.inp)), true
	}
	return view7(a, cx) // round7.odin
}

on_leaves :: proc(a: ^App, cx: ^gk.Cx, e: gk.Leaf_Geometry_Event) {
	a.r6.events += 1
	for l in e.leaves {
		g := l.geometry
		b := gk.bounds(g, cx)
		fmt.printf("editor2 leaf key=%d len=%d empty=%v sized=%v line=%v caret=%v\n", l.key, gk.len_(g, cx), gk.is_empty(g, cx),
			b.size.width > 0 && b.size.height > 0, gk.line_height(g, cx) > 0, gk.caret_height(g, cx) > 0)
		if l.key == 2 {
			p := gk.position_for_index(g, cx, 3)
			c := gk.caret_for_position(g, cx, p)
			back := gk.position_for_caret(g, cx, c)
			_, atom := gk.atom_at(g, cx, p).?
			fmt.printf("editor2 leaf hit index=%d caret=%d same=%v atom=%v\n", gk.index_for_position(g, cx, p), c.index,
				back == p, atom)
		}
	}
}

step6 :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	r := &a.r6
	switch a.mode {
	case "host":
		hosts, leaves, delivered, _, _ := gk.test_editor_host_counts(cx)
		fmt.printf("editor2 host hosts=%d leaves=%d delivered=%d events=%d\n", hosts, leaves, delivered, r.events)
		row := []gk.Paragraph{gk.paragraph_default()->push(gk.inline_node("cell")), gk.paragraph_default()->push(gk.inline_node("two"))}
		t := gk.base_table_2(cx, {row}, {.Left, .Right})
		rows := gk.base_table_2_rows(t, cx)
		first := gk.children(rows[0][0], cx)
		fmt.printf("editor2 host table rows=%d cells=%d first=%q aligns=%v\n", len(rows), len(rows[0]), gk.inline_node_read_text(first[0], cx), gk.column_aligns(t, cx))
	case "chain":
		fmt.printf("editor2 chain drawn\n")
	case "decorate":
		_, _, _, decorated, _ := gk.test_editor_host_counts(cx)
		fmt.printf("editor2 decorate decorated=%d described=%d\n", decorated, r.described)
	case "lsp":
		_, _, _, _, lent := gk.test_editor_host_counts(cx)
		fmt.printf("editor2 lsp lent=%d\n", lent)
	case "diagnostic":
		fmt.printf("editor2 diagnostic presented\n")
	case "overlay-keys":
		fmt.printf("editor2 overlay-keys routed=%v\n", gk.route_overlay_action(a.ed, cx, gk.action(cx, "input::Tab")))
	case "edits":
		fmt.printf("editor2 %s stepped\n", a.mode)
	case "context-menu":
		caps := gk.context_menu_capabilities(a.inp, cx)
		fmt.printf("editor2 context-menu events=%d caps=%v\n", r.events, caps.h != 0)
	case:
		return step7(a, cx) // round7.odin
	}
	return true
}

// The scripts of this file's modes, after "size 480 360\nframe\n".
script6 :: proc(mode: string) -> (string, bool) {
	switch mode {
	case "host":
		return "frame\nframe\nframe\nframe\n" + STEP, true
	case "chain", "lsp", "diagnostic", "overlay-keys":
		return "frame\n" + STEP, true
	case "decorate":
		return "frame\nframe\nframe\n" + STEP, true
	case "edits":
		return "click 100 72\nkey end\ntype c\nframe\nkey backspace\nframe\n" + STEP, true
	case "context-menu":
		return "click 100 72\nframe\nrclick 60 72\nframe\nadvance 50\nframe\nframe\n" + STEP, true
	}
	return script7(mode) // round7.odin
}
