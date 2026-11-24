//! Pure-Rust per-frame state-mutation reference app.
//!
//! Updates an `InputState` on every `window.on_next_frame` callback while a
//! sibling `Echo` view reads that value, verifying that writes made during
//! frame callbacks propagate cleanly to dependent views without extra latency.

use gpui_kit::component::input::{Input, InputState};
use gpui_kit::component::label::Label;
use gpui_kit::component::{ActiveTheme, h_flex, v_flex};
use gpui_kit::{
    App, AppContext, Context, Entity, IntoElement, ParentElement, Render, Styled, Window, div, px,
};

struct Root {
    name: Entity<InputState>,
    echo: Entity<Echo>,
}

struct Echo {
    name: Entity<InputState>,
}

impl Render for Root {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        v_flex()
            .size_full()
            .p_4()
            .gap_3()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .child(div().w(px(240.)).child(Input::new(&self.name)))
            .child(self.echo.clone())
    }
}

impl Render for Echo {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        h_flex().child(Label::new(format!("echo {}", self.name.read(cx).value())))
    }
}

/// Schedules a repeating next-frame callback that writes `"frame N"` into `name`.
fn arm(name: Entity<InputState>, frame: usize, window: &mut Window) {
    window.on_next_frame(move |window, cx| {
        let text = format!("frame {frame}");
        name.update(cx, |state, cx| state.set_value(text, window, cx));
        arm(name, frame + 1, window);
    });
}

fn build(window: &mut Window, cx: &mut App) -> Entity<Root> {
    gk_meter::attach(window, cx);
    let name = cx.new(|cx| InputState::new(window, cx).placeholder("Name"));
    let echo = cx.new(|_| Echo { name: name.clone() });
    arm(name.clone(), 1, window);
    cx.new(|_| Root { name, echo })
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(480., 200., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-writerule: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application()
        .with_assets(gpui_kit::assets::Assets)
        .run(move |cx| {
            gpui_kit::init(cx);
            let options = gk_meter::window_options(480., 200., cx);
            gpui_kit::open_window(options, cx, build).expect("open_window");
            cx.activate(true);
        });
}
