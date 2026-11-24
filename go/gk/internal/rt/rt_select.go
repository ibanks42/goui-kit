package rt

// SearchItem describes one selectable entry in a search or select list.
type SearchItem struct {
	Value    string
	Title    string
	Display  Option[string]
	Keywords string
	Disabled bool
}

// SearchGroup groups SearchItems under an optional section heading.
type SearchGroup struct {
	Title string
	Items []SearchItem
}

// SearchRequest carries a query string and correlation ID delivered to a
// SearchRequests callback when the user types in a searchable list or select.
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

// SearchOption configures optional callbacks on NewSearchSource and
// NewSearchSourceGroups.
type SearchOption func(*searchOpts)

type searchOpts struct {
	row      func(cx *Cx, at RowAt) Element
	headers  bool
	requests func(cx *Cx, e SearchRequest)
}

// SearchRow sets a custom row render callback for each item in a search source.
func SearchRow(row func(cx *Cx, at RowAt) Element) SearchOption {
	return func(o *searchOpts) { o.row = row }
}

// SearchHeaders enables custom rendering of group header rows via SearchRow.
func SearchHeaders() SearchOption { return func(o *searchOpts) { o.headers = true } }

// SearchRequests installs an asynchronous search handler invoked on query changes.
func SearchRequests(f func(cx *Cx, e SearchRequest)) SearchOption {
	return func(o *searchOpts) { o.requests = f }
}

// searches records the feature-flag bitmask (bit 0: custom row, bit 1: async
// requests, bit 2: custom headers) for every RowSource created as a search source.
var searches = map[uint64]uint8{}

// NewSearchSource creates a flat searchable RowSource initialized with items.
func NewSearchSource(cx *Cx, items []SearchItem, opts ...SearchOption) RowSource {
	return newSearchSource(cx, []SearchGroup{{Items: items}}, false, opts)
}

// NewSearchSourceGroups creates a grouped searchable RowSource initialized with groups.
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

// SetSearchItems replaces the items of a flat search RowSource.
func SetSearchItems(cx *Cx, src RowSource, items []SearchItem) {
	putSearch(cx, src, []SearchGroup{{Items: items}}, false)
}

// SetSearchGroups replaces the groups and items of a grouped search RowSource.
func SetSearchGroups(cx *Cx, src RowSource, groups []SearchGroup) {
	putSearch(cx, src, groups, true)
}

// AnswerSearch marks asynchronous search request id on src as fulfilled.
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

// SearchRules constrains selection changes on a search source by minimum count,
// maximum count, locked values that cannot be removed, and blocked values that
// cannot be selected.
type SearchRules struct {
	Min, Max int
	Locked   []string
	Blocked  []string
}

// SearchWillChange installs selection validation rules on search source src.
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
