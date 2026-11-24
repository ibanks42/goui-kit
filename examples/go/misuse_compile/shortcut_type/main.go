// A shortcut's handler of another shape: gk.Shortcut takes a func(cx) or a click handler, so a
// func(cx, bool) does not compile.
package main

import (
	"gpuikit.dev/gk"
)

func main() {
	gk.Run(func(cx *gk.Cx) {
		gk.Shortcut(cx, "ctrl-s", func(cx *gk.Cx, on bool) {})
	})
}
