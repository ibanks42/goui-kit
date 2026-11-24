// The values family's gallery pages, the Go twin of
// ref/src/bin/gallery/values_pages.rs, call for call (docs/PROTOCOLS.md sections 13.8 to 13.12):
// the theme's mode switched by a static that takes Option<&mut Window> (Theme::change), the
// theme's fields edited in place (a kit global's public fields, read and written), and bars placed
// by the plot scales instantiated as library objects (ScaleBand, ScaleLinear), made in init and
// read in the view. Each theme page toggles with one button, so its script clicks it twice and the
// theme is as before afterwards.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type valuesPages struct {
	band     gk.ScaleBand
	linear   gk.ScaleLinear
	fontSize float32
	radius   float32
	cal      gk.CalendarState
	scroll   gk.ScrollHandle
	dropped  string
	preview  gk.View
	// the settings page's values, held by the library for the page to read while it draws
	dark, compact gk.Cell[bool]
	name, size    gk.Cell[string]
	zoom          gk.Cell[float64]
	// the components family
	zoomIn, zoomOut gk.Action
	handles         gk.RowSource // comp6-resize: the divider the app draws
	calRange        gk.CalendarState
	calDays         gk.CalendarState
	cells           gk.RowSource // comp6-table-menu
	table           gk.TableState
	tree            gk.TreeState // comp6-tree-menu
	treeRows        gk.RowSource
}

var valuesPageTable = []Page{
	{"values-theme-mode", valuesThemeModePage},
	{"values-theme-edit", valuesThemeEditPage},
	{"values-scales", valuesScalesPage},
	{"calendar-date", calendarDatePage},
	{"values-scroll", valuesScrollPage},
	// docs/PROTOCOLS.md 13.19, 13.20
	{"values-settings", valuesSettingsPage},
	{"values-drag", valuesDragPage},
	{"values-shadow", valuesShadowPage},
	{"values-resizable", valuesResizablePage},
	{"values-scrollbar-div", valuesScrollbarDivPage},
	{"values-toolbar", valuesToolbarPage},
	// the components family, docs/PROTOCOLS.md 11.17 to 11.24
	{"comp6-tooltip", comp6Tooltip},
	{"comp6-tooltip-element", comp6TooltipElement},
	{"comp6-resize", comp6Resize},
	{"comp6-text-view", comp6TextView},
	{"comp6-calendar-matcher", comp6CalendarMatcher},
	{"comp6-table-menu", comp6TableMenu},
	{"comp6-tree-menu", comp6TreeMenu},
}

var c6Tree = []gk.TreeNode{{ID: "a", Label: "alpha"}, {ID: "b", Label: "beta"}, {ID: "c", Label: "gamma"}}

func civil(y int32, m, d uint32) gk.CivilDate { return gk.CivilDate{Year: y, Month: m, Day: d} }

func valuesPagesInit(a *App, cx *gk.Cx) {
	vp := &a.values
	vp.band = gk.NewScaleBand(cx, []string{"a", "b", "c", "d"}, [2]float32{0, 400}).PaddingInner(0.2).Build()
	vp.linear = gk.NewScaleLinear(cx, []float64{0, 10}, [2]float32{0, 200})
	th := gk.ThemeGlobal(cx)
	vp.fontSize, vp.radius = th.FontSize(cx), th.Radius(cx)
	// PROTOCOLS.md 13.13, 13.14
	vp.cal = gk.NewCalendarState(cx).Build()
	vp.cal.SetDate(cx, gk.DateSingle{Value: gk.Some(civil(2024, 2, 29))})
	vp.scroll = gk.NewScrollHandle(cx)
	vp.scroll.SetOffset(cx, gk.GpuiPoint{X: 0, Y: -120})
	// PROTOCOLS.md 13.19, 13.20
	vp.dark = gk.NewCell(cx, false)
	vp.compact = gk.NewCell(cx, true)
	vp.name = gk.NewCell(cx, "Ada")
	vp.size = gk.NewCell(cx, "m")
	vp.zoom = gk.NewCell(cx, 1.5)
	vp.dropped = "nothing dropped"
	vp.preview = gk.NewView(nil, func(cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(80)).H(gk.Px(24)).Bg(theme.Primary)
	})
	// the components family
	vp.zoomIn = gk.NewAction(cx, "gallery_menu::MenuZoomIn") // the Rust twin's actions (overlay_pages.rs)
	vp.zoomOut = gk.NewAction(cx, "gallery_menu::MenuZoomOut")
	vp.handles = gk.Items(cx, a.comp6HandleRow)
	vp.calRange = gk.NewCalendarState(cx).
		DisabledMatcher(gk.MatcherRange{Value: gk.RangeMatcher{From: gk.Some(civil(2024, 2, 12)), To: gk.Some(civil(2024, 2, 16))}}).Build()
	vp.calRange.SetDate(cx, gk.DateSingle{Value: gk.Some(civil(2024, 2, 29))})
	vp.calDays = gk.NewCalendarState(cx).Build()
	vp.calDays.SetDisabledMatcher(cx, gk.MatcherCustom{Value: []gk.CivilDate{civil(2024, 2, 5), civil(2024, 2, 20)}})
	vp.calDays.SetDate(cx, gk.DateSingle{Value: gk.Some(civil(2024, 2, 29))})
	id, name := gk.NewTableColumn("id", "Id"), gk.NewTableColumn("name", "Name")
	id.Width = 60
	vp.cells = gk.NewTableSource(cx, 4, []gk.TableColumn{id, name}, a.comp6Cell)
	vp.table = gk.NewTableState(cx, vp.cells).Build()
	vp.tree = gk.NewTreeState(cx).Build()
	gk.SetTreeItems(cx, vp.tree, c6Tree)
	vp.treeRows = gk.NewTreeSource(cx, a.comp6TreeRow)
}

func valuesSample() gk.Element {
	return gk.Div(s.Flex, s.FlexCol).Gap(gk.Px(12)).
		Child(gk.Button("primary").Primary().Label("Primary")).
		Child(gk.Button("outline").Outline().Label("Outline")).
		Child(gk.Label("The theme of the whole window")).
		Child(gk.Switch("sw").Checked(true).Label("Switch"))
}

func valuesThemeModePage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div(s.Flex, s.FlexCol).Gap(gk.Px(16)).
		Child(gk.Button("toggle").Label("Toggle").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
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
		Child(gk.Button("toggle").Label("Toggle").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
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

// calendarDatePage: a calendar whose state was given a date (jiff's civil::Date, PROTOCOLS.md 13.13).
func calendarDatePage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(320)).Child(gk.Calendar(a.values.cal))
}

// valuesSettingsPage: the kit's settings page over fields whose values the library holds (value
// cells): each field reads its cell while the page draws, and its handler writes the cell
// between frames.
func valuesSettingsPage(a *App, cx *gk.Cx) gk.Element {
	p := &a.values
	look := gk.NewSettingGroup().Title("Look").
		Item(gk.NewSettingItem("Dark", gk.NewSettingFieldBoolSwitch(p.dark, func(cx *gk.Cx, v bool) { gk.CellSet(cx, p.dark, v) }).DefaultValue(false))).
		Item(gk.NewSettingItem("Compact", gk.NewSettingFieldBoolCheckbox(p.compact, func(cx *gk.Cx, v bool) { gk.CellSet(cx, p.compact, v) }))).
		Item(gk.NewSettingItem("Name", gk.NewSettingFieldTextInput(p.name, func(cx *gk.Cx, v string) { gk.CellSet(cx, p.name, v) }))).
		Item(gk.NewSettingItem("Size", gk.NewSettingFieldTextDropdown([]gk.TupleStringString{{V0: "s", V1: "Small"}, {V0: "m", V1: "Medium"}}, p.size,
			func(cx *gk.Cx, v string) { gk.CellSet(cx, p.size, v) }))).
		Item(gk.NewSettingItem("Zoom", gk.NewSettingFieldNumberNumberInput(gk.NumberFieldOptions{Min: 0, Max: 4, Step: 0.5}, p.zoom,
			func(cx *gk.Cx, v float64) { gk.CellSet(cx, p.zoom, v) }))).
		Item(gk.NewSettingItemRender(gk.Div().Child(gk.Label("Rendered by the app"))))
	return gk.Div().W(gk.Px(760)).H(gk.Px(480)).
		Child(gk.Settings("settings").SidebarSizeRange([2]float32{100, 300}).Page(gk.NewSettingPage("General").Group(look)))
}

// valuesScrollPage: a list scrolled from code through gpui's ScrollHandle, a scrollbar over it
// (PROTOCOLS.md 13.14).
func valuesScrollPage(a *App, cx *gk.Cx) gk.Element {
	list := gk.Div(s.SizeFull).ID("list").OverflowYScroll().TrackScroll(a.values.scroll)
	for i := range 30 {
		list = list.Child(gk.Div().H(gk.Px(24)).Child(gk.Text(fmt.Sprintf("row %d", i))))
	}
	return gk.Div(s.Relative).W(gk.Px(300)).H(gk.Px(160)).Child(list).Child(gk.ScrollbarVertical(a.values.scroll))
}

// valuesDragPage: drag and drop over the library's drag value: a source with a preview view, a
// target that takes the drop and shows a style while the value is over it.
func valuesDragPage(a *App, cx *gk.Cx) gk.Element {
	src := gk.Div().ID("src").W(gk.Px(120)).H(gk.Px(40)).Bg(theme.Muted).
		OnDrag(gk.DragValue{ID: 7, Text: "row 7"}, a.values.preview).Child(gk.Label("drag me"))
	dst := gk.Div().ID("dst").W(gk.Px(200)).H(gk.Px(120)).Bg(theme.Secondary).
		DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent)).
		OnDrop(func(cx *gk.Cx, d gk.DragValue) { a.values.dropped = fmt.Sprintf("dropped %d: %s", d.ID, d.Text) }).
		Child(gk.Label(a.values.dropped))
	return gk.Div(s.Flex).Gap(gk.Px(80)).Child(src).Child(dst)
}

// valuesShadowPage: a shadow made by box_shadow, read back as a record and given to shadow().
func valuesShadowPage(a *App, cx *gk.Cx) gk.Element {
	bs := gk.BaseBoxShadow(cx, 4, 6, 12, 2, gk.HSLA(0.6, 0.8, 0.3, 0.6))
	return gk.Div().P(gk.Px(24)).Child(gk.Div().W(gk.Px(160)).H(gk.Px(80)).Bg(theme.Background).Shadow([]gk.BoxShadow{bs}))
}

// valuesResizablePage: the kit's resizable panels, a horizontal group of two panels and the
// kit's handle between them.
func valuesResizablePage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(200)).Child(gk.KitHResizable("rz").
		Child(gk.ResizablePanel().Size(200).Child(gk.Div(s.SizeFull).Bg(theme.Muted).Child(gk.Label("left")))).
		Child(gk.ResizablePanel().Child(gk.Div(s.SizeFull).Child(gk.Label("right")))))
}

// valuesScrollbarDivPage: a div that overflows with the kit's own scrollbar
// (ScrollableElement::overflow_y_scrollbar).
func valuesScrollbarDivPage(a *App, cx *gk.Cx) gk.Element {
	list := gk.Div().W(gk.Px(300)).H(gk.Px(160))
	for i := range 20 {
		list = list.Child(gk.Div().H(gk.Px(24)).Child(gk.Text(fmt.Sprintf("item %d", i))))
	}
	return list.OverflowYScrollbar().ID("sd")
}

// valuesToolbarPage: a toolbar whose children must be Sizable: buttons, one at a time and as a list.
func valuesToolbarPage(a *App, cx *gk.Cx) gk.Element {
	return gk.Toolbar("tb").Child(gk.Button("b1").Label("Bold")).Child(gk.Button("b2").Label("Italic")).
		Children([]gk.SizableNode{gk.Button("b3").Label("Code"), gk.Button("b4").Label("Wrap")})
}

// --- the components family, docs/PROTOCOLS.md 11.17 to 11.24, the twins of
// ref/src/bin/gallery/values_pages.rs: a tooltip the kit's Tooltip value describes and labels in
// Markdown (Text::TextView), a resizable group whose divider the app draws, calendars with
// disabled dates and a table's and a tree's row menus.

func comp6Tooltip(a *App, cx *gk.Cx) gk.Element {
	tip := gk.NewKitTooltip("Save the file").KeyBinding(gk.Some(gk.Kbd(gk.Keystroke{Modifiers: gk.Modifiers{Control: true}, Key: "s"})))
	return gk.Div().ID("c6tip").W(gk.Px(160)).H(gk.Px(40)).Bg(theme.Muted).Tooltip(tip).Child(gk.Label("Hover me"))
}

func comp6TooltipElement(a *App, cx *gk.Cx) gk.Element {
	tip := gk.NewKitTooltipElement(gk.HFlex(s.Gap1).Child(gk.Label("Zoom")).Child(gk.Label("in"))).Action(a.values.zoomIn, gk.Option[string]{})
	return gk.Div().ID("c6tipe").W(gk.Px(160)).H(gk.Px(40)).Bg(theme.Muted).Tooltip(tip).Child(gk.Label("Hover me"))
}

// comp6TextView: a checkbox's label and an alert's message in Markdown.
// Go's TextInto takes the TextViewEl itself (the
// kit's Into<Text> for a TextView, Text::TextView).
func comp6TextView(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).W(gk.Px(400)).
		Child(gk.Checkbox("c6cb").Checked(true).Label(gk.TextViewMarkdown("c6md1", "A **bold** label"))).
		Child(gk.Alert("c6al", gk.TextViewMarkdown("c6md2", "An alert in *Markdown*")))
}

// comp6HandleRow is the divider of the first group: a 3-pixel bar, the primary color while the
// pointer owns it (an item function, gk.Items).
func (a *App) comp6HandleRow(cx *gk.Cx, ctx gk.ResizeHandleContext) gk.Element {
	color := theme.Border
	if ctx.IsActive {
		color = theme.Primary
	}
	return gk.Div(s.HFull).W(gk.Px(3)).Bg(color)
}

func comp6Resize(a *App, cx *gk.Cx) gk.Element {
	group := func(id string, r gk.ResizeHandleRenderer) gk.Element {
		return gk.Div().W(gk.Px(600)).H(gk.Px(100)).Child(gk.HResizable(id).WithHandleAppearance(r).
			Child(gk.ResizablePanel().Size(200).Child(gk.Label("left"))).
			Child(gk.ResizablePanel().Child(gk.Label("right"))))
	}
	// A handle of its own (ResizeHandle::with_appearance) at the left edge of a band.
	alone := gk.Div(s.Relative).W(gk.Px(600)).H(gk.Px(40)).Bg(theme.Muted).
		Child(gk.ResizeHandle("c6h", gk.AxisHorizontal).WithAppearance(a.values.handles))
	return gk.VFlex(s.Gap4).Child(group("c6rz", a.values.handles)).
		Child(group("c6rk", gk.ResizeHandleRendererKitResizeHandleAppearance)).Child(alone)
}

func comp6CalendarMatcher(a *App, cx *gk.Cx) gk.Element {
	return gk.Div(s.Flex, s.Gap4).Child(gk.Div().W(gk.Px(320)).Child(gk.Calendar(a.values.calRange))).
		Child(gk.Div().W(gk.Px(320)).Child(gk.Calendar(a.values.calDays)))
}

// comp6Cell is a cell of the menu table; the first column's cell carries its row's menu.
func (a *App) comp6Cell(cx *gk.Cx, at gk.CellAt) gk.Element {
	el := gk.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
	if at.Col != 0 {
		return el
	}
	return gk.RowMenu(el, gk.NewPopupMenu().Menu(fmt.Sprintf("Open row %d", at.Row), a.values.zoomIn).
		Menu(fmt.Sprintf("Delete row %d", at.Row), a.values.zoomOut))
}

func comp6TableMenu(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(300)).H(gk.Px(200)).Child(gk.DataTable(a.values.table).ScrollbarVisible(false, false))
}

func (a *App) comp6TreeRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	label := ""
	if e, ok := gk.TreeEntryAt(cx, a.values.tree, at.Row); ok {
		label = e.Label
	}
	item := gk.ListItem(gk.Key("c6tree", at.Row)).Child(gk.Label(label))
	return gk.RowMenuItem(item, gk.NewPopupMenu().Menu(fmt.Sprintf("Rename %s", label), a.values.zoomIn))
}

func comp6TreeMenu(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(300)).H(gk.Px(160)).Child(gk.Tree(a.values.tree, a.values.treeRows).ContextMenu(a.values.treeRows))
}
