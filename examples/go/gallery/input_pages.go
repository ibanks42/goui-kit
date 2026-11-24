// The input family's gallery pages, the Go twin of ref/src/bin/gallery/input_pages.rs,
// call for call: an input edited by keys, a multi-line
// textarea, the code editor without grammars, a number input, an OTP input, a masked input and
// the editor's completion menu filled by the app's provider (gk.EditorProviders). Each page has its own state, made once in init, so a page shows
// only what its own steps did.
package main

import (
	"gpuikit.dev/gk"
	gkeditor "gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
)

type inputPages struct {
	edit       gkeditor.InputState
	text       gkeditor.TextareaState
	editor     gkeditor.State
	number     gkeditor.InputState
	otp        gkeditor.OtpState
	masked     gkeditor.InputState
	off        gkeditor.InputState
	completion gkeditor.State
	asks       gk.RowSource // the completion editor's questions
	writes     gkeditor.InputState
}

const editorText = "fn main() {\n    let x = 40 + 2;\n    println!(\"{x}\");\n}\n"

func inputPagesInit(a *App, cx *gk.Cx) {
	p := &a.input
	p.edit = gkeditor.NewInputState(cx).Placeholder("Type here").Build()
	p.text = gkeditor.NewTextareaState(cx).AutoGrow(2, 4).Placeholder("Notes").Build()
	p.editor = gkeditor.NewState(cx).Language("rust").DefaultValue(editorText).Build()
	p.editor.SetLineNumber(cx, true)
	p.number = gkeditor.NewInputState(cx).Min(0).Max(10).Step(float64(2)).DefaultValue("4").Build()
	p.otp = gkeditor.NewOtpState(cx, 4).Build()
	p.masked = gkeditor.NewInputState(cx).Masked(true).DefaultValue("secret").Build()
	p.off = gkeditor.NewInputState(cx).DefaultValue("disabled").Build()
	p.writes = gkeditor.NewInputState(cx).DefaultValue("hello world").Build()
	p.completion = gkeditor.NewState(cx).Language("rust").Build()
	p.asks = gkeditor.Providers(cx, p.completion, a.completions,
		gkeditor.ProvidersOptions{Which: gkeditor.ProvideCompletions, Triggers: gk.Some(".")})
}

// completions answers the editor-completion page's provider: the two items the Rust page's
// CompletionProvider returns.
func (a *App) completions(cx *gk.Cx, e gkeditor.ProviderEvent) {
	if _, ok := e.Request.(gkeditor.ProviderCompletions); ok {
		items := []gk.CompletionItem{{Label: "first", Kind: 3, Detail: gk.Some("fn first()")}, {Label: "second", Kind: 6}}
		gkeditor.AnswerCompletions(cx, a.input.asks, e.ID, gkeditor.CompletionResponse{Items: items})
	}
}

// writeButton is one of the input-writes page's buttons: its writes to the state are buffered
// and applied after the handler, as the Rust page's update closures do them at once.
func writeButton(id, label string, h func(cx *gk.Cx, e gk.ClickEvent)) kit.ButtonEl {
	return kit.Button(id).Label(label).W(gk.Px(80)).OnClick(h)
}

var inputPageTable = []Page{
	{"input-edit", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(240)).Child(gkeditor.Input(a.input.edit))
	}},
	{"textarea", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(240)).Child(gkeditor.Textarea(a.input.text))
	}},
	{"editor", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(480)).H(gk.Px(160)).Child(gkeditor.BaseEditor(a.input.editor))
	}},
	{"number-input", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(200)).Child(gkeditor.NumberInput(a.input.number))
	}},
	{"otp-input", func(a *App, cx *gk.Cx) gk.Element {
		return gkeditor.OtpInput(a.input.otp)
	}},
	{"input-writes", func(a *App, cx *gk.Cx) gk.Element {
		w := a.input.writes
		return gk.VFlex(s.Gap2).
			Child(gk.Div().W(gk.Px(240)).Child(gkeditor.Input(w))).
			Child(gk.HFlex(s.Gap2).
				Child(writeButton("w-select", "Select", func(cx *gk.Cx, _ gk.ClickEvent) { w.SetSelectedRange(cx, gk.Range{Lo: 0, Hi: 5}) })).
				Child(writeButton("w-replace", "Replace", func(cx *gk.Cx, _ gk.ClickEvent) { w.Replace(cx, "HELLO") })).
				Child(writeButton("w-insert", "Insert", func(cx *gk.Cx, _ gk.ClickEvent) { w.Insert(cx, "!") })).
				Child(writeButton("w-set", "Set", func(cx *gk.Cx, _ gk.ClickEvent) { w.SetValue(cx, "reset") })))
	}},
	{"input-masked", func(a *App, cx *gk.Cx) gk.Element {
		return gk.VFlex(s.Gap2).
			Child(gk.Div().W(gk.Px(240)).Child(gkeditor.Input(a.input.masked).MaskToggle())).
			Child(gk.Div().W(gk.Px(240)).Child(gkeditor.Input(a.input.off).Disabled(true)))
	}},
	{"editor-completion", func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(480)).Child(gkeditor.CodeEditor(a.input.completion, gkeditor.CodeEditorOptions{Height: 160}))
	}},
}
