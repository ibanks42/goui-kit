// Step 5 of docs/GETTING-STARTED.md: handlers in a row (a checkbox, a delete button).
package groceries

import "core:fmt"
import "core:strings"
import gk "gpuikit:gk"

Item :: struct {
	name:   string, // owned by the app (cloned from the input)
	amount: int,
	bought: bool,
}

App :: struct {
	items:    [dynamic]Item,
	new_name: gk.Input_State,
	list:     gk.List_State,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.new_name = gk.input_state(cx)->placeholder("Add an item and press Enter")->build()
	a.new_name->on_input_event(cx, gk.on(new_name_event))
	a.list = gk.list_state(cx, gk.list_source(cx, a, item_count, item_row))->build()
	add_item(a, "Milk", 2)
	add_item(a, "Bread", 1)
	add_item(a, "Apples", 6)
	gk.refresh(cx)
}

add_item :: proc(a: ^App, name: string, amount: int) {
	append(&a.items, Item{name = strings.clone(name), amount = amount})
}

item_count :: proc(a: ^App) -> int {return len(a.items)}

item_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	it := a.items[at.row]
	name := gk.label(it.name)
	if it.bought {
		name = name->style(.Line_Through)->text_color(gk.theme(.Muted_Foreground))
	}
	return gk.list_item(gk.id("item", at.row))->child(gk.h_flex(.W_Full, .Gap_2, .Items_Center)->
		child(gk.checkbox(gk.id("bought", at.row))->checked(it.bought)->on_click(gk.on(toggle_bought, at.row)))->
		child(gk.div(.Flex_1)->child(name))->
		child(gk.tag_secondary(.Text_Xs)->text(fmt.tprintf("×%d", it.amount)))->
		child(gk.button(gk.id("delete", at.row))->ghost()->xsmall()->icon(.Delete)->on_click(gk.on(delete_item, at.row))))
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.label("Groceries", .Text_Xl, .Font_Bold))->
		child(gk.input(a.new_name))->
		child(gk.div(.Flex_1, .Min_H_0)->child(gk.list(a.list)))->
		child(gk.label(fmt.tprintf("%d items", len(a.items)), .Text_Sm)->text_color(gk.theme(.Muted_Foreground)))
}

new_name_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	if _, enter := e.(gk.Input_Event_Press_Enter); !enter {
		return
	}
	name := strings.trim_space(string(a.new_name->value(cx))) // lent text: add_item clones it
	if name == "" {
		return
	}
	add_item(a, name, 1)
	a.new_name->set_value(cx, "")
	gk.select_row(cx, a.list, len(a.items) - 1, scroll = true)
	gk.refresh(cx)
}

toggle_bought :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	gk.stop_propagation(cx) // the row under the checkbox is not clicked as well
	a.items[row].bought = !a.items[row].bought
	gk.refresh(cx)
}

delete_item :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	gk.stop_propagation(cx)
	delete(a.items[row].name)
	ordered_remove(&a.items, row)
	gk.refresh(cx)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Groceries", {520, 560}, gk.view(a, init, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
