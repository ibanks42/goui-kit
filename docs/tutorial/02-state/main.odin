// Step 2 of docs/GETTING-STARTED.md: state in the app's struct, and a handler that changes it.
package groceries

import "core:fmt"
import gk "gpuikit:gk"

Item :: struct {
	name:   string,
	amount: int,
}

App :: struct {
	items: [dynamic]Item,
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	col := gk.v_flex(.Size_Full, .P_4, .Gap_3)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.label("Groceries", .Text_Xl, .Font_Bold))->
		child(gk.button("add-milk")->primary()->label("Add milk")->on_click(gk.on(add_milk)))->
		child(gk.label(fmt.tprintf("%d items", len(a.items))))
	for it in a.items {
		col = col->child(gk.label(fmt.tprintf("%s ×%d", it.name, it.amount)))
	}
	return col
}

add_milk :: proc(a: ^App, cx: ^gk.Cx) {
	append(&a.items, Item{name = "Milk", amount = 1})
	// nothing else: a handler redraws the view it was given in
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Groceries", {520, 560}, gk.view(a, nil, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
