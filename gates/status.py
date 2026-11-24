#!/usr/bin/env python3
"""Update the gates status section in docs/STATUS.md from recent gates/all.sh output.

Usage:
  status.py [--any-age]

What it does:
  Replaces the Markdown block between `<!-- gates:begin -->` and `<!-- gates:end -->` in
  `docs/STATUS.md` with:
  1. The verdict summary table from `gates/out/summary.md` (with run timestamp and Go version).
  2. A full frame-cost table aggregated from `gates/out/fps/*/rounds.txt` and `summary.md`.
     (The table displays raw medians over all rounds for transparency, while the verdict column
     comes from `fps-summary.py`'s paired quiet-round comparison.)
  3. The page-by-page gallery breakdown rendered by `gates/gallery.py report`.

Safety guard:
  Refuses `gates/out/summary.md` older than 3600 seconds unless `--any-age` is passed, preventing
  stale test results from being published into docs/STATUS.md by mistake.
"""
import datetime
import glob
import io
import json
import os
import contextlib
import statistics
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
OUT = os.path.join(HERE, "out")
DOC = os.path.join(REPO, "docs", "STATUS.md")
BEGIN, END = "<!-- gates:begin -->", "<!-- gates:end -->"

def fps_table():
    rows = []
    for d in sorted(glob.glob(f"{OUT}/fps/*/")):
        runs = {}
        try:
            for line in open(d + "rounds.txt"):
                kv = dict(p.split("=", 1) for p in line.split() if "=" in p)
                runs.setdefault(kv["label"], []).append(kv)
        except OSError:
            continue
        if "rust" not in runs:
            continue
        med = lambda l, k: statistics.median(float(r[k]) for r in runs[l])
        ratio = lambda l, k: f"{med(l, k) / med('rust', k):.3f}" if l in runs else "-"
        name = os.path.basename(d.rstrip("/"))
        scene, hz = name.rsplit("-", 2)[0], name.rsplit("-", 1)[1].replace("hz", " Hz")
        verdict = ""
        try:
            verdict = next(l for l in open(d + "summary.md") if l.startswith("GATE")).split(" ", 4)[3]
        except (OSError, StopIteration):
            pass
        rows.append(f"| {scene} | {hz} | {len(runs['rust'])} | {med('rust', 'cpu_p50_us'):.1f} µs | {ratio('rust2', 'cpu_p50_us')} | "
                    f"{ratio('go', 'cpu_p50_us')}, {ratio('go', 'cpu_p95_us')} | "
                    f"{1000 / med('rust', 'interval_p50_ms'):.0f}, "
                    + (f"{1000 / med('go', 'interval_p50_ms'):.0f}" if "go" in runs else "-")
                    + f" | {verdict} |")
    head = ["| scene | output | rounds | Rust CPU p50 | rust2 / Rust | Go / Rust (p50, p95) | "
            "frames per second shown (Rust, Go) | verdict |",
            "|---|---|---|---|---|---|---|---|"]
    return head + rows

def main():
    summary = os.path.join(OUT, "summary.md")
    age = time.time() - os.path.getmtime(summary)
    if age > 3600 and "--any-age" not in sys.argv:
        sys.exit(f"gates/status.py: gates/out/summary.md is {age / 60:.0f} minutes old; run make gates first")
    when = datetime.datetime.fromtimestamp(os.path.getmtime(summary)).strftime("%-d %B %Y, %H:%M")
    gover = os.environ.get("GK_GO_VERSION")
    if not gover:
        try:
            gover = subprocess.run(["go", "env", "GOVERSION"], capture_output=True, text=True).stdout.strip() or "unknown"
        except OSError:
            gover = "unknown"
    o = [f"Last run of `gates/all.sh`: {when} ({gover}). "
         "XFAIL is a gate known to fail today (`gates/expected.txt`).", "", open(summary).read().strip(), ""]
    o += ["Frame cost in full (ship library, 300 measured frames a round; medians over all rounds, "
          "`rust2` is the Rust reference run a second time in each round; the verdict is paired by round "
          "and leaves out rounds another job disturbed, so it can differ from these columns):", ""]
    o += fps_table() + [""]
    sys.path.insert(0, HERE)
    import gallery
    for sub, name in (("gallery-go", "Go"),):
        if not os.path.exists(f"{OUT}/{sub}/result.json"):
            continue
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            gallery.report(f"{OUT}/{sub}")
        o += [f"The {name} gallery, page by page (hand-written pages, then the pages `gates/conformance.py` "
              "generates, one per bound function):", "", buf.getvalue().strip(), ""]
    doc = open(DOC).read()
    a, b = doc.index(BEGIN) + len(BEGIN), doc.index(END)
    open(DOC, "w").write(doc[:a] + "\n" + "\n".join(o) + doc[b:])
    print("gates/status.py: docs/STATUS.md gates block rewritten")

if __name__ == "__main__":
    main()
