// The modes of the dock's node ids, saved layouts, pane tree and renderer (PROTOCOLS.md 11.8, 11.9),
// the nav stack's views, the toast manager on the library clock (11.10) and a fallible value call
// (11.15): dock-ids, dock-bad-node, dock-layout, nav, render, render-bad-frame, toast, values.
package main

import (
	"fmt"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/base"
	"gpuikit.dev/gk/chart"
	"gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
)

// What the pending modes keep whose types the package does not have yet.
var ov struct {
	node     dock.NodeID
	saved    string // the first dump, printed
	builds   int
	nav      base.NavStackState
	pages    [2]*Body
	pageV    [2]gk.View
	bar      kit.AppMenuBar
	renderer dock.Renderer
	asks     map[dock.Hook]int
	chrome   Chrome
	chromeV  gk.View
	tabbarV  gk.View
	frameV   gk.View
	dockCtx  dock.Context
	tabbars  int
	frames   int
	toasts   overlay.ToastManager
	toggles  int
	toggle   gk.Subscription
}

// Chrome is the view the renderer answers a dock's chrome with.
type Chrome struct {
	placement dock.Placement
	open      bool
	described int
}

func init() {
	ov.asks = map[dock.Hook]int{}
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
func dockPlacementName(p dock.Placement) string {
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
	g, ok := dock.PanelGroup(cx, a.panels[0])
	fmt.Printf("overlay ids group=%v\n", ok)
	if !ok {
		return
	}
	ov.node = g.Node(cx)
	ps := g.Panels(cx)
	ctx := g.Context(cx)
	fmt.Printf("overlay ids panels=%d same=%v,%v active=%d ctx_node_same=%v ctx_panels=%d closable=%v\n", len(ps),
		len(ps) == 2 && ps[0] == a.panels[0], len(ps) == 2 && ps[1] == a.panels[1],
		g.ActiveIx(cx), ctx.Node(cx) == ov.node, len(ctx.Panels(cx)), ctx.IsPanelClosable(cx, dock.PanelID(cx, a.panels[0])))
	p, found := a.dock.Panel(cx, dock.PanelID(cx, a.panels[1])).Get()
	fmt.Printf("overlay ids dock panel found=%v same=%v\n", found, found && p == a.panels[1])
	// split the group: the second panel goes to the right of it, in a new group
	a.dock.SplitAt(cx, ov.node, dock.PanelID(cx, a.panels[1]), gk.PlacementRight)
}

func dockIDsSplit(a *App, cx *gk.Cx) {
	tree, ok := a.dock.Layout(cx, dock.PlacementCenter).Get()
	ids := tree.NodeIDs(cx)
	n1, _ := tree.FindPanelNode(cx, dock.PanelID(cx, a.panels[0])).Get()
	n2, _ := tree.FindPanelNode(cx, dock.PanelID(cx, a.panels[1])).Get()
	root := tree.Root(cx).ID(cx)
	fmt.Printf("overlay ids split tree=%v nodes=%d first_kept=%v second_moved=%v\n", ok, len(ids), n1 == ov.node, n2 != ov.node)
	kind := dock.PaneNodeKind(cx, tree.Root(cx))
	sp, isSplit := kind.(dock.PaneSplit)
	walked := dock.PaneNodeWalk(cx, tree.Root(cx))
	fmt.Printf("overlay ids kind split=%v children=%d walk=%d\n", isSplit, len(sp.Children), len(walked))
	if len(sp.Children) > 0 {
		tabs, isTabs := dock.PaneNodeKind(cx, sp.Children[0]).(dock.PaneTabs)
		fmt.Printf("overlay ids first child tabs=%v panels=%d\n", isTabs, len(tabs.Panels))
	}
	a.dock.SetSplitSizes(cx, root, []float32{200, 440})
	a.dock.SetZoomedIn(cx, n2)
}

// panelStyleName prints a PanelStyle as the checks expect (the variant's name).
func panelStyleName(p dock.PanelStyle) string {
	switch p {
	case dock.PanelStyleAuto:
		return "Auto"
	case dock.PanelStyleTabBar:
		return "Tab_Bar"
	}
	return fmt.Sprint(uint16(p))
}

func dockIDsZoom(a *App, cx *gk.Cx) {
	z, zoomed := a.dock.ZoomedGroup(cx).Get()
	tree, _ := a.dock.Layout(cx, dock.PlacementCenter).Get()
	n2, _ := tree.FindPanelNode(cx, dock.PanelID(cx, a.panels[1])).Get()
	fmt.Printf("overlay ids zoomed=%v group=%v\n", zoomed, z == n2)
	a.dock.SetZoomedOut(cx)
	dock.SetDockSkinPanelStyle(cx, a.dock, dock.PanelStyleTabBar)
	dock.SetDockSkinToggleButtonVisible(cx, a.dock, false)
	fmt.Printf("overlay ids skin style=%s toggle=%v\n", panelStyleName(dock.SkinPanelStyle(cx, a.dock)), dock.SkinToggleButtonVisible(cx, a.dock))
	saved := a.dock.Dump(cx)
	var sizes []float32
	if st, ok := saved.Center.Info.(dock.PanelInfoStack); ok {
		sizes = st.Sizes
	}
	fmt.Printf("overlay done mode=dock-ids children=%d sizes=%s\n", len(saved.Center.Children), joinText(sizes))
}

func badNode(a *App, cx *gk.Cx) {
	a.dock.SetZoomedIn(cx, dock.NodeID(987654321)) // never handed out
}

// --- a saved layout restored through the registry --------------------------------------

func (a *App) restore(cx *gk.Cx, b dock.PanelBuild) {
	ov.builds++
	i := 1
	if b.Name == "first" {
		i = 0
	}
	fmt.Printf("overlay layout build %s n=%d dock=%v\n", b.Name, ov.builds, b.Dock == a.dock)
	a.panels[i] = b.Panel
	a.bodies[i].text = "restored " + b.Name
	info := ""
	if pi, ok := b.State.Info.(dock.PanelInfoPanel); ok {
		info = pi.JSON // the panel's own saved data
	}
	dock.PanelFill(cx, b.Panel, gk.NewView(nil, a.bodies[i].view), dock.PanelData{Name: b.Name, Title: b.Name, Info: info},
		dock.PanelEvents(func(cx *gk.Cx, e dock.PanelEvent) { a.panelEvent(cx, e, i) }))
}

func printState(st dock.PanelState) string {
	kind := "panel"
	switch v := st.Info.(type) {
	case dock.PanelInfoStack:
		kind = fmt.Sprintf("stack%d", len(v.Sizes))
	case dock.PanelInfoTabs:
		kind = fmt.Sprintf("tabs@%d", v.ActiveIndex)
	case dock.PanelInfoPanel:
		kind = fmt.Sprintf("panel'%s'", v.JSON)
	}
	out := fmt.Sprintf("%s:%s(", st.PanelName, kind)
	for _, c := range st.Children {
		out += printState(c) + " "
	}
	return out + ")"
}

func layoutSave(a *App, cx *gk.Cx) {
	dock.SetPanel(cx, a.panels[0], dock.PanelData{Name: "first", Title: "first", Info: `{"file":"a.demo"}`})
}

func layoutLoad(a *App, cx *gk.Cx) {
	saved := a.dock.Dump(cx)
	ov.saved = printState(saved.Center)
	fmt.Printf("overlay layout saved %s\n", ov.saved)
	dock.RegisterPanel(cx, "first", a.restore)
	dock.RegisterPanel(cx, "second", a.restore)
	ov.builds = 0
	err := a.dock.Load(cx, saved)
	fmt.Printf("overlay layout loaded failed=%v err=%s\n", err != nil, errText(err))
}

func layoutCheck(a *App, cx *gk.Cx) {
	again := printState(a.dock.Dump(cx).Center)
	fmt.Printf("overlay layout again same=%v described=%d,%d\n", again == ov.saved, a.bodies[0].described, a.bodies[1].described)
	tree, _ := a.dock.Layout(cx, dock.PlacementCenter).Get()
	st := dock.PaneTreeToState(cx, tree, a.dock)
	t2 := dock.PaneTreeFromState(cx, a.dock, st, dock.RootKindSplit)
	fmt.Printf("overlay layout tree to_state=%s from_state_panels=%d\n", printState(st), len(t2.Panels(cx)))
	p, ok := dock.BuildPanel(cx, a.dock, dock.PanelState{PanelName: "second", Info: dock.PanelInfoPanel{}})
	_, ok2 := dock.BuildPanel(cx, a.dock, dock.PanelState{PanelName: "nobody", Info: dock.PanelInfoPanel{}})
	fmt.Printf("overlay layout build_panel known=%v unknown=%v\n", ok && p != (dock.Panel{}), ok2)
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
	ov.nav = base.NewNavStackState(cx)
	ov.nav.Push(cx, ov.pageV[0], base.NavMotionImmediate)
	ov.nav.Push(cx, ov.pageV[1], base.NavMotionImmediate)
}

func navCheck(a *App, cx *gk.Cx) {
	cur, ok := ov.nav.Current(cx).Get()
	vs := ov.nav.Views(cx)
	fmt.Printf("overlay nav depth=%d current=%v views=%d,%v\n", ov.nav.Depth(cx), ok && cur == ov.pageV[1],
		len(vs), len(vs) == 2 && vs[0] == ov.pageV[0])
	popped, pok := ov.nav.Pop(cx, base.NavMotionImmediate).Get()
	cur2, _ := ov.nav.Current(cx).Get()
	fmt.Printf("overlay nav popped=%v now=%v forward=%d\n", pok && popped == ov.pageV[1], cur2 == ov.pageV[0],
		len(ov.nav.ForwardViews(cx)))
	bar := kit.NewAppMenuBar(cx)
	bar.Reload(cx)
	ov.bar = bar
	r := base.NewResizableState(cx)
	fmt.Printf("overlay done mode=nav sizes=%d container=%v\n", len(r.Sizes(cx)), r.ContainerSize(cx))
}

func navRoot(a *App, cx *gk.Cx) gk.Element {
	if ov.nav == (base.NavStackState{}) {
		return gk.Div(s.SizeFull)
	}
	root := gk.Div(s.SizeFull).Child(base.NavStack(ov.nav))
	if ov.bar != (kit.AppMenuBar{}) {
		root = root.Child(kit.AppMenuBarView(ov.bar))
	}
	return root
}

// --- PROTOCOLS.md 11.9: a dock drawn by the app's renderer -------------------------------

func (c *Chrome) view(cx *gk.Cx) gk.Element {
	c.described++
	return gk.VFlex(s.SizeFull).Child(kit.Label(fmt.Sprintf("%s open=%v", dockPlacementName(c.placement), c.open))).Child(dock.Slot())
}

func (a *App) tabbarView(cx *gk.Cx) gk.Element {
	ov.tabbars++
	return gk.HFlex(s.WFull).Child(kit.Label("tabs of the app"))
}

func (a *App) frameView(cx *gk.Cx) gk.Element {
	ov.frames++
	if a.mode == "render-bad-frame" {
		return kit.Label("a frame must be a div") // misuse: a frame hook's view is a gk.Div()
	}
	return gk.HFlex(s.SizeFull, s.P1)
}

func (a *App) asked(cx *gk.Cx, q dock.Render) {
	ov.asks[q.Hook]++
	switch q.Hook {
	case dock.HookFrame:
		dock.RenderAnswer(cx, q.Renderer, q.Hook, q.Key, ov.frameV)
	case dock.HookDock:
		ov.chrome.placement = q.Dock.Placement(cx)
		ov.chrome.open = q.Dock.IsOpen(cx)
		ov.dockCtx = q.Dock
		fmt.Printf("overlay render asked Dock placement=%s open=%v key=%d same=%v\n", dockPlacementName(q.Placement), ov.chrome.open, q.Key,
			q.Placement == ov.chrome.placement)
		dock.RenderAnswer(cx, q.Renderer, q.Hook, q.Key, ov.chromeV)
	case dock.HookTabBar:
		fmt.Printf("overlay render asked Tab_Bar panels=%d\n", len(q.Group.Panels(cx)))
		dock.RenderAnswer(cx, q.Renderer, q.Hook, q.Key, ov.tabbarV)
	}
}

func renderMake(a *App, cx *gk.Cx) {
	a.bodies = [2]*Body{{text: "first"}, {text: "second"}}
	ov.chromeV = gk.NewView(nil, ov.chrome.view)
	ov.tabbarV = gk.NewView(nil, a.tabbarView)
	ov.frameV = gk.NewView(nil, a.frameView)
	ov.renderer = dock.NewRenderer(cx, dock.Hooks(dock.HookFrame, dock.HookDock, dock.HookTabBar), a.asked)
	a.dock = dock.NewArea(cx, "rendered", gk.Option[int]{}).WithRenderer(ov.renderer).Build()
	for i := range 2 {
		name := "second"
		if i == 0 {
			name = "first"
		}
		v := gk.NewView(nil, a.bodies[i].view)
		a.panels[i] = dock.NewPanel(cx, v, dock.PanelData{Name: name, Title: a.bodies[i].text})
	}
	a.dock.SetCenter(cx, dock.NewLayoutTabs().PanelView(a.panels[0]))
	a.dock.SetDock(cx, dock.PlacementLeft, dock.NewLayoutTabs().PanelView(a.panels[1]))
	ov.toggle = dock.OnDockToggle(cx, a.dock, func(cx *gk.Cx, t dock.Toggle) { ov.toggles++ })
	fmt.Println("overlay render made")
}

func renderCheck(a *App, cx *gk.Cx) {
	_, answers, drawn := dock.TestDockRender(cx, ov.renderer)
	fmt.Printf("overlay render frame=%d dock=%d tab_bar=%d answers=%d drawn_some=%v\n", ov.asks[dock.HookFrame], ov.asks[dock.HookDock],
		ov.asks[dock.HookTabBar], answers, drawn > 0)
	fmt.Printf("overlay render described chrome=%v tabbar=%v frame=%v bodies=%v\n", ov.chrome.described > 0, ov.tabbars > 0,
		ov.frames > 0, a.bodies[0].described > 0 && a.bodies[1].described > 0)
	ov.dockCtx.Toggle(cx) // the chrome's own call: the dock closes, its context changes, it is asked again
}

func renderDone(a *App, cx *gk.Cx) {
	// the kit reports a toggle of the user's (the skin's buttons) only: the chrome's call is the app's
	fmt.Printf("overlay done mode=render open=%v dock_asks=%d chrome_open=%v toggles=%d sub=%v\n", a.dock.IsDockOpen(cx, dock.PlacementLeft),
		ov.asks[dock.HookDock], ov.chrome.open, ov.toggles, ov.toggle != gk.Subscription{})
}

// --- PROTOCOLS.md 11.10: the toast manager on the library clock ---------------------------

// toastStatusName prints a ToastTransitionStatus as the checks expect (the variant's name).
func toastStatusName(t overlay.ToastTransitionStatus) string {
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
	ov.toasts = overlay.NewToastManager(cx, overlay.ToastMotion{Duration: 100 * time.Millisecond, ExitDuration: 50 * time.Millisecond})
	t0 := kit.ClockNow(cx)
	ov.toasts.Push(cx, 1, 100, overlay.ToastOptions{Timeout: gk.Some(300 * time.Millisecond)}, t0)
	ov.toasts.Push(cx, 2, 200, overlay.ToastOptions{}, t0)
	replaced := ov.toasts.Push(cx, 2, 201, overlay.ToastOptions{}, t0)
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
	status := overlay.ToastTransitionStatusStarting
	if len(visible) > 0 {
		status = visible[0].V2
	}
	fmt.Printf("overlay toast dismissed=%v visible=%d status=%s empty=%v\n", dismissed, len(visible), toastStatusName(status), ov.toasts.IsEmpty(cx))
	ov.toasts.Push(cx, 3, 300, overlay.ToastOptions{}, t0+600*time.Millisecond)
	all := ov.toasts.Iter(cx)
	gone := ov.toasts.DismissAll(cx, t0+700*time.Millisecond)
	later := kit.ClockNow(cx)
	fmt.Printf("overlay done mode=toast iter=%d dismissed_all=%s clock_moves=%v\n", len(all), joinText(gone), later >= t0)
}

// --- PROTOCOLS.md 11.15: value calls ----------------------------------------------------------

// Value calls: computations over a value the app built (DockSizing, Sankey), run as ambient calls.
func valueCalls(a *App, cx *gk.Cx) {
	sizing := dock.NewSizing(dock.PlacementLeft).WithAreaWidth(1000).WithOppositeDockSize(200)
	big := dock.BaseDockSizingClamp(cx, sizing, 5000)
	small := dock.BaseDockSizingClamp(cx, dock.NewSizing(dock.PlacementLeft).WithAreaWidth(1000).WithOppositeDockSize(200), 10)
	from := dock.BaseDockSizingSizeFromPointer(cx, dock.NewSizing(dock.PlacementLeft).WithAreaWidth(1000), gk.GpuiPoint{X: 300, Y: 10})
	fmt.Printf("overlay values clamp=%.0f,%.0f pointer=%.0f\n", big, small, from)
	links := []chart.SankeyLink{chart.NewSankeyLink(0, 1, 5), chart.NewSankeyLink(1, 2, 5)}
	g := chart.BaseSankeyLayout(cx, chart.NewSankey().Size(300, 200), 3, links)
	layers := -1
	if graph, ok := g.Value.Get(); ok {
		layers = graph.LayerCount(cx)
	}
	bad := chart.BaseSankeyLayout(cx, chart.NewSankey(), 1, links) // a link to a node that is not there
	fmt.Printf("overlay done mode=values ok=%v layers=%d bad_ok=%v bad_err=%v\n", g.OK, layers, bad.OK, len(bad.Err) > 0)
}
