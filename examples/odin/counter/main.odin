// counter: a small app in Odin, the same app as ref/src/bin/counter.rs: a kit
// Button, a Label with the count, an Input echoed into a second Label, and a list of 10,000
// rows, in three views inside a root view. State lives here, in Odin.
//
//   counter-odin                 a real window
//   GK_SCRIPT=file counter-odin  offscreen under a gk-meter script (the diag library)
//   GK_TEST=handlers             on the first click, run the handler checks (return value, lease)
//   GK_POSTS=rate,seconds        post from a thread at that rate
package counter

import "core:fmt"
import "core:os"
import "core:strconv"
import "core:strings"
import "core:sync"
import "core:thread"
import "core:time"
import gk "gpuikit:gk"

ROWS :: 10_000

App :: struct {
	count: int,
	ticks: int, // bumped by background posts
	name:  gk.Input_State, // handles to kit state that lives in the library
	rows:  gk.Rows,
	root, controls, echo, list: gk.View,
	tested, busy: bool,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.name = gk.input_state(cx)->placeholder("Name")->build()
	a.rows = gk.rows(cx)
	all := make([]string, ROWS, context.temp_allocator)
	for &r, i in all {
		r = fmt.tprintf("row %d", i)
	}
	a.rows->push(cx, ..all) // one crossing for all rows
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->
		bg(gk.theme(.Background))->
		text_color(gk.theme(.Foreground))->
		child_view(a.controls)->
		child_view(a.echo)->
		child(gk.div(.W_Full)->h(gk.px(160))->child_view(a.list))
}

controls_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_2, .Items_Center)->
		child(gk.button("add")->primary()->label("Add")->on_click(gk.on(add)))->
		child(gk.label(fmt.tprintf("count %d", a.count + a.ticks)))->
		child(gk.div()->w(gk.px(240))->child(gk.input(a.name)))
}

// echo_view reads kit state through its handle: the library observes the input and has this
// view described again when it changes.
echo_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	name := gk.value(a.name, cx) // gk.Temp_String, valid until this turn ends
	if gk_test != "" {
		fmt.printf("gk-test echo value=%q\n", name)
	}
	return gk.h_flex()->child(gk.label(fmt.tprintf("hello %s", name)))
}

list_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.uniform_list("rows", a.rows)->style(.Size_Full)
}

add :: proc(a: ^App, cx: ^gk.Cx) {
	a.count += 1
	a.rows->push(cx, fmt.tprintf("added %d", a.count))
	if gk_test == "handlers" && !a.tested {
		a.tested = true
		handler_checks(a, cx)
	}
} // the controls view, which registered this handler, is marked dirty on return

// handler_checks: a handler's return value reaches the library; a read of a leased entity
// reports busy and the process goes on; a deferred write from that handler lands later.
handler_checks :: proc(a: ^App, cx: ^gk.Cx) {
	yes := gk.handler(proc(a: ^App, cx: ^gk.Cx) -> bool {return true})
	no := gk.handler(proc(a: ^App, cx: ^gk.Cx) -> bool {return false})
	fmt.printf("gk-test return true=%d false=%d\n", gk.test_fire(cx, yes), gk.test_fire(cx, no))
	inside := gk.handler(proc(a: ^App, cx: ^gk.Cx) -> bool {
		_, ok := gk.try_value(a.name, cx) // the input is on lease here
		a.busy = !ok
		gk.set_value(a.name, cx, "deferred") // a write; lands after the update
		return true
	})
	ret := gk.test_fire_in_update(cx, a.name, inside)
	fmt.printf("gk-test lease busy=%v ret=%d value_right_after=%q\n", a.busy, ret, gk.value(a.name, cx))
}

tick :: proc(a: ^App, cx: ^gk.Cx) {
	a.ticks += 1
	gk.notify(cx, a.controls)
}

sent: i64
rate, secs: int
// GK_TEST, read once in main: looking it up on every click and every echo was the app's own
// cost on each frame of the click-on-every-frame scene, which the Rust reference does not pay
// (seen in the profile of 11 Nov 2026, gates/profile.sh).
gk_test: string

poster :: proc() {
	time.sleep(time.Second) // let the window come up
	end := time.time_add(time.now(), time.Duration(secs) * time.Second)
	for time.diff(time.now(), end) > 0 {
		for _ in 0 ..< rate / 1000 {
			gk.post(tick)
			sync.atomic_add(&sent, 1)
		}
		time.sleep(time.Millisecond)
	}
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.controls = gk.view(a, nil, controls_view)
	a.echo = gk.view(a, nil, echo_view)
	a.list = gk.view(a, nil, list_view)
	a.root = gk.view(a, init, root_view)
	gk.open_window(cx, "Counter", {640, 480}, a.root)
}

main :: proc() {
	app: App
	gk_test, _ = os.lookup_env("GK_TEST", context.allocator)
	if path, ok := os.lookup_env("GK_SCRIPT", context.allocator); ok {
		script, err := os.read_entire_file(path, context.allocator)
		if err != nil {
			fmt.panicf("cannot read %s: %v", path, err)
		}
		gk.run_headless(&app, string(script), start)
		return
	}
	if posts, ok := os.lookup_env("GK_POSTS", context.allocator); ok {
		parts := strings.split(posts, ",")
		if len(parts) == 2 {
			rate, _ = strconv.parse_int(parts[0])
			secs, _ = strconv.parse_int(parts[1])
			thread.create_and_start(poster)
		}
	}
	gk.run(&app, start)
	fmt.printf("gk-app counter-odin count=%d ticks=%d posts_sent=%d\n", app.count, app.ticks, sync.atomic_load(&sent))
}
