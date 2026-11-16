// Step 5 of the Go tutorial: handlers in a row (a checkbox, a delete button).
package main

import (
	"fmt"
	"slices"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type Item struct {
	Name   string
	Amount int
	Bought bool
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
	a.items = append(a.items, Item{Name: "Milk", Amount: 2}, Item{Name: "Bread", Amount: 1}, Item{Name: "Apples", Amount: 6})
	gk.Refresh(cx)
}

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	row, it := at.Row, a.items[at.Row]
	name := gk.Label(it.Name)
	if it.Bought {
		name = name.Style(s.LineThrough).TextColor(theme.MutedForeground)
	}
	return gk.ListItem(gk.Key("item", row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(gk.Checkbox(gk.Key("bought", row)).Checked(it.Bought).OnClick(func(cx *gk.Cx, bought bool) {
			gk.StopPropagation(cx) // the row under the checkbox is not clicked as well
			a.items[row].Bought = bought
			gk.Refresh(cx)
		})).
		Child(gk.Div(s.Flex1).Child(name)).
		Child(gk.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))).
		Child(gk.Button(gk.Key("delete", row)).Ghost().Xsmall().Icon(gk.IconDelete).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
			gk.StopPropagation(cx)
			a.items = slices.Delete(a.items, row, row+1)
			gk.Refresh(cx)
		})))
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
	a.items = append(a.items, Item{Name: name, Amount: 1})
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
