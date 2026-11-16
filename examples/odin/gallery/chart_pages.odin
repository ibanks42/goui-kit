// The charts' gallery pages, the Odin twin of ref/src/bin/gallery/chart_pages.rs, chart for
// chart: the rows are the app's own structs, given as a data table (gk.data), and where the
// Rust page passes a closure over a row this page names a column ("month") or a template
// ("{:.1} visits"): docs/PROTOCOLS.md section 12. The bar chart's tooltip content is a row
// source over the rows, keyed by the row's index.
package gallery

import "core:fmt"
import gk "gpuikit:gk"

Month :: struct {
	month:   string,
	desktop: f64,
	mobile:  f64,
	color:   gk.Color,
}

Price :: struct {
	date:                   string,
	open, high, low, close: f64,
}

Share :: struct {
	browser: string,
	share:   f32,
	color:   gk.Color,
	inner:   f32,
	outer:   f32,
}

Score :: struct {
	dimension: string,
	alpha:     f64,
}

Flow :: struct {
	name:  string,
	color: gk.Color,
}

MONTHS := [?]Month {
	{"Jan", 186, 80, {theme = u16(gk.Theme_Color.Chart_1)}},
	{"Feb", 305, 200, {theme = u16(gk.Theme_Color.Chart_2)}},
	{"Mar", 237, 120, {theme = u16(gk.Theme_Color.Chart_3)}},
	{"Apr", 73, 190, {theme = u16(gk.Theme_Color.Chart_4)}},
	{"May", 209, 130, {theme = u16(gk.Theme_Color.Chart_5)}},
	{"Jun", 214, 140, {theme = u16(gk.Theme_Color.Chart_1)}},
}

PRICES := [?]Price {
	{"Mon", 100, 110, 95, 105},
	{"Tue", 105, 108, 98, 99},
	{"Wed", 99, 115, 97, 112},
	{"Thu", 112, 118, 106, 108},
	{"Fri", 108, 109, 96, 100},
	{"Sat", 100, 104, 92, 103},
	{"Sun", 103, 117, 101, 116},
}

SHARES := [?]Share {
	{"Chrome", 0.55, {theme = u16(gk.Theme_Color.Chart_1)}, 20, 70},
	{"Safari", 0.2, {theme = u16(gk.Theme_Color.Chart_2)}, 24, 67},
	{"Firefox", 0.12, {theme = u16(gk.Theme_Color.Chart_3)}, 28, 64},
	{"Edge", 0.08, {theme = u16(gk.Theme_Color.Chart_4)}, 32, 61},
	{"Other", 0.05, {theme = u16(gk.Theme_Color.Chart_5)}, 36, 58},
}

SCORES := [?]Score{{"Speed", 80}, {"Power", 65}, {"Range", 90}, {"Comfort", 55}, {"Price", 70}}

FLOWS := [?]Flow {
	{"Revenue", {theme = u16(gk.Theme_Color.Chart_1)}},
	{"Cost", {theme = u16(gk.Theme_Color.Chart_2)}},
	{"Profit", {theme = u16(gk.Theme_Color.Chart_3)}},
	{"Tax", {theme = u16(gk.Theme_Color.Chart_4)}},
	{"Net", {theme = u16(gk.Theme_Color.Chart_5)}},
}

Chart_Pages :: struct {
	tips: gk.Row_Source, // the bar chart's tooltip content, one row per month
}

CHART_PAGES := [?]Page {
	{"chart-line", chart_line},
	{"chart-line-more", chart_line_more},
	{"chart-area", chart_area},
	{"chart-bar", chart_bar},
	{"chart-candlestick", chart_candlestick},
	{"chart-pie", chart_pie},
	{"chart-pie-fn", chart_pie_fn},
	{"chart-radar", chart_radar},
	{"chart-sankey", chart_sankey},
	// the components family, docs/PROTOCOLS.md 12.16: lists per row
	{"comp6-sankey-labels", comp6_sankey_labels},
	{"comp6-bar-gradient", comp6_bar_gradient},
	{"comp6-bar-labels", comp6_bar_labels},
	{"chart-tooltip-line", chart_tooltip_line},
	{"chart-tooltip-bar", chart_tooltip_bar},
	{"chart-tooltip-pie", chart_tooltip_pie},
	{"plot-custom", plot_custom},
	{"generic-sidebar", generic_sidebar},
}

chart_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	a.charts.tips = gk.row_source(cx, a, len(MONTHS), chart_tip_row)
}

chart_tip_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	m := MONTHS[at.row]
	return gk.label(fmt.tprintf("%d sold in %s", int(m.desktop), m.month))
}

@(private = "file")
card :: proc(chart: gk.Element) -> gk.Element {
	return gk.div()->w(gk.px(360))->h(gk.px(220))->child(chart)
}

@(private = "file")
pair :: proc(a, b: gk.Element) -> gk.Element {
	return gk.div(.Flex, .Gap_4)->child(a)->child(b)
}


chart_line :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return pair(
		card(
			gk.line_chart(gk.data(MONTHS[:]))->id("line-a")->appear(false)->x("month")->y("desktop")->
			stroke(gk.theme(.Chart_1))->dot()->name("Desktop")->tick_margin(2)->y_axis(true)->y_tick_count(4)->
			y_tick_format("{:.0}k")->grid_dashed(false)->reference_line(150),
		),
		card(
			gk.line_chart(gk.data(MONTHS[:]))->id("line-b")->appear(false)->interactive(false)->x("month")->
			y("mobile")->stroke(gk.theme(.Chart_2))->linear()->x_axis(false)->grid(false)->y_domain(0, 300)->
			y_padding(20, 10)->y_axis(true)->y_axis_label_placement(.Inside),
		),
	)
}

chart_line_more :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return pair(
		card(
			gk.line_chart(gk.data(MONTHS[:]))->id("line-c")->appear(false)->appear_key("2025")->x("month")->
			y("desktop")->step_after()->x_tick_count(3)->grid_columns(3),
		),
		card(gk.line_chart(gk.data(MONTHS[:]))->id("line-d")->appear(false)->x("month")->y("mobile")->natural()->point_count(8)),
	)
}

chart_area :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return pair(
		card(
			gk.area_chart(gk.data(MONTHS[:]))->id("area-a")->appear(false)->x("month")->y("desktop")->
			stroke(gk.theme(.Chart_1))->fill(gk.hsla(0.6, 0.5, 0.5, 0.3))->y("mobile")->stroke(gk.theme(.Chart_2))->
			fill(gk.hsla(0.3, 0.5, 0.5, 0.3))->name("Visits")->y_axis(true)->y_tick_count(3)->
			y_tick_format("{:,.0}")->tick_margin(2),
		),
		card(
			gk.area_chart(gk.data(MONTHS[:]))->id("area-b")->appear(false)->interactive(false)->x("month")->
			y("desktop")->step_after()->linear()->natural()->x_axis(false)->grid(true)->grid_dashed(false)->
			grid_columns(4)->x_tick_count(2)->y_domain(0, 400)->point_count(7)->reference_line(200)->
			y_padding(0, 0)->y_axis(true)->y_axis_label_placement(.Inside)->appear_key("k"),
		),
	)
}

chart_bar :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return pair(
		card(
			gk.bar_chart(gk.data(MONTHS[:]))->id("bar-a")->appear(false)->band("month")->value("desktop")->
			fill("color")->label("desktop")->label_color("color")->name("Desktop")->tick_margin(1)->
			value_axis(true)->value_tick_count(4)->value_tick_format("{:.0}")->corner_radii(f32(4))->
			padding_inner(0.3)->padding_outer(0.2)->max_band_width(40)->min_length(2)->grid_dashed(false),
		),
		card(
			gk.bar_chart(gk.data(MONTHS[:]))->id("bar-b")->appear(false)->interactive(false)->band("month")->
			value("mobile")->alignment(.Left)->label_axis(true)->band_count(8)->band_tick_count(3)->grid(false)->
			value_axis(true)->value_axis_label_placement(.Inside)->appear_key("k"),
		),
	)
}

chart_candlestick :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return pair(
		card(
			gk.candlestick_chart(gk.data(PRICES[:]))->id("candle-a")->appear(false)->x("date")->open("open")->
			high("high")->low("low")->close("close")->bullish(gk.theme(.Chart_Bullish))->
			bearish(gk.theme(.Chart_Bearish))->tick_margin(2),
		),
		card(
			gk.candlestick_chart(gk.data(PRICES[:]))->id("candle-b")->appear(false)->interactive(false)->
			appear_key("k")->x("date")->open("open")->high("high")->low("low")->close("close")->
			body_width_ratio(0.4)->max_band_width(12)->x_axis(false)->grid(false),
		),
	)
}

chart_pie :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return pair(
		card(
			gk.pie_chart(gk.data(SHARES[:]))->id("pie-a")->appear(false)->value("share")->color("color")->
			inner_radius(30)->outer_radius(80)->pad_angle(0.02)->name("Share"),
		),
		card(
			gk.pie_chart(gk.data(SHARES[:]))->id("pie-b")->appear(false)->interactive(false)->appear_key("k")->
			value("share")->color("color")->label("browser")->label_line_color("color")->
			label_color(gk.theme(.Muted_Foreground))->label_gap(10)->inner_radius_ratio(0.5)->outer_radius(60),
		),
	)
}

chart_pie_fn :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return card(
		gk.pie_chart(gk.data(SHARES[:]))->id("pie-c")->appear(false)->value("share")->color("color")->
		inner_radius_fn("inner")->outer_radius_fn("outer"),
	)
}

chart_radar :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return pair(
		card(
			gk.radar_chart(gk.data(SCORES[:]))->id("radar-a")->appear(false)->value("alpha")->label("dimension")->
			stroke(gk.theme(.Chart_1))->fill(gk.hsla(0.6, 0.5, 0.5, 0.3))->name("Alpha")->dot(),
		),
		card(
			gk.radar_chart(gk.data(SCORES[:]))->id("radar-b")->appear(false)->interactive(false)->appear_key("k")->
			value("alpha")->label("dimension")->label_color(gk.theme(.Chart_2))->label_gap(4)->max_value(100)->
			outer_radius(70)->grid(true)->grid_levels(3),
		),
	)
}

chart_sankey :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	l1 := [?]gk.Sankey_Link{gk.sankey_link(0, 1, 60), gk.sankey_link(0, 2, 40), gk.sankey_link(2, 3, 10), gk.sankey_link(2, 4, 30)}
	l2 := [?]gk.Sankey_Link{gk.sankey_link(0, 1, 60), gk.sankey_link(0, 2, 40), gk.sankey_link(2, 3, 10), gk.sankey_link(2, 4, 30)}
	return pair(
		card(
			gk.sankey_chart(gk.data(FLOWS[:]), l1[:])->id("sankey-a")->appear(false)->node_color("color")->
			node_label("name")->value_label("{}"),
		),
		card(
			gk.sankey_chart(gk.data(FLOWS[:]), l2[:])->id("sankey-b")->appear(false)->interactive(false)->
			appear_key("k")->node_label("name")->node_width(14)->node_padding(8)->node_align(.Justify)->
			iterations(8)->value_scale(.Sqrt)->node_corner_radius(3)->link_opacity(0.5)->min_link_width(2)->
			label_gap(4),
		),
	)
}

chart_tooltip_line :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return card(
		gk.line_chart(gk.data(MONTHS[:]))->id("tip-line")->appear(false)->x("month")->y("desktop")->
		name("Desktop")->tooltip_title("month")->tooltip_value("{:.1} visits")->tooltip_value_color("color"),
	)
}

chart_tooltip_bar :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return card(
		gk.bar_chart(gk.data(MONTHS[:]))->id("tip-bar")->appear(false)->band("month")->value("desktop")->
		tooltip_content(a.charts.tips),
	)
}

chart_tooltip_pie :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return card(
		gk.pie_chart(gk.data(SHARES[:]))->id("tip-pie")->appear(false)->value("share")->color("color")->
		tooltip_name("browser")->tooltip_value("{} ({1:.1}%)"),
	)
}

// One point of the described plot, in pixels of the plot's bounds: the Rust page computes the
// same numbers in its closures.
Pt :: struct {
	month:            string,
	x, x_left:        f32,
	top, line:        f32,
	base:             f32,
	angle, radius:    f32,
	color:            gk.Color,
}

plot_custom :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	pts: [len(MONTHS)]Pt
	ticks: [len(MONTHS)]gk.Axis_Text
	for m, i in MONTHS {
		pts[i] = {
			month  = m.month,
			x      = 30 + 55 * f32(i),
			x_left = 20 + 55 * f32(i),
			top    = 200 - f32(m.desktop) * 0.5,
			line   = 200 - f32(m.mobile) * 0.5,
			base   = 200,
			angle  = f32(i),
			radius = 20 + f32(m.mobile) * 0.125,
			color  = m.color,
		}
		ticks[i] = gk.axis_text(m.month, pts[i].x, gk.theme(.Muted_Foreground))
	}
	rows := gk.data(pts[:])
	return card(
		gk.plot(
			gk.plot_axis()->x(200)->x_label(ticks[:])->stroke(gk.theme(.Border)),
			gk.grid()->y({50, 100, 150})->stroke(gk.theme(.Border))->dash_array({4, 2}),
			gk.bar_shape()->data(rows)->band_width(20)->cross("x_left")->base("base")->value("top")->fill("color")->
			corner_radii(f32(2)),
			gk.area_shape()->data(rows)->x("x")->y0(200)->y1("line")->fill(gk.hsla(0.3, 0.5, 0.5, 0.2))->
			stroke(gk.theme(.Chart_3))->curve(.Linear),
			gk.line_shape()->data(rows)->x("x")->y("line")->stroke(gk.theme(.Chart_2))->stroke_width(2)->
			curve(.Natural)->dot()->dot_size(6)->dot_fill(gk.theme(.Chart_2))->dot_stroke(gk.theme(.Background)),
			gk.radial_line_shape()->data(rows)->angle("angle")->radius("radius")->closed()->
			fill(gk.hsla(0.6, 0.5, 0.5, 0.2))->stroke(gk.theme(.Chart_1))->stroke_width(1)->dot()->dot_size(4)->
			dot_fill(gk.theme(.Chart_1))->dot_stroke(gk.theme(.Chart_1)),
		),
	)
}

// A generic element over its children's type (Sidebar<E: SidebarItem>), instantiated for
// SidebarMenu (`[instantiate] types` in api/overrides.toml).
generic_sidebar :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	more := [?]gk.Sidebar_Menu{gk.sidebar_menu()->child(gk.sidebar_menu_item("Settings"))}
	return gk.div()->w(gk.px(240))->h(gk.px(300))->child(
		gk.sidebar("sb")->side(.Left)->collapsible(false)->collapsed(false)->header(gk.label("Header"))->
		footer(gk.label("Footer"))->
		child(gk.sidebar_menu()->child(gk.sidebar_menu_item("Home"))->child(gk.sidebar_menu_item("Inbox")->active(true)))->
		children(more[:]),
	)
}

// --- the components family, docs/PROTOCOLS.md 12.16: closures over a row that return a
// list, answered from the list the app gives for each row (row i's at index i). The Rust twin's
// closures compute the same lists from the row (ref/src/bin/gallery/chart_pages.rs).

// Each node's two labels: its name, and "<name> flow" smaller and muted.
comp6_sankey_labels :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	links := [?]gk.Sankey_Link{gk.sankey_link(0, 1, 60), gk.sankey_link(0, 2, 40), gk.sankey_link(2, 3, 10), gk.sankey_link(2, 4, 30)}
	labels := make([][]gk.Sankey_Label, len(FLOWS), context.temp_allocator)
	for f, i in FLOWS {
		l := make([]gk.Sankey_Label, 2, context.temp_allocator)
		l[0] = gk.sankey_label(f.name)->font_size(12)
		l[1] = gk.sankey_label(fmt.tprintf("%s flow", f.name))->color(gk.theme(.Muted_Foreground))->font_size(10)
		labels[i] = l
	}
	return card(
		gk.sankey_chart(gk.data(FLOWS[:]), links[:])->id("sankey-c6")->appear(false)->node_color("color")->labels(labels),
	)
}

// Each bar a gradient from its colour at the base to a pale blue at the tip.
comp6_bar_gradient :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	stops := make([][2]gk.Linear_Color_Stop, len(MONTHS), context.temp_allocator)
	for m, i in MONTHS {
		stops[i] = {{color = m.color, percentage = 0}, {color = gk.hsla(0.6, 0.5, 0.9, 1), percentage = 1}}
	}
	return card(
		gk.bar_chart(gk.data(MONTHS[:]))->id("bar-c6")->appear(false)->band("month")->value("desktop")->fill_gradient(stops),
	)
}

// A plot's bars, each with a label above it: the value, at the bar's top (the app computes where,
// as it computes the bars' positions, docs/PROTOCOLS.md 12.6).
comp6_bar_labels :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	pts: [len(MONTHS)]Pt
	labels := make([][]gk.Text, len(MONTHS), context.temp_allocator)
	for m, i in MONTHS {
		pts[i] = {month = m.month, x = 30 + 55 * f32(i), x_left = 20 + 55 * f32(i), top = 200 - f32(m.desktop) * 0.5, base = 200, color = m.color}
		l := make([]gk.Text, 1, context.temp_allocator)
		l[0] = gk.base_text(fmt.tprintf("%.0f", m.desktop), {pts[i].x, pts[i].top - 14}, gk.theme(.Foreground))
		labels[i] = l
	}
	rows := gk.data(pts[:])
	return card(
		gk.plot(
			gk.plot_axis()->x(200)->stroke(gk.theme(.Border)),
			gk.bar_shape()->data(rows)->band_width(20)->cross("x_left")->base("base")->value("top")->fill("color")->label(labels),
		),
	)
}
