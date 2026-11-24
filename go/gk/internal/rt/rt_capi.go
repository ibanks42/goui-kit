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
// How the package reaches the library (docs/GO.md section 8): by default it loads the shared
// library at start-up through purego, with no C compiler (rt_lib_dyn.go); with the build tag
// gkstatic it links the static library through cgo (rt_lib_static.go, capi.c). Both give this
// file the same few functions: readInfo, lastError, call, replyBuf, libRun and postWake.
//
// Files: rt_*.go are the hand-written runtime; gen_bases.go, gen_use.go, gen_ops.go and the
// gen_*.go of the generated types the runtime itself names are generated (gen/gkgen.py).
package rt

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	rdebug "runtime/debug"
	"strings"
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
	i, ok := readInfo()
	if !ok {
		return Info{}
	}
	return Info{
		AbiMajor: int(i.abiMajor), AbiMinor: int(i.abiMinor), APIHash: i.apiHash,
		GpuiFast: i.features&FeatGpuiFast != 0, Diag: i.features&FeatDiag != 0,
		Versions: i.versions,
	}
}

// rawInfo is the library's gk_info, read by readInfo (rt_lib_dyn.go, rt_lib_static.go).
type rawInfo struct {
	size, magic        uint32
	abiMajor, abiMinor uint16
	apiHash            uint64
	features           uint32
	versions           string
}

// libDiag: the library is the diagnostic one (tests), where a binding fault stops loudly.
var libDiag bool

func MustCall(req []byte) []byte {
	rc, res := call(req)
	if failed.Load() {
		panic(abortTurn{}) // a turn inside this call stopped the app: unwind the app code above
	}
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
// records it, the library is asked to quit, and gk.Run returns it (rt_fail.go). Nothing unwinds
// into the library.
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

// libraryPanic: a call came back ErrPoisoned. The failure is recorded with the app's call, and
// the app code above unwinds to the boundary (rt_fail.go).
func libraryPanic(message string) {
	e := &LibraryError{Msg: message}
	e.File, e.Line, e.Func = appFrame(3)
	panic(e)
}

// boundary runs at the end of every callback: a panic must not unwind into the library. A stop
// or a panic is recorded, the library asked to quit, and gk.Run returns it or panics again
// (rt_fail.go); entry is the turn depth when the callback began.
func boundary(entry int) {
	r := recover()
	if r == nil {
		return
	}
	recovered(r, rdebug.Stack(), 3)
	unwindTurns(entry)
	askQuit()
}

// onTurn is one callback from the library, whichever way the package reaches it (rt_lib_dyn.go,
// rt_lib_static.go): it runs the turn and returns its reply, which the caller hands to gk_reply
// (it copies) before control goes back to the library. The reply is the package's memory
// (replyBuf), valid until the next turn's reply is written. Once the app is stopping (a failure
// recorded, rt_fail.go) no app code runs: the library is asked to quit and gets no reply.
func onTurn(kind uint32, in []byte) []byte {
	defer boundary(depth)
	if failed.Load() {
		askQuit()
		return nil
	}
	if kind == KPanic {
		// The library caught a panic of its own outside a gk_call (kind 0), or found a mistake
		// in app code where no call returns it (kind 1: a view shows a released handle). It
		// quits the app when this returns, and gk.Run returns the failure.
		r := Reader{B: in}
		msg := r.Str()
		if r.i < len(r.B) && r.U8() == 1 {
			fail(&Error{Msg: msg})
		} else {
			fail(&LibraryError{Msg: msg})
		}
		return nil
	}
	out := dispatch(kind, in)
	if failed.Load() {
		// A turn nested in this one failed and the app code above returned anyway: its reply
		// is dropped.
		endTurn()
		askQuit()
		return nil
	}
	endTurn()
	if pkaActive && depth == 0 && (kind == KFrame || kind == KViews || kind == KEvent || kind == KRows || kind == KWake || kind == KDone) {
		pkaNoteTurn()
	}
	return out
}

// expectHash is the interface hash this package expects; the runtime's version test changes it.
var expectHash = ApiHash

// checkLibrary is the binding's half of the version check (the library checks the gk_app).
func checkLibrary() error {
	if err := loadLibrary(); err != nil {
		return fmt.Errorf("gk: %v", err)
	}
	i, ok := readInfo()
	if !ok || i.magic != gkMagic || int(i.size) < infoSize {
		return fmt.Errorf("gk: the %s the app loaded is not a library this package can use: check %s", libFile, libFind)
	}
	info := LibraryInfo()
	if info.AbiMajor != AbiMajor || info.AbiMinor < AbiMinor || info.APIHash != expectHash {
		return fmt.Errorf("gk: the library and this package are from different versions: the library is ABI %d.%d with interface hash %#x (%s); this package was generated for ABI %d.%d and hash %#x\n    Build the app against the go/ folder of the download its %s came from (or point %s at the matching lib/)",
			info.AbiMajor, info.AbiMinor, info.APIHash, info.Versions, AbiMajor, AbiMinor, expectHash, libFile, libPoint)
	}
	libDiag = info.Diag
	if os.Getenv("GK_INFO") != "" {
		fmt.Printf("gk-info binding=go abi=%d.%d api_hash=%#x gpui_fast=%v diag=%v versions=%q\n",
			info.AbiMajor, info.AbiMinor, info.APIHash, info.GpuiFast, info.Diag, info.Versions)
	}
	return nil
}

// rcNotLoaded is libRun's status when the library could not be loaded (libLoadErr says why).
const rcNotLoaded int32 = -1

var libLoadErr error

// runFailed is the error for a status gk_run returned with no failure recorded.
func runFailed(rc int32) error {
	if rc == rcNotLoaded {
		return fmt.Errorf("gk: %v", libLoadErr)
	}
	msg := lastError()
	if rc == ErrPoisoned {
		return &LibraryError{Msg: msg}
	}
	if rc == ErrUnsupported && strings.Contains(msg, "diag") {
		return fmt.Errorf("gk: the app could not start: %s\n    GK_SCRIPT (or gk.RunHeadless) runs the app offscreen, which only the diagnostic library can do: put its lib/ folder %s", msg, libFirst)
	}
	return fmt.Errorf("gk: the app could not start: %s", msg)
}

func enter(headless bool, script string) int32 {
	return libRun(headless, script, AbiMajor, AbiMinor, ApiHash, 0, false)
}

// Run enters the library and returns when the app quits. Call it from main; start runs in the
// first turn (open a window there). With GK_SCRIPT=file in the environment the app runs
// offscreen under that gk-meter script (RunHeadless; it needs the diagnostic library).
// It returns the mistake that stopped the app (a *Error) or a fault of the library (a
// *LibraryError); a panic in app code panics again here (rt_fail.go).
func Run(start func(cx *Cx)) error {
	if path := os.Getenv("GK_SCRIPT"); path != "" {
		script, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("gk: GK_SCRIPT=%s: the script cannot be read (%w)", path, err)
		}
		return RunHeadless(string(script), start)
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		// GPUI finds no X11 or Wayland server then and waits without a word; say so instead.
		// Linux only: macOS and Windows always have their window server.
		return errors.New("gk: the app has no display to open its window on: DISPLAY and WAYLAND_DISPLAY are unset.\n    Run it in a desktop session (or over ssh -X), or offscreen with GK_SCRIPT=file and the diagnostic library first in LD_LIBRARY_PATH")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := begin(start); err != nil {
		return err
	}
	startPKeepAlive()
	defer stopPKeepAlive()
	return runResult(enter(false, ""))
}

// RunHeadless runs the app offscreen under a gk-meter script (the diagnostic library only).
func RunHeadless(script string, start func(cx *Cx)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := begin(start); err != nil {
		return err
	}
	return runResult(enter(true, script))
}

func begin(start func(cx *Cx)) error {
	if err := checkLibrary(); err != nil {
		return err
	}
	if start == nil {
		return errors.New("gk: gk.Run needs a start function (it opens the first window)")
	}
	appStart = start
	runtimeInit()
	return nil
}

// TestRunRaw calls gk_run with a gk_app changed as the runtime's version test asks: the
// library must refuse each with GK_ERR_VERSION. It returns the status and the library's message.
func TestRunRaw(hashXor uint64, majorAdd, minorAdd uint16, sizeDelta uint32, noCallback bool) (int32, string) {
	rc := libRun(false, "", AbiMajor+majorAdd, AbiMinor+minorAdd, ApiHash^hashXor, sizeDelta, noCallback)
	if rc == rcNotLoaded {
		return rc, libLoadErr.Error()
	}
	return rc, lastError()
}

// TestExpectHash makes the package expect another interface hash (the version test's other half).
func TestExpectHash(h uint64) { expectHash = h }

// inGk reports whether a function is the binding's own: package gk or one of its packages
// (gk/kit, gk/internal/rt, ...), not the app's.
func inGk(fn string) bool {
	return strings.HasPrefix(fn, "gpuikit.dev/gk.") || strings.HasPrefix(fn, "gpuikit.dev/gk/")
}
