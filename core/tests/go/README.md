# The protocol tests in Go

The runtime's protocol test suites in Go. Each family is a test app `core/tests/go/FAMILY`
(one `package main` in the module `gpuikit.dev/coretests`, which replaces `gpuikit.dev` with
`../../../go`), and its runner fragment `core/tests/go/run-FAMILY.sh` holds its modes, exit
codes and patterns.

```sh
make go-proto-all                       # gates/go-proto.sh (examples/go/proto) and this suite
core/tests/go/run.sh                    # the families only
core/tests/go/run.sh actions timers     # some modes
GK_FAMILIES="rows input" core/tests/go/run.sh
```

Needs `lib/diag` from `make lib`; binaries go to `bin/FAMILY-go`, logs to
`gates/out/go-proto-families/MODE.log`. The last line is
`GATE proto-families go PASS|FAIL n of m protocol tests`.

## Rules

- The lines are the patterns: a Go test prints the lines its fragment checks. Where a pattern is
  specific to the Go package (an enum value `RightClick`, a source line `at .*main.go:N`, Go's
  misuse messages "a view function"), the fragment says so in a comment beginning `Go:`.
- Runtime misuse checks that are compile errors in Go (a handler of the wrong shape) are
  examples/go/misuse_compile's business (gates/go-misuse.sh); the fragment says so in a comment.
  So is a misuse the Go API cannot express (lists2's bad-ctx: a row's context read for a row no
  closure drew; an item function made by gk.Items is handed its values, there is no read of a
  context).
- Item functions: Go hands the values to the function gk.Items made the source of
  (`func(cx *gk.Cx, ctx gk.ResizeHandleContext) gk.Element`); such a function gets no row number,
  so a check that prints a row prints the row the test knows (one item: 0) or numbers the rows in
  the order they are first described.
- Generated pieces (`gen`, `gensample`, `app/gen_ambient.go`) are written by `make gen`
  (gen/gk_tests_go.py).
