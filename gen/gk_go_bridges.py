"""Go-side dock wire codecs, bridge coverage accounting, and FocusTrapElement emission.

This module bridges two concerns between the generated Go code and the hand-written
Go runtime (`go/gk/` and `go/gk/internal/rt/`):

1. **Dock wire codecs (`gtype`, `put`, `rd`)**: Maps `dock` descriptors produced by
   `gen/gk_overlay.py` into Go types (`NodeID`, `DockPanelID`, `View`, `DockAreaState`,
   `PanelState`, `PanelInfo`, `DockRenderer`, `time.Duration`, and dock state handles)
   and emits the corresponding tape writes and reply reads.
2. **Bridge coverage accounting (`UNBOUND`, `PORTED`, `covered`)**: Every upstream item
   classified as `bridged` (via Rust shims or custom protocols rather than direct
   generated decoders) must be accounted for when `gen/gk_go.py` computes Go coverage:
   - `UNBOUND`: bridged items deliberately omitted from Go because no generated Rust
     element decoder routes to them (documented explicitly in `gen/out/go-coverage.md`).
   - `PORTED`: maps each bridged upstream item ID (or `Trait::` prefix) to the tuple of
     exported Go symbols that implement the bridge. `covered()` verifies at generation
     time that every listed symbol actually exists in the Go package, catching missing
     or renamed bridges immediately.
3. **Bridge marker emission (`focus_trap_elements`, `emit`)**: Writes `gen_bridges.go`
   containing the `FocusTrapElement` marker interface for every styled parent element
   eligible for `FocusTrap` in `gk_values.animatable()`.
"""

# Dock layout snapshot structs serialized by hand-written put*/rd* helpers in go/gk/dock.
DOCK_LAYOUT = {"area_state": "DockAreaState", "panel_state": "PanelState", "panel_info": "PanelInfo"}


def gtype(go, d, out=False):
    """Map a `dock` descriptor (`["dock", kind, ...]`) to its Go type name.

    Handles opaque IDs (`node_id`, `panel_id`), runtime handles (`view`, `panel`,
    `entity`, `renderer`), nanosecond/millisecond durations (`ms`, `instant`), and
    layout snapshot structs (`area_state`, `panel_state`, `panel_info`). Raises
    `gk_go.Unsup` if a referenced dock entity type was not emitted into Go.
    """
    w = d[1]
    if w == "node_id":
        return "NodeID"
    if w == "panel_id":
        return "DockPanelID"
    if w == "view":
        return "View"
    if w == "panel":
        import gk_overlay
        return go.gtype(["entity", gk_overlay.PANEL_RUST, gk_overlay.PANEL_NAME], out)
    if w in ("ms", "instant"):
        return "time.Duration"
    if w == "entity":
        st = go.m.states.get(d[3], {})
        if "go" not in st:
            from gk_go import Unsup
            raise Unsup(f"dock entity {d[3]} has no Go type")
        return st["go"]
    if w in DOCK_LAYOUT:
        return DOCK_LAYOUT[w]
    if w == "renderer":
        return "DockRenderer"
    from gk_go import Unsup
    raise Unsup(f"dock {w}")


def put(go, d, v):
    """Return Go tape-encoding statements for an inbound `dock` descriptor.

    IDs and timestamps travel as `u64`, handle wrappers (`view`, `renderer`) send
    their raw handle `.h`, and layout snapshots delegate to hand-written `put*`
    helpers. Read-only dock kinds (`panel`, `entity`, `ms`) raise `Unsup`.
    """
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
    from gk_go import Unsup
    raise Unsup(f"dock {w} is only read back")


def rd(go, d, out=True):
    """Return a Go wire-decoding expression for an outbound `dock` descriptor.

    Decodes raw `u64` IDs, handle structs (`{h: r.u64()}`), `time.Duration` values
    (encoded as signed nanoseconds/milliseconds in `u64`), and layout snapshots.
    """
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
    from gk_go import Unsup
    raise Unsup(f"dock {w} read back")


# Bridged upstream items intentionally left unbound in Go because no generated Rust decoder
# routes to them (reported explicitly in gen/out/go-coverage.md rather than silently dropped).
# StyleSized trait ops live in core/src/gen/traits.rs, but no element's generated decoder calls
# tr_style_sized_53; VirtualList is handled by core/src/source.rs with style-only ops.
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


# Mapping from bridged upstream item IDs (or `Trait::` prefixes) to the exported Go symbols
# that prove the bridge is implemented in the hand-written Go runtime. Entries are checked
# against actual declared Go symbols by `covered()`, so renaming or removing a bridge entry
# point in Go immediately surfaces in gen/out/go-report.md.
#
# Grouped by subsystem:
# - Window/menu/shortcut shims (TitleBar, GlobalState menus, DropdownMenu, Kbd, AppMenuBar)
# - Animation, motion, focus trap, text selection scope, and virtual lists
# - Dock panels, registry, pane tree serialization, DockSkin, and custom dock renderers
# - Virtualized list/table/searchable-list delegates
# - Editor LSP/highlighting/markdown/questionnaire/item-callback bridges
_DOCK_PANEL = ("NewPanel", "SetPanel", "EmitPanel", "PanelEvents")
PORTED = {
    "gpui_kit::component::TitleBar::window_options": ("WindowConfig", "OpenWindowWith"),
    "gpui_kit::base::GlobalState::app_menus": ("Menus",),
    "gpui_kit::base::GlobalState::set_app_menus": ("SetMenus",),
    "gpui_kit::component::menu::DropdownMenu::dropdown_menu": ("DropdownMenu",),
    "gpui_kit::component::menu::DropdownMenu::dropdown_menu_with_anchor": ("DropdownMenuAnchored",),
    "gpui_kit::component::kbd::Kbd::binding_for_action": ("KbdForAction",),
    "gpui_kit::component::kbd::Kbd::binding_for_action_in": ("KbdForActionIn",),
    "gpui_kit::component::kbd::Kbd::global_binding_for_action": ("KbdGlobalForAction",),
    "gpui_kit::component::menu::AppMenuBar::new": ("NewAppMenuBar", "AppMenuBarView"),
    "gpui_kit::base::animation::EffectTransition::apply": ("Effect",),
    "gpui_kit::base::animate_keyframes": ("Motion", "AtKeyframes"),
    "gpui_kit::base::spring": ("Motion", "ToSpring"),
    "gpui_kit::base::transition": ("Motion", "ToTransition"),
    "gpui_kit::base::Presence::sample": ("Motion", "AtPresence"),
    "gpui_kit::base::animation::cubic_bezier": ("Motion", "EaseCubicBezier"),
    "gpui_kit::base::FocusTrapElement::focus_trap": ("FocusTrap",),
    "gpui_kit::base::ElementExt::text_selection_scope": ("TextSelectionScopeOf",),
    "gpui_kit::base::TextSelectionScopeId::new": ("NewTextSelectionScopeID",),
    "gpui_kit::base::v_virtual_list": ("VVirtualList",),
    "gpui_kit::base::h_virtual_list": ("HVirtualList",),
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
    "gpui_kit::component::list::ListDelegate::": ("NewListSource", "ListEvents", "SetDelegateData"),
    "gpui_kit::component::table::TableDelegate::": ("NewTableSource", "TableEvents", "SetDelegateData", "TableLook",
                                                    "SetTableLook", "SetTableTexts", "SetTableRowLabels", "RowMenu"),
    "gpui_kit::component::searchable_list::SearchableListDelegate::": ("NewSearchSource", "SearchRequests",
                                                                      "SearchWillChange"),
    "gpui_kit::component::searchable_list::SearchableListItem::": ("SearchItem",),
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
    "gpui_kit::base::text::editor_host::collect_leaf_geometry": ("BlockHost",),
    "gpui_kit::base::input::InputEditsEvent::edits": ("InputEditsEvent",),
    "gpui_kit::component::setting::AnySettingField::": ("SettingFieldBool", "SettingFieldNumber", "SettingFieldText"),
    "gpui_kit::component::setting::SettingFieldElement::render_field": ("NewSettingFieldTextElement",),
}


def _names(go):
    """Collect all exported Go identifiers declared in hand-written and generated flat files (cached per emit).

    Cache invalidates whenever `len(go.files)` changes. Skips hand-written files whose
    first line is `//go:build gk_after_gen_merge`, allowing staged bridge files to sit in
    the tree without prematurely counting as ported.
    """
    import os
    cached = getattr(go, "_declared", None)
    if cached is not None and cached[0] == len(go.files):
        return cached[1]
    out = set()
    for n, w in go.names.items():
        if isinstance(w, str) and w.startswith("hand "):
            path = os.path.join(go.out, w[5:])
            # Ignore placeholder files gated behind gk_after_gen_merge until the bridge is merged.
            if os.path.exists(path) and open(path).read().startswith("//go:build gk_after_gen_merge"):
                continue
        out.add(n)
    import re
    for text in go.files.values():
        out.update(re.findall(r"(?m)^(?:func|type|var|const)\s+([A-Z]\w*)", text))
    go._declared = (len(go.files), out)
    return out


def ported_names(it):
    """Return the tuple of required Go runtime symbol names for bridged item `it` from `PORTED`, or None.

    Matches exact item IDs first; otherwise selects the longest matching `Trait::` prefix
    in `PORTED` so more specific overrides take precedence over broad trait entries.
    """
    iid = it["id"]
    if iid in PORTED:
        return PORTED[iid]
    best = None
    for k, v in PORTED.items():
        if k.endswith("::") and iid.startswith(k) and (best is None or len(k) > len(best)):
            best = k
    return PORTED[best] if best else None


def covered(go, it, mapped):
    """Return None if bridged item `it` is covered in Go, or an explanatory reason string.

    Evaluates coverage rules in order:
    1. `summary` items (generic types): covered if at least one concrete instantiation in `also` is mapped.
    2. `projected` items (field/getter projections): covered if any `<id>#<field>` was emitted.
    3. Trait table rows (`kind == ""`): covered if at least one element type emitted `::<Trait>::<method>`.
    4. Explicit `UNBOUND` exclusions: reported with their documented reason.
    5. `PORTED` runtime bridges: covered only when every symbol listed for the item is declared in Go.
    """
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


def focus_trap_elements(go):
    """Emit the FocusTrapElement interface and marker methods for all eligible element types.

    Eligibility comes directly from `gk_values.animatable()` (styled elements implementing
    both `ParentElement` and `Element`, plus `Div`), keeping Go's compile-time constraint
    in lockstep with Rust's `motion_trap` match arms.
    """
    import gk_values
    _, trap = gk_values.animatable(go.m)
    names = sorted({go.m.nodes[k]["go"] for k, _ in trap if "go" in go.m.nodes[k]})
    out = ["// FocusTrapElement marks styled parent elements that can be wrapped by FocusTrap.",
           "type FocusTrapElement interface {", "\tElement", "\tisFocusTrapElement()", "}", ""]
    out += [f"func ({n}) isFocusTrapElement() {{}}" for n in names]
    return out + [""]


def emit(go):
    """Emit gen_bridges.go with generated marker interfaces required by hand-written Go bridges."""
    lines = focus_trap_elements(go)
    go.open("gen_bridges.go", "Generated marker interfaces used by hand-written Go bridges.")
    go.add(*lines)
    go.close()

