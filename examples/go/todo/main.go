// todo: a small to-do app. Projects in a resizable sidebar, a
// task list per project with custom rows, add/edit/delete through a dialog, filters and search,
// keyboard shortcuts, a menu, light and dark themes, state saved to a JSON file, and a goroutine
// that pretends to sync and posts what it did.
//
//	todo-go                    a real window
//	TODO_FILE=path             where the state is kept (default todo.json in the current directory)
//	GK_SCRIPT=file todo-go     offscreen under a gk-meter script (gates/todo.sh)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/palette"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type Priority int

const (
	Low Priority = iota
	Medium
	High
)

var priorities = []Priority{Low, Medium, High}

var priorityNames = []string{"Low", "Medium", "High"}

func (p Priority) String() string { return priorityNames[p] }

// MarshalText and UnmarshalText keep the priority by name in the saved file.
func (p Priority) MarshalText() ([]byte, error) { return []byte(p.String()), nil }

func (p *Priority) UnmarshalText(b []byte) error {
	if i := slices.Index(priorityNames, string(b)); i >= 0 {
		*p = Priority(i)
		return nil
	}
	return fmt.Errorf("unknown priority %q", b)
}

var priorityColor = [...]gk.Color{Low: palette.Sky500, Medium: palette.Amber500, High: palette.Red500}

type Task struct {
	Title    string   `json:"title"`
	Done     bool     `json:"done"`
	Due      string   `json:"due"` // "2026-11-20" or ""
	Priority Priority `json:"priority"`
}

type Project struct {
	Name  string `json:"name"`
	Tasks []Task `json:"tasks"`
}

type Filter int

const (
	All Filter = iota
	Active
	Done
)

func (f Filter) String() string { return [...]string{"All", "Active", "Done"}[f] }

type App struct {
	projects []Project
	current  int // the project shown
	filter   Filter
	visible  []int // the current project's tasks that pass the filter and the search
	selected int   // a task of the current project, -1 none
	editing  int   // the task the dialog edits, -1 a new one
	editPrio Priority
	path     string
	dark     bool
	status   string
	dirty    int // changes not yet "synced"

	search, newProject, editTitle, editDue gk.InputState
	projectList, taskList                  gk.ListState
	editDialog                             gk.Built[gk.DialogEl]
	confirm                                gk.Built[gk.AlertDialogEl]

	newTask, deleteTask, find, toggleTheme gk.Action // shortcuts, also in the menu
}

func (a *App) tasks() *[]Task { return &a.projects[a.current].Tasks }

// --- state on disk ----------------------------------------------------------------------------

type saved struct {
	Projects []Project `json:"projects"`
	Current  int       `json:"current"`
	Dark     bool      `json:"dark"`
}

func (a *App) load() {
	var sv saved
	if data, err := os.ReadFile(a.path); err == nil && json.Unmarshal(data, &sv) == nil && len(sv.Projects) > 0 {
		a.projects, a.current, a.dark = sv.Projects, max(0, min(sv.Current, len(sv.Projects)-1)), sv.Dark
		return
	}
	a.projects = []Project{
		{"Inbox", []Task{{"Buy milk", false, "2026-11-13", Medium}, {"Call the plumber", false, "2026-11-10", High},
			{"Return library books", true, "", Low}}},
		{"Work", []Task{{"Write the quarterly report", false, "2026-11-28", High}, {"Review Dana's pull request", false, "", Medium}}},
		{"Home", []Task{{"Fix the shelf", false, "", Low}}},
	}
}

func (a *App) save() {
	if data, err := json.MarshalIndent(saved{a.projects, a.current, a.dark}, "", "  "); err == nil {
		_ = os.WriteFile(a.path, data, 0o644)
	}
}

// changed is called after every edit of the tasks or projects. gk.Refresh redraws every view
// and every row, and the lists take their counts from their count functions.
func (a *App) changed(cx *gk.Cx) {
	a.dirty++
	a.save()
	a.refilter(cx)
	gk.Refresh(cx)
}

func (a *App) refilter(cx *gk.Cx) {
	a.visible = a.visible[:0]
	needle := strings.ToLower(a.search.Value(cx))
	row := -1
	for i, t := range *a.tasks() {
		if a.filter == Active && t.Done || a.filter == Done && !t.Done {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(t.Title), needle) {
			continue
		}
		if i == a.selected {
			row = len(a.visible)
		}
		a.visible = append(a.visible, i)
	}
	gk.SelectRow(cx, a.taskList, row, gk.ScrollIntoView) // a task added at the end is shown
}

// --- init -------------------------------------------------------------------------------------

func (a *App) init(cx *gk.Cx) {
	a.selected = -1
	a.load()
	if a.dark {
		gk.ThemeChange(cx, gk.ThemeModeDark)
	}
	a.newTask = gk.Shortcut(cx, "ctrl-n", a.addTask)
	a.deleteTask = gk.Shortcut(cx, "delete", a.askDelete, gk.WhenIn("Tasks"))
	a.find = gk.Shortcut(cx, "ctrl-f", func(cx *gk.Cx) { a.search.Focus(cx) })
	a.toggleTheme = gk.Shortcut(cx, "ctrl-t", a.switchTheme)

	a.search = gk.NewInputState(cx).Placeholder("Search tasks").Build()
	a.search.OnInputEvent(cx, a.searchChanged)
	a.newProject = gk.NewInputState(cx).Placeholder("New project").Build()
	a.newProject.OnInputEvent(cx, a.newProjectEvent)
	a.editTitle = gk.NewInputState(cx).Placeholder("What needs doing?").Build()
	a.editDue = gk.NewInputState(cx).Placeholder("Due date, e.g. 2026-11-20").Build()
	a.editDialog = gk.NewBuilt(cx, a.editView)
	a.confirm = gk.NewBuilt(cx, a.confirmView)

	a.projectList = gk.NewListState(cx, gk.NewListSource(cx, func() int { return len(a.projects) }, a.projectRow)).Build()
	a.projectList.OnListEvent(cx, a.projectEvent)
	gk.SelectRow(cx, a.projectList, a.current)
	a.taskList = gk.NewListState(cx, gk.NewListSource(cx, func() int { return len(a.visible) }, a.taskRow)).Build()
	a.taskList.OnListEvent(cx, a.taskEvent)

	a.status = "not synced yet"
	a.refilter(cx)
	gk.Refresh(cx) // the task list's first count
	go a.syncLoop()
}

// --- the views --------------------------------------------------------------------------------

func (a *App) view(cx *gk.Cx) gk.Element {
	open := 0
	for _, t := range *a.tasks() {
		if !t.Done {
			open++
		}
	}
	fileMenu := gk.NewPopupMenu().
		Menu("New task", a.newTask).
		Menu("Find", a.find).
		Menu("Toggle theme", a.toggleTheme).
		Separator().
		MenuWithDisabled("Delete task…", a.deleteTask, a.selected < 0)
	icon, tip := gk.IconMoon, "Dark theme"
	if a.dark {
		icon, tip = gk.IconSun, "Light theme"
	}
	bar := gk.HFlex(s.Gap2, s.ItemsCenter, s.P2, s.BorderB1).BorderColor(theme.Border).
		Child(gk.DropdownMenu(gk.Button("file").Label("File"), fileMenu)).
		Child(gk.Label(fmt.Sprintf("%s: %d open", a.projects[a.current].Name, open))).
		Child(gk.Div(s.Flex1)).
		Child(gk.Label(a.status, s.TextSm).TextColor(theme.MutedForeground)).
		Child(gk.Button("theme").Ghost().Icon(icon).Tooltip(tip).OnClick(a.switchTheme))
	return gk.VFlex(s.SizeFull).Bg(theme.Background).TextColor(theme.Foreground).
		Child(bar).
		Child(gk.Div(s.Flex1, s.WFull, s.MinH0).Child(gk.HResizable("split").
			Child(gk.ResizablePanel().Size(220).Child(a.projectsPane())).
			Child(gk.ResizablePanel().Child(a.tasksPane()))))
}

func (a *App) projectsPane() gk.Element {
	return gk.VFlex(s.SizeFull, s.Gap2, s.P2).Bg(theme.Sidebar).
		Child(gk.Label("Projects", s.TextSm).TextColor(theme.MutedForeground)).
		Child(gk.Div(s.Flex1, s.MinH0).Child(gk.List(a.projectList).ScrollbarVisible(false))).
		Child(gk.Input(a.newProject))
}

func (a *App) projectRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	p := a.projects[at.Row]
	open := 0
	for _, t := range p.Tasks {
		if !t.Done {
			open++
		}
	}
	return gk.ListItem(gk.Key("project", at.Row)).
		Child(gk.HFlex(s.WFull, s.JustifyBetween).
			Child(gk.Label(p.Name)).
			Child(gk.Label(fmt.Sprint(open), s.TextSm).TextColor(theme.MutedForeground)))
}

func (a *App) tasksPane() gk.Element {
	filters := gk.HFlex(s.Gap1)
	for _, f := range []Filter{All, Active, Done} {
		filters = filters.Child(gk.Button(gk.Key("filter", int(f))).Small().Ghost().
			Selected(a.filter == f).Label(f.String()).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
			a.filter = f
			a.refilter(cx)
			gk.Refresh(cx)
		}))
	}
	var body gk.Element = gk.List(a.taskList)
	if len(a.visible) == 0 {
		body = gk.VFlex(s.SizeFull, s.ItemsCenter, s.JustifyCenter).
			Child(gk.Label("Nothing here").TextColor(theme.MutedForeground))
	}
	return gk.VFlex(s.SizeFull, s.Gap2, s.P2).KeyContext("Tasks").
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(filters).
			Child(gk.Div(s.Flex1).Child(gk.Input(a.search).Prefix(gk.Icon(gk.IconSearch)))).
			Child(gk.Button("add").Primary().Small().Icon(gk.IconPlus).Label("New task").OnClick(a.addTask))).
		Child(gk.Div(s.Flex1, s.MinH0).Child(body))
}

// taskRow is one task: a checkbox, the priority's colour, the title (a double click edits it)
// and the due date as a tag, red when overdue and amber when due today.
func (a *App) taskRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	i := a.visible[at.Row]
	t := (*a.tasks())[i]
	title := gk.Label(t.Title)
	if t.Done {
		title = title.Style(s.LineThrough).TextColor(theme.MutedForeground)
	}
	row := gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(gk.Checkbox(gk.Key("done", i)).Checked(t.Done).OnClick(func(cx *gk.Cx, done bool) {
			gk.StopPropagation(cx) // the row is not selected by the click
			(*a.tasks())[i].Done = done
			a.changed(cx)
		})).
		Child(gk.Div(s.RoundedFull).W(gk.Px(8)).H(gk.Px(8)).Bg(priorityColor[t.Priority])).
		Child(gk.Div(s.Flex1).ID(gk.Key("title", i)).Child(title).OnClick(func(cx *gk.Cx, e gk.ClickEvent) {
			if e.ClickCount == 2 {
				a.openEditor(cx, i)
			}
		}))
	if t.Due != "" {
		today := time.Now().Format(time.DateOnly)
		tag := gk.TagSecondary(s.TextXs)
		if !t.Done && t.Due < today {
			tag = gk.TagDanger(s.TextXs)
		} else if t.Due == today {
			tag = gk.TagWarning(s.TextXs)
		}
		row = row.Child(tag.Text(t.Due))
	}
	return gk.ListItem(gk.Key("task", i)).Child(row)
}

// --- the edit dialog --------------------------------------------------------------------------

func (a *App) editView(cx *gk.Cx) gk.DialogEl {
	prios := gk.HFlex(s.Gap1)
	for _, p := range priorities {
		prios = prios.Child(gk.Button(gk.Key("prio", int(p))).Small().Outline().
			Selected(a.editPrio == p).Label(p.String()).OnClick(func(*gk.Cx, gk.ClickEvent) { a.editPrio = p }))
	}
	title := "Edit task"
	if a.editing < 0 {
		title = "New task"
	}
	return gk.Dialog().
		Title(gk.Label(title)).
		Child(gk.VFlex(s.Gap3).
			Child(gk.Input(a.editTitle)).
			Child(gk.Input(a.editDue)).
			Child(gk.HFlex(s.Gap2, s.ItemsCenter).Child(gk.Label("Priority")).Child(prios))).
		Footer(gk.DialogFooter().
			Child(gk.DialogClose().Trigger(gk.Button("cancel").Label("Cancel"))).
			Child(gk.DialogAction().Child(gk.Button("ok").Primary().Label("Save")))).
		Entrance(gk.DialogEntranceNone).
		OnOk(a.saveEdit)
}

func (a *App) openEditor(cx *gk.Cx, task int) {
	a.editing = task
	t := Task{Priority: Medium}
	if task >= 0 {
		t = (*a.tasks())[task]
	}
	a.editTitle.SetValue(cx, t.Title)
	a.editDue.SetValue(cx, t.Due)
	a.editPrio = t.Priority
	gk.OpenDialog(cx, a.editDialog)
	a.editTitle.Focus(cx)
}

func (a *App) saveEdit(cx *gk.Cx, _ gk.ClickEvent) bool {
	title := strings.TrimSpace(a.editTitle.Value(cx))
	if title == "" {
		return false // keep the dialog open
	}
	if a.editing < 0 {
		*a.tasks() = append(*a.tasks(), Task{})
		a.editing = len(*a.tasks()) - 1
		a.selected = a.editing
	}
	t := &(*a.tasks())[a.editing]
	t.Title, t.Due, t.Priority = title, strings.TrimSpace(a.editDue.Value(cx)), a.editPrio
	a.changed(cx)
	return true
}

// --- what the app does ------------------------------------------------------------------------

func (a *App) addTask(cx *gk.Cx, _ gk.ClickEvent) { a.openEditor(cx, -1) }

func (a *App) searchChanged(cx *gk.Cx, e gk.InputEvent) {
	if _, ok := e.(gk.InputEventChange); ok {
		a.refilter(cx)
		gk.Refresh(cx)
	}
}

func (a *App) projectEvent(cx *gk.Cx, e gk.ListEvent) {
	if c, ok := e.(gk.ListEventConfirm); ok && c.Row != a.current {
		a.current, a.selected = c.Row, -1
		a.refilter(cx)
		gk.Refresh(cx)
	}
}

func (a *App) taskEvent(cx *gk.Cx, e gk.ListEvent) {
	switch e := e.(type) {
	case gk.ListEventSelect:
		a.selected = a.visible[e.Row]
	case gk.ListEventConfirm:
		a.selected = a.visible[e.Row] // a click; Enter as well
	}
}

func (a *App) newProjectEvent(cx *gk.Cx, e gk.InputEvent) {
	name := strings.TrimSpace(a.newProject.Value(cx))
	if _, enter := e.(gk.InputEventPressEnter); !enter || name == "" {
		return
	}
	a.projects = append(a.projects, Project{Name: name})
	a.newProject.SetValue(cx, "")
	a.current, a.selected = len(a.projects)-1, -1
	gk.SelectRow(cx, a.projectList, a.current)
	a.changed(cx)
}

func (a *App) askDelete(cx *gk.Cx) {
	if a.selected >= 0 {
		gk.OpenAlertDialog(cx, a.confirm)
	}
}

func (a *App) confirmView(cx *gk.Cx) gk.AlertDialogEl {
	name := ""
	if a.selected >= 0 {
		name = (*a.tasks())[a.selected].Title
	}
	return gk.AlertDialog().
		Title(gk.Label(fmt.Sprintf("Delete \"%s\"?", name))).
		Description(gk.Label("This cannot be undone.")).
		Entrance(gk.DialogEntranceNone).
		ShowCancel(true).
		OnOk(func(cx *gk.Cx, _ gk.ClickEvent) bool {
			if a.selected >= 0 {
				*a.tasks() = slices.Delete(*a.tasks(), a.selected, a.selected+1)
				a.selected = -1
				a.changed(cx)
			}
			return true
		})
}

func (a *App) switchTheme(cx *gk.Cx, _ gk.ClickEvent) {
	a.dark = !a.dark
	mode := gk.ThemeModeLight
	if a.dark {
		mode = gk.ThemeModeDark
	}
	gk.ThemeChange(cx, mode)
	a.save()
}

// --- the pretend sync: a goroutine that posts to the UI thread --------------------------------

func (a *App) syncLoop() {
	for rev := 1; ; rev++ {
		time.Sleep(2 * time.Second)
		remote := ""
		if rev == 2 {
			remote = "Reply to the sync survey" // a task another device added
		}
		gk.Post(func(cx *gk.Cx) { a.synced(cx, rev, remote) })
	}
}

func (a *App) synced(cx *gk.Cx, revision int, remote string) {
	a.status = fmt.Sprintf("synced revision %d, %d changes sent", revision, a.dirty)
	a.dirty = 0
	if remote != "" {
		a.projects[0].Tasks = append(a.projects[0].Tasks, Task{Title: remote})
		a.changed(cx)
	}
	gk.Refresh(cx)
}

func main() {
	a := &App{path: "todo.json"}
	if p, ok := os.LookupEnv("TODO_FILE"); ok {
		a.path = p
	}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Todo", 900, 600, gk.NewView(a.init, a.view))
	})
}
