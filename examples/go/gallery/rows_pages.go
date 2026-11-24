// The rows family's gallery pages, the Go twin of ref/src/bin/gallery/rows_pages.rs,
// call for call: a list of 10,000 rows, a table of
// 10,000 rows with a sortable column and a tree, over row sources (docs/PROTOCOLS.md section
// 9.5). Each row or cell is a view described once; the steps of ref/gallery/pages.txt click,
// press keys and scroll (a far scroll and a jump to the last row draw placeholders for one
// frame, and the shot follows the next frame).
package main

import (
	"fmt"
	"strings"

	"gpuikit.dev/gk"
)

const (
	listRows  = 10_000
	tableRows = 10_000
)

type rowsPages struct {
	list     gk.ListState
	table    gk.TableState
	cells    gk.RowSource // the table's
	tree     gk.TreeState
	treeRows gk.RowSource
	desc     bool // the table's rows in descending order of their id
}

var rowsPageTable = []Page{{"rows-list", rowsList}, {"rows-table", rowsTable}, {"rows-tree", rowsTreePage}}

var rowsTree = []gk.TreeNode{
	{ID: "docs", Label: "docs", Depth: 0},
	{ID: "docs/guide", Label: "guide.md", Depth: 1},
	{ID: "docs/api", Label: "api.md", Depth: 1},
	{ID: "src", Label: "src", Depth: 0, Expanded: true},
	{ID: "src/a", Label: "a.demo", Depth: 1},
	{ID: "src/b", Label: "b.demo", Depth: 1},
	{ID: "readme", Label: "README", Depth: 0},
}

func rowsPagesInit(a *App, cx *gk.Cx) {
	p := &a.rowsFam
	p.list = gk.NewListState(cx, gk.NewListSource(cx, func() int { return listRows }, rowsListRow)).Build()
	id := gk.NewTableColumn("id", "Id")
	id.Sort = gk.Some(gk.ColumnSortDefault)
	columns := []gk.TableColumn{id, gk.NewTableColumn("a", "A"), gk.NewTableColumn("b", "B"), gk.NewTableColumn("c", "C")}
	p.cells = gk.NewTableSource(cx, tableRows, columns, a.rowsCell, gk.TableEvents(a.rowsTableEvent))
	p.table = gk.NewTableState(cx, p.cells).Build()
	p.tree = gk.NewTreeState(cx).Build()
	gk.SetTreeItems(cx, p.tree, rowsTree)
	p.treeRows = gk.NewTreeSource(cx, a.rowsTreeRow)
}

func (a *App) rowsCell(cx *gk.Cx, at gk.CellAt) gk.Element {
	row := at.Row
	if a.rowsFam.desc {
		row = tableRows - 1 - at.Row
	}
	return gk.Label(fmt.Sprintf("%d:%d", row, at.Col))
}

func (a *App) rowsTableEvent(cx *gk.Cx, e gk.TableDelegateEvent) {
	if e.Kind == gk.TableSort {
		a.rowsFam.desc = e.Sort == gk.ColumnSortDescending
		gk.InvalidateTable(cx, a.rowsFam.cells, 0, tableRows)
	}
}

func (a *App) rowsTreeRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	text := ""
	if e, ok := gk.TreeEntryAt(cx, a.rowsFam.tree, at.Row); ok {
		text = strings.Repeat("  ", e.Depth) + e.Label
	}
	return gk.ListItem(gk.Key("tree", at.Row)).Child(gk.Label(text))
}

func rowsListRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	return gk.ListItem(gk.Key("item", at.Row)).Child(gk.Label(fmt.Sprintf("row %d", at.Row)))
}

func rowsList(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(gk.List(a.rowsFam.list).ScrollbarVisible(false))
}

func rowsTable(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(440)).H(gk.Px(240)).Child(gk.DataTable(a.rowsFam.table).ScrollbarVisible(false, false))
}

func rowsTreePage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(gk.Tree(a.rowsFam.tree, a.rowsFam.treeRows))
}
