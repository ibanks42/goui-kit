//go:build !cgo

package rt

// This file is compiled only when cgo is off (CGO_ENABLED=0, or no C compiler on PATH, in which
// case Go turns cgo off by itself). Package gk calls the library through cgo, so the build stops
// on the line below, whose name says what to do, before the undefined names that follow from it.
var _ = gk_needs_cgo__install_gcc_or_clang_and_build_with_CGO_ENABLED_1
