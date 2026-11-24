#!/usr/bin/env python3
"""symbolize.py: what tools/profile/sampler.c recorded, by function, for two or more groups of
runs (e.g. the Rust reference and the Go app), as main-thread CPU microseconds per frame.

  symbolize.py [--period-us 50] [--skip 2.0] [--hz 60] [--top 40] NAME=PREFIX[,PREFIX...] ...

Each PREFIX is a GK_PROF_OUT of one run (PREFIX.samples, PREFIX.maps). Samples in the first
--skip seconds of a run (start-up and warm-up frames) are left out. A sample stands for the
main thread's CPU time since the one before (the sampler records that clock); a frame is 1/--hz seconds of wall time (the gates
pace at 60 Hz). Symbols come from the binaries the maps name (`nm -n -C`), so they must not
be stripped (gates/profile.sh builds unstripped copies). Standard library only.
"""
import bisect
import collections
import os
import re
import struct
import subprocess
import sys


def load_symbols(path, cache={}):
    if path in cache:
        return cache[path]
    addrs, names = [], []
    segs = []
    try:
        out = subprocess.run(["nm", "-n", "-C", "--defined-only", path], capture_output=True, text=True).stdout
        if not out.strip():  # a stripped system library: its exported symbols
            out = subprocess.run(["nm", "-D", "-n", "-C", "--defined-only", path], capture_output=True, text=True).stdout
        for line in out.splitlines():
            p = line.split(" ", 2)
            if len(p) == 3 and p[1] in "tTwW":
                addrs.append(int(p[0], 16))
                names.append(re.sub(r"::h[0-9a-f]{16}$", "", p[2]))
        ro = subprocess.run(["readelf", "-lW", path], capture_output=True, text=True).stdout
        for line in ro.splitlines():
            f = line.split()
            if f and f[0] == "LOAD":
                segs.append((int(f[1], 16), int(f[2], 16), int(f[4], 16)))  # offset, vaddr, filesz
    except OSError:
        pass
    cache[path] = (addrs, names, segs)
    return cache[path]


def maps_of(prefix):
    out = []
    for line in open(prefix + ".maps"):
        f = line.split()
        if len(f) < 5 or "x" not in f[1]:
            continue
        lo, hi = (int(x, 16) for x in f[0].split("-"))
        out.append((lo, hi, int(f[2], 16), f[5] if len(f) > 5 else "[anonymous executable memory: JIT code]"))
    out.sort()
    return out


def where(ip, maps):
    i = bisect.bisect_right([m[0] for m in maps], ip) - 1
    if i < 0 or ip >= maps[i][1]:
        return "?", "?"
    lo, hi, off, path = maps[i]
    if not path.startswith("/"):
        return path, path
    fo = ip - lo + off
    addrs, names, segs = load_symbols(path)
    va = fo
    for so, sv, sz in segs:
        if so <= fo < so + sz:
            va = fo - so + sv
            break
    j = bisect.bisect_right(addrs, va) - 1
    return os.path.basename(path), (names[j] if j >= 0 else f"{os.path.basename(path)}+{va:#x}")


def main(argv):
    opts = {"--period-us": 50.0, "--skip": 2.0, "--hz": 60.0, "--top": 40}
    groups = []
    i = 0
    while i < len(argv):
        a = argv[i]
        if a in opts:
            opts[a] = type(opts[a])(argv[i + 1])
            i += 2
            continue
        name, prefixes = a.split("=", 1)
        groups.append((name, prefixes.split(",")))
        i += 1
    per_sym, per_dso, frames, dso_of = {}, {}, {}, {}
    for name, prefixes in groups:
        syms, dsos, nframes, where_of = collections.Counter(), collections.Counter(), 0.0, {}
        for p in prefixes:
            data = open(p + ".samples", "rb").read()
            maps = maps_of(p)
            last, prev = 0, None
            for ip, ns, cpu in struct.iter_unpack("<QQQ", data):
                t = ns / 1e9
                last = max(last, t)
                w, prev = (cpu - prev if prev is not None else 0), cpu
                if t < opts["--skip"]:
                    continue
                dso, sym = where(ip, maps)
                syms[sym] += w
                dsos[dso] += w
                where_of[sym] = dso
            nframes += max(0.0, last - opts["--skip"]) * opts["--hz"]
        per_sym[name], per_dso[name], frames[name], dso_of[name] = syms, dsos, nframes, where_of
    us = lambda name, n: n / 1000.0 / frames[name] if frames[name] else 0.0  # noqa: E731  (weights are CPU ns)
    names = [g[0] for g in groups]
    print(f"main-thread CPU per frame by group (each sample weighted by the thread's CPU time since the one before; first {opts['--skip']:g} s left out, {opts['--hz']:g} frames a second):")
    for nm in names:
        tot = sum(per_sym[nm].values())
        print(f"  {nm}: {us(nm, tot):.1f} us a frame over {frames[nm]:.0f} frames")
    def category(dso, sym):
        if dso.startswith("[anonymous") or "gallium" in dso or "LLVM" in dso or "libGL" in dso or "libEGL" in dso or "nvidia" in dso or "vulkan" in dso:
            return "software rendering (llvmpipe, its JIT code, GL)"
        if sym.startswith(("gpuikit::", "<gpuikit::")) or dso.endswith("-go"):
            return "the binding: decoders, runtime, the Go app"
        if "gk_meter" in sym:
            return "the meter (both)"
        if re.search(r"gpui_component|gpui_base|gpui_kit", sym):
            return "the kit (gpui-component, gpui-base)"
        if re.search(r"\bgpui\b|gpui::|taffy|cosmic_text|harfrust|swash|etagere", sym):
            return "gpui-fast (layout, text, paint)"
        if dso.startswith("libc") or dso.startswith("[vdso]") or dso.startswith("ld-linux"):
            return "libc, vdso, loader"
        return "other Rust and system code"
    cats = {}
    for nm in names:
        c = collections.Counter()
        for k, v in per_sym[nm].items():
            c[category(dso_of[nm].get(k, ""), k)] += v
        cats[nm] = c
    print("\nby part (us a frame):")
    for k in sorted(set().union(*[cats[n] for n in names]), key=lambda k: -max(us(n, cats[n][k]) for n in names)):
        print("  " + "  ".join(f"{us(n, cats[n][k]):8.1f}" for n in names) + f"  {k}")
    for nm in names:
        mine = [(k, v) for k, v in per_sym[nm].items() if category(dso_of[nm].get(k, ""), k).startswith("the binding")]
        if mine:
            print(f"\nthe binding's own functions in {nm} (us a frame):")
            for k, v in sorted(mine, key=lambda kv: -kv[1])[:15]:
                print(f"  {us(nm, v):8.1f}  {k[:150]}")
    print("\nby binary (us a frame):")
    keys = sorted(set().union(*[per_dso[n] for n in names]), key=lambda k: -max(us(n, per_dso[n][k]) for n in names))
    for k in keys[:15]:
        print("  " + "  ".join(f"{us(n, per_dso[n][k]):8.1f}" for n in names) + f"  {k}")
    print(f"\nby function (us a frame; the {opts['--top']} largest, then the {opts['--top']} largest differences {names[-1]} - {names[0]}):")
    allk = set().union(*[per_sym[n] for n in names])
    hdr = "  ".join(f"{n:>8}" for n in names)
    print(f"  {hdr}  function")
    for k in sorted(allk, key=lambda k: -max(us(n, per_sym[n][k]) for n in names))[:opts["--top"]]:
        print("  " + "  ".join(f"{us(n, per_sym[n][k]):8.1f}" for n in names) + f"  {k[:150]}")
    if len(names) >= 2:
        a, b = names[0], names[-1]
        print(f"\n  {'diff':>8}  function")
        for k in sorted(allk, key=lambda k: -abs(us(b, per_sym[b][k]) - us(a, per_sym[a][k])))[:opts["--top"]]:
            print(f"  {us(b, per_sym[b][k]) - us(a, per_sym[a][k]):+8.1f}  {k[:150]}")


if __name__ == "__main__":
    main(sys.argv[1:])
