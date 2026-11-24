//go:build amd64 || arm64

package rt

import "gpuikit.dev/gk/internal/goid"

func curG() uintptr { return goid.Current() }
