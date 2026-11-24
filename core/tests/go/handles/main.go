// handles: the protocol tests of library objects behind handles (docs/PROTOCOLS.md section 13):
// kit values that are not entities, held by the library and named by a handle of kind OBJ. The
// port of core/tests/handles. Each mode runs offscreen (the diag library) under a gk-meter script
// and prints lines that core/tests/go/run-handles.sh checks, with the exit code.
//
//	GK_PROTO=obj-objects | obj-temps | obj-stale-temp | obj-global | obj-dialog | obj-list |
//	         obj-given-away | obj-values | obj-stale-value | obj-columns
//
// The modes that count handles and obj-columns are in objects.go.
package main

import (
	"fmt"
	"os"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/base"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	gktable "gpuikit.dev/gk/table"
)

const rows = 200

type App struct {
	mode   string
	root   gk.View
	clicks int
	// temps
	input editor.InputState
	temp  editor.InputPresentation
	kept  editor.InputPresentation
	// list
	src    gk.RowSource
	list   gktable.ListState
	scroll gktable.VirtualListScrollHandle
	events int
	// dialog
	dialog overlay.DialogHandle
	// values
	tree  gktable.TreeState
	grid  base.GridLayoutState
	keptV gktable.TreeItem
	// columns (objects.go)
	tsrc  gk.RowSource
	table gktable.State
}

// modes: what a click runs, by mode; some register from objects.go.
var modes = map[string]func(a *App, cx *gk.Cx){}

// views: what a mode adds to the root view, by mode (objects.go registers obj-columns's table).
var views = map[string]func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl{}

// counts prints the temporary handles waiting for the next frame and the live library objects.
func counts(cx *gk.Cx, what string) {
	temps, live := gk.TestObjCounts(cx)
	fmt.Printf("handles counts %s temps=%d live=%d\n", what, temps, live)
}

// A read that hands out an object gives a temporary handle: usable in its turn, swept when
// the next frame begins; gk.Retain keeps one.
func temps(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		a.input = editor.NewInputState(cx).Placeholder("type here").Build()
		counts(cx, "before")
		a.temp = a.input.Presentation(cx)
		a.kept = a.input.Presentation(cx)
		gk.Retain(cx, a.kept)
		fmt.Printf("handles temp placeholder=%q editable=%v multi_line=%v\n", a.temp.Placeholder(cx), a.temp.IsEditable(cx), a.temp.IsMultiLine(cx))
		counts(cx, "handed out")
	case 2:
		counts(cx, "next frame")
		fmt.Printf("handles kept placeholder=%q\n", a.kept.Placeholder(cx))
		gk.Release(cx, a.kept)
		if a.mode == "obj-stale-temp" {
			fmt.Println("handles reading the swept temporary")
			fmt.Println(a.temp.Placeholder(cx)) // stops: it was temporary
		}
	}
}

// A dialog's handle opens and closes it from code (writes with the window).
func dialog(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		a.dialog = overlay.NewDialogHandle(cx, false)
		fmt.Printf("handles dialog open=%v\n", a.dialog.IsOpen(cx))
		a.dialog.Open(cx)
	case 2:
		fmt.Printf("handles dialog open=%v\n", a.dialog.IsOpen(cx))
		a.dialog.Close(cx)
		fmt.Printf("handles dialog closed open=%v\n", a.dialog.IsOpen(cx))
	}
}

func (a *App) listRow(cx *gk.Cx, at gk.RowAt) gktable.ListItemEl {
	return gktable.ListItem(gk.Key("row", at.Row)).Child(gk.Text(fmt.Sprintf("row %d", at.Row)))
}

func (a *App) listEvent(cx *gk.Cx, e gktable.ListDelegateEvent) {
	a.events++
}

// A list selected and scrolled from code: an IndexPath travels as its record, and the list's
// scroll handle is a library object.
func list(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		// a row source needs a window: made in a handler
		a.src = gktable.NewListSource(cx, func() int { return rows }, a.listRow, gktable.ListEvents(a.listEvent), gk.Initial(20))
		a.list = gktable.NewListState(cx, a.src).Build()
	case 2:
		a.list.SetSelectedIndex(cx, gk.Some(gktable.Row(120)))
		a.list.ScrollToItem(cx, gktable.Row(120), gk.ScrollStrategyTop)
		a.scroll = a.list.ScrollHandle(cx)
		gk.Retain(cx, a.scroll)
	case 3:
		sel, ok := a.list.SelectedIndex(cx).Get()
		fmt.Printf("handles list selected ok=%v row=%d\n", ok, sel.Row)
		a.scroll.ScrollToItem(cx, 3, gk.ScrollStrategyTop)
		a.list.SetSelectedIndex(cx, gktable.NoRow)
	case 4:
		_, ok := a.list.SelectedIndex(cx).Get()
		fmt.Printf("handles list cleared selected=%v\n", ok)
	}
}

// Builder values made in a handler, outside any view, given to call ops: each travels as a
// tape of its own and is decoded by the value's own decoder (a tree's items, a grid's areas).
func values(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		a.tree = gktable.NewTreeState(cx).Build()
		src := gktable.NewTreeItem("src", "src").Expanded(true).Child(gktable.NewTreeItem("src/a.rs", "a.rs")).Child(gktable.NewTreeItem("src/b.rs", "b.rs"))
		a.tree.SetItems(cx, []gktable.TreeItem{src, gktable.NewTreeItem("README", "README")})
		a.grid = base.NewGridLayoutState(cx).Build()
		a.grid.SetAreas(cx, []base.GridArea{base.NewGridArea("one").Col(1).Row(2).ColSpan(3), base.NewGridArea("two")})
		a.keptV = gktable.NewTreeItem("late", "late")
	case 2:
		n := 0
		for i := range 8 {
			e, ok := a.tree.Entry(cx, i).Get()
			if !ok {
				break
			}
			it := e.Item(cx)
			fmt.Printf("handles tree entry %d id=%s depth=%d folder=%v\n", i, it.ID, e.Depth(cx), e.IsFolder(cx))
			n++
		}
		fmt.Printf("handles tree entries=%d\n", n)
		for _, ar := range a.grid.Areas(cx) {
			fmt.Printf("handles grid area id=%s col=%d row=%d col_span=%d\n", ar.ID, ar.Col, ar.Row, ar.ColSpan)
		}
		if a.mode == "obj-stale-value" {
			fmt.Println("handles giving a value of an earlier turn")
			a.tree.SetItems(cx, []gktable.TreeItem{a.keptV}) // stops: a value lives for the turn that made it
		}
	}
}

func init() {
	modes["obj-temps"] = temps
	modes["obj-stale-temp"] = temps
	modes["obj-dialog"] = dialog
	modes["obj-list"] = list
	modes["obj-values"] = values
	modes["obj-stale-value"] = values
	views["obj-list"] = func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
		if a.list == (gktable.ListState{}) {
			return body
		}
		return body.Child(gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(gktable.List(a.list, s.SizeFull)))
	}
}

func (a *App) clicked(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	if f, ok := modes[a.mode]; ok {
		f(a, cx)
	}
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	body := gk.Div(s.SizeFull).ID("all").OnClick(a.clicked)
	if f, ok := views[a.mode]; ok {
		body = f(a, cx, body)
	}
	return body
}

func (a *App) start(cx *gk.Cx) {
	a.root = gk.NewView(nil, a.rootView)
	gk.OpenWindow(cx, "handles", 640, 480, a.root)
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	if _, ok := modes[a.mode]; !ok {
		fmt.Printf("handles: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	script := "size 640 480\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nstat\n"
	gk.RunHeadless(script, a.start)
	fmt.Printf("handles done mode=%s clicks=%d\n", a.mode, a.clicks)
}
