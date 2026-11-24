#!/usr/bin/env python3
"""gk_apidoc.py [--check]: the component index of docs/API-GO.md, read from the generated Go
package.

The index is written between two markers in the guide. It is read from the files an app compiles
against (go/gk/gen_*.go), not from the generator's model, so what it names is what exists: every
element type, value type and kit state type of the kit's component and base modules, its upstream
type, its constructor as an app calls it, and the builders, reads, writes and events it has beyond
the styling every element shares (the methods of gk.Div()).

    python3 gen/gk_apidoc.py           rewrite the index (make gen runs it)
    python3 gen/gk_apidoc.py --check   exit 1 if the guide's index is not what the package says
                                       (gates/docs-go.sh)
Python 3 standard library only.
"""
import os
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
END = "<!-- index:end -->"

# Builders an index line shows first when a type has them: what an app reaches for.
FIRST = ["label", "title", "text", "child", "children", "value", "checked", "selected", "icon", "placeholder",
         "on_click", "on_change", "on_select", "tooltip", "description", "content", "footer", "prefix", "suffix",
         "primary", "ghost", "outline", "small", "large", "disabled"]
SHOWN = 8  # builder names per line before "…"

# Trait tables whose builders every element has (styling, events, children, scrolling, focus,
# accessibility): an index line leaves them out. Other tables (ButtonVariants: .Primary, Sizable:
# .Small) are the type's own vocabulary and are listed with its builders.
SHARED_TRAITS = {"Styled", "Interactive_Element", "Stateful_Interactive_Element", "Parent_Element",
                 "Scrollable_Element", "Element_Ext", "Styled_Ext", "Focusable_Ext", "Theme_Styled",
                 "Theme_Styled_Parent_Element", "Accessible", "Accessibility_Ext"}


def module_of(path):
    """The upstream module an index section is named after: component::button, base::input, base."""
    parts = re.sub(r"<.*", "", path).split("::")
    if len(parts) >= 3 and parts[0] == "gpui_kit" and parts[1] in ("component", "base"):
        mod = parts[1] + ("::" + parts[2] if len(parts) > 3 else "")
        return mod
    return None


def short(path):
    return re.sub(r"\w+::", "", path)


# ---- The Go index (docs/API-GO.md), read from go/gk/gen_*.go ------------------------------------
#
# A type is declared by its doc comment
# (`// ButtonEl is gpui_kit::component::button::Button.`) and a struct holding a node (an element,
# or a value when it has valueNode) or a handle (kit state). Builders are its exported methods; a
# method whose body calls a trait function (tStyledBg) belongs to that trait, and the shared traits
# are left out. Read before or after gofmt alike (spacing is not significant).

GO_PKG = os.path.join(REPO, "go", "gk")
GO_DOC = os.path.join(REPO, "docs", "API-GO.md")
GO_BEGIN = "<!-- index:begin (written by gen/gk_apidoc.py from go/gk/gen_*.go; do not edit) -->"
GO_DECL = re.compile(r"^// (\w+) is (?:a handle to )?(`?)([\w:<>, ]+?)\2(?:[.,]| \().*\n"
                     r"type (\w+) struct ?\{ ?(n \*node|h uint64) ?\}", re.M)
GO_FUNC = re.compile(r"^func (?:\((\w+) (\w+)\) )?(\w+)\(", re.M)
GO_TRAITFN = re.compile(r"^// (t[A-Z]\w*): ([\w:<>, ]+)::(\w+)\.$", re.M)
# putText and slotView are the Go package's .Text and .ChildView, which every parent element has.
GO_TRAITCALL = re.compile(r"\b(t[A-Z]\w*|putText|slotView)\(e\.n\b")
# Go's names for the shared trait tables (without underscores, as upstream spells them).
GO_SHARED_TRAITS = {t.replace("_", "") for t in SHARED_TRAITS}
GO_SHARED_PREFIX = ("A11y", "Accessibility", "Aria")
GO_FIRST = ["".join(w.capitalize() for w in n.split("_")) for n in FIRST]
# The call that makes the row source a kit state's constructor takes.
GO_SOURCES = {
    "ComboboxState": "gk.NewSearchSource(cx, items)",
    "ListState": "gk.NewListSource(cx, count, row)",
    "SelectState": "gk.NewSearchSource(cx, items)",
    "TableState": "delegate",  # no Go table source yet: docs/API-GO.md says so
}


def go_read_package():
    out = {}
    for fn in sorted(os.listdir(GO_PKG)):
        if fn.startswith("gen_") and fn.endswith(".go"):
            out[fn] = open(os.path.join(GO_PKG, fn)).read()
    return out


def go_split(text, start):
    """From the '(' at start: (the text inside the balanced parentheses, the index after them)."""
    depth = 0
    for i in range(start, len(text)):
        if text[i] == "(":
            depth += 1
        elif text[i] == ")":
            depth -= 1
            if depth == 0:
                return text[start + 1:i], i + 1
    return text[start + 1:], len(text)


def go_funcs(text, fn):
    """Every func of a file: (receiver type or None, name, parameters, result, body, file)."""
    out = []
    for m in GO_FUNC.finditer(text):
        params, at = go_split(text, m.end() - 1)
        eol = text.find("\n", at)
        rest = text[at:eol]
        brace = rest.find("{")
        result = rest[:brace].strip() if brace >= 0 else rest.strip()
        if rest.rstrip().endswith("}"):  # a one-line body
            body = rest[brace:]
        else:
            end = text.find("\n}\n", eol)
            body = text[eol:end if end >= 0 else len(text)]
        out.append((m.group(2), m.group(3), params, result, body, fn))
    return out


def go_params(sig):
    """The parameter names an app writes: without the trailing style tokens (st ...Style)."""
    parts, depth, cur = [], 0, ""
    for ch in sig:
        if ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        if ch == "," and depth == 0:
            parts.append(cur.strip())
            cur = ""
        else:
            cur += ch
    if cur.strip():
        parts.append(cur.strip())
    names = []
    for p in parts:
        if p.endswith("...Style"):
            continue
        names.append(p.split()[0])
    return names


def go_index(files):
    decls, funcs, traitfn = [], [], {}
    for fn, text in files.items():
        for m in GO_DECL.finditer(text):
            decls.append((m.group(4), m.group(3), m.group(5), fn))
        funcs += go_funcs(text, fn)
        for m in GO_TRAITFN.finditer(text):
            traitfn[m.group(1)] = m.group(2).split("::")[-1]
    traitfn["putText"] = traitfn["slotView"] = "ParentElement"
    for recv, name, _, _, _, _ in funcs:  # trait functions written without a doc comment
        if recv is None and name.startswith("t") and name not in traitfn:
            hit = [t for t in GO_SHARED_TRAITS if name[1:].startswith(t)]
            if hit:
                traitfn[name] = max(hit, key=len)
    methods, ctors = {}, {}
    for recv, name, params, result, body, fn in funcs:
        if recv and not name[0].isupper():
            continue
        if recv:
            calls = [traitfn.get(c) for c in GO_TRAITCALL.findall(body)]
            trait = next((t for t in calls if t), None)
            methods.setdefault(recv, []).append((name, params, result, trait))
        elif name[0].isupper():
            ctors.setdefault(result, []).append((name, params, result.endswith("Builder")))
    shared = {n for n, _, _, _ in methods.get("DivEl", [])}
    for ms in methods.values():
        shared |= {n for n, _, _, t in ms if t in GO_SHARED_TRAITS}
    shared.add("Style")
    states = {t for t, _, f, _ in decls if f.startswith("h ")}
    sections, states_shown = {}, {}
    for typ, path, field, fn in decls:
        mod = module_of(path)
        if mod is None:
            continue
        ms = methods.get(typ, [])
        names = {n for n, _, _, _ in ms}
        if field.startswith("h "):
            kind = "state"
        elif "valueNode" in files[fn] and re.search(rf"^func \(e {typ}\) valueNode\(\)", files[fn], re.M):
            kind = "value"
        else:
            kind = "element"
        own_ms = [m for m in ms if m[0] not in ("Build",)]
        if fn == "gen_base_actions.go" or (kind == "value" and any("Default" in c[0] for c in ctors.get(typ, []))
                                           and not own_ms):
            continue
        entry = {"type": typ, "path": path, "kind": kind}
        if kind == "state":
            made = ctors.get(typ + "Builder", []) + ctors.get(typ, [])
            reads, writes, events = [], [], []
            for name, params, result, _ in ms:
                if name.startswith("On") and result == "Subscription":
                    events.append(name)
                elif result:
                    reads.append(name)
                else:
                    writes.append(name)
            builders = [n for n, _, _, _ in methods.get(typ + "Builder", []) if n != "Build"]
            entry.update(ctors=made, reads=reads, writes=writes, events=events, builders=builders)
            if not made and not reads and not writes:
                continue
        else:
            made = ctors.get(typ, [])
            direct = {n for n, _, _, t in ms if t is None}
            own = []
            for name, _, _, trait in ms:
                if trait in GO_SHARED_TRAITS or name in own:
                    continue
                own.append(name)
            if kind == "element":
                own = [f for f in own if (f in GO_FIRST and f in direct) or
                       (f not in shared and not f.startswith(GO_SHARED_PREFIX))]
            entry.update(ctors=made, builders=own)
            if not made:
                continue
            for c in made:
                for p in go_split("(" + c[1] + ")", 0)[0].split(","):
                    pm = re.match(r"\s*(\w+) (\w+)$", p)
                    if pm and pm.group(2) in states:
                        states_shown.setdefault(pm.group(2), []).append(c[0])
        sections.setdefault(mod, []).append(entry)
    return sections, states_shown


def go_ctor_text(c, kind, typ):
    name, sig, _ = c
    ps = go_params(sig)
    if kind == "state":
        for p in sig.split(","):
            pm = re.match(r"\s*(\w+) RowSource$", p)
            if pm:
                if typ not in GO_SOURCES:
                    raise SystemExit(f"gk_apidoc: {typ}'s constructor {name} takes a RowSource; name the call "
                                     "that makes it in GO_SOURCES")
                ps = [GO_SOURCES[typ] if x == pm.group(1) else x for x in ps]
        call = f"gk.{name}({', '.join(ps)})"
        return call + ("….Build()" if c[2] else "")
    return f"gk.{name}({', '.join(ps)})"


def go_render(files):
    sections, states_shown = go_index(files)
    out = [GO_BEGIN, ""]
    total = {"element": 0, "value": 0, "state": 0}
    order = sorted(sections, key=lambda m: (not m.startswith("component"), m))
    for mod in order:
        entries = sorted(sections[mod], key=lambda e: ({"element": 0, "state": 1, "value": 2}[e["kind"]], e["type"]))
        out += [f"### {mod}", ""]
        for e in entries:
            total[e["kind"]] += 1
            seen, made = set(), []
            for c in sorted(e["ctors"], key=lambda c: (len(go_params(c[1])), c[0])):
                t = go_ctor_text(c, e["kind"], e["type"])
                if t not in seen:
                    seen.add(t)
                    made.append(t)
            usage = " or ".join(f"`{t}`" for t in made[:3]) + (f" (and {len(made) - 3} more)" if len(made) > 3 else "")
            if not usage:
                usage = "handed out by the kit (no constructor)"
            line = f"- **`gk.{e['type']}`** ({short(e['path'])}"
            if e["kind"] == "state":
                line += ", kit state): " + usage
                shown = states_shown.get(e["type"])
                if shown:
                    line += "; shown by " + ", ".join(f"`gk.{s}(state)`" for s in sorted(set(shown)))
                parts = []
                if e["builders"]:
                    parts.append("built with " + go_names_text(e["builders"]))
                if e["reads"]:
                    parts.append("reads " + go_names_text(e["reads"]))
                if e["writes"]:
                    parts.append("writes " + go_names_text(e["writes"]))
                if e["events"]:
                    parts.append("events " + ", ".join(f"`.{n}(cx, f)`" for n in sorted(e["events"])))
                if parts:
                    line += "; " + "; ".join(parts)
            else:
                line += ", " + ("element" if e["kind"] == "element" else "value") + "): " + usage
                if e["builders"]:
                    line += "; " + go_names_text(e["builders"])
            out.append(line)
        out.append("")
    out.insert(2, f"{total['element']} element types, {total['state']} kit state types and {total['value']} value types, "
                  f"in {len(sections)} upstream modules. Styling builders every element has (`.Bg`, `.W`, "
                  "`.P`, `.BorderColor`, … and the style tokens) are not repeated per type.")
    out.insert(3, "")
    out.append(END)
    return "\n".join(out)


def go_names_text(names):
    first = [n for n in GO_FIRST if n in names]
    names = first + sorted(n for n in names if n not in first)
    head = ", ".join(f"`.{n}`" for n in names[:SHOWN])
    more = len(names) - SHOWN
    return head + (f" and {more} more" if more > 0 else "")


def write(doc, begin, text, check):
    """Puts text between begin and END in doc; with check, only compares. 0 when current."""
    cur = open(doc).read()
    if begin not in cur or END not in cur:
        print(f"gk_apidoc: {doc} has no index markers", file=sys.stderr)
        return 1
    head, rest = cur.split(begin, 1)
    _, tail = rest.split(END, 1)
    new = head + text + tail
    name = os.path.relpath(doc, REPO)
    if check:
        if new != cur:
            print(f"gk_apidoc: the component index of {name} is stale (python3 gen/gk_apidoc.py, or make gen)")
            return 1
        print(f"gk_apidoc: {name}'s component index matches the package")
        return 0
    if new != cur:
        open(doc, "w").write(new)
    print(f"gk_apidoc: {name} index {'rewritten' if new != cur else 'unchanged'}")
    return 0


def main(argv):
    check = "--check" in argv
    return write(GO_DOC, GO_BEGIN, go_render(go_read_package()), check)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
