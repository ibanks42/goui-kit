// Step 7 of the Go tutorial: keyboard shortcuts, and a menu that shows them.
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
	items      []Item
	editing    int // the item the dialog edits
	editAmount int

	newName, editName gk.InputState // kit state lives in the library; the app keeps handles
	list              gk.ListState
	editDialog        gk.Built[gk.DialogEl]

	newItem, clearBought gk.Action // shortcuts, also shown in the menu
}

func (a *App) init(cx *gk.Cx) {
	a.newName = gk.NewInputState(cx).Placeholder("Add an item and press Enter").Build()
	a.newName.OnInputEvent(cx, a.newNameEvent)
	a.editName = gk.NewInputState(cx).Build()
	a.list = gk.NewListState(cx, gk.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.list.OnListEvent(cx, a.listEvent)
	a.editDialog = gk.NewBuilt(cx, a.editView)

	a.newItem = gk.Shortcut(cx, "ctrl-n", a.focusNewName)
	a.clearBought = gk.Shortcut(cx, "ctrl-shift-d", a.removeBought)
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
		Child(a.toolbar()).
		Child(gk.Input(a.newName)).
		Child(gk.Div(s.Flex1, s.MinH0).Child(gk.List(a.list))).
		Child(gk.Label(fmt.Sprintf("%d items", len(a.items)), s.TextSm).TextColor(theme.MutedForeground))
}

func (a *App) toolbar() gk.Element {
	menu := gk.NewPopupMenu().
		Menu("New item", a.newItem).
		Menu("Remove bought items", a.clearBought)
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(gk.Label("Groceries", s.TextXl, s.FontBold)).
		Child(gk.Div(s.Flex1)).
		Child(gk.DropdownMenu(gk.Button("menu").Ghost().Icon(gk.IconMenu), menu))
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

func (a *App) listEvent(cx *gk.Cx, e gk.ListEvent) {
	if c, ok := e.(gk.ListEventConfirm); ok { // a click on a row, or Enter
		a.openEditor(cx, c.Row)
	}
}

func (a *App) editView(cx *gk.Cx) gk.DialogEl {
	amountBy := func(by int) func(*gk.Cx, gk.ClickEvent) {
		return func(*gk.Cx, gk.ClickEvent) { a.editAmount = max(1, a.editAmount+by) }
	}
	return gk.Dialog().Title(gk.Label("Edit item")).
		Child(gk.VFlex(s.Gap3).
			Child(gk.Input(a.editName)).
			Child(gk.HFlex(s.Gap2, s.ItemsCenter).
				Child(gk.Label("Amount")).
				Child(gk.Button("less").Small().Icon(gk.IconMinus).OnClick(amountBy(-1))).
				Child(gk.Label(fmt.Sprint(a.editAmount))).
				Child(gk.Button("more").Small().Icon(gk.IconPlus).OnClick(amountBy(1))))).
		Footer(gk.DialogFooter().
			Child(gk.DialogClose().Trigger(gk.Button("cancel").Label("Cancel"))).
			Child(gk.DialogAction().Child(gk.Button("save").Primary().Label("Save")))).
		OnOk(a.saveEdit)
}

func (a *App) openEditor(cx *gk.Cx, row int) {
	a.editing = row
	a.editAmount = a.items[row].Amount
	a.editName.SetValue(cx, a.items[row].Name)
	gk.OpenDialog(cx, a.editDialog)
	a.editName.Focus(cx)
}

func (a *App) saveEdit(cx *gk.Cx, _ gk.ClickEvent) bool {
	name := strings.TrimSpace(a.editName.Value(cx))
	if name == "" {
		return false // keep the dialog open
	}
	it := &a.items[a.editing]
	it.Name = name
	it.Amount = a.editAmount
	gk.Refresh(cx)
	return true // close the dialog
}

func (a *App) focusNewName(cx *gk.Cx) { a.newName.Focus(cx) }

func (a *App) removeBought(cx *gk.Cx) {
	a.items = slices.DeleteFunc(a.items, func(it Item) bool { return it.Bought })
	gk.Refresh(cx)
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(a.init, a.view))
	})
}
