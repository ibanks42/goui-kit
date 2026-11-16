// negative_overlay: mistakes against view-built closures (docs/PROTOCOLS.md section 11.6) that
// must stop with a message, never draw a dialog from the wrong view. Run offscreen with the diag
// library by gates/misuse.sh, one mode per case:
//
//   GK_NEG=built-ok      a dialog opened from a handler and drawn: correct (exit 0)
//   GK_NEG=built-root    the procedure returns a node that is not a Dialog: must stop
//   GK_NEG=built-freed   the view of an open dialog is released: its next build must stop
//   GK_NEG=built-view    gk.built in a view procedure: must stop at its line
//   GK_NEG=built-none    open_dialog given a Built that names no view: must stop at its line
//   GK_NEG=dock-node     a dock call given a node id the dock never handed out: must stop
//                        (docs/PROTOCOLS.md 11.8)
//   GK_NEG=dock-register gk.register_panel in a view procedure: must stop at its line
package negative_overlay

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

App :: struct {
	mode:  string,
	root:  gk.View,
	dlg:   gk.Built(gk.Dialog),
	built: int,
	dock:  gk.Dock_Area,
}

good :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	a.built += 1
	return gk.dialog()->title(gk.label("Fine"))
}

bad :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	return gk.Dialog{el = gk.label("not a dialog").el}
}

open :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "built-root":
		gk.open_dialog(cx, gk.built(cx, a, bad))
	case "built-none":
		gk.open_dialog(cx, gk.Built(gk.Dialog){})
	case "dock-node":
		a.dock = gk.dock_area_skinned(cx, "neg")
		a.dock->set_zoomed_in(cx, gk.Node_Id(424242)) // made up: no read handed it out
	case:
		a.dlg = gk.built(cx, a, good)
		gk.open_dialog(cx, a.dlg)
	}
}

later :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "built-freed":
		gk.release(cx, a.dlg.view)
		gk.open_alert_dialog(cx, gk.built(cx, a, proc(a: ^App, cx: ^gk.Cx) -> gk.Alert_Dialog {return gk.alert_dialog()}))
	case:
		fmt.printf("gk-neg done mode=%s built=%d active=%v\n", a.mode, a.built, gk.has_active_dialog(cx))
	}
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if a.mode == "built-view" {
		gk.built(cx, a, good) // a view procedure may not make views
	}
	if a.mode == "dock-register" {
		gk.register_panel(cx, "x", gk.on(proc(a: ^App, cx: ^gk.Cx) {})) // only in init or a handler
	}
	return gk.div(.Size_Full)->child(gk.label("negative overlay"))
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	gk.after(cx, 10, gk.on(open))
	gk.after(cx, 30, gk.on(later))
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "negative_overlay", {640, 480}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_NEG", context.allocator)
	gk.run_headless(&app, "size 640 480\nframe\nadvance 10\nframe\nframe\nadvance 20\nframe\nframe\nadvance 20\nframe\nstat\n", start)
}
