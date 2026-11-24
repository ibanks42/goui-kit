package kit

import (
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/base"
	"gpuikit.dev/gk/internal/rt"
	"gpuikit.dev/gk/motion"
)

// DropdownMenu is the kit's Button with a dropdown menu (DropdownMenu::dropdown_menu): the menu
// is described as a value in the view function and built from it each time it opens.
//
//	gk.DropdownMenu(gk.Button("more").Label("More"), kit.NewPopupMenu().Menu("Copy", a.copy))
func DropdownMenu(button ButtonEl, menu PopupMenu) gk.Element {
	b, m := rt.ElemNode(button), rt.N(menu.Core)
	n := rt.NewNode(rt.NDropdownButton)
	o := rt.NodeArgs(n)
	o.U32(rt.TakeNode(b, false))
	o.U32(rt.TakeValue(m))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// DropdownMenuAnchored is gk.DropdownMenu with the corner the menu opens from
// (DropdownMenu::dropdown_menu_with_anchor).
func DropdownMenuAnchored(button ButtonEl, anchor gk.Anchor, menu PopupMenu) gk.Element {
	b, m := rt.ElemNode(button), rt.N(menu.Core)
	n := rt.NewNode(rt.NDropdownAnchor)
	o := rt.NodeArgs(n)
	o.U32(rt.TakeNode(b, false))
	o.U32(rt.TakeValue(m))
	o.U16(uint16(anchor))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// Effect is the kit's EffectTransition::apply: the element with the effect's fades, slides and
// sizes over its duration, restarted whenever id changes.
//
//	gk.Effect(page, gk.NewEffectTransition(200*time.Millisecond).Fade(0, 1).SlideY(12, 0), "open-3")
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

// SliderNumber is the number a slider's event carries: a single slider's value, or a range
// slider's start (upstream's SliderValue::start).
//
//	a.font.OnSliderEvent(cx, func(cx *gk.Cx, e gk.SliderEvent) { a.size = gk.SliderNumber(e) })
func SliderNumber(e base.SliderEvent) float32 {
	switch x := e.(type) {
	case base.SliderEventChange:
		return SliderValueNumber(x.Value)
	case base.SliderEventRelease:
		return SliderValueNumber(x.Value)
	}
	return 0
}

// SliderValueNumber is a slider value's number (a range's start).
func SliderValueNumber(v base.SliderValue) float32 {
	switch x := v.(type) {
	case base.SliderValueSingle:
		return x.Value
	case base.SliderValueRange:
		return x.V0
	}
	return 0
}

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

// KbdForAction draws the keystrokes bound to action as the kit's Kbd, or nothing when no binding
// has it (Kbd::binding_for_action). gk.WhenIn("Editor") looks in that key context.
//
//	gk.HFlex(s.Gap2).Child(gk.Label("Save")).Child(gk.KbdForAction(a.save))
func KbdForAction(action gk.Action, opts ...gk.ShortcutOption) gk.Element {
	var so rt.ShortcutOpts
	for _, o := range opts {
		o(&so)
	}
	return kbdNode(action, 0, so.When, 0)
}

// KbdForActionIn draws the binding of action that reaches focus (Kbd::binding_for_action_in).
func KbdForActionIn(action gk.Action, focus gk.FocusHandle) gk.Element {
	return kbdNode(action, 1, "", rt.H(focus.HCore))
}

// KbdGlobalForAction draws a binding of action with no key context (Kbd::global_binding_for_action).
func KbdGlobalForAction(action gk.Action) gk.Element { return kbdNode(action, 2, "", 0) }

// NewAppMenuBar makes the kit's AppMenuBar (AppMenuBar::new), which draws the menus gk.SetMenus
// gave; gk.AppMenuBarView draws it. In init or a handler.
func NewAppMenuBar(cx *gk.Cx) AppMenuBar {
	t := rt.CheckCx(cx)
	if rt.Vb.Active {
		rt.Stop("kit.NewAppMenuBar in a view function: make it in init or in a handler")
	}
	t.Flush()
	r := rt.Reader{B: rt.MustCall(*rt.NewReq(rt.CAppMenuBarNew))}
	return AppMenuBar{HCore: rt.MkH(r.U64())}
}

// AppMenuBarView draws an app menu bar where the view puts it.
func AppMenuBarView(bar AppMenuBar) gk.Element {
	n := rt.NewNode(rt.NAppMenuBar)
	rt.NodeArgs(n).U64(rt.H(bar.HCore))
	return rt.ValueEl{ElCore: rt.MkElCore(n)}
}

// ClockNow reads the library's clock: std::time::Instant travels as the time since it began. A
// toast stack's bookkeeping takes the app's reading of it:
//
//	a.toasts.Push(cx, id, value, gk.ToastOptions{..}, gk.ClockNow(cx))
func ClockNow(cx *gk.Cx) time.Duration {
	rt.CheckCx(cx)
	r := rt.Reader{B: rt.MustCall(*rt.NewReq(rt.CClockNow))}
	return time.Duration(int64(r.U64()))
}
