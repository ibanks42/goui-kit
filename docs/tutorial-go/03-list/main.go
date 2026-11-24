// Step 3 of the Go tutorial: a list whose rows the app draws.
package main

import (
	"fmt"
	"log"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/table"
	"gpuikit.dev/gk/theme"
)

type Item struct {
	Name   string
	Amount int
}

type App struct {
	items []Item
	list  table.ListState // kit state: it lives in the library; the app keeps the handle
}

func (a *App) init(cx *gk.Cx) {
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.items = append(a.items, Item{"Milk", 2}, Item{"Bread", 1}, Item{"Apples", 6})
	gk.Refresh(cx)
}

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	it := a.items[at.Row]
	return table.ListItem(gk.Key("item", at.Row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(gk.Div(s.Flex1).Child(kit.Label(it.Name))).
		Child(kit.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Button("add-milk").Primary().Label("Add milk").OnClickDo(a.addMilk)).
		Child(gk.Div(s.Flex1, s.MinH0).Child(table.List(a.list))).
		Child(kit.Label(fmt.Sprintf("%d items", len(a.items)), s.TextSm).TextColor(theme.MutedForeground))
}

func (a *App) addMilk(cx *gk.Cx) {
	a.items = append(a.items, Item{"Milk", 1})
	gk.Refresh(cx) // the rows show data this handler changed: describe them again
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(a.init, a.view))
	}); err != nil {
		log.Fatal(err)
	}
}
