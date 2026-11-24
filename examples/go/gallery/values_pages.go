// Values and theming gallery pages: theme mode and token edits, chart scales, calendars with
// matchers, cell-backed Settings panels, scrollbars, drag-and-drop, shadows, tooltips, and row menus.
package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/base"
	"github.com/ibanks42/goui-kit/gk/chart"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/s"
	gktable "github.com/ibanks42/goui-kit/gk/table"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type valuesPages struct {
	band            gk.ScaleBand
	linear          chart.ScaleLinear
	fontSize        float32
	radius          float32
	cal             base.CalendarState
	scroll          gk.ScrollHandle
	dropped         string
	preview         gk.View
	dark, compact   gk.Cell[bool]
	name, size      gk.Cell[string]
	zoom            gk.Cell[float64]
	zoomIn, zoomOut gk.Action
	handles         gk.RowSource
	calRange        base.CalendarState
	calDays         base.CalendarState
	cells           gk.RowSource
	table           gktable.State
	tree            gktable.TreeState
	treeRows        gk.RowSource
}

var valuesPageTable = []Page{
	{"values-theme-mode", valuesThemeModePage},
	{"values-theme-edit", valuesThemeEditPage},
	{"values-scales", valuesScalesPage},
	{"calendar-date", calendarDatePage},
	{"values-scroll", valuesScrollPage},
	{"values-settings", valuesSettingsPage},
	{"values-drag", valuesDragPage},
	{"values-shadow", valuesShadowPage},
	{"values-resizable", valuesResizablePage},
	{"values-scrollbar-div", valuesScrollbarDivPage},
	{"values-toolbar", valuesToolbarPage},
	{"comp6-tooltip", comp6Tooltip},
	{"comp6-tooltip-element", comp6TooltipElement},
	{"comp6-resize", comp6Resize},
	{"comp6-text-view", comp6TextView},
	{"comp6-calendar-matcher", comp6CalendarMatcher},
	{"comp6-table-menu", comp6TableMenu},
	{"comp6-tree-menu", comp6TreeMenu},
}

var c6Tree = []gktable.TreeNode{{ID: "a", Label: "alpha"}, {ID: "b", Label: "beta"}, {ID: "c", Label: "gamma"}}

func civil(y int32, m, d uint32) gk.CivilDate { return gk.CivilDate{Year: y, Month: m, Day: d} }

func valuesPagesInit(a *App, cx *gk.Cx) {
	vp := &a.values
	vp.band = gk.NewScaleBand(cx, []string{"a", "b", "c", "d"}, [2]float32{0, 400}).PaddingInner(0.2).Build()
	vp.linear = chart.NewScaleLinear(cx, []float64{0, 10}, [2]float32{0, 200})
	th := gk.ThemeGlobal(cx)
	vp.fontSize, vp.radius = th.FontSize(cx), th.Radius(cx)
	vp.cal = base.NewCalendarState(cx).Build()
	vp.cal.SetDate(cx, base.DateSingle{Value: gk.Some(civil(2024, 2, 29))})
	vp.scroll = gk.NewScrollHandle(cx)
	vp.scroll.SetOffset(cx, gk.GpuiPoint{X: 0, Y: -120})
	vp.dark = gk.NewCell(cx, false)
	vp.compact = gk.NewCell(cx, true)
	vp.name = gk.NewCell(cx, "Ada")
	vp.size = gk.NewCell(cx, "m")
	vp.zoom = gk.NewCell(cx, 1.5)
	vp.dropped = "nothing dropped"
	vp.preview = gk.NewView(nil, func(cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(80)).H(gk.Px(24)).Bg(theme.Primary)
	})
	vp.zoomIn = gk.NewAction(cx, "gallery_menu::MenuZoomIn")
	vp.zoomOut = gk.NewAction(cx, "gallery_menu::MenuZoomOut")
	vp.handles = overlay.Items(cx, a.comp6HandleRow)
	vp.calRange = base.NewCalendarState(cx).
		DisabledMatcher(base.MatcherRange{Value: gk.RangeMatcher{From: gk.Some(civil(2024, 2, 12)), To: gk.Some(civil(2024, 2, 16))}}).Build()
	vp.calRange.SetDate(cx, base.DateSingle{Value: gk.Some(civil(2024, 2, 29))})
	vp.calDays = base.NewCalendarState(cx).Build()
	vp.calDays.SetDisabledMatcher(cx, base.MatcherCustom{Value: []gk.CivilDate{civil(2024, 2, 5), civil(2024, 2, 20)}})
	vp.calDays.SetDate(cx, base.DateSingle{Value: gk.Some(civil(2024, 2, 29))})
	id, name := gktable.NewTableColumn("id", "Id"), gktable.NewTableColumn("name", "Name")
	id.Width = 60
	vp.cells = gktable.NewSource(cx, 4, []gktable.TableColumn{id, name}, a.comp6Cell)
	vp.table = gktable.NewState(cx, vp.cells).Build()
	vp.tree = gktable.NewTreeState(cx).Build()
	gktable.SetTreeItems(cx, vp.tree, c6Tree)
	vp.treeRows = gktable.NewTreeSource(cx, a.comp6TreeRow)
}

func valuesSample() gk.Element {
	return gk.Div(s.Flex, s.FlexCol).Gap(gk.Px(12)).
		Child(kit.Button("primary").Primary().Label("Primary")).
		Child(kit.Button("outline").Outline().Label("Outline")).
		Child(kit.Label("The theme of the whole window")).
		Child(kit.Switch("sw").Checked(true).Label("Switch"))
}

func valuesThemeModePage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div(s.Flex, s.FlexCol).Gap(gk.Px(16)).
		Child(kit.Button("toggle").Label("Toggle").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
			if gk.ThemeGlobal(cx).IsDark(cx) {
				gk.ThemeChange(cx, gk.ThemeModeLight)
			} else {
				gk.ThemeChange(cx, gk.ThemeModeDark)
			}
		})).
		Child(valuesSample())
}

func valuesThemeEditPage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div(s.Flex, s.FlexCol).Gap(gk.Px(16)).
		Child(kit.Button("toggle").Label("Toggle").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
			th := gk.ThemeGlobal(cx)
			if th.Radius(cx) == a.values.radius {
				th.SetRadius(cx, 0)
				th.SetFontSize(cx, 20)
			} else {
				th.SetRadius(cx, a.values.radius)
				th.SetFontSize(cx, a.values.fontSize)
			}
		})).
		Child(valuesSample())
}

func valuesScalesPage(a *App, cx *gk.Cx) gk.Element {
	names := [4]string{"a", "b", "c", "d"}
	vals := [4]float64{3, 7, 5, 9}
	plot := gk.Div(s.Relative).W(gk.Px(400)).H(gk.Px(200)).Bg(theme.Muted)
	w := a.values.band.BandWidth(cx)
	for i, n := range names {
		x, _ := a.values.band.Tick(cx, n).Get()
		h, _ := a.values.linear.Tick(cx, vals[i]).Get()
		plot = plot.Child(gk.Div(s.Absolute).Left(gk.Px(x)).Bottom(gk.Px(0)).W(gk.Px(w)).H(gk.Px(h)).Bg(theme.Primary))
	}
	return plot
}

func calendarDatePage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(320)).Child(kit.Calendar(a.values.cal))
}

func valuesSettingsPage(a *App, cx *gk.Cx) gk.Element {
	p := &a.values
	look := kit.NewSettingGroup().Title("Look").
		Item(kit.NewSettingItem("Dark", kit.NewSettingFieldBoolSwitch(p.dark, func(cx *gk.Cx, v bool) { gk.CellSet(cx, p.dark, v) }).DefaultValue(false))).
		Item(kit.NewSettingItem("Compact", kit.NewSettingFieldBoolCheckbox(p.compact, func(cx *gk.Cx, v bool) { gk.CellSet(cx, p.compact, v) }))).
		Item(kit.NewSettingItem("Name", kit.NewSettingFieldTextInput(p.name, func(cx *gk.Cx, v string) { gk.CellSet(cx, p.name, v) }))).
		Item(kit.NewSettingItem("Size", kit.NewSettingFieldTextDropdown([]gk.TupleStringString{{V0: "s", V1: "Small"}, {V0: "m", V1: "Medium"}}, p.size,
			func(cx *gk.Cx, v string) { gk.CellSet(cx, p.size, v) }))).
		Item(kit.NewSettingItem("Zoom", kit.NewSettingFieldNumberNumberInput(kit.NumberFieldOptions{Min: 0, Max: 4, Step: 0.5}, p.zoom,
			func(cx *gk.Cx, v float64) { gk.CellSet(cx, p.zoom, v) }))).
		Item(kit.NewSettingItemRender(gk.Div().Child(kit.Label("Rendered by the app"))))
	return gk.Div().W(gk.Px(760)).H(gk.Px(480)).
		Child(kit.Settings("settings").SidebarSizeRange([2]float32{100, 300}).Page(kit.NewSettingPage("General").Group(look)))
}

func valuesScrollPage(a *App, cx *gk.Cx) gk.Element {
	list := gk.Div(s.SizeFull).ID("list").OverflowYScroll().TrackScroll(a.values.scroll)
	for i := range 30 {
		list = list.Child(gk.Div().H(gk.Px(24)).Child(gk.Text(fmt.Sprintf("row %d", i))))
	}
	return gk.Div(s.Relative).W(gk.Px(300)).H(gk.Px(160)).Child(list).Child(gk.ScrollbarVertical(a.values.scroll))
}

func valuesDragPage(a *App, cx *gk.Cx) gk.Element {
	src := gk.Div().ID("src").W(gk.Px(120)).H(gk.Px(40)).Bg(theme.Muted).
		OnDrag(gk.DragValue{ID: 7, Text: "row 7"}, a.values.preview).Child(kit.Label("drag me"))
	dst := gk.Div().ID("dst").W(gk.Px(200)).H(gk.Px(120)).Bg(theme.Secondary).
		DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent)).
		OnDrop(func(cx *gk.Cx, d gk.DragValue) { a.values.dropped = fmt.Sprintf("dropped %d: %s", d.ID, d.Text) }).
		Child(kit.Label(a.values.dropped))
	return gk.Div(s.Flex).Gap(gk.Px(80)).Child(src).Child(dst)
}

func valuesShadowPage(a *App, cx *gk.Cx) gk.Element {
	bs := gk.BaseBoxShadow(cx, 4, 6, 12, 2, gk.HSLA(0.6, 0.8, 0.3, 0.6))
	return gk.Div().P(gk.Px(24)).Child(gk.Div().W(gk.Px(160)).H(gk.Px(80)).Bg(theme.Background).Shadow([]gk.BoxShadow{bs}))
}

func valuesResizablePage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(200)).Child(base.KitHResizable("rz").
		Child(base.ResizablePanel().Size(200).Child(gk.Div(s.SizeFull).Bg(theme.Muted).Child(kit.Label("left")))).
		Child(base.ResizablePanel().Child(gk.Div(s.SizeFull).Child(kit.Label("right")))))
}

func valuesScrollbarDivPage(a *App, cx *gk.Cx) gk.Element {
	list := gk.Div().W(gk.Px(300)).H(gk.Px(160))
	for i := range 20 {
		list = list.Child(gk.Div().H(gk.Px(24)).Child(gk.Text(fmt.Sprintf("item %d", i))))
	}
	return kit.DivOverflowYScrollbar(list).ID("sd")
}

func valuesToolbarPage(a *App, cx *gk.Cx) gk.Element {
	return kit.Toolbar("tb").Child(kit.Button("b1").Label("Bold")).Child(kit.Button("b2").Label("Italic")).
		Children([]gk.SizableNode{kit.Button("b3").Label("Code"), kit.Button("b4").Label("Wrap")})
}

func comp6Tooltip(a *App, cx *gk.Cx) gk.Element {
	tip := overlay.NewKitTooltip("Save the file").KeyBinding(gk.Some(kit.Kbd(gk.Keystroke{Modifiers: gk.Modifiers{Control: true}, Key: "s"})))
	return gk.Div().ID("c6tip").W(gk.Px(160)).H(gk.Px(40)).Bg(theme.Muted).Tooltip(tip).Child(kit.Label("Hover me"))
}

func comp6TooltipElement(a *App, cx *gk.Cx) gk.Element {
	tip := overlay.NewKitTooltipElement(gk.HFlex(s.Gap1).Child(kit.Label("Zoom")).Child(kit.Label("in"))).Action(a.values.zoomIn, gk.Option[string]{})
	return gk.Div().ID("c6tipe").W(gk.Px(160)).H(gk.Px(40)).Bg(theme.Muted).Tooltip(tip).Child(kit.Label("Hover me"))
}

func comp6TextView(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).W(gk.Px(400)).
		Child(kit.Checkbox("c6cb").Checked(true).Label(editor.TextViewMarkdown("c6md1", "A **bold** label"))).
		Child(kit.Alert("c6al", editor.TextViewMarkdown("c6md2", "An alert in *Markdown*")))
}

func (a *App) comp6HandleRow(cx *gk.Cx, ctx base.ResizeHandleContext) gk.Element {
	color := theme.Border
	if ctx.IsActive {
		color = theme.Primary
	}
	return gk.Div(s.HFull).W(gk.Px(3)).Bg(color)
}

func comp6Resize(a *App, cx *gk.Cx) gk.Element {
	group := func(id string, r gk.ResizeHandleRenderer) gk.Element {
		return gk.Div().W(gk.Px(600)).H(gk.Px(100)).Child(base.HResizable(id).WithHandleAppearance(r).
			Child(base.ResizablePanel().Size(200).Child(kit.Label("left"))).
			Child(base.ResizablePanel().Child(kit.Label("right"))))
	}
	alone := gk.Div(s.Relative).W(gk.Px(600)).H(gk.Px(40)).Bg(theme.Muted).
		Child(base.ResizeHandle("c6h", gk.AxisHorizontal).WithAppearance(a.values.handles))
	return gk.VFlex(s.Gap4).Child(group("c6rz", a.values.handles)).
		Child(group("c6rk", gk.ResizeHandleRendererKitResizeHandleAppearance)).Child(alone)
}

func comp6CalendarMatcher(a *App, cx *gk.Cx) gk.Element {
	return gk.Div(s.Flex, s.Gap4).Child(gk.Div().W(gk.Px(320)).Child(kit.Calendar(a.values.calRange))).
		Child(gk.Div().W(gk.Px(320)).Child(kit.Calendar(a.values.calDays)))
}

func (a *App) comp6Cell(cx *gk.Cx, at gktable.CellAt) gk.Element {
	el := kit.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
	if at.Col != 0 {
		return el
	}
	return kit.RowMenu(el, kit.NewPopupMenu().Menu(fmt.Sprintf("Open row %d", at.Row), a.values.zoomIn).
		Menu(fmt.Sprintf("Delete row %d", at.Row), a.values.zoomOut))
}

func comp6TableMenu(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(300)).H(gk.Px(200)).Child(gktable.DataTable(a.values.table).ScrollbarVisible(false, false))
}

func (a *App) comp6TreeRow(cx *gk.Cx, at gk.RowAt) gktable.ListItemEl {
	label := ""
	if e, ok := gktable.TreeEntryAt(cx, a.values.tree, at.Row); ok {
		label = e.Label
	}
	item := gktable.ListItem(gk.Key("c6tree", at.Row)).Child(kit.Label(label))
	return kit.RowMenuItem(item, kit.NewPopupMenu().Menu(fmt.Sprintf("Rename %s", label), a.values.zoomIn))
}

func comp6TreeMenu(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(300)).H(gk.Px(160)).Child(gktable.Tree(a.values.tree, a.values.treeRows).ContextMenu(a.values.treeRows))
}
