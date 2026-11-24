// The providers mode: the editor's providers on the styled code editor.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
)

func (a *App) providersInit(cx *gk.Cx) {
	a.asks = editor.Providers(cx, a.editor, a.provider, editor.ProvidersOptions{Triggers: gk.Some(".")})
}

// The editor's providers: every question arrives here, after the editor's own update, and is
// answered at once (it could be later: the menu waits for the answer).
func (a *App) provider(cx *gk.Cx, e editor.ProviderEvent) {
	a.asked++
	switch q := e.Request.(type) { // the other providers are core/tests/go/lists2's (mode providers)
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
