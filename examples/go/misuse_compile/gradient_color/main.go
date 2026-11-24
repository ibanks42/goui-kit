// Compile-time misuse case: TextColor expects a single gk.Color, so a LinearGradient is rejected.
package main

import (
	"gpuikit.dev/gk"
)

func view(cx *gk.Cx) gk.Element {
	g := gk.NewLinearGradient(90, gk.NewLinearColorStop(gk.HSLA(0.5, 1, 0.5, 1), 0), gk.NewLinearColorStop(gk.HSLA(0.8, 1, 0.5, 1), 1))
	return gk.Div().TextColor(g)
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
