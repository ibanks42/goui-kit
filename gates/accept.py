#!/usr/bin/env python3
"""accept.py [--check] [--list]: builder and value types that nothing accepts.

A *value type* is a node type of the interface description (api/gk.api.json.gz, written by
`make gen`) that is not an element and has at least one usable (bound or bridged) constructor
or builder. Its values are made by the app; they are useful only if something takes them. A
value type is *accepted* when one of these takes it:

  - an argument of a usable function, at any depth (a list, an option, a reference, a tuple, a
    build closure's node), or a field of a record, a variant of an enum with data or a member
    of an Into set that such an argument reaches;
  - the target of a usable conversion (`converts`) of another node;
  - a procedure of the runtime that takes the value (RUNTIME below: `gk.table_columns` takes
    `Column`, `gk.plot` takes every type with a `paint`, `gk.action_of` takes every type that
    implements Action, listed by the generator in the description's `values.actions`).

Every value type that is not accepted is counted with its usable functions, and must have a
stated reason in REASONS (why nothing can take it today, and what would). Run it to see
the current count.

    accept.py           print the count and write gates/out/accept.md (types, functions, reasons)
    accept.py --list    also print every type left, with its reason
    accept.py --check   exit 1 when a type left has no reason
Run by `make gen` after the generator. Standard library only."""
import gzip
import json
import os
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
API = os.path.join(REPO, "api", "gk.api.json.gz")
OUT = os.path.join(REPO, "gates", "out", "accept.md")
USABLE = ("bound", "bridged")

# Procedures of the runtime that take a value type (the description cannot see them).
RUNTIME = {
    "gpui_kit::component::table::Column": "gk.table_columns (kit_rows.odin, PROTOCOLS.md 13.6)",
    # animations and motion values (rt_motion.odin, PROTOCOLS.md 13.30)
    "gpui_kit::base::Presence": "gk.motion(el, {prop, gk.At_Presence{presence, from, to}}) (rt_motion.odin, PROTOCOLS.md 13.30)",
    "gpui_kit::base::animation::EffectTransition": "gk.effect(el, effect, id): EffectTransition::apply (rt_motion.odin, PROTOCOLS.md 13.30)",
    "gpui_kit::base::Timing": "gk.motion(el, {prop, gk.At_Keyframes{id, keyframes, timing}}) (rt_motion.odin, PROTOCOLS.md 13.30)",
    "gpui_kit::base::Transition": "gk.motion(el, {prop, gk.To_Transition{id, target, policy}}) (rt_motion.odin, PROTOCOLS.md 13.30)",
    "gpui_kit::base::Spring": "gk.motion(el, {prop, gk.To_Spring{id, target, policy}}) (rt_motion.odin, PROTOCOLS.md 13.30)",
}

# Why a value type left is not accepted, by the upstream path of the type or by a prefix
# ending in "::" (a module). A type left without one fails --check.
TOKENS = ("theme tokens: the parts of the semantic token set (`SemanticThemeTokens`) that upstream builds "
          "from a theme file; no public function takes one, a Rust app reads them from the theme")
CONFIG = ("theme file: upstream deserializes it from JSON (`ThemeRegistry::load_themes_from_str`, bound) and "
          "takes it only inside the registry; no public function takes one")
EDITOR = ("taken only by the text and editor host's traits the app implements or by functions of the editor "
          "family not bound yet (PROTOCOLS.md 9.10 to 9.12)")
REASONS = {
    "gpui_kit::base::CalendarItemState": "handed to `Calendar::item`'s render closure (skipped: item-render-closure); nothing else takes it",
    "gpui_kit::base::ColorTokens": TOKENS, "gpui_kit::base::RadiusTokens": TOKENS, "gpui_kit::base::ShadowTokens": TOKENS,
    "gpui_kit::base::SpacingTokens": TOKENS, "gpui_kit::base::TypographyTokens": TOKENS,
    "gpui_kit::base::TextStyleToken": TOKENS, "gpui_kit::component::ThemeToken": TOKENS,
    "gpui_kit::component::SemanticColorConfig": CONFIG, "gpui_kit::component::SemanticRadiusConfig": CONFIG,
    "gpui_kit::component::SemanticShadowConfig": CONFIG, "gpui_kit::component::SemanticSpacingConfig": CONFIG,
    "gpui_kit::component::SemanticTextStyleConfig": CONFIG, "gpui_kit::component::SemanticThemeConfigFile": CONFIG,
    "gpui_kit::component::SemanticTypographyConfig": CONFIG, "gpui_kit::component::ThemeConfigColors": CONFIG,
    "gpui_kit::component::ThemeSet": CONFIG, "gpui_kit::component::highlighter::HighlightThemeStyle": CONFIG,
    "gpui_kit::base::IndexPath": ("a plain record too: where a call takes it, it travels as the record "
                                  "`gk.Base_Index_Path{row = ..}`, or the row alone (PROTOCOLS.md 13.6, 11.36); the builder form "
                                  "makes the same value"),
    "gpui_kit::base::InlineElement": EDITOR, "gpui_kit::base::MarkdownNode": EDITOR,
    "gpui_kit::base::text::": EDITOR, "gpui_kit::base::input::": EDITOR,
    "gpui_kit::component::text::FrontmatterPlugin": EDITOR,
    "gpui_kit::base::Measure": "upstream's own layout probe: no public function takes one",
    "gpui_kit::base::TextSelectionContentKey": ("text selection: taken by `TextSelectionHandle::register` and "
                                                "`update_runs`, whose object nothing bound hands out"),
    "gpui_kit::base::TextSelectionRegistration": ("text selection: taken by `TextSelectionHandle::register`, whose "
                                                  "object nothing bound hands out"),
    "gpui_kit::base::TextSelectionRun": ("text selection: taken by `TextSelectionHandle::update_runs`, whose object "
                                         "nothing bound hands out"),
    "gpui_kit::base::TooltipRequest": "taken by `TooltipOverlay::request_show`, whose object nothing bound hands out",
    "gpui_kit::base::TreeEntryState": "handed to the tree's item render closure (rows family shim); nothing takes one",
    "gpui_kit::base::dock::": "taken by the dock's own layout (DockArea), the (PROTOCOLS.md 11.7)",
    "gpui_kit::base::questionnaire::": "the questionnaire's definitions, taken by the questionnaire states",
    "gpui_kit::base::plot::TooltipState": "taken by `Plot::tooltip`, a method of the Plot trait the library implements once (GkPlot)",
    "gpui_kit::base::plot::label::Text": ("plot labels: taken by `PlotLabel::new(Vec<Text>)` and returned by the "
                                          "`Bar::label` and `SankeyChart::labels` closures; PlotLabel is not a plot part yet"),
    "gpui_kit::base::plot::shape::Arc": ("`Arc::paint(&ArcData<'_, T>, ..)` paints one arc of a pie the app computed "
                                         "with `Pie<T>::arcs`, which borrows the rows; not a plot part (no `&Bounds` paint)"),
    "gpui_kit::base::plot::shape::Sankey": ("the sankey layout calculator: `layout` returns a graph that borrows "
                                            "its input, which no result can carry"),
    "gpui_kit::base::plot::shape::SankeyNodeLayout": "taken by `sankey_link_path`, a free function over borrowed layouts",
    "gpui_kit::component::diff::DiffParseOptions": "taken by `DiffFile::parse_with`, a static of the diff family not bound yet",
    "gpui_kit::component::list::ListSeparatorItem": "upstream's own list rendering; no public function takes one",
    "gpui_kit::component::menu::ContextMenuState": "upstream's own context-menu element state; no public function takes one",
    "gpui_kit::component::AnyChildElement": ("the table parts' own type-erased child: upstream keeps it in private fields "
                                             "(`Table::child` wraps its argument itself) and no public function takes one; "
                                             "the binding's `Table_Child` takes every ChildElement directly"),
    # (PROTOCOLS.md 9.28): what is still taken by nothing, and what would take it.
    "gpui_kit::component::setting::RenderOptions": ("handed by upstream to a settings field's render closure, which "
                                                    "the binding serves with a fragment of the app's view (PROTOCOLS.md "
                                                    "13.19); the app wrote the options it would read, so no function "
                                                    "of the binding takes one"),
}


def walk(d, found, desc):
    """Node keys a descriptor names, following records, data enums and Into sets."""
    if isinstance(d, dict):
        for v in d.values():
            walk(v, found, desc)
        return
    if not isinstance(d, list) or not d:
        return
    head = d[0]
    if head == "node" and len(d) > 1 and isinstance(d[1], str):
        found.add(d[1])
    elif head == "build" and len(d) > 1 and isinstance(d[1], str):
        found.add(d[1])
    elif head in ("traitval", "oneof", "dragstyle") and len(d) > 2:
        # one of a kit trait's implementors (PROTOCOLS.md 13.19, 9.12), a drag-over style (13.20)
        found.update(x for x in (d[2] if isinstance(d[2], list) else [d[1]]) if isinstance(x, str))
        if head == "dragstyle":
            found.add(d[1])
    elif head in ("rec", "denum", "into") and len(d) > 1 and isinstance(d[1], str):
        key = (head, d[1])
        if key not in desc["seen"]:
            desc["seen"].add(key)
            table = {"rec": desc["records"], "denum": desc["data_enums"], "into": desc["into"]}[head]
            walk(table.get(d[1], {}), found, desc)
    # A list whose head is a tag (a string) is a descriptor: its operands follow the tag. Any other
    # list (a record's or a variant's fields, a data enum's variants, a tuple's members) is plain
    # data and every element counts, the first included.
    for x in (d[1:] if isinstance(head, str) else d):
        walk(x, found, desc)


def count(d):
    items = {it["id"]: it for it in d["items"]}
    usable = lambda i: items.get(i, {}).get("status") in USABLE  # noqa: E731
    ctx = {"seen": set(), "records": d["records"], "data_enums": d["data_enums"], "into": d["into"]}
    taken = set(RUNTIME)
    # The kit's action types: gk.action_of makes an Action of any of them (PROTOCOLS.md 13.11).
    taken |= set(d.get("values", {}).get("actions", []))
    for it in d["items"]:
        if it.get("status") not in USABLE:
            continue
        for a in it.get("args", []):
            walk(a.get("desc"), taken, ctx)
        if it.get("kind") == "paint" and it.get("node"):
            taken.add(it["node"])  # gk.plot's parts (PROTOCOLS.md 12.6)
    for n in d["nodes"].values():
        for cv in n.get("converts", []):
            if usable(cv):
                walk(items[cv].get("ret"), taken, ctx)
    left = []
    for key, n in sorted(d["nodes"].items()):
        if n["element"]:
            continue
        fns = [i for i in n["ctors"] + n["ops"] if usable(i)]
        if not fns or key in taken or n["path"] in taken:
            continue
        reason = REASONS.get(n["path"]) or next((v for k, v in REASONS.items() if k.endswith("::") and n["path"].startswith(k)), "")
        left.append({"type": key, "functions": len(fns), "reason": reason})
    return left


def main(argv):
    d = json.load(gzip.open(API))
    left = count(d)
    fns = sum(x["functions"] for x in left)
    noreason = [x for x in left if not x["reason"]]
    print(f"accept: {len(left)} value types with {fns} usable functions that nothing accepts; "
          f"{len(noreason)} without a stated reason")
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w") as f:
        f.write("# Values nothing accepts\n\nWritten by `gates/accept.py` from `api/gk.api.json.gz`.\n\n")
        f.write(f"{len(left)} types, {fns} usable functions, {len(noreason)} without a reason.\n\n")
        f.write("| type | usable functions | reason |\n|---|---|---|\n")
        for x in left:
            f.write(f"| `{x['type']}` | {x['functions']} | {x['reason'] or '(none)'} |\n")
    if "--list" in argv:
        for x in left:
            print(f"  {x['type']} ({x['functions']}): {x['reason'] or '(no reason)'}")
    if "--check" in argv and noreason:
        sys.exit(1)


if __name__ == "__main__":
    main(sys.argv[1:])
