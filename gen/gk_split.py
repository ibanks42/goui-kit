"""Partition the flat generated Go package into layered subpackages under go/gk/.

The Go emitter (`gen/gk_go.py`) produces a single flat package in memory where every
type, constructor, method, and helper sits side by side without package qualifiers.
This module breaks that flat output into an acyclic hierarchy of public subpackages
plus `internal/rt`, so apps can import only the domains they need without pulling
every symbol into the root `gk` namespace.

Pipeline stages:
1. Write flat files into `gen/out/build.go/flat/` and invoke `gen/gosplit refs`
   to extract each top-level declaration's byte span, kind, receiver, declared
   names, and referenced identifiers via `go/ast`.
2. Map each symbol to an upstream family (`base/<mod>` or `component/<mod>`),
   then map families to packages in `ORDER`.
3. Resolve package placement (`Split.plan`):
   - Group tightly coupled declarations via union-find (interface unions with
     same-prefix variants; single-type `const`/`var` declarations with their type)
     so they always move together.
   - Lift cross-package conversion methods whose return node lives in a higher
     package rank than their receiver into top-level functions in the return
     node's package (`<RecvStem><Method>`).
   - Iteratively push referenced types downward in `ORDER` whenever a lower-ranked
     package references a higher-ranked type, guaranteeing an acyclic import graph.
   - Move cross-package marker interfaces into `internal/rt` and attach their
     marker methods to shared embedded `rt.<Kind><Markers>` base structs.
4. Compute collision-free shortened names (`Split.name_all`) by stripping redundant
   package prefixes (`dock.DockArea` -> `dock.Area`) and honoring hand-written
   declarations and `gen/gk_go_legacy.json`.
5. Emit partitioned files, `go/gk/gen_rt.go` (public aliases for `rt` types and
   constants), `go/gk/internal/rt/gen_bases.go` (embedded base structs and `Mk*`
   constructors), and `gen/out/build.go/table.json`, then run `gen/gosplit apply`
   to rewrite every AST identifier, selector, and import block.
6. Rewrite `"gk.<Name>"` references inside generated string literals and verify
   via `check_messages()` that no Go or Rust message cites a symbol that moved out
   of package `gk`.

Data structures passed between stages:
- `decls`: list of dicts from `gosplit refs` (`file`, `i`, `kind`, `recv`,
  `names`, `refs`, `start`, `end`, `imports`), augmented with `text` and `id`.
- `home`: union-find root symbol name -> target package in `ORDER`.
- `dhome`: declaration `id` -> target package (`None` for local helpers copied
  into each package that calls them, or dropped declarations).
- `table.json`: symbol routing map (`names`, `rt`, `pkgs`, `local`, `sels`,
  `public`) consumed by `gen/gosplit apply`.
"""
import json
import os
import re
import subprocess

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MODULE = "github.com/ibanks42/goui-kit"
# Strict acyclic import order: any package at index i may only import packages at index < i.
# `rt` sits at rank 0 (internal/rt); `gk` is the public root; domain packages layer above it
# so higher-level shells (overlay, dock) can reference lower-level widgets (editor, table, kit).
ORDER = ["rt", "gk", "motion", "table", "editor", "base", "kit", "chart", "overlay", "dock"]
RANK = {p: i for i, p in enumerate(ORDER)}
PUBLIC = [p for p in ORDER if p != "rt"]
PATHS = {p: MODULE + ("/gk" if p == "gk" else "/gk/internal/rt" if p == "rt" else "/gk/" + p) for p in ORDER}
PATHS.update({"s": MODULE + "/gk/s", "theme": MODULE + "/gk/theme", "palette": MODULE + "/gk/palette"})
# Explicit mapping from upstream module families to target Go packages.
# Families not listed here fall back in pkg_of_family(): base/* -> base, component/* -> kit, else -> gk.
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
# Types pinned to package `gk` or `internal/rt` regardless of their upstream module family.
# PIN_GK keeps widely used button styling enums in root `gk` so callers avoid extra imports.
# PIN_RT holds core wire/geometry/handle types shared by hand-written runtime code in internal/rt
# and re-exported into root `gk` via gen_rt.go.
PIN_GK = {"ButtonVariant", "ButtonCustomVariant"}
PIN_RT = {"LinearColorStop", "Bounds", "GpuiPoint", "GpuiSize", "FocusHandle", "ScrollHandle", "Role", "FontStyle",
          "ImageFormat", "SyntaxContext", "FoldRange"}
# Hand-written rt marker interfaces whose cross-package implementors cannot declare an unexported
# marker method outside package rt; instead, their generated structs embed an rt base struct that
# carries the marker method.
RT_MARKERS = {"ViewOrTooltip", "MarkdownPluginAny"}
OUT = os.path.join(REPO, "gen", "out", "build.go")
TOOL = os.path.join(REPO, "gen", "gosplit")


def pkg_of_family(f):
    """Map an upstream family string (`base/<mod>` or `component/<mod>`) to its target Go package.

    Checks the explicit `FAMILY_PKG` table first, then falls back to `base` for
    unlisted `base/*` families, `kit` for unlisted `component/*` families, and
    `gk` for core or uncategorized items.
    """
    if f in FAMILY_PKG:
        return FAMILY_PKG[f]
    if f.startswith("base/"):
        return "base"
    if f.startswith("component/"):
        return "kit"
    return "gk"


def camel_pkg(p):
    """Capitalize a Go package name for prefix-stripping checks."""
    return p[:1].upper() + p[1:]


def strip(name, pkg):
    """Remove a redundant package prefix from `name` (for example `DockArea` -> `Area` in package `dock`).

    Leaves `gk` and `rt` untouched. For other packages, strips `<Pkg>` or `New<Pkg>`
    when followed by an uppercase letter, except when the remainder would collapse to
    the generic suffix `"El"` (so `TableEl` does not become `table.El`).
    """
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
    """Run the gen/gosplit Go AST tool with the given arguments and return stdout.

    Forces `CGO_ENABLED=0`, `GOWORK=off`, and `GOPROXY=off` so running the helper
    from inside `make gen` never triggers network access or cgo compilation.
    """
    env = dict(os.environ, GOFLAGS="", GOWORK="off", CGO_ENABLED="0")
    env.setdefault("GOPROXY", "off")
    r = subprocess.run(["go", "run", "."] + list(args), cwd=TOOL, env=env, capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit(f"gosplit {args[0]}: {r.stderr.strip()[:6000]}")
    return r.stdout


def hand_decls():
    """Scan all hand-written Go files across `ORDER` for declared identifiers, plus legacy rename maps.

    Returns `(hand_by_pkg, legacy_map, selector_rewrites)`:
    - `hand_by_pkg`: `{pkg: set(ident)}` used to detect generated/hand collisions and to
      resolve references to hand-written symbols when building `table.json`.
    - `legacy_map`: `{old_flat_name: (pkg, new_name)}` loaded from `gen/gk_go_legacy.json`
      if present so historical renames stay routed during migration.
    - `selector_rewrites`: `{old_sel: new_sel}` passed through to `gosplit apply`.
    """
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


class Split:
    """Plans and writes the split Go subpackages from gk_go's flat package output."""

    def __init__(self, go, fam, files):
        self.go, self.m, self.fam = go, go.m, fam
        self.files = files


    def families(self):
        """Map every generated flat Go symbol name to its upstream component family string."""
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
        for name, path in getattr(self.go, "handles", {}).items():
            if path and path.startswith("gpui_kit::"):
                crate = "gpui_base" if "::base::" in path else "gpui_component"
                setf(name, fam.of_type(crate, path.split("::")[-1], path))
            elif path and path.startswith("crate::"):
                mod = path.split("::")[1]
                setf(name, next((f for f in (f"base/{mod}", f"component/{mod}") if f in FAMILY_PKG), None))
        import gk_app
        for it in gk_app.bound(m):
            if "go" in it:
                setf(it["go"], fam.of_item(it))
        return F


    def run(self):
        """Write flat files, extract AST declarations via `gosplit refs`, plan package layout, and emit split files."""
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
        """Assign each declaration to a package without dependency cycles and compute renamed symbols.

        Groups coupled types (unions with their variants, enums with their constants) via
        union-find, pushes referenced types down in `ORDER` whenever a lower-ranked package
        depends on a higher-ranked one, lifts cross-package element conversions to top-level
        functions in the target package, and records `rt` marker interfaces shared across
        multiple packages.
        """
        F = self.families()
        texts = {n: t.encode() for n, t in self.files.items()}
        for d in decls:
            d["refs"] = d.get("refs") or []
            d["names"] = d.get("names") or []
            d["text"] = texts[d["file"]][d["start"]:d["end"]].decode()
            d["id"] = f"{d['file']}#{d['i']}"
        by_name = {}
        for d in decls:
            if d["kind"] != "method":
                for n in d["names"]:
                    by_name.setdefault(n, d)
        self.by_name = by_name
        gen_names = set(by_name)
        # Unexported generated functions (codec helpers put*/rd*/t* and similar) are not assigned a
        # single owning package; instead they are duplicated into every package whose generated or
        # hand-written code calls them, avoiding cross-package leakage of internal wire details.
        local = {n for n, d in by_name.items() if d["kind"] == "func" and n[:1].islower() and n != "init"}
        self.local = local
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
        # Discover sealed sum-type interfaces (`interface { is<Name>() }`) and record every type
        # implementing their empty marker method. Also seed RT_MARKERS (hand-written in internal/rt).
        ifaces = {}
        mark2 = {}
        for n in types:
            t = by_name[n]["text"]
            mm = re.search(r"\binterface\s*\{[^{}]*?\b(is\w+)\(\)\s*\}", t)
            if mm:
                ifaces[n] = []
                mark2[mm.group(1)] = n
        markers = {}
        for d in decls:
            if d["kind"] == "method":
                i = mark2.get(d["names"][0])
                if i and re.search(r"\)\s*\{\s*\}\s*$", d["text"]):
                    ifaces[i].append(d["recv"])
                    markers[d["id"]] = (d["recv"], i)
        for i in RT_MARKERS:
            if i not in ifaces:
                ifaces[i] = []
                mark2["is" + i] = i
        for d in decls:
            if d["kind"] == "method" and mark2.get(d["names"][0]) in RT_MARKERS and d["id"] not in markers:
                ifaces[mark2[d["names"][0]]].append(d["recv"])
                markers[d["id"]] = (d["recv"], mark2[d["names"][0]])
        self.mark2 = mark2
        # Co-locate a union interface with its dedicated variant structs (`<Union><Variant>`) when
        # they share a family, and co-locate single-type const/var blocks with their enum/value type.
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
        # Seed each union-find group's initial home package from PIN_RT, PIN_GK, or its upstream family.
        # Groups with no known family stay None (flexible) until derived from their type references below.
        home = {}
        for n in types:
            g = find(n)
            if g in home:
                continue
            f = F.get(g) or F.get(n)
            home[g] = "rt" if (g in PIN_RT or n in PIN_RT) else "gk" if (g in PIN_GK or n in PIN_GK) else (pkg_of_family(f) if f else None)
        rank = lambda p: RANK[p] if p else 99
        grefs = {}
        lift = set()
        node_types = {n for n in types if re.search(r"struct\{\s*n \*node\s*\}", by_name[n]["text"])}
        self.node_types = node_types
        # Build the inter-group type reference graph (`grefs`). Ignore marker methods (handled via
        # rt base embedding). When a method on a lower-ranked node returns a higher-ranked node
        # (for example a `Div` helper returning an `overlay.Popover` or `motion.Scrollable`), mark it
        # in `lift` instead of pulling the higher-ranked node down into `gk`.
        for d in decls:
            if d["kind"] == "type" or (d["kind"] in ("const", "var") and find(d["names"][0]) in home and d["names"][0] not in types and find(d["names"][0]) != d["names"][0]):
                g = find(d["names"][0])
            elif d["kind"] == "method" and d["recv"] in types:
                g = find(d["recv"])
                if d["id"] in markers:
                    continue
                res = re.search(r"(?m)^func \(\w+ \w+\) \w+\([^)]*\) (\w+) \{", d["text"])
                if res and res.group(1) in node_types and res.group(1) != d["recv"]:
                    fr, fu = F.get(d["recv"]), F.get(res.group(1))
                    if fr and fu and rank(pkg_of_family(fu)) > rank(pkg_of_family(fr)):
                        lift.add(d["id"])
                        continue
            else:
                continue
            grefs.setdefault(g, set()).update(find(r) for r in d["refs"] if r in types)
        # Include type references inside a type's own `put<Type>` / `rd<Type>` codec helpers so
        # wire serialization dependencies also respect the acyclic package order.
        for d in decls:
            if d["kind"] == "func" and d["names"] and d["names"][0] in local:
                mm = re.match(r"(?:put|rd)(\w+)$", d["names"][0])
                if mm and mm.group(1) in types:
                    grefs.setdefault(find(mm.group(1)), set()).update(find(r) for r in d["refs"] if r in types)
        self.lift = lift
        self.lifted_map = {}
        # First pass for flexible (unassigned) groups: place them at the highest rank among their
        # already-placed dependencies so they sit as high as possible before downward propagation.
        flex = [g for g in grefs if home.get(g) is None]
        for _ in range(8):
            for g in flex:
                if home.get(g) is None and all(home.get(u) for u in grefs[g] if u != g):
                    hs = [home[u] for u in grefs[g] if u != g]
                    home[g] = max(hs, key=rank) if hs else "gk"
        # Fixed-point downward push: if group g at rank(h) references group u at rank > rank(h),
        # move u down to h. Record every push into gen/out/build.go/pushed.txt for maintainer audits.
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
        # Cross-package marker interfaces (where implementors span more than one package, or touch
        # internal/rt) cannot have unexported `is<Iface>()` methods declared outside the interface's
        # own package. Move the interface and any non-node implementors into `rt`, and record node
        # implementors in `rt_marks` so they embed a generated `rt.<El|Val><Markers>` base struct.
        rt_marks = {}
        hof = lambda t: home[find(t)] if t in types else "rt"
        for i, mem in ifaces.items():
            hs = {hof(t) for t in mem} | {home[find(i)] if i in types else "rt"}
            if i in RT_MARKERS or len(hs) > 1 or "rt" in hs:
                if i in types:
                    home[find(i)] = "rt"
                for t in mem:
                    if t in types and t not in node_types:
                        home[find(t)] = "rt"
                rt_marks[i] = [t for t in mem if t in node_types]
            elif mem and home[find(i)] != hof(mem[0]):
                home[find(i)] = hof(mem[0])
        self.rt_marks = rt_marks
        with open(os.path.join(OUT, "pushed.txt"), "w") as f:
            for u, a, b, g in self.pushed:
                f.write(f"{u}: {a} -> {b} (named by {g})\n")
        self.marks_of = {}
        for i, mem in rt_marks.items():
            for t in mem:
                self.marks_of.setdefault(t, set()).add(i)
        # Assign every top-level declaration to a package: types follow `home`, non-lifted methods
        # stay with their receiver, methods on hand-written rt types go to `rt`, and methods on
        # unknown/unmapped receivers are dropped. Remaining funcs (constructors, ambient calls,
        # lifted conversions) go to the highest-ranked package among their references and family.
        dhome = {}
        self.dropped = set()
        for d in decls:
            if d["kind"] == "type":
                dhome[d["id"]] = home[find(d["names"][0])]
            elif d["kind"] == "method":
                if d["id"] in self.lift:
                    pass
                elif d["recv"] in types:
                    dhome[d["id"]] = home[find(d["recv"])]
                elif d["recv"] in self.hand.get("rt", ()):
                    dhome[d["id"]] = "rt"
                else:
                    self.dropped.add(d["id"])
            elif d["kind"] in ("const", "var") and find(d["names"][0]) in home and find(d["names"][0]) != d["names"][0]:
                dhome[d["id"]] = home[find(d["names"][0])]
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
        for _ in range(4):
            for d in pending:
                if d["names"] and d["names"][0] in local:
                    continue
                dhome[d["id"]] = fhome(d)
        for d in pending:
            if d["names"] and d["names"][0] in local:
                dhome[d["id"]] = None
        # Exported constructors/functions must remain callable by apps; promote any non-init func
        # that landed in `rt` up to root `gk` (which re-exports `rt` types via gen_rt.go).
        for d in decls:
            if dhome.get(d["id"]) == "rt" and d["kind"] in ("func",) and d["names"][0] != "init":
                dhome[d["id"]] = "gk"
        self.dhome, self.home = dhome, home
        with open(os.path.join(OUT, "moved.txt"), "w") as f:
            for d in decls:
                for n in d["names"][:1]:
                    if n in F and n[:1].isupper() and d["kind"] != "method" and dhome.get(d["id"]) not in (None, pkg_of_family(F[n])):
                        f.write(f"{d['kind']} {n}: {pkg_of_family(F[n])} -> {dhome[d['id']]}\n")
        self.newname = {}
        self.name_all(F)

    def name_all(self, F):
        """Strip redundant package prefixes from exported names where safe against collisions.

        Processes items in crate priority order (`gpui_component` before `gpui_base`
        before `gpui`) so high-level component types claim clean unprefixed names first.
        Checks both hand-written symbols (`claimed`) and unrenamed generated symbols
        (`reserved`) before shortening, and treats `rt` exports as sharing root `gk`'s
        namespace because `gen_rt.go` re-exports them into `gk`.
        """
        go = self.go
        claimed = {p: set(self.hand.get(p, ())) for p in ORDER}
        bases = getattr(go, "claim_base", {})
        items = []
        for n, d in self.by_name.items():
            p = self.dhome.get(d["id"])
            if n[:1].isupper() and p:
                items.append((n, p))
        order = {"gpui_component": 0, "gpui_base": 1, "gpui": 2, "gpui_kit": 3}
        items.sort(key=lambda np: (order.get(bases.get(np[0], (None, None))[1], 9), np[0]))
        for n, p in items:
            if n in claimed[p]:
                raise SystemExit(f"gk_split: {p}.{n} is declared by hand and generated")
        reserved = {p: {n for n, q in items if q == p or (p == "gk" and q == "rt")} for p in ORDER}
        for n, p in items:
            if p == "rt":
                p = "gk"
            b = bases.get(n, (n, None))[0]
            got = n
            for c in (strip(b, p), b, strip(n, p)):
                if c != n and c not in claimed[p] and c not in reserved[p]:
                    got = c
                    break
            claimed[p].add(got)
            self.newname[n] = (p, got)


    def write(self):
        """Write partitioned package files, `gen_rt.go`, `gen_bases.go`, and rewrite AST references via `gosplit apply`."""
        decls, dhome = self.decls, self.dhome
        self._methods_text = {}
        for d in decls:
            if d["kind"] == "method":
                self._methods_text[d["recv"]] = self._methods_text.get(d["recv"], "") + d["text"] + "\n"
        per = {}
        need_local = {}
        for d in decls:
            p = dhome.get(d["id"])
            if p is None:
                continue
            per.setdefault(p, {}).setdefault(d["file"], []).append(d)
        refs_of = {}
        for d in decls:
            if d["names"] and d["names"][0] in self.local:
                refs_of[d["names"][0]] = d
        # Replicate unexported helpers (put*/rd*/t*) into every package that calls them directly or transitively.
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
        bases = {}
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
        alias = ["// Code generated by gen/gkgen.py (gen/gk_split.py). DO NOT EDIT.", "",
                 "// Re-exports of generated types and constants defined in internal/rt so apps can",
                 "// reference them through package gk.", "", "package gk", ""]
        self.rt_alias = set()
        for d in sorted(decls, key=lambda d: d["names"][0] if d["names"] else ""):
            if dhome.get(d["id"]) == "rt" and d["kind"] in ("type", "const") and d["names"] and d["names"][0][:1].isupper():
                for n in d["names"]:
                    if n[:1].isupper():
                        alias.append(f"// {self.newname[n][1]} re-exports rt.{n}.")
                        alias.append(f"{d['kind']} {self.newname[n][1]} = rt.{n}")
                        self.rt_alias.add(n)
        out_files[("gk", "gen_rt.go")] = ("\n".join(alias[6:]), [])
        self.bases = bases
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
        lines = ["// Code generated by gen/gkgen.py (gen/gk_split.py). DO NOT EDIT.", "",
                 "// Embedded base structs for generated element and value nodes, carrying cross-package",
                 "// interface markers.", "",
                 "package rt", ""]
        for b, (kind, ms) in sorted(bases.items()):
            core = "ElCore" if kind == "El" else "ValCore"
            lines += [f"// {b} embeds {core} for generated {'element' if kind == 'El' else 'value'} types" + (f" implementing {', '.join(ms)}." if ms else "."),
                      f"type {b} struct{{ {core} }}", "",
                      f"// Mk{b} wraps runtime node pointer n in {b}.",
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
        """Rewrite `gk.<Name>` references inside string literals to match moved/renamed subpackage symbols."""
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
        """Return the set of generated handle struct names (`struct{ h uint64 }`)."""
        if not hasattr(self, "_handles"):
            self._handles = {n for n in self.types if re.search(r"struct\{\s*h uint64\s*\}", self.by_name[n]["text"])}
        return self._handles

    def lifted(self, d):
        """Rewrite a cross-package conversion method into a top-level function `<RecvStem><Method>(e <Recv>, ...)`."""
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
        """Replace flat `struct{ n *node }` / `struct{ h uint64 }` fields with embedded `rt.*` core structs."""
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


    def write_table(self):
        """Write `gen/out/build.go/table.json` consumed by `gosplit apply` to rewrite imports and qualifiers."""
        names = {}
        for p in ORDER:
            for n in self.hand.get(p, ()):
                if n[:1].isupper() or p != "rt":
                    if p == "rt" and n in names:
                        continue
                    names.setdefault(n, (p, n))
        for old, (p, new) in self.legacy.items():
            names[old] = (p, new)
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
    """Verify no Go or Rust string literal cites gk.<Name> for a symbol moved out of package gk."""
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
