// val-actions and val-action-in-view: the kit's own action values named as actions (gk.ActionOf).
package main

import (
	"fmt"

	"gpuikit.dev/gk"
)

func init() {
	modes["val-actions"] = &mode{click: once(actions)}
	modes["val-action-in-view"] = &mode{body: actionInView}
}

// The kit's own action values as actions: the same value gives the same id, a built-in one.
func actions(a *App, cx *gk.Cx) {
	x := gk.ActionOf(cx, gk.NewCancelDefault())
	y := gk.ActionOf(cx, gk.NewCancelDefault())
	z := gk.ActionOf(cx, gk.NewSelectDownDefault())
	fmt.Printf("values actions same=%v builtin=%v other=%v\n", x == y, x.IsBuiltin(), x != z)
}

// Misuse: naming an action in a view function stops at its line (exit 2).
func actionInView(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
	fmt.Println("values naming an action in a view")
	_ = gk.ActionOf(cx, gk.NewCancelDefault())
	return body
}
