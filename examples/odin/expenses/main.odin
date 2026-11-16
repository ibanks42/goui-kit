// Expenses: a small expense tracker, written from the download and the guides alone. A form with validation adds an expense (what, amount,
// category); the list shows them with a delete button each; a bar chart shows the total per
// category. Build: examples/build.sh expenses.
package expenses

import "core:fmt"
import "core:strconv"
import "core:strings"
import gk "gpuikit:gk"

Category :: enum {
	Food,
	Transport,
	Home,
	Fun,
}

Expense :: struct {
	what:     string,
	amount:   f64,
	category: Category,
}

// one bar of the chart
Total :: struct {
	category: string,
	total:    f64,
}

App :: struct {
	expenses:       [dynamic]Expense,
	what:           gk.Input_State,
	amount:         gk.Input_State,
	category:       gk.Select_State,
	list:           gk.List_State,
	what_error:     string, // "" while the field is valid
	amount_error:   string,
	category_error: string,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.what = gk.input_state(cx)->placeholder("Coffee, bus ticket, ...")->build()
	a.amount = gk.input_state(cx)->placeholder("0.00")->build()
	a.what->on_input_event(cx, gk.on(field_event))
	a.amount->on_input_event(cx, gk.on(field_event))

	items: [len(Category)]gk.Search_Item
	for c in Category {
		items[c] = gk.search_item(fmt.tprint(c))
	}
	a.category = gk.select_state(cx, gk.search_source(cx, a, items[:]), nil)->build()
	a.category->on_select_event(cx, gk.on(revalidate))
	a.list = gk.list_state(cx, gk.list_source(cx, a, expense_count, expense_row))->build()

	add_expense(a, "Groceries", 42.10, .Food)
	add_expense(a, "Monthly bus pass", 55, .Transport)
	add_expense(a, "Cinema", 12.50, .Fun)
	gk.refresh(cx)
}

add_expense :: proc(a: ^App, what: string, amount: f64, category: Category) {
	append(&a.expenses, Expense{what = strings.clone(what), amount = amount, category = category})
}

total :: proc(a: ^App) -> (sum: f64) {
	for e in a.expenses {
		sum += e.amount
	}
	return
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full, .P_4, .Gap_4)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.h_flex(.Items_Center, .Gap_2)->
			child(gk.label("Expenses", .Text_Xl, .Font_Bold))->
			child(gk.div(.Flex_1))->
			child(gk.label(fmt.tprintf("Total %.2f", total(a)), .Font_Semibold)))->
		child(entry_form(a))->
		child(gk.h_flex(.Flex_1, .Min_H_0, .Gap_4, .Items_Start)->
			child(gk.div(.Flex_1, .H_Full)->child(gk.list(a.list)))->
			child(chart(a)))
}

entry_form :: proc(a: ^App) -> gk.Element {
	what := gk.field()->label("What")->child(gk.input(a.what))
	if a.what_error != "" {
		what = what->description_fn(error_text(a.what_error))
	}
	amount := gk.field()->label("Amount")->child(gk.input(a.amount))
	if a.amount_error != "" {
		amount = amount->description_fn(error_text(a.amount_error))
	}
	category := gk.field()->label("Category")->child(gk.select(a.category)->placeholder("Choose one"))
	if a.category_error != "" {
		category = category->description_fn(error_text(a.category_error))
	}
	return gk.form()->columns(3)->child(what)->child(amount)->child(category)->
		footer(gk.button("add")->primary()->label("Add expense")->on_click(gk.on(add_clicked)))
}

error_text :: proc(text: string) -> gk.Element {
	return gk.label(text, .Text_Xs)->text_color(gk.theme(.Danger))
}

chart :: proc(a: ^App) -> gk.Element {
	totals: [len(Category)]Total
	for c in Category {
		totals[c].category = fmt.tprint(c)
	}
	for e in a.expenses {
		totals[e.category].total += e.amount
	}
	return gk.v_flex(.Gap_2)->w(gk.px(280))->
		child(gk.label("By category", .Font_Semibold))->
		child(gk.div()->h(gk.px(220))->child(gk.bar_chart(gk.data(totals[:]))->band("category")->value("total")))
}

expense_count :: proc(a: ^App) -> int {return len(a.expenses)}

expense_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	e := a.expenses[at.row]
	return gk.list_item(gk.id("expense", at.row))->child(gk.h_flex(.W_Full, .Gap_2, .Items_Center)->
		child(gk.div(.Flex_1)->child(gk.label(e.what)))->
		child(gk.tag_secondary(.Text_Xs)->text(fmt.tprint(e.category)))->
		child(gk.label(fmt.tprintf("%.2f", e.amount))->w(gk.px(70)))->
		child(gk.button(gk.id("delete", at.row))->ghost()->xsmall()->icon(.Delete)->on_click(gk.on(delete_expense, at.row))))
}

// validate checks the fields and sets their messages; it returns the values.
validate :: proc(a: ^App, cx: ^gk.Cx) -> (what: string, amount: f64, category: Category, ok: bool) {
	what = strings.trim_space(string(a.what->value(cx)))
	amount_text := strings.trim_space(string(a.amount->value(cx)))
	a.what_error = "" if what != "" else "Say what it was for"
	parsed: bool
	amount, parsed = strconv.parse_f64(amount_text)
	switch {
	case amount_text == "":
		a.amount_error = "Enter an amount"
	case !parsed:
		a.amount_error = "Enter a number, like 12.50"
	case amount <= 0:
		a.amount_error = "The amount must be more than zero"
	case:
		a.amount_error = ""
	}
	a.category_error = "Choose a category"
	if at, chosen := a.category->selected_index(cx).?; chosen {
		category = Category(at.row)
		a.category_error = ""
	}
	return what, amount, category, a.what_error == "" && a.amount_error == "" && a.category_error == ""
}

add_clicked :: proc(a: ^App, cx: ^gk.Cx) {
	what, amount, category, ok := validate(a, cx)
	if !ok {
		return
	}
	add_expense(a, what, amount, category)
	a.what->set_value(cx, "")
	a.amount->set_value(cx, "")
	a.what->focus(cx)
	gk.select_row(cx, a.list, len(a.expenses) - 1, scroll = true)
	gk.push_notification(cx, fmt.tprintf("Added %s: %.2f", what, amount))
	gk.refresh(cx)
}

// field_event: Enter in either field adds; typing clears a message once the field is fixed.
field_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	#partial switch _ in e {
	case gk.Input_Event_Press_Enter:
		add_clicked(a, cx)
	case gk.Input_Event_Change:
		revalidate(a, cx)
	}
}

// revalidate updates the messages once one is shown, so a fixed field loses its message.
revalidate :: proc(a: ^App, cx: ^gk.Cx) {
	if a.what_error != "" || a.amount_error != "" || a.category_error != "" {
		validate(a, cx)
	}
}

delete_expense :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	gk.stop_propagation(cx)
	delete(a.expenses[row].what)
	ordered_remove(&a.expenses, row)
	gk.refresh(cx)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Expenses", {860, 560}, gk.view(a, init, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
