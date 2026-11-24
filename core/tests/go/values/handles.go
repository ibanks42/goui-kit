// val-one-handle: an entity read back through the values path and the dock's path
// (gk.TestEntityHandles; Odin's gk.test_entity_handles).
package main

import (
	"fmt"

	"gpuikit.dev/gk"
)

func init() {
	modes["val-one-handle"] = &mode{click: once(oneHandle)}
}

// One table of handed-out entities: an entity read back through the values path and the dock's path
// gets the same handle (PROTOCOLS.md 13.16).
func oneHandle(a *App, cx *gk.Cx) {
	v, d, again, n := gk.TestEntityHandles(cx)
	fmt.Printf("values one handle same=%v again=%v distinct=%d nonzero=%v\n", v == d, v == again, n, v != 0)
}
