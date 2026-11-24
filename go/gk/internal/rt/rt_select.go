package rt

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
	RegisterShape(ShapeSearchEvent, ShapeInfo{"func(cx *Cx, e SearchRequest)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, SearchRequest))
		if ok {
			f(cx, SearchRequest{r.U64(), r.Str()})
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

// SearchHeaders has the row function also describe each group's header row (at.Row is the header's marker, HeaderRow).
func SearchHeaders() SearchOption { return func(o *searchOpts) { o.headers = true } }

// SearchRequests makes searches requests to the app (SearchRequest).
func SearchRequests(f func(cx *Cx, e SearchRequest)) SearchOption {
	return func(o *searchOpts) { o.requests = f }
}

var searches = map[uint64]uint8{}

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
		Stop("gk.SearchHeaders: header rows need a row function (gk.SearchRow)")
	}
	row, first, initial := so.row, 0, 0
	if row != nil {
		initial = 64
		if len(groups) > 0 {
			first = len(groups[0].Items)
		}
	} else {
		row = func(*Cx, RowAt) Element {
			Stop("a search source without a row function was asked for a row: give gk.SearchRow(f) to gk.NewSearchSource when the select draws its own rows")
			return nil
		}
	}
	sopts := []SourceOption{Initial(initial)}
	if so.requests != nil {
		sopts = append(sopts, SourceEvents(so.requests, ShapeSearchEvent))
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
	t := CheckCx(cx)
	var body Tape
	body.Bool(true)
	o := NewReq(CSourceAnswer)
	o.U64(src.h)
	o.U64(id)
	o.Bytes(body)
	t.DeferOp(*o)
}

func putSearch(cx *Cx, src RowSource, groups []SearchGroup, grouped bool) {
	t := CheckCx(cx)
	flags, ok := searches[src.h]
	if !ok {
		Stop("gk.SetSearchItems was given a row source gk.NewSearchSource did not make")
	}
	if flags&1 != 0 {
		// Rows are keyed by (group, the item's index in it): every group needs its count.
		for s, g := range groups {
			SetSectionCount(cx, src, s, len(g.Items))
		}
	}
	o := NewReq(CSearchItems)
	o.U64(src.h)
	o.U8(flags)
	o.U32(uint32(len(groups)))
	for _, g := range groups {
		o.Bool(grouped)
		if grouped {
			o.Str(g.Title)
		}
		o.U32(uint32(len(g.Items)))
		for _, it := range g.Items {
			title := it.Title
			if title == "" {
				title = it.Value
			}
			o.Str(it.Value)
			o.Str(title)
			PutOptStrOpt(o, it.Display)
			o.Str(it.Keywords)
			o.Bool(it.Disabled)
		}
	}
	t.DeferOp(*o)
}

// SearchRules is what a select's or combobox's selection may become, given in advance: upstream
// asks its delegate (SearchableListDelegate::on_will_change) while the list handles a click, where
// app code may not run, so the library applies upstream's change and then these rules. Blocked
// values are never added; Locked values are never taken out once selected; a change that would
// leave fewer than Min or more than Max values selected (0: no limit) is refused whole.
type SearchRules struct {
	Min, Max int
	Locked   []string
	Blocked  []string
}

// SearchWillChange gives a select's or combobox's source its rules (buffered; replaces the
// source's rules; the zero SearchRules is upstream's default alone).
func SearchWillChange(cx *Cx, src RowSource, rules SearchRules) {
	t := CheckCx(cx)
	if _, ok := searches[src.h]; !ok {
		Stop("gk.SearchWillChange was given a row source gk.NewSearchSource did not make")
	}
	if rules.Min < 0 || rules.Max < 0 || (rules.Max > 0 && rules.Min > rules.Max) {
		Stop("gk.SearchWillChange: min %d and max %d cannot both hold: min must not be greater than max", rules.Min, rules.Max)
	}
	o := NewReq(CSearchRules)
	o.U64(src.h)
	o.U32(uint32(rules.Min))
	o.U32(uint32(rules.Max))
	for _, list := range [2][]string{rules.Locked, rules.Blocked} {
		o.U32(uint32(len(list)))
		for _, v := range list {
			o.Str(v)
		}
	}
	t.DeferOp(*o)
}
