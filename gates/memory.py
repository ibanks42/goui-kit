#!/usr/bin/env python3
"""memory.py sample|verdict ...: resident memory over a long run (gates/memory.sh).

    memory.py sample OUT.csv -- CMD ARGS...    run CMD, write its resident set every 20 ms
    memory.py verdict NAME FRAMES DIR          judge DIR/rust.csv against DIR/odin.csv

sample follows the process it starts (not its children) through /proc/PID/statm and exits
with CMD's status; the command's output goes to OUT.log.

verdict compares the resident set at the middle of the run (median of the samples between
40 % and 50 % of its duration, when every page has been shown many times) with the end
(median of the last 10 %). A leak of b bytes a frame shows as growth of about half the
run's frames times b. Pass: the Odin app grows no more than the Rust reference plus
SLACK_KB (default 2048 kB, about 1 % of either process; the spread of this number between
two equal runs), and both finished. Prints the table rows and one line "GATE memory NAME".
Standard library only."""
import os
import statistics
import subprocess
import sys
import time

PAGE_KB = os.sysconf("SC_PAGE_SIZE") // 1024


def sample(out, cmd):
    log = open(os.path.splitext(out)[0] + ".log", "w")
    start = time.monotonic()
    p = subprocess.Popen(cmd, stdout=log, stderr=subprocess.STDOUT)
    rows = []
    # A run past GK_MEM_LIMIT_MB (default 3072) is stopped here and fails the gate: on 11 Nov
    # 2026 a gallery page that doubled an input's text on every visit took the Rust reference
    # to 6.9 GB, and the scope's OOM killer stopped the whole gate run instead of one verdict.
    limit_kb = int(os.environ.get("GK_MEM_LIMIT_MB", "3072")) * 1024
    while p.poll() is None:
        try:
            rss = int(open(f"/proc/{p.pid}/statm").read().split()[1]) * PAGE_KB
            rows.append((time.monotonic() - start, rss))
            if rss > limit_kb:
                p.kill()
                p.wait()
                log.write(f"memory.py: stopped at {rss // 1024} MB resident, over GK_MEM_LIMIT_MB\n")
                break
        except (OSError, IndexError, ValueError):
            pass
        time.sleep(0.02)
    with open(out, "w") as f:
        f.write("seconds,rss_kb\n")
        f.writelines(f"{t:.3f},{r}\n" for t, r in rows)
    return p.returncode


def read(path):
    rows = []
    try:
        for line in list(open(path))[1:]:
            t, r = line.split(",")
            rows.append((float(t), int(r)))
    except OSError:
        pass
    return rows


def windows(rows):
    end = rows[-1][0]
    pick = lambda a, b: statistics.median([r for t, r in rows if a * end <= t <= b * end] or [rows[-1][1]])
    return pick(0.4, 0.5), pick(0.9, 1.0), end, max(r for _, r in rows)


def verdict(name, frames, d):
    slack = int(os.environ.get("SLACK_KB", "2048"))
    why, growth, cells = [], {}, []
    for b in ("rust", "odin"):
        rows = read(f"{d}/{b}.csv")
        if len(rows) < 50:
            why.append(f"{b}: only {len(rows)} samples (the run failed or was too short)")
            continue
        try:
            if "over GK_MEM_LIMIT_MB" in open(f"{d}/{b}.log").read():
                why.append(f"{b}: stopped past GK_MEM_LIMIT_MB")
        except OSError:
            pass
        mid, last, secs, peak = windows(rows)
        growth[b] = last - mid
        cells.append(f"{b} {mid / 1024:.1f} -> {last / 1024:.1f} MB ({growth[b]:+.0f} kB, peak {peak / 1024:.1f} MB, {secs:.1f} s)")
        print(f"memory {name} {b}: frames={frames} seconds={secs:.1f} samples={len(rows)} "
              f"rss_mid_kb={mid:.0f} rss_end_kb={last:.0f} growth_kb={growth[b]:.0f} rss_peak_kb={peak}")
    if len(growth) == 2:
        over = growth["odin"] - growth["rust"]
        per_frame = over * 1024 / (frames / 2) if frames else 0
        cells.append(f"Odin over Rust {over:+.0f} kB ({per_frame:+.0f} bytes a frame; limit {slack} kB)")
        if over > slack:
            why.append("the Odin app grows more than the Rust reference")
    print(f"GATE memory {name} {'FAIL' if why else 'PASS'} {frames} frames: " + ", ".join(cells)
          + ("; " + "; ".join(why) if why else ""))
    return 1 if why else 0


if __name__ == "__main__":
    a = sys.argv[1:]
    if len(a) >= 4 and a[0] == "sample" and a[2] == "--":
        sys.exit(sample(a[1], a[3:]))
    if len(a) == 4 and a[0] == "verdict":
        sys.exit(verdict(a[1], int(a[2]), a[3]))
    sys.exit(__doc__)
