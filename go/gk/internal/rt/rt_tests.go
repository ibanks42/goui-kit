package rt

// TestObjCounts queries the diagnostic library for the number of frame-temporary
// object handles and total live object handles currently held in Rust.
func TestObjCounts(cx *Cx) (temps, live int) {
	CheckCx(cx).Flush()
	o := NewReq(CObjTestTemps)
	r := Reader{B: MustCall(*o)}
	return int(r.U32()), int(r.U32())
}

// TestValuePins returns diagnostic counters for pinned value objects and values
// scheduled for deferred cleanup in Rust.
func TestValuePins(cx *Cx) (pinned, later int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CValuesTest))}
	return int(r.U32()), int(r.U32())
}

// TestEntityHandles exercises entity-handle identity round-trips across the
// values and dock paths in the diagnostic library.
func TestEntityHandles(cx *Cx) (valuesPath, dockPath, again, handles uint64) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CHandlesTest))}
	return r.U64(), r.U64(), r.U64(), r.U64()
}

// TestDragPayload encodes p across the C ABI and decodes the round-tripped
// ExternalFile slice returned by the diagnostic library.
func TestDragPayload(cx *Cx, p ExternalDragPayload) []ExternalFile {
	CheckCx(cx).Flush()
	o := NewReq(CPlatformTest)
	o.U8(2)
	PutExternalDragPayload(o, p)
	r := Reader{B: MustCall(*o)}
	out := make([]ExternalFile, int(r.U32()))
	for i := range out {
		out[i].Path = r.Str()
		out[i].Directory = r.Bool()
	}
	return out
}

// TestPrepaintOrder sends an index order to the diagnostic prepaint hook and
// returns the order observed by the Rust core.
func TestPrepaintOrder(cx *Cx, order []int) []int {
	CheckCx(cx).Flush()
	o := NewReq(CPlatformTest)
	o.U8(3)
	o.U32(uint32(len(order)))
	for _, x := range order {
		o.U64(uint64(x))
	}
	r := Reader{B: MustCall(*o)}
	out := make([]int, int(r.U32()))
	for i := range out {
		out[i] = int(r.U64())
	}
	return out
}

// PlatformLog captures diagnostic counters for URL opens and asynchronous
// platform dialog requests recorded by the diagnostic library.
type PlatformLog struct {
	URLs                           []string
	Requests, Delivered, Discarded uint64
	Waiting                        uint32
	Scripted                       bool
}

// GetPlatformLog fetches the current PlatformLog snapshot from the diagnostic library.
func GetPlatformLog(cx *Cx) PlatformLog {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CPlatformLog))}
	var l PlatformLog
	l.URLs = make([]string, int(r.U32()))
	for i := range l.URLs {
		l.URLs[i] = r.Str()
	}
	l.Requests, l.Delivered, l.Discarded = r.U64(), r.U64(), r.U64()
	l.Waiting = r.U32()
	l.Scripted = r.Bool()
	return l
}

// TestEditorText queries the diagnostic library for the number of parsed
// document blocks and materialized editor view rows.
func TestEditorText(cx *Cx) (parsed, viewRows int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CEditorTest))}
	return int(r.U64()), int(r.U64())
}

// TestEditorHostCounts returns diagnostic counters for live editor hosts,
// leaf views, delivered events, decorated spans, and active lent edits.
func TestEditorHostCounts(cx *Cx) (hosts, leaves, delivered, decorated, lent int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CEditorHostTest))}
	return int(r.U64()), int(r.U64()), int(r.U64()), int(r.U64()), int(r.U64())
}

// TestKeptCount returns the number of currently live long-lived handler slots
// in keptTab.
func TestKeptCount() int { return keptLive() }
