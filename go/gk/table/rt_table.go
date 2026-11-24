package table

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
)

// TableEventKind is what a table's delegate reports.
type EventKind uint16

const (
	Sort           EventKind = iota // a sortable column's sort icon was clicked: Col, Sort (sort, then gk.InvalidateTable)
	MoveColumn                      // the user dragged column Col to position To
	VisibleRows                     // rows Row to To are visible (only with DelegateData.VisibleEvents)
	VisibleColumns                  // columns Col to To are visible (only with DelegateData.VisibleEvents)
	LoadMore                        // scrolled near the end while DelegateData.HasMore is set
)

func (k EventKind) String() string {
	return [...]string{"Sort", "MoveColumn", "VisibleRows", "VisibleColumns", "LoadMore", "?"}[min(int(k), 5)]
}

// TableDelegateEvent is the event of a table's row source. A column is named by its index in the
// columns given to the table, which stays when the user moves it. Selections, double clicks and
// widths are the table state's own events (TableState.OnTableEvent).
type DelegateEvent struct {
	ID   uint64
	Kind EventKind
	Row  int
	Col  int
	To   int
	Sort ColumnSort
}

func init() {
	rt.RegisterShape(rt.ShapeTableEvent, rt.ShapeInfo{Name: "func(cx *Cx, e TableDelegateEvent)", Call: func(h any, cx *gk.Cx, r *rt.Reader, ret *rt.Tape) bool {
		f, ok := h.(func(*gk.Cx, DelegateEvent))
		if ok {
			f(cx, DelegateEvent{ID: r.U64(), Kind: EventKind(r.U16()), Row: int(r.U64()), Col: int(r.U64()),
				To: int(r.U64()), Sort: ColumnSort(r.U16())})
		}
		return ok
	}})
}

// TableEvents gives a table's source the handler of its delegate's events.
func Events(f func(cx *gk.Cx, e DelegateEvent)) gk.SourceOption {
	return rt.SourceEvents(f, rt.ShapeTableEvent)
}

// TableColumn is one column of a table: upstream's Column as plain data. Make one with
// gk.NewTableColumn and change the fields you need.
type TableColumn struct {
	Key, Name  string
	Align      gk.TextAlign
	Sort       gk.Option[ColumnSort] // none: not sortable
	Paddings   gk.Option[[4]float32] // top, right, bottom, left
	Width      float32
	Fixed      gk.Option[ColumnFixed]
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

func putTableColumns(o *rt.Tape, columns []TableColumn) {
	o.U32(uint32(len(columns)))
	for _, c := range columns {
		o.Str(c.Key)
		o.Str(c.Name)
		o.U16(uint16(c.Align))
		if v, ok := c.Sort.Get(); ok {
			o.U8(1)
			o.U16(uint16(v))
		} else {
			o.U8(0)
		}
		if v, ok := c.Paddings.Get(); ok {
			o.U8(1)
			for _, x := range v {
				o.F32(x)
			}
		} else {
			o.U8(0)
		}
		o.F32(c.Width)
		if v, ok := c.Fixed.Get(); ok {
			o.U8(1)
			o.U16(uint16(v))
		} else {
			o.U8(0)
		}
		o.Bool(c.Resizable)
		o.Bool(c.Movable)
		o.Bool(c.Selectable)
		o.F32(c.MinWidth)
		o.F32(c.MaxWidth)
	}
}

// CellAt is the cell a cell function describes: its row and its column's index in the columns
// given to the table.
type CellAt struct{ Row, Col int }

// NewTableSource makes the row source of a table with rows rows and columns, whose rows are the
// cells: cell describes one cell, once, and is called again only when the cell is invalidated
// (gk.InvalidateTable) or what it read changed. gk.TableEvents gives it a handler; gk.Initial is
// the rows described before the first frame (default 32), gk.Capacity the cells kept (default
// 4,096). Make the table's state over it with gk.NewTableState(cx, src).Build() right after.
// Needs a window.
func NewSource(cx *gk.Cx, rows int, columns []TableColumn, cell func(cx *gk.Cx, at CellAt) gk.Element, opts ...gk.SourceOption) gk.RowSource {
	if cell == nil {
		rt.Stop("table.NewSource needs a cell function")
	}
	so := rt.SourceOpts{Initial: 32, Capacity: 4096}
	for _, o := range opts {
		o(&so)
	}
	if rows < 0 || so.Initial < 0 {
		rt.Stop("a negative number was given to table.NewSource (rows %d, initial %d)", rows, so.Initial)
	}
	n := len(columns)
	if n > 0 && rows > gk.MaxRows/n {
		rt.Stop("table.NewSource was given %d rows of %d columns; a table holds at most %d cells (rows times columns)", rows, n, gk.MaxRows)
	}
	rec := &rt.TableRec{Rows: rows, Cols: n}
	row := func(cx *gk.Cx, at gk.RowAt) gk.Element {
		c := max(rec.Cols, 1)
		return cell(cx, CellAt{at.Row / c, at.Row % c})
	}
	all := append(opts[:len(opts):len(opts)], gk.Initial(so.Initial*n), gk.Capacity(max(so.Capacity, 16)))
	src := gk.NewRowSource(cx, rows*n, row, all...)
	rt.Tables[rt.H(src.HCore)] = rec
	o := rt.NewReq(rt.CTableSourceColumns)
	o.U64(rt.H(src.HCore))
	putTableColumns(o, columns)
	rt.MustCall(*o)
	return src
}

func tableRecOf(src gk.RowSource) *rt.TableRec {
	rec := rt.Tables[rt.H(src.HCore)]
	if rec == nil {
		rt.Stop("this row source is not a table's (table.NewSource)")
	}
	return rec
}

// SetTableRows changes the number of rows. Rows at and past it are dropped; rows below it keep
// their cells (gk.InvalidateTable them if their content moved).
func SetTableRows(cx *gk.Cx, src gk.RowSource, rows int) {
	rec := tableRecOf(src)
	if rows < 0 {
		rt.Stop("table.SetTableRows was given a negative count (%d)", rows)
	}
	if rec.Cols > 0 && rows > gk.MaxRows/rec.Cols {
		rt.Stop("table.SetTableRows was given %d rows of %d columns; a table holds at most %d cells (rows times columns)", rows, rec.Cols, gk.MaxRows)
	}
	gk.SetCount(cx, src, rows*rec.Cols)
	rec.Rows = rows
}

// InvalidateTable has the cells of rows lo to hi (not included) described again before the next
// frame (after a sort, say).
func InvalidateTable(cx *gk.Cx, src gk.RowSource, lo, hi int) {
	rec := tableRecOf(src)
	gk.Invalidate(cx, src, max(lo, 0)*rec.Cols, max(hi, 0)*rec.Cols)
}

// SetTableColumns replaces the columns of the table state t over the row source src. With
// another number of columns every cell is described again.
func SetTableColumns(cx *gk.Cx, t State, src gk.RowSource, columns []TableColumn) {
	rec := tableRecOf(src)
	n := len(columns)
	if n != rec.Cols {
		gk.SetCount(cx, src, 0)
	}
	o := rt.NewReq(rt.CTableSetColumns)
	o.U64(rt.H(t.HCore))
	putTableColumns(o, columns)
	rt.CheckCx(cx).DeferOp(*o)
	if n != rec.Cols {
		rec.Cols = n
		gk.SetCount(cx, src, rec.Rows*n)
	}
}

// SetTableKitColumns gives the table over src the kit's own Column values
// (gk.NewColumn("size", "Size").Width(80).Sortable()), each sent as a value made outside a view:
// before its state is made (no t, right after table.NewSource), or replacing the columns of the
// table state t. With another number of columns every cell is described again.
func SetTableKitColumns(cx *gk.Cx, src gk.RowSource, columns []Column, t ...State) {
	rec := tableRecOf(src)
	n := len(columns)
	if n > 0 && rec.Rows > gk.MaxRows/n {
		rt.Stop("table.SetTableKitColumns: %d rows of %d columns; a table holds at most %d cells (rows times columns)", rec.Rows, n, gk.MaxRows)
	}
	var th uint64
	if len(t) > 0 {
		th = rt.H(t[0].HCore)
	}
	if n != rec.Cols {
		gk.SetCount(cx, src, 0)
	}
	o := rt.NewReq(rt.CTableColumnValues)
	o.U64(rt.H(src.HCore))
	o.U64(th)
	o.U32(uint32(n))
	for _, c := range columns {
		rt.PutVtape(o, rt.N(c.Core))
	}
	if th == 0 {
		rt.CheckCx(cx).Flush()
		rt.MustCall(*o)
	} else {
		rt.CheckCx(cx).DeferOp(*o)
	}
	if n != rec.Cols {
		rec.Cols = n
		gk.SetCount(cx, src, rec.Rows*n)
	}
}

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
func SetTreeItems(cx *gk.Cx, t TreeState, items []TreeNode) {
	o := rt.NewReq(rt.CTreeSetItems)
	o.U64(rt.H(t.HCore))
	o.U32(uint32(len(items)))
	for _, it := range items {
		if it.Depth < 0 {
			rt.Stop("a tree item has a negative depth (%d)", it.Depth)
		}
		o.Str(it.ID)
		o.Str(it.Label)
		o.U32(uint32(it.Depth))
		var flags uint8
		if it.Expanded {
			flags |= 1
		}
		if it.Disabled {
			flags |= 2
		}
		o.U8(flags)
	}
	rt.CheckCx(cx).DeferOp(*o)
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
func TreeEntryAt(cx *gk.Cx, t TreeState, ix int) (TreeEntry, bool) {
	o := rt.NewReq(rt.CTreeEntry)
	o.U64(rt.H(t.HCore))
	o.U64(uint64(max(ix, 0)))
	r := rt.StateGet(rt.H(t.HCore), cx, o)
	if !r.Bool() {
		return TreeEntry{}, false
	}
	e := TreeEntry{ID: r.Str(), Label: r.Str(), Depth: int(r.U64())}
	flags := r.U8()
	e.Folder, e.Expanded, e.Disabled = flags&1 != 0, flags&2 != 0, flags&4 != 0
	return e, true
}

// NewTreeSource makes the row source of a tree: row ix is the visible entry ix, described by row
// as a gk.ListItem (the tree marks it selected and disabled). Give it to gk.Tree. Needs a window.
func NewTreeSource(cx *gk.Cx, row func(cx *gk.Cx, at gk.RowAt) ListItemEl, opts ...gk.SourceOption) gk.RowSource {
	if row == nil {
		rt.Stop("table.NewTreeSource needs a row function")
	}
	// The count follows the tree's entries, which the kit keeps: rows past the last entry are
	// never drawn, and a row bound there reads no entry.
	return gk.NewRowSource(cx, 1<<30, func(cx *gk.Cx, at gk.RowAt) gk.Element { return row(cx, at) }, opts...)
}

// TableRowLabel is a row's screen-reader name (empty: upstream's).
type RowLabel struct {
	Row   int
	Label string
}

// TableGroup is a group header cell spanning Span columns (upstream's ColumnGroup).
type Group struct {
	Label string
	Span  int
}

// TableGroupView draws the group header cell of Label (in place of the centered label).
type GroupView struct {
	Label string
	View  gk.View
}

// TableFit is the width a double click on a column's resize handle gives it (fit_column).
type Fit struct {
	Col   int
	Width float32
}

// TableLook is how a table draws besides its cells. Zero values keep upstream's look.
type Look struct {
	Fit         []Fit
	Groups      [][]Group // group header levels from the top; nil: none
	GroupViews  []GroupView
	LastColumn  gk.View                       // in place of the 12 px filler after the last column
	HeaderStyle gk.Option[gk.StyleRefinement] // refines the header row
	RowStyle    gk.Option[gk.StyleRefinement] // refines every body row
}

// SetTableTexts sets the texts of the cells of rows firstRow to firstRow + len(texts): texts[i]
// holds one string per column, in the order the columns were given (empty: none). replace drops
// every text set before. A buffered write.
func SetTableTexts(cx *gk.Cx, t State, firstRow int, texts [][]string, replace bool) {
	tc := rt.CheckCx(cx)
	if firstRow < 0 {
		rt.Stop("table.SetTableTexts was given a negative row (%d)", firstRow)
	}
	cols := 0
	for _, r := range texts {
		cols = max(cols, len(r))
	}
	o := rt.NewReq(rt.CTableTexts)
	o.U64(rt.H(t.HCore))
	o.Bool(replace)
	o.U64(uint64(firstRow))
	o.U32(uint32(len(texts)))
	o.U32(uint32(cols))
	for _, r := range texts {
		for c := range cols {
			if c < len(r) {
				o.Str(r[c])
			} else {
				o.Str("")
			}
		}
	}
	tc.DeferOp(*o)
}

// SetTableRowLabels sets rows' screen-reader names. A buffered write.
func SetTableRowLabels(cx *gk.Cx, t State, labels []RowLabel) {
	tc := rt.CheckCx(cx)
	o := rt.NewReq(rt.CTableRowLabels)
	o.U64(rt.H(t.HCore))
	o.U32(uint32(len(labels)))
	for _, l := range labels {
		if l.Row < 0 {
			rt.Stop("table.SetTableRowLabels was given a negative row (%d)", l.Row)
		}
		o.U64(uint64(l.Row))
		o.Str(l.Label)
	}
	tc.DeferOp(*o)
}

// SetTableLook sets how a table draws besides its cells. A buffered write; the styles are values
// made in this handler or init (gk.NewStyleRefinementDefault()...).
func SetTableLook(cx *gk.Cx, t State, look Look) {
	tc := rt.CheckCx(cx)
	o := rt.NewReq(rt.CTableLook)
	o.U64(rt.H(t.HCore))
	o.U32(uint32(len(look.Fit)))
	for _, f := range look.Fit {
		if f.Col < 0 {
			rt.Stop("table.SetTableLook was given a negative column (%d)", f.Col)
		}
		o.U32(uint32(f.Col))
		o.F32(f.Width)
	}
	o.Bool(look.Groups != nil)
	o.U32(uint32(len(look.Groups)))
	for _, level := range look.Groups {
		o.U32(uint32(len(level)))
		for _, g := range level {
			if g.Span < 0 {
				rt.Stop("table.SetTableLook was given a negative span (%d)", g.Span)
			}
			o.Str(g.Label)
			o.U64(uint64(g.Span))
		}
	}
	o.U32(uint32(len(look.GroupViews)))
	for _, g := range look.GroupViews {
		o.Str(g.Label)
		o.U64(rt.H(g.View.HCore))
	}
	o.U64(rt.H(look.LastColumn.HCore))
	for _, st := range [2]gk.Option[gk.StyleRefinement]{look.HeaderStyle, look.RowStyle} {
		if s, ok := st.Get(); ok {
			rt.PutVtape(o, rt.N(s.Core))
		} else {
			o.U32(0)
		}
	}
	tc.DeferOp(*o)
}
