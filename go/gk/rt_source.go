package gk

// Row sources: the shared core of the delegate protocol (docs/PROTOCOLS.md section 9). A row
// source holds the rows of a list, table or tree in the library; each row is a view of its own,
// described by the app's row function for that row, once, and retained like any view until it
// is invalidated or evicted. The library binds row views to rows before a frame (the ROWS turn):
// a row drawn as a placeholder in one frame is described before the next. App code never runs in
// the middle of a frame.
//
//	a.src = gk.NewRowSource(cx, len(a.items), a.itemRow)      in init or in a handler
//	func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) gk.Element { ... a.items[at.Row] ... }
//	gk.SetCount(cx, a.src, n)   gk.Invalidate(cx, a.src, lo, hi)   gk.Release(cx, a.src)

// RowAt is the row a row view describes: its section (0 without sections) and its row.
type RowAt struct{ Section, Row int }

// RowSource is a handle to a row source, which lives in the library. The kit's list, table,
// tree and select states take one.
type RowSource struct{ h uint64 }

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
		stop("%s was given a negative count (%d): give the number of rows, 0 or more", what, count)
	}
	if count > MaxRows {
		stop("%s was given %d rows; a row source holds at most %d in a section", what, count, MaxRows)
	}
}

func rowKey(at RowAt) uint64 { return uint64(at.Section)<<32 | uint64(uint32(at.Row)) }

// SourceOption changes a row source made by NewRowSource.
type SourceOption func(*sourceOpts)

type sourceOpts struct {
	initial, capacity int
	events            any
	shape             uint16
	countFn           func() int
}

// Initial is how many rows get their views at once, so the first frame that draws the source is
// complete (default 64).
func Initial(n int) SourceOption { return func(o *sourceOpts) { o.initial = n } }

// Capacity is how many row views are kept; far ones are evicted (default 512).
func Capacity(n int) SourceOption { return func(o *sourceOpts) { o.capacity = n } }

// CountFunc gives the source a count function: gk.Refresh takes the number of rows from it.
func CountFunc(f func() int) SourceOption { return func(o *sourceOpts) { o.countFn = f } }

// sourceEvents gives the source an events handler of a family's event shape (the delegate
// families and the runtime's tests).
func sourceEvents(h any, shape uint16) SourceOption {
	return func(o *sourceOpts) { o.events, o.shape = h, shape }
}

// TestSourceEvents is sourceEvents for the runtime's tests: the source's test requests.
func TestSourceEvents(h func(cx *Cx, e SourceTest)) SourceOption {
	return sourceEvents(h, shapeSourceTest)
}

// NewRowSource makes a source of count rows (one section) whose rows row describes. Needs a
// window: call it in a view's init function or in a handler.
func NewRowSource(cx *Cx, count int, row func(cx *Cx, at RowAt) Element, opts ...SourceOption) RowSource {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not create row sources: create them in init or in a handler and keep the gk.RowSource")
	}
	if row == nil {
		stop("gk.NewRowSource needs a row function")
	}
	so := sourceOpts{initial: 64, capacity: 512}
	for _, o := range opts {
		o(&so)
	}
	if count < 0 || so.initial < 0 || so.capacity < 0 {
		stop("a negative number was given to gk.NewRowSource (count %d, initial %d, capacity %d)", count, so.initial, so.capacity)
	}
	checkCount(count, "gk.NewRowSource")
	var slot, gen uint32
	has := so.events != nil
	if has {
		slot, gen = keepHandler(cx, so.events, so.shape)
	}
	t.flush()
	o := newReq(cSourceNew)
	o.u64(uint64(count))
	o.u32(uint32(so.initial))
	o.u32(uint32(so.capacity))
	o.bool(has)
	o.u32(slot)
	o.u32(gen)
	r := reader{b: mustCall(*o)}
	rec := &sourceRec{h: r.u64(), row: row, rows: map[uint64]uint64{}, count: count, countFn: so.countFn}
	if has {
		rec.events = slot + 1
	}
	sources[rec.h] = rec
	n := r.u32()
	for range n {
		key := r.u64()
		bindRow(rec, key, r.u64())
	}
	return RowSource{rec.h}
}

func sourceRecOf(src RowSource) *sourceRec {
	rec := sources[src.h]
	if rec == nil {
		stop("this row source was released (or never made): release a list's source only after nothing shows it, or make a new one with gk.NewRowSource")
	}
	return rec
}

// SetCount changes the number of rows of section 0. Rows at and past the new count are dropped;
// rows below it keep their views (invalidate them if their content moved).
func SetCount(cx *Cx, src RowSource, count int) { SetSectionCount(cx, src, 0, count) }

// SetSectionCount is SetCount for one section.
func SetSectionCount(cx *Cx, src RowSource, section, count int) {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not write: it only describes (gk.SetCount). Change rows in a handler, in init or in a frame handler")
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
	o := newReq(cSourceSetCount)
	o.u64(src.h)
	o.u32(uint32(section))
	o.u64(uint64(count))
	t.deferOp(*o)
}

// Invalidate has rows lo to hi (not included) of section 0 described again before the next frame.
func Invalidate(cx *Cx, src RowSource, lo, hi int) {
	t := checkCx(cx)
	rec := sourceRecOf(src)
	if vb.active {
		stop("gk.Invalidate in a view function: a view function only describes. Call it in the handler that changed the rows (or call gk.Refresh there)")
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
	t := checkCx(cx)
	if vb.active {
		stop("gk.Refresh in a view function: a view function only describes. Call it at the end of the handler that changed the data")
	}
	for h, rec := range sources {
		if rec.countFn != nil {
			if n := rec.countFn(); n != rec.count {
				SetCount(cx, RowSource{h}, n)
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
	t := checkCx(cx)
	var body tape
	body.str(text)
	o := newReq(cSourceAnswer)
	o.u64(src.h)
	o.u64(id)
	o.bytes(body)
	t.deferOp(*o)
}

// ScrollTo scrolls the core list of a source so that row is at the top. A buffered write.
func ScrollTo(cx *Cx, src RowSource, row int) {
	o := newReq(cSourceScrollTo)
	o.u64(src.h)
	o.u64(uint64(max(row, 0)))
	checkCx(cx).deferOp(*o)
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
func rowsTurn(r *reader) {
	nSources := r.u32()
	for range nSources {
		rec := sources[r.u64()] // nil if released meanwhile: its rows are dropped
		nNew := r.u32()
		for range nNew {
			key := r.u64()
			view := r.u64()
			if rec != nil {
				bindRow(rec, key, view)
			}
		}
		nGone := r.u32()
		for range nGone {
			unbindRow(r.u64())
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
	delete(tables, src.h)
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
	checkCx(cx).flush()
	o := newReq(cTestSourceRequest)
	o.u64(src.h)
	o.str(text)
	mustCall(*o)
}

// TestSourceTake returns the answer to TestSourceRequest once it has arrived.
func TestSourceTake(cx *Cx, src RowSource) (string, bool) {
	checkCx(cx).flush()
	o := newReq(cTestSourceTake)
	o.u64(src.h)
	r := reader{b: mustCall(*o)}
	if !r.bool() {
		return "", false
	}
	return r.str(), true
}

// TestSourceRows is the number of rows of a source that have a view now.
func TestSourceRows(src RowSource) int {
	if rec := sources[src.h]; rec != nil {
		return len(rec.rows)
	}
	return 0
}

// --- lists over rows kept in the library, and plain lists over a source -----------------------

// Rows is a handle to the rows of a list kept in the library (strings only), for UniformList.
type Rows struct{ h uint64 }

func (r Rows) handle() uint64 { return r.h }

// NewRows makes an empty list of rows in the library.
func NewRows(cx *Cx) Rows { return Rows{stateMake(cx, newReq(cRowsNew))} }

// Push appends rows: one buffered op however many rows.
func (r Rows) Push(cx *Cx, rows ...string) {
	o := newReq(cRowsPush)
	o.u64(r.h)
	o.u32(uint32(len(rows)))
	for _, s := range rows {
		o.str(s)
	}
	stateSet(cx, o)
}

// UniformListEl is a list of equal-height rows (GPUI's uniform_list over Rows, or the runtime's
// list over a RowSource).
type UniformListEl struct{ n *node }

func (e UniformListEl) node() *node { return e.n }

// Style adds style tokens.
func (e UniformListEl) Style(st ...Style) UniformListEl { tokens(e.n, st); return e }

// W sets the width.
func (e UniformListEl) W(l Length) UniformListEl { tStyledW(e.n, l); return e }

// H sets the height.
func (e UniformListEl) H(l Length) UniformListEl { tStyledH(e.n, l); return e }

// UniformList is GPUI's uniform_list over rows kept in the library.
func UniformList(id string, rows Rows, st ...Style) UniformListEl {
	n := newNode(nUniformList)
	o := nodeArgs(n)
	o.str(id)
	o.u64(rows.h)
	tokens(n, st)
	return UniformListEl{n}
}

// SourceList is the shared core's own list: a uniform list over a row source whose rows are
// rowHeight high (the kit's lists, tables and trees draw sources too).
func SourceList(id string, src RowSource, rowHeight float32, st ...Style) UniformListEl {
	n := newNode(nSourceList)
	o := nodeArgs(n)
	o.str(id)
	o.u64(src.h)
	o.f32(rowHeight)
	tokens(n, st)
	return UniformListEl{n}
}

// Text is a string as an element, for a builder that takes any element where a Rust app would
// pass a string. For a child, the parent's own Text method is shorter.
func Text(s string) Element {
	n := newNode(nText)
	nodeArgs(n).str(s)
	return textEl{n}
}

type textEl struct{ n *node }

func (e textEl) node() *node { return e.n }

// VirtualListEl is gpui-kit's virtual list over a row source (docs/PROTOCOLS.md 13.31): rows of
// different sizes, each a view described by the source's row function, only those in view drawn.
type VirtualListEl struct{ n *node }

func (e VirtualListEl) node() *node { return e.n }

// Style adds style tokens (s.SizeFull, s.H64).
func (e VirtualListEl) Style(st ...Style) VirtualListEl { tokens(e.n, st); return e }

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
		stop("a virtual list needs a row source (gk.NewRowSource)")
	}
	n := newNode(nVirtualList)
	o := nodeArgs(n)
	o.str(id)
	o.u64(src.h)
	o.u8(axis)
	o.u32(uint32(len(sizes)))
	for _, s := range sizes {
		o.f32(s[0])
		o.f32(s[1])
	}
	tokens(n, st)
	return VirtualListEl{n}
}
