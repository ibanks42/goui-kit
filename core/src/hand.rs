//! Hand-written node decoder hub and row-store call handlers.
//!
//! Dispatches built-in node opcodes (text, uniform lists, source lists, virtual lists) and
//! delegates specialized families (input, rows, overlay, dock, plot, motion, items) to their
//! dedicated modules. Also implements `C_ROWS_NEW`, `C_ROWS_PUSH`, and update-reentrancy test calls.

use crate::generated;
use crate::genapi::{self, *};
use crate::rt::{self, Item, RowStore};
use crate::source;
use gpui_kit::component::input::InputState;
use gpui_kit::{AnyElement, AppContext, Div, IntoElement, ParentElement, SharedString, Styled, div, px, uniform_list};
use std::cell::RefCell;
use std::rc::Rc;

/// Decodes a hand-written node opcode `ty` into a GPUI `AnyElement`, or returns `None`.
pub fn node(dx: &mut Dx, ty: u16, args: u32, first: u32) -> Option<AnyElement> {
    match ty {
        N_UNIFORM_LIST => return Some(decode_uniform_list(dx, args, first)),
        N_SOURCE_LIST => return Some(decode_source_list(dx, args, first)),
        N_TEXT => return Some(SharedString::from(dx.args(args).str()).into_any_element()),
        _ => {}
    }

    if let Some(el) = crate::shim_input::node(dx, ty, args, first) {
        return Some(el);
    }
    if let Some(el) = crate::shim_rows::node(dx, ty, args, first) {
        return Some(el);
    }
    if let Some(el) = crate::overlay::node(dx, ty, args, first) {
        return Some(el);
    }
    if let Some(el) = crate::dock::node(dx, ty, args, first) {
        return Some(el);
    }
    if let Some(el) = crate::plot::node(dx, ty, args, first) {
        return Some(el);
    }
    if let Some(el) = crate::motion::node(dx, ty, args, first) {
        return Some(el);
    }
    if let Some(el) = crate::items::node(dx, ty, args, first) {
        return Some(el);
    }
    if ty == N_VIRTUAL_LIST {
        return Some(crate::source::virtual_list(dx, args, first));
    }
    Some(match ty {
        N_UNIFORM_LIST => decode_uniform_list(dx, args, first),
        N_SOURCE_LIST => decode_source_list(dx, args, first),
        N_TEXT => SharedString::from(dx.args(args).str()).into_any_element(),
        _ => return None,
    })
}

/// Builds a default 24 px text row for `N_UNIFORM_LIST` and [`crate::tape::FastTree`].
pub fn list_row(text: SharedString) -> Div {
    div().h(px(24.)).px_2().child(text)
}

/// Decodes `N_UNIFORM_LIST`: `[id: str][rows_handle: u64]` plus style ops, rendering each
/// `RowStore` string through [`list_row`].
fn decode_uniform_list(dx: &mut Dx, args: u32, first: u32) -> AnyElement {
    let mut a = dx.args(args);
    let id = SharedString::from(a.str());
    let store = match rt::with(|s| s.rows(a.u64())) {
        Ok(store) => store,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "Rows"),
    };

    let rows = store.read(dx.cx).rows.clone();
    let count = rows.borrow().len();
    let mut el = uniform_list(id, count, move |range, _, _| {
        let rows = rows.borrow();
        range.map(|i| list_row(rows[i].clone())).collect::<Vec<_>>()
    });
    let mut at = first;
    while let Some((op, mut r)) = dx.op(&mut at) {
        if !generated::style_op(el.style(), op, &mut r, dx) {
            eprintln!("gk: op {op} is not a UniformList op");
        }
    }
    el.into_any_element()
}

/// Decodes `N_SOURCE_LIST`: `[id: str][src_handle: u64][row_height: f32]` plus style ops,
/// watching `src` on the enclosing view so new row tapes refresh the list.
fn decode_source_list(dx: &mut Dx, args: u32, first: u32) -> AnyElement {
    let mut a = dx.args(args);
    let id = SharedString::from(a.str());
    let src = a.u64();
    let height = a.f32();
    let parts = source::count(src, 0).and_then(|n| Ok((n, source::scroll_handle(src)?)));
    let (count, scroll) = match parts {
        Ok(parts) => parts,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "row source"),
    };

    if let Ok(view) = rt::with(|s| s.view(dx.view_handle())) {
        source::watch(src, view.entity_id());
    }
    let mut el = uniform_list(id, count, move |range, window, cx| {
        range
            .map(|ix| match source::row(src, source::key(0, ix as u32), window, cx) {
                source::Row::View(v) => div().h(px(height)).child(v).into_any_element(),
                source::Row::Missing => div().h(px(height)).into_any_element(),
            })
            .collect::<Vec<_>>()
    })
    .track_scroll(&scroll);
    let mut at = first;
    while let Some((op, mut r)) = dx.op(&mut at) {
        if !generated::style_op(el.style(), op, &mut r, dx) {
            eprintln!("gk: op {op} is not a list op");
        }
    }
    el.into_any_element()
}

/// Fast path invoked by the reply processor to append strings directly to a [`RowStore`].
pub(crate) fn rows_push_slice(op: &[u8], cx: &mut gpui_kit::App) -> GkResult<()> {
    let mut r = R::new(op);
    let _ = r.u16();
    let entity = rt::with(|s| s.rows(r.u64()))?;
    let n = r.u32();
    entity.update(cx, |store, cx| {
        let mut rows = store.rows.borrow_mut();
        rows.reserve(n as usize);
        for _ in 0..n {
            rows.push(SharedString::from(r.str()));
        }
        cx.notify();
    });
    Ok(())
}

/// Handles `C_ROWS_NEW`, `C_ROWS_PUSH`, and the entity-update handler test opcodes.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_ROWS_NEW => {
            let entity = c.cx.new(|_| RowStore {
                rows: Rc::new(RefCell::new(Vec::new())),
            });
            let handle = rt::with(|s| s.insert(HK_ROWS, Item::Rows(entity)));
            c.out.put_u64(handle);
            Ok(())
        }
        C_ROWS_PUSH => (|| {
            let entity = rt::with(|s| s.rows(r.u64()))?;
            let n = r.u32();
            entity.update(c.cx, |store, cx| {
                let mut rows = store.rows.borrow_mut();
                rows.reserve(n as usize);
                for _ in 0..n {
                    rows.push(SharedString::from(r.str()));
                }
                cx.notify();
            });
            Ok(())
        })(),
        C_TEST_FIRE_IN_UPDATE => (|| {
            // Fires a handler while an `InputState` entity borrow is active on the stack.
            let entity = genapi::state::<InputState>(r.u64())?;
            let h = HandlerRef {
                view: r.u64(),
                generation: r.u32(),
                handler: r.u32(),
            };
            let window = c.window()?;
            let ret: bool =
                entity.update(c.cx, |_, cx| genapi::fire(h, SHAPE_UNIT_BOOL, &(), window, cx));
            c.out.put_i32(ret as i32);
            Ok(())
        })(),
        C_TEST_FIRE_QUEUED => (|| {
            // Queues an event from inside an `InputState` update to test deferred delivery.
            let entity = genapi::state::<InputState>(r.u64())?;
            let h = HandlerRef {
                view: r.u64(),
                generation: r.u32(),
                handler: r.u32(),
            };
            let row = r.usize();
            entity.update(c.cx, |_, cx| genapi::fire_queued(h, SHAPE_USIZE, &row, cx));
            Ok(())
        })(),
        _ => return None,
    })
}
