//! The data protocol (docs/PROTOCOLS.md section 12). A chart, plot shape or scale that is
//! generic over the type of one row of data is instantiated by the generator for `Datum`, a
//! row of a table the app passed with the element (`gk.data(rows[:])`), and every closure
//! upstream calls on a row while it draws is answered from that table: a column (`Field`) or
//! a template over the numbers upstream hands in and the row's columns (`Format`). No app code
//! runs while a frame is drawn (decision 3), and the app's rows are copied once per
//! description of the view, as a Rust app clones its rows into the chart it builds.
//!
//! Generated decoders call `rows`, `Field::new` and `Format::new` and nothing else here.

use crate::genapi::{self, R};
use gpui_kit::{App, Hsla, SharedString};
use std::cell::Cell;
use std::fmt::Write as _;
use std::rc::Rc;

const NUM: u8 = 0;
const TEXT: u8 = 1;
const COLOR: u8 = 2;
const OPT: u8 = 3;
const NUM32: u8 = 4;

/// One column of a table.
pub enum Col {
    Num(Vec<f64>),
    /// Numbers the app keeps as f32: shown as text the way an f32 prints.
    Num32(Vec<f32>),
    Text(Vec<SharedString>),
    Color(Vec<Hsla>),
    Opt(Vec<Option<f64>>),
}

/// The rows the app passed with one element: its columns by name.
pub struct Table {
    names: Vec<SharedString>,
    cols: Vec<Col>,
}

/// One row of a table: what a generic chart is instantiated with for its `T`.
#[derive(Clone)]
pub struct Datum {
    table: Rc<Table>,
    row: u32,
    /// The column a keyed closure names (`Stack::value(Fn(&T, &str))`, PROTOCOLS.md 12.15): a field
    /// given as "" reads this column.
    key: Option<SharedString>,
}

impl Datum {
    /// The row's index in the table the app passed (a key of a row source over the rows).
    pub fn index(&self) -> usize {
        self.row as usize
    }

    /// The row with the column a keyed closure was handed (generated: `f.opt(&a0.keyed(a1))`).
    pub fn keyed(&self, key: &str) -> Datum {
        Datum { table: self.table.clone(), row: self.row, key: Some(SharedString::new(key)) }
    }
}

/// Reads a table: u16 columns, (u8 kind, str name) each, u32 rows, then the rows one after
/// the other. Colours are resolved against the theme here, when the view is decoded.
pub fn rows(r: &mut R, cx: &App) -> Vec<Datum> {
    rows_with(r, Some(cx))
}

/// `rows` in a builder chain of an object's constructor, which has no app context: a theme colour
/// in the rows is transparent there (PROTOCOLS.md 12.15).
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

/// A number as text, as upstream writes a raw value (`format!("{}", v)`).
fn number_text(v: f64) -> SharedString {
    SharedString::from(format!("{v}"))
}

/// A column of the row, named by the app (`->x("month")`). The column's index is looked up
/// once per table and kept.
pub struct Field {
    name: SharedString,
    at: Cell<(usize, usize)>,
}

impl Field {
    pub fn new(name: &str) -> Self {
        Field { name: SharedString::new(name), at: Cell::new((0, 0)) }
    }

    fn col<'a>(&self, d: &'a Datum) -> Option<&'a Col> {
        if self.name.is_empty() {
            // a keyed closure's field: the column the key names (Stack::value)
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

    /// A number; a gap (Maybe without a value) and a column that is no number are 0.
    pub fn num(&self, d: &Datum) -> f64 {
        let i = d.row as usize;
        match self.col(d) {
            Some(Col::Num(v)) => v[i],
            Some(Col::Num32(v)) => v[i] as f64,
            Some(Col::Opt(v)) => v[i].unwrap_or(0.),
            _ => 0.,
        }
    }

    pub fn opt(&self, d: &Datum) -> Option<f64> {
        let i = d.row as usize;
        match self.col(d) {
            Some(Col::Num(v)) => Some(v[i]),
            Some(Col::Num32(v)) => Some(v[i] as f64),
            Some(Col::Opt(v)) => v[i],
            _ => None,
        }
    }

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

/// A template over the numbers upstream hands a closure and the row's columns
/// (`->tooltip_value("{} visits")`): "{}" the next number, "{1}" the second, "{:.2}" two
/// decimals, "{:,}" thousands separated by commas, "{month}" a column of the row, "{{" and
/// "}}" a brace. The binding checks a template before it sends it; what it cannot read here
/// is kept as text.
pub struct Format {
    parts: Vec<Part>,
}

impl Format {
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

/// A number a closure was handed, in its own type: an f32 prints as upstream's closure would
/// print it (0.35, not 0.3499999940395355).
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

/// Writes a formatted number, with commas between the thousands when asked.
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

/// The element for one row of a table (a chart's tooltip content): the view of the row
/// source `src` for the row's index, described between frames by the app's row procedure.
/// The view that draws it is told when the row arrives; until then nothing is drawn.
pub fn row_element(src: u64, d: &Datum, window: &mut gpui_kit::Window, cx: &mut App) -> gpui_kit::AnyElement {
    use gpui_kit::IntoElement as _;
    crate::source::watch(src, window.current_view());
    match crate::source::row(src, crate::source::key(0, d.row), window, cx) {
        crate::source::Row::View(v) => v.into_any_element(),
        crate::source::Row::Missing => gpui_kit::div().into_any_element(),
    }
}

// ------------------------------------------------------------------ PROTOCOLS.md 12.15
//
// Kit values over rows read back: a row is its index in the rows the app passed (Odin's Arc_Data,
// Stack_Series and Stack_Point in rt_data.odin, read by reflection).

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
