// Package rt implements the Go runtime beneath github.com/ibanks42/goui-kit/gk and its
// sub-packages.
//
// Core responsibilities:
//   - Library loading: purego dynamic loading by default (rt_lib_dyn*.go) or
//     static cgo linking under -tags gkstatic (rt_lib_static.go, capi.c).
//   - Threading: locking the main goroutine to the process's initial OS thread
//     and validating every UI call against the active turn and UI goroutine.
//   - Turn dispatch: executing KInit, KWindowInit, KFrame, KViews, KRows,
//     KEvent, KWake, KDone, and KPanic callbacks from the Rust core, batching
//     deferred writes and piggybacking inline view descriptions onto replies.
//   - Tape encoding: building element and value trees into little-endian byte
//     tapes with single-parent, liveness, and UTF-8 checks (rt_emit.go, rt_wire.go).
//   - Lifetimes: view-scoped handle ownership cascades, weak-pointer GC cleanup,
//     and generational event handler tables (rt_life.go, rt_view.go, rt_handler.go).
package rt

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	rdebug "runtime/debug"
	"strings"
)

// Lock the main goroutine to the initial OS thread during package init.
// Platform GUI frameworks (notably AppKit on macOS and Wayland/X11 event
// queues) require windowing calls and the event loop to run on thread 0.
func init() {
	runtime.LockOSThread()
}

// Info describes the loaded libgouikit binary's ABI version, opcode interface
// hash, feature flags (gpui-fast engine and diag offscreen support), and
// embedded crate version summary.
type Info struct {
	AbiMajor, AbiMinor int
	APIHash            uint64
	GpuiFast, Diag     bool
	Versions           string
}

// LibraryInfo returns metadata from the loaded native library, or a zero Info
// if the library is unavailable.
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

// rawInfo holds the raw fields decoded from C struct gk_info before feature
// flag expansion.
type rawInfo struct {
	size, magic        uint32
	abiMajor, abiMinor uint16
	apiHash            uint64
	features           uint32
	versions           string
}

// libDiag records whether the loaded library includes diagnostic offscreen and
// invariant checks.
var libDiag bool

// MustCall executes req synchronously via gk_call and returns any response
// bytes. If a prior failure already latched on this run, it aborts the turn;
// ErrPoisoned becomes a *LibraryError, and any other non-zero code stops with
// a formatted *Error.
func MustCall(req []byte) []byte {
	rc, res := call(req)
	if failed.Load() {
		panic(abortTurn{})
	}
	if rc == ErrPoisoned {
		libraryPanic(lastError())
	}
	if rc != 0 {
		stopCall(rc)
	}
	return res
}

// stopCall translates a non-zero C ABI return code into an actionable *Error
// with guidance tailored to the specific failure category.
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

// Error reports a misuse of the Go bindings together with the application
// source file, line number, and function name that caused it.
type Error struct {
	Msg  string
	File string
	Line int
	Func string
}

// Error formats the message and, when resolved, the application call site.
func (e *Error) Error() string {
	if e.Line == 0 {
		return "gk: " + e.Msg
	}
	return fmt.Sprintf("gk: %s\n    at %s:%d (%s)", e.Msg, e.File, e.Line, e.Func)
}

// appFrame walks up to 32 stack frames starting skip frames above appFrame and
// returns the first frame belonging to user code (skipping github.com/ibanks42/goui-kit/gk,
// Go runtime internals, and cgo trampolines).
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

// Stop aborts the active turn by panicking with a formatted *Error attributed
// to the first application stack frame. boundary() catches this panic before
// returning across the C ABI and surfaces it as the return value of Run.
func Stop(format string, args ...any) {
	e := &Error{Msg: fmt.Sprintf(format, args...)}
	e.File, e.Line, e.Func = appFrame(3)
	panic(e)
}

func stopAt(file string, line int, format string, args ...any) {
	e := &Error{Msg: fmt.Sprintf(format, args...), File: file, Line: line}
	panic(e)
}

func libraryPanic(message string) {
	e := &LibraryError{Msg: message}
	e.File, e.Line, e.Func = appFrame(3)
	panic(e)
}

// boundary sits at the top of onTurn to guarantee no Go panic ever unwinds
// across the C/Rust FFI frame. It latches the first error, unwinds nested turn
// frames back to entry, and asks the Rust core to quit the event loop.
func boundary(entry int) {
	r := recover()
	if r == nil {
		return
	}
	recovered(r, rdebug.Stack(), 3)
	unwindTurns(entry)
	askQuit()
}

// onTurn is the single Go entry point invoked by both dynCallback (purego) and
// gkGoCallback (cgo) for every callback from Rust.
// KPanic delivers a Rust-side panic or deferred misuse error (flag == 1 means
// user *Error, otherwise *LibraryError). Normal turns run through dispatch,
// endTurn, and (when returning to depth 0) the scheduler keep-alive tracker.
func onTurn(kind uint32, in []byte) []byte {
	defer boundary(depth)
	if failed.Load() {
		askQuit()
		return nil
	}
	if kind == KPanic {
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

// expectHash holds the opcode hash verified against the loaded library;
// protocol tests override it via TestExpectHash.
var expectHash = ApiHash

// checkLibrary loads libgouikit and validates its magic number, struct size,
// ABI version (exact major, library minor >= package minor), and ApiHash.
// Setting GK_INFO prints the verified library metadata to stdout.
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

const rcNotLoaded int32 = -1

var libLoadErr error

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

// Run starts the native UI event loop on the locked current OS thread and calls
// start during the initial KInit turn. If GK_SCRIPT is set in the environment,
// Run reads that script file and delegates to RunHeadless. On Linux without
// DISPLAY or WAYLAND_DISPLAY, Run fails fast with a clear error instead of
// hanging inside GPUI.
func Run(start func(cx *Cx)) error {
	if path := os.Getenv("GK_SCRIPT"); path != "" {
		script, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("gk: GK_SCRIPT=%s: the script cannot be read (%w)", path, err)
		}
		return RunHeadless(string(script), start)
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
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

// RunHeadless runs the application offscreen using the diagnostic library and
// drives its window and events with script. The scheduler keep-alive goroutine
// is not started in headless runs.
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

// TestRunRaw invokes libRun directly with caller-supplied ABI perturbations so
// protocol tests can verify version, hash, struct-size, and null-callback checks.
func TestRunRaw(hashXor uint64, majorAdd, minorAdd uint16, sizeDelta uint32, noCallback bool) (int32, string) {
	rc := libRun(false, "", AbiMajor+majorAdd, AbiMinor+minorAdd, ApiHash^hashXor, sizeDelta, noCallback)
	if rc == rcNotLoaded {
		return rc, libLoadErr.Error()
	}
	return rc, lastError()
}

// TestExpectHash overrides the expected API hash checked by checkLibrary.
func TestExpectHash(h uint64) { expectHash = h }

func inGk(fn string) bool {
	return strings.HasPrefix(fn, "github.com/ibanks42/goui-kit/gk.") || strings.HasPrefix(fn, "github.com/ibanks42/goui-kit/gk/")
}
