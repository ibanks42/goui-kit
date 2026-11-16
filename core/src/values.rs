//! The values family's runtime (docs/PROTOCOLS.md sections 13.8 to 13.12):
//!
//! - results that travel to the app: `Result<T, E>` (`Fallible`), a `HashMap` (its pairs);
//! - a kit action value made like any value (`gk.move_down()`) as an action id (`VALUE_ACTION`):
//!   the kit's own action types are taken wherever an action is (menus, key bindings, `Kbd`);
//! - temporary handles placed in a view (`OBJ_PIN`): a library object a read hands out lives until
//!   the next frame begins (section 13.2); one the app put into a view's elements is kept for as
//!   long as a build of that view names it, so a later redraw of the same tape finds it.
//!
//! Generated code reaches the first through `genapi::Wire` impls and `crate::values::Fallible`
//! (written by gen/gk_values.py); the call ops are the runtime's own.

use crate::genapi::Wire;
use crate::ops::*;
use crate::rt::{self, GkResult, err};
use crate::wire::{Put, R};
use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

/// `Result<T, E>` as it travels out: u8 ok, then opt<T> (the value when ok), then str (the
/// error's text when not ok, else empty: its Debug form, which every error type of the kit has;
/// some, `DiscreteError`, have no Display). Odin `gk.Fallible(T)`, read by reflection.
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

/// A map as it travels out: u32 n, then (key, value) pairs in the order of their bytes, so the
/// same map always reads the same (a HashMap's own order changes between runs).
impl<K: Wire, V: Wire, S> Wire for HashMap<K, V, S> {
    fn put(&self, out: &mut Vec<u8>) {
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

// chrono's calendar values (PROTOCOLS.md 13.13): a date is i32 year, u32 month, u32 day; a time
// u32 hour, minute, second, nanosecond; a date and time both; a weekday a u8 from Monday = 0.
// Odin's writers (rt_values.odin put_naive_date and the rest) stop on a date or time that does not
// exist, so what arrives here is valid; a raw C client that sends one anyway gets the first day of
// the common era or midnight instead (never a panic).
/// A duration read back: i64 nanoseconds (Odin's `time.Duration`), at most about 292 years.
impl Wire for std::time::Duration {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_i64(i64::try_from(self.as_nanos()).unwrap_or(i64::MAX));
    }
}

/// Kit state a read or an event hands out (`HslaSliders::hue`, `ColorPickerState::hex_input`):
/// a state handle, one per entity, kept like one the app made (gk.release frees it; the next read
/// makes a new one). Its mutators run with the window of the call that uses it.
impl<T: 'static> Wire for gpui_kit::Entity<T> {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u64(entity_handle(self));
    }
}

/// The state handle of an entity the library hands out. One table for the whole library
/// (the values family's reads and the dock's reads used to keep one each, so the same entity read
/// back by both got two handles): the dock's, `crate::dock::entity_handle`, which also holds the
/// panels and dock areas the app made, so a read of one of those gives the app's own handle.
pub fn entity_handle<T: 'static>(e: &gpui_kit::Entity<T>) -> u64 {
    crate::dock::entity_handle(e)
}

impl Wire for chrono::NaiveDate {
    fn put(&self, out: &mut Vec<u8>) {
        use chrono::Datelike;
        out.put_i32(self.year());
        out.put_u32(self.month());
        out.put_u32(self.day());
    }
}

impl crate::genapi::Unwire for chrono::NaiveDate {
    fn get(r: &mut R) -> Self {
        let (y, m, d) = (r.i32(), r.u32(), r.u32());
        chrono::NaiveDate::from_ymd_opt(y, m, d).unwrap_or_default()
    }
}

impl Wire for chrono::NaiveTime {
    fn put(&self, out: &mut Vec<u8>) {
        use chrono::Timelike;
        out.put_u32(self.hour());
        out.put_u32(self.minute());
        out.put_u32(self.second());
        out.put_u32(self.nanosecond());
    }
}

impl crate::genapi::Unwire for chrono::NaiveTime {
    fn get(r: &mut R) -> Self {
        let (h, m, s, n) = (r.u32(), r.u32(), r.u32(), r.u32());
        chrono::NaiveTime::from_hms_nano_opt(h, m, s, n).unwrap_or_default()
    }
}

impl Wire for chrono::NaiveDateTime {
    fn put(&self, out: &mut Vec<u8>) {
        self.date().put(out);
        self.time().put(out);
    }
}

impl crate::genapi::Unwire for chrono::NaiveDateTime {
    fn get(r: &mut R) -> Self {
        use crate::genapi::Unwire;
        let d = chrono::NaiveDate::get(r);
        d.and_time(chrono::NaiveTime::get(r))
    }
}

impl Wire for chrono::Weekday {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_u8(self.num_days_from_monday() as u8);
    }
}

impl crate::genapi::Unwire for chrono::Weekday {
    fn get(r: &mut R) -> Self {
        chrono::Weekday::try_from(r.u8() % 7).unwrap_or(chrono::Weekday::Mon)
    }
}

thread_local! {
    /// Temporary handles a view's build placed in its elements: view -> (build number, handles).
    static PINS: RefCell<HashMap<u64, (u32, Vec<u64>)>> = RefCell::new(HashMap::new());
    /// Handles a view's earlier build pinned, released at the next sweep unless pinned again.
    static LATER: RefCell<Vec<u64>> = const { RefCell::new(Vec::new()) };
}

/// Pins `h` to the build `build` of `view`: a temporary handle stops being temporary and lives
/// while a build of the view names it. The handles of the view's earlier build go to the next
/// sweep (a redraw of the new tape names only its own). A kept handle or a global is left alone.
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
    }
    let again = LATER.with(|l| {
        let mut l = l.borrow_mut();
        let n = l.len();
        l.retain(|x| *x != h);
        l.len() != n
    });
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

/// The handles earlier builds pinned and no build names now (called by `objects::sweep`).
pub fn sweep_pins() -> Vec<u64> {
    LATER.with(|l| std::mem::take(&mut *l.borrow_mut()))
}

/// The runtime's own call ops of the values family.
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
                None => err(ERR_BAD_OP, "gk.action_of: the value is not one of the kit's action types"),
            }
        })()),
        C_OBJ_PIN => {
            let (view, build, h) = (r.u64(), r.u32(), r.u64());
            pin(view, build, h);
            Some(Ok(()))
        }
        // u32 length, bytes -> u64 cell (PROTOCOLS.md 13.19)
        C_CELL_NEW => {
            let n = r.u32() as usize;
            let bytes = r.bytes(n).to_vec();
            c.out.put_u64(crate::objects::obj_new(ValueCell(bytes)));
            Some(Ok(()))
        }
        // u64 cell, u32 length, bytes (buffered): every window redraws, so a page that reads the
        // cell while it draws shows the new value in the next frame
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
        // u64 cell -> u32 length, bytes
        C_CELL_GET => Some((|| {
            let rc = cell(r.u64())?;
            let b = rc.try_borrow().map_err(|_| rt::GkErr(ERR_BUSY, "the value cell is being written".into()))?;
            c.out.put_u32(b.0.len() as u32);
            c.out.extend_from_slice(&b.0);
            Ok(())
        })()),
        // the tests': one entity read back by both paths of the library gets one handle (13.16)
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

/// A released view's pins go to the next sweep (rt calls nothing here yet: a view lives until
/// the app ends, DESIGN.md section 8).
#[allow(dead_code)]
pub fn view_released(view: u64) {
    if let Some((_, hs)) = PINS.with(|p| p.borrow_mut().remove(&view)) {
        LATER.with(|l| l.borrow_mut().extend(hs));
    }
    let _ = rt::with(|_| ());
}

// ------------------------------------------------------------------ value cells (PROTOCOLS.md 13.19)

/// A value the library holds for a closure upstream calls while it draws (`Fn(&App) -> T`: a
/// settings field's value, its dirty test): the value's bytes in its wire type, written by the app
/// between frames (call op CELL_SET) and decoded by the generated closure each time upstream asks.
/// A library object (kind OBJ, kept until gk.release); a tape holds the cell, not a copy.
pub struct ValueCell(pub Vec<u8>);

/// The cell behind a handle (a call op's argument).
pub fn cell(h: u64) -> GkResult<Rc<RefCell<ValueCell>>> {
    crate::objects::obj_rc::<ValueCell>(h)
}

/// The cell behind a handle named in a tape: a released one stops the program where it is drawn.
pub fn cell_shown(h: u64) -> Rc<RefCell<ValueCell>> {
    match cell(h) {
        Ok(c) => c,
        Err(rt::GkErr(_, msg)) => crate::genapi::released_shown(&msg, "value cell"),
    }
}

/// Decodes the cell's current value with the generated reader `f`. A value that is too short for
/// its type (a cell of another type given by a raw client) reads as zeros, never a panic.
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

/// One of the kit's settings fields (`SettingField<bool>`, `<SharedString>`, `<f64>`, each a value
/// node of its own) as one type: `SettingItem::new` takes it. Every method is the field's own, so
/// upstream's downcasts (`as_any`, `type_id`) see the field itself.
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

/// A colour field of a record the app also writes, read back (PROTOCOLS.md 13.20): the layout of
/// Odin's gk.Color (u16 theme 0, then h, s, l, a), which is that record's field type there.
pub fn put_color_field(c: &gpui_kit::Hsla, out: &mut Vec<u8>) {
    out.put_u16(0);
    out.put_f32(c.h);
    out.put_f32(c.s);
    out.put_f32(c.l);
    out.put_f32(c.a);
}

/// The entity HANDLES_TEST makes (any kit entity would do; this one has no window to need).
pub struct HandlesTestEntity;

// ------------------------------------------------------------------ drag and drop (PROTOCOLS.md 13.20)

/// The value an app drags (`div->on_drag(gk.Drag_Value{id = 7, text = "row 7"}, preview)`): what
/// gpui's drag methods are generic over, fixed to one library type. A drop's handler gets it back,
/// a drag-over style applies while it is over the element.
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

/// The app's view a drag shows, taken where the element is decoded (0: none).
pub fn drag_view_of(h: u64, dx: &mut crate::genapi::Dx) -> Option<gpui_kit::AnyView> {
    if h == 0 { None } else { dx.child_view(h) }
}

thread_local! {
    static EMPTY_DRAG: RefCell<Option<gpui_kit::Entity<crate::rt::GkView>>> = const { RefCell::new(None) };
}

/// The preview gpui shows under the pointer while the value is dragged: the app's view, or an
/// empty view of the library when the app gave none.
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

// ------------------------------------------------------------------ the window's own state (PROTOCOLS.md 13.21)

/// The window's own `T` (the kit's Root, which the library makes around the app's view in every
/// window), as a state handle: `Root::read(window, cx)` for the binding. None when the window's
/// root is something else.
pub fn window_root<T: gpui_kit::Render + 'static>(window: &gpui_kit::Window, _cx: &gpui_kit::App) -> Option<gpui_kit::Entity<T>> {
    window.root::<T>().flatten()
}

/// A focus handle a read hands out (`QuestionnaireState::focus_handle`, PROTOCOLS.md 13.21): a handle
/// of kind FOCUS, kept until gk.release, as one the app made with gk.focus_handle; the app focuses it
/// (gk.focus) or gives it to an element (`track_focus`). Each read makes a new handle.
impl Wire for gpui_kit::FocusHandle {
    fn put(&self, out: &mut Vec<u8>) {
        let f = self.clone();
        out.put_u64(rt::with(|s| s.insert(HK_FOCUS, rt::Item::Focus(f, None))));
    }
}

// --- the components family, docs/PROTOCOLS.md 11.21 to 11.24 ----------------------------

/// gpui's Background read back (PROTOCOLS.md 11.23): u8 kind (0 solid, 1 linear gradient, 2 slash
/// pattern, 3 checkerboard), u8 colour space (0 sRGB, 1 Oklab), the solid colour (Hsla), f32 the
/// gradient's angle or the pattern's height, then the two stops (Hsla, f32 percentage). Odin
/// `gk.Background_Value` (rt_values.odin). Its fields are private: the kind, colour space, angle and
/// percentages come from its serde form, the colours (at full precision) from its Debug form.
impl Wire for gpui_kit::Background {
    fn put(&self, out: &mut Vec<u8>) {
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

/// The `Hsla { h: .., s: .., l: .., a: .. }` values of a Debug text, in order.
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

/// A calendar matcher's parts (PROTOCOLS.md 11.21): their fields are private and have no reads,
/// so they are made by the enum's own constructors (`Matcher::interval`, `Matcher::range`), and the
/// library keeps what each one was made of, keyed by its bytes, to read it back: the binding is
/// the only way an app makes one. (A matcher's dates cannot be found through `Matcher::matched`:
/// the days it leaves out may be a single day anywhere.)
const _: () = assert!(std::mem::size_of::<gpui_kit::base::IntervalMatcher>() == 8);
const _: () = assert!(std::mem::size_of::<gpui_kit::base::RangeMatcher>() == 8);
const _: () = assert!(!std::mem::needs_drop::<gpui_kit::base::IntervalMatcher>());
const _: () = assert!(!std::mem::needs_drop::<gpui_kit::base::RangeMatcher>());

thread_local! {
    /// What each matcher part was made of: its 8 bytes -> (the two dates' wire bytes).
    static MATCHER_PARTS: RefCell<HashMap<[u8; 8], Vec<u8>>> = RefCell::new(HashMap::new());
    /// What each set of values was made of (a `Matcher::Custom`'s closure): its address -> the list's bytes.
    static VALUE_SETS: RefCell<HashMap<usize, Vec<u8>>> = RefCell::new(HashMap::new());
}

/// The bytes of a matcher part: two optional dates, no padding (8 bytes, asserted above).
fn part_bytes<T>(v: &T) -> [u8; 8] {
    let mut b = [0u8; 8];
    // SAFETY: T is IntervalMatcher or RangeMatcher, two Option<NaiveDate> (niche-packed i32s) in
    // 8 bytes with no padding (asserted above), so every byte is initialised.
    unsafe { std::ptr::copy_nonoverlapping(v as *const T as *const u8, b.as_mut_ptr(), 8) };
    b
}

fn read_part<T>(r: &mut R, make: impl FnOnce(Option<chrono::NaiveDate>, Option<chrono::NaiveDate>) -> T) -> T {
    let a = <Option<chrono::NaiveDate> as crate::genapi::Unwire>::get(r);
    let b = <Option<chrono::NaiveDate> as crate::genapi::Unwire>::get(r);
    let v = make(a, b);
    let mut wire = Vec::new();
    a.put(&mut wire);
    b.put(&mut wire);
    MATCHER_PARTS.with(|m| m.borrow_mut().insert(part_bytes(&v), wire));
    v
}

fn put_part<T>(v: &T, out: &mut Vec<u8>) {
    match MATCHER_PARTS.with(|m| m.borrow().get(&part_bytes(v)).cloned()) {
        Some(w) => out.extend_from_slice(&w),
        None => {
            out.put_u8(0); // not made by the binding: no dates known
            out.put_u8(0);
        }
    }
}

/// `Interval { before, after }`: dates before `before` or after `after` are matched.
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
        put_part(self, out)
    }
}

/// `Range { from, to }`: dates from `from` to `to` (both included) are matched.
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
        put_part(self, out)
    }
}

/// A set of values as a predicate (PROTOCOLS.md 11.21: `Matcher::Custom`, the dates it holds): the
/// closure answers whether a value is in the set, and is kept with the set's bytes so that the
/// value it is in reads back as the set.
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
            None => out.put_u32(0), // a predicate the binding did not make: no values known
        }
    }
}

/// A questionnaire item's validator as rules the app gives in advance (PROTOCOLS.md 11.26): upstream
/// calls the validator synchronously while the questionnaire updates (an answer set, Next, Submit),
/// where app code may not run, so the library checks the rules. `gk.Questionnaire_Rules`
/// (rt_values.odin), by reflection: u32 min_choices, u32 max_choices, u32 min_length, u32 max_length
/// (0: no limit; a length counts the free answer's characters, when there is one), u32 n, str one_of
/// each, u32 n, str not_one_of each (the free answer must, or must not, be one of them; empty: any),
/// str same_as, str differs_from (another item whose answer this one must equal, or differ from; ""
/// none), str message (the error, "{item}" the item's name; "" gives one per rule).
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

/// The validator closure of `QuestionnaireItemDefinition::with_validator` over the rules.
pub fn questionnaire_rules(
    r: &mut R,
) -> impl Fn(&gpui_kit::base::questionnaire::QuestionnaireValidationContext) -> Result<(), gpui_kit::SharedString> + 'static {
    let rules = <QuestionnaireRules as crate::genapi::Unwire>::get(r);
    move |ctx| rules.check(ctx)
}

/// `QuestionnaireItemDefinition::validator` read back (`Option<&QuestionnaireValidator>`): whether
/// the item has one; a closure has nothing else to read (Odin `Maybe(gk.Questionnaire_Validator)`).
impl Wire for Rc<dyn Fn(&gpui_kit::base::questionnaire::QuestionnaireValidationContext) -> Result<(), gpui_kit::SharedString>> {
    fn put(&self, _out: &mut Vec<u8>) {}
}
