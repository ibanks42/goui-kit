package gallery

// The editor family's gallery pages (docs/PROTOCOLS.md 9.20 to 9.23), the Odin twin of
// ref/src/bin/gallery/editor_pages.rs, call for call:
//
//   editor5-surface   a block surface: the app's Div around a blockquote's blocks (a typed fragment)
//   editor5-heading   a text style's headings by level (a table the app gives for Fn(u8) -> style)
//   editor5-plugin    Markdown plugins: the kit's frontmatter plugin, a data plugin drawing ```badge
//                    code blocks as rows of the app's, another drawing inline code inside the line
//   editor5-tooltip   a link's tooltip: the row of the link's URL (a view closure over a value)
//   editor6-host      the editor host (9.30): the app's blocks (a heading, a paragraph of
//                    inline nodes, a list of items, a code block) and parsed ones (held values) drawn
//                    by gk.block_host, a selection painted behind a keyed leaf
//   editor7-number    decorators kept for later (9.32, 9.40): a number input's buttons given
//                    a style and a row each (gk.Decorate), the state made in init
//   editor7-scroller  a message scroller scrolled up: its jump button decorated (a style, a row); no
//                    scrollbar (its fade after a scroll follows the wall clock, so no two runs agree)
//   editor7-lsp       the Lsp's completion highlighter (9.33, an ask): the menu the app presents on a
//                    click, each label highlighted where the app says
//   editor7-diagnostic  a diagnostic presented on a click (9.34): the editor's popover

import "core:fmt"
import gk "gpuikit:gk"

Editor_Pages :: struct {
	blocks:  gk.Row_Source, // editor5-plugin: the badge blocks
	inlines: gk.Row_Source, // editor5-plugin: the inline code
	tips:    gk.Row_Source, // editor5-tooltip: one row per URL
	held:    [dynamic]gk.Block_Node_Held, // editor6-host: the parsed blocks, kept
	// the editor7-* pages
	number:   gk.Input_State, // editor7-number
	plus:     gk.Row_Source,
	minus:    gk.Row_Source,
	scroller: gk.Message_Scroller_State, // editor7-scroller
	messages: gk.Row_Source,
	jump:     gk.Row_Source,
	lsp:      gk.Editor_State, // editor7-lsp
	hl:       gk.Ask(gk.Tuple_Temp_String_Completion_Item, []gk.Range),
	diag:     gk.Editor_State, // editor7-diagnostic
}

EDITOR_SURFACE_MD :: "> [!note]\n> Inside the surface.\n\nAfter it.\n"
EDITOR_HEADING_MD :: "# One\n\n## Two\n\n### Three\n\nBody text.\n"
EDITOR_PLUGIN_MD :: "---\ntitle: Notes\nowner: me\n---\n\n# Plugins\n\n```badge\nready\n```\n\nSome `inline` text.\n"
EDITOR_TOOLTIP_MD :: "[docs](https://example.com/docs) and more"
EDITOR_HOST_MD :: "## Parsed\n\n- one\n- two\n\n> quoted *text*\n"

EDITOR_PAGES := []Page {
	{"editor5-surface", editor_surface},
	{"editor5-heading", editor_heading},
	{"editor5-plugin", editor_plugin},
	{"editor5-tooltip", editor_tooltip},
	{"editor6-host", editor_host},
	{"editor7-number", editor_number},
	{"editor7-scroller", editor_scroller},
	{"editor7-lsp", editor_lsp},
	{"editor7-diagnostic", editor_diagnostic},
}

editor_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	p := &a.editor
	p.blocks = gk.row_source(cx, a, 0, editor_badge_row, initial = 0)
	p.inlines = gk.row_source(cx, a, 0, editor_code_row, initial = 0)
	p.tips = gk.row_source(cx, a, 0, editor_tip_row, initial = 0)
	parsed := gk.base_parse_markdown(cx, EDITOR_HOST_MD, gk.markdown_extensions_arc(cx, gk.markdown_extensions_default()))
	for b in parsed.value.? or_else nil {
		append(&p.held, gk.retain(cx, b))
	}
	// the editor7-* pages
	p.number = gk.input_state(cx)->default_value("4")->build()
	p.plus = gk.row_source(cx, a, 1, proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {return gk.label("+")}, initial = 1)
	p.minus = gk.row_source(cx, a, 1, proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {return gk.label("-")}, initial = 1)
	p.scroller = gk.message_scroller_state(cx, EDITOR_MESSAGES)
	p.messages = gk.row_source(cx, a, EDITOR_MESSAGES, editor_message_row, initial = EDITOR_MESSAGES)
	p.jump = gk.row_source(cx, a, 1, proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {return gk.label("latest")}, initial = 1)
	p.hl = gk.ask(cx, a, editor_highlight)
	p.lsp = gk.editor_state(cx)->default_value("fi")->build()
	gk.lsp_mut(p.lsp, cx, gk.lsp_default()->with_completion_highlighter(p.hl))
	p.diag = gk.editor_state(cx)->default_value("let x = 1\n")->build()
}

editor_surface :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	ext := gk.markdown_extensions_default()->block_surface("note", gk.div(.P_2)->bg(gk.theme(.Secondary))->child(gk.label("Note")))
	return gk.div()->w(gk.px(360))->child(gk.base_text_view_markdown("surface", EDITOR_SURFACE_MD)->markdown_extensions(ext))
}

editor_heading :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	// By level: 0 is never asked, 1 and 2 are coloured, the rest (past the end) upstream's default.
	levels := []gk.Style_Refinement {
		gk.style_refinement_default(),
		gk.style_refinement_default()->text_color(gk.theme(.Danger)),
		gk.style_refinement_default()->text_color(gk.theme(.Success)),
	}
	style := gk.base_text_view_style_default()->with_heading(levels)
	// The next page's text view drawn here first: a plugin's rows are described the frame after
	// its nodes are parsed (one frame of nothing, DESIGN.md 0 decision 3), and a page's first shot
	// is taken on its first frame. Laid out with no height, so this page does not show it.
	return gk.div()->w(gk.px(360))->child(gk.base_text_view_markdown("heading", EDITOR_HEADING_MD)->style(style))->
		child(gk.div(.Overflow_Hidden)->h(gk.px(0))->child(editor_plugin(a, cx)))
}

editor_badge_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	n := gk.row_context(cx, a.editor.blocks, at, gk.Markdown_Node_Read)
	return gk.label(fmt.tprintf("[%s]", gk.as_text(n, cx)))
}

editor_code_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	v := gk.row_context(cx, a.editor.inlines, at, gk.Markdown_Inline)
	return gk.label(fmt.tprintf("<%s>", gk.as_text(v.node, cx)))
}

editor_plugin :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	p := &a.editor
	ext := gk.markdown_extensions_default()->frontmatter()->plugin(gk.frontmatter_plugin())
	return gk.div()->w(gk.px(420))->child(gk.base_text_view_markdown("plugin", EDITOR_PLUGIN_MD)->markdown_extensions(ext)->
		plugin(gk.Markdown_Plugin{name = "badge", match = .Code, lang = "badge", rows = p.blocks})->
		plugin(gk.Markdown_Plugin{name = "code", match = .Inline_Code, rows = p.inlines, baseline = 12}))
}

editor_tip_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	url := gk.row_context(cx, a.editor.tips, at, gk.Temp_String)
	return gk.label(fmt.tprintf("open %s", url))
}

editor_tooltip :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(360))->child(gk.base_text_view_markdown("tooltip", EDITOR_TOOLTIP_MD)->link_tooltip(a.editor.tips))
}

editor_host :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	bs := make([dynamic]gk.Block_Node, context.temp_allocator)
	append(&bs, gk.Block_Node_Heading{level = 2, children = gk.paragraph_default()->push(gk.inline_node("Host"))->with_leaf_key(1)})
	append(&bs, gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->
		push(gk.inline_node("Hello ")->marks({{gk.Range{0, 5}, gk.text_mark_default()->bold()}}))->
		push(gk.inline_node("world"))->with_leaf_key(2)})
	one := make([]gk.Block_Node, 1, context.temp_allocator)
	one[0] = gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->push(gk.inline_node("first"))->with_leaf_key(3)}
	two := make([]gk.Block_Node, 1, context.temp_allocator)
	two[0] = gk.Block_Node_Paragraph{v0 = gk.paragraph_default()->push(gk.inline_node("second"))}
	items := make([]gk.Block_Node, 2, context.temp_allocator)
	items[0] = gk.Block_Node_List_Item{children = one}
	items[1] = gk.Block_Node_List_Item{children = two, checked = true}
	append(&bs, gk.Block_Node_List{children = items})
	append(&bs, gk.Block_Node_Code_Block{v0 = gk.base_code_block_from_code("x := 1", "odin")->with_leaf_key(4)})
	for h in a.editor.held {
		append(&bs, h)
	}
	decs := []gk.Leaf_Decoration{{key = 2, ranges = {{gk.Range{0, 5}, gk.theme(.Selection)}}}}
	return gk.div()->w(gk.px(420))->child(gk.block_host("editor6-host", bs[:], decorations = decs))
}

// ---------------------------------------------------------------- docs/PROTOCOLS.md 9.38 to 9.40

EDITOR_MESSAGES :: 30

editor_number :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	p := &a.editor
	return gk.div()->w(gk.px(200))->child(gk.base_number_input(p.number)->
		increment_button(gk.Decorate{style = gk.style_refinement_default()->bg(gk.theme(.Primary)), content = p.plus})->
		decrement_button(gk.Decorate{content = p.minus}))
}

editor_message_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	return gk.label(fmt.tprintf("message %d", at.row))
}

editor_scroller :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	p := &a.editor
	return gk.div()->w(gk.px(300))->h(gk.px(200))->child(gk.message_scroller("scroller", p.scroller, p.messages)->scrollbar(false)->
		with_jump_button_renderer(gk.Decorate{style = gk.style_refinement_default()->bg(gk.theme(.Danger)), content = p.jump}))
}

// editor_highlight: the completion menu's highlighter, the second and third character of each label.
editor_highlight :: proc(a: ^App, cx: ^gk.Cx, key: gk.Tuple_Temp_String_Completion_Item) -> []gk.Range {
	out := make([]gk.Range, 1, context.temp_allocator)
	out[0] = {min(1, len(key.v1.label)), min(3, len(key.v1.label))}
	return out
}

editor_complete :: proc(a: ^App, cx: ^gk.Cx) {
	items := []gk.Completion_Item{{label = "first", kind = 3, detail = "fn first()"}, {label = "fifth", kind = 6}}
	gk.present_completion_items(a.editor.lsp, cx, 2, "fi", items)
}

editor_lsp :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->child(gk.button("complete")->label("Complete")->on_click(gk.on(editor_complete)))->
		child(gk.div()->w(gk.px(420))->child(gk.code_editor(a.editor.lsp, height = 160)))
}

editor_diagnose :: proc(a: ^App, cx: ^gk.Cx) {
	gk.present_diagnostic(a.editor.diag, cx, {4, 5}, gk.diagnostic({{0, 4}, {0, 5}}, "unused x")->with_severity(.Warning))
}

editor_diagnostic :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->child(gk.button("diagnose")->label("Diagnose")->on_click(gk.on(editor_diagnose)))->
		child(gk.div()->w(gk.px(420))->child(gk.code_editor(a.editor.diag, height = 160)))
}
