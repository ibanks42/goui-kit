package gk

// View-built closures (docs/PROTOCOLS.md section 11.6): a dialog, an alert or a sheet the kit
// keeps open and builds again whenever it draws, described by a procedure of the app.
//
//   confirm :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
//       return gk.dialog()->title(gk.text("Delete?"))->child(gk.label(a.name))->
//           on_ok(gk.on(proc(a: ^App, cx: ^gk.Cx, e: gk.Click_Event) -> bool { a.deleted = true; return true }))
//   }
//   gk.open_dialog(cx, gk.built(cx, a, confirm))          // in a handler
//
// gk.built makes one view per procedure and state (the same pair gives the same view, so a
// dialog opened again reuses it) whose root is the node the procedure returns. The library
// applies that node's builder calls to the value the kit hands in, each time the kit builds the
// dialog. The view is an ordinary view: its handlers run with its state between frames and mark
// it for redraw, so the dialog's content changes while it is open. gk.release(cx, b.view)
// forgets it; a dialog still open on a released view stops the program (exit 2).
//
// A menu (dropdown, context menu, submenu) is described as a value: gk.popup_menu()->menu(..)
// ->separator()->submenu("More", gk.popup_menu()->menu(..)), its items taking actions (gk.action).

import "base:intrinsics"
import "base:runtime"
import "core:time"

// Built is the view behind a view-built closure whose procedure describes a T.
Built :: struct($T: typeid) {
	using view: View,
}

@(private)
Built_Key :: struct {
	fn, state: rawptr,
}

@(private)
built_views: map[Built_Key]u64

// built returns the view that describes a T with `build` over `state`, made on the first call
// for that pair and marked for redraw on every call. Only in a handler or init: a view
// procedure may not make views.
built :: proc(cx: ^Cx, state: ^$S, build: proc(s: ^S, cx: ^Cx) -> $T, loc := #caller_location) -> Built(T) {
	t := check_cx(cx, loc)
	if build_active() {
		stop(loc, "gk.built in a view procedure: make it in a handler or in init (a view procedure only describes)")
	}
	key := Built_Key{rawptr(build), rawptr(state)}
	if built_views == nil {
		built_views = make(map[Built_Key]u64, 16, runtime.heap_allocator())
	}
	if h, ok := built_views[key]; ok {
		if _, alive := views[h]; alive {
			mark_dirty(t, h)
			return Built(T){View{h}}
		}
	}
	tramp :: proc(fn, state: rawptr, cx: ^Cx) -> Element {
		x := (cast(proc(s: ^S, cx: ^Cx) -> T)fn)(cast(^S)state, cx)
		when intrinsics.type_has_field(T, "el") {
			return x.el
		} else {
			return vel(x.nd) // a described value (Popup_Menu): its node is the view's root
		}
	}
	v := view(state, nil, proc(s: ^S, cx: ^Cx) -> Element {return {}}, loc)
	rec := views[v.h]
	rec.view_fn = rawptr(build)
	rec.view_call = tramp
	built_views[key] = v.h
	mark_dirty(t, v.h)
	return Built(T){v}
}

@(private)
build_active :: proc() -> bool {
	return build.active
}

@(private)
put_built :: proc(o: ^Tape, b: Built($T), loc := #caller_location) {
	if b.h == 0 {
		stop(loc, "no view given: make it with gk.built(cx, state, procedure)")
	}
	if _, alive := views[b.h]; !alive {
		stop(loc, "the view gk.built made was released: call gk.built(cx, state, procedure) again each time the dialog, sheet or popover is opened")
	}
	put_u64(o, b.h)
}

// dropdown_menu is the kit's Button with a dropdown menu (DropdownMenu::dropdown_menu): the
// menu is described as a value and built from it each time it opens.
//   gk.dropdown_menu(gk.button("more")->label("More"), gk.popup_menu()->menu("Copy", copy))
dropdown_menu :: proc(button: Button, menu: Popup_Menu, loc := #caller_location) -> Element {
	e := new_node(N_DROPDOWN_BUTTON, loc)
	put_u32(&build.ops, take(button.el))
	put_u32(&build.ops, take_value(menu.nd))
	return e
}

// --- docs/PROTOCOLS.md 11.8 ------------------------------------------------------------

// dropdown_menu_anchored is gk.dropdown_menu with the corner the menu opens from
// (DropdownMenu::dropdown_menu_with_anchor).
dropdown_menu_anchored :: proc(button: Button, anchor: Anchor, menu: Popup_Menu, loc := #caller_location) -> Element {
	e := new_node(N_DROPDOWN_ANCHOR, loc)
	put_u32(&build.ops, take(button.el))
	put_u32(&build.ops, take_value(menu.nd))
	put_u16(&build.ops, u16(anchor))
	return e
}

@(private)
kbd_node :: proc(action: Action, how: u8, key_context: Maybe(string), focus: u64, loc: runtime.Source_Code_Location) -> Element {
	e := new_node(N_KBD_FOR_ACTION, loc)
	put_action(&build.ops, action)
	put_u8(&build.ops, how)
	if c, ok := key_context.?; ok {
		put_u8(&build.ops, 1)
		put_str(&build.ops, c)
	} else {
		put_u8(&build.ops, 0)
	}
	put_u64(&build.ops, focus)
	return e
}

// kbd_for_action draws the keystrokes bound to `action` (in key context `key_context`, if given) as the
// kit's Kbd, or nothing when no binding has it (Kbd::binding_for_action).
kbd_for_action :: proc(action: Action, key_context: Maybe(string) = nil, loc := #caller_location) -> Element {
	return kbd_node(action, 0, key_context, 0, loc)
}

// kbd_for_action_in: the binding that reaches `focus` (Kbd::binding_for_action_in).
kbd_for_action_in :: proc(action: Action, focus: Focus_Handle, loc := #caller_location) -> Element {
	return kbd_node(action, 1, nil, focus.h, loc)
}

// kbd_global_for_action: a binding with no key context (Kbd::global_binding_for_action).
kbd_global_for_action :: proc(action: Action, loc := #caller_location) -> Element {
	return kbd_node(action, 2, nil, 0, loc)
}

// app_menu_bar makes the kit's AppMenuBar (AppMenuBar::new); app_menu_bar_view draws it.
app_menu_bar :: proc(cx: ^Cx, loc := #caller_location) -> App_Menu_Bar {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "gk.app_menu_bar in a view procedure: make it in init or in a handler")
	}
	flush(t, loc)
	req := make(Tape, 0, 8, context.temp_allocator)
	put_u16(&req, C_APP_MENU_BAR_NEW)
	r := Reader{must_call(req[:], loc), 0}
	return App_Menu_Bar{state = {h = rd_u64(&r)}}
}

app_menu_bar_view :: proc(bar: App_Menu_Bar, loc := #caller_location) -> Element {
	e := new_node(N_APP_MENU_BAR, loc)
	put_u64(&build.ops, bar.h)
	return e
}

// --- docs/PROTOCOLS.md 11.10: the library clock --------------------------------------------
//
// std::time::Instant travels as the time since the library's clock began (time.Duration). A toast
// stack's bookkeeping takes the app's reading of it:
//
//   toasts := gk.toast_manager(cx, gk.Toast_Motion{..})->build()
//   toasts->push(cx, id, value, gk.Toast_Options{timeout = 3 * time.Second}, gk.clock_now(cx))
//   adv := toasts->advance(cx, gk.clock_now(cx), false)     // from a timer: presented, ending, removed

// clock_now reads the library's clock.
clock_now :: proc(cx: ^Cx, loc := #caller_location) -> time.Duration {
	check_cx(cx, loc)
	req := make(Tape, 0, 4, context.temp_allocator)
	put_u16(&req, C_CLOCK_NOW)
	r := Reader{must_call(req[:], loc), 0}
	return time.Duration(rd_i64(&r))
}

// --- docs/PROTOCOLS.md 11.11: a popover's trigger --------------------------------------------
//
// Popover::trigger takes an element that is also Selectable (it shows the popover's open state):
// one of these, attached as the popover's trigger. `[lists] unions` in api/overrides.toml names
// this union; the generator lists the kit's Selectable elements (a new one is a compile error here).
Selectable_Element :: union {
	Base_Button,
	Button,
	Dropdown_Button,
	Checkbox,
	Input,
	Input_Group_Button,
	List_Item,
	Searchable_List_Item_Element,
	Sidebar_Footer,
	Sidebar_Header,
	Tab,
}
