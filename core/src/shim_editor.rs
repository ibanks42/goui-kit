//! Editor support shims, wire codecs, temporary state lending, syntax
//! highlighting/language providers, and a compact math expression evaluator.
//!
//! This module extends `shim_input` and `editor_text` with:
//! - `Wire`/`Unwire` implementations for `char`, `Font`, `Rope`, slice smart
//!   pointers, `NonZero*` integers, and placeholder closures.
//! - Temporary lending (`lend`/`give_back`) of cloned sub-values out of GPUI
//!   entity state so the host can mutate them via object handles within a turn.
//! - `Ask`-backed implementations of `InputHighlighter`, `LanguageProvider`,
//!   and `SyntaxContextProvider`.
//! - `Expr`, a small recursive-descent parser and stack VM used to evaluate
//!   host-supplied numeric formulas on the Rust side.

use crate::genapi::{Call, GkResult, Put, R, Unwire, Wire};

impl Wire for char {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u32(*self as u32);
    }
}

impl Unwire for char {
    fn get(r: &mut R) -> Self {
        char::from_u32(r.u32()).unwrap_or('\u{fffd}')
    }
}

/// Dispatches editor-helper call opcodes (`C_LEND_TEST`, `C_HIGHLIGHTER_NEW`,
/// `C_LANGUAGES_NEW`) and falls through to `editor_text::call`.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    use crate::ops::*;
    Some(match op {
        C_LEND_TEST => {
            // Return cumulative (lent, returned) counts for lending tests.
            let (lent, back) = LENT.with(|l| l.get());
            c.out.put_u64(lent);
            c.out.put_u64(back);
            Ok(())
        }
        C_HIGHLIGHTER_NEW => (|| {
            // Payload: styles_ask: u64, folds_ask: u64 (0 if none),
            // language_count: u32, language_names: [str; count].
            let (styles, folds) = (r.u64(), r.u64());
            let n = r.u32() as usize;
            let languages = (0..n.min(4096)).map(|_| r.str().to_string()).collect();
            let styles_ask = crate::shim_text::Ask::take(styles);
            if !styles_ask.is_live() {
                return crate::rt::err(ERR_BAD_OP, "gk.NewHighlighter: the styles function's ask was not made by gk.NewAsk");
            }
            let folds = (folds != 0).then(|| crate::shim_text::Ask::take(folds));
            HIGHLIGHTERS.with(|h| h.borrow_mut().insert(styles, Rc::new(Highlighter { styles: styles_ask, folds, languages })));
            Ok(())
        })(),
        C_LANGUAGES_NEW => (|| {
            // Payload: src: u64, has_syntax: u8, alias_count: u32,
            // aliases: [(alias: str, canonical: str); count].
            let (src, has_syntax) = (r.u64(), r.u8() != 0);
            crate::source::count(src, 0)?;
            let n = r.u32() as usize;
            let mut aliases = std::collections::HashMap::new();
            for _ in 0..n.min(4096) {
                let alias = r.str().to_lowercase();
                aliases.insert(alias, r.str().to_string());
            }
            let syntax = has_syntax.then(|| crate::shim_text::Ask::take(src));
            LANGUAGES.with(|l| l.borrow_mut().insert(src, Rc::new(GkLanguages { aliases, syntax })));
            Ok(())
        })(),

        _ => return crate::editor_text::call(op, r, c),
    })
}

thread_local! {
    /// Diagnostic counters tracking `(lent, given_back)` calls to `lend`.
    static LENT: std::cell::Cell<(u64, u64)> = const { std::cell::Cell::new((0, 0)) };
}

/// Lends a cloned sub-object out of entity `state` as a temporary object handle
/// and schedules writing any host mutations back into `state`.
///
/// Two nested `cx.defer` hops ensure the writeback runs after the current turn
/// and any immediate follow-up calls targeting the temporary handle have finished.
pub fn lend<S: 'static, T: Clone + 'static>(
    state: u64,
    copy: Option<T>,
    acc: fn(&mut S) -> Option<&mut T>,
    cx: &mut gpui_kit::App,
) -> Option<u64> {
    let h = crate::genapi::obj_temp(copy?);
    LENT.with(|l| l.set((l.get().0 + 1, l.get().1)));
    cx.defer(move |cx| cx.defer(move |cx| crate::rt::guard("giving back a lent object", || give_back::<S, T>(state, h, acc, cx)).unwrap_or(())));
    Some(h)
}

/// Copies the mutated temporary object back into the owning entity slot,
/// releases the temporary handle, and notifies observers.
fn give_back<S: 'static, T: Clone + 'static>(state: u64, h: u64, acc: fn(&mut S) -> Option<&mut T>, cx: &mut gpui_kit::App) {
    // Release the temporary handle before dropping the boxed value so no
    // destructor runs while the session borrow is active.
    let Ok(copy) = crate::genapi::obj_clone::<T>(h) else { return };
    let item = crate::rt::with(|s| s.release(h));
    drop(item);
    let Ok(entity) = crate::genapi::state::<S>(state) else { return };
    entity.update(cx, |s, cx| {
        if let Some(slot) = acc(s) {
            *slot = copy;
        }
        cx.notify();
    });
    LENT.with(|l| l.set((l.get().0, l.get().1 + 1)));
}

use gpui_kit::base::input::{EditorState, FoldRange, HighlightStyleResolver, InputEdit, InputHighlighter};
use gpui_kit::base::input::{LanguageProvider, SyntaxContext, SyntaxContextProvider};
use std::cell::RefCell;
use std::ops::Range;
use std::rc::Rc;

/// Configuration registered by `C_HIGHLIGHTER_NEW`, shared across per-language
/// `GkHighlighter` instances created by `highlighter_factory`.
struct Highlighter {
    styles: crate::shim_text::Ask,
    folds: Option<crate::shim_text::Ask>,
    languages: Vec<String>,
}

thread_local! {
    /// Highlighters keyed by their `styles` ask handle.
    static HIGHLIGHTERS: RefCell<std::collections::HashMap<u64, Rc<Highlighter>>> = RefCell::new(std::collections::HashMap::new());
    /// Language providers keyed by their source handle.
    static LANGUAGES: RefCell<std::collections::HashMap<u64, Rc<GkLanguages>>> = RefCell::new(std::collections::HashMap::new());
}

/// Returns a GPUI highlighter factory closure backed by the host-registered
/// `Highlighter` for `src`.
///
/// When `languages` is non-empty, only case-insensitively matching language
/// names produce a highlighter; an empty list accepts any language.
pub fn highlighter_factory(src: u64) -> Rc<dyn Fn(&str) -> Option<Box<dyn InputHighlighter>>> {
    let h = HIGHLIGHTERS.with(|m| m.borrow().get(&src).cloned());
    if h.is_none() {
        eprintln!("gk: a highlighter factory was given a handle {src:#x} gk.NewHighlighter did not make; it highlights nothing");
    }
    Rc::new(move |lang: &str| {
        let h = h.clone()?;
        if !h.languages.is_empty() && !h.languages.iter().any(|l| l.eq_ignore_ascii_case(lang)) {
            return None;
        }
        Some(Box::new(GkHighlighter { h, lang: lang.to_string().into(), text: String::new() }) as Box<dyn InputHighlighter>)
    })
}

/// Per-editor `InputHighlighter` instance caching the latest buffer text so
/// `styles()` and `fold_ranges()` can key asynchronous `Ask` lookups.
struct GkHighlighter {
    h: Rc<Highlighter>,
    lang: gpui_kit::SharedString,
    text: String,
}

/// Decodes highlight runs: `count: u32`, then `(start: u64, end: u64, style: str)` per run.
fn read_runs(r: &mut R, _dx: &mut crate::genapi::Dx) -> Option<Vec<(Range<usize>, String)>> {
    let n = r.u32() as usize;
    let mut v = Vec::with_capacity(n.min(4096));
    for _ in 0..n {
        let (a, b) = (r.u64() as usize, r.u64() as usize);
        v.push((a..b, r.str().to_string()));
    }
    Some(v)
}

/// Decodes fold ranges: `count: u32`, then `(start_line: u64, end_line: u64)` per fold.
fn read_folds(r: &mut R, _dx: &mut crate::genapi::Dx) -> Option<Vec<(usize, usize)>> {
    let n = r.u32() as usize;
    let mut v = Vec::with_capacity(n.min(4096));
    for _ in 0..n {
        v.push((r.u64() as usize, r.u64() as usize));
    }
    Some(v)
}

/// Normalizes host-supplied highlight runs into a contiguous, non-overlapping
/// partition of `range`, filling gaps with `HighlightStyle::default()`.
fn cover(
    range: &Range<usize>,
    mut runs: Vec<(Range<usize>, String)>,
    resolver: &dyn HighlightStyleResolver,
) -> Vec<(Range<usize>, gpui_kit::HighlightStyle)> {
    runs.sort_by_key(|(r, _)| r.start);
    let mut out = Vec::with_capacity(runs.len() * 2 + 1);
    let mut at = range.start;
    for (r, name) in runs {
        // Clamp against both the cursor (to drop overlaps) and range.end.
        let (s, e) = (r.start.max(at), r.end.min(range.end));
        if s >= e {
            continue;
        }
        if s > at {
            out.push((at..s, gpui_kit::HighlightStyle::default()));
        }
        out.push((s..e, resolver.style(&name).unwrap_or_default()));
        at = e;
    }
    if at < range.end {
        out.push((at..range.end, gpui_kit::HighlightStyle::default()));
    }
    out
}

impl InputHighlighter for GkHighlighter {
    fn language(&self) -> gpui_kit::SharedString {
        self.lang.clone()
    }

    fn update(
        &mut self,
        _edit: Option<InputEdit>,
        text: &ropey::Rope,
        _folding: bool,
        _window: &mut gpui_kit::Window,
        _cx: &mut gpui_kit::Context<EditorState>,
    ) {
        let t = text.to_string();
        if t != self.text {
            self.text = t;
            // Invalidate cached replies whenever buffer content changes so
            // stale runs or folds are not reused.
            self.h.styles.clear();
            if let Some(f) = &self.h.folds {
                f.clear();
            }
        }
    }

    fn styles(&self, range: &Range<usize>, resolver: &dyn HighlightStyleResolver) -> Vec<(Range<usize>, gpui_kit::HighlightStyle)> {
        let key = crate::shim_text::key(&(self.lang.as_ref(), self.text.as_str(), range.clone()));
        let runs = self.h.styles.get(key, read_runs, |v| v, Vec::new);
        cover(range, runs, resolver)
    }

    fn fold_ranges(&self, text: &ropey::Rope) -> Vec<FoldRange> {
        let Some(folds) = &self.h.folds else { return Vec::new() };
        let key = crate::shim_text::key(&(self.lang.as_ref(), text.to_string().as_str()));
        let got = folds.get(key, read_folds, |v| v, Vec::new);
        got.into_iter().filter(|(a, b)| a <= b).map(|(a, b)| FoldRange::new(a, b)).collect()
    }
}

/// Host-configured language alias table and optional syntax-context query hook.
struct GkLanguages {
    aliases: std::collections::HashMap<String, String>,
    syntax: Option<crate::shim_text::Ask>,
}

/// Looks up the registered `LanguageProvider` for `src`, falling back to an
/// empty pass-through provider if `src` was not registered.
pub fn language_provider(src: u64) -> Rc<dyn LanguageProvider> {
    match LANGUAGES.with(|m| m.borrow().get(&src).cloned()) {
        Some(l) => l,
        None => {
            eprintln!("gk: a language provider was given a handle {src:#x} gk.NewLanguageProvider did not make; upstream's defaults answer");
            Rc::new(GkLanguages { aliases: Default::default(), syntax: None })
        }
    }
}

impl LanguageProvider for GkLanguages {
    fn language_name(&self, name: &str) -> gpui_kit::SharedString {
        let low = name.to_lowercase();
        self.aliases.get(&low).cloned().unwrap_or(low).into()
    }

    fn syntax_context_provider(&self, name: &str) -> Option<Rc<dyn SyntaxContextProvider>> {
        let ask = self.syntax.clone()?;
        Some(Rc::new(GkSyntax { ask, lang: name.to_string() }))
    }
}

/// `Ask`-backed `SyntaxContextProvider` resolving code/string/comment context
/// at a byte offset.
struct GkSyntax {
    ask: crate::shim_text::Ask,
    lang: String,
}

fn read_syntax(r: &mut R, _dx: &mut crate::genapi::Dx) -> Option<u16> {
    Some(r.u16())
}

impl SyntaxContextProvider for GkSyntax {
    fn context_at(&self, text: &ropey::Rope, offset: usize) -> SyntaxContext {
        let key = crate::shim_text::key(&(self.lang.as_str(), text.to_string().as_str(), offset));
        // Wire tags: 0 = Code (default), 1 = String, 2 = Comment.
        match self.ask.get(key, read_syntax, |v| v, || 0) {
            1 => SyntaxContext::String,
            2 => SyntaxContext::Comment,
            _ => SyntaxContext::Code,
        }
    }
}

/// Decodes a `gpui_kit::Font` from wire format:
/// `family: str`, `weight: f32` (`<= 0` keeps default), `style: u16` (0 normal, 1 italic, 2 oblique).
pub fn font(r: &mut R) -> gpui_kit::Font {
    let family = gpui_kit::SharedString::from(r.str().to_owned());
    let weight = r.f32();
    let style = match r.u16() {
        1 => gpui_kit::FontStyle::Italic,
        2 => gpui_kit::FontStyle::Oblique,
        _ => gpui_kit::FontStyle::Normal,
    };
    let mut f = gpui_kit::font(family);
    if weight > 0. {
        f.weight = gpui_kit::FontWeight(weight);
    }
    f.style = style;
    f
}

impl Wire for gpui_kit::Font {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.family);
        out.put_f32(self.weight.0);
        out.put_u16(match self.style {
            gpui_kit::FontStyle::Normal => 0,
            gpui_kit::FontStyle::Italic => 1,
            gpui_kit::FontStyle::Oblique => 2,
        });
    }
}

impl Wire for ropey::Rope {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.to_string());
    }
}

impl Wire for ropey::RopeSlice<'_> {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.to_string());
    }
}

/// Decodes a retained `(handler: u32, generation: u32)` pair with `view = 0`
/// for handlers not scoped to a specific view.
pub fn kept(r: &mut R) -> crate::genapi::HandlerRef {
    let (handler, generation) = (r.u32(), r.u32());
    crate::genapi::HandlerRef { view: 0, generation, handler }
}

/// Compiled stack-VM representation of a small floating-point math expression.
///
/// Expressions support ternary `cond ? a : b`, logical `||`/`&&`/`!`,
/// comparisons (`<`, `<=`, `>`, `>=`, `==`, `!=`), arithmetic (`+`, `-`, `*`,
/// `/`, `%`, right-associative `^`), arguments `x`/`y`/`z`/`a0`..`a9`, and
/// the builtin functions in `FUNCS`.
pub struct Expr {
    code: Vec<Op>,
}

/// Stack-machine bytecode instructions emitted by `P` and run by `Expr::eval`.
#[derive(Clone, Copy, Debug)]
enum Op {
    Num(f64),
    Arg(usize),
    Neg,
    Not,
    /// Binary operator identified by an ASCII tag (`+`, `-`, `*`, `/`, `%`,
    /// `^`, `<`, `>`, `l` for `<=`, `g` for `>=`, `=` for `==`, `n` for `!=`,
    /// `&` for `&&`, `|` for `||`).
    Bin(u8),
    /// Builtin call `(FUNCS index, arity)`.
    Call(u8, u8),
    /// Ternary selection: pops `(false_val, true_val, cond)` and pushes
    /// `if cond != 0.0 { true_val } else { false_val }`.
    Pick,
}

/// Recursive-descent parser compiling infix expression text to postfix `Op`s.
struct P<'a> {
    s: &'a [u8],
    i: usize,
    out: Vec<Op>,
}

impl P<'_> {
    fn ws(&mut self) {
        while self.i < self.s.len() && self.s[self.i].is_ascii_whitespace() {
            self.i += 1;
        }
    }
    fn eat(&mut self, t: &str) -> bool {
        self.ws();
        if self.s[self.i..].starts_with(t.as_bytes()) {
            self.i += t.len();
            true
        } else {
            false
        }
    }
    fn expr(&mut self) -> Option<()> {
        self.or()?;
        if self.eat("?") {
            self.expr()?;
            if !self.eat(":") {
                return None;
            }
            self.expr()?;
            self.out.push(Op::Pick);
        }
        Some(())
    }
    fn or(&mut self) -> Option<()> {
        self.and()?;
        while self.eat("||") {
            self.and()?;
            self.out.push(Op::Bin(b'|'));
        }
        Some(())
    }
    fn and(&mut self) -> Option<()> {
        self.cmp()?;
        while self.eat("&&") {
            self.cmp()?;
            self.out.push(Op::Bin(b'&'));
        }
        Some(())
    }
    fn cmp(&mut self) -> Option<()> {
        self.add()?;
        // Check two-character tokens before single-character prefixes.
        for (t, c) in [("<=", b'l'), (">=", b'g'), ("==", b'='), ("!=", b'n'), ("<", b'<'), (">", b'>')] {
            if self.eat(t) {
                self.add()?;
                self.out.push(Op::Bin(c));
                break;
            }
        }
        Some(())
    }
    fn add(&mut self) -> Option<()> {
        self.mul()?;
        loop {
            if self.eat("+") {
                self.mul()?;
                self.out.push(Op::Bin(b'+'));
            } else if self.eat("-") {
                self.mul()?;
                self.out.push(Op::Bin(b'-'));
            } else {
                return Some(());
            }
        }
    }
    fn mul(&mut self) -> Option<()> {
        self.unary()?;
        loop {
            let c = if self.eat("*") {
                b'*'
            } else if self.eat("/") {
                b'/'
            } else if self.eat("%") {
                b'%'
            } else {
                return Some(());
            };
            self.unary()?;
            self.out.push(Op::Bin(c));
        }
    }
    fn unary(&mut self) -> Option<()> {
        if self.eat("-") {
            self.unary()?;
            self.out.push(Op::Neg);
            return Some(());
        }
        // Avoid consuming `!` when it begins `!=`.
        if self.ws_peek() == Some(b'!') && self.s.get(self.i + 1) != Some(&b'=') {
            self.i += 1;
            self.unary()?;
            self.out.push(Op::Not);
            return Some(());
        }
        self.atom()?;
        if self.eat("^") {
            self.unary()?;
            self.out.push(Op::Bin(b'^'));
        }
        Some(())
    }
    fn ws_peek(&mut self) -> Option<u8> {
        self.ws();
        self.s.get(self.i).copied()
    }
    fn atom(&mut self) -> Option<()> {
        let c = self.ws_peek()?;
        if c == b'(' {
            self.i += 1;
            self.expr()?;
            return self.eat(")").then_some(());
        }
        if c.is_ascii_digit() || c == b'.' {
            let st = self.i;
            while self.i < self.s.len() && (self.s[self.i].is_ascii_digit() || self.s[self.i] == b'.') {
                self.i += 1;
            }
            let v: f64 = std::str::from_utf8(&self.s[st..self.i]).ok()?.parse().ok()?;
            self.out.push(Op::Num(v));
            return Some(());
        }
        if c.is_ascii_alphabetic() {
            let st = self.i;
            while self.i < self.s.len() && (self.s[self.i].is_ascii_alphanumeric() || self.s[self.i] == b'_') {
                self.i += 1;
            }
            let name = std::str::from_utf8(&self.s[st..self.i]).ok()?;
            let arg = match name {
                "x" => Some(0),
                "y" => Some(1),
                "z" => Some(2),
                _ if name.len() == 2 && name.starts_with('a') => name[1..].parse::<usize>().ok(),
                _ => None,
            };
            if let Some(a) = arg {
                self.out.push(Op::Arg(a));
                return Some(());
            }
            let f = FUNCS.iter().position(|(n, _)| *n == name)? as u8;
            if !self.eat("(") {
                return None;
            }
            let mut n = 0u8;
            if !self.eat(")") {
                loop {
                    self.expr()?;
                    n += 1;
                    if self.eat(")") {
                        break;
                    }
                    if !self.eat(",") {
                        return None;
                    }
                }
            }
            if n != FUNCS[f as usize].1 {
                return None;
            }
            self.out.push(Op::Call(f, n));
            return Some(());
        }
        None
    }
}

/// Builtin `(name, arity)` table indexed by `Op::Call`.
const FUNCS: [(&str, u8); 13] = [
    ("min", 2), ("max", 2), ("abs", 1), ("floor", 1), ("ceil", 1), ("round", 1), ("sqrt", 1),
    ("pow", 2), ("sin", 1), ("cos", 1), ("exp", 1), ("ln", 1), ("clamp", 3),
];

impl Expr {
    /// Parses `text` into bytecode; invalid syntax compiles to constant `NaN`.
    pub fn new(text: &str) -> Expr {
        let mut p = P { s: text.as_bytes(), i: 0, out: Vec::new() };
        let ok = p.expr().is_some() && {
            p.ws();
            p.i == p.s.len()
        };
        Expr { code: if ok { p.out } else { vec![Op::Num(f64::NAN)] } }
    }

    /// Evaluates the compiled bytecode with positional `args` (`x` = `args[0]`,
    /// `y` = `args[1]`, `z` = `args[2]`, `aN` = `args[N]`).
    pub fn eval(&self, args: &[f64]) -> f64 {
        let mut st: Vec<f64> = Vec::with_capacity(8);
        let b = |v: bool| if v { 1. } else { 0. };
        for op in &self.code {
            match *op {
                Op::Num(v) => st.push(v),
                Op::Arg(a) => st.push(args.get(a).copied().unwrap_or(f64::NAN)),
                Op::Neg => {
                    let v = st.pop().unwrap_or(f64::NAN);
                    st.push(-v)
                }
                Op::Not => {
                    let v = st.pop().unwrap_or(f64::NAN);
                    st.push(b(v == 0.))
                }
                Op::Pick => {
                    let (y, x, c) = (st.pop().unwrap_or(f64::NAN), st.pop().unwrap_or(f64::NAN), st.pop().unwrap_or(f64::NAN));
                    st.push(if c != 0. { x } else { y })
                }
                Op::Bin(o) => {
                    let (r, l) = (st.pop().unwrap_or(f64::NAN), st.pop().unwrap_or(f64::NAN));
                    st.push(match o {
                        b'+' => l + r,
                        b'-' => l - r,
                        b'*' => l * r,
                        b'/' => l / r,
                        b'%' => l % r,
                        b'^' => l.powf(r),
                        b'<' => b(l < r),
                        b'>' => b(l > r),
                        b'l' => b(l <= r),
                        b'g' => b(l >= r),
                        b'=' => b(l == r),
                        b'n' => b(l != r),
                        b'&' => b(l != 0. && r != 0.),
                        _ => b(l != 0. || r != 0.),
                    })
                }
                Op::Call(f, n) => {
                    let mut a = [0f64; 3];
                    for k in (0..n as usize).rev() {
                        a[k] = st.pop().unwrap_or(f64::NAN);
                    }
                    st.push(match FUNCS[f as usize].0 {
                        "min" => a[0].min(a[1]),
                        "max" => a[0].max(a[1]),
                        "abs" => a[0].abs(),
                        "floor" => a[0].floor(),
                        "ceil" => a[0].ceil(),
                        "round" => a[0].round(),
                        "sqrt" => a[0].sqrt(),
                        "pow" => a[0].powf(a[1]),
                        "sin" => a[0].sin(),
                        "cos" => a[0].cos(),
                        "exp" => a[0].exp(),
                        "ln" => a[0].ln(),
                        _ => a[0].max(a[1]).min(a[2]),
                    })
                }
            }
        }
        st.pop().unwrap_or(f64::NAN)
    }
}

impl<T: Wire> Wire for std::sync::Arc<[T]> {
    fn put(&self, out: &mut Vec<u8>) {
        (**self).put(out);
    }
}

impl<T: Wire> Wire for std::rc::Rc<[T]> {
    fn put(&self, out: &mut Vec<u8>) {
        (**self).put(out);
    }
}

impl Wire for std::num::NonZeroU32 {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u32(self.get());
    }
}

impl Wire for std::num::NonZeroUsize {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.get() as u64);
    }
}

// Host-side formula closures cannot be serialized back into source text;
// getters write an empty placeholder string to preserve wire layout.
impl Wire for std::rc::Rc<dyn Fn(f32) -> f32> {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str("");
    }
}

impl Wire for std::rc::Rc<dyn Fn(f64, gpui_kit::base::StepAction, &mut gpui_kit::App) -> f64> {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str("");
    }
}
