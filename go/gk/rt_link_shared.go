//go:build gkshared

package gk

// Linked against the shared library (build tag gkshared), which holds every component family:
// register them all. Without the tag a program links the static library, libgpuikit_static.a, and
// keeps only the families it calls into (docs/GO.md section 8).

func init() { LinkAll() }
