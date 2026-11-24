// A builder the element does not have: a Label has no OnClick (upstream's Label is not
// interactive), so the call does not compile instead of being dropped by the library.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
)

func view(cx *gk.Cx) gk.Element {
	return kit.Label("x").OnClick(func(cx *gk.Cx, e gk.ClickEvent) {})
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
