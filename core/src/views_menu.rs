//! Menu ops a described menu (docs/PROTOCOLS.md 11.6) cannot make with upstream's own function,
//! which needs the menu entity's Context: done by the library with the App instead, as
//! `crate::overlay::submenu` does for `PopupMenu::submenu`.

use gpui_kit::component::Icon;
use gpui_kit::component::menu::{PopupMenu, PopupMenuItem};
use gpui_kit::{App, Context, SharedString, Window};

/// `PopupMenu::submenu_with_icon` without the menu's own context: the child menu is built at once
/// and attached as an item with the icon, as upstream does; upstream wires it to its parent when
/// the menu draws.
pub fn submenu_with_icon(
    menu: PopupMenu,
    icon: Option<Icon>,
    label: impl Into<SharedString>,
    window: &mut Window,
    cx: &mut App,
    f: impl Fn(PopupMenu, &mut Window, &mut Context<PopupMenu>) -> PopupMenu + 'static,
) -> PopupMenu {
    let child = PopupMenu::build(window, cx, f);
    let item = PopupMenuItem::submenu(label, child);
    menu.item(match icon {
        Some(icon) => item.icon(icon),
        None => item,
    })
}
