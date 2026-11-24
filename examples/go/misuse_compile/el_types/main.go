// Compile-time misuse case: element types are distinct structs; a ButtonEl is not a LabelEl.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
)

func view(cx *gk.Cx) gk.Element {
	var l kit.LabelEl = kit.Button("b")
	return l
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
