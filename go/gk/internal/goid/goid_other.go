//go:build !amd64 && !arm64

// Package goid returns an opaque per-goroutine identity token for fast
// UI-thread checks inside the runtime package.
package goid

// Current returns 0 on architectures without an assembly g-register reader,
// signaling callers to fall back to OS thread ID checks.
func Current() uintptr { return 0 }
