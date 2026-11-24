# The protocol tests in Go

The Go port of the runtime's protocol test suites (the Odin programs under `core/tests/*` and
their runners `core/tests/run-*.sh`). Each Odin family `core/tests/FAMILY` has a Go twin
`core/tests/go/FAMILY` (one `package main` in the module `gpuikit.dev/coretests`, which replaces
`gpuikit.dev` with `../../../go`), and each runner fragment `core/tests/run-FAMILY.sh` has a twin
`core/tests/go/run-FAMILY.sh` with the same modes, exit codes and patterns.

```sh
make go-proto-all                       # gates/go-proto.sh (examples/go/proto) and this suite
core/tests/go/run.sh                    # the families only
core/tests/go/run.sh actions timers     # some modes
GK_FAMILIES="rows input" core/tests/go/run.sh
```

Needs `lib/diag` from `make lib`; binaries go to `bin/FAMILY-go`, logs to
`gates/out/go-proto-families/MODE.log`. The last line is
`GATE proto-families go PASS|FAIL n of m protocol tests`.

## Rules of the port

- The same lines: a Go test prints what its Odin twin prints, so a pattern is the Odin pattern.
  Where the Go package names a thing differently (an enum value `RightClick` for Odin's
  `Right_Click`, a source line `at .*main.go:N`, Go's misuse messages "a view function" for
  Odin's "a view procedure"), the Go pattern says so in a comment beginning `Go:`.
- Odin's runtime misuse checks that are compile errors in Go (a handler of the wrong shape) are
  examples/go/misuse_compile's business (gates/go-misuse.sh); such a mode has no Go twin and the
  fragment says so in a comment. So is a misuse the Go API cannot express (lists2's bad-ctx:
  gk.row_context for a row no closure drew; an item function made by gk.Items is handed its
  values, there is no read of a context).
- Item functions: where Odin reads a row's context with `gk.row_context(cx, src, at, T)`, Go hands
  the values to the function gk.Items made the source of (`func(cx *gk.Cx, ctx gk.ResizeHandleContext)
  gk.Element`); such a function gets no row number, so a check that prints Odin's `at.row` prints
  the row the test knows (one item: 0) or numbers the rows in the order they are first described.
- Generated pieces (`gen`, `gensample`, `app/gen_ambient.go`) are written by `make gen`
  (gen/gk_tests_go.py), like their Odin twins (gen/gk_tests.py).
