//! The runtime that stands in for the Rust types an app would define (DESIGN.md sections 7
//! to 9, docs/PROTOCOLS.md): the session, the handle table, the scoped context, turns,
//! `GkView`, the pre-frame hook, dirty marks, buffered writes, queued events and the panic
//! boundary.
//!
//! Everything here runs on the UI thread. No `RefCell` borrow of the session is held across a
//! callback into the app or across a call into GPUI that may run app code.

use crate::genapi;
use crate::ops::*;
use crate::tape::Tape;
use crate::wire::{Put, R};
use gpui_kit::{
    AnyElement, AnyWindowHandle, App, AppContext, Context, Entity, FocusHandle, IntoElement,
    Render, SharedString, Subscription, Window, div,
};
use std::any::Any;
use std::cell::{Cell, RefCell};
use std::collections::{HashMap, HashSet};
use std::ffi::c_void;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::rc::Rc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Instant;

pub type GkCallback = unsafe extern "C" fn(
    user: *mut c_void,
    cb: *mut GkCb,
    kind: u32,
    req: *const u8,
    len: usize,
) -> i32;

/// The reply slot of one callback. `gk_reply` copies into it.
pub struct GkCb {
    pub reply: Vec<u8>,
}

pub struct GkErr(pub i32, pub String);
pub type GkResult<T> = Result<T, GkErr>;

pub fn err<T>(code: i32, msg: impl Into<String>) -> GkResult<T> {
    Err(GkErr(code, msg.into()))
}

/// Rows of a list, filled by the app in one crossing and read by the list element in Rust.
/// An entity, so gpui-fast itself tracks which views read it.
pub struct RowStore {
    pub rows: Rc<RefCell<Vec<SharedString>>>,
}

/// A kit entity behind a handle. The entity is kept type-erased; `genapi::state::<T>` checks
/// the type when it takes it out.
pub struct StateItem {
    pub any: Rc<dyn Any>,
    pub type_name: &'static str,
    /// The window the state was created in: its mutators need one.
    pub window: Option<AnyWindowHandle>,
    /// Observes the entity for one view: the view is marked dirty when the entity notifies.
    pub observe: Rc<dyn Fn(&mut App, u64) -> Subscription>,
}

pub enum Item {
    View(Entity<GkView>),
    Window(AnyWindowHandle),
    State(StateItem),
    Rows(Entity<RowStore>),
    Sub(#[allow(dead_code)] Subscription),
    /// A timer (core/src/app.rs): dropping the task cancels it.
    Task(#[allow(dead_code)] gpui_kit::Task<()>),
    /// Anything whose drop undoes a registration (an app-wide action listener, core/src/app.rs).
    Guard(#[allow(dead_code)] Box<dyn Any>),
    Focus(FocusHandle, Option<AnyWindowHandle>),
    Source(Box<crate::source::Source>),
    /// A library object that is not an entity (core/src/objects.rs, PROTOCOLS.md section 13).
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

thread_local! {
    static TRACE: bool = std::env::var("GK_TRACE_TURNS").is_ok_and(|v| v == "1");
}

/// GK_TRACE_TURNS=1: one line on stderr per turn, dirty mark, pre-frame hook and accepted tape
/// (when a change lands in which frame; PROTOCOLS.md 11.32).
pub fn trace(what: impl FnOnce() -> String) {
    if TRACE.with(|t| *t) {
        eprintln!("gk-trace: {}", what());
    }
}

/// One app view: an entity that holds a tape and decodes it when gpui-fast builds the view.
pub struct GkView {
    pub handle: u64,
    pub tape: Option<Rc<Tape>>,
    deps: Vec<u64>,
    _subs: Vec<Subscription>,
}

impl Render for GkView {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let t0 = Instant::now();
        let handle = self.handle;
        let el: AnyElement = match self.tape.clone() {
            Some(tape) => guard("decoding a view's tape", || {
                genapi::root(&tape, handle, window, cx)
            })
            .unwrap_or_else(|| div().into_any_element()),
            None => div().into_any_element(),
        };
        let ns = t0.elapsed().as_nanos() as u64;
        gk_meter::bump(|c| {
            c.decodes += 1;
            c.decode_ns += ns;
        });
        el
    }
}

/// An event raised from inside an entity's own update, delivered when that update has ended.
struct Queued {
    req: Vec<u8>,
    window: Option<AnyWindowHandle>,
}

/// A map keyed by handle. Handles are the library's own numbers (no attacker picks them), so the
/// hash is a multiply and a fold rather than SipHash: a ROWS turn looks several maps up for every
/// row it binds.
pub type IdMap<V> = HashMap<u64, V, std::hash::BuildHasherDefault<IdHasher>>;
pub type IdSet = HashSet<u64, std::hash::BuildHasherDefault<IdHasher>>;

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

pub struct Session {
    pub callback: GkCallback,
    pub user: *mut c_void,
    slots: Vec<Slot>,
    free: Vec<u32>,
    /// The window each mounted view is drawn in.
    view_window: IdMap<u64>,
    /// Dirty mounted views per window, in the order they were marked.
    dirty: IdMap<Vec<u64>>,
    /// Dirty views that are in no window yet.
    unmounted_dirty: Vec<u64>,
    /// Views whose current tape read a state (the tape's dependency list), by state handle.
    dep_views: IdMap<Vec<u64>>,
    /// Windows whose pre-frame hook is registered for the next frame.
    hooked: IdSet,
    /// The window whose pre-frame hook is running.
    in_hook: Option<u64>,
    /// Windows whose app asked for a FRAME turn before every frame.
    pub frame_handler: IdSet,
    /// Buffered writes of EVENT turns, each with the window its turn had (a write such as
    /// WINDOW_CLOSE with 0, or a kit call that needs the window, gets that one, not the first).
    deferred: Vec<(Vec<u8>, Option<AnyWindowHandle>)>,
    deferred_scheduled: bool,
    queued: Vec<Queued>,
    queued_scheduled: bool,
    pub windows: Vec<u64>,
    frame_no: u64,
    pub debug: bool,
    /// Live row sources (core/src/source.rs).
    pub sources: Vec<u64>,
    /// Row views by the source they belong to.
    pub row_owner: IdMap<u64>,
    /// The current tape of each row view, kept here as well as in its entity so a delegate shim
    /// can draw a row without reading the row's entity (in gpui-fast an entity read while a
    /// view draws makes that view depend on the entity: a table would depend on every cell it
    /// drew). Set in `accept`, dropped in `release`.
    pub row_tapes: IdMap<Rc<Tape>>,
    /// What a decoded row's tape depended on (its entity is not read: see `accept`).
    pub row_deps: IdMap<Vec<u64>>,
}

#[derive(Clone, Copy)]
struct Scope {
    app: *mut App,
    window: Option<*mut Window>,
    /// The gk_cb of this turn's callback: the only one gk_reply accepts while it runs.
    cb: *mut GkCb,
}

thread_local! {
    static SESSION: RefCell<Option<Session>> = const { RefCell::new(None) };
    static SCOPES: RefCell<Vec<Scope>> = const { RefCell::new(Vec::new()) };
    /// Set while a lease probe runs, so the panic hook stays silent.
    pub static QUIET_PANIC: Cell<bool> = const { Cell::new(false) };
    /// The last panic's message and place, written by the panic hook.
    static PANIC_MSG: RefCell<String> = const { RefCell::new(String::new()) };
    /// True on the thread that called gk_run.
    pub static IS_UI: Cell<bool> = const { Cell::new(false) };
}

pub static POISONED: AtomicBool = AtomicBool::new(false);

pub fn install(callback: GkCallback, user: *mut c_void) {
    let debug = std::env::var_os("GK_DEBUG").is_some();
    IS_UI.with(|u| u.set(true));
    SESSION.with(|s| {
        *s.borrow_mut() = Some(Session {
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
            row_tapes: IdMap::default(),
            row_deps: IdMap::default(),
        });
    });
}

pub fn with<T>(f: impl FnOnce(&mut Session) -> T) -> T {
    SESSION.with(|s| f(s.borrow_mut().as_mut().expect("gk: no session; call gk_run first")))
}

// --- the handle table -------------------------------------------------------------------

const GEN_MASK: u32 = 0xff_ffff;

impl Session {
    /// Reserves a handle before the object exists, so the object can carry its own handle.
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

    pub fn put(&mut self, handle: u64, item: Item) {
        self.slots[(handle & 0xffff_ffff) as usize].item = Some(item);
    }

    /// Stores a new object and returns its handle.
    pub fn insert(&mut self, kind: u8, item: Item) -> u64 {
        let handle = self.reserve(kind);
        self.put(handle, item);
        handle
    }

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
            // The kind is right but no such object was ever made (a made-up or corrupted
            // number): it used to read "is a row source, not a row source".
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

    /// Releases any handle: the object is dropped (by the caller, outside the session
    /// borrow), the generation goes up and the index is free for reuse.
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
        match kind {
            HK_VIEW => {
                self.view_window.remove(&handle);
                for list in self.dirty.values_mut() {
                    list.retain(|v| *v != handle);
                }
                self.unmounted_dirty.retain(|v| *v != handle);
                for list in self.dep_views.values_mut() {
                    list.retain(|v| *v != handle);
                }
                self.row_owner.remove(&handle);
                self.row_tapes.remove(&handle);
                self.row_deps.remove(&handle);
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

    pub fn view(&self, handle: u64) -> GkResult<Entity<GkView>> {
        match self.item(handle, HK_VIEW)? {
            Item::View(e) => Ok(e.clone()),
            _ => err(ERR_STALE_HANDLE, "not a view"),
        }
    }

    pub fn window(&self, handle: u64) -> GkResult<AnyWindowHandle> {
        match self.item(handle, HK_WINDOW)? {
            Item::Window(w) => Ok(*w),
            _ => err(ERR_STALE_HANDLE, "not a window"),
        }
    }

    pub fn state(&self, handle: u64) -> GkResult<&StateItem> {
        match self.item(handle, HK_STATE)? {
            Item::State(s) => Ok(s),
            _ => err(ERR_STALE_HANDLE, "not a state"),
        }
    }

    /// A library object (kind OBJ, core/src/objects.rs).
    pub fn obj(&self, handle: u64) -> GkResult<&crate::objects::ObjItem> {
        match self.item(handle, HK_OBJ)? {
            Item::Obj(o) => Ok(o),
            _ => err(ERR_STALE_HANDLE, "not a library object"),
        }
    }

    /// Live handles of one kind.
    pub fn count(&self, kind: u8) -> usize {
        self.slots.iter().filter(|s| s.kind == kind && s.item.is_some()).count()
    }

    pub fn rows(&self, handle: u64) -> GkResult<Entity<RowStore>> {
        match self.item(handle, HK_ROWS)? {
            Item::Rows(e) => Ok(e.clone()),
            _ => err(ERR_STALE_HANDLE, "not a row store"),
        }
    }

    pub fn focus(&self, handle: u64) -> GkResult<(FocusHandle, Option<AnyWindowHandle>)> {
        match self.item(handle, HK_FOCUS)? {
            Item::Focus(f, w) => Ok((f.clone(), *w)),
            _ => err(ERR_STALE_HANDLE, "not a focus handle"),
        }
    }

    pub fn source_mut(&mut self, handle: u64) -> GkResult<&mut crate::source::Source> {
        let index = self.slot(handle, HK_SOURCE)?;
        match &mut self.slots[index].item {
            Some(Item::Source(s)) => Ok(&mut **s),
            _ => err(ERR_STALE_HANDLE, "not a row source"),
        }
    }

    /// The number of the frame whose pre-frame hook ran last.
    pub fn frame_no(&self) -> u64 {
        self.frame_no
    }

    /// Draws a view in window `wh` (a row view, which no tape slots).
    pub fn mount(&mut self, view: u64, wh: u64) {
        self.view_window.insert(view, wh);
    }

    /// The handle of kind WINDOW of an open window.
    pub fn window_of(&self, any: AnyWindowHandle) -> Option<u64> {
        self.windows.iter().copied().find(|w| self.window(*w).ok() == Some(any))
    }

    /// Live handles by kind, for GK_DEBUG.
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

// --- the panic boundary -----------------------------------------------------------------

/// Installs the hook that remembers a panic's message for the boundary to report.
pub fn install_panic_hook() {
    static ONCE: std::sync::Once = std::sync::Once::new();
    ONCE.call_once(|| {
        let previous = std::panic::take_hook();
        std::panic::set_hook(Box::new(move |info| {
            if QUIET_PANIC.with(|q| q.get()) {
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
            PANIC_MSG.with(|m| *m.borrow_mut() = format!("{what}{place}"));
            previous(info);
        }));
    });
}

pub fn panic_message() -> String {
    PANIC_MSG.with(|m| m.borrow().clone())
}

/// Runs a piece of the library that GPUI calls (a render, a listener, a hook, a task). A
/// panic inside it does not unwind any further: nothing may unwind through GPUI's platform
/// loop into C frames (on macOS it would abort), and never into the app.
pub fn guard<T>(what: &'static str, f: impl FnOnce() -> T) -> Option<T> {
    match catch_unwind(AssertUnwindSafe(f)) {
        Ok(v) => Some(v),
        Err(_) => {
            fatal(what);
            None
        }
    }
}

/// The library panicked outside a `gk_call`: poison it, tell the binding (the PANIC turn;
/// the binding prints the message and exits 1), and if the binding returns, exit here.
pub fn fatal(what: &str) {
    POISONED.store(true, Ordering::SeqCst);
    let msg = format!("{} (while {what})", panic_message());
    let session = SESSION.with(|s| s.try_borrow().ok().and_then(|s| s.as_ref().map(|s| (s.callback, s.user))));
    if let Some((callback, user)) = session {
        let mut req = Vec::with_capacity(msg.len() + 4);
        req.put_str(&msg);
        let mut cb = GkCb { reply: Vec::new() };
        // SAFETY: the callback is the app's, given to gk_run; req and cb live across the call.
        unsafe { callback(user, &mut cb, K_PANIC, req.as_ptr(), req.len()) };
    }
    eprintln!("gk: the library panicked: {msg}");
    std::process::exit(1);
}

// --- scoped context ---------------------------------------------------------------------

/// The `&mut App` and `&mut Window` of the innermost callback in progress, for `gk_call`.
///
/// SAFETY of the pointers: a scope is pushed by `turn` from live `&mut` references that the
/// caller does not touch until the callback has returned, and popped before `turn` returns.
/// `gk_call` runs only inside that callback, on the same thread.
/// Whether `cb` is the gk_cb of the callback running now (the innermost turn). A gk_cb kept
/// from an earlier callback points into a stack frame that has returned.
pub fn current_cb(cb: *mut GkCb) -> bool {
    SCOPES.with(|s| s.borrow().last().is_some_and(|t| t.cb == cb))
}

pub fn scope<'a>() -> Option<(&'a mut App, Option<&'a mut Window>)> {
    let top = SCOPES.with(|s| s.borrow().last().copied())?;
    // SAFETY: see above.
    unsafe { Some((&mut *top.app, top.window.map(|w| &mut *w))) }
}

/// Runs `f` with the window `any`: the one in hand if it is that window, else by handle.
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

// --- turns ------------------------------------------------------------------------------

/// What a turn's reply carries (docs/PROTOCOLS.md section 3).
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

/// One callback into the app: all app code runs inside one of these.
pub fn turn(kind: u32, req: &[u8], window: Option<&mut Window>, cx: &mut App) -> Reply {
    let (callback, user) = with(|s| (s.callback, s.user));
    trace(|| format!("turn kind={kind} window={}", window.is_some()));
    let mut cb = GkCb { reply: Vec::new() };
    SCOPES.with(|s| {
        s.borrow_mut().push(Scope {
            app: cx as *mut App,
            window: window.map(|w| w as *mut Window),
            cb: &mut cb as *mut GkCb,
        })
    });
    let t0 = Instant::now();
    // SAFETY: the callback is the app's, given to gk_run; req and cb live across the call.
    let rc = unsafe { callback(user, &mut cb, kind, req.as_ptr(), req.len()) };
    let ns = t0.elapsed().as_nanos() as u64;
    SCOPES.with(|s| s.borrow_mut().pop());
    gk_meter::bump(|c| {
        c.callbacks += 1;
        c.callback_ns += ns;
        match kind {
            K_FRAME => c.frame_turns += 1,
            K_VIEWS => c.views_turns += 1,
            K_EVENT => c.event_turns += 1,
            K_WAKE => c.wake_turns += 1,
            _ => {}
        }
    });
    if rc != 0 {
        eprintln!("gk: callback kind {kind} returned {rc}");
    }
    parse_reply(&cb.reply)
}

/// Applies what a reply carries besides its return value and tapes: dirty marks and
/// buffered writes. When the writes land depends on the turn (DESIGN.md section 7):
/// an EVENT turn may run inside an entity's update, so its writes wait for the outermost
/// update to end; every other turn runs outside any update and its writes are applied now,
/// so a FRAME turn's writes are in place before the VIEWS turn of the same frame.
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
            let wh = window.as_deref().map(|w| w.window_handle());
            let schedule = with(|s| {
                s.deferred.extend(deferred.into_iter().map(|op| (op, wh)));
                !std::mem::replace(&mut s.deferred_scheduled, true)
            });
            if schedule {
                // Runs when the outermost update has ended, so no entity is on lease, and
                // before the next draw.
                cx.defer(apply_deferred);
            }
        }
        _ => apply_ops(deferred, window, cx),
    }
}

/// Puts a write back: it runs when the outermost update has ended.
pub fn defer_op(op: Vec<u8>, cx: &mut App) {
    let schedule = with(|s| {
        s.deferred.push((op, None));
        !std::mem::replace(&mut s.deferred_scheduled, true)
    });
    if schedule {
        cx.defer(apply_deferred);
    }
}

fn apply_deferred(cx: &mut App) {
    let ops = with(|s| {
        s.deferred_scheduled = false;
        std::mem::take(&mut s.deferred)
    });
    guard("applying buffered writes", || {
        // In order, each run of writes from one window inside that window's update, so a write
        // gets its turn's window as it would have during the turn; a window closed since, or
        // none, applies them without one (as before 12 Nov 2026, when every buffered write of an
        // EVENT turn lost its window: gk.close_window(cx) in a handler failed with "this call
        // needs a window" and was dropped).
        let mut ops = ops.into_iter().peekable();
        while let Some((op, wh)) = ops.next() {
            let mut run = vec![op];
            while let Some((_, next)) = ops.peek() {
                if *next != wh {
                    break;
                }
                run.push(ops.next().unwrap().0);
            }
            match wh {
                Some(any) => {
                    let mut left = Some(run);
                    let _ = any.update(cx, |_, window, cx| apply_ops(left.take().unwrap_or_default(), Some(window), cx));
                    if let Some(run) = left {
                        apply_ops(run, None, cx);
                    }
                }
                None => apply_ops(run, None, cx),
            }
        }
    });
}

fn apply_ops(ops: Vec<Vec<u8>>, mut window: Option<&mut Window>, cx: &mut App) {
    let mut out = Vec::new();
    for op in ops {
        gk_meter::bump(|c| c.deferred_ops += 1);
        out.clear();
        if let Err(GkErr(code, msg)) =
            crate::calls::call(&op, window.as_deref_mut(), cx, &mut out, true)
        {
            eprintln!("gk: a change a handler asked for could not be made: {msg}");
            if code == ERR_STALE_HANDLE || code == ERR_BAD_OP {
                // A mistake in app code (a released or wrong handle, an op this library does
                // not know), found after the handler that made it has returned. It stops
                // here, like every other misuse; it is not left to show wrong content.
                eprintln!(
                    "    The write was buffered by a handler and applied after it returned, so its \
                     line is not known here. Run with GK_DEBUG=1: every write is then sent at \
                     once and the program stops at the line that made it."
                );
                std::process::exit(2);
            }
        }
    }
}

/// One EVENT turn for a request already encoded (view, generation, handler, shape, payload).
/// Returns the handler's return value as bytes (empty: none).
pub fn fire_req(req: &[u8], mut window: Option<&mut Window>, cx: &mut App) -> Vec<u8> {
    guard("delivering an event", || {
        let mut reply = turn(K_EVENT, req, window.as_deref_mut(), cx);
        process_reply(&mut reply, K_EVENT, window.as_deref_mut(), cx);
        reply.ret
    })
    .unwrap_or_default()
}

/// An event raised inside an entity's own update (a delegate method): delivered when the
/// outermost update has ended, before the next draw, never synchronously. The handler gets
/// the window of its view in scope; a kept handler (view 0) gets the app's first window.
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

// --- dirty views and the pre-frame hook ---------------------------------------------------

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
            if s.view(view).is_ok() && !s.unmounted_dirty.contains(&view) {
                s.unmounted_dirty.push(view);
            }
            None
        }
    });
    if let Some(w) = target {
        ensure_hook(w, window, cx);
    }
}

/// `mark_dirty` for views just made and mounted in window `wh` (the row views a ROWS turn
/// binds): none of them can be in the dirty list yet, so a batch of a few hundred costs no
/// search of that list per view.
pub fn mark_dirty_new(wh: u64, views: &[u64], window: &mut Window, cx: &mut App) {
    if views.is_empty() {
        return;
    }
    trace(|| format!("dirty new views={} window={wh:#x}", views.len()));
    with(|s| {
        let mut list = s.dirty.remove(&wh).unwrap_or_default();
        for v in views {
            if s.view_window.get(v) == Some(&wh) {
                list.push(*v);
            }
        }
        s.dirty.insert(wh, list);
    });
    ensure_hook(wh, Some(window), cx);
}

/// A state was written through the binding. Inside a pre-frame hook GPUI's observers would
/// fire only after the hook, one frame late for the views that read the state, so the
/// runtime marks those views itself (DESIGN.md section 7, rule 2).
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

/// Registers the window's pre-frame hook for the next frame (which also schedules a frame).
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
        // The window is on loan to an update further up the stack; try again when it is back.
        with(|s| s.hooked.remove(&wh));
        cx.defer(move |cx| ensure_hook(wh, None, cx));
    }
}

/// Runs inside the platform's frame request, before gpui-fast decides whether to draw
/// (window.rs: next-frame callbacks, then `invalidator.is_dirty()`).
fn pre_frame(wh: u64, window: &mut Window, cx: &mut App) {
    guard("running the pre-frame hook", || {
        // Temporary library objects of the turns before this frame (PROTOCOLS.md section 13).
        crate::objects::sweep();
        let (frame_handler, frame_no) = with(|s| {
            s.hooked.remove(&wh);
            s.in_hook = Some(wh);
            s.frame_no += 1;
            (s.frame_handler.contains(&wh), s.frame_no)
        });
        if frame_handler {
            let mut req = Vec::with_capacity(16);
            req.put_u64(wh);
            req.put_u64(frame_no);
            let mut reply = turn(K_FRAME, &req, Some(&mut *window), cx);
            // The FRAME turn's writes land here, and the views that read what was written
            // are marked dirty by `written`, before the VIEWS turn below.
            process_reply(&mut reply, K_FRAME, Some(&mut *window), cx);
        }
        // Rows drawn as placeholders in the last frame get their views here, marked dirty,
        // and are described in the VIEWS turn below (docs/PROTOCOLS.md section 9).
        crate::source::rows_turn(wh, window, cx);
        let dirty = with(|s| s.dirty.remove(&wh).unwrap_or_default());
        trace(|| format!("pre_frame window={wh:#x} frame={frame_no} dirty={}", dirty.len()));
        if !dirty.is_empty() {
            describe(wh, &dirty, window, cx);
        }
        let again = with(|s| {
            s.in_hook = None;
            s.frame_handler.contains(&wh) || s.dirty.get(&wh).is_some_and(|d| !d.is_empty())
        });
        if again {
            ensure_hook(wh, Some(window), cx);
        }
    });
}

/// (view, generation) of the descriptions dropped since the last VIEWS turn (`GenPin`'s Drop,
/// which may run on any thread).
static RETIRED: std::sync::Mutex<Vec<(u64, u32)>> = std::sync::Mutex::new(Vec::new());

/// A view's description of this generation is gone (`GenPin`'s Drop): its handler table can go.
pub fn retire(view: u64, generation: u32) {
    if let Ok(mut r) = RETIRED.lock() {
        r.push((view, generation));
    }
}

/// One VIEWS turn: the binding runs the view procedure of each listed view (and of views they
/// slot that have no tape yet) and replies with the tapes that changed. Each changed tape is
/// set on its entity and that entity alone is notified.
pub fn describe(wh: u64, views: &[u64], window: &mut Window, cx: &mut App) {
    let retired = RETIRED.lock().map(|mut r| std::mem::take(&mut *r)).unwrap_or_default();
    let mut req = Vec::with_capacity(16 + views.len() * 8 + retired.len() * 12);
    req.put_u64(wh);
    req.put_u32(views.len() as u32);
    for v in views {
        req.put_u64(*v);
    }
    // The descriptions dropped since the last VIEWS turn (`GenPin`'s Drop): their handler tables
    // can go.
    req.put_u32(retired.len() as u32);
    for (view, generation) in retired {
        req.put_u64(view);
        req.put_u32(generation);
    }
    let mut reply = turn(K_VIEWS, &req, Some(&mut *window), cx);
    // Sources whose watchers must draw again: rows they drew as placeholders arrived, or rows
    // they show were described again. The first tape of a row bound ahead of the scroll
    // (`source::want`) was never drawn and redraws nothing: notifying the table for it made
    // the kit draw every visible cell again for five frames after each batch, which was the
    // p95 of the table scene (1.15 to 1.19 of Rust; 11 Nov 2026).
    let mut sources = Vec::new();
    let mut first = Vec::new();
    for (view, generation, bytes) in std::mem::take(&mut reply.tapes) {
        // A row view: its source, whether a shim decodes the source's rows, and whether it had a
        // tape before this one.
        let row = with(|s| {
            let src = s.row_owner.get(&view).copied()?;
            let decoded = s.source_mut(src).map(|x| x.decoded).unwrap_or(false);
            Some((src, decoded, s.row_tapes.contains_key(&view)))
        });
        if let Err(GkErr(code, msg)) = accept(wh, view, generation, bytes, row.map(|r| r.1), cx) {
            eprintln!(
                "gk: a view's description was refused ({msg}; error {code}); the view keeps what it showed. \
                 The app's go/ folder and the library are from different downloads, or this is a bug in the binding."
            );
        } else if let Some((src, decoded, had_tape)) = row {
            let list = if had_tape || !decoded { &mut sources } else { &mut first };
            if !list.contains(&src) {
                list.push(src);
            }
        }
    }
    for src in first {
        if crate::source::take_missed(src) && !sources.contains(&src) {
            sources.push(src);
        }
    }
    for src in &sources {
        crate::source::take_missed(*src);
    }
    // The entities that drew those rows as placeholders draw them now, in this frame.
    crate::source::notify_watchers(&sources, cx);
    process_reply(&mut reply, K_VIEWS, Some(window), cx);
}

/// Accepts a view's new tape. `decoded`: the view is a row of a source (Some), whose rows a
/// delegate shim decodes itself (true).
fn accept(wh: u64, view: u64, generation: u32, bytes: Vec<u8>, decoded: Option<bool>, cx: &mut App) -> GkResult<()> {
    trace(|| format!("tape view={view:#x} generation={generation} bytes={}", bytes.len()));
    let n = bytes.len() as u64;
    let mut tape = match Tape::parse(bytes, generation) {
        Ok(tape) => tape,
        Err(e) => {
            retire(view, generation); // never held: its handlers cannot fire
            return Err(e);
        }
    };
    tape.pin = Some(std::sync::Arc::new(crate::tape::GenPin { view, generation }));
    // A row a delegate shim decodes itself (`Source::decoded`) is never drawn as a view: its
    // tape goes to the session's table and its entity is left alone unless what it depends on
    // changed. Updating and notifying it would make the window track one more entity for every
    // row described (a few hundred per batch while a table scrolls) and drop them the frame
    // after; its source's watchers redraw instead (`describe`).
    let is_row = decoded.is_some();
    let decoded = decoded.unwrap_or(false);
    let (entity, row_old) = with(|s| -> GkResult<_> {
        let entity = s.view(view)?;
        for slot in &tape.slots {
            s.view_window.insert(*slot, wh);
            if let Some(i) = s.unmounted_dirty.iter().position(|v| v == slot) {
                s.unmounted_dirty.swap_remove(i);
                s.dirty.entry(wh).or_default().push(*slot);
            }
        }
        let row_old = if decoded { s.row_deps.get(&view).cloned().unwrap_or_default() } else { Vec::new() };
        Ok((entity, row_old))
    })?;
    // Kit state the view procedure read: observe it, so the view is described again when it
    // changes (section 9; correction 2 of section 1).
    let deps = std::mem::take(&mut tape.deps);
    let old = if decoded { row_old } else { entity.read(cx).deps.clone() };
    let subs = if old != deps {
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
    tape.deps = deps.clone();
    let tape = Rc::new(tape);
    if is_row {
        with(|s| {
            s.row_tapes.insert(view, tape.clone());
            if decoded {
                if deps.is_empty() {
                    s.row_deps.remove(&view); // read as empty
                } else {
                    s.row_deps.insert(view, deps.clone());
                }
            }
        });
    }
    if decoded {
        if let Some(subs) = subs {
            entity.update(cx, |v, _| {
                v._subs = subs;
                v.deps = deps;
            });
        }
        gk_meter::bump(|c| {
            c.tapes += 1;
            c.tape_bytes += n;
        });
        return Ok(());
    }
    entity.update(cx, |v, cx| {
        v.tape = Some(tape);
        if let Some(subs) = subs {
            v._subs = subs;
            v.deps = deps;
        }
        cx.notify();
    });
    gk_meter::bump(|c| {
        c.tapes += 1;
        c.tape_bytes += n;
    });
    Ok(())
}

pub fn new_view(cx: &mut App) -> u64 {
    let handle = with(|s| s.reserve(HK_VIEW));
    let entity = cx.new(|_| GkView {
        handle,
        tape: None,
        deps: Vec::new(),
        _subs: Vec::new(),
    });
    with(|s| s.put(handle, Item::View(entity)));
    handle
}

pub fn open_window(width: f32, height: f32, root: u64, cx: &mut App) -> GkResult<u64> {
    let options = gk_meter::window_options(width, height, cx);
    open_window_with(options, root, cx)
}

/// Opens a window with the options the app gave (core/src/app.rs, call op WINDOW_OPEN).
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
        // The view's init procedure, with the window in hand: kit state that needs a window
        // (InputState::new) is created here. Its writes are applied before the first
        // description, so the first frame is complete.
        let mut req = Vec::with_capacity(16);
        req.put_u64(wh);
        req.put_u64(root);
        with(|s| s.in_hook = Some(wh));
        let mut reply = turn(K_WINDOW_INIT, &req, Some(&mut *window), cx);
        process_reply(&mut reply, K_WINDOW_INIT, Some(&mut *window), cx);
        // Describe the root and everything it slots.
        let mut first = vec![root];
        with(|s| {
            if let Some(d) = s.dirty.remove(&wh) {
                first.extend(d.into_iter().filter(|v| *v != root));
            }
        });
        describe(wh, &first, window, cx);
        // What the init turn asked for while the window was being built (a frame handler,
        // views marked dirty) starts with the next frame.
        let again = with(|s| {
            s.in_hook = None;
            s.frame_handler.contains(&wh) || s.dirty.get(&wh).is_some_and(|d| !d.is_empty())
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

/// Reads an entity, or reports that it is on lease (being updated further up the stack):
/// GPUI panics there before anything is changed, and the probe turns that into an error.
pub fn probe<T: 'static, V>(
    entity: &Entity<T>,
    cx: &App,
    f: impl FnOnce(&T) -> V,
) -> GkResult<V> {
    // Only the lease is probed quietly; a panic inside `f` is a real one and is reported.
    QUIET_PANIC.with(|q| q.set(true));
    let got = catch_unwind(AssertUnwindSafe(|| {
        entity.read(cx);
    }));
    QUIET_PANIC.with(|q| q.set(false));
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
