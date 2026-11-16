//! The hand-written bridges: what the generator does not emit yet and the examples need.
//! The uniform list keeps its rows in the library (DESIGN.md section 9). It is written
//! exactly as generated code is, using `crate::genapi`, and is asked after the generated
//! module. The test call ops of core/tests are here too. (InputState was a bridge here until
//! 11 Nov 2026; the generator instantiates it now: `[instantiate] states` in overrides.toml.)

use crate::generated;
use crate::genapi::{self, *};
use crate::rt::{self, Item, RowStore};
use crate::source;
use gpui_kit::component::input::InputState;
use gpui_kit::{AnyElement, AppContext, Div, IntoElement, ParentElement, SharedString, Styled, div, px, uniform_list};
use std::cell::RefCell;
use std::rc::Rc;

pub fn node(dx: &mut Dx, ty: u16, args: u32, first: u32) -> Option<AnyElement> {
    // The delegate families' own elements (docs/PROTOCOLS.md section 9).
    if let Some(el) = crate::shim_input::node(dx, ty, args, first) {
        return Some(el);
    }
    if let Some(el) = crate::shim_rows::node(dx, ty, args, first) {
        return Some(el);
    }
    // View-built closures and the dropdown button (docs/PROTOCOLS.md section 11.6).
    if let Some(el) = crate::overlay::node(dx, ty, args, first) {
        return Some(el);
    }
    if let Some(el) = crate::dock::node(dx, ty, args, first) {
        return Some(el);
    }
    // A described plot (docs/PROTOCOLS.md section 12).
    if let Some(el) = crate::plot::node(dx, ty, args, first) {
        return Some(el);
    }
    // An element wrapped by an animation, motion values or a focus trap (PROTOCOLS.md 13.30).
    if let Some(el) = crate::motion::node(dx, ty, args, first) {
        return Some(el);
    }
    // A view handed to an item procedure (PROTOCOLS.md 9.41).
    if let Some(el) = crate::items::node(dx, ty, args, first) {
        return Some(el);
    }
    // A virtual list over a row source (PROTOCOLS.md 13.31).
    if ty == N_VIRTUAL_LIST {
        return Some(crate::source::virtual_list(dx, args, first));
    }
    Some(match ty {
        N_UNIFORM_LIST => decode_uniform_list(dx, args, first),
        N_SOURCE_LIST => decode_source_list(dx, args, first),
        // A string where upstream takes `impl IntoElement` (GroupBox::title("..")).
        N_TEXT => SharedString::from(dx.args(args).str()).into_any_element(),
        _ => return None,
    })
}

/// A list row as the sample draws it. The Rust reference builds the same element.
pub fn list_row(text: SharedString) -> Div {
    div().h(px(24.)).px_2().child(text)
}

fn decode_uniform_list(dx: &mut Dx, args: u32, first: u32) -> AnyElement {
    let mut a = dx.args(args);
    let id = SharedString::from(a.str());
    let store = match rt::with(|s| s.rows(a.u64())) {
        Ok(store) => store,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "Rows"),
    };
    // Read during the view's build, so gpui-fast records that this view depends on the store.
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

/// A list over a row source (the shared core of the delegate protocol, core/src/source.rs):
/// each row is a view of its own, a row without one is drawn as an empty row of the row's
/// height for one frame. Args: str id, u64 source, f32 row height.
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
    // The view that draws the list is notified when the rows it drew as placeholders arrive.
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
            // Stands in for a delegate method: fires the handler synchronously from inside
            // the input entity's own update, where that entity is on lease.
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
            // The same, as a queued event: the handler runs when the update has ended.
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
