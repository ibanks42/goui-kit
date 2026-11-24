"""Editor, text-view, Markdown plugin, and `BlockHost` rules for model and Rust emission.

This module extends `gen/gk_model.py` and `gen/gk_rust.py` with the specialized
rules required by the text editor, syntax highlighter, Markdown renderer, and
rich-text block host:

1. **Editor closure descriptors (`closure`, `alias`, `ask_trait`, `decorate`)**:
   - `tfrag`: zero-payload `(&mut Window, &mut App) -> ConcreteElement` returning a
     concrete non-instantiated element node (wrapped in `MainThread` + `bad_fragment`).
   - `keyset`: `Fn(str|int) -> bool` predicates sent as an allow-list slice on the wire.
   - `table`: `Fn(u8) -> V` lookup functions sent as an indexed `Vec<V>`.
   - `ctxlazy`: 1..3-arg element closures with no Window/App, rendered lazily via
     `crate::editor_text::lazy_row`.
   - `viewrows`: 1..3-arg `(&..., &mut Window, &mut App) -> AnyView` closures via
     `crate::editor_text::ctx_view`.
   - `askdyn`: `Arc`/`Rc`/`Box<dyn Trait>` single-method trait objects in `ASK_TRAITS`
     (such as `HighlightStyleResolver`), backed by an async `Ask` lookup table.
   - `decorate`: `FnOnce(El) -> El` on `Styled + ParentElement` nodes lowered to an
     optional `StyleRefinement` plus child row source.
2. **Markdown plugins and held recursive enums (`bounded`, `held`, `recursive`, `denum_arms`)**:
   - `mdplug`: closed union over `PLUGIN_NODES` (enforced against rustdoc so new
     upstream `MarkdownPlugin` structs cannot be silently missed) plus `DataPlugin`.
   - `HELD_ENUMS` (`BlockNode`): recursive AST enums cannot be serialized back to Go
     in full without unbounded nesting, so outbound `BlockNode` values become opaque
     `held` handles and inbound `BlockNode::Unwire` accepts sentinel tag `HELD_TAG`
     (`0xFFFE`) to recover a held object handle.
3. **Call-op overrides and inline color records (`call_bad`, `rust_inline_rec`, `chain_cx`, `chain_body`)**:
   - Permits `Box<dyn Action>` inside immediate calls and decodes color-bearing records
     field-by-field inside call handlers (`rust_inline_rec`) because `Hsla` lookup needs
     `&c.cx` rather than a plain `Unwire` reader.
   - Upgrades state/object constructor builder loops (`sb_<fn>`) to `sbp_<fn>` returning
     `GkResult<Box<dyn FnOnce(T) -> T>>` when any builder needs `&mut Call` context,
     decoding those arguments before entering `cx.new(|cx| ...)`.
4. **Post-reachability bridges (`before_reach`, `revive`, `project`, `after_values`, `lent_edits`, `flat_records`, `event_reads`, `answer_sets`, `rust_files`)**:
   - `before_reach()` snapshots object methods/statuses before pruning so `revive()` and
     `after_values()` can restore objects exposed via readback getters or pointer ctors,
     as well as direct `Iterator<Item = Iterator<...>>` returns.
   - `project()` lifts pure `&self` getters from non-`Clone` borrowed sub-objects onto
     their owning state as `<getter>_<field>` reads.
   - `lent_edits()` bridges `&mut self -> &mut ValueNode` methods via `LentEdit` op-tape
     application instead of exposing mutable borrows across FFI.
   - `flat_records()` flattens single-record `&mut self` state writes into plain scalar
     call arguments; `event_reads()` synthesizes event payload records from private-field
     `EventEmitter` types' `&self` getters; `answer_sets()` lowers action-filter closures
     into precomputed `(Enum, action_name)` pairs; `rust_files()` emits `n_editor_host.rs`
     for `BlockHost`.
"""
import json
import re

# Concrete kit MarkdownPlugin structs mirrored by `core/src/editor_text.rs GkPlugin` variants.
# `bounded()` asserts this list matches rustdoc exactly so new upstream plugins trigger a loud error.
PLUGIN_NODES = ["gpui_kit::component::text::FrontmatterPlugin"]
PLUGIN_VARIANTS = {"gpui_kit::component::text::FrontmatterPlugin": "Frontmatter"}
PLUGIN_TRAITS = ("MarkdownPlugin", "TextViewPlugin")


def _g():
    import gk_model
    return gk_model


def _is_element_out(m, ctx, output):
    """Return True if closure output type `output` is `AnyElement` or `impl IntoElement`."""
    import rdj
    s = rdj.ty(output)
    return "impl_trait" in output and "IntoElement" in s or s.rsplit("::", 1)[-1] == "AnyElement"


def _plain(m, ctx, payload, objects_ok):
    """Validate that closure payload arguments can be serialized out to Go row callbacks."""
    import gk_lists
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
    """Classify editor-related closures into `tfrag`, `keyset`, `table`, `ctxlazy`, or `viewrows` descriptors.

    - `tfrag`: zero-payload `(&mut Window, &mut App) -> ConcreteElement` returning a typed element fragment.
    - `keyset`: single-arg string/int predicate `-> bool` sent as an explicit allowed-key list.
    - `table`: `u8 -> V` lookup closure sent as an indexed `Vec<V>` table.
    - `ctxlazy`: 1..3-arg element closure without Window/App, rendered lazily via `editor_text::lazy_row`.
    - `viewrows`: 1..3-arg `(&..., &mut Window, &mut App) -> AnyView` row closure via `editor_text::ctx_view`.
    """
    import rdj
    g = _g()
    if output is None or "context" in params:
        return None
    out_s = rdj.ty(output)
    has_cx = "window" in params and "app" in params
    if not payload and has_cx and "resolved_path" in output:
        try:
            d = m.desc(ctx, output)
        except g.Unsup:
            d = None
        if d is not None and d[0] == "node" and m.nodes[d[1]]["element"] and m.nodes[d[1]]["inst"] is None:
            return ["tfrag", d[1], params, wrap, ptypes]
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


def bounded(m, ctx, bounds, out):
    """Match `impl MarkdownPlugin` / `impl TextViewPlugin` bounds as the `mdplug` tagged union.

    Verifies that the set of concrete upstream plugin structs matches `PLUGIN_NODES` so
    new kit plugins cannot be silently omitted from `core/src/editor_text.rs GkPlugin`.
    """
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
    return ["mdplug", "crate::editor_text::DataPlugin"] + [["node", k] for k in keys]


def handed_out(it):
    """Yield object paths made reachable by `mdplug` (`InlineRenderContext`) or `BlockHost` (`LeafGeometry`)."""
    g = _g()
    for a in it.get("args", []):
        for x in g.walk_desc(a["desc"]):
            if x[0] == "mdplug":
                yield "gpui_kit::base::InlineRenderContext"
    yield from handed_out_host(it)


def gpui_value(m, hc, name, args, out):
    """Classify outbound `gpui::ElementId` (as string) and inbound `gpui::EntityId` (as raw u64 handle)."""
    g = _g()
    if hc not in g.GPUI_HOMES or args:
        return None
    if name == "ElementId" and out:
        return ["str", "ElementId"]
    if name == "EntityId" and not out:
        return ["entityid"]
    return None


# Editor descriptor tags safe to decode in immediate calls without a view Dx.
CALL_SAFE = {"keyset", "table", "ctxlazy", "viewrows", "entityid", "askdyn", "held", "lentmut"}


def before_reach(m):
    """Snapshot object methods and item statuses before reachability pruning, and mark BlockHost items.

    `revive()` and `after_values()` consult these snapshots later to restore objects and
    items that become reachable through synthesized `read:*` getters, pointer constructors,
    or nested-iterator unwrapping.
    """
    m._editor_kept = {key: {grp: list(st[grp]) for grp in ("builders", "reads", "writes", "calls")}
                      for key, st in m.objects.items()}
    m._editor_status = {id(it): (it["status"], it["reason"], it.get("detail"))
                        for st in m.objects.values()
                        for it in st["builders"] + st["reads"] + st["writes"] + st["calls"] + st["statics"]}
    host(m)
    m._editor_pre = {id(it): (it["status"], it["reason"], it.get("detail")) for it in m.all_items()}


def revive(m):
    """Revive pruned library objects that are returned by pure `&self` getters on `read:*` readback objects."""
    import rdj
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
                m._closure += 1
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


def rust_dec(rs, d):
    """Emit Rust decoder expressions for editor descriptors and inline color-bearing records."""
    k = d[0]
    if rs.call == "builder" and (k == "node" or d[:2] == ["leaf", "color"]):
        return "None?"
    if d == ["action", "box"] and rs.call not in (None, "builder", "codec"):
        return "genapi::action(r)?"
    ir = rust_inline_rec(rs, d)
    if ir is not None:
        return ir
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
        return rust_decorate(rs, d)
    if k == "lentmut":
        return f"crate::editor_text::LentEdit::read(r, apply_{rs.m.nodes[d[1][1]]['fn']})?"
    return None


def _rust_ctx(rs, d, lazy):
    """Emit Rust decoder for `ctxlazy` (`lazy=True`) and `viewrows` (`lazy=False`) closures."""
    import gk_lists
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
    """Return a deterministic JSON summary of editor `CALL_SAFE` tags."""
    return json.dumps(sorted(CALL_SAFE))


def alias(m, ctx, t, out):
    """Unwrap `Rc`/`Arc`/`Box` around dyn-trait type aliases and detect `ctxlazy`/`viewrows`/`askdyn` aliases."""
    g = _g()
    if out:
        return None
    ad = ask_trait(m, ctx, t, out)
    if ad is not None:
        return ad
    rp = t["resolved_path"]
    pa = g.path_args(rp)
    if rp["path"].rsplit("::", 1)[-1] in ("Rc", "Arc", "Box") and len(pa) == 1 and "resolved_path" in pa[0]:
        try:
            ahc, ahid, ait = m.api.item(ctx.crate, pa[0]["resolved_path"]["id"])
        except Exception:
            ait = None
        if ait is not None and ahc == ctx.crate and "dyn_trait" in ait.get("inner", {}).get("type_alias", {}).get("type", {}):
            import copy
            t2 = copy.deepcopy(t)
            t2["resolved_path"]["args"]["angle_bracketed"]["args"][0] = {"type": ait["inner"]["type_alias"]["type"]}
            return m.desc(ctx, t2, out)
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:
        return None
    if it is None or "type_alias" not in it.get("inner", {}):
        return None
    try:
        d = m.desc(g.Ctx(hc), it["inner"]["type_alias"]["type"], out)
    except g.Unsup:
        return None
    return d if d[0] in ("ctxlazy", "viewrows", "askdyn") else None


# Descriptor tags allowed inside inline-decoded color-bearing records passed to immediate calls.
_INLINE_OK = {"bool", "int", "f32", "f64", "str", "enum", "opt", "list", "ref", "tuple", "rec", "from"}


def _has_color(m, d, seen=None):
    """Return True if descriptor `d` (or any nested record) contains a `["leaf", "color", ...]` field."""
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
    """Return the first disallowed descriptor tag inside a color-bearing record, or None if inline-safe."""
    import gk_lists
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
    """Editor override for `Model.call_bad`: permits boxed `Action` values and inline-decoded color records.

    Returns a 1-tuple `(result,)` to override `Model.call_bad`, or `None` to fall back
    to the standard check. Records containing `Hsla`/`Color` fields cannot implement
    `Unwire` directly when mixed with other editor types, so `rust_inline_rec` decodes
    them field-by-field inside the call handler instead.
    """
    if builder:
        return None
    if d == ["action", "box"]:
        return (None,)
    g = _g()
    if not _ACTION_INSIDE[0] and _reaches(m, d, ("action", "box")) and not _reaches(m, d, ("action", "listener")):
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
    """Return True if descriptor `d` (recursing into `rec`/`denum`/`into`) contains a node with prefix `head`."""
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
    """Decode a color-bearing record inline inside a call handler instead of via `genapi::Unwire`."""
    inside = getattr(rs, "_editor_inline", False)
    if d[0] != "rec" or rs.call in (None, "builder", "codec") or not (inside or _has_color(rs.m, d)):
        return None
    r = rs.m.records[d[1]]
    rs._editor_inline = True
    try:
        if r["tuple"]:
            return "%s(%s)" % (d[1], ", ".join(rs.dec(f["desc"]) for f in r["fields"]))
        return "%s { %s }" % (d[1], ", ".join(f"{f['name']}: {rs.dec(f['desc'])}" for f in r["fields"]))
    finally:
        rs._editor_inline = inside


# Single-method dyn traits served by async Ask tables via core/src/editor_text.rs helpers.
ASK_TRAITS = {"HighlightStyleResolver": "crate::editor_text::style_resolver"}


def ask_trait(m, ctx, t, out):
    """Match `Arc`/`Rc`/`Box<dyn Trait>` for traits in `ASK_TRAITS` as an `askdyn` descriptor."""
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
    """Emit Rust decoder for an `askdyn` trait object shim."""
    _, key, ret, shim, trait = d
    rt = rs.m._editor_ask_rt[trait]
    was = rs.call
    rs.call = None
    try:
        dec = rs.dec(ret)
    finally:
        rs.call = was
    return (f"{shim}(r.u64(), |r: &mut genapi::R, dx: &mut genapi::Dx| -> Option<{rt}> {{ let _ = &dx; Some({dec}) }})")


def project(m):
    """Project pure read methods from non-`Clone` borrowed sub-objects onto their owning state.

    When a state method `fn sub(&self) -> &SubObj` fails with "a result that cannot be copied
    out of a borrow" (because `SubObj` is not `Clone`), every zero-context read method on
    `SubObj` is lifted directly onto the owning state as `<sub_name>_<read_name>`, cloning
    any borrowed `&T`/`Option<&T>` return via `ToOwned::to_owned`. Marks the original getter
    `kind="projected"` so `gk_go_bridges.covered()` verifies at least one projected read landed
    in Go.
    """
    import re
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
                call = f"std::borrow::ToOwned::to_owned({call})"
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
            for r in m.owners:
                if r["path"] != st.get("generic"):
                    continue
                for sm in r["items"]:
                    if sm.get("summary") and sm["name"] == mt.group(1) and sm["status"] == "todo":
                        sm.update(status="bridged", reason="", detail=it["detail"])
                        sm.setdefault("also", []).append(it["id"])


LEFT = {
}


def left_reasons(m):
    items = list(m.all_items()) + [it for r in m.owners for it in r["items"]]
    for it in items:
        if it.get("status") != "todo":
            continue
        for tail, why in LEFT.items():
            if it["id"].endswith(tail) or (tail.startswith("::") and it["id"].endswith(tail)) or tail in it["id"]:
                it["reason"] = why
                break


def variant_ctors(m, f, rt, variants, out):
    """Enable data-enum variants with private fields using `[editor].variants` constructor overrides."""
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
    """Fail if any `[editor].variants` override entry was not matched during model construction."""
    for key in m.ov.get("editor", {}).get("variants", {}):
        if "editor.variants." + key not in m.ov_used:
            raise SystemExit(f"gkgen: overrides [editor] variants names nothing the generator met: {key}")


def variant_new(rs, e, v):
    """Emit Rust constructor expression for a variant bound through `[editor].variants`."""
    if "via" not in v:
        return None
    lets = " ".join(f"let {f['name']} = {rs.dec(f['desc'])};" for f in v["fields"])
    return "{ %s %s }" % (lets, v["via"])


# BlockHost module paths and sentinel wire tag (0xFFFE) used to reference held BlockNode handles.
HOST = "gpui_kit::base::text::editor_host"
HELD_ENUMS = {HOST + "::BlockNode"}
HOST_ITEMS = {HOST + "::collect_leaf_geometry"}
GEOMETRY = HOST + "::LeafGeometry"
HELD_TAG = 0xFFFE


def host_args(m):
    """Return the `(name, desc)` argument list decoded by `n_editor_host.rs` for `BlockHost`."""
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
    """Mark an outbound recursive enum in `HELD_ENUMS` (e.g. `BlockNode`) to be returned as an owned handle."""
    if not out or rt not in HELD_ENUMS:
        return None
    if rt in m.denums:
        m.denums[rt]["held"] = True
    else:
        m._editor_held = getattr(m, "_editor_held", set()) | {rt}
    return ["held", rt]


def recursive(m, rt, out):
    """Allow inbound self-referential fields on `HELD_ENUMS` without triggering cycle rejection."""
    if out or rt not in HELD_ENUMS:
        return None
    return ["denum", rt]


def host(m):
    """Mark `collect_leaf_geometry` as bridged by `BlockHost` and apply deferred `held` flags."""
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
    """Run late editor passes: lent edits, flat record writes, event reads, answer sets, and revivals.

    After `gk_values.post()` introduces pointer constructors (`Rc::new`/`Arc::new`) and
    filters nested iterators, this pass restores state/object methods whose arguments or
    `Iterator<Item = Iterator<...>>` returns are now supported by editor codecs.
    """
    g = _g()
    pre = getattr(m, "_editor_pre", {})
    lent_edits(m)
    flat_records(m)
    event_read_statuses(m)
    answer_sets(m)
    live = {k for k, st in m.objects.items() if st.get("reachable")}
    for k in live:
        if m.objects[k].get("pointer"):
            m.dead_objs.discard(k)
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
    """Bridge `&mut self -> &mut ValueNode` methods via `LentEdit` op-tape application.

    Instead of handing out a mutable reference across FFI, Go records builder ops on the
    borrowed node type and submits them in a single `LentEdit` call that runs inside
    the owning state's `&mut self` borrow.
    """
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
    """Flatten `&mut self` state writes taking a plain non-str/opt record struct into individual call arguments."""
    g = _g()
    import rdj
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
    """Yield `LeafGeometry` when an item is a `BlockHost` host entry."""
    if it.get("host"):
        yield GEOMETRY


def put_ret(ret, expr):
    """Emit Rust wire serialization for nested iterators (`iter` of `iter`) and `held` enum handles."""
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
    """Append the `HELD_TAG` (`0xFFFE`) match arm to `Unwire` for held recursive enums (`BlockNode`)."""
    if not e.get("held"):
        return []
    return [f"        {HELD_TAG} => genapi::obj_shown::<{e['rust']}>(r.u64()),"]


def rust_files(rs, groups):
    """Emit `core/src/gen/n_editor_host.rs` with tape argument decoders for `BlockHost`."""
    m = rs.m
    if not any(i.get("host") for i in m.all_items()):
        return
    a = dict(host_args(m))
    parts = [("block", a["blocks"][1], HOST + "::BlockNode"),
             ("flow", a["flows"][1][1], m.nodes[HOST + "::FlowPosition"]["path"]),
             ("context", a["render_context"][1], m.nodes[HOST + "::RenderContext"]["path"]),
             ("ranges", a["decorations"][1][1][1], "Vec<(std::ops::Range<usize>, gpui_kit::Hsla)>")]
    rs.open("n_editor_host.rs", "Argument decoders for core/src/editor_text.rs BlockHost nodes.")
    was, rs.call = rs.call, None
    for name, d, T in parts:
        rs.add(f"pub fn editor_host_{name}(r: &mut R, dx: &mut Dx) -> Option<{T}> {{ Some({rs.dec(d)}) }}")
    rs.call = was
    rs.close()
    groups.setdefault("n_editor_host", [])


def mutator(m, node, it, fn, ctx, tp):
    """Adapt `&mut self -> ()` setters on `editor_host` value nodes into `self -> Self` builder ops."""
    import rdj
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


def chain_cx(m, d, kind, it):
    """Allow a state/object constructor builder op to decode arguments that need `genapi::Call` context."""
    if kind not in ("state-builder", "obj-builder") or m.call_bad(d, False) is not None:
        return False
    if d == ["leaf", "color", ""]:
        d[:] = ["from", ["leaf", "color", ""], "gpui_kit::Hsla", "gpui_kit::Hsla"]
    it["chain_cx"] = True
    return True


def chain_body(rs, st, body):
    """Rewrite a state/object constructor builder loop to pass `Call` context when any builder needs it.

    Normal `sb_<fn>` builder loops only receive `&mut R`. When any builder on `st` has
    `chain_cx=True`, this emits `sbp_<fn>` returning `GkResult<Box<dyn FnOnce(T) -> T>>`
    so arguments needing `&mut Call` (such as handles or views) are decoded before entering
    the `cx.new(|cx| ...)` closure.
    """
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
    """Map an upstream item path to its documentation family bucket."""
    import sys
    main = sys.modules.get("__main__")
    fams = getattr(main, "FAMILIES", None)
    if fams is None:
        import gkgen
        fams = gkgen.FAMILIES
    low = path.lower()
    return next((name for name, pat in fams if re.search(pat, low)), "other components and values")


STYLE_NODE = "gpui_kit::StyleRefinement"


def decorate(m, key, kind, params, wrap, ptypes):
    """Match a `FnOnce(El) -> El` decorator closure on a `Styled + ParentElement` element node."""
    nd = m.nodes.get(key)
    if nd is None or not nd["element"] or params != ["arg"] or kind != "FnOnce" or STYLE_NODE not in m.nodes:
        return None
    traits = m.info(nd["crate"], nd["id"])["traits"]
    if "Styled" not in traits or "ParentElement" not in traits:
        return None
    return ["decorate", key, wrap, ptypes]


def rust_decorate(rs, d):
    """Emit Rust decoder for `decorate`: applies an optional `StyleRefinement` and child row source."""
    _, key, wrap, ptypes = d
    fn = rs.m.nodes[STYLE_NODE]["fn"]
    body = ("{ let s = if r.u8() != 0 { Some(build_%s(dx, r.u32())?) } else { None }; let src = r.u64(); "
            "move |b: %s| crate::editor_text::decorate(b, s, src) }" % (fn, ptypes[0]))
    return rs.wrap(wrap, body)


def event_reads(m, st, crate, ctx, tpath, t):
    """Synthesize an event payload record from an `EventEmitter` type's `&self` getter methods.

    Used when an entity emits a private-field event struct that exposes all its data via
    zero-arg `&self` getters; the generated `Wire` impl calls each getter when the event fires.
    """
    g = _g()
    import json as _json
    import rdj
    if "resolved_path" not in t or g.path_args(t["resolved_path"]):
        return
    try:
        hc, hid, it = m.api.item(crate, t["resolved_path"]["id"])
    except Exception:
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
    """Prune unused `event_reads` records/shapes and mark active event getters as `bridged`."""
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
    """Generate `Wire::put` statements for a synthesized `event_reads` record."""
    if not rc.get("reads_of"):
        return None
    return " ".join(f"genapi::Wire::put(&self.{f['name']}(), out);" for f in rc["fields"])


def answer_sets(m):
    """Bridge `Fn(Enum, Box<dyn Action>, &mut Window, &mut Context<Self>) -> bool` predicates as `(Enum, action_name)` lists."""
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


def action_out(m):
    """Signal that outbound `&dyn Action` wire serialization is supported."""
    return True
