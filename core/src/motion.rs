//! Animations and motion values (docs/PROTOCOLS.md 13.30). Runtime of the platform and motion
//! families; the per-element wrappers are generated (`generated::motion_wrap`, `motion_trap`:
//! gen/gk_values.py `motion_wrappers`, one arm per element type, the rule of a type-changing
//! wrapper instantiated per element type).
//!
//! App code never runs while a frame is drawn, so nothing here calls the app: an animation is
//! described as data and the library evaluates it while it draws.
//!
//! - **gpui's `with_animation` / `with_animations`** over any styled element: the animator is a
//!   list of *tracks* (a style property, its value at delta 0 and at delta 1, the animation it
//!   follows); the library lerps each with the eased delta gpui hands the animator and sets it
//!   with `Styled`. The easing is one of gpui's and the kit's curves, by number.
//! - **The kit's `EffectTransition::apply`**, with the effect value the app built.
//! - **Motion values sampled while drawing**: the kit's `transition`, `spring`,
//!   `animate_keyframes`, `Presence::sample` and `TransitionState::sample`, each giving one
//!   style property its value this frame; they ask for the next frame themselves while moving,
//!   which re-renders the view that drew them (only its tape is decoded again).
//! - **`FocusTrapElement::focus_trap`** with a focus handle of the app (kind FOCUS).
//!
//! Node `ANIMATED` (args): u32 child node, u8 kind, then by kind
//!   1 animations: str id, u32 n, n x (u32 ms, u8 flags (1 repeat, 2 repeat synced), easing,
//!     f32 max fps (0: none)), u32 n, n x track (u16 animation index (0xffff: every one), u8
//!     property, value from, value to)
//!   2 effect: str id, u32 value node of the kit's EffectTransition
//!   3 motion values: u32 n, n x (u8 property, u8 source, the source's fields)
//!   4 focus trap: str id, u64 focus handle
//!   5 text selection scope (PROTOCOLS.md 13.31): u64 scope (`gk.text_selection_scope_id`)
//! A value is an f32 for a numeric property and a colour (genapi::color) for a colour property.
//! An easing is u8 kind and its parameters (see `easing`).

use crate::genapi::{self, Dx};
use crate::ops::*;
use crate::rt::{self, GkErr};
use crate::wire::R;
use gpui_kit::{
    Animation, AnimationExt, AnyElement, Hsla, IntoElement, SharedString, Styled, px,
};
use std::rc::Rc;
use std::time::Duration;

#[derive(Clone, Copy, PartialEq, Debug)]
pub enum Prop {
    Opacity,
    Top,
    Left,
    Right,
    Bottom,
    Width,
    Height,
    Bg,
    TextColor,
    BorderColor,
    Size,
}

fn prop(n: u8) -> Option<Prop> {
    use Prop::*;
    Some(match n {
        0 => Opacity,
        1 => Top,
        2 => Left,
        3 => Right,
        4 => Bottom,
        5 => Width,
        6 => Height,
        7 => Bg,
        8 => TextColor,
        9 => BorderColor,
        10 => Size,
        _ => return None,
    })
}

fn is_color(p: Prop) -> bool {
    matches!(p, Prop::Bg | Prop::TextColor | Prop::BorderColor)
}

#[derive(Clone, Copy, Debug)]
pub enum Val {
    F(f32),
    C(Hsla),
}

fn val(p: Prop, r: &mut R, cx: &gpui_kit::App) -> Val {
    if is_color(p) { Val::C(genapi::color(r, cx)) } else { Val::F(r.f32()) }
}

fn lerp(a: Val, b: Val, t: f32) -> Val {
    match (a, b) {
        (Val::F(a), Val::F(b)) => Val::F(a + (b - a) * t),
        (Val::C(a), Val::C(b)) => Val::C(gpui_kit::base::animation::Lerp::lerp(&a, &b, t)),
        (a, _) => a,
    }
}

/// Sets one property on a styled element.
pub fn set<E: Styled>(el: E, p: Prop, v: Val) -> E {
    match (p, v) {
        (Prop::Opacity, Val::F(f)) => el.opacity(f),
        (Prop::Top, Val::F(f)) => el.top(px(f)),
        (Prop::Left, Val::F(f)) => el.left(px(f)),
        (Prop::Right, Val::F(f)) => el.right(px(f)),
        (Prop::Bottom, Val::F(f)) => el.bottom(px(f)),
        (Prop::Width, Val::F(f)) => el.w(px(f)),
        (Prop::Height, Val::F(f)) => el.h(px(f)),
        (Prop::Size, Val::F(f)) => el.size(px(f)),
        (Prop::Bg, Val::C(c)) => el.bg(c),
        (Prop::TextColor, Val::C(c)) => el.text_color(c),
        (Prop::BorderColor, Val::C(c)) => el.border_color(c),
        _ => el,
    }
}

/// An easing curve by number: 0 linear, 1 quadratic, 2 ease in-out, 3 ease-out quint (gpui's);
/// 4 ease-out cubic, 5 ease-in cubic, 6 ease in-out cubic, 7 cubic Bézier (f32 x1, y1, x2, y2)
/// (the kit's `base::animation`); 8 bounce of another curve (gpui's `bounce`), 9 pulsating
/// between (f32 min, max) (gpui's).
pub fn easing(r: &mut R, depth: u32) -> Result<Rc<dyn Fn(f32) -> f32>, String> {
    use gpui_kit::base::animation as kit;
    Ok(match r.u8() {
        0 => Rc::new(gpui_kit::linear),
        1 => Rc::new(gpui_kit::quadratic),
        2 => Rc::new(gpui_kit::ease_in_out),
        3 => Rc::new(gpui_kit::ease_out_quint()),
        4 => Rc::new(kit::ease_out_cubic),
        5 => Rc::new(kit::ease_in_cubic),
        6 => Rc::new(kit::ease_in_out_cubic),
        7 => {
            let (a, b, c, d) = (r.f32(), r.f32(), r.f32(), r.f32());
            Rc::new(kit::cubic_bezier(a, b, c, d))
        }
        8 if depth == 0 => {
            let inner = easing(r, 1)?;
            Rc::new(gpui_kit::bounce(move |t| inner(t)))
        }
        9 => {
            let (lo, hi) = (r.f32(), r.f32());
            Rc::new(gpui_kit::pulsating_between(lo, hi))
        }
        k => return Err(format!("easing {k}: 0 to 9 (a bounce of a bounce is refused)")),
    })
}

pub struct Track {
    anim: u16,
    prop: Prop,
    from: Val,
    to: Val,
}

/// What a wrapper does, decoded from the node's arguments.
pub enum Wrap {
    Anim { id: SharedString, anims: Vec<Animation>, tracks: Rc<Vec<Track>> },
    Effect { id: SharedString, effect: gpui_kit::base::animation::EffectTransition },
    Values(Vec<(Prop, Val)>),
}

/// The generated arms call this for every styled element type.
pub fn styled<E: IntoElement + Styled + 'static>(el: E, w: Wrap) -> AnyElement {
    match w {
        Wrap::Anim { id, anims, tracks } => el
            .with_animations(id, anims, move |mut el, ix, delta| {
                for t in tracks.iter() {
                    if t.anim == 0xffff || t.anim as usize == ix {
                        el = set(el, t.prop, lerp(t.from, t.to, delta));
                    }
                }
                el
            })
            .into_any_element(),
        Wrap::Effect { id, effect } => effect.apply(el, id).into_any_element(),
        Wrap::Values(vals) => {
            let mut el = el;
            for (p, v) in vals {
                el = set(el, p, v);
            }
            el.into_any_element()
        }
    }
}

fn bad(msg: String) -> AnyElement {
    // A described animation the library cannot draw is the app's mistake (the Odin package
    // refuses what it can see at the app's line), found while drawing: it stops, as a released
    // handle shown by a view does (genapi::released_shown), never draws something else.
    eprintln!("gk: an animation cannot be drawn: {msg}");
    std::process::exit(2);
}

fn obj_rc<T: 'static>(h: u64, what: &str) -> std::rc::Rc<std::cell::RefCell<T>> {
    match crate::objects::obj_rc::<T>(h) {
        Ok(v) => v,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, what),
    }
}

/// One motion value this frame: u8 source, then
///   1 transition: str id, target value, u32 Transition node (the kit's `transition`)
///   2 spring: str id, f32 target, u32 Spring node (the kit's `spring`; numbers only)
///   3 keyframes: str id, u64 Keyframes<f32> object, u32 Timing node (`animate_keyframes`)
///   4 presence: u32 Presence node, value at progress 0, value at 1 (`Presence::sample`)
///   5 state: u64 TransitionState<f32> object, f32 target, u32 Transition node (`sample`)
fn sample(p: Prop, r: &mut R, dx: &mut Dx) -> Result<Val, String> {
    use gpui_kit::base;
    let src = r.u8();
    Ok(match src {
        1 => {
            let id = r.str().to_string();
            let target = val(p, r, dx.cx);
            let policy = crate::generated::build_base_transition(dx, r.u32()).ok_or("a transition's policy")?;
            match target {
                Val::F(f) => Val::F(base::transition(id, f, policy, dx.window, dx.cx)),
                Val::C(c) => Val::C(base::transition(id, c, policy, dx.window, dx.cx)),
            }
        }
        2 => {
            let id = r.str().to_string();
            let target = r.f32();
            if is_color(p) {
                return Err("a spring moves numbers: give it a numeric property (opacity, a position or a size)".into());
            }
            let policy = crate::generated::build_base_spring(dx, r.u32()).ok_or("a spring's policy")?;
            Val::F(base::spring(id, target, policy, dx.window, dx.cx))
        }
        3 => {
            let id = r.str().to_string();
            let kf = obj_rc::<base::Keyframes<f32>>(r.u64(), "Keyframes");
            let timing = crate::generated::build_base_timing(dx, r.u32()).ok_or("keyframes' timing")?;
            if is_color(p) {
                return Err("keyframes of numbers move a numeric property (opacity, a position or a size)".into());
            }
            let kf = kf.borrow();
            Val::F(base::animate_keyframes(id, &kf, timing, dx.window, dx.cx).value)
        }
        4 => {
            let presence = crate::generated::build_base_presence(dx, r.u32()).ok_or("a presence")?;
            let (from, to) = (val(p, r, dx.cx), val(p, r, dx.cx));
            let s = presence.sample(dx.window, dx.cx);
            lerp(from, to, s.progress)
        }
        5 => {
            let st = obj_rc::<base::TransitionState<f32>>(r.u64(), "TransitionState");
            let target = r.f32();
            let policy = crate::generated::build_base_transition(dx, r.u32()).ok_or("a transition's policy")?;
            if is_color(p) {
                return Err("a TransitionState<f32> moves a numeric property".into());
            }
            let st = st.borrow().clone();
            Val::F(st.sample(target, policy, dx.window, dx.cx).value)
        }
        s => return Err(format!("motion source {s}: 1 transition, 2 spring, 3 keyframes, 4 presence, 5 state")),
    })
}

/// The ANIMATED node.
pub fn node(dx: &mut Dx, ty: u16, args: u32, _first: u32) -> Option<AnyElement> {
    if ty != N_ANIMATED {
        return None;
    }
    let mut r: R = dx.args(args);
    let child = r.u32();
    let kind = r.u8();
    let w = match kind {
        1 => {
            let id = SharedString::from(r.str().to_string());
            let n = r.u32();
            let mut anims = Vec::with_capacity(n.min(16) as usize);
            for _ in 0..n {
                let ms = r.u32();
                let flags = r.u8();
                let ease = match easing(&mut r, 0) {
                    Ok(e) => e,
                    Err(e) => return Some(bad(e)),
                };
                let fps = r.f32();
                let mut a = Animation::new(Duration::from_millis(ms as u64)).with_easing(move |t| ease(t));
                if flags & 2 != 0 {
                    a = a.repeat_synced();
                } else if flags & 1 != 0 {
                    a = a.repeat();
                }
                if fps > 0.0 {
                    a = a.with_max_fps(fps);
                }
                anims.push(a);
            }
            if anims.is_empty() {
                return Some(bad("an animation needs at least one Animation".into()));
            }
            let n = r.u32();
            let mut tracks = Vec::with_capacity(n.min(32) as usize);
            for _ in 0..n {
                let anim = r.u16();
                let Some(p) = prop(r.u8()) else { return Some(bad("an animation track names no property this library knows".into())) };
                let from = val(p, &mut r, dx.cx);
                let to = val(p, &mut r, dx.cx);
                tracks.push(Track { anim, prop: p, from, to });
            }
            Wrap::Anim { id, anims, tracks: Rc::new(tracks) }
        }
        2 => {
            let id = SharedString::from(r.str().to_string());
            match crate::generated::build_base_effect_transition(dx, r.u32()) {
                Some(effect) => Wrap::Effect { id, effect },
                None => return Some(bad("an EffectTransition this library cannot build".into())),
            }
        }
        3 => {
            let n = r.u32();
            let mut vals = Vec::with_capacity(n.min(32) as usize);
            for _ in 0..n {
                let Some(p) = prop(r.u8()) else { return Some(bad("a motion value names no property this library knows".into())) };
                match sample(p, &mut r, dx) {
                    Ok(v) => vals.push((p, v)),
                    Err(e) => return Some(bad(e)),
                }
            }
            Wrap::Values(vals)
        }
        4 => {
            let id = SharedString::from(r.str().to_string());
            let focus = match rt::with(|s| s.focus(r.u64())) {
                Ok((f, _)) => f,
                Err(GkErr(_, msg)) => genapi::released_shown(&msg, "FocusHandle"),
            };
            return Some(match crate::generated::motion_trap(dx, child, id, &focus) {
                Some(el) => el,
                None => bad("focus_trap takes an element that is a parent, styled and an Element (gk.div())".into()),
            });
        }
        5 => {
            // ElementExt::text_selection_scope (PROTOCOLS.md 13.31): a scope id the app made.
            let scope = crate::platform::scope_shown(r.u64());
            return Some(match crate::generated::motion_scope(dx, child, scope) {
                Some(el) => el,
                None => bad("text_selection_scope takes an element that is a parent (gk.div())".into()),
            });
        }
        k => return Some(bad(format!("animated node kind {k}: 1 animations, 2 effect, 3 motion values, 4 focus trap, 5 text selection scope"))),
    };
    Some(match crate::generated::motion_wrap(dx, child, w) {
        Some(el) => el,
        None => bad("an animation wraps a styled element (one of gk.Animatable_Element)".into()),
    })
}

// ------------------------------------------------------------------ svg transformations (13.30)

/// gpui's Transformation of an svg: f32 scale x, y, translation x, y (pixels), rotation (radians),
/// alpha (1 opaque; Odin writes 1 for a gk.Transformation without one).
pub fn transformation(r: &mut R) -> gpui_kit::Transformation {
    let scale = gpui_kit::size(r.f32(), r.f32());
    let translate = gpui_kit::point(gpui_kit::px(r.f32()), gpui_kit::px(r.f32()));
    let rotate = gpui_kit::radians(r.f32());
    gpui_kit::Transformation::scale(scale)
        .with_translation(translate)
        .with_rotation(rotate)
        .with_alpha(r.f32())
}

/// gpui's easing of a timed transformation (Svg_Easing, Linear = 0); an unknown one is Linear.
fn svg_easing(n: u8) -> gpui_kit::Easing {
    use gpui_kit::Easing as E;
    match n {
        1 => E::Quadratic,
        2 => E::EaseInOut,
        3 => E::EaseOutQuint,
        4 => E::Heartbeat,
        _ => E::Linear,
    }
}

/// gpui's TimedTransformation (Svg::with_timed_transformation): i64 period in nanoseconds (a
/// negative one is zero: nothing moves), the transformations at the start and the end of each
/// period, u8 easings of the scale, translation, rotation and alpha, u8 alternate, f32 the most
/// frames a second it asks for (0: no limit; upstream ignores a value that is not positive).
pub fn timed_transformation(r: &mut R) -> gpui_kit::TimedTransformation {
    let period = Duration::from_nanos(r.i64().max(0) as u64);
    let from = transformation(r);
    let to = transformation(r);
    let mut t = gpui_kit::TimedTransformation::new(period)
        .from(from)
        .to(to)
        .with_scale_easing(svg_easing(r.u8()))
        .with_translation_easing(svg_easing(r.u8()))
        .with_rotation_easing(svg_easing(r.u8()))
        .with_alpha_easing(svg_easing(r.u8()));
    if r.u8() != 0 {
        t = t.alternate();
    }
    t.with_max_fps(r.f32())
}
