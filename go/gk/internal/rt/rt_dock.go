package rt

// PutOptStrOpt encodes an Option[string] onto o as a 1-byte presence tag
// followed by the string when present.
func PutOptStrOpt(o *Tape, s Option[string]) {
	if v, ok := s.Get(); ok {
		o.U8(1)
		o.Str(v)
		return
	}
	o.U8(0)
}
