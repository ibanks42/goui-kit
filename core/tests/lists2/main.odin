// lists2: the protocol tests of the lists and editor families' searchable lists and table delegate
// data (docs/PROTOCOLS.md sections 9.9 and 9.10): the kit's Select and Combobox over the
// library's search delegate (items as data, rows, header rows, search requests), and a table's
// texts, row labels and looks. Offscreen under a gk-meter script (the diag library). Each mode
// prints lines core/tests/run-lists2.sh checks, with the exit code.
//
//   GK_PROTO=select | combobox | search | search-requests | rows | table | bad-source | bad-headers | released
//            | mask | validate | text | display-map | providers | expr | bad-expr
//            | drive | kept | status-colors | ask | diff   (docs/PROTOCOLS.md 9.13 and 9.14)
package lists2

import "core:fmt"
import "core:os"
import "core:strings"
import gk "gpuikit:gk"

FRUIT := [?]gk.Search_Item {
	{value = "apple", title = "Apple"},
	{value = "banana", title = "Banana", keywords = "yellow"},
	{value = "cherry", title = "Cherry"},
	{value = "grape", title = "Grape", disabled = true},
	{value = "mango", title = "Mango", display = "Mango (ripe)"},
}

VEG := [?]gk.Search_Item{{value = "kale", title = "Kale"}, {value = "leek", title = "Leek"}}

App :: struct {
	mode:     string,
	root:     gk.View,
	src:      gk.Row_Source,
	sel:      gk.Select_State,
	combo:    gk.Combobox_State,
	table:    gk.Table_State,
	cells:    gk.Row_Source,
	group:    gk.View,
	sub:      gk.Subscription,
	step:     int,
	events:   int,
	rows:     map[u64]int,
	requests: int,
	in1, in2: gk.Input_State,
	checks:   int, // validator calls
	ed:       gk.Editor_State,
	prov:     gk.Row_Source, // the editor's providers' requests
	asked:    map[string]int,
	// drive: a list, a table and a tree selected and scrolled from code
	list:      gk.List_State,
	tree:      gk.Tree_State,
	tree_rows: gk.Row_Source,
	drawn:     map[string]int, // "list 500", "table 700", "tree 150": rows described
	kept0:     int,
	// ask: data closures answered between frames
	hl:        gk.Ask(gk.Code_Block, []gk.Tuple_Range_Highlight_Style),
	img:       gk.Ask(gk.Temp_String, string),
	hl_calls:  int,
	img_calls: int,
	r4:        R4, // round4.odin
}

key :: proc(section, row: int) -> u64 {
	return u64(section) << 32 | u64(u32(row))
}

// --- events -------------------------------------------------------------------------------------

select_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Select_Event) {
	a.events += 1
	v, has := e.v0.?
	now, ok := a.sel->selected_value(cx).?
	fmt.printf("lists2 select confirm has=%v value=%q now=%q same=%v\n", has, v, now, ok && string(now) == string(v))
}

combo_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Combobox_Event) {
	a.events += 1
	switch v in e {
	case gk.Combobox_Event_Change:
		fmt.printf("lists2 combobox change values=%v\n", v.v0)
	case gk.Combobox_Event_Confirm:
		fmt.printf("lists2 combobox confirm values=%v\n", v.v0)
	}
}

// search-requests: the app answers a query with items of its own.
search_request :: proc(a: ^App, cx: ^gk.Cx, e: gk.Search_Request) {
	a.requests += 1
	fmt.printf("lists2 search request id=%d query=%q\n", e.id, e.query)
	found := [?]gk.Search_Item{gk.search_item("xenon", "Xenon"), gk.search_item("xylophone", "Xylophone")}
	gk.search_items(cx, a.src, found[:])
	a.sel->set_items(cx, a.src)
	gk.answer(cx, a.src, e.id, true)
}

// rows: an item's row and a group's header row, each a view described once.
item_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	a.rows[key(at.section, at.row)] += 1
	if at.row == gk.HEADER {
		fmt.printf("lists2 header group=%d\n", at.section)
		return gk.label(at.section == 0 ? "== fruit ==" : "== veg ==")
	}
	items := at.section == 0 ? FRUIT[:] : VEG[:]
	fmt.printf("lists2 row group=%d row=%d value=%s\n", at.section, at.row, items[at.row].value)
	return gk.label(fmt.tprintf("> %s", items[at.row].title))
}

// drive: rows that say which of them were described (drawn after a scroll from code).
drive_list_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	a.drawn[fmt.aprintf("list %d", at.row)] += 1
	return gk.list_item(gk.id("item", at.row))->child(gk.label(fmt.tprintf("item %d", at.row)))
}

drive_cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	a.drawn[fmt.aprintf("table %d", at.row)] += 1
	return gk.label(fmt.tprintf("%d:%d", at.row, at.col))
}

drive_tree_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	a.drawn[fmt.aprintf("tree %d", at.row)] += 1
	e, _ := gk.tree_entry(cx, a.tree, at.row)
	return gk.list_item(gk.id("tree", at.row))->child(gk.label(string(e.label)))
}

drawn :: proc(a: ^App, what: string, row: int) -> bool {
	return a.drawn[fmt.tprintf("%s %d", what, row)] > 0
}

// ask: the code block's highlights ("proc" in red) and the image's path, each asked once.
ask_highlight :: proc(a: ^App, cx: ^gk.Cx, b: gk.Code_Block) -> []gk.Tuple_Range_Highlight_Style {
	a.hl_calls += 1
	lang, _ := b.lang.?
	fmt.printf("lists2 ask highlight lang=%s code=%q\n", lang, b.code)
	out := make([]gk.Tuple_Range_Highlight_Style, 1, context.temp_allocator)
	out[0] = {gk.Range{0, 4}, gk.Highlight_Style{color = gk.hsla(0, 0.7, 0.5, 1)}}
	return out
}

ask_image :: proc(a: ^App, cx: ^gk.Cx, uri: gk.Temp_String) -> string {
	a.img_calls += 1
	fmt.printf("lists2 ask image uri=%s\n", uri)
	return "/nonexistent/pic.png"
}

// --- steps (the step button) ------------------------------------------------------------------

step :: proc(a: ^App, cx: ^gk.Cx) {
	a.step += 1
	if r4_modes(a.mode) {
		r4_step(a, cx)
		return
	}
	switch a.mode {
	case "select":
		switch a.step {
		case 1:
			a.sel->set_selected_value(cx, "cherry")
		case 2:
			v, ok := a.sel->selected_value(cx).?
			ix, iok := a.sel->selected_index(cx).?
			fmt.printf("lists2 select set_selected_value value=%q ok=%v row=%d index_ok=%v\n", v, ok, ix.row, iok)
			a.sel->set_selected_index(cx, 4)
		case 3:
			v, _ := a.sel->selected_value(cx).?
			fmt.printf("lists2 select set_selected_index value=%q\n", v)
			// New items: mango stays (by value), the others go.
			items := [?]gk.Search_Item{gk.search_item("mango", "Mango"), gk.search_item("plum", "Plum")}
			gk.search_items(cx, a.src, items[:])
			a.sel->set_items(cx, a.src)
		case 4:
			v, ok := a.sel->selected_value(cx).?
			ix, _ := a.sel->selected_index(cx).?
			fmt.printf("lists2 select set_items value=%q ok=%v row=%d\n", v, ok, ix.row)
			a.sel->set_selected_index(cx, nil)
		case 5:
			_, ok := a.sel->selected_value(cx).?
			fmt.printf("lists2 select cleared has=%v\n", ok)
		}
	case "will-change":
		fmt.printf("lists2 will-change values=%v\n", a.combo->selected_values(cx))
	case "combobox":
		switch a.step {
		case 1:
			vs := a.combo->selected_values(cx)
			sel := a.combo->selection(cx)
			fmt.printf("lists2 combobox initial values=%v n=%d first=%s row=%d\n", vs, len(sel), len(sel) > 0 ? sel[0].v1.title : "", len(sel) > 0 ? sel[0].v0.row : -1)
			added := a.combo->add_selected_index(cx, 0)
			again := a.combo->add_selected_index(cx, 0)
			fmt.printf("lists2 combobox add first=%v again=%v values=%v\n", added, again, a.combo->selected_values(cx))
			removed := a.combo->remove_selected_index(cx, gk.Base_Index_Path{row = 1})
			fmt.printf("lists2 combobox remove=%v values=%v\n", removed, a.combo->selected_values(cx))
			vals := [?]string{"apple", "mango"}
			a.combo->set_selected_values(cx, vals[:])
			a.combo->set_query(cx, "an")
		case 2:
			fmt.printf("lists2 combobox set values=%v query=%q\n", a.combo->selected_values(cx), a.combo->query(cx))
			a.combo->clear_selection(cx)
		case 3:
			_, has := a.combo->selected_value(cx).?
			fmt.printf("lists2 combobox cleared n=%d has=%v\n", len(a.combo->selected_values(cx)), has)
		}
	case "table":
		switch a.step {
		case 1:
			texts := [?][]string{{"0", "zero", "a0"}, {"1", "one", "a1"}}
			gk.table_texts(cx, a.table, 0, texts[:])
			labels := [?]gk.Table_Row_Label{{row = 1, label = "the first row"}}
			gk.table_row_labels(cx, a.table, labels[:])
			fits := [?]gk.Table_Fit{{col = 1, width = 150}}
			level := [?]gk.Table_Group{{label = "Key", span = 1}, {label = "Values", span = 2}}
			groups := [?][]gk.Table_Group{level[:]}
			views := [?]gk.Table_Group_View{{label = "Values", view = a.group}}
			gk.table_look(cx, a.table, {
				fit = fits[:],
				groups = groups[:],
				group_views = views[:],
				header_style = gk.style_refinement_default()->bg(gk.theme(.Secondary)),
				row_style = gk.style_refinement_default()->text_color(gk.theme(.Muted_Foreground)),
			})
		case 2:
			d := a.table->dump_range(cx, gk.Range{0, 3})
			fmt.printf("lists2 table dump headers=%v rows=%v\n", d.v0, d.v1)
			fit := a.table->fit_col(cx, 1)
			nofit := a.table->fit_col(cx, 2)
			back := a.table->delegate(cx)
			fmt.printf("lists2 table fit=%v nofit=%v delegate_is_source=%v\n", fit, nofit, back.h == a.cells.h)
		}
	case "released":
		gk.release(cx, a.src)
		fmt.printf("lists2 released source\n")
	case "drive":
		switch a.step {
		case 1:
			// Select and scroll each from code (generated state calls).
			fmt.printf("lists2 drive before list=%v table=%v tree=%v\n", drawn(a, "list", 500), drawn(a, "table", 700), drawn(a, "tree", 150))
			a.list->set_selected_index(cx, 500)
			a.list->scroll_to_item(cx, 500, .Top)
			a.table->set_selected_row(cx, 700)
			a.table->scroll_to_row(cx, 700)
			a.tree->set_selected_index(cx, 150)
			a.tree->scroll_to_item(cx, 150, .Top)
		case 2:
			li, lok := a.list->selected_index(cx).?
			tr, tok := a.table->selected_row(cx).?
			ti, iok := a.tree->selected_index(cx).?
			fmt.printf("lists2 drive selected list=%d table=%d tree=%d ok=%v\n", li.row, tr, ti, lok && tok && iok)
			fmt.printf("lists2 drive scrolled list=%v table=%v tree=%v\n", drawn(a, "list", 500), drawn(a, "table", 700), drawn(a, "tree", 150))
			// The list's scroll handle (an object a read hands out, temporary).
			a.list->scroll_handle(cx)->scroll_to_item(cx, 900, .Top)
			a.table->scroll_to_row(cx, 0)
		case 3:
			fmt.printf("lists2 drive handle list900=%v table0=%v\n", drawn(a, "list", 900), drawn(a, "table", 0))
		}
	case "kept", "kept-gone":
		switch a.step {
		case 1:
			// Handlers given to calls (a builder's validate, set_validator) belong to the state.
			a.in2->set_validator(cx, gk.on(no_x))
			a.in2->set_validator(cx, gk.on(no_x))
		case 2:
			a.mode = "kept-gone" // the view stops showing the inputs before they are released
		case 3:
			n := gk.test_kept_count()
			gk.release(cx, a.in1)
			gk.release(cx, a.in2)
			fmt.printf("lists2 kept added=%d dropped=%v\n", n - a.kept0, gk.test_kept_count() == a.kept0)
		}
	case "ask":
		gets, asked, answered := gk.test_asks(cx)
		fmt.printf("lists2 ask counts asked=%d answered=%d highlight=%d image=%d gets_more=%v\n", asked, answered, a.hl_calls, a.img_calls, gets > asked)
	case "diff":
		// A patch parsed by the kit: the count, each file by its index, a diff state over them;
		// a patch that does not parse gives the kit's error.
		n := gk.diff_file_parse_count(cx, DIFF_PATCH)
		count, _ := n.value.?
		bad := gk.diff_file_parse_count(cx, "@@ nonsense\n")
		fmt.printf("lists2 diff count=%d ok=%v bad_ok=%v bad_err=%q\n", count, n.ok, bad.ok, bad.err)
		files := make([]gk.Diff_File, count, context.temp_allocator)
		for &f, i in files {
			f = gk.diff_file_parse_at(DIFF_PATCH, i)
		}
		st := gk.diff_state(cx, files)->build()
		gk.release(cx, st)
		fmt.printf("lists2 diff state made\n")
	case "status-colors":
		// A library object with no constructor but Default (PROTOCOLS.md 9.13): its reads.
		sc := gk.status_colors_default(cx)
		e := sc->error(cx)
		fmt.printf("lists2 status error=%v warning=%v\n", e, sc->warning(cx))
		fmt.printf("lists2 status error_opaque=%v error_not_warning=%v created_is_success=%v deleted_is_error=%v border_is_error=%v\n",
			e.a == 1, e != sc->warning(cx), sc->created(cx) == sc->success(cx), sc->deleted(cx) == e, sc->error_border(cx) == e)
		syn := gk.syntax_colors_default(cx)
		_, has := syn->style(cx, "keyword").?
		_, has0 := syn->style_for_index(cx, 0).?
		fmt.printf("lists2 status syntax keyword=%v index0=%v\n", has, has0)
		gk.release(cx, sc)
		gk.release(cx, syn)
	case "mask":
		switch a.step {
		case 1:
			// A mask the binding could not set before: the Number variant (a char separator).
			a.in2->set_mask_pattern(cx, gk.Mask_Pattern_Number{separator = ',', fraction = 2})
		case 2:
			fmt.printf("lists2 mask pattern=%q number=%q\n", a.in1->value(cx), a.in2->value(cx))
		}
	case "validate":
		fmt.printf("lists2 validate value=%q calls=%d\n", a.in1->value(cx), a.checks)
	case "text":
		a.in1->set_selected_range(cx, gk.Range{6, 11})
		fmt.printf("lists2 text text=%q selected=%q\n", a.in1->text(cx), a.in1->selected_text(cx))
	case "expr":
		fmt.printf("lists2 expr step_by=%q by_value=%q\n", a.in1->value(cx), a.in2->value(cx))
	case "display-map":
		dm := gk.display_map(cx, gk.font("Inter"), 14, nil)
		folds := [?]gk.Fold_Range{}
		dm->set_fold_candidates(cx, folds[:])
		dm->set_font(cx, gk.font("Inter", 700), 16)
		fmt.printf("lists2 display-map lines=%d rows=%d wraps=%d text=%q folded=%v candidate=%v\n",
			dm->buffer_line_count(cx), dm->display_row_count(cx), dm->wrap_row_count(cx), dm->text(cx),
			dm->is_folded_at(cx, 0), dm->is_fold_candidate(cx, 0))
		gk.release(cx, dm)
	}
}

// providers: every question of the editor's providers, answered at once.
provider_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Provider_Event) {
	name := ""
	switch q in e.request {
	case gk.Provider_Completions:
		name = "completions"
		items := [?]gk.Completion_Item{{label = "alpha"}, {label = "beta"}}
		gk.answer(cx, a.prov, e.id, gk.Completion_Response{items = items[:]})
	case gk.Provider_Hover:
		name = "hover"
		gk.answer(cx, a.prov, e.id, gk.Hover_Response{})
	case gk.Provider_Definitions:
		name = "definitions"
		defs := [?]gk.Definition{{uri = "file:///a.odin", target = {0, 0, 0, 5}, selection = {0, 0, 0, 5}}}
		gk.answer(cx, a.prov, e.id, defs[:])
	case gk.Provider_Colors:
		name = "colors"
		cols := [?]gk.Document_Color{{range = {0, 0, 0, 1}, rgba = {1, 0, 0, 1}}}
		gk.answer(cx, a.prov, e.id, cols[:])
	case gk.Provider_Code_Actions:
		name = "code-actions"
		acts := [?]gk.Code_Action{{title = "Fix it", kind = "quickfix", id = "fix-1"}}
		gk.answer(cx, a.prov, e.id, acts[:])
	case gk.Provider_Perform:
		name = fmt.tprintf("perform id=%s", q.id)
		gk.answer(cx, a.prov, e.id, true)
	case gk.Provider_Tokens:
		name = "tokens"
		toks := [?]u32{0, 0, 1, 0, 0}
		gk.answer(cx, a.prov, e.id, toks[:])
	case gk.Provider_Inline:
		name = "inline"
		gk.answer(cx, a.prov, e.id, Maybe(string)("lpha"))
	}
	a.asked[strings.clone(name)] += 1
	fmt.printf("lists2 provider %s id=%d\n", name, e.id)
}

// validate: refuses any text with an x in it (a handler kept by the state's builder).
no_x :: proc(a: ^App, cx: ^gk.Cx, text: gk.Temp_String) -> bool {
	a.checks += 1
	return !strings.contains(string(text), "x")
}


group_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	fmt.printf("lists2 group view described\n")
	return gk.label("[values]")
}

cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	return gk.label(fmt.tprintf("%d:%d", at.row, at.col))
}

COLUMNS := [?]gk.Table_Column {
	{key = "id", name = "Id", width = 60, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
	{key = "name", name = "Name", width = 100, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
	{key = "a", name = "A", width = 60, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 400},
}

// --- the views ----------------------------------------------------------------------------------

init :: proc(a: ^App, cx: ^gk.Cx) {
	if r4_modes(a.mode) {
		r4_init(a, cx)
		return
	}
	switch a.mode {
	case "select":
		a.src = gk.search_source(cx, a, FRUIT[:])
		a.sel = gk.select_state(cx, a.src, nil)->build()
		a.sub = a.sel->on_select_event(cx, gk.on(select_event))
	case "search":
		a.src = gk.search_source(cx, a, FRUIT[:])
		a.sel = gk.select_state(cx, a.src, nil)->searchable(true)->build()
		a.sub = a.sel->on_select_event(cx, gk.on(select_event))
	case "search-requests":
		a.src = gk.search_source(cx, a, FRUIT[:], requests = gk.on(search_request))
		a.sel = gk.select_state(cx, a.src, nil)->searchable(true)->build()
		a.sub = a.sel->on_select_event(cx, gk.on(select_event))
	case "rows", "released":
		groups := [?]gk.Search_Group{{title = "Fruit", items = FRUIT[:]}, {title = "Veg", items = VEG[:]}}
		a.src = gk.search_source_groups(cx, a, groups[:], item_row, headers = true)
		a.sel = gk.select_state(cx, a.src, nil)->build()
		a.sub = a.sel->on_select_event(cx, gk.on(select_event))
	case "combobox":
		a.src = gk.search_source(cx, a, FRUIT[:])
		a.combo = gk.combobox_state(cx, a.src, []int{1})->multiple(true)->build()
		a.sub = a.combo->on_combobox_event(cx, gk.on(combo_event))
	case "will-change":
		// The combobox mode's start, with rules (PROTOCOLS.md 11.25): Cherry, which Down and Enter add
		// there, is blocked here; Banana is locked.
		a.src = gk.search_source(cx, a, FRUIT[:])
		start := [?]gk.Base_Index_Path{{row = 1}}
		a.combo = gk.combobox_state(cx, a.src, start[:])->multiple(true)->build()
		a.sub = a.combo->on_combobox_event(cx, gk.on(combo_event))
		gk.search_will_change(cx, a.src, gk.Search_Rules{locked = []string{"banana"}, blocked = []string{"cherry"}})
	case "table":
		a.cells = gk.table_source(cx, a, 20, COLUMNS[:], cell, initial_rows = 8)
		a.table = gk.table_state(cx, a.cells)->build()
		a.group = gk.view(a, nil, group_view)
	case "bad-source":
		other := gk.row_source(cx, a, 3, proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {return gk.label("x")})
		gk.search_items(cx, other, FRUIT[:])
	case "bad-headers":
		groups := [?]gk.Search_Group{{title = "Fruit", items = FRUIT[:]}}
		a.src = gk.search_source_groups(cx, a, groups[:], headers = true)
	case "mask":
		a.in1 = gk.input_state(cx)->mask_pattern("999-999")->build()
		a.in2 = gk.input_state(cx)->build()
	case "validate":
		a.in1 = gk.input_state(cx)->validate(gk.on(no_x))->build()
	case "ask":
		a.hl = gk.ask(cx, a, ask_highlight)
		a.img = gk.ask(cx, a, ask_image)
	case "kept":
		a.kept0 = gk.test_kept_count()
		a.in1 = gk.input_state(cx)->validate(gk.on(no_x))->build()
		a.in2 = gk.input_state(cx)->build()
	case "drive":
		a.src = gk.list_source(cx, a, 1000, drive_list_row, initial = 20)
		a.list = gk.list_state(cx, a.src)->build()
		a.cells = gk.table_source(cx, a, 1000, COLUMNS[:], drive_cell, initial_rows = 12)
		a.table = gk.table_state(cx, a.cells)->build()
		a.tree = gk.tree_state(cx)->build()
		nodes := make([]gk.Tree_Node, 300, context.temp_allocator)
		for &n, i in nodes {
			n = {id = fmt.tprintf("n%d", i), label = fmt.tprintf("node %d", i)}
		}
		gk.tree_set_items(cx, a.tree, nodes)
		a.tree_rows = gk.tree_source(cx, a, drive_tree_row, initial = 8)
	case "text":
		a.in1 = gk.input_state(cx)->default_value("hello world")->build()
	case "expr":
		// Steps of 1 below 10 and of 10 from there: an expression the library evaluates.
		a.in1 = gk.input_state(cx)->default_value("9")->step_by("x < 10 ? 1 : 10")->build()
		// The same as NumberStep's ByValue variant, which the binding could not give before.
		a.in2 = gk.input_state(cx)->default_value("3")->build()
		a.in2->set_step(cx, gk.Number_Step_By_Value{"y == 1 ? x : 0.5"}) // Increment doubles, Decrement takes 0.5
	case "bad-expr":
		a.in1 = gk.input_state(cx)->step_by("x < 10 ? 1")->build()
	case "providers":
		a.ed = gk.editor_state(cx)->language("rust")->build()
		types := [?]string{"keyword"}
		a.prov = gk.editor_providers(cx, a.ed, a, provider_event, triggers = ".",
			which = {.Completions, .Hover, .Definitions, .Document_Colors, .Code_Actions, .Semantic_Tokens, .Inline_Completions},
			code_action_id = "lists2", inline_debounce_ms = 50, token_types = types[:])
	}
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	body: gk.Element
	switch a.mode {
	case "ctx-token", "lend", "highlight", "languages", "bad-lend", "bad-ctx", "small":
		body = r4_view(a, cx)
	case "combobox", "will-change":
		body = gk.combobox(a.combo)->placeholder("Fruit")->w(gk.px(220))
	case "table":
		body = gk.div()->w(gk.px(440))->h(gk.px(200))->child(gk.data_table(a.table))
	case "bad-source", "bad-headers", "display-map", "status-colors", "diff", "kept-gone":
		body = gk.label("-")
	case "ask":
		body = gk.div()->w(gk.px(400))->child(
			gk.base_text_view_markdown("md", "```odin\nproc x\n```\n\n![pic](pic.png)\n")->
			code_block_highlighter(a.hl)->image_source(a.img))
	case "drive":
		body = gk.h_flex(.Gap_2)->
			child(gk.div()->w(gk.px(120))->h(gk.px(200))->child(gk.list(a.list, .Size_Full)))->
			child(gk.div()->w(gk.px(200))->h(gk.px(200))->child(gk.data_table(a.table)))->
			child(gk.div()->w(gk.px(120))->h(gk.px(200))->child(gk.tree(a.tree, a.tree_rows, .Size_Full)))
	case "providers":
		body = gk.div()->w(gk.px(400))->child(gk.code_editor(a.ed, height = 120))
	case "expr":
		body = gk.v_flex(.Gap_2)->w(gk.px(200))->child(gk.number_input(a.in1))->child(gk.number_input(a.in2))
	case "bad-expr":
		body = gk.label("-")
	case "mask", "validate", "text", "kept":
		body = gk.v_flex(.Gap_2)->w(gk.px(220))->child(gk.input(a.in1))
		if a.mode == "mask" {
			body = gk.v_flex(.Gap_2)->w(gk.px(220))->child(gk.input(a.in1))->child(gk.input(a.in2))
		}
	case:
		body = gk.select(a.sel)->placeholder("Fruit")->w(gk.px(220))
	}
	return gk.v_flex(.Size_Full, .P_4, .Gap_2)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.h_flex(.Gap_2)->child(gk.button("step")->label("step")->on_click(gk.on(step))))->
		child(body)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, root_view)
	gk.open_window(cx, "lists2", {480, 360}, a.root)
}

// Positions: the step button at (36, 30); the select or combobox trigger from (16, 56), 220 wide.
STEP :: "click 36 30\nframe\n"
OPEN :: "click 100 72\nframe\nframe\n"

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	s := strings.builder_make()
	strings.write_string(&s, "size 480 360\nframe\n")
	switch app.mode {
	case "select":
		// Open the menu, Down to the first item, Enter confirms it; then the generated writes.
		strings.write_string(&s, OPEN)
		strings.write_string(&s, "key down\nkey enter\nframe\n")
		for _ in 0 ..< 5 {
			strings.write_string(&s, STEP)
		}
	case "search":
		// Searchable: the query input has the focus; "an" leaves Banana and Mango, the first one
		// under the cursor; Down and Enter take Mango.
		strings.write_string(&s, OPEN)
		strings.write_string(&s, "type an\nframe\nframe\nkey down\nkey enter\nframe\n")
	case "search-requests":
		strings.write_string(&s, OPEN)
		strings.write_string(&s, "type x\nsleep 100\nframe\nframe\nframe\nkey down\nkey enter\nframe\n")
	case "rows":
		strings.write_string(&s, OPEN)
		strings.write_string(&s, "frame\nkey down\nkey enter\nframe\n")
	case "combobox":
		strings.write_string(&s, OPEN)
		strings.write_string(&s, "key down\nkey enter\nframe\nkey escape\nframe\n")
		for _ in 0 ..< 3 {
			strings.write_string(&s, STEP)
		}
	case "will-change":
		strings.write_string(&s, OPEN)
		strings.write_string(&s, "key down\nkey enter\nframe\nkey escape\nframe\n")
		strings.write_string(&s, STEP)
	case "table":
		strings.write_string(&s, STEP)
		strings.write_string(&s, "frame\n")
		strings.write_string(&s, STEP)
	case "mask":
		strings.write_string(&s, "click 100 72\ntype 123456\nframe\n")
		strings.write_string(&s, STEP)
		strings.write_string(&s, "click 100 112\ntype 1234567\nframe\n")
		strings.write_string(&s, STEP)
	case "validate":
		strings.write_string(&s, "click 100 72\ntype abxc\nframe\n")
		strings.write_string(&s, STEP)
	case "text", "display-map", "status-colors", "diff":
		strings.write_string(&s, STEP)
	case "kept":
		strings.write_string(&s, STEP)
		strings.write_string(&s, STEP)
		strings.write_string(&s, STEP)
	case "ask":
		strings.write_string(&s, "frame\nadvance 100\nframe\nadvance 100\nframe\nframe\n")
		strings.write_string(&s, STEP)
	case "drive":
		for _ in 0 ..< 3 {
			strings.write_string(&s, STEP)
			strings.write_string(&s, "frame\nframe\nframe\n")
		}
	case "expr":
		// The increment buttons of both inputs, twice each.
		strings.write_string(&s, "click 183 72\nframe\nclick 183 72\nframe\nclick 183 112\nframe\nclick 183 112\nframe\n")
		strings.write_string(&s, STEP)
	case "providers":
		// Type, a trigger character, wait for the debounces, Enter takes the completion, ctrl-.
		// asks for code actions and Enter performs the first.
		strings.write_string(&s, "click 200 100\ntype a\nframe\nadvance 500\nframe\ntype .\nframe\nframe\nadvance 500\nframe\nframe\n")
		strings.write_string(&s, "key enter\nframe\nkey ctrl-.\nframe\nframe\nadvance 200\nframe\nkey enter\nframe\nframe\nadvance 500\nframe\n")
	case "ctx-token", "lend", "highlight", "languages", "bad-lend", "bad-ctx", "small":
		r4_script(&app, &s)
	case "released":
		// The source released, then the menu drawn: its rows cannot be drawn (exit 2).
		strings.write_string(&s, STEP)
		strings.write_string(&s, OPEN)
	}
	strings.write_string(&s, "frame\n")
	gk.run_headless(&app, strings.to_string(s), start)
	again := 0
	for _, n in app.rows {
		if n > 1 {
			again += 1
		}
	}
	if app.mode == "providers" {
		fmt.printf("lists2 providers asked=%v value=%q\n", app.asked, "")
	}
	fmt.printf("lists2 done mode=%s events=%d requests=%d rows=%d again=%d\n", app.mode, app.events, app.requests, len(app.rows), again)
}

DIFF_PATCH :: "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+2\n--- a/b.txt\n+++ b/b.txt\n@@ -1 +1 @@\n-x\n+y\n"
