//! The editor family (docs/PROTOCOLS.md sections 9.11 and
//! 9.12): what the generator's own rules need at run time (a `char` on the wire), the editor's
//! providers that are not the input family's, and the data closures. Its
//! numbers: call 176-207, shape 44-51, node 40-47, hkind 12 (shared with shim_select.rs).

use crate::genapi::{Call, GkResult, Put, R, Unwire, Wire};

/// A `char` travels as its scalar value, a u32 (the generator's `char` rule: a mask's separator,
/// a mask token). Go: `rune`.
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

/// The family's call ops; None: not one of them.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    use crate::ops::*;
    Some(match op {
        C_LEND_TEST => {
            // -> u64 objects lent, u64 given back (docs/PROTOCOLS.md 9.17)
            let (lent, back) = LENT.with(|l| l.get());
            c.out.put_u64(lent);
            c.out.put_u64(back);
            Ok(())
        }
        C_HIGHLIGHTER_NEW => (|| {
            // u64 styles ask, u64 folds ask (0: none), u32 n, str language each (none: any).
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
            // u64 handle (the syntax ask, or a row source of the provider's own), u8 has syntax,
            // u32 n, (str alias, str canonical name) each.
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
        // core/src/editor_text.rs, docs/PROTOCOLS.md 9.20 to 9.26
        _ => return crate::editor_text::call(op, r, c),
    })
}

// --- objects lent by kit state (docs/PROTOCOLS.md 9.17) ---------------------------------------

thread_local! {
    /// Objects lent, and given back (the tests' LEND_TEST).
    static LENT: std::cell::Cell<(u64, u64)> = const { std::cell::Cell::new((0, 0)) };
}

/// What a generated lend returns (`InputBaseState::diagnostics_mut`): `copy`, the object the state
/// lends, behind a new temporary handle the app reads and writes like any object of its type.
/// The copy is given back to the state, through `acc` (the same method), when the turn's buffered
/// writes have landed, and the state redraws; the handle is released then. A turn's writes are
/// applied when the outermost update has ended (rt::process_reply defers them after the turn), so
/// the give-back is deferred twice to come after them. None when the state lends nothing.
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

fn give_back<S: 'static, T: Clone + 'static>(state: u64, h: u64, acc: fn(&mut S) -> Option<&mut T>, cx: &mut gpui_kit::App) {
    // Released or given away by the app in the meantime: there is nothing to give back.
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

// --- delegates answered between frames (docs/PROTOCOLS.md 9.18) -------------------------------

use gpui_kit::base::input::{EditorState, FoldRange, HighlightStyleResolver, InputEdit, InputHighlighter};
use gpui_kit::base::input::{LanguageProvider, SyntaxContext, SyntaxContextProvider};
use std::cell::RefCell;
use std::ops::Range;
use std::rc::Rc;

/// A highlighter the app made (`gk.highlighter`): its styles procedure and its folds procedure as
/// asks (9.14), and the languages it highlights (none: any).
struct Highlighter {
    styles: crate::shim_text::Ask,
    folds: Option<crate::shim_text::Ask>,
    languages: Vec<String>,
}

thread_local! {
    static HIGHLIGHTERS: RefCell<std::collections::HashMap<u64, Rc<Highlighter>>> = RefCell::new(std::collections::HashMap::new());
    static LANGUAGES: RefCell<std::collections::HashMap<u64, Rc<GkLanguages>>> = RefCell::new(std::collections::HashMap::new());
}

/// `InputHighlighterFactory` over the highlighter the app made (`[lists] delegates`): a
/// `GkHighlighter` for each editor whose language the highlighter takes.
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

/// One editor's highlighter: the editor's text as of its last update, and the app's answers for
/// ranges of that text. A range without an answer yet is drawn unstyled, and the app is asked
/// between frames (Highlight_Query); every window redraws when the answer is in.
struct GkHighlighter {
    h: Rc<Highlighter>,
    lang: gpui_kit::SharedString,
    text: String,
}

fn read_runs(r: &mut R, _dx: &mut crate::genapi::Dx) -> Option<Vec<(Range<usize>, String)>> {
    let n = r.u32() as usize;
    let mut v = Vec::with_capacity(n.min(4096));
    for _ in 0..n {
        let (a, b) = (r.u64() as usize, r.u64() as usize);
        v.push((a..b, r.str().to_string()));
    }
    Some(v)
}

fn read_folds(r: &mut R, _dx: &mut crate::genapi::Dx) -> Option<Vec<(usize, usize)>> {
    let n = r.u32() as usize;
    let mut v = Vec::with_capacity(n.min(4096));
    for _ in 0..n {
        v.push((r.u64() as usize, r.u64() as usize));
    }
    Some(v)
}

/// Ordered runs that cover `range` exactly, as InputHighlighter::styles promises: the app's named
/// runs clamped to it, through the editor's resolver, the gaps unstyled.
fn cover(
    range: &Range<usize>,
    mut runs: Vec<(Range<usize>, String)>,
    resolver: &dyn HighlightStyleResolver,
) -> Vec<(Range<usize>, gpui_kit::HighlightStyle)> {
    runs.sort_by_key(|(r, _)| r.start);
    let mut out = Vec::with_capacity(runs.len() * 2 + 1);
    let mut at = range.start;
    for (r, name) in runs {
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
            // Answers about the old text are for a text that is gone.
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

/// A language provider the app made (`gk.language_provider`): aliases of language names, and a
/// syntax procedure answered between frames (SyntaxContext::Code until the answer is in).
struct GkLanguages {
    aliases: std::collections::HashMap<String, String>,
    syntax: Option<crate::shim_text::Ask>,
}

/// `Rc<dyn LanguageProvider>` over the provider the app made (`[lists] delegates`).
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
        // Syntax_Context's numbers (api/opcodes.lock): Code 0, String 1, Comment 2.
        match self.ask.get(key, read_syntax, |v| v, || 0) {
            1 => SyntaxContext::String,
            2 => SyntaxContext::Comment,
            _ => SyntaxContext::Code,
        }
    }
}

/// `gk.Font`: str family, f32 weight (0: upstream's normal), u16 style (Font_Style: 0 normal, 1
/// italic, 2 oblique). Features and fallbacks are `gpui::font`'s.
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

/// An editor's text read back (`InputBaseState::text`, `selected_text`, `DisplayMap::text`): the
/// whole text as a string.
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

/// A handler given to a call op (`InputState::validate`, `set_validator`): the app kept it
/// (`keep_handler`, as for a subscription) and sends its slot and generation; it is fired as a
/// kept handler (view 0), with the window when upstream's closure has one.
pub fn kept(r: &mut R) -> crate::genapi::HandlerRef {
    let (handler, generation) = (r.u32(), r.u32());
    crate::genapi::HandlerRef { view: 0, generation, handler }
}

// --- expression closures (docs/PROTOCOLS.md 9.12) --------------------------------------------

/// A closure that maps numbers to a number (`step_by(Fn(f64, StepAction, &mut App) -> f64)`,
/// `ease(Fn(f32) -> f32)`, `heading_font_size(Fn(u8, Pixels) -> Pixels)`), given as an
/// expression the library evaluates: the app's code never runs while upstream calls it (in the
/// middle of a frame, or inside the kit's own update). The arguments are `x`, `y`, `z` (or `a0` to
/// `a9`); an enum argument is its variant's number, a bool 0 or 1. Operators `+ - * / % ^`,
/// comparisons and `&& || !` (0 is false), `c ? a : b`, and `min max abs floor ceil round sqrt
/// pow sin cos exp ln clamp`. The binding checked the text (`putExpr` in go/gk); a
/// text that does not parse evaluates to NaN.
pub struct Expr {
    code: Vec<Op>,
}

#[derive(Clone, Copy, Debug)]
enum Op {
    Num(f64),
    Arg(usize),
    Neg,
    Not,
    Bin(u8),
    Call(u8, u8),
    /// Pops c, b, a: c != 0 ? a : b would need laziness; both sides are pure, so eager is fine.
    Pick,
}

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

const FUNCS: [(&str, u8); 13] = [
    ("min", 2), ("max", 2), ("abs", 1), ("floor", 1), ("ceil", 1), ("round", 1), ("sqrt", 1),
    ("pow", 2), ("sin", 1), ("cos", 1), ("exp", 1), ("ln", 1), ("clamp", 3),
];

impl Expr {
    pub fn new(text: &str) -> Expr {
        let mut p = P { s: text.as_bytes(), i: 0, out: Vec::new() };
        let ok = p.expr().is_some() && {
            p.ws();
            p.i == p.s.len()
        };
        Expr { code: if ok { p.out } else { vec![Op::Num(f64::NAN)] } }
    }

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

// Std shapes of options (docs/PROTOCOLS.md 9.12): a list behind a pointer and a non-zero number
// travelling out (an enum read back whose variant holds one).
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

// An expression closure read back (an `Easing` or a `NumberStep` that holds one): its text is not
// kept, so it travels out as an empty expression.
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
