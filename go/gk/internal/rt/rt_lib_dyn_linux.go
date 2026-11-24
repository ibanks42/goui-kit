//go:build !gkstatic

package rt

// Linux shared library filename and dynamic loader environment variable.
const (
	libName       = "libgouikit.so"
	loaderPathVar = "LD_LIBRARY_PATH"
)
