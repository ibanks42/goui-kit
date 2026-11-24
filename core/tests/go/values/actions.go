package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
)

func init() {
	// Register builtin action deduplication and view-context rejection tests.
	modes["val-actions"] = &mode{click: once(actions)}
	modes["val-action-in-view"] = &mode{body: actionInView}
}

// actions proves gk.ActionOf returns the exact same built-in action ID for
// repeated calls with Cancel (same=true, builtin=true) and a distinct ID for
// SelectDown (other=true).
func actions(a *App, cx *gk.Cx) {
	x := gk.ActionOf(cx, gk.NewCancelDefault())
	y := gk.ActionOf(cx, gk.NewCancelDefault())
	z := gk.ActionOf(cx, gk.NewSelectDownDefault())
	fmt.Printf("values actions same=%v builtin=%v other=%v\n", x == y, x.IsBuiltin(), x != z)
}

// actionInView proves calling gk.ActionOf inside a view render callback is
// rejected by the runtime misuse guard because actions must be registered outside views.
func actionInView(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
	fmt.Println("values naming an action in a view")
	_ = gk.ActionOf(cx, gk.NewCancelDefault())
	return body
}
