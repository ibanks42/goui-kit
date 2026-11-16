// negative_lists2: mistakes against the lists and editor families' protocols (docs/PROTOCOLS.md
// sections 9.9 to 9.12) that must stop with a message, never draw wrong content with exit 0. Run
// offscreen with the diag library by gates/misuse.sh, one mode per case:
//
//   GK_NEG=sel-ok        a select over items, its value set and read back: correct (exit 0)
//   GK_NEG=sel-foreign   gk.search_items on a row source that is not a select's: must stop
//   GK_NEG=sel-view      gk.search_items in a view procedure (a write): must stop
//   GK_NEG=expr-syntax   an expression closure that does not parse: must stop
//   GK_NEG=expr-arg      an expression that uses an argument the closure is not handed: must stop
//   GK_NEG=expr-func     an expression that calls a function with the wrong arguments: must stop
//   GK_NEG=nonzero       Easing's step count given 0 (upstream's NonZeroU32): must stop
//   GK_NEG=kept-view     a handler given to a call in a view procedure (it would be kept per
//                        build): must stop
package negative_lists2

import "core:fmt"
import "core:os"
import "core:time"
import gk "gpuikit:gk"

App :: struct {
	mode:  string,
	root:  gk.View,
	src:   gk.Row_Source,
	sel:   gk.Select_State,
	inp:   gk.Input_State,
	drawn: int,
}

ITEMS := [?]gk.Search_Item{{value = "a", title = "A"}, {value = "b", title = "B"}}

never :: proc(a: ^App, cx: ^gk.Cx, text: gk.Temp_String) -> bool {
	return true
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "sel-ok", "sel-view":
		a.src = gk.search_source(cx, a, ITEMS[:])
		a.sel = gk.select_state(cx, a.src, nil)->build()
		gk.set_selected_value(a.sel, cx, "b")
	case "sel-foreign":
		other := gk.row_source(cx, a, 1, proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {return gk.label("x")})
		gk.search_items(cx, other, ITEMS[:])
	case "expr-syntax":
		a.inp = gk.input_state(cx)->step_by("x <")->build()
	case "expr-arg":
		a.inp = gk.input_state(cx)->step_by("z * 2")->build()
	case "expr-func":
		a.inp = gk.input_state(cx)->step_by("clamp(x, 1)")->build()
	case "kept-view":
		a.inp = gk.input_state(cx)->build()
	}
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.drawn += 1
	body := gk.v_flex(.Size_Full)
	switch a.mode {
	case "sel-ok":
		v, ok := gk.selected_value(a.sel, cx).?
		fmt.printf("gk-neg sel-ok value=%q ok=%v\n", v, ok)
		body = body->child(gk.select(a.sel))
	case "sel-view":
		gk.search_items(cx, a.src, ITEMS[:])
	case "nonzero":
		_ = gk.transition(200 * time.Millisecond)->easing(gk.Easing_Steps{count = 0, position = .Jump_End})
	case "kept-view":
		gk.set_validator(a.inp, cx, gk.on(never))
	}
	return body
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "negative_lists2", {400, 300}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_NEG", context.allocator)
	gk.run_headless(&app, "size 400 300\nframe\nframe\n", start)
	fmt.printf("gk-neg done mode=%s drawn=%d\n", app.mode, app.drawn)
}
