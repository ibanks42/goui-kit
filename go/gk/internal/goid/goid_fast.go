//go:build amd64 || arm64

// Package goid returns an opaque per-goroutine identity token for fast
// UI-thread checks inside the runtime package.
package goid

// Current returns the address of the calling goroutine's runtime g struct as
// a uintptr. Implemented in assembly on amd64 and arm64.
func Current() uintptr
