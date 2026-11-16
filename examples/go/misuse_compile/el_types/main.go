// A button where a label is expected: element types are distinct.
package main

import "gpuikit.dev/gk"

func view(cx *gk.Cx) gk.Element {
	var l gk.LabelEl = gk.Button("b")
	return l
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
