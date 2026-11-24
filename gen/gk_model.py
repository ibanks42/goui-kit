"""gk_model: reads the committed API extract (api/rustdoc/*.json.gz, rustdoc JSON with docs and
spans removed) and classifies every public function of the kit crates by rule. The result is
the language-neutral interface description (api/gk.api.json.gz) the emitters read.

Nothing here decides that an item compiles: rustc and the Odin compiler are the judges.
Standard library only. See docs/DESIGN.md section 13 and gen/README.md.
"""
import json
import os
import re
import tomllib

import gk_app  # the app family's rules: actions, key contexts (PROTOCOLS.md section 11)
import gk_lists  # the lists and editor families' rules (PROTOCOLS.md sections 9.9 to 9.12)
import gk_values  # the values family's rules (PROTOCOLS.md sections 13.8 to 13.12)
import gk_overlay  # the overlays and dock families' rules (PROTOCOLS.md sections 11.6, 11.7)
import gk_editor  # the editor family's rules (PROTOCOLS.md sections 9.20 to 9.26)
import gk_items  # item procedures (PROTOCOLS.md 9.41)
import rdj

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
API = os.path.join(REPO, "api")
FACADE = {"gpui_component": "gpui_kit::component", "gpui_base": "gpui_kit::base", "gpui": "gpui_kit",
          "gpui_kit": "gpui_kit"}
STD = {"core", "std", "alloc"}
KIT = ("gpui_component", "gpui_base")  # priority order for names
CRATES = ("gpui", "gpui_base", "gpui_component", "gpui_kit")
SHORT = {"gpui_component": "component", "gpui_base": "base", "gpui": "gpui", "gpui_kit": "kit"}

# Skip reasons (the fixed list; DESIGN.md section 13 plus the four this generator needed).
SKIP_REASONS = {
    "platform-trait", "test-only", "third-party-type", "hidden", "feature-off", "rust-only-extension",
    "item-render-closure", "builder-getter", "builder-mutator", "value-helper", "deprecated", "unsafe",
}


class Unsup(Exception):
    """A parameter or function no rule binds. cat is 'skip:<reason>', 'todo:<protocol>' or 'norule'."""

    def __init__(self, cat, detail=""):
        super().__init__(f"{cat} {detail}")
        self.cat, self.detail = cat, detail


def last(path):
    return path.split("::")[-1]


def snake(name):
    s = re.sub(r"(?<=[a-z0-9])([A-Z])", r"_\1", name)
    s = re.sub(r"(?<=[A-Z])([A-Z][a-z])", r"_\1", s)
    return s.lower()


def ada(name):
    return "_".join(p[:1].upper() + p[1:] for p in snake(name).split("_") if p) or "X"


class Api:
    """The loaded crates with public-path lookup across them (through the kit's facade)."""

    def __init__(self, json_dir):
        self.crates, self.walks, self.by_def = {}, {}, {}
        for name in CRATES:
            p = os.path.join(json_dir, name + ".json")
            c = rdj.Crate(p if os.path.exists(p) else p + ".gz", name)
            if c.format != 61:
                raise SystemExit(f"gkgen: rustdoc JSON format {c.format} of {name} is not the pinned 61")
            self.crates[name] = c
            self.walks[name] = rdj.Walk(c)
            self.by_def[name] = {"::".join(p["path"]): i for i, p in c.paths.items() if p["crate_id"] == 0}
        self._pub = {}

    def home(self, crate, iid):
        c = self.crates[crate]
        it = c.item(iid)
        if it is not None and it["crate_id"] == 0:
            return crate, str(iid)
        p = c.paths.get(str(iid))
        if not p:
            return None, None
        ext = c.ext.get(str(p["crate_id"]), {}).get("name") if p["crate_id"] else crate
        if ext == "gpui_fast":
            ext = "gpui"
        if ext in self.crates:
            return ext, self.by_def[ext].get("::".join([ext] + p["path"][1:]))
        return ext, "::".join(p["path"])

    def item(self, crate, iid):
        hc, hid = self.home(crate, iid)
        if hc in self.crates and hid is not None:
            return hc, hid, self.crates[hc].item(hid)
        return hc, hid, None

    def pubpath(self, crate, iid):
        """The Rust path an application can write for the item, through the kit's facade."""
        hc, hid = self.home(crate, iid)
        if hc is None:
            return None
        if hc not in self.crates:
            return re.sub(r"^(alloc|core)::", "std::", hid) if hc in STD else None
        key = (hc, hid)
        if key not in self._pub:
            w, found = self.walks[hc], None
            for table in (w.types, w.traits, w.fns, w.other):
                if hid in table:
                    found = "::".join([FACADE[hc]] + table[hid][0].split("::")[1:])
                    break
            self._pub[key] = found
        return self._pub[key]


class Ctx:
    """Where a type is read: the crate its ids belong to, generic substitutions and bounds."""

    def __init__(self, crate, self_t=None, subst=None, bounds=None):
        self.crate, self.self_t, self.subst, self.bounds = crate, self_t, subst or {}, bounds or {}

    def at(self, crate):
        return self if crate == self.crate else Ctx(crate, self.self_t, self.subst, self.bounds)


def path_args(rp):
    return [a["type"] for a in ((rp.get("args") or {}).get("angle_bracketed") or {}).get("args", [])
            if isinstance(a, dict) and "type" in a]


def bound_parts(bounds):
    """[(trait name, trait ref)] of a bound list, lifetimes dropped."""
    return [(last(b["trait_bound"]["trait"]["path"]), b["trait_bound"]["trait"]) for b in bounds
            if "trait_bound" in b]


INT_WIRE = {"u8": "u8", "u16": "u16", "u32": "u32", "usize": "u64", "u64": "u64",
            "i8": "i8", "i16": "i16", "i32": "i32", "isize": "i64", "i64": "i64"}
# Event types the runtime encodes by hand (PROTOCOLS.md section 2): leaves when they travel out.
EVENT_LEAF = {"ClickEvent": "click_event", "KeyDownEvent": "key_event", "KeyUpEvent": "key_event",
              "MouseDownEvent": "mouse_event", "MouseUpEvent": "mouse_event", "MouseMoveEvent": "mouse_event",
              "DragMoveEvent": "drag_move_event"}  # DragMoveEvent<Drag>: the platform family (PROTOCOLS.md 13.31)
# The runtime's built-in handler shapes (core/wire/ops.txt): payload, return -> number.
BUILTIN_SHAPES = {('[["leaf", "click_event"]]', "unit"): 1, ("[]", "bool"): 2, ("[]", "unit"): 3,
                  ('[["bool"]]', "unit"): 4, ('[["int", "u64"]]', "unit"): 5, ('[["str"]]', "unit"): 6,
                  ('[["leaf", "key_event"]]', "unit"): 7, ('[["leaf", "click_event"]]', "bool"): 8,
                  ('[["leaf", "mouse_event"]]', "unit"): 9, ('[["f64"]]', "unit"): 10}
# gpui value types with a hand-written codec in the runtime (the closed list of DESIGN.md 13).
LEAF = {"Pixels": ["leaf", "px", ""], "Rems": ["leaf", "rems", ""], "Hsla": ["leaf", "color", ""],
        # Fill: a colour or a gradient (Styled::bg; core/src/genapi.rs `background`, PROTOCOLS.md 11.38)
        "Rgba": ["leaf", "color", "gpui_kit::Rgba"], "Fill": ["leaf", "color", "gpui_kit::Fill"],
        "Background": ["leaf", "color", "gpui_kit::Background"], "Length": ["leaf", "length", ""],
        "DefiniteLength": ["leaf", "definite_length", ""], "AbsoluteLength": ["leaf", "absolute_length", ""],
        "ElementId": ["leaf", "id", ""], "SharedString": ["str", "SharedString"],
        "AnyElement": ["child"]}
def is_fill(d):
    """A colour leaf where the kit takes a Fill (Styled::bg): it also takes a gradient, so the front
    ends type it gk.Fill and write it with put_fill (PROTOCOLS.md 11.38)."""
    return d[:2] == ["leaf", "color"] and len(d) > 2 and d[2] == "gpui_kit::Fill"


GPUI_HOMES = {"gpui", "gpui_shared_string", "gpui_util", "gpui_collections"}
HANDLES = {"FocusHandle"}
LATER_HANDLES = {"ScrollHandle", "UniformListScrollHandle", "ListState", "Subscription", "Task"}
CONTEXTS = {"Window": "window", "App": "app"}
RUST_ONLY_TRAITS = {"RenderOnce", "IntoElement", "Element", "Render", "Focusable", "EventEmitter", "Global",
                    "Action", "FluentBuilder", "View", "TestTextExt", "From", "Default", "IntoIterator",
                    "Deref", "Clone", "Copy"}


class Model:
    def __init__(self, json_dir, overrides_path):
        self.api = Api(json_dir)
        self.ov = tomllib.load(open(overrides_path, "rb"))
        self.ov_used = set()
        cpath = os.path.join(os.path.dirname(overrides_path), "clone.json")
        self.clones = {k: set(v) for k, v in json.load(open(cpath))["clone"].items()} if os.path.exists(cpath) else {}
        self.nodes = {}      # key -> node type
        self.enums = {}      # rust type -> unit enum
        self.denums = {}     # rust type -> enum with data
        self.records = {}    # rust type -> record
        self.intos = {}      # rust type of the target -> Into set
        self.shapes = {}     # key -> closure shape
        self.traits = {}     # public path -> trait table
        self.items = []      # every function, with its status
        self.owners = []     # coverage rows
        self.palette = []    # nullary functions returning a colour: palette constants
        self.tokens = []     # nullary Styled methods: style tokens
        self.style_ext = []  # (item, trait path): nullary methods of other style-only traits, tokens too
        self.theme = []      # the colour fields of the kit's theme: theme tokens
        self.states = {}     # public path -> a kit state type held in an entity
        self.tinfo = {}      # (crate, id) -> facts about a type
        self._busy = set()
        self._closure = 0   # > 0 while a closure's payload is read: objects travel there as before
        self.items_handed = set()  # element nodes handed to item procedures (gk_items, PROTOCOLS.md 9.41)
        self.objects = {}   # public path -> a library object type behind handles (PROTOCOLS.md section 13)
        self._prefer_rec = False  # a call op's argument: a builder type that is a plain record travels as the record
        self.scan_types()
        gk_overlay.after_scan(self)  # wrappers for one element type (PROTOCOLS.md 11.6)
        gk_values.after_scan(self)  # enum value nodes (PROTOCOLS.md 13.19)
        for crate in KIT:
            self.classify_crate(crate)
        self.classify_gpui()
        self.theme_tokens()
        gk_overlay.after_classify(self)  # described values, blanket tables (PROTOCOLS.md 11.6)
        gk_values.after_classify(self)  # blanket tables of the values family (PROTOCOLS.md 13.20)
        self.settle()

    # ---------------------------------------------------------------- facts about types

    def info(self, hc, hid):
        """Facts about a struct or enum, from its impl list."""
        key = (hc, hid)
        if key in self.tinfo:
            return self.tinfo[key]
        c = self.api.crates[hc]
        it = c.item(hid)
        k = c.kind(it)
        inner = it["inner"][k]
        f = {"crate": hc, "id": hid, "name": it["name"], "kind": k, "item": it, "traits": set(), "blanket": set(),
             "impls": [], "generic": [p["name"] for p in inner["generics"]["params"] if "type" in p["kind"]],
             "ctor": False, "state": False, "entity": False, "builders": 0, "default": False, "refs": 0,
             "non_exhaustive": "non_exhaustive" in json.dumps(it.get("attrs", [])),
             "deprecated": it.get("deprecation") is not None}
        for imp_id in inner.get("impls", []):
            ii = c.item(imp_id)
            if ii is None:
                continue
            im = ii["inner"]["impl"]
            if im.get("is_synthetic") or im.get("is_negative"):
                continue
            tr = im.get("trait")
            if tr is not None:
                tn = last(tr["path"])
                (f["blanket"] if im.get("blanket_impl") is not None else f["traits"]).add(tn)
                if tn == "Default":
                    f["default"] = True
                f["impls"].append(im)
                continue
            f["impls"].append(im)
            for m in im["items"]:
                fi = c.item(m)
                if not fi or "function" not in fi["inner"] or fi["visibility"] != "public":
                    continue
                sig = fi["inner"]["function"]["sig"]
                ins = sig["inputs"]
                out = rdj.ty(sig.get("output")) if sig.get("output") else "()"
                if any("Context<" in rdj.ty(t) and "Self" in rdj.ty(t) for _, t in ins):
                    f["state"] = f["entity"] = True
                if (not ins or ins[0][0] != "self") and out in ("Self", it["name"]):
                    f["ctor"] = True
                if ins and ins[0][0] == "self" and rdj.ty(ins[0][1]) == "Self" and out in ("Self", it["name"]):
                    f["builders"] += 1
                if ins and ins[0][0] == "self" and rdj.ty(ins[0][1]) in ("&Self", "&mut Self"):
                    f["refs"] += 1
        if f["traits"] & {"Render", "EventEmitter", "Global"} or it["name"] in self.ov.get("state", {}).get("types", []):
            f["state"] = True
        if f["traits"] & {"Render", "EventEmitter"}:
            f["entity"] = True
        if it["name"] in self.ov.get("state", {}).get("objects", []):
            f["state"] = f["entity"] = False  # a value held as a library object ([state] objects, PROTOCOLS.md 13.31)
        # gpui-fast 0.1.16 on: an Element gets IntoElement from `impl<T: Element> IntoElement for T`, a
        # blanket impl (rustdoc lists it apart), instead of each element's own impl.
        f["element"] = "IntoElement" in f["traits"] or "IntoElement" in f["blanket"] and "Element" in f["traits"]
        # Clone, which the reduced extract drops: api/clone.json, read from the pinned sources
        # (tools/clone_scan.py). An object handed out of a borrow is copied (PROTOCOLS.md 13).
        f["clone"] = "Clone" in f["traits"] or it["name"] in self.clones.get(hc, ())
        f["record"] = False
        if k == "struct" and not f["non_exhaustive"]:
            sk = inner["kind"]
            if isinstance(sk, dict) and "plain" in sk:
                f["record"] = not sk["plain"]["has_stripped_fields"] and bool(sk["plain"]["fields"])
            elif isinstance(sk, dict) and "tuple" in sk:
                f["record"] = bool(sk["tuple"]) and all(x is not None for x in sk["tuple"])
        self.tinfo[key] = f
        return f

    def scan_types(self):
        """Phase 1: which structs are node types (built by a constructor and builder calls)."""
        self.node_of = {}  # (crate, id) -> node key, for non-generic node types
        self.returned = set()
        for crate in KIT:
            c, w = self.api.crates[crate], self.api.walks[crate]
            # types a free function or a static method returns are constructible through it
            fns = [it for _, (_, it) in w.fns.items()]
            for _, (_, it) in w.types.items():
                for im in self.info(crate, str(it["id"]))["impls"]:
                    if im.get("trait") is None:
                        fns += [c.item(m) for m in im["items"] if c.item(m)]
            for fi in fns:
                if "function" not in fi["inner"]:
                    continue
                out = fi["inner"]["function"]["sig"].get("output")
                if out and "resolved_path" in out:
                    hc, hid = self.api.home(crate, out["resolved_path"]["id"])
                    if hc in self.api.crates and hid:
                        self.returned.add((hc, hid))
        inc = self.ov.get("gpui", {})
        for crate in KIT + ("gpui",):
            w = self.api.walks[crate]
            for iid, (path, it) in sorted(w.types.items(), key=lambda kv: kv[1][0]):
                if "struct" not in it["inner"]:
                    continue
                if crate == "gpui" and it["name"] not in inc.get("types", []):
                    continue
                f = self.info(crate, iid)
                forced = it["name"] in inc.get("types", []) or it["name"] in self.ov.get("node", {}).get("types", [])
                if not forced:
                    if f["generic"] or f["state"] or (f["record"] and not f["builders"]):
                        continue
                    if not (f["ctor"] or f["default"] or (crate, iid) in self.returned or f["element"]):
                        continue
                    if not f["element"] and not f["builders"] and f["refs"]:
                        continue  # an object with reads and calls, not a builder: the state protocol
                self.add_node(crate, iid, f, self.api.pubpath(crate, iid), it["name"], {})
        for spec in self.ov.get("instantiate", {}).get("types", []):
            # "Stateful<Div>": a generic wrapper instantiated for one argument list.
            # "List<crate::shim_rows::GkListDelegate> as List": an element generic over a delegate,
            # instantiated for the library's shim of it (docs/PROTOCOLS.md section 9.5).
            # "LineChart<crate::data::Datum, SharedString, f64> as LineChart": a generic type
            # instantiated for a whole argument list (docs/PROTOCOLS.md section 12).
            if self.instantiate_many(spec):
                continue
            m = re.fullmatch(r"(\w+)<([\w:]+)>(?:\s+as\s+(\w+))?", spec)
            base = next(((cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items()
                         if it["name"] == m.group(1)), None)
            if m.group(2).startswith("crate::"):
                # The one type of that name with a type parameter (gpui and the base crate have
                # a `List` of their own that is not generic).
                found = [(cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items()
                         if it["name"] == m.group(1) and self.info(cr, i)["generic"]]
                if len(found) != 1:
                    raise SystemExit(f"gkgen: overrides [instantiate] types: {m.group(1)} names {len(found)} generic types")
                base = found[0]
                f = self.info(*base)
                self.add_node(base[0], base[1], f, f"{self.api.pubpath(*base)}<{m.group(2)}>", m.group(3) or m.group(1),
                              {f["generic"][0]: (base[0], {"verbatim": m.group(2)})}, inst=("shim", m.group(2)))
                continue
            arg = next(((cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items()
                        if it["name"] == m.group(2)), None)
            if base is None or arg is None:
                raise SystemExit(f"gkgen: overrides [instantiate] names an unknown type: {spec}")
            f = self.info(*base)
            argpath = self.api.pubpath(*arg)
            self.add_node(base[0], base[1], f, f"{self.api.pubpath(*base)}<{argpath}>", m.group(1) + m.group(2),
                          {f["generic"][0]: (arg[0], {"resolved_path": {"path": m.group(2), "id": int(arg[1]),
                                                                        "args": None}})}, inst=arg)

    # ---------------------------------------------------------------- generic types, instantiated

    PRIMS = {"bool", "f32", "f64", "u8", "u16", "u32", "u64", "usize", "i8", "i16", "i32", "i64", "isize"}

    def data_types(self):
        """Library types of the data protocol (`[instantiate] data`): a type parameter given one
        of them is a row of a data table the app passes (docs/PROTOCOLS.md section 12)."""
        return set(self.ov.get("instantiate", {}).get("data", []))

    def inst_arg(self, crate, a):
        """(crate, type ref, Rust source) of one argument of an `[instantiate] types` line: a
        library type (`crate::...`), a primitive, or an upstream type by name."""
        if a.startswith("crate::"):
            return crate, {"verbatim": a}, a
        if a in self.PRIMS:
            return crate, {"primitive": a}, a
        if a == "()":  # a parameter's default unit (`Editor<R = ()>`, gpui-kit 0.11)
            return crate, {"tuple": []}, a
        found = [(cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items()
                 if it["name"] == a and not self.info(cr, i)["generic"]]
        if len(found) == 1:
            cr, i = found[0]
            return cr, {"resolved_path": {"path": a, "id": int(i), "args": None}}, self.api.pubpath(cr, i)
        if a in LEAF:  # gpui's own value types (SharedString, Hsla, Pixels): through the crates' path tables
            for cr in KIT:
                for pid, p in self.api.crates[cr].paths.items():
                    if p["path"][-1] == a and p["kind"] == "struct" and self.api.home(cr, pid)[0] in GPUI_HOMES:
                        return cr, {"resolved_path": {"path": a, "id": int(pid), "args": None}}, "gpui_kit::" + a
        raise SystemExit(f"gkgen: overrides [instantiate] types: {a} names {len(found)} types")

    def instantiate_many(self, spec):
        """`Base<A, B, ..> as Name`: one node type for a generic type with several parameters, or
        with one parameter given a primitive, a gpui value type or a data type. False for the
        one-argument forms the older rule reads."""
        m = re.fullmatch(r"(\w+)<(.+)>(?:\s+as\s+(\w+))?", spec)
        if m is None:
            raise SystemExit(f"gkgen: overrides [instantiate] types: not `Generic<Args> [as Name]`: {spec}")
        args = gk_values.split_args(m.group(2))  # nested arguments: Sidebar<SidebarGroup<SidebarMenu>> (13.8)
        if len(args) == 1 and not (args[0] in self.PRIMS or args[0] in LEAF or args[0] == "()" or args[0] in self.data_types()) and \
                (m.group(3) is None or args[0].startswith("crate::")):
            return False
        found = [(cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items()
                 if it["name"] == m.group(1) and len(self.info(cr, i)["generic"]) == len(args)]
        if len(found) != 1:
            raise SystemExit(f"gkgen: overrides [instantiate] types: {m.group(1)} with {len(args)} parameters "
                             f"names {len(found)} generic types")
        base = found[0]
        f = self.info(*base)
        subst, rust = {}, []
        for g, a in zip(f["generic"], args):
            cr, ref, rs = gk_values.inst_arg(self, base[0], a)
            subst[g] = (cr, ref)
            rust.append(rs)
        self.add_node(base[0], base[1], f, f"{self.api.pubpath(*base)}<{', '.join(rust)}>", m.group(3) or m.group(1),
                      subst, inst=("inst", spec))
        return True

    def inst_self(self, crate, iid, f, node):
        """The Self type of an instantiated node: the generic type applied to its arguments."""
        return (crate, {"resolved_path": {"path": f["name"], "id": int(iid), "args": {
            "angle_bracketed": {"args": [{"type": node["subst"][g][1]} for g in f["generic"]], "constraints": []}}}})

    def add_node(self, crate, iid, f, path, name, subst, inst=None):
        if path is None:
            return
        key = path
        self.nodes[key] = {"key": key, "path": path, "name": name, "crate": crate, "id": iid, "subst": subst,
                           "element": f["element"],
                           "styled": "Styled" in f["traits"], "traits": [], "ctors": [], "ops": [],
                           "inst": inst, "module": path.split("::")[2] if path.count("::") > 2 else ""}
        if inst is None:
            self.node_of[(crate, iid)] = key
        else:
            self.node_of[(crate, iid, inst)] = key

    # ---------------------------------------------------------------- Rust type printer

    def rtype(self, ctx, t):
        """The Rust source of a type, with public paths. Raises Unsup when it cannot be named."""
        if "primitive" in t:
            return t["primitive"]
        if "verbatim" in t:
            return t["verbatim"]
        if "generic" in t:
            g = t["generic"]
            if g == "Self" and ctx.self_t:
                return self.rtype(ctx.at(ctx.self_t[0]), ctx.self_t[1])
            if g in ctx.subst:
                return self.rtype(ctx.at(ctx.subst[g][0]), ctx.subst[g][1])
            raise Unsup("todo:generic type parameter without a closed set", g)
        if "resolved_path" in t:
            rp = t["resolved_path"]
            p = self.api.pubpath(ctx.crate, rp["id"])
            hc, _ = self.api.home(ctx.crate, rp["id"])
            if p is None and hc in GPUI_HOMES and last(rp["path"]) in LEAF:
                p = "gpui_kit::" + last(rp["path"])
            if p is None and hc == "lsp_types" and last(rp["path"]) == "Position":
                p = "lsp_types::Position"  # gk.Lsp_Position (gk_lists.gpui_value, PROTOCOLS.md 9.13)
            p = p or gk_lists.third_party_path(hc, last(rp["path"]))  # ropey's Rope, LSP's CompletionItem (9.19)
            if p is None:
                if hc in self.api.crates:
                    raise Unsup("skip:hidden", f"{rp['path']} has no public path")
                raise Unsup("skip:third-party-type", f"{rp['path']} ({hc})")
            args = [self.rtype(ctx, a) for a in path_args(rp)]
            return p + (f"<{', '.join(args)}>" if args else "")
        if "borrowed_ref" in t:
            b = t["borrowed_ref"]
            return "&" + ("mut " if b["is_mutable"] else "") + self.rtype(ctx, b["type"])
        if "tuple" in t:
            return "(" + ", ".join(self.rtype(ctx, x) for x in t["tuple"]) + ("," if len(t["tuple"]) == 1 else "") + ")"
        if "slice" in t:
            return f"[{self.rtype(ctx, t['slice'])}]"
        if "qualified_path" in t:
            r = gk_lists.qualified(self, ctx, t)  # an associated type of a library shim (PROTOCOLS.md 9.9)
            if r:
                return self.rtype(ctx.at(r[0]), r[1])
        raise Unsup("norule", f"type {rdj.ty(t)}")

    # ---------------------------------------------------------------- the parameter rules

    def desc(self, ctx, t, out=False):
        """The wire descriptor of a type: a JSON list. out=True: the value travels to the app."""
        if "verbatim" in t:
            lv = gk_lists.libval(self, t["verbatim"], out)  # a library value type (PROTOCOLS.md 9.9)
            if lv:
                return lv
            # A library type of the delegate protocol, an [instantiate] argument `crate::...`
            # (a list's or table's delegate): the library makes it over the row source the app
            # passes (docs/PROTOCOLS.md section 9.5). It never travels to the app.
            if t["verbatim"] in self.data_types():
                # One row of a data table the app passes (docs/PROTOCOLS.md section 12). Only a
                # whole table (`data`) or a closure over a row (`field`, `format`) is bound.
                if out:
                    raise Unsup("todo:protocol: data (a row of a data table travelling to the app)", t["verbatim"])
                return ["datum", t["verbatim"]]
            if out and not gk_lists.shim_out(self, t["verbatim"]):
                raise Unsup("todo:protocol: delegates (the library's delegate itself, read back)", t["verbatim"])
            return ["shim", t["verbatim"]]
        if "generic" in t:
            g = t["generic"]
            if g == "Self":
                if ctx.self_t is None:
                    raise Unsup("norule", "Self in a shared trait method")
                return self.desc(ctx.at(ctx.self_t[0]), ctx.self_t[1], out)
            if g in ctx.subst:
                return self.desc(ctx.at(ctx.subst[g][0]), ctx.subst[g][1], out)
            if g in ctx.bounds:
                return self.bounded(ctx, ctx.bounds[g], out)
            raise Unsup("todo:generic type parameter without a closed set", g)
        if "impl_trait" in t:
            return self.bounded(ctx, t["impl_trait"], out)
        if "primitive" in t:
            p = t["primitive"]
            if p == "bool":
                return ["bool"]
            if p in INT_WIRE:
                return ["int", p]
            if p in ("f32", "f64"):
                return [p]
            if p == "str":
                return ["str", "str"]
            if p == "char":
                return ["char"]  # a char travels as its u32 (the lists and editor families, PROTOCOLS.md 9.12)
            raise Unsup("norule", f"primitive {p}")
        if "borrowed_ref" in t:
            return self.borrowed(ctx, t["borrowed_ref"], out)
        if "tuple" in t:
            if not t["tuple"]:
                return ["unit"]
            return ["tuple", [self.desc(ctx, x, out) for x in t["tuple"]], self.rtype(ctx, t)]
        if "array" in t and gk_values.array(self, ctx, t, out):
            return gk_values.array(self, ctx, t, out)  # [T; N] (PROTOCOLS.md 13.10)
        if "slice" in t or "array" in t:
            raise Unsup("norule", f"bare slice or array {rdj.ty(t)}")
        if "dyn_trait" in t:
            names = [last(x["trait"]["path"]) for x in t["dyn_trait"]["traits"]]
            if "Action" in names and not out:
                return ["action", "dyn"]
            if "Action" in names:
                raise Unsup("todo:protocol: actions (an action value travelling to the app)", "dyn Action")
            raise Unsup("norule", f"dyn {'+'.join(names)}")
        if "resolved_path" in t:
            return self.resolved(ctx, t, out)
        if "qualified_path" in t:
            r = gk_lists.qualified(self, ctx, t)  # an associated type of a library shim (PROTOCOLS.md 9.9)
            if r:
                return self.desc(ctx.at(r[0]), r[1], out)
            raise Unsup("todo:associated type (generic over an app type)", rdj.ty(t))
        if "function_pointer" in t:
            raise Unsup("todo:function pointer (data protocol)", rdj.ty(t))
        if "raw_pointer" in t:
            raise Unsup("skip:unsafe", rdj.ty(t))
        raise Unsup("norule", f"type {rdj.ty(t)}")

    def borrowed(self, ctx, b, out):
        inner, mut = b["type"], b["is_mutable"]
        if "qualified_path" in inner:
            r = gk_lists.qualified(self, ctx, inner)  # &D::Item of a library shim (PROTOCOLS.md 9.9)
            if r:
                return self.borrowed(ctx.at(r[0]), {"type": r[1], "is_mutable": mut}, out)
        if inner.get("primitive") == "str":
            return ["str", "&str"]
        if "resolved_path" in inner:
            n = last(inner["resolved_path"]["path"])
            if n in CONTEXTS:
                return ["ctx", CONTEXTS[n] + ("" if mut else "_ref")]
            if n == "Context":
                raise Unsup("todo:protocol: state (a method on an entity's own Context)", rdj.ty(inner))
            if mut:
                d = None
                try:
                    d = self.desc(ctx, inner, out)
                except Unsup:
                    pass
                if d is not None and d[0] == "obj" and not out:
                    return ["refmut", d]
                raise Unsup("todo:protocol: state (&mut argument)", rdj.ty(inner))
            d = self.desc(ctx, inner, out)
            if d[0] in ("str",):
                return ["str", "&" + d[1]]
            return ["ref", d]
        if "slice" in inner and not mut:
            d = self.desc(ctx, inner["slice"], out)
            if d[0] == "datum" and not out:
                return ["data", "slice", d[1]]
            return ["list", d, "slice"]
        if "dyn_trait" in inner and not mut:
            return ["ref", self.desc(ctx, inner, out)]
        if "generic" in inner and not mut:
            return ["ref", self.desc(ctx, inner, out)]
        if "primitive" in inner and not mut and out:
            return ["ref", self.desc(ctx, inner, out)]
        raise Unsup("todo:protocol: state (&mut argument)" if mut else "norule", "&" + rdj.ty(inner))

    def bounded(self, ctx, bounds, out):
        eb = gk_editor.bounded(self, ctx, bounds, out)  # a Markdown plugin (PROTOCOLS.md 9.23)
        if eb is not None:
            return eb
        bi = gk_lists.bounded_out(self, ctx, bounds, out)  # an iterator read back: a list (9.11)
        if bi is not None:
            return bi
        parts = bound_parts(bounds)
        names = [n for n, _ in parts]
        for n, tr in parts:
            if n in ("Fn", "FnMut", "FnOnce"):
                p = (tr.get("args") or {}).get("parenthesized") or {"inputs": [], "output": None}
                static = any(b.get("outlives") == "'static" for b in bounds)
                return self.closure(ctx, n, p["inputs"], p.get("output"), "impl", static)
        for n, tr in parts:
            targs = path_args(tr)
            app = gk_app.bounded(self, ctx, n, targs, tr)
            if app:
                return app
            if n == "Into" and len(targs) == 1:
                return self.into(ctx, targs[0])
            if n == "AsRef" and len(targs) == 1 and targs[0].get("primitive") == "str":
                return ["str", "asref"]
            if n == "IntoElement":
                rest = set(names) - {"IntoElement", "Sized", "Send", "Sync", "Clone"}
                if rest:
                    raise Unsup("todo:generic method by instantiation (an element that must also implement a trait)",
                                " + ".join(sorted(rest)))
                return ["child"]
            if n == "IntoIterator":
                for cst in ((tr.get("args") or {}).get("angle_bracketed") or {}).get("constraints", []):
                    eq = cst["binding"].get("equality")
                    if cst["name"] == "Item" and eq and "type" in eq:
                        d = self.desc(ctx, eq["type"], out)
                        if d[0] == "datum" and not out:
                            return ["data", "iter", d[1]]
                        return ["list", d, "iter"]
                    if cst["name"] == "Item" and "constraint" in cst["binding"]:
                        return ["list", self.bounded(ctx, cst["binding"]["constraint"], out), "iter"]
            if n == "Action":
                raise Unsup("todo:protocol: actions (a typed action value)", "impl Action")
            if n == "Hash" and set(names) <= {"Hash", "Sized", "Send", "Sync", "Clone", "Eq", "PartialEq"}:
                # A key upstream only hashes (appear_key: draw in again when it changes): a string
                # is one, and every key an app has can be written as one.
                return ["str", "SharedString"]
            if n in ("Render", "Focusable", "EventEmitter", "Panel", "PanelView"):
                raise Unsup("todo:protocol: views (a generic over the app's view type)", n)
        raise Unsup("todo:generic type parameter without a closed set", " + ".join(names) or "unbounded")

    def into(self, ctx, target):
        """`impl Into<T>`: T itself, or the closed set of T's From impls (DESIGN.md 13)."""
        name = last(target["resolved_path"]["path"]) if "resolved_path" in target else None
        sub = self.stand_in(name)
        if sub is not None:
            return self.into(Ctx(sub[0]), {"resolved_path": {"path": sub[2], "id": sub[1], "args": None}})
        if name is not None and self.api.home(ctx.crate, target["resolved_path"]["id"])[0] not in GPUI_HOMES \
                and name != "String":
            pass
        elif name in LEAF:
            d = list(LEAF[name])
            if d[0] == "leaf":
                d[2] = ""  # the parameter converts by itself: hand it the base value
            elif d[0] == "str":
                d[1] = "into"
            return d
        if name == "String":
            return ["str", "String"]
        if name is None:
            return self.desc(ctx, target)
        hc, hid, it = self.api.item(ctx.crate, target["resolved_path"]["id"])
        rt = self.rtype(ctx, target)
        if rt in self.intos:
            return ["into", rt] if len(self.intos[rt]["members"]) > 1 else self.intos[rt]["single"]
        members, first_err = [], None
        try:
            d0 = self.desc(ctx, target)
            if d0[0] == "obj" and not d0[3]:  # a member is copied out of its handle (PROTOCOLS.md 13.12)
                raise Unsup("todo:protocol: object (inside an Into set, of a type that cannot be copied)", rdj.ty(target))
            members.append((d0, None))
        except Unsup as e:
            first_err = e
        if it is not None:
            c = self.api.crates[hc]
            k = c.kind(it)
            for imp_id in it["inner"][k].get("impls", []):
                ii = c.item(imp_id)
                im = ii["inner"]["impl"] if ii else None
                if not im or not im.get("trait") or last(im["trait"]["path"]) != "From" or im.get("blanket_impl"):
                    continue
                if any("type" in p["kind"] for p in im["generics"]["params"]):
                    members += gk_values.from_implementors(self, hc, im)  # impl<T: Trait> From<T> (13.11)
                    continue
                src = path_args(im["trait"])
                if len(src) == 1 and gk_lists.from_str_ref(src[0]):
                    members.append((["str", "&str"], "&str"))  # From<&str>: a string (PROTOCOLS.md 9.12)
                    continue
                if len(src) != 1 or "borrowed_ref" in src[0]:
                    continue
                try:
                    sctx = Ctx(hc)
                    members.append((self.desc(sctx, src[0]), self.rtype(sctx, src[0])))
                except Unsup:
                    continue
        seen, uniq = set(), []
        for d, src in members:
            k = json.dumps(strip_conv(d))
            if k not in seen:
                seen.add(k)
                uniq.append((d, src))
        if not uniq:
            raise first_err or Unsup("norule", f"impl Into<{rdj.ty(target)}> with no bindable member")
        if len(uniq) == 2 and uniq[0][1] is None and uniq[0][0][0] == "node" and uniq[1][0][0] == "enum":
            # An element made from an enum (Icon from an IconName): the parameter takes the enum
            # alone, so `->icon(.Search)` keeps working (Odin infers `.Search` for an enum, never
            # for a union); the element itself is made with its constructor (gk.icon(.Search))
            # where a child is wanted.
            uniq = uniq[1:]
        single = uniq[0][0] if uniq[0][1] is None else ["from", uniq[0][0], rt, uniq[0][1]]
        self.intos[rt] = {"target": rt, "name": name, "single": single,
                          "members": [{"desc": d, "from": src} for d, src in uniq]}
        return ["into", rt] if len(uniq) > 1 else single

    def stand_in(self, name):
        """`[into] stand_in` of api/overrides.toml: an `impl Into<X>` whose X the extract cannot name
        (a type in a private module of the kit, Button::icon's ButtonIcon) is bound as `impl
        Into<Y>` for the public type Y given there, which converts into X upstream. Returns Y's
        (crate, id, name), or None when X has no stand-in."""
        path = self.ov.get("into", {}).get("stand_in", {}).get(name or "")
        if path is None:
            return None
        for cr, w in self.api.walks.items():
            for tid, (_, t) in w.types.items():
                if self.api.pubpath(cr, tid) == path:
                    return cr, tid, t["name"]
        raise SystemExit(f"gk_model: [into] stand_in {name} = {path}: no public type of that path")

    def resolved(self, ctx, t, out):
        rp = t["resolved_path"]
        name, args = last(rp["path"]), path_args(rp)
        vd = gk_values.resolved(self, ctx, t, out)  # results, maps, Rc<X>, Option<&mut Window> (13.9, 13.10)
        if vd is not None:
            return vd
        ea = gk_editor.alias(self, ctx, t, out)  # an alias of a closure of the editor family's (9.22)
        if ea is not None:
            return ea
        ld = gk_lists.resolved(self, ctx, t, out)  # Arc<[T]>, NonZeroU32 (PROTOCOLS.md 9.12)
        if ld is not None:
            return ld
        od = gk_overlay.resolved(self, ctx, t, out)  # node ids, views, panels read back (PROTOCOLS.md 11.8)
        if od is not None:
            return od
        if name == "Option" and len(args) == 1:
            return ["opt", self.desc(ctx, args[0], out)]
        if name == "Vec" and len(args) == 1:
            d = self.desc(ctx, args[0], out)
            if d[0] == "datum" and not out:
                return ["data", "vec", d[1]]
            return ["list", d, "vec"]
        if name in ("Box", "Rc", "Arc") and len(args) == 1:
            a = args[0]
            if "dyn_trait" in a:
                for x in a["dyn_trait"]["traits"]:
                    n = last(x["trait"]["path"])
                    if n in ("Fn", "FnMut", "FnOnce"):
                        p = (x["trait"].get("args") or {}).get("parenthesized") or {"inputs": [], "output": None}
                        return self.closure(ctx, n, p["inputs"], p.get("output"), name.lower())
                    if n == "Action" and name == "Box":
                        if out and gk_editor.action_out(self):
                            return ["action", "box"]  # read back as the app's action id (PROTOCOLS.md 9.37)
                        if out:  # an action travels into the library only
                            raise Unsup("todo:event payload the binding cannot encode yet", "Box<dyn Action>")
                        return ["action", "box"]
            if a.get("primitive") == "str":
                return ["str", "into"]
            pv = gk_overlay.panel_view(self, ctx, name, a, out)  # Arc<dyn PanelView>: a dock panel (11.7)
            if pv:
                return pv
            raise Unsup("todo:opaque library object without a constructor the binding can drive", rdj.ty(t))
        if name == "Entity" and len(args) == 1 and not out and gk_overlay.panel_entity(self, ctx, args[0]):
            return gk_overlay.panel_entity(self, ctx, args[0])  # Entity<P: Panel>: a dock panel (11.7)
        if name == "Entity" and len(args) == 1 and out:
            raise Unsup("todo:event payload the binding cannot encode yet", "Entity")
        if name == "Entity" and len(args) == 1:
            return ["entity", self.rtype(ctx, args[0]), last(rdj.ty(args[0]).split("<")[0])]
        if name in ("WeakEntity", "AnyView", "AnyEntity", "AnyWindowHandle", "WindowHandle", "Task", "Subscription"):
            raise Unsup("todo:protocol: state (handles to views, tasks and subscriptions)", name)
        if name in ("Window", "App", "Context", "AsyncApp", "AsyncWindowContext"):
            raise Unsup("todo:protocol: state (a context passed by value)", name)
        if name == "Cow":
            return ["str", "Cow"]
        hc, hid, it = self.api.item(ctx.crate, rp["id"])
        if hc in STD:
            p = hid or ""
            if name == "String":
                return ["str", "String"]
            if out and name == "Range" and len(args) == 1 and args[0].get("primitive") == "usize":
                return ["leaf", "range", ""]  # genapi's Wire for Range<usize>: two u64
            if out and name in ("Duration", "Range", "PathBuf"):
                raise Unsup("todo:event payload the binding cannot encode yet", name)
            if name == "Duration":
                return ["leaf", "duration", ""]
            if name == "Range" and len(args) == 1 and args[0].get("primitive") == "usize":
                return ["leaf", "range", ""]
            if name == "Range":
                raise Unsup("todo:protocol: data (a range over a library type)", rdj.ty(t))
            if name == "PathBuf":
                return ["str", "PathBuf"]
            if name == "Result":
                return gk_overlay.result_desc(self, ctx, args, out)  # Result<(), E> read back (11.6)
            raise Unsup("norule", f"std type {p or name}")
        eg = gk_editor.gpui_value(self, hc, name, args, out)  # ElementId read back, EntityId (PROTOCOLS.md 9.24)
        if eg:
            return eg
        lg = gk_lists.gpui_value(self, hc, name, args, out)  # gpui's Font (PROTOCOLS.md 9.11)
        if lg:
            return lg
        if hc == "gpui" and out and name in EVENT_LEAF:
            return ["leaf", EVENT_LEAF[name], ""]
        if hc in GPUI_HOMES and name in LEAF and not args:
            d = list(LEAF[name])
            if out and d[0] == "leaf" and (d[1] not in ("px", "rems", "color") or d[2]):
                raise Unsup("todo:event payload the binding cannot encode yet", name)
            if out and d[0] == "child":
                raise Unsup("todo:event payload the binding cannot encode yet", name)
            return d
        if hc not in self.api.crates:
            raise Unsup("skip:third-party-type", f"{rp['path']} ({hc})")
        if name in LATER_HANDLES and hc == "gpui":
            raise Unsup("todo:protocol: state (scroll handles, list state, subscriptions and tasks behind handles)", name)
        if name in HANDLES:
            if out:
                raise Unsup("todo:event payload the binding cannot encode yet", name)
            return ["handle", self.rtype(ctx, t), name]
        if it is None:
            raise Unsup("skip:hidden", f"{rp['path']} is not in the public API")
        f = self.info(hc, hid)
        if (hc, hid) in self.node_of and not args and not out and not (self._prefer_rec and f["record"]):
            return ["node", self.node_of[(hc, hid)]]
        a0, acr = (args[0] if len(args) == 1 else None), ctx.crate
        if a0 is not None and a0.get("generic") == "Self" and ctx.self_t:
            acr, a0 = ctx.self_t  # Stateful<Self> in a trait method, read for one implementor
        if a0 is not None and f["kind"] == "struct" and "resolved_path" in a0 and not path_args(a0["resolved_path"]):
            ahc, ahid = self.api.home(acr, a0["resolved_path"]["id"])
            if (hc, hid, (ahc, ahid)) in self.node_of:
                return ["node", self.node_of[(hc, hid, (ahc, ahid))]]
        rt = self.rtype(ctx, t)
        if f["kind"] == "enum":
            return self.enum(ctx, hc, hid, f, rt, args, out)
        if self._closure and out and gk_lists.payload_object(self, hc, hid, f):
            return ["obj", rt, self.api.pubpath(hc, hid), f["clone"]]  # an event's object (PROTOCOLS.md 9.12)
        if not self._closure and self.is_obj(hc, hid):
            return ["obj", rt, self.api.pubpath(hc, hid), f["clone"]]
        if f["state"]:
            raise Unsup("todo:protocol: state (a state object by value)", name)
        if f["record"]:
            return self.record(ctx, hc, hid, f, rt, args, out)
        if f["kind"] == "struct" and out and not f["generic"]:
            return self.record(ctx, hc, hid, f, rt, args, out)  # an event: its public fields
        if f["generic"]:
            raise Unsup("todo:generic library type (needs an instantiation)", rdj.ty(t))
        raise Unsup("todo:opaque library object without a constructor the binding can drive", name)

    def fields(self, ctx, c, ids, out, names=True):
        """[{name, desc}] of struct or variant fields; None when a field cannot be bound."""
        res = []
        for n, fid in enumerate(ids):
            fi = c.item(fid) if fid is not None else None
            if fi is None:
                if out:
                    continue
                return None
            # A field of a record or variant: a library object there is one by handle (PROTOCOLS.md 13.12)
            sv = self._closure  # (read back, a field keeps the older reading: its own record)
            self._closure = sv + 1 if out else 0
            try:
                d = self.desc(ctx, fi["inner"]["struct_field"], out)
            except Unsup:
                if out:
                    continue
                raise
            finally:
                self._closure = sv
            res.append({"name": fi["name"] if names else str(n), "desc": d})
        return res

    def sub_ctx(self, hc, f, ctx, args):
        subst = {}
        for g, a in zip(f["generic"], args):
            subst[g] = self.resolve_arg(ctx, a)
        return Ctx(hc, None, subst)

    def resolve_arg(self, ctx, a):
        """(crate, type) of a generic argument, followed through the current substitution."""
        while "generic" in a:
            g = a["generic"]
            if g == "Self" and ctx.self_t:
                ctx, a = ctx.at(ctx.self_t[0]), ctx.self_t[1]
            elif g in ctx.subst:
                ctx, a = ctx.at(ctx.subst[g][0]), ctx.subst[g][1]
            else:
                raise Unsup("todo:generic type parameter without a closed set", g)
        return ctx.crate, a

    def enum(self, ctx, hc, hid, f, rt, args, out):
        hd = gk_editor.held(self, rt, out)  # read back: a value the library holds (PROTOCOLS.md 9.30)
        if hd is not None:
            return hd
        table = self.enums if rt in self.enums else self.denums
        if rt in table:
            e = table[rt]
            if out and not e["out"]:
                e["out"] = True
            if not out and not e["in"]:
                if not e["constructible"]:
                    raise Unsup("todo:enum with no variant the binding can build", f["name"])
                e["in"] = True
            return [e["kind"], rt]
        if ("e", rt) in self._busy:
            rec = gk_editor.recursive(self, rt, out)  # an enum inside its own variants, by a list (PROTOCOLS.md 9.30)
            if rec is not None:
                return rec
            raise Unsup("todo:recursive type", f["name"])
        self._busy.add(("e", rt))
        try:
            c = self.api.crates[hc]
            sctx = self.sub_ctx(hc, f, ctx, args)
            variants, data = [], False
            for n, vid in enumerate(f["item"]["inner"]["enum"]["variants"]):
                vi = c.item(vid)
                if vi is None:
                    continue
                k = vi["inner"]["variant"]["kind"]
                v = {"name": vi["name"], "n": n, "shape": "unit", "fields": [], "ok": True}
                vc = rdj.target_cfg(vi.get("attrs"))
                if vc:
                    v["cfg"] = vc  # a variant on some platforms only (gpui's WindowKind::LayerShell)
                if isinstance(k, dict):
                    data = True
                    ids = k["tuple"] if "tuple" in k else k["struct"]["fields"]
                    v["shape"] = "tuple" if "tuple" in k else "struct"
                    try:
                        fl = self.fields(sctx, c, ids, out, names="struct" in k)
                    except Unsup:
                        fl = None
                    if fl is None or ("struct" in k and k["struct"].get("has_stripped_fields")) or \
                            (out and len(fl) != len(ids)):
                        v["ok"], v["fields"] = False, []
                    else:
                        v["fields"] = fl
                variants.append(v)
        finally:
            self._busy.discard(("e", rt))
        gk_editor.variant_ctors(self, f, rt, variants, out)  # variants made by a constructor (PROTOCOLS.md 9.29)
        ok = [v for v in variants if v["ok"]]
        if not ok and not out:
            raise Unsup("todo:enum with no variant the binding can build", f["name"])
        e = {"kind": "denum" if data else "enum", "rust": rt, "path": rt.split("<")[0], "name": f["name"],
             "variants": variants, "in": not out, "out": out, "constructible": bool(ok), "crate": hc}
        ec = self.api.walks[hc].cfg.get(str(hid))
        if ec:
            e["cfg"] = ec  # a type on some platforms only (gpui's layer_shell::Layer, Linux)
        (self.denums if data else self.enums)[rt] = e
        return [e["kind"], rt]

    def rec_complete(self, rt, seen=None):
        """Whether a record and every record inside it has all its fields: only such a record can
        be written by the app (one made first as an event's payload may have skipped fields)."""
        seen = set() if seen is None else seen
        r = self.records.get(rt)
        if r is None or rt in seen:
            return r is not None
        seen.add(rt)
        return r["complete"] and all(self.rec_complete(x[1], seen) for fd in r["fields"]
                                     for x in walk_desc(fd["desc"]) if x[0] == "rec")

    def record(self, ctx, hc, hid, f, rt, args, out):
        if rt in self.records:
            r = self.records[rt]
            if not out and not self.rec_complete(rt):
                raise Unsup("todo:opaque library object without a constructor the binding can drive", f["name"])
            r["out" if out else "in"] = True
            return ["rec", rt]
        if ("r", rt) in self._busy:
            raise Unsup("todo:recursive type", f["name"])
        self._busy.add(("r", rt))
        try:
            c = self.api.crates[hc]
            sk = f["item"]["inner"]["struct"]["kind"]
            if sk == "unit":
                ids, tup, total = [], False, 0
            elif "plain" in sk:
                ids, tup = sk["plain"]["fields"], False
                total = len(ids) + (1 if sk["plain"]["has_stripped_fields"] else 0)
            else:
                ids, tup, total = sk["tuple"], True, len(sk["tuple"])
            fl = self.fields(self.sub_ctx(hc, f, ctx, args), c, ids, out, names=not tup)
        finally:
            self._busy.discard(("r", rt))
        complete = fl is not None and len(fl) == total and f["record"]
        if not out and not complete:
            raise Unsup("todo:opaque library object without a constructor the binding can drive", f["name"])
        if out and not fl:
            raise Unsup("todo:event payload without a public field", f["name"])
        self.records[rt] = {"rust": rt, "path": rt.split("<")[0], "name": f["name"], "fields": fl, "tuple": tup,
                            "complete": complete, "in": not out, "out": out, "crate": hc,
                            "args": [last(rdj.ty(self.resolve_arg(ctx, a)[1]).split("<")[0]) for a in args]}
        return ["rec", rt]

    def closure(self, ctx, kind, inputs, output, wrap, static=True):
        """A closure parameter: handler, fragment, build or style closure (DESIGN.md 12)."""
        self._closure += 1
        try:
            return self.closure_desc(ctx, kind, inputs, output, wrap, static)
        finally:
            self._closure -= 1

    def closure_desc(self, ctx, kind, inputs, output, wrap, static=True):
        app = gk_app.action_listener(self, ctx, inputs, output)
        if app:
            return app
        pc = gk_app.platform_closure(self, ctx, kind, inputs, output, wrap)
        if pc:
            return pc  # closures given as data: can_drop, a drag's files, a prepaint order, a11y nodes (PROTOCOLS.md 13.31)
        params, payload = [], []
        try:
            ptypes = [self.rtype(ctx, t) for t in inputs]
        except Unsup as e:
            if e.cat == "norule":
                raise Unsup("todo:protocol: data (a closure over a type the binding cannot name)", e.detail)
            raise
        for t in inputs:
            s = rdj.ty(t)
            base = re.sub(r"^&(mut )?", "", s)
            n = last(base.split("<")[0])
            if n in CONTEXTS and s.startswith("&"):
                params.append(CONTEXTS[n] if s.startswith("&mut") else CONTEXTS[n] + "_ref")
            elif n == "Context" and s.startswith("&mut"):
                params.append("context")
            else:
                params.append("arg")
                payload.append(t)
        out_s = rdj.ty(output) if output else "()"
        has_cx = "window" in params and "app" in params
        queued = "context" in params
        dc = self.data_closure(ctx, inputs, output, wrap, params, ptypes)
        if dc is not None:
            return dc
        vc = gk_values.closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload)
        if vc is not None:
            return vc  # value cells, a settings field's fragment (PROTOCOLS.md 13.19)
        ex = gk_lists.expr_closure(self, ctx, inputs, output, wrap, params, ptypes, payload)
        if ex is not None:
            return ex  # numbers to a number: an expression (PROTOCOLS.md 9.12)
        ed = gk_editor.closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload)
        if ed is not None:
            return ed  # typed fragments, key sets, tables, rows without a window, view rows (PROTOCOLS.md 9.20 to 9.22)
        if output is not None and len(payload) == 1 and rdj.ty(payload[0]) == out_s:
            if last(out_s) == "StyleRefinement" and len(params) == 1:
                return ["stylefn", self.desc(ctx, payload[0])[1], wrap]
            d = self.desc(ctx, payload[0])
            if d[0] == "node":
                if gk_overlay.context_build(params):
                    return ["build", d[1], params, wrap, ptypes]  # a menu's builder (PROTOCOLS.md 11.6)
                if queued or not has_cx and (kind != "FnOnce" or wrap != "impl" or len(params) != 1 or static):
                    dec = None if queued else gk_editor.decorate(self, d[1], kind, params, wrap, ptypes)
                    if dec:
                        return dec  # a decorator kept and called later: a style and a row (PROTOCOLS.md 9.32)
                    raise Unsup("todo:protocol: build closure that gets no window and app", out_s)
                return ["build", d[1], params, wrap, ptypes]
            raise Unsup("todo:protocol: data (a closure that maps a value)", out_s)
        is_el = output is not None and ("impl_trait" in output and "IntoElement" in rdj.ty(output)
                                        or last(out_s) == "AnyElement")
        if output is not None and not is_el and "generic" in output and output["generic"] in ctx.bounds:
            is_el = "IntoElement" in [n for n, _ in bound_parts(ctx.bounds[output["generic"]])]
        if is_el and has_cx and not queued and len(payload) == 1 and self.datum_arg(ctx, payload[0]) is not None:
            # An element for one row of a data table (a chart's tooltip content): the row's view
            # of a row source the app passes, keyed by the row's index (docs/PROTOCOLS.md
            # section 12), described between frames like any row; one frame of nothing at first.
            return ["datum_rows", wrap, ptypes, self.datum_arg(ctx, payload[0])]
        if is_el:
            cr = payload and gk_lists.ctx_rows(self, ctx, kind, inputs, output, wrap, params, ptypes, payload)
            if cr:
                return cr  # an element closure over values the kit hands it: a row source (PROTOCOLS.md 9.16)
            if payload:
                bo = gk_items.build_over_handed(self, ctx, kind, inputs, output, wrap, params, ptypes, payload, static)
                if bo:
                    return bo  # FnOnce(Button) -> E: a build closure over the element handed in (PROTOCOLS.md 9.41)
                ic = gk_items.closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload, True)
                if ic:
                    return ic  # an item procedure (PROTOCOLS.md 9.41)
                raise Unsup("skip:item-render-closure", f"{kind}({', '.join(rdj.ty(t) for t in inputs)}) -> {out_s}")
            if params == ["window", "app_ref"]:
                return gk_lists.view_closure(params, wrap, ptypes)  # an app view (PROTOCOLS.md 9.9)
            if not has_cx or queued:
                raise Unsup("todo:protocol: fragment closure that gets no window and app", out_s)
            return ["fragment", params, wrap, ptypes]
        if out_s not in ("()", "bool") and output is not None:
            # A closure a delegate shim serves over a row source ([rows] in overrides.toml): upstream
            # calls it while it draws and it answers with rows the app described earlier
            # (docs/PROTOCOLS.md section 9.5). Named by its signature in public paths.
            try:
                sig = f"{kind}({', '.join(ptypes)}) -> {self.rtype(ctx, output)}"
            except Unsup:
                sig = None
            shim = self.ov.get("rows", {}).get(sig) if sig else None
            if shim:
                self.ov_used.add("rows." + sig)
                return ["rows", shim]
        ev = gk_values.event_closure(self, ctx, kind, ptypes, output, params, payload, wrap)
        if ev is not None:
            return ev  # a closure upstream calls from an event, returning text (PROTOCOLS.md 13.12)
        ask = gk_lists.ask_closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload)
        if ask is not None:
            return ask  # a data closure answered between frames (PROTOCOLS.md 9.14)
        if payload and output is not None and out_s not in ("()", "bool"):
            ic = gk_items.closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload, False)
            if ic:
                return ic  # an item procedure that returns one element type (PROTOCOLS.md 9.41)
        if out_s not in ("()", "bool"):
            raise Unsup("todo:protocol: data (a closure that returns a value)", f"-> {out_s}")
        if queued and out_s != "()":
            raise Unsup("todo:protocol: queued events (a queued handler cannot return a value)", out_s)
        if not (has_cx or queued or "app" in params) and not gk_overlay.observed(params, out_s, kind):  # 11.14
            raise Unsup("todo:protocol: data (a closure that gets no window and app)", ", ".join(params))
        descs = []
        for t in payload:
            try:
                descs.append(self.desc(ctx, t, out=True))
            except Unsup as e:
                raise Unsup("todo:event payload the binding cannot encode yet", f"{rdj.ty(t)}: {e.detail}")
        ret = "bool" if out_s == "bool" else "unit"
        key = json.dumps([[strip_conv(d) for d in descs], ret])
        if key not in self.shapes:
            self.shapes[key] = {"key": key, "args": [strip_conv(d) for d in descs], "ret": ret,
                                "rust": [rdj.ty(t) for t in payload],
                                "builtin": BUILTIN_SHAPES.get((json.dumps([strip_conv(d) for d in descs]), ret))}
        return ["handler", key, params, wrap, ptypes, descs]

    # ---------------------------------------------------------------- data closures (section 12)

    def datum_arg(self, ctx, t):
        """How a closure parameter reaches a row of a data table: "" for `&Datum`, the field's
        name for a borrowed wrapper that carries one (`&ArcData<Datum>`: `data`), None otherwise."""
        if "borrowed_ref" not in t:
            return None
        inner = t["borrowed_ref"]["type"]
        try:
            c, a = self.resolve_arg(ctx, inner)
        except Unsup:
            return None
        if "verbatim" in a:
            return "" if a["verbatim"] in self.data_types() else None
        if "resolved_path" not in a:
            return None
        args = path_args(a["resolved_path"])
        if len(args) != 1:
            return None
        try:
            _, x = self.resolve_arg(Ctx(c, ctx.self_t, ctx.subst, ctx.bounds), args[0])
        except Unsup:
            return None
        if "verbatim" not in x or x["verbatim"] not in self.data_types():
            return None
        hc, hid, it = self.api.item(c, a["resolved_path"]["id"])
        if it is None or "struct" not in it["inner"] or "plain" not in it["inner"]["struct"]["kind"]:
            return None
        cr = self.api.crates[hc]
        param = (it["inner"]["struct"]["generics"]["params"] or [{}])
        tps = [p["name"] for p in param if "type" in p.get("kind", {})]
        for fid in it["inner"]["struct"]["kind"]["plain"]["fields"]:
            fi = cr.item(fid)
            ft = fi["inner"]["struct_field"] if fi else {}
            if fi and fi["visibility"] == "public" and "borrowed_ref" in ft and \
                    ft["borrowed_ref"]["type"].get("generic") in tps:
                return fi["name"]
        return None

    def ret_kind(self, ctx, t):
        """What a data closure returns: (kind, Rust type) with kind text, num, opt or color; None
        when the closure returns something no data closure makes."""
        if t is None:
            return None
        if "generic" in t:
            g = t["generic"]
            if g in ctx.subst:
                c, a = ctx.subst[g]
                return self.ret_kind(ctx.at(c), a)
            for n, tr in bound_parts(ctx.bounds.get(g, [])):
                if n == "Into" and len(path_args(tr)) == 1:
                    return self.ret_kind(ctx, path_args(tr)[0])
            return None
        if "primitive" in t:
            return ("num", t["primitive"]) if t["primitive"] in ("f32", "f64") else None
        if "verbatim" in t or "resolved_path" not in t:
            return None
        rp = t["resolved_path"]
        name, args = last(rp["path"]), path_args(rp)
        if name == "Option" and len(args) == 1 and args[0].get("primitive") in ("f32", "f64"):
            return ("opt", args[0]["primitive"])
        if name in ("SharedString", "Hsla", "Background", "String"):
            return ("text" if name in ("SharedString", "String") else "color",
                    "String" if name == "String" else "gpui_kit::" + name)
        try:
            rt = self.rtype(ctx, t)
        except Unsup:
            return None
        hc, hid, it = self.api.item(ctx.crate, rp["id"])
        if it is None or args:
            return None
        # An upstream type made from a string (RadarLabel: From<SharedString>): text, converted.
        c = self.api.crates[hc]
        for imp_id in it["inner"][c.kind(it)].get("impls", []):
            ii = c.item(imp_id)
            im = ii["inner"]["impl"] if ii else None
            if im and im.get("trait") and last(im["trait"]["path"]) == "From" and not im.get("blanket_impl"):
                src = path_args(im["trait"])
                if src and "resolved_path" in src[0] and last(src[0]["resolved_path"]["path"]) == "SharedString":
                    return ("text", rt)
        return None

    def data_closure(self, ctx, inputs, output, wrap, params, ptypes):
        """A closure upstream calls while it draws, over a row of a data table and numbers it
        hands in, returning a value: app code may not run then (decision 3), so the app names
        what it returns instead (docs/PROTOCOLS.md section 12). `field`: a column of the row
        (`->x("month")`); `format`: a template over the numbers and the row's columns
        (`->tooltip_value("{} visits")`). None when the closure is not one of these."""
        if output is None or any(p != "arg" for p in params) or not inputs:
            return None
        forms = []
        for t in inputs:
            k = self.datum_arg(ctx, t)
            if k is not None:
                forms.append(["datum", k])
                continue
            try:
                d = self.desc(ctx, t)
            except Unsup:
                d = ["other"]
            d = d[1] if d[0] == "ref" else d
            if d[0] in ("f32", "f64"):
                forms.append([d[0]])
            elif d[0] == "int":
                forms.append(["int", d[1]])
            else:
                forms.append(["other"])  # a field closure ignores it; a template cannot show it
        datums = [f for f in forms if f[0] == "datum"]
        if len(datums) > 1:
            return None
        rk = self.ret_kind(ctx, output)
        if rk is None:
            return None
        if datums and (len(forms) == 1 or rk[0] != "text"):
            # A colour or number from a row with other arguments too (a series index, the value):
            # the row's column; the app writes into it what it would have computed.
            return ["field", rk[0], rk[1], wrap, ptypes, forms]
        if rk[0] == "text" and ["other"] not in forms:
            return ["format", rk[0], rk[1], wrap, ptypes, forms]
        return None

    # ---------------------------------------------------------------- functions

    def fn_ctx(self, base, fn):
        """The context of one function: its generic parameters' bounds."""
        bounds = dict(base.bounds)
        for p in fn["generics"]["params"]:
            if "type" in p["kind"]:
                bounds[p["name"]] = list(p["kind"]["type"]["bounds"])
        for w in fn["generics"].get("where_predicates", []):
            bp = w.get("bound_predicate")
            if bp and "generic" in bp["type"] and bp["type"]["generic"] != "Self":
                bounds.setdefault(bp["type"]["generic"], [])
                bounds[bp["type"]["generic"]] = bounds[bp["type"]["generic"]] + bp["bounds"]
        c = Ctx(base.crate, base.self_t, base.subst, bounds)
        return gk_values.fn_ctx(self, c, fn)  # drag values, Into-only parameters (PROTOCOLS.md 13.20)

    def args(self, ctx, params):
        res = []
        for n, (pname, pt) in enumerate(params):
            d = self.desc(ctx, pt)
            if d == ["unit"]:
                raise Unsup("norule", "unit parameter")
            if any(x[0] == "datum" for x in walk_desc(d)):
                raise Unsup("todo:protocol: data (one row of a data table, not a whole table)", rdj.ty(pt))
            pname = pname if re.fullmatch(r"[a-z][a-z0-9_]*", pname) else f"arg{n}"
            res.append({"name": pname, "desc": d})
        return res

    def ret_node(self, ctx, out):
        """The node key a return type names, 'self', or None."""
        if out is None:
            return None
        if "generic" in out and out["generic"] == "Self":
            return "self"
        try:
            d = self.desc(ctx, out)
        except Unsup:
            return None
        return d[1] if d[0] == "node" else None

    def new_item(self, row, owner, f, fcrate, trait=None):
        fn = f["inner"]["function"]
        it = {"row": row["name"], "owner": owner, "name": f["name"], "sig": rdj.fn_sig(f["name"], fn),
              "trait": trait, "status": "todo", "reason": "", "kind": "", "args": [], "crate": fcrate}
        it["id"] = f"{owner}::{trait + '::' if trait else ''}{f['name']}"
        if f.get("id") is not None:
            it["doc"] = [fcrate, str(f["id"])]  # its doc comment in api/rustdoc/docs.json.gz (gen/gk_doc.py)
        self.items.append(it)
        row["items"].append(it)
        return it, fn

    def set(self, it, e):
        """Records why an item is not bound."""
        cat = e.cat
        if cat.startswith("skip:"):
            it["status"], it["reason"] = "skipped", cat[5:]
            assert it["reason"] in SKIP_REASONS, it["reason"]
        elif cat.startswith("todo:"):
            it["status"], it["reason"] = "todo", cat[5:]
        else:
            it["status"], it["reason"] = "norule", e.detail
        it["detail"] = e.detail

    def bindable(self, it, ctx, fn, params, kind, node, callee):
        """Fills an item bound by rule: its arguments and where the op lives."""
        try:
            text = " ".join(rdj.ty(t) for _, t in params)
            # A parameter named only in the bounds of one that is named (`fill<F, B>(f: F) where
            # F: Fn(&T) -> B`) is chosen by the closure's return type: it is named too.
            for w in fn["generics"].get("where_predicates", []):
                bp = w.get("bound_predicate")
                if bp and "generic" in bp["type"] and re.search(rf"\b{bp['type']['generic']}\b", text):
                    text += " " + rdj.bounds_str(bp["bounds"])
            for p in fn["generics"]["params"]:
                if "type" in p["kind"] and re.search(rf"\b{p['name']}\b", text):
                    text += " " + rdj.bounds_str(p["kind"]["type"]["bounds"])
            for p in fn["generics"]["params"]:
                if "type" in p["kind"] and not p["kind"]["type"].get("is_synthetic") and \
                        not re.search(rf"\b{p['name']}\b", text) and not gk_app.bound_only(fn, p["name"]):
                    raise Unsup("todo:generic type parameter the caller chooses (no argument names it)", p["name"])
            it["args"] = self.args(self.fn_ctx(ctx, fn), params)
            if not self.obj_arg_ok(it):
                raise Unsup("todo:protocol: object (an element argument that cannot be copied out of its handle)", text)
        except Unsup as e:
            self.set(it, e)
            return False
        it["kind"], it["status"], it["callee"] = kind, "bound", callee
        if any(uses_protocol(a["desc"]) for a in it["args"]):
            it["status"] = "bridged"
        return True

    def classify_method(self, row, node, f, fcrate, ctx, role):
        """One inherent method of a type."""
        it, fn = self.new_item(row, row["path"], f, fcrate)
        ins = fn["sig"]["inputs"]
        out = fn["sig"].get("output")
        if self.pre(it, f, fn):
            return
        recv = rdj.ty(ins[0][1]) if ins and ins[0][0] == "self" else None
        fctx = self.fn_ctx(ctx, fn)
        if role != "node":
            reason = {"state": "todo:protocol: state (an object with reads and calls that is not an entity: needs a handle kind)",
                      "state-generic": "todo:protocol: state (a state type generic over a delegate or mode: needs its bridge type)",
                      "generic": "todo:generic library type (needs an instantiation)",
                      "data": "skip:value-helper"}[role]
            return self.set(it, Unsup(reason, role))
        rn = self.ret_node(fctx, out)
        if rn == "self":
            rn = node["key"]
        tp = node["path"]
        if recv is None:
            # values made by a static that returns several (PROTOCOLS.md 9.15): its _count and _at,
            # beside the static itself read back as an ambient call when that rule binds it (13.15)
            listed = rn is None and gk_lists.list_ctor(self, node, it, fcrate, fn, fctx)
            if rn is None and gk_values.static_call(self, it, fcrate, fn, ctx.self_t):
                return  # a static that returns a value or nothing: an ambient call (13.12)
            if listed:
                return
            if rn is None:
                return self.set(it, Unsup("todo:protocol: calls (a static function that is not a constructor)",
                                          rdj.ty(out) if out else "()"))
            if self.bindable(it, ctx, fn, ins, "ctor", node, f"<{tp}>::{f['name']}"):
                it["node"] = rn
                self.nodes[rn]["ctors"].append(it)
        elif recv == "Self":
            if rn is None:
                return self.set(it, Unsup("skip:rust-only-extension", "consumes the builder: " + (rdj.ty(out) if out else "()")))
            if rn == node["key"]:
                if self.bindable(it, ctx, fn, ins[1:], "op", node, f"<{tp}>::{f['name']}"):
                    node["ops"].append(it)
            elif self.bindable(it, ctx, fn, ins[1:], "convert", node, f"<{tp}>::{f['name']}"):
                it["node"], it["from"] = rn, node["key"]
                self.nodes[rn]["ctors"].append(it)
                node.setdefault("converts", []).append(it)
        elif recv == "&Self" and self.paints(it, node, f, fn, ins, out, tp):
            pass
        elif recv == "&Self" and gk_overlay.value_read(self, node, it, fcrate, fn, fctx):
            pass  # a computation over a value the app builds: an ambient call (PROTOCOLS.md 11.15)
        elif recv == "&Self":
            self.set(it, Unsup("skip:builder-getter", "a read of a builder the app is writing"))
        elif recv == "&mut Self" and gk_editor.mutator(self, node, it, fn, ctx, tp):
            pass  # the in-place builder of an editor host value, applied as an op (PROTOCOLS.md 9.30)
        elif recv == "&mut Self":
            self.set(it, Unsup("skip:builder-mutator", "the in-place form of a builder"))
        else:
            self.set(it, Unsup("skip:rust-only-extension", f"receiver {recv}"))

    PAINT_CTX = {"&mut Window": "window", "&mut gpui::Window": "window", "&mut App": "app", "&mut gpui::App": "app"}

    def paints(self, it, node, f, fn, ins, out, tp):
        """`fn paint(&self, bounds: &Bounds<Pixels>, window: &mut Window[, cx: &mut App])` of a
        value type (an axis, a grid, a plot shape): a part of a described plot, which the library
        paints in its order (docs/PROTOCOLS.md section 12, "plots")."""
        if f["name"] != "paint" or out is not None or node["element"] or len(ins) not in (3, 4):
            return False
        if rdj.ty(ins[1][1]).replace("gpui::", "") != "&Bounds<Pixels>":
            return False
        ctx = [self.PAINT_CTX.get(rdj.ty(t)) for _, t in ins[2:]]
        if ctx not in (["window"], ["window", "app"]):
            return False
        it.update(kind="paint", status="bridged", callee=f"<{tp}>::paint", args=[], node=node["key"],
                  paint_app=ctx == ["window", "app"])
        node["paint"] = it
        return True

    # ---------------------------------------------------------------- kit state behind handles

    CALL_SAFE = {"bool", "int", "f32", "f64", "str", "leaf", "enum", "opt", "list", "tuple", "ref", "entity", "shim",
                 "obj", "refmut", "libval", "char", "font", "handler", "expr", "nonzero", "ask", "rangeof", "rcvec", "arcslice"}

    def classify_state(self, row, f, c, crate, ctx, tpath):
        """A kit state type held in an entity: constructors, the `self -> Self` chain, reads
        (&self), deferred writes (&mut self) and events (docs/PROTOCOLS.md section 5)."""
        st = {"key": tpath, "path": tpath, "name": f["name"], "crate": crate, "ctors": [], "builders": [],
              "reads": [], "writes": [], "calls": [], "events": []}
        st["doc"] = [f["crate"], str(f["id"])]
        self.states[tpath] = st
        seen = set()
        for im in f["impls"]:
            tr = im.get("trait")
            if tr is not None:
                if last(tr["path"]) == "EventEmitter" and path_args(tr):
                    try:
                        self._closure += 1
                        try:
                            d = self.desc(Ctx(crate, ctx.self_t), path_args(tr)[0], out=True)
                        finally:
                            self._closure -= 1
                        key = json.dumps([[strip_conv(d)], "unit"])
                        self.shapes.setdefault(key, {"key": key, "args": [strip_conv(d)], "ret": "unit",
                                                     "rust": [rdj.ty(path_args(tr)[0])],
                                                     "builtin": BUILTIN_SHAPES.get((json.dumps([strip_conv(d)]), "unit"))})
                        st["events"].append({"name": last(rdj.ty(path_args(tr)[0]).split("<")[0]), "shape": key,
                                             "rust": self.rtype(Ctx(crate, ctx.self_t), path_args(tr)[0]),
                                             "id": f"{tpath}::subscribe::{rdj.ty(path_args(tr)[0])}", "status": "bridged"})
                    except Unsup:
                        gk_editor.event_reads(self, st, crate, ctx, tpath, path_args(tr)[0])  # its reads (PROTOCOLS.md 9.35)
                continue
            for mid in im["items"]:
                fi = c.item(mid)
                if not fi or "function" not in fi["inner"] or fi["visibility"] != "public" or fi["name"] in seen:
                    continue
                seen.add(fi["name"])
                self.state_method(row, st, fi, crate, ctx)

    def state_method(self, row, st, f, crate, ctx):
        it, fn = self.new_item(row, row["path"], f, crate)
        if self.pre(it, f, fn):
            return
        ins, out = fn["sig"]["inputs"], fn["sig"].get("output")
        recv = rdj.ty(ins[0][1]) if ins and ins[0][0] == "self" else None
        fctx = self.fn_ctx(ctx, fn)
        out_s = rdj.ty(out) if out else "()"
        out, out_s = gk_lists.shim_result(self, fctx, out, out_s)  # `&D`, `&mut D`: the delegate's source
        kind = {None: "state-ctor", "Self": "state-builder", "&Self": "state-read", "&mut Self": "state-write"}.get(recv)
        if kind == "state-read" and out_s == "()" and any(
                re.match(r"&mut (Window|Context)\b", rdj.ty(t)) for _, t in ins[1:]):
            # `fn focus(&self, window: &mut Window, cx: &mut Context<Self>)`: it needs the
            # entity's update to get its context, so it is a buffered write like `&mut self`.
            kind = "state-write"
        if kind == "state-read" and out_s != "()" and gk_overlay.read_needs_window(ins):
            kind = "state-call"  # a read that needs the window: run at once with the turn's (11.6)
        fallible = kind in ("state-ctor", "state-builder") and gk_overlay.is_result_self(out_s, st["name"])
        lent = None
        try:
            if kind is None:
                raise Unsup("skip:rust-only-extension", f"receiver {recv}")
            if kind in ("state-ctor", "state-builder") and out_s not in ("Self", st["name"]) and not fallible:
                raise Unsup("todo:protocol: calls (a static function that is not a constructor)" if recv is None
                            else "skip:rust-only-extension", out_s)
            if kind == "state-write" and out_s != "()":
                # An update that returns a value: a call op run at once (not buffered), whose
                # result comes back like a read's (PROTOCOLS.md section 5, "calls").
                lent = "&" in out_s and gk_lists.lend(self, fctx, st, out)  # lent by the state (PROTOCOLS.md 9.17)
                if "&" in out_s and not lent:
                    raise Unsup("todo:protocol: state (an update that returns a value)", "a borrow: " + out_s)
                kind = "state-call"
            args = []
            text = gk_overlay.named_text(fn, " ".join(rdj.ty(t) for _, t in ins))  # where clauses (11.10)
            for p in fn["generics"]["params"]:
                if "type" in p["kind"] and not p["kind"]["type"].get("is_synthetic") and not re.search(rf"\b{p['name']}\b", text):
                    raise Unsup("todo:generic type parameter the caller chooses (no argument names it)", p["name"])
            for n, (pn, pt) in enumerate(ins if recv is None else ins[1:]):
                s = rdj.ty(pt)
                base = last(re.sub(r"^&(mut )?", "", s).split("<")[0])
                if s.startswith("&") and base in ("Window", "App", "Context"):
                    how = {"Window": "window", "App": "app", "Context": "context"}[base] + ("" if s.startswith("&mut") else "_ref")
                    ok = {"state-ctor": ("window", "context", "app"), "state-builder": (), "state-read": ("app_ref",),
                          "state-write": ("window", "window_ref", "context", "app", "app_ref"),
                          "state-call": ("window", "window_ref", "context", "app", "app_ref")}[kind]
                    if how not in ok:
                        raise Unsup("todo:protocol: state (a call that needs a context this kind of call has not got)", s)
                    args.append({"name": pn, "desc": ["ctx", how]})
                    continue
                d = self.call_desc(fctx, pt)
                bad = self.call_bad(d, kind == "state-builder")
                if bad and gk_editor.chain_cx(self, d, kind, it):
                    bad = None  # decoded before the chain runs, with the call's context (PROTOCOLS.md 9.31)
                if bad:
                    raise Unsup("todo:protocol: state (an argument only a view build can decode: " + bad + ")", s)
                pn = pn if re.fullmatch(r"[a-z][a-z0-9_]*", pn) else f"arg{n}"
                args.append({"name": pn, "desc": d})
            it["args"] = args
            if kind == "state-read":
                if out is None:
                    raise Unsup("todo:protocol: state (a call through &self that returns nothing)", "")
                try:
                    it["ret"] = self.desc(fctx, out, out=True)
                except Unsup as e:
                    raise Unsup("todo:protocol: state (a read whose result the binding cannot carry yet)", e.detail or out_s)
                self.check_obj_ret(it["ret"], out_s)
            if kind == "state-call" and lent:
                it["ret"] = lent
            elif kind == "state-call":
                try:
                    it["ret"] = self.desc(fctx, out, out=True)
                except Unsup as e:
                    raise Unsup("todo:protocol: state (an update that returns a value)", e.detail or out_s)
                self.check_obj_ret(it["ret"], out_s)
        except Unsup as e:
            return self.set(it, e)
        it.update(kind=kind, status="bridged", callee=f"<{st['path']}>::{f['name']}", state=st["key"])
        if fallible:  # Result<Self, E>: an error stops the app with its text (PROTOCOLS.md 11.6)
            it["callee"] = gk_overlay.fallible_callee(it["callee"], len(it["args"]) + (kind == "state-builder"))
        st[{"state-ctor": "ctors", "state-builder": "builders", "state-read": "reads", "state-write": "writes",
            "state-call": "calls"}[kind]].append(it)

    def call_bad(self, d, builder, seen=None):
        """The first part of an argument a call op cannot decode, or None. A call op has no
        decode context (no view, no element tree): scalars, strings, options, lists, tuples,
        handles to kit state, and records, enums with data and Into sets made only of those
        (decoded by their `_c` codecs). A colour (resolved against the theme) and a focus handle
        at the top level of a call that is not a builder, whose op has the app context."""
        seen = set() if seen is None else seen
        eb = None if seen else gk_editor.call_bad(self, d, builder)  # a record with a colour, in place (PROTOCOLS.md 9.26)
        if eb is not None:
            return eb[0]
        for x in walk_desc(d):
            k = x[0]
            if k == "node" and not builder and not seen and x[1] in self.nodes and not self.nodes[x[1]]["element"]:
                continue  # a builder value made outside a view travels as a tape of its own (PROTOCOLS.md 13.6)
            if k == "opt" and x[1][0] == "ref" and x[1][1][0] == "obj" and (builder or seen):
                return "obj"  # Option<&Object> inside a record or a builder chain: nothing to lend from
            if k == "refmut" and (builder or seen) or k == "obj" and (builder or seen) and not x[3]:
                return k  # inside a record or a builder chain an object is copied out of its handle (13.12)
            if k == "rec" and x[1] in self.records and not self.records[x[1]]["complete"]:
                return "rec"  # a record with fields the binding cannot write: only read
            if gk_overlay.view_built(self, x, builder, seen):
                continue  # a view-built closure (PROTOCOLS.md 11.6)
            if k in gk_editor.CALL_SAFE:
                continue  # the editor family's PROTOCOLS.md 9.21 to 9.24
            if gk_app.call_safe(x):
                continue  # image sources as bytes and paths, anchors, caches (PROTOCOLS.md 13.31)
            if k in gk_lists.CALL_SAFE:
                continue  # a row source handle (PROTOCOLS.md 9.16, 9.18)
            if k in gk_values.CALL_SAFE or (k == "ctx" and x[1] == "window_opt" and not builder and not seen):
                continue  # the values family's descriptors (PROTOCOLS.md 13.10)
            if k in self.CALL_SAFE and not (k == "leaf" and x[1] == "color") and not (k == "entity" and builder):
                continue
            if k == "from":
                continue
            if k == "leaf" and x[1] == "color" and not builder and not seen:
                continue
            if k == "handle" and not builder and not seen:
                continue
            if k in ("rec", "denum", "into"):
                table = {"rec": self.records, "denum": self.denums, "into": self.intos}[k]
                t = table.get(x[1])
                if t is None:
                    return k
                if (k, x[1]) in seen:
                    continue
                seen.add((k, x[1]))
                parts = ([f["desc"] for f in t["fields"]] if k == "rec" else
                         [f["desc"] for v in t["variants"] if v["ok"] for f in v["fields"]] if k == "denum" else
                         [m["desc"] for m in t["members"]])
                for p in parts:
                    bad = self.call_bad(p, builder, seen)
                    if bad:
                        return bad
                t["callable"] = True
                continue
            return k
        return None

    # ---------------------------------------------------------------- library objects behind handles

    def is_obj(self, hc, hid):
        """A library object the app holds by handle (docs/PROTOCOLS.md section 13): a struct of
        the kit that is not an entity, an element or builder, a plain record or generic, and has
        reads or calls (`&self`, `&mut self`) or is a kit global."""
        if hc not in KIT or hid is None:
            return False
        key = ("obj?", hc, hid)
        if key in self.tinfo:
            return self.tinfo[key]
        f = self.info(hc, hid)
        inner = f["item"]["inner"].get("struct")
        ok = (f["kind"] == "struct" and inner is not None and not f["entity"] and not f["generic"]
              and not f["record"] and (hc, hid) not in self.node_of
              and not any("lifetime" in p["kind"] for p in inner["generics"]["params"])
              and bool(f["state"] or f["refs"]) and bool(self.api.pubpath(hc, hid)))
        self.tinfo[key] = ok
        return ok

    def classify_object(self, row, f, c, crate, ctx, tpath):
        """A library object: constructors and their `self -> Self` chain (a kept handle), the
        global accessor of a kit global, reads, buffered writes, calls that return a value, and
        the type's static functions (docs/PROTOCOLS.md section 13)."""
        st = {"key": tpath, "path": tpath, "name": f["name"], "crate": crate, "ctors": [], "builders": [],
              "reads": [], "writes": [], "calls": [], "events": [], "globals": [], "statics": [], "obj": True,
              "glob": "Global" in f["traits"], "clone": f["clone"]}
        st["doc"] = [f["crate"], str(f["id"])]
        self.states[tpath] = st
        self.objects[tpath] = st
        seen = set()
        for im in f["impls"]:
            if im.get("trait") is not None:
                continue
            for mid in im["items"]:
                fi = c.item(mid)
                if not fi or "function" not in fi["inner"] or fi["visibility"] != "public" or fi["name"] in seen:
                    continue
                seen.add(fi["name"])
                self.object_method(row, st, fi, crate, ctx)
        gk_lists.default_ctor(self, st, f, crate)  # Default as the constructor (PROTOCOLS.md 9.13)

    OBJ_CTX = {"obj-ctor": ("window", "app", "app_ref"), "obj-builder": (), "obj-read": ("app_ref",),
               "obj-write": ("window", "window_ref", "app", "app_ref"), "obj-call": ("window", "window_ref", "app", "app_ref"),
               "obj-global": ("app", "app_ref"), "obj-static": ("window", "window_ref", "app", "app_ref"),
               "obj-scall": ("window", "window_ref", "app", "app_ref")}

    def object_method(self, row, st, f, crate, ctx):
        it, fn = self.new_item(row, row["path"], f, crate)
        if self.pre(it, f, fn):
            return
        ins, out = fn["sig"]["inputs"], fn["sig"].get("output")
        recv = rdj.ty(ins[0][1]) if ins and ins[0][0] == "self" else None
        fctx = self.fn_ctx(ctx, fn)
        out_s = rdj.ty(out) if out else "()"
        names = ("Self", st["name"])
        rest = ins if recv is None else ins[1:]
        mut_cx = any(re.match(r"&mut (Window|App)\b", rdj.ty(t)) for _, t in rest)
        try:
            if recv is None:
                if out_s in names:
                    kind = "obj-ctor"
                elif out_s in ("&Self", "&mut Self", "&" + st["name"], "&mut " + st["name"]):
                    if not st["glob"]:
                        raise Unsup("todo:protocol: object (a static function that lends an object it keeps)", out_s)
                    kind = "obj-global"
                elif out_s == "()":
                    kind = "obj-static"
                else:
                    kind = "obj-scall"
            elif recv == "Self":
                if out_s in names:
                    kind = "obj-builder"
                elif out_s == "()":
                    kind, it["consume"] = "obj-write", True
                else:
                    kind, it["consume"] = "obj-call", True
            elif recv == "&Self":
                kind = ("obj-write" if out_s == "()" else "obj-call") if mut_cx else ("obj-write" if out_s == "()" else "obj-read")
            elif recv == "&mut Self":
                kind = "obj-write" if out_s == "()" else "obj-call"
                it["mut"] = True
            else:
                raise Unsup("skip:rust-only-extension", f"receiver {recv}")
            text = gk_overlay.named_text(fn, " ".join(rdj.ty(t) for _, t in ins))  # where clauses (11.10)
            for p in fn["generics"]["params"]:
                if "type" in p["kind"] and not p["kind"]["type"].get("is_synthetic") and not re.search(rf"\b{p['name']}\b", text):
                    raise Unsup("todo:generic type parameter the caller chooses (no argument names it)", p["name"])
            args = []
            for n, (pn, pt) in enumerate(rest):
                s = rdj.ty(pt)
                base = last(re.sub(r"^&(mut )?", "", s).split("<")[0])
                if s.startswith("&") and base in ("Window", "App", "Context"):
                    if base == "Context":
                        raise Unsup("todo:protocol: state (a call that needs a context this kind of call has not got)", s)
                    how = {"Window": "window", "App": "app"}[base] + ("" if s.startswith("&mut") else "_ref")
                    if how not in self.OBJ_CTX[kind]:
                        raise Unsup("todo:protocol: state (a call that needs a context this kind of call has not got)", s)
                    args.append({"name": pn, "desc": ["ctx", how]})
                    continue
                try:
                    d = self.call_desc(fctx, pt)
                except Unsup as e:
                    if e.cat == "norule" and "dyn " in s:
                        raise Unsup("todo:protocol: delegates (a trait the app implements: one bridge per trait)", s)
                    raise
                bad = self.call_bad(d, kind == "obj-builder")
                if bad and gk_editor.chain_cx(self, d, kind, it):
                    bad = None  # decoded before the chain runs, with the call's context (PROTOCOLS.md 9.31)
                if bad:
                    raise Unsup("todo:protocol: state (an argument only a view build can decode: " + bad + ")", s)
                pn = pn if re.fullmatch(r"[a-z][a-z0-9_]*", pn) else f"arg{n}"
                args.append({"name": pn, "desc": d})
            if kind == "obj-global" and any(a["desc"][0] != "ctx" for a in args):
                raise Unsup("todo:protocol: object (a global accessor with arguments)", text)
            it["args"] = args
            if kind in ("obj-read", "obj-call", "obj-scall"):
                try:
                    it["ret"] = self.desc(fctx, out, out=True)
                except Unsup as e:
                    raise Unsup("todo:protocol: state (a read whose result the binding cannot carry yet)", e.detail or out_s)
                self.check_obj_ret(it["ret"], out_s)
            if "'" in out_s and kind in ("obj-read", "obj-call", "obj-scall") and not gk_overlay.borrows_arg(fn, out_s, ins):
                raise Unsup("todo:protocol: object (a result that borrows from its object)", out_s)
        except Unsup as e:
            return self.set(it, e)
        it.update(kind=kind, status="bridged", callee=f"<{st['path']}>::{f['name']}", state=st["key"])
        st[{"obj-ctor": "ctors", "obj-builder": "builders", "obj-read": "reads", "obj-write": "writes",
            "obj-call": "calls", "obj-global": "globals", "obj-static": "statics", "obj-scall": "statics"}[kind]].append(it)

    def call_desc(self, ctx, t):
        """An argument of a call op: a builder type that is also a plain record (IndexPath, whose
        fields are public) is written as the record, which a call op can decode; as a builder it
        could be made only inside a view build (PROTOCOLS.md section 13)."""
        d = self.desc(ctx, t)
        if not any(x[0] == "node" for x in walk_desc(d)):
            return d
        self._prefer_rec = True
        try:
            return self.desc(ctx, t)
        except Unsup:
            return d
        finally:
            self._prefer_rec = False

    def check_obj_ret(self, d, out_s):
        """An object travels out as a new temporary handle: by value it is moved into the library,
        anywhere else (a borrow, an option, a list) it is copied, so its type must be Clone."""
        if d[0] == "obj" or (d[0] == "fallible" and d[1][0] == "obj"):
            return  # by value: moved into a new handle (a fallible one too, PROTOCOLS.md 13.9)
        for x in walk_desc(d):
            if x[0] == "obj":
                if not x[3]:
                    raise Unsup("todo:protocol: object (a result that cannot be copied out of a borrow)", out_s)

    def obj_arg_ok(self, it):
        """An element builder's object argument is copied out of its handle each time the tape is
        decoded: by reference or by value, of a type that is Clone."""
        for a in it["args"]:
            for x in walk_desc(a["desc"]):
                if x[0] == "refmut":
                    return False
                if x[0] == "obj" and not x[3]:
                    return False
        return True

    def reach_objects(self):
        """The fixpoint over library objects the app can get: a bound constructor or global
        accessor, or a bound function that hands one out (a result, an event payload). The
        methods of one nothing hands out, and functions that take one, are not reachable."""
        why = "protocol: object (no constructor, global or bound function hands one out)"
        objs = self.objects
        items = self.all_items()
        live = lambda i: i["status"] in ("bound", "bridged")  # noqa: E731
        reach = set()
        while True:
            new = set()
            for key, st in objs.items():
                if key not in reach and any(live(i) for i in st["ctors"] + st["globals"]):
                    new.add(key)
                if key not in reach and any(live(i) and any(x[0] == "obj" and x[2] == key for x in walk_desc(i.get("ret") or ["unit"]))
                                            for i in st.get("statics", [])):
                    new.add(key)  # a static that makes one, fallibly (Keyframes::try_new, PROTOCOLS.md 13.9)
            for it in items:
                if not live(it) or (it.get("state") in objs and it["state"] not in reach):
                    continue
                for x in walk_desc(it.get("ret") or ["unit"]):
                    if x[0] == "obj" and x[2] not in reach:
                        new.add(x[2])
                new.update(k for k in gk_lists.handed_out(it) if k not in reach)  # a closure's context (9.16)
                new.update(k for k in gk_editor.handed_out(it) if k not in reach)  # a plugin's inline context (9.23)
            for sh in self.shapes.values():
                for d in sh["args"]:
                    for x in gk_values.walk_tables(self, d):  # into an event's variants (PROTOCOLS.md 13.15)
                        if x[0] == "obj" and x[2] not in reach:
                            new.add(x[2])
            if not new:
                break
            reach |= new
        for key, st in objs.items():
            st["reachable"] = key in reach
            if key not in reach:
                for it in st["builders"] + st["reads"] + st["writes"] + st["calls"] + st["statics"]:
                    if live(it):
                        it["status"], it["reason"] = "todo", why
                st["builders"], st["reads"], st["writes"], st["calls"] = [], [], [], []
        dead = set(objs) - reach
        self.dead_objs = dead
        for it in items:
            if live(it) and any(x[0] == "obj" and x[2] in dead for a in it["args"] for x in walk_desc(a["desc"])):
                it["status"], it["reason"] = "todo", why
                it["detail"] = "an argument's type cannot be got"
        for st in objs.values():
            st["statics"] = [i for i in st["statics"] if live(i)]

    # ---------------------------------------------------------------- generic states, instantiated

    STATE_SPEC = re.compile(r"\s*(\w+)<\s*([\w:]+)\s*>\s+as\s+(\w+)\s*")

    def state_specs(self, crate, iid):
        """The `[instantiate] states` lines of overrides.toml that name this generic type:
        [(argument, name)], the argument an upstream type's name or a library type
        (`crate::...`, a shim of the delegate protocol)."""
        name = self.api.crates[crate].item(iid)["name"]
        out = []
        for spec in self.ov.get("instantiate", {}).get("states", []):
            m = self.STATE_SPEC.fullmatch(spec)
            if m is None:
                raise SystemExit(f"gkgen: overrides [instantiate] states: not `Generic<Arg> as Name`: {spec}")
            if m.group(1) == name:
                self.ov_used.add("instantiate.states." + spec)
                out.append((m.group(2), m.group(3)))
        return out

    def arg_type(self, arg):
        """(crate, type ref, trait names it implements or None for any) of an instantiation's argument."""
        if arg.startswith("crate::"):
            return None, {"verbatim": arg}, None
        found = [(cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items() if it["name"] == arg]
        if len(found) != 1:
            raise SystemExit(f"gkgen: overrides [instantiate] states: {arg} names {len(found)} types")
        cr, i = found[0]
        f = self.info(cr, i)
        return cr, {"resolved_path": {"path": arg, "id": int(i), "args": None}}, f["traits"] | f["blanket"]

    def impl_applies(self, crate, im, arg_ref, arg_traits):
        """Whether an inherent impl of `Base<P>` holds for the argument, and the substitution.
        `impl<M: Bound> Base<M>` holds when the argument implements every bound (a library type
        is taken to implement them); `impl Base<Concrete>` when Concrete is the argument."""
        a = path_args(im["for"].get("resolved_path", {}))
        if len(a) != 1:
            return None
        a = a[0]
        if "generic" in a:
            p = a["generic"]
            bounds = [b for q in im["generics"]["params"] if q["name"] == p and "type" in q["kind"]
                      for b in q["kind"]["type"]["bounds"]]
            for w in im["generics"].get("where_predicates", []):
                bp = w.get("bound_predicate")
                if bp and bp["type"].get("generic") == p:
                    bounds += bp["bounds"]
            if arg_traits is not None and any(n not in arg_traits for n, _ in bound_parts(bounds)):
                return None
            return {p: arg_ref[1]}  # the type; the caller pairs it with its crate
        if "resolved_path" in a and "resolved_path" in arg_ref[1]:
            if self.api.home(crate, a["resolved_path"]["id"]) == self.api.home(arg_ref[0], arg_ref[1]["resolved_path"]["id"]):
                return {}
        return None

    def alias_path(self, crate, base_name, arg, name):
        """The public path of `type Name = Base<Arg>` when upstream declares it."""
        c = self.api.crates[crate]
        for i, it in c.idx.items():
            ta = it.get("inner", {}).get("type_alias")
            if it.get("name") == name and ta and it.get("crate_id") == 0 and "resolved_path" in ta["type"]:
                rp = ta["type"]["resolved_path"]
                args = path_args(rp)
                if last(rp["path"]) == base_name and len(args) == 1 and last(rdj.ty(args[0])) == last(arg):
                    return self.api.pubpath(crate, i)
        return None

    def instantiate_states(self, row, f, c, crate, iid, specs):
        """`[instantiate] states`: one kit state type per `Base<Arg> as Name` (PROTOCOLS.md 9.6).
        Every method of an impl that holds for the argument is classified as a state method of
        that type, `Self` and the type parameter substituted. The coverage row keeps one item per
        upstream function: bridged when some instantiation binds it, else the reason of the first
        that has it."""
        base_path = self.api.pubpath(crate, iid)
        shadows = []
        for arg, name in specs:
            acr, aref, atraits = self.arg_type(arg)
            arg_rust = arg if acr is None else self.api.pubpath(acr, aref["resolved_path"]["id"])
            tpath = self.alias_path(crate, f["name"], arg, name) or f"{base_path}<{arg_rust}>"
            self_t = (crate, {"resolved_path": {"path": f["name"], "id": int(iid),
                                                "args": {"angle_bracketed": {"args": [{"type": aref}], "constraints": []}}}})
            shadow = {"name": row["name"], "path": tpath, "group": "state", "crate": crate, "items": []}
            st = {"key": tpath, "path": tpath, "name": name, "crate": crate, "ctors": [], "builders": [],
                  "reads": [], "writes": [], "calls": [], "events": [], "generic": base_path}
            self.states[tpath] = st
            seen = set()
            for im in f["impls"]:
                subst = self.impl_applies(crate, im, (acr or crate, aref), atraits)
                if subst is None:
                    continue
                ctx = Ctx(crate, self_t, {k: (acr or crate, v) for k, v in subst.items()})
                tr = im.get("trait")
                if tr is not None:
                    if last(tr["path"]) == "EventEmitter" and path_args(tr):
                        self.state_event(st, crate, ctx, tpath, path_args(tr)[0])
                    continue
                for mid in im["items"]:
                    fi = c.item(mid)
                    if not fi or "function" not in fi["inner"] or fi["visibility"] != "public" or fi["name"] in seen:
                        continue
                    seen.add(fi["name"])
                    self.state_method(shadow, st, fi, crate, ctx)
            shadows.append(shadow)
        # The row: one item per upstream function, as type_impls would list them.
        names = []
        for im in f["impls"]:
            if im.get("trait") is None:
                for mid in im["items"]:
                    fi = c.item(mid)
                    if fi and "function" in fi["inner"] and fi["visibility"] == "public" and fi["name"] not in names:
                        names.append(fi["name"])
        for fname in names:
            got = [it for sh in shadows for it in sh["items"] if it["name"] == fname]
            best = next((it for it in got if it["status"] in ("bound", "bridged")), got[0] if got else None)
            it = {"row": row["name"], "owner": row["path"], "name": fname, "trait": None, "kind": "summary",
                  "id": f"{row['path']}::{fname}", "args": [], "crate": crate, "summary": True,
                  "sig": best["sig"] if best else "", "status": best["status"] if best else "todo",
                  "reason": best["reason"] if best else "protocol: state (a state type generic over a delegate or mode: needs its bridge type)",
                  "detail": best.get("detail", "") if best else "",
                  "also": [x["id"] for x in got if x["status"] in ("bound", "bridged")]}
            row["items"].append(it)

    def state_event(self, st, crate, ctx, tpath, ev):
        try:
            self._closure += 1
            try:
                d = self.desc(Ctx(crate, ctx.self_t, ctx.subst), ev, out=True)
            finally:
                self._closure -= 1
            key = json.dumps([[strip_conv(d)], "unit"])
            self.shapes.setdefault(key, {"key": key, "args": [strip_conv(d)], "ret": "unit", "rust": [rdj.ty(ev)],
                                         "builtin": BUILTIN_SHAPES.get((json.dumps([strip_conv(d)]), "unit"))})
            st["events"].append({"name": last(rdj.ty(ev).split("<")[0]), "shape": key,
                                 "rust": self.rtype(Ctx(crate, ctx.self_t, ctx.subst), ev),
                                 "id": f"{tpath}::subscribe::{rdj.ty(ev)}", "status": "bridged"})
        except Unsup:
            gk_editor.event_reads(self, st, crate, ctx, tpath, ev)  # its reads (PROTOCOLS.md 9.35)

    def pre(self, it, f, fn):
        """Statuses that do not depend on the rules: overrides, deprecation, unsafe."""
        ov = self.ov.get("skip", {})
        short = it["id"].split("::")[-2] + "::" + it["name"] if "::" in it["id"] else it["name"]
        for key in (it["id"], short):
            if key in ov:
                self.ov_used.add(key)
                self.set(it, Unsup("skip:" + ov[key].split(":")[0].strip(), ov[key]))
                return True
        if f.get("deprecation") is not None:
            self.set(it, Unsup("skip:deprecated", "deprecated upstream"))
            return True
        if fn["header"].get("is_unsafe"):
            self.set(it, Unsup("skip:unsafe", "unsafe fn"))
            return True
        if fn["header"].get("is_async"):
            self.set(it, Unsup("todo:protocol: completions (async function)", ""))
            return True
        return False

    def row(self, name, path, group, crate):
        r = {"name": name, "path": path, "group": group, "crate": crate, "items": []}
        self.owners.append(r)
        return r

    def classify_crate(self, crate):
        c, w = self.api.crates[crate], self.api.walks[crate]
        pre = SHORT[crate]
        for iid, (path, it) in sorted(w.types.items(), key=lambda kv: kv[1][0]):
            f = self.info(crate, iid)
            tpath = self.api.pubpath(crate, iid)
            node = self.nodes.get(self.node_of.get((crate, iid)))
            shim_node = next((self.nodes[k] for o, k in self.node_of.items()
                              if len(o) == 3 and o[:2] == (crate, iid) and o[2][0] in ("shim", "inst")), None)
            if node is None and gk_values.multi_inst(self, crate, iid, f, c, f"{pre}::{it['name']}", tpath):
                continue  # one generic builder instantiated several times (PROTOCOLS.md 13.19)
            if node is None and shim_node is not None:
                # A generic element instantiated for a delegate shim or an argument list
                # ([instantiate] types): its methods are classified on that instance, `Self` and
                # the parameters substituted.
                node = shim_node
                row = self.row(f"{pre}::{it['name']}", tpath, "component" if node["element"] else "builder", crate)
                self_t = self.inst_self(crate, iid, f, node)
                f2 = dict(f)
                f2["impls"] = [im for im in f["impls"] if self.impl_matches(im, node)]
                self.type_impls(row, node, f2, c, crate, Ctx(crate, self_t, node["subst"]), "node")
                continue
            if node is None and f["generic"] and gk_values.object_specs(self, crate, iid):
                row = self.row(f"{pre}::{it['name']}", tpath, "state", crate)  # generic objects (PROTOCOLS.md 13.8)
                gk_values.instantiate_objects(self, row, f, c, crate, iid, gk_values.object_specs(self, crate, iid))
                continue
            if node is not None:
                role, group = "node", "component" if node["element"] else "builder"
            elif f["state"] or (f["kind"] == "struct" and f["refs"] and not f["record"] and not f["generic"]):
                role, group = "state", "state"
            elif f["generic"] and f["kind"] == "struct" and not f["record"]:
                role, group = "generic", "generic"
            else:
                role, group = "data", "data"
            row = self.row(f"{pre}::{it['name']}", tpath, group, crate)
            self_t = (crate, {"resolved_path": {"path": it["name"], "id": int(iid), "args": None}})
            ctx = Ctx(crate, self_t)
            if role == "state" and self.is_obj(crate, iid):
                self.classify_object(row, f, c, crate, ctx, tpath)
            elif role == "state" and f["entity"] and not f["generic"] and f["kind"] == "struct" and tpath:
                self.classify_state(row, f, c, crate, ctx, tpath)
            elif role == "state" and f["generic"] and self.state_specs(crate, iid):
                self.instantiate_states(row, f, c, crate, iid, self.state_specs(crate, iid))
            elif role == "state" and f["generic"]:
                self.type_impls(row, node, f, c, crate, ctx, "state-generic")
            else:
                self.type_impls(row, node, f, c, crate, ctx, role)
        for iid, (path, it) in sorted(w.traits.items(), key=lambda kv: kv[1][0]):
            self.classify_trait(crate, iid, it, f"{pre}::{it['name']}")
        rows = {}
        for iid, (path, f) in sorted(w.fns.items(), key=lambda kv: kv[1][0]):
            mod = path.split("::")[1] if path.count("::") > 1 else ""
            mod = mod if path.count("::") > 1 else "(root)"
            row = rows.get(mod) or rows.setdefault(mod, self.row(f"{pre}::{mod} functions", f"{FACADE[crate]}::{mod}",
                                                                 "function", crate))
            self.classify_free(row, crate, iid, path, f)

    def type_impls(self, row, node, f, c, crate, ctx, role):
        seen = set()
        for im in f["impls"]:
            tr = im.get("trait")
            if tr is None:
                for m in im["items"]:
                    fi = c.item(m)
                    if fi and "function" in fi["inner"] and fi["visibility"] == "public" and fi["name"] not in seen:
                        seen.add(fi["name"])
                        self.classify_method(row, node, fi, crate, ctx, role)
            elif node is not None:
                tn = last(tr["path"])
                tc, tid = self.api.home(crate, tr["id"])
                if tc in self.api.crates and tid is not None and not path_args(tr):
                    node["traits"].append((tc, tid, tn))
                if tn == "Default" and tc in STD and im.get("blanket_impl") is None:
                    if row["path"] + "::Default::default" in self.ov.get("skip", {}):
                        self.ov_used.add(row["path"] + "::Default::default")
                        continue
                    it = {"row": row["name"], "owner": row["path"], "name": "default", "trait": "Default",
                          "sig": "fn default() -> Self", "id": row["path"] + "::Default::default", "kind": "ctor",
                          "status": "bound", "reason": "", "args": [], "crate": crate, "node": node["key"],
                          "callee": f"<{node['path']} as Default>::default", "synthetic": True}
                    node["ctors"].append(it)

    def classify_trait(self, crate, iid, tit, rowname):
        """A trait declaration: the builder methods become one shared table."""
        c = self.api.crates[crate]
        tr = tit["inner"]["trait"]
        tpath = self.api.pubpath(crate, iid)
        row = self.row(rowname, tpath, "trait" if crate != "gpui" else "gpui", crate)
        if tit["name"] == "Styled":
            self.token_row = self.row("Styled (nullary methods, as style tokens)", tpath, "style", crate)
            row["name"] = "gpui::Styled (methods with arguments)"
        generic = any("type" in p["kind"] for p in tr["generics"]["params"])
        selfgen = generic and gk_values.self_generic_trait(self, crate, tr)  # StyleSized<T: Styled> (13.11)
        generic = generic and not selfgen
        fns = [c.item(i) for i in tr["items"]]
        fns = [f for f in fns if f and "function" in f["inner"]]
        builders = [f for f in fns if f["inner"]["function"]["sig"]["inputs"][:1]
                    and f["inner"]["function"]["sig"]["inputs"][0][0] == "self"
                    and rdj.ty(f["inner"]["function"]["sig"]["inputs"][0][1]) == "Self"]
        table = {"key": tpath, "path": tpath, "name": tit["name"], "crate": crate, "ops": [], "converts": [],
                 "extra": [], "extra_paths": [],
                 "via_style": tit["name"] in self.ov.get("traits", {}).get("via_style", [])}
        if selfgen:
            table["path"] = f"{tpath}<E>"  # bound and called as E: StyleSized<E>
            table["extra_paths"] = ["gpui_kit::Styled"]  # the trait's own bound on its parameter
        role = self.ov.get("traits", {}).get("role", {}).get(tit["name"])
        if role:
            self.ov_used.add("traits.role." + tit["name"])
        for f in sorted(fns, key=lambda f: f["name"]):
            it, fn = self.new_item(row, tpath, f, crate)
            if self.pre(it, f, fn):
                continue
            ins = fn["sig"]["inputs"]
            out = fn["sig"].get("output")
            shim = self.ov.get("shims", {}).get(f"{tit['name']}::{f['name']}")
            if shim:
                # A method a hand-written shim of the delegate protocol implements (PROTOCOLS.md 9).
                self.ov_used.add("shims." + f"{tit['name']}::{f['name']}")
                it.update(kind="shim", status="bridged", reason="", detail=shim)
            elif role:
                if not (role.endswith("(ambient context: methods on Window and App)")
                        and gk_app.ambient(self, it, crate, fn, tpath, tit["name"])):
                    self.set(it, Unsup(role, tit["name"]))
            elif tit["name"] in RUST_ONLY_TRAITS or generic or tpath is None:
                self.set(it, Unsup("skip:rust-only-extension" if not generic else
                                   "todo:generic trait (generic over an app type)", "trait " + tit["name"]))
            elif tit["name"] == "Styled" and f in builders and len(ins) == 1 and rdj.ty(out) == "Self":
                it.update(kind="token", status="bound", callee=f["name"])
                self.tokens.append(it)
                row["items"].remove(it)
                self.token_row["items"].append(it)
                it["row"] = self.token_row["name"]
            elif f in builders and out is not None and rdj.ty(out) == "Self":
                ctx = Ctx(crate)
                extra = sorted({last(b["trait_bound"]["trait"]["path"])
                                for w in fn["generics"].get("where_predicates", []) if "bound_predicate" in w
                                and w["bound_predicate"]["type"].get("generic") == "Self"
                                for b in w["bound_predicate"]["bounds"] if "trait_bound" in b} - {"Sized"})
                if self.bindable(it, ctx, fn, ins[1:], "trait-op", None, f"<E as {tpath}>::{f['name']}"):
                    if extra:
                        try:
                            paths = [self.trait_path(n) for n in extra]
                        except Unsup as e:
                            self.set(it, e)
                            continue
                        key = (crate, iid, tuple(extra))
                        sub = self.traits.get(key) or self.traits.setdefault(key, dict(
                            table, ops=[], converts=[], key=tpath + " where Self: " + " + ".join(extra),
                            extra=extra, extra_paths=paths))
                        sub["ops"].append(it)
                    else:
                        table["ops"].append(it)
                        if table["via_style"] and len(ins) == 1 and tit["name"] != "Styled":
                            # A nullary method of a trait that only writes the style (the kit's
                            # StyledExt: font_bold, h_flex, ...) is also a style token, as gpui's
                            # Styled ones are (PROTOCOLS.md 11.36): `.Font_Bold` in a style list.
                            # The method stays; the token is a second entry, not a second function,
                            # made in gk_rust.number once quarantine has had its say.
                            self.style_ext.append((it, tpath))
            elif f in builders and out is not None:
                it["status"], it["reason"] = "todo", "type-changing method (a wrapper instantiated per element type)"
                it["fn"], it["fcrate"] = fn, crate
                table["converts"].append(it)
            elif not builders:
                self.set(it, Unsup("todo:protocol: delegates or calls (a trait without builder methods)", tit["name"]))
            elif ins and ins[0][0] == "self":
                recv = rdj.ty(ins[0][1])
                self.set(it, Unsup({"&Self": "skip:builder-getter", "&mut Self": "skip:rust-only-extension"}.get(
                    recv, "skip:rust-only-extension"), "the required half of a builder trait"))
            else:
                self.set(it, Unsup("todo:protocol: calls (a static function that is not a constructor)", ""))
        self.traits[(crate, iid)] = table

    def trait_path(self, name):
        for cr in CRATES:
            for tid, (_, t) in self.api.walks[cr].traits.items():
                if t["name"] == name:
                    return self.api.pubpath(cr, tid)
        raise Unsup("todo:generic type parameter without a closed set", "where Self: " + name)

    def classify_free(self, row, crate, iid, path, f):
        it, fn = self.new_item(row, row["path"], f, crate)
        it["id"] = "::".join([FACADE[crate]] + path.split("::")[1:])
        if self.pre(it, f, fn):
            return
        ctx = self.fn_ctx(Ctx(crate), fn)
        rn = self.ret_node(ctx, fn["sig"].get("output"))
        out = fn["sig"].get("output")
        if not fn["sig"]["inputs"] and out and last(rdj.ty(out)) == "Hsla":
            it.update(kind="palette", status="bound", callee=it["id"])
            self.palette.append(it)
            return
        if (rn is None or rn == "self") and gk_app.ambient(self, it, crate, fn, None):
            return
        if rn is None or rn == "self":
            return self.set(it, Unsup("todo:protocol: calls (a free function that is not a constructor)",
                                      rdj.ty(out) if out else "()"))
        if self.bindable(it, Ctx(crate), fn, fn["sig"]["inputs"], "ctor", None, it["id"]):
            it["node"], it["free"] = rn, True
            self.nodes[rn]["ctors"].append(it)

    def classify_gpui(self):
        """The parts of gpui the kit's elements are made of (overrides [gpui])."""
        inc = self.ov.get("gpui", {})
        c, w = self.api.crates["gpui"], self.api.walks["gpui"]
        for key, node in list(self.nodes.items()):
            if node["crate"] != "gpui":
                continue
            f = self.info("gpui", node["id"])
            row = self.row(f"gpui::{node['name']}", node["path"], "gpui", "gpui")
            base = (f["crate"], {"resolved_path": {"path": f["name"], "id": int(node["id"]), "args": None}})
            if node["inst"]:
                arg = node["subst"][f["generic"][0]][1]
                base = ("gpui", {"resolved_path": {"path": f["name"], "id": int(node["id"]), "args": {
                    "angle_bracketed": {"args": [{"type": arg}], "constraints": []}}}})
            ctx = Ctx("gpui", base, node["subst"])
            f2 = dict(f)
            if node["inst"]:
                f2["impls"] = [im for im in f["impls"] if self.impl_matches(im, node)]
            self.type_impls(row, node, f2, c, "gpui", ctx, "node")
        for iid, (path, it) in sorted(w.traits.items(), key=lambda kv: kv[1][0]):
            if it["name"] in inc.get("traits", []):
                self.classify_trait("gpui", iid, it, f"gpui::{it['name']}")
        row = self.row("gpui functions", "gpui_kit", "gpui", "gpui")
        for iid, (path, f) in sorted(w.fns.items(), key=lambda kv: kv[1][0]):
            if f["name"] in inc.get("functions", []):
                self.classify_free(row, "gpui", iid, path, f)

    def theme_tokens(self):
        """The Hsla fields of the kit's ThemeColor: colours the library resolves at replay."""
        c = self.api.crates["gpui_component"]
        for _, (path, it) in self.api.walks["gpui_component"].types.items():
            if it["name"] == "ThemeColor" and "struct" in it["inner"]:
                for fid in it["inner"]["struct"]["kind"]["plain"]["fields"]:
                    fi = c.item(fid)
                    if fi and last(rdj.ty(fi["inner"]["struct_field"])) == "Hsla":
                        self.theme.append(fi["name"])

    def impl_matches(self, im, node):
        args = path_args(im["for"]["resolved_path"]) if "resolved_path" in im["for"] else []
        if node["inst"][0] == "inst":
            # An impl for some concrete arguments holds when each is the node's own.
            f = self.info(node["crate"], node["id"])
            for a, g in zip(args, f["generic"]):
                mine = node["subst"][g][1]
                if "generic" in a:
                    continue
                if "verbatim" in mine or last(rdj.ty(a).split("<")[0]) != last(rdj.ty(mine).split("<")[0]):
                    return False
            return True
        if not args or "generic" in args[0]:
            return True
        if node["inst"][0] == "shim":
            return False
        return "resolved_path" in args[0] and last(args[0]["resolved_path"]["path"]) == \
            self.api.crates[node["inst"][0]].item(node["inst"][1])["name"]

    # ---------------------------------------------------------------- after classification

    def settle(self):
        """Trait tables per node, type-changing trait methods, names and the fixpoint over
        types nothing can construct."""
        for node in self.nodes.values():
            tabs, seen = [], set()
            f = self.info(node["crate"], node["id"])
            has = f["traits"] | f["blanket"]
            for tc, tid, tn in node["traits"]:
                t = self.traits.get((tc, tid))
                if t is None or (tc, tid) in seen:
                    continue
                seen.add((tc, tid))
                if t["ops"] or tn == "Styled":
                    tabs.append(t["key"])
                for key, sub in self.traits.items():
                    if len(key) == 3 and key[:2] == (tc, tid) and set(sub["extra"]) <= has:
                        tabs.append(sub["key"])
                for cv in t["converts"]:
                    self.trait_convert(node, t, cv)
            node["tables"] = tabs
        for st in self.states.values():
            if not st["ctors"] and not st.get("obj") and not st.get("handed"):  # handed: PROTOCOLS.md 11.8
                for it in st["builders"] + st["reads"] + st["writes"] + st["calls"]:
                    it["status"], it["reason"] = "todo", "protocol: state (no constructor of the state type is bound)"
                st["builders"], st["reads"], st["writes"], st["calls"], st["events"] = [], [], [], [], []
        gk_overlay.post(self)  # the views family's statuses decided after the rules (PROTOCOLS.md 11.14 to 11.16)
        self.dead_objs = set()
        self.prune()
        gk_editor.project(self)  # reads through a borrow (PROTOCOLS.md 9.27)
        gk_editor.before_reach(self)  # objects a read-back value hands out (PROTOCOLS.md 9.25)
        self.reach_objects()
        if self.dead_objs:
            self.prune()  # records, variants and shapes made of an object nothing hands out
        self.flow()
        gk_editor.revive(self)  # objects a read-back value hands out (PROTOCOLS.md 9.25)
        gk_values.post(self)  # speech skipped, generic traits bound through instances (PROTOCOLS.md 13.8)
        gk_editor.after_values(self)  # the editor host's items found usable late (PROTOCOLS.md 9.30)
        for key in self.ov.get("skip", {}):
            if key not in self.ov_used:
                raise SystemExit(f"gkgen: overrides [skip] key matches no function: {key}")
        for key in self.ov.get("shims", {}):
            if "shims." + key not in self.ov_used:
                raise SystemExit(f"gkgen: overrides [shims] names no trait method: {key}")
        for spec in self.ov.get("instantiate", {}).get("states", []):
            if "instantiate.states." + spec not in self.ov_used:
                raise SystemExit(f"gkgen: overrides [instantiate] states names no generic state type: {spec}")
        gk_lists.check_used(self)
        gk_editor.check_used(self)
        for key in self.ov.get("rows", {}):
            if "rows." + key not in self.ov_used:
                raise SystemExit(f"gkgen: overrides [rows] names no closure: {key}")
        for key in self.ov.get("traits", {}).get("role", {}):
            if "traits.role." + key not in self.ov_used:
                raise SystemExit(f"gkgen: overrides [traits.role] names no trait: {key}")
        # Records a parameter may also take as one integer field alone (PROTOCOLS.md 11.36): the
        # record's `shorthand` names the field; the description carries it to every emitter.
        for rt, field in self.ov.get("shorthand", {}).items():
            rec = self.records.get(rt)
            f = next((f for f in rec["fields"] if f["name"] == field), None) if rec and rec.get("in") else None
            if f is None or f["desc"][0] != "int":
                raise SystemExit(f"gkgen: overrides [shorthand] names no record an app writes with the integer field: {rt}.{field}")
            rec["shorthand"] = field

    def all_items(self):
        seen, out = set(), []
        for it in self.items + [i for n in self.nodes.values() for i in n["ctors"] + n.get("converts", [])]:
            if id(it) not in seen:
                seen.add(id(it))
                out.append(it)
        return out

    def prune(self, dead_types=()):
        """The fixpoint over what cannot be built: a node type no bound constructor reaches
        cannot be an argument, and neither can a record, variant or Into member made of one."""
        opaque = "opaque library object without a constructor the binding can drive"
        dead_t = set(dead_types)
        items = self.all_items()
        while True:
            dead = {k for k, n in self.nodes.items() if not any(i["status"] in ("bound", "bridged") for i in n["ctors"])}
            bad = lambda d: any((x[0] in ("node", "build", "stylefn") and x[1] in dead) or  # noqa: E731
                                (x[0] == "obj" and x[2] in self.dead_objs) or
                                (x[0] in ("enum", "denum", "rec", "into") and (x[0], x[1]) in dead_t) or
                                (x[0] == "handler" and ("shape", x[1]) in dead_t) for x in walk_desc(d))
            changed = False
            for rt, r in self.records.items():
                if ("rec", rt) not in dead_t and any(bad(f["desc"]) for f in r["fields"]):
                    dead_t.add(("rec", rt))
                    changed = True
            for rt, e in self.denums.items():
                for v in e["variants"]:
                    if v["ok"] and any(bad(f["desc"]) for f in v["fields"]):
                        v["ok"], changed = False, True
                if ("denum", rt) not in dead_t and not any(v["ok"] for v in e["variants"]):
                    dead_t.add(("denum", rt))
                    changed = True
            for rt, s in self.intos.items():
                keep = [x for x in s["members"] if not bad(x["desc"])]
                if len(keep) != len(s["members"]):
                    s["members"], changed = keep, True
                if not keep and ("into", rt) not in dead_t:
                    dead_t.add(("into", rt))
                    changed = True
            for key, sh in self.shapes.items():
                if ("shape", key) not in dead_t and any(bad(d) for d in sh["args"]):
                    dead_t.add(("shape", key))
                    changed = True
            for it in items:
                if it["status"] in ("bound", "bridged") and (any(bad(a["desc"]) for a in it["args"]) or it.get("from") in dead
                                                             or ("ret" in it and bad(it["ret"]))):
                    it["status"], it["reason"] = "todo", opaque
                    it["detail"] = "an argument's type cannot be built"
                    changed = True
            if not changed:
                break
        for kind, rt in dead_t:
            {"enum": self.enums, "denum": self.denums, "rec": self.records, "into": self.intos,
             "shape": self.shapes}[kind].pop(rt, None)
        self.dead = dead
        for st in self.states.values():
            st["events"] = [e for e in st["events"] if e["shape"] in self.shapes]

    def flow(self):
        """in and out reach the types a type is made of."""
        tables = {"enum": self.enums, "denum": self.denums, "rec": self.records}
        for s in self.intos.values():
            for mem in s["members"]:
                for x in walk_desc(mem["desc"]):
                    u = tables.get(x[0], {}).get(x[1]) if len(x) > 1 and isinstance(x[1], str) else None
                    if u is not None:
                        u["in"] = True
        changed = True
        while changed:
            changed = False
            for kind in ("denum", "rec"):
                for t in tables[kind].values():
                    descs = [f["desc"] for f in t.get("fields", [])] + [f["desc"] for v in t.get("variants", []) for f in v["fields"]]
                    for d in descs:
                        for x in walk_desc(d):
                            u = tables.get(x[0], {}).get(x[1]) if len(x) > 1 and isinstance(x[1], str) else None
                            if u is None:
                                continue
                            for flag in ("in", "out"):
                                if t[flag] and not u[flag]:
                                    u[flag], changed = True, True

    def trait_convert(self, node, table, cv):
        """`InteractiveElement::id(self, ..) -> Stateful<Self>` for one concrete Self."""
        fn = cv["fn"]
        self_t = self.node_self(node)
        ctx = self.fn_ctx(Ctx(cv["fcrate"], self_t, node["subst"]), fn)
        out = fn["sig"]["output"]
        try:
            d = self.desc(ctx, out)
        except Unsup:
            return
        if d[0] != "node":
            return
        it = dict(cv)
        it.pop("fn"), it.pop("fcrate")
        it.update(owner=node["path"], id=f"{node['path']}::{table['name']}::{cv['name']}", synthetic=True,
                  row=None, trait=table["name"])
        if self.bindable(it, Ctx(cv["fcrate"], self_t, node["subst"]), fn, fn["sig"]["inputs"][1:], "convert", node,
                         f"<{node['path']} as {table['path']}>::{cv['name']}"):
            it["node"], it["from"] = d[1], node["key"]
            self.nodes[d[1]]["ctors"].append(it)
            node.setdefault("converts", []).append(it)
            cv["status"], cv["reason"] = "bound", ""
            cv["instances"] = cv.get("instances", 0) + 1

    def node_self(self, node):
        f = self.info(node["crate"], node["id"])
        t = {"resolved_path": {"path": f["name"], "id": int(node["id"]), "args": None}}
        if node["inst"] and node["inst"][0] == "inst":
            return self.inst_self(node["crate"], node["id"], f, node)
        if node["inst"]:
            t["resolved_path"]["args"] = {"angle_bracketed": {"args": [{"type": node["subst"][f["generic"][0]][1]}],
                                                              "constraints": []}}
        return node["crate"], t


def strip_conv(d):
    """A descriptor without its Rust conversion tags: what the wire and the app see."""
    if d[0] == "str":
        return ["str"]
    if d[0] == "leaf":
        return ["leaf", d[1]]
    if d[0] in ("opt", "ref"):
        return [d[0], strip_conv(d[1])] if d[0] == "opt" else strip_conv(d[1])
    if d[0] == "list":
        return ["list", strip_conv(d[1])]
    if d[0] == "from":
        return strip_conv(d[1])
    if d[0] == "tuple":
        return ["tuple", [strip_conv(x) for x in d[1]]]
    if d[0] == "int":
        return ["int", INT_WIRE[d[1]]]
    return d


def walk_desc(d):
    yield d
    if d[0] in ("opt", "ref", "list", "from", "refmut", "fallible", "array"):
        yield from walk_desc(d[1])
    elif d[0] == "tuple":
        for x in d[1]:
            yield from walk_desc(x)


def uses_protocol(d):
    return any(x[0] in ("handler", "fragment", "build", "entity", "handle", "action", "stylefn", "shim", "rows",
                        "data", "field", "format", "datum_rows", "obj", "refmut", "viewfn", "expr", "ask", "dock",
                        "ctxrows", "lent", "items") or x[0] in gk_app.PROTOCOL_KINDS  # the platform family's (13.31)
               for x in walk_desc(d))


def names_node(d, keys):
    return any(x[0] in ("node", "build") and x[1] in keys for x in walk_desc(d))
