// notes: a small notes app, written the way an app developer writes one, to show that the
// pieces compose: the kit's styled dock with two Odin panels (a list of notes over a row source,
// an editor with the kit's Input), a File menu whose items are actions with keyboard shortcuts,
// a confirmation dialog, and the theme switched between light and dark. The same app in Rust is
// ref/src/bin/notes.rs; gates/pixels.sh notes runs both under gates/notes.script and compares
// every image byte for byte.
//
//   notes-odin                    a real window
//   GK_SCRIPT=file notes-odin     offscreen under a gk-meter script (the diag library)
package notes

import "core:fmt"
import "core:strings"
import gk "gpuikit:gk"

Note :: struct {
	title: string, // owned (heap)
	words: int,
}

App :: struct {
	notes:    [dynamic]Note,
	selected: int, // -1: none
	made:     int, // notes made so far, for "Untitled N"
	// kit state that lives in the library
	title:    gk.Input_State,
	list:     gk.List_State,
	dock:     gk.Dock_Area,
	// shortcuts, also in the menu (the menu shows their keys)
	new_note, delete_note, toggle_theme: gk.Action,
	dark:     bool,
}

STARTING := [?]Note{{"Groceries", 4}, {"Ideas for the weekend", 12}, {"Meeting notes", 31}}

init :: proc(a: ^App, cx: ^gk.Cx) {
	for n in STARTING {
		append(&a.notes, Note{strings.clone(n.title), n.words})
	}
	a.made = len(a.notes)

	a.new_note = gk.shortcut(cx, "ctrl-n", gk.on(new_note))
	a.delete_note = gk.shortcut(cx, "", gk.on(ask_delete)) // the menu's item only
	a.toggle_theme = gk.shortcut(cx, "ctrl-t", gk.on(toggle_theme))

	a.title = gk.input_state(cx)->placeholder("Title")->build()
	gk.on_input_event(a.title, cx, gk.on(title_changed))
	a.list = gk.list_state(cx, gk.list_source(cx, a, note_count, note_row))->build()
	gk.on_list_event(a.list, cx, gk.on(list_event))

	a.dock = gk.dock_area_skinned(cx, "notes")
	notes := gk.panel(cx, gk.view(a, nil, notes_view), {name = "notes", title = "Notes"})
	editor := gk.panel(cx, gk.view(a, nil, editor_view), {name = "editor", title = "Editor"})
	// panel_view: the kit's PanelHandle over the panel, so the tabs show the titles
	gk.set_dock(a.dock, cx, .Left, gk.dock_layout_tabs()->panel_view(notes))
	gk.set_center(a.dock, cx, gk.dock_layout_tabs()->panel_view(editor))
	select(a, cx, 0) // the first note
}

note_count :: proc(a: ^App) -> int {return len(a.notes)}

// --- the views ------------------------------------------------------------------------------------

file_menu :: proc(a: ^App) -> gk.Popup_Menu {
	return gk.popup_menu()->
		menu("New note", a.new_note)->
		menu("Toggle theme", a.toggle_theme)->
		separator()->
		menu_with_disabled("Delete note…", a.delete_note, a.selected < 0)
}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	bar := gk.h_flex(.Gap_2, .Items_Center, .P_2, .Border_B_1)->border_color(gk.theme(.Border))->
		child(gk.dropdown_menu(gk.button("file")->label("File"), file_menu(a)))->
		child(gk.label(fmt.tprintf("%d notes", len(a.notes))))->
		child(gk.div(.Flex_1))->
		child(gk.button("theme")->label("Light theme" if a.dark else "Dark theme")->on_click(gk.on(toggle_theme)))
	return gk.v_flex(.Size_Full)->
		bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(bar)->
		child(gk.div(.Flex_1, .W_Full, .Min_H_0)->child(gk.dock_view(a.dock)))
}

notes_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Size_Full)->child(gk.list(a.list)->scrollbar_visible(false))
}

note_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	n := a.notes[at.row]
	return gk.list_item(gk.id("note", at.row))->
		child(gk.v_flex()->
			child(gk.label(n.title))->
			child(gk.label(fmt.tprintf("%d words", n.words), .Text_Sm)->text_color(gk.theme(.Muted_Foreground))))
}

editor_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if a.selected < 0 {
		return gk.v_flex(.Size_Full, .P_4, .Items_Center)->child(gk.label("No note selected"))
	}
	n := a.notes[a.selected]
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->
		child(gk.label("Title"))->
		child(gk.input(a.title))->
		child(gk.label(fmt.tprintf("%d words", n.words)))->
		child(gk.h_flex(.Gap_2)->
			child(gk.button("delete")->danger()->label("Delete")->on_click(gk.on(ask_delete))))
}

// --- what the app does ----------------------------------------------------------------------------

select :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	a.selected = row
	if row >= 0 {
		gk.set_value(a.title, cx, a.notes[row].title)
	}
	gk.select_row(cx, a.list, row)
	gk.refresh(cx)
}

list_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Event) {
	#partial switch v in e {
	case gk.List_Event_Select:
		select(a, cx, v.row)
	case gk.List_Event_Confirm:
		select(a, cx, v.row)
	}
}

title_changed :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	if _, ok := e.(gk.Input_Event_Change); !ok || a.selected < 0 {
		return
	}
	value := gk.value(a.title, cx)
	n := &a.notes[a.selected]
	if n.title == string(value) {
		return
	}
	delete(n.title)
	n.title = strings.clone(string(value))
	gk.refresh(cx)
}

new_note :: proc(a: ^App, cx: ^gk.Cx) {
	a.made += 1
	append(&a.notes, Note{fmt.aprintf("Untitled %d", a.made), 0})
	select(a, cx, len(a.notes) - 1)
}

ask_delete :: proc(a: ^App, cx: ^gk.Cx) {
	if a.selected >= 0 {
		gk.open_alert_dialog(cx, gk.built(cx, a, confirm_dialog))
	}
}

confirm_dialog :: proc(a: ^App, cx: ^gk.Cx) -> gk.Alert_Dialog {
	name := a.notes[a.selected].title if a.selected >= 0 else ""
	return gk.alert_dialog()->
		title(gk.label(fmt.tprintf("Delete \"%s\"?", name)))->
		description(gk.label("The note will be gone for good."))->
		entrance(.None)->
		show_cancel(true)->
		on_ok(gk.on(confirm_delete))
}

confirm_delete :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	if a.selected < 0 {
		return true
	}
	delete(a.notes[a.selected].title)
	ordered_remove(&a.notes, a.selected)
	select(a, cx, min(a.selected, len(a.notes) - 1))
	return true // close the dialog
}

toggle_theme :: proc(a: ^App, cx: ^gk.Cx) {
	a.dark = !a.dark
	gk.theme_change(cx, gk.Theme_Mode.Dark if a.dark else gk.Theme_Mode.Light)
	gk.refresh(cx)
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Notes", {800, 600}, gk.view(a, init, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
