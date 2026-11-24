// Compile-time misuse case: overlay.OpenDialog requires a Builder[overlay.DialogEl], not a LabelEl.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
)

func main() {
	gk.Run(func(cx *gk.Cx) {
		overlay.OpenDialog(cx, gk.NewBuilt(cx, func(cx *gk.Cx) kit.LabelEl { return kit.Label("not a dialog") }))
	})
}
