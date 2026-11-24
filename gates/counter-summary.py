#!/usr/bin/env python3
"""Judge click-to-rebuild frame latency and protocol turns from gates/counter.sh CSVs.

Gates: Go (or another candidate passed as argv[2]) against the Rust reference on `counter`.
Method: computes each build's usual idle views_built count (mode over click==0 frames), then for
every click==1 frame locates the first frame within +0..+5 frames where views_built rises above
baseline and sums protocol counters across the 2-frame window [i, i+1].
PASS: both runs record clicks, every click rebuilds a view, Go's rebuild frame offsets match Rust
exactly, and every Go click uses 1 EVENT turn and at most 1 VIEWS turn (exit 0).
"""
import csv
import statistics
import sys

out = sys.argv[1]
front = sys.argv[2] if len(sys.argv) > 2 else "go"
res = {}
for b in ("rust", front):
    try:
        rows = list(csv.DictReader(open(f"{out}/{b}.csv")))
    except OSError:
        rows = []
    # Baseline views_built on frames with no click injected.
    quiet = [int(r["views_built"]) for r in rows if r["click"] == "0"]
    base = statistics.mode(quiet) if quiet else 0
    offs, costs = [], []
    for i, r in enumerate(rows):
        if r["click"] != "1" or i + 2 > len(rows):
            continue
        # Find the first frame within 0..5 frames after the click where view rebuilds rise above idle.
        offs.append(next((j - i for j in range(i, min(i + 6, len(rows)))
                          if int(rows[j]["views_built"]) > base), None))
        pair = rows[i:i + 2]
        costs.append(tuple(sum(int(x[k]) for x in pair)
                           for k in ("event_turns", "views_turns", "tapes", "deferred_ops", "calls")))
    res[b] = (offs, costs)
    print(f"{b}: clicks={len(offs)} usual views_built={base} "
          f"frame offset of the rebuilt views={sorted(set(offs), key=str)} "
          f"per click (event_turns, views_turns, tapes, deferred_ops, calls)={sorted(set(costs))}")

ro, oo = res["rust"][0], res[front][0]
why = []
if not ro or not oo:
    why.append("no clicks recorded")
if None in ro or None in oo:
    why.append("a click rebuilt no view")
if sorted(set(ro), key=str) != sorted(set(oo), key=str):
    why.append("the change is not in the same frame as in Rust")
if any(c[0] != 1 or c[1] > 1 for c in res[front][1]):
    why.append("a click cost more than 1 EVENT and 1 VIEWS turn")
print(f"GATE counter click-{front} {'FAIL' if why else 'PASS'} {len(oo)} clicks, shown "
      f"{sorted(set(oo), key=str)} frame(s) after the click, Rust {sorted(set(ro), key=str)}"
      + ("; " + "; ".join(why) if why else ""))
sys.exit(1 if why else 0)
