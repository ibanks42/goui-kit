// View table, tape deduplication, and generational handler retirement.
//
// Each Go view keeps its last encoded byte tape (v.last) and its active event
// handler slice (v.cur) under an incrementing generation number (v.gen). When a
// redraw produces bytes identical to v.last, the tape is not re-sent across the
// C ABI and v.cur is updated in place. When the tape changes, v.gen increments
// and the previous generation's handler slice moves into v.old until the Rust
// core reports that generation retired via retireTable, ensuring in-flight UI
// events targeting the previous frame still find their handlers.

package rt

import (
	"bytes"
)

// View is a handle to a GPUI view backed by a Go render function and optional
// init callback.
type View struct{ HCore }

// viewRec stores the Go-side metadata, cached description tape, per-generation
// event handler tables, and optional RowSource binding for a live view.
type viewRec struct {
	h          uint64
	init       func(*Cx)
	render     func(*Cx) Element
	last       Tape
	gen        uint32
	cur        []handlerRec
	old        []oldTable
	curRetired bool
	buildNo    uint32
	window     uint64
	described  bool
	readsLib   bool // true if the last render read Rust state; disables pure inline event redraws
	dead       bool
	row        func(*Cx, RowAt) Element
	rowAt      RowAt
	src        uint64
	file       string
	line       int
}

var views viewTable

// viewTable maps 64-bit generational view handles to *viewRec in O(1) without a
// Go map lookup by indexing on the low 32 bits (slot number) and verifying the
// full 64-bit handle equality.
type viewTable struct{ slots []*viewRec }

func (t *viewTable) get(h uint64) *viewRec {
	if i := int(uint32(h)); i < len(t.slots) {
		if v := t.slots[i]; v != nil && v.h == h {
			return v
		}
	}
	return nil
}

func (t *viewTable) set(h uint64, v *viewRec) {
	i := int(uint32(h))
	if i >= len(t.slots) {
		t.slots = append(t.slots, make([]*viewRec, i+1-len(t.slots)+len(t.slots)/2)...)
	}
	t.slots[i] = v
}

func (t *viewTable) del(h uint64) {
	if i := int(uint32(h)); i < len(t.slots) && t.slots[i] != nil && t.slots[i].h == h {
		t.slots[i] = nil
	}
}

// NewView creates a view backed by optional init and required render callbacks
// and registers it under the current turn's owning view.
func NewView(init func(cx *Cx), render func(cx *Cx) Element) View {
	if cur == nil {
		Stop("gk.NewView was called before the app started: make views in the function given to gk.Run, in init or in a handler")
	}
	if Vb.Active {
		Stop("a view function may not create views (it runs on every redraw): create them in init or in a handler and keep the gk.View in your state struct")
	}
	if render == nil {
		Stop("gk.NewView needs a view function (the second argument)")
	}
	cur.Flush()
	r := Reader{B: MustCall(*NewReq(CViewNew))}
	rec := &viewRec{h: r.U64(), init: init, render: render}
	rec.file, rec.line, _ = appFrame(2)
	views.set(rec.h, rec)
	own(rec.h, ownView)
	return View{MkH(rec.h)}
}

// Notify marks view v dirty in the current turn so it will be re-described.
func Notify(cx *Cx, v View) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("gk.Notify in a view function: a view function only describes. Call it in the handler that changed the data (or call gk.Refresh there)")
	}
	t.markDirty(v.h)
}

// Quiet suppresses the automatic dirty mark on the event handler's owning view
// for the current turn.
func Quiet(cx *Cx) { CheckCx(cx).quiet = true }

// releaseView tears down view v along with every child handle, built-view site,
// and kept handler owned by it, then releases the Rust handle.
func releaseView(t *turn, v View) {
	rec := views.get(v.h)
	if rec == nil {
		Stop("this gk.View was released already: release a view once")
	}
	views.del(v.h)
	disown(v.h)
	releaseOwned(t, v.h)
	forgetBuiltSites(v.h)
	dropKeptOf(t, v.h)
	rec.cur, rec.old, rec.last = nil, nil, nil
	rec.dead = true
	releaseHandle(t, v.h)
}

// describe executes v's render or row callback into Vb, finishes the byte tape,
// and appends it to the current turn's reply tapes if the tape changed.
// When the tape matches v.last byte-for-byte, only the handler table is updated
// in place and no tape is sent across the C ABI.
func describe(cx *Cx, v *viewRec) {
	t := cx.t
	if t.window != 0 {
		v.window = t.window
	}
	buildSerial++
	v.buildNo++
	Vb.reset(buildSerial)
	Vb.view = v
	Vb.Active = true
	var root Element
	if v.row != nil {
		root = v.row(cx, v.rowAt)
	} else {
		root = v.render(cx)
	}
	tapeBuf = finish(root, tapeBuf)
	v.described = true
	v.readsLib = Vb.calledLib || len(Vb.deps) > 0
	if v.gen != 0 && bytes.Equal(tapeBuf, v.last) {
		if len(Vb.handlers) > 0 || len(v.cur) > 0 {
			v.cur = append(v.cur[:0], Vb.handlers...)
		}
		return
	}
	if cap(v.last) < len(tapeBuf) {
		v.last = tapeMem(len(tapeBuf))
	}
	v.last = append(v.last[:0], tapeBuf...)
	// Keep the previous generation's handler table alive until Rust retires it,
	// because in-flight events may still reference the old generation number.
	if v.gen != 0 && !v.curRetired && len(v.cur) > 0 {
		v.old = append(v.old, oldTable{v.gen, v.cur})
	}
	v.curRetired = false
	v.gen++
	if len(Vb.handlers) > 0 {
		v.cur = append(newTable(), Vb.handlers...)
	} else {
		v.cur = nil
	}
	t.tapes.U64(v.h)
	t.tapes.U32(v.gen)
	t.tapes.U32(uint32(len(v.last)))
	t.tapes = append(t.tapes, v.last...)
	t.nTapes++
}

// Arena block for small cached tapes (<= 512 bytes) to reduce per-view heap
// allocations.
var tapeBlock []byte

func tapeMem(n int) Tape {
	if n > 512 {
		return make(Tape, 0, n)
	}
	if cap(tapeBlock)-len(tapeBlock) < n {
		tapeBlock = make([]byte, 0, 16<<10)
	}
	at := len(tapeBlock)
	tapeBlock = tapeBlock[:at+n]
	return tapeBlock[at : at : at+n]
}

// Pool of cleared handler slices reused across view generations.
var tablePool [][]handlerRec

func newTable() []handlerRec {
	if n := len(tablePool); n > 0 {
		t := tablePool[n-1]
		tablePool = tablePool[:n-1]
		return t
	}
	return nil
}

// retireTable releases the handler table for a generation that the Rust core
// has finished displaying and will no longer deliver events for.
func retireTable(view uint64, gen uint32) {
	v := views.get(view)
	if v == nil {
		return
	}
	if gen == v.gen {
		v.curRetired = true
		return
	}
	for i, o := range v.old {
		if o.gen == gen {
			clear(o.t)
			if len(tablePool) < 64 {
				tablePool = append(tablePool, o.t[:0])
			}
			last := len(v.old) - 1
			if i != last {
				copy(v.old[i:], v.old[i+1:])
			}
			v.old[last] = oldTable{}
			v.old = v.old[:last]
			return
		}
	}
}

type oldTable struct {
	gen uint32
	t   []handlerRec
}

func (v *viewRec) oldTableOf(gen uint32) ([]handlerRec, bool) {
	for _, o := range v.old {
		if o.gen == gen {
			return o.t, true
		}
	}
	return nil, false
}

// dep records that the active view description read state handle h so the Rust
// core can subscribe the view to changes in that state.
func dep(h uint64) {
	if !Vb.Active {
		return
	}
	for _, d := range Vb.deps {
		if d == h {
			return
		}
	}
	Vb.deps = append(Vb.deps, h)
}
