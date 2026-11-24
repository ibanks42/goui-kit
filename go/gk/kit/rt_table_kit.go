package kit

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
	"gpuikit.dev/gk/table"
)

// RowMenu attaches a context PopupMenu to any row element el.
func RowMenu(el gk.Element, menu PopupMenu) gk.Element {
	src := rt.ElemNode(el)
	n := rt.NewNode(rt.NRowMenu)
	o := rt.NodeArgs(n)
	o.U32(rt.TakeNode(src, false))
	o.U32(rt.TakeValue(rt.N(menu.Core)))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// RowMenuItem attaches a context PopupMenu to item and preserves its
// table.ListItemEl type so it fits directly into list builders.
func RowMenuItem(item table.ListItemEl, menu PopupMenu) table.ListItemEl {
	return table.ListItemEl{ElAnimatableElementSelectableNode: rt.MkElAnimatableElementSelectableNode(rt.ElemNode(RowMenu(item, menu)))}
}
