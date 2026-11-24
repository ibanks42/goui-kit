// A handler without the Cx: every handler takes the turn's *gk.Cx first.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
)

func view(cx *gk.Cx) gk.Element {
	return kit.Button("b").OnClick(func(e gk.ClickEvent) {})
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
