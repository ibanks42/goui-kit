//! Headless script runner for offscreen rendering, synthetic input, and screenshot capture.
//!
//! Used under the `diag` feature by both `libgouikit` and the pure-Rust reference apps to drive
//! deterministic frame-by-frame test sessions without an X11 or Wayland display.

use gpui_kit::{AnyWindowHandle, App, HeadlessAppContext, Keystroke, Window, point, px};
use std::cell::RefCell;
use std::sync::Arc;

thread_local! {
    /// Optional app-registered callback invoked by the script `answer` command
    /// to resolve pending platform prompts (such as file dialogs).
    static ANSWER: RefCell<Option<Box<dyn FnMut(&str, &mut App) -> anyhow::Result<()>>>> = RefCell::new(None);
}

/// Registers the handler called when a headless script executes an `answer <text>` command.
pub fn on_answer(f: impl FnMut(&str, &mut App) -> anyhow::Result<()> + 'static) {
    ANSWER.with(|a| *a.borrow_mut() = Some(Box::new(f)));
}

fn tracing() -> bool {
    static ON: std::sync::OnceLock<bool> = std::sync::OnceLock::new();
    *ON.get_or_init(|| std::env::var("GK_TRACE_TURNS").is_ok_and(|v| v == "1"))
}

/// Runs queued next-frame callbacks on `window` and performs one draw cycle.
fn step(window: &mut Window, cx: &mut App) {
    let n = window.simulate_next_frame(cx);
    if tracing() {
        eprintln!("gk-trace: step ran {n} next-frame callback(s), then draws");
    }
    window.draw(cx).clear(cx);
}

/// Drains background tasks, falls back to window 0 if the active window closed, and draws a frame.
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

/// Signals the running headless script loop to stop after the current command.
pub fn stop() {
    STOP.store(true, std::sync::atomic::Ordering::SeqCst);
}

/// Reports whether [`stop`] has been called for the current headless run.
pub fn stopped() -> bool {
    STOP.load(std::sync::atomic::Ordering::SeqCst)
}

/// Runs a headless script after opening a window with `open`.
pub fn run(script: &str, open: impl FnOnce(&mut App) -> AnyWindowHandle) -> anyhow::Result<()> {
    run_opt(script, |cx| Some(open(cx)))
}

/// Runs a newline-delimited headless script against a `HeadlessAppContext`.
///
/// Blank lines and lines starting with `#` are ignored. An optional leading `size W H` sets the
/// initial window size via [`crate::window_options`] before `open` runs.
///
/// Supported commands:
/// - `frame`: drain background tasks and render one frame
/// - `shot PATH`: capture the current window framebuffer to a PNG at `PATH`
/// - `click X Y` / `dblclick X Y` / `rclick X Y`: dispatch synthetic pointer clicks and settle
/// - `move X Y`: dispatch a mouse-move event and settle
/// - `scroll X Y DY`: dispatch a vertical pixel scroll at `(X, Y)` and settle
/// - `drag X1 Y1 X2 Y2`: press at `(X1, Y1)`, move in 4 steps to `(X2, Y2)`, and release
/// - `type TEXT`: parse and dispatch each character in `TEXT` as a keystroke
/// - `key SPEC`: parse and dispatch one GPUI keystroke spec (for example `cmd-s` or `enter`)
/// - `sleep MS` / `advance MS`: sleep wall time or advance GPUI's virtual clock by `MS` ms
/// - `window N` / `windows`: switch active window to index `N` or print the open window count
/// - `answer ARGS`: pass `ARGS` to the [`on_answer`] callback to resolve scripted OS dialogs
/// - `stat`: print current [`crate::counters`] snapshot
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
                // Park between move, down, and up so context-menu handlers can react to each step.
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
                // Drag spans 6 phases: press (0), four interpolated moves (1..=4), and release (5).
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
                // Take the closure out of the thread-local before calling it so re-entrant hooks do not panic.
                let mut hook = ANSWER.with(|a| a.borrow_mut().take());
                let Some(f) = hook.as_mut() else {
                    anyhow::bail!("answer: the app registered nothing that answers platform requests: {line}");
                };
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
