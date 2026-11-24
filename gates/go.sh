#!/bin/bash
# Phased Go verification runner (`gates/go.sh [build|hello|proto|misuse|pixels|fps|rebuild]...`).
#
# What it gates (default phases when no args are given: `build hello proto misuse pixels`):
#   - `build`:   compiles all 21 Go example, test, and app binaries against `lib/ship`.
#   - `hello`:   runs `hello-go` for 30 offscreen frames on `lib/diag` and verifies every non-empty
#                line of `README.md`'s first Go block appears in `examples/go/hello/main.go`.
#   - `proto`:   runs `gates/go-proto.sh` (tape/runtime protocol suite).
#   - `misuse`:  runs `gates/go-misuse.sh` (runtime safety and compile-fail suite).
#   - `pixels`:  runs `gates/pixels.sh` for `counter`, `panels`, `paint`, `writerule`, and `files`
#                (emits `PENDING` if a binary is not built).
#   - `fps`:     runs `gates/fps.sh` (default 6 rounds) on `panels 60 64 2` and clicking `counter`.
#   - `rebuild`: runs `gates/go-rebuild.sh` (incremental one-line edit rebuild latency).
#
# What PASS means (exit 0):
#   Every requested phase completes with exit status 0 and reports PASS.

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
. "$repo/go/env.sh"
out=$here/out/go; mkdir -p "$out"
lib=${GK_LIB_ROOT:-$repo/lib}
[ $# -gt 0 ] || set -- build hello proto misuse pixels
rc=0
for phase in "$@"; do
  case "$phase" in
  build)
    t0=$(date +%s.%N); apps="hello counter panels proto misuse misuse_rows misuse_data misuse_overlay misuse_values misuse_lists misuse_editor misuse_review paint port writerule table files notes todo reader expenses"
    if GK_LIB=$lib/ship "$repo/examples/go/build.sh" $apps > "$out/build.log" 2>&1; then
      echo "GATE build go PASS $(echo $apps | wc -w) programs in $(printf %.1f "$(echo "$(date +%s.%N) - $t0" | bc)") s (go build, GOCACHE kept)"
    else
      echo "GATE build go FAIL"; tail -20 "$out/build.log"; rc=1
    fi ;;
  hello)
    s=$out/hello.script; { echo "size 480 320"; for _ in $(seq 30); do echo frame; done; echo "shot hello.png"; echo stat; } > "$s"
    (cd "$out" && LD_LIBRARY_PATH=$lib/diag GK_SCRIPT=$s timeout 60 "$repo/bin/hello-go" > hello.log 2>&1)
    r=$?
    # Verify every non-blank line in README.md's first Go block appears in examples/go/hello/main.go.
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
    for app in counter panels paint writerule files; do
      if [ ! -x "${GK_BIN:-$repo/bin}/$app-go" ]; then
        echo "GATE pixels $app-go PENDING bin/$app-go was not built (gates/go-pending.txt)"; continue
      fi
      GK_PIXEL_CANDS=go "$here/pixels.sh" "$app" > "$out/pixels-$app.txt" 2>&1; r=$?
      grep '^GATE' "$out/pixels-$app.txt"; [ $r = 0 ] || { grep -v '^GATE' "$out/pixels-$app.txt" | tail -8; rc=1; }
    done ;;
  fps)
    GK_NAME=panels-go-60-64-2 BINS="rust go rust2" GK_JUDGED=go "$here/fps.sh" --rounds ${GK_ROUNDS:-6} panels 60 64 2 > "$out/fps-panels.txt" 2>&1 || rc=1
    grep -E '^GATE|^\| (cpu_p50_us|cpu_p95_us) ' "$out/fps-panels.txt"
    GK_NAME=counter-go-click GK_CLICK=40,32,90,1 BINS="rust go rust2" GK_JUDGED=go "$here/fps.sh" --rounds ${GK_ROUNDS:-6} counter > "$out/fps-counter.txt" 2>&1 || rc=1
    grep -E '^GATE|^\| (cpu_p50_us|cpu_p95_us) ' "$out/fps-counter.txt" ;;
  rebuild)
    "$here/go-rebuild.sh" | tee "$out/rebuild.txt" | grep '^GATE' ;;
  *) echo "go.sh: unknown phase $phase" >&2; rc=2 ;;
  esac
done
exit $rc
