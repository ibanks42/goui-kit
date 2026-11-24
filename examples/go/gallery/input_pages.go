// Input gallery pages: single-line inputs, auto-growing textareas, base and styled code
// editors, number and OTP inputs, programmatic selection/edit writes, masking, and completions.
package main

import (
	"github.com/ibanks42/goui-kit/gk"
	gkeditor "github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
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
	asks       gk.RowSource
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

func (a *App) completions(cx *gk.Cx, e gkeditor.ProviderEvent) {
	if _, ok := e.Request.(gkeditor.ProviderCompletions); ok {
		items := []gk.CompletionItem{{Label: "first", Kind: 3, Detail: gk.Some("fn first()")}, {Label: "second", Kind: 6}}
		gkeditor.AnswerCompletions(cx, a.input.asks, e.ID, gkeditor.CompletionResponse{Items: items})
	}
}

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
