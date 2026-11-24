"""The editor family's generator rules, docs/PROTOCOLS.md sections 9.20 to 9.26.

Hooked into gk_model.py, gk_rust.py and gk_go.py at a few points each (search for
`gk_editor.`); everything else of the editor rules lives here. gk_lists.py (the rest of the lists
and editor rules) is not edited: these rules come before or after its hooks.

- **Typed fragments** (`tfrag`, 9.20). A closure that gets the window and the app, no values, and
  returns one element type (`MarkdownExtensions::block_surface(name, Fn(&mut Window, &mut App) ->
  Div)`): a fragment of the app's view, as any render closure without an item is, whose root is
  decoded as that type. The app passes an element of the type (`gk.div()->...`).
- **Predicates as key sets** (`keyset`, 9.21). `FnMut(&T) -> bool` with no window or app over a
  string or an integer (`History::retain`): the app passes the keys for which it holds.
- **Tables** (`table`, 9.21). `Fn(u8) -> V` with no window or app (`TextViewStyle::with_heading`, a
  heading level): the app passes the values by index; an index past the end is V's default.
- **Element closures over values without a window** (`ctxlazy`, 9.22). As an element closure over
  a context (9.16) but upstream gives the closure no window and no app (`FoldPlaceholderRenderer =
  Rc<dyn Fn(usize) -> AnyElement>`): the closure returns a library element that becomes the row
  when it is laid out, where the window and the app are at hand.
- **View closures over values** (`viewrows`, 9.22). A closure over values that returns an `AnyView`
  (`TextView::link_tooltip(Fn(&SharedString, &mut Window, &mut App) -> AnyView)`): the row's view of
  a row source, one row per distinct value.
- **Markdown plugins** (`mdplug`, 9.23). `plugin<P: MarkdownPlugin>` and `plugin<P: TextViewPlugin>`
  take one of the kit's plugins (node types that implement MarkdownPlugin: FrontmatterPlugin) or
  the library's data plugin (`crate::editor_text::DataPlugin`, Go `gk.MarkdownPlugin`), which
  claims Markdown nodes by a rule the app gives as data and draws them through a row source.
- **Small values** (9.24): an `ElementId` read back is its text; a `gpui::EntityId` argument is the
  entity behind any input, textarea or editor handle (`gk.entity_id(state)`).
- **Objects a read-back value hands out** (9.25): an object returned by a `&self` read of a value
  that is read back (`DiffFile::conflicts() -> &[DiffConflict]`) is reachable, so its reads are.
"""

import json
import re

# The library's plugin enum has one variant per kit node type that implements MarkdownPlugin, then
# the data plugin (core/src/editor_text.rs `GkPlugin`). A new implementor upstream must be added
# there and here; generation stops until it is.
PLUGIN_NODES = ["gpui_kit::component::text::FrontmatterPlugin"]
PLUGIN_VARIANTS = {"gpui_kit::component::text::FrontmatterPlugin": "Frontmatter"}
PLUGIN_TRAITS = ("MarkdownPlugin", "TextViewPlugin")


def _g():
    import gk_model  # noqa: PLC0415 (gk_model imports this module)
    return gk_model


# ---------------------------------------------------------------- closures (9.20 to 9.22)

def _is_element_out(m, ctx, output):
    import rdj  # noqa: PLC0415
    s = rdj.ty(output)
    return "impl_trait" in output and "IntoElement" in s or s.rsplit("::", 1)[-1] == "AnyElement"


def _plain(m, ctx, payload, objects_ok):
    """The descriptors of a closure's values when every one is a value the app can read (a key's
    bytes: gk_lists.ASK_KEY) or, when objects_ok, a library object that is Clone; else None."""
    import gk_lists  # noqa: PLC0415
    g = _g()
    try:
        descs = [m.desc(ctx, t, out=True) for t in payload]
    except g.Unsup:
        return None
    for d in descs:
        for x in g.walk_desc(d):
            if x[0] == "obj":
                if not (objects_ok and x[3]):
                    return None
                continue
            if x[0] not in gk_lists.ASK_KEY or (x[0] == "leaf" and x[1] not in ("px", "range", "color")):
                return None
    return descs


def closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload):
    """The editor family's closure descriptors (gk_model.closure_desc's hook, after the expression
    closures), or None for a closure no rule of this module binds."""
    import rdj  # noqa: PLC0415
    g = _g()
    if output is None or "context" in params:
        return None
    out_s = rdj.ty(output)
    has_cx = "window" in params and "app" in params
    # 9.20: a typed fragment
    if not payload and has_cx and "resolved_path" in output:
        try:
            d = m.desc(ctx, output)
        except g.Unsup:
            d = None
        if d is not None and d[0] == "node" and m.nodes[d[1]]["element"] and m.nodes[d[1]]["inst"] is None:
            return ["tfrag", d[1], params, wrap, ptypes]
    # 9.21: a predicate over keys, a table by index
    if params == ["arg"] and len(payload) == 1:
        if out_s == "bool":
            try:
                kd = m.desc(ctx, payload[0], out=True)
            except g.Unsup:
                kd = None
            base = kd[1] if kd is not None and kd[0] == "ref" else kd
            if base is not None and (base[0] == "str" or base[0] == "int"):
                return ["keyset", base, wrap, ptypes]
        if rdj.ty(payload[0]) == "u8" and out_s != "bool":
            try:
                vd = m.desc(ctx, output)
            except g.Unsup:
                vd = None
            if vd is not None and vd[0] in ("node", "str", "int", "f32", "f64", "rec", "enum") and \
                    (vd[0] != "node" or not m.nodes[vd[1]]["element"]):
                return ["table", vd, wrap, ptypes, m.rtype(ctx, output)]
    # 9.22: element closures over values without a window; view closures over values
    if payload and len(payload) <= 3 and _is_element_out(m, ctx, output) and "window" not in params \
            and "app" not in params and all(p == "arg" for p in params):
        descs = _plain(m, ctx, payload, objects_ok=True)
        if descs is not None:
            return ["ctxlazy", wrap, ptypes, params, descs]
    if payload and len(payload) <= 3 and has_cx and out_s.rsplit("::", 1)[-1] == "AnyView":
        descs = _plain(m, ctx, payload, objects_ok=False)
        if descs is not None:
            return ["viewrows", wrap, ptypes, params, descs]
    return None


# ---------------------------------------------------------------- generic plugins (9.23)

def bounded(m, ctx, bounds, out):
    """`P: MarkdownPlugin` and `P: TextViewPlugin` going in: one of the kit's plugins or the
    library's data plugin (gk_model.bounded's hook). None for other bounds."""
    if out:
        return None
    g = _g()
    names = [n for n, _ in g.bound_parts(bounds)]
    if not any(n in PLUGIN_TRAITS for n in names) or set(names) - set(PLUGIN_TRAITS) - {"Sized", "Send", "Sync", "'static"}:
        return None
    keys = sorted(k for k, nd in m.nodes.items() if not nd["element"] and nd["inst"] is None
                  and "MarkdownPlugin" in m.info(nd["crate"], nd["id"])["traits"])
    if keys != sorted(PLUGIN_NODES):
        raise SystemExit(f"gkgen: the kit's MarkdownPlugin node types changed ({keys}); "
                         "add them to core/src/editor_text.rs GkPlugin and gen/gk_editor.py PLUGIN_NODES")
    return ["mdplug", "crate::editor_text::DataPlugin"] + [["node", k] for k in keys]  # nodes as gates/accept.py walks them


def handed_out(it):
    """Library objects an item hands the app besides its result (gk_model.reach_objects' hook): a
    data plugin's inline rows read the inline render context (9.23)."""
    g = _g()
    for a in it.get("args", []):
        for x in g.walk_desc(a["desc"]):
            if x[0] == "mdplug":
                yield "gpui_kit::base::InlineRenderContext"
    yield from handed_out_host(it)  # the editor host's leaves (9.30)


# ---------------------------------------------------------------- small values (9.24)

def gpui_value(m, hc, name, args, out):
    """gpui's ElementId read back (its text) and EntityId going in (an input, textarea or editor
    handle), gk_model.desc's hook before gpui's leaves; None for other types."""
    g = _g()
    if hc not in g.GPUI_HOMES or args:
        return None
    if name == "ElementId" and out:
        return ["str", "ElementId"]  # Wire in core/src/editor_text.rs: ElementId's text
    if name == "EntityId" and not out:
        return ["entityid"]
    return None


# Descriptors of this module a call op can decode (gk_model.call_bad's hook).
CALL_SAFE = {"keyset", "table", "ctxlazy", "viewrows", "entityid", "askdyn", "held", "lentmut"}


# ---------------------------------------------------------------- objects a read-back value hands out (9.25)

def before_reach(m):
    """Keeps every object's lists and its items' statuses as they are before the reachability
    pass (gk_model.settle's hook), so `revive` can give back an object found reachable later."""
    m._editor_kept = {key: {grp: list(st[grp]) for grp in ("builders", "reads", "writes", "calls")}
                      for key, st in m.objects.items()}
    m._editor_status = {id(it): (it["status"], it["reason"], it.get("detail"))
                        for st in m.objects.values()
                        for it in st["builders"] + st["reads"] + st["writes"] + st["calls"] + st["statics"]}
    host(m)  # the editor host (9.30)
    m._editor_pre = {id(it): (it["status"], it["reason"], it.get("detail")) for it in m.all_items()}


def revive(m):
    """An object returned by a `&self` read of a value that is read back (`DiffFile::conflicts`
    -> `&[DiffConflict]`) is reachable: the read-back value's reads are made later (the values
    family's readback_reads), so the reachability pass found nothing that hands it out. Its lists
    and statuses are given back before those reads are classified (gk_model.settle's hook, before
    gk_values.post)."""
    import rdj  # noqa: PLC0415
    g = _g()
    left_reasons(m)
    kept = getattr(m, "_editor_kept", None)
    if not kept:
        return
    dead = set(getattr(m, "dead_objs", set()))
    if not dead:
        return
    found = set()
    for key, st in sorted(m.objects.items()):
        if not st.get("readback"):
            continue
        hc, hid = st["readback"]
        f = m.info(hc, hid)
        c = m.api.crates[hc]
        ctx = g.Ctx(hc, (hc, {"resolved_path": {"path": f["name"], "id": int(hid), "args": None}}))
        for im in f["impls"]:
            if im.get("trait") is not None:
                continue
            for mid in im["items"]:
                fi = c.item(mid)
                if not fi or "function" not in fi["inner"] or fi["visibility"] != "public":
                    continue
                sig = fi["inner"]["function"]["sig"]
                ins = sig["inputs"]
                if not ins or ins[0][0] != "self" or rdj.ty(ins[0][1]) != "&Self" or len(ins) != 1 or not sig.get("output"):
                    continue
                m._closure += 1  # an object read back by value is a copy behind a handle
                try:
                    d = m.desc(m.fn_ctx(ctx, fi["inner"]["function"]), sig["output"], out=True)
                except g.Unsup:
                    continue
                finally:
                    m._closure -= 1
                for x in g.walk_desc(d):
                    if x[0] == "obj" and x[2] in dead and x[3]:
                        found.add(x[2])
    why = "protocol: object (no constructor, global or bound function hands one out)"
    for key in sorted(found):
        st = m.objects[key]
        for grp, items in kept.get(key, {}).items():
            st[grp] = items
            for it in items:
                was = m._editor_status.get(id(it))
                if was is not None and it["status"] == "todo" and it["reason"] == why:
                    it["status"], it["reason"] = was[0], was[1]
        st["reachable"] = True
        m.dead_objs.discard(key)


# ---------------------------------------------------------------- Rust (gk_rust.Rust.dec's hook)

def rust_dec(rs, d):
    k = d[0]
    if rs.call == "builder" and (k == "node" or d[:2] == ["leaf", "color"]):
        # A builder of `chain_cx` (9.31) in the chain decoder without a context: never reached, its
        # constructors decode their chain with sbp_ (only chain_cx builders get here: call_bad refuses
        # a value node or a colour in any other chain).
        return "None?"
    if d == ["action", "box"] and rs.call not in (None, "builder", "codec"):
        return "genapi::action(r)?"  # an action argument of a call op (9.26)
    ir = rust_inline_rec(rs, d)
    if ir is not None:
        return ir  # a record with a colour, in place in a call op (9.26)
    if k == "askdyn":
        return rust_askdyn(rs, d)
    if k == "tfrag":
        _, key, params, wrap, ptypes = d
        nd = rs.m.nodes[key]
        ps = rs.closure_params(params, ptypes)
        body = ("{ let f = crate::editor_text::MainThread::new(dx.frag(r.u32())); move |%s| -> %s { f.get().with(window, cx, "
                "|dx, n| { let (ty, a, fi) = dx.node(n); make_%s(dx, ty, a, fi) }).unwrap_or_else(|| crate::editor_text::bad_fragment(\"%s\")) } }"
                % (ps, nd["path"], nd["fn"], nd["name"]))
        return rs.wrap(wrap, body)
    if k == "keyset":
        _, key, wrap, ptypes = d
        if key[0] == "str":
            keep = rs.dec(["list", ["str", "String"], "vec"])
            test = "keep.iter().any(|x| x.as_str() == AsRef::<str>::as_ref(k))"
        else:
            keep = rs.dec(["list", key, "vec"])
            test = "keep.iter().any(|x| *x == *k)" if ptypes[0].startswith("&") else "keep.contains(&k)"
        return rs.wrap(wrap, "{ let keep = %s; move |k: %s| -> bool { %s } }" % (keep, ptypes[0], test))
    if k == "table":
        _, vd, wrap, ptypes, vt = d
        vals = rs.dec(["list", vd, "vec"])
        return rs.wrap(wrap, "{ let t: Vec<%s> = %s; move |k: %s| -> %s { t.get(k as usize).cloned().unwrap_or_default() } }"
                       % (vt, vals, ptypes[0], vt))
    if k == "ctxlazy":
        return _rust_ctx(rs, d, lazy=True)
    if k == "viewrows":
        return _rust_ctx(rs, d, lazy=False)
    if k == "mdplug":
        arms = []
        for n, (_, key) in enumerate(d[2:]):
            fn = rs.m.nodes[key]["fn"]
            arms.append(f"{n} => crate::editor_text::GkPlugin::{PLUGIN_VARIANTS[key]}(build_{fn}(dx, r.u32())?),")
        return ("{ match r.u8() { %s _ => crate::editor_text::GkPlugin::Data(<%s as genapi::Unwire>::get(r)) } }"
                % (" ".join(arms), d[1]))
    if k == "entityid":
        return "crate::editor_text::entity_id(r.u64())"
    if k == "decorate":
        return rust_decorate(rs, d)  # 9.32
    if k == "lentmut":  # the value's tape and its ops' decoder, applied in place (9.33)
        return f"crate::editor_text::LentEdit::read(r, apply_{rs.m.nodes[d[1][1]]['fn']})?"
    return None


def _rust_ctx(rs, d, lazy):
    import gk_lists  # noqa: PLC0415
    _, wrap, ptypes, params, descs = d
    ps, lets, puts, n = [], [], [], 0
    for p, t in zip(params, ptypes):
        if p == "arg":
            v, dd = f"k{n}", descs[n]
            ps.append(f"{v}: {t}")
            lets += gk_lists._ctx_print(rs, dd, v)
            base = dd[1] if dd[0] == "ref" else dd
            if base[0] == "obj":
                lets.append(f"let c{n} = {v}.clone();")
                puts.append(f"genapi::Wire::put(&c{n}, o);")
            else:
                lets.append(f"let b{n} = {{ let mut b = Vec::new(); genapi::Wire::put(&{v}, &mut b); b }};")
                puts.append(f"o.extend_from_slice(&b{n});")
            n += 1
        elif p == "window":
            ps.append(f"window: {t}")
        elif p == "app":
            ps.append(f"cx: {t}")
        else:
            ps.append(f"_{p}: {t}")
    write = "std::rc::Rc::new(move |o: &mut Vec<u8>| { %s })" % " ".join(puts)
    if lazy:
        deref = "*" if ptypes[0].startswith("&") else ""
        by = f"Some(({deref}k0) as u64)" if len(descs) == 1 and descs[0][0] == "int" else "None"
        body = ("{ let src = r.u64(); move |%s| -> gpui_kit::AnyElement { let mut p: Vec<u8> = Vec::new(); %s "
                "crate::editor_text::lazy_row(src, %s, p, %s) } }" % (", ".join(ps), " ".join(lets), by, write))
        ret = "gpui_kit::AnyElement"
    else:
        body = ("{ let src = r.u64(); move |%s| -> gpui_kit::AnyView { let mut p: Vec<u8> = Vec::new(); %s "
                "crate::editor_text::ctx_view(src, p, %s, window, cx) } }" % (", ".join(ps), " ".join(lets), write))
        ret = "gpui_kit::AnyView"
    if wrap in ("rc", "box", "arc"):
        ptr = {"rc": "std::rc::Rc", "box": "Box", "arc": "std::sync::Arc"}[wrap]
        return f"{{ let f: {ptr}<dyn Fn({', '.join(ptypes)}) -> {ret}> = {rs.wrap(wrap, body)}; f }}"
    return rs.wrap(wrap, body)


def check(m):
    """The editor family's descriptors as text, for the report (not used by the emitters)."""
    return json.dumps(sorted(CALL_SAFE))


def alias(m, ctx, t, out):
    """A use of a type alias whose type is one of this module's closures (`FoldPlaceholderRenderer
    = Rc<dyn Fn(usize) -> AnyElement>`), gk_model.desc's hook before the lists family's; None for
    any other type (the alias stays what it was)."""
    g = _g()
    if out:
        return None
    ad = ask_trait(m, ctx, t, out)
    if ad is not None:
        return ad  # Arc<dyn Trait> served by an ask (9.26)
    rp = t["resolved_path"]
    pa = g.path_args(rp)
    if rp["path"].rsplit("::", 1)[-1] in ("Rc", "Arc", "Box") and len(pa) == 1 and "resolved_path" in pa[0]:
        # `Rc<CompletionHighlighter>`, the alias a `dyn Fn(..)`: the closure behind the pointer (9.33)
        try:
            ahc, ahid, ait = m.api.item(ctx.crate, pa[0]["resolved_path"]["id"])
        except Exception:  # noqa: BLE001
            ait = None
        if ait is not None and ahc == ctx.crate and "dyn_trait" in ait.get("inner", {}).get("type_alias", {}).get("type", {}):
            import copy  # noqa: PLC0415
            t2 = copy.deepcopy(t)
            t2["resolved_path"]["args"]["angle_bracketed"]["args"][0] = {"type": ait["inner"]["type_alias"]["type"]}
            return m.desc(ctx, t2, out)
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:  # noqa: BLE001
        return None
    if it is None or "type_alias" not in it.get("inner", {}):
        return None
    try:
        d = m.desc(g.Ctx(hc), it["inner"]["type_alias"]["type"], out)
    except g.Unsup:
        return None
    return d if d[0] in ("ctxlazy", "viewrows", "askdyn") else None


# ---------------------------------------------------------------- colours in records of a call op (9.26)

# What a call op can decode in place, field by field, when a record holds a colour (gk_model's own
# list for call ops, without colours and handles at depth, which this rule allows inside records).
_INLINE_OK = {"bool", "int", "f32", "f64", "str", "enum", "opt", "list", "ref", "tuple", "rec", "from"}


def _has_color(m, d, seen=None):
    g = _g()
    seen = set() if seen is None else seen
    for x in g.walk_desc(d):
        if x[0] == "leaf" and x[1] == "color":
            return True
        if x[0] == "rec" and x[1] in m.records and x[1] not in seen:
            seen.add(x[1])
            if any(_has_color(m, f["desc"], seen) for f in m.records[x[1]]["fields"]):
                return True
    return False


def _inline_bad(m, d, seen):
    """The first part of d a call op cannot decode in place, or None."""
    import gk_lists  # noqa: PLC0415
    g = _g()
    for x in g.walk_desc(d):
        k = x[0]
        if k == "leaf":
            if x[1] in ("color", "px", "rems", "range", "duration"):
                continue
            return "leaf"
        if k in CALL_SAFE or k in gk_lists.CALL_SAFE or k == "askdyn":
            continue
        if k not in _INLINE_OK:
            return k
        if k == "rec":
            r = m.records.get(x[1])
            if r is None or not r["complete"]:
                return "rec"
            if x[1] in seen:
                continue
            seen.add(x[1])
            for f in r["fields"]:
                bad = _inline_bad(m, f["desc"], seen)
                if bad:
                    return bad
    return None


def call_bad(m, d, builder):
    """A call op's argument that holds a record with a colour (`Vec<TextDecoration>`, whose
    HighlightStyle has colours; `InputEditorStyle`): decoded in place, field by field, with the
    app context the op has, so its colours resolve against the theme (PROTOCOLS.md 9.26).
    gk_model.call_bad's hook: (verdict,) when this rule decides, verdict None for decodable or
    the first part that is not; None when the rule does not apply (no record with a colour)."""
    if builder:
        return None
    if d == ["action", "box"]:
        return (None,)  # a kit or app action by its id, which a call op reads as a tape does (9.26)
    g = _g()
    if not _ACTION_INSIDE[0] and _reaches(m, d, ("action", "box")) and not _reaches(m, d, ("action", "listener")):
        # An action inside a record or a variant (OwnedMenuItem::Action, read back since 9.37 serves
        # every type): read by its id in a call op too, as at the top level; the rest as the model judges.
        _ACTION_INSIDE[0] = True
        CALL_SAFE.add("action")
        try:
            return (m.call_bad(d, builder),)
        finally:
            CALL_SAFE.discard("action")
            _ACTION_INSIDE[0] = False
    if not any(x[0] == "rec" and _has_color(m, x) for x in g.walk_desc(d)):
        return None
    return (_inline_bad(m, d, set()),)


_ACTION_INSIDE = [False]


def _reaches(m, d, head, seen=None):
    """True when the descriptor d holds `head` (a descriptor's first two parts), following records,
    data enums and Into sets by name."""
    g = _g()
    seen = set() if seen is None else seen
    for x in g.walk_desc(d):
        if tuple(x[:2]) == head:
            return True
        if x[0] in ("rec", "denum", "into") and (x[0], x[1]) not in seen:
            seen.add((x[0], x[1]))
            t = {"rec": m.records, "denum": m.denums, "into": m.intos}[x[0]].get(x[1])
            if t is None:
                continue
            parts = ([f["desc"] for f in t["fields"]] if x[0] == "rec" else
                     [f["desc"] for v in t["variants"] if v["ok"] for f in v["fields"]] if x[0] == "denum" else
                     [mm["desc"] for mm in t["members"]])
            if any(_reaches(m, p, head, seen) for p in parts):
                return True
    return False


def rust_inline_rec(rs, d):
    """The in-place decoder of a record with a colour in a call op (9.26), or None."""
    inside = getattr(rs, "_editor_inline", False)
    if d[0] != "rec" or rs.call in (None, "builder", "codec") or not (inside or _has_color(rs.m, d)):
        return None
    r = rs.m.records[d[1]]
    rs._editor_inline = True  # every record inside is decoded in place too: it may have no codec of its own
    try:
        if r["tuple"]:
            return "%s(%s)" % (d[1], ", ".join(rs.dec(f["desc"]) for f in r["fields"]))
        return "%s { %s }" % (d[1], ", ".join(f"{f['name']}: {rs.dec(f['desc'])}" for f in r["fields"]))
    finally:
        rs._editor_inline = inside


# ---------------------------------------------------------------- one-method data delegates (9.26)

# Traits upstream calls while it draws whose one method maps a plain value to a value, served by an
# ask (9.14): the library function that makes the trait object from the ask and the answer's decoder.
ASK_TRAITS = {"HighlightStyleResolver": "crate::editor_text::style_resolver"}


def ask_trait(m, ctx, t, out):
    """`Arc<dyn Trait>` (or `Rc`, `Box`), directly or through a type alias (`SharedHighlightStyleResolver`),
    for a trait of ASK_TRAITS: `["askdyn", key, result, shim]`; the app passes a gk.Ask of the method's
    argument and result (gk_model.desc's hook through `alias`). None otherwise."""
    g = _g()
    if out or "resolved_path" not in t:
        return None
    rp = t["resolved_path"]
    args = g.path_args(rp)
    if rp["path"].rsplit("::", 1)[-1] not in ("Arc", "Rc", "Box") or len(args) != 1 or "dyn_trait" not in args[0]:
        return None
    names = [x["trait"]["path"].rsplit("::", 1)[-1] for x in args[0]["dyn_trait"]["traits"]]
    shim = ASK_TRAITS.get(names[0]) if len(names) == 1 else None
    if shim is None:
        return None
    tr = args[0]["dyn_trait"]["traits"][0]["trait"]
    hc, hid, it = m.api.item(ctx.crate, tr["id"])
    if it is None or "trait" not in it.get("inner", {}):
        return None
    c = m.api.crates[hc]
    fns = [c.item(i) for i in it["inner"]["trait"]["items"]]
    fns = [f for f in fns if f and "function" in f["inner"]]
    if len(fns) != 1:
        return None
    sig = fns[0]["inner"]["function"]["sig"]
    ins = [t for n, t in sig["inputs"] if n != "self"]
    if len(ins) != 1 or not sig.get("output"):
        return None
    tctx = g.Ctx(hc)
    try:
        key = m.desc(tctx, ins[0], out=True)
        ret = m.desc(tctx, sig["output"])
    except g.Unsup:
        return None
    m._editor_ask_rt = getattr(m, "_editor_ask_rt", {})
    m._editor_ask_rt[names[0]] = m.rtype(tctx, sig["output"])
    return ["askdyn", key, ret, shim, names[0]]


def rust_askdyn(rs, d):
    _, key, ret, shim, trait = d
    rt = rs.m._editor_ask_rt[trait]
    was = rs.call
    rs.call = None  # an answer is decoded as a tape's value is, with a context over no tape
    try:
        dec = rs.dec(ret)
    finally:
        rs.call = was
    return (f"{shim}(r.u64(), |r: &mut genapi::R, dx: &mut genapi::Dx| -> Option<{rt}> {{ let _ = &dx; Some({dec}) }})")


# ---------------------------------------------------------------- reads through a borrow (9.27)

def project(m):
    """A state read `fn x(&self) -> &T` where T is a library object that cannot be copied out of the
    borrow (`InputState::display_map() -> &DisplayMap`; DisplayMap is not Clone) was refused: nothing
    can carry it. Its object's reads become reads of the state (`ed->display_map_folded_ranges(cx)`,
    `display_map_buffer_pos_to_display_pos(cx, pos)`): synthetic state reads that call the object's
    read on the borrow, in place (gk_model.settle's hook, before the reachability pass). The upstream
    function counts as bridged by them (PROTOCOLS.md 9.27)."""
    import re  # noqa: PLC0415
    why = "protocol: object (a result that cannot be copied out of a borrow)"
    for it in list(m.items):
        if it["status"] != "todo" or it["reason"] != why:
            continue
        st = m.states.get(it["owner"])
        mt = re.fullmatch(r"fn (\w+)\(&self\) -> &(\w+)", it["sig"])
        if st is None or st.get("obj") or not mt:
            continue
        objs = [o for o in m.objects.values() if o["name"] == mt.group(2) and o.get("obj") and not o.get("clone")]
        if len(objs) != 1:
            continue
        made = []
        for rd in objs[0]["reads"]:
            if rd["status"] not in ("bound", "bridged") or not rd.get("ret") or any(a["desc"][0] == "ctx" for a in rd["args"]):
                continue
            ps = "".join(f", a{n}" for n in range(len(rd["args"])))
            call = f"{rd['callee']}(<{st['path']}>::{mt.group(1)}(s){ps})"
            out_s = rd["sig"].split("->", 1)[-1].strip()
            if out_s.startswith("&"):
                call = f"std::borrow::ToOwned::to_owned({call})"  # a closure cannot return a borrow of its argument
            elif out_s.startswith("Option<&"):
                call = f"({call}).map(std::borrow::ToOwned::to_owned)"
            new = {"row": None, "owner": it["owner"], "trait": None, "crate": it["crate"], "synthetic": True,
                   "name": f"{mt.group(1)}_{rd['name']}", "id": f"{it['id']}#{rd['name']}", "kind": "state-read",
                   "status": "bridged", "reason": "", "args": [dict(a) for a in rd["args"]], "ret": rd["ret"],
                   "state": st["key"],
                   "callee": f"(|s: &{st['path']}{ps}| {call})",
                   "sig": f"fn {mt.group(1)}_{rd['name']}(..): {mt.group(2)}::{rd['name']} through {mt.group(1)}()"}
            st["reads"].append(new)
            m.items.append(new)
            made.append(new["name"])
        if made:
            it.update(status="bridged", kind="projected", reason="", detail=f"{len(made)} reads: " + ", ".join(made))
            # The generic state's row (one summary item per upstream function) follows its instances.
            for r in m.owners:
                if r["path"] != st.get("generic"):
                    continue
                for sm in r["items"]:
                    if sm.get("summary") and sm["name"] == mt.group(1) and sm["status"] == "todo":
                        sm.update(status="bridged", reason="", detail=it["detail"])
                        sm.setdefault("also", []).append(it["id"])


# ---------------------------------------------------------------- what is left, and why

# The editor family's functions no rule binds yet, with the protocol each waits for (STATUS.md's
# reasons; PROTOCOLS.md 9.28 says how each would be bound). Matched on the end of the item's id.
LEFT = {
    # Every function this table named is now bound (PROTOCOLS.md 9.30 to 9.36); a function of the
    # family left without a rule gets its reason here.
}


def left_reasons(m):
    """Gives the editor family's remaining functions the reason of the protocol they wait for
    (gk_model.settle's hook, through `revive`)."""
    items = list(m.all_items()) + [it for r in m.owners for it in r["items"]]
    for it in items:
        if it.get("status") != "todo":
            continue
        for tail, why in LEFT.items():
            if it["id"].endswith(tail) or (tail.startswith("::") and it["id"].endswith(tail)) or tail in it["id"]:
                it["reason"] = why
                break


# ---------------------------------------------------------------- variants made by a constructor (9.29)

def variant_ctors(m, f, rt, variants, out):
    """A variant the binding cannot build field by field (its other fields are of private types:
    `MaskPattern::Pattern { pattern, tokens: Vec<MaskToken> }`) is made by the enum's constructor
    whose parameters are named as some of its fields, when `[editor] variants` in overrides.toml
    names the pair ("MaskPattern::Pattern" = "new"): the variant's fields are the constructor's
    parameters, and the decoder calls the constructor (gk_model.enum's hook). PROTOCOLS.md 9.29."""
    if out:
        return
    g = _g()
    table = m.ov.get("editor", {}).get("variants", {})
    for v in variants:
        key = f"{f['name']}::{v['name']}"
        ctor = table.get(key)
        if ctor is None or v["ok"]:
            continue
        hc, hid = f["crate"], f["id"]
        c = m.api.crates[hc]
        fn = None
        for im in f["impls"]:
            if im.get("trait") is not None:
                continue
            for mid in im["items"]:
                fi = c.item(mid)
                if fi and fi["name"] == ctor and "function" in fi["inner"] and fi["visibility"] == "public":
                    fn = fi["inner"]["function"]
        if fn is None:
            raise SystemExit(f"gkgen: overrides [editor] variants: {f['name']} has no public fn {ctor}")
        fields, how = [], []
        for pn, pt in fn["sig"]["inputs"]:
            d = m.desc(g.Ctx(hc), pt)
            fields.append({"name": pn, "desc": d})
            how.append("&*" + pn if d[0] == "str" else pn)
        v.update(ok=True, fields=fields, via=f"<{rt}>::{ctor}({', '.join(how)})")
        m.ov_used.add("editor.variants." + key)


def check_used(m):
    for key in m.ov.get("editor", {}).get("variants", {}):
        if "editor.variants." + key not in m.ov_used:
            raise SystemExit(f"gkgen: overrides [editor] variants names nothing the generator met: {key}")


def variant_new(rs, e, v):
    """The Rust decoder of a variant made by a constructor (9.29), or None (gk_rust.variant_new's hook)."""
    if "via" not in v:
        return None
    lets = " ".join(f"let {f['name']} = {rs.dec(f['desc'])};" for f in v["fields"])
    return "{ %s %s }" % (lets, v["via"])


# ---------------------------------------------------------------- the editor host (9.30)
#
# gpui_kit::base::text::editor_host is for an editor written outside the kit: it renders a BlockNode
# tree and, around the blocks' prepaint, collects each keyed text leaf's geometry. The library's
# element `gk.block_host` (core/src/editor_text.rs BlockHost, node EDITOR_HOST) does both: it
# renders the app's blocks (BlockNode::render with a FlowPosition and the RenderContext), runs
# collect_leaf_geometry around their prepaint with the app's decorations, and hands the leaves to the
# app after the frame (app code never runs mid-frame): an event of `[]gk.Leaf_Entry`, each a key and
# a gk.Leaf_Geometry (a temporary library object whose 10 reads are the upstream reads). The rules:
#
# - **Held values.** A data enum the app builds that a bound function also returns, whose variants
#   cannot be read back (`parse_markdown -> Result<Vec<BlockNode>, SharedString>`; BlockNode::Custom
#   holds a parsed MarkdownNode): read back, each value is a library object (`["held", rt]`, Go
#   `BlockNodeHeld`, a temporary handle; gk.Retain keeps it), and the enum gets one member more,
#   the held value, so a parsed block goes wherever the app's own blocks go (variant 0xfffe).
# - **Recursive data enums.** A variant holding a list of its own enum (`Root { children:
#   Vec<BlockNode> }`, Blockquote, Surface, List, ListItem) is built like any variant: the Go interface's
#   variant holds a slice of it, the decoder is the enum's own function.
# - **Iterators of iterators** read back are collected at each level (`Table::rows` ->
#   `[][]gk.Paragraph_Read`).

HOST = "gpui_kit::base::text::editor_host"
HELD_ENUMS = {HOST + "::BlockNode"}
HOST_ITEMS = {HOST + "::collect_leaf_geometry"}
GEOMETRY = HOST + "::LeafGeometry"
HELD_TAG = 0xFFFE


def host_args(m):
    """The editor host's arguments as descriptors: [(name, desc)] (also the arguments of the bridged
    collect_leaf_geometry item, so the description shows what the host takes)."""
    fr = next((i for n in m.nodes.values() for i in n["ops"] if i["id"] == HOST + "::RenderContext::with_foreground_ranges"), None)
    if fr is None or fr["status"] not in ("bound", "bridged"):
        raise SystemExit("gkgen: the editor host needs RenderContext::with_foreground_ranges's ranges")
    ranges = fr["args"][-1]["desc"]
    return [("blocks", ["list", ["denum", HOST + "::BlockNode"], "vec"]),
            ("flows", ["list", ["opt", ["node", HOST + "::FlowPosition"]], "vec"]),
            ("render_context", ["opt", ["node", HOST + "::RenderContext"]]),
            ("decorations", ["list", ["tuple", [["int", "u64"], ranges],
                                      "(u64, Vec<(std::ops::Range<usize>, gpui_kit::Hsla)>)"], "vec"])]


def held(m, rt, out):
    """A data enum of HELD_ENUMS read back: a value the library holds (gk_model.enum's hook)."""
    if not out or rt not in HELD_ENUMS:
        return None
    if rt in m.denums:
        m.denums[rt]["held"] = True
    else:
        m._editor_held = getattr(m, "_editor_held", set()) | {rt}
    return ["held", rt]


def recursive(m, rt, out):
    """A variant of a data enum of HELD_ENUMS that holds a list of the enum itself (gk_model.enum's
    hook where it refuses a recursive type): the enum by name, decoded by its own function."""
    if out or rt not in HELD_ENUMS:
        return None
    return ["denum", rt]


def host(m):
    """collect_leaf_geometry is what gk.block_host runs around its blocks' prepaint: bridged by the
    library's element, and the LeafGeometry objects it hands the app are reachable (`handed_out`)."""
    for it in m.all_items():
        if it["id"] in HOST_ITEMS and it["status"] == "todo":
            it.update(status="bridged", kind="host", reason="", host=True,
                      args=[{"name": n, "desc": d} for n, d in host_args(m)],
                      detail="gk.block_host: core/src/editor_text.rs BlockHost runs it around its blocks' prepaint "
                             "and hands the leaves to the app after the frame (PROTOCOLS.md 9.30)")
    for rt in getattr(m, "_editor_held", set()):
        if rt in m.denums:
            m.denums[rt]["held"] = True


def after_values(m):
    """Items the reachability pass refused for an argument of an `Rc<X>`/`Arc<X>` object that the
    values family makes constructible afterwards (gk_values.pointer_ctors: `Arc<MarkdownExtensions>`
    from the app's value), and reads of an iterator of iterators, which `put_ret` collects at each
    level (gk_values.nested_iterators refused them): given back their status (gk_model.settle's
    hook, after gk_values.post)."""
    g = _g()
    pre = getattr(m, "_editor_pre", {})
    lent_edits(m)  # 9.33
    flat_records(m)  # 9.34
    event_read_statuses(m)  # 9.35
    answer_sets(m)  # 9.36
    live = {k for k, st in m.objects.items() if st.get("reachable")}
    for k in live:
        if m.objects[k].get("pointer"):
            m.dead_objs.discard(k)  # made constructible after the reachability pass: a later prune keeps its users
    for it in m.all_items():
        was = pre.get(id(it))
        if was is None or was[0] not in ("bound", "bridged") or it["status"] != "todo":
            continue
        objs = [x for a in it.get("args", []) for x in g.walk_desc(a["desc"]) if x[0] == "obj"]
        if it.get("detail") == "an argument's type cannot be got" and objs and \
                all(x[2] in live and m.objects[x[2]].get("pointer") for x in objs):
            it["status"], it["reason"], it["detail"] = was[0], was[1], was[2]
            m._editor_late = getattr(m, "_editor_late", []) + [it["id"]]
        elif it.get("detail") == "an iterator of iterators" and it.get("ret") and it["ret"][0] == "iter" \
                and it["ret"][1][0] == "iter" and not any(x[0] == "iter" for x in g.walk_desc(it["ret"][1][1])):
            it["status"], it["reason"], it["detail"] = was[0], was[1], was[2]
            st = m.states.get(it.get("state", ""))
            if st is not None and it.get("kind") == "obj-read" and it not in st["reads"]:
                st["reads"].append(it)
            m._editor_late = getattr(m, "_editor_late", []) + [it["id"]]


def lent_edits(m):
    """`fn x(&mut self) -> &mut V` of kit state, V a value the app builds that has a Default
    (EditorState::lsp_mut -> &mut Lsp): an edit of the lent value, a state write that takes a value
    of V and applies its builder ops (not its constructor) to the state's own V in place: `ed->lsp_mut(cx,
    gk.lsp()->with_completion_highlighter(h))` (PROTOCOLS.md 9.33). Called from after_values."""
    for it in list(m.all_items()) + [i for st in m.states.values() for i in st.get("writes", [])]:
        det = it.get("detail") or ""
        if it["status"] != "todo" or not det.startswith("a borrow: &mut ") or it.get("lentmut"):
            continue
        st = m.states.get(it["owner"])
        name = det.rsplit("::", 1)[-1].split(" ")[-1].strip()
        nodes = [k for k, n in m.nodes.items() if n["name"] == name and not n["element"] and n["inst"] is None
                 and any(c["status"] in ("bound", "bridged") and c["name"] == "default" for c in n["ctors"])
                 and any(o["status"] in ("bound", "bridged") for o in n["ops"])]
        if st is None or st.get("obj") or len(nodes) != 1:
            continue
        key, T = nodes[0], st["path"]
        it.update(status="bridged", kind="state-write", reason="", detail=f"an edit of the lent {name}", lentmut=True,
                  state=st["key"], args=[{"name": "edit", "desc": ["lentmut", ["node", key]]}, {"name": "window", "desc": ["ctx", "window"]},
                                         {"name": "cx", "desc": ["ctx", "context"]}],
                  callee=(f"(|s: &mut {T}, e: crate::editor_text::LentEdit<{m.nodes[key]['path']}>, window: &mut gpui_kit::Window, "
                          f"cx: &mut gpui_kit::Context<{T}>| e.apply(s.{it['name']}(), window, cx))"))
        if it not in st["writes"]:
            st["writes"].append(it)
        for r in m.owners:
            if r["path"] == st.get("generic"):
                for sm in r["items"]:
                    if sm.get("summary") and sm["name"] == it["name"] and sm["status"] == "todo":
                        sm.update(status="bridged", reason="", detail=it["detail"])
                        sm.setdefault("also", []).append(it["id"])


def flat_records(m):
    """A state call whose argument is a struct with public fields one of which is a value node the app
    builds (`present_diagnostic(DiagnosticEntry { range, diagnostic: Diagnostic })`), refused as an
    opaque object (the struct is a record only read back, Diagnostic a record only there): its fields
    become the call's arguments (`ed->present_diagnostic(cx, range, gk.diagnostic(..))`) and the callee
    builds the struct (PROTOCOLS.md 9.34). Every family; called from after_values."""
    g = _g()
    import rdj  # noqa: PLC0415
    for st in m.states.values():
        if st.get("obj") or st.get("generic") is None and not st.get("ctors"):
            continue
        for it in [i for i in m.items if i["owner"] == st["key"]]:
            if it["status"] != "todo" or it.get("kind") or not (it.get("detail") or "").isidentifier():
                continue
            mt = re.fullmatch(r"fn (\w+)\(&mut self, (\w+): [\w:]*?(\w+)(, cx: &mut Context<'_, Self>)?\)", it["sig"])
            if not mt or mt.group(3) != it["detail"]:
                continue
            found = [(cr, i) for cr in g.KIT for i, (_, t) in m.api.walks[cr].types.items() if t["name"] == mt.group(3)]
            if len(found) != 1:
                continue
            hc, hid = found[0]
            f = m.info(hc, hid)
            sk = f["item"]["inner"]["struct"]["kind"]
            if not f["record"] or "plain" not in sk or sk["plain"]["has_stripped_fields"]:
                continue
            c = m.api.crates[hc]
            args, names = [], []
            try:
                for fid in sk["plain"]["fields"]:
                    fi = c.item(fid)
                    d = m.call_desc(g.Ctx(hc), fi["inner"]["struct_field"])
                    if m.call_bad(d, False) is not None or d[0] in ("str", "ref", "opt"):
                        raise g.Unsup("todo:x", "")
                    args.append({"name": fi["name"], "desc": d})
                    names.append(fi["name"])
            except g.Unsup:
                continue
            T, rp = st["path"], m.api.pubpath(hc, hid)
            ps = ", ".join(f"a{n}" for n in range(len(names)))
            fields = ", ".join(f"{nm}: a{n}" for n, nm in enumerate(names))
            cxp, cxa = (", cx: &mut gpui_kit::Context<%s>" % T, ", cx") if mt.group(4) else ("", "")
            it.update(status="bridged", kind="state-write", reason="", state=st["key"], flat=rp,
                      detail=f"{mt.group(3)}'s fields as the call's arguments",
                      args=args + ([{"name": "cx", "desc": ["ctx", "context"]}] if mt.group(4) else []),
                      callee=f"(|s: &mut {T}, {ps}{cxp}| s.{mt.group(1)}({rp} {{ {fields} }}{cxa}))")
            if it not in st["writes"]:
                st["writes"].append(it)
            for r in m.owners:
                if r["path"] == st.get("generic"):
                    for sm in r["items"]:
                        if sm.get("summary") and sm["name"] == it["name"] and sm["status"] == "todo":
                            sm.update(status="bridged", reason="", detail=it["detail"])
                            sm.setdefault("also", []).append(it["id"])


def handed_out_host(it):
    if it.get("host"):
        yield GEOMETRY


def put_ret(ret, expr):
    """A call op's result holding held values or an iterator of iterators (gk_rust.Rust.put_ret's
    hook), or None."""
    g = _g()
    if ret[0] == "iter" and ret[1][0] == "iter":
        return f"genapi::Wire::put(&({expr}).map(|i| i.collect::<Vec<_>>()).collect::<Vec<_>>(), &mut out)"
    if not any(x[0] == "held" for x in g.walk_desc(ret)):
        return None
    if ret[0] == "fallible" and ret[1][0] == "list" and ret[1][1][0] == "held":
        return ("genapi::Wire::put(&crate::values::Fallible(({expr}).map(|v| v.into_iter().map(genapi::Owned::new)"
                ".collect::<Vec<_>>())), &mut out)").format(expr=expr)
    if ret[0] == "list" and ret[1][0] == "held":
        return f"genapi::Wire::put(&({expr}).into_iter().map(genapi::Owned::new).collect::<Vec<_>>(), &mut out)"
    if ret[0] == "held":
        return f"genapi::Wire::put(&genapi::Owned::new({expr}), &mut out)"
    raise SystemExit(f"gkgen: a result holding held values in a form the editor rule does not write: {ret}")


def denum_arms(rs, e):
    """The decoder's arm of a held value: a copy of the value behind the handle (a released handle
    stops, as any object a tape names)."""
    if not e.get("held"):
        return []
    return [f"        {HELD_TAG} => genapi::obj_shown::<{e['rust']}>(r.u64()),"]


def rust_files(rs, groups):
    """n_editor_host.rs: the decoders gk.block_host's node uses (core/src/editor_text.rs), made from
    the descriptors of the values it takes (`host_args`), so a change upstream is a change here."""
    m = rs.m
    if not any(i.get("host") for i in m.all_items()):
        return
    a = dict(host_args(m))
    parts = [("block", a["blocks"][1], HOST + "::BlockNode"),
             ("flow", a["flows"][1][1], m.nodes[HOST + "::FlowPosition"]["path"]),
             ("context", a["render_context"][1], m.nodes[HOST + "::RenderContext"]["path"]),
             ("ranges", a["decorations"][1][1][1], "Vec<(std::ops::Range<usize>, gpui_kit::Hsla)>")]
    rs.open("n_editor_host.rs", "The editor host's decoders (docs/PROTOCOLS.md 9.30): what core/src/editor_text.rs "
            "BlockHost decodes from its node's arguments.")
    was, rs.call = rs.call, None
    for name, d, T in parts:
        rs.add(f"pub fn editor_host_{name}(r: &mut R, dx: &mut Dx) -> Option<{T}> {{ Some({rs.dec(d)}) }}")
    rs.call = was
    rs.close()
    groups.setdefault("n_editor_host", [])


def mutator(m, node, it, fn, ctx, tp):
    """`fn push(&mut self, x)` returning nothing, on a value the editor host takes, with no `self ->
    Self` form (Paragraph::push(InlineNode), TextMark::merge): an op applied in place, so the app
    builds a paragraph of inline nodes (gk_model.classify_method's hook before builder-mutator)."""
    import rdj  # noqa: PLC0415
    out = fn["sig"].get("output")
    if node["element"] or not node["path"].startswith(HOST + "::") or (out is not None and rdj.ty(out) != "()"):
        return False
    ins = fn["sig"]["inputs"][1:]
    if not ins or any(re.match(r"&(mut )?(gpui::)?(Window|App|Context)\b", rdj.ty(t)) for _, t in ins):
        return False
    ps = ", ".join(f"a{n}" for n in range(len(ins)))
    callee = f"(|mut s: {tp}, {ps}| {{ s.{it['name']}({ps}); s }})"
    if m.bindable(it, ctx, fn, ins, "op", node, callee):
        node["ops"].append(it)
    return True


# ---------------------------------------------------------------- values in a builder chain of a call op (9.31)

def chain_cx(m, d, kind, it):
    """A builder of a state's or an object's constructor chain whose argument the chain's decoder
    (`sb_*`, no context) cannot decode but a call op can (a value node the app builds, a colour):
    bound, and its constructor decodes the whole chain before it runs, with the call's context
    (gk_model's state and object builder hooks). True when the rule takes it."""
    if kind not in ("state-builder", "obj-builder") or m.call_bad(d, False) is not None:
        return False
    if d == ["leaf", "color", ""]:
        # `impl Into<Hsla>` (ColorPickerState::default_value): named, so the chain decoder's
        # placeholder (`None?`) has a type.
        d[:] = ["from", ["leaf", "color", ""], "gpui_kit::Hsla", "gpui_kit::Hsla"]
    it["chain_cx"] = True
    return True


def chain_body(rs, st, body):
    """A constructor's call op whose chain holds a builder of `chain_cx`: the chain is decoded first
    into closures (`sbp_*`, with the call's context: a value node is a tape of its own, 13.6), then
    applied to the new value (gk_rust.Rust.calls' and obj_calls' hook). The body unchanged otherwise."""
    ok = [i for i in st["builders"] if i["status"] in ("bound", "bridged")]
    if not any(i.get("chain_cx") for i in ok):
        return body
    fn, T = st["fn"], st["path"]
    done = getattr(rs, "_editor_sbp", set())
    if fn not in done:
        rs._editor_sbp = done | {fn}
        was = rs.call
        rs.call = "state-write"
        rs.add(f"fn sbp_{fn}(op: u16, r: &mut R, c: &mut genapi::Call) -> genapi::GkResult<Box<dyn FnOnce({T}) -> {T}>> {{",
               ["state", st["key"]])
        rs.add("    Ok(match op {")
        for it in sorted(ok, key=lambda i: i["op"]):
            lets = "".join(f"let a{n} = {rs.dec(a['desc'])}; " for n, a in enumerate(it["args"]) if a["desc"][0] != "ctx")
            passed = ", ".join(rs.passed(a["desc"], f"a{n}") for n, a in enumerate(it["args"]))
            sep = ", " if passed else ""
            rs.add(f"        {it['op']} => {{ {lets}Box::new(move |s: {T}| {it['callee']}(s{sep}{passed})) }}", ["item", it["id"]])
        rs.add(f"        _ => Box::new(|s: {T}| s),")
        rs.add("    })")
        rs.add("}")
        rs.call = was
    loop = f"for _ in 0..r.u16() {{ let op = r.u16(); s = sb_{fn}(s, op, r); }}"
    if loop not in body:
        raise SystemExit(f"gkgen: the editor rule cannot find the builder chain of {T}'s constructor")
    if "let entity = c.cx.new(" in body:
        pre = (f"let chain = {{ let n = r.u16(); let mut v: Vec<Box<dyn FnOnce({T}) -> {T}>> = Vec::new(); "
               f"for _ in 0..n {{ let op = r.u16(); v.push(sbp_{fn}(op, r, c)?); }} v }}; ")
        return body.replace(loop, "for f in chain { s = f(s); }").replace("let entity = c.cx.new(", pre + "let entity = c.cx.new(", 1)
    return body.replace(loop, f"for _ in 0..r.u16() {{ let op = r.u16(); s = sbp_{fn}(op, r, c)?(s); }}")


def _family(path):
    """The STATUS family of an upstream path (gen/gkgen.py FAMILIES, first match wins)."""
    import sys  # noqa: PLC0415
    main = sys.modules.get("__main__")
    fams = getattr(main, "FAMILIES", None)
    if fams is None:
        import gkgen  # noqa: PLC0415
        fams = gkgen.FAMILIES
    low = path.lower()
    return next((name for name, pat in fams if re.search(pat, low)), "other components and values")


# ---------------------------------------------------------------- decorators kept for later (9.32)

STYLE_NODE = "gpui_kit::StyleRefinement"


def decorate(m, key, kind, params, wrap, ptypes):
    """`impl FnOnce(X) -> X + 'static` with no window and app, X an element that is Styled and a
    ParentElement (NumberInput::increment_button(Button), MessageScroller::with_jump_button_renderer):
    upstream keeps it and calls it while it renders, where the app cannot run and no decode context
    is left. The app gives what it would add as data (Go `gk.Decorate`): a style, decoded where
    the closure is decoded (its colours resolved then), and content, a row of a row source laid out
    where the window is (as the fold placeholder's, 9.22). gk_model.closure_desc's hook before it
    refuses a build closure without a window; None when the rule does not apply."""
    nd = m.nodes.get(key)
    if nd is None or not nd["element"] or params != ["arg"] or kind != "FnOnce" or STYLE_NODE not in m.nodes:
        return None
    traits = m.info(nd["crate"], nd["id"])["traits"]
    if "Styled" not in traits or "ParentElement" not in traits:
        return None
    return ["decorate", key, wrap, ptypes]


def rust_decorate(rs, d):
    _, key, wrap, ptypes = d
    fn = rs.m.nodes[STYLE_NODE]["fn"]
    body = ("{ let s = if r.u8() != 0 { Some(build_%s(dx, r.u32())?) } else { None }; let src = r.u64(); "
            "move |b: %s| crate::editor_text::decorate(b, s, src) }" % (fn, ptypes[0]))
    return rs.wrap(wrap, body)


# ---------------------------------------------------------------- event payloads with reads (9.35)

def event_reads(m, st, crate, ctx, tpath, t):
    """`EventEmitter<E>` of kit state where E has no public field and is no object the binding holds,
    but has `&self` reads that take nothing (`InputEditsEvent::edits() -> &[(InputEdit, Rope)]`):
    the payload is a record of the reads' results, copied when the event is raised (`Input_Edits_Event
    {edits = ..}`); the reads count as bridged by it (gk_model's state rule, where it refused E)."""
    g = _g()
    import json as _json  # noqa: PLC0415
    import rdj  # noqa: PLC0415
    if "resolved_path" not in t or g.path_args(t["resolved_path"]):
        return
    try:
        hc, hid, it = m.api.item(crate, t["resolved_path"]["id"])
    except Exception:  # noqa: BLE001
        return
    if it is None or "struct" not in it.get("inner", {}):
        return
    f = m.info(hc, hid)
    if f["record"] or f["generic"]:
        return
    c = m.api.crates[hc]
    reads = []
    for imp_id in it["inner"]["struct"].get("impls", []):
        ii = c.item(imp_id)
        im = ii["inner"]["impl"] if ii else None
        if not im or im.get("trait") is not None or im.get("is_synthetic"):
            continue
        for mid in im["items"]:
            fi = c.item(mid)
            if not fi or "function" not in fi["inner"] or fi["visibility"] != "public":
                continue
            sig = fi["inner"]["function"]["sig"]
            if len(sig["inputs"]) != 1 or rdj.ty(sig["inputs"][0][1]) != "&Self" or not sig.get("output"):
                return
            reads.append((fi["name"], sig["output"]))
    if not reads:
        return
    rt = m.rtype(g.Ctx(crate, ctx.self_t, ctx.subst), t)
    fields = []
    for name, ot in reads:
        m._closure += 1
        try:
            d = m.desc(g.Ctx(hc), ot, out=True)
        except g.Unsup:
            return
        finally:
            m._closure -= 1
        if any(x[0] in ("obj", "node", "handle", "entity", "shim", "lent", "rows") for x in g.walk_desc(d)):
            return
        fields.append({"name": name, "desc": d})
    if rt not in m.records:
        m.records[rt] = {"rust": rt, "path": rt, "name": f["name"], "fields": fields, "tuple": False, "complete": False,
                         "in": False, "out": True, "crate": hc, "args": [], "reads_of": m.api.pubpath(hc, hid)}
    d = ["rec", rt]
    key = _json.dumps([[d], "unit"])
    m.shapes.setdefault(key, {"key": key, "args": [d], "ret": "unit", "rust": [rdj.ty(t)], "builtin": None})
    if any(e["shape"] == key for e in st["events"]):
        return
    ev = {"name": f["name"], "shape": key, "rust": rt, "id": f"{tpath}::subscribe::{rdj.ty(t)}", "status": "bridged"}
    st["events"].append(ev)
    m._editor_event_reads = getattr(m, "_editor_event_reads", []) + [(st, ev, rt, key, m.api.pubpath(hc, hid))]


def event_read_statuses(m):
    """The reads of an event payload made of them (9.35) are bridged by it. A payload whose event no
    state the binding keeps raises (the state was made again or left to a hand-written protocol: the
    kit's DockArea raises DockToggleEvent, which `gk.OnDockToggle` delivers, rt_dock.go) loses its
    record and shape, so no type is generated that nothing hands the app."""
    kept, owners = set(), set()
    for st, ev, rt, key, owner in getattr(m, "_editor_event_reads", []):
        if m.states.get(st["key"]) is st and any(e is ev for e in st["events"]):
            kept.add((rt, key))
            owners.add(owner)
    for _, _, rt, key, _ in getattr(m, "_editor_event_reads", []):
        if (rt, key) not in kept:
            m.records.pop(rt, None)
            if not any(e["shape"] == key for s in m.states.values() for e in s["events"]):
                m.shapes.pop(key, None)
    for it in m.all_items():
        if it["id"].rsplit("::", 1)[0] in owners and it["status"] in ("todo", "norule"):
            it.update(status="bridged", kind="event-read", reason="",
                      detail="a field of the event's payload, read when the event is raised (PROTOCOLS.md 9.35)")


def reads_put(rc):
    """The Wire impl of an event payload made of its reads (gk_rust's record hook), or None."""
    if not rc.get("reads_of"):
        return None
    return " ".join(f"genapi::Wire::put(&self.{f['name']}(), out);" for f in rc["fields"])


# ---------------------------------------------------------------- answers given in advance (9.36)

def answer_sets(m):
    """`set_x(&mut self, handler: impl Fn(E, Box<dyn Action>, &mut Window, &mut Context<..>) -> bool)`,
    E an enum the binding reads (EditorState::set_overlay_action_handler(InputOverlayKind, action)):
    upstream needs the answer at once, while it dispatches a key, so the app gives it in advance: the
    pairs (E, action name) it takes; the handler answers true for those (upstream then leaves the action
    to the app's own key bindings and, for Enter and Escape, closes the overlay)
    (PROTOCOLS.md 9.36)."""
    g = _g()
    pat = re.compile(r"fn (\w+)\(&mut self, (\w+): impl Fn\((\w+), Box<dyn gpui::Action>, &mut Window, &mut Context<'_, [\w<>]+>\) -> bool \+ 'static\)")
    for st in m.states.values():
        if st.get("obj") or not st.get("ctors"):
            continue
        for it in [i for i in m.items if i["owner"] == st["key"]]:
            mt = pat.fullmatch(it["sig"]) if it["status"] == "todo" else None
            if not mt:
                continue
            found = [(cr, i) for cr in g.KIT for i, (_, t) in m.api.walks[cr].types.items() if t["name"] == mt.group(3)]
            if len(found) != 1:
                continue
            hc, hid = found[0]
            try:
                ed = m.desc(g.Ctx(hc), {"resolved_path": {"path": mt.group(3), "id": int(hid), "args": None}})
            except g.Unsup:
                continue
            if ed[0] != "enum":
                continue
            T, rt = st["path"], m.api.pubpath(hc, hid)
            it.update(status="bridged", kind="state-write", reason="", state=st["key"],
                      detail=f"the ({mt.group(3)}, action name) pairs the app takes, given in advance",
                      args=[{"name": "taken", "desc": ["list", ["tuple", [ed, ["str", "String"]], f"({rt}, String)"], "vec"]}],
                      callee=(f"(|s: &mut {T}, keys: Vec<({rt}, String)>| s.{mt.group(1)}(move |k, a, _, _| "
                              "keys.iter().any(|(kk, n)| std::mem::discriminant(kk) == std::mem::discriminant(&k) && n.as_str() == a.name())))"))
            if it not in st["writes"]:
                st["writes"].append(it)
            for r in m.owners:
                if r["path"] == st.get("generic"):
                    for sm in r["items"]:
                        if sm.get("summary") and sm["name"] == it["name"] and sm["status"] == "todo":
                            sm.update(status="bridged", reason="", detail=it["detail"])
                            sm.setdefault("also", []).append(it["id"])


# ---------------------------------------------------------------- actions read back (9.37)

def action_out(m):
    """`Box<dyn Action>` read back (a context menu's `NativeMenuItem::Action { label, disabled, action }`
    handed to `on_context_menu`, gpui's `OwnedMenuItem::Action` of the app's menus): the action's id as
    the app knows actions (`gk.Action`, PROTOCOLS.md 11.1; a kit action gets an id the first time it is
    seen, `app::id_of`), so the app reads every item of a menu and can compare it with `gk.action(cx,
    "input::Copy")`. Wire for `Box<dyn Action>` in core/src/editor_text.rs. gk_model.resolved's hook.
    Every type: a value that holds an action and also goes into the library is decoded in
    a call op by the action's id (`call_bad`), so reading it back costs no call its argument."""
    return True
