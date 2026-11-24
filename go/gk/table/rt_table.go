// Hand-written virtualized table and tree helpers: cell-packing table sources,
// column metadata and kit Column builders, flattened tree items, text exports,
// row labels, and grouped-header Look overrides.

package table

import (
	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/internal/rt"
)

// EventKind identifies table delegate callbacks emitted for sorting, column
// reordering, viewport visibility changes, and infinite-scroll pagination.
type EventKind uint16

// Event kinds reported in DelegateEvent.Kind.
const (
	Sort EventKind = iota
	MoveColumn
	VisibleRows
	VisibleColumns
	LoadMore
)

// String returns the name of table event kind k.
func (k EventKind) String() string {
	return [...]string{"Sort", "MoveColumn", "VisibleRows", "VisibleColumns", "LoadMore", "?"}[min(int(k), 5)]
}

// DelegateEvent carries a table interaction payload (sort column/direction,
// moved column indices, visible row/column bounds, or load-more trigger).
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

// Events attaches a DelegateEvent handler to a table RowSource.
func Events(f func(cx *gk.Cx, e DelegateEvent)) gk.SourceOption {
	return rt.SourceEvents(f, rt.ShapeTableEvent)
}

// TableColumn defines a table column's key, heading, alignment, sort state,
// width constraints, pinning, and interactive flags.
type TableColumn struct {
	Key, Name  string
	Align      gk.TextAlign
	Sort       gk.Option[ColumnSort]
	Paddings   gk.Option[[4]float32]
	Width      float32
	Fixed      gk.Option[ColumnFixed]
	Resizable  bool
	Movable    bool
	Selectable bool
	MinWidth   float32
	MaxWidth   float32
}

// NewTableColumn creates a TableColumn with sensible default width (100 px),
// min/max bounds (20..1200 px), and resizing, moving, and selection enabled.
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

// CellAt identifies one cell by zero-based Row and Col index in a table source.
type CellAt struct{ Row, Col int }

// NewSource creates a virtualized table RowSource with rows rows and columns
// columns. Each row view packs all columns for that row into a single
// rt.TableCells node, keeping the per-row view count independent of column count.
func NewSource(cx *gk.Cx, rows int, columns []TableColumn, cell func(cx *gk.Cx, at CellAt) gk.Element, opts ...gk.SourceOption) gk.RowSource {
	if cell == nil {
		rt.Stop("table.NewSource needs a cell function")
	}
	so := rt.SourceOpts{Initial: 160, Capacity: 4096}
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
		return rt.TableCells(rec.Cols, func(col int) gk.Element {
			return cell(cx, CellAt{at.Row, col})
		})
	}
	rowCount, initRows := rows, so.Initial
	if n == 0 {
		rowCount, initRows = 0, 0
	}
	capRows := max(so.Capacity/max(n, 1), 64)
	all := append(opts[:len(opts):len(opts)], gk.Initial(initRows), gk.Capacity(capRows))
	src := gk.NewRowSource(cx, rowCount, row, all...)
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

// SetTableRows updates the row count of a table RowSource created by NewSource.
func SetTableRows(cx *gk.Cx, src gk.RowSource, rows int) {
	rec := tableRecOf(src)
	if rows < 0 {
		rt.Stop("table.SetTableRows was given a negative count (%d)", rows)
	}
	if rec.Cols > 0 && rows > gk.MaxRows/rec.Cols {
		rt.Stop("table.SetTableRows was given %d rows of %d columns; a table holds at most %d cells (rows times columns)", rows, rec.Cols, gk.MaxRows)
	}
	if rec.Cols > 0 {
		gk.SetCount(cx, src, rows)
	}
	rec.Rows = rows
}

// InvalidateTable marks cached table rows in [lo, hi) dirty so they re-render.
func InvalidateTable(cx *gk.Cx, src gk.RowSource, lo, hi int) {
	tableRecOf(src)
	gk.Invalidate(cx, src, max(lo, 0), max(hi, 0))
}

// SetTableColumns replaces the column definitions on table State t and its
// backing RowSource src. When the column count changes, existing row views are
// cleared first so they re-materialize with the new column count.
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
		if n > 0 {
			gk.SetCount(cx, src, rec.Rows)
		}
	}
}

// SetTableKitColumns replaces the columns of src (and optionally live table
// State t) using generated kit Column builder values instead of TableColumn structs.
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
		if n > 0 {
			gk.SetCount(cx, src, rec.Rows)
		}
	}
}

// TreeNode describes one flattened node supplied to SetTreeItems, where Depth
// encodes nesting level.
type TreeNode struct {
	ID, Label string
	Depth     int
	Expanded  bool
	Disabled  bool
}

// SetTreeItems replaces the hierarchical item list stored in TreeState t.
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

// TreeEntry describes one visible flattened entry read back from a TreeState.
type TreeEntry struct {
	ID, Label string
	Depth     int
	Folder    bool
	Expanded  bool
	Disabled  bool
}

// TreeEntryAt reads the visible tree entry at flat index ix, returning false
// if ix is out of range.
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

// NewTreeSource creates a RowSource for a tree view whose visible row count is
// managed by the TreeState itself.
func NewTreeSource(cx *gk.Cx, row func(cx *gk.Cx, at gk.RowAt) ListItemEl, opts ...gk.SourceOption) gk.RowSource {
	if row == nil {
		rt.Stop("table.NewTreeSource needs a row function")
	}
	return gk.NewRowSource(cx, 1<<30, func(cx *gk.Cx, at gk.RowAt) gk.Element { return row(cx, at) }, opts...)
}

// RowLabel overrides the text label shown for a specific row in a table.
type RowLabel struct {
	Row   int
	Label string
}

// Group defines a grouped column header spanning Span columns.
type Group struct {
	Label string
	Span  int
}

// GroupView attaches a custom header gk.View to a named column group.
type GroupView struct {
	Label string
	View  gk.View
}

// Fit sets an explicit auto-fit pixel width for column Col.
type Fit struct {
	Col   int
	Width float32
}

// Look configures optional visual overrides on a table State via SetTableLook:
// fitted column widths, multi-level column header groups, custom group views,
// trailing column view, and header/row style refinements.
type Look struct {
	Fit         []Fit
	Groups      [][]Group
	GroupViews  []GroupView
	LastColumn  gk.View
	HeaderStyle gk.Option[gk.StyleRefinement]
	RowStyle    gk.Option[gk.StyleRefinement]
}

// SetTableTexts sets plain-text cell values for copy/export/search starting at
// firstRow; when replace is true, any previous table texts are cleared first.
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

// SetTableRowLabels installs custom per-row labels on table State t.
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

// SetTableLook applies Look overrides (column fit widths, grouped headers,
// trailing column view, and header/row style refinements) to table State t.
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
