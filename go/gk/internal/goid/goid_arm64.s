//go:build arm64

#include "textflag.h"

// func Current() uintptr
// Reads the current goroutine's g pointer from the dedicated arm64 g register
// so the UI-thread check can identify the calling goroutine without overhead.
TEXT ·Current(SB), NOSPLIT, $0-8
	MOVD g, ret+0(FP)
	RET
