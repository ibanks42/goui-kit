// Step 7 of the Go tutorial: register keyboard shortcuts and wire them into a dropdown menu.
package main

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/table"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type Item struct {
	Name   string
	Amount int
	Bought bool
}

type App struct {
	items      []Item
	editing    int
	editAmount int

	newName, editName editor.InputState
	list              table.ListState

	newItem, clearBought gk.Action
}

func (a *App) init(cx *gk.Cx) {
	a.newName = editor.NewInputState(cx).Placeholder("Add an item and press Enter").Build()
	a.newName.OnInputEvent(cx, a.newNameEvent)
	a.editName = editor.NewInputState(cx).Build()
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.list.OnListEvent(cx, a.listEvent)

	// gk.Shortcut creates an action, binds its keystroke, and installs an app-wide listener.
	// The returned gk.Action lets menus display the shortcut and dispatch the same handler.
	a.newItem = gk.Shortcut(cx, "ctrl-n", a.focusNewName)
	a.clearBought = gk.Shortcut(cx, "ctrl-shift-d", a.removeBought)
	a.items = append(a.items, Item{Name: "Milk", Amount: 2}, Item{Name: "Bread", Amount: 1}, Item{Name: "Apples", Amount: 6})
	gk.Refresh(cx)
}

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	row, it := at.Row, a.items[at.Row]
	name := kit.Label(it.Name)
	if it.Bought {
		name = name.Style(s.LineThrough).TextColor(theme.MutedForeground)
	}
	return table.ListItem(gk.Key("item", row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(kit.Checkbox(gk.Key("bought", row)).Checked(it.Bought).OnClick(func(cx *gk.Cx, bought bool) {
			gk.StopPropagation(cx)
			a.items[row].Bought = bought
			gk.Refresh(cx)
		})).
		Child(gk.Div(s.Flex1).Child(name)).
		Child(kit.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))).
		Child(kit.Button(gk.Key("delete", row)).Ghost().Xsmall().Icon(gk.IconDelete).OnClickDo(func(cx *gk.Cx) {
			gk.StopPropagation(cx)
			a.items = slices.Delete(a.items, row, row+1)
			gk.Refresh(cx)
		})))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(a.toolbar()).
		Child(editor.Input(a.newName)).
		Child(gk.Div(s.Flex1, s.MinH0).Child(table.List(a.list))).
		Child(kit.Label(fmt.Sprintf("%d items", len(a.items)), s.TextSm).TextColor(theme.MutedForeground))
}

func (a *App) toolbar() gk.Element {
	menu := kit.NewPopupMenu().
		Menu("New item", a.newItem).
		Menu("Remove bought items", a.clearBought)
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(gk.Div(s.Flex1)).
		Child(kit.DropdownMenu(kit.Button("menu").Ghost().Icon(gk.IconMenu), menu))
}

func (a *App) newNameEvent(cx *gk.Cx, e editor.InputEvent) {
	if _, enter := e.(editor.InputEventPressEnter); !enter {
		return
	}
	name := strings.TrimSpace(a.newName.Value(cx))
	if name == "" {
		return
	}
	a.items = append(a.items, Item{Name: name, Amount: 1})
	a.newName.SetValue(cx, "")
	table.SelectRow(cx, a.list, len(a.items)-1, table.ScrollIntoView)
	gk.Refresh(cx)
}

func (a *App) listEvent(cx *gk.Cx, e table.ListEvent) {
	if c, ok := e.(table.ListEventConfirm); ok {
		a.openEditor(cx, c.Row)
	}
}

func (a *App) editView(cx *gk.Cx) overlay.DialogEl {
	amountBy := func(by int) func(*gk.Cx) {
		return func(*gk.Cx) { a.editAmount = max(1, a.editAmount+by) }
	}
	return overlay.Dialog().Title(kit.Label("Edit item")).
		Child(gk.VFlex(s.Gap3).
			Child(editor.Input(a.editName)).
			Child(gk.HFlex(s.Gap2, s.ItemsCenter).
				Child(kit.Label("Amount")).
				Child(kit.Button("less").Small().Icon(gk.IconMinus).OnClickDo(amountBy(-1))).
				Child(kit.Label(fmt.Sprint(a.editAmount))).
				Child(kit.Button("more").Small().Icon(gk.IconPlus).OnClickDo(amountBy(1))))).
		Footer(overlay.DialogFooter().
			Child(overlay.DialogClose().Trigger(kit.Button("cancel").Label("Cancel"))).
			Child(overlay.DialogAction().Child(kit.Button("save").Primary().Label("Save")))).
		OnOk(a.saveEdit)
}

func (a *App) openEditor(cx *gk.Cx, row int) {
	a.editing = row
	a.editAmount = a.items[row].Amount
	a.editName.SetValue(cx, a.items[row].Name)
	overlay.OpenDialog(cx, a.editView)
	a.editName.Focus(cx)
}

func (a *App) saveEdit(cx *gk.Cx, _ gk.ClickEvent) bool {
	name := strings.TrimSpace(a.editName.Value(cx))
	if name == "" {
		return false
	}
	it := &a.items[a.editing]
	it.Name = name
	it.Amount = a.editAmount
	gk.Refresh(cx)
	return true
}

func (a *App) focusNewName(cx *gk.Cx) { a.newName.Focus(cx) }

func (a *App) removeBought(cx *gk.Cx) {
	a.items = slices.DeleteFunc(a.items, func(it Item) bool { return it.Bought })
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
