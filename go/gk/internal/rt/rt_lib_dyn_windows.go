//go:build !gkstatic

package rt

import "syscall"

const (
	libName       = "gpuikit.dll"
	loaderPathVar = "PATH"
)

// libOpen loads gpuikit.dll: by its full path, or by name with Windows' own search (the program's
// folder, the system folders, then PATH).
func libOpen(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}

func libSym(h uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(h), name)
}
