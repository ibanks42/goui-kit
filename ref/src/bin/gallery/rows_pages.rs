//! The rows family's gallery pages (docs/PROTOCOLS.md section 9.8): a list of 10,000 rows, a
//! table of 10,000 rows with a sortable column, and a tree, each driven by clicks, keys and the
//! scroll wheel (a far scroll and a jump to the last row included), written the way a Rust
//! GPUI-kit app writes them, with Rust delegates. The Go twin is
//! examples/go/gallery/rows_pages.go, whose rows are views over row sources; the pages'
//! lines are in ref/gallery/pages.txt. Each page has its own state, made once when the gallery
//! starts. Scrollbars are hidden: they fade in and out on a clock.
use crate::Gallery;
use gpui_kit::component::IndexPath;
use gpui_kit::component::label::Label;
use gpui_kit::component::list::{List, ListDelegate, ListItem, ListState};
use gpui_kit::component::table::{Column, ColumnSort, DataTable, TableDelegate, TableState};
use gpui_kit::component::tree::{TreeItem, TreeState, tree};
use gpui_kit::{
    AnyElement, App, AppContext, Context, Entity, IntoElement, ParentElement, Styled, Window, div,
    px,
};

pub const LIST_ROWS: usize = 10_000;
pub const TABLE_ROWS: usize = 10_000;

pub struct RowsPages {
    pub list: Entity<ListState<Items>>,
    pub table: Entity<TableState<Cells>>,
    pub tree: Entity<TreeState>,
}

impl RowsPages {
    pub fn new(window: &mut Window, cx: &mut App) -> Self {
        let list = cx.new(|cx| ListState::new(Items, window, cx));
        let columns = vec![
            Column::new("id", "Id").sortable(),
            Column::new("a", "A"),
            Column::new("b", "B"),
            Column::new("c", "C"),
        ];
        let table = cx.new(|cx| TableState::new(Cells { columns, desc: false }, window, cx));
        let tree = cx.new(|cx| {
            TreeState::new(cx).items(vec![
                TreeItem::new("docs", "docs")
                    .child(TreeItem::new("docs/guide", "guide.md"))
                    .child(TreeItem::new("docs/api", "api.md")),
                TreeItem::new("src", "src")
                    .expanded(true)
                    .child(TreeItem::new("src/a", "a.demo"))
                    .child(TreeItem::new("src/b", "b.demo")),
                TreeItem::new("readme", "README"),
            ])
        });
        RowsPages { list, table, tree }
    }
}

pub struct Items;

impl ListDelegate for Items {
    type Item = ListItem;

    fn items_count(&self, _: usize, _: &App) -> usize {
        LIST_ROWS
    }

    fn render_item(
        &mut self,
        ix: IndexPath,
        _: &mut Window,
        _: &mut Context<ListState<Self>>,
    ) -> Option<ListItem> {
        Some(ListItem::new(("item", ix.row)).child(Label::new(format!("row {}", ix.row))))
    }

    fn set_selected_index(
        &mut self,
        _: Option<IndexPath>,
        _: &mut Window,
        _: &mut Context<ListState<Self>>,
    ) {
    }
}

pub struct Cells {
    columns: Vec<Column>,
    /// The rows in descending order of their id (the sortable column).
    desc: bool,
}

impl TableDelegate for Cells {
    fn columns_count(&self, _: &App) -> usize {
        self.columns.len()
    }

    fn rows_count(&self, _: &App) -> usize {
        TABLE_ROWS
    }

    fn column(&self, col_ix: usize, _: &App) -> Column {
        self.columns[col_ix].clone()
    }

    fn perform_sort(
        &mut self,
        _: usize,
        sort: ColumnSort,
        _: &mut Window,
        _: &mut Context<TableState<Self>>,
    ) {
        self.desc = sort == ColumnSort::Descending;
    }

    fn render_td(
        &mut self,
        row_ix: usize,
        col_ix: usize,
        _: &mut Window,
        _: &mut Context<TableState<Self>>,
    ) -> impl IntoElement {
        let row = if self.desc { TABLE_ROWS - 1 - row_ix } else { row_ix };
        Label::new(format!("{row}:{col_ix}"))
    }
}

/// The page of `id`, or None when it is not one of the rows family's.
pub fn page(id: &str, g: &Gallery) -> Option<AnyElement> {
    let p = &g.rows_family;
    Some(match id {
        "rows-list" => div()
            .w(px(300.))
            .h(px(240.))
            .child(List::new(&p.list).scrollbar_visible(false))
            .into_any_element(),
        "rows-table" => div()
            .w(px(440.))
            .h(px(240.))
            .child(DataTable::new(&p.table).scrollbar_visible(false, false))
            .into_any_element(),
        "rows-tree" => div()
            .w(px(300.))
            .h(px(240.))
            .child(tree(&p.tree, |ix, entry, _, _, _| {
                let indent = "  ".repeat(entry.depth());
                ListItem::new(("tree", ix)).child(Label::new(format!("{indent}{}", entry.item().label)))
            }))
            .into_any_element(),
        _ => return None,
    })
}
