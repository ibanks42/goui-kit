//go:build gkstatic

package rt

/*
#include <pthread.h>
#include <stdint.h>
static uint64_t gk_os_thread(void) { uint64_t t = 0; pthread_threadid_np(NULL, &t); return t; }
*/
import "C"

// osThread returns the calling thread's macOS kernel thread ID via
// pthread_threadid_np in static cgo builds.
func osThread() int { return int(C.gk_os_thread()) }
