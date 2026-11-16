// Step 4 of the Go tutorial: an input; Enter adds what was typed.
package main

import (
	"fmt"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type Item struct {
	Name   string
	Amount int
}

type App struct {
	items   []Item
	newName gk.InputState
	list    gk.ListState
}

func (a *App) init(cx *gk.Cx) {
	a.newName = gk.NewInputState(cx).Placeholder("Add an item and press Enter").Build()
	a.newName.OnInputEvent(cx, a.newNameEvent)
	a.list = gk.NewListState(cx, gk.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.items = append(a.items, Item{"Milk", 2}, Item{"Bread", 1}, Item{"Apples", 6})
	gk.Refresh(cx)
}

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	it := a.items[at.Row]
	return gk.ListItem(gk.Key("item", at.Row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(gk.Div(s.Flex1).Child(gk.Label(it.Name))).
		Child(gk.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.Label("Groceries", s.TextXl, s.FontBold)).
		Child(gk.Input(a.newName)).
		Child(gk.Div(s.Flex1, s.MinH0).Child(gk.List(a.list))).
		Child(gk.Label(fmt.Sprintf("%d items", len(a.items)), s.TextSm).TextColor(theme.MutedForeground))
}

func (a *App) newNameEvent(cx *gk.Cx, e gk.InputEvent) {
	if _, enter := e.(gk.InputEventPressEnter); !enter {
		return
	}
	name := strings.TrimSpace(a.newName.Value(cx))
	if name == "" {
		return
	}
	a.items = append(a.items, Item{name, 1})
	a.newName.SetValue(cx, "")
	gk.SelectRow(cx, a.list, len(a.items)-1, gk.ScrollIntoView)
	gk.Refresh(cx)
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(a.init, a.view))
	})
}
