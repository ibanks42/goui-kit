//! The gallery's pages, written the way a Rust GPUI-kit app writes them. One function per
//! line of ref/gallery/pages.txt; the Odin twin (examples/odin/gallery/pages.odin) has the
//! same pages in the same order, call for call. Change a page in both files in one commit.
//!
//! Rules that keep a page comparable: fixed sizes and fixed text, nothing animated (no
//! loading spinners), nothing read from the clock or the environment.
use crate::Gallery;
use gpui_kit::component::alert::Alert;
use gpui_kit::component::avatar::{Avatar, AvatarGroup};
use gpui_kit::component::badge::Badge;
use gpui_kit::component::breadcrumb::Breadcrumb;
use gpui_kit::component::button::{Button, ButtonVariants};
use gpui_kit::component::checkbox::Checkbox;
use gpui_kit::component::description_list::DescriptionList;
use gpui_kit::component::group_box::{GroupBox, GroupBoxVariants};
use gpui_kit::component::input::Input;
use gpui_kit::component::kbd::Kbd;
use gpui_kit::component::label::Label;
use gpui_kit::component::link::Link;
use gpui_kit::component::progress::{Progress, ProgressCircle};
use gpui_kit::component::radio::{Radio, RadioGroup};
use gpui_kit::component::rating::Rating;
use gpui_kit::component::separator::Separator;
use gpui_kit::component::skeleton::Skeleton;
use gpui_kit::component::switch::Switch;
use gpui_kit::component::tag::Tag;
use gpui_kit::component::{
    ActiveTheme, ColorName, Disableable, Selectable, Sizable, h_flex, v_flex,
};
use gpui_kit::{
    AnyElement, Context, InteractiveElement, IntoElement, Keystroke, ParentElement, SharedString,
    Styled, Window, div, hsla, px, uniform_list,
};

type Cx<'a> = Context<'a, Gallery>;

/// The element of page `id`. Every id of pages.txt has an arm; an unknown id is a mistake in
/// that file and stops the reference.
pub fn page(id: &str, g: &Gallery, _window: &mut Window, cx: &mut Cx) -> AnyElement {
    match id {
        "label-basic" => label_basic(cx),
        "div-style" => div_style(cx),
        "div-hover" => div_hover(),
        "button-primary" => button_primary(),
        "button-click" => button_click(g, cx),
        "input" => input(g, cx),
        "input-cleanable" => input_cleanable(g),
        "list" => list(g, cx),
        "switch-bare" => switch_bare(),
        "switch-bare-click" => switch_bare_click(g, cx),
        "button-variants" => button_variants(),
        "button-outline" => button_outline(),
        "button-sizes" => button_sizes(),
        "button-states" => button_states(),
        "alert-variants" => alert_variants(),
        "alert-shapes" => alert_shapes(),
        "avatar" => avatar(),
        "badge" => badge(),
        "checkbox" => checkbox(),
        "checkbox-click" => checkbox_click(g, cx),
        "progress" => progress(),
        "progress-circle" => progress_circle(),
        "separator" => separator(),
        "switch" => switch(),
        "switch-click" => switch_click(g, cx),
        "tag" => tag(),
        "label-more" => label_more(),
        "radio" => radio(),
        "rating" => rating(),
        "breadcrumb" => breadcrumb(),
        "group-box" => group_box(),
        "description-list" => description_list(),
        "kbd" => kbd(),
        "link" => link(),
        "skeleton" => skeleton(),
        _ => panic!("ref-gallery: pages.txt names page {id:?}, which has no Rust page"),
    }
}

fn label_basic(cx: &mut Cx) -> AnyElement {
    v_flex()
        .gap_2()
        .child(Label::new("A plain label"))
        .child(Label::new("An extra small label").text_xs())
        .child(Label::new("A muted label").text_color(cx.theme().muted_foreground))
        .into_any_element()
}

fn div_style(cx: &mut Cx) -> AnyElement {
    h_flex()
        .gap_3()
        .flex_wrap()
        .child(
            div()
                .w(px(120.))
                .h(px(60.))
                .bg(hsla(0.6, 0.7, 0.5, 1.))
                .rounded_sm()
                .p_2()
                .child(SharedString::from("solid")),
        )
        .child(
            div()
                .w(px(120.))
                .h(px(60.))
                .bg(cx.theme().border)
                .p_1()
                .text_xs()
                .child(SharedString::from("theme border")),
        )
        .child(
            v_flex()
                .w(px(200.))
                .gap_1()
                .child(div().w_full().h(px(12.)).bg(hsla(0., 0.8, 0.5, 1.)))
                .child(div().w_full().h(px(12.)).bg(hsla(0.33, 0.8, 0.4, 1.)))
                .child(div().w(px(100.)).h(px(12.)).bg(hsla(0.12, 0.9, 0.5, 0.5))),
        )
        .into_any_element()
}

fn div_hover() -> AnyElement {
    h_flex()
        .gap_3()
        .child(
            div()
                .w(px(160.))
                .h(px(60.))
                .p_2()
                .bg(hsla(0.6, 0.5, 0.5, 1.))
                .hover(|style| style.bg(hsla(0.05, 0.9, 0.5, 1.)))
                .child(SharedString::from("hover me")),
        )
        .child(
            div()
                .w(px(160.))
                .h(px(60.))
                .p_2()
                .bg(hsla(0.6, 0.5, 0.5, 1.))
                .hover(|style| style.bg(hsla(0.05, 0.9, 0.5, 1.)))
                .child(SharedString::from("not hovered")),
        )
        .into_any_element()
}

fn button_primary() -> AnyElement {
    h_flex()
        .gap_2()
        .child(Button::new("default").label("Default"))
        .child(Button::new("primary").primary().label("Primary"))
        .into_any_element()
}

fn button_click(g: &Gallery, cx: &mut Cx) -> AnyElement {
    h_flex()
        .gap_2()
        .items_center()
        .child(
            Button::new("inc")
                .primary()
                .label("Increment")
                .on_click(cx.listener(|this, _, _, cx| {
                    this.count += 1;
                    cx.notify();
                })),
        )
        .child(Label::new(format!("clicked {}", g.count)))
        .into_any_element()
}

fn input(g: &Gallery, cx: &mut Cx) -> AnyElement {
    v_flex()
        .gap_2()
        .child(div().w(px(240.)).child(Input::new(&g.name)))
        .child(Label::new(format!("hello {}", g.name.read(cx).value())))
        .into_any_element()
}

fn input_cleanable(g: &Gallery) -> AnyElement {
    div()
        .w(px(240.))
        .child(Input::new(&g.name).cleanable(true))
        .into_any_element()
}

fn list(g: &Gallery, cx: &mut Cx) -> AnyElement {
    let _ = g;
    div()
        .w_full()
        .h(px(160.))
        .child(
            uniform_list(
                "rows",
                crate::ROWS,
                cx.processor(|this, range: std::ops::Range<usize>, _, _| {
                    range
                        .map(|i| div().h(px(24.)).px_2().child(this.rows[i].clone()))
                        .collect::<Vec<_>>()
                }),
            )
            .size_full(),
        )
        .into_any_element()
}

fn switch_bare() -> AnyElement {
    v_flex()
        .gap_2()
        .child(Switch::new("off"))
        .child(Switch::new("on").checked(true))
        .into_any_element()
}

fn switch_bare_click(g: &Gallery, cx: &mut Cx) -> AnyElement {
    Switch::new("toggle")
        .checked(g.flag)
        .on_click(cx.listener(|this, checked: &bool, _, cx| {
            this.flag = *checked;
            cx.notify();
        }))
        .into_any_element()
}

fn button_variants() -> AnyElement {
    h_flex()
        .gap_2()
        .flex_wrap()
        .child(Button::new("secondary").secondary().label("Secondary"))
        .child(Button::new("danger").danger().label("Danger"))
        .child(Button::new("warning").warning().label("Warning"))
        .child(Button::new("success").success().label("Success"))
        .child(Button::new("info").info().label("Info"))
        .child(Button::new("ghost").ghost().label("Ghost"))
        .child(Button::new("link").link().label("Link"))
        .child(Button::new("text").text().label("Text"))
        .into_any_element()
}

fn button_outline() -> AnyElement {
    h_flex()
        .gap_2()
        .flex_wrap()
        .child(Button::new("default").outline().label("Default"))
        .child(Button::new("primary").primary().outline().label("Primary"))
        .child(Button::new("danger").danger().outline().label("Danger"))
        .child(Button::new("success").success().outline().label("Success"))
        .into_any_element()
}

fn button_sizes() -> AnyElement {
    h_flex()
        .gap_2()
        .items_center()
        .child(Button::new("xsmall").xsmall().label("XSmall"))
        .child(Button::new("small").small().label("Small"))
        .child(Button::new("medium").label("Medium"))
        .child(Button::new("large").large().label("Large"))
        .into_any_element()
}

fn button_states() -> AnyElement {
    h_flex()
        .gap_2()
        .flex_wrap()
        .child(Button::new("disabled").disabled(true).label("Disabled"))
        .child(Button::new("selected").selected(true).label("Selected"))
        .child(Button::new("compact").compact().label("Compact"))
        .child(Button::new("idle").loading(false).label("Not loading"))
        .child(Button::new("caret").dropdown_caret(true).label("Caret"))
        .into_any_element()
}

fn alert_variants() -> AnyElement {
    v_flex()
        .gap_2()
        .child(Alert::new("default", "A default alert."))
        .child(Alert::info("info", "An info alert.").title("Info"))
        .child(Alert::success("success", "A success alert.").title("Success"))
        .child(Alert::warning("warning", "A warning alert.").title("Warning"))
        .child(Alert::error("error", "An error alert.").title("Error"))
        .into_any_element()
}

fn alert_shapes() -> AnyElement {
    v_flex()
        .gap_2()
        .child(Alert::info("banner", "A banner alert.").banner())
        .child(Alert::info("small", "A small alert.").small())
        .child(Alert::info("large", "A large alert.").title("Large").large())
        .child(Alert::info("hidden", "A hidden alert.").visible(false))
        .into_any_element()
}

fn avatar() -> AnyElement {
    v_flex()
        .gap_3()
        .child(
            h_flex()
                .gap_2()
                .items_center()
                .child(Avatar::new().name("Ada Lovelace").xsmall())
                .child(Avatar::new().name("Ada Lovelace").small())
                .child(Avatar::new().name("Ada Lovelace"))
                .child(Avatar::new().name("Ada Lovelace").large())
                .child(Avatar::new()),
        )
        .child(
            AvatarGroup::new()
                .child(Avatar::new().name("Ada Lovelace"))
                .child(Avatar::new().name("Grace Hopper"))
                .child(Avatar::new().name("Alan Turing"))
                .child(Avatar::new().name("Edsger Dijkstra"))
                .limit(3)
                .ellipsis(),
        )
        .into_any_element()
}

fn badge() -> AnyElement {
    h_flex()
        .gap_3()
        .items_center()
        .child(Badge::new().count(3).child(Avatar::new().name("Ada Lovelace")))
        .child(Badge::new().dot().child(Avatar::new().name("Grace Hopper")))
        .child(Badge::new().count(120).max(99).child(Avatar::new().name("Alan Turing")))
        .into_any_element()
}

fn checkbox() -> AnyElement {
    v_flex()
        .gap_2()
        .child(Checkbox::new("off").label("Unchecked"))
        .child(Checkbox::new("on").label("Checked").checked(true))
        .child(Checkbox::new("off-disabled").label("Disabled").disabled(true))
        .child(Checkbox::new("on-disabled").label("Disabled, checked").checked(true).disabled(true))
        .child(Checkbox::new("small").label("Small").checked(true).small())
        .child(Checkbox::new("large").label("Large").checked(true).large())
        .into_any_element()
}

fn checkbox_click(g: &Gallery, cx: &mut Cx) -> AnyElement {
    Checkbox::new("toggle")
        .label("Toggle me")
        .checked(g.flag)
        .on_click(cx.listener(|this, checked: &bool, _, cx| {
            this.flag = *checked;
            cx.notify();
        }))
        .into_any_element()
}

fn progress() -> AnyElement {
    v_flex()
        .w(px(320.))
        .gap_3()
        .child(Progress::new("p0").value(0.))
        .child(Progress::new("p25").value(25.))
        .child(Progress::new("p60").value(60.))
        .child(Progress::new("p100").value(100.))
        .child(Progress::new("small").value(40.).small())
        .child(Progress::new("large").value(40.).large())
        .into_any_element()
}

fn progress_circle() -> AnyElement {
    h_flex()
        .gap_3()
        .items_center()
        .child(ProgressCircle::new("c0").value(0.))
        .child(ProgressCircle::new("c40").value(40.))
        .child(ProgressCircle::new("c100").value(100.))
        .child(ProgressCircle::new("small").value(40.).small())
        .child(ProgressCircle::new("large").value(40.).large())
        .into_any_element()
}

fn separator() -> AnyElement {
    v_flex()
        .w(px(320.))
        .gap_3()
        .child(Separator::horizontal())
        .child(Separator::horizontal().label("Label"))
        .child(Separator::horizontal_dashed())
        .child(
            h_flex()
                .h(px(40.))
                .gap_3()
                .child(SharedString::from("left"))
                .child(Separator::vertical())
                .child(SharedString::from("middle"))
                .child(Separator::vertical_dashed())
                .child(SharedString::from("right")),
        )
        .into_any_element()
}

fn switch() -> AnyElement {
    v_flex()
        .gap_2()
        .child(Switch::new("off").label("Off"))
        .child(Switch::new("on").label("On").checked(true))
        .child(Switch::new("off-disabled").label("Disabled").disabled(true))
        .child(Switch::new("on-disabled").label("Disabled, on").checked(true).disabled(true))
        .child(Switch::new("small").label("Small").checked(true).small())
        .into_any_element()
}

fn switch_click(g: &Gallery, cx: &mut Cx) -> AnyElement {
    Switch::new("toggle")
        .label("Toggle me")
        .checked(g.flag)
        .on_click(cx.listener(|this, checked: &bool, _, cx| {
            this.flag = *checked;
            cx.notify();
        }))
        .into_any_element()
}

fn tag() -> AnyElement {
    v_flex()
        .gap_2()
        .child(
            h_flex()
                .gap_2()
                .child(Tag::new().child("Default"))
                .child(Tag::primary().child("Primary"))
                .child(Tag::secondary().child("Secondary"))
                .child(Tag::danger().child("Danger"))
                .child(Tag::success().child("Success"))
                .child(Tag::warning().child("Warning"))
                .child(Tag::info().child("Info")),
        )
        .child(
            h_flex()
                .gap_2()
                .child(Tag::primary().outline().child("Outline"))
                .child(Tag::danger().rounded_full().child("Round"))
                .child(Tag::color(ColorName::Blue).child("Blue"))
                .child(Tag::success().small().child("Small")),
        )
        .into_any_element()
}

fn label_more() -> AnyElement {
    v_flex()
        .gap_2()
        .child(Label::new("Name").secondary("(optional)"))
        .child(Label::new("secret").masked(true))
        .child(Label::new("Highlighted label").highlights("light"))
        .into_any_element()
}

fn radio() -> AnyElement {
    h_flex()
        .gap_3()
        .child(
            v_flex()
                .gap_2()
                .child(Radio::new("off").label("Off"))
                .child(Radio::new("on").label("On").checked(true))
                .child(Radio::new("disabled").label("Disabled").disabled(true)),
        )
        .child(
            RadioGroup::vertical("group")
                .child("One")
                .child("Two")
                .child("Three")
                .selected_index(Some(1)),
        )
        .into_any_element()
}

fn rating() -> AnyElement {
    v_flex()
        .gap_2()
        .child(Rating::new("three").value(3).max(5))
        .child(Rating::new("none").value(0).max(5))
        .child(Rating::new("disabled").value(4).max(5).disabled(true))
        .into_any_element()
}

fn breadcrumb() -> AnyElement {
    Breadcrumb::new()
        .child("Home")
        .child("Library")
        .child("Data")
        .into_any_element()
}

fn group_box() -> AnyElement {
    v_flex()
        .w(px(360.))
        .gap_3()
        .child(GroupBox::new().title("Normal").child(Label::new("Content of the box")))
        .child(GroupBox::new().fill().title("Fill").child(Label::new("Content of the box")))
        .child(GroupBox::new().outline().title("Outline").child(Label::new("Content of the box")))
        .into_any_element()
}

fn description_list() -> AnyElement {
    div()
        .w(px(480.))
        .child(
            DescriptionList::new()
                .columns(2)
                .bordered(true)
                .item("Name", "Ada Lovelace", 1)
                .item("Born", "1815", 1)
                .item("Known for", "The first published program", 2),
        )
        .into_any_element()
}

fn kbd() -> AnyElement {
    h_flex()
        .gap_2()
        .child(Kbd::new(Keystroke::parse("ctrl-shift-p").expect("keystroke")))
        .child(Kbd::new(Keystroke::parse("enter").expect("keystroke")))
        .child(Kbd::new(Keystroke::parse("alt-k").expect("keystroke")).outline())
        .into_any_element()
}

fn link() -> AnyElement {
    v_flex()
        .gap_2()
        .child(Link::new("plain").href("https://example.com").child("A link"))
        .child(Link::new("disabled").href("https://example.com").disabled(true).child("A disabled link"))
        .into_any_element()
}

fn skeleton() -> AnyElement {
    v_flex()
        .w(px(240.))
        .gap_2()
        .child(Skeleton::new().h(px(16.)))
        .child(Skeleton::new().secondary().h(px(16.)))
        .into_any_element()
}
