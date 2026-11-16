#!/usr/bin/env python3
"""status.py: writes the gates block of docs/STATUS.md from the last run in gates/out/.

Between the markers <!-- gates:begin --> and <!-- gates:end --> it puts: the summary table
gates/all.sh printed (gates/out/summary.md), the frame-cost table of every scene measured
(gates/out/fps/*/rounds.txt, medians over all rounds), and the gallery's page tables
(gates/out/gallery/result.json). Nothing in that block is typed by hand. Run it after
`make gates`; it refuses a summary older than an hour unless given --any-age.
Standard library only."""
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
                    f"{ratio('odin', 'cpu_p50_us')}, {ratio('odin', 'cpu_p95_us')} | "
                    f"{ratio('odin-speed', 'cpu_p50_us')}, {ratio('odin-speed', 'cpu_p95_us')} | "
                    f"{1000 / med('rust', 'interval_p50_ms'):.0f}, "
                    + (f"{1000 / med('odin-speed', 'interval_p50_ms'):.0f}" if "odin-speed" in runs else "-")
                    + f" | {verdict} |")
    head = ["| scene | output | rounds | Rust CPU p50 | rust2 / Rust | Odin `-o:minimal` / Rust (p50, p95) | "
            "Odin `-o:speed` / Rust (p50, p95) | frames per second shown (Rust, Odin) | verdict |",
            "|---|---|---|---|---|---|---|---|---|"]
    return head + rows


def main():
    summary = os.path.join(OUT, "summary.md")
    age = time.time() - os.path.getmtime(summary)
    if age > 3600 and "--any-age" not in sys.argv:
        sys.exit(f"gates/status.py: gates/out/summary.md is {age / 60:.0f} minutes old; run make gates first")
    when = datetime.datetime.fromtimestamp(os.path.getmtime(summary)).strftime("%-d %B %Y, %H:%M")
    # GK_ODIN_VERSION: the compiler's version when gates/out was copied from another machine.
    # The block names no machine: the ratios against the Rust reference of the same run are
    # what carries over.
    odin = os.environ.get("GK_ODIN_VERSION")
    if not odin:
        try:
            odin = subprocess.run([os.environ.get("ODIN", "odin"), "version"],
                                  capture_output=True, text=True).stdout.split()[-1]
        except (OSError, IndexError):
            odin = "unknown"
    o = [f"Last run of `gates/all.sh`: {when} (Odin {odin}). "
         "XFAIL is a gate known to fail today (`gates/expected.txt`).", "", open(summary).read().strip(), ""]
    o += ["Frame cost in full (ship library, 300 measured frames a round; medians over all rounds, "
          "`rust2` is the Rust reference run a second time in each round; the verdict is paired by round "
          "and leaves out rounds another job disturbed, so it can differ from these columns):", ""]
    o += fps_table() + [""]
    if os.path.exists(f"{OUT}/gallery/result.json"):
        sys.path.insert(0, HERE)
        import gallery
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            gallery.report(f"{OUT}/gallery")
        o += ["The gallery, page by page (hand-written pages, then the pages `gates/conformance.py` generates, "
              "one per bound function):", "", buf.getvalue().strip(), ""]
    doc = open(DOC).read()
    a, b = doc.index(BEGIN) + len(BEGIN), doc.index(END)
    open(DOC, "w").write(doc[:a] + "\n" + "\n".join(o) + doc[b:])
    print("gates/status.py: docs/STATUS.md gates block rewritten")


if __name__ == "__main__":
    main()
