//! Columnar chart data tables ([`Table`], [`Datum`]), cached field lookups ([`Field`]),
//! and format-string evaluation ([`Format`]) for generic gpui-kit chart components.
//!
//! Upstream chart elements are generic over a row type `T`. Generated decoders instantiate them
//! with [`Datum`], a cheap `Rc<Table>` + row-index handle decoded from a columnar wire payload:
//! `[n_cols: u16][(kind: u8, name: str)*][row_count: u32][row_values*]`.

use crate::genapi::{self, R};
use gpui_kit::{App, Hsla, SharedString};
use std::cell::Cell;
use std::fmt::Write as _;
use std::rc::Rc;

/// Wire column type tags.
const NUM: u8 = 0;
const TEXT: u8 = 1;
const COLOR: u8 = 2;
const OPT: u8 = 3;
const NUM32: u8 = 4;

/// Typed column storage inside a [`Table`].
pub enum Col {
    Num(Vec<f64>),
    /// Single-precision numeric column for chart accessors that expect `f32` formatting.
    Num32(Vec<f32>),
    Text(Vec<SharedString>),
    Color(Vec<Hsla>),
    Opt(Vec<Option<f64>>),
}

/// Shared columnar table referenced by every [`Datum`] belonging to one chart dataset.
pub struct Table {
    names: Vec<SharedString>,
    cols: Vec<Col>,
}

/// Row handle passed to generic gpui-kit chart accessors, carrying a shared [`Table`],
/// zero-based `row` index, and optional series `key` for dynamic-column lookups.
#[derive(Clone)]
pub struct Datum {
    table: Rc<Table>,
    row: u32,
    /// Optional series key used when a [`Field`] has an empty name (stacked-series accessors).
    key: Option<SharedString>,
}

impl Datum {
    /// Returns this datum's zero-based row index inside its [`Table`].
    pub fn index(&self) -> usize {
        self.row as usize
    }

    /// Returns a clone of this datum bound to series column `key`.
    pub fn keyed(&self, key: &str) -> Datum {
        Datum { table: self.table.clone(), row: self.row, key: Some(SharedString::new(key)) }
    }
}

/// Decodes a wire data table into a `Vec<Datum>`, resolving theme color tokens against `cx`.
pub fn rows(r: &mut R, cx: &App) -> Vec<Datum> {
    rows_with(r, Some(cx))
}

/// Decodes a wire data table without theme context (theme color tokens fall back to transparent).
pub fn rows_plain(r: &mut R) -> Vec<Datum> {
    rows_with(r, None)
}

fn color_plain(r: &mut R) -> Hsla {
    if r.u8() == crate::ops::COLOR_THEME {
        r.u16();
        gpui_kit::transparent_black()
    } else {
        gpui_kit::hsla(r.f32(), r.f32(), r.f32(), r.f32())
    }
}

fn rows_with(r: &mut R, cx: Option<&App>) -> Vec<Datum> {
    let n = r.u16() as usize;
    let mut kinds = Vec::with_capacity(n);
    let mut names = Vec::with_capacity(n);
    for _ in 0..n {
        kinds.push(r.u8());
        names.push(SharedString::new(r.str()));
    }
    let count = r.u32() as usize;
    let cap = count.min(1 << 16);
    let mut cols: Vec<Col> = kinds
        .iter()
        .map(|k| match *k {
            NUM => Col::Num(Vec::with_capacity(cap)),
            NUM32 => Col::Num32(Vec::with_capacity(cap)),
            TEXT => Col::Text(Vec::with_capacity(cap)),
            COLOR => Col::Color(Vec::with_capacity(cap)),
            OPT => Col::Opt(Vec::with_capacity(cap)),
            k => panic!("gk: a data table has a column of kind {k}, which this library does not know"),
        })
        .collect();
    for _ in 0..count {
        for c in cols.iter_mut() {
            match c {
                Col::Num(v) => v.push(r.f64()),
                Col::Num32(v) => v.push(r.f32()),
                Col::Text(v) => v.push(SharedString::new(r.str())),
                Col::Color(v) => v.push(match cx {
                    Some(cx) => genapi::color(r, cx),
                    None => color_plain(r),
                }),
                Col::Opt(v) => {
                    let some = r.u8() != 0;
                    let x = r.f64();
                    v.push(some.then_some(x));
                }
            }
        }
    }
    let table = Rc::new(Table { names, cols });
    (0..count as u32).map(|row| Datum { table: table.clone(), row, key: None }).collect()
}

fn number_text(v: f64) -> SharedString {
    SharedString::from(format!("{v}"))
}

/// Column accessor that caches `(table_ptr, col_index)` so repeated per-row lookups stay O(1).
pub struct Field {
    name: SharedString,
    at: Cell<(usize, usize)>,
}

impl Field {
    /// Creates a field accessor targeting column `name` (or `Datum::key` if `name` is empty).
    pub fn new(name: &str) -> Self {
        Field { name: SharedString::new(name), at: Cell::new((0, 0)) }
    }

    fn col<'a>(&self, d: &'a Datum) -> Option<&'a Col> {
        if self.name.is_empty() {
            let c = d.table.names.iter().position(|n| Some(n) == d.key.as_ref())?;
            return d.table.cols.get(c);
        }
        let key = Rc::as_ptr(&d.table) as usize;
        let (k, c) = self.at.get();
        if k == key {
            return d.table.cols.get(c);
        }
        let c = d.table.names.iter().position(|n| *n == self.name)?;
        self.at.set((key, c));
        d.table.cols.get(c)
    }

    /// Reads this column as text, converting numeric values via default `Display` formatting.
    pub fn text(&self, d: &Datum) -> SharedString {
        let i = d.row as usize;
        match self.col(d) {
            Some(Col::Text(v)) => v[i].clone(),
            Some(Col::Num(v)) => number_text(v[i]),
            Some(Col::Num32(v)) => SharedString::from(format!("{}", v[i])),
            Some(Col::Opt(v)) => v[i].map(number_text).unwrap_or_default(),
            _ => SharedString::default(),
        }
    }

    /// Reads this column as `f64`, returning `0.0` for missing or non-numeric cells.
    pub fn num(&self, d: &Datum) -> f64 {
        let i = d.row as usize;
        match self.col(d) {
            Some(Col::Num(v)) => v[i],
            Some(Col::Num32(v)) => v[i] as f64,
            Some(Col::Opt(v)) => v[i].unwrap_or(0.),
            _ => 0.,
        }
    }

    /// Reads this column as `Option<f64>`, preserving `None` in optional columns.
    pub fn opt(&self, d: &Datum) -> Option<f64> {
        let i = d.row as usize;
        match self.col(d) {
            Some(Col::Num(v)) => Some(v[i]),
            Some(Col::Num32(v)) => Some(v[i] as f64),
            Some(Col::Opt(v)) => v[i],
            _ => None,
        }
    }

    /// Reads this column as an `Hsla` color, defaulting to transparent black if absent.
    pub fn color(&self, d: &Datum) -> Hsla {
        match self.col(d) {
            Some(Col::Color(v)) => v[d.row as usize],
            _ => gpui_kit::transparent_black(),
        }
    }
}

enum Part {
    Lit(String),
    Num { at: usize, spec: Spec },
    Col { field: Field, spec: Spec },
}

#[derive(Clone, Copy, Default)]
struct Spec {
    group: bool,
    precision: Option<usize>,
}

/// Pre-parsed label/tooltip format template supporting `{}`, `{0}`, `{col}`, optional `,`
/// thousands grouping and `.N` decimal precision (`{value:,.2}`), plus `{{` / `}}` escapes.
pub struct Format {
    parts: Vec<Part>,
}

impl Format {
    /// Parses format string `t` into literal segments and positional or named column placeholders.
    pub fn new(t: &str) -> Self {
        let mut parts = Vec::new();
        let mut lit = String::new();
        let mut next = 0;
        let mut rest = t;
        while let Some(i) = rest.find(['{', '}']) {
            lit.push_str(&rest[..i]);
            let tail = &rest[i..];
            if tail.starts_with("{{") || tail.starts_with("}}") {
                lit.push_str(&tail[..1]);
                rest = &tail[2..];
                continue;
            }
            let Some(end) = tail.find('}').filter(|_| tail.starts_with('{')) else {
                lit.push_str(&tail[..1]);
                rest = &tail[1..];
                continue;
            };
            let inner = &tail[1..end];
            let (name, spec) = inner.split_once(':').unwrap_or((inner, ""));
            let spec = Spec {
                group: spec.starts_with(','),
                precision: spec.trim_start_matches(',').strip_prefix('.').and_then(|p| p.parse().ok()),
            };
            if !lit.is_empty() {
                parts.push(Part::Lit(std::mem::take(&mut lit)));
            }
            if name.is_empty() {
                parts.push(Part::Num { at: next, spec });
                next += 1;
            } else if let Ok(at) = name.parse::<usize>() {
                parts.push(Part::Num { at, spec });
            } else {
                parts.push(Part::Col { field: Field::new(name), spec });
            }
            rest = &tail[end + 1..];
        }
        lit.push_str(rest);
        if !lit.is_empty() {
            parts.push(Part::Lit(lit));
        }
        Format { parts }
    }

    /// Evaluates the template against an optional [`Datum`] and positional numeric arguments `nums`.
    pub fn text(&self, d: Option<&Datum>, nums: &[Arg]) -> SharedString {
        let mut out = String::new();
        for p in &self.parts {
            match p {
                Part::Lit(s) => out.push_str(s),
                Part::Num { at, spec } => match nums.get(*at) {
                    Some(Arg::F64(v)) => number(&mut out, *v, *spec),
                    Some(Arg::F32(v)) => number32(&mut out, *v, *spec),
                    Some(Arg::Int(v)) => number(&mut out, *v as f64, *spec),
                    None => {}
                },
                Part::Col { field, spec } => {
                    if let Some(d) = d {
                        match (field.col(d), spec.group || spec.precision.is_some()) {
                            (Some(Col::Num32(v)), true) => number32(&mut out, v[d.row as usize], *spec),
                            (Some(Col::Num(_)) | Some(Col::Opt(_)), true) => {
                                if let Some(v) = field.opt(d) {
                                    number(&mut out, v, *spec);
                                }
                            }
                            _ => out.push_str(&field.text(d)),
                        }
                    }
                }
            }
        }
        SharedString::from(out)
    }
}

/// Positional numeric argument passed into [`Format::text`].
#[derive(Clone, Copy)]
pub enum Arg {
    F64(f64),
    F32(f32),
    Int(i64),
}

fn number32(out: &mut String, v: f32, spec: Spec) {
    let s = match spec.precision {
        Some(p) => format!("{v:.p$}"),
        None => format!("{v}"),
    };
    group(out, &s, spec.group && v.is_finite());
}

fn number(out: &mut String, v: f64, spec: Spec) {
    let s = match spec.precision {
        Some(p) => format!("{v:.p$}"),
        None => format!("{v}"),
    };
    group(out, &s, spec.group && v.is_finite());
}

/// Appends decimal string `s` to `out`, inserting ASCII commas every 3 integer digits when `comma` is set.
fn group(out: &mut String, s: &str, comma: bool) {
    if !comma {
        out.push_str(s);
        return;
    }
    let (sign, body) = s.strip_prefix('-').map(|b| ("-", b)).unwrap_or(("", s));
    let (int, frac) = body.split_once('.').map(|(a, b)| (a, Some(b))).unwrap_or((body, None));
    out.push_str(sign);
    for (i, c) in int.chars().enumerate() {
        if i > 0 && (int.len() - i) % 3 == 0 {
            out.push(',');
        }
        out.push(c);
    }
    if let Some(f) = frac {
        let _ = write!(out, ".{f}");
    }
}

/// Renders the row view in row source `src` matching `d.row`, watching `src` for updates.
pub fn row_element(src: u64, d: &Datum, window: &mut gpui_kit::Window, cx: &mut App) -> gpui_kit::AnyElement {
    use gpui_kit::IntoElement as _;
    crate::source::watch(src, window.current_view());
    match crate::source::row(src, crate::source::key(0, d.row), window, cx) {
        crate::source::Row::View(v) => v.into_any_element(),
        crate::source::Row::Missing => gpui_kit::div().into_any_element(),
    }
}

use crate::genapi::Wire;
use gpui_kit::base::plot::shape::{ArcData, StackSeries};

impl Wire for ArcData<'_, Datum> {
    fn put(&self, out: &mut Vec<u8>) {
        use crate::wire::Put;
        out.put_i64(self.data.index() as i64);
        out.put_i64(self.index as i64);
        out.put_f32(self.value);
        out.put_f32(self.start_angle);
        out.put_f32(self.end_angle);
        out.put_f32(self.pad_angle);
    }
}

impl Wire for StackSeries<Datum> {
    fn put(&self, out: &mut Vec<u8>) {
        use crate::wire::Put;
        out.put_str(&self.key);
        out.put_i64(self.index as i64);
        out.put_u32(self.points.len() as u32);
        for p in &self.points {
            out.put_f32(p.y0);
            out.put_f32(p.y1);
            out.put_i64(p.data.index() as i64);
        }
    }
}
