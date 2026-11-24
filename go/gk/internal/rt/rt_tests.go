package rt

// TestObjCounts returns the temporary handles waiting for the next frame and the live library
// objects (core/tests/go/handles).
func TestObjCounts(cx *Cx) (temps, live int) {
	CheckCx(cx).Flush()
	o := NewReq(CObjTestTemps)
	r := Reader{B: MustCall(*o)}
	return int(r.U32()), int(r.U32())
}

// TestValuePins returns the temporary handles pinned by views and the ones waiting for the next
// sweep (core/tests/go/values).
func TestValuePins(cx *Cx) (pinned, later int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CValuesTest))}
	return int(r.U32()), int(r.U32())
}

// TestEntityHandles reads one entity back through the library's two paths (the values path and
// the dock's) and again, and returns the three handles and how many distinct handles the table
// holds for it (core/tests/go/values).
func TestEntityHandles(cx *Cx) (valuesPath, dockPath, again, handles uint64) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CHandlesTest))}
	return r.U64(), r.U64(), r.U64(), r.U64()
}

// TestDragPayload has the library decode an external drag payload as an element's builder would and
// returns its files (core/tests/go/platform).
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

// TestPrepaintOrder has the library decode a dynamic prepaint order and returns it
// (core/tests/go/platform).
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

// PlatformLog is what the scripted platform recorded (core/tests/go/platform): the URLs opened, the
// requests made, answers delivered and discarded, requests waiting, and whether the platform is
// scripted.
type PlatformLog struct {
	URLs                           []string
	Requests, Delivered, Discarded uint64
	Waiting                        uint32
	Scripted                       bool
}

// GetPlatformLog reads the platform's log.
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

// TestEditorText returns the library's counts of the editor family: Markdown nodes the data plugins
// claimed, and rows made for the distinct values of view closures (core/tests/go/editor2).
func TestEditorText(cx *Cx) (parsed, viewRows int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CEditorTest))}
	return int(r.U64()), int(r.U64())
}

// TestEditorHostCounts returns editor hosts decoded, leaves collected, leaf events delivered,
// decorators run and lent values edited (core/tests/go/editor2).
func TestEditorHostCounts(cx *Cx) (hosts, leaves, delivered, decorated, lent int) {
	CheckCx(cx).Flush()
	r := Reader{B: MustCall(*NewReq(CEditorHostTest))}
	return int(r.U64()), int(r.U64()), int(r.U64()), int(r.U64()), int(r.U64())
}

// TestKeptCount returns the handlers the runtime keeps for the library (subscriptions, timers,
// handlers given to call ops).
func TestKeptCount() int { return keptLive() }
