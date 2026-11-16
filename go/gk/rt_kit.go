package gk

// Small hand-written pieces of the kit's API that Odin also writes by hand (rt_overlay.odin,
// rt_motion.odin, kit_input.odin): a button with a dropdown menu, the kit's effect transition
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
