package kit

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
	"gpuikit.dev/gk/table"
)

// RowMenu wraps a row's element (a table's cell) with the menu its row opens on a right-click.
func RowMenu(el gk.Element, menu PopupMenu) gk.Element {
	src := rt.ElemNode(el)
	n := rt.NewNode(rt.NRowMenu)
	o := rt.NodeArgs(n)
	o.U32(rt.TakeNode(src, false))
	o.U32(rt.TakeValue(rt.N(menu.Core)))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// RowMenuItem is gk.RowMenu for a list's or a tree's row, which is a gk.ListItem: call it last, on
// the finished item (later builder calls would go to the wrapper, which ignores them).
func RowMenuItem(item table.ListItemEl, menu PopupMenu) table.ListItemEl {
	return table.ListItemEl{ElAnimatableElementSelectableNode: rt.MkElAnimatableElementSelectableNode(rt.ElemNode(RowMenu(item, menu)))}
}
