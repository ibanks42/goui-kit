#!/usr/bin/env python3
"""conformance.py [--check] [--package DIR --odin-out FILE]: conformance pages by generation.

For every bound constructor and builder of an element type in the interface description
(api/gk.api.json.gz, written by the generator), writes one gallery page that shows the
element twice, side by side: as its constructor makes it, and with that one function
applied. A builder that lives in a shared trait table (Sizable.large) gets one page, on the
first element type that has the table. The page is written twice from the same description:

    ref/src/bin/gallery/conf.rs       calling GPUI-kit natively (no tape, no library)
    examples/odin/gallery/conf.odin   calling the Odin package, for what the package declares
    ref/gallery/conf.txt              the pages, in the format of pages.txt; the third column
                                      is the description's id of the function the page verifies

gates/gallery.sh then compares the two apps page by page like the hand-written pages; a
page that is byte-identical verifies its one function (gates/verified.json).

The Rust side is emitted for every bound function whose arguments this script can make.
The Odin side is emitted only for what the package (odin/gk, or --package DIR) declares
today, found by reading its source: the type named by a "// T is gpui_kit::path." comment
(or the upstream name in Ada_Case), its constructor procedure, the fields of <T>_VT and of
the <Trait>_Trait tables it uses, and each parameter's type. So pages light up by themselves
as generated bindings land: run this script, rebuild the reference (ref-gallery), then
gates/gallery.sh --record. Nothing here knows a component by name except DRESS (what makes
an element visible: a label, a child, a value) and FIRST (which types represent a trait).

Sample values, the same in both languages: string "Sample", id "conf-a" / "conf-b", bool
true, integer 3, float 40, pixels 40, colour hsla(0.6, 0.7, 0.5, 1), a unit enum's second
member, child a div holding the text "Child", duration 3 ms. A function with any other
kind of argument (records, enums with data, Into sets, other nodes, lists, handlers) has no
page yet and is counted by kind; gates/conformance-skip.txt quarantines single pages with
the compiler's or the run's reason.

    conformance.py                 rewrite the three files, print the counts
    conformance.py --check         exit 1 if the three files disagree with each other; says
                                   (without failing) when they are behind the description
    conformance.py --package gen/out/odin/gk --odin-out FILE
                                   write only the Odin side, against another package (to
                                   check with `odin check` what generated bindings will give)
Standard library only."""
import gzip
import json
import os
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
API = "api/gk.api.json.gz"
OUT_RS = "ref/src/bin/gallery/conf.rs"
OUT_ODIN = "examples/odin/gallery/conf.odin"
OUT_TXT = "ref/gallery/conf.txt"
SKIP = "gates/conformance-skip.txt"
KEYWORDS = {"switch", "map", "proc", "struct", "enum", "union", "import", "package", "for", "if", "in",
            "when", "defer", "return", "context", "matrix", "dynamic", "distinct", "using", "foreign",
            "case", "do", "else", "break", "continue", "fallthrough", "or_else", "or_return", "not_in",
            "bit_set", "bit_field", "cast", "transmute", "auto_cast", "where", "asm", "nil"}
# What makes an element of a type visible: (builder, text that replaces the default sample).
DRESS = {"Button": ("label", "Button"), "Checkbox": ("label", "Checkbox"), "Switch": ("label", "Switch"),
         "Radio": ("label", "Radio"), "Tag": ("child", "Tag"), "Avatar": ("name", "Ada Lovelace"),
         "Progress": ("value", None), "Badge": ("count", None), "Link": ("child", "Link"),
         "Div": ("child", "Div")}
# The types tried first as the representative of a trait table.
FIRST = ["Button", "Tag", "Checkbox", "Switch", "Label", "Div"]


def snake(name):
    return re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "_", name).lower()


def tail(path):
    """The last segment of a type's path without its generic arguments, nested or not
    (`gpui_kit::component::sidebar::Sidebar<gpui_kit::component::sidebar::SidebarMenu>` is
    `Sidebar`): the page id and the Odin names come from it."""
    return path.split("<", 1)[0].rsplit("::", 1)[-1]


def ada(name):
    return re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "_", name)


def odin_name(name):
    return name + "_" if name in KEYWORDS else name


# --- the Odin package, as declared today -------------------------------------------------------

def split_params(text):
    out, depth, cur = [], 0, ""
    for ch in text:
        depth += ch in "([{"
        depth -= ch in ")]}"
        if ch == "," and depth == 0:
            out.append(cur)
            cur = ""
        else:
            cur += ch
    if cur.strip():
        out.append(cur)
    params, pending = [], []
    for p in out:
        p = p.strip()
        if ":=" in p:          # a defaulted parameter (loc := #caller_location): not passed
            continue
        if ":" not in p:       # `a, b: T`
            pending.append(p)
            continue
        name, typ = [x.strip() for x in p.split(":", 1)]
        if typ.startswith(".."):   # variadic style flags: none passed
            pending = []
            continue
        for n in pending + [name]:
            params.append((n, typ))
        pending = []
    return params


class NoSample(Exception):
    pass


class Package:
    def __init__(self, d):
        src = ""
        for f in sorted(os.listdir(d)):
            if f.endswith(".odin"):
                src += open(os.path.join(d, f)).read() + "\n"
        self.procs, self.groups, self.vts, self.enums, self.types, self.traits = {}, {}, {}, {}, {}, {}
        for m in re.finditer(r"^// (\w+) is (gpui_kit::[\w:<>, ]+)\.$", src, re.M):
            self.types[m.group(2)] = m.group(1)
        # A constructor may be generic over the two string types (`text: $T ... where T == string
        # || T == Temp_String`): such a parameter is a string here.
        for m in re.finditer(r"^(\w+) :: proc\((.*?)\)\s*->\s*(\w+)\s*(where [^{]*)?\{", src, re.M | re.S):
            params = split_params(m.group(2))
            if m.group(4) and "== string" in m.group(4):
                params = [(n, "string" if t.startswith("$") else t) for n, t in params]
            self.procs[m.group(1)] = (params, m.group(3))
        for m in re.finditer(r"^(\w+) :: proc \{(.*?)\}", src, re.M | re.S):
            self.groups[m.group(1)] = [x.strip() for x in m.group(2).split(",") if x.strip()]

        def fields(body):
            out, uses = {}, []
            body = re.sub(r"//[^\n]*", "", body)
            for u in re.finditer(r"using \w+:\s*\^(\w+)_Trait\(", body):
                uses.append(u.group(1))
            for f in re.finditer(r"^\s*([\w, ]+?):\s*proc\((.*?)\)\s*->\s*(\w+)\s*,", body, re.M | re.S):
                params = split_params(f.group(2))
                for name in f.group(1).split(","):
                    out[name.strip()] = params[1:]            # without the receiver
            return out, uses
        for m in re.finditer(r"^(\w+)_Trait :: struct\(\$T: typeid\) \{(.*?)^\}", src, re.M | re.S):
            self.traits[m.group(1)] = fields(m.group(2))[0]
        for m in re.finditer(r"^(\w+)_VT :: struct \{(.*?)^\}", src, re.M | re.S):
            own, uses = fields(m.group(2))
            for u in uses:
                for k, v in self.traits.get(u, {}).items():
                    own.setdefault(k, v)
            self.vts[m.group(1)] = own
        for m in re.finditer(r"^(\w+) :: enum[^{\n]*\{(.*?)\}", src, re.M | re.S):
            body = re.sub(r"//[^\n]*", "", m.group(2))
            self.enums[m.group(1)] = [x.split("=")[0].strip() for x in body.split(",") if x.strip()]

    def type_of(self, path, unique=True):
        """The Odin type of an upstream path: the package's own word for it, else (a package
        without those comments) the upstream name, when only one element type has that name."""
        if path in self.types:
            return self.types[path]
        if self.types or not unique:
            raise NoSample("the package has no such type")
        return ada(tail(path))


# --- sample values ---------------------------------------------------------------------------

LENGTHS = {"length": "Length", "definite_length": "DefiniteLength", "absolute_length": "AbsoluteLength"}


def rust_arg(desc, api, text=None, ident="conf-a"):
    k = desc[0]
    if k == "leaf":
        if desc[1] == "id":
            return json.dumps(ident)
        if desc[1] == "px":
            return "gpui_kit::px(40.)"
        if desc[1] == "color":
            return "gpui_kit::hsla(0.6, 0.7, 0.5, 1.0)"
        if desc[1] == "duration":
            return "std::time::Duration::from_millis(3)"
        if desc[1] in LENGTHS:
            return f"gpui_kit::{LENGTHS[desc[1]]}::from(gpui_kit::px(40.))"
        raise NoSample(f"leaf {desc[1]}")
    if k == "bool":
        return "true"
    if k == "str" and desc[1] == "into":
        return json.dumps(text or "Sample")
    if k == "int":
        return "3"
    if k in ("f32", "f64"):
        return "40.0"
    if k == "child":
        # AnyElement, so it fits `impl IntoElement` and the few parameters typed AnyElement.
        return f'gpui_kit::div().child("{text or "Child"}").into_any_element()'
    if k == "enum":
        e = api["enums"].get(desc[1])
        units = [v for v in (e or {}).get("variants", []) if v.get("shape") == "unit" and v.get("ok", True)]
        if not units:
            raise NoSample("enum without a unit variant")
        return f"{e['rust']}::{units[1 if len(units) > 1 else 0]['name']}"
    raise NoSample(k if k != "str" else f"str {desc[1]}")


def enum_member(desc, api):
    e = api["enums"][desc[1]]
    units = [v for v in e["variants"] if v.get("shape") == "unit" and v.get("ok", True)]
    return units[1 if len(units) > 1 else 0]["name"]


INTS = {"int", "uint", "i32", "u32", "i64", "u64", "u16", "i16", "u8", "i8", "isize", "usize", "uintptr"}


def odin_arg(desc, typ, pkg, api, text=None, ident="conf-a"):
    k = desc[0]
    if k == "leaf" and desc[1] == "id" and typ in ("string", "Id"):
        return json.dumps(ident)
    if k == "leaf" and desc[1] == "px" and typ in ("f32", "Length", "Pixels"):
        return "40" if typ == "f32" else "gk.px(40)"
    if k == "leaf" and desc[1] in LENGTHS and typ == "Length":
        return "gk.px(40)"
    if k == "leaf" and desc[1] == "color" and typ in ("Color", "Fill"):
        return "gk.hsla(0.6, 0.7, 0.5, 1)"
    if k == "leaf" and desc[1] == "duration" and typ == "time.Duration":
        return "3 * time.Millisecond"
    if k == "bool" and typ == "bool":
        return "true"
    if k == "str" and typ == "string":
        return json.dumps(text or "Sample")
    if k == "int" and typ in INTS:
        return "3"
    if k in ("f32", "f64") and typ in ("f32", "f64"):
        return "40"
    if k == "child" and typ == "Element":
        return f'gk.div()->text("{text or "Child"}")'
    if k == "enum" and typ in pkg.enums:
        want = re.sub(r"_", "", enum_member(desc, api)).lower()
        for m in pkg.enums[typ]:
            if re.sub(r"_", "", m).lower() == want:
                return "." + m
        raise NoSample(f"enum {typ} has no member for {enum_member(desc, api)}")
    raise NoSample(f"a {k} argument against the Odin type {typ}")


# --- emit ------------------------------------------------------------------------------------

def main():
    argv = sys.argv[1:]
    opt = lambda name: argv[argv.index(name) + 1] if name in argv else None
    api = json.load(gzip.open(os.path.join(REPO, API)))
    items = {i["id"]: i for i in api["items"]}
    pkg = Package(os.path.join(REPO, opt("--package") or "odin/gk"))
    skip = {}
    if os.path.exists(os.path.join(REPO, SKIP)):
        for line in open(os.path.join(REPO, SKIP)):
            if line.strip() and not line.startswith("#"):
                pid, _, why = line.partition("|")
                skip[pid.strip()] = why.strip()

    # A type instantiated over a library type (`ResizeHandle<crate::values::Drag, ..>`) cannot be
    # named by the Rust reference, which links the kit only: its own pages are written by hand.
    elements = {p: n for p, n in api["nodes"].items()
                if n.get("element") and "crate::values::" not in p and "crate::rt::" not in p}
    last = {}
    for p in elements:
        last.setdefault(tail(p), []).append(p)

    def label(path):                      # Button, or base::Avatar when two types share a name
        name = tail(path)
        return name if len(last[name]) == 1 else f"{path.split('::')[1]}::{name}"

    def bound(ids):
        # A handed constructor makes only the element upstream hands an item procedure (PROTOCOLS.md 9.41).
        return [items[i] for i in ids if i in items and items[i]["status"] == "bound" and not i.endswith("::(handed)")]

    def rust_call(it, path, recv, text=None, ident="conf-a"):
        args = [rust_arg(a["desc"], api, text, ident) for a in it["args"]]
        callee = it["callee"].replace("<E as ", f"<{path} as ")
        return f"{callee}({', '.join(([recv] if recv else []) + args)})"

    def odin_ctor(it, path, ident):
        ot = pkg.type_of(path, "::" not in label(path) or "::component::" in path)
        if it.get("free"):
            name = odin_name(it["name"])
        else:
            name = odin_name(snake(ot) if it["name"] == "new" else f"{snake(ot)}_{it['name']}")
        if name in pkg.groups:       # a procedure group: the member that takes plain strings
            name = next((g for g in pkg.groups[name]
                         if g in pkg.procs and all(p[1] != "Temp_String" for p in pkg.procs[g][0])), name)
        if name not in pkg.procs or pkg.procs[name][1] != ot:
            raise NoSample(f"the package has no constructor {name}")
        params = pkg.procs[name][0]
        if len(params) != len(it["args"]):
            raise NoSample(f"{name}: the package takes {len(params)} arguments, the description {len(it['args'])}")
        return f"gk.{name}({', '.join(odin_arg(a['desc'], p[1], pkg, api, None, ident) for a, p in zip(it['args'], params))})"

    def odin_call(it, path, text=None):
        ot = pkg.type_of(path, "::" not in label(path) or "::component::" in path)
        fields = pkg.vts.get(ot, {})
        name = odin_name(it["name"])
        if name not in fields:
            raise NoSample(f"the package has no {ot}.{name}")
        params = fields[name]
        if len(params) != len(it["args"]):
            raise NoSample(f"{ot}.{name}: the package takes {len(params)} arguments, the description {len(it['args'])}")
        return f"->{name}({', '.join(odin_arg(a['desc'], p[1], pkg, api, text) for a, p in zip(it['args'], params))})"

    def samplable(it):
        try:
            for a in it["args"]:
                rust_arg(a["desc"], api)
            return True
        except NoSample:
            return False

    # Per element type: its default constructor and its dress.
    setup = {}
    for path, node in elements.items():
        ctors = [c for c in bound(node["ctors"]) if samplable(c) and c.get("kind") != "convert"]  # a convert needs its source
        default = (next((c for c in ctors if c["name"] in ("new", snake(tail(path)))), None)
                   or next((c for c in ctors if not c["args"]), None) or (ctors[0] if ctors else None))
        if default is None:
            continue
        dress = None
        want = DRESS.get(tail(path))
        if want:
            cands = bound(node["ops"]) + [o for t in node["tables"] for o in bound(api["traits"].get(t, {}).get("ops", []))]
            dress = next((o for o in cands if o["name"] == want[0] and samplable(o)), None)
        setup[path] = (default, dress, want[1] if want else None)

    def rank(p):                          # representatives of a trait: FIRST, then by path
        name = tail(p)
        plain = "::component::" in p or p.count("::") == 1
        return (FIRST.index(name) if name in FIRST and plain else len(FIRST), p)
    order = sorted(setup, key=rank)
    jobs = []          # (page id, element path, item)
    counts = {"functions": 0, "no constructor": 0, "skipped": 0}
    no_sample = {}
    seen_items = set()

    def consider(pid, path, it):
        if it["id"] in seen_items:
            return
        seen_items.add(it["id"])
        counts["functions"] += 1
        if path not in setup:
            counts["no constructor"] += 1
            return
        if it["kind"] == "convert":        # div().id(..): needs an element of the source type
            no_sample["type-changing method"] = no_sample.get("type-changing method", 0) + 1
            return
        try:
            for a in it["args"]:
                rust_arg(a["desc"], api)
        except NoSample as e:
            no_sample[str(e)] = no_sample.get(str(e), 0) + 1
            return
        if pid in skip:
            counts["skipped"] += 1
            return
        jobs.append((pid, path, it))

    for path in sorted(elements):
        node = elements[path]
        for it in bound(node["ctors"]):
            consider(f"{label(path)}.{it['name']}", path, it)
        for it in bound(node["ops"]):
            consider(f"{label(path)}.{it['name']}", path, it)
    for tpath in sorted(api["traits"]):
        rep = next((p for p in order if tpath in elements[p]["tables"]), None)
        tname = tpath.split(" where ")[0].rsplit("::", 1)[-1] + ("+" if " where " in tpath else "")
        for it in bound(api["traits"][tpath]["ops"]):
            # Not an element whose own method of that name shadows the trait's (Button::rounded
            # takes a ButtonRounded where Styled::rounded takes a length): the Odin call would
            # reach the element's own method.
            r = next((p for p in order if tpath in elements[p]["tables"] and setup[p][0].get("kind") != "convert"
                      and not any(items.get(o, {}).get("name") == it["name"] for o in elements[p]["ops"])), rep)
            consider(f"{tname}.{it['name']}", r or "-", it)

    rs, od, txt, why_not = [], [], [], {}
    used = set()
    for pid, path, it in jobs:
        if pid in used:
            continue
        used.add(pid)
        default, dress, dtext = setup[path]
        is_ctor = it["kind"] == "ctor"
        base = rust_call(default, path, None, None, "conf-a")
        if dress is not None:
            base = rust_call(dress, path, base, dtext)
        el = rust_call(it if is_ctor else default, path, None, None, "conf-b")
        if dress is not None and it is not dress:
            el = rust_call(dress, path, el, dtext)
        if not is_ctor:
            el = rust_call(it, path, el)
        rs.append(f'        "{pid}" => pair({base}, {el}),')
        txt.append(f"{pid} |  | {it['id']}")
        try:
            d = odin_call(dress, path, dtext) if dress is not None else ""
            obase = odin_ctor(default, path, "conf-a") + d
            oel = odin_ctor(it if is_ctor else default, path, "conf-b")
            if dress is not None and it is not dress:
                oel += d
            if not is_ctor:
                oel += odin_call(it, path)
            if "odin:" + pid in skip:
                raise NoSample("quarantined: " + skip["odin:" + pid])
            od.append((pid, "conf_" + re.sub(r"\W+", "_", pid).lower(), obase, oel))
        except NoSample as e:
            r = re.sub(r"constructor \w+|\w+\.\w+", "such procedure", str(e)) if "has no" in str(e) else re.sub(r"^\S+: ", "", str(e))
            why_not[r] = why_not.get(r, 0) + 1

    # The hand-written package takes pixels as f32, the generated one a Length.
    width = "240" if (pkg.vts.get("Div", {}).get("w") or [("", "f32")])[0][1] == "f32" else "gk.px(240)"
    head = f"Generated by gates/conformance.py from {API}. Do not edit: run that script."
    rust = f"""//! {head}
//! One page per bound function: the element as its constructor makes it, and with that one
//! function applied, calling GPUI-kit natively. The Odin twin is examples/odin/gallery/conf.odin.
//! One page per line: a compiler error's line names its page (line - {{FIRST_LINE}} into conf.txt).
#![allow(clippy::all, deprecated)]
#![cfg_attr(rustfmt, rustfmt_skip)]
use gpui_kit::component::h_flex;
use gpui_kit::{{AnyElement, IntoElement, ParentElement, Styled, div, px}};

fn pair(base: impl IntoElement, el: impl IntoElement) -> AnyElement {{
    h_flex()
        .gap_3()
        .child(div().w(px(240.)).child(base))
        .child(div().w(px(240.)).child(el))
        .into_any_element()
}}

/// The page of a conformance id, or None if the id is not one.
pub fn page(id: &str) -> Option<AnyElement> {{
    Some(match id {{
"""
    rust = rust.replace("{FIRST_LINE}", str(rust.count("\n") + 1)) + "\n".join(rs) + """
        _ => return None,
    })
}
"""
    odin = f"""// {head}
// One page per bound function the package declares today: the element as its constructor
// makes it, and with that one function applied. The Rust twin is ref/src/bin/gallery/conf.rs.
package gallery

import "core:time"
import gk "gpuikit:gk"

_ :: time

conf_pair :: proc(base, el: gk.Element) -> gk.Element {{
	return gk.h_flex(.Gap_3)->child(gk.div()->w({width})->child(base))->child(gk.div()->w({width})->child(el))
}}

CONF := [?]Page {{
{chr(10).join(f'	{{"{pid}", {fn}}},' for pid, fn, _, _ in od)}
}}
""" + "".join(f"""
{fn} :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {{
	base := {obase}
	el := {oel}
	return conf_pair(base, el)
}}
""" for pid, fn, obase, oel in od)
    text = (f"# {head}\n# Conformance pages, one per bound function, in the format of pages.txt; the third column is\n"
            f"# the description's id of the function. They follow the hand-written pages in the gallery's order.\n"
            + "\n".join(txt) + "\n")

    if opt("--odin-out"):
        open(opt("--odin-out"), "w").write(odin)
        print(f"conformance: {len(od)} of {len(used)} pages written for the package {opt('--package') or 'odin/gk'}")
        for r, n in sorted(why_not.items(), key=lambda kv: -kv[1])[:12]:
            print(f"  not in Odin, {n}: {r}")
        return 0
    files = {OUT_RS: rust, OUT_ODIN: odin, OUT_TXT: text}
    if "--check" in argv:
        # Fails only when the three committed files disagree with each other (a page in one
        # language and not the list, or a reference built from another list). Being behind
        # the description is said, not failed: the description moves many times a day and
        # catching up needs a rebuild of the reference apps (make lib).
        read = lambda p: open(os.path.join(REPO, p)).read() if os.path.exists(os.path.join(REPO, p)) else ""
        in_txt = [l.split("|")[0].strip() for l in read(OUT_TXT).splitlines() if l.strip() and not l.startswith("#")]
        in_rs = re.findall(r'^        "([^"]+)" => pair\(', read(OUT_RS), re.M)
        in_odin = re.findall(r'^\t\{"([^"]+)", conf_', read(OUT_ODIN), re.M)
        bad = []
        if set(in_txt) != set(in_rs):
            bad.append(f"{OUT_TXT} and {OUT_RS} name different pages")
        if set(in_odin) - set(in_txt):
            bad.append(f"{OUT_ODIN} has pages {OUT_TXT} does not")
        behind = [p for p, c in files.items() if read(p) != c]
        more = len(od) - len(in_odin)
        note = (f"; behind the description or the package in {' '.join(behind)}"
                f"{f' ({more} more pages available in Odin)' if more > 0 else ''}: run gates/conformance.py, "
                f"rebuild ref-gallery, gates/gallery.sh --record" if behind else "; current")
        print(f"GATE conformance generated {'FAIL' if bad else 'PASS'} {len(in_txt)} pages, {len(in_odin)} of them in Odin"
              + ("; " + "; ".join(bad) if bad else "") + note)
        return 1 if bad else 0
    for p, c in files.items():
        open(os.path.join(REPO, p), "w").write(c)
    print(f"conformance: {counts['functions']} bound constructors and builders of element types in {API}: "
          f"{len(used)} pages in Rust, {len(od)} of them also in Odin; without a page: "
          f"{counts['no constructor']} on types with no constructor this script can call, "
          f"{sum(no_sample.values())} with an argument it cannot make yet, {counts['skipped']} quarantined")
    for r, n in sorted(no_sample.items(), key=lambda kv: -kv[1]):
        print(f"  no sample, {n}: {r}")
    for r, n in sorted(why_not.items(), key=lambda kv: -kv[1])[:12]:
        print(f"  not in Odin, {n}: {r}")
    return 0


sys.exit(main())
