package gk

// Dock panels as Go views (docs/PROTOCOLS.md section 11.7), ported from the first part
// of odin/gk/rt_dock.odin. A panel is an ordinary view the app made with gk.NewView, put in a dock by the
// generated DockArea and DockLayout methods:
//
//	files := gk.NewPanel(cx, gk.NewView(nil, a.filesView), gk.PanelData{Name: "files", Title: "Files"})
//	a.dock = gk.NewDockAreaSkinned(cx, "main")
//	a.dock.SetCenter(cx, gk.NewDockLayoutTabs().PanelView(files))
//	gk.DockView(a.dock)                         // in a view function: the dock as an element
//
// The dock asks a panel its name, title and flags while it draws: the library answers from the
// data the app gave (gk.SetPanel changes it). What the dock tells a panel (active, zoomed, added
// to a tab group, removed) reaches the app's handler (gk.PanelEvents) as a PanelEvent, after the
// dock's update. Saved layouts, the pane tree, the skin's settings, renderers and toggles follow
// below (the rest of odin/gk/rt_dock.odin).

// PanelData is what the dock reads of a panel. The flags are upstream's defaults when false.
type PanelData struct {
	Name        string         // panel_name: the name a saved layout restores it by
	TabName     Option[string] // the tab's label, else the title
	Title       string
	NotClosable bool
	NotZoomable bool
	Hidden      bool
	NoPadding   bool // inner_padding false
	NoTitleBar  bool // title_bar false
	Suffix      View // drawn at the end of the title bar (Panel::title_suffix)
	Toolbar     View // a view whose root's gk.Button children are the toolbar (Panel::toolbar_buttons)
	Menu        Built[PopupMenu]
	Info        string // JSON: the panel's own data in a saved layout ("" for none)
}

// PanelEventKind is what the dock told a panel.
type PanelEventKind uint8

const (
	PanelActive  PanelEventKind = 0 // Flag: made active (true) or not
	PanelZoomed  PanelEventKind = 1 // Flag: zoomed in (true) or out
	PanelAdded   PanelEventKind = 2 // added to a tab group
	PanelRemoved PanelEventKind = 3 // removed from the dock
)

// PanelEvent is the event of a panel's handler.
type PanelEvent struct {
	Kind PanelEventKind
	Flag bool
}

// PanelEmit is what a panel tells the dock (upstream's PanelEvent).
type PanelEmit uint8

const (
	PanelZoomIn        PanelEmit = 0
	PanelZoomOut       PanelEmit = 1
	PanelLayoutChanged PanelEmit = 2
)

func init() {
	registerShape(shapePanelEvent, shapeInfo{"func(cx *Cx, e PanelEvent)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, PanelEvent))
		if ok {
			f(cx, PanelEvent{PanelEventKind(r.u8()), r.bool()})
		}
		return ok
	}})
}

func putPanelData(o *tape, d PanelData) {
	o.str(d.Name)
	putOptStrOpt(o, d.TabName)
	o.str(d.Title)
	var flags uint8
	for i, b := range []bool{d.NotClosable, d.NotZoomable, d.Hidden, d.NoPadding, d.NoTitleBar} {
		if b {
			flags |= 1 << i
		}
	}
	o.u8(flags)
	o.u64(d.Suffix.h)
	o.u64(d.Toolbar.h)
	o.u64(d.Menu.h)
	o.str(d.Info)
}

func putOptStrOpt(o *tape, s Option[string]) {
	if v, ok := s.Get(); ok {
		o.u8(1)
		o.str(v)
		return
	}
	o.u8(0)
}

// PanelOption changes a panel made by NewPanel.
type PanelOption func(*panelOpts)

type panelOpts struct{ events func(cx *Cx, e PanelEvent) }

// PanelEvents gives a panel the handler of what the dock tells it.
func PanelEvents(f func(cx *Cx, e PanelEvent)) PanelOption {
	return func(o *panelOpts) { o.events = f }
}

// handle makes a panel something gk.Release takes (Odin's gk.release frees a Dock_Panel).
func (p DockPanel) handle() uint64 { return p.h }

// NewPanel makes a dock panel that draws v. In a handler or init (it needs the window).
func NewPanel(cx *Cx, v View, d PanelData, opts ...PanelOption) DockPanel {
	t := checkCx(cx)
	if vb.active {
		stop("gk.NewPanel in a view function: make panels in init or in a handler")
	}
	if d.Name == "" {
		stop("a panel needs a name (PanelData.Name): a saved layout restores panels by it")
	}
	if v.h == 0 {
		stop("gk.NewPanel was given no view: make it with gk.NewView(init, render)")
	}
	var po panelOpts
	for _, o := range opts {
		o(&po)
	}
	t.flush()
	var slot, gen uint32
	if po.events != nil {
		slot, gen = keepHandler(cx, po.events, shapePanelEvent)
	}
	o := newReq(cPanelNew)
	o.u64(v.h)
	putPanelData(o, d)
	o.u32(slot)
	o.u32(gen)
	r := reader{b: mustCall(*o)}
	return DockPanel{r.u64()}
}

// SetPanel changes what the dock reads of a panel; the dock draws it again.
func SetPanel(cx *Cx, p DockPanel, d PanelData) {
	o := newReq(cPanelSet)
	o.u64(p.h)
	putPanelData(o, d)
	checkCx(cx).deferOp(*o)
}

// EmitPanel tells the dock what upstream's panels emit (zoom in, zoom out, layout changed).
func EmitPanel(cx *Cx, p DockPanel, e PanelEmit) {
	o := newReq(cPanelEmit)
	o.u64(p.h)
	o.u8(uint8(e))
	checkCx(cx).deferOp(*o)
}

// NewDockAreaSkinned makes the kit's styled dock area (DockSkin::dock_area); gk.NewDockArea is
// the bare one. In init or a handler.
func NewDockAreaSkinned(cx *Cx, id string) DockArea {
	t := checkCx(cx)
	if vb.active {
		stop("gk.NewDockAreaSkinned in a view function: make it in init or in a handler")
	}
	t.flush()
	o := newReq(cDockAreaSkinned)
	o.str(id)
	o.u8(0)
	o.u64(0)
	r := reader{b: mustCall(*o)}
	return DockArea{r.u64()}
}

// DockView draws a dock area where the view puts it.
func DockView(d DockArea) Element {
	n := newNode(nDockView)
	nodeArgs(n).u64(d.h)
	return valueEl{n}
}

// --- docs/PROTOCOLS.md 11.8: what the dock hands out ------------------------------------

// NodeID names a container of a dock's layout (a split or a tab group): only ids the dock handed
// out (DockArea.ZoomedGroup, PaneTree.NodeIDs, TabGroup.Node, ...) are taken back.
type NodeID uint64

// DockPanelID is a panel's PanelId (gk.PanelID): the panel entity's id.
type DockPanelID uint64

// PanelGroup is the tab group the dock put panel p in (Panel::on_added_to); false when none.
func PanelGroup(cx *Cx, p DockPanel) (TabGroup, bool) {
	checkCx(cx).flush()
	o := newReq(cPanelGroup)
	o.u64(p.h)
	r := reader{b: mustCall(*o)}
	h := r.u64()
	return TabGroup{h}, h != 0
}

// PanelID is the PanelId of panel p, for the dock's calls that name a panel (DockArea.SelectPanel).
func PanelID(cx *Cx, p DockPanel) DockPanelID {
	checkCx(cx)
	o := newReq(cPanelId)
	o.u64(p.h)
	r := reader{b: mustCall(*o)}
	return DockPanelID(r.u64())
}

// --- saved layouts (docs/PROTOCOLS.md 11.8) ------------------------------------------------------
//
//	saved := a.dock.Dump(cx)                              // a gk.DockAreaState: plain values
//	gk.RegisterPanel(cx, "files", a.restore)               // once, before a load
//	if err, failed := a.dock.Load(cx, saved).Get(); failed { ... }
//	func (a *App) restore(cx *gk.Cx, b gk.PanelBuild) {   // between frames, once per panel
//		gk.PanelFill(cx, b.Panel, gk.NewView(nil, a.filesView), gk.PanelData{Name: b.Name, Title: "Files"})
//	}
//
// A restored panel draws nothing until the app fills it: one frame at most.

// PanelState is a node of a saved layout: a split (PanelInfoStack), a tab group (PanelInfoTabs)
// or a panel (PanelInfoPanel).
type PanelState struct {
	PanelName string
	Children  []PanelState
	Info      PanelInfo
}

// PanelInfo is one of PanelInfoStack, PanelInfoTabs, PanelInfoPanel.
type PanelInfo interface{ isPanelInfo() }

// PanelInfoStack is a split: its slot sizes and its axis (0 horizontal, 1 vertical).
type PanelInfoStack struct {
	Sizes []float32
	Axis  int
}

// PanelInfoTabs is a tab group: its active tab.
type PanelInfoTabs struct{ ActiveIndex int }

// PanelInfoPanel is a panel's own data: JSON text ("" for none). PanelData.Info sets it.
type PanelInfoPanel struct{ JSON string }

func (PanelInfoStack) isPanelInfo() {}
func (PanelInfoTabs) isPanelInfo()  {}
func (PanelInfoPanel) isPanelInfo() {}

// DockSaved is a saved dock (upstream's DockState as a value).
type DockSaved struct {
	Panel     PanelState
	Placement DockPlacement
	Size      float32
	Open      bool
}

// DockAreaState is a whole saved dock area (DockArea.Dump, DockArea.Load).
type DockAreaState struct {
	Version    Option[int]
	Center     PanelState
	LeftDock   Option[DockSaved]
	RightDock  Option[DockSaved]
	BottomDock Option[DockSaved]
}

func putPanelInfo(o *tape, i PanelInfo) {
	switch v := i.(type) {
	case PanelInfoStack:
		o.u16(0)
		o.u32(uint32(len(v.Sizes)))
		for _, s := range v.Sizes {
			o.f32(s)
		}
		o.u64(uint64(v.Axis))
	case PanelInfoTabs:
		o.u16(1)
		o.u64(uint64(v.ActiveIndex))
	case PanelInfoPanel:
		o.u16(2)
		o.str(v.JSON)
	default:
		stop("a gk.PanelState has no Info: give a gk.PanelInfoStack, gk.PanelInfoTabs or gk.PanelInfoPanel")
	}
}

func rdPanelInfo(r *reader) PanelInfo {
	switch n := r.u16(); n {
	case 0:
		sizes := rdList(r, func(r *reader) float32 { return r.f32() })
		return PanelInfoStack{sizes, int(r.u64())}
	case 1:
		return PanelInfoTabs{int(r.u64())}
	case 2:
		return PanelInfoPanel{r.str()}
	default:
		stop("a saved panel arrived with info variant %d of 3: the library and this package disagree", n)
	}
	return nil
}

func putPanelState(o *tape, s PanelState) {
	o.str(s.PanelName)
	o.u32(uint32(len(s.Children)))
	for _, c := range s.Children {
		putPanelState(o, c)
	}
	putPanelInfo(o, s.Info)
}

func rdPanelState(r *reader) PanelState {
	var s PanelState
	s.PanelName = r.str()
	s.Children = rdList(r, rdPanelState)
	s.Info = rdPanelInfo(r)
	return s
}

func putDockSaved(o *tape, d Option[DockSaved]) {
	v, ok := d.Get()
	if !ok {
		o.u8(0)
		return
	}
	o.u8(1)
	putPanelState(o, v.Panel)
	o.u16(uint16(v.Placement))
	o.f32(v.Size)
	o.bool(v.Open)
}

func rdDockSaved(r *reader) DockSaved {
	return DockSaved{rdPanelState(r), DockPlacement(r.u16()), r.f32(), r.bool()}
}

func putDockAreaState(o *tape, s DockAreaState) {
	if v, ok := s.Version.Get(); ok {
		o.u8(1)
		o.u64(uint64(v))
	} else {
		o.u8(0)
	}
	putPanelState(o, s.Center)
	putDockSaved(o, s.LeftDock)
	putDockSaved(o, s.RightDock)
	putDockSaved(o, s.BottomDock)
}

func rdDockAreaState(r *reader) DockAreaState {
	var s DockAreaState
	s.Version = rdOpt(r, func(r *reader) int { return int(r.u64()) })
	s.Center = rdPanelState(r)
	s.LeftDock = rdOpt(r, rdDockSaved)
	s.RightDock = rdOpt(r, rdDockSaved)
	s.BottomDock = rdOpt(r, rdDockSaved)
	return s
}

// PanelBuild asks the app for a panel a load restores: fill Panel with gk.PanelFill.
type PanelBuild struct {
	Panel DockPanel
	Name  string
	State PanelState // its saved state; State.Info is its PanelInfoPanel when it saved one
	Dock  DockArea   // the dock area it goes into
}

func init() {
	registerShape(shapePanelBuild, shapeInfo{"func(cx *Cx, b PanelBuild)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, PanelBuild))
		if ok {
			b := PanelBuild{Panel: DockPanel{r.u64()}, Name: r.str()}
			b.State = rdPanelState(r)
			b.Dock = DockArea{r.u64()}
			f(cx, b)
		}
		return ok
	}})
}

// RegisterPanel makes the panel registry build panels named name: each asks f for its view with a
// PanelBuild (queued: after the load, before the next frame). In init or a handler.
func RegisterPanel(cx *Cx, name string, f func(cx *Cx, b PanelBuild)) {
	t := checkCx(cx)
	if vb.active {
		stop("gk.RegisterPanel in a view function: register panels in init or in a handler")
	}
	t.flush()
	slot, gen := keepHandler(cx, f, shapePanelBuild)
	o := newReq(cPanelRegister)
	o.str(name)
	o.u32(slot)
	o.u32(gen)
	mustCall(*o)
}

// PanelFill gives a requested panel its view, data and events handler (buffered).
func PanelFill(cx *Cx, p DockPanel, v View, d PanelData, opts ...PanelOption) {
	t := checkCx(cx)
	var po panelOpts
	for _, o := range opts {
		o(&po)
	}
	var slot, gen uint32
	if po.events != nil {
		slot, gen = keepHandler(cx, po.events, shapePanelEvent)
	}
	o := newReq(cPanelFill)
	o.u64(p.h)
	o.u64(v.h)
	putPanelData(o, d)
	o.u32(slot)
	o.u32(gen)
	t.deferOp(*o)
}

// BuildPanel builds a panel through the registry (PanelRegistry::build_panel); false when no
// builder answers to state.PanelName.
func BuildPanel(cx *Cx, dock DockArea, state PanelState) (DockPanel, bool) {
	t := checkCx(cx)
	if vb.active {
		stop("gk.BuildPanel in a view function")
	}
	t.flush()
	o := newReq(cPanelBuildNow)
	o.u64(dock.h)
	putPanelState(o, state)
	r := reader{b: mustCall(*o)}
	if r.u8() == 0 {
		return DockPanel{}, false
	}
	h := r.u64()
	return DockPanel{h}, h != 0
}

// PaneTreeToState writes a pane tree out (PaneTree::to_state), reading its panels from dock.
func PaneTreeToState(cx *Cx, tree PaneTree, dock DockArea) PanelState {
	checkCx(cx).flush()
	o := newReq(cPaneTreeToState)
	o.u64(tree.h)
	o.u64(dock.h)
	r := reader{b: mustCall(*o)}
	return rdPanelState(&r)
}

// PaneTreeFromState reads a pane tree back (PaneTree::from_state), its panels built through the
// registry. The tree is kept until gk.Release.
func PaneTreeFromState(cx *Cx, dock DockArea, state PanelState, root RootKind) PaneTree {
	t := checkCx(cx)
	if vb.active {
		stop("gk.PaneTreeFromState in a view function")
	}
	t.flush()
	o := newReq(cPaneTreeFromState)
	o.u64(dock.h)
	putPanelState(o, state)
	o.u16(uint16(root))
	r := reader{b: mustCall(*o)}
	return PaneTree{r.u64()}
}

// PaneKind is a pane node's shape (PaneNode::kind): a PaneSplit or a PaneTabs.
type PaneKind interface{ isPaneKind() }

// PaneSplit is a split with its children (temporary handles) and slot sizes.
type PaneSplit struct {
	Axis     Axis
	Children []PaneNode
	Sizes    []Option[float32]
}

// PaneTabs is a tab group with its panels and active index.
type PaneTabs struct {
	Panels   []DockPanelID
	ActiveIx int
}

func (PaneSplit) isPaneKind() {}
func (PaneTabs) isPaneKind()  {}

// PaneNodeKind is the shape of a pane node (PaneNode::kind).
func PaneNodeKind(cx *Cx, node PaneNode) PaneKind {
	checkCx(cx).flush()
	o := newReq(cPaneNodeKind)
	o.u64(node.h)
	r := reader{b: mustCall(*o)}
	if r.u16() == 0 {
		var s PaneSplit
		s.Axis = Axis(r.u16())
		s.Children = rdList(&r, func(r *reader) PaneNode { return PaneNode{r.u64()} })
		s.Sizes = rdList(&r, func(r *reader) Option[float32] { return rdOpt(r, func(r *reader) float32 { return r.f32() }) })
		return s
	}
	var t PaneTabs
	t.Panels = rdList(&r, func(r *reader) DockPanelID { return DockPanelID(r.u64()) })
	t.ActiveIx = int(r.u64())
	return t
}

// PaneNodeWalk is the node and its descendants in pre-order (PaneNode::walk): temporary handles.
func PaneNodeWalk(cx *Cx, node PaneNode) []PaneNode {
	checkCx(cx).flush()
	o := newReq(cPaneNodeWalk)
	o.u64(node.h)
	r := reader{b: mustCall(*o)}
	return rdList(&r, func(r *reader) PaneNode { return PaneNode{r.u64()} })
}

// --- the skin of a dock area made by gk.NewDockAreaSkinned (DockSkin's reads and settings) ------

func skinReq(dock DockArea, what uint8) *tape {
	o := newReq(cDockSkin)
	o.u64(dock.h)
	o.u8(what)
	return o
}

func skinRead(cx *Cx, dock DockArea, what uint8) *reader {
	checkCx(cx).flush()
	return &reader{b: mustCall(*skinReq(dock, what))}
}

func skinSet(cx *Cx, dock DockArea, what uint8, visible bool) {
	o := skinReq(dock, what)
	o.bool(visible)
	checkCx(cx).deferOp(*o)
}

// DockSkinPanelStyle is the skin's panel style (DockSkin::panel_style).
func DockSkinPanelStyle(cx *Cx, dock DockArea) PanelStyle {
	return PanelStyle(skinRead(cx, dock, 0).u16())
}

// SetDockSkinPanelStyle sets the skin's panel style (buffered).
func SetDockSkinPanelStyle(cx *Cx, dock DockArea, style PanelStyle) {
	o := skinReq(dock, 1)
	o.u16(uint16(style))
	checkCx(cx).deferOp(*o)
}

// DockSkinToggleButtonVisible is whether the docks show their toggle button.
func DockSkinToggleButtonVisible(cx *Cx, dock DockArea) bool { return skinRead(cx, dock, 2).bool() }

// SetDockSkinToggleButtonVisible shows or hides the docks' toggle button (buffered).
func SetDockSkinToggleButtonVisible(cx *Cx, dock DockArea, visible bool) {
	skinSet(cx, dock, 3, visible)
}

// SetDockSkinCloseButtonVisible shows or hides the tabs' close button (buffered).
func SetDockSkinCloseButtonVisible(cx *Cx, dock DockArea, visible bool) {
	skinSet(cx, dock, 4, visible)
}

// DockSkinCollapsedTabBarVisible is whether a tab group that has collapsed (its panels hidden)
// still shows its tab bar.
func DockSkinCollapsedTabBarVisible(cx *Cx, dock DockArea) bool { return skinRead(cx, dock, 5).bool() }

// SetDockSkinCollapsedTabBarVisible shows or hides a collapsed tab group's tab bar (buffered).
func SetDockSkinCollapsedTabBarVisible(cx *Cx, dock DockArea, visible bool) {
	skinSet(cx, dock, 6, visible)
}

// DockSkinSplitDividerLineVisible is whether the divider between two split slots rests as a
// one-pixel line (shown by default). Hidden, the indicator still appears on hover and while
// dragging, and the hit area is unchanged.
func DockSkinSplitDividerLineVisible(cx *Cx, dock DockArea) bool { return skinRead(cx, dock, 7).bool() }

// SetDockSkinSplitDividerLineVisible shows or hides the resting divider line (buffered).
func SetDockSkinSplitDividerLineVisible(cx *Cx, dock DockArea, visible bool) {
	skinSet(cx, dock, 8, visible)
}

// --- docs/PROTOCOLS.md 11.9: renderers -----------------------------------------------------
//
//	r := gk.NewDockRenderer(cx, gk.DockHooks(gk.HookDock, gk.HookTabBar), a.asked) // init or a handler
//	a.dock = gk.NewDockArea(cx, "main", ...).WithRenderer(r).Build()
//	func (a *App) asked(cx *gk.Cx, q gk.DockRender) {     // between frames, once per key and context
//		gk.DockRenderAnswer(cx, q.Renderer, q.Hook, q.Key, a.chrome)
//	}
//
// A hook the app answers draws the app's view for its key: a dock's placement, a tab group's or a
// split's node id. Until the answer arrives, and for hooks not in the set, the dock draws upstream's
// default (one frame at most). The element the kit hands a hook (a dock's content, the active panel)
// goes where the view puts gk.DockSlot(). q.Dock and q.Group are the contexts the kit drew with,
// valid until the next request for the same key.

// DockHook is a hook of the dock area's and the tab groups' renderers.
type DockHook uint8

const (
	HookFrame            DockHook = 0  // the area's outer frame (DockAreaRenderer::frame)
	HookCenterFrame      DockHook = 1  // the column of the center and the bottom dock
	HookSplitFrame       DockHook = 2  // a split's frame; key: its node
	HookSplitHandle      DockHook = 3  // a divider's painted part; key: axis + 2 * dragging
	HookDock             DockHook = 4  // a dock's chrome around its content (the slot); key: placement
	HookPlaceholder      DockHook = 5  // a panel no registered builder makes: fill q.Panel with gk.PanelFill
	HookTabFrame         DockHook = 6  // a tab group's frame; key: its node
	HookTabContentFrame  DockHook = 7  // the frame around the active panel
	HookTabBar           DockHook = 8  // the tab bar
	HookTabActivePanel   DockHook = 9  // around the active panel (the slot)
	HookTabDropIndicator DockHook = 10 // drawn where a dragged panel would land
	HookTabEmpty         DockHook = 11 // a group with no panels
)

// DockHookSet is a set of hooks (gk.DockHooks makes one).
type DockHookSet uint32

// DockHooks is the set of the hooks given.
func DockHooks(hooks ...DockHook) DockHookSet {
	var s DockHookSet
	for _, h := range hooks {
		s |= 1 << h
	}
	return s
}

// DockRenderer is the library's renderer of a dock area and its tab groups (a library object).
type DockRenderer struct{ h uint64 }

func (r DockRenderer) handle() uint64 { return r.h }

// DockRender asks the app for its view of one hook and key (queued).
type DockRender struct {
	Renderer  DockRenderer
	Hook      DockHook
	Key       uint64
	Node      NodeID          // the split's or the tab group's node
	Placement DockPlacement   // HookDock
	Axis      Axis            // HookSplitFrame, HookSplitHandle
	Flags     uint8           // HookDock: 1 open, 2 collapsible; tab hooks: 1 zoomed, 2 collapsed, 4 closable, 8 locked; HookSplitHandle: 1 dragging
	Size      float32         // HookDock: its extent along its axis
	Dock      DockContext     // HookDock
	Group     TabGroupContext // tab hooks
	Panel     DockPanel       // HookPlaceholder
	PanelName string          // HookPlaceholder
}

func init() {
	registerShape(shapeDockRender, shapeInfo{"func(cx *Cx, q DockRender)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, DockRender))
		if ok {
			var q DockRender
			q.Renderer = DockRenderer{r.u64()}
			q.Hook = DockHook(r.u8())
			q.Key = r.u64()
			q.Node = NodeID(r.u64())
			q.Placement = DockPlacement(r.u16())
			q.Axis = Axis(r.u16())
			q.Flags = r.u8()
			q.Size = r.f32()
			q.Dock = DockContext{r.u64()}
			q.Group = TabGroupContext{r.u64()}
			q.Panel = DockPanel{r.u64()}
			q.PanelName = r.str()
			f(cx, q)
		}
		return ok
	}})
	registerShape(shapeDockToggle, shapeInfo{"func(cx *Cx, e DockToggle)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, DockToggle))
		if ok {
			f(cx, DockToggle{DockPlacement(r.u16()), r.bool()})
		}
		return ok
	}})
}

// NewDockRenderer makes a renderer answering hooks through f. Give it to DockArea's or
// TabGroup's WithRenderer. In init or a handler; gk.Release frees it.
func NewDockRenderer(cx *Cx, hooks DockHookSet, f func(cx *Cx, q DockRender)) DockRenderer {
	t := checkCx(cx)
	if vb.active {
		stop("gk.NewDockRenderer in a view function: make it in init or in a handler")
	}
	t.flush()
	slot, gen := keepHandler(cx, f, shapeDockRender)
	o := newReq(cDockRendererNew)
	o.u32(uint32(hooks))
	o.u32(slot)
	o.u32(gen)
	r := reader{b: mustCall(*o)}
	return DockRenderer{r.u64()}
}

// DockRenderAnswer gives the view drawn for (hook, key); a zero View gives upstream's default
// back. Buffered; every window draws again.
func DockRenderAnswer(cx *Cx, r DockRenderer, hook DockHook, key uint64, v View) {
	o := newReq(cDockRenderAnswer)
	o.u64(r.h)
	o.u8(uint8(hook))
	o.u64(key)
	o.u64(v.h)
	checkCx(cx).deferOp(*o)
}

// DockSlot is where a renderer's view puts the element the kit handed the hook (a dock's content,
// the active panel). Nothing in a view drawn by no hook, or by one that hands no element.
func DockSlot() Element { return valueEl{newNode(nDockSlot)} }

// TestDockRender: requests sent, answers kept, hooks drawn from an answer (the tests').
func TestDockRender(cx *Cx, r DockRenderer) (sent, answers, drawn int) {
	checkCx(cx).flush()
	o := newReq(cDockRenderTest)
	o.u64(r.h)
	rd := reader{b: mustCall(*o)}
	return int(rd.u32()), int(rd.u32()), int(rd.u32())
}

// --- a dock opened or closed by the user (the kit's DockToggleEvent) ---------------------------

// DockToggle is what the kit reports when the user opens or closes a dock through the skin's toggle.
type DockToggle struct {
	Placement DockPlacement
	Open      bool
}

// OnDockToggle subscribes f to the dock area's toggles. In init or a handler.
func OnDockToggle(cx *Cx, dock DockArea, f func(cx *Cx, e DockToggle)) Subscription {
	slot, gen := keepHandler(cx, f, shapeDockToggle)
	o := newReq(cDockToggleSubscribe)
	o.u64(dock.h)
	o.u32(slot)
	o.u32(gen)
	r := stateGet(dock.h, cx, o)
	return Subscription{h: r.u64(), slot: slot}
}
