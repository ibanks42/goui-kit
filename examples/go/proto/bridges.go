package main

// Smoke tests of the Go package's hand-written bridges (go/gk/rt_dock.go, rt_table.go,
// rt_motion.go, rt_kit.go, rt_select.go, rt_editor.go), alongside core/tests/go/overlay (the
// dock's skin, ids and saved layouts) and core/tests/go/rows (tables). gates/go-proto.sh checks the
// lines they print (the bridge-* modes).
//
//	GK_PROTO=bridge-dock | bridge-kit

import (
	"fmt"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

type bridges struct {
	mode   string
	frame  int
	root   gk.View
	dock   gk.DockArea
	panels [2]gk.DockPanel
	bodies [2]int
	saved  string
	builds int
	sub    gk.Subscription

	table     gk.TableState
	tableSrc  gk.RowSource
	cells     int
	tree      gk.TreeState
	treeSrc   gk.RowSource
	vsrc      gk.RowSource
	sel       gk.RowSource
	editor    gk.EditorState
	prov      gk.RowSource
	focus     gk.FocusHandle
	scope     gk.TextSelectionScope
	frames    gk.Keyframes
	save      gk.Action
	bar       gk.AppMenuBar
	described map[string]int
	t0        time.Duration
}

// runBridges runs a bridge-* mode; false for any other mode.
func runBridges(mode string) bool {
	if !strings.HasPrefix(mode, "bridge-") {
		return false
	}
	b := &bridges{mode: mode, described: map[string]int{}}
	script := "size 640 480\n" + strings.Repeat("frame\n", 12) + "stat\n"
	gk.RunHeadless(script, b.start)
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

// --- the dock: the skin's settings, ids, the pane tree, a saved layout --------------------------

func (b *bridges) body(i int) func(cx *gk.Cx) gk.Element {
	return func(cx *gk.Cx) gk.Element {
		b.bodies[i]++
		return gk.Div(s.SizeFull).Child(gk.Label(fmt.Sprintf("body %d", i)))
	}
}

func (b *bridges) dockInit(cx *gk.Cx) {
	b.dock = gk.NewDockAreaSkinned(cx, "bridges")
	for i, name := range []string{"first", "second"} {
		b.panels[i] = gk.NewPanel(cx, gk.NewView(nil, b.body(i)), gk.PanelData{Name: name, Title: name})
	}
	b.dock.SetCenter(cx, gk.NewDockLayoutTabs().PanelView(b.panels[0]).PanelView(b.panels[1]))
	b.sub = gk.OnDockToggle(cx, b.dock, func(cx *gk.Cx, e gk.DockToggle) {})
	gk.OnFrame(cx, b.dockFrame)
	fmt.Printf("bridges dock made sub=%v\n", b.sub != gk.Subscription{})
}

func (b *bridges) dockView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(gk.DockView(b.dock))
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
	out := st.PanelName + ":" + kind + "("
	for _, c := range st.Children {
		out += printState(c) + " "
	}
	return out + ")"
}

func (b *bridges) restore(cx *gk.Cx, pb gk.PanelBuild) {
	b.builds++
	i := 0
	if pb.Name == "second" {
		i = 1
	}
	fmt.Printf("bridges layout build %s n=%d dock=%v\n", pb.Name, b.builds, pb.Dock == b.dock)
	b.panels[i] = pb.Panel
	info := ""
	if pi, ok := pb.State.Info.(gk.PanelInfoPanel); ok {
		info = pi.JSON
	}
	gk.PanelFill(cx, pb.Panel, gk.NewView(nil, b.body(i)), gk.PanelData{Name: pb.Name, Title: pb.Name, Info: info})
}

func (b *bridges) dockFrame(cx *gk.Cx) {
	b.frame++
	switch b.frame {
	case 2:
		// The skin's settings, buffered, then read back (PROTOCOLS.md 11.8; DockSkin).
		gk.SetDockSkinPanelStyle(cx, b.dock, gk.PanelStyleTabBar)
		gk.SetDockSkinToggleButtonVisible(cx, b.dock, false)
		gk.SetDockSkinCloseButtonVisible(cx, b.dock, false)
		gk.SetDockSkinCollapsedTabBarVisible(cx, b.dock, true)
		gk.SetDockSkinSplitDividerLineVisible(cx, b.dock, false)
	case 3:
		fmt.Printf("bridges skin style=%d toggle=%v collapsed=%v divider=%v\n", gk.DockSkinPanelStyle(cx, b.dock),
			gk.DockSkinToggleButtonVisible(cx, b.dock), gk.DockSkinCollapsedTabBarVisible(cx, b.dock),
			gk.DockSkinSplitDividerLineVisible(cx, b.dock))
		g, ok := gk.PanelGroup(cx, b.panels[0])
		ps := g.Panels(cx)
		p, found := b.dock.Panel(cx, gk.PanelID(cx, b.panels[1])).Get()
		fmt.Printf("bridges ids group=%v panels=%d same=%v found=%v\n", ok, len(ps), len(ps) == 2 && ps[0] == b.panels[0], found && p == b.panels[1])
		tree, ok := b.dock.Layout(cx, gk.DockPlacementCenter).Get()
		if ok {
			walk := gk.PaneNodeWalk(cx, tree.Root(cx))
			sp, isSplit := gk.PaneNodeKind(cx, tree.Root(cx)).(gk.PaneSplit)
			tabs, isTabs := gk.PaneNodeKind(cx, walk[len(walk)-1]).(gk.PaneTabs)
			fmt.Printf("bridges tree split=%v children=%d last_tabs=%v panels=%d walk=%d nodes=%d\n", isSplit, len(sp.Children),
				isTabs, len(tabs.Panels), len(walk), len(tree.NodeIDs(cx)))
		}
		gk.SetPanel(cx, b.panels[0], gk.PanelData{Name: "first", Title: "first", Info: `{"file":"a.go"}`})
	case 4:
		saved := b.dock.Dump(cx)
		b.saved = printState(saved.Center)
		fmt.Printf("bridges layout saved %s\n", b.saved)
		gk.RegisterPanel(cx, "first", b.restore)
		gk.RegisterPanel(cx, "second", b.restore)
		err := b.dock.Load(cx, saved)
		fmt.Printf("bridges layout loaded err=%v\n", err)
	case 6:
		again := printState(b.dock.Dump(cx).Center)
		fmt.Printf("bridges layout again same=%v builds=%d described=%v\n", again == b.saved, b.builds, b.bodies[0] > 0 && b.bodies[1] > 0)
		if tree, ok := b.dock.Layout(cx, gk.DockPlacementCenter).Get(); ok {
			st := gk.PaneTreeToState(cx, tree, b.dock)
			t2 := gk.PaneTreeFromState(cx, b.dock, st, gk.RootKindSplit)
			fmt.Printf("bridges layout tree to_state=%s from_state_panels=%d\n", printState(st), len(t2.Panels(cx)))
		}
		p, ok := gk.BuildPanel(cx, b.dock, gk.PanelState{PanelName: "second", Info: gk.PanelInfoPanel{}})
		_, ok2 := gk.BuildPanel(cx, b.dock, gk.PanelState{PanelName: "nobody", Info: gk.PanelInfoPanel{}})
		fmt.Printf("bridges layout build_panel known=%v unknown=%v\n", ok && p != gk.DockPanel{}, ok2)
		r := gk.NewDockRenderer(cx, gk.DockHooks(gk.HookFrame, gk.HookDock), func(cx *gk.Cx, q gk.DockRender) {})
		sent, answers, drawn := gk.TestDockRender(cx, r)
		fmt.Printf("bridges renderer sent=%d answers=%d drawn=%d\n", sent, answers, drawn)
		gk.Release(cx, r)
	case 8:
		fmt.Printf("bridges dock builds=%d\n", b.builds)
	}
}

// --- the kit: a table, a tree, a virtual list, motion, Kbd, the code editor, select rules ------

func (b *bridges) cell(cx *gk.Cx, at gk.CellAt) gk.Element {
	b.cells++
	el := gk.Label(fmt.Sprintf("r%dc%d", at.Row, at.Col))
	if at.Col == 0 {
		return gk.RowMenu(el, gk.NewPopupMenu().Label("row"))
	}
	return el
}

func (b *bridges) kitInit(cx *gk.Cx) {
	cols := []gk.TableColumn{gk.NewTableColumn("name", "Name"), gk.NewTableColumn("size", "Size")}
	cols[1].Sort = gk.Some(gk.ColumnSortDefault)
	b.tableSrc = gk.NewTableSource(cx, 50, cols, b.cell, gk.TableEvents(func(cx *gk.Cx, e gk.TableDelegateEvent) {}), gk.Initial(4))
	b.table = gk.NewTableState(cx, b.tableSrc).Build()
	gk.SetTableTexts(cx, b.table, 0, [][]string{{"a", "1"}, {"b", "2"}}, true)
	gk.SetTableRowLabels(cx, b.table, []gk.TableRowLabel{{Row: 0, Label: "first row"}})
	gk.SetTableLook(cx, b.table, gk.TableLook{Fit: []gk.TableFit{{Col: 0, Width: 120}},
		Groups: [][]gk.TableGroup{{{Label: "all", Span: 2}}}, HeaderStyle: gk.Some(gk.NewStyleRefinementDefault())})
	b.tree = gk.NewTreeState(cx).Build()
	gk.SetTreeItems(cx, b.tree, []gk.TreeNode{{ID: "src", Label: "src", Expanded: true}, {ID: "main", Label: "main.go", Depth: 1}})
	b.treeSrc = gk.NewTreeSource(cx, func(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
		e, _ := gk.TreeEntryAt(cx, b.tree, at.Row)
		b.described["tree"]++
		return gk.RowMenuItem(gk.ListItem(gk.Key("t", at.Row)).Child(gk.Label(e.Label)), gk.NewPopupMenu().Label("x"))
	})
	b.vsrc = gk.NewRowSource(cx, 3, func(cx *gk.Cx, at gk.RowAt) gk.Element {
		b.described["virtual"]++
		return gk.Label(fmt.Sprintf("v%d", at.Row))
	})
	b.sel = gk.NewSearchSource(cx, []gk.SearchItem{{Value: "a"}, {Value: "b"}})
	gk.SearchWillChange(cx, b.sel, gk.SearchRules{Max: 1, Blocked: []string{"b"}})
	b.editor = gk.NewEditorState(cx).Build()
	b.prov = gk.EditorProviders(cx, b.editor, func(cx *gk.Cx, e gk.ProviderEvent) {
		if _, ok := e.Request.(gk.ProviderCompletions); ok {
			gk.AnswerCompletions(cx, b.prov, e.ID, gk.CompletionResponse{Items: []gk.CompletionItem{{Label: "fmt", Kind: 9}}})
		}
	}, gk.ProvidersOptions{Which: gk.ProvideCompletions | gk.ProvideHover | gk.ProvideCodeActions})
	b.focus = gk.NewFocusHandle(cx)
	b.scope = gk.NewTextSelectionScopeID(cx)
	b.save = gk.NewAction(cx, "bridges::Save")
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-s", Action: b.save})
	b.bar = gk.NewAppMenuBar(cx)
	gk.OnFrame(cx, b.kitFrame)
}

func (b *bridges) kitView(cx *gk.Cx) gk.Element {
	b.described["root"]++
	box := gk.Div().W(gk.Px(40)).H(gk.Px(20))
	return gk.VFlex(s.SizeFull).
		Child(gk.Div().H(gk.Px(160)).Child(gk.DataTable(b.table))).
		Child(gk.Div().H(gk.Px(60)).Child(gk.Tree(b.tree, b.treeSrc))).
		Child(gk.VVirtualList("v", b.vsrc, [][2]float32{{0, 20}, {0, 20}, {0, 20}}).H(gk.Px(60))).
		Child(gk.HFlex().
			Child(gk.WithAnimation(box, "pulse", gk.Animation{Ms: 300, Repeat: true, Ease: gk.Ease{Kind: gk.EaseCubicBezier, P: [4]float32{0.2, 0, 0.2, 1}}},
				gk.Track{Prop: gk.MotionOpacity, From: float32(0.2), To: float32(1)})).
			Child(gk.Motion(gk.Div().W(gk.Px(40)).H(gk.Px(20)), gk.MotionValue{Prop: gk.MotionHeight,
				Src: gk.ToSpring{ID: "h", Target: 30, Policy: gk.NewSpring(200 * time.Millisecond)}})).
			Child(gk.FocusTrap(gk.Div().Child(gk.Label("trap")), "trap", b.focus)).
			Child(gk.TextSelectionScopeOf(gk.Div().Child(gk.Label("scoped")), b.scope)).
			Child(gk.KbdForAction(b.save)).
			Child(gk.KbdGlobalForAction(b.save)).
			Child(gk.AppMenuBarView(b.bar))).
		Child(gk.CodeEditor(b.editor, gk.CodeEditorOptions{Height: 80}))
}

func (b *bridges) kitFrame(cx *gk.Cx) {
	b.frame++
	switch b.frame {
	case 3:
		b.t0 = gk.ClockNow(cx)
		e, ok := gk.TreeEntryAt(cx, b.tree, 1)
		fmt.Printf("bridges kit cells=%v tree=%v entry=%q depth=%d virtual=%d providers=%v\n", b.cells >= 8, b.described["tree"] > 0 && ok,
			e.Label, e.Depth, b.described["virtual"], b.prov != gk.RowSource{})
		gk.SetTableRows(cx, b.tableSrc, 60)
		gk.InvalidateTable(cx, b.tableSrc, 0, 2)
	case 5:
		fmt.Printf("bridges kit table visible=%v clock_moves=%v\n", b.table.VisibleRangeRows(cx).Hi > 0, gk.ClockNow(cx) >= b.t0)
	}
}
