package gk

// gpui's element leftovers the generator's descriptor kinds carry (ported from
// odin/gk/rt_platform.odin; docs/PROTOCOLS.md 13.31): a drag's files outside the window, an
// element's drag moves, its prepaint order, synthetic accessibility children, image caches and
// scroll anchors.
//
//	gk.Div().ID("file").OnDrag(v, preview).ExternalDragPayload(gk.ExternalDragPayload{Files: []gk.ExternalFile{{Path: "/home/me/a.txt"}}})
//	gk.Div().OnDragMove(func(cx *gk.Cx, e gk.DragMoveEvent) { .. })
//	gk.Div().WithDynamicPrepaintOrder([]int{2, 0, 1})
//	gk.Div().ID("chart").A11ySyntheticChildren([]gk.A11yChild{{Key: "q1", Role: gk.RoleListItem, Label: "Q1: 12"}})
//	cache := gk.NewImageCache(cx); gk.Div().ImageCache(cache); gk.Img(src).ImageCache(cache)
//	anchor := gk.NewScrollAnchor(cx, handle); gk.Div().ID("x").AnchorScroll(gk.Some(anchor)); gk.ScrollAnchorTo(cx, anchor)

// ExternalFile is a file (or a directory) handed to the platform when a drag leaves the window.
type ExternalFile struct {
	Path      string
	Directory bool
}

// ExternalDragPayload is what ExternalDragPayload takes: the files a drag of the app's value
// becomes outside the window (none: the drag stays inside the app).
type ExternalDragPayload struct {
	Files []ExternalFile
}

func putExternalDragPayload(o *tape, p ExternalDragPayload) {
	o.u32(uint32(len(p.Files)))
	for _, f := range p.Files {
		o.str(f.Path)
		o.bool(f.Directory)
	}
}

// A11yChild is one synthetic accessibility node A11ySyntheticChildren adds under the element: its
// key (its id is derived from it and the element's, as upstream does), its role and its label.
type A11yChild struct {
	Key   string
	Role  Role
	Label string
}

// DragMoveEvent is gpui's DragMoveEvent of the library's drag value: where the pointer is (as a
// MouseEvent), the bounds of the element listening, and the value being dragged.
type DragMoveEvent struct {
	MouseEvent
	Bounds Bounds
	Value  DragValue
}

func rdDragMoveEvent(r *reader) DragMoveEvent {
	e := rdMouseEvent(r)
	b := rdBounds(r)
	return DragMoveEvent{e, b, rdDragValue(r)}
}

// ImageCache is an image cache the library keeps (gpui's RetainAllImageCache): the images drawn
// under an element given it stay loaded until it is released. gk.Release frees it.
type ImageCache struct{ h uint64 }

func (c ImageCache) handle() uint64 { return c.h }

// RetainAllImages is gpui's retain-all cache kept by the element id given (retain_all(id)).
type RetainAllImages struct{ ID string }

// ImageCacheProvider is what Div's ImageCache takes: an ImageCache the app made, or a
// RetainAllImages kept by id (nil: one kept by the id "images").
type ImageCacheProvider interface{ isImageCacheProvider() }

func (ImageCache) isImageCacheProvider()      {}
func (RetainAllImages) isImageCacheProvider() {}

func putImageCacheProvider(o *tape, p ImageCacheProvider) {
	switch x := p.(type) {
	case ImageCache:
		o.u8(0)
		o.u64(x.h)
	case RetainAllImages:
		o.u8(1)
		o.str(x.ID)
	default:
		o.u8(1)
		o.str("images")
	}
}

// NewImageCache makes an image cache (gpui's RetainAllImageCache::new). Keep it in the app's state.
func NewImageCache(cx *Cx) ImageCache {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not make an image cache: make it in init or in a handler")
	}
	t.flush()
	r := reader{b: mustCall(*newReq(cImageCacheNew))}
	return ImageCache{r.u64()}
}

// ScrollAnchor keeps an element in view of a scroll handle (gpui's ScrollAnchor): an element given
// it with AnchorScroll is scrolled to by gk.ScrollAnchorTo. gk.Release frees it.
type ScrollAnchor struct{ h uint64 }

func (a ScrollAnchor) handle() uint64 { return a.h }

// NewScrollAnchor makes an anchor for a scroll handle (ScrollAnchor::for_handle).
func NewScrollAnchor(cx *Cx, handle ScrollHandle) ScrollAnchor {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not make a scroll anchor: make it in init or in a handler")
	}
	t.flush()
	o := newReq(cScrollAnchorNew)
	o.u64(handle.h)
	r := reader{b: mustCall(*o)}
	return ScrollAnchor{r.u64()}
}

// ScrollAnchorTo scrolls the anchor's handle so the element given the anchor is in view
// (ScrollAnchor::scroll_to), with the turn's window.
func ScrollAnchorTo(cx *Cx, a ScrollAnchor) {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not scroll: scroll in a handler")
	}
	t.flush()
	o := newReq(cScrollAnchorTo)
	o.u64(a.h)
	mustCall(*o)
}
