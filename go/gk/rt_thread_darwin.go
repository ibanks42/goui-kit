package gk

/*
#include <pthread.h>
#include <stdint.h>
static uint64_t gk_os_thread(void) { uint64_t t = 0; pthread_threadid_np(NULL, &t); return t; }
*/
import "C"

// osThread is the calling OS thread's id: every call that takes a Cx checks it against the UI
// thread's (only gk.Post may be called from another goroutine). One file per platform; macOS
// has no gettid, so the id is pthread_threadid_np's.
func osThread() int { return int(C.gk_os_thread()) }

// How messages name the library and what finds it at run time (one set per platform).
const (
	libFile  = "libgpuikit.dylib"
	libFind  = "DYLD_LIBRARY_PATH and the rpath the app was built with"
	libPoint = "the rpath or DYLD_LIBRARY_PATH"
	libFirst = "first in DYLD_LIBRARY_PATH"
)
