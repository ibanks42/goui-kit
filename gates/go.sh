#!/bin/bash
# go.sh [PHASE...]: the Go front end's gate rows (docs/GO.md). Phases:
#   build    examples/go/build.sh hello counter panels proto misuse paint port (timed; GOCACHE reused)
#   hello    the README's app in Go offscreen, 30 frames, exit 0
#   proto    gates/go-proto.sh, the protocol tests in Go
#   misuse   gates/go-misuse.sh, the misuse cases (run time and compile time)
#   pixels   counter, panels and paint, Rust against Go, byte for byte (gates/pixels.sh)
#   fps      frame cost, panels 60 64 2 and the counter's click on every frame: Rust, Go, Odin
#            -o:speed, Rust again (gates/fps.sh, judged at 1.15 x Rust: docs/GO.md)
#   rebuild  go build after a one-line edit of an app (the edit loop)
# Default: build hello proto misuse pixels. Each phase prints lines starting with GATE.
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
. "$repo/go/env.sh"
out=$here/out/go; mkdir -p "$out"
lib=${GK_LIB_ROOT:-$repo/lib}
[ $# -gt 0 ] || set -- build hello proto misuse pixels
rc=0
for phase in "$@"; do
  case "$phase" in
  build)
    t0=$(date +%s.%N)
    if GK_LIB=$lib/ship "$repo/examples/go/build.sh" hello counter panels proto misuse paint port > "$out/build.log" 2>&1; then
      echo "GATE build go PASS 7 programs in $(printf %.1f "$(echo "$(date +%s.%N) - $t0" | bc)") s (go build, GOCACHE kept)"
    else
      echo "GATE build go FAIL"; tail -20 "$out/build.log"; rc=1
    fi ;;
  hello)
    s=$out/hello.script; { echo "size 480 320"; for _ in $(seq 30); do echo frame; done; echo "shot hello.png"; echo stat; } > "$s"
    (cd "$out" && LD_LIBRARY_PATH=$lib/diag GK_SCRIPT=$s timeout 60 "$repo/bin/hello-go" > hello.log 2>&1)
    r=$?
    # README.md's Go block is cut from examples/go/hello/main.go: every line of it is a line there.
    awk '/^```go$/{f=1;next} /^```$/{if(f)exit} f' "$repo/README.md" | while IFS= read -r l; do
      [ -z "$l" ] || grep -Fxq -- "$l" "$repo/examples/go/hello/main.go" || echo "$l"; done > "$out/readme-missing.txt"
    [ -s "$out/readme-missing.txt" ] && { echo "GATE readme go FAIL README.md's Go lines not in examples/go/hello/main.go:"; cat "$out/readme-missing.txt"; rc=1; } \
      || echo "GATE readme go PASS README.md's Go block is examples/go/hello/main.go's"
    [ $r = 0 ] && echo "GATE hello go PASS 30 frames offscreen, exit 0" || { echo "GATE hello go FAIL exit $r"; tail -5 "$out/hello.log"; rc=1; } ;;
  proto)
    "$here/go-proto.sh" | tee "$out/proto.txt" | grep -E '^(FAIL|GATE)'; [ "${PIPESTATUS[0]}" = 0 ] || rc=1 ;;
  misuse)
    "$here/go-misuse.sh" | tee "$out/misuse.txt" | grep -E '^(FAIL|GATE)'; [ "${PIPESTATUS[0]}" = 0 ] || rc=1 ;;
  pixels)
    for app in counter panels paint; do
      GK_PIXEL_CANDS=go "$here/pixels.sh" "$app" > "$out/pixels-$app.txt" 2>&1; r=$?
      grep '^GATE' "$out/pixels-$app.txt"; [ $r = 0 ] || { grep -v '^GATE' "$out/pixels-$app.txt" | tail -8; rc=1; }
    done ;;
  fps)
    # The Odin apps of the same scenes, -o:speed, against this library (the comparison column).
    OPT=-o:speed SUFFIX=-speed GK_LIB=$lib/ship "$repo/examples/odin/build.sh" panels counter > "$out/odin-build.log" 2>&1 || { echo "GATE fps go FAIL the Odin apps do not build"; rc=1; }
    GK_NAME=panels-go-60-64-2 BINS="rust go odin-speed rust2" GK_JUDGED=go GK_BAR=1.15 "$here/fps.sh" --rounds ${GK_ROUNDS:-6} panels 60 64 2 > "$out/fps-panels.txt" 2>&1 || rc=1
    grep -E '^GATE|^\| (cpu_p50_us|cpu_p95_us) ' "$out/fps-panels.txt"
    GK_NAME=counter-go-click GK_CLICK=40,32,90,1 BINS="rust go odin-speed rust2" GK_JUDGED=go GK_BAR=1.15 "$here/fps.sh" --rounds ${GK_ROUNDS:-6} counter > "$out/fps-counter.txt" 2>&1 || rc=1
    grep -E '^GATE|^\| (cpu_p50_us|cpu_p95_us) ' "$out/fps-counter.txt" ;;
  rebuild)
    "$here/go-rebuild.sh" | tee "$out/rebuild.txt" | grep '^GATE' ;;
  *) echo "go.sh: unknown phase $phase" >&2; rc=2 ;;
  esac
done
exit $rc
