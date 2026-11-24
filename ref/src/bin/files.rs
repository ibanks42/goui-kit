//! Pure-Rust text file viewer reference app exercising native menu bars,
//! keyboard shortcuts, file open/save prompts, confirmation prompts, URL
//! opening, and animated status transitions.
//!
//! In headless `--script` mode, native OS dialogs are bypassed via a
//! thread-local `WAITING` queue answered by `answer` commands from the script.

use gpui_kit::base::animation::EffectTransition;
use gpui_kit::base::{GlobalState, v_flex};
use gpui_kit::component::ActiveTheme;
use gpui_kit::component::label::Label;
use gpui_kit::component::menu::AppMenuBar;
use gpui_kit::{
    App, AppContext, Context, Entity, IntoElement, KeyBinding, Menu, MenuItem, ParentElement, PathPromptOptions,
    PromptButton, PromptLevel, Render, Styled, WeakEntity, Window, div, px,
};
use std::cell::{Cell, RefCell};
use std::collections::VecDeque;
use std::path::PathBuf;
use std::time::Duration;

gpui_kit::actions!(files, [Open, SaveAs, Quit, Website]);

/// Synthetic or native response delivered to a pending dialog callback.
enum Answer {
    Paths(Option<Vec<PathBuf>>),
    Prompt(Option<usize>),
}

type Then = Box<dyn FnOnce(Answer, &mut App)>;

thread_local! {
    /// True while running under `--script`, causing prompts to enqueue on `WAITING`
    /// instead of opening native OS dialogs.
    static SCRIPTED: Cell<bool> = const { Cell::new(false) };
    /// Pending scripted dialog continuations tagged by prompt kind (`paths`, `new-path`, `prompt`).
    static WAITING: RefCell<VecDeque<(&'static str, Then)>> = const { RefCell::new(VecDeque::new()) };
}

/// Parses a headless script `answer ...` line and resolves the first matching
/// queued prompt continuation.
#[cfg(feature = "diag")]
fn answer_line(line: &str, cx: &mut App) -> anyhow::Result<()> {
    let mut words = line.split_whitespace();
    let what = words.next().unwrap_or("");
    let rest: Vec<&str> = words.collect();
    let (kind, answer) = match what {
        "paths" | "new-path" => (if what == "paths" { "paths" } else { "new-path" }, Answer::Paths(Some(rest.iter().map(PathBuf::from).collect()))),
        "prompt" => ("prompt", Answer::Prompt(rest.first().and_then(|v| v.parse().ok()))),
        "cancel" => match rest.first().copied() {
            Some("paths") => ("paths", Answer::Paths(None)),
            Some("new-path") => ("new-path", Answer::Paths(None)),
            Some("prompt") => ("prompt", Answer::Prompt(None)),
            _ => anyhow::bail!("answer cancel paths|new-path|prompt: {line}"),
        },
        _ => anyhow::bail!("answer paths P.. | new-path P | prompt N | cancel KIND: {line}"),
    };
    let then = WAITING.with(|w| {
        let mut w = w.borrow_mut();
        w.iter().position(|(k, _)| *k == kind).and_then(|i| w.remove(i)).map(|(_, t)| t)
    });
    let Some(then) = then else { anyhow::bail!("answer {line}: no request of that kind waits") };
    println!("gk-headless: answer {line}: taken");
    cx.defer(move |cx| then(answer, cx));
    Ok(())
}

/// Root file-viewer view state.
struct Files {
    menubar: Entity<AppMenuBar>,
    path: String,
    text: String,
    status: String,
    /// Monotonic counter used as the `EffectTransition` key whenever `status` changes.
    changes: usize,
    /// Guards against opening overlapping quit confirmation prompts.
    asking: bool,
}

impl Files {
    fn set_status(&mut self, s: impl Into<String>) {
        self.status = s.into();
        self.changes += 1;
    }

    fn opened(&mut self, paths: Option<Vec<PathBuf>>) {
        match paths {
            Some(p) if !p.is_empty() => {
                let path = p[0].to_string_lossy().into_owned();
                match std::fs::read(&p[0]) {
                    Ok(data) => {
                        self.text = String::from_utf8_lossy(&data).into_owned();
                        self.path = path;
                        self.set_status(format!("opened {} ({} bytes)", self.path, data.len()));
                    }
                    Err(_) => self.set_status(format!("could not read {path}")),
                }
            }
            _ => self.set_status("open cancelled"),
        }
    }

    fn saved_to(&mut self, paths: Option<Vec<PathBuf>>) {
        let Some(p) = paths.and_then(|p| p.into_iter().next()) else {
            self.set_status("save cancelled");
            return;
        };
        let shown = p.to_string_lossy().into_owned();
        match std::fs::write(&p, self.text.as_bytes()) {
            Ok(()) => self.set_status(format!("saved {shown}")),
            Err(_) => self.set_status(format!("could not write {shown}")),
        }
    }
}

fn ask(kind: &'static str, cx: &mut App, then: impl FnOnce(Answer, &mut App) + 'static) {
    if SCRIPTED.with(|s| s.get()) {
        WAITING.with(|w| w.borrow_mut().push_back((kind, Box::new(then))));
        return;
    }
    let _ = (kind, cx, then);
}

fn do_open(this: WeakEntity<Files>, cx: &mut App) {
    let then = move |a: Answer, cx: &mut App| {
        if let Answer::Paths(p) = a {
            this.update(cx, |f, cx| {
                f.opened(p);
                cx.notify();
            })
            .ok();
        }
    };
    if SCRIPTED.with(|s| s.get()) {
        return ask("paths", cx, then);
    }
    let rx = cx.prompt_for_paths(PathPromptOptions { files: true, directories: false, multiple: false, prompt: Some("Open".into()) });
    cx.spawn(async move |cx| {
        let p = rx.await.ok().and_then(|r| r.ok()).flatten();
        cx.update(|cx| then(Answer::Paths(p), cx));
    })
    .detach();
}

fn do_save(this: WeakEntity<Files>, cx: &mut App) {
    let dir = this
        .upgrade()
        .map(|f| {
            let p = f.read(cx).path.clone();
            match p.rfind('/') {
                Some(i) => p[..i].to_string(),
                None if p.is_empty() => ".".to_string(),
                None => String::new(),
            }
        })
        .unwrap_or_else(|| ".".into());
    let then = move |a: Answer, cx: &mut App| {
        if let Answer::Paths(p) = a {
            this.update(cx, |f, cx| {
                f.saved_to(p);
                cx.notify();
            })
            .ok();
        }
    };
    if SCRIPTED.with(|s| s.get()) {
        return ask("new-path", cx, then);
    }
    let rx = cx.prompt_for_new_path(std::path::Path::new(&dir), Some("untitled.txt"));
    cx.spawn(async move |cx| {
        let p = rx.await.ok().and_then(|r| r.ok()).flatten().map(|p| vec![p]);
        cx.update(|cx| then(Answer::Paths(p), cx));
    })
    .detach();
}

fn do_quit(this: WeakEntity<Files>, cx: &mut App) {
    let Some(f) = this.upgrade() else { return };
    if f.read(cx).asking {
        return;
    }
    f.update(cx, |f, _| f.asking = true);
    let then = move |a: Answer, cx: &mut App| {
        let quit = matches!(a, Answer::Prompt(Some(0)));
        this.update(cx, |f, cx| {
            f.asking = false;
            if !quit {
                f.set_status("still here");
            }
            cx.notify();
        })
        .ok();
        if quit {
            cx.quit();
        }
    };
    if SCRIPTED.with(|s| s.get()) {
        return ask("prompt", cx, then);
    }
    let Some(window) = cx.windows().first().copied() else { return };
    let rx = window
        .update(cx, |_, window, cx| {
            window.prompt(PromptLevel::Warning, "Quit the viewer?", None, &[PromptButton::ok("Quit"), PromptButton::cancel("Stay")], cx)
        })
        .ok();
    if let Some(rx) = rx {
        cx.spawn(async move |cx| {
            let n = rx.await.ok();
            cx.update(|cx| then(Answer::Prompt(n), cx));
        })
        .detach();
    }
}

fn do_site(this: WeakEntity<Files>, cx: &mut App) {
    if !SCRIPTED.with(|s| s.get()) {
        cx.open_url("https://github.com/longbridge/gpui-component");
    }
    this.update(cx, |f, cx| {
        f.set_status("opened the website in the browser");
        cx.notify();
    })
    .ok();
}

fn menus() -> Vec<Menu> {
    vec![
        Menu {
            name: "File".into(),
            items: vec![
                MenuItem::action("Open…", Open),
                MenuItem::action("Save as…", SaveAs),
                MenuItem::separator(),
                MenuItem::action("Quit", Quit),
            ],
            disabled: false,
        },
        Menu { name: "Help".into(), items: vec![MenuItem::action("Website", Website)], disabled: false },
    ]
}

impl Render for Files {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let status = div()
            .relative()
            .text_sm()
            .h(px(24.))
            .text_color(cx.theme().muted_foreground)
            .child(Label::new(self.status.clone()));
        let enter = EffectTransition::new(Duration::from_millis(250)).fade(0., 1.).slide_y(px(8.), px(0.));
        let text = if self.text.is_empty() { "(no file)".to_string() } else { self.text.clone() };
        v_flex()
            .size_full()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .child(div().flex_none().h(px(32.)).child(self.menubar.clone()))
            .child(
                v_flex()
                    .flex_1()
                    .p_4()
                    .gap_2()
                    .child(div().flex_1().border_1().p_2().overflow_hidden().border_color(cx.theme().border).child(Label::new(text)))
                    .child(enter.apply(status, format!("status-{}", self.changes))),
            )
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<Files> {
    gk_meter::attach(window, cx);
    cx.bind_keys([
        KeyBinding::new("ctrl-o", Open, None),
        KeyBinding::new("ctrl-shift-s", SaveAs, None),
        KeyBinding::new("ctrl-q", Quit, None),
    ]);

    cx.set_menus(menus());
    GlobalState::global_mut(cx).set_app_menus(menus().into_iter().map(Menu::owned).collect());
    let menubar = AppMenuBar::new(cx);
    let files = cx.new(|_| {
        let mut f = Files { menubar, path: String::new(), text: String::new(), status: String::new(), changes: 0, asking: false };
        f.set_status("File > Open… (ctrl-o) to read a text file");
        f
    });
    let w = files.downgrade();
    cx.on_action({
        let w = w.clone();
        move |_: &Open, cx| do_open(w.clone(), cx)
    });
    cx.on_action({
        let w = w.clone();
        move |_: &SaveAs, cx| do_save(w.clone(), cx)
    });
    cx.on_action({
        let w = w.clone();
        move |_: &Quit, cx| do_quit(w.clone(), cx)
    });
    cx.on_action(move |_: &Website, cx| do_site(w.clone(), cx));
    files
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            SCRIPTED.with(|s| s.set(true));
            gk_meter::headless::on_answer(answer_line);
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(720., 480., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-files: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application().with_assets(gpui_kit::assets::Assets).run(move |cx| {
        gpui_kit::init(cx);
        let options = gk_meter::window_options(720., 480., cx);
        gpui_kit::open_window(options, cx, build).expect("open_window");
        cx.activate(true);
    });
}
