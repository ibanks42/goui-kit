"""Semantic API model built from extracted rustdoc JSON and api/overrides.toml.

Pipeline role:
  Reads the four pinned crate rustdoc archives (`gpui`, `gpui_base`,
  `gpui_component`, `gpui_kit`), applies `api/overrides.toml` and
  `api/clone.json`, and builds an intermediate model consumed by:
  - `gk_names.py` (canonical stem naming)
  - `gk_rust.py` and `gk_fam.py` (opcode assignment and Rust decoder emission)
  - `gk_go.py` and `gk_split.py` (Go package emission and splitting)
  - `gkgen.py` (coverage, API description JSON, and reports)

Core concepts in the model:
  - **Nodes** (`self.nodes`): element types (`element=True`, implement `IntoElement`)
    and builder-style value structs (`element=False`) encoded on the view tape.
  - **States** (`self.states`): GPUI `Entity<T>` types kept behind opaque u64
    handles in the runtime, driven through call ops.
  - **Objects** (`self.objects`): non-entity Rust structs (and `Global` singletons)
    also stored behind u64 handles in `self.states` with `obj=True`.
  - **Descriptors**: tagged Python lists describing how a Rust parameter or return
    type crosses the wire (for example `["int", "usize"]`, `["leaf", "color", ""]`,
    `["rec", rt]`, `["denum", rt]`, `["into", rt]`, `["handler", key, ...]`).
  - **Extension modules**: `gk_app`, `gk_values`, `gk_overlay`, `gk_lists`,
    `gk_editor`, and `gk_items` hook into type scanning, descriptor resolution,
    closure matching, and post-classification settlement.
"""
import json
import os
import re
import tomllib

import gk_app
import gk_lists
import gk_values
import gk_overlay
import gk_editor
import gk_items
import rdj

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
API = os.path.join(REPO, "api")
# Maps internal crate names to the public gpui_kit facade module prefix used in generated Rust.
FACADE = {"gpui_component": "gpui_kit::component", "gpui_base": "gpui_kit::base", "gpui": "gpui_kit",
          "gpui_kit": "gpui_kit"}
STD = {"core", "std", "alloc"}
KIT = ("gpui_component", "gpui_base")
CRATES = ("gpui", "gpui_base", "gpui_component", "gpui_kit")
SHORT = {"gpui_component": "component", "gpui_base": "base", "gpui": "gpui", "gpui_kit": "kit"}

# Allowed skip reason prefixes enforced by Model.set() so every skipped item has a tracked category.
SKIP_REASONS = {
    "platform-trait", "test-only", "third-party-type", "hidden", "feature-off", "rust-only-extension",
    "item-render-closure", "builder-getter", "builder-mutator", "value-helper", "deprecated", "unsafe",
}


class Unsup(Exception):
    """Raised during signature classification when a type or method cannot be bound directly.

    `cat` uses prefixes:
      - `skip:<reason>`: intentionally omitted (must be in SKIP_REASONS).
      - `todo:<reason>`: unsupported pattern tracked in coverage reports.
      - `norule`: unexpected signature; fails `make gen` unless `--allow-no-rule` is passed.
    """

    def __init__(self, cat, detail=""):
        super().__init__(f"{cat} {detail}")
        self.cat, self.detail = cat, detail


def last(path):
    """Return the final `::`-separated segment of a Rust path."""
    return path.split("::")[-1]


def snake(name):
    """Convert CamelCase or acronym-bearing identifiers into lowercase snake_case."""
    s = re.sub(r"(?<=[a-z0-9])([A-Z])", r"_\1", name)
    s = re.sub(r"(?<=[A-Z])([A-Z][a-z])", r"_\1", s)
    return s.lower()


def ada(name):
    """Convert an identifier into Ada_Case (capitalized snake_case segments)."""
    return "_".join(p[:1].upper() + p[1:] for p in snake(name).split("_") if p) or "X"


class Api:
    """Multi-crate rustdoc index resolving cross-crate item IDs to home crates and public facade paths.

    Each crate in CRATES is loaded via `rdj.Crate` and walked with `rdj.Walk`.
    Because rustdoc JSON references items from dependency crates by crate-local
    IDs inside `paths`, `home()` maps any `(crate, id)` pair to the defining
    crate and its canonical item ID, and `pubpath()` maps that to the shortest
    re-exported `gpui_kit::...` path.
    """

    def __init__(self, json_dir):
        self.crates, self.walks, self.by_def = {}, {}, {}
        for name in CRATES:
            p = os.path.join(json_dir, name + ".json")
            c = rdj.Crate(p if os.path.exists(p) else p + ".gz", name)
            # Pin rustdoc JSON schema version so format shifts fail loudly instead of misparsing.
            if c.format != 61:
                raise SystemExit(f"gkgen: rustdoc JSON format {c.format} of {name} is not the pinned 61")
            self.crates[name] = c
            self.walks[name] = rdj.Walk(c)
            self.by_def[name] = {"::".join(p["path"]): i for i, p in c.paths.items() if p["crate_id"] == 0}
        self._pub = {}

    def home(self, crate, iid):
        """Resolve `(crate, iid)` to `(home_crate, home_id_or_path)`.

        Normalizes `gpui_fast` external references to `gpui` so types resolve into
        the loaded `gpui.json.gz` index.
        """
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
        """Return `(home_crate, home_id, rustdoc_item)` or `(ext_crate, path, None)` for external items."""
        hc, hid = self.home(crate, iid)
        if hc in self.crates and hid is not None:
            return hc, hid, self.crates[hc].item(hid)
        return hc, hid, None

    def pubpath(self, crate, iid):
        """Return the public `gpui_kit::...` (or `std::...`) path for `(crate, iid)`, or None if hidden."""
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
    """Type-resolution context holding the active crate, `Self` type, generic substitutions, and trait bounds."""

    def __init__(self, crate, self_t=None, subst=None, bounds=None):
        self.crate, self.self_t, self.subst, self.bounds = crate, self_t, subst or {}, bounds or {}

    def at(self, crate):
        """Return a context sharing bindings/substitutions but resolving local IDs in `crate`."""
        return self if crate == self.crate else Ctx(crate, self.self_t, self.subst, self.bounds)


def path_args(rp):
    """Extract angle-bracketed type arguments (ignoring lifetimes/consts) from a rustdoc resolved_path."""
    return [a["type"] for a in ((rp.get("args") or {}).get("angle_bracketed") or {}).get("args", [])
            if isinstance(a, dict) and "type" in a]


def bound_parts(bounds):
    """Extract `(trait_short_name, trait_dict)` pairs from a rustdoc generic bounds list."""
    return [(last(b["trait_bound"]["trait"]["path"]), b["trait_bound"]["trait"]) for b in bounds
            if "trait_bound" in b]


# Wire width normalization for Rust integer primitives (usize/isize travel as fixed 64-bit values).
INT_WIRE = {"u8": "u8", "u16": "u16", "u32": "u32", "usize": "u64", "u64": "u64",
            "i8": "i8", "i16": "i16", "i32": "i32", "isize": "i64", "i64": "i64"}
# GPUI event structs serialized by hand-written runtime codecs in core/src/genapi.rs.
EVENT_LEAF = {"ClickEvent": "click_event", "KeyDownEvent": "key_event", "KeyUpEvent": "key_event",
              "MouseDownEvent": "mouse_event", "MouseUpEvent": "mouse_event", "MouseMoveEvent": "mouse_event",
              "DragMoveEvent": "drag_move_event"}
# Fixed handler shape numbers reserved by the hand-written runtime (shapes 1..10).
BUILTIN_SHAPES = {('[["leaf", "click_event"]]', "unit"): 1, ("[]", "bool"): 2, ("[]", "unit"): 3,
                  ('[["bool"]]', "unit"): 4, ('[["int", "u64"]]', "unit"): 5, ('[["str"]]', "unit"): 6,
                  ('[["leaf", "key_event"]]', "unit"): 7, ('[["leaf", "click_event"]]', "bool"): 8,
                  ('[["leaf", "mouse_event"]]', "unit"): 9, ('[["f64"]]', "unit"): 10}
# Primitive GPUI value types with dedicated wire codecs. Third element names an optional From<Hsla> target.
LEAF = {"Pixels": ["leaf", "px", ""], "Rems": ["leaf", "rems", ""], "Hsla": ["leaf", "color", ""],
        "Rgba": ["leaf", "color", "gpui_kit::Rgba"], "Fill": ["leaf", "color", "gpui_kit::Fill"],
        "Background": ["leaf", "color", "gpui_kit::Background"], "Length": ["leaf", "length", ""],
        "DefiniteLength": ["leaf", "definite_length", ""], "AbsoluteLength": ["leaf", "absolute_length", ""],
        "ElementId": ["leaf", "id", ""], "SharedString": ["str", "SharedString"],
        "AnyElement": ["child"]}


def is_fill(d):
    """Return True when descriptor `d` is the GPUI Fill color/background leaf."""
    return d[:2] == ["leaf", "color"] and len(d) > 2 and d[2] == "gpui_kit::Fill"


# Crates treated as core GPUI homes when recognizing built-in leaf and string types.
GPUI_HOMES = {"gpui", "gpui_shared_string", "gpui_util", "gpui_collections"}
HANDLES = {"FocusHandle"}
LATER_HANDLES = {"ScrollHandle", "UniformListScrollHandle", "ListState", "Subscription", "Task"}
CONTEXTS = {"Window": "window", "App": "app"}
# Traits whose methods are Rust plumbing rather than bindable component builders.
RUST_ONLY_TRAITS = {"RenderOnce", "IntoElement", "Element", "Render", "Focusable", "EventEmitter", "Global",
                    "Action", "FluentBuilder", "View", "TestTextExt", "From", "Default", "IntoIterator",
                    "Deref", "Clone", "Copy"}


class Model:
    """Complete classified representation of the gpui-kit API ready for Rust and Go code generation.

    Construction executes the full model pipeline in order:
      1. `scan_types()` + `gk_overlay.after_scan()` + `gk_values.after_scan()`
         discover all concrete and instantiated node structs.
      2. `classify_crate()` (for `gpui_component`, `gpui_base`) and `classify_gpui()`
         classify every struct, trait, and free function into items and tables.
      3. `theme_tokens()` extracts `ThemeColor` `Hsla` field names.
      4. `gk_overlay.after_classify()` and `gk_values.after_classify()` attach
         blanket traits, handed states, and runtime shims.
      5. `settle()` resolves trait tables, conversions, object reachability,
         pruning, directional flow, and unused-override checks.
    """

    def __init__(self, json_dir, overrides_path):
        self.api = Api(json_dir)
        self.ov = tomllib.load(open(overrides_path, "rb"))
        self.ov_used = set()
        cpath = os.path.join(os.path.dirname(overrides_path), "clone.json")
        self.clones = {k: set(v) for k, v in json.load(open(cpath))["clone"].items()} if os.path.exists(cpath) else {}
        self.nodes = {}
        self.enums = {}
        self.denums = {}
        self.records = {}
        self.intos = {}
        self.shapes = {}
        self.traits = {}
        self.items = []
        self.owners = []
        self.palette = []
        self.tokens = []
        self.style_ext = []
        self.theme = []
        self.states = {}
        self.tinfo = {}
        self._busy = set()
        self._closure = 0
        self.items_handed = set()
        self.objects = {}
        self._prefer_rec = False
        self.scan_types()
        gk_overlay.after_scan(self)
        gk_values.after_scan(self)
        for crate in KIT:
            self.classify_crate(crate)
        self.classify_gpui()
        self.theme_tokens()
        gk_overlay.after_classify(self)
        gk_values.after_classify(self)
        self.settle()


    def info(self, hc, hid):
        """Inspect and cache structural traits, impls, generic params, and role hints for a type.

        Decision rules computed here govern how `scan_types()` and `classify_crate()` route
        each struct/enum:
        - `state` / `entity`: True when any inherent method borrows `Context<Self>`, or the
          type implements `Render`, `EventEmitter`, or `Global` (or is listed in `[state].types`).
          Types listed in `[state].objects` override this to plain library objects (`state=False`).
        - `element`: True when `IntoElement` is implemented directly (or via blanket impl over
          an `Element` impl).
        - `clone`: True when `Clone` is in direct traits OR recorded in `api/clone.json`
          (needed because rustdoc JSON drops std trait impls during `strip()`).
        - `record`: True for non-`#[non_exhaustive]` plain/tuple structs with at least one
          field and zero private/stripped fields.
        """
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
            f["state"] = f["entity"] = False
        f["element"] = "IntoElement" in f["traits"] or "IntoElement" in f["blanket"] and "Element" in f["traits"]
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
        """Discover all element/value node structs across kit crates, gpui allowlist, and `[instantiate].types`.

        A struct becomes a tape node if it is explicitly listed in `[gpui].types` or
        `[node].types`, OR if it is a concrete, non-state struct that is either an element
        or a builder-style value (has a constructor, `Default`, or is returned by a public
        function, and is not a pure getter-only object or plain builderless record).
        After scanning concrete structs, expands every generic monomorphization in
        `[instantiate].types`.
        """
        self.node_of = {}
        self.returned = set()
        for crate in KIT:
            c, w = self.api.crates[crate], self.api.walks[crate]
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
                        continue
                self.add_node(crate, iid, f, self.api.pubpath(crate, iid), it["name"], {})
        for spec in self.ov.get("instantiate", {}).get("types", []):
            if self.instantiate_many(spec):
                continue
            m = re.fullmatch(r"(\w+)<([\w:]+)>(?:\s+as\s+(\w+))?", spec)
            base = next(((cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items()
                         if it["name"] == m.group(1)), None)
            if m.group(2).startswith("crate::"):
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


    PRIMS = {"bool", "f32", "f64", "u8", "u16", "u32", "u64", "usize", "i8", "i16", "i32", "i64", "isize"}

    def data_types(self):
        return set(self.ov.get("instantiate", {}).get("data", []))

    def inst_arg(self, crate, a):
        if a.startswith("crate::"):
            return crate, {"verbatim": a}, a
        if a in self.PRIMS:
            return crate, {"primitive": a}, a
        if a == "()":
            return crate, {"tuple": []}, a
        found = [(cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items()
                 if it["name"] == a and not self.info(cr, i)["generic"]]
        if len(found) == 1:
            cr, i = found[0]
            return cr, {"resolved_path": {"path": a, "id": int(i), "args": None}}, self.api.pubpath(cr, i)
        if a in LEAF:
            for cr in KIT:
                for pid, p in self.api.crates[cr].paths.items():
                    if p["path"][-1] == a and p["kind"] == "struct" and self.api.home(cr, pid)[0] in GPUI_HOMES:
                        return cr, {"resolved_path": {"path": a, "id": int(pid), "args": None}}, "gpui_kit::" + a
        raise SystemExit(f"gkgen: overrides [instantiate] types: {a} names {len(found)} types")

    def instantiate_many(self, spec):
        m = re.fullmatch(r"(\w+)<(.+)>(?:\s+as\s+(\w+))?", spec)
        if m is None:
            raise SystemExit(f"gkgen: overrides [instantiate] types: not `Generic<Args> [as Name]`: {spec}")
        args = gk_values.split_args(m.group(2))
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


    def rtype(self, ctx, t):
        """Render a rustdoc type node into a fully qualified Rust path usable in generated code.

        Substitutes `Self` and instantiated generic parameters via `ctx`, and resolves
        facade paths through `Api.pubpath()`. Raises `Unsup` for hidden, third-party,
        or unbounded generic types.
        """
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
                p = "lsp_types::Position"
            p = p or gk_lists.third_party_path(hc, last(rp["path"]))
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
            r = gk_lists.qualified(self, ctx, t)
            if r:
                return self.rtype(ctx.at(r[0]), r[1])
        raise Unsup("norule", f"type {rdj.ty(t)}")


    def desc(self, ctx, t, out=False):
        """Convert a rustdoc type node into a tagged wire descriptor list.

        Set out=True when classifying return values or event payloads traveling
        from Rust back to Go.
        """
        if "verbatim" in t:
            lv = gk_lists.libval(self, t["verbatim"], out)
            if lv:
                return lv
            if t["verbatim"] in self.data_types():
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
                return ["char"]
            raise Unsup("norule", f"primitive {p}")
        if "borrowed_ref" in t:
            return self.borrowed(ctx, t["borrowed_ref"], out)
        if "tuple" in t:
            if not t["tuple"]:
                return ["unit"]
            return ["tuple", [self.desc(ctx, x, out) for x in t["tuple"]], self.rtype(ctx, t)]
        if "array" in t and gk_values.array(self, ctx, t, out):
            return gk_values.array(self, ctx, t, out)
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
            r = gk_lists.qualified(self, ctx, t)
            if r:
                return self.desc(ctx.at(r[0]), r[1], out)
            raise Unsup("todo:associated type (generic over an app type)", rdj.ty(t))
        if "function_pointer" in t:
            raise Unsup("todo:function pointer (data protocol)", rdj.ty(t))
        if "raw_pointer" in t:
            raise Unsup("skip:unsafe", rdj.ty(t))
        raise Unsup("norule", f"type {rdj.ty(t)}")

    def borrowed(self, ctx, b, out):
        """Classify a `&T` or `&mut T` reference type.

        Recognizes `&mut Window`/`&mut App` context borrows, `&str`, `&[T]` slices,
        `&mut Obj` mutable object borrows, and shared references over wire-decodable types.
        """
        inner, mut = b["type"], b["is_mutable"]
        if "qualified_path" in inner:
            r = gk_lists.qualified(self, ctx, inner)
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
        """Classify an `impl Trait` or bounded generic parameter from its trait bounds.

        Checks extension hooks first (MarkdownPlugin, element unions, output iterators),
        then closure traits (`Fn`/`FnMut`/`FnOnce`), `Into<T>`, `AsRef<str>`,
        `IntoElement`, `IntoIterator`, and `Hash` key bounds.
        """
        eb = gk_editor.bounded(self, ctx, bounds, out)
        if eb is not None:
            return eb
        bi = gk_lists.bounded_out(self, ctx, bounds, out)
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
                return ["str", "SharedString"]
            if n in ("Render", "Focusable", "EventEmitter", "Panel", "PanelView"):
                raise Unsup("todo:protocol: views (a generic over the app's view type)", n)
        raise Unsup("todo:generic type parameter without a closed set", " + ".join(names) or "unbounded")

    def into(self, ctx, target):
        """Resolve an `impl Into<Target>` parameter into a direct descriptor or tagged Into set.

        Collects `Target` itself plus every bindable `impl From<Src> for Target`
        in the target's home crate. If only one unique wire representation remains,
        returns that member directly (wrapped in `["from", ...]` when conversion is
        needed); otherwise registers a multi-member set in `self.intos`.
        """
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
                d[2] = ""
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
            if d0[0] == "obj" and not d0[3]:
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
                    members += gk_values.from_implementors(self, hc, im)
                    continue
                src = path_args(im["trait"])
                if len(src) == 1 and gk_lists.from_str_ref(src[0]):
                    members.append((["str", "&str"], "&str"))
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
            uniq = uniq[1:]
        single = uniq[0][0] if uniq[0][1] is None else ["from", uniq[0][0], rt, uniq[0][1]]
        self.intos[rt] = {"target": rt, "name": name, "single": single,
                          "members": [{"desc": d, "from": src} for d, src in uniq]}
        return ["into", rt] if len(uniq) > 1 else single

    def stand_in(self, name):
        path = self.ov.get("into", {}).get("stand_in", {}).get(name or "")
        if path is None:
            return None
        for cr, w in self.api.walks.items():
            for tid, (_, t) in w.types.items():
                if self.api.pubpath(cr, tid) == path:
                    return cr, tid, t["name"]
        raise SystemExit(f"gk_model: [into] stand_in {name} = {path}: no public type of that path")

    def resolved(self, ctx, t, out):
        """Classify a concrete `resolved_path` type.

        Tries extension modules (`gk_values`, `gk_editor`, `gk_lists`, `gk_overlay`),
        then standard wrappers (`Option`, `Vec`, `Box`/`Rc`/`Arc`, `Entity`, `Cow`,
        `String`, `Duration`, `Range`, `PathBuf`, `Result`), GPUI leaves and handles,
        and finally local crate nodes, enums, objects, and plain records.
        """
        rp = t["resolved_path"]
        name, args = last(rp["path"]), path_args(rp)
        vd = gk_values.resolved(self, ctx, t, out)
        if vd is not None:
            return vd
        ea = gk_editor.alias(self, ctx, t, out)
        if ea is not None:
            return ea
        ld = gk_lists.resolved(self, ctx, t, out)
        if ld is not None:
            return ld
        od = gk_overlay.resolved(self, ctx, t, out)
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
                            return ["action", "box"]
                        if out:
                            raise Unsup("todo:event payload the binding cannot encode yet", "Box<dyn Action>")
                        return ["action", "box"]
            if a.get("primitive") == "str":
                return ["str", "into"]
            pv = gk_overlay.panel_view(self, ctx, name, a, out)
            if pv:
                return pv
            raise Unsup("todo:opaque library object without a constructor the binding can drive", rdj.ty(t))
        if name == "Entity" and len(args) == 1 and not out and gk_overlay.panel_entity(self, ctx, args[0]):
            return gk_overlay.panel_entity(self, ctx, args[0])
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
                return ["leaf", "range", ""]
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
                return gk_overlay.result_desc(self, ctx, args, out)
            raise Unsup("norule", f"std type {p or name}")
        eg = gk_editor.gpui_value(self, hc, name, args, out)
        if eg:
            return eg
        lg = gk_lists.gpui_value(self, hc, name, args, out)
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
            acr, a0 = ctx.self_t
        if a0 is not None and f["kind"] == "struct" and "resolved_path" in a0 and not path_args(a0["resolved_path"]):
            ahc, ahid = self.api.home(acr, a0["resolved_path"]["id"])
            if (hc, hid, (ahc, ahid)) in self.node_of:
                return ["node", self.node_of[(hc, hid, (ahc, ahid))]]
        rt = self.rtype(ctx, t)
        if f["kind"] == "enum":
            return self.enum(ctx, hc, hid, f, rt, args, out)
        if self._closure and out and gk_lists.payload_object(self, hc, hid, f):
            return ["obj", rt, self.api.pubpath(hc, hid), f["clone"]]
        if not self._closure and self.is_obj(hc, hid):
            return ["obj", rt, self.api.pubpath(hc, hid), f["clone"]]
        if f["state"]:
            raise Unsup("todo:protocol: state (a state object by value)", name)
        if f["record"]:
            return self.record(ctx, hc, hid, f, rt, args, out)
        if f["kind"] == "struct" and out and not f["generic"]:
            return self.record(ctx, hc, hid, f, rt, args, out)
        if f["generic"]:
            raise Unsup("todo:generic library type (needs an instantiation)", rdj.ty(t))
        raise Unsup("todo:opaque library object without a constructor the binding can drive", name)

    def fields(self, ctx, c, ids, out, names=True):
        res = []
        for n, fid in enumerate(ids):
            fi = c.item(fid) if fid is not None else None
            if fi is None:
                if out:
                    continue
                return None
            sv = self._closure
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
        """Register a unit enum in `self.enums` or a payload-bearing enum in `self.denums`.

        Tracks `_busy` to detect recursive enums (delegating `BlockNode` to
        `gk_editor.recursive`), captures per-variant target `cfg` attributes,
        and marks individual variants `ok=False` when a field cannot cross the wire.
        """
        hd = gk_editor.held(self, rt, out)
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
            rec = gk_editor.recursive(self, rt, out)
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
                    v["cfg"] = vc
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
        gk_editor.variant_ctors(self, f, rt, variants, out)
        ok = [v for v in variants if v["ok"]]
        if not ok and not out:
            raise Unsup("todo:enum with no variant the binding can build", f["name"])
        e = {"kind": "denum" if data else "enum", "rust": rt, "path": rt.split("<")[0], "name": f["name"],
             "variants": variants, "in": not out, "out": out, "constructible": bool(ok), "crate": hc}
        ec = self.api.walks[hc].cfg.get(str(hid))
        if ec:
            e["cfg"] = ec
        (self.denums if data else self.enums)[rt] = e
        return [e["kind"], rt]

    def rec_complete(self, rt, seen=None):
        seen = set() if seen is None else seen
        r = self.records.get(rt)
        if r is None or rt in seen:
            return r is not None
        seen.add(rt)
        return r["complete"] and all(self.rec_complete(x[1], seen) for fd in r["fields"]
                                     for x in walk_desc(fd["desc"]) if x[0] == "rec")

    def record(self, ctx, hc, hid, f, rt, args, out):
        """Register a plain or tuple struct in `self.records`.

        Inbound records (`out=False`) require all fields to be public and wire-decodable
        (`complete=True`). Outbound records (`out=True`, for events or readbacks) may
        omit private or unencodable fields as long as at least one public field remains.
        """
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
        self._closure += 1
        try:
            return self.closure_desc(ctx, kind, inputs, output, wrap, static)
        finally:
            self._closure -= 1

    def closure_desc(self, ctx, kind, inputs, output, wrap, static=True):
        """Classify a Rust closure signature into a wire descriptor.

        Evaluates closure families in strict precedence order because some signatures
        match multiple patterns (for example, a closure returning `AnyElement` with a
        `&Datum` argument must become `datum_rows` before generic `ctxrows`/`items`):
        1. `gk_app.action_listener` / `gk_app.platform_closure` (action, AnyView, drop/a11y).
        2. `data_closure` (chart `Field`/`Format` over `&Datum`).
        3. `gk_values.closure` (comp6, drag preview/style, reactive `Cell`, `RenderOptions` fragment).
        4. `gk_lists.expr_closure` (pure numeric/bool/enum -> scalar `Expr` formula).
        5. `gk_editor.closure` (`tfrag`, `keyset`, `table`, `ctxlazy`, `viewrows`).
        6. Endomorphism `T -> T`: `stylefn` (`StyleRefinement -> StyleRefinement`), `build`
           (node builder closure), or `gk_editor.decorate` (`FnOnce(El) -> El` decorator).
        7. Element-returning (`AnyElement` / `impl IntoElement`): `datum_rows`, `ctxrows`,
           `build_over_handed`, `gk_items.closure`, `viewfn`, or zero-payload `fragment`.
        8. Value-returning: `[rows]` shim overrides, `[values].event_closures`, `ask_closure`,
           or typed-node `gk_items.closure`.
        9. Void/bool event handlers: encodes payload args with `out=True`, normalizes
           conversions via `strip_conv()`, and interns the signature in `self.shapes`
           (matching `BUILTIN_SHAPES` 1..10 where applicable).
        """
        app = gk_app.action_listener(self, ctx, inputs, output)
        if app:
            return app
        pc = gk_app.platform_closure(self, ctx, kind, inputs, output, wrap)
        if pc:
            return pc
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
            return vc
        ex = gk_lists.expr_closure(self, ctx, inputs, output, wrap, params, ptypes, payload)
        if ex is not None:
            return ex
        ed = gk_editor.closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload)
        if ed is not None:
            return ed
        if output is not None and len(payload) == 1 and rdj.ty(payload[0]) == out_s:
            if last(out_s) == "StyleRefinement" and len(params) == 1:
                return ["stylefn", self.desc(ctx, payload[0])[1], wrap]
            d = self.desc(ctx, payload[0])
            if d[0] == "node":
                if gk_overlay.context_build(params):
                    return ["build", d[1], params, wrap, ptypes]
                if queued or not has_cx and (kind != "FnOnce" or wrap != "impl" or len(params) != 1 or static):
                    dec = None if queued else gk_editor.decorate(self, d[1], kind, params, wrap, ptypes)
                    if dec:
                        return dec
                    raise Unsup("todo:protocol: build closure that gets no window and app", out_s)
                return ["build", d[1], params, wrap, ptypes]
            raise Unsup("todo:protocol: data (a closure that maps a value)", out_s)
        is_el = output is not None and ("impl_trait" in output and "IntoElement" in rdj.ty(output)
                                        or last(out_s) == "AnyElement")
        if output is not None and not is_el and "generic" in output and output["generic"] in ctx.bounds:
            is_el = "IntoElement" in [n for n, _ in bound_parts(ctx.bounds[output["generic"]])]
        if is_el and has_cx and not queued and len(payload) == 1 and self.datum_arg(ctx, payload[0]) is not None:
            return ["datum_rows", wrap, ptypes, self.datum_arg(ctx, payload[0])]
        if is_el:
            cr = payload and gk_lists.ctx_rows(self, ctx, kind, inputs, output, wrap, params, ptypes, payload)
            if cr:
                return cr
            if payload:
                bo = gk_items.build_over_handed(self, ctx, kind, inputs, output, wrap, params, ptypes, payload, static)
                if bo:
                    return bo
                ic = gk_items.closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload, True)
                if ic:
                    return ic
                raise Unsup("skip:item-render-closure", f"{kind}({', '.join(rdj.ty(t) for t in inputs)}) -> {out_s}")
            if params == ["window", "app_ref"]:
                return gk_lists.view_closure(params, wrap, ptypes)
            if not has_cx or queued:
                raise Unsup("todo:protocol: fragment closure that gets no window and app", out_s)
            return ["fragment", params, wrap, ptypes]
        if out_s not in ("()", "bool") and output is not None:
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
            return ev
        ask = gk_lists.ask_closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload)
        if ask is not None:
            return ask
        if payload and output is not None and out_s not in ("()", "bool"):
            ic = gk_items.closure(self, ctx, kind, inputs, output, wrap, params, ptypes, payload, False)
            if ic:
                return ic
        if out_s not in ("()", "bool"):
            raise Unsup("todo:protocol: data (a closure that returns a value)", f"-> {out_s}")
        if queued and out_s != "()":
            raise Unsup("todo:protocol: queued events (a queued handler cannot return a value)", out_s)
        if not (has_cx or queued or "app" in params) and not gk_overlay.observed(params, out_s, kind):
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


    def datum_arg(self, ctx, t):
        """Detect a `&Datum` (or `&Wrapper<&Datum>`) parameter and return the field accessor or ''."""
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
        """Match chart/plot data accessor closures (`field`) and label formatting closures (`format`)."""
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
                forms.append(["other"])
        datums = [f for f in forms if f[0] == "datum"]
        if len(datums) > 1:
            return None
        rk = self.ret_kind(ctx, output)
        if rk is None:
            return None
        if datums and (len(forms) == 1 or rk[0] != "text"):
            return ["field", rk[0], rk[1], wrap, ptypes, forms]
        if rk[0] == "text" and ["other"] not in forms:
            return ["format", rk[0], rk[1], wrap, ptypes, forms]
        return None


    def fn_ctx(self, base, fn):
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
        return gk_values.fn_ctx(self, c, fn)

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
            it["doc"] = [fcrate, str(f["id"])]
        self.items.append(it)
        row["items"].append(it)
        return it, fn

    def set(self, it, e):
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
        """Attempt to classify all arguments of `fn` and mark `it` as bound or bridged.

        Rejects methods with unmentioned caller-chosen type parameters or
        non-cloneable object arguments passed to element builders. Sets status
        to `bridged` when any argument uses a runtime protocol descriptor.
        """
        try:
            text = " ".join(rdj.ty(t) for _, t in params)
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
        """Classify one inherent public method on a struct into ctor, op, convert, paint, ambient, or skip."""
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
            listed = rn is None and gk_lists.list_ctor(self, node, it, fcrate, fn, fctx)
            if rn is None and gk_values.static_call(self, it, fcrate, fn, ctx.self_t):
                return
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
            pass
        elif recv == "&Self":
            self.set(it, Unsup("skip:builder-getter", "a read of a builder the app is writing"))
        elif recv == "&mut Self" and gk_editor.mutator(self, node, it, fn, ctx, tp):
            pass
        elif recv == "&mut Self":
            self.set(it, Unsup("skip:builder-mutator", "the in-place form of a builder"))
        else:
            self.set(it, Unsup("skip:rust-only-extension", f"receiver {recv}"))

    PAINT_CTX = {"&mut Window": "window", "&mut gpui::Window": "window", "&mut App": "app", "&mut gpui::App": "app"}

    def paints(self, it, node, f, fn, ins, out, tp):
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


    # Descriptors legal inside call-op arguments (outside view-tape decoding).
    CALL_SAFE = {"bool", "int", "f32", "f64", "str", "leaf", "enum", "opt", "list", "tuple", "ref", "entity", "shim",
                 "obj", "refmut", "libval", "char", "font", "handler", "expr", "nonzero", "ask", "rangeof", "rcvec", "arcslice"}

    def classify_state(self, row, f, c, crate, ctx, tpath):
        """Classify a GPUI Entity state struct: EventEmitter subscriptions plus inherent methods."""
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
                        gk_editor.event_reads(self, st, crate, ctx, tpath, path_args(tr)[0])
                continue
            for mid in im["items"]:
                fi = c.item(mid)
                if not fi or "function" not in fi["inner"] or fi["visibility"] != "public" or fi["name"] in seen:
                    continue
                seen.add(fi["name"])
                self.state_method(row, st, fi, crate, ctx)

    def state_method(self, row, st, f, crate, ctx):
        """Classify one method on a GPUI Entity state into state-ctor, state-builder, state-read, state-write, or state-call."""
        it, fn = self.new_item(row, row["path"], f, crate)
        if self.pre(it, f, fn):
            return
        ins, out = fn["sig"]["inputs"], fn["sig"].get("output")
        recv = rdj.ty(ins[0][1]) if ins and ins[0][0] == "self" else None
        fctx = self.fn_ctx(ctx, fn)
        out_s = rdj.ty(out) if out else "()"
        out, out_s = gk_lists.shim_result(self, fctx, out, out_s)
        kind = {None: "state-ctor", "Self": "state-builder", "&Self": "state-read", "&mut Self": "state-write"}.get(recv)
        if kind == "state-read" and out_s == "()" and any(
                re.match(r"&mut (Window|Context)\b", rdj.ty(t)) for _, t in ins[1:]):
            kind = "state-write"
        if kind == "state-read" and out_s != "()" and gk_overlay.read_needs_window(ins):
            kind = "state-call"
        fallible = kind in ("state-ctor", "state-builder") and gk_overlay.is_result_self(out_s, st["name"])
        lent = None
        try:
            if kind is None:
                raise Unsup("skip:rust-only-extension", f"receiver {recv}")
            if kind in ("state-ctor", "state-builder") and out_s not in ("Self", st["name"]) and not fallible:
                raise Unsup("todo:protocol: calls (a static function that is not a constructor)" if recv is None
                            else "skip:rust-only-extension", out_s)
            if kind == "state-write" and out_s != "()":
                lent = "&" in out_s and gk_lists.lend(self, fctx, st, out)
                if "&" in out_s and not lent:
                    raise Unsup("todo:protocol: state (an update that returns a value)", "a borrow: " + out_s)
                kind = "state-call"
            args = []
            text = gk_overlay.named_text(fn, " ".join(rdj.ty(t) for _, t in ins))
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
                    bad = None
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
        if fallible:
            it["callee"] = gk_overlay.fallible_callee(it["callee"], len(it["args"]) + (kind == "state-builder"))
        st[{"state-ctor": "ctors", "state-builder": "builders", "state-read": "reads", "state-write": "writes",
            "state-call": "calls"}[kind]].append(it)

    def call_bad(self, d, builder, seen=None):
        """Check whether descriptor `d` is valid in a call op (or state builder when `builder=True`).

        Returns None if every nested descriptor can be decoded without a view-build
        `Dx` context; otherwise returns the offending descriptor tag. Marks nested
        records, data enums, and Into sets as `callable=True` so `gk_rust.py` emits
        their `_c(r)` call-mode decoder variants.
        """
        seen = set() if seen is None else seen
        eb = None if seen else gk_editor.call_bad(self, d, builder)
        if eb is not None:
            return eb[0]
        for x in walk_desc(d):
            k = x[0]
            if k == "node" and not builder and not seen and x[1] in self.nodes and not self.nodes[x[1]]["element"]:
                continue
            if k == "opt" and x[1][0] == "ref" and x[1][1][0] == "obj" and (builder or seen):
                return "obj"
            if k == "refmut" and (builder or seen) or k == "obj" and (builder or seen) and not x[3]:
                return k
            if k == "rec" and x[1] in self.records and not self.records[x[1]]["complete"]:
                return "rec"
            if gk_overlay.view_built(self, x, builder, seen):
                continue
            if k in gk_editor.CALL_SAFE:
                continue
            if gk_app.call_safe(x):
                continue
            if k in gk_lists.CALL_SAFE:
                continue
            if k in gk_values.CALL_SAFE or (k == "ctx" and x[1] == "window_opt" and not builder and not seen):
                continue
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


    def is_obj(self, hc, hid):
        """Return True if `(hc, hid)` is a non-entity, non-generic struct managed behind an object handle."""
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
        """Classify a runtime-managed library object struct (including GPUI `Global` singletons)."""
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
        gk_lists.default_ctor(self, st, f, crate)

    OBJ_CTX = {"obj-ctor": ("window", "app", "app_ref"), "obj-builder": (), "obj-read": ("app_ref",),
               "obj-write": ("window", "window_ref", "app", "app_ref"), "obj-call": ("window", "window_ref", "app", "app_ref"),
               "obj-global": ("app", "app_ref"), "obj-static": ("window", "window_ref", "app", "app_ref"),
               "obj-scall": ("window", "window_ref", "app", "app_ref")}

    def object_method(self, row, st, f, crate, ctx):
        """Classify one method on a library object into obj-ctor, obj-global, obj-builder, obj-read/write/call, or obj-static/scall."""
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
            text = gk_overlay.named_text(fn, " ".join(rdj.ty(t) for _, t in ins))
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
                    bad = None
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
        if d[0] == "obj" or (d[0] == "fallible" and d[1][0] == "obj"):
            return
        for x in walk_desc(d):
            if x[0] == "obj":
                if not x[3]:
                    raise Unsup("todo:protocol: object (a result that cannot be copied out of a borrow)", out_s)

    def obj_arg_ok(self, it):
        for a in it["args"]:
            for x in walk_desc(a["desc"]):
                if x[0] == "refmut":
                    return False
                if x[0] == "obj" and not x[3]:
                    return False
        return True

    def reach_objects(self):
        """Compute fixed-point reachability over library objects starting from constructors, globals, returns, and events.

        Objects that no bound constructor, global accessor, return value, item
        closure, or event payload can ever produce are marked unreachable (`self.dead_objs`),
        and any method taking an unreachable object argument is downgraded to `todo`.
        """
        why = "protocol: object (no constructor, global or bound function hands one out)"
        objs = self.objects
        items = self.all_items()
        live = lambda i: i["status"] in ("bound", "bridged")
        reach = set()
        while True:
            new = set()
            for key, st in objs.items():
                if key not in reach and any(live(i) for i in st["ctors"] + st["globals"]):
                    new.add(key)
                if key not in reach and any(live(i) and any(x[0] == "obj" and x[2] == key for x in walk_desc(i.get("ret") or ["unit"]))
                                            for i in st.get("statics", [])):
                    new.add(key)
            for it in items:
                if not live(it) or (it.get("state") in objs and it["state"] not in reach):
                    continue
                for x in walk_desc(it.get("ret") or ["unit"]):
                    if x[0] == "obj" and x[2] not in reach:
                        new.add(x[2])
                new.update(k for k in gk_lists.handed_out(it) if k not in reach)
                new.update(k for k in gk_editor.handed_out(it) if k not in reach)
            for sh in self.shapes.values():
                for d in sh["args"]:
                    for x in gk_values.walk_tables(self, d):
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


    STATE_SPEC = re.compile(r"\s*(\w+)<\s*([\w:]+)\s*>\s+as\s+(\w+)\s*")

    def state_specs(self, crate, iid):
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
        if arg.startswith("crate::"):
            return None, {"verbatim": arg}, None
        found = [(cr, i) for cr in CRATES for i, (_, it) in self.api.walks[cr].types.items() if it["name"] == arg]
        if len(found) != 1:
            raise SystemExit(f"gkgen: overrides [instantiate] states: {arg} names {len(found)} types")
        cr, i = found[0]
        f = self.info(cr, i)
        return cr, {"resolved_path": {"path": arg, "id": int(i), "args": None}}, f["traits"] | f["blanket"]

    def impl_applies(self, crate, im, arg_ref, arg_traits):
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
            return {p: arg_ref[1]}
        if "resolved_path" in a and "resolved_path" in arg_ref[1]:
            if self.api.home(crate, a["resolved_path"]["id"]) == self.api.home(arg_ref[0], arg_ref[1]["resolved_path"]["id"]):
                return {}
        return None

    def alias_path(self, crate, base_name, arg, name):
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
        """Instantiate generic state types from `[instantiate] states` and attach summary rows."""
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
            gk_editor.event_reads(self, st, crate, ctx, tpath, ev)

    def pre(self, it, f, fn):
        """Apply early skip/todo filters: overrides `[skip]`, deprecated items, unsafe fns, and async fns."""
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
        """Classify every public struct/enum, trait, and free function exported by a kit crate."""
        c, w = self.api.crates[crate], self.api.walks[crate]
        pre = SHORT[crate]
        for iid, (path, it) in sorted(w.types.items(), key=lambda kv: kv[1][0]):
            f = self.info(crate, iid)
            tpath = self.api.pubpath(crate, iid)
            node = self.nodes.get(self.node_of.get((crate, iid)))
            shim_node = next((self.nodes[k] for o, k in self.node_of.items()
                              if len(o) == 3 and o[:2] == (crate, iid) and o[2][0] in ("shim", "inst")), None)
            if node is None and gk_values.multi_inst(self, crate, iid, f, c, f"{pre}::{it['name']}", tpath):
                continue
            if node is None and shim_node is not None:
                node = shim_node
                row = self.row(f"{pre}::{it['name']}", tpath, "component" if node["element"] else "builder", crate)
                self_t = self.inst_self(crate, iid, f, node)
                f2 = dict(f)
                f2["impls"] = [im for im in f["impls"] if self.impl_matches(im, node)]
                self.type_impls(row, node, f2, c, crate, Ctx(crate, self_t, node["subst"]), "node")
                continue
            if node is None and f["generic"] and gk_values.object_specs(self, crate, iid):
                row = self.row(f"{pre}::{it['name']}", tpath, "state", crate)
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
        """Classify a public trait's methods into style tokens, shared trait ops, type-changing converts, shims, or skips.

        Methods with additional `where Self: Trait` bounds are split into a
        separate sub-table keyed by `(crate, iid, tuple(extra))` so only nodes
        implementing the extra bounds inherit those ops.
        """
        c = self.api.crates[crate]
        tr = tit["inner"]["trait"]
        tpath = self.api.pubpath(crate, iid)
        row = self.row(rowname, tpath, "trait" if crate != "gpui" else "gpui", crate)
        if tit["name"] == "Styled":
            self.token_row = self.row("Styled (nullary methods, as style tokens)", tpath, "style", crate)
            row["name"] = "gpui::Styled (methods with arguments)"
        generic = any("type" in p["kind"] for p in tr["generics"]["params"])
        selfgen = generic and gk_values.self_generic_trait(self, crate, tr)
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
            table["path"] = f"{tpath}<E>"
            table["extra_paths"] = ["gpui_kit::Styled"]
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
        """Classify a free function as a palette color, ambient call, or free node constructor."""
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
        """Classify the curated subset of upstream `gpui` types, traits, and free functions from `[gpui]`."""
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
        """Extract every public `Hsla` field name from `ThemeColor` into `self.theme`."""
        c = self.api.crates["gpui_component"]
        for _, (path, it) in self.api.walks["gpui_component"].types.items():
            if it["name"] == "ThemeColor" and "struct" in it["inner"]:
                for fid in it["inner"]["struct"]["kind"]["plain"]["fields"]:
                    fi = c.item(fid)
                    if fi and last(rdj.ty(fi["inner"]["struct_field"])) == "Hsla":
                        self.theme.append(fi["name"])

    def impl_matches(self, im, node):
        """Return True if impl block `im` applies to instantiated generic node `node`.

        Unconstrained generic impls (`impl<T> Foo<T>`) match every instantiation; specialized
        impls (`impl Foo<Div>`) match only when each concrete argument's short type name equals
        the instantiation's substituted type (never matching `crate::` shim instantiations).
        """
        args = path_args(im["for"]["resolved_path"]) if "resolved_path" in im["for"] else []
        if node["inst"][0] == "inst":
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


    def settle(self):
        """Finalize trait tables, run extension post-passes, prune unreachable items, and verify overrides.

        Exact stage order matters because later passes depend on reachability and wire-direction
        flags computed by earlier passes:
        1. Resolve each node's `tables` (including conditional `extra`-bound sub-tables) and
           instantiate type-changing trait conversions (`trait_convert`).
        2. Drop methods on non-object, non-handed states that have no bound constructor.
        3. `gk_overlay.post` -> `prune` -> `gk_editor.project` -> `gk_editor.before_reach`
           (snapshots pre-prune object methods) -> `reach_objects` -> second `prune` if any
           objects were found unreachable.
        4. `flow()` propagates `in`/`out` wire flags transitively through records, data enums,
           and `Into` sets.
        5. `gk_editor.revive` -> `gk_values.post` (globals, pointer ctors, readbacks, handed
           states, nested iterators, `out_flags`) -> `gk_editor.after_values` (lent edits,
           flat record writes, event reads, answer sets, nested-iterator revival).
        6. Assert that every entry in `[skip]`, `[shims]`, `[instantiate].states`, `[lists].*`,
           `[editor].variants`, `[rows]`, `[traits.role]`, and `[shorthand]` was used.
        """
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
            if not st["ctors"] and not st.get("obj") and not st.get("handed"):
                for it in st["builders"] + st["reads"] + st["writes"] + st["calls"]:
                    it["status"], it["reason"] = "todo", "protocol: state (no constructor of the state type is bound)"
                st["builders"], st["reads"], st["writes"], st["calls"], st["events"] = [], [], [], [], []
        gk_overlay.post(self)
        self.dead_objs = set()
        self.prune()
        gk_editor.project(self)
        gk_editor.before_reach(self)
        self.reach_objects()
        if self.dead_objs:
            self.prune()
        self.flow()
        gk_editor.revive(self)
        gk_values.post(self)
        gk_editor.after_values(self)
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
        for rt, field in self.ov.get("shorthand", {}).items():
            rec = self.records.get(rt)
            f = next((f for f in rec["fields"] if f["name"] == field), None) if rec and rec.get("in") else None
            if f is None or f["desc"][0] != "int":
                raise SystemExit(f"gkgen: overrides [shorthand] names no record an app writes with the integer field: {rt}.{field}")
            rec["shorthand"] = field

    def all_items(self):
        """Return a deduplicated list of all items across `self.items`, node constructors, and conversions."""
        seen, out = set(), []
        for it in self.items + [i for n in self.nodes.values() for i in n["ctors"] + n.get("converts", [])]:
            if id(it) not in seen:
                seen.add(id(it))
                out.append(it)
        return out

    def prune(self, dead_types=()):
        """Iteratively drop nodes, types, shapes, and items that depend on unconstructible or quarantined types.

        Runs to a fixed point:
        - Any node with zero bound/bridged constructors enters `dead`.
        - Records, data-enum variants, `Into` union members, and handler shapes referencing a
          dead node, dead object, or dead type are disabled/removed.
        - Any bound/bridged item whose argument, return type, or `from` node references a dead
          entity is downgraded to `todo`, which may in turn remove the last constructor of another
          node on the next iteration.
        """
        opaque = "opaque library object without a constructor the binding can drive"
        dead_t = set(dead_types)
        items = self.all_items()
        while True:
            dead = {k for k, n in self.nodes.items() if not any(i["status"] in ("bound", "bridged") for i in n["ctors"])}
            bad = lambda d: any((x[0] in ("node", "build", "stylefn") and x[1] in dead) or
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
        """Propagate `in`/`out` wire direction flags transitively through records, data enums, and `Into` sets.

        Ensures that if a parent record or data enum is decoded from the wire (`in=True`) or
        serialized back to Go (`out=True`), every nested record, enum, or data enum referenced
        in its fields/variants receives the same direction flag so `gk_rust.py` and `gk_go.py`
        emit all required `Unwire`/`Wire`/`put*`/`rd*` helpers.
        """
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
        """Instantiate a type-changing trait method (such as `InteractiveElement::id`) on `node`."""
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
    """Normalize a descriptor by removing Rust-side conversion tags so equivalent handler shapes dedupe."""
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
    """Yield `d` and every recursively nested container descriptor (opt, ref, list, tuple, etc.)."""
    yield d
    if d[0] in ("opt", "ref", "list", "from", "refmut", "fallible", "array"):
        yield from walk_desc(d[1])
    elif d[0] == "tuple":
        for x in d[1]:
            yield from walk_desc(x)


def uses_protocol(d):
    """Return True if descriptor `d` relies on a runtime protocol beyond plain value serialization."""
    return any(x[0] in ("handler", "fragment", "build", "entity", "handle", "action", "stylefn", "shim", "rows",
                        "data", "field", "format", "datum_rows", "obj", "refmut", "viewfn", "expr", "ask", "dock",
                        "ctxrows", "lent", "items") or x[0] in gk_app.PROTOCOL_KINDS
               for x in walk_desc(d))


def names_node(d, keys):
    """Return True if descriptor `d` references any node key in `keys` via `node` or `build`."""
    return any(x[0] in ("node", "build") and x[1] in keys for x in walk_desc(d))
