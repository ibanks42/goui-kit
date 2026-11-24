// The providers mode: the editor's providers (Odin's kit_input.odin: editor_providers,
// Provider_Event, Completion_Response, Hover_Response) on the styled code editor.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
)

func (a *App) providersInit(cx *gk.Cx) {
	a.asks = gk.EditorProviders(cx, a.editor, a.provider, gk.ProvidersOptions{Triggers: gk.Some(".")})
}

// The editor's providers: every question arrives here, after the editor's own update, and is
// answered at once (it could be later: the menu waits for the answer).
func (a *App) provider(cx *gk.Cx, e gk.ProviderEvent) {
	a.asked++
	switch q := e.Request.(type) { // the other providers are core/tests/go/lists2's (mode providers)
	case gk.ProviderCompletions:
		trigger, _ := q.Trigger.Get()
		fmt.Printf("provider completions id=%d offset=%d kind=%d trigger=%q text=%q\n", e.ID, q.Offset, q.TriggerKind, trigger, q.Text)
		items := []gk.CompletionItem{{Label: "first", Kind: 3, Detail: gk.Some("fn first()")}, {Label: "second", Kind: 6}}
		gk.AnswerCompletions(cx, a.asks, e.ID, gk.CompletionResponse{Items: items})
	case gk.ProviderHover:
		fmt.Printf("provider hover id=%d offset=%d\n", e.ID, q.Offset)
		gk.AnswerHover(cx, a.asks, e.ID, gk.Some("**hover** text"))
	}
}
