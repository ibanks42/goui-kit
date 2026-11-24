//! The platform and motion families' gallery pages, written the way a Rust GPUI-kit app
//! writes them; the Go twin is examples/go/gallery/motion_pages.go, the pages' lines are in
//! ref/gallery/pages.txt (docs/PROTOCOLS.md 13.30): gpui's with_animation and a chain of animations
//! with the kit's easing curves, the kit's EffectTransition, and the kit's motion values
//! (transition, spring, keyframes, presence, a TransitionState kept with the page's data) sampled in
//! render. Offscreen the clock moves only on `advance`, so every frame is at a fixed time.
//!
//! An animator's arithmetic is the binding's (core/src/motion.rs): from + (to - from) * delta.
use crate::Gallery;
use gpui_kit::base::animation::{EffectTransition, Lerp, cubic_bezier, ease_out_cubic};
use gpui_kit::base::{
    Easing, FocusTrapElement, Keyframe, Keyframes, Presence, Spring, Timing, Transition, TransitionState,
    animate_keyframes, spring, transition,
};
use gpui_kit::component::button::Button;
use gpui_kit::component::label::Label;
use gpui_kit::component::v_flex;
use gpui_kit::{
    Animation, AnimationExt, AnyElement, App, AppContext, Context, Div, FocusHandle, Hsla, InteractiveElement,
    IntoElement, ParentElement, Styled, Window, bounce, div, ease_in_out, hsla, px, quadratic,
};
// the elem-* pages
use gpui_kit::base::{ElementExt, TextSelectionScopeId, h_virtual_list, v_virtual_list};
use gpui_kit::component::avatar::Avatar;
use gpui_kit::component::{ActiveTheme, h_flex};
use gpui_kit::{
    DragMoveEvent, EmptyView, Entity, ExternalDragPayload, FileDragPaths, Image, ImageFormat, ImageSource, Pixels,
    RenderImage, Resource, RetainAllImageCache, Role, ScrollAnchor, ScrollHandle, Size, StatefulInteractiveElement,
    img, retain_all, size,
};
use std::path::{Path, PathBuf};
use std::rc::Rc;
use std::sync::Arc;
use std::time::Duration;

type Cx<'a> = Context<'a, Gallery>;

/// The pages' own state, made when the gallery starts.
pub struct MotionPages {
    frames: Keyframes<f32>,
    state: TransitionState<f32>,
    focus: FocusHandle,
    // the elem-* pages
    cache: Entity<RetainAllImageCache>,
    pic: Arc<RenderImage>, // made once, as the bindings' gk.render_image (one texture)
    scroll: ScrollHandle,
    anchor: ScrollAnchor,
    scope: TextSelectionScopeId,
}

impl MotionPages {
    pub fn new(cx: &mut App) -> Self {
        let frames = Keyframes::try_new([
            Keyframe::new(0., 40.).ease(Easing::Linear),
            Keyframe::new(0.5, 200.).ease(Easing::Linear),
            Keyframe::new(1., 100.).ease(Easing::Linear),
        ])
        .expect("keyframes");
        // ImageSource's Path variant reads this file, written where the gallery runs
        let _ = std::fs::write(ELEM_PNG_PATH, ELEM_PNG);
        let scroll = ScrollHandle::new();
        Self {
            frames,
            state: TransitionState::default(),
            focus: cx.focus_handle(),
            cache: RetainAllImageCache::new(cx),
            pic: checker(),
            anchor: ScrollAnchor::for_handle(scroll.clone()),
            scroll,
            scope: TextSelectionScopeId::new(),
        }
    }
}

fn red() -> Hsla {
    hsla(0., 0.7, 0.5, 1.)
}

fn blue() -> Hsla {
    hsla(0.6, 0.7, 0.5, 1.)
}

fn lerp(a: f32, b: f32, t: f32) -> f32 {
    a + (b - a) * t
}

fn boxed() -> Div {
    div().relative().size(px(40.)).bg(hsla(0.33, 0.6, 0.45, 1.))
}

fn toggle(label: &'static str, cx: &mut Cx) -> Button {
    Button::new("toggle").label(label).on_click(cx.listener(|this, _, _, cx| {
        this.count += 1;
        cx.notify();
    }))
}

pub fn page(id: &str, g: &Gallery, window: &mut Window, cx: &mut Cx) -> Option<AnyElement> {
    let m = &g.motion;
    Some(match id {
        "motion-animation" => v_flex()
            .gap_2()
            .w(px(320.))
            .h(px(120.))
            .child(boxed().with_animation(
                "slide",
                Animation::new(Duration::from_millis(400)).with_easing(ease_in_out),
                |el, d| el.left(px(lerp(0., 200., d))).opacity(lerp(0.2, 1., d)),
            ))
            .child(div().size(px(30.)).bg(red()).with_animation(
                "colour",
                Animation::new(Duration::from_millis(400)).with_easing(cubic_bezier(0.4, 0., 0.2, 1.)),
                |el, d| el.bg(Lerp::lerp(&red(), &blue(), d)).w(px(lerp(30., 120., d))),
            ))
            .into_any_element(),
        "motion-chain" => div()
            .w(px(320.))
            .h(px(80.))
            .child(boxed().with_animations(
                "chain",
                vec![
                    Animation::new(Duration::from_millis(200)).with_easing(ease_out_cubic),
                    Animation::new(Duration::from_millis(200)).with_easing(bounce(quadratic)),
                ],
                |el, ix, d| match ix {
                    0 => el.left(px(lerp(0., 150., d))),
                    _ => el.h(px(lerp(40., 70., d))),
                },
            ))
            .into_any_element(),
        "motion-effect" => div()
            .w(px(320.))
            .h(px(80.))
            .child(
                EffectTransition::new(Duration::from_millis(300))
                    .fade(0., 1.)
                    .slide_x(px(-40.), px(0.))
                    .width(px(60.), px(180.))
                    .apply(div().relative().h(px(40.)).bg(blue()).child(Label::new("enter")), "enter"),
            )
            .into_any_element(),
        "motion-transition" => {
            let open = g.count % 2 == 1;
            let h = transition("h", if open { 120f32 } else { 40. }, Transition::new(Duration::from_millis(300)), window, cx);
            let c = transition("c", if open { blue() } else { red() }, Transition::new(Duration::from_millis(300)), window, cx);
            v_flex()
                .gap_2()
                .w(px(320.))
                .child(toggle("Toggle", cx))
                .child(div().w(px(120.)).h(px(40.)).bg(red()).h(px(h)).bg(c))
                .into_any_element()
        }
        "motion-spring" => {
            let open = g.count % 2 == 1;
            let x = spring("x", if open { 200f32 } else { 0. }, Spring::new(Duration::from_millis(250)).with_damping(0.5), window, cx);
            v_flex().gap_2().w(px(320.)).child(toggle("Toggle", cx)).child(boxed().left(px(x))).into_any_element()
        }
        "motion-keyframes" => {
            let w = animate_keyframes("kf", &m.frames, Timing::new(Duration::from_millis(600)), window, cx).value;
            div().w(px(320.)).h(px(60.)).child(div().h(px(40.)).bg(blue()).w(px(w))).into_any_element()
        }
        "motion-presence" => {
            let shown = g.count % 2 == 1;
            let p = Presence::new("p", shown).transition(Transition::new(Duration::from_millis(300))).sample(window, cx);
            let w = m.state.sample(if shown { 200. } else { 60. }, Transition::new(Duration::from_millis(300)), window, cx).value;
            v_flex()
                .gap_2()
                .w(px(320.))
                .child(toggle("Show", cx))
                .child(div().size(px(60.)).bg(red()).opacity(lerp(0., 1., p.progress)).w(px(w)))
                .into_any_element()
        }
        "motion-focus-trap" => div()
            .flex()
            .gap_2()
            .p_2()
            .border_1()
            .w(px(320.))
            .track_focus(&m.focus)
            .child(Button::new("one").label("One"))
            .child(Button::new("two").label("Two"))
            .focus_trap("trap", &m.focus)
            .into_any_element(),
        "elem-drop" => elem_drop(g, cx),
        "elem-image" => elem_image(m),
        "elem-scope" => elem_scope(m),
        "elem-vlist" => elem_vlist(cx),
        _ => return None,
    })
}

// --- PROTOCOLS.md 13.31: gpui's element leftovers, written as a Rust app writes them.

const ELEM_PNG: [u8; 83] = [
    137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 4, 0, 0, 0, 4, 8, 6, 0, 0, 0, 169, 241, 158,
    126, 0, 0, 0, 26, 73, 68, 65, 84, 120, 218, 99, 248, 127, 130, 225, 63, 67, 195, 255, 255, 48, 154, 1, 153, 3, 166, 9,
    170, 0, 0, 141, 255, 42, 33, 250, 235, 186, 63, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130,
];
const ELEM_PNG_PATH: &str = "gk-elem.png";

#[derive(Clone)]
struct Seven {
    #[allow(dead_code)]
    id: u64,
}

/// A 4 x 4 checker of two colours, as gpui keeps pixels (BGRA).
fn checker() -> Arc<RenderImage> {
    let mut px = Vec::with_capacity(64);
    for y in 0..4 {
        for x in 0..4 {
            let c: [u8; 4] = if (x + y) % 2 == 0 { [200, 40, 40, 255] } else { [40, 40, 200, 255] };
            px.extend_from_slice(&[c[2], c[1], c[0], c[3]]);
        }
    }
    let buf = image::RgbaImage::from_raw(4, 4, px).expect("4 x 4 pixels");
    Arc::new(RenderImage::new(vec![image::Frame::new(buf)]))
}

fn elem_drop(g: &Gallery, cx: &mut Cx) -> AnyElement {
    let src = div()
        .id("src")
        .w(px(100.))
        .h(px(60.))
        .bg(cx.theme().muted)
        .on_drag(Seven { id: 7 }, |_, _, _, cx| cx.new(|_| EmptyView))
        .external_drag_payload::<Seven>(|_, _, _| {
            Some(ExternalDragPayload::Files(FileDragPaths::new([(PathBuf::from("/tmp/seven.txt"), false)])))
        })
        .child(Label::new("seven"));
    let takes7 = div()
        .id("t7")
        .group("targets")
        .w(px(100.))
        .h(px(60.))
        .bg(cx.theme().secondary)
        .can_drop(|v, _, _| v.downcast_ref::<Seven>().is_some_and(|s| s.id == 7))
        .on_drop(cx.listener(|this, _: &Seven, _, cx| {
            this.count += 1;
            cx.notify();
        }))
        .on_drag_move::<Seven>(cx.listener(|this, _: &DragMoveEvent<Seven>, _, cx| {
            this.flag = true;
            cx.notify();
        }))
        .child(div().size(px(20.)).bg(red()).group_drag_over::<Seven>("targets", |s| s.bg(blue())));
    let takes8 = div()
        .id("t8")
        .w(px(100.))
        .h(px(60.))
        .bg(cx.theme().secondary)
        .can_drop(|v, _, _| v.downcast_ref::<Seven>().is_some_and(|s| s.id == 8))
        .on_drop(cx.listener(|this, _: &Seven, _, cx| {
            this.count += 1;
            cx.notify();
        }));
    v_flex()
        .gap_2()
        .child(h_flex().gap_4().child(src).child(takes7).child(takes8))
        .child(Label::new(format!("drops {} moved {}", g.count, g.flag)))
        .into_any_element()
}

fn elem_image(m: &MotionPages) -> AnyElement {
    // one image made once and shown three times, as the bindings' page does (gk.render_image)
    let render = m.pic.clone();
    let custom = m.pic.clone();
    h_flex()
        .gap_2()
        .image_cache(retain_all("elem-images"))
        .child(img(ImageSource::Render(render)).size(px(32.)))
        .child(
            img(ImageSource::Custom(Arc::new(move |_: &mut Window, _: &mut App| Some(Ok(custom.clone())))))
                .size(px(32.))
                .image_cache(&m.cache),
        )
        .child(img(ImageSource::Image(Arc::new(Image::from_bytes(ImageFormat::Png, ELEM_PNG.to_vec())))).size(px(32.)))
        .child(img(ImageSource::Resource(Resource::Path(Arc::from(Path::new(ELEM_PNG_PATH))))).size(px(32.)))
        .child(div().image_cache(m.cache.clone()).child(Avatar::new().src(ImageSource::Render(m.pic.clone()))))
        .into_any_element()
}

fn elem_scope(m: &MotionPages) -> AnyElement {
    let scrolled = div()
        .id("scrolled")
        .h(px(60.))
        .w(px(200.))
        .overflow_y_scroll()
        .track_scroll(&m.scroll)
        .child(div().h(px(40.)).child(Label::new("above")))
        .child(div().id("anchored").anchor_scroll(Some(m.anchor.clone())).child(Label::new("anchored")));
    let tree = div()
        .id("chart")
        .a11y_synthetic_children(|b| {
            for (key, label) in [("q1", "Q1: 12"), ("q2", "Q2: 30")] {
                let id = b.synthetic_node_id(key);
                let mut node = accesskit::Node::new(Role::ListItem);
                node.set_label(label);
                b.push_child(id, node);
            }
        })
        .child(Label::new("chart"));
    let ordered = div()
        .relative()
        .h(px(30.))
        .with_dynamic_prepaint_order(|_, _| [1usize, 0].into_iter().collect())
        .child(div().absolute().size(px(30.)).bg(red()))
        .child(div().absolute().left(px(10.)).size(px(30.)).bg(blue()));
    v_flex()
        .gap_2()
        .child(div().child(Label::new("selectable in its scope")).text_selection_scope(m.scope))
        .child(scrolled)
        .child(tree)
        .child(ordered)
        .into_any_element()
}

fn elem_vlist(cx: &mut Cx) -> AnyElement {
    let sizes: Vec<Size<Pixels>> = (0..200).map(|i| size(px(60.), px(20. + (i % 3) as f32 * 8.))).collect();
    let sizes = Rc::new(sizes);
    let hs = Rc::new(sizes[..8].to_vec());
    let (vs, hz) = (sizes.clone(), hs.clone());
    v_flex()
        .gap_2()
        .child(
            v_virtual_list(cx.entity(), "v", sizes, move |_, range, _, _| {
                range.map(|ix| div().h(vs[ix].height).child(Label::new(format!("row {ix}")))).collect::<Vec<_>>()
            })
            .w(px(200.))
            .h(px(160.)),
        )
        .child(
            h_virtual_list(cx.entity(), "h", hs, move |_, range, _, _| {
                range.map(|ix| div().w(hz[ix].width).child(Label::new(format!("row {ix}")))).collect::<Vec<_>>()
            })
            .w(px(400.))
            .h(px(24.)),
        )
        .into_any_element()
}
