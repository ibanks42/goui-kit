"""Item-render closure descriptors and Rust decoder emission.

Many upstream components take per-item rendering callbacks that pass context values,
read-only state/value structs, `AnyView` instances, or pre-built elements (such as a
calendar day cell or tree row) along with `(&mut Window, &mut App)` and expect an
element back. Rather than hand-writing a protocol shim for each component, this module
generalizes them into the `items` descriptor:

    ["items", wrap, ptypes, params, vals, out]

Where:
- `wrap`: closure wrapper kind (`"impl"`, `"rc"`, `"box"`, `"arc"`).
- `ptypes` / `params`: Rust parameter type strings and role tags (`"arg"`, `"window"`, `"app"`, `"context"`).
- `vals`: one entry per `"arg"` payload, classified by `value()`:
  - `["view"]`: an owned `AnyView` stashed in the runtime's handed-object vector and exposed to Go as `KitView`.
  - `["handed", node_key, reads_rt]`: a pre-built element node stashed for Go to modify via its synthetic
    `(handed)` constructor, plus an optional `Reads` snapshot record of its getters.
  - `["reads", reads_rt]`: a synthetic `crate::items::Reads<'_, T>` record copying plain getter results out on the wire.
  - `["val", desc]`: a directly wire-encodable value (or `Clone` object handle).
  - `["none"]`: a borrowed state/struct with no wire-readable getters (accepted by the closure, omitted from the Go callback).
- `out`: `"el"` for `impl IntoElement` / `AnyElement`, or `["node", node_key]` for a concrete element type.

During Rust codegen (`rust_dec`), the emitted closure serializes its payload values into a
cache key `p` and a deferred writer closure, boxes any handed elements/views into `h`, and
calls `crate::items::row` (or `crate::source::ctx_row` when nothing is handed) to request
the rendered subtree from Go's `Items` callback.
"""
import json

import rdj

# Wire opcode for KitView (matches core/wire/ops.txt).
N_KIT_VIEW = 41
# Descriptor tags that cannot appear inside a plain read-only snapshot record field because
# they carry handles, tape nodes, closures, or mutable references rather than inert data.
_PLAIN_BAD = ("obj", "node", "handle", "entity", "shim", "lent", "rows", "build", "fragment", "child", "handler",
              "view", "dock", "ctxrows", "items")


def _g():
    import gk_model
    return gk_model


def build_over_handed(m, ctx, kind, inputs, output, wrap, params, ptypes, payload, static):
    """Match a single-argument `FnOnce(El) -> El` closure that configures a handed element node.

    Unlike full `items` closures (which run later during render with `&mut Window` and
    require a round-trip to Go), a pure `FnOnce(El) -> El` runs inline while building
    the parent node and maps to the simpler `build` descriptor (applied via a sub-tape).
    """
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
    """Classify an item-rendering closure into an `items` descriptor, or return None.

    Requires 1..4 payload arguments, a `&mut Window` parameter, and either `&mut App` or
    `&mut Context<T>`. Return type must be `impl IntoElement` / `AnyElement` (`is_el=True`)
    or a concrete element node for which `_fallback()` can construct a placeholder element
    before Go supplies its first frame. Records every handed element node in `m.items_handed`
    so `post()` synthesizes its `(handed)` constructor.
    """
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
    """Classify one payload argument passed into an item closure.

    Classification priority:
    1. Owned `AnyView` -> `["view"]`.
    2. Owned element node -> `["handed", node_key, reads_rt]` (excluding getters whose
       return type duplicates another sibling payload argument in `payload`).
    3. Value-like `Clone` struct with pure getters -> `["reads", reads_rt]` (or `["none"]`).
    4. Directly wire-encodable outbound value (restricted to `gk_lists.ASK_KEY` kinds,
       `px`/`range`/`color` leaves, and `Clone` objects so the value can be serialized
       and cached as a key) -> `["val", desc]`.
    5. Borrowed state or opaque context struct -> `["reads", reads_rt]` or `["none"]`.
    """
    g = _g()
    by_ref = "borrowed_ref" in t
    inner = t["borrowed_ref"]["type"] if by_ref else t
    if "generic" in inner:
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
        import gk_lists
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
    """Collect public inherent methods `(name, inputs, output, generics)` on a struct, ignoring trait and synthetic impls."""
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
    """Synthesize a `crate::items::Reads<'_, T>` output record from T's zero-arg getter methods.

    `mode` controls eligibility:
    - `"value"`: `Clone`, non-state, non-record, non-generic structs whose inherent methods
      are exclusively zero-arg getters or `Self`-returning builders.
    - `"node"`: element nodes handed into an item closure (skips getters returning sibling
      payload types listed in `skip_types` to avoid duplicate data on the wire).
    - `"state"` / `"none"`: state structs borrowed by reference; returns `""` when no plain
      getters exist so the caller can still treat the argument as `["none"]`.

    Invariant: inspecting a getter's return type with `out=True` must not flip an
    input-only data enum's `out` flag (which would change that enum's generated Go
    representation for every caller). Any flipped `denum` is rolled back and recorded
    in `rc["left"]` so `post()` can mark that getter as `skipped: builder-getter`.
    """
    g = _g()
    if "resolved_path" not in t:
        return None
    rp = t["resolved_path"]
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:
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
        # In "value" mode, reject structs that have mutating or non-builder methods.
        if mode == "value" and not (recv in (None, "Self") and outs in ("Self", f["name"])):
            return None
        if mode == "none" and not f["state"]:
            return None
    fields, left = [], {}
    for name, outp, by_value in reads:
        if rdj.ty(outp).lstrip("&").replace("mut ", "") in skip_types:
            continue
        was_out = {k: e.get("out") for k, e in m.denums.items()}
        try:
            # Clear closure nesting depth so m.desc() treats the getter return as a normal outbound type.
            sv = m._closure
            m._closure = 0
            try:
                d = m.desc(sctx, outp, out=True)
            finally:
                m._closure = sv
        except g.Unsup:
            continue
        # Roll back any input-only data enum that m.desc(..., out=True) would have promoted to outbound.
        flipped = [k for k, e in m.denums.items() if e.get("out") and not was_out.get(k)]
        if flipped:
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
    rt = "crate::items::Reads<'_, " + path + (f"<{', '.join(gargs)}>" if gargs else "") + ">"
    named = f["name"] + ("Reads" if mode in ("state", "node") or f["state"] else "")
    if rt in m.records:
        m.records[rt]["out"] = True
        return rt
    m.records[rt] = {"rust": rt, "path": path, "name": named, "fields": fields, "tuple": False, "complete": False,
                     "in": False, "out": True, "crate": hc, "args": [], "snap": path, "item_reads": True, "left": left}
    return rt


def field_put(rc, f):
    """Emit `Wire::put` for one field of a `crate::items::Reads` wrapper record.

    Getters that take `self` by value clone `self.0` first because `Reads` only borrows `&T`.
    """
    if not rc.get("item_reads"):
        return None
    if f.get("by_value"):
        return f"genapi::Wire::put(&self.0.clone().{f['name']}(), out);"
    return f"genapi::Wire::put(&self.0.{f['name']}(), out);"


def post(m):
    """Finalize `Reads` records and synthesize `(handed)` constructors after classification.

    1. Disambiguate `Reads` record names when the underlying type is also a reachable state
       or object handle (appending `Reads` so the snapshot struct does not collide in Go).
    2. Mark upstream getter methods whose values are copied into a `Reads` record as `bridged`
       (and mark rolled-back data-enum getters from `rc["left"]` as `skipped: builder-getter`).
    3. Append a synthetic `(handed)` constructor taking a `u8` slot index to every node in
       `m.items_handed` so Go callbacks can claim the pre-built element handed by Rust.
    """
    from gk_model import ada
    for rc in m.records.values():
        st = m.states.get(rc["path"]) if rc.get("item_reads") else None
        if st and not rc["name"].endswith("Reads") and (not st.get("obj") or st.get("reachable")):
            rc["name"] += "Reads"
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
                             f"read back, its Go type would change for every app); not a field of {ada(rc['name'])}")
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
    """Yield every library object key passed out through a `val` payload in an `items` closure.

    Called by reachability passes so objects only ever introduced via item callbacks are
    marked reachable and have their methods generated.
    """
    g = _g()
    if d[0] != "items":
        return
    for v in d[4]:
        if v[0] == "val":
            for y in g.walk_desc(v[1]):
                if y[0] == "obj":
                    yield y[2]


def _fallback(m, key):
    """Build a placeholder Rust element expression used before Go renders a typed item closure's first frame.

    When an upstream closure returns a concrete element type (rather than `AnyElement`),
    `crate::items::row` still needs a valid value of that type if the Go reply has not
    populated the cache yet. Supports `Stateful<Div>` directly and any element with a
    single-ID `new("gk-item")` constructor.
    """
    node = m.nodes[key]
    if node["path"].replace(" ", "") in ("gpui_kit::Stateful<gpui_kit::Div>",):
        return 'gpui_kit::InteractiveElement::id(gpui_kit::div(), "gk-item")'
    for it in node["ctors"]:
        if it["name"] == "new" and it["status"] in ("bound", "bridged") and len(it["args"]) == 1 \
                and it["args"][0]["desc"][0] in ("leaf", "into", "from", "str") and "id" in json.dumps(it["args"][0]["desc"]):
            return f'{it["callee"]}("gk-item")'
    return None


def rust_dec(rs, d):
    """Emit Rust decoder expression for an `items` closure descriptor.

    The emitted closure reads the Go `RowSource` handle (`src = r.u64()`) when decoding the
    parent node, then on each invocation:
    1. Builds a cache fingerprint `p: Vec<u8>` and an `Rc<dyn Fn(&mut Vec<u8>)>` writer
       that serializes the payload arguments for Go's `rowContext()` reader (cloning `obj`
       handles so the deferred writer stays `'static`).
    2. Captures the first integer payload (if any) as `by: Option<u64>` so index-keyed rows
       can be matched directly.
    3. Boxes any handed elements or `AnyView` arguments into `h: Vec<Box<dyn Any>>`.
    4. Dispatches to `crate::source::ctx_row` (when `out == "el"` and nothing is handed)
       or `crate::items::row` with a typed decoder and fallback closure.
    """
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
                import gk_lists
                lets += gk_lists._ctx_print(rs, v[1], k)
                base = v[1][1] if v[1][0] == "ref" else v[1]
                if base[0] == "obj":
                    # Clone the object handle into the deferred wire-writer closure so it is 'static.
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
            # When the closure received a handed element or AnyView, fall back to rendering it
            # untouched rather than an empty div if Go has not replied yet.
            fallback = ("|h| match crate::items::first::<%s>(h) { Some(v) => gpui_kit::IntoElement::into_any_element(v), "
                        "None => gpui_kit::IntoElement::into_any_element(gpui_kit::div()) }" % fb)
        else:
            fallback = "|_| gpui_kit::IntoElement::into_any_element(gpui_kit::div())"
        if not handed:
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
    """Wrap an emitted closure in `Rc<dyn Fn(...)>` / `Box<dyn Fn(...)>` / `Arc<dyn Fn(...)>` when required.

    Explicit trait-object type ascription prevents Rust type-inference ambiguity when passing
    the wrapped closure into generic builder arguments.
    """
    if wrap in ("rc", "box", "arc"):
        ptr = {"rc": "std::rc::Rc", "box": "Box", "arc": "std::sync::Arc"}[wrap]
        return f"{{ let f: {ptr}<dyn Fn({', '.join(ptypes)}) -> {ret}> = {rs.wrap(wrap, body)}; f }}"
    return rs.wrap(wrap, body)
