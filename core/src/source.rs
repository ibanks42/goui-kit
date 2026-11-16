//! Row sources: the shared core of the delegate protocol (docs/PROTOCOLS.md section 9).
//!
//! A row source holds the rows of one list, table or tree. Each row is a view of its own (a
//! `GkView`, described by the app's row procedure for that row and retained like any view).
//! A shim that implements a kit delegate (ListDelegate, TableDelegate, the tree's render
//! closure) asks the source for rows while GPUI draws (`row`), and never calls the app: a row
//! that has no view yet is drawn as a placeholder and recorded as wanted. Before the next
//! frame the pre-frame hook runs one ROWS turn (`rows_turn`), which binds new row views to
//! the wanted rows and evicts far ones, and the VIEWS turn right after describes them; the
//! entities that drew the source (`watch`) are then notified, so that same frame shows them.
//! A far jump therefore shows placeholders for exactly one frame, and app code never runs in
//! the middle of a frame.
//!
//! What the kit asks synchronously (counts, column definitions) is data the app pushed;
//! what it reports (selection, confirm, sort) is a queued event (`event`); what it asks
//! asynchronously (completions, hover) is a request answered later (`request`, `answer`).

use crate::genapi::{self, HandlerRef, Wire};
use crate::ops::*;
use crate::rt::{self, GkErr, GkResult, Item, err};
use crate::wire::{Put, R};
use futures::channel::oneshot;
use gpui_kit::{AnyView, App, EntityId, ScrollStrategy, UniformListScrollHandle, Window};
use std::collections::{BTreeMap, BTreeSet, HashMap};

/// A row's key: `section << 32 | row`. Lists without sections and tables use section 0.
pub fn key(section: u32, row: u32) -> u64 {
    (section as u64) << 32 | row as u64
}

/// The most rows a section can hold: a row's number is 32 bits, and the two highest are a
/// section's header and footer. Counts above it are refused (a table's count is its cells).
pub const MAX_ROWS: u64 = HEADER_ROW;

fn check_count(n: u64) -> GkResult<usize> {
    if n > MAX_ROWS {
        return err(ERR_BAD_OP, format!("a row source holds at most {MAX_ROWS} rows (a table: rows times columns) in a section; {n} were given"));
    }
    Ok(n as usize)
}

/// The row number of a section's header row; `HEADER_ROW + 1` is its footer row (the list
/// delegate's, docs/PROTOCOLS.md section 9.5). Bound when drawn, never as part of a range.
pub const HEADER_ROW: u64 = 0xffff_fffe;

pub struct Source {
    /// The window the rows are drawn in (a handle of kind WINDOW).
    pub window: u64,
    /// Rows per section.
    pub counts: Vec<usize>,
    /// Bound rows: key to row view.
    rows: BTreeMap<u64, u64>,
    /// Keys asked for while drawing that have no row view yet.
    wanted: BTreeSet<u64>,
    /// Most row views kept at once; far ones are evicted beyond it.
    pub capacity: usize,
    /// The kept handler that receives this source's events (slot, generation).
    events: Option<(u32, u32)>,
    /// Entities that drew this source, notified when rows arrive.
    watchers: Vec<EntityId>,
    /// The scroll handle of the core's own list element (hand.rs).
    pub scroll: UniformListScrollHandle,
    pending: HashMap<u64, oneshot::Sender<Vec<u8>>>,
    next_request: u64,
    /// Receivers the runtime's tests read back (C_TEST_SOURCE_TAKE).
    test_rx: Vec<oneshot::Receiver<Vec<u8>>>,
    miss_frame: u64,
    /// A row was drawn as a placeholder since the watchers were last notified: they must draw
    /// again when rows arrive. Rows bound ahead (`want`) were never drawn, and their arrival
    /// alone redraws nothing.
    missed: bool,
    /// A delegate shim decodes this source's rows itself (`row_tape`), never drawing their
    /// views: the runtime then leaves the rows' entities alone (`rt::accept`).
    pub decoded: bool,
    /// An element closure over a context (docs/PROTOCOLS.md 9.16): what each row was last
    /// handed (its identity's bytes, and a writer of the values as the row's view reads them),
    /// the next row of this frame's calls, and whether the reset of that count is armed.
    ctxs: HashMap<u64, (Vec<u8>, std::rc::Rc<dyn Fn(&mut Vec<u8>)>)>,
    ctx_next: u32,
    ctx_armed: bool,
}

impl Source {
    /// Every row view this source holds.
    pub fn views(&self) -> Vec<u64> {
        self.rows.values().copied().collect()
    }

    /// Rows to bind and row views to evict for what was wanted since the last ROWS turn: the
    /// wanted rows of each section and as many again on each side (a viewport of overscan
    /// each way), the farthest rows evicted beyond `capacity`.
    fn plan(&mut self) -> (Vec<u64>, Vec<u64>) {
        let wanted = std::mem::take(&mut self.wanted);
        let mut want = BTreeSet::new();
        let mut by_section: BTreeMap<u64, (u64, u64, u64)> = BTreeMap::new(); // lo, hi, n
        for k in &wanted {
            let row = k & 0xffff_ffff;
            if row >= HEADER_ROW {
                // A section's header or footer row (rows family): bound as asked, never a range.
                want.insert(*k);
                continue;
            }
            let e = by_section.entry(k >> 32).or_insert((u64::MAX, 0, 0));
            e.0 = e.0.min(row);
            e.1 = e.1.max(row);
            e.2 += 1;
        }
        let cap = self.capacity as u64;
        for (section, (lo, hi, n)) in by_section {
            let count = self.counts.get(section as usize).copied().unwrap_or(0) as u64;
            let over = n.clamp(8, (cap / 4).max(8));
            let (lo, hi) = (lo.saturating_sub(over), (hi + over + 1).min(count));
            for row in lo..hi {
                want.insert(section << 32 | row);
            }
        }
        let bind: Vec<u64> = want.iter().copied().filter(|k| !self.rows.contains_key(k)).collect();
        let mut evict = Vec::new();
        let excess = (self.rows.len() + bind.len()).saturating_sub(self.capacity);
        if excess > 0 {
            let mid = want.iter().nth(want.len() / 2).copied().unwrap_or(0);
            let mut far: Vec<(u64, u64)> = self
                .rows
                .keys()
                .filter(|k| !want.contains(k))
                .map(|k| (k.abs_diff(mid), *k))
                .collect();
            far.sort_unstable_by(|a, b| b.cmp(a));
            for (_, k) in far.into_iter().take(excess) {
                if let Some(v) = self.rows.remove(&k) {
                    evict.push(v);
                }
            }
        }
        (bind, evict)
    }
}

// --- what a shim calls while GPUI draws ---------------------------------------------------

/// A row as a shim draws it.
pub enum Row {
    /// The row's view, described and retained.
    View(AnyView),
    /// No view yet: draw a placeholder of the row's height. The row is asked for before the
    /// next frame.
    Missing,
}

/// The row of `key`. Never calls the app. A source that has been released is a mistake in
/// app code (a view still shows it) and stops the program, as a released state does.
pub fn row(src: u64, key: u64, window: &mut Window, cx: &mut App) -> Row {
    let found = rt::with(|s| -> GkResult<(Option<u64>, u64)> {
        let frame = s.frame_no();
        let source = s.source_mut(src)?;
        match source.rows.get(&key) {
            Some(v) => Ok((Some(*v), source.window)),
            None => {
                source.wanted.insert(key);
                source.missed = true;
                if source.miss_frame != frame {
                    source.miss_frame = frame;
                    gk_meter::bump(|c| c.placeholder_frames += 1);
                }
                Ok((None, source.window))
            }
        }
    });
    let (view, wh) = match found {
        Ok(found) => found,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "row source"),
    };
    if let Some(v) = view {
        if let Ok(entity) = rt::with(|s| s.view(v)) {
            if entity.read(cx).tape.is_some() {
                return Row::View(entity.into());
            }
        }
    }
    gk_meter::bump(|c| c.rows_missing += 1);
    rt::ensure_hook(wh, Some(window), cx);
    Row::Missing
}

/// Rows in `section` (0 for a source without sections).
pub fn count(src: u64, section: u32) -> GkResult<usize> {
    rt::with(|s| Ok(s.source_mut(src)?.counts.get(section as usize).copied().unwrap_or(0)))
}

/// The scroll handle of the core's own list element.
pub fn scroll_handle(src: u64) -> GkResult<UniformListScrollHandle> {
    rt::with(|s| Ok(s.source_mut(src)?.scroll.clone()))
}

/// Asks for `entity` to be notified when rows of `src` arrive (the entity that draws them: a
/// ListState, a TableState, a view whose tape holds the list).
pub fn watch(src: u64, entity: EntityId) {
    rt::with(|s| {
        if let Ok(source) = s.source_mut(src) {
            if !source.watchers.contains(&entity) {
                source.watchers.push(entity);
            }
        }
    });
}

/// Whether `src`'s rows were drawn as placeholders since this was last asked (and forgets it).
pub(crate) fn take_missed(src: u64) -> bool {
    rt::with(|s| s.source_mut(src).map(|x| std::mem::take(&mut x.missed)).unwrap_or(false))
}

/// Whether `src`'s row views are decoded by a shim rather than drawn as views.
pub(crate) fn is_decoded(src: u64) -> bool {
    rt::with(|s| s.source_mut(src).map(|x| x.decoded).unwrap_or(false))
}

pub(crate) fn notify_watchers(sources: &[u64], cx: &mut App) {
    for src in sources {
        let ids = rt::with(|s| s.source_mut(*src).map(|x| x.watchers.clone()).unwrap_or_default());
        for id in ids {
            cx.notify(id);
        }
    }
}

// --- events and requests -----------------------------------------------------------------

/// Delivers an event of the source (a selection, a confirm, a sort) to the handler the app
/// gave the source, after the kit's update has ended and before the next draw: the `queued`
/// resolution of DESIGN.md section 12. `shape` is the delegate family's event shape and
/// `payload` its union (u16 variant, then the variant's fields). No handler: dropped.
pub fn event<P: Wire + ?Sized>(src: u64, shape: u16, payload: &P, cx: &mut App) {
    let Some((slot, generation)) = rt::with(|s| s.source_mut(src).ok().and_then(|x| x.events)) else {
        return;
    };
    let h = HandlerRef {
        view: 0,
        generation,
        handler: slot,
    };
    genapi::fire_queued(h, shape, payload, cx);
}

/// Asks the app something it answers later (a provider method that returns a Task): the
/// event `payload` goes out with a new request id first, `(u64 id, payload..)`, and the
/// receiver gets the bytes of the app's answer (`gk.answer`). Dropping the receiver cancels
/// nothing on the app's side; the answer is then dropped.
pub fn request<P: Wire + ?Sized>(
    src: u64,
    shape: u16,
    payload: &P,
    cx: &mut App,
) -> GkResult<oneshot::Receiver<Vec<u8>>> {
    let (tx, rx) = oneshot::channel();
    let id = rt::with(|s| -> GkResult<u64> {
        let source = s.source_mut(src)?;
        source.next_request += 1;
        source.pending.insert(source.next_request, tx);
        Ok(source.next_request)
    })?;
    event(src, shape, &(id, payload), cx);
    Ok(rx)
}

// --- the ROWS turn ----------------------------------------------------------------------

/// Runs in the pre-frame hook of window `wh`, after the FRAME turn and before the VIEWS turn:
/// binds row views to the rows drawn as placeholders in the last frame (and their overscan),
/// evicts far rows, and marks the new row views dirty so the VIEWS turn describes them.
pub(crate) fn rows_turn(wh: u64, window: &mut Window, cx: &mut App) {
    let plans: Vec<(u64, Vec<u64>, Vec<u64>)> = rt::with(|s| {
        let mut plans = Vec::new();
        for h in s.sources.clone() {
            if let Ok(source) = s.source_mut(h) {
                if source.window == wh && !source.wanted.is_empty() {
                    let (bind, evict) = source.plan();
                    plans.push((h, bind, evict));
                }
            }
        }
        plans
    });
    if plans.is_empty() {
        return;
    }
    let mut req = Vec::with_capacity(64);
    req.put_u64(wh);
    req.put_u32(plans.len() as u32);
    let mut fresh = Vec::new();
    for (h, bind, evict) in &plans {
        req.put_u64(*h);
        req.put_u32(bind.len() as u32);
        for key in bind {
            let view = bind_row(*h, *key, wh, cx);
            req.put_u64(*key);
            req.put_u64(view);
            fresh.push(view);
        }
        req.put_u32(evict.len() as u32);
        for v in evict {
            req.put_u64(*v);
        }
    }
    let mut reply = rt::turn(K_ROWS, &req, Some(&mut *window), cx);
    rt::process_reply(&mut reply, K_ROWS, Some(&mut *window), cx);
    for (_, _, evict) in plans {
        for v in evict {
            let item = rt::with(|s| s.release(v));
            drop(item);
        }
    }
    for v in fresh {
        rt::mark_dirty(v, Some(&mut *window), cx);
    }
    gk_meter::bump(|c| c.rows_turns += 1);
}

/// A new row view for `key` of `src`, drawn in window `wh`.
fn bind_row(src: u64, key: u64, wh: u64, cx: &mut App) -> u64 {
    let view = rt::new_view(cx);
    rt::with(|s| {
        s.mount(view, wh);
        s.row_owner.insert(view, src);
        if let Ok(source) = s.source_mut(src) {
            source.rows.insert(key, view);
        }
    });
    view
}

// --- call ops -----------------------------------------------------------------------------

pub fn call(op: u16, r: &mut R, c: &mut genapi::Call) -> Option<GkResult<()>> {
    Some(match op {
        C_SOURCE_NEW => (|| {
            // u64 count, u32 initial, u32 capacity, u8 has events, u32 slot, u32 generation.
            // Out: u64 source, u32 n, (u64 key, u64 view) for the first rows, already marked
            // dirty, so the first frame that draws the source is complete.
            let count = check_count(r.u64())?;
            let initial = r.u32() as usize;
            let capacity = (r.u32() as usize).max(16);
            let events = (r.u8() != 0).then(|| (r.u32(), r.u32()));
            let any = c.window_handle().ok_or_else(|| {
                GkErr(
                    ERR_NO_CONTEXT,
                    "a row source needs a window: make it in a view's init procedure or in a handler".into(),
                )
            })?;
            let Some(wh) = rt::with(|s| s.window_of(any)) else {
                return err(ERR_NO_CONTEXT, "a row source needs a window opened by gk.open_window");
            };
            let src = rt::with(|s| {
                let h = s.insert(
                    HK_SOURCE,
                    Item::Source(Box::new(Source {
                        window: wh,
                        counts: vec![count],
                        rows: BTreeMap::new(),
                        wanted: BTreeSet::new(),
                        capacity,
                        events,
                        watchers: Vec::new(),
                        scroll: UniformListScrollHandle::new(),
                        pending: HashMap::new(),
                        next_request: 0,
                        test_rx: Vec::new(),
                        miss_frame: u64::MAX,
                        missed: false,
                        decoded: false,
                        ctxs: HashMap::new(),
                        ctx_next: 0,
                        ctx_armed: false,
                    })),
                );
                s.sources.push(h);
                h
            });
            let n = initial.min(count).min(capacity);
            c.out.put_u64(src);
            c.out.put_u32(n as u32);
            for row in 0..n {
                let k = key(0, row as u32);
                let view = bind_row(src, k, wh, c.cx);
                c.out.put_u64(k);
                c.out.put_u64(view);
                let window = c.window_opt();
                rt::mark_dirty(view, window, c.cx);
            }
            Ok(())
        })(),
        C_SOURCE_SET_COUNT => (|| {
            // u64 source, u32 section, u64 count. Rows at and past the count are released
            // here; the binding forgot them when it wrote the op.
            let (src, section, n) = (r.u64(), r.u32() as usize, r.u64());
            let n = check_count(n)?;
            let gone = rt::with(|s| -> GkResult<Vec<u64>> {
                let source = s.source_mut(src)?;
                if source.counts.len() <= section {
                    source.counts.resize(section + 1, 0);
                }
                source.counts[section] = n;
                let lo = key(section as u32, n.min(u32::MAX as usize) as u32);
                let hi = key(section as u32 + 1, 0);
                let keys: Vec<u64> = source.rows.range(lo..hi).map(|(k, _)| *k).collect();
                Ok(keys.into_iter().filter_map(|k| source.rows.remove(&k)).collect())
            })?;
            for v in gone {
                let item = rt::with(|s| s.release(v));
                drop(item);
            }
            notify_watchers(&[src], c.cx);
            Ok(())
        })(),
        C_SOURCE_SCROLL_TO => (|| {
            // u64 source, u64 row: the core list's scroll handle.
            let src = r.u64();
            let row = r.u64() as usize;
            scroll_handle(src)?.scroll_to_item(row, ScrollStrategy::Top);
            notify_watchers(&[src], c.cx);
            Ok(())
        })(),
        C_SOURCE_ANSWER => (|| {
            // u64 source, u64 request id, u32 len, bytes: the app's answer, in the wire type
            // the request's family fixed.
            let (src, id) = (r.u64(), r.u64());
            let n = r.u32() as usize;
            let bytes = r.bytes(n).to_vec();
            let tx = rt::with(|s| -> GkResult<_> { Ok(s.source_mut(src)?.pending.remove(&id)) })?;
            match tx {
                Some(tx) => {
                    let _ = tx.send(bytes); // the asker may have gone: then the answer is dropped
                    Ok(())
                }
                None => err(ERR_BAD_OP, format!("request {id} of this row source was answered already or never asked")),
            }
        })(),
        C_CTX_GET => ctx_get(r, c),
        C_TEST_SOURCE_REQUEST => (|| {
            // u64 source, str text: a request with the test shape; its receiver is kept for
            // C_TEST_SOURCE_TAKE.
            let src = r.u64();
            let text = r.str().to_owned();
            let rx = request(src, SHAPE_SOURCE_TEST, &text, c.cx)?;
            rt::with(|s| -> GkResult<()> {
                s.source_mut(src)?.test_rx.push(rx);
                Ok(())
            })
        })(),
        C_TEST_SOURCE_TAKE => (|| {
            // u64 source. Out: u8 1 and the answer as a str, or u8 0 while none has arrived.
            let src = r.u64();
            let got = rt::with(|s| -> GkResult<Option<Vec<u8>>> {
                let source = s.source_mut(src)?;
                let Some(rx) = source.test_rx.first_mut() else {
                    return Ok(None);
                };
                match rx.try_recv() {
                    Ok(Some(bytes)) => {
                        drop(source.test_rx.remove(0));
                        Ok(Some(bytes))
                    }
                    _ => Ok(None),
                }
            })?;
            match got {
                Some(bytes) => {
                    let text = R::new(&bytes).str().to_owned();
                    c.out.put_u8(1);
                    c.out.put_str(&text);
                }
                None => c.out.put_u8(0),
            }
            Ok(())
        })(),
        _ => return None,
    })
}

// --- element closures over a context (docs/PROTOCOLS.md 9.16) ----------------------

/// What a generated element closure over a context returns: the row view of source `src` for
/// this call. Each call in a frame is a row, in the order of the calls (the count starts again
/// at the next frame), or row `by` when the closure's only value is an index. `print` is the
/// identity of the values the kit handed (their bytes; an object's reads that take nothing):
/// when a bound row was handed other values its view is described again before the next frame,
/// and this frame shows the old one. `write` writes the values as `gk.row_context` reads them
/// (an object as a copy behind a temporary handle). Never calls the app: a row without a view
/// is drawn as nothing for one frame, as any row.
pub fn ctx_row(
    src: u64,
    by: Option<u64>,
    print: Vec<u8>,
    write: std::rc::Rc<dyn Fn(&mut Vec<u8>)>,
    window: &mut Window,
    cx: &mut App,
) -> gpui_kit::AnyElement {
    use gpui_kit::IntoElement as _;
    let k = ctx_key(src, by, print, write, window, cx);
    match row(src, k, window, cx) {
        Row::View(v) => v.into_any_element(),
        Row::Missing => gpui_kit::div().into_any_element(),
    }
}

/// The row of `src` this call of an element closure over a context is (`ctx_row`'s bookkeeping,
/// shared with the item procedures of core/src/items.rs): the drawing view watches the source,
/// the values are kept for `gk.row_context`, and a bound row handed other values is marked for a
/// new description before the next frame.
pub fn ctx_key(
    src: u64,
    by: Option<u64>,
    print: Vec<u8>,
    write: std::rc::Rc<dyn Fn(&mut Vec<u8>)>,
    window: &mut Window,
    cx: &mut App,
) -> u64 {
    watch(src, window.current_view());
    let found = rt::with(|s| -> GkResult<(u64, Option<u64>, bool)> {
        let source = s.source_mut(src)?;
        let row = match by {
            Some(i) => i.min(MAX_ROWS - 1),
            None => {
                let n = source.ctx_next;
                source.ctx_next = n.saturating_add(1).min(MAX_ROWS as u32 - 1);
                n as u64
            }
        };
        let arm = by.is_none() && !std::mem::replace(&mut source.ctx_armed, true);
        let k = key(0, row as u32);
        if source.counts.is_empty() {
            source.counts.push(0);
        }
        if source.counts[0] <= row as usize {
            source.counts[0] = row as usize + 1;
        }
        let changed = source.ctxs.get(&k).is_none_or(|(p, _)| *p != print);
        source.ctxs.insert(k, (print, write));
        Ok((k, if changed { source.rows.get(&k).copied() } else { None }, arm))
    });
    let (k, stale, arm) = match found {
        Ok(x) => x,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "row source"),
    };
    if arm {
        // The calls of the next frame count from 0 again.
        window.on_next_frame(move |_, _| {
            rt::with(|s| {
                if let Ok(source) = s.source_mut(src) {
                    source.ctx_next = 0;
                    source.ctx_armed = false;
                }
            })
        });
    }
    if let Some(v) = stale {
        rt::mark_dirty(v, Some(&mut *window), cx);
    }
    k
}

/// C_CTX_GET: u64 source, u32 section, u32 row -> the values the row was last handed (immediate;
/// allowed in a view procedure: a row's view reads them while it is described).
fn ctx_get(r: &mut R, c: &mut genapi::Call) -> GkResult<()> {
    let (src, section, row) = (r.u64(), r.u32(), r.u32());
    let write = rt::with(|s| -> GkResult<_> { Ok(s.source_mut(src)?.ctxs.get(&key(section, row)).map(|x| x.1.clone())) })?;
    match write {
        Some(w) => {
            w(&mut c.out);
            Ok(())
        }
        None => err(
            ERR_BAD_OP,
            format!("row {row} of this row source was never drawn by an element closure over a context (gk.row_context)"),
        ),
    }
}

// --- rows a shim will draw soon (added by the rows family, 11 Nov 2026) ---------------------

/// Asks for rows `lo ..< hi` of `section` to be bound before the next frame without drawing
/// them: a shim that knows what it will draw next (the rows beside the visible range while a
/// table scrolls) calls this, so a steady scroll binds rows ahead and never draws a
/// placeholder. Rows that have a view are skipped; nothing calls the app; a released source
/// is ignored here (drawing it stops the program in `row`).
pub fn want(src: u64, section: u32, lo: u32, hi: u32, window: &mut Window, cx: &mut App) {
    let wh = rt::with(|s| -> Option<u64> {
        let source = s.source_mut(src).ok()?;
        let count = source.counts.get(section as usize).copied().unwrap_or(0) as u32;
        let mut any = false;
        for row in lo..hi.min(count) {
            let k = key(section, row);
            if !source.rows.contains_key(&k) && source.wanted.insert(k) {
                any = true;
            }
        }
        any.then_some(source.window)
    });
    if let Some(wh) = wh {
        rt::ensure_hook(wh, Some(window), cx);
    }
}

/// Whether row `key` of `src` has a view now (bound; its tape may not have arrived yet).
pub fn bound(src: u64, key: u64) -> bool {
    rt::with(|s| s.source_mut(src).map(|x| x.rows.contains_key(&key)).unwrap_or(false))
}

/// The tape of row `key` of `src` and its view's handle, for a shim that decodes the row itself
/// (`genapi::with_tape`) instead of drawing its view. The row's entity is not read: in gpui-fast
/// an entity read while a view draws makes that view depend on the entity, and a table would
/// depend on every cell it drew. None when the row has no view or no tape yet: it is then a
/// placeholder for one frame and wanted, as in `row`. A released source stops the program.
pub fn row_tape(src: u64, key: u64, window: &mut Window, cx: &mut App) -> Option<(std::rc::Rc<crate::tape::Tape>, u64)> {
    let found = rt::with(|s| -> GkResult<(Option<(std::rc::Rc<crate::tape::Tape>, u64)>, u64)> {
        let frame = s.frame_no();
        let source = s.source_mut(src)?;
        let wh = source.window;
        let view = source.rows.get(&key).copied();
        let tape = view.and_then(|v| s.row_tapes.get(&v).map(|t| (t.clone(), v)));
        if tape.is_none() {
            let source = s.source_mut(src)?;
            if view.is_none() {
                source.wanted.insert(key);
            }
            source.missed = true;
            if source.miss_frame != frame {
                source.miss_frame = frame;
                gk_meter::bump(|c| c.placeholder_frames += 1);
            }
        }
        Ok((tape, wh))
    });
    match found {
        Ok((Some(t), _)) => Some(t),
        Ok((None, wh)) => {
            gk_meter::bump(|c| c.rows_missing += 1);
            rt::ensure_hook(wh, Some(window), cx);
            None
        }
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "row source"),
    }
}

/// Marks `src` as a source whose rows a shim decodes (`Source::decoded`).
pub fn set_decoded(src: u64) {
    rt::with(|s| {
        if let Ok(source) = s.source_mut(src) {
            source.decoded = true;
        }
    });
}

/// gpui-kit's `v_virtual_list` / `h_virtual_list` over a row source (docs/PROTOCOLS.md 13.31): the
/// rows protocol with a size per row. Upstream calls the list's closure while it lays out, with the
/// range it shows; the closure hands back each row's view, or a placeholder of the row's size for
/// a row without one yet (asked for before the next frame, as every row source does), so app code
/// never runs mid-frame. The list's own view is the view that draws it (upstream's `Entity<V>`).
/// Args: str id, u64 source, u8 axis (0 vertical: each row's height is used; 1 horizontal: its
/// width), u32 n, n x (f32 width, f32 height). The sizes beyond the source's count are not drawn,
/// rows beyond the sizes neither (upstream's count is the number of sizes). Then style ops.
pub fn virtual_list(dx: &mut genapi::Dx, args: u32, first: u32) -> gpui_kit::AnyElement {
    use gpui_kit::{IntoElement, ParentElement, Styled};
    let mut a = dx.args(args);
    let id = gpui_kit::SharedString::from(a.str());
    let src = a.u64();
    let horizontal = a.u8() == 1;
    let n = a.u32();
    let mut sizes: Vec<gpui_kit::Size<gpui_kit::Pixels>> = (0..n)
        .map(|_| {
            let w = a.f32();
            gpui_kit::size(gpui_kit::px(w), gpui_kit::px(a.f32()))
        })
        .collect();
    let count = match count(src, 0) {
        Ok(c) => c,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "row source"),
    };
    sizes.truncate(count);
    let view = match rt::with(|s| s.view(dx.view_handle())) {
        Ok(v) => v,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "view"),
    };
    // The view that draws the list is notified when the rows it drew as placeholders arrive.
    watch(src, view.entity_id());
    let sizes = std::rc::Rc::new(sizes);
    let each = sizes.clone();
    let rows = move |_: &mut rt::GkView, range: std::ops::Range<usize>, window: &mut Window, cx: &mut gpui_kit::Context<rt::GkView>| {
        range
            .map(|ix| {
                let s = each[ix];
                let cell = if horizontal { gpui_kit::div().w(s.width) } else { gpui_kit::div().h(s.height) };
                match row(src, key(0, ix as u32), window, cx) {
                    Row::View(v) => cell.child(v).into_any_element(),
                    Row::Missing => cell.into_any_element(),
                }
            })
            .collect::<Vec<_>>()
    };
    let mut el = if horizontal {
        gpui_kit::base::h_virtual_list(view, id, sizes, rows)
    } else {
        gpui_kit::base::v_virtual_list(view, id, sizes, rows)
    };
    let mut at = first;
    while let Some((op, mut r)) = dx.op(&mut at) {
        if !crate::generated::style_op(el.style(), op, &mut r, dx) {
            eprintln!("gk: op {op} is not a virtual list op");
        }
    }
    el.into_any_element()
}
