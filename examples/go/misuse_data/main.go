// Runtime misuse cases for chart data tables and format templates (GK_NEG):
// unknown column names, type mismatches, bad template placeholders, and unsupported struct fields.
package main

import (
	"fmt"
	"log"
	"os"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/chart"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type month struct {
	Month   string
	Desktop float64
}

type bad struct {
	Month string
	Parts []int
}

var rows = []month{{"Jan", 186}, {"Feb", 305}, {"Mar", 237}}

type App struct {
	mode  string
	root  gk.View
	drawn int
}

func (a *App) view(cx *gk.Cx) gk.Element {
	a.drawn++
	c := chart.LineChart(chart.NewData(rows)).Appear(false).X("Month")
	switch a.mode {
	case "data-ok":
		c = c.Y("Desktop").YTickFormat("{:.0}").TooltipValue("{:.1} of {Month}")
	case "data-column":
		c = c.Y("Dekstop")
	case "data-kind":
		c = c.Y("Month")
	case "data-number":
		c = c.Y("Desktop").YTickFormat("{} of {}")
	case "data-brace":
		c = c.Y("Desktop").TooltipValue("{:.1 visits")
	case "data-row":
		c = c.Y("Desktop").YTickFormat("{Month}")
	case "data-field":
		c = chart.LineChart(chart.NewData([]bad{{"Jan", nil}})).X("Month")
	}
	return gk.Div(s.SizeFull).Bg(theme.Background).Child(gk.Div().W(gk.Px(300)).H(gk.Px(200)).Child(c))
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	if err := gk.RunHeadless("size 400 300\nframe\nframe\n", func(cx *gk.Cx) {
		a.root = gk.NewView(nil, a.view)
		gk.OpenWindow(cx, "misuse_data", 400, 300, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gk-neg done mode=%s drawn=%d\n", a.mode, a.drawn)
}
