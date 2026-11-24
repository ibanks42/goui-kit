//! ref-notes: the Rust reference for examples/go/notes, written the way a Rust GPUI-kit app is
//! written: the kit's styled dock with two panels (a list of notes over a ListDelegate, an editor
//! with the kit's Input), a File menu whose items are actions with key bindings, a confirmation
//! dialog opened with a closure, and the theme switched with `Theme::change`. State is shared by
//! the app, its panels and the list's delegate through an `Rc<RefCell<..>>`. The actions are heard
//! app-wide, wherever the focus is (`shortcut`).
//!
//!   ref-notes                  a real window
//!   ref-notes --script FILE    offscreen, driven by a gk-meter script (diag build)
use gpui_kit::base::dock::{DockArea, DockLayout, DockPlacement, Panel as BasePanel, PanelEvent, PanelView};
use gpui_kit::base::{h_flex, v_flex};
use gpui_kit::component::button::{Button, ButtonVariants};
use gpui_kit::component::dialog::DialogEntrance;
use gpui_kit::component::dock::{DockSkin, Panel as KitPanel, PanelHandle};
use gpui_kit::component::input::{Input, InputEvent, InputState};
use gpui_kit::component::label::Label;
use gpui_kit::component::list::{List, ListDelegate, ListEvent, ListItem, ListState};
use gpui_kit::component::menu::{DropdownMenu, PopupMenu};
use gpui_kit::component::{ActiveTheme, IndexPath, Theme, ThemeMode, WindowExt};
use gpui_kit::{
    Action, AnyWindowHandle, App, AppContext, Context, Entity, EventEmitter, FocusHandle, Focusable, InteractiveElement,
    IntoElement, KeyBinding, ParentElement, Render, SharedString, Styled, Subscription, WeakEntity, Window, div,
};
use std::cell::RefCell;
use std::rc::Rc;
use std::sync::Arc;

gpui_kit::actions!(notes, [NewNote, DeleteNote, ToggleTheme]);

/// An app-wide listener for action A: it runs `f` on the app in its window once the key's dispatch
/// is over (a global listener runs inside the window's dispatch, where the window is taken).
fn shortcut<A: Action>(cx: &mut App, app: WeakEntity<NotesApp>, wh: AnyWindowHandle, f: fn(&mut NotesApp, &mut Window, &mut Context<NotesApp>)) {
    cx.on_action(move |_: &A, cx: &mut App| {
        let app = app.clone();
        cx.defer(move |cx| {
            let _ = wh.update(cx, |_, window, cx| app.update(cx, |app, cx| f(app, window, cx)));
        });
    });
}

struct Note {
    title: SharedString,
    words: usize,
}

/// What the app, its two panels and the list's delegate share.
struct Model {
    notes: Vec<Note>,
    selected: Option<usize>,
}

type Shared = Rc<RefCell<Model>>;

// --- the list of notes -------------------------------------------------------------------------------

struct NoteRows {
    model: Shared,
}

impl ListDelegate for NoteRows {
    type Item = ListItem;

    fn items_count(&self, _: usize, _: &App) -> usize {
        self.model.borrow().notes.len()
    }

    fn render_item(&mut self, ix: IndexPath, _: &mut Window, cx: &mut Context<ListState<Self>>) -> Option<ListItem> {
        let m = self.model.borrow();
        let n = m.notes.get(ix.row)?;
        Some(
            ListItem::new(("note", ix.row)).child(
                v_flex().child(Label::new(n.title.clone())).child(
                    Label::new(format!("{} words", n.words))
                        .text_sm()
                        .text_color(cx.theme().muted_foreground),
                ),
            ),
        )
    }

    fn set_selected_index(&mut self, _: Option<IndexPath>, _: &mut Window, _: &mut Context<ListState<Self>>) {}
}

// --- the two dock panels ------------------------------------------------------------------------------

struct NotesPanel {
    list: Entity<ListState<NoteRows>>,
    focus: FocusHandle,
}

struct EditorPanel {
    app: WeakEntity<NotesApp>,
    model: Shared,
    title: Entity<InputState>,
    focus: FocusHandle,
}

impl Render for NotesPanel {
    fn render(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        div().size_full().child(List::new(&self.list).scrollbar_visible(false))
    }
}

impl Render for EditorPanel {
    fn render(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
        let m = self.model.borrow();
        let Some(sel) = m.selected else {
            return v_flex().size_full().p_4().items_center().child(Label::new("No note selected"));
        };
        let app = self.app.clone();
        v_flex()
            .size_full()
            .p_4()
            .gap_3()
            .child(Label::new("Title"))
            .child(Input::new(&self.title))
            .child(Label::new(format!("{} words", m.notes[sel].words)))
            .child(
                h_flex().gap_2().child(Button::new("delete").danger().label("Delete").on_click(move |_, window, cx| {
                    app.update(cx, |app, cx| app.ask_delete(window, cx)).ok();
                })),
            )
    }
}

macro_rules! panel {
    ($t:ty, $name:literal, $title:literal) => {
        impl EventEmitter<PanelEvent> for $t {}
        impl Focusable for $t {
            fn focus_handle(&self, _: &App) -> FocusHandle {
                self.focus.clone()
            }
        }
        impl BasePanel for $t {
            fn panel_name(&self) -> &'static str {
                $name
            }
        }
        impl KitPanel for $t {
            fn title(&mut self, _: &mut Window, _: &mut Context<Self>) -> impl IntoElement {
                SharedString::from($title)
            }
        }
    };
}
panel!(NotesPanel, "notes", "Notes");
panel!(EditorPanel, "editor", "Editor");

// --- the app ------------------------------------------------------------------------------------------

struct NotesApp {
    model: Shared,
    made: usize,
    dark: bool,
    title: Entity<InputState>,
    list: Entity<ListState<NoteRows>>,
    dock: Entity<DockArea>,
    editor: Entity<EditorPanel>,
    focus: FocusHandle,
    _subs: Vec<Subscription>,
}

impl NotesApp {
    fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let notes = [("Groceries", 4), ("Ideas for the weekend", 12), ("Meeting notes", 31)]
            .into_iter()
            .map(|(t, w)| Note { title: t.into(), words: w })
            .collect::<Vec<_>>();
        let made = notes.len();
        let model = Rc::new(RefCell::new(Model { notes, selected: Some(0) }));
        cx.bind_keys([KeyBinding::new("ctrl-n", NewNote, None), KeyBinding::new("ctrl-t", ToggleTheme, None)]);
        let (me, wh) = (cx.weak_entity(), window.window_handle());
        shortcut::<NewNote>(cx, me.clone(), wh, Self::new_note);
        shortcut::<DeleteNote>(cx, me.clone(), wh, Self::ask_delete);
        shortcut::<ToggleTheme>(cx, me, wh, Self::toggle_theme);
        let focus = cx.focus_handle();
        window.focus(&focus, cx);

        let title = cx.new(|cx| InputState::new(window, cx).placeholder("Title"));
        let list = cx.new(|cx| ListState::new(NoteRows { model: model.clone() }, window, cx));
        let subs = vec![
            cx.subscribe_in(&title, window, |this, _, e: &InputEvent, window, cx| {
                if matches!(e, InputEvent::Change) {
                    this.title_changed(window, cx);
                }
            }),
            cx.subscribe_in(&list, window, |this, _, e: &ListEvent, window, cx| match e {
                ListEvent::Select(ix) | ListEvent::Confirm(ix) => this.select(Some(ix.row), window, cx),
                _ => {}
            }),
        ];

        let notes_panel = cx.new(|cx| NotesPanel { list: list.clone(), focus: cx.focus_handle() });
        let app = cx.weak_entity();
        let editor = cx.new(|cx| EditorPanel { app, model: model.clone(), title: title.clone(), focus: cx.focus_handle() });
        let (dock, _skin) = DockSkin::dock_area("notes", None, window, cx);
        // PanelHandle: the kit's skin draws a panel's title only through it
        let notes_view: Arc<dyn PanelView> = Arc::new(PanelHandle::new(notes_panel));
        let editor_view: Arc<dyn PanelView> = Arc::new(PanelHandle::new(editor.clone()));
        dock.update(cx, |d, cx| {
            d.set_dock(DockPlacement::Left, DockLayout::tabs().panel_view(notes_view, cx), window, cx);
            d.set_center(DockLayout::tabs().panel_view(editor_view, cx), window, cx);
        });
        // the first note selected
        let first = model.borrow().notes[0].title.clone();
        title.update(cx, |s, cx| s.set_value(first, window, cx));
        list.update(cx, |l, cx| {
            l.set_selected_index(Some(IndexPath::default()), window, cx);
            cx.notify();
        });
        NotesApp { model, made, dark: false, title, list, dock, editor, focus, _subs: subs }
    }

    fn select(&mut self, row: Option<usize>, window: &mut Window, cx: &mut Context<Self>) {
        self.model.borrow_mut().selected = row;
        if let Some(r) = row {
            let t = self.model.borrow().notes[r].title.clone();
            self.title.update(cx, |s, cx| s.set_value(t, window, cx));
        }
        self.list.update(cx, |l, cx| {
            l.set_selected_index(row.map(|r| IndexPath::default().row(r)), window, cx);
            cx.notify();
        });
        self.editor.update(cx, |_, cx| cx.notify());
        cx.notify();
    }

    fn title_changed(&mut self, _: &mut Window, cx: &mut Context<Self>) {
        let value: SharedString = self.title.read(cx).value().into();
        let mut m = self.model.borrow_mut();
        let Some(sel) = m.selected else { return };
        if m.notes[sel].title == value {
            return;
        }
        m.notes[sel].title = value;
        drop(m);
        self.list.update(cx, |_, cx| cx.notify());
    }

    fn new_note(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.made += 1;
        let title = SharedString::from(format!("Untitled {}", self.made));
        self.model.borrow_mut().notes.push(Note { title, words: 0 });
        let last = self.model.borrow().notes.len() - 1;
        self.select(Some(last), window, cx);
    }

    fn ask_delete(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.model.borrow().selected.is_none() {
            return;
        }
        let this = cx.weak_entity();
        let model = self.model.clone();
        window.open_alert_dialog(cx, move |a, _, _| {
            let m = model.borrow();
            let name = m.selected.map(|s| m.notes[s].title.clone()).unwrap_or_default();
            let this = this.clone();
            a.title(Label::new(format!("Delete \"{name}\"?")))
                .description(Label::new("The note will be gone for good."))
                .entrance(DialogEntrance::None)
                .show_cancel(true)
                .on_ok(move |_, window, cx| {
                    this.update(cx, |this, cx| this.confirm_delete(window, cx)).ok();
                    true // close the dialog
                })
        });
    }

    fn confirm_delete(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let (next, empty) = {
            let mut m = self.model.borrow_mut();
            let Some(sel) = m.selected else { return };
            m.notes.remove(sel);
            (sel.min(m.notes.len().saturating_sub(1)), m.notes.is_empty())
        };
        self.select(if empty { None } else { Some(next) }, window, cx);
    }

    fn toggle_theme(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.dark = !self.dark;
        Theme::change(if self.dark { ThemeMode::Dark } else { ThemeMode::Light }, Some(window), cx);
        cx.notify();
    }

    fn file_menu(m: PopupMenu, nothing_selected: bool) -> PopupMenu {
        m.menu("New note", Box::new(NewNote))
            .menu("Toggle theme", Box::new(ToggleTheme))
            .separator()
            .menu_with_disabled("Delete note…", Box::new(DeleteNote), nothing_selected)
    }
}

impl Render for NotesApp {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let (count, none) = {
            let m = self.model.borrow();
            (m.notes.len(), m.selected.is_none())
        };
        let bar = h_flex()
            .gap_2()
            .items_center()
            .p_2()
            .border_b_1()
            .border_color(cx.theme().border)
            .child(Button::new("file").label("File").dropdown_menu(move |m, _, _| Self::file_menu(m, none)))
            .child(Label::new(format!("{count} notes")))
            .child(div().flex_1())
            .child(
                Button::new("theme")
                    .label(if self.dark { "Light theme" } else { "Dark theme" })
                    .on_click(cx.listener(|this, _, window, cx| this.toggle_theme(window, cx))),
            );
        v_flex()
            .size_full()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .key_context("Notes")
            .track_focus(&self.focus)
            .child(bar)
            .child(div().flex_1().w_full().min_h_0().child(self.dock.clone()))
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<NotesApp> {
    gk_meter::attach(window, cx);
    cx.new(|cx| NotesApp::new(window, cx))
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(800., 600., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-notes: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application().with_assets(gpui_kit::assets::Assets).run(move |cx| {
        gpui_kit::init(cx);
        let options = gk_meter::window_options(800., 600., cx);
        gpui_kit::open_window(options, cx, build).expect("open_window");
        cx.activate(true);
    });
}
