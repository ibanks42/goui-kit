// negative_values: mistakes against the values family's protocols (docs/PROTOCOLS.md sections
// 13.8 to 13.12) that must stop with a message and the app's line, never decode a value inside
// itself until the library's stack runs out. Run offscreen with the diag library by
// gates/misuse.sh, one mode per case:
//
//   GK_NEG=val-ok          a scale made in a handler and read in the view: correct (exit 0)
//   GK_NEG=val-self        a value made in a handler given to its own builder: must stop
//   GK_NEG=val-self-view   the same in a view procedure: must stop
//   GK_NEG=val-write-view  a theme field written in a view procedure: must stop
//   GK_NEG=val-action-view a kit action value named in a view procedure: must stop
//   GK_NEG=val-bad-time    a time of day that does not exist (24:00) given to a time field: must stop
//   GK_NEG=val-scroll-freed a released ScrollHandle tracked by a view: must stop
//   (the components family, PROTOCOLS.md 11.17 and 11.25)
//   GK_NEG=c6-rules-source  selection rules given to a row source that is not a select's: must stop
//   GK_NEG=c6-rules-limits  selection rules whose min is above their max: must stop
//   GK_NEG=c6-tooltip-kept  a tooltip value made in a handler and given to a view later: must stop
package negative_values

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

App :: struct {
	mode:   string,
	root:   gk.View,
	band:   gk.Scale_Band,
	scroll: gk.Scroll_Handle,
	clicks: int,
	drawn:  int,
	tip:    gk.Kit_Tooltip, // c6-tooltip-kept: made in a handler, kept past its turn
}

clicked :: proc(a: ^App, cx: ^gk.Cx) {
	a.clicks += 1
	switch a.mode {
	case "val-ok":
		if a.band.h == 0 {
			a.band = gk.scale_band(cx, {"x", "y"}, {0, 100})->build()
		}
	case "val-self":
		t := gk.tree_item("a", "A")
		t = t->child(t)
	case "val-bad-time":
		tf := gk.time_field_state(cx)->build()
		gk.set_time(tf, cx, gk.Naive_Time{24, 0, 0, 0})
	case "val-scroll-freed":
		gk.release(cx, a.scroll)
	case "c6-rules-source":
		other := gk.row_source(cx, a, 3, proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {return gk.label("x")})
		gk.search_will_change(cx, other, gk.Search_Rules{max = 1})
	case "c6-rules-limits":
		src := gk.search_source(cx, a, []gk.Search_Item{gk.search_item("a", "A")})
		gk.search_will_change(cx, src, gk.Search_Rules{min = 3, max = 1})
	case "c6-tooltip-kept":
		a.tip = gk.kit_tooltip("made in a handler")
	}
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.drawn += 1
	body := gk.div(.Size_Full)->id("all")->on_click(gk.on(clicked))
	switch a.mode {
	case "val-ok":
		if a.band.h != 0 {
			x, _ := gk.tick(a.band, cx, "y").?
			body = body->child(gk.text(fmt.tprintf("y at %.0f", x)))
		}
	case "val-self-view":
		t := gk.tree_item("a", "A")
		t = t->child(t)
	case "val-write-view":
		gk.set_font_size(gk.theme_global(cx), cx, 20)
	case "val-action-view":
		_ = gk.action_of(cx, gk.cancel_default())
	case "val-scroll-freed":
		body = body->child(gk.div(.H_32)->id("list")->overflow_y_scroll()->track_scroll(a.scroll))
	case "c6-tooltip-kept":
		if a.clicks > 0 {
			body = body->child(gk.div()->id("tip")->w(gk.px(80))->h(gk.px(30))->tooltip(a.tip))
		}
	}
	return body
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	if a.mode == "val-scroll-freed" {
		a.scroll = gk.scroll_handle(cx)
	}
	a.root = gk.view(a, nil, view)
	gk.open_window(cx, "negative_values", {400, 300}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_NEG", context.allocator)
	gk.run_headless(&app, "size 400 300\nframe\nclick 200 150\nframe\nframe\nclick 200 150\nframe\nframe\nstat\n", start)
	fmt.printf("gk-neg done mode=%s drawn=%d clicks=%d\n", app.mode, app.drawn, app.clicks)
}
