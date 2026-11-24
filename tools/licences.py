#!/usr/bin/env python3
"""Collect third-party crate license files and generate THIRD_PARTY.md.

Usage:
    GK_LICENCE_TARGET="target1 target2" python3 tools/licences.py [OUT_DIR]

Environment variables:
    GK_LICENCE_TARGET  Space- or comma-separated Rust target triples to audit (default:
                       `x86_64-unknown-linux-gnu`). On macOS universal builds, `build-lib.sh`
                       passes both `aarch64-apple-darwin` and `x86_64-apple-darwin` so
                       architecture-specific crates are merged into one notice file.
    GK_ENGINE          `gpui-fast` (default) or `gpui` (uses `--no-default-features` for
                       the `ship` dependency graph).

How the dependency graph is computed:
- Queries `cargo metadata --locked --filter-platform <target>` separately for the `ship`
  build and the `diag` build (`--features gk-capi/diag`), then walks only normal runtime
  dependencies (`dep_kinds` containing `kind is None`, excluding `build`/`dev` deps)
  reachable from `gk-capi`.
- Excludes workspace crates (`source is None`), `gk-capi` itself, and proc-macro-only
  crates (`kinds == {"proc-macro"}`), because proc macros run at compile time and are not
  linked into `libgouikit`.
- Copies every `LICENSE*`/`LICENCE*`/`COPYING*`/`NOTICE*`/`UNLICENSE*`/`COPYRIGHT*` file
  from each crate's Cargo source directory (plus any explicit `license_file` field) into
  `OUT_DIR/texts/<name>-<version>/`.
- Crates that ship no license file in their crate tarball are listed separately with their
  SPDX expression and authors, and canonical MIT / Apache-2.0 texts harvested from sibling
  crates are placed in `OUT_DIR/texts/_canonical/`.
- Flags any crate whose license expression matches `COPYLEFT` so dual-licensed (`OR`)
  copyleft choices remain auditable.
"""
import json
import os
import re
import shutil
import subprocess
import sys

TARGETS = (os.environ.get("GK_LICENCE_TARGET") or "x86_64-unknown-linux-gnu").replace(",", " ").split()
ROOT_PKG = "gk-capi"
# Filename prefixes treated as verbatim third-party license or attribution notices.
FILES = re.compile(r"^(LICEN[CS]E|COPYING|NOTICE|UNLICENSE|COPYRIGHT)", re.I)
# SPDX license families highlighted in the Copyleft section of THIRD_PARTY.md.
COPYLEFT = re.compile(r"\b(A?GPL|LGPL|MPL|EPL|CDDL|EUPL|OSL|CC-BY-SA|CECILL)", re.I)


ENGINE = os.environ.get("GK_ENGINE", "gpui-fast")


def metadata(features, target):
    """Run `cargo metadata --locked` for one target platform and feature set.

    `features=None` passes `--no-default-features` (used when `GK_ENGINE=gpui`),
    `features=""` uses default features (`gpui-fast`), and a non-empty string passes
    `--features <features>` (for `gk-capi/diag`).
    """
    cmd = ["cargo", "metadata", "--format-version", "1", "--locked", "--filter-platform", target]
    if features is None:
        cmd += ["--no-default-features"]
    elif features:
        cmd += ["--features", features]
    return json.loads(subprocess.run(cmd, check=True, capture_output=True, text=True, encoding="utf-8").stdout)


def linked(meta):
    """Return all non-workspace, non-proc-macro normal runtime dependencies reachable from `gk-capi`.

    Performs a DFS over `meta["resolve"]["nodes"]` following only edges where at least one
    `dep_kinds` entry has `kind is None` (normal dependency, ignoring build-dependencies and
    dev-dependencies).
    """
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
            continue
        out.append(p)
    return out


def main():
    """Collect ship and diag crate graphs across TARGETS, copy license texts, and write THIRD_PARTY.md."""
    out = sys.argv[1] if len(sys.argv) > 1 else "dist/licences"
    ship, diag = {}, {}
    per_target = {}
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
    canonical = {}
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
        f.write("# Third-party software in libgouikit\n\n")
        f.write(f"libgouikit links {len(allp)} crates from crates.io and git ({len(ship)} in the shipped "
                f"library, {len(diag)} in the diagnostic build). Each crate is listed below with its declared "
                "license; verbatim license files are copied into `texts/NAME-VERSION/`. "
                f"Generated by tools/licences.py from Cargo.lock for "
                f"{' and '.join(TARGETS)}"
                f"{' (union across all universal slices)' if len(TARGETS) > 1 else ''}.\n\n")
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
            f.write("These crates list a copyleft license, alone or as one alternative in an OR expression:\n\n")
            for name, ver, lic, where in copyleft:
                f.write(f"- {name} {ver}: {lic} ({where})\n")
            f.write("\nWhere the license is `X OR Y`, this library uses the crate under the permissive option.\n\n")
        else:
            f.write("None: no linked crate declares a GPL, LGPL, AGPL, MPL, EPL, CDDL, EUPL, OSL, or CC-BY-SA license.\n\n")
        if missing:
            f.write("## Crates without a bundled license file\n\n")
            f.write("Their declared license applies; canonical license texts are provided in `texts/_canonical/`.\n\n")
            for name, ver, lic, authors in missing:
                f.write(f"- {name} {ver}: {lic}; copyright its authors: {authors}\n")
            f.write("\n")
        f.write("## Every crate\n\n| crate | version | license | texts | in | source |\n|---|---|---|---|---|---|\n")
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
