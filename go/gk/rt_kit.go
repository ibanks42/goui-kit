package gk

import "time"

// Small hand-written pieces of the kit's API: a button with a dropdown menu, the kit's effect transition
// applied to an element, and the number a slider reports.

// DropdownMenu is the kit's Button with a dropdown menu (DropdownMenu::dropdown_menu): the menu
// is described as a value in the view function and built from it each time it opens.
//
//	gk.DropdownMenu(gk.Button("more").Label("More"), gk.NewPopupMenu().Menu("Copy", a.copy))
func DropdownMenu(button ButtonEl, menu PopupMenu) Element {
	b, m := elemNode(button), menu.n
	n := newNode(nDropdownButton)
	o := nodeArgs(n)
	o.u32(takeNode(b, false))
	o.u32(takeValue(m))
	return valueEl{n}
}

// DropdownMenuAnchored is gk.DropdownMenu with the corner the menu opens from
// (DropdownMenu::dropdown_menu_with_anchor).
func DropdownMenuAnchored(button ButtonEl, anchor Anchor, menu PopupMenu) Element {
	b, m := elemNode(button), menu.n
	n := newNode(nDropdownAnchor)
	o := nodeArgs(n)
	o.u32(takeNode(b, false))
	o.u32(takeValue(m))
	o.u16(uint16(anchor))
	return valueEl{n}
}

// Effect is the kit's EffectTransition::apply: the element with the effect's fades, slides and
// sizes over its duration, restarted whenever id changes.
//
//	gk.Effect(page, gk.NewEffectTransition(200*time.Millisecond).Fade(0, 1).SlideY(12, 0), "open-3")
func Effect(el AnimatableElement, t EffectTransition, id string) Element {
	src := elemNode(el)
	n := newNode(nAnimated)
	o := nodeArgs(n)
	o.u32(takeNode(src, false))
	o.u8(2)
	o.str(id)
	o.u32(takeValue(t.n))
	return valueEl{n}
}

// SliderNumber is the number a slider's event carries: a single slider's value, or a range
// slider's start (upstream's SliderValue::start).
//
//	a.font.OnSliderEvent(cx, func(cx *gk.Cx, e gk.SliderEvent) { a.size = gk.SliderNumber(e) })
func SliderNumber(e SliderEvent) float32 {
	switch x := e.(type) {
	case SliderEventChange:
		return SliderValueNumber(x.Value)
	case SliderEventRelease:
		return SliderValueNumber(x.Value)
	}
	return 0
}

// SliderValueNumber is a slider value's number (a range's start).
func SliderValueNumber(v SliderValue) float32 {
	switch x := v.(type) {
	case SliderValueSingle:
		return x.Value
	case SliderValueRange:
		return x.V0
	}
	return 0
}

func kbdNode(action Action, how uint8, keyContext string, focus uint64) Element {
	n := newNode(nKbdForAction)
	o := nodeArgs(n)
	putAction(o, action)
	o.u8(how)
	if keyContext != "" {
		o.u8(1)
		o.str(keyContext)
	} else {
		o.u8(0)
	}
	o.u64(focus)
	return valueEl{n}
}

// KbdForAction draws the keystrokes bound to action as the kit's Kbd, or nothing when no binding
// has it (Kbd::binding_for_action). gk.WhenIn("Editor") looks in that key context.
//
//	gk.HFlex(s.Gap2).Child(gk.Label("Save")).Child(gk.KbdForAction(a.save))
func KbdForAction(action Action, opts ...ShortcutOption) Element {
	var so shortcutOpts
	for _, o := range opts {
		o(&so)
	}
	return kbdNode(action, 0, so.when, 0)
}

// KbdForActionIn draws the binding of action that reaches focus (Kbd::binding_for_action_in).
func KbdForActionIn(action Action, focus FocusHandle) Element { return kbdNode(action, 1, "", focus.h) }

// KbdGlobalForAction draws a binding of action with no key context (Kbd::global_binding_for_action).
func KbdGlobalForAction(action Action) Element { return kbdNode(action, 2, "", 0) }

// NewAppMenuBar makes the kit's AppMenuBar (AppMenuBar::new), which draws the menus gk.SetMenus
// gave; gk.AppMenuBarView draws it. In init or a handler.
func NewAppMenuBar(cx *Cx) AppMenuBar {
	t := checkCx(cx)
	if vb.active {
		stop("gk.NewAppMenuBar in a view function: make it in init or in a handler")
	}
	t.flush()
	r := reader{b: mustCall(*newReq(cAppMenuBarNew))}
	return AppMenuBar{r.u64()}
}

// AppMenuBarView draws an app menu bar where the view puts it.
func AppMenuBarView(bar AppMenuBar) Element {
	n := newNode(nAppMenuBar)
	nodeArgs(n).u64(bar.h)
	return valueEl{n}
}

// ClockNow reads the library's clock: std::time::Instant travels as the time since it began. A
// toast stack's bookkeeping takes the app's reading of it:
//
//	a.toasts.Push(cx, id, value, gk.ToastOptions{..}, gk.ClockNow(cx))
func ClockNow(cx *Cx) time.Duration {
	checkCx(cx)
	r := reader{b: mustCall(*newReq(cClockNow))}
	return time.Duration(int64(r.u64()))
}
