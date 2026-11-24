package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
)

// providersInit attaches a `.`-triggered completion and hover provider to a.editor
// so typing "a." triggers an asynchronous completion request at byte offset 2.
func (a *App) providersInit(cx *gk.Cx) {
	a.asks = editor.Providers(cx, a.editor, a.provider, editor.ProvidersOptions{Triggers: gk.Some(".")})
}

// provider proves LSP completion requests deliver the request id, offset,
// trigger kind/character, and buffer text, and that AnswerCompletions inserts
// the selected "first" item back into the editor buffer ("afirst\n").
func (a *App) provider(cx *gk.Cx, e editor.ProviderEvent) {
	a.asked++
	switch q := e.Request.(type) {
	case editor.ProviderCompletions:
		trigger, _ := q.Trigger.Get()
		fmt.Printf("provider completions id=%d offset=%d kind=%d trigger=%q text=%q\n", e.ID, q.Offset, q.TriggerKind, trigger, q.Text)
		items := []gk.CompletionItem{{Label: "first", Kind: 3, Detail: gk.Some("fn first()")}, {Label: "second", Kind: 6}}
		editor.AnswerCompletions(cx, a.asks, e.ID, editor.CompletionResponse{Items: items})
	case editor.ProviderHover:
		fmt.Printf("provider hover id=%d offset=%d\n", e.ID, q.Offset)
		editor.AnswerHover(cx, a.asks, e.ID, gk.Some("**hover** text"))
	}
}
