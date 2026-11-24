//! Binary view-tree tape parser, table-cell pre-decoder, and fast-path tree cache.
//!
//! # Tape Binary Format (`"GKT1"`)
//!
//! Each described view returns a self-contained little-endian byte buffer laid out in five
//! contiguous regions so [`Tape::parse`] can validate every bound up front and decode nodes
//! with O(1) random access:
//!
//! 1. **Fixed header (16 bytes)**: `magic: u32` (`"GKT1"`), `node_count: u32`, `root: u32`
//!    (root node index in `0..node_count`), and `handlers: u32` (number of event handler slots
//!    referenced by this tape; nonzero values require a [`GenPin`]).
//! 2. **State dependencies (`2 + 8 * n_deps` bytes)**: `n_deps: u16` followed by `HK_STATE`
//!    handles read while describing the view; `rt::accept_one` attaches GPUI observers to each.
//! 3. **Child view slots (`2 + 8 * n_slots` bytes)**: `n_slots: u16` followed by child `HK_VIEW`
//!    handles embedded in this view; `rt::accept_one` mounts each into the owning window and
//!    promotes any pending `unmounted_dirty` entries.
//! 4. **Ops arena (`4 + ops_len` bytes)**: `ops_len: u32` followed by raw bytes containing both
//!    node constructor arguments (`args` offsets) and singly linked builder-op lists
//!    (`[op: u16][next_offset: u32][payload...]`, terminated by `next_offset == 0`). Offset `0`
//!    is reserved as the null/end sentinel, so every valid op sits at offset `>= 1`.
//! 5. **Node table (`10 * node_count` bytes)**: packed 10-byte records `(ty: u16, args: u32,
//!    first_op: u32)` indexed directly by node ID at `nodes_at + n * NODE_SIZE`.
//!
//! # Fast-Path Caching and `RELEASE_EPOCH`
//!
//! Views with `handlers == 0` whose nodes and style ops all fall within the common subset
//! (div/h_flex/v_flex, bare `Label`, bare `N_TEXT`, child views, and `N_UNIFORM_LIST`) can be
//! compiled into a [`FastTree`] after their first frame (`GkView::renders >= 1` confirms the
//! tape is actually reused across frames rather than immediately replaced). `FastTree` caches
//! resolved `AnyView` and `Entity<RowStore>` handles alongside pre-decoded `FastStyle` arrays,
//! while keeping theme colors symbolic so theme switches still take effect on render. Each
//! `FastTree` stamps the current `rt::release_epoch()`; whenever any handle is released anywhere
//! in the session, `RELEASE_EPOCH` advances and `GkView::render` drops the cached `FastTree` so
//! stale cached views or row stores are never rendered.

use crate::ops::*;
use crate::rt::{GkErr, GkResult};
use crate::wire::R;
use gpui_kit::SharedString;

/// Little-endian ASCII `"GKT1"` header magic validating tape framing and format version 1.
const MAGIC: u32 = 0x3154_4b47;
/// Size in bytes of one node-table entry: `ty: u16` (2) + `args: u32` (4) + `first_op: u32` (4).
const NODE_SIZE: usize = 10;

/// Generated opcode for `gpui_kit::component::label::Label`, matched directly by both
/// [`parse_table_cells`] and [`build_fast_node`] to bypass generated builder dispatch.
const NODE_COMPONENT_LABEL: u16 = 685;

/// Pre-decoded column entry extracted from an `N_TABLE_CELLS` row tape during [`Tape::parse`].
///
/// Unstyled `Label` and `N_TEXT` cells are copied into `SharedString` once at parse time so
/// `GkTableDelegate::render_td` can emit them without opening a [`crate::genapi::Dx`] decoder
/// on every cell scroll; any cell with style ops or another opcode keeps its node index for
/// general decoding.
pub enum TableCell {
    Label(SharedString),
    Text(SharedString),
    Node(u32),
}

/// Parsed, immutable view tape owning its raw byte buffer and pre-validated section offsets.
pub struct Tape {
    bytes: Vec<u8>,
    /// Monotonic host generation counter paired with `view` when retiring old handler tables.
    pub generation: u32,
    /// Shared retirement pin cloned into every GPUI event closure decoded from this tape.
    /// When `handlers == 0`, no pin is allocated and nothing needs retirement.
    pub pin: Option<std::sync::Arc<GenPin>>,
    /// Byte offset where the ops arena begins inside `bytes`.
    ops_at: usize,
    /// Byte length of the ops arena.
    ops_len: usize,
    /// Byte offset where the fixed-stride node table begins (`ops_at + ops_len`).
    nodes_at: usize,
    node_count: u32,
    /// Root node index (`0..node_count`).
    pub root: u32,
    /// Number of host event handlers referenced by this tape; gates both [`GenPin`] allocation
    /// and [`FastTree`] eligibility.
    pub handlers: u32,
    /// `HK_STATE` handles whose mutations should mark the owning view dirty.
    pub deps: Vec<u64>,
    /// Child `HK_VIEW` handles embedded in this tape to be mounted in the parent's window.
    pub slots: Vec<u64>,
    /// Pre-parsed column entries when `root` is an `N_TABLE_CELLS` table-row container.
    pub table_cells: Option<Box<[TableCell]>>,
}

/// Reference-counted retirement token keeping a tape's host handler table alive until both the
/// [`Tape`] itself and every GPUI event closure (`TapePin`) created from it have been dropped.
///
/// Dropping the last `Arc<GenPin>` calls [`crate::rt::retire`], which is safe from any thread and
/// piggybacks `(view, generation)` onto the next turn so the host frees its handler table.
pub struct GenPin {
    pub view: u64,
    pub generation: u32,
}

impl Drop for GenPin {
    fn drop(&mut self) {
        crate::rt::retire(self.view, self.generation);
    }
}

impl Tape {
    /// Validates the tape header, dependency/slot lists, ops arena, and exact node-table length,
    /// returning `ERR_BAD_TAPE` on any truncation or magic mismatch.
    ///
    /// Minimum size is 28 bytes (16-byte header + 2-byte `n_deps` + 2-byte `n_slots` + 4-byte
    /// `ops_len` + at least one 10-byte node would be 34, with 28 guarding the fixed prefix).
    /// When the root node is `N_TABLE_CELLS`, immediately runs [`parse_table_cells`] once so
    /// table cell rendering avoids repeated per-cell tape parsing.
    pub fn parse(bytes: Vec<u8>, generation: u32) -> GkResult<Tape> {
        let bad = |m: &str| GkErr(ERR_BAD_TAPE, m.to_string());
        if bytes.len() < 28 {
            return Err(bad("tape shorter than its header"));
        }
        let mut r = R::new(&bytes);
        if r.u32() != MAGIC {
            return Err(bad("bad tape magic"));
        }
        let node_count = r.u32();
        let root = r.u32();
        let handlers = r.u32();
        let n_deps = r.u16() as usize;
        if bytes.len() < r.i + n_deps * 8 + 2 {
            return Err(bad("tape cut in its dependency list"));
        }
        let deps = (0..n_deps).map(|_| r.u64()).collect();
        let n_slots = r.u16() as usize;
        if bytes.len() < r.i + n_slots * 8 + 4 {
            return Err(bad("tape cut in its slot list"));
        }
        let slots = (0..n_slots).map(|_| r.u64()).collect();
        let ops_len = r.u32() as usize;
        let ops_at = r.i;
        let nodes_at = ops_at + ops_len;
        if bytes.len() != nodes_at + node_count as usize * NODE_SIZE || root >= node_count {
            return Err(bad("tape length does not match its node table"));
        }
        let mut tape = Tape {
            bytes,
            generation,
            pin: None,
            ops_at,
            ops_len,
            nodes_at,
            node_count,
            root,
            handlers,
            deps,
            slots,
            table_cells: None,
        };
        let (root_ty, root_args, _) = tape.node(root);
        if root_ty == N_TABLE_CELLS {
            tape.table_cells = parse_table_cells(&tape, root_args);
        }
        Ok(tape)
    }

    /// Returns `(node_type, args_offset, first_op_offset)` for node index `n`.
    pub fn node(&self, n: u32) -> (u16, u32, u32) {
        assert!(n < self.node_count, "gk: tape names node {n} of {}", self.node_count);
        let mut r = R::at(&self.bytes, self.nodes_at + n as usize * NODE_SIZE);
        (r.u16(), r.u32(), r.u32())
    }

    /// Returns the raw ops byte slice shared by all nodes in this tape.
    pub fn ops(&self) -> &[u8] {
        &self.bytes[self.ops_at..self.ops_at + self.ops_len]
    }
}

/// Bounds-checked UTF-8 string reader (`[len: u32][utf8...]`) used by fast-path pre-decoders
/// so malformed or non-UTF-8 payloads gracefully fall back instead of panicking inside `R`.
fn try_read_str(ops: &[u8], at: usize) -> Option<&str> {
    let end = at.checked_add(4)?;
    let n = u32::from_le_bytes(ops.get(at..end)?.try_into().ok()?) as usize;
    let bytes = ops.get(end..end.checked_add(n)?)?;
    std::str::from_utf8(bytes).ok()
}

/// Pre-decodes an `N_TABLE_CELLS` root (`[n_cols: u32][child_node_id: u32*]`) into a boxed slice
/// of [`TableCell`]s.
///
/// Cells with `first_op == 0` (no builder/style ops) whose type is `NODE_COMPONENT_LABEL` or
/// `N_TEXT` materialize their `SharedString` once; all other cells keep their node index so
/// `render_td` runs the full decoder on them. Returns `None` if the list is out of bounds.
fn parse_table_cells(tape: &Tape, root_args: u32) -> Option<Box<[TableCell]>> {
    let ops = tape.ops();
    let at = root_args as usize;
    let n = u32::from_le_bytes(ops.get(at..at.checked_add(4)?)?.try_into().ok()?) as usize;
    let list_at = at + 4;
    if ops.len() < list_at.checked_add(n.checked_mul(4)?)? {
        return None;
    }
    let mut cells = Vec::with_capacity(n);
    for i in 0..n {
        let off = list_at + i * 4;
        let child = u32::from_le_bytes(ops[off..off + 4].try_into().ok()?);
        if child >= tape.node_count {
            return None;
        }
        let (ty, args, first) = tape.node(child);
        let spec = if first == 0 && ty == NODE_COMPONENT_LABEL {
            match try_read_str(ops, args as usize) {
                Some(s) => TableCell::Label(SharedString::new(s)),
                None => TableCell::Node(child),
            }
        } else if first == 0 && ty == N_TEXT {
            match try_read_str(ops, args as usize) {
                Some(s) => TableCell::Text(SharedString::new(s)),
                None => TableCell::Node(child),
            }
        } else {
            TableCell::Node(child)
        };
        cells.push(spec);
    }
    Some(cells.into_boxed_slice())
}

/// Pre-decoded element tree for handler-free views containing only common divs, text, labels,
/// child views, and uniform lists.
///
/// # Caching and Invalidation Contract
///
/// - **Unchanged-tape detection**: `GkView` drops its `FastTree` and resets `renders = 0` every
///   time `rt::accept_one` installs a new `Rc<Tape>`. Only when GPUI re-renders the exact same
///   `Tape` on a subsequent frame (`renders >= 1`) does `GkView` call [`FastTree::try_build`],
///   avoiding compilation overhead on views that change every frame.
/// - **What is cached**: node hierarchy (`FastNode`), pre-parsed `FastStyle` values (lengths,
///   HSLA colors, style tokens, and symbolic theme-color IDs), `SharedString` text/labels, and
///   resolved GPUI `AnyView` / `Entity<RowStore>` handles.
/// - **`RELEASE_EPOCH` guard**: because `FastNode::ChildView` and `FastNode::UniformList` cache
///   resolved GPUI entity handles, every `Session::release` increments `RELEASE_EPOCH`. Before
///   replaying a `FastTree`, `GkView::render` checks `fast.epoch == rt::release_epoch()`; any
///   mismatch clears the cache so a released handle is caught by the general decoder rather than
///   silently keeping an old entity alive.
pub struct FastTree {
    /// Snapshot of `rt::release_epoch()` taken when this tree was compiled.
    pub epoch: u64,
    root: FastNode,
}

/// Compact pre-decoded representation of high-frequency `Styled` builder ops.
///
/// Theme colors (`ThemeBg`, `ThemeText`, `ThemeBorder`) store their token ID rather than a
/// resolved `Hsla` so switching light/dark themes updates cached views immediately on next paint.
#[derive(Clone, Copy)]
enum FastStyle {
    Token(u16),
    Bg(gpui_kit::Hsla),
    ThemeBg(u16),
    TextColor(gpui_kit::Hsla),
    ThemeText(u16),
    BorderColor(gpui_kit::Hsla),
    ThemeBorder(u16),
    W(gpui_kit::Length),
    H(gpui_kit::Length),
    MinW(gpui_kit::Length),
    MinH(gpui_kit::Length),
    MaxW(gpui_kit::Length),
    MaxH(gpui_kit::Length),
    Size(gpui_kit::Length),
    Gap(gpui_kit::DefiniteLength),
    GapX(gpui_kit::DefiniteLength),
    GapY(gpui_kit::DefiniteLength),
    P(gpui_kit::DefiniteLength),
    Px(gpui_kit::DefiniteLength),
    Py(gpui_kit::DefiniteLength),
    Pt(gpui_kit::DefiniteLength),
    Pb(gpui_kit::DefiniteLength),
    Pl(gpui_kit::DefiniteLength),
    Pr(gpui_kit::DefiniteLength),
    M(gpui_kit::Length),
    Mx(gpui_kit::Length),
    My(gpui_kit::Length),
    Mt(gpui_kit::Length),
    Mb(gpui_kit::Length),
    Ml(gpui_kit::Length),
    Mr(gpui_kit::Length),
    Border(gpui_kit::AbsoluteLength),
    BorderT(gpui_kit::AbsoluteLength),
    BorderB(gpui_kit::AbsoluteLength),
    BorderL(gpui_kit::AbsoluteLength),
    BorderR(gpui_kit::AbsoluteLength),
    BorderX(gpui_kit::AbsoluteLength),
    BorderY(gpui_kit::AbsoluteLength),
    Rounded(gpui_kit::AbsoluteLength),
    RoundedT(gpui_kit::AbsoluteLength),
    RoundedB(gpui_kit::AbsoluteLength),
    RoundedL(gpui_kit::AbsoluteLength),
    RoundedR(gpui_kit::AbsoluteLength),
    RoundedTl(gpui_kit::AbsoluteLength),
    RoundedTr(gpui_kit::AbsoluteLength),
    RoundedBl(gpui_kit::AbsoluteLength),
    RoundedBr(gpui_kit::AbsoluteLength),
    FlexBasis(gpui_kit::Length),
    FlexGrow(f32),
    FlexShrink(f32),
    LineHeight(gpui_kit::DefiniteLength),
    TextSize(gpui_kit::AbsoluteLength),
    Opacity(f32),
}

/// Compiled node variants supported by [`FastTree`].
enum FastNode {
    /// `kind`: `0` = `div()`, `1` = `h_flex()`, `2` = `v_flex()`.
    Div {
        kind: u8,
        styles: Box<[FastStyle]>,
        children: Box<[FastNode]>,
    },
    /// Cached `AnyView` resolved from `E_CHILD_VIEW` (invalidated via `RELEASE_EPOCH`).
    ChildView(gpui_kit::AnyView),
    Label(SharedString),
    Text(SharedString),
    /// Cached `RowStore` entity resolved from `N_UNIFORM_LIST` (invalidated via `RELEASE_EPOCH`).
    UniformList {
        id: SharedString,
        store: gpui_kit::Entity<crate::rt::RowStore>,
        styles: Box<[FastStyle]>,
    },
}

impl FastTree {
    /// Attempts to compile `tape` into a [`FastTree`] stamped with `epoch`.
    ///
    /// Returns `None` immediately when `tape.handlers != 0` (event closures require a `Dx` and
    /// `TapePin`) or when any node or op is outside the fast-path subset, letting `GkView::render`
    /// fall back to `genapi::root` and remember `fast_tried = true`.
    pub fn try_build(tape: &Tape, epoch: u64) -> Option<Self> {
        if tape.handlers != 0 {
            return None;
        }
        let root = build_fast_node(tape, tape.ops(), tape.root)?;
        Some(FastTree { epoch, root })
    }

    /// Instantiates a fresh GPUI `AnyElement` tree from the cached [`FastNode`]s without
    /// re-reading tape bytes.
    pub fn render(&self, cx: &mut gpui_kit::App) -> gpui_kit::AnyElement {
        self.root.render(cx)
    }
}

/// Decodes a subset of high-frequency generated `Styled` opcodes into [`FastStyle`] entries.
///
/// The numeric opcode arms (`16897..=16973`) correspond to the generated `Styled` method opcodes
/// in `core/src/gen/`. Gradients (`COLOR_GRADIENT`) and any unsupported op return `false`,
/// aborting `FastTree` compilation cleanly for that view.
fn push_fast_style_op(out: &mut Vec<FastStyle>, op: u16, r: &mut R) -> bool {
    use crate::generated::{absolute_length, definite_length, length};
    use gpui_kit::hsla;
    match op {
        E_STYLE => {
            let n = r.u16() as usize;
            out.reserve(n);
            for _ in 0..n {
                out.push(FastStyle::Token(r.u16()));
            }
            true
        }
        16897 => match r.u8() {
            COLOR_THEME => { out.push(FastStyle::ThemeBg(r.u16())); true }
            COLOR_HSLA => { out.push(FastStyle::Bg(hsla(r.f32(), r.f32(), r.f32(), r.f32()))); true }
            _ => false,
        },
        16969 => match r.u8() {
            COLOR_THEME => { out.push(FastStyle::ThemeText(r.u16())); true }
            COLOR_HSLA => { out.push(FastStyle::TextColor(hsla(r.f32(), r.f32(), r.f32(), r.f32()))); true }
            _ => false,
        },
        16900 => match r.u8() {
            COLOR_THEME => { out.push(FastStyle::ThemeBorder(r.u16())); true }
            COLOR_HSLA => { out.push(FastStyle::BorderColor(hsla(r.f32(), r.f32(), r.f32(), r.f32()))); true }
            _ => false,
        },
        16898 => { out.push(FastStyle::Border(absolute_length(r))); true }
        16899 => { out.push(FastStyle::BorderB(absolute_length(r))); true }
        16901 => { out.push(FastStyle::BorderL(absolute_length(r))); true }
        16902 => { out.push(FastStyle::BorderR(absolute_length(r))); true }
        16903 => { out.push(FastStyle::BorderT(absolute_length(r))); true }
        16904 => { out.push(FastStyle::BorderX(absolute_length(r))); true }
        16905 => { out.push(FastStyle::BorderY(absolute_length(r))); true }
        16911 => { out.push(FastStyle::FlexBasis(length(r))); true }
        16912 => { out.push(FastStyle::FlexGrow(r.f32())); true }
        16913 => { out.push(FastStyle::FlexShrink(r.f32())); true }
        16916 => { out.push(FastStyle::Gap(definite_length(r))); true }
        16917 => { out.push(FastStyle::GapX(definite_length(r))); true }
        16918 => { out.push(FastStyle::GapY(definite_length(r))); true }
        16925 => { out.push(FastStyle::H(length(r))); true }
        16929 => { out.push(FastStyle::LineHeight(definite_length(r))); true }
        16930 => { out.push(FastStyle::M(length(r))); true }
        16931 => { out.push(FastStyle::MaxH(length(r))); true }
        16933 => { out.push(FastStyle::MaxW(length(r))); true }
        16934 => { out.push(FastStyle::Mb(length(r))); true }
        16935 => { out.push(FastStyle::MinH(length(r))); true }
        16937 => { out.push(FastStyle::MinW(length(r))); true }
        16938 => { out.push(FastStyle::Ml(length(r))); true }
        16939 => { out.push(FastStyle::Mr(length(r))); true }
        16940 => { out.push(FastStyle::Mt(length(r))); true }
        16941 => { out.push(FastStyle::Mx(length(r))); true }
        16942 => { out.push(FastStyle::My(length(r))); true }
        16943 => { out.push(FastStyle::Opacity(r.f32())); true }
        16944 => { out.push(FastStyle::P(definite_length(r))); true }
        16945 => { out.push(FastStyle::Pb(definite_length(r))); true }
        16946 => { out.push(FastStyle::Pl(definite_length(r))); true }
        16947 => { out.push(FastStyle::Pr(definite_length(r))); true }
        16948 => { out.push(FastStyle::Pt(definite_length(r))); true }
        16949 => { out.push(FastStyle::Px(definite_length(r))); true }
        16950 => { out.push(FastStyle::Py(definite_length(r))); true }
        16952 => { out.push(FastStyle::Rounded(absolute_length(r))); true }
        16953 => { out.push(FastStyle::RoundedB(absolute_length(r))); true }
        16954 => { out.push(FastStyle::RoundedBl(absolute_length(r))); true }
        16955 => { out.push(FastStyle::RoundedBr(absolute_length(r))); true }
        16956 => { out.push(FastStyle::RoundedL(absolute_length(r))); true }
        16957 => { out.push(FastStyle::RoundedR(absolute_length(r))); true }
        16958 => { out.push(FastStyle::RoundedT(absolute_length(r))); true }
        16959 => { out.push(FastStyle::RoundedTl(absolute_length(r))); true }
        16960 => { out.push(FastStyle::RoundedTr(absolute_length(r))); true }
        16966 => { out.push(FastStyle::Size(length(r))); true }
        16971 => { out.push(FastStyle::TextSize(absolute_length(r))); true }
        16973 => { out.push(FastStyle::W(length(r))); true }
        _ => false,
    }
}

/// Replays pre-decoded [`FastStyle`] entries onto `style`, resolving symbolic theme tokens
/// against `cx.theme()` at render time.
#[inline]
fn apply_fast_styles(style: &mut gpui_kit::StyleRefinement, styles: &[FastStyle], cx: &gpui_kit::App) {
    use crate::generated::W;
    use gpui_kit::component::ActiveTheme;
    use gpui_kit::Styled;
    for s in styles {
        match *s {
            FastStyle::Token(t) => { crate::generated::style_token(style, t); }
            FastStyle::Bg(c) => { let _ = Styled::bg(W(style), c); }
            FastStyle::ThemeBg(id) => {
                let colors = &cx.theme().colors;
                let c = crate::generated::color_token(colors, id).unwrap_or(colors.foreground);
                let _ = Styled::bg(W(style), c);
            }
            FastStyle::TextColor(c) => { let _ = Styled::text_color(W(style), c); }
            FastStyle::ThemeText(id) => {
                let colors = &cx.theme().colors;
                let c = crate::generated::color_token(colors, id).unwrap_or(colors.foreground);
                let _ = Styled::text_color(W(style), c);
            }
            FastStyle::BorderColor(c) => { let _ = Styled::border_color(W(style), c); }
            FastStyle::ThemeBorder(id) => {
                let colors = &cx.theme().colors;
                let c = crate::generated::color_token(colors, id).unwrap_or(colors.foreground);
                let _ = Styled::border_color(W(style), c);
            }
            FastStyle::W(l) => { let _ = Styled::w(W(style), l); }
            FastStyle::H(l) => { let _ = Styled::h(W(style), l); }
            FastStyle::MinW(l) => { let _ = Styled::min_w(W(style), l); }
            FastStyle::MinH(l) => { let _ = Styled::min_h(W(style), l); }
            FastStyle::MaxW(l) => { let _ = Styled::max_w(W(style), l); }
            FastStyle::MaxH(l) => { let _ = Styled::max_h(W(style), l); }
            FastStyle::Size(l) => { let _ = Styled::size(W(style), l); }
            FastStyle::Gap(l) => { let _ = Styled::gap(W(style), l); }
            FastStyle::GapX(l) => { let _ = Styled::gap_x(W(style), l); }
            FastStyle::GapY(l) => { let _ = Styled::gap_y(W(style), l); }
            FastStyle::P(l) => { let _ = Styled::p(W(style), l); }
            FastStyle::Px(l) => { let _ = Styled::px(W(style), l); }
            FastStyle::Py(l) => { let _ = Styled::py(W(style), l); }
            FastStyle::Pt(l) => { let _ = Styled::pt(W(style), l); }
            FastStyle::Pb(l) => { let _ = Styled::pb(W(style), l); }
            FastStyle::Pl(l) => { let _ = Styled::pl(W(style), l); }
            FastStyle::Pr(l) => { let _ = Styled::pr(W(style), l); }
            FastStyle::M(l) => { let _ = Styled::m(W(style), l); }
            FastStyle::Mx(l) => { let _ = Styled::mx(W(style), l); }
            FastStyle::My(l) => { let _ = Styled::my(W(style), l); }
            FastStyle::Mt(l) => { let _ = Styled::mt(W(style), l); }
            FastStyle::Mb(l) => { let _ = Styled::mb(W(style), l); }
            FastStyle::Ml(l) => { let _ = Styled::ml(W(style), l); }
            FastStyle::Mr(l) => { let _ = Styled::mr(W(style), l); }
            FastStyle::Border(l) => { let _ = Styled::border(W(style), l); }
            FastStyle::BorderT(l) => { let _ = Styled::border_t(W(style), l); }
            FastStyle::BorderB(l) => { let _ = Styled::border_b(W(style), l); }
            FastStyle::BorderL(l) => { let _ = Styled::border_l(W(style), l); }
            FastStyle::BorderR(l) => { let _ = Styled::border_r(W(style), l); }
            FastStyle::BorderX(l) => { let _ = Styled::border_x(W(style), l); }
            FastStyle::BorderY(l) => { let _ = Styled::border_y(W(style), l); }
            FastStyle::Rounded(l) => { let _ = Styled::rounded(W(style), l); }
            FastStyle::RoundedT(l) => { let _ = Styled::rounded_t(W(style), l); }
            FastStyle::RoundedB(l) => { let _ = Styled::rounded_b(W(style), l); }
            FastStyle::RoundedL(l) => { let _ = Styled::rounded_l(W(style), l); }
            FastStyle::RoundedR(l) => { let _ = Styled::rounded_r(W(style), l); }
            FastStyle::RoundedTl(l) => { let _ = Styled::rounded_tl(W(style), l); }
            FastStyle::RoundedTr(l) => { let _ = Styled::rounded_tr(W(style), l); }
            FastStyle::RoundedBl(l) => { let _ = Styled::rounded_bl(W(style), l); }
            FastStyle::RoundedBr(l) => { let _ = Styled::rounded_br(W(style), l); }
            FastStyle::FlexBasis(l) => { let _ = Styled::flex_basis(W(style), l); }
            FastStyle::FlexGrow(v) => { let _ = Styled::flex_grow(W(style), v); }
            FastStyle::FlexShrink(v) => { let _ = Styled::flex_shrink(W(style), v); }
            FastStyle::LineHeight(l) => { let _ = Styled::line_height(W(style), l); }
            FastStyle::TextSize(l) => { let _ = Styled::text_size(W(style), l); }
            FastStyle::Opacity(v) => { let _ = Styled::opacity(W(style), v); }
        }
    }
}

/// Recursively compiles node `n` into a [`FastNode`], returning `None` if any descendant,
/// handle lookup, or op falls outside the fast-path subset.
fn build_fast_node(tape: &Tape, ops: &[u8], n: u32) -> Option<FastNode> {
    if n >= tape.node_count {
        return None;
    }
    let (ty, args, first) = tape.node(n);
    match ty {
        // Generated opcodes 256=h_flex, 257=v_flex, 258=div.
        256..=258 => {
            let kind = match ty {
                256 => 1,
                257 => 2,
                _ => 0,
            };
            let mut styles = Vec::new();
            let mut children = Vec::new();
            let mut at = first;
            while at != 0 {
                let mut r = R::at(ops, at as usize);
                let op = r.u16();
                at = r.u32();
                match op {
                    E_CHILD => {
                        children.push(build_fast_node(tape, ops, r.u32())?);
                    }
                    E_CHILD_VIEW => {
                        let view = crate::rt::with(|s| s.view(r.u64())).ok()?;
                        children.push(FastNode::ChildView(view.into()));
                    }
                    E_TEXT => {
                        children.push(FastNode::Text(SharedString::new(r.str())));
                    }
                    _ => {
                        if !push_fast_style_op(&mut styles, op, &mut r) {
                            return None;
                        }
                    }
                }
            }
            Some(FastNode::Div {
                kind,
                styles: styles.into_boxed_slice(),
                children: children.into_boxed_slice(),
            })
        }
        NODE_COMPONENT_LABEL if first == 0 => {
            let text = try_read_str(ops, args as usize)?;
            Some(FastNode::Label(SharedString::new(text)))
        }
        N_TEXT if first == 0 => {
            let text = try_read_str(ops, args as usize)?;
            Some(FastNode::Text(SharedString::new(text)))
        }
        N_UNIFORM_LIST => {
            let mut a = R::at(ops, args as usize);
            let id = SharedString::from(a.str());
            let store = crate::rt::with(|s| s.rows(a.u64())).ok()?;
            let mut styles = Vec::new();
            let mut at = first;
            while at != 0 {
                let mut r = R::at(ops, at as usize);
                let op = r.u16();
                at = r.u32();
                if !push_fast_style_op(&mut styles, op, &mut r) {
                    return None;
                }
            }
            Some(FastNode::UniformList {
                id,
                store,
                styles: styles.into_boxed_slice(),
            })
        }
        _ => None,
    }
}

impl FastNode {
    fn render(&self, cx: &mut gpui_kit::App) -> gpui_kit::AnyElement {
        use gpui_kit::{IntoElement, ParentElement, Styled};
        match self {
            FastNode::Div { kind, styles, children } => {
                let mut el = match *kind {
                    1 => gpui_kit::base::h_flex(),
                    2 => gpui_kit::base::v_flex(),
                    _ => gpui_kit::div(),
                };
                if !styles.is_empty() {
                    apply_fast_styles(el.style(), styles, cx);
                }
                for c in children.iter() {
                    match c {
                        FastNode::ChildView(v) => el = el.child(v.clone()),
                        FastNode::Text(s) => el = el.child(s.clone()),
                        FastNode::Label(s) => {
                            el = el.child(gpui_kit::component::label::Label::new(s.clone()));
                        }
                        _ => el = el.child(c.render(cx)),
                    }
                }
                el.into_any_element()
            }
            FastNode::ChildView(v) => v.clone().into_any_element(),
            FastNode::Label(s) => {
                gpui_kit::component::label::Label::new(s.clone()).into_any_element()
            }
            FastNode::Text(s) => s.clone().into_any_element(),
            FastNode::UniformList { id, store, styles } => {
                let rows = store.read(cx).rows.clone();
                let count = rows.borrow().len();
                let mut el = gpui_kit::uniform_list(id.clone(), count, move |range, _, _| {
                    let rows = rows.borrow();
                    range
                        .map(|i| crate::hand::list_row(rows[i].clone()))
                        .collect::<Vec<_>>()
                });
                if !styles.is_empty() {
                    apply_fast_styles(el.style(), styles, cx);
                }
                el.into_any_element()
            }
        }
    }
}
