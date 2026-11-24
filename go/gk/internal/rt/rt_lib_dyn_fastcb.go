//go:build !gkstatic && (linux || darwin) && (amd64 || arm64)

package rt

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

// The turn callback without reflection. purego enters every callback through one Go function,
// held in its variable callbackWrap_call, which finds the Go function by index and calls it with
// reflect (a few microseconds a turn when the caches are cold, as they are between frames). This
// file puts a wrapper in front of it that calls dynCallback directly when the index is the turn
// callback's, and hands any other callback to purego's own. It depends on purego's internals
// (the variable, and its callbackArgs: the index, then the argument block of 8 float registers
// followed by the integer registers on amd64 and arm64), which are those of the pinned version;
// installFastCallback checks them by calling the callback once with a marker, and keeps purego's
// path when the marker does not arrive where it should.

//go:linkname puregoCallbackWrap github.com/ebitengine/purego.callbackWrap_call
var puregoCallbackWrap func(*cbArgs)

// cbArgs is purego's callbackArgs on amd64 and arm64 (syscall_notstackargs.go).
type cbArgs struct {
	index  uintptr
	args   unsafe.Pointer
	result [4]uintptr
}

const cbFloatRegs = 8 // float registers come first in the argument block

var (
	puregoWrap func(*cbArgs)
	turnIndex  = ^uintptr(0)
	probing    bool
)

func fastWrap(a *cbArgs) {
	if a.index == turnIndex {
		f := (*[cbFloatRegs + 5]uintptr)(a.args)
		a.result[0] = dynCallback(f[cbFloatRegs], f[cbFloatRegs+1], f[cbFloatRegs+2], f[cbFloatRegs+3], f[cbFloatRegs+4])
		return
	}
	if probing {
		if f := (*[cbFloatRegs + 5]uintptr)(a.args); f[cbFloatRegs] == cbMarker && f[cbFloatRegs+2] == ^uintptr(0)-1 {
			turnIndex = a.index
			a.result[0] = cbMarker
			return
		}
	}
	puregoWrap(a)
}

// installFastCallback puts fastWrap in front of purego's callback wrapper for the callback at
// addr, once its probe has found the callback's index. It runs once, before the library runs.
func installFastCallback(addr uintptr) {
	if puregoCallbackWrap == nil {
		return
	}
	puregoWrap = puregoCallbackWrap
	puregoCallbackWrap = fastWrap
	probing = true
	r, _, _ := purego.SyscallN(addr, cbMarker, 0, ^uintptr(0)-1, 0, 0)
	probing = false
	if r != cbMarker || turnIndex == ^uintptr(0) {
		turnIndex = ^uintptr(0)
		puregoCallbackWrap = puregoWrap // the layout is not the one this file knows: purego's path
	}
}
