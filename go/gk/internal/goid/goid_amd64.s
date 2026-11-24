//go:build amd64

#include "textflag.h"

// func Current() uintptr
// Reads the current goroutine's g pointer directly from Go's TLS slot so the
// UI-thread check can compare goroutine identity without calling into the
// runtime or allocating.
TEXT ·Current(SB), NOSPLIT, $0-8
	MOVQ (TLS), AX
	MOVQ AX, ret+0(FP)
	RET
