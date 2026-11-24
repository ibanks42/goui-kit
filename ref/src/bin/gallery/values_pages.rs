//! The values family's gallery pages (docs/PROTOCOLS.md sections 13.8 to 13.12), written the way a
//! Rust GPUI-kit app writes them: the theme's mode switched with `Theme::change`, the theme's
//! fields edited in place through `Theme::global_mut`, and bars placed by the plot scales
//! (`ScaleBand`, `ScaleLinear`). The Odin twin is examples/odin/gallery/values_pages.odin; the
//! pages' lines are in ref/gallery/pages.txt. Each theme page toggles with one button, clicked
//! twice by its script, so the theme is as before afterwards.
use gpui_kit::base::plot::scale::{Scale, ScaleBand, ScaleLinear};
use gpui_kit::component::button::{Button, ButtonVariants};
use gpui_kit::component::label::Label;
use gpui_kit::component::switch::Switch;
use gpui_kit::component::{ActiveTheme, Theme, ThemeMode};
use gpui_kit::base::{CalendarState, Date, ScrollbarHandle, Scrollbar};
use gpui_kit::component::calendar::Calendar;
use gpui_kit::{
    AnyElement, App, AppContext, Entity, InteractiveElement, IntoElement, ParentElement, Pixels, ScrollHandle,
    SharedString, StatefulInteractiveElement, Styled, Window, div, point, px,
};
use std::cell::Cell;

thread_local! {
    /// The theme's font size and radius before the edit page changed them.
    static ORIGINAL: Cell<Option<(Pixels, Pixels)>> = const { Cell::new(None) };
}

fn sample() -> impl IntoElement {
    div()
        .flex()
        .flex_col()
        .gap(px(12.))
        .child(Button::new("primary").primary().label("Primary"))
        .child(Button::new("outline").outline().label("Outline"))
        .child(Label::new("The theme of the whole window"))
        .child(Switch::new("sw").checked(true).label("Switch"))
}

pub fn page(id: &str, cx: &App) -> Option<AnyElement> {
    Some(match id {
        "values-theme-mode" => div()
            .flex()
            .flex_col()
            .gap(px(16.))
            .child(Button::new("toggle").label("Toggle").on_click(|_, window, cx| {
                let dark = cx.theme().is_dark();
                Theme::change(if dark { ThemeMode::Light } else { ThemeMode::Dark }, Some(window), cx);
            }))
            .child(sample())
            .into_any_element(),
        "values-theme-edit" => {
            if ORIGINAL.with(|o| o.get()).is_none() {
                let t = cx.theme();
                ORIGINAL.with(|o| o.set(Some((t.font_size, t.radius))));
            }
            div()
                .flex()
                .flex_col()
                .gap(px(16.))
                .child(Button::new("toggle").label("Toggle").on_click(|_, _, cx| {
                    let (font_size, radius) = ORIGINAL.with(|o| o.get()).unwrap_or((px(16.), px(6.)));
                    let t = Theme::global_mut(cx);
                    if t.radius == radius {
                        t.radius = px(0.);
                        t.font_size = px(20.);
                    } else {
                        t.radius = radius;
                        t.font_size = font_size;
                    }
                    cx.refresh_windows();
                }))
                .child(sample())
                .into_any_element()
        }
        "values-scales" => {
            let names = ["a", "b", "c", "d"];
            let vals = [3.0f64, 7., 5., 9.];
            let band = ScaleBand::new(names.iter().map(|n| SharedString::from(*n)), [0., 400.]).padding_inner(0.2);
            let linear = ScaleLinear::new(vec![0.0f64, 10.], [0., 200.]);
            let w = band.band_width();
            div()
                .relative()
                .w(px(400.))
                .h(px(200.))
                .bg(cx.theme().muted)
                .children(names.iter().zip(vals).map(|(n, v)| {
                    let x = band.tick(&SharedString::from(*n)).unwrap_or(0.);
                    let h = linear.tick(&v).unwrap_or(0.);
                    div().absolute().left(px(x)).bottom(px(0.)).w(px(w)).h(px(h)).bg(cx.theme().primary)
                }))
                .into_any_element()
        }
        // docs/PROTOCOLS.md 13.19, 13.20
        "values-settings" => settings_page().into_any_element(),
        "values-drag" => {
            let preview = PREVIEW.with(|p| p.borrow().clone()).expect("the preview view (ValuesPages::new)");
            let accent = cx.theme().accent;
            let src = div()
                .id("src")
                .w(px(120.))
                .h(px(40.))
                .bg(cx.theme().muted)
                .on_drag(DragValue { id: 7, text: "row 7".into() }, move |_, _, _, _| preview.clone())
                .child(Label::new("drag me"));
            let dst = div()
                .id("dst")
                .w(px(200.))
                .h(px(120.))
                .bg(cx.theme().secondary)
                .drag_over::<DragValue>(move |s, _, _, _| s.bg(accent))
                .on_drop(|d: &DragValue, _, cx| {
                    DROPPED.with(|x| *x.borrow_mut() = SharedString::from(format!("dropped {}: {}", d.id, d.text)));
                    cx.refresh_windows();
                })
                .child(Label::new(DROPPED.with(|x| x.borrow().clone())));
            div().flex().gap(px(80.)).child(src).child(dst).into_any_element()
        }
        "values-shadow" => div()
            .p(px(24.))
            .child(
                div()
                    .w(px(160.))
                    .h(px(80.))
                    .bg(cx.theme().background)
                    .shadow(vec![gpui_kit::base::box_shadow(px(4.), px(6.), px(12.), px(2.), gpui_kit::hsla(0.6, 0.8, 0.3, 0.6))]),
            )
            .into_any_element(),
        "values-resizable" => div()
            .w(px(600.))
            .h(px(200.))
            .child(
                gpui_kit::component::h_resizable("rz")
                    .child(
                        gpui_kit::base::resizable_panel()
                            .size(px(200.))
                            .child(div().size_full().bg(cx.theme().muted).child(Label::new("left"))),
                    )
                    .child(gpui_kit::base::resizable_panel().child(div().size_full().child(Label::new("right")))),
            )
            .into_any_element(),
        "values-scrollbar-div" => {
            use gpui_kit::component::scroll::ScrollableElement;
            div()
                .w(px(300.))
                .h(px(160.))
                .children((0..20).map(|i| div().h(px(24.)).child(SharedString::from(format!("item {i}")))))
                .overflow_y_scrollbar()
                .id("sd")
                .into_any_element()
        }
        "values-toolbar" => {
            use gpui_kit::component::toolbar::Toolbar;
            Toolbar::new("tb")
                .child(Button::new("b1").label("Bold"))
                .child(Button::new("b2").label("Italic"))
                .children([Button::new("b3").label("Code"), Button::new("b4").label("Wrap")])
                .into_any_element()
        }
        id if id.starts_with("comp6-") => return comp6_page(id, cx),
        _ => return None,
    })
}

// --- docs/PROTOCOLS.md 13.19, 13.20: the settings page over values a Rust app keeps,
// drag and drop of a value.

thread_local! {
    static DARK: Cell<bool> = const { Cell::new(false) };
    static COMPACT: Cell<bool> = const { Cell::new(true) };
    static NAME: std::cell::RefCell<SharedString> = std::cell::RefCell::new(SharedString::from("Ada"));
    static SIZE: std::cell::RefCell<SharedString> = std::cell::RefCell::new(SharedString::from("m"));
    static ZOOM: Cell<f64> = const { Cell::new(1.5) };
    static DROPPED: std::cell::RefCell<SharedString> = std::cell::RefCell::new(SharedString::from("nothing dropped"));
    static PREVIEW: std::cell::RefCell<Option<Entity<Preview>>> = const { std::cell::RefCell::new(None) };
}

fn settings_page() -> impl IntoElement {
    use gpui_kit::component::setting::{NumberFieldOptions, SettingField, SettingGroup, SettingItem, SettingPage, Settings};
    let look = SettingGroup::new()
        .title("Look")
        .item(SettingItem::new(
            "Dark",
            SettingField::switch(|_| DARK.with(|v| v.get()), |v, cx| { DARK.with(|d| d.set(v)); cx.refresh_windows(); })
                .default_value(false),
        ))
        .item(SettingItem::new(
            "Compact",
            SettingField::checkbox(|_| COMPACT.with(|v| v.get()), |v, cx| { COMPACT.with(|d| d.set(v)); cx.refresh_windows(); }),
        ))
        .item(SettingItem::new(
            "Name",
            SettingField::input(|_| NAME.with(|v| v.borrow().clone()), |v, cx| { NAME.with(|d| *d.borrow_mut() = v); cx.refresh_windows(); }),
        ))
        .item(SettingItem::new(
            "Size",
            SettingField::dropdown(
                vec![("s".into(), "Small".into()), ("m".into(), "Medium".into())],
                |_| SIZE.with(|v| v.borrow().clone()),
                |v, cx| { SIZE.with(|d| *d.borrow_mut() = v); cx.refresh_windows(); },
            ),
        ))
        .item(SettingItem::new(
            "Zoom",
            SettingField::number_input(
                NumberFieldOptions { min: 0., max: 4., step: 0.5 },
                |_| ZOOM.with(|v| v.get()),
                |v, cx| { ZOOM.with(|d| d.set(v)); cx.refresh_windows(); },
            ),
        ))
        .item(SettingItem::render(|_, _, _| div().child(Label::new("Rendered by the app"))));
    div().w(px(760.)).h(px(480.)).child(
        Settings::new("settings").sidebar_size_range(px(100.)..px(300.)).page(SettingPage::new("General").group(look)),
    )
}

/// The value a drag carries, as the Odin app's gk.Drag_Value.
#[derive(Clone)]
struct DragValue {
    id: u64,
    text: SharedString,
}

/// The drag's preview, a view as the Odin app's.
struct Preview;
impl gpui_kit::Render for Preview {
    fn render(&mut self, _: &mut Window, cx: &mut gpui_kit::Context<Self>) -> impl IntoElement {
        div().w(px(80.)).h(px(24.)).bg(cx.theme().primary)
    }
}

/// PROTOCOLS.md 13.13, 13.14: a calendar whose state was given a date (jiff's civil::Date),
/// and a list scrolled from code through a ScrollHandle with a scrollbar over it.
pub struct ValuesPages {
    pub cal: Entity<CalendarState>,
    pub scroll: ScrollHandle,
    pub c6: Comp6Pages, // the components family
}

impl ValuesPages {
    pub fn new(window: &mut Window, cx: &mut App) -> Self {
        let cal = cx.new(|cx| CalendarState::new(window, cx));
        let date = jiff::civil::date(2024, 2, 29);
        cal.update(cx, |s, cx| s.set_date(Date::Single(Some(date)), window, cx));
        let scroll = ScrollHandle::new();
        ScrollbarHandle::set_offset(&scroll, point(px(0.), px(-120.)));
        let preview = cx.new(|_| Preview);
        PREVIEW.with(|p| *p.borrow_mut() = Some(preview));
        let c6 = Comp6Pages::new(window, cx);
        ValuesPages { cal, scroll, c6 }
    }
}

pub fn page_state(id: &str, g: &crate::Gallery) -> Option<AnyElement> {
    let v = &g.values;
    Some(match id {
        "calendar-date" => div().w(px(320.)).child(Calendar::new(&v.cal)).into_any_element(),
        id if id.starts_with("comp6-") => return comp6_page_state(id, &v.c6),
        "values-scroll" => div()
            .relative()
            .w(px(300.))
            .h(px(160.))
            .child(
                div()
                    .id("list")
                    .size_full()
                    .overflow_y_scroll()
                    .track_scroll(&v.scroll)
                    .children((0..30).map(|i| div().h(px(24.)).child(SharedString::from(format!("row {i}"))))),
            )
            .child(Scrollbar::vertical(&v.scroll))
            .into_any_element(),
        _ => return None,
    })
}

// --- the components family, docs/PROTOCOLS.md 11.17 to 11.24, written the way a Rust
// GPUI-kit app writes them; the Odin twins are at the end of examples/odin/gallery/values_pages.odin:
// tooltips the kit's Tooltip describes, a resizable group whose divider the app draws beside one the
// kit draws, labels in Markdown (Text::TextView), calendars with a disabled range and a set of
// disabled days, and a table's and a tree's context menus.

use crate::overlay_pages::{MenuZoomIn, MenuZoomOut};
use gpui_kit::component::list::ListItem;
use gpui_kit::component::menu::PopupMenu;
use gpui_kit::component::table::{Column, DataTable, TableDelegate, TableState};
use gpui_kit::component::tree::{TreeItem, TreeState, tree};

pub struct Comp6Pages {
    cal_range: Entity<CalendarState>,
    cal_days: Entity<CalendarState>,
    table: Entity<TableState<MenuTable>>,
    tree: Entity<TreeState>,
}

impl Comp6Pages {
    fn new(window: &mut Window, cx: &mut App) -> Self {
        let d = |y, m, d| jiff::civil::date(y, m, d);
        let day = d(2024, 2, 29);
        let cal_range = cx.new(|cx| {
            CalendarState::new(window, cx).disabled_matcher(gpui_kit::base::Matcher::range(Some(d(2024, 2, 12)), Some(d(2024, 2, 16))))
        });
        cal_range.update(cx, |s, cx| s.set_date(Date::Single(Some(day)), window, cx));
        let cal_days = cx.new(|cx| CalendarState::new(window, cx));
        let days = [d(2024, 2, 5), d(2024, 2, 20)];
        cal_days.update(cx, |s, cx| {
            s.set_disabled_matcher(gpui_kit::base::Matcher::custom(move |x| days.contains(x)), window, cx);
            s.set_date(Date::Single(Some(day)), window, cx);
        });
        let table = cx.new(|cx| TableState::new(MenuTable::new(), window, cx));
        let tree = cx.new(|cx| {
            TreeState::new(cx).items(vec![TreeItem::new("a", "alpha"), TreeItem::new("b", "beta"), TreeItem::new("c", "gamma")])
        });
        Comp6Pages { cal_range, cal_days, table, tree }
    }
}

/// A table whose rows each open a menu on a right-click.
pub struct MenuTable {
    columns: Vec<Column>,
}

impl MenuTable {
    fn new() -> Self {
        MenuTable { columns: vec![Column::new("id", "Id").width(px(60.)), Column::new("name", "Name").width(px(100.))] }
    }
}

impl TableDelegate for MenuTable {
    fn columns_count(&self, _: &App) -> usize {
        self.columns.len()
    }
    fn rows_count(&self, _: &App) -> usize {
        4
    }
    fn column(&self, col_ix: usize, _: &App) -> Column {
        self.columns[col_ix].clone()
    }
    fn render_td(
        &mut self,
        row_ix: usize,
        col_ix: usize,
        _: &mut Window,
        _: &mut gpui_kit::Context<TableState<Self>>,
    ) -> impl IntoElement {
        Label::new(format!("{row_ix}:{col_ix}"))
    }
    fn context_menu(
        &mut self,
        row_ix: usize,
        menu: PopupMenu,
        _: &mut Window,
        _: &mut gpui_kit::Context<TableState<Self>>,
    ) -> PopupMenu {
        menu.menu(format!("Open row {row_ix}"), Box::new(MenuZoomIn)).menu(format!("Delete row {row_ix}"), Box::new(MenuZoomOut))
    }
}

fn comp6_page(id: &str, _cx: &App) -> Option<AnyElement> {
    use gpui_kit::component::tooltip::Tooltip;
    Some(match id {
        "comp6-tooltip" => div()
            .id("c6tip")
            .w(px(160.))
            .h(px(40.))
            .bg(_cx.theme().muted)
            .tooltip(|window, cx| {
                Tooltip::new("Save the file")
                    .key_binding(Some(gpui_kit::component::kbd::Kbd::new(gpui_kit::Keystroke::parse("ctrl-s").unwrap())))
                    .build(window, cx)
            })
            .child(Label::new("Hover me"))
            .into_any_element(),
        "comp6-tooltip-element" => div()
            .id("c6tipe")
            .w(px(160.))
            .h(px(40.))
            .bg(_cx.theme().muted)
            .tooltip(|window, cx| {
                Tooltip::element(|_, _| gpui_kit::component::h_flex().gap_1().child(Label::new("Zoom")).child(Label::new("in")))
                    .action(&MenuZoomIn, None)
                    .build(window, cx)
            })
            .child(Label::new("Hover me"))
            .into_any_element(),
        "comp6-resize" => {
            let group = |id: &'static str, r: gpui_kit::base::ResizeHandleRenderer| {
                div().w(px(600.)).h(px(100.)).child(
                    gpui_kit::base::h_resizable(id)
                        .with_handle_appearance(r)
                        .child(gpui_kit::base::resizable_panel().size(px(200.)).child(Label::new("left")))
                        .child(gpui_kit::base::resizable_panel().child(Label::new("right"))),
                )
            };
            let app_drawn: gpui_kit::base::ResizeHandleRenderer =
                std::rc::Rc::new(|ctx: &gpui_kit::base::ResizeHandleContext, _: &mut Window, cx: &mut App| {
                let color = if ctx.is_active() { cx.theme().primary } else { cx.theme().border };
                Some(div().h_full().w(px(3.)).bg(color).into_any_element())
            });
            // A handle of its own (ResizeHandle::with_appearance) at the left edge of a band.
            let alone = div().relative().w(px(600.)).h(px(40.)).bg(_cx.theme().muted).child(
                gpui_kit::base::resize_handle::<u32, gpui_kit::EmptyView>("c6h", gpui_kit::Axis::Horizontal)
                    .with_appearance(app_drawn.clone()),
            );
            gpui_kit::component::v_flex()
                .gap_4()
                .child(group("c6rz", app_drawn))
                .child(group("c6rk", gpui_kit::component::resize_handle_appearance()))
                .child(alone)
                .into_any_element()
        }
        "comp6-text-view" => {
            use gpui_kit::component::text::{Text, TextView};
            gpui_kit::component::v_flex()
                .gap_3()
                .w(px(400.))
                .child(
                    gpui_kit::component::checkbox::Checkbox::new("c6cb")
                        .checked(true)
                        .label(Text::TextView(Box::new(TextView::markdown("c6md1", "A **bold** label")))),
                )
                .child(gpui_kit::component::alert::Alert::new(
                    "c6al",
                    Text::TextView(Box::new(TextView::markdown("c6md2", "An alert in *Markdown*"))),
                ))
                .into_any_element()
        }
        _ => return None,
    })
}

fn comp6_page_state(id: &str, p: &Comp6Pages) -> Option<AnyElement> {
    Some(match id {
        "comp6-calendar-matcher" => div()
            .flex()
            .gap_4()
            .child(div().w(px(320.)).child(Calendar::new(&p.cal_range)))
            .child(div().w(px(320.)).child(Calendar::new(&p.cal_days)))
            .into_any_element(),
        "comp6-table-menu" => div()
            .w(px(300.))
            .h(px(200.))
            .child(DataTable::new(&p.table).scrollbar_visible(false, false))
            .into_any_element(),
        "comp6-tree-menu" => div()
            .w(px(300.))
            .h(px(160.))
            .child(
                tree(&p.tree, |ix, entry, _, _, _| ListItem::new(("c6tree", ix)).child(Label::new(entry.item().label.clone())))
                    .context_menu(|_, entry, menu, _, _| menu.menu(format!("Rename {}", entry.item().label), Box::new(MenuZoomIn))),
            )
            .into_any_element(),
        _ => return None,
    })
}
