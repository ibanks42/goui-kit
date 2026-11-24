#!/usr/bin/env python3
"""Scan pinned gpui-base and gpui-component crate sources for Clone structs.

Usage:
    python3 tools/clone_scan.py CARGO_REGISTRY_SRC_DIR > api/clone.json

Rustdoc JSON omits blanket/derived Clone details after stripping non-local trait
impls, so this script scans the pinned source trees (matching versions in
Cargo.lock) for `#[derive(...Clone...)] pub struct` and `impl Clone for` items.
"""
import json
import os
import re
import sys

LOCK = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "Cargo.lock")


def locked(name):
    """Read the exact pinned version of crate `name` from Cargo.lock."""
    found = re.findall(r'name = "%s"\nversion = "([^"]+)"' % re.escape(name), open(LOCK).read())
    if len(found) != 1:
        raise SystemExit(f"clone_scan: Cargo.lock has {len(found)} versions of {name}")
    return found[0]


CRATES = {"gpui_base": "gpui-base", "gpui_component": "gpui-component"}
DERIVE = re.compile(r"#\[derive\(([^\]]*)\)\]\s*(?:#\[[^\]]*\]\s*)*pub(?:\([^)]*\))?\s+struct\s+(\w+)", re.S)
IMPL = re.compile(r"impl(?:<[^>]*>)?\s+(?:(?:std|core)::clone::)?Clone\s+for\s+(\w+)")


def scan(root):
    """Walk src/ under each pinned crate in root and return sorted Clone struct names."""
    out = {}
    for crate, pkg in CRATES.items():
        folder = f"{pkg}-{locked(pkg)}"
        names = set()
        for d, _, files in os.walk(os.path.join(root, folder, "src")):
            for f in files:
                if not f.endswith(".rs"):
                    continue
                text = open(os.path.join(d, f), encoding="utf-8", errors="replace").read()
                for m in DERIVE.finditer(text):
                    if re.search(r"\bClone\b", m.group(1)):
                        names.add(m.group(2))
                names.update(IMPL.findall(text))
        out[crate] = sorted(names)
    return out


if __name__ == "__main__":
    res = scan(sys.argv[1])
    json.dump({"note": "structs that implement Clone, read from the pinned sources by tools/clone_scan.py",
               "pin": {"gpui-kit": locked("gpui-kit")}, "clone": res}, sys.stdout, indent=1)
    sys.stdout.write("\n")
