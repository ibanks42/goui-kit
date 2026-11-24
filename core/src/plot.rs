//! Custom GPUI `Plot` implementation (`GkPlot`) and decoders for `N_PLOT`, `N_PLOT_PATH`,
//! and `N_PLOT_QUAD`, plus row-indexed lookup helpers for generated chart series.
//!
//! # Wire Layouts and Coordinate Space
//!
//! All coordinates inside `N_PLOT_PATH` and `N_PLOT_QUAD` are expressed in local pixels relative
//! to the laid-out `Bounds<Pixels>` origin passed to `Plot::paint`:
//! - `N_PLOT`: `[n_parts: u32][part_node_id: u32*][has_id: u8][element_id?][interactive: u8][appear_gen: u64]`
//!   (where `appear_gen == 0` disables appear-animation tracking).
//! - `N_PLOT_PATH`: `[fill: u8 (1=fill+close, 0=stroke)][stroke_width: f32][n_pts: u32][(x: f32, y: f32)*][background]`
//!   (paths with fewer than 2 points are skipped cleanly; `n_pts` pre-allocation caps at 65,536).
//! - `N_PLOT_QUAD`: `[x: f32][y: f32][w: f32][h: f32][corner_radius: f32][background][border_width: f32][border_color]`
//!   (negative dimensions clamp to `0.0`).
//! - Any other child node opcode inside `N_PLOT` delegates to `generated::paint_part` for
//!   upstream plot primitives (`PlotAxis`, `Grid`, `Bar`, `Area`, `Line`, `RadialLine`).

use crate::generated::{self, PaintFn};
use crate::genapi::Dx;
use crate::ops::{N_PLOT, N_PLOT_PATH, N_PLOT_QUAD};
use gpui_kit::base::plot::{Plot, PlotElement};
use gpui_kit::{AnyElement, App, Bounds, ElementId, IntoElement, Pixels, Window};
use gpui_kit::{BorderStyle, PathBuilder, point, px, quad, size};

/// `Plot` element that paints a list of decoded path, quad, and generated chart primitives
/// relative to its laid-out bounds.
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

/// Decodes an `N_PLOT` node and each of its child paint primitives into a [`GkPlot`].
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

/// Decodes one child primitive node of an `N_PLOT` container into a `PaintFn` closure.
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

/// Returns the entry in `rows` corresponding to `d`'s row index, or `T::default()` if out of bounds.
pub fn row_value<T: Clone + Default>(rows: &[T], d: &crate::data::Datum) -> T {
    rows.get(d.index()).cloned().unwrap_or_default()
}

/// Clones the plot label `Text` entries for `d`'s row index (upstream `Text` is not `Clone`).
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
