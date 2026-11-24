"""gk_go_bridges: the Go emitter's side of the runtime's hand-written bridges (go/gk/rt_*.go) and
the protocols behind them, kept apart from gk_go.py's descriptor rules.

Three things live here:

- the `["dock", what, ..]` descriptor family (PROTOCOLS.md 11.8): node and panel ids, views,
  panels, durations, entity handles, saved layouts and renderers, read and written by the
  hand-written Go codecs of go/gk/rt_dock.go;
- `PORTED`: the items the model counts bound or bridged through a hand-written bridge or a library
  shim, each with the Go runtime names that serve it. An item counts as ported only when every
  one of those names is declared by a hand-written file of go/gk (rt_*.go, kit_*.go), so the
  report cannot claim a bridge the runtime lacks;
- what the report derives from items already generated: a trait's own row (its instances on
  each element type), a summary row (its instantiated objects), a projected read (the reads it
  was split into);
- `UNBOUND`: items the model counts bridged although nothing can call them (no decoder the
  library dispatches to), reported with that reason instead of a missing Go name.
"""

# ---------------------------------------------------------------- the dock family (11.8)

# Saved layouts: Go types and codecs written by hand in go/gk/rt_dock.go.
DOCK_LAYOUT = {"area_state": "DockAreaState", "panel_state": "PanelState", "panel_info": "PanelInfo"}


def gtype(go, d, out=False):
    w = d[1]
    if w == "node_id":
        return "NodeID"
    if w == "panel_id":
        return "DockPanelID"
    if w == "view":
        return "View"
    if w == "panel":
        import gk_overlay  # noqa: PLC0415
        return go.gtype(["entity", gk_overlay.PANEL_RUST, gk_overlay.PANEL_NAME], out)
    if w in ("ms", "instant"):
        return "time.Duration"
    if w == "entity":
        st = go.m.states.get(d[3], {})
        if "go" not in st:
            from gk_go import Unsup  # noqa: PLC0415
            raise Unsup(f"dock entity {d[3]} has no Go type")
        return st["go"]
    if w in DOCK_LAYOUT:
        return DOCK_LAYOUT[w]
    if w == "renderer":
        return "DockRenderer"
    from gk_go import Unsup  # noqa: PLC0415
    raise Unsup(f"dock {w}")


def put(go, d, v):
    w = d[1]
    go.gtype(d)
    if w in ("node_id", "panel_id"):
        return [f"o.u64(uint64({v}))"]
    if w in ("view", "renderer"):
        return [f"o.u64({v}.h)"]
    if w == "instant":
        return [f"o.u64(uint64({v}))"]
    if w in DOCK_LAYOUT:
        return [f"put{DOCK_LAYOUT[w]}(o, {v})"]
    from gk_go import Unsup  # noqa: PLC0415
    raise Unsup(f"dock {w} is only read back")


def rd(go, d, out=True):
    w = d[1]
    t = go.gtype(d, out)
    if w in ("node_id", "panel_id"):
        return f"{t}(r.u64())"
    if w in ("view", "panel", "entity", "renderer"):
        return f"{t}{{h: r.u64()}}"
    if w in ("ms", "instant"):
        return "time.Duration(int64(r.u64()))"
    if w in DOCK_LAYOUT:
        return f"rd{DOCK_LAYOUT[w]}(r)"
    from gk_go import Unsup  # noqa: PLC0415
    raise Unsup(f"dock {w} read back")


# ---------------------------------------------------------------- what the model counts but nothing binds

# Items the model counts bridged although nothing binds them: the report names why instead of a
# missing Go runtime name.
_STYLE_SIZED = ("no element's decoder dispatches to "
                "core/src/gen/traits.rs tr_style_sized_53 (the library answers 'op is not an op of Div')")
UNBOUND = {
    **{f"gpui_kit::component::StyleSized::{n}": _STYLE_SIZED for n in (
        "button_text_size", "input_h", "input_pl", "input_pr", "input_px", "input_py", "input_size", "input_text_size",
        "list_px", "list_py", "list_size", "size_with", "table_cell_size")},
    "gpui_kit::base::VirtualList::track_scroll": "the library's virtual list node (core/src/source.rs) takes only style ops: no decoder to call",
    "gpui_kit::base::VirtualList::with_sizing_behavior": "the library's virtual list node (core/src/source.rs) takes only style ops: no decoder to call",
    "gpui_kit::base::VirtualList::with_item_to_measure_index": "the library's virtual list node (core/src/source.rs) takes only style ops: no decoder to call",
}


# ---------------------------------------------------------------- what the Go runtime ports

# Item id (or an id prefix ending in "::") -> the hand-written Go names that serve it. The names
# are what an app calls (or the type it gets); an item whose names are all declared counts as
# ported. Grouped by family.
_DOCK_PANEL = ("NewPanel", "SetPanel", "EmitPanel", "PanelEvents")  # GkPanel: from the data the app pushed
PORTED = {
    # windows (rt_state.go, rt_app.go)
    "gpui_kit::component::TitleBar::window_options": ("WindowConfig", "OpenWindowWith"),
    # the menu bar (rt_platform.go)
    "gpui_kit::base::GlobalState::app_menus": ("Menus",),
    "gpui_kit::base::GlobalState::set_app_menus": ("SetMenus",),
    # dropdowns, key bindings, the app menu bar, the clock
    "gpui_kit::component::menu::DropdownMenu::dropdown_menu": ("DropdownMenu",),
    "gpui_kit::component::menu::DropdownMenu::dropdown_menu_with_anchor": ("DropdownMenuAnchored",),
    "gpui_kit::component::kbd::Kbd::binding_for_action": ("KbdForAction",),
    "gpui_kit::component::kbd::Kbd::binding_for_action_in": ("KbdForActionIn",),
    "gpui_kit::component::kbd::Kbd::global_binding_for_action": ("KbdGlobalForAction",),
    "gpui_kit::component::menu::AppMenuBar::new": ("NewAppMenuBar", "AppMenuBarView"),
    # animations (rt_motion.go)
    "gpui_kit::base::animation::EffectTransition::apply": ("Effect",),
    "gpui_kit::base::animate_keyframes": ("Motion", "AtKeyframes"),
    "gpui_kit::base::spring": ("Motion", "ToSpring"),
    "gpui_kit::base::transition": ("Motion", "ToTransition"),
    "gpui_kit::base::Presence::sample": ("Motion", "AtPresence"),
    "gpui_kit::base::animation::cubic_bezier": ("Motion", "EaseCubicBezier"),
    "gpui_kit::base::FocusTrapElement::focus_trap": ("FocusTrap",),
    "gpui_kit::base::ElementExt::text_selection_scope": ("TextSelectionScopeOf",),
    "gpui_kit::base::TextSelectionScopeId::new": ("NewTextSelectionScopeID",),
    # virtual lists (rt_source.go)
    "gpui_kit::base::v_virtual_list": ("VVirtualList",),
    "gpui_kit::base::h_virtual_list": ("HVirtualList",),
    # panels, saved layouts, the skin, renderers, toggles (rt_dock.go)
    "gpui_kit::base::dock::Panel::": _DOCK_PANEL,
    "gpui_kit::base::dock::PanelView::": _DOCK_PANEL,
    "gpui_kit::component::dock::Panel::": _DOCK_PANEL,
    "gpui_kit::component::dock::PanelView::": _DOCK_PANEL,
    "gpui_kit::base::dock::register_panel": ("RegisterPanel", "PanelFill"),
    "gpui_kit::base::dock::PanelRegistry::build_panel": ("BuildPanel",),
    "gpui_kit::base::dock::PanelBuildContext::": ("BuildPanel", "PanelBuild"),
    "gpui_kit::base::dock::PanelSource::": ("PaneTreeToState",),
    "gpui_kit::base::dock::PanelBuilder::build": ("PaneTreeFromState",),
    "gpui_kit::base::dock::PaneTree::to_state": ("PaneTreeToState",),
    "gpui_kit::base::dock::PaneTree::from_state": ("PaneTreeFromState",),
    "gpui_kit::base::dock::PaneNode::kind": ("PaneNodeKind",),
    "gpui_kit::base::dock::PaneNode::walk": ("PaneNodeWalk",),
    "gpui_kit::component::dock::DockSkin::dock_area": ("NewDockAreaSkinned",),
    "gpui_kit::component::dock::DockSkin::panel_style": ("DockSkinPanelStyle",),
    "gpui_kit::component::dock::DockSkin::set_panel_style": ("SetDockSkinPanelStyle",),
    "gpui_kit::component::dock::DockSkin::is_toggle_button_visible": ("DockSkinToggleButtonVisible",),
    "gpui_kit::component::dock::DockSkin::set_toggle_button_visible": ("SetDockSkinToggleButtonVisible",),
    "gpui_kit::component::dock::DockSkin::set_close_button_visible": ("SetDockSkinCloseButtonVisible",),
    "gpui_kit::component::dock::DockSkin::is_collapsed_tab_bar_visible": ("DockSkinCollapsedTabBarVisible",),
    "gpui_kit::component::dock::DockSkin::set_collapsed_tab_bar_visible": ("SetDockSkinCollapsedTabBarVisible",),
    "gpui_kit::component::dock::DockSkin::is_split_divider_line_visible": ("DockSkinSplitDividerLineVisible",),
    "gpui_kit::component::dock::DockSkin::set_split_divider_line_visible": ("SetDockSkinSplitDividerLineVisible",),
    "gpui_kit::base::dock::DockAreaRenderer::": ("NewDockRenderer", "DockRenderAnswer", "DockSlot"),
    "gpui_kit::base::dock::TabGroupRenderer::": ("NewDockRenderer", "DockRenderAnswer", "DockSlot"),
    "gpui_kit::base::dock::DockContext::(library hand-out)": ("DockRender",),
    "gpui_kit::base::dock::TabGroupContext::(library hand-out)": ("DockRender",),
    "gpui_kit::component::dock::DockToggleEvent::": ("OnDockToggle", "DockToggle"),
    # lists, tables, per-row menus (rt_rows.go, rt_table.go)
    "gpui_kit::component::list::ListDelegate::": ("NewListSource", "ListEvents", "SetDelegateData"),
    "gpui_kit::component::table::TableDelegate::": ("NewTableSource", "TableEvents", "SetDelegateData", "TableLook",
                                                    "SetTableLook", "SetTableTexts", "SetTableRowLabels", "RowMenu"),
    # selects and comboboxes (rt_select.go)
    "gpui_kit::component::searchable_list::SearchableListDelegate::": ("NewSearchSource", "SearchRequests",
                                                                      "SearchWillChange"),
    "gpui_kit::component::searchable_list::SearchableListItem::": ("SearchItem",),
    # the editor's providers and languages (rt_editor.go, rt_editor_lang.go)
    "gpui_kit::base::input::CompletionProvider::": ("EditorProviders", "ProviderRequest"),
    "gpui_kit::base::input::HoverProvider::": ("EditorProviders", "ProviderRequest"),
    "gpui_kit::base::input::DefinitionProvider::": ("EditorProviders", "ProviderRequest"),
    "gpui_kit::base::input::DocumentColorProvider::": ("EditorProviders", "ProviderRequest"),
    "gpui_kit::base::input::CodeActionProvider::": ("EditorProviders", "ProviderRequest"),
    "gpui_kit::base::input::DocumentRangeSemanticTokensProvider::": ("EditorProviders", "ProviderRequest"),
    "gpui_kit::base::input::InputHighlighter::": ("NewHighlighter",),
    "gpui_kit::base::input::LanguageProvider::": ("NewLanguageProvider",),
    "gpui_kit::base::input::SyntaxContextProvider::context_at": ("NewLanguageProvider",),
    "gpui_kit::base::input::HighlightStyleResolver::style": ("NewAsk",),
    # the go-gen agent's runtime (go/gk/rt_gen_*.go): items, Markdown plugins, questionnaire rules
    "gpui_kit::base::MarkdownPlugin::": ("MarkdownPlugin",),
    "gpui_kit::base::questionnaire::QuestionnaireValidationContext::": ("QuestionnaireRules",),
    "gpui_kit::base::CalendarItemState::": ("Items",),
    "gpui_kit::base::TimeFieldSegmentState::": ("Items",),
    "gpui_kit::base::TreeEntryState::": ("Items",),
    "gpui_kit::base::HoverCardState::is_open": ("Items",),
    "gpui_kit::base::NavPage::": ("Items",),
    "gpui_kit::component::combobox::ComboboxTriggerContext::": ("Items",),
    "gpui_kit::base::ResizeHandleContext::": ("Items",),
    "gpui_kit::component::resize_handle_appearance": ("Items",),
    # rt_editor_host.go: the editor host's element
    "gpui_kit::base::text::editor_host::collect_leaf_geometry": ("BlockHost",),
    # an event payload made of its reads (PROTOCOLS.md 9.35): gk.InputEditsEvent carries the edits
    "gpui_kit::base::input::InputEditsEvent::edits": ("InputEditsEvent",),
    # the settings fields (a field's element is a generated fragment)
    "gpui_kit::component::setting::AnySettingField::": ("SettingFieldBool", "SettingFieldNumber", "SettingFieldText"),
    "gpui_kit::component::setting::SettingFieldElement::render_field": ("NewSettingFieldTextElement",),
}


def _names(go):
    """Every name package gk declares: the hand-written runtime's and the generated ones. A file
    built only after another branch's merge (`//go:build gk_after_gen_merge`) does not count."""
    import os  # noqa: PLC0415
    cached = getattr(go, "_declared", None)
    if cached is not None and cached[0] == len(go.files):
        return cached[1]
    out = set()
    for n, w in go.names.items():
        if isinstance(w, str) and w.startswith("hand "):
            path = os.path.join(go.out, w[5:])
            if os.path.exists(path) and open(path).read().startswith("//go:build gk_after_gen_merge"):
                continue
        out.add(n)
    # the generated files' own declarations (gk.Items is a generic func in gen_items.go, which
    # go.names does not record because it is written by gen/gk_go_kinds.py, not claimed)
    import re  # noqa: PLC0415
    for text in go.files.values():
        out.update(re.findall(r"(?m)^(?:func|type|var|const)\s+([A-Z]\w*)", text))
    go._declared = (len(go.files), out)
    return out


def ported_names(it):
    iid = it["id"]
    if iid in PORTED:
        return PORTED[iid]
    best = None
    for k, v in PORTED.items():
        if k.endswith("::") and iid.startswith(k) and (best is None or len(k) > len(best)):
            best = k
    return PORTED[best] if best else None


def covered(go, it, mapped):
    """None when the item counts as in Go (ported or derived from generated items), else the
    reason it is not."""
    k = it.get("kind")
    if k == "summary" and it.get("also"):
        if any(a in mapped for a in it["also"]):
            return None
        return "summary: none of its instantiated objects is in Go yet"
    if k == "projected":
        pre = it["id"] + "#"
        if any(x.startswith(pre) for x in mapped):
            return None
        return "projected: none of its reads is in Go yet"
    if k == "" and it.get("trait") is None and it.get("row"):
        trait, name = it["id"].rsplit("::", 2)[-2:]
        suf = f"::{trait}::{name}"
        if any(x.endswith(suf) for x in mapped):
            return None
        return "trait row: no element type's instance is in Go yet"
    if it["id"] in UNBOUND:
        return "not bound: " + UNBOUND[it["id"]]
    names = ported_names(it)
    if names is None:
        return f"kind {k}: a hand-written bridge or protocol the Go runtime does not have yet"
    have = _names(go)
    missing = [n for n in names if n not in have]
    if not missing:
        return None
    return f"kind {k}: the Go runtime does not declare {', '.join('gk.' + n for n in missing)} yet"


# ---------------------------------------------------------------- what the generated package adds

def focus_trap_elements(go):
    """FocusTrapElement: the elements gk.FocusTrap and gk.TextSelectionScopeOf wrap."""
    import gk_values  # noqa: PLC0415
    _, trap = gk_values.animatable(go.m)
    names = sorted({go.m.nodes[k]["go"] for k, _ in trap if "go" in go.m.nodes[k]})
    out = ["// FocusTrapElement is an element gk.FocusTrap may wrap: a parent, styled, a gpui Element (PROTOCOLS.md 13.30).",
           "type FocusTrapElement interface {", "\tElement", "\tisFocusTrapElement()", "}", ""]
    out += [f"func ({n}) isFocusTrapElement() {{}}" for n in names]
    return out + [""]


def emit(go):
    """gen_bridges.go: what this module adds to the generated package."""
    lines = focus_trap_elements(go)
    go.open("gen_bridges.go", "What the hand-written bridges need of the generated package (gen/gk_go_bridges.py).")
    go.add(*lines)
    go.close()

