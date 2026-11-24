//! Virtualized list, table, and tree delegates backed by row `Source`s.
//!
//! Host code populates row tapes lazily through `crate::source`. This module
//! adapts those row sources to gpui-kit's `ListDelegate`, `TableDelegate`, and
//! `TreeState` callbacks, handling:
//! - **Lists (`GkListDelegate`)**: section headers (`HEADER_ROW`) and footers
//!   (`HEADER_ROW + 1`), custom empty/loading/initial auxiliary views mounted via
//!   [`adopt`], host-driven async search (`LE_SEARCH` via `source::request`),
//!   and infinite-scroll pagination (`LE_LOAD_MORE`).
//! - **Tables (`GkTableDelegate`) and `N_TABLE_CELLS`**:
//!   - Every row tape encodes an `N_TABLE_CELLS` root containing one child node
//!     per logical column. `Tape::parse` pre-decodes plain `Label` and `N_TEXT`
//!     cells into `Tape::table_cells` so `render_td` avoids creating a `Dx` per
//!     cell on scroll.
//!   - `render_tr` resets the single-row cache (`CurRow`) and invokes `bind_ahead`;
//!     the first `render_td` call for that row fetches the `Rc<Tape>` once and all
//!     remaining columns in that row reuse it directly.
//!   - Visual column reordering updates `order: Vec<usize>` (visual position ->
//!     logical column ID) so `render_td`, `cell_text`, `fit_column`, and
//!     `perform_sort` always address the host's original logical column index.
//!   - `bind_ahead` remembers `(rows_epoch, covered_range)` in `asked` and merges
//!     overlapping prefetch ranges within the same epoch, avoiding redundant calls
//!     into `source::table_bind_ahead` while the viewport stays within the 2x margin.
//!   - Per-row context menus wrap a cell (or list/tree row root) in `N_ROW_MENU(child, menu)`:
//!     normal rendering unwraps `child`, while `context_menu` locates `N_ROW_MENU`
//!     and applies its popup-menu ops.
//! - **Trees**: [`build_tree`] reconstructs nested `TreeItem`s from a flat pre-order
//!   `(id, label, depth, flags)` wire stream, while [`tree_item`] and [`tree_menu`]
//!   bind decoded row sources to `Tree` callbacks.

use crate::generated;
use crate::genapi::{self, Call, Dx, GkErr, GkResult, Put, R};
use crate::ops::*;
use crate::rt;
use crate::source::{self, HEADER_ROW};
use crate::tape::{TableCell, Tape};
use gpui_kit::component::label::Label;
use gpui_kit::component::list::{ListDelegate, ListItem, ListState};
use gpui_kit::component::skeleton::Skeleton;
use gpui_kit::component::table::{Column, ColumnFixed, ColumnGroup, ColumnSort, TableDelegate, TableState};
use gpui_kit::component::tree::{TreeEntry, TreeItem, TreeState};
use gpui_kit::component::{ActiveTheme, Icon, IconName, IndexPath, h_flex, v_flex};
use gpui_kit::{
    AnyElement, AnyView, App, Context, Div, Edges, InteractiveElement, IntoElement, ParentElement, Pixels,
    SharedString, Stateful, StyleRefinement, Styled, Task, TextAlign, Window, div, px,
};
use std::cell::RefCell;
use std::collections::HashMap;
use std::ops::Range;
use std::rc::Rc;

/// Shared display and pagination options applied to either `GkListDelegate` or
/// `GkTableDelegate` via `C_ROWS_DATA`.
#[derive(Clone)]
pub struct Extras {
    /// Whether the list or table is currently showing its loading state.
    loading: bool,
    /// Whether additional pages can be loaded via `load_more`.
    has_more: bool,
    /// Remaining-row threshold at which `load_more` fires.
    threshold: usize,
    /// Optional host view handles for empty, loading, and initial states (`0` = default).
    empty: u64,
    loading_view: u64,
    initial: u64,
    /// When true, emit `TE_VISIBLE_ROWS` and `TE_VISIBLE_COLUMNS` events to the host.
    visible_events: bool,
    /// Fallback height in pixels for uncached list row placeholders.
    placeholder_h: f32,
    /// Enable section headers (`HEADER_ROW`) and footers (`HEADER_ROW + 1`) on lists.
    headers: bool,
    footers: bool,
}

impl Default for Extras {
    fn default() -> Self {
        Extras {
            loading: false,
            has_more: false,
            threshold: 20,
            empty: 0,
            loading_view: 0,
            initial: 0,
            visible_events: false,
            placeholder_h: 0.,
            headers: false,
            footers: false,
        }
    }
}

/// Resolves an optional view handle (`0` means absent) into an `AnyView`.
fn app_view(h: u64) -> Option<AnyView> {
    if h == 0 {
        return None;
    }
    rt::with(|s| s.view(h)).ok().map(Into::into)
}

/// Returns the window handle that owns row source `src`.
fn source_window(src: u64) -> Option<u64> {
    rt::with(|s| s.source_mut(src).ok().map(|x| x.window))
}

/// Returns the item count for `section` in `src`, aborting the app if `src`
/// was already released while still displayed.
fn count(src: u64, section: u32) -> usize {
    match source::count(src, section) {
        Ok(n) => n,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "row source"),
    }
}

// Wire event kind tags sent on `SHAPE_LIST_EVENT`.
const LE_SELECT: u16 = 0;
const LE_RIGHT_CLICK: u16 = 1;
const LE_CONFIRM: u16 = 2;
const LE_CANCEL: u16 = 3;
const LE_LOAD_MORE: u16 = 4;
const LE_SEARCH: u16 = 5;

fn index(ix: Option<IndexPath>) -> Option<(usize, usize, usize)> {
    ix.map(|i| (i.section, i.row, i.column))
}

fn list_event(src: u64, kind: u16, ix: Option<IndexPath>, secondary: bool, cx: &mut App) {
    source::event(src, SHAPE_LIST_EVENT, &(0u64, (kind, index(ix), secondary, "")), cx);
}

/// `ListDelegate` backed by a row `Source` identified by `src`.
pub struct GkListDelegate {
    pub src: u64,
    x: Extras,
}

impl GkListDelegate {
    /// Creates a list delegate bound to row source `src` and marks the source
    /// as tape-decoded (rows are parsed as `ListItem` tapes rather than views).
    pub fn from_source(src: u64) -> GkResult<Self> {
        source::count(src, 0)?;
        source::set_decoded(src);
        Ok(GkListDelegate {
            src,
            x: Extras::default(),
        })
    }
}

/// Decodes a cached row tape into a `ListItem`, unwrapping `N_ROW_MENU` if
/// present, or returns a blank placeholder item of height `h` while loading.
fn row_item(src: u64, key: u64, h: f32, window: &mut Window, cx: &mut App) -> ListItem {
    if let Some((tape, view)) = source::row_tape(src, key, window, cx) {
        let item = genapi::with_tape(&tape, view, window, cx, |dx: &mut Dx, root| {
            let root = menu_child(dx, root);
            generated::build_component_list_item(dx, root)
        });
        if let Some(item) = item {
            return item;
        }
    }
    ListItem::new(("gk-row-placeholder", key as usize)).child(div().h(px(h)))
}

impl genapi::Wire for GkListDelegate {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.src);
    }
}

impl genapi::Wire for GkTableDelegate {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.src);
    }
}

impl genapi::Wire for &mut GkListDelegate {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.src);
    }
}

impl genapi::Wire for &mut GkTableDelegate {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.src);
    }
}

impl ListDelegate for GkListDelegate {
    type Item = ListItem;

    fn perform_search(
        &mut self,
        query: &str,
        _: &mut Window,
        cx: &mut Context<ListState<Self>>,
    ) -> Task<()> {
        let payload = (LE_SEARCH, index(None), false, query);
        match source::request(self.src, SHAPE_LIST_EVENT, &payload, cx) {
            Ok(rx) => cx.foreground_executor().spawn(async move {
                let _ = rx.await;
            }),
            Err(_) => Task::ready(()),
        }
    }

    fn sections_count(&self, _: &App) -> usize {
        let n = rt::with(|s| s.source_mut(self.src).map(|x| x.counts.len()).ok());
        n.unwrap_or(1).max(1)
    }

    fn items_count(&self, section: usize, _: &App) -> usize {
        count(self.src, section as u32)
    }

    fn render_item(
        &mut self,
        ix: IndexPath,
        window: &mut Window,
        cx: &mut Context<ListState<Self>>,
    ) -> Option<ListItem> {
        source::watch(self.src, cx.entity_id());
        let key = source::key(ix.section as u32, ix.row as u32);
        Some(row_item(self.src, key, self.x.placeholder_h, window, cx))
    }

    fn render_section_header(
        &mut self,
        section: usize,
        window: &mut Window,
        cx: &mut Context<ListState<Self>>,
    ) -> Option<impl IntoElement> {
        if !self.x.headers {
            return None;
        }
        let key = source::key(section as u32, HEADER_ROW as u32);
        Some(row_item(self.src, key, self.x.placeholder_h, window, cx))
    }

    fn render_section_footer(
        &mut self,
        section: usize,
        window: &mut Window,
        cx: &mut Context<ListState<Self>>,
    ) -> Option<impl IntoElement> {
        if !self.x.footers {
            return None;
        }
        let key = source::key(section as u32, HEADER_ROW as u32 + 1);
        Some(row_item(self.src, key, self.x.placeholder_h, window, cx))
    }

    fn render_empty(
        &mut self,
        _: &mut Window,
        cx: &mut Context<ListState<Self>>,
    ) -> impl IntoElement {
        match app_view(self.x.empty) {
            Some(v) => v.into_any_element(),
            None => empty_default(cx),
        }
    }

    fn render_initial(
        &mut self,
        _: &mut Window,
        _: &mut Context<ListState<Self>>,
    ) -> Option<AnyElement> {
        app_view(self.x.initial).map(IntoElement::into_any_element)
    }

    fn loading(&self, _: &App) -> bool {
        self.x.loading
    }

    fn render_loading(
        &mut self,
        _: &mut Window,
        _: &mut Context<ListState<Self>>,
    ) -> impl IntoElement {
        match app_view(self.x.loading_view) {
            Some(v) => v.into_any_element(),
            None => list_loading_default(),
        }
    }

    fn set_selected_index(
        &mut self,
        ix: Option<IndexPath>,
        _: &mut Window,
        cx: &mut Context<ListState<Self>>,
    ) {
        list_event(self.src, LE_SELECT, ix, false, cx);
    }

    fn set_right_clicked_index(
        &mut self,
        ix: Option<IndexPath>,
        _: &mut Window,
        cx: &mut Context<ListState<Self>>,
    ) {
        list_event(self.src, LE_RIGHT_CLICK, ix, false, cx);
    }

    fn confirm(&mut self, secondary: bool, _: &mut Window, cx: &mut Context<ListState<Self>>) {
        list_event(self.src, LE_CONFIRM, None, secondary, cx);
    }

    fn cancel(&mut self, _: &mut Window, cx: &mut Context<ListState<Self>>) {
        list_event(self.src, LE_CANCEL, None, false, cx);
    }

    fn has_more(&self, _: &App) -> bool {
        self.x.has_more
    }

    fn load_more_threshold(&self) -> usize {
        self.x.threshold
    }

    fn load_more(&mut self, _: &mut Window, cx: &mut Context<ListState<Self>>) {
        list_event(self.src, LE_LOAD_MORE, None, false, cx);
    }
}

/// Default empty-state view when the host has not supplied `Extras::empty`.
fn empty_default(cx: &App) -> AnyElement {
    h_flex()
        .size_full()
        .justify_center()
        .text_color(cx.theme().muted_foreground.opacity(0.6))
        .child(Icon::new(IconName::Inbox).size_12())
        .into_any_element()
}

/// Default skeleton placeholder when the host has not supplied `Extras::loading_view`.
fn list_loading_default() -> AnyElement {
    let item = || {
        ListItem::new("skeleton").disabled(true).child(
            v_flex()
                .gap_1p5()
                .overflow_hidden()
                .child(Skeleton::new().h_5().w_48().max_w_full())
                .child(Skeleton::new().secondary().h_3().w_64().max_w_full()),
        )
    };
    v_flex()
        .py_2p5()
        .gap_3()
        .child(item())
        .child(item())
        .child(item())
        .into_any_element()
}

// Wire event kind tags sent on `SHAPE_TABLE_EVENT`.
const TE_SORT: u16 = 0;
const TE_MOVE_COLUMN: u16 = 1;
const TE_VISIBLE_ROWS: u16 = 2;
const TE_VISIBLE_COLUMNS: u16 = 3;
const TE_LOAD_MORE: u16 = 4;

fn table_event(src: u64, kind: u16, row: usize, col: usize, to: usize, sort: u16, cx: &mut App) {
    let payload = (0u64, (kind, row as u64, col as u64, to as u64), sort);
    source::event(src, SHAPE_TABLE_EVENT, &payload, cx);
}

fn sort_num(s: ColumnSort) -> u16 {
    match s {
        ColumnSort::Default => 0,
        ColumnSort::Ascending => 1,
        ColumnSort::Descending => 2,
    }
}

/// Single-row tape cache populated on the first `render_td` call for a row and
/// reused across all remaining columns in that row.
enum CurRow {
    Empty,
    Missing(usize),
    Found(usize, Rc<Tape>, u64),
}

/// `TableDelegate` backed by a row `Source` where each row tape encodes an
/// `N_TABLE_CELLS` container of per-column cells.
pub struct GkTableDelegate {
    pub src: u64,
    /// Column definitions indexed by logical column id.
    columns: Vec<Column>,
    /// Maps visual column position `col_ix` to logical column id in `columns`.
    order: Vec<usize>,
    x: Extras,
    watched: bool,
    /// Last `(rows_epoch, prefetched_range)` requested via `bind_ahead`.
    asked: (u64, Range<usize>),
    cur_row: CurRow,
    look: Look,
}

/// Optional visual and accessibility metadata configured by `C_TABLE_TEXTS`,
/// `C_TABLE_ROW_LABELS`, and `C_TABLE_LOOK`.
#[derive(Default)]
struct Look {
    /// Plain cell text indexed by `(row_ix, logical_col_id)` for copy/a11y.
    texts: HashMap<(usize, usize), SharedString>,
    /// Explicit per-row accessible labels overriding the joined cell labels.
    row_labels: HashMap<usize, SharedString>,
    /// Preferred auto-fit column widths keyed by logical column id.
    fit: HashMap<usize, f32>,
    /// Multi-level column group headers, if configured.
    groups: Option<Vec<Vec<ColumnGroup>>>,
    /// Optional custom views for column group header cells, keyed by label.
    group_views: HashMap<SharedString, u64>,
    /// Optional custom view handle for the trailing empty column.
    last_col: u64,
    header_style: Option<StyleRefinement>,
    row_style: Option<StyleRefinement>,
}

impl GkTableDelegate {
    /// Creates a table delegate for `src`, adopting any columns staged earlier
    /// by `C_TABLE_SOURCE_COLUMNS` or `source_columns`.
    pub fn from_source(src: u64) -> GkResult<Self> {
        source::count(src, 0)?;
        source::set_decoded(src);
        let mut d = GkTableDelegate {
            src,
            columns: Vec::new(),
            order: Vec::new(),
            x: Extras::default(),
            watched: false,
            asked: (0, 0..0),
            cur_row: CurRow::Empty,
            look: Look::default(),
        };
        d.set_columns(SOURCE_COLUMNS.with(|m| m.borrow_mut().remove(&src)).unwrap_or_default());
        Ok(d)
    }

    fn set_columns(&mut self, columns: Vec<Column>) {
        self.order = (0..columns.len()).collect();
        self.columns = columns;
        self.cur_row = CurRow::Empty;
    }

    /// Translates a visual column position into its logical column id.
    fn id(&self, pos: usize) -> usize {
        self.order.get(pos).copied().unwrap_or(pos)
    }

    fn row_key(&self, row: usize) -> u64 {
        source::key(0, row as u32)
    }

    /// Prefetches row tapes around `rows` with a 2x margin (minimum 24 rows)
    /// whenever the current `rows_epoch` or cached window no longer covers it.
    fn bind_ahead(&mut self, rows: Range<usize>, window: &mut Window, cx: &mut Context<TableState<Self>>) {
        if self.columns.is_empty() || rows.is_empty() {
            return;
        }
        let epoch = source::rows_epoch();
        let len = rows.len().max(24);
        let behind = rows.start.saturating_sub(2 * len);
        let ahead = rows.end + 2 * len;
        if self.asked.0 == epoch && self.asked.1.start <= behind && ahead <= self.asked.1.end {
            return;
        }
        if let Some(asked) = source::table_bind_ahead(self.src, rows, window, cx) {
            // Merge overlapping or adjacent prefetch ranges within the same epoch.
            if self.asked.0 == asked.0 && asked.1.start <= self.asked.1.end && self.asked.1.start <= asked.1.end {
                self.asked.1 = self.asked.1.start.min(asked.1.start)..self.asked.1.end.max(asked.1.end);
            } else {
                self.asked = asked;
            }
        }
    }

    /// Registers the table entity as an observer of `self.src` once.
    fn watch(&mut self, cx: &mut Context<TableState<Self>>) {
        if !self.watched {
            self.watched = true;
            source::watch(self.src, cx.entity_id());
        }
    }
}

impl TableDelegate for GkTableDelegate {
    fn columns_count(&self, _: &App) -> usize {
        self.columns.len()
    }

    fn rows_count(&self, _: &App) -> usize {
        if self.columns.is_empty() { 0 } else { count(self.src, 0) }
    }

    fn column(&self, col_ix: usize, _: &App) -> Column {
        self.columns.get(self.id(col_ix)).cloned().unwrap_or_default()
    }

    fn perform_sort(
        &mut self,
        col_ix: usize,
        sort: ColumnSort,
        _: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) {
        if let Some(c) = self.order.get(col_ix).and_then(|id| self.columns.get_mut(*id)) {
            c.sort = Some(sort);
        }
        table_event(self.src, TE_SORT, 0, self.id(col_ix), 0, sort_num(sort), cx);
    }

    fn render_th(
        &mut self,
        col_ix: usize,
        _: &mut Window,
        _: &mut Context<TableState<Self>>,
    ) -> impl IntoElement {
        let name = self.columns.get(self.id(col_ix)).map(|c| c.name.clone()).unwrap_or_default();
        div().w_full().child(name)
    }

    fn render_td(
        &mut self,
        row_ix: usize,
        col_ix: usize,
        window: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) -> impl IntoElement {
        let need_fetch = match &self.cur_row {
            CurRow::Found(r, _, _) | CurRow::Missing(r) => *r != row_ix,
            CurRow::Empty => true,
        };
        if need_fetch {
            self.watch(cx);
            self.cur_row = match source::row_tape(self.src, self.row_key(row_ix), window, cx) {
                Some((tape, view)) => CurRow::Found(row_ix, tape, view),
                None => CurRow::Missing(row_ix),
            };
        }
        let CurRow::Found(_, tape, view) = &self.cur_row else {
            return div().into_any_element();
        };
        let col_id = self.id(col_ix);
        if let Some(cells) = &tape.table_cells {
            if let Some(cell) = cells.get(col_id) {
                // Fast path: plain Label/Text cells skip full tape decoding.
                return match cell {
                    TableCell::Label(s) => genapi::timed_decode(|| Label::new(s.clone()).into_any_element()),
                    TableCell::Text(s) => genapi::timed_decode(|| s.clone().into_any_element()),
                    TableCell::Node(node) => genapi::with_tape_node(tape, *view, *node, window, cx, |dx: &mut Dx, n| dx.child(n)),
                };
            }
        }
        div().into_any_element()
    }

    fn move_column(
        &mut self,
        col_ix: usize,
        to_ix: usize,
        _: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) {
        if col_ix < self.order.len() && to_ix < self.order.len() {
            let id = self.order.remove(col_ix);
            self.order.insert(to_ix, id);
            table_event(self.src, TE_MOVE_COLUMN, 0, id, to_ix, 0, cx);
        }
    }

    fn render_empty(
        &mut self,
        _: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) -> impl IntoElement {
        match app_view(self.x.empty) {
            Some(v) => v.into_any_element(),
            None => empty_default(cx),
        }
    }

    fn loading(&self, _: &App) -> bool {
        self.x.loading
    }

    fn render_loading(
        &mut self,
        _: gpui_kit::component::Size,
        _: &mut Window,
        _: &mut Context<TableState<Self>>,
    ) -> impl IntoElement {
        match app_view(self.x.loading_view) {
            Some(v) => v.into_any_element(),
            None => div().size_full().into_any_element(),
        }
    }

    fn has_more(&self, _: &App) -> bool {
        self.x.has_more
    }

    fn load_more_threshold(&self) -> usize {
        self.x.threshold
    }

    fn load_more(&mut self, _: &mut Window, cx: &mut Context<TableState<Self>>) {
        table_event(self.src, TE_LOAD_MORE, 0, 0, 0, 0, cx);
    }

    fn cell_text(&self, row_ix: usize, col_ix: usize, _: &App) -> String {
        self.look.texts.get(&(row_ix, self.id(col_ix))).map(|t| t.to_string()).unwrap_or_default()
    }

    fn cell_label(&self, row_ix: usize, col_ix: usize, _: &App) -> Option<SharedString> {
        self.look.texts.get(&(row_ix, self.id(col_ix))).filter(|t| !t.is_empty()).cloned()
    }

    fn row_label(&self, row_ix: usize, cx: &App) -> Option<SharedString> {
        if let Some(l) = self.look.row_labels.get(&row_ix) {
            return Some(l.clone());
        }
        // Fall back to joining non-empty cell labels in visual column order.
        let parts: Vec<SharedString> = (0..self.columns_count(cx)).filter_map(|c| self.cell_label(row_ix, c, cx)).collect();
        (!parts.is_empty()).then(|| parts.iter().map(|p| p.as_ref()).collect::<Vec<&str>>().join(", ").into())
    }

    fn fit_column(&mut self, col_ix: usize, _: &mut Window, _: &mut Context<TableState<Self>>) -> Option<Pixels> {
        self.look.fit.get(&self.id(col_ix)).map(|w| px(*w))
    }

    fn group_headers(&self, _: &App) -> Option<Vec<Vec<ColumnGroup>>> {
        self.look.groups.clone()
    }

    fn render_group_th(
        &mut self,
        label: &SharedString,
        _col_span: usize,
        width: Pixels,
        _: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) -> impl IntoElement {
        if let Some(v) = self.look.group_views.get(label).and_then(|v| app_view(*v)) {
            return div().w(width).h_full().flex_shrink_0().child(v).into_any_element();
        }
        div()
            .w(width)
            .h_full()
            .flex_shrink_0()
            .flex()
            .items_center()
            .justify_center()
            .border_r_1()
            .border_color(cx.theme().border)
            .child(label.clone())
            .into_any_element()
    }

    fn render_header(&mut self, _: &mut Window, _: &mut Context<TableState<Self>>) -> Stateful<Div> {
        let mut d = div().id("header");
        if let Some(st) = &self.look.header_style {
            *d.style() = generated::refined(d.style().clone(), st);
        }
        d
    }

    fn render_tr(&mut self, row_ix: usize, window: &mut Window, cx: &mut Context<TableState<Self>>) -> Stateful<Div> {
        self.watch(cx);
        // Reset the single-row cache before cells of `row_ix` are rendered.
        self.cur_row = CurRow::Empty;
        self.bind_ahead(row_ix..row_ix + 1, window, cx);
        let mut d = div().id(("row", row_ix));
        if let Some(st) = &self.look.row_style {
            *d.style() = generated::refined(d.style().clone(), st);
        }
        d
    }

    fn context_menu(
        &mut self,
        row_ix: usize,
        menu: gpui_kit::component::menu::PopupMenu,
        window: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) -> gpui_kit::component::menu::PopupMenu {
        table_row_menu(self.src, self.row_key(row_ix), menu, window, cx)
    }

    fn render_last_empty_col(&mut self, _: &mut Window, _: &mut Context<TableState<Self>>) -> impl IntoElement {
        match app_view(self.look.last_col) {
            Some(v) => h_flex().h_full().flex_shrink_0().child(v).into_any_element(),
            None => h_flex().w_3().h_full().flex_shrink_0().into_any_element(),
        }
    }

    fn visible_rows_changed(
        &mut self,
        visible: Range<usize>,
        window: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) {
        self.bind_ahead(visible.clone(), window, cx);
        if self.x.visible_events {
            table_event(self.src, TE_VISIBLE_ROWS, visible.start, 0, visible.end, 0, cx);
        }
    }

    fn visible_columns_changed(
        &mut self,
        visible: Range<usize>,
        _: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) {
        if self.x.visible_events {
            table_event(self.src, TE_VISIBLE_COLUMNS, 0, visible.start, visible.end, 0, cx);
        }
    }
}

/// Decodes a single `Column` definition from the wire stream.
fn read_column(r: &mut R) -> Column {
    let key = SharedString::from(r.str().to_owned());
    let name = SharedString::from(r.str().to_owned());
    let align = match r.u16() {
        1 => TextAlign::Center,
        2 => TextAlign::Right,
        _ => TextAlign::Left,
    };
    let sort = (r.u8() != 0).then(|| match r.u16() {
        1 => ColumnSort::Ascending,
        2 => ColumnSort::Descending,
        _ => ColumnSort::Default,
    });
    let paddings = (r.u8() != 0).then(|| {
        let (top, right, bottom, left) = (r.f32(), r.f32(), r.f32(), r.f32());
        Edges {
            top: px(top),
            right: px(right),
            bottom: px(bottom),
            left: px(left),
        }
    });
    let width = px(r.f32());
    let fixed = (r.u8() != 0).then(|| match r.u16() {
        1 => ColumnFixed::Right,
        _ => ColumnFixed::Left,
    });
    let (resizable, movable, selectable) = (r.u8() != 0, r.u8() != 0, r.u8() != 0);
    let (min_width, max_width) = (px(r.f32()), px(r.f32()));
    Column {
        key,
        name,
        align,
        sort,
        paddings,
        width,
        fixed,
        resizable,
        movable,
        selectable,
        min_width,
        max_width,
    }
}

fn read_columns(r: &mut R) -> Vec<Column> {
    let n = r.u32();
    (0..n).map(|_| read_column(r)).collect()
}

/// Stages columns for row source `src` prior to `GkTableDelegate::from_source`.
pub(crate) fn source_columns(src: u64, columns: Vec<Column>) -> GkResult<()> {
    source::count(src, 0)?;
    SOURCE_COLUMNS.with(|m| m.borrow_mut().insert(src, columns));
    Ok(())
}

/// Replaces columns on an existing `TableState<GkTableDelegate>` entity and refreshes it.
pub(crate) fn state_columns(h: u64, c: &mut genapi::Call, columns: Vec<Column>) -> GkResult<()> {
    genapi::state_update_cx::<TableOf>(h, c, move |s, cx| {
        s.delegate_mut().set_columns(columns);
        s.refresh(cx);
        cx.notify();
    })
}

thread_local! {
    /// Staging map holding initial columns set on a source before the table
    /// delegate is constructed.
    static SOURCE_COLUMNS: RefCell<HashMap<u64, Vec<Column>>> = RefCell::new(HashMap::new());
}

/// Builds a tree row-renderer closure backed by row source `src`.
pub fn tree_item(
    src: u64,
) -> impl Fn(usize, &TreeEntry, bool, &mut Window, &mut App) -> ListItem + 'static {
    source::set_decoded(src);
    move |ix, _entry, _selected, window, cx| {
        source::watch(src, window.current_view());
        row_item(src, source::key(0, ix as u32), 0., window, cx)
    }
}

/// Decodes flat pre-order tree rows: `(id: str, label: str, depth: u32, flags: u8)`.
fn read_flat(r: &mut R) -> Vec<(String, String, u32, u8)> {
    let n = r.u32();
    (0..n).map(|_| (r.str().to_owned(), r.str().to_owned(), r.u32(), r.u8())).collect()
}

/// Reconstructs nested `TreeItem` hierarchies from a flat pre-order slice of
/// `(id, label, depth, flags)` tuples (`flags & 1` = expanded, `flags & 2` = disabled).
fn build_tree(flat: &[(String, String, u32, u8)]) -> Vec<TreeItem> {
    fn level(flat: &[(String, String, u32, u8)], at: &mut usize, depth: u32) -> Vec<TreeItem> {
        let mut out = Vec::new();
        while *at < flat.len() && flat[*at].2 == depth {
            let (id, label, _, flags) = &flat[*at];
            *at += 1;
            let mut item = TreeItem::new(id.clone(), label.clone())
                .expanded(flags & 1 != 0)
                .disabled(flags & 2 != 0);
            if *at < flat.len() && flat[*at].2 > depth {
                item = item.children(level(flat, at, depth + 1));
            }
            out.push(item);
        }
        out
    }
    let mut at = 0;
    let mut out = level(flat, &mut at, 0);
    // Recover gracefully if the host omitted depth 0 or jumped depths.
    while at < flat.len() {
        let d = flat[at].2;
        out.extend(level(flat, &mut at, d));
    }
    out
}

/// Decodes helper tape nodes `N_ROW_MENU` (renders its inner child, ignoring
/// the attached menu subtree) and `N_TABLE_CELLS` (empty fallback if rendered directly).
pub fn node(dx: &mut Dx, ty: u16, args: u32, _first: u32) -> Option<AnyElement> {
    match ty {
        N_ROW_MENU => {
            let child = dx.args(args).u32();
            Some(dx.child(child))
        }
        N_TABLE_CELLS => Some(div().into_any_element()),
        _ => None,
    }
}

/// Populates a table row's context menu by scanning its `N_TABLE_CELLS` children
/// for the first `N_ROW_MENU` wrapper and applying its popup-menu ops.
fn table_row_menu(
    src: u64,
    key: u64,
    menu: gpui_kit::component::menu::PopupMenu,
    window: &mut Window,
    cx: &mut App,
) -> gpui_kit::component::menu::PopupMenu {
    let Some((tape, view)) = source::row_tape(src, key, window, cx) else {
        return menu;
    };
    genapi::with_tape(&tape, view, window, cx, move |dx: &mut Dx, root| {
        let (ty, args, _) = dx.node(root);
        if ty != N_TABLE_CELLS {
            return menu;
        }
        let mut a = dx.args(args);
        let n = a.u32();
        for _ in 0..n {
            let cell = a.u32();
            let (cty, cargs, _) = dx.node(cell);
            if cty == N_ROW_MENU {
                let mut ca = dx.args(cargs);
                let (_, m) = (ca.u32(), ca.u32());
                let (_, _, first) = dx.node(m);
                return generated::apply_component_popup_menu(dx, menu, first);
            }
        }
        menu
    })
}

/// Unwraps `N_ROW_MENU` to return its inner row element node id.
fn menu_child(dx: &mut Dx, root: u32) -> u32 {
    let (ty, args, _) = dx.node(root);
    if ty == N_ROW_MENU { dx.args(args).u32() } else { root }
}

/// Populates a `PopupMenu` from the first key in `keys` whose row tape has an
/// `N_ROW_MENU` root node.
pub fn row_menu(
    src: u64,
    keys: impl IntoIterator<Item = u64>,
    menu: gpui_kit::component::menu::PopupMenu,
    window: &mut Window,
    cx: &mut App,
) -> gpui_kit::component::menu::PopupMenu {
    let mut menu = menu;
    for key in keys {
        let Some((tape, view)) = source::row_tape(src, key, window, cx) else {
            continue;
        };
        let got = genapi::with_tape(&tape, view, window, cx, move |dx: &mut Dx, root| {
            let (ty, args, _) = dx.node(root);
            if ty != N_ROW_MENU {
                return Err(menu);
            }
            let mut a = dx.args(args);
            let (_, m) = (a.u32(), a.u32());
            let (_, _, first) = dx.node(m);
            Ok(generated::apply_component_popup_menu(dx, menu, first))
        });
        match got {
            Ok(m) => return m,
            Err(m) => menu = m,
        }
    }
    menu
}

/// Builds a tree context-menu callback backed by row source `src`.
pub fn tree_menu(
    src: u64,
) -> impl Fn(usize, &TreeEntry, gpui_kit::component::menu::PopupMenu, &mut Window, &mut Context<'_, TreeState>) -> gpui_kit::component::menu::PopupMenu + 'static {
    move |ix, _entry, menu, window, cx| row_menu(src, [source::key(0, ix as u32)], menu, window, cx)
}

/// Mounts an auxiliary view handle into the window owning `src` and marks it
/// dirty if it has not yet received an initial tape.
fn adopt(view: u64, src: u64, c: &mut Call) {
    if view == 0 {
        return;
    }
    let Some(wh) = source_window(src) else { return };
    let fresh = rt::with(|s| {
        s.mount(view, wh);
        s.view(view).ok()
    });
    if let Some(entity) = fresh {
        if entity.read(c.cx).tape.is_none() {
            let window = c.window_opt();
            rt::mark_dirty(view, window, c.cx);
        }
    }
}

/// Decodes `Extras` from `C_ROWS_DATA` (`flags: u8`, `threshold: u64`,
/// `empty: u64`, `loading_view: u64`, `initial: u64`, `placeholder_h: f32`).
fn read_extras(r: &mut R) -> Extras {
    let flags = r.u8();
    Extras {
        loading: flags & 1 != 0,
        has_more: flags & 2 != 0,
        visible_events: flags & 4 != 0,
        headers: flags & 8 != 0,
        footers: flags & 16 != 0,
        threshold: r.u64() as usize,
        empty: r.u64(),
        loading_view: r.u64(),
        initial: r.u64(),
        placeholder_h: r.f32(),
    }
}

type ListOf = ListState<GkListDelegate>;
type TableOf = TableState<GkTableDelegate>;

/// Dispatches list, table, and tree call opcodes, falling through to `shim_select::call`.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_ROWS_DATA => (|| {
            // Applies `Extras` to either a `ListState` or `TableState` handle.
            let h = r.u64();
            let x = read_extras(r);
            if let Ok(list) = genapi::state::<ListOf>(h) {
                let src = list.read(c.cx).delegate().src;
                for v in [x.empty, x.loading_view, x.initial] {
                    adopt(v, src, c);
                }
                return genapi::state_update_cx::<ListOf>(h, c, move |s, cx| {
                    s.delegate_mut().x = x;
                    cx.notify();
                });
            }
            let table = genapi::state::<TableOf>(h)?;
            let src = table.read(c.cx).delegate().src;
            for v in [x.empty, x.loading_view, x.initial] {
                adopt(v, src, c);
            }
            genapi::state_update_cx::<TableOf>(h, c, move |s, cx| {
                s.delegate_mut().x = x;
                cx.notify();
            })
        })(),
        C_TABLE_SOURCE_COLUMNS => (|| {
            // Stages initial columns on a source before TableState creation.
            let src = r.u64();
            let columns = read_columns(r);
            source::count(src, 0)?;
            SOURCE_COLUMNS.with(|m| m.borrow_mut().insert(src, columns));
            Ok(())
        })(),
        C_TABLE_SET_COLUMNS => (|| {
            // Replaces columns on an existing TableState and refreshes layout.
            let h = r.u64();
            let columns = read_columns(r);
            genapi::state_update_cx::<TableOf>(h, c, move |s, cx| {
                s.delegate_mut().set_columns(columns);
                s.refresh(cx);
                cx.notify();
            })
        })(),
        C_TABLE_TEXTS => (|| {
            // Bulk-updates plain cell text for rows `lo .. lo + rows` across `0 .. cols`.
            let (h, replace, lo) = (r.u64(), r.u8() != 0, r.u64() as usize);
            let (rows, cols) = (r.u32() as usize, r.u32() as usize);
            let mut cells = Vec::with_capacity((rows * cols).min(1 << 16));
            for row in 0..rows {
                for col in 0..cols {
                    cells.push(((lo + row, col), SharedString::from(r.str().to_owned())));
                }
            }
            genapi::state_update_cx::<TableOf>(h, c, move |s, cx| {
                let texts = &mut s.delegate_mut().look.texts;
                if replace {
                    texts.clear();
                }
                for (k, t) in cells {
                    if t.is_empty() {
                        texts.remove(&k);
                    } else {
                        texts.insert(k, t);
                    }
                }
                cx.notify();
            })
        })(),
        C_TABLE_ROW_LABELS => (|| {
            // Sets or clears per-row accessible labels (`empty` string removes the override).
            let h = r.u64();
            let n = r.u32() as usize;
            let mut labels = Vec::with_capacity(n.min(1 << 16));
            for _ in 0..n {
                labels.push((r.u64() as usize, SharedString::from(r.str().to_owned())));
            }
            genapi::state_update_cx::<TableOf>(h, c, move |s, cx| {
                let map = &mut s.delegate_mut().look.row_labels;
                for (row, l) in labels {
                    if l.is_empty() {
                        map.remove(&row);
                    } else {
                        map.insert(row, l);
                    }
                }
                cx.notify();
            })
        })(),
        C_TABLE_LOOK => (|| {
            // Updates table fit widths, group headers, custom views, and
            // optional header/row `StyleRefinement` mini-tapes.
            let h = r.u64();
            let fit: HashMap<usize, f32> = (0..r.u32()).map(|_| (r.u32() as usize, r.f32())).collect();
            let has_groups = r.u8() != 0;
            let levels = r.u32();
            let mut groups = Vec::new();
            for _ in 0..levels {
                let n = r.u32();
                groups.push((0..n).map(|_| ColumnGroup::new(r.str().to_owned(), r.u64() as usize)).collect::<Vec<_>>());
            }
            let group_views: HashMap<SharedString, u64> =
                (0..r.u32()).map(|_| (SharedString::from(r.str().to_owned()), r.u64())).collect();
            let last_col = r.u64();
            let style = |r: &mut R, c: &mut Call| -> GkResult<Option<StyleRefinement>> {
                let n = r.u32() as usize;
                if n == 0 {
                    return Ok(None);
                }
                // Re-prefix the length because `value_arg` expects a length-prefixed slice.
                let mut whole = Vec::with_capacity(4 + n);
                whole.extend_from_slice(&(n as u32).to_le_bytes());
                whole.extend_from_slice(r.bytes(n));
                genapi::value_arg(c, &mut R::new(&whole), generated::build_gpui_style_refinement).map(Some)
            };
            let header_style = style(r, c)?;
            let row_style = style(r, c)?;
            let table = genapi::state::<TableOf>(h)?;
            let src = table.read(c.cx).delegate().src;
            for v in group_views.values().copied().chain([last_col]) {
                adopt(v, src, c);
            }
            genapi::state_update_cx::<TableOf>(h, c, move |s, cx| {
                let look = &mut s.delegate_mut().look;
                look.fit = fit;
                look.groups = has_groups.then_some(groups);
                look.group_views = group_views;
                look.last_col = last_col;
                look.header_style = header_style;
                look.row_style = row_style;
                s.refresh_header_layout(cx);
            })
        })(),
        C_TREE_SET_ITEMS => (|| {
            let h = r.u64();
            let items = build_tree(&read_flat(r));
            genapi::state_update_cx::<TreeState>(h, c, move |s, cx| s.set_items(items, cx))
        })(),
        C_TREE_ENTRY => (|| {
            // Reads visible entry `ix` from `TreeState`: present u8, id, label,
            // depth u64, flags u8 (1=folder, 2=expanded, 4=disabled).
            let (h, ix) = (r.u64(), r.u64() as usize);
            let bytes = genapi::state_read::<TreeState, _>(h, c, |s, _| {
                let mut out = Vec::with_capacity(48);
                match s.entry(ix) {
                    Some(e) => {
                        out.put_u8(1);
                        out.put_str(&e.item().id);
                        out.put_str(&e.item().label);
                        out.put_u64(e.depth() as u64);
                        let flags = e.is_folder() as u8 | (e.is_expanded() as u8) << 1 | (e.is_disabled() as u8) << 2;
                        out.put_u8(flags);
                    }
                    None => out.put_u8(0),
                }
                out
            })?;
            c.out.extend_from_slice(&bytes);
            Ok(())
        })(),

        _ => return crate::shim_select::call(op, r, c),
    })
}
