// Paint example: showcases linear gradients (sRGB and Oklab) on divs and custom
// described plots with grid lines, filled polygons, stroked polylines, and quads.
package main

import (
	"log"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/chart"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type App struct{ root gk.View }

func cyan(a float32) gk.Color   { return gk.HSLA(0.5, 0.75, 0.55, a) }
func violet(a float32) gk.Color { return gk.HSLA(0.75, 0.6, 0.65, a) }

func glow(a float32) gk.LinearGradient {
	return gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan(a), 0), gk.NewLinearColorStop(violet(a), 1))
}

func swatch() gk.DivEl { return gk.Div().W(gk.Px(160)).H(gk.Px(48)).Rounded(gk.Px(8)) }

// view builds three gradient/solid swatches and a described chart.Plot.
// Because plot parts travel on the tape as data (N_PLOT, N_PLOT_PATH, N_PLOT_QUAD),
// Rust paints the entire scene natively without crossing back into Go during paint.
func (a *App) view(cx *gk.Cx) gk.Element {
	pts := make([][2]float32, 49)
	for i := range pts {
		pts[i] = [2]float32{float32(i * 12), float32(40 + (i*37)%150)}
	}
	area := append(append([][2]float32{{0, 120}}, pts...), [2]float32{576, 120})
	white := gk.HSLA(0, 0, 1, 1)
	// Linear gradients accept symbolic theme.* stops and optional Oklab interpolation.
	bar := gk.NewLinearGradient(0, gk.NewLinearColorStop(theme.Primary, 0), gk.NewLinearColorStop(violet(1), 1)).InOklab()
	plot := chart.Plot(
		chart.NewGrid().X([]float32{144, 288, 432}).Y([]float32{60, 120, 180}).Stroke(theme.Border),
		chart.NewPlotArea(area, gk.HSLA(0.6, 0.8, 0.7, 0.12)),
		chart.NewPlotLine(pts, 7, glow(0.12)),
		chart.NewPlotLine(pts, 3, glow(0.25)),
		chart.NewPlotLine(pts, 1.5, glow(1)),
		chart.NewPlotQuad(480, 10, 80, 36, bar, chart.QuadStyle{CornerRadius: 6, BorderWidth: 2, BorderColor: white}),
		chart.NewPlotQuad(283, 103, 10, 10, white, chart.QuadStyle{CornerRadius: 5}),
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
	if err := gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(nil, a.view)
		gk.OpenWindow(cx, "Paint", 640, 400, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}
