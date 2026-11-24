//! Library objects behind handles (docs/PROTOCOLS.md section 13): kit values that are not
//! entities (a dialog's handle, a scroll handle, a display map, a search matcher, the theme)
//! held by the library and named by a handle of kind OBJ. Generated call ops reach them only
//! through the functions here, re-exported by `genapi`.
//!
//! - An object made by a constructor is *kept*: it lives until `gk.release`.
//! - An object a read, a call or an event hands out is *temporary*: it lives until the next
//!   frame begins (the pre-frame hook sweeps it), after the buffered writes of the turn that
//!   got it have landed. `gk.retain` (call op OBJ_RETAIN) keeps it.
//! - A kit global (`impl Global`: the theme, the theme registry) has one handle per type,
//!   interned, that reads and updates the global in place (`cx.global`, `cx.update_global`).
//!
//! An owned object sits in an `Rc<RefCell<T>>`, cloned out of the handle table before any kit
//! code runs, so no kit method runs inside the session borrow. A method that re-enters the
//! binding and touches the same object meets a borrowed cell: GK_ERR_BUSY, never a panic.

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

/// What a handle of kind OBJ names.
pub enum ObjItem {
    /// An `Rc<RefCell<T>>` as `dyn Any`.
    Owned { cell: Rc<dyn Any>, type_name: &'static str },
    /// The kit global of this type.
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
    /// Temporary handles, released when the next frame begins.
    static TEMPS: RefCell<Vec<u64>> = const { RefCell::new(Vec::new()) };
    /// The interned handle of each global type.
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

/// A new kept object (a constructor's result): released with gk.release.
pub fn obj_new<T: 'static>(v: T) -> u64 {
    insert(v)
}

/// A new temporary object (a read's, a call's or an event's result).
pub fn obj_temp<T: 'static>(v: T) -> u64 {
    let h = insert(v);
    TEMPS.with(|t| t.borrow_mut().push(h));
    HAS_TEMPS.store(true, Ordering::Relaxed);
    h
}

/// The handle of the kit global `T`. One per type, never temporary; releasing it is allowed
/// and the next request makes a new one.
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

/// The cell of an owned object (an argument passed by reference).
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

/// A copy of an owned object (an argument passed by value, of a type that is Clone).
pub fn obj_clone<T: Clone + 'static>(h: u64) -> GkResult<T> {
    let c = obj_rc::<T>(h)?;
    let v = c.try_borrow().map_err(|_| busy::<T>())?.clone();
    Ok(v)
}

/// An owned object given away (an argument passed by value, of a type that is not Clone):
/// the handle is released, and using it again stops with a message.
pub fn obj_take<T: 'static>(h: u64) -> GkResult<T> {
    let c = obj_rc::<T>(h)?;
    drop(rt::with(|s| s.release(h))?);
    match Rc::try_unwrap(c) {
        Ok(cell) => Ok(cell.into_inner()),
        Err(_) => err(ERR_BUSY, format!("the {} is in use and cannot be given away", std::any::type_name::<T>())),
    }
}

/// A copy of an object a view's tape names (an element's argument). A released or swept
/// handle there is a mistake in app code and the element cannot be drawn: exit 2.
pub fn obj_shown<T: Clone + 'static>(h: u64) -> T {
    match obj_clone::<T>(h) {
        Ok(v) => v,
        Err(GkErr(_, msg)) => crate::genapi::released_shown(&msg, std::any::type_name::<T>()),
    }
}

/// A read (`&self`) of an owned object.
pub fn obj_read<T: 'static, V>(h: u64, c: &mut crate::genapi::Call, f: impl FnOnce(&T, &App) -> V) -> GkResult<V> {
    match get::<T>(h)? {
        Got::Cell(cell) => {
            let s = cell.try_borrow().map_err(|_| busy::<T>())?;
            Ok(f(&s, &*c.cx))
        }
        Got::Global => unreachable!("a global handle of a type that is not Global"),
    }
}

/// A read of an owned object or of the global of that type.
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

/// An update of an owned object (`&mut self`, or `&self` with the app): a buffered write
/// returns (), a call returns its value.
pub fn obj_update<T: 'static, V>(h: u64, c: &mut crate::genapi::Call, f: impl FnOnce(&mut T, &mut App) -> V) -> GkResult<V> {
    match get::<T>(h)? {
        Got::Cell(cell) => {
            let mut s = cell.try_borrow_mut().map_err(|_| busy::<T>())?;
            Ok(f(&mut s, c.cx))
        }
        Got::Global => unreachable!("a global handle of a type that is not Global"),
    }
}

/// `obj_update` that also accepts the global of that type.
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

/// Runs `f` with a window: the turn's, or the app's first open window (a buffered write
/// applied after its turn). GK_ERR_NO_CONTEXT when no window is open.
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

/// `obj_update` for a method that also takes the window.
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

/// `obj_update_w` that also accepts the global of that type.
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

/// A method that takes the object by value (`self`): a copy when the type is Clone (the
/// handle stays good), else the object itself (the handle is released).
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

/// A result that is a library object by value, handed out as a temporary handle without a
/// copy (`Wire::put` takes `&self`, so the value waits in a cell).
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

/// A builder value made outside a view as a call op's argument (PROTOCOLS.md section 13.6):
/// u32 length, then a tape whose root is the value; decoded by the value's generated decoder
/// with a window (the turn's, or the app's first) and the app in hand.
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

/// Rems (a length in the theme's tokens) as it travels out: an f32, as `Pixels` does.
impl Wire for gpui_kit::Rems {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_f32(self.0)
    }
}

/// Releases the temporary handles handed out before this point. Called when a frame begins
/// (`rt::pre_frame`), after every buffered write of the turns that got them has landed.
pub fn sweep() {
    if !needs_sweep() {
        return;
    }
    let mut temps = if HAS_TEMPS.swap(false, Ordering::Relaxed) {
        TEMPS.with(|t| std::mem::take(&mut *t.borrow_mut()))
    } else {
        Vec::new()
    };
    // Handles an earlier build of a view pinned and no build names now (values.rs, OBJ_PIN).
    temps.extend(crate::values::sweep_pins());
    if temps.is_empty() {
        return;
    }
    let items: Vec<_> = rt::with(|s| temps.iter().filter_map(|h| s.release(*h).ok()).collect());
    drop(items); // outside the session borrow: dropping a kit value may run GPUI code
}

/// Whether temporary handles or retired view pins are waiting for `sweep` at the next frame.
#[inline]
pub fn needs_sweep() -> bool {
    HAS_TEMPS.load(Ordering::Relaxed) || crate::values::has_pending_pins()
}

/// Makes a temporary handle kept (it is no longer swept); false when `h` was not temporary.
/// A view that places one in its elements pins it this way (values.rs, OBJ_PIN).
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

/// The runtime's own call ops of this protocol.
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
            // A table's columns as the kit's own Column values, each a tape of its own.
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
            // then the live kit states, views and focus handles (the Go runtime's lifetime tests)
            for kind in [HK_STATE, HK_VIEW, HK_FOCUS] {
                c.out.put_u32(rt::with(|s| s.count(kind)) as u32);
            }
            Some(Ok(()))
        }
        _ => None,
    }
}
