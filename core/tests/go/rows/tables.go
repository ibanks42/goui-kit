// The modes of the table's and tree's row sources (Odin's kit_rows.odin: table_source,
// table_invalidate, tree_source, tree_set_items, tree_entry).
package main

import (
	"fmt"
	"strings"

	"gpuikit.dev/gk"
)

// --- tables -----------------------------------------------------------------------------------

var columns = []gk.TableColumn{
	{Key: "id", Name: "Id", Width: 80, Sort: gk.Some(gk.ColumnSortDefault), Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "name", Name: "Name", Width: 120, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "a", Name: "A", Width: 60, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "b", Name: "B", Width: 60, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "c", Name: "C", Width: 60, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
}

var sortNames = map[gk.ColumnSort]string{gk.ColumnSortDefault: "Default", gk.ColumnSortAscending: "Ascending", gk.ColumnSortDescending: "Descending"}

func (a *App) cell(cx *gk.Cx, at gk.CellAt) gk.Element {
	a.described[key(at.Col, at.Row)]++
	row := at.Row
	if a.sorted {
		row = tableCount - 1 - at.Row
	}
	return gk.Label(fmt.Sprintf("%d:%d", row, at.Col))
}

func (a *App) tableEvent(cx *gk.Cx, e gk.TableDelegateEvent) {
	a.events++
	fmt.Printf("rows table %v row=%d col=%d to=%d sort=%s\n", e.Kind, e.Row, e.Col, e.To, sortNames[e.Sort])
	if e.Kind == gk.TableSort {
		a.sorted = e.Sort == gk.ColumnSortDescending
		gk.InvalidateTable(cx, a.src, 0, tableCount)
		row, ok := a.table.SelectedRow(cx).Get()
		fmt.Printf("rows table-state read selected_row ok=%v row=%d\n", ok, row)
	}
}

// The table state's own events (the generated subscription): selection, double clicks, widths.
func (a *App) tableStateEvent(cx *gk.Cx, e gk.TableEvent) {
	a.events++
	switch v := e.(type) {
	case gk.TableEventSelectRow:
		fmt.Printf("rows table Select_Row row=%d\n", v.Value)
	case gk.TableEventDoubleClickedRow:
		fmt.Printf("rows table Double_Clicked_Row row=%d\n", v.Value)
	case gk.TableEventColumnWidthsChanged:
		fmt.Printf("rows table Column_Widths widths=%v\n", v.Value)
	case gk.TableEventMoveColumn:
		fmt.Printf("rows table state Move_Column from=%d to=%d\n", v.V0, v.V1)
	default:
		fmt.Printf("rows table state %s\n", variant(e))
	}
}

func (a *App) tableInit(cx *gk.Cx) {
	a.src = gk.NewTableSource(cx, tableCount, columns, a.cell, gk.TableEvents(a.tableEvent), gk.Initial(12))
	a.table = gk.NewTableState(cx, a.src).Build()
	a.sub = a.table.OnTableEvent(cx, a.tableStateEvent)
	gk.SetDelegateData(cx, a.table, gk.DelegateData{Threshold: 20, VisibleEvents: true})
}

// --- trees ------------------------------------------------------------------------------------

var treeNodes = []gk.TreeNode{
	{ID: "src", Label: "src", Depth: 0, Expanded: true},
	{ID: "src/a", Label: "a.odin", Depth: 1},
	{ID: "src/b", Label: "b.odin", Depth: 1},
	{ID: "docs", Label: "docs", Depth: 0},
	{ID: "docs/c", Label: "c.md", Depth: 1},
	{ID: "d", Label: "README", Depth: 0},
}

func (a *App) treeRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	a.described[key(0, at.Row)]++
	e, ok := gk.TreeEntryAt(cx, a.tree, at.Row)
	if ok {
		fmt.Printf("rows tree row=%d id=%s depth=%d folder=%v expanded=%v\n", at.Row, e.ID, e.Depth, e.Folder, e.Expanded)
	}
	text := ""
	if ok {
		text = strings.Repeat("  ", e.Depth) + e.Label
	}
	return gk.ListItem(gk.Key("tree", at.Row)).Child(gk.Label(text))
}

func (a *App) treeEvent(cx *gk.Cx, e gk.TreeEvent) {
	ix, ok := a.tree.SelectedIndex(cx).Get()
	fmt.Printf("rows tree-state %s selected ok=%v ix=%d\n", variant(e), ok, ix)
}

func (a *App) treeInit(cx *gk.Cx) {
	a.tree = gk.NewTreeState(cx).Build()
	gk.SetTreeItems(cx, a.tree, treeNodes)
	a.sub = a.tree.OnTreeEvent(cx, a.treeEvent)
	a.treeRows = gk.NewTreeSource(cx, a.treeRow, gk.Initial(8))
}
