// Step 1 of docs/GETTING-STARTED.md: a window with a view.
package groceries

import gk "gpuikit:gk"

App :: struct {}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.label("Groceries", .Text_Xl, .Font_Bold))->
		child(gk.label("Nothing to buy yet"))
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Groceries", {520, 560}, gk.view(a, nil, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
