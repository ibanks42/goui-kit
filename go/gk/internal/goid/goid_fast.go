//go:build amd64 || arm64

// Package goid reads the current goroutine pointer in one instruction so
// rt.CheckCx can verify UI-thread affinity without a syscall (and in a
// separate package from rt so cgo static builds pass go vet).
package goid

// Current returns the current goroutine's pointer.
func Current() uintptr
