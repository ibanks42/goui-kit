// Smoke tests for hand-written bridges: dock panels, layout save/restore, dock renderers,
// table/tree/virtual-list sources, editor providers, motion wrappers, and app menus.
package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gpuikit.dev/gk"
	gkdock "gpuikit.dev/gk/dock"
	gkeditor "gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/motion"
	"gpuikit.dev/gk/s"
	gktable "gpuikit.dev/gk/table"
)

type bridges struct {
	mode   string
	frame  int
	root   gk.View
	dock   gkdock.Area
	panels [2]gkdock.Panel
	bodies [2]int
	saved  string
	builds int
	sub    gk.Subscription

	table     gktable.State
	tableSrc  gk.RowSource
	cells     int
	tree      gktable.TreeState
	treeSrc   gk.RowSource
	vsrc      gk.RowSource
	sel       gk.RowSource
	editor    gkeditor.State
	prov      gk.RowSource
	focus     gk.FocusHandle
	scope     motion.TextSelectionScope
	frames    motion.Keyframes
	save      gk.Action
	bar       kit.AppMenuBar
	described map[string]int
	t0        time.Duration
}

func runBridges(mode string) bool {
	if !strings.HasPrefix(mode, "bridge-") {
		return false
	}
	b := &bridges{mode: mode, described: map[string]int{}}
	script := "size 640 480\n" + strings.Repeat("frame\n", 12) + "stat\n"
	if err := gk.RunHeadless(script, b.start); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("bridges done mode=%s frames=%d\n", mode, b.frame)
	return true
}

func (b *bridges) start(cx *gk.Cx) {
	switch b.mode {
	case "bridge-dock":
		b.root = gk.NewView(b.dockInit, b.dockView)
	default:
		b.root = gk.NewView(b.kitInit, b.kitView)
	}
	gk.OpenWindow(cx, "bridges", 640, 480, b.root)
}

func (b *bridges) body(i int) func(cx *gk.Cx) gk.Element {
	return func(cx *gk.Cx) gk.Element {
		b.bodies[i]++
		return gk.Div(s.SizeFull).Child(kit.Label(fmt.Sprintf("body %d", i)))
	}
}

func (b *bridges) dockInit(cx *gk.Cx) {
	b.dock = gkdock.NewAreaSkinned(cx, "bridges")
	for i, name := range []string{"first", "second"} {
		b.panels[i] = gkdock.NewPanel(cx, gk.NewView(nil, b.body(i)), gkdock.PanelData{Name: name, Title: name})
	}
	b.dock.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(b.panels[0]).PanelView(b.panels[1]))
	b.sub = gkdock.OnDockToggle(cx, b.dock, func(cx *gk.Cx, e gkdock.Toggle) {})
	gk.OnFrame(cx, b.dockFrame)
	fmt.Printf("bridges dock made sub=%v\n", b.sub != gk.Subscription{})
}

func (b *bridges) dockView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(gkdock.View(b.dock))
}

func printState(st gkdock.PanelState) string {
	kind := "panel"
	switch v := st.Info.(type) {
	case gkdock.PanelInfoStack:
		kind = fmt.Sprintf("stack%d", len(v.Sizes))
	case gkdock.PanelInfoTabs:
		kind = fmt.Sprintf("tabs@%d", v.ActiveIndex)
	case gkdock.PanelInfoPanel:
		kind = fmt.Sprintf("panel'%s'", v.JSON)
	}
	out := st.PanelName + ":" + kind + "("
	for _, c := range st.Children {
		out += printState(c) + " "
	}
	return out + ")"
}

func (b *bridges) restore(cx *gk.Cx, pb gkdock.PanelBuild) {
	b.builds++
	i := 0
	if pb.Name == "second" {
		i = 1
	}
	fmt.Printf("bridges layout build %s n=%d dock=%v\n", pb.Name, b.builds, pb.Dock == b.dock)
	b.panels[i] = pb.Panel
	info := ""
	if pi, ok := pb.State.Info.(gkdock.PanelInfoPanel); ok {
		info = pi.JSON
	}
	gkdock.PanelFill(cx, pb.Panel, gk.NewView(nil, b.body(i)), gkdock.PanelData{Name: pb.Name, Title: pb.Name, Info: info})
}

func (b *bridges) dockFrame(cx *gk.Cx) {
	b.frame++
	switch b.frame {
	case 2:
		gkdock.SetDockSkinPanelStyle(cx, b.dock, gkdock.PanelStyleTabBar)
		gkdock.SetDockSkinToggleButtonVisible(cx, b.dock, false)
		gkdock.SetDockSkinCloseButtonVisible(cx, b.dock, false)
		gkdock.SetDockSkinCollapsedTabBarVisible(cx, b.dock, true)
		gkdock.SetDockSkinSplitDividerLineVisible(cx, b.dock, false)
	case 3:
		fmt.Printf("bridges skin style=%d toggle=%v collapsed=%v divider=%v\n", gkdock.SkinPanelStyle(cx, b.dock),
			gkdock.SkinToggleButtonVisible(cx, b.dock), gkdock.SkinCollapsedTabBarVisible(cx, b.dock),
			gkdock.SkinSplitDividerLineVisible(cx, b.dock))
		g, ok := gkdock.PanelGroup(cx, b.panels[0])
		ps := g.Panels(cx)
		p, found := b.dock.Panel(cx, gkdock.PanelID(cx, b.panels[1])).Get()
		fmt.Printf("bridges ids group=%v panels=%d same=%v found=%v\n", ok, len(ps), len(ps) == 2 && ps[0] == b.panels[0], found && p == b.panels[1])
		tree, ok := b.dock.Layout(cx, gkdock.PlacementCenter).Get()
		if ok {
			walk := gkdock.PaneNodeWalk(cx, tree.Root(cx))
			sp, isSplit := gkdock.PaneNodeKind(cx, tree.Root(cx)).(gkdock.PaneSplit)
			tabs, isTabs := gkdock.PaneNodeKind(cx, walk[len(walk)-1]).(gkdock.PaneTabs)
			fmt.Printf("bridges tree split=%v children=%d last_tabs=%v panels=%d walk=%d nodes=%d\n", isSplit, len(sp.Children),
				isTabs, len(tabs.Panels), len(walk), len(tree.NodeIDs(cx)))
		}
		gkdock.SetPanel(cx, b.panels[0], gkdock.PanelData{Name: "first", Title: "first", Info: `{"file":"a.go"}`})
	case 4:
		saved := b.dock.Dump(cx)
		b.saved = printState(saved.Center)
		fmt.Printf("bridges layout saved %s\n", b.saved)
		gkdock.RegisterPanel(cx, "first", b.restore)
		gkdock.RegisterPanel(cx, "second", b.restore)
		err := b.dock.Load(cx, saved)
		fmt.Printf("bridges layout loaded err=%v\n", err)
	case 6:
		again := printState(b.dock.Dump(cx).Center)
		fmt.Printf("bridges layout again same=%v builds=%d described=%v\n", again == b.saved, b.builds, b.bodies[0] > 0 && b.bodies[1] > 0)
		if tree, ok := b.dock.Layout(cx, gkdock.PlacementCenter).Get(); ok {
			st := gkdock.PaneTreeToState(cx, tree, b.dock)
			t2 := gkdock.PaneTreeFromState(cx, b.dock, st, gkdock.RootKindSplit)
			fmt.Printf("bridges layout tree to_state=%s from_state_panels=%d\n", printState(st), len(t2.Panels(cx)))
		}
		p, ok := gkdock.BuildPanel(cx, b.dock, gkdock.PanelState{PanelName: "second", Info: gkdock.PanelInfoPanel{}})
		_, ok2 := gkdock.BuildPanel(cx, b.dock, gkdock.PanelState{PanelName: "nobody", Info: gkdock.PanelInfoPanel{}})
		fmt.Printf("bridges layout build_panel known=%v unknown=%v\n", ok && p != gkdock.Panel{}, ok2)
		r := gkdock.NewRenderer(cx, gkdock.Hooks(gkdock.HookFrame, gkdock.HookDock), func(cx *gk.Cx, q gkdock.Render) {})
		sent, answers, drawn := gkdock.TestDockRender(cx, r)
		fmt.Printf("bridges renderer sent=%d answers=%d drawn=%d\n", sent, answers, drawn)
		gk.Release(cx, r)
	case 8:
		fmt.Printf("bridges dock builds=%d\n", b.builds)
	}
}

func (b *bridges) cell(cx *gk.Cx, at gktable.CellAt) gk.Element {
	b.cells++
	el := kit.Label(fmt.Sprintf("r%dc%d", at.Row, at.Col))
	if at.Col == 0 {
		return kit.RowMenu(el, kit.NewPopupMenu().Label("row"))
	}
	return el
}

func (b *bridges) kitInit(cx *gk.Cx) {
	cols := []gktable.TableColumn{gktable.NewTableColumn("name", "Name"), gktable.NewTableColumn("size", "Size")}
	cols[1].Sort = gk.Some(gktable.ColumnSortDefault)
	b.tableSrc = gktable.NewSource(cx, 50, cols, b.cell, gktable.Events(func(cx *gk.Cx, e gktable.DelegateEvent) {}), gk.Initial(4))
	b.table = gktable.NewState(cx, b.tableSrc).Build()
	gktable.SetTableTexts(cx, b.table, 0, [][]string{{"a", "1"}, {"b", "2"}}, true)
	gktable.SetTableRowLabels(cx, b.table, []gktable.RowLabel{{Row: 0, Label: "first row"}})
	gktable.SetTableLook(cx, b.table, gktable.Look{Fit: []gktable.Fit{{Col: 0, Width: 120}},
		Groups: [][]gktable.Group{{{Label: "all", Span: 2}}}, HeaderStyle: gk.Some(gk.NewStyleRefinementDefault())})
	b.tree = gktable.NewTreeState(cx).Build()
	gktable.SetTreeItems(cx, b.tree, []gktable.TreeNode{{ID: "src", Label: "src", Expanded: true}, {ID: "main", Label: "main.go", Depth: 1}})
	b.treeSrc = gktable.NewTreeSource(cx, func(cx *gk.Cx, at gk.RowAt) gktable.ListItemEl {
		e, _ := gktable.TreeEntryAt(cx, b.tree, at.Row)
		b.described["tree"]++
		return kit.RowMenuItem(gktable.ListItem(gk.Key("t", at.Row)).Child(kit.Label(e.Label)), kit.NewPopupMenu().Label("x"))
	})
	b.vsrc = gk.NewRowSource(cx, 3, func(cx *gk.Cx, at gk.RowAt) gk.Element {
		b.described["virtual"]++
		return kit.Label(fmt.Sprintf("v%d", at.Row))
	})
	b.sel = gk.NewSearchSource(cx, []gk.SearchItem{{Value: "a"}, {Value: "b"}})
	gk.SearchWillChange(cx, b.sel, gk.SearchRules{Max: 1, Blocked: []string{"b"}})
	b.editor = gkeditor.NewState(cx).Build()
	b.prov = gkeditor.Providers(cx, b.editor, func(cx *gk.Cx, e gkeditor.ProviderEvent) {
		if _, ok := e.Request.(gkeditor.ProviderCompletions); ok {
			gkeditor.AnswerCompletions(cx, b.prov, e.ID, gkeditor.CompletionResponse{Items: []gk.CompletionItem{{Label: "fmt", Kind: 9}}})
		}
	}, gkeditor.ProvidersOptions{Which: gkeditor.ProvideCompletions | gkeditor.ProvideHover | gkeditor.ProvideCodeActions})
	b.focus = gk.NewFocusHandle(cx)
	b.scope = motion.NewTextSelectionScopeID(cx)
	b.save = gk.NewAction(cx, "bridges::Save")
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-s", Action: b.save})
	b.bar = kit.NewAppMenuBar(cx)
	gk.OnFrame(cx, b.kitFrame)
}

func (b *bridges) kitView(cx *gk.Cx) gk.Element {
	b.described["root"]++
	box := gk.Div().W(gk.Px(40)).H(gk.Px(20))
	return gk.VFlex(s.SizeFull).
		Child(gk.Div().H(gk.Px(160)).Child(gktable.DataTable(b.table))).
		Child(gk.Div().H(gk.Px(60)).Child(gktable.Tree(b.tree, b.treeSrc))).
		Child(gk.VVirtualList("v", b.vsrc, [][2]float32{{0, 20}, {0, 20}, {0, 20}}).H(gk.Px(60))).
		Child(gk.HFlex().
			Child(motion.WithAnimation(box, "pulse", motion.Animation{Ms: 300, Repeat: true, Ease: motion.Ease{Kind: motion.EaseCubicBezier, P: [4]float32{0.2, 0, 0.2, 1}}},
				motion.Track{Prop: motion.Opacity, From: float32(0.2), To: float32(1)})).
			Child(motion.Motion(gk.Div().W(gk.Px(40)).H(gk.Px(20)), motion.Value{Prop: motion.Height,
				Src: motion.ToSpring{ID: "h", Target: 30, Policy: motion.NewSpring(200 * time.Millisecond)}})).
			Child(motion.FocusTrap(gk.Div().Child(kit.Label("trap")), "trap", b.focus)).
			Child(motion.TextSelectionScopeOf(gk.Div().Child(kit.Label("scoped")), b.scope)).
			Child(kit.KbdForAction(b.save)).
			Child(kit.KbdGlobalForAction(b.save)).
			Child(kit.AppMenuBarView(b.bar))).
		Child(gkeditor.CodeEditor(b.editor, gkeditor.CodeEditorOptions{Height: 80}))
}

func (b *bridges) kitFrame(cx *gk.Cx) {
	b.frame++
	switch b.frame {
	case 3:
		b.t0 = kit.ClockNow(cx)
		e, ok := gktable.TreeEntryAt(cx, b.tree, 1)
		fmt.Printf("bridges kit cells=%v tree=%v entry=%q depth=%d virtual=%d providers=%v\n", b.cells >= 8, b.described["tree"] > 0 && ok,
			e.Label, e.Depth, b.described["virtual"], b.prov != gk.RowSource{})
		gktable.SetTableRows(cx, b.tableSrc, 60)
		gktable.InvalidateTable(cx, b.tableSrc, 0, 2)
	case 5:
		fmt.Printf("bridges kit table visible=%v clock_moves=%v\n", b.table.VisibleRangeRows(cx).Hi > 0, kit.ClockNow(cx) >= b.t0)
	}
}
