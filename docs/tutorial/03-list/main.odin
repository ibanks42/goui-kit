// Step 3 of docs/GETTING-STARTED.md: a list whose rows the app draws.
package groceries

import "core:fmt"
import gk "gpuikit:gk"

Item :: struct {
	name:   string,
	amount: int,
}

App :: struct {
	items: [dynamic]Item,
	list:  gk.List_State, // kit state: it lives in the library; the app keeps the handle
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.list = gk.list_state(cx, gk.list_source(cx, a, item_count, item_row))->build()
	append(&a.items, Item{name = "Milk", amount = 2}, Item{name = "Bread", amount = 1}, Item{name = "Apples", amount = 6})
	gk.refresh(cx)
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
		child(gk.button("add-milk")->primary()->label("Add milk")->on_click(gk.on(add_milk)))->
		child(gk.div(.Flex_1, .Min_H_0)->child(gk.list(a.list)))->
		child(gk.label(fmt.tprintf("%d items", len(a.items)), .Text_Sm)->text_color(gk.theme(.Muted_Foreground)))
}

add_milk :: proc(a: ^App, cx: ^gk.Cx) {
	append(&a.items, Item{name = "Milk", amount = 1})
	gk.refresh(cx) // the rows show data this handler changed: describe them again
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Groceries", {520, 560}, gk.view(a, init, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
