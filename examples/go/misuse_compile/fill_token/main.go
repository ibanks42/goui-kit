// Compile-time misuse case: Bg accepts gk.Fill (a color or gradient), not a style token.
package main

import (
	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/s"
)

func view(cx *gk.Cx) gk.Element { return gk.Div().Bg(s.Gap2) }

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
