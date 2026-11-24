package rt

import (
	"runtime"
	"strings"
)

// Built wraps a View whose render function produces a specific element or value
// type T, used for overlays such as dialogs, sheets, and popovers.
type Built[T any] struct{ View }

// ValueEl adapts a value node into an Element so a Built view can return kit
// values (such as menus or tooltips) through the standard view tape pipeline.
type ValueEl struct{ ElCore }

// NewBuilt creates a reusable Built[T] view from render. The returned view must
// be created outside a view function and kept across turns.
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
			return ValueEl{MkElCore(n.valueNode())}
		}
		Stop("the function given to gk.NewBuilt returned a %T, which is not a kit element or value: return what the overlay shows (a overlay.Dialog(), overlay.AlertDialog(), overlay.Sheet(), a menu)", x)
		return nil
	})
	return Built[T]{v}
}

// Builder constrains overlay-opening APIs to accept either a pre-created
// Built[T] handle or an inline render closure func(*Cx) T.
type Builder[T any] interface {
	Built[T] | func(cx *Cx) T
}

// builtKey identifies an inline overlay call site by owning view and caller PC
// so repeated calls from the same line reuse one underlying View instead of
// leaking a new view on every open.
type builtKey struct {
	owner uint64
	pc    uintptr
}

type builtSite struct {
	view View
	fn   any
}

var builtSites = map[builtKey]*builtSite{}

// BuiltView resolves a Builder[T] into a live View, reusing a per-call-site
// Built[T] view when given an inline closure.
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

// forgetBuiltSites removes cached inline overlay sites owned by a released view.
func forgetBuiltSites(owner uint64) {
	for k := range builtSites {
		if k.owner == owner {
			delete(builtSites, k)
		}
	}
}

// appPC returns the program counter of the first application stack frame above
// skip frames, used to key cached inline BuiltView sites.
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

// PutBuilt encodes a Built view handle onto o and marks it dirty in the current
// turn so it is described before the overlay renders.
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
