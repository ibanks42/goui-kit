// negative_data: mistakes against the data protocol (docs/PROTOCOLS.md section 12) that must
// stop with a message naming the column or template, never draw a chart from a wrong column.
// Run offscreen with the diag library by gates/misuse.sh, one mode per case:
//
//   GK_NEG=data-ok        a line chart over the rows, drawn: correct (exit 0)
//   GK_NEG=data-column    ->y("dekstop"), a column the rows do not have: must stop
//   GK_NEG=data-kind      ->y("month"), a text column where the chart wants a number: must stop
//   GK_NEG=data-number    ->y_tick_format("{} of {}"): the closure is handed one number: must stop
//   GK_NEG=data-brace     ->tooltip_value("{:.1 visits"): a brace that is not closed: must stop
//   GK_NEG=data-row       ->y_tick_format("{month}"): a tick has no row to read a column from
//   GK_NEG=data-field     gk.data of rows with a slice field, which a table cannot carry
package negative_data

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

Month :: struct {
	month:   string,
	desktop: f64,
}

Bad :: struct {
	month: string,
	parts: []int,
}

ROWS := [?]Month{{"Jan", 186}, {"Feb", 305}, {"Mar", 237}}
BAD := [?]Bad{{"Jan", nil}}

App :: struct {
	mode:  string,
	root:  gk.View,
	drawn: int,
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	a.drawn += 1
	c := gk.line_chart(gk.data(ROWS[:]))->appear(false)->x("month")
	switch a.mode {
	case "data-ok":
		c = c->y("desktop")->y_tick_format("{:.0}")->tooltip_value("{:.1} of {month}")
	case "data-column":
		c = c->y("dekstop")
	case "data-kind":
		c = c->y("month")
	case "data-number":
		c = c->y("desktop")->y_tick_format("{} of {}")
	case "data-brace":
		c = c->y("desktop")->tooltip_value("{:.1 visits")
	case "data-row":
		c = c->y("desktop")->y_tick_format("{month}")
	case "data-field":
		c = gk.line_chart(gk.data(BAD[:]))->x("month")
	}
	return gk.div(.Size_Full)->bg(gk.theme(.Background))->child(gk.div()->w(gk.px(300))->h(gk.px(200))->child(c))
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, proc(a: ^App, cx: ^gk.Cx) {}, view)
	gk.open_window(cx, "negative_data", {400, 300}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_NEG", context.allocator)
	gk.run_headless(&app, "size 400 300\nframe\nframe\n", start)
	fmt.printf("gk-neg done mode=%s drawn=%d\n", app.mode, app.drawn)
}
