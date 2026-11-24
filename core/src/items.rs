//! Item procedures (docs/PROTOCOLS.md 9.41): the element closures upstream calls while it draws
//! a part of a component for one item (a calendar's day, a time field's segment, a tree's entry,
//! a nav stack's page, a popover's content, a notification's action button), described by an
//! app function between frames.
//!
//! Each call of such a closure in a frame is a row of a row source (`source::ctx_key`, the
//! bookkeeping of the element closures over a context of 9.16): the values upstream hands in are
//! kept for the row's procedure, and a row handed other values is described again before the next
//! frame. Nothing calls the app while upstream draws.
//!
//! What 9.16 could not serve, and this module does:
//! - an element upstream builds and hands in to be decorated (`CalendarItem`, `TimeFieldSegment`,
//!   `NavPage`, the dialog's close `Button`) or a view it hands in (a tooltip's content): the row's
//!   procedure gets it as a node of its own tape (the node's constructor `handed`, decoded here by
//!   taking the value upstream handed for this call), styles it, adds children or places it inside
//!   other elements. So the row is decoded inside upstream's closure (`source::row_tape`, as a
//!   delegate shim decodes its rows), not drawn as the row's view;
//! - a closure that returns one element type (`Fn(..) -> Button`): the row's root, decoded as that
//!   type.
//! Before the row has a tape upstream's own value is drawn (the handed element as it came, or the
//! type's plain value) for one frame.

use crate::genapi::{self, Dx};
use crate::source;
use gpui_kit::{App, Window};
use std::any::Any;
use std::cell::RefCell;

thread_local! {
    /// The values handed to the closure whose row is being decoded, by position.
    static HANDED: RefCell<Vec<Option<Box<dyn Any>>>> = const { RefCell::new(Vec::new()) };
}

/// The constructor `handed` of a node type: the value upstream handed the item's closure at
/// position `i`, for the row's tape to decorate. Once per call: a second use (the procedure
/// placed the same element twice), or a use outside an item procedure's row, stops the program.
pub fn handed<T: 'static>(i: u8) -> T {
    let v = HANDED.with(|h| h.borrow_mut().get_mut(i as usize).and_then(Option::take));
    match v.map(|b| b.downcast::<T>()) {
        Some(Ok(b)) => *b,
        _ => {
            crate::rt::app_stop(format!(
                "the {} an item function is handed was placed twice, or outside its own row: \
                 an item's element can be used once, in the function that received it",
                std::any::type_name::<T>()
            ))
        }
    }
}

/// The element of one call of an item closure whose row is decoded here: `handed` are the values
/// upstream handed (elements and views) by position; `decode` makes the result from the row's
/// root; `fallback` is what this frame draws while the row has no tape (upstream's own value).
#[allow(clippy::too_many_arguments)]
pub fn row<R>(
    src: u64,
    by: Option<u64>,
    print: Vec<u8>,
    write: std::rc::Rc<dyn Fn(&mut Vec<u8>)>,
    handed: Vec<Box<dyn Any>>,
    window: &mut Window,
    cx: &mut App,
    decode: impl FnOnce(&mut Dx, u32) -> Option<R>,
    fallback: impl FnOnce(Vec<Box<dyn Any>>) -> R,
) -> R {
    source::set_decoded(src);
    let k = source::ctx_key(src, by, print, write, window, cx);
    let Some((tape, view)) = source::row_tape(src, k, window, cx) else {
        return fallback(handed);
    };
    let saved = HANDED.with(|h| std::mem::replace(&mut *h.borrow_mut(), handed.into_iter().map(Some).collect()));
    let out = genapi::with_tape(&tape, view, window, cx, decode);
    let left: Vec<Box<dyn Any>> = HANDED.with(|h| std::mem::replace(&mut *h.borrow_mut(), saved)).into_iter().flatten().collect();
    match out {
        Some(v) => v,
        None => fallback(left),
    }
}

/// The handed value at position 0 back as `T`, for a fallback.
pub fn first<T: 'static>(handed: Vec<Box<dyn Any>>) -> Option<T> {
    handed.into_iter().next().and_then(|b| b.downcast::<T>().ok()).map(|b| *b)
}

/// The library's node of this module: `KIT_VIEW` (u8 index), the view handed at that position.
pub fn node(dx: &mut Dx, ty: u16, args: u32, _first: u32) -> Option<gpui_kit::AnyElement> {
    if ty != crate::ops::N_KIT_VIEW {
        return None;
    }
    let i = dx.args(args).u8();
    Some(gpui_kit::IntoElement::into_any_element(handed::<gpui_kit::AnyView>(i)))
}

/// A value an item procedure is handed, written as the record of its reads (gen/gk_items.py's
/// records: their `Wire` impls are on this borrow, never on the type, so a library object the app
/// also holds keeps its own way to travel, as a handle).
pub struct Reads<'a, T: ?Sized>(pub &'a T);
