package gk

// Decorate is what an element upstream hands a decorator would get, given as data (upstream calls
// the decorator while it renders, where the app cannot run): Style refines the element's own style,
// and Content, a row source (zero: none), adds its row 0 as the element's child.
type Decorate struct {
	Style   Option[StyleRefinement]
	Content RowSource
}
