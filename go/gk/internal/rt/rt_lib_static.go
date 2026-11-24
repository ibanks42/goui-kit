//go:build gkstatic

package rt

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo linux LDFLAGS: -lgouikit_static -lgouikit_sys -ldl -lgcc_s -lutil -lrt -lpthread -lm -lc
#cgo darwin LDFLAGS: -lgouikit_static -framework CoreServices -framework ApplicationServices -framework CoreFoundation -framework CoreVideo -framework Carbon -framework AppKit -framework Foundation -framework UserNotifications -framework Security -framework CoreText -framework CoreGraphics -framework CoreData -framework CoreImage -framework CloudKit -framework QuartzCore -framework UniformTypeIdentifiers -framework Metal -lobjc -liconv -lc -lm
#cgo windows LDFLAGS: -lgouikit_static -lgouikit_sys -lkernel32 -lntdll -luserenv -lws2_32 -ldbghelp
#include <stdlib.h>
#include "gouikit.h"
int32_t gk_go_run(int headless, const uint8_t *script, size_t len, uint16_t major, uint16_t minor,
                  uint64_t hash, uint32_t size_delta, int no_callback);
*/
import "C"

import (
	"unsafe"
)

// Static-link backend constants matching the C header and linker setup.
const (
	gkMagic  = uint32(C.GK_MAGIC)
	infoSize = int(unsafe.Sizeof(C.gk_info{}))
	libFile  = "static library"
	libFind  = "the libgouikit_static.a the program was linked with (the lib/ folder CGO_LDFLAGS named)"
	libPoint = "CGO_LDFLAGS"
	libFirst = "first: link the program with the diagnostic library's libgouikit_static.a (lib/diag in CGO_LDFLAGS)"
)

// loadLibrary is a no-op because libgouikit_static.a is linked at build time.
func loadLibrary() error { return nil }

// readInfo fetches the library metadata struct exported by gk_info_get.
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

// lastError copies the most recent error message stored by the Rust core.
func lastError() string {
	var n C.size_t
	p := C.gk_last_error(&n)
	if p == nil {
		return ""
	}
	return C.GoStringN(p, C.int(n))
}

// call sends an encoded request tape to gk_call and copies any response bytes
// into a Go slice. Calls made from an inline event during view building abort
// the inline fast path so the event can run on a full turn.
func call(req []byte) (int32, []byte) {
	if Vb.Active {
		if cur != nil && cur.inInlineEvent {
			panic(abortInline{})
		}
		Vb.calledLib = true
	}
	var res *C.uint8_t
	var n C.size_t
	rc := C.gk_call((*C.uint8_t)(unsafe.Pointer(&req[0])), C.size_t(len(req)), &res, &n)
	if rc != 0 || n == 0 {
		return int32(rc), nil
	}
	return 0, C.GoBytes(unsafe.Pointer(res), C.int(n))
}

// gkGoCallback is invoked by go_callback in capi.c for every turn coming from
// the Rust core. It wraps the request buffer without copying, runs onTurn, and
// returns a pointer to the C-allocated reply buffer via out and outN.
//
//export gkGoCallback
func gkGoCallback(user unsafe.Pointer, cb *C.gk_cb, kind C.uint32_t, req *C.uint8_t, n C.size_t, out **C.uint8_t, outN *C.size_t) C.int32_t {
	var in []byte
	if n > 0 {
		in = unsafe.Slice((*byte)(unsafe.Pointer(req)), int(n))
	}
	if r := onTurn(uint32(kind), in); len(r) > 0 {
		*out, *outN = (*C.uint8_t)(unsafe.Pointer(&r[0])), C.size_t(len(r))
	}
	return 0
}

// C-heap buffer reused across turns for replies so cgo does not reject passing
// a Go pointer back through out to capi.c.
var (
	replyMem unsafe.Pointer
	replyCap int
)

// replyBuf returns a zero-length Tape backed by a C-allocated buffer with at
// least n bytes of capacity.
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

// libRun enters the Rust event loop through gk_go_run in capi.c.
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

// postWake wakes the UI event loop so queued gk.Post callbacks run.
func postWake() { C.gk_post(0) }
