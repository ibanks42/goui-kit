//! The editor family's gallery pages (docs/PROTOCOLS.md 9.20 to 9.23), the Rust reference of
//! examples/go/gallery/editor_pages.go, call for call: a block surface, headings styled by level,
//! Markdown plugins (the kit's frontmatter plugin, a block plugin for ```badge code and an inline one
//! for inline code, written in Rust here as an app would) and a link's tooltip; the editor host
//! and the decorated buttons, the Lsp's completion highlighter and a presented diagnostic.
use gpui_kit::base::{
    InlineElement, InlineRenderContext, MarkdownExtensions, MarkdownNode, MarkdownParseContext, MarkdownPlugin, TextView,
    TextViewStyle, markdown_ast,
};
use gpui_kit::component::ActiveTheme;
use gpui_kit::component::label::Label;
use gpui_kit::component::text::FrontmatterPlugin;
use gpui_kit::base::text::CodeBlock;
use gpui_kit::base::text::editor_host::{
    BlockNode, FlowPosition, InlineNode, Paragraph, RenderContext, TextMark, collect_leaf_geometry, parse_markdown,
};
use gpui_kit::{
    AnyElement, App, AppContext, Bounds, Context, Element, ElementId, Entity, GlobalElementId, Hsla, InspectorElementId,
    InteractiveElement, IntoElement, LayoutId, ParentElement, Pixels, Render, SharedString, StyleRefinement, Styled, Window,
    div, px,
};
use std::collections::HashMap;
use std::sync::Arc;

const SURFACE_MD: &str = "> [!note]\n> Inside the surface.\n\nAfter it.\n";
const HEADING_MD: &str = "# One\n\n## Two\n\n### Three\n\nBody text.\n";
const PLUGIN_MD: &str = "---\ntitle: Notes\nowner: me\n---\n\n# Plugins\n\n```badge\nready\n```\n\nSome `inline` text.\n";
const TOOLTIP_MD: &str = "[docs](https://example.com/docs) and more";

/// ```badge code blocks, drawn as their text in brackets.
struct Badge;

impl MarkdownPlugin for Badge {
    fn is_block(&self) -> bool {
        true
    }

    fn name(&self) -> &str {
        "badge"
    }

    fn parse(&self, node: &markdown_ast::Node, cx: &MarkdownParseContext<'_>) -> Option<MarkdownNode> {
        let markdown_ast::Node::Code(c) = node else { return None };
        if c.lang.as_deref() != Some("badge") {
            return None;
        }
        let source = cx.node_source(node).unwrap_or("").to_string();
        Some(MarkdownNode::new("badge", ()).text(c.value.clone()).markdown(source))
    }

    fn render(&self, node: &MarkdownNode, _: &mut Window, _: &mut App) -> impl IntoElement {
        Label::new(format!("[{}]", node.as_text()))
    }
}

/// Inline code, drawn as its text in angle brackets with a baseline 12 px from its top.
struct Code;

impl MarkdownPlugin for Code {
    fn name(&self) -> &str {
        "code"
    }

    fn parse(&self, node: &markdown_ast::Node, cx: &MarkdownParseContext<'_>) -> Option<MarkdownNode> {
        let markdown_ast::Node::InlineCode(c) = node else { return None };
        let source = cx.node_source(node).unwrap_or("").to_string();
        Some(MarkdownNode::new("code", ()).text(c.value.clone()).markdown(source))
    }

    fn render_inline(&self, node: &MarkdownNode, _: &InlineRenderContext, _: &mut Window, _: &mut App) -> Option<InlineElement> {
        Some(InlineElement::new(Label::new(format!("<{}>", node.as_text()))).with_baseline(px(12.)))
    }
}

/// A link's tooltip: "open" and the URL.
struct Tip(SharedString);

impl Render for Tip {
    fn render(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        Label::new(format!("open {}", self.0))
    }
}

fn plugin_view() -> AnyElement {
    let ext = MarkdownExtensions::default().frontmatter().plugin(FrontmatterPlugin::new());
    div()
        .w(px(420.))
        .child(TextView::markdown("plugin", PLUGIN_MD).markdown_extensions(ext).plugin(Badge).plugin(Code))
        .into_any_element()
}

pub fn page(id: &str, cx: &mut App) -> Option<AnyElement> {
    Some(match id {
        "editor5-surface" => {
            let ext = MarkdownExtensions::default()
                .block_surface("note", |_, cx| div().p_2().bg(cx.theme().secondary).child(Label::new("Note")));
            div().w(px(360.)).child(TextView::markdown("surface", SURFACE_MD).markdown_extensions(ext)).into_any_element()
        }
        "editor5-heading" => {
            let (danger, success) = (cx.theme().danger, cx.theme().success);
            let style = TextViewStyle::default().with_heading(move |level| match level {
                1 => StyleRefinement::default().text_color(danger),
                2 => StyleRefinement::default().text_color(success),
                _ => StyleRefinement::default(),
            });
            // The next page's text view drawn here first (its plugin rows are
            // described the frame after the nodes are parsed).
            div()
                .w(px(360.))
                .child(TextView::markdown("heading", HEADING_MD).style(style))
                .child(div().h(px(0.)).overflow_hidden().child(plugin_view()))
                .into_any_element()
        }
        "editor5-plugin" => plugin_view(),
        "editor6-host" => {
            let sel = cx.theme().selection;
            div().w(px(420.)).child(Host { blocks: host_blocks(), decorations: HashMap::from([(2, vec![(0..5, sel)])]) }).into_any_element()
        }
        "editor7-number" | "editor7-scroller" | "editor7-lsp" | "editor7-diagnostic" => round7(id, cx),
        "editor5-tooltip" => div()
            .w(px(360.))
            .child(TextView::markdown("tooltip", TOOLTIP_MD).link_tooltip(|url, _, cx| cx.new(|_| Tip(url.clone())).into()))
            .into_any_element(),
        _ => return None,
    })
}

// ---------------------------------------------------------------- the editor host (9.30)

const HOST_MD: &str = "## Parsed\n\n- one\n- two\n\n> quoted *text*\n";

fn para(text: &[InlineNode]) -> Paragraph {
    let mut p = Paragraph::default();
    for t in text {
        p.push(t.clone());
    }
    p
}

/// The page's blocks, as an editor written outside the kit would hand them over: a heading, a
/// paragraph of two inline nodes, a list of two items, a code block, then the parsed ones.
fn host_blocks() -> Vec<BlockNode> {
    let mut v = vec![
        BlockNode::Heading { level: 2, children: para(&[InlineNode::new("Host")]).with_leaf_key(1), span: None },
        BlockNode::Paragraph(
            para(&[InlineNode::new("Hello ").marks(vec![(0..5, TextMark::default().bold())]), InlineNode::new("world")])
                .with_leaf_key(2),
        ),
        BlockNode::List {
            children: vec![
                BlockNode::ListItem {
                    children: vec![BlockNode::Paragraph(para(&[InlineNode::new("first")]).with_leaf_key(3))],
                    spread: false,
                    checked: None,
                    task_marker: None,
                    span: None,
                },
                BlockNode::ListItem {
                    children: vec![BlockNode::Paragraph(para(&[InlineNode::new("second")]))],
                    spread: false,
                    checked: Some(true),
                    task_marker: None,
                    span: None,
                },
            ],
            ordered: false,
            start: None,
            span: None,
        },
        BlockNode::CodeBlock(CodeBlock::from_code("x := 1", Some("demo")).with_leaf_key(4)),
    ];
    v.extend(parse_markdown(HOST_MD, Arc::new(MarkdownExtensions::default())).unwrap_or_default());
    v
}

/// What an editor's host element does: the blocks rendered as TextView renders them, their prepaint
/// inside collect_leaf_geometry with the decorations (the geometry is not used here).
struct Host {
    blocks: Vec<BlockNode>,
    decorations: HashMap<u64, Vec<(std::ops::Range<usize>, Hsla)>>,
}

// gpui-fast (0.1.16 on) has `impl<T: Element> IntoElement for T`; upstream GPUI does not.
#[cfg(not(feature = "gpui-fast"))]
impl IntoElement for Host {
    type Element = Self;

    fn into_element(self) -> Self {
        self
    }
}

impl Element for Host {
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
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (LayoutId, AnyElement) {
        let rc = RenderContext::from_app(cx);
        let refs: Vec<&BlockNode> = self.blocks.iter().collect();
        let n = refs.len();
        let children: Vec<AnyElement> =
            (0..n).map(|ix| refs[ix].render(ix, FlowPosition::at(&refs, ix, ix + 1 == n), &rc, window, cx)).collect();
        let mut el = div().id("editor6-host").children(children).into_any_element();
        (el.request_layout(window, cx), el)
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        _: Bounds<Pixels>,
        el: &mut AnyElement,
        window: &mut Window,
        cx: &mut App,
    ) {
        let decorations = std::mem::take(&mut self.decorations);
        let _ = collect_leaf_geometry(decorations, || el.prepaint(window, cx));
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        _: Bounds<Pixels>,
        el: &mut AnyElement,
        _: &mut (),
        window: &mut Window,
        cx: &mut App,
    ) {
        el.paint(window, cx);
    }
}

// ---------------------------------------------------------------- 9.38 to 9.40

use gpui_kit::base::input::{Diagnostic, DiagnosticEntry, DiagnosticSeverity, EditorState, Position};
use gpui_kit::component::button::Button;
use gpui_kit::base::input::InputState;
use gpui_kit::component::message_scroller::{MessageScroller, MessageScrollerState};
use gpui_kit::component::v_flex;
use lsp_types::{CompletionItem, CompletionItemKind};
use std::cell::RefCell;
use std::rc::Rc;

const MESSAGES: usize = 30;

/// The editor7-* pages, one view: its states are made the first time it renders, where the window is.
#[derive(Default)]
struct Round7 {
    page: &'static str,
    number: Option<Entity<InputState>>,
    scroller: Option<Entity<MessageScrollerState>>,
    lsp: Option<Entity<EditorState>>,
    diag: Option<Entity<EditorState>>,
}

thread_local! {
    static ROUND7: RefCell<Option<Entity<Round7>>> = const { RefCell::new(None) };
}

fn round7(id: &str, cx: &mut App) -> AnyElement {
    let page: &'static str = match id {
        "editor7-number" => "editor7-number",
        "editor7-scroller" => "editor7-scroller",
        "editor7-lsp" => "editor7-lsp",
        _ => "editor7-diagnostic",
    };
    let view = ROUND7.with(|r| r.borrow().clone()).unwrap_or_else(|| {
        let v = cx.new(|_| Round7::default());
        ROUND7.with(|r| *r.borrow_mut() = Some(v.clone()));
        v
    });
    view.update(cx, |v, _| v.page = page);
    view.into_any_element()
}

fn item(label: &str, kind: CompletionItemKind, detail: Option<&str>) -> CompletionItem {
    CompletionItem { label: label.into(), kind: Some(kind), detail: detail.map(Into::into), ..Default::default() }
}

impl Render for Round7 {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let number = self.number.get_or_insert_with(|| cx.new(|cx| InputState::new(window, cx).default_value("4"))).clone();
        let scroller = self.scroller.get_or_insert_with(|| cx.new(|cx| MessageScrollerState::new(MESSAGES, cx))).clone();
        let lsp = self
            .lsp
            .get_or_insert_with(|| {
                cx.new(|cx| {
                    let mut s = EditorState::new(window, cx).default_value("fi");
                    // The second and third character of each label.
                    s.lsp_mut().set_completion_highlighter(Rc::new(|_: &str, item: &CompletionItem| {
                        let n = item.label.len();
                        vec![1.min(n)..3.min(n)]
                    }));
                    s
                })
            })
            .clone();
        let diag = self.diag.get_or_insert_with(|| cx.new(|cx| EditorState::new(window, cx).default_value("let x = 1\n"))).clone();
        match self.page {
            "editor7-number" => {
                let primary = cx.theme().primary;
                div()
                    .w(px(200.))
                    .child(
                        gpui_kit::base::NumberInput::new(&number)
                            .increment_button(move |b| b.bg(primary).child(Label::new("+")))
                            .decrement_button(|b| b.child(Label::new("-"))),
                    )
                    .into_any_element()
            }
            "editor7-scroller" => {
                let danger = cx.theme().danger;
                div()
                    .w(px(300.))
                    .h(px(200.))
                    .child(
                        MessageScroller::new("scroller", scroller, |ix, _, _| Label::new(format!("message {ix}")))
                            .scrollbar(false)
                            .with_jump_button_renderer(move |b| b.bg(danger).child(Label::new("latest"))),
                    )
                    .into_any_element()
            }
            "editor7-lsp" => v_flex()
                .gap_2()
                .child(Button::new("complete").label("Complete").on_click({
                    let lsp = lsp.clone();
                    move |_, _, cx| {
                        let items = vec![
                            item("first", CompletionItemKind::FUNCTION, Some("fn first()")),
                            item("fifth", CompletionItemKind::VARIABLE, None),
                        ];
                        lsp.update(cx, |s, cx| s.present_completion_items(2, "fi", items, cx));
                    }
                }))
                .child(div().w(px(420.)).child(gpui_kit::component::input::Editor::new(&lsp).h(px(160.))))
                .into_any_element(),
            _ => v_flex()
                .gap_2()
                .child(Button::new("diagnose").label("Diagnose").on_click({
                    let diag = diag.clone();
                    move |_, _, cx| {
                        let diagnostic = Diagnostic::new(Position::new(0, 4)..Position::new(0, 5), "unused x")
                            .with_severity(DiagnosticSeverity::Warning);
                        diag.update(cx, |s, cx| s.present_diagnostic(DiagnosticEntry { range: 4..5, diagnostic }, cx));
                    }
                }))
                .child(div().w(px(420.)).child(gpui_kit::component::input::Editor::new(&diag).h(px(160.))))
                .into_any_element(),
        }
    }
}
