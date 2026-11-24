// A handler of the wrong kind: a switch's OnClick hands a bool, not a ClickEvent (Go does
// not compile).
package main

import "gpuikit.dev/gk"

func view(cx *gk.Cx) gk.Element {
	return gk.Switch("sw").OnClick(func(cx *gk.Cx, e gk.ClickEvent) {})
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
