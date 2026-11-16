// The views family's gallery pages, the Odin twin of ref/src/bin/gallery/views_pages.rs,
// call for call: a sidebar whose items are groups of the kit's menus (SidebarGroup<SidebarMenu> and
// Sidebar<SidebarGroup<SidebarMenu>>, `[instantiate] types` in api/overrides.toml), one group
// collapsed; a context menu whose submenu item has an icon (PopupMenu::submenu_with_icon, made by the
// library without the menu's own context, docs/PROTOCOLS.md 11.16); a context menu of items made as
// values (PopupMenuItem, an enum built like a builder: `[values] enum_nodes`).
package gallery

import gk "gpuikit:gk"

VIEWS_PAGES := [?]Page {
	{"views-sidebar-groups", views_sidebar_groups},
	{"views-submenu-icon", views_submenu_icon},
	{"views-menu-items", views_menu_items},
}

@(private = "file")
zoom_in, zoom_out: gk.Action

views_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	zoom_in = gk.action(cx, "gallery_menu::MenuZoomIn") // the Rust twin's actions (overlay_pages.rs)
	zoom_out = gk.action(cx, "gallery_menu::MenuZoomOut")
}

views_sidebar_groups :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	later := [?]gk.Sidebar_Menu {
		gk.sidebar_menu()->child(gk.sidebar_menu_item("Archive")),
		gk.sidebar_menu()->child(gk.sidebar_menu_item("Trash")),
	}
	groups := [?]gk.Sidebar_Group{gk.sidebar_group("Later")->children(later[:])->collapsed(true)}
	return gk.div()->w(gk.px(240))->h(gk.px(300))->child(
		gk.grouped_sidebar("gsb")->side(.Left)->collapsible(false)->collapsed(false)->header(gk.label("Mail"))->
		child(gk.sidebar_group("Inbox")->
			child(gk.sidebar_menu()->child(gk.sidebar_menu_item("Unread")->active(true))->child(gk.sidebar_menu_item("Flagged"))))->
		children(groups[:]),
	)
}

views_submenu_icon :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	zoom := gk.popup_menu()->menu("Zoom in", zoom_in)->menu("Zoom out", zoom_out)
	plain := gk.popup_menu()->menu("Zoom in", zoom_in)->menu("Zoom out", zoom_out)
	return gk.div(.Border_1)->w(gk.px(300))->h(gk.px(120))->
		child(gk.label("Right-click here"))->
		context_menu(gk.popup_menu()->submenu_with_icon(gk.icon(.Folder), "Zoom", zoom)->submenu_with_icon(nil, "Plain", plain))
}

menu_item_clicked :: proc(a: ^App, cx: ^gk.Cx) {
	a.count += 1
}

views_menu_items :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	items := gk.popup_menu()->
		item(gk.popup_menu_item("Copy")->icon(.Copy)->action(zoom_in))->
		item(gk.popup_menu_item_separator())->
		item(gk.popup_menu_item_label("Section"))->
		item(gk.popup_menu_item("Checked")->checked(true)->action(zoom_out))->
		item(gk.popup_menu_item("Off")->disabled(true))->
		item(gk.popup_menu_item("Count")->on_click(gk.on(menu_item_clicked)))->
		item(gk.popup_menu_item_element(gk.label("an element")))->
		item(gk.popup_menu_item_link("Docs", "https://example.com"))
	return gk.div(.Border_1)->w(gk.px(300))->h(gk.px(120))->child(gk.label("Right-click here"))->context_menu(items)
}
