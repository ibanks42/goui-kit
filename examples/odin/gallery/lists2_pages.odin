// The lists and editor families' gallery pages, the Odin twin of ref/src/bin/gallery/lists2_pages.rs,
// call for call: selects and a combobox over the library's search delegate (items as data, rows
// and group headers as row views) and a table whose looks are data (docs/PROTOCOLS.md sections
// 9.9 and 9.10). The steps of ref/gallery/pages.txt open the menus, type and press keys.
package gallery

import "core:fmt"
import gk "gpuikit:gk"

Lists2_Pages :: struct {
	select:  gk.Select_State,
	search:  gk.Select_State,
	rows:    gk.Select_State,
	combo:   gk.Combobox_State,
	table:   gk.Table_State,
	cells:   gk.Row_Source,
	values:  gk.View, // the "Values" group header cell
	// docs/PROTOCOLS.md 9.14: a code block highlighter the app answers between frames
	hl:      gk.Ask(gk.Code_Block, []gk.Tuple_Range_Highlight_Style),
	diff:    gk.Diff_State, // a patch parsed by DiffFile::parse (PROTOCOLS.md 9.15)
	// docs/PROTOCOLS.md 9.16 to 9.18
	token:    gk.Input_State, // an input with a token drawn by the app's row (9.16)
	editor:   gk.Editor_State, // diagnostics lent (9.17), a highlighter of the app's (9.18)
	keywords: gk.Highlighter,
}

LISTS2_PAGES := [?]Page {
	{"lists2-select", lists2_select},
	{"lists2-select-search", lists2_select_search},
	{"lists2-select-rows", lists2_select_rows},
	{"lists2-combobox", lists2_combobox},
	{"lists2-table-look", lists2_table_look},
	{"lists2-table-static", lists2_table_static},
	{"text-highlight", text_highlight},
	{"diff-view", diff_view},
	{"text-token", text_token},
	{"diff-header", diff_header},
	{"editor-diagnostics", editor_diagnostics},
}

LISTS2_FRUIT := [?]gk.Search_Item {
	{value = "apple", title = "Apple"},
	{value = "banana", title = "Banana"},
	{value = "cherry", title = "Cherry"},
	{value = "grape", title = "Grape", disabled = true},
	{value = "mango", title = "Mango", display = "Mango (ripe)"},
}

LISTS2_VEG := [?]gk.Search_Item{{value = "kale", title = "Kale"}, {value = "leek", title = "Leek"}}

lists2_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	p := &a.lists2
	p.hl = gk.ask(cx, a, text_highlight_spans)
	// A patch parsed by the kit: how many files it holds, then each file by its index.
	n := gk.diff_file_parse_count(cx, DIFF_VIEW_PATCH)
	files := make([]gk.Diff_File, n.value.? or_else 0, context.temp_allocator)
	for &f, i in files {
		f = gk.diff_file_parse_at(DIFF_VIEW_PATCH, i)
	}
	p.diff = gk.diff_state(cx, files)->build()
	// a token drawn by a row of the app's (9.16), a diff's header suffixes, an editor
	// whose diagnostics are lent and given back (9.17) and whose highlighter is the app's (9.18).
	p.token = gk.input_state(cx)->default_value("Hello world")->build()
	_ = p.token->replace_range_with_token(cx, gk.Range{6, 11}, gk.inline_token("t1", "world")->with_label("World"))
	p.keywords = gk.highlighter(cx, a, editor_keywords)
	p.editor = gk.editor_state(cx)->language("odin")->default_value(EDITOR_DIAG_TEXT)->build()
	p.editor->set_highlighter_factory(cx, p.keywords.src)
	if d, ok := p.editor->diagnostics_mut(cx).?; ok {
		d->push(cx, gk.diagnostic({gk.Lsp_Position{1, 4}, gk.Lsp_Position{1, 10}}, "never called"))
	}
	src := gk.search_source(cx, a, LISTS2_FRUIT[:])
	p.select = gk.select_state(cx, src, 4)->build()
	groups := [?]gk.Search_Group{{title = "Fruit", items = LISTS2_FRUIT[:]}, {title = "Veg", items = LISTS2_VEG[:]}}
	p.search = gk.select_state(cx, gk.search_source_groups(cx, a, groups[:]), nil)->searchable(true)->build()
	p.rows = gk.select_state(cx, gk.search_source_groups(cx, a, groups[:], lists2_row, headers = true), nil)->build()
	p.combo = gk.combobox_state(cx, gk.search_source(cx, a, LISTS2_FRUIT[:]), []int{1})->multiple(true)->build()
	columns := [?]gk.Table_Column{gk.table_column("id", "Id", 60), gk.table_column("name", "Name", 100), gk.table_column("a", "A", 60)}
	p.cells = gk.table_source(cx, a, 6, columns[:], lists2_cell)
	p.table = gk.table_state(cx, p.cells)->build()
	p.values = gk.view(a, nil, lists2_values)
	level := [?]gk.Table_Group{{label = "Key", span = 1}, {label = "Values", span = 2}}
	levels := [?][]gk.Table_Group{level[:]}
	views := [?]gk.Table_Group_View{{label = "Values", view = p.values}}
	gk.table_look(cx, p.table, {
		groups = levels[:],
		group_views = views[:],
		header_style = gk.style_refinement_default()->bg(gk.theme(.Secondary)),
		row_style = gk.style_refinement_default()->text_color(gk.theme(.Muted_Foreground)),
	})
}

lists2_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	if at.row == gk.HEADER {
		return gk.label(at.section == 0 ? "== fruit ==" : "== veg ==")
	}
	items := at.section == 0 ? LISTS2_FRUIT[:] : LISTS2_VEG[:]
	return gk.label(fmt.tprintf("> %s", items[at.row].title))
}

lists2_cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	return gk.label(fmt.tprintf("%d:%d", at.row, at.col))
}

lists2_values :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.label("[values]")
}

lists2_select :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(220))->child(gk.select(a.lists2.select)->placeholder("Fruit"))
}

lists2_select_search :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(220))->child(gk.select(a.lists2.search)->placeholder("Food")->search_placeholder("Find"))
}

lists2_select_rows :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(220))->child(gk.select(a.lists2.rows)->placeholder("Food"))
}

lists2_combobox :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(260))->child(gk.combobox(a.lists2.combo)->placeholder("Fruit"))
}

lists2_table_look :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(440))->h(gk.px(240))->child(gk.data_table(a.lists2.table)->scrollbar_visible(false, false))
}

lists2_table_static :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	fruit := [?]string{"Apple", "Banana", "Cherry"}
	rows: [3]gk.Table_Child
	for f, n in fruit {
		rows[n] = gk.table_row()->child(gk.table_cell()->child(gk.text(f)))->child(gk.table_cell()->child(gk.text(fmt.tprintf("%d", n * 3))))
	}
	return gk.div()->w(gk.px(360))->child(
		gk.table()->
		child(gk.table_header()->child(gk.table_row()->child(gk.table_head()->child(gk.text("Fruit")))->child(gk.table_head()->child(gk.text("Count")))))->
		child(gk.table_body()->children(rows[:])),
	)
}

// text-highlight: a Markdown code block whose highlights the app gives (an ask, PROTOCOLS.md
// 9.14): "proc" and "return" in red, as the Rust page's closure does.
TEXT_HIGHLIGHT_WORDS := [?]string{"proc", "return"}
TEXT_HIGHLIGHT_MD :: "# Code\n\n```odin\nmain :: proc() {\n    return\n}\n```\n"

text_highlight_spans :: proc(a: ^App, cx: ^gk.Cx, b: gk.Code_Block) -> []gk.Tuple_Range_Highlight_Style {
	code := string(b.code)
	out := make([dynamic]gk.Tuple_Range_Highlight_Style, context.temp_allocator)
	i := 0
	for i < len(code) {
		n := 0
		for kw in TEXT_HIGHLIGHT_WORDS {
			if len(code) - i >= len(kw) && code[i:i + len(kw)] == kw {
				n = len(kw)
			}
		}
		if n > 0 {
			append(&out, gk.Tuple_Range_Highlight_Style{gk.Range{i, i + n}, gk.Highlight_Style{color = gk.hsla(0, 0.7, 0.5, 1)}})
			i += n
		} else {
			i += 1
		}
	}
	return out[:]
}

text_highlight :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(360))->child(gk.base_text_view_markdown("hl", TEXT_HIGHLIGHT_MD)->code_block_highlighter(a.lists2.hl))
}

// diff-view: a patch parsed by the kit (DiffFile::parse) shown by its Diff.
DIFF_VIEW_PATCH :: "diff --git a/hello.odin b/hello.odin\n--- a/hello.odin\n+++ b/hello.odin\n@@ -1,4 +1,4 @@\n package hello\n \n-main :: proc() {}\n+main :: proc() { run() }\n // end\n"

diff_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(440))->h(gk.px(200))->child(gk.diff(a.lists2.diff))
}

// --- docs/PROTOCOLS.md 9.16 to 9.18 ----------------------------------------------------

// text-token: the input's token drawn by an item procedure of the app's: the kit's InputToken over
// the token's context, which the kit hands it (9.16, given with gk.items since 9.41).
text_token_item :: proc(a: ^App, cx: ^gk.Cx, t: gk.Inline_Token_Context) -> gk.Element {
	return gk.input_token(t)
}

text_token :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(260))->child(gk.input(a.lists2.token)->token(gk.items(cx, a, text_token_item)))
}

// diff-header: each file's header ends with its path in brackets, an item procedure over the file.
diff_header_item :: proc(a: ^App, cx: ^gk.Cx, f: gk.Diff_File_Read) -> gk.Element {
	return gk.label(fmt.tprintf("[%s]", f->path(cx)))
}

diff_header :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(440))->h(gk.px(200))->child(gk.diff(a.lists2.diff)->render_header_suffix(gk.items(cx, a, diff_header_item)))
}

// editor-diagnostics: "proc" and "return" are keyword runs (the editor's resolver gives their
// colour), as the Rust page's highlighter answers.
EDITOR_DIAG_TEXT :: "main :: proc() {\n    helper()\n    return\n}\n"

editor_keywords :: proc(a: ^App, cx: ^gk.Cx, q: gk.Highlight_Query) -> []gk.Highlight_Run {
	text := string(q.text)
	out := make([dynamic]gk.Highlight_Run, context.temp_allocator)
	i := 0
	for i < len(text) {
		n := 0
		for kw in TEXT_HIGHLIGHT_WORDS {
			if len(text) - i >= len(kw) && text[i:i + len(kw)] == kw {
				n = len(kw)
			}
		}
		if n > 0 {
			append(&out, gk.Highlight_Run{gk.Range{i, i + n}, "keyword"})
			i += n
		} else {
			i += 1
		}
	}
	return out[:]
}

editor_diagnostics :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(480))->child(gk.code_editor(a.lists2.editor, height = 160))
}
