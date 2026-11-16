package hello

import "core:fmt"
import gk "gpuikit:gk"

App :: struct {
	count: int,
	name:  gk.Input_State, // a handle to the kit's InputState, which lives in the library
	root:  gk.View,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.name = gk.input_state(cx)->placeholder("Your name")->build()
}

add :: proc(a: ^App, cx: ^gk.Cx) {
	a.count += 1 // no notify: a handler marks the view that registered it for redraw
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->
		bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.h_flex(.Gap_2, .Items_Center)->
			child(gk.button("add")->primary()->label("Add")->on_click(gk.on(add)))->
			child(gk.label(fmt.tprintf("clicked %d times", a.count))))->
		child(gk.div()->w(gk.px(240))->child(gk.input(a.name)))->
		child(gk.label(fmt.tprintf("hello %s", a.name->value(cx)))) // read: redrawn when it changes
}

main :: proc() {
	app: App
	gk.run(&app, proc(a: ^App, cx: ^gk.Cx) {
		a.root = gk.view(a, init, view)
		gk.open_window(cx, "Hello", {480, 320}, a.root)
	})
}
