//! Hand-written `Wire`/`Unwire` codecs, view-scoped temporary handle pins, mutable value cells,
//! drag payloads, date-matcher layout bridging, value-set predicates, and questionnaire validators.
//!
//! # Key Protocols and Invariants
//!
//! - **Wire codecs**: covers upstream and standard types that the generator cannot synthesize
//!   struct-field codecs for (`Result` via [`Fallible`], deterministically sorted `HashMap`,
//!   `Duration` as clamped `i64` nanoseconds, `Entity<T>` via canonical `HK_STATE` handles,
//!   `jiff::civil::{Date, Time, DateTime, Weekday}`, `Drag`, `FocusHandle`, `Background`, and
//!   `IntervalMatcher`/`RangeMatcher`).
//! - **Build-scoped pins (`C_OBJ_PIN`)**: views that store a temporary `HK_OBJ` handle across
//!   frames pin it to `(view, build_id)`. Pinning promotes the handle out of `objects::TEMPS`
//!   (or rescues it from `LATER` if re-pinned in a new build), while handles from superseded
//!   builds move to `LATER` and are released on the next `objects::sweep()`.
//! - **Value cells (`C_CELL_*`)**: mutable byte-buffer objects (`ValueCell`) backing reactive
//!   closures like `SettingField`. [`cell_read`] pads the cloned bytes with 64 zero bytes so
//!   decoders reading from an uninitialized or shorter cell observe typed defaults safely.
//! - **Private-field matchers (`Part`)**: upstream `IntervalMatcher` and `RangeMatcher` hold two
//!   private `Option<jiff::civil::Date>` fields. Compile-time size/align/`!needs_drop` assertions
//!   paired with the one-time runtime canary [`parts_readable`] guard the raw `ptr::read` before
//!   any production matcher is serialized.
//! - **Value-set readback (`VALUE_SETS`)**: predicate closures created by [`value_set`] register
//!   their original serialized list under the boxed closure's heap address so reading the
//!   predicate back across the wire recovers the exact set of elements.

use crate::genapi::Wire;
use crate::ops::*;
use crate::rt::{self, GkResult, err};
use crate::wire::{Put, R};
use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

/// Wire adapter encoding a `Result<T, E>` as `[ok: u8][has_val: u8][val?][err_str]`.
///
/// The duplicate `ok`/`has_val` bytes match the generated optional-result envelope expected by
/// Go callers (`(value, ok, err)`).
pub struct Fallible<T, E>(pub Result<T, E>);

impl<T: Wire, E: std::fmt::Debug> Wire for Fallible<T, E> {
    fn put(&self, out: &mut Vec<u8>) {
        match &self.0 {
            Ok(v) => {
                out.put_u8(1);
                out.put_u8(1);
                v.put(out);
                out.put_str("");
            }
            Err(e) => {
                out.put_u8(0);
                out.put_u8(0);
                out.put_str(&format!("{e:?}"));
            }
        }
    }
}

/// Encodes `HashMap<K, V>` as `[count: u32][(key, value)*]` sorted lexicographically by encoded
/// bytes so wire snapshots and tests remain deterministic across Rust's randomized hash seeds.
impl<K: Wire, V: Wire, S> Wire for HashMap<K, V, S> {
    fn put(&self, out: &mut Vec<u8>) {
        // Sort serialized key-value entries for deterministic wire output across hash seeds.
        let mut pairs: Vec<Vec<u8>> = self
            .iter()
            .map(|(k, v)| {
                let mut b = Vec::new();
                k.put(&mut b);
                v.put(&mut b);
                b
            })
            .collect();
        pairs.sort();
        out.put_u32(pairs.len() as u32);
        for p in pairs {
            out.extend_from_slice(&p);
        }
    }
}

/// Encodes `Duration` as signed little-endian nanoseconds (`i64`, matching Go's `time.Duration`),
/// saturating at `i64::MAX` for durations longer than ~292 years.
impl Wire for std::time::Duration {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_i64(i64::try_from(self.as_nanos()).unwrap_or(i64::MAX));
    }
}

/// Encodes any GPUI `Entity<T>` as its canonical deduplicated `HK_STATE` `u64` handle.
impl<T: 'static> Wire for gpui_kit::Entity<T> {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(entity_handle(self));
    }
}

/// Returns or creates the canonical `HK_STATE` handle for a GPUI `Entity<T>`.
pub fn entity_handle<T: 'static>(e: &gpui_kit::Entity<T>) -> u64 {
    crate::dock::entity_handle(e)
}

/// Wire layout for `jiff::civil::Date`: `[year: i32][month: u32][day: u32]`.
impl Wire for jiff::civil::Date {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_i32(self.year().into());
        out.put_u32(self.month() as u32);
        out.put_u32(self.day() as u32);
    }
}

fn civil_date(y: i32, m: u32, d: u32) -> Option<jiff::civil::Date> {
    jiff::civil::Date::new(i16::try_from(y).ok()?, i8::try_from(m).ok()?, i8::try_from(d).ok()?).ok()
}

impl crate::genapi::Unwire for jiff::civil::Date {
    fn get(r: &mut R) -> Self {
        let (y, m, d) = (r.i32(), r.u32(), r.u32());
        civil_date(y, m, d).unwrap_or_default()
    }
}

/// Wire layout for `jiff::civil::Time`: `[hour: u32][minute: u32][second: u32][subsec_ns: u32]`.
impl Wire for jiff::civil::Time {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u32(self.hour() as u32);
        out.put_u32(self.minute() as u32);
        out.put_u32(self.second() as u32);
        out.put_u32(self.subsec_nanosecond() as u32);
    }
}

fn civil_time(h: u32, m: u32, s: u32, n: u32) -> Option<jiff::civil::Time> {
    let b = |v: u32| i8::try_from(v).ok();
    jiff::civil::Time::new(b(h)?, b(m)?, b(s)?, i32::try_from(n).ok()?).ok()
}

impl crate::genapi::Unwire for jiff::civil::Time {
    fn get(r: &mut R) -> Self {
        let (h, m, s, n) = (r.u32(), r.u32(), r.u32(), r.u32());
        civil_time(h, m, s, n).unwrap_or_default()
    }
}

/// Wire layout for `jiff::civil::DateTime`: `Date` immediately followed by `Time`.
impl Wire for jiff::civil::DateTime {
    fn put(&self, out: &mut Vec<u8>) {
        self.date().put(out);
        self.time().put(out);
    }
}

impl crate::genapi::Unwire for jiff::civil::DateTime {
    fn get(r: &mut R) -> Self {
        let d = jiff::civil::Date::get(r);
        d.to_datetime(jiff::civil::Time::get(r))
    }
}

/// Wire layout for `jiff::civil::Weekday`: `u8` zero-based offset from Monday (`0 = Monday .. 6 = Sunday`).
impl Wire for jiff::civil::Weekday {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u8(self.to_monday_zero_offset() as u8);
    }
}

impl crate::genapi::Unwire for jiff::civil::Weekday {
    fn get(r: &mut R) -> Self {
        jiff::civil::Weekday::from_monday_zero_offset((r.u8() % 7) as i8).unwrap_or(jiff::civil::Weekday::Monday)
    }
}

thread_local! {
    /// Maps each view handle to `(current_build_id, pinned_temp_handles)` kept alive while that
    /// view build still references those temporary objects.
    static PINS: RefCell<HashMap<u64, (u32, Vec<u64>)>> = RefCell::new(HashMap::new());
    /// Previously pinned handles superseded by a newer view build, awaiting the next frame sweep.
    static LATER: RefCell<Vec<u64>> = const { RefCell::new(Vec::new()) };
}

/// Pins temporary object handle `h` to `(view, build)` so it survives across frames while `build`
/// remains current; older builds' pins move to `LATER` for release on the next sweep.
///
/// Handles that are re-pinned during a rebuild are rescued from `LATER` before `objects::sweep`
/// runs; brand-new handles are removed from `objects::TEMPS` via `untemp`. Permanent handles
/// (not in `LATER` and not in `TEMPS`) are ignored so pinning never takes ownership of retained objects.
fn pin(view: u64, build: u32, h: u64) {
    let old = PINS.with(|p| {
        let mut p = p.borrow_mut();
        let e = p.entry(view).or_insert((build, Vec::new()));
        if e.0 != build {
            e.0 = build;
            std::mem::take(&mut e.1)
        } else {
            Vec::new()
        }
    });
    if !old.is_empty() {
        LATER.with(|l| l.borrow_mut().extend(old));
        HAS_PINS.store(true, std::sync::atomic::Ordering::Relaxed);
    }
    let again = if HAS_PINS.load(std::sync::atomic::Ordering::Relaxed) {
        LATER.with(|l| {
            let mut l = l.borrow_mut();
            let n = l.len();
            l.retain(|x| *x != h);
            if l.is_empty() {
                HAS_PINS.store(false, std::sync::atomic::Ordering::Relaxed);
            }
            l.len() != n
        })
    } else {
        false
    };
    if again || crate::objects::untemp(h) {
        PINS.with(|p| {
            let mut p = p.borrow_mut();
            let e = p.entry(view).or_insert((build, Vec::new()));
            if !e.1.contains(&h) {
                e.1.push(h);
            }
        });
    }
}

static HAS_PINS: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);

/// Drains expired pinned handles for [`crate::objects::sweep`].
pub fn sweep_pins() -> Vec<u64> {
    if !HAS_PINS.swap(false, std::sync::atomic::Ordering::Relaxed) {
        return Vec::new();
    }
    LATER.with(|l| std::mem::take(&mut *l.borrow_mut()))
}

#[inline]
pub fn has_pending_pins() -> bool {
    HAS_PINS.load(std::sync::atomic::Ordering::Relaxed)
}

/// Handles value-family wire calls:
/// - `C_VALUE_ACTION`: `[len: u32][mini_tape...]` -> `[action_id: u32]` (resolves an action value's ID)
/// - `C_OBJ_PIN`: `[view: u64][build: u32][obj: u64]` (pins a temp handle to `(view, build)`)
/// - `C_CELL_NEW`: `[len: u32][bytes...]` -> `[cell_handle: u64]`
/// - `C_CELL_SET`: `[cell: u64][len: u32][bytes...]` (replaces bytes and refreshes windows)
/// - `C_CELL_GET`: `[cell: u64]` -> `[len: u32][bytes...]`
/// - `C_HANDLES_TEST`: allocates one dummy entity and checks `Wire` vs `dock::put_entity` dedup
/// - `C_VALUES_TEST`: returns `(pinned_count: u32, later_count: u32)` for pin-sweep tests
pub fn call(op: u16, r: &mut R, c: &mut crate::genapi::Call) -> Option<GkResult<()>> {
    match op {
        C_VALUE_ACTION => Some((|| {
            let n = r.u32() as usize;
            let tape = Rc::new(crate::tape::Tape::parse(r.bytes(n).to_vec(), 0)?);
            let got = crate::objects::with_window(c, |window, cx| {
                crate::genapi::with_tape(&tape, 0, window, cx, |dx, root| crate::generated::value_action(dx, root))
            })?;
            match got {
                Some(action) => {
                    c.out.put_u32(crate::app::id_of(&*action));
                    Ok(())
                }
                None => err(ERR_BAD_OP, "gk.ActionOf: the value is not one of the kit's action types"),
            }
        })()),
        C_OBJ_PIN => {
            let (view, build, h) = (r.u64(), r.u32(), r.u64());
            pin(view, build, h);
            Some(Ok(()))
        }
        C_CELL_NEW => {
            let n = r.u32() as usize;
            let bytes = r.bytes(n).to_vec();
            c.out.put_u64(crate::objects::obj_new(ValueCell(bytes)));
            Some(Ok(()))
        }
        C_CELL_SET => Some((|| {
            let h = r.u64();
            let n = r.u32() as usize;
            let bytes = r.bytes(n).to_vec();
            let rc = cell(h)?;
            rc.try_borrow_mut()
                .map_err(|_| rt::GkErr(ERR_BUSY, "the value cell is being read by the kit".into()))?
                .0 = bytes;
            c.cx.refresh_windows();
            Ok(())
        })()),
        C_CELL_GET => Some((|| {
            let rc = cell(r.u64())?;
            let b = rc.try_borrow().map_err(|_| rt::GkErr(ERR_BUSY, "the value cell is being written".into()))?;
            c.out.put_u32(b.0.len() as u32);
            c.out.extend_from_slice(&b.0);
            Ok(())
        })()),
        C_HANDLES_TEST => {
            let e = gpui_kit::AppContext::new(c.cx, |_| HandlesTestEntity);
            let mut a = Vec::new();
            Wire::put(&e, &mut a);
            let mut b = Vec::new();
            crate::dock::put_entity(&e, &mut b);
            let mut again = Vec::new();
            Wire::put(&e, &mut again);
            let h = |v: &[u8]| u64::from_le_bytes(v[..8].try_into().unwrap_or([0; 8]));
            let (ha, hb, hc) = (h(&a), h(&b), h(&again));
            let n = [ha, hb, hc].iter().collect::<std::collections::HashSet<_>>().len();
            c.out.put_u64(ha);
            c.out.put_u64(hb);
            c.out.put_u64(hc);
            c.out.put_u64(n as u64);
            Some(Ok(()))
        }
        C_VALUES_TEST => {
            let pinned = PINS.with(|p| p.borrow().values().map(|e| e.1.len()).sum::<usize>());
            let later = LATER.with(|l| l.borrow().len());
            c.out.put_u32(pinned as u32);
            c.out.put_u32(later as u32);
            Some(Ok(()))
        }
        _ => None,
    }
}

/// Moves any handles pinned by `view` into the deferred sweep list when `view` is released.
#[allow(dead_code)]
pub fn view_released(view: u64) {
    if let Some((_, hs)) = PINS.with(|p| p.borrow_mut().remove(&view)) {
        LATER.with(|l| l.borrow_mut().extend(hs));
    }
    let _ = rt::with(|_| ());
}

/// Mutable byte-buffer cell (`C_CELL_*`) backing host-driven reactive values (such as setting fields).
pub struct ValueCell(pub Vec<u8>);

/// Looks up a [`ValueCell`] handle for wire calls (`C_CELL_GET`, `C_CELL_SET`).
pub fn cell(h: u64) -> GkResult<Rc<RefCell<ValueCell>>> {
    crate::objects::obj_rc::<ValueCell>(h)
}

/// Tape-decode counterpart to [`cell`]; stops the app on a stale handle.
pub fn cell_shown(h: u64) -> Rc<RefCell<ValueCell>> {
    match cell(h) {
        Ok(c) => c,
        Err(rt::GkErr(_, msg)) => crate::genapi::released_shown(&msg, "value cell"),
    }
}

/// Clones `c`'s bytes with 64 bytes of zero padding so callers can read default values safely
/// even if the cell is empty, then invokes `f`.
pub fn cell_read<T>(c: &Rc<RefCell<ValueCell>>, f: impl FnOnce(&mut R) -> T) -> T {
    let mut b = match c.try_borrow() {
        Ok(v) => v.0.clone(),
        Err(_) => Vec::new(),
    };
    b.resize(b.len() + 64, 0);
    let mut r = R::new(&b);
    f(&mut r)
}

use gpui_kit::component::setting::{AnySettingField, SettingFieldType};

/// Thin `Rc<dyn AnySettingField>` wrapper so generated setting-field builders can type-erase fields.
pub struct AnyField(Rc<dyn AnySettingField>);

impl AnyField {
    pub fn new<F: AnySettingField + 'static>(f: F) -> Self {
        AnyField(Rc::new(f))
    }
}

impl AnySettingField for AnyField {
    fn as_any(&self) -> &dyn std::any::Any {
        self.0.as_any()
    }
    fn type_name(&self) -> &'static str {
        self.0.type_name()
    }
    fn type_id(&self) -> std::any::TypeId {
        self.0.type_id()
    }
    fn field_type(&self) -> &SettingFieldType {
        self.0.field_type()
    }
    fn style(&self) -> &gpui_kit::StyleRefinement {
        self.0.style()
    }
    fn is_resettable(&self, cx: &gpui_kit::App) -> bool {
        self.0.is_resettable(cx)
    }
    fn reset(&self, window: &mut gpui_kit::Window, cx: &mut gpui_kit::App) {
        self.0.reset(window, cx)
    }
}

/// Encodes an `Hsla` color in the tagged color-field format (`u16` tag 0 followed by h, s, l, a).
pub fn put_color_field(c: &gpui_kit::Hsla, out: &mut Vec<u8>) {
    out.put_u16(0);
    out.put_f32(c.h);
    out.put_f32(c.s);
    out.put_f32(c.l);
    out.put_f32(c.a);
}

/// Dummy entity used by `C_HANDLES_TEST` to verify entity handle deduplication.
pub struct HandlesTestEntity;

/// Standard drag payload type shared by all host-initiated drag-and-drop operations.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct Drag {
    pub id: u64,
    pub text: String,
}

impl Wire for Drag {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(self.id);
        out.put_str(&self.text);
    }
}

impl crate::genapi::Unwire for Drag {
    fn get(r: &mut R) -> Self {
        let id = r.u64();
        Drag { id, text: r.str().to_string() }
    }
}

/// Resolves an optional drag preview view handle (`0` means no custom preview view).
pub fn drag_view_of(h: u64, dx: &mut crate::genapi::Dx) -> Option<gpui_kit::AnyView> {
    if h == 0 { None } else { dx.child_view(h) }
}

thread_local! {
    /// Lazily created empty `GkView` reused whenever a drag operation provides no custom preview view.
    static EMPTY_DRAG: RefCell<Option<gpui_kit::Entity<crate::rt::GkView>>> = const { RefCell::new(None) };
}

/// Extracts the `Entity<GkView>` from `v`, or falls back to the cached empty drag preview view.
pub fn drag_view(v: &Option<gpui_kit::AnyView>, cx: &mut gpui_kit::App) -> gpui_kit::Entity<crate::rt::GkView> {
    if let Some(e) = v.as_ref().and_then(|v| v.clone().downcast::<crate::rt::GkView>().ok()) {
        return e;
    }
    if let Some(e) = EMPTY_DRAG.with(|x| x.borrow().clone()) {
        return e;
    }
    let h = rt::new_view(cx);
    let e = match rt::with(|s| s.view(h)) {
        Ok(e) => e,
        Err(rt::GkErr(_, msg)) => panic!("the library's empty drag view: {msg}"),
    };
    EMPTY_DRAG.with(|x| *x.borrow_mut() = Some(e.clone()));
    e
}

/// Returns the root entity of `window` if its concrete type is `T`.
pub fn window_root<T: gpui_kit::Render + 'static>(window: &gpui_kit::Window, _cx: &gpui_kit::App) -> Option<gpui_kit::Entity<T>> {
    window.root::<T>().flatten()
}

impl Wire for gpui_kit::FocusHandle {
    fn put(&self, out: &mut Vec<u8>) {
        let f = self.clone();
        out.put_u64(rt::with(|s| s.insert(HK_FOCUS, rt::Item::Focus(f, None))));
    }
}

/// Wire layout for `gpui_kit::Background`:
/// `[kind: u8 (0=Solid, 1=LinearGradient, 2=PatternSlash, 3=Checkerboard)][color_space: u8 (0=sRGB, 1=Oklab)][solid: Hsla][angle_or_height: f32][(stop_color: Hsla, pct: f32) * 2]`.
///
/// Upstream `Background` does not expose getters for its fields, so we combine `serde_json`
/// serialization (tag, color space, angle/height, stop percentages) with parsing `Hsla { ... }`
/// fields out of its `Debug` representation.
impl Wire for gpui_kit::Background {
    fn put(&self, out: &mut Vec<u8>) {
        // `gpui_kit::Background` has private fields; inspect via Serialize + Debug to recover
        // tag, color space, angle/height, stop percentages, and HSLA stops.
        let j = serde_json::to_value(self).unwrap_or_default();
        let kind = match j.get("tag").and_then(|t| t.as_str()) {
            Some("LinearGradient") => 1u8,
            Some("PatternSlash") => 2,
            Some("Checkerboard") => 3,
            _ => 0,
        };
        let space = (j.get("color_space").and_then(|t| t.as_str()) == Some("Oklab")) as u8;
        let num = |v: Option<&serde_json::Value>| v.and_then(|x| x.as_f64()).unwrap_or(0.0) as f32;
        let colors = hsla_in_debug(&format!("{self:?}"));
        let at = |i: usize| colors.get(i).copied().unwrap_or_default();
        out.put_u8(kind);
        out.put_u8(space);
        let (solid, stops) = if kind == 1 {
            (gpui_kit::Hsla::default(), [at(0), at(1)])
        } else {
            (at(0), [gpui_kit::Hsla::default(), gpui_kit::Hsla::default()])
        };
        solid.put(out);
        out.put_f32(num(j.get("gradient_angle_or_pattern_height")));
        for (i, c) in stops.iter().enumerate() {
            c.put(out);
            out.put_f32(num(j.get("colors").and_then(|s| s.get(i)).and_then(|s| s.get("percentage"))));
        }
    }
}

/// Parses every `Hsla { h: .., s: .., l: .., a: .. }` record from a `Debug` string in order.
fn hsla_in_debug(text: &str) -> Vec<gpui_kit::Hsla> {
    let mut out = Vec::new();
    let mut rest = text;
    while let Some(i) = rest.find("Hsla {") {
        rest = &rest[i + 6..];
        let end = rest.find('}').unwrap_or(rest.len());
        let body = &rest[..end];
        let field = |name: &str| -> f32 {
            body.split(',')
                .filter_map(|p| p.trim().strip_prefix(name))
                .filter_map(|p| p.trim_start().strip_prefix(':'))
                .find_map(|v| v.trim().parse::<f32>().ok())
                .unwrap_or(0.0)
        };
        out.push(gpui_kit::hsla(field("h"), field("s"), field("l"), field("a")));
        rest = &rest[end..];
    }
    out
}

/// Mirror layout for `gpui_kit::base::IntervalMatcher` and `RangeMatcher`, whose two
/// `Option<jiff::civil::Date>` fields are private in upstream.
///
/// The six `const` assertions below guarantee size, alignment, and `!needs_drop` parity at
/// compile time; [`parts_readable`] additionally probes live constructed matchers once at
/// runtime to catch any field reordering across compiler or upstream upgrades before reading.
#[derive(Clone, Copy)]
struct Part {
    a: Option<jiff::civil::Date>,
    b: Option<jiff::civil::Date>,
}
const _: () = assert!(std::mem::size_of::<gpui_kit::base::IntervalMatcher>() == std::mem::size_of::<Part>());
const _: () = assert!(std::mem::size_of::<gpui_kit::base::RangeMatcher>() == std::mem::size_of::<Part>());
const _: () = assert!(std::mem::align_of::<gpui_kit::base::IntervalMatcher>() == std::mem::align_of::<Part>());
const _: () = assert!(std::mem::align_of::<gpui_kit::base::RangeMatcher>() == std::mem::align_of::<Part>());
const _: () = assert!(!std::mem::needs_drop::<gpui_kit::base::IntervalMatcher>());
const _: () = assert!(!std::mem::needs_drop::<gpui_kit::base::RangeMatcher>());

thread_local! {
    /// Side-table mapping the heap address of a [`value_set`] closure to its original serialized list
    /// so predicates can be read back across the wire.
    static VALUE_SETS: RefCell<HashMap<usize, Vec<u8>>> = RefCell::new(HashMap::new());
}

/// Reads the two private `Option<jiff::civil::Date>` fields out of `v`.
///
/// # Safety
/// `T` must be a `Copy`/non-`Drop` struct with the exact size, alignment, and bit-level field
/// layout of [`Part`] (verified at compile time by the `const` assertions above and at runtime by
/// [`parts_readable`]).
unsafe fn part_of<T>(v: &T) -> Part {
    // SAFETY: Caller guarantees `T` has identical layout to `Part` and contains valid `Part` bits.
    unsafe { std::ptr::read(v as *const T as *const Part) }
}

/// Runtime self-test confirming that both `IntervalMatcher` and `RangeMatcher` match [`Part`]'s
/// exact field order before any production value is serialized.
fn parts_readable() -> bool {
    static OK: std::sync::OnceLock<bool> = std::sync::OnceLock::new();
    *OK.get_or_init(|| {
        let (x, y) = (jiff::civil::Date::constant(2001, 2, 3), jiff::civil::Date::constant(2004, 5, 6));
        let check = |m: gpui_kit::base::Matcher, a, b| {
            let p = match &m {
                // SAFETY: `IntervalMatcher` and `RangeMatcher` have size, alignment, and `!needs_drop`
                // asserted equal to `Part` at compile time, and both consist of two `Option<Date>` fields.
                gpui_kit::base::Matcher::Interval(v) => unsafe { part_of(v) },
                gpui_kit::base::Matcher::Range(v) => unsafe { part_of(v) },
                _ => return false,
            };
            p.a == a && p.b == b
        };
        check(gpui_kit::base::Matcher::interval(Some(x), None), Some(x), None)
            && check(gpui_kit::base::Matcher::interval(None, Some(y)), None, Some(y))
            && check(gpui_kit::base::Matcher::range(Some(x), Some(y)), Some(x), Some(y))
            && check(gpui_kit::base::Matcher::range(Some(y), None), Some(y), None)
    })
}

fn read_part<T>(r: &mut R, make: impl FnOnce(Option<jiff::civil::Date>, Option<jiff::civil::Date>) -> T) -> T {
    let a = <Option<jiff::civil::Date> as crate::genapi::Unwire>::get(r);
    let b = <Option<jiff::civil::Date> as crate::genapi::Unwire>::get(r);
    make(a, b)
}

/// Serializes `v` as two `Option<Date>` values if [`parts_readable`] passes; otherwise writes `(None, None)`.
///
/// # Safety
/// Same layout precondition as [`part_of`]: `T` must be `IntervalMatcher` or `RangeMatcher`.
unsafe fn put_part<T>(v: &T, out: &mut Vec<u8>) {
    if parts_readable() {
        // SAFETY: Caller passes `IntervalMatcher` or `RangeMatcher`, and `parts_readable()` verified
        // their layout and field order match `Part`.
        let p = unsafe { part_of(v) };
        p.a.put(out);
        p.b.put(out);
    } else {
        out.put_u8(0);
        out.put_u8(0);
    }
}

impl crate::genapi::Unwire for gpui_kit::base::IntervalMatcher {
    fn get(r: &mut R) -> Self {
        read_part(r, |a, b| match gpui_kit::base::Matcher::interval(a, b) {
            gpui_kit::base::Matcher::Interval(m) => m,
            _ => unreachable!("Matcher::interval makes an Interval"),
        })
    }
}

impl Wire for gpui_kit::base::IntervalMatcher {
    fn put(&self, out: &mut Vec<u8>) {
        // SAFETY: `IntervalMatcher` satisfies `put_part`'s layout assertions and runtime probe.
        unsafe { put_part(self, out) }
    }
}

impl crate::genapi::Unwire for gpui_kit::base::RangeMatcher {
    fn get(r: &mut R) -> Self {
        read_part(r, |a, b| match gpui_kit::base::Matcher::range(a, b) {
            gpui_kit::base::Matcher::Range(m) => m,
            _ => unreachable!("Matcher::range makes a Range"),
        })
    }
}

impl Wire for gpui_kit::base::RangeMatcher {
    fn put(&self, out: &mut Vec<u8>) {
        // SAFETY: `RangeMatcher` satisfies `put_part`'s layout assertions and runtime probe.
        unsafe { put_part(self, out) }
    }
}

/// Wraps a finite list `set` into a membership predicate closure and records its wire bytes
/// in `VALUE_SETS` so reading the predicate back recovers the original items.
pub fn value_set<K: Wire + PartialEq + Send + Sync + 'static>(set: Vec<K>) -> Box<dyn Fn(&K) -> bool + Send + Sync> {
    let mut bytes = Vec::new();
    set.put(&mut bytes);
    let f: Box<dyn Fn(&K) -> bool + Send + Sync> = Box::new(move |k: &K| set.iter().any(|x| x == k));
    let at = &*f as *const (dyn Fn(&K) -> bool + Send + Sync) as *const () as usize;
    VALUE_SETS.with(|m| m.borrow_mut().insert(at, bytes));
    f
}

impl<K: 'static> Wire for Box<dyn Fn(&K) -> bool + Send + Sync> {
    fn put(&self, out: &mut Vec<u8>) {
        let at = &**self as *const (dyn Fn(&K) -> bool + Send + Sync) as *const () as usize;
        match VALUE_SETS.with(|m| m.borrow().get(&at).cloned()) {
            Some(b) => out.extend_from_slice(&b),
            None => out.put_u32(0),
        }
    }
}

/// Declarative validation rules for a questionnaire item decoded from the wire.
///
/// Numeric bounds (`min_choices`, `max_choices`, `min_length`, `max_length`) treat `0` as disabled;
/// `message` overrides the default error text and substitutes `{item}` with the item's key.
pub struct QuestionnaireRules {
    min_choices: usize,
    max_choices: usize,
    min_length: usize,
    max_length: usize,
    one_of: Vec<String>,
    not_one_of: Vec<String>,
    same_as: String,
    differs_from: String,
    message: String,
}

/// Wire layout for [`QuestionnaireRules`]:
/// `[min_choices: u32][max_choices: u32][min_length: u32][max_length: u32][one_of: Vec<str>][not_one_of: Vec<str>][same_as: str][differs_from: str][message: str]`.
impl crate::genapi::Unwire for QuestionnaireRules {
    fn get(r: &mut R) -> Self {
        let mut n = || r.u32() as usize;
        let (min_choices, max_choices, min_length, max_length) = (n(), n(), n(), n());
        let mut strs = |r: &mut R| -> Vec<String> {
            let k = r.u32() as usize;
            (0..k).map(|_| r.str().to_owned()).collect()
        };
        let one_of = strs(r);
        let not_one_of = strs(r);
        let same_as = r.str().to_owned();
        let differs_from = r.str().to_owned();
        let message = r.str().to_owned();
        QuestionnaireRules { min_choices, max_choices, min_length, max_length, one_of, not_one_of, same_as, differs_from, message }
    }
}

impl QuestionnaireRules {
    fn check(&self, ctx: &gpui_kit::base::questionnaire::QuestionnaireValidationContext) -> Result<(), gpui_kit::SharedString> {
        let item = ctx.item();
        let fail = |why: String| -> Result<(), gpui_kit::SharedString> {
            let text = if self.message.is_empty() { why } else { self.message.clone() };
            Err(gpui_kit::SharedString::from(text.replace("{item}", item)))
        };
        let a = ctx.answer();
        let n = a.choices().len();
        if self.min_choices > 0 && n < self.min_choices {
            return fail(format!("choose at least {}", self.min_choices));
        }
        if self.max_choices > 0 && n > self.max_choices {
            return fail(format!("choose at most {}", self.max_choices));
        }
        if let Some(free) = a.freeform() {
            let len = free.chars().count();
            if self.min_length > 0 && len < self.min_length {
                return fail(format!("at least {} characters", self.min_length));
            }
            if self.max_length > 0 && len > self.max_length {
                return fail(format!("at most {} characters", self.max_length));
            }
            if !self.one_of.is_empty() && !self.one_of.iter().any(|x| x == free.as_ref()) {
                return fail(format!("one of {}", self.one_of.join(", ")));
            }
            if self.not_one_of.iter().any(|x| x == free.as_ref()) {
                return fail(format!("not {free}"));
            }
        }
        if !self.same_as.is_empty() && ctx.answers().get(&self.same_as) != Some(a) {
            return fail(format!("the same as {}", self.same_as));
        }
        if !self.differs_from.is_empty() && ctx.answers().get(&self.differs_from) == Some(a) {
            return fail(format!("not the same as {}", self.differs_from));
        }
        Ok(())
    }
}

/// Decodes [`QuestionnaireRules`] into a validation closure accepted by upstream questionnaire items.
pub fn questionnaire_rules(
    r: &mut R,
) -> impl Fn(&gpui_kit::base::questionnaire::QuestionnaireValidationContext) -> Result<(), gpui_kit::SharedString> + 'static {
    let rules = <QuestionnaireRules as crate::genapi::Unwire>::get(r);
    move |ctx| rules.check(ctx)
}

impl Wire for Rc<dyn Fn(&gpui_kit::base::questionnaire::QuestionnaireValidationContext) -> Result<(), gpui_kit::SharedString>> {
    fn put(&self, _out: &mut Vec<u8>) {}
}
