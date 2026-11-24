package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
)

func init() {
	// Register CSS solid-color, linear-gradient, and invalid-background parse tests.
	modes["c6-background"] = &mode{click: once(background)}
}

func kindName(k gk.BackgroundKind) string {
	switch k {
	case gk.BackgroundKindSolid:
		return "Solid"
	case gk.BackgroundKindLinearGradient:
		return "Linear_Gradient"
	}
	return fmt.Sprint(uint16(k))
}

// background proves TryParseBackground decodes CSS solid colors (#ff0000 ->
// Solid, L=0.50, A=1.00) and linear gradients (90deg, 0% red -> 100% blue,
// hue 0.00 -> 0.667) while rejecting invalid strings with ok=false and an error.
func background(a *App, cx *gk.Cx) {
	solid := gk.TryParseBackground(cx, "#ff0000")
	s, sok := solid.Value.Get()
	fmt.Printf("values c6 background solid ok=%v kind=%s l=%.2f a=%.2f\n", solid.OK, kindName(s.Kind), s.Solid.L, s.Solid.A)
	if !sok {
		fmt.Println("values c6 background solid has no value")
	}
	grad := gk.TryParseBackground(cx, "linear-gradient(90deg, #ff0000 0%, #0000ff 100%)")
	g, _ := grad.Value.Get()
	fmt.Printf("values c6 background gradient ok=%v kind=%s angle=%.0f from_h=%.2f to_h=%.3f to_pct=%.2f\n", grad.OK, kindName(g.Kind),
		g.AngleOrHeight, g.Stops[0].Color.H, g.Stops[1].Color.H, g.Stops[1].Percentage)
	bad := gk.TryParseBackground(cx, "not a background")
	_, has := bad.Value.Get()
	fmt.Printf("values c6 background bad ok=%v has=%v err=%v\n", bad.OK, has, len(bad.Err) > 0)
}
