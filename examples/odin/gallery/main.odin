// gallery: the Odin twin of ref/src/bin/gallery, the app that shows every stateless component
// in a fixed set of states, one page per component and state group. The pages are named by
// ref/gallery/pages.txt (compiled into both apps); gates/gallery.sh compares the two apps
// pixel for pixel, page by page. A page the Odin package cannot build yet is absent from
// PAGES below: it shows a placeholder and `--list` leaves it out, so the gate counts it as
// not bound instead of comparing it.
//
//   GK_SCRIPT=file gallery-odin   offscreen under a gk-meter script (the diag library)
//   gallery-odin --list           the page ids this app has, one per line
//   gallery-odin                  a real window (GK_FRAMES and GK_CLICK drive and measure it)
//   GALLERY_START=id              the page shown first
//   GALLERY_LEAK=bytes            leak that much on every view build: the memory gate's
//                                 self-test (gates/memory.sh --selftest must fail on it)
package gallery

import "base:runtime"
import "core:fmt"
import "core:mem"
import "core:os"
import "core:strconv"
import "core:strings"
import gk "gpuikit:gk"

PAGES_TXT :: #load("../../../ref/gallery/pages.txt", string)
CONF_TXT :: #load("../../../ref/gallery/conf.txt", string) // the generated conformance pages
ROWS :: 10_000

App :: struct {
	ids:   [dynamic]string,
	page:  int,
	count: int, // clicks on the current page's button
	flag:  bool, // the current page's checkbox or switch
	name:  gk.Input_State,
	rows:  gk.Rows,
	input: Input_Pages, // the input family's pages' own states (input_pages.odin)
	rows_family: Rows_Pages, // the rows family's pages' own states (rows_pages.odin)
	charts: Chart_Pages, // the charts' pages' own state (chart_pages.odin)
	lists2: Lists2_Pages, // the lists and editor families' pages (lists2_pages.odin)
	editor: Editor_Pages, // the editor family's pages (editor_pages.odin)
	root:  gk.View,
	leak:  int,
}

Page :: struct {
	id:    string,
	build: proc(a: ^App, cx: ^gk.Cx) -> gk.Element,
}

load_ids :: proc(a: ^App) {
	load_list(a, PAGES_TXT)
	load_list(a, CONF_TXT)
}

load_list :: proc(a: ^App, list: string) {
	text := list
	for line in strings.split_lines_iterator(&text) {
		l := strings.trim_space(line)
		if len(l) == 0 || l[0] == '#' {
			continue
		}
		id := l
		if i := strings.index_byte(l, '|'); i >= 0 {
			id = l[:i]
		}
		append(&a.ids, strings.trim_space(id))
	}
}

find_page :: proc(id: string) -> (Page, bool) {
	for p in PAGES {
		if p.id == id {
			return p, true
		}
	}
	for p in CONF {
		if p.id == id {
			return p, true
		}
	}
	for p in INPUT_PAGES {
		if p.id == id {
			return p, true
		}
	}
	for p in ROWS_PAGES {
		if p.id == id {
			return p, true
		}
	}
	for p in CHART_PAGES {
		if p.id == id {
			return p, true
		}
	}
	for p in LISTS2_PAGES {
		if p.id == id {
			return p, true
		}
	}
	for p in OVERLAY_PAGES { // overlay_pages.odin (docs/PROTOCOLS.md 11.6)
		if p.id == id {
			return p, true
		}
	}
	for p in EDITOR_PAGES { // editor_pages.odin (docs/PROTOCOLS.md 9.20 to 9.23)
		if p.id == id {
			return p, true
		}
	}
	for p in VALUES_PAGES { // values_pages.odin (docs/PROTOCOLS.md 13.8 to 13.12)
		if p.id == id {
			return p, true
		}
	}
	for p in VIEWS_PAGES { // views_pages.odin (the views family)
		if p.id == id {
			return p, true
		}
	}
	for p in MOTION_PAGES { // motion_pages.odin (the platform and motion families)
		if p.id == id {
			return p, true
		}
	}
	return {}, false
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.name = gk.input_state(cx)->placeholder("Name")->build()
	a.rows = gk.rows(cx)
	all := make([]string, ROWS, context.temp_allocator)
	for &r, i in all {
		r = fmt.tprintf("row %d", i)
	}
	a.rows->push(cx, ..all)
	input_pages_init(a, cx)
	rows_pages_init(a, cx)
	chart_pages_init(a, cx)
	lists2_pages_init(a, cx)
	overlay_pages_init(a, cx)
	editor_pages_init(a, cx) // the editor family (once, in the order find_page and the Rust twin's or_else chain look pages up)
	values_pages_init(a, cx)
	views_pages_init(a, cx) // the views family
	motion_pages_init(a, cx) // the platform and motion families
}

next :: proc(a: ^App, cx: ^gk.Cx) {
	a.page = (a.page + 1) % len(a.ids)
	a.count = 0
	a.flag = false
	a.name->set_value(cx, "")
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if a.leak > 0 {
		lost, _ := mem.alloc_bytes(a.leak, allocator = runtime.heap_allocator())
		for &b in lost {
			b = 1 // touched, so it is resident
		}
	}
	id := a.ids[a.page]
	content: gk.Element
	if p, ok := find_page(id); ok {
		content = p.build(a, cx)
	} else {
		content = gk.label("not bound in Odin yet")
	}
	return gk.v_flex(.Size_Full)->
		bg(gk.theme(.Background))->
		text_color(gk.theme(.Foreground))->
		child(
			gk.h_flex(.P_4, .Gap_3, .Items_Center)->
			child(gk.button("next")->label("Next")->on_click(gk.on(next)))->
			child(gk.label(fmt.tprintf("%d/%d %s", a.page + 1, len(a.ids), id))),
		)->
		child(gk.div(.W_Full, .P_4)->child(content))
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, root_view)
	gk.open_window(cx, "Gallery", {800, 600}, a.root)
}

main :: proc() {
	app: App
	load_ids(&app)
	if len(os.args) > 1 && os.args[1] == "--list" {
		for id in app.ids {
			if _, ok := find_page(id); ok {
				fmt.println(id)
			}
		}
		return
	}
	if first, ok := os.lookup_env("GALLERY_START", context.allocator); ok {
		for id, i in app.ids {
			if id == first {
				app.page = i
			}
		}
	}
	if leak, ok := os.lookup_env("GALLERY_LEAK", context.allocator); ok {
		app.leak, _ = strconv.parse_int(leak)
	}
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
