//! Editor element decoding and asynchronous LSP provider shims.
//!
//! `Editor` nodes are decoded here (`N_INPUT_EDITOR`) alongside forwarding
//! `N_EDITOR_HOST` to `editor_text`. All six gpui-kit LSP provider traits
//! (completions/inline completions, hover, definitions, document colors,
//! code actions, and range semantic tokens) are bridged to host code over
//! `source::request` using the `SHAPE_PROVIDER` request-reply channel.

use crate::genapi::{self, Call, Dx, GkErr, GkResult, R, Wire};
use crate::ops::*;
use crate::source;
use crate::wire::Put;
use gpui_kit::base::input::{
    CodeActionProvider, CompletionProvider, DefinitionProvider, DocumentColorProvider,
    DocumentRangeSemanticTokensProvider, EditorState, HoverProvider,
};
use gpui_kit::component::input::Editor;
use gpui_kit::{AnyElement, App, IntoElement, SharedString, Styled, Task, Window, px};
use lsp_types::{
    CodeAction, CodeActionKind, Color, ColorInformation, Command, CompletionContext, CompletionItem,
    CompletionItemKind, CompletionList, CompletionResponse, CompletionTriggerKind, Hover,
    HoverContents, InlineCompletionContext, InlineCompletionItem, InlineCompletionResponse,
    InlineCompletionTriggerKind, LocationLink, MarkupContent, MarkupKind, Position,
    Range as LspRange, SemanticToken, SemanticTokenModifier, SemanticTokenType, SemanticTokens,
    SemanticTokensLegend, Uri,
};
use gpui_kit::Entity;
use std::time::Duration;
use ropey::Rope;
use std::rc::Rc;

/// Decodes custom editor tape nodes (`N_INPUT_EDITOR` and `N_EDITOR_HOST`).
pub fn node(dx: &mut Dx, ty: u16, args: u32, first: u32) -> Option<AnyElement> {
    Some(match ty {
        N_INPUT_EDITOR => decode_editor(dx, args, first),
        crate::ops::N_EDITOR_HOST => return crate::editor_text::host_node(dx, args, first),
        _ => return None,
    })
}

/// Decodes an `N_INPUT_EDITOR` node.
///
/// Argument layout: `state: u64`, `height: f32` (`> 0` applies fixed height),
/// `flags: u8` (`1`=bordered, `2`=appearance, `4`=disabled, `8`=readonly),
/// followed by style ops on the node's op list.
fn decode_editor(dx: &mut Dx, args: u32, first: u32) -> AnyElement {
    let mut a = dx.args(args);
    let state = genapi::shown::<EditorState>(a.u64());
    let height = a.f32();
    let flags = a.u8();
    let mut el = Editor::new(&state)
        .bordered(flags & 1 != 0)
        .appearance(flags & 2 != 0)
        .disabled(flags & 4 != 0)
        .readonly(flags & 8 != 0);
    if height > 0. {
        el = el.h(px(height));
    }
    let mut at = first;
    while let Some((op, mut r)) = dx.op(&mut at) {
        if !crate::generated::style_op(el.style(), op, &mut r, dx) {
            eprintln!("gk: op {op} is not an Editor op");
        }
    }
    el.into_any_element()
}

/// Tagged LSP provider query payloads sent over `SHAPE_PROVIDER`.
///
/// Each variant writes a `u16` discriminant (`0..=7`) followed by its fields.
enum Ask {
    Completions {
        text: String,
        offset: usize,
        kind: u8,
        trigger: Option<String>,
    },
    Hover {
        text: String,
        offset: usize,
    },
    Definitions {
        text: String,
        offset: usize,
    },
    Colors {
        text: String,
    },
    CodeActions {
        start: usize,
        end: usize,
    },
    Perform {
        title: String,
        id: String,
        push_to_history: bool,
    },
    Tokens {
        text: String,
        start: usize,
        end: usize,
    },
    Inline {
        text: String,
        offset: usize,
        automatic: bool,
    },
}

impl Wire for Ask {
    fn put(&self, out: &mut Vec<u8>) {
        match self {
            Ask::Completions {
                text,
                offset,
                kind,
                trigger,
            } => {
                out.put_u16(0);
                text.put(out);
                offset.put(out);
                kind.put(out);
                trigger.put(out);
            }
            Ask::Hover { text, offset } => {
                out.put_u16(1);
                text.put(out);
                offset.put(out);
            }
            Ask::Definitions { text, offset } => {
                out.put_u16(2);
                text.put(out);
                offset.put(out);
            }
            Ask::Colors { text } => {
                out.put_u16(3);
                text.put(out);
            }
            Ask::CodeActions { start, end } => {
                out.put_u16(4);
                start.put(out);
                end.put(out);
            }
            Ask::Perform { title, id, push_to_history } => {
                out.put_u16(5);
                title.put(out);
                id.put(out);
                push_to_history.put(out);
            }
            Ask::Tokens { text, start, end } => {
                out.put_u16(6);
                text.put(out);
                start.put(out);
                end.put(out);
            }
            Ask::Inline { text, offset, automatic } => {
                out.put_u16(7);
                text.put(out);
                offset.put(out);
                automatic.put(out);
            }
        }
    }
}

/// Reads an optional string prefixed by a `u8` presence flag.
fn opt_str(r: &mut R) -> Option<String> {
    (r.u8() != 0).then(|| r.str().to_string())
}

/// Decodes a `CompletionResponse::List` reply: `count: u32`, items, then `is_incomplete: u8`.
fn completions(bytes: &[u8]) -> CompletionResponse {
    let mut r = R::new(bytes);
    let n = r.u32() as usize;
    let mut items = Vec::with_capacity(n.min(4096));
    for _ in 0..n {
        let label = r.str().to_string();
        let kind = r.u8();
        let detail = opt_str(&mut r);
        let insert_text = opt_str(&mut r);
        items.push(CompletionItem {
            label,
            kind: item_kind(kind),
            detail,
            insert_text,
            ..Default::default()
        });
    }
    let incomplete = r.u8() != 0;
    CompletionResponse::List(CompletionList {
        is_incomplete: incomplete,
        items,
    })
}

impl crate::genapi::Unwire for CompletionItem {
    fn get(r: &mut R) -> Self {
        let label = r.str().to_string();
        let kind = r.u8();
        let detail = opt_str(r);
        let insert_text = opt_str(r);
        CompletionItem { label, kind: item_kind(kind), detail, insert_text, ..Default::default() }
    }
}

impl crate::genapi::Wire for CompletionItem {
    fn put(&self, out: &mut Vec<u8>) {
        use crate::genapi::Wire as _;
        self.label.put(out);
        let kind = self.kind.and_then(|k| (1..=25u8).find(|n| item_kind(*n) == Some(k))).unwrap_or(0);
        kind.put(out);
        self.detail.put(out);
        self.insert_text.put(out);
    }
}

/// Maps wire completion-kind tags (`1..=25`, matching LSP `CompletionItemKind`)
/// to `Option<CompletionItemKind>`, with `0` or out-of-range returning `None`.
fn item_kind(n: u8) -> Option<CompletionItemKind> {
    use CompletionItemKind as K;
    const KINDS: [CompletionItemKind; 25] = [
        K::TEXT, K::METHOD, K::FUNCTION, K::CONSTRUCTOR, K::FIELD, K::VARIABLE, K::CLASS,
        K::INTERFACE, K::MODULE, K::PROPERTY, K::UNIT, K::VALUE, K::ENUM, K::KEYWORD, K::SNIPPET,
        K::COLOR, K::FILE, K::REFERENCE, K::FOLDER, K::ENUM_MEMBER, K::CONSTANT, K::STRUCT,
        K::EVENT, K::OPERATOR, K::TYPE_PARAMETER,
    ];
    (n as usize).checked_sub(1).and_then(|i| KINDS.get(i)).copied()
}

/// Decodes a markdown hover reply (`opt_str`).
fn hover(bytes: &[u8]) -> Option<Hover> {
    let mut r = R::new(bytes);
    opt_str(&mut r).map(|value| Hover {
        contents: HoverContents::Markup(MarkupContent {
            kind: MarkupKind::Markdown,
            value,
        }),
        range: None,
    })
}

/// Sends an asynchronous LSP provider request to `src` and decodes the host
/// reply on the foreground executor.
fn ask<T: 'static>(src: u64, ask: Ask, cx: &mut App, decode: fn(&[u8]) -> T) -> Task<anyhow::Result<T>> {
    match source::request(src, SHAPE_PROVIDER, &ask, cx) {
        Ok(rx) => cx.foreground_executor().spawn(async move {
            match rx.await {
                Ok(bytes) => Ok(decode(&bytes)),
                Err(_) => Err(anyhow::anyhow!("the request was released before the app answered it")),
            }
        }),
        Err(GkErr(_, msg)) => Task::ready(Err(anyhow::anyhow!(msg))),
    }
}

/// Completion and optional inline-completion provider backed by a host source.
pub struct GkCompletionProvider {
    src: u64,
    triggers: String,
    inline: bool,
    debounce_ms: u32,
}

impl CompletionProvider for GkCompletionProvider {
    fn completions(
        &self,
        text: &Rope,
        offset: usize,
        trigger: CompletionContext,
        _window: &mut Window,
        cx: &mut App,
    ) -> Task<anyhow::Result<CompletionResponse>> {
        let kind = if trigger.trigger_kind == CompletionTriggerKind::INVOKED {
            1
        } else if trigger.trigger_kind == CompletionTriggerKind::TRIGGER_CHARACTER {
            2
        } else {
            3
        };
        let q = Ask::Completions {
            text: text.to_string(),
            offset,
            kind,
            trigger: trigger.trigger_character,
        };
        ask(self.src, q, cx, completions)
    }

    fn is_completion_trigger(&self, _offset: usize, new_text: &str, _cx: &mut App) -> bool {
        !self.triggers.is_empty() && new_text.chars().any(|c| self.triggers.contains(c))
    }

    fn inline_completion(
        &self,
        rope: &Rope,
        offset: usize,
        trigger: InlineCompletionContext,
        _window: &mut Window,
        cx: &mut App,
    ) -> Task<anyhow::Result<InlineCompletionResponse>> {
        if !self.inline {
            return Task::ready(Ok(InlineCompletionResponse::Array(vec![])));
        }
        let q = Ask::Inline {
            text: rope.to_string(),
            offset,
            automatic: trigger.trigger_kind == InlineCompletionTriggerKind::Automatic,
        };
        ask(self.src, q, cx, inline_items)
    }

    fn inline_completion_debounce(&self) -> Duration {
        // Treat 0 as "use default debounce" (300 ms).
        Duration::from_millis(if self.debounce_ms == 0 { 300 } else { self.debounce_ms as u64 })
    }
}

/// Decodes a single optional inline-completion suggestion (`opt_str`).
fn inline_items(bytes: &[u8]) -> InlineCompletionResponse {
    let mut r = R::new(bytes);
    InlineCompletionResponse::Array(
        opt_str(&mut r)
            .map(|insert_text| InlineCompletionItem {
                insert_text,
                filter_text: None,
                range: None,
                command: None,
                insert_text_format: None,
            })
            .into_iter()
            .collect(),
    )
}

/// Decodes an LSP range as four `u32` values: `(start_line, start_char, end_line, end_char)`.
fn lsp_range(r: &mut R) -> LspRange {
    let (a, b, c, d) = (r.u32(), r.u32(), r.u32(), r.u32());
    LspRange::new(Position::new(a, b), Position::new(c, d))
}

/// Decodes go-to-definition targets (`count: u32`, then `uri`, `target_range`, `selection_range`).
fn definitions(bytes: &[u8]) -> Vec<LocationLink> {
    let mut r = R::new(bytes);
    let n = r.u32() as usize;
    let mut out = Vec::with_capacity(n.min(1024));
    for _ in 0..n {
        let uri = r.str().to_string();
        let (target_range, target_selection_range) = (lsp_range(&mut r), lsp_range(&mut r));
        if let Ok(target_uri) = uri.parse::<Uri>() {
            out.push(LocationLink { origin_selection_range: None, target_uri, target_range, target_selection_range });
        }
    }
    out
}

/// Decodes document color swatches (`count: u32`, then `range` and RGBA `f32`s).
fn colors(bytes: &[u8]) -> Vec<ColorInformation> {
    let mut r = R::new(bytes);
    let n = r.u32() as usize;
    let mut out = Vec::with_capacity(n.min(10_000));
    for _ in 0..n {
        let range = lsp_range(&mut r);
        let (red, green, blue, alpha) = (r.f32(), r.f32(), r.f32(), r.f32());
        out.push(ColorInformation { range, color: Color { red, green, blue, alpha } });
    }
    out
}

/// Decodes code actions (`count: u32`, then `title`, `kind`, `id` strings).
fn code_actions(bytes: &[u8]) -> Vec<CodeAction> {
    let mut r = R::new(bytes);
    let n = r.u32() as usize;
    let mut out = Vec::with_capacity(n.min(1024));
    for _ in 0..n {
        let title = r.str().to_string();
        let kind = r.str().to_string();
        let id = r.str().to_string();
        out.push(CodeAction {
            title: title.clone(),
            kind: (!kind.is_empty()).then(|| CodeActionKind::from(kind)),
            command: Some(Command { title, command: id, arguments: None }),
            ..Default::default()
        });
    }
    out
}

/// Decodes LSP relative semantic-token data (`u32` count followed by flat 5-tuples).
fn tokens(bytes: &[u8]) -> SemanticTokens {
    let mut r = R::new(bytes);
    let n = r.u32() as usize;
    let mut data = Vec::with_capacity((n / 5).min(1 << 20));
    let mut v = Vec::with_capacity(n.min(1 << 22));
    for _ in 0..n {
        v.push(r.u32());
    }
    for c in v.chunks_exact(5) {
        data.push(SemanticToken {
            delta_line: c[0],
            delta_start: c[1],
            length: c[2],
            token_type: c[3],
            token_modifiers_bitset: c[4],
        });
    }
    SemanticTokens { result_id: None, data }
}

/// Go-to-definition provider backed by a host source.
pub struct GkDefinitionProvider {
    src: u64,
}

impl DefinitionProvider for GkDefinitionProvider {
    fn definitions(&self, text: &Rope, offset: usize, _: &mut Window, cx: &mut App) -> Task<anyhow::Result<Vec<LocationLink>>> {
        ask(self.src, Ask::Definitions { text: text.to_string(), offset }, cx, definitions)
    }
}

/// Document color swatch provider backed by a host source.
pub struct GkDocumentColorProvider {
    src: u64,
}

impl DocumentColorProvider for GkDocumentColorProvider {
    fn document_colors(&self, text: &Rope, _: &mut Window, cx: &mut App) -> Task<anyhow::Result<Vec<ColorInformation>>> {
        ask(self.src, Ask::Colors { text: text.to_string() }, cx, colors)
    }
}

/// Code action provider backed by a host source and identified by `id`.
pub struct GkCodeActionProvider {
    src: u64,
    id: SharedString,
}

impl CodeActionProvider for GkCodeActionProvider {
    fn id(&self) -> SharedString {
        self.id.clone()
    }

    fn code_actions(
        &self,
        _state: Entity<EditorState>,
        range: std::ops::Range<usize>,
        _: &mut Window,
        cx: &mut App,
    ) -> Task<anyhow::Result<Vec<CodeAction>>> {
        ask(self.src, Ask::CodeActions { start: range.start, end: range.end }, cx, code_actions)
    }

    fn perform_code_action(
        &self,
        _state: Entity<EditorState>,
        action: CodeAction,
        push_to_history: bool,
        _: &mut Window,
        cx: &mut App,
    ) -> Task<anyhow::Result<()>> {
        // Recover the host-assigned action id stored in `command.command`.
        let id = action.command.as_ref().map(|c| c.command.clone()).unwrap_or_default();
        ask(self.src, Ask::Perform { title: action.title, id, push_to_history }, cx, |_| ())
    }
}

/// Range semantic-tokens provider backed by a host source and static legend.
pub struct GkSemanticTokensProvider {
    src: u64,
    legend: SemanticTokensLegend,
}

impl DocumentRangeSemanticTokensProvider for GkSemanticTokensProvider {
    fn legend(&self) -> SemanticTokensLegend {
        self.legend.clone()
    }

    fn semantic_tokens(
        &self,
        text: &Rope,
        range: std::ops::Range<usize>,
        _: &mut Window,
        cx: &mut App,
    ) -> Task<anyhow::Result<SemanticTokens>> {
        ask(self.src, Ask::Tokens { text: text.to_string(), start: range.start, end: range.end }, cx, tokens)
    }
}

/// Markdown hover provider backed by a host source.
pub struct GkHoverProvider {
    src: u64,
}

impl HoverProvider for GkHoverProvider {
    fn hover(&self, text: &Rope, offset: usize, _window: &mut Window, cx: &mut App) -> Task<anyhow::Result<Option<Hover>>> {
        let q = Ask::Hover {
            text: text.to_string(),
            offset,
        };
        ask(self.src, q, cx, hover)
    }
}

/// Dispatches `C_EDITOR_PROVIDERS` and falls through to `shim_editor::call`.
///
/// `C_EDITOR_PROVIDERS` installs or clears LSP providers on an `EditorState`
/// according to the `which` bitmask:
/// `1` = completions, `2` = hover, `4` = definitions, `8` = document colors,
/// `16` = code actions, `32` = semantic tokens, `64` = inline completions.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_EDITOR_PROVIDERS => {
            let (state, src, which) = (r.u64(), r.u64(), r.u32());
            let triggers = r.str().to_string();
            let action_id = SharedString::from(r.str().to_string());
            let debounce_ms = r.u32();
            let types: Vec<SemanticTokenType> = (0..r.u32()).map(|_| SemanticTokenType::from(r.str().to_string())).collect();
            let modifiers: Vec<SemanticTokenModifier> =
                (0..r.u32()).map(|_| SemanticTokenModifier::from(r.str().to_string())).collect();
            let legend = SemanticTokensLegend { token_types: types, token_modifiers: modifiers };
            if let Err(e) = source::count(src, 0) {
                return Some(Err(e));
            }
            genapi::state_update::<EditorState>(state, c, move |s, _, cx| {
                let lsp = s.lsp_mut();
                lsp.completion_provider = (which & (1 | 64) != 0).then(|| {
                    let inline = which & 64 != 0;
                    Rc::new(GkCompletionProvider { src, triggers, inline, debounce_ms }) as Rc<dyn CompletionProvider>
                });
                lsp.hover_provider =
                    (which & 2 != 0).then(|| Rc::new(GkHoverProvider { src }) as Rc<dyn HoverProvider>);
                lsp.definition_provider = (which & 4 != 0)
                    .then(|| Rc::new(GkDefinitionProvider { src }) as Rc<dyn DefinitionProvider>);
                lsp.document_color_provider = (which & 8 != 0)
                    .then(|| Rc::new(GkDocumentColorProvider { src }) as Rc<dyn DocumentColorProvider>);
                // Replace any previous code-action provider registered under the same id.
                lsp.code_action_providers.retain(|p| p.id() != action_id);
                if which & 16 != 0 {
                    lsp.code_action_providers
                        .push(Rc::new(GkCodeActionProvider { src, id: action_id }) as Rc<dyn CodeActionProvider>);
                }
                lsp.semantic_tokens_provider = (which & 32 != 0).then(|| {
                    Rc::new(GkSemanticTokensProvider { src, legend }) as Rc<dyn DocumentRangeSemanticTokensProvider>
                });
                cx.notify();
            })
        }

        _ => return crate::shim_editor::call(op, r, c),
    })
}
