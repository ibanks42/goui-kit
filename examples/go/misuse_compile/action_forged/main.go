// An action id the library never made (in Go an Action's id is unexported, so it cannot be
// forged: only gk.NewAction makes one).
package main

import "gpuikit.dev/gk"

func main() {
	gk.Run(func(cx *gk.Cx) {
		gk.DispatchAction(cx, gk.Action{id: 4242})
	})
}
