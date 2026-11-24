//! Dock-area panels, layout persistence, pane-tree inspection, skin settings, and custom
//! `DockAreaRenderer` / `TabGroupRenderer` hooks.
//!
//! # Protocols and Invariants
//!
//! - **Panels ([`GkPanel`], [`PanelData`])**: wraps a host `GkView` to implement both
//!   `base::dock::Panel` and `component::dock::Panel`. Panel names are interned once into
//!   `&'static str` via [`intern`] as required by `BasePanel::panel_name`. Auxiliary handles
//!   (`suffix`, `toolbar`, `menu_view`) are mounted/prepared by [`prepare`]; `toolbar_buttons`
//!   walks `E_CHILD` ops on the toolbar view's tape to build `Vec<Button>`. Lifecycle callbacks
//!   (`set_active`, `set_zoomed`, `on_added_to`, `on_removed`) emit queued `SHAPE_PANEL_EVENT`
//!   notifications.
//! - **Canonical handle caches (`NODE_IDS`, `HANDED`, `VIEWS`)**:
//!   - `NODE_IDS` records every `NodeId` handed out to the host so returning an unknown raw `u64`
//!     fails cleanly with `ERR_BAD_OP` instead of fabricating invalid `NodeId`s.
//!   - `HANDED` maps `EntityId -> HK_STATE` so repeated reads of the same `DockArea`, `TabGroup`,
//!     or `GkPanel` return identical handles via [`entity_handle`].
//!   - `VIEWS` maps `GkView` `EntityId -> HK_VIEW` for round-tripping `AnyView` back to Go.
//! - **Layout save/restore (`C_PANEL_REGISTER`, `C_PANEL_FILL`, `C_PANE_TREE_*`)**: because
//!   `PanelRegistry::build_panel` is synchronous while Go constructs panels on a callback turn,
//!   [`requested`] returns an empty placeholder `GkPanel` immediately and queues
//!   `SHAPE_PANEL_BUILD`; the host attaches its view and metadata via `C_PANEL_FILL`. During
//!   `PaneTree::from_state`, [`GkBuilder`] pins every created `Arc<dyn PanelView>` in thread-local
//!   `BUILT` so panel entities stay alive until attached to the dock.
//! - **Custom dock chrome ([`GkRenderer`], `BOOK`, `SLOT`)**: `hooks` is a 12-bit mask (`H_*`)
//!   selecting which renderer hooks cross the wire. Each hooked site computes a snapshot hash
//!   (`dock_snap` / `group_snap`) and only queues `SHAPE_DOCK_RENDER` when the hash changes,
//!   releasing the previous request's context handles (`DockContext`/`TabGroupContext`). When the
//!   host answers via `C_DOCK_RENDER_ANSWER`, [`GkRenderer::element`] stashes the upstream child
//!   element in thread-local `SLOT`, decodes the host view tape (where `N_DOCK_SLOT` takes `SLOT`),
//!   and clears `SLOT` on return so elements never leak across frames. Frame hooks (`H_FRAME`,
//!   `H_SPLIT_FRAME`, `H_CENTER_FRAME`, `H_TAB_FRAME`, `H_TAB_CONTENT_FRAME`) require a div root
//!   (`256..=258`) and abort via `rt::app_stop` otherwise.

use crate::genapi::{self, Call, GkResult, HandlerRef, Put, R};
use crate::ops::*;
use crate::rt::{self, GkView};
use gpui_kit::base::dock::{DockArea, Panel as BasePanel, PanelEvent, PanelView, TabGroup};
use gpui_kit::component::dock::{DockSkin, Panel as KitPanel, PanelView as KitPanelView};
use gpui_kit::{
    AnyElement, App, AppContext as _, Context, Entity, EventEmitter, FocusHandle, Focusable, IntoElement,
    Render, SharedString, WeakEntity, Window,
};
use std::collections::HashMap;
use std::sync::{Arc, Mutex, OnceLock};

/// Host-supplied metadata and auxiliary view handles for a [`GkPanel`].
#[derive(Clone)]
pub struct PanelData {
    name: &'static str,
    tab_name: Option<SharedString>,
    title: SharedString,
    flags: u8,
    /// Optional view handle rendered in `title_suffix`.
    suffix: u64,
    /// Optional view handle whose root children are decoded as `toolbar_buttons`.
    toolbar: u64,
    menu: Option<std::rc::Rc<crate::overlay::ViewBuild>>,
    menu_view: u64,
    /// JSON payload returned by `BasePanel::dump`.
    info: String,
}

/// Bit flags packed into [`PanelData::flags`].
const NOT_CLOSABLE: u8 = 1;
const NOT_ZOOMABLE: u8 = 2;
const HIDDEN: u8 = 4;
const NO_PADDING: u8 = 8;
const NO_TITLE_BAR: u8 = 16;

/// Interns a panel name into `&'static str` as required by `BasePanel::panel_name`.
fn intern(name: &str) -> &'static str {
    static NAMES: OnceLock<Mutex<HashMap<String, &'static str>>> = OnceLock::new();
    let mut names = NAMES.get_or_init(Default::default).lock().unwrap();
    if let Some(s) = names.get(name) {
        return s;
    }
    let s: &'static str = Box::leak(name.to_string().into_boxed_str());
    names.insert(name.to_string(), s);
    s
}

fn read_data(r: &mut R) -> PanelData {
    let name = intern(r.str());
    let tab_name = if r.u8() != 0 { Some(SharedString::new(r.str())) } else { None };
    let title = SharedString::new(r.str());
    let flags = r.u8();
    let (suffix, toolbar, menu_view) = (r.u64(), r.u64(), r.u64());
    let info = r.str().to_string();
    PanelData { name, tab_name, title, flags, suffix, toolbar, menu: None, menu_view, info }
}

fn prepare(d: &mut PanelData, c: &mut Call) -> GkResult<()> {
    for v in [d.suffix, d.toolbar] {
        if v != 0 {
            mount(v, c)?;
        }
    }
    if d.menu_view != 0 {
        d.menu = Some(std::rc::Rc::new(crate::overlay::ViewBuild::take(d.menu_view, c, &[], "PopupMenu")?));
    }
    Ok(())
}

/// Dock panel entity bridging a host [`GkView`] into `gpui-kit`'s `BasePanel` and `KitPanel` traits.
pub struct GkPanel {
    view: Option<Entity<GkView>>,
    focus: FocusHandle,
    data: PanelData,
    events: Option<HandlerRef>,
    group: Option<WeakEntity<TabGroup>>,
}

/// Wire event tags sent over `SHAPE_PANEL_EVENT`.
const EV_ACTIVE: u8 = 0;
const EV_ZOOMED: u8 = 1;
const EV_ADDED: u8 = 2;
const EV_REMOVED: u8 = 3;

impl GkPanel {
    fn event(&self, kind: u8, flag: bool, cx: &mut App) {
        if let Some(h) = self.events {
            genapi::fire_queued(h, SHAPE_PANEL_EVENT, &(kind, flag), cx);
        }
    }
}

impl EventEmitter<PanelEvent> for GkPanel {}

impl Focusable for GkPanel {
    fn focus_handle(&self, _: &App) -> FocusHandle {
        self.focus.clone()
    }
}

impl Render for GkPanel {
    fn render(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        match self.view.clone() {
            Some(v) => v.into_any_element(),
            None => gpui_kit::Empty.into_any_element(),
        }
    }
}

impl BasePanel for GkPanel {
    fn panel_name(&self) -> &'static str {
        self.data.name
    }
    fn visible(&self, _: &App) -> bool {
        self.data.flags & HIDDEN == 0
    }
    fn closable(&self, _: &App) -> bool {
        self.data.flags & NOT_CLOSABLE == 0
    }
    fn zoomable(&self, _: &App) -> bool {
        self.data.flags & NOT_ZOOMABLE == 0
    }
    fn set_active(&mut self, active: bool, _: &mut Window, cx: &mut Context<Self>) {
        self.event(EV_ACTIVE, active, cx);
    }
    fn set_zoomed(&mut self, zoomed: bool, _: &mut Window, cx: &mut Context<Self>) {
        self.event(EV_ZOOMED, zoomed, cx);
    }
    fn on_added_to(&mut self, group: WeakEntity<TabGroup>, _: &mut Window, cx: &mut Context<Self>) {
        self.group = Some(group);
        self.event(EV_ADDED, true, cx);
    }
    fn on_removed(&mut self, _: &mut Window, cx: &mut Context<Self>) {
        self.event(EV_REMOVED, false, cx);
    }
    fn dump(&self, _: &App) -> gpui_kit::base::dock::PanelState {
        let mut s = gpui_kit::base::dock::PanelState::new(self.data.name);
        if !self.data.info.is_empty() {
            if let Ok(v) = serde_json::from_str(&self.data.info) {
                s.info = gpui_kit::base::dock::PanelInfo::Panel(v);
            }
        }
        s
    }
}

impl KitPanel for GkPanel {
    fn tab_name(&self, _: &App) -> Option<SharedString> {
        self.data.tab_name.clone()
    }
    fn title(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        self.data.title.clone()
    }
    fn inner_padding(&self, _: &App) -> bool {
        self.data.flags & NO_PADDING == 0
    }
    fn title_bar(&self, _: &App) -> bool {
        self.data.flags & NO_TITLE_BAR == 0
    }
    fn title_suffix(&mut self, _: &mut Window, _: &mut Context<Self>) -> Option<impl IntoElement> {
        if self.data.suffix == 0 {
            return None;
        }
        rt::with(|s| s.view(self.data.suffix)).ok().map(AnyView::from)
    }
    fn toolbar_buttons(
        &mut self,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Option<Vec<gpui_kit::component::button::Button>> {
        if self.data.toolbar == 0 {
            return None;
        }
        let entity = rt::with(|s| s.view(self.data.toolbar)).ok()?;
        let tape = entity.read(cx).tape.clone()?;
        let view = self.data.toolbar;
        Some(genapi::with_root(&tape, view, window, cx, |dx, _ty, first| {
            let mut out = Vec::new();
            let mut at = first;
            while let Some((op, mut r)) = dx.op(&mut at) {
                if op == E_CHILD {
                    let n = r.u32();
                    if let Some(b) = crate::generated::build_component_button(dx, n) {
                        out.push(b);
                    }
                }
            }
            out
        }))
    }
    fn dropdown_menu(
        &mut self,
        menu: gpui_kit::component::menu::PopupMenu,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui_kit::component::menu::PopupMenu {
        match self.data.menu.clone() {
            Some(vb) => vb.apply(menu, window, cx, crate::generated::apply_component_popup_menu),
            None => menu,
        }
    }
}

/// Wraps a `GkPanel` state handle as a `base::dock::PanelView` for `DockArea` calls.
pub fn panel_view(h: u64) -> GkResult<Arc<dyn PanelView>> {
    Ok(Arc::new(gpui_kit::component::dock::PanelHandle::new(genapi::state::<GkPanel>(h)?)))
}

/// Tape-decode counterpart to [`panel_view`]; aborts via `app_stop` on a stale handle.
pub fn panel_view_shown(h: u64) -> Arc<dyn PanelView> {
    Arc::new(gpui_kit::component::dock::PanelHandle::new(genapi::shown::<GkPanel>(h)))
}

/// Wraps a `GkPanel` state handle as a `component::dock::PanelView` (`KitPanelView`).
pub fn kit_panel_view(h: u64) -> GkResult<Arc<dyn KitPanelView>> {
    Ok(Arc::new(genapi::state::<GkPanel>(h)?))
}

/// Tape-decode counterpart to [`kit_panel_view`]; aborts via `app_stop` on a stale handle.
pub fn kit_panel_view_shown(h: u64) -> Arc<dyn KitPanelView> {
    Arc::new(genapi::shown::<GkPanel>(h))
}

/// Mounts `view` into the active (or first open) window and marks it dirty so its tape is built.
fn mount(view: u64, c: &mut Call) -> GkResult<Entity<GkView>> {
    let entity = rt::with(|s| s.view(view))?;
    let wh = match c.window_handle() {
        Some(any) => rt::with(|s| s.window_of(any)),
        None => rt::with(|s| s.windows.first().copied()),
    };
    if let Some(wh) = wh {
        rt::with(|s| s.mount(view, wh));
        rt::mark_dirty(view, None, c.cx);
    }
    Ok(entity)
}

/// Handles `C_PANEL_NEW`, `C_PANEL_SET`, `C_PANEL_EMIT`, `C_DOCK_AREA_SKINNED`, and delegates
/// layout, overlay, renderer, and pane-tree calls.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {

        C_PANEL_NEW => (|| {
            let view = mount(r.u64(), c)?;
            let mut data = read_data(r);
            prepare(&mut data, c)?;
            let (slot, generation) = (r.u32(), r.u32());
            let events = (slot != 0 || generation != 0).then_some(HandlerRef { view: 0, generation, handler: slot });
            let entity = c.cx.new(|cx| GkPanel { view: Some(view), focus: cx.focus_handle(), data, events, group: None });
            let id = entity.entity_id();
            let h = genapi::state_new(entity, c);
            HANDED.with(|m| m.borrow_mut().insert(id, h));
            c.out.put_u64(h);
            Ok(())
        })(),

        C_PANEL_SET => (|| {
            let entity = genapi::state::<GkPanel>(r.u64())?;
            let mut data = read_data(r);
            prepare(&mut data, c)?;
            entity.update(c.cx, |p, cx| {
                p.data = data;
                cx.notify();
            });
            Ok(())
        })(),

        C_PANEL_EMIT => (|| {
            let entity = genapi::state::<GkPanel>(r.u64())?;
            let ev = match r.u8() {
                0 => PanelEvent::ZoomIn,
                1 => PanelEvent::ZoomOut,
                2 => PanelEvent::LayoutChanged,
                n => return genapi::err(ERR_BAD_OP, format!("panel event {n}")),
            };
            entity.update(c.cx, |_, cx| cx.emit(ev));
            Ok(())
        })(),

        C_DOCK_AREA_SKINNED => (|| {
            let id = SharedString::new(r.str());
            let version = if r.u8() != 0 { Some(r.u64() as usize) } else { r.u64(); None };
            let window = c.window()?;
            let (area, skin) = DockSkin::dock_area(id, version, window, c.cx);
            let id = area.entity_id();
            let h = genapi::state_new(area, c);
            HANDED.with(|m| m.borrow_mut().insert(id, h));
            SKINS.with(|s| s.borrow_mut().insert(h, skin));
            c.out.put_u64(h);
            Ok(())
        })(),
        _ => return call3(op, r, c),
    })
}

/// Decodes `N_DOCK_VIEW` (an embedded `DockArea` entity) and `N_DOCK_SLOT` (takes the active
/// upstream child element stashed by [`GkRenderer::element`]).
pub fn node(dx: &mut genapi::Dx, ty: u16, args: u32, _first: u32) -> Option<AnyElement> {
    match ty {
        N_DOCK_VIEW => {
            let mut r = dx.args(args);
            Some(genapi::shown::<DockArea>(r.u64()).into_any_element())
        }
        N_DOCK_SLOT => Some(SLOT.with(|s| s.borrow_mut().take()).unwrap_or_else(|| gpui_kit::Empty.into_any_element())),
        _ => None,
    }
}

use gpui_kit::base::dock::{NodeId, PanelId};
use gpui_kit::{AnyView, AnyWindowHandle, EntityId};
use std::cell::RefCell;

thread_local! {
    /// Remembers `NodeId` values handed to the host so wire `u64` IDs can be validated on return.
    static NODE_IDS: RefCell<HashMap<u64, NodeId>> = RefCell::new(HashMap::new());
    /// Canonical `EntityId -> HK_STATE` handle cache so repeated reads of an entity return the same handle.
    static HANDED: RefCell<HashMap<EntityId, u64>> = RefCell::new(HashMap::new());
    /// Reverse index from `GkView` `EntityId` to its `HK_VIEW` handle.
    static VIEWS: RefCell<HashMap<EntityId, u64>> = RefCell::new(HashMap::new());
}

fn remember_node(id: NodeId) -> u64 {
    NODE_IDS.with(|m| m.borrow_mut().insert(id.as_u64(), id));
    id.as_u64()
}

/// Resolves a raw `u64` previously handed out by [`remember_node`] back to its `NodeId`.
pub fn node_id(raw: u64) -> GkResult<NodeId> {
    NODE_IDS.with(|m| m.borrow().get(&raw).copied()).ok_or_else(|| {
        genapi::GkErr(ERR_BAD_OP, format!("node id {raw} was never handed out by the dock (ids come from its reads)"))
    })
}

/// Tape-decode counterpart to [`node_id`]; stops the app if `raw` was never issued by the dock.
pub fn node_id_shown(raw: u64) -> NodeId {
    match node_id(raw) {
        Ok(id) => id,
        Err(genapi::GkErr(_, msg)) => genapi::released_shown(&msg, "dock node id"),
    }
}

/// Mounts host view `h` into the active window, caches its `EntityId -> handle` reverse entry,
/// and returns it as a GPUI `AnyView`.
pub fn any_view(h: u64, c: &mut Call) -> GkResult<AnyView> {
    let entity = mount(h, c)?;
    VIEWS.with(|m| m.borrow_mut().insert(entity.entity_id(), h));
    Ok(entity.into())
}

/// Resolves an already-mounted view handle `h` into `AnyView` without requiring a `Call` context.
pub fn any_view_c(h: u64) -> GkResult<AnyView> {
    let entity = rt::with(|s| s.view(h))?;
    VIEWS.with(|m| m.borrow_mut().insert(entity.entity_id(), h));
    Ok(entity.into())
}

/// Tape-decode counterpart to [`any_view_c`]; stops the app on a stale view handle.
pub fn any_view_shown(h: u64) -> AnyView {
    match any_view_c(h) {
        Ok(v) => v,
        Err(genapi::GkErr(_, msg)) => genapi::released_shown(&msg, "view"),
    }
}

fn view_handle(v: &AnyView) -> u64 {
    let id = v.entity_id();
    let h = VIEWS.with(|m| m.borrow().get(&id).copied()).unwrap_or(0);
    if h != 0 && rt::with(|s| s.view(h).map(|e| e.entity_id() == id).unwrap_or(false)) { h } else { 0 }
}

fn panel_handle_of(v: AnyView) -> u64 {
    let id = v.entity_id();
    let h = HANDED.with(|m| m.borrow().get(&id).copied()).unwrap_or(0);
    if h != 0 && rt::with(|s| s.state(h).is_ok()) { h } else { 0 }
}

fn first_window() -> Option<AnyWindowHandle> {
    rt::with(|s| s.windows.first().copied().and_then(|w| s.window(w).ok()))
}

/// Returns the canonical `HK_STATE` handle for `entity`, creating and caching one on first sight.
pub fn entity_handle<T: 'static>(entity: &Entity<T>) -> u64 {
    let id = entity.entity_id();
    if let Some(h) = HANDED.with(|m| m.borrow().get(&id).copied()) {
        if genapi::state::<T>(h).map(|e| e.entity_id() == id).unwrap_or(false) {
            return h;
        }
    }
    let observed = entity.clone();
    let observe = std::rc::Rc::new(move |cx: &mut App, view: u64| {
        cx.observe(&observed, move |_, cx| {
            rt::guard("marking a view dirty", || rt::mark_dirty(view, None, cx));
        })
    });
    let window = first_window();
    let h = rt::with(|s| {
        s.insert(
            HK_STATE,
            rt::Item::State(rt::StateItem {
                any: std::rc::Rc::new(entity.clone()),
                type_name: std::any::type_name::<T>(),
                window,
                observe,
            }),
        )
    });
    HANDED.with(|m| m.borrow_mut().insert(id, h));
    h
}

/// Uniform handle extraction for strong and weak GPUI entity references (`0` if a `WeakEntity` dropped).
pub trait AsEntity {
    fn handle(&self) -> u64;
}
impl<T: 'static> AsEntity for Entity<T> {
    fn handle(&self) -> u64 {
        entity_handle(self)
    }
}
impl<T: AsEntity + ?Sized> AsEntity for &T {
    fn handle(&self) -> u64 {
        (**self).handle()
    }
}
impl<T: 'static> AsEntity for WeakEntity<T> {
    fn handle(&self) -> u64 {
        self.upgrade().map(|e| entity_handle(&e)).unwrap_or(0)
    }
}

/// Writes an entity's canonical `u64` handle onto `out`.
pub fn put_entity<E: AsEntity>(e: &E, out: &mut Vec<u8>) {
    out.put_u64(e.handle());
}

/// Writes `Duration` as signed little-endian nanoseconds (clamped to `i64::MAX`).
pub fn put_ms(d: &std::time::Duration, out: &mut Vec<u8>) {
    out.extend_from_slice(&(d.as_nanos().min(i64::MAX as u128) as i64).to_le_bytes());
}

/// Encodes `Option<T>` using custom serializer `f` with a 1-byte presence tag.
pub fn opt_with<T>(v: &Option<T>, out: &mut Vec<u8>, f: impl Fn(&T, &mut Vec<u8>)) {
    match v {
        Some(x) => {
            out.put_u8(1);
            f(x, out);
        }
        None => out.put_u8(0),
    }
}

/// Encodes slice `v` with a `u32` element count followed by `f` per element.
pub fn list_with<T>(v: &[T], out: &mut Vec<u8>, f: impl Fn(&T, &mut Vec<u8>)) {
    out.put_u32(v.len() as u32);
    for x in v {
        f(x, out);
    }
}

/// Serialization trait for dock-domain identifiers, panel views, and layout structs.
pub trait DockOut {
    fn put_dock(&self, out: &mut Vec<u8>);
}
impl DockOut for NodeId {
    fn put_dock(&self, out: &mut Vec<u8>) {
        out.put_u64(remember_node(*self));
    }
}
impl DockOut for PanelId {
    fn put_dock(&self, out: &mut Vec<u8>) {
        out.put_u64(self.as_u64());
    }
}
impl DockOut for AnyView {
    fn put_dock(&self, out: &mut Vec<u8>) {
        out.put_u64(view_handle(self));
    }
}
impl DockOut for Arc<dyn PanelView> {
    fn put_dock(&self, out: &mut Vec<u8>) {
        out.put_u64(panel_handle_of(self.view()));
    }
}
impl DockOut for Arc<dyn KitPanelView> {
    fn put_dock(&self, out: &mut Vec<u8>) {
        out.put_u64(panel_handle_of(self.view()));
    }
}
impl<T: DockOut + ?Sized> DockOut for &T {
    fn put_dock(&self, out: &mut Vec<u8>) {
        (**self).put_dock(out)
    }
}
impl<T: DockOut> DockOut for Option<T> {
    fn put_dock(&self, out: &mut Vec<u8>) {
        match self {
            Some(x) => {
                out.put_u8(1);
                x.put_dock(out)
            }
            None => out.put_u8(0),
        }
    }
}
impl<T: DockOut> DockOut for [T] {
    fn put_dock(&self, out: &mut Vec<u8>) {
        out.put_u32(self.len() as u32);
        for x in self {
            x.put_dock(out)
        }
    }
}
impl<T: DockOut> DockOut for Vec<T> {
    fn put_dock(&self, out: &mut Vec<u8>) {
        self.as_slice().put_dock(out)
    }
}

impl genapi::Wire for NodeId {
    fn put(&self, out: &mut Vec<u8>) {
        self.put_dock(out)
    }
}
impl genapi::Wire for PanelId {
    fn put(&self, out: &mut Vec<u8>) {
        self.put_dock(out)
    }
}
impl genapi::Wire for AnyView {
    fn put(&self, out: &mut Vec<u8>) {
        self.put_dock(out)
    }
}

/// Newtype bridging any [`DockOut`] value to [`genapi::Wire`].
pub struct Out<T>(pub T);
impl<T: DockOut> genapi::Wire for Out<T> {
    fn put(&self, out: &mut Vec<u8>) {
        self.0.put_dock(out)
    }
}

/// Handles `C_PANEL_GROUP` and `C_PANEL_ID`, then chains to layout, overlay, renderer, and pane calls.
fn call3(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {

        C_PANEL_GROUP => (|| {
            let entity = genapi::state::<GkPanel>(r.u64())?;
            let g = entity.read(c.cx).group.as_ref().and_then(|g| g.upgrade());
            c.out.put_u64(g.map(|g| entity_handle(&g)).unwrap_or(0));
            Ok(())
        })(),

        C_PANEL_ID => (|| {
            let entity = genapi::state::<GkPanel>(r.u64())?;
            c.out.put_u64(PanelId::from(entity.entity_id()).as_u64());
            Ok(())
        })(),
        _ => {
            return layout_call(op, r, c)
                .or_else(|| crate::overlay::call(op, r, c))
                .or_else(|| render_call(op, r, c))
                .or_else(|| pane_call(op, r, c))
        }
    })
}

use gpui_kit::base::dock::{
    DockAreaState, DockPlacement, DockState, PaneTree, PanelBuildContext, PanelBuilder, PanelInfo, PanelRegistry,
    PanelSource, PanelState, RootKind,
};
use genapi::{Unwire, Wire};

/// Wire encoding for `PanelInfo`: tag `0` = Stack (`[n: u32][px: f32*][axis: u64]`),
/// tag `1` = Tabs (`[active_index: u64]`), tag `2` = Panel (`[json: str]`).
impl Wire for PanelInfo {
    fn put(&self, out: &mut Vec<u8>) {
        match self {
            PanelInfo::Stack { sizes, axis } => {
                out.put_u16(0);
                out.put_u32(sizes.len() as u32);
                for s in sizes {
                    s.put(out);
                }
                out.put_u64(*axis as u64);
            }
            PanelInfo::Tabs { active_index } => {
                out.put_u16(1);
                out.put_u64(*active_index as u64);
            }
            PanelInfo::Panel(v) => {
                out.put_u16(2);
                out.put_str(&if v.is_null() { String::new() } else { v.to_string() });
            }
        }
    }
}

impl Unwire for PanelInfo {
    fn get(r: &mut R) -> Self {
        match r.u16() {
            0 => {
                let n = r.u32() as usize;
                let sizes = (0..n).map(|_| gpui_kit::px(r.f32())).collect();
                PanelInfo::Stack { sizes, axis: r.u64() as usize }
            }
            1 => PanelInfo::Tabs { active_index: r.u64() as usize },
            _ => {
                let s = r.str();
                PanelInfo::Panel(if s.is_empty() {
                    serde_json::Value::Null
                } else {
                    serde_json::from_str(s).unwrap_or(serde_json::Value::String(s.to_string()))
                })
            }
        }
    }
}

impl Wire for PanelState {
    fn put(&self, out: &mut Vec<u8>) {
        out.put_str(&self.panel_name);
        out.put_u32(self.children.len() as u32);
        for c in &self.children {
            c.put(out);
        }
        self.info.put(out);
    }
}

impl Unwire for PanelState {
    fn get(r: &mut R) -> Self {
        let panel_name = r.str().to_string();
        let n = r.u32() as usize;
        let children = (0..n.min(1 << 20)).map(|_| PanelState::get(r)).collect();
        let info = PanelInfo::get(r);
        PanelState { panel_name, children, info }
    }
}

fn placement(n: u16) -> DockPlacement {
    match n {
        1 => DockPlacement::Left,
        2 => DockPlacement::Bottom,
        3 => DockPlacement::Right,
        _ => DockPlacement::Center,
    }
}

fn put_dock_state(d: &Option<DockState>, out: &mut Vec<u8>) {
    match d {
        Some(d) => {
            out.put_u8(1);
            d.panel().put(out);
            d.placement().put(out);
            d.size().put(out);
            out.put_u8(d.open() as u8);
        }
        None => out.put_u8(0),
    }
}

fn get_dock_state(r: &mut R) -> Option<DockState> {
    if r.u8() == 0 {
        return None;
    }
    let panel = PanelState::get(r);
    let p = placement(r.u16());
    let size = gpui_kit::px(r.f32());
    Some(DockState::new(panel, p, size, r.u8() != 0))
}

impl Wire for DockAreaState {
    fn put(&self, out: &mut Vec<u8>) {
        self.version.map(|v| v as u64).put(out);
        self.center.put(out);
        put_dock_state(&self.left_dock, out);
        put_dock_state(&self.right_dock, out);
        put_dock_state(&self.bottom_dock, out);
    }
}

impl Unwire for DockAreaState {
    fn get(r: &mut R) -> Self {
        let version = if r.u8() != 0 { Some(r.u64() as usize) } else { None };
        let center = PanelState::get(r);
        let left_dock = get_dock_state(r);
        let right_dock = get_dock_state(r);
        let bottom_dock = get_dock_state(r);
        DockAreaState { version, center, left_dock, right_dock, bottom_dock }
    }
}

impl DockOut for PanelState {
    fn put_dock(&self, out: &mut Vec<u8>) {
        self.put(out)
    }
}
impl DockOut for PanelInfo {
    fn put_dock(&self, out: &mut Vec<u8>) {
        self.put(out)
    }
}
impl DockOut for DockAreaState {
    fn put_dock(&self, out: &mut Vec<u8>) {
        self.put(out)
    }
}

/// Callback registered in `PanelRegistry` by `C_PANEL_REGISTER`: allocates an empty placeholder
/// `GkPanel` immediately and queues `SHAPE_PANEL_BUILD` so the host fills it via `C_PANEL_FILL`.
fn requested(h: HandlerRef, ctx: PanelBuildContext, cx: &mut App) -> Arc<dyn PanelView> {
    let state = ctx.state().clone();
    let entity = new_empty_panel(&state, cx);
    let ph = entity_handle(&entity);
    let dock = ctx.dock_area().upgrade().map(|d| entity_handle(&d)).unwrap_or(0);
    genapi::fire_queued(h, SHAPE_PANEL_BUILD, &(ph, state.panel_name.as_str(), &state, dock), cx);
    Arc::new(entity)
}

/// Creates an unpopulated `GkPanel` carrying only the persisted name and JSON info until filled.
fn new_empty_panel(state: &PanelState, cx: &mut App) -> Entity<GkPanel> {
    let info = match &state.info {
        PanelInfo::Panel(v) if !v.is_null() => v.to_string(),
        _ => String::new(),
    };
    let data = PanelData {
        name: intern(&state.panel_name),
        tab_name: None,
        title: SharedString::new(state.panel_name.as_str()),
        flags: 0,
        suffix: 0,
        toolbar: 0,
        menu: None,
        menu_view: 0,
        info,
    };
    cx.new(|cx| GkPanel { view: None, focus: cx.focus_handle(), data, events: None, group: None })
}

/// Snapshot `PanelSource` pre-populated from a `DockArea` so `PaneTree::to_state` needs no `&App`.
struct GkSource(HashMap<u64, (&'static str, bool, PanelState)>);

impl PanelSource for GkSource {
    fn panel_name(&self, id: PanelId) -> &'static str {
        self.0.get(&id.as_u64()).map(|p| p.0).unwrap_or("InvalidPanel")
    }
    fn is_visible(&self, id: PanelId) -> bool {
        self.0.get(&id.as_u64()).map(|p| p.1).unwrap_or(false)
    }
    fn dump(&self, id: PanelId) -> PanelState {
        self.0.get(&id.as_u64()).map(|p| p.2.clone()).unwrap_or_default()
    }
}

/// `PanelBuilder` adapter used by `C_PANE_TREE_FROM_STATE` to instantiate registered panels or
/// fallback empty placeholders while pinning their `Arc<dyn PanelView>` in `BUILT`.
struct GkBuilder<'w, 'c> {
    dock: WeakEntity<DockArea>,
    window: &'w mut Window,
    cx: &'c mut App,
}

thread_local! {
    /// Holds `PanelView` arcs created during `PaneTree::from_state` so they stay alive until
    /// attached or returned.
    static BUILT: RefCell<Vec<Arc<dyn PanelView>>> = const { RefCell::new(Vec::new()) };
}

impl PanelBuilder for GkBuilder<'_, '_> {
    fn build(&mut self, state: &PanelState, info: &PanelInfo) -> PanelId {
        let ctx = PanelBuildContext::new(self.dock.clone(), state, info);
        let registered = self.cx.try_global::<PanelRegistry>().is_some();
        let view = if registered { PanelRegistry::build_panel(&state.panel_name, ctx, self.window, self.cx) } else { None };
        let view = view.unwrap_or_else(|| Arc::new(new_empty_panel(state, self.cx)) as Arc<dyn PanelView>);
        let id = view.panel_id(self.cx);
        BUILT.with(|b| b.borrow_mut().push(view));
        id
    }
}

/// Handles panel-registry and pane-tree persistence calls (`C_PANEL_REGISTER`, `C_PANEL_FILL`,
/// `C_PANEL_BUILD_NOW`, `C_PANE_TREE_TO_STATE`, `C_PANE_TREE_FROM_STATE`).
fn layout_call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {

        C_PANEL_REGISTER => (|| {
            let name = r.str().to_string();
            let (slot, generation) = (r.u32(), r.u32());
            let h = HandlerRef { view: 0, generation, handler: slot };
            gpui_kit::base::dock::register_panel(c.cx, &name, move |ctx, _window, cx| requested(h, ctx, cx));
            Ok(())
        })(),

        C_PANEL_FILL => (|| {
            let entity = genapi::state::<GkPanel>(r.u64())?;
            let view = mount(r.u64(), c)?;
            let mut data = read_data(r);
            prepare(&mut data, c)?;
            let (slot, generation) = (r.u32(), r.u32());
            let events = (slot != 0 || generation != 0).then_some(HandlerRef { view: 0, generation, handler: slot });
            entity.update(c.cx, |p, cx| {
                p.view = Some(view);
                p.data = data;
                p.events = events;
                cx.notify();
            });
            Ok(())
        })(),

        C_PANEL_BUILD_NOW => (|| {
            let dock = genapi::state::<DockArea>(r.u64())?;
            let state = PanelState::get(r);
            let window = c.window()?;
            let built = if c.cx.try_global::<PanelRegistry>().is_some() {
                let ctx = PanelBuildContext::new(dock.downgrade(), &state, &state.info);
                PanelRegistry::build_panel(&state.panel_name, ctx, window, c.cx)
            } else {
                None
            };
            Out(built).put(c.out);
            Ok(())
        })(),

        C_PANE_TREE_TO_STATE => (|| {
            let tree = genapi::obj_rc::<PaneTree>(r.u64())?;
            let dock = genapi::state::<DockArea>(r.u64())?;
            let tree = tree.try_borrow().map_err(|_| genapi::GkErr(ERR_BUSY, "the pane tree is in use".into()))?;
            let mut map = HashMap::new();
            let area = dock.read(c.cx);
            for id in tree.panels() {
                if let Some(pv) = area.panel(id) {
                    map.insert(id.as_u64(), (pv.panel_name(c.cx), pv.visible(c.cx), pv.dump(c.cx)));
                }
            }
            tree.to_state(&GkSource(map)).put(c.out);
            Ok(())
        })(),

        C_PANE_TREE_FROM_STATE => (|| {
            let dock = genapi::state::<DockArea>(r.u64())?;
            let state = PanelState::get(r);
            let kind = if r.u16() == 0 { RootKind::Split } else { RootKind::Any };
            let window = c.window()?;
            let mut b = GkBuilder { dock: dock.downgrade(), window, cx: &mut *c.cx };
            let tree = PaneTree::from_state(&state, kind, &mut b);
            c.out.put_u64(genapi::obj_new(tree));
            Ok(())
        })(),
        _ => return None,
    })
}

/// Handles pane-node inspection (`C_PANE_NODE_KIND`, `C_PANE_NODE_WALK`) and `C_DOCK_SKIN`.
fn pane_call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    use gpui_kit::base::dock::{PaneNode, PaneRef};
    Some(match op {

        C_PANE_NODE_KIND => (|| {
            let node = genapi::obj_rc::<PaneNode>(r.u64())?;
            let node = node.try_borrow().map_err(|_| genapi::GkErr(ERR_BUSY, "the pane node is in use".into()))?;
            match node.kind() {
                PaneRef::Split { axis, children, sizes } => {
                    c.out.put_u16(0);
                    c.out.put_u16(if axis == gpui_kit::Axis::Vertical { 0 } else { 1 });
                    c.out.put_u32(children.len() as u32);
                    for ch in children {
                        c.out.put_u64(crate::objects::obj_temp(ch.clone()));
                    }
                    c.out.put_u32(sizes.len() as u32);
                    for s in sizes {
                        s.put(c.out);
                    }
                }
                PaneRef::Tabs { panels, active_ix } => {
                    c.out.put_u16(1);
                    c.out.put_u32(panels.len() as u32);
                    for p in panels {
                        c.out.put_u64(p.as_u64());
                    }
                    c.out.put_u64(active_ix as u64);
                }
            }
            Ok(())
        })(),

        C_DOCK_SKIN => skin_call(r, c),
        C_PANE_NODE_WALK => (|| {
            let node = genapi::obj_rc::<PaneNode>(r.u64())?;
            let node = node.try_borrow().map_err(|_| genapi::GkErr(ERR_BUSY, "the pane node is in use".into()))?;
            let mut all = Vec::new();
            node.walk(&mut |n: &PaneNode| all.push(n.clone()));
            c.out.put_u32(all.len() as u32);
            for n in all {
                c.out.put_u64(crate::objects::obj_temp(n));
            }
            Ok(())
        })(),
        _ => return None,
    })
}

thread_local! {
    /// Associates each `DockArea` state handle opened via `C_DOCK_AREA_SKINNED` with its `DockSkin`.
    static SKINS: RefCell<HashMap<u64, std::rc::Rc<DockSkin>>> = RefCell::new(HashMap::new());
}

/// Dispatches `C_DOCK_SKIN` sub-opcodes (`0..=8`) to read or update `DockSkin` visibility and
/// panel-style flags on a skinned `DockArea`.
fn skin_call(r: &mut R, c: &mut Call) -> GkResult<()> {
    use gpui_kit::component::dock::PanelStyle;
    let h = r.u64();
    genapi::state::<DockArea>(h)?;
    let skin = SKINS.with(|s| s.borrow().get(&h).cloned()).ok_or_else(|| {
        genapi::GkErr(ERR_BAD_OP, "this dock area has no skin (make it with dock.NewAreaSkinned)".into())
    })?;
    match r.u8() {
        0 => c.out.put_u16(match skin.panel_style() {
            PanelStyle::TabBar => 1,
            _ => 0,
        }),
        1 => skin.set_panel_style(if r.u16() == 1 { PanelStyle::TabBar } else { PanelStyle::Auto }, c.cx),
        2 => c.out.put_u8(skin.is_toggle_button_visible() as u8),
        3 => skin.set_toggle_button_visible(r.u8() != 0, c.cx),
        4 => skin.set_close_button_visible(r.u8() != 0, c.cx),
        5 => c.out.put_u8(skin.is_collapsed_tab_bar_visible() as u8),
        6 => skin.set_collapsed_tab_bar_visible(r.u8() != 0, c.cx),
        7 => c.out.put_u8(skin.is_split_divider_line_visible() as u8),
        8 => skin.set_split_divider_line_visible(r.u8() != 0, c.cx),
        n => return genapi::err(ERR_BAD_OP, format!("dock skin call {n}")),
    }
    Ok(())
}

use gpui_kit::base::dock::{DockAreaRenderer, DockContext, DropIndicator, TabGroupContext, TabGroupRenderer};
use gpui_kit::base::ResizeHandleContext;
use gpui_kit::{Axis, Div, ElementId, InteractiveElement as _, ParentElement as _, Stateful, Styled as _};
use std::hash::{Hash, Hasher};
use std::rc::Rc;

/// Hook bit indices for [`GkRenderer::hooks`].
const H_FRAME: u8 = 0;
const H_CENTER_FRAME: u8 = 1;
const H_SPLIT_FRAME: u8 = 2;
const H_SPLIT_HANDLE: u8 = 3;
const H_DOCK: u8 = 4;
const H_PLACEHOLDER: u8 = 5;
const H_TAB_FRAME: u8 = 6;
const H_TAB_CONTENT_FRAME: u8 = 7;
const H_TAB_BAR: u8 = 8;
const H_TAB_ACTIVE_PANEL: u8 = 9;
const H_TAB_DROP_INDICATOR: u8 = 10;
const H_TAB_EMPTY: u8 = 11;

/// Host-driven implementation of `DockAreaRenderer` and `TabGroupRenderer`.
///
/// `hooks` is a bitmask of `H_*` hooks the host opted to customize; unmasked hooks fall back to
/// upstream defaults without crossing the wire.
#[derive(Clone, Copy)]
pub struct GkRenderer {
    id: u64,
    hooks: u32,
    events: HandlerRef,
}

#[derive(Default)]
struct RenderBook {
    /// Host-supplied view handle per `(renderer_id, hook, key)` set by `C_DOCK_RENDER_ANSWER`.
    answers: HashMap<(u64, u8, u64), u64>,
    /// Last context hash requested for `(renderer_id, hook, key)` so unchanged frames do not re-ask.
    asked: HashMap<(u64, u8, u64), u64>,
    /// Owned `DockContext`/`TabGroupContext` handles kept alive for the latest request per key.
    ctx: HashMap<(u64, u8, u64), Vec<u64>>,
    sent: HashMap<u64, u32>,
    drawn: HashMap<u64, u32>,
}

thread_local! {
    static BOOK: RefCell<RenderBook> = RefCell::new(RenderBook::default());
    /// Active upstream child element exposed to `N_DOCK_SLOT` while decoding a renderer view tape.
    static SLOT: RefCell<Option<AnyElement>> = const { RefCell::new(None) };
}

/// Resolves a `GkRenderer` object handle into an `Rc<GkRenderer>` for wire calls.
pub fn renderer(h: u64) -> GkResult<Rc<GkRenderer>> {
    genapi::obj_clone::<GkRenderer>(h).map(Rc::new)
}

/// Tape-decode counterpart to [`renderer`]; stops the app on a stale handle.
pub fn renderer_shown(h: u64) -> Rc<GkRenderer> {
    Rc::new(genapi::obj_shown::<GkRenderer>(h))
}

/// Wire payload fields for a `SHAPE_DOCK_RENDER` request describing one dock/tab-group hook site.
#[derive(Default)]
struct Req {
    node: u64,
    placement: u16,
    axis: u16,
    flags: u8,
    size: f32,
    dock: u64,
    group: u64,
    panel: u64,
    name: String,
}

struct Ask<'a>(u64, u8, u64, &'a Req);

impl Wire for Ask<'_> {
    fn put(&self, out: &mut Vec<u8>) {
        let r = self.3;
        out.put_u64(self.0);
        out.put_u8(self.1);
        out.put_u64(self.2);
        out.put_u64(r.node);
        out.put_u16(r.placement);
        out.put_u16(r.axis);
        out.put_u8(r.flags);
        out.put_f32(r.size);
        out.put_u64(r.dock);
        out.put_u64(r.group);
        out.put_u64(r.panel);
        out.put_str(&r.name);
    }
}

fn snap(f: impl FnOnce(&mut std::collections::hash_map::DefaultHasher)) -> u64 {
    let mut h = std::collections::hash_map::DefaultHasher::new();
    f(&mut h);
    h.finish() | 1
}

fn placement_num(p: DockPlacement) -> u16 {
    match p {
        DockPlacement::Left => 1,
        DockPlacement::Bottom => 2,
        DockPlacement::Right => 3,
        _ => 0,
    }
}

fn axis_num(a: Axis) -> u16 {
    if a == Axis::Vertical { 0 } else { 1 }
}

fn dock_snap(d: &DockContext) -> u64 {
    snap(|h| (placement_num(d.placement()), f32::from(d.size()).to_bits(), d.is_open(), d.is_collapsible()).hash(h))
}

fn group_snap(g: &TabGroupContext, cx: &App) -> u64 {
    snap(|h| {
        g.node().as_u64().hash(h);
        for p in g.panels() {
            p.panel_id(cx).as_u64().hash(h);
        }
        (g.active_ix(), g.is_zoomed(), g.is_collapsed(), g.is_closable(), g.is_locked()).hash(h);
    })
}

fn group_req(g: &TabGroupContext) -> Req {
    let node = remember_node(g.node());
    let flags = g.is_zoomed() as u8 | (g.is_collapsed() as u8) << 1 | (g.is_closable() as u8) << 2 | (g.is_locked() as u8) << 3;
    Req { node, flags, group: crate::objects::obj_new(g.clone()), ..Req::default() }
}

impl GkRenderer {
    /// Looks up the host view answering `(self.id, hook, key)`, sending a queued
    /// `SHAPE_DOCK_RENDER` request whenever the context snapshot `sn` changed since the last ask.
    ///
    /// Context object handles from the previous request for the same key are released once replaced.
    fn view(&self, hook: u8, key: u64, sn: u64, make: impl FnOnce() -> Req, cx: &mut App) -> Option<u64> {
        if self.hooks & (1u32 << hook) == 0 {
            return None;
        }
        let k = (self.id, hook, key);
        let (ans, asked) = BOOK.with(|b| {
            let b = b.borrow();
            (b.answers.get(&k).copied(), b.asked.get(&k).copied())
        });
        if asked != Some(sn) {
            let req = make();
            let objs: Vec<u64> = [req.dock, req.group].into_iter().filter(|h| *h != 0).collect();
            let old = BOOK.with(|b| {
                let mut b = b.borrow_mut();
                b.asked.insert(k, sn);
                *b.sent.entry(self.id).or_default() += 1;
                b.ctx.insert(k, objs)
            });
            if let Some(old) = old {
                let items: Vec<_> = rt::with(|s| old.iter().filter_map(|h| s.release(*h).ok()).collect());
                drop(items);
            }
            genapi::fire_queued(self.events, SHAPE_DOCK_RENDER, &Ask(self.id, hook, key, &req), cx);
        }
        ans.filter(|v| *v != 0 && rt::with(|s| s.view(*v)).is_ok())
    }

    fn drawn(&self) {
        BOOK.with(|b| *b.borrow_mut().drawn.entry(self.id).or_default() += 1);
    }

    /// Decodes `view`'s current tape with `slot` stashed in thread-local `SLOT` for `N_DOCK_SLOT`.
    /// Returns `Err(slot)` without losing `slot` if the view has no tape yet.
    fn element(&self, view: u64, slot: Option<AnyElement>, window: &mut Window, cx: &mut App) -> Result<AnyElement, Option<AnyElement>> {
        let tape = rt::with(|s| s.view(view)).ok().and_then(|e| e.read(cx).tape.clone());
        let Some(tape) = tape else { return Err(slot) };
        SLOT.with(|s| *s.borrow_mut() = slot);
        let el = genapi::root(&tape, view, window, cx);
        SLOT.with(|s| s.borrow_mut().take());
        self.drawn();
        Ok(el)
    }

    /// Decodes a `Stateful<Div>` frame hook (`256..=258` = Div/HFlex/VFlex), stopping the app if
    /// the host returned a non-div root node.
    fn frame(&self, hook: u8, key: u64, sn: u64, id: ElementId, make: impl FnOnce() -> Req, window: &mut Window, cx: &mut App) -> Stateful<Div> {
        let Some(view) = self.view(hook, key, sn, make, cx) else { return gpui_kit::div().id(id) };
        let tape = rt::with(|s| s.view(view)).ok().and_then(|e| e.read(cx).tape.clone());
        let Some(tape) = tape else { return gpui_kit::div().id(id) };
        let root = tape.root;
        let d = genapi::with_root(&tape, view, window, cx, |dx, _, _| {
            let (ty, args, first) = dx.node(root);
            match ty {
                256..=258 => crate::generated::make_gpui_div(dx, ty, args, first),
                _ => {
                    crate::rt::app_stop(format!(
                        "the view a dock renderer gave for a frame hook returned a node of type {ty}: a frame is a \
                         gk.Div() (or gk.HFlex(), gk.VFlex()), whose styles and children upstream's frame takes"
                    ))
                }
            }
        });
        self.drawn();
        d.unwrap_or_else(gpui_kit::div).id(id)
    }

    fn el(
        &self,
        hook: u8,
        key: u64,
        sn: u64,
        slot: Option<AnyElement>,
        make: impl FnOnce() -> Req,
        window: &mut Window,
        cx: &mut App,
    ) -> Result<AnyElement, Option<AnyElement>> {
        match self.view(hook, key, sn, make, cx) {
            Some(v) => self.element(v, slot, window, cx),
            None => Err(slot),
        }
    }
}

impl DockAreaRenderer for GkRenderer {
    fn frame(&self, window: &mut Window, cx: &mut App) -> Stateful<Div> {
        GkRenderer::frame(self, H_FRAME, 0, 1, "dock-area".into(), Req::default, window, cx)
    }

    fn split_frame(&self, node: NodeId, axis: Axis, window: &mut Window, cx: &mut App) -> Stateful<Div> {
        let n = remember_node(node);
        let make = || Req { node: n, axis: axis_num(axis), ..Req::default() };
        GkRenderer::frame(self, H_SPLIT_FRAME, n, 1 + axis_num(axis) as u64, ("dock-split-frame", n).into(), make, window, cx)
    }

    fn center_frame(&self, window: &mut Window, cx: &mut App) -> Stateful<Div> {
        GkRenderer::frame(self, H_CENTER_FRAME, 0, 1, "dock-area-center".into(), Req::default, window, cx)
    }

    fn render_split_handle(&self, handle: &ResizeHandleContext, window: &mut Window, cx: &mut App) -> Option<AnyElement> {
        let (axis, active) = (axis_num(handle.axis()), handle.is_active());
        let key = axis as u64 | (active as u64) << 1;
        let make = || Req { axis, flags: active as u8, ..Req::default() };
        self.el(H_SPLIT_HANDLE, key, 1, None, make, window, cx).ok()
    }

    fn render_dock(&self, dock: &DockContext, content: AnyElement, window: &mut Window, cx: &mut App) -> AnyElement {
        let p = placement_num(dock.placement());
        let make = || Req {
            placement: p,
            flags: dock.is_open() as u8 | (dock.is_collapsible() as u8) << 1,
            size: f32::from(dock.size()),
            dock: crate::objects::obj_new(dock.clone()),
            ..Req::default()
        };
        match self.el(H_DOCK, p as u64, dock_snap(dock), Some(content), make, window, cx) {
            Ok(el) => el,
            Err(content) => content.unwrap_or_else(|| gpui_kit::Empty.into_any_element()),
        }
    }

    fn build_placeholder(&self, state: &PanelState, _window: &mut Window, cx: &mut App) -> Option<Arc<dyn PanelView>> {
        if self.hooks & (1u32 << H_PLACEHOLDER) == 0 {
            return None;
        }

        let entity = new_empty_panel(state, cx);
        let req = Req { panel: entity_handle(&entity), name: state.panel_name.clone(), ..Req::default() };
        BOOK.with(|b| *b.borrow_mut().sent.entry(self.id).or_default() += 1);
        genapi::fire_queued(self.events, SHAPE_DOCK_RENDER, &Ask(self.id, H_PLACEHOLDER, 0, &req), cx);
        Some(Arc::new(entity))
    }

    fn tab_group_renderer(&self) -> Rc<dyn TabGroupRenderer> {
        Rc::new(*self)
    }
}

impl TabGroupRenderer for GkRenderer {
    fn frame(&self, group: &TabGroupContext, window: &mut Window, cx: &mut App) -> Stateful<Div> {
        let n = group.node().as_u64();
        GkRenderer::frame(self, H_TAB_FRAME, n, group_snap(group, cx), "tab-group".into(), || group_req(group), window, cx)
    }

    fn content_frame(&self, group: &TabGroupContext, window: &mut Window, cx: &mut App) -> Stateful<Div> {
        let n = group.node().as_u64();
        let sn = group_snap(group, cx);
        GkRenderer::frame(self, H_TAB_CONTENT_FRAME, n, sn, "tab-group-content".into(), || group_req(group), window, cx)
    }

    fn render_tab_bar(&self, group: &TabGroupContext, window: &mut Window, cx: &mut App) -> AnyElement {
        let sn = group_snap(group, cx);
        match self.el(H_TAB_BAR, group.node().as_u64(), sn, None, || group_req(group), window, cx) {
            Ok(el) => el,
            Err(_) => gpui_kit::Empty.into_any_element(),
        }
    }

    fn render_active_panel(&self, panel: AnyView, group: &TabGroupContext, window: &mut Window, cx: &mut App) -> AnyElement {
        let sn = group_snap(group, cx);
        let content = Some(panel.into_any_element());
        match self.el(H_TAB_ACTIVE_PANEL, group.node().as_u64(), sn, content, || group_req(group), window, cx) {
            Ok(el) => el,
            Err(content) => content.unwrap_or_else(|| gpui_kit::Empty.into_any_element()),
        }
    }

    fn render_drop_indicator(&self, indicator: DropIndicator, window: &mut Window, cx: &mut App) -> Option<AnyElement> {

        let el = self.el(H_TAB_DROP_INDICATOR, 0, 1, None, Req::default, window, cx).ok()?;
        let to = indicator.to();
        Some(
            gpui_kit::div()
                .absolute()
                .left(to.origin().x)
                .top(to.origin().y)
                .w(to.size().width)
                .h(to.size().height)
                .child(el)
                .into_any_element(),
        )
    }

    fn render_empty(&self, group: &TabGroupContext, window: &mut Window, cx: &mut App) -> Option<AnyElement> {
        let sn = group_snap(group, cx);
        self.el(H_TAB_EMPTY, group.node().as_u64(), sn, None, || group_req(group), window, cx).ok()
    }
}

/// Handles custom renderer registration, hook answers, dock-toggle subscriptions, and renderer
/// test counters (`C_DOCK_RENDERER_NEW`, `C_DOCK_RENDER_ANSWER`, `C_DOCK_TOGGLE_SUBSCRIBE`,
/// `C_DOCK_RENDER_TEST`).
fn render_call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {

        C_DOCK_RENDERER_NEW => {
            let (hooks, slot, generation) = (r.u32(), r.u32(), r.u32());
            let events = HandlerRef { view: 0, generation, handler: slot };
            let h = crate::objects::obj_new(GkRenderer { id: 0, hooks, events });
            match crate::objects::obj_rc::<GkRenderer>(h) {
                Ok(cell) => cell.borrow_mut().id = h,
                Err(e) => return Some(Err(e)),
            }
            c.out.put_u64(h);
            Ok(())
        }

        C_DOCK_RENDER_ANSWER => (|| {
            let (h, hook, key, view) = (r.u64(), r.u8(), r.u64(), r.u64());
            genapi::obj_clone::<GkRenderer>(h)?;
            if view != 0 {
                mount(view, c)?;
            }
            BOOK.with(|b| b.borrow_mut().answers.insert((h, hook, key), view));
            c.cx.refresh_windows();
            Ok(())
        })(),

        C_DOCK_TOGGLE_SUBSCRIBE => (|| {
            let (h, slot, generation) = (r.u64(), r.u32(), r.u32());
            let sub = genapi::state_subscribe_with::<DockArea, gpui_kit::component::dock::DockToggleEvent>(
                h,
                slot,
                generation,
                SHAPE_DOCK_TOGGLE,
                c,
                |e, out| {
                    out.put_u16(placement_num(e.placement()));
                    out.put_u8(e.is_open() as u8);
                },
            )?;
            c.out.put_u64(sub);
            Ok(())
        })(),

        C_DOCK_RENDER_TEST => {
            let h = r.u64();
            let (sent, answers, drawn) = BOOK.with(|b| {
                let b = b.borrow();
                (
                    b.sent.get(&h).copied().unwrap_or(0),
                    b.answers.keys().filter(|k| k.0 == h).count() as u32,
                    b.drawn.get(&h).copied().unwrap_or(0),
                )
            });
            c.out.put_u32(sent);
            c.out.put_u32(answers);
            c.out.put_u32(drawn);
            Ok(())
        }
        _ => return None,
    })
}
