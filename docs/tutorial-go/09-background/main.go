// Step 9 of the Go tutorial: run background work with gk.Go and surface results via notifications.
package main

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

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
	Price  float64
}

type App struct {
	items      []Item
	editing    int
	editAmount int
	dark       bool

	newName, editName editor.InputState
	list              table.ListState

	newItem, clearBought, toggleTheme gk.Action
}

func (a *App) init(cx *gk.Cx) {
	a.newName = editor.NewInputState(cx).Placeholder("Add an item and press Enter").Build()
	a.newName.OnInputEvent(cx, a.newNameEvent)
	a.editName = editor.NewInputState(cx).Build()
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.list.OnListEvent(cx, a.listEvent)

	a.newItem = gk.Shortcut(cx, "ctrl-n", a.focusNewName)
	a.clearBought = gk.Shortcut(cx, "ctrl-shift-d", a.removeBought)
	a.toggleTheme = gk.Shortcut(cx, "ctrl-t", a.switchTheme)
	a.add(cx, "Milk", 2, false)
	a.add(cx, "Bread", 1, false)
	a.add(cx, "Apples", 6, false)
	gk.Refresh(cx)
}

func (a *App) add(cx *gk.Cx, name string, amount int, announce bool) {
	a.items = append(a.items, Item{Name: name, Amount: amount})
	a.lookUpPrice(cx, name, announce)
}

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	row, it := at.Row, a.items[at.Row]
	name := kit.Label(it.Name)
	if it.Bought {
		name = name.Style(s.LineThrough).TextColor(theme.MutedForeground)
	}
	price := "…"
	if it.Price != 0 {
		price = fmt.Sprintf("%.2f", it.Price*float64(it.Amount))
	}
	return table.ListItem(gk.Key("item", row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(kit.Checkbox(gk.Key("bought", row)).Checked(it.Bought).OnClick(func(cx *gk.Cx, bought bool) {
			gk.StopPropagation(cx)
			a.items[row].Bought = bought
			gk.Refresh(cx)
		})).
		Child(gk.Div(s.Flex1).Child(name)).
		Child(kit.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))).
		Child(kit.Label(price, s.TextSm).TextColor(theme.MutedForeground)).
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
		Child(kit.Label(a.summary(), s.TextSm).TextColor(theme.MutedForeground))
}

func (a *App) summary() string {
	toBuy, waiting, total := 0, 0, 0.0
	for _, it := range a.items {
		if it.Price == 0 {
			waiting++
		}
		if !it.Bought {
			toBuy++
			total += it.Price * float64(it.Amount)
		}
	}
	text := fmt.Sprintf("%d items, %d to buy, %.2f in all", len(a.items), toBuy, total)
	if waiting > 0 {
		text += fmt.Sprintf(" (%d prices still loading)", waiting)
	}
	return text
}

func (a *App) toolbar() gk.Element {
	themeLabel, icon := "Dark theme", gk.IconMoon
	if a.dark {
		themeLabel, icon = "Light theme", gk.IconSun
	}
	menu := kit.NewPopupMenu().
		Menu("New item", a.newItem).
		Menu("Remove bought items", a.clearBought).
		Separator().
		Menu(themeLabel, a.toggleTheme)
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(gk.Div(s.Flex1)).
		Child(kit.DropdownMenu(kit.Button("menu").Ghost().Icon(gk.IconMenu), menu)).
		Child(kit.Button("theme").Ghost().Icon(icon).Tooltip("Toggle the theme").OnClickDo(a.switchTheme))
}

func (a *App) newNameEvent(cx *gk.Cx, e editor.InputEvent) {
	if _, enter := e.(editor.InputEventPressEnter); !enter {
		return
	}
	name := strings.TrimSpace(a.newName.Value(cx))
	if name == "" {
		return
	}
	a.add(cx, name, 1, true)
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
	if name != it.Name {
		it.Name, it.Price = name, 0
		a.lookUpPrice(cx, name, true)
	}
	it.Amount = a.editAmount
	gk.Refresh(cx)
	return true
}

func (a *App) focusNewName(cx *gk.Cx) { a.newName.Focus(cx) }

func (a *App) removeBought(cx *gk.Cx) {
	a.items = slices.DeleteFunc(a.items, func(it Item) bool { return it.Bought })
	gk.Refresh(cx)
}

func (a *App) switchTheme(cx *gk.Cx) {
	a.dark = !a.dark
	mode := gk.ThemeModeLight
	if a.dark {
		mode = gk.ThemeModeDark
	}
	gk.ThemeChange(cx, mode)
	gk.Refresh(cx)
}

// lookUpPrice runs slow work off the UI thread via gk.Go and applies the result back
// on the UI thread with a fresh *gk.Cx, optionally showing a toast notification.
func (a *App) lookUpPrice(cx *gk.Cx, name string, announce bool) {
	gk.Go(cx, func() float64 {
		time.Sleep(300 * time.Millisecond)
		return 0.49 + float64(len(name))*0.35
	}, func(cx *gk.Cx, price float64) {
		for i := range a.items {
			if it := &a.items[i]; it.Name == name && it.Price == 0 {
				it.Price = price
			}
		}
		if announce {
			overlay.PushNotification(cx, fmt.Sprintf("%s costs %.2f", name, price))
		}
		gk.Refresh(cx)
	})
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(a.init, a.view))
	}); err != nil {
		log.Fatal(err)
	}
}
