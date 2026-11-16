// The values family's gallery pages, the Odin twin of ref/src/bin/gallery/values_pages.rs, call for
// call (docs/PROTOCOLS.md sections 13.8 to 13.12): the theme's mode switched by a static that
// takes Option<&mut Window> (Theme::change), the theme's fields edited in place (a kit global's
// public fields, read and written), and bars placed by the plot scales instantiated as library
// objects (ScaleBand, ScaleLinear), made in init and read in the view. Each theme page toggles
// with one button, so its script clicks it twice and the theme is as before afterwards.
package gallery

import "core:fmt"
import gk "gpuikit:gk"

VALUES_PAGES := [?]Page {
	{"values-theme-mode", values_theme_mode_page},
	{"values-theme-edit", values_theme_edit_page},
	{"values-scales", values_scales_page},
	{"calendar-date", calendar_date_page},
	{"values-scroll", values_scroll_page},
	// docs/PROTOCOLS.md 13.19, 13.20
	{"values-settings", values_settings_page},
	{"values-drag", values_drag_page},
	{"values-shadow", values_shadow_page},
	{"values-resizable", values_resizable_page},
	{"values-scrollbar-div", values_scrollbar_div_page},
	{"values-toolbar", values_toolbar_page},
	// the components family, docs/PROTOCOLS.md 11.17 to 11.24
	{"comp6-tooltip", comp6_tooltip},
	{"comp6-tooltip-element", comp6_tooltip_element},
	{"comp6-resize", comp6_resize},
	{"comp6-text-view", comp6_text_view},
	{"comp6-calendar-matcher", comp6_calendar_matcher},
	{"comp6-table-menu", comp6_table_menu},
	{"comp6-tree-menu", comp6_tree_menu},
}

@(private = "file")
Values_Pages :: struct {
	band:          gk.Scale_Band,
	linear:        gk.Scale_Linear,
	font_size:     f32,
	radius:        f32,
	cal:           gk.Calendar_State,
	scroll:        gk.Scroll_Handle,
	// the settings page's values, held by the library for the page to read while it draws
	dark:          gk.Cell(bool),
	compact:       gk.Cell(bool),
	name:          gk.Cell(string),
	size:          gk.Cell(string),
	zoom:          gk.Cell(f64),
	dropped:       string,
	preview:       gk.View,
}
@(private = "file")
vp: Values_Pages

values_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	vp.band = gk.scale_band(cx, {"a", "b", "c", "d"}, {0, 400})->padding_inner(0.2)->build()
	vp.linear = gk.scale_linear(cx, {0, 10}, {0, 200})
	th := gk.theme_global(cx)
	vp.font_size, vp.radius = th->font_size(cx), th->radius(cx)
	// PROTOCOLS.md 13.13, 13.14
	vp.cal = gk.calendar_state(cx)->build()
	vp.cal->set_date(cx, gk.Date_Single{gk.Naive_Date{2024, 2, 29}})
	vp.scroll = gk.scroll_handle(cx)
	vp.scroll->set_offset(cx, {0, -120})
	// PROTOCOLS.md 13.19, 13.20
	vp.dark = gk.cell(cx, false)
	vp.compact = gk.cell(cx, true)
	vp.name = gk.cell(cx, "Ada")
	vp.size = gk.cell(cx, "m")
	vp.zoom = gk.cell(cx, 1.5)
	vp.dropped = "nothing dropped"
	vp.preview = gk.view(a, nil, drag_preview)
	comp6_pages_init(a, cx) // the components family
}

@(private = "file")
sample :: proc() -> gk.Element {
	return gk.div(.Flex, .Flex_Col)->gap(gk.px(12))->
		child(gk.button("primary")->primary()->label("Primary"))->
		child(gk.button("outline")->outline()->label("Outline"))->
		child(gk.label("The theme of the whole window"))->
		child(gk.switch_("sw")->checked(true)->label("Switch"))
}

@(private = "file")
toggle_mode :: proc(a: ^App, cx: ^gk.Cx) {
	dark := gk.theme_global(cx)->is_dark(cx)
	gk.theme_change(cx, gk.Theme_Mode.Light if dark else gk.Theme_Mode.Dark)
}

values_theme_mode_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Flex, .Flex_Col)->gap(gk.px(16))->
		child(gk.button("toggle")->label("Toggle")->on_click(gk.on(toggle_mode)))->
		child(sample())
}

@(private = "file")
toggle_edit :: proc(a: ^App, cx: ^gk.Cx) {
	th := gk.theme_global(cx)
	if th->radius(cx) == vp.radius {
		th->set_radius(cx, 0)
		th->set_font_size(cx, 20)
	} else {
		th->set_radius(cx, vp.radius)
		th->set_font_size(cx, vp.font_size)
	}
}

values_theme_edit_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Flex, .Flex_Col)->gap(gk.px(16))->
		child(gk.button("toggle")->label("Toggle")->on_click(gk.on(toggle_edit)))->
		child(sample())
}

values_scales_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	names := [4]string{"a", "b", "c", "d"}
	vals := [4]f64{3, 7, 5, 9}
	plot := gk.div(.Relative)->w(gk.px(400))->h(gk.px(200))->bg(gk.theme(.Muted))
	w := vp.band->band_width(cx)
	for n, i in names {
		x, _ := vp.band->tick(cx, n).?
		h, _ := vp.linear->tick(cx, vals[i]).?
		plot = plot->child(gk.div(.Absolute)->left(gk.px(x))->bottom(gk.px(0))->w(gk.px(w))->h(gk.px(h))->bg(gk.theme(.Primary)))
	}
	return plot
}

// A calendar whose state was given a date (chrono's NaiveDate, PROTOCOLS.md 13.13).
calendar_date_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(320))->child(gk.calendar(vp.cal))
}

// A list scrolled from code through gpui's ScrollHandle, a scrollbar over it (PROTOCOLS.md 13.14).
values_scroll_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	list := gk.div(.Size_Full)->id("list")->overflow_y_scroll()->track_scroll(vp.scroll)
	for i in 0 ..< 30 {
		list = list->child(gk.div()->h(gk.px(24))->child(gk.text(fmt.tprintf("row %d", i))))
	}
	return gk.div(.Relative)->w(gk.px(300))->h(gk.px(160))->child(list)->child(gk.scrollbar_vertical(vp.scroll))
}

// --- docs/PROTOCOLS.md 13.19, 13.20 ----------------------------------------------------

@(private = "file")
set_dark :: proc(a: ^App, cx: ^gk.Cx, v: bool) {gk.cell_set(cx, vp.dark, v)}
@(private = "file")
set_compact :: proc(a: ^App, cx: ^gk.Cx, v: bool) {gk.cell_set(cx, vp.compact, v)}
@(private = "file")
set_name :: proc(a: ^App, cx: ^gk.Cx, v: gk.Temp_String) {gk.cell_set(cx, vp.name, string(v))}
@(private = "file")
set_size :: proc(a: ^App, cx: ^gk.Cx, v: gk.Temp_String) {gk.cell_set(cx, vp.size, string(v))}
@(private = "file")
set_zoom :: proc(a: ^App, cx: ^gk.Cx, v: f64) {gk.cell_set(cx, vp.zoom, v)}

// The kit's settings page over fields whose values the library holds (value cells): each field
// reads its cell while the page draws, and its handler writes the cell between frames.
values_settings_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	look := gk.setting_group()->title("Look")->
		item(gk.setting_item("Dark", gk.setting_field_bool_switch(vp.dark, gk.on(set_dark))->default_value(false)))->
		item(gk.setting_item("Compact", gk.setting_field_bool_checkbox(vp.compact, gk.on(set_compact))))->
		item(gk.setting_item("Name", gk.setting_field_text_input(vp.name, gk.on(set_name))))->
		item(gk.setting_item("Size", gk.setting_field_text_dropdown({{"s", "Small"}, {"m", "Medium"}}, vp.size, gk.on(set_size))))->
		item(gk.setting_item("Zoom", gk.setting_field_number_number_input({min = 0, max = 4, step = 0.5}, vp.zoom, gk.on(set_zoom))))->
		item(gk.setting_item_render(gk.div()->child(gk.label("Rendered by the app"))))
	return gk.div()->w(gk.px(760))->h(gk.px(480))->
		child(gk.settings(gk.Id("settings"))->sidebar_size_range({100, 300})->page(gk.setting_page("General")->group(look)))
}

@(private = "file")
drag_preview :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(80))->h(gk.px(24))->bg(gk.theme(.Primary))
}

@(private = "file")
dropped :: proc(a: ^App, cx: ^gk.Cx, d: gk.Drag_Value) {
	vp.dropped = fmt.aprintf("dropped %d: %s", d.id, d.text)
}

// Drag and drop over the library's drag value: a source with a preview view, a target that takes
// the drop and shows a style while the value is over it.
values_drag_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	src := gk.div()->id("src")->w(gk.px(120))->h(gk.px(40))->bg(gk.theme(.Muted))->
		on_drag(gk.Drag_Value{7, "row 7"}, vp.preview)->child(gk.label("drag me"))
	dst := gk.div()->id("dst")->w(gk.px(200))->h(gk.px(120))->bg(gk.theme(.Secondary))->
		drag_over(gk.style_refinement_default()->bg(gk.theme(.Accent)))->on_drop(gk.on(dropped))->child(gk.label(vp.dropped))
	return gk.div(.Flex)->gap(gk.px(80))->child(src)->child(dst)
}

// A shadow made by box_shadow, read back as a record and given to shadow().
values_shadow_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	bs := gk.base_box_shadow(cx, 4, 6, 12, 2, gk.hsla(0.6, 0.8, 0.3, 0.6))
	return gk.div()->p(gk.px(24))->child(gk.div()->w(gk.px(160))->h(gk.px(80))->bg(gk.theme(.Background))->shadow({bs}))
}

// The kit's resizable panels: a horizontal group of two panels and the kit's handle between them.
values_resizable_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(600))->h(gk.px(200))->child(gk.kit_h_resizable(gk.Id("rz"))->
		child(gk.resizable_panel()->size(200)->child(gk.div(.Size_Full)->bg(gk.theme(.Muted))->child(gk.label("left"))))->
		child(gk.resizable_panel()->child(gk.div(.Size_Full)->child(gk.label("right")))))
}

// A div that overflows with the kit's own scrollbar (ScrollableElement::overflow_y_scrollbar).
values_scrollbar_div_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	list := gk.div()->w(gk.px(300))->h(gk.px(160))
	for i in 0 ..< 20 {
		list = list->child(gk.div()->h(gk.px(24))->child(gk.text(fmt.tprintf("item %d", i))))
	}
	return list->overflow_y_scrollbar()->id("sd")
}

// A toolbar whose children must be Sizable: buttons, one at a time and as a list.
values_toolbar_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.toolbar(gk.Id("tb"))->child(gk.button("b1")->label("Bold"))->child(gk.button("b2")->label("Italic"))->
		children({gk.button("b3")->label("Code"), gk.button("b4")->label("Wrap")})
}

// --- the components family, docs/PROTOCOLS.md 11.17 to 11.24, the twins of
// ref/src/bin/gallery/values_pages.rs: a tooltip the kit's Tooltip value describes, a resizable
// group whose divider the app draws (from the snapshot of the kit's context) beside one the kit
// draws, labels in Markdown (Text::TextView), calendars with a disabled range and a set of
// disabled days, and a table's and a tree's context menus described with their rows.

@(private = "file")
C6_Pages :: struct {
	zoom_in, zoom_out: gk.Action,
	handles:           gk.Row_Source, // comp6-resize: the divider the app draws
	cal_range:         gk.Calendar_State,
	cal_days:          gk.Calendar_State,
	cells:             gk.Row_Source, // comp6-table-menu
	table:             gk.Table_State,
	tree:              gk.Tree_State, // comp6-tree-menu
	tree_rows:         gk.Row_Source,
}
@(private = "file")
c6p: C6_Pages

@(private = "file")
C6_TREE := [?]gk.Tree_Node{{id = "a", label = "alpha"}, {id = "b", label = "beta"}, {id = "c", label = "gamma"}}

@(private = "file")
comp6_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	c6p.zoom_in = gk.action(cx, "gallery_menu::MenuZoomIn") // the Rust twin's actions (overlay_pages.rs)
	c6p.zoom_out = gk.action(cx, "gallery_menu::MenuZoomOut")
	c6p.handles = gk.row_source(cx, a, 0, comp6_handle_row)
	c6p.cal_range = gk.calendar_state(cx)->
		disabled_matcher(gk.Matcher_Range{gk.Range_Matcher{from = gk.Naive_Date{2024, 2, 12}, to = gk.Naive_Date{2024, 2, 16}}})->build()
	c6p.cal_range->set_date(cx, gk.Date_Single{gk.Naive_Date{2024, 2, 29}})
	c6p.cal_days = gk.calendar_state(cx)->build()
	c6p.cal_days->set_disabled_matcher(cx, gk.Matcher_Custom{[]gk.Naive_Date{{2024, 2, 5}, {2024, 2, 20}}})
	c6p.cal_days->set_date(cx, gk.Date_Single{gk.Naive_Date{2024, 2, 29}})
	columns := [?]gk.Table_Column{gk.table_column("id", "Id", 60), gk.table_column("name", "Name", 100)}
	c6p.cells = gk.table_source(cx, a, 4, columns[:], comp6_cell)
	c6p.table = gk.table_state(cx, c6p.cells)->build()
	c6p.tree = gk.tree_state(cx)->build()
	gk.tree_set_items(cx, c6p.tree, C6_TREE[:])
	c6p.tree_rows = gk.tree_source(cx, a, comp6_tree_row)
}

@(private = "file")
comp6_tooltip :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	tip := gk.kit_tooltip("Save the file")->key_binding(gk.kbd(gk.Keystroke{modifiers = {control = true}, key = "s"}))
	return gk.div()->id("c6tip")->w(gk.px(160))->h(gk.px(40))->bg(gk.theme(.Muted))->tooltip(tip)->child(gk.label("Hover me"))
}

@(private = "file")
comp6_tooltip_element :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	tip := gk.kit_tooltip_element(gk.h_flex(.Gap_1)->child(gk.label("Zoom"))->child(gk.label("in")))->action(c6p.zoom_in, nil)
	return gk.div()->id("c6tipe")->w(gk.px(160))->h(gk.px(40))->bg(gk.theme(.Muted))->tooltip(tip)->child(gk.label("Hover me"))
}

// The divider of the first group: a 3-pixel bar, the primary colour while the pointer owns it.
@(private = "file")
comp6_handle_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	ctx := gk.row_context(cx, c6p.handles, at, gk.Resize_Handle_Context)
	return gk.div(.H_Full)->w(gk.px(3))->bg(gk.theme(.Primary) if ctx.is_active else gk.theme(.Border))
}

@(private = "file")
comp6_resize :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	group :: proc(id: string, r: gk.Resize_Handle_Renderer) -> gk.Element {
		return gk.div()->w(gk.px(600))->h(gk.px(100))->child(gk.h_resizable(gk.Id(id))->with_handle_appearance(r)->
			child(gk.resizable_panel()->size(200)->child(gk.label("left")))->
			child(gk.resizable_panel()->child(gk.label("right"))))
	}
	// A handle of its own (ResizeHandle::with_appearance) at the left edge of a band.
	alone := gk.div(.Relative)->w(gk.px(600))->h(gk.px(40))->bg(gk.theme(.Muted))->
		child(gk.resize_handle(gk.Id("c6h"), .Horizontal)->with_appearance(c6p.handles))
	return gk.v_flex(.Gap_4)->child(group("c6rz", c6p.handles))->
		child(group("c6rk", gk.Resize_Handle_Renderer_Kit.Resize_Handle_Appearance))->child(alone)
}

@(private = "file")
comp6_text_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_3)->w(gk.px(400))->
		child(gk.checkbox(gk.Id("c6cb"))->checked(true)->label(gk.Kit_Text_Text_View{gk.text_view_markdown(gk.Id("c6md1"), "A **bold** label")}))->
		child(gk.alert(gk.Id("c6al"), gk.Kit_Text_Text_View{gk.text_view_markdown(gk.Id("c6md2"), "An alert in *Markdown*")}))
}

@(private = "file")
comp6_calendar_matcher :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div(.Flex, .Gap_4)->child(gk.div()->w(gk.px(320))->child(gk.calendar(c6p.cal_range)))->
		child(gk.div()->w(gk.px(320))->child(gk.calendar(c6p.cal_days)))
}

// A cell of the menu table; the first column's cell carries its row's menu.
@(private = "file")
comp6_cell :: proc(a: ^App, cx: ^gk.Cx, at: gk.Cell_At) -> gk.Element {
	el := gk.label(fmt.tprintf("%d:%d", at.row, at.col))
	if at.col != 0 {
		return el
	}
	return gk.row_menu(el, gk.popup_menu()->menu(fmt.tprintf("Open row %d", at.row), c6p.zoom_in)->
		menu(fmt.tprintf("Delete row %d", at.row), c6p.zoom_out))
}

@(private = "file")
comp6_table_menu :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(300))->h(gk.px(200))->child(gk.data_table(c6p.table)->scrollbar_visible(false, false))
}

@(private = "file")
comp6_tree_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	label := ""
	if e, ok := gk.tree_entry(cx, c6p.tree, at.row); ok {
		label = string(e.label)
	}
	item := gk.list_item(gk.id("c6tree", at.row))->child(gk.label(label))
	return gk.row_menu(item, gk.popup_menu()->menu(fmt.tprintf("Rename %s", label), c6p.zoom_in))
}

@(private = "file")
comp6_tree_menu :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(300))->h(gk.px(160))->child(gk.tree(c6p.tree, c6p.tree_rows)->context_menu(c6p.tree_rows))
}
