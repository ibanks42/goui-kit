package gk

import (
	"gpuikit.dev/gk/internal/rt"
)

// TestFileDropEntered returns gpui's FileDropEvent::Entered as the library writes a handler's
// payload (core/tests/go/platform).
func TestFileDropEntered(cx *Cx) FileDropEvent {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CPlatformTest)
	o.U8(5)
	r := rt.Reader{B: rt.MustCall(*o)}
	return rdFileDropEvent(&r)
}
