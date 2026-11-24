//! Input, textarea, code editor, number input, OTP, and LSP completion gallery pages.

use crate::Gallery;
use gpui_kit::base::Editor;
use gpui_kit::base::input::{CompletionProvider, EditorState, TextareaState};
use gpui_kit::base::OtpState;
use gpui_kit::component::input::{Input, InputState, NumberInput, OtpInput, Textarea};
use gpui_kit::component::button::Button;
use gpui_kit::component::{Disableable, h_flex, v_flex};
use gpui_kit::{AnyElement, App, AppContext, Entity, IntoElement, ParentElement, Styled, Task, Window, div, px};
use lsp_types::{CompletionContext, CompletionItem, CompletionItemKind, CompletionList, CompletionResponse};
use ropey::Rope;
use std::rc::Rc;

/// Persistent state entities shared across the `input-*`, `textarea`, `editor*`,
/// `number-input`, and `otp-input` gallery pages.
pub struct InputPages {
    pub edit: Entity<InputState>,
    pub text: Entity<TextareaState>,
    pub editor: Entity<EditorState>,
    pub number: Entity<InputState>,
    pub otp: Entity<OtpState>,
    pub masked: Entity<InputState>,
    pub off: Entity<InputState>,
    pub completion: Entity<EditorState>,
    pub writes: Entity<InputState>,
}

/// Static completion provider triggered by `.` on the `editor-completion` page.
struct Completions;

impl CompletionProvider for Completions {
    fn completions(
        &self,
        _text: &Rope,
        _offset: usize,
        _trigger: CompletionContext,
        _window: &mut Window,
        _cx: &mut App,
    ) -> Task<anyhow::Result<CompletionResponse>> {
        let item = |label: &str, kind, detail: Option<&str>| CompletionItem {
            label: label.into(),
            kind: Some(kind),
            detail: detail.map(Into::into),
            ..Default::default()
        };
        Task::ready(Ok(CompletionResponse::List(CompletionList {
            is_incomplete: false,
            items: vec![
                item("first", CompletionItemKind::FUNCTION, Some("fn first()")),
                item("second", CompletionItemKind::VARIABLE, None),
            ],
        })))
    }

    fn is_completion_trigger(&self, _offset: usize, new_text: &str, _cx: &mut App) -> bool {
        new_text.contains('.')
    }
}

/// Sample Rust snippet preloaded into the `editor` page.
pub const EDITOR_TEXT: &str = "fn main() {\n    let x = 40 + 2;\n    println!(\"{x}\");\n}\n";

impl InputPages {
    /// Allocates and configures all input/editor state entities for the gallery.
    pub fn new(window: &mut Window, cx: &mut App) -> Self {
        InputPages {
            edit: cx.new(|cx| InputState::new(window, cx).placeholder("Type here")),
            text: cx.new(|cx| TextareaState::new(window, cx).auto_grow(2, 4).placeholder("Notes")),
            editor: cx.new(|cx| {
                let mut s = EditorState::new(window, cx).language("rust").default_value(EDITOR_TEXT);
                s.set_line_number(true, window, cx);
                s
            }),
            number: cx.new(|cx| {
                InputState::new(window, cx).min(0.).max(10.).step(2.).default_value("4")
            }),
            otp: cx.new(|cx| OtpState::new(4, window, cx)),
            masked: cx.new(|cx| InputState::new(window, cx).masked(true).default_value("secret")),
            off: cx.new(|cx| InputState::new(window, cx).default_value("disabled")),
            writes: cx.new(|cx| InputState::new(window, cx).default_value("hello world")),
            completion: cx.new(|cx| {
                let mut s = EditorState::new(window, cx).language("rust");
                s.lsp_mut().completion_provider = Some(Rc::new(Completions));
                s
            }),
        }
    }
}

/// Renders an input/editor gallery page if `id` belongs to this module.
pub fn page(id: &str, g: &Gallery) -> Option<AnyElement> {
    let p = &g.input;
    Some(match id {
        "input-edit" => div().w(px(240.)).child(Input::new(&p.edit)).into_any_element(),
        "textarea" => div().w(px(240.)).child(Textarea::new(&p.text)).into_any_element(),
        "editor" => div().w(px(480.)).h(px(160.)).child(Editor::new(&p.editor)).into_any_element(),
        "number-input" => div().w(px(200.)).child(NumberInput::new(&p.number)).into_any_element(),
        "otp-input" => OtpInput::new(&p.otp).into_any_element(),
        "editor-completion" => {
            let editor = gpui_kit::component::input::Editor::new(&p.completion).h(px(160.));
            div().w(px(480.)).child(editor).into_any_element()
        }
        "input-writes" => {

            let s = &p.writes;
            let button = |id: &'static str, label: &'static str| Button::new(id).label(label).w(px(80.));
            v_flex()
                .gap_2()
                .child(div().w(px(240.)).child(Input::new(s)))
                .child(
                    h_flex()
                        .gap_2()
                        .child(button("w-select", "Select").on_click({
                            let s = s.clone();
                            move |_, _, cx| s.update(cx, |s, cx| s.set_selected_range(0..5, cx))
                        }))
                        .child(button("w-replace", "Replace").on_click({
                            let s = s.clone();
                            move |_, window, cx| s.update(cx, |s, cx| s.replace("HELLO", window, cx))
                        }))
                        .child(button("w-insert", "Insert").on_click({
                            let s = s.clone();
                            move |_, window, cx| s.update(cx, |s, cx| s.insert("!", window, cx))
                        }))
                        .child(button("w-set", "Set").on_click({
                            let s = s.clone();
                            move |_, window, cx| s.update(cx, |s, cx| s.set_value("reset", window, cx))
                        })),
                )
                .into_any_element()
        }
        "input-masked" => v_flex()
            .gap_2()
            .child(div().w(px(240.)).child(Input::new(&p.masked).mask_toggle()))
            .child(div().w(px(240.)).child(Input::new(&p.off).disabled(true)))
            .into_any_element(),
        _ => return None,
    })
}
