package gk

// View-built closures (docs/PROTOCOLS.md section 11.6): a dialog, an alert or a sheet the kit
// keeps open and builds again whenever it draws, described by a function of the app. The view is
// an ordinary view: its handlers run between frames and mark it for redraw, so the dialog's
// content changes while it is open.
//
//	func (a *App) confirm(cx *gk.Cx) gk.DialogEl {
//		return gk.Dialog().Title(gk.Text("Delete?")).Child(gk.Label(a.name)).
//			OnOk(func(cx *gk.Cx, _ gk.ClickEvent) bool { a.deleted = true; return true })
//	}
//	a.confirm = gk.NewBuilt(cx, a.confirm)    // once, in init
//	gk.OpenDialog(cx, a.confirm)             // in a handler, as often as it opens

// Built is the view behind a view-built closure whose function describes a T (a DialogEl, a
// SheetEl, a PopupMenu). Make it once (in init) and give it to the call that opens the overlay
// each time; gk.Release(cx, b.View) forgets it.
type Built[T any] struct{ View }

type valueEl struct{ n *node }

func (e valueEl) node() *node { return e.n }

// NewBuilt makes the view that describes a T with render. Only in a handler or init: a view
// function may not make views.
func NewBuilt[T any](cx *Cx, render func(cx *Cx) T) Built[T] {
	checkCx(cx)
	if vb.active {
		stop("gk.NewBuilt in a view function: make it in init (once) or in a handler; a view function only describes")
	}
	if render == nil {
		stop("gk.NewBuilt needs a function that describes the overlay")
	}
	v := NewView(nil, func(cx *Cx) Element {
		var x any = render(cx)
		switch n := x.(type) {
		case Element:
			return n
		case value:
			return valueEl{n.valueNode()} // a described value (a popup menu): its node is the root
		}
		stop("the function given to gk.NewBuilt returned a %T, which is not a kit element or value: return what the overlay shows (a gk.Dialog(), gk.AlertDialog(), gk.Sheet(), a menu)", x)
		return nil
	})
	return Built[T]{v}
}

// putBuilt writes a view-built closure: the view's handle, described again before the overlay
// is built from it (a dialog opened again shows the state of now).
func putBuilt(o *tape, v View) {
	if v.h == 0 {
		stop("no view given: make it with gk.NewBuilt(cx, function), once, in init")
	}
	if views[v.h] == nil {
		stop("the view gk.NewBuilt made was released: make a new one with gk.NewBuilt")
	}
	o.u64(v.h)
	if cur != nil {
		cur.markDirty(v.h)
	}
}
