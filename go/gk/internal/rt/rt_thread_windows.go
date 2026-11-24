package rt

/*
#include <windows.h>
static unsigned long gk_os_thread(void) { return GetCurrentThreadId(); }
*/
import "C"

// osThread is the calling OS thread's id: every call that takes a Cx checks it against the UI
// thread's (only gk.Post may be called from another goroutine). One file per platform; the
// syscall package has no GetCurrentThreadId and the module has no dependencies, so it is
// called through cgo, which package gk needs anyway.
func osThread() int { return int(C.gk_os_thread()) }

// How messages name the library and what finds it at run time (one set per platform): Windows
// loads gpuikit.dll from the program's folder first, then from PATH.
const (
	libFile  = "gpuikit.dll"
	libFind  = "which gpuikit.dll is in the program's folder (or first on PATH)"
	libPoint = "PATH (no other gpuikit.dll in the program's folder)"
	libFirst = "in the program's folder (or first on PATH)"
)
