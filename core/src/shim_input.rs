//! Delegate shims of the input family (docs/PROTOCOLS.md section 9.6). The state types
//! themselves (InputBaseState<M> as InputState, EditorState and TextareaState) are generated
//! by the `[instantiate] states` rule; what is here is what an app implements upstream: the
//! editor's completion and hover providers. Each is a shim over a row source used only for
//! its request channel (count 0): the kit asks, the shim sends the question to the app as a
//! queued event with a request id (`source::request`), and the app answers whenever it likes
//! with `gk.answer`; the kit's task completes with the decoded answer. App code never runs
//! while the kit lays out or draws, and an unanswered request leaves the menu empty, as an
//! upstream provider that returns nothing does.
//! Call op 32 (EDITOR_PROVIDERS), shape 14 (PROVIDER), node 10 (INPUT_EDITOR, the kit's styled
//! code editor, which draws the menus).

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

/// The family's element nodes; None: not one of them.
pub fn node(dx: &mut Dx, ty: u16, args: u32, first: u32) -> Option<AnyElement> {
    Some(match ty {
        N_INPUT_EDITOR => decode_editor(dx, args, first),
        // the editor host (docs/PROTOCOLS.md 9.30)
        crate::ops::N_EDITOR_HOST => return crate::editor_text::host_node(dx, args, first),
        _ => return None,
    })
}

/// The kit's styled code editor, which draws the providers' menus. Upstream declares it in a
/// doc(hidden) module (re-exported as gpui_kit::component::input::Editor), so the API extract
/// has no item for the generator: this node is written by hand, as generated decoders are.
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

/// A provider's question, after the request id (shape PROVIDER).
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
    // The lists and editor families' providers (docs/PROTOCOLS.md 9.11): variants 2 to 7.
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

fn opt_str(r: &mut R) -> Option<String> {
    (r.u8() != 0).then(|| r.str().to_string())
}

/// `gk.Completion_Response`: u32 n, (str label, u8 kind, opt<str> detail, opt<str>
/// insert_text) each, u8 incomplete. Kind 0 is none, otherwise LSP's CompletionItemKind.
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

/// An LSP completion item given to a call (`highlight_prefix(query, &item)`, PROTOCOLS.md 9.19):
/// `gk.Completion_Item` as a provider answers one, str label, u8 kind, opt<str> detail, opt<str>
/// insert_text.
impl crate::genapi::Unwire for CompletionItem {
    fn get(r: &mut R) -> Self {
        let label = r.str().to_string();
        let kind = r.u8();
        let detail = opt_str(r);
        let insert_text = opt_str(r);
        CompletionItem { label, kind: item_kind(kind), detail, insert_text, ..Default::default() }
    }
}

/// An LSP completion item read back (a key of the completion highlighter's ask, PROTOCOLS.md 9.33):
/// the layout `Unwire` reads, kind by number (0: none).
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

/// LSP's CompletionItemKind by number (1 text .. 25 type parameter); 0 or unknown: none.
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

/// `Maybe(string)`: the hover's markdown, or none.
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

/// Sends `ask` to the app and decodes its answer on the foreground executor.
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

/// CompletionProvider over a row source. The trigger characters and the inline completions'
/// debounce are data the app set.
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

    /// A request when the app asked for inline completions; the answer is the text to insert,
    /// or none.
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
        // 0: upstream's default (300 ms).
        Duration::from_millis(if self.debounce_ms == 0 { 300 } else { self.debounce_ms as u64 })
    }
}

/// `Maybe(string)`: the inline completion's text.
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

/// An LSP range as `gk.Text_Range` writes it: u32 start line, start character, end line, end
/// character (LSP positions: UTF-16 characters).
fn lsp_range(r: &mut R) -> LspRange {
    let (a, b, c, d) = (r.u32(), r.u32(), r.u32(), r.u32());
    LspRange::new(Position::new(a, b), Position::new(c, d))
}

/// `[]gk.Definition`: (str uri, range target, range selection) each.
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

/// `[]gk.Document_Color`: (range, [4]f32 rgba) each.
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

/// `[]gk.Code_Action`: (str title, str kind, str id) each. The id comes back to the app in
/// Perform (it rides in the action's command).
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

/// `[]u32`: LSP's encoded semantic tokens, five numbers each.
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

/// DefinitionProvider over a row source.
pub struct GkDefinitionProvider {
    src: u64,
}

impl DefinitionProvider for GkDefinitionProvider {
    fn definitions(&self, text: &Rope, offset: usize, _: &mut Window, cx: &mut App) -> Task<anyhow::Result<Vec<LocationLink>>> {
        ask(self.src, Ask::Definitions { text: text.to_string(), offset }, cx, definitions)
    }
}

/// DocumentColorProvider over a row source.
pub struct GkDocumentColorProvider {
    src: u64,
}

impl DocumentColorProvider for GkDocumentColorProvider {
    fn document_colors(&self, text: &Rope, _: &mut Window, cx: &mut App) -> Task<anyhow::Result<Vec<ColorInformation>>> {
        ask(self.src, Ask::Colors { text: text.to_string() }, cx, colors)
    }
}

/// CodeActionProvider over a row source; its id is data the app set.
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
        // The app performs it (with the editor's generated writes) and answers when done.
        let id = action.command.as_ref().map(|c| c.command.clone()).unwrap_or_default();
        ask(self.src, Ask::Perform { title: action.title, id, push_to_history }, cx, |_| ())
    }
}

/// DocumentRangeSemanticTokensProvider over a row source; its legend is data the app set.
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

/// HoverProvider over a row source.
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

/// The family's call ops; None: not one of them.
pub fn call(op: u16, r: &mut R, c: &mut Call) -> Option<GkResult<()>> {
    Some(match op {
        C_EDITOR_PROVIDERS => {
            // u64 state, u64 source, u32 which (1 completions, 2 hover, 4 definitions, 8 document
            // colors, 16 code actions, 32 semantic tokens, 64 inline completions), str triggers;
            // then str code action provider id, u32 inline debounce ms, u32 n, str token type
            // each, u32 n, str token modifier each.
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
        // The editor family's ops (core/src/shim_editor.rs), asked
        // from here so that the shared dispatch in calls.rs needs no line of its own.
        _ => return crate::shim_editor::call(op, r, c),
    })
}
