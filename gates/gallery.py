#!/usr/bin/env python3
"""gallery.py script|compare ...: the page list of the gallery gate (gates/gallery.sh).

    gallery.py script PAGES.txt [--cycles N]     print the gk-meter script that walks the pages
    gallery.py frames PAGES.txt [--cycles N]     how many frames that script draws
    gallery.py compare PAGES.txt OUT ODIN_IDS [--record]   compare OUT/rust, OUT/rust2, OUT/odin
    gallery.py report OUT                        OUT/result.json as two Markdown tables

The script shows each page (a click on Next at 40,32 between pages), takes a shot named
NNN-id-0.png after its first frame and one more (NNN-id-K.png) after each of its steps. With
--cycles N it walks the pages N times and takes no shots (the memory gate's workload).

compare prints one line per page and the verdict lines. A page is
    identical   every shot of the Odin app is byte-identical to the Rust reference's
    DIFFERS     the Odin app has the page and a shot differs or is missing (fails the gate)
    not bound   the Odin app does not have the page yet (ODIN_IDS: its --list output)
    unstable    two runs of the Rust reference differ: the page itself is not comparable
                (fails the gate: fix the page, do not excuse it)
It writes OUT/result.json: per page its state, shots and the upstream functions it shows;
`verified` lists the functions shown by at least one identical page.

The ratchet: gates/verified.json (committed) lists the pages that were byte-identical and
the functions they verify. A page listed there that is not identical in this run fails the
gate ("regressed"), whatever its new state. --record rewrites the file from a passing run;
it is the list the coverage table's "verified" column is counted from. Standard library only."""
import json
import os
import sys


def pages(path):
    """The hand-written pages of PAGES.txt, then the generated ones of conf.txt beside it."""
    out = []
    conf = os.path.join(os.path.dirname(path), "conf.txt")
    for line in list(open(path)) + (list(open(conf)) if os.path.exists(conf) else []):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        cols = [c.strip() for c in line.split("|")] + ["", ""]
        steps = [s.strip() for s in cols[1].split(";") if s.strip()]
        out.append({"id": cols[0], "steps": steps, "uses": cols[2].split()})
    return out


def shots(ps):
    for n, p in enumerate(ps, 1):
        yield p, [f"{n:03d}-{p['id']}-{k}.png" for k in range(len(p["steps"]) + 1)]


def script(path, cycles):
    ps = pages(path)
    print("size 800 600")
    print("frame")
    first = True
    for cycle in range(cycles or 1):
        for p, names in shots(ps):
            if not first:
                # Escape first: a page that leaves a modal open (an alert dialog) swallowed the
                # click on Next, and every later shot was of the page before.
                print("key escape")
                print("frame")
                print("click 40 32")
            first = False
            print("frame")
            if not cycles:
                print(f"shot {names[0]}")
            for step, name in zip(p["steps"], names[1:]):
                print(step)
                print("frame")
                if not cycles:
                    print(f"shot {name}")
    print("stat")


def frames(path, cycles):
    """Frames the script draws: one per frame line, one per click, one per typed character."""
    import contextlib, io
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        script(path, cycles)
    n = 0
    for line in buf.getvalue().splitlines():
        cmd, _, rest = line.partition(" ")
        n += {"frame": 1, "click": 1}.get(cmd, len(rest) if cmd == "type" else 0)
    return n


def same(a, b):
    try:
        return open(a, "rb").read() == open(b, "rb").read()
    except OSError:
        return False


RECORD = os.path.join(os.path.dirname(os.path.abspath(__file__)), "verified.json")


def compare(path, out, odin_ids, record):
    have = set(open(odin_ids).read().split())
    ps = pages(path)
    res, counts, verified = [], {}, set()
    for p, names in shots(ps):
        stable = all(same(f"{out}/rust/{n}", f"{out}/rust2/{n}") for n in names)
        bad = [n for n in names if not same(f"{out}/rust/{n}", f"{out}/odin/{n}")]
        if not stable:
            state = "unstable"
        elif p["id"] not in have:
            state = "not bound"
        elif bad:
            state = "DIFFERS"
        else:
            state = "identical"
            verified.update(p["uses"])
        counts[state] = counts.get(state, 0) + 1
        res.append({"id": p["id"], "state": state, "shots": len(names), "differing": bad if state == "DIFFERS" else [],
                    "uses": p["uses"]})
        print(f"{p['id']}: {state}, {len(names)} shot(s)" + (f"; differing: {' '.join(bad)}" if state == "DIFFERS" else ""))
    shown = set(u for p in ps for u in p["uses"])
    json.dump({"pages": res, "counts": counts, "verified": sorted(verified), "shown": sorted(shown)},
              open(f"{out}/result.json", "w"), indent=1)
    n = len(ps)
    conf = [r for r in res if "." in r["id"]]
    cident = sum(1 for r in conf if r["state"] == "identical")
    ident, diff = counts.get("identical", 0), counts.get("DIFFERS", 0)
    unbound, unstable = counts.get("not bound", 0), counts.get("unstable", 0)
    nshots = sum(r["shots"] for r in res if r["state"] == "identical")
    was = json.load(open(RECORD))["pages"] if os.path.exists(RECORD) else []
    now = [r["id"] for r in res if r["state"] == "identical"]
    # A page renamed by the generator (Select.new became base::Select.new when a second Select
    # type entered the description) is the same page under its module-qualified id: it
    # counts as kept when that id is identical, and --record writes the new id.
    ids = {r["id"] for r in res}
    renamed = {p: q for p in was if p not in ids for q in now if q.endswith("::" + p)}
    regressed = [p for p in was if p not in now and p not in renamed]
    ok = diff == 0 and unstable == 0 and ident > 0 and not regressed
    if record and ok:
        json.dump({"note": "written by gates/gallery.sh --record from a passing run; do not edit by hand",
                   "pages": now, "functions": sorted(verified)}, open(RECORD, "w"), indent=1)
    print(f"GATE gallery pixels {'PASS' if ok else 'FAIL'} {ident} of {n} pages byte-identical to Rust "
          f"({nshots} images), {diff} differ, {unbound} not bound in Odin yet, {unstable} unstable in Rust; "
          f"of the {len(conf)} generated one-function pages {cident} identical; "
          f"{len(verified)} of {len(shown)} upstream functions shown are verified"
          + (f"; REGRESSED (identical in gates/verified.json, not now): {' '.join(regressed)}" if regressed else ""))
    return 0 if ok else 1


def report(out):
    """The hand-written pages one per row; the generated one-function pages counted per type."""
    res = json.load(open(f"{out}/result.json"))["pages"]
    hand = [r for r in res if "." not in r["id"]]
    conf = [r for r in res if "." in r["id"]]
    print("| page | Odin against Rust | images | functions shown |")
    print("|---|---|---|---|")
    for r in hand:
        print(f"| {r['id']} | {r['state']} | {r['shots']} | {len(r['uses'])} |")
    n = lambda rows, s: sum(1 for r in rows if r["state"] == s)
    print(f"| **{len(hand)} pages** | **{n(hand, 'identical')} identical, {n(hand, 'DIFFERS')} differ, "
          f"{n(hand, 'not bound')} not bound** | **{sum(r['shots'] for r in hand)}** | "
          f"**{len(set(u for r in hand for u in r['uses']))}** |")
    print()
    print("| type | generated one-function pages | identical | differ | not bound in Odin yet |")
    print("|---|---|---|---|---|")
    types = {}
    for r in conf:
        types.setdefault(r["id"].split(".")[0], []).append(r)
    quiet = 0
    for t, rows in sorted(types.items()):
        if n(rows, "identical") or n(rows, "DIFFERS"):
            print(f"| {t} | {len(rows)} | {n(rows, 'identical')} | {n(rows, 'DIFFERS')} | {n(rows, 'not bound')} |")
        else:
            quiet += 1
    rest = [r for t, rows in types.items() if not (n(rows, "identical") or n(rows, "DIFFERS")) for r in rows]
    print(f"| {quiet} other types and trait tables | {len(rest)} | 0 | 0 | {len(rest)} |")
    print(f"| **all** | **{len(conf)}** | **{n(conf, 'identical')}** | **{n(conf, 'DIFFERS')}** | **{n(conf, 'not bound')}** |")


if __name__ == "__main__":
    a = sys.argv[1:]
    if a and a[0] == "script":
        script(a[1], int(a[a.index("--cycles") + 1]) if "--cycles" in a else 0)
    elif a and a[0] == "report":
        report(a[1])
    elif a and a[0] == "frames":
        print(frames(a[1], int(a[a.index("--cycles") + 1]) if "--cycles" in a else 0))
    elif a and a[0] == "compare":
        sys.exit(compare(a[1], a[2], a[3], "--record" in a))
    else:
        sys.exit(__doc__)
