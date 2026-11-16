#!/usr/bin/env python3
"""fps-summary.py --hz N --name NAME [--zero-crossings] DIR: the verdict of gates/fps.sh.

DIR holds rounds.txt (one gk-meter line per run, prefixed round=R) and LABEL-R.csv (one row
per measured frame). Prints medians over rounds, each binary's ratio to the Rust reference,
the Rust-against-Rust pair as the noise, missed frames at this refresh rate, and one line
"GATE fps NAME PASS|FAIL ...". Exit 0 on pass.

The verdict is paired by round and leaves out rounds another job disturbed (see QUIET
below); the tables above it show every round."""
import csv
import glob
import os
import statistics
import sys

args = sys.argv[1:]
hz = float(args[args.index("--hz") + 1])
name = args[args.index("--name") + 1]
zero = "--zero-crossings" in args
d = args[-1]
period_ns = 1e9 / hz
BAR = float(os.environ.get("GK_BAR", "1.03"))  # the pass mark: within 3 % of pure Rust (Go: 1.15, docs/GO.md)
JUDGED = os.environ.get("GK_JUDGED", "odin-speed")  # the build an app ships with; gates/go.sh judges go

runs = {}
path = os.path.join(d, "rounds.txt")
for line in open(path) if os.path.exists(path) else []:
    kv = dict(p.split("=", 1) for p in line.split() if "=" in p)
    runs.setdefault(kv["label"], []).append(kv)
if "rust" not in runs:
    print(f"GATE fps {name} FAIL no Rust reference run")
    sys.exit(1)

missed, frames = {}, {}
for label in runs:
    m, n = [], 0
    for f in sorted(glob.glob(os.path.join(d, f"{label}-*.csv"))):
        rows = list(csv.DictReader(open(f)))
        n = max(n, len(rows))
        m.append(sum(1 for r in rows if int(r["interval_ns"]) > 1.5 * period_ns))
    missed[label] = statistics.median(m) if m else float("nan")
    frames[label] = n


def med(label, key):
    return statistics.median(float(r[key]) for r in runs[label])


labels = [l for l in ("rust", "rust2", "odin", "odin-speed", "go") if l in runs]
keys = ["cpu_p50_us", "cpu_p95_us", "cpu_mean_us", "interval_p50_ms", "interval_p95_ms",
        "callbacks_per_frame", "calls_per_frame", "callback_us_per_frame",
        "decode_us_per_frame", "tapes_per_frame", "tape_bytes_per_frame", "rss_peak_kb"]
print(f"# {name}: {len(runs['rust'])} rounds of {frames['rust']} frames, medians over rounds; "
      f"output at {hz:g} Hz (period {period_ns / 1e6:.3f} ms)")
print()
print("| per frame | " + " | ".join(labels) + " |")
print("|---|" + "---|" * len(labels))
for k in keys:
    print(f"| {k} | " + " | ".join(f"{med(l, k):.3f}" for l in labels) + " |")
print("| frames per second shown | " + " | ".join(f"{1000 / med(l, 'interval_p50_ms'):.1f}" for l in labels) + " |")
print("| frames per second the main thread has time for (1 / cpu_p50) | "
      + " | ".join(f"{1e6 / med(l, 'cpu_p50_us'):.0f}" for l in labels) + " |")
print("| missed frames (interval over 1.5 periods) | " + " | ".join(f"{missed[l]:g}" for l in labels) + " |")
print()
print("| ratio to rust | " + " | ".join(labels[1:]) + " |")
print("|---|" + "---|" * (len(labels) - 1))
ratio = {}
for k in ("cpu_p50_us", "cpu_p95_us", "cpu_mean_us", "interval_p50_ms"):
    base = med("rust", k)
    ratio[k] = {l: med(l, k) / base for l in labels[1:]}
    print(f"| {k} | " + " | ".join(f"{ratio[k][l]:.3f}" for l in labels[1:]) + " |")
print()
for l in labels[1:]:
    n = min(len(runs[l]), len(runs["rust"]))
    rs = sorted(float(runs[l][i]["cpu_p50_us"]) / float(runs["rust"][i]["cpu_p50_us"]) for i in range(n))
    print(f"{l}/rust cpu_p50 per round: min {rs[0]:.3f} median {statistics.median(rs):.3f} max {rs[-1]:.3f}")

stalls = {}
sp = os.path.join(d, "stalls.txt")
for line in open(sp) if os.path.exists(sp) else []:
    stalls[line.split()[0]] = stalls.get(line.split()[0], 0) + 1
if stalls:
    print("runs that showed no frame and were repeated: " + ", ".join(f"{k} {v}" for k, v in sorted(stalls.items())))

judged = JUDGED if JUDGED in runs else ("odin" if "odin" in runs else None)
if judged is None:
    print(f"GATE fps {name} FAIL no Odin run")
    sys.exit(1)
# The verdict is paired by round: within a round the four binaries ran one after another,
# so their ratio is free of whatever the machine was doing in that minute. A round in which
# the two Rust runs differ by more than QUIET (another job was running, e.g. a build beside the gate) says nothing
# about the Odin app and is left out; the choice looks only at Rust. Fewer than half the
# rounds (or fewer than three) left is a failed measurement, to be run again.
QUIET = 0.05
needed = ("rust", judged) + (("rust2",) if "rust2" in runs else ())
at = {l: {r["round"]: r for r in runs[l]} for l in needed}
complete = sorted(set.intersection(*(set(at[l]) for l in needed)), key=int)
n_rounds = len(complete)
by_round = lambda l, k, i: float(at[l][i][k])
quiet = [i for i in complete
         if "rust2" not in runs or abs(by_round("rust2", "cpu_p50_us", i) / by_round("rust", "cpu_p50_us", i) - 1) <= QUIET]
why = []
detail = []
disturbed = len(quiet) < max(3, (n_rounds + 1) // 2) and n_rounds >= 3
if len(quiet) < n_rounds:
    detail.append(f"{n_rounds - len(quiet)} of {n_rounds} rounds left out as disturbed")
use = quiet or complete
for k in ("cpu_p50_us", "cpu_p95_us"):
    paired = lambda l: statistics.median(by_round(l, k, i) / by_round("rust", k, i) for i in use)
    # The noise is what Rust against itself shows in this run: the larger of its bias (the
    # median of rust2 / rust away from 1) and the uncertainty of such a median over this many
    # rounds (its standard error, 1.2533 sigma / sqrt(n), sigma from the median absolute
    # deviation so that one wild round does not open the limit). The bias alone was often
    # 0.00x while the rounds spread by several per cent, so a binding at 1.02 of Rust read
    # anywhere from 1.026 to 1.048 on the click-on-every-frame scene (in profiles,
    # the binding's own functions were 19 us of a 1,000 us frame; the rest of the difference
    # moved between runs in the kernel time of the same system calls, counted equal for both).
    noise = 0.0
    if "rust2" in runs:
        rr = [by_round("rust2", k, i) / by_round("rust", k, i) for i in use]
        m = statistics.median(rr)
        sigma = 1.4826 * statistics.median(abs(x - m) for x in rr)
        se = 1.2533 * sigma / len(rr) ** 0.5 if len(rr) > 2 else 0.0
        noise = max(abs(m - 1), se)
    limit = BAR + noise
    r = paired(judged)
    detail.append(f"{k[4:7]} {r:.3f} (limit {limit:.3f})")
    if r > limit:
        why.append(f"{k} {r:.3f} over {limit:.3f}")
tolerated = 0.01 * frames["rust"]
detail.append(f"missed {missed[judged]:g} against Rust's {missed['rust']:g}")
if missed[judged] > missed["rust"] + tolerated:
    why.append("more missed frames than Rust")
if stalls.get(judged, 0) > max(stalls.get("rust", 0), stalls.get("rust2", 0)) + 1:
    why.append(f"{stalls[judged]} stalled runs against Rust's {stalls.get('rust', 0)}")
if len(runs[judged]) < len(runs["rust"]):
    why.append(f"only {len(runs[judged])} of {len(runs['rust'])} rounds completed")
if zero:
    c = max(med(l, "callbacks_per_frame") + med(l, "calls_per_frame") for l in labels if l.startswith("odin"))
    detail.append(f"crossings per frame {c:g}")
    if c != 0:
        why.append("crossings on frames with nothing changing")
print()
if disturbed:
    # No verdict on the Odin app: exit 3, and gates/fps.sh measures the scene once more.
    print(f"GATE fps {name} FAIL {judged}: the measurement was disturbed, only {len(quiet)} of {n_rounds} rounds had the two "
          f"Rust runs within {QUIET:.0%} of each other (another job was running): run it again; " + ", ".join(detail))
    sys.exit(3)
print(f"GATE fps {name} {'FAIL' if why else 'PASS'} {judged}: " + ", ".join(detail) + ("; " + "; ".join(why) if why else ""))
sys.exit(1 if why else 0)
