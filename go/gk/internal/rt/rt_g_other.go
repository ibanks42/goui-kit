//go:build !amd64 && !arm64

package rt

// curG falls back to the OS thread ID on architectures without goid assembly.
func curG() uintptr { return uintptr(osThread()) }
