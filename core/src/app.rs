//! The app-level protocols (docs/PROTOCOLS.md section 11): actions and key bindings, timers,
//! windows and the clipboard. Hand-written runtime of the app family; generated code reaches it
//! only through `genapi` (`action`, `action_listener`, the `Wire` impl of `dyn Action`).
//!
//! - **Actions.** An action is a u32 id. `ACTION_NEW` names one: a name GPUI's action registry
//!   knows (`menu::Confirm`, the kit's input actions) is that built-in action, built with
//!   `App::build_action` (id with bit 31 set); any other name is an app action, `GkAction { id }`,
//!   one Rust type for all of them whose `partial_eq` compares ids. Key bindings, menus,
//!   tooltips and `Kbd` take either kind; listeners on elements filter app actions by id.
//! - **Timers** are foreground tasks: they fire a kept handler as an EVENT turn between frames,
//!   never inside a draw. Releasing the handle drops the task, which cancels it.
//! - **Windows** carry their options; a closed window releases its handle, fires the app's
//!   listeners and, by default, quits the app when it was the last. The app may be asked before
//!   the user closes one (`ON_WINDOW_SHOULD_CLOSE`), and told when the app quits (`ON_APP_QUIT`).

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

/// Bit 31 of an action id: a built-in action (GPUI's or the kit's), not one of the app's.
pub const BUILTIN: u32 = 0x8000_0000;

// --- actions --------------------------------------------------------------------------------

/// Every action of the app: one type, told apart by id.
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
        Err(gpui_kit::private::anyhow::anyhow!("an app action of the binding is made with gk.action, not from a keymap file"))
    }
}

struct Entry {
    name: &'static str,
    data: String,
    /// The built-in action, or None for an app action (a GkAction).
    builtin: Option<Box<dyn Action>>,
}

thread_local! {
    static ACTIONS: RefCell<Vec<Entry>> = const { RefCell::new(Vec::new()) };
    /// App-wide listeners by action id: (listener handle, kept handler).
    static GLOBAL: RefCell<HashMap<u32, Vec<(u64, HandlerRef)>>> = RefCell::new(HashMap::new());
    static GLOBAL_INSTALLED: Cell<bool> = const { Cell::new(false) };
    /// Listeners of a window's closing, by window handle.
    static CLOSED: RefCell<HashMap<u64, Vec<(u64, HandlerRef)>>> = RefCell::new(HashMap::new());
    /// The app's answers to "may the user close this window?", by window handle (PROTOCOLS.md 11.38).
    static SHOULD_CLOSE: RefCell<HashMap<u64, Vec<(u64, HandlerRef)>>> = RefCell::new(HashMap::new());
    /// The app's quit hooks, by listener handle (key 0: one list).
    static ON_QUIT: RefCell<HashMap<u64, Vec<(u64, HandlerRef)>>> = RefCell::new(HashMap::new());
    /// The title each window was given (gk.open_window, gk.set_window_title), for window_info:
    /// the platform does not give it back on Linux.
    static TITLES: RefCell<HashMap<gpui_kit::WindowId, String>> = RefCell::new(HashMap::new());
    static QUIT_ON_LAST: Cell<bool> = const { Cell::new(true) };
    /// The windows whose gpui should-close callback the library installed (one each; it asks the
    /// app's handlers of SHOULD_CLOSE).
    static INSTALLED_CLOSE: RefCell<std::collections::HashSet<u64>> = RefCell::new(std::collections::HashSet::new());
}

fn name_of(id: u32) -> &'static str {
    ACTIONS.with(|a| a.borrow().get(((id & !BUILTIN) as usize).wrapping_sub(1)).map(|e| e.name).unwrap_or("gk::Unknown"))
}

/// The action an id names, as a box GPUI takes.
pub fn boxed(id: u32) -> GkResult<Box<dyn Action>> {
    ACTIONS.with(|a| {
        let a = a.borrow();
        match a.get(((id & !BUILTIN) as usize).wrapping_sub(1)) {
            Some(Entry { builtin: Some(b), .. }) if id & BUILTIN != 0 => Ok(b.boxed_clone()),
            Some(Entry { builtin: None, .. }) if id & BUILTIN == 0 => Ok(Box::new(GkAction { id }) as Box<dyn Action>),
            _ => err(ERR_STALE_HANDLE, format!("action {id:#x} names no action this library made (make actions with gk.action)")),
        }
    })
}

/// The id of an action the library hands to the app (a menu item's, a listener's). A built-in
/// action the app never named gets an id here, under its own name.
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
    let leaked: &'static str = Box::leak(name.to_string().into_boxed_str());
    Ok(ACTIONS.with(|t| {
        let mut t = t.borrow_mut();
        let is_builtin = builtin.is_some();
        t.push(Entry { name: leaked, data: data.to_string(), builtin });
        t.len() as u32 | if is_builtin { BUILTIN } else { 0 }
    }))
}

/// The listener a generated `on_action` / `capture_action` hands GPUI: it fires the handler for
/// its own action and lets every other app action go on to the next listener.
pub fn action_listener(
    id: u32,
    h: HandlerRef,
) -> impl Fn(&GkAction, &mut Window, &mut App) + 'static {
    move |a, window, cx| {
        if a.id == id {
            genapi::fire::<_, ()>(h, SHAPE_ACTION, &a.id, window, cx);
        } else {
            cx.propagate();
        }
    }
}

/// Removes a listener from its table when its handle is released.
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

/// Fires a kept handler with `window` in scope when it is free, else with the app alone.
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

// --- windows ----------------------------------------------------------------------------------

/// Called once when the app starts: what happens when a window closes, and when the app quits.
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

/// The app's quit hooks, each once, with no window (gpui calls them as the app shuts down, and
/// waits a short time for what they start; a hook's own writes are applied before it returns).
fn app_quit(cx: &mut App) {
    let hooks = ON_QUIT.with(|t| t.borrow_mut().remove(&0)).unwrap_or_default();
    for (_, h) in hooks {
        genapi::fire_app::<_, ()>(h, SHAPE_UNIT, &(), cx);
    }
}

/// Whether the user may close the window `wh`: every handler the app gave asked in turn, with the
/// window in scope; the first that returns false keeps it open. A handler that is gone (its
/// listener released, its view's table replaced) or that returned nothing does not hold the window.
/// The platform asks through gpui's `on_window_should_close`; the tests ask through
/// `PLATFORM_TEST` 6, the same function.
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

// --- the call ops -------------------------------------------------------------------------------

pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    // Dock panels (docs/PROTOCOLS.md 11.7, core/src/dock.rs).
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
                        // Queued, not fired here: GPUI calls an app-wide listener inside the window's
                        // dispatch, where the window is not available, and the handler wants one (to open
                        // a dialog, focus an input). Delivered when the dispatch has ended, before the
                        // next draw, with the window of the view that registered it (PROTOCOLS.md 11.33).
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
                    // A one-shot timer forgets itself when it has fired (the binding forgets its
                    // handler then too); dropped outside this task.
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
            // PROTOCOLS.md 11.38: the app id the desktop matches the window to its
            // entry by (Wayland's app_id, X11's WM_CLASS), and the window's icon as a PNG.
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
            if flags & 32 != 0 {
                // the kit's own title bar (TitleBar::window_options, PROTOCOLS.md 13.21): its
                // titlebar and window decorations over the meter's bounds and display
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
                // X11 and Wayland give no title back (gpui's window_title is empty there): the
                // title the app gave when it opened the window or set it last.
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
            // Inside a mouse or key listener: GPUI calls no listener of an element further out.
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
