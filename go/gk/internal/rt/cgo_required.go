//go:build gkstatic && !cgo

package rt

// This file is compiled only for the static build (build tag gkstatic) when cgo is off
// (CGO_ENABLED=0, or no C compiler on PATH, in which case Go turns cgo off by itself). The static
// build links the library through cgo, so it stops on the line below, whose name says what to do,
// before the undefined names that follow from it. The default build needs no C compiler.
var _ = gk_static_needs_cgo__install_gcc_or_clang_and_build_with_CGO_ENABLED_1_or_drop_tag_gkstatic
