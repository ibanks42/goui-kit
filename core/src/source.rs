//! Virtualized row-source runtime (`HK_SOURCE`) for lists, tables, trees, and item closures.
//!
//! # Architecture and Row Lifecycle
//!
//! A [`Source`] bridges large host datasets (up to [`MAX_ROWS`] rows per section) to GPUI
//! virtualized components without materializing every row in Rust or Go:
//!
//! - **Key packing**: every row is addressed by `key(section, row) = (section << 32) | row`.
//!   Row indices `0xffff_fffe` ([`HEADER_ROW`]) and `0xffff_ffff` (footer) are reserved sentinels
//!   that bypass neighbor overscan expansion.
//! - **Two rendering modes**:
//!   - *View-backed* (`decoded == false`, used by `N_SOURCE_LIST` and `N_VIRTUAL_LIST`): each
//!     bound row owns a `GkView` entity looked up via [`row`].
//!   - *Decoded* (`decoded == true`, used by `ListDelegate`, `TableDelegate`, `TreeState`, and
//!     `items::row`): bound rows use lightweight `Item::DecodedRow` handles with no GPUI entity;
//!     [`row_tape`] hands back the cached `Rc<Tape>` directly for inline decoding.
//! - **Bind-ahead and prefetch throttling**: during layout/render, missing rows are recorded in
//!   `wanted` (visible misses) while `GkTableDelegate::bind_ahead` calls [`table_bind_ahead`] to
//!   queue nearby offscreen keys into `ahead`. Before the next frame paints, [`rows_turn`] plans
//!   binds and evictions for every source in the window and sends one batched `K_ROWS` turn:
//!   - Visible misses expand with an overscan margin `clamp(8, capacity / 4)` per section.
//!   - Pure prefetch turns (no visible misses) cap at `PREFETCH_PER_FRAME = 24` rows sorted
//!     nearest `ahead_anchor`, keeping per-frame Go work smooth while scrolling fast; `pre_frame`
//!     checks `has_pending_ahead` and schedules another frame until `ahead` drains.
//!   - When `rows.len() + bind.len() > capacity`, keys farthest from the midpoint of the wanted
//!     window are evicted, and their view handles are released *after* `K_ROWS` returns so the
//!     host processes eviction notices before Rust drops the handles.
//! - **`ROWS_EPOCH` fast skip**: every `rows_turn` and `C_SOURCE_SET_COUNT` bumps `ROWS_EPOCH`,
//!   allowing table delegates to skip redundant lookahead checks while the row cache is unchanged.
//! - **Context-parameterized closures (`ctx_row` / `ctx_key` / `C_CTX_GET`)**: upstream closures
//!   that pass rich per-item context structs store a serialized fingerprint + lazy context writer
//!   on the source. When a fingerprint changes, the bound row is dirtied immediately; unkeyed
//!   closures auto-increment `ctx_next` and reset it on `window.on_next_frame` via `ctx_armed`.

use crate::genapi::{self, HandlerRef, Wire};
use crate::ops::*;
use crate::rt::{self, GkErr, GkResult, IdMap, Item, err};
use crate::tape::Tape;
use crate::wire::{Put, R};
use futures::channel::oneshot;
use gpui_kit::{AnyView, App, EntityId, ScrollStrategy, UniformListScrollHandle, Window};
use std::cell::Cell;
use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::rc::Rc;

thread_local! {
    /// Monotonic epoch bumped whenever row bindings or counts change, letting callers skip
    /// redundant lookahead checks within the same state.
    static ROWS_EPOCH: Cell<u64> = const { Cell::new(1) };
}

/// Returns the current row-source mutation epoch.
#[inline]
pub fn rows_epoch() -> u64 {
    ROWS_EPOCH.with(|e| e.get())
}

fn bump_epoch() {
    ROWS_EPOCH.with(|e| e.set(e.get().wrapping_add(1)));
}

/// Packs `(section, row)` into a single 64-bit row key (`section << 32 | row`).
pub fn key(section: u32, row: u32) -> u64 {
    (section as u64) << 32 | row as u64
}

/// Maximum number of rows permitted per section (reserves [`HEADER_ROW`] and `u32::MAX`).
pub const MAX_ROWS: u64 = HEADER_ROW;

/// Rejects section sizes exceeding [`MAX_ROWS`] so `(section, row)` keys never collide with
/// [`HEADER_ROW`] or footer sentinels.
fn check_count(n: u64) -> GkResult<usize> {
    if n > MAX_ROWS {
        return err(ERR_BAD_OP, format!("a row source holds at most {MAX_ROWS} rows (a table: rows times columns) in a section; {n} were given"));
    }
    Ok(n as usize)
}

/// Sentinel row index reserved for section headers (`0xffff_fffe`; footer uses `0xffff_ffff`).
pub const HEADER_ROW: u64 = 0xffff_fffe;

/// Per-source state stored under `HK_SOURCE`.
pub struct Source {
    /// Owning window handle whose `pre_frame` runs `K_ROWS` for this source.
    pub window: u64,
    /// Row count per section.
    pub counts: Vec<usize>,
    /// Live bound rows: `row_key -> (view_handle, cached_tape)`.
    rows: IdMap<(u64, Option<Rc<Tape>>)>,
    /// Row keys requested during layout/render that were missing from `rows`.
    wanted: BTreeSet<u64>,
    /// Prefetch row keys queued ahead of the viewport by [`table_bind_ahead`].
    ahead: BTreeSet<u64>,
    ahead_anchor: u64,
    /// Maximum live row views kept in `rows` before distant keys are evicted.
    pub capacity: usize,
    /// Optional host `(slot, generation)` handler for delegate events and async requests.
    events: Option<(u32, u32)>,
    /// GPUI entity IDs notified whenever this source's rows or counts change.
    watchers: Vec<EntityId>,
    pub scroll: UniformListScrollHandle,
    /// Pending async host requests keyed by monotonically increasing request ID.
    pending: HashMap<u64, oneshot::Sender<Vec<u8>>>,
    next_request: u64,
    test_rx: Vec<oneshot::Receiver<Vec<u8>>>,
    miss_frame: u64,
    /// Set when any row missed since the last tape acceptance, triggering watcher notification.
    missed: bool,
    /// When `true`, rows decode directly from `Tape` via `Item::DecodedRow` without a `GkView` entity.
    pub decoded: bool,
    /// Context fingerprints and serializers for closure-backed rows (`C_CTX_GET`).
    ctxs: HashMap<u64, (Vec<u8>, std::rc::Rc<dyn Fn(&mut Vec<u8>)>)>,
    ctx_next: u32,
    ctx_armed: bool,
}

impl Source {
    /// Returns all currently bound row view handles (used to cascade release on `C_RELEASE`).
    pub fn views(&self) -> Vec<u64> {
        self.rows.values().map(|(v, _)| *v).collect()
    }

    pub(crate) fn has_ahead(&self) -> bool {
        !self.ahead.is_empty()
    }

    pub(crate) fn has_tape(&self, key: u64) -> bool {
        self.rows.get(&key).is_some_and(|(_, t)| t.is_some())
    }

    pub(crate) fn set_tape(&mut self, key: u64, tape: Rc<Tape>) {
        if let Some((_, slot)) = self.rows.get_mut(&key) {
            *slot = Some(tape);
        }
    }

    /// Computes `(keys_to_bind, view_handles_to_evict)` for the next `K_ROWS` turn.
    ///
    /// - When `wanted` is empty (pure prefetch) and `ahead` has more than `24 + 4` entries, only
    ///   the 24 keys nearest `ahead_anchor` are pulled this frame so scrolling does not stall on
    ///   a large prefetch burst; the remaining keys stay in `ahead` for subsequent frames.
    /// - When `wanted` is non-empty (visible misses), every section's `[lo..=hi]` miss range
    ///   expands by `over = clamp(miss_count, 8, capacity / 4)` rows so small scrolls immediately
    ///   hit warm rows.
    /// - If adding `bind` would exceed `capacity`, cached keys outside `want` are sorted by
    ///   distance from `want`'s midpoint and the farthest ones are evicted first.
    fn plan(&mut self) -> (Vec<u64>, Vec<u64>) {
        const PREFETCH_PER_FRAME: usize = 24;
        let wanted = std::mem::take(&mut self.wanted);
        self.ahead.retain(|k| !self.rows.contains_key(k));
        let mut want = if wanted.is_empty() && self.ahead.len() > PREFETCH_PER_FRAME + 4 {
            let anchor = self.ahead_anchor;
            let mut near: Vec<(u64, u64)> = self.ahead.iter().map(|&k| (k.abs_diff(anchor), k)).collect();
            near.sort_unstable();
            let mut batch = BTreeSet::new();
            for (_, k) in near.into_iter().take(PREFETCH_PER_FRAME) {
                self.ahead.remove(&k);
                batch.insert(k);
            }
            batch
        } else {
            std::mem::take(&mut self.ahead)
        };
        let mut by_section: BTreeMap<u64, (u64, u64, u64)> = BTreeMap::new();
        for k in &wanted {
            let row = k & 0xffff_ffff;
            if row >= HEADER_ROW {
                // Header and footer sentinel keys do not expand into neighbor ranges.
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
            // Evict cached rows farthest from the midpoint of the active window first.
            let mid = want.iter().nth(want.len() / 2).copied().unwrap_or(0);
            let mut far: Vec<(u64, u64)> = self
                .rows
                .keys()
                .filter(|k| !want.contains(k))
                .map(|k| (k.abs_diff(mid), *k))
                .collect();
            far.sort_unstable_by(|a, b| b.cmp(a));
            for (_, k) in far.into_iter().take(excess) {
                if let Some((v, _)) = self.rows.remove(&k) {
                    evict.push(v);
                }
            }
        }
        (bind, evict)
    }
}

/// Result of looking up a row in a view-backed row source.
pub enum Row {
    View(AnyView),
    Missing,
}

/// Fetches the rendered `AnyView` for `(src, key)`, recording a miss and scheduling `pre_frame`
/// if the row has not been bound or described yet.
pub fn row(src: u64, key: u64, window: &mut Window, cx: &mut App) -> Row {
    let found = rt::with(|s| -> GkResult<(Option<u64>, u64)> {
        let frame = s.frame_no();
        let source = s.source_mut(src)?;
        match source.rows.get(&key) {
            Some((v, _)) => Ok((Some(*v), source.window)),
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

/// Returns the current row count of `section` in source `src`.
pub fn count(src: u64, section: u32) -> GkResult<usize> {
    rt::with(|s| Ok(s.source_mut(src)?.counts.get(section as usize).copied().unwrap_or(0)))
}

/// Returns a clone of source `src`'s `UniformListScrollHandle`.
pub fn scroll_handle(src: u64) -> GkResult<UniformListScrollHandle> {
    rt::with(|s| Ok(s.source_mut(src)?.scroll.clone()))
}

/// Registers GPUI `entity` to be notified whenever source `src` receives updated rows or counts.
pub fn watch(src: u64, entity: EntityId) {
    rt::with(|s| {
        if let Ok(source) = s.source_mut(src) {
            if !source.watchers.contains(&entity) {
                source.watchers.push(entity);
            }
        }
    });
}

/// Clears and returns whether source `src` recorded a render miss since the last tape batch.
pub(crate) fn take_missed(src: u64) -> bool {
    rt::with(|s| s.source_mut(src).map(|x| std::mem::take(&mut x.missed)).unwrap_or(false))
}

/// Calls `cx.notify` on every GPUI entity watching any source in `sources`.
pub(crate) fn notify_watchers(sources: &[u64], cx: &mut App) {
    for src in sources {
        let ids = rt::with(|s| s.source_mut(*src).map(|x| x.watchers.clone()).unwrap_or_default());
        for id in ids {
            cx.notify(id);
        }
    }
}

/// Queues a source delegate event to the host handler attached to `src`, if any.
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

/// Sends an asynchronous request to `src`'s event handler and returns a oneshot receiver resolved
/// by `C_SOURCE_ANSWER`.
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

/// Runs one batched `K_ROWS` turn for every source owned by window `wh` with pending binds or evictions.
///
/// Request layout:
/// `[wh: u64][n_plans: u32]([src: u64][n_bind: u32][(key: u64, view: u64)*][n_evict: u32][view: u64*])*[n_dirty: u32][dirty: u64*][n_retired: u32][(view: u64, gen: u32)*]`.
///
/// Ordering invariants:
/// 1. Newly bound row views are allocated and inserted into `Source::rows` before calling `K_ROWS`
///    so the host receives their view handles and can immediately return their tapes in the reply.
/// 2. Any already-queued dirty views for `wh` are piggybacked in `pre_dirty` so a single `K_ROWS`
///    turn replaces a separate `K_VIEWS` round-trip.
/// 3. Evicted views are released only *after* `rt::turn(K_ROWS)` returns so their handles stay
///    valid if the host inspects them while processing the eviction list.
pub(crate) fn rows_turn(wh: u64, window: &mut Window, cx: &mut App) {
    let plans: Vec<(u64, Vec<u64>, Vec<u64>)> = rt::with(|s| {
        let mut plans = Vec::new();
        for h in s.sources.clone() {
            if let Ok(source) = s.source_mut(h) {
                if source.window == wh && (!source.wanted.is_empty() || !source.ahead.is_empty()) {
                    let (bind, evict) = source.plan();
                    if !bind.is_empty() || !evict.is_empty() {
                        plans.push((h, bind, evict));
                    }
                }
            }
        }
        plans
    });
    if plans.is_empty() {
        return;
    }
    let pre_dirty = rt::with(|s| s.dirty.remove(&wh).unwrap_or_default());
    let retired = rt::take_retired();
    let mut req = Vec::with_capacity(64 + pre_dirty.len() * 8 + retired.len() * 12);
    req.put_u64(wh);
    req.put_u32(plans.len() as u32);
    for (h, bind, evict) in &plans {
        req.put_u64(*h);
        req.put_u32(bind.len() as u32);
        for key in bind {
            let view = bind_row(*h, *key, wh, cx);
            req.put_u64(*key);
            req.put_u64(view);
        }
        req.put_u32(evict.len() as u32);
        for v in evict {
            req.put_u64(*v);
        }
    }
    req.put_u32(pre_dirty.len() as u32);
    for v in &pre_dirty {
        req.put_u64(*v);
    }
    req.put_u32(retired.len() as u32);
    for (view, generation) in retired {
        req.put_u64(view);
        req.put_u32(generation);
    }
    let mut reply = rt::turn(K_ROWS, &req, Some(&mut *window), cx);
    for (_, _, evict) in plans {
        for v in evict {
            let item = rt::with(|s| s.release(v));
            drop(item);
        }
    }
    rt::accept_tapes(wh, std::mem::take(&mut reply.tapes), cx);
    rt::process_reply(&mut reply, K_ROWS, Some(&mut *window), cx);
    bump_epoch();
    gk_meter::bump(|c| c.rows_turns += 1);
}

/// Allocates a row view (either `Item::DecodedRow` or `Item::View`), mounts it into `wh`, and
/// registers `(src, key)` in `Session::row_owner` and `Source::rows`.
fn bind_row(src: u64, key: u64, wh: u64, cx: &mut App) -> u64 {
    let decoded = rt::with(|s| s.source_mut(src).map(|x| x.decoded).unwrap_or(false));
    let view = if decoded { rt::new_decoded_row() } else { rt::new_view(cx) };
    rt::with(|s| {
        s.mount(view, wh);
        s.row_owner.insert(view, (src, key));
        if let Ok(source) = s.source_mut(src) {
            source.rows.insert(key, (view, None));
        }
    });
    view
}

/// Handles `C_SOURCE_*`, `C_CTX_GET`, and `C_TEST_SOURCE_*` wire calls.
///
/// - `C_SOURCE_NEW`: `[count: u64][initial: u32][capacity: u32][has_events: u8][slot: u32, gen: u32]?` -> `[src: u64][n: u32][(key: u64, view: u64)*]`
/// - `C_SOURCE_SET_COUNT`: `[src: u64][section: u32][count: u64]` (evicts bound rows `>= count`)
/// - `C_SOURCE_SCROLL_TO`: `[src: u64][row: u64]`
/// - `C_SOURCE_ANSWER`: `[src: u64][req_id: u64][len: u32][bytes...]`
/// - `C_CTX_GET`: `[src: u64][section: u32][row: u32]` -> serialized context payload
pub fn call(op: u16, r: &mut R, c: &mut genapi::Call) -> Option<GkResult<()>> {
    Some(match op {
        C_SOURCE_NEW => (|| {
            let count = check_count(r.u64())?;
            let initial = r.u32() as usize;
            let capacity = (r.u32() as usize).max(16);
            let events = (r.u8() != 0).then(|| (r.u32(), r.u32()));
            let any = c.window_handle().ok_or_else(|| {
                GkErr(
                    ERR_NO_CONTEXT,
                    "a row source needs a window: make it in a view's init function or in a handler".into(),
                )
            })?;
            let Some(wh) = rt::with(|s| s.window_of(any)) else {
                return err(ERR_NO_CONTEXT, "a row source needs a window opened by gk.OpenWindow");
            };
            let src = rt::with(|s| {
                let h = s.insert(
                    HK_SOURCE,
                    Item::Source(Box::new(Source {
                        window: wh,
                        counts: vec![count],
                        rows: IdMap::default(),
                        wanted: BTreeSet::new(),
                        ahead: BTreeSet::new(),
                        ahead_anchor: 0,
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
                let keys: Vec<u64> = source.rows.keys().copied().filter(|&k| k >= lo && k < hi).collect();
                Ok(keys.into_iter().filter_map(|k| source.rows.remove(&k).map(|(v, _)| v)).collect())
            })?;
            for v in gone {
                let item = rt::with(|s| s.release(v));
                drop(item);
            }
            bump_epoch();
            notify_watchers(&[src], c.cx);
            Ok(())
        })(),
        C_SOURCE_SCROLL_TO => (|| {
            let src = r.u64();
            let row = r.u64() as usize;
            scroll_handle(src)?.scroll_to_item(row, ScrollStrategy::Top);
            notify_watchers(&[src], c.cx);
            Ok(())
        })(),
        C_SOURCE_ANSWER => (|| {
            let (src, id) = (r.u64(), r.u64());
            let n = r.u32() as usize;
            let bytes = r.bytes(n).to_vec();
            let tx = rt::with(|s| -> GkResult<_> { Ok(s.source_mut(src)?.pending.remove(&id)) })?;
            match tx {
                Some(tx) => {
                    let _ = tx.send(bytes);
                    Ok(())
                }
                None => err(ERR_BAD_OP, format!("request {id} of this row source was answered already or never asked")),
            }
        })(),
        C_CTX_GET => ctx_get(r, c),
        C_TEST_SOURCE_REQUEST => (|| {
            let src = r.u64();
            let text = r.str().to_owned();
            let rx = request(src, SHAPE_SOURCE_TEST, &text, c.cx)?;
            rt::with(|s| -> GkResult<()> {
                s.source_mut(src)?.test_rx.push(rx);
                Ok(())
            })
        })(),
        C_TEST_SOURCE_TAKE => (|| {
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

/// Renders one context-parameterized row from `src`, storing `(print, write)` for `C_CTX_GET`.
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

/// Assigns or looks up a row key for a context-parameterized row, updating its cached context
/// and marking the row view dirty if `print` changed since the previous frame.
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
        Ok((k, if changed { source.rows.get(&k).map(|(v, _)| *v) } else { None }, arm))
    });
    let (k, stale, arm) = match found {
        Ok(x) => x,
        Err(GkErr(_, msg)) => genapi::released_shown(&msg, "row source"),
    };
    if arm {
        // Reset the auto-incrementing row counter once per frame so unkeyed closures stay aligned.
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
            format!("row {row} of this row source was never drawn by an element closure over a context (overlay.Items)"),
        ),
    }
}

/// Schedules lookahead row prefetching ahead of and behind the visible `rows` range of a table,
/// returning `(rows_epoch, covered_range)` so callers can skip repeat checks while unchanged.
///
/// Probes `2 * len` rows ahead and behind (`len = max(rows.len(), 24)`). If both boundary rows
/// are already bound or queued in `ahead`, returns immediately without touching `ahead`;
/// otherwise queues missing rows up to `rows.end + 2 * len + batch` (with `batch` capped at
/// `capacity / 4`) and arms `rt::ensure_hook` so `rows_turn` populates them on the next frame.
pub fn table_bind_ahead(
    src: u64,
    rows: std::ops::Range<usize>,
    window: &mut Window,
    cx: &mut App,
) -> Option<(u64, std::ops::Range<usize>)> {
    if rows.is_empty() {
        return None;
    }
    let epoch = rows_epoch();
    let len = rows.len().max(24);
    let res = rt::with(|s| -> Option<(Option<u64>, std::ops::Range<usize>)> {
        let source = s.source_mut(src).ok()?;
        let count = source.counts.first().copied().unwrap_or(0);
        if count == 0 {
            return None;
        }
        let ahead = (rows.end + 2 * len).min(count - 1);
        let behind = rows.start.saturating_sub(2 * len);
        let ahead_k = key(0, ahead as u32);
        let behind_k = key(0, behind as u32);
        let miss_ahead = !source.rows.contains_key(&ahead_k) && !source.ahead.contains(&ahead_k);
        let miss_behind = !source.rows.contains_key(&behind_k) && !source.ahead.contains(&behind_k);
        crate::rt::trace(|| format!("table rows={rows:?} ahead={ahead} behind={behind} missing={miss_ahead}/{miss_behind}"));
        if !miss_ahead && !miss_behind {
            return Some((None, behind..ahead + 1));
        }
        source.ahead_anchor = key(0, rows.end.min(count - 1) as u32);
        let batch = (2 * len).min((source.capacity / 4).max(12));
        let lo = behind;
        let hi = (rows.end + 2 * len + batch).min(count);
        let mut any = false;
        for row in lo..hi {
            let k = key(0, row as u32);
            if !source.rows.contains_key(&k) && source.ahead.insert(k) {
                any = true;
            }
        }
        Some((any.then_some(source.window), lo..hi))
    });
    if let Some((wh, range)) = res {
        if let Some(wh) = wh {
            rt::ensure_hook(wh, Some(window), cx);
        }
        return Some((epoch, range));
    }
    None
}

/// Fetches the cached `(Tape, view_handle)` for a decoded-mode row, recording a miss and
/// scheduling `pre_frame` if the tape is not present yet.
pub fn row_tape(src: u64, key: u64, window: &mut Window, cx: &mut App) -> Option<(Rc<Tape>, u64)> {
    let found = rt::with(|s| -> GkResult<(Option<(Rc<Tape>, u64)>, u64)> {
        let frame = s.frame_no();
        let source = s.source_mut(src)?;
        let wh = source.window;
        let (has_view, tape) = match source.rows.get(&key) {
            Some((v, Some(t))) => (true, Some((t.clone(), *v))),
            Some((_, None)) => (true, None),
            None => (false, None),
        };
        if tape.is_none() {
            if !has_view {
                source.wanted.insert(key);
            }
            source.missed = true;
            crate::rt::trace(|| format!("row placeholder source={src} key={key:#x} frame={frame}"));
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

/// Switches `src` to decoded-row mode (tapes decoded inline without a `GkView` entity per row)
/// and converts any still-empty bound rows to `Item::DecodedRow`.
pub fn set_decoded(src: u64) {
    let convert: Vec<u64> = rt::with(|s| {
        let Ok(source) = s.source_mut(src) else { return Vec::new() };
        if source.decoded {
            return Vec::new();
        }
        source.decoded = true;
        source.rows.values().filter_map(|(v, t)| t.is_none().then_some(*v)).collect()
    });
    if !convert.is_empty() {
        rt::convert_to_decoded(&convert);
    }
}

/// Decodes an `N_VIRTUAL_LIST` node (vertical or horizontal) backed by a row source and per-item sizes.
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
