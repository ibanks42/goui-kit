package gk

import "syscall"

// osThread is the calling OS thread's id: every call that takes a Cx checks it against the UI
// thread's (only gk.Post may be called from another goroutine). One file per platform.
func osThread() int { return syscall.Gettid() }

// How messages name the library and what finds it at run time (one set per platform).
const (
	libFile  = "libgpuikit.so"
	libFind  = "LD_LIBRARY_PATH and the rpath the app was built with"
	libPoint = "the rpath or LD_LIBRARY_PATH"
	libFirst = "first in LD_LIBRARY_PATH"
)
