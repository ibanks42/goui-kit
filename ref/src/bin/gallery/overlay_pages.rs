//! The overlays family's gallery pages (docs/PROTOCOLS.md section 11.6): a dialog, an alert, a
//! sheet at the right and at the left, a dropdown menu and a context menu with a submenu, each
//! opened by the page's script (a click on the page's button, or a right click), written the way
//! a Rust GPUI-kit app writes them, with closures. The Odin twin is
//! examples/odin/gallery/overlay_pages.odin, whose dialogs are views (gk.built) and whose menus
//! are described values; the pages' lines are in ref/gallery/pages.txt.
use gpui_kit::component::button::Button;
use gpui_kit::component::dialog::DialogEntrance;
use gpui_kit::component::label::Label;
use gpui_kit::component::menu::{ContextMenuExt, DropdownMenu, PopupMenu};
use gpui_kit::component::{Placement, WindowExt};
use gpui_kit::base::dock::{DockArea, DockLayout, DockPlacement, Panel as BasePanel, PanelEvent};
use gpui_kit::component::dock::{DockSkin, Panel as KitPanel};
use gpui_kit::{
    AnyElement, App, AppContext, Context, Entity, EventEmitter, FocusHandle, Focusable, IntoElement, ParentElement,
    Render, SharedString, Styled, Window, div, px,
};
use std::cell::RefCell;

gpui_kit::actions!(gallery_menu, [MenuCopy, MenuPaste, MenuDelete, MenuPinned, MenuZoomIn, MenuZoomOut, MenuKbd]);

fn menu(m: PopupMenu) -> PopupMenu {
    m.menu("Copy", Box::new(MenuCopy))
        .menu("Paste", Box::new(MenuPaste))
        .separator()
        .label("Danger")
        .menu_with_check("Pinned", true, Box::new(MenuPinned))
        .menu_with_disabled("Delete", Box::new(MenuDelete), true)
}

pub fn page(id: &str, window: &mut Window, cx: &mut App) -> Option<AnyElement> {
    Some(match id {
        "dock-panels" => div().w(px(600.)).h(px(300.)).child(dock(window, cx)).into_any_element(),
        "dock-split" => div().w(px(600.)).h(px(300.)).child(r3::split(window, cx)).into_any_element(),
        "dock-hooks" => div().w(px(600.)).h(px(300.)).child(r3::hooks(window, cx)).into_any_element(),
        "dock-restore" => div().w(px(600.)).h(px(300.)).child(r3::restored(window, cx)).into_any_element(),
        "dock-skin" => div().w(px(600.)).h(px(300.)).child(r3::skinned(window, cx)).into_any_element(),
        "dock-renderer" => div().w(px(600.)).h(px(300.)).child(r4::rendered(window, cx)).into_any_element(),
        "overlay-popover-open" => gpui_kit::component::popover::Popover::new("pop")
            .trigger(Button::new("trigger").label("Open"))
            .open(true)
            .child(Label::new("inside the popover"))
            .into_any_element(),
        "menu-kbd" => {
            r3::bind_once(cx);
            gpui_kit::base::v_flex()
                .children(gpui_kit::component::kbd::Kbd::binding_for_action(&MenuKbd, None, window))
                .children(gpui_kit::component::kbd::Kbd::binding_for_action(&MenuPaste, None, window))
                .children(gpui_kit::component::kbd::Kbd::global_binding_for_action(&MenuKbd, window))
                .into_any_element()
        }
        "menu-dropdown-anchor" => Button::new("open")
            .label("Open")
            .dropdown_menu_with_anchor(gpui_kit::Anchor::TopRight, |m, _, _| menu(m))
            .into_any_element(),
        "overlay-dialog" => Button::new("open")
            .label("Open")
            .on_click(|_, window, cx| {
                window.open_dialog(cx, |d, _, _| {
                    d.title(Label::new("Delete report.txt?"))
                        .entrance(DialogEntrance::None)
                        .child(Label::new("The file will be gone for good."))
                        .footer(Label::new("Press Escape to keep it."))
                })
            })
            .into_any_element(),
        "overlay-alert" => Button::new("open")
            .label("Open")
            .on_click(|_, window, cx| {
                window.open_alert_dialog(cx, |a, _, _| {
                    a.title(Label::new("Unsaved changes"))
                        .description(Label::new("Leave without saving?"))
                        .entrance(DialogEntrance::None)
                        .show_cancel(true)
                })
            })
            .into_any_element(),
        "overlay-sheet" => Button::new("open")
            .label("Open")
            .on_click(|_, window, cx| {
                window.open_sheet(cx, |s, _, _| s.title(Label::new("Settings")).child(Label::new("The sheet's body")))
            })
            .into_any_element(),
        "overlay-sheet-left" => Button::new("open")
            .label("Open")
            .on_click(|_, window, cx| {
                window.open_sheet_at(Placement::Left, cx, |s, _, _| {
                    s.title(Label::new("Files")).resizable(false).child(Label::new("On the left"))
                })
            })
            .into_any_element(),
        "overlay-dropdown" => Button::new("open")
            .label("Open")
            .dropdown_menu(|m, _, _| menu(m))
            .into_any_element(),
        "overlay-context" => div()
            .w(px(300.))
            .h(px(120.))
            .border_1()
            .child(Label::new("Right-click here"))
            .context_menu(|m, window, cx| {
                menu(m).separator().submenu("Zoom", window, cx, |m, _, _| {
                    m.menu("Zoom in", Box::new(MenuZoomIn)).menu("Zoom out", Box::new(MenuZoomOut))
                })
            })
            .into_any_element(),
        _ => return None,
    })
}

/// A dock panel that shows one line of text: what the Odin page's panels are (an Odin view each).
struct TextPanel {
    name: &'static str,
    title: &'static str,
    body: &'static str,
    focus: FocusHandle,
}

impl EventEmitter<PanelEvent> for TextPanel {}

impl Focusable for TextPanel {
    fn focus_handle(&self, _: &App) -> FocusHandle {
        self.focus.clone()
    }
}

impl Render for TextPanel {
    fn render(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        Label::new(self.body)
    }
}

impl BasePanel for TextPanel {
    fn panel_name(&self) -> &'static str {
        self.name
    }
}

impl KitPanel for TextPanel {
    fn title(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        SharedString::from(self.title)
    }
}

thread_local! {
    static DOCK: RefCell<Option<Entity<DockArea>>> = const { RefCell::new(None) };
}

/// The kit's styled dock with two tabs in the center and one panel in the left dock, made once.
fn dock(window: &mut Window, cx: &mut App) -> Entity<DockArea> {
    if let Some(d) = DOCK.with(|d| d.borrow().clone()) {
        return d;
    }
    let (area, _skin) = DockSkin::dock_area("main", None, window, cx);
    let mut panel = |name, title, body| cx.new(|cx| TextPanel { name, title, body, focus: cx.focus_handle() });
    let files = panel("files", "Files", "src/main.odin");
    let editor = panel("editor", "Editor", "package main");
    let outline = panel("outline", "Outline", "main :: proc()");
    area.update(cx, |a, cx| {
        a.set_center(DockLayout::tabs().panel(files).panel(editor), window, cx);
        a.set_dock(DockPlacement::Left, DockLayout::tabs().panel(outline), window, cx);
    });
    DOCK.with(|d| *d.borrow_mut() = Some(area.clone()));
    area
}

/// docs/PROTOCOLS.md 11.8: what the Odin pages dock-split, dock-hooks and dock-restore do,
/// the way a Rust app does it.
mod r3 {
    use super::TextPanel;
    use gpui_kit::base::dock::{
        DockArea, DockAreaState, DockLayout, Panel as BasePanel, PanelEvent, PanelInfo, PanelState, PanelView,
        register_panel,
    };
    use gpui_kit::base::Placement as BasePlacement;
    use gpui_kit::component::button::Button;
    use gpui_kit::component::dock::{DockSkin, Panel as KitPanel};
    use gpui_kit::component::label::Label;
    use gpui_kit::{
        App, AppContext, Axis, Context, Entity, EventEmitter, FocusHandle, Focusable, IntoElement, Render,
        SharedString, Window, px,
    };
    use std::cell::RefCell;
    use std::collections::HashMap;
    use std::sync::Arc;

    thread_local! {
        static DOCKS: RefCell<HashMap<&'static str, Entity<DockArea>>> = RefCell::new(HashMap::new());
    }

    fn once(key: &'static str, window: &mut Window, cx: &mut App, make: impl FnOnce(&mut Window, &mut App) -> Entity<DockArea>) -> Entity<DockArea> {
        if let Some(d) = DOCKS.with(|d| d.borrow().get(key).cloned()) {
            return d;
        }
        let d = make(window, cx);
        DOCKS.with(|m| m.borrow_mut().insert(key, d.clone()));
        d
    }

    fn text(cx: &mut App, name: &'static str, title: &'static str, body: &'static str) -> Entity<TextPanel> {
        cx.new(|cx| TextPanel { name, title, body, focus: cx.focus_handle() })
    }

    pub fn split(window: &mut Window, cx: &mut App) -> Entity<DockArea> {
        once("split", window, cx, |window, cx| {
            let (area, _skin) = DockSkin::dock_area("split", None, window, cx);
            let files = text(cx, "files", "Files", "src/main.odin");
            let editor = text(cx, "editor", "Editor", "package main");
            let outline = text(cx, "outline", "Outline", "main :: proc()");
            let (eid, oid) = (editor.entity_id().into(), outline.entity_id().into());
            area.update(cx, |a, cx| {
                a.set_center(DockLayout::tabs().panel(files).panel(editor).panel(outline), window, cx);
                let group = a.layout(gpui_kit::base::dock::DockPlacement::Center).and_then(|t| t.find_panel_node(eid)).unwrap();
                a.split_at(group, oid, BasePlacement::Right, window, cx);
                a.split_at(group, eid, BasePlacement::Bottom, window, cx);
            });
            area
        })
    }

    /// A panel with a title suffix and toolbar buttons.
    struct Hooked {
        focus: FocusHandle,
    }
    impl EventEmitter<PanelEvent> for Hooked {}
    impl Focusable for Hooked {
        fn focus_handle(&self, _: &App) -> FocusHandle {
            self.focus.clone()
        }
    }
    impl Render for Hooked {
        fn render(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
            Label::new("Panel body")
        }
    }
    impl BasePanel for Hooked {
        fn panel_name(&self) -> &'static str {
            "hooked"
        }
    }
    impl KitPanel for Hooked {
        fn title(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
            SharedString::from("Hooked")
        }
        fn title_suffix(&mut self, _: &mut Window, _: &mut Context<Self>) -> Option<impl IntoElement> {
            Some(Label::new("v0.9.1"))
        }
        fn toolbar_buttons(&mut self, _: &mut Window, _: &mut Context<Self>) -> Option<Vec<Button>> {
            Some(vec![Button::new("run").label("Run"), Button::new("stop").label("Stop")])
        }
        fn dropdown_menu(
            &mut self,
            menu: gpui_kit::component::menu::PopupMenu,
            _: &mut Window,
            _: &mut Context<Self>,
        ) -> gpui_kit::component::menu::PopupMenu {
            menu.separator().menu("Copy", Box::new(super::MenuCopy))
        }
    }

    pub fn hooks(window: &mut Window, cx: &mut App) -> Entity<DockArea> {
        once("hooks", window, cx, |window, cx| {
            let (area, _skin) = DockSkin::dock_area("hooks", None, window, cx);
            let hooked = cx.new(|cx| Hooked { focus: cx.focus_handle() });
            let handle: Arc<dyn PanelView> = Arc::new(gpui_kit::component::dock::PanelHandle::new(hooked));
            area.update(cx, |a, cx| a.set_center(DockLayout::tabs().panel_view(handle, cx), window, cx));
            area
        })
    }

    fn leaf(name: &str) -> PanelState {
        PanelState::new(name)
    }

    thread_local! {
        static BOUND: std::cell::Cell<bool> = const { std::cell::Cell::new(false) };
    }

    /// ctrl-s for MenuKbd, as the Odin gallery binds it at start (only the page menu-kbd names it).
    pub fn bind_once(cx: &mut App) {
        if !BOUND.with(|b| b.replace(true)) {
            cx.bind_keys([gpui_kit::KeyBinding::new("ctrl-s", super::MenuKbd, None)]);
        }
    }

    pub fn skinned(window: &mut Window, cx: &mut App) -> Entity<DockArea> {
        once("skin", window, cx, |window, cx| {
            let (area, skin) = DockSkin::dock_area("skin", None, window, cx);
            let files = text(cx, "files", "Files", "src/main.odin");
            area.update(cx, |a, cx| a.set_center(DockLayout::tabs().panel(files), window, cx));
            skin.set_panel_style(gpui_kit::component::dock::PanelStyle::TabBar, cx);
            skin.set_close_button_visible(false, cx);
            area
        })
    }

    pub fn restored(window: &mut Window, cx: &mut App) -> Entity<DockArea> {
        once("restored", window, cx, |window, cx| {
            for (name, title, body) in
                [("files", "Files", "src/main.odin"), ("editor", "Editor", "package main"), ("outline", "Outline", "main :: proc()")]
            {
                register_panel(cx, name, move |_, _, cx| Arc::new(text(cx, name, title, body)) as Arc<dyn PanelView>);
            }
            let (area, _skin) = DockSkin::dock_area("restored", None, window, cx);
            let mut left = PanelState::new("TabPanel");
            left.children = vec![leaf("files")];
            left.info = PanelInfo::tabs(0);
            let mut right = PanelState::new("TabPanel");
            right.children = vec![leaf("editor"), leaf("outline")];
            right.info = PanelInfo::tabs(1);
            let mut center = PanelState::new("StackPanel");
            center.children = vec![left, right];
            center.info = PanelInfo::stack(vec![px(200.), px(400.)], Axis::Horizontal);
            let state = DockAreaState { version: None, center, left_dock: None, right_dock: None, bottom_dock: None };
            area.update(cx, |a, cx| a.load(state, window, cx).ok());
            area
        })
    }
}

/// docs/PROTOCOLS.md 11.9: a dock drawn by a renderer of the app's own, the way a Rust app
/// writes one: a padded frame, each dock's chrome a title over its content, the tab bar a label.
mod r4 {
    use super::TextPanel;
    use gpui_kit::base::dock::{
        DockArea, DockAreaRenderer, DockContext, DockLayout, DockPlacement, TabGroupContext, TabGroupRenderer,
    };
    use gpui_kit::component::label::Label;
    use gpui_kit::{
        AnyElement, App, AppContext, Div, Entity, InteractiveElement, IntoElement, ParentElement, Stateful, Styled,
        Window,
    };
    use std::cell::RefCell;
    use std::rc::Rc;

    struct Chrome;

    impl DockAreaRenderer for Chrome {
        fn frame(&self, _: &mut Window, _: &mut App) -> Stateful<Div> {
            gpui_kit::base::h_flex().size_full().p_1().id("dock-area")
        }

        fn render_dock(&self, dock: &DockContext, content: AnyElement, _: &mut Window, _: &mut App) -> AnyElement {
            gpui_kit::base::v_flex()
                .size_full()
                .child(Label::new(format!("{:?} open={}", dock.placement(), dock.is_open())))
                .child(content)
                .into_any_element()
        }

        fn tab_group_renderer(&self) -> Rc<dyn TabGroupRenderer> {
            Rc::new(Chrome)
        }
    }

    impl TabGroupRenderer for Chrome {
        fn render_tab_bar(&self, _: &TabGroupContext, _: &mut Window, _: &mut App) -> AnyElement {
            gpui_kit::base::h_flex().w_full().child(Label::new("tabs of the app")).into_any_element()
        }
    }

    thread_local! {
        static DOCK: RefCell<Option<Entity<DockArea>>> = const { RefCell::new(None) };
    }

    fn text(cx: &mut App, name: &'static str, title: &'static str, body: &'static str) -> Entity<TextPanel> {
        cx.new(|cx| TextPanel { name, title, body, focus: cx.focus_handle() })
    }

    pub fn rendered(window: &mut Window, cx: &mut App) -> Entity<DockArea> {
        if let Some(d) = DOCK.with(|d| d.borrow().clone()) {
            return d;
        }
        let files = text(cx, "files", "Files", "src/main.odin");
        let outline = text(cx, "outline", "Outline", "main :: proc()");
        let area = cx.new(|cx| DockArea::new("rendered", None, window, cx).with_renderer(Rc::new(Chrome)));
        area.update(cx, |a, cx| {
            a.set_center(DockLayout::tabs().panel(files), window, cx);
            a.set_dock(DockPlacement::Left, DockLayout::tabs().panel(outline), window, cx);
        });
        DOCK.with(|d| *d.borrow_mut() = Some(area.clone()));
        area
    }
}
