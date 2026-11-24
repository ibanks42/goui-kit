// Compile-time misuse case: builder values like StyleRefinement do not implement gk.Element.
package main

import (
	"github.com/ibanks42/goui-kit/gk"
)

func view(cx *gk.Cx) gk.Element { return gk.Div().Child(gk.NewStyleRefinementDefault()) }

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
