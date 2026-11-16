//! Observations (docs/PROTOCOLS.md 11.14): closures upstream calls while it lays out or paints
//! and that get no window, app or context (`Popover::on_position(Fn(ResolvedPosition,
//! Bounds<Pixels>))`, `Popup::on_position`, `Positioner::on_position`). App code may not run in
//! the middle of a frame (DESIGN.md section 0, decision 3), so the generated closure only records
//! its values: `observe` encodes them as the handler's EVENT request and, when they differ from
//! the last values that handler was given, keeps the request and schedules one main-thread task
//! that delivers every kept request after the frame (`rt::queue_event`, with the handler's view's
//! window in scope). A handler marks its view for redraw; the view is described again and lays
//! out the same values again, which are not delivered again, so an observation does not keep the
//! app drawing. The comparison is per view and handler id, not per tape generation (a new
//! description gives the same handler a new generation).
//!
//! No `&mut App` is in scope where upstream calls such a closure, so the task is spawned through
//! the app's `AsyncApp`, which `install` keeps when the app starts.

use crate::genapi::{HandlerRef, Put, Wire};
use crate::rt;
use gpui_kit::{App, AsyncApp};
use std::cell::{Cell, RefCell};
use std::collections::HashMap;

thread_local! {
    static ASYNC: RefCell<Option<AsyncApp>> = const { RefCell::new(None) };
    /// The last payload delivered (or kept for delivery) per (view, handler id).
    static LAST: RefCell<HashMap<(u64, u32), Vec<u8>>> = RefCell::new(HashMap::new());
    /// Kept requests: (view, the encoded EVENT request).
    static PENDING: RefCell<Vec<(u64, Vec<u8>)>> = const { RefCell::new(Vec::new()) };
    static SCHEDULED: Cell<bool> = const { Cell::new(false) };
}

/// Keeps the app's async context, for observations made where no context is in scope. Called
/// once when the app starts (lib.rs `start`).
pub fn install(cx: &App) {
    ASYNC.with(|a| *a.borrow_mut() = Some(cx.to_async()));
}

/// The body of a generated observation: its values as the handler's request, delivered after
/// the frame when they changed.
pub fn observe<P: Wire + ?Sized>(h: HandlerRef, shape: u16, payload: &P) {
    let mut bytes = Vec::with_capacity(32);
    payload.put(&mut bytes);
    let fresh = LAST.with(|l| {
        let mut l = l.borrow_mut();
        let key = (h.view, h.handler);
        if l.get(&key) == Some(&bytes) {
            return false;
        }
        l.insert(key, bytes.clone());
        true
    });
    if !fresh {
        return;
    }
    let mut req = Vec::with_capacity(20 + bytes.len());
    req.put_u64(h.view);
    req.put_u32(h.generation);
    req.put_u32(h.handler);
    req.put_u16(shape);
    req.extend_from_slice(&bytes);
    PENDING.with(|p| p.borrow_mut().push((h.view, req)));
    later();
}

/// Schedules one task that delivers the kept requests, if none is scheduled yet.
fn later() {
    if SCHEDULED.with(|s| s.replace(true)) {
        return;
    }
    let Some(a) = ASYNC.with(|a| a.borrow().clone()) else {
        // Before the app started nothing lays out; keep the requests for the first task.
        SCHEDULED.with(|s| s.set(false));
        return;
    };
    a.spawn(async move |cx: &mut AsyncApp| {
        let _ = cx.update(|cx| deliver(cx));
    })
    .detach();
}

/// Hands every kept request to the runtime's queue of events raised outside an update: each is
/// delivered as an EVENT turn with its view's window, after the current update ends.
fn deliver(cx: &mut App) {
    SCHEDULED.with(|s| s.set(false));
    let pending = PENDING.with(|p| std::mem::take(&mut *p.borrow_mut()));
    for (view, req) in pending {
        rt::queue_event(req, view, cx);
    }
    // The values of views that are gone are forgotten (a view handle is never reused).
    LAST.with(|l| {
        let mut l = l.borrow_mut();
        if l.len() > 256 {
            l.retain(|(view, _), _| *view == 0 || rt::with(|s| s.view(*view).is_ok()));
        }
    });
}

/// A view closure's Tooltip (docs/PROTOCOLS.md 11.17): the kit's own Tooltip value, a node the
/// app placed in its view's tape, built each time upstream asks for the tooltip (when the pointer
/// rests on the element), as a Rust app builds `Tooltip::new(..).build(window, cx)` in its closure.
/// The node is kept with the view's tape (`Frag`), so its text, key binding, action and element
/// fragment are decoded with the window and the app then; a node that is not a Tooltip (only a
/// hand-written tape can make one) shows an empty view.
pub fn tooltip_view(
    fr: &crate::genapi::Frag,
    window: &mut gpui_kit::Window,
    cx: &mut App,
    build: fn(&mut crate::genapi::Dx, u32) -> Option<gpui_kit::component::tooltip::Tooltip>,
) -> gpui_kit::AnyView {
    match fr.with(window, cx, |dx, n| build(dx, n)) {
        Some(t) => t.build(window, cx),
        None => gpui_kit::AppContext::new(cx, |_| gpui_kit::EmptyView).into(),
    }
}
