// Step 1 of the Go tutorial: open a single window and draw static labels.
package main

import (
	"log"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

// view describes the window's root element tree.
// Style tokens (package s) are passed variadically to layout containers and labels;
// semantic colors (package theme) resolve dynamically against the active theme.
func view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Label("Nothing to buy yet"))
}

func main() {
	// gk.Run locks this goroutine to the UI OS thread and enters GPUI's event loop.
	// The start callback runs once in the INIT turn to open the initial window.
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(nil, view))
	}); err != nil {
		log.Fatal(err)
	}
}
