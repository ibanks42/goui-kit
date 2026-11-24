package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/chart"
)

// Visit holds sample row fields for pie arc layout tests, including an optional Gap field.
type Visit struct {
	Month  string
	Visits float64
	Gap    gk.Option[float64]
}

// Fruit holds two numeric series columns for stacked layout tests.
type Fruit struct {
	Month  string
	Apples float64
	Pears  float64
}

func init() {
	modes["pie"] = pie
	modes["stack"] = stack
}

// pie proves PieArcs preserves row order and values across a full 2*pi sweep
// (6.2832 rad for 10, 30, 20) and skips None rows in optional columns so the
// two present rows span exactly the configured 0..pi half-circle (end=3.142).
func pie(a *App, cx *gk.Cx) {
	rows := []Visit{{"jan", 10, gk.Some(1.0)}, {"feb", 30, gk.Option[float64]{}}, {"mar", 20, gk.Some(2.0)}}
	p := chart.NewPie(cx).Value("Visits").Build()
	arcs := p.Arcs(cx, chart.NewData(rows))
	var total float32
	for _, x := range arcs {
		total += x.EndAngle - x.StartAngle
	}
	fmt.Printf("overlay-charts pie arcs=%d rows=%d,%d,%d values=%v,%v,%v\n", len(arcs), arcs[0].Row, arcs[1].Row, arcs[2].Row,
		arcs[0].Value, arcs[1].Value, arcs[2].Value)
	fmt.Printf("overlay-charts pie sweep=%.4f order=%d,%d,%d\n", total, arcs[0].Index, arcs[1].Index, arcs[2].Index)

	g := chart.NewPie(cx).Value("Gap").StartAngle(0).EndAngle(3.14159265).PadAngle(0).Build()
	ga := g.Arcs(cx, chart.NewData(rows))
	end := float32(0)
	if len(ga) > 0 {
		end = ga[len(ga)-1].EndAngle
	}
	fmt.Printf("overlay-charts pie gap arcs=%d end=%.3f\n", len(ga), end)
	gk.Release(cx, p)
	gk.Release(cx, g)
	fmt.Println("overlay-charts done mode=pie")
}

// stack proves StackSeries accumulates each key atop prior keys (Pears stacks on
// Apples: jan 1..3, feb 3..7) and respects an explicit Value column override
// (stacking Apples twice yields feb 3..6).
func stack(a *App, cx *gk.Cx) {
	rows := []Fruit{{"jan", 1, 2}, {"feb", 3, 4}}
	keys := []string{"Apples", "Pears"}
	st := chart.NewStack(cx).Data(chart.NewData(rows)).Keys(keys).Value("").Build()
	series := st.Series(cx)
	fmt.Printf("overlay-charts stack series=%d keys=%s,%s\n", len(series), series[0].Key, series[1].Key)
	p := series[1].Points
	fmt.Printf("overlay-charts stack pears jan=%v..%v feb=%v..%v rows=%d,%d\n", p[0].Y0, p[0].Y1, p[1].Y0, p[1].Y1, p[0].Row, p[1].Row)

	t := chart.NewStack(cx).Data(chart.NewData(rows)).Keys(keys).Value("Apples").Build()
	q := t.Series(cx)[1].Points
	fmt.Printf("overlay-charts stack named feb=%v..%v\n", q[1].Y0, q[1].Y1)
	fmt.Println("overlay-charts done mode=stack")
}
