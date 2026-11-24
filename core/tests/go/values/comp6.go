package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/base"
	"github.com/ibanks42/goui-kit/gk/table"
)

func init() {
	// Register builder-chain readback tests for ColorPicker, GridLayout, Tree, and ScaleOrdinal.
	modes["c6-chains"] = &mode{click: once(chains)}
}

// chains proves fluent builder chains preserve every configured property across
// ColorPickerState (hue=0.50), GridLayoutState (2 areas), TreeState (2 valid
// entries then invalid index 2), and ScaleOrdinal (overridden range b_h=0.75
// plus neutral unknown fallback L=0.50, S=0.00).
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
