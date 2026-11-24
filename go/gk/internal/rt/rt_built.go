package rt

// Built is the view behind a view-built closure whose function describes a T (a DialogEl, a
// SheetEl, a PopupMenu). Make it once (in init) and give it to the call that opens the overlay
// each time; gk.Release(cx, b.View) forgets it.
type Built[T any] struct{ View }

type ValueEl struct{ ElCore }

// NewBuilt makes the view that describes a T with render. Only in a handler or init: a view
// function may not make views.
func NewBuilt[T any](cx *Cx, render func(cx *Cx) T) Built[T] {
	CheckCx(cx)
	if Vb.Active {
		Stop("gk.NewBuilt in a view function: make it in init (once) or in a handler; a view function only describes")
	}
	if render == nil {
		Stop("gk.NewBuilt needs a function that describes the overlay")
	}
	v := NewView(nil, func(cx *Cx) Element {
		var x any = render(cx)
		switch n := x.(type) {
		case Element:
			return n
		case value:
			return ValueEl{MkElCore(n.valueNode())} // a described value (a popup menu): its node is the root
		}
		Stop("the function given to gk.NewBuilt returned a %T, which is not a kit element or value: return what the overlay shows (a overlay.Dialog(), overlay.AlertDialog(), overlay.Sheet(), a menu)", x)
		return nil
	})
	return Built[T]{v}
}

// putBuilt writes a view-built closure: the view's handle, described again before the overlay
// is built from it (a dialog opened again shows the state of now).
func PutBuilt(o *Tape, v View) {
	if v.h == 0 {
		Stop("no view given: make it with gk.NewBuilt(cx, function), once, in init")
	}
	if views.get(v.h) == nil {
		Stop("the view gk.NewBuilt made was released: make a new one with gk.NewBuilt")
	}
	o.U64(v.h)
	if cur != nil {
		cur.markDirty(v.h)
	}
}
