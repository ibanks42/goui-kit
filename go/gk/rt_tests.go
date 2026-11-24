package gk

import (
	"github.com/ibanks42/goui-kit/gk/internal/rt"
)

// TestFileDropEntered triggers a synthetic file-drop-entered payload from the
// diagnostic library and decodes the resulting FileDropEvent.
func TestFileDropEntered(cx *Cx) FileDropEvent {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CPlatformTest)
	o.U8(5)
	r := rt.Reader{B: rt.MustCall(*o)}
	return rdFileDropEvent(&r)
}
