//! Pure-Rust expense tracker reference app with form validation, a virtualized
//! list, a category bar chart, and toast notifications.
//!
//! Mirrors the Go expenses example for parity testing and supports headless
//! `--script <path>` execution under the `diag` feature.

use gpui_kit::base::{h_flex, v_flex};
use gpui_kit::component::button::{Button, ButtonVariants};
use gpui_kit::component::chart::BarChart;
use gpui_kit::component::form::{Field, Form};
use gpui_kit::component::input::{Input, InputEvent, InputState};
use gpui_kit::component::label::Label;
use gpui_kit::component::list::{List, ListDelegate, ListItem, ListState};
use gpui_kit::component::searchable_list::SearchableListItem;
use gpui_kit::component::select::{SearchableVec, Select, SelectEvent, SelectState};
use gpui_kit::component::tag::Tag;
use gpui_kit::component::{ActiveTheme, IconName, IndexPath, Sizable, StyledExt, WindowExt};
use gpui_kit::{
    App, AppContext, Context, Entity, IntoElement, ParentElement, Render, ScrollStrategy, SharedString, Styled,
    Subscription, WeakEntity, Window, div, px,
};
use std::cell::RefCell;
use std::rc::Rc;

/// Fixed list of expense categories indexed by `Expense::category`.
const CATEGORIES: [&str; 4] = ["Food", "Transport", "Home", "Fun"];

#[derive(Clone)]
struct Expense {
    what: String,
    amount: f64,
    category: usize,
}

/// Select dropdown option wrapping a category name.
#[derive(Clone)]
struct Item {
    value: SharedString,
}

impl SearchableListItem for Item {
    type Value = SharedString;

    fn title(&self) -> SharedString {
        self.value.clone()
    }

    fn render(&self, _: &mut Window, _: &mut App) -> impl IntoElement {
        self.value.clone()
    }

    fn value(&self) -> &SharedString {
        &self.value
    }
}

type Shared = Rc<RefCell<Vec<Expense>>>;

/// `ListDelegate` rendering each expense row with an inline delete button.
struct ExpenseRows {
    expenses: Shared,
    app: WeakEntity<ExpensesApp>,
}

impl ListDelegate for ExpenseRows {
    type Item = ListItem;

    fn items_count(&self, _: usize, _: &App) -> usize {
        self.expenses.borrow().len()
    }

    fn render_item(&mut self, ix: IndexPath, _: &mut Window, _: &mut Context<ListState<Self>>) -> Option<ListItem> {
        let row = ix.row;
        let e = self.expenses.borrow().get(row)?.clone();
        let app = self.app.clone();
        Some(
            ListItem::new(("expense", row)).child(
                h_flex()
                    .w_full()
                    .gap_2()
                    .items_center()
                    .child(div().flex_1().child(Label::new(e.what)))
                    .child(Tag::secondary().text_xs().child(CATEGORIES[e.category]))
                    .child(Label::new(format!("{:.2}", e.amount)).w(px(70.)))
                    .child(Button::new(("delete", row)).ghost().xsmall().icon(IconName::Delete).on_click(move |_, _, cx| {
                        // Prevent the row click handler from also selecting the removed item.
                        cx.stop_propagation();
                        app.update(cx, |a, cx| {
                            a.expenses.borrow_mut().remove(row);
                            a.refresh(cx);
                        })
                        .ok();
                    })),
            ),
        )
    }

    fn set_selected_index(&mut self, _: Option<IndexPath>, _: &mut Window, _: &mut Context<ListState<Self>>) {}
}

/// Root application state holding form inputs, expense rows, and validation errors.
struct ExpensesApp {
    expenses: Shared,
    what: Entity<InputState>,
    amount: Entity<InputState>,
    category: Entity<SelectState<SearchableVec<Item>>>,
    list: Entity<ListState<ExpenseRows>>,
    /// Per-field validation messages cleared as the user edits after a failed submit.
    what_err: String,
    amount_err: String,
    category_err: String,
    _subs: Vec<Subscription>,
}

impl ExpensesApp {
    fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let what = cx.new(|cx| InputState::new(window, cx).placeholder("Coffee, bus ticket, ..."));
        let amount = cx.new(|cx| InputState::new(window, cx).placeholder("0.00"));
        let items = SearchableVec::new(CATEGORIES.iter().map(|c| Item { value: (*c).into() }).collect::<Vec<_>>());
        let category = cx.new(|cx| SelectState::new(items, None, window, cx));
        let expenses: Shared = Rc::new(RefCell::new(Vec::new()));
        let app = cx.weak_entity();
        let list = cx.new(|cx| ListState::new(ExpenseRows { expenses: expenses.clone(), app }, window, cx));
        let field_event = |this: &mut Self, _: &Entity<InputState>, e: &InputEvent, window: &mut Window, cx: &mut Context<Self>| match e {
            InputEvent::PressEnter { .. } => this.add(window, cx),
            InputEvent::Change => this.revalidate(cx),
            _ => {}
        };
        let subs = vec![
            cx.subscribe_in(&what, window, field_event),
            cx.subscribe_in(&amount, window, field_event),
            cx.subscribe_in(&category, window, |this, _, _: &SelectEvent<SearchableVec<Item>>, _, cx| this.revalidate(cx)),
        ];
        expenses.borrow_mut().extend([
            Expense { what: "Groceries".into(), amount: 42.10, category: 0 },
            Expense { what: "Monthly bus pass".into(), amount: 55., category: 1 },
            Expense { what: "Cinema".into(), amount: 12.50, category: 3 },
        ]);
        ExpensesApp { expenses, what, amount, category, list, what_err: String::new(), amount_err: String::new(), category_err: String::new(), _subs: subs }
    }

    fn refresh(&mut self, cx: &mut Context<Self>) {
        self.list.update(cx, |_, cx| cx.notify());
        cx.notify();
    }

    fn total(&self) -> f64 {
        self.expenses.borrow().iter().map(|e| e.amount).sum()
    }

    fn form(&self, cx: &mut Context<Self>) -> impl IntoElement {
        Form::new()
            .columns(3)
            .child(field("What", Input::new(&self.what), &self.what_err))
            .child(field("Amount", Input::new(&self.amount), &self.amount_err))
            .child(field("Category", Select::new(&self.category).placeholder("Choose one"), &self.category_err))
            .footer(
                Button::new("add")
                    .primary()
                    .label("Add expense")
                    .on_click(cx.listener(|this, _, window, cx| this.add(window, cx))),
            )
    }

    fn chart(&self) -> impl IntoElement {
        let mut totals: Vec<(SharedString, f64)> = CATEGORIES.iter().map(|c| (SharedString::from(*c), 0.)).collect();
        for e in self.expenses.borrow().iter() {
            totals[e.category].1 += e.amount;
        }
        v_flex()
            .gap_2()
            .w(px(280.))
            .child(Label::new("By category").font_semibold())
            .child(div().h(px(220.)).child(BarChart::new(totals).band(|t| t.0.clone()).value(|t| t.1)))
    }

    fn validate(&mut self, cx: &mut Context<Self>) -> Option<Expense> {
        let what = self.what.read(cx).value().trim().to_string();
        self.what_err = if what.is_empty() { "Say what it was for".into() } else { String::new() };
        let text = self.amount.read(cx).value().trim().to_string();
        let parsed = parse_float(&text);
        let amount = parsed.unwrap_or(0.);
        self.amount_err = if text.is_empty() {
            "Enter an amount".into()
        } else if parsed.is_none() {
            "Enter a number, like 12.50".into()
        } else if amount <= 0. {
            "The amount must be more than zero".into()
        } else {
            String::new()
        };
        self.category_err = "Choose a category".into();
        let mut category = 0;
        if let Some(at) = self.category.read(cx).selected_index(cx) {
            category = at.row;
            self.category_err.clear();
        }
        (self.what_err.is_empty() && self.amount_err.is_empty() && self.category_err.is_empty()).then_some(Expense { what, amount, category })
    }

    fn add(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let Some(e) = self.validate(cx) else {
            cx.notify();
            return;
        };
        let note = format!("Added {}: {:.2}", e.what, e.amount);
        self.expenses.borrow_mut().push(e);
        self.what.update(cx, |s, cx| s.set_value("", window, cx));
        self.amount.update(cx, |s, cx| s.set_value("", window, cx));
        self.what.update(cx, |s, cx| s.focus(window, cx));
        let last = self.expenses.borrow().len() - 1;
        self.list.update(cx, |l, cx| {
            l.set_selected_index(Some(IndexPath::default().row(last)), window, cx);
            l.scroll_to_item(IndexPath::default().row(last), ScrollStrategy::Nearest, window, cx);
        });
        window.push_notification(note, cx);
        self.refresh(cx);
    }

    fn revalidate(&mut self, cx: &mut Context<Self>) {
        if !self.what_err.is_empty() || !self.amount_err.is_empty() || !self.category_err.is_empty() {
            self.validate(cx);
        }
        cx.notify();
    }
}

fn parse_float(text: &str) -> Option<f64> {
    text.parse::<f64>().ok()
}

fn field(label: &'static str, input: impl IntoElement, message: &str) -> Field {
    let mut f = Field::new().label(label).child(input);
    if !message.is_empty() {
        let message = message.to_string();
        f = f.description_fn(move |_, cx| Label::new(message.clone()).text_xs().text_color(cx.theme().danger));
    }
    f
}

impl Render for ExpensesApp {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        v_flex()
            .size_full()
            .p_4()
            .gap_4()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .child(
                h_flex()
                    .items_center()
                    .gap_2()
                    .child(Label::new("Expenses").text_xl().font_bold())
                    .child(div().flex_1())
                    .child(Label::new(format!("Total {:.2}", self.total())).font_semibold()),
            )
            .child(self.form(cx))
            .child(
                h_flex()
                    .flex_1()
                    .min_h_0()
                    .gap_4()
                    .items_start()
                    .child(div().flex_1().h_full().child(List::new(&self.list)))
                    .child(self.chart()),
            )
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<ExpensesApp> {
    gk_meter::attach(window, cx);
    cx.new(|cx| ExpensesApp::new(window, cx))
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(860., 560., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-expenses: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application().with_assets(gpui_kit::assets::Assets).run(move |cx| {
        gpui_kit::init(cx);
        let options = gk_meter::window_options(860., 560., cx);
        gpui_kit::open_window(options, cx, build).expect("open_window");
        cx.activate(true);
    });
}
