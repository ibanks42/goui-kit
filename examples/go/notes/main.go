// notes: a small notes app, the Go twin of ref/src/bin/notes.rs: the kit's
// styled dock with two panels (a list of notes, an editor with the kit's Input), a File menu whose
// items are actions with keyboard shortcuts, a confirmation dialog, and the theme switched
// between light and dark. gates/pixels.sh notes runs Rust and Go under gates/notes.script
// and compares every image byte for byte.
//
//	notes-go                    a real window
//	GK_SCRIPT=file notes-go     offscreen under a gk-meter script (the diag library)
package main

import (
	"fmt"
	"log"
	"slices"

	"gpuikit.dev/gk"
	gkdock "gpuikit.dev/gk/dock"
	gkeditor "gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/table"
	"gpuikit.dev/gk/theme"
)

type Note struct {
	Title string
	Words int
}

type App struct {
	notes    []Note
	selected int // -1: none
	made     int // notes made so far, for "Untitled N"
	dark     bool

	title gkeditor.InputState // kit state that lives in the library
	list  table.ListState
	dock  gkdock.Area

	newNote, deleteNote, toggleTheme gk.Action // shortcuts, also in the menu (it shows their keys)
}

func (a *App) init(cx *gk.Cx) {
	a.notes = []Note{{"Groceries", 4}, {"Ideas for the weekend", 12}, {"Meeting notes", 31}}
	a.made = len(a.notes)

	a.newNote = gk.Shortcut(cx, "ctrl-n", a.addNote)
	a.deleteNote = gk.Shortcut(cx, "", a.askDelete) // the menu's item only
	a.toggleTheme = gk.Shortcut(cx, "ctrl-t", a.switchTheme)

	a.title = gkeditor.NewInputState(cx).Placeholder("Title").Build()
	a.title.OnInputEvent(cx, a.titleChanged)
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.notes) }, a.noteRow)).Build()
	a.list.OnListEvent(cx, a.listEvent)

	a.dock = gkdock.NewAreaSkinned(cx, "notes")
	notes := gkdock.NewPanel(cx, gk.NewView(nil, a.notesView), gkdock.PanelData{Name: "notes", Title: "Notes"})
	editor := gkdock.NewPanel(cx, gk.NewView(nil, a.editorView), gkdock.PanelData{Name: "editor", Title: "Editor"})
	// PanelView: the kit's PanelHandle over the panel, so the tabs show the titles
	a.dock.SetDock(cx, gkdock.PlacementLeft, gkdock.NewLayoutTabs().PanelView(notes))
	a.dock.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(editor))
	a.selectNote(cx, 0)
}

// --- the views --------------------------------------------------------------------------------

func (a *App) fileMenu() kit.PopupMenu {
	return kit.NewPopupMenu().
		Menu("New note", a.newNote).
		Menu("Toggle theme", a.toggleTheme).
		Separator().
		MenuWithDisabled("Delete note…", a.deleteNote, a.selected < 0)
}

func (a *App) view(cx *gk.Cx) gk.Element {
	themeLabel := "Dark theme"
	if a.dark {
		themeLabel = "Light theme"
	}
	bar := gk.HFlex(s.Gap2, s.ItemsCenter, s.P2, s.BorderB1).BorderColor(theme.Border).
		Child(kit.DropdownMenu(kit.Button("file").Label("File"), a.fileMenu())).
		Child(kit.Label(fmt.Sprintf("%d notes", len(a.notes)))).
		Child(gk.Div(s.Flex1)).
		Child(kit.Button("theme").Label(themeLabel).OnClickDo(a.switchTheme))
	return gk.VFlex(s.SizeFull).Bg(theme.Background).TextColor(theme.Foreground).
		Child(bar).
		Child(gk.Div(s.Flex1, s.WFull, s.MinH0).Child(gkdock.View(a.dock)))
}

func (a *App) notesView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(table.List(a.list).ScrollbarVisible(false))
}

func (a *App) noteRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	n := a.notes[at.Row]
	return table.ListItem(gk.Key("note", at.Row)).
		Child(gk.VFlex().
			Child(kit.Label(n.Title)).
			Child(kit.Label(fmt.Sprintf("%d words", n.Words), s.TextSm).TextColor(theme.MutedForeground)))
}

func (a *App) editorView(cx *gk.Cx) gk.Element {
	if a.selected < 0 {
		return gk.VFlex(s.SizeFull, s.P4, s.ItemsCenter).Child(kit.Label("No note selected"))
	}
	n := a.notes[a.selected]
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Child(kit.Label("Title")).
		Child(gkeditor.Input(a.title)).
		Child(kit.Label(fmt.Sprintf("%d words", n.Words))).
		Child(gk.HFlex(s.Gap2).
			Child(kit.Button("delete").Danger().Label("Delete").OnClickDo(a.askDelete)))
}

func (a *App) confirmView(cx *gk.Cx) overlay.AlertDialogEl {
	name := ""
	if a.selected >= 0 {
		name = a.notes[a.selected].Title
	}
	return overlay.AlertDialog().
		Title(kit.Label(fmt.Sprintf("Delete \"%s\"?", name))).
		Description(kit.Label("The note will be gone for good.")).
		Entrance(overlay.DialogEntranceNone).
		ShowCancel(true).
		OnOk(a.confirmDelete)
}

// --- what the app does ------------------------------------------------------------------------

func (a *App) selectNote(cx *gk.Cx, row int) {
	a.selected = row
	if row >= 0 {
		a.title.SetValue(cx, a.notes[row].Title)
	}
	table.SelectRow(cx, a.list, row)
	gk.Refresh(cx)
}

func (a *App) listEvent(cx *gk.Cx, e table.ListEvent) {
	switch e := e.(type) {
	case table.ListEventSelect:
		a.selectNote(cx, e.Row)
	case table.ListEventConfirm:
		a.selectNote(cx, e.Row)
	}
}

func (a *App) titleChanged(cx *gk.Cx, e gkeditor.InputEvent) {
	if _, ok := e.(gkeditor.InputEventChange); !ok || a.selected < 0 {
		return
	}
	if title := a.title.Value(cx); title != a.notes[a.selected].Title {
		a.notes[a.selected].Title = title
		gk.Refresh(cx)
	}
}

func (a *App) addNote(cx *gk.Cx) {
	a.made++
	a.notes = append(a.notes, Note{Title: fmt.Sprintf("Untitled %d", a.made)})
	a.selectNote(cx, len(a.notes)-1)
}

func (a *App) askDelete(cx *gk.Cx) {
	if a.selected >= 0 {
		overlay.OpenAlertDialog(cx, a.confirmView)
	}
}

func (a *App) confirmDelete(cx *gk.Cx, _ gk.ClickEvent) bool {
	if a.selected >= 0 {
		a.notes = slices.Delete(a.notes, a.selected, a.selected+1)
		a.selectNote(cx, min(a.selected, len(a.notes)-1))
	}
	return true // close the dialog
}

func (a *App) switchTheme(cx *gk.Cx) {
	a.dark = !a.dark
	mode := gk.ThemeModeLight
	if a.dark {
		mode = gk.ThemeModeDark
	}
	gk.ThemeChange(cx, mode)
	gk.Refresh(cx)
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Notes", 800, 600, gk.NewView(a.init, a.view))
	}); err != nil {
		log.Fatal(err)
	}
}
