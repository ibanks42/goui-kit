//go:build !gkstatic

package rt

import (
	"sync"

	"github.com/ebitengine/purego"
)

const (
	libName       = "libgpuikit.dylib"
	loaderPathVar = "DYLD_LIBRARY_PATH"
)

// pthreadSelf is libSystem's pthread_self, found the first time osThread runs.
var (
	pthreadSelf     uintptr
	pthreadSelfOnce sync.Once
)

// osThread identifies the calling OS thread: every call that takes a Cx checks it against the UI
// thread's (only gk.Post may be called from another goroutine). macOS has no gettid; a running
// thread's pthread_self is unique among the running threads, which is all the check needs, and
// takes no out-parameter (pthread_threadid_np, which the static build calls, does).
func osThread() int {
	pthreadSelfOnce.Do(func() {
		if h, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			pthreadSelf, _ = purego.Dlsym(h, "pthread_self")
		}
	})
	if pthreadSelf == 0 {
		return 0
	}
	t, _, _ := purego.SyscallN(pthreadSelf)
	return int(t)
}
