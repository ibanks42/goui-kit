#!/usr/bin/env python3
"""clone_scan.py SRC_ROOT > api/clone.json: which structs of the kit crates implement Clone.

SRC_ROOT is the cargo registry's source folder (~/.cargo/registry/src/index.crates.io-*). Run it
after every change of the pinned gpui-kit version (docs/CONTRIBUTING.md): the versions are read
from Cargo.lock.

The reduced extract in api/rustdoc drops std-trait impls (gen/gkgen.py strip), so the generator
cannot tell which library objects can be copied (docs/PROTOCOLS.md section 13: an object handed
out of a borrow is copied into a new handle). This reads the pinned crates' sources (in the
build folder's cargo registry) and lists every struct with `#[derive(.. Clone ..)]` or an
`impl Clone for` / `impl<..> Clone for`. Read-only; standard library only. A wrong entry is
caught by rustc (the generated copy does not compile and is quarantined)."""
import json
import os
import re
import sys

LOCK = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "Cargo.lock")


def locked(name):
    """The version of a crate in Cargo.lock (exactly one)."""
    found = re.findall(r'name = "%s"\nversion = "([^"]+)"' % re.escape(name), open(LOCK).read())
    if len(found) != 1:
        raise SystemExit(f"clone_scan: Cargo.lock has {len(found)} versions of {name}")
    return found[0]


CRATES = {"gpui_base": "gpui-base", "gpui_component": "gpui-component"}
DERIVE = re.compile(r"#\[derive\(([^\]]*)\)\]\s*(?:#\[[^\]]*\]\s*)*pub(?:\([^)]*\))?\s+struct\s+(\w+)", re.S)
IMPL = re.compile(r"impl(?:<[^>]*>)?\s+(?:(?:std|core)::clone::)?Clone\s+for\s+(\w+)")


def scan(root):
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
