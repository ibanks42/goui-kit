#!/bin/bash
# Diagnostic CPU and syscall profiler (`gates/profile.sh [--rounds N] [scene] [args...]`).
#
# What it measures (diagnostic tool, not a PASS/FAIL gate):
#   Compares Rust reference and Go main-thread hot stacks and system calls on the exact same
#   unstripped `libgouikit.so` from `GK_SYMBOLS_DIR` (default `dist/symbols`). Defaults to 6
#   rounds of `counter` with per-frame synthetic clicks (`GK_CLICK=40,32,61,1`).
#
# How it keeps the comparison fair:
#   - Copies the unstripped `libgouikit.so` and `ref-<scene>` into scratch and builds `<scene>-go`
#     against that exact same shared library so symbol resolution and optimization flags match.
#   - Compiles `tools/profile/sampler.c` into `sampler.so` and runs `rust` and `go` interleaved
#     inside one 60 Hz `display.sh` session (default `GK_FRAMES=60,600`) under `LD_PRELOAD`.
#   - Runs a separate `strace -c` pass without the sampler preloaded so profiling signals do not
#     pollute the syscall counts.
#   - Symbolizes the top 45 frames via `tools/profile/symbolize.py --skip 2.5` (skipping the first
#     2.5 s of startup/warmup) and writes the full report to `<scratch>/profile/report.txt`.

set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
rounds=6
[ "${1:-}" = --rounds ] && { rounds=$2; shift 2; }
[ $# -gt 0 ] || set -- counter
scene=$1; shift
export GK_CLICK=${GK_CLICK-40,32,61,1}
sym=${GK_SYMBOLS_DIR:-$repo/dist/symbols}
cc=${GK_CC:-cc}
base=${GK_SCRATCH:-${TMPDIR:-/tmp}/gk-gates}
out=$base/profile; rm -rf "$out"; mkdir -p "$out/lib/ship" "$out/bin" "$out/runs"
[ -f "$sym/libgouikit.so" ] && [ -f "$sym/ref-$scene" ] || { echo "profile.sh: no unstripped library and ref-$scene in $sym"; exit 2; }
cp "$sym/libgouikit.so" "$sym/ref-$scene" "$out/lib/ship/"
"$cc" -shared -fPIC -O2 -o "$out/sampler.so" "$repo/tools/profile/sampler.c"
GK_LIB=$out/lib/ship OUT=$out/bin "$repo/examples/go/build.sh" "$scene"
export GK_FRAMES=${GK_FRAMES:-60,600} LD_LIBRARY_PATH=$out/lib/ship

# Interleaved sampling runs under sampler.so.
"$here/display.sh" --hz 60 bash -c '
  out=$1; scene=$2; rounds=$3; shift 3
  for r in $(seq "$rounds"); do
    for b in rust go; do
      exe=$out/lib/ship/ref-$scene; [ $b = go ] && exe=$out/bin/$scene-go
      GK_LABEL=$b GK_STATS=$out/runs/$b-$r.csv LD_PRELOAD=$out/sampler.so GK_PROF_OUT=$out/runs/$b-$r \
        "$exe" "$@" | grep "^gk-meter" | sed "s/^/round=$r /" >> "$out/meter.txt"
    done
  done' _ "$out" "$scene" "$rounds" "$@"

# Separate single-run syscall summary without the sampler preloaded.
"$here/display.sh" --hz 60 bash -c '
  out=$1; scene=$2; shift 2
  for b in rust go; do
    exe=$out/lib/ship/ref-$scene; [ $b = go ] && exe=$out/bin/$scene-go
    GK_LABEL=$b GK_STATS=$out/runs/$b-strace.csv strace -c -o "$out/runs/$b-strace.txt" "$exe" "$@" > /dev/null
  done' _ "$out" "$scene" "$@"

pre() { local s=""; for r in $(seq "$rounds"); do s="$s${s:+,}$out/runs/$1-$r"; done; echo "$s"; }
{
  echo "profile of $scene $* (GK_CLICK=$GK_CLICK), $rounds rounds of $GK_FRAMES frames each, Rust reference and Go on the same library"
  cat "$out/meter.txt"
  echo
  python3 "$repo/tools/profile/symbolize.py" --skip 2.5 --top 45 rust="$(pre rust)" go="$(pre go)"
  for b in rust go; do echo; echo "system calls of the main thread, $b, one run of $GK_FRAMES frames:"; head -25 "$out/runs/$b-strace.txt"; done
} > "$out/report.txt"
cat "$out/report.txt"
