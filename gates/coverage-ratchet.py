#!/usr/bin/env python3
"""Enforce non-regression of upstream API coverage against gates/coverage-baseline.json.

What it gates:
  Per-component and per-group upstream coverage recorded in `api/coverage.json`.

How it measures and why the check is fair:
  - Tracks each component by `path [name]` with `done = bound + bridged`, `verified`, and `failed`.
  - Checks both individual components and aggregate group totals so reorganizing or splitting
    components cannot silently drop bound surface area.
  - Run with `--accept` after intentional API coverage increases or upstream removals to update
    `gates/coverage-baseline.json` in place.

What PASS means (exit 0):
  Every component in `gates/coverage-baseline.json` still exists in `api/coverage.json`, no
  component or group lost `bound + bridged` count, no component lost `verified` count, and no
  component increased `failed` count.
"""
import json
import os
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SRC = os.path.join(REPO, "api", "coverage.json")
BASE = os.path.join(REPO, "gates", "coverage-baseline.json")

def state():
    cov = json.load(open(SRC))
    comps = {}
    for r in cov["components"]:
        key = f"{r.get('path') or '-'} [{r['name']}]"
        assert key not in comps, f"api/coverage.json has two rows {key}"
        comps[key] = {"group": r.get("group", ""), "done": r["bound"] + r["bridged"],
                            "verified": r["verified"], "failed": r["failed"], "total": r["total"]}
    return comps

def groups(comps):
    g = {}
    for c in comps.values():
        g[c["group"]] = g.get(c["group"], 0) + c["done"]
    return g

def main():
    if not os.path.exists(SRC):
        print("GATE coverage ratchet NONE api/coverage.json does not exist")
        return 1
    now = state()
    if "--accept" in sys.argv:
        json.dump({"note": "written by gates/coverage-ratchet.py --accept; do not edit by hand",
                   "components": now}, open(BASE, "w"), indent=1, sort_keys=True)
        print(f"coverage ratchet: baseline is now {len(now)} components, "
              f"{sum(c['done'] for c in now.values())} bound or bridged, {sum(c['verified'] for c in now.values())} verified")
        return 0
    if not os.path.exists(BASE):
        print("GATE coverage ratchet NONE no baseline (gates/coverage-ratchet.py --accept)")
        return 1
    was = json.load(open(BASE))["components"]
    bad = []
    for name, b in sorted(was.items()):
        n = now.get(name)
        if n is None:
            bad.append(f"{name} is gone")
            continue
        if n["done"] < b["done"]:
            bad.append(f"{name}: bound + bridged {b['done']} -> {n['done']}")
        if n["verified"] < b["verified"]:
            bad.append(f"{name}: verified {b['verified']} -> {n['verified']}")
        if n["failed"] > b["failed"]:
            bad.append(f"{name}: failed {b['failed']} -> {n['failed']}")
    gw, gn = groups(was), groups(now)
    for g, v in sorted(gw.items()):
        if gn.get(g, 0) < v:
            bad.append(f"group {g}: bound + bridged {v} -> {gn.get(g, 0)}")
    for line in bad:
        print("coverage ratchet:", line)
    done = lambda s: sum(c["done"] for c in s.values())
    ver = lambda s: sum(c["verified"] for c in s.values())
    print(f"GATE coverage ratchet {'FAIL' if bad else 'PASS'} {len(now)} components against {len(was)} recorded: "
          f"bound + bridged {done(was)} -> {done(now)}, verified {ver(was)} -> {ver(now)}, "
          f"failed {sum(c['failed'] for c in was.values())} -> {sum(c['failed'] for c in now.values())}"
          + (f"; {len(bad)} regression(s): " + "; ".join(bad[:4]) if bad else ""))
    return 1 if bad else 0

sys.exit(main())
