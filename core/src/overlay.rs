//! View-built closures, described values and fallible calls (docs/PROTOCOLS.md section 11.6).
//!
//! A closure upstream keeps and calls later, `Fn(T, &mut Window, &mut App) -> T + 'static`,
//! handed over by a call op (`WindowExt::open_dialog`, `open_sheet`): the op carries the handle
//! of an app view whose procedure describes a T (`gk.NewBuilt(cx, state, fn)` in Go). Each time
//! upstream calls the closure, the view's current tape is read and its root node's ops are
//! applied to the T upstream hands in (`ViewBuild::apply`). Reading the view's entity while
//! upstream draws makes the drawing view depend on it, so the dialog draws again when the view
//! is described again: its handlers run between frames, never during one.
//!
//! Also here: the dropdown button (the kit's `DropdownMenuPopover<Button>` has no public path,
//! so generated code cannot name it), `PopupMenu::submenu` without the menu's own context, the
//! constructor of a described value (never called), and the results of fallible calls.

use crate::genapi::{self, Dx, GkErr, GkResult, R};
use crate::ops::*;
use crate::rt;
use gpui_kit::component::kbd::Kbd;
use gpui_kit::component::menu::{AppMenuBar, DropdownMenu as _, PopupMenu, PopupMenuItem};
use gpui_kit::{AnyElement, App, Context, IntoElement as _, SharedString, Window};

/// The view behind a view-built closure, the node types its root may have, and the name of the
/// type upstream hands in (for the message when the root is another node type).
pub struct ViewBuild {
    view: u64,
    types: &'static [u16],
    name: &'static str,
}

impl ViewBuild {
    /// Reads the view's handle, checks it names a view, and draws it in the turn's window (the
    /// first open window for a write applied after its turn): it is marked dirty, so the frame
    /// that first calls the closure has its tape.
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

    /// Applies the root node of the view's current tape to the value upstream hands in. Before
    /// the view's first description the value is upstream's as it is (one frame at most).
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

/// The constructor of a described value (`PopupMenu`): the app starts the node with it, and the
/// library only ever applies the node's ops to the value upstream hands in.
pub fn described<T>() -> T {
    genapi::released_shown(
        "a described value was used on its own",
        &format!("{} (only the closure of a dropdown, context or submenu builds one)", std::any::type_name::<T>()),
    )
}

/// `PopupMenu::submenu` without the menu's own context: the child menu is built at once, as
/// upstream does, and attached as an item; upstream wires it to its parent when the menu draws.
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

/// `Result<(), E>` read back: the error's text, or nothing.
pub fn outcome<E: std::fmt::Debug>(r: Result<(), E>) -> Option<String> {
    r.err().map(|e| format!("{e:?}"))
}

/// A constructor or builder that returns `Result<Self, E>`: an error stops the app with its text
/// (exit 2), as a mistake in app code does (a questionnaire with two items of one name).
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

/// The library's own element nodes of this protocol (hand::node asks first).
pub fn node(dx: &mut Dx, ty: u16, args: u32, _first: u32) -> Option<AnyElement> {
    match ty {
        // u32 button node, u32 menu node (a PopupMenu described value): the kit's Button with a
        // dropdown menu (`DropdownMenu::dropdown_menu`), whose menu is built from the node each
        // time it opens.
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
        // u32 button node, u32 menu node, u16 anchor (gk.Anchor's number): the same with the menu's
        // corner (DropdownMenu::dropdown_menu_with_anchor)
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
        // u32 action, u8 how (0 in a key context, 1 for a focus handle, 2 global), opt<str> context,
        // u64 focus handle: the kit's Kbd of the action's binding, or nothing when it has none
        // (Kbd::binding_for_action, binding_for_action_in, global_binding_for_action)
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
        // u64 app menu bar (a state handle): the kit's AppMenuBar drawn where the view puts it
        N_APP_MENU_BAR => {
            let mut r: R = dx.args(args);
            Some(genapi::shown::<AppMenuBar>(r.u64()).into_any_element())
        }
        _ => None,
    }
}

/// gpui's Anchor from its number on the wire (api/opcodes.lock, as gk.Anchor declares it).
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

/// The overlays' library calls (core/src/dock.rs asks).
pub fn call(op: u16, _r: &mut R, c: &mut genapi::Call) -> Option<GkResult<()>> {
    Some(match op {
        // -> u64 app menu bar (kind STATE): AppMenuBar::new
        C_APP_MENU_BAR_NEW => {
            let bar = AppMenuBar::new(c.cx);
            genapi::Put::put_u64(c.out, crate::dock::entity_handle(&bar));
            Ok(())
        }
        // -> i64 nanoseconds since the library's clock began (PROTOCOLS.md 11.10)
        C_CLOCK_NOW => {
            genapi::Put::put_i64(c.out, since_epoch(std::time::Instant::now()));
            Ok(())
        }
        _ => return None,
    })
}

// ------------------------------------------------------------------ the library clock (PROTOCOLS.md 11.10)
//
// `std::time::Instant` has no number of its own: both ways it is the time since the library's clock
// began (the first time anything asked), as i64 nanoseconds (Go `time.Duration`). The toast
// manager's `push`, `dismiss`, `advance` take the app's reading of it (gk.clock_now).

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

/// An Instant argument: `ns` after the library's clock began (before it, for a negative `ns`).
pub fn instant(ns: i64) -> std::time::Instant {
    let e = epoch();
    let d = std::time::Duration::from_nanos(ns.unsigned_abs());
    if ns >= 0 { e + d } else { e.checked_sub(d).unwrap_or(e) }
}

/// An Instant read back.
pub fn put_instant(t: &std::time::Instant, out: &mut Vec<u8>) {
    genapi::Put::put_i64(out, since_epoch(*t));
}
