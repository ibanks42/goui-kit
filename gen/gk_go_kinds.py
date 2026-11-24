"""Extended Go descriptor codecs and `gen_items.go` emission.

`gen/gk_go.py` delegates non-core descriptor tags to this module so the base emitter
stays focused on primitives, structs, enums, nodes, and calls. This module handles:
- `civil` (`jiff::civil` dates/times), `SIMPLE` input-only wire structs (`xform`,
  `txform`, `font`, `qrules`, `dragpayload`, `imagecache`, `decorate`), and `LIBVALS`
  (hand-wired runtime structs with per-type `(go_name, writable, readable)` flags).
- `fallible` (`Fallible[T]` / Rust `Result<T, E>`), `cell` (`Cell[T]` reactive values),
  `expr` (parsed expressions with variable count), `imgsrc`, `entityid`, `valset` /
  `table` / `rowvals` / `keyset`, `prepaintorder`, `a11ychildren`, `imagecacheref`,
  `scrollanchor`, `lent` / `lentmut` / `tfrag` / `uscroll`.
- `held` recursive data-enum handles (`denum_held`, tagged with `gk_editor.HELD_TAG`
  when written back to Rust).
- `ask` / `askdyn` (`Ask[K, V]` query callbacks whose `(K, V)` wire codecs are
  registered in `init()` via `regAsk`).
- `items` / `ctxrows` / `ctxlazy` / `viewrows` (per-item render callbacks aggregated into
  the `ItemFunc` type constraint and the `Items[F ItemFunc]` switch dispatcher).
- Marker union interfaces: `MarkdownPluginAny` (`mdplug`), type-erased trait values
  (`traitval`), and `kitfn` built-in-or-custom renderer unions.

Per-emitter state (`go.kinds_state`, returned by `_state(go)`) accumulates every
referenced `items` callback signature, `ask` codec pair, `mdplug` implementor,
`traitval` implementor, and `kitfn` union during the flat-package pass, and
`emit_files(go)` flushes them into `gen_items.go` at the end.
"""
import re

from gk_model import ada

# Maps Rust `libval` stems (from gk_editor/gk_comp6) to `(go_type_name, writable, readable)`.
# Types with writable=False can only appear in outbound replies or callbacks; trying to
# encode them in `put()` raises Unsup so callers cannot accidentally construct one.
LIBVALS = {"Lsp_Position": ("LspPosition", True, True), "Completion_Item": ("CompletionItem", True, True),
           "Code_Block": ("CodeBlock", True, True), "Search_Item": ("SearchItem", True, True),
           "Arc_Data": ("ArcData", False, True), "Stack_Series": ("StackSeries", False, True),
           "Questionnaire_Validator": ("QuestionnaireValidator", False, True),
           "Background_Value": ("BackgroundValue", False, True),
           "Interval_Matcher": ("IntervalMatcher", True, True), "Range_Matcher": ("RangeMatcher", True, True)}
# `jiff::civil` calendar/time types wired via hand-written `putCivil*` / `rdCivil*` helpers in internal/rt.
CIVIL = {"Date": "CivilDate", "Time": "CivilTime", "DateTime": "CivilDateTime", "Weekday": "Weekday"}
# Input-only descriptors that map 1-to-1 to a Go struct type and a single tape-writer helper.
SIMPLE = {"xform": ("Transformation", "putTransformation"), "txform": ("TimedTransformation", "putTimedTransformation"),
          "font": ("Font", "putFont"), "qrules": ("QuestionnaireRules", "putQuestionnaireRules"),
          "dragpayload": ("ExternalDragPayload", "putExternalDragPayload"),
          "imagecache": ("ImageCacheProvider", "putImageCacheProvider"), "decorate": ("Decorate", "putDecorate")}


def _unsup():
    import gk_go
    return gk_go.Unsup


def _camel(name):
    import gk_go
    return gk_go.camel(name)


def _state(go):
    """Return the per-`Go` accumulator dict for items, asks, mdplug, traitval, and kitfn declarations.

    Stored directly on the `Go` emitter instance so multiple runs do not share global state:
    - `items`: `{go_func_sig: (kind_id, decoder_stmts, call_args)}`
    - `asks`: `{(K_go, V_go): (key_reader_expr, val_put_stmts)}`
    - `mdplug`, `traitval`, `kitfn`: union name -> implementor node keys or built-in fn names.
    """
    st = go.__dict__.get("kinds_state")
    if st is None:
        st = go.kinds_state = {"items": {}, "asks": {}, "mdplug": {}, "traitval": {}, "kitfn": {}}
    return st


def gtype(go, d, out=False):
    """Map extended descriptors (`civil`, `libval`, `cell`, `ask`, `items`, `mdplug`, `traitval`, `kitfn`, `held`, etc.) to Go types.

    Side effect: encountering `mdplug`, `traitval`, `kitfn`, or any row-item descriptor
    registers its union members / callback signature in `_state(go)` so `emit_files()`
    knows to emit the corresponding Go interface or `ItemFunc` switch case.
    Returns `None` for descriptors not owned by this module.
    """
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
        import gk_values
        name = _camel(gk_values.TRAIT_VALUES[d[1]][1])
        _state(go)["traitval"][name] = d[2]
        return name
    if k == "kitfn":
        name = _camel(ada(d[2]))
        _state(go)["kitfn"][name] = d[3]
        item_kind(go, d[4])
        return name
    if k in ("ask", "askdyn"):
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


def put(go, d, v, depth=0):
    """Return Go tape-encoding statements for extended descriptors handled by `gk_go_kinds`.

    Key wire-format conventions:
    - `mdplug`: `u8` tag (0..N-1 for generated node plugins followed by `takeValue(x.n)`;
      N for hand-written `MarkdownPlugin` struct or nil fallback).
    - `traitval`: dispatches over implementing value nodes and writes `takeValue(x.n)`
      (nil interface aborts via `stop(...)`).
    - `kitfn`: `u8(0) + u64(RowSource.h)` for a custom callback, or `u8(1) + u16(enum)`
      for a built-in upstream renderer.
    - `ctxrows` / `items`: validates the callback's kind ID at runtime via `itemsCheck`
      before writing the `RowSource` handle, catching mismatched callback signatures.
    - `held`: raw `u64` inside call ops, or `putObj(o, h)` inside view tapes so the
      frame keeps the handle pinned.
    """
    m, k = go.m, d[0]
    if k == "civil":
        return [f"put{CIVIL[d[1]]}(o, {v})"]
    if k == "decorate":
        return [f"rt.PutDecorate(o, {v}.Style, {v}.Content)"]
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


def rd(go, d, out=True):
    """Return a Go wire-decoding expression for extended descriptors read back from Rust.

    For `drag_move_event`, ensures `rdBounds` is marked needed in `go.need_readers`
    because `rdDragMoveEvent` calls it to decode the drag bounds.
    """
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
    """Emit a handle-backed Held variant type for recursive data enums kept in the runtime.

    Only applies to input-only recursive data enums marked `held=True` by `gk_editor.py`
    (such as `BlockNode`). Appends a wire serializer case to `puts` that writes
    `gk_editor.HELD_TAG` followed by `putObj(o, x.h)` so Rust recovers the held object.
    """
    import gk_editor
    if not e.get("held") or e["out"]:
        return
    E = e["go"]
    V = E + "Held"
    go.add(f"// {V} wraps a runtime-held {e['name']} returned by a call; call Retain to keep it across turns.",
           f"type {V} struct{{ h uint64 }}", "",
           f"func (v {V}) handle() uint64 {{ return v.h }}", "", f"func ({V}) is{E}() {{}}", "")
    puts.append((V, [f"o.u16({gk_editor.HELD_TAG:#x})", "putObj(o, x.h)"]))


def _as_items(d):
    """Normalize `ctxrows`/`ctxlazy`/`viewrows` descriptors into the uniform `items` shape."""
    if d[0] == "items":
        return d
    if d[0] in ("ctxrows", "ctxlazy", "viewrows"):
        return ["items", d[1], d[2], d[3], [["val", x] for x in d[4]], "el"]
    raise KeyError(d[0])


def _inline_plugin_kind(go):
    """Ensure the `MarkdownPlugin` inline renderer callback signature is registered in `ItemFunc`.

    Even though `MarkdownPlugin` is a hand-written Go struct rather than a generated
    `items` builder argument, apps pass an `Items(...)` callback taking
    `(cx, MarkdownNodeRead, InlineRenderContext)` to it.
    """
    keys = {}
    for key, st in go.m.states.items():
        if st.get("go") in ("MarkdownNodeRead", "InlineRenderContext"):
            keys[st["go"]] = key
    if len(keys) < 2:
        raise _unsup()("the inline plugin's row needs gk.MarkdownNodeRead and gk.InlineRenderContext")
    vals = [["val", ["ref", ["obj", None, keys[n], True]]] for n in ("MarkdownNodeRead", "InlineRenderContext")]
    item_kind(go, ["items", "impl", [], [], vals, "el"])


def item_kind(go, d):
    """Register an `Items` callback signature and its row-context decoder body, returning its 1-based kind ID.

    Walks the descriptor's payload list in exact Rust parameter order so the generated
    Go decoder reads wire fields (`val`, `reads`) and claims handed-object slots
    (`handed`, `view`, indexed by `hi`) in lockstep with `gk_items.rust_dec()`.
    Deduplicates identical Go callback signatures onto the same kind ID.
    """
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


def emit_files(go):
    """Emit gen_items.go with ItemFunc, Items, ask codecs, and plugin/trait/kitfn union types."""
    st = _state(go)
    for rc in go.m.records.values():
        for f in rc["fields"]:
            if rc["in"] and f["desc"][0] in ("ask", "askdyn"):
                try:
                    go.put(f["desc"], "v")
                except _unsup():
                    pass
    if st["mdplug"]:
        _inline_plugin_kind(go)
    go.open("gen_items.go", "Item row-source dispatchers (Items), ask codecs, and generated union markers.")
    kinds = sorted(st["items"].items(), key=lambda kv: kv[1][0])
    go.add("// ItemFunc constrains callback signatures accepted by Items for per-item component rendering.")
    if kinds:
        go.add("type ItemFunc interface {", "\t" + " |\n\t\t".join(ty for ty, _ in kinds), "}", "")
    else:
        go.add("type ItemFunc interface{ func(cx *Cx) Element }", "")
    go.add("func init() { rt.ItemsKinds = []string{\"\", " + ", ".join(f'"{ty}"' for ty, _ in kinds) + "} }", "")
    go.add("// Items creates or reuses a RowSource backed by callback f, which renders individual",
           "// component items (such as calendar days, tree rows, or popovers) on demand between frames.",
           "// Passing the same callback in the same view returns the same source.",
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
        go.add(f"// {name} marks generated value types accepted for this trait: " + ", ".join(_node_go(go, k) for k in keys) + ".",
               f"type {name} interface{{ is{name}() }}", "")
        for key in keys:
            go.add(f"func ({_node_go(go, key)}) is{name}() {{}}")
        go.add("")
    for name, fns in sorted(st["kitfn"].items()):
        go.add(f"// {name} accepts either a custom RowSource (from Items) or a built-in {name}Kit renderer.",
               f"type {name} interface{{ is{name}() }}", "", f"func (RowSource) is{name}() {{}}", "",
               f"// {name}Kit selects a built-in upstream renderer for {name}.", f"type {name}Kit uint16", "", f"func ({name}Kit) is{name}() {{}}", "",
               f"// {name}Kit built-in renderer constants.", "const (")
        for i, fn in enumerate(fns):
            go.add(f"\t{name}Kit{_camel(ada(fn.rsplit('::', 1)[-1]))} {name}Kit = {i} // {fn}")
        go.add(")", "")
    go.close()


_ = re
