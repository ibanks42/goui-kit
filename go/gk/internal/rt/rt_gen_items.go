package rt

import (
	"reflect"
)

// itemsKey identifies a dynamic item-builder closure by the owning view and
// the function's code pointer so repeated view renders reuse a single
// underlying RowSource instead of allocating a fresh one every frame.
type itemsKey struct {
	view uint64
	fn   uintptr
}

type itemsRec struct {
	f    any
	kind int
}

var (
	itemsSources = map[itemsKey]RowSource{}
	itemsOf      = map[uint64]*itemsRec{}
)

// ItemsSource returns a cached or newly created RowSource backed by closure f
// and adapter tramp for item-producing kit closures (such as popup menus or
// dynamic item lists). If called during a view render, Vb.Active is briefly
// cleared so NewRowSource can allocate the backing source safely.
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
	Vb.Active = false
	src = NewRowSource(cx, 0, row, Initial(0), Capacity(512))
	Vb.Active = active
	itemsSources[key] = src
	itemsOf[src.h] = rec
	return src
}

// ItemsCheck verifies that src was created for item procedure kind and returns
// its raw handle for tape encoding.
func ItemsCheck(src RowSource, kind int) uint64 {
	if rec := itemsOf[src.h]; rec != nil && rec.kind != kind && sources[src.h] != nil {
		Stop("overlay.Items was given a function of the kind %s, but this closure calls one of the kind %s",
			ItemsKinds[rec.kind], ItemsKinds[kind])
	}
	return src.h
}

// RowContext fetches the per-row context bytes supplied by Rust for (src, at).
func RowContext(cx *Cx, src RowSource, at RowAt) *Reader {
	CheckCx(cx)
	o := NewReq(CCtxGet)
	o.U64(src.h)
	o.U32(uint32(at.Section))
	o.U32(uint32(at.Row))
	return &Reader{B: MustCall(*o)}
}

// KitView is a placeholder element referencing a kit-supplied child view by
// slot index inside an item-procedure template.
type KitView struct{ ElCore }

// XKitView creates a KitView node referencing kit-provided child view index.
func XKitView(index int) KitView {
	n := NewNode(NKitView)
	NodeArgs(n).U8(uint8(index))
	return KitView{MkElCore(n)}
}

// Ask represents a typed synchronous request/response callback channel backed
// by a RowSource, used when Rust kit components query Go for computed data
// (such as highlight runs, code folds, or syntax context).
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

var askCodecs = map[reflect.Type]askCodec{}

// RegAsk registers the wire reader for key type K and writer for value type V
// so NewAsk[K, V] can decode incoming requests and encode replies.
func RegAsk[K, V any](rd func(r *Reader) K, put func(o *Tape, v V)) {
	askCodecs[reflect.TypeFor[Ask[K, V]]()] = askCodec{func(r *Reader) any { return rd(r) }, func(o *Tape, v any) { put(o, v.(V)) }}
}

func (a *askRec[K, V]) serve(cx *Cx, r *Reader) {
	Quiet(cx)
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

// NewAsk creates a typed Ask[K, V] callback channel backed by a RowSource and
// registers it with the Rust core. Must be called outside a view function.
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

// TestAsks returns diagnostic counters for Ask gets, requests sent, and answers received.
func TestAsks(cx *Cx) (gets, asked, answered int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CAskTest))}
	return int(r.U64()), int(r.U64()), int(r.U64())
}

// TestLends returns diagnostic counters for lent handles issued and returned.
func TestLends(cx *Cx) (lent, givenBack int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CLendTest))}
	return int(r.U64()), int(r.U64())
}

// ItemsKinds holds the human-readable names of generated item procedure kinds,
// populated by generated package init code for ItemsCheck error messages.
var ItemsKinds []string
