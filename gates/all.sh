#!/bin/bash
# Master gate suite runner (`gates/all.sh [quick]`).
#
# What it gates:
#   Every verification gate for the Go bindings in one pass: builds, misuse checks, core pixel
#   parity, standalone app sessions (todo, reader), guide snippets and tutorial, conformance
#   generation, full gallery pixels, click latency, README example, multi-rate frame cost (FPS),
#   protocol and family tests, generated API smoke test, sample apps (dynamic and static), long
#   memory-growth checks plus leak self-test, and upstream coverage ratchet.
#
# How it measures and why the run is fair:
#   - Snapshots `lib/ship` and `lib/diag` (plus any external `GK_REF/ref-*` binaries) into
#     `gates/out/lib-snapshot` with `cp -a --reflink=auto` before step 1. Every gate therefore
#     judges the exact same Rust library build even if another build overwrites `lib/` mid-suite.
#   - Logs host memory, load average, library timestamp, library sha256 prefix, and repo commit
#     (noting uncommitted edits in `go/` or `examples/`) so every summary is traceable.
#   - Records wall-clock duration of every step in `gates/out/times.txt` and full combined stdout
#     and stderr in `gates/out/all.log`.
#   - Tier `full` (default): 10 FPS rounds at 60, 240, and 1000 Hz (override with `GK_RATES`),
#     plus per-frame counter clicks (20 rounds at 60 Hz), full memory runs, and leak self-test.
#   - Tier `quick`: 5 FPS rounds at 1000 Hz only, `--quick` memory runs, and skips `fps-click-go`
#     and `memory-selftest-go` for fast iteration.
#
# What PASS means (exit 0):
#   `gates/summary.py` compiles `gates/out/summary.md` and exits 0 only when at least one gate
#   ran, every step printed at least one `GATE` verdict, zero gates reported `FAIL`, `NONE`, or
#   `XPASS`, and every `PENDING` gate is listed in `gates/go-pending.txt`.

here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
tier=full; rounds=10; rates="60 240 1000"; mem=""
[ "${1:-}" = quick ] && { tier=quick; rounds=5; rates="1000"; mem=--quick; }
rates=${GK_RATES:-$rates}
export GK_LIB_ROOT=${GK_LIB_ROOT:-$repo/lib} GK_BIN=${GK_BIN:-$repo/bin}
log=$here/out/all.log; mkdir -p "$here/out"; : > "$log"; : > "$here/out/times.txt"
rm -rf "$here/out/fps"

# Snapshot ship and diag libraries (plus any external GK_REF binaries) so concurrent rebuilds
# cannot change the artifacts mid-suite. Removed automatically on exit.
snap=$here/out/lib-snapshot; rm -rf "$snap"; mkdir -p "$snap"
cp -a --reflink=auto "$GK_LIB_ROOT/ship" "$GK_LIB_ROOT/diag" "$snap/" || { echo "all.sh: $GK_LIB_ROOT has no ship and diag library (make lib)"; exit 1; }
if [ -n "${GK_REF:-}" ]; then cp -a "$GK_REF"/ref-* "$snap/diag/"; unset GK_REF; fi
trap 'rm -rf "$snap"' EXIT
from=$GK_LIB_ROOT
export GK_LIB_ROOT=$snap OUT=$GK_BIN
began=$(date +%s.%N)

# Execute one named gate step, teeing output into all.log and recording elapsed seconds.
step() {
  local name=$1 t=$(date +%s.%N); shift
  echo "== $name: $*" | tee -a "$log"
  "$@" 2>&1 | tee -a "$log"
  echo "$name $(echo "$(date +%s.%N) - $t" | bc)" >> "$here/out/times.txt"
}

echo "machine: $(free -g | awk 'NR==2{print "available " $7 " GB"}'), load$(uptime | sed 's/.*load average://'); tier $tier" | tee -a "$log"
echo "judged: library ${from#"$repo"/}/ship/libgouikit.so of $(date -r "$from/ship/libgouikit.so" '+%-d %b %H:%M') (sha256 $(sha256sum "$snap/ship/libgouikit.so" | cut -c1-12)), copied at $(date +%H:%M); Go package and examples at commit $(git -C "$repo" rev-parse --short HEAD 2>/dev/null || echo "${GK_COMMIT:-unknown} (a copy without git)")$(git -C "$repo" diff --quiet -- go examples 2>/dev/null || [ -n "${GK_COMMIT:-}" ] || echo ' with uncommitted changes')" | tee -a "$log"

# Functional, safety, pixel, session, docs, conformance, gallery, latency, and README gates.
step build-go "$here/go.sh" build
step misuse-go "$here/go-misuse.sh"
step pixels-go "$here/go.sh" pixels
step todo-go "$here/todo.sh"
step reader-go "$here/reader.sh"
step docs-go "$here/docs-go.sh"
step conformance python3 "$here/conformance.py" --check
step gallery-go "$here/gallery.sh" ${GK_GALLERY_RECORD:+--record}
step counter-go "$here/counter.sh"
step readme-go "$here/readme-go.sh"

# Frame-cost gates: static panels (enforcing zero FFI crossings per frame) and animated panels
# at every configured refresh rate, plus full-tier per-frame clicks and high-rate table scroll.
for hz in $rates; do
  step "fps-go-$hz" env BINS="rust go rust2" GK_JUDGED=go bash -c '
    GK_NAME=panels-60-64-0-go "$0" --hz "$1" --rounds "$2" --zero-crossings panels 60 64 0
    GK_NAME=panels-60-64-2-go "$0" --hz "$1" --rounds "$2" panels 60 64 2' "$here/fps.sh" "$hz" "$rounds"
done
[ $tier = full ] && step fps-click-go env GK_CLICK=40,32,61,1 GK_NAME=counter-click-every-frame-go \
  BINS="rust go rust2" GK_JUDGED=go "$here/fps.sh" --hz 60 --rounds "$((rounds * 2))" counter
step fps-table-go env GK_SCROLL=450,300,-24 GK_NAME=table-scroll-go BINS="rust go rust2" GK_JUDGED=go \
  "$here/fps.sh" --hz "${rates##* }" --rounds "$rounds" table

# Protocol, family tests, generated sample, full apps (dynamic + static), memory, and coverage.
step go "$here/go.sh" hello proto rebuild
step go-proto-families "$repo/core/tests/go/run.sh"
step go-gensample "$here/go-gensample.sh"
step go-apps "$here/go-apps.sh"
step go-apps-static env GK_STATIC=1 "$here/go-apps.sh" build notes todo reader expenses tutorial
step memory-go "$here/memory.sh" $mem
[ $tier = full ] && step memory-selftest-go "$here/memory.sh" --selftest
step coverage "$here/coverage.sh"
echo "all $(echo "$(date +%s.%N) - $began" | bc)" >> "$here/out/times.txt"
python3 "$here/summary.py" "$tier" "$log" "$here/expected.txt" "$here/out/times.txt" | tee "$here/out/summary.md"
# Return summary.py's exit status rather than tee's.
exit "${PIPESTATUS[0]}"
