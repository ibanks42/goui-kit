#!/usr/bin/env python3
"""counter-summary.py DIR: the verdict of gates/counter.sh.

DIR holds rust.csv and odin.csv (one row per measured frame, diag library). For every
injected click (a row with click=1: the click arrived during that frame's interval), finds
the later frame in which gpui-fast built more views than usual, and what the click cost in
turns. Prints one line "GATE counter click PASS|FAIL ...". Exit 0 on pass."""
import csv
import statistics
import sys

out = sys.argv[1]
res = {}
for b in ("rust", "odin"):
    try:
        rows = list(csv.DictReader(open(f"{out}/{b}.csv")))
    except OSError:
        rows = []
    quiet = [int(r["views_built"]) for r in rows if r["click"] == "0"]
    base = statistics.mode(quiet) if quiet else 0
    offs, costs = [], []
    for i, r in enumerate(rows):
        if r["click"] != "1" or i + 2 > len(rows):
            continue
        offs.append(next((j - i for j in range(i, min(i + 6, len(rows)))
                          if int(rows[j]["views_built"]) > base), None))
        pair = rows[i:i + 2]
        costs.append(tuple(sum(int(x[k]) for x in pair)
                           for k in ("event_turns", "views_turns", "tapes", "deferred_ops", "calls")))
    res[b] = (offs, costs)
    print(f"{b}: clicks={len(offs)} usual views_built={base} "
          f"frame offset of the rebuilt views={sorted(set(offs), key=str)} "
          f"per click (event_turns, views_turns, tapes, deferred_ops, calls)={sorted(set(costs))}")

ro, oo = res["rust"][0], res["odin"][0]
why = []
if not ro or not oo:
    why.append("no clicks recorded")
if None in ro or None in oo:
    why.append("a click rebuilt no view")
if sorted(set(ro), key=str) != sorted(set(oo), key=str):
    why.append("the change is not in the same frame as in Rust")
if any(c[0] != 1 or c[1] > 1 for c in res["odin"][1]):
    why.append("a click cost more than 1 EVENT and 1 VIEWS turn")
print(f"GATE counter click {'FAIL' if why else 'PASS'} {len(oo)} clicks, shown "
      f"{sorted(set(oo), key=str)} frame(s) after the click, Rust {sorted(set(ro), key=str)}"
      + ("; " + "; ".join(why) if why else ""))
sys.exit(1 if why else 0)
