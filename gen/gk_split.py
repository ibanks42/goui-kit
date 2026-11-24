"""gk_split: the Go package split into its packages (docs/GO.md sections 1 and 2).

gk_go.Go writes one flat package, as it always did: every name in one namespace, the runtime's
own lower-case names. This module decides where each declaration lives and writes the packages:

  gpuikit.dev/gk               the core: gpui's elements and styles, the app, views, windows,
                               handles, events, the theme, and the kit's shared vocabulary
  gpuikit.dev/gk/motion        animations, transitions, springs
  gpuikit.dev/gk/table         tables, lists, trees, selects, row sources
  gpuikit.dev/gk/editor        inputs, the code editor, text views, highlighters, diffs
  gpuikit.dev/gk/base          the unstyled primitives of gpui_kit::base not in a package above
  gpuikit.dev/gk/kit           the kit's other components (buttons, labels, forms, menus, ...)
  gpuikit.dev/gk/chart         charts and plots
  gpuikit.dev/gk/overlay       dialogs, sheets, popovers, hover cards, tooltips, notifications
  gpuikit.dev/gk/dock          docks and panels
  gpuikit.dev/gk/internal/rt   the runtime every package calls into (not for apps)

The packages are in that order and a package imports only packages before it, so there is no
cycle. The rules (docs/GO.md section 2):

  1. Every item belongs to the package of its family (its upstream module: PACKAGE below; a family
     not listed goes to base or kit by its crate, gpui's own to gk).
  2. A type goes down: when a type of an earlier package names it (a field, a method's parameter or
     result), it lives in the earliest such package (the theme's settings types are in gk).
  3. A function goes up: a constructor or a free function lives in the latest package among its
     family's and those of the types it takes and returns.
  4. A method that makes an element of a later package (gpui's Div.ContextMenu makes the kit's
     ContextMenuDiv) becomes a function of that package named after the receiver and the method
     (kit.DivContextMenu(div, menu)).
  5. Names are made per package: a crate prefix or number that only told two items apart in the
     flat package goes when they now live apart, and a name that starts with its package's name
     loses it (dock.Area, table.State, motion.Value), unless what is left is El or taken.

Types the runtime names (PIN_RT) live in internal/rt and gk aliases them, and so do the interfaces
whose members are in several packages (AnimatableElement, SizableNode, ...): every member embeds an
rt base that carries the markers (internal/rt/gen_bases.go). Unexported helpers (a record's writer,
a trait's op) are copied into every package that calls one.

Then gen/gosplit (Go) rewrites every name of the written files to where it lives now.
"""
import json
import os
import re
import subprocess

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MODULE = "gpuikit.dev"
ORDER = ["rt", "gk", "motion", "table", "editor", "base", "kit", "chart", "overlay", "dock"]
RANK = {p: i for i, p in enumerate(ORDER)}
PUBLIC = [p for p in ORDER if p != "rt"]
PATHS = {p: MODULE + ("/gk" if p == "gk" else "/gk/internal/rt" if p == "rt" else "/gk/" + p) for p in ORDER}
PATHS.update({"s": MODULE + "/gk/s", "theme": MODULE + "/gk/theme", "palette": MODULE + "/gk/palette"})
PACKAGE = {
    "gk": ["core", "component/root", "component/theme", "base/theme", "base/theme_tokens", "base/styled",
           "base/state_style", "base/geometry", "base/layout_direction", "base/reduce_motion", "component/sizing",
           "base/root", "base/measure", "base/scrollbar", "component/icon", "component/element_ext",
           "component/inspector", "base/actions", "base/global_state"],
    "motion": ["base/motion", "base/animation", "base/scroll_bounce"],
    "table": ["base/table", "base/tree", "base/virtual_list", "base/list_settings", "base/index_path",
              "component/table", "component/list", "component/tree", "component/searchable_list", "component/select",
              "component/combobox", "base/select", "base/combobox", "component/command"],
    "editor": ["base/input", "component/input", "component/highlighter", "component/diff", "base/text",
               "component/text", "base/text_selection", "base/selectable_text", "base/otp_input", "base/number_input",
               "base/touch_selection", "component/native_menu"],
    "chart": ["base/plot", "component/plot", "component/chart"],
    "overlay": ["base/dialog", "base/alert_dialog", "base/popover", "base/popup", "base/positioner", "base/sheet",
                "base/hover_card", "base/tooltip", "base/toast", "component/dialog", "component/popover",
                "component/sheet", "component/hover_card", "component/tooltip", "component/notification",
                "component/window_ext"],
    "dock": ["base/dock", "component/dock"],
}
FAMILY_PKG = {f: p for p, fs in PACKAGE.items() for f in fs}
# Generated types the hand-written runtime itself names: they live in rt (and their package
# aliases them).
# Types of the kit's vocabulary that a package below their family's names: gk, not the lowest
# package that names them.
PIN_GK = {"ButtonVariant", "ButtonCustomVariant"}
PIN_RT = {"LinearColorStop", "Bounds", "GpuiPoint", "GpuiSize", "FocusHandle", "ScrollHandle", "Role", "FontStyle",
          "ImageFormat", "SyntaxContext", "FoldRange"}
# Marker interfaces whose members a package of any family may hold: kept in rt, each member
# embeds an rt base that carries the marker (an unexported method can only be declared in the
# interface's own package). Found by shape: an interface with one is<Name>() marker whose members
# live in more than one package, or one of these.
RT_MARKERS = {"ViewOrTooltip", "MarkdownPluginAny"}
OUT = os.path.join(REPO, "gen", "out", "build.go")
TOOL = os.path.join(REPO, "gen", "gosplit")


def pkg_of_family(f):
    if f in FAMILY_PKG:
        return FAMILY_PKG[f]
    if f.startswith("base/"):
        return "base"
    if f.startswith("component/"):
        return "kit"
    return "gk"


def camel_pkg(p):
    return p[:1].upper() + p[1:]


def strip(name, pkg):
    """Rule 5: a name starting with its package's name loses it (New + it too)."""
    if pkg in ("gk", "rt"):
        return name
    P = camel_pkg(pkg)
    for pre in ("New", ""):
        h = pre + P
        if name.startswith(h) and len(name) > len(h) and name[len(h)].isupper():
            rest = name[len(h):]
            if rest != "El" and not (pre and rest == "El"):
                return pre + rest
    return name


def gosplit(*args, stdin=None):
    env = dict(os.environ, GOFLAGS="", GOWORK="off", CGO_ENABLED="0")
    env.setdefault("GOPROXY", "off")
    r = subprocess.run(["go", "run", "."] + list(args), cwd=TOOL, env=env, capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit(f"gosplit {args[0]}: {r.stderr.strip()[:6000]}")
    return r.stdout


# ---------------------------------------------------------------- hand-written names


def hand_decls():
    """Every package's hand-written top-level names: pkg -> {name}. And the legacy spellings the
    flat package had (gen/gk_go_legacy.json): flat name -> (pkg, name)."""
    out = {}
    for p in ORDER:
        d = os.path.join(REPO, "go", PATHS[p][len(MODULE) + 1:])
        names = set()
        if os.path.isdir(d):
            for fn in sorted(os.listdir(d)):
                if not fn.endswith(".go") or fn.startswith("gen_"):
                    continue
                text = open(os.path.join(d, fn)).read()
                names.update(re.findall(r"(?m)^(?:func|type|var|const)\s+([A-Za-z_]\w*)", text))
                for block in re.findall(r"(?ms)^(?:const|var|type) \((.*?)^\)", text):
                    names.update(re.findall(r"(?m)^\t([A-Za-z_]\w*)", block))
        out[p] = names
    legacy = {}
    path = os.path.join(REPO, "gen", "gk_go_legacy.json")
    if os.path.exists(path):
        for p, m in json.load(open(path)).items():
            if p == "sels":
                continue
            for new, old in m.items():
                legacy[old] = (p, new)
    sels = json.load(open(path)).get("sels", {}) if os.path.exists(path) else {}
    return out, legacy, sels


# ---------------------------------------------------------------- the split


class Split:
    def __init__(self, go, fam, files):
        self.go, self.m, self.fam = go, go.m, fam
        self.files = files  # flat file name -> text (package gk, flat names)

    # ---- families of generated names

    def families(self):
        m, fam = self.m, self.fam
        F = {}

        def setf(name, f):
            if name and name not in F:
                F[name] = f
        for n in m.nodes.values():
            if "go" in n:
                f = fam.of_node(n)
                setf(n["go"], f)
                for it in n["ctors"]:
                    setf(it.get("go"), f)
        for s in m.states.values():
            if "go" in s:
                f = fam.of_type(s["crate"], s["name"], s["path"])
                setf(s["go"], f)
                setf(s["go"] + "Builder", f)
                for g in ("ctors", "globals", "statics"):
                    for it in s.get(g, []):
                        setf(it.get("go"), f)
        for tab in (m.enums, m.denums, m.records):
            for rt, t in tab.items():
                f = fam.of_type(t["crate"], rt.split("::")[-1], rt)
                for x in (t["go"], t.get("go_kind"), t.get("short_go"), "No" + t["short_go"] if t.get("short_go") else None):
                    setf(x, f)
                for v in t.get("variants", []):
                    setf(v.get("go"), f)
        for name, path in getattr(self.go, "handles", {}).items():  # a handle type: its entity's family
            if path and path.startswith("gpui_kit::"):
                crate = "gpui_base" if "::base::" in path else "gpui_component"
                setf(name, fam.of_type(crate, path.split("::")[-1], path))
            elif path and path.startswith("crate::"):  # the library's own (crate::dock::GkPanel)
                mod = path.split("::")[1]
                setf(name, next((f for f in (f"base/{mod}", f"component/{mod}") if f in FAMILY_PKG), None))
        import gk_app  # noqa: PLC0415
        for it in gk_app.bound(m):
            if "go" in it:
                setf(it["go"], fam.of_item(it))
        return F

    # ---- run

    def run(self):
        os.makedirs(OUT, exist_ok=True)
        flat = os.path.join(OUT, "flat")
        os.makedirs(flat, exist_ok=True)
        for f in os.listdir(flat):
            os.remove(os.path.join(flat, f))
        for name, text in self.files.items():
            open(os.path.join(flat, name), "w").write(text)
        decls = json.loads(gosplit("refs", *[os.path.join(flat, n) for n in sorted(self.files)]))
        self.decls = decls
        self.hand, self.legacy, self.sels = hand_decls()
        self.plan(decls)
        self.write()

    def plan(self, decls):
        F = self.families()
        texts = {n: t.encode() for n, t in self.files.items()}
        for d in decls:
            d["refs"] = d.get("refs") or []
            d["names"] = d.get("names") or []
            d["text"] = texts[d["file"]][d["start"]:d["end"]].decode()
            d["id"] = f"{d['file']}#{d['i']}"
        # every generated top-level name -> its declaration
        by_name = {}
        for d in decls:
            if d["kind"] != "method":
                for n in d["names"]:
                    by_name.setdefault(n, d)
        self.by_name = by_name
        gen_names = set(by_name)
        # local helpers: unexported funcs, copied into each package that calls one
        local = {n for n, d in by_name.items() if d["kind"] == "func" and n[:1].islower() and n != "init"}
        self.local = local
        # groups: a type with its methods; an enum's const block with its type; a marker method with
        # the interface it marks when that interface is local to the group (denum variants)
        parent = {}

        def find(x):
            while parent.get(x, x) != x:
                parent[x] = parent.get(parent[x], parent[x])
                x = parent[x]
            return x

        def union(a, b):
            ra, rb = find(a), find(b)
            if ra != rb:
                parent[rb] = ra
        types = {n for n, d in by_name.items() if d["kind"] == "type"}
        self.types = types
        ifaces = {}   # interface -> its members
        mark2 = {}    # marker method -> interface
        for n in types:
            t = by_name[n]["text"]
            mm = re.search(r"\binterface\s*\{[^{}]*?\b(is\w+)\(\)\s*\}", t)
            if mm:
                ifaces[n] = []
                mark2[mm.group(1)] = n
        markers = {}  # method declaration id -> (recv, interface)
        for d in decls:
            if d["kind"] == "method":
                i = mark2.get(d["names"][0])
                if i and re.search(r"\)\s*\{\s*\}\s*$", d["text"]):
                    ifaces[i].append(d["recv"])
                    markers[d["id"]] = (d["recv"], i)
        for i in RT_MARKERS:  # the runtime's own interfaces generated types implement
            if i not in ifaces:
                ifaces[i] = []
                mark2["is" + i] = i
        for d in decls:
            if d["kind"] == "method" and mark2.get(d["names"][0]) in RT_MARKERS and d["id"] not in markers:
                ifaces[mark2[d["names"][0]]].append(d["recv"])
                markers[d["id"]] = (d["recv"], mark2[d["names"][0]])
        self.mark2 = mark2
        # a denum's variants (and its kind and held types) go with it: members of the same family
        for i, mem in ifaces.items():
            fi = F.get(i)
            for t in mem:
                if t in types and t.startswith(i) and (F.get(t) == fi or t not in F):
                    union(i, t)
        for d in decls:
            if d["kind"] in ("const", "var"):
                ts = [r for r in d["refs"] if r in types]
                if len(ts) == 1 and all(r in types or r in local or r[:1].islower() for r in d["refs"]):
                    for n in d["names"]:
                        union(ts[0], n)
        self.find = find
        # initial homes
        home = {}
        for n in types:
            g = find(n)
            if g in home:
                continue
            f = F.get(g) or F.get(n)
            home[g] = "rt" if (g in PIN_RT or n in PIN_RT) else "gk" if (g in PIN_GK or n in PIN_GK) else (pkg_of_family(f) if f else None)
        # members of a group without a family use their group's
        rank = lambda p: RANK[p] if p else 99  # noqa: E731
        # references of each type group: its declarations' and its methods'
        grefs = {}
        lift = set()
        node_types = {n for n in types if re.search(r"struct\{\s*n \*node\s*\}", by_name[n]["text"])}
        self.node_types = node_types
        for d in decls:
            if d["kind"] == "type" or (d["kind"] in ("const", "var") and find(d["names"][0]) in home and d["names"][0] not in types and find(d["names"][0]) != d["names"][0]):
                g = find(d["names"][0])
            elif d["kind"] == "method" and d["recv"] in types:
                g = find(d["recv"])
                if d["id"] in markers:
                    continue
                # rule 4: a method making a node of a later family's package
                res = re.search(r"(?m)^func \(\w+ \w+\) \w+\([^)]*\) (\w+) \{", d["text"])
                if res and res.group(1) in node_types and res.group(1) != d["recv"]:
                    fr, fu = F.get(d["recv"]), F.get(res.group(1))
                    if fr and fu and rank(pkg_of_family(fu)) > rank(pkg_of_family(fr)):
                        lift.add(d["id"])
                        continue
            else:
                continue
            grefs.setdefault(g, set()).update(find(r) for r in d["refs"] if r in types)
        # a type's writer and reader name what it is made of (an Into set's members): with it
        for d in decls:
            if d["kind"] == "func" and d["names"] and d["names"][0] in local:
                mm = re.match(r"(?:put|rd)(\w+)$", d["names"][0])
                if mm and mm.group(1) in types:
                    grefs.setdefault(find(mm.group(1)), set()).update(find(r) for r in d["refs"] if r in types)
        self.lift = lift
        self.lifted_map = {}  # (receiver, method) -> the function it became (rule 4)
        # a type of no family (an Into set, a tuple, an event's payload) starts in the latest package
        # of what it is made of, then goes down like any other
        flex = [g for g in grefs if home.get(g) is None]
        for _ in range(8):
            for g in flex:
                if home.get(g) is None and all(home.get(u) for u in grefs[g] if u != g):
                    hs = [home[u] for u in grefs[g] if u != g]
                    home[g] = max(hs, key=rank) if hs else "gk"
        # rule 2: push down to a fixpoint
        self.pushed = []
        changed = True
        while changed:
            changed = False
            for g, rs in grefs.items():
                h = home.get(g)
                if h is None:
                    continue
                for u in rs:
                    if u != g and rank(home.get(u)) > rank(h):
                        if home.get(u) is not None:
                            self.pushed.append((u, home[u], h, g))
                        home[u] = h
                        changed = True
            if not changed:
                # groups no type names yet: the latest package of what they name
                for g, rs in grefs.items():
                    if home.get(g) is None:
                        hs = [home[u] for u in rs if u != g and home.get(u)]
                        if all(home.get(u) for u in rs if u != g):
                            home[g] = max(hs, key=rank) if hs else "gk"
                            changed = True
        for g in list(home):
            if home[g] is None:
                home[g] = "gk"
        for n in types:
            home.setdefault(find(n), "gk")
        # marker interfaces: local, or in rt with bases carrying them
        rt_marks = {}
        hof = lambda t: home[find(t)] if t in types else "rt"  # noqa: E731 (a runtime type: rt)
        for i, mem in ifaces.items():
            hs = {hof(t) for t in mem} | {home[find(i)] if i in types else "rt"}
            if i in RT_MARKERS or len(hs) > 1 or "rt" in hs:
                if i in types:
                    home[find(i)] = "rt"
                for t in mem:
                    if t in types and t not in node_types:
                        home[find(t)] = "rt"  # a kind type the interface takes: with it
                rt_marks[i] = [t for t in mem if t in node_types]
            elif mem and home[find(i)] != hof(mem[0]):
                home[find(i)] = hof(mem[0])
        self.rt_marks = rt_marks
        with open(os.path.join(OUT, "pushed.txt"), "w") as f:
            for u, a, b, g in self.pushed:
                f.write(f"{u}: {a} -> {b} (named by {g})\n")
        # each node's base: its rt markers
        self.marks_of = {}
        for i, mem in rt_marks.items():
            for t in mem:
                self.marks_of.setdefault(t, set()).add(i)
        # the home of every declaration
        dhome = {}
        self.dropped = set()
        for d in decls:
            if d["kind"] == "type":
                dhome[d["id"]] = home[find(d["names"][0])]
            elif d["kind"] == "method":
                if d["id"] in self.lift:
                    pass  # rule 4: a function now, placed below
                elif d["recv"] in types:
                    dhome[d["id"]] = home[find(d["recv"])]
                elif d["recv"] in self.hand.get("rt", ()):
                    dhome[d["id"]] = "rt"  # a method of a runtime type (RowSource): rt
                else:
                    self.dropped.add(d["id"])  # a marker of a hand-written type: its rt base has it
            elif d["kind"] in ("const", "var") and find(d["names"][0]) in home and find(d["names"][0]) != d["names"][0]:
                dhome[d["id"]] = home[find(d["names"][0])]
        # rule 3: functions (and the rest) go up
        def fhome(d):
            hs = [home[find(r)] for r in d["refs"] if r in types]
            hs += [dhome_of(r) for r in d["refs"] if r in by_name and r not in types and r not in local and by_name[r]["id"] in dhome]
            f = None
            for n in d["names"]:
                if n in F:
                    f = pkg_of_family(F[n])
            if d["id"] in lift:
                hs.append(home[find(d["recv"])])
            if f:
                hs.append(f)
            return max(hs, key=rank) if hs else "gk"

        def dhome_of(name):
            return dhome[by_name[name]["id"]]
        pending = [d for d in decls if d["id"] not in dhome and d["id"] not in self.dropped]
        for _ in range(4):  # functions naming functions (a shorthand's var): settle in a few rounds
            for d in pending:
                if d["names"] and d["names"][0] in local:
                    continue
                dhome[d["id"]] = fhome(d)
        for d in pending:
            if d["names"] and d["names"][0] in local:
                dhome[d["id"]] = None  # copied where used
        # a function that would land in rt (it names only runtime types) lives in gk, unless it is
        # an init or the declaration of an rt-pinned type's helper
        for d in decls:
            if dhome.get(d["id"]) == "rt" and d["kind"] in ("func",) and d["names"][0] != "init":
                dhome[d["id"]] = "gk"
        self.dhome, self.home = dhome, home
        with open(os.path.join(OUT, "moved.txt"), "w") as f:  # what rules 2 and 3 moved, for review
            for d in decls:
                for n in d["names"][:1]:
                    if n in F and n[:1].isupper() and d["kind"] != "method" and dhome.get(d["id"]) not in (None, pkg_of_family(F[n])):
                        f.write(f"{d['kind']} {n}: {pkg_of_family(F[n])} -> {dhome[d['id']]}\n")
        # ---- names
        self.newname = {}
        self.name_all(F)

    def name_all(self, F):
        go = self.go
        claimed = {p: set(self.hand.get(p, ())) for p in ORDER}
        bases = getattr(go, "claim_base", {})  # flat name -> (base, crate): gk_go.Go.claim's
        items = []
        for n, d in self.by_name.items():
            p = self.dhome.get(d["id"])
            if n[:1].isupper() and p:
                items.append((n, p))
        order = {"gpui_component": 0, "gpui_base": 1, "gpui": 2, "gpui_kit": 3}
        items.sort(key=lambda np: (order.get(bases.get(np[0], (None, None))[1], 9), np[0]))
        # a flat name is its item's last resort: reserved for it
        for n, p in items:
            if n in claimed[p]:
                raise SystemExit(f"gk_split: {p}.{n} is declared by hand and generated")
        reserved = {p: {n for n, q in items if q == p or (p == "gk" and q == "rt")} for p in ORDER}
        for n, p in items:
            if p == "rt":
                p = "gk"  # declared in rt by its flat name, aliased in gk by its own
            b = bases.get(n, (n, None))[0]
            got = n
            for c in (strip(b, p), b, strip(n, p)):
                if c != n and c not in claimed[p] and c not in reserved[p]:
                    got = c
                    break
            claimed[p].add(got)
            self.newname[n] = (p, got)

    # ---- writing

    def write(self):
        decls, dhome = self.decls, self.dhome
        self._methods_text = {}
        for d in decls:
            if d["kind"] == "method":
                self._methods_text[d["recv"]] = self._methods_text.get(d["recv"], "") + d["text"] + "\n"
        # per package: the declarations, by flat file
        per = {}
        need_local = {}
        for d in decls:
            p = dhome.get(d["id"])
            if p is None:
                continue
            per.setdefault(p, {}).setdefault(d["file"], []).append(d)
        # local helpers: every package that names one (transitively) gets a copy
        refs_of = {}
        for d in decls:
            if d["names"] and d["names"][0] in self.local:
                refs_of[d["names"][0]] = d
        # the hand-written code of a package may call a local helper too (a trait's op, a reader)
        hand_refs = {}
        for p in ORDER:
            d = os.path.join(REPO, "go", PATHS[p][len(MODULE) + 1:])
            if os.path.isdir(d):
                text = "\n".join(open(os.path.join(d, f)).read() for f in sorted(os.listdir(d))
                                 if f.endswith(".go") and not f.startswith("gen_"))
                hand_refs[p] = {w for w in re.findall(r"\b([a-z]\w*)\(", text) if w in self.local}
                if hand_refs[p]:
                    per.setdefault(p, {})
        for p, files in per.items():
            want = set()
            stack = [r for ds in files.values() for d in ds for r in d["refs"] if r in self.local] + sorted(hand_refs.get(p, ()))
            while stack:
                r = stack.pop()
                if r in want:
                    continue
                want.add(r)
                stack += [x for x in refs_of[r]["refs"] if x in self.local]
            for r in sorted(want):
                d = refs_of[r]
                files.setdefault(d["file"], []).append(d)
        bases = {}  # base name -> (kind, markers)
        out_files = {}
        for p, files in per.items():
            for fname, ds in files.items():
                ds.sort(key=lambda d: (d["file"], d["i"]))
                body = []
                for d in ds:
                    t = d["text"]
                    if d["kind"] == "method" and d["recv"] in self.node_types | self.handle_types():
                        nm = d["names"][0]
                        if nm in ("node", "valueNode", "handle"):
                            continue
                        i = self.mark2.get(nm)
                        if i in self.rt_marks and d["recv"] in self.rt_marks[i]:
                            continue
                    if d["id"] in self.lift:
                        t = self.lifted(d)
                    body.append(t)
                text = "\n\n".join(body)
                text = self.fix_bases(text, bases)
                imports = []
                for d in self.decls:
                    if d["file"] == fname and d.get("imports"):
                        imports = d["imports"]
                out_files[(p, fname)] = (text, imports)
        # what generated code put in rt, aliased in gk (an app names gk's)
        alias = ["// Code generated by gen/gkgen.py (gen/gk_split.py). DO NOT EDIT.", "",
                 "// The generated names internal/rt holds (the runtime names them, or members of every package",
                 "// implement them): gk names them for apps.", "", "package gk", ""]
        self.rt_alias = set()
        for d in sorted(decls, key=lambda d: d["names"][0] if d["names"] else ""):
            if dhome.get(d["id"]) == "rt" and d["kind"] in ("type", "const") and d["names"] and d["names"][0][:1].isupper():
                for n in d["names"]:
                    if n[:1].isupper():
                        alias.append(f"{d['kind']} {self.newname[n][1]} = rt.{n}")
                        self.rt_alias.add(n)
        out_files[("gk", "gen_rt.go")] = ("\n".join(alias[6:]), [])
        self.bases = bases
        # write
        written = []
        dirs = {p: os.path.join(REPO, "go", PATHS[p][len(MODULE) + 1:]) for p in ORDER}
        keep = {}
        for (p, fname), (text, imports) in sorted(out_files.items()):
            name = fname
            path = os.path.join(dirs[p], name)
            head = ("// Code generated by gen/gkgen.py (gen/gk_go.py, gen/gk_split.py). DO NOT EDIT.\n\n"
                    f"package {p}\n\n")
            if name == "gen_rt.go":
                head = "\n".join(alias[:6]) + "\n"
            imp = ("import (\n" + "".join(f"\t{i}\n" for i in imports) + ")\n\n") if imports else ""
            os.makedirs(dirs[p], exist_ok=True)
            open(path, "w").write(head + imp + text + "\n")
            written.append(path)
            keep.setdefault(p, set()).add(name)
        # rt: the bases and the interfaces
        lines = ["// Code generated by gen/gkgen.py (gen/gk_split.py). DO NOT EDIT.", "",
                 "// The bases of the generated node types: the element or value core, and the markers of the",
                 "// interfaces whose members live in more than one package (docs/GO.md section 1).", "",
                 "package rt", ""]
        for b, (kind, ms) in sorted(bases.items()):
            core = "ElCore" if kind == "El" else "ValCore"
            lines += [f"// {b} is the base of a generated {'element' if kind == 'El' else 'value'} type" + (f": {', '.join(ms)}." if ms else "."),
                      f"type {b} struct{{ {core} }}", "",
                      f"func Mk{b}(n *Node) {b} {{ return {b}{{{core}{{Core{{n}}}}}} }}", ""]
            marker = {i: m for m, i in self.mark2.items()}
            for mk in ms:
                lines.append(f"func ({b}) {marker[mk]}() {{}}")
            lines.append("")
        path = os.path.join(dirs["rt"], "gen_bases.go")
        open(path, "w").write("\n".join(lines) + "\n")
        written.append(path)
        keep.setdefault("rt", set()).add("gen_bases.go")
        for p, d in dirs.items():
            if not os.path.isdir(d):
                continue
            for f in os.listdir(d):
                if f.startswith("gen_") and f.endswith(".go") and f not in keep.get(p, set()) and f not in ("gen_ops.go", "gen_use.go", "gen_use_dyn.go"):
                    os.remove(os.path.join(d, f))
        self.written = written
        self.write_table()
        gosplit("apply", os.path.join(OUT, "table.json"), *written)
        self.fix_messages(written)

    def fix_messages(self, paths):
        """A message (a Go string) that names a gk item names it by the flat spelling the generator
        writes (gk.TableSelection); the split moved it (table.Selection). Each is renamed to the
        package-qualified name an app calls."""
        names = self.table["names"]

        def fix_str(mt):
            def one(m):
                p, n = names.get(m.group(1), ("gk", m.group(1)))
                return ("gk" if p in ("gk", "rt") else p) + "." + n
            return re.sub(r"\bgk\.([A-Z]\w*)", one, mt.group(0))
        for path in paths:
            text = open(path).read()
            out = re.sub(r'"(?:[^"\\\n]|\\.)*"', fix_str, text)
            if out != text:
                open(path, "w").write(out)
        check_messages()


    def handle_types(self):
        if not hasattr(self, "_handles"):
            self._handles = {n for n in self.types if re.search(r"struct\{\s*h uint64\s*\}", self.by_name[n]["text"])}
        return self._handles

    def lifted(self, d):
        """Rule 4: func (e Recv) Name(args) Ret { -> func RecvStemName(e Recv, args) Ret {"""
        lines = d["text"].split("\n")
        k = next(i for i, l in enumerate(lines) if l.startswith("func "))
        mm = re.match(r"func \((\w+) (\w+)\) (\w+)\((.*?)\) (\w+) \{$", lines[k])
        recv_var, recv, name, args, ret = mm.groups()
        stem = recv[:-2] if recv.endswith("El") else recv
        fname = stem + name
        lines[k] = f"func {fname}({recv_var} {recv}{', ' + args if args else ''}) {ret} {{"
        lines = [re.sub(r"^// " + name + r"\b", "// " + fname, l) for l in lines]
        d["lifted_name"] = fname
        self.lifted_map[(recv, name)] = fname
        return "\n".join(lines)

    def fix_bases(self, text, bases):
        def tbase(t):
            ms = sorted(self.marks_of.get(t, ()))
            kind = "El" if f"func (e {t}) node()" in self._methods_text.get(t, "") or t.endswith("El") and f"func (e {t}) valueNode()" not in self._methods_text.get(t, "") else "Val"
            b = kind + "".join(ms)
            bases[b] = (kind, ms)
            return b
        text = re.sub(r"(?m)^type (\w+) struct\{ n \*node \}", lambda mm: f"type {mm.group(1)} struct{{ rt.{tbase(mm.group(1))} }}", text)
        text = re.sub(r"(?m)^type (\w+) struct\{ h uint64 \}", lambda mm: f"type {mm.group(1)} struct{{ rt.HCore }}", text)
        text = re.sub(r"\b(\w+)\{n\}", lambda mm: f"{mm.group(1)}{{{tbase(mm.group(1))}: rt.Mk{tbase(mm.group(1))}(n)}}" if mm.group(1) in self.node_types else mm.group(0), text)
        return text

    # ---- the table gosplit resolves names with

    def write_table(self):
        names = {}
        # hand-written names, by their flat spelling
        for p in ORDER:
            for n in self.hand.get(p, ()):
                if n[:1].isupper() or p != "rt":
                    if p == "rt" and n in names:
                        continue
                    names.setdefault(n, (p, n))
        for old, (p, new) in self.legacy.items():
            names[old] = (p, new)
        # public facade names win over rt's own for a name both declare
        for n in self.hand.get("gk", ()):
            if n[:1].isupper():
                names[n] = ("gk", n)
        for p in PUBLIC:
            for n in self.hand.get(p, ()):
                if n[:1].isupper() and p != "gk":
                    names.setdefault(n, (p, n))
        for n, v in self.newname.items():
            names[n] = v
        for n, u in getattr(self, "uses", {}).items():
            names[n] = ("rt", u)
        for d in self.decls:
            if d.get("lifted_name"):
                names[d["lifted_name"]] = (self.dhome[d["id"]], d["lifted_name"])
        # unexported generated names that live in one package
        for n, d in self.by_name.items():
            if n[:1].islower() and n not in self.local and self.dhome.get(d["id"]):
                names[n] = (self.dhome[d["id"]], n)
        rtn = set(self.hand.get("rt", ()))
        rtn |= {n for n, d in self.by_name.items() if self.dhome.get(d["id"]) == "rt"}
        rtn |= set(getattr(self, "uses", {}).values())
        rtn |= set(self.bases) | {"Mk" + b for b in self.bases}
        t = {"rt": sorted(rtn), "module": MODULE, "pkgs": {p: PATHS[p] for p in ORDER}, "names": names,
             "local": sorted(self.local), "sels": self.sels, "public": PUBLIC}
        json.dump(t, open(os.path.join(OUT, "table.json"), "w"), indent=0, sort_keys=True)
        self.table = t


def check_messages():
    """Every message names a function or type the way an app calls it: a gk.Name in a Go string of
    the packages or in a message of the library (core/src) must be one of package gk's names. One
    that moved to another package by the split (gk.NewInputState, now editor.NewInputState) fails
    make gen, with the file and line."""
    gkdir = os.path.join(REPO, "go", "gk")
    have = set()
    for f in os.listdir(gkdir):
        if not f.endswith(".go") or f.endswith("_test.go"):
            continue
        text = open(os.path.join(gkdir, f)).read()
        have |= set(re.findall(r"^(?:func|type|const|var) ([A-Z]\w*)", text, re.M))
        for block in re.findall(r"^(?:const|var|type) \((.*?)^\)", text, re.M | re.S):
            have |= set(re.findall(r"^\t([A-Z]\w*)", block, re.M))
    bad = []
    roots = [(gkdir, ".go"), (os.path.join(REPO, "core", "src"), ".rs")]
    for root, ext in roots:
        for d, _, files in os.walk(root):
            for f in sorted(files):
                if not f.endswith(ext) or f.endswith("_test.go"):
                    continue
                path = os.path.join(d, f)
                for n, line in enumerate(open(path), 1):
                    for lit in re.findall(r'"(?:[^"\\\n]|\\.)*"', line):
                        for name in re.findall(r"\bgk\.([A-Z]\w*)", lit):
                            if name not in have:
                                bad.append(f"{os.path.relpath(path, REPO)}:{n}: gk.{name}")
    if bad:
        raise SystemExit("a message names what package gk does not have (name it by its package, as an app calls it):\n  " + "\n  ".join(bad))
