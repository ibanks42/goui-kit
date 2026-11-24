package dock

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
	"gpuikit.dev/gk/kit"
)

// PanelData is what the dock reads of a panel. The flags are upstream's defaults when false.
type PanelData struct {
	Name        string            // panel_name: the name a saved layout restores it by
	TabName     gk.Option[string] // the tab's label, else the title
	Title       string
	NotClosable bool
	NotZoomable bool
	Hidden      bool
	NoPadding   bool    // inner_padding false
	NoTitleBar  bool    // title_bar false
	Suffix      gk.View // drawn at the end of the title bar (Panel::title_suffix)
	Toolbar     gk.View // a view whose root's gk.Button children are the toolbar (Panel::toolbar_buttons)
	Menu        gk.Built[kit.PopupMenu]
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
	rt.RegisterShape(rt.ShapePanelEvent, rt.ShapeInfo{Name: "func(cx *Cx, e PanelEvent)", Call: func(h any, cx *gk.Cx, r *rt.Reader, ret *rt.Tape) bool {
		f, ok := h.(func(*gk.Cx, PanelEvent))
		if ok {
			f(cx, PanelEvent{PanelEventKind(r.U8()), r.Bool()})
		}
		return ok
	}})
}

func putPanelData(o *rt.Tape, d PanelData) {
	o.Str(d.Name)
	rt.PutOptStrOpt(o, d.TabName)
	o.Str(d.Title)
	var flags uint8
	for i, b := range []bool{d.NotClosable, d.NotZoomable, d.Hidden, d.NoPadding, d.NoTitleBar} {
		if b {
			flags |= 1 << i
		}
	}
	o.U8(flags)
	o.U64(rt.H(d.Suffix.HCore))
	o.U64(rt.H(d.Toolbar.HCore))
	o.U64(rt.H(d.Menu.HCore))
	o.Str(d.Info)
}

// PanelOption changes a panel made by NewPanel.
type PanelOption func(*panelOpts)

type panelOpts struct {
	events func(cx *gk.Cx, e PanelEvent)
	keep   bool
}

// PanelEvents gives a panel the handler of what the dock tells it.
func PanelEvents(f func(cx *gk.Cx, e PanelEvent)) PanelOption {
	return func(o *panelOpts) { o.events = f }
}

// KeepWhenClosed keeps a panel the dock removed (the user closed its tab) for the app to add
// again: without it the panel is released once it is removed and not added back, and with it its
// view and all the view made (docs/GO.md section 4). gk.Release ends a kept one.
func KeepWhenClosed() PanelOption {
	return func(o *panelOpts) { o.keep = true }
}

// panelLife follows a panel in and out of the dock: removed and not added back before the next
// WAKE turn, it is closed.
type panelLife struct {
	h       uint64
	removed bool
}

func (pl *panelLife) event(cx *gk.Cx, e PanelEvent, keep bool) {
	switch e.Kind {
	case PanelAdded:
		pl.removed = false
	case PanelRemoved:
		if keep || pl.removed {
			return
		}
		pl.removed = true
		// Moved to another tab group, the panel is added back by the dock's same update: the
		// posted check runs after it.
		gk.Post(func(cx *gk.Cx) {
			if pl.removed && rt.Tracked(pl.h) {
				gk.Release(cx, Panel{HCore: rt.MkH(pl.h)})
			}
		})
	}
}

// handle makes a panel something gk.Release takes.
func (p Panel) handle() uint64 { return rt.H(p.HCore) }

// NewPanel makes a dock panel that draws v. In a handler or init (it needs the window).
func NewPanel(cx *gk.Cx, v gk.View, d PanelData, opts ...PanelOption) Panel {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("dock.NewPanel in a view function: make panels in init or in a handler")
	}
	if d.Name == "" {
		rt.Stop("a panel needs a name (PanelData.Name): a saved layout restores panels by it")
	}
	if rt.H(v.HCore) == 0 {
		rt.Stop("dock.NewPanel was given no view: make it with gk.NewView(init, render)")
	}
	var po panelOpts
	for _, o := range opts {
		o(&po)
	}
	t.Flush()
	// The panel hears what the dock tells it, for its own lifetime (panelLife) and the app's
	// handler; with none, the event marks no view.
	pl := &panelLife{}
	user, keep := po.events, po.keep
	slot, gen := rt.KeepHandler(cx, func(cx *gk.Cx, e PanelEvent) {
		pl.event(cx, e, keep)
		if user != nil {
			user(cx, e)
		} else {
			gk.Quiet(cx)
		}
	}, rt.ShapePanelEvent)
	o := rt.NewReq(rt.CPanelNew)
	o.U64(rt.H(v.HCore))
	putPanelData(o, d)
	o.U32(slot)
	o.U32(gen)
	r := rt.Reader{B: rt.MustCall(*o)}
	pl.h = r.U64()
	rt.Made(pl.h)         // owned by the turn's view, released when unreachable (rt_life.go)
	rt.AdoptView(pl.h, v) // the panel owns its view: releasing it releases the view
	return Panel{HCore: rt.MkH(pl.h)}
}

// SetPanel changes what the dock reads of a panel; the dock draws it again.
func SetPanel(cx *gk.Cx, p Panel, d PanelData) {
	o := rt.NewReq(rt.CPanelSet)
	o.U64(rt.H(p.HCore))
	putPanelData(o, d)
	rt.CheckCx(cx).DeferOp(*o)
}

// EmitPanel tells the dock what upstream's panels emit (zoom in, zoom out, layout changed).
func EmitPanel(cx *gk.Cx, p Panel, e PanelEmit) {
	o := rt.NewReq(rt.CPanelEmit)
	o.U64(rt.H(p.HCore))
	o.U8(uint8(e))
	rt.CheckCx(cx).DeferOp(*o)
}

// NewDockAreaSkinned makes the kit's styled dock area (DockSkin::dock_area); gk.NewDockArea is
// the bare one. In init or a handler.
func NewAreaSkinned(cx *gk.Cx, id string) Area {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("dock.NewAreaSkinned in a view function: make it in init or in a handler")
	}
	t.Flush()
	o := rt.NewReq(rt.CDockAreaSkinned)
	o.Str(id)
	o.U8(0)
	o.U64(0)
	r := rt.Reader{B: rt.MustCall(*o)}
	return Area{HCore: rt.MkH(r.U64())}
}

// DockView draws a dock area where the view puts it.
func View(d Area) gk.Element {
	n := rt.NewNode(rt.NDockView)
	rt.NodeArgs(n).U64(rt.H(d.HCore))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// NodeID names a container of a dock's layout (a split or a tab group): only ids the dock handed
// out (DockArea.ZoomedGroup, PaneTree.NodeIDs, TabGroup.Node, ...) are taken back.
type NodeID uint64

// DockPanelID is a panel's PanelId (gk.PanelID): the panel entity's id.
type DockPanelID uint64

// PanelGroup is the tab group the dock put panel p in (Panel::on_added_to); false when none.
func PanelGroup(cx *gk.Cx, p Panel) (TabGroup, bool) {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CPanelGroup)
	o.U64(rt.H(p.HCore))
	r := rt.Reader{B: rt.MustCall(*o)}
	h := r.U64()
	return TabGroup{HCore: rt.MkH(h)}, h != 0
}

// PanelID is the PanelId of panel p, for the dock's calls that name a panel (DockArea.SelectPanel).
func PanelID(cx *gk.Cx, p Panel) DockPanelID {
	rt.CheckCx(cx)
	o := rt.NewReq(rt.CPanelId)
	o.U64(rt.H(p.HCore))
	r := rt.Reader{B: rt.MustCall(*o)}
	return DockPanelID(r.U64())
}

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

func (PanelInfoTabs) isPanelInfo() {}

func (PanelInfoPanel) isPanelInfo() {}

// DockSaved is a saved dock (upstream's DockState as a value).
type Saved struct {
	Panel     PanelState
	Placement Placement
	Size      float32
	Open      bool
}

// DockAreaState is a whole saved dock area (DockArea.Dump, DockArea.Load).
type AreaState struct {
	Version    gk.Option[int]
	Center     PanelState
	LeftDock   gk.Option[Saved]
	RightDock  gk.Option[Saved]
	BottomDock gk.Option[Saved]
}

func putPanelInfo(o *rt.Tape, i PanelInfo) {
	switch v := i.(type) {
	case PanelInfoStack:
		o.U16(0)
		o.U32(uint32(len(v.Sizes)))
		for _, s := range v.Sizes {
			o.F32(s)
		}
		o.U64(uint64(v.Axis))
	case PanelInfoTabs:
		o.U16(1)
		o.U64(uint64(v.ActiveIndex))
	case PanelInfoPanel:
		o.U16(2)
		o.Str(v.JSON)
	default:
		rt.Stop("a dock.PanelState has no Info: give a dock.PanelInfoStack, dock.PanelInfoTabs or dock.PanelInfoPanel")
	}
}

func rdPanelInfo(r *rt.Reader) PanelInfo {
	switch n := r.U16(); n {
	case 0:
		sizes := rt.RdList(r, func(r *rt.Reader) float32 { return r.F32() })
		return PanelInfoStack{sizes, int(r.U64())}
	case 1:
		return PanelInfoTabs{int(r.U64())}
	case 2:
		return PanelInfoPanel{r.Str()}
	default:
		rt.Stop("a saved panel arrived with info variant %d of 3: the library and this package disagree", n)
	}
	return nil
}

func putPanelState(o *rt.Tape, s PanelState) {
	o.Str(s.PanelName)
	o.U32(uint32(len(s.Children)))
	for _, c := range s.Children {
		putPanelState(o, c)
	}
	putPanelInfo(o, s.Info)
}

func rdPanelState(r *rt.Reader) PanelState {
	var s PanelState
	s.PanelName = r.Str()
	s.Children = rt.RdList(r, rdPanelState)
	s.Info = rdPanelInfo(r)
	return s
}

func putDockSaved(o *rt.Tape, d gk.Option[Saved]) {
	v, ok := d.Get()
	if !ok {
		o.U8(0)
		return
	}
	o.U8(1)
	putPanelState(o, v.Panel)
	o.U16(uint16(v.Placement))
	o.F32(v.Size)
	o.Bool(v.Open)
}

func rdDockSaved(r *rt.Reader) Saved {
	return Saved{rdPanelState(r), Placement(r.U16()), r.F32(), r.Bool()}
}

func putDockAreaState(o *rt.Tape, s AreaState) {
	if v, ok := s.Version.Get(); ok {
		o.U8(1)
		o.U64(uint64(v))
	} else {
		o.U8(0)
	}
	putPanelState(o, s.Center)
	putDockSaved(o, s.LeftDock)
	putDockSaved(o, s.RightDock)
	putDockSaved(o, s.BottomDock)
}

func rdDockAreaState(r *rt.Reader) AreaState {
	var s AreaState
	s.Version = rt.RdOpt(r, func(r *rt.Reader) int { return int(r.U64()) })
	s.Center = rdPanelState(r)
	s.LeftDock = rt.RdOpt(r, rdDockSaved)
	s.RightDock = rt.RdOpt(r, rdDockSaved)
	s.BottomDock = rt.RdOpt(r, rdDockSaved)
	return s
}

// PanelBuild asks the app for a panel a load restores: fill Panel with gk.PanelFill.
type PanelBuild struct {
	Panel Panel
	Name  string
	State PanelState // its saved state; State.Info is its PanelInfoPanel when it saved one
	Dock  Area       // the dock area it goes into
}

func init() {
	rt.RegisterShape(rt.ShapePanelBuild, rt.ShapeInfo{Name: "func(cx *Cx, b PanelBuild)", Call: func(h any, cx *gk.Cx, r *rt.Reader, ret *rt.Tape) bool {
		f, ok := h.(func(*gk.Cx, PanelBuild))
		if ok {
			b := PanelBuild{Panel: Panel{HCore: rt.MkH(r.U64())}, Name: r.Str()}
			b.State = rdPanelState(r)
			b.Dock = Area{HCore: rt.MkH(r.U64())}
			f(cx, b)
		}
		return ok
	}})
}

// RegisterPanel makes the panel registry build panels named name: each asks f for its view with a
// PanelBuild (queued: after the load, before the next frame). In init or a handler.
func RegisterPanel(cx *gk.Cx, name string, f func(cx *gk.Cx, b PanelBuild)) {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("dock.RegisterPanel in a view function: register panels in init or in a handler")
	}
	t.Flush()
	slot, gen := rt.KeepHandler(cx, f, rt.ShapePanelBuild)
	o := rt.NewReq(rt.CPanelRegister)
	o.Str(name)
	o.U32(slot)
	o.U32(gen)
	rt.MustCall(*o)
}

// PanelFill gives a requested panel its view, data and events handler (buffered).
func PanelFill(cx *gk.Cx, p Panel, v gk.View, d PanelData, opts ...PanelOption) {
	t := rt.CheckCx(cx)
	var po panelOpts
	for _, o := range opts {
		o(&po)
	}
	var slot, gen uint32
	if po.events != nil {
		slot, gen = rt.KeepHandler(cx, po.events, rt.ShapePanelEvent)
	}
	o := rt.NewReq(rt.CPanelFill)
	o.U64(rt.H(p.HCore))
	o.U64(rt.H(v.HCore))
	putPanelData(o, d)
	o.U32(slot)
	o.U32(gen)
	t.DeferOp(*o)
}

// BuildPanel builds a panel through the registry (PanelRegistry::build_panel); false when no
// builder answers to state.PanelName.
func BuildPanel(cx *gk.Cx, dock Area, state PanelState) (Panel, bool) {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("dock.BuildPanel in a view function")
	}
	t.Flush()
	o := rt.NewReq(rt.CPanelBuildNow)
	o.U64(rt.H(dock.HCore))
	putPanelState(o, state)
	r := rt.Reader{B: rt.MustCall(*o)}
	if r.U8() == 0 {
		return Panel{}, false
	}
	h := r.U64()
	return Panel{HCore: rt.MkH(h)}, h != 0
}

// PaneTreeToState writes a pane tree out (PaneTree::to_state), reading its panels from dock.
func PaneTreeToState(cx *gk.Cx, tree PaneTree, dock Area) PanelState {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CPaneTreeToState)
	o.U64(rt.H(tree.HCore))
	o.U64(rt.H(dock.HCore))
	r := rt.Reader{B: rt.MustCall(*o)}
	return rdPanelState(&r)
}

// PaneTreeFromState reads a pane tree back (PaneTree::from_state), its panels built through the
// registry. The tree is kept until gk.Release.
func PaneTreeFromState(cx *gk.Cx, dock Area, state PanelState, root RootKind) PaneTree {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("dock.PaneTreeFromState in a view function")
	}
	t.Flush()
	o := rt.NewReq(rt.CPaneTreeFromState)
	o.U64(rt.H(dock.HCore))
	putPanelState(o, state)
	o.U16(uint16(root))
	r := rt.Reader{B: rt.MustCall(*o)}
	return PaneTree{HCore: rt.MkH(r.U64())}
}

// PaneKind is a pane node's shape (PaneNode::kind): a PaneSplit or a PaneTabs.
type PaneKind interface{ isPaneKind() }

// PaneSplit is a split with its children (temporary handles) and slot sizes.
type PaneSplit struct {
	Axis     gk.Axis
	Children []PaneNode
	Sizes    []gk.Option[float32]
}

// PaneTabs is a tab group with its panels and active index.
type PaneTabs struct {
	Panels   []DockPanelID
	ActiveIx int
}

func (PaneSplit) isPaneKind() {}

func (PaneTabs) isPaneKind() {}

// PaneNodeKind is the shape of a pane node (PaneNode::kind).
func PaneNodeKind(cx *gk.Cx, node PaneNode) PaneKind {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CPaneNodeKind)
	o.U64(rt.H(node.HCore))
	r := rt.Reader{B: rt.MustCall(*o)}
	if r.U16() == 0 {
		var s PaneSplit
		s.Axis = gk.Axis(r.U16())
		s.Children = rt.RdList(&r, func(r *rt.Reader) PaneNode { return PaneNode{HCore: rt.MkH(r.U64())} })
		s.Sizes = rt.RdList(&r, func(r *rt.Reader) gk.Option[float32] {
			return rt.RdOpt(r, func(r *rt.Reader) float32 { return r.F32() })
		})
		return s
	}
	var t PaneTabs
	t.Panels = rt.RdList(&r, func(r *rt.Reader) DockPanelID { return DockPanelID(r.U64()) })
	t.ActiveIx = int(r.U64())
	return t
}

// PaneNodeWalk is the node and its descendants in pre-order (PaneNode::walk): temporary handles.
func PaneNodeWalk(cx *gk.Cx, node PaneNode) []PaneNode {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CPaneNodeWalk)
	o.U64(rt.H(node.HCore))
	r := rt.Reader{B: rt.MustCall(*o)}
	return rt.RdList(&r, func(r *rt.Reader) PaneNode { return PaneNode{HCore: rt.MkH(r.U64())} })
}

func skinReq(dock Area, what uint8) *rt.Tape {
	o := rt.NewReq(rt.CDockSkin)
	o.U64(rt.H(dock.HCore))
	o.U8(what)
	return o
}

func skinRead(cx *gk.Cx, dock Area, what uint8) *rt.Reader {
	rt.CheckCx(cx).Flush()
	return &rt.Reader{B: rt.MustCall(*skinReq(dock, what))}
}

func skinSet(cx *gk.Cx, dock Area, what uint8, visible bool) {
	o := skinReq(dock, what)
	o.Bool(visible)
	rt.CheckCx(cx).DeferOp(*o)
}

// DockSkinPanelStyle is the skin's panel style (DockSkin::panel_style).
func SkinPanelStyle(cx *gk.Cx, dock Area) PanelStyle {
	return PanelStyle(skinRead(cx, dock, 0).U16())
}

// SetDockSkinPanelStyle sets the skin's panel style (buffered).
func SetDockSkinPanelStyle(cx *gk.Cx, dock Area, style PanelStyle) {
	o := skinReq(dock, 1)
	o.U16(uint16(style))
	rt.CheckCx(cx).DeferOp(*o)
}

// DockSkinToggleButtonVisible is whether the docks show their toggle button.
func SkinToggleButtonVisible(cx *gk.Cx, dock Area) bool { return skinRead(cx, dock, 2).Bool() }

// SetDockSkinToggleButtonVisible shows or hides the docks' toggle button (buffered).
func SetDockSkinToggleButtonVisible(cx *gk.Cx, dock Area, visible bool) {
	skinSet(cx, dock, 3, visible)
}

// SetDockSkinCloseButtonVisible shows or hides the tabs' close button (buffered).
func SetDockSkinCloseButtonVisible(cx *gk.Cx, dock Area, visible bool) {
	skinSet(cx, dock, 4, visible)
}

// DockSkinCollapsedTabBarVisible is whether a tab group that has collapsed (its panels hidden)
// still shows its tab bar.
func SkinCollapsedTabBarVisible(cx *gk.Cx, dock Area) bool { return skinRead(cx, dock, 5).Bool() }

// SetDockSkinCollapsedTabBarVisible shows or hides a collapsed tab group's tab bar (buffered).
func SetDockSkinCollapsedTabBarVisible(cx *gk.Cx, dock Area, visible bool) {
	skinSet(cx, dock, 6, visible)
}

// DockSkinSplitDividerLineVisible is whether the divider between two split slots rests as a
// one-pixel line (shown by default). Hidden, the indicator still appears on hover and while
// dragging, and the hit area is unchanged.
func SkinSplitDividerLineVisible(cx *gk.Cx, dock Area) bool { return skinRead(cx, dock, 7).Bool() }

// SetDockSkinSplitDividerLineVisible shows or hides the resting divider line (buffered).
func SetDockSkinSplitDividerLineVisible(cx *gk.Cx, dock Area, visible bool) {
	skinSet(cx, dock, 8, visible)
}

// DockHook is a hook of the dock area's and the tab groups' renderers.
type Hook uint8

const (
	HookFrame            Hook = 0  // the area's outer frame (DockAreaRenderer::frame)
	HookCenterFrame      Hook = 1  // the column of the center and the bottom dock
	HookSplitFrame       Hook = 2  // a split's frame; key: its node
	HookSplitHandle      Hook = 3  // a divider's painted part; key: axis + 2 * dragging
	HookDock             Hook = 4  // a dock's chrome around its content (the slot); key: placement
	HookPlaceholder      Hook = 5  // a panel no registered builder makes: fill q.Panel with gk.PanelFill
	HookTabFrame         Hook = 6  // a tab group's frame; key: its node
	HookTabContentFrame  Hook = 7  // the frame around the active panel
	HookTabBar           Hook = 8  // the tab bar
	HookTabActivePanel   Hook = 9  // around the active panel (the slot)
	HookTabDropIndicator Hook = 10 // drawn where a dragged panel would land
	HookTabEmpty         Hook = 11 // a group with no panels
)

// DockHookSet is a set of hooks (gk.DockHooks makes one).
type HookSet uint32

// DockHooks is the set of the hooks given.
func Hooks(hooks ...Hook) HookSet {
	var s HookSet
	for _, h := range hooks {
		s |= 1 << h
	}
	return s
}

// DockRenderer is the library's renderer of a dock area and its tab groups (a library object).
type Renderer struct{ rt.HCore }

// DockRender asks the app for its view of one hook and key (queued).
type Render struct {
	Renderer  Renderer
	Hook      Hook
	Key       uint64
	Node      NodeID          // the split's or the tab group's node
	Placement Placement       // HookDock
	Axis      gk.Axis         // HookSplitFrame, HookSplitHandle
	Flags     uint8           // HookDock: 1 open, 2 collapsible; tab hooks: 1 zoomed, 2 collapsed, 4 closable, 8 locked; HookSplitHandle: 1 dragging
	Size      float32         // HookDock: its extent along its axis
	Dock      Context         // HookDock
	Group     TabGroupContext // tab hooks
	Panel     Panel           // HookPlaceholder
	PanelName string          // HookPlaceholder
}

func init() {
	rt.RegisterShape(rt.ShapeDockRender, rt.ShapeInfo{Name: "func(cx *Cx, q DockRender)", Call: func(h any, cx *gk.Cx, r *rt.Reader, ret *rt.Tape) bool {
		f, ok := h.(func(*gk.Cx, Render))
		if ok {
			var q Render
			q.Renderer = Renderer{HCore: rt.MkH(r.U64())}
			q.Hook = Hook(r.U8())
			q.Key = r.U64()
			q.Node = NodeID(r.U64())
			q.Placement = Placement(r.U16())
			q.Axis = gk.Axis(r.U16())
			q.Flags = r.U8()
			q.Size = r.F32()
			q.Dock = Context{HCore: rt.MkH(r.U64())}
			q.Group = TabGroupContext{HCore: rt.MkH(r.U64())}
			q.Panel = Panel{HCore: rt.MkH(r.U64())}
			q.PanelName = r.Str()
			f(cx, q)
		}
		return ok
	}})
	rt.RegisterShape(rt.ShapeDockToggle, rt.ShapeInfo{Name: "func(cx *Cx, e DockToggle)", Call: func(h any, cx *gk.Cx, r *rt.Reader, ret *rt.Tape) bool {
		f, ok := h.(func(*gk.Cx, Toggle))
		if ok {
			f(cx, Toggle{Placement(r.U16()), r.Bool()})
		}
		return ok
	}})
}

// NewDockRenderer makes a renderer answering hooks through f. Give it to DockArea's or
// TabGroup's WithRenderer. In init or a handler; gk.Release frees it.
func NewRenderer(cx *gk.Cx, hooks HookSet, f func(cx *gk.Cx, q Render)) Renderer {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("dock.NewRenderer in a view function: make it in init or in a handler")
	}
	t.Flush()
	slot, gen := rt.KeepHandler(cx, f, rt.ShapeDockRender)
	o := rt.NewReq(rt.CDockRendererNew)
	o.U32(uint32(hooks))
	o.U32(slot)
	o.U32(gen)
	r := rt.Reader{B: rt.MustCall(*o)}
	return Renderer{HCore: rt.MkH(r.U64())}
}

// DockRenderAnswer gives the view drawn for (hook, key); a zero View gives upstream's default
// back. Buffered; every window draws again.
func RenderAnswer(cx *gk.Cx, r Renderer, hook Hook, key uint64, v gk.View) {
	o := rt.NewReq(rt.CDockRenderAnswer)
	o.U64(rt.H(r.HCore))
	o.U8(uint8(hook))
	o.U64(key)
	o.U64(rt.H(v.HCore))
	rt.CheckCx(cx).DeferOp(*o)
}

// DockSlot is where a renderer's view puts the element the kit handed the hook (a dock's content,
// the active panel). Nothing in a view drawn by no hook, or by one that hands no element.
func Slot() gk.Element { return rt.ValueEl{ElCore: rt.MkElCore(rt.NewNode(rt.NDockSlot))} }

// TestDockRender: requests sent, answers kept, hooks drawn from an answer (the tests').
func TestDockRender(cx *gk.Cx, r Renderer) (sent, answers, drawn int) {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CDockRenderTest)
	o.U64(rt.H(r.HCore))
	rd := rt.Reader{B: rt.MustCall(*o)}
	return int(rd.U32()), int(rd.U32()), int(rd.U32())
}

// DockToggle is what the kit reports when the user opens or closes a dock through the skin's toggle.
type Toggle struct {
	Placement Placement
	Open      bool
}

// OnDockToggle subscribes f to the dock area's toggles. In init or a handler.
func OnDockToggle(cx *gk.Cx, dock Area, f func(cx *gk.Cx, e Toggle)) gk.Subscription {
	slot, gen := rt.KeepHandler(cx, f, rt.ShapeDockToggle)
	o := rt.NewReq(rt.CDockToggleSubscribe)
	o.U64(rt.H(dock.HCore))
	o.U32(slot)
	o.U32(gen)
	r := rt.StateGet(rt.H(dock.HCore), cx, o)
	return rt.MkSubscription(r.U64(), slot)
}
