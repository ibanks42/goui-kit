// The gallery's pages in Go: the twin of ref/src/bin/gallery/pages.rs,
// page for page and call for call.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

var pages = []Page{
	{"label-basic", labelBasic},
	{"div-style", divStyle},
	{"div-hover", divHover},
	{"button-primary", buttonPrimary},
	{"button-click", buttonClick},
	{"input", input},
	{"input-cleanable", inputCleanable},
	{"list", list},
	{"switch-bare", switchBare},
	{"switch-bare-click", switchBareClick},
	{"button-variants", buttonVariants},
	{"button-outline", buttonOutline},
	{"button-sizes", buttonSizes},
	{"button-states", buttonStates},
	{"alert-variants", alertVariants},
	{"alert-shapes", alertShapes},
	{"avatar", avatar},
	{"badge", badge},
	{"checkbox", checkbox},
	{"checkbox-click", checkboxClick},
	{"progress", progress},
	{"progress-circle", progressCircle},
	{"separator", separator},
	{"switch", switchPage},
	{"switch-click", switchClick},
	{"tag", tag},
	{"label-more", labelMore},
	{"radio", radio},
	{"rating", rating},
	{"breadcrumb", breadcrumb},
	{"group-box", groupBox},
	{"description-list", descriptionList},
	{"kbd", kbd},
	{"link", link},
	{"skeleton", skeleton},
}

func labelBasic(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.Label("A plain label")).
		Child(kit.Label("An extra small label").Style(s.TextXs)).
		Child(kit.Label("A muted label").TextColor(theme.MutedForeground))
}

func divStyle(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap3, s.FlexWrap).
		Child(gk.Div().W(gk.Px(120)).H(gk.Px(60)).Bg(gk.HSLA(0.6, 0.7, 0.5, 1)).Style(s.RoundedSm, s.P2).Text("solid")).
		Child(gk.Div().W(gk.Px(120)).H(gk.Px(60)).Bg(theme.Border).Style(s.P1, s.TextXs).Text("theme border")).
		Child(gk.VFlex(s.Gap1).W(gk.Px(200)).
			Child(gk.Div(s.WFull).H(gk.Px(12)).Bg(gk.HSLA(0, 0.8, 0.5, 1))).
			Child(gk.Div(s.WFull).H(gk.Px(12)).Bg(gk.HSLA(0.33, 0.8, 0.4, 1))).
			Child(gk.Div().W(gk.Px(100)).H(gk.Px(12)).Bg(gk.HSLA(0.12, 0.9, 0.5, 0.5))))
}

func divHover(a *App, cx *gk.Cx) gk.Element {
	hover := gk.NewStyleRefinementDefault().Bg(gk.HSLA(0.05, 0.9, 0.5, 1))
	return gk.HFlex(s.Gap3).
		Child(gk.Div().W(gk.Px(160)).H(gk.Px(60)).Style(s.P2).Bg(gk.HSLA(0.6, 0.5, 0.5, 1)).Hover(hover).Text("hover me")).
		Child(gk.Div().W(gk.Px(160)).H(gk.Px(60)).Style(s.P2).Bg(gk.HSLA(0.6, 0.5, 0.5, 1)).Hover(hover).Text("not hovered"))
}

func buttonPrimary(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2).
		Child(kit.Button("default").Label("Default")).
		Child(kit.Button("primary").Primary().Label("Primary"))
}

func (a *App) increment(cx *gk.Cx, _ gk.ClickEvent) { a.count++ }

func buttonClick(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(kit.Button("inc").Primary().Label("Increment").OnClick(a.increment)).
		Child(kit.Label(fmt.Sprintf("clicked %d", a.count)))
}

func input(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))).
		Child(kit.Label(fmt.Sprintf("hello %s", a.name.Value(cx))))
}

func inputCleanable(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(240)).Child(editor.Input(a.name).Cleanable(true))
}

func list(a *App, cx *gk.Cx) gk.Element {
	return gk.Div(s.WFull).H(gk.Px(160)).Child(gk.UniformList("rows", a.rows).Style(s.SizeFull))
}

func switchBare(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.Switch("off")).
		Child(kit.Switch("on").Checked(true))
}

func (a *App) toggle(cx *gk.Cx, on bool) { a.flag = on }

func switchBareClick(a *App, cx *gk.Cx) gk.Element {
	return kit.Switch("toggle").Checked(a.flag).OnClick(a.toggle)
}

func buttonVariants(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.FlexWrap).
		Child(kit.Button("secondary").Secondary().Label("Secondary")).
		Child(kit.Button("danger").Danger().Label("Danger")).
		Child(kit.Button("warning").Warning().Label("Warning")).
		Child(kit.Button("success").Success().Label("Success")).
		Child(kit.Button("info").Info().Label("Info")).
		Child(kit.Button("ghost").Ghost().Label("Ghost")).
		Child(kit.Button("link").Link().Label("Link")).
		Child(kit.Button("text").Text().Label("Text"))
}

func buttonOutline(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.FlexWrap).
		Child(kit.Button("default").Outline().Label("Default")).
		Child(kit.Button("primary").Primary().Outline().Label("Primary")).
		Child(kit.Button("danger").Danger().Outline().Label("Danger")).
		Child(kit.Button("success").Success().Outline().Label("Success"))
}

func buttonSizes(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(kit.Button("xsmall").Xsmall().Label("XSmall")).
		Child(kit.Button("small").Small().Label("Small")).
		Child(kit.Button("medium").Label("Medium")).
		Child(kit.Button("large").Large().Label("Large"))
}

func buttonStates(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.FlexWrap).
		Child(kit.Button("disabled").Disabled(true).Label("Disabled")).
		Child(kit.Button("selected").Selected(true).Label("Selected")).
		Child(kit.Button("compact").Compact().Label("Compact")).
		Child(kit.Button("idle").Loading(false).Label("Not loading")).
		Child(kit.Button("caret").DropdownCaret(true).Label("Caret"))
}

func alertVariants(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.Alert("default", "A default alert.")).
		Child(kit.AlertInfo("info", "An info alert.").Title("Info")).
		Child(kit.AlertSuccess("success", "A success alert.").Title("Success")).
		Child(kit.AlertWarning("warning", "A warning alert.").Title("Warning")).
		Child(kit.AlertError("error", "An error alert.").Title("Error"))
}

func alertShapes(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.AlertInfo("banner", "A banner alert.").Banner()).
		Child(kit.AlertInfo("small", "A small alert.").Small()).
		Child(kit.AlertInfo("large", "A large alert.").Title("Large").Large()).
		Child(kit.AlertInfo("hidden", "A hidden alert.").Visible(false))
}

func avatar(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(kit.Avatar().Name("Ada Lovelace").Xsmall()).
			Child(kit.Avatar().Name("Ada Lovelace").Small()).
			Child(kit.Avatar().Name("Ada Lovelace")).
			Child(kit.Avatar().Name("Ada Lovelace").Large()).
			Child(kit.Avatar())).
		Child(kit.AvatarGroup().
			Child(kit.Avatar().Name("Ada Lovelace")).
			Child(kit.Avatar().Name("Grace Hopper")).
			Child(kit.Avatar().Name("Alan Turing")).
			Child(kit.Avatar().Name("Edsger Dijkstra")).
			Limit(3).
			Ellipsis())
}

func badge(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap3, s.ItemsCenter).
		Child(kit.Badge().Count(3).Child(kit.Avatar().Name("Ada Lovelace"))).
		Child(kit.Badge().Dot().Child(kit.Avatar().Name("Grace Hopper"))).
		Child(kit.Badge().Count(120).Max(99).Child(kit.Avatar().Name("Alan Turing")))
}

func checkbox(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.Checkbox("off").Label("Unchecked")).
		Child(kit.Checkbox("on").Label("Checked").Checked(true)).
		Child(kit.Checkbox("off-disabled").Label("Disabled").Disabled(true)).
		Child(kit.Checkbox("on-disabled").Label("Disabled, checked").Checked(true).Disabled(true)).
		Child(kit.Checkbox("small").Label("Small").Checked(true).Small()).
		Child(kit.Checkbox("large").Label("Large").Checked(true).Large())
}

func checkboxClick(a *App, cx *gk.Cx) gk.Element {
	return kit.Checkbox("toggle").Label("Toggle me").Checked(a.flag).OnClick(a.toggle)
}

func progress(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).W(gk.Px(320)).
		Child(kit.Progress("p0").Value(0)).
		Child(kit.Progress("p25").Value(25)).
		Child(kit.Progress("p60").Value(60)).
		Child(kit.Progress("p100").Value(100)).
		Child(kit.Progress("small").Value(40).Small()).
		Child(kit.Progress("large").Value(40).Large())
}

func progressCircle(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap3, s.ItemsCenter).
		Child(kit.ProgressCircle("c0").Value(0)).
		Child(kit.ProgressCircle("c40").Value(40)).
		Child(kit.ProgressCircle("c100").Value(100)).
		Child(kit.ProgressCircle("small").Value(40).Small()).
		Child(kit.ProgressCircle("large").Value(40).Large())
}

func separator(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).W(gk.Px(320)).
		Child(kit.SeparatorHorizontal()).
		Child(kit.SeparatorHorizontal().Label("Label")).
		Child(kit.SeparatorHorizontalDashed()).
		Child(gk.HFlex(s.Gap3).H(gk.Px(40)).
			Text("left").
			Child(kit.SeparatorVertical()).
			Text("middle").
			Child(kit.SeparatorVerticalDashed()).
			Text("right"))
}

func switchPage(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.Switch("off").Label("Off")).
		Child(kit.Switch("on").Label("On").Checked(true)).
		Child(kit.Switch("off-disabled").Label("Disabled").Disabled(true)).
		Child(kit.Switch("on-disabled").Label("Disabled, on").Checked(true).Disabled(true)).
		Child(kit.Switch("small").Label("Small").Checked(true).Small())
}

func switchClick(a *App, cx *gk.Cx) gk.Element {
	return kit.Switch("toggle").Label("Toggle me").Checked(a.flag).OnClick(a.toggle)
}

func tag(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.HFlex(s.Gap2).
			Child(kit.Tag().Text("Default")).
			Child(kit.TagPrimary().Text("Primary")).
			Child(kit.TagSecondary().Text("Secondary")).
			Child(kit.TagDanger().Text("Danger")).
			Child(kit.TagSuccess().Text("Success")).
			Child(kit.TagWarning().Text("Warning")).
			Child(kit.TagInfo().Text("Info"))).
		Child(gk.HFlex(s.Gap2).
			Child(kit.TagPrimary().Outline().Text("Outline")).
			Child(kit.TagDanger().RoundedFull().Text("Round")).
			Child(kit.TagColor(gk.ColorNameBlue).Text("Blue")).
			Child(kit.TagSuccess().Small().Text("Small")))
}

func labelMore(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.Label("Name").Secondary("(optional)")).
		Child(kit.Label("secret").Masked(true)).
		Child(kit.Label("Highlighted label").Highlights("light"))
}

func radio(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap3).
		Child(gk.VFlex(s.Gap2).
			Child(kit.Radio("off").Label("Off")).
			Child(kit.Radio("on").Label("On").Checked(true)).
			Child(kit.Radio("disabled").Label("Disabled").Disabled(true))).
		Child(kit.RadioGroupVertical("group").Child("One").Child("Two").Child("Three").SelectedIndex(gk.Some(1)))
}

func rating(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.Rating("three").Value(3).Max(5)).
		Child(kit.Rating("none").Value(0).Max(5)).
		Child(kit.Rating("disabled").Value(4).Max(5).Disabled(true))
}

func breadcrumb(a *App, cx *gk.Cx) gk.Element {
	return kit.Breadcrumb().Child("Home").Child("Library").Child("Data")
}

func groupBox(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).W(gk.Px(360)).
		Child(kit.GroupBox().Title(gk.Text("Normal")).Child(kit.Label("Content of the box"))).
		Child(kit.GroupBox().Fill().Title(gk.Text("Fill")).Child(kit.Label("Content of the box"))).
		Child(kit.GroupBox().Outline().Title(gk.Text("Outline")).Child(kit.Label("Content of the box")))
}

func descriptionList(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(480)).
		Child(kit.DescriptionList().Columns(2).Bordered(true).
			Item("Name", "Ada Lovelace", 1).
			Item("Born", "1815", 1).
			Item("Known for", "The first published program", 2))
}

func kbd(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2).
		Child(kit.Kbd(gk.Keystroke{Modifiers: gk.Modifiers{Control: true, Shift: true}, Key: "p"})).
		Child(kit.Kbd(gk.Keystroke{Key: "enter"})).
		Child(kit.Kbd(gk.Keystroke{Modifiers: gk.Modifiers{Alt: true}, Key: "k"}).Outline())
}

func link(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(kit.Link("plain").Href("https://example.com").Text("A link")).
		Child(kit.Link("disabled").Href("https://example.com").Disabled(true).Text("A disabled link"))
}

func skeleton(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).W(gk.Px(240)).
		Child(kit.Skeleton().H(gk.Px(16))).
		Child(kit.Skeleton().Secondary().H(gk.Px(16)))
}
