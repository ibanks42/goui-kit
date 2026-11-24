// gallery: the Go twin of ref/src/bin/gallery, the app that shows
// every stateless component in a fixed set of states, one page per component and state group.
// The pages are named by ref/gallery/pages.txt and ref/gallery/conf.txt (their ids are compiled
// in through conf.go, which gates/conformance.py writes); gates/gallery.sh go compares this app
// with the Rust reference pixel for pixel, page by page. A page the Go package cannot build yet
// is absent from the page tables: it shows a placeholder and `--list` leaves it out, so the gate
// counts it as not bound instead of comparing it. gates/gallery-go-pending.txt lists the pages
// that wait for a Go binding (each with the missing function); a page waiting is written in a
// *_pending.go file, built only with the tag gallery_pending, until the binding lands (none now).
//
//	GK_SCRIPT=file gallery-go   offscreen under a gk-meter script (the diag library)
//	gallery-go --list           the page ids this app has, one per line
//	gallery-go                  a real window (GK_FRAMES and GK_CLICK drive and measure it)
//	GALLERY_START=id            the page shown first
//	GALLERY_LEAK=bytes          leak that much on every view build: the memory gate's
//	                            self-test (gates/memory.sh --selftest must fail on it)
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"gpuikit.dev/gk"
	gkeditor "gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

const rowsCount = 10_000

// App is the gallery's state: the current page and each family's own states.
type App struct {
	page  int
	count int  // clicks on the current page's button
	flag  bool // the current page's checkbox or switch
	name  gkeditor.InputState
	rows  gk.Rows

	input   inputPages   // input_pages.go
	rowsFam rowsPages    // rows_pages.go
	charts  chartPages   // chart_pages.go
	lists2  lists2Pages  // lists2_pages.go
	overlay overlayPages // overlay_pages.go
	editor  editorPages  // editor_pages.go
	values  valuesPages  // values_pages.go
	views   viewsPages   // views_pages.go
	motion  motionPages  // motion_pages.go

	root gk.View
	leak int
	lost [][]byte
}

// Page is one page: its id in pages.txt or conf.txt and the function that builds it.
type Page struct {
	ID    string
	Build func(a *App, cx *gk.Cx) gk.Element
}

// pageTables are the pages of every file, looked up in this order (as the Rust twin's
// or_else chain), then the pending ones.
func pageTables() [][]Page {
	return append([][]Page{pages, confPages, inputPageTable, rowsPageTable, chartPageTable, lists2PageTable,
		overlayPageTable, editorPageTable, valuesPageTable, viewsPageTable, motionPageTable}, pendingTables...)
}

// pendingTables and pendingInits are the pages that wait for a Go binding and their states: a
// file built only with the tag gallery_pending adds to them (gates/gallery-go-pending.txt).
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
	// The families in the order of the Rust twin's or_else chain.
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
			lost[i] = 1 // touched, so it is resident
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
