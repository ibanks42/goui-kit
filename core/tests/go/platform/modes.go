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

// ov stores retained image cache, scroll anchor, selection scope, and menu bar
// handles used by the platform element and real-menu test modes.
var ov struct {
	cache   gk.ImageCache
	anchor  gk.ScrollAnchor
	scope   motion.TextSelectionScope
	menubar kit.AppMenuBar
}

func init() {
	// Register scripted dialog, cancellation/race, menu, element/image, drag,
	// virtual-list, misuse, and real-display (`real-*`) protocol modes.
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
	inits["cache-in-view"] = func(a *App, cx *gk.Cx) {}
	views["cache-in-view"] = func(a *App, cx *gk.Cx) gk.Element {
		gk.NewImageCache(cx)
		return gk.VFlex(s.SizeFull)
	}
	inits["scope-zero"] = func(a *App, cx *gk.Cx) {}
	views["scope-zero"] = func(a *App, cx *gk.Cx) gk.Element {
		return motion.TextSelectionScopeOf(gk.Div(), motion.TextSelectionScope{})
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
			Child(kit.AppMenuBarView(ov.menubar)))
	}
}

func (a *App) opened(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	a.last = strings.Join(d.Paths, ",")
	fmt.Printf("platform opened n=%d status=%s paths=%s error=%q\n", a.done, doneName(d.Status), a.last, d.Error)
}

func (a *App) savedTo(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	fmt.Printf("platform saved n=%d status=%s paths=%s\n", a.done, doneName(d.Status), strings.Join(d.Paths, ","))
	gk.Release(cx, a.c2)
}

func (a *App) answered(cx *gk.Cx, d gk.PromptDone) {
	a.done++
	fmt.Printf("platform prompt n=%d status=%s answer=%d\n", a.done, doneName(d.Status), d.Answer)
}

func (a *App) dismissed(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	fmt.Printf("platform dismissed n=%d status=%s paths=%d\n", a.done, doneName(d.Status), len(d.Paths))
}

// askOpen proves scripted platform dialogs queue 4 requests (open, save, prompt,
// directory), deliver a pre-queued answer immediately to save (`taken=false` on
// the early-queue call because no save prompt was waiting yet), and settle all 4
// with waiting=0 when answerAll responds to the remaining 3.
func (a *App) askOpen(cx *gk.Cx) {
	a.c1 = gk.PromptForPaths(cx, gk.PathPrompt{Files: true, Multiple: true, Prompt: "Open"}, a.opened)

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
	gk.Release(cx, a.c1)
}

func (a *App) checkLogCancel(cx *gk.Cx) { a.printLog(cx) }

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

// race proves that releasing a Completion in the same turn right after an
// answer is queued still suppresses the callback (done=0) even though the
// scripted queue marked the answer delivered (taken=true, delivered=1).
func (a *App) race(cx *gk.Cx) {
	took := gk.PlatformAnswerPaths(cx, gk.DoneAnswered, []string{"/tmp/race.txt"}, false, "")
	gk.Release(cx, a.c1)
	fmt.Printf("platform race answer taken=%v\n", took)
}

func (a *App) childAsked(cx *gk.Cx, d gk.PathsDone) {
	a.done++
	fmt.Println("platform child's request answered after its view was released (wrong)")
}

func (a *App) childInit(cx *gk.Cx) {
	gk.PromptForPaths(cx, gk.PathPrompt{Files: true}, a.childAsked)
	fmt.Println("platform child asked")
}

func (a *App) childView(cx *gk.Cx) gk.Element {
	return kit.Label("child")
}

func (a *App) openChild(cx *gk.Cx) {

	a.childV = gk.NewView(a.childInit, a.childView)
	a.childW = gk.OpenWindow(cx, "child", 200, 100, a.childV)
}

func (a *App) releaseChild(cx *gk.Cx) {
	gk.CloseWindow(cx, a.childW)
	gk.Release(cx, a.childV)
	a.childV = gk.View{}
	fmt.Println("platform child released")
}

func osName(o gk.OsAction) string {
	names := []string{"None", "Cut", "Copy", "Paste", "Select_All", "Undo", "Redo"}
	if int(o) < len(names) {
		return names[o]
	}
	return fmt.Sprint(uint8(o))
}

// setBar proves SetMenus preserves nested submenus, separators, services, OS
// action roles, and checked/disabled flags across readback in readBar, and
// proves OpenURL records the URL in the scripted platform log.
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

func joinText[T any](xs []T) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// elements proves drop-accept predicates (7 accepted, 8 rejected), external
// file drag payloads, dynamic prepaint order [2, 0, 1], RGBA->BGRA pixel image
// conversion (first=ffff0000), image-cache reuse across redraws (new images=0),
// and GPU texture drop (dropped=1) when the RenderImage is released.
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
	fmt.Printf("platform prepaint order=%s\n", joinText(order))
	red := gk.NewRenderImage(cx, gk.PixelImage{Width: 2, Height: 1, RGBA: []byte{255, 0, 0, 255, 0, 0, 255, 255}})
	w, h, first, id := gk.TestRenderImage(cx, red, false)
	cw, ch, cfirst, cid := gk.TestRenderImage(cx, red, true)
	fmt.Printf("platform pixel image %dx%d first=%08x custom %dx%d first=%08x same=%v\n", w, h, first, cw, ch, cfirst, id == cid)
	gk.Release(cx, red)

	a.pic = gk.NewRenderImage(cx, gk.PixelImage{Width: 2, Height: 2, RGBA: []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255}})
	_, _, _, a.picID = gk.TestRenderImage(cx, a.pic, false)
	a.shown0, a.images0, a.dropped0 = gk.TestImagesShown(cx)
	a.ticks = gk.Every(cx, 5*ms, a.elementsTick)
	if e, ok := gk.TestFileDropEntered(cx).(gk.FileDropEventEntered); ok {
		fmt.Printf("platform file drop entered at=%.0f,%.0f paths=%s\n", e.Position.X, e.Position.Y, strings.Join(e.Paths, ","))
	}

	seq := motion.NewSequence(cx, "seq", 10).WithStep(40, motion.NewTransition(200*time.Millisecond)).WithStep(80, motion.NewTransition(100*time.Millisecond)).Build()
	steps := seq.Steps(cx)
	fmt.Printf("platform sequence steps=%d from=%.0f targets=%.0f,%.0f\n", len(steps), seq.From(cx), steps[0].Target(cx), steps[1].Target(cx))
	gk.Release(cx, seq)
	ov.cache = gk.NewImageCache(cx)
	a.scroll = gk.NewScrollHandle(cx)
	ov.anchor = gk.NewScrollAnchor(cx, a.scroll)
	ov.scope = motion.NewTextSelectionScopeID(cx)
	fmt.Printf("platform objects cache=%v anchor=%v scope=%v\n", ov.cache != gk.ImageCache{}, ov.anchor != gk.ScrollAnchor{},
		ov.scope != motion.TextSelectionScope{})
}

func (a *App) elementsDrawn(cx *gk.Cx) {

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
	ov.cache = gk.ImageCache{}
	fmt.Printf("platform elements released described=%d\n", a.described)
}

func (a *App) elementsGone(cx *gk.Cx) {
	_, _, dropped := gk.TestImagesShown(cx)
	fmt.Printf("platform image released dropped=%d\n", dropped-a.dropped0)
}

func (a *App) elementsTick(cx *gk.Cx) {
	a.fired++
}

func (a *App) elementsView(cx *gk.Cx) gk.Element {
	if ov.cache == (gk.ImageCache{}) {
		return kit.Label("waiting")
	}
	pic := gk.Div().W(gk.Px(20)).H(gk.Px(20))
	if a.pic != (gk.RenderImage{}) {
		pic = pic.Child(gk.Img(gk.ImageSourceRender{Value: a.pic}).W(gk.Px(20)).H(gk.Px(20)).ImageCache(ov.cache))
	}
	inner := gk.Div().ID("anchored").AnchorScroll(gk.Some(ov.anchor)).
		A11ySyntheticChildren([]gk.A11yChild{{Key: "q1", Role: gk.RoleListItem, Label: "first"}, {Key: "q2", Role: gk.RoleListItem, Label: "second"}}).
		Child(kit.Label("anchored"))
	box := gk.Div().ImageCache(ov.cache).WithDynamicPrepaintOrder([]int{1, 0}).Child(pic).Child(inner)
	files := gk.Div().ID("files").OnDrag(gk.DragValue{ID: 3, Text: "three"}, gk.View{}).
		ExternalDragPayload(gk.ExternalDragPayload{Files: []gk.ExternalFile{{Path: "/tmp/a.txt"}}}).Child(kit.Label("files"))
	return gk.VFlex(s.SizeFull).Child(motion.TextSelectionScopeOf(box, ov.scope)).Child(files)
}

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

// dragView lays out a draggable source (`id=7`) beside one matching target (`t7`) and one rejecting target (`t8`).
func (a *App) dragView(cx *gk.Cx) gk.Element {
	src := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("src").OnDrag(gk.DragValue{ID: 7, Text: "seven"}, gk.View{}).Child(kit.Label("seven"))
	t7 := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("t7").Group("targets").CanDrop(gk.DropAccept{IDs: []uint64{7}}).OnDrop(a.dropped7).
		OnDragMove(a.moved).GroupDragOver("targets", gk.NewStyleRefinementDefault().Bg(theme.Accent))
	t8 := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("t8").CanDrop(gk.DropAccept{IDs: []uint64{8}}).OnDrop(a.dropped8)
	return gk.HFlex().Child(src).Child(t7).Child(t8)
}

func (a *App) dragDone(cx *gk.Cx) {
	fmt.Printf("platform drag done drops=%d moved=%v\n", a.drops, a.moves > 0)
}

func (a *App) vlistRow(cx *gk.Cx, at gk.RowAt) gk.Element {
	a.rowViews++
	return kit.Label(fmt.Sprintf("row %d", at.Row))
}

func (a *App) vlistInit(cx *gk.Cx) {
	a.rows = gk.NewRowSource(cx, 1000, a.vlistRow, gk.Initial(0))
}

// vlistView renders both vertical and horizontal virtual lists over a 1000-row source to confirm lazy paging.
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

func (a *App) realGuard(cx *gk.Cx) {
	fmt.Printf("platform real guard: still waiting after 15 s (done=%d)\n", a.done)
	gk.Quit(cx)
}

// realAsk proves unscripted OS file dialogs under headless Xvfb (no portal)
// report DoneFailed promptly (<10s) with an error message rather than hanging.
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

// realURL proves unscripted OpenURL spawns the PATH xdg-open wrapper with the
// exact URL instead of recording it in the scripted log (recorded=0, scripted=false).
func (a *App) realURL(cx *gk.Cx) {
	gk.OpenURL(cx, "https://example.com/gk-real?q=1")
	gk.After(cx, 100*ms, a.realURLPoll)
}

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

// realMenusInit proves SetMenus + AppMenuBarView lays out a real menu bar on
// Xvfb with positive bounds (w>100, h>10) and exposes both registered menus.
func (a *App) realMenusInit(cx *gk.Cx) {
	file := []gk.MenuItem{gk.MenuAction{Name: "Open", Action: a.open}, gk.MenuSeparator{}, gk.MenuAction{Name: "Save", Action: a.save}}
	help := []gk.MenuItem{gk.MenuAction{Name: "About", Action: a.open}}
	gk.SetMenus(cx, gk.Menu{Name: "File", Items: file}, gk.Menu{Name: "Help", Items: help})
	ov.menubar = kit.NewAppMenuBar(cx)
}

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
