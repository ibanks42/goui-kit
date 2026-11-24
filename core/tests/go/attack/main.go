// attack: what another binding (or a broken one) can send through the C interface of
// docs/DESIGN.md section 5, without the Go package's checks in between; the port of
// core/tests/attack. This program is its own raw client: its own callback (attack.c and
// attackCallback), its own requests and replies, the numbers from the C header (gk_ops.h); it
// does not import the Go package. Each attack must end in an error code or a stop with a message,
// never in a crash, a hang or memory written where it must not be (the second review, 11 November
// 2026). core/tests/go/run-attack.sh checks the lines and the exit code.
//
//	GK_PROTO=truncated     a request cut short in its arguments: the library stops (poisoned,
//	                       exit 1) with the message; later calls are refused, nothing crashes
//	GK_PROTO=unknown-op    an op number no library has, an empty and a null request: refused
//	                       with GK_ERR_BAD_OP, and the library goes on working
//	GK_PROTO=forged        a made-up handle, a handle of another kind, one released twice, a
//	                       call from another thread: each refused with its code
//	GK_PROTO=stale-cb      gk_reply with the gk_cb of a callback that has returned: refused
//	                       (it pointed into a stack frame that is gone)
//	GK_PROTO=bad-reply     a reply whose counts run past its bytes: the library stops with the
//	                       message (exit 1), it does not read past the reply
package main

/*
#cgo CFLAGS: -I${SRCDIR}/../../../include
#cgo LDFLAGS: -lgpuikit
#include <stdlib.h>
#include "gpuikit.h"
int32_t attack_run(const uint8_t *script, size_t len);
*/
import "C"

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"unsafe"
)

func init() {
	// The library runs on the thread that enters it (the UI thread): the main goroutine stays on
	// the main OS thread.
	runtime.LockOSThread()
}

var (
	mode      string
	staleCb   *C.gk_cb
	viewsSeen int
	firstView uint64
)

func putU16(b []byte, v uint16) []byte { return append(b, byte(v), byte(v>>8)) }
func putU32(b []byte, v uint32) []byte {
	for i := range 4 {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}
func putU64(b []byte, v uint64) []byte {
	for i := range 8 {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}
func putF32(b []byte, v float32) []byte { return putU32(b, math.Float32bits(v)) }

// call sends one request; ret is the first u64 of the result, if any.
func call(req []byte) (rc int32, ret uint64) {
	var p *C.uint8_t
	var n C.size_t
	buf := req
	if cap(buf) == 0 {
		buf = make([]byte, 0, 1) // an empty request, not a null one: a pointer to no bytes
	}
	rc = int32(C.gk_call((*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(buf[:cap(buf)]))), C.size_t(len(req)), &p, &n))
	if rc == 0 && n >= 8 {
		res := unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n))
		for i := range 8 {
			ret |= uint64(res[i]) << (8 * i)
		}
	}
	return
}

func last() string {
	var n C.size_t
	p := C.gk_last_error(&n)
	if p == nil {
		return ""
	}
	return C.GoStringN(p, C.int(n))
}

func op(code uint16, args ...uint64) []byte {
	b := putU16(nil, code)
	for _, a := range args {
		b = putU64(b, a)
	}
	return b
}

// fromThread makes a call from another OS thread than the UI thread.
func fromThread(got chan<- int32) {
	runtime.LockOSThread() // a thread of its own: the UI thread is the main goroutine's
	rc, _ := call(op(C.GK_CALL_VIEW_NEW))
	got <- rc
}

func lastIf(rc int32) string {
	if rc != 0 {
		return last()
	}
	return ""
}

func onInit() {
	_, view := call(op(C.GK_CALL_VIEW_NEW))
	firstView = view
	w := putU16(nil, C.GK_CALL_OPEN_WINDOW)
	w = putF32(w, 200)
	w = putF32(w, 100)
	w = putU64(w, view)
	rc, _ := call(w)
	fmt.Printf("attack init view=%#x window rc=%d msg=%q\n", view, rc, lastIf(rc))
	switch mode {
	case "truncated":
		// SOURCE_SET_COUNT takes u64 source, u32 section, u64 count: 3 bytes are sent.
		req := []byte{byte(C.GK_CALL_SOURCE_SET_COUNT), 0, 1, 2, 3}
		rc, _ = call(req)
		fmt.Printf("attack truncated rc=%d msg=%q\n", rc, last())
		rc2, _ := call(op(C.GK_CALL_VIEW_NEW))
		fmt.Printf("attack truncated after rc=%d\n", rc2)
		os.Exit(1)
	case "unknown-op":
		rc, _ = call(op(0xfff0))
		fmt.Printf("attack unknown-op rc=%d msg=%q\n", rc, last())
		rc, _ = call(nil) // an empty request, not a null one
		fmt.Printf("attack empty rc=%d msg=%q\n", rc, last())
		var p *C.uint8_t
		var n C.size_t
		rc = int32(C.gk_call(nil, 0, &p, &n))
		fmt.Printf("attack null rc=%d msg=%q\n", rc, last())
		rc, view = call(op(C.GK_CALL_VIEW_NEW))
		fmt.Printf("attack unknown-op after rc=%d view=%#x\n", rc, view)
	case "forged":
		forged := uint64(C.GK_HKIND_SOURCE)<<56 | 0x0000_0001_0000_0099
		req := putU16(nil, C.GK_CALL_SOURCE_SET_COUNT)
		req = putU64(req, forged)
		req = putU32(req, 0)
		req = putU64(req, 5)
		rc, _ = call(req)
		fmt.Printf("attack forged source rc=%d msg=%q\n", rc, last())
		req = putU16(nil, C.GK_CALL_SOURCE_SET_COUNT)
		req = putU64(req, view) // a view, not a source
		req = putU32(req, 0)
		req = putU64(req, 5)
		rc, _ = call(req)
		fmt.Printf("attack wrong-kind rc=%d msg=%q\n", rc, last())
		_, v2 := call(op(C.GK_CALL_VIEW_NEW))
		rc1, _ := call(op(C.GK_CALL_RELEASE, v2))
		rc2, _ := call(op(C.GK_CALL_RELEASE, v2))
		fmt.Printf("attack release-twice first=%d second=%d msg=%q\n", rc1, rc2, last())
		got := make(chan int32)
		go fromThread(got)
		fmt.Printf("attack thread rc=%d\n", <-got)
	}
}

//export attackCallback
func attackCallback(user unsafe.Pointer, cb *C.gk_cb, kind C.uint32_t, req *C.uint8_t, n C.size_t) C.int32_t {
	in := unsafe.Slice((*byte)(unsafe.Pointer(req)), int(n))
	switch kind {
	case C.GK_KIND_PANIC:
		fmt.Printf("attack library-panic msg=%q\n", string(in[4:n]))
		os.Exit(1)
	case C.GK_KIND_INIT:
		staleCb = cb
		onInit()
	case C.GK_KIND_VIEWS:
		viewsSeen++
		if viewsSeen == 1 && mode == "stale-cb" {
			reply := make([]byte, 16)
			rc := C.gk_reply(staleCb, (*C.uint8_t)(unsafe.Pointer(&reply[0])), C.size_t(len(reply)))
			fmt.Printf("attack stale-cb rc=%d msg=%q\n", rc, last())
		}
		if viewsSeen == 1 && mode == "bad-reply" {
			// ret of 0 bytes, then 4,294,967,295 dirty views and no bytes for them.
			reply := []byte{0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
			rc := C.gk_reply(cb, (*C.uint8_t)(unsafe.Pointer(&reply[0])), C.size_t(len(reply)))
			fmt.Printf("attack bad-reply sent rc=%d\n", rc)
		}
	}
	return 0
}

func main() {
	mode = os.Getenv("GK_PROTO")
	script := "size 200 100\nframe\nframe\n"
	cs := C.CBytes([]byte(script)) // C memory: the library reads it for the whole run
	defer C.free(cs)
	rc := int32(C.attack_run((*C.uint8_t)(cs), C.size_t(len(script))))
	fmt.Printf("attack done mode=%s rc=%d views=%d msg=%q\n", mode, rc, viewsSeen, lastIf(rc))
	if rc != 0 {
		os.Exit(1)
	}
}
