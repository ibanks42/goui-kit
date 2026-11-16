// A style token where a fill (a colour or a gradient) goes.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

func view(cx *gk.Cx) gk.Element { return gk.Div().Bg(s.Gap2) }

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
