// Step 2 of the Go tutorial: store application data in a Go struct and mutate it on click.
package main

import (
	"fmt"
	"log"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type Item struct {
	Name   string
	Amount int
}

// App holds ordinary Go state; GPUI-kit redraws from it whenever the view is marked dirty.
type App struct {
	items []Item
}

func (a *App) view(cx *gk.Cx) gk.Element {
	// Interactive elements take an ID ("add-milk") so GPUI tracks hover, active,
	// and focus state across redraws. OnClickDo omits the ClickEvent argument.
	col := gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Button("add-milk").Primary().Label("Add milk").OnClickDo(a.addMilk)).
		Child(kit.Label(fmt.Sprintf("%d items", len(a.items))))
	for _, it := range a.items {
		col = col.Child(kit.Label(fmt.Sprintf("%s ×%d", it.Name, it.Amount)))
	}
	return col
}

// addMilk mutates Go state. Because OnClickDo runs inside this view's turn,
// the runtime marks the view dirty automatically when addMilk returns.
func (a *App) addMilk(cx *gk.Cx) {
	a.items = append(a.items, Item{Name: "Milk", Amount: 1})
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(nil, a.view))
	}); err != nil {
		log.Fatal(err)
	}
}
