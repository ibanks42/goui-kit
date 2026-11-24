//! The editor family's runtime (docs/PROTOCOLS.md sections 9.20 to 9.26): what the
//! generated decoders of gen/gk_editor.py call.
//!
//! - **Typed fragments** (9.20): `MainThread` keeps a fragment of the app's view (a `genapi::Frag`,
//!   which holds the view's tape in an `Rc`) inside a closure upstream wants `Send + Sync`
//!   (`MarkdownExtensions::block_surface`); upstream calls it while it draws, on the main thread,
//!   and `get` checks that it is.
//! - **Rows without a window** (9.22): `lazy_row` is the element an element closure over values
//!   returns when upstream gives the closure no window (`FoldPlaceholderRenderer`); it becomes the
//!   row of the app's row source when it is laid out (`source::ctx_row`), where the window is.
//! - **View rows** (9.22): `ctx_view` gives a closure over values that returns an `AnyView`
//!   (`TextView::link_tooltip`) the row view of one value, a row per distinct value.
//! - **Markdown plugins** (9.23): `GkPlugin` is the one `MarkdownPlugin` the library hands
//!   upstream: one of the kit's own plugins (`FrontmatterPlugin`), or `DataPlugin`, which claims the
//!   Markdown nodes of one kind by the app's rule (a fenced code block's language, a JSX element's
//!   name), without calling the app (upstream parses on a background task), and draws each node
//!   as a row of the app's row source: a block as the row, an inline node as the row inside an
//!   `InlineElement`. The row reads the node (and, inline, the `InlineRenderContext`) with
//!   `gk.row_context`.
//! - **Small values** (9.24): `entity_id` (a `gpui::EntityId` argument: the entity behind an input,
//!   textarea or editor handle), `Wire for ElementId` (an id read back: its text).

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

// ---------------------------------------------------------------- 9.20 typed fragments

/// A value only the main thread touches, inside a closure upstream wants `Send + Sync`.
pub struct MainThread<T> {
    v: T,
    thread: std::thread::ThreadId,
}

// SAFETY: `get` is the only access to the value and it refuses any thread but the one that made
// it (the main thread: decoders run there), so the value is never shared across threads.
unsafe impl<T> Send for MainThread<T> {}
unsafe impl<T> Sync for MainThread<T> {}

impl<T> MainThread<T> {
    pub fn new(v: T) -> Self {
        MainThread { v, thread: std::thread::current().id() }
    }

    pub fn get(&self) -> &T {
        if std::thread::current().id() != self.thread {
            eprintln!("gk: a fragment of the app's view was called off the main thread; the library stops");
            std::process::exit(70);
        }
        &self.v
    }
}

/// A typed fragment whose root is not of the type upstream asked for: a mistake in app code (the
/// Go type of the argument prevents it; a tape written by hand could do it). Exit 2.
pub fn bad_fragment(what: &str) -> ! {
    crate::rt::app_stop(format!("a fragment given where upstream wants a {what} is not a {what}"))
}

// ---------------------------------------------------------------- 9.22 rows without a window

/// The row of an element closure over values that upstream calls without a window: an element
/// that becomes the row of `src` (`source::ctx_row`) when it is laid out.
pub fn lazy_row(src: u64, by: Option<u64>, print: Vec<u8>, write: Rc<dyn Fn(&mut Vec<u8>)>) -> AnyElement {
    LazyRow { src, by, row: Some((print, write)) }.into_any_element()
}

struct LazyRow {
    src: u64,
    by: Option<u64>,
    row: Option<(Vec<u8>, Rc<dyn Fn(&mut Vec<u8>)>)>,
}

// gpui-fast (0.1.16 on) has `impl<T: Element> IntoElement for T`; upstream GPUI does not.
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

// ---------------------------------------------------------------- 9.22 view rows

thread_local! {
    /// Per row source, the row of each distinct value a view closure was called with, and the view
    /// handed upstream for it.
    static VIEW_ROWS: RefCell<HashMap<u64, HashMap<Vec<u8>, (u32, AnyView)>>> = RefCell::new(HashMap::new());
}

/// The view handed upstream for a row of a view closure: the row's own view once it is described
/// (one frame of nothing before), so a view upstream keeps (a link's tooltip, cached per URL) shows
/// the row when it arrives: it watches the source, which notifies it then.
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

/// The view of the row of `src` for these values (`print`: their bytes): the same row and view each
/// time the same values come, a new row for new ones.
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
    // Keeps the row's values (gk.row_context) and binds the row; its element is not used here.
    let _ = source::ctx_row(src, Some(row as u64), print, write, window, cx);
    view
}

// ---------------------------------------------------------------- 9.23 Markdown plugins

/// The Markdown nodes a data plugin claims (Go `gk.MarkdownMatch`, in this order).
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

/// The library's data plugin (Go `gk.MarkdownPlugin`): its name, the nodes it claims (a kind
/// and, for a fenced code block, its language, for a JSX element, its name; empty: every one),
/// the row source that draws its nodes, and an inline node's baseline (0: upstream's).
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
    /// The node's text and its language or name when it is of the claimed kind.
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

    fn parse(&self, node: &markdown_ast::Node, cx: &MarkdownParseContext<'_>) -> Option<MarkdownNode> {
        let (value, _) = self.claims(node)?;
        let source = cx.node_source(node).unwrap_or("").to_string();
        let text = if value.is_empty() { source.clone() } else { value };
        PARSED.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        Some(MarkdownNode::new(SharedString::from(self.name.clone()), ()).text(text).markdown(source))
    }

    /// The row of a node: its identity is its name, text and Markdown, so a node that changes is
    /// described again.
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

/// Nodes the data plugins claimed (tests); upstream parses on a background task.
static PARSED: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);

/// The one plugin the library gives upstream: a kit plugin or the library's data plugin.
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

// ---------------------------------------------------------------- 9.26 one-method data delegates

/// The editor's highlight resolver over the app's procedure (an ask, PROTOCOLS.md 9.14): a name
/// asked between frames, unstyled until the answer is in, then cached.
pub struct GkStyles {
    ask: crate::shim_text::Ask,
    dec: fn(&mut R, &mut crate::genapi::Dx) -> Option<Option<gpui_kit::HighlightStyle>>,
}

impl gpui_kit::base::input::HighlightStyleResolver for GkStyles {
    fn style(&self, name: &str) -> Option<gpui_kit::HighlightStyle> {
        self.ask.get(crate::shim_text::key(&name), self.dec, |v| v, || None)
    }
}

/// `SharedHighlightStyleResolver` from the ask the app passes (its row source handle).
pub fn style_resolver(
    src: u64,
    dec: fn(&mut R, &mut crate::genapi::Dx) -> Option<Option<gpui_kit::HighlightStyle>>,
) -> std::sync::Arc<dyn gpui_kit::base::input::HighlightStyleResolver> {
    std::sync::Arc::new(GkStyles { ask: crate::shim_text::Ask::take(src), dec })
}

// ---------------------------------------------------------------- 9.24 small values

/// An ElementId read back: its text (`DiffAnnotation::id`).
impl Wire for ElementId {
    fn put(&self, out: &mut Vec<u8>) {
        Wire::put(&self.to_string(), out);
    }
}

/// The entity behind an input, textarea or editor handle, as a `gpui::EntityId` argument
/// (`InputBase::semantic_owner`). Any other handle stops the program (exit 2), as a released
/// handle a tape names does.
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
    // A handle of another type says "is a X, not a Y"; a released or unknown one says why itself.
    let why = if first.contains(", not a ") {
        format!("handle {handle:#x} is not an input, a textarea or an editor (gk.EntityIDOf takes one of those)")
    } else {
        first
    };
    genapi::released_shown(&why, "kit state (as an entity id)")
}

/// Tests (call `EDITOR_TEST`): the nodes the data plugins claimed, the view rows made.
pub fn test_counts() -> (u64, u64) {
    let parsed = PARSED.load(std::sync::atomic::Ordering::Relaxed);
    let rows = VIEW_ROWS.with(|m| m.borrow().values().map(|r| r.len() as u64).sum());
    (parsed, rows)
}

/// The editor family's call ops (asked from shim_editor::call, so the shared dispatch needs no line).
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

// ---------------------------------------------------------------- 9.30 the editor host

/// The editor host (node `EDITOR_HOST`): renders the app's blocks as
/// TextView does (`BlockNode::render` with the block's `FlowPosition`, or its place among the
/// blocks, and the `RenderContext`, or the one TextView would start from), runs upstream's
/// `collect_leaf_geometry` around their prepaint with the app's decorations, and hands the keyed
/// leaves to the app after the frame: an event of shape `LEAF_GEOMETRY` (u32 n, (u64 key, u64
/// temporary handle of a `LeafGeometry`) each), only when the leaves' keys or geometry changed since
/// the handler's last event, so describing the view again does not keep the app drawing.
pub struct BlockHost {
    id: ElementId,
    blocks: Vec<gpui_kit::base::text::editor_host::BlockNode>,
    flows: Vec<Option<gpui_kit::base::text::editor_host::FlowPosition>>,
    context: Option<gpui_kit::base::text::editor_host::RenderContext>,
    decorations: HashMap<u64, Vec<(std::ops::Range<usize>, gpui_kit::Hsla)>>,
    handler: Option<genapi::HandlerRef>,
    _keep: genapi::TapePin,
}

/// Node `EDITOR_HOST`: str id; u32 n, block each; u32 n, (u8 has, u32 FlowPosition value node)
/// each; u8 has, u32 RenderContext value node; u32 n, (u64 key, ranges) each; u8 has, u32 handler.
/// The decoders of the values are generated (`generated::editor_host_*`, gen/gk_editor.py).
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

// gpui-fast (0.1.16 on) has `impl<T: Element> IntoElement for T`; upstream GPUI does not.
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
    /// The last leaves' fingerprint delivered per (view, handler id).
    static HOST_LAST: RefCell<HashMap<(u64, u32), Vec<u8>>> = RefCell::new(HashMap::new());
    /// Leaves waiting for the task that delivers them, per (view, handler id): the newest wins.
    static HOST_PENDING: RefCell<Vec<(genapi::HandlerRef, Vec<(u64, gpui_kit::base::text::editor_host::LeafGeometry)>)>> =
        const { RefCell::new(Vec::new()) };
    static HOST_SCHEDULED: std::cell::Cell<bool> = const { std::cell::Cell::new(false) };
    static HOSTS: std::cell::Cell<u64> = const { std::cell::Cell::new(0) };
    static LEAVES: std::cell::Cell<u64> = const { std::cell::Cell::new(0) };
    static DELIVERED: std::cell::Cell<u64> = const { std::cell::Cell::new(0) };
}

/// What decides whether the app hears of the leaves again: their keys and what the app reads
/// first (bounds, length, line and caret heights).
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

/// After the frame: each handler's leaves as temporary handles (13.2), one EVENT turn per handler
/// with its view's window.
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

/// Tests (call `EDITOR_HOST_TEST`): host elements decoded, leaves collected, events delivered.
pub fn host_counts() -> (u64, u64, u64) {
    (HOSTS.with(|h| h.get()), LEAVES.with(|l| l.get()), DELIVERED.with(|d| d.get()))
}

// ---------------------------------------------------------------- 9.32 decorators kept for later

/// A decorator upstream keeps and calls while it renders, without a window
/// (`NumberInput::increment_button(FnOnce(Button) -> Button)`): the app's style, decoded where the
/// closure was decoded, refines the element's own, and the app's content is a row of `src` (0:
/// none), laid out where the window is (`lazy_row`, as the fold placeholder's).
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

// ---------------------------------------------------------------- 9.33 edits of a value lent by &mut

/// An edit of a value kit state lends by `&mut` (`EditorState::lsp_mut -> &mut Lsp`): the app's
/// value of the type, a tape of its own (13.6), whose builder ops (not its constructor) are applied
/// to the state's own value in place, with the window and app of the state's update.
pub struct LentEdit<V> {
    tape: Rc<crate::tape::Tape>,
    apply: fn(&mut genapi::Dx, V, u32) -> V,
}

impl<V: Default> LentEdit<V> {
    pub fn read(r: &mut R, apply: fn(&mut genapi::Dx, V, u32) -> V) -> genapi::GkResult<Self> {
        let n = r.u32() as usize;
        let tape = Rc::new(crate::tape::Tape::parse(r.bytes(n).to_vec(), 0)?);
        Ok(LentEdit { tape, apply })
    }

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

// ---------------------------------------------------------------- 9.37 actions read back

/// A boxed action read back (`NativeMenuItem::Action`'s action): its id as the app knows actions
/// (`app::id_of`: an app action's own id, a kit action's id the first time it is seen).
impl Wire for Box<dyn gpui_kit::Action> {
    fn put(&self, out: &mut Vec<u8>) {
        Wire::put(&**self, out);
    }
}
