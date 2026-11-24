//! ref-tutorial: the Rust reference for the tutorial's last step (docs/tutorial-go/09-background,
//! docs/GETTING-STARTED-GO.md), written the way a Rust GPUI-kit app is written, call for call with
//! the Go app: a shopping list with a list of custom rows (a ListDelegate), an input, an edit dialog
//! opened with a closure, shortcuts and a menu, a light and a dark theme, and prices looked up on a
//! thread that posts the answer to the UI thread.
//!
//!   ref-tutorial                  a real window
//!   ref-tutorial --script FILE    offscreen, driven by a gk-meter script (diag build)
use futures::channel::oneshot;
use gpui_kit::base::{h_flex, v_flex};
use gpui_kit::component::button::{Button, ButtonVariants};
use gpui_kit::component::checkbox::Checkbox;
use gpui_kit::component::dialog::{DialogAction, DialogClose, DialogFooter};
use gpui_kit::component::input::{Input, InputEvent, InputState};
use gpui_kit::component::label::Label;
use gpui_kit::component::list::{List, ListDelegate, ListEvent, ListItem, ListState};
use gpui_kit::component::menu::{DropdownMenu, PopupMenu};
use gpui_kit::component::tag::Tag;
use gpui_kit::component::{ActiveTheme, IconName, IndexPath, Sizable, StyledExt, Theme, ThemeMode, WindowExt};
use gpui_kit::{
    Action, AnyWindowHandle, App, AppContext, Context, Entity, FocusHandle, InteractiveElement, IntoElement, KeyBinding,
    ParentElement, Render, ScrollStrategy, Styled, Subscription, WeakEntity, Window, div,
};
use std::cell::RefCell;
use std::rc::Rc;
use std::time::Duration;

gpui_kit::actions!(groceries, [NewItem, ClearBought, ToggleTheme]);

/// An app-wide listener for action A: it runs `f` on the app in its window once the key's dispatch
/// is over (as the Go app's gk.Shortcut does).
fn shortcut<A: Action>(cx: &mut App, app: WeakEntity<Groceries>, wh: AnyWindowHandle, f: fn(&mut Groceries, &mut Window, &mut Context<Groceries>)) {
    cx.on_action(move |_: &A, cx: &mut App| {
        let app = app.clone();
        cx.defer(move |cx| {
            let _ = wh.update(cx, |_, window, cx| {
                app.update(cx, |app, cx| {
                    f(app, window, cx);
                    cx.notify();
                })
            });
        });
    });
}

#[derive(Clone)]
struct Item {
    name: String,
    amount: i64,
    bought: bool,
    price: f64, // 0 until the price lookup has answered
}

type Shared = Rc<RefCell<Vec<Item>>>;

struct ItemRows {
    items: Shared,
    app: WeakEntity<Groceries>,
}

impl ListDelegate for ItemRows {
    type Item = ListItem;

    fn items_count(&self, _: usize, _: &App) -> usize {
        self.items.borrow().len()
    }

    fn render_item(&mut self, ix: IndexPath, _: &mut Window, cx: &mut Context<ListState<Self>>) -> Option<ListItem> {
        let row = ix.row;
        let it = self.items.borrow().get(row)?.clone();
        let muted = cx.theme().muted_foreground;
        let mut name = Label::new(it.name.clone());
        if it.bought {
            name = name.line_through().text_color(muted);
        }
        let price = if it.price != 0. { format!("{:.2}", it.price * it.amount as f64) } else { "…".into() };
        let (app, app2) = (self.app.clone(), self.app.clone());
        Some(
            ListItem::new(("item", row)).child(
                h_flex()
                    .w_full()
                    .gap_2()
                    .items_center()
                    .child(Checkbox::new(("bought", row)).checked(it.bought).on_click(move |bought, _, cx| {
                        cx.stop_propagation(); // the row under the checkbox is not clicked as well
                        let bought = *bought;
                        app.update(cx, |a, cx| {
                            a.items.borrow_mut()[row].bought = bought;
                            a.refresh(cx);
                        })
                        .ok();
                    }))
                    .child(div().flex_1().child(name))
                    .child(Tag::secondary().text_xs().child(format!("×{}", it.amount)))
                    .child(Label::new(price).text_sm().text_color(muted))
                    .child(Button::new(("delete", row)).ghost().xsmall().icon(IconName::Delete).on_click(move |_, _, cx| {
                        cx.stop_propagation();
                        app2.update(cx, |a, cx| {
                            a.items.borrow_mut().remove(row);
                            a.refresh(cx);
                        })
                        .ok();
                    })),
            ),
        )
    }

    fn set_selected_index(&mut self, _: Option<IndexPath>, _: &mut Window, _: &mut Context<ListState<Self>>) {}
}

struct Groceries {
    items: Shared,
    editing: usize, // the item the dialog edits
    edit_amount: i64,
    dark: bool,
    new_name: Entity<InputState>,
    edit_name: Entity<InputState>,
    list: Entity<ListState<ItemRows>>,
    focus: FocusHandle,
    _subs: Vec<Subscription>,
}

impl Groceries {
    fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let new_name = cx.new(|cx| InputState::new(window, cx).placeholder("Add an item and press Enter"));
        let edit_name = cx.new(|cx| InputState::new(window, cx));
        let items: Shared = Rc::new(RefCell::new(Vec::new()));
        let app = cx.weak_entity();
        let list = cx.new(|cx| ListState::new(ItemRows { items: items.clone(), app }, window, cx));
        let subs = vec![
            cx.subscribe_in(&new_name, window, |this, _, e: &InputEvent, window, cx| {
                if matches!(e, InputEvent::PressEnter { .. }) {
                    this.new_name_entered(window, cx);
                }
            }),
            cx.subscribe_in(&list, window, |this, _, e: &ListEvent, window, cx| {
                if let ListEvent::Confirm(ix) = e {
                    this.open_editor(ix.row, window, cx); // a click on a row, or Enter
                }
            }),
        ];
        cx.bind_keys([
            KeyBinding::new("ctrl-n", NewItem, None),
            KeyBinding::new("ctrl-shift-d", ClearBought, None),
            KeyBinding::new("ctrl-t", ToggleTheme, None),
        ]);
        let (me, wh) = (cx.weak_entity(), window.window_handle());
        shortcut::<NewItem>(cx, me.clone(), wh, |a, w, cx| a.new_name.update(cx, |s, cx| s.focus(w, cx)));
        shortcut::<ClearBought>(cx, me.clone(), wh, |a, _, cx| {
            a.items.borrow_mut().retain(|it| !it.bought);
            a.refresh(cx);
        });
        shortcut::<ToggleTheme>(cx, me, wh, Self::switch_theme);
        let focus = cx.focus_handle();
        window.focus(&focus, cx);
        let mut app = Groceries { items, editing: 0, edit_amount: 0, dark: false, new_name, edit_name, list, focus, _subs: subs };
        app.add("Milk", 2, false, window, cx);
        app.add("Bread", 1, false, window, cx);
        app.add("Apples", 6, false, window, cx);
        app
    }

    /// Every view and the list drawn again (the Go app's gk.Refresh).
    fn refresh(&mut self, cx: &mut Context<Self>) {
        self.list.update(cx, |_, cx| cx.notify());
        cx.notify();
    }

    fn add(&mut self, name: &str, amount: i64, announce: bool, window: &mut Window, cx: &mut Context<Self>) {
        self.items.borrow_mut().push(Item { name: name.into(), amount, bought: false, price: 0. });
        self.look_up_price(name.into(), announce, window, cx);
    }

    fn summary(&self) -> String {
        let (mut to_buy, mut waiting, mut total) = (0, 0, 0.);
        let items = self.items.borrow();
        for it in items.iter() {
            if it.price == 0. {
                waiting += 1;
            }
            if !it.bought {
                to_buy += 1;
                total += it.price * it.amount as f64;
            }
        }
        let mut text = format!("{} items, {to_buy} to buy, {total:.2} in all", items.len());
        if waiting > 0 {
            text += &format!(" ({waiting} prices still loading)");
        }
        text
    }

    fn toolbar(&self, cx: &mut Context<Self>) -> impl IntoElement {
        let (theme_label, icon) = if self.dark { ("Light theme", IconName::Sun) } else { ("Dark theme", IconName::Moon) };
        h_flex()
            .gap_2()
            .items_center()
            .child(Label::new("Groceries").text_xl().font_bold())
            .child(div().flex_1())
            .child(Button::new("menu").ghost().icon(IconName::Menu).dropdown_menu(move |m: PopupMenu, _, _| {
                m.menu("New item", Box::new(NewItem))
                    .menu("Remove bought items", Box::new(ClearBought))
                    .separator()
                    .menu(theme_label, Box::new(ToggleTheme))
            }))
            .child(
                Button::new("theme")
                    .ghost()
                    .icon(icon)
                    .tooltip("Toggle the theme")
                    .on_click(cx.listener(|this, _, window, cx| this.switch_theme(window, cx))),
            )
    }

    fn new_name_entered(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let name = self.new_name.read(cx).value().trim().to_string();
        if name.is_empty() {
            return;
        }
        self.add(&name, 1, true, window, cx);
        self.new_name.update(cx, |s, cx| s.set_value("", window, cx));
        let last = self.items.borrow().len() - 1;
        self.list.update(cx, |l, cx| {
            l.set_selected_index(Some(IndexPath::default().row(last)), window, cx);
            l.scroll_to_item(IndexPath::default().row(last), ScrollStrategy::Nearest, window, cx);
        });
        self.refresh(cx);
    }

    fn open_editor(&mut self, row: usize, window: &mut Window, cx: &mut Context<Self>) {
        self.editing = row;
        let it = self.items.borrow()[row].clone();
        self.edit_amount = it.amount;
        self.edit_name.update(cx, |s, cx| s.set_value(it.name.clone(), window, cx));
        let this = cx.weak_entity();
        let edit_name = self.edit_name.clone();
        window.open_dialog(cx, move |d, _, cx| {
            let Some(app) = this.upgrade() else { return d };
            let amount = app.read(cx).edit_amount;
            let by = |n: i64| {
                let this = this.clone();
                move |_: &gpui_kit::ClickEvent, window: &mut Window, cx: &mut App| {
                    this.update(cx, |a, _| a.edit_amount = (a.edit_amount + n).max(1)).ok();
                    window.refresh();
                }
            };
            let this = this.clone();
            d.title(Label::new("Edit item"))
                .child(
                    v_flex().gap_3().child(Input::new(&edit_name)).child(
                        h_flex()
                            .gap_2()
                            .items_center()
                            .child(Label::new("Amount"))
                            .child(Button::new("less").small().icon(IconName::Minus).on_click(by(-1)))
                            .child(Label::new(amount.to_string()))
                            .child(Button::new("more").small().icon(IconName::Plus).on_click(by(1))),
                    ),
                )
                .footer(
                    DialogFooter::new()
                        .child(DialogClose::new().trigger(|b| b.label("Cancel")))
                        .child(DialogAction::new().child(Button::new("save").primary().label("Save"))),
                )
                .on_ok(move |_, window, cx| this.update(cx, |a, cx| a.save_edit(window, cx)).unwrap_or(true))
        });
        self.edit_name.update(cx, |s, cx| s.focus(window, cx));
    }

    fn save_edit(&mut self, window: &mut Window, cx: &mut Context<Self>) -> bool {
        let name = self.edit_name.read(cx).value().trim().to_string();
        if name.is_empty() {
            return false; // keep the dialog open
        }
        let renamed = {
            let mut items = self.items.borrow_mut();
            let it = &mut items[self.editing];
            let renamed = name != it.name;
            if renamed {
                it.name = name.clone();
                it.price = 0.;
            }
            it.amount = self.edit_amount;
            renamed
        };
        if renamed {
            self.look_up_price(name, true, window, cx);
        }
        self.refresh(cx);
        true // close the dialog
    }

    fn switch_theme(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.dark = !self.dark;
        Theme::change(if self.dark { ThemeMode::Dark } else { ThemeMode::Light }, Some(window), cx);
        self.refresh(cx);
    }

    /// Asks a slow price service on a thread; the answer comes back on the UI thread.
    fn look_up_price(&mut self, name: String, announce: bool, window: &mut Window, cx: &mut Context<Self>) {
        let (tx, rx) = oneshot::channel();
        let n = name.len();
        std::thread::spawn(move || {
            std::thread::sleep(Duration::from_millis(300));
            let _ = tx.send(0.49 + n as f64 * 0.35);
        });
        let wh = window.window_handle();
        cx.spawn(async move |this, cx| {
            let Ok(price) = rx.await else { return };
            let _ = cx.update_window(wh, |_, window, cx| {
                this.update(cx, |a, cx| {
                    for it in a.items.borrow_mut().iter_mut() {
                        if it.name == name && it.price == 0. {
                            it.price = price;
                        }
                    }
                    if announce {
                        window.push_notification(format!("{name} costs {price:.2}"), cx);
                    }
                    a.refresh(cx);
                })
                .ok();
            });
        })
        .detach();
    }
}

impl Render for Groceries {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        v_flex()
            .size_full()
            .p_4()
            .gap_3()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .track_focus(&self.focus)
            .child(self.toolbar(cx))
            .child(Input::new(&self.new_name))
            .child(div().flex_1().min_h_0().child(List::new(&self.list)))
            .child(Label::new(self.summary()).text_sm().text_color(cx.theme().muted_foreground))
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<Groceries> {
    gk_meter::attach(window, cx);
    cx.new(|cx| Groceries::new(window, cx))
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(520., 560., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-tutorial: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application().with_assets(gpui_kit::assets::Assets).run(move |cx| {
        gpui_kit::init(cx);
        let options = gk_meter::window_options(520., 560., cx);
        gpui_kit::open_window(options, cx, build).expect("open_window");
        cx.activate(true);
    });
}
