// A view-built closure whose function describes something other than a dialog, given to
// gk.OpenDialog (Go does not compile:
// gk.NewBuilt over a func returning a gk.LabelEl is a gk.Built[gk.LabelEl]).
package main

import "gpuikit.dev/gk"

func main() {
	gk.Run(func(cx *gk.Cx) {
		gk.OpenDialog(cx, gk.NewBuilt(cx, func(cx *gk.Cx) gk.LabelEl { return gk.Label("not a dialog") }))
	})
}
