// panels: the `panels` scene in Odin, the same dashboard as ref/src/bin/panels.rs (a port of
// gpui-fast's views_frames benchmark). 60 panels of 64 labels, each panel a view of its own;
// `changing` of them change before every frame.
//
//   panels-odin [panels] [labels] [changing]      (default 60 64 2)
package panels

import "core:fmt"
import "core:os"
import "core:strconv"
import gk "gpuikit:gk"

LABELS :: 97 // distinct labels a cell can show

Panel :: struct {
	app:         ^App,
	index, tick: int,
	view:        gk.View,
}

App :: struct {
	panels:          [dynamic]Panel,
	labels:          [LABELS]string,
	count, changing: int,
	tick:            int,
	root:            gk.View,
}

panel_view :: proc(p: ^Panel, cx: ^gk.Cx) -> gk.Element {
	hover := gk.style_refinement_default()->bg(gk.hsla(0.1, 0.6, 0.4, 1))
	cells := gk.div(.Flex, .Flex_Row, .Flex_Wrap)
	for cell in 0 ..< p.app.count {
		cells->child(gk.div()->w(gk.px(26))->h(gk.px(14))->hover(hover)->text(p.app.labels[(cell + p.tick) % LABELS]))
	}
	return gk.div(.Flex, .Flex_Col)->w(gk.px(220))->style(.P_1, .Gap_1, .Rounded_Sm)->
		bg(gk.hsla(0.6, 0.2, 0.18, 1))->
		text(fmt.tprintf("panel %d · %d", p.index, p.tick))->
		child(cells)
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	d := gk.div(.Size_Full, .Flex, .Flex_Row, .Flex_Wrap, .Gap_1)->
		bg(gk.hsla(0, 0, 0.12, 1))->
		text_color(gk.hsla(0, 0, 0.85, 1))->
		style(.Text_Xs)
	for p in a.panels {
		d->child_view(p.view)
	}
	return d
}

// frame runs before every frame (only because init asked for it): change a few panels and
// mark only them, as a timer or a message from elsewhere would.
frame :: proc(a: ^App, cx: ^gk.Cx) {
	a.tick += 1
	for n in 0 ..< a.changing {
		p := &a.panels[(a.tick * 7 + n * 13) % len(a.panels)]
		p.tick += 1
		gk.notify(cx, p.view)
	}
}

root_init :: proc(a: ^App, cx: ^gk.Cx) {
	if a.changing > 0 && len(a.panels) > 0 {
		gk.on_frame(cx, frame)
	}
}

arg :: proc(i, def: int) -> int {
	if len(os.args) > i {
		if v, ok := strconv.parse_int(os.args[i]); ok {
			return v
		}
	}
	return def
}

main :: proc() {
	app: App
	app.count = arg(2, 64)
	app.changing = arg(3, 2)
	resize(&app.panels, arg(1, 60))
	for &l, i in app.labels {
		l = fmt.aprintf("%02d", i)
	}
	gk.run(&app, proc(a: ^App, cx: ^gk.Cx) {
		for &p, i in a.panels {
			p = Panel{a, i, i, {}}
			p.view = gk.view(&p, nil, panel_view)
		}
		a.root = gk.view(a, root_init, root_view)
		gk.open_window(cx, "Panels", {1400, 900}, a.root)
	})
}
