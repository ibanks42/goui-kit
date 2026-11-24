//! Chart, low-level plot-shape, and generic `Sidebar<SidebarMenu>` gallery pages.
//!
//! Covers `LineChart`, `AreaChart`, `BarChart`, `CandlestickChart`, `PieChart`,
//! `RadarChart`, `SankeyChart`, interactive chart tooltips, custom `Plot`
//! shapes (`Shapes`, `LabeledBars`), and `generic-sidebar`.

use gpui_kit::base::plot::shape::{
    Area, Bar, BarAlignment, Line, RadialLine, SankeyAlign, SankeyLink, SankeyValueScale,
};
use gpui_kit::base::plot::{AxisLabelPlacement, AxisText, Curve, Grid, Plot, PlotAxis, PlotElement};
use gpui_kit::component::ActiveTheme;
use gpui_kit::component::chart::{
    AreaChart, BarChart, CandlestickChart, LineChart, PieChart, RadarChart, SankeyChart, SankeyLabel,
};
use gpui_kit::base::Side;
use gpui_kit::component::label::Label;
use gpui_kit::component::sidebar::{Sidebar, SidebarMenu, SidebarMenuItem};
use gpui_kit::{
    AnyElement, App, Bounds, Hsla, IntoElement, ParentElement, Pixels, SharedString, Styled, Window,
    div, hsla, px,
};

/// Monthly desktop/mobile traffic sample shared by line, area, and bar pages.
#[derive(Clone)]
struct Month {
    month: SharedString,
    desktop: f64,
    mobile: f64,
    color: Hsla,
}

/// Daily OHLC price datum for `chart-candlestick`.
#[derive(Clone)]
struct Price {
    date: SharedString,
    open: f64,
    high: f64,
    low: f64,
    close: f64,
}

/// Browser market-share datum with per-slice inner/outer radii for pie pages.
#[derive(Clone)]
struct Share {
    browser: SharedString,
    share: f32,
    color: Hsla,
    inner: f32,
    outer: f32,
}

/// Single radar axis datum for `chart-radar`.
#[derive(Clone)]
struct Score {
    dimension: SharedString,
    alpha: f64,
}

/// Sankey node metadata for `chart-sankey` and `comp6-sankey-labels`.
#[derive(Clone)]
struct Flow {
    name: SharedString,
    color: Hsla,
}

fn months(cx: &App) -> Vec<Month> {
    let t = &cx.theme().colors;
    let c = [t.chart_1, t.chart_2, t.chart_3, t.chart_4, t.chart_5, t.chart_1];
    [("Jan", 186., 80.), ("Feb", 305., 200.), ("Mar", 237., 120.), ("Apr", 73., 190.), ("May", 209., 130.), ("Jun", 214., 140.)]
        .iter()
        .zip(c)
        .map(|(&(m, d, mo), color)| Month { month: m.into(), desktop: d, mobile: mo, color })
        .collect()
}

fn prices() -> Vec<Price> {
    [
        ("Mon", 100., 110., 95., 105.),
        ("Tue", 105., 108., 98., 99.),
        ("Wed", 99., 115., 97., 112.),
        ("Thu", 112., 118., 106., 108.),
        ("Fri", 108., 109., 96., 100.),
        ("Sat", 100., 104., 92., 103.),
        ("Sun", 103., 117., 101., 116.),
    ]
    .iter()
    .map(|&(d, o, h, l, c)| Price { date: d.into(), open: o, high: h, low: l, close: c })
    .collect()
}

fn shares(cx: &App) -> Vec<Share> {
    let t = &cx.theme().colors;
    [("Chrome", 0.55, t.chart_1), ("Safari", 0.2, t.chart_2), ("Firefox", 0.12, t.chart_3), ("Edge", 0.08, t.chart_4), ("Other", 0.05, t.chart_5)]
        .iter()
        .enumerate()
        .map(|(i, &(b, s, color))| Share { browser: b.into(), share: s, color, inner: 20. + 4. * i as f32, outer: 70. - 3. * i as f32 })
        .collect()
}

fn scores() -> Vec<Score> {
    [("Speed", 80.), ("Power", 65.), ("Range", 90.), ("Comfort", 55.), ("Price", 70.)]
        .iter()
        .map(|&(d, a)| Score { dimension: d.into(), alpha: a })
        .collect()
}

fn flows(cx: &App) -> (Vec<Flow>, Vec<SankeyLink>) {
    let t = &cx.theme().colors;
    let nodes = [("Revenue", t.chart_1), ("Cost", t.chart_2), ("Profit", t.chart_3), ("Tax", t.chart_4), ("Net", t.chart_5)]
        .iter()
        .map(|&(n, color)| Flow { name: n.into(), color })
        .collect();
    let links = vec![
        SankeyLink::new(0, 1, 60.),
        SankeyLink::new(0, 2, 40.),
        SankeyLink::new(2, 3, 10.),
        SankeyLink::new(2, 4, 30.),
    ];
    (nodes, links)
}

#[derive(Clone)]
struct Pt {
    month: SharedString,
    x: f32,
    top: f32,
    line: f32,
    base: f32,
    angle: f32,
    radius: f32,
    color: Hsla,
}

/// Composite low-level `Plot` exercising `PlotAxis`, `Grid`, `Bar`, `Area`,
/// `Line`, and `RadialLine` shapes on a single canvas.
struct Shapes {
    pts: Vec<Pt>,
}

impl Plot for Shapes {
    fn paint(&mut self, bounds: Bounds<Pixels>, window: &mut Window, cx: &mut App) {
        let t = &cx.theme().colors;
        let (muted, border, chart_1, chart_2, chart_3, background) =
            (t.muted_foreground, t.border, t.chart_1, t.chart_2, t.chart_3, t.background);
        PlotAxis::new()
            .x(px(200.))
            .x_label(self.pts.iter().map(|p| AxisText::new(p.month.clone(), px(p.x), muted)))
            .stroke(border)
            .paint(&bounds, window, cx);
        Grid::new()
            .y([px(50.), px(100.), px(150.)])
            .stroke(border)
            .dash_array(&[px(4.), px(2.)])
            .paint(&bounds, window);
        Bar::new()
            .data(self.pts.clone())
            .band_width(20.)
            .cross(|p| Some(p.x - 10.))
            .base(|p| p.base)
            .value(|p| Some(p.top))
            .fill(|p, _, _| p.color)
            .corner_radii(px(2.))
            .paint(&bounds, window, cx);
        Area::new()
            .data(self.pts.clone())
            .x(|p| Some(p.x))
            .y0(200.)
            .y1(|p| Some(p.line))
            .fill(hsla(0.3, 0.5, 0.5, 0.2))
            .stroke(chart_3)
            .curve(Curve::Linear)
            .paint(&bounds, window);
        Line::new()
            .data(self.pts.clone())
            .x(|p| Some(p.x))
            .y(|p| Some(p.line))
            .stroke(chart_2)
            .stroke_width(px(2.))
            .curve(Curve::Natural)
            .dot()
            .dot_size(px(6.))
            .dot_fill(chart_2)
            .dot_stroke(background)
            .paint(&bounds, window);
        RadialLine::new()
            .data(self.pts.clone())
            .angle(|p, _| Some(p.angle))
            .radius(|p, _| Some(p.radius))
            .closed()
            .fill(hsla(0.6, 0.5, 0.5, 0.2))
            .stroke(chart_1)
            .stroke_width(px(1.))
            .dot()
            .dot_size(px(4.))
            .dot_fill(chart_1)
            .dot_stroke(chart_1)
            .paint(&bounds, window);
    }
}

impl IntoElement for Shapes {
    type Element = PlotElement<Self>;

    fn into_element(self) -> Self::Element {
        PlotElement::new(self)
    }
}

fn pts(cx: &App) -> Vec<Pt> {
    months(cx)
        .into_iter()
        .enumerate()
        .map(|(i, m)| Pt {
            month: m.month,
            x: 30. + 55. * i as f32,
            top: 200. - (m.desktop as f32) * 0.5,
            line: 200. - (m.mobile as f32) * 0.5,
            base: 200.,
            angle: i as f32,
            radius: 20. + (m.mobile as f32) * 0.125,
            color: m.color,
        })
        .collect()
}

fn card(chart: impl IntoElement) -> AnyElement {
    div().w(px(360.)).h(px(220.)).child(chart).into_any_element()
}

fn pair(a: AnyElement, b: AnyElement) -> AnyElement {
    div().flex().gap_4().child(a).child(b).into_any_element()
}

/// Renders a chart, custom plot, or `generic-sidebar` page if `id` belongs here.
pub fn page(id: &str, cx: &App) -> Option<AnyElement> {
    let t = &cx.theme().colors;
    Some(match id {
        "chart-line" => pair(
            card(
                LineChart::new(months(cx))
                    .id("line-a")
                    .appear(false)
                    .x(|d| d.month.clone())
                    .y(|d| d.desktop)
                    .stroke(t.chart_1)
                    .dot()
                    .name("Desktop")
                    .tick_margin(2)
                    .y_axis(true)
                    .y_tick_count(4)
                    .y_tick_format(|v| format!("{v:.0}k"))
                    .grid_dashed(false)
                    .reference_line(150.),
            ),
            card(
                LineChart::new(months(cx))
                    .id("line-b")
                    .appear(false)
                    .interactive(false)
                    .x(|d| d.month.clone())
                    .y(|d| d.mobile)
                    .stroke(t.chart_2)
                    .linear()
                    .x_axis(false)
                    .grid(false)
                    .y_domain(0., 300.)
                    .y_padding(20., 10.)
                    .y_axis(true)
                    .y_axis_label_placement(AxisLabelPlacement::Inside),
            ),
        ),
        "chart-line-more" => pair(
            card(
                LineChart::new(months(cx))
                    .id("line-c")
                    .appear(false)
                    .appear_key("2025")
                    .x(|d| d.month.clone())
                    .y(|d| d.desktop)
                    .step_after()
                    .x_tick_count(3)
                    .grid_columns(3),
            ),
            card(
                LineChart::new(months(cx))
                    .id("line-d")
                    .appear(false)
                    .x(|d| d.month.clone())
                    .y(|d| d.mobile)
                    .natural()
                    .point_count(8),
            ),
        ),
        "chart-area" => pair(
            card(
                AreaChart::new(months(cx))
                    .id("area-a")
                    .appear(false)
                    .x(|d| d.month.clone())
                    .y(|d| d.desktop)
                    .stroke(t.chart_1)
                    .fill(hsla(0.6, 0.5, 0.5, 0.3))
                    .y(|d| d.mobile)
                    .stroke(t.chart_2)
                    .fill(hsla(0.3, 0.5, 0.5, 0.3))
                    .name("Visits")
                    .y_axis(true)
                    .y_tick_count(3)
                    .y_tick_format(|v| format!("{v:.0}"))
                    .tick_margin(2),
            ),
            card(
                AreaChart::new(months(cx))
                    .id("area-b")
                    .appear(false)
                    .interactive(false)
                    .x(|d| d.month.clone())
                    .y(|d| d.desktop)
                    .step_after()
                    .linear()
                    .natural()
                    .x_axis(false)
                    .grid(true)
                    .grid_dashed(false)
                    .grid_columns(4)
                    .x_tick_count(2)
                    .y_domain(0., 400.)
                    .point_count(7)
                    .reference_line(200.)
                    .y_padding(0., 0.)
                    .y_axis(true)
                    .y_axis_label_placement(AxisLabelPlacement::Inside)
                    .appear_key("k"),
            ),
        ),
        "chart-bar" => pair(
            card(
                BarChart::new(months(cx))
                    .id("bar-a")
                    .appear(false)
                    .band(|d| d.month.clone())
                    .value(|d| d.desktop)
                    .fill(|d, _, _, _| d.color)
                    .label(|d| format!("{}", d.desktop))
                    .label_color(|d| d.color)
                    .name("Desktop")
                    .tick_margin(1)
                    .value_axis(true)
                    .value_tick_count(4)
                    .value_tick_format(|v| format!("{v:.0}"))
                    .corner_radii(px(4.))
                    .padding_inner(0.3)
                    .padding_outer(0.2)
                    .max_band_width(px(40.))
                    .min_length(2.)
                    .grid_dashed(false),
            ),
            card(
                BarChart::new(months(cx))
                    .id("bar-b")
                    .appear(false)
                    .interactive(false)
                    .band(|d| d.month.clone())
                    .value(|d| d.mobile)
                    .alignment(BarAlignment::Left)
                    .label_axis(true)
                    .band_count(8)
                    .band_tick_count(3)
                    .grid(false)
                    .value_axis(true)
                    .value_axis_label_placement(AxisLabelPlacement::Inside)
                    .appear_key("k"),
            ),
        ),
        "chart-candlestick" => pair(
            card(
                CandlestickChart::new(prices())
                    .id("candle-a")
                    .appear(false)
                    .x(|d| d.date.clone())
                    .open(|d| d.open)
                    .high(|d| d.high)
                    .low(|d| d.low)
                    .close(|d| d.close)
                    .bullish(t.chart_bullish)
                    .bearish(t.chart_bearish)
                    .tick_margin(2),
            ),
            card(
                CandlestickChart::new(prices())
                    .id("candle-b")
                    .appear(false)
                    .interactive(false)
                    .appear_key("k")
                    .x(|d| d.date.clone())
                    .open(|d| d.open)
                    .high(|d| d.high)
                    .low(|d| d.low)
                    .close(|d| d.close)
                    .body_width_ratio(0.4)
                    .max_band_width(px(12.))
                    .x_axis(false)
                    .grid(false),
            ),
        ),
        "chart-pie" => pair(
            card(
                PieChart::new(shares(cx))
                    .id("pie-a")
                    .appear(false)
                    .value(|d| d.share)
                    .color(|d| d.color)
                    .inner_radius(30.)
                    .outer_radius(80.)
                    .pad_angle(0.02)
                    .name("Share"),
            ),
            card(
                PieChart::new(shares(cx))
                    .id("pie-b")
                    .appear(false)
                    .interactive(false)
                    .appear_key("k")
                    .value(|d| d.share)
                    .color(|d| d.color)
                    .label(|d| d.browser.clone())
                    .label_line_color(|d| d.color)
                    .label_color(t.muted_foreground)
                    .label_gap(10.)
                    .inner_radius_ratio(0.5)
                    .outer_radius(60.),
            ),
        ),
        "chart-pie-fn" => card(
            PieChart::new(shares(cx))
                .id("pie-c")
                .appear(false)
                .value(|d| d.share)
                .color(|d| d.color)
                .inner_radius_fn(|a| a.data.inner)
                .outer_radius_fn(|a| a.data.outer),
        ),
        "chart-radar" => pair(
            card(
                RadarChart::new(scores())
                    .id("radar-a")
                    .appear(false)
                    .value(|d| d.alpha)
                    .label(|d| d.dimension.clone())
                    .stroke(t.chart_1)
                    .fill(hsla(0.6, 0.5, 0.5, 0.3))
                    .name("Alpha")
                    .dot(),
            ),
            card(
                RadarChart::new(scores())
                    .id("radar-b")
                    .appear(false)
                    .interactive(false)
                    .appear_key("k")
                    .value(|d| d.alpha)
                    .label(|d| d.dimension.clone())
                    .label_color(t.chart_2)
                    .label_gap(4.)
                    .max_value(100.)
                    .outer_radius(70.)
                    .grid(true)
                    .grid_levels(3),
            ),
        ),
        "chart-sankey" => {
            let (nodes, links) = flows(cx);
            let (nodes2, links2) = flows(cx);
            pair(
                card(
                    SankeyChart::new(nodes, links)
                        .id("sankey-a")
                        .appear(false)
                        .node_color(|d| d.color)
                        .node_label(|d| d.name.clone())
                        .value_label(|_, v| format!("{v}").into()),
                ),
                card(
                    SankeyChart::new(nodes2, links2)
                        .id("sankey-b")
                        .appear(false)
                        .interactive(false)
                        .appear_key("k")
                        .node_label(|d| d.name.clone())
                        .node_width(14.)
                        .node_padding(8.)
                        .node_align(SankeyAlign::Justify)
                        .iterations(8)
                        .value_scale(SankeyValueScale::Sqrt)
                        .node_corner_radius(px(3.))
                        .link_opacity(0.5)
                        .min_link_width(2.)
                        .label_gap(4.),
                ),
            )
        }
        "chart-tooltip-line" => card(
            LineChart::new(months(cx))
                .id("tip-line")
                .appear(false)
                .x(|d| d.month.clone())
                .y(|d| d.desktop)
                .name("Desktop")
                .tooltip_title(|d| d.month.clone())
                .tooltip_value(|_, v| format!("{v:.1} visits").into())
                .tooltip_value_color(|d, _| d.color),
        ),
        "chart-tooltip-bar" => card(
            BarChart::new(months(cx))
                .id("tip-bar")
                .appear(false)
                .band(|d| d.month.clone())
                .value(|d| d.desktop)
                .tooltip_content(|d, _, _| Label::new(format!("{} sold in {}", d.desktop, d.month))),
        ),
        "chart-tooltip-pie" => card(
            PieChart::new(shares(cx))
                .id("tip-pie")
                .appear(false)
                .value(|d| d.share)
                .color(|d| d.color)
                .tooltip_name(|d| d.browser.clone())
                .tooltip_value(|_, v, p| format!("{v} ({p:.1}%)").into()),
        ),
        "plot-custom" => card(Shapes { pts: pts(cx) }),

        "comp6-sankey-labels" => {
            let (nodes, links) = flows(cx);
            let muted = cx.theme().muted_foreground;
            card(
                SankeyChart::new(nodes, links)
                    .id("sankey-c6")
                    .appear(false)
                    .node_color(|d| d.color)
                    .labels(move |d, _| {
                        vec![
                            SankeyLabel::new(d.name.clone()).font_size(12.),
                            SankeyLabel::new(format!("{} flow", d.name)).color(muted).font_size(10.),
                        ]
                    }),
            )
        }
        "comp6-bar-gradient" => card(
            BarChart::new(months(cx))
                .id("bar-c6")
                .appear(false)
                .band(|d| d.month.clone())
                .value(|d| d.desktop)
                .fill_gradient(|d, _, _| [linear_color_stop(d.color, 0.), linear_color_stop(hsla(0.6, 0.5, 0.9, 1.), 1.)]),
        ),
        "comp6-bar-labels" => card(LabeledBars { months: months(cx) }),

        "generic-sidebar" => div()
            .w(px(240.))
            .h(px(300.))
            .child(
                Sidebar::new("sb")
                    .side(Side::Left)
                    .collapsible(false)
                    .collapsed(false)
                    .header(Label::new("Header"))
                    .footer(Label::new("Footer"))
                    .child(
                        SidebarMenu::new()
                            .child(SidebarMenuItem::new("Home"))
                            .child(SidebarMenuItem::new("Inbox").active(true)),
                    )
                    .children([SidebarMenu::new().child(SidebarMenuItem::new("Settings"))]),
            )
            .into_any_element(),
        _ => return None,
    })
}

struct LabeledBars {
    months: Vec<Month>,
}

#[derive(Clone)]
struct BarPt {
    x_left: f32,
    x: f32,
    top: f32,
    desktop: f64,
    color: Hsla,
}

impl Plot for LabeledBars {
    fn paint(&mut self, bounds: Bounds<Pixels>, window: &mut Window, cx: &mut App) {
        let t = &cx.theme().colors;
        let (border, foreground) = (t.border, t.foreground);
        let pts: Vec<BarPt> = self
            .months
            .iter()
            .enumerate()
            .map(|(i, m)| BarPt {
                x_left: 20. + 55. * i as f32,
                x: 30. + 55. * i as f32,
                top: 200. - (m.desktop as f32) * 0.5,
                desktop: m.desktop,
                color: m.color,
            })
            .collect();
        PlotAxis::new().x(px(200.)).stroke(border).paint(&bounds, window, cx);
        Bar::new()
            .data(pts)
            .band_width(20.)
            .cross(|p| Some(p.x_left))
            .base(|_| 200.)
            .value(|p| Some(p.top))
            .fill(|p, _, _| p.color)
            .label(move |p, _| {
                vec![gpui_kit::base::plot::label::Text::new(
                    format!("{:.0}", p.desktop),
                    gpui_kit::point(px(p.x), px(p.top - 14.)),
                    foreground,
                )]
            })
            .paint(&bounds, window, cx);
    }
}

impl IntoElement for LabeledBars {
    type Element = PlotElement<Self>;

    fn into_element(self) -> Self::Element {
        PlotElement::new(self)
    }
}

fn linear_color_stop(color: Hsla, percentage: f32) -> gpui_kit::LinearColorStop {
    gpui_kit::LinearColorStop { color, percentage }
}
