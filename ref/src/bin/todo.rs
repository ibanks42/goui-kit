//! Pure-Rust multi-project task manager reference app with JSON persistence
//! (`TODO_FILE`, defaulting to `todo.json`), priority/due-date tags, filter
//! pills, search, edit/delete dialogs, keyboard shortcuts, and a simulated
//! background sync worker.

use futures::StreamExt;
use futures::channel::mpsc;
use gpui_kit::base::{h_flex, h_resizable, resizable_panel, v_flex};
use gpui_kit::component::button::{Button, ButtonVariants};
use gpui_kit::component::checkbox::Checkbox;
use gpui_kit::component::dialog::{DialogAction, DialogClose, DialogEntrance, DialogFooter};
use gpui_kit::component::input::{Input, InputEvent, InputState};
use gpui_kit::component::label::Label;
use gpui_kit::component::list::{List, ListDelegate, ListEvent, ListItem, ListState};
use gpui_kit::component::menu::{DropdownMenu, PopupMenu};
use gpui_kit::component::tag::Tag;
use gpui_kit::component::{ActiveTheme, Icon, IconName, IndexPath, Selectable, Sizable, Theme, ThemeMode, WindowExt};
use gpui_kit::{
    Action, AnyWindowHandle, App, AppContext, Context, Entity, FocusHandle, Hsla, InteractiveElement, IntoElement,
    KeyBinding, ParentElement, Render, ScrollStrategy, StatefulInteractiveElement, Styled, Subscription, WeakEntity,
    Window, div, px,
};
use serde::{Deserialize, Serialize};
use std::cell::RefCell;
use std::rc::Rc;

gpui_kit::actions!(todo, [NewTask, DeleteTask, Find, ToggleTheme]);

/// Registers an application action handler that defers updating `TodoApp` in `wh`.
fn shortcut<A: Action>(cx: &mut App, app: WeakEntity<TodoApp>, wh: AnyWindowHandle, f: fn(&mut TodoApp, &mut Window, &mut Context<TodoApp>)) {
    cx.on_action(move |_: &A, cx: &mut App| {
        let app = app.clone();
        cx.defer(move |cx| {
            let _ = wh.update(cx, |_, window, cx| {
                app.update(cx, |app, cx| {
                    f(app, window, cx);
                    cx.notify();
                })
            });
        });
    });
}

#[derive(Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
enum Priority {
    Low,
    Medium,
    High,
}

const PRIORITIES: [Priority; 3] = [Priority::Low, Priority::Medium, Priority::High];

impl Priority {
    fn name(self) -> &'static str {
        ["Low", "Medium", "High"][self as usize]
    }

    fn color(self) -> Hsla {
        match self {
            Priority::Low => gpui_kit::component::sky_500(),
            Priority::Medium => gpui_kit::component::amber_500(),
            Priority::High => gpui_kit::component::red_500(),
        }
    }
}

#[derive(Clone, Serialize, Deserialize)]
struct Task {
    title: String,
    done: bool,
    due: String,
    priority: Priority,
}

#[derive(Clone, Serialize, Deserialize)]
struct Project {
    name: String,
    tasks: Vec<Task>,
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum Filter {
    All,
    Active,
    Done,
}

impl Filter {
    fn name(self) -> &'static str {
        ["All", "Active", "Done"][self as usize]
    }
}

#[derive(Serialize, Deserialize)]
struct Saved {
    projects: Vec<Project>,
    current: i64,
    dark: bool,
}

struct Model {
    projects: Vec<Project>,
    current: usize,
    visible: Vec<usize>,
}

impl Model {
    fn tasks(&self) -> &Vec<Task> {
        &self.projects[self.current].tasks
    }
    fn tasks_mut(&mut self) -> &mut Vec<Task> {
        &mut self.projects[self.current].tasks
    }
}

type Shared = Rc<RefCell<Model>>;

fn open_count(p: &Project) -> usize {
    p.tasks.iter().filter(|t| !t.done).count()
}

struct ProjectRows {
    model: Shared,
}

impl ListDelegate for ProjectRows {
    type Item = ListItem;

    fn items_count(&self, _: usize, _: &App) -> usize {
        self.model.borrow().projects.len()
    }

    fn render_item(&mut self, ix: IndexPath, _: &mut Window, cx: &mut Context<ListState<Self>>) -> Option<ListItem> {
        let m = self.model.borrow();
        let p = m.projects.get(ix.row)?;
        Some(
            ListItem::new(("project", ix.row)).child(
                h_flex()
                    .w_full()
                    .justify_between()
                    .child(Label::new(p.name.clone()))
                    .child(Label::new(open_count(p).to_string()).text_sm().text_color(cx.theme().muted_foreground)),
            ),
        )
    }

    fn set_selected_index(&mut self, _: Option<IndexPath>, _: &mut Window, _: &mut Context<ListState<Self>>) {}
}

struct TaskRows {
    model: Shared,
    app: WeakEntity<TodoApp>,
}

impl ListDelegate for TaskRows {
    type Item = ListItem;

    fn items_count(&self, _: usize, _: &App) -> usize {
        self.model.borrow().visible.len()
    }

    fn render_item(&mut self, ix: IndexPath, _: &mut Window, cx: &mut Context<ListState<Self>>) -> Option<ListItem> {
        let m = self.model.borrow();
        let i = *m.visible.get(ix.row)?;
        let t = m.tasks()[i].clone();
        drop(m);
        let mut title = Label::new(t.title.clone());
        if t.done {
            title = title.line_through().text_color(cx.theme().muted_foreground);
        }
        let (app, app2) = (self.app.clone(), self.app.clone());
        let mut row = h_flex()
            .w_full()
            .gap_2()
            .items_center()
            .child(Checkbox::new(("done", i)).checked(t.done).on_click(move |done, window, cx| {
                cx.stop_propagation();
                let done = *done;
                app.update(cx, |app, cx| {
                    app.model.borrow_mut().tasks_mut()[i].done = done;
                    app.changed(window, cx);
                })
                .ok();
            }))
            .child(div().rounded_full().w(px(8.)).h(px(8.)).bg(t.priority.color()))
            .child(div().flex_1().id(("title", i)).child(title).on_click(move |e, window, cx| {
                if e.click_count() == 2 {
                    app2.update(cx, |app, cx| app.open_editor(Some(i), window, cx)).ok();
                }
            }));
        if !t.due.is_empty() {
            let today = today();
            let tag = if !t.done && t.due < today {
                Tag::danger()
            } else if t.due == today {
                Tag::warning()
            } else {
                Tag::secondary()
            };
            row = row.child(tag.text_xs().child(t.due.clone()));
        }
        Some(ListItem::new(("task", i)).child(row))
    }

    fn set_selected_index(&mut self, _: Option<IndexPath>, _: &mut Window, _: &mut Context<ListState<Self>>) {}
}

/// Returns today's local calendar date formatted as `YYYY-MM-DD` for comparing
/// against ISO due-date strings.
fn today() -> String {
    // SAFETY: `libc::time` accepts a null pointer, `tm` is a plain C struct
    // valid when zero-initialized, and `localtime_r` writes into our stack
    // local `tm` without using shared static storage.
    unsafe {
        let now = libc::time(std::ptr::null_mut());
        let mut tm: libc::tm = std::mem::zeroed();
        libc::localtime_r(&now, &mut tm);
        format!("{:04}-{:02}-{:02}", tm.tm_year + 1900, tm.tm_mon + 1, tm.tm_mday)
    }
}

struct TodoApp {
    model: Shared,
    filter: Filter,
    selected: Option<usize>,
    editing: Option<usize>,
    edit_prio: Priority,
    path: String,
    dark: bool,
    status: String,
    dirty: usize,
    search: Entity<InputState>,
    new_project: Entity<InputState>,
    edit_title: Entity<InputState>,
    edit_due: Entity<InputState>,
    project_list: Entity<ListState<ProjectRows>>,
    task_list: Entity<ListState<TaskRows>>,
    focus: FocusHandle,
    _subs: Vec<Subscription>,
}

impl TodoApp {
    fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let path = std::env::var("TODO_FILE").unwrap_or_else(|_| "todo.json".into());
        let (projects, current, dark) = load(&path);
        let model = Rc::new(RefCell::new(Model { projects, current, visible: Vec::new() }));
        if dark {
            Theme::change(ThemeMode::Dark, Some(window), cx);
        }
        cx.bind_keys([
            KeyBinding::new("ctrl-n", NewTask, None),
            KeyBinding::new("delete", DeleteTask, Some("Tasks")),
            KeyBinding::new("ctrl-f", Find, None),
            KeyBinding::new("ctrl-t", ToggleTheme, None),
        ]);
        let (me, wh) = (cx.weak_entity(), window.window_handle());
        shortcut::<NewTask>(cx, me.clone(), wh, |a, w, cx| a.open_editor(None, w, cx));
        shortcut::<DeleteTask>(cx, me.clone(), wh, Self::ask_delete);
        shortcut::<Find>(cx, me.clone(), wh, |a, w, cx| a.search.update(cx, |s, cx| s.focus(w, cx)));
        shortcut::<ToggleTheme>(cx, me, wh, Self::switch_theme);
        let focus = cx.focus_handle();
        window.focus(&focus, cx);

        let search = cx.new(|cx| InputState::new(window, cx).placeholder("Search tasks"));
        let new_project = cx.new(|cx| InputState::new(window, cx).placeholder("New project"));
        let edit_title = cx.new(|cx| InputState::new(window, cx).placeholder("What needs doing?"));
        let edit_due = cx.new(|cx| InputState::new(window, cx).placeholder("Due date, e.g. 2026-11-20"));
        let project_list = cx.new(|cx| ListState::new(ProjectRows { model: model.clone() }, window, cx));
        let app = cx.weak_entity();
        let task_list = cx.new(|cx| ListState::new(TaskRows { model: model.clone(), app }, window, cx));
        let subs = vec![
            cx.subscribe_in(&search, window, |this, _, e: &InputEvent, window, cx| {
                if matches!(e, InputEvent::Change) {
                    this.refilter(window, cx);
                    this.refresh(cx);
                }
            }),
            cx.subscribe_in(&new_project, window, |this, _, e: &InputEvent, window, cx| {
                if matches!(e, InputEvent::PressEnter { .. }) {
                    this.add_project(window, cx);
                }
            }),
            cx.subscribe_in(&project_list, window, |this, _, e: &ListEvent, window, cx| {
                if let ListEvent::Confirm(ix) = e {
                    if ix.row != this.model.borrow().current {
                        this.model.borrow_mut().current = ix.row;
                        this.selected = None;
                        this.refilter(window, cx);
                        this.refresh(cx);
                    }
                }
            }),
            cx.subscribe_in(&task_list, window, |this, _, e: &ListEvent, _, _| match e {
                ListEvent::Select(ix) | ListEvent::Confirm(ix) => {
                    this.selected = this.model.borrow().visible.get(ix.row).copied();
                }
                _ => {}
            }),
        ];
        project_list.update(cx, |l, cx| l.set_selected_index(Some(IndexPath::default().row(current)), window, cx));

        let mut app = TodoApp {
            model,
            filter: Filter::All,
            selected: None,
            editing: None,
            edit_prio: Priority::Medium,
            path,
            dark,
            status: "not synced yet".into(),
            dirty: 0,
            search,
            new_project,
            edit_title,
            edit_due,
            project_list,
            task_list,
            focus,
            _subs: subs,
        };
        app.refilter(window, cx);
        app.start_sync(window, cx);
        app
    }

    fn save(&self) {
        let m = self.model.borrow();
        let sv = Saved { projects: m.projects.clone(), current: m.current as i64, dark: self.dark };
        if let Ok(data) = serde_json::to_string_pretty(&sv) {
            let _ = std::fs::write(&self.path, data);
        }
    }

    fn refresh(&mut self, cx: &mut Context<Self>) {
        self.project_list.update(cx, |_, cx| cx.notify());
        self.task_list.update(cx, |_, cx| cx.notify());
        cx.notify();
    }

    fn changed(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.dirty += 1;
        self.save();
        self.refilter(window, cx);
        self.refresh(cx);
    }

    fn refilter(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let needle = self.search.read(cx).value().to_lowercase();
        let mut row = None;
        {
            let mut m = self.model.borrow_mut();
            let mut visible = Vec::new();
            for (i, t) in m.tasks().iter().enumerate() {
                if self.filter == Filter::Active && t.done || self.filter == Filter::Done && !t.done {
                    continue;
                }
                if !needle.is_empty() && !t.title.to_lowercase().contains(&needle) {
                    continue;
                }
                if Some(i) == self.selected {
                    row = Some(visible.len());
                }
                visible.push(i);
            }
            m.visible = visible;
        }

        self.task_list.update(cx, |l, cx| match row {
            Some(r) => {
                l.set_selected_index(Some(IndexPath::default().row(r)), window, cx);
                l.scroll_to_item(IndexPath::default().row(r), ScrollStrategy::Nearest, window, cx);
            }
            None => l.set_selected_index(None, window, cx),
        });
    }

    fn projects_pane(&self, cx: &App) -> impl IntoElement {
        v_flex()
            .size_full()
            .gap_2()
            .p_2()
            .bg(cx.theme().sidebar)
            .child(Label::new("Projects").text_sm().text_color(cx.theme().muted_foreground))
            .child(div().flex_1().min_h_0().child(List::new(&self.project_list).scrollbar_visible(false)))
            .child(Input::new(&self.new_project))
    }

    fn tasks_pane(&self, cx: &mut Context<Self>) -> impl IntoElement {
        let mut filters = h_flex().gap_1();
        for f in [Filter::All, Filter::Active, Filter::Done] {
            filters = filters.child(
                Button::new(("filter", f as usize))
                    .small()
                    .ghost()
                    .selected(self.filter == f)
                    .label(f.name())
                    .on_click(cx.listener(move |this, _, window, cx| {
                        this.filter = f;
                        this.refilter(window, cx);
                        this.refresh(cx);
                    })),
            );
        }
        let body = if self.model.borrow().visible.is_empty() {
            v_flex()
                .size_full()
                .items_center()
                .justify_center()
                .child(Label::new("Nothing here").text_color(cx.theme().muted_foreground))
                .into_any_element()
        } else {
            List::new(&self.task_list).into_any_element()
        };
        v_flex()
            .size_full()
            .gap_2()
            .p_2()
            .key_context("Tasks")
            .child(
                h_flex()
                    .gap_2()
                    .items_center()
                    .child(filters)
                    .child(div().flex_1().child(Input::new(&self.search).prefix(Icon::new(IconName::Search))))
                    .child(
                        Button::new("add")
                            .primary()
                            .small()
                            .icon(IconName::Plus)
                            .label("New task")
                            .on_click(cx.listener(|this, _, window, cx| this.open_editor(None, window, cx))),
                    ),
            )
            .child(div().flex_1().min_h_0().child(body))
    }

    fn open_editor(&mut self, task: Option<usize>, window: &mut Window, cx: &mut Context<Self>) {
        self.editing = task;
        let t = match task {
            Some(i) => self.model.borrow().tasks()[i].clone(),
            None => Task { title: String::new(), done: false, due: String::new(), priority: Priority::Medium },
        };
        self.edit_title.update(cx, |s, cx| s.set_value(t.title.clone(), window, cx));
        self.edit_due.update(cx, |s, cx| s.set_value(t.due.clone(), window, cx));
        self.edit_prio = t.priority;
        let this = cx.weak_entity();
        let (title_s, due_s) = (self.edit_title.clone(), self.edit_due.clone());
        window.open_dialog(cx, move |d, _, cx| {
            let Some(app) = this.upgrade() else { return d };
            let (editing, prio) = {
                let a = app.read(cx);
                (a.editing, a.edit_prio)
            };
            let mut prios = h_flex().gap_1();
            for p in PRIORITIES {
                let this = this.clone();
                prios = prios.child(
                    Button::new(("prio", p as usize))
                        .small()
                        .outline()
                        .selected(prio == p)
                        .label(p.name())
                        .on_click(move |_, window, cx| {
                            this.update(cx, |a, _| a.edit_prio = p).ok();
                            window.refresh();
                        }),
                );
            }
            let this = this.clone();
            d.title(Label::new(if editing.is_none() { "New task" } else { "Edit task" }))
                .child(
                    v_flex()
                        .gap_3()
                        .child(Input::new(&title_s))
                        .child(Input::new(&due_s))
                        .child(h_flex().gap_2().items_center().child(Label::new("Priority")).child(prios)),
                )
                .footer(
                    DialogFooter::new()
                        .child(DialogClose::new().trigger(|b| b.label("Cancel")))
                        .child(DialogAction::new().child(Button::new("ok").primary().label("Save"))),
                )
                .entrance(DialogEntrance::None)
                .on_ok(move |_, window, cx| this.update(cx, |a, cx| a.save_edit(window, cx)).unwrap_or(true))
        });
        self.edit_title.update(cx, |s, cx| s.focus(window, cx));
    }

    fn save_edit(&mut self, window: &mut Window, cx: &mut Context<Self>) -> bool {
        let title = self.edit_title.read(cx).value().trim().to_string();
        if title.is_empty() {
            return false;
        }
        let due = self.edit_due.read(cx).value().trim().to_string();
        {
            let mut m = self.model.borrow_mut();
            let idx = match self.editing {
                Some(i) => i,
                None => {
                    m.tasks_mut().push(Task { title: String::new(), done: false, due: String::new(), priority: Priority::Low });
                    let i = m.tasks().len() - 1;
                    self.editing = Some(i);
                    self.selected = Some(i);
                    i
                }
            };
            let t = &mut m.tasks_mut()[idx];
            t.title = title;
            t.due = due;
            t.priority = self.edit_prio;
        }
        self.changed(window, cx);
        true
    }

    fn add_project(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let name = self.new_project.read(cx).value().trim().to_string();
        if name.is_empty() {
            return;
        }
        let current = {
            let mut m = self.model.borrow_mut();
            m.projects.push(Project { name, tasks: Vec::new() });
            m.current = m.projects.len() - 1;
            m.current
        };
        self.new_project.update(cx, |s, cx| s.set_value("", window, cx));
        self.selected = None;
        self.project_list.update(cx, |l, cx| l.set_selected_index(Some(IndexPath::default().row(current)), window, cx));
        self.changed(window, cx);
    }

    fn ask_delete(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.selected.is_none() {
            return;
        }
        let this = cx.weak_entity();
        window.open_alert_dialog(cx, move |a, _, cx| {
            let Some(app) = this.upgrade() else { return a };
            let name = {
                let app = app.read(cx);
                app.selected.map(|s| app.model.borrow().tasks()[s].title.clone()).unwrap_or_default()
            };
            let this = this.clone();
            a.title(Label::new(format!("Delete \"{name}\"?")))
                .description(Label::new("This cannot be undone."))
                .entrance(DialogEntrance::None)
                .show_cancel(true)
                .on_ok(move |_, window, cx| {
                    this.update(cx, |a, cx| {
                        if let Some(s) = a.selected {
                            a.model.borrow_mut().tasks_mut().remove(s);
                            a.selected = None;
                            a.changed(window, cx);
                        }
                    })
                    .ok();
                    true
                })
        });
    }

    fn switch_theme(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.dark = !self.dark;
        Theme::change(if self.dark { ThemeMode::Dark } else { ThemeMode::Light }, Some(window), cx);
        self.save();
        cx.notify();
    }

    fn start_sync(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let (tx, mut rx) = mpsc::unbounded::<(usize, String)>();
        std::thread::spawn(move || {
            for rev in 1.. {
                std::thread::sleep(std::time::Duration::from_secs(2));

                let remote = if rev == 2 { "Reply to the sync survey".to_string() } else { String::new() };
                if tx.unbounded_send((rev, remote)).is_err() {
                    return;
                }
            }
        });
        let wh = window.window_handle();
        cx.spawn(async move |this, cx| {
            while let Some((rev, remote)) = rx.next().await {
                let ok = cx.update_window(wh, |_, window, cx| {
                    this.update(cx, |a, cx| a.synced(rev, &remote, window, cx)).ok();
                });
                if ok.is_err() {
                    return;
                }
            }
        })
        .detach();
    }

    fn synced(&mut self, revision: usize, remote: &str, window: &mut Window, cx: &mut Context<Self>) {
        self.status = format!("synced revision {revision}, {} changes sent", self.dirty);
        self.dirty = 0;
        if !remote.is_empty() {
            self.model.borrow_mut().projects[0].tasks.push(Task {
                title: remote.into(),
                done: false,
                due: String::new(),
                priority: Priority::Low,
            });
            self.changed(window, cx);
        }
        self.refresh(cx);
    }

    fn file_menu(m: PopupMenu, nothing_selected: bool) -> PopupMenu {
        m.menu("New task", Box::new(NewTask))
            .menu("Find", Box::new(Find))
            .menu("Toggle theme", Box::new(ToggleTheme))
            .separator()
            .menu_with_disabled("Delete task…", Box::new(DeleteTask), nothing_selected)
    }
}

fn load(path: &str) -> (Vec<Project>, usize, bool) {
    if let Ok(data) = std::fs::read_to_string(path) {
        if let Ok(sv) = serde_json::from_str::<Saved>(&data) {
            if !sv.projects.is_empty() {
                let n = sv.projects.len() as i64;
                let current = sv.current.min(n - 1).max(0) as usize;
                return (sv.projects, current, sv.dark);
            }
        }
    }
    let task = |title: &str, done, due: &str, priority| Task { title: title.into(), done, due: due.into(), priority };
    let projects = vec![
        Project {
            name: "Inbox".into(),
            tasks: vec![
                task("Buy milk", false, "2026-11-13", Priority::Medium),
                task("Call the plumber", false, "2026-11-10", Priority::High),
                task("Return library books", true, "", Priority::Low),
            ],
        },
        Project {
            name: "Work".into(),
            tasks: vec![
                task("Write the quarterly report", false, "2026-11-28", Priority::High),
                task("Review Dana's pull request", false, "", Priority::Medium),
            ],
        },
        Project { name: "Home".into(), tasks: vec![task("Fix the shelf", false, "", Priority::Low)] },
    ];
    (projects, 0, false)
}

impl Render for TodoApp {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let (name, open) = {
            let m = self.model.borrow();
            (m.projects[m.current].name.clone(), open_count(&m.projects[m.current]))
        };
        let none = self.selected.is_none();
        let (icon, tip) = if self.dark { (IconName::Sun, "Light theme") } else { (IconName::Moon, "Dark theme") };
        let bar = h_flex()
            .gap_2()
            .items_center()
            .p_2()
            .border_b_1()
            .border_color(cx.theme().border)
            .child(Button::new("file").label("File").dropdown_menu(move |m, _, _| Self::file_menu(m, none)))
            .child(Label::new(format!("{name}: {open} open")))
            .child(div().flex_1())
            .child(Label::new(self.status.clone()).text_sm().text_color(cx.theme().muted_foreground))
            .child(
                Button::new("theme")
                    .ghost()
                    .icon(icon)
                    .tooltip(tip)
                    .on_click(cx.listener(|this, _, window, cx| this.switch_theme(window, cx))),
            );
        v_flex()
            .size_full()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .track_focus(&self.focus)
            .child(bar)
            .child(
                div().flex_1().w_full().min_h_0().child(
                    h_resizable("split")
                        .child(resizable_panel().size(px(220.)).child(self.projects_pane(cx)))
                        .child(resizable_panel().child(self.tasks_pane(cx))),
                ),
            )
    }
}

fn build(window: &mut Window, cx: &mut App) -> Entity<TodoApp> {
    gk_meter::attach(window, cx);
    cx.new(|cx| TodoApp::new(window, cx))
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--script") {
        #[cfg(feature = "diag")]
        {
            let script = std::fs::read_to_string(&args[1]).expect("script file");
            gk_meter::headless::run(&script, |cx| {
                let options = gk_meter::window_options(900., 600., cx);
                gpui_kit::open_window(options, cx, build).expect("open_window").0
            })
            .expect("headless run");
            return;
        }
        #[cfg(not(feature = "diag"))]
        {
            eprintln!("ref-todo: --script needs the diag build");
            std::process::exit(2);
        }
    }
    gpui_kit::application().with_assets(gpui_kit::assets::Assets).run(move |cx| {
        gpui_kit::init(cx);
        let options = gk_meter::window_options(900., 600., cx);
        gpui_kit::open_window(options, cx, build).expect("open_window");
        cx.activate(true);
    });
}
