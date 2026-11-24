//! Overlay, popup-menu, dropdown-button, keyboard-shortcut badge, and monotonic-clock helpers.
//!
//! # Protocols and Invariants
//!
//! - **View-backed overlay builders ([`ViewBuild`])**: modal dialogs, alert dialogs, sheets, and
//!   panel dropdown menus take a GPUI builder closure invoked on every frame while the overlay is
//!   open. The host supplies a dedicated view handle (`gk.NewBuilt`); [`ViewBuild::take`] mounts
//!   and dirties that view in the owning window, and [`ViewBuild::apply`] replays its latest tape's
//!   root builder ops onto the upstream overlay struct while verifying that the tape's root opcode
//!   is in `types` (stopping the app cleanly on root-type mismatch).
//! - **Described-value guard ([`described`])**: builder-only nodes such as `PopupMenu` or `Dialog`
//!   must only appear inside their corresponding builder closure; placing one standalone in a view
//!   tree aborts with an actionable misuse error.
//! - **Fallible constructor adapter ([`Fallible`])**: wraps upstream constructors returning
//!   `Result<T, E>` (such as `QuestionnaireState` or `Keyframes`) so invalid arguments invoke
//!   `rt::app_stop` with the debug error instead of panicking.
//! - **Monotonic clock (`C_CLOCK_NOW`, [`instant`], [`put_instant`])**: because `std::time::Instant`
//!   has no portable raw integer representation, all instants crossing the wire are encoded as
//!   signed `i64` nanoseconds relative to the process-local [`EPOCH`] initialized on first use.

use crate::genapi::{self, Dx, GkErr, GkResult, R};
use crate::ops::*;
use crate::rt;
use gpui_kit::component::kbd::Kbd;
use gpui_kit::component::menu::{AppMenuBar, DropdownMenu as _, PopupMenu, PopupMenuItem};
use gpui_kit::{AnyElement, App, Context, IntoElement as _, SharedString, Window};

/// References a host view whose tape supplies builder ops to configure an upstream overlay value.
pub struct ViewBuild {
    view: u64,
    types: &'static [u16],
    name: &'static str,
}

impl ViewBuild {
    /// Mounts `view` into the active window, marks it dirty, and binds expected root node types.
    pub fn take(view: u64, c: &mut genapi::Call, types: &'static [u16], name: &'static str) -> GkResult<ViewBuild> {
        rt::with(|s| s.view(view))?;
        let wh = match c.window_handle() {
            Some(any) => rt::with(|s| s.window_of(any)),
            None => rt::with(|s| s.windows.first().copied()),
        };
        let Some(wh) = wh else {
            return genapi::err(ERR_NO_CONTEXT, format!("a {name} needs an open window"));
        };
        rt::with(|s| s.mount(view, wh));
        rt::mark_dirty(view, None, c.cx);
        Ok(ViewBuild { view, types, name })
    }

    /// Replays the ops on `self.view`'s latest tape onto `v` via `f`.
    pub fn apply<T>(&self, v: T, window: &mut Window, cx: &mut App, f: fn(&mut Dx, T, u32) -> T) -> T {
        let entity = match rt::with(|s| s.view(self.view)) {
            Ok(e) => e,
            Err(GkErr(_, msg)) => genapi::released_shown(&msg, &format!("{} (the view gk.NewBuilt made)", self.name)),
        };
        let Some(tape) = entity.read(cx).tape.clone() else {
            return v;
        };
        let (types, name, view) = (self.types, self.name, self.view);
        genapi::with_root(&tape, view, window, cx, |dx, ty, first| {
            if !types.is_empty() && !types.contains(&ty) {
                crate::rt::app_stop(format!(
                    "the function given to gk.NewBuilt for a {name} returned a node of type {ty}, not a {name}: \
                     it must return the value it describes, e.g. overlay.{name}()..."
                ));
            }
            f(dx, v, first)
        })
    }
}

/// Stops the app if a menu-builder placeholder node is placed directly in the element tree.
pub fn described<T>() -> T {
    genapi::released_shown(
        "a described value was used on its own",
        &format!("{} (only the closure of a dropdown, context or submenu builds one)", std::any::type_name::<T>()),
    )
}

/// Appends a child submenu built by `f` to `menu`.
pub fn submenu(
    menu: PopupMenu,
    label: impl Into<SharedString>,
    window: &mut Window,
    cx: &mut App,
    f: impl Fn(PopupMenu, &mut Window, &mut Context<PopupMenu>) -> PopupMenu + 'static,
) -> PopupMenu {
    let child = PopupMenu::build(window, cx, f);
    menu.item(PopupMenuItem::submenu(label, child))
}

/// Converts a `Result<(), E>` into `Option<String>` (`None` on `Ok`, debug-formatted error on `Err`).
pub fn outcome<E: std::fmt::Debug>(r: Result<(), E>) -> Option<String> {
    r.err().map(|e| format!("{e:?}"))
}

/// Wrapper used by generated decoders to invoke fallible upstream constructors and report errors
/// via [`crate::rt::app_stop`].
pub struct Fallible<F>(pub F);

fn refused<E: std::fmt::Debug>(e: E) -> ! {
    crate::rt::app_stop(format!("a constructor refused its arguments: {e:?}\n    Run with GK_DEBUG=1 for the line that made it."))
}

impl<F> Fallible<F> {
    pub fn call1<A, T, E: std::fmt::Debug>(self, a: A) -> T
    where
        F: FnOnce(A) -> Result<T, E>,
    {
        (self.0)(a).unwrap_or_else(|e| refused(e))
    }
    pub fn call2<A, B, T, E: std::fmt::Debug>(self, a: A, b: B) -> T
    where
        F: FnOnce(A, B) -> Result<T, E>,
    {
        (self.0)(a, b).unwrap_or_else(|e| refused(e))
    }
    pub fn call3<A, B, C, T, E: std::fmt::Debug>(self, a: A, b: B, c: C) -> T
    where
        F: FnOnce(A, B, C) -> Result<T, E>,
    {
        (self.0)(a, b, c).unwrap_or_else(|e| refused(e))
    }
    pub fn call4<A, B, C, D, T, E: std::fmt::Debug>(self, a: A, b: B, c: C, d: D) -> T
    where
        F: FnOnce(A, B, C, D) -> Result<T, E>,
    {
        (self.0)(a, b, c, d).unwrap_or_else(|e| refused(e))
    }
}

/// Decodes hand-written overlay/menu nodes:
/// - `N_DROPDOWN_BUTTON`: `[button_node: u32][menu_node: u32]`
/// - `N_DROPDOWN_ANCHOR`: `[button_node: u32][menu_node: u32][anchor: u16]`
/// - `N_KBD_FOR_ACTION`: `[action: u32][mode: u8 (0=context, 1=focus, 2=global)][has_ctx: u8][ctx?][focus: u64]`
/// - `N_APP_MENU_BAR`: `[state_handle: u64]`
pub fn node(dx: &mut Dx, ty: u16, args: u32, _first: u32) -> Option<AnyElement> {
    match ty {
        N_DROPDOWN_BUTTON => {
            let mut r: R = dx.args(args);
            let (b, m) = (r.u32(), r.u32());
            let button = crate::generated::build_component_button(dx, b)?;
            let f = dx.frag(m);
            Some(
                button
                    .dropdown_menu(move |v, window, cx| {
                        f.with(window, cx, |dx, n| crate::generated::apply_component_popup_menu(dx, v, dx.node(n).2))
                    })
                    .into_any_element(),
            )
        }
        N_DROPDOWN_ANCHOR => {
            let mut r: R = dx.args(args);
            let (b, m, a) = (r.u32(), r.u32(), r.u16());
            let button = crate::generated::build_component_button(dx, b)?;
            let f = dx.frag(m);
            Some(
                button
                    .dropdown_menu_with_anchor(anchor(a), move |v, window, cx| {
                        f.with(window, cx, |dx, n| crate::generated::apply_component_popup_menu(dx, v, dx.node(n).2))
                    })
                    .into_any_element(),
            )
        }
        N_KBD_FOR_ACTION => {
            let mut r: R = dx.args(args);
            let action = genapi::action(&mut r).ok()?;
            let how = r.u8();
            let context = if r.u8() != 0 { Some(r.str().to_string()) } else { None };
            let focus = r.u64();
            let kbd = match how {
                0 => Kbd::binding_for_action(&*action, context.as_deref(), &*dx.window),
                1 => Kbd::binding_for_action_in(&*action, &genapi::shown_focus(focus), &*dx.window),
                _ => Kbd::global_binding_for_action(&*action, &*dx.window),
            };
            Some(match kbd {
                Some(k) => k.into_any_element(),
                None => gpui_kit::Empty.into_any_element(),
            })
        }
        N_APP_MENU_BAR => {
            let mut r: R = dx.args(args);
            Some(genapi::shown::<AppMenuBar>(r.u64()).into_any_element())
        }
        _ => None,
    }
}

/// Maps wire anchor tags (`0..=7`) to `gpui_kit::Anchor`:
/// `0` TopLeft (default), `1` TopRight, `2` BottomLeft, `3` BottomRight,
/// `4` TopCenter, `5` BottomCenter, `6` LeftCenter, `7` RightCenter.
fn anchor(n: u16) -> gpui_kit::Anchor {
    use gpui_kit::Anchor::*;
    match n {
        1 => TopRight,
        2 => BottomLeft,
        3 => BottomRight,
        4 => TopCenter,
        5 => BottomCenter,
        6 => LeftCenter,
        7 => RightCenter,
        _ => TopLeft,
    }
}

/// Handles `C_APP_MENU_BAR_NEW` and `C_CLOCK_NOW`.
pub fn call(op: u16, _r: &mut R, c: &mut genapi::Call) -> Option<GkResult<()>> {
    Some(match op {
        C_APP_MENU_BAR_NEW => {
            let bar = AppMenuBar::new(c.cx);
            genapi::Put::put_u64(c.out, crate::dock::entity_handle(&bar));
            Ok(())
        }
        C_CLOCK_NOW => {
            genapi::Put::put_i64(c.out, since_epoch(std::time::Instant::now()));
            Ok(())
        }
        _ => return None,
    })
}

/// Process-local monotonic origin used to encode `std::time::Instant` values as signed nanoseconds.
static EPOCH: std::sync::OnceLock<std::time::Instant> = std::sync::OnceLock::new();

fn epoch() -> std::time::Instant {
    *EPOCH.get_or_init(std::time::Instant::now)
}

fn since_epoch(t: std::time::Instant) -> i64 {
    let e = epoch();
    if t >= e {
        t.duration_since(e).as_nanos().min(i64::MAX as u128) as i64
    } else {
        -(e.duration_since(t).as_nanos().min(i64::MAX as u128) as i64)
    }
}

/// Reconstructs an `Instant` from signed nanoseconds relative to [`EPOCH`].
pub fn instant(ns: i64) -> std::time::Instant {
    let e = epoch();
    let d = std::time::Duration::from_nanos(ns.unsigned_abs());
    if ns >= 0 { e + d } else { e.checked_sub(d).unwrap_or(e) }
}

/// Encodes `t` as signed nanoseconds since [`EPOCH`] onto `out`.
pub fn put_instant(t: &std::time::Instant, out: &mut Vec<u8>) {
    genapi::Put::put_i64(out, since_epoch(*t));
}
