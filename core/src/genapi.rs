//! The one module generated Rust may use (docs/PROTOCOLS.md section 7): the tape reader and
//! decode context, the handler trampolines (`fire`), the payload codecs, the style applier
//! and the state accessors. Everything here is thin and non-generic where it can be, so a
//! generated decoder of any size adds no copies of the runtime.

pub use crate::ops::*;
pub use crate::rt::{GkErr, GkResult, err};
pub use crate::wire::{Put, R};
// Library objects behind handles (docs/PROTOCOLS.md section 13).
pub use crate::objects::{
    Owned, obj_by_value, obj_clone, obj_global, obj_new, obj_rc, obj_read, obj_read_global, obj_shown, obj_take,
    obj_temp, obj_update, obj_update_global, obj_update_w, obj_update_w_global, value_arg, with_window,
};

use crate::rt::{self, Item, StateItem};
use crate::tape::Tape;
use gpui_kit::{
    Action, AnyElement, AnyView, AnyWindowHandle, App, ClickEvent, Context, Entity,
    EventEmitter, FocusHandle, Hsla, IntoElement, KeyDownEvent, KeyUpEvent, Modifiers,
    MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent, NavigationDirection, Pixels,
    Point, SharedString, StyleRefinement, Styled, Window, div, hsla,
};
use std::marker::PhantomData;
use std::rc::Rc;

// --- decoding a tape ----------------------------------------------------------------------

/// The context of one tape decode: the tape, the view it belongs to, and GPUI's window and
/// app as `Render::render` received them.
pub struct Dx<'a> {
    t: &'a Rc<Tape>,
    ops: &'a [u8],
    view: u64,
    pub window: &'a mut Window,
    pub cx: &'a mut App,
}

/// Decodes a view's tape into its root element. Called from `GkView::render` only.
pub(crate) fn root(tape: &Rc<Tape>, view: u64, window: &mut Window, cx: &mut App) -> AnyElement {
    let mut dx = Dx {
        t: tape,
        ops: tape.ops(),
        view,
        window,
        cx,
    };
    dx.child(tape.root)
}

/// A node of a tape kept by a closure and decoded each time upstream calls the closure: the
/// `fragment` resolution (tooltip, popover and dialog bodies) and the `build` resolution
/// (the node's ops applied to a value upstream hands in). It keeps the tape alive. A handler
/// inside it fires with that tape's generation, and the app keeps a view's handler tables
/// for its current tape and the one before: a closure that outlives two re-descriptions of
/// its view (a dialog left open) loses its handlers, and their events are dropped.
#[derive(Clone)]
pub struct Frag {
    tape: Rc<Tape>,
    view: u64,
    node: u32,
}

impl Frag {
    /// Runs `f` with a decode context over the kept tape and the kept node.
    pub fn with<T>(
        &self,
        window: &mut Window,
        cx: &mut App,
        f: impl FnOnce(&mut Dx, u32) -> T,
    ) -> T {
        let mut dx = Dx {
            t: &self.tape,
            ops: self.tape.ops(),
            view: self.view,
            window,
            cx,
        };
        f(&mut dx, self.node)
    }
}

impl<'a> Dx<'a> {
    /// Decodes node `n` (the argument of a `CHILD` op) into its element.
    pub fn child(&mut self, n: u32) -> AnyElement {
        let (ty, args, first) = self.t.node(n);
        if ty < 256 {
            return match crate::hand::node(self, ty, args, first) {
                Some(el) => el,
                None => {
                    eprintln!("gk: unknown node type {ty}");
                    div().into_any_element()
                }
            };
        }
        // Fast-path Div / HFlex / VFlex (core family, always linked): skips KEYS_NODE + FAM_REG.
        if (256..=258).contains(&ty) {
            return crate::generated::make_gpui_div(self, ty, args, first)
                .map(|e| e.into_any_element())
                .unwrap_or_else(|| div().into_any_element());
        }
        if let Some(el) = crate::generated::node(self, ty, args, first) {
            return el;
        }
        match crate::hand::node(self, ty, args, first) {
            Some(el) => el,
            None => {
                eprintln!("gk: unknown node type {ty}");
                div().into_any_element()
            }
        }
    }

    /// The view behind a `CHILD_VIEW` op, as a child element. None (and a message) if the
    /// handle is stale.
    pub fn child_view(&mut self, handle: u64) -> Option<AnyView> {
        match rt::with(|s| s.view(handle)) {
            Ok(view) => Some(view.into()),
            Err(GkErr(_, msg)) => {
                eprintln!("gk: child_view: {msg}");
                None
            }
        }
    }

    /// (type, args offset, first op offset) of any node: for an argument that is a node of
    /// a known type (an icon, a column, a menu item, a style).
    pub fn node(&self, n: u32) -> (u16, u32, u32) {
        self.t.node(n)
    }

    /// Keeps a node for a closure that decodes it later.
    pub fn frag(&self, node: u32) -> Frag {
        Frag {
            tape: self.t.clone(),
            view: self.view,
            node,
        }
    }

    /// A reader at a node's constructor arguments.
    pub fn args(&self, at: u32) -> R<'a> {
        R::at(self.ops, at as usize)
    }

    /// The next op of a node: its opcode and a reader at its arguments. `at` starts as the
    /// node's first op offset and is advanced; None at the end.
    #[inline]
    pub fn op(&self, at: &mut u32) -> Option<(u16, R<'a>)> {
        if *at == 0 {
            return None;
        }
        let mut r = R::at(self.ops, *at as usize);
        let op = r.u16();
        *at = r.u32();
        Some((op, r))
    }

    /// The view whose tape is being decoded.
    pub fn view_handle(&self) -> u64 {
        self.view
    }

    /// Keeps this description's handler table in the binding for as long as the value holding the
    /// pin (`tape::GenPin`): every handler closure made from the tape holds one, so a handler can
    /// fire for as long as upstream keeps its closure (an open menu's items, made from the
    /// description that drew the button, after the view has been described again many times).
    pub fn pin(&self) -> TapePin {
        TapePin(self.t.pin.clone())
    }

    /// Reads a handler argument (u32 id) of an op.
    pub fn handler(&self, r: &mut R) -> HandlerRef {
        HandlerRef {
            view: self.view,
            generation: self.t.generation,
            handler: r.u32(),
        }
    }
}

// --- styles: one non-generic table for every styled type ----------------------------------

/// Lets the by-value `Styled` methods write into a borrowed `StyleRefinement`.
pub struct StyleOf<'a>(pub &'a mut StyleRefinement);

impl Styled for StyleOf<'_> {
    fn style(&mut self) -> &mut StyleRefinement {
        self.0
    }
}

/// A colour: u8 kind, then four f32 (kind 0, an HSLA literal) or a u16 colour number
/// (kind 1): a number of the generator's table (theme colours, palette colours with bit 15
/// set), resolved against the current theme when the tape is decoded, so a theme switch
/// needs no new tape.
pub fn color(r: &mut R, cx: &App) -> Hsla {
    let kind = r.u8();
    color_of(kind, r, cx)
}

fn color_of(kind: u8, r: &mut R, cx: &App) -> Hsla {
    match kind {
        COLOR_THEME => {
            let token = r.u16();
            use gpui_kit::component::ActiveTheme;
            match crate::generated::color_token(&cx.theme().colors, token) {
                Some(c) => c,
                None => {
                    eprintln!("gk: unknown colour {token}");
                    cx.theme().foreground
                }
            }
        }
        COLOR_GRADIENT => {
            // Only a Fill takes a gradient; the front ends' types keep one from anywhere else
            // (Go's gk.Fill). Read past it all the same, and draw its first stop.
            let (_, _, from, _) = gradient_parts(r, cx);
            eprintln!("gk: a gradient was given where the kit takes one colour: its first stop is drawn");
            from.color
        }
        _ => hsla(r.f32(), r.f32(), r.f32(), r.f32()),
    }
}

/// A Fill (docs/PROTOCOLS.md 11.38): a colour as `color` reads it, or a linear gradient (kind
/// COLOR_GRADIENT: f32 angle in degrees, u8 colour space (0 sRGB, 1 Oklab), two stops of a colour
/// and an f32 percentage), as gpui's `linear_gradient(angle, linear_color_stop(..), ..)`. What
/// `Styled::bg` and the binding's plot paths and quads take.
pub fn background(r: &mut R, cx: &App) -> gpui_kit::Background {
    match r.u8() {
        COLOR_GRADIENT => gradient(r, cx),
        kind => gpui_kit::Background::from(color_of(kind, r, cx)),
    }
}

fn gradient(r: &mut R, cx: &App) -> gpui_kit::Background {
    let (angle, oklab, from, to) = gradient_parts(r, cx);
    let g = gpui_kit::linear_gradient(angle, from, to);
    if oklab { g.color_space(gpui_kit::ColorSpace::Oklab) } else { g }
}

fn gradient_parts(r: &mut R, cx: &App) -> (f32, bool, gpui_kit::LinearColorStop, gpui_kit::LinearColorStop) {
    let angle = r.f32();
    let oklab = r.u8() == 1;
    let c = color(r, cx);
    let from = gpui_kit::linear_color_stop(c, r.f32());
    let c = color(r, cx);
    let to = gpui_kit::linear_color_stop(c, r.f32());
    (angle, oklab, from, to)
}

/// Applies one style token (a nullary `Styled` method).
pub fn style_token(style: &mut StyleRefinement, token: u16) {
    if !crate::generated::style_token(style, token) {
        eprintln!("gk: unknown style token {token}");
    }
}

// --- payloads -----------------------------------------------------------------------------

/// A value that travels to the app as an event payload (docs/PROTOCOLS.md section 2).
pub trait Wire {
    fn put(&self, out: &mut Vec<u8>);
}

/// A value the app returns from a handler.
pub trait Unwire: Sized {
    fn get(r: &mut R) -> Self;
}

macro_rules! wire_num {
    ($($t:ty => $put:ident, $get:ident;)*) => {$(
        impl Wire for $t {
            #[inline]
            fn put(&self, out: &mut Vec<u8>) { out.$put(*self) }
        }
        impl Unwire for $t {
            #[inline]
            fn get(r: &mut R) -> Self { r.$get() }
        }
    )*};
}
wire_num! {
    u8 => put_u8, u8; u16 => put_u16, u16; u32 => put_u32, u32; u64 => put_u64, u64;
    i32 => put_i32, i32; i64 => put_i64, i64; f32 => put_f32, f32; f64 => put_f64, f64;
    bool => put_bool, bool;
}
impl Wire for i8 {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u8(*self as u8)
    }
}
impl Wire for i16 {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u16(*self as u16)
    }
}
impl Wire for Hsla {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_f32(self.h);
        out.put_f32(self.s);
        out.put_f32(self.l);
        out.put_f32(self.a);
    }
}

impl Wire for () {
    fn put(&self, _: &mut Vec<u8>) {}
}
impl Unwire for () {
    fn get(_: &mut R) -> Self {}
}
impl Wire for usize {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(*self as u64)
    }
}
impl Unwire for usize {
    fn get(r: &mut R) -> Self {
        r.usize()
    }
}
impl Wire for isize {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_i64(*self as i64)
    }
}
impl Wire for str {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(self)
    }
}
impl Wire for String {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(self)
    }
}
impl Wire for SharedString {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(self)
    }
}
impl Unwire for String {
    fn get(r: &mut R) -> Self {
        r.str().to_owned()
    }
}
impl Unwire for SharedString {
    fn get(r: &mut R) -> Self {
        SharedString::from(r.str().to_owned())
    }
}
impl<T: Wire + ?Sized> Wire for &T {
    #[inline]
    fn put(&self, out: &mut Vec<u8>) {
        (**self).put(out)
    }
}
impl<T: Wire> Wire for Option<T> {
    fn put(&self, out: &mut Vec<u8>) {
        match self {
            Some(v) => {
                out.put_u8(1);
                v.put(out);
            }
            None => out.put_u8(0),
        }
    }
}
impl<T: Unwire> Unwire for Option<T> {
    fn get(r: &mut R) -> Self {
        (r.u8() != 0).then(|| T::get(r))
    }
}
impl<T: Wire> Wire for [T] {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u32(self.len() as u32);
        for v in self {
            v.put(out);
        }
    }
}
impl<T: Wire> Wire for Vec<T> {
    fn put(&self, out: &mut Vec<u8>) {
        self.as_slice().put(out)
    }
}
impl<T: Unwire> Unwire for Vec<T> {
    fn get(r: &mut R) -> Self {
        (0..r.u32()).map(|_| T::get(r)).collect()
    }
}
macro_rules! wire_tuple {
    ($($n:ident $i:tt),+) => {
        impl<$($n: Wire),+> Wire for ($($n,)+) {
            fn put(&self, out: &mut Vec<u8>) { $(self.$i.put(out);)+ }
        }
    };
}
wire_tuple!(A 0, B 1);
wire_tuple!(A 0, B 1, C 2);
wire_tuple!(A 0, B 1, C 2, D 3);

impl Wire for Pixels {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_f32(f32::from(*self))
    }
}
/// A byte range (a selection, the visible rows): two u64, as `gk.Range{lo, hi}` reads it.
impl Wire for std::ops::Range<usize> {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.start as u64);
        out.put_u64(self.end as u64);
    }
}
// The runtime implements `Wire` for scalars, strings, `Pixels`, `Hsla`, references, options,
// lists, tuples and the six built-in event types, and for nothing else: every other record
// and enum is the generator's to implement, and an impl on both sides would not compile.
// So the parts of the built-in events are written by plain functions.

fn put_point(p: Point<Pixels>, out: &mut Vec<u8>) {
    out.put_f32(f32::from(p.x));
    out.put_f32(f32::from(p.y));
}

/// Bits: 1 control, 2 alt, 4 shift, 8 platform, 16 function.
fn put_mods(m: Modifiers, out: &mut Vec<u8>) {
    out.put_u8(
        m.control as u8
            | (m.alt as u8) << 1
            | (m.shift as u8) << 2
            | (m.platform as u8) << 3
            | (m.function as u8) << 4,
    )
}

/// 0 left, 1 right, 2 middle, 3 back, 4 forward.
fn put_button(b: MouseButton, out: &mut Vec<u8>) {
    out.put_u8(match b {
        MouseButton::Left => 0,
        MouseButton::Right => 1,
        MouseButton::Middle => 2,
        MouseButton::Navigate(NavigationDirection::Back) => 3,
        MouseButton::Navigate(NavigationDirection::Forward) => 4,
    })
}

/// Click_Event: position, button, modifiers, click count, kind (0 mouse, 1 keyboard, 2 touch).
impl Wire for ClickEvent {
    fn put(&self, out: &mut Vec<u8>) {
        put_point(self.position(), out);
        let button = if self.is_right_click() {
            1
        } else if self.is_middle_click() {
            2
        } else {
            0
        };
        out.put_u8(button);
        put_mods(self.modifiers(), out);
        out.put_u32(self.click_count() as u32);
        out.put_u8(match self {
            ClickEvent::Mouse(_) => 0,
            ClickEvent::Keyboard(_) => 1,
            ClickEvent::Touch(_) => 2,
        });
    }
}
/// Key_Event: key, key_char, modifiers, is_held.
impl Wire for KeyDownEvent {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.keystroke.key);
        self.keystroke.key_char.put(out);
        put_mods(self.keystroke.modifiers, out);
        out.put_bool(self.is_held);
    }
}
impl Wire for KeyUpEvent {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.keystroke.key);
        self.keystroke.key_char.put(out);
        put_mods(self.keystroke.modifiers, out);
        out.put_bool(false);
    }
}
/// Mouse_Event: position, button (255 none), modifiers, click count.
impl Wire for MouseDownEvent {
    fn put(&self, out: &mut Vec<u8>) {
        put_point(self.position, out);
        put_button(self.button, out);
        put_mods(self.modifiers, out);
        out.put_u32(self.click_count as u32);
    }
}
impl Wire for MouseUpEvent {
    fn put(&self, out: &mut Vec<u8>) {
        put_point(self.position, out);
        put_button(self.button, out);
        put_mods(self.modifiers, out);
        out.put_u32(self.click_count as u32);
    }
}
impl Wire for MouseMoveEvent {
    fn put(&self, out: &mut Vec<u8>) {
        put_point(self.position, out);
        match self.pressed_button {
            Some(b) => put_button(b, out),
            None => out.put_u8(255),
        }
        put_mods(self.modifiers, out);
        out.put_u32(0);
    }
}

// --- handlers -----------------------------------------------------------------------------

/// A view description's handlers kept by a closure made from it (`Dx::pin`, `tape::GenPin`).
#[derive(Clone)]
pub struct TapePin(#[allow(dead_code)] Option<std::sync::Arc<crate::tape::GenPin>>);

/// A handler of the app as a tape names it: the view, the tape generation it was registered
/// in, and its id in that generation's table. View 0 is a kept handler (slot, generation).
#[derive(Clone, Copy, Debug)]
pub struct HandlerRef {
    pub view: u64,
    pub generation: u32,
    pub handler: u32,
}

fn event_req<P: Wire + ?Sized>(h: HandlerRef, shape: u16, payload: &P) -> Vec<u8> {
    let mut req = Vec::with_capacity(48);
    req.put_u64(h.view);
    req.put_u32(h.generation);
    req.put_u32(h.handler);
    req.put_u16(shape);
    payload.put(&mut req);
    req
}

fn returned<T: Unwire + Default>(ret: Vec<u8>) -> T {
    if ret.is_empty() {
        T::default()
    } else {
        T::get(&mut R::new(&ret))
    }
}

/// An event from an element listener: one synchronous EVENT turn inside GPUI's dispatch,
/// with the real window and app in scope. Returns the handler's return value, or the
/// default when the handler returned none.
pub fn fire<P: Wire + ?Sized, T: Unwire + Default>(
    h: HandlerRef,
    shape: u16,
    payload: &P,
    window: &mut Window,
    cx: &mut App,
) -> T {
    returned(rt::fire_event(
        h.view,
        h.generation,
        h.handler,
        shape,
        |buf| payload.put(buf),
        Some(window),
        cx,
    ))
}

/// `fire` for a closure that receives no window.
pub fn fire_app<P: Wire + ?Sized, T: Unwire + Default>(
    h: HandlerRef,
    shape: u16,
    payload: &P,
    cx: &mut App,
) -> T {
    returned(rt::fire_event(
        h.view,
        h.generation,
        h.handler,
        shape,
        |buf| payload.put(buf),
        None,
        cx,
    ))
}

/// `fire` with a payload that is already on the wire, and the handler's return value as it
/// came back (empty when it returned none). The generated handler-shape test's fallback for a
/// payload that no codec can build in Rust (call op TEST_FIRE_SHAPE).
pub fn fire_raw(h: HandlerRef, shape: u16, payload: &[u8], window: &mut Window, cx: &mut App) -> Vec<u8> {
    struct Raw<'a>(&'a [u8]);
    impl Wire for Raw<'_> {
        fn put(&self, out: &mut Vec<u8>) {
            out.extend_from_slice(self.0)
        }
    }
    rt::fire_req(&event_req(h, shape, &Raw(payload)), Some(window), cx)
}

/// An event raised inside an entity's own update (delegate methods): queued, and delivered
/// when the outermost update has ended and before the next draw, never synchronously. The
/// handler gets its view's window in scope (for a kept handler, the app's first window).
pub fn fire_queued<P: Wire + ?Sized>(h: HandlerRef, shape: u16, payload: &P, cx: &mut App) {
    rt::queue_event(event_req(h, shape, payload), h.view, cx);
}

/// A focus handle behind a handle of kind FOCUS.
pub fn focus(handle: u64) -> GkResult<FocusHandle> {
    rt::with(|s| s.focus(handle)).map(|f| f.0)
}

/// An action argument: the u32 id of the app's action table (core/src/app.rs).
pub fn action(r: &mut R) -> GkResult<Box<dyn Action>> {
    crate::app::boxed(r.u32())
}

// Actions (core/src/app.rs, docs/PROTOCOLS.md section 11): the app's actions are one type, and
// an action travels to the app as its u32 id.
pub use crate::app::{GkAction, action_listener};

impl Wire for dyn Action {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u32(crate::app::id_of(self));
    }
}

impl Wire for GkAction {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u32(self.id);
    }
}

// --- call ops and state behind handles ------------------------------------------------------

/// The context of one call op: GPUI's app, the window if the turn has one, the result
/// buffer, and the op's own bytes (so a write that meets a leased state can be put back).
pub struct Call<'a> {
    pub cx: &'a mut App,
    pub out: &'a mut Vec<u8>,
    /// True when the op comes from a reply and runs outside the turn that wrote it.
    #[allow(dead_code)]
    pub deferred: bool,
    pub req: &'a [u8],
    window: Option<*mut Window>,
    _window: PhantomData<&'a mut Window>,
}

impl<'a> Call<'a> {
    pub(crate) fn new(
        cx: &'a mut App,
        window: Option<&'a mut Window>,
        out: &'a mut Vec<u8>,
        deferred: bool,
        req: &'a [u8],
    ) -> Self {
        Call {
            cx,
            out,
            deferred,
            req,
            window: window.map(|w| w as *mut Window),
            _window: PhantomData,
        }
    }

    /// The turn's window. GK_ERR_NO_CONTEXT if the turn has none. The reference is not a
    /// borrow of the `Call`, so `let window = c.window()?; c.cx.new(|cx| T::new(window, cx))`
    /// compiles; use one window reference at a time.
    pub fn window(&mut self) -> GkResult<&'a mut Window> {
        match self.window_opt() {
            Some(window) => Ok(window),
            None => err(
                ERR_NO_CONTEXT,
                "this call needs a window: make it in a view's init function or in a handler",
            ),
        }
    }

    pub(crate) fn window_opt(&mut self) -> Option<&'a mut Window> {
        // SAFETY: the pointer was made from a `&'a mut Window` that the caller of the call
        // op does not touch while the `Call` lives, and the op uses one reference at a time.
        self.window.map(|w| unsafe { &mut *w })
    }

    pub fn window_handle(&self) -> Option<AnyWindowHandle> {
        // SAFETY: as above; a shared read.
        self.window.map(|w| unsafe { (*w).window_handle() })
    }
}

/// Puts a kit entity behind a new state handle. The window in scope, if any, is remembered:
/// the state's mutators run with it.
pub fn state_new<T: 'static>(entity: Entity<T>, c: &mut Call) -> u64 {
    let window = c.window_handle();
    let observed = entity.clone();
    let observe = Rc::new(move |cx: &mut App, view: u64| {
        cx.observe(&observed, move |_, cx| {
            rt::guard("marking a view dirty", || rt::mark_dirty(view, None, cx));
        })
    });
    rt::with(|s| {
        s.insert(
            HK_STATE,
            Item::State(StateItem {
                any: Rc::new(entity),
                type_name: std::any::type_name::<T>(),
                window,
                observe,
            }),
        )
    })
}

fn state_parts<T: 'static>(handle: u64) -> GkResult<(Entity<T>, Option<AnyWindowHandle>)> {
    rt::with(|s| {
        let item = s.state(handle)?;
        match item.any.downcast_ref::<Entity<T>>() {
            Some(entity) => Ok((entity.clone(), item.window)),
            None => err(
                ERR_STALE_HANDLE,
                format!(
                    "handle {handle:#x} is a {}, not a {}",
                    item.type_name,
                    std::any::type_name::<T>()
                ),
            ),
        }
    })
}

/// The entity behind a state handle, with its generation and type checked.
pub fn state<T: 'static>(handle: u64) -> GkResult<Entity<T>> {
    state_parts::<T>(handle).map(|p| p.0)
}

/// The entity behind a state handle that a tape names, for an element being drawn. A
/// released or wrong handle there is a mistake in app code, and the element cannot be drawn:
/// the program stops with the message (exit 2), as a buffered write to a released handle
/// does. (Until 11 Nov 2026 the decoder drew nothing in the element's place, with exit 0.)
pub fn shown<T: 'static>(handle: u64) -> Entity<T> {
    match state::<T>(handle) {
        Ok(entity) => entity,
        Err(GkErr(_, msg)) => released_shown(&msg, std::any::type_name::<T>()),
    }
}

/// `shown` for a focus handle a tape names (`track_focus`).
pub fn shown_focus(handle: u64) -> FocusHandle {
    match focus(handle) {
        Ok(f) => f,
        Err(GkErr(_, msg)) => released_shown(&msg, "FocusHandle"),
    }
}

/// Stops the app because a view's tape names a handle that cannot be used: a mistake in app
/// code (rt::app_stop).
pub fn released_shown(msg: &str, what: &str) -> ! {
    let what = app_type_name(what);
    let hint = if msg.contains("never made") {
        "The handle is zero: the kit state (or row source) this element shows was never made. \
         Make it once in init, keep the handle in your state struct, and give that field to the \
         element, as in editor.Input(a.search)."
    } else {
        "The element cannot be drawn. Release kit state only after every view that shows it \
         has been described without it: a handler that releases it marks its own view for \
         redraw, gk.Notify marks any other."
    };
    crate::rt::app_stop(format!("a view shows a {what} through a handle that cannot be used: {msg}\n    {hint}"))
}

/// A Rust type name as an app developer reads it: the module paths left out
/// (`gpui_base::input::state::InputBaseState<gpui_base::input::kind::InputMode>` is
/// `InputBaseState<InputMode>`), the upstream name the API guide maps to the Go one.
pub fn app_type_name(t: &str) -> String {
    let mut out = String::with_capacity(t.len());
    let mut seg = String::new();
    for ch in t.chars() {
        if ch.is_alphanumeric() || ch == '_' || ch == ':' {
            seg.push(ch);
        } else {
            out.push_str(seg.rsplit("::").next().unwrap_or(""));
            seg.clear();
            out.push(ch);
        }
    }
    out.push_str(seg.rsplit("::").next().unwrap_or(""));
    out
}

/// A read (`&self` method). GK_ERR_BUSY if the state is on lease.
pub fn state_read<T: 'static, V>(
    handle: u64,
    c: &mut Call,
    f: impl FnOnce(&T, &App) -> V,
) -> GkResult<V> {
    let entity = state::<T>(handle)?;
    let cx: &App = &*c.cx;
    rt::probe(&entity, cx, |s| f(s, cx))
}

/// A write (`&mut self, .., window, cx: &mut Context<Self>` method), with the state's own
/// window. If the state is on lease the op is put back and runs when that update has ended.
pub fn state_update<T: 'static>(
    handle: u64,
    c: &mut Call,
    f: impl FnOnce(&mut T, &mut Window, &mut Context<T>),
) -> GkResult<()> {
    let (entity, window) = state_parts::<T>(handle)?;
    if rt::probe(&entity, c.cx, |_| ()).is_err() {
        rt::defer_op(c.req.to_vec(), c.cx);
        return Ok(());
    }
    let any = match window.or_else(|| c.window_handle()) {
        Some(any) => any,
        None => return err(ERR_NO_CONTEXT, "this state has no window to be updated in"),
    };
    rt::with_window(any, c.window_opt(), c.cx, |window, cx| {
        entity.update(cx, |s, cx| f(s, window, cx));
        rt::written(handle, Some(window), cx);
    })
}

/// An update that returns a value (`&mut self, .., cx: &mut Context<Self>) -> V`): run at
/// once, not buffered, with the state's own window; the app's buffered writes were sent
/// before it (the binding flushes them), so it sees them. GK_ERR_BUSY if the state is on
/// lease: the value cannot wait for the update to end.
pub fn state_call<T: 'static>(
    handle: u64,
    c: &mut Call,
    f: impl FnOnce(&mut T, &mut Window, &mut Context<T>) -> Vec<u8>,
) -> GkResult<Vec<u8>> {
    let (entity, window) = state_parts::<T>(handle)?;
    rt::probe(&entity, c.cx, |_| ())?;
    let any = match window.or_else(|| c.window_handle()) {
        Some(any) => any,
        None => return err(ERR_NO_CONTEXT, "this state has no window to be updated in"),
    };
    rt::with_window(any, c.window_opt(), c.cx, |window, cx| {
        let out = entity.update(cx, |s, cx| f(s, window, cx));
        rt::written(handle, Some(window), cx);
        out
    })
}

/// The value a call op's `_c` codec decoded, or GK_ERR_BAD_ENUM for a variant this library
/// does not know (the package and the library disagree; the version check makes it rare).
pub fn decoded<T>(v: Option<T>) -> GkResult<T> {
    match v {
        Some(v) => Ok(v),
        None => err(ERR_BAD_ENUM, "a value with a variant this library does not know"),
    }
}

/// A write that needs no window (`&mut self, .., cx: &mut Context<Self>`).
pub fn state_update_cx<T: 'static>(
    handle: u64,
    c: &mut Call,
    f: impl FnOnce(&mut T, &mut Context<T>),
) -> GkResult<()> {
    let entity = state::<T>(handle)?;
    if rt::probe(&entity, c.cx, |_| ()).is_err() {
        rt::defer_op(c.req.to_vec(), c.cx);
        return Ok(());
    }
    entity.update(c.cx, |s, cx| f(s, cx));
    let window = c.window_opt();
    rt::written(handle, window, c.cx);
    Ok(())
}

/// Subscribes a kept handler (slot, generation) to the events `E` of a state. Returns the
/// subscription's handle; releasing it, or the state, ends the subscription.
pub fn state_subscribe<T, E>(
    handle: u64,
    slot: u32,
    generation: u32,
    shape: u16,
    c: &mut Call,
) -> GkResult<u64>
where
    T: EventEmitter<E> + 'static,
    E: Wire + 'static,
{
    state_subscribe_with::<T, E>(handle, slot, generation, shape, c, |e, out| e.put(out))
}

/// `state_subscribe` with the event's encoder given as a function (for an event type that
/// has no `Wire` impl of its own).
pub fn state_subscribe_with<T, E>(
    handle: u64,
    slot: u32,
    generation: u32,
    shape: u16,
    c: &mut Call,
    encode: fn(&E, &mut Vec<u8>),
) -> GkResult<u64>
where
    T: EventEmitter<E> + 'static,
    E: 'static,
{
    let (entity, window) = state_parts::<T>(handle)?;
    let sub = c.cx.subscribe(&entity, move |_, event: &E, cx| {
        // Subscription events arrive when the emitter's update has ended. The handler gets
        // the state's window in scope when that window is free.
        let mut req = event_req(
            HandlerRef {
                view: 0,
                generation,
                handler: slot,
            },
            shape,
            &(),
        );
        encode(event, &mut req);
        let with_window = match window {
            Some(any) => any
                .update(cx, |_, window, cx| {
                    rt::fire_req(&req, Some(window), cx);
                })
                .is_ok(),
            None => false,
        };
        if !with_window {
            rt::fire_req(&req, None, cx);
        }
    });
    Ok(rt::with(|s| s.insert(HK_SUB, Item::Sub(sub))))
}

// --- a view's tape decoded outside the view (delegate shims) ---------------------------------

/// Runs `f` over `tape`, the current tape of the view `view`, and its root node, outside that
/// view's own render: a delegate shim (core/src/shim_rows.rs) decodes a row's or cell's tape
/// (from `source::row_tape`) into the value the kit asks for, inside the kit element's render,
/// as a Rust delegate builds it. Handlers in it fire as the row view's, with the tape's
/// generation. The caller arranges to be redrawn when the tape changes (`source::watch`): the
/// row view itself is never drawn.
pub fn with_tape<T>(
    tape: &Rc<Tape>,
    view: u64,
    window: &mut Window,
    cx: &mut App,
    f: impl FnOnce(&mut Dx, u32) -> T,
) -> T {
    with_tape_node(tape, view, tape.root, window, cx, f)
}

/// Runs `f` over a specific `node` of `tape` (a cell inside a table row's tape).
#[inline]
pub fn with_tape_node<T>(
    tape: &Rc<Tape>,
    view: u64,
    node: u32,
    window: &mut Window,
    cx: &mut App,
    f: impl FnOnce(&mut Dx, u32) -> T,
) -> T {
    let mut dx = Dx {
        t: tape,
        ops: tape.ops(),
        view,
        window,
        cx,
    };
    timed_decode(|| f(&mut dx, node))
}

/// Runs a decode and counts it for gk-meter (`decodes`, `decode_ns`). The ship library times one
/// decode in eight, picked at random, and counts it eight times: reading the clock twice for
/// every table cell drawn cost about 4 us a frame of the 100,000-row table at 1,000 Hz (two
/// `clock_gettime` per cell, about 100 cells a frame), main-thread time the Rust reference does
/// not spend. The diag library times every decode.
#[inline]
pub(crate) fn timed_decode<T>(f: impl FnOnce() -> T) -> T {
    #[cfg(not(feature = "diag"))]
    {
        if rt::next_pick() & 7 != 0 {
            let out = f();
            gk_meter::bump_decode(0);
            return out;
        }
        let t0 = std::time::Instant::now();
        let out = f();
        let ns = t0.elapsed().as_nanos() as u64 * 8;
        gk_meter::bump_decode(ns);
        out
    }
    #[cfg(feature = "diag")]
    {
        let t0 = std::time::Instant::now();
        let out = f();
        let ns = t0.elapsed().as_nanos() as u64;
        gk_meter::bump_decode(ns);
        out
    }
}

// --- view-built closures (docs/PROTOCOLS.md section 11.6) ---------------------------------

/// Runs `f` with a decode context over a view's tape and its root node's type and first op:
/// a view-built closure applies the root's ops to the value upstream hands in
/// (core/src/overlay.rs).
pub(crate) fn with_root<T>(
    tape: &Rc<Tape>,
    view: u64,
    window: &mut Window,
    cx: &mut App,
    f: impl FnOnce(&mut Dx, u16, u32) -> T,
) -> T {
    let mut dx = Dx { t: tape, ops: tape.ops(), view, window, cx };
    let (ty, _, first) = tape.node(tape.root);
    f(&mut dx, ty, first)
}
