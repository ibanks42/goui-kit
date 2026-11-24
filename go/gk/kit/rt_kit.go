package kit

import (
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/base"
	"github.com/ibanks42/goui-kit/gk/internal/rt"
	"github.com/ibanks42/goui-kit/gk/motion"
)

// DropdownMenu wraps button so clicking it opens menu with default placement.
func DropdownMenu(button ButtonEl, menu PopupMenu) gk.Element {
	b, m := rt.ElemNode(button), rt.N(menu.Core)
	n := rt.NewNode(rt.NDropdownButton)
	o := rt.NodeArgs(n)
	o.U32(rt.TakeNode(b, false))
	o.U32(rt.TakeValue(m))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// DropdownMenuAnchored wraps button so clicking it opens menu aligned to anchor.
func DropdownMenuAnchored(button ButtonEl, anchor gk.Anchor, menu PopupMenu) gk.Element {
	b, m := rt.ElemNode(button), rt.N(menu.Core)
	n := rt.NewNode(rt.NDropdownAnchor)
	o := rt.NodeArgs(n)
	o.U32(rt.TakeNode(b, false))
	o.U32(rt.TakeValue(m))
	o.U16(uint16(anchor))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// Effect wraps el in an animated transition node controlled by t and keyed by id.
func Effect(el gk.AnimatableElement, t motion.EffectTransition, id string) gk.Element {
	src := rt.ElemNode(el)
	n := rt.NewNode(rt.NAnimated)
	o := rt.NodeArgs(n)
	o.U32(rt.TakeNode(src, false))
	o.U8(2)
	o.Str(id)
	o.U32(rt.TakeValue(rt.N(t.Core)))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// SliderNumber extracts the primary numeric value from a slider change or
// release event.
func SliderNumber(e base.SliderEvent) float32 {
	switch x := e.(type) {
	case base.SliderEventChange:
		return SliderValueNumber(x.Value)
	case base.SliderEventRelease:
		return SliderValueNumber(x.Value)
	}
	return 0
}

// SliderValueNumber returns the scalar value for a single slider or the lower
// endpoint (V0) for a range slider.
func SliderValueNumber(v base.SliderValue) float32 {
	switch x := v.(type) {
	case base.SliderValueSingle:
		return x.Value
	case base.SliderValueRange:
		return x.V0
	}
	return 0
}

// kbdNode encodes a Kbd shortcut badge node using lookup mode how:
// 0 = optional key context, 1 = focused handle context, 2 = global bindings.
func kbdNode(action gk.Action, how uint8, keyContext string, focus uint64) gk.Element {
	n := rt.NewNode(rt.NKbdForAction)
	o := rt.NodeArgs(n)
	rt.PutAction(o, action)
	o.U8(how)
	if keyContext != "" {
		o.U8(1)
		o.Str(keyContext)
	} else {
		o.U8(0)
	}
	o.U64(focus)
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// KbdForAction renders a keycap hint for the keystroke bound to action,
// optionally scoped with gk.WhenIn.
func KbdForAction(action gk.Action, opts ...gk.ShortcutOption) gk.Element {
	var so rt.ShortcutOpts
	for _, o := range opts {
		o(&so)
	}
	return kbdNode(action, 0, so.When, 0)
}

// KbdForActionIn renders a keycap hint for action resolved in focus's key context.
func KbdForActionIn(action gk.Action, focus gk.FocusHandle) gk.Element {
	return kbdNode(action, 1, "", rt.H(focus.HCore))
}

// KbdGlobalForAction renders a keycap hint for action's global key binding.
func KbdGlobalForAction(action gk.Action) gk.Element { return kbdNode(action, 2, "", 0) }

// NewAppMenuBar creates an in-window application menu bar state handle outside
// a view function.
func NewAppMenuBar(cx *gk.Cx) AppMenuBar {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("kit.NewAppMenuBar in a view function: make it in init or in a handler")
	}
	t.Flush()
	r := rt.Reader{B: rt.MustCall(*rt.NewReq(rt.CAppMenuBarNew))}
	return AppMenuBar{HCore: rt.MkH(r.U64())}
}

// AppMenuBarView embeds bar as a renderable element inside a view tree.
func AppMenuBarView(bar AppMenuBar) gk.Element {
	n := rt.NewNode(rt.NAppMenuBar)
	rt.NodeArgs(n).U64(rt.H(bar.HCore))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// ClockNow returns the monotonic kit clock reading used for animations and timing.
func ClockNow(cx *gk.Cx) time.Duration {
	rt.CheckCx(cx)
	r := rt.Reader{B: rt.MustCall(*rt.NewReq(rt.CClockNow))}
	return time.Duration(int64(r.U64()))
}
