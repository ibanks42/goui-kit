#!/bin/bash
# fps.sh [--hz N] [--rounds R] [--zero-crossings] SCENE [ARGS...]: the frame-cost gate.
# R interleaved rounds (default 10) of the Rust reference, the Odin app at -o:minimal, the
# Odin app at -o:speed and the Rust reference again (the Rust-against-Rust pair is the
# noise), one process at a time, in one private compositor session whose output refreshes
# at N Hz (default 60; see display.sh; 1000 is the most gamescope gives). Prints the table
# and one line starting with "GATE fps". Exit 0 = pass.
#   gates/fps.sh --hz 240 panels 60 64 2
# Pass (docs/DESIGN.md section 18): the -o:speed Odin app's median and p95 main-thread CPU
# per frame are within 3 % of Rust's plus the measured Rust-against-Rust difference (both
# as the median over rounds of the ratio inside each round; a round whose two Rust runs
# differ by more than 5 % was disturbed by another job and is left out, and a run with
# fewer than half its rounds left is measured once more with as many rounds again, and
# fails as a measurement if that is not enough), it
# misses no more frames than Rust (an interval over 1.5 periods; 1 % of frames tolerated),
# and with --zero-crossings no callback into the app happens on any measured frame.
# A run that shows no frames is killed and repeated, up to three times, and counted in
# stalls.txt: under gamescope a run can fail to receive any frame callback (seen once in
# about 240 runs, on the Rust reference). More stalls in the Odin app than in Rust (plus
# one) fails the gate.
# VARIANT=ship|diag; BINS overrides the list (default "rust odin odin-speed rust2"); GK_NAME
# names the run when the scene alone does not (the counter with GK_CLICK=x,y,frame,1 is the
# click-on-every-frame scene: one EVENT turn and one re-described view per frame).
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
# A run is 360 frames: allow three times that plus start-up before calling it stalled.
export GK_TIMEOUT=${GK_TIMEOUT:-$(( 360 * 3 / hz + 15 ))}
measure() { # FIRST LAST: rounds FIRST..LAST, appended to rounds.txt
  "$here/display.sh" --hz "$hz" bash -c '
    here=$1; out=$2; first=$3; last=$4; shift 4
    for r in $(seq "$first" "$last"); do
      for b in ${BINS:-rust odin odin-speed rust2}; do
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
if [ $rc -eq 3 ]; then
  # Too many rounds were disturbed by another job: the same number of rounds once more, and
  # the verdict over all of them.
  echo "fps.sh: $name was disturbed ($(grep -o 'only [0-9]* of [0-9]* rounds' "$out/summary.md")); measuring $rounds more rounds"
  measure $((rounds + 1)) $((rounds * 2))
  python3 "$here/fps-summary.py" --hz "$hz" --name "$name" $zero "$out" > "$out/summary.md"; rc=$?
fi
cat "$out/summary.md"
exit $rc
