//! ref-reader: the Rust reference for examples/go/reader (and examples/odin/reader), written the way
//! a Rust GPUI-kit app is written, call for call with the Go app: the articles of a feed file (or of
//! a folder of .md files) in a list on the left (a ListDelegate), the open articles as tabs on the
//! right, each drawn by the kit's Markdown view; a toolbar of icon buttons with tooltips, a settings
//! dialog opened with a closure (font size, theme, order, summaries), a notification when a load
//! finishes (the load runs on a thread), and the article fading in (`EffectTransition`) when it is
//! opened.
//!
//!   ref-reader                  a real window
//!   ref-reader --script FILE    offscreen, driven by a gk-meter script (diag build)
//!   READER_FEED=path            a feed file or a folder of .md files; default the bundled sample
use futures::channel::oneshot;
use gpui_kit::base::animation::EffectTransition;
use gpui_kit::base::{h_flex, h_resizable, resizable_panel, v_flex};
use gpui_kit::component::button::{Button, ButtonVariants};
use gpui_kit::component::dialog::{DialogClose, DialogFooter};
use gpui_kit::component::input::{Input, InputEvent, InputState};
use gpui_kit::component::label::Label;
use gpui_kit::component::list::{List, ListDelegate, ListEvent, ListItem, ListState};
use gpui_kit::component::searchable_list::SearchableListItem;
use gpui_kit::component::select::{SearchableVec, Select, SelectEvent, SelectState};
use gpui_kit::component::slider::{Slider, SliderEvent, SliderState};
use gpui_kit::component::switch::Switch;
use gpui_kit::component::tab::{Tab, TabBar};
use gpui_kit::component::text::TextView;
use gpui_kit::component::{
    ActiveTheme, Disableable, Icon, IconName, IndexPath, Sizable, StyledExt, Theme, ThemeMode, WindowExt,
};
use gpui_kit::{
    Action, AnyWindowHandle, App, AppContext, Context, Entity, FocusHandle, InteractiveElement, IntoElement, KeyBinding,
    ParentElement, Render, ScrollStrategy, SharedString, Styled, Subscription, WeakEntity, Window, div, px,
};
use std::cell::RefCell;
use std::rc::Rc;
use std::time::{Duration, Instant};

gpui_kit::actions!(reader, [Reload, CloseActive, OpenSettings, NextArticle, PrevArticle]);

const SAMPLE: &str = include_str!("../../../examples/go/reader/sample-feed.md");

/// An app-wide listener for action A: it runs `f` on the app in its window once the key's dispatch
/// is over (as the Go app's gk.Shortcut does).
fn shortcut<A: Action>(cx: &mut App, app: WeakEntity<ReaderApp>, wh: AnyWindowHandle, f: fn(&mut ReaderApp, &mut Window, &mut Context<ReaderApp>)) {
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

#[derive(Clone, Default)]
struct Article {
    title: String,
    source: String,
    date: String,
    body: String,
    read: bool,
    starred: bool,
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum Order {
    Newest,
    Oldest,
    ByTitle,
}

/// A select's item: a value and the title shown (the Go app's gk.SearchItem).
#[derive(Clone)]
struct Item {
    value: SharedString,
    title: SharedString,
}

impl SearchableListItem for Item {
    type Value = SharedString;

    fn title(&self) -> SharedString {
        self.title.clone()
    }

    fn render(&self, _: &mut Window, _: &mut App) -> impl IntoElement {
        self.title.clone()
    }

    fn value(&self) -> &SharedString {
        &self.value
    }
}

fn items(list: &[(&str, &str)]) -> SearchableVec<Item> {
    SearchableVec::new(list.iter().map(|(v, t)| Item { value: (*v).into(), title: (*t).into() }).collect::<Vec<_>>())
}

// --- loading, on a thread ---------------------------------------------------------------------------

struct Loaded {
    articles: Vec<Article>,
    took: Duration,
    err: Option<String>,
}

fn load_feed(path: &str) -> Loaded {
    let start = Instant::now();
    let out = if path.is_empty() {
        parse_feed(SAMPLE)
    } else if std::path::Path::new(path).is_dir() {
        let mut files: Vec<_> = std::fs::read_dir(path)
            .map(|d| d.filter_map(|e| e.ok()).map(|e| e.path()).filter(|p| p.extension().is_some_and(|x| x == "md")).collect())
            .unwrap_or_default();
        files.sort();
        let base = std::path::Path::new(path).file_name().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default();
        let mut out = Vec::new();
        for f in files {
            let Ok(text) = std::fs::read_to_string(&f) else { continue };
            let mut title = f.file_stem().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default();
            if let Some(rest) = text.strip_prefix("# ") {
                title = rest.split('\n').next().unwrap_or("").to_string();
            }
            out.push(Article { title, source: base.clone(), body: text, ..Default::default() });
        }
        out
    } else {
        match std::fs::read_to_string(path) {
            Ok(text) => parse_feed(&text),
            Err(_) => return Loaded { articles: Vec::new(), took: Duration::ZERO, err: Some(format!("cannot read {path}")) },
        }
    };
    Loaded { articles: out, took: start.elapsed(), err: None }
}

/// Reads "+++ title | source | date" lines, each followed by the article's Markdown.
fn parse_feed(text: &str) -> Vec<Article> {
    let mut out = Vec::new();
    let mut rest = text;
    while !rest.is_empty() {
        if !rest.starts_with("+++ ") {
            let Some(nl) = rest.find("\n+++ ") else { break };
            rest = &rest[nl + 1..];
        }
        let (head, after) = rest[4..].split_once('\n').unwrap_or((&rest[4..], ""));
        let mut body = after;
        rest = "";
        if let Some(next) = after.find("\n+++ ") {
            body = &after[..next + 1];
            rest = &after[next + 1..];
        }
        let mut a = Article { body: body.trim().to_string(), ..Default::default() };
        let parts: Vec<&str> = head.split('|').collect();
        for (i, p) in [&mut a.title, &mut a.source, &mut a.date].into_iter().enumerate() {
            if i < parts.len() {
                *p = parts[i].trim().to_string();
            }
        }
        out.push(a);
    }
    out
}

/// The first line of the article that is not a heading.
fn summary(body: &str) -> String {
    for line in body.split_inclusive('\n') {
        let l = line.trim();
        if !l.is_empty() && !l.starts_with('#') {
            return l.to_string();
        }
    }
    String::new()
}

/// What the app and the list's delegate share.
struct Model {
    articles: Vec<Article>,
    shown: Vec<usize>, // the articles the list shows, in order
    summaries: bool,
}

type Shared = Rc<RefCell<Model>>;

struct ArticleRows {
    model: Shared,
}

impl ListDelegate for ArticleRows {
    type Item = ListItem;

    fn items_count(&self, _: usize, _: &App) -> usize {
        self.model.borrow().shown.len()
    }

    fn render_item(&mut self, ix: IndexPath, _: &mut Window, cx: &mut Context<ListState<Self>>) -> Option<ListItem> {
        let m = self.model.borrow();
        let i = *m.shown.get(ix.row)?;
        let art = &m.articles[i];
        let mut title = Label::new(art.title.clone()).truncate();
        if !art.read {
            title = title.font_semibold();
        }
        let muted = cx.theme().muted_foreground;
        let mut col = v_flex()
            .flex_1()
            .min_w_0()
            .child(title)
            .child(Label::new(format!("{} · {}", art.source, art.date)).text_xs().text_color(muted));
        if m.summaries {
            col = col.child(Label::new(summary(&art.body)).text_xs().truncate().text_color(muted));
        }
        let mut row = h_flex().w_full().gap_2().items_center().child(col);
        if art.starred {
            row = row.child(Icon::new(IconName::StarFill).text_color(cx.theme().warning));
        }
        Some(ListItem::new(("article", i)).child(row))
    }

    fn set_selected_index(&mut self, _: Option<IndexPath>, _: &mut Window, _: &mut Context<ListState<Self>>) {}
}

// --- the app ----------------------------------------------------------------------------------------

struct ReaderApp {
    model: Shared,
    feed: String,
    tabs: Vec<usize>,      // open articles
    active: Option<usize>, // the tab shown
    loading: bool,
    opened: usize, // how many times an article was opened, for the fade-in's key
    font_size: f32,
    dark: bool,
    order: Order,
    list: Entity<ListState<ArticleRows>>,
    search: Entity<InputState>,
    font: Entity<SliderState>,
    theme_select: Entity<SelectState<SearchableVec<Item>>>,
    order_select: Entity<SelectState<SearchableVec<Item>>>,
    focus: FocusHandle,
    _subs: Vec<Subscription>,
}

impl ReaderApp {
    fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let model = Rc::new(RefCell::new(Model { articles: Vec::new(), shown: Vec::new(), summaries: true }));
        cx.bind_keys([
            KeyBinding::new("ctrl-r", Reload, None),
            KeyBinding::new("ctrl-w", CloseActive, None),
            KeyBinding::new("ctrl-,", OpenSettings, None),
            KeyBinding::new("alt-down", NextArticle, None),
            KeyBinding::new("alt-up", PrevArticle, None),
        ]);
        let (me, wh) = (cx.weak_entity(), window.window_handle());
        shortcut::<Reload>(cx, me.clone(), wh, Self::reload);
        shortcut::<CloseActive>(cx, me.clone(), wh, Self::close_active);
        shortcut::<OpenSettings>(cx, me.clone(), wh, Self::open_settings);
        shortcut::<NextArticle>(cx, me.clone(), wh, |a, w, cx| a.step(1, w, cx));
        shortcut::<PrevArticle>(cx, me, wh, |a, w, cx| a.step(-1, w, cx));
        let focus = cx.focus_handle();
        window.focus(&focus, cx);

        let search = cx.new(|cx| InputState::new(window, cx).placeholder("Search titles"));
        let list = cx.new(|cx| ListState::new(ArticleRows { model: model.clone() }, window, cx));
        let font = cx.new(|_| SliderState::new().min(12.).max(24.).step(1.).default_value(15.));
        let theme_select = cx.new(|cx| SelectState::new(items(&[("light", "Light"), ("dark", "Dark")]), None, window, cx));
        theme_select.update(cx, |s, cx| s.set_selected_value(&"light".into(), window, cx));
        let order_select = cx.new(|cx| {
            SelectState::new(items(&[("newest", "Newest first"), ("oldest", "Oldest first"), ("title", "By title")]), None, window, cx)
        });
        order_select.update(cx, |s, cx| s.set_selected_value(&"newest".into(), window, cx));
        let subs = vec![
            cx.subscribe_in(&search, window, |this, _, e: &InputEvent, window, cx| {
                if matches!(e, InputEvent::Change) {
                    this.resort(window, cx);
                }
            }),
            cx.subscribe_in(&list, window, |this, _, e: &ListEvent, window, cx| match e {
                ListEvent::Confirm(ix) | ListEvent::Select(ix) => {
                    let i = this.model.borrow().shown.get(ix.row).copied();
                    if let Some(i) = i {
                        this.open(i, window, cx);
                    }
                }
                _ => {}
            }),
            cx.subscribe_in(&font, window, |this, _, e: &SliderEvent, _, cx| {
                let (SliderEvent::Change(v) | SliderEvent::Release(v)) = e;
                this.font_size = v.start();
                this.refresh(cx);
            }),
            cx.subscribe_in(&theme_select, window, |this, _, e: &SelectEvent<SearchableVec<Item>>, window, cx| {
                let SelectEvent::Confirm(Some(v)) = e else { return };
                this.set_dark(v == "dark", window, cx);
            }),
            cx.subscribe_in(&order_select, window, |this, _, e: &SelectEvent<SearchableVec<Item>>, window, cx| {
                let SelectEvent::Confirm(Some(v)) = e else { return };
                this.order = match v.as_ref() {
                    "oldest" => Order::Oldest,
                    "title" => Order::ByTitle,
                    _ => Order::Newest,
                };
                this.resort(window, cx);
            }),
        ];
        let mut app = ReaderApp {
            model,
            feed: std::env::var("READER_FEED").unwrap_or_default(),
            tabs: Vec::new(),
            active: None,
            loading: false,
            opened: 0,
            font_size: 15.,
            dark: false,
            order: Order::Newest,
            list,
            search,
            font,
            theme_select,
            order_select,
            focus,
            _subs: subs,
        };
        app.start_load(window, cx);
        app
    }

    /// Every view and the list drawn again (the Go app's gk.Refresh).
    fn refresh(&mut self, cx: &mut Context<Self>) {
        self.list.update(cx, |_, cx| cx.notify());
        cx.notify();
        cx.defer(|cx| cx.refresh_windows());
    }

    fn start_load(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.loading {
            return;
        }
        self.loading = true;
        let feed = self.feed.clone();
        let (tx, rx) = oneshot::channel();
        std::thread::spawn(move || {
            let _ = tx.send(load_feed(&feed));
        });
        let wh = window.window_handle();
        cx.spawn(async move |this, cx| {
            let Ok(m) = rx.await else { return };
            let _ = cx.update_window(wh, |_, window, cx| {
                this.update(cx, |a, cx| a.loaded(m, window, cx)).ok();
            });
        })
        .detach();
    }

    fn loaded(&mut self, m: Loaded, window: &mut Window, cx: &mut Context<Self>) {
        self.loading = false;
        if let Some(err) = m.err {
            window.push_notification(err, cx);
            self.refresh(cx);
            return;
        }
        let n = m.articles.len();
        self.model.borrow_mut().articles = m.articles;
        self.tabs.clear();
        self.active = None;
        self.resort(window, cx); // and refresh
        window.push_notification(format!("Loaded {n} articles in {:.0} ms", m.took.as_secs_f64() * 1000.), cx);
    }

    /// The list's articles, filtered by the search and in the chosen order; then refresh.
    fn resort(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let needle = self.search.read(cx).value().to_lowercase();
        {
            let mut m = self.model.borrow_mut();
            let mut shown: Vec<usize> = (0..m.articles.len())
                .filter(|&i| needle.is_empty() || m.articles[i].title.to_lowercase().contains(&needle))
                .collect();
            let arts = &m.articles;
            let key = |i: usize| if self.order == Order::ByTitle { &arts[i].title } else { &arts[i].date };
            shown.sort_by(|&x, &y| if self.order == Order::Newest { key(y).cmp(key(x)) } else { key(x).cmp(key(y)) });
            m.shown = shown;
        }
        self.select_active(false, window, cx);
    }

    /// Selects the open article's row in the list, if the list shows it.
    fn select_active(&mut self, scroll: bool, window: &mut Window, cx: &mut Context<Self>) {
        let row = self.active.and_then(|n| {
            let t = self.tabs[n];
            self.model.borrow().shown.iter().position(|&s| s == t)
        });
        self.list.update(cx, |l, cx| match row {
            Some(r) => {
                l.set_selected_index(Some(IndexPath::default().row(r)), window, cx);
                if scroll {
                    l.scroll_to_item(IndexPath::default().row(r), ScrollStrategy::Nearest, window, cx);
                }
            }
            None => l.set_selected_index(None, window, cx),
        });
        self.refresh(cx);
    }

    // --- the views ------------------------------------------------------------------------------------

    fn toolbar(&self, cx: &mut Context<Self>) -> impl IntoElement {
        let starred = self.active.is_some_and(|n| self.model.borrow().articles[self.tabs[n]].starred);
        let star = if starred { IconName::StarFill } else { IconName::Star };
        let (theme_icon, theme_tip) = if self.dark { (IconName::Sun, "Light theme") } else { (IconName::Moon, "Dark theme") };
        let none = self.active.is_none();
        h_flex()
            .gap_1()
            .items_center()
            .p_2()
            .border_b_1()
            .border_color(cx.theme().border)
            .child(
                Button::new("reload")
                    .ghost()
                    .icon(IconName::RefreshCw)
                    .tooltip("Reload the feed")
                    .loading(self.loading)
                    .on_click(cx.listener(|this, _, window, cx| this.reload(window, cx))),
            )
            .child(
                Button::new("prev")
                    .ghost()
                    .icon(IconName::ArrowUp)
                    .tooltip("Previous article")
                    .on_click(cx.listener(|this, _, window, cx| this.step(-1, window, cx))),
            )
            .child(
                Button::new("next")
                    .ghost()
                    .icon(IconName::ArrowDown)
                    .tooltip("Next article")
                    .on_click(cx.listener(|this, _, window, cx| this.step(1, window, cx))),
            )
            .child(
                Button::new("star")
                    .ghost()
                    .icon(star)
                    .tooltip("Star")
                    .disabled(none)
                    .on_click(cx.listener(|this, _, _, cx| this.toggle_star(cx))),
            )
            .child(
                Button::new("close")
                    .ghost()
                    .icon(IconName::Close)
                    .tooltip("Close the tab")
                    .disabled(none)
                    .on_click(cx.listener(|this, _, window, cx| this.close_active(window, cx))),
            )
            .child(div().flex_1())
            .child(
                Button::new("theme")
                    .ghost()
                    .icon(theme_icon)
                    .tooltip(theme_tip)
                    .on_click(cx.listener(|this, _, window, cx| this.set_dark(!this.dark, window, cx))),
            )
            .child(
                Button::new("settings")
                    .ghost()
                    .icon(IconName::Settings)
                    .tooltip("Settings")
                    .on_click(cx.listener(|this, _, window, cx| this.open_settings(window, cx))),
            )
    }

    fn sidebar(&self, cx: &App) -> impl IntoElement {
        let m = self.model.borrow();
        let unread = m.articles.iter().filter(|a| !a.read).count();
        let body = if self.loading && m.articles.is_empty() {
            v_flex().size_full().items_center().justify_center().child(Label::new("Loading…")).into_any_element()
        } else {
            List::new(&self.list).into_any_element()
        };
        v_flex()
            .size_full()
            .gap_2()
            .p_2()
            .bg(cx.theme().sidebar)
            .child(Input::new(&self.search))
            .child(
                Label::new(format!("{} articles, {} unread", m.shown.len(), unread))
                    .text_sm()
                    .text_color(cx.theme().muted_foreground),
            )
            .child(div().flex_1().min_h_0().child(body))
    }

    fn reading_pane(&self, cx: &mut Context<Self>) -> impl IntoElement {
        let Some(active) = self.active else {
            return v_flex()
                .size_full()
                .items_center()
                .justify_center()
                .child(Label::new("Open an article from the list").text_color(cx.theme().muted_foreground))
                .into_any_element();
        };
        let m = self.model.borrow();
        let mut bar = TabBar::new("tabs").selected_index(active).on_click(cx.listener(|this, n: &usize, window, cx| {
            let t = this.tabs[*n];
            this.open(t, window, cx);
        }));
        for (n, &t) in self.tabs.iter().enumerate() {
            bar = bar.child(
                Tab::new().label(m.articles[t].title.clone()).suffix(
                    Button::new(("close-tab", n)).ghost().xsmall().icon(IconName::Close).on_click(cx.listener(
                        move |this, _, window, cx| {
                            cx.stop_propagation(); // the tab itself is not clicked
                            this.close_tab(n, window, cx);
                        },
                    )),
                ),
            );
        }
        let i = self.tabs[active];
        let page = TextView::markdown(("article", i), m.articles[i].body.clone()).scrollable(true).text_size(px(self.font_size));
        let fade = EffectTransition::new(Duration::from_millis(200)).fade(0., 1.).slide_y(px(12.), px(0.));
        v_flex()
            .size_full()
            .child(bar)
            .child(div().flex_1().min_h_0().p_4().child(fade.apply(page, SharedString::from(format!("open-{}", self.opened)))))
            .into_any_element()
    }

    // --- the settings dialog --------------------------------------------------------------------------

    fn open_settings(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let this = cx.weak_entity();
        let (font, theme_select, order_select) = (self.font.clone(), self.theme_select.clone(), self.order_select.clone());
        window.open_dialog(cx, move |d, _, cx| {
            let Some(app) = this.upgrade() else { return d };
            let (font_size, summaries) = {
                let a = app.read(cx);
                (a.font_size, a.model.borrow().summaries)
            };
            let row = |label: &'static str, sel: &Entity<SelectState<SearchableVec<Item>>>| {
                h_flex().gap_2().items_center().child(Label::new(label).w(px(120.))).child(Select::new(sel))
            };
            let this = this.clone();
            d.title(Label::new("Settings"))
                .child(
                    v_flex()
                        .gap_4()
                        .child(v_flex().gap_1().child(Label::new(format!("Font size: {font_size:.0} px"))).child(Slider::new(&font)))
                        .child(row("Theme", &theme_select))
                        .child(row("Order", &order_select))
                        .child(
                            Switch::new("summaries")
                                .label("Show a summary under each title")
                                .checked(summaries)
                                .on_click(move |on, _, cx| {
                                    let on = *on;
                                    this.update(cx, |a, cx| {
                                        a.model.borrow_mut().summaries = on;
                                        a.refresh(cx);
                                    })
                                    .ok();
                                }),
                        ),
                )
                .footer(DialogFooter::new().child(DialogClose::new().trigger(|b| b.primary().label("Done"))))
        });
    }

    fn set_dark(&mut self, dark: bool, window: &mut Window, cx: &mut Context<Self>) {
        self.dark = dark;
        Theme::change(if dark { ThemeMode::Dark } else { ThemeMode::Light }, Some(window), cx);
        let value: SharedString = if dark { "dark".into() } else { "light".into() };
        self.theme_select.update(cx, |s, cx| s.set_selected_value(&value, window, cx));
        self.refresh(cx);
    }

    // --- what the app does ----------------------------------------------------------------------------

    fn reload(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.start_load(window, cx);
        self.refresh(cx);
    }

    fn open(&mut self, i: usize, window: &mut Window, cx: &mut Context<Self>) {
        let n = match self.tabs.iter().position(|&t| t == i) {
            Some(n) => n,
            None => {
                self.tabs.push(i);
                self.tabs.len() - 1
            }
        };
        if Some(n) != self.active {
            self.active = Some(n);
            self.opened += 1;
        }
        self.model.borrow_mut().articles[i].read = true;
        self.select_active(true, window, cx);
    }

    fn close_tab(&mut self, n: usize, window: &mut Window, cx: &mut Context<Self>) {
        self.tabs.remove(n);
        if let Some(a) = self.active {
            if n < a || a == self.tabs.len() {
                self.active = a.checked_sub(1);
            }
        }
        self.opened += 1;
        self.select_active(false, window, cx);
    }

    fn close_active(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if let Some(a) = self.active {
            self.close_tab(a, window, cx);
        }
    }

    fn step(&mut self, by: i64, window: &mut Window, cx: &mut Context<Self>) {
        let (len, row) = {
            let m = self.model.borrow();
            let row = self.active.and_then(|n| m.shown.iter().position(|&s| s == self.tabs[n])).map_or(-1, |r| r as i64);
            (m.shown.len() as i64, row)
        };
        if len == 0 {
            return;
        }
        let target = (row + by).min(len - 1).max(0) as usize;
        let i = self.model.borrow().shown[target];
        self.open(i, window, cx);
    }

    fn toggle_star(&mut self, cx: &mut Context<Self>) {
        if let Some(n) = self.active {
            let t = self.tabs[n];
            let mut m = self.model.borrow_mut();
            m.articles[t].starred = !m.articles[t].starred;
            drop(m);
            self.refresh(cx);
        }
    }
}

impl Render for ReaderApp {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        v_flex()
            .size_full()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .track_focus(&self.focus)
            .child(self.toolbar(cx))
            .child(
                div().flex_1().w_full().min_h_0().child(
                    h_resizable("split")
                        .child(resizable_panel().size(px(300.)).child(self.sidebar(cx)))
                        .child(resizable_panel().child(self.reading_pane(cx))),
                ),
            )
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<ReaderApp> {
    gk_meter::attach(window, cx);
    cx.new(|cx| ReaderApp::new(window, cx))
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(1000., 660., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-reader: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application().with_assets(gpui_kit::assets::Assets).run(move |cx| {
        gpui_kit::init(cx);
        let options = gk_meter::window_options(1000., 660., cx);
        gpui_kit::open_window(options, cx, build).expect("open_window");
        cx.activate(true);
    });
}
