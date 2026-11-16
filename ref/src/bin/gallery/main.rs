//! ref-gallery: the Rust reference of the gallery, a pure Rust GPUI-kit app on gpui-fast that
//! shows every stateless component in a fixed set of states, one page per component and state
//! group, and then one generated page per bound function (conf.rs, written by
//! gates/conformance.py). The Odin twin is examples/odin/gallery; the pages of both are named
//! by ref/gallery/pages.txt and conf.txt and compared pixel for pixel by gates/gallery.sh.
//!
//!   ref-gallery --script FILE   offscreen, driven by a gk-meter script (diag build)
//!   ref-gallery --list          the page ids, one per line
//!   ref-gallery                 a real window (GK_FRAMES and GK_CLICK drive and measure it)
//!   GALLERY_START=id            the page shown first (default: the first of pages.txt)
//!
//! The shell is one view: a bar with a Next button and the page's name, then the page. A click
//! on Next (window position 40,32) shows the following page and resets the pages' state.
use gpui_kit::component::button::Button;
use gpui_kit::component::input::InputState;
use gpui_kit::component::label::Label;
use gpui_kit::component::{ActiveTheme, h_flex, v_flex};
use gpui_kit::{
    App, AppContext, Context, Entity, IntoElement, ParentElement, Render, SharedString, Styled,
    Window, div,
};

mod chart_pages;
mod conf;
mod input_pages;
mod lists2_pages;
mod rows_pages;
mod overlay_pages;
mod pages;
mod editor_pages; // the editor family, docs/PROTOCOLS.md 9.20 to 9.23
mod values_pages; // the values family (docs/PROTOCOLS.md 13.8 to 13.12)
mod views_pages; // the views family
mod motion_pages; // the platform and motion families, docs/PROTOCOLS.md 13.30

/// The hand-written pages, then the conformance pages gates/conformance.py generates.
const PAGES: &str = concat!(
    include_str!("../../../gallery/pages.txt"),
    include_str!("../../../gallery/conf.txt")
);
pub const ROWS: usize = 10_000;

/// The page ids of pages.txt, in order.
fn ids() -> Vec<&'static str> {
    PAGES
        .lines()
        .map(str::trim)
        .filter(|l| !l.is_empty() && !l.starts_with('#'))
        .map(|l| l.split('|').next().unwrap_or("").trim())
        .collect()
}

pub struct Gallery {
    ids: Vec<&'static str>,
    page: usize,
    /// Clicks on the current page's button.
    pub count: usize,
    /// The current page's checkbox or switch.
    pub flag: bool,
    pub name: Entity<InputState>,
    pub rows: Vec<SharedString>,
    /// The input family's pages' own states (input_pages.rs).
    pub input: input_pages::InputPages,
    /// The rows family's pages' own states (rows_pages.rs).
    pub rows_family: rows_pages::RowsPages,
    /// The lists and editor families' pages' own states (lists2_pages.rs).
    pub lists2: lists2_pages::Lists2Pages,
    /// The values family's pages' own states (values_pages.rs).
    pub values: values_pages::ValuesPages,
    /// The platform and motion families' pages' own state (motion_pages.rs).
    pub motion: motion_pages::MotionPages,
}

impl Render for Gallery {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let id = self.ids[self.page];
        let title = format!("{}/{} {}", self.page + 1, self.ids.len(), id);
        v_flex()
            .size_full()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .child(
                h_flex()
                    .p_4()
                    .gap_3()
                    .items_center()
                    .child(Button::new("next").label("Next").on_click(cx.listener(
                        |this, _, window, cx| {
                            this.page = (this.page + 1) % this.ids.len();
                            this.count = 0;
                            this.flag = false;
                            this.name.update(cx, |name, cx| name.set_value("", window, cx));
                            cx.notify();
                        },
                    )))
                    .child(Label::new(title)),
            )
            .child(div().w_full().p_4().child(
                conf::page(id)
                    .or_else(|| input_pages::page(id, self))
                    .or_else(|| rows_pages::page(id, self))
                    .or_else(|| lists2_pages::page(id, self))
                    .or_else(|| chart_pages::page(id, cx))
                    .or_else(|| overlay_pages::page(id, window, cx))
                    .or_else(|| editor_pages::page(id, cx))
                    .or_else(|| values_pages::page(id, cx))
                    .or_else(|| values_pages::page_state(id, self))
                    .or_else(|| views_pages::page(id))
                    .or_else(|| motion_pages::page(id, self, window, cx))
                    .unwrap_or_else(|| pages::page(id, self, window, cx)),
            ))
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<Gallery> {
    gk_meter::attach(window, cx);
    let ids = ids();
    let start = std::env::var("GALLERY_START").ok();
    let page = start
        .and_then(|s| ids.iter().position(|id| *id == s))
        .unwrap_or(0);
    let name = cx.new(|cx| InputState::new(window, cx).placeholder("Name"));
    let input = input_pages::InputPages::new(window, cx);
    let rows_family = rows_pages::RowsPages::new(window, cx);
    let lists2 = lists2_pages::Lists2Pages::new(window, cx);
    let values = values_pages::ValuesPages::new(window, cx);
    let motion = motion_pages::MotionPages::new(cx);
    cx.new(|_| Gallery {
        ids,
        page,
        count: 0,
        flag: false,
        name,
        rows: (0..ROWS).map(|i| SharedString::from(format!("row {i}"))).collect(),
        input,
        rows_family,
        lists2,
        values,
        motion,
    })
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    match args.first().map(String::as_str) {
        Some("--list") => {
            for id in ids() {
                println!("{id}");
            }
            return;
        }
        Some("--script") => {
            #[cfg(feature = "diag")]
            {
                let script = std::fs::read_to_string(&args[1]).expect("script file");
                gk_meter::headless::run(&script, |cx| {
                    let options = gk_meter::window_options(800., 600., cx);
                    gpui_kit::open_window(options, cx, build).expect("open_window").0
                })
                .expect("headless run");
                return;
            }
            #[cfg(not(feature = "diag"))]
            {
                eprintln!("ref-gallery: --script needs the diag build");
                std::process::exit(2);
            }
        }
        _ => {}
    }
    gpui_kit::application()
        .with_assets(gpui_kit::assets::Assets)
        .run(move |cx| {
            gpui_kit::init(cx);
            let options = gk_meter::window_options(800., 600., cx);
            gpui_kit::open_window(options, cx, build).expect("open_window");
            cx.activate(true);
        });
}
