// The modes of the dock's node ids, saved layouts, pane tree and renderer (PROTOCOLS.md 11.8, 11.9),
// the nav stack's views, the toast manager on the library clock (11.10) and a fallible value call
// (11.15): dock-ids, dock-bad-node, dock-layout, nav, render, render-bad-frame, toast, values.
package main

import (
	"fmt"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

// What the pending modes keep whose types the package does not have yet.
var ov struct {
	node     gk.NodeID
	saved    string // the first dump, printed
	builds   int
	nav      gk.NavStackState
	pages    [2]*Body
	pageV    [2]gk.View
	bar      gk.AppMenuBar
	renderer gk.DockRenderer
	asks     map[gk.DockHook]int
	chrome   Chrome
	chromeV  gk.View
	tabbarV  gk.View
	frameV   gk.View
	dockCtx  gk.DockContext
	tabbars  int
	frames   int
	toasts   gk.ToastManager
	toggles  int
	toggle   gk.Subscription
}

// Chrome is the view the renderer answers a dock's chrome with.
type Chrome struct {
	placement gk.DockPlacement
	open      bool
	described int
}

func init() {
	ov.asks = map[gk.DockHook]int{}
	steps["dock-ids"] = []step{{10, dockMake}, {30, dockIDs}, {50, dockIDsSplit}, {70, dockIDsZoom}}
	steps["dock-bad-node"] = []step{{10, dockMake}, {30, badNode}}
	steps["dock-layout"] = []step{{10, dockMake}, {30, layoutSave}, {50, layoutLoad}, {70, layoutCheck}, {90, layoutDone}}
	steps["nav"] = []step{{10, navMake}, {30, navCheck}}
	steps["render-bad-frame"] = []step{{10, renderMake}}
	steps["render"] = []step{{10, renderMake}, {50, renderCheck}, {90, renderDone}}
	steps["toast"] = []step{{10, toastRun}}
	steps["values"] = []step{{10, valueCalls}}
	for _, m := range []string{"dock-ids", "dock-bad-node", "dock-layout", "render", "render-bad-frame"} {
		roots[m] = (*App).dockRoot
	}
	roots["nav"] = navRoot
}

// dockPlacementName prints a DockPlacement as the checks expect (the variant's name).
func dockPlacementName(p gk.DockPlacement) string {
	names := []string{"Center", "Left", "Bottom", "Right"}
	if int(p) < len(names) {
		return names[p]
	}
	return fmt.Sprint(uint16(p))
}

// joinText prints a slice in the form the checks expect: [200, 440].
func joinText[T any](xs []T) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// --- PROTOCOLS.md 11.8: node ids, the tab group, a split, zoom by node -----------------

func dockIDs(a *App, cx *gk.Cx) {
	g, ok := gk.PanelGroup(cx, a.panels[0])
	fmt.Printf("overlay ids group=%v\n", ok)
	if !ok {
		return
	}
	ov.node = g.Node(cx)
	ps := g.Panels(cx)
	ctx := g.Context(cx)
	fmt.Printf("overlay ids panels=%d same=%v,%v active=%d ctx_node_same=%v ctx_panels=%d closable=%v\n", len(ps),
		len(ps) == 2 && ps[0] == a.panels[0], len(ps) == 2 && ps[1] == a.panels[1],
		g.ActiveIx(cx), ctx.Node(cx) == ov.node, len(ctx.Panels(cx)), ctx.IsPanelClosable(cx, gk.PanelID(cx, a.panels[0])))
	p, found := a.dock.Panel(cx, gk.PanelID(cx, a.panels[1])).Get()
	fmt.Printf("overlay ids dock panel found=%v same=%v\n", found, found && p == a.panels[1])
	// split the group: the second panel goes to the right of it, in a new group
	a.dock.SplitAt(cx, ov.node, gk.PanelID(cx, a.panels[1]), gk.PlacementRight)
}

func dockIDsSplit(a *App, cx *gk.Cx) {
	tree, ok := a.dock.Layout(cx, gk.DockPlacementCenter).Get()
	ids := tree.NodeIDs(cx)
	n1, _ := tree.FindPanelNode(cx, gk.PanelID(cx, a.panels[0])).Get()
	n2, _ := tree.FindPanelNode(cx, gk.PanelID(cx, a.panels[1])).Get()
	root := tree.Root(cx).ID(cx)
	fmt.Printf("overlay ids split tree=%v nodes=%d first_kept=%v second_moved=%v\n", ok, len(ids), n1 == ov.node, n2 != ov.node)
	kind := gk.PaneNodeKind(cx, tree.Root(cx))
	sp, isSplit := kind.(gk.PaneSplit)
	walked := gk.PaneNodeWalk(cx, tree.Root(cx))
	fmt.Printf("overlay ids kind split=%v children=%d walk=%d\n", isSplit, len(sp.Children), len(walked))
	if len(sp.Children) > 0 {
		tabs, isTabs := gk.PaneNodeKind(cx, sp.Children[0]).(gk.PaneTabs)
		fmt.Printf("overlay ids first child tabs=%v panels=%d\n", isTabs, len(tabs.Panels))
	}
	a.dock.SetSplitSizes(cx, root, []float32{200, 440})
	a.dock.SetZoomedIn(cx, n2)
}

// panelStyleName prints a PanelStyle as the checks expect (the variant's name).
func panelStyleName(p gk.PanelStyle) string {
	switch p {
	case gk.PanelStyleAuto:
		return "Auto"
	case gk.PanelStyleTabBar:
		return "Tab_Bar"
	}
	return fmt.Sprint(uint16(p))
}

func dockIDsZoom(a *App, cx *gk.Cx) {
	z, zoomed := a.dock.ZoomedGroup(cx).Get()
	tree, _ := a.dock.Layout(cx, gk.DockPlacementCenter).Get()
	n2, _ := tree.FindPanelNode(cx, gk.PanelID(cx, a.panels[1])).Get()
	fmt.Printf("overlay ids zoomed=%v group=%v\n", zoomed, z == n2)
	a.dock.SetZoomedOut(cx)
	gk.SetDockSkinPanelStyle(cx, a.dock, gk.PanelStyleTabBar)
	gk.SetDockSkinToggleButtonVisible(cx, a.dock, false)
	fmt.Printf("overlay ids skin style=%s toggle=%v\n", panelStyleName(gk.DockSkinPanelStyle(cx, a.dock)), gk.DockSkinToggleButtonVisible(cx, a.dock))
	saved := a.dock.Dump(cx)
	var sizes []float32
	if st, ok := saved.Center.Info.(gk.PanelInfoStack); ok {
		sizes = st.Sizes
	}
	fmt.Printf("overlay done mode=dock-ids children=%d sizes=%s\n", len(saved.Center.Children), joinText(sizes))
}

func badNode(a *App, cx *gk.Cx) {
	a.dock.SetZoomedIn(cx, gk.NodeID(987654321)) // never handed out
}

// --- a saved layout restored through the registry --------------------------------------

func (a *App) restore(cx *gk.Cx, b gk.PanelBuild) {
	ov.builds++
	i := 1
	if b.Name == "first" {
		i = 0
	}
	fmt.Printf("overlay layout build %s n=%d dock=%v\n", b.Name, ov.builds, b.Dock == a.dock)
	a.panels[i] = b.Panel
	a.bodies[i].text = "restored " + b.Name
	info := ""
	if pi, ok := b.State.Info.(gk.PanelInfoPanel); ok {
		info = pi.JSON // the panel's own saved data
	}
	gk.PanelFill(cx, b.Panel, gk.NewView(nil, a.bodies[i].view), gk.PanelData{Name: b.Name, Title: b.Name, Info: info},
		gk.PanelEvents(func(cx *gk.Cx, e gk.PanelEvent) { a.panelEvent(cx, e, i) }))
}

func printState(st gk.PanelState) string {
	kind := "panel"
	switch v := st.Info.(type) {
	case gk.PanelInfoStack:
		kind = fmt.Sprintf("stack%d", len(v.Sizes))
	case gk.PanelInfoTabs:
		kind = fmt.Sprintf("tabs@%d", v.ActiveIndex)
	case gk.PanelInfoPanel:
		kind = fmt.Sprintf("panel'%s'", v.JSON)
	}
	out := fmt.Sprintf("%s:%s(", st.PanelName, kind)
	for _, c := range st.Children {
		out += printState(c) + " "
	}
	return out + ")"
}

func layoutSave(a *App, cx *gk.Cx) {
	gk.SetPanel(cx, a.panels[0], gk.PanelData{Name: "first", Title: "first", Info: `{"file":"a.demo"}`})
}

func layoutLoad(a *App, cx *gk.Cx) {
	saved := a.dock.Dump(cx)
	ov.saved = printState(saved.Center)
	fmt.Printf("overlay layout saved %s\n", ov.saved)
	gk.RegisterPanel(cx, "first", a.restore)
	gk.RegisterPanel(cx, "second", a.restore)
	ov.builds = 0
	err := a.dock.Load(cx, saved)
	fmt.Printf("overlay layout loaded failed=%v err=%s\n", err != nil, errText(err))
}

func layoutCheck(a *App, cx *gk.Cx) {
	again := printState(a.dock.Dump(cx).Center)
	fmt.Printf("overlay layout again same=%v described=%d,%d\n", again == ov.saved, a.bodies[0].described, a.bodies[1].described)
	tree, _ := a.dock.Layout(cx, gk.DockPlacementCenter).Get()
	st := gk.PaneTreeToState(cx, tree, a.dock)
	t2 := gk.PaneTreeFromState(cx, a.dock, st, gk.RootKindSplit)
	fmt.Printf("overlay layout tree to_state=%s from_state_panels=%d\n", printState(st), len(t2.Panels(cx)))
	p, ok := gk.BuildPanel(cx, a.dock, gk.PanelState{PanelName: "second", Info: gk.PanelInfoPanel{}})
	_, ok2 := gk.BuildPanel(cx, a.dock, gk.PanelState{PanelName: "nobody", Info: gk.PanelInfoPanel{}})
	fmt.Printf("overlay layout build_panel known=%v unknown=%v\n", ok && p != (gk.DockPanel{}), ok2)
}

func layoutDone(a *App, cx *gk.Cx) {
	fmt.Printf("overlay done mode=dock-layout builds=%d\n", ov.builds)
}

// --- the nav stack takes and gives back app views -------------------------------------

func navMake(a *App, cx *gk.Cx) {
	ov.pages = [2]*Body{{text: "home"}, {text: "details"}}
	for i := range 2 {
		ov.pageV[i] = gk.NewView(nil, ov.pages[i].view)
	}
	ov.nav = gk.NewNavStackState(cx)
	ov.nav.Push(cx, ov.pageV[0], gk.NavMotionImmediate)
	ov.nav.Push(cx, ov.pageV[1], gk.NavMotionImmediate)
}

func navCheck(a *App, cx *gk.Cx) {
	cur, ok := ov.nav.Current(cx).Get()
	vs := ov.nav.Views(cx)
	fmt.Printf("overlay nav depth=%d current=%v views=%d,%v\n", ov.nav.Depth(cx), ok && cur == ov.pageV[1],
		len(vs), len(vs) == 2 && vs[0] == ov.pageV[0])
	popped, pok := ov.nav.Pop(cx, gk.NavMotionImmediate).Get()
	cur2, _ := ov.nav.Current(cx).Get()
	fmt.Printf("overlay nav popped=%v now=%v forward=%d\n", pok && popped == ov.pageV[1], cur2 == ov.pageV[0],
		len(ov.nav.ForwardViews(cx)))
	bar := gk.NewAppMenuBar(cx)
	bar.Reload(cx)
	ov.bar = bar
	r := gk.NewResizableState(cx)
	fmt.Printf("overlay done mode=nav sizes=%d container=%v\n", len(r.Sizes(cx)), r.ContainerSize(cx))
}

func navRoot(a *App, cx *gk.Cx) gk.Element {
	if ov.nav == (gk.NavStackState{}) {
		return gk.Div(s.SizeFull)
	}
	root := gk.Div(s.SizeFull).Child(gk.NavStack(ov.nav))
	if ov.bar != (gk.AppMenuBar{}) {
		root = root.Child(gk.AppMenuBarView(ov.bar))
	}
	return root
}

// --- PROTOCOLS.md 11.9: a dock drawn by the app's renderer -------------------------------

func (c *Chrome) view(cx *gk.Cx) gk.Element {
	c.described++
	return gk.VFlex(s.SizeFull).Child(gk.Label(fmt.Sprintf("%s open=%v", dockPlacementName(c.placement), c.open))).Child(gk.DockSlot())
}

func (a *App) tabbarView(cx *gk.Cx) gk.Element {
	ov.tabbars++
	return gk.HFlex(s.WFull).Child(gk.Label("tabs of the app"))
}

func (a *App) frameView(cx *gk.Cx) gk.Element {
	ov.frames++
	if a.mode == "render-bad-frame" {
		return gk.Label("a frame must be a div") // misuse: a frame hook's view is a gk.Div()
	}
	return gk.HFlex(s.SizeFull, s.P1)
}

func (a *App) asked(cx *gk.Cx, q gk.DockRender) {
	ov.asks[q.Hook]++
	switch q.Hook {
	case gk.HookFrame:
		gk.DockRenderAnswer(cx, q.Renderer, q.Hook, q.Key, ov.frameV)
	case gk.HookDock:
		ov.chrome.placement = q.Dock.Placement(cx)
		ov.chrome.open = q.Dock.IsOpen(cx)
		ov.dockCtx = q.Dock
		fmt.Printf("overlay render asked Dock placement=%s open=%v key=%d same=%v\n", dockPlacementName(q.Placement), ov.chrome.open, q.Key,
			q.Placement == ov.chrome.placement)
		gk.DockRenderAnswer(cx, q.Renderer, q.Hook, q.Key, ov.chromeV)
	case gk.HookTabBar:
		fmt.Printf("overlay render asked Tab_Bar panels=%d\n", len(q.Group.Panels(cx)))
		gk.DockRenderAnswer(cx, q.Renderer, q.Hook, q.Key, ov.tabbarV)
	}
}

func renderMake(a *App, cx *gk.Cx) {
	a.bodies = [2]*Body{{text: "first"}, {text: "second"}}
	ov.chromeV = gk.NewView(nil, ov.chrome.view)
	ov.tabbarV = gk.NewView(nil, a.tabbarView)
	ov.frameV = gk.NewView(nil, a.frameView)
	ov.renderer = gk.NewDockRenderer(cx, gk.DockHooks(gk.HookFrame, gk.HookDock, gk.HookTabBar), a.asked)
	a.dock = gk.NewDockArea(cx, "rendered", gk.Option[int]{}).WithRenderer(ov.renderer).Build()
	for i := range 2 {
		name := "second"
		if i == 0 {
			name = "first"
		}
		v := gk.NewView(nil, a.bodies[i].view)
		a.panels[i] = gk.NewPanel(cx, v, gk.PanelData{Name: name, Title: a.bodies[i].text})
	}
	a.dock.SetCenter(cx, gk.NewDockLayoutTabs().PanelView(a.panels[0]))
	a.dock.SetDock(cx, gk.DockPlacementLeft, gk.NewDockLayoutTabs().PanelView(a.panels[1]))
	ov.toggle = gk.OnDockToggle(cx, a.dock, func(cx *gk.Cx, t gk.DockToggle) { ov.toggles++ })
	fmt.Println("overlay render made")
}

func renderCheck(a *App, cx *gk.Cx) {
	_, answers, drawn := gk.TestDockRender(cx, ov.renderer)
	fmt.Printf("overlay render frame=%d dock=%d tab_bar=%d answers=%d drawn_some=%v\n", ov.asks[gk.HookFrame], ov.asks[gk.HookDock],
		ov.asks[gk.HookTabBar], answers, drawn > 0)
	fmt.Printf("overlay render described chrome=%v tabbar=%v frame=%v bodies=%v\n", ov.chrome.described > 0, ov.tabbars > 0,
		ov.frames > 0, a.bodies[0].described > 0 && a.bodies[1].described > 0)
	ov.dockCtx.Toggle(cx) // the chrome's own call: the dock closes, its context changes, it is asked again
}

func renderDone(a *App, cx *gk.Cx) {
	// the kit reports a toggle of the user's (the skin's buttons) only: the chrome's call is the app's
	fmt.Printf("overlay done mode=render open=%v dock_asks=%d chrome_open=%v toggles=%d sub=%v\n", a.dock.IsDockOpen(cx, gk.DockPlacementLeft),
		ov.asks[gk.HookDock], ov.chrome.open, ov.toggles, ov.toggle != gk.Subscription{})
}

// --- PROTOCOLS.md 11.10: the toast manager on the library clock ---------------------------

// toastStatusName prints a ToastTransitionStatus as the checks expect (the variant's name).
func toastStatusName(t gk.ToastTransitionStatus) string {
	names := []string{"Starting", "Present", "Ending"}
	if int(t) < len(names) {
		return names[t]
	}
	return fmt.Sprint(uint16(t))
}

// optText prints an optional value as the checks expect (nil, or the value).
func optText[T any](o gk.Option[T]) string {
	v, ok := o.Get()
	if !ok {
		return "nil"
	}
	return fmt.Sprint(v)
}

func toastRun(a *App, cx *gk.Cx) {
	ov.toasts = gk.NewToastManager(cx, gk.ToastMotion{Duration: 100 * time.Millisecond, ExitDuration: 50 * time.Millisecond})
	t0 := gk.ClockNow(cx)
	ov.toasts.Push(cx, 1, 100, gk.ToastOptions{Timeout: gk.Some(300 * time.Millisecond)}, t0)
	ov.toasts.Push(cx, 2, 200, gk.ToastOptions{}, t0)
	replaced := ov.toasts.Push(cx, 2, 201, gk.ToastOptions{}, t0)
	fmt.Printf("overlay toast len=%d replaced=%s get2=%s\n", ov.toasts.Len(cx), optText(replaced), optText(ov.toasts.Get(cx, 2)))
	adv := ov.toasts.Advance(cx, t0+150*time.Millisecond, false)
	fmt.Printf("overlay toast presented=%s changed=%v\n", joinText(adv.Presented), adv.Changed)
	adv = ov.toasts.Advance(cx, t0+500*time.Millisecond, false)
	fmt.Printf("overlay toast ending=%s\n", joinText(adv.Ending))
	adv = ov.toasts.Advance(cx, t0+600*time.Millisecond, false)
	var removed gk.TupleUint64Uint64
	if len(adv.Removed) > 0 {
		removed = adv.Removed[0]
	}
	fmt.Printf("overlay toast removed=%d value=%d left=%d\n", removed.V0, removed.V1, ov.toasts.Len(cx))
	dismissed := ov.toasts.Dismiss(cx, 2, t0+600*time.Millisecond)
	visible := ov.toasts.Visible(cx, 5)
	status := gk.ToastTransitionStatusStarting
	if len(visible) > 0 {
		status = visible[0].V2
	}
	fmt.Printf("overlay toast dismissed=%v visible=%d status=%s empty=%v\n", dismissed, len(visible), toastStatusName(status), ov.toasts.IsEmpty(cx))
	ov.toasts.Push(cx, 3, 300, gk.ToastOptions{}, t0+600*time.Millisecond)
	all := ov.toasts.Iter(cx)
	gone := ov.toasts.DismissAll(cx, t0+700*time.Millisecond)
	later := gk.ClockNow(cx)
	fmt.Printf("overlay done mode=toast iter=%d dismissed_all=%s clock_moves=%v\n", len(all), joinText(gone), later >= t0)
}

// --- PROTOCOLS.md 11.15: value calls ----------------------------------------------------------

// Value calls: computations over a value the app built (DockSizing, Sankey), run as ambient calls.
func valueCalls(a *App, cx *gk.Cx) {
	sizing := gk.NewDockSizing(gk.DockPlacementLeft).WithAreaWidth(1000).WithOppositeDockSize(200)
	big := gk.BaseDockSizingClamp(cx, sizing, 5000)
	small := gk.BaseDockSizingClamp(cx, gk.NewDockSizing(gk.DockPlacementLeft).WithAreaWidth(1000).WithOppositeDockSize(200), 10)
	from := gk.BaseDockSizingSizeFromPointer(cx, gk.NewDockSizing(gk.DockPlacementLeft).WithAreaWidth(1000), gk.GpuiPoint{X: 300, Y: 10})
	fmt.Printf("overlay values clamp=%.0f,%.0f pointer=%.0f\n", big, small, from)
	links := []gk.SankeyLink{gk.NewSankeyLink(0, 1, 5), gk.NewSankeyLink(1, 2, 5)}
	g := gk.BaseSankeyLayout(cx, gk.NewSankey().Size(300, 200), 3, links)
	layers := -1
	if graph, ok := g.Value.Get(); ok {
		layers = graph.LayerCount(cx)
	}
	bad := gk.BaseSankeyLayout(cx, gk.NewSankey(), 1, links) // a link to a node that is not there
	fmt.Printf("overlay done mode=values ok=%v layers=%d bad_ok=%v bad_err=%v\n", g.OK, layers, bad.OK, len(bad.Err) > 0)
}
