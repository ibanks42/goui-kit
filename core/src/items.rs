//! Handed-element slot bridge for upstream item-builder closures.
//!
//! # Single-Use Handed-Element Protocol
//!
//! Several gpui-kit components (such as `Calendar`, `TimeField`, `Popover`, `Combobox`,
//! `HoverCard`, `Command`, and `NavigationStack`) invoke a builder closure passing both a
//! read-only context value and one or more move-only upstream elements or `AnyView`s that the
//! closure must incorporate into its return value. Because host closures run in Go and return a
//! byte tape rather than owning Rust elements directly:
//!
//! 1. [`row`] switches `src` to decoded-row mode (`source::set_decoded`), records the context
//!    fingerprint/writer via `source::ctx_key`, and looks up the row's cached `Rc<Tape>`.
//! 2. Before decoding the tape, [`row`] swaps the move-only `handed: Vec<Box<dyn Any>>` into
//!    thread-local `HANDED` (saving any enclosing `HANDED` frame so nested item closures compose
//!    without clobbering outer slots).
//! 3. While decoding, `N_KIT_VIEW` or generated placeholder nodes call [`handed`] to `Option::take`
//!    slot `i` exactly once and downcast it to `T`. Attempting to place a handed element twice,
//!    with the wrong type, or outside its row stops the app via `rt::app_stop`.
//! 4. On return, [`row`] restores the outer `HANDED` frame and passes any unconsumed values to
//!    `fallback` if the row tape was missing or failed to decode.

use crate::genapi::{self, Dx};
use crate::source;
use gpui_kit::{App, Window};
use std::any::Any;
use std::cell::RefCell;

thread_local! {
    /// Stack-scoped slots holding values handed by upstream during the active [`row`] decode.
    static HANDED: RefCell<Vec<Option<Box<dyn Any>>>> = const { RefCell::new(Vec::new()) };
}

/// Takes the `i`-th handed upstream value of type `T`; stops the app if already consumed or wrong type.
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

/// Decodes one row of decoded row source `src` while exposing `handed` values via [`handed`].
///
/// Restores any enclosing `HANDED` slots on return so nested item closures compose safely. If the
/// row tape is not ready yet or `decode` returns `None`, calls `fallback` with any remaining values.
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

/// Extracts and downcasts the first value from a `handed` list (used in fallback closures).
pub fn first<T: 'static>(handed: Vec<Box<dyn Any>>) -> Option<T> {
    handed.into_iter().next().and_then(|b| b.downcast::<T>().ok()).map(|b| *b)
}

/// Decodes `N_KIT_VIEW` by taking the indexed `AnyView` from the active `HANDED` slots.
pub fn node(dx: &mut Dx, ty: u16, args: u32, _first: u32) -> Option<gpui_kit::AnyElement> {
    if ty != crate::ops::N_KIT_VIEW {
        return None;
    }
    let i = dx.args(args).u8();
    Some(gpui_kit::IntoElement::into_any_element(handed::<gpui_kit::AnyView>(i)))
}

/// Marker wrapper for generated decoders that read an upstream reference into a wire context.
pub struct Reads<'a, T: ?Sized>(pub &'a T);
