package gk

// The wire: little-endian writers and readers, and the reflective codec that reads an event
// payload into, and writes a handler's return value from, any Odin type built from the wire
// types of docs/PROTOCOLS.md section 2.

import "base:intrinsics"
import "base:runtime"
import "core:strings"
import "core:unicode/utf8"

Tape :: [dynamic]u8

#assert(ODIN_ENDIAN == .Little) // the wire is little-endian and the writers store natively

// grow makes room for n more bytes and returns where they go. The writers below are the
// hot path of every view description (a few hundred calls per described view), so they
// write through this pointer instead of calling append once per byte group.
@(private)
grow :: #force_inline proc(b: ^Tape, n: int) -> [^]u8 {
	raw := cast(^runtime.Raw_Dynamic_Array)b
	at := raw.len
	if at + n > raw.cap {
		reserve(b, max(2 * raw.cap, at + n + 64))
	}
	raw.len = at + n
	return cast([^]u8)(uintptr(raw.data) + uintptr(at))
}

@(private)
put_u8 :: proc(b: ^Tape, v: u8) {
	grow(b, 1)[0] = v
}
@(private)
put_bool :: proc(b: ^Tape, v: bool) {
	grow(b, 1)[0] = u8(v)
}
@(private)
put_u16 :: proc(b: ^Tape, v: u16) {
	intrinsics.unaligned_store(cast(^u16)grow(b, 2), v)
}
// put_op writes a generated call's op: put_u16 under a name of its own, so gen/gk_fam.py finds
// the call ops of the generated procedures and links each one's component family.
@(private)
put_op :: #force_inline proc(b: ^Tape, op: u16) {
	put_u16(b, op)
}
@(private)
put_u32 :: proc(b: ^Tape, v: u32) {
	intrinsics.unaligned_store(cast(^u32)grow(b, 4), v)
}
@(private)
put_u64 :: proc(b: ^Tape, v: u64) {
	intrinsics.unaligned_store(cast(^u64)grow(b, 8), v)
}
@(private)
put_i32 :: proc(b: ^Tape, v: i32) {
	put_u32(b, u32(v))
}
@(private)
put_i64 :: proc(b: ^Tape, v: i64) {
	put_u64(b, u64(v))
}
@(private)
put_f32 :: proc(b: ^Tape, v: f32) {
	put_u32(b, transmute(u32)v)
}
@(private)
put_f64 :: proc(b: ^Tape, v: f64) {
	put_u64(b, transmute(u64)v)
}
@(private)
put_str :: proc(b: ^Tape, s: string) {
	// The kit's strings are UTF-8 (a Rust str). Bytes that are not used to arrive as one U+FFFD
	// in place of the whole string; they stop here instead. ASCII costs one compare a byte.
	for i in 0 ..< len(s) {
		if s[i] >= 0x80 {
			if !utf8.valid_string(s[i:]) {
				not_utf8(s)
			}
			break
		}
	}
	p := grow(b, 4 + len(s))
	intrinsics.unaligned_store(cast(^u32)p, u32(len(s)))
	if len(s) > 0 {
		runtime.mem_copy_non_overlapping(&p[4], raw_data(s), len(s))
	}
}

// not_utf8 stops the program: a string given to the kit is not UTF-8.
@(private)
not_utf8 :: proc(s: string) -> ! {
	at := 0
	for at < len(s) {
		r, n := utf8.decode_rune_in_string(s[at:])
		if r == utf8.RUNE_ERROR && n <= 1 {
			break
		}
		at += n
	}
	stop({}, "a string given to the kit is not UTF-8 (byte %#02x at offset %d of %d: %q); text from outside the program (a file, the network) can be made valid with strings.to_valid_utf8", s[at], at, len(s), s)
}

@(private)
Reader :: struct {
	b: []u8,
	i: int,
}
@(private)
rd_u8 :: proc(r: ^Reader) -> u8 {
	v := r.b[r.i]
	r.i += 1
	return v
}
@(private)
rd_bool :: proc(r: ^Reader) -> bool {
	return rd_u8(r) != 0
}
@(private)
rd_u16 :: proc(r: ^Reader) -> u16 {
	v := u16(r.b[r.i]) | u16(r.b[r.i + 1]) << 8
	r.i += 2
	return v
}
@(private)
rd_u32 :: proc(r: ^Reader) -> u32 {
	v := u32(r.b[r.i]) | u32(r.b[r.i + 1]) << 8 | u32(r.b[r.i + 2]) << 16 | u32(r.b[r.i + 3]) << 24
	r.i += 4
	return v
}
@(private)
rd_u64 :: proc(r: ^Reader) -> u64 {
	lo := u64(rd_u32(r))
	return lo | u64(rd_u32(r)) << 32
}
@(private)
rd_i32 :: proc(r: ^Reader) -> i32 {
	return i32(rd_u32(r))
}
@(private)
rd_i64 :: proc(r: ^Reader) -> i64 {
	return i64(rd_u64(r))
}
@(private)
rd_f32 :: proc(r: ^Reader) -> f32 {
	return transmute(f32)rd_u32(r)
}
@(private)
rd_f64 :: proc(r: ^Reader) -> f64 {
	return transmute(f64)rd_u64(r)
}

// rd_str reads a string that points into the reader's buffer: valid only while that buffer
// is (a gk_call result until the next gk_call, a request during its callback).
@(private)
rd_str :: proc(r: ^Reader) -> string {
	n := int(rd_u32(r))
	s := string(r.b[r.i:r.i + n])
	r.i += n
	return s
}

// rd_temp reads a string and copies it into the turn's arena: lent for this turn.
@(private)
rd_temp :: proc(r: ^Reader) -> Temp_String {
	return Temp_String(strings.clone(rd_str(r), context.temp_allocator))
}

// rd_value reads one value of any wire-able Odin type. Strings and slices land in the turn's
// arena, so they are lent for the turn like every other Temp_String.
@(private)
rd_value :: proc(r: ^Reader, p: rawptr, ti: ^runtime.Type_Info) {
	base := runtime.type_info_base(ti)
	#partial switch v in base.variant {
	case runtime.Type_Info_Integer, runtime.Type_Info_Float, runtime.Type_Info_Bit_Set, runtime.Type_Info_Rune:
		runtime.mem_copy_non_overlapping(p, &r.b[r.i], base.size)
		r.i += base.size
	case runtime.Type_Info_Boolean:
		(cast(^u8)p)^ = u8(rd_u8(r) != 0)
	case runtime.Type_Info_Enum:
		rd_value(r, p, v.base)
	case runtime.Type_Info_String:
		(cast(^string)p)^ = string(rd_temp(r))
	case runtime.Type_Info_Array:
		for i in 0 ..< v.count {
			rd_value(r, rawptr(uintptr(p) + uintptr(i * v.elem_size)), v.elem)
		}
	case runtime.Type_Info_Struct:
		if read_obj(r, p, ti) { // a library object: its handle (rt_handles.odin)
			return
		}
		for i in 0 ..< int(v.field_count) {
			rd_value(r, rawptr(uintptr(p) + v.offsets[i]), v.types[i])
		}
	case runtime.Type_Info_Union:
		// One variant: Maybe(T), a presence byte. More: an enum with data, a u16 variant
		// number from 0. Odin's own tag is one higher unless the union is #no_nil.
		index: int
		if len(v.variants) == 1 {
			if rd_u8(r) == 0 {
				runtime.mem_zero(p, base.size)
				return
			}
		} else {
			index = int(rd_u16(r))
			if index >= len(v.variants) {
				stop({}, "an event of type %v arrived with variant %d of %d: the library and this package disagree", ti, index, len(v.variants))
			}
		}
		runtime.mem_zero(p, base.size)
		rd_value(r, p, v.variants[index])
		if v.tag_type != nil {
			tag := i64(index) if v.no_nil else i64(index + 1)
			runtime.mem_copy_non_overlapping(rawptr(uintptr(p) + v.tag_offset), &tag, v.tag_type.size)
		}
	case runtime.Type_Info_Slice:
		n := int(rd_u32(r))
		raw := cast(^runtime.Raw_Slice)p
		raw.len = n
		raw.data = nil
		if n > 0 {
			mem, _ := runtime.mem_alloc_bytes(n * v.elem_size, v.elem.align, context.temp_allocator)
			raw.data = raw_data(mem)
			for i in 0 ..< n {
				rd_value(r, rawptr(uintptr(raw.data) + uintptr(i * v.elem_size)), v.elem)
			}
		}
	case:
		stop({}, "a value of type %v cannot travel as an event payload", ti)
	}
}

// put_value writes one value of any wire-able Odin type (a handler's return value).
@(private)
put_value :: proc(b: ^Tape, p: rawptr, ti: ^runtime.Type_Info) {
	if ti.id == Color {
		put_color(b, (cast(^Color)p)^) // a theme token or HSLA, as the library reads a colour (PROTOCOLS.md 9.14)
		return
	}
	base := runtime.type_info_base(ti)
	#partial switch v in base.variant {
	case runtime.Type_Info_Integer, runtime.Type_Info_Float, runtime.Type_Info_Bit_Set, runtime.Type_Info_Rune:
		append(b, ..(cast([^]u8)p)[:base.size])
	case runtime.Type_Info_Boolean:
		append(b, u8((cast(^u8)p)^ != 0))
	case runtime.Type_Info_Enum:
		put_value(b, p, v.base)
	case runtime.Type_Info_String:
		put_str(b, (cast(^string)p)^)
	case runtime.Type_Info_Array:
		for i in 0 ..< v.count {
			put_value(b, rawptr(uintptr(p) + uintptr(i * v.elem_size)), v.elem)
		}
	case runtime.Type_Info_Struct:
		for i in 0 ..< int(v.field_count) {
			put_value(b, rawptr(uintptr(p) + v.offsets[i]), v.types[i])
		}
	case runtime.Type_Info_Union:
		tag: i64
		if v.tag_type != nil {
			runtime.mem_copy_non_overlapping(&tag, rawptr(uintptr(p) + v.tag_offset), v.tag_type.size)
		}
		index := int(tag) if v.no_nil else int(tag) - 1
		if len(v.variants) == 1 {
			append(b, u8(index == 0))
			if index != 0 {
				return
			}
		} else {
			if index < 0 {
				stop({}, "a handler returned a nil %v", ti)
			}
			put_u16(b, u16(index))
		}
		put_value(b, p, v.variants[index])
	case runtime.Type_Info_Slice:
		raw := cast(^runtime.Raw_Slice)p
		put_u32(b, u32(raw.len))
		for i in 0 ..< raw.len {
			put_value(b, rawptr(uintptr(raw.data) + uintptr(i * v.elem_size)), v.elem)
		}
	case:
		stop({}, "a value of type %v cannot be returned from a handler", ti)
	}
}
