// negative: the mistakes the Odin API must make impossible or loud (docs/DESIGN.md section 14,
// item 7). Each mode is one mistake; the expected outcome is in measure/odin-negative.sh.
// Runs offscreen under a gk-meter script that clicks the first button a few times.
//
//   GK_NEG=rune58    a string made during a view is handler data and the handler stores it
//                    (it must show the right content)
//   GK_NEG=keep      event text stored with gk.keep: correct
//   GK_NEG=cast      event text stored through a string(x) cast: must stop, naming the turn
//   GK_NEG=cx        a ^Cx stored and used in a later turn: must stop
//   GK_NEG=pointer   a pointer as handler data: must stop at registration, naming type and line
//   GK_NEG=element   an element kept across view builds: must stop
//   GK_NEG=self      an element made a child of itself: must stop (the library would recurse
//                    until its stack ran out)
//   GK_NEG=view      a view made inside a view procedure (a new view on every build): must stop
//   GK_NEG=usize     a negative int where the kit takes a usize (it would arrive near 2^64): must stop
//   GK_NEG=twice     one handler with a value given to two builders (its value would be freed twice): must stop
//   GK_NEG=stored    a handler with a value kept past the turn that made it and given to a builder
//                    later (its value was freed when that turn ended): must stop
package negative

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

App :: struct {
	mode:     string,
	name:     gk.Input_State,
	selected: string, // stored by a handler
	kept:     string,
	clicks:   int,
	cx:       ^gk.Cx,
	old:      gk.Div,
	saved:    gk.Handler,
	root:     gk.View,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.name = gk.input_state(cx)->default_value("typed text")->build()
}

// select is the kept view string: `title` was made with tprintf during a view, in memory that is
// reused two views later. The binding copied it at registration and keeps the strings of a
// value whose handler ran, so storing it is correct.
select :: proc(a: ^App, cx: ^gk.Cx, title: string) {
	a.clicks += 1
	a.selected = title
}

store :: proc(a: ^App, cx: ^gk.Cx) {
	a.clicks += 1
	text := gk.value(a.name, cx) // gk.Temp_String: lent for this turn
	switch a.mode {
	case "keep":
		a.kept = gk.keep(text)
	case "cast":
		a.kept = string(text) // compiles, and is wrong: the memory dies with this turn
	case "cx":
		a.cx = cx
	case "stored":
		a.saved = gk.on(select, "kept past its turn") // given to no builder in this turn
	}
}

Item :: struct {
	id:   int,
	name: ^string,
}

with_pointer :: proc(a: ^App, cx: ^gk.Cx, item: Item) {}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	row := gk.h_flex(.Gap_2)
	switch a.mode {
	case "rune58":
		for i in 0 ..< 3 {
			// A different string in every build, so a stale one would show.
			title := fmt.tprintf("item %d of build after %d clicks", i, a.clicks)
			row->child(gk.button(fmt.tprintf("item-%d", i))->label("item")->on_click(gk.on(select, title)))
		}
	case "pointer":
		s := "x"
		row->child(gk.button("p")->label("p")->on_click(gk.on(with_pointer, Item{1, &s})))
	case "cx":
		if a.cx != nil {
			gk.notify(a.cx, a.root) // a Cx from an earlier turn
		}
		row->child(gk.button("b")->label("store")->on_click(gk.on(store)))
	case "element":
		if a.clicks > 0 {
			a.old->text("again") // an element of an earlier build
		}
		a.old = gk.div()
		row->child(gk.button("b")->label("store")->on_click(gk.on(store)))
	case "self":
		d := gk.div()->w(gk.px(10))
		d->child(d)
		return d
	case "view":
		row->child_view(gk.view(a, nil, view))
	case "twice":
		h := gk.on(select, "one value")
		row->child(gk.button("t1")->label("a")->on_click(h))
		row->child(gk.button("t2")->label("b")->on_click(h))
	case "stored":
		row->child(gk.button("b")->label("store")->on_click(gk.on(store)))
		if a.clicks > 0 {
			row->child(gk.button("s")->label("saved")->on_click(a.saved))
		}
	case "usize":
		row->child(gk.badge()->count(a.clicks - 1)->child(gk.label("b")))
	case:
		row->child(gk.button("b")->label("store")->on_click(gk.on(store)))
	}
	fmt.printf("gk-neg view clicks=%d selected=%q kept=%q\n", a.clicks, a.selected, a.kept)
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->
		bg(gk.theme(.Background))->
		text_color(gk.theme(.Foreground))->
		child(row)->
		child(gk.label(fmt.tprintf("selected %s kept %s", a.selected, a.kept)))
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "negative", {480, 200}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_NEG", context.allocator)
	script := "size 480 200\nframe\nclick 40 32\nframe\nclick 40 32\nframe\nclick 40 32\nframe\nframe\n"
	gk.run_headless(&app, script, start)
	fmt.printf("gk-neg done mode=%s clicks=%d selected=%q kept=%q\n", app.mode, app.clicks, app.selected, app.kept)
}
