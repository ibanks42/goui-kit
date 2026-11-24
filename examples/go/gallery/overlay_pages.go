// Overlay, menu, and dock gallery pages: dialogs, alerts, sheets, popovers, dropdown and
// context menus, key-binding Kbd displays, dock splits, layout save/restore, and custom renderers.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	gkdock "gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
)

type menuActions struct {
	copy, paste, delete, pinned, zoomIn, zoomOut, kbd gk.Action
}

type overlayPages struct {
	acts          menuActions
	dock          gkdock.Area
	hooks         gkdock.Area
	split         gkdock.Area
	restore       gkdock.Area
	skin          gkdock.Area
	render        gkdock.Area
	chromes       [4]chromeState
	chromeViews   [4]gk.View
	frameV, tabsV gk.View
}

var placementNames = [...]string{"Center", "Left", "Bottom", "Right"}

type chromeState struct {
	placement gkdock.Placement
	open      bool
}

var overlayPageTable = []Page{
	{"overlay-dialog", overlayDialogPage},
	{"overlay-alert", overlayAlertPage},
	{"overlay-sheet", overlaySheetPage},
	{"overlay-sheet-left", overlaySheetLeftPage},
	{"overlay-dropdown", overlayDropdownPage},
	{"overlay-context", overlayContextPage},
	{"dock-panels", overlayDockPage},
	{"dock-split", func(a *App, cx *gk.Cx) gk.Element { return dockPage(a.overlay.split) }},
	{"dock-hooks", dockHooksPage},
	{"dock-restore", func(a *App, cx *gk.Cx) gk.Element { return dockPage(a.overlay.restore) }},
	{"menu-kbd", menuKbdPage},
	{"dock-skin", func(a *App, cx *gk.Cx) gk.Element { return dockPage(a.overlay.skin) }},
	{"dock-renderer", func(a *App, cx *gk.Cx) gk.Element { return dockPage(a.overlay.render) }},
	{"menu-dropdown-anchor", menuAnchorPage},
	{"overlay-popover-open", popoverOpenPage},
}

func overlayPagesInit(a *App, cx *gk.Cx) {
	a.overlay.acts = menuActions{
		copy:    gk.NewAction(cx, "gallery_menu::MenuCopy"),
		paste:   gk.NewAction(cx, "gallery_menu::MenuPaste"),
		delete:  gk.NewAction(cx, "gallery_menu::MenuDelete"),
		pinned:  gk.NewAction(cx, "gallery_menu::MenuPinned"),
		zoomIn:  gk.NewAction(cx, "gallery_menu::MenuZoomIn"),
		zoomOut: gk.NewAction(cx, "gallery_menu::MenuZoomOut"),
		kbd:     gk.NewAction(cx, "gallery_menu::MenuKbd"),
	}
	overlayDockInit(a, cx)
	dockR3Init(a, cx)
	dockRendererInit(a, cx)
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-s", Action: a.overlay.acts.kbd})
}

func overlayOpenButton(h func(cx *gk.Cx, e gk.ClickEvent)) gk.Element {
	return kit.Button("open").Label("Open").OnClick(h)
}

func (a *App) menu() kit.PopupMenu {
	acts := &a.overlay.acts
	return kit.NewPopupMenu().
		Menu("Copy", acts.copy).
		Menu("Paste", acts.paste).
		Separator().
		Label("Danger").
		MenuWithCheck("Pinned", true, acts.pinned).
		MenuWithDisabled("Delete", acts.delete, true)
}

func deleteDialog(cx *gk.Cx) overlay.DialogEl {
	return overlay.Dialog().
		Title(kit.Label("Delete report.txt?")).
		Entrance(overlay.DialogEntranceNone).
		Child(kit.Label("The file will be gone for good.")).
		Footer(kit.Label("Press Escape to keep it."))
}

func overlayDialogPage(a *App, cx *gk.Cx) gk.Element {
	return overlayOpenButton(func(cx *gk.Cx, _ gk.ClickEvent) { overlay.OpenDialog(cx, gk.NewBuilt(cx, deleteDialog)) })
}

func unsavedAlert(cx *gk.Cx) overlay.AlertDialogEl {
	return overlay.AlertDialog().
		Title(kit.Label("Unsaved changes")).
		Description(kit.Label("Leave without saving?")).
		Entrance(overlay.DialogEntranceNone).
		ShowCancel(true)
}

func overlayAlertPage(a *App, cx *gk.Cx) gk.Element {
	return overlayOpenButton(func(cx *gk.Cx, _ gk.ClickEvent) { overlay.OpenAlertDialog(cx, gk.NewBuilt(cx, unsavedAlert)) })
}

func settingsSheet(cx *gk.Cx) overlay.SheetEl {
	return overlay.Sheet().Title(kit.Label("Settings")).Child(kit.Label("The sheet's body"))
}

func overlaySheetPage(a *App, cx *gk.Cx) gk.Element {
	return overlayOpenButton(func(cx *gk.Cx, _ gk.ClickEvent) { overlay.OpenSheet(cx, gk.NewBuilt(cx, settingsSheet)) })
}

func filesSheet(cx *gk.Cx) overlay.SheetEl {
	return overlay.Sheet().Title(kit.Label("Files")).Resizable(false).Child(kit.Label("On the left"))
}

func overlaySheetLeftPage(a *App, cx *gk.Cx) gk.Element {
	return overlayOpenButton(func(cx *gk.Cx, _ gk.ClickEvent) {
		overlay.OpenSheetAt(cx, gk.PlacementLeft, gk.NewBuilt(cx, filesSheet))
	})
}

func overlayDropdownPage(a *App, cx *gk.Cx) gk.Element {
	return kit.DropdownMenu(kit.Button("open").Label("Open"), a.menu())
}

func overlayContextPage(a *App, cx *gk.Cx) gk.Element {
	acts := &a.overlay.acts
	zoom := kit.NewPopupMenu().Menu("Zoom in", acts.zoomIn).Menu("Zoom out", acts.zoomOut)
	return kit.DivContextMenu(gk.Div(s.Border1).W(gk.Px(300)).H(gk.Px(120)).
		Child(kit.Label("Right-click here")),
		a.menu().Separator().Submenu("Zoom", zoom))
}

func dockTextPanel(body string) gk.View {
	return gk.NewView(nil, func(cx *gk.Cx) gk.Element { return kit.Label(body) })
}

func overlayDockInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.dock = gkdock.NewAreaSkinned(cx, "main")
	files := gkdock.NewPanel(cx, dockTextPanel("src/main.demo"), gkdock.PanelData{Name: "files", Title: "Files"})
	editor := gkdock.NewPanel(cx, dockTextPanel("package main"), gkdock.PanelData{Name: "editor", Title: "Editor"})
	outline := gkdock.NewPanel(cx, dockTextPanel("main :: proc()"), gkdock.PanelData{Name: "outline", Title: "Outline"})
	p.dock.SetCenter(cx, gkdock.NewLayoutTabs().Panel(files).Panel(editor))
	p.dock.SetDock(cx, gkdock.PlacementLeft, gkdock.NewLayoutTabs().Panel(outline))
}

func overlayDockPage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(300)).Child(gkdock.View(a.overlay.dock))
}

func dockHooksInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.hooks = gkdock.NewAreaSkinned(cx, "hooks")
	data := gkdock.PanelData{Name: "hooked", Title: "Hooked"}
	data.Suffix = dockTextPanel("v0.9.1")
	data.Toolbar = gk.NewView(nil, func(cx *gk.Cx) gk.Element {
		return gk.HFlex().Child(kit.Button("run").Label("Run")).Child(kit.Button("stop").Label("Stop"))
	})
	data.Menu = gk.NewBuilt(cx, func(cx *gk.Cx) kit.PopupMenu {
		return kit.NewPopupMenu().Separator().Menu("Copy", a.overlay.acts.copy)
	})
	hooked := gkdock.NewPanel(cx, dockTextPanel("Panel body"), data)
	p.hooks.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(hooked))
}

func dockHooksPage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(300)).Child(gkdock.View(a.overlay.hooks))
}

func menuAnchorPage(a *App, cx *gk.Cx) gk.Element {
	return kit.DropdownMenuAnchored(kit.Button("open").Label("Open"), gk.AnchorTopRight, a.menu())
}

func popoverOpenPage(a *App, cx *gk.Cx) gk.Element {
	return overlay.Popover("pop").Trigger(kit.Button("trigger").Label("Open")).Open(true).Child(kit.Label("inside the popover"))
}

func dockPage(d gkdock.Area) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(300)).Child(gkdock.View(d))
}

func dockR3Init(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.skin = gkdock.NewAreaSkinned(cx, "skin")
	one := gkdock.NewPanel(cx, dockTextPanel("src/main.demo"), gkdock.PanelData{Name: "files", Title: "Files"})
	p.skin.SetCenter(cx, gkdock.NewLayoutTabs().Panel(one))
	gkdock.SetDockSkinPanelStyle(cx, p.skin, gkdock.PanelStyleTabBar)
	gkdock.SetDockSkinCloseButtonVisible(cx, p.skin, false)
	dockSplitInit(a, cx)
	dockHooksInit(a, cx)
	dockRestoreInit(a, cx)
}

func dockSplitInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.split = gkdock.NewAreaSkinned(cx, "split")
	files := gkdock.NewPanel(cx, dockTextPanel("src/main.demo"), gkdock.PanelData{Name: "files", Title: "Files"})
	editor := gkdock.NewPanel(cx, dockTextPanel("package main"), gkdock.PanelData{Name: "editor", Title: "Editor"})
	outline := gkdock.NewPanel(cx, dockTextPanel("main :: proc()"), gkdock.PanelData{Name: "outline", Title: "Outline"})
	p.split.SetCenter(cx, gkdock.NewLayoutTabs().Panel(files).Panel(editor).Panel(outline))
	tree, _ := p.split.Layout(cx, gkdock.PlacementCenter).Get()
	group, _ := tree.FindPanelNode(cx, gkdock.PanelID(cx, editor)).Get()
	p.split.SplitAt(cx, group, gkdock.PanelID(cx, outline), gk.PlacementRight)
	p.split.SplitAt(cx, group, gkdock.PanelID(cx, editor), gk.PlacementBottom)
}

func restorePanel(cx *gk.Cx, b gkdock.PanelBuild) {
	switch b.Name {
	case "files":
		gkdock.PanelFill(cx, b.Panel, dockTextPanel("src/main.demo"), gkdock.PanelData{Name: "files", Title: "Files"})
	case "editor":
		gkdock.PanelFill(cx, b.Panel, dockTextPanel("package main"), gkdock.PanelData{Name: "editor", Title: "Editor"})
	case "outline":
		gkdock.PanelFill(cx, b.Panel, dockTextPanel("main :: proc()"), gkdock.PanelData{Name: "outline", Title: "Outline"})
	}
}

func leafPanel(name string) gkdock.PanelState {
	return gkdock.PanelState{PanelName: name, Info: gkdock.PanelInfoPanel{}}
}

func dockRestoreInit(a *App, cx *gk.Cx) {
	for _, name := range []string{"files", "editor", "outline"} {
		gkdock.RegisterPanel(cx, name, restorePanel)
	}
	p := &a.overlay
	p.restore = gkdock.NewAreaSkinned(cx, "restored")
	left := []gkdock.PanelState{leafPanel("files")}
	right := []gkdock.PanelState{leafPanel("editor"), leafPanel("outline")}
	groups := []gkdock.PanelState{
		{PanelName: "TabPanel", Children: left, Info: gkdock.PanelInfoTabs{ActiveIndex: 0}},
		{PanelName: "TabPanel", Children: right, Info: gkdock.PanelInfoTabs{ActiveIndex: 1}},
	}
	state := gkdock.AreaState{
		Center: gkdock.PanelState{PanelName: "StackPanel", Children: groups, Info: gkdock.PanelInfoStack{Sizes: []float32{200, 400}, Axis: 0}},
	}
	_ = p.restore.Load(cx, state)
}

func menuKbdPage(a *App, cx *gk.Cx) gk.Element {
	acts := &a.overlay.acts
	return gk.VFlex().
		Child(kit.KbdForAction(acts.kbd)).
		Child(kit.KbdForAction(acts.paste)).
		Child(kit.KbdGlobalForAction(acts.kbd))
}

func (a *App) renderAsked(cx *gk.Cx, q gkdock.Render) {
	p := &a.overlay
	switch q.Hook {
	case gkdock.HookFrame:
		gkdock.RenderAnswer(cx, q.Renderer, q.Hook, q.Key, p.frameV)
	case gkdock.HookDock:
		i := int(q.Placement)
		p.chromes[i] = chromeState{q.Placement, q.Flags&1 != 0}
		if p.chromeViews[i] == (gk.View{}) {
			c := &p.chromes[i]
			p.chromeViews[i] = gk.NewView(nil, func(cx *gk.Cx) gk.Element {
				return gk.VFlex(s.SizeFull).Child(kit.Label(fmt.Sprintf("%s open=%v", placementNames[c.placement], c.open))).Child(gkdock.Slot())
			})
		}
		gkdock.RenderAnswer(cx, q.Renderer, q.Hook, q.Key, p.chromeViews[i])
	case gkdock.HookTabBar:
		gkdock.RenderAnswer(cx, q.Renderer, q.Hook, q.Key, p.tabsV)
	}
}

func dockRendererInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.frameV = gk.NewView(nil, func(cx *gk.Cx) gk.Element { return gk.HFlex(s.SizeFull, s.P1) })
	p.tabsV = gk.NewView(nil, func(cx *gk.Cx) gk.Element { return gk.HFlex(s.WFull).Child(kit.Label("tabs of the app")) })
	r := gkdock.NewRenderer(cx, gkdock.Hooks(gkdock.HookFrame, gkdock.HookDock, gkdock.HookTabBar), a.renderAsked)
	p.render = gkdock.NewArea(cx, "rendered", gk.Option[int]{}).WithRenderer(r).Build()
	files := gkdock.NewPanel(cx, dockTextPanel("src/main.demo"), gkdock.PanelData{Name: "files", Title: "Files"})
	outline := gkdock.NewPanel(cx, dockTextPanel("main :: proc()"), gkdock.PanelData{Name: "outline", Title: "Outline"})
	p.render.SetCenter(cx, gkdock.NewLayoutTabs().Panel(files))
	p.render.SetDock(cx, gkdock.PlacementLeft, gkdock.NewLayoutTabs().Panel(outline))
}
