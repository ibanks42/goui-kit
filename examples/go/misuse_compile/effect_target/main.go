// An effect on an element no animation can wrap (gk.Text is a bare string, not a styled
// element): gk.Effect takes a gk.AnimatableElement, so the call does not compile.
package main

import (
	"time"

	"gpuikit.dev/gk"
)

func view(cx *gk.Cx) gk.Element {
	return gk.Effect(gk.Text("x"), gk.NewEffectTransition(200*time.Millisecond).Fade(0, 1), "in")
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
