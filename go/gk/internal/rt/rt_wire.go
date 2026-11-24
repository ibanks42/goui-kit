package rt

import (
	"encoding/binary"
	"math"
	"time"
	"unicode/utf8"
)

var le = binary.LittleEndian

// tape is a byte stream being written: a view's ops, a call op's request, a reply.
type Tape []byte

func (o *Tape) U8(v uint8) { *o = append(*o, v) }

func (o *Tape) U16(v uint16) { *o = le.AppendUint16(*o, v) }

func (o *Tape) U32(v uint32) { *o = le.AppendUint32(*o, v) }

func (o *Tape) U64(v uint64) { *o = le.AppendUint64(*o, v) }

func (o *Tape) F32(v float32) { *o = le.AppendUint32(*o, math.Float32bits(v)) }

func (o *Tape) F64(v float64) { *o = le.AppendUint64(*o, math.Float64bits(v)) }

func (o *Tape) Bool(v bool) {
	if v {
		*o = append(*o, 1)
	} else {
		*o = append(*o, 0)
	}
}

// str writes a string: u32 length, UTF-8. Bytes that are not UTF-8 stop here (the library
// would show U+FFFD in place of the whole string).
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

func (o *Tape) Bytes(b []byte) {
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
	Stop("a string given to the kit is not UTF-8 (byte %#02x at offset %d of %d: %q); text from outside the program can be made valid with strings.ToValidUTF8", s[at], at, len(s), s)
}

// reqBuf is a request with room for its first bytes: newReq costs one allocation, not two.
type reqBuf struct {
	t Tape
	b [48]byte
}

func NewReq(op uint16) *Tape {
	r := new(reqBuf)
	r.t = r.b[:0]
	r.t.U16(op)
	return &r.t
}

// reader reads a request, a reply or a call's result.
type Reader struct {
	B []byte
	i int
}

func (r *Reader) U8() uint8 { v := r.B[r.i]; r.i++; return v }

func (r *Reader) Bool() bool { return r.U8() != 0 }

func (r *Reader) U16() uint16 {
	v := le.Uint16(r.B[r.i:])
	r.i += 2
	return v
}

func (r *Reader) U32() uint32 {
	v := le.Uint32(r.B[r.i:])
	r.i += 4
	return v
}

func (r *Reader) U64() uint64 {
	v := le.Uint64(r.B[r.i:])
	r.i += 8
	return v
}

func (r *Reader) F32() float32 { return math.Float32frombits(r.U32()) }

func (r *Reader) F64() float64 { return math.Float64frombits(r.U64()) }

// str reads a string as a copy.
func (r *Reader) Str() string {
	n := int(r.U32())
	s := string(r.B[r.i : r.i+n])
	r.i += n
	return s
}

func RdOpt[T any](r *Reader, f func(r *Reader) T) Option[T] {
	if r.U8() == 0 {
		return Option[T]{}
	}
	return Some(f(r))
}

func RdList[T any](r *Reader, f func(r *Reader) T) []T {
	n := int(r.U32())
	out := make([]T, n)
	for i := range out {
		out[i] = f(r)
	}
	return out
}

func RdRange(r *Reader) Range {
	lo := int(r.U64())
	return Range{Lo: lo, Hi: int(r.U64())}
}

func RdDuration(r *Reader) time.Duration { return time.Duration(r.U32()) * time.Millisecond }

// rdResult reads Result<(), E> as the error's text (PROTOCOLS.md 11.6): nil when it succeeded.
func RdResult(r *Reader) error {
	if r.U8() == 0 {
		return nil
	}
	return &KitError{r.Str()}
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
