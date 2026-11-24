//! Grouped sidebar (`views-sidebar-groups`) and advanced popup-menu
//! (`views-submenu-icon`, `views-menu-items`) gallery pages.

use gpui_kit::base::Side;
use crate::overlay_pages::{MenuZoomIn, MenuZoomOut};
use gpui_kit::component::menu::{ContextMenuExt, PopupMenu, PopupMenuItem};
use gpui_kit::component::{Collapsible, Icon, IconName};
use gpui_kit::component::label::Label;
use gpui_kit::component::sidebar::{Sidebar, SidebarGroup, SidebarMenu, SidebarMenuItem};
use gpui_kit::{AnyElement, IntoElement, ParentElement, Styled, div, px};

fn zoom(m: PopupMenu) -> PopupMenu {
    m.menu("Zoom in", Box::new(MenuZoomIn)).menu("Zoom out", Box::new(MenuZoomOut))
}

/// Renders one of the `views-*` gallery pages if `id` matches.
pub fn page(id: &str) -> Option<AnyElement> {
    Some(match id {
        "views-sidebar-groups" => div()
            .w(px(240.))
            .h(px(300.))
            .child(
                Sidebar::new("gsb")
                    .side(Side::Left)
                    .collapsible(false)
                    .collapsed(false)
                    .header(Label::new("Mail"))
                    .child(
                        SidebarGroup::new("Inbox").child(
                            SidebarMenu::new()
                                .child(SidebarMenuItem::new("Unread").active(true))
                                .child(SidebarMenuItem::new("Flagged")),
                        ),
                    )
                    .children([SidebarGroup::new("Later")
                        .children([
                            SidebarMenu::new().child(SidebarMenuItem::new("Archive")),
                            SidebarMenu::new().child(SidebarMenuItem::new("Trash")),
                        ])
                        .collapsed(true)]),
            )
            .into_any_element(),
        "views-submenu-icon" => div()
            .w(px(300.))
            .h(px(120.))
            .border_1()
            .child(Label::new("Right-click here"))
            .context_menu(|m, window, cx| {
                m.submenu_with_icon(Some(Icon::new(IconName::Folder)), "Zoom", window, cx, |m, _, _| zoom(m))
                    .submenu_with_icon(None, "Plain", window, cx, |m, _, _| zoom(m))
            })
            .into_any_element(),
        "views-menu-items" => div()
            .w(px(300.))
            .h(px(120.))
            .border_1()
            .child(Label::new("Right-click here"))
            .context_menu(|m, _, _| {
                m.item(PopupMenuItem::new("Copy").icon(IconName::Copy).action(Box::new(MenuZoomIn)))
                    .item(PopupMenuItem::separator())
                    .item(PopupMenuItem::label("Section"))
                    .item(PopupMenuItem::new("Checked").checked(true).action(Box::new(MenuZoomOut)))
                    .item(PopupMenuItem::new("Off").disabled(true))
                    .item(PopupMenuItem::new("Count").on_click(|_, _, _| {}))
                    .item(PopupMenuItem::element(|_, _| Label::new("an element")))
                    .item(PopupMenuItem::link("Docs", "https://example.com"))
            })
            .into_any_element(),
        _ => return None,
    })
}
