package gk

// Tables and trees of the rows family (docs/PROTOCOLS.md 9.5, 9.10, 11.24), ported from
// odin/gk/kit_rows.odin. A table's row source holds its cells: each cell is a view of its own,
// described once by the app's cell function and retained.
//
//	src := gk.NewTableSource(cx, 100_000, columns, a.cell, gk.TableEvents(a.tableEvent))
//	a.table = gk.NewTableState(cx, src).Build()          view: gk.DataTable(a.table)
//	func (a *App) cell(cx *gk.Cx, at gk.CellAt) gk.Element { ... }   // at.Col: the column's index in columns
//
//	a.tree = gk.NewTreeState(cx).Build()    gk.SetTreeItems(cx, a.tree, nodes)
//	rows := gk.NewTreeSource(cx, a.treeRow)  view: gk.Tree(a.tree, rows)
//	treeRow reads its entry with gk.TreeEntryAt(cx, a.tree, at.Row).

// TableEventKind is what a table's delegate reports.
type TableEventKind uint16

const (
	TableSort           TableEventKind = iota // a sortable column's sort icon was clicked: Col, Sort (sort, then gk.InvalidateTable)
	TableMoveColumn                           // the user dragged column Col to position To
	TableVisibleRows                          // rows Row to To are visible (only with DelegateData.VisibleEvents)
	TableVisibleColumns                       // columns Col to To are visible (only with DelegateData.VisibleEvents)
	TableLoadMore                             // scrolled near the end while DelegateData.HasMore is set
)

func (k TableEventKind) String() string {
	return [...]string{"Sort", "MoveColumn", "VisibleRows", "VisibleColumns", "LoadMore", "?"}[min(int(k), 5)]
}

// TableDelegateEvent is the event of a table's row source. A column is named by its index in the
// columns given to the table, which stays when the user moves it. Selections, double clicks and
// widths are the table state's own events (TableState.OnTableEvent).
type TableDelegateEvent struct {
	ID   uint64
	Kind TableEventKind
	Row  int
	Col  int
	To   int
	Sort ColumnSort
}

func init() {
	registerShape(shapeTableEvent, shapeInfo{"func(cx *Cx, e TableDelegateEvent)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, TableDelegateEvent))
		if ok {
			f(cx, TableDelegateEvent{ID: r.u64(), Kind: TableEventKind(r.u16()), Row: int(r.u64()), Col: int(r.u64()),
				To: int(r.u64()), Sort: ColumnSort(r.u16())})
		}
		return ok
	}})
}

// TableEvents gives a table's source the handler of its delegate's events.
func TableEvents(f func(cx *Cx, e TableDelegateEvent)) SourceOption {
	return sourceEvents(f, shapeTableEvent)
}

// TableColumn is one column of a table: upstream's Column as plain data. Make one with
// gk.NewTableColumn and change the fields you need.
type TableColumn struct {
	Key, Name  string
	Align      TextAlign
	Sort       Option[ColumnSort] // none: not sortable
	Paddings   Option[[4]float32] // top, right, bottom, left
	Width      float32
	Fixed      Option[ColumnFixed]
	Resizable  bool
	Movable    bool
	Selectable bool
	MinWidth   float32
	MaxWidth   float32
}

// NewTableColumn is a column with upstream's defaults (100 px wide, resizable, movable,
// selectable, 20 to 1,200 px).
func NewTableColumn(key, name string) TableColumn {
	return TableColumn{Key: key, Name: name, Width: 100, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 1200}
}

func putTableColumns(o *tape, columns []TableColumn) {
	o.u32(uint32(len(columns)))
	for _, c := range columns {
		o.str(c.Key)
		o.str(c.Name)
		o.u16(uint16(c.Align))
		if v, ok := c.Sort.Get(); ok {
			o.u8(1)
			o.u16(uint16(v))
		} else {
			o.u8(0)
		}
		if v, ok := c.Paddings.Get(); ok {
			o.u8(1)
			for _, x := range v {
				o.f32(x)
			}
		} else {
			o.u8(0)
		}
		o.f32(c.Width)
		if v, ok := c.Fixed.Get(); ok {
			o.u8(1)
			o.u16(uint16(v))
		} else {
			o.u8(0)
		}
		o.bool(c.Resizable)
		o.bool(c.Movable)
		o.bool(c.Selectable)
		o.f32(c.MinWidth)
		o.f32(c.MaxWidth)
	}
}

// CellAt is the cell a cell function describes: its row and its column's index in the columns
// given to the table.
type CellAt struct{ Row, Col int }

type tableRec struct{ rows, cols int }

var tables = map[uint64]*tableRec{} // by source

// NewTableSource makes the row source of a table with rows rows and columns, whose rows are the
// cells: cell describes one cell, once, and is called again only when the cell is invalidated
// (gk.InvalidateTable) or what it read changed. gk.TableEvents gives it a handler; gk.Initial is
// the rows described before the first frame (default 32), gk.Capacity the cells kept (default
// 4,096). Make the table's state over it with gk.NewTableState(cx, src).Build() right after.
// Needs a window.
func NewTableSource(cx *Cx, rows int, columns []TableColumn, cell func(cx *Cx, at CellAt) Element, opts ...SourceOption) RowSource {
	if cell == nil {
		stop("gk.NewTableSource needs a cell function")
	}
	so := sourceOpts{initial: 32, capacity: 4096}
	for _, o := range opts {
		o(&so)
	}
	if rows < 0 || so.initial < 0 {
		stop("a negative number was given to gk.NewTableSource (rows %d, initial %d)", rows, so.initial)
	}
	n := len(columns)
	if n > 0 && rows > MaxRows/n {
		stop("gk.NewTableSource was given %d rows of %d columns; a table holds at most %d cells (rows times columns)", rows, n, MaxRows)
	}
	rec := &tableRec{rows, n}
	row := func(cx *Cx, at RowAt) Element {
		c := max(rec.cols, 1)
		return cell(cx, CellAt{at.Row / c, at.Row % c})
	}
	all := append(opts[:len(opts):len(opts)], Initial(so.initial*n), Capacity(max(so.capacity, 16)))
	src := NewRowSource(cx, rows*n, row, all...)
	tables[src.h] = rec
	o := newReq(cTableSourceColumns)
	o.u64(src.h)
	putTableColumns(o, columns)
	mustCall(*o)
	return src
}

func tableRecOf(src RowSource) *tableRec {
	rec := tables[src.h]
	if rec == nil {
		stop("this row source is not a table's (gk.NewTableSource)")
	}
	return rec
}

// SetTableRows changes the number of rows. Rows at and past it are dropped; rows below it keep
// their cells (gk.InvalidateTable them if their content moved).
func SetTableRows(cx *Cx, src RowSource, rows int) {
	rec := tableRecOf(src)
	if rows < 0 {
		stop("gk.SetTableRows was given a negative count (%d)", rows)
	}
	if rec.cols > 0 && rows > MaxRows/rec.cols {
		stop("gk.SetTableRows was given %d rows of %d columns; a table holds at most %d cells (rows times columns)", rows, rec.cols, MaxRows)
	}
	SetCount(cx, src, rows*rec.cols)
	rec.rows = rows
}

// InvalidateTable has the cells of rows lo to hi (not included) described again before the next
// frame (after a sort, say).
func InvalidateTable(cx *Cx, src RowSource, lo, hi int) {
	rec := tableRecOf(src)
	Invalidate(cx, src, max(lo, 0)*rec.cols, max(hi, 0)*rec.cols)
}

// SetTableColumns replaces the columns of the table state t over the row source src. With
// another number of columns every cell is described again.
func SetTableColumns(cx *Cx, t TableState, src RowSource, columns []TableColumn) {
	rec := tableRecOf(src)
	n := len(columns)
	if n != rec.cols {
		SetCount(cx, src, 0)
	}
	o := newReq(cTableSetColumns)
	o.u64(t.h)
	putTableColumns(o, columns)
	checkCx(cx).deferOp(*o)
	if n != rec.cols {
		rec.cols = n
		SetCount(cx, src, rec.rows*n)
	}
}

// SetTableKitColumns gives the table over src the kit's own Column values
// (gk.NewColumn("size", "Size").Width(80).Sortable()), each sent as a value made outside a view:
// before its state is made (no t, right after gk.NewTableSource), or replacing the columns of the
// table state t. With another number of columns every cell is described again.
func SetTableKitColumns(cx *Cx, src RowSource, columns []Column, t ...TableState) {
	rec := tableRecOf(src)
	n := len(columns)
	if n > 0 && rec.rows > MaxRows/n {
		stop("gk.SetTableKitColumns: %d rows of %d columns; a table holds at most %d cells (rows times columns)", rec.rows, n, MaxRows)
	}
	var th uint64
	if len(t) > 0 {
		th = t[0].h
	}
	if n != rec.cols {
		SetCount(cx, src, 0)
	}
	o := newReq(cTableColumnValues)
	o.u64(src.h)
	o.u64(th)
	o.u32(uint32(n))
	for _, c := range columns {
		putVtape(o, c.n)
	}
	if th == 0 {
		checkCx(cx).flush()
		mustCall(*o)
	} else {
		checkCx(cx).deferOp(*o)
	}
	if n != rec.cols {
		rec.cols = n
		SetCount(cx, src, rec.rows*n)
	}
}

// --- trees -----------------------------------------------------------------------------------

// TreeNode is one item of a tree, depth first: an item is a folder when the next item is one
// level deeper.
type TreeNode struct {
	ID, Label string
	Depth     int
	Expanded  bool
	Disabled  bool
}

// SetTreeItems replaces the items of a tree state (made with gk.NewTreeState(cx).Build()). A
// buffered write.
func SetTreeItems(cx *Cx, t TreeState, items []TreeNode) {
	o := newReq(cTreeSetItems)
	o.u64(t.h)
	o.u32(uint32(len(items)))
	for _, it := range items {
		if it.Depth < 0 {
			stop("a tree item has a negative depth (%d)", it.Depth)
		}
		o.str(it.ID)
		o.str(it.Label)
		o.u32(uint32(it.Depth))
		var flags uint8
		if it.Expanded {
			flags |= 1
		}
		if it.Disabled {
			flags |= 2
		}
		o.u8(flags)
	}
	checkCx(cx).deferOp(*o)
}

// TreeEntry is a visible entry of a tree, as its row reads it.
type TreeEntry struct {
	ID, Label string
	Depth     int
	Folder    bool
	Expanded  bool
	Disabled  bool
}

// TreeEntryAt reads the visible entry ix of a tree; false past the last one. In a row function it
// is a dependency: the row is described again when the tree changes (an expand, a selection).
func TreeEntryAt(cx *Cx, t TreeState, ix int) (TreeEntry, bool) {
	o := newReq(cTreeEntry)
	o.u64(t.h)
	o.u64(uint64(max(ix, 0)))
	r := stateGet(t.h, cx, o)
	if !r.bool() {
		return TreeEntry{}, false
	}
	e := TreeEntry{ID: r.str(), Label: r.str(), Depth: int(r.u64())}
	flags := r.u8()
	e.Folder, e.Expanded, e.Disabled = flags&1 != 0, flags&2 != 0, flags&4 != 0
	return e, true
}

// NewTreeSource makes the row source of a tree: row ix is the visible entry ix, described by row
// as a gk.ListItem (the tree marks it selected and disabled). Give it to gk.Tree. Needs a window.
func NewTreeSource(cx *Cx, row func(cx *Cx, at RowAt) ListItemEl, opts ...SourceOption) RowSource {
	if row == nil {
		stop("gk.NewTreeSource needs a row function")
	}
	// The count follows the tree's entries, which the kit keeps: rows past the last entry are
	// never drawn, and a row bound there reads no entry.
	return NewRowSource(cx, 1<<30, func(cx *Cx, at RowAt) Element { return row(cx, at) }, opts...)
}

// --- a table's texts and looks (docs/PROTOCOLS.md section 9.10) ------------------------------
//
// What the kit asks a table delegate while it draws or copies, answered from data the app pushes:
//
//	gk.SetTableTexts(cx, a.table, 0, texts, false)   cell_text and cell_label (copying, a screen reader)
//	gk.SetTableRowLabels(cx, a.table, labels)        row_label (else the row's cells' labels, joined)
//	gk.SetTableLook(cx, a.table, gk.TableLook{Groups: .., HeaderStyle: gk.Some(..)})
//
// Columns are named by their index in the columns given to the table (CellAt.Col).

// TableRowLabel is a row's screen-reader name (empty: upstream's).
type TableRowLabel struct {
	Row   int
	Label string
}

// TableGroup is a group header cell spanning Span columns (upstream's ColumnGroup).
type TableGroup struct {
	Label string
	Span  int
}

// TableGroupView draws the group header cell of Label (in place of the centered label).
type TableGroupView struct {
	Label string
	View  View
}

// TableFit is the width a double click on a column's resize handle gives it (fit_column).
type TableFit struct {
	Col   int
	Width float32
}

// TableLook is how a table draws besides its cells. Zero values keep upstream's look.
type TableLook struct {
	Fit         []TableFit
	Groups      [][]TableGroup // group header levels from the top; nil: none
	GroupViews  []TableGroupView
	LastColumn  View                    // in place of the 12 px filler after the last column
	HeaderStyle Option[StyleRefinement] // refines the header row
	RowStyle    Option[StyleRefinement] // refines every body row
}

// SetTableTexts sets the texts of the cells of rows firstRow to firstRow + len(texts): texts[i]
// holds one string per column, in the order the columns were given (empty: none). replace drops
// every text set before. A buffered write.
func SetTableTexts(cx *Cx, t TableState, firstRow int, texts [][]string, replace bool) {
	tc := checkCx(cx)
	if firstRow < 0 {
		stop("gk.SetTableTexts was given a negative row (%d)", firstRow)
	}
	cols := 0
	for _, r := range texts {
		cols = max(cols, len(r))
	}
	o := newReq(cTableTexts)
	o.u64(t.h)
	o.bool(replace)
	o.u64(uint64(firstRow))
	o.u32(uint32(len(texts)))
	o.u32(uint32(cols))
	for _, r := range texts {
		for c := range cols {
			if c < len(r) {
				o.str(r[c])
			} else {
				o.str("")
			}
		}
	}
	tc.deferOp(*o)
}

// SetTableRowLabels sets rows' screen-reader names. A buffered write.
func SetTableRowLabels(cx *Cx, t TableState, labels []TableRowLabel) {
	tc := checkCx(cx)
	o := newReq(cTableRowLabels)
	o.u64(t.h)
	o.u32(uint32(len(labels)))
	for _, l := range labels {
		if l.Row < 0 {
			stop("gk.SetTableRowLabels was given a negative row (%d)", l.Row)
		}
		o.u64(uint64(l.Row))
		o.str(l.Label)
	}
	tc.deferOp(*o)
}

// SetTableLook sets how a table draws besides its cells. A buffered write; the styles are values
// made in this handler or init (gk.NewStyleRefinementDefault()...).
func SetTableLook(cx *Cx, t TableState, look TableLook) {
	tc := checkCx(cx)
	o := newReq(cTableLook)
	o.u64(t.h)
	o.u32(uint32(len(look.Fit)))
	for _, f := range look.Fit {
		if f.Col < 0 {
			stop("gk.SetTableLook was given a negative column (%d)", f.Col)
		}
		o.u32(uint32(f.Col))
		o.f32(f.Width)
	}
	o.bool(look.Groups != nil)
	o.u32(uint32(len(look.Groups)))
	for _, level := range look.Groups {
		o.u32(uint32(len(level)))
		for _, g := range level {
			if g.Span < 0 {
				stop("gk.SetTableLook was given a negative span (%d)", g.Span)
			}
			o.str(g.Label)
			o.u64(uint64(g.Span))
		}
	}
	o.u32(uint32(len(look.GroupViews)))
	for _, g := range look.GroupViews {
		o.str(g.Label)
		o.u64(g.View.h)
	}
	o.u64(look.LastColumn.h)
	for _, st := range [2]Option[StyleRefinement]{look.HeaderStyle, look.RowStyle} {
		if s, ok := st.Get(); ok {
			putVtape(o, s.n)
		} else {
			o.u32(0)
		}
	}
	tc.deferOp(*o)
}

// --- per-row menus (docs/PROTOCOLS.md 11.24) ------------------------------------------------------
//
// A table's or a tree's context menu is described with each row, ahead of the right-click that
// opens it: the row (a table's cell, any column; a tree's or a list's item) wraps its element in
// gk.RowMenu with a gk.NewPopupMenu() of the items it offers. The menu's handlers are the row's.
//
//	func (a *App) cell(cx *gk.Cx, at gk.CellAt) gk.Element {
//		el := gk.Label(a.names[at.Row])
//		if at.Col != 0 { return el }
//		return gk.RowMenu(el, gk.NewPopupMenu().Menu("Open", a.open).Menu("Delete", a.delete))
//	}

// RowMenu wraps a row's element (a table's cell) with the menu its row opens on a right-click.
func RowMenu(el Element, menu PopupMenu) Element {
	src := elemNode(el)
	n := newNode(nRowMenu)
	o := nodeArgs(n)
	o.u32(takeNode(src, false))
	o.u32(takeValue(menu.n))
	return valueEl{n}
}

// RowMenuItem is gk.RowMenu for a list's or a tree's row, which is a gk.ListItem: call it last, on
// the finished item (later builder calls would go to the wrapper, which ignores them).
func RowMenuItem(item ListItemEl, menu PopupMenu) ListItemEl {
	return ListItemEl{elemNode(RowMenu(item, menu))}
}
