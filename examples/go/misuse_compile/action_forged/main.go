// Compile-time misuse case: gk.Action's id field is unexported so callers cannot forge actions.
package main

import (
	"github.com/ibanks42/goui-kit/gk"
)

func main() {
	gk.Run(func(cx *gk.Cx) {
		gk.DispatchAction(cx, gk.Action{id: 4242})
	})
}
