// A value that is not an element (a style refinement) given as a child.
package main

import "gpuikit.dev/gk"

func view(cx *gk.Cx) gk.Element { return gk.Div().Child(gk.NewStyleRefinementDefault()) }

func main() { gk.Run(func(cx *gk.Cx) { gk.OpenWindow(cx, "x", 100, 100, gk.NewView(nil, view)) }) }
