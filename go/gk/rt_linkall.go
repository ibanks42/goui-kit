package gk

// extern void gk_use_all(void);
import "C"

// LinkAll links every GPUI-kit component family into the program and registers them all.
//
// A Go program built against the static library (the default) links a family only when
// it calls into it: gk.Button links the button family, gk.NewInputState the input family, and the
// linker leaves out the rest (with -ldflags=-extldflags=-Wl,--gc-sections, -Wl,-dead_strip on macOS;
// docs/GO.md section 8).
// LinkAll is the escape hatch for a program that wants everything, at the size of everything. It
// lives in a file of its own so that a program that never calls it does not link every family.
func LinkAll() { C.gk_use_all() }
