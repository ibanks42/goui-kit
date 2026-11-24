package editor

import "gpuikit.dev/gk/internal/rt"

// TestInputState wraps a raw handle h as an InputState for misuse and protocol tests.
func TestInputState(h uint64) InputState { return InputState{HCore: rt.MkH(h)} }
