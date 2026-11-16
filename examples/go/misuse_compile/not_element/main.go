// A number where an element goes.
package main

import "gpuikit.dev/gk"

func view(cx *gk.Cx) gk.Element { return gk.Div().Child(42) }

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
