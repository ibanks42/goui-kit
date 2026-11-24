// The charts' gallery pages, the Go twin of
// ref/src/bin/gallery/chart_pages.rs, chart for chart: the rows are the app's own structs, given
// as a data table (gk.NewData, a column per exported field), and where the Rust page passes a
// closure over a row this page names a column ("Month") or a template ("{:.1} visits"):
// docs/PROTOCOLS.md section 12. The bar chart's tooltip content is a row source over the rows,
// keyed by the row's index.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	gkchart "gpuikit.dev/gk/chart"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type chartMonth struct {
	Month   string
	Desktop float64
	Mobile  float64
	Color   gk.Color
}

type chartPrice struct {
	Date                   string
	Open, High, Low, Close float64
}

type chartShare struct {
	Browser string
	Share   float32
	Color   gk.Color
	Inner   float32
	Outer   float32
}

type chartScore struct {
	Dimension string
	Alpha     float64
}

type chartFlow struct {
	Name  string
	Color gk.Color
}

var chartMonths = []chartMonth{
	{"Jan", 186, 80, theme.Chart1},
	{"Feb", 305, 200, theme.Chart2},
	{"Mar", 237, 120, theme.Chart3},
	{"Apr", 73, 190, theme.Chart4},
	{"May", 209, 130, theme.Chart5},
	{"Jun", 214, 140, theme.Chart1},
}

var chartPrices = []chartPrice{
	{"Mon", 100, 110, 95, 105},
	{"Tue", 105, 108, 98, 99},
	{"Wed", 99, 115, 97, 112},
	{"Thu", 112, 118, 106, 108},
	{"Fri", 108, 109, 96, 100},
	{"Sat", 100, 104, 92, 103},
	{"Sun", 103, 117, 101, 116},
}

var chartShares = []chartShare{
	{"Chrome", 0.55, theme.Chart1, 20, 70},
	{"Safari", 0.2, theme.Chart2, 24, 67},
	{"Firefox", 0.12, theme.Chart3, 28, 64},
	{"Edge", 0.08, theme.Chart4, 32, 61},
	{"Other", 0.05, theme.Chart5, 36, 58},
}

var chartScores = []chartScore{{"Speed", 80}, {"Power", 65}, {"Range", 90}, {"Comfort", 55}, {"Price", 70}}

var chartFlows = []chartFlow{
	{"Revenue", theme.Chart1},
	{"Cost", theme.Chart2},
	{"Profit", theme.Chart3},
	{"Tax", theme.Chart4},
	{"Net", theme.Chart5},
}

type chartPages struct {
	tips gk.RowSource // the bar chart's tooltip content, one row per month
}

// The comp6-* pages (comp6-sankey-labels, comp6-bar-gradient,
// comp6-bar-labels) wait for bindings: chart_pending.go.
var chartPageTable = []Page{
	{"chart-line", chartLine},
	{"chart-line-more", chartLineMore},
	{"chart-area", chartArea},
	{"chart-bar", chartBar},
	{"chart-candlestick", chartCandlestick},
	{"chart-pie", chartPie},
	{"chart-pie-fn", chartPieFn},
	{"chart-radar", chartRadar},
	{"chart-sankey", chartSankey},
	{"chart-tooltip-line", chartTooltipLine},
	{"chart-tooltip-bar", chartTooltipBar},
	{"chart-tooltip-pie", chartTooltipPie},
	{"plot-custom", plotCustom},
	{"generic-sidebar", genericSidebar},
	// the components family, docs/PROTOCOLS.md 12.16: closures over a row that return a list,
	// answered from the list the app gives for each row (row i's at index i)
	{"comp6-sankey-labels", comp6SankeyLabels},
	{"comp6-bar-gradient", comp6BarGradient},
	{"comp6-bar-labels", comp6BarLabels},
}

func chartPagesInit(a *App, cx *gk.Cx) {
	a.charts.tips = gk.NewRowSource(cx, len(chartMonths), func(cx *gk.Cx, at gk.RowAt) gk.Element {
		m := chartMonths[at.Row]
		return kit.Label(fmt.Sprintf("%d sold in %s", int(m.Desktop), m.Month))
	})
}

func chartCard(chart gk.Element) gk.Element {
	return gk.Div().W(gk.Px(360)).H(gk.Px(220)).Child(chart)
}

func chartPair(a, b gk.Element) gk.Element {
	return gk.Div(s.Flex, s.Gap4).Child(a).Child(b)
}

func chartLine(a *App, cx *gk.Cx) gk.Element {
	return chartPair(
		chartCard(gkchart.LineChart(gkchart.NewData(chartMonths)).ID("line-a").Appear(false).X("Month").Y("Desktop").
			Stroke(theme.Chart1).Dot().Name("Desktop").TickMargin(2).YAxis(true).YTickCount(4).
			YTickFormat("{:.0}k").GridDashed(false).ReferenceLine(150)),
		chartCard(gkchart.LineChart(gkchart.NewData(chartMonths)).ID("line-b").Appear(false).Interactive(false).X("Month").
			Y("Mobile").Stroke(theme.Chart2).Linear().XAxis(false).Grid(false).YDomain(0, 300).
			YPadding(20, 10).YAxis(true).YAxisLabelPlacement(gkchart.AxisLabelPlacementInside)),
	)
}

func chartLineMore(a *App, cx *gk.Cx) gk.Element {
	return chartPair(
		chartCard(gkchart.LineChart(gkchart.NewData(chartMonths)).ID("line-c").Appear(false).AppearKey("2025").X("Month").
			Y("Desktop").StepAfter().XTickCount(3).GridColumns(3)),
		chartCard(gkchart.LineChart(gkchart.NewData(chartMonths)).ID("line-d").Appear(false).X("Month").Y("Mobile").Natural().PointCount(8)),
	)
}

func chartArea(a *App, cx *gk.Cx) gk.Element {
	return chartPair(
		chartCard(gkchart.AreaChart(gkchart.NewData(chartMonths)).ID("area-a").Appear(false).X("Month").Y("Desktop").
			Stroke(theme.Chart1).Fill(gk.HSLA(0.6, 0.5, 0.5, 0.3)).Y("Mobile").Stroke(theme.Chart2).
			Fill(gk.HSLA(0.3, 0.5, 0.5, 0.3)).Name("Visits").YAxis(true).YTickCount(3).
			YTickFormat("{:,.0}").TickMargin(2)),
		chartCard(gkchart.AreaChart(gkchart.NewData(chartMonths)).ID("area-b").Appear(false).Interactive(false).X("Month").
			Y("Desktop").StepAfter().Linear().Natural().XAxis(false).Grid(true).GridDashed(false).
			GridColumns(4).XTickCount(2).YDomain(0, 400).PointCount(7).ReferenceLine(200).
			YPadding(0, 0).YAxis(true).YAxisLabelPlacement(gkchart.AxisLabelPlacementInside).AppearKey("k")),
	)
}

func chartBar(a *App, cx *gk.Cx) gk.Element {
	return chartPair(
		chartCard(gkchart.BarChart(gkchart.NewData(chartMonths)).ID("bar-a").Appear(false).Band("Month").Value("Desktop").
			Fill("Color").Label("Desktop").LabelColor("Color").Name("Desktop").TickMargin(1).
			ValueAxis(true).ValueTickCount(4).ValueTickFormat("{:.0}").CornerRadii(float32(4)).
			PaddingInner(0.3).PaddingOuter(0.2).MaxBandWidth(40).MinLength(2).GridDashed(false)),
		chartCard(gkchart.BarChart(gkchart.NewData(chartMonths)).ID("bar-b").Appear(false).Interactive(false).Band("Month").
			Value("Mobile").Alignment(gkchart.BarAlignmentLeft).LabelAxis(true).BandCount(8).BandTickCount(3).Grid(false).
			ValueAxis(true).ValueAxisLabelPlacement(gkchart.AxisLabelPlacementInside).AppearKey("k")),
	)
}

func chartCandlestick(a *App, cx *gk.Cx) gk.Element {
	return chartPair(
		chartCard(gkchart.CandlestickChart(gkchart.NewData(chartPrices)).ID("candle-a").Appear(false).X("Date").Open("Open").
			High("High").Low("Low").Close("Close").Bullish(theme.ChartBullish).
			Bearish(theme.ChartBearish).TickMargin(2)),
		chartCard(gkchart.CandlestickChart(gkchart.NewData(chartPrices)).ID("candle-b").Appear(false).Interactive(false).
			AppearKey("k").X("Date").Open("Open").High("High").Low("Low").Close("Close").
			BodyWidthRatio(0.4).MaxBandWidth(12).XAxis(false).Grid(false)),
	)
}

func chartPie(a *App, cx *gk.Cx) gk.Element {
	return chartPair(
		chartCard(gkchart.PieChart(gkchart.NewData(chartShares)).ID("pie-a").Appear(false).Value("Share").Color("Color").
			InnerRadius(30).OuterRadius(80).PadAngle(0.02).Name("Share")),
		chartCard(gkchart.PieChart(gkchart.NewData(chartShares)).ID("pie-b").Appear(false).Interactive(false).AppearKey("k").
			Value("Share").Color("Color").Label("Browser").LabelLineColor("Color").
			LabelColor(theme.MutedForeground).LabelGap(10).InnerRadiusRatio(0.5).OuterRadius(60)),
	)
}

func chartPieFn(a *App, cx *gk.Cx) gk.Element {
	return chartCard(gkchart.PieChart(gkchart.NewData(chartShares)).ID("pie-c").Appear(false).Value("Share").Color("Color").
		InnerRadiusFn("Inner").OuterRadiusFn("Outer"))
}

func chartRadar(a *App, cx *gk.Cx) gk.Element {
	return chartPair(
		chartCard(gkchart.RadarChart(gkchart.NewData(chartScores)).ID("radar-a").Appear(false).Value("Alpha").Label("Dimension").
			Stroke(theme.Chart1).Fill(gk.HSLA(0.6, 0.5, 0.5, 0.3)).Name("Alpha").Dot()),
		chartCard(gkchart.RadarChart(gkchart.NewData(chartScores)).ID("radar-b").Appear(false).Interactive(false).AppearKey("k").
			Value("Alpha").Label("Dimension").LabelColor(theme.Chart2).LabelGap(4).MaxValue(100).
			OuterRadius(70).Grid(true).GridLevels(3)),
	)
}

func chartLinks() []gkchart.SankeyLink {
	return []gkchart.SankeyLink{gkchart.NewSankeyLink(0, 1, 60), gkchart.NewSankeyLink(0, 2, 40), gkchart.NewSankeyLink(2, 3, 10), gkchart.NewSankeyLink(2, 4, 30)}
}

func chartSankey(a *App, cx *gk.Cx) gk.Element {
	l1 := chartLinks()
	l2 := chartLinks()
	return chartPair(
		chartCard(gkchart.SankeyChart(gkchart.NewData(chartFlows), l1).ID("sankey-a").Appear(false).NodeColor("Color").
			NodeLabel("Name").ValueLabel("{}")),
		chartCard(gkchart.SankeyChart(gkchart.NewData(chartFlows), l2).ID("sankey-b").Appear(false).Interactive(false).
			AppearKey("k").NodeLabel("Name").NodeWidth(14).NodePadding(8).NodeAlign(gkchart.SankeyAlignJustify).
			Iterations(8).ValueScale(gkchart.SankeyValueScaleSqrt).NodeCornerRadius(3).LinkOpacity(0.5).MinLinkWidth(2).
			LabelGap(4)),
	)
}

func chartTooltipLine(a *App, cx *gk.Cx) gk.Element {
	return chartCard(gkchart.LineChart(gkchart.NewData(chartMonths)).ID("tip-line").Appear(false).X("Month").Y("Desktop").
		Name("Desktop").TooltipTitle("Month").TooltipValue("{:.1} visits").TooltipValueColor("Color"))
}

func chartTooltipBar(a *App, cx *gk.Cx) gk.Element {
	return chartCard(gkchart.BarChart(gkchart.NewData(chartMonths)).ID("tip-bar").Appear(false).Band("Month").Value("Desktop").
		TooltipContent(a.charts.tips))
}

func chartTooltipPie(a *App, cx *gk.Cx) gk.Element {
	return chartCard(gkchart.PieChart(gkchart.NewData(chartShares)).ID("tip-pie").Appear(false).Value("Share").Color("Color").
		TooltipName("Browser").TooltipValue("{} ({1:.1}%)"))
}

// chartPt is one point of the described plot, in pixels of the plot's bounds: the Rust page
// computes the same numbers in its closures.
type chartPt struct {
	Month         string
	X, XLeft      float32
	Top, Line     float32
	Base          float32
	Angle, Radius float32
	Color         gk.Color
}

func plotCustom(a *App, cx *gk.Cx) gk.Element {
	pts := make([]chartPt, len(chartMonths))
	ticks := make([]gkchart.AxisText, len(chartMonths))
	for i, m := range chartMonths {
		pts[i] = chartPt{
			Month:  m.Month,
			X:      30 + 55*float32(i),
			XLeft:  20 + 55*float32(i),
			Top:    200 - float32(m.Desktop)*0.5,
			Line:   200 - float32(m.Mobile)*0.5,
			Base:   200,
			Angle:  float32(i),
			Radius: 20 + float32(m.Mobile)*0.125,
			Color:  m.Color,
		}
		ticks[i] = gkchart.NewAxisText(m.Month, pts[i].X, theme.MutedForeground)
	}
	rows := gkchart.NewData(pts)
	return chartCard(gkchart.Plot(
		gkchart.NewPlotAxis().X(200).XLabel(ticks).Stroke(theme.Border),
		gkchart.NewGrid().Y([]float32{50, 100, 150}).Stroke(theme.Border).DashArray([]float32{4, 2}),
		gkchart.NewBarShape().Data(rows).BandWidth(20).Cross("XLeft").Base("Base").Value("Top").Fill("Color").
			CornerRadii(float32(2)),
		gkchart.NewAreaShape().Data(rows).X("X").Y0(200).Y1("Line").Fill(gk.HSLA(0.3, 0.5, 0.5, 0.2)).
			Stroke(theme.Chart3).Curve(gkchart.CurveLinear),
		gkchart.NewLineShape().Data(rows).X("X").Y("Line").Stroke(theme.Chart2).StrokeWidth(2).
			Curve(gkchart.CurveNatural).Dot().DotSize(6).DotFill(theme.Chart2).DotStroke(theme.Background),
		gkchart.NewRadialLineShape().Data(rows).Angle("Angle").Radius("Radius").Closed().
			Fill(gk.HSLA(0.6, 0.5, 0.5, 0.2)).Stroke(theme.Chart1).StrokeWidth(1).Dot().DotSize(4).
			DotFill(theme.Chart1).DotStroke(theme.Chart1),
	))
}

// genericSidebar is a generic element over its children's type (Sidebar<E: SidebarItem>),
// instantiated for SidebarMenu (`[instantiate] types` in api/overrides.toml).
func genericSidebar(a *App, cx *gk.Cx) gk.Element {
	more := []kit.SidebarMenu{kit.NewSidebarMenu().Child(kit.NewSidebarMenuItem("Settings"))}
	return gk.Div().W(gk.Px(240)).H(gk.Px(300)).Child(
		kit.Sidebar("sb").Side(gk.SideLeft).Collapsible(false).Collapsed(false).Header(kit.Label("Header")).
			Footer(kit.Label("Footer")).
			Child(kit.NewSidebarMenu().Child(kit.NewSidebarMenuItem("Home")).Child(kit.NewSidebarMenuItem("Inbox").Active(true))).
			Children(more),
	)
}

// comp6SankeyLabels: each node's two labels, its name and "<name> flow" smaller and muted.
func comp6SankeyLabels(a *App, cx *gk.Cx) gk.Element {
	labels := make([][]gkchart.SankeyLabel, len(chartFlows))
	for i, f := range chartFlows {
		labels[i] = []gkchart.SankeyLabel{
			gkchart.NewSankeyLabel(f.Name).FontSize(12),
			gkchart.NewSankeyLabel(fmt.Sprintf("%s flow", f.Name)).Color(theme.MutedForeground).FontSize(10),
		}
	}
	return chartCard(gkchart.SankeyChart(gkchart.NewData(chartFlows), chartLinks()).ID("sankey-c6").Appear(false).
		NodeColor("Color").Labels(labels))
}

// comp6BarGradient: each bar a gradient from its color at the base to a pale blue at the tip.
func comp6BarGradient(a *App, cx *gk.Cx) gk.Element {
	stops := make([][2]gk.LinearColorStop, len(chartMonths))
	for i, m := range chartMonths {
		stops[i] = [2]gk.LinearColorStop{gk.NewLinearColorStop(m.Color, 0), gk.NewLinearColorStop(gk.HSLA(0.6, 0.5, 0.9, 1), 1)}
	}
	return chartCard(gkchart.BarChart(gkchart.NewData(chartMonths)).ID("bar-c6").Appear(false).Band("Month").Value("Desktop").
		FillGradient(stops))
}

// comp6BarLabels: a plot's bars, each with a label above it at the bar's top (the app computes
// where, as it computes the bars' positions, docs/PROTOCOLS.md 12.6).
func comp6BarLabels(a *App, cx *gk.Cx) gk.Element {
	pts := make([]chartPt, len(chartMonths))
	labels := make([][]gkchart.Text, len(chartMonths))
	for i, m := range chartMonths {
		pts[i] = chartPt{Month: m.Month, X: 30 + 55*float32(i), XLeft: 20 + 55*float32(i), Top: 200 - float32(m.Desktop)*0.5, Base: 200, Color: m.Color}
		labels[i] = []gkchart.Text{gkchart.NewBaseText(fmt.Sprintf("%.0f", m.Desktop), gk.GpuiPoint{X: pts[i].X, Y: pts[i].Top - 14}, theme.Foreground)}
	}
	rows := gkchart.NewData(pts)
	return chartCard(gkchart.Plot(
		gkchart.NewPlotAxis().X(200).Stroke(theme.Border),
		gkchart.NewBarShape().Data(rows).BandWidth(20).Cross("XLeft").Base("Base").Value("Top").Fill("Color").Label(labels),
	))
}
