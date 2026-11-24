// misuse_data: mistakes against the data protocol (docs/PROTOCOLS.md section 12) that must stop
// with a message naming the column or template, never draw a chart from a wrong column. Run
// offscreen with the diag library by gates/go-misuse.sh, one
// mode per case. A column is an exported field, named as the field is (Month, Desktop):
//
//	GK_NEG=data-ok      a line chart over the rows, drawn: correct (exit 0)
//	GK_NEG=data-column  .Y("Dekstop"), a column the rows do not have: must stop
//	GK_NEG=data-kind    .Y("Month"), a text column where the chart wants a number: must stop
//	GK_NEG=data-number  .YTickFormat("{} of {}"): the closure is handed one number: must stop
//	GK_NEG=data-brace   .TooltipValue("{:.1 visits"): a brace that is not closed: must stop
//	GK_NEG=data-row     .YTickFormat("{Month}"): a tick has no row to read a column from
//	GK_NEG=data-field   gk.NewData of rows with a slice field, which a table cannot carry
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
