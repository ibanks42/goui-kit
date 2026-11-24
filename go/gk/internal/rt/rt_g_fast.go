//go:build amd64 || arm64

package rt

import "github.com/ibanks42/goui-kit/gk/internal/goid"

// curG returns the current goroutine's g pointer via assembly so UI-thread
// assertions avoid making an OS syscall on every call.
func curG() uintptr { return goid.Current() }
