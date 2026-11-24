"""Flat Go binding emitter (pre-split package `gk` plus `s`, `palette`, and `theme` subpackages).

This module translates the classified, wire-numbered `Model` into idiomatic Go:
1. **Deterministic naming (`Go.claim`, `Go.name_all`)**:
   Pre-seeds every hand-written exported identifier across `go/gk/**` into `self.names`
   (inverting any legacy renames from `gen/gk_go_legacy.json`) so generated declarations
   never collide with runtime types or helpers. Processes crates in priority order
   (`gpui_component` > `gpui_base` > `gpui` > `gpui_kit`) so high-level components win
   clean unprefixed names (`Button`, `Input`) while lower-level collisions receive a crate
   prefix (`BaseButton`, `GpuiPoint`). Element node types receive an `El` suffix (`DivEl`)
   so their constructor can take the clean noun (`Div(...)`).
2. **Type mapping and wire codecs (`gtype`, `put`, `rd`)**:
   - Maps model descriptors to Go types (`bool`, `GO_INT`, `float32`/`float64`, `string`,
     `Option[T]`, slices, fixed arrays, `Tuple*`, generated structs/enums/interfaces,
     `Element`, `RowSource`, `View`, `Data`, `Built[T]`, `Ask[K, V]`, `Cell[T]`, etc.).
   - Distinguishes inbound vs outbound color types (`Color` inbound vs `Hsla` outbound)
     and call-op vs view-tape handle encoding (`self.in_call`: `putVtape` vs `takeValue`,
     `held(h)` vs `putObj`).
   - Delegates extended descriptors to `gk_go_kinds` and dock descriptors to `gk_go_bridges`.
3. **Flat file emission (`emit`)**:
   - Emits enums, data enums (sealed interfaces + unit-variant `Kind` enums + variant structs),
     records (with optional shorthands), `Into` union interfaces, `oneof` element unions,
     event structs + `init()` handler shape decoders, ambient/state/object handles +
     `StateBuilder` chains, per-module element/value node files, `gen_items.go`,
     `gen_bridges.go`, and the three token subpackages (`go/gk/s`, `go/gk/palette`, `go/gk/theme`).
   - Keeps package `gk`'s files in `self.files` in memory, passes them through
     `gk_fam.Families.go_link()` (to inject `use<Family>()` hooks for static dead-stripping),
     and hands them to `gk_split.Split` to partition into the final layered Go subpackages.
4. **Coverage report (`report`)**:
   Verifies every bound/bridged model item was either emitted in `self.mapped` or accounted
   for by `gk_go_bridges.covered()`, writing any gaps to `gen/out/go-report.md`.
"""
import os
import re
import gk_doc
import gk_go_kinds
import gk_go_bridges
import gk_values

from gk_model import INT_WIRE, SHORT, ada, is_fill, snake

# Go keywords and predeclared identifiers suffixed with `_` when used as parameter names.
GO_KEYWORDS = {"break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for",
               "func", "go", "goto", "if", "import", "interface", "map", "package", "range", "return", "select",
               "struct", "switch", "type", "var",
               "len", "cap", "make", "new", "append", "copy", "delete", "panic", "string", "int", "bool", "byte",
               "error", "any", "nil", "true", "false", "rune", "float32", "float64", "uint8", "uint16", "uint32",
               "uint64", "int8", "int16", "int32", "int64", "min", "max", "clear", "print", "real", "close"}
# Local variable names used inside generated Go method bodies that parameter names must not shadow.
LOCALS = {"o", "e", "b", "cx", "r", "n", "src", "s", "slot", "gen", "ok", "c"}
# Uppercase Go initialisms preserved by `camel()` and `lower_camel()`.
INITIALISMS = {"id": "ID", "ids": "IDs", "url": "URL", "urls": "URLs", "uri": "URI", "json": "JSON", "html": "HTML",
               "http": "HTTP", "https": "HTTPS", "svg": "SVG", "css": "CSS", "api": "API", "ascii": "ASCII",
               "utf8": "UTF8"}
# Mapping from Rust integer primitive names to Go integer types (`usize`/`isize` map to Go `int`).
GO_INT = {"u8": "uint8", "u16": "uint16", "u32": "uint32", "u64": "uint64", "i8": "int8", "i16": "int16",
          "i32": "int32", "i64": "int64", "usize": "int", "isize": "int"}

# Hand-written Go value types wired directly in `gk_go.py` (additional libvals live in `gk_go_kinds.py`).
GO_LIBVALS = {"Drag_Value": "DragValue"}
# Hand-written Go plot part types that also implement the generated `PlotPart` marker interface.
GO_PLOT_OWN_PARTS = ("PlotPath", "PlotQuad")


class Unsup(Exception):
    """Raised when a descriptor cannot yet be mapped into Go; caught by `try_item()` to record unmapped reasons."""
    pass


def camel(name):
    """Convert a snake_case or mixed identifier into exported Go CamelCase with initialisms.

    Strips trailing underscores, expands `INITIALISMS`, and preserves an underscore between
    adjacent numeric segments (e.g. `2_1` stays `2_1`, not `21`).
    """
    name = name.rstrip("_")
    parts = [p for p in name.split("_") if p]
    out = ""
    for p in parts:
        low = p.lower()
        w = INITIALISMS.get(low) or (p[:1].upper() + p[1:])
        # Preserve an underscore between adjacent digits (for example 2_1 -> 2_1, not 21).
        if out and out[-1].isdigit() and w[:1].isdigit():
            out += "_"
        out += w
    return out or "X"


def lower_camel(name):
    """Convert an identifier into unexported Go lowerCamelCase, lowercasing leading initialisms (e.g. `IDFoo` -> `idFoo`)."""
    c = camel(name)
    for k, v in INITIALISMS.items():
        if c.startswith(v) and (len(c) == len(v) or not c[len(v)].islower()):
            return k + c[len(v):]
    return c[:1].lower() + c[1:]


def pname(name):
    """Sanitize a Rust parameter name for Go, avoiding keywords, predeclared names, digits-only names, and body locals."""
    if re.fullmatch(r"\d+", name or ""):
        name = "arg" + name
    n = lower_camel(snake(name)) if name else "arg"
    return n + "_" if n in GO_KEYWORDS or n in LOCALS else n


class Go:
    """Generates flat Go source files for all bound/bridged model items and reports Go coverage.

    Key internal accumulators shared across emission helpers:
    - `self.names`: `{go_ident: owner_key}` enforcing uniqueness across the entire flat package.
    - `self.claim_base`: `{go_ident: (unprefixed_base, crate)}` passed to `gk_split.Split.name_all()`
      so package-prefix stripping knows the original crate priority and unprefixed stem.
    - `self.files`: `{filename: source_text}` flat Go files written to `gen/out/build.go/flat/`.
    - `self.mapped` / `self.unmapped`: tracks every bound/bridged model item ID emitted vs skipped.
    - `self.need_readers` / `self.reader_src`: deferred `rd<Type>` wire decoders emitted only
      for types actually read back by calls, events, `Ask` keys, or `Items` rows.
    - `self.tuples` / `self.handles` / `self.oneofs`: on-demand auxiliary declarations synthesized
      whenever `gtype()` encounters a tuple, opaque handle, or `oneof` element union.
    """

    def __init__(self, m, outdir):
        self.m, self.out = m, outdir
        self.docs = self.doc_names = None
        self.names = {}
        self.claim_base = {}
        self.collisions = []
        self.files = {}
        self.lines = []
        self.file = None
        self.tuples = {}
        self.handles = {}
        self.unmapped = []
        self.mapped = set()
        self.in_call = False
        self.oneofs = {}
        self.readers = {}
        self.reader_src = {}
        self.need_readers = set()
        prefixes = m.ov.get("go", {}).get("enum_prefix", {})
        self.enum_prefix = prefixes
        hand = []
        for root, _, fns in sorted(os.walk(outdir)) if os.path.isdir(outdir) else []:
            hand += [os.path.join(root, fn) for fn in sorted(fns)]
        import gk_split
        legacy = gk_split.hand_decls()[1]
        back = {v: k for k, v in legacy.items()}
        # Reserve every exported identifier declared by hand-written Go files so generated
        # symbols automatically receive a crate prefix instead of colliding.
        for path in hand:
            fn = os.path.relpath(path, outdir)
            if fn.endswith(".go") and not os.path.basename(fn).startswith("gen_"):
                text = open(path).read()
                pkg = (re.search(r"(?m)^package (\w+)", text) or [None, ""])[1]
                found = re.findall(r"(?m)^(?:func|type|var|const)\s+([A-Z]\w*)", text)
                for block in re.findall(r"(?ms)^(?:const|var) \((.*?)^\)", text):
                    found += re.findall(r"(?m)^\t([A-Z]\w*)", block)
                for name in found:
                    self.names.setdefault(back.get((pkg, name), name), "hand " + fn)
        self.name_all()


    def claim(self, base, crate, key):
        """Claim a unique exported Go identifier in the flat namespace, adding a crate prefix or suffix on collision."""
        pre = {"gpui_base": "Base", "gpui": "Gpui", "gpui_component": "Kit", "gpui_kit": "Kit"}[crate]
        for c in (base, pre + base):
            if self.names.get(c, key) == key:
                self.names[c] = key
                self.claim_base[c] = (base, crate)
                return c
        n = 2
        while f"{pre}{base}{n}" in self.names:
            n += 1
        self.collisions.append((base, key, self.names.get(pre + base)))
        self.names[f"{pre}{base}{n}"] = key
        self.claim_base[f"{pre}{base}{n}"] = (base, crate)
        return f"{pre}{base}{n}"

    def name_all(self):
        """Assign deterministic Go identifiers to all nodes, types, enums, states, constructors, and shorthands.

        Crates are processed in priority order (`gpui_component`, `gpui_base`, `gpui`, `gpui_kit`)
        so higher-level kit components keep unprefixed names when base and component types share a stem.
        """
        m = self.m
        order = {"gpui_component": 0, "gpui_base": 1, "gpui": 2, "gpui_kit": 3}
        bound = lambda i: i["status"] in ("bound", "bridged")
        for key in sorted(m.nodes, key=lambda k: (order[m.nodes[k]["crate"]], k)):
            node = m.nodes[key]
            if "stem" in node:
                node["go"] = self.claim(camel(node["stem"]) + ("El" if node["element"] else ""), node["crate"], key)
        for table in (m.enums, m.denums, m.records):
            for rt in sorted(table, key=lambda k: (order[table[k]["crate"]], k)):
                t = table[rt]
                t["go"] = self.claim(camel(t["stem"]), t["crate"], rt)
        for key in sorted(m.states, key=lambda k: (order[m.states[k]["crate"]], k)):
            st = m.states[key]
            if "stem" in st:
                st["go"] = self.claim(camel(st["stem"]), st["crate"], "state " + key)
        for rt in sorted(m.intos):
            s = m.intos[rt]
            if "stem" in s:
                s["go"] = self.claim(camel(s["stem"]), "gpui_kit", "into " + rt)
        for rt in sorted(m.enums):
            e = m.enums[rt]
            pre = self.enum_prefix.get(e["name"], e["go"])
            for v in e["variants"]:
                v["go"] = self.claim(pre + camel(ada(v["name"])), e["crate"], f"{rt}::{v['name']}")
        for rt in sorted(m.denums):
            e = m.denums[rt]
            for v in e["variants"]:
                if v.get("ok"):
                    v["go"] = self.claim(e["go"] + camel(ada(v["name"])), e["crate"], f"{rt}::{v['name']}")
            if self.unit_values(e):
                e["go_kind"] = self.claim(e["go"] + "Kind", e["crate"], rt + " (unit variants)")
        for key in sorted(m.nodes, key=lambda k: (order[m.nodes[k]["crate"]], k)):
            node = m.nodes[key]
            for it in sorted(node["ctors"], key=lambda i: i["id"]):
                if not bound(it) or it["kind"] == "convert" or "stem" not in it:
                    continue
                base = camel(it["stem"]) if node["element"] else "New" + camel(it["stem"])
                it["go"] = self.claim(base, it["crate"], it["id"])
        for key in sorted(m.states, key=lambda k: (order[m.states[k]["crate"]], k)):
            st = m.states[key]
            for it in sorted(st.get("ctors", []), key=lambda i: i["id"]):
                if it["status"] == "bridged" and "stem" in it:
                    it["go"] = self.claim("New" + camel(it["stem"]), st["crate"], it["id"])
            for it in sorted(st.get("globals", []) + st.get("statics", []), key=lambda i: i["id"]):
                if it["status"] == "bridged" and "stem" in it:
                    it["go"] = self.claim(camel(it["stem"]), st["crate"], it["id"])
        for rt in sorted(m.records):
            rc = m.records[rt]
            if rc.get("short_stem"):
                rc["short_go"] = self.claim(camel(rc["shorthand"]), rc["crate"], "shorthand " + rt)


    def gtype(self, d, out=False):
        """Return the Go type expression for model descriptor `d` (`out=True` for Rust-to-Go reads)."""
        m, k = self.m, d[0]
        x = gk_go_kinds.gtype(self, d, out)
        if x is not None:
            return x
        if k == "dock":
            return gk_go_bridges.gtype(self, d, out)
        if k == "bool":
            return "bool"
        if k == "int":
            return GO_INT.get(d[1], "int")
        if k in ("f32", "f64"):
            return {"f32": "float32", "f64": "float64"}[k]
        if k == "str":
            return "string"
        if k == "leaf":
            if out and d[1] == "color":
                return "Hsla"
            if is_fill(d):
                return "Fill"
            t = {"px": "float32", "rems": "float32", "color": "Color", "length": "Length", "definite_length": "Length",
                 "absolute_length": "Length", "id": "ID", "duration": "time.Duration", "range": "Range",
                 "click_event": "ClickEvent", "key_event": "KeyEvent", "mouse_event": "MouseEvent"}.get(d[1])
            if t is None:
                raise Unsup(f"leaf {d[1]}")
            return t
        if k == "enum":
            return m.enums[d[1]]["go"]
        if k == "denum":
            return m.denums[d[1]]["go"]
        if k == "rec":
            return m.records[d[1]]["go"]
        if k == "into":
            if "go" not in m.intos[d[1]]:
                return self.gtype(m.intos[d[1]]["members"][0]["desc"], out)
            return m.intos[d[1]]["go"]
        if k in ("ref", "from", "refmut", "rcvec"):
            return self.gtype(d[1], out)
        if k == "opt":
            return f"Option[{self.gtype(d[1], out)}]"
        if k in ("list", "arcslice", "iter"):
            return "[]" + self.gtype(d[1], out)
        if k == "array":
            return f"[{d[2]}]{self.gtype(d[1], out)}"
        if k == "rangeof":
            return f"[2]{self.gtype(d[1], out)}"
        if k == "nonzero":
            return GO_INT.get(d[1], "int")
        if k == "char":
            return "rune"
        if k == "tuple":
            ts = [self.gtype(x, out) for x in d[1]]
            up = lambda t: re.sub(r"(^|[^A-Za-z0-9])([a-z])", lambda x: x.group(1) + x.group(2).upper(), t)
            name = "Tuple" + "".join(re.sub(r"\W+", "", up(t.replace("[]", "List "))) for t in ts)
            self.tuples[name] = (ts, d[1])
            return name
        if k in ("child", "fragment"):
            return "Element"
        if k in ("node", "build", "stylefn"):
            node = m.nodes[d[1]]
            if "go" not in node:
                raise Unsup(f"node {d[1]} has no Go type")
            return node["go"]
        if k in ("shim", "rows", "datum_rows"):
            return "RowSource"
        if k == "entity" and "go" in m.states.get(d[1], {}):
            return m.states[d[1]]["go"]
        if k == "obj":
            st = m.states.get(d[2], {})
            if "go" not in st:
                raise Unsup(f"object {d[2]} has no Go type")
            return st["go"]
        if k in ("entity", "handle"):
            name = camel(ada(d[2]))
            if self.names.get(name, "handle " + d[1]) != "handle " + d[1]:
                name += "Handle"
            self.names[name] = "handle " + d[1]
            self.handles[name] = d[1]
            return name
        if k == "action":
            return "ActionHandler" if d[1] == "listener" else "Action"
        if k == "handler":
            return self.handler_type(m.shapes[d[1]])
        if k == "result":
            return "error"
        if k == "viewfn":
            return "View"
        if k == "oneof":
            return self.oneof(d)
        if k == "panelview":
            import gk_overlay
            return self.gtype(["entity", gk_overlay.PANEL_RUST, gk_overlay.PANEL_NAME], out)
        if k == "data":
            return "Data"
        if k in ("field", "format"):
            return "string"
        if k == "viewbuild":
            node = m.nodes[d[1]]
            if "go" not in node:
                raise Unsup(f"viewbuild of {d[1]} without a Go type")
            return f"Built[{node['go']}]"
        if k == "libval" and d[2] in GO_LIBVALS:
            return GO_LIBVALS[d[2]]
        if k == "dragview":
            return "View"
        if k == "dragstyle":
            return self.gtype(["stylefn", d[1], d[2]], out)
        if k == "dropaccept":
            return "DropAccept"
        if k == "viewany":
            if len(d) > 1 and "go" in m.nodes.get(d[1][1], {}):
                return "ViewOrTooltip"
            return "View"
        if k in ("uri", "arcpath"):
            return "string"
        if k == "imagebytes":
            return "ImageBytes"
        if k in ("pixelimage", "customimage"):
            return "RenderImage"
        raise Unsup(k)

    @staticmethod
    def shape_args(sh):
        def fix(d):
            if isinstance(d, list) and d[:2] == ["int", "u64"]:
                return ["int", "usize"]
            return [fix(x) if isinstance(x, list) else x for x in d] if isinstance(d, list) else d
        return [fix(a) if "usize" in (r or "") else a for a, r in zip(sh["args"], sh.get("rust") or [""] * len(sh["args"]))]

    def event_type(self, sh):
        args = self.shape_args(sh)
        if not args:
            return None
        if len(args) == 1:
            return self.gtype(args[0], True)
        return f"Event{sh['num']}"

    def ret_type(self, sh):
        r = sh["ret"]
        if r in ("unit", None):
            return None
        if r == "bool":
            return "bool"
        if r == "str":
            return "string"
        raise Unsup(f"handler return {r}")

    def handler_type(self, sh):
        e, r = self.event_type(sh), self.ret_type(sh)
        return f"func(cx *Cx{', e ' + e if e else ''}){' ' + r if r else ''}"

    def oneof(self, d):
        name = camel(d[1]) + "Node"
        if d[1] not in self.oneofs:
            name = self.claim(name, "gpui_kit", "oneof " + d[1])
            self.oneofs[d[1]] = (name, [k for k in d[2] if "go" in self.m.nodes.get(k, {})])
        return self.oneofs[d[1]][0]


    def put(self, d, v, depth=0):
        """Return Go statements that encode Go expression `v` of descriptor `d` onto tape `o`."""
        m, k = self.m, d[0]
        x = gk_go_kinds.put(self, d, v, depth)
        if x is not None:
            return x
        if k == "dock":
            return gk_go_bridges.put(self, d, v)
        if k == "bool":
            return [f"o.bool({v})"]
        if k in ("int", "nonzero"):
            if k == "nonzero":
                return [f"putNonzero(o, uint64({v}), {8 if d[1] in ('u64', 'usize') else {'u8': 1, 'u16': 2, 'u32': 4}.get(d[1], 8)})"]
            if d[1] == "usize":
                return [f"putUsize(o, {v})"]
            w = INT_WIRE[d[1]]
            return [{"u8": f"o.u8(uint8({v}))", "i8": f"o.u8(uint8({v}))", "u16": f"o.u16(uint16({v}))",
                     "i16": f"o.u16(uint16({v}))", "u32": f"o.u32(uint32({v}))", "i32": f"o.u32(uint32({v}))",
                     "u64": f"o.u64(uint64({v}))", "i64": f"o.u64(uint64({v}))"}[w]]
        if k == "f32":
            return [f"o.f32({v})"]
        if k == "f64":
            return [f"o.f64({v})"]
        if k == "char":
            return [f"o.u32(uint32({v}))"]
        if k == "str":
            return [f"o.str({v})"]
        if k == "leaf":
            if is_fill(d):
                return [f"putFill(o, {v})"]
            r = {"px": f"o.f32({v})", "rems": f"o.f32({v})", "color": f"putColor(o, {v})",
                 "length": f"putLength(o, {v})", "definite_length": f"putDefiniteLength(o, {v})",
                 "absolute_length": f"putAbsoluteLength(o, {v})", "id": f"putID(o, {v})",
                 "duration": f"putMs(o, {v})", "range": f"putRange(o, {v})"}.get(d[1])
            if r is None:
                raise Unsup(f"leaf {d[1]} as an argument")
            return [r]
        if k == "enum":
            return [f"o.u16(uint16({v}))"]
        if k in ("denum", "rec", "into"):
            t = self.gtype(d)
            if k == "into" and "go" not in m.intos[d[1]]:
                return [f"o.u8(0)"] + self.put(m.intos[d[1]]["members"][0]["desc"], v, depth)
            if k == "rec" and not m.records[d[1]]["in"]:
                raise Unsup(f"record {d[1]} only travels out")
            if k == "denum" and not m.denums[d[1]]["in"]:
                raise Unsup(f"enum {d[1]} only travels out")
            return [f"put{t}(o, {v})"]
        if k in ("ref", "from", "refmut", "rcvec"):
            return self.put(d[1], v, depth)
        if k == "opt":
            x = f"x{depth}"
            return [f"if {x}, ok := {v}.Get(); ok {{", "\to.u8(1)"] + ["\t" + s for s in self.put(d[1], x, depth + 1)] + \
                ["} else {", "\to.u8(0)", "}"]
        if k in ("list", "arcslice", "iter"):
            x = f"x{depth}"
            return [f"o.u32(uint32(len({v})))", f"for _, {x} := range {v} {{"] + \
                ["\t" + s for s in self.put(d[1], x, depth + 1)] + ["}"]
        if k == "array":
            x = f"x{depth}"
            return [f"for _, {x} := range {v} {{"] + ["\t" + s for s in self.put(d[1], x, depth + 1)] + ["}"]
        if k == "rangeof":
            return self.put(d[1], f"{v}[0]", depth) + self.put(d[1], f"{v}[1]", depth)
        if k == "tuple":
            self.gtype(d)
            out = []
            for n, x in enumerate(d[1]):
                out += self.put(x, f"{v}.V{n}", depth)
            return out
        if k in ("child", "fragment"):
            return [f"o.u32(take({v}))"]
        if k in ("node", "build", "stylefn"):
            node = m.nodes[d[1]]
            self.gtype(d)
            if not node["element"]:
                if self.in_call:
                    return [f"putVtape(o, {v}.n)"]
                return [f"o.u32(takeValue({v}.n))"]
            return [f"o.u32(takeNode({v}.n, false))"]
        if k == "obj":
            self.gtype(d)
            if not self.in_call:
                return [f"putObj(o, {v}.h)"]
            return [f"o.u64(held({v}.h))"]
        if k in ("entity", "handle", "shim", "rows", "datum_rows", "panelview"):
            self.gtype(d)
            return [f"o.u64(held({v}.h))"]
        if k == "data":
            return [f"putData(o, {v})"]
        if k == "field":
            return [f"putField(o, {v}, data{camel(d[1])})"]
        if k == "format":
            nums = sum(1 for f in d[5] if f[0] in ("f32", "f64", "int"))
            row = "true" if any(f[0] == "datum" for f in d[5]) else "false"
            return [f"putFormat(o, {v}, {nums}, {row})"]
        if k == "action":
            return [f"putActionHandler(o, {v})" if d[1] == "listener" else f"putAction(o, {v})"]
        if k == "handler":
            sh = m.shapes[d[1]]
            self.handler_type(sh)
            if self.in_call:
                return [f"putKeptHandler(o, {v}, {sh['num']})"]
            return [f"o.u32(register({v}, {sh['num']}))"]
        if k == "viewfn":
            return [f"putViewArg(o, {v})"]
        if k == "oneof":
            self.oneof(d)
            return [f"o.u32(take({v}))"]
        if k == "viewbuild":
            X = self.gtype(d)[len("Built["):-1]
            return [f"putBuilt(o, builtView[{X}](cx, {v}))"]
        if k == "libval" and d[2] in GO_LIBVALS:
            return [f"put{GO_LIBVALS[d[2]]}(o, {v})"]
        if k == "dragview":
            return [f"putViewArg(o, {v})"]
        if k == "dragstyle":
            return self.put(["stylefn", d[1], d[2]], v, depth)
        if k == "dropaccept":
            return [f"putDropAccept(o, {v})"]
        if k == "viewany":
            if self.gtype(d) == "View":
                return [f"putViewArg(o, {v})"]
            return [f"putViewOrTooltip(o, {v})"]
        if k in ("uri", "arcpath"):
            return [f"o.str({v})"]
        if k == "imagebytes":
            return self.put(["enum", d[1]], f"{v}.Format", depth) + [f"putBytes(o, {v}.Bytes)"]
        if k in ("pixelimage", "customimage"):
            return [f"putRenderImage(o, {v})"]
        raise Unsup(k)


    def rd(self, d, out=True):
        """Return a Go expression that decodes descriptor `d` from wire reader `r`."""
        m, k = self.m, d[0]
        x = gk_go_kinds.rd(self, d, out)
        if x is not None:
            return x
        if k == "dock":
            return gk_go_bridges.rd(self, d, out)
        if k == "bool":
            return "r.bool()"
        if k in ("int", "nonzero"):
            w = INT_WIRE.get(d[1], "u64")
            base = {"u8": "r.u8()", "i8": "r.u8()", "u16": "r.u16()", "i16": "r.u16()", "u32": "r.u32()",
                    "i32": "r.u32()", "u64": "r.u64()", "i64": "r.u64()"}[w]
            return f"{GO_INT.get(d[1], 'int')}({base})"
        if k == "f32":
            return "r.f32()"
        if k == "f64":
            return "r.f64()"
        if k == "char":
            return "rune(r.u32())"
        if k == "str":
            return "r.str()"
        if k == "leaf":
            r = {"color": "rdHsla(r)" if out else "rdHsla(r).Color()", "click_event": "rdClickEvent(r)", "key_event": "rdKeyEvent(r)",
                 "mouse_event": "rdMouseEvent(r)", "px": "r.f32()", "rems": "r.f32()", "range": "rdRange(r)",
                 "duration": "rdDuration(r)"}.get(d[1])
            if r is None:
                raise Unsup(f"leaf {d[1]} read back")
            return r
        if k == "enum":
            return f"{m.enums[d[1]]['go']}(r.u16())"
        if k in ("denum", "rec"):
            t = self.gtype(d, out)
            self.need_readers.add((k, d[1]))
            return f"rd{t}(r)"
        if k in ("ref", "from", "refmut", "rcvec"):
            return self.rd(d[1], out)
        if k == "opt":
            return f"rdOpt(r, func(r *reader) {self.gtype(d[1], out)} {{ return {self.rd(d[1], out)} }})"
        if k in ("list", "arcslice", "iter"):
            return f"rdList(r, func(r *reader) {self.gtype(d[1], out)} {{ return {self.rd(d[1], out)} }})"
        if k == "array":
            t = self.gtype(d, out)
            return f"func() (a {t}) {{ for i := range a {{ a[i] = {self.rd(d[1], out)} }}; return }}()"
        if k == "rangeof":
            return f"{self.gtype(d, out)}{{{self.rd(d[1], out)}, {self.rd(d[1], out)}}}"
        if k == "tuple":
            t = self.gtype(d, out)
            return f"{t}{{" + ", ".join(f"V{n}: {self.rd(x, out)}" for n, x in enumerate(d[1])) + "}"
        if k in ("entity", "handle", "shim", "rows", "datum_rows", "obj"):
            return f"{self.gtype(d, out)}{{h: r.u64()}}"
        if k == "action":
            if d[1] == "listener":
                raise Unsup("action listener read back")
            return "rt.MkAction(r.u32())"
        if k == "result":
            return "rdResult(r)"
        if k == "libval" and d[2] in GO_LIBVALS:
            return f"rd{GO_LIBVALS[d[2]]}(r)"
        raise Unsup(f"{k} read back")


    def open(self, name, doc, imports=()):
        """Begin accumulating lines for a generated flat Go file."""
        self.file = name
        self.lines = ["// Code generated by gen/gkgen.py (gen/gk_go.py). DO NOT EDIT.", "", "// " + doc, "", "package gk", ""]
        self.imports = set(imports)

    def add(self, *lines):
        """Append lines to the currently open generated Go file."""
        self.lines += lines

    def close(self):
        """Finalize imports (adding `time` if `time.*` is referenced) and store the file in `self.files`."""
        text = "\n".join(self.lines) + "\n"
        imps = set(self.imports)
        if re.search(r"\btime\.[A-Z]", text):
            imps.add("time")
        if imps:
            text = text.replace("package gk\n", "package gk\n\nimport (\n" + "".join(f'\t"{i}"\n' for i in sorted(imps)) + ")\n", 1)
        self.files[self.file] = text

    def try_item(self, it, fn):
        """Run emitter thunk `fn()` for item `it`, rolling back side tables if `Unsup` is raised."""
        try:
            saved = (dict(self.tuples), set(self.need_readers))
            lines = fn()
        except Unsup as e:
            self.tuples, self.need_readers = dict(saved[0]), set(saved[1])
            self.unmapped.append((it, str(e)))
            return []
        self.mapped.add(it["id"])
        return lines

    def params(self, it, extra=(), do=False):
        """Format Go parameter declarations for item `it`, omitting Rust-only `ctx` parameters."""
        ps = []
        at = self.do_handler(it) if do else None
        for i, a in enumerate(it["args"]):
            if a["desc"][0] == "ctx":
                continue
            ps.append(f"{pname(a['name'])} {'func(cx *Cx)' if i == at else self.ptype(a['desc'])}")
        return list(extra) + ps

    # Single-event handler payload types that also get an event-free `*Do(func(cx *Cx))` convenience method.
    DO_EVENTS = {"ClickEvent", "MouseEvent", "MouseExitEvent"}

    def do_handler(self, it):
        """Return the arg index of a single void handler taking a `DO_EVENTS` payload, or None."""
        hs = [i for i, a in enumerate(it["args"]) if a["desc"][0] == "handler"]
        if len(hs) != 1:
            return None
        sh = self.m.shapes.get(it["args"][hs[0]]["desc"][1])
        if not sh or self.ret_type(sh) is not None or len(self.shape_args(sh)) != 1:
            return None
        try:
            ev = self.event_type(sh)
        except Unsup:
            return None
        return hs[0] if ev in self.DO_EVENTS else None

    def shorthand(self, d):
        how = "one"
        while d[0] in ("ref", "from", "refmut"):
            d = d[1]
        if d[0] in ("opt", "list"):
            how, d = d[0], d[1]
            while d[0] in ("ref", "from"):
                d = d[1]
        if d[0] == "rec" and self.m.records[d[1]].get("short_go"):
            return self.m.records[d[1]], how
        return None

    def ptype(self, d):
        return self.gtype(d)

    def body(self, it):
        out = []
        for a in it["args"]:
            if a["desc"][0] != "ctx":
                out += self.put(a["desc"], pname(a["name"]))
        return out

    def doc(self, it, name):
        """Format a Go doc comment combining upstream rustdoc text (if any) with the Rust target."""
        head = f"// {name} calls {it.get('callee') or it['id']}."
        body = self.docs.comment(it.get("doc"), "go", self.doc_names, it["id"]) if self.docs else []
        if not body:
            return head
        return "\n".join([("// " + x).rstrip() for x in body] + ["//", head])


    def emit(self):
        """Emit all flat Go files plus s/, palette/, and theme/ token packages."""
        m = self.m
        bound = lambda i: i["status"] in ("bound", "bridged")
        groups = {}
        for key in sorted(m.nodes):
            node = m.nodes[key]
            if "go" in node and any(bound(i) for i in node["ctors"]):
                fname = f"gen_{SHORT[node['crate']]}_{re.sub(r'[^a-z0-9]+', '_', node['module'].lower()) or 'root'}"
                groups.setdefault(fname, []).append(node)
        trait_fns = {}
        for fname in sorted(groups):
            self.open(fname + ".go", "Generated element and value builders for one upstream module.")
            for node in groups[fname]:
                self.node(node, trait_fns)
            self.close()
        self.open("gen_traits.go", "Shared helper functions for builder-trait methods.")
        for name in sorted(trait_fns):
            self.add(*trait_fns[name])
        self.close()
        self.open("gen_state.go", "Handle types and methods for runtime-owned kit states and library objects.")
        for key in sorted(m.states, key=lambda k: m.states[k].get("go", "")):
            if "go" in m.states[key]:
                self.in_call = True
                try:
                    self.state(m.states[key])
                finally:
                    self.in_call = False
        self.close()
        self.ambient()
        self.plot_parts()
        gk_go_kinds.emit_files(self)
        gk_go_bridges.emit(self)
        self.types()
        self.tokens()
        for name, text in self.files.items():
            if "/" in name:
                path = os.path.join(self.out, name)
                os.makedirs(os.path.dirname(path), exist_ok=True)
                if not os.path.exists(path) or open(path).read() != text:
                    open(path, "w").write(text)


    def node(self, node, trait_fns):
        """Emit Go struct, constructors, inherent builder ops, conversions, and trait methods for one node.

        Trait methods delegate to shared `t<Trait><Method>` helpers in `gen_traits.go` so
        the tape-encoding logic is emitted once per trait method rather than duplicated
        across every element type.
        """
        m = self.m
        bound = lambda i: i["status"] in ("bound", "bridged")
        T = node["go"]
        el = node["element"]
        # Format `// <T> is <path>.` is parsed by gates/conformance.py and gen/gk_apidoc.py.
        self.add(f"// {T} is {node['path']}.", f"type {T} struct{{ n *node }}", "")
        if el:
            self.add(f"func (e {T}) node() *node {{ return e.n }}", "")
        else:
            self.add(f"func (e {T}) valueNode() *node {{ return e.n }}", "")
        for it in sorted([i for i in node["ctors"] if bound(i) and i["kind"] != "convert" and "go" in i], key=lambda i: i["go"]):
            def ctor(it=it):
                ps = self.params(it)
                variadic = bool(it["args"]) and it["args"][-1]["desc"][0] == "list" and it["args"][-1]["desc"][1] == ["child"]
                if variadic:
                    ps[-1] = ps[-1].replace(" []Element", " ...Element")
                styled = node["styled"] and not variadic
                if styled:
                    ps.append("st ...Style")
                lines = [self.doc(it, it["go"]), f"func {it['go']}({', '.join(ps)}) {T} {{",
                         f"\tn := {'newNode' if el else 'newValue'}({it['op']})"]
                body = self.body(it)
                if body:
                    lines += ["\to := nodeArgs(n)"] + ["\t" + s for s in body]
                if styled:
                    lines.append("\ttokens(n, st)")
                lines += [f"\treturn {T}{{n}}", "}", ""]
                return lines
            self.add(*self.try_item(it, ctor))
        seen = {"node", "valueNode"}
        allnames = {camel(i["name"]) for i in node.get("ops", []) + node.get("converts", [])}
        for tk in node["tables"]:
            for t in self.m.traits.values():
                if t["key"] == tk:
                    allnames |= {camel(i["name"]) for i in t["ops"]}
        ops = sorted([i for i in node.get("ops", []) if bound(i)], key=lambda i: i["name"])
        for it in ops:
            name = camel(it["name"])
            if name in seen:
                continue

            def meth(it=it, name=name):
                ps = self.params(it)
                body = self.body(it)
                lines = [self.doc(it, name), f"func (e {T}) {name}({', '.join(ps)}) {T} {{"]
                lines += ([f"\to := op(e.n, {it['op']})"] + ["\t" + s for s in body]) if body else [f"\top(e.n, {it['op']})"]
                lines += ["\treturn e", "}", ""]
                if self.do_handler(it) is not None and name + "Do" not in allnames:
                    lines += [f"// {name}Do is the event-free convenience variant of {name}.",
                              f"func (e {T}) {name}Do({', '.join(self.params(it, do=True))}) {T} {{",
                              f"\to := op(e.n, {it['op']})"] + ["\t" + s for s in body] + ["\treturn e", "}", ""]
                return lines
            lines = self.try_item(it, meth)
            if lines:
                seen.add(name)
                if self.do_handler(it) is not None:
                    seen.add(name + "Do")
                self.add(*lines)
        convs = sorted([i for i in node.get("converts", []) if bound(i)], key=lambda i: (i["trait"] or "", i["name"]))
        for it in convs:
            name = camel(it["name"])
            target = m.nodes.get(it["node"], {})
            if name in seen or "go" not in target:
                if "go" not in target:
                    self.unmapped.append((it, "convert target without a Go type"))
                continue

            def conv(it=it, name=name, target=target):
                U = target["go"]
                ps = self.params(it)
                # Leading `// <name> ` is rewritten by gk_split.lifted() if this method is lifted cross-package.
                lines = [f"// {name} calls {it.get('callee') or it['id']} and converts the element into {U}.",
                         f"func (e {T}) {name}({', '.join(ps)}) {U} {{", "\tsrc := takeNode(e.n, true)",
                         f"\tn := {'newNode' if target['element'] else 'newValue'}({it['op']})", "\to := nodeArgs(n)", "\to.u32(src)"]
                lines += ["\t" + s for s in self.body(it)] + [f"\treturn {U}{{n}}", "}", ""]
                return lines
            lines = self.try_item(it, conv)
            if lines:
                seen.add(name)
                self.add(*lines)
        for tk in node["tables"]:
            t = next(t for t in m.traits.values() if t["key"] == tk)
            if "stem" not in t:
                continue
            tl = camel(t["low"])
            if t["name"] == "Styled" and "Style" not in seen:
                seen.add("Style")
                self.add(f"// Style appends nullary style tokens (such as s.Gap2 or s.SizeFull).", f"func (e {T}) Style(st ...Style) {T} {{ tokens(e.n, st); return e }}", "")
            if t["name"] == "ParentElement" and el:
                if "Text" not in seen:
                    seen.add("Text")
                    self.add("// Text appends a plain text child to the element.",
                             f"func (e {T}) Text(s string) {T} {{ putText(e.n, s); return e }}", "")
                if "ChildView" not in seen:
                    seen.add("ChildView")
                    self.add(f"// ChildView attaches a child view slot so the child can re-render independently.",
                             f"func (e {T}) ChildView(v View) {T} {{ slotView(e.n, v); return e }}", "")
            for it in sorted([i for i in t["ops"] if bound(i)], key=lambda i: i["name"]):
                name = camel(it["name"])
                if name in seen:
                    continue
                fn = f"t{tl}{name}"
                runtime_child = it.get("runtime_op")
                if fn not in trait_fns:
                    def tfn(it=it, fn=fn):
                        ps = self.params(it, ["n *node"])
                        if runtime_child:
                            a = pname(it["args"][0]["name"])
                            if it["args"][0]["desc"][0] == "list":
                                ps[-1] = ps[-1].replace(" []Element", " ...Element")
                                return [f"func {fn}({', '.join(ps)}) {{", f"\tfor _, c := range {a} {{", "\t\tattach(n, c)", "\t}", "}", ""]
                            return [f"func {fn}({', '.join(ps)}) {{", f"\tattach(n, {a})", "}", ""]
                        body = self.body(it)
                        # Format parsed by gen/gk_apidoc.py (GO_TRAITFN).
                        lines = [f"// {fn}: {it['id']}.", f"func {fn}({', '.join(ps)}) {{"]
                        lines += ([f"\to := op(n, {it['op']})"] + ["\t" + s for s in body]) if body else [f"\top(n, {it['op']})"]
                        return lines + ["}", ""]
                    lines = self.try_item(it, tfn)
                    trait_fns[fn] = lines
                if not trait_fns[fn]:
                    continue
                seen.add(name)
                ps = self.params(it)
                args = [pname(a["name"]) for a in it["args"] if a["desc"][0] != "ctx"]
                if runtime_child and it["args"][0]["desc"][0] == "list":
                    ps[-1] = ps[-1].replace(" []Element", " ...Element")
                    args[-1] += "..."
                self.add(self.doc(it, name),
                         f"func (e {T}) {name}({', '.join(ps)}) {T} {{ {fn}({', '.join(['e.n'] + args)}); return e }}")
                if not runtime_child and self.do_handler(it) is not None and name + "Do" not in seen and name + "Do" not in allnames:
                    fdo = fn + "Do"
                    if fdo not in trait_fns:
                        dps = self.params(it, ["n *node"], do=True)
                        dbody = self.body(it)
                        trait_fns[fdo] = [f"// {fdo}: event-free helper for {it['id']}.", f"func {fdo}({', '.join(dps)}) {{",
                                          f"\to := op(n, {it['op']})"] + ["\t" + s for s in dbody] + ["}", ""]
                    seen.add(name + "Do")
                    self.add(f"// {name}Do is the event-free convenience variant of {name}.",
                             f"func (e {T}) {name}Do({', '.join(self.params(it, do=True))}) {T} {{ {fdo}({', '.join(['e.n'] + args)}); return e }}")
            self.add("")
        if node.get("oneof_of"):
            pass


    def state(self, st):
        """Emit Go handle type, read/write/call/event methods, optional builder chain, and ctors for `st`."""
        m = self.m
        ok = lambda i: i["status"] == "bridged"
        S = st["go"]
        obj = st.get("obj", False)
        get = "objGet" if obj else "stateGet"
        what = "a runtime-owned library object" if obj else "a runtime-owned kit state entity"
        # Format parsed by gen/gk_apidoc.py (GO_DECL).
        self.add(f"// {S} is a handle to {st['path']}, {what}.", f"type {S} struct{{ h uint64 }}", "",
                 f"func (s {S}) handle() uint64 {{ return s.h }}", "")
        reads = sorted([i for i in st["reads"] if ok(i)], key=lambda i: i["name"])
        writes = sorted([i for i in st["writes"] if ok(i)], key=lambda i: i["name"])
        calls = sorted([i for i in st.get("calls", []) if ok(i)], key=lambda i: i["name"])
        builders = sorted([i for i in st["builders"] if ok(i)], key=lambda i: i["name"])
        want = set(m.ov.get("state", {}).get("try_reads", []))
        names = set()
        head = lambda it: [f"\to := newReq({it['op']})", "\to.u64(s.h)"]
        for it in reads:
            name = camel(it["name"])
            if name in names:
                continue

            def rd(it=it, name=name):
                R = self.gtype(it["ret"], True)
                ex = self.rd(it["ret"])
                lines = [self.doc(it, name), f"func (s {S}) {name}({', '.join(self.params(it, ['cx *Cx']))}) {R} {{"] + head(it)
                lines += ["\t" + x for x in self.body(it)] + [f"\tr := {get}(s.h, cx, o)", f"\treturn {ex}", "}", ""]
                if it["name"] in want and not obj:
                    lines += [f"// Try{name} is a non-blocking variant of {name} that returns ok=false if the state is currently borrowed.",
                              f"func (s {S}) Try{name}({', '.join(self.params(it, ['cx *Cx']))}) (v {R}, ok bool) {{"] + head(it)
                    lines += ["\t" + x for x in self.body(it)] + ["\tr, ok := stateTry(s.h, cx, o)", "\tif !ok {", "\t\treturn v, false", "\t}",
                                                                  f"\treturn {ex}, true", "}", ""]
                return lines
            lines = self.try_item(it, rd)
            if lines:
                names.add(name)
                self.add(*lines)
        for it in calls:
            name = camel(it["name"])
            if name in names:
                continue

            def cl(it=it, name=name):
                R = self.gtype(it["ret"], True)
                lines = [self.doc(it, name), f"func (s {S}) {name}({', '.join(self.params(it, ['cx *Cx']))}) {R} {{"] + head(it)
                lines += ["\t" + x for x in self.body(it)] + ["\tr := stateCall(s.h, cx, o)", f"\treturn {self.rd(it['ret'])}", "}", ""]
                return lines
            lines = self.try_item(it, cl)
            if lines:
                names.add(name)
                self.add(*lines)
        for it in writes:
            name = camel(it["name"])
            if name in names:
                continue

            def wr(it=it, name=name):
                lines = [self.doc(it, name), f"func (s {S}) {name}({', '.join(self.params(it, ['cx *Cx']))}) {{"] + head(it)
                return lines + ["\t" + x for x in self.body(it)] + ["\tstateSet(cx, o)", "}", ""]
            lines = self.try_item(it, wr)
            if lines:
                names.add(name)
                self.add(*lines)
        for ev in st["events"]:
            name = "On" + camel(snake(ev["name"]))
            if name in names:
                continue

            def sub(ev=ev, name=name):
                sh = m.shapes[ev["shape"]]
                H = self.handler_type(sh)
                return [f"// {name} registers a listener for {ev['name']} events on this state; release the returned Subscription to unsubscribe.",
                        f"func (s {S}) {name}(cx *Cx, h {H}) Subscription {{",
                        f"\tslot, gen := keepHandler(cx, h, {sh['num']})", f"\to := newReq({ev['op']})", "\to.u64(s.h)",
                        "\to.u32(slot)", "\to.u32(gen)", f"\tr := stateGet(s.h, cx, o)", "\treturn rt.MkSubscription(r.u64(), slot)", "}", ""]
            lines = self.try_item(ev, sub)
            if lines:
                names.add(name)
                self.add(*lines)
        B = S + "Builder" if builders else S
        if builders:
            self.add(f"// {B} accumulates chained {st['name']} builder options before creating the handle in Build.",
                     f"type {B} struct{{ sb *stateBuilder }}", "")
            for it in builders:
                name = camel(it["name"])

                def bl(it=it, name=name):
                    body = self.body(it)
                    lines = [self.doc(it, name),
                             f"func (b {B}) {name}({', '.join(self.params(it))}) {B} {{",
                             f"\to := b.sb.op({it['op']})" if body else f"\tb.sb.op({it['op']})"]
                    return lines + ["\t" + x for x in body] + ["\treturn b", "}", ""]
                self.add(*self.try_item(it, bl))
            self.add(f"// Build commits the queued options and returns the new {st['name']} handle.", f"func (b {B}) Build() {S} {{ return {S}{{HCore: rt.MkH(b.sb.build())}} }}", "")
        for it in sorted([i for i in st["ctors"] if ok(i) and "go" in i], key=lambda i: i["go"]):
            def ct(it=it):
                lines = [self.doc(it, it["go"]), f"func {it['go']}({', '.join(self.params(it, ['cx *Cx']))}) {B} {{",
                         f"\to := newReq({it['op']})"]
                lines += ["\t" + x for x in self.body(it)]
                if builders:
                    lines += ["\treturn " + B + "{newStateBuilder(cx, o)}", "}", ""]
                else:
                    lines += ["\to.u16(0) // zero chained builder ops", f"\treturn {S}{{HCore: rt.MkH(stateMake(cx, o))}}", "}", ""]
                return lines
            self.add(*self.try_item(it, ct))
        for it in sorted([i for i in st.get("globals", []) + st.get("statics", []) if ok(i) and "go" in i], key=lambda i: i["go"]):
            def gl(it=it):
                lines = [self.doc(it, it["go"])]
                hd = [f"\to := newReq({it['op']})"]
                body = ["\t" + x for x in self.body(it)]
                if it["kind"] == "obj-global":
                    return lines + [f"func {it['go']}({', '.join(self.params(it, ['cx *Cx']))}) {S} {{"] + hd + body + \
                        ["\tr := objGet(0, cx, o)", f"\treturn {S}{{h: r.u64()}}", "}", ""]
                if it["kind"] == "obj-scall":
                    R = self.gtype(it["ret"], True)
                    return lines + [f"func {it['go']}({', '.join(self.params(it, ['cx *Cx']))}) {R} {{"] + hd + body + \
                        ["\tr := stateCall(0, cx, o)", f"\treturn {self.rd(it['ret'])}", "}", ""]
                return lines + [f"func {it['go']}({', '.join(self.params(it, ['cx *Cx']))}) {{"] + hd + body + ["\tstateSet(cx, o)", "}", ""]
            self.add(*self.try_item(it, gl))


    def plot_parts(self):
        """Emit `gen_plot.go` declaring `PlotPart` and implementing it for paintable value nodes."""
        m = self.m
        bound = lambda i: i["status"] in ("bound", "bridged")
        nodes = sorted((n for n in m.nodes.values() if n.get("paint") and n["paint"]["status"] == "bridged"
                        and any(bound(i) for i in n["ctors"]) and "go" in n), key=lambda n: n["go"])
        if not nodes:
            return
        self.open("gen_plot.go", "PlotPart marker implementations for all paintable value nodes.")
        self.add("// PlotPart marks a value accepted by Plot for painting in order: " +
                 ", ".join(sorted([n["go"] for n in nodes] + list(GO_PLOT_OWN_PARTS))) + ".",
                 "type PlotPart interface {", "\tvalueNode() *node", "\tisPlotPart()", "}", "")
        for n in nodes:
            self.add(f"func ({n['go']}) isPlotPart() {{}}")
            self.mapped.add(n["paint"]["id"])
        for n in GO_PLOT_OWN_PARTS:
            self.add(f"func ({n}) isPlotPart() {{}}")
        self.add("")
        self.close()
        for it in m.all_items():
            if bound(it) and it["id"].startswith("gpui_kit::base::plot::Plot::"):
                self.mapped.add(it["id"])


    def ambient(self):
        """Emit `gen_ambient.go` with top-level functions for immediate ambient calls."""
        import gk_app
        items = sorted([i for i in gk_app.bound(self.m) if "stem" in i], key=lambda i: i["stem"])
        self.open("gen_ambient.go", "Immediate ambient calls backed by upstream free functions and Window/App extension traits.")
        self.in_call = True
        try:
            for it in items:
                it["go"] = self.claim(camel(it["stem"]), it["crate"], it["id"])

                def amb(it=it):
                    ps = self.params(it, ["cx *Cx"])
                    tps = ""
                    vb = [a for a in it["args"] if a["desc"][0] == "viewbuild"]
                    if vb:
                        X = self.m.nodes[vb[0]["desc"][1]]["go"]
                        tps = f"[B Builder[{X}]]"
                        n = pname(vb[0]["name"])
                        ps = [f"{n} B" if q == f"{n} Built[{X}]" else q for q in ps]
                    if it.get("ret") is None:
                        lines = [self.doc(it, it["go"]), f"func {it['go']}{tps}({', '.join(ps)}) {{"]
                    else:
                        lines = [self.doc(it, it["go"]), f"func {it['go']}{tps}({', '.join(ps)}) {self.gtype(it['ret'], True)} {{"]
                    lines += [f"\to := newReq({it['op']})"] + ["\t" + x for x in self.body(it)]
                    if it.get("ret") is None:
                        return lines + ["\tambientCall(cx, o, false)", "}", ""]
                    return lines + ["\tr := ambientCall(cx, o, true)", f"\treturn {self.rd(it['ret'])}", "}", ""]
                self.add(*self.try_item(it, amb))
        finally:
            self.in_call = False
        self.close()


    def types(self):
        """Emit `gen_types.go` with unit enums, records, data enums, Into sets, handler codecs, tuples, handles, and readers."""
        m = self.m
        self.open("gen_types.go", "Generated enums, records, tagged unions, Into interfaces, handles, and handler shape codecs.")
        for rt in sorted(m.enums, key=lambda k: m.enums[k]["go"]):
            e = m.enums[rt]
            E = e["go"]
            self.add(f"// {E} is {rt}.", f"type {E} uint16", "", f"// {E} enum values.", "const (")
            for v in e["variants"]:
                self.add(f"\t{v['go']} {E} = {v['num']}")
            self.add(")", "")
        for rt in sorted(m.records, key=lambda k: m.records[k]["go"]):
            self.record(rt, m.records[rt])
        for rt in sorted(m.denums, key=lambda k: m.denums[k]["go"]):
            self.denum(rt, m.denums[rt])
        for rt in sorted(m.intos, key=lambda k: m.intos[k].get("go", "")):
            if "go" in m.intos[rt]:
                self.into(rt, m.intos[rt])
        reg = []
        for key in sorted(m.shapes, key=lambda k: m.shapes[k]["num"]):
            sh = m.shapes[key]
            try:
                H = self.handler_type(sh)
                args = []
                sargs = self.shape_args(sh)
                if len(sargs) > 1:
                    S = f"Event{sh['num']}"
                    fields = [f"\tA{n} {self.gtype(a, True)}" for n, a in enumerate(sargs)]
                    exprs = [self.rd(a) for a in sargs]
                    self.add(f"// {S} holds the multi-argument payload for handler shape {sh['num']} ({', '.join(sh['rust'])}).",
                             f"type {S} struct {{", *fields, "}", "")
                    args = [f"{S}{{" + ", ".join(exprs) + "}"]
                elif sargs:
                    args = [self.rd(sargs[0])]
                R = self.ret_type(sh)
                callexpr = f"f({', '.join(['cx'] + args)})"
                if R == "bool":
                    callexpr = f"ret.bool({callexpr})"
                elif R == "string":
                    callexpr = f"ret.str({callexpr})"
                reg += ["func init() {", f'\trt.RegisterShape({sh["num"]}, rt.ShapeInfo{{Name: "{H}", Call: func(h any, cx *Cx, r *reader, ret *tape) bool {{']
                reg += [f"\t\tf, ok := h.({H})", "\t\tif !ok {", "\t\t\treturn false", "\t\t}", f"\t\t{callexpr}", "\t\treturn true", "\t}})", "}", ""]
                sh["go_ok"] = True
            except Unsup as e:
                sh["go_ok"] = False
                sh["go_why"] = str(e)
        self.add(*reg)
        for name, (ts, ds) in sorted(self.tuples.items()):
            self.add(f"// {name} holds a {len(ts)}-element tuple ({', '.join(ts)}).",
                     f"type {name} struct {{", *[f"\tV{n} {t}" for n, t in enumerate(ts)], "}", "")
        for name, rt in sorted(self.handles.items()):
            self.add(f"// {name} is an opaque runtime handle to {rt}.", f"type {name} struct{{ h uint64 }}", "")
        import gk_comp6
        tip = gk_comp6.tooltip_key(m)
        if tip is not None and "go" in m.nodes[tip]:
            self.add(f"// {m.nodes[tip]['go']} satisfies ViewOrTooltip so it can be passed directly to Tooltip builders.",
                     f"func ({m.nodes[tip]['go']}) isViewOrTooltip() {{}}", "")
        import gk_values
        anim, _ = gk_values.animatable(m)
        anim = [m.nodes[k]["go"] for k, _ in anim if "go" in m.nodes[k]]
        self.add("// AnimatableElement marks styled elements accepted by Motion and Effect wrappers.",
                 "type AnimatableElement interface {", "\tElement", "\tisAnimatable()", "}", "")
        for name in anim:
            self.add(f"func ({name}) isAnimatable() {{}}")
        self.add("")
        for trait, (iface, nodes) in sorted(self.oneofs.items()):
            self.add(f"// {iface} marks element nodes implementing the upstream {trait} trait.", f"type {iface} interface {{", "\tElement", f"\tis{iface}()", "}", "")
            for k in nodes:
                self.add(f"func ({self.m.nodes[k]['go']}) is{iface}() {{}}")
            self.add("")
        done = set()
        while self.need_readers - done:
            for k, rt in sorted(self.need_readers - done):
                done.add((k, rt))
                self.add(*self.reader(k, rt))
        for name, (ts, ds) in sorted(self.tuples.items()):
            pass
        self.close()

    def record(self, rt, rc):
        """Emit Go struct, tape encoder `put<R>`, and optional integer shorthand for record `rc`."""
        R = rc["go"]
        fname = lambda f: ("V" + f["name"]) if rc["tuple"] else camel(f["name"])
        try:
            fields = [f"\t{fname(f)} {self.gtype(f['desc'], not rc['in'])}" for f in rc["fields"]]
        except Unsup as e:
            rc["go_bad"] = str(e)
            self.add(f"// {R} mirrors {rt} (placeholder; fields not yet wired in Go: {e}).", f"type {R} struct{{}}", "")
            if rc["in"]:
                self.add(f"func put{R}(o *tape, v {R}) {{ stop(\"{R} is not carried by the Go package yet: leave out the builder that takes it (docs/API-GO.md, section Not in Go yet)\") }}", "")
            return
        self.add(f"// {R} mirrors {rt}." + ("" if rc["complete"] else " Includes only wire-supported fields."),
                 f"type {R} struct {{", *fields, "}", "")
        if rc["in"]:
            lines = [f"func put{R}(o *tape, v {R}) {{"]
            try:
                for f in rc["fields"]:
                    lines += ["\t" + s for s in self.put(f["desc"], f"v.{fname(f)}")]
            except Unsup as e:
                lines = [f"func put{R}(o *tape, v {R}) {{", f"\tstop(\"{R} cannot be written by the Go package yet ({e}): leave out the builder that takes it (docs/API-GO.md, section Not in Go yet)\")"]
            self.add(*lines, "}", "")
        if rc.get("short_go"):
            f = camel(rc["shorthand"])
            self.add(f"// {rc['short_go']} constructs a {R} with {f} set and all other fields zero.",
                     f"func {rc['short_go']}({lower_camel(f)} int) {R} {{ return {R}{{{f}: {lower_camel(f)}}} }}", "",
                     f"// No{rc['short_go']} represents an unset optional {R}.",
                     f"var No{rc['short_go']} = Option[{R}]{{}}", "")

    def reader(self, k, rt):
        """Emit a `rd<T>(r *reader)` decoder function for a record or data enum read from Rust."""
        m = self.m
        if k == "rec":
            rc = m.records[rt]
            R = rc["go"]
            fname = lambda f: ("V" + f["name"]) if rc["tuple"] else camel(f["name"])
            try:
                body = [f"\tv.{fname(f)} = rdColorField(r)" if "put_color_field" in gk_values.rust_field_put(rc, f)
                        else f"\tv.{fname(f)} = {self.rd(f['desc'], not rc['in'])}" for f in rc["fields"]]
            except Unsup as e:
                body = [f"\tstop(\"{R} cannot be read by the Go package yet ({e}): leave out the call that returns it (docs/API-GO.md, section Not in Go yet)\")"]
            return [f"func rd{R}(r *reader) (v {R}) {{", *body, "\treturn", "}", ""]
        e = m.denums[rt]
        E = e["go"]
        units = self.unit_values(e)
        lines = [f"func rd{E}(r *reader) {E} {{", "\tswitch r.u16() {"]
        for v in e["variants"]:
            if not v.get("ok") or "go" not in v:
                continue
            if v in units:
                lines += [f"\tcase {v['num']}:", f"\t\treturn {v['go']}"]
                continue
            fld = lambda f: self.vfield(v, f)
            try:
                inits = [f"{fld(f)}: {self.rd(f['desc'], not e['in'])}" for f in v["fields"]]
            except Unsup as ex:
                lines += [f"\tcase {v['num']}:", f"\t\tstop(\"{v['go']} cannot be read by the Go package yet ({ex}): leave out the call or handler that hands it (docs/API-GO.md, section Not in Go yet)\")"]
                continue
            if self.embeds(v):
                inits = [f"{self.gtype(v['fields'][0]['desc'], True)}: {self.rd(v['fields'][0]['desc'], not e['in'])}"]
            lines += [f"\tcase {v['num']}:", f"\t\treturn {v['go']}{{{', '.join(inits)}}}"]
        lines += ["\t}", f"\tstop(\"an event of type {E} arrived with a variant this package does not know: the library and the package disagree. Build the app against the go/ folder of the download its libgpuikit.so came from\")",
                  "\treturn nil", "}", ""]
        return lines

    @staticmethod
    def vfield(v, f):
        if v["shape"] == "struct":
            return camel(f["name"])
        return "Value" if len(v["fields"]) == 1 else "V" + f["name"]

    def embeds(self, v):
        return v["shape"] not in ("struct", "unit") and len(v["fields"]) == 1 and v["fields"][0]["desc"][0] == "rec"

    @staticmethod
    def unit_values(e):
        if not e["in"]:
            return []
        return [v for v in e["variants"] if v.get("ok") and "go" in v and v["shape"] == "unit" and not v["fields"]]

    def denum(self, rt, e):
        """Emit Go tagged-union interface, unit-variant `Kind` enum, variant structs, and `put<E>` encoder."""
        E = e["go"]
        units = self.unit_values(e)
        if units:
            K = e["go_kind"]
            self.add(f"// {E} represents {rt}: unit variants use {K} constants and payload variants use distinct struct types.",
                     f"type {E} interface {{ is{E}() }}", "",
                     f"// {K} represents unit variants of {rt} (for example {units[0]['go']}).", f"type {K} uint16", "",
                     f"func ({K}) is{E}() {{}}", "", f"// {K} unit variant values.", "const (")
            for v in units:
                # Trailing Rust path comment is parsed by gates/conformance.py.
                self.add(f"\t{v['go']} {K} = {v['num']} // {rt}::{v['name']}")
            self.add(")", "")
        else:
            self.add(f"// {E} represents {rt} with one struct type per variant.", f"type {E} interface {{ is{E}() }}", "")
        puts = [(e["go_kind"], ["o.u16(uint16(x))"])] if units else []
        for v in e["variants"]:
            if not v.get("ok") or "go" not in v or v in units:
                continue
            V = v["go"]
            fld = lambda f: self.vfield(v, f)
            try:
                if self.embeds(v):
                    fields = ["\t" + self.gtype(v["fields"][0]["desc"], not e["in"])]
                else:
                    fields = [f"\t{fld(f)} {self.gtype(f['desc'], not e['in'])}" for f in v["fields"]]
            except Unsup as ex:
                v["go_bad"] = str(ex)
                self.add(f"// {V} represents {rt}::{v['name']} (placeholder; not yet wired in Go: {ex}).", f"type {V} struct{{}}",
                         f"func ({V}) is{E}() {{}}", "")
                continue
            self.add(*gk_doc.note(f"{rt}::{v['name']}", "go"), f"// {V} represents {rt}::{v['name']}.", f"type {V} struct {{", *fields, "}",
                     f"func ({V}) is{E}() {{}}", "")
            if e["in"]:
                try:
                    if self.embeds(v):
                        body = self.put(v["fields"][0]["desc"], f"x.{self.gtype(v['fields'][0]['desc'])}")
                    else:
                        body = [s for f in v["fields"] for s in self.put(f["desc"], f"x.{fld(f)}")]
                    puts.append((V, [f"o.u16({v['num']})"] + body))
                except Unsup as ex:
                    puts.append((V, [f"stop(\"{V} cannot be written by the Go package yet ({ex}): give another variant (docs/API-GO.md, section Not in Go yet)\")"]))
        gk_go_kinds.denum_held(self, e, puts)
        if e["in"]:
            lines = [f"func put{E}(o *tape, v {E}) {{", "\tswitch x := v.(type) {" if puts else "\tswitch v.(type) {"]
            for V, body in puts:
                lines += [f"\tcase {V}:", "\t\t_ = x"] + ["\t\t" + b for b in body]
            lines += ["\tdefault:", f"\t\tstop(\"a nil or unknown gk.{E} was given to a builder: give one of its variants (gk.{E}...)\")", "\t}", "}", ""]
            self.add(*lines)

    def into(self, rt, s):
        """Emit an untagged Go interface and type-switch encoder `put<I>` for an `impl Into<T>` set."""
        I = s["go"]
        cases, seen = [], set()
        for n, mem in enumerate(s["members"]):
            d = mem["desc"]
            try:
                t = self.gtype(d)
                body = self.put(d, "x")
            except Unsup:
                continue
            if t not in seen:
                seen.add(t)
                cases.append((t, n, body))
        names = ", ".join(t for t, _, _ in cases)
        self.add(f"// {I} accepts any Go type accepted by `impl Into<{rt}>`: {names}.", f"type {I} interface{{}}", "")
        lines = [f"func put{I}(o *tape, v {I}) {{", "\tswitch x := v.(type) {" if cases else "\tswitch v.(type) {"]
        for t, n, body in cases:
            lines += [f"\tcase {t}:", f"\t\to.u8({n})", "\t\t_ = x"] + ["\t\t" + b for b in body]
        lines += ["\tdefault:", f"\t\tstop(\"a %T was given where the kit takes a {I}: give one of {names}\", v)", "\t}", "}", ""]
        self.add(*lines)


    def tokens(self):
        """Emit the generated `s/`, `palette/`, and `theme/` Go subpackages."""
        m = self.m
        bound = lambda i: i["status"] in ("bound", "bridged")
        head = ["// Code generated by gen/gkgen.py (gen/gk_go.py). DO NOT EDIT.", ""]
        lines = head + ["// Package s defines nullary style tokens for GPUI's Styled trait and style-only kit traits,",
                        "// passed to element constructors or .Style(...) (for example gk.VFlex(s.SizeFull, s.Gap3)).", "package s", "",
                        'import "gpuikit.dev/gk"', "", "// Nullary style token constants.", "const ("]
        seen = set()
        for it in sorted(m.tokens, key=lambda i: i["op"]):
            if bound(it):
                n = camel(ada(it["name"]))
                if n in seen:
                    n += "_"
                seen.add(n)
                lines.append(f"\t{n} gk.Style = {it['op']}")
                self.mapped.add(it["id"])
        self.files["s/gen_s.go"] = "\n".join(lines + [")"]) + "\n"
        lines = head + ["// Package palette provides fixed palette color tokens (for example .Bg(palette.Blue500)).",
                        "package palette", "", 'import "gpuikit.dev/gk"', "", "// Fixed palette color tokens.", "var ("]
        for it in sorted(m.palette, key=lambda i: i["op"]):
            if bound(it):
                lines.append(f"\t{camel(ada(it['name']))} = gk.PaletteColor({0x8000 + it['op']})")
                self.mapped.add(it["id"])
        self.files["palette/gen_palette.go"] = "\n".join(lines + [")"]) + "\n"
        lines = head + ["// Package theme provides semantic theme color tokens resolved at draw time",
                        "// (for example .Bg(theme.Background)) so theme switches update automatically.", "package theme", "",
                        'import "gpuikit.dev/gk"', "", "// Semantic theme color tokens.", "var ("]
        for name, n in sorted(m.theme_nums.items(), key=lambda kv: kv[1]):
            lines.append(f"\t{camel(ada(name))} = gk.ThemeColor({n})")
        self.files["theme/gen_theme.go"] = "\n".join(lines + [")"]) + "\n"


    def report(self, path, family):
        """Write `gen/out/go-coverage.md` summarizing bound vs emitted Go items and unmapped reasons."""
        m = self.m
        rows = {}
        bound = lambda i: i["status"] in ("bound", "bridged")
        why = {it["id"]: r for it, r in self.unmapped}
        for it in m.all_items():
            if not bound(it):
                continue
            fam = family(it.get("owner") or it["id"])
            r = rows.setdefault(fam, [0, 0, 0])
            r[0] += 1
            if it["id"] in self.mapped:
                r[1] += 1
            elif it["id"] not in why and gk_go_bridges.covered(self, it, self.mapped) is None:
                r[1] += 1
            else:
                r[2] += 1
                why.setdefault(it["id"], gk_go_bridges.covered(self, it, self.mapped))
        tot = [sum(r[i] for r in rows.values()) for i in range(3)]
        out = ["# Go coverage", "", f"Generated by gen/gk_go.py. Bound: bound or bridged in the model; Go: generated into go/gk.", "",
               "| family | bound | Go | not yet in Go |", "|---|---:|---:|---:|"]
        for fam in sorted(rows):
            r = rows[fam]
            out.append(f"| {fam} | {r[0]:,} | {r[1]:,} | {r[2]:,} |")
        out.append(f"| **all** | **{tot[0]:,}** | **{tot[1]:,}** | **{tot[2]:,}** |")
        reasons = {}
        for iid, r in why.items():
            key = re.sub(r"\s+\S+::\S+", " (type)", r)
            reasons.setdefault(key, []).append(iid)
        out += ["", "## Why an item is not in Go yet, by reason", ""]
        for r, ids in sorted(reasons.items(), key=lambda kv: -len(kv[1])):
            out.append(f"- {len(ids):,}: {r} (e.g. `{ids[0]}`)")
        if self.collisions:
            out += ["", "## Names that collided and got a prefix or a number", ""]
            out += [f"- `{b}`: {k} (taken by {o})" for b, k, o in self.collisions]
        out += ["", "## Every item not in Go yet", ""]
        out += [f"- `{iid}`: {r}" for iid, r in sorted(why.items()) if iid not in self.mapped]
        with open(path, "w") as f:
            f.write("\n".join(out) + "\n")
        return rows, tot
