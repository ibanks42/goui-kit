// notes: a small notes app, the Go twin of examples/odin/notes and ref/src/bin/notes.rs: the kit's
// styled dock with two panels (a list of notes, an editor with the kit's Input), a File menu whose
// items are actions with keyboard shortcuts, a confirmation dialog, and the theme switched
// between light and dark. gates/pixels.sh notes runs Rust, Odin and Go under gates/notes.script
// and compares every image byte for byte.
//
//	notes-go                    a real window
//	GK_SCRIPT=file notes-go     offscreen under a gk-meter script (the diag library)
package main

import (
	"fmt"
	"slices"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
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

	title   gk.InputState // kit state that lives in the library
	list    gk.ListState
	dock    gk.DockArea
	confirm gk.Built[gk.AlertDialogEl]

	newNote, deleteNote, toggleTheme gk.Action // shortcuts, also in the menu (it shows their keys)
}

func (a *App) init(cx *gk.Cx) {
	a.notes = []Note{{"Groceries", 4}, {"Ideas for the weekend", 12}, {"Meeting notes", 31}}
	a.made = len(a.notes)

	a.newNote = gk.Shortcut(cx, "ctrl-n", a.addNote)
	a.deleteNote = gk.Shortcut(cx, "", a.askDelete) // the menu's item only
	a.toggleTheme = gk.Shortcut(cx, "ctrl-t", a.switchTheme)

	a.title = gk.NewInputState(cx).Placeholder("Title").Build()
	a.title.OnInputEvent(cx, a.titleChanged)
	a.list = gk.NewListState(cx, gk.NewListSource(cx, func() int { return len(a.notes) }, a.noteRow)).Build()
	a.list.OnListEvent(cx, a.listEvent)
	a.confirm = gk.NewBuilt(cx, a.confirmView)

	a.dock = gk.NewDockAreaSkinned(cx, "notes")
	notes := gk.NewPanel(cx, gk.NewView(nil, a.notesView), gk.PanelData{Name: "notes", Title: "Notes"})
	editor := gk.NewPanel(cx, gk.NewView(nil, a.editorView), gk.PanelData{Name: "editor", Title: "Editor"})
	// PanelView: the kit's PanelHandle over the panel, so the tabs show the titles
	a.dock.SetDock(cx, gk.DockPlacementLeft, gk.NewDockLayoutTabs().PanelView(notes))
	a.dock.SetCenter(cx, gk.NewDockLayoutTabs().PanelView(editor))
	a.selectNote(cx, 0)
}

// --- the views --------------------------------------------------------------------------------

func (a *App) fileMenu() gk.PopupMenu {
	return gk.NewPopupMenu().
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
		Child(gk.DropdownMenu(gk.Button("file").Label("File"), a.fileMenu())).
		Child(gk.Label(fmt.Sprintf("%d notes", len(a.notes)))).
		Child(gk.Div(s.Flex1)).
		Child(gk.Button("theme").Label(themeLabel).OnClick(a.switchTheme))
	return gk.VFlex(s.SizeFull).Bg(theme.Background).TextColor(theme.Foreground).
		Child(bar).
		Child(gk.Div(s.Flex1, s.WFull, s.MinH0).Child(gk.DockView(a.dock)))
}

func (a *App) notesView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(gk.List(a.list).ScrollbarVisible(false))
}

func (a *App) noteRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	n := a.notes[at.Row]
	return gk.ListItem(gk.Key("note", at.Row)).
		Child(gk.VFlex().
			Child(gk.Label(n.Title)).
			Child(gk.Label(fmt.Sprintf("%d words", n.Words), s.TextSm).TextColor(theme.MutedForeground)))
}

func (a *App) editorView(cx *gk.Cx) gk.Element {
	if a.selected < 0 {
		return gk.VFlex(s.SizeFull, s.P4, s.ItemsCenter).Child(gk.Label("No note selected"))
	}
	n := a.notes[a.selected]
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Child(gk.Label("Title")).
		Child(gk.Input(a.title)).
		Child(gk.Label(fmt.Sprintf("%d words", n.Words))).
		Child(gk.HFlex(s.Gap2).
			Child(gk.Button("delete").Danger().Label("Delete").OnClick(a.askDelete)))
}

func (a *App) confirmView(cx *gk.Cx) gk.AlertDialogEl {
	name := ""
	if a.selected >= 0 {
		name = a.notes[a.selected].Title
	}
	return gk.AlertDialog().
		Title(gk.Label(fmt.Sprintf("Delete \"%s\"?", name))).
		Description(gk.Label("The note will be gone for good.")).
		Entrance(gk.DialogEntranceNone).
		ShowCancel(true).
		OnOk(a.confirmDelete)
}

// --- what the app does ------------------------------------------------------------------------

func (a *App) selectNote(cx *gk.Cx, row int) {
	a.selected = row
	if row >= 0 {
		a.title.SetValue(cx, a.notes[row].Title)
	}
	gk.SelectRow(cx, a.list, row)
	gk.Refresh(cx)
}

func (a *App) listEvent(cx *gk.Cx, e gk.ListEvent) {
	switch e := e.(type) {
	case gk.ListEventSelect:
		a.selectNote(cx, e.Row)
	case gk.ListEventConfirm:
		a.selectNote(cx, e.Row)
	}
}

func (a *App) titleChanged(cx *gk.Cx, e gk.InputEvent) {
	if _, ok := e.(gk.InputEventChange); !ok || a.selected < 0 {
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

func (a *App) askDelete(cx *gk.Cx, _ gk.ClickEvent) {
	if a.selected >= 0 {
		gk.OpenAlertDialog(cx, a.confirm)
	}
}

func (a *App) confirmDelete(cx *gk.Cx, _ gk.ClickEvent) bool {
	if a.selected >= 0 {
		a.notes = slices.Delete(a.notes, a.selected, a.selected+1)
		a.selectNote(cx, min(a.selected, len(a.notes)-1))
	}
	return true // close the dialog
}

func (a *App) switchTheme(cx *gk.Cx, _ gk.ClickEvent) {
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
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Notes", 800, 600, gk.NewView(a.init, a.view))
	})
}
