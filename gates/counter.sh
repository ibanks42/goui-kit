#!/bin/bash
# counter.sh [--hz N]: the counter-app gate (latency and handler cost). The Rust reference
# and the Odin app, diag library, real window in a private compositor; a click on "Add" is
# injected 5 ms after a frame request (between two frames, as real input arrives) every 30
# frames. Pass: the changed views are built in the same frame as in Rust (frame offset after
# the click), and a click costs the Odin app 1 EVENT turn and at most 1 VIEWS turn.
# GK_FRONT=go: the Go app (bin/counter-go) in place of the Odin app; the line is "GATE counter click-go".
#   gates/counter.sh
here=$(cd "$(dirname "$0")" && pwd)
hz=60; if [ "${1:-}" = --hz ]; then hz=$2; shift 2; fi
front=${GK_FRONT:-odin}
out=$here/out/counter${GK_FRONT:+-$front}; rm -rf "$out"; mkdir -p "$out"
export GK_CLICK=${GK_CLICK:-40,32,90,30} GK_FRAMES=${GK_FRAMES:-60,240}
"$here/display.sh" --hz "$hz" bash -c '
  here=$1; out=$2
  for b in rust $3; do
    GK_STATS=$out/$b.csv "$here/one.sh" $b diag counter | grep "^gk-meter" > "$out/$b.txt"
  done
' _ "$here" "$out" "$front"
exec python3 "$here/counter-summary.py" "$out" "$front"
