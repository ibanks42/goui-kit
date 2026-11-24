#!/usr/bin/env python3
"""Sample process RSS and judge memory growth of a front end against Rust.

Usage:
  memory.py sample <out.csv> -- <cmd...>   Run <cmd>, poll /proc/<pid>/statm every 20 ms, write CSV.
  memory.py verdict <name> <frames> <dir>  Compare rust.csv and <GK_FRONT>.csv in <dir>.

What it gates:
  Long-running resident memory (RSS) stability of Go apps against their Rust reference twins.

How it measures and why the comparison is fair:
  - `sample` reads `/proc/<pid>/statm` field 1 (resident pages * system page size) every 20 ms
    and stops the process immediately if RSS exceeds `GK_MEM_LIMIT_MB` (default 3072 MB).
  - `windows` compares median RSS in the middle window [40%, 50%] of elapsed time against the
    final window [90%, 100%]. Waiting until 40% skips startup, font/glyph cache warm-up, GPU
    buffer setup, and initial Go GC sizing, so only steady-state drift is measured.
  - `verdict` subtracts Rust's growth from the front end's growth (`over = growth[front] -
    growth["rust"]`). Shared growth inside GPUI, graphics drivers, or system libraries cancels
    out, isolating leaks introduced by the bindings or Go runtime.
  - Per-frame excess (`bytes a frame`) divides `over` by `frames / 2` because the midpoints of
    the two windows (45% and 95%) are separated by half of the run.

What PASS means (exit 0):
  Both `rust` and `GK_FRONT` collect at least 50 samples (~1 s, ensuring crashed runs cannot
  pass as zero growth), neither hits `GK_MEM_LIMIT_MB`, and `over <= SLACK_KB` (default 2048 kB).
"""
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
    # Hard safety cap so a runaway leak stops before exhausting host RAM.
    limit_kb = int(os.environ.get("GK_MEM_LIMIT_MB", "3072")) * 1024
    while p.poll() is None:
        try:
            # /proc/<pid>/statm field 1 is resident set size in pages.
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
    # Compare median RSS in the middle window [40%, 50%] (past startup/JIT/allocator warmup)
    # against the final window [90%, 100%] to measure steady-state memory growth.
    end = rows[-1][0]
    pick = lambda a, b: statistics.median([r for t, r in rows if a * end <= t <= b * end] or [rows[-1][1]])
    return pick(0.4, 0.5), pick(0.9, 1.0), end, max(r for _, r in rows)

def verdict(name, frames, d):
    slack = int(os.environ.get("SLACK_KB", "2048"))
    why, growth, cells = [], {}, []
    front = os.environ.get("GK_FRONT", "go")
    title = {"go": "Go"}.get(front, front)
    for b in ("rust", front):
        rows = read(f"{d}/{b}.csv")
        # At 20 ms polling, 50 samples is roughly 1 second; fewer means the run died early.
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
        # Divide excess growth by frames/2 because [40%,50%] to [90%,100%] spans half the run.
        over = growth[front] - growth["rust"]
        per_frame = over * 1024 / (frames / 2) if frames else 0
        cells.append(f"{title} over Rust {over:+.0f} kB ({per_frame:+.0f} bytes a frame; limit {slack} kB)")
        if over > slack:
            why.append(f"the {title} app grows more than the Rust reference")
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
