// files: a small text viewer that uses the platform's services (docs/PROTOCOLS.md 11.30, 13.30):
// the menu bar (File: Open…, Save as…, Quit; Help: Website), the platform's file dialogs, a
// prompt before quitting, opening a URL, and a status line that slides and fades in when it
// changes. The menu bar is the platform's where it has one (macOS) and the kit's AppMenuBar drawn
// at the top of the window everywhere.
//
//   examples/build.sh files && bin/files-odin
//
// Offscreen (the diag library's driver) the dialogs are not shown: a test answers them with
// gk.platform_answer_paths (core/tests/platform does), or the driver's script with `answer ..`
// (gates/files.script, against its Rust twin ref/src/bin/files.rs: gates/pixels.sh files).
package files

import "core:fmt"
import "core:os"
import "core:strings"
import "core:time"
import gk "gpuikit:gk"

App :: struct {
	root:    gk.View,
	menubar: gk.App_Menu_Bar,
	open:    gk.Action,
	save:    gk.Action,
	quit:    gk.Action,
	site:    gk.Action,
	path:    string,
	text:    string,
	status:  string,
	changes: int, // a new id for the status line's animation each time it changes
	asking:  gk.Completion,
}

set_status :: proc(a: ^App, s: string) {
	delete(a.status)
	a.status = strings.clone(s)
	a.changes += 1
}

opened :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) {
	switch d.status {
	case .Answered:
		data, err := os.read_entire_file(d.paths[0], context.allocator)
		if err != nil {
			set_status(a, fmt.tprintf("could not read %s", d.paths[0]))
			return
		}
		delete(a.text)
		delete(a.path)
		a.text = string(data)
		a.path = strings.clone(d.paths[0])
		set_status(a, fmt.tprintf("opened %s (%d bytes)", a.path, len(data)))
	case .Cancelled:
		set_status(a, "open cancelled")
	case .Failed:
		set_status(a, fmt.tprintf("no file dialog: %s", d.error))
	}
}

saved_to :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) {
	if d.status != .Answered || len(d.paths) == 0 {
		set_status(a, "save cancelled")
		return
	}
	if os.write_entire_file(d.paths[0], transmute([]u8)a.text) == nil {
		set_status(a, fmt.tprintf("saved %s", d.paths[0]))
	} else {
		set_status(a, fmt.tprintf("could not write %s", d.paths[0]))
	}
}

quit_answered :: proc(a: ^App, cx: ^gk.Cx, d: gk.Prompt_Done) {
	a.asking = {}
	if d.status == .Answered && d.answer == 0 {
		gk.quit(cx)
		return
	}
	set_status(a, "still here")
}

do_open :: proc(a: ^App, cx: ^gk.Cx) {
	gk.prompt_for_paths(cx, {files = true, prompt = "Open"}, gk.on(opened))
}

do_save :: proc(a: ^App, cx: ^gk.Cx) {
	dir := "."
	if a.path != "" {
		dir = a.path[:max(strings.last_index_byte(a.path, '/'), 0)]
	}
	gk.prompt_for_new_path(cx, dir, gk.on(saved_to), "untitled.txt")
}

do_quit :: proc(a: ^App, cx: ^gk.Cx) {
	if a.asking.h != 0 {
		return // one question at a time
	}
	a.asking = gk.prompt(cx, .Warning, "Quit the viewer?", {{.Ok, "Quit"}, {.Cancel, "Stay"}}, gk.on(quit_answered))
}

do_site :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_url(cx, "https://github.com/longbridge/gpui-component")
	set_status(a, "opened the website in the browser")
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.open = gk.action(cx, "files::Open")
	a.save = gk.action(cx, "files::SaveAs")
	a.quit = gk.action(cx, "files::Quit")
	a.site = gk.action(cx, "files::Website")
	gk.bind_keys(cx, {"ctrl-o", a.open, ""}, {"ctrl-shift-s", a.save, ""}, {"ctrl-q", a.quit, ""})
	gk.on_app_action(cx, a.open, gk.on(do_open))
	gk.on_app_action(cx, a.save, gk.on(do_save))
	gk.on_app_action(cx, a.quit, gk.on(do_quit))
	gk.on_app_action(cx, a.site, gk.on(do_site))
	file := []gk.Menu_Item{
		gk.Menu_Action{name = "Open…", action = a.open},
		gk.Menu_Action{name = "Save as…", action = a.save},
		gk.Menu_Separator{},
		gk.Menu_Action{name = "Quit", action = a.quit},
	}
	help := []gk.Menu_Item{gk.Menu_Action{name = "Website", action = a.site}}
	gk.set_menus(cx, {name = "File", items = file}, {name = "Help", items = help})
	a.menubar = gk.app_menu_bar(cx)
	set_status(a, "File > Open… (ctrl-o) to read a text file")
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	status := gk.div(.Relative, .Text_Sm)->h(gk.px(24))->text_color(gk.theme(.Muted_Foreground))->child(gk.label(a.status))
	enter := gk.effect_transition(250 * time.Millisecond)->fade(0, 1)->slide_y(8, 0)
	return gk.v_flex(.Size_Full)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.div(.Flex_None)->h(gk.px(32))->child(gk.app_menu_bar_view(a.menubar)))->
		child(gk.v_flex(.Flex_1, .P_4, .Gap_2)->
			child(gk.div(.Flex_1, .Border_1, .P_2, .Overflow_Hidden)->border_color(gk.theme(.Border))->
				child(gk.label(a.text == "" ? "(no file)" : a.text)))->
			child(gk.effect(status, enter, fmt.tprintf("status-%d", a.changes))))
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, view)
	gk.open_window(cx, "Files", {720, 480}, a.root)
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
