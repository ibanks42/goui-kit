//go:build !gkstatic

package rt

import "syscall"

// Windows DLL filename and loader search path variable.
const (
	libName       = "gouikit.dll"
	loaderPathVar = "PATH"
)

// libOpen loads a DLL via Win32 LoadLibrary.
func libOpen(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}

// libSym resolves an exported symbol from a loaded DLL via GetProcAddress.
func libSym(h uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(h), name)
}
