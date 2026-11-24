//! Support surface imported by generated decoders and call handlers in `core/src/gen/`.
//!
//! # Responsibilities
//!
//! - **Tape decoding ([`Dx`], [`Frag`], [`with_tape`], [`with_root`])**: walks a parsed
//!   [`crate::tape::Tape`]'s node table and singly linked op arena (`[op: u16][next: u32][args...]`),
//!   dispatching opcodes `< 256` to hand-written decoders, `256..=258` directly to the div/flex
//!   fast path, and `>= 256` to generated builders. Lazy subtrees (menus, popovers, tooltips)
//!   capture a cloneable [`Frag`] holding an `Rc<Tape>` + node index so upstream closures can
//!   decode them on demand later.
//! - **Wire serialization ([`Wire`], [`Unwire`])**: little-endian encoders/decoders for primitives,
//!   length-prefixed strings/slices, option tags, tuples, colors/gradients, and GPUI mouse/key
//!   events.
//! - **Event dispatch ([`HandlerRef`], [`TapePin`], [`fire`], [`fire_app`], [`fire_raw`],
//!   [`fire_queued`])**: encodes `(view, generation, handler, shape, payload)` onto `K_EVENT`
//!   turns while [`TapePin`] prevents the tape's handler generation from retiring while GPUI
//!   closures still reference it.
//! - **Typed GPUI entity access ([`Call`], [`state_new`], [`state_read`], [`state_update`],
//!   [`state_call`], [`state_update_cx`], [`state_subscribe`])**: guards against GPUI's re-entrant
//!   entity-borrow panic via `rt::probe`. Void mutations (`state_update`, `state_update_cx`)
//!   automatically re-queue the raw wire op via `rt::defer_op` when the target entity is already
//!   borrowed on the call stack, whereas reads and value-returning calls fail with `ERR_BUSY`.

pub use crate::ops::*;
pub use crate::rt::{GkErr, GkResult, err};
pub use crate::wire::{Put, R};

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

/// Borrowed decode context threaded through element decoders while walking a [`Tape`].
pub struct Dx<'a> {
    t: &'a Rc<Tape>,
    ops: &'a [u8],
    view: u64,
    pub window: &'a mut Window,
    pub cx: &'a mut App,
}

/// Decodes the root node of `tape` into a GPUI `AnyElement`.
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

/// Owned handle to a subtree inside a [`Tape`], used by lazy builder closures (such as menus
/// or popovers) that decode their content on demand.
#[derive(Clone)]
pub struct Frag {
    tape: Rc<Tape>,
    view: u64,
    node: u32,
}

impl Frag {
    /// Opens a fresh [`Dx`] against this fragment's tape and invokes `f` on its root node index.
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
    /// Decodes node index `n`, routing opcodes `< 256` to hand-written decoders and `>= 256`
    /// to generated decoders (with a fast path for div/h_flex/v_flex opcodes 256..=258).
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

    /// Resolves a view handle into an `AnyView` for embedding a child view element.
    pub fn child_view(&mut self, handle: u64) -> Option<AnyView> {
        match rt::with(|s| s.view(handle)) {
            Ok(view) => Some(view.into()),
            Err(GkErr(_, msg)) => {
                eprintln!("gk: child_view: {msg}");
                None
            }
        }
    }

    /// Reads the raw `(node_type, args_offset, first_op_offset)` tuple for node `n`.
    pub fn node(&self, n: u32) -> (u16, u32, u32) {
        self.t.node(n)
    }

    /// Captures a cloneable [`Frag`] pointing at `node` inside the current tape.
    pub fn frag(&self, node: u32) -> Frag {
        Frag {
            tape: self.t.clone(),
            view: self.view,
            node,
        }
    }

    /// Returns a reader positioned at `at` in the tape's ops section for constructor arguments.
    pub fn args(&self, at: u32) -> R<'a> {
        R::at(self.ops, at as usize)
    }

    /// Advances an element's singly linked op list (`at == 0` terminates) and returns `(op, payload_reader)`.
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

    /// Returns the handle of the view currently being decoded.
    pub fn view_handle(&self) -> u64 {
        self.view
    }

    /// Clones the tape's [`TapePin`] so closures keep this tape's handler generation alive.
    pub fn pin(&self) -> TapePin {
        TapePin(self.t.pin.clone())
    }

    /// Reads a handler slot ID from `r` and binds it to the current view and tape generation.
    pub fn handler(&self, r: &mut R) -> HandlerRef {
        HandlerRef {
            view: self.view,
            generation: self.t.generation,
            handler: r.u32(),
        }
    }
}

/// Adapter letting `Styled` trait methods operate directly on a `&mut StyleRefinement`.
pub struct StyleOf<'a>(pub &'a mut StyleRefinement);

impl Styled for StyleOf<'_> {
    fn style(&mut self) -> &mut StyleRefinement {
        self.0
    }
}

/// Decodes a wire color into `Hsla`.
///
/// Wire formats by leading tag byte:
/// - `COLOR_THEME`: `[tag: u8][token: u16]` -> looks up `cx.theme().colors` via generated table
/// - `COLOR_HSLA`: `[tag: u8][h: f32][s: f32][l: f32][a: f32]`
/// - `COLOR_GRADIENT`: `[tag: u8][angle: f32][oklab: u8][from_color][from_pct: f32][to_color][to_pct: f32]`
///   (invalid where a single color is required; consumes the full gradient payload so subsequent
///   fields stay aligned and falls back to the first stop's color).
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
            // Still consume the full gradient payload so subsequent wire fields stay aligned.
            let (_, _, from, _) = gradient_parts(r, cx);
            eprintln!("gk: a gradient was given where the kit takes one colour: its first stop is drawn");
            from.color
        }
        _ => hsla(r.f32(), r.f32(), r.f32(), r.f32()),
    }
}

/// Decodes a wire background (solid color or two-stop linear gradient).
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

/// Applies a nullary style-token opcode to `style`.
pub fn style_token(style: &mut StyleRefinement, token: u16) {
    if !crate::generated::style_token(style, token) {
        eprintln!("gk: unknown style token {token}");
    }
}

/// Encodes a Rust value onto the little-endian wire format.
///
/// Container conventions shared across all codecs:
/// - `Option<T>`: `0u8` for `None`; `1u8` followed by `T` for `Some`
/// - `[T]` / `Vec<T>`: `u32` element count followed by each element in order
/// - `str` / `String` / `SharedString`: `u32` byte length followed by UTF-8 bytes
/// - `usize` / `isize`: widened to `u64` / `i64` for platform-independent ABI framing
pub trait Wire {
    fn put(&self, out: &mut Vec<u8>);
}

/// Decodes a Rust value from the little-endian wire format matching [`Wire`].
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

impl Wire for std::ops::Range<usize> {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.start as u64);
        out.put_u64(self.end as u64);
    }
}

fn put_point(p: Point<Pixels>, out: &mut Vec<u8>) {
    out.put_f32(f32::from(p.x));
    out.put_f32(f32::from(p.y));
}

/// Encodes `Modifiers` as a 5-bit mask: bit 0=control, 1=alt, 2=shift, 3=platform, 4=function.
fn put_mods(m: Modifiers, out: &mut Vec<u8>) {
    out.put_u8(
        m.control as u8
            | (m.alt as u8) << 1
            | (m.shift as u8) << 2
            | (m.platform as u8) << 3
            | (m.function as u8) << 4,
    )
}

/// Encodes `MouseButton` as `0=Left, 1=Right, 2=Middle, 3=Back, 4=Forward`.
fn put_button(b: MouseButton, out: &mut Vec<u8>) {
    out.put_u8(match b {
        MouseButton::Left => 0,
        MouseButton::Right => 1,
        MouseButton::Middle => 2,
        MouseButton::Navigate(NavigationDirection::Back) => 3,
        MouseButton::Navigate(NavigationDirection::Forward) => 4,
    })
}

/// Wire layout: `[x: f32][y: f32][button: u8][mods: u8][click_count: u32][source: u8 (0=Mouse, 1=Keyboard, 2=Touch)]`.
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

/// Wire layout: `[key: str][key_char: Option<str>][mods: u8][is_held: bool]`.
impl Wire for KeyDownEvent {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.keystroke.key);
        self.keystroke.key_char.put(out);
        put_mods(self.keystroke.modifiers, out);
        out.put_bool(self.is_held);
    }
}
/// Shares the [`KeyDownEvent`] wire layout with `is_held` always encoded as `false`.
impl Wire for KeyUpEvent {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.keystroke.key);
        self.keystroke.key_char.put(out);
        put_mods(self.keystroke.modifiers, out);
        out.put_bool(false);
    }
}

/// Wire layout: `[x: f32][y: f32][button: u8][mods: u8][click_count: u32]`.
impl Wire for MouseDownEvent {
    fn put(&self, out: &mut Vec<u8>) {
        put_point(self.position, out);
        put_button(self.button, out);
        put_mods(self.modifiers, out);
        out.put_u32(self.click_count as u32);
    }
}
/// Shares the [`MouseDownEvent`] wire layout.
impl Wire for MouseUpEvent {
    fn put(&self, out: &mut Vec<u8>) {
        put_point(self.position, out);
        put_button(self.button, out);
        put_mods(self.modifiers, out);
        out.put_u32(self.click_count as u32);
    }
}
/// Shares the mouse event wire layout with `button == 255` when no button is pressed and `click_count == 0`.
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

/// Cloneable RAII guard captured by GPUI event closures to defer retiring their tape generation.
#[derive(Clone)]
pub struct TapePin(#[allow(dead_code)] Option<std::sync::Arc<crate::tape::GenPin>>);

/// Identifies a host event handler by owning view handle, tape generation, and slot index.
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

/// Dispatches a synchronous event with window context to the host and decodes its return value.
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

/// Dispatches a synchronous event without window context to the host and decodes its return value.
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

/// Dispatches a pre-encoded raw payload to handler `h` and returns the raw reply bytes.
pub fn fire_raw(h: HandlerRef, shape: u16, payload: &[u8], window: &mut Window, cx: &mut App) -> Vec<u8> {
    struct Raw<'a>(&'a [u8]);
    impl Wire for Raw<'_> {
        fn put(&self, out: &mut Vec<u8>) {
            out.extend_from_slice(self.0)
        }
    }
    rt::fire_req(&event_req(h, shape, &Raw(payload)), Some(window), cx)
}

/// Enqueues an event for deferred delivery after the current entity update finishes.
pub fn fire_queued<P: Wire + ?Sized>(h: HandlerRef, shape: u16, payload: &P, cx: &mut App) {
    rt::queue_event(event_req(h, shape, payload), h.view, cx);
}

/// Resolves a focus handle ID into its GPUI `FocusHandle`.
pub fn focus(handle: u64) -> GkResult<FocusHandle> {
    rt::with(|s| s.focus(handle)).map(|f| f.0)
}

/// Decodes an action ID from `r` into a boxed `dyn Action`.
pub fn action(r: &mut R) -> GkResult<Box<dyn Action>> {
    crate::app::boxed(r.u32())
}

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

/// Context passed to every wire call handler during `gk_call` or deferred-op replay.
///
/// `window` is stored as a raw pointer (with `PhantomData<&'a mut Window>`) so callers can
/// simultaneously borrow `&mut self.cx`, `&mut self.out`, and obtain `&mut Window` via
/// [`Call::window`] without splitting borrows across struct fields. `req` holds the full wire
/// request slice so re-entrant writes can defer themselves verbatim via `rt::defer_op`.
pub struct Call<'a> {
    pub cx: &'a mut App,
    pub out: &'a mut Vec<u8>,
    #[allow(dead_code)]
    pub deferred: bool,
    pub req: &'a [u8],
    // Stored as a raw pointer so `Call` can hand out `&mut Window` alongside `&mut App`.
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

    /// Returns the active mutable window, or `ERR_NO_CONTEXT` if the call ran outside a window.
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
        // SAFETY: `window` came from `Option<&'a mut Window>` in `Call::new`, is valid for `'a`,
        // and `&mut self` prevents creating aliasing `&mut Window` references concurrently.
        self.window.map(|w| unsafe { &mut *w })
    }

    /// Returns the `AnyWindowHandle` of the active window, if one is in scope.
    pub fn window_handle(&self) -> Option<AnyWindowHandle> {
        // SAFETY: `window` is valid for `'a` and we only take a shared borrow to read its handle.
        self.window.map(|w| unsafe { (*w).window_handle() })
    }
}

/// Registers a newly created GPUI entity in the handle table and wires its dirty-view observer.
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

/// Looks up a typed GPUI `Entity<T>` from its `HK_STATE` handle.
pub fn state<T: 'static>(handle: u64) -> GkResult<Entity<T>> {
    state_parts::<T>(handle).map(|p| p.0)
}

/// Resolves a state handle referenced by an element on a tape, stopping the app cleanly if invalid.
pub fn shown<T: 'static>(handle: u64) -> Entity<T> {
    match state::<T>(handle) {
        Ok(entity) => entity,
        Err(GkErr(_, msg)) => released_shown(&msg, std::any::type_name::<T>()),
    }
}

/// Resolves a focus handle referenced by an element on a tape, stopping the app cleanly if invalid.
pub fn shown_focus(handle: u64) -> FocusHandle {
    match focus(handle) {
        Ok(f) => f,
        Err(GkErr(_, msg)) => released_shown(&msg, "FocusHandle"),
    }
}

/// Aborts the current turn with an explanatory misuse message when a view references a zero or
/// released handle.
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

/// Strips module prefixes from a Rust `type_name` string while keeping generic punctuation.
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

/// Reads an `Entity<T>` after verifying it is not currently borrowed on the stack.
pub fn state_read<T: 'static, V>(
    handle: u64,
    c: &mut Call,
    f: impl FnOnce(&T, &App) -> V,
) -> GkResult<V> {
    let entity = state::<T>(handle)?;
    let cx: &App = &*c.cx;
    rt::probe(&entity, cx, |s| f(s, cx))
}

/// Updates an `Entity<T>` with window access, deferring the raw wire op if `entity` is already borrowed.
///
/// Calls `rt::probe` first; if GPUI currently holds a mutable borrow of `entity` up the stack
/// (for example inside an event emitted by that entity), the raw wire bytes in `c.req` are pushed
/// onto `rt::defer_op` to run after the handler returns instead of panicking. After a live update,
/// calls `rt::written` so views depending on `handle` re-describe in the same frame if inside a hook.
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

/// Updates an `Entity<T>` with window access and returns serialized bytes; fails immediately if re-entrant.
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

/// Converts an `Option<T>` from an enum decoder into a `GkResult`, returning `ERR_BAD_ENUM` on `None`.
pub fn decoded<T>(v: Option<T>) -> GkResult<T> {
    match v {
        Some(v) => Ok(v),
        None => err(ERR_BAD_ENUM, "a value with a variant this library does not know"),
    }
}

/// Updates an `Entity<T>` without window access, deferring the wire op if `entity` is already borrowed.
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

/// Subscribes the host handler `slot` to GPUI events of type `E: Wire` emitted by state `handle`.
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

/// Subscribes the host handler `slot` to GPUI events of type `E` using a custom encoder function.
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
        // Subscription handlers are global to the host slot table (`view == 0`).
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

/// Opens a timed [`Dx`] at `tape.root` and runs `f`.
pub fn with_tape<T>(
    tape: &Rc<Tape>,
    view: u64,
    window: &mut Window,
    cx: &mut App,
    f: impl FnOnce(&mut Dx, u32) -> T,
) -> T {
    with_tape_node(tape, view, tape.root, window, cx, f)
}

/// Opens a timed [`Dx`] at `node` inside `tape` and runs `f`.
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

/// Measures decode duration for `gk-meter`, sampling 1 in 8 calls in non-`diag` builds to keep
/// clock overhead negligible.
///
/// In release builds, `rt::next_pick() & 7 == 0` samples ~12.5% of decodes and multiplies the
/// measured elapsed nanoseconds by 8 to estimate cumulative decode time while still incrementing
/// the decode count on every call; `diag` builds time every decode accurately for deterministic gates.
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

/// Opens a [`Dx`] on `tape` and passes the root node's `(node_type, first_op_offset)` to `f`.
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
