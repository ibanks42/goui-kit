"""Item procedures (docs/PROTOCOLS.md 9.41): the element closures upstream calls while it draws one
item of a component (a calendar's day, a time field's segment, a tree's entry, a nav stack's page,
a combobox's trigger, a command palette's header, a popover's or hover card's content, a
notification's content and action button, a tooltip's frame), which 9.16's element closures over a
context could not serve.

The app writes one procedure per item kind, whose parameters are the values upstream hands the
closure, typed:

    day :: proc(a: ^App, cx: ^gk.Cx, item: gk.Calendar_Item, st: gk.Calendar_Item_State) -> gk.Element {
        return item->rounded_full()->bg(st.is_active ? gk.theme(.Primary) : gk.theme(.Background))
    }
    gk.base_calendar(a.cal)->item(gk.items(cx, a, day))

`gk.items` (a procedure group generated here, one member per kind of item procedure) makes, or
finds again, the row source of that (procedure, state) pair; it may be called in a view procedure.
Each call of the closure in a frame is a row of it (9.16), described between frames by the
procedure. The values arrive as:

- `["val", d]`: what 9.16 already reads (plain values; a library object as a temporary handle);
- `["reads", rt]`: a record of the value's reads (`gk.Calendar_Item_State{kind, is_active, ..}`,
  `gk.Popover_State_Reads{is_open}`), for a value 9.16 could not read: a Clone value whose
  methods are reads (`fn x(self)` or `fn x(&self)`) besides its constructors and builders, an
  entity's state handed by reference, a generic context (`ComboboxTriggerContext<'_, D>` with the
  closure's D). Reads that take arguments, return a library object, an element or a view are left
  out; a state with no read left is not passed at all;
- `["handed", node, rt]`: an element upstream built and hands in to be decorated (`CalendarItem`,
  `TimeFieldSegment`, `NavPage`): the procedure gets it as a node of the row's tape (constructor
  `handed`, synthetic and private, decoded by taking upstream's value: core/src/items.rs) and returns
  it styled, with children, or placed inside other elements; `rt` is the record of its own reads
  (`gk.Nav_Page_Reads{index, phase, operation, progress}`), passed after it, or None;
- `["view"]`: a view upstream hands in (a tooltip's content): `gk.Kit_View`, a node to place.

The closure returns an element (`"el"`) or one element type (`["node", key]`: `Notification::
action -> Button`, the base HoverCard's `-> Stateful<Div>`), which the procedure returns typed.

`FnOnce(X) -> E` without a window, X an element node (`DialogClose::trigger(FnOnce(Button) -> E)`),
is the build closure of DESIGN.md 12 with E taken as X.
"""
import json

import rdj

N_KIT_VIEW = 41  # core/wire/ops.txt: node KIT_VIEW (a view handed to an item procedure)
_PLAIN_BAD = ("obj", "node", "handle", "entity", "shim", "lent", "rows", "build", "fragment", "child", "handler",
              "view", "dock", "ctxrows", "items")


def _g():
    import gk_model  # noqa: PLC0415
    return gk_model


# ---------------------------------------------------------------- the model's hook


def build_over_handed(m, ctx, kind, inputs, output, wrap, params, ptypes, payload, static):
    """`FnOnce(X) -> E` with no window, X an element node: a build closure returning X."""
    g = _g()
    if kind != "FnOnce" or wrap != "impl" or params != ["arg"] or len(payload) != 1 or static or output is None:
        return None
    if "borrowed_ref" in payload[0]:
        return None
    try:
        d = m.desc(ctx, payload[0])
    except g.Unsup:
        return None
    if d[0] != "node" or not m.nodes[d[1]]["element"]:
        return None
    return ["build", d[1], params, wrap, ptypes]


def closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload, is_el):
    """An item closure (module doc), or None. Called by gk_model.closure_desc where 9.16's rule
    refused a closure with values."""
    g = _g()
    if not payload or len(payload) > 4 or "window" not in params:
        return None
    if not ("app" in params or "context" in params):
        return None
    if is_el:
        out = "el"
    else:
        if output is None:
            return None
        try:
            d = m.desc(ctx, output)
        except g.Unsup:
            return None
        if d[0] != "node" or not m.nodes[d[1]]["element"] or not _fallback(m, d[1]):
            return None
        out = ["node", d[1]]
    vals = []
    for t in payload:
        v = value(m, ctx, t, payload)
        if v is None:
            return None
        vals.append(v)
    for v in vals:
        if v[0] == "handed":
            m.items_handed.add(v[1])
    return ["items", wrap, ptypes, params, vals, out]


def value(m, ctx, t, payload):
    g = _g()
    by_ref = "borrowed_ref" in t
    inner = t["borrowed_ref"]["type"] if by_ref else t
    if "generic" in inner:  # `&mut Self` of a closure over the value itself (Notification::content)
        try:
            cr, inner = m.resolve_arg(ctx, inner)
        except g.Unsup:
            return None
        ctx = ctx.at(cr)
    name = g.last(rdj.ty(inner).split("<")[0])
    if name == "AnyView" and not by_ref:
        return ["view"]
    if not by_ref:
        try:
            d = m.desc(ctx, t)
        except g.Unsup:
            d = None
        if d and d[0] == "node" and m.nodes[d[1]]["element"]:
            others = {rdj.ty(x).lstrip("&").replace("mut ", "") for x in payload if x is not t}
            return ["handed", d[1], reads_record(m, ctx, inner, "node", skip_types=others)]
    rec = reads_record(m, ctx, inner, "value")
    if rec is not None:
        return ["reads", rec] if rec else ["none"]
    try:
        d = m.desc(ctx, t, out=True)
        import gk_lists  # noqa: PLC0415
        ok = True
        for x in g.walk_desc(d):
            if x[0] == "obj":
                ok = ok and bool(x[3])
            elif x[0] not in gk_lists.ASK_KEY or (x[0] == "leaf" and x[1] not in ("px", "range", "color")):
                ok = False
        if ok:
            return ["val", d]
    except g.Unsup:
        pass
    rec = reads_record(m, ctx, inner, "state" if by_ref else "none")
    if rec is None:
        return None
    return ["reads", rec] if rec else ["none"]


def _methods(m, hc, hid):
    """(name, inputs, output, generics) of every public inherent method of a struct."""
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
            out.append((fi["name"], sig["inputs"], sig.get("output"), fi["inner"]["function"]["generics"]["params"]))
    return out


def reads_record(m, ctx, t, mode, skip_types=()):
    """The key of a record of a type's reads (module doc), "" when it has none, or None when the
    type is not one this rule reads. mode: "value" (a Clone value: every method a read, a
    constructor or a builder), "state" (kit state handed by reference: its reads, other methods
    ignored), "node" (an element handed in: its reads), "none" (a generic context: every method a
    read)."""
    g = _g()
    if "resolved_path" not in t:
        return None
    rp = t["resolved_path"]
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:  # noqa: BLE001 (an id outside the extract)
        return None
    if it is None or hc not in m.api.crates or "struct" not in it.get("inner", {}):
        return None
    f = m.info(hc, hid)
    if mode == "value" and (not f["clone"] or f["state"] or f["record"] or f["generic"]):
        return None
    if mode == "state" and not f["state"]:
        mode = "none"
    if mode == "none" and not f["generic"] and not f["state"]:
        return None
    try:
        sctx = m.sub_ctx(hc, f, ctx, g.path_args(rp)) if f["generic"] else g.Ctx(hc)
    except g.Unsup:
        return None
    reads = []
    for name, ins, outp, gens in _methods(m, hc, hid):
        recv = rdj.ty(ins[0][1]) if ins and ins[0][0] == "self" else None
        outs = rdj.ty(outp) if outp else "()"
        is_read = recv in ("&Self", "Self") and len(ins) == 1 and outp is not None and outs not in ("Self", f["name"]) \
            and not any("type" in p["kind"] for p in gens)
        if is_read:
            reads.append((name, outp, recv == "Self"))
            continue
        if mode == "value" and not (recv in (None, "Self") and outs in ("Self", f["name"])):
            return None  # a method that is neither a read, a constructor nor a builder
        if mode == "none" and not f["state"]:
            return None
    fields, left = [], {}
    for name, outp, by_value in reads:
        if rdj.ty(outp).lstrip("&").replace("mut ", "") in skip_types:
            continue
        was_out = {k: e.get("out") for k, e in m.denums.items()}
        try:
            sv = m._closure
            m._closure = 0
            try:
                d = m.desc(sctx, outp, out=True)
            finally:
                m._closure = sv
        except g.Unsup:
            continue
        flipped = [k for k, e in m.denums.items() if e.get("out") and not was_out.get(k)]
        if flipped:
            # A data enum read back changes its Odin type (gk.Size's Size_Kind becomes unit structs): the
            # read is left out rather than every app's `with_size(.Small)` changed.
            for k in flipped:
                m.denums[k]["out"] = was_out.get(k, False)
            left[name] = f"{flipped[0].rsplit('::', 1)[-1]}, a data enum the app only writes"
            continue
        if any(x[0] in _PLAIN_BAD for x in g.walk_desc(d)):
            continue
        fields.append({"name": name, "desc": d, "by_value": by_value})
    if mode == "value" and not fields:
        return None
    if not fields:
        return ""
    path = m.api.pubpath(hc, hid)
    if path is None:
        return None
    lifetimes = [p for p in it["inner"]["struct"]["generics"]["params"] if "lifetime" in p["kind"]]
    targs = [m.rtype(sctx.at(sctx.subst[gn][0]), sctx.subst[gn][1]) if gn in sctx.subst else None for gn in f["generic"]]
    if None in targs:
        return None
    gargs = (["'_"] if lifetimes else []) + targs
    # The record's Wire impl is on a borrow of the value (crate::items::Reads), never on the type:
    # a library object the app holds is written as a handle by its own Wire path (13.4).
    rt = "crate::items::Reads<'_, " + path + (f"<{', '.join(gargs)}>" if gargs else "") + ">"
    named = f["name"] + ("Reads" if mode in ("state", "node") or f["state"] else "")
    if rt in m.records:
        m.records[rt]["out"] = True
        return rt
    m.records[rt] = {"rust": rt, "path": path, "name": named, "fields": fields, "tuple": False, "complete": False,
                     "in": False, "out": True, "crate": hc, "args": [], "snap": path, "item_reads": True, "left": left}
    return rt


def field_put(rc, f):
    """A field of a record of reads: its read, called on the value (gk_values.rust_field_put)."""
    if not rc.get("item_reads"):
        return None
    if f.get("by_value"):
        return f"genapi::Wire::put(&self.0.clone().{f['name']}(), out);"
    return f"genapi::Wire::put(&self.0.{f['name']}(), out);"


# ---------------------------------------------------------------- statuses


def post(m):
    """Statuses once every rule has run: reads read into a record are bridged; a value's own
    makers are value helpers; the handed nodes get their private constructor."""
    from gk_model import ada  # noqa: PLC0415
    for rc in m.records.values():
        st = m.states.get(rc["path"]) if rc.get("item_reads") else None
        if st and not rc["name"].endswith("Reads") and (not st.get("obj") or st.get("reachable")):
            rc["name"] += "Reads"  # the type's own name is the object's the app holds (gk.Base_Tree_Entry)
    recs = {rc["path"]: (rt, rc) for rt, rc in m.records.items() if rc.get("item_reads") and rc.get("out")}
    for it in m.all_items():
        owner = it["id"].rsplit("::", 1)[0]
        if owner not in recs:
            continue
        rt, rc = recs[owner]
        names = {f["name"] for f in rc["fields"]}
        if it["name"] in names and it["status"] not in ("bound", "bridged"):
            it.update(status="bridged", kind="shim", reason="",
                      detail=f"a field of the record {ada(rc['name'])} the library copies out of the value it hands an "
                             "item procedure (PROTOCOLS.md 9.41)")
        elif it["name"] in rc.get("left", {}) and it["status"] in ("todo", "norule"):
            it.update(status="skipped", reason="builder-getter",
                      detail=f"builder-getter: reads back what the app set on the component ({rc['left'][it['name']]}: "
                             f"read back, its Odin type would change for every app); not a field of {ada(rc['name'])}")
    for key in sorted(m.items_handed):
        node = m.nodes[key]
        if any(i.get("handed") for i in node["ctors"]):
            continue
        node["ctors"].append({"row": None, "owner": node["path"], "name": "handed", "trait": None,
                              "sig": "(handed to an item procedure)", "id": node["path"] + "::(handed)", "kind": "ctor",
                              "status": "bound", "reason": "", "args": [{"name": "index", "desc": ["int", "u8"]}],
                              "crate": node["crate"], "node": key, "callee": "crate::items::handed::<%s>" % node["path"],
                              "synthetic": True, "handed": True})


def handed_out(d):
    """Library objects an item closure hands the app (gk_lists.handed_out's hook)."""
    g = _g()
    if d[0] != "items":
        return
    for v in d[4]:
        if v[0] == "val":
            for y in g.walk_desc(v[1]):
                if y[0] == "obj":
                    yield y[2]


# ---------------------------------------------------------------- Rust


def _fallback(m, key):
    """What the closure returns, for one frame, while a typed row has no tape: the type's plain
    value (a button with an id, a div with an id), or None when the binding knows none."""
    node = m.nodes[key]
    if node["path"].replace(" ", "") in ("gpui_kit::Stateful<gpui_kit::Div>",):
        return 'gpui_kit::InteractiveElement::id(gpui_kit::div(), "gk-item")'
    for it in node["ctors"]:
        if it["name"] == "new" and it["status"] in ("bound", "bridged") and len(it["args"]) == 1 \
                and it["args"][0]["desc"][0] in ("leaf", "into", "from", "str") and "id" in json.dumps(it["args"][0]["desc"]):
            return f'{it["callee"]}("gk-item")'
    return None


def rust_dec(rs, d):
    if d[0] != "items":
        return None
    m = rs.m
    wrap, ptypes, params, vals, out = d[1:6]
    ps, lets, puts, handed, n, fb = [], [], [], [], 0, None
    first_int = None
    for p, t in zip(params, ptypes):
        if p == "arg":
            v, k = vals[n], f"k{n}"
            ps.append(f"{k}: {t}")
            ref = "&*" + k if t.startswith("&mut ") else (k if t.startswith("&") else "&" + k)
            if v[0] == "val":
                import gk_lists  # noqa: PLC0415
                lets += gk_lists._ctx_print(rs, v[1], k)  # as 9.16 does
                base = v[1][1] if v[1][0] == "ref" else v[1]
                if base[0] == "obj":
                    lets.append(f"let c{n} = {k}.clone();")
                    puts.append(f"genapi::Wire::put(&c{n}, o);")
                else:
                    lets.append(f"let b{n} = {{ let mut b = Vec::new(); genapi::Wire::put(&{k}, &mut b); b }};")
                    puts.append(f"o.extend_from_slice(&b{n});")
            elif v[0] == "reads" or (v[0] == "handed" and v[2]):
                rd = f"&crate::items::Reads({ref})"
                lets.append(f"genapi::Wire::put({rd}, &mut p);")
                lets.append(f"let b{n} = {{ let mut b = Vec::new(); genapi::Wire::put({rd}, &mut b); b }};")
                puts.append(f"o.extend_from_slice(&b{n});")
            if v[0] == "val" and v[1][0] == "int" and n == 0:
                first_int = f"Some(({'*' if t.startswith('&') else ''}{k}) as u64)"
            if v[0] in ("handed", "view"):
                handed.append(k)
                if fb is None:
                    fb = t
            n += 1
        elif p == "window":
            ps.append(f"window: {t}")
        else:
            ps.append(f"cx: {t}")
    by = first_int or "None"
    body_pre = ("let mut p: Vec<u8> = Vec::new(); " if lets else "let p: Vec<u8> = Vec::new(); ") + " ".join(lets)
    write = "std::rc::Rc::new(move |o: &mut Vec<u8>| { %s })" % " ".join(puts)
    if out == "el":
        ret = "gpui_kit::AnyElement"
        decode = "|dx, n| Some(dx.child(n))"
        if fb:
            fallback = ("|h| match crate::items::first::<%s>(h) { Some(v) => gpui_kit::IntoElement::into_any_element(v), "
                        "None => gpui_kit::IntoElement::into_any_element(gpui_kit::div()) }" % fb)
        else:
            fallback = "|_| gpui_kit::IntoElement::into_any_element(gpui_kit::div())"
        if not handed:
            # No element handed in: the row's view itself, retained like any row (9.16).
            call = f"crate::source::ctx_row(src, {by}, p, {write}, window, cx)"
            body = f"{{ let src = r.u64(); move |{', '.join(ps)}| -> {ret} {{ {body_pre} {call} }} }}"
            return _wrapped(rs, wrap, ptypes, ret, body)
    else:
        node = m.nodes[out[1]]
        ret = node["path"]
        decode = f"|dx, n| build_{node['fn']}(dx, n)"
        fallback = f"|_| {_fallback(m, out[1])}"
    hv = "let h: Vec<Box<dyn std::any::Any>> = vec![%s];" % ", ".join(f"Box::new({k})" for k in handed)
    call = f"crate::items::row(src, {by}, p, {write}, h, window, cx, {decode}, {fallback})"
    body = f"{{ let src = r.u64(); move |{', '.join(ps)}| -> {ret} {{ {body_pre} {hv} {call} }} }}"
    return _wrapped(rs, wrap, ptypes, ret, body)


def _wrapped(rs, wrap, ptypes, ret, body):
    if wrap in ("rc", "box", "arc"):
        ptr = {"rc": "std::rc::Rc", "box": "Box", "arc": "std::sync::Arc"}[wrap]
        return f"{{ let f: {ptr}<dyn Fn({', '.join(ptypes)}) -> {ret}> = {rs.wrap(wrap, body)}; f }}"
    return rs.wrap(wrap, body)


# ---------------------------------------------------------------- Odin


def odin_type(od, d, out=False):
    if d[0] == "items":
        return "Row_Source"
    if d[0] == "view":
        return "Kit_View"
    return None


def as_items(d):
    """An item descriptor, or 9.16's element closure over a context seen as one (its values are
    all `val`s): both take `gk.items`, so every element closure over values has a typed procedure."""
    if d[0] == "items":
        return d
    if d[0] == "ctxrows":
        return ["items", d[1], d[2], d[3], [["val", x] for x in d[4]], "el"]
    return None


def odin_put(od, d, v):
    d = as_items(d)
    if d is None:
        return None
    n = signature(od, d)[0]
    return [f"put_u64(o, items_check({v}, {n}))"]


def _params(od, d):
    """(Odin parameter list of the procedure, values read with row_context as (field, type), calls
    that make the handed nodes) of an item descriptor."""
    m = od.m
    vals, out = d[4], d[5]
    ps, reads, makes, args = [], [], [], []
    hi = 0
    taken = {"s", "cx"}

    def pname(t, n):
        base = t.lower() if t[:1].isupper() and not t.startswith(("Maybe", "Temp")) else f"v{n}"
        nm = base
        while nm in taken:
            nm += "_"
        taken.add(nm)
        return nm
    for n, v in enumerate(vals):
        if v[0] == "val":
            t = od.otype(v[1], True)
            reads.append((f"v{n}", t))
            ps.append((pname(t, n), t))
            args.append(f"vals.v{n}")
        elif v[0] == "reads":
            t = m.records[v[1]]["odin"]
            reads.append((f"v{n}", t))
            ps.append((pname(t, n), t))
            args.append(f"vals.v{n}")
        elif v[0] == "handed":
            node = m.nodes[v[1]]
            ctor = next(i for i in node["ctors"] if i.get("handed"))
            t = node["odin"]
            ps.append((pname(t, n), t))
            makes.append(f"h{n} := {ctor['odin']}({hi})")
            args.append(f"h{n}")
            hi += 1
            if v[2]:
                rt = m.records[v[2]]["odin"]
                reads.append((f"v{n}", rt))
                ps.append((pname(rt, n), rt))
                args.append(f"vals.v{n}")
        elif v[0] == "view":
            ps.append((pname("Kit_View", n), "Kit_View"))
            makes.append(f"h{n} := kit_view({hi})")
            args.append(f"h{n}")
            hi += 1
    ret = "Element" if out == "el" else m.nodes[out[1]]["odin"]
    return ps, reads, makes, args, ret


def vt_note(od, it):
    """A comment after a builder's table entry that takes an item procedure's rows: the procedure
    to write, since `Row_Source` alone does not say."""
    for a in it.get("args", []):
        if as_items(a["desc"]):
            ps, _, _, _, ret = _params(od, as_items(a["desc"]))
            vals = "".join(f", {nm}: {t}" for nm, t in ps)
            return f" // gk.items(cx, state, proc(s: ^S, cx: ^Cx{vals}) -> {ret})"
    return ""


def signature(od, d):
    """(number, Odin procedure type) of an item descriptor's kind; numbers are given in order of
    first use in a run (the Odin emitter's), so a run makes the same numbers for the same sources."""
    ps, _, _, _, ret = _params(od, d)
    ty = f"proc(s: ^S, cx: ^Cx{''.join(f', {nm}: {t}' for nm, t in ps)}) -> {ret}"
    sigs = od.__dict__.setdefault("item_sigs", {})
    if ty not in sigs:
        sigs[ty] = (len(sigs) + 1, d)
    return sigs[ty][0], ty


def odin_emit(od):
    """gen_items.odin: `gk.items`, one member per kind of item procedure."""
    sigs = getattr(od, "item_sigs", {})
    od.open("gen_items.odin", "Item procedures (docs/PROTOCOLS.md 9.41): gk.items, one member per kind of procedure.")
    names = []
    lines = []
    for ty, (n, d) in sorted(sigs.items(), key=lambda x: x[1][0]):
        ps, reads, makes, args, ret = _params(od, d)
        name = f"items_{n}"
        names.append(name)
        lines += [f"// {name}: {', '.join(t for _, t in ps)} -> {ret}",
                  f"{name} :: proc(cx: ^Cx, state: ^$S, describe: {ty}, loc := #caller_location) -> Row_Source {{",
                  "\ttramp :: proc(fn, state: rawptr, cx: ^Cx, at: Row_At) -> Element {"]
        if reads:
            lines.append(f"\t\tVals :: struct {{{', '.join(f'{f}: {t}' for f, t in reads)}}}")
            lines.append("\t\tvals := row_context(cx, Row_Source{build.view.src}, at, Vals)")
        lines += ["\t\t" + s for s in makes]
        lines.append(f"\t\treturn (cast({ty})fn)(cast(^S)state, cx{''.join(', ' + a for a in args)})")
        lines += ["\t}", f"\treturn items_source(cx, state, rawptr(describe), tramp, {n}, loc)", "}", ""]
    od.add("// items makes the row source of an item procedure: a procedure that describes one item of a component",
           "// (a calendar's day, a tree's entry, a popover's content), called between frames with the values the",
           "// kit hands that item. The same (procedure, state) pair gives the same source; it may be called in a view.",
           "items :: proc {" + ", ".join(names) + "}" if names else "items :: proc {}", "")
    od.add("@(private)", "ITEMS_KINDS := [?]string{\"\", " + ", ".join(json.dumps(t) for t in sorted(sigs, key=lambda t: sigs[t][0])) + "}", "")
    od.add(*lines)
    od.close()
