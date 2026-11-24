//! Delegate shims of the rows family: the kit's `ListDelegate`, `TableDelegate` and
//! the tree's render closure, each answered from a row source (docs/PROTOCOLS.md section 9.5;
//! the shared core is core/src/source.rs). Call ops 48 to 63, shapes 16 to 19. App code never
//! runs from here: a delegate method answers from data the app pushed, draws rows the app
//! described earlier (a placeholder for one frame when a row has none yet), and turns
//! everything else into a queued event or a request the app answers later.
//!
//! The states and elements over these shims are generated: `ListState<GkListDelegate>`,
//! `TableState<GkTableDelegate>` (overrides.toml `[instantiate] states`, constructed with
//! `from_source`), `List<GkListDelegate>`, `DataTable<GkTableDelegate>` (`[instantiate] types`),
//! the tree's closure (`[rows]`: `tree_item`) and `TreeState`. This file holds only what the
//! generator cannot write: the delegates themselves and the data ops.
//!
//! - **List** (`GkListDelegate`, `type Item = ListItem`): row `(section, row)` is key
//!   `section << 32 | row`; a section's header and footer are rows `HEADER_ROW` and
//!   `HEADER_ROW + 1` of it. A row's view describes a `ListItem`, decoded here into the value
//!   the list then marks selected, so selection styling stays the kit's.
//! - **Table** (`GkTableDelegate`): each cell is a view of its own, key `row * columns + id`
//!   in section 0, `id` the column's index in the columns the app gave (stable when the user
//!   moves a column). Cells are retained while the table scrolls; the rows beside the visible
//!   ones are bound ahead (`source::want`), so a steady scroll draws no placeholder.
//! - **Tree** (`tree_item`): row `ix` is the tree's flat entry `ix`; its view describes a
//!   `ListItem` and reads the entry with `TREE_ENTRY` (a dependency: the row is described again
//!   when the tree changes).

use crate::generated;
use crate::genapi::{self, Call, Dx, GkErr, GkResult, Put, R};
use crate::ops::*;
use crate::rt;
use crate::source::{self, HEADER_ROW, Row};
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

// --- what the app pushes besides rows ------------------------------------------------------

/// What a delegate shows or reports besides its rows: data the app pushed (`ROWS_DATA`).
#[derive(Clone)]
pub struct Extras {
    /// `loading()`: the kit shows the loading view instead of the rows.
    loading: bool,
    /// `has_more()` and `load_more_threshold()`: when the kit asks for more rows.
    has_more: bool,
    threshold: usize,
    /// Views the app made for the empty, loading and initial states; 0: upstream's.
    empty: u64,
    loading_view: u64,
    initial: u64,
    /// Tables: report the visible rows and columns as events (one per change while scrolling).
    visible_events: bool,
    /// Lists: the height of a placeholder row (the rows' own height).
    placeholder_h: f32,
    /// Lists: draw each section's header and footer row (rows HEADER_ROW and HEADER_ROW + 1).
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

/// A view the app gave as data, drawn by the kit: None when unset or released.
fn app_view(h: u64) -> Option<AnyView> {
    if h == 0 {
        return None;
    }
    rt::with(|s| s.view(h)).ok().map(Into::into)
}

/// The window a source draws in (a handle of kind WINDOW).
fn source_window(src: u64) -> Option<u64> {
    rt::with(|s| s.source_mut(src).ok().map(|x| x.window))
}

/// Rows in section `section` of a source a view still draws; a released source stops.
fn count(src: u64, section: u32) -> usize {
    match source::count(src, section) {
        Ok(n) => n,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "row source"),
    }
}

// --- lists -----------------------------------------------------------------------------------

/// List_Event kinds (shape LIST_EVENT): the `kind` field of the app's record.
const LE_SELECT: u16 = 0;
const LE_RIGHT_CLICK: u16 = 1;
const LE_CONFIRM: u16 = 2;
const LE_CANCEL: u16 = 3;
const LE_LOAD_MORE: u16 = 4;
const LE_SEARCH: u16 = 5;

/// An index as the List_Event record carries it (Base_Index_Path: section, row, column).
fn index(ix: Option<IndexPath>) -> Option<(usize, usize, usize)> {
    ix.map(|i| (i.section, i.row, i.column))
}

/// A list event, queued for the app: `(u64 id, u16 kind, opt<index>, bool secondary, str query)`.
fn list_event(src: u64, kind: u16, ix: Option<IndexPath>, secondary: bool, cx: &mut App) {
    source::event(src, SHAPE_LIST_EVENT, &(0u64, (kind, index(ix), secondary, "")), cx);
}

/// The `ListDelegate` over a row source.
pub struct GkListDelegate {
    pub src: u64,
    x: Extras,
}

impl GkListDelegate {
    pub fn from_source(src: u64) -> GkResult<Self> {
        source::count(src, 0)?;
        source::set_decoded(src);
        Ok(GkListDelegate {
            src,
            x: Extras::default(),
        })
    }
}

/// The row `key` of `src` as the `ListItem` its view describes, or a placeholder row of height
/// `h` while it has no view (one frame after a far jump).
fn row_item(src: u64, key: u64, h: f32, window: &mut Window, cx: &mut App) -> ListItem {
    if let Some((tape, view)) = source::row_tape(src, key, window, cx) {
        let item = genapi::with_tape(&tape, view, window, cx, |dx: &mut Dx, root| {
            let root = menu_child(dx, root); // a row with a menu (PROTOCOLS.md 11.24)
            generated::build_component_list_item(dx, root)
        });
        if let Some(item) = item {
            return item;
        }
    }
    ListItem::new(("gk-row-placeholder", key as usize)).child(div().h(px(h)))
}

/// A delegate read back (`ListState::delegate`, `TableState::delegate`): the row source it is
/// made over, which is what the app holds (docs/PROTOCOLS.md section 9.10).
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

// `delegate_mut` hands out `&mut D`.
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

    /// A request: the app answers it (`gk.answer`) after it has set the new counts.
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

/// Upstream's `render_empty` (ListDelegate and TableDelegate): an inbox icon.
fn empty_default(cx: &App) -> AnyElement {
    h_flex()
        .size_full()
        .justify_center()
        .text_color(cx.theme().muted_foreground.opacity(0.6))
        .child(Icon::new(IconName::Inbox).size_12())
        .into_any_element()
}

/// Upstream's list `Loading` (list/loading.rs, private there): three skeleton rows.
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

// --- tables ----------------------------------------------------------------------------------

/// Table_Delegate_Event kinds (shape TABLE_EVENT): the `kind` field of the app's record. The
/// table's own events (selection, double clicks, widths) are TableState's, subscribed to with
/// the generated `on_table_event`.
const TE_SORT: u16 = 0;
const TE_MOVE_COLUMN: u16 = 1;
const TE_VISIBLE_ROWS: u16 = 2;
const TE_VISIBLE_COLUMNS: u16 = 3;
const TE_LOAD_MORE: u16 = 4;

/// A table event, queued for the app: `(u64 id, u16 kind, u64 row, u64 col, u64 to, u16 sort)`.
/// Columns are the app's column ids.
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

/// The `TableDelegate` over a row source whose rows are the table's cells.
pub struct GkTableDelegate {
    pub src: u64,
    columns: Vec<Column>,
    /// Position to column id: the order the user has moved the columns into.
    order: Vec<usize>,
    x: Extras,
    watched: bool,
    /// The rows `bind_ahead` last asked for and the frame it asked in: they are bound before the
    /// next frame, so asking again for rows inside them in the same frame is skipped (every row
    /// drawn asks while the rows ahead are missing).
    asked: (u64, Range<usize>),
    /// What the app pushed besides cells (TABLE_TEXTS, TABLE_ROW_LABELS, TABLE_LOOK).
    look: Look,
}

/// A table's texts and looks, pushed by the app (docs/PROTOCOLS.md section 9.10). Columns are
/// the app's column ids (stable when the user moves a column).
#[derive(Default)]
struct Look {
    /// A cell's text: `cell_text` (copying), and its screen-reader label `cell_label`.
    texts: HashMap<(usize, usize), SharedString>,
    /// A row's screen-reader name; upstream's default (its cells' labels) when absent.
    row_labels: HashMap<usize, SharedString>,
    /// `fit_column`: the width a double click on the column's resize handle gives.
    fit: HashMap<usize, f32>,
    /// `group_headers`, levels from the top.
    groups: Option<Vec<Vec<ColumnGroup>>>,
    /// Views drawn as a group header cell, by the group's label (`render_group_th`).
    group_views: HashMap<SharedString, u64>,
    /// `render_last_empty_col`: a view in place of upstream's 12 px filler (0: upstream's).
    last_col: u64,
    /// Styles over upstream's header row and body rows (`render_header`, `render_tr`).
    header_style: Option<StyleRefinement>,
    row_style: Option<StyleRefinement>,
}

impl GkTableDelegate {
    /// The delegate over `src`, with the columns given for it (TABLE_SOURCE_COLUMNS).
    pub fn from_source(src: u64) -> GkResult<Self> {
        source::count(src, 0)?;
        if decode_cells() {
            source::set_decoded(src);
        }
        let mut d = GkTableDelegate {
            src,
            columns: Vec::new(),
            order: Vec::new(),
            x: Extras::default(),
            watched: false,
            asked: (0, 0..0),
            look: Look::default(),
        };
        d.set_columns(SOURCE_COLUMNS.with(|m| m.borrow_mut().remove(&src)).unwrap_or_default());
        Ok(d)
    }

    fn set_columns(&mut self, columns: Vec<Column>) {
        self.order = (0..columns.len()).collect();
        self.columns = columns;
    }

    fn id(&self, pos: usize) -> usize {
        self.order.get(pos).copied().unwrap_or(pos)
    }

    fn cell_key(&self, row: usize, id: usize) -> u64 {
        source::key(0, (row * self.columns.len() + id) as u32)
    }

    /// Binds rows before they are drawn: when the row two screens past `rows` (or before it) has
    /// no cells, asks for two screens behind and two and a half ahead, all columns, in one ROWS
    /// turn (`source::want`: the rows not bound yet, with no overscan added). A steady scroll so
    /// costs one turn of about half a screen of cells every half screen of rows, not one per
    /// frame. Measured 23 Nov 2026 (the table scene, 100,000 rows): batches of two or three
    /// screens cost a missed frame or more each at 1,000 Hz and set the p95 at 60 Hz; one screen
    /// missed a median of 112 frames of 300 at 1,000 Hz against 107 for half a screen (Rust
    /// 105), and half a screen also had the lower p95 at 60 Hz (987 against 1,017 us). gpui-kit 0.11 on gpui-fast 0.1.19 keeps two viewports of rows
    /// drawn on each side of the visible ones (the list layer's overscan, grown a few rows a
    /// frame). `rows` is a visible range the kit reports, or a row drawn now (`render_td`): the
    /// list layer draws rows beyond the range the kit reports, and bound only from the reported
    /// ranges such a row was drawn as a placeholder first, which redrew the table for five
    /// frames.
    fn bind_ahead(&mut self, rows: Range<usize>, window: &mut Window, cx: &mut Context<TableState<Self>>) {
        let n = self.columns.len();
        let count = self.rows_count(cx);
        if n == 0 || count == 0 || rows.is_empty() {
            return;
        }
        // The kit also reports short ranges (a few rows, from a measuring pass): a screen is
        // taken as at least 24 rows.
        let len = rows.len().max(24);
        let ahead = (rows.end + 2 * len).min(count - 1);
        let behind = rows.start.saturating_sub(2 * len);
        let missing = |row: usize| !source::bound(self.src, self.cell_key(row, n - 1));
        crate::rt::trace(|| format!("table rows={rows:?} ahead={ahead} behind={behind} missing={}/{}", missing(ahead), missing(behind)));
        if missing(ahead) || missing(behind) {
            let frame = rt::with(|s| s.frame_no());
            let (lo, hi) = (rows.start.saturating_sub(2 * len), (rows.end + 2 * len + len / 2).min(count));
            if self.asked.0 == frame && self.asked.1.start <= behind && ahead < self.asked.1.end {
                return;
            }
            self.asked = (frame, lo..hi);
            source::want(self.src, 0, (lo * n) as u32, (hi * n).min(u32::MAX as usize) as u32, window, cx);
        }
    }

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
        match self.columns.len() {
            0 => 0,
            n => count(self.src, 0) / n,
        }
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
        // Upstream's default: the column's name.
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
        self.watch(cx);
        if col_ix == 0 {
            self.bind_ahead(row_ix..row_ix + 1, window, cx);
        }
        let key = self.cell_key(row_ix, self.id(col_ix));
        if !decode_cells() {
            return match source::row(self.src, key, window, cx) {
                Row::View(v) => v.into_any_element(),
                Row::Missing => div().into_any_element(),
            };
        }
        // The cell's tape decoded here, inside the table's render, as a Rust delegate builds
        // its cell: no view of its own in the element tree, and its entity is not read.
        match source::row_tape(self.src, key, window, cx) {
            Some((tape, view)) => genapi::with_tape(&tape, view, window, cx, |dx: &mut Dx, root| dx.child(root)),
            None => div().into_any_element(),
        }
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
        // The app's view, or nothing: upstream's skeleton table is private to the kit.
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
        // Upstream's default: the row's cell labels, joined with commas.
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
        // Upstream's default: the label centred, a border on the right.
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

    fn render_tr(&mut self, row_ix: usize, _: &mut Window, _: &mut Context<TableState<Self>>) -> Stateful<Div> {
        let mut d = div().id(("row", row_ix));
        if let Some(st) = &self.look.row_style {
            *d.style() = generated::refined(d.style().clone(), st);
        }
        d
    }

    /// The menu a cell of the row describes (`gk.row_menu` at a cell's root, the first column that
    /// has one), PROTOCOLS.md 11.24.
    fn context_menu(
        &mut self,
        row_ix: usize,
        menu: gpui_kit::component::menu::PopupMenu,
        window: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) -> gpui_kit::component::menu::PopupMenu {
        let keys: Vec<u64> = (0..self.columns.len()).map(|c| self.cell_key(row_ix, c)).collect();
        row_menu(self.src, keys, menu, window, cx)
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
        // Bind rows before they are drawn while the table scrolls (`bind_ahead`; the rows the
        // table draws ask too, from `render_td`). A far jump still shows placeholders for one
        // frame.
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

/// One column as gk.Table_Column writes it: str key, str name, u16 align, opt<u16> sort,
/// opt<[4]f32> paddings (top, right, bottom, left), f32 width, opt<u16> fixed, bool resizable,
/// bool movable, bool selectable, f32 min_width, f32 max_width.
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

/// GK_TABLE_CELLS=view draws each cell as its view (gpui-fast retains it); the default decodes
/// the cell's tape inside the table's render. Read once.
fn decode_cells() -> bool {
    thread_local! {
        static DECODE: bool = std::env::var("GK_TABLE_CELLS").map(|v| v != "view").unwrap_or(true);
    }
    DECODE.with(|d| *d)
}

/// Columns given as the kit's own `Column` values (`TABLE_COLUMN_VALUES`, core/src/objects.rs,
/// PROTOCOLS.md 13.6): for the table state about to be made over `src`.
pub(crate) fn source_columns(src: u64, columns: Vec<Column>) -> GkResult<()> {
    source::count(src, 0)?;
    SOURCE_COLUMNS.with(|m| m.borrow_mut().insert(src, columns));
    Ok(())
}

/// The same for a table state that exists: replaces its columns (as TABLE_SET_COLUMNS).
pub(crate) fn state_columns(h: u64, c: &mut genapi::Call, columns: Vec<Column>) -> GkResult<()> {
    genapi::state_update_cx::<TableOf>(h, c, move |s, cx| {
        s.delegate_mut().set_columns(columns);
        s.refresh(cx);
        cx.notify();
    })
}

thread_local! {
    /// Columns given with a row source before its table state is made (TABLE_SOURCE_COLUMNS),
    /// taken by `GkTableDelegate::from_source`.
    static SOURCE_COLUMNS: RefCell<HashMap<u64, Vec<Column>>> = RefCell::new(HashMap::new());
}

// --- trees -----------------------------------------------------------------------------------

/// The tree's render closure over a row source: entry `ix` is row `ix`, whose view describes
/// a `ListItem` (it reads its entry with TREE_ENTRY, so it is described again when the tree
/// changes). The tree marks it selected and disabled itself.
pub fn tree_item(
    src: u64,
) -> impl Fn(usize, &TreeEntry, bool, &mut Window, &mut App) -> ListItem + 'static {
    source::set_decoded(src);
    move |ix, _entry, _selected, window, cx| {
        // The entity that draws the rows is the tree state's view, being drawn now.
        source::watch(src, window.current_view());
        row_item(src, source::key(0, ix as u32), 0., window, cx)
    }
}

/// u32 n, then (str id, str label, u32 depth, u8 flags) each.
fn read_flat(r: &mut R) -> Vec<(String, String, u32, u8)> {
    let n = r.u32();
    (0..n).map(|_| (r.str().to_owned(), r.str().to_owned(), r.u32(), r.u8())).collect()
}

/// Builds the tree from a depth-first list of (id, label, depth, flags), flags 1 expanded, 2
/// disabled.
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
    // A depth that jumps by more than one, or rows left over: the rest at the top level.
    while at < flat.len() {
        let d = flat[at].2;
        out.extend(level(flat, &mut at, d));
    }
    out
}

// --- elements ------------------------------------------------------------------------------

/// The family's element nodes: ROW_MENU (PROTOCOLS.md 11.24); List, DataTable and Tree are
/// generated over the shims.
pub fn node(dx: &mut Dx, ty: u16, args: u32, _first: u32) -> Option<AnyElement> {
    if ty == N_ROW_MENU {
        let child = dx.args(args).u32();
        return Some(dx.child(child));
    }
    None
}

// --- per-row menus (docs/PROTOCOLS.md 11.24) ---------------------------------------------------

/// Node ROW_MENU (`gk.row_menu(el, menu)`): u32 child node, u32 menu node (a described PopupMenu,
/// 11.6). It draws as its child; the menu is the row's, described with the row ahead of the
/// right-click that opens it. The child node of a ROW_MENU root, else the root.
fn menu_child(dx: &mut Dx, root: u32) -> u32 {
    let (ty, args, _) = dx.node(root);
    if ty == N_ROW_MENU { dx.args(args).u32() } else { root }
}

/// The menu the first of `keys` whose view's root is a ROW_MENU describes, applied to the menu
/// upstream hands in (its ops, as a view-built menu's are, 11.6), so its handlers are the row
/// view's. A row with no view yet, or none with a menu, leaves the menu as upstream made it (empty:
/// upstream opens nothing). Called when the row is right-clicked: never while a frame is drawn.
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

/// `Tree::context_menu`'s closure over the tree's row source (`[rows]` in overrides.toml): the
/// menu row `ix`'s view describes (`row_menu`).
pub fn tree_menu(
    src: u64,
) -> impl Fn(usize, &TreeEntry, gpui_kit::component::menu::PopupMenu, &mut Window, &mut Context<'_, TreeState>) -> gpui_kit::component::menu::PopupMenu + 'static {
    move |ix, _entry, menu, window, cx| row_menu(src, [source::key(0, ix as u32)], menu, window, cx)
}

// --- call ops --------------------------------------------------------------------------------

/// Makes `view` (a view the app gave as data) drawn in the source's window: it is described
/// before the next frame like any view.
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

/// ROWS_DATA after the state handle: u8 flags (1 loading, 2 has_more, 4 visible events, 8
/// section headers, 16 section footers), u64 threshold, u64 empty view, u64 loading view, u64
/// initial view, f32 placeholder height. Views are 0 for upstream's.
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

/// The family's call ops; None: not one of them.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_ROWS_DATA => (|| {
            // u64 list or table state, the delegate's data (read_extras).
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
            // u64 source, columns: the columns of the table state about to be made over it.
            let src = r.u64();
            let columns = read_columns(r);
            source::count(src, 0)?;
            SOURCE_COLUMNS.with(|m| m.borrow_mut().insert(src, columns));
            Ok(())
        })(),
        C_TABLE_SET_COLUMNS => (|| {
            // u64 table state, columns. The app re-keys the cells itself (set_count) when the
            // number of columns changes.
            let h = r.u64();
            let columns = read_columns(r);
            genapi::state_update_cx::<TableOf>(h, c, move |s, cx| {
                s.delegate_mut().set_columns(columns);
                s.refresh(cx);
                cx.notify();
            })
        })(),
        C_TABLE_TEXTS => (|| {
            // u64 table state, u8 replace, u64 first row, u32 rows, u32 columns, then rows *
            // columns str (by column id; an empty text removes the cell's).
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
            // u64 table state, u32 n, (u64 row, str label; empty: upstream's) each.
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
            // u64 table state; u32 n, (u32 column id, f32 fit width) each; u8 has groups, u32
            // levels, (u32 n, (str label, u64 span) each) each; u32 n, (str label, u64 view) each;
            // u64 last column view; header style and row style as values (u32 length 0: none).
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
                // Upstream's rule after the group headers change.
                s.refresh_header_layout(cx);
            })
        })(),
        C_TREE_SET_ITEMS => (|| {
            // u64 tree state, u32 n, (str id, str label, u32 depth, u8 flags) each.
            let h = r.u64();
            let items = build_tree(&read_flat(r));
            genapi::state_update_cx::<TreeState>(h, c, move |s, cx| s.set_items(items, cx))
        })(),
        C_TREE_ENTRY => (|| {
            // u64 tree state, u64 index -> u8 present, str id, str label, u64 depth, u8 flags
            // (1 folder, 2 expanded, 4 disabled).
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
        // The searchable-list family's ops (core/src/shim_select.rs), asked from here so that the
        // shared dispatch in calls.rs needs no line of its own.
        _ => return crate::shim_select::call(op, r, c),
    })
}
