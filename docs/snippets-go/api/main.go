// Every Go call that docs/API-GO.md shows, collected into one program that compiles, so the guide
// cannot drift from the package (the docs tests check that each call in API-GO.md is text of this
// file). Not an app to read for itself: the tutorial in docs/tutorial-go is.
package main

import (
	"fmt"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/palette"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	name    gk.InputState
	list    gk.ListState
	save    gk.Action
	dark    bool
	items   []string
	savedAt string
	ask     gk.Built[gk.AlertDialogEl]
	count   gk.Model[int]
	preview gk.View        // a drag's preview (docs/GO.md section 10)
	tipView gk.View        // a view as a tooltip
	pic     gk.RenderImage // an image made once from pixels (docs/GO.md section 7)
}

func (a *App) init(cx *gk.Cx) {
	// Kit state: Rust's cx.new(|cx| InputState::new(window, cx).placeholder("Name"))
	a.name = gk.NewInputState(cx).Placeholder("Name").Build()
	a.name.OnInputEvent(cx, a.nameEvent)
	a.list = gk.NewListState(cx, gk.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.list.OnListEvent(cx, a.listEvent)
	// actions!, KeyBinding::new and on_action, in one call
	a.save = gk.Shortcut(cx, "ctrl-s", a.saveNow)
	a.ask = gk.NewBuilt(cx, a.confirm)
	w, h, px := uint32(1), uint32(1), []byte{255, 0, 0, 255}
	a.pic = gk.NewRenderImage(cx, gk.PixelImage{Width: w, Height: h, RGBA: px})
	gk.After(cx, 500*time.Millisecond, a.tick)
	gk.ThemeChange(cx, gk.ThemeModeDark)
}

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	return gk.ListItem(gk.Key("item", at.Row)).Child(gk.Label(a.items[at.Row]))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	ok := gk.Button("ok").Primary().Label("OK").OnClick(a.clicked)
	tags := gk.HFlex(s.Gap2).
		Child(gk.TagSecondary().Text("draft")).
		Child(gk.Switch("wifi").Checked(a.dark)).
		Child(gk.Icon(gk.IconStarFill).TextColor(theme.Warning)).
		Child(gk.BaseCheckbox("raw"))
	row := gk.Div(s.SizeFull, s.Gap2).Style(s.ItemsCenter).
		Child(gk.Div().W(gk.Px(240)).H(gk.Rems(2)).Bg(gk.HSLA(0.6, 0.5, 0.5, 1))).
		Child(gk.Div().W(gk.Relative(0.5)).BorderColor(palette.Blue500)).
		Child(gk.Label("bold", s.FontBold)).
		Child(gk.Label("25:00").TextSize(gk.Px(64)).LineHeight(gk.Px(80))).
		Child(gk.Button("small").WithSize(gk.SizeSmall).Label("Small"))
	if a.dark {
		row = row.Bg(theme.Muted)
	}
	return gk.VFlex(s.SizeFull, s.P4).Bg(theme.Background).
		Child(ok).
		Child(tags).
		Child(row).
		Child(gk.Input(a.name)).
		Child(gk.List(a.list)).
		Child(gk.Label(fmt.Sprintf("%d clicks", a.count.Get()))).
		Child(gk.Button(gk.Key("row", 3)).Label("row 3").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.remove(cx, 3) })).
		Child(gk.Button("ask").Label("Ask").OnClick(a.askNow))
}

func (a *App) clicked(cx *gk.Cx, e gk.ClickEvent) {
	if e.ClickCount == 2 {
		fmt.Println("double click")
	}
	a.count.Set(cx, a.count.Peek()+1)
}

func (a *App) remove(cx *gk.Cx, row int) {
	gk.StopPropagation(cx)
	gk.Refresh(cx)
}

func (a *App) nameEvent(cx *gk.Cx, e gk.InputEvent) {
	switch e.(type) {
	case gk.InputEventPressEnter:
		text := a.name.Value(cx) // a Go string: a copy, the app's to keep
		a.items = append(a.items, text)
		a.name.SetValue(cx, "")
		gk.Refresh(cx)
	}
}

func (a *App) listEvent(cx *gk.Cx, e gk.ListEvent) {
	if c, ok := e.(gk.ListEventConfirm); ok {
		fmt.Println("row", c.Row)
	}
}

func (a *App) saveNow(cx *gk.Cx) {
	a.savedAt = "now"
	gk.PushNotification(cx, "Saved")
}

func (a *App) tick(cx *gk.Cx) {
	gk.Refresh(cx)
}

func (a *App) forget(cx *gk.Cx) {
	gk.Release(cx, a.list)
}

func (a *App) pick(cx *gk.Cx) {
	a.list.SetSelectedIndex(cx, gk.Some(gk.Row(4))) // an IndexPath: gk.Row(n), or gk.BaseIndexPath{Section: 1, Row: 4}
	a.list.ScrollToItem(cx, gk.Row(4), gk.ScrollStrategyTop)
	a.list.SetSelectedIndex(cx, gk.NoRow)
	if at, chosen := a.list.SelectedIndex(cx).Get(); chosen {
		fmt.Println("row", at.Row)
	}
}

func (a *App) more(cx *gk.Cx) {
	gk.OpenWindowWith(cx, gk.WindowConfig{Width: 320, Height: 240, Title: "More", MinWidth: 200}, gk.NewView(nil, a.columns))
}

func (a *App) columns(cx *gk.Cx) gk.Element {
	return gk.HFlex().Child(gk.VFlex().W(gk.Auto()))
}

func (a *App) askNow(cx *gk.Cx, _ gk.ClickEvent) {
	gk.OpenAlertDialog(cx, a.ask)
}

func (a *App) confirm(cx *gk.Cx) gk.AlertDialogEl {
	return gk.AlertDialog().Title(gk.Label("Delete?")).ShowCancel(true).OkVariant(gk.ButtonVariantDanger).OnOk(a.confirmed)
}

func (a *App) confirmed(cx *gk.Cx, _ gk.ClickEvent) bool {
	return true
}

func (a *App) load(cx *gk.Cx) {
	gk.Go(cx, func() []string {
		time.Sleep(10 * time.Millisecond) // on a goroutine
		return []string{"a", "b"}
	}, func(cx *gk.Cx, names []string) { // on the UI thread
		a.items = append(a.items, names...)
		gk.Refresh(cx)
	})
}

func (a *App) stream() {
	go func() {
		for i := range 3 {
			gk.Post(func(cx *gk.Cx) { a.items = append(a.items, fmt.Sprint(i)); gk.Refresh(cx) })
		}
	}()
}

// --- windows and drawing: a window's icon and app id, the quit, reduced motion, gradients, drag
// and drop, tooltips, the menu bar, images

var pngBytes []byte // the app's icon, a PNG (go:embed in an app)

func (a *App) mainWindow(cx *gk.Cx, root gk.View) {
	gk.OpenWindowWith(cx, gk.WindowConfig{Title: "Notes", AppID: "notes", Icon: pngBytes}, root)
	gk.OnAppQuit(cx, a.quitting)
	if gk.ReduceMotion(cx) {
		fmt.Println("still")
	}
	quit := gk.Shortcut(cx, "ctrl-q", a.quitting)
	gk.SetMenus(cx, gk.Menu{Name: "App", Items: []gk.MenuItem{gk.MenuAction{Name: "Quit", Action: quit}}})
}

func (a *App) quitting(cx *gk.Cx) {}

func (a *App) port(cx *gk.Cx) gk.Element {
	cyan, violet := gk.HSLA(0.5, 0.75, 0.55, 1), gk.HSLA(0.75, 0.6, 0.65, 1)
	g := gk.NewLinearGradient(0, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1)).InOklab()
	el := gk.Label("custom")
	b, img := pngBytes, a.pic
	return gk.VFlex().
		Child(gk.Div().H(gk.Px(8)).Bg(gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1)))).
		Child(gk.Div().H(gk.Px(8)).Bg(g)).
		Child(gk.Div().ID("src").OnDrag(gk.DragValue{ID: 7, Text: "seven"}, a.preview).CanDrop(gk.DropAccept{IDs: []uint64{7}})).
		Child(gk.Div().ID("saved").Tooltip(gk.NewKitTooltip("Saved"))).
		Child(gk.Div().ID("custom").Tooltip(gk.NewKitTooltipElement(el))).
		Child(gk.Div().ID("view").Tooltip(a.tipView)).
		Child(gk.Img(gk.ImageSourceRender{Value: img})).
		Child(gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: b}}))
}

func main() {
	a := &App{count: gk.NewModel(0)}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "API", 640, 480, gk.NewView(a.init, a.view))
	})
}
