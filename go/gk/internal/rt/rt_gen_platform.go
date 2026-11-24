package rt

// ExternalFile describes one filesystem path dragged into the application from
// outside GPUI.
type ExternalFile struct {
	Path      string
	Directory bool
}

// ExternalDragPayload carries a list of external files dropped onto a target.
type ExternalDragPayload struct {
	Files []ExternalFile
}

// PutExternalDragPayload encodes p onto o.
func PutExternalDragPayload(o *Tape, p ExternalDragPayload) {
	o.U32(uint32(len(p.Files)))
	for _, f := range p.Files {
		o.Str(f.Path)
		o.Bool(f.Directory)
	}
}

// A11yChild describes a child accessibility node with a stable key, role, and label.
type A11yChild struct {
	Key   string
	Role  Role
	Label string
}

// DragMoveEvent carries pointer coordinates, target element bounds, and the
// active DragValue during a drag-move callback.
type DragMoveEvent struct {
	MouseEvent
	Bounds Bounds
	Value  DragValue
}

// RdDragMoveEvent decodes a DragMoveEvent from r.
func RdDragMoveEvent(r *Reader) DragMoveEvent {
	e := RdMouseEvent(r)
	b := rdBounds(r)
	return DragMoveEvent{e, b, RdDragValue(r)}
}

// ImageCache is a handle to a GPUI image cache entity shared across elements.
type ImageCache struct{ HCore }

// RetainAllImages selects a named GPUI retain-all image cache provider.
type RetainAllImages struct{ ID string }

// ImageCacheProvider is implemented by ImageCache and RetainAllImages.
type ImageCacheProvider interface{ isImageCacheProvider() }

func (ImageCache) isImageCacheProvider() {}

func (RetainAllImages) isImageCacheProvider() {}

// PutImageCacheProvider encodes p onto o, defaulting to RetainAllImages{"images"}
// when p is nil.
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

// NewImageCache allocates an ImageCache handle outside a view function.
func NewImageCache(cx *Cx) ImageCache {
	t := CheckCx(cx)
	if Vb.Active {
		Stop("a view function may not make an image cache: make it in init or in a handler")
	}
	t.Flush()
	r := Reader{B: MustCall(*NewReq(CImageCacheNew))}
	return ImageCache{MkH(r.U64())}
}

// ScrollAnchor is a handle bound to a ScrollHandle that can scroll an element
// into view on demand.
type ScrollAnchor struct{ HCore }

// NewScrollAnchor creates a ScrollAnchor attached to handle outside a view function.
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

// ScrollAnchorTo scrolls the container associated with a so the anchored
// element becomes visible.
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
