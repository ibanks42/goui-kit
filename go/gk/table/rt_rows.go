package table

import (
	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/internal/rt"
	"slices"
)

// Sentinel row indices passed in gk.RowAt.Row when a sectioned list asks a
// RowSource to render a section header or footer.
const (
	HeaderRow = 0xffff_fffe
	FooterRow = 0xffff_ffff
)

// ListIndex locates an item by section, row, and column inside a list delegate event.
type ListIndex struct{ Section, Row, Column int }

// ListEventKind identifies user interactions and pagination triggers emitted
// by a list delegate.
type ListEventKind uint16

// Event kinds reported in ListDelegateEvent.Kind.
const (
	ListSelect ListEventKind = iota
	ListRightClick
	ListConfirm
	ListCancel
	ListLoadMore
	ListSearch
)

// String returns the name of list event kind k.
func (k ListEventKind) String() string {
	return [...]string{"Select", "RightClick", "Confirm", "Cancel", "LoadMore", "Search", "?"}[min(int(k), 6)]
}

// ListDelegateEvent carries a list delegate callback payload, including its
// correlation ID, kind, optional item index, secondary-click flag, and search query.
type ListDelegateEvent struct {
	ID        uint64
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

// ListEvents attaches a ListDelegateEvent handler to a list RowSource.
func ListEvents(f func(cx *gk.Cx, e ListDelegateEvent)) gk.SourceOption {
	return rt.SourceEvents(f, rt.ShapeListEvent)
}

// NewListSource creates a RowSource for a list component using count to keep
// its item count in sync on gk.Refresh and row to build each ListItemEl.
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

// SelectOption configures optional behavior on SelectRow.
type SelectOption uint8

// ScrollIntoView scrolls the list so the newly selected row is visible.
const ScrollIntoView SelectOption = 1

// SelectRow updates the selected row of list (clearing selection when row < 0)
// and optionally scrolls it into view when ScrollIntoView is passed.
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

// DelegateData configures loading, pagination, visibility events, section
// headers/footers, and empty/loading/initial state views on a list or table delegate.
type DelegateData struct {
	Loading           bool
	HasMore           bool
	VisibleEvents     bool
	Headers, Footers  bool
	Threshold         int
	Empty             gk.View
	LoadingView       gk.View
	Initial           gk.View
	PlaceholderHeight float32
}

// SetDelegateData updates the DelegateData configuration on a list or table
// state handle s.
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
