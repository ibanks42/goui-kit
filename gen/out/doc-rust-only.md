# Sentences of GPUI-kit's docs left out of the bindings

Written by gen/gk_doc.py: each sentence names Rust-only mechanics (gk_doc.RUST_ONLY).

- `gpui_kit::InteractiveElement::tab_stop`: Useful for container elements: focus the container, then call `window.focus_next(cx)` to focus the first tab stop inside it while having the container element itself be unreachable via the keyboard.
- `gpui_kit::base::InputBase::semantic_owner`: The relationship follows the frame's GPUI element-state lifetime.
- `gpui_kit::base::ResizablePanelGroup::on_resize`: - Entity<ResizableState>: The state of the ResizablePanelGroup.
- `gpui_kit::base::ScrollBounce::on_scroll`: Runs after the handle update, with no internal state borrowed.
- `gpui_kit::base::input::EditorState::selected_text`: Return the active selection's text, borrowed out of the `Rope` the state owns.
- `gpui_kit::base::input::EditorState::selected_value`: See `SelectedText` for the same selection borrowed out of the `Rope` the state owns.
- `gpui_kit::base::input::EditorState::set_scrollbar_renderer`: `EditorScrollbarContext`: crate::input::EditorScrollbarContext
- `gpui_kit::base::input::EditorState::value`: See `Text` for the `Rope` the state owns, which is borrowed and costs nothing to read.
- `gpui_kit::base::input::InputState::selected_text`: Return the active selection's text, borrowed out of the `Rope` the state owns.
- `gpui_kit::base::input::InputState::selected_value`: See `SelectedText` for the same selection borrowed out of the `Rope` the state owns.
- `gpui_kit::base::input::InputState::set_scrollbar_renderer`: `EditorScrollbarContext`: crate::input::EditorScrollbarContext
- `gpui_kit::base::input::InputState::value`: See `Text` for the `Rope` the state owns, which is borrowed and costs nothing to read.
- `gpui_kit::base::input::TextareaState::selected_text`: Return the active selection's text, borrowed out of the `Rope` the state owns.
- `gpui_kit::base::input::TextareaState::selected_value`: See `SelectedText` for the same selection borrowed out of the `Rope` the state owns.
- `gpui_kit::base::input::TextareaState::set_scrollbar_renderer`: `EditorScrollbarContext`: crate::input::EditorScrollbarContext
- `gpui_kit::base::input::TextareaState::value`: See `Text` for the `Rope` the state owns, which is borrowed and costs nothing to read.
- `gpui_kit::component::TitleBar::on_close_window`: Add custom for close window event, default is None, then click X button will call `window.remove_window()`.
- `gpui_kit::component::WindowExt::open_dialog`: Creating them with `cx.new(...)` inside the builder replaces them on each rebuild and loses their editing state.
- `gpui_kit::component::dock::PanelHandle::of`: Recover the handle behind one of base's, or `None` when base is holding a panel this crate did not wrap — a bare `Entity<P>` handed straight to `panel`, say.
- `gpui_kit::component::dock::PanelHandle::panel`: Owned rather than borrowed because the title bar's ellipsis menu is built inside a `'static` callback: the menu closure outlives the `render_tab_bar` call that created it, so it cannot borrow from the render context it was made in.
- `gpui_kit::component::message_scroller::MessageScrollerState::new`: The constructor receives the entity context so the list's scroll handler can safely defer its entity update until GPUI has released the list's internal borrow.
- `gpui_kit::component::table::TableRow::hover`: Apply the given style to the row while the pointer is over it, such as `.hover(|this| this.bg(cx.theme().table_hover))`.
- `gpui_kit::component::try_parse_color`: - `name` - The color name `black`, `white`, or any other defined in `crate::color`.
