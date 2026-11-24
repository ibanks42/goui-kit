// The modes of the scripted platform's log (dialogs, cancel, race, view-released, menus, real-dialogs,
// real-url), gpui's element leftovers of PROTOCOLS.md 13.31 (elements, cache-in-view, scope-zero), a
// drag's move handler (drag), the source's virtual lists (vlist) and the kit's menu bar (real-menus).
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

// What these modes keep.
var ov struct {
	cache   gk.ImageCache
	anchor  gk.ScrollAnchor
	scope   gk.TextSelectionScope
	menubar gk.AppMenuBar
}

func init() {
	inits["dialogs"] = func(a *App, cx *gk.Cx) {
		gk.After(cx, 10*ms, a.askOpen)
		gk.After(cx, 30*ms, a.answerAll)
		gk.After(cx, 60*ms, a.checkLog)
	}
	inits["cancel"] = func(a *App, cx *gk.Cx) {
		gk.After(cx, 10*ms, a.askThenRelease)
		gk.After(cx, 20*ms, a.releaseIt)
		gk.After(cx, 30*ms, a.answerLate)
		gk.After(cx, 60*ms, a.checkLogCancel)
	}
	inits["race"] = func(a *App, cx *gk.Cx) {
		gk.After(cx, 10*ms, a.askThenRelease)
		gk.After(cx, 20*ms, a.race)
		gk.After(cx, 60*ms, a.checkLogCancel)
	}
	inits["view-released"] = func(a *App, cx *gk.Cx) {
		gk.After(cx, 5*ms, a.openChild)
		gk.After(cx, 20*ms, a.releaseChild)
		gk.After(cx, 30*ms, a.answerLate)
		gk.After(cx, 60*ms, a.checkLogCancel)
	}
	inits["menus"] = func(a *App, cx *gk.Cx) {
		gk.After(cx, 10*ms, a.setBar)
		gk.After(cx, 30*ms, a.readBar)
	}
	inits["elements"] = func(a *App, cx *gk.Cx) {
		gk.After(cx, 10*ms, a.elements)
		gk.After(cx, 40*ms, a.elementsDrawn)
		gk.After(cx, 60*ms, a.elementsGone)
	}
	views["elements"] = (*App).elementsView
	inits["drag"] = func(a *App, cx *gk.Cx) { gk.After(cx, 60*ms, a.dragDone) }
	views["drag"] = (*App).dragView
	inits["vlist"] = func(a *App, cx *gk.Cx) {
		a.vlistInit(cx)
		gk.After(cx, 60*ms, a.vlistCheck)
	}
	views["vlist"] = (*App).vlistView
	inits["cache-in-view"] = func(a *App, cx *gk.Cx) {} // misuse checked in the view
	views["cache-in-view"] = func(a *App, cx *gk.Cx) gk.Element {
		gk.NewImageCache(cx) // stops: a view function may not make an image cache
		return gk.VFlex(s.SizeFull)
	}
	inits["scope-zero"] = func(a *App, cx *gk.Cx) {}
	views["scope-zero"] = func(a *App, cx *gk.Cx) gk.Element {
		return gk.TextSelectionScopeOf(gk.Div(), gk.TextSelectionScope{}) // stops: no scope
	}
	inits["real-dialogs"] = func(a *App, cx *gk.Cx) {
		gk.After(cx, 100*ms, a.realAsk)
		gk.After(cx, 15000*ms, a.realGuard)
	}
	inits["real-url"] = func(a *App, cx *gk.Cx) {
		gk.After(cx, 100*ms, a.realURL)
		gk.After(cx, 15000*ms, a.realGuard)
	}
	inits["real-menus"] = func(a *App, cx *gk.Cx) {
		a.realMenusInit(cx)
		gk.After(cx, 15000*ms, a.realGuard)
	}
	views["real-menus"] = func(a *App, cx *gk.Cx) gk.Element {
		return gk.VFlex(s.SizeFull).Child(gk.Div().H(gk.Px(32)).OnChildrenPrepainted(a.realBarBounds).
			Child(gk.AppMenuBarView(ov.menubar)))
	}
}

// --- dialogs: answered later, answered before, a prompt, a cancel --------------------------------

func (a *App) opened(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	a.last = strings.Join(d.Paths, ",")
	fmt.Printf("platform opened n=%d status=%s paths=%s error=%q\n", a.done, doneName(d.Status), a.last, d.Error)
}

func (a *App) savedTo(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	fmt.Printf("platform saved n=%d status=%s paths=%s\n", a.done, doneName(d.Status), strings.Join(d.Paths, ","))
	gk.Release(cx, a.c2) // answered: releasing it does nothing
}

func (a *App) answered(cx *gk.Cx, d gk.PromptDone) {
	a.done++
	fmt.Printf("platform prompt n=%d status=%s answer=%d\n", a.done, doneName(d.Status), d.Answer)
}

func (a *App) dismissed(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	fmt.Printf("platform dismissed n=%d status=%s paths=%d\n", a.done, doneName(d.Status), len(d.Paths))
}

func (a *App) askOpen(cx *gk.Cx) {
	a.c1 = gk.PromptForPaths(cx, gk.PathPrompt{Files: true, Multiple: true, Prompt: "Open"}, a.opened)
	// the save dialog's answer was given before it was asked: it is taken at once
	took := gk.PlatformAnswerPaths(cx, gk.DoneAnswered, []string{"/tmp/new.txt"}, true, "")
	fmt.Printf("platform early answer taken=%v\n", took)
	a.c2 = gk.PromptForNewPath(cx, "/tmp", "new.txt", a.savedTo)
	a.c3 = gk.Prompt(cx, gk.PromptWarning, "Replace it?", "It exists", []gk.PromptButton{{Label: "Replace"}, {Kind: gk.ButtonCancel, Label: "Keep"}}, a.answered)
	a.c4 = gk.PromptForPaths(cx, gk.PathPrompt{Directories: true}, a.dismissed)
}

func (a *App) answerAll(cx *gk.Cx) {
	t1 := gk.PlatformAnswerPaths(cx, gk.DoneAnswered, []string{"/tmp/a.txt", "/tmp/b.txt"}, false, "")
	t2 := gk.PlatformAnswerPrompt(cx, gk.DoneAnswered, 1)
	t3 := gk.PlatformAnswerPaths(cx, gk.DoneCancelled, nil, false, "")
	fmt.Printf("platform answers taken=%v,%v,%v\n", t1, t2, t3)
}

func (a *App) printLog(cx *gk.Cx) {
	l := gk.GetPlatformLog(cx)
	fmt.Printf("platform log requests=%d delivered=%d discarded=%d waiting=%d scripted=%v done=%d\n",
		l.Requests, l.Delivered, l.Discarded, l.Waiting, l.Scripted, a.done)
}

func (a *App) checkLog(cx *gk.Cx) {
	a.printLog(cx)
	gk.Release(cx, a.c1) // answered already: nothing
}

func (a *App) checkLogCancel(cx *gk.Cx) { a.printLog(cx) }

// --- cancel: released before its answer ----------------------------------------------------------

func (a *App) askThenRelease(cx *gk.Cx) {
	a.c1 = gk.PromptForPaths(cx, gk.PathPrompt{Files: true}, a.never)
}

func (a *App) releaseIt(cx *gk.Cx) {
	gk.Release(cx, a.c1)
}

func (a *App) answerLate(cx *gk.Cx) {
	took := gk.PlatformAnswerPaths(cx, gk.DoneAnswered, []string{"/tmp/late.txt"}, false, "")
	fmt.Printf("platform late answer taken=%v\n", took)
}

// --- race: released and answered in the same turn (the answer arrives before the release lands) ----

func (a *App) race(cx *gk.Cx) {
	took := gk.PlatformAnswerPaths(cx, gk.DoneAnswered, []string{"/tmp/race.txt"}, false, "") // taken: delivered after this turn
	gk.Release(cx, a.c1)                                                                      // buffered: lands after this turn, after the answer was delivered
	fmt.Printf("platform race answer taken=%v\n", took)
}

// --- view-released: a request of a child view goes with the view ---------------------------------

func (a *App) childAsked(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	fmt.Println("platform child's request answered after its view was released (wrong)")
}

func (a *App) childInit(cx *gk.Cx) {
	gk.PromptForPaths(cx, gk.PathPrompt{Files: true}, a.childAsked)
	fmt.Println("platform child asked")
}

func (a *App) childView(cx *gk.Cx) gk.Element {
	return gk.Label("child")
}

func (a *App) openChild(cx *gk.Cx) {
	// a window's root view runs its init when the window opens
	a.childV = gk.NewView(a.childInit, a.childView)
	a.childW = gk.OpenWindow(cx, "child", 200, 100, a.childV)
}

func (a *App) releaseChild(cx *gk.Cx) {
	gk.CloseWindow(cx, a.childW)
	gk.Release(cx, a.childV)
	a.childV = gk.View{}
	fmt.Println("platform child released")
}

// --- menus and URLs -----------------------------------------------------------------------------

// osName prints an OsAction as Odin's %v does (the variant's name).
func osName(o gk.OsAction) string {
	names := []string{"None", "Cut", "Copy", "Paste", "Select_All", "Undo", "Redo"}
	if int(o) < len(names) {
		return names[o]
	}
	return fmt.Sprint(uint8(o))
}

func (a *App) setBar(cx *gk.Cx) {
	recent := []gk.MenuItem{gk.MenuAction{Name: "notes.txt", Action: a.open}}
	file := []gk.MenuItem{
		gk.MenuAction{Name: "Open", Action: a.open},
		gk.MenuSeparator{},
		gk.MenuSubmenu{Menu: gk.Menu{Name: "Recent", Items: recent}},
		gk.MenuServices{Name: "Services"},
	}
	edit := []gk.MenuItem{gk.MenuAction{Name: "Save", Action: a.save, OsAction: gk.OsActionCopy, Checked: true, Disabled: true}}
	gk.SetMenus(cx, gk.Menu{Name: "File", Items: file}, gk.Menu{Name: "Edit", Items: edit, Disabled: true})
	gk.OpenURL(cx, "https://example.com/gk")
}

func (a *App) readBar(cx *gk.Cx) {
	menus := gk.Menus(cx)
	fmt.Printf("platform menus n=%d\n", len(menus))
	for _, m := range menus {
		fmt.Printf("platform menu %q disabled=%v items=%d\n", m.Name, m.Disabled, len(m.Items))
		for _, it := range m.Items {
			switch x := it.(type) {
			case gk.MenuSeparator:
				fmt.Println("platform item separator")
			case gk.MenuAction:
				fmt.Printf("platform item action %q same=%v os=%s checked=%v disabled=%v\n", x.Name,
					x.Action == a.open || x.Action == a.save, osName(x.OsAction), x.Checked, x.Disabled)
			case gk.MenuSubmenu:
				fmt.Printf("platform item submenu %q items=%d\n", x.Menu.Name, len(x.Menu.Items))
			case gk.MenuServices:
				fmt.Printf("platform item services %q\n", x.Name)
			}
		}
	}
	l := gk.GetPlatformLog(cx)
	first := ""
	if len(l.URLs) > 0 {
		first = l.URLs[0]
	}
	fmt.Printf("platform urls n=%d first=%q\n", len(l.URLs), first)
}

// --- gpui's element leftovers (PROTOCOLS.md 13.31) ---------------------------------------

// joinOdin prints a slice as Odin's %v does: [2, 0, 1].
func joinOdin[T any](xs []T) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// elements: the decoders generated code uses, run by the library on the bytes the package wrote;
// the objects the library makes for the elements, made and released.
func (a *App) elements(cx *gk.Cx) {
	v1, f1 := gk.TestDropAccept(cx, gk.DropAccept{IDs: []uint64{7, 9}}, 7)
	v2, _ := gk.TestDropAccept(cx, gk.DropAccept{IDs: []uint64{7, 9}}, 8)
	v3, f3 := gk.TestDropAccept(cx, gk.DropAccept{Values: true, Paths: true}, 8)
	v4, _ := gk.TestDropAccept(cx, gk.DropAccept{}, 7)
	fmt.Printf("platform drop accept 7=%v 8=%v any=%v none=%v files=%v,%v\n", v1, v2, v3, v4, f1, f3)
	fs := gk.TestDragPayload(cx, gk.ExternalDragPayload{Files: []gk.ExternalFile{{Path: "/tmp/a.txt"}, {Path: "/tmp/dir", Directory: true}}})
	fmt.Printf("platform drag payload n=%d first=%s dir=%v,%v\n", len(fs), fs[0].Path, fs[0].Directory, fs[1].Directory)
	none := gk.TestDragPayload(cx, gk.ExternalDragPayload{})
	fmt.Printf("platform drag payload none n=%d\n", len(none))
	order := gk.TestPrepaintOrder(cx, []int{2, 0, 1})
	fmt.Printf("platform prepaint order=%s\n", joinOdin(order))
	red := gk.NewRenderImage(cx, gk.PixelImage{Width: 2, Height: 1, RGBA: []byte{255, 0, 0, 255, 0, 0, 255, 255}})
	w, h, first, id := gk.TestRenderImage(cx, red, false)
	cw, ch, cfirst, cid := gk.TestRenderImage(cx, red, true)
	fmt.Printf("platform pixel image %dx%d first=%08x custom %dx%d first=%08x same=%v\n", w, h, first, cw, ch, cfirst, id == cid)
	gk.Release(cx, red)
	// the image elementsView shows on every frame: one gpui image, its id kept (PROTOCOLS.md 13.31)
	a.pic = gk.NewRenderImage(cx, gk.PixelImage{Width: 2, Height: 2, RGBA: []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255}})
	_, _, _, a.picID = gk.TestRenderImage(cx, a.pic, false)
	a.shown0, a.images0, a.dropped0 = gk.TestImagesShown(cx)
	a.ticks = gk.Every(cx, 5*ms, a.elementsTick) // the view draws again on every tick
	if e, ok := gk.TestFileDropEntered(cx).(gk.FileDropEventEntered); ok {
		fmt.Printf("platform file drop entered at=%.0f,%.0f paths=%s\n", e.Position.X, e.Position.Y, strings.Join(e.Paths, ","))
	}
	// Sequence.WithStep: a Transition node in a call op's builder chain (the rule of 9.31)
	seq := gk.NewSequence(cx, "seq", 10).WithStep(40, gk.NewTransition(200*time.Millisecond)).WithStep(80, gk.NewTransition(100*time.Millisecond)).Build()
	steps := seq.Steps(cx)
	fmt.Printf("platform sequence steps=%d from=%.0f targets=%.0f,%.0f\n", len(steps), seq.From(cx), steps[0].Target(cx), steps[1].Target(cx))
	gk.Release(cx, seq)
	ov.cache = gk.NewImageCache(cx)
	a.scroll = gk.NewScrollHandle(cx)
	ov.anchor = gk.NewScrollAnchor(cx, a.scroll)
	ov.scope = gk.NewTextSelectionScopeID(cx)
	fmt.Printf("platform objects cache=%v anchor=%v scope=%v\n", ov.cache != gk.ImageCache{}, ov.anchor != gk.ScrollAnchor{},
		ov.scope != gk.TextSelectionScope{})
}

func (a *App) elementsDrawn(cx *gk.Cx) {
	// drawn again and again since elements: gpui was handed the one image each time (one texture)
	times, images, _ := gk.TestImagesShown(cx)
	_, _, _, id := gk.TestRenderImage(cx, a.pic, false)
	fmt.Printf("platform image kept redrawn=%v same=%v new images=%d\n", times-a.shown0 >= 2, id == a.picID, images-a.images0)
	gk.Release(cx, a.pic)
	gk.Release(cx, a.ticks)
	a.pic = gk.RenderImage{}
	gk.ScrollAnchorTo(cx, ov.anchor)
	gk.Release(cx, ov.anchor)
	gk.Release(cx, ov.cache)
	gk.Release(cx, ov.scope)
	ov.cache = gk.ImageCache{} // the view shows none of them from now on
	fmt.Printf("platform elements released described=%d\n", a.described)
}

// After the release: its texture dropped from the window's atlas (Window::drop_image, one window).
func (a *App) elementsGone(cx *gk.Cx) {
	_, _, dropped := gk.TestImagesShown(cx)
	fmt.Printf("platform image released dropped=%d\n", dropped-a.dropped0)
}

func (a *App) elementsTick(cx *gk.Cx) {
	a.fired++
}

// The elements of 13.31 drawn (a mode's view): a scope, a cache on a div and an img, an anchored
// scrolled child, synthetic accessibility nodes, a prepaint order, a drag's files.
func (a *App) elementsView(cx *gk.Cx) gk.Element {
	if ov.cache == (gk.ImageCache{}) {
		return gk.Label("waiting")
	}
	pic := gk.Div().W(gk.Px(20)).H(gk.Px(20)) // the image, until it is released
	if a.pic != (gk.RenderImage{}) {
		pic = pic.Child(gk.Img(gk.ImageSourceRender{Value: a.pic}).W(gk.Px(20)).H(gk.Px(20)).ImageCache(ov.cache))
	}
	inner := gk.Div().ID("anchored").AnchorScroll(gk.Some(ov.anchor)).
		A11ySyntheticChildren([]gk.A11yChild{{Key: "q1", Role: gk.RoleListItem, Label: "first"}, {Key: "q2", Role: gk.RoleListItem, Label: "second"}}).
		Child(gk.Label("anchored"))
	box := gk.Div().ImageCache(ov.cache).WithDynamicPrepaintOrder([]int{1, 0}).Child(pic).Child(inner)
	files := gk.Div().ID("files").OnDrag(gk.DragValue{ID: 3, Text: "three"}, gk.View{}).
		ExternalDragPayload(gk.ExternalDragPayload{Files: []gk.ExternalFile{{Path: "/tmp/a.txt"}}}).Child(gk.Label("files"))
	return gk.VFlex(s.SizeFull).Child(gk.TextSelectionScopeOf(box, ov.scope)).Child(files)
}

// drag: a drag of value 7 onto a target that takes 7 (its drop runs, its drag-move handler sees the
// value), then onto one that takes only 8 (no drop); a group's drag-over style.
func (a *App) dropped7(cx *gk.Cx, d gk.DragValue) {
	a.drops++
	fmt.Printf("platform drop on seven id=%d text=%s drops=%d\n", d.ID, d.Text, a.drops)
}

func (a *App) dropped8(cx *gk.Cx, d gk.DragValue) {
	a.drops++
	fmt.Printf("platform drop on eight id=%d (wrong: it takes only 8)\n", d.ID)
}

func (a *App) moved(cx *gk.Cx, e gk.DragMoveEvent) {
	a.moves++
	if a.moves == 1 {
		fmt.Printf("platform drag move id=%d text=%s bounds=%.0fx%.0f at x>0=%v\n", e.Value.ID, e.Value.Text,
			e.Bounds.Size.Width, e.Bounds.Size.Height, e.Position[0] > 0)
	}
}

func (a *App) dragView(cx *gk.Cx) gk.Element {
	src := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("src").OnDrag(gk.DragValue{ID: 7, Text: "seven"}, gk.View{}).Child(gk.Label("seven"))
	t7 := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("t7").Group("targets").CanDrop(gk.DropAccept{IDs: []uint64{7}}).OnDrop(a.dropped7).
		OnDragMove(a.moved).GroupDragOver("targets", gk.NewStyleRefinementDefault().Bg(theme.Accent))
	t8 := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("t8").CanDrop(gk.DropAccept{IDs: []uint64{8}}).OnDrop(a.dropped8)
	return gk.HFlex().Child(src).Child(t7).Child(t8)
}

func (a *App) dragDone(cx *gk.Cx) {
	fmt.Printf("platform drag done drops=%d moved=%v\n", a.drops, a.moves > 0)
}

// vlist: a vertical and a horizontal virtual list over a row source: rows drawn as placeholders for
// one frame, then described (only those in view).
func (a *App) vlistRow(cx *gk.Cx, at gk.RowAt) gk.Element {
	a.rowViews++
	return gk.Label(fmt.Sprintf("row %d", at.Row))
}

func (a *App) vlistInit(cx *gk.Cx) {
	a.rows = gk.NewRowSource(cx, 1000, a.vlistRow, gk.Initial(0))
}

func (a *App) vlistView(cx *gk.Cx) gk.Element {
	sizes := make([][2]float32, 1000)
	for i := range sizes {
		sizes[i] = [2]float32{80, float32(20 + (i%3)*10)}
	}
	return gk.VFlex(s.SizeFull).
		Child(gk.VVirtualList("v", a.rows, sizes).H(gk.Px(200))).
		Child(gk.HVirtualList("h", a.rows, sizes[:10]).H(gk.Px(40)))
}

func (a *App) vlistCheck(cx *gk.Cx) {
	fmt.Printf("platform vlist rows described=%v of 1000 some=%v\n", a.rowViews < 1000, a.rowViews > 0)
}

// --- the real platform, not the script (PROTOCOLS.md 11.32) -------------------------------
//
// Run on a real window (gk.Run) under gates/display.sh: a private X server or compositor with its own
// D-Bus session bus that has no service files, so no xdg-desktop-portal answers; bin/platform-real-go
// puts a fake xdg-open first on PATH that writes its arguments to $GK_URL_LOG. A guard timer quits
// after 15 s, so a request that never answers fails the check instead of hanging the run.

func (a *App) realGuard(cx *gk.Cx) {
	fmt.Printf("platform real guard: still waiting after 15 s (done=%d)\n", a.done)
	gk.Quit(cx)
}

func (a *App) realAsk(cx *gk.Cx) {
	a.askedAt = time.Now()
	a.c1 = gk.PromptForPaths(cx, gk.PathPrompt{Files: true, Prompt: "Open"}, a.realAnswered)
	a.c2 = gk.PromptForNewPath(cx, "/tmp", "new.txt", a.realAnswered)
	fmt.Println("platform real asked")
}

func (a *App) realAnswered(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	ms := time.Since(a.askedAt).Milliseconds()
	fmt.Printf("platform real answer n=%d status=%s paths=%d error=%q in_time=%v\n", a.done, doneName(d.Status), len(d.Paths), d.Error, ms < 10000)
	if a.done == 2 {
		a.printLog(cx)
		gk.Quit(cx)
	}
}

func (a *App) realURL(cx *gk.Cx) {
	gk.OpenURL(cx, "https://example.com/gk-real?q=1")
	gk.After(cx, 100*ms, a.realURLPoll)
}

// The system opener runs on gpui's background executor: the app looks for what it wrote.
func (a *App) realURLPoll(cx *gk.Cx) {
	a.polls++
	path := os.Getenv("GK_URL_LOG")
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		l := gk.GetPlatformLog(cx)
		fmt.Printf("platform real url opener got %s", string(data))
		fmt.Printf("platform real url recorded=%d scripted=%v\n", len(l.URLs), l.Scripted)
		gk.Quit(cx)
		return
	}
	if a.polls < 50 {
		gk.After(cx, 100*ms, a.realURLPoll)
	} else {
		fmt.Printf("platform real url: the opener wrote nothing to %q in 5 s\n", path)
		gk.Quit(cx)
	}
}

func (a *App) realMenusInit(cx *gk.Cx) {
	file := []gk.MenuItem{gk.MenuAction{Name: "Open", Action: a.open}, gk.MenuSeparator{}, gk.MenuAction{Name: "Save", Action: a.save}}
	help := []gk.MenuItem{gk.MenuAction{Name: "About", Action: a.open}}
	gk.SetMenus(cx, gk.Menu{Name: "File", Items: file}, gk.Menu{Name: "Help", Items: help})
	ov.menubar = gk.NewAppMenuBar(cx)
}

// The menu bar's bounds when it is laid out in a real window's frame (OnChildrenPrepainted of the
// div that holds it): it is drawn though the platform has no menu bar of its own.
func (a *App) realBarBounds(cx *gk.Cx, bs []gk.Bounds) {
	if a.barSeen || len(bs) == 0 {
		return
	}
	a.barSeen = true
	menus := gk.Menus(cx)
	first := ""
	if len(menus) > 0 {
		first = menus[0].Name
	}
	fmt.Printf("platform real menubar drawn w>100=%v h>10=%v menus=%d first=%q\n", bs[0].Size.Width > 100, bs[0].Size.Height > 10,
		len(menus), first)
	gk.After(cx, 50*ms, func(cx *gk.Cx) { gk.Quit(cx) })
}
