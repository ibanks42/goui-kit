"""Adapt upstream rustdoc comments from `api/rustdoc/docs.json.gz` for generated Go bindings.

`gen/gkgen.py strip` extracts raw rustdoc Markdown (`d`) and intra-doc link ID maps (`l`)
into `api/rustdoc/docs.json.gz` so the main rustdoc JSON stays small. During Go emission,
`Docs.comment()` transforms each item's upstream comment into clean godoc:
1. Drops fenced Rust code blocks (` ``` ` / `~~~`) and empty `# Examples` headings.
2. Applies targeted phrasing `REWRITES` (e.g. `cx.notify()` -> language-neutral wording).
3. Resolves Markdown intra-doc links (`[Foo]`, `[bar](baz)`) through `resolver(m)` into
   the corresponding Go type/method/function name (falling back to the last path segment
   or keeping external `https?://` URLs inline).
4. Splits paragraphs into sentences and drops any sentence matching `RUST_ONLY` (Rust
   ownership/borrow/pointer/GPUI-context internals that do not apply to Go), recording
   every dropped sentence in `gen/out/doc-rust-only.md` for auditability.
5. Wraps paragraphs and bullet lists to `WIDTH` columns (`100`, leaving room for `// `).
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

# Sentences matching this pattern describe Rust-specific APIs or ownership semantics and are
# omitted from generated Go godoc (and logged in gen/out/doc-rust-only.md).
RUST_ONLY = re.compile(
    r"\bcx\.\w+\(|\bwindow\.\w+\(|&mut\b|\bRc<|\bArc<|\bBox<|\bdyn\b|\bimpl\s+(Fn|Into|IntoElement)\b"
    r"|\b(Entity|Context|WeakEntity)<|::new\(|\.clone\(\)|\bunsafe\b|#\[|\bClone\b|\bDrop\b|\blifetime\b"
    r"|`Self`|\bborrow(ed|s)?\b|\bmacro\b|\bcrate\b")
# Patterns rewritten before sentence filtering so common GPUI idioms survive into Go godoc
# instead of being dropped by RUST_ONLY.
REWRITES = [
    (re.compile(r"\s+and call `cx\.notify\(\)` to render it"), " for it to show"),
    (re.compile(r"`cx\.theme\(\)\.(\w+)`"), r"the theme's `\1` color"),
]
# Markdown code fences, intra-doc links, and sentence boundaries used when cleaning rustdoc blocks.
FENCE = re.compile(r"^\s*(```|~~~)")
LINK = re.compile(r"\[([^\]\n]+)\](?:\(([^)\s]+)\)|\[([^\]\n]+)\])?")
EM_DASH = re.compile("\\s*\u2014\\s*")
SENTENCE = re.compile(r"(?<=[.!?])\s+(?=[A-Z`(\[])")


def rewrite(text):
    """Apply `REWRITES` substitutions to replace common GPUI Rust phrases with language-neutral wording."""
    for pat, new in REWRITES:
        text = pat.sub(new, text)
    # Upstream prose uses em dashes; the project's style forbids them, so a spaced hyphen stands in.
    return EM_DASH.sub(" - ", text)


def _load():
    """Load `api/rustdoc/docs.json.gz` into memory, or return `{}` if absent."""
    if not os.path.exists(PATH):
        return {}
    with gzip.open(PATH, "rt", encoding="utf-8") as f:
        return json.load(f)


class Docs:
    """Renders filtered, link-resolved doc comments from api/rustdoc/docs.json.gz."""

    def __init__(self):
        self.raw = _load()
        self.dropped = []
        self.asked, self.found = set(), set()

    def has(self, ref):
        """Return True if upstream rustdoc text exists for (crate, item_id)."""
        return bool(ref) and str(ref[1]) in self.raw.get(ref[0], {}).get("items", {})

    def comment(self, ref, lang, names, owner=""):
        """Return wrapped comment lines (without leading //) for a rustdoc item ref."""
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
            if re.fullmatch(r"[\w:]+(\(\))?", plain):
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
            if p[0].startswith("#"):
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
        if RUST_ONLY.search(sentence):
            self.dropped.append((lang, owner, sentence))
            return True
        return False

    def report(self):
        """Write every deduplicated Rust-only sentence omitted during emission to REPORT."""
        os.makedirs(os.path.dirname(REPORT), exist_ok=True)
        seen, rows = set(), []
        for lang, owner, s in self.dropped:
            if (owner, s) in seen:
                continue
            seen.add((owner, s))
            rows.append(f"- `{owner}`: {s}")
        with open(REPORT, "w", encoding="utf-8") as f:
            f.write("# Omitted Rust-only documentation sentences\n\n"
                    "Generated by gen/gk_doc.py. Each entry matched gk_doc.RUST_ONLY and was left out of generated comments.\n\n")
            f.write("\n".join(sorted(rows)) + "\n")
        return len(rows)


# Hand-written binding notes prepended to generated Go comments for specific upstream items.
NOTES = {
    "gpui_kit::ImageSource::Render": {
        "go": ["Displays a pixel image created in the runtime. Construct the gk.RenderImage once",
               "with gk.NewRenderImage, hold it while any view displays it, and call gk.Release",
               "when finished so GPUI can free the GPU texture."],
    },
    "gpui_kit::ImageSource::Custom": {
        "go": ["In Go, upstream's draw-time loader is represented directly by a gk.RenderImage",
               "created once with gk.NewRenderImage, matching gk.ImageSourceRender."],
    },
}


def note(path, lang):
    """Return formatted Go comment lines for a binding-specific note, or an empty list."""
    lines = NOTES.get(path, {}).get(lang)
    return [f"// {x}" for x in lines] + ["//"] if lines else []


FACADE = {"gpui_component": "gpui_kit::component", "gpui_base": "gpui_kit::base", "gpui": "gpui_kit"}


def resolver(m):
    """Build a lookup function mapping rustdoc link targets to Go symbol names."""
    import gk_go
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
        if x.get(lang):
            return "gk." + x[lang]
        return gk_go.camel(x["name"])

    return names
