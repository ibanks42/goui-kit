//go:build !gkstatic

// Dynamic-library backend used by default (CGO_ENABLED=0).
// It opens libgpuikit at runtime via purego (dlopen/dlsym on Unix, LoadLibrary
// /GetProcAddress on Windows), resolves the C ABI symbols, and registers a Go
// turn callback without requiring a C compiler at build time.

package rt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Dynamic-loader constants matching the C ABI header (include/gpuikit.h).
const (
	gkMagic  = 0x314b5047 // "GPK1" little-endian magic verified in readInfo
	infoSize = int(unsafe.Sizeof(cInfo{}))
	libFile  = libName
	libPoint = "GK_LIB"
)

// Human-readable library location hints embedded into startup diagnostics.
var (
	libFind  = "the file it loaded"
	libFirst = "first: GK_LIB=<its lib/ folder> (or the folder itself first in " + loaderPathVar + ")"
)

// cInfo matches the exact C struct layout of gk_info in include/gpuikit.h on
// 64-bit targets so readInfo can cast the pointer returned by gk_info_get.
type cInfo struct {
	size, magic        uint32
	abiMajor, abiMinor uint16
	apiHash            uint64
	features           uint32
	versions           *byte
	versionsLen        uintptr
}

// cApp matches the exact C struct layout of gk_app in include/gpuikit.h.
type cApp struct {
	size               uint32
	abiMajor, abiMinor uint16
	apiHash            uint64
	callback, user     uintptr
	flags              uint32
}

// Resolved symbol addresses and one-time loader state for the shared library.
// inlineReply is enabled in readInfo when the library advertises FeatInlineReply.
var (
	loadOnce                                       sync.Once
	loadErr                                        error
	libPath                                        string
	fnInfo, fnRun, fnCall, fnReply, fnPost, fnLast uintptr
	fnHeadless, fnUseAll, turnCallback             uintptr
	inlineReply                                    bool
)

// loadLibrary opens libgpuikit once per process and caches any load error.
func loadLibrary() error {
	loadOnce.Do(func() { loadErr = load() })
	return loadErr
}

// load checks 64-bit pointer width, locates and opens the shared library,
// resolves all required C ABI functions (plus optional gk_headless_run),
// activates all component families via gk_use_all, creates the purego turn
// callback trampoline, and installs the fast non-reflecting callback path
// unless GK_PUREGO_REFLECT is set.
func load() error {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return errors.New("the shared library needs a 64-bit program")
	}
	h, path, err := findLibrary()
	if err != nil {
		return err
	}
	libPath = path
	libFind = path
	for _, f := range []struct {
		p    *uintptr
		name string
	}{{&fnInfo, "gk_info_get"}, {&fnRun, "gk_run"}, {&fnCall, "gk_call"}, {&fnReply, "gk_reply"},
		{&fnPost, "gk_post"}, {&fnLast, "gk_last_error"}, {&fnUseAll, "gk_use_all"}} {
		if *f.p, err = libSym(h, f.name); err != nil {
			return fmt.Errorf("%s (%s) is not a library this package can use: it has no %s (%v)", libName, path, f.name, err)
		}
	}
	// gk_headless_run is exported only by the diagnostic (diag) build.
	fnHeadless, _ = libSym(h, "gk_headless_run")
	purego.SyscallN(fnUseAll)
	turnCallback = purego.NewCallback(dynCallback)
	if os.Getenv("GK_PUREGO_REFLECT") == "" {
		installFastCallback(turnCallback)
	}
	return nil
}

// findLibrary searches for the shared library in priority order:
//  1. GK_LIB (pointing to either the library file itself or a directory containing it)
//  2. Directories relative to the resolved executable path: ".", "lib", "../lib",
//     and "../Frameworks" on macOS app bundles
//  3. The OS dynamic loader search path (LD_LIBRARY_PATH, DYLD_LIBRARY_PATH, or PATH)
func findLibrary() (uintptr, string, error) {
	var looked []string
	try := func(path string) (uintptr, string, error, bool) {
		if _, err := os.Stat(path); err != nil {
			looked = append(looked, path)
			return 0, "", nil, false
		}
		h, err := libOpen(path)
		if err != nil {
			return 0, "", fmt.Errorf("%s was found but does not load: %v", path, err), true
		}
		return h, path, nil, true
	}
	if g := os.Getenv("GK_LIB"); g != "" {
		p := g
		if st, err := os.Stat(g); err == nil && st.IsDir() {
			p = filepath.Join(g, libName)
		}
		if h, path, err, done := try(p); done {
			return h, path, err
		}
		return 0, "", fmt.Errorf("the library was not found: GK_LIB=%s, and there is no %s (GK_LIB names the library file or the folder that holds it)", g, p)
	}
	if exe, err := os.Executable(); err == nil {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
		dir := filepath.Dir(exe)
		dirs := []string{dir, filepath.Join(dir, "lib"), filepath.Join(dir, "..", "lib")}
		if runtime.GOOS == "darwin" {
			dirs = append(dirs, filepath.Join(dir, "..", "Frameworks"))
		}
		for _, d := range dirs {
			if h, path, err, done := try(filepath.Join(filepath.Clean(d), libName)); done {
				return h, path, err
			}
		}
	}
	h, err := libOpen(libName)
	if err == nil {
		return h, libName + " (found by the system loader)", nil
	}
	return 0, "", fmt.Errorf("the library %s was not found. Looked in:\n    %s\n    and the system loader's path (%s): %v\n    Put %s beside the program (or in its lib/ folder), or set GK_LIB to the folder that holds it (the download's lib/)",
		libName, strings.Join(looked, "\n    "), loaderPathVar, err, libName)
}

func lib() bool { return loadLibrary() == nil }

// readInfo calls gk_info_get and copies its version, hash, and feature bits
// into a Go rawInfo struct. When FeatInlineReply is set, dynCallback writes
// reply pointers directly into the caller's cb slot instead of calling gk_reply.
func readInfo() (rawInfo, bool) {
	if !lib() {
		return rawInfo{}, false
	}
	r, _, _ := purego.SyscallN(fnInfo)
	if r == 0 {
		return rawInfo{}, false
	}
	i := *(**cInfo)(unsafe.Pointer(&r))
	ri := rawInfo{size: i.size, magic: i.magic}
	if int(i.size) >= infoSize {
		ri.abiMajor, ri.abiMinor, ri.apiHash, ri.features = i.abiMajor, i.abiMinor, i.apiHash, i.features
		ri.versions = string(unsafe.Slice(i.versions, int(i.versionsLen)))
		inlineReply = i.features&FeatInlineReply != 0
	}
	return ri, true
}

// Package-level length out-parameter for gk_last_error. Keeping it in a static
// global avoids passing a Go stack address (which could move on stack growth)
// across purego.SyscallN.
var lastN uintptr

// lastError copies the most recent error string stored by the Rust core.
func lastError() string {
	if !lib() {
		return ""
	}
	r, _, _ := purego.SyscallN(fnLast, uintptr(unsafe.Pointer(&lastN)))
	if r == 0 {
		return ""
	}
	return string(unsafe.Slice(*(**byte)(unsafe.Pointer(&r)), int(lastN)))
}

// Pre-allocated [ptr, len] out-parameter slots indexed by callDepth.
// Storing them in a package array prevents stack-growth invalidation across
// purego.SyscallN and keeps nested re-entrant gk_call frames isolated.
var (
	callOut   [64][2]uintptr
	callDepth int
)

// call invokes gk_call with req and returns a Go copy of the response bytes
// (because the Rust reply buffer is only valid until the next call on this thread).
// Calls made while re-describing a view inline during KEvent panic with
// abortInline so tryDescribe can bail out cleanly.
func call(req []byte) (int32, []byte) {
	if Vb.Active {
		if cur != nil && cur.inInlineEvent {
			panic(abortInline{})
		}
		Vb.calledLib = true
	}
	d := callDepth
	if d >= len(callOut) {
		Stop("gk_calls nest more than %d deep: a handler that makes a call that runs it again", len(callOut))
	}
	callDepth++
	o := &callOut[d]
	rc, _, _ := purego.SyscallN(fnCall, uintptr(unsafe.Pointer(&req[0])), uintptr(len(req)),
		uintptr(unsafe.Pointer(&o[0])), uintptr(unsafe.Pointer(&o[1])))
	callDepth--
	if int32(rc) != 0 || o[1] == 0 {
		return int32(rc), nil
	}
	res, n := o[0], int(o[1])
	return 0, append([]byte(nil), unsafe.Slice(*(**byte)(unsafe.Pointer(&res)), n)...)
}

// dynCallback is the Go callback invoked by Rust on every UI turn.
// During installFastCallback's probe call, user equals cbMarker and dynCallback
// immediately echoes cbMarker without touching runtime state.
// For real turns, if FeatInlineReply is supported, cb points to a [ptr, len]
// struct in the Rust stack frame that receives the reply slice directly,
// eliminating an extra FFI call into gk_reply.
func dynCallback(user, cb, kind, req, n uintptr) uintptr {
	if user == cbMarker {
		return cbMarker
	}
	var in []byte
	if n > 0 {
		in = unsafe.Slice(*(**byte)(unsafe.Pointer(&req)), int(n))
	}
	if r := onTurn(uint32(kind), in); len(r) > 0 {
		if inlineReply {
			slot := *(**[2]uintptr)(unsafe.Pointer(&cb))
			slot[0] = uintptr(unsafe.Pointer(&r[0]))
			slot[1] = uintptr(len(r))
		} else {
			purego.SyscallN(fnReply, cb, uintptr(unsafe.Pointer(&r[0])), uintptr(len(r)))
		}
	}
	return 0
}

// Sentinel value passed in the user argument while installFastCallback probes
// purego's callback register layout.
const cbMarker = 0x676b5f7475726e

// Reusable Go slice holding the serialized turn reply. Keeping it referenced in
// a package variable guarantees the backing array remains live until the next
// turn (well after Rust finishes reading the reply pointer).
var replyMem []byte

// replyBuf returns a zero-length Tape backed by replyMem with at least n bytes
// of capacity.
func replyBuf(n int) Tape {
	if n > cap(replyMem) {
		replyMem = make([]byte, 0, max(n, 2*cap(replyMem), 4096))
	}
	return replyMem[:0]
}

// Package-level descriptors kept alive in memory for the full duration of
// gk_run or gk_headless_run.
var (
	runApp    cApp
	runScript []byte
)

// libRun loads the shared library if needed, populates runApp, and enters
// gk_run or gk_headless_run.
func libRun(headless bool, script string, major, minor uint16, hash uint64, sizeDelta uint32, noCallback bool) int32 {
	if err := loadLibrary(); err != nil {
		libLoadErr = err
		return rcNotLoaded
	}
	runApp = cApp{size: uint32(unsafe.Sizeof(cApp{})) - sizeDelta, abiMajor: major, abiMinor: minor, apiHash: hash, callback: turnCallback}
	if noCallback {
		runApp.callback = 0
	}
	app := uintptr(unsafe.Pointer(&runApp))
	if headless {
		if fnHeadless == 0 {
			return ErrUnsupported
		}
		runScript = append([]byte(script), 0)
		rc, _, _ := purego.SyscallN(fnHeadless, app, uintptr(unsafe.Pointer(&runScript[0])), uintptr(len(script)))
		return int32(rc)
	}
	rc, _, _ := purego.SyscallN(fnRun, app)
	return int32(rc)
}

// postWake wakes the Rust UI event loop from any goroutine so queued Post
// callbacks and GC handle releases run on a KWake turn.
func postWake() {
	if lib() {
		purego.SyscallN(fnPost, 0)
	}
}

// LinkAll loads the shared library eagerly; dynamic builds already expose all
// component families via gk_use_all during load.
func LinkAll() { lib() }
