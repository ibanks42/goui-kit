#!/bin/bash
# Generated-bindings compile and runtime smoke gate (`gates/go-gensample.sh`).
#
# What it gates:
#   Exercises the generated Go API surface at scale by compiling and running
#   `core/tests/go/gensample` (which invokes tens of thousands of generated builder calls across
#   hundreds of node types) against `lib/diag`.
#
# How it measures:
#   Removes any stale `bin/gensample-go`, times a fresh `go build`, and runs the binary offscreen
#   (display vars cleared, 120 s timeout) capturing `gensample: <N> calls ...` and `done runs=`.
# What PASS means (exit 0): compiles with zero errors, exits 0, and logs `gensample: done runs>=1`.

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
. "$repo/go/env.sh"
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$here/out/go-gensample; rm -rf "$out"; mkdir -p "$out" "$bin"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
start=$(date +%s.%N)
rm -f "$bin/gensample-go"
if ! (cd "$repo/core/tests/go" && go build -o "$bin/gensample-go" ./gensample) \
    > "$out/build.log" 2>&1; then
  echo "GATE gensample go FAIL the sample does not compile: $(grep -c ': ' "$out/build.log") errors"; head -5 "$out/build.log"; exit 1
fi
built=$(echo "$(date +%s.%N) - $start" | bc | cut -c1-4)
timeout 120 "$bin/gensample-go" > "$out/run.log" 2>&1; rc=$?
line=$(grep -m1 '^gensample: [0-9]* calls' "$out/run.log")
if [ $rc -eq 0 ] && grep -q '^gensample: done runs=[1-9]' "$out/run.log"; then
  echo "GATE gensample go PASS ${line#gensample: }; built in $built s"
else
  echo "GATE gensample go FAIL exit $rc"; tail -5 "$out/run.log"; exit 1
fi
