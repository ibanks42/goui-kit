// The views family's gallery pages, the Go twin of ref/src/bin/gallery/views_pages.rs (and of
// examples/odin/gallery/views_pages.odin), call for call: a sidebar whose items are groups of
// the kit's menus (SidebarGroup<SidebarMenu> and Sidebar<SidebarGroup<SidebarMenu>>), one group
// collapsed; a context menu whose submenu item has an icon (PopupMenu::submenu_with_icon); a
// context menu of items made as values (PopupMenuItem).
package main

import (
	"gpuikit.dev/gk"
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
	later := []gk.SidebarMenu{
		gk.NewSidebarMenu().Child(gk.NewSidebarMenuItem("Archive")),
		gk.NewSidebarMenu().Child(gk.NewSidebarMenuItem("Trash")),
	}
	groups := []gk.SidebarGroup{gk.NewSidebarGroup("Later").Children(later).Collapsed(true)}
	return gk.Div().W(gk.Px(240)).H(gk.Px(300)).Child(
		gk.GroupedSidebar("gsb").Side(gk.SideLeft).Collapsible(false).Collapsed(false).Header(gk.Label("Mail")).
			Child(gk.NewSidebarGroup("Inbox").
				Child(gk.NewSidebarMenu().Child(gk.NewSidebarMenuItem("Unread").Active(true)).Child(gk.NewSidebarMenuItem("Flagged")))).
			Children(groups))
}

func viewsSubmenuIcon(a *App, cx *gk.Cx) gk.Element {
	v := &a.views
	zoom := gk.NewPopupMenu().Menu("Zoom in", v.zoomIn).Menu("Zoom out", v.zoomOut)
	plain := gk.NewPopupMenu().Menu("Zoom in", v.zoomIn).Menu("Zoom out", v.zoomOut)
	return gk.Div(s.Border1).W(gk.Px(300)).H(gk.Px(120)).
		Child(gk.Label("Right-click here")).
		ContextMenu(gk.NewPopupMenu().
			SubmenuWithIcon(gk.Some(gk.Icon(gk.IconFolder)), "Zoom", zoom).
			SubmenuWithIcon(gk.Option[gk.IconEl]{}, "Plain", plain))
}

func viewsMenuItems(a *App, cx *gk.Cx) gk.Element {
	v := &a.views
	items := gk.NewPopupMenu().
		Item(gk.NewPopupMenuItem("Copy").Icon(gk.IconCopy).Action(v.zoomIn)).
		Item(gk.NewPopupMenuItemSeparator()).
		Item(gk.NewPopupMenuItemLabel("Section")).
		Item(gk.NewPopupMenuItem("Checked").Checked(true).Action(v.zoomOut)).
		Item(gk.NewPopupMenuItem("Off").Disabled(true)).
		Item(gk.NewPopupMenuItem("Count").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.count++ })).
		Item(gk.NewPopupMenuItemElement(gk.Label("an element"))).
		Item(gk.NewPopupMenuItemLink("Docs", "https://example.com"))
	return gk.Div(s.Border1).W(gk.Px(300)).H(gk.Px(120)).Child(gk.Label("Right-click here")).ContextMenu(items)
}
