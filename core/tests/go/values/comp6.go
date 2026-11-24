// The component protocol tests (docs/PROTOCOLS.md 11.18 to 11.26), in the values test app:
//
//	c6-background   try_parse_background read back as gk.BackgroundValue (11.23; background.go)
//	c6-matcher      a calendar's disabled matcher of every variant, written and read back (11.21;
//	                dates.go)
//	c6-q-rules      a questionnaire item's validator as rules, checked by the library (11.26;
//	                qrules.go)
//	c6-resize       a resizable group whose handles the app draws: the snapshot of the kit's context
//	                (11.18) read by the row, described again when the pointer moves over the handle
//	                (resize.go)
//	c6-chains       colours and value nodes in a constructor's builder chain (11.27, the editor
//	                family's rule of 9.31): a colour picker's default value, a grid's default areas,
//	                a tree's items, an ordinal scale's colours and its colour for unknown values
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/base"
	"gpuikit.dev/gk/table"
)

func init() {
	modes["c6-chains"] = &mode{click: once(chains)}
}

func chains(a *App, cx *gk.Cx) {
	picker := base.NewColorPickerState(cx).DefaultValue(gk.HSLA(0.5, 1, 0.5, 1)).Build()
	v, has := picker.Value(cx).Get()
	fmt.Printf("values c6 chains picker has=%v h=%.2f\n", has, v.H)
	grid := base.NewGridLayoutState(cx).DefaultAreas([]base.GridArea{base.NewGridArea("a"), base.NewGridArea("b")}).Build()
	fmt.Printf("values c6 chains grid areas=%d\n", len(grid.Areas(cx)))
	tree := table.NewTreeState(cx).Items([]table.TreeItem{table.NewTreeItem("r", "root").Expanded(true).Child(table.NewTreeItem("c", "child"))}).Build()
	e0 := tree.Entry(cx, 0).Valid()
	e1 := tree.Entry(cx, 1).Valid()
	e2 := tree.Entry(cx, 2).Valid()
	fmt.Printf("values c6 chains tree entries=%v,%v,%v\n", e0, e1, e2)
	scale := gk.NewScaleOrdinal(cx, []string{"a", "b"}, []gk.Color{gk.HSLA(0, 1, 0.5, 1), gk.HSLA(0.5, 1, 0.5, 1)}).
		Range([]gk.Color{gk.HSLA(0.25, 1, 0.5, 1), gk.HSLA(0.75, 1, 0.5, 1)}).Unknown(gk.HSLA(0, 0, 0.5, 1)).Build()
	b, _ := scale.Map(cx, "b").Get()
	z, hz := scale.Map(cx, "zz").Get()
	fmt.Printf("values c6 chains scale b_h=%.2f unknown=%v unknown_l=%.2f unknown_s=%.2f\n", b.H, hz, z.L, z.S)
}
