package gk

// Everything in the runtime that is specific to one operating system: reserving and
// protecting the turn arenas' address range and catching a fault in it. One file per
// platform (this one is built on macOS only, by its name); rt_platform_linux.odin and
// rt_platform_windows.odin have the same five procedures.

import "core:sys/posix"

// platform_reserve reserves `size` bytes of address space, unreadable, without committing
// memory (macOS commits anonymous pages when they are first touched).
@(private)
platform_reserve :: proc(size: int) -> (base: rawptr, ok: bool) {
	p := posix.mmap(nil, uint(size), {}, {.PRIVATE, .ANONYMOUS})
	return p, p != posix.MAP_FAILED
}

// platform_open makes a range readable and writable.
@(private)
platform_open :: proc(p: rawptr, size: int) {
	posix.mprotect(p, uint(size), {.READ, .WRITE})
}

// platform_close makes a range unreadable and gives its pages back; they read as zero when
// the range is opened again. madvise on macOS promises neither (MADV_DONTNEED and MADV_FREE
// may keep the old bytes), so a fresh anonymous mapping replaces the range in place.
@(private)
platform_close :: proc(p: rawptr, size: int) {
	posix.mmap(p, uint(size), {}, {.PRIVATE, .ANONYMOUS, .FIXED})
}

@(private)
Fault_Proc :: #type proc "contextless" (addr: uintptr)

@(private)
platform_fault_proc: Fault_Proc

// platform_on_fault has `handler` called with the faulting address on a memory fault. On
// macOS a read of a page mapped without access arrives as SIGBUS or as SIGSEGV, by the kind of
// mapping; both are caught.
@(private)
platform_on_fault :: proc(handler: Fault_Proc) {
	platform_fault_proc = handler
	on_signal :: proc "c" (sig: posix.Signal, info: ^posix.siginfo_t, uctx: rawptr) {
		platform_fault_proc(uintptr(info.si_addr))
	}
	act: posix.sigaction_t
	act.sa_sigaction = on_signal
	act.sa_flags = {.SIGINFO}
	posix.sigaction(.SIGSEGV, &act, nil)
	posix.sigaction(.SIGBUS, &act, nil)
}
