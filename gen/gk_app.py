"""The app family's generator rules (docs/PROTOCOLS.md section 11): actions and key contexts.

Called from gk_model.py at two points, so the rules live here and not in the shared model:

- `action_listener(m, ctx, inputs, output)`, at the start of every closure parameter:
  * a listener generic over the action type, `fn on_action<A: Action>(self, listener: impl
    Fn(&A, &mut Window, &mut App))` (and `capture_action`): A is the binding's one action type
    (`GkAction`), the op takes the action to listen for and the handler (Go
    `gk.Act(action, handler)`, an `ActionHandler`), and the library's listener fires the
    handler for that action only and lets every other one go on (`genapi::action_listener`).
    Descriptor `["action", "listener"]`.
  * a listener over a boxed action, `Fn(&dyn Action, &mut Window, &mut App)`: a handler of the
    runtime's shape ACTION, whose payload is the action's id (`Wire for dyn Action`).
- `bounded(m, ctx, name, targs)`, for one bound of an `impl Trait` or generic argument:
  `TryInto<KeyContext>` (the element's `key_context`) is a string.

Standard library only."""
import json

import rdj

SHAPE_ACTION = 20  # core/wire/ops.txt, the app family's block


def _bound_names(bounds):
    out = []
    for b in bounds or []:
        tb = b.get("trait_bound")
        if tb:
            out.append(tb["trait"]["path"].split("::")[-1])
    return out


def view_closure(m, ctx, inputs, output):
    """`Fn(&mut Window, &mut App) -> AnyView` (a tooltip): the app passes a gk.View, which the
    closure hands back (PROTOCOLS.md 13.18). None for any other closure."""
    if output is None or rdj.ty(output).split("::")[-1] != "AnyView" or len(inputs) != 2:
        return None
    tys = [rdj.ty(t) for t in inputs]
    if not (tys[0].startswith("&mut") and tys[0].endswith("Window") and tys[1].startswith("&mut") and tys[1].endswith("App")):
        return None
    return ["viewany"]


def action_listener(m, ctx, inputs, output):
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


# Kit traits implemented for App whose `&self` methods are ambient reads (`cx.theme()`).
APP_TRAITS = {"ActiveTheme"}


def bounded(m, ctx, name, targs, tr=None):
    import gk_values  # noqa: PLC0415
    pb = platform_bounded(m, ctx, name)
    if pb is not None:
        return pb  # the library's image cache providers (PROTOCOLS.md 13.31)
    vb = gk_values.bounded(m, ctx, name)  # a settings field's element, a trait value (PROTOCOLS.md 13.19)
    if vb is not None:
        return vb
    if name == "TryInto" and len(targs) == 1 and rdj.ty(targs[0]).split("::")[-1] == "KeyContext":
        return ["str", "str"]
    if name == "Deref" and tr is not None:
        cs = ((tr.get("args") or {}).get("angle_bracketed") or {}).get("constraints", [])
        if any(c["name"] == "Target" and (c["binding"].get("equality") or {}).get("type", {}).get("primitive") == "str"
               for c in cs):
            return ["str", "deref"]  # `impl Deref<Target = str>` read back: its text (locale)
    import gk_values  # noqa: PLC0415
    for obj, traits in gk_values.GPUI_OBJECTS.items():
        if name in traits:
            # A generic parameter bounded by a kit trait whose one implementor the binding holds
            # as a library object (`H: ScrollbarHandle` is gpui's ScrollHandle, PROTOCOLS.md 13.14).
            return gk_values.gpui_object(m, obj)
    return None


def bound_only(fn, name):
    """A generic parameter named only as another parameter's associated type (`C: TryInto<K,
    Error = E>`): the caller does not choose it, the argument's type does."""
    import gk_values  # noqa: PLC0415
    if gk_values.fixed_param(fn, name):
        return True  # fixed by the binding: the library's drag value or view (PROTOCOLS.md 13.20)
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


# ---------------------------------------------------------------- ambient calls

# Free functions the library runs itself at start (gpui_kit::init): not callable again.
AMBIENT_SKIP = {"init"}
# Free functions that read state GPUI keeps for the element being drawn (Window::use_keyed_state):
# callable only while a component renders, which app code never does (they panic otherwise).
DRAW_ONLY = {"hover_progress", "is_hover_entering"}


def ambient(m, it, crate, fn, tpath, trait=None, self_t=None):
    """A function of the ambient context (docs/PROTOCOLS.md section 11.5): a free function of the
    kit that builds no element (`amber(500)`, `set_locale`, `try_parse_color`), or a method of a
    trait the kit implements on Window (`WindowExt::close_dialog`). One call op each, run at once
    with the turn's window and app; its result, if any, travels back in the wire type of its
    descriptor. Its arguments are what a call op can carry (no element, no closure but a
    handler-free one). Returns True when the item is bound."""
    from gk_model import Ctx, Unsup  # noqa: PLC0415 (gk_model imports this module)
    import gk_values  # noqa: PLC0415
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
            recv, ins, on = [{"name": "cx", "desc": ["ctx", "app_ref"]}], ins[1:], "App"  # cx.theme()
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
        fctx = m.fn_ctx(Ctx(crate, self_t), fn)  # self_t: a static of a type (gen/gk_values.py)
        args = m.args(fctx, ins)
        for a in args:
            if a["desc"][0] != "ctx" and m.call_bad(a["desc"], False) is not None:
                return False
        ret = None
        if out is not None and rdj.ty(out) != "()":
            ret = m.desc(fctx, out, out=True)
            bad = m.call_bad(ret, False)
            if bad == "leaf" and gk_values.ret_readable(m, ret):
                bad = None  # a record holding a colour, read as the gk.Color the app writes (13.20)
            if bad is not None:
                # A record holding a colour (box_shadow's BoxShadow) cannot be read back: the app's
                # record has a gk.Color (what the app writes), the library writes an Hsla.
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
    """The ambient calls still bound once the model has settled (a call whose type was pruned is not)."""
    return [i for i in getattr(m, "ambient", []) if i["status"] == "bound"]


def number(m, lock, base):
    for it in sorted(bound(m), key=lambda i: i["id"]):
        it["op"] = lock.num("call", "", it["id"], first=base)


def rust_calls(rs):
    """The call-op functions of the ambient calls (gk_rust.Rust.calls); returns the items armed."""
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
            # `impl Deref<Target = str>` (locale) or a borrowed result: written through its Wire impl
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


# ---------------------------------------------------------------- platform family, PROTOCOLS.md 13.31
#
# gpui's element leftovers. Closures upstream calls while it draws or drags become data the app
# gives beforehand (decision 3: app code never runs mid-frame); the library makes and holds the
# image caches and scroll anchors; image sources are bytes; paths dragged in are read back. Each
# descriptor below has its Rust decoder (rust_dec) and its Go type and writer (gen/gk_go_kinds.py);
# the Go types are declared in go/gk/rt_platform.go, the Rust helpers in core/src/platform.rs.
# Hooked from gk_model (closure_desc), gk_values (resolved) and gk_rust (dec, the handler payload,
# trait ops).

KINDS = {"dropaccept", "dragpayload", "prepaintorder", "a11ychildren", "imagecache", "imagecacheref",
         "scrollanchor", "uri", "arcpath", "imagebytes", "pixelimage", "customimage"}


def _cx_tail(tys):
    return (len(tys) >= 2 and tys[-2].startswith("&mut") and tys[-2].endswith("Window")
            and tys[-1].startswith("&mut") and tys[-1].endswith("App"))


def platform_closure(m, ctx, kind, inputs, output, wrap):
    """Closures whose answer the app gives as data (gk_model.Model.closure_desc's hook), or None:

    - `Fn(&dyn Any, &mut Window, &mut App) -> bool` (`can_drop`): which dragged things the element
      takes, `["dropaccept", wrap]` (gk.Drop_Accept: the library's drag values, some of their ids,
      files dragged in from outside);
    - `Fn(&Drag, &mut Window, &mut App) -> Option<ExternalDragPayload>` (`external_drag_payload`,
      its value fixed to the library's Drag): the files handed to the platform when the drag leaves
      the window, `["dragpayload", wrap]` (gk.External_Drag_Payload);
    - `Fn(&mut Window, &mut App) -> SmallVec<[usize; N]>` (`with_dynamic_prepaint_order`): a list
      the library holds, `["prepaintorder", wrap]` ([]int);
    - `FnOnce(&mut A11ySubtreeBuilder)` (`a11y_synthetic_children`): the synthetic accessibility
      nodes as data, `["a11ychildren", wrap, the role's descriptor]` ([]gk.A11y_Child)."""
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
        import gk_values  # noqa: PLC0415
        return ["a11ychildren", wrap, gk_values.access_role(m)]
    return None


def platform_bounded(m, ctx, name):
    """`impl ImageCacheProvider` (Div::image_cache): one of the library's providers,
    `["imagecache"]` (gk.Image_Cache_Provider: a cache the app made with gk.image_cache, or gpui's
    retain-all cache keyed by an id)."""
    if name == "ImageCacheProvider":
        return ["imagecache"]
    return None


def resolved(m, ctx, t, out):
    """gk_values.resolved's hook: a descriptor of this section for a resolved path, or None."""
    import gk_model as g  # noqa: PLC0415
    rp = t["resolved_path"]
    name, args = g.last(rp["path"]), g.path_args(rp)
    try:
        home = m.api.home(ctx.crate, rp["id"])[0]
    except Exception:  # noqa: BLE001 (a path the extract does not hold)
        home = None
    if home == "gpui" and name == "ScrollAnchor" and not args and not out:
        return ["scrollanchor"]  # a scroll anchor the library made (gk.scroll_anchor)
    if home == "gpui" and name == "SharedUri" and not args and not out:
        return ["uri"]
    if home == "gpui" and name == "ExternalPaths" and not args and out:
        return ["list", ["str", "PathBuf"], "extpaths"]  # files dragged in, read back (Wire in platform.rs)
    if name == "Entity" and len(args) == 1 and args[0].get("generic") and not out:
        names = [n for n, _ in g.bound_parts(ctx.bounds.get(args[0]["generic"], []))]
        if "ImageCache" in names:
            return ["imagecacheref"]  # &Entity<I: ImageCache>: a cache the app made (gk.image_cache)
    if name == "Arc" and len(args) == 1 and not out:
        a = args[0]
        if "resolved_path" in a:
            an = g.last(a["resolved_path"]["path"])
            if an == "Path":
                return ["arcpath"]
            if an == "Image":
                return ["imagebytes", _image_format(m)]  # gpui's Image: a format and the encoded bytes
            if an == "RenderImage":
                return ["pixelimage"]
        dt = a.get("dyn_trait")
        if dt and [x["trait"]["path"].split("::")[-1] for x in dt["traits"]] == ["Fn"] and "RenderImage" in rdj.ty(a):
            return ["customimage"]  # ImageSource::Custom: the image is data, not a loader run while drawing
    return None


def _image_format(m):
    """The enum key of gpui's ImageFormat, registered through the kit's path tables."""
    import gk_model as g  # noqa: PLC0415
    for cr in g.KIT:
        for pid, p in m.api.crates[cr].paths.items():
            if p["path"][-1] == "ImageFormat" and p["kind"] == "enum" and m.api.home(cr, pid)[0] in g.GPUI_HOMES:
                d = m.desc(g.Ctx(cr), {"resolved_path": {"path": "ImageFormat", "id": int(pid), "args": None}})
                if d[0] == "enum":
                    return d[1]
    raise g.Unsup("norule", "gpui's ImageFormat is not in the extract")


def rust_dec(rs, d):
    """The Rust expression that reads a value of one of this section's descriptors, or None."""
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
        return "crate::platform::image_cache_shown(r.u64())"  # lent by the call (`&a0`)
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
    """gk_rust.Rust.handler's hook: the payload expression of a handler whose event the library
    completes from the app context, or None. `DragMoveEvent<Drag>`: the dragged value is read with
    `drag(cx)`, so the payload is a copy made before the handler fires (core/src/platform.rs)."""
    descs = d[5] if len(d) > 5 and isinstance(d[5], list) else []
    first = descs[0][1] if len(descs) == 1 and descs[0][0] == "ref" else (descs[0] if len(descs) == 1 else None)
    if first is not None and first[:2] == ["leaf", "drag_move_event"]:
        return "&crate::platform::DragMove::of(a0, cx)"
    return None


def turbofish(it):
    """gk_rust's trait ops: a generic parameter the binding fixes and no argument names gets it
    written out (`group_drag_over::<crate::values::Drag>`, PROTOCOLS.md 13.31)."""
    import gk_values  # noqa: PLC0415
    if it.get("name") == "group_drag_over":
        return f"::<{gk_values.DRAG}>"
    return ""


def call_safe(d):
    """Descriptors of this section a call op can carry (no view build): the image sources' bytes and
    paths (an avatar's source set from code)."""
    return d[0] in ("uri", "arcpath", "imagebytes", "pixelimage", "customimage", "scrollanchor", "imagecacheref")


# Descriptors of this section that run the library's protocol (counted bridged, not bound): data a
# closure answers with, objects the library holds.
PROTOCOL_KINDS = {"dropaccept", "dragpayload", "prepaintorder", "a11ychildren", "imagecache", "imagecacheref",
                  "scrollanchor", "customimage"}


# Upstream functions the platform runtime calls for the app where no generated call fits (counted
# bridged, as gk_values.MOTION_ITEMS): the item, where.
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
# Generated bindings the runtime's own call replaces because it reaches every variant the generated
# one cannot (a menu item's Submenu and Action: a recursive record and a boxed action): counted
# bridged through the runtime, the generated call left out.
PLATFORM_REPLACES = {
    "gpui_kit::base::GlobalState::set_app_menus":
        "core/src/platform.rs call SET_MENUS (gk.set_menus(cx, menus..), rt_platform.go): every OwnedMenuItem variant "
        "(separator, action with its OS action, checked and disabled flags, submenu, the OS's Services menu)",
    "gpui_kit::base::GlobalState::app_menus":
        "core/src/platform.rs call MENUS_GET (gk.menus_get(cx), rt_platform.go): the menus as the kit's AppMenuBar draws them, every variant",
}


def runtime_items(m):
    """gk_values.runtime_items's hook: PLATFORM_ITEMS counted bridged."""
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
    """`generated::motion_scope` (core/src/motion.rs, node ANIMATED kind 5): ElementExt's
    text_selection_scope over an element node, one arm per element type that is a parent (the
    elements focus_trap takes, gk.Focus_Trap_Element): the rule of a type-changing wrapper
    instantiated per element type (PROTOCOLS.md 13.30)."""
    import gk_values  # noqa: PLC0415
    _, trap = gk_values.animatable(m)
    lines = ["/// ElementExt::text_selection_scope over an element node (core/src/motion.rs, node ANIMATED kind 5).",
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


# Sequence::with_step (a Transition node in a constructor's builder chain) takes the editor family's rule
# of 9.31 (gk_editor.chain_cx), which binds every family's builders.
