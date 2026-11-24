package main

import (
	"fmt"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
)

// Round6 holds per-mode state for TextView defaults, overlay key routing,
// input-edit streams, and editor context-menu tests.
type Round6 struct {
	events    int
	plus      gk.RowSource
	minus     gk.RowSource
	described int
	tvs       editor.TextViewState
	edits     gk.Subscription
}

func init() {
	// chain proves chained TextViewDefaults/TextViewMotion builders preserve
	// InheritTextColor=true and render Markdown TextViewState cleanly.
	modes["chain"] = &mode{init: chainInit, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(300)).Child(editor.TextView(a.r6.tvs))
	}, step: func(a *App, cx *gk.Cx) { fmt.Printf("editor2 chain drawn\n") }, script: "frame\n" + stepScript}
	// overlay-keys proves registering overlay action mappings still reports
	// routed=false when no completion or code-action overlay is currently open.
	modes["overlay-keys"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = editor.NewState(cx).DefaultValue("let x = 1\n").Build()
		a.ed.SetOverlayActionHandler(cx, []editor.TupleInputOverlayKindString{{V0: editor.InputOverlayKindCompletion, V1: "input::Tab"},
			{V0: editor.InputOverlayKindCodeAction, V1: "input::Escape"}})
	}, view: editorBox, step: func(a *App, cx *gk.Cx) {
		fmt.Printf("editor2 overlay-keys routed=%v\n", a.ed.RouteOverlayAction(cx, gk.NewAction(cx, "input::Tab")))
	}, script: "frame\n" + stepScript}
	// edits proves OnInputEditsEvent delivers exact UTF-8 byte ranges for typing
	// "c" (2..2 -> 3, "abc") and Backspace (2..3 -> 2, "ab").
	modes["edits"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.inp = editor.NewInputState(cx).DefaultValue("ab").Build()
		a.r6.edits = a.inp.OnInputEditsEvent(cx, a.onEdits)
	}, view: inputBox, step: func(a *App, cx *gk.Cx) { fmt.Printf("editor2 %s stepped\n", a.mode) },
		script: "click 100 72\nkey end\ntype c\nframe\nkey backspace\nframe\n" + stepScript}
	modes["context-menu"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.inp = editor.NewInputState(cx).DefaultValue("menu text").Build()
		a.inp.OnContextMenu(cx, a.onMenu)
	}, view: inputBox, step: func(a *App, cx *gk.Cx) {
		caps := a.inp.ContextMenuCapabilities(cx)
		fmt.Printf("editor2 context-menu events=%d caps=%v\n", a.r6.events, gk.Handle(caps) != 0)
	}, script: "click 100 72\nframe\nrclick 60 72\nframe\nadvance 50\nframe\nframe\n" + stepScript}
}

func editorBox(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(420)).H(gk.Px(120)).Child(editor.BaseEditor(a.ed))
}

func inputBox(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(220)).Child(editor.Input(a.inp))
}

func chainInit(a *App, cx *gk.Cx) {
	a.r6.tvs = editor.NewTextViewStateMarkdown(cx, "Chain **text**").
		Motion(editor.NewTextViewMotionDefault().WithStreamFade(200 * time.Millisecond)).Build()
	d := editor.NewTextViewDefaults(cx).WithStyle(editor.NewBaseTextViewStyleDefault().WithParagraphGap(6)).
		WithInheritTextColor(true).Build()
	fmt.Printf("editor2 chain inherit=%v\n", d.InheritTextColor(cx))
}

func (a *App) onEdits(cx *gk.Cx, e editor.InputEditsEvent) {
	for _, x := range e.Edits {
		fmt.Printf("editor2 edits start=%d old_end=%d new_end=%d text=%q\n", x.V0.StartByte, x.V0.OldEndByte, x.V0.NewEndByte, x.V1)
	}
}

func (a *App) onMenu(cx *gk.Cx, e editor.Event114) {
	cp := gk.NewAction(cx, "input::Copy")
	actions, same := 0, 0
	for _, it := range e.A0.Items {
		if x, ok := it.(editor.NativeMenuItemAction); ok {
			actions++
			if x.Action == cp {
				same++
				fmt.Printf("editor2 context-menu copy label=%q disabled=%v\n", x.Label, x.Disabled)
			}
		}
	}
	a.r6.events++
	fmt.Printf("editor2 context-menu items=%d actions=%d copy=%d at_x=%v\n", len(e.A0.Items), actions, same, e.A2.X > 0)
}
