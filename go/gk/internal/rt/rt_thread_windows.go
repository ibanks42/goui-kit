package rt

import "syscall"

var getCurrentThreadID = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")

// osThread returns the calling Win32 thread ID via GetCurrentThreadId.
func osThread() int {
	id, _, _ := getCurrentThreadID.Call()
	return int(id)
}
