// The misuse_values modes of the kit's action values, civil times and selection rules
// (PROTOCOLS.md 13.11, 13.13, 11.21).
//
//	GK_NEG=val-action-view  a kit action value named in a view function: must stop
//	                        (gk.ActionOf)
//	GK_NEG=val-bad-time     a time of day that does not exist (24:00) given to a time field:
//	                        must stop (gk.CivilTime and TimeFieldState.SetTime)
//	GK_NEG=c6-rules-source  selection rules given to a row source that is not a select's: must
//	                        stop (gk.SearchWillChange and gk.SearchRules)
//	GK_NEG=c6-rules-limits  selection rules whose min is above their max: must stop
package main

import "gpuikit.dev/gk"

func init() {
	pendingView["val-action-view"] = func(a *App, cx *gk.Cx) {
		_ = gk.ActionOf(cx, gk.NewCancelDefault())
	}
	pendingClick["val-bad-time"] = func(a *App, cx *gk.Cx) {
		tf := gk.NewTimeFieldState(cx).Build()
		tf.SetTime(cx, gk.CivilTime{Hour: 24})
	}
	pendingClick["c6-rules-source"] = func(a *App, cx *gk.Cx) {
		other := gk.NewRowSource(cx, 3, func(cx *gk.Cx, at gk.RowAt) gk.Element { return gk.Label("x") })
		gk.SearchWillChange(cx, other, gk.SearchRules{Max: 1})
	}
	pendingClick["c6-rules-limits"] = func(a *App, cx *gk.Cx) {
		src := gk.NewSearchSource(cx, []gk.SearchItem{{Value: "a", Title: "A"}})
		gk.SearchWillChange(cx, src, gk.SearchRules{Min: 3, Max: 1})
	}
}
