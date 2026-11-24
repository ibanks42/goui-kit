//! The offscreen driver (diag builds): one app window on gpui's HeadlessAppContext with the
//! real wgpu renderer and text system, no display and no compositor, driven by a script. The
//! Rust reference and the library run the same driver, so their PNGs can be compared byte for
//! byte in one run.
//!
//! Script, one command per line:
//!   size W H          window size; must come first (default 640 480)
//!   frame             run pending tasks, the pre-frame callbacks and one draw
//!   shot FILE.png     write the last drawn frame
//!   click X Y         left click at a window position, then a frame
//!   type TEXT         one keystroke per character to the focused element, a frame after each
//!   move X Y          move the pointer to a window position, then a frame (hover)
//!   key NAME          one named keystroke ("enter", "escape", "ctrl-a"), then a frame
//!   sleep MS          let other threads run for MS milliseconds (background posts)
//!   stat              print the boundary counters
//!   scroll X Y DY     a scroll of DY pixels at a window position (negative: content up), then a frame
//!   dblclick X Y      a double click at a window position, then a frame
//!   rclick X Y        a right click at a window position: a move, a press and a release, each in
//!                     its own update with pending tasks run between (as a real window delivers
//!                     them), then a frame
//!   drag X1 Y1 X2 Y2  press at the first position, move to the second with the button held
//!                     (four moves, a frame after each), release, then a frame
//!   answer WHAT       answers the oldest platform request of its kind the app made offscreen,
//!                     through what the app registered (`on_answer`): `paths P..`, `new-path P`,
//!                     `prompt N`, `cancel paths|new-path|prompt`, inside an update of the driven
//!                     window (what the answer defers runs before the frame); then a frame
//!                     (PROTOCOLS.md 11.31, 11.32)

use gpui_kit::{AnyWindowHandle, App, HeadlessAppContext, Keystroke, Window, point, px};
use std::cell::RefCell;
use std::sync::Arc;

thread_local! {
    /// What answers the script's `answer` lines: the library's scripted platform, or a Rust twin's.
    static ANSWER: RefCell<Option<Box<dyn FnMut(&str, &mut App) -> anyhow::Result<()>>>> = RefCell::new(None);
}

/// Registers what answers the script's `answer WHAT` lines (file dialogs, prompts) offscreen.
pub fn on_answer(f: impl FnMut(&str, &mut App) -> anyhow::Result<()> + 'static) {
    ANSWER.with(|a| *a.borrow_mut() = Some(Box::new(f)));
}

/// GK_TRACE_TURNS=1: each script line and each frame on stderr (the library adds its turns).
fn tracing() -> bool {
    static ON: std::sync::OnceLock<bool> = std::sync::OnceLock::new();
    *ON.get_or_init(|| std::env::var("GK_TRACE_TURNS").is_ok_and(|v| v == "1"))
}

fn step(window: &mut Window, cx: &mut App) {
    let n = window.simulate_next_frame(cx);
    if tracing() {
        eprintln!("gk-trace: step ran {n} next-frame callback(s), then draws");
    }
    window.draw(cx).clear(cx);
}

/// Runs what the last command started, then one frame of the driven window. When that command
/// closed the driven window (a key binding or a click whose handler closes its own window, a timer
/// that closes it), the script goes on with the first window still open, as `window 0` would, and
/// says so; with no window left it stops with an error. Before 12 Nov 2026 the frame after such a
/// command failed with "window not found" and ended the run.
fn settle(cx: &mut HeadlessAppContext, handle: &mut AnyWindowHandle) -> anyhow::Result<()> {
    cx.run_until_parked();
    let open = cx.update(|cx| cx.windows());
    if !open.contains(handle) {
        let Some(first) = open.first() else {
            anyhow::bail!("the script's window was closed and no window is left");
        };
        println!("gk-headless: the script's window was closed; window 0 from now on");
        *handle = *first;
    }
    cx.update_window(*handle, |_, window, cx| step(window, cx))?;
    Ok(())
}

static STOP: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);

/// Ends the script early: the line running finishes, the rest are not run, and `run` returns.
/// The offscreen platform has no loop that `App::quit` ends, so the app's quit (and a binding
/// that stops the app after a mistake in its code) asks here. Any thread.
pub fn stop() {
    STOP.store(true, std::sync::atomic::Ordering::SeqCst);
}

/// Whether `stop` was called since the script started.
pub fn stopped() -> bool {
    STOP.load(std::sync::atomic::Ordering::SeqCst)
}

/// Runs `script`. `open` is called once inside the app, after `gpui_kit::init`, and returns
/// the window to drive; it must build its window with `gk_meter::window_options`.
pub fn run(script: &str, open: impl FnOnce(&mut App) -> AnyWindowHandle) -> anyhow::Result<()> {
    run_opt(script, |cx| Some(open(cx)))
}

/// `run` for an app that may open no window: `open` returns none when the app stopped (`stop`)
/// before it opened one, and the script is not run.
pub fn run_opt(script: &str, open: impl FnOnce(&mut App) -> Option<AnyWindowHandle>) -> anyhow::Result<()> {
    STOP.store(false, std::sync::atomic::Ordering::SeqCst);
    let mut lines = script.lines().map(str::trim).filter(|l| !l.is_empty() && !l.starts_with('#'));
    let mut pending = None;
    let (mut w, mut h) = (640., 480.);
    if let Some(first) = lines.next() {
        let mut p = first.split_whitespace();
        if p.next() == Some("size") {
            w = p.next().and_then(|v| v.parse().ok()).unwrap_or(w);
            h = p.next().and_then(|v| v.parse().ok()).unwrap_or(h);
        } else {
            pending = Some(first);
        }
    }
    crate::set_headless_size(w, h);

    let mut cx = HeadlessAppContext::with_platform(
        gpui_kit::platform::current_platform(true).text_system(),
        Arc::new(gpui_kit::assets::Assets),
        gpui_kit::platform::current_headless_renderer,
    );
    cx.update(gpui_kit::init);
    let Some(mut handle) = cx.update(open) else {
        if stopped() {
            return Ok(());
        }
        anyhow::bail!("the app opened no window when it started");
    };
    cx.run_until_parked();

    for line in pending.into_iter().chain(lines) {
        if stopped() {
            break;
        }
        if tracing() {
            eprintln!("gk-trace: script {line}");
        }
        let (cmd, rest) = line.split_once(' ').unwrap_or((line, ""));
        match cmd {
            "frame" => {
                settle(&mut cx, &mut handle)?;
            }
            "shot" => {
                let image = cx.capture_screenshot(handle)?;
                image.save(rest)?;
                println!("gk-headless: wrote {rest} {}x{}", image.width(), image.height());
            }
            "click" => {
                let mut p = rest.split_whitespace().map(|v| v.parse::<f32>());
                let (Some(Ok(x)), Some(Ok(y))) = (p.next(), p.next()) else {
                    anyhow::bail!("click needs X Y: {line}");
                };
                cx.update_window(handle, |_, window, cx| {
                    crate::click(window, point(px(x), px(y)), cx);
                })?;
                settle(&mut cx, &mut handle)?;
            }
            "type" => {
                for ch in rest.chars() {
                    let text = ch.to_string();
                    cx.update_window(handle, |_, window, cx| -> anyhow::Result<()> {
                        let mut key = Keystroke::parse_with_os(&text, cx.platform_info().os)
                            .map_err(|e| anyhow::anyhow!("keystroke {text:?}: {e}"))?;
                        key.key_char = Some(text.clone());
                        window.dispatch_keystroke(key, cx);
                        Ok(())
                    })??;
                    settle(&mut cx, &mut handle)?;
                }
            }
            "move" => {
                let mut p = rest.split_whitespace().map(|v| v.parse::<f32>());
                let (Some(Ok(x)), Some(Ok(y))) = (p.next(), p.next()) else {
                    anyhow::bail!("move needs X Y: {line}");
                };
                cx.update_window(handle, |_, window, cx| {
                    window.dispatch_event(
                        gpui_kit::PlatformInput::MouseMove(gpui_kit::MouseMoveEvent {
                            position: point(px(x), px(y)),
                            pressed_button: None,
                            modifiers: Default::default(),
                        }),
                        cx,
                    );
                })?;
                settle(&mut cx, &mut handle)?;
            }
            "key" => {
                let name = rest.trim().to_string();
                cx.update_window(handle, |_, window, cx| -> anyhow::Result<()> {
                    let key = Keystroke::parse_with_os(&name, cx.platform_info().os)
                        .map_err(|e| anyhow::anyhow!("keystroke {name:?}: {e}"))?;
                    window.dispatch_keystroke(key, cx);
                    Ok(())
                })??;
                settle(&mut cx, &mut handle)?;
            }
            "sleep" => {
                let ms = rest.trim().parse::<u64>().unwrap_or(0);
                std::thread::sleep(std::time::Duration::from_millis(ms));
            }
            "rclick" => {
                let v: Vec<f32> = rest.split_whitespace().filter_map(|v| v.parse().ok()).collect();
                if v.len() != 2 {
                    anyhow::bail!("rclick needs 2 numbers: {line}");
                }
                // Before 12 Nov 2026 the three were dispatched in one update, so what the move's and
                // the press's handlers deferred ran only after the release, and nothing was drawn
                // between them. Now each update's end runs what it deferred and (gpui's test-support
                // flush) draws a window it dirtied, as a real window draws frames during a click
                // (PROTOCOLS.md 11.32).
                for input in crate::right_click_inputs(point(px(v[0]), px(v[1]))) {
                    cx.update_window(handle, |_, window, cx| {
                        window.dispatch_event(input, cx);
                    })?;
                    cx.run_until_parked();
                }
                settle(&mut cx, &mut handle)?;
            }
            "scroll" | "dblclick" | "drag" => {
                let v: Vec<f32> = rest.split_whitespace().filter_map(|v| v.parse().ok()).collect();
                let want = match cmd {
                    "scroll" => 3,
                    "dblclick" => 2,
                    _ => 4,
                };
                if v.len() != want {
                    anyhow::bail!("{cmd} needs {want} numbers: {line}");
                }
                let a = point(px(v[0]), px(v[1]));
                let phases = if cmd == "drag" { 6 } else { 1 };
                for phase in 0..phases {
                    cx.update_window(handle, |_, window, cx| match cmd {
                        "scroll" => crate::scroll(window, a, v[2], cx),
                        "dblclick" => crate::double_click(window, a, cx),
                        _ => crate::drag(window, a, point(px(v[2]), px(v[3])), phase, 4, cx),
                    })?;
                    settle(&mut cx, &mut handle)?;
                }
            }
            "window" => {
                let n: usize = rest.trim().parse().map_err(|_| anyhow::anyhow!("window needs N: {line}"))?;
                cx.run_until_parked();
                let all = cx.update(|cx| cx.windows());
                handle = *all.get(n).ok_or_else(|| anyhow::anyhow!("window {n}: {} windows are open", all.len()))?;
                cx.update_window(handle, |_, window, cx| step(window, cx))?;
            }
            "advance" => {
                let ms = rest.trim().parse::<u64>().map_err(|_| anyhow::anyhow!("advance needs MS: {line}"))?;
                cx.advance_clock(std::time::Duration::from_millis(ms));
                settle(&mut cx, &mut handle)?;
            }
            "answer" => {
                cx.run_until_parked();
                let mut hook = ANSWER.with(|a| a.borrow_mut().take());
                let Some(f) = hook.as_mut() else {
                    anyhow::bail!("answer: the app registered nothing that answers platform requests: {line}");
                };
                // Inside an update of the driven window, as a platform's answer arrives in an update
                // of the app: what the answer defers (the library's DONE turn, a twin's handler) runs
                // when that update ends, before the frame below. HeadlessAppContext::update calls its
                // closure without ending an update, so before 12 Nov 2026 the deferred work waited for
                // the end of the frame's own update, after the draw: the change was in the next frame
                // (PROTOCOLS.md 11.32).
                let res = cx
                    .update_window(handle, |_, _, cx| f(rest.trim(), cx))
                    .unwrap_or_else(|e| Err(e));
                ANSWER.with(|a| *a.borrow_mut() = hook);
                res?;
                settle(&mut cx, &mut handle)?;
            }
            "windows" => {
                cx.run_until_parked();
                println!("gk-headless: windows {}", cx.update(|cx| cx.windows().len()));
            }
            "stat" => {
                println!("gk-headless: {:?}", crate::counters());
            }
            _ => anyhow::bail!("unknown script command: {line}"),
        }
    }
    Ok(())
}
