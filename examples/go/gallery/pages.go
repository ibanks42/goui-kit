// The gallery's pages in Go: the twin of ref/src/bin/gallery/pages.rs,
// page for page and call for call.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
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
		Child(gk.Label("A plain label")).
		Child(gk.Label("An extra small label").Style(s.TextXs)).
		Child(gk.Label("A muted label").TextColor(theme.MutedForeground))
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
		Child(gk.Button("default").Label("Default")).
		Child(gk.Button("primary").Primary().Label("Primary"))
}

func (a *App) increment(cx *gk.Cx, _ gk.ClickEvent) { a.count++ }

func buttonClick(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(gk.Button("inc").Primary().Label("Increment").OnClick(a.increment)).
		Child(gk.Label(fmt.Sprintf("clicked %d", a.count)))
}

func input(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Div().W(gk.Px(240)).Child(gk.Input(a.name))).
		Child(gk.Label(fmt.Sprintf("hello %s", a.name.Value(cx))))
}

func inputCleanable(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(240)).Child(gk.Input(a.name).Cleanable(true))
}

func list(a *App, cx *gk.Cx) gk.Element {
	return gk.Div(s.WFull).H(gk.Px(160)).Child(gk.UniformList("rows", a.rows).Style(s.SizeFull))
}

func switchBare(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Switch("off")).
		Child(gk.Switch("on").Checked(true))
}

func (a *App) toggle(cx *gk.Cx, on bool) { a.flag = on }

func switchBareClick(a *App, cx *gk.Cx) gk.Element {
	return gk.Switch("toggle").Checked(a.flag).OnClick(a.toggle)
}

func buttonVariants(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.FlexWrap).
		Child(gk.Button("secondary").Secondary().Label("Secondary")).
		Child(gk.Button("danger").Danger().Label("Danger")).
		Child(gk.Button("warning").Warning().Label("Warning")).
		Child(gk.Button("success").Success().Label("Success")).
		Child(gk.Button("info").Info().Label("Info")).
		Child(gk.Button("ghost").Ghost().Label("Ghost")).
		Child(gk.Button("link").Link().Label("Link")).
		Child(gk.Button("text").Text().Label("Text"))
}

func buttonOutline(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.FlexWrap).
		Child(gk.Button("default").Outline().Label("Default")).
		Child(gk.Button("primary").Primary().Outline().Label("Primary")).
		Child(gk.Button("danger").Danger().Outline().Label("Danger")).
		Child(gk.Button("success").Success().Outline().Label("Success"))
}

func buttonSizes(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(gk.Button("xsmall").Xsmall().Label("XSmall")).
		Child(gk.Button("small").Small().Label("Small")).
		Child(gk.Button("medium").Label("Medium")).
		Child(gk.Button("large").Large().Label("Large"))
}

func buttonStates(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2, s.FlexWrap).
		Child(gk.Button("disabled").Disabled(true).Label("Disabled")).
		Child(gk.Button("selected").Selected(true).Label("Selected")).
		Child(gk.Button("compact").Compact().Label("Compact")).
		Child(gk.Button("idle").Loading(false).Label("Not loading")).
		Child(gk.Button("caret").DropdownCaret(true).Label("Caret"))
}

func alertVariants(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Alert("default", "A default alert.")).
		Child(gk.AlertInfo("info", "An info alert.").Title("Info")).
		Child(gk.AlertSuccess("success", "A success alert.").Title("Success")).
		Child(gk.AlertWarning("warning", "A warning alert.").Title("Warning")).
		Child(gk.AlertError("error", "An error alert.").Title("Error"))
}

func alertShapes(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.AlertInfo("banner", "A banner alert.").Banner()).
		Child(gk.AlertInfo("small", "A small alert.").Small()).
		Child(gk.AlertInfo("large", "A large alert.").Title("Large").Large()).
		Child(gk.AlertInfo("hidden", "A hidden alert.").Visible(false))
}

func avatar(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(gk.Avatar().Name("Ada Lovelace").Xsmall()).
			Child(gk.Avatar().Name("Ada Lovelace").Small()).
			Child(gk.Avatar().Name("Ada Lovelace")).
			Child(gk.Avatar().Name("Ada Lovelace").Large()).
			Child(gk.Avatar())).
		Child(gk.AvatarGroup().
			Child(gk.Avatar().Name("Ada Lovelace")).
			Child(gk.Avatar().Name("Grace Hopper")).
			Child(gk.Avatar().Name("Alan Turing")).
			Child(gk.Avatar().Name("Edsger Dijkstra")).
			Limit(3).
			Ellipsis())
}

func badge(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap3, s.ItemsCenter).
		Child(gk.Badge().Count(3).Child(gk.Avatar().Name("Ada Lovelace"))).
		Child(gk.Badge().Dot().Child(gk.Avatar().Name("Grace Hopper"))).
		Child(gk.Badge().Count(120).Max(99).Child(gk.Avatar().Name("Alan Turing")))
}

func checkbox(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Checkbox("off").Label("Unchecked")).
		Child(gk.Checkbox("on").Label("Checked").Checked(true)).
		Child(gk.Checkbox("off-disabled").Label("Disabled").Disabled(true)).
		Child(gk.Checkbox("on-disabled").Label("Disabled, checked").Checked(true).Disabled(true)).
		Child(gk.Checkbox("small").Label("Small").Checked(true).Small()).
		Child(gk.Checkbox("large").Label("Large").Checked(true).Large())
}

func checkboxClick(a *App, cx *gk.Cx) gk.Element {
	return gk.Checkbox("toggle").Label("Toggle me").Checked(a.flag).OnClick(a.toggle)
}

func progress(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).W(gk.Px(320)).
		Child(gk.Progress("p0").Value(0)).
		Child(gk.Progress("p25").Value(25)).
		Child(gk.Progress("p60").Value(60)).
		Child(gk.Progress("p100").Value(100)).
		Child(gk.Progress("small").Value(40).Small()).
		Child(gk.Progress("large").Value(40).Large())
}

func progressCircle(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap3, s.ItemsCenter).
		Child(gk.ProgressCircle("c0").Value(0)).
		Child(gk.ProgressCircle("c40").Value(40)).
		Child(gk.ProgressCircle("c100").Value(100)).
		Child(gk.ProgressCircle("small").Value(40).Small()).
		Child(gk.ProgressCircle("large").Value(40).Large())
}

func separator(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).W(gk.Px(320)).
		Child(gk.SeparatorHorizontal()).
		Child(gk.SeparatorHorizontal().Label("Label")).
		Child(gk.SeparatorHorizontalDashed()).
		Child(gk.HFlex(s.Gap3).H(gk.Px(40)).
			Text("left").
			Child(gk.SeparatorVertical()).
			Text("middle").
			Child(gk.SeparatorVerticalDashed()).
			Text("right"))
}

func switchPage(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Switch("off").Label("Off")).
		Child(gk.Switch("on").Label("On").Checked(true)).
		Child(gk.Switch("off-disabled").Label("Disabled").Disabled(true)).
		Child(gk.Switch("on-disabled").Label("Disabled, on").Checked(true).Disabled(true)).
		Child(gk.Switch("small").Label("Small").Checked(true).Small())
}

func switchClick(a *App, cx *gk.Cx) gk.Element {
	return gk.Switch("toggle").Label("Toggle me").Checked(a.flag).OnClick(a.toggle)
}

func tag(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.HFlex(s.Gap2).
			Child(gk.Tag().Text("Default")).
			Child(gk.TagPrimary().Text("Primary")).
			Child(gk.TagSecondary().Text("Secondary")).
			Child(gk.TagDanger().Text("Danger")).
			Child(gk.TagSuccess().Text("Success")).
			Child(gk.TagWarning().Text("Warning")).
			Child(gk.TagInfo().Text("Info"))).
		Child(gk.HFlex(s.Gap2).
			Child(gk.TagPrimary().Outline().Text("Outline")).
			Child(gk.TagDanger().RoundedFull().Text("Round")).
			Child(gk.TagColor(gk.ColorNameBlue).Text("Blue")).
			Child(gk.TagSuccess().Small().Text("Small")))
}

func labelMore(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Label("Name").Secondary("(optional)")).
		Child(gk.Label("secret").Masked(true)).
		Child(gk.Label("Highlighted label").Highlights("light"))
}

func radio(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap3).
		Child(gk.VFlex(s.Gap2).
			Child(gk.Radio("off").Label("Off")).
			Child(gk.Radio("on").Label("On").Checked(true)).
			Child(gk.Radio("disabled").Label("Disabled").Disabled(true))).
		Child(gk.RadioGroupVertical("group").Child("One").Child("Two").Child("Three").SelectedIndex(gk.Some(1)))
}

func rating(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Rating("three").Value(3).Max(5)).
		Child(gk.Rating("none").Value(0).Max(5)).
		Child(gk.Rating("disabled").Value(4).Max(5).Disabled(true))
}

func breadcrumb(a *App, cx *gk.Cx) gk.Element {
	return gk.Breadcrumb().Child("Home").Child("Library").Child("Data")
}

func groupBox(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap3).W(gk.Px(360)).
		Child(gk.GroupBox().Title(gk.Text("Normal")).Child(gk.Label("Content of the box"))).
		Child(gk.GroupBox().Fill().Title(gk.Text("Fill")).Child(gk.Label("Content of the box"))).
		Child(gk.GroupBox().Outline().Title(gk.Text("Outline")).Child(gk.Label("Content of the box")))
}

func descriptionList(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(480)).
		Child(gk.DescriptionList().Columns(2).Bordered(true).
			Item("Name", "Ada Lovelace", 1).
			Item("Born", "1815", 1).
			Item("Known for", "The first published program", 2))
}

func kbd(a *App, cx *gk.Cx) gk.Element {
	return gk.HFlex(s.Gap2).
		Child(gk.Kbd(gk.Keystroke{Modifiers: gk.Modifiers{Control: true, Shift: true}, Key: "p"})).
		Child(gk.Kbd(gk.Keystroke{Key: "enter"})).
		Child(gk.Kbd(gk.Keystroke{Modifiers: gk.Modifiers{Alt: true}, Key: "k"}).Outline())
}

func link(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).
		Child(gk.Link("plain").Href("https://example.com").Text("A link")).
		Child(gk.Link("disabled").Href("https://example.com").Disabled(true).Text("A disabled link"))
}

func skeleton(a *App, cx *gk.Cx) gk.Element {
	return gk.VFlex(s.Gap2).W(gk.Px(240)).
		Child(gk.Skeleton().H(gk.Px(16))).
		Child(gk.Skeleton().Secondary().H(gk.Px(16)))
}
