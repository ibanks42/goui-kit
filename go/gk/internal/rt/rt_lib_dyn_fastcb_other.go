//go:build !gkstatic && !((linux || darwin) && (amd64 || arm64))

package rt

// installFastCallback is a no-op on targets without a fast purego callback
// wrapper; those platforms use purego's standard callback dispatcher.
func installFastCallback(addr uintptr) {}
