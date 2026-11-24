#!/bin/bash
# README Go snippet freshness and runtime gate (`gates/readme-go.sh`).
# Gates: keeping README.md's first fenced Go block in sync with examples/go/hello/main.go and
# verifying that the example builds against lib/ship and renders live frames.
# Method: checks every non-empty line of the README Go block against examples/go/hello/main.go,
# compiles hello-go-readme, and runs it for 5 warmup + 30 measured frames inside gates/display.sh.
# PASS: all README lines match, the binary builds, exits 0, and emits a `gk-meter` summary line.

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
out=$here/out/readme-go; rm -rf "$out"; mkdir -p "$out"
src=$repo/examples/go/hello/main.go
awk '/^```go$/{f=1;next} /^```$/{if(f)exit} f' "$repo/README.md" > "$out/readme.go"
[ -s "$out/readme.go" ] || { echo "GATE readme example-go FAIL README.md has no Go block"; exit 1; }
while IFS= read -r l; do [ -z "$l" ] || grep -Fxq -- "$l" "$src" || echo "$l"; done < "$out/readme.go" > "$out/missing.txt"
if [ -s "$out/missing.txt" ]; then
  echo "GATE readme example-go FAIL README.md's Go lines not in examples/go/hello/main.go:"; cat "$out/missing.txt"; exit 1
fi
lib=${GK_LIB_ROOT:-$repo/lib}/ship; bin=${GK_BIN:-$repo/bin}; mkdir -p "$bin"
if ! GK_LIB=$lib OUT=$bin SUFFIX=-readme "$repo/examples/go/build.sh" hello > "$out/build.log" 2>&1; then
  echo "GATE readme example-go FAIL it does not build"; head -5 "$out/build.log"; exit 1
fi
LD_LIBRARY_PATH=$lib GK_FRAMES=5,30 "$here/display.sh" timeout 60 "$bin/hello-go-readme" > "$out/run.log" 2>&1; rc=$?
if [ $rc -eq 0 ] && grep -q '^gk-meter' "$out/run.log"; then
  echo "GATE readme example-go PASS $(wc -l < "$out/readme.go") lines, all in examples/go/hello/main.go; builds and draws 30 frames"
else
  echo "GATE readme example-go FAIL exit $rc when run"; tail -5 "$out/run.log"; exit 1
fi
