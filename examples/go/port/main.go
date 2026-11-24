// port: protocol tests in Go (docs/PROTOCOLS.md 11.38, 12.17, and the Go ports
// of 11.17, 11.30, 13.20, 13.31): a window's close asked of the app, the quit hook, reduced motion, a
// window's icon and app id, drag and drop, the menu bar, image sources as bytes and pixels, an
// element's tooltip, a plot of paths and quads. Each mode runs offscreen (the diag library) under a
// gk-meter script and prints lines gates/go-proto.sh checks, with the exit code.
//
//	GK_PROTO=close-veto | quit-hook | reduce-motion | window-icon | bad-icon | drag | menus | menu-no-action |
//	         images | bad-pixels | tooltip | plot | fill-nil
package main

import (
	"fmt"
	"os"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	mode         string
	root, second gk.View
	preview, tip gk.View
	allow        bool
	asked, quits int
	veto         gk.ActionListener
	motion       [3]bool
	drops, tips  int
	previews     int
	quit, open   gk.Action
	plotted      int
	win          gk.Window
	red          gk.RenderImage // made once from pixels (13.31)
	redID        uint64
	shown0       uint64 // gk.TestImagesShown when the checks began
	images0      uint32
	dropped0     uint64
	menuTicks    int // menu-redraw: the view's redraws while the menu is open
	menuA, menuB int // menu-redraw: clicks that reached item A's handler and item B's action
	menuAct      gk.Action
	ticks        gk.Timer
	nTicks       int // shown by imagesView, so each tick is a new description
}

// A 2 x 2 PNG (red and green), a window's icon and an image of encoded bytes.
var iconPNG = []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 2, 0, 0, 0, 2, 8, 6, 0, 0, 0, 114, 182, 13, 36, 0, 0, 0, 16, 73, 68, 65, 84, 120, 156, 99, 248, 207, 192, 240, 31, 10, 33, 20, 0, 69, 204, 7, 249, 102, 203, 255, 57, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130}

// --- a window's close asked of the app (11.38) --------------------------------------------------------

func (a *App) mayClose(cx *gk.Cx) bool {
	a.asked++
	fmt.Printf("port asked may close n=%d answer=%v\n", a.asked, a.allow)
	return a.allow
}

// saysYes: Go's types leave no handler without an answer.
func (a *App) saysYes(cx *gk.Cx) bool {
	a.asked++
	fmt.Printf("port asked by a second handler n=%d\n", a.asked)
	return true
}

func (a *App) vetoCheck(cx *gk.Cx) {
	w := gk.Windows(cx)[0]
	a.veto = gk.OnWindowShouldClose(cx, w, a.mayClose)
	first := gk.TestShouldClose(cx, w) // the handler says no
	a.allow = true
	second := gk.TestShouldClose(cx, w) // now yes
	gk.Release(cx, a.veto)
	third := gk.TestShouldClose(cx, w)                          // no handler: it closes
	a.veto = gk.OnWindowShouldClose(cx, gk.Window{}, a.saysYes) // the turn's window
	fourth := gk.TestShouldClose(cx, w)
	fmt.Printf("port should close first=%v second=%v third=%v fourth=%v asked=%d windows=%d\n", first, second, third, fourth,
		a.asked, len(gk.Windows(cx)))
}

// --- the quit hook ------------------------------------------------------------------------------------

func (a *App) quitHook(cx *gk.Cx) {
	a.quits++
	fmt.Printf("port quit hook ran n=%d windows=%d\n", a.quits, len(gk.Windows(cx)))
}

func (a *App) quitReleased(cx *gk.Cx) {
	a.quits++ // counted, so "quits=1" shows it never ran
	fmt.Println("port quit hook ran after its release (wrong)")
}

// --- reduced motion ----------------------------------------------------------------------------------

func (a *App) motionRead(cx *gk.Cx) {
	a.motion[1] = gk.ReduceMotion(cx)
	gk.SetReduceMotion(cx, false)
	gk.After(cx, 20e6, a.motionReadAgain)
}

func (a *App) motionReadAgain(cx *gk.Cx) {
	a.motion[2] = gk.ReduceMotion(cx)
	fmt.Printf("port reduce motion start=%v set=%v cleared=%v\n", a.motion[0], a.motion[1], a.motion[2])
}

// --- a window's icon and app id ----------------------------------------------------------------------

func (a *App) secondView(cx *gk.Cx) gk.Element {
	fmt.Println("port second view described")
	return gk.Div(s.SizeFull).Child(gk.Label("second window"))
}

func (a *App) openIconWindow(cx *gk.Cx) {
	a.second = gk.NewView(nil, a.secondView)
	gk.OpenWindowWith(cx, gk.WindowConfig{Width: 300, Height: 200, Title: "Icon", AppID: "gk-test", Icon: iconPNG}, a.second)
	fmt.Printf("port icon window opened n=%d\n", len(gk.Windows(cx)))
}

func (a *App) openBadIcon(cx *gk.Cx) {
	a.second = gk.NewView(nil, a.secondView)
	gk.OpenWindowWith(cx, gk.WindowConfig{Title: "Bad", Icon: []byte{1, 2, 3, 4}}, a.second) // stops: not a PNG
	fmt.Println("port bad icon accepted (wrong)")
}

// --- drag and drop (13.20) -------------------------------------------------------------------------------

func (a *App) previewView(cx *gk.Cx) gk.Element {
	a.previews++
	if a.previews == 1 {
		fmt.Println("port drag preview described")
	}
	return gk.Div().Px(gk.Px(8)).Bg(theme.Accent).Child(gk.Label("dragging"))
}

func (a *App) dropped7(cx *gk.Cx, d gk.DragValue) {
	a.drops++
	fmt.Printf("port drop on seven id=%d text=%s drops=%d\n", d.ID, d.Text, a.drops)
}

func (a *App) dropped8(cx *gk.Cx, d gk.DragValue) {
	a.drops++
	fmt.Printf("port drop on eight id=%d (wrong: it takes only 8)\n", d.ID)
}

func (a *App) dragView(cx *gk.Cx) gk.Element {
	src := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("src").OnDrag(gk.DragValue{ID: 7, Text: "seven"}, a.preview).Child(gk.Label("seven"))
	t7 := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("t7").CanDrop(gk.DropAccept{IDs: []uint64{7}}).OnDrop(a.dropped7).
		DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent))
	t8 := gk.Div().W(gk.Px(100)).H(gk.Px(100)).ID("t8").CanDrop(gk.DropAccept{IDs: []uint64{8}}).OnDrop(a.dropped8)
	return gk.HFlex().Child(src).Child(t7).Child(t8)
}

func (a *App) dragDone(cx *gk.Cx) {
	v1, f1 := gk.TestDropAccept(cx, gk.DropAccept{IDs: []uint64{7, 9}}, 7)
	v2, _ := gk.TestDropAccept(cx, gk.DropAccept{IDs: []uint64{7, 9}}, 8)
	v3, f3 := gk.TestDropAccept(cx, gk.DropAccept{Values: true, Paths: true}, 8)
	v4, _ := gk.TestDropAccept(cx, gk.DropAccept{}, 7)
	fmt.Printf("port drop accept 7=%v 8=%v any=%v none=%v files=%v,%v\n", v1, v2, v3, v4, f1, f3)
	fmt.Printf("port drag done drops=%d previewed=%v\n", a.drops, a.previews > 0)
}

// --- the menu bar (11.30) ------------------------------------------------------------------------------------

func (a *App) setMenus(cx *gk.Cx) {
	a.quit = gk.NewAction(cx, "port::Quit")
	a.open = gk.NewAction(cx, "port::Open")
	gk.SetMenus(cx,
		gk.Menu{Name: "Port", Items: []gk.MenuItem{gk.MenuAction{Name: "Open", Action: a.open}, gk.MenuSeparator{},
			gk.MenuSubmenu{Menu: gk.Menu{Name: "More", Items: []gk.MenuItem{gk.MenuAction{Name: "Copy", Action: a.open, OsAction: gk.OsActionCopy, Checked: true}}}},
			gk.MenuAction{Name: "Quit", Action: a.quit}}},
		gk.Menu{Name: "Edit", Disabled: true, Items: []gk.MenuItem{gk.MenuAction{Name: "Quit", Action: a.quit}}})
	gk.After(cx, 20e6, a.readMenus)
}

func (a *App) readMenus(cx *gk.Cx) {
	ms := gk.Menus(cx)
	var names []string
	for _, m := range ms {
		names = append(names, fmt.Sprintf("%s:%d", m.Name, len(m.Items)))
	}
	sub, _ := ms[0].Items[2].(gk.MenuSubmenu)
	copyItem, _ := sub.Menu.Items[0].(gk.MenuAction)
	quit, _ := ms[0].Items[3].(gk.MenuAction)
	fmt.Printf("port menus %s edit_disabled=%v sub=%s copy_os=%v checked=%v quit_same=%v\n", strings.Join(names, ","), ms[1].Disabled,
		sub.Menu.Name, copyItem.OsAction == gk.OsActionCopy, copyItem.Checked, quit.Action == a.quit)
}

func (a *App) menuNoAction(cx *gk.Cx) {
	gk.SetMenus(cx, gk.Menu{Name: "Port", Items: []gk.MenuItem{gk.MenuAction{Name: "Open"}}}) // stops: no action
}

// --- image sources as bytes and pixels (13.31) ----------------------------------------------------------------

// Where menu-redraw clicks: the dropdown's button, and its two items once it is open.
const (
	menuButton = "click 30 22\n"
	menuAlpha  = "click 50 60\n"
	menuBravo  = "click 50 88\n"
)

var redPixels = gk.PixelImage{Width: 2, Height: 1, RGBA: []byte{255, 0, 0, 255, 0, 0, 255, 255}}

// imagesView is described again on every tick (its label changes): the same gk.RenderImage each time.
func (a *App) imagesView(cx *gk.Cx) gk.Element {
	row := gk.HFlex(s.Gap2).Child(gk.Label(fmt.Sprintf("tick %d", a.nTicks))).
		Child(gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: iconPNG}}).W(gk.Px(40)).H(gk.Px(40)))
	if a.red != (gk.RenderImage{}) {
		row = row.Child(gk.Img(gk.ImageSourceRender{Value: a.red}).W(gk.Px(40)).H(gk.Px(20))).
			Child(gk.Img(gk.ImageSourceCustom{Value: a.red}).W(gk.Px(40)).H(gk.Px(20)))
	}
	return row
}

func (a *App) imagesInit(cx *gk.Cx) {
	a.red = gk.NewRenderImage(cx, redPixels)
	a.ticks = gk.Every(cx, 10e6, func(cx *gk.Cx) { a.nTicks++ }) // the view's own handler: it draws again
	gk.After(cx, 10e6, a.imagesCheck)
	gk.After(cx, 50e6, a.imagesKept)
	gk.After(cx, 80e6, a.imagesGone)
}

func (a *App) imagesCheck(cx *gk.Cx) {
	w, h, first, id := gk.TestRenderImage(cx, a.red, false)
	cw, ch, cfirst, cid := gk.TestRenderImage(cx, a.red, true)
	a.redID = id
	a.shown0, a.images0, a.dropped0 = gk.TestImagesShown(cx)
	fmt.Printf("port pixel image %dx%d first=%08x custom %dx%d first=%08x same=%v\n", w, h, first, cw, ch, cfirst, id == cid)
}

// After frames that each named the image: gpui was handed the one image each time (one id, so one
// texture), through ImageSourceRender and ImageSourceCustom alike; then released.
func (a *App) imagesKept(cx *gk.Cx) {
	times, images, _ := gk.TestImagesShown(cx)
	_, _, _, id := gk.TestRenderImage(cx, a.red, false)
	fmt.Printf("port image kept redrawn=%v same=%v new images=%d\n", times-a.shown0 >= 4, id == a.redID, images-a.images0)
	gk.Release(cx, a.red)
	gk.Release(cx, a.ticks)
	a.red = gk.RenderImage{} // the view shows it no more
}

// After the release: its texture dropped from the window's atlas (Window::drop_image, one window).
func (a *App) imagesGone(cx *gk.Cx) {
	_, _, dropped := gk.TestImagesShown(cx)
	fmt.Printf("port image released dropped=%d\n", dropped-a.dropped0)
}

// --- menu-redraw: a menu item's handler after its view has been described again many times ----------

// menuView shows a dropdown with an item that has its own click handler (A) and one that runs an
// action (B), beside a label that changes on every tick, so each redraw is a new description.
func (a *App) menuView(cx *gk.Cx) gk.Element {
	menu := gk.NewPopupMenu().
		Item(gk.NewPopupMenuItem("Alpha").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.menuA++ })).
		Menu("Bravo", a.menuAct)
	return gk.HFlex(s.Gap2, s.P2).
		Child(gk.DropdownMenu(gk.Button("menu").Label("Menu"), menu)).
		Child(gk.Label(fmt.Sprintf("ticks %d", a.menuTicks)))
}

func (a *App) menuInit(cx *gk.Cx) {
	a.menuAct = gk.NewAction(cx, "port.bravo")
	gk.OnAppAction(cx, a.menuAct, func(cx *gk.Cx, _ gk.Action) { a.menuB++ })
	gk.Every(cx, 10e6, func(cx *gk.Cx) { a.menuTicks++ }) // the view's own handler: it draws again
}

func (a *App) badPixels(cx *gk.Cx) {
	gk.NewRenderImage(cx, gk.PixelImage{Width: 2, Height: 2, RGBA: []byte{1, 2, 3}}) // stops: 3 bytes for 2x2
}

func (a *App) imageInView(cx *gk.Cx) gk.Element {
	gk.NewRenderImage(cx, redPixels) // stops: a view function may not make an image
	return gk.Div()
}

// --- an element's tooltip (11.17) --------------------------------------------------------------------------------

func (a *App) tipView(cx *gk.Cx) gk.Element {
	a.tips++
	if a.tips == 1 {
		fmt.Println("port tooltip view described")
	}
	return gk.Div().P(gk.Px(6)).Bg(theme.Popover).Child(gk.Label("a view as the tooltip"))
}

func (a *App) tooltipView(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap4, s.P4).
		Child(gk.Div().ID("viewtip").W(gk.Px(120)).H(gk.Px(40)).Bg(theme.Muted).Tooltip(a.tip).Child(gk.Label("view"))).
		Child(gk.Div().ID("kittip").W(gk.Px(120)).H(gk.Px(40)).Bg(theme.Muted).
			Tooltip(gk.NewKitTooltipElement(gk.HFlex(s.Gap1).Child(gk.Icon(gk.IconInfo)).Child(gk.Label("custom content")))).Child(gk.Label("kit"))).
		Child(gk.Div().ID("texttip").W(gk.Px(120)).H(gk.Px(40)).Bg(theme.Muted).Tooltip(gk.NewKitTooltip("plain text")).Child(gk.Label("text")))
}

// --- a plot of paths and quads (12.6, 12.17) ------------------------------------------------------------------------

func (a *App) plotView(cx *gk.Cx) gk.Element {
	a.plotted++
	pts := [][2]float32{{0, 80}, {40, 20}, {80, 60}, {120, 10}}
	return gk.Div().W(gk.Px(200)).H(gk.Px(100)).Child(gk.PlotWith(gk.PlotOptions{ID: "port-plot", NotInteractive: true},
		gk.NewGrid().Y([]float32{50}).Stroke(theme.Border),
		gk.NewPlotArea(pts, gk.HSLA(0.6, 0.5, 0.5, 0.2)),
		gk.NewPlotLine(pts, 2, gk.NewLinearGradient(90, gk.NewLinearColorStop(theme.Primary, 0), gk.NewLinearColorStop(gk.HSLA(0.8, 0.5, 0.5, 1), 1))),
		gk.NewPlotQuad(150, 40, 20, 20, theme.Accent, gk.QuadStyle{CornerRadius: 4, BorderWidth: 1, BorderColor: theme.Border}),
		gk.NewPlotLine(pts[:1], 2, theme.Primary), // one point: nothing painted
	))
}

func (a *App) plotCheck(cx *gk.Cx) {
	fmt.Printf("port plot described=%v\n", a.plotted > 0)
}

func (a *App) fillNil(cx *gk.Cx) gk.Element {
	var f gk.Fill
	return gk.Div().Bg(f) // stops: a nil Fill
}

// --- the app ------------------------------------------------------------------------------------------------------------

func (a *App) plain(cx *gk.Cx) gk.Element {
	return gk.Div(s.SizeFull).Bg(theme.Background).Child(gk.Label("port"))
}

func (a *App) motionView(cx *gk.Cx) gk.Element {
	fmt.Printf("port reduce motion in view=%v\n", gk.ReduceMotion(cx)) // a read a view function may make
	return gk.Div(s.SizeFull).Child(gk.Label("motion"))
}

func (a *App) init(cx *gk.Cx) {
	switch a.mode {
	case "close-veto":
		gk.After(cx, 10e6, a.vetoCheck)
	case "quit-hook":
		gk.OnAppQuit(cx, a.quitHook)
		gone := gk.OnAppQuit(cx, a.quitReleased)
		gk.Release(cx, gone)
	case "reduce-motion":
		a.motion[0] = gk.ReduceMotion(cx)
		gk.SetReduceMotion(cx, true)
		gk.After(cx, 10e6, a.motionRead)
	case "window-icon":
		gk.After(cx, 10e6, a.openIconWindow)
	case "bad-icon":
		gk.After(cx, 10e6, a.openBadIcon)
	case "drag":
		a.preview = gk.NewView(nil, a.previewView)
		gk.After(cx, 60e6, a.dragDone)
	case "menus":
		gk.After(cx, 10e6, a.setMenus)
	case "menu-no-action":
		gk.After(cx, 10e6, a.menuNoAction)
	case "images":
		a.imagesInit(cx)
	case "bad-pixels":
		a.badPixels(cx)
	case "menu-redraw", "menu-redraw-b":
		a.menuInit(cx)
	case "tooltip":
		a.tip = gk.NewView(nil, a.tipView)
	case "plot":
		gk.After(cx, 10e6, a.plotCheck)
	}
}

func (a *App) start(cx *gk.Cx) {
	render := a.plain
	switch a.mode {
	case "reduce-motion":
		render = a.motionView
	case "drag":
		render = a.dragView
	case "images":
		render = a.imagesView
	case "image-in-view":
		render = a.imageInView
	case "menu-redraw", "menu-redraw-b":
		render = a.menuView
	case "tooltip":
		render = a.tooltipView
	case "plot":
		render = a.plotView
	case "fill-nil":
		render = a.fillNil
	}
	a.root = gk.NewView(a.init, render)
	a.win = gk.OpenWindow(cx, "port", 640, 480, a.root)
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	script := "size 640 480\nframe\n"
	advance := strings.Repeat("advance 25\n", 6) + "frame\nstat\n"
	switch a.mode {
	case "drag":
		// a drag of value 7 onto the target that takes 7, then onto the one that takes only 8
		script = "size 400 300\nframe\nframe\ndrag 50 50 150 50\nframe\ndrag 50 50 250 50\nframe\nadvance 30\nframe\nadvance 40\nframe\nstat\n"
	case "tooltip":
		// rest on each element past the tooltip's delay
		script += "frame\nmove 60 36\n" + strings.Repeat("advance 200\nframe\n", 4) + "shot tooltip-view.png\n" +
			"move 60 92\n" + strings.Repeat("advance 200\nframe\n", 4) + "shot tooltip-kit.png\nstat\n"
	case "menu-redraw", "menu-redraw-b":
		// open the menu, let the view be described again ten times (a tick every 10 ms changes
		// its label), then click item Alpha (menu-redraw) or Bravo (menu-redraw-b) once
		item := menuAlpha
		if a.mode == "menu-redraw-b" {
			item = menuBravo
		}
		script = "size 640 480\nframe\n" + menuButton + "frame\n" + strings.Repeat("advance 10\nframe\n", 10) +
			item + "frame\n" + strings.Repeat("advance 10\nframe\n", 3) + "stat\n"
	case "images":
		// a frame every 10 ms: the view names its image on each one
		script = "size 640 480\n" + strings.Repeat("frame\nadvance 10\n", 10) + "frame\nstat\n"
	default:
		script += advance
	}
	if path := os.Getenv("GK_PORT_SCRIPT"); path != "" {
		// a test's own script instead of the mode's (diagnosis: shots of what a mode draws)
		if b, err := os.ReadFile(path); err == nil {
			script = string(b)
		}
	}
	gk.RunHeadless(script, a.start)
	if strings.HasPrefix(a.mode, "menu-redraw") {
		fmt.Printf("port menu fired alpha=%d bravo=%d redrawn=%v\n", a.menuA, a.menuB, a.menuTicks >= 10)
	}
	if a.mode == "quit-hook" {
		fmt.Printf("port after the run quits=%d\n", a.quits) // the hook ran as the offscreen app shut down
	}
	fmt.Printf("port done mode=%s asked=%d drops=%d tips=%d\n", a.mode, a.asked, a.drops, a.tips)
}
