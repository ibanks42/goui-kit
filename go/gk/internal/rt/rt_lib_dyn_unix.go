//go:build !gkstatic && (linux || darwin)

package rt

import "github.com/ebitengine/purego"

// libOpen opens a shared library on Unix with immediate symbol resolution and
// local visibility so its symbols do not pollute the global namespace.
func libOpen(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

// libSym resolves a symbol address from an open shared library handle.
func libSym(h uintptr, name string) (uintptr, error) { return purego.Dlsym(h, name) }
