package gk

import "slices"

// The rows family of the delegate protocol (docs/PROTOCOLS.md section 9.5), lists: the kit's List
// over a row source. Each row is a view of its own, described once by the app's row function and
// retained; the kit asks the library for rows while it draws, and the library never calls the app
// then: a row without a view is a placeholder for one frame and is described before the next.
//
//	src := gk.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)
//	a.list = gk.NewListState(cx, src).Build()          a.list.OnListEvent(cx, a.listEvent)
//	func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
//		return gk.ListItem(gk.Key("item", at.Row)).Child(gk.Label(a.items[at.Row]))
//	}
//	view: gk.Div().H(gk.Px(300)).Child(gk.List(a.list))

// ListIndex is a row of a list: its section and row (Column is always 0 for a list).
type ListIndex struct{ Section, Row, Column int }

// ListEventKind is what a list's delegate reports.
type ListEventKind uint16

const (
	ListSelect     ListEventKind = iota // the selected row changed (Index: none when cleared)
	ListRightClick                      // a row was right-clicked
	ListConfirm                         // a click on a row or Enter (Secondary: with the platform key)
	ListCancel                          // Escape
	ListLoadMore                        // scrolled near the end while DelegateData.HasMore is set
	ListSearch                          // a request: the query changed (searchable lists); answer with gk.Answer
)

func (k ListEventKind) String() string {
	return [...]string{"Select", "RightClick", "Confirm", "Cancel", "LoadMore", "Search", "?"}[min(int(k), 6)]
}

// ListDelegateEvent is the event of a list's row source.
type ListDelegateEvent struct {
	ID        uint64 // a request's id (ListSearch), 0 otherwise
	Kind      ListEventKind
	Index     Option[ListIndex]
	Secondary bool
	Query     string
}

func init() {
	registerShape(shapeListEvent, shapeInfo{"func(cx *Cx, e ListDelegateEvent)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, ListDelegateEvent))
		if ok {
			e := ListDelegateEvent{ID: r.u64(), Kind: ListEventKind(r.u16())}
			e.Index = rdOpt(r, func(r *reader) ListIndex { return ListIndex{int(r.u64()), int(r.u64()), int(r.u64())} })
			e.Secondary = r.bool()
			e.Query = r.str()
			f(cx, e)
		}
		return ok
	}})
}

// ListEvents gives a list's source the handler of its delegate's events.
func ListEvents(f func(cx *Cx, e ListDelegateEvent)) SourceOption {
	return sourceEvents(f, shapeListEvent)
}

// NewListSource makes the row source of a list: count() rows (one section), each described by row
// as a gk.ListItem, which the list marks selected. count is asked now and at every gk.Refresh, so
// after a change of the app's data gk.Refresh is all the list needs (no gk.SetCount). Make the
// list's state over it with gk.NewListState(cx, src).Build(). Needs a window: init or a handler.
func NewListSource(cx *Cx, count func() int, row func(cx *Cx, at RowAt) ListItemEl, opts ...SourceOption) RowSource {
	if count == nil {
		stop("gk.NewListSource needs a count function: func() int { return len(a.items) }")
	}
	if row == nil {
		stop("gk.NewListSource needs a row function")
	}
	opts = append(opts, CountFunc(count))
	return NewRowSource(cx, count(), func(cx *Cx, at RowAt) Element { return row(cx, at) }, opts...)
}

// SelectOption changes what gk.SelectRow does besides selecting.
type SelectOption uint8

// ScrollIntoView: gk.SelectRow also scrolls the list so that the row is in view.
const ScrollIntoView SelectOption = 1

// SelectRow selects a row of a list (row < 0: none) without the list's event: what the app calls
// when its own selection changed (after a filter, after a delete). With gk.ScrollIntoView the
// list also scrolls the row into view.
func SelectRow(cx *Cx, list ListState, row int, opts ...SelectOption) {
	if row < 0 {
		list.SetSelectedIndex(cx, NoRow)
		return
	}
	list.SetSelectedIndex(cx, Some(Row(row)))
	if slices.Contains(opts, ScrollIntoView) {
		list.ScrollToItem(cx, Row(row), ScrollStrategyNearest)
	}
}

// DelegateData is what a list or table shows or reports besides its rows. Views are zero for the
// kit's own (the empty inbox icon, the skeleton rows).
type DelegateData struct {
	Loading           bool // show the loading view instead of the rows
	HasMore           bool // send LoadMore when the user scrolls within Threshold rows of the end
	VisibleEvents     bool // tables: send the visible rows and columns
	Headers, Footers  bool // lists: draw each section's header or footer row
	Threshold         int
	Empty             View
	LoadingView       View
	Initial           View    // lists: shown before the first search
	PlaceholderHeight float32 // lists: the height of a row drawn as a placeholder for one frame
}

// SetDelegateData sets what a list or table shows besides its rows. A buffered write.
func SetDelegateData(cx *Cx, s handled, d DelegateData) {
	t := checkCx(cx)
	if d.Threshold < 0 {
		stop("gk.SetDelegateData was given a negative threshold (%d): give 0 or more rows", d.Threshold)
	}
	var flags uint8
	for i, b := range []bool{d.Loading, d.HasMore, d.VisibleEvents, d.Headers, d.Footers} {
		if b {
			flags |= 1 << i
		}
	}
	o := newReq(cRowsData)
	o.u64(s.handle())
	o.u8(flags)
	o.u64(uint64(d.Threshold))
	o.u64(d.Empty.h)
	o.u64(d.LoadingView.h)
	o.u64(d.Initial.h)
	o.f32(d.PlaceholderHeight)
	t.deferOp(*o)
}
