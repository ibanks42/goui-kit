package gk

// Animations and motion values (docs/PROTOCOLS.md 13.30). The
// library side is core/src/motion.rs.
//
// App code never runs while a frame is drawn, so an animation is described as data and the library
// evaluates it while it draws, frame after frame, without a turn:
//
//	// gpui's with_animation: a fade and a slide over 300 ms, eased
//	gk.WithAnimation(gk.Div().Size(gk.Px(40)).Bg(theme.Primary), "pulse",
//		gk.Animation{Ms: 300, Ease: gk.Ease{Kind: gk.EaseInOut}, Repeat: true},
//		gk.Track{Prop: gk.MotionOpacity, From: float32(0.2), To: float32(1)})
//	// the kit's transition, spring and keyframes: the value of a property this frame, moving
//	// towards the target the view described last
//	gk.Motion(gk.Div().H(gk.Px(24)), gk.MotionValue{Prop: gk.MotionHeight,
//		Src: gk.ToTransition{ID: "h", Target: float32(120), Policy: gk.NewTransition(250 * time.Millisecond)}})
//	gk.FocusTrap(gk.Div().Child(..), "dialog-trap", a.focus)
//
// Each returns the wrapped element as an Element, which any Child takes.

// MotionProp is the style property an animation moves: numbers (opacity 0 to 1, positions and
// sizes in pixels) or colours.
type MotionProp uint8

const (
	MotionOpacity     MotionProp = 0
	MotionTop         MotionProp = 1
	MotionLeft        MotionProp = 2
	MotionRight       MotionProp = 3
	MotionBottom      MotionProp = 4
	MotionWidth       MotionProp = 5
	MotionHeight      MotionProp = 6
	MotionBg          MotionProp = 7
	MotionTextColor   MotionProp = 8
	MotionBorderColor MotionProp = 9
	MotionSize        MotionProp = 10
)

func (p MotionProp) isColor() bool {
	return p == MotionBg || p == MotionTextColor || p == MotionBorderColor
}

// MotionVal is a property's value: a float32 for a number, a gk.Color for a colour property. A
// value of another type stops with a message.
type MotionVal any

func putMotionVal(o *tape, p MotionProp, v MotionVal) {
	switch x := v.(type) {
	case float32:
		if p.isColor() {
			stop("the motion property %d takes a gk.Color, not a number", p)
		}
		o.f32(x)
	case float64:
		if p.isColor() {
			stop("the motion property %d takes a gk.Color, not a number", p)
		}
		o.f32(float32(x))
	case Color:
		if !p.isColor() {
			stop("the motion property %d takes a number (float32), not a colour", p)
		}
		putColor(o, x)
	case nil:
		stop("a motion value for property %d is missing", p)
	default:
		stop("a motion value is a float32 or a gk.Color, not a %T", v)
	}
}

// EaseKind is an easing curve: gpui's (linear, quadratic, ease in-out, ease-out quint, bounce,
// pulsating between) and the kit's base::animation curves (cubic ones, cubic Bezier).
type EaseKind uint8

const (
	EaseLinear           EaseKind = 0
	EaseQuadratic        EaseKind = 1
	EaseInOut            EaseKind = 2
	EaseOutQuint         EaseKind = 3
	EaseOutCubic         EaseKind = 4
	EaseInCubic          EaseKind = 5
	EaseInOutCubic       EaseKind = 6
	EaseCubicBezier      EaseKind = 7 // P = {x1, y1, x2, y2} (base::animation::cubic_bezier)
	EaseBounce           EaseKind = 8 // there and back with the curve Bounce
	EasePulsatingBetween EaseKind = 9 // P = {min, max}
)

// Ease is an easing curve and its parameters.
type Ease struct {
	Kind   EaseKind
	P      [4]float32
	Bounce EaseKind // the curve an EaseBounce runs forth and back (not EaseBounce itself)
}

func putEase(o *tape, e Ease) {
	if e.Kind > EasePulsatingBetween || e.Bounce > EasePulsatingBetween {
		stop("an easing curve this package does not know (%d)", e.Kind)
	}
	o.u8(uint8(e.Kind))
	switch e.Kind {
	case EaseCubicBezier:
		for _, x := range e.P {
			o.f32(x)
		}
	case EasePulsatingBetween:
		o.f32(e.P[0])
		o.f32(e.P[1])
	case EaseBounce:
		if e.Bounce == EaseBounce {
			stop("a bounce of a bounce: give Ease.Bounce another curve")
		}
		putEase(o, Ease{Kind: e.Bounce, P: e.P})
	}
}

// Animation is gpui's Animation: a duration in milliseconds, once or repeating (Synced: every
// repeating animation of the app in phase), an easing, and an optional cap on the frame rate.
type Animation struct {
	Ms     int
	Repeat bool
	Synced bool
	Ease   Ease
	MaxFPS float32
}

// Track is one property an animation moves from From (delta 0) to To (delta 1). Step: 0 for every
// animation of the chain, n for the n-th only (from 1; gk.WithAnimations).
type Track struct {
	Prop     MotionProp
	From, To MotionVal
	Step     int
}

func animatedNode(el Element, kind uint8) (*node, *tape) {
	src := elemNode(el)
	n := newNode(nAnimated)
	o := nodeArgs(n)
	o.u32(takeNode(src, false))
	o.u8(kind)
	return n, o
}

// WithAnimation is gpui's AnimationExt::with_animation: the element, animated by a, its tracks set
// from the eased delta on every frame while it runs. id: the element id that keeps its clock.
func WithAnimation(el AnimatableElement, id string, a Animation, tracks ...Track) Element {
	return WithAnimations(el, id, []Animation{a}, tracks...)
}

// WithAnimations is gpui's with_animations: a chain of animations one after the other; a track's
// Step says which one moves it (0: all).
func WithAnimations(el AnimatableElement, id string, anims []Animation, tracks ...Track) Element {
	if len(anims) == 0 {
		stop("gk.WithAnimations: give at least one gk.Animation")
	}
	n, o := animatedNode(el, 1)
	o.str(id)
	o.u32(uint32(len(anims)))
	for _, a := range anims {
		if a.Ms <= 0 || a.Ms > 0xffff_ffff {
			stop("an animation of %d ms: give 1 ms or more", a.Ms)
		}
		o.u32(uint32(a.Ms))
		var flags uint8
		if a.Repeat {
			flags |= 1
		}
		if a.Synced {
			flags |= 2
		}
		o.u8(flags)
		putEase(o, a.Ease)
		o.f32(a.MaxFPS)
	}
	o.u32(uint32(len(tracks)))
	for _, t := range tracks {
		if t.Step < 0 || t.Step > len(anims) {
			stop("a track's step %d: 0 for every animation, or 1 to %d", t.Step, len(anims))
		}
		if t.Step == 0 {
			o.u16(0xffff)
		} else {
			o.u16(uint16(t.Step - 1))
		}
		o.u8(uint8(t.Prop))
		putMotionVal(o, t.Prop, t.From)
		putMotionVal(o, t.Prop, t.To)
	}
	return valueEl{n}
}

// MotionSource is where a motion value comes from: gk.ToTransition, gk.ToSpring, gk.AtKeyframes,
// gk.AtPresence or gk.ToState, each sampled while the element is drawn (with its window's clock).
type MotionSource interface{ isMotionSource() }

// ToTransition is the kit's transition: the value moves to Target under Policy.
type ToTransition struct {
	ID     string
	Target MotionVal
	Policy Transition
}

// ToSpring is the kit's spring: a number moves to Target under Policy.
type ToSpring struct {
	ID     string
	Target float32
	Policy Spring
}

// AtKeyframes is the kit's animate_keyframes: a number along Frames with Timing.
type AtKeyframes struct {
	ID     string
	Frames Keyframes
	Timing Timing
}

// AtPresence is the kit's Presence::sample: between From and To with the presence's progress.
type AtPresence struct {
	Presence Presence
	From, To MotionVal
}

// ToState is the kit's TransitionState::sample: a number moves to Target under Policy, its state
// kept in State.
type ToState struct {
	State  TransitionState
	Target float32
	Policy Transition
}

func (ToTransition) isMotionSource() {}
func (ToSpring) isMotionSource()     {}
func (AtKeyframes) isMotionSource()  {}
func (AtPresence) isMotionSource()   {}
func (ToState) isMotionSource()      {}

// MotionValue is one property and where its value comes from.
type MotionValue struct {
	Prop MotionProp
	Src  MotionSource
}

// Motion is the element with properties set from motion values sampled as it is drawn: a value
// moving towards its target asks for the next frame itself, and only this view is drawn again.
func Motion(el AnimatableElement, values ...MotionValue) Element {
	n, o := animatedNode(el, 3)
	o.u32(uint32(len(values)))
	for _, m := range values {
		o.u8(uint8(m.Prop))
		switch s := m.Src.(type) {
		case ToTransition:
			o.u8(1)
			o.str(s.ID)
			putMotionVal(o, m.Prop, s.Target)
			o.u32(takeValue(s.Policy.n))
		case ToSpring:
			if m.Prop.isColor() {
				stop("a spring moves numbers: property %d is a colour", m.Prop)
			}
			o.u8(2)
			o.str(s.ID)
			o.f32(s.Target)
			o.u32(takeValue(s.Policy.n))
		case AtKeyframes:
			if m.Prop.isColor() {
				stop("keyframes of numbers move a number: property %d is a colour", m.Prop)
			}
			o.u8(3)
			o.str(s.ID)
			o.u64(s.Frames.h)
			o.u32(takeValue(s.Timing.n))
		case AtPresence:
			o.u8(4)
			o.u32(takeValue(s.Presence.n))
			putMotionVal(o, m.Prop, s.From)
			putMotionVal(o, m.Prop, s.To)
		case ToState:
			if m.Prop.isColor() {
				stop("a TransitionState of numbers moves a number: property %d is a colour", m.Prop)
			}
			o.u8(5)
			o.u64(s.State.h)
			o.f32(s.Target)
			o.u32(takeValue(s.Policy.n))
		default:
			stop("a motion value for property %d has no source", m.Prop)
		}
	}
	return valueEl{n}
}

// FocusTrap is the kit's FocusTrapElement::focus_trap: tab and shift-tab stay inside the element
// while focus is in it.
func FocusTrap(el FocusTrapElement, id string, focus FocusHandle) Element {
	n, o := animatedNode(el, 4)
	o.str(id)
	o.u64(focus.h)
	return valueEl{n}
}

// TextSelectionScope is a scope of text selection (gk.NewTextSelectionScopeID makes one; keep it).
type TextSelectionScope struct{ h uint64 }

func (s TextSelectionScope) handle() uint64 { return s.h }

// NewTextSelectionScopeID makes a text selection scope (TextSelectionScopeId::new). Once, in init.
func NewTextSelectionScopeID(cx *Cx) TextSelectionScope {
	t := checkCx(cx)
	if vb.active {
		stop("a view function may not make a text selection scope: make it once, in init")
	}
	t.flush()
	r := reader{b: mustCall(*newReq(cTextScopeNew))}
	return TextSelectionScope{r.u64()}
}

// TextSelectionScopeOf is the kit's ElementExt::text_selection_scope (PROTOCOLS.md 13.31): text
// selected under the element stays in the scope.
func TextSelectionScopeOf(el FocusTrapElement, scope TextSelectionScope) Element {
	if scope.h == 0 {
		stop("gk.TextSelectionScopeOf: a scope made by gk.NewTextSelectionScopeID")
	}
	n, o := animatedNode(el, 5)
	o.u64(scope.h)
	return valueEl{n}
}
