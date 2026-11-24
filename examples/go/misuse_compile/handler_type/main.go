// Compile-time misuse case: Switch.OnClick expects func(*gk.Cx, bool), not a ClickEvent handler.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
)

func view(cx *gk.Cx) gk.Element {
	return kit.Switch("sw").OnClick(func(cx *gk.Cx, e gk.ClickEvent) {})
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
