// Step 7 of docs/GETTING-STARTED.md: keyboard shortcuts, and a menu that shows them.
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
	items:       [dynamic]Item,
	editing:     int, // the item the dialog edits
	edit_amount: int,
	new_name:    gk.Input_State,
	edit_name:   gk.Input_State,
	list:        gk.List_State,
	// shortcuts, also shown in the menu
	new_item:     gk.Action,
	clear_bought: gk.Action,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.new_name = gk.input_state(cx)->placeholder("Add an item and press Enter")->build()
	gk.on_input_event(a.new_name, cx, gk.on(new_name_event))
	a.edit_name = gk.input_state(cx)->build()
	a.list = gk.list_state(cx, gk.list_source(cx, a, item_count, item_row))->build()
	gk.on_list_event(a.list, cx, gk.on(list_event))

	a.new_item = gk.shortcut(cx, "ctrl-n", gk.on(focus_new_name))
	a.clear_bought = gk.shortcut(cx, "ctrl-shift-d", gk.on(clear_bought))
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
		child(toolbar(a))->
		child(gk.input(a.new_name))->
		child(gk.div(.Flex_1, .Min_H_0)->child(gk.list(a.list)))->
		child(gk.label(fmt.tprintf("%d items", len(a.items)), .Text_Sm)->text_color(gk.theme(.Muted_Foreground)))
}

toolbar :: proc(a: ^App) -> gk.Element {
	menu := gk.popup_menu()->
		menu("New item", a.new_item)->
		menu("Remove bought items", a.clear_bought)
	return gk.h_flex(.Gap_2, .Items_Center)->
		child(gk.label("Groceries", .Text_Xl, .Font_Bold))->
		child(gk.div(.Flex_1))->
		child(gk.dropdown_menu(gk.button("menu")->ghost()->icon(.Menu), menu))
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

list_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Event) {
	if c, ok := e.(gk.List_Event_Confirm); ok { // a click on a row, or Enter
		open_editor(a, cx, c.row)
	}
}

edit_dialog :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	return gk.dialog()->title(gk.label("Edit item"))->
		child(gk.v_flex(.Gap_3)->
			child(gk.input(a.edit_name))->
			child(gk.h_flex(.Gap_2, .Items_Center)->
				child(gk.label("Amount"))->
				child(gk.button("less")->small()->icon(.Minus)->on_click(gk.on(change_amount, -1)))->
				child(gk.label(fmt.tprintf("%d", a.edit_amount)))->
				child(gk.button("more")->small()->icon(.Plus)->on_click(gk.on(change_amount, 1)))))->
		footer(gk.dialog_footer()->
			child(gk.dialog_close()->trigger(gk.button("cancel")->label("Cancel")))->
			child(gk.dialog_action()->child(gk.button("save")->primary()->label("Save"))))->
		on_ok(gk.on(save_edit))
}

open_editor :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	a.editing = row
	a.edit_amount = a.items[row].amount
	gk.set_value(a.edit_name, cx, a.items[row].name)
	gk.open_dialog(cx, gk.built(cx, a, edit_dialog))
	gk.input_state_focus(a.edit_name, cx)
}

change_amount :: proc(a: ^App, cx: ^gk.Cx, by: int) {
	a.edit_amount = max(1, a.edit_amount + by)
}

save_edit :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	name := strings.trim_space(string(gk.value(a.edit_name, cx)))
	if name == "" {
		return false // keep the dialog open
	}
	it := &a.items[a.editing]
	delete(it.name)
	it.name = strings.clone(name)
	it.amount = a.edit_amount
	gk.refresh(cx)
	return true // close the dialog
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

focus_new_name :: proc(a: ^App, cx: ^gk.Cx) {
	gk.input_state_focus(a.new_name, cx)
}

clear_bought :: proc(a: ^App, cx: ^gk.Cx) {
	#reverse for it, i in a.items {
		if it.bought {
			delete(it.name)
			ordered_remove(&a.items, i)
		}
	}
	gk.refresh(cx)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Groceries", {520, 560}, gk.view(a, init, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
