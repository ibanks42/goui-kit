// val-motion: the fallible results (Keyframes<f32>::try_new, Discrete<f32>::switch_at) as
// gk.Fallible[T].
package main

import (
	"fmt"
	"time"

	"gpuikit.dev/gk"
	gkmotion "gpuikit.dev/gk/motion"
)

func init() {
	modes["val-motion"] = &mode{click: once(motion)}
}

// Motion values over f32: keyframes (a fallible constructor), a discrete switch, a sequence.
func motion(a *App, cx *gk.Cx) {
	kf := gkmotion.KeyframesTryNew(cx, []gk.KeyframeF32{{Offset: 0, Value: 0, Easing: gk.EasingLinear}, {Offset: 1, Value: 10, Easing: gk.EasingLinear}})
	k, kok := kf.Value.Get()
	n, mid := -1, float32(-1)
	if kok {
		n, mid = k.Len(cx), k.Sample(cx, 0.5)
	}
	fmt.Printf("values keyframes ok=%v len=%d mid=%.1f\n", kf.OK, n, mid)
	bad := gkmotion.KeyframesTryNew(cx, nil)
	fmt.Printf("values keyframes empty ok=%v err=%v\n", bad.OK, len(bad.Err) > 0)
	d := gkmotion.NewDiscrete(cx, 1, 2)
	fmt.Printf("values discrete before=%.0f after=%.0f\n", d.Sample(cx, 0.2), d.Sample(cx, 0.9))
	sw := d.SwitchAt(cx, 0.1)
	fmt.Printf("values discrete switch ok=%v\n", sw.OK)
	step := gkmotion.NewSequenceStep(cx, 5, gkmotion.NewTransition(10*time.Millisecond))
	seq := gkmotion.NewSequence(cx, "seq", 1).WithSteps([]gkmotion.SequenceStep{step}).Build()
	fmt.Printf("values sequence from=%.0f steps=%d\n", seq.From(cx), len(seq.Steps(cx)))
}
