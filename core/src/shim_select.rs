//! Searchable-list and select-menu delegate shims.
//!
//! Host code registers grouped or flat item lists (`C_SEARCH_ITEMS`) and
//! optional selection rules (`C_SEARCH_RULES`) against a row `Source`.
//! `GkSearchDelegate` implements `SearchableListDelegate`, supporting local
//! substring filtering, asynchronous host-side search (`SHAPE_SEARCH_EVENT`),
//! custom row/header views from `source::row`, and min/max/locked/blocked
//! selection enforcement.

use crate::genapi::{Call, GkResult, Put, R, Unwire, Wire};
use crate::ops::*;
use crate::source::{self, Row};
use gpui_kit::component::IndexPath;
use gpui_kit::component::searchable_list::{SearchableListChange, SearchableListDelegate, SearchableListItem};
use gpui_kit::{AnyElement, App, IntoElement, SharedString, Task, Window};
use std::cell::RefCell;
use std::collections::HashMap;

/// Selectable item transferred between host code and `GkSearchDelegate`.
#[derive(Clone, Debug, PartialEq)]
pub struct GkItem {
    /// Stable identifier compared for selection equality.
    pub value: SharedString,
    /// Primary label displayed in the list and matched during local search.
    pub title: SharedString,
    /// Optional alternate text shown in the closed trigger when selected.
    pub display: Option<SharedString>,
    /// Additional search terms matched case-insensitively during local search.
    pub keywords: SharedString,
    /// Prevents the item from being selected when true.
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

#[derive(Clone, Default)]
struct Section {
    title: Option<SharedString>,
    items: Vec<GkItem>,
}

/// Staged searchable-list dataset and feature flags for a row source.
#[derive(Clone, Default)]
struct Items {
    sections: Vec<Section>,
    /// Render items using host row views from `source::row` when true.
    rows: bool,
    /// Delegate search queries asynchronously to the host via `SHAPE_SEARCH_EVENT`.
    requests: bool,
    /// Render section headers using host views (`HEADER_ROW`) when true.
    headers: bool,
}

const F_ROWS: u8 = 1;
const F_REQUESTS: u8 = 2;
const F_HEADERS: u8 = 4;

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
    /// Latest item lists staged by `C_SEARCH_ITEMS`, keyed by source handle.
    static ITEMS: RefCell<HashMap<u64, Items>> = RefCell::new(HashMap::new());
}

/// `SearchableListDelegate` backed by staged `Items` and row source `src`.
///
/// `view`, `keys`, `titles`, and `groups` hold the post-filter projection while
/// retaining original `(section, row)` coordinates so custom row and header
/// views resolve against the unfiltered source indices.
pub struct GkSearchDelegate {
    pub src: u64,
    items: Items,
    query: String,
    /// Filtered items organized by visible section.
    view: Vec<Vec<GkItem>>,
    /// Parallel `(orig_section, orig_row)` coordinates for each entry in `view`.
    keys: Vec<Vec<(usize, usize)>>,
    /// Visible section header titles (populated only in grouped mode).
    titles: Vec<SharedString>,
    /// Parallel original section indices for each entry in `titles`.
    groups: Vec<usize>,
}

impl GkSearchDelegate {
    /// Constructs a delegate for source `src` using the item list staged by
    /// `C_SEARCH_ITEMS` (or empty if none was staged yet).
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

    /// Rebuilds the filtered `view`/`keys`/`titles`/`groups` projection from
    /// `self.items` and `self.query`.
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
        // Host-managed search: send the query and wait for the host to reply
        // after updating items.
        match source::request(self.src, SHAPE_SEARCH_EVENT, query, cx) {
            Ok(rx) => cx.foreground_executor().spawn(async move {
                let _ = rx.await;
            }),
            Err(_) => Task::ready(()),
        }
    }

    fn render_item(&self, ix: IndexPath, _item: &GkItem, _checked: bool, window: &mut Window, cx: &mut App) -> Option<AnyElement> {
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

/// Selection constraints enforced in `on_will_change`.
#[derive(Clone, Default)]
struct Rules {
    min: usize,
    max: usize,
    /// Values that cannot be removed once present in `before`.
    locked: Vec<SharedString>,
    /// Values that can never enter `selection`.
    blocked: Vec<SharedString>,
}

impl Rules {
    /// Applies blocked/locked value rules and rolls back to `before` if `min`
    /// or `max` bounds (when non-zero) are violated.
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
    /// Active selection rules keyed by source handle.
    static RULES: RefCell<HashMap<u64, Rules>> = RefCell::new(HashMap::new());
}

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
        // Prune rules belonging to sources that have been released.
        m.retain(|s, _| source::count(*s, 0).is_ok());
        m.insert(src, Rules { min, max, locked, blocked });
    });
    Ok(())
}

/// Dispatches `C_SEARCH_ITEMS` and `C_SEARCH_RULES`, falling through to `shim_text::call`.
pub fn call(op: u16, r: &mut R, _c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_SEARCH_ITEMS => (|| {
            let src = r.u64();
            let items = read_items(r);
            source::count(src, 0)?;
            ITEMS.with(|m| {
                let mut m = m.borrow_mut();
                // Drop staged item sets for released sources.
                m.retain(|s, _| source::count(*s, 0).is_ok());
                m.insert(src, items);
            });
            Ok(())
        })(),
        C_SEARCH_RULES => search_rules(r),
        _ => return crate::shim_text::call(op, r, _c),
    })
}

/// Converts an optional `AnyView` into an `AnyElement`, defaulting to an empty `div`.
pub fn view_element(v: &Option<gpui_kit::AnyView>) -> AnyElement {
    match v {
        Some(v) => v.clone().into_any_element(),
        None => gpui_kit::div().into_any_element(),
    }
}
