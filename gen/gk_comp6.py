"""gk_comp6: the rules of the components, charts, dialogs, tables and layout families
(docs/PROTOCOLS.md 11.17 to 11.24 and 12.16). Hooked into the other families' modules at one line
each (gk_values.resolved, gk_values.closure, gk_values.post, gk_values' viewany emitters,
gk_values.rust_field_put, gk_values.odin_unions, gk_lists.bounded_out, gk_lists.alias, gk_lists'
emitters), never into the shared model or emitters directly.

- 11.17 Tooltip values in view closures: a closure that returns a view (`["viewany"]`, 13.18)
  takes the kit's own Tooltip, a value node built in the view build, as well as an app view.
- 11.18 Snapshots: an object upstream hands a closure that cannot be copied (not Clone) and
  whose public methods are reads that take nothing is copied out as the values of its reads, a
  record whose fields are the reads (`ResizeHandleContext`); the reads count bridged.
- 11.19 Optional element closures over a context: `-> Option<AnyElement>` over the values the kit
  hands it is 9.16's row source; `Some(row)` once a row was asked for.
- 11.20 Closure types the kit also makes: a type alias of a closure that a kit function with no
  arguments returns (`ResizeHandleRenderer`, `resize_handle_appearance()`) is the app's rows or
  that function, named by an enum.
- 11.21 Sets of values: `Fn(&X) -> bool` with no window and app over a plain value (a date) is the
  set of values for which it holds (`Matcher::Custom`).
- 11.22 Boxed values: `Box<T>` of a value node or element in a variant (`Text::TextView`).
- 11.23 Background read back: gpui's Background as the library value `gk.Background_Value`.
- 12.16 Lists per row: a closure over a row of a data table that returns a list or a fixed array of
  values (`SankeyChart::labels`, `Bar::label`, `BarChart::fill_gradient`) is answered from the
  list the app gives for each row (row i's at index i).
"""
import json

import gk_editor
import rdj

TOOLTIP = "gpui_kit::component::tooltip::Tooltip"
VIEWANY_UNION = "View_Or_Tooltip"


def _g():
    import gk_model  # noqa: PLC0415 (gk_model imports the modules that import this one)
    return gk_model


# ---------------------------------------------------------------- 11.17 tooltips in view closures

def tooltip_key(m):
    n = m.nodes.get(TOOLTIP)
    return n["key"] if n is not None and not n["element"] else None


def viewany_tooltips(m):
    """Every `["viewany"]` argument also takes the kit's Tooltip value: `["viewany", key]`."""
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
    """A view closure over the app's view (0: an empty view) or the kit's Tooltip, whose node is kept
    with the view's tape and built each time upstream asks (crate::views_after::tooltip_view)."""
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


def odin_viewany_type(od, d):
    return VIEWANY_UNION if len(d) > 1 else "View"


def odin_viewany_put(od, d, v):
    if len(d) < 2:
        return [f"put_view_arg(o, {v})"]
    t = od.m.nodes[d[1][1]]["odin"]
    return ["{", f"\tswitch x in {v} {{", "\tcase View:", "\t\tput_u8(o, 0)", "\t\tput_view_arg(o, x)",
            f"\tcase {t}:", "\t\tput_u8(o, 1)", "\t\tput_u32(o, take_value(x.nd))",
            "\tcase:", "\t\tput_u8(o, 0)", "\t\tput_u64(o, 0)", "\t}", "}"]


# ---------------------------------------------------------------- 11.18 snapshots

def _reads(m, hc, hid):
    """(name, output type) of every public inherent method of a type, or None when one is not a
    `&self` read that takes nothing and returns a value."""
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
    """`["rec", rt]` of a snapshot record for a library object handed to a closure that cannot be
    copied (PROTOCOLS.md 11.18), or None. The record is out only; its Wire impl calls the reads."""
    g = _g()
    if not out or not m._closure or "resolved_path" not in t:
        return None
    rp = t["resolved_path"]
    if g.path_args(rp):
        return None
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:  # noqa: BLE001 (an id outside the extract)
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
    """A snapshot record's field is its read, called on the object (11.18)."""
    if not rc.get("snap"):
        return None
    return f"genapi::Wire::put(&self.{f['name']}(), out);"


def snapshot_statuses(m):
    paths = {rc["snap"]: rt for rt, rc in m.records.items() if rc.get("snap") and rc.get("out")}
    for it in m.all_items():
        owner = it["id"].rsplit("::", 1)[0]
        if owner in paths and it["status"] in ("todo", "norule"):
            it.update(status="bridged", kind="shim", reason="",
                      detail=f"a field of the snapshot record {m.records[paths[owner]]['name']} the library copies out "
                             "of the object it hands a closure (PROTOCOLS.md 11.18)")


# ---------------------------------------------------------------- closures (11.19, 11.21, 12.16)

def _opt_element(m, ctx, output):
    """The element type inside `Option<..>` of a closure's output, or None."""
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


SET_KEYS = {"chrono", "rec", "enum", "int", "str", "bool"}


def closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload):
    """gk_values.closure's hook for this module, None for other closures."""
    import gk_lists  # noqa: PLC0415
    g = _g()
    el = _opt_element(m, ctx, output)
    if el is not None and payload and "window" in params and "app" in params:
        d = gk_lists.ctx_rows(m, ctx, kind, inputs, el, wrap, params, ptypes, payload)
        if d is not None:
            return d + ["opt"]  # 11.19
    if params and all(p == "arg" for p in params):
        rv = rowvals(m, ctx, kind, inputs, output, wrap, ptypes)
        if rv is not None:
            return rv  # a list per row (12.16)
    out_s = rdj.ty(output) if output else "()"
    if len(payload) == 1 and params == ["arg"] and out_s.replace(" ", "") == "Result<(),SharedString>" \
            and g.last(rdj.ty(payload[0]).lstrip("&")) == "QuestionnaireValidationContext":
        return ["qrules", wrap, ptypes]  # a questionnaire item's validator as rules (11.26)
    if out_s == "bool" and len(payload) == 1 and params == ["arg"]:
        try:
            d = m.desc(ctx, payload[0])
        except g.Unsup:
            d = None
        base = d[1] if d and d[0] == "ref" else d
        if base and base[0] in SET_KEYS and all(x[0] in SET_KEYS | {"opt", "list", "tuple", "leaf"}
                                                for x in g.walk_desc(base)):
            return ["valset", base, wrap, ptypes]  # 11.21
    return None


def rowvals(m, ctx, kind, inputs, output, wrap, ptypes):
    """12.16: a closure over a row of a data table that returns a list or a fixed array of values the
    binding builds (not numbers, text or a colour, which 12.3's field answers), with no window or app:
    `["rowvals", out desc, wrap, ptypes, datum accessor, index of the row's parameter]`."""
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
    """gk_lists.bounded_out's hook: `impl Fn(&T, ..) -> V` over a row (12.16) when its other
    parameters name types no closure rule can (`&dyn Fn(f32) -> f32`), read before the model's
    closure rule names every parameter."""
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
                ptypes.append(rdj.ty(t))  # `&dyn Fn(f32) -> f32`: std and primitive types only
        if named:
            return None  # the model's own rule names them all
        return rowvals(m, ctx, n, p["inputs"], p.get("output"), "impl", ptypes)
    return None


def rust_rowvals(rs, d):
    _, od, wrap, ptypes, acc, at = d
    ps = []
    for i, t in enumerate(ptypes):
        name = "v" if i == at else f"_a{i}"
        ps.append(f"{name}: {t}" if t != "_" else name)
    row = "v" if not acc else f"v.{acc}"
    inner = rs.dec(["list", od, "vec"])
    # A plot label (`Text`) is not Clone: the library copies it field by field (crate::plot::row_texts).
    get = "row_texts" if od == ["list", ["node", PLOT_TEXT], "vec"] else "row_value"
    return rs.wrap(wrap, "{ let rows: Vec<%s> = %s; move |%s| crate::plot::%s(&rows, %s) }"
                   % (_rust_t(rs, od), inner, ", ".join(ps), get, row))


PLOT_TEXT = "gpui_kit::base::plot::label::Text"


def _rust_t(rs, d):
    """The Rust type of a decoded value of the descriptors rowvals carries."""
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
    _, kd, wrap, ptypes = d
    if wrap == "box":
        # Kept with its bytes, so that the value it is in reads back as the set (core/src/values.rs).
        return "crate::values::value_set(%s)" % rs.dec(["list", kd, "vec"])
    return rs.wrap(wrap, "{ let set: Vec<_> = %s; move |k: %s| set.iter().any(|x| x == k) }"
                   % (rs.dec(["list", kd, "vec"]), ptypes[0]))


def set_variants(m):
    """A variant of an enum with data whose field is a set of values (11.21: `Matcher::Custom(Box<dyn
    Fn(&NaiveDate) -> bool + Send + Sync>)`) is buildable, and reads back as the set the app gave
    (the library's boxed closure keeps it): when the enum was first met read back, the variant was
    left out for both directions."""
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


# ---------------------------------------------------------------- 11.20 closure types the kit makes

def kit_fns(m, alias_id_crate):
    """Kit free functions with no arguments that return the alias (crate, id)."""
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
            except Exception:  # noqa: BLE001
                continue
    return sorted(set(out))


def kitfn(m, hc, hid, it, d):
    """`["kitfn", alias path, odin name, [kit fns], inner]` for an alias whose value the kit also
    makes (11.20), else d."""
    fns = kit_fns(m, (hc, hid))
    if not fns or d[0] != "ctxrows":
        return d
    m.kitfns = getattr(m, "kitfns", {})
    path = m.api.pubpath(hc, hid) or it["name"]
    m.kitfns[path] = {"name": it["name"], "fns": fns}
    return ["kitfn", path, it["name"], fns, d]


def _kit_odin(name):
    g = _g()
    return g.ada(name)


def rust_kitfn(rs, d):
    _, path, name, fns, inner = d
    arms = " ".join(f"{i} => {fn}()," for i, fn in enumerate(fns))
    return "{ if r.u8() == 1 { match r.u16() { %s _ => %s() } } else { %s } }" % (arms, fns[0], rs.dec(inner))


def odin_kitfn_type(od, d):
    return _kit_odin(d[2])


def odin_kitfn_put(od, d, v):
    k = _kit_odin(d[2]) + "_Kit"
    return ["{", f"\tswitch x in {v} {{", "\tcase Row_Source:", "\t\tput_u8(o, 0)", "\t\tput_u64(o, x.h)",
            f"\tcase {k}:", "\t\tput_u8(o, 1)", "\t\tput_u16(o, u16(x))",
            "\tcase:", f'\t\tstop({{}}, "a zero gk.{_kit_odin(d[2])} was given to a builder: it was never made (make it with its constructor before giving it)")', "\t}", "}"]


def odin_decls(od):
    """Generated Odin types of this module (gen_values.odin, through gk_values.odin_unions)."""
    m = od.m
    out = []
    key = tooltip_key(m)
    if key is not None and "odin" in m.nodes[key]:
        out += ["// A view closure's argument: the app's view or the kit's Tooltip value (PROTOCOLS.md 11.17).",
                f"{VIEWANY_UNION} :: union {{View, {m.nodes[key]['odin']}}}", ""]
    for path, k in sorted(getattr(m, "kitfns", {}).items()):
        n = _kit_odin(k["name"])
        out += [f"// {n} is the app's rows (9.16) or a renderer the kit makes (PROTOCOLS.md 11.20).",
                f"{n}_Kit :: enum u16 {{" + ", ".join(_kit_odin(f.rsplit('::', 1)[-1]) for f in k["fns"]) + "}",
                f"{n} :: union {{Row_Source, {n}_Kit}}", ""]
    return out


# ---------------------------------------------------------------- 11.22 boxed values, 11.23 Background

BACKGROUND = ["libval", "gpui_kit::Background", "Background_Value"]
# A questionnaire item's validator read back (11.26): present or not (core/src/values.rs).
VALIDATOR_READ = ["libval", "std::rc::Rc<dyn Fn(&gpui_kit::base::questionnaire::QuestionnaireValidationContext) -> "
                  "Result<(), gpui_kit::SharedString>>", "Questionnaire_Validator"]
# A calendar matcher's parts (11.21): private fields, made by the enum's own constructors
# (`Matcher::interval(before, after)`, `range(from, to)`) and read back through `Matcher::matched`
# (core/src/values.rs); Odin records in rt_values.odin.
MATCHER_PARTS = {"IntervalMatcher": ["libval", "gpui_kit::base::IntervalMatcher", "Interval_Matcher"],
                 "RangeMatcher": ["libval", "gpui_kit::base::RangeMatcher", "Range_Matcher"]}


def resolved(m, ctx, t, out):
    """gk_values.resolved's hook: snapshots, Box<T> of a value or element node, Background read
    back. None for any other type."""
    g = _g()
    rp = t["resolved_path"]
    name = g.last(rp["path"])
    if name == "Background" and out:
        try:
            hc, _, _ = m.api.item(ctx.crate, rp["id"])
        except Exception:  # noqa: BLE001
            hc = None
        if hc in g.GPUI_HOMES:
            return list(BACKGROUND)  # 11.23
    if name == "QuestionnaireValidator" and out:
        return list(VALIDATOR_READ)  # read back: whether the item has a validator (11.26)
    if name in MATCHER_PARTS:
        try:
            hc, _, _ = m.api.item(ctx.crate, rp["id"])
        except Exception:  # noqa: BLE001
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
                return ["boxed", d]  # 11.22
        return None
    return snapshot(m, ctx, t, out)


# ---------------------------------------------------------------- emitters

def rust_dec(rs, d):
    k = d[0]
    if k == "ctxrows" and len(d) > 5:
        return None  # gk_lists.rust_ctxrows reads the flag
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


def odin_type(od, d, out=False):
    k = d[0]
    if k == "kitfn":
        return odin_kitfn_type(od, d)
    if k == "valset":
        return "[]" + od.otype(d[1], out)
    if k == "rowvals":
        return "[]" + od.otype(d[1], out)
    if k == "boxed":
        return od.otype(d[1], out)
    if k == "qrules":
        return "Questionnaire_Rules"
    return None


def odin_put(od, d, v):
    k = d[0]
    if k == "kitfn":
        return odin_kitfn_put(od, d, v)
    if k == "valset":
        return od.put(["list", d[1], "vec"], v)
    if k == "rowvals":
        return od.put(["list", d[1], "vec"], v)
    if k == "boxed":
        return od.put(d[1], v)
    if k == "qrules":
        return [f"{{ qv := {v}; put_value(o, &qv, type_info_of(Questionnaire_Rules)) }}"]
    return None


CALL_SAFE = {"valset"}


# ---------------------------------------------------------------- statuses

# Upstream functions the library's own code calls for the app (counted bridged): id -> where.
_Q = "gpui_kit::base::questionnaire::QuestionnaireValidationContext::"
_QWHY = ("called by the library's validator, which checks the rules the app gave the item "
         "(gk.Questionnaire_Rules, core/src/values.rs; PROTOCOLS.md 11.26)")
LIBRARY_CALLS = {_Q + "item": _QWHY + ": the message's {item}", _Q + "answer": _QWHY + ": the choices and the free answer",
                 _Q + "answers": _QWHY + ": same_as and differs_from"}


def post(m):
    """Statuses decided once every rule has run (gk_values.post)."""
    import gk_items  # noqa: PLC0415
    gk_items.post(m)  # item procedures (PROTOCOLS.md 9.41)
    viewany_tooltips(m)
    snapshot_statuses(m)
    set_variants(m)
    for path, k in getattr(m, "kitfns", {}).items():
        for fn in k["fns"]:
            LIBRARY_CALLS.setdefault(fn, f"made by the library where the app names gk.{_kit_odin(k['name'])}_Kit."
                                         f"{_kit_odin(fn.rsplit('::', 1)[-1])} (PROTOCOLS.md 11.20)")
    for it in m.all_items():
        if it["id"] in LIBRARY_CALLS and it["status"] in ("todo", "norule"):
            it.update(status="bridged", kind="shim", reason="", detail=LIBRARY_CALLS[it["id"]])



def post_late(m):
    """Statuses decided at the end of gk_values.post, after the reads of read-back values exist."""
    g = _g()
    for it in m.all_items():
        if it["id"] in SKIPS and it["status"] in ("todo", "norule"):
            m.set(it, g.Unsup(SKIPS[it["id"]][0], SKIPS[it["id"]][1]))


# Skipped with their reason (DESIGN.md section 13's fixed list).
SKIPS = {
    "read:gpui_kit::base::ScrollbarTheme::styles": (
        "skip:builder-getter", "reads back the ScrollbarStyles the app wrote with with_styles: six private "
        "track and thumb styles with no public read, so a copy would show the app nothing; the app keeps the "
        "styles it built"),
}


_ = json


# ---------------------------------------------------------------- 11.27 builders of a chain with the call's context

# This family's builders whose argument the chain decoder cannot decode (a colour, a value node:
# ColorPickerState::default_value, GridLayoutState::default_areas, TreeState::items, ScaleOrdinal's
# range and unknown) take the editor family's rule of 9.31 (gk_editor.chain_cx and chain_body), which
# binds every family's builders (PROTOCOLS.md 11.27).
