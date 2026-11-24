// paint: gradient backgrounds and a plot of the binding's own paths and quads (docs/PROTOCOLS.md
// 11.38, 12.17), the Go twin of ref/src/bin/paint.rs. gates/pixels.sh paint
// compares the two image by image.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct{ root gk.View }

func cyan(a float32) gk.Color   { return gk.HSLA(0.5, 0.75, 0.55, a) }
func violet(a float32) gk.Color { return gk.HSLA(0.75, 0.6, 0.65, a) }

func glow(a float32) gk.LinearGradient {
	return gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan(a), 0), gk.NewLinearColorStop(violet(a), 1))
}

func swatch() gk.DivEl { return gk.Div().W(gk.Px(160)).H(gk.Px(48)).Rounded(gk.Px(8)) }

func (a *App) view(cx *gk.Cx) gk.Element {
	// the wave: integers only, so every language computes the same pixels
	pts := make([][2]float32, 49)
	for i := range pts {
		pts[i] = [2]float32{float32(i * 12), float32(40 + (i*37)%150)}
	}
	area := append(append([][2]float32{{0, 120}}, pts...), [2]float32{576, 120}) // down to the middle line
	white := gk.HSLA(0, 0, 1, 1)
	bar := gk.NewLinearGradient(0, gk.NewLinearColorStop(theme.Primary, 0), gk.NewLinearColorStop(violet(1), 1)).InOklab()
	plot := gk.Plot(
		gk.NewGrid().X([]float32{144, 288, 432}).Y([]float32{60, 120, 180}).Stroke(theme.Border),
		gk.NewPlotArea(area, gk.HSLA(0.6, 0.8, 0.7, 0.12)),
		gk.NewPlotLine(pts, 7, glow(0.12)),
		gk.NewPlotLine(pts, 3, glow(0.25)),
		gk.NewPlotLine(pts, 1.5, glow(1)),
		gk.NewPlotQuad(480, 10, 80, 36, bar, gk.QuadStyle{CornerRadius: 6, BorderWidth: 2, BorderColor: white}),
		gk.NewPlotQuad(283, 103, 10, 10, white, gk.QuadStyle{CornerRadius: 5}),
	)
	warm := gk.NewLinearGradient(180, gk.NewLinearColorStop(theme.Primary, 0.2), gk.NewLinearColorStop(gk.HSLA(0.1, 0.9, 0.6, 1), 0.9)).InOklab()
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap3).
			Child(swatch().Bg(glow(1))).
			Child(swatch().Bg(warm)).
			Child(swatch().Bg(gk.HSLA(0.33, 0.5, 0.5, 1)))).
		Child(gk.Div().W(gk.Px(576)).H(gk.Px(240)).Child(plot))
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(nil, a.view)
		gk.OpenWindow(cx, "Paint", 640, 400, a.root)
	})
}
