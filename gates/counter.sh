#!/bin/bash
# Click-latency and protocol-turn gate (`gates/counter.sh [--hz N]`).
# Gates: frame latency from click injection to view rebuild and protocol turn count on `counter`.
# Method: runs Rust (`ref-counter`) and Go (`counter-go`) on lib/diag inside one headless display.sh
# session (default 60 Hz, 60 warmup + 240 measured frames, synthetic clicks via GK_CLICK=40,32,90,30),
# recording per-frame CSVs for gates/counter-summary.py.
# PASS: gates/counter-summary.py exits 0 (Go rebuilds on the same frame offset as Rust with 1 EVENT
# turn and at most 1 VIEWS turn per click).

here=$(cd "$(dirname "$0")" && pwd)
hz=60; if [ "${1:-}" = --hz ]; then hz=$2; shift 2; fi
front=go
out=$here/out/counter-$front; rm -rf "$out"; mkdir -p "$out"
export GK_CLICK=${GK_CLICK:-40,32,90,30} GK_FRAMES=${GK_FRAMES:-60,240}
"$here/display.sh" --hz "$hz" bash -c '
  here=$1; out=$2
  for b in rust $3; do
    GK_STATS=$out/$b.csv "$here/one.sh" $b diag counter | grep "^gk-meter" > "$out/$b.txt"
  done
' _ "$here" "$out" "$front"
exec python3 "$here/counter-summary.py" "$out" "$front"
