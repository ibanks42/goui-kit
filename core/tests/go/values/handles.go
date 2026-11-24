package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
)

func init() {
	// Register the entity-handle deduplication check (`gk.TestEntityHandles`).
	modes["val-one-handle"] = &mode{click: once(oneHandle)}
}

// oneHandle proves looking up the same GPUI entity via multiple paths yields
// one deduplicated non-zero handle (same=true, again=true, distinct=1, nonzero=true).
func oneHandle(a *App, cx *gk.Cx) {
	v, d, again, n := gk.TestEntityHandles(cx)
	fmt.Printf("values one handle same=%v again=%v distinct=%d nonzero=%v\n", v == d, v == again, n, v != 0)
}
