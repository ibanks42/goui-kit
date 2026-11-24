// Hand-written dock helpers: panel lifecycle and auto-release, recursive
// PanelState/AreaState serialization, registered panel factories, dock skin
// queries, and custom dock chrome Renderer hooks.

package dock

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
	"gpuikit.dev/gk/kit"
)

// PanelData configures a dock panel's persistent identity, title bar, flags,
// optional suffix/toolbar views, context menu, and serialized metadata string.
type PanelData struct {
	Name        string
	TabName     gk.Option[string]
	Title       string
	NotClosable bool
	NotZoomable bool
	Hidden      bool
	NoPadding   bool
	NoTitleBar  bool
	Suffix      gk.View
	Toolbar     gk.View
	Menu        gk.Built[kit.PopupMenu]
	Info        string
}

// PanelEventKind identifies lifecycle and visibility transitions emitted for a
// dock panel.
type PanelEventKind uint8

// Lifecycle event kinds delivered in PanelEvent.Kind.
const (
	PanelActive  PanelEventKind = 0
	PanelZoomed  PanelEventKind = 1
	PanelAdded   PanelEventKind = 2
	PanelRemoved PanelEventKind = 3
)

// PanelEvent reports a change in a dock panel's active, zoomed, or attached state.
type PanelEvent struct {
	Kind PanelEventKind
	Flag bool
}

// PanelEmit identifies a panel-level command sent to the dock area via EmitPanel.
type PanelEmit uint8

// Commands accepted by EmitPanel.
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

// PanelOption configures event handling and close retention on NewPanel and PanelFill.
type PanelOption func(*panelOpts)

type panelOpts struct {
	events func(cx *gk.Cx, e PanelEvent)
	keep   bool
}

// PanelEvents attaches a lifecycle callback invoked on PanelActive,
// PanelZoomed, PanelAdded, and PanelRemoved transitions.
func PanelEvents(f func(cx *gk.Cx, e PanelEvent)) PanelOption {
	return func(o *panelOpts) { o.events = f }
}

// KeepWhenClosed keeps a closed panel's handle and view alive after
// PanelRemoved instead of automatically posting gk.Release on the next wake.
func KeepWhenClosed() PanelOption {
	return func(o *panelOpts) { o.keep = true }
}

// panelLife tracks whether a panel has been removed from its dock area so
// automatic release can be canceled if the panel is re-added before the posted
// cleanup callback runs (for example, during a tab drag across groups).
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
		gk.Post(func(cx *gk.Cx) {
			if pl.removed && rt.Tracked(pl.h) {
				gk.Release(cx, Panel{HCore: rt.MkH(pl.h)})
			}
		})
	}
}

func (p Panel) handle() uint64 { return rt.H(p.HCore) }

// NewPanel creates a dock Panel wrapping view v with metadata d.
// Ownership of v transfers to the panel so closing or releasing the panel also
// releases v and any handles v owns. Unless KeepWhenClosed is passed, removing
// the panel from the dock posts an automatic gk.Release.
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
	rt.Made(pl.h)
	rt.AdoptView(pl.h, v)
	return Panel{HCore: rt.MkH(pl.h)}
}

// SetPanel updates the PanelData metadata of an existing dock Panel.
func SetPanel(cx *gk.Cx, p Panel, d PanelData) {
	o := rt.NewReq(rt.CPanelSet)
	o.U64(rt.H(p.HCore))
	putPanelData(o, d)
	rt.CheckCx(cx).DeferOp(*o)
}

// EmitPanel triggers a dock command (zoom in, zoom out, or layout changed) on p.
func EmitPanel(cx *gk.Cx, p Panel, e PanelEmit) {
	o := rt.NewReq(rt.CPanelEmit)
	o.U64(rt.H(p.HCore))
	o.U8(uint8(e))
	rt.CheckCx(cx).DeferOp(*o)
}

// NewAreaSkinned creates a dock Area with customizable skin settings outside a
// view function.
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

// View renders dock Area d as an element inside a view tree.
func View(d Area) gk.Element {
	n := rt.NewNode(rt.NDockView)
	rt.NodeArgs(n).U64(rt.H(d.HCore))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// NodeID identifies a split or tab-group node inside a dock pane tree.
type NodeID uint64

// DockPanelID uniquely identifies a panel instance inside a dock pane tree.
type DockPanelID uint64

// PanelGroup returns the TabGroup currently containing panel p, or false if p
// is not placed in a tab group.
func PanelGroup(cx *gk.Cx, p Panel) (TabGroup, bool) {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CPanelGroup)
	o.U64(rt.H(p.HCore))
	r := rt.Reader{B: rt.MustCall(*o)}
	h := r.U64()
	return TabGroup{HCore: rt.MkH(h)}, h != 0
}

// PanelID returns the stable DockPanelID assigned to panel p.
func PanelID(cx *gk.Cx, p Panel) DockPanelID {
	rt.CheckCx(cx)
	o := rt.NewReq(rt.CPanelId)
	o.U64(rt.H(p.HCore))
	r := rt.Reader{B: rt.MustCall(*o)}
	return DockPanelID(r.U64())
}

// PanelState is a serializable tree node representing a split stack, tab group,
// or leaf panel inside a saved dock layout.
type PanelState struct {
	PanelName string
	Children  []PanelState
	Info      PanelInfo
}

// PanelInfo is the tagged union held by PanelState.Info: PanelInfoStack,
// PanelInfoTabs, or PanelInfoPanel.
type PanelInfo interface{ isPanelInfo() }

// PanelInfoStack describes a split container with child sizes and layout axis.
type PanelInfoStack struct {
	Sizes []float32
	Axis  int
}

// PanelInfoTabs describes a tabbed container with its active tab index.
type PanelInfoTabs struct{ ActiveIndex int }

// PanelInfoPanel holds the JSON payload associated with a leaf panel.
type PanelInfoPanel struct{ JSON string }

func (PanelInfoStack) isPanelInfo() {}

func (PanelInfoTabs) isPanelInfo() {}

func (PanelInfoPanel) isPanelInfo() {}

// Saved stores the saved layout state, placement edge, pixel size, and open
// flag for one side dock (left, right, or bottom).
type Saved struct {
	Panel     PanelState
	Placement Placement
	Size      float32
	Open      bool
}

// AreaState captures the full serializable layout of a dock Area, including
// its center pane tree and optional left, right, and bottom docks.
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

// PanelBuild is passed to a RegisterPanel factory when restoring a saved panel.
// The callback populates b.Panel by calling PanelFill.
type PanelBuild struct {
	Panel Panel
	Name  string
	State PanelState
	Dock  Area
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

// RegisterPanel associates a panel type name with factory f so saved layouts
// can reconstruct panels of that name via BuildPanel or Area.Load.
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

// PanelFill binds view v, metadata d, and optional panel options into the
// placeholder Panel handed to a RegisterPanel callback.
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

// BuildPanel reconstructs a Panel from a saved PanelState using the factory
// previously registered for state.PanelName.
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

// PaneTreeToState serializes a live PaneTree into a PanelState snapshot.
func PaneTreeToState(cx *gk.Cx, tree PaneTree, dock Area) PanelState {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CPaneTreeToState)
	o.U64(rt.H(tree.HCore))
	o.U64(rt.H(dock.HCore))
	r := rt.Reader{B: rt.MustCall(*o)}
	return rdPanelState(&r)
}

// PaneTreeFromState rebuilds a live PaneTree from a serialized PanelState.
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

// PaneKind is implemented by PaneSplit and PaneTabs returned by PaneNodeKind.
type PaneKind interface{ isPaneKind() }

// PaneSplit describes a split PaneNode dividing space along Axis among Children.
type PaneSplit struct {
	Axis     gk.Axis
	Children []PaneNode
	Sizes    []gk.Option[float32]
}

// PaneTabs describes a leaf tab-group PaneNode holding Panels and an active index.
type PaneTabs struct {
	Panels   []DockPanelID
	ActiveIx int
}

func (PaneSplit) isPaneKind() {}

func (PaneTabs) isPaneKind() {}

// PaneNodeKind inspects node and returns either a PaneSplit or PaneTabs value.
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

// PaneNodeWalk returns all PaneNodes reachable in a traversal rooted at node.
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

// SkinPanelStyle returns the active PanelStyle of a skinned dock Area.
func SkinPanelStyle(cx *gk.Cx, dock Area) PanelStyle {
	return PanelStyle(skinRead(cx, dock, 0).U16())
}

// SetDockSkinPanelStyle updates the PanelStyle of a skinned dock Area.
func SetDockSkinPanelStyle(cx *gk.Cx, dock Area, style PanelStyle) {
	o := skinReq(dock, 1)
	o.U16(uint16(style))
	rt.CheckCx(cx).DeferOp(*o)
}

// SkinToggleButtonVisible reports whether dock toggle buttons are shown.
func SkinToggleButtonVisible(cx *gk.Cx, dock Area) bool { return skinRead(cx, dock, 2).Bool() }

// SetDockSkinToggleButtonVisible shows or hides dock toggle buttons.
func SetDockSkinToggleButtonVisible(cx *gk.Cx, dock Area, visible bool) {
	skinSet(cx, dock, 3, visible)
}

// SetDockSkinCloseButtonVisible shows or hides panel close buttons.
func SetDockSkinCloseButtonVisible(cx *gk.Cx, dock Area, visible bool) {
	skinSet(cx, dock, 4, visible)
}

// SkinCollapsedTabBarVisible reports whether collapsed dock tab bars are shown.
func SkinCollapsedTabBarVisible(cx *gk.Cx, dock Area) bool { return skinRead(cx, dock, 5).Bool() }

// SetDockSkinCollapsedTabBarVisible shows or hides collapsed dock tab bars.
func SetDockSkinCollapsedTabBarVisible(cx *gk.Cx, dock Area, visible bool) {
	skinSet(cx, dock, 6, visible)
}

// SkinSplitDividerLineVisible reports whether split divider lines are visible.
func SkinSplitDividerLineVisible(cx *gk.Cx, dock Area) bool { return skinRead(cx, dock, 7).Bool() }

// SetDockSkinSplitDividerLineVisible shows or hides split divider lines.
func SetDockSkinSplitDividerLineVisible(cx *gk.Cx, dock Area, visible bool) {
	skinSet(cx, dock, 8, visible)
}

// Hook identifies one customizable chrome slot in a dock Renderer.
type Hook uint8

// Render hooks that a dock Renderer can override via Hooks and NewRenderer.
const (
	HookFrame            Hook = 0
	HookCenterFrame      Hook = 1
	HookSplitFrame       Hook = 2
	HookSplitHandle      Hook = 3
	HookDock             Hook = 4
	HookPlaceholder      Hook = 5
	HookTabFrame         Hook = 6
	HookTabContentFrame  Hook = 7
	HookTabBar           Hook = 8
	HookTabActivePanel   Hook = 9
	HookTabDropIndicator Hook = 10
	HookTabEmpty         Hook = 11
)

// HookSet is a bitmask of Hook values enabled on a custom dock Renderer.
type HookSet uint32

// Hooks combines one or more Hook values into a HookSet bitmask.
func Hooks(hooks ...Hook) HookSet {
	var s HookSet
	for _, h := range hooks {
		s |= 1 << h
	}
	return s
}

// Renderer is a handle to a custom dock chrome renderer created by NewRenderer.
type Renderer struct{ rt.HCore }

// Render carries the context for one custom dock chrome hook invocation.
// The callback supplies a view for (q.Hook, q.Key) by calling RenderAnswer.
type Render struct {
	Renderer  Renderer
	Hook      Hook
	Key       uint64
	Node      NodeID
	Placement Placement
	Axis      gk.Axis
	Flags     uint8
	Size      float32
	Dock      Context
	Group     TabGroupContext
	Panel     Panel
	PanelName string
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

// NewRenderer creates a custom dock chrome Renderer that invokes f for every
// hook enabled in hooks.
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

// RenderAnswer supplies view v as the rendered output for (hook, key) on r.
func RenderAnswer(cx *gk.Cx, r Renderer, hook Hook, key uint64, v gk.View) {
	o := rt.NewReq(rt.CDockRenderAnswer)
	o.U64(rt.H(r.HCore))
	o.U8(uint8(hook))
	o.U64(key)
	o.U64(rt.H(v.HCore))
	rt.CheckCx(cx).DeferOp(*o)
}

// Slot renders the inner dock content placeholder inside a custom Renderer view.
func Slot() gk.Element { return rt.ValueEl{ElCore: rt.MkElCore(rt.NewNode(rt.NDockSlot))} }

// TestDockRender queries diagnostic counts of render requests, answers, and
// drawn slots on Renderer r.
func TestDockRender(cx *gk.Cx, r Renderer) (sent, answers, drawn int) {
	rt.CheckCx(cx).Flush()
	o := rt.NewReq(rt.CDockRenderTest)
	o.U64(rt.H(r.HCore))
	rd := rt.Reader{B: rt.MustCall(*o)}
	return int(rd.U32()), int(rd.U32()), int(rd.U32())
}

// Toggle reports a side dock opening or closing at Placement.
type Toggle struct {
	Placement Placement
	Open      bool
}

// OnDockToggle subscribes to side-dock open/close toggle events on dock.
func OnDockToggle(cx *gk.Cx, dock Area, f func(cx *gk.Cx, e Toggle)) gk.Subscription {
	slot, gen := rt.KeepHandler(cx, f, rt.ShapeDockToggle)
	o := rt.NewReq(rt.CDockToggleSubscribe)
	o.U64(rt.H(dock.HCore))
	o.U32(slot)
	o.U32(gen)
	r := rt.StateGet(rt.H(dock.HCore), cx, o)
	return rt.MkSubscription(r.U64(), slot)
}
