package gk

// The per-turn arena. Everything made during a turn (elements, tapes, the reply,
// Temp_Strings, the Cx, whatever context.temp_allocator hands out) lives in that turn's
// arena. gk_reply copies the reply before the callback returns, so nothing in the arena is
// needed afterwards. With GK_GUARD (default in debug builds) the arena is then made
// unreadable, and a later read faults on the line that reads, naming the turn.

import "base:runtime"
import "core:fmt"
import "core:os"

// Guard pages on retired turn arenas. On in debug builds; -define:GK_GUARD=true turns it on
// in any build. Cost: two system calls per turn.
GUARD :: #config(GK_GUARD, ODIN_DEBUG)

@(private)
RING :: 32
@(private)
SLOT_SIZE :: 16 << 20

@(private)
Turn_Slot :: struct {
	base:    [^]u8,
	used:    int,
	turn_no: u64,
	kind:    u32,
	live:    bool,
}

@(private)
ring: [RING]Turn_Slot
@(private)
ring_base: uintptr
@(private)
ring_next: int

@(private)
kind_name :: proc "contextless" (kind: u32) -> string {
	switch kind {
	case K_INIT:
		return "INIT"
	case K_WINDOW_INIT:
		return "WINDOW_INIT"
	case K_FRAME:
		return "FRAME"
	case K_VIEWS:
		return "VIEWS"
	case K_EVENT:
		return "EVENT"
	case K_WAKE:
		return "WAKE"
	}
	return "?"
}

@(private)
arena_init :: proc() {
	base, ok := platform_reserve(RING * SLOT_SIZE)
	if !ok {
		stop({}, "cannot reserve address space for the per-call temporary allocator (the system refused the mapping; is a memory limit set with ulimit -v?)")
	}
	ring_base = uintptr(base)
	for &s, i in ring {
		s.base = cast([^]u8)(ring_base + uintptr(i * SLOT_SIZE))
	}
	when !GUARD {
		platform_open(base, RING * SLOT_SIZE)
	}
	platform_on_fault(fault)
}

// A read or write of a retired turn arena lands here (GK_GUARD builds).
@(private)
fault :: proc "contextless" (addr: uintptr) {
	context = runtime.default_context()
	if addr >= ring_base && addr < ring_base + RING * SLOT_SIZE {
		s := &ring[int(addr - ring_base) / SLOT_SIZE]
		fmt.eprintf(
			"gk: memory of turn %d (%s) was used after that turn ended: something lent for one call of a\n" +
			"    handler, init or view procedure was kept past it: a gk.Temp_String (a ->value(cx) read, event\n" +
			"    text; also through a string(x) cast), a ^gk.Cx, or memory from context.temp_allocator.\n" +
			"    Copy text with gk.keep (or strings.clone) before storing it in your state.\n",
			s.turn_no, kind_name(s.kind),
		)
		os.exit(3)
	}
	fmt.eprintf("gk: segmentation fault at address %p (not in a turn arena)\n", rawptr(addr))
	os.exit(139)
}

@(private)
begin_slot :: proc(kind: u32) -> ^Turn_Slot {
	// The ring turns, so an arena just retired is not reused at once (with GK_GUARD a read of
	// it still faults). A slot still live belongs to a turn this one is nested in: skip it.
	// It used to stop here instead, so the 32nd synchronous turn inside one handler (32 events
	// fired by calls in one handler, found by the generated shape test) stopped the program
	// although only two turns were nested.
	s := &ring[ring_next]
	for tries := 0; s.live; tries += 1 {
		if tries == RING {
			stop({}, "calls into the app nested %d deep (a handler that makes the kit call another handler, and so on): look for a handler that triggers its own event", RING)
		}
		ring_next = (ring_next + 1) % RING
		s = &ring[ring_next]
	}
	ring_next = (ring_next + 1) % RING
	when GUARD {
		platform_open(s.base, SLOT_SIZE)
	}
	turn_serial += 1
	s.used = 0
	s.turn_no = turn_serial
	s.kind = kind
	s.live = true
	return s
}

@(private)
retire_slot :: proc(s: ^Turn_Slot) {
	s.live = false
	when GUARD {
		// Unreadable from now on, and the pages go back to the system.
		platform_close(s.base, SLOT_SIZE)
	}
}

@(private)
turn_allocator_proc :: proc(
	data: rawptr,
	mode: runtime.Allocator_Mode,
	size, alignment: int,
	old_memory: rawptr,
	old_size: int,
	loc := #caller_location,
) -> ([]byte, runtime.Allocator_Error) {
	s := cast(^Turn_Slot)data
	bump :: proc(s: ^Turn_Slot, size, alignment: int) -> ([]byte, runtime.Allocator_Error) {
		a := max(alignment, 1)
		at := (s.used + a - 1) & ~(a - 1)
		if at + size > SLOT_SIZE {
			return nil, .Out_Of_Memory
		}
		s.used = at + size
		return s.base[at:at + size], nil
	}
	switch mode {
	case .Alloc, .Alloc_Non_Zeroed:
		b, err := bump(s, size, alignment)
		when !GUARD {
			if err == nil && mode == .Alloc {
				runtime.mem_zero(raw_data(b), len(b))
			}
		}
		return b, err
	case .Resize, .Resize_Non_Zeroed:
		// Grow in place when this was the last allocation.
		if old_memory != nil && uintptr(old_memory) + uintptr(old_size) == uintptr(s.base) + uintptr(s.used) {
			at := int(uintptr(old_memory) - uintptr(s.base))
			if at + size > SLOT_SIZE {
				return nil, .Out_Of_Memory
			}
			s.used = at + size
			when !GUARD {
				if size > old_size && mode == .Resize {
					runtime.mem_zero(&s.base[at + old_size], size - old_size)
				}
			}
			return s.base[at:at + size], nil
		}
		b, err := bump(s, size, alignment)
		if err != nil {
			return nil, err
		}
		when !GUARD {
			if mode == .Resize {
				runtime.mem_zero(raw_data(b), len(b))
			}
		}
		if old_memory != nil {
			runtime.mem_copy_non_overlapping(raw_data(b), old_memory, min(old_size, size))
		}
		return b, nil
	case .Free:
		return nil, nil
	case .Free_All:
		s.used = 0
		return nil, nil
	case .Query_Features, .Query_Info:
		return nil, .Mode_Not_Implemented
	}
	return nil, .Mode_Not_Implemented
}
