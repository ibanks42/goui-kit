// Step 3 of the Go tutorial: render items through a virtualized List backed by a RowSource.
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
	list  table.ListState
}

// init runs once when OpenWindow mounts the root view, which is when window-backed
// GPUI-kit states (such as ListState) can be constructed.
func (a *App) init(cx *gk.Cx) {
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.items = append(a.items, Item{"Milk", 2}, Item{"Bread", 1}, Item{"Apples", 6})
	// Refresh notifies the view and re-syncs row source counts after changing a.items.
	gk.Refresh(cx)
}

// itemRow describes one visible row on demand as its own retained view.
// gk.Key("item", at.Row) gives each row a stable, unique element ID.
func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	it := a.items[at.Row]
	return table.ListItem(gk.Key("item", at.Row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(gk.Div(s.Flex1).Child(kit.Label(it.Name))).
		Child(kit.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	// s.Flex1 and s.MinH0 constrain the list's height inside the flex column so it scrolls
	// instead of expanding off-screen.
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Button("add-milk").Primary().Label("Add milk").OnClickDo(a.addMilk)).
		Child(gk.Div(s.Flex1, s.MinH0).Child(table.List(a.list))).
		Child(kit.Label(fmt.Sprintf("%d items", len(a.items)), s.TextSm).TextColor(theme.MutedForeground))
}

func (a *App) addMilk(cx *gk.Cx) {
	a.items = append(a.items, Item{"Milk", 1})
	// Refresh is required here so the row source re-reads len(a.items).
	gk.Refresh(cx)
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(a.init, a.view))
	}); err != nil {
		log.Fatal(err)
	}
}
