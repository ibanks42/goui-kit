//go:build !gkstatic

package rt

// Linux shared library filename and dynamic loader environment variable.
const (
	libName       = "libgpuikit.so"
	loaderPathVar = "LD_LIBRARY_PATH"
)
