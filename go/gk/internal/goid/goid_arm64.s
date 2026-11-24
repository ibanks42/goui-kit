//go:build arm64

#include "textflag.h"

// func Current() uintptr
// Reads the current goroutine pointer (g) from R28 (g).
TEXT ·Current(SB), NOSPLIT, $0-8
	MOVD g, ret+0(FP)
	RET
