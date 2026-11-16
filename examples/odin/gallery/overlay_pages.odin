// The overlays family's gallery pages, the Odin twin of ref/src/bin/gallery/overlay_pages.rs,
// call for call (docs/PROTOCOLS.md section 11.6): a dialog, an alert and two sheets opened by a
// handler with a view-built procedure (gk.built), a dropdown button and a context menu with a
// submenu whose menus are described values. The page's script opens each (pages.txt).
package gallery

import "core:fmt"
import gk "gpuikit:gk"

OVERLAY_PAGES := [?]Page {
	{"overlay-dialog", overlay_dialog_page},
	{"overlay-alert", overlay_alert_page},
	{"overlay-sheet", overlay_sheet_page},
	{"overlay-sheet-left", overlay_sheet_left_page},
	{"overlay-dropdown", overlay_dropdown_page},
	{"overlay-context", overlay_context_page},
	{"dock-panels", overlay_dock_page},
	{"dock-split", dock_split_page},
	{"dock-hooks", dock_hooks_page},
	{"dock-restore", dock_restore_page},
	{"menu-kbd", menu_kbd_page},
	{"menu-dropdown-anchor", menu_anchor_page},
	{"dock-skin", dock_skin_page},
	{"dock-renderer", dock_renderer_page},
	{"overlay-popover-open", popover_open_page},
}

// The menus' actions, named once (an app action: no key binding, so no shortcut is shown).
@(private = "file")
Menu_Actions :: struct {
	copy, paste, delete, pinned, zoom_in, zoom_out, kbd: gk.Action,
}
@(private = "file")
acts: Menu_Actions

overlay_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	acts = {
		copy     = gk.action(cx, "gallery_menu::MenuCopy"),
		paste    = gk.action(cx, "gallery_menu::MenuPaste"),
		delete   = gk.action(cx, "gallery_menu::MenuDelete"),
		pinned   = gk.action(cx, "gallery_menu::MenuPinned"),
		zoom_in  = gk.action(cx, "gallery_menu::MenuZoomIn"),
		zoom_out = gk.action(cx, "gallery_menu::MenuZoomOut"),
		kbd      = gk.action(cx, "gallery_menu::MenuKbd"), // bound to ctrl-s; no menu shows it
	}
	overlay_dock_init(a, cx)
	dock_r3_init(a, cx)
	dock_r4_init(a, cx)
	gk.bind_keys(cx, {"ctrl-s", acts.kbd, ""}) // the page menu-kbd shows it
}

@(private = "file")
open_button :: proc(h: gk.Handler) -> gk.Element {
	return gk.button("open")->label("Open")->on_click(h)
}

@(private = "file")
menu :: proc() -> gk.Popup_Menu {
	return gk.popup_menu()->
		menu("Copy", acts.copy)->
		menu("Paste", acts.paste)->
		separator()->
		label("Danger")->
		menu_with_check("Pinned", true, acts.pinned)->
		menu_with_disabled("Delete", acts.delete, true)
}

// --- the dialog: a procedure that describes it, run whenever the kit builds it

@(private = "file")
delete_dialog :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	return gk.dialog()->
		title(gk.label("Delete report.txt?"))->
		entrance(.None)->
		child(gk.label("The file will be gone for good."))->
		footer(gk.label("Press Escape to keep it."))
}

overlay_dialog_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return open_button(gk.on(proc(a: ^App, cx: ^gk.Cx) {gk.open_dialog(cx, gk.built(cx, a, delete_dialog))}))
}

@(private = "file")
unsaved_alert :: proc(a: ^App, cx: ^gk.Cx) -> gk.Alert_Dialog {
	return gk.alert_dialog()->
		title(gk.label("Unsaved changes"))->
		description(gk.label("Leave without saving?"))->
		entrance(.None)->
		show_cancel(true)
}

overlay_alert_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return open_button(gk.on(proc(a: ^App, cx: ^gk.Cx) {gk.open_alert_dialog(cx, gk.built(cx, a, unsaved_alert))}))
}

@(private = "file")
settings_sheet :: proc(a: ^App, cx: ^gk.Cx) -> gk.Sheet {
	return gk.sheet()->title(gk.label("Settings"))->child(gk.label("The sheet's body"))
}

overlay_sheet_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return open_button(gk.on(proc(a: ^App, cx: ^gk.Cx) {gk.open_sheet(cx, gk.built(cx, a, settings_sheet))}))
}

@(private = "file")
files_sheet :: proc(a: ^App, cx: ^gk.Cx) -> gk.Sheet {
	return gk.sheet()->title(gk.label("Files"))->resizable(false)->child(gk.label("On the left"))
}

overlay_sheet_left_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return open_button(gk.on(proc(a: ^App, cx: ^gk.Cx) {gk.open_sheet_at(cx, .Left, gk.built(cx, a, files_sheet))}))
}

// --- menus: described values, built by the library each time the menu opens

overlay_dropdown_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.dropdown_menu(gk.button("open")->label("Open"), menu())
}

overlay_context_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	zoom := gk.popup_menu()->menu("Zoom in", acts.zoom_in)->menu("Zoom out", acts.zoom_out)
	return gk.div(.Border_1)->w(gk.px(300))->h(gk.px(120))->
		child(gk.label("Right-click here"))->
		context_menu(menu()->separator()->submenu("Zoom", zoom))
}

// --- a docked layout of three Odin panels (docs/PROTOCOLS.md 11.7): two tabs in the center, one
// in the left dock, in the kit's styled dock area

Text_Panel :: struct {
	body: string,
}

@(private = "file")
panel_bodies := [3]Text_Panel{{"src/main.odin"}, {"package main"}, {"main :: proc()"}}
@(private = "file")
dock: gk.Dock_Area

@(private = "file")
text_panel_view :: proc(p: ^Text_Panel, cx: ^gk.Cx) -> gk.Element {
	return gk.label(p.body)
}

overlay_dock_init :: proc(a: ^App, cx: ^gk.Cx) {
	dock = gk.dock_area_skinned(cx, "main")
	files := gk.panel(cx, gk.view(&panel_bodies[0], nil, text_panel_view), {name = "files", title = "Files"})
	editor := gk.panel(cx, gk.view(&panel_bodies[1], nil, text_panel_view), {name = "editor", title = "Editor"})
	outline := gk.panel(cx, gk.view(&panel_bodies[2], nil, text_panel_view), {name = "outline", title = "Outline"})
	dock->set_center(cx, gk.dock_layout_tabs()->panel(files)->panel(editor))
	dock->set_dock(cx, .Left, gk.dock_layout_tabs()->panel(outline))
}

overlay_dock_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(600))->h(gk.px(300))->child(gk.dock_view(dock))
}

// --- docs/PROTOCOLS.md 11.8 ------------------------------------------------------------

@(private = "file")
r3_bodies := [?]Text_Panel{{"src/main.odin"}, {"package main"}, {"main :: proc()"}, {"Panel body"}, {"v0.9.1"}}
@(private = "file")
split_dock, hooks_dock, restore_dock, skin_dock: gk.Dock_Area
@(private = "file")
restored_bodies: [3]Text_Panel

// The tab group's node id read back from the layout, a split at it and the split's sizes.
@(private = "file")
dock_split_init :: proc(a: ^App, cx: ^gk.Cx) {
	split_dock = gk.dock_area_skinned(cx, "split")
	files := gk.panel(cx, gk.view(&r3_bodies[0], nil, text_panel_view), {name = "files", title = "Files"})
	editor := gk.panel(cx, gk.view(&r3_bodies[1], nil, text_panel_view), {name = "editor", title = "Editor"})
	outline := gk.panel(cx, gk.view(&r3_bodies[2], nil, text_panel_view), {name = "outline", title = "Outline"})
	split_dock->set_center(cx, gk.dock_layout_tabs()->panel(files)->panel(editor)->panel(outline))
	tree, _ := split_dock->layout(cx, .Center).? // a read: the layout above is applied first
	group, _ := tree->find_panel_node(cx, gk.panel_id(cx, editor)).?
	split_dock->split_at(cx, group, gk.panel_id(cx, outline), .Right)
	split_dock->split_at(cx, group, gk.panel_id(cx, editor), .Bottom)
}

// A panel's title suffix and toolbar buttons, from views the app pushed.
@(private = "file")
toolbar_view :: proc(p: ^Text_Panel, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex()->child(gk.button("run")->label("Run"))->child(gk.button("stop")->label("Stop"))
}

@(private = "file")
hooked_menu :: proc(p: ^Text_Panel, cx: ^gk.Cx) -> gk.Popup_Menu {
	return gk.popup_menu()->separator()->menu("Copy", acts.copy)
}

@(private = "file")
dock_hooks_init :: proc(a: ^App, cx: ^gk.Cx) {
	hooks_dock = gk.dock_area_skinned(cx, "hooks")
	data := gk.Panel_Data{name = "hooked", title = "Hooked"}
	data.suffix = gk.view(&r3_bodies[4], nil, text_panel_view)
	data.toolbar = gk.view(&r3_bodies[3], nil, toolbar_view)
	data.menu = gk.built(cx, &r3_bodies[3], hooked_menu)
	hooked := gk.panel(cx, gk.view(&r3_bodies[3], nil, text_panel_view), data)
	// panel_view: the kit's PanelHandle over the panel, through which the skin draws its title,
	// suffix, toolbar and menu (->panel adds the bare panel, drawn with its name)
	hooks_dock->set_center(cx, gk.dock_layout_tabs()->panel_view(hooked))
}

// A saved layout loaded: the registry asks for each panel, the app fills it.
@(private = "file")
restore_panel :: proc(a: ^App, cx: ^gk.Cx, b: gk.Panel_Build) {
	switch b.name {
	case "files":
		restored_bodies[0] = {"src/main.odin"}
		gk.panel_fill(cx, b.panel, gk.view(&restored_bodies[0], nil, text_panel_view), {name = "files", title = "Files"})
	case "editor":
		restored_bodies[1] = {"package main"}
		gk.panel_fill(cx, b.panel, gk.view(&restored_bodies[1], nil, text_panel_view), {name = "editor", title = "Editor"})
	case "outline":
		restored_bodies[2] = {"main :: proc()"}
		gk.panel_fill(cx, b.panel, gk.view(&restored_bodies[2], nil, text_panel_view), {name = "outline", title = "Outline"})
	}
}

@(private = "file")
leaf :: proc(name: string) -> gk.Panel_State {
	return {panel_name = name, info = gk.Panel_Info_Panel{}}
}

@(private = "file")
dock_restore_init :: proc(a: ^App, cx: ^gk.Cx) {
	for name in ([]string{"files", "editor", "outline"}) {
		gk.register_panel(cx, name, gk.on(restore_panel))
	}
	restore_dock = gk.dock_area_skinned(cx, "restored")
	left := []gk.Panel_State{leaf("files")}
	right := []gk.Panel_State{leaf("editor"), leaf("outline")}
	groups := []gk.Panel_State{
		{panel_name = "TabPanel", children = left, info = gk.Panel_Info_Tabs{0}},
		{panel_name = "TabPanel", children = right, info = gk.Panel_Info_Tabs{1}},
	}
	state := gk.Dock_Area_State {
		center = {panel_name = "StackPanel", children = groups, info = gk.Panel_Info_Stack{sizes = {200, 400}, axis = 0}},
	}
	restore_dock->load(cx, state)
}

dock_r3_init :: proc(a: ^App, cx: ^gk.Cx) {
	skin_dock = gk.dock_area_skinned(cx, "skin")
	one := gk.panel(cx, gk.view(&r3_bodies[0], nil, text_panel_view), {name = "files", title = "Files"})
	skin_dock->set_center(cx, gk.dock_layout_tabs()->panel(one))
	gk.dock_skin_set_panel_style(cx, skin_dock, .Tab_Bar) // a tab bar even for one panel
	gk.dock_skin_set_close_button_visible(cx, skin_dock, false)
	dock_split_init(a, cx)
	dock_hooks_init(a, cx)
	dock_restore_init(a, cx)
}

dock_split_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(600))->h(gk.px(300))->child(gk.dock_view(split_dock))
}

dock_hooks_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(600))->h(gk.px(300))->child(gk.dock_view(hooks_dock))
}

dock_restore_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(600))->h(gk.px(300))->child(gk.dock_view(restore_dock))
}

// The keystrokes bound to an action, as the kit's Kbd: bound, unbound (nothing), with no context.
menu_kbd_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex()->
		child(gk.kbd_for_action(acts.kbd))->
		child(gk.kbd_for_action(acts.paste))->
		child(gk.kbd_global_for_action(acts.kbd))
}

// A dropdown that opens from its top right corner.
menu_anchor_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.dropdown_menu_anchored(gk.button("open")->label("Open"), .Top_Right, menu())
}

dock_skin_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(600))->h(gk.px(300))->child(gk.dock_view(skin_dock))
}

// --- docs/PROTOCOLS.md 11.9, 11.12 ----------------------------------------------------------

// A dock drawn by the app's renderer: a padded frame, each dock's chrome a title over its content
// (the slot), the tab bar a label. Each hook asks once per key and context; the answer is a view.
@(private = "file")
r4_bodies := [?]Text_Panel{{"src/main.odin"}, {"main :: proc()"}}
@(private = "file")
render_dock: gk.Dock_Area
@(private = "file")
Chrome_State :: struct {
	placement: gk.Dock_Placement,
	open:      bool,
}
@(private = "file")
chromes: [4]Chrome_State
@(private = "file")
chrome_views: [4]gk.View
@(private = "file")
frame_v, tabs_v: gk.View

@(private = "file")
chrome_view :: proc(c: ^Chrome_State, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full)->child(gk.label(fmt.tprintf("%v open=%v", c.placement, c.open)))->child(gk.dock_slot())
}

@(private = "file")
frame_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.Size_Full, .P_1)
}

@(private = "file")
tabs_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.h_flex(.W_Full)->child(gk.label("tabs of the app"))
}

@(private = "file")
render_asked :: proc(a: ^App, cx: ^gk.Cx, q: gk.Dock_Render) {
	#partial switch q.hook {
	case .Frame:
		gk.dock_render_answer(cx, q.renderer, q.hook, q.key, frame_v)
	case .Dock:
		i := int(q.placement)
		chromes[i] = {q.placement, q.flags & 1 != 0}
		if chrome_views[i].h == 0 {
			chrome_views[i] = gk.view(&chromes[i], nil, chrome_view)
		}
		gk.dock_render_answer(cx, q.renderer, q.hook, q.key, chrome_views[i])
	case .Tab_Bar:
		gk.dock_render_answer(cx, q.renderer, q.hook, q.key, tabs_v)
	}
}

dock_r4_init :: proc(a: ^App, cx: ^gk.Cx) {
	frame_v = gk.view(a, nil, frame_view)
	tabs_v = gk.view(a, nil, tabs_view)
	r := gk.dock_renderer(cx, {.Frame, .Dock, .Tab_Bar}, gk.on(render_asked))
	render_dock = gk.dock_area(cx, "rendered", nil)->with_renderer(r)->build()
	files := gk.panel(cx, gk.view(&r4_bodies[0], nil, text_panel_view), {name = "files", title = "Files"})
	outline := gk.panel(cx, gk.view(&r4_bodies[1], nil, text_panel_view), {name = "outline", title = "Outline"})
	render_dock->set_center(cx, gk.dock_layout_tabs()->panel(files))
	render_dock->set_dock(cx, .Left, gk.dock_layout_tabs()->panel(outline))
}

dock_renderer_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(600))->h(gk.px(300))->child(gk.dock_view(render_dock))
}

// A popover the app holds open (Popover::open from the app's state), its trigger a kit button.
popover_open_page :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.popover("pop")->trigger(gk.button("trigger")->label("Open"))->open(true)->child(gk.label("inside the popover"))
}
