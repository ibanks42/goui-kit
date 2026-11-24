package rt

import (
	"runtime"
	"strings"
)

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

// Builder is what an overlay opens with (overlay.OpenDialog, OpenSheet, ...): a Built[T] made once
// with gk.NewBuilt, or the function that describes the overlay, given right there:
//
//	overlay.OpenDialog(cx, func(cx *gk.Cx) overlay.DialogEl { return overlay.Dialog().Title(..) })
//
// A function gets a view of its own for the place in the app's code that opens it (one view per
// call site and per view whose handler runs it): opening again from there shows the newer
// function, and the view is released with the view that owns it.
type Builder[T any] interface {
	Built[T] | func(cx *Cx) T
}

type builtKey struct {
	owner uint64  // the view whose turn opened it (0: none)
	pc    uintptr // where in the app's code
}

type builtSite struct {
	view View
	fn   any // the func(*Cx) T given last
}

var builtSites = map[builtKey]*builtSite{}

// BuiltView is the view b names: a Built's own, or the call site's view for a function.
func BuiltView[T any, B Builder[T]](cx *Cx, b B) View {
	switch x := any(b).(type) {
	case Built[T]:
		return x.View
	case func(*Cx) T:
		t := CheckCx(cx)
		if Vb.Active {
			Stop("an overlay was opened in a view function: open it in a handler (a view function only describes)")
		}
		if x == nil {
			Stop("a nil function was given to open an overlay: give the function that describes it")
		}
		key := builtKey{pc: appPC(3)}
		if t.owner != nil {
			key.owner = t.owner.h
		}
		s := builtSites[key]
		if s == nil || views.get(s.view.h) == nil {
			s = &builtSite{}
			s.view = NewBuilt(cx, func(cx *Cx) T { return s.fn.(func(*Cx) T)(cx) }).View
			builtSites[key] = s
		}
		s.fn = x
		return s.view
	}
	panic("unreachable")
}

// forgetBuiltSites drops the call-site views of a view being released (their views go with it).
func forgetBuiltSites(owner uint64) {
	for k := range builtSites {
		if k.owner == owner {
			delete(builtSites, k)
		}
	}
}

// appPC is the program counter of the innermost caller outside this package (appFrame's line).
func appPC(skip int) uintptr {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(skip, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" && !inGk(f.Function) && !strings.HasPrefix(f.Function, "runtime.") {
			return f.PC
		}
		if !more {
			return 0
		}
	}
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
