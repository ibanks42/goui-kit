// charts: the protocol tests of the charts' calculators over the app's rows (docs/PROTOCOLS.md
// 12.15): the pie and stack layouts as library objects instantiated for a row of a data table,
// their value closures answered from a column (or, for a stack, from the column each key names),
// their results read back with each row as its index in the rows the app passed. Each mode runs
// offscreen (the diag library) and prints lines core/tests/run-charts.sh checks.
//
//   GK_PROTO=pie | stack | pie-bad-column
package charts_test

import "core:fmt"
import "core:os"
import gk "gpuikit:gk"

Visit :: struct {
	month:  string,
	visits: f64,
	gap:    Maybe(f64),
}

Fruit :: struct {
	month:  string,
	apples: f64,
	pears:  f64,
}

App :: struct {
	mode: string,
	root: gk.View,
}

pie :: proc(a: ^App, cx: ^gk.Cx) {
	rows := []Visit{{"jan", 10, 1}, {"feb", 30, nil}, {"mar", 20, 2}}
	p := gk.pie(cx)->value("visits")->build()
	arcs := gk.arcs(p, cx, gk.data(rows[:]))
	total: f32
	for x in arcs {
		total += x.end_angle - x.start_angle
	}
	fmt.printf("overlay-charts pie arcs=%d rows=%d,%d,%d values=%v,%v,%v\n", len(arcs), arcs[0].row, arcs[1].row, arcs[2].row,
		arcs[0].value, arcs[1].value, arcs[2].value)
	fmt.printf("overlay-charts pie sweep=%.4f order=%d,%d,%d\n", total, arcs[0].index, arcs[1].index, arcs[2].index)
	// a gap (Maybe without a value) is no slice: the pie of the gap column has two arcs
	g := gk.pie(cx)->value("gap")->start_angle(0)->end_angle(3.14159265)->pad_angle(0)->build()
	ga := gk.arcs(g, cx, gk.data(rows[:]))
	fmt.printf("overlay-charts pie gap arcs=%d end=%.3f\n", len(ga), ga[len(ga) - 1].end_angle if len(ga) > 0 else 0)
	gk.release(cx, p)
	gk.release(cx, g)
	fmt.println("overlay-charts done mode=pie")
}

stack :: proc(a: ^App, cx: ^gk.Cx) {
	rows := []Fruit{{"jan", 1, 2}, {"feb", 3, 4}}
	keys := []string{"apples", "pears"}
	s := gk.stack(cx)->data(gk.data(rows[:]))->keys(keys)->value("")->build()
	series := gk.series(s, cx)
	fmt.printf("overlay-charts stack series=%d keys=%s,%s\n", len(series), series[0].key, series[1].key)
	p := series[1].points
	fmt.printf("overlay-charts stack pears jan=%v..%v feb=%v..%v rows=%d,%d\n", p[0].y0, p[0].y1, p[1].y0, p[1].y1, p[0].row, p[1].row)
	// a named column: every key reads it
	t := gk.stack(cx)->data(gk.data(rows[:]))->keys(keys)->value("apples")->build()
	q := gk.series(t, cx)[1].points
	fmt.printf("overlay-charts stack named feb=%v..%v\n", q[1].y0, q[1].y1)
	fmt.println("overlay-charts done mode=stack")
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->child(gk.label("charts"))
}

init :: proc(a: ^App, cx: ^gk.Cx) {
	switch a.mode {
	case "pie":
		gk.after(cx, 10, gk.on(pie))
	case "stack":
		gk.after(cx, 10, gk.on(stack))
	}
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	a.root = gk.view(a, init, root_view)
	gk.open_window(cx, "charts", {320, 240}, a.root)
}

main :: proc() {
	app: App
	app.mode, _ = os.lookup_env("GK_PROTO", context.allocator)
	gk.run_headless(&app, "size 320 240\nframe\nadvance 10\nframe\nframe\nadvance 20\nframe\nstat\n", start)
}
