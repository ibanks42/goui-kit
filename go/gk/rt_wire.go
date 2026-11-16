package gk

// The wire (docs/PROTOCOLS.md section 2): little-endian writers on a byte slice and a reader.
// Strings read are copies (Go strings are immutable and the collector owns them), so text the
// library hands over is always safe to keep.

import (
	"encoding/binary"
	"math"
	"time"
	"unicode/utf8"
)

var le = binary.LittleEndian

// tape is a byte stream being written: a view's ops, a call op's request, a reply.
type tape []byte

func (o *tape) u8(v uint8)    { *o = append(*o, v) }
func (o *tape) u16(v uint16)  { *o = le.AppendUint16(*o, v) }
func (o *tape) u32(v uint32)  { *o = le.AppendUint32(*o, v) }
func (o *tape) u64(v uint64)  { *o = le.AppendUint64(*o, v) }
func (o *tape) f32(v float32) { *o = le.AppendUint32(*o, math.Float32bits(v)) }
func (o *tape) f64(v float64) { *o = le.AppendUint64(*o, math.Float64bits(v)) }
func (o *tape) bool(v bool) {
	if v {
		*o = append(*o, 1)
	} else {
		*o = append(*o, 0)
	}
}

// str writes a string: u32 length, UTF-8. Bytes that are not UTF-8 stop here (the library
// would show U+FFFD in place of the whole string).
func (o *tape) str(s string) {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			if !utf8.ValidString(s[i:]) {
				notUTF8(s)
			}
			break
		}
	}
	*o = le.AppendUint32(*o, uint32(len(s)))
	*o = append(*o, s...)
}

func (o *tape) bytes(b []byte) {
	*o = le.AppendUint32(*o, uint32(len(b)))
	*o = append(*o, b...)
}

func notUTF8(s string) {
	at := 0
	for at < len(s) {
		r, n := utf8.DecodeRuneInString(s[at:])
		if r == utf8.RuneError && n <= 1 {
			break
		}
		at += n
	}
	stop("a string given to the kit is not UTF-8 (byte %#02x at offset %d of %d: %q); text from outside the program can be made valid with strings.ToValidUTF8", s[at], at, len(s), s)
}

func newReq(op uint16) *tape {
	o := make(tape, 0, 32)
	o.u16(op)
	return &o
}

// reader reads a request, a reply or a call's result.
type reader struct {
	b []byte
	i int
}

func (r *reader) u8() uint8  { v := r.b[r.i]; r.i++; return v }
func (r *reader) bool() bool { return r.u8() != 0 }
func (r *reader) u16() uint16 {
	v := le.Uint16(r.b[r.i:])
	r.i += 2
	return v
}
func (r *reader) u32() uint32 {
	v := le.Uint32(r.b[r.i:])
	r.i += 4
	return v
}
func (r *reader) u64() uint64 {
	v := le.Uint64(r.b[r.i:])
	r.i += 8
	return v
}
func (r *reader) f32() float32 { return math.Float32frombits(r.u32()) }
func (r *reader) f64() float64 { return math.Float64frombits(r.u64()) }

// str reads a string as a copy.
func (r *reader) str() string {
	n := int(r.u32())
	s := string(r.b[r.i : r.i+n])
	r.i += n
	return s
}

func rdOpt[T any](r *reader, f func(r *reader) T) Option[T] {
	if r.u8() == 0 {
		return Option[T]{}
	}
	return Some(f(r))
}

func rdList[T any](r *reader, f func(r *reader) T) []T {
	n := int(r.u32())
	out := make([]T, n)
	for i := range out {
		out[i] = f(r)
	}
	return out
}

func rdRange(r *reader) Range {
	lo := int(r.u64())
	return Range{Lo: lo, Hi: int(r.u64())}
}

func rdDuration(r *reader) time.Duration { return time.Duration(r.u32()) * time.Millisecond }

// rdResult reads Result<(), E> as the error's text (PROTOCOLS.md 11.6): nil when it succeeded.
func rdResult(r *reader) error {
	if r.u8() == 0 {
		return nil
	}
	return &KitError{r.str()}
}

// KitError is an error an upstream call that may fail returned.
type KitError struct{ Text string }

func (e *KitError) Error() string { return e.Text }

// Option is an optional value: Some(v), or the zero Option for none.
type Option[T any] struct {
	v  T
	ok bool
}

// Some is an Option holding v.
func Some[T any](v T) Option[T] { return Option[T]{v, true} }

// None is the empty Option (the zero value is the same).
func None[T any]() Option[T] { return Option[T]{} }

// Get returns the value and whether there is one.
func (o Option[T]) Get() (T, bool) { return o.v, o.ok }

// Valid reports whether the Option holds a value.
func (o Option[T]) Valid() bool { return o.ok }
