// Package gk is the Odin binding of GPUI-kit, on gpui-fast by default (the library also builds
// on upstream GPUI).
//
// The library is prebuilt Rust behind seven C functions and one callback. All app code runs in
// turns: one callback on the UI thread, between frames. A view is a procedure that describes
// its elements into a tape; the library replays the tape into real GPUI-kit elements when
// GPUI builds that view, and not otherwise.
//
// Files: rt_*.odin are the hand-written runtime (docs/PROTOCOLS.md); rt_emit.odin is the part
// generated code may call; gen_*.odin are generated; elements.odin is the hand-written sample
// of generated code, replaced as the generator's output lands.
package gk

import "base:runtime"
import "core:c"
import "core:fmt"
import "core:os"
import "core:strings"

// The library is linked into the program: libgpuikit_static.a (gpuikit_static.lib on Windows),
// found with -extra-linker-flags (-L, or /LIBPATH: on Windows). The linker keeps only the component
// families the program calls into (each generated procedure links its own: gen_use.odin), told to
// drop the rest by the flags below, so an app needs no flag of its own for it (docs/DESIGN.md
// section 16). Without -debug the symbol table is left out too (the library's alone is over 20 MB).
//
// -define:GK_SHARED=true links the shared library instead (libgpuikit.so, libgpuikit.dylib, or
// gpuikit.dll through its import library gpuikit.dll.lib), which the program loads at run time.
// The repository's own tests use it, so one build can run against either library variant.
GK_SHARED :: #config(GK_SHARED, false)

when ODIN_OS == .Windows {
	GK_LIB :: "system:gpuikit.dll.lib" when GK_SHARED else "system:gpuikit_static.lib"
	when GK_SHARED {
		foreign import capi {GK_LIB}
	} else {
		@(extra_linker_flags = "/OPT:REF")
		foreign import capi {
			GK_LIB,
			"system:gpuikit_sys.lib",
			"system:kernel32.lib",
			"system:ntdll.lib",
			"system:userenv.lib",
			"system:ws2_32.lib",
			"system:dbghelp.lib",
		}
	}
} else when ODIN_OS == .Darwin {
	GK_LIB :: "system:gpuikit" when GK_SHARED else "system:gpuikit_static"
	when GK_SHARED {
		foreign import capi {GK_LIB}
	} else {
		@(extra_linker_flags = "-Wl,-dead_strip" + ("" when ODIN_DEBUG else " -Wl,-x"))
		foreign import capi {
			GK_LIB,
			"system:CoreServices.framework",
			"system:ApplicationServices.framework",
			"system:CoreFoundation.framework",
			"system:CoreVideo.framework",
			"system:Carbon.framework",
			"system:AppKit.framework",
			"system:Foundation.framework",
			"system:UserNotifications.framework",
			"system:Security.framework",
			"system:CoreText.framework",
			"system:CoreGraphics.framework",
			"system:CoreData.framework",
			"system:CoreImage.framework",
			"system:CloudKit.framework",
			"system:QuartzCore.framework",
			"system:UniformTypeIdentifiers.framework",
			"system:Metal.framework",
			"system:objc",
			"system:iconv",
		}
	}
} else {
	GK_LIB :: "system:gpuikit" when GK_SHARED else "system:gpuikit_static"
	when GK_SHARED {
		foreign import capi {GK_LIB}
	} else {
		@(extra_linker_flags = "-Wl,--gc-sections" + ("" when ODIN_DEBUG else " -Wl,-s"))
		foreign import capi {
			GK_LIB,
			"system:gpuikit_sys",
			"system:dl",
			"system:gcc_s",
			"system:util",
			"system:rt",
			"system:pthread",
			"system:m",
		}
	}
}

GK_MAGIC :: 0x314b5047

Gk_Info :: struct {
	size, magic:          u32,
	abi_major, abi_minor: u16,
	api_hash:             u64,
	features:             u32,
	versions:             [^]u8,
	versions_len:         c.size_t,
}

Gk_Cb :: struct {}

Gk_Callback :: #type proc "c" (user: rawptr, cb: ^Gk_Cb, kind: u32, req: [^]u8, len: c.size_t) -> i32

Gk_App :: struct {
	size:                 u32,
	abi_major, abi_minor: u16,
	api_hash:             u64,
	callback:             Gk_Callback,
	user:                 rawptr,
	flags:                u32,
}

@(default_calling_convention = "c")
foreign capi {
	gk_info_get :: proc() -> ^Gk_Info ---
	// Registers every component family (gen/gk_fam.py): link_all, and the shared library's start-up.
	gk_use_all :: proc() ---
	gk_run :: proc(app: ^Gk_App) -> i32 ---
	gk_call :: proc(req: [^]u8, len: c.size_t, res: ^[^]u8, res_len: ^c.size_t) -> i32 ---
	gk_reply :: proc(cb: ^Gk_Cb, out: [^]u8, len: c.size_t) -> i32 ---
	gk_post :: proc(token: u64) -> i32 ---
	gk_last_error :: proc(len: ^c.size_t) -> [^]u8 ---
	gk_headless_run :: proc(app: ^Gk_App, script: [^]u8, len: c.size_t) -> i32 ---
}

// The interface hash this package expects of the library. A test may override it to show
// what a mismatch looks like; nothing else should.
EXPECT_API_HASH :: u64(#config(GK_TEST_API_HASH, API_HASH))

// last_error is the message of the last failed export on this thread.
last_error :: proc() -> string {
	n: c.size_t
	p := gk_last_error(&n)
	return string(p[:n])
}

// stop ends the program for a mistake in app code, with the message and the app's line.
// Exit code 2. Odin does not unwind, and nothing may unwind into the library.
@(private)
stop :: proc(loc: runtime.Source_Code_Location, format: string, args: ..any) -> ! {
	fmt.eprintf("gk: ")
	fmt.eprintf(format, ..args)
	if loc.line != 0 {
		fmt.eprintf("\n    at %s:%d:%d in %s\n", loc.file_path, loc.line, loc.column, loc.procedure)
	} else {
		fmt.eprintln()
	}
	os.exit(2)
}

// library_panic ends the program after a panic inside the library. Exit code 1.
@(private)
library_panic :: proc(message: string, loc: runtime.Source_Code_Location) -> ! {
	fmt.eprintf("gk: the library panicked: %s\n", message)
	if loc.line != 0 {
		fmt.eprintf("    in the call at %s:%d:%d in %s\n", loc.file_path, loc.line, loc.column, loc.procedure)
	}
	os.exit(1)
}

// call makes one gk_call. The result is the library's and valid until the next gk_call;
// callers read it at once.
@(private)
call :: proc(req: []u8) -> (rc: i32, res: []u8) {
	p: [^]u8
	n: c.size_t
	rc = gk_call(raw_data(req), len(req), &p, &n)
	if rc == 0 && n > 0 {
		res = p[:n]
	}
	return
}

@(private)
must_call :: proc(req: []u8, loc := #caller_location) -> []u8 {
	rc, res := call(req)
	if rc == ERR_POISONED {
		library_panic(last_error(), loc)
	}
	if rc != 0 {
		stop_call(loc, rc)
	}
	return res
}

// stop_call stops for a call the library refused: its message, and what the app does about it,
// by the kind of refusal (the error number itself is protocol, not something an app acts on).
@(private)
stop_call :: proc(loc: runtime.Source_Code_Location, rc: i32) -> ! {
	msg := last_error()
	hint := ""
	switch rc {
	case ERR_STALE_HANDLE:
		if strings.contains(msg, "never made") {
			hint = "\n    The handle is zero: the kit state, view or row source in this field was never made. Make it in init (for example a.search = gk.input_state(cx)->build()) and keep the handle in your state struct."
		} else if !strings.contains(msg, "temporary") {
			hint = "\n    A handle stays usable until gk.release; this one was released, or it was copied from another kind of handle."
		}
	case ERR_NO_CONTEXT:
		hint = "\n    This call needs an open window: make it in a view's init procedure or in a handler, not before gk.open_window."
	case ERR_WRONG_THREAD:
		hint = "\n    Only gk.post may be called from another thread: post the result, and the posted procedure runs on the UI thread."
	case ERR_BAD_OP, ERR_BAD_TAPE, ERR_VERSION, ERR_BAD_ENUM:
		if !strings.contains(msg, " ") {
			hint = "\n    The app and the library do not match: build the app against the odin/ folder of the download its library came from."
		}
	}
	stop(loc, "%s%s", msg, hint)
}

// element_origin is what the runtime can say about where an element came from, for a stop
// inside a builder (the generated builders take no caller location): the line that made the
// element when GK_DEBUG=1 recorded it, else the view procedure that was running.
@(private)
element_origin :: proc(node: u32, this_build: bool) -> string {
	if build.active && this_build && int(node) < len(build.locs) {
		l := build.locs[node]
		return fmt.tprintf("\n    the element was made at %s:%d:%d in %s", l.file_path, l.line, l.column, l.procedure)
	}
	s := ""
	if build.active && build.view != nil {
		v := build.view
		if v.row_call != nil {
			s = fmt.tprintf("\n    while the row procedure described row %d of a list", v.row_at.row)
		} else if v.loc.line != 0 {
			s = fmt.tprintf("\n    while the view procedure of the gk.view made at %s:%d ran", v.loc.file_path, v.loc.line)
		}
	}
	if !debug {
		s = fmt.tprintf("%s\n    Run the app with GK_DEBUG=1 to see the line that made the element.", s)
	}
	return s
}

// callback is the one way the library reaches Odin: on the UI thread, never concurrently.
// A "c" procedure has no context, so it takes the one `run` saved.
@(private)
callback :: proc "c" (user: rawptr, cb: ^Gk_Cb, kind: u32, req: [^]u8, n: c.size_t) -> i32 {
	context = ctx
	if kind == K_PANIC {
		// The library caught a panic of its own outside a gk_call. It cannot go on.
		r := Reader{req[:n], 0}
		library_panic(rd_str(&r), {})
	}
	out := dispatch(kind, req[:n])
	if len(out) > 0 {
		gk_reply(cb, raw_data(out), len(out)) // copies before it returns
	}
	end_turn()
	if cur == nil {
		sweep_owned() // handler values no builder took, and releases that waited for a handler
		sweep_rows() // records of rows that were dropped
	}
	return 0
}

// check_library is the binding's half of the version check (the library checks the gk_app
// it is given in gk_run). A library and a package from different generations refuse each
// other here, before anything else happens.
@(private)
check_library :: proc() {
	info := gk_info_get()
	if info == nil || info.magic != GK_MAGIC || int(info.size) < size_of(Gk_Info) {
		fmt.eprintln("gk: the libgpuikit.so the app loaded is not a library this package can use: check LD_LIBRARY_PATH and the rpath the app was built with")
		os.exit(1)
	}
	versions := string(info.versions[:info.versions_len])
	when GK_SHARED {
		gk_use_all() // the shared library holds every family
	}
	if info.abi_major != ABI_MAJOR || info.abi_minor < ABI_MINOR || info.api_hash != EXPECT_API_HASH {
		fmt.eprintf(
			"gk: the library and this package are from different versions: the library is ABI %d.%d with interface hash %#x (%s); this package was generated for ABI %d.%d and hash %#x\n    Build the app against the odin/ folder of the download its libgpuikit.so came from (or point the rpath or LD_LIBRARY_PATH at the matching lib/).\n",
			info.abi_major, info.abi_minor, info.api_hash, versions, ABI_MAJOR, ABI_MINOR, EXPECT_API_HASH,
		)
		os.exit(1)
	}
	lib_diag = info.features & FEAT_DIAG != 0
	if _, show := os.lookup_env("GK_INFO", context.allocator); show {
		fmt.printf(
			"gk-info binding=odin abi=%d.%d api_hash=%#x gpui_fast=%v diag=%v versions=%q\n",
			info.abi_major, info.abi_minor, info.api_hash,
			info.features & FEAT_GPUI_FAST != 0, info.features & FEAT_DIAG != 0, versions,
		)
	}
}

// A failed assertion or a panic in app code must not unwind into Rust. Odin never unwinds;
// this prints the message and exits 2.
@(private)
app_panic :: proc(prefix, message: string, loc: runtime.Source_Code_Location) -> ! {
	fmt.eprintf("gk: %s: %s\n    at %s:%d:%d in %s\n", prefix, message, loc.file_path, loc.line, loc.column, loc.procedure)
	os.exit(2)
}

@(private)
run_failed :: proc(rc: i32) -> ! {
	if rc == ERR_POISONED {
		library_panic(last_error(), {})
	}
	fmt.eprintf("gk: the app could not start: %s\n", last_error())
	if rc == ERR_UNSUPPORTED && strings.contains(last_error(), "diag") {
		fmt.eprintln("    GK_SCRIPT (or gk.run_headless) runs the app offscreen, which only the diagnostic library can do: put its lib/ folder first in LD_LIBRARY_PATH.")
	}
	os.exit(1)
}

// run enters the library with the app's state and returns when the app quits. The calling
// thread becomes the UI thread. With GK_SCRIPT=file in the environment the app runs offscreen
// under that gk-meter script instead (gk.run_headless; it needs the diagnostic library), so a
// test can drive any app without code of its own for it.
run :: proc(state: ^$S, init: proc(s: ^S, cx: ^Cx)) {
	if path, ok := os.lookup_env("GK_SCRIPT", context.allocator); ok {
		script, err := os.read_entire_file(path, context.allocator)
		if err != nil {
			fmt.eprintf("gk: GK_SCRIPT=%s: the script cannot be read (%v)\n", path, err)
			os.exit(1)
		}
		run_headless(state, string(script), init)
		return
	}
	start(state, init)
	app := Gk_App{size_of(Gk_App), ABI_MAJOR, ABI_MINOR, API_HASH, callback, nil, 0}
	if rc := gk_run(&app); rc != 0 {
		run_failed(rc)
	}
	report()
}

// run_headless runs the app offscreen under a gk-meter script (the diag library only).
run_headless :: proc(state: ^$S, script: string, init: proc(s: ^S, cx: ^Cx)) {
	start(state, init)
	app := Gk_App{size_of(Gk_App), ABI_MAJOR, ABI_MINOR, API_HASH, callback, nil, 0}
	if rc := gk_headless_run(&app, raw_data(script), len(script)); rc != 0 {
		run_failed(rc)
	}
	report()
}

@(private)
start :: proc(state: ^$S, init: proc(s: ^S, cx: ^Cx)) {
	check_library()
	tramp :: proc(fn, state: rawptr, cx: ^Cx) {
		(cast(proc(s: ^S, cx: ^Cx))fn)(cast(^S)state, cx)
	}
	app_state = state
	app_state_type = typeid_of(S)
	app_init = rawptr(init)
	app_init_call = tramp
	runtime_init()
	ctx = context
	ctx.assertion_failure_proc = app_panic
}

// link_all links every GPUI-kit component family into the program and registers them all. A
// program links a family only when it calls into it (gk.button links the button family); link_all
// is the escape hatch for one that wants all of them, at the size of all of them.
link_all :: proc() {
	gk_use_all()
}
