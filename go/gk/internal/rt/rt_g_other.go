//go:build !amd64 && !arm64

package rt

// curG falls back to the OS thread id on architectures without an assembly reader.
func curG() uintptr { return uintptr(osThread()) }
