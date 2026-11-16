//! ref-table: the Rust reference of the `table` scene and the gallery's table pages' twin: the
//! kit's DataTable over a TableDelegate written in Rust, `rows` rows of `cols` columns, each
//! cell a Label with "row:col". The frame-cost gate scrolls it before every frame with
//! GK_SCROLL (gates/fps.sh table); the Odin twin is examples/odin/table, whose cells are views
//! over a row source (docs/PROTOCOLS.md section 9.5).
//!
//!   ref-table [rows] [cols]      (default 100000 6)
use gpui_kit::component::label::Label;
use gpui_kit::component::table::{Column, DataTable, TableDelegate, TableState};
use gpui_kit::component::{ActiveTheme, v_flex};
use gpui_kit::{
    App, AppContext, Context, Entity, IntoElement, ParentElement, Render, Styled, Window,
};

/// The rows, as an app would keep them: here only their count, the text is made from the
/// position when a cell is drawn (the Odin twin makes it when a cell is described).
struct Cells {
    rows: usize,
    columns: Vec<Column>,
}

impl TableDelegate for Cells {
    fn columns_count(&self, _: &App) -> usize {
        self.columns.len()
    }

    fn rows_count(&self, _: &App) -> usize {
        self.rows
    }

    fn column(&self, col_ix: usize, _: &App) -> Column {
        self.columns[col_ix].clone()
    }

    fn render_td(
        &mut self,
        row_ix: usize,
        col_ix: usize,
        _: &mut Window,
        _: &mut Context<TableState<Self>>,
    ) -> impl IntoElement {
        Label::new(format!("{row_ix}:{col_ix}"))
    }
}

struct Scene {
    table: Entity<TableState<Cells>>,
}

impl Render for Scene {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        v_flex()
            .size_full()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .child(DataTable::new(&self.table))
    }
}

fn main() {
    let mut args = std::env::args().skip(1);
    let rows: usize = args.next().and_then(|a| a.parse().ok()).unwrap_or(100_000);
    let cols: usize = args.next().and_then(|a| a.parse().ok()).unwrap_or(6);
    gpui_kit::application()
        .with_assets(gpui_kit::assets::Assets)
        .run(move |cx| {
            gpui_kit::init(cx);
            let options = gk_meter::window_options(900., 600., cx);
            gpui_kit::open_window(options, cx, |window, cx| {
                gk_meter::attach(window, cx);
                let columns = (0..cols)
                    .map(|c| Column::new(format!("c{c}"), format!("Column {c}")))
                    .collect();
                let table = cx.new(|cx| TableState::new(Cells { rows, columns }, window, cx));
                cx.new(|_| Scene { table })
            })
            .expect("open_window");
            cx.activate(true);
        });
}
