#!/usr/bin/env python3
"""coverage-ratchet.py [--accept]: the half of the coverage gate that refuses a regression.

Components are matched by upstream path and row name together.
Compares api/coverage.json (what the generator wrote; the source of the table in
docs/STATUS.md) with gates/coverage-baseline.json (the last accepted state, committed).
Fails when, for a component of the baseline, the component is gone, bound + bridged went
down, verified went down or failed went up, or when a group's bound + bridged total went
down. More components, more bound and fewer failed always pass. Prints one line
"GATE coverage ratchet PASS|FAIL ...".

    coverage-ratchet.py            check
    coverage-ratchet.py --accept   record the present state as the baseline (after a check
                                   of what changed; commit the file with the change that
                                   explains it, e.g. a component split in two)
Standard library only."""
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
        key = f"{r.get('path') or '-'} [{r['name']}]"     # two rows can share a path
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
