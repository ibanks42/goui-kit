"""gk_names: the stem of every name the package declares, before any front end spells it.

A stem is snake_case for a function (`button_large`) and Ada_Case for a type (`Button_Variant`).
Names are claimed in one namespace, with the priority component, base, gpui: when two upstream
items want the same stem, the one claimed later gets its crate's prefix (Kit_, Base_, Gpui_) and,
if that is taken too, a number. gk_go turns every stem into a Go name by the rules of
docs/GO.md section 2 (camel), so the set of stems decides which items get a prefix.

The keys written into the model: node["stem"], enum, record and data enum ["stem"],
record["short_stem"] (a record a parameter may take as one field alone), state ["stem"],
Into set ["stem"], trait ["stem"] and ["low"], and the constructor, global and ambient
call items' ["stem"]; and item["emitted"] on the convert methods the package declares.
"""
from gk_model import ada

# Words a stem may not be: it gets a trailing underscore instead (camel drops it again).
KEYWORDS = {"in", "proc", "map", "context", "import", "defer", "when", "using", "struct", "enum", "union", "if",
            "for", "do", "switch", "case", "return", "package", "foreign", "distinct", "dynamic", "cast",
            "transmute", "nil", "true", "false", "or_else", "or_return", "bit_set", "matrix", "typeid", "break",
            "continue", "fallthrough", "else", "where", "not_in", "auto_cast", "asm", "or_break", "or_continue",
            "bit_field", "size_of", "align_of", "len", "cap", "string", "int", "bool", "any"}
# Stems the runtime owns: an upstream item that wants one gets its crate's prefix.
RESERVED = {"Element", "Any_El", "Value", "Color", "Length", "Id", "Named_Id", "Handler", "Tape", "Reader", "View",
            "Window", "Cx", "Temp_String", "Style_Flag", "Action", "Range", "Build", "Handle", "Window_Options",
            "Rows", "Model", "Palette", "Theme_Color", "Node_Rec", "Turn", "Post", "Entity", "Data", "Data_Kind",
            "on", "quiet", "keep", "view", "notify", "post", "run", "quit", "open_window", "text", "px", "rems",
            "relative", "auto", "hsla", "id", "style", "op", "take", "tokens", "register", "build", "finish",
            "new_node", "theme", "palette", "main", "report", "dispatch", "event", "handler", "rows", "window",
            "data"}
# Stems of types the hand-written runtime declares (a row source's VirtualList, a dock's NodeId):
# an upstream type of the same name gets its crate's prefix.
RUNTIME_TYPES = {"Virtual_List", "Node_Id", "Code_Block", "Os_Action", "Tree_Entry"}

ORDER = {"gpui_component": 0, "gpui_base": 1, "gpui": 2, "gpui_kit": 3}


def word(name):
    return name + "_" if name in KEYWORDS else name


def bound(it):
    return it["status"] in ("bound", "bridged")


class Names:
    def __init__(self, m):
        self.m = m
        self.names = {name: "runtime" for name in sorted(RUNTIME_TYPES)}  # stem -> what owns it
        self.collisions = []
        self.name_types()
        self.mark_converts()

    def claim(self, base, crate, key, lower=False):
        """A package-level stem. Priority component, base, gpui; the loser gets a prefix."""
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
        # A record a parameter may take as one field alone (PROTOCOLS.md 11.36): three unions,
        # named after upstream's type. The name the record's own builder node claimed is free
        # when that node is never emitted (IndexPath: its builders are value helpers).
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
                # A library object (PROTOCOLS.md section 13): named when the app can get one.
                if not st.get("reachable"):
                    continue
                st["stem"] = self.claim(ada(st["name"]), st["crate"], "state " + key)
                low = st["stem"].lower()
                for it in sorted(st["globals"] + st["statics"], key=lambda i: i["id"]):
                    if it["status"] == "bridged":
                        it["stem"] = self.claim(f"{low}_{it['name']}", st["crate"], it["id"], lower=True)
            elif not any(i["status"] == "bridged" for i in st["ctors"]) and not st.get("handed"):  # 11.8
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
        import gk_app  # noqa: PLC0415
        for it in sorted(gk_app.bound(m), key=lambda i: i["id"]):  # the ambient calls (PROTOCOLS.md 11.5)
            base = it["name"] if it["crate"] == "gpui_component" or it.get("trait") else "base_" + it["name"]
            base = it.get("stem_base", base)  # a static of a type, named after it (gk.RootRead, 13.21)
            it["stem"] = self.claim(base, it["crate"], it["id"], lower=True)

    def mark_converts(self):
        """The methods that make another node of an element (div().id(..) makes a stateful div)
        the package declares: on a node it can construct, a bound convert whose name no bound
        method of the node has, the first of each name by (trait, name). Marked item["emitted"]
        (the generated sample makes a node through one)."""
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
