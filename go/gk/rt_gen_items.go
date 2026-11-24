package gk

// Item functions and asks (ported from odin/gk/rt_items.odin, rt_source.odin's row_context and
// kit_text.odin; docs/PROTOCOLS.md 9.14, 9.16, 9.22, 9.41).
//
// Item functions: some components draw one part per item through a closure upstream calls while it
// draws (a calendar's days, a tree's entries, a popover's content, an input's tokens, a diff's
// headers). The app gives a typed function that describes one item from the values the kit hands
// it; gk.Items makes the row source the builder takes:
//
//	func (a *App) day(cx *gk.Cx, item gk.CalendarItemEl, st gk.CalendarItemState) gk.Element {
//		return item.RoundedFull()
//	}
//	gk.Calendar(a.cal).Item(gk.Items(cx, a.day))          in a view function
//
// The function runs between frames, like a row function: each item drawn in a frame is a row,
// described when it is first drawn and again when the kit hands it other values. gk.Items takes
// only a function of a kind some closure of the kit calls (gen_items.go's ItemFunc, checked by the
// compiler); one given to a closure of another kind stops the app where it is given.
//
// Asks: a closure upstream calls while it draws and that returns a value (a code block's
// highlights, an image for a URI, a calendar label) is answered from a cache keyed by its
// arguments; a key not there is asked of the app between frames:
//
//	a.hl = gk.NewAsk(cx, a.highlight)    // func (a *App) highlight(cx *gk.Cx, b gk.CodeBlock) []gk.TupleRangeHighlightStyle
//	gk.TextView(..).CodeBlockHighlighter(a.hl)
//	gk.Release(cx, a.hl.Src)

import "reflect"

type itemsKey struct {
	view uint64
	fn   uintptr
}

type itemsRec struct {
	f    any // the app's function, the latest given for this key
	kind int
}

var (
	itemsSources = map[itemsKey]RowSource{}
	itemsOf      = map[uint64]*itemsRec{} // row source handle -> its item function
)

// itemsSource is the row source of one item function, made on first use (no rows, at most 512
// row views). The same function given in the same view gives the same source; the function last
// given is the one called (a closure's captures stay current).
func itemsSource(cx *Cx, f any, kind int, tramp func(f any, cx *Cx, src RowSource, at RowAt) Element) RowSource {
	t := checkCx(cx)
	if isNilFunc(f) {
		stop("gk.Items was given a nil function")
	}
	key := itemsKey{fn: reflect.ValueOf(f).Pointer()}
	if vb.active && vb.view != nil {
		key.view = vb.view.h
	} else if t.owner != nil {
		key.view = t.owner.h
	}
	if src, ok := itemsSources[key]; ok {
		if rec := itemsOf[src.h]; rec != nil && sources[src.h] != nil {
			rec.f = f
			return src
		}
	}
	rec := &itemsRec{f: f, kind: kind}
	var src RowSource
	row := func(cx *Cx, at RowAt) Element { return tramp(rec.f, cx, src, at) }
	active := vb.active
	vb.active = false // an item function's source may be made in a view function (Odin's new_source)
	src = NewRowSource(cx, 0, row, Initial(0), Capacity(512))
	vb.active = active
	itemsSources[key] = src
	itemsOf[src.h] = rec
	return src
}

// itemsCheck is the source a builder of an item closure is given, checked against the kind of
// function that closure calls. A source gk.NewRowSource made passes.
func itemsCheck(src RowSource, kind int) uint64 {
	if rec := itemsOf[src.h]; rec != nil && rec.kind != kind && sources[src.h] != nil {
		stop("gk.Items was given a function of the kind %s, but this closure calls one of the kind %s",
			itemsKinds[rec.kind], itemsKinds[kind])
	}
	return src.h
}

// rowContext reads what the element closure over a context last handed row at of src (CTX_GET).
func rowContext(cx *Cx, src RowSource, at RowAt) *reader {
	checkCx(cx)
	o := newReq(cCtxGet)
	o.u64(src.h)
	o.u32(uint32(at.Section))
	o.u32(uint32(at.Row))
	return &reader{b: mustCall(*o)}
}

// KitView is a view the kit hands an item function (a tooltip's content): place it as a child,
// once, in the element the function returns.
type KitView struct{ n *node }

func (e KitView) node() *node { return e.n }

func kitView(index int) KitView {
	n := newNode(nKitView)
	nodeArgs(n).u8(uint8(index))
	return KitView{n}
}

// --- asks (PROTOCOLS.md 9.14) ---------------------------------------------------------------------

// Ask is a data closure's function, kept by the library with its cache of answers: K the closure's
// arguments (one argument, or a Tuple of them), V its result. gk.Release(cx, a.Src) ends it.
type Ask[K, V any] struct {
	Src RowSource
}

type asker interface {
	serve(cx *Cx, r *reader)
}

type askRec[K, V any] struct {
	f func(cx *Cx, key K) V
}

type askCodec struct {
	rd  func(r *reader) any
	put func(o *tape, v any)
}

// askCodecs: the wire forms of each ask's key and value, by the Ask type (gen_items.go).
var askCodecs = map[reflect.Type]askCodec{}

func regAsk[K, V any](rd func(r *reader) K, put func(o *tape, v V)) {
	askCodecs[reflect.TypeFor[Ask[K, V]]()] = askCodec{func(r *reader) any { return rd(r) }, func(o *tape, v any) { put(o, v.(V)) }}
}

func (a *askRec[K, V]) serve(cx *Cx, r *reader) {
	Quiet(cx) // no redraw of the view that made it: the library redraws the windows when the answer is in
	id := r.u64()
	src := RowSource{r.u64()}
	c := askCodecs[reflect.TypeFor[Ask[K, V]]()]
	k := c.rd(r).(K)
	v := a.f(cx, k)
	var body tape
	c.put(&body, v)
	o := newReq(cSourceAnswer)
	o.u64(src.h)
	o.u64(id)
	o.bytes(body)
	checkCx(cx).deferOp(*o)
}

func init() {
	registerShape(shapeAskEvent, shapeInfo{"gk.Ask", func(h any, cx *Cx, r *reader, ret *tape) bool {
		a, ok := h.(asker)
		if !ok {
			return false
		}
		a.serve(cx, r)
		return true
	}})
}

// NewAsk keeps f as a data closure's function: the library asks it for each key it draws without
// an answer, between frames, and caches what it returns. Needs a window: call it in init or in a
// handler, and keep the gk.Ask. The key and value types are the closure's: an ask of other types
// does not fit the builder, and one of types no closure of the kit has stops here.
func NewAsk[K, V any](cx *Cx, f func(cx *Cx, key K) V) Ask[K, V] {
	checkCx(cx)
	if vb.active {
		stop("a view function may not make an ask: make it in init or in a handler and keep the gk.Ask")
	}
	if f == nil {
		stop("gk.NewAsk needs a function")
	}
	if _, ok := askCodecs[reflect.TypeFor[Ask[K, V]]()]; !ok {
		stop("no closure of the kit asks a func(cx *gk.Cx, key %v) %v: give a function of the types the builder's gk.Ask names",
			reflect.TypeFor[K](), reflect.TypeFor[V]())
	}
	none := func(cx *Cx, at RowAt) Element { return nil }
	src := NewRowSource(cx, 0, none, Initial(0), Capacity(16), sourceEvents(&askRec[K, V]{f}, shapeAskEvent))
	checkCx(cx).flush()
	o := newReq(cAskNew)
	o.u64(src.h)
	mustCall(*o)
	return Ask[K, V]{src}
}

// TestAsks is the library's counts of data closure answers (gets, keys asked, answers cached).
func TestAsks(cx *Cx) (gets, asked, answered int) {
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cAskTest))}
	return int(r.u64()), int(r.u64()), int(r.u64())
}

// TestLends is the library's counts of objects lent by kit state and given back (PROTOCOLS.md 9.17).
func TestLends(cx *Cx) (lent, givenBack int) {
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cLendTest))}
	return int(r.u64()), int(r.u64())
}
