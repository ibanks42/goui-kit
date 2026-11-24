"""Wire opcode allocator and Rust decoder emitter (`core/src/gen/*.rs`).

This module consumes the classified `Model` produced by `gen/gk_model.py` and runs
two sequential phases:

1. **Stable wire numbering (`Lock`, `number`)**:
   Loads `api/opcodes.lock` (an append-only TSV of `kind`, `scope`, `name`, `number`)
   and assigns immutable numeric IDs to every generated wire entity:
   - Trait tables (`trait` -> `t["num"]`, 1..N) and trait builder ops (`trait-op` ->
     `TRAIT_BASE + (trait_num << 7) + idx`, reserving 128 consecutive opcodes per trait
     starting at `0x4000` so per-trait dispatch uses a single range match `lo..=lo+127`).
   - Inherent node ops (`op`, scoped per node, starting at `OP_BASE = 256`, below `TRAIT_BASE`).
   - Node constructors (`node`, starting at `NODE_BASE = 256`, leaving `1..255` for hand-written nodes).
   - State/object/ambient call ops (`call`, starting at `CALL_BASE = 256`).
   - Style tokens (`style`), palette colors (`palette`), theme colors (`theme`),
     enum variants (`variant`, 0-based per enum), and handler shapes (`shape`, starting at
     `SHAPE_BASE = 64` so low IDs remain reserved for built-in runtime shapes).
   Also assigns deterministic internal Rust helper names (`en_*`, `de_*`, `rc_*`,
   `in_*`, `tr_*`, and unique per-node `fn` stems).

2. **Rust module emission (`Rust.emit`)**:
   Generates:
   - `codecs.rs`: `Unwire` and `Wire` impls for unit enums, data enums, records, and `Into` unions.
   - `style.rs`: `style_tokens`, `palette`, and `theme_color` tables.
   - `traits.rs`: per-trait builder op dispatchers (`tr_<trait>_<n>`).
   - `calls.rs`: synchronous/buffered call op dispatchers (`c_<op>` and top-level `call()`).
   - `n_<crate>_<mod>.rs`: per-module node constructor (`make_*`), value builder (`build_*`),
     and op-loop (`apply_*`) functions.
   - `mod.rs`: top-level `node()`, `style_op()`, `paint_part()`, `value_action()`,
     `traitval_*`, `motion_*`, and module re-exports.
   Finally hands the in-memory `files` map and line-to-item provenance map `where` to
   `gk_fam.Families.split()` so non-core families are moved into `f_<fam>.rs` modules
   for static dead-stripping, then writes `core/src/gen/*.rs` (removing stale files).
"""
import json
import os
import re

import gk_model
from gk_model import INT_WIRE, SHORT, last, snake, walk_desc
import gk_app
import gk_lists
import gk_values
import gk_overlay
import gk_editor
import gk_fam
import gk_tests

# Reserved low opcodes shared with the hand-written Rust runtime (`core/wire/ops.txt`).
# - Node builder ops: 1..255 runtime-reserved (`OP_CHILD`, `OP_CHILD_VIEW`, `OP_TEXT`, `OP_STYLE`);
#   256..0x3FFF inherent node ops (`OP_BASE`); 0x4000..0xFFFF trait ops (`TRAIT_BASE + (trait_num << 7) + idx`).
# - Node constructor type IDs: 1..255 hand-written nodes; >= 256 (`NODE_BASE`) generated nodes.
# - Handler shapes: 1..63 built-in runtime shapes; >= 64 (`SHAPE_BASE`) generated shapes.
# - Call ops: 1..255 hand-written runtime calls; >= 256 (`CALL_BASE`) generated state/object/ambient calls.
OP_CHILD, OP_CHILD_VIEW, OP_TEXT, OP_STYLE = 1, 2, 3, 4
OP_BASE = 256
TRAIT_BASE = 0x4000
NODE_BASE = 256
SHAPE_BASE = 64
CALL_BASE = 256
# ParentElement methods intercepted by runtime tape ops instead of allocating trait opcodes.
RUNTIME_OPS = {"child": OP_CHILD, "children": OP_CHILD}


SUPPORT = '''
// Core GPUI value decoders shared by generated modules.

/// Decode a GPUI Length: u8 kind (0 = px, 1 = rems, 2 = relative fraction, 3 = auto) followed by f32.
pub fn length(r: &mut R) -> gpui_kit::Length {
    match (r.u8(), r.f32()) {
        (3, _) => gpui_kit::Length::Auto,
        (k, v) => gpui_kit::Length::Definite(definite(k, v)),
    }
}

fn definite(kind: u8, v: f32) -> gpui_kit::DefiniteLength {
    match kind {
        1 => gpui_kit::rems(v).into(),
        2 => gpui_kit::relative(v),
        _ => gpui_kit::px(v).into(),
    }
}

pub fn definite_length(r: &mut R) -> gpui_kit::DefiniteLength {
    let k = r.u8();
    definite(k, r.f32())
}

pub fn absolute_length(r: &mut R) -> gpui_kit::AbsoluteLength {
    match (r.u8(), r.f32()) {
        (1, v) => gpui_kit::rems(v).into(),
        (_, v) => gpui_kit::px(v).into(),
    }
}

/// Decode a GPUI ElementId: u8 kind (0 = name, 1 = integer, 2 = name and integer).
pub fn element_id(r: &mut R) -> gpui_kit::ElementId {
    match r.u8() {
        1 => gpui_kit::ElementId::Integer(r.u64()),
        2 => {
            let name = gpui_kit::SharedString::new(r.str());
            gpui_kit::ElementId::NamedInteger(name, r.u64())
        }
        _ => gpui_kit::ElementId::Name(gpui_kit::SharedString::new(r.str())),
    }
}

/// Apply all style ops recorded on a StyleRefinement node onto `style`.
pub fn style_into(dx: &mut Dx, n: u32, style: &mut gpui_kit::StyleRefinement) {
    let mut at = dx.node(n).2;
    while let Some((op, mut r)) = dx.op(&mut at) {
        if !style_op(style, op, &mut r, dx) {
            bad_op("StyleRefinement", op);
        }
    }
}

/// Merge `patch` onto `style` via `Refineable::refine` and return the updated style.
pub fn refined(mut style: gpui_kit::StyleRefinement, patch: &gpui_kit::StyleRefinement) -> gpui_kit::StyleRefinement {
    use gpui_kit::Refineable;
    style.refine(patch);
    style
}

pub fn bad_op(owner: &str, op: u16) {
    eprintln!("gk: op {op} is not an op of {owner}");
}

pub fn bad_node<T>(owner: &str, ty: u16) -> Option<T> {
    eprintln!("gk: node type {ty} does not build {owner}");
    None
}'''


class Lock:
    """Append-only wire-number table persisted to api/opcodes.lock."""

    def __init__(self, path):
        self.path, self.nums, self.next = path, {}, {}
        if os.path.exists(path):
            for line in open(path):
                if line.strip() and not line.startswith("#"):
                    kind, scope, name, n = line.rstrip("\n").split("\t")
                    self.nums[(kind, scope, name)] = int(n)
                    self.next[(kind, scope)] = max(self.next.get((kind, scope), 0), int(n) + 1)

    def num(self, kind, scope, name, first=1):
        """Return the existing wire ID for `(kind, scope, name)` or allocate the next sequential ID."""
        k = (kind, scope, name)
        if k not in self.nums:
            self.nums[k] = self.next.get((kind, scope), first)
            self.next[(kind, scope)] = self.nums[k] + 1
        return self.nums[k]

    def save(self):
        """Persist the sorted append-only lock table to `api/opcodes.lock`."""
        with open(self.path, "w") as f:
            f.write("# opcodes.lock: numbers of generated things on the wire. Written by gen/gkgen.py.\n"
                    "# Append-only: a number is never changed or reused. kind, scope, name, number.\n")
            for (kind, scope, name), n in sorted(self.nums.items(), key=lambda kv: (kv[0][0], kv[0][1], kv[1])):
                f.write(f"{kind}\t{scope}\t{name}\t{n}\n")


def number(m, lock):
    """Assign stable wire opcodes, variant tags, shape IDs, and Rust helper function names.

    Allocates IDs across traits, node ops, node constructors, state/object/ambient calls,
    style tokens, palette colors, theme colors, enum variants, and handler shapes, then
    assigns unique Rust helper identifiers (`en_*`, `de_*`, `rc_*`, `in_*`, `tr_*`, node `fn`).
    """
    for key in sorted(m.traits, key=lambda k: m.traits[k]["key"] or ""):
        t = m.traits[key]
        if not t["ops"]:
            continue
        t["num"] = lock.num("trait", "", t["key"])
        for it in sorted(t["ops"], key=lambda i: i["name"]):
            if t["name"] == "ParentElement" and it["name"] in RUNTIME_OPS and not t["extra"]:
                it["op"], it["runtime_op"] = None, RUNTIME_OPS[it["name"]]
                continue
            idx = lock.num("trait-op", t["key"], it["name"], first=0)
            if idx > 127:
                raise SystemExit(f"gkgen: trait {t['path']} has more than 128 builder methods")
            it["op"] = TRAIT_BASE + (t["num"] << 7) + idx
        if TRAIT_BASE + (t["num"] << 7) + 127 > 0xFFFF:
            raise SystemExit("gkgen: out of trait op numbers")
    for key in sorted(m.nodes):
        node = m.nodes[key]
        for it in sorted(node["ops"], key=lambda i: i["name"]):
            it["op"] = lock.num("op", key, it["name"], first=OP_BASE)
            if it["op"] >= TRAIT_BASE:
                raise SystemExit(f"gkgen: {key} has too many builder methods")
        for it in sorted(node["ctors"], key=lambda i: i["id"]):
            it["op"] = lock.num("node", "", it["id"], first=NODE_BASE)
    for key in sorted(m.states):
        st = m.states[key]
        for it in sorted(st["ctors"] + st["reads"] + st["writes"] + st["calls"] + st["events"]
                         + st.get("globals", []) + st.get("statics", []), key=lambda i: i["id"]):
            it["op"] = lock.num("call", "", it["id"], first=CALL_BASE)
        for it in sorted(st["builders"], key=lambda i: i["name"]):
            it["op"] = lock.num("state-op", key, it["name"])
        st["fn"] = f"{SHORT[st['crate']]}_{snake(st['name'])}"
    gk_app.number(m, lock, CALL_BASE)
    names = {i["name"] for i in m.tokens}
    for it, tpath in m.style_ext:
        if it["status"] in ("bound", "bridged") and not any(t.get("method_id") == it["id"] for t in m.tokens):
            assert it["name"] not in names, f"style token {it['name']} twice ({tpath})"
            names.add(it["name"])
            m.tokens.append(dict(it, kind="token", token_of=tpath, method_id=it["id"]))
    for it in sorted(m.tokens, key=lambda i: i["name"]):
        it["op"] = lock.num("style", "", it["name"])
    by_id = {i["id"]: i for i, _ in m.style_ext}
    for t in m.tokens:
        if t.get("method_id"):
            by_id[t["method_id"]]["token"] = t["op"]
    for it in sorted(m.palette, key=lambda i: i["id"]):
        it["op"] = lock.num("palette", "", it["id"])
    m.theme_nums = {name: lock.num("theme", "", name) for name in m.theme}
    for table in (m.enums, m.denums):
        for rt in sorted(table):
            for v in table[rt]["variants"]:
                v["num"] = lock.num("variant", rt, v["name"], first=0)
    for key in sorted(m.shapes):
        sh = m.shapes[key]
        sh["num"] = sh["builtin"] or lock.num("shape", "", key, first=SHAPE_BASE)
    for n, rt in enumerate(sorted(m.enums)):
        m.enums[rt]["fn"] = f"en_{n}"
    for n, rt in enumerate(sorted(m.denums)):
        m.denums[rt]["fn"] = f"de_{n}"
    for n, rt in enumerate(sorted(m.records)):
        m.records[rt]["fn"] = f"rc_{n}"
    for n, rt in enumerate(sorted(m.intos)):
        m.intos[rt]["fn"] = f"in_{n}"
    seen = {}
    for key in sorted(m.nodes):
        node = m.nodes[key]
        base = f"{SHORT[node['crate']]}_{snake(node['name'])}"
        if base in seen:
            base += "_" + re.sub(r"\W+", "_", node["module"])
        while base in seen:
            base += "x"
        seen[base] = key
        node["fn"] = base
    for n, key in enumerate(sorted(m.traits, key=lambda k: (m.traits[k]["key"] or "", str(k)))):
        m.traits[key]["fn"] = f"tr_{snake(m.traits[key]['name'])}_{n}"


def overrides_trait(m, node, t):
    """Return True if `node` implements any method of style-only trait `t` explicitly instead of inheriting the blanket impl."""
    names = {i["name"] for i in t["ops"]}
    c = m.api.crates[node["crate"]]
    for im in m.info(node["crate"], node["id"])["impls"]:
        tr = im.get("trait")
        if tr and not im.get("blanket_impl") and str(tr["path"]).rsplit("::", 1)[-1] == t["name"]:
            if any((c.item(x) or {}).get("name") in names for x in im["items"]):
                return True
    return False


class Rust:
    """Emits all generated Rust decoder modules under `core/src/gen` and tracks line-to-item ownership.

    Key state tracked during emission:
    - `self.files`: `{filename: source_text}` buffered in memory until `gk_fam.Families.split()`
      finishes partitioning non-core families.
    - `self.where`: `{filename: {line_num_str: model_key}}` saved to `gen/out/where.json` so
      compiler diagnostics can map any generated Rust line back to the upstream item that produced it.
    - `self.call`: controls how `dec()` and `passed()` handle borrows, fallibility, and handles:
      - `None`: view-tape decode with `&mut Dx` and `&mut R` (returns `Option<T>`, uses `*_shown` handles).
      - `"builder"` / `"codec"`: inner decode inside a record/enum `Unwire` or `StateBuilder`
        (returns `Option<T>`, converts `GkResult` lookups with `.ok()?`).
      - `"state-*"` / `"obj-*"` / `"ambient"`: call handler returning `GkResult<()>` with `&mut Call`
        (propagates handle lookup errors directly via `?`, clones `&str` inputs into `String`).
    - `self.stylefn_owned`: selects whether `stylefn` closures borrow `&mut Dx` inline or
      pre-build an owned `StyleRefinement` (set automatically in `arm()` when a builder has
      multiple `Dx`-borrowing arguments or is listed in `[rust].owned_style_closure`).
    """

    def __init__(self, m, outdir):
        self.m, self.out = m, outdir
        self.lines, self.where, self.file = [], {}, None
        self.files = {}
        self.call = None
        self.stylefn_owned = True


    def open(self, name, doc):
        """Start a new generated Rust module with standard lint allows and `genapi` imports."""
        self.file, self.lines = name, []
        self.where[name] = {}
        self.add("// Code generated by gen/gkgen.py. DO NOT EDIT. " + doc)
        self.add("#![allow(unused_variables, unused_imports, unused_mut, unreachable_patterns, unused_parens)]")
        self.add("#![allow(clippy::all)]")
        self.add("use crate::genapi::{self, Dx, R};")
        self.add("use super::*;")
        self.add("")

    def add(self, text, key=None):
        """Append one source line and optionally record its owning model key for `gen/out/where.json`."""
        self.lines.append(text)
        if key is not None:
            self.where[self.file][str(len(self.lines))] = key

    def close(self):
        """Store the completed module source in `self.files`."""
        self.files[self.file] = "\n".join(self.lines) + "\n"


    def dec(self, d):
        """Emit a Rust expression that decodes descriptor `d` from tape reader `r` (and `dx`/`c`)."""
        k = d[0]
        px = gk_app.rust_dec(self, d)
        if px is not None:
            return px
        ex = gk_editor.rust_dec(self, d)
        if ex is not None:
            return ex
        lx = gk_lists.rust_dec(self, d)
        if lx is not None:
            return lx
        if k == "dock":
            return gk_overlay.rust_dock_dec(self, d)
        if k in gk_values.CALL_SAFE or k in gk_values.DEC_ONLY:
            return gk_values.rust_dec(self, d)
        if k == "bool":
            return "r.u8() != 0"
        if k == "int":
            return {"u8": "r.u8()", "u16": "r.u16()", "u32": "r.u32()", "u64": "r.u64()", "usize": "r.u64() as usize",
                    "i8": "r.u8() as i8", "i16": "r.u16() as i16", "i32": "r.i32()", "i64": "r.i64()",
                    "isize": "r.i64() as isize"}[d[1]]
        if k == "f32":
            return "r.f32()"
        if k == "f64":
            return "r.f64()"
        if k == "str":
            c = d[1].lstrip("&")
            if self.call and c in ("str", "asref"):
                return "r.str().to_string()"
            return {"str": "r.str()", "asref": "r.str()", "String": "r.str().to_string()",
                    "SharedString": "gpui_kit::SharedString::new(r.str())",
                    "into": "gpui_kit::SharedString::new(r.str())",
                    "PathBuf": "std::path::PathBuf::from(r.str())",
                    "Cow": "std::borrow::Cow::Owned(r.str().to_string())"}[c]
        if k == "leaf":
            e = {"px": "gpui_kit::px(r.f32())", "rems": "gpui_kit::rems(r.f32())",
                 "color": "genapi::color(r, &*c.cx)" if self.call else "genapi::color(r, &*dx.cx)", "length": "length(r)",
                 "definite_length": "definite_length(r)", "absolute_length": "absolute_length(r)",
                 "id": "element_id(r)", "duration": "std::time::Duration::from_millis(r.u32() as u64)",
                 "range": "{ let a = r.u64() as usize; a..(r.u64() as usize) }"}[d[1]]
            if gk_model.is_fill(d):
                e = "genapi::background(r, &*c.cx)" if self.call else "genapi::background(r, &*dx.cx)"
            return f"<{d[2]}>::from({e})" if d[2] else e
        if k == "enum":
            return f"{self.m.enums[d[1]]['fn']}(r.u16())"
        if k in ("denum", "rec", "into"):
            fn = {"denum": self.m.denums, "rec": self.m.records, "into": self.m.intos}[k][d[1]]["fn"]
            if self.call in ("builder", "codec"):
                return f"{fn}_c(r)?"
            if self.call:
                return f"genapi::decoded({fn}_c(r))?"
            return f"{fn}(r, dx)?"
        if k == "from":
            return f"<{d[2]} as From<{d[3]}>>::from({self.dec(d[1])})"
        if k == "opt":
            return f"if r.u8() != 0 {{ Some({self.dec(d[1])}) }} else {{ None }}"
        if k == "list":
            return ("{ let n = r.u32() as usize; let mut v = Vec::with_capacity(n.min(4096)); "
                    f"for _ in 0..n {{ v.push({self.dec(d[1])}); }} v }}")
        if k == "tuple":
            return "(" + ", ".join(self.dec(x) for x in d[1]) + ("," if len(d[1]) == 1 else "") + ")"
        if k == "ref" and self.call and d[1][0] == "obj" and self.call in ("builder", "codec"):
            return f"genapi::obj_clone::<{d[1][1]}>(r.u64()).ok()?"
        if k == "ref" and self.call and d[1][0] == "obj":
            return f"genapi::obj_rc::<{d[1][1]}>(r.u64())?"
        if k == "ref":
            return self.dec(d[1])
        if k == "refmut":
            return f"genapi::obj_rc::<{d[1][1]}>(r.u64())?"
        if k == "obj":
            if self.call in ("builder", "codec"):
                return f"genapi::obj_clone::<{d[1]}>(r.u64()).ok()?"
            if self.call:
                return f"genapi::obj_clone::<{d[1]}>(r.u64())?" if d[3] else f"genapi::obj_take::<{d[1]}>(r.u64())?"
            return f"genapi::obj_shown::<{d[1]}>(r.u64())"

        if k == "child":
            return "dx.child(r.u32())"
        if k == "node" and self.call:
            return f"genapi::value_arg(c, r, build_{self.m.nodes[d[1]]['fn']})?"
        if k == "node":
            return f"build_{self.m.nodes[d[1]]['fn']}(dx, r.u32())?"
        if k == "entity":
            return f"genapi::state::<{d[1]}>(r.u64())?" if self.call else f"genapi::shown::<{d[1]}>(r.u64())"
        if k == "rows":
            return f"{d[1]}(r.u64())"
        if k == "shim":
            return f"{d[1]}::from_source(r.u64())?" if self.call else f"{d[1]}::from_source(r.u64()).ok()?"
        if k == "panelview":
            return gk_overlay.rust_panel_view(self, d)
        if k == "handle":
            return "genapi::focus(r.u64())?" if self.call else "genapi::shown_focus(r.u64())"
        if k == "action" and d[1] == "listener":
            return "{ let a = r.u32(); let h = dx.handler(r); genapi::action_listener(a, h, dx.pin()) }"
        if k == "data":
            if self.call == "builder":
                return "crate::data::rows_plain(r)"
            return "crate::data::rows(r, &*c.cx)" if self.call else "crate::data::rows(r, &*dx.cx)"
        if k in ("field", "format"):
            return self.data_closure(d)
        if k == "datum_rows":
            ps = self.closure_params(["arg", "window", "app"], d[2], "v")
            row = "v" if not d[3] else f"v.{d[3]}"
            return self.wrap(d[1], "{ let src = r.u64(); move |%s| crate::data::row_element(src, %s, window, cx) }" % (ps, row))
        if k == "action":
            return "genapi::action(r).ok()?"
        if k == "handler":
            return self.handler(d)
        if k == "fragment":
            ps = self.closure_params(d[1], d[3])
            return self.wrap(d[2], "{ let f = dx.frag(r.u32()); move |%s| f.with(window, cx, |dx, n| dx.child(n)) }" % ps)
        if k == "viewbuild":
            return gk_overlay.rust_dec(self, d)
        if k == "build":
            fn = self.m.nodes[d[1]]["fn"]
            if "window" not in d[2]:
                return "{ let n = r.u32(); let dx = &mut *dx; move |v: %s| { let first = dx.node(n).2; apply_%s(dx, v, first) } }" % (d[4][0], fn)
            ps = self.closure_params(d[2], d[4], "v")
            return self.wrap(d[3], "{ let f = dx.frag(r.u32()); move |%s| f.with(window, cx, |dx, n| "
                             "apply_%s(dx, v, dx.node(n).2)) }" % (ps, fn))
        if k == "stylefn":
            if d[2] == "impl" and not self.stylefn_owned:
                return ("{ let n = r.u32(); let dx = &mut *dx; move |mut v: gpui_kit::StyleRefinement| "
                        "{ style_into(dx, n, &mut v); v } }")
            return self.wrap(d[2], "{ let s = build_%s(dx, r.u32())?; move |v: gpui_kit::StyleRefinement| refined(v, &s) }"
                             % self.m.nodes[d[1]]["fn"])
        raise KeyError(k)

    def data_closure(self, d):
        """Emit a Rust closure reading a chart `Field` or `Format` template from `crate::data::Datum`."""
        k, kind, ret, how, ptypes, forms = d
        ps, row, nums = [], None, []
        for n, (t, f) in enumerate(zip(ptypes, forms)):
            ps.append(f"a{n}: {t}")
            if f[0] == "datum":
                row = f"a{n}" if not f[1] else f"a{n}.{f[1]}"
            elif f[0] in ("f32", "f64", "int"):
                v = f"*a{n}" if t.startswith("&") else f"a{n}"
                nums.append({"f32": f"crate::data::Arg::F32({v})", "f64": f"crate::data::Arg::F64({v})",
                             "int": f"crate::data::Arg::Int({v} as i64)"}[f[0]])
        if k == "format":
            body = "f.text(%s, &[%s])" % (f"Some({row})" if row else "None", ", ".join(nums))
            return self.wrap(how, "{ let f = crate::data::Format::new(r.str()); move |%s| -> %s { %s.into() } }"
                             % (", ".join(ps), ret, body))
        key = next((f"a{n}" for n, (t, f) in enumerate(zip(ptypes, forms)) if f == ["other"] and t == "&str"), None)
        if key and kind in ("num", "opt"):
            row = f"&{row}.keyed({key})" if not row.startswith("&") else f"&{row[1:]}.keyed({key})"
        body = {"text": f"f.text({row}).into()", "color": f"f.color({row}).into()",
                "num": f"f.num({row}) as {ret}", "opt": f"f.opt({row}).map(|v| v as {ret})"}[kind]
        rt = f"Option<{ret}>" if kind == "opt" else ret
        return self.wrap(how, "{ let f = crate::data::Field::new(r.str()); move |%s| -> %s { %s } }"
                         % (", ".join(ps), rt, body))

    @staticmethod
    def wrap(how, closure):
        """Wrap a Rust closure expression in `Box::new`, `Rc::new`, `Arc::new`, or leave bare (`impl`)."""
        return {"impl": closure, "box": f"Box::new({closure})", "rc": f"std::rc::Rc::new({closure})",
                "arc": f"std::sync::Arc::new({closure})"}[how]

    @staticmethod
    def closure_params(params, types, arg="_"):
        """Format Rust closure parameter bindings (`window`, `cx`, and `_aN` or custom `arg`)."""
        n, out = 0, []
        for p, t in zip(params, types):
            if p == "arg":
                out.append((arg if arg != "_" else f"_a{n}") + ": " + t)
                n += 1
            else:
                out.append(("window" if p.startswith("window") else "cx") + ": " + t)
        return ", ".join(out)

    def handler(self, d):
        """Emit a Rust event-callback closure that fires Go handler `h` through `genapi::fire*`."""
        sh = self.m.shapes[d[1]]
        names, refs, n = [], [], 0
        for p, t in zip(d[2], d[4]):
            if p == "arg":
                names.append(f"a{n}: {t}")
                refs.append(f"a{n}" if d[5][n][0] == "ref" else f"&a{n}")
                n += 1
            else:
                names.append({"window": "window", "app": "cx", "context": "cx"}.get(p, "_x") + ": " + t)
        payload = "&()" if not refs else refs[0] if len(refs) == 1 else "&(" + ", ".join(refs) + ")"
        payload = gk_app.rust_payload(d, payload) or payload
        ret = {"bool": "bool", "str": "gpui_kit::SharedString"}.get(sh["ret"], "()")
        if gk_overlay.rust_observe(d, sh['num'], payload):
            call = gk_overlay.rust_observe(d, sh['num'], payload)
        elif "context" in d[2]:
            call = f"genapi::fire_queued(h, {sh['num']}, {payload}, cx)"
        elif "window" in d[2]:
            call = f"genapi::fire::<_, {ret}>(h, {sh['num']}, {payload}, window, cx)"
        else:
            call = f"genapi::fire_app::<_, {ret}>(h, {sh['num']}, {payload}, cx)"
        return self.wrap(d[3], "{ let h = dx.handler(r); let k = dx.pin(); move |%s| { let _ = &k; %s } }" % (", ".join(names), call))

    def passed(self, d, var):
        """Adapt decoded local variable `var` to the exact reference/slice/context form expected by the callee."""
        if d == ["ctx", "window_opt"]:
            return "Some(window)" if self.call else "Some(&mut *dx.window)"
        if d[0] == "ctx" and self.call:
            return "window" if d[1].startswith("window") else "cx"
        if d[0] == "str" and self.call and d[1].lstrip("&") in ("str", "asref"):
            return f"&{var}[..]"
        if d[0] == "ctx":
            return {"window": "&mut *dx.window", "app": "&mut *dx.cx", "window_ref": "&*dx.window",
                    "app_ref": "&*dx.cx"}[d[1]]
        if d[0] == "opt" and d[1][0] == "ref" and self.call and d[1][1][0] == "obj":
            return f"{var}.as_ref().map(|x| x.borrow()).as_deref()"
        if d[0] == "opt" and d[1][0] == "ref":
            return f"{var}.as_ref()"
        if d[0] == "ref" and self.call and d[1][0] == "obj":
            return f"&*{var}.borrow()"
        if d[0] == "refmut":
            return f"&mut *{var}.borrow_mut()"
        if d[0] == "ref":
            return f"&*{var}" if d[1][0] == "action" else f"&{var}"
        if d[0] == "list" and d[2] == "slice" and d[1][0] == "ref" and d[1][1][0] != "obj":
            return f"&{var}.iter().collect::<Vec<_>>()[..]"
        if d[0] == "list" and d[2] == "slice":
            return f"&{var}[..]"
        if d[0] == "data" and d[1] == "slice":
            return f"&{var}[..]"
        if d[0] == "str" and d[1].startswith("&") and d[1] != "&str":
            return f"&{var}"
        return var

    def arm(self, it, call, bail):
        """Emit a match-arm block that decodes `it["args"]`, handles `?` failures via `bail`, and runs `call`.

        When an `impl FnOnce(StyleRefinement) -> StyleRefinement` closure would otherwise
        conflict with a second `&mut Dx` borrow in the same argument list, falls back to
        pre-building an owned `StyleRefinement` (`stylefn_owned = True`) before re-decoding.
        """
        sp = gk_lists.rust_arm(self, it, call, bail)
        if sp:
            return sp
        def decode():
            lets, passed = [], []
            for n, a in enumerate(it["args"]):
                d = a["desc"]
                if d[0] != "ctx":
                    lets.append((f"a{n}", self.dec(d)))
                passed.append(self.passed(d, f"a{n}"))
            return lets, passed
        self.stylefn_owned = bool(self.call) or it["id"] in self.m.ov.get("rust", {}).get("owned_style_closure", [])
        lets, passed = decode()
        borrows = sum(1 for _, e in lets if "let dx = &mut *dx" in e)
        if borrows and (borrows > 1 or len(lets) > 1 and any("dx" in e for _, e in lets if "let dx = &mut *dx" not in e)):
            self.stylefn_owned = True
            lets, passed = decode()
        text = call % ", ".join(passed) if "%s" in call else call
        if not lets:
            return "{ " + text + " }"
        if bail is None or not any("?" in e for _, e in lets):
            return "{ " + "".join(f"let {v} = {e}; " for v, e in lets) + text + " }"
        vs = ", ".join(v for v, _ in lets)
        es = ", ".join(e for _, e in lets)
        one = "," if len(lets) == 1 else ""
        return "{ let Some((%s%s)) = (|| Some((%s%s)))() else { %s }; %s }" % (vs, one, es, one, bail, text)


    def emit(self):
        """Emit all Rust files, run `gk_fam.Families.split()`, and write `core/src/gen/*.rs`."""
        m = self.m
        self.codecs()
        self.style()
        self.traits()
        self.calls()
        groups = {}
        for key in sorted(m.nodes):
            node = m.nodes[key]
            if not [i for i in node["ctors"] if i["status"] in ("bound", "bridged")]:
                continue
            fname = f"n_{SHORT[node['crate']]}_{re.sub(r'[^a-z0-9]+', '_', node['module'].lower()) or 'root'}"
            groups.setdefault(fname, []).append(node)
        for fname in sorted(groups):
            self.open(fname + ".rs", "Node decoders for a single upstream module.")
            for node in groups[fname]:
                self.node(node)
            self.close()
        gk_editor.rust_files(self, groups)
        self.file, self.lines = "mod.rs", []
        self.where["mod.rs"] = {}
        self.add("// Code generated by gen/gkgen.py. DO NOT EDIT.")
        self.add("// Generated decoders, codecs, style tables, and dispatchers calling only crate::genapi.")
        self.add("#![allow(unused_variables, unused_imports, unused_mut, unreachable_patterns, dead_code, non_snake_case, clippy::all)]")
        self.add("use crate::genapi::{self, Dx, R};")
        self.add("use gpui_kit::IntoElement as _;")
        for f in ["codecs", "style", "traits", "calls"] + sorted(groups):
            self.add(f"mod {f};")
            self.add(f"pub use {f}::*;")
        self.add("")
        self.add("/// Decode a generated element node into `AnyElement`; returns None for non-generated type IDs.")
        self.add("pub fn node(dx: &mut Dx, ty: u16, args: u32, first: u32) -> Option<gpui_kit::AnyElement> {")
        self.add("    let el = match ty {")
        for key in sorted(m.nodes):
            node = m.nodes[key]
            nums = sorted(i["op"] for i in node["ctors"] if i["status"] in ("bound", "bridged"))
            if node["element"] and nums:
                self.add(f"        {' | '.join(map(str, nums))} => make_{node['fn']}(dx, ty, args, first).map(|e| e.into_any_element()),",
                         ["node", key])
        self.add("        _ => return None,")
        self.add("    };")
        self.add("    Some(el.unwrap_or_else(|| gpui_kit::div().into_any_element()))")
        self.add("}")
        self.add("")
        self.add("/// Apply shared style tokens and style-only trait ops to `style`; returns false if `op` is unknown.")
        self.add("pub fn style_op(style: &mut gpui_kit::StyleRefinement, op: u16, r: &mut R, dx: &mut Dx) -> bool {")
        self.add("    match op {")
        self.add(f"        {OP_STYLE} => style_tokens(style, r),")
        for t in sorted(m.traits.values(), key=lambda t: t.get("num") or 0):
            ops = [i for i in t["ops"] if i["status"] in ("bound", "bridged") and i.get("op") is not None]
            if t["via_style"] and ops and not t["extra"] and t["name"] in ("Styled", "StyledExt"):
                lo = TRAIT_BASE + (t["num"] << 7)
                self.add(f"        {lo}..={lo + 127} => {{ let _ = {t['fn']}(W(style), op, r, dx); }}")
        self.add("        _ => return false,")
        self.add("    }")
        self.add("    true")
        self.add("}")
        self.add("")
        self.add("/// Draw callback for one value part inside a custom plot.")
        self.add("pub type PaintFn = Box<dyn Fn(&gpui_kit::Bounds<gpui_kit::Pixels>, &mut gpui_kit::Window, &mut gpui_kit::App)>;")
        self.add("")
        self.add("/// Decode value node `n` into a `PaintFn` if its type supports painting; otherwise returns None.")
        self.add("pub fn paint_part(dx: &mut Dx, n: u32) -> Option<PaintFn> {")
        self.add("    match dx.node(n).0 {")
        for key in sorted(m.nodes):
            node = m.nodes[key]
            it = node.get("paint")
            nums = sorted(i["op"] for i in node["ctors"] if i["status"] in ("bound", "bridged"))
            if it is None or it["status"] != "bridged" or not nums:
                continue
            cx = ", cx" if it.get("paint_app") else ""
            self.add(f"        {' | '.join(map(str, nums))} => {{ let v = build_{node['fn']}(dx, n)?; "
                     f"Some(Box::new(move |b: &gpui_kit::Bounds<gpui_kit::Pixels>, window: &mut gpui_kit::Window, cx: &mut gpui_kit::App| "
                     f"{{ let _ = &cx; {it['callee']}(&v, b, window{cx}) }})) }}",
                     ["item", it["id"]])
        self.add("        _ => None,")
        self.add("    }")
        self.add("}")
        self.add("")
        for line in gk_values.rust_mod(self, m):
            self.add(line)
        for line in gk_tests.rust_test_shape(self, m):
            self.add(line)
        self.add(SUPPORT)
        self.close()
        self.fam = gk_fam.Families(m, self.call_items)
        self.fam.split(self.files, self.where)
        os.makedirs(self.out, exist_ok=True)
        for f in os.listdir(self.out):
            if f.endswith(".rs") and f not in self.files:
                os.remove(os.path.join(self.out, f))
        for name, text in self.files.items():
            open(os.path.join(self.out, name), "w").write(text)
        return self.where

    def cfg(self, item, tags):
        if item.get("cfg"):
            self.add(f"#[cfg({item['cfg']})]", tags)

    @staticmethod
    def arm_cfg(v):
        return f"#[cfg({v['cfg']})] " if v.get("cfg") else ""

    def codecs(self):
        """Emit codecs.rs with enum/record/Into decoders, Wire encoders, and color token lookup."""
        m = self.m
        self.open("codecs.rs", "Wire decoders and encoders for enums, records, Into sets, and color tokens.")
        for rt in sorted(m.enums):
            e = m.enums[rt]
            vs = [v for v in e["variants"]]
            if e["in"] and vs:
                self.cfg(e, ["enum", rt])
                self.add(f"pub fn {e['fn']}(v: u16) -> {rt} {{", ["enum", rt])
                self.add("    match v {", ["enum", rt])
                for v in vs[1:]:
                    self.add(f"        {self.arm_cfg(v)}{v['num']} => {e['path']}::{v['name']},", ["variant", rt, v["name"]])
                self.add(f"        _ => {e['path']}::{vs[0]['name']},", ["enum", rt])
                self.add("    }", ["enum", rt])
                self.add("}", ["enum", rt])
            if e["out"]:
                self.cfg(e, ["enum", rt])
                self.add(f"impl genapi::Wire for {rt} {{ fn put(&self, out: &mut Vec<u8>) {{ let n: u16 = match self {{ "
                         + " ".join(f"{self.arm_cfg(v)}{e['path']}::{v['name']} => {v['num']}," for v in vs) + " _ => 0xffff }; "
                         "out.extend_from_slice(&n.to_le_bytes()); } }", ["enum", rt])
        for rt in sorted(m.denums):
            e = m.denums[rt]
            for sfx, ps, mode in (("", "r: &mut R, dx: &mut Dx", None), ("_c", "r: &mut R", "codec")):
                if not e["in"] or (mode and not e.get("callable")):
                    continue
                self.call = mode
                self.cfg(e, ["denum", rt])
                self.add(f"pub fn {e['fn']}{sfx}({ps}) -> Option<{rt}> {{", ["denum", rt])
                self.add("    Some(match r.u16() {", ["denum", rt])
                for v in e["variants"]:
                    if not v["ok"] or not v.get("in", True):
                        continue
                    self.add(f"        {self.arm_cfg(v)}{v['num']} => {self.variant_new(e, v)},", ["variant", rt, v["name"]])
                for arm in gk_editor.denum_arms(self, e):
                    self.add(arm, ["denum", rt])
                self.add("        _ => return None,", ["denum", rt])
                self.add("    })", ["denum", rt])
                self.add("}", ["denum", rt])
                self.call = None
            if e["out"]:
                self.cfg(e, ["denum", rt])
                self.add(f"impl genapi::Wire for {rt} {{", ["denum", rt])
                self.add("    fn put(&self, out: &mut Vec<u8>) {", ["denum", rt])
                self.add("        match self {", ["denum", rt])
                for v in e["variants"]:
                    tag = f"out.extend_from_slice(&{v['num']}u16.to_le_bytes());"
                    put = lambda x: f" genapi::Wire::put({x}, out);"
                    ac = self.arm_cfg(v)
                    if v["shape"] == "unit" or not v["ok"]:
                        pat = {"unit": "", "tuple": "(..)", "struct": " { .. }"}[v["shape"]]
                        self.add(f"            {ac}{e['path']}::{v['name']}{pat} => {{ {tag} }}", ["variant", rt, v["name"]])
                    elif v["shape"] == "tuple":
                        ns = ", ".join(f"f{n}" for n in range(len(v["fields"])))
                        self.add(f"            {ac}{e['path']}::{v['name']}({ns}) => {{ {tag}" +
                                 "".join(put(f"f{n}") for n in range(len(v["fields"]))) + " }", ["variant", rt, v["name"]])
                    else:
                        ns = ", ".join(f["name"] for f in v["fields"])
                        self.add(f"            {ac}{e['path']}::{v['name']} {{ {ns}, .. }} => {{ {tag}" +
                                 "".join(put(f["name"]) for f in v["fields"]) + " }", ["variant", rt, v["name"]])
                self.add("            _ => out.extend_from_slice(&0xffffu16.to_le_bytes()),", ["denum", rt])
                self.add("        }", ["denum", rt])
                self.add("    }", ["denum", rt])
                self.add("}", ["denum", rt])
        for rt in sorted(m.records):
            rc = m.records[rt]
            for sfx, ps, mode in (("", "r: &mut R, dx: &mut Dx", None), ("_c", "r: &mut R", "codec")):
                if not rc["in"] or (mode and not rc.get("callable")):
                    continue
                self.call = mode
                if rc["tuple"]:
                    body = f"{rc['path']}(" + ", ".join(self.dec(f["desc"]) for f in rc["fields"]) + ")"
                else:
                    body = f"{rc['path']} {{ " + ", ".join(f"{f['name']}: {self.dec(f['desc'])}" for f in rc["fields"]) + " }"
                self.add(f"pub fn {rc['fn']}{sfx}({ps}) -> Option<{rt}> {{ Some({body}) }}", ["rec", rt])
                self.call = None
            if rc["out"]:
                body = " ".join(gk_values.rust_field_put(rc, f) for f in rc["fields"])
                body = gk_editor.reads_put(rc) or body
                self.add(f"impl genapi::Wire for {rt} {{ fn put(&self, out: &mut Vec<u8>) {{ {body} }} }}", ["rec", rt])
        for rt in sorted(m.intos):
            s = m.intos[rt]
            if len(s["members"]) < 2:
                continue
            for sfx, ps, mode in (("", "r: &mut R, dx: &mut Dx", None), ("_c", "r: &mut R", "codec")):
                if mode and not s.get("callable"):
                    continue
                self.call = mode
                self.add(f"pub fn {s['fn']}{sfx}({ps}) -> Option<{rt}> {{", ["into", rt])
                self.add("    Some(match r.u8() {", ["into", rt])
                for n, mem in enumerate(s["members"]):
                    e = self.dec(mem["desc"])
                    if mem["from"]:
                        e = f"<{rt} as From<{mem['from']}>>::from({gk_lists.from_arg(mem['from'], e)})"
                    self.add(f"        {n} => {e},", ["into", rt])
                self.add("        _ => return None,", ["into", rt])
                self.add("    })", ["into", rt])
                self.add("}", ["into", rt])
                self.call = None
        self.add("")
        self.add("// Cloneable library objects serialize by cloning into a fresh temporary handle.")
        for key in sorted(m.objects):
            st = m.objects[key]
            if st.get("reachable") and st["clone"] and st["path"] not in m.records:
                self.add(f"impl genapi::Wire for {st['path']} {{ fn put(&self, out: &mut Vec<u8>) {{ "
                         f"genapi::Put::put_u64(out, genapi::obj_temp(self.clone())); }} }}", ["state", key])
        self.add("")
        self.add("/// Resolve a numeric color token: values below 0x8000 index `ThemeColor`;")
        self.add("/// values at or above 0x8000 select a fixed palette color.")
        self.add("pub fn color_token(t: &gpui_kit::component::ThemeColor, id: u16) -> Option<gpui_kit::Hsla> {")
        self.add("    if id >= 0x8000 {")
        self.add("        return palette_color(id);")
        self.add("    }")
        self.add("    Some(match id {")
        for name, n in sorted(m.theme_nums.items(), key=lambda kv: kv[1]):
            self.add(f"        {n} => t.{name},", ["theme", name])
        self.add("        _ => return None,")
        self.add("    })")
        self.add("}")
        self.add("")
        self.add("#[cold]")
        self.add("#[inline(never)]")
        self.add("fn palette_color(id: u16) -> Option<gpui_kit::Hsla> {")
        self.add("    Some(match id {")
        for it in sorted(m.palette, key=lambda i: i["op"]):
            if it["status"] == "bound":
                self.add(f"        {0x8000 + it['op']} => {it['callee']}(),", ["item", it["id"]])
        self.add("        _ => return None,")
        self.add("    })")
        self.add("}")
        self.close()

    def variant_new(self, e, v):
        vn = gk_editor.variant_new(self, e, v)
        if vn:
            return vn
        if v["shape"] == "unit":
            return f"{e['path']}::{v['name']}"
        if v["shape"] == "tuple":
            return f"{e['path']}::{v['name']}(" + ", ".join(self.dec(f["desc"]) for f in v["fields"]) + ")"
        return f"{e['path']}::{v['name']} {{ " + ", ".join(f"{f['name']}: {self.dec(f['desc'])}" for f in v["fields"]) + " }"

    def style(self):
        """Emit style.rs mapping nullary style token IDs onto StyleRefinement mutations."""
        m = self.m
        self.open("style.rs", "Nullary style token dispatch table for StyleRefinement.")
        self.add("use gpui_kit::{StyleRefinement, Styled};")
        self.add("")
        self.add("/// Adapter allowing by-value `Styled` methods to mutate a borrowed `StyleRefinement`.")
        self.add("pub struct W<'a>(pub &'a mut StyleRefinement);")
        self.add("impl Styled for W<'_> {")
        self.add("    fn style(&mut self) -> &mut StyleRefinement {")
        self.add("        self.0")
        self.add("    }")
        self.add("}")
        self.add("")
        hot_names = {
            "flex", "flex_1", "flex_row", "flex_col", "flex_wrap", "flex_shrink_0", "flex_grow",
            "items_center", "items_start", "items_end", "justify_between", "justify_center",
            "justify_start", "justify_end", "size_full", "w_full", "h_full",
            "gap_1", "gap_2", "gap_3", "gap_4", "p_1", "p_2", "p_3", "p_4",
            "px_1", "px_2", "px_3", "px_4", "py_1", "py_2", "py_3", "py_4",
            "rounded_sm", "rounded_md", "rounded_lg", "border_1", "overflow_hidden",
            "relative", "absolute", "cursor_pointer", "truncate",
            "text_xs", "text_sm", "text_base", "font_medium", "font_semibold", "font_bold",
            "h_flex", "v_flex",
        }
        bound = [it for it in sorted(m.tokens, key=lambda i: i["op"]) if it["status"] == "bound"]
        hot = [it for it in bound if it["name"] in hot_names]
        cold = [it for it in bound if it["name"] not in hot_names]
        self.add("pub fn style_token(style: &mut StyleRefinement, token: u16) -> bool {")
        self.add("    let s = W(style);")
        self.add("    let _ = match token {")
        for it in hot:
            self.add(f"        {it['op']} => {it.get('token_of', 'Styled')}::{it['name']}(s),", ["item", it["id"]])
        self.add("        _ => return style_token_rest(s.0, token),")
        self.add("    };")
        self.add("    true")
        self.add("}")
        self.add("")
        self.add("#[cold]")
        self.add("#[inline(never)]")
        self.add("fn style_token_rest(style: &mut StyleRefinement, token: u16) -> bool {")
        self.add("    let s = W(style);")
        self.add("    let _ = match token {")
        for it in cold:
            self.add(f"        {it['op']} => {it.get('token_of', 'Styled')}::{it['name']}(s),", ["item", it["id"]])
        self.add("        _ => return false,")
        self.add("    };")
        self.add("    true")
        self.add("}")
        self.add("")
        self.add("/// Decode a style-token list op (u16 count followed by u16 token IDs).")
        self.add("pub fn style_tokens(style: &mut StyleRefinement, r: &mut R) {")
        self.add("    for _ in 0..r.u16() {")
        self.add("        let t = r.u16();")
        self.add("        if !style_token(style, t) {")
        self.add("            bad_op(\"Styled\", t);")
        self.add("        }")
        self.add("    }")
        self.add("}")
        self.close()

    def traits(self):
        """Emit traits.rs with one generic op dispatcher per builder trait."""
        m = self.m
        self.open("traits.rs", "Generic builder-trait op dispatchers shared across implementing nodes.")
        for key in sorted(m.traits, key=lambda k: m.traits[k]["fn"]):
            t = m.traits[key]
            ops = [i for i in t["ops"] if i["status"] in ("bound", "bridged") and i.get("op") is not None]
            if not ops:
                continue
            bound = " + ".join([t["path"]] + t["extra_paths"])
            if not t["via_style"] and not os.environ.get("GK_NO_INLINE"):
                self.add("#[inline(always)]")
            hot_styled = {
                "bg", "text_color", "border_color", "w", "h", "min_w", "min_h", "max_w", "max_h",
                "size", "p", "px", "py", "pt", "pb", "pl", "pr", "m", "mx", "my", "mt", "mb",
                "ml", "mr", "gap", "gap_x", "gap_y", "rounded", "border", "opacity", "text_size",
                "flex_grow", "flex_shrink", "flex_basis",
            }
            ordered = sorted(ops, key=lambda i: i["op"])
            if t["name"] == "Styled" and t["via_style"] and len(ordered) > 40:
                hot_ops = [it for it in ordered if it["name"] in hot_styled]
                cold_ops = [it for it in ordered if it["name"] not in hot_styled]
                self.add(f"pub fn {t['fn']}<E: {bound}>(el: E, op: u16, r: &mut R, dx: &mut Dx) -> E {{", ["trait", t["key"]])
                self.add("    match op {")
                for it in hot_ops:
                    call = f"<E as {t['path']}>::{it['name']}{gk_app.turbofish(it)}(el%s)" % ("" if not it["args"] else ", %s")
                    self.add(f"        {it['op']} => {self.arm(it, call, 'return el')}", ["item", it["id"]])
                self.add(f"        _ => {t['fn']}_rest(el, op, r, dx),")
                self.add("    }")
                self.add("}")
                self.add("#[cold]")
                self.add("#[inline(never)]")
                self.add(f"fn {t['fn']}_rest<E: {bound}>(el: E, op: u16, r: &mut R, dx: &mut Dx) -> E {{")
                self.add("    match op {")
                for it in cold_ops:
                    call = f"<E as {t['path']}>::{it['name']}{gk_app.turbofish(it)}(el%s)" % ("" if not it["args"] else ", %s")
                    self.add(f"        {it['op']} => {self.arm(it, call, 'return el')}", ["item", it["id"]])
                self.add("        _ => { bad_op(\"%s\", op); el }" % t["name"])
                self.add("    }")
                self.add("}")
                continue
            self.add(f"pub fn {t['fn']}<E: {bound}>(el: E, op: u16, r: &mut R, dx: &mut Dx) -> E {{", ["trait", t["key"]])
            self.add("    match op {")
            for it in ordered:
                call = f"<E as {t['path']}>::{it['name']}{gk_app.turbofish(it)}(el%s)" % ("" if not it["args"] else ", %s")
                self.add(f"        {it['op']} => {self.arm(it, call, 'return el')}", ["item", it["id"]])
            self.add("        _ => { bad_op(\"%s\", op); el }" % t["name"])
            self.add("    }")
            self.add("}")
        self.close()

    def calls(self):
        """Emit calls.rs with state/object/ambient call handlers and the top-level call() dispatcher."""
        m = self.m
        ok = lambda i: i["status"] in ("bound", "bridged")
        self.open("calls.rs", "Call handlers for state entities, library objects, and ambient context calls.")
        self.add("use gpui_kit::AppContext as _;")
        self.add("")
        arms = []
        for key in sorted(m.states):
            st = m.states[key]
            T = st["path"]
            if st.get("obj"):
                if st.get("reachable"):
                    self.obj_calls(st, arms)
                continue
            if not any(ok(i) for i in st["ctors"]) and not st.get("handed"):
                continue
            self.add(f"fn sb_{st['fn']}(s: {T}, op: u16, r: &mut R) -> {T} {{", ["state", key])
            self.add("    match op {")
            self.call = "builder"
            for it in sorted(st["builders"], key=lambda i: i["op"]):
                if ok(it):
                    call = f"{it['callee']}(s%s)" % ("" if not it["args"] else ", %s")
                    self.add(f"        {it['op']} => {self.arm(it, call, 'return s')}", ["item", it["id"]])
            self.add("        _ => s,")
            self.add("    }")
            self.add("}")
            for it in sorted(st["ctors"] + st["reads"] + st["writes"] + st["calls"] + st["events"], key=lambda i: i["op"]):
                if not ok(it):
                    continue
                head = f"fn c_{it['op']}(r: &mut R, c: &mut genapi::Call) -> genapi::GkResult<()> {{ "
                if "shape" in it:
                    body = ("let (h, slot, generation) = (r.u64(), r.u32(), r.u32()); "
                            f"let sub = genapi::state_subscribe::<{T}, {it['rust']}>(h, slot, generation, {m.shapes[it['shape']]['num']}, c)?; "
                            "c.out.extend_from_slice(&sub.to_le_bytes());")
                    self.add(head + body + " Ok(()) }", ["item", it["id"]])
                    arms.append(it)
                    continue
                self.call = it["kind"]
                lets = "".join(f"let a{n} = {self.dec(a['desc'])}; " for n, a in enumerate(it["args"]) if a["desc"][0] != "ctx")
                passed = ", ".join(self.passed(a["desc"], f"a{n}") for n, a in enumerate(it["args"]))
                if it["kind"] == "state-ctor":
                    win = "let window = c.window()?; " if any(a["desc"] == ["ctx", "window"] for a in it["args"]) else ""
                    cxn = "cx" if any(a["desc"] in (["ctx", "context"], ["ctx", "app"]) for a in it["args"]) else "_cx"
                    body = (f"{lets}{win}let entity = c.cx.new(|{cxn}| {{ let mut s = {it['callee']}({passed}); "
                            f"for _ in 0..r.u16() {{ let op = r.u16(); s = sb_{st['fn']}(s, op, r); }} s }}); "
                            "let h = genapi::state_new(entity, c); c.out.extend_from_slice(&h.to_le_bytes());")
                    body = gk_editor.chain_body(self, st, body)
                elif it["kind"] == "state-call":
                    sep = ", " if passed else ""
                    body = (f"let h = r.u64(); {lets}let bytes = genapi::state_call::<{T}>(h, c, move |s, window, cx| {{ let mut out = Vec::new(); "
                            f"{self.put_ret(it['ret'], it['callee'] + '(s' + sep + passed + ')')}; out }})?; c.out.extend_from_slice(&bytes);")
                elif it["kind"] == "state-read":
                    sep = ", " if passed else ""
                    body = (f"let h = r.u64(); {lets}let bytes = genapi::state_read::<{T}, _>(h, c, |s, cx| {{ let mut out = Vec::new(); "
                            f"{self.put_ret(it['ret'], it['callee'] + '(s' + sep + passed + ')')}; out }})?; c.out.extend_from_slice(&bytes);")
                else:
                    sep = ", " if passed else ""
                    body = (f"let h = r.u64(); {lets}genapi::state_update::<{T}>(h, c, move |s, window, cx| {{ "
                            f"{it['callee']}(s{sep}{passed}); }})?;")
                self.add(head + body + " Ok(()) }", ["item", it["id"]])
                arms.append(it)
        arms += gk_app.rust_calls(self)
        self.call_items = list(arms)
        self.call = None
        self.add("")
        self.add("/// Dispatch a generated call opcode; returns None if `op` is not generated.")
        self.add("pub fn call(op: u16, r: &mut R, c: &mut genapi::Call) -> Option<genapi::GkResult<()>> {")
        self.add("    Some(match op {")
        for it in sorted(arms, key=lambda i: i["op"]):
            self.add(f"        {it['op']} => c_{it['op']}(r, c),", ["item", it["id"]])
        self.add("        _ => return None,")
        self.add("    })")
        self.add("}")
        self.close()

    @staticmethod
    def put_ret(ret, expr):
        """Emit the Rust `Wire::put` statement serializing return expression `expr` for descriptor `ret`."""
        er = gk_editor.put_ret(ret, expr)
        if er:
            return er
        li = gk_lists.put_ret(ret, expr)
        if li:
            return li
        ov = gk_overlay.rust_dock_put(ret, expr)
        if ov:
            return ov
        if ret[0] == "obj":
            return f"genapi::Wire::put(&genapi::Owned::new({expr}), &mut out)"
        if gk_values.rust_put_ret(ret, expr):
            return gk_values.rust_put_ret(ret, expr)
        if ret[0] == "result":
            return gk_overlay.rust_put_ret(expr)
        return f"genapi::Wire::put(&{expr}, &mut out)"

    def obj_calls(self, st, arms):
        """Emit `sb_<fn>` builder loop and `c_<op>` call handlers for a non-Entity library object or Global."""
        m = self.m
        ok = lambda i: i["status"] in ("bound", "bridged")
        T, g = st["path"], "_global" if st["glob"] else ""
        if any(ok(i) for i in st["ctors"]):
            self.add(f"fn sb_{st['fn']}(s: {T}, op: u16, r: &mut R) -> {T} {{", ["state", st["key"]])
            self.add("    match op {")
            self.call = "builder"
            for it in sorted(st["builders"], key=lambda i: i["op"]):
                if ok(it):
                    call = f"{it['callee']}(s%s)" % ("" if not it["args"] else ", %s")
                    self.add(f"        {it['op']} => {self.arm(it, call, 'return s')}", ["item", it["id"]])
            self.add("        _ => s,")
            self.add("    }")
            self.add("}")
        items = st["ctors"] + st["globals"] + st["reads"] + st["writes"] + st["calls"] + st["statics"]
        for it in sorted([i for i in items if ok(i)], key=lambda i: i["op"]):
            head = f"fn c_{it['op']}(r: &mut R, c: &mut genapi::Call) -> genapi::GkResult<()> {{ "
            self.call = it["kind"]
            lets = "".join(f"let a{n} = {self.dec(a['desc'])}; " for n, a in enumerate(it["args"]) if a["desc"][0] != "ctx")
            passed = ", ".join(self.passed(a["desc"], f"a{n}") for n, a in enumerate(it["args"]))
            sep = ", " if passed else ""
            win = any(a["desc"][0] == "ctx" and a["desc"][1].startswith("window") for a in it["args"])
            k = it["kind"]
            if k == "obj-ctor":
                pre = "let window = c.window()?; " if win else ""
                body = (f"{lets}{pre}let cx = &mut *c.cx; let mut s = {it['callee']}({passed}); "
                        f"for _ in 0..r.u16() {{ let op = r.u16(); s = sb_{st['fn']}(s, op, r); }} "
                        "let h = genapi::obj_new(s); c.out.extend_from_slice(&h.to_le_bytes());")
                body = gk_editor.chain_body(self, st, body)
            elif k == "obj-global":
                body = f"let h = genapi::obj_global::<{T}>(); c.out.extend_from_slice(&h.to_le_bytes());"
            elif k == "obj-read":
                expr = gk_values.field_expr(it) or (it['callee'] + '(s' + sep + passed + ')')
                body = (f"let h = r.u64(); {lets}let bytes = genapi::obj_read{g}::<{T}, _>(h, c, |s, cx| {{ let mut out = Vec::new(); "
                        f"{self.put_ret(it['ret'], expr)}; out }})?; c.out.extend_from_slice(&bytes);")
            elif k in ("obj-static", "obj-scall"):
                call = f"{it['callee']}({passed})"
                inner = (f"{{ let mut out = Vec::new(); {self.put_ret(it['ret'], call)}; out }}" if k == "obj-scall"
                         else f"{{ {call}; Vec::new() }}")
                if win:
                    body = f"{lets}let bytes = genapi::with_window(c, |window, cx| {inner})?; c.out.extend_from_slice(&bytes);"
                else:
                    body = f"{lets}let bytes = {{ let cx = &mut *c.cx; {inner} }}; c.out.extend_from_slice(&bytes);"
            else:
                call = gk_values.field_set(it, passed) or f"{it['callee']}(s{sep}{passed})"
                inner = (f"{{ let mut out = Vec::new(); {self.put_ret(it['ret'], call)}; out }}" if k == "obj-call"
                         else f"{{ {call}; Vec::new() }}")
                if it.get("consume"):
                    copy = "|v| Some(v.clone())" if st["clone"] else "|_| None"
                    get = f"let s = genapi::obj_by_value::<{T}>(h, {copy})?; "
                    run = (f"genapi::with_window(c, move |window, cx| {inner})?" if win
                           else f"{{ let cx = &mut *c.cx; {inner} }}")
                    body = f"let h = r.u64(); {lets}{get}let bytes = {run}; c.out.extend_from_slice(&bytes);"
                elif win:
                    body = (f"let h = r.u64(); {lets}let bytes = genapi::obj_update_w{g}::<{T}, _>(h, c, move |s, window, cx| {inner})?; "
                            "c.out.extend_from_slice(&bytes);")
                else:
                    body = (f"let h = r.u64(); {lets}let bytes = genapi::obj_update{g}::<{T}, _>(h, c, move |s, cx| {inner})?; "
                            "c.out.extend_from_slice(&bytes);")
            self.add(head + body + " Ok(()) }", ["item", it["id"]])
            arms.append(it)
        self.call = None

    def node(self, node):
        """Emit `build_<fn>`, `make_<fn>`, and `apply_<fn>` tape decoders for one element or value node.

        `make_<fn>` matches the constructor opcode (`ty`) and decodes its arguments;
        `apply_<fn>` loops over the op chain starting at `first`, dispatching inherent ops,
        reserved `ParentElement`/`Styled` opcodes, and 128-wide trait op ranges.
        """
        m = self.m
        fn, tp = node["fn"], node["path"]
        ctors = sorted([i for i in node["ctors"] if i["status"] in ("bound", "bridged")], key=lambda i: i["op"])
        self.add(f"pub fn build_{fn}(dx: &mut Dx, n: u32) -> Option<{tp}> {{", ["node", node["key"]])
        self.add("    let (ty, args, first) = dx.node(n);")
        self.add(f"    make_{fn}(dx, ty, args, first)")
        self.add("}")
        self.add(f"pub fn make_{fn}(dx: &mut Dx, ty: u16, args: u32, first: u32) -> Option<{tp}> {{", ["node", node["key"]])
        self.add("    let r = &mut dx.args(args);")
        self.add(f"    let el: {tp} = match ty {{", ["node", node["key"]])
        for it in ctors:
            if it["kind"] == "convert":
                src = m.nodes[it["from"]]
                call = f"{it['callee']}(src%s)" % ("" if not it["args"] else ", %s")
                body = self.arm(it, call, None)
                self.add(f"        {it['op']} => {{ let src = build_{src['fn']}(dx, r.u32())?; {body} }}", ["item", it["id"]])
            else:
                self.add(f"        {it['op']} => {self.arm(it, it['callee'] + '(%s)', None)}", ["item", it["id"]])
        self.add(f"        _ => return bad_node(\"{node['name']}\", ty),")
        self.add("    };")
        self.add(f"    Some(apply_{fn}(dx, el, first))")
        self.add("}")
        self.add("#[inline]")
        self.add(f"pub fn apply_{fn}(dx: &mut Dx, mut el: {tp}, first: u32) -> {tp} {{", ["node", node["key"]])
        self.add("    let mut at = first;")
        self.add("    while let Some((op, mut r)) = dx.op(&mut at) {")
        self.add("        let r = &mut r;")
        self.add("        match op {")
        for it in sorted(node["ops"], key=lambda i: i["op"]):
            if it["status"] not in ("bound", "bridged"):
                continue
            call = f"{it['callee']}(el%s)" % ("" if not it["args"] else ", %s")
            self.add(f"            {it['op']} => {{ el = {self.arm(it, call, 'continue')}; }}", ["item", it["id"]])
        for tk in node["tables"]:
            t = next(t for t in m.traits.values() if t["key"] == tk)
            ops = [i["op"] for i in t["ops"] if i["status"] in ("bound", "bridged") and i.get("op") is not None]
            if t["name"] == "Styled" and not t["extra"]:
                self.add(f"            {OP_STYLE} => {{ style_tokens(gpui_kit::Styled::style(&mut el), r); }}",
                         ["table", node["key"], tk])
            if t["name"] == "ParentElement" and not t["extra"]:
                P = "gpui_kit::ParentElement"
                self.add(f"            {OP_CHILD} => {{ let c = dx.child(r.u32()); el = {P}::child(el, c); }}", ["table", node["key"], tk])
                self.add(f"            {OP_CHILD_VIEW} => {{ if let Some(v) = dx.child_view(r.u64()) {{ el = {P}::child(el, v); }} }}",
                         ["table", node["key"], tk])
                self.add(f"            {OP_TEXT} => {{ el = {P}::child(el, gpui_kit::SharedString::new(r.str())); }}", ["table", node["key"], tk])
            ops = [o for o in ops if o is not None]
            if not ops:
                continue
            lo, hi = TRAIT_BASE + (t["num"] << 7), TRAIT_BASE + (t["num"] << 7) + 127
            if t["via_style"] and node["styled"] and not t["extra"] and not overrides_trait(m, node, t):
                self.add(f"            {lo}..={hi} => {{ let _ = {t['fn']}(W(gpui_kit::Styled::style(&mut el)), op, r, dx); }}",
                         ["table", node["key"], tk])
            else:
                self.add(f"            {lo}..={hi} => {{ el = {t['fn']}(el, op, r, dx); }}", ["table", node["key"], tk])
        self.add(f"            _ => bad_op(\"{node['name']}\", op),")
        self.add("        }")
        self.add("    }")
        self.add("    el")
        self.add("}")
