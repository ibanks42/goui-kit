//go:build !gkstatic && (linux || darwin)

package rt

import "github.com/ebitengine/purego"

func libOpen(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

func libSym(h uintptr, name string) (uintptr, error) { return purego.Dlsym(h, name) }
