// Every Odin call that docs/API.md shows, collected into one program that compiles, so the guide
// cannot drift from the package (the docs tests check that each call in API.md is text of this
// file). Not an app to read for itself: the tutorial in docs/tutorial is.
package api

import "core:fmt"
import "core:strings"
import "core:time"
import gk "gpuikit:gk"

App :: struct {
	name:     gk.Input_State,
	list:     gk.List_State,
	save:     gk.Action,
	dark:     bool,
	items:    [dynamic]string,
	saved_at: string,
}

Loaded :: struct {
	names: []string,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	// Kit state: Rust's cx.new(|cx| InputState::new(window, cx).placeholder("Name"))
	a.name = gk.input_state(cx)->placeholder("Name")->build()
	gk.on_input_event(a.name, cx, gk.on(name_event))
	a.list = gk.list_state(cx, gk.list_source(cx, a, item_count, item_row))->build()
	// actions!, KeyBinding::new and on_action, in one call
	a.save = gk.shortcut(cx, "ctrl-s", gk.on(save))
	gk.after(cx, 500, gk.on(tick))
	gk.theme_change(cx, gk.Theme_Mode.Dark)
}

item_count :: proc(a: ^App) -> int {return len(a.items)}

item_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	return gk.list_item(gk.id("item", at.row))->child(gk.label(a.items[at.row]))
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	ok := gk.button("ok")->primary()->label("OK")->on_click(gk.on(clicked))
	tags := gk.h_flex(.Gap_2)->
		child(gk.tag_secondary()->text("draft"))->
		child(gk.switch_("wifi")->checked(a.dark))->
		child(gk.icon(.Star_Fill)->text_color(gk.theme(.Warning)))->
		child(gk.base_checkbox("raw"))
	row := gk.div(.Size_Full, .Gap_2)->style(.Items_Center)->
		child(gk.div()->w(gk.px(240))->h(gk.rems(2))->bg(gk.hsla(0.6, 0.5, 0.5, 1)))->
		child(gk.div()->w(gk.relative(0.5))->border_color(gk.palette(.Blue_500)))->
		child(gk.label("bold", .Font_Bold))
	if a.dark {
		row = row->bg(gk.theme(.Muted))
	}
	return gk.v_flex(.Size_Full, .P_4)->bg(gk.theme(.Background))->
		child(ok)->
		child(tags)->
		child(row)->
		child(gk.input(a.name))->
		child(gk.list(a.list))->
		child(gk.button(gk.id("row", 3))->label("row 3")->on_click(gk.on(remove, 3)))->
		child(gk.button("ask")->label("Ask")->on_click(gk.on(ask)))
}

clicked :: proc(a: ^App, cx: ^gk.Cx, e: gk.Click_Event) {
	if e.click_count == 2 {
		fmt.println("double click")
	}
}

remove :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	gk.stop_propagation(cx)
	gk.refresh(cx)
}

name_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	#partial switch v in e {
	case gk.Input_Event_Press_Enter:
		text := gk.value(a.name, cx) // gk.Temp_String: lent until this handler returns
		append(&a.items, gk.keep(text)) // a copy the app owns
		gk.set_value(a.name, cx, "")
		gk.refresh(cx)
	}
}

list_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Event) {
	if c, ok := e.(gk.List_Event_Confirm); ok {
		fmt.println("row", c.row)
	}
}

save :: proc(a: ^App, cx: ^gk.Cx) {
	delete(a.saved_at)
	a.saved_at = strings.clone("now")
	gk.push_notification(cx, "Saved")
}

tick :: proc(a: ^App, cx: ^gk.Cx) {
	gk.refresh(cx)
}

forget :: proc(a: ^App, cx: ^gk.Cx) {
	gk.release(cx, a.list)
}

pick :: proc(a: ^App, cx: ^gk.Cx) {
	gk.set_selected_index(a.list, cx, 4) // an IndexPath: the row alone, or gk.Base_Index_Path{section = 1, row = 4}
	gk.scroll_to_item(a.list, cx, 4, .Top)
}

more :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window_with(cx, {width = 320, height = 240, title = "More", min_width = 200}, gk.view(a, nil, columns))
}

columns :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex()->child(gk.v_flex()->w(gk.auto()))
}

ask :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_alert_dialog(cx, gk.built(cx, a, confirm))
}

confirm :: proc(a: ^App, cx: ^gk.Cx) -> gk.Alert_Dialog {
	return gk.alert_dialog()->title(gk.label("Delete?"))->show_cancel(true)->on_ok(gk.on(confirmed))
}

confirmed :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	return true
}

load :: proc() {
	names := []string{"a", "b"}
	time.sleep(10 * time.Millisecond)
	gk.post(loaded, Loaded{names})
}

loaded :: proc(a: ^App, cx: ^gk.Cx, m: Loaded) {
	append(&a.items, ..m.names)
	gk.refresh(cx)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "API", {640, 480}, gk.view(a, init, view))
}

// --- windows and drawing: a window's icon and app id, asking before a window closes, the quit,
// reduced motion, gradients, a plot of paths and quads

ask_close :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	return !a.dark
}

quitting :: proc(a: ^App, cx: ^gk.Cx) {}

port_window :: proc(a: ^App, cx: ^gk.Cx, png: []u8, root: gk.View) {
	w := gk.open_window_with(cx, {title = "Notes", app_id = "notes", icon = png}, root)
	gk.on_window_should_close(cx, w, gk.on(ask_close))
	gk.on_app_quit(cx, gk.on(quitting))
	if gk.reduce_motion(cx) {
		fmt.println("still")
	}
}

port_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	cyan, violet := gk.hsla(0.5, 0.75, 0.55, 1), gk.hsla(0.75, 0.6, 0.65, 1)
	ys := []f32{60, 120}
	points := [][2]f32{{0, 10}, {40, 30}}
	fill := gk.Fill(cyan)
	x, y, w, h: f32 = 1, 2, 3, 4
	return gk.v_flex()->
		child(gk.div()->h(gk.px(8))->bg(gk.linear_gradient(90, gk.linear_color_stop(cyan, 0), gk.linear_color_stop(violet, 1))))->
		child(gk.div()->w(gk.px(200))->h(gk.px(100))->child(gk.plot(gk.grid()->y(ys), gk.plot_area(points, fill), gk.plot_line(points, 2.5, fill), gk.plot_quad({x, y, w, h}, fill))))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
