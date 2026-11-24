//go:build !gkstatic && !((linux || darwin) && (amd64 || arm64))

package rt

// installFastCallback: purego's own callback path (Windows' callbacks are Go's syscall.NewCallback).
func installFastCallback(addr uintptr) {}
