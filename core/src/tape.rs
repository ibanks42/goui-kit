//! The tape (DESIGN.md section 6): what a view procedure wrote, kept by the view entity and
//! decoded into real GPUI-kit elements when GPUI builds that view.
//!
//! Tape bytes, as the binding writes them:
//!   u32 magic "GKT1", u32 node_count, u32 root, u32 handler_count,
//!   u16 n_deps, u64 * n      kit state the view procedure read
//!   u16 n_slots, u64 * n     child views the tape embeds
//!   u32 ops_len, ops         the op stream; offsets are relative to its start, 0 = none
//!   nodes: node_count * (u16 type, u32 args, u32 first_op)
//! An op is u16 opcode, u32 offset of the next op of the same node, arguments.

use crate::ops::*;
use crate::rt::{GkErr, GkResult};
use crate::wire::R;
use gpui_kit::SharedString;

const MAGIC: u32 = 0x3154_4b47;
const NODE_SIZE: usize = 10;
/// `gpui_kit::component::label::Label::new` in `api/opcodes.lock` (append-only).
const NODE_COMPONENT_LABEL: u16 = 685;

/// One cell of an `N_TABLE_CELLS` tape, pre-parsed once when the row's tape is accepted.
/// Plain labels and text nodes keep their `SharedString` ready to clone with zero per-frame
/// allocation or indirect family dispatch; any other cell decodes its node through the full
/// tape decoder.
pub enum TableCell {
    Label(SharedString),
    Text(SharedString),
    Node(u32),
}

pub struct Tape {
    bytes: Vec<u8>,
    pub generation: u32,
    /// The token every holder of this description shares (the tape, and each handler closure made
    /// from it: `genapi::Dx::pin`); set when the library accepts a view's tape (`rt::accept`),
    /// none for a tape of no view (a fragment a call op carries) or a tape with zero handlers.
    pub pin: Option<std::sync::Arc<GenPin>>,
    ops_at: usize,
    ops_len: usize,
    nodes_at: usize,
    node_count: u32,
    pub root: u32,
    pub handlers: u32,
    pub deps: Vec<u64>,
    pub slots: Vec<u64>,
    pub table_cells: Option<Box<[TableCell]>>,
}

/// One view description's handlers, held by everything that may still fire one: the tape (its
/// view, an element or kept value built from it, such as an open menu) and each handler closure
/// made from it. When the last holder lets go its handlers can no longer fire, so the binding may
/// forget their table: `rt::retire` hands (view, generation) to the next VIEWS turn (docs/
/// PROTOCOLS.md section 4). Shared with an Arc: upstream may ask for a closure that is Send.
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

    /// (type, args offset, first op offset) of a node.
    pub fn node(&self, n: u32) -> (u16, u32, u32) {
        assert!(n < self.node_count, "gk: tape names node {n} of {}", self.node_count);
        let mut r = R::at(&self.bytes, self.nodes_at + n as usize * NODE_SIZE);
        (r.u16(), r.u32(), r.u32())
    }

    pub fn ops(&self) -> &[u8] {
        &self.bytes[self.ops_at..self.ops_at + self.ops_len]
    }
}

fn try_read_str(ops: &[u8], at: usize) -> Option<&str> {
    let end = at.checked_add(4)?;
    let n = u32::from_le_bytes(ops.get(at..end)?.try_into().ok()?) as usize;
    let bytes = ops.get(end..end.checked_add(n)?)?;
    std::str::from_utf8(bytes).ok()
}

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

// --- Lazy fast-path cache for unchanged tapes decoded repeatedly -----------------------------

/// A compact, pre-parsed element tree built lazily on the second render of an unchanged tape
/// (for views like counter's root, echo, and list that gpui-fast rebuilds on every click while
/// their tape never changes). Changing tapes never build one. Style ops are stored directly as a
/// small flat slice instead of cloning `StyleRefinement`; theme colors stay symbolic and resolve
/// against `cx.theme()` on every render; handles are guarded by `rt::release_epoch()`.
pub struct FastTree {
    pub epoch: u64,
    root: FastNode,
}

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

enum FastNode {
    Div {
        kind: u8, // 0 = div, 1 = h_flex, 2 = v_flex
        styles: Box<[FastStyle]>,
        children: Box<[FastNode]>,
    },
    ChildView(gpui_kit::AnyView),
    Label(SharedString),
    Text(SharedString),
    UniformList {
        id: SharedString,
        store: gpui_kit::Entity<crate::rt::RowStore>,
        styles: Box<[FastStyle]>,
    },
}

impl FastTree {
    pub fn try_build(tape: &Tape, epoch: u64) -> Option<Self> {
        if tape.handlers != 0 {
            return None;
        }
        let root = build_fast_node(tape, tape.ops(), tape.root)?;
        Some(FastTree { epoch, root })
    }

    pub fn render(&self, cx: &mut gpui_kit::App) -> gpui_kit::AnyElement {
        self.root.render(cx)
    }
}

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

fn build_fast_node(tape: &Tape, ops: &[u8], n: u32) -> Option<FastNode> {
    if n >= tape.node_count {
        return None;
    }
    let (ty, args, first) = tape.node(n);
    match ty {
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
