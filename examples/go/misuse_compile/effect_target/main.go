// Compile-time misuse case: kit.Effect only accepts AnimatableElement values, not bare gk.Text.
package main

import (
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/motion"
)

func view(cx *gk.Cx) gk.Element {
	return kit.Effect(gk.Text("x"), motion.NewEffectTransition(200*time.Millisecond).Fade(0, 1), "in")
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
