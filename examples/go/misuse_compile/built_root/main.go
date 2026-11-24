// Compile-time misuse case: overlay.OpenDialog requires a Builder[overlay.DialogEl], not a LabelEl.
package main

import (
	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
)

func main() {
	gk.Run(func(cx *gk.Cx) {
		overlay.OpenDialog(cx, gk.NewBuilt(cx, func(cx *gk.Cx) kit.LabelEl { return kit.Label("not a dialog") }))
	})
}
