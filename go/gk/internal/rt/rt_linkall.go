//go:build gkstatic

// Static family link-all escape hatch.
// Under -tags gkstatic, each component package registers only its own family
// via gen_use.go so the linker strips unused Rust component code. Calling
// LinkAll references gk_use_all and forces every component family into the
// binary (used by tests and tools that exercise the full library).

package rt

// extern void gk_use_all(void);
import "C"

// LinkAll forces the static linker to include every component family in
// libgpuikit_static.a.
func LinkAll() { C.gk_use_all() }
