// Runtime misuse cases for row sources, data tables, lists, and InputState (GK_NEG):
// released sources, re-entrant release during row rendering, oversized counts, and input release.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	gktable "github.com/ibanks42/goui-kit/gk/table"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type App struct {
	mode      string
	root      gk.View
	src       gk.RowSource
	table     gktable.State
	list      gktable.ListState
	name      editor.InputState
	clicks    int
	maxRow    int
	described int
	got       string
}

func columns() []gktable.TableColumn {
	a := gktable.NewTableColumn("a", "A")
	a.MaxWidth = 400
	b := gktable.NewTableColumn("b", "B")
	b.MaxWidth = 400
	return []gktable.TableColumn{a, b}
}

func (a *App) cell(cx *gk.Cx, at gktable.CellAt) gk.Element {
	a.described++
	a.maxRow = max(a.maxRow, at.Row)
	if a.mode == "row-reenter" && a.described == 3 {
		gk.Release(cx, a.src)
		fmt.Println("gk-neg row-reenter went on after releasing its own source")
	}
	return kit.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
}

func (a *App) listRow(cx *gk.Cx, at gk.RowAt) gktable.ListItemEl {
	text := fmt.Sprintf("row %d of a list", at.Row)
	return gktable.ListItem(gk.Key("r", at.Row)).Child(kit.Label(text)).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
		gk.SetCount(cx, a.src, 0)
		a.got = text
		fmt.Printf("gk-neg row-handler clicked text=%q\n", a.got)
	})
}

func (a *App) act(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	switch a.mode {
	case "table-freed":
		gk.Release(cx, a.src)
		fmt.Println("gk-neg table-freed released the source")
	case "huge-count":
		a.table.ScrollToRow(cx, 2_147_483_700)
	}
}

func (a *App) changed(cx *gk.Cx, e editor.InputEvent) {
	if _, ok := e.(editor.InputEventChange); ok {
		v := a.name.Value(cx)
		gk.Release(cx, a.name)
		fmt.Printf("gk-neg input-release released in its handler value=%q\n", v)
	}
}

func (a *App) init(cx *gk.Cx) {
	switch a.mode {
	case "table-freed", "row-reenter":
		a.src = gktable.NewSource(cx, 1000, columns(), a.cell)
		a.table = gktable.NewState(cx, a.src).Build()
	case "huge-count":
		a.src = gktable.NewSource(cx, 3_000_000_000, columns(), a.cell)
		a.table = gktable.NewState(cx, a.src).Build()
	case "row-handler":
		a.src = gktable.NewListSource(cx, func() int { return 20 }, a.listRow)
		a.list = gktable.NewListState(cx, a.src).Build()
	default:
		a.name = editor.NewInputState(cx).Build()
		if a.mode == "input-release" {
			a.name.OnInputEvent(cx, a.changed)
		}
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	var body gk.Element
	switch a.mode {
	case "table-freed", "row-reenter", "huge-count":
		body = gk.Div().W(gk.Px(300)).H(gk.Px(200)).Child(gktable.DataTable(a.table))
	case "row-handler":
		body = gk.Div().W(gk.Px(300)).H(gk.Px(200)).Child(gktable.List(a.list, s.SizeFull))
	default:
		body = gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))
	}
	return gk.VFlex(s.SizeFull, s.P4, s.Gap2).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2).Child(kit.Button("act").Label("act").OnClick(a.act))).
		Child(body)
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	script := "size 400 300\nframe\nclick 36 30\nframe\nframe\n"
	switch a.mode {
	case "row-handler":
		script = "size 400 300\nframe\nclick 60 74\nframe\nframe\n"
	case "input-release":
		script = "size 400 300\nframe\nclick 60 72\ntype x\nframe\nframe\n"
	}
	if err := gk.RunHeadless(script, func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "misuse_rows", 400, 300, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gk-neg done mode=%s clicks=%d described=%d max_row=%d got=%q\n", a.mode, a.clicks, a.described, a.maxRow, a.got)
}
