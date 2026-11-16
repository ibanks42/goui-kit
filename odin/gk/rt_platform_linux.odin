package gk

// Everything in the runtime that is specific to one operating system: reserving and
// protecting the turn arenas' address range and catching a fault in it. One file per
// platform (this one is built on Linux only, by its name); rt_platform_darwin.odin and
// rt_platform_windows.odin have the same five procedures.

import "core:sys/linux"
import "core:sys/posix"

// platform_reserve reserves `size` bytes of address space, unreadable, without committing
// memory.
@(private)
platform_reserve :: proc(size: int) -> (base: rawptr, ok: bool) {
	p, errno := linux.mmap(0, uint(size), {}, {.PRIVATE, .ANONYMOUS, .NORESERVE})
	return p, errno == nil
}

// platform_open makes a range readable and writable.
@(private)
platform_open :: proc(p: rawptr, size: int) {
	linux.mprotect(p, uint(size), {.READ, .WRITE})
}

// platform_close makes a range unreadable and gives its pages back; they read as zero when
// the range is opened again.
@(private)
platform_close :: proc(p: rawptr, size: int) {
	linux.mprotect(p, uint(size), {})
	linux.madvise(p, uint(size), .DONTNEED)
}

@(private)
Fault_Proc :: #type proc "contextless" (addr: uintptr)

@(private)
platform_fault_proc: Fault_Proc

// platform_on_fault has `handler` called with the faulting address on a memory fault.
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
}
