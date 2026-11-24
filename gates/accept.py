#!/usr/bin/env python3
"""Audit non-element value types in api/gk.api.json.gz that nothing accepts as input.

What it gates:
  Ensures bound or bridged value types (non-element structs/builders) are not orphaned: every
  such type must either be accepted by some usable API surface or have an explicit explanation
  in REASONS.

How it measures:
  1. Seeds the `taken` set with hand-wired runtime types (RUNTIME) and registered action types.
  2. Walks every argument descriptor of every usable (`bound` or `bridged`) item, every paint
     op's node, and every usable conversion's return descriptor. Expands nested `rec`, `denum`,
     and `into` definitions recursively while tracking visited keys to avoid cycles.
  3. Lists every non-element node with at least one usable constructor or method that never
     appears in `taken`, matching each against exact keys or `::` module prefixes in REASONS.
  4. Writes the full table to `gates/out/accept.md` (and prints each item if `--list` is given).

What PASS means (`--check`, exit 0):
  Every unaccepted non-element value type with usable functions has a documented reason in
  REASONS (zero unexplained orphans). Exits 1 if any lack a reason.
"""
import gzip
import json
import os
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
API = os.path.join(REPO, "api", "gk.api.json.gz")
OUT = os.path.join(REPO, "gates", "out", "accept.md")
USABLE = ("bound", "bridged")

# Value types accepted by hand-written runtime functions rather than generated descriptors.
RUNTIME = {
    "gpui_kit::component::table::Column": "table.SetTableKitColumns (rt_table.go, PROTOCOLS.md 13.6)",
    "gpui_kit::base::Presence": "gk.Motion(el, gk.MotionValue{prop, gk.AtPresence{presence, from, to}}) (rt_motion.go, PROTOCOLS.md 13.30)",
    "gpui_kit::base::animation::EffectTransition": "gk.Effect(el, effect, id): EffectTransition::apply (rt_kit.go, PROTOCOLS.md 13.30)",
    "gpui_kit::base::Timing": "gk.Motion(el, gk.MotionValue{prop, gk.AtKeyframes{id, keyframes, timing}}) (rt_motion.go, PROTOCOLS.md 13.30)",
    "gpui_kit::base::Transition": "gk.Motion(el, gk.MotionValue{prop, gk.ToTransition{id, target, policy}}) (rt_motion.go, PROTOCOLS.md 13.30)",
    "gpui_kit::base::Spring": "gk.Motion(el, gk.MotionValue{prop, gk.ToSpring{id, target, policy}}) (rt_motion.go, PROTOCOLS.md 13.30)",
}

# Documented reasons for non-element value types that no usable function takes (keys ending in `::` match prefixes).
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
    "gpui_kit::component::setting::RenderOptions": ("handed by upstream to a settings field's render closure, which "
                                                    "the binding serves with a fragment of the app's view (PROTOCOLS.md "
                                                    "13.19); the app wrote the options it would read, so no function "
                                                    "of the binding takes one"),
}

def walk(d, found, desc):
    # Recursively collect every referenced value/node type, following records, data enums, and into conversions once.
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
        found.update(x for x in (d[2] if isinstance(d[2], list) else [d[1]]) if isinstance(x, str))
        if head == "dragstyle":
            found.add(d[1])
    elif head in ("rec", "denum", "into") and len(d) > 1 and isinstance(d[1], str):
        key = (head, d[1])
        if key not in desc["seen"]:
            desc["seen"].add(key)
            table = {"rec": desc["records"], "denum": desc["data_enums"], "into": desc["into"]}[head]
            walk(table.get(d[1], {}), found, desc)
    for x in (d[1:] if isinstance(head, str) else d):
        walk(x, found, desc)

def count(d):
    """Collect all accepted value types and return unaccepted non-element nodes with reasons."""
    items = {it["id"]: it for it in d["items"]}
    usable = lambda i: items.get(i, {}).get("status") in USABLE  # noqa: E731
    ctx = {"seen": set(), "records": d["records"], "data_enums": d["data_enums"], "into": d["into"]}
    taken = set(RUNTIME)
    taken |= set(d.get("values", {}).get("actions", []))
    # Record every type reachable from arguments of usable items and paint ops.
    for it in d["items"]:
        if it.get("status") not in USABLE:
            continue
        for a in it.get("args", []):
            walk(a.get("desc"), taken, ctx)
        if it.get("kind") == "paint" and it.get("node"):
            taken.add(it["node"])
    # Also mark targets produced by usable conversion helpers on nodes.
    for n in d["nodes"].values():
        for cv in n.get("converts", []):
            if usable(cv):
                walk(items[cv].get("ret"), taken, ctx)
    # Report non-element nodes that expose usable constructors/ops yet never appeared in `taken`.
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
