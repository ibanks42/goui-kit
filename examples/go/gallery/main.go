// Component gallery app used by gates/gallery.sh and gates/memory.sh to compare every
// hand-written page and generated conformance page pixel-for-pixel against ref-gallery.
// Supports --list, GALLERY_START=<page-id>, and GALLERY_LEAK=<bytes-per-build>.
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/ibanks42/goui-kit/gk"
	gkeditor "github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

const rowsCount = 10_000

type App struct {
	page  int
	count int
	flag  bool
	name  gkeditor.InputState
	rows  gk.Rows

	input   inputPages
	rowsFam rowsPages
	charts  chartPages
	lists2  lists2Pages
	overlay overlayPages
	editor  editorPages
	values  valuesPages
	views   viewsPages
	motion  motionPages

	root gk.View
	leak int
	lost [][]byte
}

type Page struct {
	ID    string
	Build func(a *App, cx *gk.Cx) gk.Element
}

func pageTables() [][]Page {
	return append([][]Page{pages, confPages, inputPageTable, rowsPageTable, chartPageTable, lists2PageTable,
		overlayPageTable, editorPageTable, valuesPageTable, viewsPageTable, motionPageTable}, pendingTables...)
}

var (
	pendingTables [][]Page
	pendingInits  []func(a *App, cx *gk.Cx)
)

var byID map[string]Page

func findPage(id string) (Page, bool) {
	if byID == nil {
		byID = map[string]Page{}
		for _, t := range pageTables() {
			for _, p := range t {
				if _, dup := byID[p.ID]; !dup {
					byID[p.ID] = p
				}
			}
		}
	}
	p, ok := byID[id]
	return p, ok
}

func (a *App) init(cx *gk.Cx) {
	a.name = gkeditor.NewInputState(cx).Placeholder("Name").Build()
	a.rows = gk.NewRows(cx)
	all := make([]string, rowsCount)
	for i := range all {
		all[i] = fmt.Sprintf("row %d", i)
	}
	a.rows.Push(cx, all...)
	inputPagesInit(a, cx)
	rowsPagesInit(a, cx)
	chartPagesInit(a, cx)
	lists2PagesInit(a, cx)
	overlayPagesInit(a, cx)
	editorPagesInit(a, cx)
	valuesPagesInit(a, cx)
	viewsPagesInit(a, cx)
	motionPagesInit(a, cx)
	for _, f := range pendingInits {
		f(a, cx)
	}
}

func (a *App) next(cx *gk.Cx, _ gk.ClickEvent) {
	a.page = (a.page + 1) % len(pageIDs)
	a.count = 0
	a.flag = false
	a.name.SetValue(cx, "")
}

func (a *App) view(cx *gk.Cx) gk.Element {
	if a.leak > 0 {
		lost := make([]byte, a.leak)
		for i := range lost {
			lost[i] = 1
		}
		a.lost = append(a.lost, lost)
	}
	id := pageIDs[a.page]
	var content gk.Element
	if p, ok := findPage(id); ok {
		content = p.Build(a, cx)
	} else {
		content = kit.Label("not bound in Go yet")
	}
	return gk.VFlex(s.SizeFull).
		Bg(theme.Background).
		TextColor(theme.Foreground).
		Child(gk.HFlex(s.P4, s.Gap3, s.ItemsCenter).
			Child(kit.Button("next").Label("Next").OnClick(a.next)).
			Child(kit.Label(fmt.Sprintf("%d/%d %s", a.page+1, len(pageIDs), id)))).
		Child(gk.Div(s.WFull, s.P4).Child(content))
}

func main() {
	a := &App{}
	if len(os.Args) > 1 && os.Args[1] == "--list" {
		for _, id := range pageIDs {
			if _, ok := findPage(id); ok {
				fmt.Println(id)
			}
		}
		return
	}
	if first, ok := os.LookupEnv("GALLERY_START"); ok {
		for i, id := range pageIDs {
			if id == first {
				a.page = i
			}
		}
	}
	if leak, ok := os.LookupEnv("GALLERY_LEAK"); ok {
		a.leak, _ = strconv.Atoi(leak)
	}
	if err := gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "Gallery", 800, 600, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}
