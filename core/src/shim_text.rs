//! Data closures answered between frames (docs/PROTOCOLS.md 9.14).
//!
//! Upstream calls closures like `TextView::code_block_highlighter(Fn(&CodeBlock) -> Vec<(Range<usize>,
//! HighlightStyle)>)` or `image_source(Fn(&SharedUri) -> ImageSource)` while it lays out and draws,
//! without a window or an app. App code never runs in the middle of a frame (DESIGN.md section 0), so
//! the closure the library gives upstream answers from a cache held by an *ask*: a row source the
//! app made with `gk.ask(cx, state, procedure)` (call `ASK_NEW`), used for its request channel only
//! (section 9.2). A key the cache has not got returns the type's default (one frame or two of
//! placeholder: unhighlighted code, an empty image) and is sent to a main-thread task, which asks
//! the app (`ASK_EVENT`, `(u64 id, u64 ask, key bytes)`, a queued event, between frames); the app's
//! procedure returns the value, the binding answers it (`gk.answer`), and the task decodes it with
//! the app context (colours are resolved against the theme here), caches it under the key's bytes
//! and redraws every window (`refresh_windows`: the views' tapes are decoded again, so upstream gets
//! a new closure and asks again, now answered from the cache). A key is the closure's arguments in
//! their wire form, so the same code block or the same URI is asked once.

use crate::genapi::{Call, Dx, GkResult, Put, R, Wire};
use crate::tape::Tape;
use crate::ops::*;
use crate::source;
use futures::StreamExt;
use futures::channel::mpsc::{UnboundedSender, unbounded};
use gpui_kit::{App, AppContext};
use std::any::Any;
use std::rc::Rc;
use std::collections::{HashMap, HashSet};
use std::sync::{Arc, Mutex};

type Decoded = Arc<dyn Any + Send + Sync>;
type Decoder = Arc<dyn Fn(&[u8], &mut App) -> Option<Decoded> + Send + Sync>;

/// A decoding context over no tape, with the app's first window: an answer is a value with no
/// nodes, read by the generated record decoders (which resolve colours against the theme).
fn with_dx<T>(cx: &mut App, f: impl FnOnce(&mut Dx) -> T) -> Option<T> {
    thread_local! {
        // A tape of one empty node: the header (magic, 1 node, root 0, no handlers, deps, slots
        // or ops) and the node.
        static EMPTY: Rc<Tape> = Rc::new({
            let mut b = Vec::with_capacity(34);
            b.put_u32(0x3154_4b47);
            b.put_u32(1);
            b.put_u32(0);
            b.put_u32(0);
            b.put_u16(0);
            b.put_u16(0);
            b.put_u32(0);
            b.extend_from_slice(&[0; 10]);
            Tape::parse(b, 0).ok().expect("the empty tape")
        });
    }
    let tape = EMPTY.with(|t| t.clone());
    let w = cx.windows().into_iter().next()?;
    w.update(cx, |_, window, cx| crate::genapi::with_root(&tape, 0, window, cx, |dx, _, _| f(dx))).ok()
}

#[derive(Default)]
struct Cache {
    answers: HashMap<Vec<u8>, Decoded>,
    asked: HashSet<Vec<u8>>,
    decoders: HashMap<Vec<u8>, Decoder>,
}

static ASKS: Mutex<Option<HashMap<u64, Arc<Mutex<Cache>>>>> = Mutex::new(None);
static PUMP: Mutex<Option<UnboundedSender<(u64, Vec<u8>)>>> = Mutex::new(None);

/// The asks the library made (tests: `ASK_TEST`).
static STATS: Mutex<(u64, u64, u64)> = Mutex::new((0, 0, 0)); // gets, misses, answers

/// What a data closure holds: the ask's cache.
#[derive(Clone)]
pub struct Ask {
    src: u64,
    cache: Option<Arc<Mutex<Cache>>>,
}

impl Ask {
    /// The ask behind a row source handle (a tape's or a call's argument). A handle that names no
    /// ask answers every key with the default.
    pub fn take(src: u64) -> Ask {
        let cache = ASKS.lock().unwrap().as_ref().and_then(|m| m.get(&src).cloned());
        if cache.is_none() && src != 0 {
            eprintln!("gk: a data closure was given a handle {src:#x} gk.NewAsk did not make; it answers nothing");
        }
        Ask { src, cache }
    }

    /// The answer for `key` (the arguments' bytes), converted by `conv`; `fallback` while it has
    /// not arrived. `dec` reads the app's answer with the app context, in the task that receives it.
    pub fn get<S: Clone + Send + Sync + 'static, V>(
        &self,
        key: Vec<u8>,
        dec: fn(&mut R, &mut Dx) -> Option<S>,
        conv: fn(S) -> V,
        fallback: fn() -> V,
    ) -> V {
        STATS.lock().unwrap().0 += 1;
        let Some(cache) = &self.cache else {
            return fallback();
        };
        let mut c = cache.lock().unwrap();
        if let Some(v) = c.answers.get(&key) {
            if let Some(s) = v.downcast_ref::<S>() {
                return conv(s.clone());
            }
        }
        if c.asked.insert(key.clone()) {
            STATS.lock().unwrap().1 += 1;
            let d: Decoder = Arc::new(move |bytes: &[u8], cx: &mut App| {
                let mut r = R::new(bytes);
                with_dx(cx, |dx| dec(&mut r, dx)).flatten().map(|s| Arc::new(s) as Decoded)
            });
            c.decoders.insert(key.clone(), d);
            if let Some(tx) = PUMP.lock().unwrap().as_ref() {
                let _ = tx.unbounded_send((self.src, key));
            }
        }
        fallback()
    }
}

impl Ask {
    /// Forgets every answer and every key asked (a delegate whose text changed: the old answers
    /// are for a text that is gone, docs/PROTOCOLS.md 9.18). Answers on their way are dropped.
    pub fn clear(&self) {
        if let Some(cache) = &self.cache {
            let mut c = cache.lock().unwrap();
            c.answers.clear();
            c.asked.clear();
            c.decoders.clear();
        }
    }

    /// Whether this ask was made by gk.ask (a handle that names none answers nothing).
    pub fn is_live(&self) -> bool {
        self.cache.is_some()
    }
}

/// The key of a closure's arguments: their wire form.
pub fn key<P: Wire + ?Sized>(args: &P) -> Vec<u8> {
    let mut out = Vec::with_capacity(32);
    args.put(&mut out);
    out
}

/// The event payload after the request id: the ask's handle, then the key's bytes as they are
/// (the app reads them as the closure's argument types).
struct AskKey<'a> {
    src: u64,
    key: &'a [u8],
}

impl Wire for AskKey<'_> {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.src);
        out.extend_from_slice(self.key);
    }
}

/// Runs on the main thread, between frames: asks the app for one key.
fn ask_app(src: u64, key: Vec<u8>, cx: &mut App) {
    let rx = match source::request(src, SHAPE_ASK_EVENT, &AskKey { src, key: &key }, cx) {
        Ok(rx) => rx,
        Err(_) => return, // the ask was released: nothing more is asked
    };
    cx.spawn(async move |cx| {
        let Ok(bytes) = rx.await else { return };
        let _ = cx.update(|cx| {
            crate::rt::guard("decoding a data closure's answer", || answered(src, key, &bytes, cx));
        });
    })
    .detach();
}

fn answered(src: u64, key: Vec<u8>, bytes: &[u8], cx: &mut App) {
    let Some(cache) = ASKS.lock().unwrap().as_ref().and_then(|m| m.get(&src).cloned()) else {
        return;
    };
    let dec = cache.lock().unwrap().decoders.remove(&key);
    let Some(dec) = dec else { return };
    if let Some(v) = dec(bytes, cx) {
        cache.lock().unwrap().answers.insert(key, v);
        STATS.lock().unwrap().2 += 1;
        cx.refresh_windows();
    }
}

/// `ImageSource` as the app gives one: a URL (`scheme://...`) or a file path.
pub fn image_source(s: String) -> gpui_kit::ImageSource {
    if s.contains("://") {
        gpui_kit::ImageSource::from(gpui_kit::SharedUri::from(s))
    } else {
        gpui_kit::ImageSource::from(std::path::PathBuf::from(s))
    }
}

impl Wire for gpui_kit::SharedUri {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(self.as_ref());
    }
}

/// A fenced code block as the app's highlighter gets it (`gk.Code_Block`): its language, its
/// code and its info string after the language.
impl Wire for gpui_kit::base::text::CodeBlock {
    fn put(&self, out: &mut Vec<u8>) {
        self.lang().map(|s| s.to_string()).put(out);
        self.code().put(out);
        self.meta().map(|s| s.to_string()).put(out);
    }
}

pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_ASK_NEW => (|| {
            // u64 source (a row source made for its request channel): its cache, and the task
            // that asks the app, started once.
            let src = r.u64();
            source::count(src, 0)?;
            ASKS.lock().unwrap().get_or_insert_with(HashMap::new).insert(src, Arc::new(Mutex::new(Cache::default())));
            let mut pump = PUMP.lock().unwrap();
            if pump.is_none() {
                let (tx, mut rx) = unbounded::<(u64, Vec<u8>)>();
                *pump = Some(tx);
                c.cx
                    .spawn(async move |cx| {
                        while let Some((src, key)) = rx.next().await {
                            let _ = cx.update(|cx| {
                                crate::rt::guard("asking the app for a data closure's answer", || ask_app(src, key, cx));
                            });
                        }
                    })
                    .detach();
            }
            Ok(())
        })(),
        C_ASK_TEST => {
            // -> u64 gets, u64 misses (keys asked), u64 answers cached
            let (g, m, a) = *STATS.lock().unwrap();
            c.out.put_u64(g);
            c.out.put_u64(m);
            c.out.put_u64(a);
            Ok(())
        }
        _ => return None,
    })
}

// --- values a static makes several of (docs/PROTOCOLS.md 9.15) --------------------------------

/// What a static that makes several values returns: a list, or a list or an error.
pub trait Listed {
    type Item;
    fn listed(self) -> Result<Vec<Self::Item>, String>;
}

impl<T> Listed for Vec<T> {
    type Item = T;
    fn listed(self) -> Result<Vec<T>, String> {
        Ok(self)
    }
}

impl<T, E: std::fmt::Debug> Listed for Result<Vec<T>, E> {
    type Item = T;
    fn listed(self) -> Result<Vec<T>, String> {
        self.map_err(|e| format!("{e:?}"))
    }
}

/// The value at `index` of what the static made (`gk.diff_file_parse_at(patch, i)`). An index
/// past its count, or a static that failed, is a mistake in app code (the app asks
/// `..._count` first): the program stops with exit 2.
pub fn nth<L: Listed>(made: L, index: usize) -> L::Item {
    match made.listed() {
        Ok(mut v) if index < v.len() => v.swap_remove(index),
        Ok(v) => {
            eprintln!("gk: value {index} of a static that made {} was asked for (ask its _count first)", v.len());
            std::process::exit(2);
        }
        Err(e) => {
            eprintln!("gk: value {index} of a static that failed was asked for: {e} (its _count says so)");
            std::process::exit(2);
        }
    }
}

// --- values read back (docs/PROTOCOLS.md 9.13) ------------------------------------------------

/// An LSP position (`cursor_position`): `gk.Lsp_Position {line, character}`, both ways.
impl Wire for lsp_types::Position {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u32(self.line);
        out.put_u32(self.character);
    }
}

impl crate::genapi::Unwire for lsp_types::Position {
    fn get(r: &mut R) -> Self {
        let line = r.u32();
        lsp_types::Position::new(line, r.u32())
    }
}

/// `Arc<Vec<T>>` (a font's fallbacks and features): the list.
impl<T: Wire> Wire for std::sync::Arc<Vec<T>> {
    fn put(&self, out: &mut Vec<u8>) {
        self.as_slice().put(out);
    }
}

/// `Rc<Vec<T>>` read back (`SearchMatcher::matched_ranges`): the list.
impl<T: Wire> Wire for std::rc::Rc<Vec<T>> {
    fn put(&self, out: &mut Vec<u8>) {
        self.as_slice().put(out);
    }
}
