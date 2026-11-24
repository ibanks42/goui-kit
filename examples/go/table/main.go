// table: the `table` scene in Go, the same table as ref/src/bin/table.rs:
// the kit's DataTable with `rows` rows of `cols` columns over a row source
// whose rows are the cells (docs/PROTOCOLS.md section 9.5). Each cell is a view described once,
// when it is first drawn near the visible rows, and kept while the table scrolls; the
// frame-cost gate scrolls it before every frame with GK_SCROLL (gates/fps.sh table).
//
//	table-go [rows] [cols]      (default 100000 6)
package main

import (
	"fmt"
	"os"
	"strconv"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	rows, cols int
	src        gk.RowSource
	table      gk.TableState
	root       gk.View
}

func (a *App) cell(cx *gk.Cx, at gk.CellAt) gk.Element {
	return gk.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
}

func (a *App) init(cx *gk.Cx) {
	columns := make([]gk.TableColumn, a.cols)
	for i := range columns {
		columns[i] = gk.NewTableColumn(fmt.Sprintf("c%d", i), fmt.Sprintf("Column %d", i))
	}
	a.src = gk.NewTableSource(cx, a.rows, columns, a.cell)
	a.table = gk.NewTableState(cx, a.src).Build()
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.DataTable(a.table))
}

func arg(i, def int) int {
	if len(os.Args) > i {
		if v, err := strconv.Atoi(os.Args[i]); err == nil {
			return v
		}
	}
	return def
}

func main() {
	a := &App{rows: arg(1, 100_000), cols: arg(2, 6)}
	gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.rootView)
		gk.OpenWindow(cx, "Table", 900, 600, a.root)
	})
}
