//! Cached asynchronous host-query bridge (`Ask`), text/URI wire codecs, and
//! list-constructor indexing helpers.
//!
//! Some gpui-kit traits (such as syntax highlighters, syntax-context providers,
//! and markdown/text data callbacks) invoke synchronous closures without an
//! `App` reference or from background threads. `Ask` decouples those calls from
//! the host round trip: a cache miss immediately returns a fallback value and
//! enqueues `(src, key)` onto a global unbounded channel drained on the GPUI
//! foreground executor, which asks the host via `SHAPE_ASK_EVENT`, caches the
//! decoded reply, and refreshes windows.

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

/// Runs `f` with a `Dx` attached to the first open window and a minimal static
/// empty tape, allowing `Ask` reply decoders that need `Dx` to decode values
/// outside of a normal view render pass.
fn with_dx<T>(cx: &mut App, f: impl FnOnce(&mut Dx) -> T) -> Option<T> {
    thread_local! {
        // Minimal valid GKT1 tape header (34 bytes: magic, version=1, zero counts/offsets).
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

/// Per-`Ask` reply cache and in-flight deduplication state.
#[derive(Default)]
struct Cache {
    answers: HashMap<Vec<u8>, Decoded>,
    asked: HashSet<Vec<u8>>,
    decoders: HashMap<Vec<u8>, Decoder>,
}

/// Registry of active `Ask` caches keyed by source handle.
static ASKS: Mutex<Option<HashMap<u64, Arc<Mutex<Cache>>>>> = Mutex::new(None);
/// Sender feeding cache-miss `(src, key)` requests to the foreground pump task.
static PUMP: Mutex<Option<UnboundedSender<(u64, Vec<u8>)>>> = Mutex::new(None);
/// Global diagnostic counters `(gets, misses, decoded_answers)` read by `C_ASK_TEST`.
static STATS: Mutex<(u64, u64, u64)> = Mutex::new((0, 0, 0));

/// Thread-safe handle to a host-backed cached query closure.
#[derive(Clone)]
pub struct Ask {
    src: u64,
    cache: Option<Arc<Mutex<Cache>>>,
}

impl Ask {
    /// Attaches to the `Ask` cache created for source handle `src` by `C_ASK_NEW`.
    pub fn take(src: u64) -> Ask {
        let cache = ASKS.lock().unwrap().as_ref().and_then(|m| m.get(&src).cloned());
        if cache.is_none() && src != 0 {
            eprintln!("gk: a data closure was given a handle {src:#x} gk.NewAsk did not make; it answers nothing");
        }
        Ask { src, cache }
    }

    /// Looks up `key` in the cache, returning `conv(cached)` on hit or
    /// scheduling a single asynchronous host query and returning `fallback()` on miss.
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
    /// Discards all cached answers and in-flight keys so subsequent lookups
    /// re-query the host.
    pub fn clear(&self) {
        if let Some(cache) = &self.cache {
            let mut c = cache.lock().unwrap();
            c.answers.clear();
            c.asked.clear();
            c.decoders.clear();
        }
    }

    /// Reports whether this handle resolved to a live `C_ASK_NEW` cache.
    pub fn is_live(&self) -> bool {
        self.cache.is_some()
    }
}

/// Serializes `args` into a byte vector suitable as an `Ask::get` cache key.
pub fn key<P: Wire + ?Sized>(args: &P) -> Vec<u8> {
    let mut out = Vec::with_capacity(32);
    args.put(&mut out);
    out
}

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

/// Issues a `SHAPE_ASK_EVENT` request to the host and spawns a foreground task
/// to store the decoded reply when it arrives.
fn ask_app(src: u64, key: Vec<u8>, cx: &mut App) {
    let rx = match source::request(src, SHAPE_ASK_EVENT, &AskKey { src, key: &key }, cx) {
        Ok(rx) => rx,
        Err(_) => return,
    };
    cx.spawn(async move |cx| {
        let Ok(bytes) = rx.await else { return };
        let _ = cx.update(|cx| {
            crate::rt::guard("decoding a data closure's answer", || answered(src, key, &bytes, cx));
        });
    })
    .detach();
}

/// Decodes a reply for `(src, key)`, inserts it into the cache, and triggers a
/// window refresh so the UI repaints with the real value.
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

/// Converts a host string into a `gpui_kit::ImageSource`, treating strings
/// containing `"://"` as URIs and all others as filesystem paths.
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

impl Wire for gpui_kit::base::text::CodeBlock {
    fn put(&self, out: &mut Vec<u8>) {
        self.lang().map(|s| s.to_string()).put(out);
        self.code().put(out);
        self.meta().map(|s| s.to_string()).put(out);
    }
}

/// Dispatches `C_ASK_NEW` (cache registration + one-time pump startup) and
/// `C_ASK_TEST` (stats readback). This is the tail of the call-dispatch chain.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_ASK_NEW => (|| {
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
            let (g, m, a) = *STATS.lock().unwrap();
            c.out.put_u64(g);
            c.out.put_u64(m);
            c.out.put_u64(a);
            Ok(())
        }
        _ => return None,
    })
}

/// Uniform adapter over `Vec<T>` and `Result<Vec<T>, E>` return types used by
/// generated indexed static constructors (`nth`).
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

/// Extracts item `index` from a `Listed` constructor result, stopping the app
/// with a diagnostic message if the constructor failed or `index` is out of bounds.
pub fn nth<L: Listed>(made: L, index: usize) -> L::Item {
    match made.listed() {
        Ok(mut v) if index < v.len() => v.swap_remove(index),
        Ok(v) => {
            crate::rt::app_stop(format!("value {index} of a static that made {} was asked for (ask its _count first)", v.len()))
        }
        Err(e) => {
            crate::rt::app_stop(format!("value {index} of a static that failed was asked for: {e} (its _count says so)"))
        }
    }
}

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

impl<T: Wire> Wire for std::sync::Arc<Vec<T>> {
    fn put(&self, out: &mut Vec<u8>) {
        self.as_slice().put(out);
    }
}

impl<T: Wire> Wire for std::rc::Rc<Vec<T>> {
    fn put(&self, out: &mut Vec<u8>) {
        self.as_slice().put(out);
    }
}
