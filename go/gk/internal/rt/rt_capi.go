// Package rt is the runtime of GPUI-kit's Go front end: the turns, the tapes, handlers, views,
// handles and the C interface (docs/PROTOCOLS.md, docs/GO.md). Apps import gpuikit.dev/gk and its
// packages, which call into this one; it is internal, so nothing outside the module can.
//
// The library is prebuilt Rust behind seven C functions and one callback (docs/DESIGN.md
// section 5). All app code runs
// in turns: one callback on the UI thread, between frames. A view is a function that describes
// its elements; the library replays the description into real GPUI-kit elements when GPUI
// builds that view, and not otherwise.
//
// Files: rt_*.go are the hand-written runtime; gen_bases.go, gen_use.go, gen_ops.go and the
// gen_*.go of the generated types the runtime itself names are generated (gen/gkgen.py).
package rt

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo linux,gkshared LDFLAGS: -lgpuikit
#cgo linux,!gkshared LDFLAGS: -lgpuikit_static -lgpuikit_sys -ldl -lgcc_s -lutil -lrt -lpthread -lm -lc
#cgo darwin,gkshared LDFLAGS: -lgpuikit
#cgo darwin,!gkshared LDFLAGS: -lgpuikit_static -framework CoreServices -framework ApplicationServices -framework CoreFoundation -framework CoreVideo -framework Carbon -framework AppKit -framework Foundation -framework UserNotifications -framework Security -framework CoreText -framework CoreGraphics -framework CoreData -framework CoreImage -framework CloudKit -framework QuartzCore -framework UniformTypeIdentifiers -framework Metal -lobjc -liconv -lc -lm
#cgo windows,gkshared LDFLAGS: -lgpuikit
#cgo windows,!gkshared LDFLAGS: -lgpuikit_static -lgpuikit_sys -lkernel32 -lntdll -luserenv -lws2_32 -ldbghelp
#include <stdlib.h>
#include "gpuikit.h"
int32_t gk_go_run(int headless, const uint8_t *script, size_t len, uint16_t major, uint16_t minor,
                  uint64_t hash, uint32_t size_delta, int no_callback);
*/
import "C"

import (
	"fmt"
	"os"
	"runtime"
	rdebug "runtime/debug"
	"strings"
	"unsafe"
)

func init() {
	// gk.Run must be called from main: the main goroutine stays on the main OS thread, which
	// becomes the UI thread.
	runtime.LockOSThread()
}

// Info is what the library reports about itself (any goroutine may ask).
type Info struct {
	AbiMajor, AbiMinor int
	APIHash            uint64
	GpuiFast, Diag     bool // GpuiFast: built on gpui-fast (the default engine) rather than upstream GPUI
	Versions           string
}

// LibraryInfo reads the library's gk_info.
func LibraryInfo() Info {
	i := C.gk_info_get()
	if i == nil {
		return Info{}
	}
	return Info{
		AbiMajor: int(i.abi_major), AbiMinor: int(i.abi_minor), APIHash: uint64(i.api_hash),
		GpuiFast: uint32(i.features)&FeatGpuiFast != 0, Diag: uint32(i.features)&FeatDiag != 0,
		Versions: C.GoStringN(i.versions, C.int(i.versions_len)),
	}
}

// libDiag: the library is the diagnostic one (tests), where a binding fault stops loudly.
var libDiag bool

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

func MustCall(req []byte) []byte {
	rc, res := call(req)
	if rc == ErrPoisoned {
		libraryPanic(lastError())
	}
	if rc != 0 {
		stopCall(rc)
	}
	return res
}

// stopCall stops for a call the library refused: its message, and what the app does about it.
func stopCall(rc int32) {
	msg := lastError()
	hint := ""
	switch rc {
	case ErrStaleHandle:
		if strings.Contains(msg, "never made") {
			hint = "\n    The handle is zero: the kit state, view or row source in this field was never made. Make it in init (a.search = editor.NewInputState(cx).Build()) and keep the handle in your state struct."
		} else if !strings.Contains(msg, "temporary") {
			hint = "\n    A handle stays usable until gk.Release; this one was released, or it was copied from another kind of handle."
		}
	case ErrNoContext:
		hint = "\n    This call needs an open window: make it in a view's init function or in a handler, not before gk.OpenWindow."
	case ErrWrongThread:
		hint = "\n    Only gk.Post may be called from another goroutine: post the result, and the posted function runs on the UI thread."
	case ErrBusy:
		hint = "\n    The component that called this handler is updating the state right now. Write to it without reading it back, read it after the handler returns (gk.Post(func(cx *gk.Cx) { ... }) runs after this turn), or use the state's Try read where it has one (TryValue: ok is false while the state is busy)."
	case ErrBadOp, ErrBadTape, ErrVersion, ErrBadEnum:
		if !strings.Contains(msg, " ") {
			hint = "\n    The app and the library do not match: build the app against the go/ folder of the download its library came from."
		}
	}
	Stop("%s%s", msg, hint)
}

// Error is a mistake in app code the package found (an element used after its view returned, a
// released handle, a write in a view, ...). gk panics with it; the boundary with the library
// prints it with the app's line and exits 2. Nothing unwinds into the library.
type Error struct {
	Msg  string
	File string
	Line int
	Func string
}

func (e *Error) Error() string {
	if e.Line == 0 {
		return "gk: " + e.Msg
	}
	return fmt.Sprintf("gk: %s\n    at %s:%d (%s)", e.Msg, e.File, e.Line, e.Func)
}

// appFrame is the innermost caller outside this package: where a stop points.
func appFrame(skip int) (file string, line int, fn string) {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(skip, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" && !inGk(f.Function) && !strings.HasPrefix(f.Function, "runtime.") &&
			!strings.HasPrefix(f.Function, "_cgo") {
			return f.File, f.Line, f.Function
		}
		if !more {
			return "", 0, ""
		}
	}
}

// stop ends the turn for a mistake in app code: a panic with an *Error naming the app's line.
func Stop(format string, args ...any) {
	e := &Error{Msg: fmt.Sprintf(format, args...)}
	e.File, e.Line, e.Func = appFrame(3)
	panic(e)
}

// stopAt is stop with a location the runtime recorded (where an element was made).
func stopAt(file string, line int, format string, args ...any) {
	e := &Error{Msg: fmt.Sprintf(format, args...), File: file, Line: line}
	panic(e)
}

func libraryPanic(message string) {
	fmt.Fprintf(os.Stderr, "gk: the library panicked: %s\n", message)
	if file, line, fn := appFrame(3); line != 0 {
		fmt.Fprintf(os.Stderr, "    in the call at %s:%d (%s)\n", file, line, fn)
	}
	fmt.Fprintln(os.Stderr, "    This is a fault inside the library (GPUI-kit or the binding's runtime), not in the app's Go code: the app cannot go on. Run it again with GK_DEBUG=1, which sends every change at once so the call above is the one that failed, and report the message with that line.")
	os.Exit(1)
}

// boundary runs at the end of every callback: a panic must not unwind into the library. A
// stop prints its message and the app's line, any other panic its value and stack; exit 2.
func boundary() {
	r := recover()
	if r == nil {
		return
	}
	if e, ok := r.(*Error); ok {
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(2)
	}
	// Any other panic is the app's own (an index out of range, a nil map, a panic call): its
	// value, the app's line it came from, then Go's stack. Nothing unwinds into the library.
	fmt.Fprintf(os.Stderr, "gk: panic in app code: %v\n", r)
	if file, line, fn := appFrame(3); line != 0 {
		fmt.Fprintf(os.Stderr, "    at %s:%d (%s)\n", file, line, fn)
	}
	fmt.Fprintf(os.Stderr, "    The app stops here (exit 2): a panic in a handler, a view, init or a posted function ends the app, as in any Go program. Go's stack follows.\n%s", rdebug.Stack())
	os.Exit(2)
}

//export gkGoCallback
func gkGoCallback(user unsafe.Pointer, cb *C.gk_cb, kind C.uint32_t, req *C.uint8_t, n C.size_t, out **C.uint8_t, outN *C.size_t) C.int32_t {
	defer boundary()
	var in []byte
	if n > 0 {
		in = unsafe.Slice((*byte)(unsafe.Pointer(req)), int(n)) // the library's, valid during this call
	}
	if uint32(kind) == KPanic {
		// The library caught a panic of its own outside a gk_call. It cannot go on.
		r := Reader{B: in}
		libraryPanic(r.Str() + "")
	}
	if r := dispatch(uint32(kind), in); len(r) > 0 {
		// The C side passes it to gk_reply when this returns (capi.c): C memory, valid until the
		// next callback's reply is written.
		*out, *outN = (*C.uint8_t)(unsafe.Pointer(&r[0])), C.size_t(len(r))
	}
	endTurn()
	return 0
}

// replyMem is the C memory a turn's reply is written into (see gkGoCallback); it grows as needed
// and is never freed.
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
			fmt.Fprintln(os.Stderr, "gk: out of memory for a turn's reply")
			os.Exit(1)
		}
		replyCap = c
	}
	return unsafe.Slice((*byte)(replyMem), replyCap)[:0]
}

// expectHash is the interface hash this package expects; the runtime's version test changes it.
var expectHash = ApiHash

// checkLibrary is the binding's half of the version check (the library checks the gk_app).
func checkLibrary() {
	i := C.gk_info_get()
	if i == nil || uint32(i.magic) != C.GK_MAGIC || int(i.size) < int(unsafe.Sizeof(*i)) {
		fmt.Fprintf(os.Stderr, "gk: the %s the app loaded is not a library this package can use: check %s\n", libFile, libFind)
		os.Exit(1)
	}
	info := LibraryInfo()
	if info.AbiMajor != AbiMajor || info.AbiMinor < AbiMinor || info.APIHash != expectHash {
		fmt.Fprintf(os.Stderr, "gk: the library and this package are from different versions: the library is ABI %d.%d with interface hash %#x (%s); this package was generated for ABI %d.%d and hash %#x\n    Build the app against the go/ folder of the download its %s came from (or point %s at the matching lib/).\n",
			info.AbiMajor, info.AbiMinor, info.APIHash, info.Versions, AbiMajor, AbiMinor, expectHash, libFile, libPoint)
		os.Exit(1)
	}
	libDiag = info.Diag
	if os.Getenv("GK_INFO") != "" {
		fmt.Printf("gk-info binding=go abi=%d.%d api_hash=%#x gpui_fast=%v diag=%v versions=%q\n",
			info.AbiMajor, info.AbiMinor, info.APIHash, info.GpuiFast, info.Diag, info.Versions)
	}
}

func runFailed(rc int32) {
	if rc == ErrPoisoned {
		libraryPanic(lastError())
	}
	fmt.Fprintf(os.Stderr, "gk: the app could not start: %s\n", lastError())
	if rc == ErrUnsupported && strings.Contains(lastError(), "diag") {
		fmt.Fprintf(os.Stderr, "    GK_SCRIPT (or gk.RunHeadless) runs the app offscreen, which only the diagnostic library can do: put its lib/ folder %s.\n", libFirst)
	}
	os.Exit(1)
}

func enter(headless bool, script string) int32 {
	var p *C.uint8_t
	if headless {
		p = (*C.uint8_t)(C.CBytes([]byte(script + "\x00"))) // C memory: the library reads it for the whole run
		defer C.free(unsafe.Pointer(p))
	}
	h := C.int(0)
	if headless {
		h = 1
	}
	return int32(C.gk_go_run(h, p, C.size_t(len(script)), AbiMajor, AbiMinor, C.uint64_t(ApiHash), 0, 0))
}

// Run enters the library and returns when the app quits. Call it from main; start runs in the
// first turn (open a window there). With GK_SCRIPT=file in the environment the app runs
// offscreen under that gk-meter script (RunHeadless; it needs the diagnostic library).
func Run(start func(cx *Cx)) {
	if path := os.Getenv("GK_SCRIPT"); path != "" {
		script, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gk: GK_SCRIPT=%s: the script cannot be read (%v)\n", path, err)
			os.Exit(1)
		}
		RunHeadless(string(script), start)
		return
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		// GPUI finds no X11 or Wayland server then and waits without a word; say so instead.
		// Linux only: macOS and Windows always have their window server.
		fmt.Fprintln(os.Stderr, "gk: the app has no display to open its window on: DISPLAY and WAYLAND_DISPLAY are unset.\n    Run it in a desktop session (or over ssh -X), or offscreen with GK_SCRIPT=file and the diagnostic library first in LD_LIBRARY_PATH.")
		os.Exit(1)
	}
	begin(start)
	if rc := enter(false, ""); rc != 0 {
		runFailed(rc)
	}
	report()
}

// RunHeadless runs the app offscreen under a gk-meter script (the diagnostic library only).
func RunHeadless(script string, start func(cx *Cx)) {
	begin(start)
	if rc := enter(true, script); rc != 0 {
		runFailed(rc)
	}
	report()
}

func begin(start func(cx *Cx)) {
	checkLibrary()
	if start == nil {
		fmt.Fprintln(os.Stderr, "gk: gk.Run needs a start function (it opens the first window)")
		os.Exit(2)
	}
	appStart = start
	runtimeInit()
}

// TestRunRaw calls gk_run with a gk_app changed as the runtime's version test asks: the
// library must refuse each with GK_ERR_VERSION. It returns the status and the library's message.
func TestRunRaw(hashXor uint64, majorAdd, minorAdd uint16, sizeDelta uint32, noCallback bool) (int32, string) {
	nc := C.int(0)
	if noCallback {
		nc = 1
	}
	rc := int32(C.gk_go_run(0, nil, 0, C.uint16_t(AbiMajor+majorAdd), C.uint16_t(AbiMinor+minorAdd),
		C.uint64_t(ApiHash^hashXor), C.uint32_t(sizeDelta), nc))
	return rc, lastError()
}

// TestExpectHash makes the package expect another interface hash (the version test's other half).
func TestExpectHash(h uint64) { expectHash = h }

func postWake() { C.gk_post(0) }

// inGk reports whether a function is the binding's own: package gk or one of its packages
// (gk/kit, gk/internal/rt, ...), not the app's.
func inGk(fn string) bool {
	return strings.HasPrefix(fn, "gpuikit.dev/gk.") || strings.HasPrefix(fn, "gpuikit.dev/gk/")
}
