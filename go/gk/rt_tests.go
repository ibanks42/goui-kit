package gk

// Test hooks of the protocol suites (core/tests/go): reads of the runtime's and the library's
// bookkeeping that only a test asks for.

// TestObjCounts returns the temporary handles waiting for the next frame and the live library
// objects (core/tests/go/handles).
func TestObjCounts(cx *Cx) (temps, live int) {
	checkCx(cx).flush()
	o := newReq(cObjTestTemps)
	r := reader{b: mustCall(*o)}
	return int(r.u32()), int(r.u32())
}

// TestValuePins returns the temporary handles pinned by views and the ones waiting for the next
// sweep (core/tests/go/values).
func TestValuePins(cx *Cx) (pinned, later int) {
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cValuesTest))}
	return int(r.u32()), int(r.u32())
}

// TestEntityHandles reads one entity back through the library's two paths (the values path and
// the dock's) and again, and returns the three handles and how many distinct handles the table
// holds for it (core/tests/go/values).
func TestEntityHandles(cx *Cx) (valuesPath, dockPath, again, handles uint64) {
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cHandlesTest))}
	return r.u64(), r.u64(), r.u64(), r.u64()
}

// TestDragPayload has the library decode an external drag payload as an element's builder would and
// returns its files (core/tests/go/platform).
func TestDragPayload(cx *Cx, p ExternalDragPayload) []ExternalFile {
	checkCx(cx).flush()
	o := newReq(cPlatformTest)
	o.u8(2)
	putExternalDragPayload(o, p)
	r := reader{b: mustCall(*o)}
	out := make([]ExternalFile, int(r.u32()))
	for i := range out {
		out[i].Path = r.str()
		out[i].Directory = r.bool()
	}
	return out
}

// TestPrepaintOrder has the library decode a dynamic prepaint order and returns it
// (core/tests/go/platform).
func TestPrepaintOrder(cx *Cx, order []int) []int {
	checkCx(cx).flush()
	o := newReq(cPlatformTest)
	o.u8(3)
	o.u32(uint32(len(order)))
	for _, x := range order {
		o.u64(uint64(x))
	}
	r := reader{b: mustCall(*o)}
	out := make([]int, int(r.u32()))
	for i := range out {
		out[i] = int(r.u64())
	}
	return out
}

// TestFileDropEntered returns gpui's FileDropEvent::Entered as the library writes a handler's
// payload (core/tests/go/platform).
func TestFileDropEntered(cx *Cx) FileDropEvent {
	checkCx(cx).flush()
	o := newReq(cPlatformTest)
	o.u8(5)
	r := reader{b: mustCall(*o)}
	return rdFileDropEvent(&r)
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
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cPlatformLog))}
	var l PlatformLog
	l.URLs = make([]string, int(r.u32()))
	for i := range l.URLs {
		l.URLs[i] = r.str()
	}
	l.Requests, l.Delivered, l.Discarded = r.u64(), r.u64(), r.u64()
	l.Waiting = r.u32()
	l.Scripted = r.bool()
	return l
}

// TestEditorText returns the library's counts of the editor family: Markdown nodes the data plugins
// claimed, and rows made for the distinct values of view closures (core/tests/go/editor2).
func TestEditorText(cx *Cx) (parsed, viewRows int) {
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cEditorTest))}
	return int(r.u64()), int(r.u64())
}

// TestEditorHostCounts returns editor hosts decoded, leaves collected, leaf events delivered,
// decorators run and lent values edited (core/tests/go/editor2).
func TestEditorHostCounts(cx *Cx) (hosts, leaves, delivered, decorated, lent int) {
	checkCx(cx).flush()
	r := reader{b: mustCall(*newReq(cEditorHostTest))}
	return int(r.u64()), int(r.u64()), int(r.u64()), int(r.u64()), int(r.u64())
}

// TestKeptCount returns the handlers the runtime keeps for the library (subscriptions, timers,
// handlers given to call ops).
func TestKeptCount() int { return keptLive() }
