//! Pure-Rust counter, text input, and 10,000-row uniform list reference app.
//!
//! Serves as the baseline against the Go counter example for pixel parity and
//! frame-cost gates. Supports interactive launch as well as headless script
//! playback (`--script <path>`) when compiled with the `diag` feature.

use gpui_kit::component::button::{Button, ButtonVariants};
use gpui_kit::component::input::{Input, InputState};
use gpui_kit::component::label::Label;
use gpui_kit::component::{ActiveTheme, h_flex, v_flex};
use gpui_kit::{
    App, AppContext, Context, Entity, IntoElement, ParentElement, Render, SharedString, Styled,
    Window, div, px, uniform_list,
};

/// Initial row count populated into the virtualized list.
const ROWS: usize = 10_000;

/// Top-level window view composing controls, live text echo, and the list.
struct Root {
    controls: Entity<Controls>,
    echo: Entity<Echo>,
    list: Entity<List>,
}

/// Toolbar view holding the increment button, counter label, and name input.
struct Controls {
    count: usize,
    name: Entity<InputState>,
    list: Entity<List>,
}

/// Separate view that mirrors `InputState::value` to test cross-view updates.
struct Echo {
    name: Entity<InputState>,
}

/// Virtualized 24 px row list backed by `uniform_list`.
struct List {
    rows: Vec<SharedString>,
}

impl Render for Root {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        v_flex()
            .size_full()
            .p_4()
            .gap_3()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .child(self.controls.clone())
            .child(self.echo.clone())
            .child(div().w_full().h(px(160.)).child(self.list.clone()))
    }
}

impl Render for Controls {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        h_flex()
            .gap_2()
            .items_center()
            .child(
                Button::new("add")
                    .primary()
                    .label("Add")
                    .on_click(cx.listener(|this, _, _, cx| {
                        this.count += 1;
                        let row = SharedString::from(format!("added {}", this.count));
                        this.list.update(cx, |list, cx| {
                            list.rows.push(row);
                            cx.notify();
                        });
                        cx.notify();
                    })),
            )
            .child(Label::new(format!("count {}", self.count)))
            .child(div().w(px(240.)).child(Input::new(&self.name)))
    }
}

impl Render for Echo {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        h_flex().child(Label::new(format!("hello {}", self.name.read(cx).value())))
    }
}

fn list_row(text: SharedString) -> gpui_kit::Div {
    div().h(px(24.)).px_2().child(text)
}

impl Render for List {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        uniform_list(
            "rows",
            self.rows.len(),
            cx.processor(|this, range: std::ops::Range<usize>, _, _| {
                range.map(|i| list_row(this.rows[i].clone())).collect::<Vec<_>>()
            }),
        )
        .size_full()
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<Root> {
    gk_meter::attach(window, cx);
    let name = cx.new(|cx| InputState::new(window, cx).placeholder("Name"));
    let list = cx.new(|_| List {
        rows: (0..ROWS).map(|i| SharedString::from(format!("row {i}"))).collect(),
    });
    let controls = cx.new(|_| Controls {
        count: 0,
        name: name.clone(),
        list: list.clone(),
    });
    let echo = cx.new(|_| Echo { name });
    cx.new(|_| Root { controls, echo, list })
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(640., 480., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-counter: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application()
        .with_assets(gpui_kit::assets::Assets)
        .run(move |cx| {
            gpui_kit::init(cx);
            let options = gk_meter::window_options(640., 480., cx);
            gpui_kit::open_window(options, cx, build).expect("open_window");
            cx.activate(true);
        });
}
