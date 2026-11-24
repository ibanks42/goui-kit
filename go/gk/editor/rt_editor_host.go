package editor

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
)

// LeafDecoration associates colored byte-range highlights with a specific
// block leaf key rendered inside BlockHost.
type LeafDecoration struct {
	Key    uint64
	Ranges []gk.TupleRangeColor
}

// LeafEntry pairs a leaf key with its frame-temporary LeafGeometry handle.
type LeafEntry struct {
	Key      uint64
	Geometry LeafGeometry
}

// LeafGeometryEvent delivers layout geometry handles for leaves laid out by
// BlockHost during the current frame. Geometry handles are frame-temporary
// unless retained via gk.Retain.
type LeafGeometryEvent struct {
	Leaves []LeafEntry
}

func init() {
	rt.RegisterShape(rt.ShapeLeafGeometry, rt.ShapeInfo{Name: "func(cx *Cx, e LeafGeometryEvent)", Call: func(h any, cx *gk.Cx, r *rt.Reader, ret *rt.Tape) bool {
		f, ok := h.(func(*gk.Cx, LeafGeometryEvent))
		if ok {
			e := LeafGeometryEvent{Leaves: make([]LeafEntry, int(r.U32()))}
			for i := range e.Leaves {
				e.Leaves[i].Key = r.U64()
				e.Leaves[i].Geometry = LeafGeometry{HCore: rt.MkH(r.U64())}
			}
			f(cx, e)
		}
		return ok
	}})
}

// BlockHost renders a sequence of structured document BlockNodes with optional
// per-block FlowPositions, RenderContext, LeafDecorations, and a geometry
// callback invoked after layout.
func BlockHost(id string, blocks []BlockNode, flows []gk.Option[FlowPosition], renderContext gk.Option[RenderContext],
	decorations []LeafDecoration, onGeometry func(cx *gk.Cx, e LeafGeometryEvent)) gk.Element {
	n := rt.NewNode(rt.NEditorHost)
	o := rt.NodeArgs(n)
	o.Str(id)
	o.U32(uint32(len(blocks)))
	for _, b := range blocks {
		putBlockNode(o, b)
	}
	o.U32(uint32(len(flows)))
	for _, f := range flows {
		if v, ok := f.Get(); ok {
			o.U8(1)
			o.U32(rt.TakeValue(rt.N(v.Core)))
		} else {
			o.U8(0)
		}
	}
	if v, ok := renderContext.Get(); ok {
		o.U8(1)
		o.U32(rt.TakeValue(rt.N(v.Core)))
	} else {
		o.U8(0)
	}
	o.U32(uint32(len(decorations)))
	for _, d := range decorations {
		o.U64(d.Key)
		o.U32(uint32(len(d.Ranges)))
		for _, x := range d.Ranges {
			rt.PutRange(o, x.V0)
			rt.PutColor(o, x.V1)
		}
	}
	if onGeometry != nil {
		o.U8(1)
		o.U32(rt.Register(onGeometry, rt.ShapeLeafGeometry))
	} else {
		o.U8(0)
	}
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}
