// Compile-time misuse case: LabelEl does not implement interactive builders such as OnClick.
package main

import (
	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/kit"
)

func view(cx *gk.Cx) gk.Element {
	return kit.Label("x").OnClick(func(cx *gk.Cx, e gk.ClickEvent) {})
}

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
