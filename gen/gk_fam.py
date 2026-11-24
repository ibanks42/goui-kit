"""gk_fam: component families an app opts into.

Every upstream module of the kit (gpui_base::accordion, gpui_component::chart, ...) is one family.
The generated dispatchers of the library (`node`, `call`, `paint_part`, ...) used to be one `match`
each over every element type and call op, so every component was reachable from the library's entry
points and a linker had to keep all of them. Here each dispatcher is split into one function per
family, and the public dispatcher finds the family of a key in a generated table (data, not code) and
calls the family's function through a registry the app fills at start-up:

    gk_use_component_chart()      registers the chart family (one exported function per family)
    gk_use_all()                  registers every family (the shared library's bindings call it)

A program linked against the static library keeps only the families whose `gk_use_*` function it
references; the linker drops the rest. The core families (gpui's own elements and anything that is no
upstream module's) are registered on first use. A key whose family is not registered stops the app
with a message that names the family and how to opt in.

Run from gk_rust.Rust.emit on the finished files (`split`), and from gkgen.gen for the Go packages
(`go_packages`).
"""
import os
import re

SHORT = {"gpui_base": "base", "gpui_component": "component"}
CORE = "core"

# Dispatchers split by family: the generated function's name and its key space.
NODE_SPACE, CALL_SPACE = "node", "call"
DISPATCHERS = {"node": NODE_SPACE, "paint_part": NODE_SPACE, "value_action": NODE_SPACE,
               "motion_wrap": NODE_SPACE, "motion_trap": NODE_SPACE, "motion_scope": NODE_SPACE,
               "call": CALL_SPACE}
TRAITVAL = re.compile(r"traitval_[a-z0-9_]+$")

ARM = re.compile(r"^(\s+)((?:#\[cfg\([^\]]*\)\]\s*)?)(\d+(?:\s*\|\s*\d+)*)\s*=>")


class Families:
    def __init__(self, m, call_items):
        self.m = m
        self.byname = {}  # (crate, type name) -> {module of its definition}
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
        for it in call_items:  # the call ops are their own numbering; only the call dispatcher's arms
            self.call_fam[it["op"]] = self.of_item(it)

    @staticmethod
    def name(crate, module):
        if crate not in SHORT:
            return CORE
        return f"{SHORT[crate]}/{module or 'root'}"

    def of_type(self, crate, name, pubpath):
        # A kit type behind a shared pointer (Rc<gpui_kit::component::ThemeConfig>): the type's family.
        inner = re.search(r"<(gpui_kit::[A-Za-z0-9_:]+)", f"{pubpath} {name}")
        if inner:
            p = inner.group(1)
            return self.of_type(crate, p.split("::")[-1], p)
        mods = self.byname.get((crate, name))
        if mods and len(mods) == 1:
            return self.name(crate, next(iter(mods)))
        segs = pubpath.split("::")
        # gpui_kit::component::chart::LineChart -> chart; gpui_kit::component::Button -> root
        return self.name(crate, segs[2] if len(segs) > 3 else "")

    def of_node(self, n):
        c = self.m.api.crates.get(n["crate"])
        p = c.paths.get(str(n["id"])) if c else None
        if p and p.get("crate_id") == 0:
            segs = p["path"]
            return self.name(n["crate"], segs[1] if len(segs) > 2 else "")
        return self.of_type(n["crate"], n["name"], n["path"])

    def of_item(self, it):
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
        # A call on a shared pointer to a kit type (Rc<gpui_kit::component::Theme>::...): the type's family.
        inner = re.search(r"\b(?:Rc|Arc|Box)<(gpui_kit::[A-Za-z0-9_:]+)", path)
        if inner:
            owner = m.states.get(inner.group(1)) or getattr(m, "objects", {}).get(inner.group(1)) or m.nodes.get(inner.group(1))
            if owner:
                return self.of_type(owner["crate"], owner["name"], owner["path"])
            return self.of_type(crate, inner.group(1).split("::")[-1], inner.group(1))
        path = re.sub(r"<.*", "", path)
        if crate in SHORT and path.startswith("gpui_kit::"):
            segs = path.split("::")
            # a free function: gpui_kit::base::input::highlight_prefix
            if len(segs) > 3 and segs[2][:1].islower():
                return self.name(crate, segs[2])
            if len(segs) > 3:
                return self.of_type(crate, segs[2], "::".join(segs[:3]))
            return self.name(crate, "")
        return CORE

    # ------------------------------------------------------------------ the Rust side

    def split(self, files, where):
        """Rewrites the dispatchers of mod.rs and calls.rs in `files` (name -> text), appends the family
        registry to mod.rs and moves each family's code into f_<family>.rs. `where` (file -> {line:
        item key}, gkgen.py's map from a compiler error to the item it is in) follows every line."""
        lines = {f: [_ln(t, where.get(f, {}).get(str(n + 1))) for n, t in enumerate(text.split("\n"))]
                 for f, text in files.items() if f in ("calls.rs", "mod.rs") or f.startswith("n_")}
        fams = {CORE: set()}
        sigs = {}  # dispatcher -> (params, ret)
        # The family of every key a dispatcher has an arm for: some arms come from other parts of the
        # generator than the model's constructors (the editor host, the lists family, ...).
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

    # Each family's generated code in a module of its own (f_<family>.rs), beside its registration:
    # what a family links is in one place. (Leaving families out works at the level of functions,
    # with --gc-sections: rustc puts generic code instantiated for a family into the codegen units
    # of the shared runtime, so the objects of the static library do not line up with families.)
    ITEM = re.compile(r"^(?:pub(?:\(crate\))? )?(?:extern \"C\" )?(?:fn|static) ([A-Za-z0-9_]+)")

    def _family_of_name(self, name):
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

    def _move(self, files):  # file -> lines (_ln)
        m = self.m
        self.node_by_fn = {n["fn"]: self.of_node(n) for n in m.nodes.values() if "fn" in n}
        self.state_by_fn = {}
        for st in list(m.states.values()) + list(getattr(m, "objects", {}).values()):
            if "fn" in st:
                self.state_by_fn[st["fn"]] = self.of_type(st["crate"], st["name"], st["path"])
        self.by_ident = {_ident(f): f for f in self.order}
        moved = {}  # family -> lines
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
                        item[0] = _ln("pub " + item[0], item[0].key)  # its family's calls reach it from their own module
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
            files[f + ".rs"] = [_ln(t) for t in [f"// Generated by gen/gk_fam.py. Do not edit. The {fam} family: its decoders, calls and registration."]
                                + head] + moved[fam] + [_ln("")]
            mods.append(f)
        files["f_all.rs"] = [_ln(t) for t in ["// Generated by gen/gk_fam.py. Do not edit. gk_use_all, in a module of its own: it reaches every family."]
                             + head] + allfn + [_ln("")]
        mods.append("f_all")
        at = files["mod.rs"].index("mod codecs;")
        files["mod.rs"][at:at] = [_ln(t) for f in mods for t in (f"mod {f};", f"pub use {f}::*;")]

    def _split_file(self, lines, fams, sigs):  # lines (_ln) -> lines
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
            # The family functions: the dispatcher's own body with only that family's arms.
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
        nk, ck = self.keys[NODE_SPACE], self.keys[CALL_SPACE]
        node_keys = [0] * (max(nk, default=0) + 1)
        for op, f in nk.items():
            node_keys[op] = ids.get(f, ids[CORE])
        call_keys = [0] * (max(ck, default=0) + 1)
        for op, f in ck.items():
            call_keys[op] = ids.get(f, ids[CORE])
        L = ["", "// ---- Families (gen/gk_fam.py): what an app opts into ----------------------------------", "",
             "/// One family's dispatchers; None where the family has no key of that dispatcher.",
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
                L.append(f"/// Links and registers the {f} family (the app calls it once, before gk_run).")
                L.append("#[unsafe(no_mangle)]")
                L.append(f"pub extern \"C\" fn gk_use_{_ident(f)}() {{ fam_register({n}, &FAM_{n}); }}")
        L += ["",
              "/// Registers every family: for bindings that load the shared library, which holds all of them.",
              "#[unsafe(no_mangle)]", "pub extern \"C\" fn gk_use_all() {"]
        for f in order:
            L.append(f"    fam_register({ids[f]}, &FAM_{ids[f]});")
        L += ["}", "",
              f"const NFAM: usize = {len(order) + 1};",
              "/// Each family's name, by its number (for the message about a family not linked in).",
              "static FAM_NAMES: [&str; NFAM] = [\"\", " + ", ".join(f'"{f}"' for f in order) + "];",
              "static FAM_REG: [std::sync::atomic::AtomicPtr<Fam>; NFAM] = [const { std::sync::atomic::AtomicPtr::new(std::ptr::null_mut()) }; NFAM];",
              "/// The family of each element (node) type number; 0: not a generated type.",
              f"pub(crate) static KEYS_NODE: [u8; {len(node_keys)}] = [{', '.join(map(str, node_keys))}];",
              "/// The family of each generated call op; 0: not a generated call.",
              f"pub(crate) static KEYS_CALL: [u8; {len(call_keys)}] = [{', '.join(map(str, call_keys))}];",
              "",
              "pub(crate) fn fam_register(n: usize, f: &'static Fam) {",
              "    FAM_REG[n].store(f as *const Fam as *mut Fam, std::sync::atomic::Ordering::Release);",
              "}",
              "",
              "/// The registered family of key `k`; None when `k` is no generated key. The core family is",
              "/// registered on first use; any other family must have been registered by the app.",
              "pub(crate) fn fam_get(keys: &[u8], k: u16) -> Option<&'static Fam> {",
              "    let n = *keys.get(k as usize)? as usize;",
              "    if n == 0 {",
              "        return None;",
              "    }",
              "    let p = FAM_REG[n].load(std::sync::atomic::Ordering::Acquire);",
              "    if !p.is_null() {",
              "        // SAFETY: only pointers to the static Fam values are stored.",
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

    # ------------------------------------------------------------------ the Go side

    GO_OP = re.compile(r"\b(newNode|newValue|newReq)\((\d+)\)")

    def go_link(self, go_out):
        """Links each family into a Go program only when the program calls into it.

        useX() takes the address of the family's registration function, gk_use_<family>, and calls
        it through one shared C helper. Taking a C function's address makes cgo refer to the symbol
        from Go's own object, not from a C stub, and the Go linker leaves out what no live function
        reaches: so the reference, and through it the family's code in the static library, is in the
        program only when a live Go function calls useX(). The C linker then drops the families
        nothing refers to (--gc-sections; -dead_strip on macOS). A C stub per family would not do:
        on Windows a linker keeps every plain section of an object, so every stub, and every family,
        would stay. Each generated function calls the useX() of every family whose element type
        (newNode, newValue) or call op (newReq) it writes."""
        fn = {f: "use" + "".join(w.capitalize() for w in _ident(f).split("_")) for f in self.order if f != CORE}
        L = ["// Code generated by gen/gk_fam.py. DO NOT EDIT.", "",
             "package gk", "", "/*"]
        L += [f"extern void gk_use_{_ident(f)}(void);" for f in fn]
        L += ["static void gk_go_use(void *f) { ((void (*)(void))f)(); }", "*/",
              "import \"C\"", "", "import (", "\t\"sync/atomic\"", "\t\"unsafe\"", ")", "",
              "// The component families a program has linked in (docs/GO.md section 8).", "var ("]
        L += [f"\t{name}Done atomic.Bool" for name in fn.values()]
        L += [")", ""]
        for f, name in fn.items():
            L += [f"// {name} links GPUI-kit's {f} family into the program and registers it with the library.",
                  f"func {name}() {{",
                  f"\tif !{name}Done.Load() {{",
                  f"\t\t{name}Done.Store(true)",
                  f"\t\tC.gk_go_use(unsafe.Pointer(C.gk_use_{_ident(f)}))",
                  "\t}", "}", ""]
        _write(os.path.join(go_out, "gen_use.go"), "\n".join(L))
        for old in os.listdir(go_out):  # the file-per-family stubs of an earlier design
            if old.startswith("use_") and old.endswith(".go"):
                os.remove(os.path.join(go_out, old))
        old_pkgs = os.path.join(go_out, "use")  # the import-per-family packages of an earlier design
        if os.path.isdir(old_pkgs):
            import shutil
            shutil.rmtree(old_pkgs)
        for name in sorted(os.listdir(go_out)):
            if name.startswith("gen_") and name.endswith(".go") and name != "gen_use.go":
                path = os.path.join(go_out, name)
                _write(path, self._go_inject(open(path).read(), fn))

    def _go_inject(self, text, fn):
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
    """A generated line and the item it was written for (gk_rust.Rust.where), kept as lines move."""


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
