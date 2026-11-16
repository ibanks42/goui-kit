//! The lists and editor families' gallery pages (docs/PROTOCOLS.md sections 9.9 and 9.10), written
//! the way a Rust GPUI-kit app writes them: a Select over a SearchableVec of its own item type, a
//! searchable Select over groups, a Select whose delegate draws its own rows and group headers,
//! a multiple Combobox, and a table whose delegate gives group headers, a header and a row style
//! and a group header cell of its own. The Odin twin is examples/odin/gallery/lists2_pages.odin,
//! over the library's search delegate and table shim; the pages' lines are in
//! ref/gallery/pages.txt. Each page has its own state, made once when the gallery starts.
use crate::Gallery;
use gpui_kit::component::combobox::{Combobox, ComboboxState};
use gpui_kit::component::label::Label;
use gpui_kit::component::searchable_list::{SearchableGroup, SearchableListDelegate, SearchableListItem, SearchableVec};
use gpui_kit::component::select::{Select, SelectState};
use gpui_kit::component::table::{
    Column, ColumnGroup, DataTable, Table, TableBody, TableCell, TableDelegate, TableHead, TableHeader, TableRow,
    TableState,
};
use gpui_kit::component::{ActiveTheme, IndexPath};
use gpui_kit::{
    AnyElement, App, AppContext, Context, Div, Entity, InteractiveElement, IntoElement, ParentElement,
    Pixels, SharedString, Stateful, Styled, Task, Window, div, px,
};

/// An item as the Odin pages give one (gk.Search_Item): value, title, a trigger display, a
/// disabled flag.
#[derive(Clone)]
pub struct Fruit {
    value: SharedString,
    title: SharedString,
    display: Option<SharedString>,
    disabled: bool,
}

fn fruit(value: &str, title: &str) -> Fruit {
    Fruit { value: value.to_string().into(), title: title.to_string().into(), display: None, disabled: false }
}

impl SearchableListItem for Fruit {
    type Value = SharedString;
    fn title(&self) -> SharedString {
        self.title.clone()
    }
    fn display_title(&self) -> Option<AnyElement> {
        self.display.clone().map(IntoElement::into_any_element)
    }
    fn value(&self) -> &SharedString {
        &self.value
    }
    fn disabled(&self) -> bool {
        self.disabled
    }
}

pub fn fruits() -> Vec<Fruit> {
    let mut grape = fruit("grape", "Grape");
    grape.disabled = true;
    let mut mango = fruit("mango", "Mango");
    mango.display = Some("Mango (ripe)".into());
    vec![fruit("apple", "Apple"), fruit("banana", "Banana"), fruit("cherry", "Cherry"), grape, mango]
}

pub fn veg() -> Vec<Fruit> {
    vec![fruit("kale", "Kale"), fruit("leek", "Leek")]
}

fn groups() -> SearchableVec<SearchableGroup<Fruit>> {
    SearchableVec::new(vec![
        SearchableGroup::new("Fruit").items(fruits()),
        SearchableGroup::new("Veg").items(veg()),
    ])
}

/// A delegate that draws its own rows ("> Title") and group headers ("== fruit ==").
pub struct Rows {
    inner: SearchableVec<SearchableGroup<Fruit>>,
}

impl SearchableListDelegate for Rows {
    type Item = Fruit;
    fn sections_count(&self, cx: &App) -> usize {
        self.inner.sections_count(cx)
    }
    fn items_count(&self, section: usize) -> usize {
        self.inner.items_count(section)
    }
    fn item(&self, ix: IndexPath) -> Option<&Fruit> {
        self.inner.item(ix)
    }
    fn position<V>(&self, value: &V) -> Option<IndexPath>
    where
        Self::Item: SearchableListItem<Value = V>,
        V: PartialEq,
    {
        self.inner.position(value)
    }
    fn perform_search(&mut self, query: &str, window: &mut Window, cx: &mut App) -> Task<()> {
        self.inner.perform_search(query, window, cx)
    }
    fn render_item(&self, _: IndexPath, item: &Fruit, _: bool, _: &mut Window, _: &mut App) -> Option<AnyElement> {
        Some(Label::new(format!("> {}", item.title)).into_any_element())
    }
    fn render_section_header(&self, section: usize, _: &mut Window, _: &mut App) -> Option<AnyElement> {
        Some(Label::new(if section == 0 { "== fruit ==" } else { "== veg ==" }).into_any_element())
    }
}

pub struct Lists2Pages {
    pub select: Entity<SelectState<SearchableVec<Fruit>>>,
    pub search: Entity<SelectState<SearchableVec<SearchableGroup<Fruit>>>>,
    pub rows: Entity<SelectState<Rows>>,
    pub combo: Entity<ComboboxState<SearchableVec<Fruit>>>,
    pub table: Entity<TableState<Look>>,
    pub diff: Entity<gpui_kit::component::diff::DiffState>,
    // docs/PROTOCOLS.md 9.16 to 9.18
    pub token: Entity<gpui_kit::base::input::InputState>,
    pub editor: Entity<gpui_kit::base::input::EditorState>,
}

impl Lists2Pages {
    pub fn new(window: &mut Window, cx: &mut App) -> Self {
        let select = cx.new(|cx| SelectState::new(SearchableVec::new(fruits()), Some(IndexPath::default().row(4)), window, cx));
        let search = cx.new(|cx| SelectState::new(groups(), None, window, cx).searchable(true));
        let rows = cx.new(|cx| SelectState::new(Rows { inner: groups() }, None, window, cx));
        let combo = cx.new(|cx| {
            ComboboxState::new(SearchableVec::new(fruits()), vec![IndexPath::default().row(1)], window, cx).multiple(true)
        });
        let table = cx.new(|cx| TableState::new(Look::new(), window, cx));
        let diff = cx.new(|cx| {
            gpui_kit::component::diff::DiffState::new(gpui_kit::component::diff::DiffFile::parse(DIFF_VIEW_PATCH).unwrap(), cx)
        });
        let token = cx.new(|cx| {
            let mut s = gpui_kit::base::input::InputState::new(window, cx).default_value("Hello world");
            let _ = s.replace_range_with_token(6..11, gpui_kit::base::input::InlineToken::new("t1", "world").with_label("World"), window, cx);
            s
        });
        let editor = cx.new(|cx| {
            let mut s = gpui_kit::base::input::EditorState::new(window, cx).language("odin").default_value(EDITOR_DIAG_TEXT);
            s.set_highlighter_factory(std::rc::Rc::new(|_lang: &str| Some(Box::new(Keywords::default()) as Box<dyn gpui_kit::base::input::InputHighlighter>)), cx);
            if let Some(d) = s.diagnostics_mut() {
                d.push(gpui_kit::base::input::Diagnostic::new(
                    lsp_types::Position::new(1, 4)..lsp_types::Position::new(1, 10),
                    "never called",
                ));
            }
            s
        });
        Lists2Pages { select, search, rows, combo, table, diff, token, editor }
    }
}

/// A table whose delegate gives group headers, a styled header row and styled body rows, and a
/// group header cell of its own for "Values".
pub struct Look {
    columns: Vec<Column>,
}

impl Look {
    fn new() -> Self {
        Look {
            columns: vec![Column::new("id", "Id").width(px(60.)), Column::new("name", "Name").width(px(100.)), Column::new("a", "A").width(px(60.))],
        }
    }
}

impl TableDelegate for Look {
    fn columns_count(&self, _: &App) -> usize {
        self.columns.len()
    }
    fn rows_count(&self, _: &App) -> usize {
        6
    }
    fn column(&self, col_ix: usize, _: &App) -> Column {
        self.columns[col_ix].clone()
    }
    fn render_td(&mut self, row_ix: usize, col_ix: usize, _: &mut Window, _: &mut Context<TableState<Self>>) -> impl IntoElement {
        Label::new(format!("{row_ix}:{col_ix}"))
    }
    fn group_headers(&self, _: &App) -> Option<Vec<Vec<ColumnGroup>>> {
        Some(vec![vec![ColumnGroup::new("Key", 1), ColumnGroup::new("Values", 2)]])
    }
    fn render_group_th(
        &mut self,
        label: &SharedString,
        _: usize,
        width: Pixels,
        _: &mut Window,
        cx: &mut Context<TableState<Self>>,
    ) -> impl IntoElement {
        if label.as_ref() == "Values" {
            return div().w(width).h_full().flex_shrink_0().child(Label::new("[values]")).into_any_element();
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
    fn render_header(&mut self, _: &mut Window, cx: &mut Context<TableState<Self>>) -> Stateful<Div> {
        div().id("header").bg(cx.theme().secondary)
    }
    fn render_tr(&mut self, row_ix: usize, _: &mut Window, cx: &mut Context<TableState<Self>>) -> Stateful<Div> {
        div().id(("row", row_ix)).text_color(cx.theme().muted_foreground)
    }
}

/// The page of `id`, or None when it is not one of this family's.
pub fn page(id: &str, g: &Gallery) -> Option<AnyElement> {
    let p = &g.lists2;
    Some(match id {
        "lists2-select" => div().w(px(220.)).child(Select::new(&p.select).placeholder("Fruit")).into_any_element(),
        "lists2-select-search" => div()
            .w(px(220.))
            .child(Select::new(&p.search).placeholder("Food").search_placeholder("Find"))
            .into_any_element(),
        "lists2-select-rows" => div().w(px(220.)).child(Select::new(&p.rows).placeholder("Food")).into_any_element(),
        "lists2-combobox" => div().w(px(260.)).child(Combobox::new(&p.combo).placeholder("Fruit")).into_any_element(),
        "lists2-table-static" => div()
            .w(px(360.))
            .child(
                Table::new()
                    .child(TableHeader::new().child(
                        TableRow::new().child(TableHead::new().child("Fruit")).child(TableHead::new().child("Count")),
                    ))
                    .child(TableBody::new().children(["Apple", "Banana", "Cherry"].iter().enumerate().map(|(n, f)| {
                        TableRow::new().child(TableCell::new().child(*f)).child(TableCell::new().child(format!("{}", n * 3)))
                    }))),
            )
            .into_any_element(),
        "text-token" => div()
            .w(px(260.))
            .child(gpui_kit::component::input::Input::new(&p.token).token(|ctx, _, _| gpui_kit::component::input::InputToken::new(ctx)))
            .into_any_element(),
        "diff-header" => div()
            .w(px(440.))
            .h(px(200.))
            .child(
                gpui_kit::component::diff::Diff::new(&p.diff)
                    .render_header_suffix(|file, _, _| Label::new(format!("[{}]", file.path()))),
            )
            .into_any_element(),
        "editor-diagnostics" => div()
            .w(px(480.))
            .child(gpui_kit::component::input::Editor::new(&p.editor).h(px(160.)))
            .into_any_element(),
        "diff-view" => div()
            .w(px(440.))
            .h(px(200.))
            .child(gpui_kit::component::diff::Diff::new(&p.diff))
            .into_any_element(),
        "text-highlight" => div()
            .w(px(360.))
            .child(gpui_kit::base::TextView::markdown("hl", TEXT_HIGHLIGHT_MD).code_block_highlighter(|block| text_highlight_spans(&block.code())))
            .into_any_element(),
        "lists2-table-look" => div().w(px(440.)).h(px(240.)).child(DataTable::new(&p.table).scrollbar_visible(false, false)).into_any_element(),
        _ => return None,
    })
}

/// text-highlight: a Markdown code block highlighted by the app's closure: "proc" and "return"
/// in red (the Odin page answers the same between frames, docs/PROTOCOLS.md 9.14).
const TEXT_HIGHLIGHT_MD: &str = "# Code\n\n```odin\nmain :: proc() {\n    return\n}\n```\n";

fn text_highlight_spans(code: &str) -> Vec<(std::ops::Range<usize>, gpui_kit::HighlightStyle)> {
    let b = code.as_bytes();
    let mut out = Vec::new();
    let mut i = 0;
    while i < b.len() {
        let mut n = 0;
        for kw in ["proc", "return"] {
            if b.len() - i >= kw.len() && &b[i..i + kw.len()] == kw.as_bytes() {
                n = kw.len();
            }
        }
        if n > 0 {
            out.push((i..i + n, gpui_kit::HighlightStyle { color: Some(gpui_kit::hsla(0., 0.7, 0.5, 1.)), ..Default::default() }));
            i += n;
        } else {
            i += 1;
        }
    }
    out
}

/// diff-view: a patch parsed by the kit, shown by its Diff.
const DIFF_VIEW_PATCH: &str = "diff --git a/hello.odin b/hello.odin\n--- a/hello.odin\n+++ b/hello.odin\n@@ -1,4 +1,4 @@\n package hello\n \n-main :: proc() {}\n+main :: proc() { run() }\n // end\n";

// --- docs/PROTOCOLS.md 9.16 to 9.18 ---------------------------------------------------

/// editor-diagnostics: the editor's text, a diagnostic on `helper` (line 1, columns 4 to 10), and
/// "proc" and "return" highlighted as keywords by the app's highlighter.
pub const EDITOR_DIAG_TEXT: &str = "main :: proc() {\n    helper()\n    return\n}\n";

/// A highlighter as a Rust app writes one: "proc" and "return" are "keyword" runs, resolved by the
/// editor's resolver (the theme's syntax colours); the Odin page answers the same runs between
/// frames (gk.highlighter).
#[derive(Default)]
pub struct Keywords {
    text: String,
}

pub fn keyword_runs(text: &str) -> Vec<(std::ops::Range<usize>, &'static str)> {
    let b = text.as_bytes();
    let mut out = Vec::new();
    let mut i = 0;
    while i < b.len() {
        let mut n = 0;
        for kw in ["proc", "return"] {
            if b.len() - i >= kw.len() && &b[i..i + kw.len()] == kw.as_bytes() {
                n = kw.len();
            }
        }
        if n > 0 {
            out.push((i..i + n, "keyword"));
            i += n;
        } else {
            i += 1;
        }
    }
    out
}

impl gpui_kit::base::input::InputHighlighter for Keywords {
    fn language(&self) -> SharedString {
        "odin".into()
    }
    fn update(
        &mut self,
        _: Option<gpui_kit::base::input::InputEdit>,
        text: &ropey::Rope,
        _: bool,
        _: &mut Window,
        _: &mut Context<gpui_kit::base::input::EditorState>,
    ) {
        self.text = text.to_string();
    }
    fn styles(
        &self,
        range: &std::ops::Range<usize>,
        resolver: &dyn gpui_kit::base::input::HighlightStyleResolver,
    ) -> Vec<(std::ops::Range<usize>, gpui_kit::HighlightStyle)> {
        let mut out = Vec::new();
        let mut at = range.start;
        for (r, name) in keyword_runs(&self.text) {
            let (s, e) = (r.start.max(at), r.end.min(range.end));
            if s >= e {
                continue;
            }
            if s > at {
                out.push((at..s, gpui_kit::HighlightStyle::default()));
            }
            out.push((s..e, resolver.style(name).unwrap_or_default()));
            at = e;
        }
        if at < range.end {
            out.push((at..range.end, gpui_kit::HighlightStyle::default()));
        }
        out
    }
    fn fold_ranges(&self, _: &ropey::Rope) -> Vec<gpui_kit::base::input::FoldRange> {
        Vec::new()
    }
}
