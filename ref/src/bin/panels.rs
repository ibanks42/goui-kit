//! Pure-Rust multi-panel dashboard benchmark reference app.
//!
//! Renders `panels` independent subviews (default 60), each containing
//! `labels_per_panel` hoverable cells (default 64), and dirties `changing`
//! panels per frame (default 2) via `window.on_next_frame`. CLI arguments:
//! `[--script <path>] [panels] [labels_per_panel] [changing]`.

use gpui_kit::{
    App, AppContext, Context, Entity, InteractiveElement, IntoElement, ParentElement, Render,
    SharedString, Styled, Window, div, hsla, px,
};
use std::cell::Cell;
use std::rc::Rc;

/// Prime-sized pool of preallocated two-digit cell labels rotated each tick.
const LABELS: usize = 97;

/// Root flex-wrap container displaying all panel subviews.
struct Dashboard {
    panels: Vec<Entity<Panel>>,
}

/// Single independently invalidated dashboard card.
struct Panel {
    index: usize,
    labels: Rc<Vec<SharedString>>,
    count: usize,
    tick: usize,
}

impl Render for Panel {
    fn render(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        let tick = self.tick;
        div()
            .flex()
            .flex_col()
            .w(px(220.))
            .p_1()
            .gap_1()
            .rounded_sm()
            .bg(hsla(0.6, 0.2, 0.18, 1.))
            .child(format!("panel {} · {}", self.index, tick))
            .child(
                div()
                    .flex()
                    .flex_row()
                    .flex_wrap()
                    .children((0..self.count).map(|cell| {
                        div()
                            .w(px(26.))
                            .h(px(14.))
                            .hover(|style| style.bg(hsla(0.1, 0.6, 0.4, 1.)))
                            .child(self.labels[(cell + tick) % LABELS].clone())
                    })),
            )
    }
}

impl Render for Dashboard {
    fn render(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        div()
            .size_full()
            .flex()
            .flex_row()
            .flex_wrap()
            .gap_1()
            .bg(hsla(0., 0., 0.12, 1.))
            .text_color(hsla(0., 0., 0.85, 1.))
            .text_xs()
            .children(self.panels.iter().cloned())
    }
}

fn change(
    panels: Rc<Vec<Entity<Panel>>>,
    changing: usize,
    tick: Rc<Cell<usize>>,
    window: &mut Window,
    cx: &mut App,
) {
    tick.set(tick.get() + 1);
    let count = panels.len();
    for n in 0..changing {
        panels[(tick.get() * 7 + n * 13) % count].update(cx, |panel, cx| {
            panel.tick += 1;
            cx.notify();
        });
    }
    window.on_next_frame(move |window, cx| change(panels, changing, tick, window, cx));
}

fn dashboard(panels: usize, labels_per_panel: usize, changing: usize, window: &mut Window, cx: &mut App) -> Entity<Dashboard> {
    gk_meter::attach(window, cx);
    let labels: Rc<Vec<SharedString>> = Rc::new(
        (0..LABELS)
            .map(|n| SharedString::from(format!("{n:02}")))
            .collect(),
    );
    let views: Vec<Entity<Panel>> = (0..panels)
        .map(|index| {
            let labels = labels.clone();
            cx.new(|_| Panel {
                index,
                labels,
                count: labels_per_panel,
                tick: index,
            })
        })
        .collect();
    if changing > 0 && panels > 0 {
        let all = Rc::new(views.clone());
        let tick = Rc::new(Cell::new(0));
        window.on_next_frame(move |window, cx| change(all, changing, tick, window, cx));
    }
    cx.new(|_| Dashboard { panels: views })
}

fn main() {

    let args: Vec<String> = std::env::args().skip(1).collect();
    let (script, rest) = if args.first().map(String::as_str) == Some("--script") && args.len() > 1 {
        (Some(args[1].clone()), &args[2..])
    } else {
        (None, &args[..])
    };
    let num = |i: usize, d: usize| rest.get(i).and_then(|a| a.parse().ok()).unwrap_or(d);
    let (panels, labels_per_panel, changing) = (num(0, 60), num(1, 64), num(2, 2));
    if let Some(path) = script {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&path).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(1400., 900., cx);
                gpui_kit::open_window(options, cx, |window, cx| dashboard(panels, labels_per_panel, changing, window, cx))
                    .expect("open_window")
                    .0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            let _ = path;
            eprintln!("ref-panels: --script needs the diag build");
            std::process::exit(2);
        }
    }

    gpui_kit::application()
        .with_assets(gpui_kit::assets::Assets)
        .run(move |cx| {
            gpui_kit::init(cx);
            let options = gk_meter::window_options(1400., 900., cx);
            gpui_kit::open_window(options, cx, move |window, cx| dashboard(panels, labels_per_panel, changing, window, cx))
                .expect("open_window");
            cx.activate(true);
        });
}
