package editor

import "gpuikit.dev/gk/internal/rt"

// TestInputState makes an InputState of any handle (tests only: one of another kind shows the
// library's type check).
func TestInputState(h uint64) InputState { return InputState{HCore: rt.MkH(h)} }
