"""gk_go_kinds: the Go emitter's rules for the generator's descriptor kinds beyond the core ones
(docs/GO.md; the Odin side is gk_values, gk_lists, gk_comp6, gk_items, gk_editor and gk_app's
odin_type / odin_put). gk_go.Go calls the three hooks first (gtype, put, rd: None when the kind is
not one of these) and emit_files once every item was written, before the types.

Every writer here writes the bytes the Odin writer writes (most by reflection there, by hand in
the runtime here: go/gk/rt_gen_*.go). What the hooks need from the runtime:

- values: CivilDate/CivilTime/CivilDateTime/Weekday, Transformation, TimedTransformation, Font,
  Fallible[T], Cell[T], Decorate, MarkdownPlugin(Any), EntityID, QuestionnaireRules,
  ExternalDragPayload, A11yChild, DragMoveEvent, ImageCache(Provider), ScrollAnchor, and the
  library values of LIBVALS;
- closures: item functions (gk.Items over the kinds this module numbers, gen_items.go) and asks
  (gk.NewAsk, whose key and value codecs gen_items.go registers).
"""
import re

from gk_model import ada

# Library values (`["libval", rust path, odin name]`) the Go runtime declares (rt_gen_values.go):
# Odin name -> (Go type, has a put function, has a rd function).
LIBVALS = {"Lsp_Position": ("LspPosition", True, True), "Completion_Item": ("CompletionItem", True, True),
           "Code_Block": ("CodeBlock", True, True), "Search_Item": ("SearchItem", True, True),
           "Arc_Data": ("ArcData", False, True), "Stack_Series": ("StackSeries", False, True),
           "Questionnaire_Validator": ("QuestionnaireValidator", False, True),
           "Background_Value": ("BackgroundValue", False, True),
           "Interval_Matcher": ("IntervalMatcher", True, True), "Range_Matcher": ("RangeMatcher", True, True)}
CIVIL = {"Date": "CivilDate", "Time": "CivilTime", "DateTime": "CivilDateTime", "Weekday": "Weekday"}
# Simple values: kind -> (Go type, put function).
SIMPLE = {"xform": ("Transformation", "putTransformation"), "txform": ("TimedTransformation", "putTimedTransformation"),
          "font": ("Font", "putFont"), "qrules": ("QuestionnaireRules", "putQuestionnaireRules"),
          "dragpayload": ("ExternalDragPayload", "putExternalDragPayload"),
          "imagecache": ("ImageCacheProvider", "putImageCacheProvider"), "decorate": ("Decorate", "putDecorate")}


def _unsup():
    import gk_go  # noqa: PLC0415
    return gk_go.Unsup


def _camel(name):
    import gk_go  # noqa: PLC0415
    return gk_go.camel(name)


def _state(go):
    st = go.__dict__.get("kinds_state")
    if st is None:
        st = go.kinds_state = {"items": {}, "asks": {}, "mdplug": {}, "traitval": {}, "kitfn": {}}
    return st


# ---------------------------------------------------------------- types

def gtype(go, d, out=False):
    m, k = go.m, d[0]
    if k == "civil":
        return CIVIL[d[1]]
    if k in SIMPLE:
        return SIMPLE[k][0]
    if k == "fallible":
        return f"Fallible[{go.gtype(d[1], True)}]"
    if k == "cell":
        return f"Cell[{go.gtype(d[1])}]"
    if k == "expr":
        return "string"
    if k == "imgsrc":
        return "string"
    if k == "entityid":
        return "EntityID"
    if k in ("valset", "table", "rowvals", "keyset"):
        return "[]" + go.gtype(d[1])
    if k == "prepaintorder":
        return "[]int"
    if k == "a11ychildren":
        return "[]A11yChild"
    if k == "imagecacheref":
        return "ImageCache"
    if k == "scrollanchor":
        return "ScrollAnchor"
    if k == "leaf" and len(d) > 1 and d[1] == "drag_move_event":
        return "DragMoveEvent"
    if k == "libval" and d[2] in LIBVALS:
        return LIBVALS[d[2]][0]
    if k == "lentmut":
        return _node_go(go, d[1][1])
    if k == "tfrag":
        return _node_go(go, d[1])
    if k == "mdplug":
        _state(go)["mdplug"][d[1]] = [x[1] for x in d[2:]]
        return "MarkdownPluginAny"
    if k == "traitval":
        import gk_values  # noqa: PLC0415
        name = _camel(gk_values.TRAIT_VALUES[d[1]][1])
        _state(go)["traitval"][name] = d[2]
        return name
    if k == "kitfn":
        name = _camel(ada(d[2]))
        _state(go)["kitfn"][name] = d[3]
        item_kind(go, d[4])  # its rows: an item function over the closure's values
        return name
    if k in ("ask", "askdyn"):  # askdyn: a one-method trait object served by an ask (PROTOCOLS.md 9.26)
        K, V = go.gtype(d[1], True), go.gtype(d[2])
        return f"Ask[{K}, {V}]"
    if k in ("ctxrows", "items", "ctxlazy", "viewrows"):
        item_kind(go, d)
        return "RowSource"
    if k == "lent":
        t = go.gtype(d[1], True)
        return f"Option[{t}]" if d[3] else t
    if k == "uscroll":
        return go.gtype(d[1], out)
    if k == "held":
        return go.m.denums[d[1]]["go"] + "Held"
    return None


def _node_go(go, key):
    node = go.m.nodes.get(key, {})
    if "go" not in node:
        raise _unsup()(f"node {key} has no Go type")
    return node["go"]


# ---------------------------------------------------------------- writers

def put(go, d, v, depth=0):
    m, k = go.m, d[0]
    if k == "civil":
        return [f"put{CIVIL[d[1]]}(o, {v})"]
    if k in SIMPLE:
        return [f"{SIMPLE[k][1]}(o, {v})"]
    if k == "cell":
        go.gtype(d)
        return [f"o.u64({v}.h)"]
    if k == "expr":
        return [f"putExpr(o, {v}, {len(d[2])})"]
    if k == "imgsrc":
        return [f"o.str({v})"]
    if k == "entityid":
        return [f"o.u64(uint64({v}))"]
    if k in ("valset", "table", "rowvals", "keyset"):
        return go.put(["list", d[1], "vec"], v, depth)
    if k == "prepaintorder":
        return [f"o.u32(uint32(len({v})))", f"for _, x := range {v} {{", "\to.u64(uint64(x))", "}"]
    if k == "a11ychildren":
        x = f"x{depth}"
        return ([f"o.u32(uint32(len({v})))", f"for _, {x} := range {v} {{", f"\to.str({x}.Key)"]
                + ["\t" + s for s in go.put(d[2], f"{x}.Role", depth + 1)] + [f"\to.str({x}.Label)", "}"])
    if k in ("imagecacheref", "scrollanchor"):
        return [f"o.u64({v}.h)"]
    if k == "libval" and d[2] in LIBVALS:
        if not LIBVALS[d[2]][1]:
            raise _unsup()(f"libval {d[2]} only travels out")
        return [f"put{LIBVALS[d[2]][0]}(o, {v})"]
    if k == "lentmut":
        go.gtype(d)
        return [f"putVtape(o, {v}.n)"]
    if k == "tfrag":
        go.gtype(d)
        return [f"o.u32(takeNode({v}.n, false))"]
    if k == "mdplug":
        go.gtype(d)
        out = [f"switch x := {v}.(type) {{"]
        for n, (_, key) in enumerate(d[2:]):
            out += [f"case {_node_go(go, key)}:", f"\to.u8({n})", "\to.u32(takeValue(x.n))"]
        return out + ["case MarkdownPlugin:", f"\to.u8({len(d) - 2})", "\tputMarkdownPlugin(o, x)",
                      "default:", f"\to.u8({len(d) - 2})", "\tputMarkdownPlugin(o, MarkdownPlugin{})", "}"]
    if k == "traitval":
        t = go.gtype(d)
        out = [f"switch x := {v}.(type) {{"]
        for key in d[2]:
            out += [f"case {_node_go(go, key)}:", "\to.u32(takeValue(x.n))"]
        return out + ["default:", f'\tstop("a nil gk.{t} was given to a builder: make it with one of its constructors before giving it")', "}"]
    if k == "kitfn":
        t = go.gtype(d)
        return [f"switch x := {v}.(type) {{", "case RowSource:", "\to.u8(0)", "\to.u64(x.h)",
                f"case {t}Kit:", "\to.u8(1)", "\to.u16(uint16(x))",
                "default:", f'\tstop("a nil gk.{t} was given to a builder: give a gk.RowSource (gk.Items) or one of the gk.{t}Kit values")', "}"]
    if k in ("ask", "askdyn"):
        K, V = go.gtype(d[1], True), go.gtype(d[2])
        asks = _state(go)["asks"]
        if (K, V) not in asks:
            asks[(K, V)] = (go.rd(d[1], True), go.put(d[2], "v"))
        return [f"o.u64({v}.Src.h)"]
    if k in ("ctxrows", "items"):
        n = item_kind(go, d)
        return [f"o.u64(itemsCheck({v}, {n}))"]
    if k in ("ctxlazy", "viewrows"):
        item_kind(go, d)
        return [f"o.u64({v}.h)"]
    if k == "held":
        return [f"o.u64({v}.h)" if go.in_call else f"putObj(o, {v}.h)"]
    return None


# ---------------------------------------------------------------- readers

def rd(go, d, out=True):
    m, k = go.m, d[0]
    if k == "civil":
        return f"rd{CIVIL[d[1]]}(r)"
    if k == "fallible":
        return f"rdFallible(r, func(r *reader) {go.gtype(d[1], True)} {{ return {go.rd(d[1])} }})"
    if k == "leaf" and len(d) > 1 and d[1] == "drag_move_event":
        bounds = next((rt for rt, rc in m.records.items() if rc.get("go") == "Bounds"), None)
        if bounds is None:
            raise _unsup()("drag_move_event without gk.Bounds")
        go.need_readers.add(("rec", bounds))
        return "rdDragMoveEvent(r)"
    if k == "libval" and d[2] in LIBVALS:
        return f"rd{LIBVALS[d[2]][0]}(r)"
    if k == "valset":
        # a set of values read back as the list it was made of (core/src/values.rs value_set's Wire)
        return go.rd(["list", d[1], "vec"], out)
    if k == "lent":
        if d[3]:
            return f"rdOpt(r, func(r *reader) {go.gtype(d[1], True)} {{ return {go.rd(d[1])} }})"
        return go.rd(d[1])
    if k == "uscroll":
        return go.rd(d[1], out)
    if k == "imgsrc":
        return "r.str()"
    if k == "held":
        return f"{gtype(go, d)}{{h: r.u64()}}"
    return None


def denum_held(go, e, puts):
    """A held value of an enum with data the app gives (`BlockNodeHeld`, PROTOCOLS.md 9.30): a
    library object's handle, one more variant of the enum's interface (gk_editor.odin_denum)."""
    import gk_editor  # noqa: PLC0415
    if not e.get("held") or e["out"]:
        return
    E = e["go"]
    V = E + "Held"
    go.add(f"// {V} is a {e['name']} the library holds (a function's result, PROTOCOLS.md 9.30): temporary until",
           "// gk.Retain, as any object a call hands out.", f"type {V} struct{{ h uint64 }}", "",
           f"func (v {V}) handle() uint64 {{ return v.h }}", "", f"func ({V}) is{E}() {{}}", "")
    puts.append((V, [f"o.u16({gk_editor.HELD_TAG:#x})", "putObj(o, x.h)"]))


# ---------------------------------------------------------------- item functions (9.16, 9.22, 9.41)

def _as_items(d):
    """An item descriptor of every closure gk.Items serves: items, and the element closures over a
    context (ctxrows, ctxlazy, viewrows) with their values as `val`s."""
    if d[0] == "items":
        return d
    if d[0] in ("ctxrows", "ctxlazy", "viewrows"):
        return ["items", d[1], d[2], d[3], [["val", x] for x in d[4]], "el"]
    raise KeyError(d[0])


def _inline_plugin_kind(go):
    """The kind of an inline data plugin's row (PROTOCOLS.md 9.23): the node and the inherited
    typography (Odin's Markdown_Inline, read with gk.row_context), as gk.Items's function
    func(cx, gk.MarkdownNodeRead, gk.InlineRenderContext) gk.Element. A block plugin's row is the
    MarkdownNodeRead kind a closure of the kit already calls."""
    keys = {}
    for key, st in go.m.states.items():
        if st.get("go") in ("MarkdownNodeRead", "InlineRenderContext"):
            keys[st["go"]] = key
    if len(keys) < 2:
        raise _unsup()("the inline plugin's row needs gk.MarkdownNodeRead and gk.InlineRenderContext")
    vals = [["val", ["ref", ["obj", None, keys[n], True]]] for n in ("MarkdownNodeRead", "InlineRenderContext")]
    item_kind(go, ["items", "impl", [], [], vals, "el"])


def item_kind(go, d):
    """The number of the kind of item function a closure calls (numbered in order of first use),
    and its Go function type, its value reads and the handed elements it makes."""
    d = _as_items(d)
    m = go.m
    ps, body, args = [], [], []
    hi = 0
    reads = False
    for n, v in enumerate(d[4]):
        if v[0] == "val":
            t = go.gtype(v[1], True)
            ps.append(t)
            body.append(f"v{n} := {go.rd(v[1])}")
            args.append(f"v{n}")
            reads = True
        elif v[0] == "reads":
            t = m.records[v[1]].get("go")
            if t is None:
                raise _unsup()(f"reads record {v[1]} without a Go type")
            ps.append(t)
            body.append(f"v{n} := {go.rd(['rec', v[1]])}")
            args.append(f"v{n}")
            reads = True
        elif v[0] == "handed":
            node = m.nodes[v[1]]
            ctor = next((i for i in node["ctors"] if i.get("handed") and "go" in i), None)
            if ctor is None or "go" not in node:
                raise _unsup()(f"handed {v[1]} without a Go constructor")
            ps.append(node["go"])
            body.append(f"h{n} := {ctor['go']}({hi})")
            args.append(f"h{n}")
            hi += 1
            if v[2]:
                t = m.records[v[2]].get("go")
                if t is None:
                    raise _unsup()(f"reads record {v[2]} without a Go type")
                ps.append(t)
                body.append(f"w{n} := {go.rd(['rec', v[2]])}")
                args.append(f"w{n}")
                reads = True
        elif v[0] == "view":
            ps.append("KitView")
            body.append(f"h{n} := kitView({hi})")
            args.append(f"h{n}")
            hi += 1
    ret = "Element" if d[5] == "el" else _node_go(go, d[5][1])
    ty = "func(cx *Cx" + "".join(f", v{i} {t}" for i, t in enumerate(ps)) + f") {ret}"
    kinds = _state(go)["items"]
    if ty not in kinds:
        if reads:
            body.insert(0, "r := rowContext(cx, src, at)")
        kinds[ty] = (len(kinds) + 1, body, args)
    return kinds[ty][0]


# ---------------------------------------------------------------- files

def emit_files(go):
    """gen_items.go: gk.Items over the kinds of item functions, the asks' codecs, and the marker
    methods of the interfaces this module's kinds name."""
    st = _state(go)
    # the asks a record's field takes (InputEditorStyle's HighlightStyles): gen_types.go is written
    # after this file, so their codecs are registered here
    for rc in go.m.records.values():
        for f in rc["fields"]:
            if rc["in"] and f["desc"][0] in ("ask", "askdyn"):
                try:
                    go.put(f["desc"], "v")
                except _unsup():
                    pass
    if st["mdplug"]:
        _inline_plugin_kind(go)  # numbered last: the kinds the closures call keep their numbers
    go.open("gen_items.go", "Item functions (docs/PROTOCOLS.md 9.41): gk.Items, one case per kind of function; "
            "the asks' codecs (9.14); the kit's values of a few closures.")
    kinds = sorted(st["items"].items(), key=lambda kv: kv[1][0])
    go.add("// ItemFunc is a function that describes one item of a component: one of the kinds of function a",
           "// closure of the kit calls (a builder that takes a gk.RowSource made by gk.Items names its kind).")
    if kinds:
        go.add("type ItemFunc interface {", "\t" + " |\n\t\t".join(ty for ty, _ in kinds), "}", "")
    else:
        go.add("type ItemFunc interface{ func(cx *Cx) Element }", "")
    go.add("var itemsKinds = []string{\"\", " + ", ".join(f'"{ty}"' for ty, _ in kinds) + "}", "")
    go.add("// Items makes the row source of an item function: a function that describes one item of a component",
           "// (a calendar's day, a tree's entry, a popover's content, an input's token), called between frames with",
           "// the values the kit hands that item. The same function given in the same view gives the same source;",
           "// it may be called in a view function.",
           "func Items[F ItemFunc](cx *Cx, f F) RowSource {", "\tswitch x := any(f).(type) {")
    for ty, (n, body, args) in kinds:
        go.add(f"\tcase {ty}:", f"\t\treturn itemsSource(cx, x, {n}, func(f any, cx *Cx, src RowSource, at RowAt) Element {{")
        go.add(*["\t\t\t" + b for b in body])
        go.add(f"\t\t\treturn f.({ty})({', '.join(['cx'] + args)})", "\t\t})")
    go.add("\t}", '\tstop("gk.Items was given a function of no kind a closure of the kit calls")', "\treturn RowSource{}", "}", "")
    if st["asks"]:
        go.add("func init() {")
        for (K, V), (rdx, puts) in sorted(st["asks"].items()):
            go.add(f"\tregAsk(func(r *reader) {K} {{ return {rdx} }}, func(o *tape, v {V}) {{")
            go.add(*["\t\t" + s for s in puts])
            go.add("\t})")
        go.add("}", "")
    for _, keys in sorted(st["mdplug"].items()):
        for key in keys:
            go.add(f"func ({_node_go(go, key)}) isMarkdownPluginAny() {{}}")
        go.add("")
    for name, keys in sorted(st["traitval"].items()):
        go.add(f"// {name} is one of the kit's values of this kind: " + ", ".join(_node_go(go, k) for k in keys) + ".",
               f"type {name} interface{{ is{name}() }}", "")
        for key in keys:
            go.add(f"func ({_node_go(go, key)}) is{name}() {{}}")
        go.add("")
    for name, fns in sorted(st["kitfn"].items()):
        go.add(f"// {name} is the app's rows (a gk.RowSource, made with gk.Items) or a renderer the kit makes ({name}Kit).",
               f"type {name} interface{{ is{name}() }}", "", f"func (RowSource) is{name}() {{}}", "",
               f"// {name}Kit is a renderer the kit makes.", f"type {name}Kit uint16", "", f"func ({name}Kit) is{name}() {{}}", "", "const (")
        for i, fn in enumerate(fns):
            go.add(f"\t{name}Kit{_camel(ada(fn.rsplit('::', 1)[-1]))} {name}Kit = {i} // {fn}")
        go.add(")", "")
    go.close()


_ = re
