// The modes of the table's and tree's row sources.
package main

import (
	"fmt"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/table"
)

// --- tables -----------------------------------------------------------------------------------

var columns = []table.TableColumn{
	{Key: "id", Name: "Id", Width: 80, Sort: gk.Some(table.ColumnSortDefault), Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "name", Name: "Name", Width: 120, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "a", Name: "A", Width: 60, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "b", Name: "B", Width: 60, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "c", Name: "C", Width: 60, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
}

var sortNames = map[table.ColumnSort]string{table.ColumnSortDefault: "Default", table.ColumnSortAscending: "Ascending", table.ColumnSortDescending: "Descending"}

func (a *App) cell(cx *gk.Cx, at table.CellAt) gk.Element {
	a.described[key(at.Col, at.Row)]++
	row := at.Row
	if a.sorted {
		row = tableCount - 1 - at.Row
	}
	return kit.Label(fmt.Sprintf("%d:%d", row, at.Col))
}

func (a *App) tableEvent(cx *gk.Cx, e table.DelegateEvent) {
	a.events++
	fmt.Printf("rows table %v row=%d col=%d to=%d sort=%s\n", e.Kind, e.Row, e.Col, e.To, sortNames[e.Sort])
	if e.Kind == table.Sort {
		a.sorted = e.Sort == table.ColumnSortDescending
		table.InvalidateTable(cx, a.src, 0, tableCount)
		row, ok := a.table.SelectedRow(cx).Get()
		fmt.Printf("rows table-state read selected_row ok=%v row=%d\n", ok, row)
	}
}

// The table state's own events (the generated subscription): selection, double clicks, widths.
func (a *App) tableStateEvent(cx *gk.Cx, e table.Event) {
	a.events++
	switch v := e.(type) {
	case table.EventSelectRow:
		fmt.Printf("rows table Select_Row row=%d\n", v.Value)
	case table.EventDoubleClickedRow:
		fmt.Printf("rows table Double_Clicked_Row row=%d\n", v.Value)
	case table.EventColumnWidthsChanged:
		fmt.Printf("rows table Column_Widths widths=%v\n", v.Value)
	case table.EventMoveColumn:
		fmt.Printf("rows table state Move_Column from=%d to=%d\n", v.V0, v.V1)
	default:
		fmt.Printf("rows table state %s\n", variant(e))
	}
}

func (a *App) tableInit(cx *gk.Cx) {
	a.src = table.NewSource(cx, tableCount, columns, a.cell, table.Events(a.tableEvent), gk.Initial(12))
	a.table = table.NewState(cx, a.src).Build()
	a.sub = a.table.OnTableEvent(cx, a.tableStateEvent)
	table.SetDelegateData(cx, a.table, table.DelegateData{Threshold: 20, VisibleEvents: true})
}

// --- trees ------------------------------------------------------------------------------------

var treeNodes = []table.TreeNode{
	{ID: "src", Label: "src", Depth: 0, Expanded: true},
	{ID: "src/a", Label: "a.demo", Depth: 1},
	{ID: "src/b", Label: "b.demo", Depth: 1},
	{ID: "docs", Label: "docs", Depth: 0},
	{ID: "docs/c", Label: "c.md", Depth: 1},
	{ID: "d", Label: "README", Depth: 0},
}

func (a *App) treeRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	a.described[key(0, at.Row)]++
	e, ok := table.TreeEntryAt(cx, a.tree, at.Row)
	if ok {
		fmt.Printf("rows tree row=%d id=%s depth=%d folder=%v expanded=%v\n", at.Row, e.ID, e.Depth, e.Folder, e.Expanded)
	}
	text := ""
	if ok {
		text = strings.Repeat("  ", e.Depth) + e.Label
	}
	return table.ListItem(gk.Key("tree", at.Row)).Child(kit.Label(text))
}

func (a *App) treeEvent(cx *gk.Cx, e table.TreeEvent) {
	ix, ok := a.tree.SelectedIndex(cx).Get()
	fmt.Printf("rows tree-state %s selected ok=%v ix=%d\n", variant(e), ok, ix)
}

func (a *App) treeInit(cx *gk.Cx) {
	a.tree = table.NewTreeState(cx).Build()
	table.SetTreeItems(cx, a.tree, treeNodes)
	a.sub = a.tree.OnTreeEvent(cx, a.treeEvent)
	a.treeRows = table.NewTreeSource(cx, a.treeRow, gk.Initial(8))
}
