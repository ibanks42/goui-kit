// Compile-time misuse case: gk.Shortcut only accepts func(*gk.Cx) or func(*gk.Cx, gk.ClickEvent).
package main

import (
	"gpuikit.dev/gk"
)

func main() {
	gk.Run(func(cx *gk.Cx) {
		gk.Shortcut(cx, "ctrl-s", func(cx *gk.Cx, on bool) {})
	})
}
