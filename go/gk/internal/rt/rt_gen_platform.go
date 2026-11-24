package rt

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

func PutExternalDragPayload(o *Tape, p ExternalDragPayload) {
	o.U32(uint32(len(p.Files)))
	for _, f := range p.Files {
		o.Str(f.Path)
		o.Bool(f.Directory)
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

func RdDragMoveEvent(r *Reader) DragMoveEvent {
	e := RdMouseEvent(r)
	b := rdBounds(r)
	return DragMoveEvent{e, b, RdDragValue(r)}
}

// ImageCache is an image cache the library keeps (gpui's RetainAllImageCache): the images drawn
// under an element given it stay loaded until it is released. gk.Release frees it.
type ImageCache struct{ HCore }

// RetainAllImages is gpui's retain-all cache kept by the element id given (retain_all(id)).
type RetainAllImages struct{ ID string }

// ImageCacheProvider is what Div's ImageCache takes: an ImageCache the app made, or a
// RetainAllImages kept by id (nil: one kept by the id "images").
type ImageCacheProvider interface{ isImageCacheProvider() }

func (ImageCache) isImageCacheProvider() {}

func (RetainAllImages) isImageCacheProvider() {}

func PutImageCacheProvider(o *Tape, p ImageCacheProvider) {
	switch x := p.(type) {
	case ImageCache:
		o.U8(0)
		o.U64(x.h)
	case RetainAllImages:
		o.U8(1)
		o.Str(x.ID)
	default:
		o.U8(1)
		o.Str("images")
	}
}

// NewImageCache makes an image cache (gpui's RetainAllImageCache::new). Keep it in the app's state.
func NewImageCache(cx *Cx) ImageCache {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not make an image cache: make it in init or in a handler")
	}
	t.Flush()
	r := Reader{B: MustCall(*NewReq(CImageCacheNew))}
	return ImageCache{MkH(r.U64())}
}

// ScrollAnchor keeps an element in view of a scroll handle (gpui's ScrollAnchor): an element given
// it with AnchorScroll is scrolled to by gk.ScrollAnchorTo. gk.Release frees it.
type ScrollAnchor struct{ HCore }

// NewScrollAnchor makes an anchor for a scroll handle (ScrollAnchor::for_handle).
func NewScrollAnchor(cx *Cx, handle ScrollHandle) ScrollAnchor {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not make a scroll anchor: make it in init or in a handler")
	}
	t.Flush()
	o := NewReq(CScrollAnchorNew)
	o.U64(handle.h)
	r := Reader{B: MustCall(*o)}
	return ScrollAnchor{MkH(r.U64())}
}

// ScrollAnchorTo scrolls the anchor's handle so the element given the anchor is in view
// (ScrollAnchor::scroll_to), with the turn's window.
func ScrollAnchorTo(cx *Cx, a ScrollAnchor) {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not scroll: scroll in a handler")
	}
	t.Flush()
	o := NewReq(CScrollAnchorTo)
	o.U64(a.h)
	MustCall(*o)
}
