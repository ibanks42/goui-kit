//go:build !amd64 && !arm64

package goid

// Current returns 0 on architectures without an assembly reader; rt falls back to osThread.
func Current() uintptr { return 0 }
