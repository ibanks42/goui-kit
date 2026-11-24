// Additional misuse cases for kit action registration in views, invalid civil times,
// and SearchRules validation errors.
package main

import (
	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/base"
	"github.com/ibanks42/goui-kit/gk/kit"
)

func init() {
	pendingView["val-action-view"] = func(a *App, cx *gk.Cx) {
		_ = gk.ActionOf(cx, gk.NewCancelDefault())
	}
	pendingClick["val-bad-time"] = func(a *App, cx *gk.Cx) {
		tf := base.NewTimeFieldState(cx).Build()
		tf.SetTime(cx, gk.CivilTime{Hour: 24})
	}
	pendingClick["c6-rules-source"] = func(a *App, cx *gk.Cx) {
		other := gk.NewRowSource(cx, 3, func(cx *gk.Cx, at gk.RowAt) gk.Element { return kit.Label("x") })
		gk.SearchWillChange(cx, other, gk.SearchRules{Max: 1})
	}
	pendingClick["c6-rules-limits"] = func(a *App, cx *gk.Cx) {
		src := gk.NewSearchSource(cx, []gk.SearchItem{{Value: "a", Title: "A"}})
		gk.SearchWillChange(cx, src, gk.SearchRules{Min: 3, Max: 1})
	}
}
