package rt

type modelReader struct {
	view    *viewRec
	buildNo uint32
}

// Model holds an observable Go value of type T. Any view that reads the model
// with Get during its render function is automatically marked dirty when Set,
// Update, or Touch changes the model.
type Model[T any] struct {
	value     T
	readers   []modelReader
	observers []func(*Cx, T)
	changing  bool
}

// NewModel creates a Model initialized to v.
func NewModel[T any](v T) Model[T] { return Model[T]{value: v} }

// Get returns the current value and, when called inside a view render function,
// subscribes that view (for its current buildNo) to future writes on m.
func (m *Model[T]) Get() T {
	if Vb.Active && Vb.view != nil {
		v := Vb.view
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

// Peek returns the current value without subscribing the active view.
func (m *Model[T]) Peek() T { return m.value }

func modelWrite(cx *Cx) *turn {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not change app state: it only describes. Change state in a handler, in init or in a frame handler")
	}
	return t
}

// Set replaces the model's value with v, marks subscribed views dirty, and
// notifies registered observers.
func (m *Model[T]) Set(cx *Cx, v T) {
	t := modelWrite(cx)
	m.value = v
	m.changed(t)
}

// Update passes a pointer to the stored value to f, then marks subscribed views
// dirty and notifies observers.
func (m *Model[T]) Update(cx *Cx, f func(v *T)) {
	t := modelWrite(cx)
	f(&m.value)
	m.changed(t)
}

// Touch marks subscribed views dirty and notifies observers without modifying
// the stored value directly (useful after mutating a slice or map in place).
func (m *Model[T]) Touch(cx *Cx) { m.changed(modelWrite(cx)) }

// Observe registers f to run whenever m changes.
func (m *Model[T]) Observe(cx *Cx, f func(cx *Cx, v T)) {
	CheckCx(cx)
	m.observers = append(m.observers, f)
}

// changed marks all still-subscribed views dirty, prunes dead or stale readers
// that stopped calling Get on their latest build, and runs observers once
// (guarding against re-entrant observer notifications via m.changing).
func (m *Model[T]) changed(t *turn) {
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
		return
	}
	m.changing = true
	defer func() { m.changing = false }()
	for _, f := range m.observers {
		f(t.cx, m.value)
	}
}
