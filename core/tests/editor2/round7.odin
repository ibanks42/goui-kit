// editor2: the protocol tests of docs/PROTOCOLS.md 9.38 to 9.40 (round6.odin dispatches the
// modes it does not know here).
//
//   GK_PROTO=custom      a parsed Custom block (the kit's frontmatter plugin claims the document's
//                        YAML: BlockNode::Custom wrapping the parser's MarkdownNode, a held value)
//                        placed among the app's own blocks by gk.block_host, its flow given by
//                        FlowPosition::at over a list holding it; the keyed leaves around it read
//                        back: the block takes room (the leaf after it lies lower than in a host
//                        without it)
//   overlay-fire         the overlay actions the app takes, given in advance, fired by a key: the
//                        completion menu (presented by the app) is open, Enter is the app's, so
//                        the overlay closes and the editor does nothing with it; with the menu
//                        closed the same action routes nowhere
//   overlay-control      the same without the key set: Enter is the editor's (a new line). Both draw the
//                        base Editor: the kit's code editor installs its own overlay handler
//   lsp-ask              the Lsp's completion highlighter (an ask, 9.33) consulted by the open
//                        completion menu: the query and each item's label asked, the ranges the
//                        app answers kept
//   diagnostic-shown     a diagnostic presented on the editor (9.34) and read back as the popover
//                        the editor shows; cleared
package editor2

import "core:fmt"
import gk "gpuikit:gk"

Round7 :: struct {
	ext:      gk.Markdown_Extensions_Arc,
	custom:   gk.Block_Node_Held,
	parsed:   int,
	tops:     map[u64]f32, // leaf key -> top
	bottoms:  map[u64]f32, // leaf key -> bottom
	leaf_events: int,
	presented: bool,
	asked:    int,
}

CUSTOM_MD :: "---\ntitle: Notes\nowner: me\n---\n\nAfter the front matter.\n"

init7 :: proc(a: ^App, cx: ^gk.Cx) {
	r := &a.r7
	switch a.mode {
	case "custom":
		r.ext = gk.markdown_extensions_arc(cx, gk.markdown_extensions_default()->frontmatter()->plugin(gk.frontmatter_plugin()))
		parsed := gk.base_parse_markdown(cx, CUSTOM_MD, r.ext)
		blocks, ok := parsed.value.?
		r.parsed = len(blocks)
		fmt.printf("editor2 custom parsed ok=%v n=%d\n", ok, len(blocks))
		if ok && len(blocks) > 0 {
			r.custom = gk.retain(cx, blocks[0]) // the front matter: BlockNode::Custom
		}
	case "overlay-fire", "overlay-control":
		a.ed = gk.editor_state(cx)->default_value("ab")->build()
		if a.mode == "overlay-fire" {
			gk.set_overlay_action_handler(a.ed, cx, {{.Completion, "input::Enter"}})
		}
		a.r6.edits = gk.on_input_edits_event(a.ed, cx, gk.on(present_on_edit))
	case "lsp-ask":
		a.r6.hl = gk.ask(cx, a, highlight7)
		a.ed = gk.editor_state(cx)->default_value("fi")->build()
		gk.lsp_mut(a.ed, cx, gk.lsp_default()->with_completion_highlighter(a.r6.hl))
		a.r6.edits = gk.on_input_edits_event(a.ed, cx, gk.on(present_on_edit))
	case "diagnostic-shown":
		a.ed = gk.editor_state(cx)->default_value("let x = 1\n")->build()
		gk.present_diagnostic(a.ed, cx, {4, 5}, gk.diagnostic({{0, 4}, {0, 5}}, "unused x")->with_severity(.Warning))
	}
}

// present_on_edit opens the completion menu at the caret once the user typed (the editor is focused
// then, so a key reaches it): two items, the query the text typed so far.
present_on_edit :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Edits_Event) {
	r := &a.r7
	if r.presented || len(e.edits) == 0 {
		return
	}
	r.presented = true
	last := e.edits[len(e.edits) - 1]
	items := []gk.Completion_Item{{label = "first", kind = 3, detail = "fn first()"}, {label = "second", kind = 6}}
	gk.present_completion_items(a.ed, cx, last.v0.new_end_byte, "", items)
	fmt.printf("editor2 %s presented at=%d text=%q\n", a.mode, last.v0.new_end_byte, last.v1)
}

highlight7 :: proc(a: ^App, cx: ^gk.Cx, key: gk.Tuple_Temp_String_Completion_Item) -> []gk.Range {
	a.r7.asked += 1
	fmt.printf("editor2 lsp-ask asked query=%q label=%q\n", key.v0, key.v1.label)
	out := make([]gk.Range, 1, context.temp_allocator)
	out[0] = {0, min(2, len(key.v1.label))}
	return out
}

para7 :: proc(text: string, key: u64) -> gk.Block_Node {
	return gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->push(gk.inline_node(text))->with_leaf_key(key)}
}

view7 :: proc(a: ^App, cx: ^gk.Cx) -> (gk.Element, bool) {
	r := &a.r7
	switch a.mode {
	case "custom":
		rc := gk.render_context_from_app()->with_markdown_extensions(r.ext)
		with := make([]gk.Block_Node, 3, context.temp_allocator)
		with[0] = para7("Before", 1)
		with[1] = r.custom
		with[2] = para7("After", 2)
		flows := make([]Maybe(gk.Flow_Position), len(with), context.temp_allocator)
		for _, i in with {
			flows[i] = gk.flow_position_at(with, i, i == len(with) - 1) // FlowPosition::at over the held block
		}
		without := make([]gk.Block_Node, 2, context.temp_allocator)
		without[0] = para7("Before", 3)
		without[1] = para7("After", 4)
		return gk.h_flex(.Gap_2, .Items_Start)->
			child(gk.div()->w(gk.px(200))->child(gk.block_host("with", with, flows, rc, nil, gk.on(on_leaves7))))->
			child(gk.div()->w(gk.px(200))->child(gk.block_host("without", without, nil, rc, nil, gk.on(on_leaves7)))), true
	case "overlay-fire", "overlay-control":
		// The base Editor: an editor drawn outside the kit's overlays, which is what the key set is for.
		// The kit's code editor (component::input::Editor) installs its own overlay handler each time
		// it renders (its completion menu takes the menu's actions), replacing the app's.
		return gk.div()->w(gk.px(420))->h(gk.px(160))->child(gk.base_editor(a.ed)), true
	case "lsp-ask", "diagnostic-shown":
		// The kit's code editor (component::input::Editor): its overlays draw the completion menu and
		// the diagnostic popover.
		return gk.div()->w(gk.px(420))->child(gk.code_editor(a.ed, height = 160)), true
	}
	return {}, false
}

on_leaves7 :: proc(a: ^App, cx: ^gk.Cx, e: gk.Leaf_Geometry_Event) {
	r := &a.r7
	r.leaf_events += 1
	for l in e.leaves {
		b := gk.bounds(l.geometry, cx)
		r.tops[l.key] = b.origin.y
		r.bottoms[l.key] = b.origin.y + b.size.height
	}
}

step7 :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	r := &a.r7
	switch a.mode {
	case "custom":
		gap_with := r.tops[2] - r.bottoms[1]
		gap_without := r.tops[4] - r.bottoms[3]
		fmt.printf("editor2 custom leaves=%d events=%d placed=%v gap_with=%.0f gap_without=%.0f\n", len(r.tops),
			r.leaf_events, gap_with > gap_without, gap_with, gap_without)
	case "overlay-fire", "overlay-control":
		routed := gk.route_overlay_action(a.ed, cx, gk.action(cx, "input::Enter"))
		fmt.printf("editor2 %s value=%q routed_after=%v taken=%v\n", a.mode, gk.value(a.ed, cx), routed,
			gk.has_overlay_action_handler(a.ed, cx))
	case "lsp-ask":
		fmt.printf("editor2 lsp-ask asked=%d\n", r.asked)
	case "diagnostic-shown":
		switch a.step {
		case 1:
			_, shown := gk.diagnostic_popover(a.ed, cx).?
			fmt.printf("editor2 diagnostic-shown popover=%v\n", shown)
			gk.clear_diagnostic_popover(a.ed, cx)
		case 2:
			_, shown := gk.diagnostic_popover(a.ed, cx).?
			fmt.printf("editor2 diagnostic-shown cleared popover=%v\n", shown)
		}
	case:
		return false
	}
	return true
}

// The scripts of this file's modes, after "size 480 360\nframe\n". The editor's first line is at
// y 72; a click at x 200 puts the caret after its text.
script7 :: proc(mode: string) -> (string, bool) {
	switch mode {
	case "custom":
		return "frame\nframe\nframe\nframe\n" + STEP, true
	case "overlay-fire", "overlay-control":
		return "click 200 72\nframe\ntype x\nframe\nframe\nadvance 100\nframe\nframe\nkey enter\nframe\nframe\n" + STEP, true
	case "lsp-ask":
		return "click 200 72\nframe\ntype x\nframe\nframe\nadvance 100\nframe\nframe\nframe\nframe\n" + STEP, true
	case "diagnostic-shown":
		return "frame\nframe\n" + STEP + STEP, true
	}
	return "", false
}
