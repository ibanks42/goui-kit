package gk

// The rows family of the delegate protocol (docs/PROTOCOLS.md section 9.5): the kit's list,
// table and tree over row sources (rt_source.odin). Each row (each cell of a table) is a view
// of its own, described once by the app's row procedure and retained; the kit asks the library
// for rows while it draws, and the library never calls the app then: a row without a view is a
// placeholder for one frame and is described before the next.
//
// The states and elements are generated (gen_state.odin, gen_component_list.odin, ...): the
// app makes a row source here, then the kit's state over it, and draws the kit's element:
//
//   src := gk.list_source(cx, a, len(a.items), item_row, gk.on(list_event))   init or a handler
//   a.list = gk.list_state(cx, src)->searchable(true)->build()
//   item_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
//       return gk.list_item(gk.id("item", at.row))->child(gk.label(a.items[at.row]))
//   }
//   view: gk.div()->h(gk.px(300))->child(gk.list(a.list))
//
//   src := gk.table_source(cx, a, 100_000, columns, cell, gk.on(table_event))
//   a.table = gk.table_state(cx, src)->build()         view: gk.data_table(a.table)
//   cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {..}    at.col: the column's index in `columns`
//
//   a.tree = gk.tree_state(cx)->build()    gk.tree_set_items(cx, a.tree, nodes)
//   rows := gk.tree_source(cx, a, tree_row)    view: gk.tree(a.tree, rows)
//   tree_row reads its entry with gk.tree_entry(cx, a.tree, at.row) and is described again when the tree changes.
//
// What the kit reports through the delegate (a selection, a confirm, a sort) arrives as one
// record per family through the handler given to the source, after the kit's own update; what
// it asks (a search) is a request the app answers with gk.answer. The states' own events are
// the generated subscriptions (on_list_event, on_table_event).

import "base:runtime"

// HEADER and FOOTER are the Row_At.row of a list section's header and footer rows (drawn when
// the list's Delegate_Data asks for them).
HEADER :: 0xffff_fffe
FOOTER :: 0xffff_ffff

// --- events ----------------------------------------------------------------------------------

// List_Index is a row of a list: its section and row (column is always 0 for a list).
List_Index :: struct {
	section, row, column: int,
}

List_Delegate_Event_Kind :: enum u16 {
	Select, // the selected row changed (index: none when cleared)
	Right_Click, // a row was right-clicked (index: none when none is)
	Confirm, // a click on a row or Enter: the selected row is confirmed (secondary: with the platform key)
	Cancel, // Escape
	Load_More, // scrolled near the end while Delegate_Data.has_more is set
	Search, // a request: the query changed (searchable lists). Set the new counts, then gk.answer(cx, src, e.id, true)
}

// List_Delegate_Event is the event record of a list's row source (shape LIST_EVENT).
List_Delegate_Event :: struct {
	id:        u64, // a request's id (.Search), 0 otherwise
	kind:      List_Delegate_Event_Kind,
	index:     Maybe(List_Index),
	secondary: bool,
	query:     Temp_String,
}

Table_Delegate_Event_Kind :: enum u16 {
	Sort, // a sortable column's sort icon was clicked: col, sort (sort the rows, then gk.table_invalidate)
	Move_Column, // the user dragged column col to position to
	Visible_Rows, // rows row ..< to are visible (only with Delegate_Data.visible_events)
	Visible_Columns, // columns col ..< to are visible (only with Delegate_Data.visible_events)
	Load_More, // scrolled near the end while Delegate_Data.has_more is set
}

// Table_Delegate_Event is the event record of a table's row source (shape TABLE_EVENT). A
// column is named by its index in the columns given to the table, which stays when the user
// moves it. Selections, double clicks and widths are the table state's own events
// (the generated on_table_event).
Table_Delegate_Event :: struct {
	id:   u64,
	kind: Table_Delegate_Event_Kind,
	row:  int,
	col:  int,
	to:   int,
	sort: Column_Sort,
}

@(init, private)
rows_shapes :: proc "contextless" () {
	register_shape(SHAPE_LIST_EVENT, List_Delegate_Event, nil)
	register_shape(SHAPE_TABLE_EVENT, Table_Delegate_Event, nil)
}

// Delegate_Data is what a list or table shows or reports besides its rows. Views are {0} for
// the kit's own (the empty inbox icon, the skeleton rows).
Delegate_Data :: struct {
	loading:            bool, // show the loading view instead of the rows
	has_more:           bool, // send Load_More when the user scrolls within `threshold` rows of the end
	visible_events:     bool, // tables: send Visible_Rows and Visible_Columns (one event per change while scrolling)
	headers:            bool, // lists: draw each section's header row (Row_At.row == gk.HEADER)
	footers:            bool, // lists: draw each section's footer row (Row_At.row == gk.FOOTER)
	threshold:          int,
	empty:              View,
	loading_view:       View,
	initial:            View, // lists: shown before the first search
	placeholder_height: f32, // lists: the height of a row drawn as a placeholder for one frame
}

default_delegate_data :: proc() -> Delegate_Data {
	return {threshold = 20}
}

// delegate_data sets what a list or table shows besides its rows. A buffered write.
delegate_data :: proc(cx: ^Cx, s: $T, d: Delegate_Data, loc := #caller_location) where T == List_State || T == Table_State {
	t := check_cx(cx, loc)
	if d.threshold < 0 {
		stop(loc, "gk.delegate_data was given a negative threshold (%d)", d.threshold)
	}
	flags: u8
	if d.loading {flags |= 1}
	if d.has_more {flags |= 2}
	if d.visible_events {flags |= 4}
	if d.headers {flags |= 8}
	if d.footers {flags |= 16}
	req := make(Tape, 0, 48, context.temp_allocator)
	put_u16(&req, C_ROWS_DATA)
	put_u64(&req, s.h)
	put_u8(&req, flags)
	put_u64(&req, u64(d.threshold))
	put_u64(&req, d.empty.h)
	put_u64(&req, d.loading_view.h)
	put_u64(&req, d.initial.h)
	put_f32(&req, d.placeholder_height)
	defer_op(t, req[:], loc)
}

// --- lists -----------------------------------------------------------------------------------

// list_source makes the row source of a list: `count` rows (one section; gk.set_count with a
// section adds more), each described by `row` as a gk.list_item, which the list marks
// selected. `events` receives List_Delegate_Event records. Make the list's state over it with
// gk.list_state(cx, src)->build(). Needs a window: call it in init or in a handler.
// `count` may be a procedure that counts the rows (task_count :: proc(a: ^App) -> int): then
// gk.refresh(cx) after a change of the data is all the list needs, with no gk.set_count.
list_source :: proc {
	list_source_n,
	list_source_counted,
}

list_source_counted :: proc(
	cx: ^Cx,
	state: ^$S,
	count: proc(s: ^S) -> int,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> List_Item,
	events: Handler = {},
	initial := 64,
	capacity := 512,
	loc := #caller_location,
) -> Row_Source {
	src := item_source(cx, state, count(state), row, events, SHAPE_LIST_EVENT, initial, capacity, loc)
	counted(src, count)
	return src
}

list_source_n :: proc(
	cx: ^Cx,
	state: ^$S,
	count: int,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> List_Item,
	events: Handler = {},
	initial := 64,
	capacity := 512,
	loc := #caller_location,
) -> Row_Source {
	return item_source(cx, state, count, row, events, SHAPE_LIST_EVENT, initial, capacity, loc)
}

// select_row selects a row of a list (row < 0: none), without the list's event: what the app
// calls when its own selection changed (after a filter, after a delete). With scroll, the list
// also scrolls so that the row is in view (a keyboard "next item").
select_row :: proc(cx: ^Cx, list: List_State, row: int, section := 0, scroll := false, loc := #caller_location) {
	if row < 0 {
		list_state_set_selected_index(list, cx, nil, loc)
	} else {
		list_state_set_selected_index(list, cx, Base_Index_Path{section = section, row = row}, loc)
		if scroll {
			list_state_scroll_to_item(list, cx, Base_Index_Path{section = section, row = row}, .Nearest, loc)
		}
	}
}

// --- tables ----------------------------------------------------------------------------------

// Table_Column is one column of a table: upstream's Column as plain data. Make one with
// gk.table_column and change the fields you need.
Table_Column :: struct {
	key, name:  string,
	align:      Text_Align,
	sort:       Maybe(Column_Sort), // none: not sortable
	paddings:   Maybe([4]f32), // top, right, bottom, left
	width:      f32,
	fixed:      Maybe(Column_Fixed),
	resizable:  bool,
	movable:    bool,
	selectable: bool,
	min_width:  f32,
	max_width:  f32,
}

// table_column is a column with upstream's defaults (100 px wide, resizable, movable,
// selectable, 20 to 1,200 px).
table_column :: proc(key, name: string, width: f32 = 100, sort: Maybe(Column_Sort) = nil, align := Text_Align.Left) -> Table_Column {
	return {key = key, name = name, align = align, sort = sort, width = width, resizable = true, movable = true, selectable = true, min_width = 20, max_width = 1200}
}

// Cell_At is the cell a cell view describes: its row and its column's index in the columns
// given to the table.
Cell_At :: struct {
	row, col: int,
}

@(private = "file")
Table_Rec :: struct {
	rows, cols: int,
}

@(private = "file")
tables: map[u64]Table_Rec // by source

// table_source makes the row source of a table with `rows` rows and `columns`, whose rows are
// the cells: `cell` describes one cell, once, and is called again only when the cell is
// invalidated (gk.table_invalidate) or what it read changed. `events` receives
// Table_Delegate_Event records. The first `initial_rows` rows are described before the first
// frame; at most `capacity` cells are kept, far ones are evicted. Make the table's state over it
// with gk.table_state(cx, src)->build() right after. Needs a window.
table_source :: proc(
	cx: ^Cx,
	state: ^$S,
	rows: int,
	columns: []Table_Column,
	cell: proc(s: ^S, cx: ^Cx, at: Cell_At) -> Element,
	events: Handler = {},
	initial_rows := 32,
	capacity := 4096,
	loc := #caller_location,
) -> Row_Source {
	if rows < 0 || initial_rows < 0 {
		stop(loc, "a negative number was given to gk.table_source (rows %d, initial_rows %d)", rows, initial_rows)
	}
	n := len(columns)
	if n > 0 && rows > MAX_ROWS / n {
		stop(loc, "gk.table_source was given %d rows of %d columns; a table holds at most %d cells (rows times columns)", rows, n, MAX_ROWS)
	}
	placeholder :: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element {return {}}
	tramp :: proc(fn, state: rawptr, cx: ^Cx, at: Row_At) -> Element {
		n := max(tables[build.view.src].cols, 1)
		return (cast(proc(s: ^S, cx: ^Cx, at: Cell_At) -> Element)fn)(cast(^S)state, cx, Cell_At{at.row / n, at.row % n})
	}
	src := row_source(cx, state, rows * n, placeholder, events, SHAPE_TABLE_EVENT, initial_rows * n, max(capacity, 16), loc)
	retarget(src, rawptr(cell), tramp)
	if tables == nil {
		tables = make(map[u64]Table_Rec, 4, runtime.heap_allocator())
	}
	tables[src.h] = {rows, n}
	req := make(Tape, 0, 16 + 64 * n, context.temp_allocator)
	put_u16(&req, C_TABLE_SOURCE_COLUMNS)
	put_u64(&req, src.h)
	put_columns(&req, columns)
	must_call(req[:], loc)
	return src
}

// table_set_rows changes the number of rows. Rows at and past it are dropped; rows below it
// keep their cells (gk.table_invalidate them if their content moved).
table_set_rows :: proc(cx: ^Cx, src: Row_Source, rows: int, loc := #caller_location) {
	rec := table_rec(src, loc)
	if rows < 0 {
		stop(loc, "gk.table_set_rows was given a negative count (%d)", rows)
	}
	if rec.cols > 0 && rows > MAX_ROWS / rec.cols {
		stop(loc, "gk.table_set_rows was given %d rows of %d columns; a table holds at most %d cells (rows times columns)", rows, rec.cols, MAX_ROWS)
	}
	set_count(cx, src, rows * rec.cols, 0, loc)
	tables[src.h] = {rows, rec.cols}
}

// table_invalidate has the cells of rows lo ..< hi described again before the next frame
// (after a sort, say).
table_invalidate :: proc(cx: ^Cx, src: Row_Source, lo, hi: int, loc := #caller_location) {
	rec := table_rec(src, loc)
	invalidate(cx, src, max(lo, 0) * rec.cols, max(hi, 0) * rec.cols, 0, loc)
}

// table_set_columns replaces the columns of the table state `t` over the row source `src`.
// With another number of columns every cell is described again.
table_set_columns :: proc(cx: ^Cx, t: Table_State, src: Row_Source, columns: []Table_Column, loc := #caller_location) {
	rec := table_rec(src, loc)
	n := len(columns)
	if n != rec.cols {
		set_count(cx, src, 0, 0, loc)
	}
	req := make(Tape, 0, 16 + 64 * n, context.temp_allocator)
	put_u16(&req, C_TABLE_SET_COLUMNS)
	put_u64(&req, t.h)
	put_columns(&req, columns)
	defer_op(check_cx(cx, loc), req[:], loc)
	if n != rec.cols {
		tables[src.h] = {rec.rows, n}
		set_count(cx, src, rec.rows * n, 0, loc)
	}
}

// table_columns gives the table over `src` the kit's own Column values
// (gk.column("size", "Size")->width(80)->sortable()), each sent as a value made outside a view
// (section 13.6): before its state is made (t nil, right after gk.table_source), or replacing
// the columns of the table state t. With another number of columns every cell is described
// again, as gk.table_set_columns does with gk.Table_Column.
table_columns :: proc(cx: ^Cx, src: Row_Source, columns: []Column, t: Table_State = {}, loc := #caller_location) {
	rec, ok := tables[src.h]
	if !ok {
		stop(loc, "gk.table_columns was given a row source that gk.table_source did not make")
	}
	n := len(columns)
	if n > 0 && rec.rows > MAX_ROWS / n {
		stop(loc, "gk.table_columns: %d rows of %d columns; a table holds at most %d cells (rows times columns)", rec.rows, n, MAX_ROWS)
	}
	if n != rec.cols {
		set_count(cx, src, 0, 0, loc)
	}
	req := make(Tape, 0, 64 + 256 * n, context.temp_allocator)
	put_u16(&req, C_TABLE_COLUMN_VALUES)
	put_u64(&req, src.h)
	put_u64(&req, t.h)
	put_u32(&req, u32(n))
	for c in columns {
		put_vtape(&req, c.nd)
	}
	if t.h == 0 {
		flush(check_cx(cx, loc), loc)
		must_call(req[:], loc)
	} else {
		defer_op(check_cx(cx, loc), req[:], loc)
	}
	if n != rec.cols {
		tables[src.h] = {rec.rows, n}
		set_count(cx, src, rec.rows * n, 0, loc)
	}
}

@(private = "file")
table_rec :: proc(src: Row_Source, loc: runtime.Source_Code_Location) -> Table_Rec {
	rec, ok := tables[src.h]
	if !ok {
		stop(loc, "this row source is not a table's (gk.table_source)")
	}
	return rec
}

@(private = "file")
put_columns :: proc(req: ^Tape, columns: []Table_Column) {
	put_u32(req, u32(len(columns)))
	for c in columns {
		c := c
		put_value(req, &c, type_info_of(Table_Column))
	}
}

// --- trees -----------------------------------------------------------------------------------

// Tree_Node is one item of a tree, depth first: an item is a folder when the next item is one
// level deeper.
Tree_Node :: struct {
	id, label: string,
	depth:     int,
	expanded:  bool,
	disabled:  bool,
}

// tree_set_items replaces the items of a tree state (made with gk.tree_state(cx)->build()).
// A buffered write.
tree_set_items :: proc(cx: ^Cx, t: Tree_State, items: []Tree_Node, loc := #caller_location) {
	req := make(Tape, 0, 16 + 32 * len(items), context.temp_allocator)
	put_u16(&req, C_TREE_SET_ITEMS)
	put_u64(&req, t.h)
	put_u32(&req, u32(len(items)))
	for it in items {
		if it.depth < 0 {
			stop(loc, "a tree item has a negative depth (%d)", it.depth)
		}
		put_str(&req, it.id)
		put_str(&req, it.label)
		put_u32(&req, u32(it.depth))
		put_u8(&req, u8(it.expanded) | u8(it.disabled) << 1)
	}
	defer_op(check_cx(cx, loc), req[:], loc)
}

// Tree_Entry is a visible entry of a tree, as its row reads it.
Tree_Entry :: struct {
	id, label: Temp_String,
	depth:     int,
	folder:    bool,
	expanded:  bool,
	disabled:  bool,
}

// tree_entry reads the visible entry `ix` of a tree; ok is false past the last one. In a row
// procedure it is a dependency: the row is described again when the tree changes (an expand,
// a selection).
tree_entry :: proc(cx: ^Cx, t: Tree_State, ix: int, loc := #caller_location) -> (e: Tree_Entry, ok: bool) {
	req := make(Tape, 0, 18, context.temp_allocator)
	put_u16(&req, C_TREE_ENTRY)
	put_u64(&req, t.h)
	put_u64(&req, u64(max(ix, 0)))
	r := state_get(t.h, cx, req[:], loc)
	if !rd_bool(&r) {
		return {}, false
	}
	e.id = rd_temp(&r)
	e.label = rd_temp(&r)
	e.depth = int(rd_u64(&r))
	flags := rd_u8(&r)
	e.folder, e.expanded, e.disabled = flags & 1 != 0, flags & 2 != 0, flags & 4 != 0
	return e, true
}

// tree_source makes the row source of a tree: row `ix` is the visible entry `ix`, described by
// `row` as a gk.list_item (the tree marks it selected and disabled). Give it to gk.tree. Needs
// a window.
tree_source :: proc(
	cx: ^Cx,
	state: ^$S,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> List_Item,
	initial := 64,
	capacity := 512,
	loc := #caller_location,
) -> Row_Source {
	// The count follows the tree's entries, which the kit keeps: rows past the last entry are
	// never drawn, and a row bound there reads no entry.
	return item_source(cx, state, 1 << 30, row, {}, 0, initial, capacity, loc)
}

// --- a table's texts and looks (docs/PROTOCOLS.md section 9.10) ------------------------------
//
// What the kit asks a table delegate while it draws or copies, answered from data the app pushes:
//   gk.table_texts(cx, a.table, 0, texts)      cell_text and cell_label (copying a cell, a screen reader)
//   gk.table_row_labels(cx, a.table, labels)   row_label (else the row's cells' labels, joined)
//   gk.table_look(cx, a.table, {groups = .., header_style = gk.style_refinement_default()->bg(..)})
// Columns are named by their index in the columns given to the table (Cell_At.col).

// Table_Row_Label is a row's screen-reader name (empty: upstream's).
Table_Row_Label :: struct {
	row:   int,
	label: string,
}

// Table_Group is a group header cell spanning `span` columns (upstream's ColumnGroup).
Table_Group :: struct {
	label: string,
	span:  int,
}

// Table_Group_View draws the group header cell of `label` (in place of the centred label).
Table_Group_View :: struct {
	label: string,
	view:  View,
}

// Table_Fit is the width a double click on a column's resize handle gives it (fit_column).
Table_Fit :: struct {
	col:   int,
	width: f32,
}

// Table_Look is how a table draws besides its cells. Zero values keep upstream's look.
Table_Look :: struct {
	fit:          []Table_Fit,
	groups:       [][]Table_Group, // group header levels from the top; nil: none
	group_views:  []Table_Group_View,
	last_column:  View, // in place of the 12 px filler after the last column
	header_style: Maybe(Style_Refinement), // refines the header row
	row_style:    Maybe(Style_Refinement), // refines every body row
}

// table_texts sets the texts of the cells of rows first_row ..< first_row + len(texts): texts[i]
// holds one string per column, in the order the columns were given (empty: none). `replace`
// drops every text set before. A buffered write.
table_texts :: proc(cx: ^Cx, t: Table_State, first_row: int, texts: [][]string, replace := false, loc := #caller_location) {
	tc := check_cx(cx, loc)
	if first_row < 0 {
		stop(loc, "gk.table_texts was given a negative row (%d)", first_row)
	}
	cols := 0
	for r in texts {
		cols = max(cols, len(r))
	}
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_TABLE_TEXTS)
	put_u64(&req, t.h)
	put_u8(&req, replace ? 1 : 0)
	put_u64(&req, u64(first_row))
	put_u32(&req, u32(len(texts)))
	put_u32(&req, u32(cols))
	for r in texts {
		for c in 0 ..< cols {
			put_str(&req, c < len(r) ? r[c] : "")
		}
	}
	defer_op(tc, req[:], loc)
}

// table_row_labels sets rows' screen-reader names. A buffered write.
table_row_labels :: proc(cx: ^Cx, t: Table_State, labels: []Table_Row_Label, loc := #caller_location) {
	tc := check_cx(cx, loc)
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_TABLE_ROW_LABELS)
	put_u64(&req, t.h)
	put_u32(&req, u32(len(labels)))
	for l in labels {
		if l.row < 0 {
			stop(loc, "gk.table_row_labels was given a negative row (%d)", l.row)
		}
		put_u64(&req, u64(l.row))
		put_str(&req, l.label)
	}
	defer_op(tc, req[:], loc)
}

// table_look sets how a table draws besides its cells (Table_Look). A buffered write; the
// styles are values made in this handler or init (gk.style_refinement_default()->..).
table_look :: proc(cx: ^Cx, t: Table_State, look: Table_Look, loc := #caller_location) {
	tc := check_cx(cx, loc)
	req := make(Tape, 0, 128, context.temp_allocator)
	o := &req
	put_u16(o, C_TABLE_LOOK)
	put_u64(o, t.h)
	put_u32(o, u32(len(look.fit)))
	for f in look.fit {
		if f.col < 0 {
			stop(loc, "gk.table_look was given a negative column (%d)", f.col)
		}
		put_u32(o, u32(f.col))
		put_f32(o, f.width)
	}
	put_u8(o, look.groups != nil ? 1 : 0)
	put_u32(o, u32(len(look.groups)))
	for level in look.groups {
		put_u32(o, u32(len(level)))
		for g in level {
			if g.span < 0 {
				stop(loc, "gk.table_look was given a negative span (%d)", g.span)
			}
			put_str(o, g.label)
			put_u64(o, u64(g.span))
		}
	}
	put_u32(o, u32(len(look.group_views)))
	for g in look.group_views {
		put_str(o, g.label)
		put_u64(o, g.view.h)
	}
	put_u64(o, look.last_column.h)
	for st in ([2]Maybe(Style_Refinement){look.header_style, look.row_style}) {
		if s, ok := st.?; ok {
			put_vtape(o, s.nd)
		} else {
			put_u32(o, 0)
		}
	}
	defer_op(tc, req[:], loc)
}

// --- the family's side of rt_source.odin -------------------------------------------------------

// item_source is row_source for rows described as a gk.list_item (lists and trees).
@(private = "file")
item_source :: proc(
	cx: ^Cx,
	state: ^$S,
	count: int,
	row: proc(s: ^S, cx: ^Cx, at: Row_At) -> List_Item,
	events: Handler,
	shape: u16,
	initial, capacity: int,
	loc: runtime.Source_Code_Location,
) -> Row_Source {
	placeholder :: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element {return {}}
	tramp :: proc(fn, state: rawptr, cx: ^Cx, at: Row_At) -> Element {
		return (cast(proc(s: ^S, cx: ^Cx, at: Row_At) -> List_Item)fn)(cast(^S)state, cx, at).el
	}
	src := row_source(cx, state, count, placeholder, events, shape, initial, capacity, loc)
	retarget(src, rawptr(row), tramp)
	return src
}

// retarget gives a source and the rows it bound already another row procedure: row_source
// knows only procedures that return an Element.
@(private = "file")
retarget :: proc(src: Row_Source, fn: rawptr, call: proc(fn, state: rawptr, cx: ^Cx, at: Row_At) -> Element) {
	rec := sources[src.h]
	rec.row_fn = fn
	rec.row_call = call
	for _, v in rec.rows {
		if vr, ok := views[v]; ok {
			vr.view_fn = fn
			vr.row_call = call
		}
	}
}

// Table_Child is what a static table's parts take as a child (upstream's `impl ChildElement`):
// a header, a body, a footer, a row, a head or body cell, a caption or a table (PROTOCOLS.md 9.10).
Table_Child :: union {
	Table,
	Table_Header,
	Table_Body,
	Table_Footer,
	Table_Row,
	Table_Head,
	Table_Cell,
	Table_Caption,
}

// --- per-row menus (docs/PROTOCOLS.md 11.24) ------------------------------------------------------
//
// A table's or a tree's context menu is described with each row, ahead of the right-click that
// opens it: the row (a table's cell, any column; a tree's or a list's item) wraps its element in
// gk.row_menu with a gk.popup_menu() of the items it offers. The kit's TableDelegate::context_menu
// and Tree::context_menu (`gk.tree(..)->context_menu(rows)`, the tree's own row source) apply that
// menu when the row is right-clicked; a row without one opens nothing. The menu's handlers are the
// row's: they run with the row's state and mark it for redraw.
//
//   cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
//       el := gk.label(a.names[at.row])
//       if at.col != 0 {return el}
//       return gk.row_menu(el, gk.popup_menu()->menu("Open", open)->menu("Delete", delete))
//   }

// row_menu wraps a row's element (a table's cell) with the menu its row opens on a right-click.
row_menu_element :: proc(el: Element, menu: Popup_Menu, loc := #caller_location) -> Element {
	e := new_node(N_ROW_MENU, loc)
	put_u32(&build.ops, take(el))
	put_u32(&build.ops, take_value(menu.nd))
	return e
}

// row_menu_item is row_menu for a list's or a tree's row, which is a gk.list_item: call it last,
// on the finished item (later builder calls would go to the wrapper, which ignores them).
row_menu_item :: proc(item: List_Item, menu: Popup_Menu, loc := #caller_location) -> List_Item {
	out := item
	out.el = row_menu_element(item.el, menu, loc)
	return out
}

row_menu :: proc {
	row_menu_element,
	row_menu_item,
}
