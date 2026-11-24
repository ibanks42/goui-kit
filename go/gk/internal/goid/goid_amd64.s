//go:build amd64

#include "textflag.h"

// func Current() uintptr
// Loads g through the TLS pseudo-register, which the assembler supports for
// ABI0 functions on every amd64 target (R14 holds g only under ABIInternal).
TEXT ·Current(SB), NOSPLIT, $0-8
	MOVQ (TLS), AX
	MOVQ AX, ret+0(FP)
	RET
