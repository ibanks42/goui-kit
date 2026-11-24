package rt

import (
	"math"
	"time"
)

// Transformation defines a 2D affine transform (scale, translation, rotation in
// radians) and optional opacity alpha for SVG elements.
type Transformation struct {
	Scale     [2]float32
	Translate [2]float32
	Rotate    float32
	Alpha     Option[float32]
}

// NewTransformation returns the identity Transformation (scale 1, 1; zero
// translation and rotation; unset alpha).
func NewTransformation() Transformation { return Transformation{Scale: [2]float32{1, 1}} }

// Scaled returns a copy of t with Scale set to (x, y).
func (t Transformation) Scaled(x, y float32) Transformation { t.Scale = [2]float32{x, y}; return t }

// Translated returns a copy of t with Translate set to (x, y).
func (t Transformation) Translated(x, y float32) Transformation {
	t.Translate = [2]float32{x, y}
	return t
}

// Rotated returns a copy of t with Rotate set to radians.
func (t Transformation) Rotated(radians float32) Transformation { t.Rotate = radians; return t }

// WithAlpha returns a copy of t with Alpha set to a.
func (t Transformation) WithAlpha(a float32) Transformation { t.Alpha = Some(a); return t }

// PutTransformation encodes t onto o, defaulting an unset Alpha to 1.
func PutTransformation(o *Tape, t Transformation) {
	o.F32(t.Scale[0])
	o.F32(t.Scale[1])
	o.F32(t.Translate[0])
	o.F32(t.Translate[1])
	o.F32(t.Rotate)
	a, ok := t.Alpha.Get()
	if !ok {
		a = 1
	}
	o.F32(a)
}

// SvgEasing selects the interpolation curve for an animated SVG transform channel.
type SvgEasing uint8

// Easing curves accepted by TimedTransformation.
const (
	SvgEasingLinear SvgEasing = iota
	SvgEasingQuadratic
	SvgEasingEaseInOut
	SvgEasingEaseOutQuint
	SvgEasingHeartbeat
)

// TimedTransformation animates an SVG element between From and To over Period
// with per-channel easing curves, optional ping-pong alternation, and FPS cap.
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

// NewTimedTransform creates a TimedTransformation using easing for all four channels.
func NewTimedTransform(period time.Duration, from, to Transformation, easing SvgEasing) TimedTransformation {
	return TimedTransformation{Period: period, From: from, To: to, ScaleEasing: easing, TranslationEasing: easing,
		RotationEasing: easing, AlphaEasing: easing}
}

// NewTimedRotation creates a continuous linear 2*pi rotation animation starting at from.
func NewTimedRotation(period time.Duration, from Transformation) TimedTransformation {
	to := from
	to.Rotate += 2 * math.Pi
	return NewTimedTransform(period, from, to, SvgEasingLinear)
}

// PutTimedTransformation validates all four easing enums and encodes t onto o.
func PutTimedTransformation(o *Tape, t TimedTransformation) {
	for _, e := range [4]SvgEasing{t.ScaleEasing, t.TranslationEasing, t.RotationEasing, t.AlphaEasing} {
		if e > SvgEasingHeartbeat {
			Stop("gk.SvgEasing(%d) is not an easing", uint8(e))
		}
	}
	o.U64(uint64(int64(t.Period)))
	PutTransformation(o, t.From)
	PutTransformation(o, t.To)
	o.U8(uint8(t.ScaleEasing))
	o.U8(uint8(t.TranslationEasing))
	o.U8(uint8(t.RotationEasing))
	o.U8(uint8(t.AlphaEasing))
	o.Bool(t.Alternate)
	o.F32(t.MaxFPS)
}
