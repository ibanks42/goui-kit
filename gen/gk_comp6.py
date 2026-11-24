"""Specialized descriptor rules for tooltips, closure snapshots, value sets, and kit renderers.

Extends `gen/gk_values.py`, `gen/gk_lists.py`, and `gen/gk_rust.py` with the sixth
generation of component rules:

1. **Tooltip-capable `viewany` (`viewany_tooltips`, `rust_viewany`)**:
   - Upgrades every `["viewany"]` argument descriptor in the model to
     `["viewany", ["node", Tooltip]]` so Go callers can pass either a child `View`
     handle (wire tag `0`) or a described `Tooltip` fragment (wire tag `1`, decoded
     through `crate::views_after::tooltip_view`).
2. **Non-`Clone` closure snapshots (`snapshot`, `snap_field_put`, `snapshot_statuses`)**:
   - When an upstream closure hands a non-`Clone` borrowed context struct whose public
     inherent methods are exclusively zero-arg `&self` getters (for example
     `CalendarItemState`, `TreeEntryState`, `ResizeHandleContext`), synthesizes an
     output-only wire record (`snap=path`) whose `Wire::put` invokes each getter at
     closure-call time and marks those getters `bridged`.
3. **Additional closure-to-data patterns (`closure`, `bounded`, `rust_dec`)**:
   - `rowvals`: per-row scalar/value lookup closures sent as a `Vec<V>`.
   - `valset`: membership predicates (`Fn(&T) -> bool`) sent as an allow-list `Vec<T>`.
   - `qrules`: questionnaire validation rule closures (`QuestionnaireValidationContext`).
   - Optional-element row closures (`Option<impl IntoElement>`) routed through item sources.
   - `kitfn`: parameters accepting either an app-supplied item closure or one of a closed
     set of built-in upstream renderer functions (wire tag `0` = `RowSource`, tag `1` =
     `u16` built-in index).
4. **Late post-processing (`post`, `post_late`)**:
   - Attaches tooltip nodes to `viewany`, marks snapshot getter statuses, and validates
     any comp6-specific override usage after `gk_values.post()` completes.
"""
import json

import gk_editor
import rdj

TOOLTIP = "gpui_kit::component::tooltip::Tooltip"


def _g():
    import gk_model
    return gk_model


def tooltip_key(m):
    """Return the model node key for `Tooltip` if it is registered as a non-element value node."""
    n = m.nodes.get(TOOLTIP)
    return n["key"] if n is not None and not n["element"] else None


def viewany_tooltips(m):
    """Attach the `Tooltip` value-node descriptor to every `viewany` argument in the model.

    With the tooltip node attached, `rust_viewany` emits a 1-byte tag prefix so Go callers
    can pass either a child view handle (tag 0) or a described `Tooltip` fragment (tag 1).
    """
    key = tooltip_key(m)
    if key is None:
        return
    g = _g()
    for it in m.all_items():
        for a in it.get("args", []) or []:
            for x in g.walk_desc(a["desc"]):
                if x[0] == "viewany" and len(x) == 1:
                    x.append(["node", key])


def rust_viewany(rs, d):
    """Emit Rust decoder for `viewany`: either a child view handle or a tagged Tooltip fragment."""
    view = ("{ let h = r.u64(); let v = if h == 0 { None } else { dx.child_view(h) }; "
            "let f: std::rc::Rc<dyn Fn(&mut gpui_kit::Window, &mut gpui_kit::App) -> gpui_kit::AnyView> = "
            "std::rc::Rc::new(move |_: &mut gpui_kit::Window, cx: &mut gpui_kit::App| -> gpui_kit::AnyView { match &v { "
            "Some(v) => v.clone(), None => gpui_kit::AppContext::new(cx, |_| gpui_kit::EmptyView).into() } }); f }")
    if len(d) < 2:
        return view
    fn = rs.m.nodes[d[1][1]]["fn"]
    tip = ("{ let fr = dx.frag(r.u32()); "
           "let f: std::rc::Rc<dyn Fn(&mut gpui_kit::Window, &mut gpui_kit::App) -> gpui_kit::AnyView> = "
           f"std::rc::Rc::new(move |window: &mut gpui_kit::Window, cx: &mut gpui_kit::App| crate::views_after::tooltip_view(&fr, window, cx, build_{fn})); f }}")
    return ("{ let f = if r.u8() == 1 { %s } else { %s }; "
            "move |window: &mut gpui_kit::Window, cx: &mut gpui_kit::App| -> gpui_kit::AnyView { f(window, cx) } }" % (tip, view))


def _reads(m, hc, hid):
    """Collect all public zero-arg `&self` getter methods on a struct, or None if any inherent method differs."""
    c = m.api.crates[hc]
    it = c.item(hid)
    out = []
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
            ins = sig["inputs"]
            if len(ins) != 1 or ins[0][0] != "self" or rdj.ty(ins[0][1]) != "&Self" or not sig.get("output"):
                return None
            if fi["inner"]["function"]["generics"]["params"]:
                return None
            out.append((fi["name"], sig["output"]))
    return out


def snapshot(m, ctx, t, out):
    """Synthesize an output-only snapshot record for a non-Clone struct handed to a closure.

    Some kit closures hand borrowed context structs that cannot be cloned into a handle
    (for example `CalendarItemState` or `TreeEntryState`) and expose only pure `&self`
    getters. When inside closure argument analysis (`m._closure > 0`), this turns those
    getters into fields of an outbound wire record (`snap=path`) serialized at call time.
    """
    g = _g()
    if not out or not m._closure or "resolved_path" not in t:
        return None
    rp = t["resolved_path"]
    if g.path_args(rp):
        return None
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:
        return None
    if it is None or hc not in m.api.crates or "struct" not in it.get("inner", {}):
        return None
    f = m.info(hc, hid)
    if f["clone"] or f["record"] or f["state"] or f["generic"] or not m.is_obj(hc, hid):
        return None
    reads = _reads(m, hc, hid)
    if not reads:
        return None
    rt = m.rtype(ctx, t)
    if rt in m.records:
        m.records[rt]["out"] = True
        return ["rec", rt]
    fields = []
    for name, ot in reads:
        try:
            d = m.desc(g.Ctx(hc), ot, out=True)
        except g.Unsup:
            return None
        if any(x[0] in ("obj", "node", "handle", "entity", "shim", "lent", "rows") for x in g.walk_desc(d)):
            return None
        fields.append({"name": name, "desc": d})
    path = m.api.pubpath(hc, hid)
    m.records[rt] = {"rust": rt, "path": rt, "name": f["name"], "fields": fields, "tuple": False, "complete": False,
                     "in": False, "out": True, "crate": hc, "args": [], "snap": path}
    return ["rec", rt]


def snap_field_put(rc, f):
    """Serialize a snapshot record field by calling `self.<getter>()` rather than field access."""
    if not rc.get("snap"):
        return None
    return f"genapi::Wire::put(&self.{f['name']}(), out);"


def snapshot_statuses(m):
    """Mark getter items of snapshot structs as `bridged` since they are serialized in the snapshot record."""
    paths = {rc["snap"]: rt for rt, rc in m.records.items() if rc.get("snap") and rc.get("out")}
    for it in m.all_items():
        owner = it["id"].rsplit("::", 1)[0]
        if owner in paths and it["status"] in ("todo", "norule"):
            it.update(status="bridged", kind="shim", reason="",
                      detail=f"a field of the snapshot record {m.records[paths[owner]]['name']} the library copies out "
                             "of the object it hands a closure (PROTOCOLS.md 11.18)")


def _opt_element(m, ctx, output):
    """If `output` is `Option<AnyElement>` or `Option<impl IntoElement>`, return the inner type."""
    g = _g()
    if output is None or "resolved_path" not in output or g.last(output["resolved_path"]["path"]) != "Option":
        return None
    a = g.path_args(output["resolved_path"])
    if len(a) != 1:
        return None
    s = rdj.ty(a[0])
    if g.last(s) == "AnyElement" or ("impl_trait" in a[0] and "IntoElement" in s):
        return a[0]
    return None


# Key descriptor tags permitted in a `valset` membership predicate closure (`Fn(&T) -> bool`).
SET_KEYS = {"civil", "rec", "enum", "int", "str", "bool"}


def closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload):
    """Match optional-element row closures, datum `rowvals`, questionnaire rules, and `valset` predicates.

    - `ctxrows + ["opt"]`: element row closures returning `Option<impl IntoElement>`.
    - `rowvals`: pure per-datum closures returning a `Vec`/`[T; N]` (for example plot labels).
    - `qrules`: questionnaire validation closures decoded into declarative rule lists.
    - `valset`: single-argument `-> bool` predicates encoded as an explicit match list.
    """
    import gk_lists
    g = _g()
    el = _opt_element(m, ctx, output)
    if el is not None and payload and "window" in params and "app" in params:
        d = gk_lists.ctx_rows(m, ctx, kind, inputs, el, wrap, params, ptypes, payload)
        if d is not None:
            return d + ["opt"]
    if params and all(p == "arg" for p in params):
        rv = rowvals(m, ctx, kind, inputs, output, wrap, ptypes)
        if rv is not None:
            return rv
    out_s = rdj.ty(output) if output else "()"
    if len(payload) == 1 and params == ["arg"] and out_s.replace(" ", "") == "Result<(),SharedString>" \
            and g.last(rdj.ty(payload[0]).lstrip("&")) == "QuestionnaireValidationContext":
        return ["qrules", wrap, ptypes]
    if out_s == "bool" and len(payload) == 1 and params == ["arg"]:
        try:
            d = m.desc(ctx, payload[0])
        except g.Unsup:
            d = None
        base = d[1] if d and d[0] == "ref" else d
        if base and base[0] in SET_KEYS and all(x[0] in SET_KEYS | {"opt", "list", "tuple", "leaf"}
                                                for x in g.walk_desc(base)):
            return ["valset", base, wrap, ptypes]
    return None


def rowvals(m, ctx, kind, inputs, output, wrap, ptypes):
    """Match a closure taking a `Datum` row and returning a list/array of plain values.

    Go pre-computes one value sequence per row and sends the outer slice on the tape;
    at paint time the closure indexes that slice by `Datum::row`.
    """
    g = _g()
    if output is None or not inputs:
        return None
    at = [i for i, t in enumerate(inputs) if m.datum_arg(ctx, t) is not None]
    if len(at) != 1:
        return None
    try:
        d = m.desc(ctx, output)
    except g.Unsup:
        return None
    if d[0] not in ("list", "array"):
        return None
    for x in g.walk_desc(d):
        if x[0] in ("handler", "fragment", "build", "child", "obj", "entity", "data", "field", "format"):
            return None
    return ["rowvals", d, wrap, ptypes, m.datum_arg(ctx, inputs[at[0]]), at[0]]


def bounded(m, ctx, bounds, out):
    """Fallback `impl Fn(...)` matcher for `rowvals` closures with unnameable extra parameter types."""
    g = _g()
    if out:
        return None
    for n, tr in g.bound_parts(bounds):
        if n not in ("Fn", "FnMut", "FnOnce"):
            continue
        p = (tr.get("args") or {}).get("parenthesized") or {"inputs": [], "output": None}
        if any("window" in rdj.ty(t).lower() or rdj.ty(t).endswith("App") for t in p["inputs"]):
            return None
        ptypes, named = [], True
        for t in p["inputs"]:
            try:
                ptypes.append(m.rtype(ctx, t).replace("std::ops::range::", "std::ops::"))
            except g.Unsup:
                named = False
                ptypes.append(rdj.ty(t))
        # If every parameter type is concrete, the regular closure path already handled it.
        if named:
            return None
        return rowvals(m, ctx, n, p["inputs"], p.get("output"), "impl", ptypes)
    return None


def rust_rowvals(rs, d):
    """Emit Rust decoder for `rowvals`: reads `Vec<RowOutput>` and indexes it via `crate::plot`."""
    _, od, wrap, ptypes, acc, at = d
    ps = []
    for i, t in enumerate(ptypes):
        name = "v" if i == at else f"_a{i}"
        ps.append(f"{name}: {t}" if t != "_" else name)
    row = "v" if not acc else f"v.{acc}"
    inner = rs.dec(["list", od, "vec"])
    get = "row_texts" if od == ["list", ["node", PLOT_TEXT], "vec"] else "row_value"
    return rs.wrap(wrap, "{ let rows: Vec<%s> = %s; move |%s| crate::plot::%s(&rows, %s) }"
                   % (_rust_t(rs, od), inner, ", ".join(ps), get, row))


PLOT_TEXT = "gpui_kit::base::plot::label::Text"


def _rust_t(rs, d):
    """Format the concrete Rust type annotation for a `rowvals` element descriptor."""
    if d[0] == "list":
        return f"Vec<{_rust_t(rs, d[1])}>"
    if d[0] == "array":
        return f"[{_rust_t(rs, d[1])}; {d[2]}]"
    if d[0] == "node":
        return rs.m.nodes[d[1]]["path"]
    if d[0] in ("rec", "denum", "enum", "into"):
        return d[1]
    if d[0] == "leaf" and d[1] == "color":
        return d[2] or "gpui_kit::Hsla"
    if d[0] == "str":
        return "gpui_kit::SharedString"
    if d[0] in ("f32", "f64", "bool"):
        return d[0]
    if d[0] == "int":
        return d[1]
    raise KeyError(f"rowvals: {d}")


def rust_valset(rs, d):
    """Emit Rust decoder for `valset`: reads a Vec of allowed keys and checks membership."""
    _, kd, wrap, ptypes = d
    if wrap == "box":
        return "crate::values::value_set(%s)" % rs.dec(["list", kd, "vec"])
    return rs.wrap(wrap, "{ let set: Vec<_> = %s; move |k: %s| set.iter().any(|x| x == k) }"
                   % (rs.dec(["list", kd, "vec"]), ptypes[0]))


def set_variants(m):
    """Enable data-enum tuple variants whose fields are boxed `valset` predicate closures (e.g. date matchers)."""
    g = _g()
    for rt, e in m.denums.items():
        if not e.get("in"):
            continue
        for v in e["variants"]:
            if v["ok"] or v["shape"] != "tuple":
                continue
            hit = _variant_item(m, e, v["name"])
            if hit is None:
                continue
            hc, ids = hit
            try:
                fl = m.fields(g.Ctx(hc), m.api.crates[hc], ids, False, names=False)
            except g.Unsup:
                continue
            if fl and len(fl) == len(ids) and all(f["desc"][0] == "valset" and f["desc"][2] == "box" for f in fl):
                v.update(ok=True, fields=fl)


def _variant_item(m, e, vname):
    """Look up the tuple field item IDs for a named variant of data enum `e` in rustdoc."""
    for iid, (path, it) in m.api.walks[e["crate"]].types.items():
        if "enum" not in it.get("inner", {}) or m.api.pubpath(e["crate"], iid) != e["path"]:
            continue
        c = m.api.crates[e["crate"]]
        for vid in it["inner"]["enum"]["variants"]:
            vi = c.item(vid)
            if vi and vi["name"] == vname and isinstance(vi["inner"]["variant"]["kind"], dict) \
                    and "tuple" in vi["inner"]["variant"]["kind"]:
                return e["crate"], vi["inner"]["variant"]["kind"]["tuple"]
    return None


def kit_fns(m, alias_id_crate):
    """Find all zero-argument kit functions that construct a given renderer type alias."""
    out = []
    for crate in ("gpui_component", "gpui_base"):
        w = m.api.walks[crate]
        for iid, (path, fi) in sorted(w.fns.items(), key=lambda kv: kv[1][0]):
            sig = fi["inner"].get("function", {}).get("sig")
            o = sig and sig.get("output")
            if not o or sig["inputs"] or "resolved_path" not in o:
                continue
            try:
                if m.api.home(crate, o["resolved_path"]["id"]) == alias_id_crate:
                    out.append(m.api.pubpath(crate, iid) or path)
            except Exception:
                continue
    return sorted(set(out))


def kitfn(m, hc, hid, it, d):
    """Wrap a `ctxrows` type alias into a `kitfn` union when upstream provides built-in factory functions."""
    fns = kit_fns(m, (hc, hid))
    if not fns or d[0] != "ctxrows":
        return d
    m.kitfns = getattr(m, "kitfns", {})
    path = m.api.pubpath(hc, hid) or it["name"]
    m.kitfns[path] = {"name": it["name"], "fns": fns}
    return ["kitfn", path, it["name"], fns, d]


def rust_kitfn(rs, d):
    """Emit Rust decoder for `kitfn`: tag 1 selects a built-in kit factory by index, tag 0 decodes a Go row source."""
    _, path, name, fns, inner = d
    arms = " ".join(f"{i} => {fn}()," for i, fn in enumerate(fns))
    return "{ if r.u8() == 1 { match r.u16() { %s _ => %s() } } else { %s } }" % (arms, fns[0], rs.dec(inner))


# Hand-wired library value descriptors for GPUI Background, questionnaire validators, and date matchers.
BACKGROUND = ["libval", "gpui_kit::Background", "Background_Value"]
VALIDATOR_READ = ["libval", "std::rc::Rc<dyn Fn(&gpui_kit::base::questionnaire::QuestionnaireValidationContext) -> "
                  "Result<(), gpui_kit::SharedString>>", "Questionnaire_Validator"]
MATCHER_PARTS = {"IntervalMatcher": ["libval", "gpui_kit::base::IntervalMatcher", "Interval_Matcher"],
                 "RangeMatcher": ["libval", "gpui_kit::base::RangeMatcher", "Range_Matcher"]}


def resolved(m, ctx, t, out):
    """Classify `Background`, `QuestionnaireValidator`, matcher structs, `Box<Node>`, and snapshot structs."""
    g = _g()
    rp = t["resolved_path"]
    name = g.last(rp["path"])
    if name == "Background" and out:
        try:
            hc, _, _ = m.api.item(ctx.crate, rp["id"])
        except Exception:
            hc = None
        if hc in g.GPUI_HOMES:
            return list(BACKGROUND)
    if name == "QuestionnaireValidator" and out:
        return list(VALIDATOR_READ)
    if name in MATCHER_PARTS:
        try:
            hc, _, _ = m.api.item(ctx.crate, rp["id"])
        except Exception:
            hc = None
        if hc == "gpui_base":
            return list(MATCHER_PARTS[name])
    if name == "Box" and not out:
        a = g.path_args(rp)
        if len(a) == 1 and "resolved_path" in a[0]:
            try:
                d = m.desc(ctx, a[0])
            except g.Unsup:
                return None
            if d[0] == "node":
                return ["boxed", d]
        return None
    return snapshot(m, ctx, t, out)


def rust_dec(rs, d):
    """Emit Rust decoder expression for `kitfn`, `valset`, `rowvals`, `boxed`, and `qrules` descriptors."""
    k = d[0]
    if k == "ctxrows" and len(d) > 5:
        return None
    if k == "kitfn":
        return rust_kitfn(rs, d)
    if k == "valset":
        return rust_valset(rs, d)
    if k == "rowvals":
        return rust_rowvals(rs, d)
    if k == "boxed":
        return f"Box::new({rs.dec(d[1])})"
    if k == "qrules":
        return rs.wrap(d[1], "crate::values::questionnaire_rules(r)")
    return None


# Descriptor tags from gk_comp6 that can be decoded in immediate calls without a view Dx.
CALL_SAFE = {"valset"}


# QuestionnaireValidationContext getter methods evaluated inside core/src/values.rs rule checks.
_Q = "gpui_kit::base::questionnaire::QuestionnaireValidationContext::"
_QWHY = ("called by the library's validator, which checks the rules the app gave the item "
         "(gk.Questionnaire_Rules, core/src/values.rs; PROTOCOLS.md 11.26)")
LIBRARY_CALLS = {_Q + "item": _QWHY + ": the message's {item}", _Q + "answer": _QWHY + ": the choices and the free answer",
                 _Q + "answers": _QWHY + ": same_as and differs_from"}


def post(m):
    """Run item post-processing, tooltip wiring, snapshot statuses, valset variants, and shim marks."""
    import gk_items
    gk_items.post(m)
    viewany_tooltips(m)
    snapshot_statuses(m)
    set_variants(m)
    for path, k in getattr(m, "kitfns", {}).items():
        for fn in k["fns"]:
            LIBRARY_CALLS.setdefault(fn, f"made by the library where the app names gk.{_g().ada(k['name'])}_Kit."
                                         f"{_g().ada(fn.rsplit('::', 1)[-1])} (PROTOCOLS.md 11.20)")
    for it in m.all_items():
        if it["id"] in LIBRARY_CALLS and it["status"] in ("todo", "norule"):
            it.update(status="bridged", kind="shim", reason="", detail=LIBRARY_CALLS[it["id"]])


def post_late(m):
    """Apply late skip rules that depend on synthesized readback items (such as `read:ScrollbarTheme::styles`)."""
    g = _g()
    for it in m.all_items():
        if it["id"] in SKIPS and it["status"] in ("todo", "norule"):
            m.set(it, g.Unsup(SKIPS[it["id"]][0], SKIPS[it["id"]][1]))


# Late skips for synthesized readback getters whose return type exposes no readable fields.
SKIPS = {
    "read:gpui_kit::base::ScrollbarTheme::styles": (
        "skip:builder-getter", "reads back the ScrollbarStyles the app wrote with with_styles: six private "
        "track and thumb styles with no public read, so a copy would show the app nothing; the app keeps the "
        "styles it built"),
}


_ = json


