//! Handle-backed Rust library objects (`HK_OBJ`), temporary-handle sweeping, and global singletons.
//!
//! # Object Handle Lifetimes and Retirement
//!
//! Non-entity Rust values (scroll handles, keyframes, image caches, diff files, theme configs,
//! and GPUI `Global` singletons) cannot cross the wire by value and are stored under `HK_OBJ`
//! generational handles in `Session`:
//!
//! - **Permanent handles ([`obj_new`])**: explicitly constructed by the host; live until released
//!   via `C_RELEASE` (or moved out via [`obj_take`]).
//! - **Temporary handles ([`obj_temp`], [`Owned`])**: returned by getters, method calls, or event
//!   payloads. Each is appended to thread-local `TEMPS` (setting `HAS_TEMPS`), and `rt::pre_frame`
//!   invokes [`sweep`] at the start of the next frame to release every unretained temporary handle
//!   along with expired view pins from `values::sweep_pins`. Calling `C_OBJ_RETAIN` (or pinning
//!   via `values::pin` / [`untemp`]) removes a handle from `TEMPS` so it survives across frames.
//!   When a host mistakenly uses a swept temporary, [`stale`] augments `ERR_STALE_HANDLE` with a
//!   hint pointing to `gk.Retain`.
//! - **Global singletons ([`obj_global`])**: maps each `Global` type's `TypeId` to one cached
//!   `ObjItem::Global` handle (`GLOBALS`). `obj_read_global` / `obj_update_global` /
//!   `obj_update_w_global` route global handles to `cx.global::<T>()` / `cx.update_global::<T>()`,
//!   while `obj_rc` rejects them so globals cannot be moved or taken by value.
//! - **Borrow & move safety**: owned objects live in `Rc<RefCell<T>>`. `try_borrow` /
//!   `try_borrow_mut` convert re-entrant conflicts into `ERR_BUSY` rather than panicking, and
//!   [`obj_take`] releases the handle before `Rc::try_unwrap` so an in-use object fails with
//!   `ERR_BUSY` instead of corrupting an active borrow.

use crate::genapi::Wire;
use crate::ops::*;
use crate::rt::{self, GkErr, GkResult, Item, err};
use crate::wire::{Put, R};
use gpui_kit::{App, BorrowAppContext, Global, Window};
use std::any::{Any, TypeId};
use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::rc::Rc;
use std::sync::atomic::{AtomicBool, Ordering};

static HAS_TEMPS: AtomicBool = AtomicBool::new(false);

/// Stored payload for an `HK_OBJ` handle: either an owned `RefCell<T>` or a GPUI global marker.
pub enum ObjItem {
    Owned { cell: Rc<dyn Any>, type_name: &'static str },
    Global { type_id: TypeId, type_name: &'static str },
}

impl ObjItem {
    fn type_name(&self) -> &'static str {
        match self {
            ObjItem::Owned { type_name, .. } | ObjItem::Global { type_name, .. } => type_name,
        }
    }
}

thread_local! {
    /// Temporary `HK_OBJ` handles scheduled for release at the next [`sweep`].
    static TEMPS: RefCell<Vec<u64>> = const { RefCell::new(Vec::new()) };
    /// Cached singleton handles for GPUI `Global` types, keyed by `TypeId`.
    static GLOBALS: RefCell<HashMap<TypeId, u64>> = RefCell::new(HashMap::new());
}

const HINT: &str = "a library object a read, a call or an event hands out is temporary: it lives \
                    until the next frame begins unless gk.Retain keeps it";

fn stale(e: GkErr) -> GkErr {
    if e.0 == ERR_STALE_HANDLE {
        GkErr(e.0, format!("{} ({HINT})", e.1))
    } else {
        e
    }
}

fn insert<T: 'static>(v: T) -> u64 {
    let item = ObjItem::Owned {
        cell: Rc::new(RefCell::new(v)),
        type_name: std::any::type_name::<T>(),
    };
    rt::with(|s| s.insert(HK_OBJ, Item::Obj(item)))
}

/// Inserts a permanent (retained) library object and returns its `HK_OBJ` handle.
pub fn obj_new<T: 'static>(v: T) -> u64 {
    insert(v)
}

/// Inserts a temporary library object that [`sweep`] will release before the next frame unless
/// the host calls `C_OBJ_RETAIN`.
pub fn obj_temp<T: 'static>(v: T) -> u64 {
    let h = insert(v);
    TEMPS.with(|t| t.borrow_mut().push(h));
    HAS_TEMPS.store(true, Ordering::Relaxed);
    h
}

/// Returns the cached `HK_OBJ` handle representing GPUI global `T`, creating one if needed.
pub fn obj_global<T: Global>() -> u64 {
    let id = TypeId::of::<T>();
    if let Some(h) = GLOBALS.with(|g| g.borrow().get(&id).copied()) {
        if rt::with(|s| s.obj(h).is_ok()) {
            return h;
        }
    }
    let item = ObjItem::Global {
        type_id: id,
        type_name: std::any::type_name::<T>(),
    };
    let h = rt::with(|s| s.insert(HK_OBJ, Item::Obj(item)));
    GLOBALS.with(|g| g.borrow_mut().insert(id, h));
    h
}

enum Got<T> {
    Cell(Rc<RefCell<T>>),
    Global,
}

fn get<T: 'static>(h: u64) -> GkResult<Got<T>> {
    rt::with(|s| {
        let item = s.obj(h).map_err(stale)?;
        let want = std::any::type_name::<T>();
        match item {
            ObjItem::Owned { cell, .. } => match cell.clone().downcast::<RefCell<T>>() {
                Ok(c) => Ok(Got::Cell(c)),
                Err(_) => err(ERR_STALE_HANDLE, format!("handle {h:#x} is a {}, not a {want}", item.type_name())),
            },
            ObjItem::Global { type_id, .. } if *type_id == TypeId::of::<T>() => Ok(Got::Global),
            _ => err(ERR_STALE_HANDLE, format!("handle {h:#x} is a {}, not a {want}", item.type_name())),
        }
    })
}

/// Returns the `Rc<RefCell<T>>` backing an owned `HK_OBJ` handle (rejecting global handles).
pub fn obj_rc<T: 'static>(h: u64) -> GkResult<Rc<RefCell<T>>> {
    match get::<T>(h)? {
        Got::Cell(c) => Ok(c),
        Got::Global => err(
            ERR_BAD_OP,
            format!("the global {} cannot be passed as an argument", std::any::type_name::<T>()),
        ),
    }
}

fn busy<T>() -> GkErr {
    GkErr(
        ERR_BUSY,
        format!("the {} is in use by the kit code that called this handler", std::any::type_name::<T>()),
    )
}

/// Clones the value inside owned object handle `h`.
pub fn obj_clone<T: Clone + 'static>(h: u64) -> GkResult<T> {
    let c = obj_rc::<T>(h)?;
    let v = c.try_borrow().map_err(|_| busy::<T>())?.clone();
    Ok(v)
}

/// Releases handle `h` and moves the owned value out if no other `Rc` reference is held.
pub fn obj_take<T: 'static>(h: u64) -> GkResult<T> {
    let c = obj_rc::<T>(h)?;
    drop(rt::with(|s| s.release(h))?);
    match Rc::try_unwrap(c) {
        Ok(cell) => Ok(cell.into_inner()),
        Err(_) => err(ERR_BUSY, format!("the {} is in use and cannot be given away", std::any::type_name::<T>())),
    }
}

/// Clones an object handle referenced by an element on a tape, stopping the app on stale handles.
pub fn obj_shown<T: Clone + 'static>(h: u64) -> T {
    match obj_clone::<T>(h) {
        Ok(v) => v,
        Err(GkErr(_, msg)) => crate::genapi::released_shown(&msg, std::any::type_name::<T>()),
    }
}

/// Immutably borrows an owned `HK_OBJ` value of type `T`.
pub fn obj_read<T: 'static, V>(h: u64, c: &mut crate::genapi::Call, f: impl FnOnce(&T, &App) -> V) -> GkResult<V> {
    match get::<T>(h)? {
        Got::Cell(cell) => {
            let s = cell.try_borrow().map_err(|_| busy::<T>())?;
            Ok(f(&s, &*c.cx))
        }
        Got::Global => unreachable!("a global handle of a type that is not Global"),
    }
}

/// Immutably borrows either an owned `HK_OBJ` or the GPUI `Global` singleton of type `T`.
pub fn obj_read_global<T: Global, V>(h: u64, c: &mut crate::genapi::Call, f: impl FnOnce(&T, &App) -> V) -> GkResult<V> {
    match get::<T>(h)? {
        Got::Cell(cell) => {
            let s = cell.try_borrow().map_err(|_| busy::<T>())?;
            Ok(f(&s, &*c.cx))
        }
        Got::Global => {
            let cx: &App = &*c.cx;
            if !cx.has_global::<T>() {
                return err(ERR_NO_CONTEXT, format!("the global {} is not set", std::any::type_name::<T>()));
            }
            Ok(f(cx.global::<T>(), cx))
        }
    }
}

/// Mutably borrows an owned `HK_OBJ` value of type `T`.
pub fn obj_update<T: 'static, V>(h: u64, c: &mut crate::genapi::Call, f: impl FnOnce(&mut T, &mut App) -> V) -> GkResult<V> {
    match get::<T>(h)? {
        Got::Cell(cell) => {
            let mut s = cell.try_borrow_mut().map_err(|_| busy::<T>())?;
            Ok(f(&mut s, c.cx))
        }
        Got::Global => unreachable!("a global handle of a type that is not Global"),
    }
}

/// Mutably borrows either an owned `HK_OBJ` or the GPUI `Global` singleton of type `T`.
pub fn obj_update_global<T: Global, V>(h: u64, c: &mut crate::genapi::Call, f: impl FnOnce(&mut T, &mut App) -> V) -> GkResult<V> {
    match get::<T>(h)? {
        Got::Cell(cell) => {
            let mut s = cell.try_borrow_mut().map_err(|_| busy::<T>())?;
            Ok(f(&mut s, c.cx))
        }
        Got::Global => {
            if !c.cx.has_global::<T>() {
                return err(ERR_NO_CONTEXT, format!("the global {} is not set", std::any::type_name::<T>()));
            }
            Ok(c.cx.update_global::<T, _>(|g, cx| f(g, cx)))
        }
    }
}

/// Runs `f` with the call's current window, falling back to the first open window when the call
/// arrived without window context.
pub fn with_window<V>(c: &mut crate::genapi::Call, f: impl FnOnce(&mut Window, &mut App) -> V) -> GkResult<V> {
    if let Some(window) = c.window_opt() {
        return Ok(f(window, c.cx));
    }
    let any = rt::with(|s| s.windows.iter().find_map(|w| s.window(*w).ok()));
    match any {
        Some(any) => rt::with_window(any, None, c.cx, f),
        None => err(ERR_NO_CONTEXT, "this call needs a window: open one first"),
    }
}

/// Mutably borrows an owned `HK_OBJ` with window access.
pub fn obj_update_w<T: 'static, V>(
    h: u64,
    c: &mut crate::genapi::Call,
    f: impl FnOnce(&mut T, &mut Window, &mut App) -> V,
) -> GkResult<V> {
    let cell = match get::<T>(h)? {
        Got::Cell(cell) => cell,
        Got::Global => unreachable!("a global handle of a type that is not Global"),
    };
    let mut s = cell.try_borrow_mut().map_err(|_| busy::<T>())?;
    with_window(c, |window, cx| f(&mut s, window, cx))
}

/// Mutably borrows an owned `HK_OBJ` or GPUI `Global` with window access.
pub fn obj_update_w_global<T: Global, V>(
    h: u64,
    c: &mut crate::genapi::Call,
    f: impl FnOnce(&mut T, &mut Window, &mut App) -> V,
) -> GkResult<V> {
    match get::<T>(h)? {
        Got::Cell(cell) => {
            let mut s = cell.try_borrow_mut().map_err(|_| busy::<T>())?;
            with_window(c, |window, cx| f(&mut s, window, cx))
        }
        Got::Global => with_window(c, |window, cx| cx.update_global::<T, _>(|g, cx| f(g, window, cx))),
    }
}

/// Extracts `T` by value from handle `h`, cloning via `clone` when supported and otherwise
/// consuming the handle with [`obj_take`].
pub fn obj_by_value<T: 'static>(h: u64, clone: impl FnOnce(&T) -> Option<T>) -> GkResult<T> {
    let cell = obj_rc::<T>(h)?;
    let copy = {
        let s = cell.try_borrow().map_err(|_| busy::<T>())?;
        clone(&s)
    };
    drop(cell);
    match copy {
        Some(v) => Ok(v),
        None => obj_take::<T>(h),
    }
}

/// Move-once wrapper that encodes an owned `T` onto the wire as a temporary `HK_OBJ` handle.
pub struct Owned<T>(Cell<Option<T>>);

impl<T> Owned<T> {
    pub fn new(v: T) -> Self {
        Owned(Cell::new(Some(v)))
    }
}

impl<T: 'static> Wire for Owned<T> {
    fn put(&self, out: &mut Vec<u8>) {
        match self.0.take() {
            Some(v) => out.put_u64(obj_temp(v)),
            None => out.put_u64(0),
        }
    }
}

/// Decodes a length-prefixed mini-tape argument using `build`.
pub fn value_arg<T>(
    c: &mut crate::genapi::Call,
    r: &mut R,
    build: fn(&mut crate::genapi::Dx, u32) -> Option<T>,
) -> GkResult<T> {
    let n = r.u32() as usize;
    let tape = Rc::new(crate::tape::Tape::parse(r.bytes(n).to_vec(), 0)?);
    let got = with_window(c, |window, cx| crate::genapi::with_tape(&tape, 0, window, cx, |dx, root| build(dx, root)))?;
    match got {
        Some(v) => Ok(v),
        None => err(ERR_BAD_OP, format!("a {} argument could not be built from its value", std::any::type_name::<T>())),
    }
}

impl Wire for gpui_kit::Rems {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_f32(self.0)
    }
}

/// Releases all unretained temporary object handles and expired value pins before a frame begins.
pub fn sweep() {
    if !needs_sweep() {
        return;
    }
    let mut temps = if HAS_TEMPS.swap(false, Ordering::Relaxed) {
        TEMPS.with(|t| std::mem::take(&mut *t.borrow_mut()))
    } else {
        Vec::new()
    };
    temps.extend(crate::values::sweep_pins());
    if temps.is_empty() {
        return;
    }
    let items: Vec<_> = rt::with(|s| temps.iter().filter_map(|h| s.release(*h).ok()).collect());
    drop(items);
}

#[inline]
pub fn needs_sweep() -> bool {
    HAS_TEMPS.load(Ordering::Relaxed) || crate::values::has_pending_pins()
}

/// Removes `h` from the temporary list without requiring a `C_OBJ_RETAIN` call; returns `true`
/// if `h` was pending sweep.
pub fn untemp(h: u64) -> bool {
    if !HAS_TEMPS.load(Ordering::Relaxed) {
        return false;
    }
    TEMPS.with(|t| {
        let mut t = t.borrow_mut();
        let n = t.len();
        t.retain(|x| *x != h);
        if t.is_empty() {
            HAS_TEMPS.store(false, Ordering::Relaxed);
        }
        t.len() != n
    })
}

/// Handles `C_OBJ_RETAIN`, `C_TABLE_COLUMN_VALUES`, and `C_OBJ_TEST_TEMPS`.
pub fn call(op: u16, r: &mut R, c: &mut crate::genapi::Call) -> Option<GkResult<()>> {
    match op {
        C_OBJ_RETAIN => {
            let h = r.u64();
            Some((|| {
                rt::with(|s| s.obj(h).map(|_| ())).map_err(stale)?;
                if HAS_TEMPS.load(Ordering::Relaxed) {
                    TEMPS.with(|t| {
                        let mut t = t.borrow_mut();
                        t.retain(|x| *x != h);
                        if t.is_empty() {
                            HAS_TEMPS.store(false, Ordering::Relaxed);
                        }
                    });
                }
                Ok(())
            })())
        }
        C_TABLE_COLUMN_VALUES => Some((|| {
            let (src, state) = (r.u64(), r.u64());
            let n = r.u32() as usize;
            let mut columns = Vec::with_capacity(n.min(4096));
            for _ in 0..n {
                columns.push(value_arg(c, r, crate::generated::build_component_column)?);
            }
            if state == 0 {
                crate::shim_rows::source_columns(src, columns)
            } else {
                crate::shim_rows::state_columns(state, c, columns)
            }
        })()),
        C_OBJ_TEST_TEMPS => {
            let n = TEMPS.with(|t| t.borrow().len());
            let live = rt::with(|s| s.count(HK_OBJ));
            c.out.put_u32(n as u32);
            c.out.put_u32(live as u32);
            for kind in [HK_STATE, HK_VIEW, HK_FOCUS] {
                c.out.put_u32(rt::with(|s| s.count(kind)) as u32);
            }
            Some(Ok(()))
        }
        _ => None,
    }
}
