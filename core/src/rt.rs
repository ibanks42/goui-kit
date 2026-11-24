//! Core UI-thread runtime state, generational handle table, turn loop, and view lifecycle.
//!
//! All mutable session state lives in thread-local storage (`UI`) on the GPUI UI thread.
//! Handles pack `(kind: u8 << 56) | (generation: u24 << 32) | (slot_index: u32)` so stale or
//! mistyped handles fail cleanly with `ERR_STALE_HANDLE` rather than aliasing reused slots.

use crate::genapi;
use crate::ops::*;
use crate::tape::Tape;
use crate::wire::{Put, R};
use gpui_kit::{
    AnyWindowHandle, App, AppContext, Context, Entity, FocusHandle, IntoElement, Render,
    SharedString, Subscription, Window, div,
};
use std::any::Any;
use std::cell::{Cell, RefCell};
use std::collections::{HashMap, HashSet};
use std::ffi::c_void;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::rc::Rc;
#[cfg(not(feature = "diag"))]
use std::sync::atomic::AtomicU32;
use std::sync::atomic::{AtomicBool, AtomicU8, AtomicU64, Ordering};
use std::time::Instant;

/// C ABI callback signature implemented by the host binding to receive runtime turns.
///
/// # Safety
/// The host callback must only dereference `cb` and `req[..len]` during the synchronous call,
/// must not unwind across the FFI boundary, and must honor the `FEAT_INLINE_REPLY` lifetime if
/// writing `cb.inline_ptr` directly.
pub type GkCallback = unsafe extern "C" fn(
    user: *mut c_void,
    cb: *mut GkCb,
    kind: u32,
    req: *const u8,
    len: usize,
) -> i32;

/// Per-turn reply context passed as an opaque pointer to [`GkCallback`] and [`crate::gk_reply`].
///
/// Hosts that support `FEAT_INLINE_REPLY` may populate `inline_ptr`/`inline_len` directly in C
/// without copying; otherwise `gk_reply` copies into `reply`.
#[repr(C)]
pub struct GkCb {
    pub inline_ptr: *const u8,
    pub inline_len: usize,
    pub reply: Vec<u8>,
}

impl GkCb {
    #[inline]
    pub fn new() -> Self {
        GkCb {
            inline_ptr: std::ptr::null(),
            inline_len: 0,
            reply: Vec::new(),
        }
    }
}

/// Wire error pair `(ERR_* code, human-readable message)`.
pub struct GkErr(pub i32, pub String);
pub type GkResult<T> = Result<T, GkErr>;

pub fn err<T>(code: i32, msg: impl Into<String>) -> GkResult<T> {
    Err(GkErr(code, msg.into()))
}

/// Backing entity for `N_UNIFORM_LIST` text rows (`HK_ROWS`).
pub struct RowStore {
    pub rows: Rc<RefCell<Vec<SharedString>>>,
}

/// Type-erased GPUI entity entry stored under `HK_STATE`.
pub struct StateItem {
    pub any: Rc<dyn Any>,
    pub type_name: &'static str,
    /// Window active when this entity was created, used as the default update window.
    pub window: Option<AnyWindowHandle>,
    /// Factory that attaches a GPUI observer marking a dependent view dirty on state change.
    pub observe: Rc<dyn Fn(&mut App, u64) -> Subscription>,
}

/// Tagged payload stored in a [`Session`] handle slot.
pub enum Item {
    View(Entity<GkView>),
    /// Row-source views decoded directly without a GPUI entity keep only their observer subscriptions.
    DecodedRow(#[allow(dead_code)] Vec<Subscription>),
    Window(AnyWindowHandle),
    State(StateItem),
    Rows(Entity<RowStore>),
    Sub(#[allow(dead_code)] Subscription),
    Task(#[allow(dead_code)] gpui_kit::Task<()>),
    Guard(#[allow(dead_code)] Box<dyn Any>),
    Focus(FocusHandle, Option<AnyWindowHandle>),
    Source(Box<crate::source::Source>),
    Obj(crate::objects::ObjItem),
}

struct Slot {
    generation: u32,
    kind: u8,
    item: Option<Item>,
}

fn kind_name(kind: u8) -> &'static str {
    match kind {
        HK_VIEW => "view",
        HK_WINDOW => "window",
        HK_STATE => "state",
        HK_ROWS => "rows",
        HK_SUB => "subscription",
        HK_FOCUS => "focus handle",
        HK_SOURCE => "row source",
        HK_OBJ => "library object",
        HK_TASK => "timer",
        _ => "nothing",
    }
}

static TRACE_STATE: AtomicU8 = AtomicU8::new(0);
/// Monotonic counter incremented on every handle release to invalidate [`crate::tape::FastTree`] caches.
static RELEASE_EPOCH: AtomicU64 = AtomicU64::new(0);

#[inline]
pub fn release_epoch() -> u64 {
    RELEASE_EPOCH.load(Ordering::Relaxed)
}

#[inline]
fn trace_on() -> bool {
    match TRACE_STATE.load(Ordering::Relaxed) {
        1 => false,
        2 => true,
        _ => {
            let on = std::env::var("GK_TRACE_TURNS").is_ok_and(|v| v == "1");
            TRACE_STATE.store(if on { 2 } else { 1 }, Ordering::Relaxed);
            on
        }
    }
}

/// Prints a turn trace line to stderr when `GK_TRACE_TURNS=1`.
#[inline]
pub fn trace(what: impl FnOnce() -> String) {
    if trace_on() {
        eprintln!("gk-trace: {}", what());
    }
}

/// GPUI view entity backed by a host-supplied [`Tape`] and optional [`crate::tape::FastTree`] cache.
pub struct GkView {
    pub handle: u64,
    pub tape: Option<Rc<Tape>>,
    deps: Vec<u64>,
    _subs: Vec<Subscription>,
    renders: u32,
    fast_tried: bool,
    fast: Option<crate::tape::FastTree>,
}

impl Render for GkView {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let handle = self.handle;
        genapi::timed_decode(|| {
            let Some(tape) = self.tape.as_ref() else {
                return div().into_any_element();
            };
            let epoch = release_epoch();
            if let Some(fast) = self.fast.as_ref() {
                if fast.epoch == epoch {
                    return guard("decoding a view's tape", || fast.render(cx))
                        .unwrap_or_else(|| div().into_any_element());
                }
                self.fast = None;
                self.fast_tried = false;
            } else if self.renders >= 1 && !self.fast_tried {
                // Build a FastTree only after the first render confirms the tape is reused across frames.
                self.fast_tried = true;
                if let Some(fast) = crate::tape::FastTree::try_build(tape, epoch) {
                    let out = guard("decoding a view's tape", || fast.render(cx))
                        .unwrap_or_else(|| div().into_any_element());
                    self.fast = Some(fast);
                    return out;
                }
            }
            self.renders = self.renders.saturating_add(1);
            guard("decoding a view's tape", || genapi::root(tape, handle, window, cx))
                .unwrap_or_else(|| div().into_any_element())
        })
    }
}

struct Queued {
    req: Vec<u8>,
    window: Option<AnyWindowHandle>,
}

/// Hash map keyed by handle IDs using [`IdHasher`].
pub type IdMap<V> = HashMap<u64, V, std::hash::BuildHasherDefault<IdHasher>>;
/// Hash set of handle IDs using [`IdHasher`].
pub type IdSet = HashSet<u64, std::hash::BuildHasherDefault<IdHasher>>;

/// Fast multiplicative mixer for 64-bit handle keys.
#[derive(Default)]
pub struct IdHasher(u64);

impl std::hash::Hasher for IdHasher {
    fn write(&mut self, bytes: &[u8]) {
        for b in bytes {
            self.0 = (self.0.rotate_left(8) ^ *b as u64).wrapping_mul(0x9e37_79b9_7f4a_7c15);
        }
    }
    fn write_u64(&mut self, v: u64) {
        let x = (self.0 ^ v ^ (v >> 29)).wrapping_mul(0x9e37_79b9_7f4a_7c15);
        self.0 = x ^ (x >> 32);
    }
    fn finish(&self) -> u64 {
        self.0
    }
}

/// Thread-local session holding the handle table, view-to-window routing, dirty queues,
/// and deferred-op buffers for the active `gk_run` call.
pub struct Session {
    pub callback: GkCallback,
    pub user: *mut c_void,
    slots: Vec<Slot>,
    free: Vec<u32>,
    /// Maps each mounted view handle to the window handle that owns it.
    view_window: IdMap<u64>,
    /// Per-window list of views queued for `K_VIEWS` before the next frame.
    pub(crate) dirty: IdMap<Vec<u64>>,
    /// Views marked dirty before being mounted into any window.
    unmounted_dirty: Vec<u64>,
    /// Reverse index from `HK_STATE` handle to views that listed it in their tape `deps`.
    dep_views: IdMap<Vec<u64>>,
    /// Windows that already have a `pre_frame` next-frame hook scheduled.
    hooked: IdSet,
    /// Window currently executing `pre_frame` or `open_window_with`, suppressing duplicate hooks.
    in_hook: Option<u64>,
    pub frame_handler: IdSet,
    /// Buffered write ops deferred until after the current event handler returns.
    deferred: Vec<(Vec<u8>, Option<AnyWindowHandle>)>,
    deferred_scheduled: bool,
    queued: Vec<Queued>,
    queued_scheduled: bool,
    pub windows: Vec<u64>,
    frame_no: u64,
    pub debug: bool,
    pub sources: Vec<u64>,
    /// Maps a row-source view handle to `(source_handle, row_key)`.
    pub row_owner: IdMap<(u64, u64)>,
    /// Dependency list tracked for `Item::DecodedRow` views that have no `GkView` entity.
    pub row_deps: IdMap<Vec<u64>>,
}

/// Active FFI callback frame on the UI thread stack, enabling `gk_call` and `gk_reply`.
#[derive(Clone, Copy)]
struct Scope {
    app: *mut App,
    window: Option<*mut Window>,
    cb: *mut GkCb,
}

struct UiTls {
    session: RefCell<Option<Session>>,
    scopes: RefCell<Vec<Scope>>,
    reply_buf: RefCell<Vec<u8>>,
    event_buf: RefCell<Vec<u8>>,
    retired: RefCell<Vec<(u64, u32)>>,
    quiet_panic: Cell<bool>,
    panic_msg: RefCell<String>,
    is_ui: Cell<bool>,
}

thread_local! {
    static UI: UiTls = const {
        UiTls {
            session: RefCell::new(None),
            scopes: RefCell::new(Vec::new()),
            reply_buf: RefCell::new(Vec::new()),
            event_buf: RefCell::new(Vec::new()),
            retired: RefCell::new(Vec::new()),
            quiet_panic: Cell::new(false),
            panic_msg: RefCell::new(String::new()),
            is_ui: Cell::new(false),
        }
    };
}

#[cfg(not(feature = "diag"))]
static PICK: AtomicU32 = AtomicU32::new(0x9e37_79b9);

/// Returns `true` when called on the GPUI UI thread initialized by [`install`].
#[inline]
pub fn is_ui() -> bool {
    UI.with(|u| u.is_ui.get())
}

/// Fast xorshift32 PRNG used for low-overhead decode timing sampling in release builds.
#[cfg(not(feature = "diag"))]
#[inline(always)]
pub fn next_pick() -> u32 {
    let mut x = PICK.load(Ordering::Relaxed);
    x ^= x << 13;
    x ^= x >> 17;
    x ^= x << 5;
    PICK.store(x, Ordering::Relaxed);
    x
}

/// Takes the recycled reply buffer from UI thread-local storage for [`crate::gk_reply`].
pub fn take_reply_buf() -> Vec<u8> {
    UI.with(|u| std::mem::take(&mut *u.reply_buf.borrow_mut()))
}

/// Set once an unexpected Rust panic crosses an FFI or turn boundary; blocks further `gk_call`s.
pub static POISONED: AtomicBool = AtomicBool::new(false);

/// Initializes the UI-thread [`Session`] with slot 0 reserved as the permanent invalid/zero handle.
pub fn install(callback: GkCallback, user: *mut c_void) {
    let debug = std::env::var_os("GK_DEBUG").is_some();
    UI.with(|u| {
        u.is_ui.set(true);
        *u.session.borrow_mut() = Some(Session {
            callback,
            user,
            slots: vec![Slot {
                generation: 0,
                kind: 0,
                item: None,
            }],
            free: Vec::new(),
            view_window: IdMap::default(),
            dirty: IdMap::default(),
            unmounted_dirty: Vec::new(),
            dep_views: IdMap::default(),
            hooked: IdSet::default(),
            in_hook: None,
            frame_handler: IdSet::default(),
            deferred: Vec::new(),
            deferred_scheduled: false,
            queued: Vec::new(),
            queued_scheduled: false,
            windows: Vec::new(),
            frame_no: 0,
            debug,
            sources: Vec::new(),
            row_owner: IdMap::default(),
            row_deps: IdMap::default(),
        });
    });
}

/// Runs `f` with mutable access to the active UI-thread [`Session`].
#[inline]
pub fn with<T>(f: impl FnOnce(&mut Session) -> T) -> T {
    UI.with(|u| {
        f(u.session
            .borrow_mut()
            .as_mut()
            .expect("gk: no session; call gk_run first"))
    })
}

/// 24-bit generation mask stored in handle bits `32..56` (generation 0 is never issued).
const GEN_MASK: u32 = 0xff_ffff;

impl Session {
    /// Allocates a new handle of `kind` without storing its payload yet (for cyclic self-references
    /// like `GkView` storing its own handle before `put`).
    pub fn reserve(&mut self, kind: u8) -> u64 {
        let index = match self.free.pop() {
            Some(index) => {
                self.slots[index as usize].kind = kind;
                index
            }
            None => {
                self.slots.push(Slot {
                    generation: 1,
                    kind,
                    item: None,
                });
                (self.slots.len() - 1) as u32
            }
        };
        let generation = self.slots[index as usize].generation;
        ((kind as u64) << 56) | ((generation as u64) << 32) | index as u64
    }

    /// Populates the payload of a handle previously created with [`Self::reserve`].
    pub fn put(&mut self, handle: u64, item: Item) {
        self.slots[(handle & 0xffff_ffff) as usize].item = Some(item);
    }

    /// Allocates a handle of `kind` and immediately populates it with `item`.
    pub fn insert(&mut self, kind: u8, item: Item) -> u64 {
        let handle = self.reserve(kind);
        self.put(handle, item);
        handle
    }

    /// Validates `handle`'s kind tag, slot index, and 24-bit generation, returning a diagnostic
    /// `ERR_STALE_HANDLE` message distinguishing zero, released, unknown, and mistyped handles.
    fn slot(&self, handle: u64, kind: u8) -> GkResult<usize> {
        let index = (handle & 0xffff_ffff) as usize;
        let generation = ((handle >> 32) as u32) & GEN_MASK;
        let named = (handle >> 56) as u8;
        match self.slots.get(index) {
            Some(slot) if index != 0 && named == kind && slot.kind == kind && slot.generation == generation => {
                Ok(index)
            }
            Some(slot) if index != 0 && named == kind && slot.kind == kind => err(
                ERR_STALE_HANDLE,
                format!("handle {handle:#x} ({}) was released", kind_name(kind)),
            ),
            _ if handle == 0 => err(
                ERR_STALE_HANDLE,
                format!("a zero handle (one that was never made) was used where a {} is needed", kind_name(kind)),
            ),

            slot if named == kind && slot.is_none_or(|s| index == 0 || s.kind != kind) => err(
                ERR_STALE_HANDLE,
                format!("handle {handle:#x} names no {} this library made", kind_name(kind)),
            ),
            _ => err(
                ERR_STALE_HANDLE,
                format!(
                    "handle {handle:#x} is a {}, not a {}",
                    kind_name(named),
                    kind_name(kind)
                ),
            ),
        }
    }

    fn item(&self, handle: u64, kind: u8) -> GkResult<&Item> {
        let index = self.slot(handle, kind)?;
        match &self.slots[index].item {
            Some(item) => Ok(item),
            None => err(ERR_STALE_HANDLE, format!("handle {handle:#x} has no object yet")),
        }
    }

    /// Releases `handle`, bumps its slot generation (skipping 0), increments `RELEASE_EPOCH`,
    /// scrubs secondary indexes, and returns the removed [`Item`] so the caller drops it outside
    /// the `Session` `RefCell` borrow.
    pub fn release(&mut self, handle: u64) -> GkResult<Option<Item>> {
        let kind = (handle >> 56) as u8;
        let index = self.slot(handle, kind)?;
        let slot = &mut self.slots[index];
        let item = slot.item.take();
        slot.generation = (slot.generation + 1) & GEN_MASK;
        if slot.generation == 0 {
            slot.generation = 1;
        }
        self.free.push(index as u32);
        RELEASE_EPOCH.fetch_add(1, Ordering::Relaxed);
        match kind {
            HK_VIEW => {
                let was_mounted = self.view_window.remove(&handle).is_some();
                for list in self.dirty.values_mut() {
                    if !list.is_empty() {
                        list.retain(|v| *v != handle);
                    }
                }
                if !was_mounted && !self.unmounted_dirty.is_empty() {
                    self.unmounted_dirty.retain(|v| *v != handle);
                }
                if let Some(deps) = self.row_deps.remove(&handle) {
                    for dep in deps {
                        if let Some(list) = self.dep_views.get_mut(&dep) {
                            list.retain(|v| *v != handle);
                        }
                    }
                } else if matches!(item, Some(Item::View(_))) && !self.dep_views.is_empty() {
                    for list in self.dep_views.values_mut() {
                        list.retain(|v| *v != handle);
                    }
                }
                self.row_owner.remove(&handle);
            }
            HK_SOURCE => {
                self.sources.retain(|s| *s != handle);
            }
            HK_STATE => {
                self.dep_views.remove(&handle);
            }
            HK_WINDOW => {
                self.windows.retain(|w| *w != handle);
                self.frame_handler.remove(&handle);
                self.dirty.remove(&handle);
            }
            _ => {}
        }
        Ok(item)
    }

    /// Looks up a GPUI `Entity<GkView>` by its `HK_VIEW` handle.
    pub fn view(&self, handle: u64) -> GkResult<Entity<GkView>> {
        match self.item(handle, HK_VIEW)? {
            Item::View(e) => Ok(e.clone()),
            _ => err(ERR_STALE_HANDLE, "not a view"),
        }
    }

    /// Returns `true` if `handle` refers to any live view (`Item::View` or `Item::DecodedRow`).
    pub fn has_view(&self, handle: u64) -> bool {
        matches!(self.item(handle, HK_VIEW), Ok(Item::View(_) | Item::DecodedRow(_)))
    }

    /// Replaces the state-observer subscriptions retained for an `Item::DecodedRow` handle.
    pub fn set_decoded_subs(&mut self, handle: u64, subs: Vec<Subscription>) {
        if let Ok(index) = self.slot(handle, HK_VIEW) {
            if let Some(Item::DecodedRow(s)) = &mut self.slots[index].item {
                *s = subs;
            }
        }
    }

    /// Looks up a GPUI `AnyWindowHandle` by its `HK_WINDOW` handle.
    pub fn window(&self, handle: u64) -> GkResult<AnyWindowHandle> {
        match self.item(handle, HK_WINDOW)? {
            Item::Window(w) => Ok(*w),
            _ => err(ERR_STALE_HANDLE, "not a window"),
        }
    }

    /// Looks up a type-erased [`StateItem`] by its `HK_STATE` handle.
    pub fn state(&self, handle: u64) -> GkResult<&StateItem> {
        match self.item(handle, HK_STATE)? {
            Item::State(s) => Ok(s),
            _ => err(ERR_STALE_HANDLE, "not a state"),
        }
    }

    /// Looks up a library object entry by its `HK_OBJ` handle.
    pub fn obj(&self, handle: u64) -> GkResult<&crate::objects::ObjItem> {
        match self.item(handle, HK_OBJ)? {
            Item::Obj(o) => Ok(o),
            _ => err(ERR_STALE_HANDLE, "not a library object"),
        }
    }

    /// Counts currently live handles of `kind`.
    pub fn count(&self, kind: u8) -> usize {
        self.slots.iter().filter(|s| s.kind == kind && s.item.is_some()).count()
    }

    /// Looks up a `RowStore` entity by its `HK_ROWS` handle.
    pub fn rows(&self, handle: u64) -> GkResult<Entity<RowStore>> {
        match self.item(handle, HK_ROWS)? {
            Item::Rows(e) => Ok(e.clone()),
            _ => err(ERR_STALE_HANDLE, "not a row store"),
        }
    }

    /// Looks up a `FocusHandle` and optional creation window by its `HK_FOCUS` handle.
    pub fn focus(&self, handle: u64) -> GkResult<(FocusHandle, Option<AnyWindowHandle>)> {
        match self.item(handle, HK_FOCUS)? {
            Item::Focus(f, w) => Ok((f.clone(), *w)),
            _ => err(ERR_STALE_HANDLE, "not a focus handle"),
        }
    }

    /// Looks up a mutable row [`crate::source::Source`] by its `HK_SOURCE` handle.
    pub fn source_mut(&mut self, handle: u64) -> GkResult<&mut crate::source::Source> {
        let index = self.slot(handle, HK_SOURCE)?;
        match &mut self.slots[index].item {
            Some(Item::Source(s)) => Ok(&mut **s),
            _ => err(ERR_STALE_HANDLE, "not a row source"),
        }
    }

    /// Returns `true` if any row source owned by window `wh` still has queued prefetch rows.
    pub fn has_pending_ahead(&self, wh: u64) -> bool {
        self.sources.iter().any(|&h| {
            self.item(h, HK_SOURCE).is_ok_and(|item| match item {
                Item::Source(s) => s.window == wh && s.has_ahead(),
                _ => false,
            })
        })
    }

    /// Returns the monotonic count of `pre_frame` ticks executed so far.
    pub fn frame_no(&self) -> u64 {
        self.frame_no
    }

    /// Associates `view` with owning window `wh` so dirty notifications schedule `wh`'s `pre_frame`.
    pub fn mount(&mut self, view: u64, wh: u64) {
        self.view_window.insert(view, wh);
    }

    /// Finds the `HK_WINDOW` handle corresponding to GPUI `AnyWindowHandle`.
    pub fn window_of(&self, any: AnyWindowHandle) -> Option<u64> {
        self.windows.iter().copied().find(|w| self.window(*w).ok() == Some(any))
    }

    /// Formats a human-readable summary of live handle counts for `GK_DEBUG` exit diagnostics.
    pub fn live(&self) -> String {
        let mut n = [0usize; 8];
        for slot in &self.slots {
            if slot.item.is_some() {
                n[(slot.kind as usize).min(7)] += 1;
            }
        }
        format!(
            "views {}, windows {}, states {}, rows {}, subscriptions {}, focus handles {}",
            n[HK_VIEW as usize],
            n[HK_WINDOW as usize],
            n[HK_STATE as usize],
            n[HK_ROWS as usize],
            n[HK_SUB as usize],
            n[HK_FOCUS as usize]
        )
    }
}

/// Installs a process-wide panic hook once that records panic messages in thread-local storage
/// and silences expected re-entrancy probes run by [`probe`].
pub fn install_panic_hook() {
    static ONCE: std::sync::Once = std::sync::Once::new();
    ONCE.call_once(|| {
        let previous = std::panic::take_hook();
        std::panic::set_hook(Box::new(move |info| {
            if UI.with(|u| u.quiet_panic.get()) {
                return;
            }
            let what = info
                .payload()
                .downcast_ref::<&str>()
                .map(|s| s.to_string())
                .or_else(|| info.payload().downcast_ref::<String>().cloned())
                .unwrap_or_else(|| "a panic without a message".to_string());
            let place = info
                .location()
                .map(|l| format!(" at {}:{}", l.file(), l.line()))
                .unwrap_or_default();
            UI.with(|u| *u.panic_msg.borrow_mut() = format!("{what}{place}"));
            previous(info);
        }));
    });
}

/// Returns the last panic message captured on the calling thread.
pub fn panic_message() -> String {
    UI.with(|u| u.panic_msg.borrow().clone())
}

/// Catches panics around internal GPUI callbacks so unwinding never crosses C/GPUI boundaries.
///
/// [`AppStop`] unwinds are absorbed silently (already reported via `K_PANIC`); any other panic
/// poisons the library via [`fatal`].
pub fn guard<T>(what: &'static str, f: impl FnOnce() -> T) -> Option<T> {
    match catch_unwind(AssertUnwindSafe(f)) {
        Ok(v) => Some(v),
        Err(e) if e.is::<AppStop>() => None,
        Err(_) => {
            fatal(what);
            None
        }
    }
}

/// Unwind sentinel thrown by [`app_stop`] to abort the current turn without poisoning the runtime.
pub struct AppStop;

static APP_STOPPED: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);

/// Reports a host misuse error via `K_PANIC` (with app-fault flag `1`), schedules shutdown,
/// and unwinds to the nearest [`guard`] or C ABI boundary.
pub fn app_stop(msg: String) -> ! {
    let session = UI.with(|u| u.session.try_borrow().ok().and_then(|s| s.as_ref().map(|s| (s.callback, s.user))));
    let Some((callback, user)) = session else {
        eprintln!("gk: {msg}");
        std::process::exit(2);
    };
    if !APP_STOPPED.swap(true, Ordering::SeqCst) {
        let mut req = Vec::with_capacity(msg.len() + 5);
        req.put_str(&msg);
        req.put_u8(1);
        let mut cb = GkCb::new();
        // SAFETY: `callback` and `user` were validated in `check_app`; `cb` and `req` are valid
        // stack/heap buffers for the duration of the synchronous FFI call.
        unsafe { callback(user, &mut cb, K_PANIC, req.as_ptr(), req.len()) };
        crate::quit_soon();
    }
    std::panic::resume_unwind(Box::new(AppStop))
}

/// Poisons the library after an unexpected Rust panic, notifies the host via `K_PANIC`,
/// and schedules shutdown.
pub fn fatal(what: &str) {
    POISONED.store(true, Ordering::SeqCst);
    let msg = format!("{} (while {what})", panic_message());
    let session = UI.with(|u| u.session.try_borrow().ok().and_then(|s| s.as_ref().map(|s| (s.callback, s.user))));
    if let Some((callback, user)) = session {
        let mut req = Vec::with_capacity(msg.len() + 4);
        req.put_str(&msg);
        let mut cb = GkCb::new();
        // SAFETY: `callback` and `user` were validated in `check_app`; `cb` and `req` are live
        // for the synchronous duration of this notification.
        unsafe { callback(user, &mut cb, K_PANIC, req.as_ptr(), req.len()) };
        crate::quit_soon();
        return;
    }
    eprintln!("gk: the library panicked: {msg}");
    std::process::exit(1);
}

/// Returns `true` if `cb` matches the innermost active [`turn_raw`] callback frame on this thread.
pub fn current_cb(cb: *mut GkCb) -> bool {
    UI.with(|u| u.scopes.borrow().last().is_some_and(|t| t.cb == cb))
}

/// Borrows the active `App` and optional `Window` for a `gk_call` running inside [`turn_raw`].
pub fn scope<'a>() -> Option<(&'a mut App, Option<&'a mut Window>)> {
    let top = UI.with(|u| u.scopes.borrow().last().copied())?;
    // SAFETY: `turn_raw` pushed `top` from live `&mut App` and `Option<&mut Window>` references
    // before invoking the host callback and pops it immediately on return. `gk_call` only runs
    // synchronously on the UI thread while the callback is paused, so no Rust caller aliases them.
    unsafe { Some((&mut *top.app, top.window.map(|w| &mut *w))) }
}

/// Runs `f` on window `any`, reusing `window` directly when it already matches `any` to avoid
/// GPUI's re-entrant window borrow panic.
pub fn with_window<T>(
    any: AnyWindowHandle,
    window: Option<&mut Window>,
    cx: &mut App,
    f: impl FnOnce(&mut Window, &mut App) -> T,
) -> GkResult<T> {
    if let Some(window) = window {
        if window.window_handle() == any {
            return Ok(f(window, cx));
        }
    }
    any.update(cx, |_, window, cx| f(window, cx))
        .map_err(|e| GkErr(ERR_BUSY, format!("window not available: {e:#}")))
}

/// Decoded sections of a host callback reply: return bytes, dirty view IDs, deferred ops, and view tapes.
#[derive(Default)]
pub struct Reply {
    pub ret: Vec<u8>,
    pub dirty: Vec<u64>,
    pub deferred: Vec<Vec<u8>>,
    pub tapes: Vec<(u64, u32, Vec<u8>)>,
}

fn parse_reply(b: &[u8]) -> Reply {
    let mut reply = Reply::default();
    if b.is_empty() {
        return reply;
    }
    let mut r = R::new(b);
    let n = r.u32() as usize;
    reply.ret = r.bytes(n).to_vec();
    for _ in 0..r.u32() {
        reply.dirty.push(r.u64());
    }
    for _ in 0..r.u32() {
        let n = r.u32() as usize;
        reply.deferred.push(r.bytes(n).to_vec());
    }
    for _ in 0..r.u32() {
        let view = r.u64();
        let generation = r.u32();
        let n = r.u32() as usize;
        reply.tapes.push((view, generation, r.bytes(n).to_vec()));
    }
    reply
}

/// Invokes the host callback for one turn of wire `kind`, passing its reply slice to `on_reply`.
fn turn_raw<T>(
    kind: u32,
    req: &[u8],
    mut window: Option<&mut Window>,
    cx: &mut App,
    on_reply: impl FnOnce(&[u8], Option<&mut Window>, &mut App) -> T,
) -> T {
    trace(|| format!("turn kind={kind} window={}", window.is_some()));
    let mut cb = GkCb::new();
    let (callback, user) = UI.with(|u| {
        let s = u.session.borrow();
        let s = s.as_ref().expect("gk: no session; call gk_run first");
        u.scopes.borrow_mut().push(Scope {
            app: cx as *mut App,
            window: window.as_deref_mut().map(|w| w as *mut Window),
            cb: &mut cb as *mut GkCb,
        });
        (s.callback, s.user)
    });
    let t0 = Instant::now();
    // SAFETY: `callback` and `user` were validated in `check_app`; `cb` and `req` remain valid
    // until `callback` returns, and the matching `Scope` entry is popped immediately afterward.
    let rc = unsafe { callback(user, &mut cb, kind, req.as_ptr(), req.len()) };
    let ns = t0.elapsed().as_nanos() as u64;
    UI.with(|u| {
        u.scopes.borrow_mut().pop();
    });
    gk_meter::bump_turn(kind, ns);
    if rc != 0 {
        eprintln!("gk: callback kind {kind} returned {rc}");
    }
    let out = if !cb.inline_ptr.is_null() && cb.inline_len > 0 {
        // SAFETY: By the `FEAT_INLINE_REPLY` contract, the host's inline reply buffer stays
        // readable on the UI thread until `turn_raw` finishes consuming it in `on_reply`.
        let bytes = unsafe { std::slice::from_raw_parts(cb.inline_ptr, cb.inline_len) };
        on_reply(bytes, window, cx)
    } else {
        on_reply(&cb.reply, window, cx)
    };
    // Recycle medium-sized heap buffers across turns to avoid repeated allocations.
    if cb.reply.capacity() > 0 && cb.reply.capacity() <= 256 * 1024 {
        cb.reply.clear();
        UI.with(|u| {
            let mut slot = u.reply_buf.borrow_mut();
            if slot.capacity() < cb.reply.capacity() {
                *slot = cb.reply;
            }
        });
    }
    out
}

/// Executes a host callback turn and parses its standard four-section [`Reply`].
pub fn turn(kind: u32, req: &[u8], window: Option<&mut Window>, cx: &mut App) -> Reply {
    turn_raw(kind, req, window, cx, |bytes, _, _| parse_reply(bytes))
}

#[inline]
fn is_rows_push_op(op: &[u8]) -> bool {
    op.len() >= 2 && u16::from_le_bytes([op[0], op[1]]) == C_ROWS_PUSH
}

/// Applies dirty-view notifications and deferred write ops returned by a host callback turn.
///
/// Writes emitted during `K_VIEWS` or `K_ROWS` are rejected because view functions must be pure.
/// Writes from `K_EVENT` are queued onto `cx.defer` (except pure `C_ROWS_PUSH` batches, which run
/// inline) so entity mutations never collide with the GPUI entity borrow firing the event.
pub fn process_reply(reply: &mut Reply, kind: u32, mut window: Option<&mut Window>, cx: &mut App) {
    for view in std::mem::take(&mut reply.dirty) {
        mark_dirty(view, window.as_deref_mut(), cx);
    }
    let deferred = std::mem::take(&mut reply.deferred);
    if deferred.is_empty() {
        return;
    }
    match kind {
        K_VIEWS | K_ROWS => eprintln!(
            "gk: {} write(s) from a view function were refused: a view function may not write",
            deferred.len()
        ),
        K_EVENT => {
            // Fast path: pure `C_ROWS_PUSH` batches touch only `RowStore` and never re-enter
            // component state entities, so they can execute synchronously when the queue is empty.
            if deferred.iter().all(|op| is_rows_push_op(op))
                && with(|s| s.deferred.is_empty())
            {
                apply_ops(&deferred, None, cx);
                return;
            }
            let wh = window.as_deref().map(|w| w.window_handle());
            let schedule = with(|s| {
                s.deferred.extend(deferred.into_iter().map(|op| (op, wh)));
                !std::mem::replace(&mut s.deferred_scheduled, true)
            });
            if schedule {
                cx.defer(apply_deferred);
            }
        }
        _ => apply_ops(&deferred, window, cx),
    }
}

/// Enqueues a single raw wire call to be replayed on the next `cx.defer` drain.
pub fn defer_op(op: Vec<u8>, cx: &mut App) {
    let schedule = with(|s| {
        s.deferred.push((op, None));
        !std::mem::replace(&mut s.deferred_scheduled, true)
    });
    if schedule {
        cx.defer(apply_deferred);
    }
}

/// Drains buffered deferred write ops in contiguous per-window runs so each window is entered once.
fn apply_deferred(cx: &mut App) {
    let ops = with(|s| {
        s.deferred_scheduled = false;
        std::mem::take(&mut s.deferred)
    });
    guard("applying buffered writes", || {

        let mut i = 0;
        while i < ops.len() {
            let wh = ops[i].1;
            let mut j = i + 1;
            while j < ops.len() && ops[j].1 == wh {
                j += 1;
            }
            if j - i == 1 {
                let op = std::slice::from_ref(&ops[i].0);
                match wh {
                    Some(any) if !is_rows_push_op(&ops[i].0) => {
                        let mut ran = false;
                        let _ = any.update(cx, |_, window, cx| {
                            ran = true;
                            apply_ops(op, Some(window), cx);
                        });
                        if !ran {
                            apply_ops(op, None, cx);
                        }
                    }
                    _ => apply_ops(op, None, cx),
                }
            } else {
                let run: Vec<Vec<u8>> = ops[i..j].iter().map(|(op, _)| op.clone()).collect();
                match wh {
                    Some(any) => {
                        let mut ran = false;
                        let _ = any.update(cx, |_, window, cx| {
                            ran = true;
                            apply_ops(&run, Some(window), cx);
                        });
                        if !ran {
                            apply_ops(&run, None, cx);
                        }
                    }
                    None => apply_ops(&run, None, cx),
                }
            }
            i = j;
        }
    });
}

fn apply_ops(ops: &[Vec<u8>], mut window: Option<&mut Window>, cx: &mut App) {
    let mut out = Vec::new();
    for op in ops {
        gk_meter::bump_deferred(1);
        out.clear();
        if let Err(GkErr(code, msg)) =
            crate::calls::call(op, window.as_deref_mut(), cx, &mut out, true)
        {
            report_deferred_err(code, msg);
        }
    }
}

fn report_deferred_err(code: i32, msg: String) {
    if code == ERR_STALE_HANDLE || code == ERR_BAD_OP {

        app_stop(format!(
            "a change a handler asked for could not be made: {msg}\n    The write was \
             buffered by a handler and applied after it returned, so its line is not known \
             here. Run with GK_DEBUG=1: every write is then sent at once and the program \
             stops at the line that made it."
        ));
    }
    eprintln!("gk: a change a handler asked for could not be made: {msg}");
}

/// Dispatches a `K_EVENT` turn for `(view, generation, handler, shape)` with payload written by
/// `put_payload`, returning any return-value bytes from the host reply.
pub fn fire_event(
    view: u64,
    generation: u32,
    handler: u32,
    shape: u16,
    put_payload: impl FnOnce(&mut Vec<u8>),
    window: Option<&mut Window>,
    cx: &mut App,
) -> Vec<u8> {
    fire_inner(
        |buf| {
            buf.put_u64(view);
            buf.put_u32(generation);
            buf.put_u32(handler);
            buf.put_u16(shape);
            put_payload(buf);
        },
        window,
        cx,
    )
}

/// Dispatches a pre-encoded `K_EVENT` body (`[view][gen][handler][shape][payload...]`) to the host.
pub fn fire_req(req: &[u8], window: Option<&mut Window>, cx: &mut App) -> Vec<u8> {
    fire_inner(|buf| buf.extend_from_slice(req), window, cx)
}

/// Shared `K_EVENT` wire framing and zero-copy reply parser.
///
/// Request header: `[wh: u64][flags: u8][n_retired: u32][(view: u64, gen: u32)*][body...]`.
/// `flags == 1` on an outermost windowed event lets the host piggyback tapes directly on the reply
/// and retire old handler generations without waiting for a separate `K_VIEWS` turn.
fn fire_inner(
    write_body: impl FnOnce(&mut Vec<u8>),
    window: Option<&mut Window>,
    cx: &mut App,
) -> Vec<u8> {
    guard("delivering an event", || {
        let win_handle = window.as_deref().map(|w| w.window_handle());
        let (wh, mut buf) = UI.with(|u| {
            let wh = win_handle
                .and_then(|any| {
                    u.session
                        .borrow()
                        .as_ref()
                        .expect("gk: no session; call gk_run first")
                        .window_of(any)
                })
                .unwrap_or(0);
            let outer = u.scopes.borrow().is_empty();
            let flags: u8 = if outer && wh != 0 { 1 } else { 0 };
            let mut buf = std::mem::take(&mut *u.event_buf.borrow_mut());
            buf.clear();
            buf.put_u64(wh);
            buf.put_u8(flags);
            if flags != 0 {
                put_retired_ui(u, &mut buf);
            } else {
                buf.put_u32(0);
            }
            (wh, buf)
        });
        write_body(&mut buf);
        let ret = turn_raw(K_EVENT, &buf, window, cx, |bytes, mut window, cx| {
            let ret = if bytes.is_empty() {
                Vec::new()
            } else {
                let mut r = R::new(bytes);
                let ret_len = r.u32() as usize;
                let ret = if ret_len > 0 { r.bytes(ret_len).to_vec() } else { Vec::new() };
                let n_dirty = r.u32() as usize;
                let dirty_at = r.i;
                r.i += n_dirty * 8;
                let n_def = r.u32() as usize;
                let def_at = r.i;
                let mut all_rows_push = n_def > 0;
                for _ in 0..n_def {
                    let n = r.u32() as usize;
                    let op = r.bytes(n);
                    if !is_rows_push_op(op) {
                        all_rows_push = false;
                    }
                }
                let n_tapes = r.u32() as usize;
                if wh != 0 && n_tapes > 0 {
                    accept_tapes_from(wh, n_tapes, &mut r, cx);
                }
                if n_dirty > 0 {
                    let mut dr = R::at(bytes, dirty_at);
                    for _ in 0..n_dirty {
                        mark_dirty(dr.u64(), window.as_deref_mut(), cx);
                    }
                }
                if n_def > 0 {
                    if all_rows_push && with(|s| s.deferred.is_empty()) {
                        let mut fr = R::at(bytes, def_at);
                        for _ in 0..n_def {
                            let n = fr.u32() as usize;
                            let op = fr.bytes(n);
                            gk_meter::bump_deferred(1);
                            if let Err(GkErr(code, msg)) = crate::hand::rows_push_slice(op, cx) {
                                report_deferred_err(code, msg);
                            }
                        }
                    } else {
                        let wh_any = window.as_deref().map(|w| w.window_handle());
                        let mut fr = R::at(bytes, def_at);
                        let schedule = with(|s| {
                            s.deferred.reserve(n_def);
                            for _ in 0..n_def {
                                let n = fr.u32() as usize;
                                s.deferred.push((fr.bytes(n).to_vec(), wh_any));
                            }
                            !std::mem::replace(&mut s.deferred_scheduled, true)
                        });
                        if schedule {
                            cx.defer(apply_deferred);
                        }
                    }
                }
                ret
            };

            if wh != 0 && crate::objects::needs_sweep() {
                ensure_hook(wh, window.as_deref_mut(), cx);
            }
            ret
        });
        if buf.capacity() <= 64 * 1024 {
            buf.clear();
            UI.with(|u| {
                let mut slot = u.event_buf.borrow_mut();
                if slot.capacity() < buf.capacity() {
                    *slot = buf;
                }
            });
        }
        ret
    })
    .unwrap_or_default()
}

/// Queues a pre-encoded event body for deferred delivery once the active GPUI entity update ends.
pub fn queue_event(req: Vec<u8>, view: u64, cx: &mut App) {
    let schedule = with(|s| {
        let window = match s.view_window.get(&view) {
            Some(wh) => s.window(*wh).ok(),
            None => s.windows.first().and_then(|wh| s.window(*wh).ok()),
        };
        s.queued.push(Queued { req, window });
        !std::mem::replace(&mut s.queued_scheduled, true)
    });
    if schedule {
        cx.defer(|cx| {
            let queued = with(|s| {
                s.queued_scheduled = false;
                std::mem::take(&mut s.queued)
            });
            for q in queued {
                let delivered = match q.window {
                    Some(any) => any
                        .update(cx, |_, window, cx| {
                            fire_req(&q.req, Some(window), cx);
                        })
                        .is_ok(),
                    None => false,
                };
                if !delivered {
                    fire_req(&q.req, None, cx);
                }
            }
        });
    }
}

/// Marks `view` dirty and ensures a `pre_frame` hook is scheduled on its owning window.
///
/// Views not yet mounted in any window are recorded in `unmounted_dirty` and promoted as soon as
/// a parent tape mounts them via `tape.slots`.
pub fn mark_dirty(view: u64, window: Option<&mut Window>, cx: &mut App) {
    trace(|| format!("dirty view={view:#x} window={} mounted_in={:?} in_hook={:?}", window.is_some(),
        with(|s| s.view_window.get(&view).copied()), with(|s| s.in_hook)));
    let target = with(|s| match s.view_window.get(&view).copied() {
        Some(w) => {
            let list = s.dirty.entry(w).or_default();
            if !list.contains(&view) {
                list.push(view);
            }
            Some(w)
        }
        None => {
            if s.has_view(view) && !s.unmounted_dirty.contains(&view) {
                s.unmounted_dirty.push(view);
            }
            None
        }
    });
    if let Some(w) = target {
        ensure_hook(w, window, cx);
    }
}

/// Immediately marks views depending on `state` dirty when `state` is written during `pre_frame`
/// or `open_window_with` (so they re-describe in the same frame rather than waiting one frame for
/// GPUI's asynchronous `cx.observe` notification).
pub fn written(state: u64, window: Option<&mut Window>, cx: &mut App) {
    let views = with(|s| match s.in_hook {
        Some(_) => s.dep_views.get(&state).cloned().unwrap_or_default(),
        None => Vec::new(),
    });
    let mut window = window;
    for view in views {
        mark_dirty(view, window.as_deref_mut(), cx);
    }
}

/// Registers a single `on_next_frame` [`pre_frame`] hook for window `wh` if not already hooked
/// or currently executing inside `pre_frame`.
pub fn ensure_hook(wh: u64, window: Option<&mut Window>, cx: &mut App) {
    let need = with(|s| s.in_hook != Some(wh) && s.hooked.insert(wh));
    trace(|| format!("ensure_hook window={wh:#x} need={need} with_window={}", window.is_some()));
    if !need {
        return;
    }
    let Ok(any) = with(|s| s.window(wh)) else {
        with(|s| s.hooked.remove(&wh));
        return;
    };
    if let Some(window) = window {
        if window.window_handle() == any {
            window.on_next_frame(move |window, cx| pre_frame(wh, window, cx));
            return;
        }
    }
    let registered = any
        .update(cx, |_, window, _| {
            window.on_next_frame(move |window, cx| pre_frame(wh, window, cx));
        })
        .is_ok();
    if !registered {
        // Window is currently borrowed on the stack; retry after the active update finishes.
        with(|s| s.hooked.remove(&wh));
        cx.defer(move |cx| ensure_hook(wh, None, cx));
    }
}

/// Per-frame synchronization hook for window `wh`.
///
/// Runs in order: (1) sweeps unretained temporary handles and expired pins, (2) delivers `K_FRAME`
/// if the host installed a frame callback, (3) batches row-source binds/evictions via `K_ROWS`,
/// (4) requests tapes for all remaining dirty views via `K_VIEWS`, and (5) re-arms itself if the
/// frame handler, newly dirtied views, or prefetch lookahead need another frame.
fn pre_frame(wh: u64, window: &mut Window, cx: &mut App) {
    guard("running the pre-frame hook", || {

        crate::objects::sweep();
        let (frame_handler, frame_no) = with(|s| {
            s.hooked.remove(&wh);
            s.in_hook = Some(wh);
            s.frame_no += 1;
            (s.frame_handler.contains(&wh), s.frame_no)
        });
        if frame_handler {
            let mut req = UI.with(|u| {
                let mut buf = std::mem::take(&mut *u.event_buf.borrow_mut());
                buf.clear();
                buf.put_u64(wh);
                buf.put_u64(frame_no);
                {
                    let mut s = u.session.borrow_mut();
                    let s = s.as_mut().expect("gk: no session; call gk_run first");
                    if let Some(pre_dirty) = s.dirty.get_mut(&wh) {
                        buf.put_u32(pre_dirty.len() as u32);
                        for &v in pre_dirty.iter() {
                            buf.put_u64(v);
                        }
                        pre_dirty.clear();
                    } else {
                        buf.put_u32(0);
                    }
                }
                put_retired_ui(u, &mut buf);
                buf
            });
            let deferred = turn_raw(K_FRAME, &req, Some(&mut *window), cx, |bytes, mut window, cx| {
                if bytes.is_empty() {
                    return Vec::new();
                }
                let mut r = R::new(bytes);
                let ret_len = r.u32() as usize;
                r.i += ret_len;
                let n_dirty = r.u32() as usize;
                let dirty_at = r.i;
                r.i += n_dirty * 8;
                let n_def = r.u32() as usize;
                let deferred = if n_def > 0 {
                    let mut v = Vec::with_capacity(n_def);
                    for _ in 0..n_def {
                        let n = r.u32() as usize;
                        v.push(r.bytes(n).to_vec());
                    }
                    v
                } else {
                    Vec::new()
                };
                let n_tapes = r.u32() as usize;
                if n_tapes > 0 {

                    accept_tapes_from(wh, n_tapes, &mut r, cx);
                }
                if n_dirty > 0 {
                    let mut dr = R::at(bytes, dirty_at);
                    for _ in 0..n_dirty {
                        mark_dirty(dr.u64(), window.as_deref_mut(), cx);
                    }
                }
                deferred
            });
            if req.capacity() <= 64 * 1024 {
                req.clear();
                UI.with(|u| {
                    let mut slot = u.event_buf.borrow_mut();
                    if slot.capacity() < req.capacity() {
                        *slot = req;
                    }
                });
            }
            if !deferred.is_empty() {

                apply_ops(&deferred, Some(&mut *window), cx);
            }
        }

        crate::source::rows_turn(wh, window, cx);
        let dirty = with(|s| s.dirty.remove(&wh).unwrap_or_default());
        trace(|| format!("pre_frame window={wh:#x} frame={frame_no} dirty={}", dirty.len()));
        if !dirty.is_empty() {
            describe(wh, &dirty, window, cx);
        }
        let again = with(|s| {
            s.in_hook = None;
            s.frame_handler.contains(&wh)
                || s.dirty.get(&wh).is_some_and(|d| !d.is_empty())
                || s.has_pending_ahead(wh)
        });
        if again {
            ensure_hook(wh, Some(window), cx);
        }
    });
}

static RETIRED: std::sync::Mutex<Vec<(u64, u32)>> = std::sync::Mutex::new(Vec::new());
static HAS_RETIRED: AtomicBool = AtomicBool::new(false);

/// Queues a retired `(view, generation)` pair from any thread so the host can free its old
/// handler table on the next turn.
pub fn retire(view: u64, generation: u32) {
    let on_ui = UI.with(|u| {
        if u.is_ui.get() {
            u.retired.borrow_mut().push((view, generation));
            true
        } else {
            false
        }
    });
    if !on_ui {
        if let Ok(mut r) = RETIRED.lock() {
            r.push((view, generation));
            HAS_RETIRED.store(true, Ordering::Release);
        }
    }
}

fn put_retired_ui(u: &UiTls, buf: &mut Vec<u8>) {
    let mut local = u.retired.borrow_mut();
    let remote = if HAS_RETIRED.load(Ordering::Acquire) {
        RETIRED
            .lock()
            .map(|mut r| {
                HAS_RETIRED.store(false, Ordering::Release);
                std::mem::take(&mut *r)
            })
            .unwrap_or_default()
    } else {
        Vec::new()
    };
    buf.put_u32((local.len() + remote.len()) as u32);
    for &(view, generation) in local.iter() {
        buf.put_u64(view);
        buf.put_u32(generation);
    }
    local.clear();
    for (view, generation) in remote {
        buf.put_u64(view);
        buf.put_u32(generation);
    }
}

pub(crate) fn take_retired() -> Vec<(u64, u32)> {
    let mut out = UI.with(|u| {
        let mut local = u.retired.borrow_mut();
        if local.is_empty() {
            Vec::new()
        } else {
            let v = local.clone();
            local.clear();
            v
        }
    });
    if HAS_RETIRED.load(Ordering::Acquire) {
        if let Ok(mut r) = RETIRED.lock() {
            HAS_RETIRED.store(false, Ordering::Release);
            out.extend(r.drain(..));
        }
    }
    out
}

/// Installs a batch of host-supplied `(view, generation, tape_bytes)` triples for window `wh`.
pub(crate) fn accept_tapes(wh: u64, tapes: Vec<(u64, u32, Vec<u8>)>, cx: &mut App) {
    if tapes.is_empty() {
        return;
    }
    let mut sources = Vec::new();
    let mut first = Vec::new();
    for (view, generation, bytes) in tapes {
        if let Err(GkErr(code, msg)) = accept_one(wh, view, generation, bytes, &mut sources, &mut first, cx) {
            eprintln!(
                "gk: a view's description was refused ({msg}; error {code}); the view keeps what it showed. \
                 The app's go/ folder and the library are from different downloads, or this is a bug in the binding."
            );
        }
    }
    finish_accept_tapes(sources, first, cx);
}

/// Streams `n_tapes` entries directly out of a reply reader `r` without copying into an intermediate `Vec`.
fn accept_tapes_from(wh: u64, n_tapes: usize, r: &mut R, cx: &mut App) {
    let mut sources = Vec::new();
    let mut first = Vec::new();
    for _ in 0..n_tapes {
        let view = r.u64();
        let generation = r.u32();
        let n = r.u32() as usize;
        let bytes = r.bytes(n).to_vec();
        if let Err(GkErr(code, msg)) = accept_one(wh, view, generation, bytes, &mut sources, &mut first, cx) {
            eprintln!(
                "gk: a view's description was refused ({msg}; error {code}); the view keeps what it showed. \
                 The app's go/ folder and the library are from different downloads, or this is a bug in the binding."
            );
        }
    }
    finish_accept_tapes(sources, first, cx);
}

/// Notifies row-source watchers only when an existing row changed or a first-time row had a
/// recorded render miss (so prefetch-only fills do not trigger redundant parent redraws).
fn finish_accept_tapes(mut sources: Vec<u64>, first: Vec<u64>, cx: &mut App) {
    for src in first {
        if crate::source::take_missed(src) && !sources.contains(&src) {
            sources.push(src);
        }
    }
    for src in &sources {
        crate::source::take_missed(*src);
    }
    if !sources.is_empty() {
        crate::source::notify_watchers(&sources, cx);
    }
}

/// Parses one tape, updates mounted child slots and `HK_STATE` observer subscriptions, and stores
/// the new `Rc<Tape>` on the view entity or decoded row source.
fn accept_one(
    wh: u64,
    view: u64,
    generation: u32,
    bytes: Vec<u8>,
    sources: &mut Vec<u64>,
    first: &mut Vec<u64>,
    cx: &mut App,
) -> GkResult<()> {
    trace(|| format!("tape view={view:#x} generation={generation} bytes={}", bytes.len()));
    let n = bytes.len() as u64;
    let mut tape = match Tape::parse(bytes, generation) {
        Ok(tape) => tape,
        Err(e) => {
            retire(view, generation);
            return Err(e);
        }
    };
    if tape.handlers > 0 {
        tape.pin = Some(std::sync::Arc::new(crate::tape::GenPin { view, generation }));
    }

    let (row, entity_opt, row_old) = with(|s| -> GkResult<(Option<(u64, u64, bool, bool)>, Option<Entity<GkView>>, Vec<u64>)> {
        if !s.dirty.is_empty() {
            if let Some(list) = s.dirty.get_mut(&wh) {
                if !list.is_empty() {
                    list.retain(|v| *v != view);
                }
            }
        }
        let row = if s.row_owner.is_empty() {
            None
        } else {
            s.row_owner.get(&view).copied().and_then(|(src, key)| {
                let source = s.source_mut(src).ok()?;
                Some((src, key, source.decoded, source.has_tape(key)))
            })
        };
        let decoded = row.is_some_and(|r| r.2);
        let entity_opt = if decoded {
            match s.item(view, HK_VIEW)? {
                Item::DecodedRow(_) => None,
                Item::View(e) => Some(e.clone()),
                _ => return err(ERR_STALE_HANDLE, "not a view"),
            }
        } else {
            Some(s.view(view)?)
        };
        for slot in &tape.slots {
            s.view_window.insert(*slot, wh);
            if let Some(i) = s.unmounted_dirty.iter().position(|v| v == slot) {
                s.unmounted_dirty.swap_remove(i);
                s.dirty.entry(wh).or_default().push(*slot);
            }
        }
        let row_old = if entity_opt.is_none() {
            s.row_deps.get(&view).cloned().unwrap_or_default()
        } else {
            Vec::new()
        };
        Ok((row, entity_opt, row_old))
    })?;
    let decoded = row.is_some_and(|r| r.2);
    let deps = std::mem::take(&mut tape.deps);
    let deps_changed = match &entity_opt {
        Some(e) => e.read(cx).deps != deps,
        None => row_old != deps,
    };
    let subs = if deps_changed {
        let old = match &entity_opt {
            Some(e) => e.read(cx).deps.clone(),
            None => row_old,
        };
        let observers: Vec<_> = with(|s| {
            for dep in &old {
                if let Some(list) = s.dep_views.get_mut(dep) {
                    list.retain(|v| *v != view);
                }
            }
            deps.iter()
                .filter_map(|dep| {
                    let observe = s.state(*dep).ok()?.observe.clone();
                    s.dep_views.entry(*dep).or_default().push(view);
                    Some(observe)
                })
                .collect()
        });
        Some(observers.into_iter().map(|observe| observe(cx, view)).collect::<Vec<_>>())
    } else {
        None
    };
    if !deps.is_empty() {
        tape.deps = deps.clone();
    }
    let tape = Rc::new(tape);
    if decoded {
        let (src, key, _, had_tape) = row.unwrap();
        let mut entity_subs = None;
        with(|s| {
            if let Ok(source) = s.source_mut(src) {
                source.set_tape(key, tape);
            }
            if entity_opt.is_none() {
                if deps.is_empty() {
                    s.row_deps.remove(&view);
                } else {
                    s.row_deps.insert(view, deps.clone());
                }
                if let Some(subs) = subs {
                    s.set_decoded_subs(view, subs);
                }
            } else {
                entity_subs = subs;
            }
        });
        if let (Some(entity), Some(subs)) = (entity_opt, entity_subs) {
            entity.update(cx, |v, _| {
                v._subs = subs;
                v.deps = deps;
            });
        }
        let list = if had_tape { sources } else { first };
        if !list.contains(&src) {
            list.push(src);
        }
    } else {
        if let Some((src, key, _, _)) = row {
            with(|s| {
                if let Ok(source) = s.source_mut(src) {
                    source.set_tape(key, tape.clone());
                }
            });
            if !sources.contains(&src) {
                sources.push(src);
            }
        }
        let entity = entity_opt.unwrap();
        entity.update(cx, |v, cx| {
            v.tape = Some(tape);
            v.renders = 0;
            v.fast_tried = false;
            v.fast = None;
            if let Some(subs) = subs {
                v._subs = subs;
                v.deps = deps;
            }
            cx.notify();
        });
    }
    gk_meter::bump_tape(n);
    Ok(())
}

/// Sends a `K_VIEWS` turn requesting fresh tapes for `views` in window `wh`.
///
/// Request layout: `[wh: u64][n_views: u32][view: u64*][n_retired: u32][(view: u64, gen: u32)*]`.
pub fn describe(wh: u64, views: &[u64], window: &mut Window, cx: &mut App) {
    let mut req = UI.with(|u| {
        let mut buf = std::mem::take(&mut *u.event_buf.borrow_mut());
        buf.clear();
        buf.put_u64(wh);
        buf.put_u32(views.len() as u32);
        for v in views {
            buf.put_u64(*v);
        }

        put_retired_ui(u, &mut buf);
        buf
    });
    turn_raw(K_VIEWS, &req, Some(&mut *window), cx, |bytes, mut window, cx| {
        if bytes.is_empty() {
            return;
        }
        let mut r = R::new(bytes);
        let ret_len = r.u32() as usize;
        r.i += ret_len;
        let n_dirty = r.u32() as usize;
        let dirty_at = r.i;
        r.i += n_dirty * 8;
        let n_def = r.u32() as usize;
        for _ in 0..n_def {
            let n = r.u32() as usize;
            r.i += n;
        }
        let n_tapes = r.u32() as usize;
        if n_tapes > 0 {
            accept_tapes_from(wh, n_tapes, &mut r, cx);
        }
        if n_dirty > 0 {
            let mut dr = R::at(bytes, dirty_at);
            for _ in 0..n_dirty {
                mark_dirty(dr.u64(), window.as_deref_mut(), cx);
            }
        }
        if n_def > 0 {
            eprintln!(
                "gk: {n_def} write(s) from a view function were refused: a view function may not write"
            );
        }
    });
    if req.capacity() <= 64 * 1024 {
        req.clear();
        UI.with(|u| {
            let mut slot = u.event_buf.borrow_mut();
            if slot.capacity() < req.capacity() {
                *slot = req;
            }
        });
    }
}

/// Allocates a new `GkView` GPUI entity and returns its `HK_VIEW` handle.
pub fn new_view(cx: &mut App) -> u64 {
    let handle = with(|s| s.reserve(HK_VIEW));
    let entity = cx.new(|_| GkView {
        handle,
        tape: None,
        deps: Vec::new(),
        _subs: Vec::new(),
        renders: 0,
        fast_tried: false,
        fast: None,
    });
    with(|s| s.put(handle, Item::View(entity)));
    handle
}

/// Allocates a lightweight `HK_VIEW` handle backed by `Item::DecodedRow` without a GPUI entity.
pub fn new_decoded_row() -> u64 {
    with(|s| s.insert(HK_VIEW, Item::DecodedRow(Vec::new())))
}

/// Downgrades unpopulated `Item::View` handles to `Item::DecodedRow` when a row source enters
/// decoded mode, dropping the replaced GPUI entities outside the session borrow.
pub fn convert_to_decoded(views: &[u64]) {
    let dropped: Vec<Item> = with(|s| {
        let mut out = Vec::new();
        for &v in views {
            if let Ok(index) = s.slot(v, HK_VIEW) {
                if matches!(s.slots[index].item, Some(Item::View(_))) {
                    if let Some(old) = s.slots[index].item.replace(Item::DecodedRow(Vec::new())) {
                        out.push(old);
                    }
                }
            }
        }
        out
    });
    drop(dropped);
}

/// Opens a window sized `(width, height)` displaying `root`, applying `gk-meter` window options.
pub fn open_window(width: f32, height: f32, root: u64, cx: &mut App) -> GkResult<u64> {
    let options = gk_meter::window_options(width, height, cx);
    open_window_with(options, root, cx)
}

/// Opens a GPUI window with explicit `WindowOptions`, runs `K_WINDOW_INIT`, and performs the
/// initial `K_VIEWS` describe pass for `root` and any views dirtied during init.
pub fn open_window_with(options: gpui_kit::WindowOptions, root: u64, cx: &mut App) -> GkResult<u64> {
    let root_entity = with(|s| s.view(root))?;
    let wh = with(|s| s.reserve(HK_WINDOW));
    let opened = gpui_kit::open_window(options, cx, |window, cx| {
        with(|s| {
            s.put(wh, Item::Window(window.window_handle()));
            s.view_window.insert(root, wh);
            s.windows.push(wh);
        });
        gk_meter::attach(window, cx);

        let mut req = Vec::with_capacity(16);
        req.put_u64(wh);
        req.put_u64(root);
        with(|s| s.in_hook = Some(wh));
        let mut reply = turn(K_WINDOW_INIT, &req, Some(&mut *window), cx);
        process_reply(&mut reply, K_WINDOW_INIT, Some(&mut *window), cx);

        let mut first = vec![root];
        with(|s| {
            if let Some(d) = s.dirty.remove(&wh) {
                first.extend(d.into_iter().filter(|v| *v != root));
            }
        });
        describe(wh, &first, window, cx);

        let again = with(|s| {
            s.in_hook = None;
            s.frame_handler.contains(&wh)
                || s.dirty.get(&wh).is_some_and(|d| !d.is_empty())
                || s.has_pending_ahead(wh)
        });
        if again {
            ensure_hook(wh, Some(window), cx);
        }
        root_entity
    });
    match opened {
        Ok(_) => {
            cx.activate(true);
            Ok(wh)
        }
        Err(e) => err(ERR_UNSUPPORTED, format!("open_window: {e:#}")),
    }
}

/// Probes whether `entity` is currently readable without triggering a visible panic hook;
/// returns `ERR_BUSY` if GPUI already holds a mutable borrow of `entity` up the call stack.
pub fn probe<T: 'static, V>(
    entity: &Entity<T>,
    cx: &App,
    f: impl FnOnce(&T) -> V,
) -> GkResult<V> {
    UI.with(|u| u.quiet_panic.set(true));
    let got = catch_unwind(AssertUnwindSafe(|| {
        entity.read(cx);
    }));
    UI.with(|u| u.quiet_panic.set(false));
    let got = got.map(|()| f(entity.read(cx)));
    got.map_err(|_| {
        GkErr(
            ERR_BUSY,
            "this state is being updated by the component that called the handler; write to \
             it without reading it back, or do it in a deferred call"
                .into(),
        )
    })
}
