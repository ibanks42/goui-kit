package rt

import (
	"encoding/binary"
	"math"
	"time"
	"unicode/utf8"
)

var le = binary.LittleEndian

// Tape is an append-only little-endian byte buffer used to encode requests,
// view descriptions, and turn replies sent across the C ABI.
type Tape []byte

// U8 appends an unsigned 8-bit integer.
func (o *Tape) U8(v uint8) { *o = append(*o, v) }

// U16 appends a little-endian uint16.
func (o *Tape) U16(v uint16) { *o = le.AppendUint16(*o, v) }

// U32 appends a little-endian uint32.
func (o *Tape) U32(v uint32) { *o = le.AppendUint32(*o, v) }

// U64 appends a little-endian uint64.
func (o *Tape) U64(v uint64) { *o = le.AppendUint64(*o, v) }

// F32 appends an IEEE 754 float32 in little-endian bit order.
func (o *Tape) F32(v float32) { *o = le.AppendUint32(*o, math.Float32bits(v)) }

// F64 appends an IEEE 754 float64 in little-endian bit order.
func (o *Tape) F64(v float64) { *o = le.AppendUint64(*o, math.Float64bits(v)) }

// Bool appends 1 for true or 0 for false.
func (o *Tape) Bool(v bool) {
	if v {
		*o = append(*o, 1)
	} else {
		*o = append(*o, 0)
	}
}

// Str validates that s is valid UTF-8 and appends its uint32 byte length
// followed by its bytes. Pure ASCII strings pass without calling utf8.ValidString.
func (o *Tape) Str(s string) {
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

// Bytes appends a uint32 byte length followed by raw bytes without UTF-8 validation.
func (o *Tape) Bytes(b []byte) {
	*o = le.AppendUint32(*o, uint32(len(b)))
	*o = append(*o, b...)
}

// notUTF8 locates the first invalid byte in s and aborts the turn with a
// descriptive error.
func notUTF8(s string) {
	at := 0
	for at < len(s) {
		r, n := utf8.DecodeRuneInString(s[at:])
		if r == utf8.RuneError && n <= 1 {
			break
		}
		at += n
	}
	Stop("a string given to the kit is not UTF-8 (byte %#02x at offset %d of %d: %q); text from outside the program can be made valid with strings.ToValidUTF8", s[at], at, len(s), s)
}

// reqBuf pairs a Tape header with an inline 48-byte backing array so small
// requests stay on the stack when escape analysis allows it.
type reqBuf struct {
	t Tape
	b [48]byte
}

// NewReq returns a Tape pre-populated with call opcode op.
func NewReq(op uint16) *Tape {
	r := new(reqBuf)
	r.t = r.b[:0]
	r.t.U16(op)
	return &r.t
}

// Reader decodes little-endian values sequentially from a byte slice received
// from the Rust core.
type Reader struct {
	B []byte
	i int
}

// U8 reads one byte.
func (r *Reader) U8() uint8 { v := r.B[r.i]; r.i++; return v }

// Bool reads one byte and interprets non-zero as true.
func (r *Reader) Bool() bool { return r.U8() != 0 }

// U16 reads a little-endian uint16.
func (r *Reader) U16() uint16 {
	v := le.Uint16(r.B[r.i:])
	r.i += 2
	return v
}

// U32 reads a little-endian uint32.
func (r *Reader) U32() uint32 {
	v := le.Uint32(r.B[r.i:])
	r.i += 4
	return v
}

// U64 reads a little-endian uint64.
func (r *Reader) U64() uint64 {
	v := le.Uint64(r.B[r.i:])
	r.i += 8
	return v
}

// F32 reads an IEEE 754 float32.
func (r *Reader) F32() float32 { return math.Float32frombits(r.U32()) }

// F64 reads an IEEE 754 float64.
func (r *Reader) F64() float64 { return math.Float64frombits(r.U64()) }

// Str reads a uint32-length-prefixed UTF-8 string.
func (r *Reader) Str() string {
	n := int(r.U32())
	s := string(r.B[r.i : r.i+n])
	r.i += n
	return s
}

// RdOpt decodes a presence byte followed by f(r) when present.
func RdOpt[T any](r *Reader, f func(r *Reader) T) Option[T] {
	if r.U8() == 0 {
		return Option[T]{}
	}
	return Some(f(r))
}

// RdList decodes a uint32 element count followed by that many items decoded with f.
func RdList[T any](r *Reader, f func(r *Reader) T) []T {
	n := int(r.U32())
	out := make([]T, n)
	for i := range out {
		out[i] = f(r)
	}
	return out
}

// RdRange decodes two uint64 endpoints into a Range.
func RdRange(r *Reader) Range {
	lo := int(r.U64())
	return Range{Lo: lo, Hi: int(r.U64())}
}

// RdDuration decodes a uint32 millisecond count into a time.Duration.
func RdDuration(r *Reader) time.Duration { return time.Duration(r.U32()) * time.Millisecond }

// RdResult decodes a status byte and, when non-zero, an error string as *KitError.
func RdResult(r *Reader) error {
	if r.U8() == 0 {
		return nil
	}
	return &KitError{r.Str()}
}

// KitError wraps an error string returned by an underlying Rust gpui-kit operation.
type KitError struct{ Text string }

// Error returns the error message from the kit.
func (e *KitError) Error() string { return e.Text }

// Option represents an optional value of type T.
type Option[T any] struct {
	v  T
	ok bool
}

// Some returns a present Option holding v.
func Some[T any](v T) Option[T] { return Option[T]{v, true} }

// None returns an empty Option of type T.
func None[T any]() Option[T] { return Option[T]{} }

// Get returns the stored value and whether the Option is present.
func (o Option[T]) Get() (T, bool) { return o.v, o.ok }

// Valid reports whether the Option holds a value.
func (o Option[T]) Valid() bool { return o.ok }
