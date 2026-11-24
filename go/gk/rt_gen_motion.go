package gk

// gpui's Transformation and TimedTransformation of an svg
// (docs/PROTOCOLS.md 13.30): Svg's WithTransformation and WithTimedTransformation, Icon's Transform.
//
//	gk.Svg().Path("icons/spinner.svg").WithTimedTransformation(gk.NewTimedRotation(time.Second, gk.NewTransformation()))
//	gk.NewIcon(gk.IconStarFill).Transform(gk.NewTransformation().Rotated(0.5).WithAlpha(0.6))

import (
	"math"
	"time"
)

// Transformation is a scale (1 is the svg's size), a translation in pixels and a rotation in
// radians, about the svg's centre, and an alpha that multiplies the svg's own (none is 1, opaque;
// unlike an element's opacity it applies to the svg alone). Make one with gk.NewTransformation: the
// zero value has a zero scale, which draws nothing.
type Transformation struct {
	Scale     [2]float32
	Translate [2]float32
	Rotate    float32
	Alpha     Option[float32]
}

// NewTransformation is the identity: scale 1, no translation, no rotation, opaque.
func NewTransformation() Transformation { return Transformation{Scale: [2]float32{1, 1}} }

// Scaled sets the scale.
func (t Transformation) Scaled(x, y float32) Transformation { t.Scale = [2]float32{x, y}; return t }

// Translated sets the translation in pixels.
func (t Transformation) Translated(x, y float32) Transformation {
	t.Translate = [2]float32{x, y}
	return t
}

// Rotated sets the rotation in radians.
func (t Transformation) Rotated(radians float32) Transformation { t.Rotate = radians; return t }

// WithAlpha sets the alpha.
func (t Transformation) WithAlpha(a float32) Transformation { t.Alpha = Some(a); return t }

func putTransformation(o *tape, t Transformation) {
	o.f32(t.Scale[0])
	o.f32(t.Scale[1])
	o.f32(t.Translate[0])
	o.f32(t.Translate[1])
	o.f32(t.Rotate)
	a, ok := t.Alpha.Get()
	if !ok {
		a = 1
	}
	o.f32(a)
}

// SvgEasing is how a value of a TimedTransformation changes over one pass (gpui's Easing of an svg,
// not gk.Easing of an animation). Heartbeat is two quick beats then a rest: it reaches To at the
// first beat and is back at From by the end of the period.
type SvgEasing uint8

const (
	SvgEasingLinear SvgEasing = iota
	SvgEasingQuadratic
	SvgEasingEaseInOut
	SvgEasingEaseOutQuint
	SvgEasingHeartbeat
)

// TimedTransformation is gpui's TimedTransformation: the svg moves from From to To over every
// Period, forever, drawn by the library without a turn of the app (it replaces
// WithTransformation; layout and hit testing keep the svg's place). Each value has its own easing;
// Alternate goes back from To to From within each period instead of jumping; MaxFPS (0: no limit)
// caps the frames a second asked for while nothing else needs one.
type TimedTransformation struct {
	Period            time.Duration
	From, To          Transformation
	ScaleEasing       SvgEasing
	TranslationEasing SvgEasing
	RotationEasing    SvgEasing
	AlphaEasing       SvgEasing
	Alternate         bool
	MaxFPS            float32
}

// NewTimedTransform moves from `from` to `to` every period, every value with one easing.
func NewTimedTransform(period time.Duration, from, to Transformation, easing SvgEasing) TimedTransformation {
	return TimedTransformation{Period: period, From: from, To: to, ScaleEasing: easing, TranslationEasing: easing,
		RotationEasing: easing, AlphaEasing: easing}
}

// NewTimedRotation turns `from` clockwise, one full turn every period (TimedTransformation::rotation).
func NewTimedRotation(period time.Duration, from Transformation) TimedTransformation {
	to := from
	to.Rotate += 2 * math.Pi
	return NewTimedTransform(period, from, to, SvgEasingLinear)
}

func putTimedTransformation(o *tape, t TimedTransformation) {
	for _, e := range [4]SvgEasing{t.ScaleEasing, t.TranslationEasing, t.RotationEasing, t.AlphaEasing} {
		if e > SvgEasingHeartbeat {
			stop("gk.SvgEasing(%d) is not an easing", uint8(e))
		}
	}
	o.u64(uint64(int64(t.Period)))
	putTransformation(o, t.From)
	putTransformation(o, t.To)
	o.u8(uint8(t.ScaleEasing))
	o.u8(uint8(t.TranslationEasing))
	o.u8(uint8(t.RotationEasing))
	o.u8(uint8(t.AlphaEasing))
	o.bool(t.Alternate)
	o.f32(t.MaxFPS)
}
