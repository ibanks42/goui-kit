// todo: a small to-do app. Projects in a resizable sidebar, a task list per project with
// custom rows, add/edit/delete through a dialog, filters and search, keyboard shortcuts, a
// menu, light and dark themes, state saved to a JSON file, and a background thread that
// pretends to sync and posts what it did.
//
//   examples/odin/build.sh todo && bin/todo-odin            a real window
//   TODO_FILE=path   where the state is kept (default todo.json in the current directory)
//   GK_SCRIPT=file   offscreen under a gk-meter script (the diag library; gates/todo.sh)
package todo

import "core:encoding/json"
import "core:fmt"
import "core:os"
import "core:strings"
import "core:thread"
import "core:time"
import gk "gpuikit:gk"

Priority :: enum {
	Low,
	Medium,
	High,
}

Task :: struct {
	title:    string,
	done:     bool,
	due:      string, // "2026-11-20" or ""
	priority: Priority,
}

Project :: struct {
	name:  string,
	tasks: [dynamic]Task,
}

Filter :: enum {
	All,
	Active,
	Done,
}

App :: struct {
	projects:     [dynamic]Project,
	current:      int, // the project shown
	filter:       Filter,
	visible:      [dynamic]int, // the current project's tasks that pass the filter and the search
	selected:     int, // a task of the current project, -1 none
	editing:      int, // the task the dialog edits, -1 a new one
	edit_prio:    Priority,
	path:         string,
	dark:         bool,
	status:       string,
	dirty:        int, // changes not yet "synced"
	// kit state
	search:       gk.Input_State,
	new_project:  gk.Input_State,
	edit_title:   gk.Input_State,
	edit_due:     gk.Input_State,
	project_list: gk.List_State,
	task_list:    gk.List_State,
	// shortcuts, also in the menu
	new_task, delete_task, find, toggle_theme: gk.Action,
}

tasks :: proc(a: ^App) -> ^[dynamic]Task {
	return &a.projects[a.current].tasks
}

// --- state on disk ---------------------------------------------------------------------------

Saved :: struct {
	projects: []Project,
	current:  int,
	dark:     bool,
}

load :: proc(a: ^App) {
	s: Saved
	data, err := os.read_entire_file(a.path, context.allocator)
	if err == nil && json.unmarshal(data, &s) == nil && len(s.projects) > 0 {
		append(&a.projects, ..s.projects)
		a.current = clamp(s.current, 0, len(a.projects) - 1)
		a.dark = s.dark
		return
	}
	inbox := Project{name = "Inbox"}
	append(&inbox.tasks, Task{"Buy milk", false, "2026-11-13", .Medium}, Task{"Call the plumber", false, "2026-11-10", .High},
		Task{"Return library books", true, "", .Low})
	work := Project{name = "Work"}
	append(&work.tasks, Task{"Write the quarterly report", false, "2026-11-28", .High}, Task{"Review Dana's pull request", false, "", .Medium})
	home := Project{name = "Home"}
	append(&home.tasks, Task{"Fix the shelf", false, "", .Low})
	append(&a.projects, inbox, work, home)
}

save :: proc(a: ^App) {
	data, err := json.marshal(Saved{a.projects[:], a.current, a.dark}, {pretty = true, use_enum_names = true})
	if err == nil {
		_ = os.write_entire_file(a.path, data)
		delete(data)
	}
}

// changed: after every edit of the tasks or projects. gk.refresh redraws every view and every
// row, and the lists take their counts from task_count and project_count.
changed :: proc(a: ^App, cx: ^gk.Cx) {
	a.dirty += 1
	save(a)
	refilter(a, cx)
	gk.refresh(cx)
}

refilter :: proc(a: ^App, cx: ^gk.Cx) {
	clear(&a.visible)
	needle := strings.to_lower(string(gk.value(a.search, cx)), context.temp_allocator)
	row := -1
	for t, i in tasks(a)^ {
		if a.filter == .Active && t.done || a.filter == .Done && !t.done {
			continue
		}
		if needle != "" && !strings.contains(strings.to_lower(t.title, context.temp_allocator), needle) {
			continue
		}
		if i == a.selected {
			row = len(a.visible)
		}
		append(&a.visible, i)
	}
	gk.select_row(cx, a.task_list, row, scroll = true) // a task added at the end is shown
}

task_count :: proc(a: ^App) -> int {return len(a.visible)}
project_count :: proc(a: ^App) -> int {return len(a.projects)}

// --- init ------------------------------------------------------------------------------------

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.selected = -1
	load(a)
	if a.dark {
		gk.theme_change(cx, gk.Theme_Mode.Dark)
	}
	a.new_task = gk.shortcut(cx, "ctrl-n", gk.on(new_task))
	a.delete_task = gk.shortcut(cx, "delete", gk.on(ask_delete), when_in = "Tasks")
	a.find = gk.shortcut(cx, "ctrl-f", gk.on(find))
	a.toggle_theme = gk.shortcut(cx, "ctrl-t", gk.on(toggle_theme))

	a.search = gk.input_state(cx)->placeholder("Search tasks")->build()
	gk.on_input_event(a.search, cx, gk.on(search_changed))
	a.new_project = gk.input_state(cx)->placeholder("New project")->build()
	gk.on_input_event(a.new_project, cx, gk.on(new_project_event))
	a.edit_title = gk.input_state(cx)->placeholder("What needs doing?")->build()
	a.edit_due = gk.input_state(cx)->placeholder("Due date, e.g. 2026-11-20")->build()

	a.project_list = gk.list_state(cx, gk.list_source(cx, a, project_count, project_row))->build()
	gk.on_list_event(a.project_list, cx, gk.on(project_event))
	gk.select_row(cx, a.project_list, a.current)
	a.task_list = gk.list_state(cx, gk.list_source(cx, a, task_count, task_row))->build()
	gk.on_list_event(a.task_list, cx, gk.on(task_event))

	a.status = strings.clone("not synced yet")
	refilter(a, cx)
	gk.refresh(cx) // the task list's first count, from task_count
	thread.create_and_start(sync_loop)
}

// --- the views -------------------------------------------------------------------------------

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	open := 0
	for t in tasks(a)^ {
		open += int(!t.done)
	}
	file_menu := gk.popup_menu()->
		menu("New task", a.new_task)->
		menu("Find", a.find)->
		menu("Toggle theme", a.toggle_theme)->
		separator()->
		menu_with_disabled("Delete task…", a.delete_task, a.selected < 0)
	bar := gk.h_flex(.Gap_2, .Items_Center, .P_2, .Border_B_1)->border_color(gk.theme(.Border))->
		child(gk.dropdown_menu(gk.button("file")->label("File"), file_menu))->
		child(gk.label(fmt.tprintf("%s: %d open", a.projects[a.current].name, open)))->
		child(gk.div(.Flex_1))->
		child(gk.label(a.status, .Text_Sm)->text_color(gk.theme(.Muted_Foreground)))->
		child(gk.button("theme")->ghost()->icon(.Sun if a.dark else .Moon)->tooltip("Light theme" if a.dark else "Dark theme")->on_click(gk.on(toggle_theme)))
	return gk.v_flex(.Size_Full)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(bar)->
		child(gk.div(.Flex_1, .W_Full, .Min_H_0)->child(gk.h_resizable("split")->
			child(gk.resizable_panel()->size(220)->child(projects_pane(a)))->
			child(gk.resizable_panel()->child(tasks_pane(a)))))
}

projects_pane :: proc(a: ^App) -> gk.Element {
	return gk.v_flex(.Size_Full, .Gap_2, .P_2)->bg(gk.theme(.Sidebar))->
		child(gk.label("Projects", .Text_Sm)->text_color(gk.theme(.Muted_Foreground)))->
		child(gk.div(.Flex_1, .Min_H_0)->child(gk.list(a.project_list)->scrollbar_visible(false)))->
		child(gk.input(a.new_project))
}

project_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	p := a.projects[at.row]
	open := 0
	for t in p.tasks {
		open += int(!t.done)
	}
	return gk.list_item(gk.id("project", at.row))->
		child(gk.h_flex(.W_Full, .Justify_Between)->
			child(gk.label(p.name))->
			child(gk.label(fmt.tprintf("%d", open), .Text_Sm)->text_color(gk.theme(.Muted_Foreground))))
}

tasks_pane :: proc(a: ^App) -> gk.Element {
	filters := gk.h_flex(.Gap_1)
	for f in Filter {
		filters = filters->child(gk.button(gk.id("filter", int(f)))->small()->ghost()->
			selected(a.filter == f)->label(fmt.tprintf("%v", f))->on_click(gk.on(set_filter, f)))
	}
	body: gk.Element = gk.list(a.task_list)
	if len(a.visible) == 0 {
		body = gk.v_flex(.Size_Full, .Items_Center, .Justify_Center)->
			child(gk.label("Nothing here")->text_color(gk.theme(.Muted_Foreground)))
	}
	return gk.v_flex(.Size_Full, .Gap_2, .P_2)->key_context("Tasks")->
		child(gk.h_flex(.Gap_2, .Items_Center)->
			child(filters)->
			child(gk.div(.Flex_1)->child(gk.input(a.search)->prefix(gk.icon(.Search))))->
			child(gk.button("add")->primary()->small()->icon(.Plus)->label("New task")->on_click(gk.on(new_task))))->
		child(gk.div(.Flex_1, .Min_H_0)->child(body))
}

PRIORITY_COLOR := [Priority]gk.Palette{.Low = .Sky_500, .Medium = .Amber_500, .High = .Red_500}

// task_row is one task: a checkbox, the priority's colour, the title (a double click edits it)
// and the due date as a tag, red when overdue and amber when due today.
task_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	i := a.visible[at.row]
	t := tasks(a)[i]
	title := gk.label(t.title)
	if t.done {
		title = title->style(.Line_Through)->text_color(gk.theme(.Muted_Foreground))
	}
	row := gk.h_flex(.W_Full, .Gap_2, .Items_Center)->
		child(gk.checkbox(gk.id("done", i))->checked(t.done)->on_click(gk.on(toggle_done, i)))->
		child(gk.div(.Rounded_Full)->w(gk.px(8))->h(gk.px(8))->bg(gk.palette(PRIORITY_COLOR[t.priority])))->
		child(gk.div(.Flex_1)->id(gk.id("title", i))->child(title)->on_click(gk.on(title_clicked, i)))
	if t.due != "" {
		today := today_string()
		tag := gk.tag_secondary(.Text_Xs)
		if !t.done && t.due < today {
			tag = gk.tag_danger(.Text_Xs)
		} else if t.due == today {
			tag = gk.tag_warning(.Text_Xs)
		}
		row = row->child(tag->text(t.due))
	}
	return gk.list_item(gk.id("task", i))->child(row)
}

today_string :: proc() -> string {
	y, m, d := time.date(time.now())
	return fmt.tprintf("%04d-%02d-%02d", y, int(m), d)
}

// --- the edit dialog -------------------------------------------------------------------------

edit_dialog :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	prios := gk.h_flex(.Gap_1)
	for p in Priority {
		prios = prios->child(gk.button(gk.id("prio", int(p)))->small()->outline()->
			selected(a.edit_prio == p)->label(fmt.tprintf("%v", p))->on_click(gk.on(set_prio, p)))
	}
	return gk.dialog()->
		title(gk.label("New task" if a.editing < 0 else "Edit task"))->
		child(gk.v_flex(.Gap_3)->
			child(gk.input(a.edit_title))->
			child(gk.input(a.edit_due))->
			child(gk.h_flex(.Gap_2, .Items_Center)->child(gk.label("Priority"))->child(prios)))->
		footer(gk.dialog_footer()->
			child(gk.dialog_close()->trigger(gk.button("cancel")->label("Cancel")))->
			child(gk.dialog_action()->child(gk.button("ok")->primary()->label("Save"))))->
		entrance(.None)->
		on_ok(gk.on(save_edit))
}

open_editor :: proc(a: ^App, cx: ^gk.Cx, task: int) {
	a.editing = task
	t := tasks(a)[task] if task >= 0 else Task{priority = .Medium}
	gk.set_value(a.edit_title, cx, t.title)
	gk.set_value(a.edit_due, cx, t.due)
	a.edit_prio = t.priority
	gk.open_dialog(cx, gk.built(cx, a, edit_dialog))
	gk.input_state_focus(a.edit_title, cx)
}

set_prio :: proc(a: ^App, cx: ^gk.Cx, p: Priority) {
	a.edit_prio = p
}

save_edit :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	title := strings.trim_space(string(gk.value(a.edit_title, cx)))
	if title == "" {
		return false // keep the dialog open
	}
	due := strings.trim_space(string(gk.value(a.edit_due, cx)))
	if a.editing < 0 {
		append(tasks(a), Task{priority = a.edit_prio})
		a.editing = len(tasks(a)) - 1
		a.selected = a.editing
	}
	t := &tasks(a)[a.editing]
	t.title = strings.clone(title)
	t.due = strings.clone(due)
	t.priority = a.edit_prio
	changed(a, cx)
	return true
}

// --- what the app does -----------------------------------------------------------------------

new_task :: proc(a: ^App, cx: ^gk.Cx) {
	open_editor(a, cx, -1)
}

find :: proc(a: ^App, cx: ^gk.Cx) {
	gk.input_state_focus(a.search, cx)
}

search_changed :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	if _, ok := e.(gk.Input_Event_Change); ok {
		refilter(a, cx)
		gk.refresh(cx)
	}
}

set_filter :: proc(a: ^App, cx: ^gk.Cx, f: Filter) {
	a.filter = f
	refilter(a, cx)
	gk.refresh(cx)
}

toggle_done :: proc(a: ^App, cx: ^gk.Cx, task: int) {
	gk.stop_propagation(cx) // the row is not selected by the click
	tasks(a)[task].done = !tasks(a)[task].done
	changed(a, cx)
}

title_clicked :: proc(a: ^App, cx: ^gk.Cx, e: gk.Click_Event, task: int) {
	if e.click_count == 2 {
		open_editor(a, cx, task)
	}
}

project_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Event) {
	if c, ok := e.(gk.List_Event_Confirm); ok && c.row != a.current {
		a.current = c.row
		a.selected = -1
		refilter(a, cx)
		gk.refresh(cx)
	}
}

task_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Event) {
	#partial switch v in e {
	case gk.List_Event_Select:
		a.selected = a.visible[v.row]
	case gk.List_Event_Confirm:
		a.selected = a.visible[v.row] // a click; Enter as well
	}
}

new_project_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	name := strings.trim_space(string(gk.value(a.new_project, cx)))
	if _, enter := e.(gk.Input_Event_Press_Enter); !enter || name == "" {
		return
	}
	append(&a.projects, Project{name = strings.clone(name)})
	gk.set_value(a.new_project, cx, "")
	a.current = len(a.projects) - 1
	a.selected = -1
	gk.select_row(cx, a.project_list, a.current)
	changed(a, cx)
}

ask_delete :: proc(a: ^App, cx: ^gk.Cx) {
	if a.selected >= 0 {
		gk.open_alert_dialog(cx, gk.built(cx, a, confirm_dialog))
	}
}

confirm_dialog :: proc(a: ^App, cx: ^gk.Cx) -> gk.Alert_Dialog {
	name := tasks(a)[a.selected].title if a.selected >= 0 else ""
	return gk.alert_dialog()->
		title(gk.label(fmt.tprintf("Delete \"%s\"?", name)))->
		description(gk.label("This cannot be undone."))->
		entrance(.None)->
		show_cancel(true)->
		on_ok(gk.on(confirm_delete))
}

confirm_delete :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	if a.selected >= 0 {
		ordered_remove(tasks(a), a.selected)
		a.selected = -1
		changed(a, cx)
	}
	return true
}

toggle_theme :: proc(a: ^App, cx: ^gk.Cx) {
	a.dark = !a.dark
	gk.theme_change(cx, gk.Theme_Mode.Dark if a.dark else gk.Theme_Mode.Light)
	save(a)
}

// --- the pretend sync: a thread that posts to the UI thread ------------------------------------

Synced :: struct {
	revision: int,
	remote:   string, // a task another device added, or ""
}

sync_loop :: proc() {
	for rev := 1;; rev += 1 {
		time.sleep(2 * time.Second)
		gk.post(synced, Synced{rev, "Reply to the sync survey" if rev == 2 else ""})
	}
}

synced :: proc(a: ^App, cx: ^gk.Cx, m: Synced) {
	delete(a.status)
	a.status = fmt.aprintf("synced revision %d, %d changes sent", m.revision, a.dirty)
	a.dirty = 0
	if m.remote != "" {
		append(&a.projects[0].tasks, Task{title = m.remote})
		changed(a, cx)
	}
	gk.refresh(cx)
}

// --- start -----------------------------------------------------------------------------------

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Todo", {900, 600}, gk.view(a, init, root_view))
}

main :: proc() {
	app: App
	app.path = os.lookup_env("TODO_FILE", context.allocator) or_else "todo.json"
	gk.run(&app, start)
}
