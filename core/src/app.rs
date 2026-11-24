//! Application-level runtime services: dynamic actions, keybindings, timers, window management,
//! lifecycle hooks, clipboard access, and reduced-motion settings.
//!
//! # Action Namespace and Dispatch Protocol
//!
//! - **Unified action IDs**: `ACTIONS` stores 1-based entries `(name, json_data, Option<Box<dyn Action>>)`.
//!   Host-defined actions use plain indices (`id & BUILTIN == 0`) and dispatch through the single
//!   Rust type [`GkAction`]. Upstream built-in actions constructed via `cx.build_action` or
//!   discovered via [`id_of`] set the [`BUILTIN`] (`0x8000_0000`) flag bit so callers can check
//!   `IsBuiltin()` and cannot accidentally register app-wide `GkAction` listeners for built-in types.
//! - **Propagation invariant**: because every custom host action shares the Rust type `GkAction`,
//!   GPUI invokes every `on_action::<GkAction>` listener on the bubble path. [`action_listener`]
//!   checks `a.id == id` and calls `cx.propagate()` on mismatch so sibling/ancestor handlers for
//!   other custom action IDs still run.
//! - **Subscription cleanup**: app-wide action listeners (`C_ON_ACTION`), window-closed hooks
//!   (`C_ON_WINDOW_CLOSED`), close-veto hooks (`C_ON_WINDOW_SHOULD_CLOSE`), and quit hooks
//!   (`C_ON_APP_QUIT`) store RAII drop guards ([`Unlisten`], [`UnlistenAction`]) in `HK_SUB` slots
//!   so `C_RELEASE` cleanly unregisters them.
//! - **Window flags (`C_WINDOW_OPEN`)**: `flags` packs 6 bits: bit 0 = not resizable, bit 1 = not
//!   minimizable, bit 2 = not movable, bit 3 = unfocused, bit 4 = hidden (`show = false`), bit 5 =
//!   use `gpui_kit::component::TitleBar::window_options()`. `TITLES` caches each window's title
//!   as a fallback for platforms/headless drivers where `window.window_title()` returns empty.

use crate::genapi::{self, Call, HandlerRef};
use crate::ops::*;
use crate::rt::{self, GkResult, Item, err};
use crate::wire::{Put, R};
use gpui_kit::{
    Action, AnyWindowHandle, App, Bounds, ClipboardItem, KeyBinding,
    KeyBindingContextPredicate, KeyContext, Window, WindowBounds, WindowKind, point, px, size,
};
use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::rc::Rc;
use std::time::Duration;

/// Bit flag set on action IDs that wrap an upstream built-in GPUI/gpui-kit `Action`.
pub const BUILTIN: u32 = 0x8000_0000;

/// Single GPUI `Action` type representing all dynamically registered host actions by `id`.
#[derive(Clone, PartialEq, Debug)]
pub struct GkAction {
    pub id: u32,
}

impl Action for GkAction {
    fn boxed_clone(&self) -> Box<dyn Action> {
        Box::new(self.clone())
    }
    fn partial_eq(&self, action: &dyn Action) -> bool {
        action.as_any().downcast_ref::<GkAction>().is_some_and(|a| a.id == self.id)
    }
    fn name(&self) -> &'static str {
        name_of(self.id)
    }
    fn name_for_type() -> &'static str {
        "gk::Action"
    }
    fn build(_value: gpui_kit::private::serde_json::Value) -> gpui_kit::Result<Box<dyn Action>> {
        Err(gpui_kit::private::anyhow::anyhow!("an app action of the binding is made with gk.NewAction, not from a keymap file"))
    }
}

struct Entry {
    name: &'static str,
    data: String,
    builtin: Option<Box<dyn Action>>,
}

thread_local! {
    static ACTIONS: RefCell<Vec<Entry>> = const { RefCell::new(Vec::new()) };
    /// App-wide `GkAction` listeners keyed by action ID.
    static GLOBAL: RefCell<HashMap<u32, Vec<(u64, HandlerRef)>>> = RefCell::new(HashMap::new());
    static GLOBAL_INSTALLED: Cell<bool> = const { Cell::new(false) };
    /// Per-window closed listeners keyed by window handle.
    static CLOSED: RefCell<HashMap<u64, Vec<(u64, HandlerRef)>>> = RefCell::new(HashMap::new());
    /// Per-window close-veto callbacks keyed by window handle.
    static SHOULD_CLOSE: RefCell<HashMap<u64, Vec<(u64, HandlerRef)>>> = RefCell::new(HashMap::new());
    /// App-quit hooks (stored under key 0).
    static ON_QUIT: RefCell<HashMap<u64, Vec<(u64, HandlerRef)>>> = RefCell::new(HashMap::new());
    /// Fallback title cache for platforms or headless windows where `window.window_title()` is empty.
    static TITLES: RefCell<HashMap<gpui_kit::WindowId, String>> = RefCell::new(HashMap::new());
    static QUIT_ON_LAST: Cell<bool> = const { Cell::new(true) };
    /// Tracks which windows already have a GPUI `on_window_should_close` hook installed.
    static INSTALLED_CLOSE: RefCell<std::collections::HashSet<u64>> = RefCell::new(std::collections::HashSet::new());
}

fn name_of(id: u32) -> &'static str {
    ACTIONS.with(|a| a.borrow().get(((id & !BUILTIN) as usize).wrapping_sub(1)).map(|e| e.name).unwrap_or("gk::Unknown"))
}

/// Resolves an action ID to a boxed GPUI `Action` (either a cloned built-in or a [`GkAction`]).
pub fn boxed(id: u32) -> GkResult<Box<dyn Action>> {
    ACTIONS.with(|a| {
        let a = a.borrow();
        match a.get(((id & !BUILTIN) as usize).wrapping_sub(1)) {
            Some(Entry { builtin: Some(b), .. }) if id & BUILTIN != 0 => Ok(b.boxed_clone()),
            Some(Entry { builtin: None, .. }) if id & BUILTIN == 0 => Ok(Box::new(GkAction { id }) as Box<dyn Action>),
            _ => err(ERR_STALE_HANDLE, format!("action {id:#x} names no action this library made (make actions with gk.NewAction)")),
        }
    })
}

/// Returns the wire ID for `action`, interning unseen built-in actions in the table.
pub fn id_of(action: &dyn Action) -> u32 {
    if let Some(a) = action.as_any().downcast_ref::<GkAction>() {
        return a.id;
    }
    ACTIONS.with(|t| {
        let mut t = t.borrow_mut();
        if let Some(i) = t.iter().position(|e| e.builtin.as_ref().is_some_and(|b| b.partial_eq(action))) {
            return (i as u32 + 1) | BUILTIN;
        }
        t.push(Entry { name: action.name(), data: String::new(), builtin: Some(action.boxed_clone()) });
        t.len() as u32 | BUILTIN
    })
}

/// Registers or deduplicates an action `(name, json_data)`. If GPUI recognizes `name` as a
/// built-in action, its constructed `Box<dyn Action>` is cached with the [`BUILTIN`] flag bit set;
/// otherwise a new host `GkAction` ID is allocated.
fn action_new(name: &str, data: &str, cx: &App) -> GkResult<u32> {
    if name.is_empty() {
        return err(ERR_BAD_OP, "an action needs a name (\"app::Save\")");
    }
    let found = ACTIONS.with(|t| {
        t.borrow()
            .iter()
            .position(|e| e.name == name && e.data == data)
            .map(|i| (i as u32 + 1) | if t.borrow()[i].builtin.is_some() { BUILTIN } else { 0 })
    });
    if let Some(id) = found {
        return Ok(id);
    }
    let json = if data.is_empty() {
        None
    } else {
        match gpui_kit::private::serde_json::from_str(data) {
            Ok(v) => Some(v),
            Err(e) => return err(ERR_BAD_OP, format!("action {name}: its data is not JSON: {e}")),
        }
    };
    let builtin = match cx.build_action(name, json) {
        Ok(b) => Some(b),
        Err(_) if data.is_empty() => None,
        Err(e) => return err(ERR_BAD_OP, format!("action {name} with data {data}: {e}")),
    };
    // GPUI's `Action::name` requires `&'static str`; action names are interned once per unique pair.
    let leaked: &'static str = Box::leak(name.to_string().into_boxed_str());
    Ok(ACTIONS.with(|t| {
        let mut t = t.borrow_mut();
        let is_builtin = builtin.is_some();
        t.push(Entry { name: leaked, data: data.to_string(), builtin });
        t.len() as u32 | if is_builtin { BUILTIN } else { 0 }
    }))
}

/// Builds an element-level listener for a single `GkAction` ID, propagating non-matching IDs
/// because all host actions share the Rust type `GkAction`.
pub fn action_listener(
    id: u32,
    h: HandlerRef,
    keep: genapi::TapePin,
) -> impl Fn(&GkAction, &mut Window, &mut App) + 'static {
    move |a, window, cx| {
        let _ = &keep;
        if a.id == id {
            genapi::fire::<_, ()>(h, SHAPE_ACTION, &a.id, window, cx);
        } else {
            cx.propagate();
        }
    }
}

/// RAII drop guard stored under `HK_SUB` that unregisters a window or app-quit listener on `C_RELEASE`.
struct Unlisten {
    table: &'static std::thread::LocalKey<RefCell<HashMap<u64, Vec<(u64, HandlerRef)>>>>,
    key: u64,
    handle: u64,
}

impl Drop for Unlisten {
    fn drop(&mut self) {
        let (key, handle) = (self.key, self.handle);
        let _ = self.table.try_with(|t| {
            if let Ok(mut t) = t.try_borrow_mut() {
                if let Some(list) = t.get_mut(&key) {
                    list.retain(|(h, _)| *h != handle);
                }
            }
        });
    }
}

/// RAII drop guard stored under `HK_SUB` that removes an app-wide `GkAction` listener on `C_RELEASE`.
struct UnlistenAction {
    id: u32,
    handle: u64,
}

impl Drop for UnlistenAction {
    fn drop(&mut self) {
        let (id, handle) = (self.id, self.handle);
        let _ = GLOBAL.try_with(|t| {
            if let Ok(mut t) = t.try_borrow_mut() {
                if let Some(list) = t.get_mut(&id) {
                    list.retain(|(h, _)| *h != handle);
                }
            }
        });
    }
}

fn kept(r: &mut R) -> HandlerRef {
    let handler = r.u32();
    let generation = r.u32();
    HandlerRef { view: 0, generation, handler }
}

fn fire_kept<P: genapi::Wire + ?Sized>(h: HandlerRef, shape: u16, payload: &P, window: Option<AnyWindowHandle>, cx: &mut App) {
    let done = match window {
        Some(any) => any
            .update(cx, |_, window, cx| {
                genapi::fire::<_, ()>(h, shape, payload, window, cx);
            })
            .is_ok(),
        None => false,
    };
    if !done {
        genapi::fire_app::<_, ()>(h, shape, payload, cx);
    }
}

/// Registers GPUI window-closed and app-quit lifecycle observers once at startup.
pub fn install(cx: &mut App) {
    cx.on_window_closed(|cx, id| {
        rt::guard("closing a window", || window_closed(id, cx));
    })
    .detach();
    cx.on_app_quit(|cx| {
        rt::guard("quitting", || app_quit(cx));
        async {}
    })
    .detach();
}

fn app_quit(cx: &mut App) {
    let hooks = ON_QUIT.with(|t| t.borrow_mut().remove(&0)).unwrap_or_default();
    for (_, h) in hooks {
        genapi::fire_app::<_, ()>(h, SHAPE_UNIT, &(), cx);
    }
}

/// Invokes registered close-veto handlers for `wh`; returns `false` if any handler vetoes closing.
pub fn ask_should_close(wh: u64, window: &mut Window, cx: &mut App) -> bool {
    let list = SHOULD_CLOSE.with(|t| t.borrow().get(&wh).cloned().unwrap_or_default());
    for (_, h) in list {
        let ret = genapi::fire_raw(h, SHAPE_UNIT_BOOL, &[], window, cx);
        if ret.first() == Some(&0) {
            return false;
        }
    }
    true
}

/// Fires all `C_ON_WINDOW_CLOSED` listeners for `id`, releases its `HK_WINDOW` handle, and quits
/// the GPUI app if `QUIT_ON_LAST` is enabled and no windows remain open.
fn window_closed(id: gpui_kit::WindowId, cx: &mut App) {
    let wh = rt::with(|s| {
        s.windows.iter().copied().find(|w| s.window(*w).ok().is_some_and(|any| any.window_id() == id))
    });
    if let Some(wh) = wh {
        SHOULD_CLOSE.with(|t| t.borrow_mut().remove(&wh));
        let listeners = CLOSED.with(|t| t.borrow_mut().remove(&wh)).unwrap_or_default();
        for (_, h) in listeners {
            genapi::fire_app::<_, ()>(h, SHAPE_WINDOW, &wh, cx);
        }
        let item = rt::with(|s| s.release(wh)).ok();
        drop(item);
    }
    if QUIT_ON_LAST.with(|q| q.get()) && cx.windows().is_empty() {
        cx.quit();
    }
}

fn window_of(c: &mut Call, wh: u64) -> GkResult<AnyWindowHandle> {
    if wh == 0 {
        return match c.window_handle() {
            Some(w) => Ok(w),
            None => err(ERR_NO_CONTEXT, "this call needs a window: give one, or make it in a handler or init"),
        };
    }
    rt::with(|s| s.window(wh))
}

fn on_window<T>(c: &mut Call, wh: u64, f: impl FnOnce(&mut Window, &mut App) -> T) -> GkResult<T> {
    let any = window_of(c, wh)?;
    let window = c.window_opt();
    rt::with_window(any, window, c.cx, f)
}

/// Dispatches app-family wire calls (actions, keybindings, timers, windows, clipboard) and dock calls.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    if let Some(done) = crate::dock::call(op, r, c) {
        return Some(done);
    }
    Some(match op {
        C_ACTION_NEW => (|| {
            let name = r.str().to_string();
            let data = r.str().to_string();
            let id = action_new(&name, &data, c.cx)?;
            c.out.put_u32(id);
            Ok(())
        })(),
        C_BIND_KEYS => (|| {
            let n = r.u32();
            let mut bindings = Vec::with_capacity(n as usize);
            for _ in 0..n {
                let keys = r.str().to_string();
                let action = boxed(r.u32())?;
                let context = r.str();
                let predicate = if context.is_empty() {
                    None
                } else {
                    match KeyBindingContextPredicate::parse(context) {
                        Ok(p) => Some(Rc::new(p)),
                        Err(e) => return err(ERR_BAD_OP, format!("key binding {keys:?}: context {context:?}: {e}")),
                    }
                };
                match KeyBinding::load(&keys, action, predicate, false, None, c.cx) {
                    Ok(b) => bindings.push(b),
                    Err(e) => return err(ERR_BAD_OP, format!("key binding {keys:?}: {e}")),
                }
            }
            c.cx.bind_keys(bindings);
            Ok(())
        })(),
        C_CLEAR_KEY_BINDINGS => {
            c.cx.clear_key_bindings();
            Ok(())
        }
        C_ON_ACTION => (|| {
            let id = r.u32();
            boxed(id)?;
            if id & BUILTIN != 0 {
                return err(
                    ERR_UNSUPPORTED,
                    format!(
                        "{} is a built-in action: listen to it on an element (on_boxed_action); app-wide listeners take the app's own actions",
                        name_of(id)
                    ),
                );
            }
            let h = kept(r);
            if !GLOBAL_INSTALLED.with(|g| g.replace(true)) {
                c.cx.on_action::<GkAction>(|a, cx| {
                    let list = GLOBAL.with(|t| t.borrow().get(&a.id).cloned().unwrap_or_default());
                    for (_, h) in list {
                        // Queue delivery so listeners run outside GPUI's action-dispatch borrow.
                        genapi::fire_queued(h, SHAPE_ACTION, &a.id, cx);
                    }
                });
            }
            let handle = rt::with(|s| s.reserve(HK_SUB));
            GLOBAL.with(|t| t.borrow_mut().entry(id).or_default().push((handle, h)));
            rt::with(|s| s.put(handle, Item::Guard(Box::new(UnlistenAction { id, handle }))));
            c.out.put_u64(handle);
            Ok(())
        })(),
        C_DISPATCH_ACTION => (|| {
            let action = boxed(r.u32())?;
            match c.window_handle() {
                Some(_) => on_window(c, 0, move |window, cx| window.dispatch_action(action, cx)),
                None => {
                    c.cx.dispatch_action(&*action);
                    Ok(())
                }
            }
        })(),
        C_ACTION_AVAILABLE => (|| {
            let action = boxed(r.u32())?;
            let yes = match c.window_opt() {
                Some(window) => window.is_action_available(&*action, c.cx),
                None => c.cx.is_action_available(&*action),
            };
            c.out.put_u8(yes as u8);
            Ok(())
        })(),
        C_ACTION_KEYSTROKES => (|| {
            let action = boxed(r.u32())?;
            let context = r.str();
            let os = c.cx.platform_info().os;
            let text = |b: &KeyBinding| {
                b.keystrokes().iter().map(|k| k.display(os).to_string()).collect::<Vec<_>>().join(" ")
            };
            let found = if context.is_empty() {
                c.cx.key_bindings().borrow().bindings_for_action(&*action).last().map(text)
            } else {
                let ctx = match KeyContext::parse(context) {
                    Ok(k) => k,
                    Err(e) => return err(ERR_BAD_OP, format!("key context {context:?}: {e}")),
                };
                match c.window_opt() {
                    Some(window) => window.highest_precedence_binding_for_action_in_context(&*action, ctx).as_ref().map(text),
                    None => return err(ERR_NO_CONTEXT, "keystrokes in a key context need a window: ask in a handler"),
                }
            };
            match found {
                Some(t) => {
                    c.out.put_u8(1);
                    c.out.put_str(&t);
                }
                None => c.out.put_u8(0),
            }
            Ok(())
        })(),
        C_TIMER_NEW => (|| {
            let ms = r.u32();
            let repeat = r.u8() != 0;
            let h = kept(r);
            if repeat && ms == 0 {
                return err(ERR_BAD_OP, "a repeating timer needs a period of at least 1 ms");
            }
            let window = c.window_handle();
            let th = rt::with(|s| s.reserve(HK_TASK));
            let d = Duration::from_millis(ms as u64);
            let task = c.cx.spawn(async move |cx| {
                loop {
                    cx.background_executor().timer(d).await;
                    cx.update(|cx| {
                        rt::guard("firing a timer", || fire_kept(h, SHAPE_UNIT, &(), window, cx));
                    });
                    if !repeat {
                        break;
                    }
                }
                if !repeat {
                    // Self-release one-shot timers on a deferred tick after the task finishes.
                    cx.update(|cx| {
                        cx.defer(move |_| {
                            let item = rt::with(|s| s.release(th)).ok();
                            drop(item);
                        });
                    });
                }
            });
            rt::with(|s| s.put(th, Item::Task(task)));
            c.out.put_u64(th);
            Ok(())
        })(),
        C_WINDOW_OPEN => (|| {
            let (w, h) = (r.f32(), r.f32());
            let root = r.u64();
            let title = r.str().to_string();
            let kept_title = title.clone();
            let flags = r.u8();
            let (min_w, min_h) = (r.f32(), r.f32());
            let kind = r.u8();
            let has_pos = r.u8() != 0;
            let (x, y) = (r.f32(), r.f32());

            let app_id = r.str().to_string();
            let n = r.u32() as usize;
            let icon = if n == 0 {
                None
            } else {
                match image::load_from_memory_with_format(r.bytes(n), image::ImageFormat::Png) {
                    Ok(img) => Some(std::sync::Arc::new(img.into_rgba8())),
                    Err(e) => return err(ERR_BAD_OP, format!("the window's icon is not a PNG the library can read ({n} bytes): {e}")),
                }
            };
            let mut o = gk_meter::window_options(w, h, c.cx);
            if !app_id.is_empty() {
                o.app_id = Some(app_id);
            }
            if icon.is_some() {
                o.icon = icon;
            }
            // Bit 5 selects gpui-kit's custom TitleBar chrome options.
            if flags & 32 != 0 {
                let k = gpui_kit::component::TitleBar::window_options();
                o.titlebar = k.titlebar;
                o.window_decorations = k.window_decorations;
                o.app_owns_titlebar_drag = k.app_owns_titlebar_drag;
            }
            if !title.is_empty() {
                let mut t = o.titlebar.take().unwrap_or_default();
                t.title = Some(title.into());
                o.titlebar = Some(t);
            }
            o.is_resizable = flags & 1 == 0;
            o.is_minimizable = flags & 2 == 0;
            o.is_movable = flags & 4 == 0;
            if flags & 8 != 0 {
                o.focus = false;
            }
            if flags & 16 != 0 {
                o.show = false;
            }
            if min_w > 0.0 || min_h > 0.0 {
                o.window_min_size = Some(size(px(min_w), px(min_h)));
            }
            o.kind = match kind {
                0 => WindowKind::Normal,
                1 => WindowKind::PopUp,
                2 => WindowKind::Floating,
                k => return err(ERR_BAD_ENUM, format!("window kind {k}: 0 normal, 1 popup, 2 floating")),
            };
            if has_pos {
                if let Some(WindowBounds::Windowed(b)) = o.window_bounds {
                    o.window_bounds = Some(WindowBounds::Windowed(Bounds { origin: point(px(x), px(y)), size: b.size }));
                }
            }
            let wh = rt::open_window_with(o, root, c.cx)?;
            if let Ok(any) = rt::with(|s| s.window(wh)) {
                TITLES.with(|t| t.borrow_mut().insert(any.window_id(), kept_title));
            }
            c.out.put_u64(wh);
            Ok(())
        })(),
        C_WINDOW_CLOSE => (|| {
            let wh = r.u64();
            on_window(c, wh, |window, _| window.remove_window())
        })(),
        C_WINDOW_SET_TITLE => (|| {
            let wh = r.u64();
            let title = r.str().to_string();
            on_window(c, wh, move |window, _| {
                window.set_window_title(&title);
                TITLES.with(|t| t.borrow_mut().insert(window.window_handle().window_id(), title));
            })
        })(),
        C_WINDOW_ACTIVATE => (|| {
            let wh = r.u64();
            on_window(c, wh, |window, _| window.activate_window())
        })(),
        C_ON_WINDOW_CLOSED => (|| {
            let wh = r.u64();
            rt::with(|s| s.window(wh))?;
            let h = kept(r);
            let handle = rt::with(|s| s.reserve(HK_SUB));
            CLOSED.with(|t| t.borrow_mut().entry(wh).or_default().push((handle, h)));
            rt::with(|s| s.put(handle, Item::Guard(Box::new(Unlisten { table: &CLOSED, key: wh, handle }))));
            c.out.put_u64(handle);
            Ok(())
        })(),
        C_ON_WINDOW_SHOULD_CLOSE => (|| {
            let mut wh = r.u64();
            if wh == 0 {
                let Some(any) = c.window_handle() else {
                    return err(ERR_NO_CONTEXT, "this call needs a window: give one, or make it in a handler or init");
                };
                let id = any.window_id();
                match rt::with(|s| s.windows.iter().copied().find(|w| s.window(*w).ok().is_some_and(|x| x.window_id() == id))) {
                    Some(found) => wh = found,
                    None => return err(ERR_NO_CONTEXT, "the turn's window is not one the app opened"),
                }
            }
            rt::with(|s| s.window(wh))?;
            let h = kept(r);
            let handle = rt::with(|s| s.reserve(HK_SUB));
            SHOULD_CLOSE.with(|t| t.borrow_mut().entry(wh).or_default().push((handle, h)));
            rt::with(|s| s.put(handle, Item::Guard(Box::new(Unlisten { table: &SHOULD_CLOSE, key: wh, handle }))));
            if INSTALLED_CLOSE.with(|i| i.borrow_mut().insert(wh)) {
                on_window(c, wh, move |window, cx| {
                    window.on_window_should_close(cx, move |window, cx| {
                        rt::guard("asking whether a window may close", || ask_should_close(wh, window, cx)).unwrap_or(true)
                    })
                })?;
            }
            c.out.put_u64(handle);
            Ok(())
        })(),
        C_ON_APP_QUIT => {
            let h = kept(r);
            let handle = rt::with(|s| s.reserve(HK_SUB));
            ON_QUIT.with(|t| t.borrow_mut().entry(0).or_default().push((handle, h)));
            rt::with(|s| s.put(handle, Item::Guard(Box::new(Unlisten { table: &ON_QUIT, key: 0, handle }))));
            c.out.put_u64(handle);
            Ok(())
        }
        C_REDUCE_MOTION => {
            c.out.put_u8(c.cx.reduce_motion() as u8);
            Ok(())
        }
        C_SET_REDUCE_MOTION => {
            let on = r.u8() != 0;
            c.cx.set_reduce_motion(on);
            Ok(())
        }
        C_WINDOW_INFO => (|| {
            let wh = r.u64();
            let (title, b, active) = on_window(c, wh, |window, _| {
                let mut title = window.window_title();
                if title.is_empty() {
                    let id = window.window_handle().window_id();
                    title = TITLES.with(|t| t.borrow().get(&id).cloned()).unwrap_or_default();
                }
                (title, window.bounds(), window.is_window_active())
            })?;
            c.out.put_str(&title);
            c.out.put_f32(f32::from(b.origin.x));
            c.out.put_f32(f32::from(b.origin.y));
            c.out.put_f32(f32::from(b.size.width));
            c.out.put_f32(f32::from(b.size.height));
            c.out.put_u8(active as u8);
            Ok(())
        })(),
        C_WINDOWS => {
            let list = rt::with(|s| s.windows.clone());
            c.out.put_u32(list.len() as u32);
            for w in list {
                c.out.put_u64(w);
            }
            Ok(())
        }
        C_QUIT_ON_LAST_WINDOW => {
            QUIT_ON_LAST.with(|q| q.set(r.u8() != 0));
            Ok(())
        }
        C_STOP_PROPAGATION => {
            c.cx.stop_propagation();
            Ok(())
        }
        C_CLIPBOARD_WRITE => {
            let text = r.str().to_string();
            c.cx.write_to_clipboard(ClipboardItem::new_string(text));
            Ok(())
        }
        C_CLIPBOARD_READ => {
            match c.cx.read_from_clipboard().and_then(|item| item.text()) {
                Some(t) => {
                    c.out.put_u8(1);
                    c.out.put_str(&t);
                }
                None => c.out.put_u8(0),
            }
            Ok(())
        }
        _ => return None,
    })
}
