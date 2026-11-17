"""The lists and editor families' generator rules (docs/PROTOCOLS.md sections 9.9 to 9.12).

Hooked into gk_model.py, gk_rust.py and gk_odin.py at a few points each (search for
`gk_lists.`); everything else of the family's rules lives here.

- **Associated types of a shim** (`qualified`). A state or element generic over a delegate
  names the delegate's associated types in its methods: `selection(&self) -> &[(IndexPath,
  D::Item)]`, `selected_value(&self) -> Option<<D::Item as SearchableListItem>::Value>`. With D
  instantiated for a library shim (`crate::shim_select::GkSearchDelegate`), the projection is
  the shim's own type, read from `[lists] assoc` in api/overrides.toml:
  `"crate::shim_select::GkSearchDelegate::Item" = "crate::shim_select::GkItem"`. The value is a
  library type (`crate::...`), a primitive, or an upstream or gpui type by name.
- **Library values** (`libval`). A library type that travels both ways as a plain value (the
  shim's item: `gk.Search_Item` in Odin) is `["libval", rust path, odin name]`: Rust reads it
  with its `genapi::Unwire` impl and writes it with its `genapi::Wire` impl (both hand-written
  beside the type), Odin declares the struct in the family's kit file and reads and writes it by
  reflection (`rd_value`, `put_value`), field for field in wire order. `[lists] values` names
  them: `"crate::shim_select::GkItem" = "Search_Item"`.
"""
import gk_comp6  # the components family's rules (PROTOCOLS.md 11.17 to 11.23, 12.16)
import gk_items  # item procedures (PROTOCOLS.md 9.41)


def table(m, name):
    return m.ov.get("lists", {}).get(name, {})


def libval(m, path, out):
    """The descriptor of a library value type, or None when `path` is not one."""
    odin = table(m, "values").get(path)
    if odin is None:
        return None
    m.ov_used.add("lists.values." + path)
    return ["libval", path, odin]


def resolve(m, ctx, t):
    """A type reference with generic parameters of the context and projections on library shims
    replaced: (crate, ref) or None when nothing is known about it."""
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
    """`D::Item`, `<D::Item as Trait>::Value`: the type `[lists] assoc` gives for the shim D was
    instantiated with, as (crate, type ref); None when D is not a library shim with that entry."""
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
    """Every key of the family's override tables matched something (as overrides.toml asks). Also
    drops the out codec of an enum with a lifetime parameter (`PaneRef<'a>`, met as the result of
    a read the object rule then refuses): its Wire impl cannot be written without the lifetime."""
    for e in m.denums.values():
        if e.get("out") and _has_lifetime(m, e):
            e["out"] = False
    for name in ("assoc", "values", "unions", "delegates"):
        for key in table(m, name):
            if f"lists.{name}.{key}" not in m.ov_used:
                raise SystemExit(f"gkgen: overrides [lists] {name} names nothing the generator met: {key}")


def view_closure(params, wrap, ptypes):
    """An element closure without a payload that gets `&App`, not `&mut App` (`empty(Fn(&mut
    Window, &App) -> E)`): a fragment cannot be decoded without `&mut App`, so the app passes a
    `gk.View` and the closure draws it: `["viewfn", params, wrap, ptypes]`. Element decoders only
    (the view is mounted where the element is decoded, as a CHILD_VIEW)."""
    return ["viewfn", params, wrap, ptypes]


def rust_dec(rs, d):
    """The Rust expression for the family's descriptors, or None (gk_rust.Rust.dec's hook)."""
    k = d[0]
    ip = gk_items.rust_dec(rs, d)
    if ip is not None:
        return ip  # an item procedure (PROTOCOLS.md 9.41)
    c6 = gk_comp6.rust_dec(rs, d)
    if c6 is not None:
        return c6  # the components family's PROTOCOLS.md 11.19 to 11.22, 12.16
    if k == "handler" and rs.call:
        # A handler in a call op: a kept handler (slot, generation), fired like a subscription's.
        return (rs.handler(d).replace("dx.handler(r)", "crate::shim_editor::kept(r)")
                .replace("let k = dx.pin(); ", "").replace("let _ = &k; ", ""))  # a kept handler: no tape
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
        return rust_ask(rs, d)  # a data closure (PROTOCOLS.md 9.14)
    if k == "ctxrows":
        return rust_ctxrows(rs, d)  # an element closure over a context (PROTOCOLS.md 9.16)
    if k == "rangeof":
        return "{ let a = %s; a..%s }" % (rs.dec(d[1]), rs.dec(d[1]))
    if k == "rcvec":
        return "std::%s::%s::new(%s)" % ("rc" if d[2] == "Rc" else "sync", d[2], rs.dec(d[1]))
    if k == "viewfn":
        ps = rs.closure_params(d[1], d[3])
        return rs.wrap(d[2], "{ let h = r.u64(); let v = if h == 0 { None } else { dx.child_view(h) }; "
                             "move |%s| crate::shim_select::view_element(&v) }" % ps)
    return None


def odin_type(od, d, out=False):
    """The Odin type of the family's descriptors, or None (gk_odin.Odin.otype's hook)."""
    ip = gk_items.odin_type(od, d, out)
    if ip is not None:
        return ip  # an item procedure's rows (PROTOCOLS.md 9.41)
    c6 = gk_comp6.odin_type(od, d, out)
    if c6 is not None:
        return c6  # the components family's PROTOCOLS.md 11.19 to 11.22, 12.16
    if d[0] == "libval":
        return d[2]
    if d[0] == "viewfn":
        return "View"
    if d[0] == "char":
        return "rune"
    if d[0] == "expr":
        return "string"
    if d[0] == "arcslice":
        return "[]" + od.otype(d[1], out)
    if d[0] == "iter":
        return "[]" + od.otype(d[1], out)
    if d[0] == "oneof":
        return table(od.m, "unions")[d[1]]
    if d[0] == "nonzero":
        return {"usize": "int"}.get(d[1], d[1])
    if d[0] == "font":
        return "Font"
    if d[0] == "ask":
        return f"Ask({od.otype(d[1], True)}, {od.otype(d[2], False)})"  # PROTOCOLS.md 9.14
    if d[0] == "imgsrc":
        return "string"
    if d[0] == "rangeof":
        return f"[2]{od.otype(d[1], out)}"
    if d[0] == "rcvec":
        return od.otype(d[1], out)
    if d[0] == "ctxrows":
        return "Row_Source"  # an element closure over a context: its rows (PROTOCOLS.md 9.16)
    if d[0] == "lent":
        t = od.otype(d[1], True)  # an object lent by kit state (PROTOCOLS.md 9.17)
        return f"Maybe({t})" if d[3] else t
    if d[0] == "uscroll":
        return od.otype(d[1], True)  # a uniform list's scroll handle as gpui's ScrollHandle (9.19)
    return None


def odin_put(od, d, v):
    """Statements that write the family's descriptors, or None (gk_odin.Odin.put's hook)."""
    ip = gk_items.odin_put(od, d, v)
    if ip is not None:
        return ip  # an item procedure's rows (PROTOCOLS.md 9.41)
    c6 = gk_comp6.odin_put(od, d, v)
    if c6 is not None:
        return c6  # the components family's PROTOCOLS.md 11.19 to 11.22, 12.16
    if d[0] == "handler" and getattr(od, "in_call", False):
        # A handler given to a call op (a state's or an object's method, or a builder of its
        # constructor): kept, as a subscription's is (PROTOCOLS.md 9.12).
        return [f"put_kept_handler(o, {v}, {od.m.shapes[d[1]]['num']})"]
    if d[0] == "libval":
        return [f"{{ lv := {v}; put_value(o, &lv, type_info_of({d[2]})) }}"]
    if d[0] == "viewfn":
        return [f"put_view_arg(o, {v})"]
    if d[0] == "char":
        return [f"put_u32(o, u32({v}))"]
    if d[0] == "expr":
        return [f"put_expr(o, {v}, {len(d[2])})"]
    if d[0] == "arcslice":
        return od.put(["list", d[1], "vec"], v)
    if d[0] == "oneof":
        # One of the trait's node types: the union's variant, attached as a child node.
        out = ["{", f"\toe: Element", f"\tswitch x in {v} {{"]
        for k in d[2]:
            out += [f"\tcase {od.m.nodes[k]['odin']}:", "\t\toe = x.el"]
        return out + ["\t}", "\tput_u32(o, take(oe))", "}"]
    if d[0] == "nonzero":
        return [f"put_nonzero(o, u64({v}), {8 if d[1] in ('u64', 'usize') else {'u8': 1, 'u16': 2, 'u32': 4}[d[1]]})"]
    if d[0] == "font":
        return [f"{{ fv := {v}; put_value(o, &fv, type_info_of(Font)) }}"]
    if d[0] == "rangeof":
        return od.put(d[1], f"{v}[0]") + od.put(d[1], f"{v}[1]")
    if d[0] == "rcvec":
        return od.put(d[1], v)
    if d[0] == "ask":
        return [f"put_u64(o, {v}.src.h)"]  # a data closure's ask (PROTOCOLS.md 9.14)
    if d[0] == "ctxrows":
        return [f"put_u64(o, {v}.h)"]  # an element closure over a context (PROTOCOLS.md 9.16)
    return None


SHIMS_OUT = {"crate::shim_rows::GkListDelegate", "crate::shim_rows::GkTableDelegate",
             "crate::shim_select::GkSearchDelegate"}


def shim_out(m, path):
    """Whether a library delegate shim travels out (a state's `delegate()` read back): as the row
    source it is made over (its Wire impl writes the source's handle; Odin: a `Row_Source`)."""
    return path in SHIMS_OUT


def shim_result(m, ctx, out, out_s):
    """A state method returning `&D` or `&mut D` for a delegate shim D (`ListState::delegate`,
    `delegate_mut`): the result is the shim by value as the wire sees it (its row source), so the
    method is a read or a call like any other. Anything else is returned unchanged."""
    if out is None or "borrowed_ref" not in out:
        return out, out_s
    inner = out["borrowed_ref"]["type"]
    r = resolve(m, ctx, inner)
    if r is None or "verbatim" not in r[1] or r[1]["verbatim"] not in SHIMS_OUT:
        return out, out_s
    return inner, r[1]["verbatim"]


def from_str_ref(t):
    """`impl From<&str> for T` (not `&'static str`, which a string read from a request is not):
    the Into set of T gets a string member (`MaskPattern::from("999-999")` is upstream's Pattern
    variant, whose tokens are private), PROTOCOLS.md 9.12."""
    b = t.get("borrowed_ref")
    return bool(b) and not b["is_mutable"] and b["type"].get("primitive") == "str" and b.get("lifetime") in (None, "'_")


def from_arg(src, e):
    """The argument of `From<src>::from` for a decoded member: a `&str` source takes `&*e` (the
    member is decoded as a String in call ops and as &str in tapes)."""
    return f"&*{e}" if src == "&str" else e


def gpui_value(m, hc, name, args, out):
    """gpui's `Font` (a family, a weight and a style; its features and fallbacks are the
    defaults `gpui::font` gives) as `["font"]`: Odin's `gk.Font`, written and read by reflection;
    Rust `crate::shim_editor::font` and its Wire impl (PROTOCOLS.md 9.11). None for other types."""
    from gk_model import GPUI_HOMES  # noqa: PLC0415 (gk_model imports this module)
    if name == "Font" and not args and hc in GPUI_HOMES:
        return ["font"]
    if name == "Position" and not args and hc == "lsp_types":
        return ["libval", "lsp_types::Position", "Lsp_Position"]  # a cursor position (9.13; Wire in shim_text.rs)
    if name == "SharedUri" and out and not args and hc in GPUI_HOMES:
        return ["str", "SharedUri"]  # a URI an ask is given (PROTOCOLS.md 9.14; Wire in shim_text.rs)
    if name == "ImageSource" and not out and not args and hc in GPUI_HOMES and getattr(m, "_ask", False):
        return ["imgsrc"]  # what an ask answers for an image: a path or a URL (9.14)
    if name == "CodeBlock" and out and not args and hc not in GPUI_HOMES:
        return ["libval", "gpui_kit::base::text::CodeBlock", "Code_Block"]  # Wire in shim_text.rs
    if name == "CompletionItem" and not args and hc == "lsp_types":
        # An LSP completion item, as the editor's completion provider answers one (gk.Completion_Item:
        # label, kind, detail, insert text; Unwire in shim_input.rs), PROTOCOLS.md 9.19.
        return ["libval", "lsp_types::CompletionItem", "Completion_Item"]
    if out and hc == "ropey" and name in ("Rope", "RopeSlice"):
        return ["str", name]  # an editor's text read back: the whole text (Wire impls in shim_editor.rs)
    if out and name == "UniformListScrollHandle" and not args and hc in GPUI_HOMES:
        # A uniform list's scroll handle read back (`TreeState::scroll_handle`): the gpui ScrollHandle
        # it scrolls by, the values family's object (PROTOCOLS.md 13.14), so the app scrolls and reads
        # it as any gk.Scroll_Handle (9.19).
        import gk_values  # noqa: PLC0415
        return ["uscroll", gk_values.gpui_object(m, "ScrollHandle")]
    return None


# Third-party types the library carries (PROTOCOLS.md 9.19): (crate, name) -> the Rust path.
THIRD_PARTY = {("ropey", "Rope"): "ropey::Rope", ("ropey", "RopeSlice"): "ropey::RopeSlice",
               ("lsp_types", "CompletionItem"): "lsp_types::CompletionItem",
               ("chrono", "NaiveDate"): "chrono::NaiveDate"}  # a set of dates (Matcher::Custom, PROTOCOLS.md 11.21)


def third_party_path(hc, name):
    """The Rust path of a third-party type the family's rules carry (gk_model.rtype's hook): an
    editor's text read back inside a tuple (`InputEditsEvent::edits`), an LSP completion item given
    to the completion highlighters (`highlight_prefix`). None for any other type."""
    return THIRD_PARTY.get((hc, name))


def payload_object(m, hc, hid, f):
    """A library object that is an event's payload (`InlineTokenClickEvent`, `DiffLinePosition`):
    a copy behind a temporary handle, as a read's result is (PROTOCOLS.md 13.2), so its reads are
    the handler's to call. Only a Clone type can be copied out of the borrow upstream hands the
    closure."""
    return bool(f.get("clone")) and m.is_obj(hc, hid)


def _has_lifetime(m, e):
    name = e["path"].rsplit("::", 1)[-1]
    for cr in m.api.walks:
        for iid, (_, it) in m.api.walks[cr].types.items():
            if it["name"] == name and "enum" in it.get("inner", {}):
                return any("lifetime" in p["kind"] for p in it["inner"]["enum"]["generics"]["params"])
    return False


# ---------------------------------------------------------------- expression closures (9.12)

def _num_kind(m, d):
    """How a value of descriptor d becomes a number of an expression, or None."""
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
    """A closure that maps numbers to a number (arguments numbers, Pixels, bools or fieldless
    enums; the result a number, Pixels or a bool; at most the app as context): an expression the
    library evaluates (`["expr", result kind, argument kinds, wrap, ptypes, params]`, Odin a
    `string`, PROTOCOLS.md 9.12). None for any other closure."""
    if output is None or not payload or any(p not in ("arg", "app", "app_ref") for p in params):
        return None
    try:
        rk = _num_kind(m, m.desc(ctx, output))
        kinds = [_num_kind(m, m.desc(ctx, t)) for t in payload]
    except Exception:  # noqa: BLE001 (an Unsup: not an expression closure)
        return None
    if rk is None or rk.startswith(("enum", "ref")) or None in kinds:
        return None
    return ["expr", rk, kinds, wrap, ptypes, params]


def _to_f64(m, k, v):
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
    if k == "f64":
        return v
    if k == "f32":
        return f"({v} as f32)"
    if k.startswith("int:"):
        return f"({v} as {k[4:]})"
    if k == "bool":
        return f"({v} != 0.0)"
    return f"gpui_kit::px({v} as f32)"  # px


def rust_expr(rs, d):
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


# ---------------------------------------------------------------- std shapes of options (9.12)

def resolved(m, ctx, t, out):
    """`Arc<[T]>`, `Rc<[T]>`, `Box<[T]>` (a list: `Easing::LinearStops`) and `NonZeroU32` and
    its kin (`Easing::Steps { count }`), both ways (Wire impls in shim_editor.rs). None for any
    other type."""
    rp = t["resolved_path"]
    name = rp["path"].rsplit("::", 1)[-1]
    al = alias(m, ctx, t, out)  # a delegate or an element closure behind a type alias (9.16, 9.18)
    if al is not None:
        return al
    from gk_model import path_args  # noqa: PLC0415 (gk_model imports this module)
    args = path_args(rp)
    if name in ("Arc", "Rc", "Box") and len(args) == 1 and "slice" in args[0]:
        return ["arcslice", m.desc(ctx, args[0]["slice"], out), name]
    if name in NONZERO and not args:
        return ["nonzero", NONZERO[name]]
    if name == "Range" and len(args) == 1 and not out and "impl_trait" in args[0]:
        # Range<impl Into<X>> (`Diagnostic::new(Range<impl Into<Position>>, ..)`): a range of X,
        # `[2]X` in Odin (PROTOCOLS.md 9.13).
        from gk_model import bound_parts  # noqa: PLC0415
        for n, tr in bound_parts(args[0]["impl_trait"]):
            ga = ((tr.get("args") or {}).get("angle_bracketed") or {}).get("args", [])
            if n == "Into" and len(ga) == 1 and "type" in ga[0]:
                return ["rangeof", m.desc(ctx, ga[0]["type"], out)]
    if name in ("Rc", "Arc") and len(args) == 1 and "resolved_path" in args[0] \
            and args[0]["resolved_path"]["path"].rsplit("::", 1)[-1] == "Vec":
        # Rc<Vec<T>>, Arc<Vec<T>>: the list, both ways (Wire in shim_text.rs, PROTOCOLS.md 9.13)
        return ["rcvec", m.desc(ctx, args[0], out), name]
    return None


NONZERO = {"NonZeroU8": "u8", "NonZeroU16": "u16", "NonZeroU32": "u32", "NonZeroU64": "u64", "NonZeroUsize": "usize"}


def rust_std(rs, d):
    if d[0] == "arcslice":
        ctor = {"Arc": "std::sync::Arc::from", "Rc": "std::rc::Rc::from", "Box": "Box::from"}[d[2]]
        return "{ let v: Vec<_> = %s; %s(v) }" % (rs.dec(["list", d[1], "vec"]), ctor)
    if d[0] == "nonzero":
        t = {"u8": "U8", "u16": "U16", "u32": "U32", "u64": "U64", "usize": "Usize"}[d[1]]
        get = "r.u64() as usize" if d[1] == "usize" else f"r.{d[1]}()"
        return f"std::num::NonZero{t}::new({get}).unwrap_or(std::num::NonZero{t}::MIN)"
    return None


# ---------------------------------------------------------------- iterators read back (9.11)

ITERS = ("Iterator", "DoubleEndedIterator", "ExactSizeIterator")


def bounded_out(m, ctx, bounds, out):
    """`impl Iterator<Item = X>` (with DoubleEndedIterator, ExactSizeIterator) as a result: the
    items collected into a list (`History::entries`, `DiagnosticSet::iter`). Going in, a trait
    of `[lists] unions` whose implementors are node types (`impl ChildElement`): one of them, a
    union in Odin (`["oneof", trait, node keys]`). None otherwise."""
    from gk_model import Unsup, bound_parts  # noqa: PLC0415 (gk_model imports this module)
    b6 = gk_comp6.bounded(m, ctx, bounds, out)
    if b6 is not None:
        return b6  # a list per row of a data table (PROTOCOLS.md 12.16)
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
            # A closure handed out (`cubic_bezier(..) -> impl Fn(f32) -> f32`): nothing can carry it.
            raise Unsup("todo:protocol: data (a closure read back)", n)
        if n in ITERS:
            for cst in ((tr.get("args") or {}).get("angle_bracketed") or {}).get("constraints", []):
                eq = cst.get("binding", {}).get("equality")
                if cst.get("name") == "Item" and eq and "type" in eq:
                    return ["iter", m.desc(ctx, eq["type"], out)]
    return None


def put_ret(ret, expr):
    """A call op's result written by the Rust emitter, for the family's descriptors; None
    otherwise (gk_rust.Rust.put_ret's hook)."""
    if ret[0] == "iter":
        return f"genapi::Wire::put(&({expr}).collect::<Vec<_>>(), &mut out)"
    if ret[0] == "lent":
        return rust_lent(ret, expr)  # an object lent by kit state (PROTOCOLS.md 9.17)
    if ret[0] == "uscroll" or ret[0] == "ref" and ret[1][0] == "uscroll":
        return f"genapi::Wire::put(&({expr}).0.borrow().base_handle.clone(), &mut out)"  # 9.19
    return None


def rust_arm(rs, it, call, bail):
    """A node op whose argument is one of a trait's node types (`child(impl ChildElement)`), or a
    list of them (`children`): the node's type picks the decoder, and the op is called per child.
    None for any other op (gk_rust.Rust.arm's hook)."""
    args = [a for a in it["args"] if a["desc"][0] != "ctx"]
    if len(args) != 1:
        return None
    d = args[0]["desc"]
    many = d[0] == "list" and d[1][0] == "oneof"
    if not many and d[0] != "oneof":
        return None
    keys = (d[1] if many else d)[2]
    bail = bail if bail is not None else "return None"  # a constructor's arm: no value without the child
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


# ---------------------------------------------------------------- Default as a constructor (9.13)

def default_ctor(m, st, f, crate):
    """A library object with no constructor of its own that implements Default (`StatusColors`,
    `SyntaxColors`: a theme's colours with every value left to the theme) is made by
    `Default::default`: a synthetic constructor (kept handle, not counted as an upstream function),
    so its reads become reachable (PROTOCOLS.md 9.13)."""
    if not f.get("default") or f.get("ctor") or st["glob"]:
        return
    key = st["key"]
    it = {"row": None, "owner": key, "trait": "Default", "crate": crate, "synthetic": True, "name": "default",
          "id": f"{key}::Default::default", "kind": "obj-ctor", "status": "bridged", "reason": "", "state": key,
          "args": [], "callee": f"<{st['path']} as Default>::default", "sig": "fn default() -> Self"}
    st["ctors"].append(it)
    m.items.append(it)


# ---------------------------------------------------------------- data closures (9.14)

ASK_KEY = {"bool", "int", "f32", "f64", "str", "leaf", "enum", "opt", "list", "tuple", "rec", "ref", "libval", "denum"}
ASK_RET = {"bool", "int", "f32", "f64", "str", "leaf", "enum", "opt", "list", "tuple", "rec", "denum", "imgsrc"}


def ask_closure(m, ctx, kind, inputs, output, wrap, params, ptypes, payload):
    """A closure that returns a value which is not a number (`code_block_highlighter(Fn(&CodeBlock)
    -> Vec<(Range<usize>, HighlightStyle)>)`, `image_source(Fn(&SharedUri) -> ImageSource)`): the
    library answers it from a cache keyed by the arguments' bytes and asks the app between frames
    for a key it has not got (the result type's default meanwhile), PROTOCOLS.md 9.14:
    `["ask", key, result, wrap, ptypes, params, rust result type]`. The arguments are values the
    app reads (no objects or handles: a key is their bytes); the result is a value the app writes
    (no nodes or handles), or an image source (a path or URL). None for any other closure."""
    from gk_model import Unsup  # noqa: PLC0415
    if output is None or not payload or any(p not in ("arg", "window", "app", "app_ref", "window_ref") for p in params):
        return None
    try:
        out_s = m.rtype(ctx, output)
        if out_s in ("bool", "()"):
            return None
        keys = [m.desc(ctx, t, out=True) for t in payload]
        m._ask = True  # an image source answered by the app is a path or a URL (gpui_value)
        try:
            ret = m.desc(ctx, output)
        finally:
            m._ask = False
    except Unsup:
        return None
    from gk_model import walk_desc  # noqa: PLC0415
    if any(x[0] not in ASK_KEY or (x[0] == "leaf" and x[1] not in ("px", "range", "color")) for k in keys for x in walk_desc(k)):
        return None
    if any(x[0] not in ASK_RET or (x[0] == "leaf" and x[1] in ("child", "id")) for x in walk_desc(ret)):
        return None
    key = keys[0] if len(keys) == 1 else ["tuple", keys]
    out_s = out_s.replace("std::ops::range::Range", "std::ops::Range")  # the private module rustdoc names
    return ["ask", key, ret, wrap, ptypes, params, out_s]


def rust_ask(rs, d):
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
        rs.call = None  # an answer is decoded as a tape's value is, with a context over no tape (dx)
        try:
            dec = rs.dec(ret)
        finally:
            rs.call = was
        s_t, conv, fb = rt, "|v| v", "Default::default"
    body = (f"a.get::<{s_t}, {rt}>(crate::shim_text::key(&{args}), "
            f"|r: &mut genapi::R, dx: &mut genapi::Dx| -> Option<{s_t}> {{ let _ = &dx; Some({dec}) }}, {conv}, {fb})")
    return rs.wrap(wrap, "{ let a = crate::shim_text::Ask::take(r.u64()); move |%s| -> %s { %s } }" % (", ".join(ps), rt, body))


# ---------------------------------------------------------------- values a static makes several of (9.15)



def list_ctor(m, node, it, crate, fn, fctx):
    """A static of a value type that returns several of it (`DiffFile::parse(patch) ->
    Result<Vec<Self>, DiffParseError>`, `parse_with`): two synthetic functions (PROTOCOLS.md 9.15),
    `<type>_<name>_count`, an ambient call that runs the static and returns how many values it made
    (a `gk.Fallible(int)`: the error's text when it fails), and the constructor `<type>_<name>_at`
    with the same arguments and an index, whose value is the static's value at that index (the
    library runs the static again where the value is decoded, keeping its last result by its
    arguments' bytes). The upstream function counts as bridged. True when bound."""
    import re as _re  # noqa: PLC0415
    from gk_model import Unsup, path_args  # noqa: PLC0415
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
    # the count: an ambient call (PROTOCOLS.md 11.5) of the node type
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
    # the value at an index: a constructor of the node type
    at = {"row": None, "owner": it["owner"], "trait": None, "crate": crate, "synthetic": True,
          "name": f"{it['name']}_at", "id": f"{it['id']}#at", "kind": "ctor", "status": "bridged", "reason": "",
          "args": [dict(a) for a in args] + [{"name": "index", "desc": ["int", "usize"]}], "node": key,
          "callee": f"(|{ps}, index: usize| crate::shim_text::nth(<{tp}>::{it['name']}({ps}), index))",
          "sig": f"fn {it['name']}_at(.., index: usize) -> Self"}
    node["ctors"].append(at)
    m.items.append(at)
    names = f"{count['name']} and {it['name']}_at"
    if fallible:
        # The static's error as the kit's own value (`DiffParseError`: its line and message), when
        # it is a library object: `<type>_<name>_error`, an ambient call returning it or nothing
        # (PROTOCOLS.md 9.19).
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
    import rdj  # noqa: PLC0415
    return rdj.ty(t)


# ---------------------------------------------------------------- element closures over a context (9.16)

# Descriptors of these families a call op can decode (gk_model.call_bad's hook): a row source handle.
CALL_SAFE = {"ctxrows", "rows", "items"} | gk_comp6.CALL_SAFE  # items: an item procedure's rows (PROTOCOLS.md 9.41)


def ctx_rows(m, ctx, kind, inputs, output, wrap, params, ptypes, payload):
    """An element closure that gets the window, the app and one to three values the kit hands it
    (`Input::token(Fn(&InlineTokenContext, &mut Window, &mut App) -> R)`, `Diff::render_header(Fn(
    &DiffFile, ..) -> E)`, an editor's scrollbar renderer `Fn(EditorScrollbarContext, ..)`): a row
    source the app passes (PROTOCOLS.md 9.16). Each call of the closure in a frame is a row of the
    source, keyed by its order in the frame; the row's view reads the values with
    `gk.row_context`: plain values as they are, a library object as a copy behind a temporary
    handle (so its type must be Clone). The row is described again when the values change
    (compared by their bytes; an object by the bytes of its reads that take nothing).
    `["ctxrows", wrap, ptypes, params, descs]`; None for any other closure."""
    from gk_model import Unsup, walk_desc  # noqa: PLC0415
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
    """Library objects an item hands the app besides its result: the values of an element closure
    over a context (9.16), which make those objects reachable (gk_model.reach_objects' hook)."""
    from gk_model import walk_desc  # noqa: PLC0415
    for a in it.get("args", []):
        for x in walk_desc(a["desc"]):
            yield from gk_items.handed_out(x)  # an item procedure's objects (PROTOCOLS.md 9.41)
            if x[0] == "ctxrows":
                for d in x[4]:
                    for y in walk_desc(d):
                        if y[0] == "obj":
                            yield y[2]


def _ctx_print(rs, d, v):
    """Statements that add the identity of one closure value to `p` (9.16): a plain value's bytes;
    an object's reads that take nothing and return plain values."""
    from gk_model import walk_desc  # noqa: PLC0415
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
    wrap, ptypes, params, descs = d[1:5]
    opt = len(d) > 5 and d[5] == "opt"  # -> Option<AnyElement>: Some(the row) (PROTOCOLS.md 11.19)
    ps, lets, puts, n = [], [], [], 0
    for p, t in zip(params, ptypes):
        if p == "arg":
            v, dd = f"k{n}", descs[n]
            ps.append(f"{v}: {t}")
            lets += _ctx_print(rs, dd, v)
            base = dd[1] if dd[0] == "ref" else dd
            if base[0] == "obj":
                lets.append(f"let c{n} = {v}.clone();")  # copied out at read time: a temporary handle
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
    # The only value an index (a scroller's item): the row is keyed by it, not by the call's order.
    deref = "*" if ptypes[params.index("arg")].startswith("&") else ""
    by = f"Some(({deref}k0) as u64)" if len(descs) == 1 and descs[0][0] == "int" else "None"
    ret = "Option<gpui_kit::AnyElement>" if opt else "gpui_kit::AnyElement"
    body = ("{ let src = r.u64(); move |%s| -> %s { let mut p: Vec<u8> = Vec::new(); %s "
            "%s(crate::source::ctx_row(src, %s, p, std::rc::Rc::new(move |o: &mut Vec<u8>| { %s }), window, cx)) } }"
            % (", ".join(ps), ret, " ".join(lets), "Some" if opt else "", by, " ".join(puts)))
    if wrap in ("rc", "box", "arc"):
        # Named as the trait object, so that an Option of it (`Option<EditorScrollbarRenderer>`) has its type.
        ptr = {"rc": "std::rc::Rc", "box": "Box", "arc": "std::sync::Arc"}[wrap]
        dyn = f"dyn Fn({', '.join(ptypes)}) -> {ret}"
        return f"{{ let f: {ptr}<{dyn}> = {rs.wrap(wrap, body)}; f }}"
    return rs.wrap(wrap, body)


def alias(m, ctx, t, out):
    """A use of a type alias (`EditorScrollbarRenderer`, `InputHighlighterFactory`): a delegate the
    library makes over a row source when `[lists] delegates` names the alias (PROTOCOLS.md 9.18:
    `["rows", shim]`), an element closure over a context when the alias's type is one (9.16), else
    None (the alias stays what it was)."""
    from gk_model import Ctx, Unsup  # noqa: PLC0415
    rp = t["resolved_path"]
    from gk_model import path_args  # noqa: PLC0415
    args = path_args(rp)
    if rp["path"].rsplit("::", 1)[-1] in ("Rc", "Arc", "Box") and len(args) == 1 and "dyn_trait" in args[0] and not out:
        # `Rc<dyn LanguageProvider>`: a delegate the library makes when `[lists] delegates` names
        # "dyn Trait" (PROTOCOLS.md 9.18).
        names = [x["trait"]["path"].rsplit("::", 1)[-1] for x in args[0]["dyn_trait"]["traits"]]
        shim = table(m, "delegates").get("dyn " + names[0]) if len(names) == 1 else None
        if shim:
            m.ov_used.add("lists.delegates.dyn " + names[0])
            return ["rows", shim]
        return None
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:  # noqa: BLE001
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
    return gk_comp6.kitfn(m, hc, hid, it, d) if d[0] == "ctxrows" else None  # or a kit function's (11.20)


# ---------------------------------------------------------------- objects lent by kit state (9.17)

def lend(m, ctx, st, out):
    """A state method `fn x_mut(&mut self) -> &mut T` or `-> Option<&mut T>` where T is a library
    object that is Clone (`InputBaseState::diagnostics_mut`): a call that lends the object
    (PROTOCOLS.md 9.17). The app gets a temporary handle to a copy, writes to it with the turn's
    buffered writes, and the library gives the copy back to the state when the turn's writes have
    landed (then the state redraws). `["lent", obj desc, state path, optional]`; None otherwise."""
    from gk_model import last, path_args  # noqa: PLC0415
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
    _, d, sp, opt = ret
    T = d[1]
    got = f"({expr}).map(|v| v.clone())" if opt else f"Some(({expr}).clone())"
    acc = expr if opt else f"Some({expr})"
    put = "genapi::Wire::put(&lent, &mut out)" if opt else "genapi::Put::put_u64(&mut out, lent.unwrap_or(0))"
    return (f"{{ let copy: Option<{T}> = {got}; let lent: Option<u64> = crate::shim_editor::lend::<{sp}, {T}>(h, copy, "
            f"|s| {{ {acc} }}, cx); {put}; }}")
