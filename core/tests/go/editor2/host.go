// Round 6's modes host, decorate, lsp and diagnostic (round6.go lists them): the editor host
// (gk.BlockHost) over the app's blocks and parsed ones (gk.BaseParseMarkdown's held values), a number
// input's decorated buttons (gk.Decorate), the editor's Lsp edited in place (LspMut) with a
// completion highlighter (an ask), and a diagnostic presented on the editor.
package main

import (
	"fmt"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/theme"
)

const parsedMD = "## Parsed\n\n- one\n- two\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"

// The state of these modes.
var r6 struct {
	ext  gk.MarkdownExtensionsArc
	held []gk.BlockNodeHeld
	hl   gk.Ask[gk.TupleStringCompletionItem, []gk.Range]
}

func init() {
	modes["host"] = &mode{init: hostInit, view: hostView, step: hostStep, script: "frame\nframe\nframe\nframe\n" + stepScript}
	modes["decorate"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.r6.plus = gk.NewRowSource(cx, 0, a.plusRow, gk.Initial(1))
		a.r6.minus = gk.NewRowSource(cx, 0, a.minusRow, gk.Initial(1))
		a.inp = gk.NewInputState(cx).DefaultValue("4").Build()
	}, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(200)).Child(gk.BaseNumberInput(a.inp).
			IncrementButton(gk.Decorate{Style: gk.Some(gk.NewStyleRefinementDefault().Bg(theme.Primary)), Content: a.r6.plus}).
			DecrementButton(gk.Decorate{Content: a.r6.minus}))
	}, step: func(a *App, cx *gk.Cx) {
		_, _, _, decorated, _ := gk.TestEditorHostCounts(cx)
		fmt.Printf("editor2 decorate decorated=%d described=%d\n", decorated, a.r6.described)
	}, script: "frame\nframe\nframe\n" + stepScript}
	modes["lsp"] = &mode{init: func(a *App, cx *gk.Cx) {
		r6.hl = gk.NewAsk(cx, a.highlight)
		a.ed = gk.NewEditorState(cx).DefaultValue("let x = 1\n").Build()
		a.ed.LspMut(cx, gk.NewLspDefault().WithCompletionHighlighter(r6.hl))
	}, view: editorBox, step: func(a *App, cx *gk.Cx) {
		_, _, _, _, lent := gk.TestEditorHostCounts(cx)
		fmt.Printf("editor2 lsp lent=%d\n", lent)
	}, script: "frame\n" + stepScript}
	modes["diagnostic"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = gk.NewEditorState(cx).DefaultValue("let x = 1\n").Build()
		a.ed.PresentDiagnostic(cx, gk.Range{Lo: 0, Hi: 3}, gk.NewDiagnostic([2]gk.LspPosition{{Line: 0, Character: 0}, {Line: 0, Character: 3}}, "unused").
			WithSeverity(gk.DiagnosticSeverityWarning).WithCode("W1"))
	}, view: editorBox, step: func(a *App, cx *gk.Cx) { fmt.Printf("editor2 diagnostic presented\n") }, script: "frame\n" + stepScript}
}

func hostInit(a *App, cx *gk.Cx) {
	r6.ext = gk.NewMarkdownExtensionsArc(cx, gk.NewMarkdownExtensionsDefault())
	parsed := gk.BaseParseMarkdown(cx, parsedMD, r6.ext)
	blocks, ok := parsed.Value.Get()
	fmt.Printf("editor2 host parsed ok=%v n=%d\n", ok, len(blocks))
	for _, b := range blocks {
		gk.Retain(cx, b) // temporary until retained (13.2)
		r6.held = append(r6.held, b)
	}
}

func (a *App) plusRow(cx *gk.Cx, at gk.RowAt) gk.Element {
	a.r6.described++
	return gk.Label("+")
}

func (a *App) minusRow(cx *gk.Cx, at gk.RowAt) gk.Element {
	a.r6.described++
	return gk.Label("-")
}

func (a *App) highlight(cx *gk.Cx, key gk.TupleStringCompletionItem) []gk.Range {
	return []gk.Range{{Lo: 0, Hi: len(key.V0)}}
}

// The host's blocks: a heading, a paragraph of two inline nodes (the first marked bold), a list of two
// items (variants holding blocks), a code block, then the parsed ones.
func hostBlocks() []gk.BlockNode {
	bs := []gk.BlockNode{
		gk.BlockNodeHeading{Level: 2, Children: gk.NewParagraphDefault().Push(gk.NewInlineNode("Host")).WithLeafKey(1)},
		gk.BlockNodeParagraph{Value: gk.NewParagraphDefault().
			Push(gk.NewInlineNode("Hello ").Marks([]gk.TupleRangeTextMark{{V0: gk.Range{Lo: 0, Hi: 5}, V1: gk.NewTextMarkDefault().Bold()}})).
			Push(gk.NewInlineNode("world")).WithLeafKey(2)},
	}
	one := []gk.BlockNode{gk.BlockNodeParagraph{Value: gk.NewParagraphDefault().Push(gk.NewInlineNode("first")).WithLeafKey(3)}}
	two := []gk.BlockNode{gk.BlockNodeParagraph{Value: gk.NewParagraphDefault().Push(gk.NewInlineNode("second"))}}
	items := []gk.BlockNode{gk.BlockNodeListItem{Children: one}, gk.BlockNodeListItem{Children: two, Checked: gk.Some(true)}}
	bs = append(bs, gk.BlockNodeList{Children: items})
	bs = append(bs, gk.BlockNodeCodeBlock{Value: gk.NewBaseCodeBlockFromCode("x := 1", gk.Some("demo")).WithLeafKey(4)})
	for _, h := range r6.held {
		bs = append(bs, h)
	}
	return bs
}

func hostView(a *App, cx *gk.Cx) gk.Element {
	blocks := hostBlocks()
	flows := make([]gk.Option[gk.FlowPosition], len(blocks))
	flows[0] = gk.Some(gk.NewFlowPositionAt(blocks, 0, false).First(true))
	rc := gk.NewRenderContextFromApp().WithMarkdownExtensions(r6.ext).WithBodyFontSize(14)
	decs := []gk.LeafDecoration{{Key: 2, Ranges: []gk.TupleRangeColor{{V0: gk.Range{Lo: 0, Hi: 5}, V1: theme.Selection}}}}
	return gk.Div().W(gk.Px(420)).Child(gk.BlockHost("doc", blocks, flows, gk.Some(rc), decs, a.onLeaves))
}

func (a *App) onLeaves(cx *gk.Cx, e gk.LeafGeometryEvent) {
	a.r6.events++
	for _, l := range e.Leaves {
		g := l.Geometry
		b := g.Bounds(cx)
		fmt.Printf("editor2 leaf key=%d len=%d empty=%v sized=%v line=%v caret=%v\n", l.Key, g.Len(cx), g.IsEmpty(cx),
			b.Size.Width > 0 && b.Size.Height > 0, g.LineHeight(cx) > 0, g.CaretHeight(cx) > 0)
		if l.Key == 2 {
			p := g.PositionForIndex(cx, 3)
			c := g.CaretForPosition(cx, p)
			back := g.PositionForCaret(cx, c)
			atom := g.AtomAt(cx, p).Valid()
			fmt.Printf("editor2 leaf hit index=%d caret=%d same=%v atom=%v\n", g.IndexForPosition(cx, p), c.Index, back == p, atom)
		}
	}
}

// alignNames prints column alignments in the form the checks expect: ["Left", "Right"].
func alignNames(as []gk.ColumnumnAlign) string {
	names := [...]string{"None", "Left", "Center", "Right"}
	parts := make([]string, len(as))
	for i, x := range as {
		parts[i] = fmt.Sprintf("%q", names[x])
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func hostStep(a *App, cx *gk.Cx) {
	hosts, leaves, delivered, _, _ := gk.TestEditorHostCounts(cx)
	fmt.Printf("editor2 host hosts=%d leaves=%d delivered=%d events=%d\n", hosts, leaves, delivered, a.r6.events)
	row := []gk.Paragraph{gk.NewParagraphDefault().Push(gk.NewInlineNode("cell")), gk.NewParagraphDefault().Push(gk.NewInlineNode("two"))}
	t := gk.NewBaseTable2(cx, [][]gk.Paragraph{row}, []gk.ColumnumnAlign{gk.ColumnumnAlignLeft, gk.ColumnumnAlignRight})
	rows := t.Rows(cx)
	first := rows[0][0].Children(cx)
	fmt.Printf("editor2 host table rows=%d cells=%d first=%q aligns=%s\n", len(rows), len(rows[0]), first[0].Text(cx), alignNames(t.ColumnAligns(cx)))
}
