package rt

func PutOptStrOpt(o *Tape, s Option[string]) {
	if v, ok := s.Get(); ok {
		o.U8(1)
		o.Str(v)
		return
	}
	o.U8(0)
}
