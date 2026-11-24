// Compile-time misuse case: Child requires a gk.Element, rejecting arbitrary values like int.
package main

import (
	"gpuikit.dev/gk"
)

func view(cx *gk.Cx) gk.Element { return gk.Div().Child(42) }

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
