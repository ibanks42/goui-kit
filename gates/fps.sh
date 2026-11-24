#!/bin/bash
# Frame-cost gate (`gates/fps.sh [--hz N] [--rounds R] [--zero-crossings] SCENE [ARGS...]`).
#
# What it gates:
#   Main-thread frame cost (p50 and p95 CPU microseconds), missed frames, compositor stalls, round
#   completion, and (with `--zero-crossings`) zero per-frame FFI crossings on static scenes for
#   `GK_JUDGED` (default `go`) compared against the pure-Rust reference.
#
# How it measures and why the comparison is fair:
#   - Runs `R` interleaved rounds (default 10) inside a single headless `display.sh --hz N` session
#     on `VARIANT` (default `ship`). Every round executes `BINS` (default `rust go rust2`) back to
#     back on the same compositor and library so both builds experience the same machine state.
#   - Running Rust twice per round (`rust` and `rust2`) gives `fps-summary.py` a same-round control
#     to detect background CPU load and measure Rust's own variance.
#   - Each run gets a rate-scaled timeout (`360 * 3 / hz + 15` s for 60 warmup + 300 measured
#     frames). If a headless compositor stalls and no `gk-meter` line is printed, the run is retried
#     up to 3 times and logged in `stalls.txt`.
#   - If `fps-summary.py` exits 3 (too many rounds had `rust` and `rust2` more than 5% apart),
#     `fps.sh` automatically measures `R` additional rounds and re-runs the summary over all rounds.
#
# What PASS means (exit 0):
#   `gates/fps-summary.py` prints `GATE fps <name> PASS` and exits 0 (not disturbed, paired CPU
#   p50 and p95 within `GK_BAR + noise`, missed frames within 1% of Rust, bounded stalls, and zero
#   crossings when `--zero-crossings` is set).

here=$(cd "$(dirname "$0")" && pwd)
hz=60; rounds=10; zero=""
while :; do case "${1:-}" in
  --hz) hz=$2; shift 2 ;; --rounds) rounds=$2; shift 2 ;; --zero-crossings) zero=--zero-crossings; shift ;;
  *) break ;; esac; done
[ $# -gt 0 ] || { echo "usage: fps.sh [--hz N] [--rounds R] [--zero-crossings] SCENE [ARGS...]" >&2; exit 2; }
export VARIANT=${VARIANT:-ship}
name=${GK_NAME:-$(IFS=-; echo "$*")}-$VARIANT-${hz}hz
out=$here/out/fps/$name
rm -rf "$out"; mkdir -p "$out"; : > "$out/stalls.txt"

# Allow 3x the nominal duration of 360 frames (60 warmup + 300 measured) plus 15 s startup margin.
export GK_TIMEOUT=${GK_TIMEOUT:-$(( 360 * 3 / hz + 15 ))}

# Run rounds [first..last] inside a single display session; retry up to 3 times if a run produces
# no gk-meter line (for example, a headless compositor frame-callback stall).
measure() {
  "$here/display.sh" --hz "$hz" bash -c '
    here=$1; out=$2; first=$3; last=$4; shift 4
    for r in $(seq "$first" "$last"); do
      for b in ${BINS:-rust go rust2}; do
        for try in 1 2 3; do
          line=$(GK_LABEL=$b GK_STATS=$out/$b-$r.csv "$here/one.sh" "${b%2}" "$VARIANT" "$@" | grep "^gk-meter")
          [ -n "$line" ] && { echo "round=$r $line" >> "$out/rounds.txt"; break; }
          echo "$b round=$r try=$try" >> "$out/stalls.txt"
        done
      done
    done
  ' _ "$here" "$out" "$1" "$2" "${scene[@]}"
}

scene=("$@")
measure 1 "$rounds"
python3 "$here/fps-summary.py" --hz "$hz" --name "$name" $zero "$out" > "$out/summary.md"; rc=$?
# Exit code 3 means rust and rust2 diverged in too many rounds; gather one more batch of rounds.
if [ $rc -eq 3 ]; then
  echo "fps.sh: $name was disturbed ($(grep -o 'only [0-9]* of [0-9]* rounds' "$out/summary.md")); measuring $rounds more rounds"
  measure $((rounds + 1)) $((rounds * 2))
  python3 "$here/fps-summary.py" --hz "$hz" --name "$name" $zero "$out" > "$out/summary.md"; rc=$?
fi
cat "$out/summary.md"
exit $rc
