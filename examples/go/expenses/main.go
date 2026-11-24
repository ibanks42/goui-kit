// expenses: a small expense tracker. A form with
// validation adds an expense (what, amount, category); the list shows them with a delete button
// each; a bar chart shows the total per category.
//
//	expenses-go                 a real window
//	GK_SCRIPT=file expenses-go  offscreen under a gk-meter script (gates/go-apps.sh)
package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/chart"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/table"
	"gpuikit.dev/gk/theme"
)

type Category int

const (
	Food Category = iota
	Transport
	Home
	Fun
)

var categories = []Category{Food, Transport, Home, Fun}

func (c Category) String() string { return [...]string{"Food", "Transport", "Home", "Fun"}[c] }

type Expense struct {
	What     string
	Amount   float64
	Category Category
}

type App struct {
	expenses     []Expense
	what, amount editor.InputState
	category     table.SelectState
	list         table.ListState
	// the fields' messages, "" while a field is valid
	whatErr, amountErr, categoryErr string
}

func (a *App) init(cx *gk.Cx) {
	a.what = editor.NewInputState(cx).Placeholder("Coffee, bus ticket, ...").Build()
	a.amount = editor.NewInputState(cx).Placeholder("0.00").Build()
	a.what.OnInputEvent(cx, a.fieldEvent)
	a.amount.OnInputEvent(cx, a.fieldEvent)

	items := make([]gk.SearchItem, len(categories))
	for i, c := range categories {
		items[i] = gk.SearchItem{Value: c.String()}
	}
	a.category = table.NewSelectState(cx, gk.NewSearchSource(cx, items), table.NoRow).Build()
	a.category.OnSelectEvent(cx, func(cx *gk.Cx, _ table.SelectEvent) { a.revalidate(cx) })
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.expenses) }, a.expenseRow)).Build()

	a.expenses = append(a.expenses,
		Expense{"Groceries", 42.10, Food},
		Expense{"Monthly bus pass", 55, Transport},
		Expense{"Cinema", 12.50, Fun})
	gk.Refresh(cx)
}

func (a *App) total() (sum float64) {
	for _, e := range a.expenses {
		sum += e.Amount
	}
	return sum
}

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap4).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.ItemsCenter, s.Gap2).
			Child(kit.Label("Expenses", s.TextXl, s.FontBold)).
			Child(gk.Div(s.Flex1)).
			Child(kit.Label(fmt.Sprintf("Total %.2f", a.total()), s.FontSemibold))).
		Child(a.form()).
		Child(gk.HFlex(s.Flex1, s.MinH0, s.Gap4, s.ItemsStart).
			Child(gk.Div(s.Flex1, s.HFull).Child(table.List(a.list))).
			Child(a.chart()))
}

func (a *App) form() gk.Element {
	return kit.Form().Columns(3).
		Child(field("What", editor.Input(a.what), a.whatErr)).
		Child(field("Amount", editor.Input(a.amount), a.amountErr)).
		Child(field("Category", table.Select(a.category).Placeholder("Choose one"), a.categoryErr)).
		Footer(kit.Button("add").Primary().Label("Add expense").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.add(cx) }))
}

// field is a form field with its message under it while it has one.
func field(label string, input gk.Element, message string) kit.FieldEl {
	f := kit.Field().Label(label).Child(input)
	if message != "" {
		f = f.DescriptionFn(kit.Label(message, s.TextXs).TextColor(theme.Danger))
	}
	return f
}

// total is one bar of the chart.
type total struct {
	Category string
	Total    float64
}

func (a *App) chart() gk.Element {
	totals := make([]total, len(categories))
	for i, c := range categories {
		totals[i].Category = c.String()
	}
	for _, e := range a.expenses {
		totals[e.Category].Total += e.Amount
	}
	return gk.VFlex(s.Gap2).W(gk.Px(280)).
		Child(kit.Label("By category", s.FontSemibold)).
		Child(gk.Div().H(gk.Px(220)).Child(chart.BarChart(chart.NewData(totals)).Band("Category").Value("Total")))
}

func (a *App) expenseRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	row, e := at.Row, a.expenses[at.Row]
	return table.ListItem(gk.Key("expense", row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(gk.Div(s.Flex1).Child(kit.Label(e.What))).
		Child(kit.TagSecondary(s.TextXs).Text(e.Category.String())).
		Child(kit.Label(fmt.Sprintf("%.2f", e.Amount)).W(gk.Px(70))).
		Child(kit.Button(gk.Key("delete", row)).Ghost().Xsmall().Icon(gk.IconDelete).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
			gk.StopPropagation(cx)
			a.expenses = slices.Delete(a.expenses, row, row+1)
			gk.Refresh(cx)
		})))
}

// validate checks the fields, sets their messages, and returns the expense they describe.
func (a *App) validate(cx *gk.Cx) (e Expense, ok bool) {
	e.What = strings.TrimSpace(a.what.Value(cx))
	a.whatErr = ""
	if e.What == "" {
		a.whatErr = "Say what it was for"
	}
	text := strings.TrimSpace(a.amount.Value(cx))
	amount, err := strconv.ParseFloat(text, 64)
	switch {
	case text == "":
		a.amountErr = "Enter an amount"
	case err != nil:
		a.amountErr = "Enter a number, like 12.50"
	case amount <= 0:
		a.amountErr = "The amount must be more than zero"
	default:
		a.amountErr = ""
	}
	e.Amount = amount
	a.categoryErr = "Choose a category"
	if at, chosen := a.category.SelectedIndex(cx).Get(); chosen {
		e.Category, a.categoryErr = Category(at.Row), ""
	}
	return e, a.whatErr == "" && a.amountErr == "" && a.categoryErr == ""
}

func (a *App) add(cx *gk.Cx) {
	e, ok := a.validate(cx)
	if !ok {
		return
	}
	a.expenses = append(a.expenses, e)
	a.what.SetValue(cx, "")
	a.amount.SetValue(cx, "")
	a.what.Focus(cx)
	table.SelectRow(cx, a.list, len(a.expenses)-1, table.ScrollIntoView)
	overlay.PushNotification(cx, fmt.Sprintf("Added %s: %.2f", e.What, e.Amount))
	gk.Refresh(cx)
}

// fieldEvent: Enter in either field adds; typing clears a message once the field is fixed.
func (a *App) fieldEvent(cx *gk.Cx, e editor.InputEvent) {
	switch e.(type) {
	case editor.InputEventPressEnter:
		a.add(cx)
	case editor.InputEventChange:
		a.revalidate(cx)
	}
}

// revalidate updates the messages once one is shown, so a fixed field loses its message.
func (a *App) revalidate(cx *gk.Cx) {
	if a.whatErr != "" || a.amountErr != "" || a.categoryErr != "" {
		a.validate(cx)
	}
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Expenses", 860, 560, gk.NewView(a.init, a.view))
	})
}
