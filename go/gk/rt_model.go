package gk

// App state lives in Go and the library never sees it. A Model is the bookkeeping a Rust app gets
// from an Entity: it knows which views read it, and changing it marks exactly those views for
// redraw. It is optional: a handler already marks the view that registered it. No crossing is
// involved; the marks travel in the turn's reply.
//
//	type App struct{ count gk.Model[int] }
//	a.count.Get()                     in a view function: records that this view shows it
//	a.count.Set(cx, 3)                in a handler, a post, a frame handler: marks those views
//	a.count.Update(cx, func(v *int) { *v++ })
//	a.count.Observe(cx, func(cx *gk.Cx, v int) { ... })   runs on every change

type modelReader struct {
	view    *viewRec
	buildNo uint32
}

// Model holds a value of type T and the views that read it.
type Model[T any] struct {
	value     T
	readers   []modelReader
	observers []func(*Cx, T)
	changing  bool
}

// NewModel is a Model holding v (the zero Model holds T's zero value).
func NewModel[T any](v T) Model[T] { return Model[T]{value: v} }

// Get reads the value. In a view function it also records that the view shows it.
func (m *Model[T]) Get() T {
	if vb.active && vb.view != nil {
		v := vb.view
		found := false
		for i := range m.readers {
			if m.readers[i].view == v {
				m.readers[i].buildNo = v.buildNo
				found = true
				break
			}
		}
		if !found {
			m.readers = append(m.readers, modelReader{v, v.buildNo})
		}
	}
	return m.value
}

// Peek reads the value without recording anything.
func (m *Model[T]) Peek() T { return m.value }

func modelWrite(cx *Cx) *turn {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not change app state: it only describes. Change state in a handler, in init or in a frame handler")
	}
	return t
}

// Set stores a value and marks the views that read the model in their latest build.
func (m *Model[T]) Set(cx *Cx, v T) {
	t := modelWrite(cx)
	m.value = v
	m.changed(t)
}

// Update changes the value in place and marks the views that read the model.
func (m *Model[T]) Update(cx *Cx, f func(v *T)) {
	t := modelWrite(cx)
	f(&m.value)
	m.changed(t)
}

// Touch marks the model's readers after its value was changed through a pointer.
func (m *Model[T]) Touch(cx *Cx) { m.changed(modelWrite(cx)) }

// Observe runs f after every change of the model (GPUI's cx.observe).
func (m *Model[T]) Observe(cx *Cx, f func(cx *Cx, v T)) {
	checkCx(cx)
	m.observers = append(m.observers, f)
}

func (m *Model[T]) changed(t *turn) {
	// Readers whose view has been described again without reading the model are dropped.
	keep := m.readers[:0]
	for _, r := range m.readers {
		if r.view.dead || r.view.buildNo != r.buildNo {
			continue
		}
		t.markDirty(r.view.h)
		keep = append(keep, r)
	}
	m.readers = keep
	if m.changing {
		return // an observer changed the model it observes: its readers are marked, no recursion
	}
	m.changing = true
	defer func() { m.changing = false }()
	for _, f := range m.observers {
		f(t.cx, m.value)
	}
}
