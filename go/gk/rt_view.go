package gk

// Views: a function that describes elements. gpui-fast retains each view as one of its own,
// and the binding runs the function only when the view is dirty.

import "bytes"

// View is an app view: a function that describes its elements, retained by gpui-fast as a view
// of its own. Embed one in another with .ChildView(v): the parent stays retained when the child
// is described again, which is the main tool for a fast app.
type View struct{ h uint64 }

func (v View) handle() uint64 { return v.h }

type viewRec struct {
	h         uint64
	init      func(*Cx)
	render    func(*Cx) Element
	last      tape   // the accepted tape
	gen       uint32 // generation of last
	cur, prev []handlerRec
	buildNo   uint32 // counts runs of the view function (what a Model's reader list refers to)
	described bool
	dead      bool
	// a row view (rt_source.go): the source's row function for one row
	row   func(*Cx, RowAt) Element
	rowAt RowAt
	src   uint64
	file  string // where gk.NewView made it (for messages)
	line  int
}

var views = map[uint64]*viewRec{}

// NewView makes a view: render describes it whenever it is dirty, and init, if not nil, runs
// once when a window builds this view as its root (kit state that needs a window is made there).
// Call it in the start function, in an init function or in a handler, never in a view function.
//
//	a.root = gk.NewView(a.init, a.view)
func NewView(init func(cx *Cx), render func(cx *Cx) Element) View {
	if cur == nil {
		stop("gk.NewView was called before the app started: make views in the function given to gk.Run, in init or in a handler")
	}
	if vb.active {
		stop("a view function may not create views (it runs on every redraw): create them in init or in a handler and keep the gk.View in your state struct")
	}
	if render == nil {
		stop("gk.NewView needs a view function (the second argument)")
	}
	cur.flush()
	r := reader{b: mustCall(*newReq(cViewNew))}
	rec := &viewRec{h: r.u64(), init: init, render: render}
	rec.file, rec.line, _ = appFrame(2)
	views[rec.h] = rec
	return View{rec.h}
}

// Notify marks a view dirty: it is described again before the next frame. An unchanged
// description costs no redraw.
func Notify(cx *Cx, v View) {
	t := checkCx(cx)
	if vb.active {
		stop("gk.Notify in a view function: a view function only describes. Call it in the handler that changed the data (or call gk.Refresh there)")
	}
	t.markDirty(v.h)
}

// Quiet, called in a handler, keeps the handler's view from being marked for redraw when it
// returns (a hover that changes nothing on screen).
func Quiet(cx *Cx) { checkCx(cx).quiet = true }

func releaseView(t *turn, v View) {
	rec := views[v.h]
	if rec == nil {
		stop("this gk.View was released already: release a view once")
	}
	delete(views, v.h)
	dropKeptOf(t, v.h)
	rec.cur, rec.prev, rec.last = nil, nil, nil
	rec.dead = true
	releaseHandle(t, v.h)
}

func describe(cx *Cx, v *viewRec) {
	t := cx.t
	buildSerial++
	v.buildNo++
	vb.reset(buildSerial)
	vb.view = v
	vb.active = true
	defer func() { vb.active = false }()
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
		v.cur = append(v.cur[:0], vb.handlers...)
		return
	}
	v.last = append(v.last[:0], tapeBuf...)
	v.gen++
	v.prev, v.cur = v.cur, append(v.prev[:0], vb.handlers...)
	t.tapes.u64(v.h)
	t.tapes.u32(v.gen)
	t.tapes.u32(uint32(len(v.last)))
	t.tapes = append(t.tapes, v.last...)
	t.nTapes++
}

// dep records that the view being built read this state: the library observes it and has the
// view described again when it changes.
func dep(h uint64) {
	if !vb.active {
		return
	}
	for _, d := range vb.deps {
		if d == h {
			return
		}
	}
	vb.deps = append(vb.deps, h)
}
