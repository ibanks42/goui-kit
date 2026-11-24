package rt

// RowAt is the row a row view describes: its section (0 without sections) and its row.
type RowAt struct{ Section, Row int }

// RowSource is a handle to a row source, which lives in the library. The kit's list, table,
// tree and select states take one.
type RowSource struct{ HCore }

// SourceTest is the event of the row sources' own test request.
type SourceTest struct {
	ID   uint64
	Text string
}

type sourceRec struct {
	h       uint64
	row     func(*Cx, RowAt) Element
	rows    map[uint64]uint64 // row key to row view
	events  uint32            // kept handler slot + 1; 0 for none
	count   int
	countFn func() int
}

var (
	sources  = map[uint64]*sourceRec{}
	deadRows []*viewRec
)

// MaxRows is the most rows a section of a row source holds.
const MaxRows = 0xffff_fffe

func checkCount(count int, what string) {
	if count < 0 {
		Stop("%s was given a negative count (%d): give the number of rows, 0 or more", what, count)
	}
	if count > MaxRows {
		Stop("%s was given %d rows; a row source holds at most %d in a section", what, count, MaxRows)
	}
}

func rowKey(at RowAt) uint64 { return uint64(at.Section)<<32 | uint64(uint32(at.Row)) }

// SourceOption changes a row source made by NewRowSource.
type SourceOption func(*SourceOpts)

type SourceOpts struct {
	Initial, Capacity int
	events            any
	shape             uint16
	countFn           func() int
}

// Initial is how many rows get their views at once, so the first frame that draws the source is
// complete (default 64).
func Initial(n int) SourceOption { return func(o *SourceOpts) { o.Initial = n } }

// Capacity is how many row views are kept; far ones are evicted (default 512).
func Capacity(n int) SourceOption { return func(o *SourceOpts) { o.Capacity = n } }

// CountFunc gives the source a count function: gk.Refresh takes the number of rows from it.
func CountFunc(f func() int) SourceOption { return func(o *SourceOpts) { o.countFn = f } }

// sourceEvents gives the source an events handler of a family's event shape (the delegate
// families and the runtime's tests).
func SourceEvents(h any, shape uint16) SourceOption {
	return func(o *SourceOpts) { o.events, o.shape = h, shape }
}

// TestSourceEvents is sourceEvents for the runtime's tests: the source's test requests.
func TestSourceEvents(h func(cx *Cx, e SourceTest)) SourceOption {
	return SourceEvents(h, ShapeSourceTest)
}

// NewRowSource makes a source of count rows (one section) whose rows row describes. Needs a
// window: call it in a view's init function or in a handler.
func NewRowSource(cx *Cx, count int, row func(cx *Cx, at RowAt) Element, opts ...SourceOption) RowSource {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not create row sources: create them in init or in a handler and keep the gk.RowSource")
	}
	if row == nil {
		Stop("gk.NewRowSource needs a row function")
	}
	so := SourceOpts{Initial: 64, Capacity: 512}
	for _, o := range opts {
		o(&so)
	}
	if count < 0 || so.Initial < 0 || so.Capacity < 0 {
		Stop("a negative number was given to gk.NewRowSource (count %d, initial %d, capacity %d)", count, so.Initial, so.Capacity)
	}
	checkCount(count, "gk.NewRowSource")
	var slot, gen uint32
	has := so.events != nil
	if has {
		slot, gen = KeepHandler(cx, so.events, so.shape)
	}
	t.Flush()
	o := NewReq(CSourceNew)
	o.U64(uint64(count))
	o.U32(uint32(so.Initial))
	o.U32(uint32(so.Capacity))
	o.Bool(has)
	o.U32(slot)
	o.U32(gen)
	r := Reader{B: MustCall(*o)}
	rec := &sourceRec{h: r.U64(), row: row, rows: map[uint64]uint64{}, count: count, countFn: so.countFn}
	if has {
		rec.events = slot + 1
	}
	sources[rec.h] = rec
	n := r.U32()
	for range n {
		key := r.U64()
		bindRow(rec, key, r.U64())
	}
	return RowSource{MkH(rec.h)}
}

func sourceRecOf(src RowSource) *sourceRec {
	rec := sources[src.h]
	if rec == nil {
		Stop("this row source was released (or never made): release a list's source only after nothing shows it, or make a new one with gk.NewRowSource")
	}
	return rec
}

// SetCount changes the number of rows of section 0. Rows at and past the new count are dropped;
// rows below it keep their views (invalidate them if their content moved).
func SetCount(cx *Cx, src RowSource, count int) { SetSectionCount(cx, src, 0, count) }

// SetSectionCount is SetCount for one section.
func SetSectionCount(cx *Cx, src RowSource, section, count int) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not write: it only describes (gk.SetCount). Change rows in a handler, in init or in a frame handler")
	}
	rec := sourceRecOf(src)
	checkCount(count, "gk.SetCount")
	if section == 0 {
		rec.count = count
	}
	lo := rowKey(RowAt{section, count})
	hi := uint64(section+1) << 32
	var gone []uint64
	for key, view := range rec.rows {
		if key >= lo && key < hi {
			gone = append(gone, view)
		}
	}
	for _, view := range gone {
		unbindRow(view)
	}
	o := NewReq(CSourceSetCount)
	o.U64(src.h)
	o.U32(uint32(section))
	o.U64(uint64(count))
	t.DeferOp(*o)
}

// Invalidate has rows lo to hi (not included) of section 0 described again before the next frame.
func Invalidate(cx *Cx, src RowSource, lo, hi int) {
	t := CheckCx(cx)
	rec := sourceRecOf(src)
	if Vb.Active {
		Stop("gk.Invalidate in a view function: a view function only describes. Call it in the handler that changed the rows (or call gk.Refresh there)")
	}
	a, b := rowKey(RowAt{0, max(lo, 0)}), rowKey(RowAt{0, max(hi, 0)})
	for key, view := range rec.rows {
		if key >= a && key < b {
			t.markDirty(view)
		}
	}
}

// Refresh is for after the app's data changed: every view is described again before the next
// frame, and so is every row a list, table or tree shows; a source made with CountFunc first
// takes its number of rows from it. A description that did not change costs a compare.
func Refresh(cx *Cx) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("gk.Refresh in a view function: a view function only describes. Call it at the end of the handler that changed the data")
	}
	for h, rec := range sources {
		if rec.countFn != nil {
			if n := rec.countFn(); n != rec.count {
				SetCount(cx, RowSource{MkH(h)}, n)
			}
		}
	}
	for _, v := range views.slots {
		if v != nil && !v.dead {
			t.markDirty(v.h)
		}
	}
}

// Answer answers a request a row source made (SourceTest's id) with a string.
func Answer(cx *Cx, src RowSource, id uint64, text string) {
	t := CheckCx(cx)
	var body Tape
	body.Str(text)
	o := NewReq(CSourceAnswer)
	o.U64(src.h)
	o.U64(id)
	o.Bytes(body)
	t.DeferOp(*o)
}

// ScrollTo scrolls the core list of a source so that row is at the top. A buffered write.
func ScrollTo(cx *Cx, src RowSource, row int) {
	o := NewReq(CSourceScrollTo)
	o.U64(src.h)
	o.U64(uint64(max(row, 0)))
	CheckCx(cx).DeferOp(*o)
}

// rowRecs is the block row views are taken from: a ROWS turn binds a few hundred at a time.
var rowRecs []viewRec

func bindRow(rec *sourceRec, key, view uint64) {
	if len(rowRecs) == cap(rowRecs) {
		rowRecs = make([]viewRec, 0, 128)
	}
	rowRecs = append(rowRecs, viewRec{h: view, row: rec.row, rowAt: RowAt{int(key >> 32), int(key & 0xffff_ffff)}, src: rec.h})
	v := &rowRecs[len(rowRecs)-1]
	views.set(view, v)
	rec.rows[key] = view
}

func unbindRow(view uint64) {
	v := views.get(view)
	if v == nil {
		return
	}
	views.del(view)
	if rec := sources[v.src]; rec != nil {
		delete(rec.rows, rowKey(v.rowAt))
	}
	v.dead = true
	deadRows = append(deadRows, v)
}

// rowsTurn is the ROWS turn: bind the rows the library asks for, forget the ones it evicted. No
// app code runs in it; the new rows are described in the VIEWS turn that follows.
func rowsTurn(r *Reader) {
	nSources := r.U32()
	for range nSources {
		rec := sources[r.U64()] // nil if released meanwhile: its rows are dropped
		nNew := r.U32()
		for range nNew {
			key := r.U64()
			view := r.U64()
			if rec != nil {
				bindRow(rec, key, view)
			}
		}
		nGone := r.U32()
		for range nGone {
			unbindRow(r.U64())
		}
	}
}

func releaseSource(t *turn, src RowSource) {
	rec := sourceRecOf(src)
	for _, view := range rec.rows {
		unbindRow(view)
	}
	if rec.events != 0 {
		dropHandler(rec.events - 1)
	}
	delete(sources, src.h)
	delete(Tables, src.h)
	delete(searches, src.h)
	releaseHandle(t, src.h)
}

func sweepRows() {
	for _, v := range deadRows {
		v.cur, v.old, v.last = nil, nil, nil
	}
	deadRows = deadRows[:0]
}

// TestSourceRequest has the library ask the source's events handler a question (SourceTest).
func TestSourceRequest(cx *Cx, src RowSource, text string) {
	CheckCx(cx).Flush()
	o := NewReq(CTestSourceRequest)
	o.U64(src.h)
	o.Str(text)
	MustCall(*o)
}

// TestSourceTake returns the answer to TestSourceRequest once it has arrived.
func TestSourceTake(cx *Cx, src RowSource) (string, bool) {
	CheckCx(cx).Flush()
	o := NewReq(CTestSourceTake)
	o.U64(src.h)
	r := Reader{B: MustCall(*o)}
	if !r.Bool() {
		return "", false
	}
	return r.Str(), true
}

// TestSourceRows is the number of rows of a source that have a view now.
func TestSourceRows(src RowSource) int {
	if rec := sources[src.h]; rec != nil {
		return len(rec.rows)
	}
	return 0
}

// Rows is a handle to the rows of a list kept in the library (strings only), for UniformList.
type Rows struct{ HCore }

// NewRows makes an empty list of rows in the library.
func NewRows(cx *Cx) Rows { return Rows{MkH(StateMake(cx, NewReq(CRowsNew)))} }

// Push appends rows: one buffered op however many rows.
func (r Rows) Push(cx *Cx, rows ...string) {
	o := NewReq(CRowsPush)
	o.U64(r.h)
	o.U32(uint32(len(rows)))
	for _, s := range rows {
		o.Str(s)
	}
	StateSet(cx, o)
}

// UniformListEl is a list of equal-height rows (GPUI's uniform_list over Rows, or the runtime's
// list over a RowSource).
type UniformListEl struct{ ElCore }

// Style adds style tokens.
func (e UniformListEl) Style(st ...Style) UniformListEl { Tokens(e.n, st); return e }

// W sets the width.
func (e UniformListEl) W(l Length) UniformListEl { tStyledW(e.n, l); return e }

// H sets the height.
func (e UniformListEl) H(l Length) UniformListEl { tStyledH(e.n, l); return e }

// UniformList is GPUI's uniform_list over rows kept in the library.
func UniformList(id string, rows Rows, st ...Style) UniformListEl {
	n := NewNode(NUniformList)
	o := NodeArgs(n)
	o.Str(id)
	o.U64(rows.h)
	Tokens(n, st)
	return UniformListEl{MkElCore(n)}
}

// SourceList is the shared core's own list: a uniform list over a row source whose rows are
// rowHeight high (the kit's lists, tables and trees draw sources too).
func SourceList(id string, src RowSource, rowHeight float32, st ...Style) UniformListEl {
	n := NewNode(NSourceList)
	o := NodeArgs(n)
	o.Str(id)
	o.U64(src.h)
	o.F32(rowHeight)
	Tokens(n, st)
	return UniformListEl{MkElCore(n)}
}

// Text is a string as an element, for a builder that takes any element where a Rust app would
// pass a string. For a child, the parent's own Text method is shorter.
func Text(s string) Element {
	n := NewNode(NText)
	NodeArgs(n).Str(s)
	return textEl{MkElCore(n)}
}

type textEl struct{ ElCore }

// VirtualListEl is gpui-kit's virtual list over a row source (docs/PROTOCOLS.md 13.31): rows of
// different sizes, each a view described by the source's row function, only those in view drawn.
type VirtualListEl struct{ ElCore }

// Style adds style tokens (s.SizeFull, s.H64).
func (e VirtualListEl) Style(st ...Style) VirtualListEl { Tokens(e.n, st); return e }

// W sets the width.
func (e VirtualListEl) W(l Length) VirtualListEl { tStyledW(e.n, l); return e }

// H sets the height.
func (e VirtualListEl) H(l Length) VirtualListEl { tStyledH(e.n, l); return e }

// VVirtualList lays a source's rows out top to bottom, row i sizes[i][1] high (the width is the
// list's). One size per row: rows beyond the sizes are not drawn, sizes beyond the rows neither. A
// row without a view yet is a gap of its size for one frame (the rows protocol).
//
//	gk.VVirtualList("log", a.rows, a.heights, s.SizeFull)
func VVirtualList(id string, src RowSource, sizes [][2]float32, st ...Style) VirtualListEl {
	return virtualList(id, src, 0, sizes, st)
}

// HVirtualList lays a source's rows out left to right, row i sizes[i][0] wide.
func HVirtualList(id string, src RowSource, sizes [][2]float32, st ...Style) VirtualListEl {
	return virtualList(id, src, 1, sizes, st)
}

func virtualList(id string, src RowSource, axis uint8, sizes [][2]float32, st []Style) VirtualListEl {
	if src.h == 0 {
		Stop("a virtual list needs a row source (gk.NewRowSource)")
	}
	n := NewNode(NVirtualList)
	o := NodeArgs(n)
	o.Str(id)
	o.U64(src.h)
	o.U8(axis)
	o.U32(uint32(len(sizes)))
	for _, s := range sizes {
		o.F32(s[0])
		o.F32(s[1])
	}
	Tokens(n, st)
	return VirtualListEl{MkElCore(n)}
}
