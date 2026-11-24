// The input family's gallery pages, the Go twin of ref/src/bin/gallery/input_pages.rs,
// call for call: an input edited by keys, a multi-line
// textarea, the code editor without grammars, a number input, an OTP input, a masked input and
// the editor's completion menu filled by the app's provider (gk.EditorProviders). Each page has its own state, made once in init, so a page shows
// only what its own steps did.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

type inputPages struct {
	edit       gk.InputState
	text       gk.TextareaState
	editor     gk.EditorState
	number     gk.InputState
	otp        gk.OtpState
	masked     gk.InputState
	off        gk.InputState
	completion gk.EditorState
	asks       gk.RowSource // the completion editor's questions
	writes     gk.InputState
}

const editorText = "fn main() {\n    let x = 40 + 2;\n    println!(\"{x}\");\n}\n"

func inputPagesInit(a *App, cx *gk.Cx) {
	p := &a.input
	p.edit = gk.NewInputState(cx).Placeholder("Type here").Build()
	p.text = gk.NewTextareaState(cx).AutoGrow(2, 4).Placeholder("Notes").Build()
	p.editor = gk.NewEditorState(cx).Language("rust").DefaultValue(editorText).Build()
	p.editor.SetLineNumber(cx, true)
	p.number = gk.NewInputState(cx).Min(0).Max(10).Step(float64(2)).DefaultValue("4").Build()
	p.otp = gk.NewOtpState(cx, 4).Build()
	p.masked = gk.NewInputState(cx).Masked(true).DefaultValue("secret").Build()
	p.off = gk.NewInputState(cx).DefaultValue("disabled").Build()
	p.writes = gk.NewInputState(cx).DefaultValue("hello world").Build()
	p.completion = gk.NewEditorState(cx).Language("rust").Build()
	p.asks = gk.EditorProviders(cx, p.completion, a.completions,
		gk.ProvidersOptions{Which: gk.ProvideCompletions, Triggers: gk.Some(".")})
}

// completions answers the editor-completion page's provider: the two items the Rust page's
// CompletionProvider returns.
func (a *App) completions(cx *gk.Cx, e gk.ProviderEvent) {
	if _, ok := e.Request.(gk.ProviderCompletions); ok {
		items := []gk.CompletionItem{{Label: "first", Kind: 3, Detail: gk.Some("fn first()")}, {Label: "second", Kind: 6}}
		gk.AnswerCompletions(cx, a.input.asks, e.ID, gk.CompletionResponse{Items: items})
	}
}

// writeButton is one of the input-writes page's buttons: its writes to the state are buffered
// and applied after the handler, as the Rust page's update closures do them at once.
func writeButton(id, label string, h func(cx *gk.Cx, e gk.ClickEvent)) gk.ButtonEl {
	return gk.Button(id).Label(label).W(gk.Px(80)).OnClick(h)
}

var inputPageTable = []Page{
	{"input-edit", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(240)).Child(gk.Input(a.input.edit))
	}},
	{"textarea", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(240)).Child(gk.Textarea(a.input.text))
	}},
	{"editor", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(480)).H(gk.Px(160)).Child(gk.BaseEditor(a.input.editor))
	}},
	{"number-input", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(200)).Child(gk.NumberInput(a.input.number))
	}},
	{"otp-input", func(a *App, cx *gk.Cx) gk.Element {
		return gk.OtpInput(a.input.otp)
	}},
	{"input-writes", func(a *App, cx *gk.Cx) gk.Element {
		w := a.input.writes
		return gk.VFlex(s.Gap2).
			Child(gk.Div().W(gk.Px(240)).Child(gk.Input(w))).
			Child(gk.HFlex(s.Gap2).
				Child(writeButton("w-select", "Select", func(cx *gk.Cx, _ gk.ClickEvent) { w.SetSelectedRange(cx, gk.Range{Lo: 0, Hi: 5}) })).
				Child(writeButton("w-replace", "Replace", func(cx *gk.Cx, _ gk.ClickEvent) { w.Replace(cx, "HELLO") })).
				Child(writeButton("w-insert", "Insert", func(cx *gk.Cx, _ gk.ClickEvent) { w.Insert(cx, "!") })).
				Child(writeButton("w-set", "Set", func(cx *gk.Cx, _ gk.ClickEvent) { w.SetValue(cx, "reset") })))
	}},
	{"input-masked", func(a *App, cx *gk.Cx) gk.Element {
		return gk.VFlex(s.Gap2).
			Child(gk.Div().W(gk.Px(240)).Child(gk.Input(a.input.masked).MaskToggle())).
			Child(gk.Div().W(gk.Px(240)).Child(gk.Input(a.input.off).Disabled(true)))
	}},
	{"editor-completion", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(480)).Child(gk.CodeEditor(a.input.completion, gk.CodeEditorOptions{Height: 160}))
	}},
}
