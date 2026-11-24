// The views family's gallery pages, the Go twin of ref/src/bin/gallery/views_pages.rs,
// call for call: a sidebar whose items are groups of
// the kit's menus (SidebarGroup<SidebarMenu> and Sidebar<SidebarGroup<SidebarMenu>>), one group
// collapsed; a context menu whose submenu item has an icon (PopupMenu::submenu_with_icon); a
// context menu of items made as values (PopupMenuItem).
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
)

type viewsPages struct {
	zoomIn, zoomOut gk.Action
}

var viewsPageTable = []Page{
	{"views-sidebar-groups", viewsSidebarGroups},
	{"views-submenu-icon", viewsSubmenuIcon},
	{"views-menu-items", viewsMenuItems},
}

func viewsPagesInit(a *App, cx *gk.Cx) {
	a.views.zoomIn = gk.NewAction(cx, "gallery_menu::MenuZoomIn") // the Rust twin's actions (overlay_pages.rs)
	a.views.zoomOut = gk.NewAction(cx, "gallery_menu::MenuZoomOut")
}

func viewsSidebarGroups(a *App, cx *gk.Cx) gk.Element {
	later := []kit.SidebarMenu{
		kit.NewSidebarMenu().Child(kit.NewSidebarMenuItem("Archive")),
		kit.NewSidebarMenu().Child(kit.NewSidebarMenuItem("Trash")),
	}
	groups := []kit.SidebarGroup{kit.NewSidebarGroup("Later").Children(later).Collapsed(true)}
	return gk.Div().W(gk.Px(240)).H(gk.Px(300)).Child(
		kit.GroupedSidebar("gsb").Side(gk.SideLeft).Collapsible(false).Collapsed(false).Header(kit.Label("Mail")).
			Child(kit.NewSidebarGroup("Inbox").
				Child(kit.NewSidebarMenu().Child(kit.NewSidebarMenuItem("Unread").Active(true)).Child(kit.NewSidebarMenuItem("Flagged")))).
			Children(groups))
}

func viewsSubmenuIcon(a *App, cx *gk.Cx) gk.Element {
	v := &a.views
	zoom := kit.NewPopupMenu().Menu("Zoom in", v.zoomIn).Menu("Zoom out", v.zoomOut)
	plain := kit.NewPopupMenu().Menu("Zoom in", v.zoomIn).Menu("Zoom out", v.zoomOut)
	return kit.DivContextMenu(gk.Div(s.Border1).W(gk.Px(300)).H(gk.Px(120)).
		Child(kit.Label("Right-click here")),
		kit.NewPopupMenu().
			SubmenuWithIcon(gk.Some(gk.Icon(gk.IconFolder)), "Zoom", zoom).
			SubmenuWithIcon(gk.Option[gk.IconEl]{}, "Plain", plain))
}

func viewsMenuItems(a *App, cx *gk.Cx) gk.Element {
	v := &a.views
	items := kit.NewPopupMenu().
		Item(kit.NewPopupMenuItem("Copy").Icon(gk.IconCopy).Action(v.zoomIn)).
		Item(kit.NewPopupMenuItemSeparator()).
		Item(kit.NewPopupMenuItemLabel("Section")).
		Item(kit.NewPopupMenuItem("Checked").Checked(true).Action(v.zoomOut)).
		Item(kit.NewPopupMenuItem("Off").Disabled(true)).
		Item(kit.NewPopupMenuItem("Count").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.count++ })).
		Item(kit.NewPopupMenuItemElement(kit.Label("an element"))).
		Item(kit.NewPopupMenuItemLink("Docs", "https://example.com"))
	return kit.DivContextMenu(gk.Div(s.Border1).W(gk.Px(300)).H(gk.Px(120)).Child(kit.Label("Right-click here")), items)
}
