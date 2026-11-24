//go:build !gkstatic && (linux || darwin) && (amd64 || arm64)

package rt

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

// puregoCallbackWrap links to purego's internal callbackWrap_call function
// variable. By default, purego unpacks callback arguments using reflect.Call on
// every C-to-Go transition; replacing this dispatcher lets the hot UI turn
// callback read the saved integer argument registers directly.
//
//go:linkname puregoCallbackWrap github.com/ebitengine/purego.callbackWrap_call
var puregoCallbackWrap func(*cbArgs)

// cbArgs mirrors purego's internal callbackArgs struct passed from assembly:
// index identifies the registered callback slot, args points to the saved
// register spill area, and result holds up to four return words.
type cbArgs struct {
	index  uintptr
	args   unsafe.Pointer
	result [4]uintptr
}

// Number of floating-point register save slots (XMM0..XMM7 on amd64, V0..V7 on
// arm64) that precede the integer argument registers in purego's spill frame.
const cbFloatRegs = 8

var (
	puregoWrap func(*cbArgs)
	turnIndex  = ^uintptr(0)
	probing    bool
)

// fastWrap intercepts purego callback dispatch. Once turnIndex is confirmed by
// installFastCallback, calls for that index read the five integer arguments
// directly from a.args[cbFloatRegs : cbFloatRegs+5] and call dynCallback
// without reflection. All other callbacks fall through to puregoWrap.
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

// installFastCallback wraps puregoCallbackWrap and performs a self-verifying
// probe call to addr with sentinel arguments (cbMarker, 0, ^uintptr(0)-1, 0, 0).
// If purego's internal register layout ever changes, the probe fails to match
// and installFastCallback cleanly restores purego's original dispatcher.
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
		puregoCallbackWrap = puregoWrap
	}
}
