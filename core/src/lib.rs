//! gk-capi: GPUI-kit on gpui-fast behind a C interface of seven functions and one callback
//! (docs/DESIGN.md section 5, docs/PROTOCOLS.md). The protocol runtime is hand-written
//! (rt.rs, genapi.rs, calls.rs); element and call ops come from the generator (src/gen/,
//! written by `make gen`) and, for the few things it leaves to a bridge, from hand.rs.

// GPUI's proc macros emit `gpui::` paths; the kit resolves them through this alias.
extern crate gpui_kit as gpui;

// The engine is gpui-fast (the default `gpui-fast` feature) or upstream GPUI (gpui-pre, with
// `--no-default-features`). `gk_info` reports which one in its feature bits and version string.

mod app;
mod calls;
// The data protocol: rows of the app's data for charts and plots (docs/PROTOCOLS.md section 12).
mod data;
// A plot described as a list of parts (docs/PROTOCOLS.md section 12).
mod plot;
pub mod genapi;
// The generator's output (docs/PROTOCOLS.md section 7). `gen` is a reserved word in the
// 2024 edition, hence the path.
#[path = "gen/mod.rs"]
mod generated;
mod hand;
mod items; // item procedures: element closures over handed elements (docs/PROTOCOLS.md 9.41)
#[cfg(feature = "all")]
mod keep;
mod objects;
mod overlay; // view-built closures, described values (docs/PROTOCOLS.md 11.6)
mod dock; // dock panels as app views (docs/PROTOCOLS.md 11.7)
mod editor_text; // the editor's runtime (docs/PROTOCOLS.md 9.20 to 9.26)
mod ops;
mod rt;
mod shim_editor; // the editor's providers (docs/PROTOCOLS.md 9.11, 9.12)
mod shim_input;
mod shim_rows;
mod shim_select; // select and combobox over a row source (docs/PROTOCOLS.md 9.9)
mod shim_text; // data closures answered between frames (docs/PROTOCOLS.md 9.14)
mod source;
mod tape;
mod values; // results, maps and kit values that travel to the app (docs/PROTOCOLS.md 13.8 to 13.12)
mod platform; // requests that answer later, dialogs, menus, URLs (docs/PROTOCOLS.md 11.30)
mod motion; // animations and motion values (docs/PROTOCOLS.md 13.30)
mod views_after; // observations delivered after the frame (docs/PROTOCOLS.md 11.14)
mod views_menu; // PopupMenu::submenu_with_icon (docs/PROTOCOLS.md 11.16)
mod wire;

use futures::StreamExt;
use futures::channel::mpsc::{UnboundedSender, unbounded};
use ops::*;
use rt::{GkCallback, GkCb, GkErr, POISONED};
use std::cell::RefCell;
use std::ffi::c_void;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::sync::Mutex;
use std::sync::atomic::{AtomicBool, Ordering};

const MAGIC: u32 = 0x314b_5047;

#[repr(C)]
pub struct GkInfo {
    size: u32,
    magic: u32,
    abi_major: u16,
    abi_minor: u16,
    api_hash: u64,
    features: u32,
    versions: *const u8,
    versions_len: usize,
}

// SAFETY: the only pointer is to a static string.
unsafe impl Sync for GkInfo {}

#[repr(C)]
pub struct GkApp {
    size: u32,
    abi_major: u16,
    abi_minor: u16,
    api_hash: u64,
    callback: Option<GkCallback>,
    user: *mut c_void,
    flags: u32,
}

#[cfg(feature = "gpui-fast")]
macro_rules! engine { () => { "gpui-fast 0.1.19" } }
#[cfg(not(feature = "gpui-fast"))]
macro_rules! engine { () => { "gpui-pre 0.3.14" } }

const VERSIONS: &str = concat!(
    "gk-capi ",
    env!("CARGO_PKG_VERSION"),
    "; gpui-kit 0.11.0; ",
    engine!(),
    "; rustc 1.99.1"
);

static INFO: GkInfo = GkInfo {
    size: std::mem::size_of::<GkInfo>() as u32,
    magic: MAGIC,
    abi_major: ABI_MAJOR,
    abi_minor: ABI_MINOR,
    api_hash: API_HASH,
    features: (if gpui_kit::GPUI_FAST { FEAT_GPUI_FAST } else { 0 })
        | (if cfg!(feature = "diag") { FEAT_DIAG } else { 0 })
        | (if cfg!(feature = "all") { FEAT_ALL } else { 0 }),
    versions: VERSIONS.as_ptr(),
    versions_len: VERSIONS.len(),
};

static WAKE: Mutex<Option<UnboundedSender<()>>> = Mutex::new(None);
static WAKE_PENDING: AtomicBool = AtomicBool::new(false);
static QUIT_SOON: AtomicBool = AtomicBool::new(false);

/// Ends the app from where no `App` is in hand (a panic caught outside a call): the wake task
/// quits it at its next turn, and the offscreen driver stops after the line it runs.
pub(crate) fn quit_soon() {
    QUIT_SOON.store(true, Ordering::SeqCst);
    #[cfg(feature = "diag")]
    gk_meter::headless::stop();
    if let Some(tx) = WAKE.lock().ok().and_then(|w| w.clone()) {
        let _ = tx.unbounded_send(());
    }
}

thread_local! {
    static LAST_ERROR: RefCell<String> = const { RefCell::new(String::new()) };
    static RESULT: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) };
}

fn fail(code: i32, msg: impl Into<String>) -> i32 {
    LAST_ERROR.with(|e| *e.borrow_mut() = msg.into());
    code
}

/// Any thread.
#[unsafe(no_mangle)]
pub extern "C" fn gk_info_get() -> *const GkInfo {
    &INFO
}

/// Thread-local; valid until the next failing export on this thread.
#[unsafe(no_mangle)]
pub extern "C" fn gk_last_error(len: *mut usize) -> *const u8 {
    LAST_ERROR.with(|e| {
        let e = e.borrow();
        if !len.is_null() {
            // SAFETY: the caller passed a valid pointer or null.
            unsafe { *len = e.len() };
        }
        e.as_ptr()
    })
}

fn check_app(app: *const GkApp) -> Result<(GkCallback, *mut c_void), i32> {
    if app.is_null() {
        return Err(fail(ERR_VERSION, "gk_run: null gk_app"));
    }
    // SAFETY: the caller passes a valid gk_app; `size` is its first field.
    let size = unsafe { (*app).size } as usize;
    if size != std::mem::size_of::<GkApp>() {
        return Err(fail(
            ERR_VERSION,
            format!(
                "the binding passed a gk_app of {size} bytes, this library (ABI {ABI_MAJOR}.{ABI_MINOR}) \
                 expects {}: the binding and the library are from different versions",
                std::mem::size_of::<GkApp>()
            ),
        ));
    }
    // SAFETY: the size matches.
    let app = unsafe { &*app };
    // An app built against an older minor of the same major is accepted once the major is
    // above 0. While it is 0 the interface hash must be equal as well.
    if app.abi_major != ABI_MAJOR || app.abi_minor > ABI_MINOR || app.api_hash != API_HASH {
        return Err(fail(
            ERR_VERSION,
            format!(
                "the binding and the library are from different versions: the binding was \
                 generated for ABI {}.{} with interface hash {:#x}, this library is ABI {}.{} \
                 with interface hash {:#x} ({})",
                app.abi_major, app.abi_minor, app.api_hash, ABI_MAJOR, ABI_MINOR, API_HASH, VERSIONS
            ),
        ));
    }
    match app.callback {
        Some(cb) => Ok((cb, app.user)),
        None => Err(fail(ERR_VERSION, "gk_app.callback is null")),
    }
}

/// What both ways of running share: the wake channel and the INIT turn.
fn start(cx: &mut gpui_kit::App) {
    // The wake: gk_post, from any thread, sends on this channel; one main-thread task turns
    // any number of posts into one WAKE turn.
    let (tx, mut rx) = unbounded::<()>();
    *WAKE.lock().unwrap() = Some(tx);
    cx.spawn(async move |cx| {
        while rx.next().await.is_some() {
            WAKE_PENDING.store(false, Ordering::SeqCst);
            if QUIT_SOON.load(Ordering::SeqCst) {
                let _ = cx.update(|cx| cx.quit());
                continue;
            }
            let _ = cx.update(|cx| {
                rt::guard("running a WAKE turn", || {
                    // The posted procedures run with the app's first window in scope, as a queued
                    // event does, so they may open a dialog or push a notification (PROTOCOLS.md
                    // 11.33); before a window is open, without one.
                    let first = rt::with(|s| s.windows.first().and_then(|wh| s.window(*wh).ok()));
                    let ran = first.is_some_and(|w| {
                        w.update(cx, |_, window, cx| {
                            let mut reply = rt::turn(K_WAKE, &[], Some(window), cx);
                            rt::process_reply(&mut reply, K_WAKE, Some(window), cx);
                        })
                        .is_ok()
                    });
                    if !ran {
                        let mut reply = rt::turn(K_WAKE, &[], None, cx);
                        rt::process_reply(&mut reply, K_WAKE, None, cx);
                    }
                });
            });
        }
    })
    .detach();
    app::install(cx);
    views_after::install(cx); // observations (docs/PROTOCOLS.md 11.14)
    let mut reply = rt::turn(K_INIT, &[], None, cx);
    rt::process_reply(&mut reply, K_INIT, None, cx);
}

fn finish(run: std::thread::Result<Result<(), String>>) -> i32 {
    *WAKE.lock().unwrap() = None;
    if rt::with(|s| s.debug) {
        eprintln!("gk: live handles at exit: {}", rt::with(|s| s.live()));
    }
    match run {
        Ok(Ok(())) => ERR_OK,
        Ok(Err(e)) => fail(ERR_UNSUPPORTED, e),
        Err(e) if e.is::<rt::AppStop>() => ERR_OK, // the binding was told (rt::app_stop)
        Err(_) => {
            POISONED.store(true, Ordering::SeqCst);
            fail(ERR_POISONED, rt::panic_message())
        }
    }
}

/// Blocks; the caller becomes the UI thread. Returns when the app quits.
#[unsafe(no_mangle)]
pub extern "C" fn gk_run(app: *const GkApp) -> i32 {
    let (callback, user) = match check_app(app) {
        Ok(v) => v,
        Err(code) => return code,
    };
    rt::install_panic_hook();
    rt::install(callback, user);
    let run = catch_unwind(AssertUnwindSafe(|| {
        gpui_kit::application()
            .with_assets(gpui_kit::assets::Assets)
            .run(move |cx| {
                gpui_kit::init(cx);
                start(cx);
            });
        Ok(())
    }));
    finish(run)
}

/// The diag variant only: runs the app on gpui's HeadlessAppContext (no display), driven by
/// a gk-meter script (frames, clicks, typed text, PNG captures).
#[unsafe(no_mangle)]
pub extern "C" fn gk_headless_run(app: *const GkApp, script: *const u8, len: usize) -> i32 {
    #[cfg(feature = "diag")]
    {
        let (callback, user) = match check_app(app) {
            Ok(v) => v,
            Err(code) => return code,
        };
        rt::install_panic_hook();
        rt::install(callback, user);
        platform::set_scripted(true); // file dialogs and prompts answered by the test (PROTOCOLS.md 11.30)
        gk_meter::headless::on_answer(platform::answer_line); // or by the script's `answer` lines (11.31)
        // SAFETY: the caller passes `len` readable bytes.
        let script = unsafe { std::slice::from_raw_parts(script, len) };
        let script = String::from_utf8_lossy(script).into_owned();
        let run = catch_unwind(AssertUnwindSafe(|| {
            gk_meter::headless::run_opt(&script, |cx| {
                start(cx);
                // None: the app opened no window (it stopped first, or the script says so).
                let first = rt::with(|s| s.windows.first().copied())?;
                rt::with(|s| s.window(first)).ok()
            })
            .map_err(|e| format!("headless run: {e:#}"))
        }));
        finish(run)
    }
    #[cfg(not(feature = "diag"))]
    {
        let _ = (app, script, len);
        fail(ERR_UNSUPPORTED, "gk_headless_run needs the diag variant of the library")
    }
}

/// UI thread, inside a callback. `*res` is the library's and valid until the next gk_call.
#[unsafe(no_mangle)]
pub extern "C" fn gk_call(
    req: *const u8,
    len: usize,
    res: *mut *const u8,
    res_len: *mut usize,
) -> i32 {
    // A poisoned library refuses every call but QUIT, which a binding sends to end gk_run
    // after the panic (it then reports the panic from gk_run's caller, PROTOCOLS.md 3.4).
    let quit = !req.is_null() && len >= 2 && {
        // SAFETY: the caller passes `len` readable bytes (not null, checked here).
        let b = unsafe { std::slice::from_raw_parts(req, 2) };
        u16::from_le_bytes([b[0], b[1]]) == ops::C_QUIT
    };
    if POISONED.load(Ordering::Relaxed) && !quit {
        return fail(
            ERR_POISONED,
            format!("{} (an earlier panic; the library stays poisoned)", rt::panic_message()),
        );
    }
    if !rt::IS_UI.with(|u| u.get()) {
        return fail(ERR_WRONG_THREAD, "gk_call from a thread that is not the UI thread");
    }
    let Some((cx, window)) = rt::scope() else {
        return fail(ERR_NO_CONTEXT, "gk_call outside a callback");
    };
    gk_meter::bump(|c| c.calls += 1);
    if req.is_null() {
        return fail(ERR_BAD_OP, "gk_call with a null request");
    }
    // SAFETY: the caller passes `len` readable bytes (not null, checked above).
    let req = unsafe { std::slice::from_raw_parts(req, len) };
    let mut out = RESULT.with(|r| std::mem::take(&mut *r.borrow_mut()));
    out.clear();
    let done = catch_unwind(AssertUnwindSafe(|| calls::call(req, window, cx, &mut out, false)));
    let code = match done {
        Ok(Ok(())) => ERR_OK,
        Ok(Err(GkErr(code, msg))) => fail(code, msg),
        // A mistake in app code the call found where it could not return it (rt::app_stop): the
        // binding was told, the library is not poisoned.
        Err(e) if e.is::<rt::AppStop>() => fail(ERR_BAD_OP, "the app is stopping after a mistake in its code"),
        Err(_) => {
            // Nothing unwinds into the app: the panic stops here, the library is poisoned
            // and the binding prints the message with the app's call site and exits.
            POISONED.store(true, Ordering::SeqCst);
            fail(ERR_POISONED, rt::panic_message())
        }
    };
    if !res.is_null() && !res_len.is_null() {
        // SAFETY: valid out-pointers from the caller; the buffer is kept in RESULT below.
        unsafe {
            *res = out.as_ptr();
            *res_len = out.len();
        }
    }
    RESULT.with(|r| *r.borrow_mut() = out);
    code
}

/// Inside the callback that received `cb`. Copies `out` before it returns.
#[unsafe(no_mangle)]
pub extern "C" fn gk_reply(cb: *mut GkCb, out: *const u8, len: usize) -> i32 {
    if cb.is_null() {
        return fail(ERR_NO_CONTEXT, "gk_reply without a callback");
    }
    // A gk_cb is valid only inside the callback that received it: one kept from an earlier
    // callback points into a stack frame that has returned, and writing through it would
    // corrupt memory. Only the running callback's own is accepted.
    if !rt::IS_UI.with(|u| u.get()) || !rt::current_cb(cb) {
        return fail(ERR_NO_CONTEXT, "gk_reply with a gk_cb that is not the running callback's (a gk_cb is valid only inside the callback that received it)");
    }
    if out.is_null() && len > 0 {
        return fail(ERR_BAD_OP, "gk_reply with a null reply of nonzero length");
    }
    // SAFETY: cb is the running callback's (checked above) and live for its duration; out
    // points to `len` readable bytes.
    unsafe {
        let cb = &mut *cb;
        cb.reply.clear();
        if len > 0 {
            cb.reply.extend_from_slice(std::slice::from_raw_parts(out, len));
        }
    }
    ERR_OK
}

/// Any thread. Asks for one WAKE turn on the UI thread; posts made before that turn runs
/// are folded into it. Before `gk_run` has started the loop it returns GK_ERR_NO_CONTEXT;
/// the binding drains what was posted that early at the end of its INIT turn.
#[unsafe(no_mangle)]
pub extern "C" fn gk_post(_token: u64) -> i32 {
    if WAKE_PENDING.swap(true, Ordering::SeqCst) {
        return ERR_OK;
    }
    match WAKE.lock().unwrap().as_ref() {
        Some(tx) => {
            let _ = tx.unbounded_send(());
            ERR_OK
        }
        None => {
            WAKE_PENDING.store(false, Ordering::SeqCst);
            fail(ERR_NO_CONTEXT, "gk_post before gk_run or after it returned")
        }
    }
}
