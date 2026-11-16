// Step 4 of docs/GETTING-STARTED.md: an input; Enter adds what was typed.
package groceries

import "core:fmt"
import "core:strings"
import gk "gpuikit:gk"

Item :: struct {
	name:   string, // owned by the app (cloned from the input)
	amount: int,
}

App :: struct {
	items:    [dynamic]Item,
	new_name: gk.Input_State,
	list:     gk.List_State,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.new_name = gk.input_state(cx)->placeholder("Add an item and press Enter")->build()
	gk.on_input_event(a.new_name, cx, gk.on(new_name_event))
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
	return gk.list_item(gk.id("item", at.row))->child(gk.h_flex(.W_Full, .Gap_2, .Items_Center)->
		child(gk.div(.Flex_1)->child(gk.label(it.name)))->
		child(gk.tag_secondary(.Text_Xs)->text(fmt.tprintf("×%d", it.amount))))
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
	name := strings.trim_space(string(gk.value(a.new_name, cx))) // lent text: add_item clones it
	if name == "" {
		return
	}
	add_item(a, name, 1)
	gk.set_value(a.new_name, cx, "")
	gk.select_row(cx, a.list, len(a.items) - 1, scroll = true)
	gk.refresh(cx)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Groceries", {520, 560}, gk.view(a, init, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
