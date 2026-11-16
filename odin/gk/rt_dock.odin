package gk

// Dock panels as Odin views (docs/PROTOCOLS.md section 11.7). A panel is an ordinary view the app
// made with gk.view, put in a dock by the generated DockArea and DockLayout procedures:
//
//   files := gk.panel(cx, gk.view(a, nil, files_view), {name = "files", title = "Files"}, gk.on(panel_event))
//   dock := gk.dock_area_skinned(cx, "main")
//   dock->set_center(cx, gk.dock_layout_tabs()->panel(files)->panel(editor))
//   gk.dock_view(dock)                                    // in a view procedure: the dock as an element
//
// The dock asks a panel its name, title and flags while it draws: the library answers from the
// data the app gave (gk.panel_set changes it). What the dock tells a panel (active, zoomed, added
// to a tab group, removed) reaches the app's handler as a Panel_Event, after the dock's update.

// Panel_Data is what the dock reads. The flags are upstream's defaults when zero.
Panel_Data :: struct {
	name:         string, // panel_name: the name a saved layout restores it by
	tab_name:     Maybe(string), // the tab's label, else the title
	title:        string,
	not_closable: bool,
	not_zoomable: bool,
	hidden:       bool,
	no_padding:   bool, // inner_padding false
	no_title_bar: bool, // title_bar false
	// PROTOCOLS.md 11.8, zero for upstream's defaults:
	suffix:       View, // drawn at the end of the title bar (Panel::title_suffix)
	toolbar:      View, // a view whose root's gk.button children are the toolbar (Panel::toolbar_buttons)
	menu:         Built(Popup_Menu), // applied to the title bar's menu (Panel::dropdown_menu)
	info:         string, // JSON: the panel's own data in a saved layout ("" for none)
}

Panel_Event_Kind :: enum u8 {
	Active  = 0, // flag: made active (true) or not
	Zoomed  = 1, // flag: zoomed in (true) or out
	Added   = 2, // added to a tab group
	Removed = 3, // removed from the dock
}

// Panel_Event is the payload of a panel's events handler.
Panel_Event :: struct {
	kind: Panel_Event_Kind,
	flag: bool,
}

// Panel_Emit is what a panel tells the dock (upstream's PanelEvent).
Panel_Emit :: enum u8 {
	Zoom_In        = 0,
	Zoom_Out       = 1,
	Layout_Changed = 2,
}

@(init, private)
dock_shapes :: proc "contextless" () {
	register_shape(SHAPE_PANEL_EVENT, Panel_Event, nil)
}

@(private)
put_panel_data :: proc(o: ^Tape, d: Panel_Data) {
	put_str(o, d.name)
	if t, ok := d.tab_name.?; ok {
		put_u8(o, 1)
		put_str(o, t)
	} else {
		put_u8(o, 0)
	}
	put_str(o, d.title)
	flags: u8
	if d.not_closable {flags |= 1}
	if d.not_zoomable {flags |= 2}
	if d.hidden {flags |= 4}
	if d.no_padding {flags |= 8}
	if d.no_title_bar {flags |= 16}
	put_u8(o, flags)
	put_u64(o, d.suffix.h)
	put_u64(o, d.toolbar.h)
	put_u64(o, d.menu.h)
	put_str(o, d.info)
}

// panel makes a dock panel that draws `v`. `events`, if given, gets each Panel_Event (a handler
// that may take the Panel_Event). In a handler or init (it needs the window).
panel :: proc(cx: ^Cx, v: View, data: Panel_Data, events: Handler = {}, loc := #caller_location) -> Dock_Panel {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.panel in a view procedure: make panels in init or in a handler")
	}
	if data.name == "" {
		stop(loc, "a panel needs a name (Panel_Data.name): a saved layout restores panels by it")
	}
	flush(t, loc)
	slot, gen: u32
	if events.fn != nil {
		slot, gen = keep_handler(cx, events, SHAPE_PANEL_EVENT, loc)
	}
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_PANEL_NEW)
	put_u64(&req, v.h)
	put_panel_data(&req, data)
	put_u32(&req, slot)
	put_u32(&req, gen)
	r := Reader{must_call(req[:], loc), 0}
	return Dock_Panel{rd_u64(&r)}
}

// panel_set changes what the dock reads of a panel; the dock draws it again.
panel_set :: proc(cx: ^Cx, p: Dock_Panel, data: Panel_Data, loc := #caller_location) {
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_PANEL_SET)
	put_u64(&req, p.h)
	put_panel_data(&req, data)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// panel_emit tells the dock what upstream's panels emit (zoom in, zoom out, layout changed).
panel_emit :: proc(cx: ^Cx, p: Dock_Panel, e: Panel_Emit, loc := #caller_location) {
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_PANEL_EMIT)
	put_u64(&req, p.h)
	put_u8(&req, u8(e))
	defer_op(check_cx(cx, loc), req[:], loc)
}

// dock_area_skinned makes the kit's styled dock area (DockSkin::dock_area). gk.dock_area is the
// bare one.
dock_area_skinned :: proc(cx: ^Cx, id: string, version: Maybe(int) = nil, loc := #caller_location) -> Dock_Area {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.dock_area_skinned in a view procedure: make it in init or in a handler")
	}
	flush(t, loc)
	req := make(Tape, 0, 32 + len(id), context.temp_allocator)
	put_u16(&req, C_DOCK_AREA_SKINNED)
	put_str(&req, id)
	ver, has := version.?
	put_u8(&req, u8(has))
	put_u64(&req, u64(ver))
	r := Reader{must_call(req[:], loc), 0}
	return Dock_Area{state = {h = rd_u64(&r)}, vt = &dock_area_vt}
}

// dock_view draws a dock area where the view puts it.
dock_view :: proc(d: Dock_Area, loc := #caller_location) -> Element {
	e := new_node(N_DOCK_VIEW, loc)
	put_u64(&build.ops, d.h)
	return e
}

// --- docs/PROTOCOLS.md 11.8: what the dock hands out ------------------------------------

// Node_Id names a container of a dock's layout (a split or a tab group): only ids the dock handed
// out (dock->zoomed_group, tree->node_ids, group->node, ...) are taken back.
Node_Id :: distinct u64
// Dock_Panel_Id is a panel's PanelId (gk.panel_id): the panel entity's id.
Dock_Panel_Id :: distinct u64

// panel_group is the tab group the dock put panel `p` in (Panel::on_added_to), false when none.
panel_group :: proc(cx: ^Cx, p: Dock_Panel, loc := #caller_location) -> (Tab_Group, bool) {
	t := check_cx(cx, loc)
	flush(t, loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_PANEL_GROUP)
	put_u64(&req, p.h)
	r := Reader{must_call(req[:], loc), 0}
	h := rd_u64(&r)
	if h == 0 {
		return {}, false
	}
	return Tab_Group{state = {h = h}, vt = &tab_group_vt}, true
}

// panel_id is the PanelId of panel `p`, for the dock's calls that name a panel (select_panel).
panel_id :: proc(cx: ^Cx, p: Dock_Panel, loc := #caller_location) -> Dock_Panel_Id {
	t := check_cx(cx, loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_PANEL_ID)
	put_u64(&req, p.h)
	r := Reader{must_call(req[:], loc), 0}
	return Dock_Panel_Id(rd_u64(&r))
}

// --- saved layouts (docs/PROTOCOLS.md 11.8) ------------------------------------------------------
//
//   saved := dock->dump(cx)                                  // a Dock_Area_State: plain values
//   gk.register_panel(cx, "files", gk.on(restore))          // once, before a load
//   err, failed := dock->load(cx, saved).?                    // panels come back through the registry
//   restore :: proc(a: ^App, cx: ^gk.Cx, b: gk.Panel_Build) { // asked between frames, once per panel
//       gk.panel_fill(cx, b.panel, gk.view(..), {name = b.name, title = ..})
//   }
//
// A restored panel draws nothing until the app fills it: one frame at most.

// Panel_State is a node of a saved layout: a split ("stack"), a tab group ("tabs") or a panel.
Panel_State :: struct {
	panel_name: string,
	children:   []Panel_State,
	info:       Panel_Info,
}

Panel_Info :: union {
	Panel_Info_Stack,
	Panel_Info_Tabs,
	Panel_Info_Panel,
}
Panel_Info_Stack :: struct {
	sizes: []f32,
	axis:  int, // 0 horizontal, 1 vertical
}
Panel_Info_Tabs :: struct {
	active_index: int,
}
// Panel_Info_Panel is a panel's own data: JSON text ("" for none). Panel_Data.info sets it.
Panel_Info_Panel :: struct {
	json: string,
}

// Dock_Saved is a saved dock (upstream's DockState as a value).
Dock_Saved :: struct {
	panel:     Panel_State,
	placement: Dock_Placement,
	size:      f32,
	open:      bool,
}

// Dock_Area_State is a whole saved dock area (DockArea::dump, DockArea::load).
Dock_Area_State :: struct {
	version:     Maybe(int),
	center:      Panel_State,
	left_dock:   Maybe(Dock_Saved),
	right_dock:  Maybe(Dock_Saved),
	bottom_dock: Maybe(Dock_Saved),
}

// Panel_Build asks the app for a panel a load restores: fill `panel` with gk.panel_fill.
Panel_Build :: struct {
	panel:       Dock_Panel,
	name:        string,
	state:       Panel_State, // its saved state; state.info is its Panel_Info_Panel when it saved one
	dock_handle: u64, // the dock area it goes into (gk.dock_area_of)
}

@(init, private)
dock_layout_shapes :: proc "contextless" () {
	register_shape(SHAPE_PANEL_BUILD, Panel_Build, nil)
}

// dock_area_of is the dock area a Panel_Build names.
dock_area_of :: proc(h: u64) -> Dock_Area {
	return Dock_Area{state = {h = h}, vt = &dock_area_vt}
}

// register_panel makes the panel registry build panels named `name`: each asks `h` for its view
// with a Panel_Build (queued: after the load, before the next frame). In init or a handler.
register_panel :: proc(cx: ^Cx, name: string, h: Handler, loc := #caller_location) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.register_panel in a view procedure: register panels in init or in a handler")
	}
	flush(t, loc)
	slot, gen := keep_handler(cx, h, SHAPE_PANEL_BUILD, loc)
	req := make(Tape, 0, 32 + len(name), context.temp_allocator)
	put_u16(&req, C_PANEL_REGISTER)
	put_str(&req, name)
	put_u32(&req, slot)
	put_u32(&req, gen)
	must_call(req[:], loc)
}

// panel_fill gives a requested panel its view, data and events handler (buffered).
panel_fill :: proc(cx: ^Cx, p: Dock_Panel, v: View, data: Panel_Data, events: Handler = {}, loc := #caller_location) {
	t := check_cx(cx, loc)
	slot, gen: u32
	if events.fn != nil {
		slot, gen = keep_handler(cx, events, SHAPE_PANEL_EVENT, loc)
	}
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_PANEL_FILL)
	put_u64(&req, p.h)
	put_u64(&req, v.h)
	put_panel_data(&req, data)
	put_u32(&req, slot)
	put_u32(&req, gen)
	defer_op(t, req[:], loc)
}

// build_panel builds a panel through the registry (PanelRegistry::build_panel); false when no
// builder answers to state.panel_name.
build_panel :: proc(cx: ^Cx, dock: Dock_Area, state: Panel_State, loc := #caller_location) -> (Dock_Panel, bool) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.build_panel in a view procedure")
	}
	flush(t, loc)
	req := make(Tape, 0, 128, context.temp_allocator)
	put_u16(&req, C_PANEL_BUILD_NOW)
	put_u64(&req, dock.h)
	st := state
	put_value(&req, &st, type_info_of(Panel_State))
	r := Reader{must_call(req[:], loc), 0}
	if rd_u8(&r) == 0 {
		return {}, false
	}
	h := rd_u64(&r)
	return Dock_Panel{h}, h != 0
}

// pane_tree_to_state writes a pane tree out (PaneTree::to_state), reading its panels from `dock`.
pane_tree_to_state :: proc(cx: ^Cx, tree: Pane_Tree, dock: Dock_Area, loc := #caller_location) -> Panel_State {
	t := check_cx(cx, loc)
	flush(t, loc)
	req := make(Tape, 0, 32, context.temp_allocator)
	put_u16(&req, C_PANE_TREE_TO_STATE)
	put_u64(&req, tree.h)
	put_u64(&req, dock.h)
	r := Reader{must_call(req[:], loc), 0}
	out: Panel_State
	rd_value(&r, &out, type_info_of(Panel_State))
	return out
}

// pane_tree_from_state reads a pane tree back (PaneTree::from_state), its panels built through
// the registry. The tree is kept until gk.release.
pane_tree_from_state :: proc(cx: ^Cx, dock: Dock_Area, state: Panel_State, root: Root_Kind, loc := #caller_location) -> Pane_Tree {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.pane_tree_from_state in a view procedure")
	}
	flush(t, loc)
	req := make(Tape, 0, 128, context.temp_allocator)
	put_u16(&req, C_PANE_TREE_FROM_STATE)
	put_u64(&req, dock.h)
	st := state
	put_value(&req, &st, type_info_of(Panel_State))
	put_u16(&req, u16(root))
	r := Reader{must_call(req[:], loc), 0}
	return Pane_Tree{state = {h = rd_u64(&r)}, vt = &pane_tree_vt}
}

// Pane_Kind is a pane node's shape (PaneNode::kind): a split with its children (temporary handles)
// and slot sizes, or a tab group with its panels and active index.
Pane_Kind :: union {
	Pane_Split,
	Pane_Tabs,
}
Pane_Split :: struct {
	axis:     Axis,
	children: []Pane_Node,
	sizes:    []Maybe(f32),
}
Pane_Tabs :: struct {
	panels:    []Dock_Panel_Id,
	active_ix: int,
}

pane_node_kind :: proc(cx: ^Cx, node: Pane_Node, loc := #caller_location) -> Pane_Kind {
	t := check_cx(cx, loc)
	flush(t, loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_PANE_NODE_KIND)
	put_u64(&req, node.h)
	r := Reader{must_call(req[:], loc), 0}
	out: Pane_Kind
	rd_value(&r, &out, type_info_of(Pane_Kind))
	return out
}

// pane_node_walk is the node and its descendants in pre-order (PaneNode::walk): temporary handles.
pane_node_walk :: proc(cx: ^Cx, node: Pane_Node, loc := #caller_location) -> []Pane_Node {
	t := check_cx(cx, loc)
	flush(t, loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_PANE_NODE_WALK)
	put_u64(&req, node.h)
	r := Reader{must_call(req[:], loc), 0}
	out: []Pane_Node
	rd_value(&r, &out, type_info_of([]Pane_Node))
	return out
}

// --- the skin of a dock area made by gk.dock_area_skinned (DockSkin's reads and settings) ---------

@(private)
skin_req :: proc(dock: Dock_Area, what: u8) -> Tape {
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_DOCK_SKIN)
	put_u64(&req, dock.h)
	put_u8(&req, what)
	return req
}

dock_skin_panel_style :: proc(cx: ^Cx, dock: Dock_Area, loc := #caller_location) -> Panel_Style {
	flush(check_cx(cx, loc), loc)
	req := skin_req(dock, 0)
	r := Reader{must_call(req[:], loc), 0}
	return Panel_Style(rd_u16(&r))
}

dock_skin_set_panel_style :: proc(cx: ^Cx, dock: Dock_Area, style: Panel_Style, loc := #caller_location) {
	req := skin_req(dock, 1)
	put_u16(&req, u16(style))
	defer_op(check_cx(cx, loc), req[:], loc)
}

dock_skin_toggle_button_visible :: proc(cx: ^Cx, dock: Dock_Area, loc := #caller_location) -> bool {
	flush(check_cx(cx, loc), loc)
	req := skin_req(dock, 2)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u8(&r) != 0
}

dock_skin_set_toggle_button_visible :: proc(cx: ^Cx, dock: Dock_Area, visible: bool, loc := #caller_location) {
	req := skin_req(dock, 3)
	put_u8(&req, u8(visible))
	defer_op(check_cx(cx, loc), req[:], loc)
}

dock_skin_set_close_button_visible :: proc(cx: ^Cx, dock: Dock_Area, visible: bool, loc := #caller_location) {
	req := skin_req(dock, 4)
	put_u8(&req, u8(visible))
	defer_op(check_cx(cx, loc), req[:], loc)
}

// --- docs/PROTOCOLS.md 11.9: renderers -----------------------------------------------------
//
//   r := gk.dock_renderer(cx, {.Dock, .Tab_Bar}, gk.on(asked))     // in init or a handler
//   dock := gk.dock_area(cx, "main", nil)->with_renderer(r)->build()
//   asked :: proc(a: ^App, cx: ^gk.Cx, q: gk.Dock_Render) {          // between frames, once per key and context
//       switch q.hook {
//       case .Dock:    gk.dock_render_answer(cx, q.renderer, q.hook, q.key, gk.view(a, nil, dock_chrome))
//       case .Tab_Bar: gk.dock_render_answer(cx, q.renderer, q.hook, q.key, gk.view(..))
//       }
//   }
//
// A hook the app answers draws the app's view for its key: a dock's placement, a tab group's or a
// split's node id. Until the answer arrives, and for hooks not in the set, the dock draws upstream's
// default (one frame at most). A key is asked again when its context changes (a dock opened or
// resized, a tab added or selected); the older answer is drawn meanwhile. The element the kit hands
// a hook (a dock's content, the active panel) goes where the view puts gk.dock_slot(). A frame hook's
// view returns a gk.div() (h_flex, v_flex): upstream's frame is that div with upstream's id.
// q.dock and q.group are the contexts the kit drew with (their reads and calls: dock->toggle(cx),
// group->select_tab(cx, i)), valid until the next request for the same key.

Dock_Hook :: enum u8 {
	Frame              = 0, // the area's outer frame (DockAreaRenderer::frame)
	Center_Frame       = 1, // the column of the center and the bottom dock
	Split_Frame        = 2, // a split's frame; key: its node
	Split_Handle       = 3, // a divider's painted part; key: axis + 2 * dragging
	Dock               = 4, // a dock's chrome around its content (the slot); key: placement
	Placeholder        = 5, // a panel no registered builder makes: fill q.panel with gk.panel_fill
	Tab_Frame          = 6, // a tab group's frame; key: its node
	Tab_Content_Frame  = 7, // the frame around the active panel
	Tab_Bar            = 8, // the tab bar
	Tab_Active_Panel   = 9, // around the active panel (the slot)
	Tab_Drop_Indicator = 10, // drawn where a dragged panel would land
	Tab_Empty          = 11, // a group with no panels
}
Dock_Hooks :: bit_set[Dock_Hook;u32]

// Dock_Renderer is the library's renderer of a dock area and its tab groups (a library object).
Dock_Renderer :: struct {
	h: u64,
}

// Dock_Render asks the app for its view of one hook and key (queued).
Dock_Render :: struct {
	renderer:   Dock_Renderer,
	hook:       Dock_Hook,
	key:        u64,
	node:       Node_Id, // the split's or the tab group's node
	placement:  Dock_Placement, // .Dock
	axis:       Axis, // .Split_Frame, .Split_Handle
	flags:      u8, // .Dock: 1 open, 2 collapsible; tab hooks: 1 zoomed, 2 collapsed, 4 closable, 8 locked; .Split_Handle: 1 dragging
	size:       f32, // .Dock: its extent along its axis
	dock:       Dock_Context, // .Dock
	group:      Tab_Group_Context, // tab hooks
	panel:      Dock_Panel, // .Placeholder
	panel_name: string, // .Placeholder
}

@(init, private)
dock_render_shapes :: proc "contextless" () {
	register_shape(SHAPE_DOCK_RENDER, Dock_Render, nil)
}

// dock_renderer makes a renderer answering `hooks` through `h` (a Dock_Render handler). Give it to
// DockArea's or TabGroup's with_renderer. In init or a handler; gk.release(cx, r.h) frees it.
dock_renderer :: proc(cx: ^Cx, hooks: Dock_Hooks, h: Handler, loc := #caller_location) -> Dock_Renderer {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.dock_renderer in a view procedure: make it in init or in a handler")
	}
	flush(t, loc)
	slot, gen := keep_handler(cx, h, SHAPE_DOCK_RENDER, loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_DOCK_RENDERER_NEW)
	put_u32(&req, transmute(u32)hooks)
	put_u32(&req, slot)
	put_u32(&req, gen)
	r := Reader{must_call(req[:], loc), 0}
	return Dock_Renderer{rd_u64(&r)}
}

// dock_render_answer gives the view drawn for (hook, key); a zero View gives upstream's default back.
// Buffered; every window draws again.
dock_render_answer :: proc(cx: ^Cx, r: Dock_Renderer, hook: Dock_Hook, key: u64, v: View, loc := #caller_location) {
	req := make(Tape, 0, 32, context.temp_allocator)
	put_u16(&req, C_DOCK_RENDER_ANSWER)
	put_u64(&req, r.h)
	put_u8(&req, u8(hook))
	put_u64(&req, key)
	put_u64(&req, v.h)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// dock_slot is where a renderer's view puts the element the kit handed the hook (a dock's content,
// the active panel). Nothing in a view drawn by no hook, or by one that hands no element.
dock_slot :: proc(loc := #caller_location) -> Element {
	return new_node(N_DOCK_SLOT, loc)
}

// test_dock_render: requests sent, answers kept, hooks drawn from an answer (the tests').
test_dock_render :: proc(cx: ^Cx, r: Dock_Renderer, loc := #caller_location) -> (sent, answers, drawn: int) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_DOCK_RENDER_TEST)
	put_u64(&req, r.h)
	rd := Reader{must_call(req[:], loc), 0}
	return int(rd_u32(&rd)), int(rd_u32(&rd)), int(rd_u32(&rd))
}

// --- a dock opened or closed by the user (the kit's DockToggleEvent) ---------------------------

// Dock_Toggle is what the kit reports when the user opens or closes a dock through the skin's toggle.
Dock_Toggle :: struct {
	placement: Dock_Placement,
	open:      bool,
}

@(init, private)
dock_toggle_shapes :: proc "contextless" () {
	register_shape(SHAPE_DOCK_TOGGLE, Dock_Toggle, nil)
}

// on_dock_toggle subscribes `h` (a Dock_Toggle handler) to the dock area's toggles. In init or a handler.
on_dock_toggle :: proc(cx: ^Cx, dock: Dock_Area, h: Handler, loc := #caller_location) -> Subscription {
	slot, generation := keep_handler(cx, h, SHAPE_DOCK_TOGGLE, loc)
	req := make(Tape, 0, 32, context.temp_allocator)
	put_u16(&req, C_DOCK_TOGGLE_SUBSCRIBE)
	put_u64(&req, dock.h)
	put_u32(&req, slot)
	put_u32(&req, generation)
	r := state_get(dock.h, cx, req[:], loc)
	return {rd_u64(&r), slot}
}
