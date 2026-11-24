// val-theme: the theme registry, a config applied, the mode changed, and try_parse_color's Result
// read back as gk.Fallible[gk.Hsla].
package main

import (
	"fmt"

	"gpuikit.dev/gk"
)

func init() {
	modes["val-theme"] = &mode{click: themeClick}
}

// The theme: the registry's themes (a map, read back sorted), a config applied, the mode changed (a
// static taking Option<&mut Window>), fields read and written, results read back.
func themeClick(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		reg := gk.ThemeRegistryGlobal(cx)
		themes := reg.Themes(cx)
		light := reg.DefaultLightTheme(cx)
		th := gk.ThemeGlobal(cx)
		th.ApplyConfig(cx, light)
		fmt.Printf("values registry themes=%v dark=%v\n", len(themes) > 0, th.IsDark(cx))
		gk.ThemeChange(cx, gk.ThemeModeDark)
		th.SetFontSize(cx, 17)
		err := reg.LoadThemesFromStr(cx, "not json")
		fmt.Printf("values registry bad json refused=%v\n", err != nil)
		ok := gk.TryParseColor(cx, "#ff0000")
		no := gk.TryParseColor(cx, "not a colour")
		col, _ := ok.Value.Get()
		fmt.Printf("values parse ok=%v red=%v bad_ok=%v bad_err=%v\n", ok.OK, col.L > 0.4 && col.L < 0.6, no.OK, len(no.Err) > 0)
		cfg := gk.NewThemeConfigRc(cx, gk.NewThemeConfigDefault())
		fmt.Printf("values config made=%v\n", gk.Handle(cfg) != 0)
	case 2:
		th := gk.ThemeGlobal(cx)
		fmt.Printf("values theme dark=%v font_size=%.0f\n", th.IsDark(cx), th.FontSize(cx))
		gk.ThemeChange(cx, gk.ThemeModeLight)
	case 3:
		fmt.Printf("values theme light again dark=%v\n", gk.ThemeGlobal(cx).IsDark(cx))
	}
}
