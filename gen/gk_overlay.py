"""The overlays and dock families' generator rules (docs/PROTOCOLS.md sections 11.6 and 11.7).

Called from the shared model and emitters at a few hook points, so the rules live here:

- view-built closures (11.6): a closure `Fn(T, &mut Window, &mut App) -> T + 'static` that a
  call op hands to upstream, which keeps it and calls it later (`WindowExt::open_dialog`,
  `open_sheet`): the descriptor `build` of an element op becomes `viewbuild` in a call op
  (`view_built`, from `Model.call_bad`). The op carries the u64 handle of a view whose root is
  a node of type T; the library applies that root's ops to the T upstream hands in, each time
  upstream calls the closure (`crate::overlay::ViewBuild`). Go: `gk.NewBuilt(cx, state, fn)`,
  a `gk.Built[T]`.
- build closures with the entity's context (11.6): `Fn(PopupMenu, &mut Window,
  &mut Context<PopupMenu>) -> PopupMenu` in an element op (dropdown and context menus) is a
  `build` closure like `Fn(X, &mut Window, &mut App) -> X` (`context_build`, from
  `Model.closure_desc`); the context derefs to the App the decode needs.
- described values (11.6): an entity type upstream builds itself and hands to such a closure
  (`PopupMenu`) is a value node of its own (`[node] types` in api/overrides.toml), whose ops are
  its `self -> Self` methods. It has no public constructor: `after_classify` gives it one that
  only the app uses to start the node (the library never builds one alone; not counted).
  `PopupMenu::submenu` needs the menu's own context; the library's `overlay::submenu` builds the
  child with `PopupMenu::build` and attaches it with `item(PopupMenuItem::submenu(..))`, which
  upstream wires to its parent when the menu draws.
- wrappers a type-changing method returns, for one element type named by its full path
  (`OVERLAY_INSTANCES`): `ContextMenuExt::context_menu` on Div gives `ContextMenu<Div>` (a
  blanket trait, entered into Div's tables here).
- fallible calls (questionnaire): `Result<(), E>` as a result is `opt<str>` on the wire, the
  error's text or nothing (`result_desc`); a constructor or builder that returns
  `Result<Self, E>` stops the app with the error's text (`fallible`), which is what a schema
  the app wrote wrongly deserves; a `&self` method that needs the window is a call op run at
  once with the turn's window (`read_needs_window`).

Standard library only."""
import json
import re

import rdj

# Generic wrappers a type-changing method returns, instantiated for one element type given by
# its public path (the older `[instantiate] types` rule finds an argument by its short name, and
# there are two `Button`s): (wrapper's public path, argument's public path, name). The kit's
# `DropdownMenuPopover<Button>` has no public path, so rustc cannot name it: the dropdown button
# is the library's own node (`gk.dropdown_button`, core/src/overlay.rs).
OVERLAY_INSTANCES = [
    ("gpui_kit::component::menu::ContextMenu", "gpui_kit::Div", "ContextMenuDiv"),
]
# Blanket traits whose methods change the type, entered into a node's trait tables because the
# extract lists a blanket impl only in the crate that declares the trait (trait path, node path).
BLANKET_TABLES = [("ContextMenuExt", "gpui_kit::Div")]
# Functions a node of the library itself binds, where generated code cannot name the type upstream
# returns: the item, where.
LIBRARY_NODES = {
    "gpui_kit::component::menu::DropdownMenu::dropdown_menu":
        "core/src/overlay.rs, node DROPDOWN_BUTTON (gk.dropdown_menu): the kit's DropdownMenuPopover<Button> has no public path",
}
# Methods of a described value that need the value's own entity context, done by the library
# without it (`crate::overlay::NAME`): the method, the library function.
CONTEXT_OPS = {("PopupMenu", "submenu"): "crate::overlay::submenu",
               ("PopupMenu", "submenu_with_icon"): "crate::views_menu::submenu_with_icon"}  # the views family


def _find(m, path):
    """(crate, id) of a type by its public path."""
    for cr, w in m.api.walks.items():
        for iid in w.types:
            if m.api.pubpath(cr, iid) == path:
                return cr, iid
    return None


def after_scan(m):
    """Node types these families add once the node types are known (Model.scan_types)."""
    merge_skips(m)  # SKIPS5 as [skip] keys
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
    return set(m.ov.get("node", {}).get("types", []))


def after_classify(m):
    """Before the model settles: blanket trait tables, the described values' constructors and
    their context ops (Model.__init__, after classify_crate)."""
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
    after_classify4(m)  # PROTOCOLS.md 11.9 onward
    for it in m.items:
        if it["id"] in LIBRARY_CALLS and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="", detail="core/src/dock.rs: " + LIBRARY_CALLS[it["id"]])
    for st in list(m.objects.values()) + list(m.states.values()):
        # served by the library's own calls: not a method the object or state rule emits (and not
        # one reach_objects may take back)
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
            # The app starts the node with it; the library applies the node's ops to the value
            # upstream hands in and never builds one alone. Not an upstream function: no row.
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
    """A wrapper instantiated for one element type (`ContextMenu<Div>`) implements the builder
    traits of its argument through bounded impls (`impl<E: ParentElement + Styled> Styled for
    ContextMenu<E>`), which the scan does not match to the instance: it gets the argument's
    entries of the traits the wrapper implements (the gensample gate failed on a
    `Context_Menu_Div` with no `style`)."""
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
    """`submenu(self, label, window, cx: &mut Context<Self>, f)` as an op of the described value:
    the library's function takes `&mut App` where upstream takes the entity's context."""
    from gk_model import Ctx, Unsup  # noqa: PLC0415 (gk_model imports this module)
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
                one = _single_into(m, s)  # Option<Icon> as the Icon its Into set makes
                if one is None:
                    raise
                args.append({"name": pn, "desc": one})
    except Unsup as e:
        return m.set(it, e)
    it.update(kind="op", status="bridged", reason="", callee=callee, args=args, detail="")
    if it not in node["ops"]:
        node["ops"].append(it)


def _single_into(m, s):
    """`T` or `Option<T>` where T is a kit type the binding makes only through its Into set's one
    member (`Icon` from an `IconName`): that member's descriptor (PopupMenu::submenu_with_icon)."""
    mt = re.fullmatch(r"(Option<)?(\w+)(>)?", s)
    if mt is None or (mt.group(1) is None) != (mt.group(3) is None):
        return None
    hit = [x for k, x in m.intos.items() if k.rsplit("::", 1)[-1] == mt.group(2) and x.get("single")]
    if len(hit) != 1:
        return None
    return ["opt", hit[0]["single"]] if mt.group(1) else hit[0]["single"]


# ---------------------------------------------------------------- closures


def context_build(params):
    """A build closure that gets the window and the entity's own context (a menu's)."""
    return "window" in params and "context" in params and "app" not in params


def view_built(m, x, builder, seen):
    """A `build` closure at the top of a call op's argument: view-built (PROTOCOLS.md 11.6).
    The descriptor is changed in place, so the emitters see `viewbuild`. False when it is not."""
    if x[0] == "panelview" and not builder:
        return True  # a dock panel's handle (PROTOCOLS.md 11.7)
    if x[0] in ("field", "format", "data"):
        return True  # rows and closures over a row in a call op (PROTOCOLS.md 12.15)
    if call_safe(x):
        return True  # node ids, views (PROTOCOLS.md 11.8)
    if x[0] not in ("build", "viewbuild") or builder or seen:
        return False
    if "window" not in x[2]:
        return False
    x[0] = "viewbuild"
    return True


def ctor_nums(m, node_key):
    return sorted(i["op"] for i in m.nodes[node_key]["ctors"] if i["status"] in ("bound", "bridged") and "op" in i)


def rust_dec(rs, d):
    """`{ let vb = crate::overlay::ViewBuild::take(r.u64(), c, &[..])?; move |v, window, cx| vb.apply(..) }`"""
    node = rs.m.nodes[d[1]]
    nums = ", ".join(map(str, ctor_nums(rs.m, d[1]))) or "0"
    ps = rs.closure_params(d[2], d[4], "v")
    return rs.wrap(d[3], '{ let vb = crate::overlay::ViewBuild::take(r.u64(), c, &[%s], "%s")?; move |%s| vb.apply(v, window, cx, apply_%s) }'
                   % (nums, node["name"], ps, node["fn"]))


# ---------------------------------------------------------------- fallible calls


def result_desc(m, ctx, args, out):
    """`Result<(), E>` read back from a call: the error's text, or nothing (`opt<str>`)."""
    from gk_model import Unsup  # noqa: PLC0415
    if not out or len(args) != 2:
        raise Unsup("norule", "std type core::result::Result")
    ok = args[0]
    if not ("tuple" in ok and not ok["tuple"]):
        raise Unsup("todo:protocol: calls (a result other than Result<(), E>)", rdj.ty(ok))
    return ["result", "unit"]


def rust_put_ret(expr):
    return f"genapi::Wire::put(&crate::overlay::outcome({expr}), &mut out)"


def fallible_callee(callee, n_args):
    """A constructor or builder that returns Result<Self, E>: the library stops the app with the
    error's text (crate::overlay::Fallible)."""
    return f"crate::overlay::Fallible({callee}).call{n_args}"


def is_result_self(out_s, name):
    s = out_s.replace(" ", "")
    return bool(re.fullmatch(rf"Result<(Self|{name}),\w+>", s))


def read_needs_window(ins):
    """A `&self` method that takes the window: a call op run at once with the turn's window."""
    return any(re.match(r"&(mut )?(gpui::)?Window\b", rdj.ty(t)) for _, t in ins[1:])


def describe(m):
    """For the report: what this family's rules bound."""
    return json.dumps({"instances": [n for _, _, n in OVERLAY_INSTANCES], "described": sorted(described_values(m))})


# ---------------------------------------------------------------- dock panels (PROTOCOLS.md 11.7)

# The library's one Panel: an app view behind a state handle (core/src/dock.rs GkPanel).
PANEL_RUST = "crate::dock::GkPanel"
PANEL_NAME = "DockPanel"  # Go: gk.DockPanel
# Trait methods GkPanel serves (its own impl from the data the app pushed, or upstream's blanket
# PanelView impl over Entity<GkPanel>): counted bridged.
PANEL_TRAITS = ("gpui_kit::base::dock::Panel::", "gpui_kit::base::dock::PanelView::",
                "gpui_kit::component::dock::Panel::", "gpui_kit::component::dock::PanelView::")


def _bound_names(bounds):
    out = []
    for b in bounds or []:
        tb = b.get("trait_bound")
        if tb:
            out.append(tb["trait"]["path"].split("::")[-1])
    return out


def panel_entity(m, ctx, a):
    """`Entity<P>` with `P: Panel`: the app's panels are all GkPanel (one closed set)."""
    g = a.get("generic")
    if g and g in ctx.bounds and "Panel" in _bound_names(ctx.bounds[g]):
        return ["entity", PANEL_RUST, PANEL_NAME]
    return None


def panel_view(m, ctx, name, a, out):
    """`Arc<dyn PanelView>` given to the kit: made from a GkPanel handle. gpui-base and the
    component layer each have a PanelView; the descriptor names which."""
    if out or name != "Arc" or "dyn_trait" not in a:
        return None
    trs = a["dyn_trait"]["traits"]
    if [x["trait"]["path"].split("::")[-1] for x in trs] != ["PanelView"]:
        return None
    hc, _ = m.api.home(ctx.crate, trs[0]["trait"]["id"])
    return ["panelview", "kit" if hc == "gpui_component" else "base"]


def rust_panel_view(rs, d):
    f = "kit_panel_view" if d[1] == "kit" else "panel_view"
    return f"crate::dock::{f}(r.u64())?" if rs.call else f"crate::dock::{f}_shown(r.u64())"


def mark_panel_traits(m):
    for it in m.items:
        if it["id"].startswith(PANEL_TRAITS) and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="",
                      detail="core/src/dock.rs GkPanel: from the data the app pushed (gk.panel_set), or upstream's "
                             "PanelView over Entity<GkPanel>; set_active, set_zoomed, on_added_to, on_removed are "
                             "Panel_Event handler calls")


# ---------------------------------------------------------------- PROTOCOLS.md 11.8
#
# Values the dock and the kit hand out and take back, as one descriptor family `["dock", what,
# ..]` (call-safe, both directions):
#   node_id   gpui-base's NodeId: a u64 on the wire. NodeId has no public constructor, so the library
#             keeps every id it handed out (crate::dock::node_id) and refuses one it never did;
#   panel_id  PanelId: its u64 (PanelId::from_u64 / as_u64);
#   view      gpui's AnyView (and `impl Into<AnyView>`): an app view's handle, gk.View in Go. Taken
#             in a call op the view is drawn in the turn's window and marked dirty; read back, a
#             view of the app is its handle and any other view 0;
#   panel     Arc<dyn PanelView> read back: the gk.Dock_Panel it was made from (0 for the kit's
#             own placeholders);
#   ms        std::time::Duration read back: i64 nanoseconds (Go time.Duration);
#   entity    Entity<T> read back, T a state type: a state handle, the same handle for the same
#             entity while it lives (crate::dock::entity_handle).

# Entity states nothing constructs but the library hands out (crate::dock): their methods are the
# state rule's. TabGroup comes from gk.panel_group (a panel's tab group, told to the panel by the
# dock), and from reads that return it.
HANDED = ("gpui_kit::base::dock::TabGroup", "gpui_kit::component::menu::AppMenuBar")
# Entity states with no constructor but `impl Default`: `T::default()` is their constructor (not an
# upstream function of their own: no coverage row).
DEFAULT_CTORS = ("gpui_kit::base::ResizableState",)


def _rp_home(m, ctx, t):
    rp = t["resolved_path"]
    try:
        hc, hid, it = m.api.item(ctx.crate, rp["id"])
    except Exception:  # noqa: BLE001 (an id outside the extract)
        return None, None, None
    return hc, hid, it


def resolved(m, ctx, t, out):
    """The descriptor of the dock family for a resolved path, or None (Model.resolved's hook)."""
    rp = t["resolved_path"]
    name = rp["path"].split("::")[-1]
    if name not in ("NodeId", "PanelId", "AnyView", "Duration", "Arc", "Entity", "Rc") and name not in LAYOUT_VALUES \
            and name not in ROUND4_NAMES:
        return None
    r4 = resolved4(m, ctx, t, out, name)  # renderers, the library clock (PROTOCOLS.md 11.9, 11.10)
    if r4 is not None:
        return r4
    if name == "Rc":
        return None
    hc, hid, it = _rp_home(m, ctx, t)
    if name in LAYOUT_VALUES and hc == "gpui_base":
        return ["dock", LAYOUT_VALUES[name]]  # a saved layout, a wire tree both ways
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


# A saved layout (DockArea::dump and load): plain values with hand-written codecs in core/src/dock.rs
# (PanelState is recursive, PanelInfo holds JSON), Go structs and codecs in go/gk/rt_dock.go.
LAYOUT_VALUES = {"DockAreaState": "area_state", "PanelState": "panel_state", "PanelInfo": "panel_info"}
LAYOUT_RUST = {"area_state": "gpui_kit::base::dock::DockAreaState", "panel_state": "gpui_kit::base::dock::PanelState",
               "panel_info": "gpui_kit::base::dock::PanelInfo"}

# Functions the library's own calls serve (core/src/dock.rs; go/gk/rt_dock.go): the registry,
# the build context and the two persistence traits, implemented once by the library.
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
    return x[0] == "dock"


def mark_handed(m):
    """After classification: states the library hands out, and Default constructors."""
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
    w = d[1]
    if w in ROUND4_PUT:
        return ROUND4_PUT[w](expr)
    if w == "entity":
        return f"crate::dock::put_entity(&{expr}, &mut out)"
    if w == "ms":
        return f"crate::dock::put_ms(&{expr}, &mut out)"
    return f"genapi::Wire::put(&crate::dock::Out({expr}), &mut out)"


def rust_dock_put(ret, expr):
    """A result that holds a dock descriptor: written through crate::dock's writers."""
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


# ---------------------------------------------------------------- PROTOCOLS.md 11.9 onward
#
# Dock renderers (11.9): `Rc<dyn DockAreaRenderer>` and `Rc<dyn TabGroupRenderer>` given to the kit
# are one library renderer (crate::dock::GkRenderer, implementing both traits) behind an object
# handle, `gk.DockRenderer` in Go. Its hooks answer from app views the app gave per (hook, key)
# between frames, asked by a queued Dock_Render request when a key has no answer for the context it
# is drawn with; the trait methods count bridged. The contexts the kit hands the hooks
# (DockContext, TabGroupContext) reach the app inside the request: a library hand-out (below).
#
# The library clock (11.10): std::time::Instant both ways is a duration since the library's clock
# began (crate::overlay::instant), `time.Duration` in Go, `gk.ClockNow(cx)` the clock's reading.

ROUND4_NAMES = ("Instant", "ArcData", "StackSeries")

RENDERER_TRAITS = ("DockAreaRenderer", "TabGroupRenderer")
RENDERER_ITEMS = ("gpui_kit::base::dock::DockAreaRenderer::", "gpui_kit::base::dock::TabGroupRenderer::")

# Library objects the library itself hands to the app (inside a request it sends), so they are
# reachable without an upstream function returning one: the object's public path, where.
LIBRARY_HANDOUTS = {
    "gpui_kit::base::dock::DockContext": "the dock hook's Dock_Render request (crate::dock::GkRenderer::render_dock)",
    "gpui_kit::base::dock::TabGroupContext": "the tab group hooks' Dock_Render request (crate::dock::GkRenderer)",
}


def resolved4(m, ctx, t, out, name):
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


# Kit values over rows of the app's data read back as library values (Wire in core/src/data.rs,
# Go structs in go/gk/rt_data.go): a row is its index in the rows the app passed.
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


# Functions the library's calls of PROTOCOLS.md 11.9 onward serve: the event's reads are the fields of its payload.
LIBRARY_CALLS4 = {
    "gpui_kit::component::dock::DockToggleEvent::placement": "gk.on_dock_toggle (call DOCK_TOGGLE_SUBSCRIBE): Dock_Toggle.placement",
    "gpui_kit::component::dock::DockToggleEvent::is_open": "gk.on_dock_toggle (call DOCK_TOGGLE_SUBSCRIBE): Dock_Toggle.open",
}


def after_classify4(m):
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
    """The argument text a generic parameter must appear in to count as named, with the bounds of
    the parameters named there (`keys<I, S>(keys: I) where I: IntoIterator<Item = S>`: S is named
    by I's bound), as Model.bindable does for node methods (PROTOCOLS.md 12.1)."""
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
    """A result whose every lifetime is the method's own and is given to an argument (`arcs<'a>(&self,
    data: &'a [T]) -> Vec<ArcData<'a, T>>`) borrows from that argument, which lives while the call
    writes the result out: it can be read back (PROTOCOLS.md 12.15)."""
    own = {p["name"] for p in fn["generics"]["params"] if "lifetime" in p["kind"]}
    used = set(re.findall(r"'\w+", out_s))
    if not used or not used <= own:
        return False
    args = " ".join(rdj.ty(t) for _, t in ins[1:])
    return all(re.search(rf"{u}\b", args) for u in used)


# ---------------------------------------------------------------- the views family
#
# PROTOCOLS.md 11.14 to 11.16: functions skipped with a reason of DESIGN.md section 13's fixed
# list (SKIPS5, merged into `[skip]` so the model's own rule applies them and a key that matches
# nothing stops generation), observations (closures upstream calls while it lays out, delivered
# after the frame), value calls (a `&self` method of a value the app builds that takes arguments:
# a computation, run as an ambient call over the value), and library calls whose items the model
# re-marked after this module bridged them.

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
    # dock
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
    # tables and lists
    "gpui_kit::base::TreeEntryState::Default::default": _RENDER + "the state upstream hands a tree item's render closure; no function takes one",
    "gpui_kit::component::list::ListSeparatorItem::new": _NEVER + "upstream's own list rendering",
    "gpui_kit::component::AnyChildElement::new": _NEVER + "the table parts' own type-erased child, kept in private fields (Table::child wraps its argument itself); gk.Table_Child takes every ChildElement directly",
    "gpui_kit::component::menu::ContextMenuState::Default::default": _NEVER + "upstream's own context-menu element state",
    # dialogs
    "gpui_kit::component::notification::Notification::id": ("rust-only-extension: keys the notification by a Rust type "
                                                             "(TypeId of T); the binding cannot name an app's Rust type"),
    "gpui_kit::component::notification::Notification::id1": ("rust-only-extension: keys the notification by a Rust type "
                                                              "(TypeId of T) and an id; the binding cannot name an app's Rust type"),
    "gpui_kit::component::dialog::DialogFooterButton::is_action": ("rust-only-extension: a trait no kit type implements and "
                                                                   "no kit function takes; for a Rust app's own footer buttons"),
    "gpui_kit::component::dialog::DialogFooterButton::is_cancel": ("rust-only-extension: a trait no kit type implements and "
                                                                   "no kit function takes; for a Rust app's own footer buttons"),
    # menus
    "gpui_kit::component::menu::PopupMenu::build": ("rust-only-extension: a menu entity built by a Rust closure with "
                                                    "the menu's own Context; the binding describes a menu as the value a "
                                                    "dropdown, context or submenu's closure builds (PROTOCOLS.md 11.6)"),
    "gpui_kit::component::menu::PopupMenuItem::submenu": ("rust-only-extension: takes a menu entity built by a Rust "
                                                          "closure (PopupMenu::build); a described menu's submenus are "
                                                          "PopupMenu::submenu and submenu_with_icon"),
    "gpui_kit::component::menu::ContextMenu::new": ("rust-only-extension: a context menu with no menu (its builder "
                                                    "`menu` is private); ContextMenuExt::context_menu (bound) makes one with it"),
    # other components
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

# Trait methods the library's own implementations serve, by item id: where (bridged).
BRIDGED5 = {
    "gpui_kit::component::searchable_list::SearchableListDelegate::on_confirm":
        "core/src/shim_select.rs GkSearchDelegate: upstream's default (nothing); the same final selection reaches the "
        "app in the state's Confirm event (SelectEvent, ComboboxEvent), which upstream emits beside the call",
}


def merge_skips(m):
    """SKIPS5 as `[skip]` keys of the model (Model.pre applies them, settle checks each matched)."""
    sk = m.ov.setdefault("skip", {})
    for k, v in SKIPS5.items():
        if k in sk:
            raise SystemExit(f"gk_overlay: {k} is in api/overrides.toml [skip] and in SKIPS5")
        sk[k] = v


# ---------------------------------------------------------------- observations (PROTOCOLS.md 11.14)


def observed(params, out_s, kind="Fn"):
    """A closure that gets values and no window, app or context and returns nothing
    (`Popover::on_position(Fn(ResolvedPosition, Bounds<Pixels>))`): upstream calls it while it lays
    out, where app code may not run. It is a handler delivered after the frame, only when its
    values differ from the last ones that handler got (`crate::views_after::observe`)."""
    return kind == "Fn" and bool(params) and all(p == "arg" for p in params) and out_s == "()"


def rust_observe(d, num, payload):
    """The body of an observation's closure (gk_rust.Rust.handler's hook)."""
    if not observed(d[2], "()"):
        return None
    return f"crate::views_after::observe(h, {num}, {payload})"


# ---------------------------------------------------------------- value calls (PROTOCOLS.md 11.15)


def value_read(m, node, it, crate, fn, ctx):
    """A `&self` method of a value node that takes arguments (`DockSizing::clamp(size)`,
    `Sankey::layout(node_count, links)`): a computation over a value the app builds with the
    node's constructor and builders, so an ambient call (PROTOCOLS.md 11.5) that takes the value as
    a tape of its own (13.6) first. One without arguments reads back what the app wrote and stays a
    builder-getter. True when bound."""
    from gk_model import Unsup, snake  # noqa: PLC0415
    import gk_app  # noqa: PLC0415
    ins = fn["sig"]["inputs"]
    out = fn["sig"].get("output")
    if node["element"] or node.get("inst") or node["subst"] or len(ins) < 2 or out is None:
        return False
    out_s = rdj.ty(out)
    if "&" in out_s or "'" in out_s or out_s == "()" or out_s == "Self":
        return False  # a result borrowing the value, which is gone after the call; a builder
    if any(re.match(r"&(mut )?(gpui::)?(Window|App|Context)\b", rdj.ty(t)) for _, t in ins[1:]):
        return False
    if fn["generics"]["params"] and any("type" in p["kind"] for p in fn["generics"]["params"]):
        return False  # a type parameter (Arc::centroid<T>): no closed set
    # Describing the arguments and the result marks the records and enums they reach as travelling
    # in or out (a codec is generated for each); a method this rule then refuses must leave no mark
    # (a StyleRefinement result marked out generated a Wire impl over fields that have none).
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


# ---------------------------------------------------------------- after the rules (Model.settle)


def post(m):
    """Statuses these families decide once every rule has run, before the model prunes (Model.settle)."""
    for it in m.all_items():
        if it["id"] in BRIDGED5 and it["status"] in ("todo", "norule"):
            it.update(kind="shim", status="bridged", reason="", detail=BRIDGED5[it["id"]], args=[])
            it.pop("ret", None)
        if it["id"] in LIBRARY_CALLS and it["status"] == "bridged":
            # served by the library's own call: its upstream arguments are not on any wire, so the
            # pruning (an argument type nothing builds) must not take it back (DockSkin::dock_area)
            it["args"] = []
            it.pop("ret", None)
