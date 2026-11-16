"""gk_doc: GPUI-kit's own documentation, carried into the generated Go and Odin (docs/DESIGN.md
section 13), so an editor shows it on hover.

Reads api/rustdoc/docs.json.gz (gkgen.py strip keeps each item's doc text and intra-doc links
there) and translates a doc comment for a binding:
  - an intra-doc link to something the binding has becomes that thing's name in the language
    (`[`Self::on_click`]` is `OnClick` in Go, `on_click` in Odin); any other link keeps its text;
  - Rust code examples are left out (a ```-fenced block, and an Examples heading left empty);
  - a sentence about Rust-only mechanics (`cx.notify()`, `&mut Window`, `Rc<..>`, ...) is left
    out: a binding's handler redraws its own view and has no window or context to pass, so the
    sentence would be wrong. Each one left out is listed in gen/out/doc-rust-only.md, which shows
    what a new GPUI-kit version adds;
  - the text is wrapped to 100 columns.
"""
import gzip
import json
import os
import re
import textwrap

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
PATH = os.path.join(REPO, "api", "rustdoc", "docs.json.gz")
REPORT = os.path.join(HERE, "out", "doc-rust-only.md")
WIDTH = 100

# Wording only Rust code can act on. A sentence with any of these is left out.
RUST_ONLY = re.compile(
    r"\bcx\.\w+\(|\bwindow\.\w+\(|&mut\b|\bRc<|\bArc<|\bBox<|\bdyn\b|\bimpl\s+(Fn|Into|IntoElement)\b"
    r"|\b(Entity|Context|WeakEntity)<|::new\(|\.clone\(\)|\bunsafe\b|#\[|\bClone\b|\bDrop\b|\blifetime\b"
    r"|`Self`|\bborrow(ed|s)?\b|\bmacro\b|\bcrate\b")
# Rust-only wording with a plain equivalent, rewritten before RUST_ONLY is applied: a binding's
# handler redraws its own view (no cx.notify()), and the theme's colours are read by name.
REWRITES = [
    (re.compile(r"\s+and call `cx\.notify\(\)` to render it"), " for it to show"),
    (re.compile(r"`cx\.theme\(\)\.(\w+)`"), r"the theme's `\1` colour"),
]
FENCE = re.compile(r"^\s*(```|~~~)")
LINK = re.compile(r"\[([^\]\n]+)\](?:\(([^)\s]+)\)|\[([^\]\n]+)\])?")
SENTENCE = re.compile(r"(?<=[.!?])\s+(?=[A-Z`(\[])")


def rewrite(text):
    for pat, new in REWRITES:
        text = pat.sub(new, text)
    return text


def _load():
    if not os.path.exists(PATH):
        return {}
    with gzip.open(PATH, "rt", encoding="utf-8") as f:
        return json.load(f)


class Docs:
    """Doc comments by rustdoc id, translated per language. names(lang, crate, id) gives the
    binding's name for a linked item, or None."""

    def __init__(self):
        self.raw = _load()
        self.dropped = []  # (language, owner, sentence)
        self.asked, self.found = set(), set()  # distinct (crate, id) asked for, and those with a doc

    def has(self, ref):
        return bool(ref) and str(ref[1]) in self.raw.get(ref[0], {}).get("items", {})

    def comment(self, ref, lang, names, owner=""):
        """The translated doc of ref = (crate, id) as text lines without a comment marker, or []."""
        if not ref:
            return []
        crate, iid = ref[0], str(ref[1])
        self.asked.add((crate, iid))
        entry = self.raw.get(crate, {}).get("items", {}).get(iid)
        if not entry:
            return []
        self.found.add((crate, iid))
        paths = self.raw.get(crate, {}).get("paths", {})
        links = entry["l"]

        def link(m):
            text, dest = m.group(1), m.group(2) or m.group(3)
            key = dest or text
            if dest and re.match(r"https?://", dest):
                return f"{text} ({dest})"
            target = links.get(key)
            if target is None and not dest:
                target = links.get(text.strip("`"))
            if target is not None:
                name = names(lang, crate, str(target), paths.get(str(target)))
                if name:
                    return f"`{name}`" if dest is None or text.startswith("`") else f"{text} (`{name}`)"
            plain = text.strip("`")
            if re.fullmatch(r"[\w:]+(\(\))?", plain):  # a Rust path: its last part
                plain = plain.split("::")[-1]
                return f"`{plain}`"
            return text

        out, fenced = [], False
        for line in entry["d"].split("\n"):
            if FENCE.match(line):
                fenced = not fenced
                continue
            if not fenced:
                out.append(line)
        paras, cur = [], []
        for line in out:
            if line.strip():
                cur.append(line.strip())
            elif cur:
                paras.append(cur)
                cur = []
        if cur:
            paras.append(cur)
        result = []
        for p in paras:
            if p[0].startswith("#"):  # a heading: kept as "Panics:" when text follows it
                result.append(("head", p[0].lstrip("#").strip()))
                if len(p) == 1:
                    continue
                p = p[1:]
            if all(x.startswith(("- ", "* ")) or re.match(r"\d+\. ", x) for x in p[:1]):
                items = []
                for x in p:
                    if x.startswith(("- ", "* ")) or re.match(r"\d+\. ", x):
                        items.append(x)
                    elif items:
                        items[-1] += " " + x
                kept = [LINK.sub(link, rewrite(x)) for x in items]
                kept = [x for x in kept if not self._rust(lang, owner, x)]
                if kept:
                    result.append(("list", kept))
                continue
            text = LINK.sub(link, rewrite(" ".join(p)))
            keep = [s for s in SENTENCE.split(text) if not self._rust(lang, owner, s)]
            if keep:
                result.append(("para", " ".join(keep)))
        lines = []
        for i, (kind, val) in enumerate(result):
            if kind == "head":
                nxt = result[i + 1][0] if i + 1 < len(result) else None
                if nxt in ("para", "list") and val.lower() not in ("examples", "example"):
                    lines += ([""] if lines else []) + [val + ":"]
                continue
            if lines and lines[-1] != "" and not lines[-1].endswith(":"):
                lines.append("")
            if kind == "para":
                lines += textwrap.wrap(val, WIDTH - 4, break_long_words=False, break_on_hyphens=False)
            else:
                for x in val:
                    w = textwrap.wrap(x, WIDTH - 6, break_long_words=False, break_on_hyphens=False)
                    lines += [w[0]] + ["  " + y for y in w[1:]]
        while lines and lines[-1] == "":
            lines.pop()
        return lines

    def _rust(self, lang, owner, sentence):
        """Whether to leave the sentence out (Rust-only wording, after REWRITES)."""
        if RUST_ONLY.search(sentence):
            self.dropped.append((lang, owner, sentence))
            return True
        return False

    def report(self):
        os.makedirs(os.path.dirname(REPORT), exist_ok=True)
        seen, rows = set(), []
        for lang, owner, s in self.dropped:
            if (owner, s) in seen:
                continue
            seen.add((owner, s))
            rows.append(f"- `{owner}`: {s}")
        with open(REPORT, "w", encoding="utf-8") as f:
            f.write("# Sentences of GPUI-kit's docs left out of the bindings\n\n"
                    "Written by gen/gk_doc.py: each sentence names Rust-only mechanics (gk_doc.RUST_ONLY).\n\n")
            f.write("\n".join(sorted(rows)) + "\n")
        return len(rows)


FACADE = {"gpui_component": "gpui_kit::component", "gpui_base": "gpui_kit::base", "gpui": "gpui_kit"}


def resolver(m):
    """names(lang, crate, id, path) for Docs.comment: the binding's name of a linked item that it
    binds (a type, or a function or method by its name in that language), else None."""
    import gk_go  # noqa: PLC0415
    bound = ("bound", "bridged")
    by, by_path = {}, {}
    for n in m.nodes.values():
        by[(n["crate"], str(n["id"]))] = ("type", n)
        by_path[n["path"]] = ("type", n)
    for st in m.states.values():
        if st.get("doc"):
            by[tuple(st["doc"])] = ("state", st)
        by_path[st["path"]] = ("state", st)
    for table in (m.enums, m.denums, m.records):
        for rt, t in table.items():
            by_path[rt] = ("type", t)
    owner = {}
    for st in m.states.values():
        for g in ("reads", "writes", "calls", "events"):
            for it in st.get(g, []):
                owner[id(it)] = st
    for it in m.items + [i for n in m.nodes.values() for i in n["ctors"]]:
        if it.get("doc") and it["status"] in bound:
            by.setdefault(tuple(it["doc"]), ("item", it))

    def names(lang, crate, iid, path):
        e = by.get((crate, iid))
        if e is None and path:
            first, _, rest = path.partition("::")
            e = by_path.get(FACADE.get(first, first) + ("::" + rest if rest else ""))
        if e is None:
            return None
        kind, x = e
        if kind in ("type", "state"):
            return x.get(lang)
        if x.get(lang):  # a free function or a constructor
            return ("gk." if lang == "odin" else "gk.") + x[lang]
        if lang == "go":
            return gk_go.camel(x["name"])
        st = owner.get(id(x))
        if st is not None and st.get("odin"):
            return f"gk.{st['odin'].lower()}_{x['name']}"
        return "->" + x["name"]

    return names
