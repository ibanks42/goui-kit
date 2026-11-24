//go:build gkstatic

package rt

// The static way to the library (build tag gkstatic): libgpuikit_static.a is linked into the
// program through cgo, and only the component families the program calls into are kept
// (gen_use.go; docs/GO.md section 8). The C half of the callback is in capi.c.

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo linux LDFLAGS: -lgpuikit_static -lgpuikit_sys -ldl -lgcc_s -lutil -lrt -lpthread -lm -lc
#cgo darwin LDFLAGS: -lgpuikit_static -framework CoreServices -framework ApplicationServices -framework CoreFoundation -framework CoreVideo -framework Carbon -framework AppKit -framework Foundation -framework UserNotifications -framework Security -framework CoreText -framework CoreGraphics -framework CoreData -framework CoreImage -framework CloudKit -framework QuartzCore -framework UniformTypeIdentifiers -framework Metal -lobjc -liconv -lc -lm
#cgo windows LDFLAGS: -lgpuikit_static -lgpuikit_sys -lkernel32 -lntdll -luserenv -lws2_32 -ldbghelp
#include <stdlib.h>
#include "gpuikit.h"
int32_t gk_go_run(int headless, const uint8_t *script, size_t len, uint16_t major, uint16_t minor,
                  uint64_t hash, uint32_t size_delta, int no_callback);
*/
import "C"

import (
	"unsafe"
)

const (
	gkMagic  = uint32(C.GK_MAGIC)
	infoSize = int(unsafe.Sizeof(C.gk_info{}))
	// How messages name the library and where it came from.
	libFile  = "static library"
	libFind  = "the libgpuikit_static.a the program was linked with (the lib/ folder CGO_LDFLAGS named)"
	libPoint = "CGO_LDFLAGS"
	libFirst = "first: link the program with the diagnostic library's libgpuikit_static.a (lib/diag in CGO_LDFLAGS)"
)

// loadLibrary has nothing to do: the library is in the program.
func loadLibrary() error { return nil }

func readInfo() (rawInfo, bool) {
	i := C.gk_info_get()
	if i == nil {
		return rawInfo{}, false
	}
	return rawInfo{
		size: uint32(i.size), magic: uint32(i.magic), abiMajor: uint16(i.abi_major), abiMinor: uint16(i.abi_minor),
		apiHash: uint64(i.api_hash), features: uint32(i.features),
		versions: C.GoStringN(i.versions, C.int(i.versions_len)),
	}, true
}

func lastError() string {
	var n C.size_t
	p := C.gk_last_error(&n)
	if p == nil {
		return ""
	}
	return C.GoStringN(p, C.int(n))
}

// call makes one gk_call. The request is Go memory lent for the call (it holds no Go pointers);
// the result is the library's and valid until the next gk_call, so it is copied out at once.
func call(req []byte) (int32, []byte) {
	var res *C.uint8_t
	var n C.size_t
	rc := C.gk_call((*C.uint8_t)(unsafe.Pointer(&req[0])), C.size_t(len(req)), &res, &n)
	if rc != 0 || n == 0 {
		return int32(rc), nil
	}
	return 0, C.GoBytes(unsafe.Pointer(res), C.int(n))
}

//export gkGoCallback
func gkGoCallback(user unsafe.Pointer, cb *C.gk_cb, kind C.uint32_t, req *C.uint8_t, n C.size_t, out **C.uint8_t, outN *C.size_t) C.int32_t {
	var in []byte
	if n > 0 {
		in = unsafe.Slice((*byte)(unsafe.Pointer(req)), int(n)) // the library's, valid during this call
	}
	if r := onTurn(uint32(kind), in); len(r) > 0 {
		// The C side passes it to gk_reply when this returns (capi.c), so the turn costs one
		// crossing into Go and none back out.
		*out, *outN = (*C.uint8_t)(unsafe.Pointer(&r[0])), C.size_t(len(r))
	}
	return 0
}

// replyMem is the C memory a turn's reply is written into (cgo's rules forbid handing Go memory
// to C after the call returns); it grows as needed and is never freed.
var (
	replyMem unsafe.Pointer
	replyCap int
)

// replyBuf is an empty slice over C memory of at least n bytes.
func replyBuf(n int) Tape {
	if n > replyCap {
		c := max(n, 2*replyCap, 4096)
		replyMem = C.realloc(replyMem, C.size_t(c))
		if replyMem == nil {
			replyCap = 0
			Stop("out of memory for a turn's reply (%d bytes)", c)
		}
		replyCap = c
	}
	return unsafe.Slice((*byte)(replyMem), replyCap)[:0]
}

// libRun enters the library (gk_run, or gk_headless_run with a script when headless). The
// script is C memory: the library reads it for the whole run. sizeDelta and noCallback exist
// only for the runtime's version test, which shows the library refusing a wrong gk_app.
func libRun(headless bool, script string, major, minor uint16, hash uint64, sizeDelta uint32, noCallback bool) int32 {
	var p *C.uint8_t
	h := C.int(0)
	if headless {
		h = 1
		p = (*C.uint8_t)(C.CBytes([]byte(script + "\x00")))
		defer C.free(unsafe.Pointer(p))
	}
	nc := C.int(0)
	if noCallback {
		nc = 1
	}
	return int32(C.gk_go_run(h, p, C.size_t(len(script)), C.uint16_t(major), C.uint16_t(minor), C.uint64_t(hash), C.uint32_t(sizeDelta), nc))
}

func postWake() { C.gk_post(0) }
