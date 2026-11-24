// Large DataTable example: renders a virtualized table (default 100,000 rows by 6 columns)
// backed by a row source. Used by frame-cost scroll gates against ref-table.
// Usage: table [rows] [cols]
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	gktable "gpuikit.dev/gk/table"
	"gpuikit.dev/gk/theme"
)

type App struct {
	rows, cols int
	src        gk.RowSource
	table      gktable.State
	root       gk.View
}

// cell describes one cell at (at.Row, at.Col). The table row source packs every column
// of a visible row into a single N_TABLE_CELLS tape pre-parsed by Rust during Tape::parse.
func (a *App) cell(cx *gk.Cx, at gktable.CellAt) gk.Element {
	return kit.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
}

// init constructs column definitions, the virtualized row source, and the TableState.
func (a *App) init(cx *gk.Cx) {
	columns := make([]gktable.TableColumn, a.cols)
	for i := range columns {
		columns[i] = gktable.NewTableColumn(fmt.Sprintf("c%d", i), fmt.Sprintf("Column %d", i))
	}
	a.src = gktable.NewSource(cx, a.rows, columns, a.cell)
	a.table = gktable.NewState(cx, a.src).Build()
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gktable.DataTable(a.table))
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
	if err := gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.rootView)
		gk.OpenWindow(cx, "Table", 900, 600, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}
