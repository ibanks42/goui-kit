// Step 1 of the Go tutorial: a window with a view.
package main

import (
	"log"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

func view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Label("Nothing to buy yet"))
}

func main() {
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(nil, view))
	}); err != nil {
		log.Fatal(err)
	}
}
