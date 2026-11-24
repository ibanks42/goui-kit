// editor2: the protocol tests of docs/PROTOCOLS.md 9.30 to 9.37.
//
//	GK_PROTO=host        the editor host: the app's blocks (a heading, a paragraph of inline nodes,
//	                     a list of list items, a code block: recursive variants) and Markdown parsed
//	                     by gk.BaseParseMarkdown (held values, kept by gk.Retain) drawn by
//	                     gk.BlockHost with a flow, a render context with Markdown extensions and a
//	                     decoration; each keyed leaf's geometry after the frame, read back (bounds,
//	                     lengths, a position and the index it maps back to, a caret); delivered
//	                     once while nothing changes; a parsed table's rows read back
//	                     (host.go)
//	chain                a text view state whose builder chain holds a value node (motion), and the
//	                     text view defaults with a style in theirs: decoded with the call's context
//	decorate             a number input's buttons decorated: a style and a row each, the rows
//	                     described when the buttons are laid out (no window where upstream calls)
//	                     (host.go)
//	lsp                  the editor's Lsp edited in place (LspMut) with a completion highlighter
//	                     (host.go)
//	diagnostic           a diagnostic presented on the editor (a record holding a value node)
//	                     (host.go)
//	overlay-keys         the overlay actions the app takes, given in advance; routed with none open
//	edits                an input's InputEditsEvent: the edits and the text after each, typed in
//	context-menu         an input's context menu read back by its handler: the items' actions as
//	                     the app's action ids
package main

import (
	"fmt"
	"time"

	"gpuikit.dev/gk"
)

// Round6 is the state of this file's modes (host.go keeps its own).
type Round6 struct {
	events    int
	plus      gk.RowSource
	minus     gk.RowSource
	described int
	tvs       gk.TextViewState
	edits     gk.Subscription
}

func init() {
	modes["chain"] = &mode{init: chainInit, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(300)).Child(gk.TextView(a.r6.tvs))
	}, step: func(a *App, cx *gk.Cx) { fmt.Printf("editor2 chain drawn\n") }, script: "frame\n" + stepScript}
	modes["overlay-keys"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = gk.NewEditorState(cx).DefaultValue("let x = 1\n").Build()
		a.ed.SetOverlayActionHandler(cx, []gk.TupleInputOverlayKindString{{V0: gk.InputOverlayKindCompletion, V1: "input::Tab"},
			{V0: gk.InputOverlayKindCodeAction, V1: "input::Escape"}})
	}, view: editorBox, step: func(a *App, cx *gk.Cx) {
		fmt.Printf("editor2 overlay-keys routed=%v\n", a.ed.RouteOverlayAction(cx, gk.NewAction(cx, "input::Tab")))
	}, script: "frame\n" + stepScript}
	modes["edits"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.inp = gk.NewInputState(cx).DefaultValue("ab").Build()
		a.r6.edits = a.inp.OnInputEditsEvent(cx, a.onEdits)
	}, view: inputBox, step: func(a *App, cx *gk.Cx) { fmt.Printf("editor2 %s stepped\n", a.mode) },
		script: "click 100 72\nkey end\ntype c\nframe\nkey backspace\nframe\n" + stepScript}
	modes["context-menu"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.inp = gk.NewInputState(cx).DefaultValue("menu text").Build()
		a.inp.OnContextMenu(cx, a.onMenu)
	}, view: inputBox, step: func(a *App, cx *gk.Cx) {
		caps := a.inp.ContextMenuCapabilities(cx)
		fmt.Printf("editor2 context-menu events=%d caps=%v\n", a.r6.events, gk.Handle(caps) != 0)
	}, script: "click 100 72\nframe\nrclick 60 72\nframe\nadvance 50\nframe\nframe\n" + stepScript}
}

// editorBox and inputBox are the bodies of the modes that draw the base editor or an input.
func editorBox(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(420)).H(gk.Px(120)).Child(gk.BaseEditor(a.ed))
}

func inputBox(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(220)).Child(gk.Input(a.inp))
}

func chainInit(a *App, cx *gk.Cx) {
	a.r6.tvs = gk.NewTextViewStateMarkdown(cx, "Chain **text**").
		Motion(gk.NewTextViewMotionDefault().WithStreamFade(200 * time.Millisecond)).Build()
	d := gk.NewTextViewDefaults(cx).WithStyle(gk.NewBaseTextViewStyleDefault().WithParagraphGap(6)).
		WithInheritTextColor(true).Build()
	fmt.Printf("editor2 chain inherit=%v\n", d.InheritTextColor(cx))
}

func (a *App) onEdits(cx *gk.Cx, e gk.InputEditsEvent) {
	for _, x := range e.Edits {
		fmt.Printf("editor2 edits start=%d old_end=%d new_end=%d text=%q\n", x.V0.StartByte, x.V0.OldEndByte, x.V0.NewEndByte, x.V1)
	}
}

func (a *App) onMenu(cx *gk.Cx, e gk.Event114) {
	cp := gk.NewAction(cx, "input::Copy")
	actions, same := 0, 0
	for _, it := range e.A0.Items {
		if x, ok := it.(gk.NativeMenuItemAction); ok {
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
