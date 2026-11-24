"""Partition generated Rust decoders into link-on-demand component families and emit Go registration hooks.

Why this exists (dead-stripping static binaries):
If `core/src/gen/mod.rs` dispatched directly to every generated widget decoder inside a
single giant `match` statement, then linking `libgouikit_static.a` into a minimal Go
program would pull in every upstream GPUI-kit component (editor, charts, dock, markdown,
tables, etc.).

Instead, this module breaks the static call graph at the dispatcher boundary:
1. **Family assignment (`Families.__init__`, `of_type`, `of_node`, `of_item`)**:
   Maps every node opcode (`NODE_SPACE`) and call opcode (`CALL_SPACE`) to a family
   key: `"base/<mod>"`, `"component/<mod>"`, or `"core"` (for core GPUI / uncategorized
   items that are always linked).
2. **Dispatcher splitting (`_split_file`)**:
   Rewrites each top-level dispatcher (`node`, `call`, `paint_part`, `value_action`,
   `motion_wrap`, `motion_trap`, `motion_scope`, and `traitval_*`) into per-family
   functions (`<disp>__<fam_ident>`) and replaces the top-level dispatcher body with a
   table lookup `fam_get(&KEYS_<SPACE>, key)`.
3. **Registry table (`_registry`)**:
   Emits the `Fam` vtable struct, one `static FAM_<id>: Fam` per family, `KEYS_NODE` /
   `KEYS_CALL` (`[u8; N]` opcode -> family ID tables), an atomic pointer registry
   `FAM_REG`, one `#[unsafe(no_mangle)] pub extern "C" fn gk_use_<fam>()` per non-core
   family, and `gk_use_all()` (which registers every family for dynamic builds or
   `gk.LinkAll()`).
4. **Module partitioning (`_move`)**:
   Moves every non-core decoder (`make_*`, `build_*`, `apply_*`, `c_*`, `sb_*`, `sbp_*`),
   per-family dispatcher helper, `FAM_<id>`, and `gk_use_<fam>` into its own Rust module
   `core/src/gen/f_<fam_ident>.rs`, and isolates `gk_use_all` in `f_all.rs`. With
   `-ffunction-sections` / `--gc-sections` (or `-dead_strip`), any family whose
   `gk_use_<fam>` symbol is never referenced by the Go binary is dead-stripped wholesale.
5. **Go registration hooks (`go_link`, `_go_inject`)**:
   Generates `go/gk/internal/rt/gen_use.go` (`//go:build gkstatic`, calling
   `C.gk_use_<fam>` once via `atomic.Bool`) and `gen_use_dyn.go` (`!gkstatic`, no-ops),
   then injects the corresponding `use<Fam>()` call at the top of every generated Go
   function whose body emits a non-core opcode (`newNode`, `newValue`, `newReq`).

Invariants checked during splitting:
- Every opcode in `NODE_SPACE` or `CALL_SPACE` must map to a single family across all
  dispatchers (mismatch raises `SystemExit`).
- Total family count must fit in `u8` (<= 254, reserving 0 for "not a generated opcode").
- Source-location maps (`where`) are preserved line-for-line through `_Line` subclasses
  so `gk_rust.py` diagnostics still point to the exact moved line in `f_<fam>.rs`.
"""
import os
import re

SHORT = {"gpui_base": "base", "gpui_component": "component"}
CORE = "core"

NODE_SPACE, CALL_SPACE = "node", "call"
# Dispatcher functions split into per-family arms and routed through the Fam registry table.
# Keyed by function name -> opcode space (`NODE_SPACE` for node type IDs, `CALL_SPACE` for call opcodes).
DISPATCHERS = {"node": NODE_SPACE, "paint_part": NODE_SPACE, "value_action": NODE_SPACE,
               "motion_wrap": NODE_SPACE, "motion_trap": NODE_SPACE, "motion_scope": NODE_SPACE,
               "call": CALL_SPACE}
# Dynamically generated trait-value dispatchers (e.g. `traitval_any_setting_field`) also key off NODE_SPACE.
TRAITVAL = re.compile(r"traitval_[a-z0-9_]+$")

# Matches one opcode match arm inside a generated dispatcher (optionally preceded by a `#[cfg(...)]` attribute).
ARM = re.compile(r"^(\s+)((?:#\[cfg\([^\]]*\)\]\s*)?)(\d+(?:\s*\|\s*\d+)*)\s*=>")


class Families:
    """Maps opcodes to upstream families, splits Rust modules, and emits Go link hooks.

    Key attributes populated across stages:
    - `byname`: `(crate, type_name) -> set(defining_module)` from rustdoc `paths`.
    - `node_fam`, `call_fam`: `opcode -> family_key` computed at init.
    - `keys`: `{NODE_SPACE: {op: fam}, CALL_SPACE: {op: fam}}` verified during `_split_file`.
    - `order`, `ids`: deterministic sorted list of families (with `"core"` first at ID 1).
    """

    def __init__(self, m, call_items):
        """Index defining modules from rustdoc JSON and precompute `node_fam` and `call_fam` opcode maps."""
        self.m = m
        self.byname = {}
        for crate, c in m.api.crates.items():
            for i, p in c.paths.items():
                if p.get("crate_id") != 0 or p.get("kind") not in ("struct", "enum", "union", "trait", "type_alias"):
                    continue
                segs = p["path"]
                self.byname.setdefault((crate, segs[-1]), set()).add(segs[1] if len(segs) > 2 else "")
        self.node_fam, self.call_fam = {}, {}
        for key, n in m.nodes.items():
            f = self.of_node(n)
            for it in n["ctors"]:
                if it.get("op") is not None:
                    self.node_fam[it["op"]] = f
        for it in call_items:
            self.call_fam[it["op"]] = self.of_item(it)

    @staticmethod
    def name(crate, module):
        """Format a family key (`base/<mod>` or `component/<mod>`), or `core` for core/gpui items."""
        if crate not in SHORT:
            return CORE
        return f"{SHORT[crate]}/{module or 'root'}"

    def of_type(self, crate, name, pubpath):
        """Determine the family of a type from its rustdoc defining module or public path.

        Generic wrappers like `Scrollable<gpui_kit::Div>` or `Rc<gpui_kit::...>` inherit
        the family of their inner `gpui_kit::` type argument. Otherwise, if a type name
        is uniquely defined in one module of `crate`, that defining module wins over a
        re-export path.
        """
        inner = re.search(r"<(gpui_kit::[A-Za-z0-9_:]+)", f"{pubpath} {name}")
        if inner:
            p = inner.group(1)
            return self.of_type(crate, p.split("::")[-1], p)
        mods = self.byname.get((crate, name))
        if mods and len(mods) == 1:
            return self.name(crate, next(iter(mods)))
        segs = pubpath.split("::")
        return self.name(crate, segs[2] if len(segs) > 3 else "")

    def of_node(self, n):
        """Determine the family of a node from its rustdoc defining path (falling back to `of_type`)."""
        c = self.m.api.crates.get(n["crate"])
        p = c.paths.get(str(n["id"])) if c else None
        if p and p.get("crate_id") == 0:
            segs = p["path"]
            return self.name(n["crate"], segs[1] if len(segs) > 2 else "")
        return self.of_type(n["crate"], n["name"], n["path"])

    def of_item(self, it):
        """Determine the family of a call or ambient item from its owning node, state, smart pointer, or path."""
        m = self.m
        if it.get("node") in m.nodes:
            return self.of_node(m.nodes[it["node"]])
        owner = it.get("owner")
        if owner in m.nodes:
            return self.of_node(m.nodes[owner])
        st = m.states.get(owner) or getattr(m, "objects", {}).get(owner)
        if st:
            return self.of_type(st["crate"], st["name"], st["path"])
        crate = it.get("crate")
        path = it.get("id", "")
        # Pointer-wrapped object calls (Rc<T>/Arc<T>/Box<T>) follow the pointed-to type's family.
        inner = re.search(r"\b(?:Rc|Arc|Box)<(gpui_kit::[A-Za-z0-9_:]+)", path)
        if inner:
            owner = m.states.get(inner.group(1)) or getattr(m, "objects", {}).get(inner.group(1)) or m.nodes.get(inner.group(1))
            if owner:
                return self.of_type(owner["crate"], owner["name"], owner["path"])
            return self.of_type(crate, inner.group(1).split("::")[-1], inner.group(1))
        path = re.sub(r"<.*", "", path)
        if crate in SHORT and path.startswith("gpui_kit::"):
            segs = path.split("::")
            if len(segs) > 3 and segs[2][:1].islower():
                return self.name(crate, segs[2])
            if len(segs) > 3:
                return self.of_type(crate, segs[2], "::".join(segs[:3]))
            return self.name(crate, "")
        return CORE


    def split(self, files, where):
        """Split dispatcher match arms by family, append the registry to mod.rs, and move items into f_*.rs.

        Mutates `files` and `where` in place, returning `files`. `where` maps
        `filename -> {1-based-line-str: item_key}` and is rebuilt after moving functions
        between files so error-reporting maps remain accurate.
        """
        lines = {f: [_ln(t, where.get(f, {}).get(str(n + 1))) for n, t in enumerate(text.split("\n"))]
                 for f, text in files.items() if f in ("calls.rs", "mod.rs") or f.startswith("n_")}
        fams = {CORE: set()}
        sigs = {}
        self.keys = {NODE_SPACE: {}, CALL_SPACE: {}}
        for fname in ("calls.rs", "mod.rs"):
            lines[fname] = self._split_file(lines[fname], fams, sigs)
        order = sorted(fams, key=lambda f: (f != CORE, f))
        ids = {f: n + 1 for n, f in enumerate(order)}
        if len(ids) > 254:
            raise SystemExit(f"gk_fam: {len(ids)} families do not fit the u8 key tables")
        self.ids, self.order = ids, order
        lines["mod.rs"] += [_ln(t) for t in self._registry(fams, sigs, ids, order).split("\n")]
        self._move(lines)
        for f, ls in lines.items():
            files[f] = "\n".join(ls)
            where[f] = {str(n + 1): x.key for n, x in enumerate(ls) if x.key is not None}
        return files

    ITEM = re.compile(r"^(?:pub(?:\(crate\))? )?(?:extern \"C\" )?(?:fn|static) ([A-Za-z0-9_]+)")

    def _family_of_name(self, name):
        """Map a generated Rust function or static symbol name to its owning component family.

        Matches the exact naming conventions emitted by `gk_rust.py` and `_split_file`:
        - `c_<op>` -> call op family
        - `sb_<stem>` / `sbp_<stem>` -> state builder family
        - `make_<fn>` / `build_<fn>` / `apply_<fn>` -> node decoder family
        - `<disp>__<fam_ident>` -> split dispatcher arm family
        - `FAM_<id>` / `gk_use_<fam_ident>` -> family registration static/hook
        """
        mt = re.match(r"c_(\d+)$", name)
        if mt:
            return self.call_fam.get(int(mt.group(1)), CORE)
        mt = re.match(r"(?:sb|sbp)_(.+)$", name)
        if mt:
            return self.state_by_fn.get(mt.group(1), CORE)
        mt = re.match(r"(?:make|build|apply)_(.+)$", name)
        if mt:
            return self.node_by_fn.get(mt.group(1), CORE)
        mt = re.match(r"[a-z_]+?__([a-z0-9_]+)$", name)
        if mt:
            return self.by_ident.get(mt.group(1), CORE)
        mt = re.match(r"FAM_(\d+)$", name)
        if mt:
            return self.order[int(mt.group(1)) - 1]
        mt = re.match(r"gk_use_([a-z0-9_]+)$", name)
        if mt and mt.group(1) != "all":
            return self.by_ident.get(mt.group(1), CORE)
        return CORE

    def _move(self, files):
        """Partition non-core functions and `FAM_*` statics into per-family `f_<ident>.rs` modules plus `f_all.rs`.

        Scans top-level brace-delimited items (preserving attached `///` doc comments and
        `#[...]` attributes in `pend`), promotes private moved `fn` items to `pub fn` so
        sibling family modules can call cross-family helpers via `use super::*;`, and wires
        `mod f_*; pub use f_*::*;` into `mod.rs` ahead of `mod codecs;`.
        """
        m = self.m
        self.node_by_fn = {n["fn"]: self.of_node(n) for n in m.nodes.values() if "fn" in n}
        self.state_by_fn = {}
        for st in list(m.states.values()) + list(getattr(m, "objects", {}).values()):
            if "fn" in st:
                self.state_by_fn[st["fn"]] = self.of_type(st["crate"], st["name"], st["path"])
        self.by_ident = {_ident(f): f for f in self.order}
        moved = {}
        allfn = []
        for fname in sorted(files):
            if not (fname in ("calls.rs", "mod.rs") or fname.startswith("n_")):
                continue
            lines = files[fname]
            keep, pend, i = [], [], 0
            while i < len(lines):
                line = lines[i]
                if line.startswith("#[") or line.startswith("///"):
                    pend.append(line)
                    i += 1
                    continue
                mt = self.ITEM.match(line) if line[:1].isalpha() else None
                if mt and line.rstrip().endswith("{"):
                    j = i
                    while lines[j] != "}":
                        j += 1
                    item = lines[i:j + 1]
                    i = j + 1
                else:
                    item = [line]
                    i += 1
                name = mt.group(1) if mt else None
                fam = self._family_of_name(name) if name else CORE
                if fam not in self.ids:
                    fam = CORE
                if name == "gk_use_all":
                    allfn += pend + item
                elif fam != CORE:
                    if item[0].startswith("fn "):
                        item[0] = _ln("pub " + item[0], item[0].key)
                    moved.setdefault(fam, []).extend(pend + item)
                else:
                    if fname == "calls.rs" and item[0].startswith("fn "):
                        item[0] = _ln("pub " + item[0], item[0].key)
                    keep += pend + item
                pend = []
            keep += pend
            files[fname] = keep
        head = ["#![allow(unused_variables, unused_imports, unused_mut, unreachable_patterns, unused_parens, dead_code)]",
                "#![allow(clippy::all)]", "use crate::genapi::{self, Dx, R};", "use super::*;",
                "use gpui_kit::AppContext as _;", "use gpui_kit::IntoElement as _;", ""]
        mods = []
        for fam in sorted(moved):
            f = f"f_{_ident(fam)}"
            files[f + ".rs"] = [_ln(t) for t in [f"// Code generated by gen/gk_fam.py. DO NOT EDIT. Decoders, calls, and registration for family {fam}."]
                                + head] + moved[fam] + [_ln("")]
            mods.append(f)
        files["f_all.rs"] = [_ln(t) for t in ["// Code generated by gen/gk_fam.py. DO NOT EDIT. Registers every generated component family via gk_use_all."]
                             + head] + allfn + [_ln("")]
        mods.append("f_all")
        at = files["mod.rs"].index("mod codecs;")
        files["mod.rs"][at:at] = [_ln(t) for f in mods for t in (f"mod {f};", f"pub use {f}::*;")]

    def _split_file(self, lines, fams, sigs):
        """Split each dispatcher function in `lines` into per-family helpers and a `fam_get` trampoline."""
        out, i = [], 0
        while i < len(lines):
            line = lines[i]
            mt = re.match(r"^pub fn ([a-z0-9_]+)\((.*)\) -> (.*) \{$", line)
            name = mt and mt.group(1)
            if not mt or not (name in DISPATCHERS or TRAITVAL.match(name)):
                out.append(line)
                i += 1
                continue
            space = DISPATCHERS.get(name, NODE_SPACE)
            j = i
            while lines[j] != "}":
                j += 1
            body = lines[i + 1:j]
            params, ret = mt.group(2), mt.group(3)
            sigs[name] = (params, ret)
            table = self.call_fam if space == CALL_SPACE else self.node_fam
            byfam, shared = {}, []
            for b in body:
                a = ARM.match(b)
                if not a:
                    shared.append((len(shared), b))
                    continue
                nums = [int(x) for x in re.findall(r"\d+", a.group(3))]
                f = next((table[x] for x in nums if x in table), CORE)
                seen = self.keys[space]
                for x in nums:
                    if seen.setdefault(x, f) != f:
                        raise SystemExit(f"gk_fam: {space} key {x} is in family {seen[x]} in one dispatcher and {f} in {name}")
                byfam.setdefault(f, []).append(b)
            for f, arms in sorted(byfam.items()):
                fams.setdefault(f, set()).add(name)
                out.append(_ln(f"pub fn {name}__{_ident(f)}({params}) -> {ret} {{"))
                placed = False
                for b in body:
                    if ARM.match(b):
                        if not placed:
                            out += arms
                            placed = True
                        continue
                    out.append(b)
                out += [_ln("}"), _ln("")]
            names = _param_names(params)
            key = {"ty": "ty", "op": "op"}.get(next((p for p in names if p in ("ty", "op")), ""), "dx.node(n).0")
            out.append(line)
            out.append(_ln(f"    let f = fam_get(&KEYS_{space.upper()}, {key})?;"))
            out.append(_ln(f"    (f.{name}?)({', '.join(names)})"))
            out.append(_ln("}"))
            i = j + 1
        return out

    def _registry(self, fams, sigs, ids, order):
        """Generate the Fam vtable, opcode-to-family lookup tables, and extern C registration functions."""
        nk, ck = self.keys[NODE_SPACE], self.keys[CALL_SPACE]
        node_keys = [0] * (max(nk, default=0) + 1)
        for op, f in nk.items():
            node_keys[op] = ids.get(f, ids[CORE])
        call_keys = [0] * (max(ck, default=0) + 1)
        for op, f in ck.items():
            call_keys[op] = ids.get(f, ids[CORE])
        L = ["", "// Component family registry generated by gen/gk_fam.py.", "",
             "/// Dispatch function table for a single component family; entries are None if unused.",
             "pub struct Fam {", "    pub name: &'static str,"]
        for d in sorted(sigs):
            p, r = sigs[d]
            L.append(f"    pub {d}: Option<fn({', '.join(_param_types(p))}) -> {r}>,")
        L += ["}", ""]
        for f in order:
            n = ids[f]
            have = fams.get(f) or set()
            fields = "".join(f" {d}: {f'Some({d}__{_ident(f)})' if d in have else 'None'}," for d in sorted(sigs))
            L.append(f'pub static FAM_{n}: Fam = Fam {{ name: "{f}",{fields} }};')
            if f != CORE:
                L.append(f"/// Register the {f} component family (called once before gk_run in static builds).")
                L.append("#[unsafe(no_mangle)]")
                L.append(f"pub extern \"C\" fn gk_use_{_ident(f)}() {{ fam_register({n}, &FAM_{n}); }}")
        L += ["",
              "/// Register every component family at once (used with the shared library or gk.LinkAll).",
              "#[unsafe(no_mangle)]", "pub extern \"C\" fn gk_use_all() {"]
        for f in order:
            L.append(f"    fam_register({ids[f]}, &FAM_{ids[f]});")
        L += ["}", "",
              f"const NFAM: usize = {len(order) + 1};",
              "/// Family display names indexed by family ID for missing-family panic diagnostics.",
              "static FAM_NAMES: [&str; NFAM] = [\"\", " + ", ".join(f'"{f}"' for f in order) + "];",
              "static FAM_REG: [std::sync::atomic::AtomicPtr<Fam>; NFAM] = [const { std::sync::atomic::AtomicPtr::new(std::ptr::null_mut()) }; NFAM];",
              "/// Maps each node type opcode to its family ID (0 means not a generated node).",
              f"pub(crate) static KEYS_NODE: [u8; {len(node_keys)}] = [{', '.join(map(str, node_keys))}];",
              "/// Maps each call opcode to its family ID (0 means not a generated call).",
              f"pub(crate) static KEYS_CALL: [u8; {len(call_keys)}] = [{', '.join(map(str, call_keys))}];",
              "",
              "pub(crate) fn fam_register(n: usize, f: &'static Fam) {",
              "    FAM_REG[n].store(f as *const Fam as *mut Fam, std::sync::atomic::Ordering::Release);",
              "}",
              "",
              "/// Look up the registered Fam table for opcode `k`, auto-registering the core family on first use.",
              "pub(crate) fn fam_get(keys: &[u8], k: u16) -> Option<&'static Fam> {",
              "    let n = *keys.get(k as usize)? as usize;",
              "    if n == 0 {",
              "        return None;",
              "    }",
              "    let p = FAM_REG[n].load(std::sync::atomic::Ordering::Acquire);",
              "    if !p.is_null() {",
              "        // SAFETY: FAM_REG only ever stores pointers to 'static Fam items.",
              "        return Some(unsafe { &*p });",
              "    }",
              f"    if n == {ids[CORE]} {{",
              f"        fam_register(n, &FAM_{ids[CORE]});",
              f"        return Some(&FAM_{ids[CORE]});",
              "    }",
              "    let f = FAM_NAMES[n];",
              "    panic!(\"GPUI-kit's {f} family is used but not linked into this program. This is a bug in the \\",
              "            bindings (the call that uses it should link it): please report it. Until then, Go: call \\",
              "            gk.LinkAll() at start-up\")",
              "}", ""]
        return "\n".join(L)


    GO_OP = re.compile(r"\b(newNode|newValue|newReq)\((\d+)\)")

    def go_link(self, rt_out, files):
        """Emit Go family-registration stubs in internal/rt and inject calls into generated constructors."""
        fn = {f: "use" + "".join(w.capitalize() for w in _ident(f).split("_")) for f in self.order if f != CORE}
        L = ["// Code generated by gen/gk_fam.py. DO NOT EDIT.", "",
             "//go:build gkstatic", "",
             "package rt", "", "/*"]
        L += [f"extern void gk_use_{_ident(f)}(void);" for f in fn]
        L += ["static void gk_go_use(void *f) { ((void (*)(void))f)(); }", "*/",
              "import \"C\"", "", "import (", "\t\"sync/atomic\"", "\t\"unsafe\"", ")", "",
              "// Tracks which component families have already been registered in this process.", "var ("]
        L += [f"\t{name}Done atomic.Bool" for name in fn.values()]
        L += [")", ""]
        for f, name in fn.items():
            U = "U" + name[1:]
            L += [f"// {U} forces linking and registers the {f} component family once.",
                  f"func {U}() {{",
                  f"\tif !{name}Done.Load() {{",
                  f"\t\t{name}Done.Store(true)",
                  f"\t\tC.gk_go_use(unsafe.Pointer(C.gk_use_{_ident(f)}))",
                  "\t}", "}", ""]
        os.makedirs(rt_out, exist_ok=True)
        _write(os.path.join(rt_out, "gen_use.go"), "\n".join(L))
        D = ["// Code generated by gen/gk_fam.py. DO NOT EDIT.", "",
             "//go:build !gkstatic", "",
             "package rt", "",
             "// In dynamic builds the shared library registers all component families at startup.", ""]
        for f, name in fn.items():
            D += [f"// U{name[1:]} is a no-op in dynamic builds because the shared library includes {f}.",
                  f"func U{name[1:]}() {{}}", ""]
        _write(os.path.join(rt_out, "gen_use_dyn.go"), "\n".join(D))
        for name in sorted(files):
            if "/" not in name and name.startswith("gen_") and name.endswith(".go"):
                files[name] = self._go_inject(files[name], fn)
        return {name: "U" + name[1:] for name in fn.values()}

    def _go_inject(self, text, fn):
        """Insert `use<Family>()` registration calls at the top of generated Go functions that emit non-core opcodes."""
        lines = text.split("\n")
        out, i = [], 0
        while i < len(lines):
            line = lines[i]
            if not line.startswith("func ") or not line.rstrip().endswith("{"):
                out.append(line)
                i += 1
                continue
            j = i
            while lines[j] != "}":
                j += 1
            body = lines[i + 1:j]
            fams = []
            for b in body:
                for kind, num in self.GO_OP.findall(b):
                    table = self.keys[CALL_SPACE] if kind == "newReq" else self.keys[NODE_SPACE]
                    f = table.get(int(num), CORE)
                    if f != CORE and f in fn and fn[f] not in fams:
                        fams.append(fn[f])
            out.append(line)
            out += [f"\t{u}()" for u in fams]
            out += body + ["}"]
            i = j + 1
        return "\n".join(out)


class _Line(str):
    pass


def _ln(text, key=None):
    x = _Line(text)
    x.key = key
    return x


def _ident(f):
    return re.sub(r"[^a-z0-9]+", "_", f.lower())


def _split_params(params):
    out, depth, cur = [], 0, ""
    for ch in params:
        if ch in "<([":
            depth += 1
        elif ch in ">)]":
            depth -= 1
        if ch == "," and depth == 0:
            out.append(cur.strip())
            cur = ""
        else:
            cur += ch
    if cur.strip():
        out.append(cur.strip())
    return out


def _param_names(params):
    return [p.split(":", 1)[0].strip() for p in _split_params(params)]


def _param_types(params):
    return [p.split(":", 1)[1].strip() for p in _split_params(params)]


def _write(path, text):
    if not os.path.exists(path) or open(path).read() != text:
        open(path, "w").write(text)
