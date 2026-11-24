#!/usr/bin/env python3
"""Render and check the API coverage summary tables in docs/STATUS.md.

Usage:
    python3 tools/status.py           rewrite the marked coverage block in docs/STATUS.md
    python3 tools/status.py --check   verify consistency and freshness, printing a GATE line

Input schema (`api/coverage.json`, produced by `gen/gkgen.py` and updated with gallery
verification counts by `gates/gallery.sh`):
- `pin`: upstream crate versions (`gpui-kit`, `gpui-fast`, enabled `features`).
- `universe`: total catalogued types and public functions in the pinned upstream API.
- `components`: list of owner rows (`name`, `group`, `family`, `total`, `bound`,
  `bridged`, `skipped`, `failed`, `todo`, `verified`, `todo_reasons`, `skip_reasons`).
- `partial`: list of bound/bridged items whose enums omit one or more upstream variants
  (from `gen/gk_values.py:partials()`).

Invariants enforced by `table()` and `--check`:
1. For every component row, `total == bound + bridged + skipped + failed + todo`.
2. For every component row, `verified <= bound + bridged`.
3. Under `--check`, the rendered Markdown between `BEGIN` and `END` markers must match
   `docs/STATUS.md` byte-for-byte (otherwise fails as stale).

Style-token rows (`group == "style"`) are excluded from the headline "Real functions"
and per-family remaining tables so thousands of generated color/size tokens do not skew
component coverage percentages.
"""
import json
import os
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SRC = os.path.join(REPO, "api", "coverage.json")
DOC = os.path.join(REPO, "docs", "STATUS.md")
BEGIN, END = "<!-- coverage:begin (written by tools/status.py; do not edit) -->", "<!-- coverage:end -->"
COLS = ("total", "bound", "bridged", "skipped", "failed", "todo", "verified")


def pct(a, b):
    """Format a/b as a one-decimal percentage string, or '-' when b is zero."""
    return f"{100 * a / b:.1f} %" if b else "-"


def table(cov):
    """Validate `api/coverage.json` row invariants and render the Markdown coverage block.

    Returns `(markdown_text, column_totals, validation_errors)` covering:
    1. Pin header, non-style headline totals, per-group breakdown, and universe counts.
    2. Remaining functions and deliberate skip reasons aggregated by component family.
    3. Partial-enum reachability table distinguishing direct gaps from `Into`-set alternatives.
    4. Full per-component table plus aggregated missing-rule totals.
    """
    rows = cov["components"]
    bad = []
    for r in rows:
        if r["total"] != r["bound"] + r["bridged"] + r["skipped"] + r["failed"] + r["todo"]:
            bad.append(f"{r['name']}: total {r['total']} is not the sum of its statuses")
        if r["verified"] > r["bound"] + r["bridged"]:
            bad.append(f"{r['name']}: verified exceeds bound + bridged")
    tot = {c: sum(r[c] for r in rows) for c in COLS}
    pin, uni = cov.get("pin", {}), cov.get("universe", {})
    o = []
    o.append(f"Pin: gpui-kit {pin.get('gpui-kit', '?')}, gpui-fast {pin.get('gpui-fast', '?')}, "
             f"features {', '.join(pin.get('features', [])) or '?'}.")
    o.append("")
    real = {c: sum(r[c] for r in rows if r.get("group") != "style") for c in COLS}
    use = real["bound"] + real["bridged"]
    o.append(f"**Real functions (style tokens left out): {real['total']}. Usable (bound or bridged): {use} "
             f"({pct(use, real['total'])}). Not attempted yet: {real['todo']}. Skipped on purpose: "
             f"{real['skipped']}. Failed: {real['failed']}. Verified by a byte-identical gallery page: "
             f"{real['verified']}.**")
    o.append("")
    groups = {}
    for r in rows:
        g = groups.setdefault(r.get("group", ""), {c: 0 for c in COLS})
        for c in COLS:
            g[c] += r[c]
    for name, g in sorted(groups.items()):
        o.append(f"- **{name}: {g['bound'] + g['bridged']} of {g['total']} functions bound or bridged "
                 f"({pct(g['bound'] + g['bridged'], g['total'])})**: {g['bound']} bound, "
                 f"{g['bridged']} bridged, {g['skipped']} skipped, {g['failed']} failed, "
                 f"{g['todo']} not attempted yet; {g['verified']} verified by a run.")
    o.append("")
    if uni:
        o.append(f"The table covers {len(rows)} of {uni.get('types', '?')} catalogued types and "
                 f"{tot['total']} of {uni.get('functions', '?')} public functions in the pin"
                 + (f" ({uni['note']})" if uni.get("note") else "") + ".")
    o.append("")
    fams = {}
    for r in rows:
        if r.get("group") in ("style",):
            continue
        f = fams.setdefault(r.get("family", "other"), {"rows": 0, "total": 0, "done": 0, "skipped": 0, "left": 0, "why": {}, "skip": {}, "verified": 0})
        f["rows"] += 1
        f["total"] += r["total"]
        f["done"] += r["bound"] + r["bridged"]
        f["skipped"] += r["skipped"]
        f["left"] += r["todo"] + r["failed"]
        f["verified"] += r["verified"]
        for k, v in r.get("todo_reasons", {}).items():
            f["why"][k] = f["why"].get(k, 0) + v
        for k, v in r.get("skip_reasons", {}).items():
            f["skip"][k] = f["skip"].get(k, 0) + v
    if fams:
        o.append("What remains, by family (style tokens left out; a row is an upstream type, trait or module):")
        o.append("")
        o.append("| family | rows | functions | bound or bridged | verified | skipped | remaining | what the remaining ones need (functions) |")
        o.append("|---|---|---|---|---|---|---|---|")
        for name, f in sorted(fams.items(), key=lambda kv: -kv[1]["left"]):
            why = "; ".join(f"{k.replace('protocol: ', '')} {v}" for k, v in sorted(f["why"].items(), key=lambda kv: -kv[1]))
            o.append(f"| {name} | {f['rows']} | {f['total']} | {f['done']} | {f['verified']} | {f['skipped']} | {f['left']} | {why} |")
        o.append("")
        o.append("Skipped on purpose, by family and reason (the reasons are DESIGN.md section 13's fixed list):")
        o.append("")
        o.append("| family | skipped | reasons (functions) |")
        o.append("|---|---|---|")
        for name, f in sorted(fams.items(), key=lambda kv: -kv[1]["skipped"]):
            why = "; ".join(f"{k} {v}" for k, v in sorted(f["skip"].items(), key=lambda kv: -kv[1]))
            o.append(f"| {name} | {f['skipped']} | {why} |")
        o.append("")
    part = cov.get("partial", [])
    if part:
        fns = {}
        for p in part:
            fns.setdefault(p["id"], []).append(p)
        direct = sorted(k for k, v in fns.items() if not all(p["note"] for p in v))
        o.append(f"Options only partly reachable: {len(fns)} usable functions take, give or read an enum of which "
                 f"the binding builds only some variants; {len(direct)} have no other way to the missing variants, "
                 f"{len(fns) - len(direct)} take an Into set whose other members may reach them (by family; "
                 "written by gen/gk_values.py `partials`):")
        o.append("")
        o.append("| family | function | enum (direction) | variants missing | other members of the Into set |")
        o.append("|---|---|---|---|---|")
        for k, v in sorted(fns.items(), key=lambda kv: (kv[1][0]["family"], kv[0])):
            o.append(f"| {v[0]['family']} | `{k.replace('gpui_kit::', '')}` | "
                     + "; ".join(f"`{p['enum'].rsplit('::', 1)[-1]}` ({p['direction']})" for p in v) + " | "
                     + "; ".join(", ".join(p["missing"]) for p in v) + " | "
                     + "; ".join(p["note"].replace("Into set, also: ", "").replace("gpui_kit::", "") or "-" for p in v) + " |")
        o.append("")
    o.append("| component | group | total | bound | bridged | skipped (reason) | failed | not yet | bound + bridged | verified |")
    o.append("|---|---|---|---|---|---|---|---|---|---|")
    for r in sorted(rows, key=lambda r: (r.get("group", ""), r["name"])):
        why = ", ".join(f"{k} {v}" for k, v in sorted(r.get("skip_reasons", {}).items(), key=lambda kv: -kv[1]))
        o.append(f"| {r['name']} | {r.get('group', '')} | {r['total']} | {r['bound']} | {r['bridged']} | "
                 f"{r['skipped']}{' (' + why + ')' if why else ''} | {r['failed']} | {r['todo']} | "
                 f"{pct(r['bound'] + r['bridged'], r['total'])} | {r['verified']} |")
    o.append(f"| **all** | | **{tot['total']}** | **{tot['bound']}** | **{tot['bridged']}** | "
             f"**{tot['skipped']}** | **{tot['failed']}** | **{tot['todo']}** | "
             f"**{pct(tot['bound'] + tot['bridged'], tot['total'])}** | **{tot['verified']}** |")
    reasons = {}
    for r in rows:
        for k, v in r.get("todo_reasons", {}).items():
            reasons[k] = reasons.get(k, 0) + v
    if reasons:
        o.append("")
        o.append("Not attempted yet, by what is missing:")
        o.append("")
        o.append("| missing rule | functions |")
        o.append("|---|---|")
        for k, v in sorted(reasons.items(), key=lambda kv: -kv[1]):
            o.append(f"| {k} | {v} |")
    return "\n".join(o), tot, bad


def main():
    """Update docs/STATUS.md in place, or verify freshness and print a GATE coverage line under --check."""
    check = "--check" in sys.argv
    if not os.path.exists(SRC):
        print("GATE coverage all NONE api/coverage.json does not exist yet")
        return 1 if check else 0
    text, tot, bad = table(json.load(open(SRC)))
    doc = open(DOC).read()
    a, b = doc.index(BEGIN) + len(BEGIN), doc.index(END)
    new = doc[:a] + "\n" + text + "\n" + doc[b:]
    stale = new != doc
    if check:
        if stale:
            bad.append("docs/STATUS.md is stale (run make status)")
        for line in bad:
            print("coverage:", line)
        print(f"GATE coverage all {'FAIL' if bad else 'PASS'} {tot['bound']} bound, {tot['bridged']} bridged, "
              f"{tot['skipped']} skipped, {tot['failed']} failed, {tot['todo']} not yet of {tot['total']}; "
              f"{tot['verified']} verified")
        return 1 if bad else 0
    for line in bad:
        print("coverage:", line)
    if stale:
        open(DOC, "w").write(new)
    print(f"status: docs/STATUS.md {'rewritten' if stale else 'unchanged'}")
    return 1 if bad else 0


sys.exit(main())
