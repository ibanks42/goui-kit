#!/bin/bash
# go-gensample.sh: the sample by generation in Go, against the real runtime (the twin of
# gates/gensample.sh). core/tests/go/gensample is written by `make gen` (gen/gk_tests_go.py): one
# view function that calls every constructor and builder the Go package generated once. Built and
# run offscreen with the diag library, it shows that every generated function exists, compiles
# against the arguments its parameters take, and writes a tape the runtime accepts, with every
# handler registration. The elements are not attached to the view's root, so the library does not
# decode them: what the library makes of each is the gallery's business.
#   gates/go-gensample.sh
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
. "$repo/go/env.sh"
lib=${GK_LIB_ROOT:-$repo/lib}/diag
bin=${GK_BIN:-$repo/bin}
out=$here/out/go-gensample; rm -rf "$out"; mkdir -p "$out" "$bin"
export LD_LIBRARY_PATH=$lib
unset WAYLAND_DISPLAY DISPLAY
start=$(date +%s.%N)
rm -f "$bin/gensample-go"
if ! (cd "$repo/core/tests/go" && CGO_LDFLAGS="-L$lib -Wl,-rpath,\$ORIGIN/../lib/diag" go build -o "$bin/gensample-go" ./gensample) \
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
