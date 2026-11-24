// editor2: the protocol tests of docs/PROTOCOLS.md 9.38 to 9.40.
//
//	GK_PROTO=custom      a parsed Custom block (the kit's frontmatter plugin claims the document's
//	                     YAML: BlockNode::Custom wrapping the parser's MarkdownNode, a held value)
//	                     placed among the app's own blocks by gk.BlockHost, its flow given by
//	                     FlowPosition::at over a list holding it; the keyed leaves around it read
//	                     back: the block takes room (the leaf after it lies lower than in a host
//	                     without it)
//	overlay-fire         the overlay actions the app takes, given in advance, fired by a key: the
//	                     completion menu (presented by the app) is open, Enter is the app's, so
//	                     the overlay closes and the editor does nothing with it; with the menu
//	                     closed the same action routes nowhere
//	overlay-control      the same without the key set: Enter is the editor's (a new line). Both draw the
//	                     base Editor: the kit's code editor installs its own overlay handler
//	lsp-ask              the Lsp's completion highlighter (an ask, 9.33) consulted by the open
//	                     completion menu: the query and each item's label asked, the ranges the
//	                     app answers kept
//	diagnostic-shown     a diagnostic presented on the editor (9.34) and read back as the popover
//	                     the editor shows; cleared
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/s"
)

const customMD = "---\ntitle: Notes\nowner: me\n---\n\nAfter the front matter.\n"

// Round7 is the state of this file's modes.
var r7 struct {
	ext        editor.MarkdownExtensionsArc
	custom     editor.BlockNodeHeld
	parsed     int
	tops       map[uint64]float32 // leaf key -> top
	bottoms    map[uint64]float32 // leaf key -> bottom
	leafEvents int
	presented  bool
	asked      int
	hl         gk.Ask[gk.TupleStringCompletionItem, []gk.Range]
}

// The scripts of this file's modes. The editor's first line is at y 72; a click at x 200 puts the
// caret after its text.
const overlayScript = "click 200 72\nframe\ntype x\nframe\nframe\nadvance 100\nframe\nframe\nkey enter\nframe\nframe\n" + stepScript

func init() {
	r7.tops = map[uint64]float32{}
	r7.bottoms = map[uint64]float32{}
	modes["custom"] = &mode{init: customInit, view: customView, step: func(a *App, cx *gk.Cx) {
		gapWith := r7.tops[2] - r7.bottoms[1]
		gapWithout := r7.tops[4] - r7.bottoms[3]
		fmt.Printf("editor2 custom leaves=%d events=%d placed=%v gap_with=%.0f gap_without=%.0f\n", len(r7.tops),
			r7.leafEvents, gapWith > gapWithout, gapWith, gapWithout)
	}, script: "frame\nframe\nframe\nframe\n" + stepScript}
	// The base Editor: an editor drawn outside the kit's overlays, which is what the key set is for. The
	// kit's code editor (component::input::Editor) installs its own overlay handler each time it renders
	// (its completion menu takes the menu's actions), replacing the app's.
	baseEditor := func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(420)).H(gk.Px(160)).Child(editor.BaseEditor(a.ed))
	}
	// The kit's code editor (component::input::Editor): its overlays draw the completion menu and the
	// diagnostic popover.
	codeEditor := func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(420)).Child(editor.CodeEditor(a.ed, editor.CodeEditorOptions{Height: 160}))
	}
	modes["overlay-fire"] = &mode{init: overlayInit, view: baseEditor, step: overlayStep, script: overlayScript}
	modes["overlay-control"] = &mode{init: overlayInit, view: baseEditor, step: overlayStep, script: overlayScript}
	modes["lsp-ask"] = &mode{init: func(a *App, cx *gk.Cx) {
		r7.hl = gk.NewAsk(cx, a.highlight7)
		a.ed = editor.NewState(cx).DefaultValue("fi").Build()
		a.ed.LspMut(cx, editor.NewLspDefault().WithCompletionHighlighter(r7.hl))
		a.r6.edits = a.ed.OnInputEditsEvent(cx, a.presentOnEdit)
	}, view: codeEditor, step: func(a *App, cx *gk.Cx) {
		fmt.Printf("editor2 lsp-ask asked=%d\n", r7.asked)
	}, script: "click 200 72\nframe\ntype x\nframe\nframe\nadvance 100\nframe\nframe\nframe\nframe\n" + stepScript}
	modes["diagnostic-shown"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = editor.NewState(cx).DefaultValue("let x = 1\n").Build()
		a.ed.PresentDiagnostic(cx, gk.Range{Lo: 4, Hi: 5}, editor.NewDiagnostic([2]gk.LspPosition{{Line: 0, Character: 4}, {Line: 0, Character: 5}}, "unused x").
			WithSeverity(editor.DiagnosticSeverityWarning))
	}, view: codeEditor, step: diagnosticShownStep, script: "frame\nframe\n" + stepScript + stepScript}
}

func customInit(a *App, cx *gk.Cx) {
	r7.ext = editor.NewMarkdownExtensionsArc(cx, editor.NewMarkdownExtensionsDefault().Frontmatter().Plugin(editor.NewFrontmatterPlugin()))
	parsed := editor.BaseParseMarkdown(cx, customMD, r7.ext)
	blocks, ok := parsed.Value.Get()
	r7.parsed = len(blocks)
	fmt.Printf("editor2 custom parsed ok=%v n=%d\n", ok, len(blocks))
	if ok && len(blocks) > 0 {
		gk.Retain(cx, blocks[0]) // the front matter: BlockNode::Custom
		r7.custom = blocks[0]
	}
}

func overlayInit(a *App, cx *gk.Cx) {
	a.ed = editor.NewState(cx).DefaultValue("ab").Build()
	if a.mode == "overlay-fire" {
		a.ed.SetOverlayActionHandler(cx, []editor.TupleInputOverlayKindString{{V0: editor.InputOverlayKindCompletion, V1: "input::Enter"}})
	}
	a.r6.edits = a.ed.OnInputEditsEvent(cx, a.presentOnEdit)
}

// presentOnEdit opens the completion menu at the caret once the user typed (the editor is focused then,
// so a key reaches it): two items, the query the text typed so far.
func (a *App) presentOnEdit(cx *gk.Cx, e editor.InputEditsEvent) {
	if r7.presented || len(e.Edits) == 0 {
		return
	}
	r7.presented = true
	last := e.Edits[len(e.Edits)-1]
	items := []gk.CompletionItem{{Label: "first", Kind: 3, Detail: gk.Some("fn first()")}, {Label: "second", Kind: 6}}
	a.ed.PresentCompletionItems(cx, last.V0.NewEndByte, "", items)
	fmt.Printf("editor2 %s presented at=%d text=%q\n", a.mode, last.V0.NewEndByte, last.V1)
}

func (a *App) highlight7(cx *gk.Cx, key gk.TupleStringCompletionItem) []gk.Range {
	r7.asked++
	fmt.Printf("editor2 lsp-ask asked query=%q label=%q\n", key.V0, key.V1.Label)
	return []gk.Range{{Lo: 0, Hi: min(2, len(key.V1.Label))}}
}

func para7(text string, key uint64) editor.BlockNode {
	return editor.BlockNodeParagraph{Value: editor.NewParagraphDefault().Push(editor.NewInlineNode(text)).WithLeafKey(key)}
}

func customView(a *App, cx *gk.Cx) gk.Element {
	rc := editor.NewRenderContextFromApp().WithMarkdownExtensions(r7.ext)
	with := []editor.BlockNode{para7("Before", 1), r7.custom, para7("After", 2)}
	flows := make([]gk.Option[editor.FlowPosition], len(with))
	for i := range with {
		flows[i] = gk.Some(editor.NewFlowPositionAt(with, i, i == len(with)-1)) // FlowPosition::at over the held block
	}
	without := []editor.BlockNode{para7("Before", 3), para7("After", 4)}
	return gk.HFlex(s.Gap2, s.ItemsStart).
		Child(gk.Div().W(gk.Px(200)).Child(editor.BlockHost("with", with, flows, gk.Some(rc), nil, a.onLeaves7))).
		Child(gk.Div().W(gk.Px(200)).Child(editor.BlockHost("without", without, nil, gk.Some(rc), nil, a.onLeaves7)))
}

func (a *App) onLeaves7(cx *gk.Cx, e editor.LeafGeometryEvent) {
	r7.leafEvents++
	for _, l := range e.Leaves {
		b := l.Geometry.Bounds(cx)
		r7.tops[l.Key] = b.Origin.Y
		r7.bottoms[l.Key] = b.Origin.Y + b.Size.Height
	}
}

func overlayStep(a *App, cx *gk.Cx) {
	routed := a.ed.RouteOverlayAction(cx, gk.NewAction(cx, "input::Enter"))
	fmt.Printf("editor2 %s value=%q routed_after=%v taken=%v\n", a.mode, a.ed.Value(cx), routed, a.ed.HasOverlayActionHandler(cx))
}

func diagnosticShownStep(a *App, cx *gk.Cx) {
	switch a.step {
	case 1:
		shown := a.ed.DiagnosticPopover(cx).Valid()
		fmt.Printf("editor2 diagnostic-shown popover=%v\n", shown)
		a.ed.ClearDiagnosticPopover(cx)
	case 2:
		shown := a.ed.DiagnosticPopover(cx).Valid()
		fmt.Printf("editor2 diagnostic-shown cleared popover=%v\n", shown)
	}
}
