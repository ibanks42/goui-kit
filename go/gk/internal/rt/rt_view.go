package rt

import (
	"bytes"
)

// View is an app view: a function that describes its elements, retained by gpui-fast as a view
// of its own. Embed one in another with .ChildView(v): the parent stays retained when the child
// is described again, which is the main tool for a fast app.
type View struct{ HCore }

type viewRec struct {
	h      uint64
	init   func(*Cx)
	render func(*Cx) Element
	last   Tape         // the accepted tape
	gen    uint32       // generation of last
	cur    []handlerRec // the handler table of gen
	// The handler tables of older generations the library still holds (an open menu made from
	// one, a closure upstream keeps): each goes when the library reports its description dropped
	// (retireTable, from a VIEWS turn).
	old        []oldTable
	curRetired bool   // the library dropped gen's description: its table is not kept when gen moves on
	buildNo    uint32 // counts runs of the view function (what a Model's reader list refers to)
	described  bool
	dead       bool
	// a row view (rt_source.go): the source's row function for one row
	row   func(*Cx, RowAt) Element
	rowAt RowAt
	src   uint64
	file  string // where gk.NewView made it (for messages)
	line  int
}

// views holds every live view by handle (viewTable).
var views viewTable

// viewTable is the live views by handle: a slice indexed by the handle's slot number (its low 32
// bits; the library reuses slot numbers, so the table stays as small as the handles alive), each
// entry checked against the whole handle. A ROWS turn binds a few hundred row views at once:
// a map paid a hash and, now and then, a regrowth for each.
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

// NewView makes a view: render describes it whenever it is dirty, and init, if not nil, runs
// once when a window builds this view as its root (kit state that needs a window is made there).
// Call it in the start function, in an init function or in a handler, never in a view function.
//
//	a.root = gk.NewView(a.init, a.view)
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

// Notify marks a view dirty: it is described again before the next frame. An unchanged
// description costs no redraw.
func Notify(cx *Cx, v View) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("gk.Notify in a view function: a view function only describes. Call it in the handler that changed the data (or call gk.Refresh there)")
	}
	t.markDirty(v.h)
}

// Quiet, called in a handler, keeps the handler's view from being marked for redraw when it
// returns (a hover that changes nothing on screen).
func Quiet(cx *Cx) { CheckCx(cx).quiet = true }

func releaseView(t *turn, v View) {
	rec := views.get(v.h)
	if rec == nil {
		Stop("this gk.View was released already: release a view once")
	}
	views.del(v.h)
	disown(v.h)
	releaseOwned(t, v.h) // what the view made: its states, views, sources, overlays (rt_life.go)
	forgetBuiltSites(v.h)
	dropKeptOf(t, v.h)
	rec.cur, rec.old, rec.last = nil, nil, nil
	rec.dead = true
	releaseHandle(t, v.h)
}

func describe(cx *Cx, v *viewRec) {
	t := cx.t
	buildSerial++
	v.buildNo++
	Vb.reset(buildSerial)
	Vb.view = v
	Vb.Active = true
	defer func() { Vb.Active = false }()
	var root Element
	if v.row != nil {
		root = v.row(cx, v.rowAt)
	} else {
		root = v.render(cx)
	}
	tapeBuf = finish(root, tapeBuf)
	v.described = true
	if v.gen != 0 && bytes.Equal(tapeBuf, v.last) {
		// An identical tape is dropped: nothing is notified and the generation stays. The handler
		// table is still replaced in place: same ids, but the closures may have captured new values.
		v.cur = append(v.cur[:0], Vb.handlers...)
		return
	}
	if cap(v.last) < len(tapeBuf) {
		v.last = tapeMem(len(tapeBuf))
	}
	v.last = append(v.last[:0], tapeBuf...)
	if v.gen != 0 && !v.curRetired {
		v.old = append(v.old, oldTable{v.gen, v.cur})
	}
	v.curRetired = false
	v.gen++
	v.cur = append(newTable(), Vb.handlers...)
	t.tapes.U64(v.h)
	t.tapes.U32(v.gen)
	t.tapes.U32(uint32(len(v.last)))
	t.tapes = append(t.tapes, v.last...)
	t.nTapes++
}

// tapeBlock is where small accepted tapes are kept: a row view's first tape (a ROWS turn binds a
// few hundred rows) is a piece of it rather than an allocation of its own.
var tapeBlock []byte

// tapeMem is an empty slice of capacity n for a view's accepted tape: a piece of tapeBlock when
// small (capped, so an append past it never runs into the next piece).
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

// Handler tables no description uses any more, for reuse.
var tablePool [][]handlerRec

func newTable() []handlerRec {
	if n := len(tablePool); n > 0 {
		t := tablePool[n-1]
		tablePool = tablePool[:n-1]
		return t
	}
	return nil
}

// retireTable forgets the handler table of a view's description the library has dropped (no
// element, open menu or closure upstream keeps holds it): none of its handlers can fire again.
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
			v.old = append(v.old[:i], v.old[i+1:]...)
			return
		}
	}
}

// oldTable is the handler table of an older description of a view that the library still holds
// (viewRec.old: usually none or one, so a slice rather than a map).
type oldTable struct {
	gen uint32
	t   []handlerRec
}

// oldTableOf is the handler table of description gen of v, if the library still holds it.
func (v *viewRec) oldTableOf(gen uint32) ([]handlerRec, bool) {
	for _, o := range v.old {
		if o.gen == gen {
			return o.t, true
		}
	}
	return nil, false
}

// dep records that the view being built read this state: the library observes it and has the
// view described again when it changes.
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
