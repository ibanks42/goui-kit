//! A described plot (docs/PROTOCOLS.md section 12): `gk.plot(parts..)`, a plot the app writes
//! as a list of parts instead of a `Plot` impl whose `paint` would run app code while the
//! frame is drawn (decision 3). A part is any value type of the kit that paints itself in the
//! plot's bounds (`PlotAxis`, `Grid`, the shapes over the app's rows); the generator lists
//! them (`generated::paint_part`). The library implements `Plot` once, for `GkPlot`, and paints
//! the parts in the order the app gave them, as a Rust `paint` that calls each part's `paint`.
//!
//! Node `PLOT 28`: u32 n, u32 value node each, then the options (PROTOCOLS.md 12.14): u8 has id,
//! the id; u8 interactive; u64 appear generation (0: none). No ops.
//!
//! Besides the kit's parts, two of the binding's own (PROTOCOLS.md 12.17), value nodes the app makes
//! with `gk.plot_line`, `gk.plot_area` and `gk.plot_quad`: what a Rust `paint` draws with gpui's
//! `PathBuilder` and `paint_path`, and `paint_quad`, with a colour or a gradient (a Fill,
//! `genapi::background`). Their positions are pixels from the plot's top left, as a Rust app adds
//! `bounds.origin` to its own.

use crate::generated::{self, PaintFn};
use crate::genapi::Dx;
use crate::ops::{N_PLOT, N_PLOT_PATH, N_PLOT_QUAD};
use gpui_kit::base::plot::{Plot, PlotElement};
use gpui_kit::{AnyElement, App, Bounds, ElementId, IntoElement, Pixels, Window};
use gpui_kit::{BorderStyle, PathBuilder, point, px, quad, size};

pub struct GkPlot {
    parts: Vec<PaintFn>,
    id: Option<ElementId>,
    interactive: bool,
    appear: Option<u64>,
}

impl Plot for GkPlot {
    fn id(&self) -> Option<ElementId> {
        self.id.clone()
    }

    fn interactive(&self) -> bool {
        self.interactive
    }

    fn appear_generation(&self) -> Option<u64> {
        self.appear
    }

    fn paint(&mut self, bounds: Bounds<Pixels>, window: &mut Window, cx: &mut App) {
        for part in &self.parts {
            part(&bounds, window, cx);
        }
    }
}

impl IntoElement for GkPlot {
    type Element = PlotElement<Self>;

    fn into_element(self) -> Self::Element {
        PlotElement::new(self)
    }
}

pub fn node(dx: &mut Dx, ty: u16, args: u32, _first: u32) -> Option<AnyElement> {
    if ty != N_PLOT {
        return None;
    }
    let mut r = dx.args(args);
    let n = r.u32();
    let nodes: Vec<u32> = (0..n).map(|_| r.u32()).collect();
    let id = (r.u8() != 0).then(|| crate::generated::element_id(&mut r));
    let interactive = r.u8() != 0;
    let appear = Some(r.u64()).filter(|g| *g != 0);
    let parts = nodes.into_iter().filter_map(|n| part(dx, n)).collect();
    Some(GkPlot { parts, id, interactive, appear }.into_any_element())
}

/// One part of a plot: the binding's own (a path, a quad) or a kit value that paints itself.
fn part(dx: &mut Dx, n: u32) -> Option<PaintFn> {
    let (ty, args, _) = dx.node(n);
    match ty {
        N_PLOT_PATH => {
            let mut r = dx.args(args);
            let fill = r.u8() == 1;
            let width = r.f32();
            let count = r.u32() as usize;
            let mut pts = Vec::with_capacity(count.min(1 << 16));
            for _ in 0..count {
                pts.push((r.f32(), r.f32()));
            }
            let bg = crate::genapi::background(&mut r, &*dx.cx);
            Some(Box::new(move |b: &Bounds<Pixels>, window: &mut Window, _: &mut App| {
                if pts.len() < 2 {
                    return;
                }
                let at = |(x, y): (f32, f32)| point(b.origin.x + px(x), b.origin.y + px(y));
                let mut path = if fill { PathBuilder::fill() } else { PathBuilder::stroke(px(width)) };
                path.move_to(at(pts[0]));
                for p in &pts[1..] {
                    path.line_to(at(*p));
                }
                if fill {
                    path.close();
                }
                if let Ok(path) = path.build() {
                    window.paint_path(path, bg);
                }
            }))
        }
        N_PLOT_QUAD => {
            let mut r = dx.args(args);
            let (x, y, w, h, radius) = (r.f32(), r.f32(), r.f32(), r.f32(), r.f32());
            let bg = crate::genapi::background(&mut r, &*dx.cx);
            let border = r.f32();
            let border_color = crate::genapi::color(&mut r, &*dx.cx);
            Some(Box::new(move |b: &Bounds<Pixels>, window: &mut Window, _: &mut App| {
                let at = Bounds::new(point(b.origin.x + px(x), b.origin.y + px(y)), size(px(w.max(0.)), px(h.max(0.))));
                window.paint_quad(quad(at, px(radius), bg, px(border), border_color, BorderStyle::Solid));
            }))
        }
        _ => generated::paint_part(dx, n),
    }
}

/// A list per row (docs/PROTOCOLS.md 12.16): what a closure over a row returns, from the list the
/// app gave for each row of the data (row i's at index i; past the end, the type's default).
/// `SankeyChart::labels`, `Bar::label`, `BarChart::fill_gradient`. The closure's other arguments
/// (the node's throughput, the bar's position, the value range and its remap) are not used: the app
/// wrote into each row's list what it would have computed from them.
pub fn row_value<T: Clone + Default>(rows: &[T], d: &crate::data::Datum) -> T {
    rows.get(d.index()).cloned().unwrap_or_default()
}

/// `row_value` for a list of plot labels (`Bar::label`), which are not Clone: each copied field by
/// field (its public fields; `Text` is non-exhaustive, so a copy starts from `Text::new`).
pub fn row_texts(rows: &[Vec<gpui_kit::base::plot::label::Text>], d: &crate::data::Datum) -> Vec<gpui_kit::base::plot::label::Text> {
    let Some(row) = rows.get(d.index()) else {
        return Vec::new();
    };
    row.iter()
        .map(|t| {
            let mut c = gpui_kit::base::plot::label::Text::new(t.text.clone(), t.origin, t.color);
            c.font_size = t.font_size;
            c.font_weight = t.font_weight;
            c.align = t.align;
            c
        })
        .collect()
}
