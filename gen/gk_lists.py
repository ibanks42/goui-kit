"""List, table, delegate, ask-closure, expression, and iterator descriptor rules.

Extends `gen/gk_model.py` and `gen/gk_rust.py` with the type/closure rules needed for
virtualized lists, tables, comboboxes, select dropdowns, charts, and editor text helpers:

1. **Override-driven resolution (`table`, `libval`, `resolve`, `qualified`, `check_used`)**:
   Reads `[lists]` sub-tables from `api/overrides.toml`:
   - `assoc`: maps `<Shim>::Assoc` qualified paths (like `GkListDelegate::Item`) to concrete types.
   - `values`: maps third-party/kit struct paths to `["libval", rust_path, stem]` codecs.
   - `unions`: maps element marker traits (`impl Trait` parameters) to closed `["oneof", trait, [nodes]]` unions.
   - `delegates`: registers delegate traits bridged by hand-written Rust shims (`SHIMS_OUT`).
   `check_used()` asserts every configured entry was exercised during classification so stale
   overrides fail generation, and disables `out` codecs on lifetime-bearing data enums.
2. **Closures converted to data (`expr_closure`, `ask_closure`, `ctx_rows`, `view_closure`)**:
   - `expr`: pure numeric/bool/unit-enum -> numeric/bool/px closures compiled into
     `crate::shim_editor::Expr` strings evaluated synchronously on the Rust thread (zero IPC).
   - `ask`: key -> value closures backed by `crate::shim_text::Ask` (async lookup table;
     caches Go answers by serialized key and returns `Default::default()` until populated).
   - `ctxrows`: 1..3-arg `(&..., &mut Window, &mut App) -> impl IntoElement` closures backed
     by `crate::source::ctx_row`, fingerprinting argument values (`_ctx_print`) to avoid
     re-requesting unchanged rows.
   - `viewfn`: closures returning a child `AnyView` backed by `crate::shim_select::view_element`.
3. **Standard wrappers and `Vec<Self>` factories (`resolved`, `bounded_out`, `list_ctor`, `rust_arm`)**:
   - Handles `Arc<[T]>`/`Rc<[T]>`/`Box<[T]>` (`arcslice`), `NonZero*` (`nonzero`, clamping 0
     to `MIN`), `Range<impl Into<T>>` (`rangeof`), `Rc<Vec<T>>`/`Arc<Vec<T>>` (`rcvec`),
     `impl Iterator<Item = T>` (`iter`, collected into `Vec<T>` on wire write), and
     `UniformListScrollHandle` (`uscroll`, projected to its inner `ScrollHandle`).
   - `list_ctor()` splits static `Vec<Self>` / `Result<Vec<Self>, E>` factories into
     `*_count`, indexed `*_at(index)`, and optional `*_error` items so tape nodes can construct
     one element at a time.
   - `rust_arm()` emits opcode-dispatched match arms for builders taking `oneof` or `Vec<oneof>`.
"""
import gk_comp6
import gk_items


def table(m, name):
    """Return a sub-table under `[lists]` in `api/overrides.toml` (`assoc`, `values`, `unions`, `delegates`)."""
    return m.ov.get("lists", {}).get(name, {})


def libval(m, path, out):
    """Look up a runtime value struct in `[lists].values` and return `["libval", path, stem]`."""
    stem = table(m, "values").get(path)
    if stem is None:
        return None
    m.ov_used.add("lists.values." + path)
    return ["libval", path, stem]


def resolve(m, ctx, t):
    """Substitute generic parameters, `Self`, or overridden associated types in a rustdoc type node."""
    if "generic" in t:
        g = t["generic"]
        if g in ctx.subst:
            return ctx.subst[g]
        if g == "Self" and ctx.self_t:
            return ctx.self_t
        return None
    if "qualified_path" in t:
        return qualified(m, ctx, t)
    return ctx.crate, t


def qualified(m, ctx, t):
    """Resolve `T::Assoc` against `[lists].assoc` when `T` resolves to a runtime shim verbatim path.

    Used when a generic state method references an associated type of a delegate shim
    (for example `<GkTableDelegate as TableDelegate>::Column`), which rustdoc does not
    monomorphize automatically.
    """
    q = t["qualified_path"]
    base = resolve(m, ctx, q["self_type"])
    if base is None or "verbatim" not in base[1]:
        return None
    key = f"{base[1]['verbatim']}::{q['name']}"
    target = table(m, "assoc").get(key)
    if target is None:
        return None
    m.ov_used.add("lists.assoc." + key)
    cr, ref, _ = m.inst_arg(base[0], target)
    return cr, ref


def check_used(m):
    """Disable outbound emission on lifetime-bearing data enums and assert every `[lists]` override was used.

    Lifetime-bearing data enums cannot implement `'static` outbound `Wire` helpers safely,
    so their `out` flag is cleared before codegen. Failing on unused `[lists]` entries keeps
    `api/overrides.toml` clean when upstream APIs change.
    """
    for e in m.denums.values():
        if e.get("out") and _has_lifetime(m, e):
            e["out"] = False
    for name in ("assoc", "values", "unions", "delegates"):
        for key in table(m, name):
            if f"lists.{name}.{key}" not in m.ov_used:
                raise SystemExit(f"gkgen: overrides [lists] {name} names nothing the generator met: {key}")


def view_closure(params, wrap, ptypes):
    """Build a `viewfn` descriptor for a closure returning a child view element."""
    return ["viewfn", params, wrap, ptypes]


def rust_dec(rs, d):
    """Emit Rust decoder code for descriptors handled by `gk_lists`, `gk_items`, or `gk_comp6`.

    Key subtlety: when a `handler` descriptor appears inside a call op (`rs.call` is set,
    such as registering a callback on an entity state rather than on a single-frame view
    node), the closure outlives any single render tape. It therefore rewrites `dx.handler(r)`
    to `crate::shim_editor::kept(r)` and drops the frame pin (`dx.pin()`).
    """
    k = d[0]
    ip = gk_items.rust_dec(rs, d)
    if ip is not None:
        return ip
    c6 = gk_comp6.rust_dec(rs, d)
    if c6 is not None:
        return c6
    if k == "handler" and rs.call:
        # Call-site handlers outlive any single render tape, so keep them in the editor handler table.
        return (rs.handler(d).replace("dx.handler(r)", "crate::shim_editor::kept(r)")
                .replace("let k = dx.pin(); ", "").replace("let _ = &k; ", ""))
    if k == "libval":
        return f"<{d[1]} as genapi::Unwire>::get(r)"
    if k == "char":
        return "char::from_u32(r.u32()).unwrap_or('\\u{fffd}')"
    if k == "expr":
        return rust_expr(rs, d)
    if k in ("arcslice", "nonzero"):
        return rust_std(rs, d)
    if k == "font":
        return "crate::shim_editor::font(r)"
    if k == "ask":
        return rust_ask(rs, d)
    if k == "ctxrows":
        return rust_ctxrows(rs, d)
    if k == "rangeof":
        return "{ let a = %s; a..%s }" % (rs.dec(d[1]), rs.dec(d[1]))
    if k == "rcvec":
        return "std::%s::%s::new(%s)" % ("rc" if d[2] == "Rc" else "sync", d[2], rs.dec(d[1]))
    if k == "viewfn":
        ps = rs.closure_params(d[1], d[3])
        return rs.wrap(d[2], "{ let h = r.u64(); let v = if h == 0 { None } else { dx.child_view(h) }; "
                             "move |%s| crate::shim_select::view_element(&v) }" % ps)
    return None


# Hand-written delegate shims whose `&D` return values are skipped as direct object handles
# because Go interacts with their data through the runtime's delegate protocol instead.
SHIMS_OUT = {"crate::shim_rows::GkListDelegate", "crate::shim_rows::GkTableDelegate",
             "crate::shim_select::GkSearchDelegate"}


def shim_out(m, path):
    """Return True if `path` is one of the hand-written delegate shims in `SHIMS_OUT`."""
    return path in SHIMS_OUT


def shim_result(m, ctx, out, out_s):
    """Unwrap `&D` return types when `D` resolves to a runtime delegate shim in `SHIMS_OUT`."""
    if out is None or "borrowed_ref" not in out:
        return out, out_s
    inner = out["borrowed_ref"]["type"]
    r = resolve(m, ctx, inner)
    if r is None or "verbatim" not in r[1] or r[1]["verbatim"] not in SHIMS_OUT:
        return out, out_s
    return inner, r[1]["verbatim"]


def from_str_ref(t):
    """Return True if rustdoc type `t` is an elided `&str` reference (`&str` or `&'_ str`)."""
    b = t.get("borrowed_ref")
    return bool(b) and not b["is_mutable"] and b["type"].get("primitive") == "str" and b.get("lifetime") in (None, "'_")


def from_arg(src, e):
    """Re-borrow an owned decoded `String` expression as `&*e` when a `From<&str>` impl expects `&str`."""
    return f"&*{e}" if src == "&str" else e


def gpui_value(m, hc, name, args, out):
    """Classify third-party and GPUI types wired through dedicated codecs or runtime shims.

    Handles `gpui::Font`, `lsp_types::Position` and `CompletionItem`, `SharedUri`,
    `ImageSource` (inside `Ask` returns), `CodeBlock`, `ropey::Rope`/`RopeSlice`, and
    `UniformListScrollHandle` (projected to its underlying `ScrollHandle`).
    """
    from gk_model import GPUI_HOMES
    if name == "Font" and not args and hc in GPUI_HOMES:
        return ["font"]
    if name == "Position" and not args and hc == "lsp_types":
        return ["libval", "lsp_types::Position", "Lsp_Position"]
    if name == "SharedUri" and out and not args and hc in GPUI_HOMES:
        return ["str", "SharedUri"]
    if name == "ImageSource" and not out and not args and hc in GPUI_HOMES and getattr(m, "_ask", False):
        return ["imgsrc"]
    if name == "CodeBlock" and out and not args and hc not in GPUI_HOMES:
        return ["libval", "gpui_kit::base::text::CodeBlock", "Code_Block"]
    if name == "CompletionItem" and not args and hc == "lsp_types":
        return ["libval", "lsp_types::CompletionItem", "Completion_Item"]
    if out and hc == "ropey" and name in ("Rope", "RopeSlice"):
        return ["str", name]
    if out and name == "UniformListScrollHandle" and not args and hc in GPUI_HOMES:
        import gk_values
        return ["uscroll", gk_values.gpui_object(m, "ScrollHandle")]
    return None


# Fully-qualified Rust paths for non-kit third-party types re-exported or referenced in signatures.
THIRD_PARTY = {("ropey", "Rope"): "ropey::Rope", ("ropey", "RopeSlice"): "ropey::RopeSlice",
               ("lsp_types", "CompletionItem"): "lsp_types::CompletionItem",
               ("jiff", "Date"): "jiff::civil::Date"}


def third_party_path(hc, name):
    """Look up the Rust path for a known third-party type `(home_crate, name)`."""
    return THIRD_PARTY.get((hc, name))


def payload_object(m, hc, hid, f):
    """Return True if a kit type can be cloned and passed out of a closure as an object handle."""
    return bool(f.get("clone")) and m.is_obj(hc, hid)


def _has_lifetime(m, e):
    """Return True if data enum `e` declares any lifetime generic parameters in rustdoc."""
    name = e["path"].rsplit("::", 1)[-1]
    for cr in m.api.walks:
        for iid, (_, it) in m.api.walks[cr].types.items():
            if it["name"] == name and "enum" in it.get("inner", {}):
                return any("lifetime" in p["kind"] for p in it["inner"]["enum"]["generics"]["params"])
    return False


def _num_kind(m, d):
    """Map a scalar, `Pixels`, unit enum, or shared reference descriptor to its `expr` coercion key."""
    k = d[0]
    if k in ("f32", "f64", "bool"):
        return k
    if k == "int":
        return "int:" + d[1]
    if k == "leaf" and d[1] == "px":
        return "px"
    if k == "enum":
        return "enum:" + d[1]
    if k == "ref":
        inner = _num_kind(m, d[1])
        return inner and "ref:" + inner
    return None


def expr_closure(m, ctx, inputs, output, wrap, params, ptypes, payload):
    """Match a pure numeric/boolean/enum-to-scalar closure encoded as an `Expr` formula string.

    Go passes a compact arithmetic/conditional expression string evaluated on the Rust
    thread via `crate::shim_editor::Expr` with all numeric inputs widened to `f64`.
    """
    if output is None or not payload or any(p not in ("arg", "app", "app_ref") for p in params):
        return None
    try:
        rk = _num_kind(m, m.desc(ctx, output))
        kinds = [_num_kind(m, m.desc(ctx, t)) for t in payload]
    except Exception:
        return None
    if rk is None or rk.startswith(("enum", "ref")) or None in kinds:
        return None
    return ["expr", rk, kinds, wrap, ptypes, params]


def _to_f64(m, k, v):
    """Generate a Rust expression converting an `expr` closure input `v` of kind `k` to `f64`."""
    if k.startswith("ref:"):
        return _to_f64(m, k[4:], f"(*{v})")
    if k in ("f32", "f64") or k.startswith("int:"):
        return f"({v} as f64)"
    if k == "bool":
        return f"(if {v} {{ 1.0 }} else {{ 0.0 }})"
    if k == "px":
        return f"(f32::from({v}) as f64)"
    e = m.enums[k[5:]]
    arms = " ".join(f"{e['path']}::{x['name']} => {float(x['num'])}," for x in e["variants"])
    return f"(match {v} {{ {arms} #[allow(unreachable_patterns)] _ => -1.0 }})"


def _from_f64(k, v):
    """Generate a Rust expression narrowing an `Expr::eval` `f64` result back to return kind `k`."""
    if k == "f64":
        return v
    if k == "f32":
        return f"({v} as f32)"
    if k.startswith("int:"):
        return f"({v} as {k[4:]})"
    if k == "bool":
        return f"({v} != 0.0)"
    return f"gpui_kit::px({v} as f32)"


def rust_expr(rs, d):
    """Emit Rust decoder for an `expr` descriptor wrapping `crate::shim_editor::Expr`."""
    _, rk, kinds, wrap, ptypes, params = d
    ps, conv, n = [], [], 0
    for p, t in zip(params, ptypes):
        if p == "arg":
            ps.append(f"a{n}: {t}")
            conv.append(_to_f64(rs.m, kinds[n], f"a{n}"))
            n += 1
        else:
            ps.append(f"_cx: {t}")
    body = _from_f64(rk, f"e.eval(&[{', '.join(conv)}])")
    return rs.wrap(wrap, "{ let e = crate::shim_editor::Expr::new(r.str()); move |%s| { %s } }" % (", ".join(ps), body))


def resolved(m, ctx, t, out):
    """Classify standard wrapper types (`Arc<[T]>`, `NonZero*`, `Range<impl Into<T>>`, `Rc<Vec<T>>`) and aliases."""
    rp = t["resolved_path"]
    name = rp["path"].rsplit("::", 1)[-1]
    al = alias(m, ctx, t, out)
    if al is not None:
        return al
    from gk_model import path_args
    args = path_args(rp)
    if name in ("Arc", "Rc", "Box") and len(args) == 1 and "slice" in args[0]:
        return ["arcslice", m.desc(ctx, args[0]["slice"], out), name]
    if name in NONZERO and not args:
        return ["nonzero", NONZERO[name]]
    if name == "Range" and len(args) == 1 and not out and "impl_trait" in args[0]:
        from gk_model import bound_parts
        for n, tr in bound_parts(args[0]["impl_trait"]):
            ga = ((tr.get("args") or {}).get("angle_bracketed") or {}).get("args", [])
            if n == "Into" and len(ga) == 1 and "type" in ga[0]:
                return ["rangeof", m.desc(ctx, ga[0]["type"], out)]
    if name in ("Rc", "Arc") and len(args) == 1 and "resolved_path" in args[0] \
            and args[0]["resolved_path"]["path"].rsplit("::", 1)[-1] == "Vec":
        return ["rcvec", m.desc(ctx, args[0], out), name]
    return None


NONZERO = {"NonZeroU8": "u8", "NonZeroU16": "u16", "NonZeroU32": "u32", "NonZeroU64": "u64", "NonZeroUsize": "usize"}


def rust_std(rs, d):
    """Emit Rust decoder for `arcslice` (`Arc`/`Rc`/`Box<[T]>`) and `nonzero` (`std::num::NonZero*`)."""
    if d[0] == "arcslice":
        ctor = {"Arc": "std::sync::Arc::from", "Rc": "std::rc::Rc::from", "Box": "Box::from"}[d[2]]
        return "{ let v: Vec<_> = %s; %s(v) }" % (rs.dec(["list", d[1], "vec"]), ctor)
    if d[0] == "nonzero":
        t = {"u8": "U8", "u16": "U16", "u32": "U32", "u64": "U64", "usize": "Usize"}[d[1]]
        get = "r.u64() as usize" if d[1] == "usize" else f"r.{d[1]}()"
        # Clamp 0 to MIN so malformed zero values never panic during tape decode.
        return f"std::num::NonZero{t}::new({get}).unwrap_or(std::num::NonZero{t}::MIN)"
    return None


ITERS = ("Iterator", "DoubleEndedIterator", "ExactSizeIterator")


def bounded_out(m, ctx, bounds, out):
    """Classify `impl Trait` bounds into `oneof` element unions (input) or `iter` descriptors (output)."""
    from gk_model import Unsup, bound_parts
    b6 = gk_comp6.bounded(m, ctx, bounds, out)
    if b6 is not None:
        return b6
    if not out:
        for n, _ in bound_parts(bounds):
            if n in table(m, "unions"):
                m.ov_used.add("lists.unions." + n)
                keys = sorted(k for k, nd in m.nodes.items() if nd["element"] and nd["inst"] is None
                              and n in m.info(nd["crate"], nd["id"])["traits"])
                if keys:
                    return ["oneof", n, keys]
        return None
    for n, tr in bound_parts(bounds):
        if n in ("Fn", "FnMut", "FnOnce"):
            raise Unsup("todo:protocol: data (a closure read back)", n)
        if n in ITERS:
            for cst in ((tr.get("args") or {}).get("angle_bracketed") or {}).get("constraints", []):
                eq = cst.get("binding", {}).get("equality")
                if cst.get("name") == "Item" and eq and "type" in eq:
                    return ["iter", m.desc(ctx, eq["type"], out)]
    return None


def put_ret(ret, expr):
    """Emit Rust wire serialization for `iter`, `lent`, and `uscroll` return descriptors."""
    if ret[0] == "iter":
        return f"genapi::Wire::put(&({expr}).collect::<Vec<_>>(), &mut out)"
    if ret[0] == "lent":
        return rust_lent(ret, expr)
    if ret[0] == "uscroll" or ret[0] == "ref" and ret[1][0] == "uscroll":
        return f"genapi::Wire::put(&({expr}).0.borrow().base_handle.clone(), &mut out)"
    return None


def rust_arm(rs, it, call, bail):
    """Emit a Rust match arm for ops accepting a single `oneof` element union or `Vec<oneof>`.

    Dispatches on each child tape node's constructor opcode to build the concrete Rust
    type (`make_<fn>`) and apply `call` before folding back into the parent element.
    """
    args = [a for a in it["args"] if a["desc"][0] != "ctx"]
    if len(args) != 1:
        return None
    d = args[0]["desc"]
    many = d[0] == "list" and d[1][0] == "oneof"
    if not many and d[0] != "oneof":
        return None
    keys = (d[1] if many else d)[2]
    bail = bail if bail is not None else "return None"
    arms = []
    for k in keys:
        nd = rs.m.nodes[k]
        tys = sorted(i["op"] for i in nd["ctors"] if i["status"] in ("bound", "bridged") and i.get("op") is not None)
        if tys:
            arms.append((" | ".join(map(str, tys)), f"make_{nd['fn']}"))
    if many:
        recv = call.replace("(el", "(acc", 1)
        body = " ".join(f"{t} => match {mk}(dx, ty, a, f) {{ Some(v) => {recv % 'vec![v]'}, None => acc }}," for t, mk in arms)
        return ("{ let k = r.u32(); let mut acc = el; for _ in 0..k { let n = r.u32(); let (ty, a, f) = dx.node(n); "
                "acc = match ty { %s _ => { bad_node::<()>(\"a child element\", ty); acc } }; } acc }" % body)
    body = " ".join(f"{t} => match {mk}(dx, ty, a, f) {{ Some(v) => {call % 'v'}, None => {{ {bail} }} }}," for t, mk in arms)
    return ("{ let n = r.u32(); let (ty, a, f) = dx.node(n); match ty { %s _ => { bad_node::<()>(\"a child element\", ty); %s } } }"
            % (body, bail))


def default_ctor(m, st, f, crate):
    """Synthesize a `Default::default` constructor item for a library object with no inherent ctor."""
    if not f.get("default") or f.get("ctor") or st["glob"]:
        return
    key = st["key"]
    it = {"row": None, "owner": key, "trait": "Default", "crate": crate, "synthetic": True, "name": "default",
          "id": f"{key}::Default::default", "kind": "obj-ctor", "status": "bridged", "reason": "", "state": key,
          "args": [], "callee": f"<{st['path']} as Default>::default", "sig": "fn default() -> Self"}
    st["ctors"].append(it)
    m.items.append(it)


# Allowed descriptor tags for Ask closure keys (serialized to Go) and returns (decoded from Go).
ASK_KEY = {"bool", "int", "f32", "f64", "str", "leaf", "enum", "opt", "list", "tuple", "rec", "ref", "libval", "denum"}
ASK_RET = {"bool", "int", "f32", "f64", "str", "leaf", "enum", "opt", "list", "tuple", "rec", "denum", "imgsrc"}


def ask_closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload):
    """Match a value-returning closure that can be served by an async key->value `Ask` table.

    At paint time Rust looks up the serialized key in `crate::shim_text::Ask`; on a cache
    miss it enqueues a request for Go and returns `Default::default()` until answered.
    """
    from gk_model import Unsup
    if output is None or not payload or any(p not in ("arg", "window", "app", "app_ref", "window_ref") for p in params):
        return None
    try:
        out_s = m.rtype(ctx, output)
        if out_s in ("bool", "()"):
            return None
        keys = [m.desc(ctx, t, out=True) for t in payload]
        m._ask = True
        try:
            ret = m.desc(ctx, output)
        finally:
            m._ask = False
    except Unsup:
        return None
    from gk_model import walk_desc
    if any(x[0] not in ASK_KEY or (x[0] == "leaf" and x[1] not in ("px", "range", "color")) for k in keys for x in walk_desc(k)):
        return None
    if any(x[0] not in ASK_RET or (x[0] == "leaf" and x[1] in ("child", "id")) for x in walk_desc(ret)):
        return None
    key = keys[0] if len(keys) == 1 else ["tuple", keys]
    out_s = out_s.replace("std::ops::range::Range", "std::ops::Range")
    return ["ask", key, ret, wrap, ptypes, params, out_s]


def rust_ask(rs, d):
    """Emit Rust decoder for an `ask` closure backed by `crate::shim_text::Ask`."""
    _, key, ret, wrap, ptypes, params, rt = d
    ps, names, n = [], [], 0
    for p, t in zip(params, ptypes):
        if p == "arg":
            ps.append(f"k{n}: {t}")
            names.append(f"k{n}")
            n += 1
        else:
            ps.append(f"_{p}: {t}")
    args = names[0] if len(names) == 1 else "(" + ", ".join(f"&{x}" for x in names) + ")"
    if ret[0] == "imgsrc":
        s_t, dec, conv, fb = "String", "r.str().to_string()", "crate::shim_text::image_source", \
            "|| crate::shim_text::image_source(String::new())"
    else:
        was = rs.call
        rs.call = None
        try:
            dec = rs.dec(ret)
        finally:
            rs.call = was
        s_t, conv, fb = rt, "|v| v", "Default::default"
    body = (f"a.get::<{s_t}, {rt}>(crate::shim_text::key(&{args}), "
            f"|r: &mut genapi::R, dx: &mut genapi::Dx| -> Option<{s_t}> {{ let _ = &dx; Some({dec}) }}, {conv}, {fb})")
    return rs.wrap(wrap, "{ let a = crate::shim_text::Ask::take(r.u64()); move |%s| -> %s { %s } }" % (", ".join(ps), rt, body))


def list_ctor(m, node, it, crate, fn, fctx):
    """Split a static constructor returning `Vec<Self>` or `Result<Vec<Self>, E>` into count/at/error items.

    Because tape node constructors always build a single `Self`, a vector-returning factory
    (like CSV/JSON table row parsers) becomes an ambient `*_count` query, an indexed
    `*_at(index)` node constructor, and (when `E` is a Clone object) an ambient `*_error` query.
    """
    import re as _re
    from gk_model import Unsup, path_args
    out = fn["sig"].get("output")
    out_s = rdj_ty(out) if out else "()"
    mt = _re.fullmatch(r"(Result<)?Vec<Self>(, [\w:<>]+>)?", out_s)
    if not mt or (mt.group(1) is None) != (mt.group(2) is None):
        return False
    fallible = mt.group(1) is not None
    ins = fn["sig"]["inputs"]
    try:
        args = m.args(fctx, ins)
    except Unsup:
        return False
    if any(a["desc"][0] == "ctx" or m.call_bad(a["desc"], False) is not None for a in args):
        return False
    tp, key = node["path"], node["key"]
    names = [a["name"] for a in args]
    ps = ", ".join(names)
    snake = _re.sub(r"(?<!^)(?=[A-Z])", "_", node["name"]).lower()
    run = f"<{tp}>::{it['name']}({ps})"
    count = {"row": None, "owner": it["owner"], "trait": None, "crate": crate, "synthetic": True,
             "name": f"{snake}_{it['name']}_count", "id": f"{it['id']}#count", "kind": "ambient", "status": "bound",
             "reason": "", "args": [dict(a) for a in args],
             "callee": f"(|{ps}| {run}.map(|v| v.len()))" if fallible else f"(|{ps}| {run}.len())",
             "ret": ["fallible", ["int", "usize"]] if fallible else ["int", "usize"],
             "sig": f"fn {it['name']}_count(..) -> {'Result<usize, E>' if fallible else 'usize'}"}
    if not hasattr(m, "ambient"):
        m.ambient = []
    m.ambient.append(count)
    m.items.append(count)
    at = {"row": None, "owner": it["owner"], "trait": None, "crate": crate, "synthetic": True,
          "name": f"{it['name']}_at", "id": f"{it['id']}#at", "kind": "ctor", "status": "bridged", "reason": "",
          "args": [dict(a) for a in args] + [{"name": "index", "desc": ["int", "usize"]}], "node": key,
          "callee": f"(|{ps}, index: usize| crate::shim_text::nth(<{tp}>::{it['name']}({ps}), index))",
          "sig": f"fn {it['name']}_at(.., index: usize) -> Self"}
    node["ctors"].append(at)
    m.items.append(at)
    names = f"{count['name']} and {it['name']}_at"
    if fallible:
        try:
            ed = m.desc(fctx, path_args(out["resolved_path"])[1], out=True)
        except Unsup:
            ed = None
        if ed is not None and ed[0] == "obj" and ed[3]:
            err = {"row": None, "owner": it["owner"], "trait": None, "crate": crate, "synthetic": True,
                   "name": f"{snake}_{it['name']}_error", "id": f"{it['id']}#error", "kind": "ambient", "status": "bound",
                   "reason": "", "args": [dict(a) for a in args], "callee": f"(|{ps}| {run}.err())",
                   "ret": ["opt", ed], "sig": f"fn {it['name']}_error(..) -> Option<E>"}
            m.ambient.append(err)
            m.items.append(err)
            names += f" and {err['name']}"
    it.update(status="bridged", kind="listed", reason="", detail=names)
    return True


def rdj_ty(t):
    import rdj
    return rdj.ty(t)


CALL_SAFE = {"ctxrows", "rows", "items"} | gk_comp6.CALL_SAFE


def ctx_rows(m, ctx, kind, inputs, output, wrap, params, ptypes, payload):
    """Match an element-rendering closure carrying wire-encodable context arguments."""
    from gk_model import Unsup, walk_desc
    if not payload or len(payload) > 3 or "context" in params or "window" not in params or "app" not in params:
        return None
    try:
        descs = [m.desc(ctx, t, out=True) for t in payload]
    except Unsup:
        return None
    for d in descs:
        for x in walk_desc(d):
            if x[0] == "obj":
                if not x[3]:
                    return None
                continue
            if x[0] not in ASK_KEY or (x[0] == "leaf" and x[1] not in ("px", "range", "color")):
                return None
    return ["ctxrows", wrap, ptypes, params, descs]


def handed_out(it):
    """Yield every library object key handed out through `ctxrows` or `gk_items` closure arguments."""
    from gk_model import walk_desc
    for a in it.get("args", []):
        for x in walk_desc(a["desc"]):
            yield from gk_items.handed_out(x)
            if x[0] == "ctxrows":
                for d in x[4]:
                    for y in walk_desc(d):
                        if y[0] == "obj":
                            yield y[2]


def _ctx_print(rs, d, v):
    """Build Rust statements that serialize a stable fingerprint of row argument `v` into `p`.

    For primitive/record values this wires `v` directly; for object handles it hashes all
    zero-arg pure value getters so `ctx_row` can detect when an object's content changed
    between renders without allocating a fresh handle on every frame.
    """
    from gk_model import walk_desc
    base = d[1] if d[0] == "ref" else d
    if base[0] != "obj":
        return [f"genapi::Wire::put(&{v}, &mut p);"]
    st = rs.m.objects.get(base[2])
    out = []
    for it in (st or {}).get("reads", []):
        if it["status"] not in ("bound", "bridged") or it["args"] or not it.get("ret"):
            continue
        if any(x[0] in ("obj", "node", "handle", "entity", "shim", "lent") for x in walk_desc(it["ret"])):
            continue
        r = "&" + v if d[0] != "ref" else v
        out.append(f"genapi::Wire::put(&{it['callee']}({r}), &mut p);")
    return out


def rust_ctxrows(rs, d):
    """Emit Rust decoder for a `ctxrows` row-source closure.

    Captures a row-source handle from the tape, fingerprints the closure's payload
    arguments into `p`, and calls `crate::source::ctx_row` so Go renders the row on demand
    with lazily serialized argument payloads (`o`).
    """
    wrap, ptypes, params, descs = d[1:5]
    opt = len(d) > 5 and d[5] == "opt"
    ps, lets, puts, n = [], [], [], 0
    for p, t in zip(params, ptypes):
        if p == "arg":
            v, dd = f"k{n}", descs[n]
            ps.append(f"{v}: {t}")
            lets += _ctx_print(rs, dd, v)
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
    deref = "*" if ptypes[params.index("arg")].startswith("&") else ""
    by = f"Some(({deref}k0) as u64)" if len(descs) == 1 and descs[0][0] == "int" else "None"
    ret = "Option<gpui_kit::AnyElement>" if opt else "gpui_kit::AnyElement"
    body = ("{ let src = r.u64(); move |%s| -> %s { let mut p: Vec<u8> = Vec::new(); %s "
            "%s(crate::source::ctx_row(src, %s, p, std::rc::Rc::new(move |o: &mut Vec<u8>| { %s }), window, cx)) } }"
            % (", ".join(ps), ret, " ".join(lets), "Some" if opt else "", by, " ".join(puts)))
    if wrap in ("rc", "box", "arc"):
        ptr = {"rc": "std::rc::Rc", "box": "Box", "arc": "std::sync::Arc"}[wrap]
        dyn = f"dyn Fn({', '.join(ptypes)}) -> {ret}"
        return f"{{ let f: {ptr}<{dyn}> = {rs.wrap(wrap, body)}; f }}"
    return rs.wrap(wrap, body)


def alias(m, ctx, t, out):
    """Resolve delegate trait objects and type aliases (`[lists].delegates` or `ctxrows` -> `kitfn`)."""
    from gk_model import Ctx, Unsup
    rp = t["resolved_path"]
    from gk_model import path_args
    args = path_args(rp)
    if rp["path"].rsplit("::", 1)[-1] in ("Rc", "Arc", "Box") and len(args) == 1 and "dyn_trait" in args[0] and not out:
        names = [x["trait"]["path"].rsplit("::", 1)[-1] for x in args[0]["dyn_trait"]["traits"]]
        shim = table(m, "delegates").get("dyn " + names[0]) if len(names) == 1 else None
        if shim:
            m.ov_used.add("lists.delegates.dyn " + names[0])
            return ["rows", shim]
        return None
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:
        return None
    if it is None or "type_alias" not in it.get("inner", {}):
        return None
    shim = table(m, "delegates").get(it["name"])
    if shim:
        m.ov_used.add("lists.delegates." + it["name"])
        return ["rows", shim]
    if out:
        return None
    try:
        d = m.desc(Ctx(hc), it["inner"]["type_alias"]["type"], out)
    except Unsup:
        return None
    return gk_comp6.kitfn(m, hc, hid, it, d) if d[0] == "ctxrows" else None


def lend(m, ctx, st, out):
    """Match a `&mut T` or `Option<&mut T>` return that lends a Clone library object for editing.

    Because Rust borrows cannot cross the FFI boundary, `crate::shim_editor::lend` clones
    the borrowed field into a temporary handle and records a write-back accessor closure
    that writes mutations back into the owning entity when committed.
    """
    from gk_model import last, path_args
    if out is None:
        return None
    t, opt = out, False
    if "resolved_path" in t and last(t["resolved_path"]["path"]) == "Option" and len(path_args(t["resolved_path"])) == 1:
        t, opt = path_args(t["resolved_path"])[0], True
    b = t.get("borrowed_ref")
    if not b or not b["is_mutable"] or "resolved_path" not in b["type"]:
        return None
    hc, hid, it = m.api.item(ctx.crate, b["type"]["resolved_path"]["id"])
    if it is None or not m.is_obj(hc, hid) or not m.info(hc, hid)["clone"]:
        return None
    d = m.desc(ctx, b["type"])
    if d[0] != "obj":
        return None
    return ["lent", d, st["path"], opt]


def rust_lent(ret, expr):
    """Emit Rust serialization for a `lent` handle returned from `crate::shim_editor::lend`."""
    _, d, sp, opt = ret
    T = d[1]
    got = f"({expr}).map(|v| v.clone())" if opt else f"Some(({expr}).clone())"
    acc = expr if opt else f"Some({expr})"
    put = "genapi::Wire::put(&lent, &mut out)" if opt else "genapi::Put::put_u64(&mut out, lent.unwrap_or(0))"
    return (f"{{ let copy: Option<{T}> = {got}; let lent: Option<u64> = crate::shim_editor::lend::<{sp}, {T}>(h, copy, "
            f"|s| {{ {acc} }}, cx); {put}; }}")
