//! Deferred post-layout observation delivery and tooltip view construction.
//!
//! # Why Post-Layout Events Must Be Deferred via `AsyncApp`
//!
//! GPUI layout/prepaint hooks (such as `on_children_prepainted`, `Positioner::on_position`, and
//! `Popup::on_position`) execute while GPUI is actively walking the element tree and does not
//! expose `&mut App` for issuing synchronous FFI turns or mutating entities.
//!
//! 1. At startup, [`install`] snapshots a thread-local `AsyncApp` handle (`ASYNC`).
//! 2. During prepaint, [`observe`] encodes the observation payload into bytes and compares it
//!    against `LAST[(view, handler)]` so unchanged bounds across steady frames never cross the
//!    wire.
//! 3. Changed observations are appended to `PENDING` and [`later`] spawns at most one async
//!    wakeup (`SCHEDULED`) on `ASYNC` per frame.
//! 4. After prepaint completes, [`deliver`] drains `PENDING` into `rt::queue_event` and prunes
//!    `LAST` entries belonging to released views once the map exceeds 256 keys.

use crate::genapi::{HandlerRef, Put, Wire};
use crate::rt;
use gpui_kit::{App, AsyncApp};
use std::cell::{Cell, RefCell};
use std::collections::HashMap;

thread_local! {
    static ASYNC: RefCell<Option<AsyncApp>> = const { RefCell::new(None) };
    /// Last serialized payload sent per `(view, handler)` pair to avoid redundant notifications.
    static LAST: RefCell<HashMap<(u64, u32), Vec<u8>>> = RefCell::new(HashMap::new());
    static PENDING: RefCell<Vec<(u64, Vec<u8>)>> = const { RefCell::new(Vec::new()) };
    static SCHEDULED: Cell<bool> = const { Cell::new(false) };
}

/// Captures an `AsyncApp` handle at startup so prepaint observers can schedule UI-thread delivery.
pub fn install(cx: &App) {
    ASYNC.with(|a| *a.borrow_mut() = Some(cx.to_async()));
}

/// Records an observation for `h` if its serialized payload changed since the previous frame,
/// and schedules asynchronous delivery through `rt::queue_event`.
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

/// Schedules a single `AsyncApp` wakeup to drain `PENDING` after the current prepaint pass.
fn later() {
    if SCHEDULED.with(|s| s.replace(true)) {
        return;
    }
    let Some(a) = ASYNC.with(|a| a.borrow().clone()) else {
        SCHEDULED.with(|s| s.set(false));
        return;
    };
    a.spawn(async move |cx: &mut AsyncApp| {
        let _ = cx.update(|cx| deliver(cx));
    })
    .detach();
}

/// Flushes queued prepaint observations into `rt::queue_event` and trims stale entries from `LAST`.
fn deliver(cx: &mut App) {
    SCHEDULED.with(|s| s.set(false));
    let pending = PENDING.with(|p| std::mem::take(&mut *p.borrow_mut()));
    for (view, req) in pending {
        rt::queue_event(req, view, cx);
    }
    // Prune entries belonging to released views once the cache grows past 256 entries.
    LAST.with(|l| {
        let mut l = l.borrow_mut();
        if l.len() > 256 {
            l.retain(|(view, _), _| *view == 0 || rt::with(|s| s.view(*view).is_ok()));
        }
    });
}

/// Decodes a tooltip fragment into an `AnyView`, falling back to `EmptyView` on decode failure.
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
