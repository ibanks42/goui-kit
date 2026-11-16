package gk

// Animations and motion values (docs/PROTOCOLS.md 13.30). The library side is core/src/motion.rs;
// the element types an animation may wrap are the generated unions Animatable_Element and
// Focus_Trap_Element (gen_values.odin).
//
// App code never runs while a frame is drawn, so an animation is described as data and the library
// evaluates it while it draws, frame after frame, without a turn:
//
//   // gpui's with_animation: a fade and a slide over 300 ms, eased
//   gk.with_animation(gk.div()->size(gk.px(40))->bg(gk.theme(.Primary)), "pulse",
//       {ms = 300, ease = {kind = .Ease_In_Out}, repeat = true},
//       {prop = .Opacity, from = f32(0.2), to = f32(1)}, {prop = .Left, from = f32(0), to = f32(80)})
//   // the kit's EffectTransition::apply
//   gk.effect(gk.div()->child(..), gk.effect_transition(200 * time.Millisecond)->fade(0, 1), "enter")
//   // the kit's transition, spring and keyframes: the value of a property this frame, moving towards
//   // the target the view described last (a.open toggled by a click)
//   gk.motion(gk.div()->h(gk.px(24)), {.Height, gk.To_Transition{"h", f32(a.open ? 120 : 24), gk.transition(250 * time.Millisecond)}})
//   gk.focus_trap(gk.div()->child(..), "dialog-trap", a.focus)
//
// Each returns the wrapped element as an Element, which any child() takes.

import "base:intrinsics"
import "base:runtime"

// Motion_Prop is the style property an animation moves: numbers (opacity 0 to 1, positions and
// sizes in pixels) or colours.
Motion_Prop :: enum u8 {
	Opacity      = 0,
	Top          = 1,
	Left         = 2,
	Right        = 3,
	Bottom       = 4,
	Width        = 5,
	Height       = 6,
	Bg           = 7,
	Text_Color   = 8,
	Border_Color = 9,
	Size         = 10,
}

// Motion_Val is a property's value: an f32 for a number, a Color for a colour property.
Motion_Val :: union {
	f32,
	Color,
}

// Ease_Kind is an easing curve: gpui's (linear, quadratic, ease in-out, ease-out quint, bounce,
// pulsating between) and the kit's base::animation curves (cubic ones, cubic Bézier).
Ease_Kind :: enum u8 {
	Linear            = 0,
	Quadratic         = 1,
	Ease_In_Out       = 2,
	Ease_Out_Quint    = 3,
	Ease_Out_Cubic    = 4,
	Ease_In_Cubic     = 5,
	Ease_In_Out_Cubic = 6,
	Cubic_Bezier      = 7, // p = {x1, y1, x2, y2}
	Bounce            = 8, // there and back with the curve `bounce`
	Pulsating_Between = 9, // p = {min, max}
}

// Ease is an easing curve and its parameters.
Ease :: struct {
	kind:   Ease_Kind,
	p:      [4]f32,
	bounce: Ease_Kind, // the curve a Bounce runs forth and back (not Bounce itself)
}

// Animation is gpui's Animation: a duration in milliseconds, once or repeating (synced: every
// repeating animation of the app in phase), an easing, and an optional cap on the frame rate.
Animation :: struct {
	ms:      int,
	repeat:  bool,
	synced:  bool,
	ease:    Ease,
	max_fps: f32,
}

// Track is one property an animation moves from `from` (delta 0) to `to` (delta 1). step: 0 for
// every animation of the chain, n for the n-th only (from 1; gk.with_animations).
Track :: struct {
	prop:     Motion_Prop,
	from, to: Motion_Val,
	step:     int,
}

@(private)
motion_is_color :: proc(p: Motion_Prop) -> bool {
	return p == .Bg || p == .Text_Color || p == .Border_Color
}

@(private)
put_motion_val :: proc(b: ^Tape, p: Motion_Prop, v: Motion_Val, loc: runtime.Source_Code_Location) {
	switch x in v {
	case f32:
		if motion_is_color(p) {
			stop(loc, "the property %v takes a gk.Color, not a number", p)
		}
		put_f32(b, x)
	case Color:
		if !motion_is_color(p) {
			stop(loc, "the property %v takes a number (f32), not a colour", p)
		}
		put_color(b, x)
	case:
		stop(loc, "a motion value for %v is missing", p)
	}
}

@(private)
put_ease :: proc(b: ^Tape, e: Ease, loc: runtime.Source_Code_Location) {
	if e.kind > .Pulsating_Between || e.bounce > .Pulsating_Between {
		stop(loc, "an easing curve this package does not know")
	}
	put_u8(b, u8(e.kind))
	#partial switch e.kind {
	case .Cubic_Bezier:
		for x in e.p {
			put_f32(b, x)
		}
	case .Pulsating_Between:
		put_f32(b, e.p[0])
		put_f32(b, e.p[1])
	case .Bounce:
		if e.bounce == .Bounce {
			stop(loc, "a bounce of a bounce: give Ease.bounce another curve")
		}
		put_ease(b, Ease{kind = e.bounce, p = e.p}, loc)
	}
}

@(private)
animated_node :: proc(el: Element, kind: u8, loc: runtime.Source_Code_Location) -> Element {
	e := new_node(N_ANIMATED, loc)
	put_u32(&build.ops, take(el))
	put_u8(&build.ops, kind)
	return e
}

// with_animation is gpui's AnimationExt::with_animation: the element, animated by `a`, its tracks
// set from the eased delta on every frame while it runs. id: the element id that keeps its clock.
with_animation :: proc(el: $E, id: string, a: Animation, tracks: ..Track, loc := #caller_location) -> Element
	where intrinsics.type_is_variant_of(Animatable_Element, E) {
	anims := [1]Animation{a}
	return with_animations(el, id, anims[:], ..tracks, loc = loc)
}

// with_animations is gpui's with_animations: a chain of animations one after the other; a track's
// step says which one moves it (0: all).
with_animations :: proc(el: $E, id: string, anims: []Animation, tracks: ..Track, loc := #caller_location) -> Element
	where intrinsics.type_is_variant_of(Animatable_Element, E) {
	if len(anims) == 0 {
		stop(loc, "with_animations: give at least one gk.Animation")
	}
	e := animated_node(el.el, 1, loc)
	b := &build.ops
	put_str(b, id)
	put_u32(b, u32(len(anims)))
	for a in anims {
		if a.ms <= 0 || a.ms > int(max(u32)) {
			stop(loc, "an animation of %d ms: give 1 ms or more", a.ms)
		}
		put_u32(b, u32(a.ms))
		put_u8(b, (a.repeat ? 1 : 0) | (a.synced ? 2 : 0))
		put_ease(b, a.ease, loc)
		put_f32(b, a.max_fps)
	}
	put_u32(b, u32(len(tracks)))
	for t in tracks {
		if t.step < 0 || t.step > len(anims) {
			stop(loc, "a track's step %d: 0 for every animation, or 1 to %d", t.step, len(anims))
		}
		put_u16(b, t.step == 0 ? 0xffff : u16(t.step - 1))
		put_u8(b, u8(t.prop))
		put_motion_val(b, t.prop, t.from, loc)
		put_motion_val(b, t.prop, t.to, loc)
	}
	return e
}

// effect is the kit's EffectTransition::apply: the element with the effect's fades, slides and
// sizes over its duration.
effect :: proc(el: $E, t: Effect_Transition, id: string, loc := #caller_location) -> Element
	where intrinsics.type_is_variant_of(Animatable_Element, E) {
	e := animated_node(el.el, 2, loc)
	put_str(&build.ops, id)
	put_u32(&build.ops, take_value(t.nd))
	return e
}

// The sources of a motion value: the kit's transition, spring, animate_keyframes, Presence::sample
// and TransitionState::sample, each sampled while the element is drawn (with its window's clock).
To_Transition :: struct {
	id:     string,
	target: Motion_Val,
	policy: Transition,
}
To_Spring :: struct {
	id:     string,
	target: f32,
	policy: Spring,
}
At_Keyframes :: struct {
	id:     string,
	frames: Keyframes,
	timing: Timing,
}
At_Presence :: struct {
	presence: Presence,
	from, to: Motion_Val,
}
To_State :: struct {
	state:  Transition_State,
	target: f32,
	policy: Transition,
}
Motion_Source :: union {
	To_Transition,
	To_Spring,
	At_Keyframes,
	At_Presence,
	To_State,
}

// Motion is one property and where its value comes from.
Motion :: struct {
	prop: Motion_Prop,
	src:  Motion_Source,
}

// motion is the element with properties set from motion values sampled as it is drawn: a value
// moving towards its target asks for the next frame itself, and only this view is drawn again.
motion :: proc(el: $E, values: ..Motion, loc := #caller_location) -> Element
	where intrinsics.type_is_variant_of(Animatable_Element, E) {
	e := animated_node(el.el, 3, loc)
	b := &build.ops
	put_u32(b, u32(len(values)))
	for m in values {
		put_u8(b, u8(m.prop))
		switch s in m.src {
		case To_Transition:
			put_u8(b, 1)
			put_str(b, s.id)
			put_motion_val(b, m.prop, s.target, loc)
			put_u32(b, take_value(s.policy.nd))
		case To_Spring:
			if motion_is_color(m.prop) {
				stop(loc, "a spring moves numbers: %v is a colour", m.prop)
			}
			put_u8(b, 2)
			put_str(b, s.id)
			put_f32(b, s.target)
			put_u32(b, take_value(s.policy.nd))
		case At_Keyframes:
			if motion_is_color(m.prop) {
				stop(loc, "keyframes of numbers move a number: %v is a colour", m.prop)
			}
			put_u8(b, 3)
			put_str(b, s.id)
			put_u64(b, s.frames.h)
			put_u32(b, take_value(s.timing.nd))
		case At_Presence:
			put_u8(b, 4)
			put_u32(b, take_value(s.presence.nd))
			put_motion_val(b, m.prop, s.from, loc)
			put_motion_val(b, m.prop, s.to, loc)
		case To_State:
			if motion_is_color(m.prop) {
				stop(loc, "a TransitionState of numbers moves a number: %v is a colour", m.prop)
			}
			put_u8(b, 5)
			put_u64(b, s.state.h)
			put_f32(b, s.target)
			put_u32(b, take_value(s.policy.nd))
		case:
			stop(loc, "a motion value for %v has no source", m.prop)
		}
	}
	return e
}

// focus_trap is the kit's FocusTrapElement::focus_trap: tab and shift-tab stay inside the element
// while focus is in it.
focus_trap :: proc(el: $E, id: string, focus: Focus_Handle, loc := #caller_location) -> Element
	where intrinsics.type_is_variant_of(Focus_Trap_Element, E) {
	e := animated_node(el.el, 4, loc)
	put_str(&build.ops, id)
	put_u64(&build.ops, focus.h)
	return e
}

// text_selection_scope is the kit's ElementExt::text_selection_scope (PROTOCOLS.md 13.31): text
// selected under the element stays in the scope (gk.text_selection_scope_id makes one; keep it).
text_selection_scope :: proc(el: $E, scope: Text_Selection_Scope, loc := #caller_location) -> Element
	where intrinsics.type_is_variant_of(Focus_Trap_Element, E) {
	if scope.h == 0 {
		stop(loc, "gk.text_selection_scope: a scope made by gk.text_selection_scope_id")
	}
	e := animated_node(el.el, 5, loc)
	put_u64(&build.ops, scope.h)
	return e
}

// Transformation is gpui's Transformation of an svg (Svg::with_transformation): a scale (1 is the
// svg's size), a translation in pixels and a rotation in radians, about the svg's centre. Make one
// with gk.transform; a zero scale draws nothing.
Transformation :: struct {
	scale:     [2]f32,
	translate: [2]f32,
	rotate:    f32,
}

transform :: proc(scale := [2]f32{1, 1}, translate := [2]f32{0, 0}, rotate: f32 = 0) -> Transformation {
	return {scale, translate, rotate}
}

@(private)
put_transformation :: proc(b: ^Tape, t: Transformation) {
	put_f32(b, t.scale[0])
	put_f32(b, t.scale[1])
	put_f32(b, t.translate[0])
	put_f32(b, t.translate[1])
	put_f32(b, t.rotate)
}
