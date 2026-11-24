// Command attack drives libgouikit's raw C ABI directly via cgo to verify that
// malformed, forged, off-thread, and out-of-turn calls fail with documented
// error codes instead of crashing or corrupting state.
package main

/*
#cgo CFLAGS: -I${SRCDIR}/../../../include
#cgo LDFLAGS: -lgouikit
#include <stdlib.h>
#include "gouikit.h"
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
	// Keep the Go main goroutine pinned to the OS main thread for GPUI.
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

// call invokes gk_call with raw request bytes and decodes the leading uint64
// return value when the call succeeds with at least 8 bytes.
func call(req []byte) (rc int32, ret uint64) {
	var p *C.uint8_t
	var n C.size_t
	buf := req
	if cap(buf) == 0 {
		buf = make([]byte, 0, 1)
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

// last reads the thread-local error message from gk_last_error.
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

// fromThread exercises the off-thread guard by calling gk_call on a separate OS thread.
func fromThread(got chan<- int32) {
	runtime.LockOSThread()
	rc, _ := call(op(C.GK_CALL_VIEW_NEW))
	got <- rc
}

func lastIf(rc int32) string {
	if rc != 0 {
		return last()
	}
	return ""
}

// onInit opens a valid root window and runs the raw-ABI attack scenario chosen
// by GK_PROTO: truncated frames poison the runtime (rc=6), unknown/empty/null
// ops return ERR_BAD_OP (rc=1) without poisoning, and forged/wrong-kind/
// double-released handles plus off-thread calls return ERR_BAD_HANDLE (rc=2)
// and ERR_WRONG_THREAD (rc=5) while keeping the session healthy.
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
		// Proves truncated arguments return ERR_POISONED (6) and keep subsequent
		// calls poisoned rather than reading past the buffer.
		req := []byte{byte(C.GK_CALL_SOURCE_SET_COUNT), 0, 1, 2, 3}
		rc, _ = call(req)
		fmt.Printf("attack truncated rc=%d msg=%q\n", rc, last())
		rc2, _ := call(op(C.GK_CALL_VIEW_NEW))
		fmt.Printf("attack truncated after rc=%d\n", rc2)
		os.Exit(1)
	case "unknown-op":
		// Proves invalid opcode, empty slice, and nil pointer all return ERR_BAD_OP (1)
		// while leaving the runtime unpoisoned so VIEW_NEW succeeds afterward (rc=0).
		rc, _ = call(op(0xfff0))
		fmt.Printf("attack unknown-op rc=%d msg=%q\n", rc, last())
		rc, _ = call(nil)
		fmt.Printf("attack empty rc=%d msg=%q\n", rc, last())
		var p *C.uint8_t
		var n C.size_t
		rc = int32(C.gk_call(nil, 0, &p, &n))
		fmt.Printf("attack null rc=%d msg=%q\n", rc, last())
		rc, view = call(op(C.GK_CALL_VIEW_NEW))
		fmt.Printf("attack unknown-op after rc=%d view=%#x\n", rc, view)
	case "forged":
		// Proves nonexistent handles, mismatched handle kinds, and second releases
		// all fail with ERR_BAD_HANDLE (2), and another OS thread gets ERR_WRONG_THREAD (5).
		forged := uint64(C.GK_HKIND_SOURCE)<<56 | 0x0000_0001_0000_0099
		req := putU16(nil, C.GK_CALL_SOURCE_SET_COUNT)
		req = putU64(req, forged)
		req = putU32(req, 0)
		req = putU64(req, 5)
		rc, _ = call(req)
		fmt.Printf("attack forged source rc=%d msg=%q\n", rc, last())
		req = putU16(nil, C.GK_CALL_SOURCE_SET_COUNT)
		req = putU64(req, view)
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
		// Save the init callback token so `stale-cb` can replay it during a later turn.
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
			// Claim an impossible tape length so the core rejects the view reply.
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
	cs := C.CBytes([]byte(script))
	defer C.free(cs)
	rc := int32(C.attack_run((*C.uint8_t)(cs), C.size_t(len(script))))
	fmt.Printf("attack done mode=%s rc=%d views=%d msg=%q\n", mode, rc, viewsSeen, lastIf(rc))
	if rc != 0 {
		os.Exit(1)
	}
}
