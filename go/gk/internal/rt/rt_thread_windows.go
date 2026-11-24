package rt

import "syscall"

var getCurrentThreadID = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")

// osThread is the calling OS thread's id: every call that takes a Cx checks it against the UI
// thread's (only gk.Post may be called from another goroutine). One file per platform; the
// syscall package has no GetCurrentThreadId, so it is called from kernel32 (no cgo, in both
// builds).
func osThread() int {
	id, _, _ := getCurrentThreadID.Call()
	return int(id)
}
