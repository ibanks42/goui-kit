package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
)

// cells holds typed gk.Cell handles exercised by `val-cells` and `val-settings`.
var cells struct {
	b           gk.Cell[bool]
	s           gk.Cell[string]
	f           gk.Cell[float64]
	dark, dirty gk.Cell[bool]
}

func init() {
	modes["val-cells"] = &mode{click: cellsClick}
	modes["val-settings"] = &mode{view: settingsPage, start: func(a *App, cx *gk.Cx) {
		cells.dark = gk.NewCell(cx, false)
		cells.dirty = gk.NewCell(cx, false)
	},

		script: "size 640 480\nframe\nclick 284 153\nframe\nframe\nclick 300 192\nframe\nframe\nstat\n"}
}

// cellsClick proves typed ValueCell handles store initial bool/string/float64
// values (true, "first", 1.5), persist mutations across turns (false, "second",
// 2.25), and release cleanly.
func cellsClick(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		cells.b = gk.NewCell(cx, true)
		cells.s = gk.NewCell(cx, "first")
		cells.f = gk.NewCell(cx, 1.5)
		fmt.Printf("values cells made b=%v s=%s f=%.1f\n", gk.CellGet(cx, cells.b), gk.CellGet(cx, cells.s), gk.CellGet(cx, cells.f))
		gk.CellSet(cx, cells.b, false)
		gk.CellSet(cx, cells.s, "second")
		gk.CellSet(cx, cells.f, 2.25)
	case 2:
		fmt.Printf("values cells again b=%v s=%s f=%.2f\n", gk.CellGet(cx, cells.b), gk.CellGet(cx, cells.s), gk.CellGet(cx, cells.f))
		gk.Release(cx, cells.b)
		gk.Release(cx, cells.s)
		gk.Release(cx, cells.f)
	}
}

func setDark(cx *gk.Cx, v bool) {
	fmt.Printf("values settings switch set=%v was=%v\n", v, gk.CellGet(cx, cells.dark))
	gk.CellSet(cx, cells.dark, v)
	gk.CellSet(cx, cells.dirty, true)
}

func fragClick(cx *gk.Cx, _ gk.ClickEvent) {
	fmt.Printf("values settings fragment click dark=%v\n", gk.CellGet(cx, cells.dark))
}

func resetDark(cx *gk.Cx) {
	fmt.Println("values settings reset")
	gk.CellSet(cx, cells.dark, false)
	gk.CellSet(cx, cells.dirty, false)
}

// settingsPage proves kit.Settings binds a boolean switch to cells.dark (flipping
// false -> true on click) and renders a custom setting fragment that reads the
// updated cell state (dark=true) on the following click.
func settingsPage(a *App, cx *gk.Cx) (gk.Element, bool) {
	sw := kit.NewSettingFieldBoolSwitch(cells.dark, setDark).OnReset(cells.dirty, resetDark)
	frag := gk.Div().W(gk.Px(200)).H(gk.Px(40)).ID("frag").OnClick(fragClick).Child(gk.Text("fragment"))
	group := kit.NewSettingGroup().Title("Look").Item(kit.NewSettingItem("Dark", sw)).Item(kit.NewSettingItemRender(frag))
	return gk.Div(s.SizeFull).Child(kit.Settings("s").SidebarSizeRange([2]float32{100, 300}).
		Page(kit.NewSettingPage("General").Group(group))), true
}
