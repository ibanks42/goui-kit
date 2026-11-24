package rt

import (
	"syscall"
)

// osThread returns the calling Linux kernel thread ID (TID).
func osThread() int { return syscall.Gettid() }
