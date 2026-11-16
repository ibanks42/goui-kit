"""Reader for rustdoc JSON (format 61, rustdoc 1.99): public-API walk and type printer.

Reads the raw JSON rustdoc writes or the stripped, gzipped extract under api/rustdoc/.
No third-party imports.
"""
import gzip
import json
import re


class Crate:
    def __init__(self, path, label):
        with (gzip.open(path, "rt") if path.endswith(".gz") else open(path)) as f:
            d = json.load(f)
        self.label = label
        self.format = d["format_version"]
        self.version = d.get("crate_version")
        self.idx = d["index"]
        self.paths = d["paths"]
        self.ext = d["external_crates"]
        self.root = str(d["root"])
        self.rootname = self.idx[self.root]["name"]

    def item(self, i):
        return self.idx.get(str(i))

    def is_local(self, i):
        it = self.idx.get(str(i))
        if it is not None:
            return it["crate_id"] == 0
        p = self.paths.get(str(i))
        return bool(p) and p["crate_id"] == 0

    def ext_path(self, i):
        p = self.paths.get(str(i))
        return "::".join(p["path"]) if p else None

    def kind(self, it):
        return next(iter(it["inner"]))


# ---------------------------------------------------------------- type printer

def args_str(a):
    if not a:
        return ""
    if "angle_bracketed" in a:
        parts = []
        for x in a["angle_bracketed"]["args"]:
            if isinstance(x, str):
                parts.append("_")
            elif "type" in x:
                parts.append(ty(x["type"]))
            elif "lifetime" in x:
                parts.append(x["lifetime"])
            elif "const" in x:
                parts.append(str(x["const"].get("expr", "_")))
        for c in a["angle_bracketed"].get("constraints", []):
            b = c["binding"]
            if "equality" in b:
                e = b["equality"]
                parts.append("%s = %s" % (c["name"], ty(e["type"]) if "type" in e else "_"))
            else:
                parts.append("%s: %s" % (c["name"], bounds_str(b["constraint"])))
        return "<%s>" % ", ".join(parts) if parts else ""
    if "parenthesized" in a:
        p = a["parenthesized"]
        s = "(%s)" % ", ".join(ty(x) for x in p["inputs"])
        if p.get("output"):
            s += " -> " + ty(p["output"])
        return s
    return ""


def bound_str(b):
    if "trait_bound" in b:
        tb = b["trait_bound"]
        m = {"none": "", "maybe": "?", "maybe_const": "~const "}.get(tb.get("modifier", "none"), "")
        hr = ""
        if tb.get("generic_params"):
            hr = "for<%s> " % ", ".join(g["name"] for g in tb["generic_params"])
        return hr + m + tb["trait"]["path"] + args_str(tb["trait"].get("args"))
    if "outlives" in b:
        return b["outlives"]
    if "use" in b:
        return "use<..>"
    return "?"


def bounds_str(bs):
    return " + ".join(bound_str(b) for b in bs)


def ty(t):
    if t is None:
        return "()"
    if "resolved_path" in t:
        r = t["resolved_path"]
        return r["path"] + args_str(r.get("args"))
    if "generic" in t:
        return t["generic"]
    if "primitive" in t:
        return t["primitive"]
    if "borrowed_ref" in t:
        r = t["borrowed_ref"]
        return "&%s%s%s" % ((r["lifetime"] + " ") if r.get("lifetime") else "",
                            "mut " if r["is_mutable"] else "", ty(r["type"]))
    if "tuple" in t:
        return "(%s)" % ", ".join(ty(x) for x in t["tuple"])
    if "slice" in t:
        return "[%s]" % ty(t["slice"])
    if "array" in t:
        return "[%s; %s]" % (ty(t["array"]["type"]), t["array"]["len"])
    if "impl_trait" in t:
        return "impl " + bounds_str(t["impl_trait"])
    if "dyn_trait" in t:
        d = t["dyn_trait"]
        parts = []
        for p in d["traits"]:
            hr = ""
            if p.get("generic_params"):
                hr = "for<%s> " % ", ".join(g["name"] for g in p["generic_params"])
            parts.append(hr + p["trait"]["path"] + args_str(p["trait"].get("args")))
        if d.get("lifetime"):
            parts.append(d["lifetime"])
        return "dyn " + " + ".join(parts)
    if "function_pointer" in t:
        f = t["function_pointer"]
        s = "fn(%s)" % ", ".join(ty(x[1]) for x in f["sig"]["inputs"])
        if f["sig"].get("output"):
            s += " -> " + ty(f["sig"]["output"])
        if f["header"].get("is_unsafe"):
            s = "unsafe " + s
        return s
    if "raw_pointer" in t:
        r = t["raw_pointer"]
        return "*%s %s" % ("mut" if r["is_mutable"] else "const", ty(r["type"]))
    if "qualified_path" in t:
        q = t["qualified_path"]
        st = ty(q["self_type"])
        if q.get("trait") and q["trait"].get("path"):
            return "<%s as %s>::%s%s" % (st, q["trait"]["path"] + args_str(q["trait"].get("args")),
                                         q["name"], args_str(q.get("args")))
        return "%s::%s%s" % (st, q["name"], args_str(q.get("args")))
    if "pat" in t:
        return ty(t["pat"]["type"])
    if "infer" in t:
        return "_"
    return "?"


def generics_str(g):
    ps = []
    for p in g["params"]:
        k = p["kind"]
        if "type" in k:
            if k["type"].get("is_synthetic"):
                continue
            b = [x for x in k["type"]["bounds"]]
            ps.append(p["name"] + (": " + bounds_str(b) if b else ""))
        elif "lifetime" in k:
            ps.append(p["name"])
        elif "const" in k:
            ps.append("const %s: %s" % (p["name"], ty(k["const"]["type"])))
    return "<%s>" % ", ".join(ps) if ps else ""


def where_str(g):
    ws = []
    for w in g.get("where_predicates", []):
        if "bound_predicate" in w:
            b = w["bound_predicate"]
            ws.append("%s: %s" % (ty(b["type"]), bounds_str(b["bounds"])))
        elif "lifetime_predicate" in w:
            b = w["lifetime_predicate"]
            ws.append("%s: %s" % (b["lifetime"], " + ".join(b["outlives"])))
    return " where " + ", ".join(ws) if ws else ""


def fn_sig(name, f):
    sig = f["sig"]
    ps = []
    for pname, pt in sig["inputs"]:
        if pname == "self":
            s = ty(pt)
            ps.append({"Self": "self", "&Self": "&self", "&mut Self": "&mut self"}.get(s, "self: " + s))
        else:
            ps.append("%s: %s" % (pname, ty(pt)))
    h = f["header"]
    pre = ("const " if h.get("is_const") else "") + ("async " if h.get("is_async") else "") + \
          ("unsafe " if h.get("is_unsafe") else "")
    out = (" -> " + ty(sig["output"])) if sig.get("output") else ""
    return "%sfn %s%s(%s)%s%s" % (pre, name, generics_str(f["generics"]), ", ".join(ps), out,
                                  where_str(f["generics"]))


# ---------------------------------------------------------------- platform cfg

_SPAN = r"\S+:\d+:\d+: \d+:\d+ \(#\d+\)"
_TOKENS = re.compile(r'"(?:[^"\\]|\\.)*"|[A-Za-z_][A-Za-z0-9_]*|[\[\](){},:]')


def target_cfg(attrs):
    """The platform part of an item's #[cfg]: a Rust cfg predicate over target_* and unix/windows
    only (`target_os = "linux"`), or None when the item is on every platform. The extract is made
    on Linux with the library's features, so every other part of a cfg (a feature, a test flag)
    held there and is dropped: what remains is what keeps the item off macOS or Windows
    (gpui-fast's Wayland layer shell is Linux only). rustdoc 1.99 writes a cfg
    as `#[attr = CfgTrace([All([NameValue { name: "target_os", value: Some("linux"), span: .. }, ..])])]`."""
    preds = []
    for a in attrs or ():
        text = a.get("other", "") if isinstance(a, dict) else str(a)
        if "CfgTrace(" not in text:
            continue
        text = re.sub(r",\s*span: " + _SPAN, "", text)
        text = re.sub(r",\s*" + _SPAN, "", text)
        toks = _TOKENS.findall(text[text.index("CfgTrace(") + len("CfgTrace"):])
        pos = 0

        def expect(t):
            nonlocal pos
            if toks[pos] != t:
                raise ValueError(f"cfg: {t} expected at {toks[pos:pos + 5]} in {text[:200]}")
            pos += 1

        def node():
            nonlocal pos
            t = toks[pos]; pos += 1
            if t in ("All", "Any"):
                expect("("); kids = seq(); expect(")")
                if t == "Any" and any(k is None for k in kids):
                    return None
                kids = [k for k in kids if k is not None]
                if not kids:
                    return None
                return kids[0] if len(kids) == 1 else f"{t.lower()}({', '.join(kids)})"
            if t == "Not":
                expect("("); k = node(); expect(")")
                return None if k is None else f"not({k})"
            if t == "NameValue":
                expect("{"); name = value = None
                while toks[pos] != "}":
                    key = toks[pos]; pos += 1; expect(":")
                    if key == "name":
                        name = toks[pos].strip('"'); pos += 1
                    elif key == "value":
                        if toks[pos] == "Some":
                            pos += 1; expect("("); value = toks[pos]; pos += 2
                        else:
                            pos += 1
                    if toks[pos] == ",":
                        pos += 1
                expect("}")
                if not (name.startswith("target_") or name in ("unix", "windows")):
                    return None
                return name if value is None else f"{name} = {value}"
            if t in ("True", "False"):
                return None if t == "True" else "any()"
            raise ValueError(f"cfg: unknown {t} in {text[:200]}")

        def seq():
            nonlocal pos
            expect("["); kids = []
            while toks[pos] != "]":
                kids.append(node())
                if toks[pos] == ",":
                    pos += 1
            expect("]")
            return kids

        expect("(")
        kids = [k for k in seq() if k is not None]
        preds += kids
    if not preds:
        return None
    return preds[0] if len(preds) == 1 else f"all({', '.join(preds)})"


def all_cfg(*preds):
    """Both platform predicates (either may be None, for every platform)."""
    ps = [p for p in preds if p]
    if not ps:
        return None
    return ps[0] if len(ps) == 1 else f"all({', '.join(ps)})"


# ---------------------------------------------------------------- public walk

class Walk:
    """Collects every item reachable through public paths from the crate root."""

    def __init__(self, crate):
        self.c = crate
        self.types = {}      # id -> (path, item)   structs, enums, unions
        self.traits = {}     # id -> (path, item)
        self.fns = {}        # id -> (path, item)   free functions
        self.macros = {}     # id -> (path, item)
        self.other = {}      # id -> (path, kind)   consts, statics, aliases
        self.ext_reexports = []  # (path, source, is_glob)
        self.cfg = {}        # id -> the platform cfg of a type and its modules (target_cfg), if any
        self._mods = set()
        self._module(crate.root, [crate.rootname])

    def _module(self, mid, path, cfg=None):
        key = (mid, "::".join(path))
        if key in self._mods:
            return
        self._mods.add(key)
        m = self.c.item(mid)
        cfg = all_cfg(cfg, target_cfg(m.get("attrs")))
        for i in m["inner"]["module"]["items"]:
            it = self.c.item(i)
            if it is None:
                continue
            self._item(it, path, it.get("name"), cfg)

    def _item(self, it, path, name, cfg=None):
        c = self.c
        k = c.kind(it)
        if k == "use":
            u = it["inner"]["use"]
            if it["visibility"] != "public":
                return
            tid = u.get("id")
            target = c.item(tid) if tid is not None else None
            if target is None or target["crate_id"] != 0:
                self.ext_reexports.append(("::".join(path), u["source"], u["is_glob"]))
                return
            tk = c.kind(target)
            if u["is_glob"]:
                if tk == "module":
                    self._module(str(tid), path, cfg)
                return
            uname = target.get("name") if u["name"] == "_" else u["name"]
            if tk == "module":
                self._module(str(tid), path + [uname], cfg)
            else:
                self._item(target, path, uname, cfg)
            return
        if it["visibility"] != "public" and k not in ("macro", "proc_macro"):
            return
        full = "::".join(path + [name or "?"])
        iid = str(it["id"])
        if k == "module":
            self._module(iid, path + [name], cfg)
        elif k in ("struct", "enum", "union"):
            own = all_cfg(cfg, target_cfg(it.get("attrs")))
            if self._keep(self.types, iid, full, it):
                if own:
                    self.cfg[iid] = own
                else:
                    self.cfg.pop(iid, None)
        elif k == "trait":
            self._keep(self.traits, iid, full, it)
        elif k == "function":
            self._keep(self.fns, iid, full, it)
        elif k in ("macro", "proc_macro"):
            self._keep(self.macros, iid, full, it)
        else:
            self._keep(self.other, iid, full, k)

    @staticmethod
    def _keep(table, iid, full, value):
        """An item re-exported under several paths is listed once, under its shortest path."""
        old = table.get(iid)
        if old is None or (full.count("::"), full) < (old[0].count("::"), old[0]):
            table[iid] = (full, value)
            return True
        return False
