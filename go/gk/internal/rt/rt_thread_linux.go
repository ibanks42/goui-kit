package rt

import (
	"syscall"
)

// osThread is the calling OS thread's id: every call that takes a Cx checks it against the UI
// thread's (only gk.Post may be called from another goroutine). One file per platform (macOS:
// rt_thread_darwin.go with gkstatic, rt_lib_dyn_darwin.go without).
func osThread() int { return syscall.Gettid() }
