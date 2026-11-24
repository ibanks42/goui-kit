//! Runtime support for rich-text, markdown plugins, highlight style resolvers, and the block editor host.
//!
//! # Components and Protocols
//!
//! - **Thread-affinity bridge ([`MainThread`])**: upstream markdown plugin and highlight traits
//!   require `Send + Sync`, while tape fragments (`Frag`) hold `!Send` `Rc<Tape>`. `MainThread<T>`
//!   implements `Send + Sync` and verifies at runtime that `get()` is only ever called on the
//!   creating UI thread (terminating with exit code 70 otherwise).
//! - **Deferred and stable row adapters ([`LazyRow`], [`ctx_view`])**: `LazyRow` defers
//!   `source::ctx_row` until `Element::request_layout` when upstream constructs elements without
//!   `&mut Window`. `ctx_view` caches `(row_index, AnyView)` per `(source, context_fingerprint)`
//!   in `VIEW_ROWS` so upstream APIs expecting a persistent `AnyView` (like markdown link tooltips)
//!   keep stable GPUI view identities across frames.
//! - **Markdown plugins ([`DataPlugin`], [`GkPlugin`])**: matches one `markdown_ast::Node` kind
//!   plus optional language/JSX tag filter and renders block or inline nodes through a host row
//!   source (`source::ctx_row`).
//! - **Block editor host (`N_EDITOR_HOST`, [`BlockHost`])**: lays out decoded `BlockNode`s with
//!   optional `FlowPosition`s and `RenderContext`, runs `editor_host::collect_leaf_geometry`
//!   around `prepaint` with host decoration ranges, fingerprints the sorted leaf geometries in
//!   `HOST_LAST`, and coalesces changed batches in `HOST_PENDING` for async delivery under
//!   `SHAPE_LEAF_GEOMETRY` (each `LeafGeometry` encoded as a temporary `HK_OBJ` handle).
//! - **In-place lent edits ([`LentEdit`])**: lets the host mutate a borrowed upstream value
//!   (`&mut V` where `V: Default`) via a mini-tape by moving the value out with `mem::take`,
//!   replaying generated builder ops, and writing the updated value back.

use crate::genapi::{self, Unwire, Wire, R};
use crate::source;
use gpui_kit::base::{InlineElement, InlineRenderContext, MarkdownNode, MarkdownParseContext, MarkdownPlugin, markdown_ast};
use gpui_kit::{
    AnyElement, AnyView, App, AppContext as _, Bounds, Element, ElementId, GlobalElementId, InspectorElementId,
    IntoElement, LayoutId, Pixels, Render, SharedString, Window,
};
use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

/// Wrapper asserting at runtime that a `!Send` value is only ever dereferenced on the thread that
/// created it (required by upstream plugin traits bounded by `Send + Sync`).
pub struct MainThread<T> {
    v: T,
    thread: std::thread::ThreadId,
}

// SAFETY: `MainThread<T>` exposes `&T` exclusively through `get()`, which verifies
// `std::thread::current().id() == self.thread` and terminates the process otherwise.
// Thus `T` is never accessed concurrently or from any thread other than its creator.
unsafe impl<T> Send for MainThread<T> {}
// SAFETY: Same invariant as `Send`: `get()` restricts all shared access to `self.thread`.
unsafe impl<T> Sync for MainThread<T> {}

impl<T> MainThread<T> {
    pub fn new(v: T) -> Self {
        MainThread { v, thread: std::thread::current().id() }
    }

    /// Returns a reference to the inner value, exiting the process if called off the creating thread.
    pub fn get(&self) -> &T {
        if std::thread::current().id() != self.thread {
            eprintln!("gk: a fragment of the app's view was called off the main thread; the library stops");
            std::process::exit(70);
        }
        &self.v
    }
}

/// Stops the app when a tape fragment fails to decode as the expected upstream type `what`.
pub fn bad_fragment(what: &str) -> ! {
    crate::rt::app_stop(format!("a fragment given where upstream wants a {what} is not a {what}"))
}

/// Returns an element that defers [`source::ctx_row`] resolution until `request_layout` runs
/// (needed when upstream builds elements outside a `Window` / `App` render context).
pub fn lazy_row(src: u64, by: Option<u64>, print: Vec<u8>, write: Rc<dyn Fn(&mut Vec<u8>)>) -> AnyElement {
    LazyRow { src, by, row: Some((print, write)) }.into_any_element()
}

struct LazyRow {
    src: u64,
    by: Option<u64>,
    row: Option<(Vec<u8>, Rc<dyn Fn(&mut Vec<u8>)>)>,
}

#[cfg(not(feature = "gpui-fast"))]
impl IntoElement for LazyRow {
    type Element = Self;

    fn into_element(self) -> Self {
        self
    }
}

impl Element for LazyRow {
    type RequestLayoutState = AnyElement;
    type PrepaintState = ();

    fn id(&self) -> Option<ElementId> {
        None
    }

    fn source_location(&self) -> Option<&'static std::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (LayoutId, AnyElement) {
        let (print, write) = self.row.take().unwrap_or_else(|| (Vec::new(), Rc::new(|_: &mut Vec<u8>| {})));
        let mut el = source::ctx_row(self.src, self.by, print, write, window, cx);
        let id = el.request_layout(window, cx);
        (id, el)
    }

    fn prepaint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        _bounds: Bounds<Pixels>,
        el: &mut AnyElement,
        window: &mut Window,
        cx: &mut App,
    ) {
        el.prepaint(window, cx);
    }

    fn paint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        _bounds: Bounds<Pixels>,
        el: &mut AnyElement,
        _prepaint: &mut (),
        window: &mut Window,
        cx: &mut App,
    ) {
        el.paint(window, cx);
    }
}

thread_local! {
    /// Caches `(row_index, AnyView)` per `(source_handle, context_fingerprint)` so upstream APIs
    /// returning `AnyView` keep stable GPUI view identities across frames.
    static VIEW_ROWS: RefCell<HashMap<u64, HashMap<Vec<u8>, (u32, AnyView)>>> = RefCell::new(HashMap::new());
}

struct RowView {
    src: u64,
    row: u32,
}

impl Render for RowView {
    fn render(&mut self, window: &mut Window, cx: &mut gpui_kit::Context<Self>) -> impl IntoElement {
        source::watch(self.src, cx.entity_id());
        match source::row(self.src, source::key(0, self.row), window, cx) {
            source::Row::View(v) => v.into_any_element(),
            source::Row::Missing => gpui_kit::div().into_any_element(),
        }
    }
}

/// Returns a stable `AnyView` backed by a row in `src` keyed by fingerprint `print`.
pub fn ctx_view(src: u64, print: Vec<u8>, write: Rc<dyn Fn(&mut Vec<u8>)>, window: &mut Window, cx: &mut App) -> AnyView {
    let known = VIEW_ROWS.with(|m| m.borrow().get(&src).and_then(|rows| rows.get(&print).cloned()));
    let (row, view) = match known {
        Some(x) => x,
        None => {
            let row = VIEW_ROWS.with(|m| m.borrow().get(&src).map_or(0, |rows| rows.len() as u32));
            let view: AnyView = cx.new(|_| RowView { src, row }).into();
            VIEW_ROWS.with(|m| m.borrow_mut().entry(src).or_default().insert(print.clone(), (row, view.clone())));
            (row, view)
        }
    };
    let _ = source::ctx_row(src, Some(row as u64), print, write, window, cx);
    view
}

/// Markdown AST node classes that a host-defined [`DataPlugin`] can intercept.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Match {
    Code,
    Math,
    Yaml,
    Toml,
    Html,
    MdxElement,
    InlineCode,
    InlineMath,
    InlineHtml,
    MdxInlineElement,
}

impl Match {
    fn from_u8(v: u8) -> Match {
        use Match::*;
        [Code, Math, Yaml, Toml, Html, MdxElement, InlineCode, InlineMath, InlineHtml, MdxInlineElement]
            .get(v as usize)
            .copied()
            .unwrap_or(Code)
    }

    fn is_block(self) -> bool {
        matches!(self, Match::Code | Match::Math | Match::Yaml | Match::Toml | Match::Html | Match::MdxElement)
    }
}

/// Data-driven `MarkdownPlugin` that matches one AST node kind (and optional language/tag name)
/// and delegates rendering to a host row source `src`.
#[derive(Clone, Debug)]
pub struct DataPlugin {
    name: String,
    kind: Match,
    lang: String,
    src: u64,
    baseline: f32,
}

impl Unwire for DataPlugin {
    fn get(r: &mut R) -> Self {
        let name = r.str().to_string();
        let kind = Match::from_u8(r.u8());
        let lang = r.str().to_string();
        let src = r.u64();
        let baseline = r.f32();
        DataPlugin { name, kind, lang, src, baseline }
    }
}

impl DataPlugin {
    /// Checks whether `node` matches this plugin's AST kind and optional `lang`/JSX tag filter,
    /// returning `(text_value, optional_lang_or_tag)`.
    fn claims(&self, node: &markdown_ast::Node) -> Option<(String, Option<String>)> {
        use markdown_ast::Node;
        let (value, name) = match (self.kind, node) {
            (Match::Code, Node::Code(c)) => (c.value.clone(), c.lang.clone()),
            (Match::Math, Node::Math(c)) => (c.value.clone(), None),
            (Match::Yaml, Node::Yaml(c)) => (c.value.clone(), None),
            (Match::Toml, Node::Toml(c)) => (c.value.clone(), None),
            (Match::Html | Match::InlineHtml, Node::Html(c)) => (c.value.clone(), None),
            (Match::InlineCode, Node::InlineCode(c)) => (c.value.clone(), None),
            (Match::InlineMath, Node::InlineMath(c)) => (c.value.clone(), None),
            (Match::MdxElement, Node::MdxJsxFlowElement(e)) => (String::new(), e.name.clone()),
            (Match::MdxInlineElement, Node::MdxJsxTextElement(e)) => (String::new(), e.name.clone()),
            _ => return None,
        };
        if !self.lang.is_empty() && name.as_deref() != Some(self.lang.as_str()) {
            return None;
        }
        Some((value, name))
    }

    /// Converts a claimed markdown AST node into a `MarkdownNode` carrying extracted text and raw source.
    fn parse(&self, node: &markdown_ast::Node, cx: &MarkdownParseContext<'_>) -> Option<MarkdownNode> {
        let (value, _) = self.claims(node)?;
        let source = cx.node_source(node).unwrap_or("").to_string();
        let text = if value.is_empty() { source.clone() } else { value };
        PARSED.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        Some(MarkdownNode::new(SharedString::from(self.name.clone()), ()).text(text).markdown(source))
    }

    /// Encodes the node (and optional inline font/line metrics) as a row fingerprint and context
    /// writer for [`source::ctx_row`].
    fn row(&self, node: &MarkdownNode, context: Option<&InlineRenderContext>, window: &mut Window, cx: &mut App) -> AnyElement {
        let mut p: Vec<u8> = Vec::new();
        Wire::put(node.name(), &mut p);
        Wire::put(node.as_text(), &mut p);
        Wire::put(node.as_markdown(), &mut p);
        if let Some(c) = context {
            Wire::put(&f32::from(c.font_size()), &mut p);
            Wire::put(&f32::from(c.line_height()), &mut p);
        }
        let n = node.clone();
        let c = context.cloned();
        let write: Rc<dyn Fn(&mut Vec<u8>)> = Rc::new(move |o: &mut Vec<u8>| {
            Wire::put(&n, o);
            if let Some(c) = &c {
                Wire::put(c, o);
            }
        });
        source::ctx_row(self.src, None, p, write, window, cx)
    }
}

static PARSED: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);

/// Polymorphic `MarkdownPlugin` combining built-in `FrontmatterPlugin` and host [`DataPlugin`].
pub enum GkPlugin {
    Frontmatter(gpui_kit::component::text::FrontmatterPlugin),
    Data(DataPlugin),
}

impl MarkdownPlugin for GkPlugin {
    fn is_block(&self) -> bool {
        match self {
            GkPlugin::Frontmatter(p) => p.is_block(),
            GkPlugin::Data(p) => p.kind.is_block(),
        }
    }

    fn name(&self) -> &str {
        match self {
            GkPlugin::Frontmatter(p) => p.name(),
            GkPlugin::Data(p) => &p.name,
        }
    }

    fn parse(&self, node: &markdown_ast::Node, cx: &MarkdownParseContext<'_>) -> Option<MarkdownNode> {
        match self {
            GkPlugin::Frontmatter(p) => p.parse(node, cx),
            GkPlugin::Data(p) => p.parse(node, cx),
        }
    }

    fn render(&self, node: &MarkdownNode, window: &mut Window, cx: &mut App) -> impl IntoElement {
        match self {
            GkPlugin::Frontmatter(p) => p.render(node, window, cx).into_any_element(),
            GkPlugin::Data(p) => p.row(node, None, window, cx),
        }
    }

    fn render_inline(
        &self,
        node: &MarkdownNode,
        context: &InlineRenderContext,
        window: &mut Window,
        cx: &mut App,
    ) -> Option<InlineElement> {
        match self {
            GkPlugin::Frontmatter(p) => p.render_inline(node, context, window, cx),
            GkPlugin::Data(p) => {
                let el = InlineElement::new(p.row(node, Some(context), window, cx));
                Some(if p.baseline > 0.0 { el.with_baseline(gpui_kit::px(p.baseline)) } else { el })
            }
        }
    }
}

/// `HighlightStyleResolver` backed by a host request-reply [`crate::shim_text::Ask`] source.
pub struct GkStyles {
    ask: crate::shim_text::Ask,
    dec: fn(&mut R, &mut crate::genapi::Dx) -> Option<Option<gpui_kit::HighlightStyle>>,
}

impl gpui_kit::base::input::HighlightStyleResolver for GkStyles {
    fn style(&self, name: &str) -> Option<gpui_kit::HighlightStyle> {
        self.ask.get(crate::shim_text::key(&name), self.dec, |v| v, || None)
    }
}

/// Creates an `Arc<dyn HighlightStyleResolver>` querying row source `src`.
pub fn style_resolver(
    src: u64,
    dec: fn(&mut R, &mut crate::genapi::Dx) -> Option<Option<gpui_kit::HighlightStyle>>,
) -> std::sync::Arc<dyn gpui_kit::base::input::HighlightStyleResolver> {
    std::sync::Arc::new(GkStyles { ask: crate::shim_text::Ask::take(src), dec })
}

impl Wire for ElementId {
    fn put(&self, out: &mut Vec<u8>) {
        Wire::put(&self.to_string(), out);
    }
}

/// Resolves an `InputState`, `EditorState`, or `TextareaState` handle to its GPUI `EntityId`.
pub fn entity_id(handle: u64) -> gpui_kit::EntityId {
    use gpui_kit::base::input::{EditorState, InputState, TextareaState};
    let first = match genapi::state::<InputState>(handle) {
        Ok(e) => return e.entity_id(),
        Err(genapi::GkErr(_, msg)) => msg,
    };
    if let Ok(e) = genapi::state::<EditorState>(handle) {
        return e.entity_id();
    }
    if let Ok(e) = genapi::state::<TextareaState>(handle) {
        return e.entity_id();
    }
    let why = if first.contains(", not a ") {
        format!("handle {handle:#x} is not an input, a textarea or an editor (gk.EntityIDOf takes one of those)")
    } else {
        first
    };
    genapi::released_shown(&why, "kit state (as an entity id)")
}

/// Returns diagnostic counts `(markdown_nodes_parsed, cached_ctx_views)`.
pub fn test_counts() -> (u64, u64) {
    let parsed = PARSED.load(std::sync::atomic::Ordering::Relaxed);
    let rows = VIEW_ROWS.with(|m| m.borrow().values().map(|r| r.len() as u64).sum());
    (parsed, rows)
}

/// Handles `C_EDITOR_TEST` and `C_EDITOR_HOST_TEST` diagnostic counter queries.
pub fn call(op: u16, _r: &mut R, c: &mut genapi::Call) -> Option<genapi::GkResult<()>> {
    use crate::genapi::Put as _;
    Some(match op {
        crate::ops::C_EDITOR_TEST => {
            let (parsed, rows) = test_counts();
            c.out.put_u64(parsed);
            c.out.put_u64(rows);
            Ok(())
        }
        crate::ops::C_EDITOR_HOST_TEST => {
            let (hosts, leaves, delivered) = host_counts();
            c.out.put_u64(hosts);
            c.out.put_u64(leaves);
            c.out.put_u64(delivered);
            c.out.put_u64(DECORATED.with(|d| d.get()));
            c.out.put_u64(LENT_EDITS.with(|n| n.get()));
            Ok(())
        }
        _ => return None,
    })
}

/// Element implementing `N_EDITOR_HOST`: lays out `BlockNode` children, collects leaf geometry
/// around prepaint, and asynchronously sends changed `LeafGeometry` handles to the host.
pub struct BlockHost {
    id: ElementId,
    blocks: Vec<gpui_kit::base::text::editor_host::BlockNode>,
    flows: Vec<Option<gpui_kit::base::text::editor_host::FlowPosition>>,
    context: Option<gpui_kit::base::text::editor_host::RenderContext>,
    decorations: HashMap<u64, Vec<(std::ops::Range<usize>, gpui_kit::Hsla)>>,
    handler: Option<genapi::HandlerRef>,
    _keep: genapi::TapePin,
}

/// Decodes an `N_EDITOR_HOST` node (id, blocks, optional flow positions, render context,
/// decoration ranges, and optional leaf-geometry callback).
pub fn host_node(dx: &mut genapi::Dx, args: u32, _first: u32) -> Option<AnyElement> {
    use crate::generated as g;
    let r = &mut dx.args(args);
    let id = ElementId::Name(SharedString::from(r.str().to_string()));
    let n = r.u32();
    let mut blocks = Vec::with_capacity(n.min(4096) as usize);
    for _ in 0..n {
        blocks.push(g::editor_host_block(r, dx)?);
    }
    let n = r.u32();
    let mut flows = Vec::with_capacity(n.min(4096) as usize);
    for _ in 0..n {
        flows.push(if r.u8() != 0 { Some(g::editor_host_flow(r, dx)?) } else { None });
    }
    let context = if r.u8() != 0 { Some(g::editor_host_context(r, dx)?) } else { None };
    let n = r.u32();
    let mut decorations = HashMap::new();
    for _ in 0..n {
        let key = r.u64();
        let ranges = g::editor_host_ranges(r, dx)?;
        decorations.entry(key).or_insert_with(Vec::new).extend(ranges);
    }
    let handler = (r.u8() != 0).then(|| dx.handler(r));
    HOSTS.with(|h| h.set(h.get() + 1));
    Some(BlockHost { id, blocks, flows, context, decorations, handler, _keep: dx.pin() }.into_any_element())
}

#[cfg(not(feature = "gpui-fast"))]
impl IntoElement for BlockHost {
    type Element = Self;

    fn into_element(self) -> Self {
        self
    }
}

impl Element for BlockHost {
    type RequestLayoutState = AnyElement;
    type PrepaintState = ();

    fn id(&self) -> Option<ElementId> {
        None
    }

    fn source_location(&self) -> Option<&'static std::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (LayoutId, AnyElement) {
        use gpui_kit::base::text::editor_host::{FlowPosition, RenderContext};
        use gpui_kit::{InteractiveElement as _, ParentElement as _};
        let rc = self.context.take().unwrap_or_else(|| RenderContext::from_app(cx));
        let refs: Vec<&gpui_kit::base::text::editor_host::BlockNode> = self.blocks.iter().collect();
        let n = refs.len();
        let mut children = Vec::with_capacity(n);
        for ix in 0..n {
            let flow = self.flows.get(ix).copied().flatten().unwrap_or_else(|| FlowPosition::at(&refs, ix, ix + 1 == n));
            children.push(refs[ix].render(ix, flow, &rc, window, cx));
        }
        let mut el = gpui_kit::div().id(self.id.clone()).children(children).into_any_element();
        let layout = el.request_layout(window, cx);
        (layout, el)
    }

    fn prepaint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        _bounds: Bounds<Pixels>,
        el: &mut AnyElement,
        window: &mut Window,
        cx: &mut App,
    ) {
        let decorations = std::mem::take(&mut self.decorations);
        let (_, leaves) =
            gpui_kit::base::text::editor_host::collect_leaf_geometry(decorations, || el.prepaint(window, cx));
        LEAVES.with(|l| l.set(l.get() + leaves.len() as u64));
        if let Some(h) = self.handler.take() {
            leaves_after(h, leaves, cx);
        }
    }

    fn paint(
        &mut self,
        _id: Option<&GlobalElementId>,
        _inspector_id: Option<&InspectorElementId>,
        _bounds: Bounds<Pixels>,
        el: &mut AnyElement,
        _prepaint: &mut (),
        window: &mut Window,
        cx: &mut App,
    ) {
        el.paint(window, cx);
    }
}

thread_local! {
    /// Last leaf-geometry fingerprint sent per `(view, handler)` to suppress duplicate events.
    static HOST_LAST: RefCell<HashMap<(u64, u32), Vec<u8>>> = RefCell::new(HashMap::new());
    /// Coalesced leaf-geometry batches queued during prepaint for delivery after the frame.
    static HOST_PENDING: RefCell<Vec<(genapi::HandlerRef, Vec<(u64, gpui_kit::base::text::editor_host::LeafGeometry)>)>> =
        const { RefCell::new(Vec::new()) };
    static HOST_SCHEDULED: std::cell::Cell<bool> = const { std::cell::Cell::new(false) };
    static HOSTS: std::cell::Cell<u64> = const { std::cell::Cell::new(0) };
    static LEAVES: std::cell::Cell<u64> = const { std::cell::Cell::new(0) };
    static DELIVERED: std::cell::Cell<u64> = const { std::cell::Cell::new(0) };
}

/// Packs each leaf's key, bounds, line/caret height, and character length into a compact
/// comparison key so unchanged layout passes do not trigger redundant `SHAPE_LEAF_GEOMETRY` events.
fn fingerprint(leaves: &[(u64, gpui_kit::base::text::editor_host::LeafGeometry)]) -> Vec<u8> {
    let mut b = Vec::with_capacity(leaves.len() * 36);
    for (key, g) in leaves {
        let r = g.bounds();
        b.extend_from_slice(&key.to_le_bytes());
        for v in [r.origin.x, r.origin.y, r.size.width, r.size.height, g.line_height(), g.caret_height()] {
            b.extend_from_slice(&f32::from(v).to_le_bytes());
        }
        b.extend_from_slice(&(g.len() as u64).to_le_bytes());
    }
    b
}

/// Coalesces changed leaf geometry batches per `(view, handler)` during prepaint and schedules a
/// single async [`deliver_leaves`] pass after layout finishes.
fn leaves_after(
    h: genapi::HandlerRef,
    leaves: HashMap<u64, gpui_kit::base::text::editor_host::LeafGeometry>,
    cx: &mut App,
) {
    let mut leaves: Vec<_> = leaves.into_iter().collect();
    leaves.sort_by_key(|(k, _)| *k);
    let print = fingerprint(&leaves);
    let fresh = HOST_LAST.with(|l| {
        let mut l = l.borrow_mut();
        if l.get(&(h.view, h.handler)) == Some(&print) {
            return false;
        }
        l.insert((h.view, h.handler), print);
        true
    });
    if !fresh {
        return;
    }
    HOST_PENDING.with(|p| {
        let mut p = p.borrow_mut();
        p.retain(|(o, _)| (o.view, o.handler) != (h.view, h.handler));
        p.push((h, leaves));
    });
    if HOST_SCHEDULED.with(|s| s.replace(true)) {
        return;
    }
    cx.spawn(async move |cx: &mut gpui_kit::AsyncApp| {
        let _ = cx.update(|cx| deliver_leaves(cx));
    })
    .detach();
}

/// Encodes each pending leaf-geometry batch as temporary `HK_OBJ` handles under
/// `SHAPE_LEAF_GEOMETRY` and queues them via `rt::queue_event`.
fn deliver_leaves(cx: &mut App) {
    use crate::genapi::Put as _;
    HOST_SCHEDULED.with(|s| s.set(false));
    let pending = HOST_PENDING.with(|p| std::mem::take(&mut *p.borrow_mut()));
    for (h, leaves) in pending {
        let mut req = Vec::with_capacity(24 + leaves.len() * 16);
        req.put_u64(h.view);
        req.put_u32(h.generation);
        req.put_u32(h.handler);
        req.put_u16(crate::ops::SHAPE_LEAF_GEOMETRY);
        req.put_u32(leaves.len() as u32);
        for (key, g) in leaves {
            req.put_u64(key);
            Wire::put(&genapi::Owned::new(g), &mut req);
        }
        DELIVERED.with(|d| d.set(d.get() + 1));
        crate::rt::queue_event(req, h.view, cx);
    }
    HOST_LAST.with(|l| {
        let mut l = l.borrow_mut();
        if l.len() > 256 {
            l.retain(|(view, _), _| crate::rt::with(|s| s.view(*view).is_ok()));
        }
    });
}

/// Returns diagnostic counters `(block_hosts_built, leaf_geometries_collected, batches_delivered)`.
pub fn host_counts() -> (u64, u64, u64) {
    (HOSTS.with(|h| h.get()), LEAVES.with(|l| l.get()), DELIVERED.with(|d| d.get()))
}

/// Refines `el`'s style and optionally appends a lazy row from `src` (used by editor decorator hooks).
pub fn decorate<E: gpui_kit::Styled + gpui_kit::ParentElement>(
    mut el: E,
    style: Option<gpui_kit::StyleRefinement>,
    src: u64,
) -> E {
    use gpui_kit::Refineable as _;
    if let Some(s) = &style {
        el.style().refine(s);
    }
    if src != 0 {
        el.extend(std::iter::once(lazy_row(src, None, Vec::new(), Rc::new(|_: &mut Vec<u8>| {}))));
    }
    DECORATED.with(|d| d.set(d.get() + 1));
    el
}

thread_local! {
    static DECORATED: std::cell::Cell<u64> = const { std::cell::Cell::new(0) };
}

/// Mini-tape edit that temporarily takes ownership of a `&mut V` (via `Default::default`),
/// applies builder ops from the tape, and writes the updated value back.
pub struct LentEdit<V> {
    tape: Rc<crate::tape::Tape>,
    apply: fn(&mut genapi::Dx, V, u32) -> V,
}

impl<V: Default> LentEdit<V> {
    /// Parses a length-prefixed mini-tape from `r` and pairs it with builder-op applier `apply`.
    pub fn read(r: &mut R, apply: fn(&mut genapi::Dx, V, u32) -> V) -> genapi::GkResult<Self> {
        let n = r.u32() as usize;
        let tape = Rc::new(crate::tape::Tape::parse(r.bytes(n).to_vec(), 0)?);
        Ok(LentEdit { tape, apply })
    }

    /// Replays the mini-tape's ops onto `lent` in place and increments `LENT_EDITS`.
    pub fn apply(self, lent: &mut V, window: &mut Window, cx: &mut App) {
        let taken = std::mem::take(lent);
        let apply = self.apply;
        *lent = genapi::with_tape(&self.tape, 0, window, cx, |dx, root| {
            let first = dx.node(root).2;
            apply(dx, taken, first)
        });
        LENT_EDITS.with(|n| n.set(n.get() + 1));
    }
}

thread_local! {
    static LENT_EDITS: std::cell::Cell<u64> = const { std::cell::Cell::new(0) };
}

impl Wire for Box<dyn gpui_kit::Action> {
    fn put(&self, out: &mut Vec<u8>) {
        Wire::put(&**self, out);
    }
}
