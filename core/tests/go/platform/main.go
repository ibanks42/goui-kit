// Command platform runs headless and real-display tests for platform bridges:
// scripted/real file and confirmation dialogs, menus, URL opening, render
// images and caches, drag-and-drop, virtual lists, and input context menus.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
)

const ms = time.Millisecond

// App holds completion handles, image fixtures, and counters for the active GK_PROTO mode.
type App struct {
	mode      string
	root      gk.View
	childV    gk.View
	childW    gk.Window
	c1, c2    gk.Completion
	c3, c4    gk.Completion
	done      int
	described int
	last      string
	open      gk.Action
	save      gk.Action

	pic      gk.RenderImage
	picID    uint64
	ticks    gk.Timer
	shown0   uint64
	images0  uint32
	dropped0 uint64
	scroll   gk.ScrollHandle
	rows     gk.RowSource
	drops    int
	moves    int
	rowViews int

	askedAt time.Time
	polls   int
	barSeen bool
	inp     editor.InputState
	fired   int
}

var (
	inits = map[string]func(a *App, cx *gk.Cx){}
	views = map[string]func(a *App, cx *gk.Cx) gk.Element{}
)

func doneName(d gk.DoneStatus) string {
	switch d {
	case gk.DoneAnswered:
		return "Answered"
	case gk.DoneCancelled:
		return "Cancelled"
	case gk.DoneFailed:
		return "Failed"
	}
	return fmt.Sprint(uint8(d))
}

func (a *App) never(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	fmt.Println("platform cancelled request answered (wrong)")
}

// menuFired logs the custom context-menu callback payload for `context-menu` mode.
func (a *App) menuFired(cx *gk.Cx, e editor.Event114) {
	a.fired++
	copyAct := gk.NewAction(cx, "input::Copy")
	actions := 0
	for _, it := range e.A0.Items {
		if x, ok := it.(editor.NativeMenuItemAction); ok && x.Action == copyAct {
			actions++
		}
	}
	fmt.Printf("platform context-menu fired items=%d copy=%d at=%.0f,%.0f editable=%v disabled=%v cursor=%d\n", len(e.A0.Items), actions,
		e.A2.X, e.A2.Y, e.A1.IsEditable(cx), e.A1.IsDisabled(cx), a.inp.Cursor(cx))
}

// contextMenuView proves OnContextMenu fires on right-click for BaseInput
// (`context-menu`, fired=1) while kit Input (`context-menu-kit`, fired=0) uses
// the kit's built-in context menu unless overridden.
func (a *App) contextMenuView(cx *gk.Cx) gk.Element {
	box := gk.Div().W(gk.Px(220)).H(gk.Px(32))
	if a.mode == "context-menu" {
		box = box.Child(editor.BaseInput(a.inp))
	} else {
		box = box.Child(editor.Input(a.inp))
	}
	return gk.VFlex(s.SizeFull).Child(box)
}

func (a *App) contextMenuCheck(cx *gk.Cx) {
	fmt.Printf("platform context-menu check mode=%s fired=%d cursor=%d\n", a.mode, a.fired, a.inp.Cursor(cx))
}

// misuse proves releasing a Completion twice fails cleanly (`release-twice`)
// and submitting a MenuAction with a zero Action fails validation (`menu-no-action`).
func (a *App) misuse(cx *gk.Cx) {
	switch a.mode {
	case "release-twice":
		c := gk.PromptForPaths(cx, gk.PathPrompt{Files: true}, a.never)
		gk.Release(cx, c)
		gk.Release(cx, c)
	case "menu-no-action":
		gk.SetMenus(cx, gk.Menu{Name: "File", Items: []gk.MenuItem{gk.MenuAction{Name: "Open"}}})
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	a.described++
	fmt.Printf("platform view described=%d done=%d\n", a.described, a.done)
	if a.mode == "request-in-view" {
		gk.PromptForPaths(cx, gk.PathPrompt{Files: true}, a.never)
	}
	if a.mode == "image-in-view" {
		gk.NewRenderImage(cx, gk.PixelImage{Width: 1, Height: 1, RGBA: []byte{0, 0, 0, 255}})
	}
	if f, ok := views[a.mode]; ok {
		return f(a, cx)
	}
	switch a.mode {
	case "context-menu", "context-menu-kit":
		return a.contextMenuView(cx)
	}
	return gk.VFlex(s.SizeFull).Child(kit.Label(fmt.Sprintf("done %d %s", a.done, a.last)))
}

func (a *App) init(cx *gk.Cx) {
	a.open = gk.NewAction(cx, "platform::Open")
	a.save = gk.NewAction(cx, "platform::Save")
	if f, ok := inits[a.mode]; ok {
		f(a, cx)
		return
	}
	switch a.mode {
	case "release-twice", "menu-no-action":
		gk.After(cx, 10*ms, a.misuse)
	case "image-in-view", "request-in-view":

	case "context-menu", "context-menu-kit":
		a.inp = editor.NewInputState(cx).DefaultValue("menu text").Build()
		a.inp.OnContextMenu(cx, a.menuFired)
		gk.After(cx, 10*ms, a.contextMenuCheck)
	}
}

func (a *App) start(cx *gk.Cx) {
	a.root = gk.NewView(a.init, a.view)
	gk.OpenWindow(cx, "platform", 400, 300, a.root)
}

var known = map[string]bool{"release-twice": true, "request-in-view": true, "menu-no-action": true, "image-in-view": true,
	"context-menu": true, "context-menu-kit": true}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	if _, ok := inits[a.mode]; !ok && !known[a.mode] {
		fmt.Printf("platform: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	script := "size 400 300\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nadvance 10\nframe\nstat\n"
	if strings.HasPrefix(a.mode, "real-") {

		if err := gk.Run(a.start); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("platform done mode=%s done=%d described=%d\n", a.mode, a.done, a.described)
		return
	}
	if a.mode == "context-menu" || a.mode == "context-menu-kit" {

		script = "size 400 300\nframe\nframe\nrclick 40 16\nframe\nadvance 10\nframe\nstat\n"
	}
	if path, ok := os.LookupEnv("GK_PLATFORM_SCRIPT"); ok {

		if b, err := os.ReadFile(path); err == nil {
			script = string(b)
		}
	}
	if a.mode == "drag" {

		script = "size 400 300\nframe\nframe\ndrag 50 50 150 50\nframe\ndrag 50 50 250 50\nframe\nadvance 30\nframe\nadvance 40\nframe\nstat\n"
	}
	if err := gk.RunHeadless(script, a.start); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("platform done mode=%s done=%d described=%d\n", a.mode, a.done, a.described)
}
