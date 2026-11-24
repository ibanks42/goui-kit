// platform: the protocol tests of the platform and motion families (docs/PROTOCOLS.md 11.30), the
// port of core/tests/platform: requests that answer later (completions delivered by a DONE turn),
// file dialogs, a window's prompt, the menu bar and OpenURL. Offscreen the platform is scripted: the
// test answers its own dialogs with gk.PlatformAnswer*. Each mode prints lines that
// core/tests/go/run-platform.sh checks, with the exit code.
//
//	GK_PROTO=dialogs | cancel | race | view-released | menus | release-twice | request-in-view |
//	         menu-no-action
//	         PROTOCOLS.md 13.31: elements | drag | vlist | cache-in-view | scope-zero | image-in-view
//	         PROTOCOLS.md 11.32: real-dialogs | real-url | real-menus (a real window, the
//	         platform asked: run by bin/platform-real-go under gates/display.sh, GK_SCRIPTED_PLATFORM
//	         unset); context-menu | context-menu-kit (offscreen: an input's right click)
//	GK_PLATFORM_SCRIPT=FILE   a gk-meter script instead of the mode's (to take shots of what a mode draws)
//
// A wrong payload (a handler whose payload is not the request's) is a compile error in Go
// (examples/go/misuse_compile). The modes of the scripted platform's log, the leftovers of 13.31 and
// the kit's menu bar view are in modes.go.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

const ms = time.Millisecond

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
	// PROTOCOLS.md 13.31
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
	// PROTOCOLS.md 11.32
	askedAt time.Time
	polls   int
	barSeen bool
	inp     gk.InputState
	fired   int
}

// inits: what a mode starts in the root view's init, by mode; views: what it draws instead of the
// default. The modes of modes.go register from it.
var (
	inits = map[string]func(a *App, cx *gk.Cx){}
	views = map[string]func(a *App, cx *gk.Cx) gk.Element{}
)

// doneName prints a DoneStatus as the checks expect (the variant's name).
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

// --- cancel: released before its answer ----------------------------------------------------------

func (a *App) never(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	fmt.Println("platform cancelled request answered (wrong)")
}

// --- an input's context menu under the offscreen driver (PROTOCOLS.md 11.32) -------------
//
// InputState.OnContextMenu (upstream: "the right-click context menu" handler of the state) set in
// init. Drawn by the base element (gk.BaseInput, gpui-base's Input) the state keeps it: a right
// click runs it, after the release, with upstream's empty NativeMenu, the click's position and the
// input's capabilities (mode context-menu). Drawn by the kit's element (gk.Input, gpui-component's
// Input) it is replaced each time the element draws: Input::render installs the kit's own handler
// (its Cut/Copy/Paste/Select All menu, or Input::context_menu's), so the app's never runs (mode
// context-menu-kit). Either way the right press reaches the state: its cursor moves into the text.

func (a *App) menuFired(cx *gk.Cx, e gk.Event114) {
	a.fired++
	copyAct := gk.NewAction(cx, "input::Copy")
	actions := 0
	for _, it := range e.A0.Items {
		if x, ok := it.(gk.NativeMenuItemAction); ok && x.Action == copyAct {
			actions++
		}
	}
	fmt.Printf("platform context-menu fired items=%d copy=%d at=%.0f,%.0f editable=%v disabled=%v cursor=%d\n", len(e.A0.Items), actions,
		e.A2.X, e.A2.Y, e.A1.IsEditable(cx), e.A1.IsDisabled(cx), a.inp.Cursor(cx))
}

func (a *App) contextMenuView(cx *gk.Cx) gk.Element {
	box := gk.Div().W(gk.Px(220)).H(gk.Px(32))
	if a.mode == "context-menu" {
		box = box.Child(gk.BaseInput(a.inp))
	} else {
		box = box.Child(gk.Input(a.inp))
	}
	return gk.VFlex(s.SizeFull).Child(box)
}

func (a *App) contextMenuCheck(cx *gk.Cx) {
	fmt.Printf("platform context-menu check mode=%s fired=%d cursor=%d\n", a.mode, a.fired, a.inp.Cursor(cx))
}

// --- misuse ---------------------------------------------------------------------------------------

func (a *App) misuse(cx *gk.Cx) {
	switch a.mode {
	case "release-twice":
		c := gk.PromptForPaths(cx, gk.PathPrompt{Files: true}, a.never)
		gk.Release(cx, c)
		gk.Release(cx, c) // stops: released already
	case "menu-no-action":
		gk.SetMenus(cx, gk.Menu{Name: "File", Items: []gk.MenuItem{gk.MenuAction{Name: "Open"}}}) // stops: no action
	}
}

// --- the app ------------------------------------------------------------------------------------

func (a *App) view(cx *gk.Cx) gk.Element {
	a.described++
	fmt.Printf("platform view described=%d done=%d\n", a.described, a.done)
	if a.mode == "request-in-view" {
		gk.PromptForPaths(cx, gk.PathPrompt{Files: true}, a.never) // stops: a view function may not start a request
	}
	if a.mode == "image-in-view" {
		gk.NewRenderImage(cx, gk.PixelImage{Width: 1, Height: 1, RGBA: []byte{0, 0, 0, 255}}) // stops: a view function may not make an image
	}
	if f, ok := views[a.mode]; ok {
		return f(a, cx)
	}
	switch a.mode {
	case "context-menu", "context-menu-kit":
		return a.contextMenuView(cx)
	}
	return gk.VFlex(s.SizeFull).Child(gk.Label(fmt.Sprintf("done %d %s", a.done, a.last)))
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
		// misuse checked in view
	case "context-menu", "context-menu-kit":
		a.inp = gk.NewInputState(cx).DefaultValue("menu text").Build()
		a.inp.OnContextMenu(cx, a.menuFired)
		gk.After(cx, 10*ms, a.contextMenuCheck)
	}
}

func (a *App) start(cx *gk.Cx) {
	a.root = gk.NewView(a.init, a.view)
	gk.OpenWindow(cx, "platform", 400, 300, a.root)
}

// known: the modes of main.go; modes.go's register in inits.
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
		// a real window and the platform's own services (PROTOCOLS.md 11.32)
		gk.Run(a.start)
		fmt.Printf("platform done mode=%s done=%d described=%d\n", a.mode, a.done, a.described)
		return
	}
	if a.mode == "context-menu" || a.mode == "context-menu-kit" {
		// a right click on the input's text, inside it (the cursor was at its end)
		script = "size 400 300\nframe\nframe\nrclick 40 16\nframe\nadvance 10\nframe\nstat\n"
	}
	if path, ok := os.LookupEnv("GK_PLATFORM_SCRIPT"); ok {
		// a test's own script instead of the mode's (diagnosis: shots of what a mode draws)
		if b, err := os.ReadFile(path); err == nil {
			script = string(b)
		}
	}
	if a.mode == "drag" {
		// onto the target that takes 7, then from the source onto the one that takes only 8
		script = "size 400 300\nframe\nframe\ndrag 50 50 150 50\nframe\ndrag 50 50 250 50\nframe\nadvance 30\nframe\nadvance 40\nframe\nstat\n"
	}
	gk.RunHeadless(script, a.start)
	fmt.Printf("platform done mode=%s done=%d described=%d\n", a.mode, a.done, a.described)
}
