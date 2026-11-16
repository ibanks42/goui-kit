package gk

// Everything in the runtime that is specific to one operating system: reserving and
// protecting the turn arenas' address range and catching a fault in it. One file per
// platform (this one is built on Windows only, by its name); rt_platform_linux.odin and
// rt_platform_darwin.odin have the same five procedures.

import win "core:sys/windows"

// platform_reserve reserves `size` bytes of address space, unreadable, without committing
// memory.
@(private)
platform_reserve :: proc(size: int) -> (base: rawptr, ok: bool) {
	p := win.VirtualAlloc(nil, uint(size), win.MEM_RESERVE, win.PAGE_NOACCESS)
	return p, p != nil
}

// platform_open makes a range readable and writable. Windows counts committed pages against
// the system's commit limit (memory plus page file) even before they are touched: without
// GK_GUARD the whole ring (RING * SLOT_SIZE, 512 MB) is committed once at start.
@(private)
platform_open :: proc(p: rawptr, size: int) {
	win.VirtualAlloc(p, uint(size), win.MEM_COMMIT, win.PAGE_READWRITE)
}

// platform_close makes a range unreadable and gives its pages back (decommits it); they read
// as zero when the range is opened (committed) again.
@(private)
platform_close :: proc(p: rawptr, size: int) {
	win.VirtualFree(p, uint(size), win.MEM_DECOMMIT)
}

@(private)
Fault_Proc :: #type proc "contextless" (addr: uintptr)

@(private)
platform_fault_proc: Fault_Proc

// platform_on_fault has `handler` called with the faulting address on an access violation in
// the turn arenas. A vectored handler sees every first-chance exception of the process,
// including ones other code catches itself (drivers, the library), so only a fault inside the
// ring reaches the handler; anything else goes on to the next handler as if this one were not
// there.
@(private)
platform_on_fault :: proc(handler: Fault_Proc) {
	platform_fault_proc = handler
	on_exception :: proc "system" (info: ^win.EXCEPTION_POINTERS) -> win.LONG {
		r := info.ExceptionRecord
		if r.ExceptionCode == win.EXCEPTION_ACCESS_VIOLATION && r.NumberParameters >= 2 {
			addr := uintptr(r.ExceptionInformation[1])
			if addr >= ring_base && addr < ring_base + RING * SLOT_SIZE {
				platform_fault_proc(addr)
			}
		}
		return win.EXCEPTION_CONTINUE_SEARCH
	}
	win.AddVectoredExceptionHandler(1, on_exception)
}
