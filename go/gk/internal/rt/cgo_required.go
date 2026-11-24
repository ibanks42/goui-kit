//go:build gkstatic && !cgo

package rt

// Intentionally references an undefined identifier whose name explains the
// build error when -tags gkstatic is used with CGO_ENABLED=0.
var _ = gk_static_needs_cgo__install_gcc_or_clang_and_build_with_CGO_ENABLED_1_or_drop_tag_gkstatic
