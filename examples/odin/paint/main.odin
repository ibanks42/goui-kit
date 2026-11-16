// paint: gradient backgrounds and a plot of the binding's own paths and quads (docs/PROTOCOLS.md 11.38,
// 12.17), the Odin twin of ref/src/bin/paint.rs and examples/go/paint. gates/pixels.sh paint compares
// the three image by image.
package paint

import gk "gpuikit:gk"

App :: struct {
	root: gk.View,
}

cyan :: proc(a: f32) -> gk.Color {return gk.hsla(0.5, 0.75, 0.55, a)}
violet :: proc(a: f32) -> gk.Color {return gk.hsla(0.75, 0.6, 0.65, a)}

glow :: proc(a: f32) -> gk.Linear_Gradient {
	return gk.linear_gradient(90, gk.linear_color_stop(cyan(a), 0), gk.linear_color_stop(violet(a), 1))
}

swatch :: proc() -> gk.Div {
	return gk.div()->w(gk.px(160))->h(gk.px(48))->rounded(gk.px(8))
}

view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	// the wave: integers only, so every language computes the same pixels
	pts: [49][2]f32
	for i in 0 ..= 48 {
		pts[i] = {f32(i * 12), f32(40 + (i * 37) % 150)}
	}
	area: [51][2]f32 // down to the middle line at both ends
	area[0] = {0, 120}
	for p, i in pts {
		area[i + 1] = p
	}
	area[50] = {576, 120}
	white := gk.hsla(0, 0, 1, 1)
	bar := gk.linear_gradient(0, gk.linear_color_stop(gk.theme(.Primary), 0), gk.linear_color_stop(violet(1), 1), oklab = true)
	plot := gk.plot(
		gk.grid()->x([]f32{144, 288, 432})->y([]f32{60, 120, 180})->stroke(gk.theme(.Border)),
		gk.plot_area(area[:], gk.hsla(0.6, 0.8, 0.7, 0.12)),
		gk.plot_line(pts[:], 7, glow(0.12)),
		gk.plot_line(pts[:], 3, glow(0.25)),
		gk.plot_line(pts[:], 1.5, glow(1)),
		gk.plot_quad({480, 10, 80, 36}, bar, {corner_radius = 6, border_width = 2, border_color = white}),
		gk.plot_quad({283, 103, 10, 10}, white, {corner_radius = 5}),
	)
	warm := gk.linear_gradient(180, gk.linear_color_stop(gk.theme(.Primary), 0.2), gk.linear_color_stop(gk.hsla(0.1, 0.9, 0.6, 1), 0.9), oklab = true)
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.h_flex(.Gap_3)->
			child(swatch()->bg(glow(1)))->
			child(swatch()->bg(warm))->
			child(swatch()->bg(gk.hsla(0.33, 0.5, 0.5, 1))))->
		child(gk.div()->w(gk.px(576))->h(gk.px(240))->child(plot))
}

main :: proc() {
	app: App
	gk.run(&app, proc(a: ^App, cx: ^gk.Cx) {
		a.root = gk.view(a, nil, view)
		gk.open_window(cx, "Paint", {640, 400}, a.root)
	})
}
