// files: a small text viewer that uses the platform's services (docs/PROTOCOLS.md 11.30, 13.30),
// the same app as ref/src/bin/files.rs: the menu bar (File: Open…,
// Save as…, Quit; Help: Website), the platform's file dialogs, a prompt before quitting, opening a
// URL, and a status line that slides and fades in when it changes. The menu bar is the platform's
// where it has one (macOS) and the kit's AppMenuBar drawn at the top of the window everywhere.
//
//	examples/go/build.sh files && bin/files-go
//
// Offscreen (the diag library's driver) the dialogs are not shown: the driver's script answers
// them with `answer ..` (gates/files.script, against its Rust twin: gates/pixels.sh files).
//
// Needs gk.NewAppMenuBar and gk.AppMenuBarView;
// until the Go package has them this program does not build (gates/go-pending.txt).
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/motion"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	root                   gk.View
	menubar                kit.AppMenuBar
	open, save, quit, site gk.Action
	path, text, status     string
	changes                int // a new id for the status line's animation each time it changes
	asking                 gk.Completion
	askingNow              bool
}

func (a *App) setStatus(s string) {
	a.status = s
	a.changes++
}

func (a *App) opened(cx *gk.Cx, d gk.PathsDone) {
	switch d.Status {
	case gk.DoneAnswered:
		data, err := os.ReadFile(d.Paths[0])
		if err != nil {
			a.setStatus(fmt.Sprintf("could not read %s", d.Paths[0]))
			return
		}
		a.text = string(data)
		a.path = d.Paths[0]
		a.setStatus(fmt.Sprintf("opened %s (%d bytes)", a.path, len(data)))
	case gk.DoneCancelled:
		a.setStatus("open cancelled")
	case gk.DoneFailed:
		a.setStatus(fmt.Sprintf("no file dialog: %s", d.Error))
	}
}

func (a *App) savedTo(cx *gk.Cx, d gk.PathsDone) {
	if d.Status != gk.DoneAnswered || len(d.Paths) == 0 {
		a.setStatus("save cancelled")
		return
	}
	if os.WriteFile(d.Paths[0], []byte(a.text), 0o644) == nil {
		a.setStatus(fmt.Sprintf("saved %s", d.Paths[0]))
	} else {
		a.setStatus(fmt.Sprintf("could not write %s", d.Paths[0]))
	}
}

func (a *App) quitAnswered(cx *gk.Cx, d gk.PromptDone) {
	a.askingNow = false
	if d.Status == gk.DoneAnswered && d.Answer == 0 {
		gk.Quit(cx)
		return
	}
	a.setStatus("still here")
}

func (a *App) doOpen(cx *gk.Cx, _ gk.Action) {
	gk.PromptForPaths(cx, gk.PathPrompt{Files: true, Prompt: "Open"}, a.opened)
}

func (a *App) doSave(cx *gk.Cx, _ gk.Action) {
	dir := "."
	if a.path != "" {
		dir = a.path[:max(strings.LastIndexByte(a.path, '/'), 0)]
	}
	gk.PromptForNewPath(cx, dir, "untitled.txt", a.savedTo)
}

func (a *App) doQuit(cx *gk.Cx, _ gk.Action) {
	if a.askingNow {
		return // one question at a time
	}
	a.askingNow = true
	a.asking = gk.Prompt(cx, gk.PromptWarning, "Quit the viewer?", "",
		[]gk.PromptButton{{Kind: gk.ButtonOk, Label: "Quit"}, {Kind: gk.ButtonCancel, Label: "Stay"}}, a.quitAnswered)
}

func (a *App) doSite(cx *gk.Cx, _ gk.Action) {
	gk.OpenURL(cx, "https://github.com/longbridge/gpui-component")
	a.setStatus("opened the website in the browser")
}

func (a *App) init(cx *gk.Cx) {
	a.open = gk.NewAction(cx, "files::Open")
	a.save = gk.NewAction(cx, "files::SaveAs")
	a.quit = gk.NewAction(cx, "files::Quit")
	a.site = gk.NewAction(cx, "files::Website")
	gk.BindKeys(cx, gk.KeyBinding{Keys: "ctrl-o", Action: a.open}, gk.KeyBinding{Keys: "ctrl-shift-s", Action: a.save},
		gk.KeyBinding{Keys: "ctrl-q", Action: a.quit})
	gk.OnAppAction(cx, a.open, a.doOpen)
	gk.OnAppAction(cx, a.save, a.doSave)
	gk.OnAppAction(cx, a.quit, a.doQuit)
	gk.OnAppAction(cx, a.site, a.doSite)
	gk.SetMenus(cx,
		gk.Menu{Name: "File", Items: []gk.MenuItem{
			gk.MenuAction{Name: "Open…", Action: a.open},
			gk.MenuAction{Name: "Save as…", Action: a.save},
			gk.MenuSeparator{},
			gk.MenuAction{Name: "Quit", Action: a.quit},
		}},
		gk.Menu{Name: "Help", Items: []gk.MenuItem{gk.MenuAction{Name: "Website", Action: a.site}}})
	a.menubar = kit.NewAppMenuBar(cx)
	a.setStatus("File > Open… (ctrl-o) to read a text file")
}

func (a *App) view(cx *gk.Cx) gk.Element {
	status := gk.Div(s.Relative, s.TextSm).H(gk.Px(24)).TextColor(theme.MutedForeground).Child(kit.Label(a.status))
	enter := motion.NewEffectTransition(250*time.Millisecond).Fade(0, 1).SlideY(8, 0)
	text := a.text
	if text == "" {
		text = "(no file)"
	}
	return gk.VFlex(s.SizeFull).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.Div(s.FlexNone).H(gk.Px(32)).Child(kit.AppMenuBarView(a.menubar))).
		Child(gk.VFlex(s.Flex1, s.P4, s.Gap2).
			Child(gk.Div(s.Flex1, s.Border1, s.P2, s.OverflowHidden).BorderColor(theme.Border).Child(kit.Label(text))).
			Child(kit.Effect(status, enter, fmt.Sprintf("status-%d", a.changes))))
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "Files", 720, 480, a.root)
	})
}
