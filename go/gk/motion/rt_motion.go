package motion

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
)

// MotionProp is the style property an animation moves: numbers (opacity 0 to 1, positions and
// sizes in pixels) or colours.
type Prop uint8

const (
	Opacity     Prop = 0
	Top         Prop = 1
	Left        Prop = 2
	Right       Prop = 3
	Bottom      Prop = 4
	Width       Prop = 5
	Height      Prop = 6
	Bg          Prop = 7
	TextColor   Prop = 8
	BorderColor Prop = 9
	Size        Prop = 10
)

func (p Prop) isColor() bool {
	return p == Bg || p == TextColor || p == BorderColor
}

// MotionVal is a property's value: a float32 for a number, a gk.Color for a colour property. A
// value of another type stops with a message.
type Val any

func putMotionVal(o *rt.Tape, p Prop, v Val) {
	switch x := v.(type) {
	case float32:
		if p.isColor() {
			rt.Stop("the motion property %d takes a gk.Color, not a number", p)
		}
		o.F32(x)
	case float64:
		if p.isColor() {
			rt.Stop("the motion property %d takes a gk.Color, not a number", p)
		}
		o.F32(float32(x))
	case gk.Color:
		if !p.isColor() {
			rt.Stop("the motion property %d takes a number (float32), not a colour", p)
		}
		rt.PutColor(o, x)
	case nil:
		rt.Stop("a motion value for property %d is missing", p)
	default:
		rt.Stop("a motion value is a float32 or a gk.Color, not a %T", v)
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

func putEase(o *rt.Tape, e Ease) {
	if e.Kind > EasePulsatingBetween || e.Bounce > EasePulsatingBetween {
		rt.Stop("an easing curve this package does not know (%d)", e.Kind)
	}
	o.U8(uint8(e.Kind))
	switch e.Kind {
	case EaseCubicBezier:
		for _, x := range e.P {
			o.F32(x)
		}
	case EasePulsatingBetween:
		o.F32(e.P[0])
		o.F32(e.P[1])
	case EaseBounce:
		if e.Bounce == EaseBounce {
			rt.Stop("a bounce of a bounce: give Ease.Bounce another curve")
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
// animation of the chain, n for the n-th only (from 1; motion.WithAnimations).
type Track struct {
	Prop     Prop
	From, To Val
	Step     int
}

func animatedNode(el gk.Element, kind uint8) (*rt.Node, *rt.Tape) {
	src := rt.ElemNode(el)
	n := rt.NewNode(rt.NAnimated)
	o := rt.NodeArgs(n)
	o.U32(rt.TakeNode(src, false))
	o.U8(kind)
	return n, o
}

// WithAnimation is gpui's AnimationExt::with_animation: the element, animated by a, its tracks set
// from the eased delta on every frame while it runs. id: the element id that keeps its clock.
func WithAnimation(el gk.AnimatableElement, id string, a Animation, tracks ...Track) gk.Element {
	return WithAnimations(el, id, []Animation{a}, tracks...)
}

// WithAnimations is gpui's with_animations: a chain of animations one after the other; a track's
// Step says which one moves it (0: all).
func WithAnimations(el gk.AnimatableElement, id string, anims []Animation, tracks ...Track) gk.Element {
	if len(anims) == 0 {
		rt.Stop("motion.WithAnimations: give at least one motion.Animation")
	}
	n, o := animatedNode(el, 1)
	o.Str(id)
	o.U32(uint32(len(anims)))
	for _, a := range anims {
		if a.Ms <= 0 || a.Ms > 0xffff_ffff {
			rt.Stop("an animation of %d ms: give 1 ms or more", a.Ms)
		}
		o.U32(uint32(a.Ms))
		var flags uint8
		if a.Repeat {
			flags |= 1
		}
		if a.Synced {
			flags |= 2
		}
		o.U8(flags)
		putEase(o, a.Ease)
		o.F32(a.MaxFPS)
	}
	o.U32(uint32(len(tracks)))
	for _, t := range tracks {
		if t.Step < 0 || t.Step > len(anims) {
			rt.Stop("a track's step %d: 0 for every animation, or 1 to %d", t.Step, len(anims))
		}
		if t.Step == 0 {
			o.U16(0xffff)
		} else {
			o.U16(uint16(t.Step - 1))
		}
		o.U8(uint8(t.Prop))
		putMotionVal(o, t.Prop, t.From)
		putMotionVal(o, t.Prop, t.To)
	}
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// MotionSource is where a motion value comes from: gk.ToTransition, gk.ToSpring, gk.AtKeyframes,
// gk.AtPresence or gk.ToState, each sampled while the element is drawn (with its window's clock).
type Source interface{ isMotionSource() }

// ToTransition is the kit's transition: the value moves to Target under Policy.
type ToTransition struct {
	ID     string
	Target Val
	Policy gk.Transition
}

// ToSpring is the kit's spring: a number moves to Target under Policy.
type ToSpring struct {
	ID     string
	Target float32
	Policy gk.Spring
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
	From, To Val
}

// ToState is the kit's TransitionState::sample: a number moves to Target under Policy, its state
// kept in State.
type ToState struct {
	State  TransitionState
	Target float32
	Policy gk.Transition
}

func (ToTransition) isMotionSource() {}

func (ToSpring) isMotionSource() {}

func (AtKeyframes) isMotionSource() {}

func (AtPresence) isMotionSource() {}

func (ToState) isMotionSource() {}

// MotionValue is one property and where its value comes from.
type Value struct {
	Prop Prop
	Src  Source
}

// Motion is the element with properties set from motion values sampled as it is drawn: a value
// moving towards its target asks for the next frame itself, and only this view is drawn again.
func Motion(el gk.AnimatableElement, values ...Value) gk.Element {
	n, o := animatedNode(el, 3)
	o.U32(uint32(len(values)))
	for _, m := range values {
		o.U8(uint8(m.Prop))
		switch s := m.Src.(type) {
		case ToTransition:
			o.U8(1)
			o.Str(s.ID)
			putMotionVal(o, m.Prop, s.Target)
			o.U32(rt.TakeValue(rt.N(s.Policy.Core)))
		case ToSpring:
			if m.Prop.isColor() {
				rt.Stop("a spring moves numbers: property %d is a colour", m.Prop)
			}
			o.U8(2)
			o.Str(s.ID)
			o.F32(s.Target)
			o.U32(rt.TakeValue(rt.N(s.Policy.Core)))
		case AtKeyframes:
			if m.Prop.isColor() {
				rt.Stop("keyframes of numbers move a number: property %d is a colour", m.Prop)
			}
			o.U8(3)
			o.Str(s.ID)
			o.U64(rt.H(s.Frames.HCore))
			o.U32(rt.TakeValue(rt.N(s.Timing.Core)))
		case AtPresence:
			o.U8(4)
			o.U32(rt.TakeValue(rt.N(s.Presence.Core)))
			putMotionVal(o, m.Prop, s.From)
			putMotionVal(o, m.Prop, s.To)
		case ToState:
			if m.Prop.isColor() {
				rt.Stop("a TransitionState of numbers moves a number: property %d is a colour", m.Prop)
			}
			o.U8(5)
			o.U64(rt.H(s.State.HCore))
			o.F32(s.Target)
			o.U32(rt.TakeValue(rt.N(s.Policy.Core)))
		default:
			rt.Stop("a motion value for property %d has no source", m.Prop)
		}
	}
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// FocusTrap is the kit's FocusTrapElement::focus_trap: tab and shift-tab stay inside the element
// while focus is in it.
func FocusTrap(el gk.FocusTrapElement, id string, focus gk.FocusHandle) gk.Element {
	n, o := animatedNode(el, 4)
	o.Str(id)
	o.U64(rt.H(focus.HCore))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// TextSelectionScope is a scope of text selection (motion.NewTextSelectionScopeID makes one; keep it).
type TextSelectionScope struct{ rt.HCore }

func (s TextSelectionScope) handle() uint64 { return rt.H(s.HCore) }

// NewTextSelectionScopeID makes a text selection scope (TextSelectionScopeId::new). Once, in init.
func NewTextSelectionScopeID(cx *gk.Cx) TextSelectionScope {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("a view function may not make a text selection scope: make it once, in init")
	}
	t.Flush()
	r := rt.Reader{B: rt.MustCall(*rt.NewReq(rt.CTextScopeNew))}
	return TextSelectionScope{HCore: rt.MkH(r.U64())}
}

// TextSelectionScopeOf is the kit's ElementExt::text_selection_scope (PROTOCOLS.md 13.31): text
// selected under the element stays in the scope.
func TextSelectionScopeOf(el gk.FocusTrapElement, scope TextSelectionScope) gk.Element {
	if rt.H(scope.HCore) == 0 {
		rt.Stop("motion.TextSelectionScopeOf: a scope made by motion.NewTextSelectionScopeID")
	}
	n, o := animatedNode(el, 5)
	o.U64(rt.H(scope.HCore))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}
