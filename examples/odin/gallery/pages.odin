// The gallery's pages in Odin: the twin of ref/src/bin/gallery/pages.rs, page for page and
// call for call. A page is added here in the commit that adds or changes its Rust twin, as
// soon as the package (odin/gk) has every builder the page calls.
package gallery

import "core:fmt"
import gk "gpuikit:gk"

PAGES := [?]Page {
	{"label-basic", label_basic},
	{"div-style", div_style},
	{"div-hover", div_hover},
	{"button-primary", button_primary},
	{"button-click", button_click},
	{"input", input},
	{"input-cleanable", input_cleanable},
	{"list", list},
	{"switch-bare", switch_bare},
	{"switch-bare-click", switch_bare_click},
	{"button-variants", button_variants},
	{"button-outline", button_outline},
	{"button-sizes", button_sizes},
	{"button-states", button_states},
	{"alert-variants", alert_variants},
	{"alert-shapes", alert_shapes},
	{"avatar", avatar},
	{"badge", badge},
	{"checkbox", checkbox},
	{"checkbox-click", checkbox_click},
	{"progress", progress},
	{"progress-circle", progress_circle},
	{"separator", separator},
	{"switch", switch_page},
	{"switch-click", switch_click},
	{"tag", tag},
	{"label-more", label_more},
	{"radio", radio},
	{"rating", rating},
	{"breadcrumb", breadcrumb},
	{"group-box", group_box},
	{"description-list", description_list},
	{"kbd", kbd},
	{"link", link},
	{"skeleton", skeleton},
}

label_basic :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.label("A plain label"))->
		child(gk.label("An extra small label")->style(.Text_Xs))->
		child(gk.label("A muted label")->text_color(gk.theme(.Muted_Foreground)))
}

div_style :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_3, .Flex_Wrap)->
		child(gk.div()->w(gk.px(120))->h(gk.px(60))->bg(gk.hsla(0.6, 0.7, 0.5, 1))->style(.Rounded_Sm, .P_2)->text("solid"))->
		child(gk.div()->w(gk.px(120))->h(gk.px(60))->bg(gk.theme(.Border))->style(.P_1, .Text_Xs)->text("theme border"))->
		child(
			gk.v_flex(.Gap_1)->w(gk.px(200))->
			child(gk.div(.W_Full)->h(gk.px(12))->bg(gk.hsla(0, 0.8, 0.5, 1)))->
			child(gk.div(.W_Full)->h(gk.px(12))->bg(gk.hsla(0.33, 0.8, 0.4, 1)))->
			child(gk.div()->w(gk.px(100))->h(gk.px(12))->bg(gk.hsla(0.12, 0.9, 0.5, 0.5))),
		)
}

div_hover :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	hover := gk.style_refinement_default()->bg(gk.hsla(0.05, 0.9, 0.5, 1))
	return gk.h_flex(.Gap_3)->
		child(gk.div()->w(gk.px(160))->h(gk.px(60))->style(.P_2)->bg(gk.hsla(0.6, 0.5, 0.5, 1))->hover(hover)->text("hover me"))->
		child(gk.div()->w(gk.px(160))->h(gk.px(60))->style(.P_2)->bg(gk.hsla(0.6, 0.5, 0.5, 1))->hover(hover)->text("not hovered"))
}

button_primary :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_2)->
		child(gk.button("default")->label("Default"))->
		child(gk.button("primary")->primary()->label("Primary"))
}

increment :: proc(a: ^App, cx: ^gk.Cx) {
	a.count += 1
}

button_click :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_2, .Items_Center)->
		child(gk.button("inc")->primary()->label("Increment")->on_click(gk.on(increment)))->
		child(gk.label(fmt.tprintf("clicked %d", a.count)))
}

input :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.div()->w(gk.px(240))->child(gk.input(a.name)))->
		child(gk.label(fmt.tprintf("hello %s", gk.value(a.name, cx))))
}

input_cleanable :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(240))->child(gk.input(a.name)->cleanable(true))
}

list :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.W_Full)->h(gk.px(160))->child(gk.uniform_list("rows", a.rows)->style(.Size_Full))
}

switch_bare :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.switch_("off"))->
		child(gk.switch_("on")->checked(true))
}

toggle :: proc(a: ^App, cx: ^gk.Cx, on: bool) {
	a.flag = on
}

switch_bare_click :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.switch_("toggle")->checked(a.flag)->on_click(gk.on(toggle))
}

button_variants :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_2, .Flex_Wrap)->
		child(gk.button("secondary")->secondary()->label("Secondary"))->
		child(gk.button("danger")->danger()->label("Danger"))->
		child(gk.button("warning")->warning()->label("Warning"))->
		child(gk.button("success")->success()->label("Success"))->
		child(gk.button("info")->info()->label("Info"))->
		child(gk.button("ghost")->ghost()->label("Ghost"))->
		child(gk.button("link")->link()->label("Link"))->
		child(gk.button("text")->text()->label("Text"))
}

button_outline :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_2, .Flex_Wrap)->
		child(gk.button("default")->outline()->label("Default"))->
		child(gk.button("primary")->primary()->outline()->label("Primary"))->
		child(gk.button("danger")->danger()->outline()->label("Danger"))->
		child(gk.button("success")->success()->outline()->label("Success"))
}

button_sizes :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_2, .Items_Center)->
		child(gk.button("xsmall")->xsmall()->label("XSmall"))->
		child(gk.button("small")->small()->label("Small"))->
		child(gk.button("medium")->label("Medium"))->
		child(gk.button("large")->large()->label("Large"))
}

button_states :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_2, .Flex_Wrap)->
		child(gk.button("disabled")->disabled(true)->label("Disabled"))->
		child(gk.button("selected")->selected(true)->label("Selected"))->
		child(gk.button("compact")->compact()->label("Compact"))->
		child(gk.button("idle")->loading(false)->label("Not loading"))->
		child(gk.button("caret")->dropdown_caret(true)->label("Caret"))
}

alert_variants :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.alert("default", "A default alert."))->
		child(gk.alert_info("info", "An info alert.")->title("Info"))->
		child(gk.alert_success("success", "A success alert.")->title("Success"))->
		child(gk.alert_warning("warning", "A warning alert.")->title("Warning"))->
		child(gk.alert_error("error", "An error alert.")->title("Error"))
}

alert_shapes :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.alert_info("banner", "A banner alert.")->banner())->
		child(gk.alert_info("small", "A small alert.")->small())->
		child(gk.alert_info("large", "A large alert.")->title("Large")->large())->
		child(gk.alert_info("hidden", "A hidden alert.")->visible(false))
}

avatar :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_3)->
		child(
			gk.h_flex(.Gap_2, .Items_Center)->
			child(gk.avatar()->name("Ada Lovelace")->xsmall())->
			child(gk.avatar()->name("Ada Lovelace")->small())->
			child(gk.avatar()->name("Ada Lovelace"))->
			child(gk.avatar()->name("Ada Lovelace")->large())->
			child(gk.avatar()),
		)->
		child(
			gk.avatar_group()->
			child(gk.avatar()->name("Ada Lovelace"))->
			child(gk.avatar()->name("Grace Hopper"))->
			child(gk.avatar()->name("Alan Turing"))->
			child(gk.avatar()->name("Edsger Dijkstra"))->
			limit(3)->
			ellipsis(),
		)
}

badge :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_3, .Items_Center)->
		child(gk.badge()->count(3)->child(gk.avatar()->name("Ada Lovelace")))->
		child(gk.badge()->dot()->child(gk.avatar()->name("Grace Hopper")))->
		child(gk.badge()->count(120)->max(99)->child(gk.avatar()->name("Alan Turing")))
}

checkbox :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.checkbox("off")->label("Unchecked"))->
		child(gk.checkbox("on")->label("Checked")->checked(true))->
		child(gk.checkbox("off-disabled")->label("Disabled")->disabled(true))->
		child(gk.checkbox("on-disabled")->label("Disabled, checked")->checked(true)->disabled(true))->
		child(gk.checkbox("small")->label("Small")->checked(true)->small())->
		child(gk.checkbox("large")->label("Large")->checked(true)->large())
}

checkbox_click :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.checkbox("toggle")->label("Toggle me")->checked(a.flag)->on_click(gk.on(toggle))
}

progress :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_3)->w(gk.px(320))->
		child(gk.progress("p0")->value(0))->
		child(gk.progress("p25")->value(25))->
		child(gk.progress("p60")->value(60))->
		child(gk.progress("p100")->value(100))->
		child(gk.progress("small")->value(40)->small())->
		child(gk.progress("large")->value(40)->large())
}

progress_circle :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_3, .Items_Center)->
		child(gk.progress_circle("c0")->value(0))->
		child(gk.progress_circle("c40")->value(40))->
		child(gk.progress_circle("c100")->value(100))->
		child(gk.progress_circle("small")->value(40)->small())->
		child(gk.progress_circle("large")->value(40)->large())
}

separator :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_3)->w(gk.px(320))->
		child(gk.separator_horizontal())->
		child(gk.separator_horizontal()->label("Label"))->
		child(gk.separator_horizontal_dashed())->
		child(
			gk.h_flex(.Gap_3)->h(gk.px(40))->
			text("left")->
			child(gk.separator_vertical())->
			text("middle")->
			child(gk.separator_vertical_dashed())->
			text("right"),
		)
}

switch_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.switch_("off")->label("Off"))->
		child(gk.switch_("on")->label("On")->checked(true))->
		child(gk.switch_("off-disabled")->label("Disabled")->disabled(true))->
		child(gk.switch_("on-disabled")->label("Disabled, on")->checked(true)->disabled(true))->
		child(gk.switch_("small")->label("Small")->checked(true)->small())
}

switch_click :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.switch_("toggle")->label("Toggle me")->checked(a.flag)->on_click(gk.on(toggle))
}

tag :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(
			gk.h_flex(.Gap_2)->
			child(gk.tag()->text("Default"))->
			child(gk.tag_primary()->text("Primary"))->
			child(gk.tag_secondary()->text("Secondary"))->
			child(gk.tag_danger()->text("Danger"))->
			child(gk.tag_success()->text("Success"))->
			child(gk.tag_warning()->text("Warning"))->
			child(gk.tag_info()->text("Info")),
		)->
		child(
			gk.h_flex(.Gap_2)->
			child(gk.tag_primary()->outline()->text("Outline"))->
			child(gk.tag_danger()->rounded_full()->text("Round"))->
			child(gk.tag_color(.Blue)->text("Blue"))->
			child(gk.tag_success()->small()->text("Small")),
		)
}

label_more :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.label("Name")->secondary("(optional)"))->
		child(gk.label("secret")->masked(true))->
		child(gk.label("Highlighted label")->highlights("light"))
}

radio :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_3)->
		child(
			gk.v_flex(.Gap_2)->
			child(gk.radio("off")->label("Off"))->
			child(gk.radio("on")->label("On")->checked(true))->
			child(gk.radio("disabled")->label("Disabled")->disabled(true)),
		)->
		child(gk.radio_group_vertical("group")->child("One")->child("Two")->child("Three")->selected_index(1))
}

rating :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.rating("three")->value(3)->max(5))->
		child(gk.rating("none")->value(0)->max(5))->
		child(gk.rating("disabled")->value(4)->max(5)->disabled(true))
}

breadcrumb :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.breadcrumb()->child("Home")->child("Library")->child("Data")
}

group_box :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_3)->w(gk.px(360))->
		child(gk.group_box()->title(gk.text("Normal"))->child(gk.label("Content of the box")))->
		child(gk.group_box()->fill()->title(gk.text("Fill"))->child(gk.label("Content of the box")))->
		child(gk.group_box()->outline()->title(gk.text("Outline"))->child(gk.label("Content of the box")))
}

description_list :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(480))->
		child(
			gk.description_list()->columns(2)->bordered(true)->
			item("Name", "Ada Lovelace", 1)->
			item("Born", "1815", 1)->
			item("Known for", "The first published program", 2),
		)
}

kbd :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Gap_2)->
		child(gk.kbd(gk.Keystroke{modifiers = {control = true, shift = true}, key = "p"}))->
		child(gk.kbd(gk.Keystroke{key = "enter"}))->
		child(gk.kbd(gk.Keystroke{modifiers = {alt = true}, key = "k"})->outline())
}

link :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->
		child(gk.link("plain")->href("https://example.com")->text("A link"))->
		child(gk.link("disabled")->href("https://example.com")->disabled(true)->text("A disabled link"))
}

skeleton :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->w(gk.px(240))->
		child(gk.skeleton()->h(gk.px(16)))->
		child(gk.skeleton()->secondary()->h(gk.px(16)))
}
