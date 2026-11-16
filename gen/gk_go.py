"""gk_go: writes the generated half of the Go package (go/gk/gen_*.go and the token packages
s, theme and palette) from the same model the Odin emitter reads (docs/GO.md).

It runs after gk_odin.Odin.emit, whose names (node["odin"], it["odin"], ...) it turns into Go
names by the rules of docs/GO.md section 2, so the two packages stay side by side. Generated Go
calls only the runtime's emitter interface (go/gk/rt_emit.go). An item whose parameter or result
is a descriptor this emitter does not map yet is left out of the package and listed, with the
descriptor, in gen/out/go-report.md; nothing is left out silently.
"""
import os
import re

from gk_model import INT_WIRE, SHORT, ada, is_fill, snake

GO_KEYWORDS = {"break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for",
               "func", "go", "goto", "if", "import", "interface", "map", "package", "range", "return", "select",
               "struct", "switch", "type", "var",
               # predeclared names a parameter would shadow inside generated bodies
               "len", "cap", "make", "new", "append", "copy", "delete", "panic", "string", "int", "bool", "byte",
               "error", "any", "nil", "true", "false", "rune", "float32", "float64", "uint8", "uint16", "uint32",
               "uint64", "int8", "int16", "int32", "int64", "min", "max", "clear", "print", "real", "close"}
# Names generated bodies use for themselves: a parameter that meets one gets a trailing underscore.
LOCALS = {"o", "e", "b", "cx", "r", "n", "src", "s", "slot", "gen", "ok", "c"}
INITIALISMS = {"id": "ID", "ids": "IDs", "url": "URL", "urls": "URLs", "uri": "URI", "json": "JSON", "html": "HTML",
               "http": "HTTP", "https": "HTTPS", "svg": "SVG", "css": "CSS", "api": "API", "ascii": "ASCII",
               "utf8": "UTF8"}
# Names the hand-written runtime declares in package gk (read from go/gk/rt_*.go at start, these
# are only the ones a generated name is likely to meet first).
GO_INT = {"u8": "uint8", "u16": "uint16", "u32": "uint32", "u64": "uint64", "i8": "int8", "i16": "int16",
          "i32": "int32", "i64": "int64", "usize": "int", "isize": "int"}


# Library values (`["libval", rust path, odin name]`, gk_lists.libval) the Go runtime declares, with
# their put and rd functions (go/gk/rt_platform.go): Odin name -> Go type.
GO_LIBVALS = {"Drag_Value": "DragValue"}
# Plot parts of the binding's own (go/gk/rt_data.go; gk_odin.PLOT_OWN_PARTS; PROTOCOLS.md 12.17).
GO_PLOT_OWN_PARTS = ("PlotPath", "PlotQuad")


class Unsup(Exception):
    """A descriptor the Go emitter does not map (yet): the item is left out and listed."""


def camel(name):
    """Odin snake_case or Ada_Case -> exported Go CamelCase (docs/GO.md section 2)."""
    name = name.rstrip("_")
    parts = [p for p in name.split("_") if p]
    out = ""
    for p in parts:
        low = p.lower()
        w = INITIALISMS.get(low) or (p[:1].upper() + p[1:])
        if out and out[-1].isdigit() and w[:1].isdigit():
            out += "_"
        out += w
    return out or "X"


def lower_camel(name):
    c = camel(name)
    for k, v in INITIALISMS.items():
        if c.startswith(v) and (len(c) == len(v) or not c[len(v)].islower()):
            return k + c[len(v):]
    return c[:1].lower() + c[1:]


def pname(name):
    if re.fullmatch(r"\d+", name or ""):
        name = "arg" + name
    n = lower_camel(snake(name)) if name else "arg"
    return n + "_" if n in GO_KEYWORDS or n in LOCALS else n


class Go:
    def __init__(self, m, od, outdir):
        self.m, self.od, self.out = m, od, outdir
        self.docs = self.doc_names = None  # gk_doc.Docs and its link resolver (gkgen.py sets them)
        self.names = {}
        self.collisions = []
        self.files = {}
        self.lines = []
        self.file = None
        self.tuples = {}
        self.handles = {}
        self.unmapped = []      # (item, reason)
        self.mapped = set()     # item ids emitted
        self.in_call = False
        self.oneofs = {}        # trait name -> (iface, [node keys])
        self.readers = {}       # Go type -> reader function name (records, denums, tuples)
        self.reader_src = {}    # reader name -> lines
        self.need_readers = set()
        prefixes = m.ov.get("go", {}).get("enum_prefix", {})
        self.enum_prefix = prefixes
        for fn in sorted(os.listdir(outdir)) if os.path.isdir(outdir) else []:
            if fn.endswith(".go") and not fn.startswith("gen_"):
                text = open(os.path.join(outdir, fn)).read()
                for name in re.findall(r"(?m)^(?:func|type|var|const)\s+([A-Z]\w*)", text):
                    self.names.setdefault(name, "hand " + fn)
                for block in re.findall(r"(?ms)^(?:const|var) \((.*?)^\)", text):
                    for name in re.findall(r"(?m)^\t([A-Z]\w*)", block):
                        self.names.setdefault(name, "hand " + fn)
        self.name_all()

    # ------------------------------------------------------------ names

    def claim(self, base, crate, key):
        pre = {"gpui_base": "Base", "gpui": "Gpui", "gpui_component": "Kit", "gpui_kit": "Kit"}[crate]
        for c in (base, pre + base):
            if self.names.get(c, key) == key:
                self.names[c] = key
                return c
        n = 2
        while f"{pre}{base}{n}" in self.names:
            n += 1
        self.collisions.append((base, key, self.names.get(pre + base)))
        self.names[f"{pre}{base}{n}"] = key
        return f"{pre}{base}{n}"

    def name_all(self):
        m = self.m
        order = {"gpui_component": 0, "gpui_base": 1, "gpui": 2, "gpui_kit": 3}
        bound = lambda i: i["status"] in ("bound", "bridged")  # noqa: E731
        for key in sorted(m.nodes, key=lambda k: (order[m.nodes[k]["crate"]], k)):
            node = m.nodes[key]
            if "odin" in node:
                node["go"] = self.claim(camel(node["odin"]) + ("El" if node["element"] else ""), node["crate"], key)
        for table in (m.enums, m.denums, m.records):
            for rt in sorted(table, key=lambda k: (order[table[k]["crate"]], k)):
                t = table[rt]
                t["go"] = self.claim(camel(t["odin"]), t["crate"], rt)
        for key in sorted(m.states, key=lambda k: (order[m.states[k]["crate"]], k)):
            st = m.states[key]
            if "odin" in st:
                st["go"] = self.claim(camel(st["odin"]), st["crate"], "state " + key)
        for rt in sorted(m.intos):
            s = m.intos[rt]
            if "odin" in s:
                s["go"] = self.claim(camel(s["odin"]), "gpui_kit", "into " + rt)
        # enum values and variant structs
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
        # constructors (elements: the type's name without El; values and states: New + name)
        for key in sorted(m.nodes, key=lambda k: (order[m.nodes[k]["crate"]], k)):
            node = m.nodes[key]
            for it in sorted(node["ctors"], key=lambda i: i["id"]):
                if not bound(it) or it["kind"] == "convert" or "odin" not in it:
                    continue
                base = camel(it["odin"]) if node["element"] else "New" + camel(it["odin"])
                it["go"] = self.claim(base, it["crate"], it["id"])
        for key in sorted(m.states, key=lambda k: (order[m.states[k]["crate"]], k)):
            st = m.states[key]
            for it in sorted(st.get("ctors", []), key=lambda i: i["id"]):
                if it["status"] == "bridged" and "odin" in it:
                    it["go"] = self.claim("New" + camel(it["odin"]), st["crate"], it["id"])
            for it in sorted(st.get("globals", []) + st.get("statics", []), key=lambda i: i["id"]):
                if it["status"] == "bridged" and "odin" in it:
                    it["go"] = self.claim(camel(it["odin"]), st["crate"], it["id"])
        for rt in sorted(m.records):
            rc = m.records[rt]
            if rc.get("short_odin"):
                rc["short_go"] = self.claim(camel(rc["shorthand"]), rc["crate"], "shorthand " + rt)

    # ------------------------------------------------------------ types

    def gtype(self, d, out=False):
        m, k = self.m, d[0]
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
                return "Fill"  # a colour or a gradient (rt_emit.go, PROTOCOLS.md 11.38)
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
            name = "Tuple" + "".join(re.sub(r"\W+", "", t.title()) for t in ts)
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
            import gk_overlay  # noqa: PLC0415
            return self.gtype(["entity", gk_overlay.PANEL_RUST, gk_overlay.PANEL_NAME], out)  # a dock panel (11.7)
        if k == "data":
            return "Data"  # gk.NewData(rows): a slice of the app's structs (rt_data.go, PROTOCOLS.md 12)
        if k in ("field", "format"):
            return "string"  # a column's name, or a template
        if k == "viewbuild":
            node = m.nodes[d[1]]
            if "go" not in node:
                raise Unsup(f"viewbuild of {d[1]} without a Go type")
            return f"Built[{node['go']}]"  # a view-built closure (PROTOCOLS.md 11.6, rt_built.go)
        # The port of Odin's hand-written values and platform families (PROTOCOLS.md
        # 11.17, 13.20, 13.31; go/gk/rt_platform.go).
        if k == "libval" and d[2] in GO_LIBVALS:
            return GO_LIBVALS[d[2]]
        if k == "dragview":
            return "View"  # a drag's preview: the app's view
        if k == "dragstyle":
            return self.gtype(["stylefn", d[1], d[2]], out)  # a described style while a drag is over
        if k == "dropaccept":
            return "DropAccept"
        if k == "viewany":
            if len(d) > 1 and "go" in m.nodes.get(d[1][1], {}):
                return "ViewOrTooltip"  # the app's view or the kit's Tooltip value (11.17)
            return "View"
        if k in ("uri", "arcpath"):
            return "string"
        if k == "imagebytes":
            return "ImageBytes"
        if k in ("pixelimage", "customimage"):
            return "PixelImage"
        raise Unsup(k)

    @staticmethod
    def shape_args(sh):
        """A shape's payload descriptors; an index (Rust usize) is a Go int, though the model
        keeps it as a u64 on the wire."""
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

    # ------------------------------------------------------------ writers

    def put(self, d, v, depth=0):
        """Go statements that write `v` (of descriptor d) to the op stream `o`."""
        m, k = self.m, d[0]
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
            return [f"o.u64({v}.h)"]
        if k in ("entity", "handle", "shim", "rows", "datum_rows", "panelview"):
            self.gtype(d)
            return [f"o.u64({v}.h)"]
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
            self.gtype(d)
            return [f"putBuilt(o, {v}.View)"]
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
            return [f"putPixelImage(o, {v})"]
        raise Unsup(k)

    # ------------------------------------------------------------ readers

    def rd(self, d, out=True):
        """A Go expression reading a value of descriptor d from the reader `r`, as gtype(d, out)
        (a record that also travels in holds the in-types: a colour is then a Color)."""
        m, k = self.m, d[0]
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
            return f"{t}{{" + ", ".join(self.rd(x, out) for x in d[1]) + "}"
        if k in ("entity", "handle", "shim", "rows", "datum_rows", "obj"):
            return f"{self.gtype(d, out)}{{h: r.u64()}}"
        if k == "action":
            if d[1] == "listener":
                raise Unsup("action listener read back")
            return "Action{id: r.u32()}"
        if k == "result":
            return "rdResult(r)"
        if k == "libval" and d[2] in GO_LIBVALS:
            return f"rd{GO_LIBVALS[d[2]]}(r)"
        raise Unsup(f"{k} read back")

    # ------------------------------------------------------------ files

    def open(self, name, doc, imports=()):
        self.file = name
        self.lines = ["// Code generated by gen/gkgen.py (gen/gk_go.py). DO NOT EDIT.", "", "// " + doc, "", "package gk", ""]
        self.imports = set(imports)

    def add(self, *lines):
        self.lines += lines

    def close(self):
        text = "\n".join(self.lines) + "\n"
        imps = set(self.imports)
        if re.search(r"\btime\.[A-Z]", text):
            imps.add("time")
        if imps:
            text = text.replace("package gk\n", "package gk\n\nimport (\n" + "".join(f'\t"{i}"\n' for i in sorted(imps)) + ")\n", 1)
        self.files[self.file] = text

    def try_item(self, it, fn):
        """Runs fn() -> lines for an item; an unmapped descriptor leaves the item out, listed."""
        try:
            saved = (dict(self.tuples), set(self.need_readers))
            lines = fn()
        except Unsup as e:
            self.tuples, self.need_readers = dict(saved[0]), set(saved[1])
            self.unmapped.append((it, str(e)))
            return []
        self.mapped.add(it["id"])
        return lines

    def params(self, it, extra=()):
        ps = []
        for a in it["args"]:
            if a["desc"][0] == "ctx":
                continue
            ps.append(f"{pname(a['name'])} {self.ptype(a['desc'])}")
        return list(extra) + ps

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
        """The comment above a function: GPUI-kit's own doc of it (gen/gk_doc.py), then what it is
        upstream, last."""
        head = f"// {name} is {it.get('callee') or it['id']}."
        body = self.docs.comment(it.get("doc"), "go", self.doc_names, it["id"]) if self.docs else []
        if not body:
            return head
        return "\n".join([("// " + x).rstrip() for x in body] + ["//", head])

    # ------------------------------------------------------------ emit

    def emit(self):
        m = self.m
        bound = lambda i: i["status"] in ("bound", "bridged")  # noqa: E731
        # ---- nodes, per upstream module (as the Odin package)
        groups = {}
        for key in sorted(m.nodes):
            node = m.nodes[key]
            if "go" in node and any(bound(i) for i in node["ctors"]):
                fname = f"gen_{SHORT[node['crate']]}_{re.sub(r'[^a-z0-9]+', '_', node['module'].lower()) or 'root'}"
                groups.setdefault(fname, []).append(node)
        trait_fns = {}
        for fname in sorted(groups):
            self.open(fname + ".go", "The node types of one upstream module.")
            for node in groups[fname]:
                self.node(node, trait_fns)
            self.close()
        # ---- shared trait op bodies
        self.open("gen_traits.go", "One function per builder-trait method, shared by every node type that has the trait.")
        for name in sorted(trait_fns):
            self.add(*trait_fns[name])
        self.close()
        # ---- kit state and library objects
        self.open("gen_state.go", "Kit state types and library objects: handles to what the library keeps (PROTOCOLS.md 5, 13).")
        for key in sorted(m.states, key=lambda k: m.states[k].get("go", "")):
            if "go" in m.states[key]:
                self.in_call = True
                try:
                    self.state(m.states[key])
                finally:
                    self.in_call = False
        self.close()
        # ---- ambient calls
        self.ambient()
        # ---- parts of a described plot (gk.Plot, rt_data.go; PROTOCOLS.md 12.6, 12.17)
        self.plot_parts()
        # ---- types (last: the items above asked for tuples, readers and handle types)
        self.types()
        self.tokens()
        os.makedirs(self.out, exist_ok=True)
        for f in os.listdir(self.out):
            if f.startswith("gen_") and f.endswith(".go") and f not in self.files and f != "gen_ops.go":  # core/wire/gen.py's
                os.remove(os.path.join(self.out, f))
        for name, text in self.files.items():
            path = os.path.join(self.out, name)
            os.makedirs(os.path.dirname(path), exist_ok=True)
            open(path, "w").write(text)

    # ------------------------------------------------------------ nodes

    def node(self, node, trait_fns):
        m = self.m
        bound = lambda i: i["status"] in ("bound", "bridged")  # noqa: E731
        T = node["go"]
        el = node["element"]
        self.add(f"// {T} is {node['path']}.", f"type {T} struct{{ n *node }}", "")
        if el:
            self.add(f"func (e {T}) node() *node {{ return e.n }}", "")
        else:
            self.add(f"func (e {T}) valueNode() *node {{ return e.n }}", "")
        # constructors
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
        # methods: own ops, converts, then the traits' (an inherent method shadows a trait's)
        seen = {"node", "valueNode"}
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
                return lines + ["\treturn e", "}", ""]
            lines = self.try_item(it, meth)
            if lines:
                seen.add(name)
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
                lines = [f"// {name} is {it.get('callee') or it['id']}: it makes a {U} of the element.",
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
            if "odin" not in t:
                continue
            tl = camel(t["low"])
            if t["name"] == "Styled" and "Style" not in seen:
                seen.add("Style")
                self.add(f"// Style adds style tokens (s.Gap2, s.SizeFull).", f"func (e {T}) Style(st ...Style) {T} {{ tokens(e.n, st); return e }}", "")
            if t["name"] == "ParentElement" and el:
                if "Text" not in seen:
                    seen.add("Text")
                    self.add(f"func (e {T}) Text(s string) {T} {{ putText(e.n, s); return e }}", "")
                if "ChildView" not in seen:
                    seen.add("ChildView")
                    self.add(f"// ChildView embeds a view: the parent's description does not change when the child is described again.",
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
                body = self.docs.comment(it.get("doc"), "go", self.doc_names, it["id"]) if self.docs else []
                self.add(*[("// " + x).rstrip() for x in body],
                         f"func (e {T}) {name}({', '.join(ps)}) {T} {{ {fn}({', '.join(['e.n'] + args)}); return e }}")
            self.add("")
        if node.get("oneof_of"):
            pass

    # ------------------------------------------------------------ kit state

    def state(self, st):
        m = self.m
        ok = lambda i: i["status"] == "bridged"  # noqa: E731
        S = st["go"]
        obj = st.get("obj", False)
        get = "objGet" if obj else "stateGet"
        what = "a library object (PROTOCOLS.md section 13)" if obj else "kit state that lives in the library"
        self.add(f"// {S} is a handle to {st['path']}, {what}.", f"type {S} struct{{ h uint64 }}", "",
                 f"func (s {S}) handle() uint64 {{ return s.h }}", "")
        reads = sorted([i for i in st["reads"] if ok(i)], key=lambda i: i["name"])
        writes = sorted([i for i in st["writes"] if ok(i)], key=lambda i: i["name"])
        calls = sorted([i for i in st.get("calls", []) if ok(i)], key=lambda i: i["name"])
        builders = sorted([i for i in st["builders"] if ok(i)], key=lambda i: i["name"])
        want = set(m.ov.get("state", {}).get("try_reads", []))
        names = set()
        head = lambda it: [f"\to := newReq({it['op']})", "\to.u64(s.h)"]  # noqa: E731
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
                    lines += [f"// Try{name} is {name} for a handler the component calls from inside its own update: ok is false while the state is busy.",
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
                return [f"// {name} subscribes a handler to the state's {ev['name']} events; gk.Release ends it.",
                        f"func (s {S}) {name}(cx *Cx, h {H}) Subscription {{",
                        f"\tslot, gen := keepHandler(cx, h, {sh['num']})", f"\to := newReq({ev['op']})", "\to.u64(s.h)",
                        "\to.u32(slot)", "\to.u32(gen)", f"\tr := stateGet(s.h, cx, o)", "\treturn Subscription{h: r.u64(), slot: slot}", "}", ""]
            lines = self.try_item(ev, sub)
            if lines:
                names.add(name)
                self.add(*lines)
        B = S + "Builder" if builders else S
        if builders:
            self.add(f"// {B} is the chain of {st['name']}'s `self -> Self` methods; Build makes it, in one crossing.",
                     f"type {B} struct{{ sb *stateBuilder }}", "")
            for it in builders:
                name = camel(it["name"])

                def bl(it=it, name=name):
                    body = self.body(it)
                    lines = [f"func (b {B}) {name}({', '.join(self.params(it))}) {B} {{",
                             f"\to := b.sb.op({it['op']})" if body else f"\tb.sb.op({it['op']})"]
                    return lines + ["\t" + x for x in body] + ["\treturn b", "}", ""]
                self.add(*self.try_item(it, bl))
            self.add(f"// Build makes the {st['name']}.", f"func (b {B}) Build() {S} {{ return {S}{{b.sb.build()}} }}", "")
        for it in sorted([i for i in st["ctors"] if ok(i) and "go" in i], key=lambda i: i["go"]):
            def ct(it=it):
                lines = [self.doc(it, it["go"]), f"func {it['go']}({', '.join(self.params(it, ['cx *Cx']))}) {B} {{",
                         f"\to := newReq({it['op']})"]
                lines += ["\t" + x for x in self.body(it)]
                if builders:
                    lines += ["\treturn " + B + "{newStateBuilder(cx, o)}", "}", ""]
                else:
                    lines += ["\to.u16(0) // no builder ops follow", f"\treturn {S}{{stateMake(cx, o)}}", "}", ""]
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

    # ------------------------------------------------------------ plot parts

    def plot_parts(self):
        """gen_plot.go: the PlotPart interface over every value type that paints itself (the Odin
        emitter's Plot_Part union), and the binding's own parts. gk.Plot and gk.PlotWith (rt_data.go)
        stand for the kit's Plot trait, so its items and each part's paint are mapped."""
        m = self.m
        bound = lambda i: i["status"] in ("bound", "bridged")  # noqa: E731
        nodes = sorted((n for n in m.nodes.values() if n.get("paint") and n["paint"]["status"] == "bridged"
                        and any(bound(i) for i in n["ctors"]) and "go" in n), key=lambda n: n["go"])
        if not nodes:
            return
        self.open("gen_plot.go", "The parts of a described plot: every value type that paints itself.")
        self.add("// PlotPart is a value gk.Plot paints, in the order given: one of " +
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
                self.mapped.add(it["id"])  # Plot's id, interactive, appear_generation, paint: gk.PlotWith

    # ------------------------------------------------------------ ambient calls

    def ambient(self):
        import gk_app  # noqa: PLC0415
        items = sorted([i for i in gk_app.bound(self.m) if "odin" in i], key=lambda i: i["odin"])
        self.open("gen_ambient.go", "Calls of the ambient context: free functions of the kit and methods of Window "
                  "(PROTOCOLS.md 11.5). Each runs at once, with the turn's window and app.")
        self.in_call = True
        try:
            for it in items:
                it["go"] = self.claim(camel(it["odin"]), it["crate"], it["id"])

                def amb(it=it):
                    ps = self.params(it, ["cx *Cx"])
                    if it.get("ret") is None:
                        lines = [self.doc(it, it["go"]), f"func {it['go']}({', '.join(ps)}) {{"]
                    else:
                        lines = [self.doc(it, it["go"]), f"func {it['go']}({', '.join(ps)}) {self.gtype(it['ret'], True)} {{"]
                    lines += [f"\to := newReq({it['op']})"] + ["\t" + x for x in self.body(it)]
                    if it.get("ret") is None:
                        return lines + ["\tambientCall(cx, o, false)", "}", ""]
                    return lines + ["\tr := ambientCall(cx, o, true)", f"\treturn {self.rd(it['ret'])}", "}", ""]
                self.add(*self.try_item(it, amb))
        finally:
            self.in_call = False
        self.close()

    # ------------------------------------------------------------ types

    def types(self):
        m = self.m
        self.open("gen_types.go", "Enums, records, enums with data, Into sets, handle types and handler shapes.")
        for rt in sorted(m.enums, key=lambda k: m.enums[k]["go"]):
            e = m.enums[rt]
            E = e["go"]
            self.add(f"// {E} is {rt}.", f"type {E} uint16", "", "const (")
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
        # handler shapes: event structs and the table the runtime calls handlers through
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
                    self.add(f"// {S} is the payload of handlers of shape {sh['num']}: ({', '.join(sh['rust'])}).",
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
                reg.append(f'\tshapes[{sh["num"]}] = shapeInfo{{name: "{H}", call: func(h any, cx *Cx, r *reader, ret *tape) bool {{')
                reg += [f"\t\tf, ok := h.({H})", "\t\tif !ok {", "\t\t\treturn false", "\t\t}", f"\t\t{callexpr}", "\t\treturn true", "\t}}"]
                sh["go_ok"] = True
            except Unsup as e:
                sh["go_ok"] = False
                sh["go_why"] = str(e)
        self.add("func init() {", *reg, "}", "")
        for name, (ts, ds) in sorted(self.tuples.items()):
            self.add(f"type {name} struct {{", *[f"\tV{n} {t}" for n, t in enumerate(ts)], "}", "")
        for name, rt in sorted(self.handles.items()):
            self.add(f"// {name} is a handle to {rt}, kept by the library.", f"type {name} struct{{ h uint64 }}", "")
        import gk_comp6  # noqa: PLC0415
        tip = gk_comp6.tooltip_key(m)
        if tip is not None and "go" in m.nodes[tip]:
            self.add(f"// {m.nodes[tip]['go']} is a tooltip an element's Tooltip takes (PROTOCOLS.md 11.17; rt_platform.go).",
                     f"func ({m.nodes[tip]['go']}) isViewOrTooltip() {{}}", "")
        import gk_values  # noqa: PLC0415
        anim, _ = gk_values.animatable(m)
        anim = [m.nodes[k]["go"] for k, _ in anim if "go" in m.nodes[k]]
        self.add("// AnimatableElement is a styled element an animation or effect may wrap (gk.Effect; PROTOCOLS.md 13.30).",
                 "type AnimatableElement interface {", "\tElement", "\tisAnimatable()", "}", "")
        for name in anim:
            self.add(f"func ({name}) isAnimatable() {{}}")
        self.add("")
        for trait, (iface, nodes) in sorted(self.oneofs.items()):
            self.add(f"// {iface} is one of the node types implementing {trait}.", f"type {iface} interface {{", "\tElement", f"\tis{iface}()", "}", "")
            for k in nodes:
                self.add(f"func ({self.m.nodes[k]['go']}) is{iface}() {{}}")
            self.add("")
        # readers asked for (records, enums with data travelling out), transitively
        done = set()
        while self.need_readers - done:
            for k, rt in sorted(self.need_readers - done):
                done.add((k, rt))
                self.add(*self.reader(k, rt))
        for name, (ts, ds) in sorted(self.tuples.items()):
            pass
        self.close()

    def record(self, rt, rc):
        R = rc["go"]
        fname = lambda f: ("V" + f["name"]) if rc["tuple"] else camel(f["name"])  # noqa: E731
        try:
            fields = [f"\t{fname(f)} {self.gtype(f['desc'], not rc['in'])}" for f in rc["fields"]]
        except Unsup as e:
            rc["go_bad"] = str(e)
            self.add(f"// {R} is {rt}: not carried in Go yet ({e}).", f"type {R} struct{{}}", "")
            if rc["in"]:
                self.add(f"func put{R}(o *tape, v {R}) {{ stop(\"{R} is not carried by the Go package yet: leave out the builder that takes it (docs/API-GO.md, section Not in Go yet)\") }}", "")
            return
        self.add(f"// {R} is {rt}." + ("" if rc["complete"] else " Only the fields the binding can carry."),
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
            self.add(f"// {rc['short_go']} is a {R} with only {f} set (the other fields 0): gk.{rc['short_go']}(4) (PROTOCOLS.md 11.36).",
                     f"func {rc['short_go']}({lower_camel(f)} int) {R} {{ return {R}{{{f}: {lower_camel(f)}}} }}", "",
                     f"// No{rc['short_go']} is no {R}, for a parameter that takes an optional one: gk.NewSelectState(cx, src, gk.No{rc['short_go']}).",
                     f"var No{rc['short_go']} = Option[{R}]{{}}", "")

    def reader(self, k, rt):
        m = self.m
        if k == "rec":
            rc = m.records[rt]
            R = rc["go"]
            fname = lambda f: ("V" + f["name"]) if rc["tuple"] else camel(f["name"])  # noqa: E731
            try:
                body = [f"\tv.{fname(f)} = {self.rd(f['desc'], not rc['in'])}" for f in rc["fields"]]
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
            fld = lambda f: self.vfield(v, f)  # noqa: E731
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
        """A variant's field name: a struct variant's own (CamelCase); the one field of a tuple
        variant is Value (e.Value of SelectEventConfirm), more are V0, V1, ..."""
        if v["shape"] == "struct":
            return camel(f["name"])
        return "Value" if len(v["fields"]) == 1 else "V" + f["name"]

    def embeds(self, v):
        """A tuple variant whose one field is a record embeds it: e.Row for ListEvent::Select(IndexPath)."""
        return v["shape"] not in ("struct", "unit") and len(v["fields"]) == 1 and v["fields"][0]["desc"][0] == "rec"

    @staticmethod
    def unit_values(e):
        """The unit variants of an enum with data that the app gives to the kit (ButtonVariant,
        Size, Easing): they are constants of one kind type, values like a plain enum's
        (`.OkVariant(gk.ButtonVariantDanger)`, `.WithSize(gk.SizeLarge)`), where a struct type per
        variant would need `gk.ButtonVariantDanger{}`. An enum only the kit
        hands out (an event) keeps a struct type per variant, for a type switch."""
        if not e["in"]:
            return []
        return [v for v in e["variants"] if v.get("ok") and "go" in v and v["shape"] == "unit" and not v["fields"]]

    def denum(self, rt, e):
        E = e["go"]
        units = self.unit_values(e)
        if units:
            K = e["go_kind"]
            self.add(f"// {E} is {rt}: a constant of {K} for each variant without data, a struct type for each other one.",
                     f"type {E} interface {{ is{E}() }}", "",
                     f"// {K} is a variant of {rt} without data: gk.{units[0]['go']}.", f"type {K} uint16", "",
                     f"func ({K}) is{E}() {{}}", "", "const (")
            for v in units:
                self.add(f"\t{v['go']} {K} = {v['num']} // {rt}::{v['name']}")
            self.add(")", "")
        else:
            self.add(f"// {E} is {rt}: one struct type per variant.", f"type {E} interface {{ is{E}() }}", "")
        puts = [(e["go_kind"], ["o.u16(uint16(x))"])] if units else []
        for v in e["variants"]:
            if not v.get("ok") or "go" not in v or v in units:
                continue
            V = v["go"]
            fld = lambda f: self.vfield(v, f)  # noqa: E731
            try:
                if self.embeds(v):
                    fields = ["\t" + self.gtype(v["fields"][0]["desc"], not e["in"])]
                else:
                    fields = [f"\t{fld(f)} {self.gtype(f['desc'], not e['in'])}" for f in v["fields"]]
            except Unsup as ex:
                v["go_bad"] = str(ex)
                self.add(f"// {V} is {rt}::{v['name']}: not carried in Go yet ({ex}).", f"type {V} struct{{}}",
                         f"func ({V}) is{E}() {{}}", "")
                continue
            self.add(f"// {V} is {rt}::{v['name']}.", f"type {V} struct {{", *fields, "}", f"func ({V}) is{E}() {{}}", "")
            if e["in"]:
                try:
                    if self.embeds(v):
                        body = self.put(v["fields"][0]["desc"], f"x.{self.gtype(v['fields'][0]['desc'])}")
                    else:
                        body = [s for f in v["fields"] for s in self.put(f["desc"], f"x.{fld(f)}")]
                    puts.append((V, [f"o.u16({v['num']})"] + body))
                except Unsup as ex:
                    puts.append((V, [f"stop(\"{V} cannot be written by the Go package yet ({ex}): give another variant (docs/API-GO.md, section Not in Go yet)\")"]))
        if e["in"]:
            lines = [f"func put{E}(o *tape, v {E}) {{", "\tswitch x := v.(type) {" if puts else "\tswitch v.(type) {"]
            for V, body in puts:
                lines += [f"\tcase {V}:", "\t\t_ = x"] + ["\t\t" + b for b in body]
            lines += ["\tdefault:", f"\t\tstop(\"a nil or unknown {E} was given to a builder: give one of its variants (gk.{E}...)\")", "\t}", "}", ""]
            self.add(*lines)

    def into(self, rt, s):
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
        self.add(f"// {I} is what `impl Into<{rt}>` accepts: one of {names}.", f"type {I} interface{{}}", "")
        lines = [f"func put{I}(o *tape, v {I}) {{", "\tswitch x := v.(type) {" if cases else "\tswitch v.(type) {"]
        for t, n, body in cases:
            lines += [f"\tcase {t}:", f"\t\to.u8({n})", "\t\t_ = x"] + ["\t\t" + b for b in body]
        lines += ["\tdefault:", f"\t\tstop(\"a %T was given where the kit takes a {I}: give one of {names}\", v)", "\t}", "}", ""]
        self.add(*lines)

    # ------------------------------------------------------------ tokens, palette, theme

    def tokens(self):
        m = self.m
        bound = lambda i: i["status"] in ("bound", "bridged")  # noqa: E731
        head = ["// Code generated by gen/gkgen.py (gen/gk_go.py). DO NOT EDIT.", ""]
        lines = head + ["// Package s holds the style tokens: every nullary method of GPUI's Styled trait and of the",
                        "// kit's style-only traits, by upstream's name: gk.VFlex(s.SizeFull, s.Gap3).", "package s", "",
                        'import "gpuikit.dev/gk"', "", "const ("]
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
        lines = head + ["// Package palette holds the kit's fixed colours (the colour functions without arguments):",
                        "// .Bg(palette.Blue500).", "package palette", "", 'import "gpuikit.dev/gk"', "", "var ("]
        for it in sorted(m.palette, key=lambda i: i["op"]):
            if bound(it):
                lines.append(f"\t{camel(ada(it['name']))} = gk.PaletteColor({0x8000 + it['op']})")
                self.mapped.add(it["id"])
        self.files["palette/gen_palette.go"] = "\n".join(lines + [")"]) + "\n"
        lines = head + ["// Package theme holds the colours of the kit's theme, resolved by the library each time it",
                        "// draws (a theme switch redraws without the app): .Bg(theme.Background).", "package theme", "",
                        'import "gpuikit.dev/gk"', "", "var ("]
        for name, n in sorted(m.theme_nums.items(), key=lambda kv: kv[1]):
            lines.append(f"\t{camel(ada(name))} = gk.ThemeColor({n})")
        self.files["theme/gen_theme.go"] = "\n".join(lines + [")"]) + "\n"

    # ------------------------------------------------------------ report

    def report(self, path, family):
        """Go against Odin per family: bound or bridged in Odin, generated in Go; and what is left."""
        m = self.m
        rows = {}
        bound = lambda i: i["status"] in ("bound", "bridged")  # noqa: E731
        why = {it["id"]: r for it, r in self.unmapped}
        for it in m.all_items():
            if not bound(it):
                continue
            fam = family(it.get("owner") or it["id"])
            r = rows.setdefault(fam, [0, 0, 0])
            r[0] += 1
            if it["id"] in self.mapped:
                r[1] += 1
            else:
                r[2] += 1
                why.setdefault(it["id"], f"kind {it.get('kind')}: an Odin hand-written bridge or protocol the Go runtime has not ported yet")
        tot = [sum(r[i] for r in rows.values()) for i in range(3)]
        out = ["# Go coverage against Odin", "", f"Generated by gen/gk_go.py. Odin: bound or bridged; Go: generated into go/gk.", "",
               "| family | Odin | Go | not yet in Go |", "|---|---:|---:|---:|"]
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
