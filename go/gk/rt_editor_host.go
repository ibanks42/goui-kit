package gk

// The editor host's element (docs/PROTOCOLS.md 9.30): upstream's editor_host renders a BlockNode tree and, around the
// blocks' prepaint, collects each keyed text leaf's geometry (collect_leaf_geometry). The library's
// element (core/src/editor_text.rs BlockHost) does both and hands the leaves to the app after the
// frame (app code never runs mid-frame).
//
//	blocks := []gk.BlockNode{gk.BlockNodeParagraph{Value: gk.NewParagraphDefault().Push(gk.NewInlineNode("Hello")).WithLeafKey(1)}}
//	gk.BlockHost("doc", blocks, nil, gk.None[gk.RenderContext](), nil, a.onLeaves)   in a view function
//	func (a *App) onLeaves(cx *gk.Cx, e gk.LeafGeometryEvent) { for _, l := range e.Leaves { ... l.Geometry.Bounds(cx) } }

// LeafDecoration is byte ranges of one keyed leaf's text painted behind it (a selection, find
// matches), as upstream's collect_leaf_geometry takes them.
type LeafDecoration struct {
	Key    uint64
	Ranges []TupleRangeColor
}

// LeafEntry is one keyed text leaf the host laid out, and its geometry (a temporary library object:
// gk.Retain keeps it).
type LeafEntry struct {
	Key      uint64
	Geometry LeafGeometry
}

// LeafGeometryEvent is the payload of BlockHost's onGeometry, after the frame the leaves were laid
// out in.
type LeafGeometryEvent struct {
	Leaves []LeafEntry
}

func init() {
	registerShape(shapeLeafGeometry, shapeInfo{"func(cx *Cx, e LeafGeometryEvent)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, LeafGeometryEvent))
		if ok {
			e := LeafGeometryEvent{Leaves: make([]LeafEntry, int(r.u32()))}
			for i := range e.Leaves {
				e.Leaves[i].Key = r.u64()
				e.Leaves[i].Geometry = LeafGeometry{r.u64()}
			}
			f(cx, e)
		}
		return ok
	}})
}

// BlockHost is the editor host: the app's blocks drawn as TextView draws them (each with its flow,
// or its place among the blocks: flows may be shorter than blocks, or nil; the render context, or
// TextView's), the decorations painted behind the keyed leaves, and onGeometry (nil: none) given
// each keyed leaf's geometry after the frame, when the leaves changed. A parsed document's blocks
// are gk.BaseParseMarkdown's held values.
func BlockHost(id string, blocks []BlockNode, flows []Option[FlowPosition], renderContext Option[RenderContext],
	decorations []LeafDecoration, onGeometry func(cx *Cx, e LeafGeometryEvent)) Element {
	n := newNode(nEditorHost)
	o := nodeArgs(n)
	o.str(id)
	o.u32(uint32(len(blocks)))
	for _, b := range blocks {
		putBlockNode(o, b)
	}
	o.u32(uint32(len(flows)))
	for _, f := range flows {
		if v, ok := f.Get(); ok {
			o.u8(1)
			o.u32(takeValue(v.n))
		} else {
			o.u8(0)
		}
	}
	if v, ok := renderContext.Get(); ok {
		o.u8(1)
		o.u32(takeValue(v.n))
	} else {
		o.u8(0)
	}
	o.u32(uint32(len(decorations)))
	for _, d := range decorations {
		o.u64(d.Key)
		o.u32(uint32(len(d.Ranges)))
		for _, x := range d.Ranges {
			putRange(o, x.V0)
			putColor(o, x.V1)
		}
	}
	if onGeometry != nil {
		o.u8(1)
		o.u32(register(onGeometry, shapeLeafGeometry))
	} else {
		o.u8(0)
	}
	return valueEl{n}
}
