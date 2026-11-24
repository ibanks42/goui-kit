// The overlays family's gallery pages, the Go twin of ref/src/bin/gallery/overlay_pages.rs,
// call for call (docs/PROTOCOLS.md section 11.6): a
// dialog, an alert and two sheets opened by a handler with a view-built function (gk.NewBuilt),
// a dropdown button and a context menu with a submenu whose menus are described values. The
// page's script opens each (pages.txt). Then docks: panels, a split at a node read back from its
// layout, a saved layout restored through the panel registry, the skin's setters and a dock drawn
// by the app's renderer, and keystrokes as the kit's Kbd.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

// menuActions are the menus' actions, named once (an app action: no key binding, so no
// shortcut is shown).
type menuActions struct {
	copy, paste, delete, pinned, zoomIn, zoomOut, kbd gk.Action
}

type overlayPages struct {
	acts    menuActions
	dock    gk.DockArea // dock-panels
	hooks   gk.DockArea // dock-hooks
	split   gk.DockArea // dock-split
	restore gk.DockArea // dock-restore
	skin    gk.DockArea // dock-skin
	render  gk.DockArea // dock-renderer
	// the renderer's views
	chromes       [4]chromeState
	chromeViews   [4]gk.View
	frameV, tabsV gk.View
}

// placementNames are the dock placements by their variant names (the page's label).
var placementNames = [...]string{"Center", "Left", "Bottom", "Right"}

type chromeState struct {
	placement gk.DockPlacement
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
		kbd:     gk.NewAction(cx, "gallery_menu::MenuKbd"), // bound to ctrl-s; no menu shows it
	}
	overlayDockInit(a, cx)
	dockR3Init(a, cx)
	dockRendererInit(a, cx)
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-s", Action: a.overlay.acts.kbd}) // the page menu-kbd shows it
}

func overlayOpenButton(h func(cx *gk.Cx, e gk.ClickEvent)) gk.Element {
	return gk.Button("open").Label("Open").OnClick(h)
}

func (a *App) menu() gk.PopupMenu {
	acts := &a.overlay.acts
	return gk.NewPopupMenu().
		Menu("Copy", acts.copy).
		Menu("Paste", acts.paste).
		Separator().
		Label("Danger").
		MenuWithCheck("Pinned", true, acts.pinned).
		MenuWithDisabled("Delete", acts.delete, true)
}

// --- the dialog: a function that describes it, run whenever the kit builds it

func deleteDialog(cx *gk.Cx) gk.DialogEl {
	return gk.Dialog().
		Title(gk.Label("Delete report.txt?")).
		Entrance(gk.DialogEntranceNone).
		Child(gk.Label("The file will be gone for good.")).
		Footer(gk.Label("Press Escape to keep it."))
}

func overlayDialogPage(a *App, cx *gk.Cx) gk.Element {
	return overlayOpenButton(func(cx *gk.Cx, _ gk.ClickEvent) { gk.OpenDialog(cx, gk.NewBuilt(cx, deleteDialog)) })
}

func unsavedAlert(cx *gk.Cx) gk.AlertDialogEl {
	return gk.AlertDialog().
		Title(gk.Label("Unsaved changes")).
		Description(gk.Label("Leave without saving?")).
		Entrance(gk.DialogEntranceNone).
		ShowCancel(true)
}

func overlayAlertPage(a *App, cx *gk.Cx) gk.Element {
	return overlayOpenButton(func(cx *gk.Cx, _ gk.ClickEvent) { gk.OpenAlertDialog(cx, gk.NewBuilt(cx, unsavedAlert)) })
}

func settingsSheet(cx *gk.Cx) gk.SheetEl {
	return gk.Sheet().Title(gk.Label("Settings")).Child(gk.Label("The sheet's body"))
}

func overlaySheetPage(a *App, cx *gk.Cx) gk.Element {
	return overlayOpenButton(func(cx *gk.Cx, _ gk.ClickEvent) { gk.OpenSheet(cx, gk.NewBuilt(cx, settingsSheet)) })
}

func filesSheet(cx *gk.Cx) gk.SheetEl {
	return gk.Sheet().Title(gk.Label("Files")).Resizable(false).Child(gk.Label("On the left"))
}

func overlaySheetLeftPage(a *App, cx *gk.Cx) gk.Element {
	return overlayOpenButton(func(cx *gk.Cx, _ gk.ClickEvent) {
		gk.OpenSheetAt(cx, gk.PlacementLeft, gk.NewBuilt(cx, filesSheet))
	})
}

// --- menus: described values, built by the library each time the menu opens

func overlayDropdownPage(a *App, cx *gk.Cx) gk.Element {
	return gk.DropdownMenu(gk.Button("open").Label("Open"), a.menu())
}

func overlayContextPage(a *App, cx *gk.Cx) gk.Element {
	acts := &a.overlay.acts
	zoom := gk.NewPopupMenu().Menu("Zoom in", acts.zoomIn).Menu("Zoom out", acts.zoomOut)
	return gk.Div(s.Border1).W(gk.Px(300)).H(gk.Px(120)).
		Child(gk.Label("Right-click here")).
		ContextMenu(a.menu().Separator().Submenu("Zoom", zoom))
}

// --- a docked layout of three Go panels (docs/PROTOCOLS.md 11.7): two tabs in the center, one
// in the left dock, in the kit's styled dock area

// dockTextPanel is a panel's view: a label of its body.
func dockTextPanel(body string) gk.View {
	return gk.NewView(nil, func(cx *gk.Cx) gk.Element { return gk.Label(body) })
}

func overlayDockInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.dock = gk.NewDockAreaSkinned(cx, "main")
	files := gk.NewPanel(cx, dockTextPanel("src/main.demo"), gk.PanelData{Name: "files", Title: "Files"})
	editor := gk.NewPanel(cx, dockTextPanel("package main"), gk.PanelData{Name: "editor", Title: "Editor"})
	outline := gk.NewPanel(cx, dockTextPanel("main :: proc()"), gk.PanelData{Name: "outline", Title: "Outline"})
	p.dock.SetCenter(cx, gk.NewDockLayoutTabs().Panel(files).Panel(editor))
	p.dock.SetDock(cx, gk.DockPlacementLeft, gk.NewDockLayoutTabs().Panel(outline))
}

func overlayDockPage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(300)).Child(gk.DockView(a.overlay.dock))
}

// --- docs/PROTOCOLS.md 11.8 ------------------------------------------------------------

// A panel's title suffix and toolbar buttons, from views the app pushed.
func dockHooksInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.hooks = gk.NewDockAreaSkinned(cx, "hooks")
	data := gk.PanelData{Name: "hooked", Title: "Hooked"}
	data.Suffix = dockTextPanel("v0.9.1")
	data.Toolbar = gk.NewView(nil, func(cx *gk.Cx) gk.Element {
		return gk.HFlex().Child(gk.Button("run").Label("Run")).Child(gk.Button("stop").Label("Stop"))
	})
	data.Menu = gk.NewBuilt(cx, func(cx *gk.Cx) gk.PopupMenu {
		return gk.NewPopupMenu().Separator().Menu("Copy", a.overlay.acts.copy)
	})
	hooked := gk.NewPanel(cx, dockTextPanel("Panel body"), data)
	// PanelView: the kit's PanelHandle over the panel, through which the skin draws its title,
	// suffix, toolbar and menu (Panel adds the bare panel, drawn with its name)
	p.hooks.SetCenter(cx, gk.NewDockLayoutTabs().PanelView(hooked))
}

func dockHooksPage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(300)).Child(gk.DockView(a.overlay.hooks))
}

// A dropdown that opens from its top right corner.
func menuAnchorPage(a *App, cx *gk.Cx) gk.Element {
	return gk.DropdownMenuAnchored(gk.Button("open").Label("Open"), gk.AnchorTopRight, a.menu())
}

// A popover the app holds open (Popover::open from the app's state), its trigger a kit button.
func popoverOpenPage(a *App, cx *gk.Cx) gk.Element {
	return gk.Popover("pop").Trigger(gk.Button("trigger").Label("Open")).Open(true).Child(gk.Label("inside the popover"))
}

func dockPage(d gk.DockArea) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(300)).Child(gk.DockView(d))
}

// dockR3Init makes the skin dock, then the split, hooks and restore
// docks.
func dockR3Init(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.skin = gk.NewDockAreaSkinned(cx, "skin")
	one := gk.NewPanel(cx, dockTextPanel("src/main.demo"), gk.PanelData{Name: "files", Title: "Files"})
	p.skin.SetCenter(cx, gk.NewDockLayoutTabs().Panel(one))
	gk.SetDockSkinPanelStyle(cx, p.skin, gk.PanelStyleTabBar) // a tab bar even for one panel
	gk.SetDockSkinCloseButtonVisible(cx, p.skin, false)
	dockSplitInit(a, cx)
	dockHooksInit(a, cx)
	dockRestoreInit(a, cx)
}

// The tab group's node id read back from the layout, a split at it and the split's sizes.
func dockSplitInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.split = gk.NewDockAreaSkinned(cx, "split")
	files := gk.NewPanel(cx, dockTextPanel("src/main.demo"), gk.PanelData{Name: "files", Title: "Files"})
	editor := gk.NewPanel(cx, dockTextPanel("package main"), gk.PanelData{Name: "editor", Title: "Editor"})
	outline := gk.NewPanel(cx, dockTextPanel("main :: proc()"), gk.PanelData{Name: "outline", Title: "Outline"})
	p.split.SetCenter(cx, gk.NewDockLayoutTabs().Panel(files).Panel(editor).Panel(outline))
	tree, _ := p.split.Layout(cx, gk.DockPlacementCenter).Get() // a read: the layout above is applied first
	group, _ := tree.FindPanelNode(cx, gk.PanelID(cx, editor)).Get()
	p.split.SplitAt(cx, group, gk.PanelID(cx, outline), gk.PlacementRight)
	p.split.SplitAt(cx, group, gk.PanelID(cx, editor), gk.PlacementBottom)
}

// A saved layout loaded: the registry asks for each panel, the app fills it.
func restorePanel(cx *gk.Cx, b gk.PanelBuild) {
	switch b.Name {
	case "files":
		gk.PanelFill(cx, b.Panel, dockTextPanel("src/main.demo"), gk.PanelData{Name: "files", Title: "Files"})
	case "editor":
		gk.PanelFill(cx, b.Panel, dockTextPanel("package main"), gk.PanelData{Name: "editor", Title: "Editor"})
	case "outline":
		gk.PanelFill(cx, b.Panel, dockTextPanel("main :: proc()"), gk.PanelData{Name: "outline", Title: "Outline"})
	}
}

func leafPanel(name string) gk.PanelState {
	return gk.PanelState{PanelName: name, Info: gk.PanelInfoPanel{}}
}

func dockRestoreInit(a *App, cx *gk.Cx) {
	for _, name := range []string{"files", "editor", "outline"} {
		gk.RegisterPanel(cx, name, restorePanel)
	}
	p := &a.overlay
	p.restore = gk.NewDockAreaSkinned(cx, "restored")
	left := []gk.PanelState{leafPanel("files")}
	right := []gk.PanelState{leafPanel("editor"), leafPanel("outline")}
	groups := []gk.PanelState{
		{PanelName: "TabPanel", Children: left, Info: gk.PanelInfoTabs{ActiveIndex: 0}},
		{PanelName: "TabPanel", Children: right, Info: gk.PanelInfoTabs{ActiveIndex: 1}},
	}
	state := gk.DockAreaState{
		Center: gk.PanelState{PanelName: "StackPanel", Children: groups, Info: gk.PanelInfoStack{Sizes: []float32{200, 400}, Axis: 0}},
	}
	_ = p.restore.Load(cx, state)
}

// The keystrokes bound to an action, as the kit's Kbd: bound, unbound (nothing), with no context.
func menuKbdPage(a *App, cx *gk.Cx) gk.Element {
	acts := &a.overlay.acts
	return gk.VFlex().
		Child(gk.KbdForAction(acts.kbd)).
		Child(gk.KbdForAction(acts.paste)).
		Child(gk.KbdGlobalForAction(acts.kbd))
}

// --- docs/PROTOCOLS.md 11.9, 11.12 ----------------------------------------------------------

// A dock drawn by the app's renderer: a padded frame, each dock's chrome a title over its content
// (the slot), the tab bar a label. Each hook asks once per key and context; the answer is a view.
func (a *App) renderAsked(cx *gk.Cx, q gk.DockRender) {
	p := &a.overlay
	switch q.Hook {
	case gk.HookFrame:
		gk.DockRenderAnswer(cx, q.Renderer, q.Hook, q.Key, p.frameV)
	case gk.HookDock:
		i := int(q.Placement)
		p.chromes[i] = chromeState{q.Placement, q.Flags&1 != 0}
		if p.chromeViews[i] == (gk.View{}) {
			c := &p.chromes[i]
			p.chromeViews[i] = gk.NewView(nil, func(cx *gk.Cx) gk.Element {
				return gk.VFlex(s.SizeFull).Child(gk.Label(fmt.Sprintf("%s open=%v", placementNames[c.placement], c.open))).Child(gk.DockSlot())
			})
		}
		gk.DockRenderAnswer(cx, q.Renderer, q.Hook, q.Key, p.chromeViews[i])
	case gk.HookTabBar:
		gk.DockRenderAnswer(cx, q.Renderer, q.Hook, q.Key, p.tabsV)
	}
}

func dockRendererInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.frameV = gk.NewView(nil, func(cx *gk.Cx) gk.Element { return gk.HFlex(s.SizeFull, s.P1) })
	p.tabsV = gk.NewView(nil, func(cx *gk.Cx) gk.Element { return gk.HFlex(s.WFull).Child(gk.Label("tabs of the app")) })
	r := gk.NewDockRenderer(cx, gk.DockHooks(gk.HookFrame, gk.HookDock, gk.HookTabBar), a.renderAsked)
	p.render = gk.NewDockArea(cx, "rendered", gk.Option[int]{}).WithRenderer(r).Build()
	files := gk.NewPanel(cx, dockTextPanel("src/main.demo"), gk.PanelData{Name: "files", Title: "Files"})
	outline := gk.NewPanel(cx, dockTextPanel("main :: proc()"), gk.PanelData{Name: "outline", Title: "Outline"})
	p.render.SetCenter(cx, gk.NewDockLayoutTabs().Panel(files))
	p.render.SetDock(cx, gk.DockPlacementLeft, gk.NewDockLayoutTabs().Panel(outline))
}
