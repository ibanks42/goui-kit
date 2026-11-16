// writerule: the write-rule case in Odin, the twin of ref/src/bin/writerule.rs (DESIGN.md
// section 7, "When a write lands"; docs/PROTOCOLS.md section 3). A per-frame handler writes
// the frame's number into an input's state; a second view echoes the input's value. The rule
// says the write is in the pixels of the frame the handler ran in, so every image must show
// the same number in the input and in the echo, as the Rust reference does.
//
// The handler does not notify the echo view: that view is described again only because it
// read the state that was written. An app that notified it by hand would hide a broken rule.
//
//   GK_SCRIPT=file writerule-odin   offscreen under a gk-meter script (the diag library)
package writerule

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

App :: struct {
	frame:      int,
	name:       gk.Input_State,
	root, echo: gk.View,
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.name = gk.input_state(cx)->placeholder("Name")->build()
	gk.on_frame(cx, frame)
}

frame :: proc(a: ^App, cx: ^gk.Cx) {
	a.frame += 1
	gk.set_value(a.name, cx, fmt.tprintf("frame %d", a.frame))
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->
		bg(gk.theme(.Background))->
		text_color(gk.theme(.Foreground))->
		child(gk.div()->w(gk.px(240))->child(gk.input(a.name)))->
		child_view(a.echo)
}

echo_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex()->child(gk.label(fmt.tprintf("echo %s", gk.value(a.name, cx))))
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.echo = gk.view(a, nil, echo_view)
	a.root = gk.view(a, init, root_view)
	gk.open_window(cx, "Write rule", {480, 200}, a.root)
}

main :: proc() {
	app: App
	if path, ok := os.lookup_env("GK_SCRIPT", context.allocator); ok {
		script, err := os.read_entire_file(path, context.allocator)
		if err != nil {
			fmt.panicf("cannot read %s: %v", path, err)
		}
		gk.run_headless(&app, string(script), start)
		return
	}
	gk.run(&app, start)
}
