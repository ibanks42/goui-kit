#!/bin/bash
# profile.sh [--rounds R] [SCENE ARGS...]: where the main thread's time goes in one scene, the
# Rust reference against the Odin app (-o:speed), on the same library build. Default scene:
# the counter with a click on every frame (GK_CLICK=40,32,61,1), the one frame-cost scene that
# sat at its limit.
#
# perf needs kernel.perf_event_paranoid 2 or lower (or root), which many machines lack, so the
# profiler is tools/profile/sampler.c, loaded with LD_PRELOAD into the measured binary only: a
# timer on the main thread's own CPU clock samples the interrupted address every 50 us of
# that thread's CPU time. R rounds (default 6) alternate the two binaries in one private
# display (gates/display.sh); tools/profile/symbolize.py adds them up per function and per
# binary, in microseconds a frame. Then one run of each under strace (main thread only) counts
# its system calls.
#
# Needs unstripped copies of the ship library and of the reference in $GK_SYMBOLS_DIR (default
# dist/symbols: tools/build-lib.sh symbols ref-counter) and a C compiler for the sampler
# ($GK_CC, default cc). Output: $GK_SCRATCH/profile/report.txt.
#   gates/profile.sh
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); repo=$(dirname "$here")
rounds=6
[ "${1:-}" = --rounds ] && { rounds=$2; shift 2; }
[ $# -gt 0 ] || set -- counter
scene=$1; shift
export GK_CLICK=${GK_CLICK-40,32,61,1}
sym=${GK_SYMBOLS_DIR:-$repo/dist/symbols}
cc=${GK_CC:-cc}
odin=${ODIN:-odin}
base=${GK_SCRATCH:-${TMPDIR:-/tmp}/gk-gates}
out=$base/profile; rm -rf "$out"; mkdir -p "$out/lib/ship" "$out/bin" "$out/runs"
[ -f "$sym/libgpuikit.so" ] && [ -f "$sym/ref-$scene" ] || { echo "profile.sh: no unstripped library and ref-$scene in $sym"; exit 2; }
cp "$sym/libgpuikit.so" "$sym/ref-$scene" "$out/lib/ship/"
"$cc" -shared -fPIC -O2 -o "$out/sampler.so" "$repo/tools/profile/sampler.c"
"$odin" build "$repo/examples/odin/$scene" -o:speed -define:GK_SHARED=true -collection:gpuikit="$repo/odin" \
  "-extra-linker-flags:-L$out/lib/ship -Wl,-rpath,\$ORIGIN/../lib/ship" -out:"$out/bin/$scene-odin-speed"
export GK_FRAMES=${GK_FRAMES:-60,600} LD_LIBRARY_PATH=$out/lib/ship
"$here/display.sh" --hz 60 bash -c '
  out=$1; scene=$2; rounds=$3; shift 3
  for r in $(seq "$rounds"); do
    for b in rust odin; do
      exe=$out/lib/ship/ref-$scene; [ $b = odin ] && exe=$out/bin/$scene-odin-speed
      GK_LABEL=$b GK_STATS=$out/runs/$b-$r.csv LD_PRELOAD=$out/sampler.so GK_PROF_OUT=$out/runs/$b-$r \
        "$exe" "$@" | grep "^gk-meter" | sed "s/^/round=$r /" >> "$out/meter.txt"
    done
  done' _ "$out" "$scene" "$rounds" "$@"
# The main thread's system calls, counted (strace without -f follows the main thread only):
# one run of each binary.
"$here/display.sh" --hz 60 bash -c '
  out=$1; scene=$2; shift 2
  for b in rust odin; do
    exe=$out/lib/ship/ref-$scene; [ $b = odin ] && exe=$out/bin/$scene-odin-speed
    GK_LABEL=$b GK_STATS=$out/runs/$b-strace.csv strace -c -o "$out/runs/$b-strace.txt" "$exe" "$@" > /dev/null
  done' _ "$out" "$scene" "$@"
pre() { local s=""; for r in $(seq "$rounds"); do s="$s${s:+,}$out/runs/$1-$r"; done; echo "$s"; }
{
  echo "profile of $scene $* (GK_CLICK=$GK_CLICK), $rounds rounds of $GK_FRAMES frames each, Rust reference and Odin -o:speed on the same library"
  cat "$out/meter.txt"
  echo
  python3 "$repo/tools/profile/symbolize.py" --skip 2.5 --top 45 rust="$(pre rust)" odin="$(pre odin)"
  for b in rust odin; do echo; echo "system calls of the main thread, $b, one run of $GK_FRAMES frames:"; head -25 "$out/runs/$b-strace.txt"; done
} > "$out/report.txt"
cat "$out/report.txt"
