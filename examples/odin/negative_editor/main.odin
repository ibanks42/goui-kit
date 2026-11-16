// negative_editor: mistakes against the editor family's protocols (docs/PROTOCOLS.md 9.20 to 9.40)
// that must stop with a message, never draw wrong content with exit 0. Run offscreen with
// the diag library by gates/misuse.sh, one mode per case:
//
//   GK_NEG=ed-ok             a data plugin's rows, a surface and an owned InputBase: correct (exit 0)
//   GK_NEG=ed-owner          gk.entity_id of a handle that is not an input (a History): must stop
//   GK_NEG=ed-owner-released gk.entity_id of an editor released before the view names it: must stop
//   GK_NEG=ed-plugin-freed   a data plugin whose row source was released, drawing a node: must stop
//
// 9.30 to 9.37:
//   GK_NEG=ed6-ok            an editor host over the app's blocks and parsed ones kept by gk.retain, its
//                            leaves read in the handler, a decorated number input: correct (exit 0)
//   GK_NEG=ed6-held-stale    a parsed block kept by gk.retain, released, then placed in a view: must stop
//   GK_NEG=ed6-leaf-stale    a leaf's geometry kept from its event without gk.retain, read a frame
//                            later: must stop
//   GK_NEG=ed6-decorate-freed a decorator whose content row source was released: must stop
//   GK_NEG=ed6-lent-view     an edit of the editor's lent Lsp from a view procedure: must stop
//
// 9.38 to 9.40:
//   GK_NEG=ed7-ok            a parsed Custom block (the front matter) kept by gk.retain, placed among the
//                            app's blocks with flows from gk.flow_position_at over that list: correct
//   GK_NEG=ed7-custom-unretained the same Custom block not kept, placed from the second frame on: must
//                            stop (a held value a call hands out lives until the next frame begins)
package negative_editor

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

App :: struct {
	mode:  string,
	root:  gk.View,
	rows:  gk.Row_Source,
	ed:    gk.Editor_State,
	hist:  gk.History,
	drawn: int,
	rowed: int,
	// 9.30 to 9.37
	held:  []gk.Block_Node_Held,
	leaf:  Maybe(gk.Leaf_Geometry),
	leaves: int,
	inp:   gk.Input_State,
	plus:  gk.Row_Source,
	// 9.38 to 9.40
	ext:    gk.Markdown_Extensions_Arc,
	custom: Maybe(gk.Block_Node_Held),
}

MD :: "> [!note]\n> inside\n\n```badge\nok\n```\n"

badge :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.rowed += 1
	n := gk.row_context(cx, a.rows, at, gk.Markdown_Node_Read)
	return gk.label(string(gk.as_text(n, cx)))
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.rows = gk.row_source(cx, a, 0, badge, initial = 0)
	a.ed = gk.editor_state(cx)->default_value("x")->build()
	a.hist = gk.history(cx)->build()
	switch a.mode {
	case "ed-owner-released":
		gk.release(cx, a.ed)
	case "ed-plugin-freed":
		gk.release(cx, a.rows)
	case "ed6-ok", "ed6-held-stale", "ed6-leaf-stale", "ed6-decorate-freed", "ed6-lent-view":
		ext := gk.markdown_extensions_arc(cx, gk.markdown_extensions_default())
		held := gk.base_parse_markdown(cx, "# Kept\n\ntext\n", ext).value.? or_else nil
		for &h in held {
			h = gk.retain(cx, h)
		}
		if a.mode == "ed6-held-stale" && len(held) > 0 {
			gk.release(cx, held[0])
		}
		a.held = held
		a.inp = gk.input_state(cx)->default_value("3")->build()
		a.plus = gk.row_source(cx, a, 0, plus, initial = 1)
		if a.mode == "ed6-decorate-freed" {
			gk.release(cx, a.plus)
		}
	case "ed7-ok", "ed7-custom-unretained":
		a.ext = gk.markdown_extensions_arc(cx, gk.markdown_extensions_default()->frontmatter()->plugin(gk.frontmatter_plugin()))
		held := gk.base_parse_markdown(cx, "---\ntitle: kept\n---\n\ntext\n", a.ext).value.? or_else nil
		if len(held) > 0 {
			a.custom = gk.retain(cx, held[0]) if a.mode == "ed7-ok" else held[0]
		}
	}
}

view7 :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	blocks := make([dynamic]gk.Block_Node, context.temp_allocator)
	append(&blocks, gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->push(gk.inline_node("mine"))->with_leaf_key(7)})
	if c, ok := a.custom.?; ok && (a.mode == "ed7-ok" || a.drawn >= 2) {
		append(&blocks, c) // unretained: swept when the second frame began, so this stops
	}
	flows := make([]Maybe(gk.Flow_Position), len(blocks), context.temp_allocator)
	for _, i in blocks {
		flows[i] = gk.flow_position_at(blocks[:], i, i == len(blocks) - 1)
	}
	rc := gk.render_context_from_app()->with_markdown_extensions(a.ext)
	return gk.v_flex(.Size_Full, .P_4)->child(gk.block_host("doc", blocks[:], flows, rc, on_geometry = gk.on(leaves)))
}

plus :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.rowed += 1
	return gk.label("+")
}

leaves :: proc(a: ^App, cx: ^gk.Cx, e: gk.Leaf_Geometry_Event) {
	for l in e.leaves {
		a.leaves += gk.len_(l.geometry, cx) // read in the handler: correct
		if a.mode == "ed6-leaf-stale" {
			a.leaf = l.geometry // kept without gk.retain: temporary, swept when the next frame begins
		}
	}
}

view6 :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if g, ok := a.leaf.?; ok {
		_ = gk.len_(g, cx) // a released temporary: stops here
	}
	if a.mode == "ed6-lent-view" {
		gk.lsp_mut(a.ed, cx, gk.lsp_default())
	}
	blocks := make([dynamic]gk.Block_Node, context.temp_allocator)
	append(&blocks, gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->push(gk.inline_node("mine"))->with_leaf_key(7)})
	for h in a.held {
		append(&blocks, h)
	}
	return gk.v_flex(.Size_Full, .P_4)->child(gk.block_host("doc", blocks[:], on_geometry = gk.on(leaves)))->
		child(gk.div()->w(gk.px(160))->child(gk.base_number_input(a.inp)->increment_button(gk.Decorate{content = a.plus})))
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.drawn += 1
	if len(a.mode) > 4 && a.mode[:4] == "ed6-" {
		return view6(a, cx)
	}
	if len(a.mode) > 4 && a.mode[:4] == "ed7-" {
		return view7(a, cx)
	}
	body := gk.v_flex(.Size_Full, .P_4)
	ext := gk.markdown_extensions_default()->block_surface("note", gk.div(.P_2)->child(gk.label("Note")))
	text := gk.base_text_view_markdown("md", MD)->markdown_extensions(ext)->
		plugin(gk.Markdown_Plugin{name = "badge", match = .Code, lang = "badge", rows = a.rows})
	switch a.mode {
	case "ed-ok", "ed-plugin-freed":
		body = body->child(text)->child(gk.input_base("ib")->semantic_owner(gk.entity_id(a.ed)))
	case "ed-owner":
		body = body->child(gk.input_base("ib")->semantic_owner(gk.Entity_Id(a.hist.h)))
	case "ed-owner-released":
		body = body->child(gk.input_base("ib")->semantic_owner(gk.entity_id(a.ed)))
	}
	return body
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "negative_editor", {400, 300}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_NEG", context.allocator)
	gk.run_headless(&app, "size 400 300\nframe\nadvance 100\nframe\nframe\nframe\n", start)
	fmt.printf("gk-neg done mode=%s drawn=%d rows=%d leaves=%d\n", app.mode, app.drawn, app.rowed, app.leaves)
}
