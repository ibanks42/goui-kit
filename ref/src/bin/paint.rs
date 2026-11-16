//! ref-paint: the Rust reference for the `paint` app (examples/odin/paint, examples/go/paint;
//! gates/pixels.sh paint): gradient backgrounds and a hand-written plot, as a Rust app writes them.
//! The bindings describe the same: `.bg(linear_gradient(..))` (a Fill, PROTOCOLS.md 11.38) and
//! `gk.plot(grid, area, lines, quads)` with the binding's own paths and quads (PROTOCOLS.md 12.17),
//! which draw with the same `PathBuilder`, `paint_path` and `paint_quad` calls as `Paint::paint`.
//!
//!   ref-paint                  a real window
//!   ref-paint --script FILE    offscreen, driven by a gk-meter script (diag build)
use gpui_kit::base::plot::{Grid, Plot, PlotElement};
use gpui_kit::component::{ActiveTheme, h_flex, v_flex};
use gpui_kit::{
    App, AppContext, BorderStyle, Bounds, ColorSpace, Context, Entity, Hsla, IntoElement, ParentElement, PathBuilder,
    Pixels, Render, Styled, Window, div, hsla, linear_color_stop, linear_gradient, point, px, quad, size,
    transparent_black,
};

fn cyan(a: f32) -> Hsla {
    hsla(0.5, 0.75, 0.55, a)
}

fn violet(a: f32) -> Hsla {
    hsla(0.75, 0.6, 0.65, a)
}

/// The wave the plot draws: integers only, so every language computes the same pixels.
fn wave() -> Vec<(f32, f32)> {
    (0..=48).map(|i| ((i * 12) as f32, (40 + (i * 37) % 150) as f32)).collect()
}

struct Root;

struct Paint {
    pts: Vec<(f32, f32)>,
}

impl Plot for Paint {
    fn paint(&mut self, b: Bounds<Pixels>, window: &mut Window, cx: &mut App) {
        let (border, primary) = (cx.theme().border, cx.theme().primary);
        Grid::new()
            .x([px(144.), px(288.), px(432.)])
            .y([px(60.), px(120.), px(180.)])
            .stroke(border)
            .paint(&b, window);
        let at = |(x, y): (f32, f32)| point(b.origin.x + px(x), b.origin.y + px(y));
        // the filled area under the wave, down to the middle line
        let mut area = PathBuilder::fill();
        area.move_to(at((0., 120.)));
        for p in &self.pts {
            area.line_to(at(*p));
        }
        area.line_to(at((576., 120.)));
        area.close();
        if let Ok(path) = area.build() {
            window.paint_path(path, hsla(0.6, 0.8, 0.7, 0.12));
        }
        // the line in a gradient, with two wider strokes under it as its glow
        for (width, alpha) in [(7., 0.12), (3., 0.25), (1.5, 1.)] {
            let mut line = PathBuilder::stroke(px(width));
            line.move_to(at(self.pts[0]));
            for p in &self.pts[1..] {
                line.line_to(at(*p));
            }
            if let Ok(path) = line.build() {
                window.paint_path(path, linear_gradient(90., linear_color_stop(cyan(alpha), 0.), linear_color_stop(violet(alpha), 1.)));
            }
        }
        // a quad in a gradient from a theme colour, mixed in Oklab, with a border; a round dot
        let bar = linear_gradient(0., linear_color_stop(primary, 0.), linear_color_stop(violet(1.), 1.)).color_space(ColorSpace::Oklab);
        window.paint_quad(quad(Bounds::new(at((480., 10.)), size(px(80.), px(36.))), px(6.), bar, px(2.), hsla(0., 0., 1., 1.), BorderStyle::Solid));
        window.paint_quad(quad(Bounds::new(at((283., 103.)), size(px(10.), px(10.))), px(5.), hsla(0., 0., 1., 1.), px(0.), transparent_black(), BorderStyle::Solid));
    }
}

impl IntoElement for Paint {
    type Element = PlotElement<Self>;

    fn into_element(self) -> Self::Element {
        PlotElement::new(self)
    }
}

impl Render for Root {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let swatch = || div().w(px(160.)).h(px(48.)).rounded(px(8.));
        v_flex()
            .size_full()
            .p_4()
            .gap_3()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .child(
                h_flex()
                    .gap_3()
                    .child(swatch().bg(linear_gradient(90., linear_color_stop(cyan(1.), 0.), linear_color_stop(violet(1.), 1.))))
                    .child(swatch().bg(
                        linear_gradient(180., linear_color_stop(cx.theme().primary, 0.2), linear_color_stop(hsla(0.1, 0.9, 0.6, 1.), 0.9))
                            .color_space(ColorSpace::Oklab),
                    ))
                    .child(swatch().bg(hsla(0.33, 0.5, 0.5, 1.))),
            )
            .child(div().w(px(576.)).h(px(240.)).child(Paint { pts: wave() }))
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<Root> {
    gk_meter::attach(window, cx);
    cx.new(|_| Root)
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(640., 400., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-paint: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application()
        .with_assets(gpui_kit::assets::Assets)
        .run(move |cx| {
            gpui_kit::init(cx);
            let options = gk_meter::window_options(640., 400., cx);
            gpui_kit::open_window(options, cx, build).expect("open_window");
            cx.activate(true);
        });
}
