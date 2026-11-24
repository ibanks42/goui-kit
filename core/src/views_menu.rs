//! Popup-menu helper for building a submenu item with an optional leading icon.

use gpui_kit::component::Icon;
use gpui_kit::component::menu::{PopupMenu, PopupMenuItem};
use gpui_kit::{App, Context, SharedString, Window};

/// Appends a submenu built by `f` to `menu`, attaching `icon` when provided.
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
