//go:build !gkstatic

package rt

import (
	"sync"

	"github.com/ebitengine/purego"
)

// macOS shared library filename and dynamic loader environment variable.
const (
	libName       = "libgpuikit.dylib"
	loaderPathVar = "DYLD_LIBRARY_PATH"
)

var (
	pthreadSelf     uintptr
	pthreadSelfOnce sync.Once
)

// osThread returns the calling thread's pthread handle via libSystem when cgo
// is disabled on macOS.
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
