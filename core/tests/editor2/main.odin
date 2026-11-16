// editor2: the protocol tests of the editor family: docs/PROTOCOLS.md 9.20 to 9.26 here,
// 9.30 to 9.37 in round6.odin and 9.38 to 9.40 in round7.odin.
// Offscreen under a gk-meter script (the diag library). Each mode prints lines
// core/tests/run-editor2.sh checks, with the exit code.
//
//   GK_PROTO=plugin      Markdown plugins: the kit's frontmatter plugin, a data plugin claiming
//                        ```badge code blocks (a block row) and one claiming inline code (an inline
//                        row reading the node and the inherited typography); the claimed count
//   tooltip              a text view's link tooltip: the mouse on the link, the row of its URL
//                        described (a view closure over a value, one row per distinct URL)
//   surface              a block surface: a Div fragment of the app's view around a blockquote
//   history              History::retain over the entries the app keeps (a predicate as a key set)
//   conflict             a working file's merge conflict read back (DiffFile::conflicts: the
//                        object a read-back value hands out, its seven reads)
//   annotation           a diff annotation's id read back as text
//   fold                 an editor's fold placeholder: the row of the folded line, drawn by an
//                        element closure upstream calls without a window
//   owner                an InputBase whose semantic owner is an editor (gk.entity_id)
//   bad-owner            gk.entity_id of a handle that is not an input: must stop (exit 2)
//   decorations          an editor's decoration collection: made from decorations with colours (a
//                        record with a colour in a call op, decoded in place), appended, cleared
//   display-map          an editor's display map read through the borrow (reads of the state)
//   mask-variant         a mask as MaskPattern's Pattern variant (made by its constructor), typed into
//   route                an action routed to the editor's overlays (an action in a call op): none open
//   editor-style         an editor's style set by the app: its colours, and its highlight resolver an
//                        ask the editor consults for the highlighter's names (a one-method delegate)
package editor2

import "core:fmt"
import "core:os"
import "core:strings"
import gk "gpuikit:gk"

App :: struct {
	mode:     string,
	root:     gk.View,
	step:     int,
	blocks:   gk.Row_Source,
	inlines:  gk.Row_Source,
	tips:     gk.Row_Source,
	folds:    gk.Row_Source,
	ed:       gk.Editor_State,
	inp:      gk.Input_State,
	hist:     gk.History,
	hl:       gk.Highlighter,
	styles:   gk.Ask(gk.Temp_String, Maybe(gk.Highlight_Style)),
	coll:     gk.Text_Decoration_Collection,
	block_rows, inline_rows, tip_rows, fold_rows: int,
	r6:       Round6, // round6.odin
	r7:       Round7, // round7.odin
}

PLUGIN_MD :: "---\ntitle: Notes\nowner: me\n---\n\n# Plugins\n\n```badge\nready\n```\n\nSome `inline` text.\n"
TOOLTIP_MD :: "[docs](https://example.com/docs) and more"
SURFACE_MD :: "> [!note]\n> Inside the surface.\n\nAfter it.\n"
CONFLICT :: "a\n<<<<<<< ours\nb\n=======\nc\n>>>>>>> theirs\nd\n"
FOLD_TEXT :: "top {\nhidden\n}\nlast\n"

block_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.block_rows += 1
	n := gk.row_context(cx, a.blocks, at, gk.Markdown_Node_Read)
	fmt.printf("editor2 block row=%d name=%s text=%q markdown=%q\n", at.row, n->name(cx), n->as_text(cx), n->as_markdown(cx))
	return gk.label(fmt.tprintf("[%s]", n->as_text(cx)))
}

inline_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.inline_rows += 1
	v := gk.row_context(cx, a.inlines, at, gk.Markdown_Inline)
	fmt.printf("editor2 inline row=%d name=%s text=%q font_ok=%v line_ok=%v rem_ok=%v\n", at.row, v.node->name(cx),
		v.node->as_text(cx), v.typography->font_size(cx) > 0, v.typography->line_height(cx) >= v.typography->font_size(cx),
		v.typography->rem_size(cx) > 0)
	return gk.label(fmt.tprintf("<%s>", v.node->as_text(cx)))
}

tip_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.tip_rows += 1
	url := gk.row_context(cx, a.tips, at, gk.Temp_String)
	fmt.printf("editor2 tooltip row=%d url=%s\n", at.row, url)
	return gk.label(fmt.tprintf("open %s", url))
}

fold_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.fold_rows += 1
	line := gk.row_context(cx, a.folds, at, int)
	fmt.printf("editor2 fold row=%d line=%d\n", at.row, line)
	return gk.label("...")
}

keyword_runs :: proc(a: ^App, cx: ^gk.Cx, q: gk.Highlight_Query) -> []gk.Highlight_Run {
	out := make([dynamic]gk.Highlight_Run, context.temp_allocator)
	if i := strings.index(string(q.text), "proc"); i >= 0 {
		append(&out, gk.Highlight_Run{gk.Range{i, i + 4}, "keyword"})
	}
	return out[:]
}

style_for :: proc(a: ^App, cx: ^gk.Cx, name: gk.Temp_String) -> Maybe(gk.Highlight_Style) {
	fmt.printf("editor2 style asked name=%s\n", name)
	if name == "keyword" {
		return gk.Highlight_Style{color = gk.theme(.Danger)}
	}
	return nil
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "decorations", "route":
		a.ed = gk.editor_state(cx)->default_value("hello world\n")->build()
	case "mask-variant":
		a.inp = gk.input_state(cx)->build()
		a.inp->set_mask_pattern(cx, gk.Mask_Pattern_Pattern{pattern = "999-999"})
	case "display-map":
		a.ed = gk.editor_state(cx)->default_value(FOLD_TEXT)->build()
	case "editor-style":
		a.styles = gk.ask(cx, a, style_for)
		langs := [?]string{"odin"}
		a.hl = gk.highlighter(cx, a, keyword_runs, languages = langs[:])
		a.ed = gk.editor_state(cx)->language("odin")->default_value("main :: proc() {}\n")->build()
		a.ed->set_highlighter_factory(cx, a.hl.src)
		a.ed->set_editor_style(cx, gk.Input_Editor_Style {
			foreground = gk.theme(.Foreground), muted_foreground = gk.theme(.Muted_Foreground),
			background = gk.theme(.Background), border = gk.theme(.Border), selection = gk.theme(.Selection),
			caret = gk.theme(.Caret), diagnostics = {gk.theme(.Danger), gk.theme(.Warning), gk.theme(.Info), gk.theme(.Info)},
			highlight_styles = a.styles,
		})
	case "plugin":
		a.blocks = gk.row_source(cx, a, 0, block_row, initial = 0)
		a.inlines = gk.row_source(cx, a, 0, inline_row, initial = 0)
	case "tooltip":
		a.tips = gk.row_source(cx, a, 0, tip_row, initial = 0)
	case "history":
		a.hist = gk.history(cx)->build()
		for e in ([]string{"a", "b", "c", "d"}) {
			a.hist->push(cx, e)
		}
	case "fold":
		a.folds = gk.row_source(cx, a, 0, fold_row, initial = 0)
		a.ed = gk.editor_state(cx)->default_value(FOLD_TEXT)->build()
		a.ed->set_folding(cx, true)
		a.ed->set_line_number(cx, true)
		a.ed->set_fold_placeholder_renderer(cx, a.folds)
		folds := [?]gk.Fold_Range{{start_line = 0, end_line = 2}}
		a.ed->apply_highlighter_fold_candidates(cx, folds[:])
	case "owner":
		a.ed = gk.editor_state(cx)->default_value("x")->build()
	case "bad-owner":
		a.hist = gk.history(cx)->build()
	case:
		init6(a, cx) // round6.odin
	}
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	body: gk.Element = gk.label("-")
	switch a.mode {
	case "plugin":
		ext := gk.markdown_extensions_default()->frontmatter()->plugin(gk.frontmatter_plugin())
		body = gk.div()->w(gk.px(420))->child(gk.base_text_view_markdown("md", PLUGIN_MD)->markdown_extensions(ext)->
			plugin(gk.Markdown_Plugin{name = "badge", match = .Code, lang = "badge", rows = a.blocks})->
			plugin(gk.Markdown_Plugin{name = "code", match = .Inline_Code, rows = a.inlines, baseline = 12}))
	case "tooltip":
		body = gk.div()->w(gk.px(420))->child(gk.base_text_view_markdown("md", TOOLTIP_MD)->link_tooltip(a.tips))
	case "surface":
		ext := gk.markdown_extensions_default()->block_surface("note",
			gk.div(.P_2)->bg(gk.theme(.Secondary))->child(gk.label("Note")))
		body = gk.div()->w(gk.px(420))->child(gk.base_text_view_markdown("md", SURFACE_MD)->markdown_extensions(ext))
	case "fold":
		body = gk.div()->w(gk.px(420))->child(gk.code_editor(a.ed, height = 120))
	case "decorations", "editor-style", "display-map", "route":
		body = gk.div()->w(gk.px(420))->h(gk.px(120))->child(gk.editor(a.ed))
	case "mask-variant":
		body = gk.div()->w(gk.px(220))->child(gk.input(a.inp))
	case "owner":
		body = gk.div()->w(gk.px(220))->child(gk.input_base("ib")->semantic_owner(gk.entity_id(a.ed))->accessibility_label("owned"))
		fmt.printf("editor2 owner drawn\n")
	case "bad-owner":
		body = gk.div()->w(gk.px(220))->child(gk.input_base("ib")->semantic_owner(gk.Entity_Id(a.hist.h)))
	case:
		if el, ok := view6(a, cx); ok {
			body = el // round6.odin
		}
	}
	return gk.v_flex(.Size_Full, .P_4, .Gap_2)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.h_flex(.Gap_2)->child(gk.button("step")->label("step")->on_click(gk.on(step))))->
		child(body)
}

step :: proc(a: ^App, cx: ^gk.Cx) {
	a.step += 1
	switch a.mode {
	case "plugin":
		parsed, _ := gk.test_editor_text(cx)
		fmt.printf("editor2 plugin parsed=%d blocks=%d inlines=%d\n", parsed, a.block_rows, a.inline_rows)
	case "tooltip":
		_, rows := gk.test_editor_text(cx)
		fmt.printf("editor2 tooltip view_rows=%d described=%d\n", rows, a.tip_rows)
	case "history":
		a.hist->retain(cx, []string{"b", "d", "x"})
		es := a.hist->entries(cx)
		cur, _ := a.hist->current(cx).?
		fmt.printf("editor2 history entries=%v current=%q\n", es, cur)
	case "conflict":
		f := gk.parse_conflicts(cx, "a.txt", CONFLICT)
		file, ok := f.value.?
		if !ok {
			fmt.printf("editor2 conflict parse failed: %s\n", f.err)
			return
		}
		cs := file->conflicts(cx)
		fmt.printf("editor2 conflict n=%d\n", len(cs))
		for c in cs {
			base, has_base := c->base_lines(cx).?
			bl, has_bl := c->base_label(cx).?
			fmt.printf("editor2 conflict lines=%v current=%v incoming=%v base=%v/%v current_label=%q incoming_label=%q base_label=%v/%q\n",
				c->lines(cx), c->current_lines(cx), c->incoming_lines(cx), has_base, base, c->current_label(cx),
				c->incoming_label(cx), has_bl, bl)
		}
	case "annotation":
		an := gk.diff_annotation_file(cx, "ann-1", "src/a.rs")
		fmt.printf("editor2 annotation id=%q path=%q\n", an->id(cx), an->path(cx))
	case "fold":
		fmt.printf("editor2 fold described=%d\n", a.fold_rows)
	case "display-map":
		folds := a.ed->display_map_folded_ranges(cx)
		fmt.printf("editor2 display-map lines=%d text=%q rows=%d folded=%v hidden=%v row1=%d folds=%d\n",
			a.ed->display_map_buffer_line_count(cx), a.ed->display_map_text(cx), a.ed->display_map_display_row_count(cx),
			a.ed->display_map_is_folded_at(cx, 0), a.ed->display_map_is_buffer_line_hidden(cx, 1),
			a.ed->display_map_buffer_line_to_display_row(cx, 1), len(folds))
	case "mask-variant":
		fmt.printf("editor2 mask-variant value=%q\n", a.inp->value(cx))
	case "route":
		act := gk.action(cx, "editor2.nothing")
		fmt.printf("editor2 route handled=%v\n", a.ed->route_overlay_action(cx, act))
	case "decorations":
		switch a.step {
		case 1:
			first := []gk.Text_Decoration{{range = {0, 5}, style = {color = gk.theme(.Danger), font_weight = gk.Font_Weight{700}}}}
			// a call's object is temporary (until the next frame): kept across the steps
			a.coll = gk.retain(cx, a.ed->create_decorations_collection(cx, first))
			fmt.printf("editor2 decorations made ranges=%v\n", a.coll->get_ranges(cx))
			more := []gk.Text_Decoration{{range = {6, 11}, style = {background_color = gk.theme(.Accent)}}}
			a.coll->append(cx, more)
		case 2:
			fmt.printf("editor2 decorations appended ranges=%v\n", a.coll->get_ranges(cx))
			a.coll->clear(cx)
		case 3:
			fmt.printf("editor2 decorations cleared ranges=%v\n", a.coll->get_ranges(cx))
		}
	case:
		step6(a, cx) // round6.odin
	}
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "editor2", {480, 360}, a.root)
}

// The step button at (36, 30); the body from (16, 56).
STEP :: "click 36 30\nframe\n"

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	s := strings.builder_make()
	strings.write_string(&s, "size 480 360\nframe\n")
	switch app.mode {
	case "plugin":
		strings.write_string(&s, "frame\nadvance 100\nframe\nframe\nframe\n")
		strings.write_string(&s, STEP)
	case "tooltip":
		// On the link "docs" (the first word of the text), then over it again: one row.
		strings.write_string(&s, "frame\nmove 30 66\nframe\nadvance 600\nframe\nframe\nmove 32 67\nframe\nframe\n")
		strings.write_string(&s, STEP)
	case "fold":
		// Clicks along the first line's gutter, each followed by a frame: the one on the fold icon
		// folds line 0 (a second one there unfolds it); a frame while it is folded draws the
		// placeholder, the row of the folded line.
		strings.write_string(&s, "frame\n")
		for y in ([]int{68, 74}) {
			for x := 16; x <= 96; x += 3 {
				fmt.sbprintf(&s, "move %d %d\nclick %d %d\nframe\n", x, y, x, y)
			}
		}
		strings.write_string(&s, STEP)
	case "history", "conflict", "annotation", "display-map", "route":
		strings.write_string(&s, "frame\n")
		strings.write_string(&s, STEP)
	case "mask-variant":
		strings.write_string(&s, "click 100 72\ntype 123456\nframe\n")
		strings.write_string(&s, STEP)
	case "decorations":
		strings.write_string(&s, STEP)
		strings.write_string(&s, STEP)
		strings.write_string(&s, STEP)
	case "editor-style":
		strings.write_string(&s, "frame\nadvance 100\nframe\nadvance 100\nframe\nframe\n")
	case "surface", "owner", "bad-owner":
		strings.write_string(&s, "frame\nframe\n")
	case:
		if more, ok := script6(app.mode); ok {
			strings.write_string(&s, more) // round6.odin
		}
	}
	strings.write_string(&s, "frame\n")
	gk.run_headless(&app, strings.to_string(s), start)
	fmt.printf("editor2 done mode=%s\n", app.mode)
}
