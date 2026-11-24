package rt

import (
	"reflect"
)

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
func ItemsSource(cx *Cx, f any, kind int, tramp func(f any, cx *Cx, src RowSource, at RowAt) Element) RowSource {
	t := CheckCx(cx)
	if isNilFunc(f) {
		Stop("overlay.Items was given a nil function")
	}
	key := itemsKey{fn: reflect.ValueOf(f).Pointer()}
	if Vb.Active && Vb.view != nil {
		key.view = Vb.view.h
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
	active := Vb.Active
	Vb.Active = false // an item function's source may be made in a view function
	src = NewRowSource(cx, 0, row, Initial(0), Capacity(512))
	Vb.Active = active
	itemsSources[key] = src
	itemsOf[src.h] = rec
	return src
}

// itemsCheck is the source a builder of an item closure is given, checked against the kind of
// function that closure calls. A source gk.NewRowSource made passes.
func ItemsCheck(src RowSource, kind int) uint64 {
	if rec := itemsOf[src.h]; rec != nil && rec.kind != kind && sources[src.h] != nil {
		Stop("overlay.Items was given a function of the kind %s, but this closure calls one of the kind %s",
			ItemsKinds[rec.kind], ItemsKinds[kind])
	}
	return src.h
}

// rowContext reads what the element closure over a context last handed row at of src (CTX_GET).
func RowContext(cx *Cx, src RowSource, at RowAt) *Reader {
	CheckCx(cx)
	o := NewReq(CCtxGet)
	o.U64(src.h)
	o.U32(uint32(at.Section))
	o.U32(uint32(at.Row))
	return &Reader{B: MustCall(*o)}
}

// KitView is a view the kit hands an item function (a tooltip's content): place it as a child,
// once, in the element the function returns.
type KitView struct{ ElCore }

func XKitView(index int) KitView {
	n := NewNode(NKitView)
	NodeArgs(n).U8(uint8(index))
	return KitView{MkElCore(n)}
}

// Ask is a data closure's function, kept by the library with its cache of answers: K the closure's
// arguments (one argument, or a Tuple of them), V its result. gk.Release(cx, a.Src) ends it.
type Ask[K, V any] struct {
	Src RowSource
}

type asker interface {
	serve(cx *Cx, r *Reader)
}

type askRec[K, V any] struct {
	f func(cx *Cx, key K) V
}

type askCodec struct {
	rd  func(r *Reader) any
	put func(o *Tape, v any)
}

// askCodecs: the wire forms of each ask's key and value, by the Ask type (gen_items.go).
var askCodecs = map[reflect.Type]askCodec{}

func RegAsk[K, V any](rd func(r *Reader) K, put func(o *Tape, v V)) {
	askCodecs[reflect.TypeFor[Ask[K, V]]()] = askCodec{func(r *Reader) any { return rd(r) }, func(o *Tape, v any) { put(o, v.(V)) }}
}

func (a *askRec[K, V]) serve(cx *Cx, r *Reader) {
	Quiet(cx) // no redraw of the view that made it: the library redraws the windows when the answer is in
	id := r.U64()
	src := RowSource{MkH(r.U64())}
	c := askCodecs[reflect.TypeFor[Ask[K, V]]()]
	k := c.rd(r).(K)
	v := a.f(cx, k)
	var body Tape
	c.put(&body, v)
	o := NewReq(CSourceAnswer)
	o.U64(src.h)
	o.U64(id)
	o.Bytes(body)
	CheckCx(cx).DeferOp(*o)
}

func init() {
	RegisterShape(ShapeAskEvent, ShapeInfo{"gk.Ask", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
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
	CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not make an ask: make it in init or in a handler and keep the gk.Ask")
	}
	if f == nil {
		Stop("gk.NewAsk needs a function")
	}
	if _, ok := askCodecs[reflect.TypeFor[Ask[K, V]]()]; !ok {
		Stop("no closure of the kit asks a func(cx *gk.Cx, key %v) %v: give a function of the types the builder's gk.Ask names",
			reflect.TypeFor[K](), reflect.TypeFor[V]())
	}
	none := func(cx *Cx, at RowAt) Element { return nil }
	src := NewRowSource(cx, 0, none, Initial(0), Capacity(16), SourceEvents(&askRec[K, V]{f}, ShapeAskEvent))
	CheckCx(cx).Flush()
	o := NewReq(CAskNew)
	o.U64(src.h)
	MustCall(*o)
	return Ask[K, V]{src}
}

// TestAsks is the library's counts of data closure answers (gets, keys asked, answers cached).
func TestAsks(cx *Cx) (gets, asked, answered int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CAskTest))}
	return int(r.U64()), int(r.U64()), int(r.U64())
}

// TestLends is the library's counts of objects lent by kit state and given back (PROTOCOLS.md 9.17).
func TestLends(cx *Cx) (lent, givenBack int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CLendTest))}
	return int(r.U64()), int(r.U64())
}

// ItemsKinds names the kinds of item functions (overlay.Items), for messages: the generated package sets it.
var ItemsKinds []string
