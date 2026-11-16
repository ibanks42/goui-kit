// The lists and editor families' modes (docs/PROTOCOLS.md 9.16 to 9.18), called from main.odin:
//
//   ctx-token   an input's token drawn by a row of the app's (an element closure over a context):
//               the row reads the token's context, and is described again when it is selected
//   lend        an editor's diagnostics lent (diagnostics_mut), written, and given back
//   highlight   an editor whose highlighter is the app's: the styles and folds procedures asked
//               between frames with the language, the text and a range
//   languages   the app's language service: an alias, and the syntax procedure asked while typing
//   bad-lend    a lent object used after the turn that lent it (exit 2)
//   bad-ctx     gk.row_context for a row no closure drew (exit 2)
//   small       9.19: a patch's error as the kit's value, a tree's scroll handle as gpui's
//               ScrollHandle (scrolled from code), the completion highlighters over an LSP item
package lists2

import "core:fmt"
import "core:strings"
import gk "gpuikit:gk"

R4 :: struct {
	tokens:   gk.Row_Source,
	token_rows: int,
	lent:     gk.Diagnostic_Set,
	hl:       gk.Highlighter,
	styles_asked, folds_asked, syntax_asked: int,
	prov:     gk.Row_Source,
}

r4_modes :: proc(mode: string) -> bool {
	switch mode {
	case "ctx-token", "lend", "highlight", "languages", "bad-lend", "bad-ctx", "small":
		return true
	}
	return false
}

r4_token_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.r4.token_rows += 1
	t := gk.row_context(cx, a.r4.tokens, at, gk.Inline_Token_Context)
	fmt.printf("lists2 ctx token row=%d range=%v selected=%v disabled=%v readonly=%v label=%q height_ok=%v\n",
		at.row, t->range(cx), t->is_selected(cx), t->is_disabled(cx), t->is_readonly(cx), t->token(cx)->label(cx),
		t->line_height(cx) > 0)
	return gk.input_token(t)
}

r4_styles :: proc(a: ^App, cx: ^gk.Cx, q: gk.Highlight_Query) -> []gk.Highlight_Run {
	a.r4.styles_asked += 1
	fmt.printf("lists2 highlight lang=%s range=%v text=%q\n", q.language, q.range, q.text)
	out := make([dynamic]gk.Highlight_Run, context.temp_allocator)
	if i := strings.index(string(q.text), "proc"); i >= 0 {
		append(&out, gk.Highlight_Run{gk.Range{i, i + 4}, "keyword"})
	}
	return out[:]
}

r4_folds :: proc(a: ^App, cx: ^gk.Cx, q: gk.Fold_Query) -> []gk.Fold_Range {
	a.r4.folds_asked += 1
	fmt.printf("lists2 highlight folds lang=%s lines=%d\n", q.language, strings.count(string(q.text), "\n"))
	out := make([]gk.Fold_Range, 1, context.temp_allocator)
	out[0] = {start_line = 0, end_line = 2}
	return out
}

r4_syntax :: proc(a: ^App, cx: ^gk.Cx, q: gk.Syntax_Query) -> gk.Syntax_Context {
	a.r4.syntax_asked += 1
	fmt.printf("lists2 syntax asked lang=%s offset=%d\n", q.language, q.offset)
	return .String
}

r4_init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "ctx-token":
		a.in1 = gk.input_state(cx)->default_value("Hello world")->build()
		_ = a.in1->replace_range_with_token(cx, gk.Range{6, 11}, gk.inline_token("t1", "world")->with_label("World"))
		a.r4.tokens = gk.row_source(cx, a, 0, r4_token_row, initial = 0)
	case "lend", "bad-lend":
		a.ed = gk.editor_state(cx)->language("odin")->default_value("main :: proc() {}\n")->build()
	case "highlight":
		a.r4.hl = gk.highlighter(cx, a, r4_styles, r4_folds, languages = {"odin"})
		a.ed = gk.editor_state(cx)->language("odin")->default_value("main :: proc() {\n\n}\n")->build()
		a.ed->set_highlighter_factory(cx, a.r4.hl.src)
	case "languages":
		aliases := [?]gk.Language_Alias{{"od", "odin"}}
		a.r4.prov = gk.language_provider(cx, a, aliases[:], r4_syntax)
		gk.base_set_language_provider(cx, a.r4.prov)
		a.ed = gk.editor_state(cx)->language("odin")->default_value("x := \n")->build()
	case "bad-ctx":
		a.r4.tokens = gk.row_source(cx, a, 0, r4_token_row, initial = 0)
	case "small":
		a.tree = gk.tree_state(cx)->build()
		nodes := make([]gk.Tree_Node, 300, context.temp_allocator)
		for &n, i in nodes {
			n = {id = fmt.tprintf("n%d", i), label = fmt.tprintf("node %d", i)}
		}
		gk.tree_set_items(cx, a.tree, nodes)
		a.tree_rows = gk.tree_source(cx, a, drive_tree_row, initial = 8)
	}
}

r4_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	switch a.mode {
	case "ctx-token":
		return gk.div()->w(gk.px(260))->child(gk.input(a.in1)->token(a.r4.tokens))
	case "lend", "bad-lend", "highlight", "languages":
		return gk.div()->w(gk.px(400))->child(gk.code_editor(a.ed, height = 120))
	case "small":
		return gk.div()->w(gk.px(120))->h(gk.px(200))->child(gk.tree(a.tree, a.tree_rows, .Size_Full))
	}
	return gk.label("-")
}

r4_step :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "ctx-token":
		a.in1->set_selected_range(cx, gk.Range{0, 11}) // the token is selected now: its row is described again
	case "lend", "bad-lend":
		switch a.step {
		case 1:
			d, ok := a.ed->diagnostics_mut(cx).?
			fmt.printf("lists2 lend got=%v empty=%v\n", ok, ok && d->is_empty(cx))
			if ok {
				d->push(cx, gk.diagnostic({gk.Lsp_Position{0, 0}, gk.Lsp_Position{0, 4}}, "unused"))
				d->push(cx, gk.diagnostic({gk.Lsp_Position{0, 8}, gk.Lsp_Position{0, 12}}, "twice"))
				a.r4.lent = d
			}
		case 2:
			if a.mode == "bad-lend" {
				fmt.printf("lists2 bad-lend using the lent handle\n")
				_ = a.r4.lent->len_(cx) // given back at the end of the turn that lent it: stops
				return
			}
			got, ok := a.ed->diagnostics(cx).?
			n := ok ? got->len_(cx) : -1
			lent, back := gk.test_lends(cx)
			fmt.printf("lists2 lend after len=%d lent=%d given_back=%d\n", n, lent, back)
		}
	case "highlight":
		gets, asked, answered := gk.test_asks(cx)
		fmt.printf("lists2 highlight counts styles=%v folds=%v asked=%d answered=%d gets_more=%v\n",
			a.r4.styles_asked > 0, a.r4.folds_asked > 0, asked, answered, gets > asked)
	case "languages":
		fmt.printf("lists2 languages syntax_asked=%v\n", a.r4.syntax_asked > 0)
	case "small":
		switch a.step {
		case 1:
			e, ok := gk.diff_file_parse_error(cx, "@@ nonsense\n").?
			_, none := gk.diff_file_parse_error(cx, DIFF_PATCH).?
			fmt.printf("lists2 small parse-error ok=%v line=%d message=%q good_has_error=%v\n", ok, ok ? e->line(cx) : -1,
				ok ? e->message(cx) : "", none)
			ranges := gk.base_highlight_prefix(cx, "ma", gk.Completion_Item{label = "main"})
			fmt.printf("lists2 small highlight_prefix=%v\n", ranges)
			sh := a.tree->scroll_handle(cx)
			fmt.printf("lists2 small scroll before y=%v\n", sh->offset(cx).y)
			sh->set_offset(cx, gk.Gpui_Point{0, -4000})
		case 2:
			far := false
			for row in 100 ..< 300 {
				if drawn(a, "tree", row) {
					far = true
				}
			}
			fmt.printf("lists2 small scroll after y=%v drawn_far=%v\n", a.tree->scroll_handle(cx)->offset(cx).y, far)
		}
	case "bad-ctx":
		fmt.printf("lists2 bad-ctx reading a row no closure drew\n")
		_ = gk.row_context(cx, a.r4.tokens, {0, 3}, gk.Inline_Token_Context)
	}
}

r4_script :: proc(a: ^App, s: ^strings.Builder) {
	switch a.mode {
	case "ctx-token":
		strings.write_string(s, "frame\nframe\nframe\n")
		strings.write_string(s, STEP)
		strings.write_string(s, "frame\nframe\nframe\n")
	case "lend", "bad-lend":
		strings.write_string(s, "frame\n")
		strings.write_string(s, STEP)
		strings.write_string(s, "frame\nframe\n")
		strings.write_string(s, STEP)
	case "highlight":
		strings.write_string(s, "frame\nadvance 100\nframe\nadvance 100\nframe\nframe\n")
		strings.write_string(s, STEP)
	case "languages":
		// The caret after "x := ", then a quote: the editor asks where in the syntax it is.
		strings.write_string(s, "click 300 80\nframe\nkey end\ntype \"\nframe\nadvance 100\nframe\ntype \"\nframe\nframe\n")
		strings.write_string(s, STEP)
	case "bad-ctx":
		strings.write_string(s, STEP)
	case "small":
		strings.write_string(s, "frame\n")
		strings.write_string(s, STEP)
		strings.write_string(s, "frame\nframe\nframe\n")
		strings.write_string(s, STEP)
	}
}
