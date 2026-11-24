//! The searchable-list family of the delegate protocol (docs/PROTOCOLS.md section 9.9): the
//! kit's `SelectState<D>`, `ComboboxState<D>`, `Select<D>` and `Combobox<D>` are generic over a
//! `SearchableListDelegate` the app implements upstream. The binding implements it once, here,
//! over a row source: `GkSearchDelegate`, whose items are data the app pushed
//! (`C_SEARCH_ITEMS`), filtered in the library as upstream's `SearchableVec` filters them, and
//! whose rows, when the app asked for them, are the source's row views (one per item, keyed by
//! the item's section and its index among all the section's items, so a row keeps its view
//! while the filter changes). Searching is local; a source made with search requests also asks
//! the app (`SEARCH_EVENT`), which answers with new items. App code never runs from here.
//!
//! The states and elements over this delegate are generated (overrides.toml `[instantiate]`,
//! `SelectState<crate::shim_select::GkSearchDelegate>` and the others): this file holds the
//! delegate, its item type `GkItem` (`gk.SearchItem` in Go, a value both ways: the
//! generator's `libval` rule, gen/gk_lists.py) and the family's call op. Its
//! numbers: call 176-191, shape 44-47.

use crate::genapi::{Call, GkResult, Put, R, Unwire, Wire};
use crate::ops::*;
use crate::source::{self, Row};
use gpui_kit::component::IndexPath;
use gpui_kit::component::searchable_list::{SearchableListChange, SearchableListDelegate, SearchableListItem};
use gpui_kit::{AnyElement, App, IntoElement, SharedString, Task, Window};
use std::cell::RefCell;
use std::collections::HashMap;

// --- the item ------------------------------------------------------------------------------

/// One item of a select or combobox: what upstream's `SearchableListItem` asks of an item, as
/// data. `value` identifies it (`Value = SharedString`), `title` is shown in the menu and the
/// trigger, `display` (when set) replaces the title in the trigger, `keywords` are searched
/// besides the title, and a disabled item is shown but cannot be chosen.
#[derive(Clone, Debug, PartialEq)]
pub struct GkItem {
    pub value: SharedString,
    pub title: SharedString,
    pub display: Option<SharedString>,
    pub keywords: SharedString,
    pub disabled: bool,
}

impl SearchableListItem for GkItem {
    type Value = SharedString;

    fn title(&self) -> SharedString {
        self.title.clone()
    }

    fn display_title(&self) -> Option<AnyElement> {
        self.display.clone().map(IntoElement::into_any_element)
    }

    fn render(&self, _: &mut Window, _: &mut App) -> impl IntoElement {
        self.title.clone()
    }

    fn value(&self) -> &SharedString {
        &self.value
    }

    fn matches(&self, query: &str) -> bool {
        let q = query.to_lowercase();
        self.title.to_lowercase().contains(&q) || (!self.keywords.is_empty() && self.keywords.to_lowercase().contains(&q))
    }

    fn disabled(&self) -> bool {
        self.disabled
    }
}

/// `gk.Search_Item`: str value, str title, opt<str> display, str keywords, bool disabled.
impl Wire for GkItem {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.value);
        out.put_str(&self.title);
        match &self.display {
            Some(d) => {
                out.put_u8(1);
                out.put_str(d);
            }
            None => out.put_u8(0),
        }
        out.put_str(&self.keywords);
        out.put_u8(self.disabled as u8);
    }
}

impl Unwire for GkItem {
    fn get(r: &mut R) -> Self {
        let value = SharedString::from(r.str().to_owned());
        let title = SharedString::from(r.str().to_owned());
        let display = (r.u8() != 0).then(|| SharedString::from(r.str().to_owned()));
        let keywords = SharedString::from(r.str().to_owned());
        let disabled = r.u8() != 0;
        GkItem { value, title, display, keywords, disabled }
    }
}

impl Default for GkItem {
    fn default() -> Self {
        GkItem {
            value: SharedString::default(),
            title: SharedString::default(),
            display: None,
            keywords: SharedString::default(),
            disabled: false,
        }
    }
}

// --- the items of a source -------------------------------------------------------------------

/// A section of items; `title` set means the items are grouped (`SearchableGroup` upstream).
#[derive(Clone, Default)]
struct Section {
    title: Option<SharedString>,
    items: Vec<GkItem>,
}

/// What the app pushed for a source (`C_SEARCH_ITEMS`).
#[derive(Clone, Default)]
struct Items {
    sections: Vec<Section>,
    /// The source's rows draw the items (the app gave a row procedure).
    rows: bool,
    /// perform_search also asks the app (a request on SHAPE_SEARCH_EVENT).
    requests: bool,
    /// The source's HEADER row of each group draws the group's header.
    headers: bool,
}

const F_ROWS: u8 = 1;
const F_REQUESTS: u8 = 2;
const F_HEADERS: u8 = 4;

/// u8 flags, u32 n, (opt<str> title, u32 n, item each) each.
fn read_items(r: &mut R) -> Items {
    let flags = r.u8();
    Items {
        sections: read_sections(r),
        rows: flags & F_ROWS != 0,
        requests: flags & F_REQUESTS != 0,
        headers: flags & F_HEADERS != 0,
    }
}

fn read_sections(r: &mut R) -> Vec<Section> {
    let n = r.u32() as usize;
    let mut out = Vec::with_capacity(n.min(4096));
    for _ in 0..n {
        let title = (r.u8() != 0).then(|| SharedString::from(r.str().to_owned()));
        let k = r.u32() as usize;
        let mut items = Vec::with_capacity(k.min(4096));
        for _ in 0..k {
            items.push(GkItem::get(r));
        }
        out.push(Section { title, items });
    }
    out
}

thread_local! {
    /// The items the app pushed for each source; every delegate made from the source takes a
    /// copy (`from_source`), so a state's `set_items(src)` after a new `gk.search_items` sees them.
    static ITEMS: RefCell<HashMap<u64, Items>> = RefCell::new(HashMap::new());
}

// --- the delegate ------------------------------------------------------------------------------

/// The `SearchableListDelegate` over a row source. Without search requests it filters the items
/// as upstream's `SearchableVec` does (an item matches by `matches`; a group stays while one of
/// its items or its title matches). With search requests it shows exactly the items the app
/// pushed: the app answers a query by pushing new items (`gk.search_items`), setting them on the
/// state (`set_items`), and answering the request.
pub struct GkSearchDelegate {
    pub src: u64,
    items: Items,
    query: String,
    /// The visible items in the order `item` hands them out, and each one's (section, index
    /// among all the section's items): the key of its row.
    view: Vec<Vec<GkItem>>,
    keys: Vec<Vec<(usize, usize)>>,
    /// The visible groups' titles and their sections among all (grouped items only).
    titles: Vec<SharedString>,
    groups: Vec<usize>,
}

impl GkSearchDelegate {
    /// The delegate over `src`, with the items the app pushed for it (`gk.search_items`).
    pub fn from_source(src: u64) -> GkResult<Self> {
        source::count(src, 0)?;
        let items = ITEMS.with(|m| m.borrow().get(&src).cloned()).unwrap_or_default();
        let mut d = GkSearchDelegate { src, items, query: String::new(), view: Vec::new(), keys: Vec::new(), titles: Vec::new(), groups: Vec::new() };
        d.filter();
        Ok(d)
    }

    fn grouped(&self) -> bool {
        self.items.sections.iter().any(|s| s.title.is_some())
    }

    /// Rebuilds the visible items from the items and the query.
    fn filter(&mut self) {
        let q = self.query.to_lowercase();
        let local = !self.items.requests;
        let grouped = self.grouped();
        self.view.clear();
        self.keys.clear();
        self.titles.clear();
        self.groups.clear();
        if !grouped {
            self.view.push(Vec::new());
            self.keys.push(Vec::new());
        }
        for (s, sec) in self.items.sections.iter().enumerate() {
            let rows: Vec<usize> = sec.items.iter().enumerate()
                .filter(|(_, i)| !local || i.matches(&self.query)).map(|(n, _)| n).collect();
            if grouped {
                let title = sec.title.clone().unwrap_or_default();
                if local && rows.is_empty() && !title.to_lowercase().contains(&q) {
                    continue;
                }
                self.titles.push(title);
                self.groups.push(s);
                self.view.push(Vec::new());
                self.keys.push(Vec::new());
            }
            let (v, k) = (self.view.last_mut().unwrap(), self.keys.last_mut().unwrap());
            for r in rows {
                v.push(sec.items[r].clone());
                k.push((s, r));
            }
        }
    }
}

/// The delegate read back: its row source.
impl Wire for GkSearchDelegate {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.src);
    }
}

impl SearchableListDelegate for GkSearchDelegate {
    type Item = GkItem;

    fn sections_count(&self, _: &App) -> usize {
        self.view.len()
    }

    #[allow(deprecated)]
    fn section(&self, section: usize) -> Option<AnyElement> {
        // The group's title, which the kit wraps in its section header (SearchableGroup).
        self.titles.get(section).cloned().map(IntoElement::into_any_element)
    }

    fn items_count(&self, section: usize) -> usize {
        self.view.get(section).map_or(0, Vec::len)
    }

    fn item(&self, ix: IndexPath) -> Option<&GkItem> {
        self.view.get(ix.section)?.get(ix.row)
    }

    fn position<V>(&self, value: &V) -> Option<IndexPath>
    where
        Self::Item: SearchableListItem<Value = V>,
        V: PartialEq,
    {
        for (s, items) in self.view.iter().enumerate() {
            if let Some(r) = items.iter().position(|i| i.value() == value) {
                return Some(IndexPath::default().section(s).row(r));
            }
        }
        None
    }

    fn perform_search(&mut self, query: &str, _: &mut Window, cx: &mut App) -> Task<()> {
        self.query = query.to_string();
        if !self.items.requests {
            self.filter();
            return Task::ready(());
        }
        // A request: the task ends when the app answers (after it set the items it found).
        match source::request(self.src, SHAPE_SEARCH_EVENT, query, cx) {
            Ok(rx) => cx.foreground_executor().spawn(async move {
                let _ = rx.await;
            }),
            Err(_) => Task::ready(()),
        }
    }

    fn render_item(&self, ix: IndexPath, _item: &GkItem, _checked: bool, window: &mut Window, cx: &mut App) -> Option<AnyElement> {
        // The app's row view when it gave a row procedure; upstream's row (the title and the
        // check mark) otherwise, and for the one frame a row has no view yet.
        if !self.items.rows {
            return None;
        }
        let (s, r) = *self.keys.get(ix.section)?.get(ix.row)?;
        source::watch(self.src, window.current_view());
        match source::row(self.src, source::key(s as u32, r as u32), window, cx) {
            Row::View(v) => Some(v.into_any_element()),
            Row::Missing => None,
        }
    }

    fn render_section_header(&self, section: usize, window: &mut Window, cx: &mut App) -> Option<AnyElement> {
        // The group's HEADER row view when the app asked for header rows; otherwise none, and the
        // kit draws the group's title (`section`) in its own header.
        if !self.items.headers {
            return None;
        }
        let s = *self.groups.get(section)?;
        source::watch(self.src, window.current_view());
        match source::row(self.src, source::key(s as u32, source::HEADER_ROW as u32), window, cx) {
            Row::View(v) => Some(v.into_any_element()),
            Row::Missing => None,
        }
    }

    fn is_item_enabled(&self, _ix: IndexPath, item: &GkItem, _cx: &App) -> bool {
        !item.disabled
    }

    fn is_item_checked(&self, _ix: IndexPath, item: &GkItem, current: &[(IndexPath, GkItem)], _cx: &App) -> bool {
        current.iter().any(|(_, i)| i.value == item.value)
    }

    /// Upstream's default (each change applied in order), then the rules the app gave the source
    /// in advance (`gk.search_will_change`, PROTOCOLS.md 11.25): upstream calls this synchronously
    /// while the list is borrowed, with no context, so it answers from data.
    fn on_will_change(&mut self, selection: &mut Vec<(IndexPath, GkItem)>, changes: &[SearchableListChange]) {
        let before = selection.clone();
        for change in changes {
            match change {
                SearchableListChange::Select { index } => {
                    let Some(item) = self.item(*index).cloned() else {
                        continue;
                    };
                    if !selection.iter().any(|(_, s)| s.value == item.value) {
                        selection.push((*index, item));
                    }
                }
                SearchableListChange::Deselect { index } => {
                    if let Some(item) = self.item(*index) {
                        let v = item.value.clone();
                        if selection.iter().any(|(_, s)| s.value == v) {
                            selection.retain(|(_, s)| s.value != v);
                            continue;
                        }
                    }
                    selection.retain(|(ix, _)| ix != index);
                }
            }
        }
        if let Some(rules) = RULES.with(|m| m.borrow().get(&self.src).cloned()) {
            rules.apply(&before, selection);
        }
    }
}

// --- rules for a selection change (docs/PROTOCOLS.md 11.25) --------------------------------------

/// What a selection may become, given by the app in advance (`gk.search_will_change`):
/// `blocked` values are never added, `locked` values are never taken out once selected, and a
/// change that would leave fewer than `min` or more than `max` items selected (0: no limit) is
/// refused whole (the selection stays as it was, upstream's veto).
#[derive(Clone, Default)]
struct Rules {
    min: usize,
    max: usize,
    locked: Vec<SharedString>,
    blocked: Vec<SharedString>,
}

impl Rules {
    fn apply(&self, before: &[(IndexPath, GkItem)], selection: &mut Vec<(IndexPath, GkItem)>) {
        selection.retain(|(_, s)| !self.blocked.contains(&s.value));
        for (ix, item) in before {
            if self.locked.contains(&item.value) && !selection.iter().any(|(_, s)| s.value == item.value) {
                selection.push((*ix, item.clone()));
            }
        }
        let n = selection.len();
        if (self.min > 0 && n < self.min) || (self.max > 0 && n > self.max) {
            *selection = before.to_vec();
        }
    }
}

thread_local! {
    static RULES: RefCell<HashMap<u64, Rules>> = RefCell::new(HashMap::new());
}

/// C_SEARCH_RULES: u64 source, u32 min, u32 max, u32 n, str locked each, u32 n, str blocked each
/// (buffered; replaces the source's rules).
fn search_rules(r: &mut R) -> GkResult<()> {
    let src = r.u64();
    let (min, max) = (r.u32() as usize, r.u32() as usize);
    let mut strs = |r: &mut R| -> Vec<SharedString> {
        let n = r.u32() as usize;
        (0..n).map(|_| SharedString::from(r.str().to_owned())).collect()
    };
    let locked = strs(r);
    let blocked = strs(r);
    source::count(src, 0)?;
    RULES.with(|m| {
        let mut m = m.borrow_mut();
        m.retain(|s, _| source::count(*s, 0).is_ok());
        m.insert(src, Rules { min, max, locked, blocked });
    });
    Ok(())
}

// --- the call op -----------------------------------------------------------------------------

/// The family's call ops; None: not one of them.
pub fn call(op: u16, r: &mut R, _c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_SEARCH_ITEMS => (|| {
            // u64 source, the items (read_items). Immediate when no state uses the source yet
            // (it is read by the state's constructor), and also when buffered: the state's
            // set_items(src) after it reads them.
            let src = r.u64();
            let items = read_items(r);
            source::count(src, 0)?;
            ITEMS.with(|m| {
                let mut m = m.borrow_mut();
                // The items of released sources go when new ones arrive.
                m.retain(|s, _| source::count(*s, 0).is_ok());
                m.insert(src, items);
            });
            Ok(())
        })(),
        C_SEARCH_RULES => search_rules(r), // rules for a selection change (PROTOCOLS.md 11.25)
        _ => return crate::shim_text::call(op, r, _c), // data closures (PROTOCOLS.md 9.14)
    })
}

/// An app view drawn by an element closure that gets no `&mut App` (`Select::empty`, the
/// generator's `viewfn` rule): the view, or nothing when none was given.
pub fn view_element(v: &Option<gpui_kit::AnyView>) -> AnyElement {
    match v {
        Some(v) => v.clone().into_any_element(),
        None => gpui_kit::div().into_any_element(),
    }
}
