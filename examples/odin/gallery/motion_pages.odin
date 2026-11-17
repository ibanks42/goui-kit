// The platform and motion families' gallery pages, the Odin twin of
// ref/src/bin/gallery/motion_pages.rs, call for call (docs/PROTOCOLS.md 13.30): gpui's with_animation
// and a chain of animations with the kit's easing curves, the kit's EffectTransition, and the kit's
// motion values (transition, spring, keyframes, presence, a TransitionState kept with the page's
// data) sampled while the library draws. Offscreen the clock moves only on `advance`, so every frame
// is at a fixed time and the pages compare byte for byte with Rust.
package gallery

import "core:fmt"
import "core:os"
import "core:time"
import gk "gpuikit:gk"

MOTION_PAGES := [?]Page {
	{"motion-animation", motion_animation},
	{"motion-chain", motion_chain},
	{"motion-effect", motion_effect},
	{"motion-transition", motion_transition},
	{"motion-spring", motion_spring},
	{"motion-keyframes", motion_keyframes},
	{"motion-presence", motion_presence},
	{"motion-focus-trap", motion_focus_trap},
	// PROTOCOLS.md 13.31: gpui's element leftovers
	{"elem-drop", elem_drop},
	{"elem-image", elem_image},
	{"elem-scope", elem_scope},
	{"elem-vlist", elem_vlist},
}

@(private = "file")
Motion_Pages :: struct {
	frames: gk.Keyframes,
	state:  gk.Transition_State,
	focus:  gk.Focus_Handle,
	// the elem-* pages
	cache:  gk.Image_Cache,
	pic:    gk.Render_Image, // made once from pixels: gpui keeps one texture for it
	scroll: gk.Scroll_Handle,
	anchor: gk.Scroll_Anchor,
	scope:  gk.Text_Selection_Scope,
	rows:   gk.Row_Source,
	hrows:  gk.Row_Source,
}
@(private = "file")
mp: Motion_Pages

motion_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	frames := [?]gk.Keyframe_F32{{0, 40, gk.Easing_Linear{}}, {0.5, 200, gk.Easing_Linear{}}, {1, 100, gk.Easing_Linear{}}}
	mp.frames = gk.retain(cx, gk.keyframes_try_new(cx, frames[:]).value.?) // a fallible constructor hands out a temporary handle
	mp.state = gk.transition_state_default(cx)
	mp.focus = gk.focus_handle(cx)
	mp.cache = gk.image_cache(cx)
	mp.pic = gk.render_image(cx, checker())
	mp.scroll = gk.scroll_handle(cx)
	mp.anchor = gk.scroll_anchor(cx, mp.scroll)
	mp.scope = gk.text_selection_scope_id(cx)
	mp.rows = gk.row_source(cx, a, 200, elem_row, initial = 0)
	mp.hrows = gk.row_source(cx, a, 8, elem_row, initial = 0)
	// ImageSource's Path variant reads this file (written where the gallery runs, as the Rust twin does)
	_ = os.write_entire_file(ELEM_PNG_PATH, ELEM_PNG[:])
}

@(private = "file")
RED :: gk.Color{h = 0, s = 0.7, l = 0.5, a = 1}
@(private = "file")
BLUE :: gk.Color{h = 0.6, s = 0.7, l = 0.5, a = 1}

@(private = "file")
toggle :: proc(a: ^App, label: string) -> gk.Element {
	return gk.button("toggle")->label(label)->on_click(gk.on(increment))
}

@(private = "file")
box :: proc() -> gk.Div {
	return gk.div(.Relative)->size(gk.px(40))->bg(gk.Color{h = 0.33, s = 0.6, l = 0.45, a = 1})
}

motion_animation :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Gap_2)->w(gk.px(320))->h(gk.px(120))->
		child(gk.with_animation(box(), "slide", {ms = 400, ease = {kind = .Ease_In_Out}},
			{prop = .Left, from = f32(0), to = f32(200)}, {prop = .Opacity, from = f32(0.2), to = f32(1)}))->
		child(gk.with_animation(gk.div()->size(gk.px(30))->bg(RED), "colour", {ms = 400, ease = {kind = .Cubic_Bezier, p = {0.4, 0, 0.2, 1}}},
			{prop = .Bg, from = RED, to = BLUE}, {prop = .Width, from = f32(30), to = f32(120)}))
}

motion_chain :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	anims := [?]gk.Animation{{ms = 200, ease = {kind = .Ease_Out_Cubic}}, {ms = 200, ease = {kind = .Bounce, bounce = .Quadratic}}}
	return gk.div()->w(gk.px(320))->h(gk.px(80))->
		child(gk.with_animations(box(), "chain", anims[:],
			{prop = .Left, from = f32(0), to = f32(150), step = 1}, {prop = .Height, from = f32(40), to = f32(70), step = 2}))
}

motion_effect :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	e := gk.effect_transition(300 * time.Millisecond)->fade(0, 1)->slide_x(-40, 0)->width(60, 180)
	return gk.div()->w(gk.px(320))->h(gk.px(80))->
		child(gk.effect(gk.div(.Relative)->h(gk.px(40))->bg(BLUE)->child(gk.label("enter")), e, "enter"))
}

motion_transition :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	open := a.count % 2 == 1
	return gk.v_flex(.Gap_2)->w(gk.px(320))->
		child(toggle(a, "Toggle"))->
		child(gk.motion(gk.div()->w(gk.px(120))->h(gk.px(40))->bg(RED),
			{.Height, gk.To_Transition{"h", f32(open ? 120 : 40), gk.transition(300 * time.Millisecond)}},
			{.Bg, gk.To_Transition{"c", open ? BLUE : RED, gk.transition(300 * time.Millisecond)}}))
}

motion_spring :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	open := a.count % 2 == 1
	return gk.v_flex(.Gap_2)->w(gk.px(320))->
		child(toggle(a, "Toggle"))->
		child(gk.motion(box(), {.Left, gk.To_Spring{"x", open ? 200 : 0, gk.spring(250 * time.Millisecond)->with_damping(0.5)}}))
}

motion_keyframes :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.div()->w(gk.px(320))->h(gk.px(60))->
		child(gk.motion(gk.div()->h(gk.px(40))->bg(BLUE), {.Width, gk.At_Keyframes{"kf", mp.frames, gk.timing(600 * time.Millisecond)}}))
}

motion_presence :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	shown := a.count % 2 == 1
	return gk.v_flex(.Gap_2)->w(gk.px(320))->
		child(toggle(a, "Show"))->
		child(gk.motion(gk.div()->size(gk.px(60))->bg(RED),
			{.Opacity, gk.At_Presence{gk.presence("p", shown)->transition(gk.transition(300 * time.Millisecond)), f32(0), f32(1)}},
			{.Width, gk.To_State{mp.state, shown ? 200 : 60, gk.transition(300 * time.Millisecond)}}))
}

motion_focus_trap :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.focus_trap(gk.div(.Flex, .Gap_2, .P_2, .Border_1)->w(gk.px(320))->track_focus(mp.focus)->
		child(gk.button("one")->label("One"))->child(gk.button("two")->label("Two")), "trap", mp.focus)
}

// --- PROTOCOLS.md 13.31: gpui's element leftovers, the Odin twin of the elem-* pages of
// motion_pages.rs. The drop targets take data where Rust gives closures: can_drop's ids, the files
// a drag becomes outside the window; the images are bytes and pixels.

@(private = "file")
ELEM_PNG := [?]u8{
	137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 4, 0, 0, 0, 4, 8, 6, 0, 0, 0, 169, 241, 158,
	126, 0, 0, 0, 26, 73, 68, 65, 84, 120, 218, 99, 248, 127, 130, 225, 63, 67, 195, 255, 255, 48, 154, 1, 153, 3, 166, 9,
	170, 0, 0, 141, 255, 42, 33, 250, 235, 186, 63, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130,
}
@(private = "file")
ELEM_PNG_PATH :: "gk-elem.png"

// a 4 x 4 checker of two colours, as RGBA pixels
@(private = "file")
checker :: proc() -> gk.Pixel_Image {
	px := make([]u8, 4 * 4 * 4, context.temp_allocator)
	for y in 0 ..< 4 {
		for x in 0 ..< 4 {
			c := (x + y) % 2 == 0 ? [4]u8{200, 40, 40, 255} : [4]u8{40, 40, 200, 255}
			copy(px[(y * 4 + x) * 4:], c[:])
		}
	}
	return {4, 4, px}
}

@(private = "file")
elem_dropped :: proc(a: ^App, cx: ^gk.Cx, d: gk.Drag_Value) {
	a.count += 1
}

@(private = "file")
elem_moved :: proc(a: ^App, cx: ^gk.Cx, e: gk.Drag_Move_Event) {
	a.flag = true
}

elem_drop :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	src := gk.div()->id("src")->w(gk.px(100))->h(gk.px(60))->bg(gk.theme(.Muted))->
		on_drag(gk.Drag_Value{7, "seven"}, {})->external_drag_payload({files = {{path = "/tmp/seven.txt"}}})->child(gk.label("seven"))
	takes7 := gk.div()->id("t7")->group("targets")->w(gk.px(100))->h(gk.px(60))->bg(gk.theme(.Secondary))->
		can_drop({ids = {7}})->on_drop(gk.on(elem_dropped))->on_drag_move(gk.on(elem_moved))->
		child(gk.div()->size(gk.px(20))->bg(RED)->group_drag_over("targets", gk.style_refinement_default()->bg(BLUE)))
	takes8 := gk.div()->id("t8")->w(gk.px(100))->h(gk.px(60))->bg(gk.theme(.Secondary))->
		can_drop({ids = {8}})->on_drop(gk.on(elem_dropped))
	return gk.v_flex(.Gap_2)->
		child(gk.h_flex(.Gap_4)->child(src)->child(takes7)->child(takes8))->
		child(gk.label(fmt.tprintf("drops %d moved %v", a.count, a.flag)))
}

elem_image :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	pic := mp.pic
	return gk.h_flex(.Gap_2)->image_cache(gk.Retain_All_Images{"elem-images"})->
		child(gk.img(gk.Image_Source_Render{pic})->size(gk.px(32)))->
		child(gk.img(gk.Image_Source_Custom{pic})->size(gk.px(32))->image_cache(mp.cache))->
		child(gk.img(gk.Image_Source_Image{{.Png, ELEM_PNG[:]}})->size(gk.px(32)))->
		child(gk.img(gk.Image_Source_Resource{gk.Resource_Path{ELEM_PNG_PATH}})->size(gk.px(32)))->
		child(gk.div()->image_cache(mp.cache)->child(gk.avatar()->src(gk.Image_Source_Render{pic})))
}

elem_scope :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	scrolled := gk.div()->id("scrolled")->h(gk.px(60))->w(gk.px(200))->overflow_y_scroll()->track_scroll(mp.scroll)->
		child(gk.div()->h(gk.px(40))->child(gk.label("above")))->
		child(gk.div()->id("anchored")->anchor_scroll(mp.anchor)->child(gk.label("anchored")))
	tree := gk.div()->id("chart")->a11y_synthetic_children({{"q1", .List_Item, "Q1: 12"}, {"q2", .List_Item, "Q2: 30"}})->
		child(gk.label("chart"))
	ordered := gk.div(.Relative)->h(gk.px(30))->with_dynamic_prepaint_order({1, 0})->
		child(gk.div(.Absolute)->size(gk.px(30))->bg(RED))->child(gk.div(.Absolute)->left(gk.px(10))->size(gk.px(30))->bg(BLUE))
	return gk.v_flex(.Gap_2)->
		child(gk.text_selection_scope(gk.div()->child(gk.label("selectable in its scope")), mp.scope))->
		child(scrolled)->child(tree)->child(ordered)
}

@(private = "file")
elem_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.Element {
	return gk.label(fmt.tprintf("row %d", at.row))
}

elem_vlist :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	sizes := make([][2]f32, 200, context.temp_allocator)
	for &s, i in sizes {
		s = {60, f32(20 + (i % 3) * 8)}
	}
	return gk.v_flex(.Gap_2)->
		child(gk.v_virtual_list("v", mp.rows, sizes)->w(gk.px(200))->h(gk.px(160)))->
		child(gk.h_virtual_list("h", mp.hrows, sizes[:8])->w(gk.px(400))->h(gk.px(24)))
}
