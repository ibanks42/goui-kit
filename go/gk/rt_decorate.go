package gk

// Decorate configures visual styling and optional RowSource content attached
// to a decorated text or editor range.
type Decorate struct {
	Style   Option[StyleRefinement]
	Content RowSource
}
