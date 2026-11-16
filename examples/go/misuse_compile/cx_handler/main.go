// A handler without the Cx: every handler takes the turn's *gk.Cx first.
package main

import "gpuikit.dev/gk"

func view(cx *gk.Cx) gk.Element {
	return gk.Button("b").OnClick(func(e gk.ClickEvent) {})
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
