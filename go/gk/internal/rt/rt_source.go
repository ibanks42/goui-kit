package rt

// RowAt identifies a single item inside a sectioned RowSource.
type RowAt struct{ Section, Row int }

// RowSource is a handle to a virtualized row/item provider whose visible rows
// are materialized on demand as child views.
type RowSource struct{ HCore }

// SourceTest is a diagnostic request payload used to test RowSource event delivery.
type SourceTest struct {
	ID   uint64
	Text string
}

type sourceRec struct {
	h       uint64
	row     func(*Cx, RowAt) Element
	rows    map[uint64]uint64
	events  uint32 // slot + 1 when an event handler is attached, 0 otherwise
	count   int
	countFn func() int
}

var (
	sources  = map[uint64]*sourceRec{}
	deadRows []*viewRec
)

// MaxRows is the maximum number of rows supported in a single section.
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

// SourceOption configures optional behavior on NewRowSource.
type SourceOption func(*SourceOpts)

// SourceOpts holds configuration values for NewRowSource.
type SourceOpts struct {
	Initial, Capacity int
	events            any
	shape             uint16
	countFn           func() int
}

// Initial sets the number of row views pre-allocated when the source is created.
func Initial(n int) SourceOption { return func(o *SourceOpts) { o.Initial = n } }

// Capacity sets the maximum number of cached row views retained by the source.
func Capacity(n int) SourceOption { return func(o *SourceOpts) { o.Capacity = n } }

// CountFunc registers a function consulted by Refresh to sync section 0's row count.
func CountFunc(f func() int) SourceOption { return func(o *SourceOpts) { o.countFn = f } }

// SourceEvents attaches a long-lived event callback of the given wire shape to
// a RowSource.
func SourceEvents(h any, shape uint16) SourceOption {
	return func(o *SourceOpts) { o.events, o.shape = h, shape }
}

// TestSourceEvents attaches a diagnostic SourceTest event handler to a RowSource.
func TestSourceEvents(h func(cx *Cx, e SourceTest)) SourceOption {
	return SourceEvents(h, ShapeSourceTest)
}

// NewRowSource creates a virtualized RowSource with count rows in section 0 and
// row callback invoked on demand for each visible row.
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
	own(rec.h, ownSource)
	n := r.U32()
	for range n {
		key := r.U64()
		bindRow(rec, key, r.U64(), t.window)
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

// SetCount updates the row count of section 0 in src.
func SetCount(cx *Cx, src RowSource, count int) { SetSectionCount(cx, src, 0, count) }

// SetSectionCount updates the row count of section in src and unbinds any
// cached row views at or beyond count.
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

// Invalidate marks cached row views in section 0 with indices in [lo, hi) dirty
// so they are re-described on the next frame.
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

// Refresh syncs every RowSource that has a CountFunc and marks every live view
// dirty in the current turn.
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

// Answer sends a string reply for diagnostic request id on src.
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

// ScrollTo scrolls a list backed by src to row.
func ScrollTo(cx *Cx, src RowSource, row int) {
	o := NewReq(CSourceScrollTo)
	o.U64(src.h)
	o.U64(uint64(max(row, 0)))
	CheckCx(cx).DeferOp(*o)
}

var (
	rowRecs   []viewRec
	rowsFresh []uint64
)

func bindRow(rec *sourceRec, key, view, window uint64) {
	if len(rowRecs) == cap(rowRecs) {
		rowRecs = make([]viewRec, 0, 128)
	}
	rowRecs = append(rowRecs, viewRec{h: view, window: window, row: rec.row, rowAt: RowAt{int(key >> 32), int(key & 0xffff_ffff)}, src: rec.h})
	v := &rowRecs[len(rowRecs)-1]
	views.set(view, v)
	rec.rows[key] = view
}

// unbindRow removes a materialized row view from the view table and defers
// clearing its handler slices until endTurn via deadRows so an in-progress
// event turn does not lose references prematurely.
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

// rowsTurn processes a KRows batch from the Rust core, binding newly visible
// row views, unbinding evicted rows, and returning the newly bound handles.
func rowsTurn(r *Reader, window uint64) []uint64 {
	var fresh []uint64
	if depth == 1 {
		fresh = rowsFresh[:0]
	}
	nSources := r.U32()
	for range nSources {
		rec := sources[r.U64()]
		nNew := r.U32()
		for range nNew {
			key := r.U64()
			view := r.U64()
			if rec != nil {
				bindRow(rec, key, view, window)
				fresh = append(fresh, view)
			}
		}
		nGone := r.U32()
		for range nGone {
			unbindRow(r.U64())
		}
	}
	if depth == 1 {
		rowsFresh = fresh
	}
	return fresh
}

func releaseSource(t *turn, src RowSource) {
	rec := sourceRecOf(src)
	disown(src.h)
	releaseOwned(t, src.h)
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

// TestSourceRequest sends a synthetic request string to src in diagnostic tests.
func TestSourceRequest(cx *Cx, src RowSource, text string) {
	CheckCx(cx).Flush()
	o := NewReq(CTestSourceRequest)
	o.U64(src.h)
	o.Str(text)
	MustCall(*o)
}

// TestSourceTake reads and consumes the last answer recorded for src in tests.
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

// TestSourceRows returns the number of currently bound row views on src.
func TestSourceRows(src RowSource) int {
	if rec := sources[src.h]; rec != nil {
		return len(rec.rows)
	}
	return 0
}

// Rows is a handle to a plain string list backing UniformList.
type Rows struct{ HCore }

// NewRows allocates an empty Rows store on the Rust side.
func NewRows(cx *Cx) Rows { return Rows{MkH(StateMake(cx, NewReq(CRowsNew)))} }

// Push appends one or more strings to r. When not in debug mode, it encodes
// the op directly into t.deferred to avoid an intermediate allocation.
func (r Rows) Push(cx *Cx, rows ...string) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not write: it only describes what the view shows. Change kit state in a handler, in init or in a frame handler")
	}
	if debug {
		o := NewReq(CRowsPush)
		o.U64(r.h)
		o.U32(uint32(len(rows)))
		for _, s := range rows {
			o.Str(s)
		}
		MustCall(*o)
		return
	}
	at := len(t.deferred)
	t.deferred.U32(0)
	t.deferred.U16(CRowsPush)
	t.deferred.U64(r.h)
	t.deferred.U32(uint32(len(rows)))
	for _, s := range rows {
		t.deferred.Str(s)
	}
	le.PutUint32(t.deferred[at:], uint32(len(t.deferred)-at-4))
	t.nDefer++
}

// UniformListEl is the element handle returned by UniformList and SourceList.
type UniformListEl struct{ ElCore }

// Style applies nullary style tokens to e.
func (e UniformListEl) Style(st ...Style) UniformListEl { Tokens(e.n, st); return e }

// W sets the width of e.
func (e UniformListEl) W(l Length) UniformListEl { tStyledW(e.n, l); return e }

// H sets the height of e.
func (e UniformListEl) H(l Length) UniformListEl { tStyledH(e.n, l); return e }

// UniformList creates a fixed-height scrollable list backed by string store rows.
func UniformList(id string, rows Rows, st ...Style) UniformListEl {
	n := NewNode(NUniformList)
	o := NodeArgs(n)
	o.Str(id)
	o.U64(rows.h)
	Tokens(n, st)
	return UniformListEl{MkElCore(n)}
}

// SourceList creates a fixed-row-height virtualized list backed by RowSource src.
func SourceList(id string, src RowSource, rowHeight float32, st ...Style) UniformListEl {
	n := NewNode(NSourceList)
	o := NodeArgs(n)
	o.Str(id)
	o.U64(src.h)
	o.F32(rowHeight)
	Tokens(n, st)
	return UniformListEl{MkElCore(n)}
}

// Text creates a leaf text element displaying s.
func Text(s string) Element {
	n := NewNode(NText)
	NodeArgs(n).Str(s)
	return textEl{MkElCore(n)}
}

type textEl struct{ ElCore }

// TableCells builds a row node containing cols cell elements produced by cell.
func TableCells(cols int, cell func(col int) Element) Element {
	var stack [16]uint32
	var ids []uint32
	if cols <= len(stack) {
		ids = stack[:cols]
	} else {
		ids = make([]uint32, cols)
	}
	for col := range cols {
		el := cell(col)
		c := ElemNode(el)
		if !c.build().live(c) {
			Stop("%s%s", staleElement, origin(c, false))
		}
		if c.attached {
			Stop("an element was given to a second parent: an element has one place in a view. Make another one for the second place (a function that returns it, called twice)%s", origin(c, true))
		}
		c.attached = true
		ids[col] = c.idx
	}
	n := NewNode(NTableCells)
	o := NodeArgs(n)
	o.U32(uint32(cols))
	for _, id := range ids {
		o.U32(id)
	}
	return textEl{MkElCore(n)}
}

// VirtualListEl is the element handle returned by VVirtualList and HVirtualList.
type VirtualListEl struct{ ElCore }

// Style applies nullary style tokens to e.
func (e VirtualListEl) Style(st ...Style) VirtualListEl { Tokens(e.n, st); return e }

// W sets the width of e.
func (e VirtualListEl) W(l Length) VirtualListEl { tStyledW(e.n, l); return e }

// H sets the height of e.
func (e VirtualListEl) H(l Length) VirtualListEl { tStyledH(e.n, l); return e }

// VVirtualList creates a vertically scrolling variable-size virtual list backed
// by src with per-item [width, height] sizes.
func VVirtualList(id string, src RowSource, sizes [][2]float32, st ...Style) VirtualListEl {
	return virtualList(id, src, 0, sizes, st)
}

// HVirtualList creates a horizontally scrolling variable-size virtual list
// backed by src with per-item [width, height] sizes.
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
