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
	gkdock "gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
)

// menuActions are the menus' actions, named once (an app action: no key binding, so no
// shortcut is shown).
type menuActions struct {
	copy, paste, delete, pinned, zoomIn, zoomOut, kbd gk.Action
}

type overlayPages struct {
	acts    menuActions
	dock    gkdock.Area // dock-panels
	hooks   gkdock.Area // dock-hooks
	split   gkdock.Area // dock-split
	restore gkdock.Area // dock-restore
	skin    gkdock.Area // dock-skin
	render  gkdock.Area // dock-renderer
	// the renderer's views
	chromes       [4]chromeState
	chromeViews   [4]gk.View
	frameV, tabsV gk.View
}

// placementNames are the dock placements by their variant names (the page's label).
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
		kbd:     gk.NewAction(cx, "gallery_menu::MenuKbd"), // bound to ctrl-s; no menu shows it
	}
	overlayDockInit(a, cx)
	dockR3Init(a, cx)
	dockRendererInit(a, cx)
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-s", Action: a.overlay.acts.kbd}) // the page menu-kbd shows it
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

// --- the dialog: a function that describes it, run whenever the kit builds it

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

// --- menus: described values, built by the library each time the menu opens

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

// --- a docked layout of three Go panels (docs/PROTOCOLS.md 11.7): two tabs in the center, one
// in the left dock, in the kit's styled dock area

// dockTextPanel is a panel's view: a label of its body.
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

// --- docs/PROTOCOLS.md 11.8 ------------------------------------------------------------

// A panel's title suffix and toolbar buttons, from views the app pushed.
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
	// PanelView: the kit's PanelHandle over the panel, through which the skin draws its title,
	// suffix, toolbar and menu (Panel adds the bare panel, drawn with its name)
	p.hooks.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(hooked))
}

func dockHooksPage(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(300)).Child(gkdock.View(a.overlay.hooks))
}

// A dropdown that opens from its top right corner.
func menuAnchorPage(a *App, cx *gk.Cx) gk.Element {
	return kit.DropdownMenuAnchored(kit.Button("open").Label("Open"), gk.AnchorTopRight, a.menu())
}

// A popover the app holds open (Popover::open from the app's state), its trigger a kit button.
func popoverOpenPage(a *App, cx *gk.Cx) gk.Element {
	return overlay.Popover("pop").Trigger(kit.Button("trigger").Label("Open")).Open(true).Child(kit.Label("inside the popover"))
}

func dockPage(d gkdock.Area) gk.Element {
	return gk.Div().W(gk.Px(600)).H(gk.Px(300)).Child(gkdock.View(d))
}

// dockR3Init makes the skin dock, then the split, hooks and restore
// docks.
func dockR3Init(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.skin = gkdock.NewAreaSkinned(cx, "skin")
	one := gkdock.NewPanel(cx, dockTextPanel("src/main.demo"), gkdock.PanelData{Name: "files", Title: "Files"})
	p.skin.SetCenter(cx, gkdock.NewLayoutTabs().Panel(one))
	gkdock.SetDockSkinPanelStyle(cx, p.skin, gkdock.PanelStyleTabBar) // a tab bar even for one panel
	gkdock.SetDockSkinCloseButtonVisible(cx, p.skin, false)
	dockSplitInit(a, cx)
	dockHooksInit(a, cx)
	dockRestoreInit(a, cx)
}

// The tab group's node id read back from the layout, a split at it and the split's sizes.
func dockSplitInit(a *App, cx *gk.Cx) {
	p := &a.overlay
	p.split = gkdock.NewAreaSkinned(cx, "split")
	files := gkdock.NewPanel(cx, dockTextPanel("src/main.demo"), gkdock.PanelData{Name: "files", Title: "Files"})
	editor := gkdock.NewPanel(cx, dockTextPanel("package main"), gkdock.PanelData{Name: "editor", Title: "Editor"})
	outline := gkdock.NewPanel(cx, dockTextPanel("main :: proc()"), gkdock.PanelData{Name: "outline", Title: "Outline"})
	p.split.SetCenter(cx, gkdock.NewLayoutTabs().Panel(files).Panel(editor).Panel(outline))
	tree, _ := p.split.Layout(cx, gkdock.PlacementCenter).Get() // a read: the layout above is applied first
	group, _ := tree.FindPanelNode(cx, gkdock.PanelID(cx, editor)).Get()
	p.split.SplitAt(cx, group, gkdock.PanelID(cx, outline), gk.PlacementRight)
	p.split.SplitAt(cx, group, gkdock.PanelID(cx, editor), gk.PlacementBottom)
}

// A saved layout loaded: the registry asks for each panel, the app fills it.
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

// The keystrokes bound to an action, as the kit's Kbd: bound, unbound (nothing), with no context.
func menuKbdPage(a *App, cx *gk.Cx) gk.Element {
	acts := &a.overlay.acts
	return gk.VFlex().
		Child(kit.KbdForAction(acts.kbd)).
		Child(kit.KbdForAction(acts.paste)).
		Child(kit.KbdGlobalForAction(acts.kbd))
}

// --- docs/PROTOCOLS.md 11.9, 11.12 ----------------------------------------------------------

// A dock drawn by the app's renderer: a padded frame, each dock's chrome a title over its content
// (the slot), the tab bar a label. Each hook asks once per key and context; the answer is a view.
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
