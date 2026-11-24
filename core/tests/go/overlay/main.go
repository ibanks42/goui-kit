// overlay: the protocol tests of the overlays family (docs/PROTOCOLS.md section 11.6), the port of
// core/tests/overlay: a dialog, an alert and a sheet opened from handlers with view-built functions
// (gk.NewBuilt), described again while open, closed by the keyboard and by the app, opened again on
// the same view; a questionnaire (a state whose constructor and updates can fail). Each mode runs
// offscreen (the diag library) under a gk-meter script and prints lines
// core/tests/go/run-overlay.sh checks.
//
//	GK_PROTO=dialog | alert | sheet | questionnaire | release | wrong-root | questionnaire-dup
//	(and the further modes below: dock, ..., popover, observe, values, menu-redraw, menu-redraw-b)
//
// The modes of the dock's node ids, saved layouts, pane tree and renderer, the nav stack's views,
// the toast manager's clock and a fallible value call are in docks.go.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/base"
	gkdock "gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
)

const ms = time.Millisecond

type App struct {
	mode string
	// menu-redraw: the view's redraws while its menu is open, and the clicks that reached item
	// Alpha's handler and item Bravo's action
	menuTicks    int
	menuA, menuB int
	menuAct      gk.Action
	root         gk.View
	count        int // shown in the dialog; bumped while it is open
	described    int // runs of the dialog's function
	cancels      int
	oks          int
	closes       int
	first        gk.Built[overlay.DialogEl]
	sheet        gk.Built[overlay.SheetEl]
	q            base.QuestionnaireState
	// the dock
	dock   gkdock.Area
	panels [2]gkdock.Panel
	bodies [2]*Body
	// popover
	popoverOpen    bool
	popoverChanges int
	// views family: observations
	obsX      float32
	positions int
	lastPos   overlay.ResolvedPosition
	pops      int
	lastPop   overlay.Event117
	frames5   int
}

// Body is a panel's (or a page's) view: its text, and how often it was described.
type Body struct {
	text      string
	described int
}

func (b *Body) view(cx *gk.Cx) gk.Element {
	b.described++
	return kit.Label(b.text)
}

// step: a function a timer runs at a time after init.
type step struct {
	at time.Duration
	f  func(a *App, cx *gk.Cx)
}

// steps: what a mode's init starts, by mode; roots: the root view of a mode that draws more than
// the default. The modes of docks.go register from it.
var (
	steps = map[string][]step{}
	inits = map[string]func(a *App, cx *gk.Cx){}
	roots = map[string]func(a *App, cx *gk.Cx) gk.Element{}
)

// --- the dialog --------------------------------------------------------------------------------

func (a *App) confirmDialog(cx *gk.Cx) overlay.DialogEl {
	a.described++
	fmt.Printf("overlay dialog described n=%d count=%d\n", a.described, a.count)
	seen := a.count // the count of this description: which tape the kit used
	return overlay.Dialog().
		Title(kit.Label("Delete?")).
		Child(kit.Label(fmt.Sprintf("count %d", a.count))).
		OnCancel(func(cx *gk.Cx, _ gk.ClickEvent) bool {
			a.cancels++
			fmt.Printf("overlay dialog cancel n=%d seen=%d count=%d\n", a.cancels, seen, a.count)
			return true // closes it
		})
}

func openFirst(a *App, cx *gk.Cx) {
	a.first = gk.NewBuilt(cx, a.confirmDialog)
	overlay.OpenDialog(cx, a.first)
	fmt.Println("overlay dialog opened")
}

func bump(a *App, cx *gk.Cx) {
	fmt.Printf("overlay dialog active=%v\n", overlay.HasActiveDialog(cx))
	a.count++
	gk.Notify(cx, a.first.View) // described again while open
}

// Go: gk.NewBuilt makes a new view on every call; the app keeps the one it made and opens it again, so "again" is the
// kept Built, the same view the dialog was first opened on.
func reopen(a *App, cx *gk.Cx) {
	again := a.first
	fmt.Printf("overlay dialog closed active=%v; again same=%v\n", overlay.HasActiveDialog(cx), again.View == a.first.View)
	overlay.OpenDialog(cx, again)
}

func closeByApp(a *App, cx *gk.Cx) {
	overlay.CloseDialog(cx)
}

func dialogDone(a *App, cx *gk.Cx) {
	fmt.Printf("overlay done mode=dialog active=%v described=%d cancels=%d count=%d\n",
		overlay.HasActiveDialog(cx), a.described, a.cancels, a.count)
}

// --- the alert ---------------------------------------------------------------------------------

func (a *App) alert(cx *gk.Cx) overlay.AlertDialogEl {
	return overlay.AlertDialog().Title(kit.Label("Unsaved")).Description(kit.Label("Leave?")).ShowCancel(true).
		OnOk(func(cx *gk.Cx, _ gk.ClickEvent) bool {
			a.oks++
			fmt.Printf("overlay alert ok n=%d\n", a.oks)
			return true
		})
}

func openAlert(a *App, cx *gk.Cx) {
	overlay.OpenAlertDialog(cx, gk.NewBuilt(cx, a.alert))
	fmt.Println("overlay alert opened")
}

func alertDone(a *App, cx *gk.Cx) {
	fmt.Printf("overlay done mode=alert active=%v oks=%d\n", overlay.HasActiveDialog(cx), a.oks)
}

// --- the sheet ---------------------------------------------------------------------------------

func (a *App) settings(cx *gk.Cx) overlay.SheetEl {
	fmt.Println("overlay sheet described")
	return overlay.Sheet().Title(kit.Label("Settings")).Child(kit.Label("body")).OnClose(func(cx *gk.Cx, _ gk.ClickEvent) {
		a.closes++
		fmt.Printf("overlay sheet on_close n=%d\n", a.closes)
	})
}

func openSheet(a *App, cx *gk.Cx) {
	a.sheet = gk.NewBuilt(cx, a.settings)
	overlay.OpenSheetAt(cx, gk.PlacementLeft, a.sheet)
}

func sheetCheck(a *App, cx *gk.Cx) {
	fmt.Printf("overlay sheet active=%v\n", overlay.HasActiveSheet(cx))
	overlay.CloseSheet(cx)
}

func sheetDone(a *App, cx *gk.Cx) {
	fmt.Printf("overlay done mode=sheet active=%v\n", overlay.HasActiveSheet(cx))
}

// --- misuse: a released view, a function that returns another node ----------------------------

func releaseFirst(a *App, cx *gk.Cx) {
	gk.Release(cx, a.first.View) // the dialog is still open: its next build stops
	fmt.Println("overlay released the open dialog's view")
	overlay.OpenSheet(cx, gk.NewBuilt(cx, a.settings)) // the window's layers are built again
}

// Go: a view-built function's type is checked by the compiler; a Built made around another view
// (a plain view whose root is a div) compiles, and the library refuses it.
func openWrong(a *App, cx *gk.Cx) {
	wrong := gk.NewView(nil, func(cx *gk.Cx) gk.Element { return gk.Div().Child(kit.Label("not a dialog")) })
	overlay.OpenDialog(cx, gk.Built[overlay.DialogEl]{View: wrong})
}

// --- the questionnaire ---------------------------------------------------------------------------

func items(dup bool) []base.QuestionnaireItemDefinition {
	color := base.NewQuestionnaireItemDefinition("color", "Favourite colour").
		WithChoice(base.NewQuestionnaireChoiceDefinition("red", "Red")).
		WithChoice(base.NewQuestionnaireChoiceDefinition("blue", "Blue"))
	name := "size"
	if dup {
		name = "color"
	}
	size := base.NewQuestionnaireItemDefinition(name, "Size").
		WithChoice(base.NewQuestionnaireChoiceDefinition("s", "Small")).
		WithChoice(base.NewQuestionnaireChoiceDefinition("l", "Large"))
	return []base.QuestionnaireItemDefinition{color, size}
}

// errText is an error's text ("" for none), as the checks expect.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func questionnaire(a *App, cx *gk.Cx) {
	a.q = base.NewQuestionnaireState(cx, items(a.mode == "questionnaire-dup")).Build()
	cur, _ := a.q.CurrentItem(cx).Get()
	fmt.Printf("overlay q made total=%d current=%s\n", a.q.Total(cx), cur)
	err := a.q.Choose(cx, "color", "green")
	fmt.Printf("overlay q choose unknown failed=%v err=%s\n", err != nil, errText(err))
	err2 := a.q.Choose(cx, "color", "blue")
	fmt.Printf("overlay q choose blue failed=%v\n", err2 != nil)
	moved := a.q.GoNext(cx)
	cur2, _ := a.q.CurrentItem(cx).Get()
	fmt.Printf("overlay q next moved=%v current=%s\n", moved, cur2)
	err3 := a.q.SetCurrentItem(cx, "nope")
	fmt.Printf("overlay q set_current_item unknown failed=%v\n", err3 != nil)
	fmt.Printf("overlay done mode=questionnaire complete=%v\n", a.q.IsComplete(cx))
}

// --- the dock: two Go views as panels ---------------------------------------------------------

// panelKind prints a PanelEventKind as the checks expect (the variant's name).
func panelKind(k gkdock.PanelEventKind) string {
	names := []string{"Active", "Zoomed", "Added", "Removed"}
	if int(k) < len(names) {
		return names[k]
	}
	return fmt.Sprint(uint8(k))
}

func (a *App) panelEvent(cx *gk.Cx, e gkdock.PanelEvent, which int) {
	fmt.Printf("overlay panel %d event %s %v\n", which, panelKind(e.Kind), e.Flag)
}

func dockMake(a *App, cx *gk.Cx) {
	a.bodies = [2]*Body{{text: "first"}, {text: "second"}}
	a.dock = gkdock.NewAreaSkinned(cx, "test")
	for i := range 2 {
		name := "second"
		if i == 0 {
			name = "first"
		}
		v := gk.NewView(nil, a.bodies[i].view)
		a.panels[i] = gkdock.NewPanel(cx, v, gkdock.PanelData{Name: name, Title: a.bodies[i].text},
			gkdock.PanelEvents(func(cx *gk.Cx, e gkdock.PanelEvent) { a.panelEvent(cx, e, i) }))
	}
	a.dock.SetCenter(cx, gkdock.NewLayoutTabs().PanelView(a.panels[0]).PanelView(a.panels[1]))
	fmt.Println("overlay dock made")
}

func dockCheck(a *App, cx *gk.Cx) {
	fmt.Printf("overlay dock described first=%d second=%d empty=%v\n", a.bodies[0].described, a.bodies[1].described,
		a.dock.IsEmpty(cx, gkdock.PlacementCenter))
	gkdock.SetPanel(cx, a.panels[0], gkdock.PanelData{Name: "first", Title: "renamed", NotClosable: true})
	gkdock.EmitPanel(cx, a.panels[0], gkdock.PanelZoomIn)
}

func dockZoomed(a *App, cx *gk.Cx) {
	fmt.Printf("overlay dock zoomed=%v\n", a.dock.IsZoomed(cx))
	a.dock.RemovePanel(cx, a.panels[1])
}

func dockDone(a *App, cx *gk.Cx) {
	fmt.Printf("overlay done mode=dock zoomed=%v\n", a.dock.IsZoomed(cx))
}

func (a *App) dockRoot(cx *gk.Cx) gk.Element {
	if a.dock == (gkdock.Area{}) {
		return gk.Div(s.SizeFull)
	}
	return gk.Div(s.SizeFull).Child(gkdock.View(a.dock))
}

// --- a real window on a display (an X server such as Xvfb): title and size ------------------------
// Offscreen every window has the meter's size and the headless platform keeps no title, so the
// app tests could only read them; here the X server holds them. GPUI reads a window's title back
// on macOS only (gpui-fast's get_title is empty elsewhere), so run-overlay.sh asks the X server
// (xwininfo) while the app runs; the size comes back through gk.GetWindowInfo.

func (a *App) realView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(kit.Label("a real window"))
}

func (a *App) realRead(cx *gk.Cx) {
	i := gk.GetWindowInfo(cx, gk.Windows(cx)[0])
	fmt.Printf("overlay window title=%q w=%d h=%d\n", i.Title, int(i.Width), int(i.Height))
	gk.SetWindowTitle(cx, gk.Windows(cx)[0], "Renamed by the app")
}

func (a *App) realAgain(cx *gk.Cx) {
	i := gk.GetWindowInfo(cx, gk.Windows(cx)[0])
	fmt.Printf("overlay window again title=%q w=%d h=%d\n", i.Title, int(i.Width), int(i.Height))
	gk.Quit(cx)
}

func (a *App) realInit(cx *gk.Cx) {
	gk.After(cx, 1000*ms, a.realRead)
	gk.After(cx, 2000*ms, a.realAgain)
}

func (a *App) realStart(cx *gk.Cx) {
	a.root = gk.NewView(a.realInit, a.realView)
	gk.OpenWindow(cx, "Go on the kit", 420, 300, a.root)
}

// --- PROTOCOLS.md 11.12: a popover opened and closed from code ----------------------------

func (a *App) popoverChanged(cx *gk.Cx, open bool) {
	a.popoverChanges++
	a.popoverOpen = open
}

func (a *App) popoverRoot(cx *gk.Cx) gk.Element {
	p := overlay.Popover("pop").Trigger(kit.Button("trigger").Label("Open")).Open(a.popoverOpen).
		OnOpenChange(a.popoverChanged).Child(kit.Label("inside the popover"))
	return gk.Div(s.SizeFull).Child(p)
}

func popoverOpen(a *App, cx *gk.Cx) {
	a.popoverOpen = true
	fmt.Println("overlay popover opened by the app")
}

func popoverClose(a *App, cx *gk.Cx) {
	fmt.Printf("overlay popover open=%v\n", a.popoverOpen)
	a.popoverOpen = false
}

func popoverDone(a *App, cx *gk.Cx) {
	fmt.Printf("overlay done mode=popover open=%v changes=%d\n", a.popoverOpen, a.popoverChanges)
}

// --- views family, PROTOCOLS.md 11.14: observations --------------------

// A positioner at a corner the app moves, and a popup (always open), each reporting where it was laid
// out (the popup also its trigger's bounds): the reports come after the frame, once per change, never
// again for the same values.
func (a *App) positioned(cx *gk.Cx, p overlay.ResolvedPosition) {
	a.positions++
	a.lastPos = p
}

func (a *App) popped(cx *gk.Cx, e overlay.Event117) {
	a.pops++
	a.lastPop = e
}

func (a *App) observeRoot(cx *gk.Cx) gk.Element {
	a.frames5++
	pos := overlay.PositionerCorner(gk.AnchorTopLeft, gk.GpuiPoint{X: a.obsX, Y: 50}).OnPosition(a.positioned).Child(kit.Label("placed"))
	pop := overlay.Popup("pp", kit.Button("bt").Label("Trigger")).Content(kit.Label("inside")).OnPosition(a.popped)
	return gk.Div(s.SizeFull).Child(pos).Child(gk.Div(s.P4).Child(pop))
}

// placementName prints an optional Placement as the checks expect (nil, or the name).
func placementName(p gk.Option[gk.Placement]) string {
	v, ok := p.Get()
	if !ok {
		return "nil"
	}
	names := []string{"Top", "Bottom", "Left", "Right"}
	if int(v) < len(names) {
		return names[v]
	}
	return fmt.Sprint(uint16(v))
}

func observeFirst(a *App, cx *gk.Cx) {
	fmt.Printf("overlay observe first positions=%d x=%.0f y=%.0f placement=%s pops=%d pop_w=%v\n", a.positions,
		a.lastPos.Bounds.Origin.X, a.lastPos.Bounds.Origin.Y, placementName(a.lastPos.Placement), a.pops, a.lastPop.A1.Size.Width > 0)
	a.obsX = 120 // the positioner moves: one more report
}

func observeMoved(a *App, cx *gk.Cx) {
	fmt.Printf("overlay observe moved positions=%d x=%.0f pops=%d\n", a.positions, a.lastPos.Bounds.Origin.X, a.pops)
}

func observeDone(a *App, cx *gk.Cx) {
	fmt.Printf("overlay done mode=observe positions=%d pops=%d frames_seen=%v\n", a.positions, a.pops, a.frames5 >= 3)
}

// --- menu-redraw: a menu item's handler after its view has been described again many times --------

// A dropdown with an item that has its own click handler (Alpha) and one that runs an action
// (Bravo), beside a label that changes on every tick, so each redraw is a new description.
func (a *App) menuRoot(cx *gk.Cx) gk.Element {
	menu := kit.NewPopupMenu().Item(kit.NewPopupMenuItem("Alpha").OnClick(a.menuAlpha)).Menu("Bravo", a.menuAct)
	return gk.HFlex(s.Gap2, s.P2).Child(kit.DropdownMenu(kit.Button("menu").Label("Menu"), menu)).
		Child(kit.Label(fmt.Sprintf("ticks %d", a.menuTicks)))
}

func (a *App) menuAlpha(cx *gk.Cx, e gk.ClickEvent) {
	a.menuA++
}

func (a *App) menuBravo(cx *gk.Cx, act gk.Action) {
	a.menuB++
}

func (a *App) menuTick(cx *gk.Cx) {
	a.menuTicks++ // the view's own handler: it draws again
}

// --- the app -----------------------------------------------------------------------------------

func (a *App) rootView(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Child(kit.Label("overlay"))
}

func init() {
	steps["dialog"] = []step{{10, openFirst}, {30, bump}, {50, reopen}, {70, closeByApp}, {90, dialogDone}}
	steps["alert"] = []step{{10, openAlert}, {50, alertDone}}
	steps["sheet"] = []step{{10, openSheet}, {30, sheetCheck}, {50, sheetDone}}
	steps["release"] = []step{{10, openFirst}, {30, releaseFirst}}
	steps["wrong-root"] = []step{{10, openWrong}}
	steps["dock"] = []step{{10, dockMake}, {30, dockCheck}, {50, dockZoomed}, {70, dockDone}}
	steps["questionnaire"] = []step{{10, questionnaire}}
	steps["questionnaire-dup"] = []step{{10, questionnaire}}
	steps["popover"] = []step{{10, popoverOpen}, {30, popoverClose}, {50, popoverDone}}
	steps["observe"] = []step{{10, observeFirst}, {30, observeMoved}, {70, observeDone}}
	for _, m := range []string{"menu-redraw", "menu-redraw-b"} {
		inits[m] = func(a *App, cx *gk.Cx) {
			a.menuAct = gk.NewAction(cx, "overlay::Bravo")
			gk.OnAppAction(cx, a.menuAct, a.menuBravo)
			gk.Every(cx, 10*ms, a.menuTick)
		}
		roots[m] = (*App).menuRoot
	}
	roots["dock"] = (*App).dockRoot
	roots["popover"] = (*App).popoverRoot
	roots["observe"] = (*App).observeRoot
}

func (a *App) init(cx *gk.Cx) {
	if f, ok := inits[a.mode]; ok {
		f(a, cx)
	}
	for _, st := range steps[a.mode] {
		gk.After(cx, st.at*ms, func(cx *gk.Cx) { st.f(a, cx) })
	}
}

func (a *App) start(cx *gk.Cx) {
	root := a.rootView
	if f, ok := roots[a.mode]; ok {
		root = func(cx *gk.Cx) gk.Element { return f(a, cx) }
	}
	a.root = gk.NewView(a.init, root)
	gk.OpenWindow(cx, "overlay", 640, 480, a.root)
}

// Where menu-redraw clicks: the dropdown's button, and its two items once it is open.
const (
	menuButton  = "click 30 22\n"
	menuAlphaAt = "click 50 60\n"
	menuBravoAt = "click 50 88\n"
)

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	_, hasSteps := steps[a.mode]
	_, hasInit := inits[a.mode]
	if !hasSteps && !hasInit && a.mode != "window-real" {
		fmt.Printf("overlay: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	step := "frame\nframe\n"
	script := "size 640 480\nframe\n"
	switch a.mode {
	case "dialog":
		// open at 10; bumped at 30 while open; escape (on_cancel of the second description); reopened
		// at 50 on the same view; closed by the app at 70; done at 90
		script += "advance 10\n" + step + "advance 20\n" + step + "key escape\n" + step +
			"advance 20\n" + step + "advance 20\n" + step + "advance 20\n" + step + "stat\n"
	case "alert":
		script += "advance 10\n" + step + "key enter\n" + step + "advance 40\n" + step + "stat\n"
	case "menu-redraw", "menu-redraw-b":
		// open the menu, let the view be described again ten times (a tick every 10 ms changes its
		// label), then click item Alpha (menu-redraw) or Bravo (menu-redraw-b) once
		item := menuAlphaAt
		if a.mode == "menu-redraw-b" {
			item = menuBravoAt
		}
		script += menuButton + "frame\n" + strings.Repeat("advance 10\nframe\n", 10) +
			item + "frame\nadvance 10\nframe\nadvance 10\nframe\nstat\n"
	case "dock-layout", "render":
		script += "advance 10\n" + step + "advance 20\n" + step + "advance 20\n" + step + "advance 20\n" + step +
			"advance 20\n" + step + "stat\n"
	default:
		script += "advance 10\n" + step + "advance 20\n" + step + "advance 20\n" + step + "advance 20\n" + step + "stat\n"
	}
	if a.mode == "window-real" {
		gk.Run(a.realStart)
		return
	}
	gk.RunHeadless(script, a.start)
	if a.mode == "menu-redraw" || a.mode == "menu-redraw-b" {
		fmt.Printf("overlay menu fired alpha=%d bravo=%d redrawn=%v\n", a.menuA, a.menuB, a.menuTicks >= 10)
	}
}
