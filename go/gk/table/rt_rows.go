package table

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
	"slices"
)

// HeaderRow and FooterRow are the RowAt.Row of a list section's header and footer rows (drawn when
// the list's DelegateData asks for them).
const (
	HeaderRow = 0xffff_fffe
	FooterRow = 0xffff_ffff
)

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
	Index     gk.Option[ListIndex]
	Secondary bool
	Query     string
}

func init() {
	rt.RegisterShape(rt.ShapeListEvent, rt.ShapeInfo{Name: "func(cx *Cx, e ListDelegateEvent)", Call: func(h any, cx *gk.Cx, r *rt.Reader, ret *rt.Tape) bool {
		f, ok := h.(func(*gk.Cx, ListDelegateEvent))
		if ok {
			e := ListDelegateEvent{ID: r.U64(), Kind: ListEventKind(r.U16())}
			e.Index = rt.RdOpt(r, func(r *rt.Reader) ListIndex { return ListIndex{int(r.U64()), int(r.U64()), int(r.U64())} })
			e.Secondary = r.Bool()
			e.Query = r.Str()
			f(cx, e)
		}
		return ok
	}})
}

// ListEvents gives a list's source the handler of its delegate's events.
func ListEvents(f func(cx *gk.Cx, e ListDelegateEvent)) gk.SourceOption {
	return rt.SourceEvents(f, rt.ShapeListEvent)
}

// NewListSource makes the row source of a list: count() rows (one section), each described by row
// as a gk.ListItem, which the list marks selected. count is asked now and at every gk.Refresh, so
// after a change of the app's data gk.Refresh is all the list needs (no gk.SetCount). Make the
// list's state over it with gk.NewListState(cx, src).Build(). Needs a window: init or a handler.
func NewListSource(cx *gk.Cx, count func() int, row func(cx *gk.Cx, at gk.RowAt) ListItemEl, opts ...gk.SourceOption) gk.RowSource {
	if count == nil {
		rt.Stop("table.NewListSource needs a count function: func() int { return len(a.items) }")
	}
	if row == nil {
		rt.Stop("table.NewListSource needs a row function")
	}
	opts = append(opts, gk.CountFunc(count))
	return gk.NewRowSource(cx, count(), func(cx *gk.Cx, at gk.RowAt) gk.Element { return row(cx, at) }, opts...)
}

// SelectOption changes what gk.SelectRow does besides selecting.
type SelectOption uint8

// ScrollIntoView: gk.SelectRow also scrolls the list so that the row is in view.
const ScrollIntoView SelectOption = 1

// SelectRow selects a row of a list (row < 0: none) without the list's event: what the app calls
// when its own selection changed (after a filter, after a delete). With gk.ScrollIntoView the
// list also scrolls the row into view.
func SelectRow(cx *gk.Cx, list ListState, row int, opts ...SelectOption) {
	if row < 0 {
		list.SetSelectedIndex(cx, NoRow)
		return
	}
	list.SetSelectedIndex(cx, gk.Some(Row(row)))
	if slices.Contains(opts, ScrollIntoView) {
		list.ScrollToItem(cx, Row(row), gk.ScrollStrategyNearest)
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
	Empty             gk.View
	LoadingView       gk.View
	Initial           gk.View // lists: shown before the first search
	PlaceholderHeight float32 // lists: the height of a row drawn as a placeholder for one frame
}

// SetDelegateData sets what a list or table shows besides its rows. A buffered write.
func SetDelegateData(cx *gk.Cx, s rt.Handled, d DelegateData) {
	t := rt.CheckCx(cx)
	if d.Threshold < 0 {
		rt.Stop("table.SetDelegateData was given a negative threshold (%d): give 0 or more rows", d.Threshold)
	}
	var flags uint8
	for i, b := range []bool{d.Loading, d.HasMore, d.VisibleEvents, d.Headers, d.Footers} {
		if b {
			flags |= 1 << i
		}
	}
	o := rt.NewReq(rt.CRowsData)
	o.U64(rt.HandleOf(s))
	o.U8(flags)
	o.U64(uint64(d.Threshold))
	o.U64(rt.H(d.Empty.HCore))
	o.U64(rt.H(d.LoadingView.HCore))
	o.U64(rt.H(d.Initial.HCore))
	o.F32(d.PlaceholderHeight)
	t.DeferOp(*o)
}
