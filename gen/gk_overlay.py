"""Overlay, menu, dock, panel, and value-read model extensions.

Extends `gen/gk_model.py` and `gen/gk_rust.py` with the rules needed for overlays
(dialogs, sheets, popovers, tooltips, context/dropdown menus), dock panels and layout
trees, and plot/value readbacks:

1. **Concrete wrapper instantiations and blanket traits (`after_scan`, `after_classify`, `instance_tables`)**:
   - Instantiates `ContextMenu<Div>` (`ContextMenuDiv`) and attaches `ContextMenuExt`
     onto `Div` because rustdoc does not expand blanket extension impls onto concrete nodes.
   - Copies non-`Element` builder traits from the wrapped inner node (`Div`) onto each
     instantiated wrapper so `ContextMenuDiv` inherits styling/layout builders.
2. **Described values and `ViewBuild` closures (`described_values`, `view_built`, `rust_dec`)**:
   - Upstream overlay APIs (`open_sheet`, `context_menu`, `tooltip`, etc.) pass a
     pre-constructed value (`Sheet`, `PopupMenu`, `ResizablePanelGroup`, `Tooltip`, `Icon`)
     into a closure `Fn(T, &mut Window, &mut App) -> T`.
   - Listing those types under `[node].types` in `api/overrides.toml` forces them into
     `m.nodes` with a synthetic `(described)` constructor so Go records builder ops on a
     standalone value tape, and `view_built()` upgrades `build` descriptors inside call
     ops to `viewbuild` (decoded via `crate::overlay::ViewBuild::take` and replayed with
     `apply_<fn>`).
3. **Dock and panel protocol (`panel_entity`, `panel_view`, `resolved`, `resolved4`, `rust_dock_*`)**:
   - Maps `Entity<P: Panel>` to `Entity<crate::dock::GkPanel>`, `Arc<dyn PanelView>` to
     `panelview`, `NodeId`/`PanelId`/`AnyView`/`DockAreaState`/`PanelState`/`PanelInfo`/
     `Instant`/`Rc<dyn DockAreaRenderer|TabGroupRenderer>` to `dock` descriptors, and
     plot readbacks (`ArcData`, `StackSeries`) to `libval` descriptors.
   - Marks `HANDED` runtime-owned states (`TabGroup`, `AppMenuBar`) and `LIBRARY_HANDOUTS`
     (`DockContext`, `TabGroupContext`) reachable, and registers `LIBRARY_CALLS` /
     `LIBRARY_CALLS4` / `LIBRARY_NODES` shims in `core/src/dock.rs` and `core/src/overlay.rs`.
4. **Fallible builders, observed closures, and skip tables (`result_desc`, `fallible_callee`, `SKIPS5`)**:
   - Maps outbound `Result<(), E>` to `["result", "unit"]` (`crate::overlay::outcome`),
     wraps `Result<Self, E>` builders in `crate::overlay::Fallible`, and merges grouped
     skip explanations (`SKIPS5`) for Rust-only plot paint hooks and dock internals.
"""
import json
import re

import rdj

# Generic overlay elements instantiated over concrete inner types during after_scan().
OVERLAY_INSTANCES = [
    ("gpui_kit::component::menu::ContextMenu", "gpui_kit::Div", "ContextMenuDiv"),
]
# Blanket extension traits attached explicitly onto concrete nodes whose rustdoc does not list them.
BLANKET_TABLES = [("ContextMenuExt", "gpui_kit::Div")]
# Kit methods whose return type is private and replaced by a dedicated runtime node opcode.
LIBRARY_NODES = {
    "gpui_kit::component::menu::DropdownMenu::dropdown_menu":
        "core/src/overlay.rs, node DROPDOWN_BUTTON (gk.dropdown_menu): the kit's DropdownMenuPopover<Button> has no public path",
}
# PopupMenu methods requiring a `Context<PopupMenu>` that are bridged through runtime helpers
# because `PopupMenu` is built as a described value without exposing a `Context<PopupMenu>` to Go.
CONTEXT_OPS = {("PopupMenu", "submenu"): "crate::overlay::submenu",
               ("PopupMenu", "submenu_with_icon"): "crate::views_menu::submenu_with_icon"}


def _find(m, path):
    """Locate `(crate, item_id)` for a canonical public type path across all loaded crates."""
    for cr, w in m.api.walks.items():
        for iid in w.types:
            if m.api.pubpath(cr, iid) == path:
                return cr, iid
    return None


def after_scan(m):
    """Merge module skip rules and instantiate concrete overlay wrapper nodes (`ContextMenu<Div>`)."""
    merge_skips(m)
    for base_path, arg_path, name in OVERLAY_INSTANCES:
        base, arg = _find(m, base_path), _find(m, arg_path)
        if base is None or arg is None:
            raise SystemExit(f"gk_overlay: no type {base_path if base is None else arg_path}")
        f = m.info(*base)
        if len(f["generic"]) != 1:
            raise SystemExit(f"gk_overlay: {base_path} is not generic over one type")
        aname = arg_path.rsplit("::", 1)[-1]
        m.add_node(base[0], base[1], f, f"{base_path}<{arg_path}>", name,
                   {f["generic"][0]: (arg[0], {"resolved_path": {"path": aname, "id": int(arg[1]), "args": None}})},
                   inst=arg)


def described_values(m):
    """Return the set of `[node].types` names forced into the node table as described values."""
    return set(m.ov.get("node", {}).get("types", []))


def after_classify(m):
    """Attach blanket traits, panel/dock shims, handed states, and synthetic described-value ctors.

    Described values (`Sheet`, `PopupMenu`, `ResizablePanelGroup`, `Tooltip`, `Icon`) are
    constructed on the Rust side by upstream closures rather than directly by user code;
    synthesizing a placeholder `(described)` ctor lets Go build op chains for `ViewBuild`
    closures that replay ops onto the upstream-supplied value.
    """
    for tname, npath in BLANKET_TABLES:
        node = m.nodes.get(npath)
        hit = [(cr, tid) for cr, w in m.api.walks.items() for tid, (_, t) in w.traits.items() if t["name"] == tname]
        if node is None or len(hit) != 1:
            raise SystemExit(f"gk_overlay: blanket table {tname} for {npath}: not found")
        if not any((tc, tid) == hit[0] for tc, tid, _ in node["traits"]):
            node["traits"].append((hit[0][0], hit[0][1], tname))
    instance_tables(m)
    mark_panel_traits(m)
    mark_handed(m)
    after_classify4(m)
    for it in m.items:
        if it["id"] in LIBRARY_CALLS and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="", detail="core/src/dock.rs: " + LIBRARY_CALLS[it["id"]])
    # Strip methods overridden by hand-written dock calls from auto-generated state/object tables.
    for st in list(m.objects.values()) + list(m.states.values()):
        for g in ("builders", "reads", "writes", "calls", "statics", "ctors"):
            if g in st:
                st[g] = [i for i in st[g] if i["id"] not in LIBRARY_CALLS]
    for it in m.items:
        if it["id"] in LIBRARY_NODES:
            it.update(kind="shim", status="bridged", reason="", detail=LIBRARY_NODES[it["id"]])
    for key, node in m.nodes.items():
        if node["name"] not in described_values(m) or node["element"]:
            continue
        if not any(i["status"] in ("bound", "bridged") for i in node["ctors"]):
            node["ctors"].append({"row": None, "owner": node["path"], "name": "new", "trait": None,
                                  "sig": "(described value)", "id": node["path"] + "::(described)", "kind": "ctor",
                                  "status": "bound", "reason": "", "args": [], "crate": node["crate"],
                                  "node": key, "callee": "crate::overlay::described::<%s>" % node["path"],
                                  "synthetic": True, "described": True})
        for it in list(m.items):
            if it.get("owner") != node["path"] or (node["name"], it["name"]) not in CONTEXT_OPS:
                continue
            _context_op(m, node, it, CONTEXT_OPS[(node["name"], it["name"])])


def instance_tables(m):
    """Copy non-Element builder traits from the wrapped inner node onto each `OVERLAY_INSTANCES` wrapper."""
    for base_path, arg_path, name in OVERLAY_INSTANCES:
        node = m.nodes.get(f"{base_path}<{arg_path}>")
        arg = m.nodes.get(arg_path)
        if node is None or arg is None:
            continue
        f = m.info(node["crate"], node["id"])
        have = {(tc, tid) for tc, tid, _ in node["traits"]}
        for tc, tid, tn in arg["traits"]:
            if tn in f["traits"] and tn not in ("Element", "IntoElement") and (tc, tid) not in have:
                node["traits"].append((tc, tid, tn))


def _context_op(m, node, it, callee):
    """Bind a `CONTEXT_OPS` method on a described value by lowering `&mut Context<Self>` to `["ctx", "app"]`."""
    from gk_model import Ctx, Unsup
    c = m.api.crates[node["crate"]]
    f = next((c.item(i) for im in m.info(node["crate"], node["id"])["impls"] if im.get("trait") is None
              for i in im["items"] if c.item(i) and c.item(i)["name"] == it["name"]), None)
    if f is None:
        return
    fn = f["inner"]["function"]
    self_t = (node["crate"], {"resolved_path": {"path": node["name"], "id": int(node["id"]), "args": None}})
    fctx = m.fn_ctx(Ctx(node["crate"], self_t), fn)
    args = []
    try:
        for n, (pn, pt) in enumerate(fn["sig"]["inputs"][1:]):
            s = rdj.ty(pt)
            if re.match(r"&mut (gpui::)?Context\b", s):
                args.append({"name": "cx", "desc": ["ctx", "app"]})
                continue
            try:
                args += m.args(fctx, [(pn, pt)])
            except Unsup:
                one = _single_into(m, s)
                if one is None:
                    raise
                args.append({"name": pn, "desc": one})
    except Unsup as e:
        return m.set(it, e)
    it.update(kind="op", status="bridged", reason="", callee=callee, args=args, detail="")
    if it not in node["ops"]:
        node["ops"].append(it)


def _single_into(m, s):
    """Fallback for single-member `Into<T>` wrapper types (optionally wrapped in `Option<...>`)."""
    mt = re.fullmatch(r"(Option<)?(\w+)(>)?", s)
    if mt is None or (mt.group(1) is None) != (mt.group(3) is None):
        return None
    hit = [x for k, x in m.intos.items() if k.rsplit("::", 1)[-1] == mt.group(2) and x.get("single")]
    if len(hit) != 1:
        return None
    return ["opt", hit[0]["single"]] if mt.group(1) else hit[0]["single"]


def context_build(params):
    """Return True if a builder closure takes `(&mut Window, &mut Context<T>)` instead of `&mut App`."""
    return "window" in params and "context" in params and "app" not in params


def view_built(m, x, builder, seen):
    """Check whether descriptor `x` is call-safe via overlay rules, upgrading `build` to `viewbuild`.

    Inside immediate calls (like `WindowExt::open_sheet`), a `build` closure that takes a
    window cannot use a render-frame `Dx`; rewriting its tag in place to `viewbuild`
    routes it through `crate::overlay::ViewBuild::take` instead.
    """
    if x[0] == "panelview" and not builder:
        return True
    if x[0] in ("field", "format", "data"):
        return True
    if call_safe(x):
        return True
    if x[0] not in ("build", "viewbuild") or builder or seen:
        return False
    if "window" not in x[2]:
        return False
    x[0] = "viewbuild"
    return True


def ctor_nums(m, node_key):
    """Return sorted constructor opcodes for `node_key` used by `ViewBuild::take` to validate tape roots."""
    return sorted(i["op"] for i in m.nodes[node_key]["ctors"] if i["status"] in ("bound", "bridged") and "op" in i)


def rust_dec(rs, d):
    """Emit Rust decoder for `viewbuild`: takes a tape handle and replays `apply_<fn>` onto upstream `v`."""
    node = rs.m.nodes[d[1]]
    nums = ", ".join(map(str, ctor_nums(rs.m, d[1]))) or "0"
    ps = rs.closure_params(d[2], d[4], "v")
    return rs.wrap(d[3], '{ let vb = crate::overlay::ViewBuild::take(r.u64(), c, &[%s], "%s")?; move |%s| vb.apply(v, window, cx, apply_%s) }'
                   % (nums, node["name"], ps, node["fn"]))


def result_desc(m, ctx, args, out):
    """Classify outbound `Result<(), E>` as `["result", "unit"]` (other `Result<T, E>` handled by `gk_values`)."""
    from gk_model import Unsup
    if not out or len(args) != 2:
        raise Unsup("norule", "std type core::result::Result")
    ok = args[0]
    if not ("tuple" in ok and not ok["tuple"]):
        raise Unsup("todo:protocol: calls (a result other than Result<(), E>)", rdj.ty(ok))
    return ["result", "unit"]


def rust_put_ret(expr):
    """Emit wire serialization for `Result<(), E>` via `crate::overlay::outcome`."""
    return f"genapi::Wire::put(&crate::overlay::outcome({expr}), &mut out)"


def fallible_callee(callee, n_args):
    """Wrap a builder method returning `Result<Self, E>` so errors are logged and the receiver stays intact."""
    return f"crate::overlay::Fallible({callee}).call{n_args}"


def is_result_self(out_s, name):
    """Return True if return type text `out_s` matches `Result<Self, E>` or `Result<Name, E>`."""
    s = out_s.replace(" ", "")
    return bool(re.fullmatch(rf"Result<(Self|{name}),\w+>", s))


def read_needs_window(ins):
    """Return True if any non-self parameter in `ins` borrows `gpui::Window`."""
    return any(re.match(r"&(mut )?(gpui::)?Window\b", rdj.ty(t)) for _, t in ins[1:])


def describe(m):
    """Serialize overlay instantiation and described-value lists for model inspection."""
    return json.dumps({"instances": [n for _, _, n in OVERLAY_INSTANCES], "described": sorted(described_values(m))})


# Dock panel shim type and upstream trait prefixes bridged by core/src/dock.rs GkPanel.
PANEL_RUST = "crate::dock::GkPanel"
PANEL_NAME = "DockPanel"
PANEL_TRAITS = ("gpui_kit::base::dock::Panel::", "gpui_kit::base::dock::PanelView::",
                "gpui_kit::component::dock::Panel::", "gpui_kit::component::dock::PanelView::")


def _bound_names(bounds):
    """Extract simple trait names from a list of rustdoc GenericBound dicts."""
    out = []
    for b in bounds or []:
        tb = b.get("trait_bound")
        if tb:
            out.append(tb["trait"]["path"].split("::")[-1])
    return out


def panel_entity(m, ctx, a):
    """Resolve `Entity<P>` where `P: Panel` to `Entity<crate::dock::GkPanel>`."""
    g = a.get("generic")
    if g and g in ctx.bounds and "Panel" in _bound_names(ctx.bounds[g]):
        return ["entity", PANEL_RUST, PANEL_NAME]
    return None


def panel_view(m, ctx, name, a, out):
    """Match input `Arc<dyn PanelView>` parameters for both `gpui_base` and `gpui_component`."""
    if out or name != "Arc" or "dyn_trait" not in a:
        return None
    trs = a["dyn_trait"]["traits"]
    if [x["trait"]["path"].split("::")[-1] for x in trs] != ["PanelView"]:
        return None
    hc, _ = m.api.home(ctx.crate, trs[0]["trait"]["id"])
    return ["panelview", "kit" if hc == "gpui_component" else "base"]


def rust_panel_view(rs, d):
    """Emit Rust decoder for `panelview`, choosing checked (`?`) vs tape-shown lookup."""
    f = "kit_panel_view" if d[1] == "kit" else "panel_view"
    return f"crate::dock::{f}(r.u64())?" if rs.call else f"crate::dock::{f}_shown(r.u64())"


def mark_panel_traits(m):
    """Mark all `Panel` and `PanelView` trait methods as bridged via `core/src/dock.rs GkPanel`."""
    for it in m.items:
        if it["id"].startswith(PANEL_TRAITS) and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="",
                      detail="core/src/dock.rs GkPanel: from the data the app pushed (gk.panel_set), or upstream's "
                             "PanelView over Entity<GkPanel>; set_active, set_zoomed, on_added_to, on_removed are "
                             "Panel_Event handler calls")


# Entity state types handed out by the runtime (no public constructor needed) and Default-constructed states.
HANDED = ("gpui_kit::base::dock::TabGroup", "gpui_kit::component::menu::AppMenuBar")
DEFAULT_CTORS = ("gpui_kit::base::ResizableState",)


def _rp_home(m, ctx, t):
    """Resolve a rustdoc `resolved_path` type node to `(home_crate, home_id, item)` or `(None, None, None)`."""
    rp = t["resolved_path"]
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:
        return None, None, None
    return hc, hid, it


def resolved(m, ctx, t, out):
    """Classify dock, panel, layout-state, `Instant`, and plot readback types.

    Checks `resolved4()` first for `Instant`, `ArcData`/`StackSeries` plot readbacks, and
    `Rc<dyn DockAreaRenderer|TabGroupRenderer>`, then maps `gpui_base` layout structs,
    `NodeId`/`PanelId`, `gpui::AnyView`, outbound `Duration` (encoded as `dock.ms`),
    outbound `Arc<dyn PanelView>`, and outbound `Entity<State>` handles whose state is
    constructible or runtime-handed (`HANDED`).
    """
    rp = t["resolved_path"]
    name = rp["path"].split("::")[-1]
    if name not in ("NodeId", "PanelId", "AnyView", "Duration", "Arc", "Entity", "Rc") and name not in LAYOUT_VALUES \
            and name not in ROUND4_NAMES:
        return None
    r4 = resolved4(m, ctx, t, out, name)
    if r4 is not None:
        return r4
    if name == "Rc":
        return None
    hc, hid, it = _rp_home(m, ctx, t)
    if name in LAYOUT_VALUES and hc == "gpui_base":
        return ["dock", LAYOUT_VALUES[name]]
    if name in ("NodeId", "PanelId") and hc == "gpui_base":
        return ["dock", "node_id" if name == "NodeId" else "panel_id"]
    if name == "AnyView" and hc == "gpui":
        return ["dock", "view"]
    if name == "Duration" and out and hc not in m.api.crates:
        return ["dock", "ms"]
    args = (rp.get("args") or {}).get("angle_bracketed", {}).get("args", [])
    args = [a["type"] for a in args if "type" in a]
    if name == "Arc" and out and len(args) == 1 and "dyn_trait" in args[0]:
        trs = args[0]["dyn_trait"]["traits"]
        if [x["trait"]["path"].split("::")[-1] for x in trs] == ["PanelView"]:
            pc, _ = m.api.home(ctx.crate, trs[0]["trait"]["id"])
            return ["dock", "panel", "kit" if pc == "gpui_component" else "base"]
    if name == "Entity" and out and len(args) == 1 and "resolved_path" in args[0]:
        a = args[0]
        ahc, ahid, _ = _rp_home(m, ctx, a)
        if ahc in m.api.crates:
            path = m.api.pubpath(ahc, ahid)
            st = m.states.get(path)
            if st is not None and (st.get("ctors") or st.get("handed") or path in HANDED):
                return ["dock", "entity", m.rtype(ctx, a), path]
    return None


LAYOUT_VALUES = {"DockAreaState": "area_state", "PanelState": "panel_state", "PanelInfo": "panel_info"}
LAYOUT_RUST = {"area_state": "gpui_kit::base::dock::DockAreaState", "panel_state": "gpui_kit::base::dock::PanelState",
               "panel_info": "gpui_kit::base::dock::PanelInfo"}

LIBRARY_CALLS = {
    "gpui_kit::base::dock::register_panel": "gk.register_panel (call PANEL_REGISTER): a request to the app per panel",
    "gpui_kit::base::dock::PanelRegistry::build_panel": "gk.build_panel (call PANEL_BUILD_NOW)",
    "gpui_kit::base::dock::PanelBuildContext::new": "gk.build_panel makes it from the dock, the state and its info",
    "gpui_kit::base::dock::PanelBuildContext::dock_area": "the Panel_Build request's dock",
    "gpui_kit::base::dock::PanelBuildContext::state": "the Panel_Build request's state",
    "gpui_kit::base::dock::PanelBuildContext::info": "the Panel_Build request's state.info",
    "gpui_kit::base::dock::PanelSource::dump": "crate::dock::GkSource over a dock's panels (gk.pane_tree_to_state)",
    "gpui_kit::base::dock::PanelSource::is_visible": "crate::dock::GkSource over a dock's panels (gk.pane_tree_to_state)",
    "gpui_kit::base::dock::PanelSource::panel_name": "crate::dock::GkSource over a dock's panels (gk.pane_tree_to_state)",
    "gpui_kit::base::dock::PanelBuilder::build": "crate::dock::GkBuilder through the registry (gk.pane_tree_from_state)",
    "gpui_kit::base::dock::PaneTree::to_state": "gk.pane_tree_to_state (call PANE_TREE_TO_STATE)",
    "gpui_kit::base::dock::PaneTree::from_state": "gk.pane_tree_from_state (call PANE_TREE_FROM_STATE)",
    "gpui_kit::base::dock::PaneNode::kind": "gk.pane_node_kind (call PANE_NODE_KIND): PaneRef as a value, children as handles",
    "gpui_kit::base::dock::PaneNode::walk": "gk.pane_node_walk (call PANE_NODE_WALK): the nodes the closure would see, in order",
    "gpui_kit::component::menu::AppMenuBar::new": "gk.app_menu_bar (call APP_MENU_BAR_NEW), drawn by gk.app_menu_bar_view",
    "gpui_kit::component::kbd::Kbd::binding_for_action": "node KBD_FOR_ACTION (gk.kbd_for_action): nothing drawn when unbound",
    "gpui_kit::component::kbd::Kbd::binding_for_action_in": "node KBD_FOR_ACTION (gk.kbd_for_action_in)",
    "gpui_kit::component::kbd::Kbd::global_binding_for_action": "node KBD_FOR_ACTION (gk.kbd_global_for_action)",
    "gpui_kit::component::dock::DockSkin::dock_area": "gk.dock_area_skinned (call DOCK_AREA_SKINNED): the area's handle, its skin kept beside it",
    "gpui_kit::component::dock::DockSkin::panel_style": "gk.dock_skin_panel_style (call DOCK_SKIN)",
    "gpui_kit::component::dock::DockSkin::set_panel_style": "gk.dock_skin_set_panel_style (call DOCK_SKIN, buffered)",
    "gpui_kit::component::dock::DockSkin::is_toggle_button_visible": "gk.dock_skin_toggle_button_visible (call DOCK_SKIN)",
    "gpui_kit::component::dock::DockSkin::set_toggle_button_visible": "gk.dock_skin_set_toggle_button_visible (call DOCK_SKIN, buffered)",
    "gpui_kit::component::dock::DockSkin::set_close_button_visible": "gk.dock_skin_set_close_button_visible (call DOCK_SKIN, buffered)",
    "gpui_kit::component::dock::DockSkin::is_collapsed_tab_bar_visible": "gk.dock_skin_collapsed_tab_bar_visible (call DOCK_SKIN)",
    "gpui_kit::component::dock::DockSkin::set_collapsed_tab_bar_visible": "gk.dock_skin_set_collapsed_tab_bar_visible (call DOCK_SKIN, buffered)",
    "gpui_kit::component::dock::DockSkin::is_split_divider_line_visible": "gk.dock_skin_split_divider_line_visible (call DOCK_SKIN)",
    "gpui_kit::component::dock::DockSkin::set_split_divider_line_visible": "gk.dock_skin_set_split_divider_line_visible (call DOCK_SKIN, buffered)",
    "gpui_kit::component::menu::DropdownMenu::dropdown_menu_with_anchor":
        "node DROPDOWN_ANCHOR (gk.dropdown_menu_anchored): DropdownMenuPopover<Button> has no public path",
}


def call_safe(x):
    """Return True if descriptor `x` is a `dock` descriptor legal inside immediate call ops."""
    return x[0] == "dock"


def mark_handed(m):
    """Mark `HANDED` state types as runtime-supplied and attach synthetic `Default` ctors to `DEFAULT_CTORS`."""
    for path in HANDED:
        st = m.states.get(path)
        if st is None:
            raise SystemExit(f"gk_overlay: no state type {path}")
        st["handed"] = True
    for path in DEFAULT_CTORS:
        st = m.states.get(path)
        if st is None:
            raise SystemExit(f"gk_overlay: no state type {path}")
        if any(i["status"] in ("bound", "bridged") for i in st["ctors"]):
            continue
        st["ctors"].append({"row": None, "owner": path, "name": "new", "trait": None, "sig": "(Default)",
                            "id": path + "::new (Default)", "kind": "state-ctor", "status": "bridged", "reason": "",
                            "args": [], "crate": st["crate"], "callee": f"<{path} as Default>::default",
                            "synthetic": True, "detail": "impl Default: T::default() (PROTOCOLS.md 11.8)"})


def rust_dock_dec(rs, d):
    """Emit Rust decoder expression for a `dock` descriptor (`node_id`, `panel_id`, layout state, `view`, `renderer`, `instant`)."""
    w = d[1]
    inner = rs.call in ("builder", "codec")
    if w == "node_id":
        if inner:
            return "crate::dock::node_id(r.u64()).ok()?"
        return "crate::dock::node_id(r.u64())?" if rs.call else "crate::dock::node_id_shown(r.u64())"
    if w == "panel_id":
        return "gpui_kit::base::dock::PanelId::from_u64(r.u64())"
    if w in LAYOUT_RUST:
        return f"<{LAYOUT_RUST[w]} as genapi::Unwire>::get(r)"
    if w in ROUND4_DEC:
        return ROUND4_DEC[w](rs, d, inner)
    if w == "view":
        if inner:
            return "crate::dock::any_view_c(r.u64()).ok()?"
        return "crate::dock::any_view(r.u64(), c)?" if rs.call else "crate::dock::any_view_shown(r.u64())"
    raise KeyError(f"dock {w} is only read back")


def _put_one(d, expr):
    """Emit wire serialization for a single outbound `dock` value (`entity`, `ms`, `instant`, or `Out`)."""
    w = d[1]
    if w in ROUND4_PUT:
        return ROUND4_PUT[w](expr)
    if w == "entity":
        return f"crate::dock::put_entity(&{expr}, &mut out)"
    if w == "ms":
        return f"crate::dock::put_ms(&{expr}, &mut out)"
    return f"genapi::Wire::put(&crate::dock::Out({expr}), &mut out)"


def rust_dock_put(ret, expr):
    """Emit wire serialization for direct, optional, list, or borrowed `dock` return descriptors."""
    if not ret:
        return None
    if ret[0] == "dock":
        return _put_one(ret, expr)
    if ret[0] in ("opt", "list", "ref") and ret[1][0] == "dock":
        w = ret[1][1]
        if ret[0] == "ref":
            return _put_one(ret[1], f"*{expr}") if w in ("ms",) else _put_one(ret[1], expr)
        if w in ("entity", "ms"):
            f = "put_entity" if w == "entity" else "put_ms"
            return f"crate::dock::{ret[0]}_with(&{expr}, &mut out, crate::dock::{f})"
        return f"genapi::Wire::put(&crate::dock::Out({expr}), &mut out)"
    if ret[0] in ("opt", "list") and ret[1][0] == "ref" and ret[1][1][0] == "dock":
        return f"genapi::Wire::put(&crate::dock::Out({expr}), &mut out)"
    return None


# Additional type names handled in resolved4(): std::time::Instant and plot shape readbacks.
ROUND4_NAMES = ("Instant", "ArcData", "StackSeries")

RENDERER_TRAITS = ("DockAreaRenderer", "TabGroupRenderer")
RENDERER_ITEMS = ("gpui_kit::base::dock::DockAreaRenderer::", "gpui_kit::base::dock::TabGroupRenderer::")

# Library objects handed to Go by core/src/dock.rs render hooks (synthetic handouts keep them reachable).
LIBRARY_HANDOUTS = {
    "gpui_kit::base::dock::DockContext": "the dock hook's Dock_Render request (crate::dock::GkRenderer::render_dock)",
    "gpui_kit::base::dock::TabGroupContext": "the tab group hooks' Dock_Render request (crate::dock::GkRenderer)",
}


def resolved4(m, ctx, t, out, name):
    """Classify `Instant`, plot readbacks (`ArcData`, `StackSeries`), and `Rc<dyn DockAreaRenderer|TabGroupRenderer>`."""
    rp = t["resolved_path"]
    if name == "Instant":
        hc, _, _ = _rp_home(m, ctx, t)
        if hc not in m.api.crates:
            return ["dock", "instant"]
        return None
    if name in LIBRARY_READS and out:
        return ["libval", LIBRARY_READS[name][0], LIBRARY_READS[name][1]]
    if name == "Rc" and not out:
        args = (rp.get("args") or {}).get("angle_bracketed", {}).get("args", [])
        args = [a["type"] for a in args if "type" in a]
        if len(args) == 1 and "dyn_trait" in args[0]:
            trs = args[0]["dyn_trait"]["traits"]
            if [x["trait"]["path"].split("::")[-1] for x in trs] in ([n] for n in RENDERER_TRAITS):
                return ["dock", "renderer"]
    return None


LIBRARY_READS = {
    "ArcData": ("gpui_kit::base::plot::shape::ArcData<'_, crate::data::Datum>", "Arc_Data"),
    "StackSeries": ("gpui_kit::base::plot::shape::StackSeries<crate::data::Datum>", "Stack_Series"),
}


def _dec_renderer(rs, d, inner):
    if inner:
        return "crate::dock::renderer(r.u64()).ok()?"
    return "crate::dock::renderer(r.u64())?" if rs.call else "crate::dock::renderer_shown(r.u64())"


def _dec_instant(rs, d, inner):
    return "crate::overlay::instant(r.i64())"


ROUND4_DEC = {"renderer": _dec_renderer, "instant": _dec_instant}
ROUND4_PUT = {"instant": lambda expr: f"crate::overlay::put_instant(&{expr}, &mut out)"}


LIBRARY_CALLS4 = {
    "gpui_kit::component::dock::DockToggleEvent::placement": "gk.on_dock_toggle (call DOCK_TOGGLE_SUBSCRIBE): Dock_Toggle.placement",
    "gpui_kit::component::dock::DockToggleEvent::is_open": "gk.on_dock_toggle (call DOCK_TOGGLE_SUBSCRIBE): Dock_Toggle.open",
}


def after_classify4(m):
    """Bridge dock toggle events and renderer traits, and add synthetic handouts for dock render contexts.

    Synthetic `(library hand-out)` items return `["obj", path, path, True]` for `DockContext`
    and `TabGroupContext` so `Model.reach_objects()` treats both objects as reachable even
    though no generated constructor creates them (they are handed out at render time by
    `core/src/dock.rs GkRenderer`).
    """
    for it in m.items:
        if it["id"] in LIBRARY_CALLS4:
            it.update(kind="shim", status="bridged", reason="", detail="core/src/dock.rs: " + LIBRARY_CALLS4[it["id"]])
    for st in list(m.objects.values()) + list(m.states.values()):
        for g in ("builders", "reads", "writes", "calls", "statics", "ctors"):
            if g in st:
                st[g] = [i for i in st[g] if i["id"] not in LIBRARY_CALLS4]
    for it in m.items:
        if it["id"].startswith(RENDERER_ITEMS) and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="",
                      detail="core/src/dock.rs GkRenderer: answered from the app views given per hook and key "
                             "(gk.dock_render_answer), asked by a queued Dock_Render request (PROTOCOLS.md 11.9)")
    for path, where in LIBRARY_HANDOUTS.items():
        if path not in m.objects:
            raise SystemExit(f"gk_overlay: {path} is not a library object")
        st = m.objects[path]
        m.items.append({"row": None, "owner": None, "name": "(library hand-out)", "trait": None, "sig": "(library)",
                        "id": path + "::(library hand-out)", "kind": "shim", "status": "bridged", "reason": "",
                        "args": [], "crate": st.get("crate"), "synthetic": True, "detail": where,
                        "ret": ["obj", path, path, True]})


def named_text(fn, text):
    """Append trait bounds referenced by generic names appearing in `text` (two hops for nested bounds)."""
    for _ in range(2):
        for w in fn["generics"].get("where_predicates", []):
            bp = w.get("bound_predicate")
            if bp and "generic" in bp["type"] and re.search(rf"\b{bp['type']['generic']}\b", text):
                text += " " + rdj.bounds_str(bp["bounds"])
        for p in fn["generics"]["params"]:
            if "type" in p["kind"] and re.search(rf"\b{p['name']}\b", text):
                text += " " + rdj.bounds_str(p["kind"]["type"]["bounds"])
    return text


def borrows_arg(fn, out_s, ins):
    """Return True if every lifetime in return type `out_s` is declared on `fn` and appears in an argument."""
    own = {p["name"] for p in fn["generics"]["params"] if "lifetime" in p["kind"]}
    used = set(re.findall(r"'\w+", out_s))
    if not used or not used <= own:
        return False
    args = " ".join(rdj.ty(t) for _, t in ins[1:])
    return all(re.search(rf"{u}\b", args) for u in used)


# Built-in skip explanations merged into [skip] during after_scan().
# Kept in code rather than api/overrides.toml because they group large families of
# Rust-only plot paint helpers, dock internals, and render-closure state builders.


_PAINT = ("rust-only-extension: a paint-time cache of a plot that paints itself in Rust (Plot::paint with its "
          "window and bounds); the library's plot (core/src/plot.rs GkPlot) paints the parts described with gk.plot")
_PATHS = ("rust-only-extension: path geometry (gpui Path<Pixels>) for a Plot::paint written in Rust; the parts of a "
          "described plot (gk.plot) are painted by the library")
_HOVER = ("rust-only-extension: handed only to Plot::hover of a plot that paints itself in Rust (its hover motion "
          "repaints within a frame); the library's GkPlot keeps upstream's default")
_HOOKS = ("rust-only-extension: a hook of a plot that paints itself in Rust (appear and hover motion, children made "
          "while it prepaints, tooltip elements built while it paints); the library's GkPlot keeps upstream's default")
_ARC = ("rust-only-extension: paints one arc of a pie over a borrowed row (ArcData) from a Plot::paint written in "
        "Rust; Pie::arcs reads the arcs back as gk.Arc_Data (PROTOCOLS.md 12.15)")
_NEVER = "rust-only-extension: upstream never takes one: "
_RENDER = "item-render-closure: "
_MAKER = ("value-helper: makes the state Calendar::item's closure is handed, which no kit function takes; an item "
          "procedure reads it as the record gk.Calendar_Item_State (PROTOCOLS.md 9.41)")
SKIPS5 = {
    "gpui_kit::base::plot::PathCaches::for_paint": _PAINT,
    "gpui_kit::base::plot::PathCaches::slot": _PAINT,
    "gpui_kit::base::plot::PathCaches::slot_pair": _PAINT,
    "gpui_kit::base::plot::PathCache::get": _PAINT,
    "gpui_kit::base::plot::ShapeKey::point": _PAINT,
    "gpui_kit::base::plot::ShapeKey::f32": _PAINT,
    "gpui_kit::base::plot::origin_point": _PATHS,
    "gpui_kit::base::plot::polygon": _PATHS,
    "gpui_kit::base::plot::shape::sankey_link_path": _PATHS,
    "gpui_kit::base::plot::PlotHover::state": _HOVER,
    "gpui_kit::base::plot::PlotHover::progress": _HOVER,
    "gpui_kit::base::plot::PlotHover::is_hovered": _HOVER,
    "gpui_kit::base::plot::PlotHover::is_entering": _HOVER,
    "gpui_kit::base::plot::PlotHover::glide": _HOVER,
    "gpui_kit::base::plot::Plot::appear": _HOOKS,
    "gpui_kit::base::plot::Plot::hover": _HOOKS,
    "gpui_kit::base::plot::Plot::prepaint": _HOOKS,
    "gpui_kit::base::plot::Plot::tooltip": _HOOKS,
    "gpui_kit::base::plot::Plot::tooltip_state": _HOOKS,
    "gpui_kit::base::plot::PlotElement::new": ("rust-only-extension: wraps a Plot implemented in Rust; gk.plot and "
                                               "gk.plot_with make the element of the library's GkPlot"),
    "gpui_kit::base::plot::shape::ArcData::new": _ARC,
    "gpui_kit::base::plot::shape::Arc::new": _ARC,
    "gpui_kit::base::plot::shape::Arc::inner_radius": _ARC,
    "gpui_kit::base::plot::shape::Arc::outer_radius": _ARC,
    "gpui_kit::base::plot::shape::Arc::Default::default": _ARC,
    "gpui_kit::base::plot::TooltipState::new": _NEVER + "taken only by Plot::tooltip of a plot that paints itself in Rust",
    "gpui_kit::base::plot::shape::SankeyNodeLayout::Default::default": _NEVER + "taken only by sankey_link_path (path "
                                                                       "geometry for a Rust Plot::paint)",
    "gpui_kit::component::dock::DockSkin::new": ("rust-only-extension: needs the DockArea's own Context while the area "
                                                 "is constructed; DockSkin::dock_area (gk.dock_area_skinned) makes both"),
    "gpui_kit::base::dock::AnyDrag::new": ("rust-only-extension: Rust's dynamic typing (std::any::Any) of a drag "
                                           "payload; the app's drag values travel as gk.Drag_Value (PROTOCOLS.md 13.20)"),
    "gpui_kit::base::dock::AnyDrag::value": ("rust-only-extension: Rust's dynamic typing (Arc<dyn Any>) of a drag "
                                             "payload, for downcasting in Rust"),
    "gpui_kit::base::dock::TabGroupConstraints::sealed": _NEVER + "the facts the dock pushes into a tab group (TabGroup::set_constraints is pub(crate)); no public function takes or returns one",
    "gpui_kit::base::dock::TabGroupConstraints::in_split": _NEVER + "the facts the dock pushes into a tab group (TabGroup::set_constraints is pub(crate)); no public function takes or returns one",
    "gpui_kit::base::dock::TabGroupConstraints::dock_locked": _NEVER + "the facts the dock pushes into a tab group (TabGroup::set_constraints is pub(crate)); no public function takes or returns one",
    "gpui_kit::base::dock::TabGroupConstraints::collapsed": _NEVER + "the facts the dock pushes into a tab group (TabGroup::set_constraints is pub(crate)); no public function takes or returns one",
    "gpui_kit::base::dock::TabGroupConstraints::closable": _NEVER + "the facts the dock pushes into a tab group (TabGroup::set_constraints is pub(crate)); no public function takes or returns one",
    "gpui_kit::base::dock::PanelId::from_u64": ("rust-only-extension: a panel id from a raw number; the binding's panel "
                                                "ids are the ones the dock hands out (gk.Panel_Id, PROTOCOLS.md 11.8)"),
    "gpui_kit::base::TreeEntryState::Default::default": _RENDER + "the state upstream hands a tree item's render closure; no function takes one",
    "gpui_kit::component::list::ListSeparatorItem::new": _NEVER + "upstream's own list rendering",
    "gpui_kit::component::AnyChildElement::new": _NEVER + "the table parts' own type-erased child, kept in private fields (Table::child wraps its argument itself); gk.Table_Child takes every ChildElement directly",
    "gpui_kit::component::menu::ContextMenuState::Default::default": _NEVER + "upstream's own context-menu element state",
    "gpui_kit::component::notification::Notification::id": ("rust-only-extension: keys the notification by a Rust type "
                                                             "(TypeId of T); the binding cannot name an app's Rust type"),
    "gpui_kit::component::notification::Notification::id1": ("rust-only-extension: keys the notification by a Rust type "
                                                              "(TypeId of T) and an id; the binding cannot name an app's Rust type"),
    "gpui_kit::component::dialog::DialogFooterButton::is_action": ("rust-only-extension: a trait no kit type implements and "
                                                                   "no kit function takes; for a Rust app's own footer buttons"),
    "gpui_kit::component::dialog::DialogFooterButton::is_cancel": ("rust-only-extension: a trait no kit type implements and "
                                                                   "no kit function takes; for a Rust app's own footer buttons"),
    "gpui_kit::component::menu::PopupMenu::build": ("rust-only-extension: a menu entity built by a Rust closure with "
                                                    "the menu's own Context; the binding describes a menu as the value a "
                                                    "dropdown, context or submenu's closure builds (PROTOCOLS.md 11.6)"),
    "gpui_kit::component::menu::PopupMenuItem::submenu": ("rust-only-extension: takes a menu entity built by a Rust "
                                                          "closure (PopupMenu::build); a described menu's submenus are "
                                                          "PopupMenu::submenu and submenu_with_icon"),
    "gpui_kit::component::menu::ContextMenu::new": ("rust-only-extension: a context menu with no menu (its builder "
                                                    "`menu` is private); ContextMenuExt::context_menu (bound) makes one with it"),
    "gpui_kit::base::AutoScroll::set": ("rust-only-extension: drives a Rust entity T through its own Context on each "
                                        "tick; the kit's text views and lists scroll themselves"),
    "gpui_kit::base::CalendarState::set_disabled_matcher_shared": ("rust-only-extension: one Rc'd matcher shared between "
                                                                   "Rust states; set_disabled_matcher takes the same matcher by value (bound)"),
    "gpui_kit::base::CalendarItemState::new": _MAKER,
    "gpui_kit::base::CalendarItemState::active": _MAKER,
    "gpui_kit::base::CalendarItemState::in_range": _MAKER,
    "gpui_kit::base::CalendarItemState::muted": _MAKER,
    "gpui_kit::base::CalendarItemState::disabled": _MAKER,
    "gpui_kit::base::CalendarItemState::today": _MAKER,
}
for _n in ("new", "with_page_ix", "with_group_ix", "with_item_ix", "with_size", "with_group_variant", "with_layout",
           "with_disabled", "Default::default"):
    SKIPS5["gpui_kit::component::setting::RenderOptions::" + _n] = (
        _NEVER + "what upstream hands a Rust settings field's render (SettingFieldElement::render_field), which the "
        "library serves with the app's fragment (PROTOCOLS.md 13.19)")
SKIPS5["gpui_kit::base::IndexPath::eq_row"] = ("value-helper: compares the rows of two IndexPath records the app holds "
                                               "(gk.Base_Index_Path)")
for _n in ("new", "section", "row", "column", "Default::default"):
    SKIPS5["gpui_kit::base::IndexPath::" + _n] = (
        "value-helper: IndexPath is a plain record (gk.Base_Index_Path) wherever a call takes it (PROTOCOLS.md 13.6); "
        "its builders set the same fields")

BRIDGED5 = {
    "gpui_kit::component::searchable_list::SearchableListDelegate::on_confirm":
        "core/src/shim_select.rs GkSearchDelegate: upstream's default (nothing); the same final selection reaches the "
        "app in the state's Confirm event (SelectEvent, ComboboxEvent), which upstream emits beside the call",
}


def merge_skips(m):
    """Merge `SKIPS5` into `m.ov["skip"]`, rejecting duplicates also declared in `api/overrides.toml`."""
    sk = m.ov.setdefault("skip", {})
    for k, v in SKIPS5.items():
        if k in sk:
            raise SystemExit(f"gk_overlay: {k} is in api/overrides.toml [skip] and in SKIPS5")
        sk[k] = v


def observed(params, out_s, kind="Fn"):
    """Return True if a void `Fn` closure takes only plain payload args (no window/app/context)."""
    return kind == "Fn" and bool(params) and all(p == "arg" for p in params) and out_s == "()"


def rust_observe(d, num, payload):
    """Emit `crate::views_after::observe` for plain-payload void closures that lack a Window/App handle."""
    if not observed(d[2], "()"):
        return None
    return f"crate::views_after::observe(h, {num}, {payload})"


def value_read(m, node, it, crate, fn, ctx):
    """Promote a pure `&self` read method on a value node into a standalone ambient call.

    Because value nodes live only briefly while building a tape, their query methods
    (taking `&self` plus wire-safe args and returning a plain owned value) are exposed as
    ambient calls taking the described value node as the first argument. On refusal, any
    speculative `in`/`out` flags set on records/denums during probing are rolled back.
    """
    from gk_model import Unsup, snake
    import gk_app
    ins = fn["sig"]["inputs"]
    out = fn["sig"].get("output")
    if node["element"] or node.get("inst") or node["subst"] or len(ins) < 2 or out is None:
        return False
    out_s = rdj.ty(out)
    if "&" in out_s or "'" in out_s or out_s == "()" or out_s == "Self":
        return False
    if any(re.match(r"&(mut )?(gpui::)?(Window|App|Context)\b", rdj.ty(t)) for _, t in ins[1:]):
        return False
    if fn["generics"]["params"] and any("type" in p["kind"] for p in fn["generics"]["params"]):
        return False
    flags = {(k, n): (t.get("in"), t.get("out")) for k, tab in (("rec", m.records), ("denum", m.denums))
             for n, t in tab.items()}

    def refuse():
        for k, tab in (("rec", m.records), ("denum", m.denums)):
            for n, t in tab.items():
                t["in"], t["out"] = flags.get((k, n), (False, False))
        return False
    try:
        args = m.args(ctx, ins[1:])
        for a in args:
            if m.call_bad(a["desc"], False) is not None:
                return refuse()
        ret = m.desc(ctx, out, out=True)
        if m.call_bad(ret, False) is not None:
            return refuse()
    except Unsup:
        return refuse()
    tp = node["path"]
    names = [a["name"] for a in args]
    tail = "".join(", " + n for n in names)
    it.update(kind="ambient", status="bound", reason="", crate=crate, borrowed_ret=False, ret=ret,
              args=[{"name": "value", "desc": ["node", node["key"]]}] + args,
              callee=f"(|value: {tp}{tail}| <{tp}>::{it['name']}(&value{tail}))",
              stem_base=f"{'' if crate == 'gpui_component' else 'base_'}{snake(node['name'])}_{it['name']}",
              detail="a value call (PROTOCOLS.md 11.15)")
    if not hasattr(m, "ambient"):
        m.ambient = []
    m.ambient.append(it)
    return True


def post(m):
    """Apply late overlay shim marks (`BRIDGED5`) and clear args/ret on bridged `LIBRARY_CALLS`."""
    for it in m.all_items():
        if it["id"] in BRIDGED5 and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="", detail=BRIDGED5[it["id"]], args=[])
            it.pop("ret", None)
        if it["id"] in LIBRARY_CALLS and it["status"] == "bridged":
            it["args"] = []
            it.pop("ret", None)
