#!/usr/bin/env python3
"""licences.py OUT_DIR: the licence texts of every crate linked into libgpuikit, for the download.

Runs in the checkout after the library was built (tools/build-lib.sh runs it), when cargo's
registry holds every crate's sources. Standard library only.

Which crates: `cargo metadata` resolves the workspace for the library's target (x86_64 Linux, or
GK_LICENCE_TARGET: tools/build-lib.sh passes the CI runner's triple, x86_64-pc-windows-msvc, or on
macOS both slices of the universal library, "aarch64-apple-darwin x86_64-apple-darwin"; the
graphs differ per target), once for the shipped library (package gk-capi with the workspace's
features) and once for the diagnostic one (feature diag), for each target, and this script walks
gk-capi's normal dependencies. Build-script and dev dependencies are not linked and are left out;
so are proc-macro crates (they run in the compiler and leave no code in the library). What is
linked is the union of the walks; with more than one target, crates linked for only some of them
are named in THIRD_PARTY.md (with Cargo.lock as committed: x86_64 macOS links
multiversion 0.8.1 and target-features 0.1.6, which arm64 does not, and core_detect in the shipped
variant, which arm64 links only in diag; 469 crates for the two together, 467 for arm64 alone).

Writes:
  OUT_DIR/THIRD_PARTY.md        one table: crate, version, licence, where its text is, source
  OUT_DIR/texts/NAME-VERSION/   every LICENSE*, LICENCE*, COPYING*, NOTICE*, UNLICENSE*,
                                COPYRIGHT* file the crate ships, verbatim
A crate that ships no licence file is listed with its declared licence and authors, and the
notice says so; the canonical texts of the licences such crates name are taken from another
crate that ships that licence verbatim (texts/_canonical/). Copyleft licences are listed
first in the notice.
"""
import json
import os
import re
import shutil
import subprocess
import sys

TARGETS = (os.environ.get("GK_LICENCE_TARGET") or "x86_64-unknown-linux-gnu").replace(",", " ").split()
ROOT_PKG = "gk-capi"
FILES = re.compile(r"^(LICEN[CS]E|COPYING|NOTICE|UNLICENSE|COPYRIGHT)", re.I)
COPYLEFT = re.compile(r"\b(A?GPL|LGPL|MPL|EPL|CDDL|EUPL|OSL|CC-BY-SA|CECILL)", re.I)


# GK_ENGINE=gpui: the shipped library was built on upstream GPUI (--no-default-features); the
# diagnostic one is always on gpui-fast.
ENGINE = os.environ.get("GK_ENGINE", "gpui-fast")


def metadata(features, target):
    cmd = ["cargo", "metadata", "--format-version", "1", "--locked", "--filter-platform", target]
    if features is None:
        cmd += ["--no-default-features"]
    elif features:
        cmd += ["--features", features]
    return json.loads(subprocess.run(cmd, check=True, capture_output=True, text=True, encoding="utf-8").stdout)


def linked(meta):
    pkgs = {p["id"]: p for p in meta["packages"]}
    nodes = {n["id"]: n for n in meta["resolve"]["nodes"]}
    root = next(p["id"] for p in meta["packages"] if p["name"] == ROOT_PKG)
    seen, todo = set(), [root]
    while todo:
        pid = todo.pop()
        if pid in seen:
            continue
        seen.add(pid)
        for d in nodes[pid]["deps"]:
            if any(k["kind"] is None for k in d["dep_kinds"]):
                todo.append(d["pkg"])
    out = []
    for pid in seen:
        p = pkgs[pid]
        kinds = {k for t in p["targets"] for k in t["kind"]}
        if kinds == {"proc-macro"} or p["name"] == ROOT_PKG or p["source"] is None:
            continue  # compiler plugins, and this workspace's own crates
        out.append(p)
    return out


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else "dist/licences"
    ship, diag = {}, {}
    per_target = {}  # target -> ids linked for it (either variant)
    for target in TARGETS:
        s = {p["id"]: p for p in linked(metadata(None if ENGINE == "gpui" else "", target))}
        d = {p["id"]: p for p in linked(metadata("gk-capi/diag", target))}
        ship.update(s)
        diag.update(d)
        per_target[target] = set(s) | set(d)
    allp = dict(ship)
    allp.update(diag)
    shutil.rmtree(out, ignore_errors=True)
    os.makedirs(os.path.join(out, "texts"))
    rows, missing, copyleft = [], [], []
    canonical = {}  # licence id -> a file that holds its text verbatim
    for pid, p in sorted(allp.items(), key=lambda kv: (kv[1]["name"], kv[1]["version"])):
        name, ver = p["name"], p["version"]
        lic = (p.get("license") or "").strip() or ("see " + p["license_file"] if p.get("license_file") else "none declared")
        src = os.path.dirname(p["manifest_path"])
        files = sorted(f for f in os.listdir(src) if FILES.match(f) and os.path.isfile(os.path.join(src, f)))
        if p.get("license_file") and os.path.basename(p["license_file"]) not in files:
            lf = os.path.join(src, p["license_file"])
            if os.path.isfile(lf):
                files.append(os.path.relpath(lf, src))
        dest = os.path.join("texts", f"{name}-{ver}")
        if files:
            os.makedirs(os.path.join(out, dest))
            for f in files:
                shutil.copyfile(os.path.join(src, f), os.path.join(out, dest, os.path.basename(f)))
                up = os.path.basename(f).upper()
                if "APACHE" in up:
                    canonical.setdefault("Apache-2.0", os.path.join(src, f))
                elif up in ("LICENSE-MIT", "LICENSE-MIT.MD", "LICENSE-MIT.TXT"):
                    canonical.setdefault("MIT", os.path.join(src, f))
        else:
            missing.append((name, ver, lic, ", ".join(p.get("authors") or []) or "no authors listed"))
            dest = "(none shipped; see below)"
        where = "ship and diag" if pid in ship and pid in diag else ("diag only" if pid in diag else "ship only")
        source = p["source"].split("+", 1)[-1].split("?")[0] if p["source"] else "workspace"
        rows.append((name, ver, lic, dest, where, source))
        if COPYLEFT.search(lic):
            copyleft.append((name, ver, lic, where))
    if missing:
        os.makedirs(os.path.join(out, "texts", "_canonical"))
        for lid, path in canonical.items():
            shutil.copyfile(path, os.path.join(out, "texts", "_canonical", lid + ".txt"))
    with open(os.path.join(out, "THIRD_PARTY.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write("# Third-party software in libgpuikit\n\n")
        f.write(f"libgpuikit links {len(allp)} crates from crates.io and git ({len(ship)} in the shipped "
                f"library, {len(diag)} in the diagnostic one). Each is listed below with the licence it "
                "declares; the licence files each crate ships are in `texts/NAME-VERSION/`, verbatim. "
                f"Written by tools/licences.py from Cargo.lock and the crates' sources, for "
                f"{' and '.join(TARGETS)}"
                f"{' (every slice of the universal library: the union of their crates)' if len(TARGETS) > 1 else ''}.\n\n")
        if len(TARGETS) > 1:
            f.write("## Crates linked for some targets only\n\n")
            some = [(allp[pid]["name"], allp[pid]["version"], [t for t in TARGETS if pid in per_target[t]])
                    for pid in allp if not all(pid in per_target[t] for t in TARGETS)]
            if some:
                for name, ver, ts in sorted(some):
                    f.write(f"- {name} {ver}: {', '.join(ts)}\n")
                f.write("\n")
            else:
                f.write(f"None: every crate below is linked for all of {', '.join(TARGETS)}.\n\n")
        f.write("## Copyleft\n\n")
        if copyleft:
            f.write("These crates are under a copyleft licence, alone or as one choice of an OR:\n\n")
            for name, ver, lic, where in copyleft:
                f.write(f"- {name} {ver}: {lic} ({where})\n")
            f.write("\nWhere the licence is `X OR Y`, the library uses the crate under the permissive choice.\n\n")
        else:
            f.write("None: no linked crate declares a GPL, LGPL, AGPL, MPL, EPL, CDDL, EUPL, OSL or CC-BY-SA licence.\n\n")
        if missing:
            f.write("## Crates that ship no licence file\n\n")
            f.write("Their declared licence applies; the canonical texts are in `texts/_canonical/`.\n\n")
            for name, ver, lic, authors in missing:
                f.write(f"- {name} {ver}: {lic}; copyright its authors: {authors}\n")
            f.write("\n")
        f.write("## Every crate\n\n| crate | version | licence | texts | in | source |\n|---|---|---|---|---|---|\n")
        for r in rows:
            f.write("| " + " | ".join(r) + " |\n")
    counts = {}
    for r in rows:
        counts[r[2]] = counts.get(r[2], 0) + 1
    for target in TARGETS[1:] if len(TARGETS) > 1 else []:
        only = sorted(f"{allp[i]['name']} {allp[i]['version']}" for i in per_target[target] - per_target[TARGETS[0]])
        less = sorted(f"{allp[i]['name']} {allp[i]['version']}" for i in per_target[TARGETS[0]] - per_target[target])
        print(f"licences: {target} against {TARGETS[0]}: {len(only)} more ({', '.join(only) or 'none'}), "
              f"{len(less)} fewer ({', '.join(less) or 'none'})")
    print(f"licences: {len(allp)} crates ({len(ship)} ship, {len(diag)} diag) for {' '.join(TARGETS)}, "
          f"{len(missing)} without a licence file, "
          f"{len(copyleft)} with a copyleft licence{': ' + ', '.join(c[0] + ' ' + c[2] for c in copyleft) if copyleft else ''}")
    for lic, n in sorted(counts.items(), key=lambda kv: -kv[1])[:12]:
        print(f"  {n:4d}  {lic}")


if __name__ == "__main__":
    main()
