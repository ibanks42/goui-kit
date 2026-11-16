package gk

// The editor family (docs/PROTOCOLS.md 9.20 to 9.26): what the generated procedures of
// gen/gk_editor.py's rules take that the generator does not declare.
//
// Markdown plugins (9.23). `plugin` of a text view or of Markdown extensions takes one of the kit's
// plugins (gk.frontmatter_plugin()) or a data plugin of the app's:
//
//   a.badges = gk.row_source(cx, a, 0, badge_row, initial = 0)
//   gk.text_view(..)->plugin(gk.Markdown_Plugin{name = "badge", match = .Code, lang = "badge", rows = a.badges})
//
//   badge_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
//       n := gk.row_context(cx, a.badges, at, gk.Markdown_Node_Read)
//       return gk.label(n->as_text(cx))
//   }
//
// The library claims the nodes of the rule while upstream parses (a background task: the app is not
// asked), and draws each node as a row of `rows`, described between frames like any row (one frame
// of nothing at first). An inline plugin's row reads a Markdown_Inline: the node and the inherited
// typography (gk.Inline_Render_Context: text style, font size, line height, rem size).

// Markdown_Match is the kind of Markdown node a data plugin claims. The first six are blocks, the
// rest inline. Code and the JSX elements also match on `lang`: a fenced code block's language, an
// element's name ("" matches every one).
Markdown_Match :: enum u8 {
	Code,
	Math,
	Yaml,
	Toml,
	Html,
	Mdx_Element,
	Inline_Code,
	Inline_Math,
	Inline_Html,
	Mdx_Inline_Element,
}

// Markdown_Plugin is a data plugin: its name (the node's name, `as_text` the node's text, `as_markdown`
// its source), the nodes it claims, the row source that draws them, and an inline node's baseline in
// pixels from its top (0: upstream's, the box's bottom on the text's descent).
Markdown_Plugin :: struct {
	name:     string,
	match:    Markdown_Match,
	lang:     string,
	rows:     Row_Source,
	baseline: f32,
}

// Markdown_Plugin_Any is what `plugin` takes: a kit plugin or a data plugin.
Markdown_Plugin_Any :: union {
	Frontmatter_Plugin,
	Markdown_Plugin,
}

// Markdown_Inline is what an inline data plugin's row reads with gk.row_context.
Markdown_Inline :: struct {
	node:       Markdown_Node_Read,
	typography: Inline_Render_Context,
}

put_markdown_plugin :: proc(o: ^Tape, p: Markdown_Plugin) {
	put_str(o, p.name)
	put_u8(o, u8(p.match))
	put_str(o, p.lang)
	put_u64(o, p.rows.h)
	put_f32(o, p.baseline)
}

// Entity_Id is the entity behind an input, textarea or editor handle, where upstream takes a
// gpui::EntityId (`InputBase::semantic_owner`): gk.entity_id(a.editor) (9.24).
Entity_Id :: distinct u64

entity_id :: proc(state: $T) -> Entity_Id {
	return Entity_Id(state.h)
}

// test_editor_text: the library's counts for the editor family (tests): Markdown nodes the data plugins
// claimed, rows made for the distinct values of view closures.
test_editor_text :: proc(cx: ^Cx, loc := #caller_location) -> (parsed, view_rows: int) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_EDITOR_TEST)
	r := Reader{must_call(req[:], loc), 0}
	return int(rd_u64(&r)), int(rd_u64(&r))
}

// --- docs/PROTOCOLS.md 9.30 onward ------------------------------------------------------
//
// The editor host (9.30) is generated: gk.block_host(id, blocks, flows, render_context, decorations,
// on_geometry) in gen_editor_host.odin, gk.base_parse_markdown(cx, source, extensions) returning
// []gk.Block_Node_Held (each goes wherever a gk.Block_Node goes), and the leaves' gk.Leaf_Geometry
// reads (`g->bounds(cx)`, `g->index_for_position(cx, at)`, ..):
//
//   blocks := []gk.Block_Node{gk.Block_Node_Paragraph{v0 = gk.paragraph()->push(gk.inline_node("Hello"))->with_leaf_key(1)}}
//   gk.block_host("doc", blocks, on_geometry = gk.on(proc(a: ^App, cx: ^gk.Cx, e: gk.Leaf_Geometry_Event) {
//       for l in e.leaves { a.caret = l.geometry->position_for_index(cx, 3) }
//   }))
//
// Decorators kept for later (9.32): what an element upstream hands a decorator would get, given as
// data, because upstream calls the decorator while it renders, where the app cannot run:
//
//   gk.base_number_input(st)->increment_button(gk.Decorate{style = gk.style_refinement()->bg(.Primary), content = a.plus_rows})
//
// `style` refines the element's own style (its colours resolved where the view is described), and
// `content`, a row source (zero: none), adds its row 0 as the element's child, laid out where the
// window is (as the fold placeholder's rows, 9.22).
Decorate :: struct {
	style:   Maybe(Style_Refinement),
	content: Row_Source,
}

@(private)
put_decorate :: proc(o: ^Tape, d: Decorate) {
	if s, ok := d.style.?; ok {
		put_u8(o, 1)
		put_u32(o, take_value(s.nd))
	} else {
		put_u8(o, 0)
	}
	put_u64(o, d.content.h)
}

// test_editor_host_counts: editor hosts decoded, leaves collected, leaf events delivered, decorators
// run, lent values edited (tests; call EDITOR_HOST_TEST).
test_editor_host_counts :: proc(cx: ^Cx, loc := #caller_location) -> (hosts, leaves, delivered, decorated, lent: int) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_EDITOR_HOST_TEST)
	r := Reader{must_call(req[:], loc), 0}
	return int(rd_u64(&r)), int(rd_u64(&r)), int(rd_u64(&r)), int(rd_u64(&r)), int(rd_u64(&r))
}
