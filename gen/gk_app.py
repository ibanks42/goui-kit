"""Ambient context calls, action listeners, and platform-level descriptors.

This module plugs into `gen/gk_model.py`, `gen/gk_rust.py`, and `gen/gk_values.py`
to handle APIs that operate on the window/app runtime rather than inside a single
component's tape node:

1. **Ambient context calls (`ambient`, `bound`, `number`)**:
   - Classifies free functions, static associated functions, and `WindowExt` / `ActiveTheme`
     extension trait methods as immediate `ambient` call ops (allocated in `CALL_BASE`
     space via `number()`).
   - Injects implicit `["ctx", "window"]` (`&mut Window`) or `["ctx", "app_ref"]` (`&App`
     for `APP_TRAITS`) receiver arguments so Go callers pass only the explicit parameters.
   - Skips runtime-owned startup hooks (`AMBIENT_SKIP`: `init`) and draw-time-only helpers
     (`DRAW_ONLY`: `hover_progress`, `is_hover_entering`).
2. **Action and `AnyView` closures (`action_listener`, `view_closure`)**:
   - `(&mut Window, &mut App) -> AnyView` -> `["viewany"]` (later extended with `Tooltip`
     by `gk_comp6.viewany_tooltips()`).
   - `(&A, &mut Window, &mut App)` where `A: Action` -> `["action", "listener"]`.
   - `(&dyn Action, &mut Window, &mut App)` -> handler shape `SHAPE_ACTION` (20).
3. **Platform-level descriptors (`resolved`, `platform_bounded`, `rust_dec`, `rust_payload`)**:
   - Handles drag-and-drop (`DragMoveEvent`, `DropAccept`, `ExternalDragPayload`),
     accessibility child slices (`a11ychildren`), `ImageCacheProvider` / `ImageCache`,
     `ScrollAnchor`, `SharedUri`, `Arc<Path>`, `ImageBytes`, `PixelImage`, `CustomImage`,
     and `TryInto<KeyContext>` / `Deref<Target = str>`.
4. **Selection-scope wrapper (`rust_wrappers`)**:
   - Emits `motion_scope` into `core/src/gen/mod.rs` for `ElementExt::text_selection_scope`,
     dispatching over eligible generated element opcodes.
"""
import json

import rdj

# Built-in handler shape ID for `&dyn gpui_kit::Action` listeners (shared with core/wire/ops.txt).
SHAPE_ACTION = 20


def _bound_names(bounds):
    """Extract the trailing trait identifiers from a list of rustdoc GenericBound dicts."""
    out = []
    for b in bounds or []:
        tb = b.get("trait_bound")
        if tb:
            out.append(tb["trait"]["path"].split("::")[-1])
    return out


def view_closure(m, ctx, inputs, output):
    """Match a `(&mut Window, &mut App) -> AnyView` closure and return a `viewany` descriptor."""
    if output is None or rdj.ty(output).split("::")[-1] != "AnyView" or len(inputs) != 2:
        return None
    tys = [rdj.ty(t) for t in inputs]
    if not (tys[0].startswith("&mut") and tys[0].endswith("Window") and tys[1].startswith("&mut") and tys[1].endswith("App")):
        return None
    return ["viewany"]


def action_listener(m, ctx, inputs, output):
    """Match `AnyView` closures, generic `Action` listeners, and `&dyn Action` handlers.

    Generic `fn(&A, &mut Window, &mut App)` where `A: Action` becomes `["action", "listener"]`
    (dispatched by action name on the Rust side). Trait-object `&dyn Action` registers the
    built-in handler shape `SHAPE_ACTION` so Go receives the dispatched action wire value.
    """
    vc = view_closure(m, ctx, inputs, output)
    if vc:
        return vc
    if output is not None and rdj.ty(output) != "()":
        return None
    if len(inputs) != 3:
        return None
    tail = [rdj.ty(t) for t in inputs[1:]]
    if not (tail[0].startswith("&mut") and tail[0].endswith("Window") and tail[1].startswith("&mut")
            and tail[1].endswith("App")):
        return None
    b = inputs[0].get("borrowed_ref")
    if not b or b["is_mutable"]:
        return None
    inner = b["type"]
    g = inner.get("generic")
    if g and g != "Self" and "Action" in _bound_names(ctx.bounds.get(g)):
        return ["action", "listener"]
    dt = inner.get("dyn_trait")
    if dt and [x["trait"]["path"].split("::")[-1] for x in dt["traits"]] == ["Action"]:
        payload = ["action", "dyn"]
        key = json.dumps([[payload], "unit"])
        m.shapes.setdefault(key, {"key": key, "args": [payload], "ret": "unit", "rust": ["&dyn gpui_kit::Action"],
                                  "builtin": SHAPE_ACTION})
        ptypes = ["&dyn gpui_kit::Action", "&mut gpui_kit::Window", "&mut gpui_kit::App"]
        return ["handler", key, ["arg", "window", "app"], "impl", ptypes, [["ref", payload]]]
    return None


# Traits whose `&self` receiver binds to `&gpui_kit::App` rather than `&mut gpui_kit::Window`.
APP_TRAITS = {"ActiveTheme"}


def bounded(m, ctx, name, targs, tr=None):
    """Classify trait-bounded `impl Trait` / generic parameters for platform and app APIs.

    Checks platform bounds (`ImageCacheProvider`), value bounds (`SettingFieldElement`,
    `AnySettingField`), `TryInto<KeyContext>`, `Deref<Target = str>`, and traits
    implemented on core GPUI handle objects (`ScrollbarHandle` -> `ScrollHandle`).
    """
    import gk_values
    pb = platform_bounded(m, ctx, name)
    if pb is not None:
        return pb
    vb = gk_values.bounded(m, ctx, name)
    if vb is not None:
        return vb
    if name == "TryInto" and len(targs) == 1 and rdj.ty(targs[0]).split("::")[-1] == "KeyContext":
        return ["str", "str"]
    if name == "Deref" and tr is not None:
        cs = ((tr.get("args") or {}).get("angle_bracketed") or {}).get("constraints", [])
        if any(c["name"] == "Target" and (c["binding"].get("equality") or {}).get("type", {}).get("primitive") == "str"
               for c in cs):
            return ["str", "deref"]
    import gk_values
    for obj, traits in gk_values.GPUI_OBJECTS.items():
        if name in traits:
            return gk_values.gpui_object(m, obj)
    return None


def bound_only(fn, name):
    """Return True if generic parameter `name` is fixed by gk_values or only constrained via an associated type."""
    import gk_values
    if gk_values.fixed_param(fn, name):
        return True
    lists = [(w.get("bound_predicate") or {}).get("bounds", []) for w in fn["generics"].get("where_predicates", [])]
    lists += [p["kind"]["type"]["bounds"] for p in fn["generics"]["params"] if "type" in p["kind"]]
    for bounds in lists:
        for b in bounds:
            tb = b.get("trait_bound")
            args = ((tb or {}).get("trait", {}).get("args") or {}).get("angle_bracketed") or {}
            for c in args.get("constraints", []):
                eq = c.get("binding", {}).get("equality") or {}
                if eq.get("type", {}).get("generic") == name:
                    return True
    return False


# Free functions intentionally skipped because the runtime owns initialization or paint-time state.
AMBIENT_SKIP = {"init"}
DRAW_ONLY = {"hover_progress", "is_hover_entering"}


def ambient(m, it, crate, fn, tpath, trait=None, self_t=None):
    """Classify a free function, static call, or Window/App trait method as an ambient call op.

    Ambient calls execute immediately via `call` opcodes rather than inside a view tape.
    For extension traits (`WindowExt`, `ActiveTheme`), the receiver is injected as an
    implicit `["ctx", "window"]` or `["ctx", "app_ref"]` argument so Go callers supply
    only the explicit method arguments. Returns True if the item was handled (bound or skipped).
    """
    from gk_model import Ctx, Unsup
    import gk_values
    if it["name"] in AMBIENT_SKIP and trait is None:
        m.set(it, Unsup("skip:rust-only-extension", "the library runs it itself when it starts (gpui_kit::init); "
                        "a second call would register the kit's actions, key bindings and globals twice"))
        return True
    if it["name"] in DRAW_ONLY and trait is None:
        m.set(it, Unsup("skip:rust-only-extension", "reads the keyed state of the element being drawn; only a "
                        "component's render may call it"))
        return True
    ins = fn["sig"]["inputs"]
    out = fn["sig"].get("output")
    recv = []
    on = "Window"
    if trait is not None:
        if trait in APP_TRAITS and ins and ins[0][0] == "self" and rdj.ty(ins[0][1]) == "&Self":
            recv, ins, on = [{"name": "cx", "desc": ["ctx", "app_ref"]}], ins[1:], "App"
        elif not ins or ins[0][0] != "self" or rdj.ty(ins[0][1]) != "&mut Self":
            return False
        else:
            recv, ins = [{"name": "window", "desc": ["ctx", "window"]}], ins[1:]
    try:
        text = " ".join(rdj.ty(t) for _, t in ins)
        for p in fn["generics"]["params"]:
            if "type" in p["kind"] and not p["kind"]["type"].get("is_synthetic") and \
                    not __import__("re").search(rf"\b{p['name']}\b", text) and not bound_only(fn, p["name"]):
                return False
        fctx = m.fn_ctx(Ctx(crate, self_t), fn)
        args = m.args(fctx, ins)
        for a in args:
            if a["desc"][0] != "ctx" and m.call_bad(a["desc"], False) is not None:
                return False
        ret = None
        if out is not None and rdj.ty(out) != "()":
            ret = m.desc(fctx, out, out=True)
            bad = m.call_bad(ret, False)
            if bad == "leaf" and gk_values.ret_readable(m, ret):
                bad = None
            if bad is not None:
                return False
    except Unsup:
        return False
    callee = it["id"] if trait is None else f"<gpui_kit::{on} as {tpath}>::{it['name']}"
    it.update(kind="ambient", status="bound", reason="", callee=callee, args=recv + args,
              crate=crate, trait=trait or it.get("trait"), borrowed_ret=bool(out and "borrowed_ref" in out))
    if ret is not None:
        it["ret"] = ret
    if not hasattr(m, "ambient"):
        m.ambient = []
    m.ambient.append(it)
    return True


def bound(m):
    """Return every ambient item in the model whose status is `bound`."""
    return [i for i in getattr(m, "ambient", []) if i["status"] == "bound"]


def number(m, lock, base):
    """Assign stable call opcodes to bound ambient items using `api/opcodes.lock`."""
    for it in sorted(bound(m), key=lambda i: i["id"]):
        it["op"] = lock.num("call", "", it["id"], first=base)


def rust_calls(rs):
    """Emit Rust call handlers `c_<op>` for every bound ambient function.

    Each handler reads non-context arguments from the call tape, acquires `&mut Window`
    from `Call::window()` when needed, invokes the upstream callee, and serializes any
    return value into `c.out`.
    """
    arms = []
    for it in sorted(bound(rs.m), key=lambda i: i["op"]):
        rs.call = "ambient"
        lets = "".join(f"let a{n} = {rs.dec(a['desc'])}; " for n, a in enumerate(it["args"]) if a["desc"][0] != "ctx")
        need_w = any(a["desc"][0] == "ctx" and a["desc"][1].startswith("window") for a in it["args"])
        ctxs = ("let window = c.window()?; " if need_w else "") + "let cx = &mut *c.cx; "
        passed = ", ".join(rs.passed(a["desc"], f"a{n}") for n, a in enumerate(it["args"]))
        if it.get("ret") is None:
            body = f"{lets}{ctxs}{it['callee']}({passed}); let _ = cx;"
        elif it["ret"] == ["str", "deref"] or it.get("borrowed_ret"):
            # Borrowed returns (&T or Deref<Target=str>) serialize directly before the local drops.
            e = "&*v" if it["ret"] == ["str", "deref"] else "&v"
            body = (f"{lets}{ctxs}let v = {it['callee']}({passed}); "
                    f"let mut out = Vec::new(); genapi::Wire::put({e}, &mut out); c.out.extend_from_slice(&out);")
        else:
            body = (f"{lets}{ctxs}let v = {it['callee']}({passed}); let _ = cx; "
                    f"let mut out = Vec::new(); {rs.put_ret(it['ret'], 'v')}; c.out.extend_from_slice(&out);")
        rs.add(f"fn c_{it['op']}(r: &mut R, c: &mut genapi::Call) -> genapi::GkResult<()> {{ {body} Ok(()) }}",
               ["item", it["id"]])
        arms.append(it)
    rs.call = None
    return arms


# Platform-layer descriptor tags decoded by rust_dec() via core/src/platform.rs helpers.
KINDS = {"dropaccept", "dragpayload", "prepaintorder", "a11ychildren", "imagecache", "imagecacheref",
         "scrollanchor", "uri", "arcpath", "imagebytes", "pixelimage", "customimage"}


def _cx_tail(tys):
    """Return True if the last two parameter types are `(&mut Window, &mut App)`."""
    return (len(tys) >= 2 and tys[-2].startswith("&mut") and tys[-2].endswith("Window")
            and tys[-1].startswith("&mut") and tys[-1].endswith("App"))


def platform_closure(m, ctx, kind, inputs, output, wrap):
    """Match platform-specific closures that encode data tables rather than live Go callbacks.

    Recognizes:
    - `(&dyn Any, &mut Window, &mut App) -> bool` -> `dropaccept`
    - `(..., &mut Window, &mut App) -> Option<ExternalDragPayload>` -> `dragpayload`
    - `(&mut Window, &mut App) -> SmallVec<[usize; N]>` -> `prepaintorder`
    - `(&mut A11ySubtreeBuilder) -> ()` -> `a11ychildren`
    """
    tys = [rdj.ty(t) for t in inputs]
    out_s = rdj.ty(output) if output is not None else "()"
    if len(tys) == 3 and _cx_tail(tys) and out_s == "bool":
        dt = (inputs[0].get("borrowed_ref") or {}).get("type", {}).get("dyn_trait")
        if dt and [x["trait"]["path"].split("::")[-1] for x in dt["traits"]] == ["Any"]:
            return ["dropaccept", wrap]
    if len(tys) == 3 and _cx_tail(tys) and out_s.startswith("Option<") and "ExternalDragPayload" in out_s:
        return ["dragpayload", wrap]
    if len(tys) == 2 and _cx_tail(tys) and out_s.replace(" ", "").startswith("SmallVec<[usize;"):
        return ["prepaintorder", wrap]
    if len(tys) == 1 and tys[0].startswith("&mut") and "A11ySubtreeBuilder" in tys[0] and out_s == "()":
        import gk_values
        return ["a11ychildren", wrap, gk_values.access_role(m)]
    return None


def platform_bounded(m, ctx, name):
    """Recognize `impl ImageCacheProvider` bounds as the `imagecache` descriptor."""
    if name == "ImageCacheProvider":
        return ["imagecache"]
    return None


def resolved(m, ctx, t, out):
    """Match platform-level GPUI types and `Arc<...>` wrappers into wire descriptors.

    Handles `ScrollAnchor`, `SharedUri`, `ExternalPaths`, `Entity<impl ImageCache>`,
    `Arc<Path>`, `Arc<Image>`, `Arc<RenderImage>`, and `Arc<dyn Fn(...) -> RenderImage>`.
    """
    import gk_model as g
    rp = t["resolved_path"]
    name, args = g.last(rp["path"]), g.path_args(rp)
    try:
        home = m.api.home(ctx.crate, rp["id"])[0]
    except Exception:
        home = None
    if home == "gpui" and name == "ScrollAnchor" and not args and not out:
        return ["scrollanchor"]
    if home == "gpui" and name == "SharedUri" and not args and not out:
        return ["uri"]
    if home == "gpui" and name == "ExternalPaths" and not args and out:
        return ["list", ["str", "PathBuf"], "extpaths"]
    if name == "Entity" and len(args) == 1 and args[0].get("generic") and not out:
        names = [n for n, _ in g.bound_parts(ctx.bounds.get(args[0]["generic"], []))]
        if "ImageCache" in names:
            return ["imagecacheref"]
    if name == "Arc" and len(args) == 1 and not out:
        a = args[0]
        if "resolved_path" in a:
            an = g.last(a["resolved_path"]["path"])
            if an == "Path":
                return ["arcpath"]
            if an == "Image":
                return ["imagebytes", _image_format(m)]
            if an == "RenderImage":
                return ["pixelimage"]
        dt = a.get("dyn_trait")
        if dt and [x["trait"]["path"].split("::")[-1] for x in dt["traits"]] == ["Fn"] and "RenderImage" in rdj.ty(a):
            return ["customimage"]
    return None


def _image_format(m):
    """Locate gpui's `ImageFormat` unit enum in the rustdoc extract and register it."""
    import gk_model as g
    for cr in g.KIT:
        for pid, p in m.api.crates[cr].paths.items():
            if p["path"][-1] == "ImageFormat" and p["kind"] == "enum" and m.api.home(cr, pid)[0] in g.GPUI_HOMES:
                d = m.desc(g.Ctx(cr), {"resolved_path": {"path": "ImageFormat", "id": int(pid), "args": None}})
                if d[0] == "enum":
                    return d[1]
    raise g.Unsup("norule", "gpui's ImageFormat is not in the extract")


def rust_dec(rs, d):
    """Emit Rust decoder expression for a platform descriptor in `KINDS`."""
    k = d[0]
    if k == "leaf" and d[1] == "drag_move_event":
        return None
    if k not in KINDS:
        return None
    if k == "dropaccept":
        return rs.wrap(d[1], "crate::platform::drop_accept(r)")
    if k == "dragpayload":
        return rs.wrap(d[1], "crate::platform::drag_payload(r)")
    if k == "prepaintorder":
        return rs.wrap(d[1], "{ let v = crate::platform::usizes(r); move |_: &mut gpui_kit::Window, _: &mut gpui_kit::App| "
                             "v.iter().copied().collect() }")
    if k == "a11ychildren":
        return rs.wrap(d[1], "{ let n = r.u32(); let mut v = Vec::with_capacity(n.min(256) as usize); for _ in 0..n { "
                             "let key = r.str().to_string(); let role = %s; let label = r.str().to_string(); "
                             "v.push((key, role, label)); } crate::platform::a11y_children(v) }" % rs.dec(d[2]))
    if k == "imagecache":
        return "crate::platform::cache_provider(r)"
    if k == "imagecacheref":
        return "crate::platform::image_cache_shown(r.u64())"
    if k == "scrollanchor":
        return "crate::platform::anchor_shown(r.u64())"
    if k == "uri":
        return "gpui_kit::SharedUri::from(r.str().to_string())"
    if k == "arcpath":
        return "std::sync::Arc::<std::path::Path>::from(std::path::Path::new(r.str()))"
    if k == "imagebytes":
        return "{ let f = %s; let n = r.u32() as usize; std::sync::Arc::new(gpui_kit::Image::from_bytes(f, r.bytes(n).to_vec())) }" \
            % rs.dec(["enum", d[1]])
    if k == "pixelimage":
        return "crate::platform::pixel_image(r)"
    if k == "customimage":
        return "crate::platform::custom_image(r)"
    raise KeyError(k)


def rust_payload(d, payload):
    """Rewrite a `DragMoveEvent` handler argument to `crate::platform::DragMove::of(a0, cx)`."""
    descs = d[5] if len(d) > 5 and isinstance(d[5], list) else []
    first = descs[0][1] if len(descs) == 1 and descs[0][0] == "ref" else (descs[0] if len(descs) == 1 else None)
    if first is not None and first[:2] == ["leaf", "drag_move_event"]:
        return "&crate::platform::DragMove::of(a0, cx)"
    return None


def turbofish(it):
    """Supply explicit type arguments where Rust cannot infer `crate::values::Drag`."""
    import gk_values
    if it.get("name") == "group_drag_over":
        return f"::<{gk_values.DRAG}>"
    return ""


def call_safe(d):
    """Return True if a platform descriptor can be decoded in immediate calls without `Dx`."""
    return d[0] in ("uri", "arcpath", "imagebytes", "pixelimage", "customimage", "scrollanchor", "imagecacheref")


# Platform descriptors that count as protocol-bridged (rather than simple direct bindings) in reports.
PROTOCOL_KINDS = {"dropaccept", "dragpayload", "prepaintorder", "a11ychildren", "imagecache", "imagecacheref",
                  "scrollanchor", "customimage"}


# Upstream items served by hand-written runtime nodes or calls in core/src/{source,motion,platform}.rs.
PLATFORM_ITEMS = {
    "gpui_kit::base::v_virtual_list":
        "core/src/source.rs node VIRTUAL_LIST (gk.v_virtual_list(id, source, sizes), rt_source.go): a row source's "
        "rows with a size each, drawn through upstream's list; a row without a view yet is its size's placeholder for one frame",
    "gpui_kit::base::h_virtual_list":
        "core/src/source.rs node VIRTUAL_LIST (gk.h_virtual_list(id, source, sizes), rt_source.go)",
    "gpui_kit::base::ElementExt::text_selection_scope":
        "core/src/motion.rs node ANIMATED kind 5 (gk.text_selection_scope(el, scope), rt_motion.go): a type-changing "
        "wrapper over any element",
    "gpui_kit::base::TextSelectionScopeId::new":
        "core/src/platform.rs call TEXT_SCOPE_NEW (gk.text_selection_scope_id(cx), rt_platform.go): a scope id the "
        "library makes and keeps until gk.release",
}
# Generated state methods superseded by richer hand-written runtime calls (stripped from state op tables).
PLATFORM_REPLACES = {
    "gpui_kit::base::GlobalState::set_app_menus":
        "core/src/platform.rs call SET_MENUS (gk.set_menus(cx, menus..), rt_platform.go): every OwnedMenuItem variant "
        "(separator, action with its OS action, checked and disabled flags, submenu, the OS's Services menu)",
    "gpui_kit::base::GlobalState::app_menus":
        "core/src/platform.rs call MENUS_GET (gk.menus_get(cx), rt_platform.go): the menus as the kit's AppMenuBar draws them, every variant",
}


def runtime_items(m):
    """Mark `PLATFORM_ITEMS` as bridged shims and remove `PLATFORM_REPLACES` from generated state methods."""
    for it in m.items:
        if it["id"] in PLATFORM_ITEMS and it["status"] in ("todo", "norule", "skipped"):
            it.update(kind="shim", status="bridged", reason="", detail=PLATFORM_ITEMS[it["id"]])
        if it["id"] in PLATFORM_REPLACES and it["status"] in ("bound", "bridged"):
            it.update(kind="shim", status="bridged", reason="", detail=PLATFORM_REPLACES[it["id"]], args=[])
            it.pop("op", None)
            it.pop("ret", None)
            for st in getattr(m, "states", {}).values():
                for grp in ("reads", "writes", "calls", "statics", "globals", "ctors", "builders"):
                    if it in st.get(grp, []):
                        st[grp].remove(it)


def rust_wrappers(m):
    """Emit motion_scope, which wraps supported element nodes in ElementExt::text_selection_scope."""
    import gk_values
    _, trap = gk_values.animatable(m)
    lines = ["/// Wrap a generated element node with `ElementExt::text_selection_scope` (ANIMATED kind 5).",
             "pub fn motion_scope(dx: &mut Dx, n: u32, scope: gpui_kit::base::TextSelectionScopeId) -> Option<gpui_kit::AnyElement> {",
             "    use gpui_kit::base::ElementExt as _;",
             "    use gpui_kit::IntoElement as _;",
             "    let (ty, args, first) = dx.node(n);",
             "    Some(match ty {"]
    for key, nums in trap:
        lines.append(f"        {' | '.join(map(str, nums))} => make_{m.nodes[key]['fn']}(dx, ty, args, first)?"
                     ".text_selection_scope(scope).into_any_element(),")
    lines += ["        _ => return None,", "    })", "}", ""]
    return lines


