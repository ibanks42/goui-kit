//! C ABI entry points for `libgpuikit`.
//!
//! Exports the lifecycle and message-passing surface (`gk_info_get`, `gk_last_error`, `gk_run`,
//! `gk_headless_run`, `gk_call`, `gk_reply`, and `gk_post`), validates ABI compatibility, catches
//! panics across the FFI boundary, and bridges background wakes onto the GPUI UI thread.

extern crate gpui_kit as gpui;

mod app;
mod calls;
mod data;
mod plot;
pub mod genapi;

#[path = "gen/mod.rs"]
mod generated;
mod hand;
mod items;
#[cfg(feature = "all")]
mod keep;
mod objects;
mod overlay;
mod dock;
mod editor_text;
mod ops;
mod rt;
mod shim_editor;
mod shim_input;
mod shim_rows;
mod shim_select;
mod shim_text;
mod source;
mod tape;
mod values;
mod platform;
mod motion;
mod views_after;
mod views_menu;
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

/// Little-endian ASCII `"GPK1"` tag identifying [`GkInfo`].
const MAGIC: u32 = 0x314b_5047;

/// Static library metadata returned by [`gk_info_get`].
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

// SAFETY: `GkInfo` is immutable once constructed and its `versions` pointer references
// `'static` string data compiled into the binary.
unsafe impl Sync for GkInfo {}

/// Startup descriptor passed by the host binding to [`gk_run`] and [`gk_headless_run`].
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
        | (if cfg!(feature = "all") { FEAT_ALL } else { 0 })
        | FEAT_INLINE_REPLY,
    versions: VERSIONS.as_ptr(),
    versions_len: VERSIONS.len(),
};

/// Channel sender used by [`gk_post`] and [`quit_soon`] to wake the UI thread's async task.
static WAKE: Mutex<Option<UnboundedSender<()>>> = Mutex::new(None);
/// Coalesces repeated `gk_post` notifications into a single queued wake message.
static WAKE_PENDING: AtomicBool = AtomicBool::new(false);
static QUIT_SOON: AtomicBool = AtomicBool::new(false);

/// Requests an asynchronous shutdown of the GPUI application loop from any thread.
pub(crate) fn quit_soon() {
    QUIT_SOON.store(true, Ordering::SeqCst);
    #[cfg(feature = "diag")]
    gk_meter::headless::stop();
    if let Some(tx) = WAKE.lock().ok().and_then(|w| w.clone()) {
        let _ = tx.unbounded_send(());
    }
}

thread_local! {
    /// Last error message recorded on the calling thread for [`gk_last_error`].
    static LAST_ERROR: RefCell<String> = const { RefCell::new(String::new()) };
    /// Reusable reply buffer backing the output slice returned by [`gk_call`].
    static RESULT: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) };
}

fn fail(code: i32, msg: impl Into<String>) -> i32 {
    LAST_ERROR.with(|e| *e.borrow_mut() = msg.into());
    code
}

/// Returns a pointer to the static [`GkInfo`] describing this library's ABI and features.
///
/// # Safety
/// The symbol is exported unmangled for C FFI; the returned pointer references static read-only
/// memory valid for the lifetime of the loaded library.
#[unsafe(no_mangle)]
pub extern "C" fn gk_info_get() -> *const GkInfo {
    &INFO
}

/// Returns the calling thread's last error message as UTF-8 bytes and optionally writes its length.
///
/// The returned pointer stays valid until the next FFI call on the same thread overwrites it.
///
/// # Safety
/// If `len` is non-null it must point to writable memory aligned for `usize`.
#[unsafe(no_mangle)]
pub extern "C" fn gk_last_error(len: *mut usize) -> *const u8 {
    LAST_ERROR.with(|e| {
        let e = e.borrow();
        if !len.is_null() {
            // SAFETY: Caller passed a non-null writable `usize` out-pointer per the C ABI contract.
            unsafe { *len = e.len() };
        }
        e.as_ptr()
    })
}

/// Validates the host's [`GkApp`] struct size, ABI version, interface hash, and callback pointer.
fn check_app(app: *const GkApp) -> Result<(GkCallback, *mut c_void), i32> {
    if app.is_null() {
        return Err(fail(ERR_VERSION, "gk_run: null gk_app"));
    }
    // SAFETY: `app` is non-null; the first 4 bytes are always `size: u32` across ABI revisions.
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
    // SAFETY: `app` is non-null and its reported byte size matches `GkApp` exactly.
    let app = unsafe { &*app };

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

/// Installs the background-wake task, platform hooks, and runs the host's `K_INIT` turn.
fn start(cx: &mut gpui_kit::App) {
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
                    // Prefer delivering WAKE with the first open window so window-scoped calls succeed.
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
    views_after::install(cx);
    let mut reply = rt::turn(K_INIT, &[], None, cx);
    rt::process_reply(&mut reply, K_INIT, None, cx);
}

/// Clears the wake channel and translates the caught run result into a C ABI status code.
fn finish(run: std::thread::Result<Result<(), String>>) -> i32 {
    *WAKE.lock().unwrap() = None;
    if rt::with(|s| s.debug) {
        eprintln!("gk: live handles at exit: {}", rt::with(|s| s.live()));
    }
    match run {
        Ok(Ok(())) => ERR_OK,
        Ok(Err(e)) => fail(ERR_UNSUPPORTED, e),
        Err(e) if e.is::<rt::AppStop>() => ERR_OK,
        Err(_) => {
            POISONED.store(true, Ordering::SeqCst);
            fail(ERR_POISONED, rt::panic_message())
        }
    }
}

/// Runs a live GPUI application event loop until the app quits or panics.
///
/// # Safety
/// `app` must be null or point to a valid [`GkApp`] struct with `size == sizeof(GkApp)`, and
/// `app.callback` must be a valid C function pointer safe to call with `app.user` on the UI thread.
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

/// Runs the application offscreen under a headless test script (requires the `diag` feature).
///
/// # Safety
/// `app` must satisfy the same requirements as [`gk_run`], and `script` must point to at least
/// `len` readable bytes for the duration of this call (when `diag` is enabled).
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
        platform::set_scripted(true);
        gk_meter::headless::on_answer(platform::answer_line);
        // SAFETY: Caller passes a readable buffer of `len` bytes for the duration of this call.
        let script = unsafe { std::slice::from_raw_parts(script, len) };
        let script = String::from_utf8_lossy(script).into_owned();
        let run = catch_unwind(AssertUnwindSafe(|| {
            gk_meter::headless::run_opt(&script, |cx| {
                start(cx);
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

/// Executes a synchronous wire call from inside a host callback on the UI thread.
///
/// The reply slice written to `*res` and `*res_len` borrows a thread-local buffer that remains
/// valid until the next `gk_call` on this thread.
///
/// # Safety
/// `req` must point to `len` readable bytes when non-null; `res` and `res_len`, when both
/// non-null, must point to writable memory aligned for `*const u8` and `usize` respectively.
#[unsafe(no_mangle)]
pub extern "C" fn gk_call(
    req: *const u8,
    len: usize,
    res: *mut *const u8,
    res_len: *mut usize,
) -> i32 {
    // Allow C_QUIT even when poisoned so the host can exit cleanly after reporting an error.
    let quit = !req.is_null() && len >= 2 && {
        // SAFETY: `req` is non-null and `len >= 2`, so reading the 2-byte opcode header is valid.
        let b = unsafe { std::slice::from_raw_parts(req, 2) };
        u16::from_le_bytes([b[0], b[1]]) == ops::C_QUIT
    };
    if POISONED.load(Ordering::Relaxed) && !quit {
        return fail(
            ERR_POISONED,
            format!("{} (an earlier panic; the library stays poisoned)", rt::panic_message()),
        );
    }
    if !rt::is_ui() {
        return fail(ERR_WRONG_THREAD, "gk_call from a thread that is not the UI thread");
    }
    let Some((cx, window)) = rt::scope() else {
        return fail(ERR_NO_CONTEXT, "gk_call outside a callback");
    };
    gk_meter::bump(|c| c.calls += 1);
    if req.is_null() {
        return fail(ERR_BAD_OP, "gk_call with a null request");
    }
    // SAFETY: `req` is non-null and the C ABI guarantees `len` readable bytes for this call.
    let req = unsafe { std::slice::from_raw_parts(req, len) };
    // Take `RESULT` out of the RefCell so re-entrant callbacks inside `calls::call` do not conflict.
    let mut out = RESULT.with(|r| std::mem::take(&mut *r.borrow_mut()));
    out.clear();
    let done = catch_unwind(AssertUnwindSafe(|| calls::call(req, window, cx, &mut out, false)));
    let code = match done {
        Ok(Ok(())) => ERR_OK,
        Ok(Err(GkErr(code, msg))) => fail(code, msg),
        Err(e) if e.is::<rt::AppStop>() => fail(ERR_BAD_OP, "the app is stopping after a mistake in its code"),
        Err(_) => {
            POISONED.store(true, Ordering::SeqCst);
            fail(ERR_POISONED, rt::panic_message())
        }
    };
    if !res.is_null() && !res_len.is_null() {
        // SAFETY: Both output pointers are non-null and writable per the C ABI contract.
        // Moving `out` back into `RESULT` preserves the heap allocation pointer `out.as_ptr()`.
        unsafe {
            *res = out.as_ptr();
            *res_len = out.len();
        }
    }
    RESULT.with(|r| *r.borrow_mut() = out);
    code
}

/// Copies the host callback's reply payload into the active [`GkCb`] frame.
///
/// # Safety
/// `cb` must be the opaque pointer passed to the currently executing callback on the UI thread,
/// and `out` must point to `len` readable bytes whenever `len > 0`.
#[unsafe(no_mangle)]
pub extern "C" fn gk_reply(cb: *mut GkCb, out: *const u8, len: usize) -> i32 {
    if cb.is_null() {
        return fail(ERR_NO_CONTEXT, "gk_reply without a callback");
    }
    if !rt::is_ui() || !rt::current_cb(cb) {
        return fail(ERR_NO_CONTEXT, "gk_reply with a gk_cb that is not the running callback's (a gk_cb is valid only inside the callback that received it)");
    }
    if out.is_null() && len > 0 {
        return fail(ERR_BAD_OP, "gk_reply with a null reply of nonzero length");
    }
    // SAFETY: `rt::current_cb(cb)` verified `cb` is the live `GkCb` on the UI thread's active
    // turn frame, and `out` is non-null whenever `len > 0`.
    unsafe {
        let cb = &mut *cb;
        cb.inline_ptr = std::ptr::null();
        cb.inline_len = 0;
        if cb.reply.capacity() == 0 && len > 0 {
            cb.reply = rt::take_reply_buf();
        }
        cb.reply.clear();
        if len > 0 {
            cb.reply.extend_from_slice(std::slice::from_raw_parts(out, len));
        }
    }
    ERR_OK
}

/// Wakes the UI thread from any thread so the host receives a `K_WAKE` callback turn.
///
/// Multiple calls before the UI thread drains the channel are coalesced into one wake.
///
/// # Safety
/// Thread-safe C ABI entry point with no pointer arguments; safe to invoke from any thread.
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
