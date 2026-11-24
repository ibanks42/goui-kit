// Step 2 of the Go tutorial: state in the app's struct, and a handler that changes it.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type Item struct {
	Name   string
	Amount int
}

type App struct {
	items []Item
}

func (a *App) view(cx *gk.Cx) gk.Element {
	col := gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Button("add-milk").Primary().Label("Add milk").OnClick(a.addMilk)).
		Child(kit.Label(fmt.Sprintf("%d items", len(a.items))))
	for _, it := range a.items {
		col = col.Child(kit.Label(fmt.Sprintf("%s ×%d", it.Name, it.Amount)))
	}
	return col
}

func (a *App) addMilk(cx *gk.Cx, _ gk.ClickEvent) {
	a.items = append(a.items, Item{Name: "Milk", Amount: 1})
	// nothing else: a handler redraws the view it was given in
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(nil, a.view))
	})
}
