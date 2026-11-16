package gk

// The searchable-list family of the delegate protocol (docs/PROTOCOLS.md section 9.9), the port of
// odin/gk/kit_select.odin: the kit's Select and Combobox, whose states are generic over a
// delegate the app implements upstream. The binding's delegate lives in the library over a row
// source; the app gives it items as data, and optionally a row function that draws each item,
// header rows for groups, and a handler that answers searches.
//
//	src := gk.NewSearchSource(cx, []gk.SearchItem{{Value: "apple"}, {Value: "pear", Title: "Pear"}})
//	a.fruit = gk.NewSelectState(cx, src, gk.NoRow).Searchable(true).Build()   gk.Some(gk.Row(0)): the first
//	view: gk.Select(a.fruit).Placeholder("Fruit")
//	a.fruit.SelectedValue(cx)                                                  an Option[string]: the item's Value
//
// Searching filters the items in the library as upstream's SearchableVec does (the title or the
// keywords contain the query, ignoring case; a group stays while an item or its title matches).
// With gk.SearchRequests the source asks the app instead: the handler gets a SearchRequest,
// replaces the items (gk.SetSearchItems), sets them on the state (.SetItems(cx, src)) and
// answers (gk.AnswerSearch(cx, src, e.ID)); until then the menu shows the items it had.

// SearchItem is one item of a select or combobox (upstream's SearchableListItem as data).
type SearchItem struct {
	Value    string         // identifies the item: what SelectedValue and the events report
	Title    string         // shown in the menu and, unless Display is set, in the trigger; "" is Value
	Display  Option[string] // shown in the trigger instead of the title
	Keywords string         // searched besides the title
	Disabled bool           // shown, but cannot be chosen
}

// SearchGroup is a titled group of items (upstream's SearchableGroup).
type SearchGroup struct {
	Title string
	Items []SearchItem
}

// SearchRequest is the event of a source made with gk.SearchRequests: the query changed.
type SearchRequest struct {
	ID    uint64
	Query string
}

func init() {
	registerShape(shapeSearchEvent, shapeInfo{"func(cx *Cx, e SearchRequest)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, SearchRequest))
		if ok {
			f(cx, SearchRequest{r.u64(), r.str()})
		}
		return ok
	}})
}

// SearchOption changes a source made by NewSearchSource.
type SearchOption func(*searchOpts)

type searchOpts struct {
	row      func(cx *Cx, at RowAt) Element
	headers  bool
	requests func(cx *Cx, e SearchRequest)
}

// SearchRow has the kit draw each item's row with row (at: the group and the item's index in it)
// instead of its title.
func SearchRow(row func(cx *Cx, at RowAt) Element) SearchOption {
	return func(o *searchOpts) { o.row = row }
}

// SearchHeaders has the row function also describe each group's header row (at.Row is the header's marker, as in Odin's gk.HEADER).
func SearchHeaders() SearchOption { return func(o *searchOpts) { o.headers = true } }

// SearchRequests makes searches requests to the app (SearchRequest).
func SearchRequests(f func(cx *Cx, e SearchRequest)) SearchOption {
	return func(o *searchOpts) { o.requests = f }
}

var searches = map[uint64]uint8{} // source -> flags: 1 rows draw the items, 2 requests, 4 headers

// NewSearchSource makes the row source of a select or combobox over a flat list of items. Needs
// a window: init or a handler.
func NewSearchSource(cx *Cx, items []SearchItem, opts ...SearchOption) RowSource {
	return newSearchSource(cx, []SearchGroup{{Items: items}}, false, opts)
}

// NewSearchSourceGroups is NewSearchSource over titled groups.
func NewSearchSourceGroups(cx *Cx, groups []SearchGroup, opts ...SearchOption) RowSource {
	return newSearchSource(cx, groups, true, opts)
}

func newSearchSource(cx *Cx, groups []SearchGroup, grouped bool, opts []SearchOption) RowSource {
	var so searchOpts
	for _, o := range opts {
		o(&so)
	}
	if so.headers && so.row == nil {
		stop("gk.SearchHeaders: header rows need a row function (gk.SearchRow)")
	}
	row, first, initial := so.row, 0, 0
	if row != nil {
		initial = 64
		if len(groups) > 0 {
			first = len(groups[0].Items)
		}
	} else {
		row = func(*Cx, RowAt) Element {
			stop("a search source without a row function was asked for a row: give gk.SearchRow(f) to gk.NewSearchSource when the select draws its own rows")
			return nil
		}
	}
	sopts := []SourceOption{Initial(initial)}
	if so.requests != nil {
		sopts = append(sopts, sourceEvents(so.requests, shapeSearchEvent))
	}
	src := NewRowSource(cx, first, row, sopts...)
	var flags uint8
	if so.row != nil {
		flags |= 1
	}
	if so.requests != nil {
		flags |= 2
	}
	if so.headers {
		flags |= 4
	}
	searches[src.h] = flags
	putSearch(cx, src, groups, grouped)
	return src
}

// SetSearchItems replaces the items of a select's or combobox's source (a buffered write). A
// state made over it before shows them after .SetItems(cx, src).
func SetSearchItems(cx *Cx, src RowSource, items []SearchItem) {
	putSearch(cx, src, []SearchGroup{{Items: items}}, false)
}

// SetSearchGroups is SetSearchItems for titled groups.
func SetSearchGroups(cx *Cx, src RowSource, groups []SearchGroup) {
	putSearch(cx, src, groups, true)
}

// AnswerSearch answers a SearchRequest once the items are set.
func AnswerSearch(cx *Cx, src RowSource, id uint64) {
	t := checkCx(cx)
	var body tape
	body.bool(true)
	o := newReq(cSourceAnswer)
	o.u64(src.h)
	o.u64(id)
	o.bytes(body)
	t.deferOp(*o)
}

func putSearch(cx *Cx, src RowSource, groups []SearchGroup, grouped bool) {
	t := checkCx(cx)
	flags, ok := searches[src.h]
	if !ok {
		stop("gk.SetSearchItems was given a row source gk.NewSearchSource did not make")
	}
	if flags&1 != 0 {
		// Rows are keyed by (group, the item's index in it): every group needs its count.
		for s, g := range groups {
			SetSectionCount(cx, src, s, len(g.Items))
		}
	}
	o := newReq(cSearchItems)
	o.u64(src.h)
	o.u8(flags)
	o.u32(uint32(len(groups)))
	for _, g := range groups {
		o.bool(grouped)
		if grouped {
			o.str(g.Title)
		}
		o.u32(uint32(len(g.Items)))
		for _, it := range g.Items {
			title := it.Title
			if title == "" {
				title = it.Value
			}
			o.str(it.Value)
			o.str(title)
			putOptStrOpt(o, it.Display)
			o.str(it.Keywords)
			o.bool(it.Disabled)
		}
	}
	t.deferOp(*o)
}
