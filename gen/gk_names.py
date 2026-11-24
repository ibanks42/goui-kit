"""Assign canonical, collision-free stem names to every type, trait, state, and constructor.

Runs after the model is built and quarantined, before Go emission. Every model entity
(nodes, enums, data enums, records, record shorthands, states, objects, `Into` sets,
traits, constructors, and ambient calls) receives a deterministic `stem` attribute in
Ada_Case (or lowercase for constructors and functions).

Collision resolution rules (`Names.claim`):
1. `RUNTIME_TYPES` are pre-seeded in `self.names` so generated types never shadow hand-written
   runtime structs.
2. Any stem in `RESERVED` is skipped directly to the crate-prefixed fallback (`Base_`,
   `Gpui_`, `Kit_`), preventing generated names from colliding with core runtime verbs/types.
3. Crates are processed in priority order (`gpui_component` > `gpui_base` > `gpui` >
   `gpui_kit`), so high-level components keep unprefixed stems when base and component
   crates define the same type name.
4. Any stem matching a reserved language keyword in `KEYWORDS` gets a trailing `_`.
5. If even the crate-prefixed candidate is taken, a numeric suffix (`_2`, `_3`, ...) is
   appended and the collision is logged in `self.collisions` (written to
   `gen/out/collisions.md` by `gkgen.py`).

Downstream emitters (`gen/gk_go.py`, `gen/gk_tests_go.py`) derive Go CamelCase names
directly from these stems.
"""
from gk_model import ada

# Words avoided when claiming stems (suffixed with `_`) so generated names stay valid across bindings.
KEYWORDS = {"in", "proc", "map", "context", "import", "defer", "when", "using", "struct", "enum", "union", "if",
            "for", "do", "switch", "case", "return", "package", "foreign", "distinct", "dynamic", "cast",
            "transmute", "nil", "true", "false", "or_else", "or_return", "bit_set", "matrix", "typeid", "break",
            "continue", "fallthrough", "else", "where", "not_in", "auto_cast", "asm", "or_break", "or_continue",
            "bit_field", "size_of", "align_of", "len", "cap", "string", "int", "bool", "any"}
# Runtime-reserved stems that generated types/functions must never claim directly (they receive a crate prefix).
RESERVED = {"Element", "Any_El", "Value", "Color", "Length", "Id", "Named_Id", "Handler", "Tape", "Reader", "View",
            "Window", "Cx", "Temp_String", "Style_Flag", "Action", "Range", "Build", "Handle", "Window_Options",
            "Rows", "Model", "Palette", "Theme_Color", "Node_Rec", "Turn", "Post", "Entity", "Data", "Data_Kind",
            "on", "quiet", "keep", "view", "notify", "post", "run", "quit", "open_window", "text", "px", "rems",
            "relative", "auto", "hsla", "id", "style", "op", "take", "tokens", "register", "build", "finish",
            "new_node", "theme", "palette", "main", "report", "dispatch", "event", "handler", "rows", "window",
            "data"}
# Hand-written runtime type stems pre-populated in `Names.names` before model naming runs.
RUNTIME_TYPES = {"Virtual_List", "Node_Id", "Code_Block", "Os_Action", "Tree_Entry"}

# Crate priority when two upstream crates define a type with the same stem.
ORDER = {"gpui_component": 0, "gpui_base": 1, "gpui": 2, "gpui_kit": 3}


def word(name):
    """Suffix reserved language keywords with `_` so stems remain valid identifiers."""
    return name + "_" if name in KEYWORDS else name


def bound(it):
    """Return True if model item `it` is bound or bridged."""
    return it["status"] in ("bound", "bridged")


class Names:
    """Claims unique stems for all model entities and flags emitted conversion methods."""

    def __init__(self, m):
        self.m = m
        self.names = {name: "runtime" for name in sorted(RUNTIME_TYPES)}
        self.collisions = []
        self.name_types()
        self.mark_converts()

    def claim(self, base, crate, key, lower=False):
        """Claim base (or a crate-prefixed / numbered fallback) for key."""
        pre = {"gpui_base": "Base_", "gpui": "Gpui_", "gpui_component": "Kit_", "gpui_kit": "Kit_"}[crate]
        cands = [base] if base not in RESERVED else []
        cands += [(pre.lower() if lower else pre) + base]
        for c in cands:
            c = word(c)
            if c not in self.names:
                self.names[c] = key
                return c
        self.collisions.append((base, key, self.names.get(cands[-1])))
        n = 2
        while f"{cands[-1]}_{n}" in self.names:
            n += 1
        self.names[f"{cands[-1]}_{n}"] = key
        return f"{cands[-1]}_{n}"

    def name_types(self):
        """Populate stem attributes on nodes, value types, states, Into sets, traits, and ctors."""
        m, order = self.m, ORDER
        for key in sorted(m.nodes, key=lambda k: (order[m.nodes[k]["crate"]], k)):
            node = m.nodes[key]
            base = self.m.ov.get("rename", {}).get(node["name"], ada(node["name"]))
            node["stem"] = self.claim(base, node["crate"], key)
        for table in (m.enums, m.denums, m.records):
            for rt in sorted(table, key=lambda k: (order[table[k]["crate"]], k)):
                t = table[rt]
                base = ada(t["name"]) + "".join("_" + ada(a) for a in t.get("args", []) if a != "Pixels")
                t["stem"] = self.claim(base, t["crate"], rt)
        for rt in sorted(m.records):
            rc = m.records[rt]
            if not rc.get("shorthand"):
                continue
            base = ada(rc["name"])
            node = m.nodes.get(rt)
            if self.names.get(base) == rt and node is not None and not any(
                    bound(i) for i in node["ctors"] + node.get("ops", [])):
                del self.names[base]
            rc["short_stem"] = {how: self.claim(name, rc["crate"], f"shorthand {how} {rt}") for how, name in
                                (("one", base), ("opt", "Maybe_" + base), ("list", base + "_List"))}
        for key in sorted(m.states, key=lambda k: (order[m.states[k]["crate"]], k)):
            st = m.states[key]
            if st.get("obj"):
                if not st.get("reachable"):
                    continue
                st["stem"] = self.claim(ada(st["name"]), st["crate"], "state " + key)
                low = st["stem"].lower()
                for it in sorted(st["globals"] + st["statics"], key=lambda i: i["id"]):
                    if it["status"] == "bridged":
                        it["stem"] = self.claim(f"{low}_{it['name']}", st["crate"], it["id"], lower=True)
            elif not any(i["status"] == "bridged" for i in st["ctors"]) and not st.get("handed"):
                continue
            else:
                st["stem"] = self.claim(ada(st["name"]), st["crate"], "state " + key)
            for it in sorted(st["ctors"], key=lambda i: i["id"]):
                if it["status"] == "bridged":
                    base = st["stem"].lower() + ("" if it["name"] == "new" else "_" + it["name"])
                    it["stem"] = self.claim(base, st["crate"], it["id"], lower=True)
        for rt in sorted(m.intos):
            s = m.intos[rt]
            if len(s["members"]) > 1:
                s["stem"] = self.claim(ada(s["name"]) + "_Into", "gpui_kit", "into " + rt)
        for key in sorted(m.traits, key=lambda k: (order[m.traits[k]["crate"]], m.traits[k]["key"] or "")):
            t = m.traits[key]
            if t["ops"]:
                t["stem"] = self.claim(ada(t["name"]) + "".join("_" + ada(x) for x in t["extra"]) + "_Trait",
                                       t["crate"], "trait " + t["key"])
                t["low"] = t["stem"][:-6].lower()
        for key in sorted(m.nodes, key=lambda k: (order[m.nodes[k]["crate"]], k)):
            node = m.nodes[key]
            for it in sorted(node["ctors"], key=lambda i: i["id"]):
                if not bound(it) or it["kind"] == "convert":
                    continue
                if it.get("free"):
                    base = it["name"]
                elif it["name"] == "new":
                    base = node["stem"].lower()
                else:
                    owner = m.nodes.get(it["owner"], node)["stem"].lower() if it["owner"] in m.nodes else node["stem"].lower()
                    base = f"{owner}_{it['name']}"
                it["stem"] = self.claim(base, it["crate"], it["id"], lower=True)
        import gk_app
        for it in sorted(gk_app.bound(m), key=lambda i: i["id"]):
            base = it["name"] if it["crate"] == "gpui_component" or it.get("trait") else "base_" + it["name"]
            base = it.get("stem_base", base)
            it["stem"] = self.claim(base, it["crate"], it["id"], lower=True)

    def mark_converts(self):
        """Mark type-changing conversion methods that do not shadow an inherent op."""
        m = self.m
        for key in sorted(m.nodes):
            node = m.nodes[key]
            if not any(bound(i) for i in node["ctors"]):
                continue
            opnames = {i["name"] for i in node["ops"] if bound(i)}
            seen = set()
            for it in sorted([i for i in node.get("converts", []) if bound(i)], key=lambda i: (i["trait"] or "", i["name"])):
                if it["name"] not in opnames and it["name"] not in seen:
                    seen.add(it["name"])
                    it["emitted"] = True
