//go:build !gkstatic

package rt

// The default way to the library: the shared library (libgpuikit.so, libgpuikit.dylib or
// gpuikit.dll) is loaded when the program starts the app, and its seven functions are called
// through purego, so the package needs no C compiler, CGO_ENABLED=0 works and GOOS cross-compiles
// (docs/GO.md section 8). The build tag gkstatic links the static library through cgo instead
// (rt_lib_static.go).
//
// Where the library is looked for, in order (the first file that exists is the one loaded; one
// that exists but does not load is an error, not a reason to look further):
//   1. GK_LIB: the library file itself, or the folder that holds it; when it is set nothing else is tried
//   2. the program's folder, its lib/, and ../lib/ beside it (macOS also ../Frameworks/, for an
//      app bundle's Contents/MacOS/app)
//   3. the system loader's own search by name (LD_LIBRARY_PATH and ld.so's paths on Linux,
//      DYLD_LIBRARY_PATH and DYLD_FALLBACK_LIBRARY_PATH on macOS; Windows: the program's folder,
//      the system folders and PATH)

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

const (
	gkMagic  = 0x314b5047 // "GPK1", GK_MAGIC in gpuikit.h
	infoSize = int(unsafe.Sizeof(cInfo{}))
	libFile  = libName
	libPoint = "GK_LIB"
)

// libFind and libFirst say where the library came from and how to choose another one.
var (
	libFind  = "the file it loaded"
	libFirst = "first: GK_LIB=<its lib/ folder> (or the folder itself first in " + loaderPathVar + ")"
)

// cInfo is gk_info, cApp gk_app (gpuikit.h), as C lays them out on a 64-bit system.
type cInfo struct {
	size, magic        uint32
	abiMajor, abiMinor uint16
	apiHash            uint64
	features           uint32
	versions           *byte
	versionsLen        uintptr
}

type cApp struct {
	size               uint32
	abiMajor, abiMinor uint16
	apiHash            uint64
	callback, user     uintptr
	flags              uint32
}

// The library's functions, found once by loadLibrary.
var (
	loadOnce                                       sync.Once
	loadErr                                        error
	libPath                                        string
	fnInfo, fnRun, fnCall, fnReply, fnPost, fnLast uintptr
	fnHeadless, fnUseAll, turnCallback             uintptr
)

// loadLibrary finds and loads the library, once; every later call returns the first one's answer.
func loadLibrary() error {
	loadOnce.Do(func() { loadErr = load() })
	return loadErr
}

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
	fnHeadless, _ = libSym(h, "gk_headless_run") // the diagnostic library's; gk_run's message says so otherwise
	// The shared library holds every component family: register them all (the static build
	// registers each when the program first uses it, gen_use.go).
	purego.SyscallN(fnUseAll)
	// One callback for the whole program: purego keeps it for good (it allows about 2000).
	turnCallback = purego.NewCallback(dynCallback)
	if os.Getenv("GK_PUREGO_REFLECT") == "" { // GK_PUREGO_REFLECT=1: purego's own path, to compare
		installFastCallback(turnCallback)
	}
	return nil
}

// findLibrary looks where the comment at the top of this file says and loads the first library found.
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

// lib returns whether the library is loaded (loading it if need be), for the calls that may come
// before gk.Run (LibraryInfo, gk.Post, LinkAll).
func lib() bool { return loadLibrary() == nil }

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
	}
	return ri, true
}

// lastN is gk_last_error's length out-parameter. The library keeps its message per thread, and
// the package asks for it only on the UI thread (or before the app runs).
var lastN uintptr

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

// callOut holds gk_call's two out-parameters, one pair per nesting depth (a gk_call can run a
// turn, which makes gk_calls of its own), so a call needs no allocation.
var (
	callOut   [64][2]uintptr
	callDepth int
)

// call makes one gk_call. The request is Go memory lent for the call; the result is the
// library's and valid until the next gk_call, so it is copied out at once.
func call(req []byte) (int32, []byte) {
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

// dynCallback is the library's callback (gk_callback): every argument is register-sized, as
// purego's callbacks need on every platform. The reply is handed to gk_reply, which copies it,
// before the callback returns: the one crossing back out of a turn that has a reply (the static
// build hands it over in C after the return, capi.c).
func dynCallback(user, cb, kind, req, n uintptr) uintptr {
	if user == cbMarker {
		return cbMarker // installFastCallback's probe, when it went through purego's own path
	}
	var in []byte
	if n > 0 {
		in = unsafe.Slice(*(**byte)(unsafe.Pointer(&req)), int(n)) // the library's, valid during this call
	}
	if r := onTurn(uint32(kind), in); len(r) > 0 {
		purego.SyscallN(fnReply, cb, uintptr(unsafe.Pointer(&r[0])), uintptr(len(r)))
	}
	return 0
}

// cbMarker is the first argument (user) of installFastCallback's probe: the library's is always 0.
const cbMarker = 0x676b5f7475726e // "gk_turn"

// replyMem is the Go memory a turn's reply is written into; gk_reply copies it during the
// callback, and it is reused by the next turn.
var replyMem []byte

// replyBuf is an empty slice of at least n bytes.
func replyBuf(n int) Tape {
	if n > cap(replyMem) {
		replyMem = make([]byte, 0, max(n, 2*cap(replyMem), 4096))
	}
	return replyMem[:0]
}

// The gk_app and the script the library reads for the whole run; kept here so they stay alive.
var (
	runApp    cApp
	runScript []byte
)

// libRun enters the library (gk_run, or gk_headless_run with a script when headless).
// sizeDelta and noCallback exist only for the runtime's version test, which shows the library
// refusing a wrong gk_app.
func libRun(headless bool, script string, major, minor uint16, hash uint64, sizeDelta uint32, noCallback bool) int32 {
	if err := loadLibrary(); err != nil {
		libLoadErr = err // gk.Run's error (runFailed); nothing exits the process
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

func postWake() {
	if lib() {
		purego.SyscallN(fnPost, 0)
	}
}

// LinkAll loads the library, which holds every component family and registers them all as it
// loads (the static build's LinkAll links them all into the program: rt_linkall.go).
func LinkAll() { lib() }
