// Compilable snippet host for docs/API-GO.md.
// gates/docs-snippets.py checks every code block and inline API call span in docs/API-GO.md
// against this file.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/base"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/palette"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/table"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type App struct {
	name    editor.InputState
	list    table.ListState
	save    gk.Action
	dark    bool
	items   []string
	savedAt string
	ask     gk.Built[overlay.AlertDialogEl]
	count   gk.Model[int]
	preview gk.View
	tipView gk.View
	pic     gk.RenderImage
}

func (a *App) init(cx *gk.Cx) {
	a.name = editor.NewInputState(cx).Placeholder("Name").Build()
	a.name.OnInputEvent(cx, a.nameEvent)
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.list.OnListEvent(cx, a.listEvent)
	a.save = gk.Shortcut(cx, "ctrl-s", a.saveNow)
	a.ask = gk.NewBuilt(cx, a.confirm)
	w, h, px := uint32(1), uint32(1), []byte{255, 0, 0, 255}
	a.pic = gk.NewRenderImage(cx, gk.PixelImage{Width: w, Height: h, RGBA: px})
	gk.After(cx, 500*time.Millisecond, a.tick)
	gk.ThemeChange(cx, gk.ThemeModeDark)
}

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	return table.ListItem(gk.Key("item", at.Row)).Child(kit.Label(a.items[at.Row]))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	ok := kit.Button("ok").Primary().Label("OK").OnClick(a.clicked)
	tags := gk.HFlex(s.Gap2).
		Child(kit.TagSecondary().Text("draft")).
		Child(kit.Switch("wifi").Checked(a.dark)).
		Child(gk.Icon(gk.IconStarFill).TextColor(theme.Warning)).
		Child(base.Checkbox("raw"))
	row := gk.Div(s.SizeFull, s.Gap2).Style(s.ItemsCenter).
		Child(gk.Div().W(gk.Px(240)).H(gk.Rems(2)).Bg(gk.HSLA(0.6, 0.5, 0.5, 1))).
		Child(gk.Div().W(gk.Relative(0.5)).BorderColor(palette.Blue500)).
		Child(kit.Label("bold", s.FontBold)).
		Child(kit.Label("25:00").TextSize(gk.Px(64)).LineHeight(gk.Px(80))).
		Child(kit.Button("small").WithSize(gk.SizeSmall).Label("Small"))
	if a.dark {
		row = row.Bg(theme.Muted)
	}
	return gk.VFlex(s.SizeFull, s.P4).Bg(theme.Background).
		Child(ok).
		Child(tags).
		Child(row).
		Child(editor.Input(a.name)).
		Child(table.List(a.list)).
		Child(kit.Label(fmt.Sprintf("%d clicks", a.count.Get()))).
		Child(kit.Button(gk.Key("row", 3)).Label("row 3").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.remove(cx, 3) })).
		Child(kit.Button("ask").Label("Ask").OnClick(a.askNow))
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

func (a *App) nameEvent(cx *gk.Cx, e editor.InputEvent) {
	switch e.(type) {
	case editor.InputEventPressEnter:
		text := a.name.Value(cx)
		a.items = append(a.items, text)
		a.name.SetValue(cx, "")
		gk.Refresh(cx)
	}
}

func (a *App) listEvent(cx *gk.Cx, e table.ListEvent) {
	if c, ok := e.(table.ListEventConfirm); ok {
		fmt.Println("row", c.Row)
	}
}

func (a *App) saveNow(cx *gk.Cx) {
	a.savedAt = "now"
	overlay.PushNotification(cx, "Saved")
}

func (a *App) tick(cx *gk.Cx) {
	gk.Refresh(cx)
}

func (a *App) forget(cx *gk.Cx) {
	gk.Release(cx, a.list)
}

func (a *App) pick(cx *gk.Cx) {
	a.list.SetSelectedIndex(cx, gk.Some(table.Row(4)))
	a.list.ScrollToItem(cx, table.Row(4), gk.ScrollStrategyTop)
	a.list.SetSelectedIndex(cx, table.NoRow)
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
	overlay.OpenAlertDialog(cx, a.ask)
}

func (a *App) confirm(cx *gk.Cx) overlay.AlertDialogEl {
	return overlay.AlertDialog().Title(kit.Label("Delete?")).ShowCancel(true).OkVariant(gk.ButtonVariantDanger).OnOk(a.confirmed)
}

func (a *App) confirmed(cx *gk.Cx, _ gk.ClickEvent) bool {
	return true
}

func (a *App) load(cx *gk.Cx) {
	gk.Go(cx, func() []string {
		time.Sleep(10 * time.Millisecond)
		return []string{"a", "b"}
	}, func(cx *gk.Cx, names []string) {
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

var pngBytes []byte

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
	el := kit.Label("custom")
	b, img := pngBytes, a.pic
	return gk.VFlex().
		Child(gk.Div().H(gk.Px(8)).Bg(gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1)))).
		Child(gk.Div().H(gk.Px(8)).Bg(g)).
		Child(gk.Div().ID("src").OnDrag(gk.DragValue{ID: 7, Text: "seven"}, a.preview).CanDrop(gk.DropAccept{IDs: []uint64{7}})).
		Child(gk.Div().ID("saved").Tooltip(overlay.NewKitTooltip("Saved"))).
		Child(gk.Div().ID("custom").Tooltip(overlay.NewKitTooltipElement(el))).
		Child(gk.Div().ID("view").Tooltip(a.tipView)).
		Child(gk.Img(gk.ImageSourceRender{Value: img})).
		Child(gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: b}}))
}

func main() {
	a := &App{count: gk.NewModel(0)}
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "API", 640, 480, gk.NewView(a.init, a.view))
	}); err != nil {
		log.Fatal(err)
	}
}
