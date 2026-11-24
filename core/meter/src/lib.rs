//! Frame timing, runtime counters, window-option helpers, and synthetic input dispatch.
//!
//! Both `libgouikit` and the pure-Rust reference apps share this crate so automated tests and
//! frame-cost measurements use identical window sizing, input sequences, and summary reporting.

use gpui_kit::{
    App, Bounds, EntityId, Modifiers, MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent,
    Pixels, Point, Window, WindowBounds, WindowOptions, point, px, size,
};
use std::cell::{Cell, RefCell};
use std::fmt::Write as _;
use std::rc::Rc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, Instant};

#[cfg(feature = "diag")]
pub mod headless;

/// Snapshot of cumulative runtime counters recorded by `libgouikit` and sampled once per frame
/// by [`attach`].
#[derive(Default, Clone, Copy, Debug)]
pub struct Counters {
    /// Total FFI callback turns invoked across all turn kinds.
    pub callbacks: u64,
    /// Total synchronous `gk_call` invocations from host callbacks.
    pub calls: u64,
    /// Cumulative wall nanoseconds spent inside host callbacks.
    pub callback_ns: u64,
    /// Cumulative estimated nanoseconds spent decoding tapes into GPUI elements.
    pub decode_ns: u64,
    /// Number of tape decode passes.
    pub decodes: u64,
    /// Number of tapes parsed and accepted.
    pub tapes: u64,
    /// Total raw byte length of accepted tapes.
    pub tape_bytes: u64,
    /// Counts of `K_FRAME`, `K_VIEWS`, `K_EVENT`, and `K_WAKE` callback turns.
    pub frame_turns: u64,
    pub views_turns: u64,
    pub event_turns: u64,
    pub wake_turns: u64,
    /// Number of buffered write operations executed outside the originating event handler.
    pub deferred_ops: u64,
    /// Number of `K_ROWS` row-source sync turns.
    pub rows_turns: u64,
    /// Individual row lookups that missed the cache and rendered a placeholder.
    pub rows_missing: u64,
    /// Frames on which at least one row source rendered a placeholder.
    pub placeholder_frames: u64,
}

/// Global atomic backing store for [`Counters`].
pub struct CounterRefs {
    callbacks: AtomicU64,
    calls: AtomicU64,
    callback_ns: AtomicU64,
    decode_ns: AtomicU64,
    decodes: AtomicU64,
    tapes: AtomicU64,
    tape_bytes: AtomicU64,
    frame_turns: AtomicU64,
    views_turns: AtomicU64,
    event_turns: AtomicU64,
    wake_turns: AtomicU64,
    deferred_ops: AtomicU64,
    rows_turns: AtomicU64,
    rows_missing: AtomicU64,
    placeholder_frames: AtomicU64,
}

static ATOMIC_COUNTERS: CounterRefs = CounterRefs {
    callbacks: AtomicU64::new(0),
    calls: AtomicU64::new(0),
    callback_ns: AtomicU64::new(0),
    decode_ns: AtomicU64::new(0),
    decodes: AtomicU64::new(0),
    tapes: AtomicU64::new(0),
    tape_bytes: AtomicU64::new(0),
    frame_turns: AtomicU64::new(0),
    views_turns: AtomicU64::new(0),
    event_turns: AtomicU64::new(0),
    wake_turns: AtomicU64::new(0),
    deferred_ops: AtomicU64::new(0),
    rows_turns: AtomicU64::new(0),
    rows_missing: AtomicU64::new(0),
    placeholder_frames: AtomicU64::new(0),
};

/// Single-writer relaxed increment used on the UI thread to avoid atomic RMW overhead.
#[inline(always)]
fn add_ui(a: &AtomicU64, v: u64) {
    if v != 0 {
        a.store(a.load(Ordering::Relaxed).wrapping_add(v), Ordering::Relaxed);
    }
}

/// Records one tape decode pass taking `ns` nanoseconds.
#[inline(always)]
pub fn bump_decode(ns: u64) {
    add_ui(&ATOMIC_COUNTERS.decodes, 1);
    add_ui(&ATOMIC_COUNTERS.decode_ns, ns);
}

/// Records one submitted tape buffer of `bytes` length.
#[inline(always)]
pub fn bump_tape(bytes: u64) {
    add_ui(&ATOMIC_COUNTERS.tapes, 1);
    add_ui(&ATOMIC_COUNTERS.tape_bytes, bytes);
}

/// Records `n` deferred tape operations queued for later execution.
#[inline(always)]
pub fn bump_deferred(n: u64) {
    add_ui(&ATOMIC_COUNTERS.deferred_ops, n);
}

/// Records one host callback turn of wire kind `kind` (3=frame, 4=views, 5=event, 7=wake).
#[inline(always)]
pub fn bump_turn(kind: u32, ns: u64) {
    add_ui(&ATOMIC_COUNTERS.callbacks, 1);
    add_ui(&ATOMIC_COUNTERS.callback_ns, ns);
    match kind {
        3 => add_ui(&ATOMIC_COUNTERS.frame_turns, 1),
        4 => add_ui(&ATOMIC_COUNTERS.views_turns, 1),
        5 => add_ui(&ATOMIC_COUNTERS.event_turns, 1),
        7 => add_ui(&ATOMIC_COUNTERS.wake_turns, 1),
        _ => {}
    }
}

thread_local! {
    static HEADLESS_SIZE: Cell<Option<(f32, f32)>> = const { Cell::new(None) };
}

/// Applies a batch of deltas populated by `f` to the global counters.
#[inline]
pub fn bump(f: impl FnOnce(&mut Counters)) {
    let mut d = Counters::default();
    f(&mut d);
    add_ui(&ATOMIC_COUNTERS.callbacks, d.callbacks);
    add_ui(&ATOMIC_COUNTERS.calls, d.calls);
    add_ui(&ATOMIC_COUNTERS.callback_ns, d.callback_ns);
    add_ui(&ATOMIC_COUNTERS.decode_ns, d.decode_ns);
    add_ui(&ATOMIC_COUNTERS.decodes, d.decodes);
    add_ui(&ATOMIC_COUNTERS.tapes, d.tapes);
    add_ui(&ATOMIC_COUNTERS.tape_bytes, d.tape_bytes);
    add_ui(&ATOMIC_COUNTERS.frame_turns, d.frame_turns);
    add_ui(&ATOMIC_COUNTERS.views_turns, d.views_turns);
    add_ui(&ATOMIC_COUNTERS.event_turns, d.event_turns);
    add_ui(&ATOMIC_COUNTERS.wake_turns, d.wake_turns);
    add_ui(&ATOMIC_COUNTERS.deferred_ops, d.deferred_ops);
    add_ui(&ATOMIC_COUNTERS.rows_turns, d.rows_turns);
    add_ui(&ATOMIC_COUNTERS.rows_missing, d.rows_missing);
    add_ui(&ATOMIC_COUNTERS.placeholder_frames, d.placeholder_frames);
}

/// Returns a relaxed snapshot of all global runtime counters.
pub fn counters() -> Counters {
    Counters {
        callbacks: ATOMIC_COUNTERS.callbacks.load(Ordering::Relaxed),
        calls: ATOMIC_COUNTERS.calls.load(Ordering::Relaxed),
        callback_ns: ATOMIC_COUNTERS.callback_ns.load(Ordering::Relaxed),
        decode_ns: ATOMIC_COUNTERS.decode_ns.load(Ordering::Relaxed),
        decodes: ATOMIC_COUNTERS.decodes.load(Ordering::Relaxed),
        tapes: ATOMIC_COUNTERS.tapes.load(Ordering::Relaxed),
        tape_bytes: ATOMIC_COUNTERS.tape_bytes.load(Ordering::Relaxed),
        frame_turns: ATOMIC_COUNTERS.frame_turns.load(Ordering::Relaxed),
        views_turns: ATOMIC_COUNTERS.views_turns.load(Ordering::Relaxed),
        event_turns: ATOMIC_COUNTERS.event_turns.load(Ordering::Relaxed),
        wake_turns: ATOMIC_COUNTERS.wake_turns.load(Ordering::Relaxed),
        deferred_ops: ATOMIC_COUNTERS.deferred_ops.load(Ordering::Relaxed),
        rows_turns: ATOMIC_COUNTERS.rows_turns.load(Ordering::Relaxed),
        rows_missing: ATOMIC_COUNTERS.rows_missing.load(Ordering::Relaxed),
        placeholder_frames: ATOMIC_COUNTERS.placeholder_frames.load(Ordering::Relaxed),
    }
}

/// Returns cumulative CPU time consumed by the calling thread.
#[cfg(unix)]
pub fn thread_cpu() -> Duration {
    let mut t = std::mem::MaybeUninit::<libc::timespec>::uninit();
    // SAFETY: `t.as_mut_ptr()` is a valid, aligned pointer to a writable `libc::timespec`,
    // and `assume_init` is only called after `clock_gettime` returns 0 (success).
    let t = unsafe {
        if libc::clock_gettime(libc::CLOCK_THREAD_CPUTIME_ID, t.as_mut_ptr()) != 0 {
            return Duration::ZERO;
        }
        t.assume_init()
    };
    Duration::new(t.tv_sec as u64, t.tv_nsec as u32)
}

/// Returns cumulative kernel plus user CPU time consumed by the calling thread.
#[cfg(windows)]
pub fn thread_cpu() -> Duration {
    #[repr(C)]
    #[derive(Clone, Copy)]
    struct FileTime {
        low: u32,
        high: u32,
    }
    // SAFETY: Standard Win32 kernel32 signatures matching Microsoft's C ABI.
    #[link(name = "kernel32")]
    unsafe extern "system" {
        fn GetCurrentThread() -> *mut std::ffi::c_void;
        fn GetThreadTimes(
            thread: *mut std::ffi::c_void,
            creation: *mut FileTime,
            exit: *mut FileTime,
            kernel: *mut FileTime,
            user: *mut FileTime,
        ) -> i32;
    }
    let z = FileTime { low: 0, high: 0 };
    let (mut c, mut e, mut k, mut u) = (z, z, z, z);
    // SAFETY: `GetCurrentThread` returns a valid pseudo-handle and all four out-pointers
    // refer to valid stack-allocated `FileTime` values.
    if unsafe { GetThreadTimes(GetCurrentThread(), &mut c, &mut e, &mut k, &mut u) } == 0 {
        return Duration::ZERO;
    }
    let ticks = |t: FileTime| (u64::from(t.high) << 32) | u64::from(t.low);
    Duration::from_nanos((ticks(k) + ticks(u)) * 100)
}

fn env_nums(name: &str) -> Option<Vec<f64>> {
    let v = std::env::var(name).ok()?;
    v.split(',').map(|p| p.trim().parse::<f64>().ok()).collect()
}

/// Builds `WindowOptions` using headless dimensions when active, `GK_SIZE` when set,
/// or the caller's default `width` and `height`.
pub fn window_options(width: f32, height: f32, cx: &App) -> WindowOptions {
    if let Some((w, h)) = HEADLESS_SIZE.with(|s| s.get()) {
        return WindowOptions {
            window_bounds: Some(WindowBounds::Windowed(Bounds {
                origin: Default::default(),
                size: size(px(w), px(h)),
            })),
            focus: false,
            show: false,
            ..Default::default()
        };
    }
    let (w, h) = match env_nums("GK_SIZE").as_deref() {
        Some([w, h]) => (*w as f32, *h as f32),
        _ => (width, height),
    };
    WindowOptions {
        focus: true,
        window_bounds: Some(WindowBounds::Windowed(Bounds::centered(
            None,
            size(px(w), px(h)),
            cx,
        ))),
        inactive_frame_interval: None,
        ..Default::default()
    }
}

#[derive(Default, Clone, Copy)]
struct Layout {
    views_built: u64,
    views_reused: u64,
    build_ns: u64,
    prepaint_ns: u64,
    paint_ns: u64,
}

#[cfg(feature = "diag")]
fn layout(window: &Window) -> Layout {
    let s = window.layout_stats();
    Layout {
        views_built: s.views_built,
        views_reused: s.views_reused,
        build_ns: s.build_time.as_nanos() as u64,
        prepaint_ns: s.prepaint_time.as_nanos() as u64,
        paint_ns: s.paint_time.as_nanos() as u64,
    }
}

#[cfg(not(feature = "diag"))]
fn layout(_: &Window) -> Layout {
    Layout::default()
}

#[cfg(feature = "diag")]
fn retention(window: &Window) -> &'static str {
    if window.view_retention() { "on" } else { "off" }
}

#[cfg(not(feature = "diag"))]
fn retention(_: &Window) -> &'static str {
    "unknown"
}

struct Row {
    frame: usize,
    cpu_ns: u64,
    interval_ns: u64,
    c: Counters,
    l: Layout,
    click: bool,
}

struct Meter {
    warmup: usize,
    measured: usize,
    frame: usize,
    last_cpu: Duration,
    last_wall: Instant,
    last_c: Counters,
    last_l: Layout,
    last_click: bool,
    rows: Vec<Row>,
    click: Option<(f32, f32, usize, usize)>,
    scroll: Option<(f32, f32, f32)>,
    root: Option<EntityId>,
}

/// Attaches a per-frame timing harness to `window` when `GK_FRAMES` is set in the environment.
///
/// Optional environment variables: `GK_FRAMES=warmup,measured`, `GK_CLICK=x,y,at[,every]`,
/// `GK_SCROLL=x,y,dy`, `GK_LABEL`, and `GK_STATS=<csv-path>`.
pub fn attach(window: &mut Window, _cx: &mut App) {
    let Some(frames) = env_nums("GK_FRAMES") else {
        return;
    };
    let (warmup, measured) = match frames.as_slice() {
        [w, m] => (*w as usize, *m as usize),
        [m] => (60, *m as usize),
        _ => (60, 300),
    };
    let click = env_nums("GK_CLICK").and_then(|c| match c.as_slice() {
        [x, y, f] => Some((*x as f32, *y as f32, *f as usize, 0)),
        [x, y, f, e] => Some((*x as f32, *y as f32, *f as usize, *e as usize)),
        _ => None,
    });
    let meter = Rc::new(RefCell::new(Meter {
        warmup,
        measured,
        frame: 0,
        last_cpu: Duration::ZERO,
        last_wall: Instant::now(),
        last_c: Counters::default(),
        last_l: Layout::default(),
        last_click: false,
        rows: Vec::with_capacity(measured),
        click,
        scroll: env_nums("GK_SCROLL").and_then(|s| match s.as_slice() {
            [x, y, dy] => Some((*x as f32, *y as f32, *dy as f32)),
            _ => None,
        }),
        root: None,
    }));
    window.on_next_frame(move |window, cx| tick(meter, window, cx));
}

fn tick(meter: Rc<RefCell<Meter>>, window: &mut Window, cx: &mut App) {
    let cpu = thread_cpu();
    let wall = Instant::now();
    let c = counters();
    let l = layout(window);
    let mut click_at = None;
    let mut scroll_by = None;
    let done = {
        let mut m = meter.borrow_mut();
        if m.frame > m.warmup {
            let row = Row {
                frame: m.frame,
                cpu_ns: (cpu - m.last_cpu).as_nanos() as u64,
                interval_ns: (wall - m.last_wall).as_nanos() as u64,
                c: delta(c, m.last_c),
                l: Layout {
                    views_built: l.views_built - m.last_l.views_built,
                    views_reused: l.views_reused - m.last_l.views_reused,
                    build_ns: l.build_ns - m.last_l.build_ns,
                    prepaint_ns: l.prepaint_ns - m.last_l.prepaint_ns,
                    paint_ns: l.paint_ns - m.last_l.paint_ns,
                },
                click: m.last_click,
            };
            m.rows.push(row);
        }
        m.frame += 1;
        m.last_click = false;
        if let Some((x, y, at, every)) = m.click {
            if m.frame == at || (every > 0 && m.frame > at && (m.frame - at) % every == 0) {
                click_at = Some(point(px(x), px(y)));
                m.last_click = true;
            }
        }
        if let Some((x, y, dy)) = m.scroll {
            scroll_by = Some((point(px(x), px(y)), dy));
        }
        if m.root.is_none() {
            // Cache the gpui-kit Root entity ID so we can notify it directly instead of full window refresh.
            m.root = window
                .root::<gpui_kit::base::Root>()
                .flatten()
                .map(|root| root.entity_id());
        }
        m.rows.len() >= m.measured
    };
    if done {
        report(&meter.borrow(), window);
        cx.quit();
        return;
    }
    if let Some(position) = click_at {
        // Defer synthetic input slightly so it arrives outside the current frame callback.
        window
            .spawn(cx, async move |cx| {
                cx.background_executor().timer(Duration::from_millis(5)).await;
                let _ = cx.update(|window, cx| click(window, position, cx));
            })
            .detach();
    }
    if let Some((position, dy)) = scroll_by {
        window
            .spawn(cx, async move |cx| {
                cx.background_executor().timer(Duration::from_millis(5)).await;
                let _ = cx.update(|window, cx| scroll(window, position, dy, cx));
            })
            .detach();
    }
    if let Some(root) = meter.borrow().root {
        cx.notify(root);
    } else {
        window.refresh();
    }
    {
        // Re-sample thread CPU after bookkeeping so the meter's own work is excluded from next frame's delta.
        let mut m = meter.borrow_mut();
        m.last_cpu = thread_cpu();
        m.last_wall = wall;
        m.last_c = c;
        m.last_l = l;
    }
    window.on_next_frame(move |window, cx| tick(meter, window, cx));
}

fn delta(a: Counters, b: Counters) -> Counters {
    Counters {
        callbacks: a.callbacks - b.callbacks,
        calls: a.calls - b.calls,
        callback_ns: a.callback_ns - b.callback_ns,
        decode_ns: a.decode_ns - b.decode_ns,
        decodes: a.decodes - b.decodes,
        tapes: a.tapes - b.tapes,
        tape_bytes: a.tape_bytes - b.tape_bytes,
        frame_turns: a.frame_turns - b.frame_turns,
        views_turns: a.views_turns - b.views_turns,
        event_turns: a.event_turns - b.event_turns,
        wake_turns: a.wake_turns - b.wake_turns,
        deferred_ops: a.deferred_ops - b.deferred_ops,
        rows_turns: a.rows_turns - b.rows_turns,
        rows_missing: a.rows_missing - b.rows_missing,
        placeholder_frames: a.placeholder_frames - b.placeholder_frames,
    }
}

/// Dispatches a synthetic left click (move, down, up) at `position`.
pub fn click(window: &mut Window, position: Point<Pixels>, cx: &mut App) {
    let modifiers = Modifiers::default();
    window.dispatch_event(
        gpui_kit::PlatformInput::MouseMove(MouseMoveEvent {
            position,
            pressed_button: None,
            modifiers,
        }),
        cx,
    );
    window.dispatch_event(
        gpui_kit::PlatformInput::MouseDown(MouseDownEvent {
            button: MouseButton::Left,
            position,
            modifiers,
            click_count: 1,
            first_mouse: false,
        }),
        cx,
    );
    window.dispatch_event(
        gpui_kit::PlatformInput::MouseUp(MouseUpEvent {
            button: MouseButton::Left,
            position,
            modifiers,
            click_count: 1,
        }),
        cx,
    );
}

fn peak_rss_kb() -> u64 {
    std::fs::read_to_string("/proc/self/status")
        .ok()
        .and_then(|s| {
            s.lines()
                .find(|l| l.starts_with("VmHWM:"))
                .and_then(|l| l.split_whitespace().nth(1)?.parse().ok())
        })
        .unwrap_or(0)
}

/// Prints the `gk-meter ...` summary line (CPU percentiles, frame-interval deadline misses,
/// per-frame turn/decode/layout averages, and Linux `VmHWM` peak RSS) and optionally writes
/// per-frame CSV rows to `GK_STATS`.
fn report(m: &Meter, window: &Window) {
    let n = m.rows.len().max(1) as f64;
    let mut cpu: Vec<u64> = m.rows.iter().map(|r| r.cpu_ns).collect();
    let mut gap: Vec<u64> = m.rows.iter().map(|r| r.interval_ns).collect();
    cpu.sort();
    gap.sort();
    let pct = |v: &[u64], p: f64| {
        v.get(((v.len() as f64 - 1.) * p).round() as usize)
            .copied()
            .unwrap_or(0) as f64
    };
    let sum = |f: &dyn Fn(&Row) -> u64| m.rows.iter().map(f).sum::<u64>() as f64;
    let max = |f: &dyn Fn(&Row) -> u64| m.rows.iter().map(f).max().unwrap_or(0);
    let label = std::env::var("GK_LABEL").unwrap_or_default();
    println!(
        "gk-meter label={label} frames={} retention={} cpu_mean_us={:.1} cpu_p50_us={:.1} \
         cpu_p95_us={:.1} interval_p50_ms={:.3} interval_p95_ms={:.3} over16={} over8={} over25={} \
         callbacks_per_frame={:.3} calls_per_frame={:.3} callback_us_per_frame={:.2} \
         decode_us_per_frame={:.2} decodes_per_frame={:.3} tapes_per_frame={:.3} \
         tape_bytes_per_frame={:.0} frame_turns={} views_turns={} max_views_turns_in_a_frame={} \
         event_turns={} wake_turns={} deferred_ops={} views_built_per_frame={:.3} \
         views_reused_per_frame={:.3} build_us={:.1} prepaint_us={:.1} paint_us={:.1} \
         rss_peak_kb={} rows_turns={} rows_missing={} placeholder_frames={}",
        m.rows.len(),
        retention(window),
        sum(&|r| r.cpu_ns) / n / 1e3,
        pct(&cpu, 0.5) / 1e3,
        pct(&cpu, 0.95) / 1e3,
        pct(&gap, 0.5) / 1e6,
        pct(&gap, 0.95) / 1e6,
        gap.iter().filter(|g| **g > 16_700_000).count(),
        gap.iter().filter(|g| **g > 8_300_000).count(),
        gap.iter().filter(|g| **g > 25_000_000).count(),
        sum(&|r| r.c.callbacks) / n,
        sum(&|r| r.c.calls) / n,
        sum(&|r| r.c.callback_ns) / n / 1e3,
        sum(&|r| r.c.decode_ns) / n / 1e3,
        sum(&|r| r.c.decodes) / n,
        sum(&|r| r.c.tapes) / n,
        sum(&|r| r.c.tape_bytes) / n,
        sum(&|r| r.c.frame_turns),
        sum(&|r| r.c.views_turns),
        max(&|r| r.c.views_turns),
        sum(&|r| r.c.event_turns),
        sum(&|r| r.c.wake_turns),
        sum(&|r| r.c.deferred_ops),
        sum(&|r| r.l.views_built) / n,
        sum(&|r| r.l.views_reused) / n,
        sum(&|r| r.l.build_ns) / n / 1e3,
        sum(&|r| r.l.prepaint_ns) / n / 1e3,
        sum(&|r| r.l.paint_ns) / n / 1e3,
        peak_rss_kb(),
        sum(&|r| r.c.rows_turns),
        sum(&|r| r.c.rows_missing),
        sum(&|r| r.c.placeholder_frames),
    );
    let Ok(path) = std::env::var("GK_STATS") else {
        return;
    };
    let mut out = String::from(
        "frame,cpu_ns,interval_ns,click,callbacks,calls,callback_ns,decode_ns,decodes,tapes,\
         tape_bytes,frame_turns,views_turns,event_turns,wake_turns,deferred_ops,views_built,\
         views_reused,build_ns,prepaint_ns,paint_ns,rows_turns,rows_missing,placeholder_frames\n",
    );
    for r in &m.rows {
        let _ = writeln!(
            out,
            "{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{}",
            r.frame,
            r.cpu_ns,
            r.interval_ns,
            r.click as u8,
            r.c.callbacks,
            r.c.calls,
            r.c.callback_ns,
            r.c.decode_ns,
            r.c.decodes,
            r.c.tapes,
            r.c.tape_bytes,
            r.c.frame_turns,
            r.c.views_turns,
            r.c.event_turns,
            r.c.wake_turns,
            r.c.deferred_ops,
            r.l.views_built,
            r.l.views_reused,
            r.l.build_ns,
            r.l.prepaint_ns,
            r.l.paint_ns,
            r.c.rows_turns,
            r.c.rows_missing,
            r.c.placeholder_frames,
        );
    }
    if let Err(e) = std::fs::write(&path, out) {
        eprintln!("gk-meter: cannot write {path}: {e}");
    }
}

pub(crate) fn set_headless_size(w: f32, h: f32) {
    HEADLESS_SIZE.with(|s| s.set(Some((w, h))));
}

/// Dispatches a vertical pixel scroll event at `position` with delta `dy`.
pub fn scroll(window: &mut Window, position: Point<Pixels>, dy: f32, cx: &mut App) {
    window.dispatch_event(
        gpui_kit::PlatformInput::Scroll(gpui_kit::ScrollEvent {
            position,
            delta: gpui_kit::ScrollDelta::Pixels(point(px(0.), px(dy))),
            ..Default::default()
        }),
        cx,
    );
}

/// Dispatches a synthetic left double-click (click count 1 followed by click count 2) at `position`.
pub fn double_click(window: &mut Window, position: Point<Pixels>, cx: &mut App) {
    click(window, position, cx);
    let modifiers = Modifiers::default();
    window.dispatch_event(
        gpui_kit::PlatformInput::MouseDown(MouseDownEvent {
            button: MouseButton::Left,
            position,
            modifiers,
            click_count: 2,
            first_mouse: false,
        }),
        cx,
    );
    window.dispatch_event(
        gpui_kit::PlatformInput::MouseUp(MouseUpEvent {
            button: MouseButton::Left,
            position,
            modifiers,
            click_count: 2,
        }),
        cx,
    );
}

/// Dispatches one phase of a left-button drag from `from` to `to` over `steps` intermediate moves.
///
/// Phase 0 moves and presses at `from`, phases `1..=steps` interpolate pointer movement with the
/// left button held, and any higher phase releases at `to`.
pub fn drag(
    window: &mut Window,
    from: Point<Pixels>,
    to: Point<Pixels>,
    phase: usize,
    steps: usize,
    cx: &mut App,
) {
    let modifiers = Modifiers::default();
    if phase == 0 {
        window.dispatch_event(
            gpui_kit::PlatformInput::MouseMove(MouseMoveEvent { position: from, pressed_button: None, modifiers }),
            cx,
        );
        window.dispatch_event(
            gpui_kit::PlatformInput::MouseDown(MouseDownEvent {
                button: MouseButton::Left,
                position: from,
                modifiers,
                click_count: 1,
                first_mouse: false,
            }),
            cx,
        );
    } else if phase <= steps {
        let t = phase as f32 / steps as f32;
        let position = point(from.x + (to.x - from.x) * t, from.y + (to.y - from.y) * t);
        window.dispatch_event(
            gpui_kit::PlatformInput::MouseMove(MouseMoveEvent {
                position,
                pressed_button: Some(MouseButton::Left),
                modifiers,
            }),
            cx,
        );
    } else {
        window.dispatch_event(
            gpui_kit::PlatformInput::MouseUp(MouseUpEvent {
                button: MouseButton::Left,
                position: to,
                modifiers,
                click_count: 1,
            }),
            cx,
        );
    }
}

/// Dispatches a synthetic right click (move, down, up) at `position`.
pub fn right_click(window: &mut Window, position: Point<Pixels>, cx: &mut App) {
    for input in right_click_inputs(position) {
        window.dispatch_event(input, cx);
    }
}

/// Builds the move, right-button-down, and right-button-up events for a right click at `position`.
pub fn right_click_inputs(position: Point<Pixels>) -> [gpui_kit::PlatformInput; 3] {
    let modifiers = Modifiers::default();
    [
        gpui_kit::PlatformInput::MouseMove(MouseMoveEvent { position, pressed_button: None, modifiers }),
        gpui_kit::PlatformInput::MouseDown(MouseDownEvent {
            button: MouseButton::Right,
            position,
            modifiers,
            click_count: 1,
            first_mouse: false,
        }),
        gpui_kit::PlatformInput::MouseUp(MouseUpEvent {
            button: MouseButton::Right,
            position,
            modifiers,
            click_count: 1,
        }),
    ]
}
