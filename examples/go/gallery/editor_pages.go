// Editor and Markdown gallery pages: custom block surfaces, heading styles, data plugins,
// link tooltips, BlockHost with held parsed blocks, number inputs, message scrollers, and LSP UI.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type editorPages struct {
	blocks   gk.RowSource
	inlines  gk.RowSource
	tips     gk.RowSource
	held     []editor.BlockNodeHeld
	number   editor.InputState
	plus     gk.RowSource
	minus    gk.RowSource
	scroller kit.MessageScrollerState
	messages gk.RowSource
	jump     gk.RowSource
	lsp      editor.State
	hl       gk.Ask[gk.TupleStringCompletionItem, []gk.Range]
	diag     editor.State
}

const (
	editorSurfaceMD = "> [!note]\n> Inside the surface.\n\nAfter it.\n"
	editorHeadingMD = "# One\n\n## Two\n\n### Three\n\nBody text.\n"
	editorPluginMD  = "---\ntitle: Notes\nowner: me\n---\n\n# Plugins\n\n```badge\nready\n```\n\nSome `inline` text.\n"
	editorTooltipMD = "[docs](https://example.com/docs) and more"
	editorHostMD    = "## Parsed\n\n- one\n- two\n\n> quoted *text*\n"
	editorMessages  = 30
)

var editorPageTable = []Page{
	{"editor5-surface", editorSurface},
	{"editor5-heading", editorHeading},
	{"editor5-plugin", editorPlugin},
	{"editor5-tooltip", editorTooltip},
	{"editor6-host", editorHost},
	{"editor7-number", editorNumber},
	{"editor7-scroller", editorScroller},
	{"editor7-lsp", editorLsp},
	{"editor7-diagnostic", editorDiagnostic},
}

func editorRowLabel(text string) func(cx *gk.Cx, at gk.RowAt) gk.Element {
	return func(cx *gk.Cx, at gk.RowAt) gk.Element { return kit.Label(text) }
}

func editorPagesInit(a *App, cx *gk.Cx) {
	p := &a.editor
	p.blocks = overlay.Items(cx, editorBadgeRow)
	p.inlines = overlay.Items(cx, editorCodeRow)
	p.tips = overlay.Items(cx, editorTipRow)
	parsed, _ := editor.BaseParseMarkdown(cx, editorHostMD, editor.NewMarkdownExtensionsArc(cx, editor.NewMarkdownExtensionsDefault())).Value.Get()
	for _, b := range parsed {
		gk.Retain(cx, b)
		p.held = append(p.held, b)
	}
	p.number = editor.NewInputState(cx).DefaultValue("4").Build()
	p.plus = gk.NewRowSource(cx, 1, editorRowLabel("+"), gk.Initial(1))
	p.minus = gk.NewRowSource(cx, 1, editorRowLabel("-"), gk.Initial(1))
	p.scroller = kit.NewMessageScrollerState(cx, editorMessages)
	p.messages = gk.NewRowSource(cx, editorMessages, func(cx *gk.Cx, at gk.RowAt) gk.Element {
		return kit.Label(fmt.Sprintf("message %d", at.Row))
	}, gk.Initial(editorMessages))
	p.jump = gk.NewRowSource(cx, 1, editorRowLabel("latest"), gk.Initial(1))
	p.hl = gk.NewAsk(cx, editorHighlight)
	p.lsp = editor.NewState(cx).DefaultValue("fi").Build()
	p.lsp.LspMut(cx, editor.NewLspDefault().WithCompletionHighlighter(p.hl))
	p.diag = editor.NewState(cx).DefaultValue("let x = 1\n").Build()
}

func editorSurface(a *App, cx *gk.Cx) gk.Element {
	ext := editor.NewMarkdownExtensionsDefault().BlockSurface("note", gk.Div(s.P2).Bg(theme.Secondary).Child(kit.Label("Note")))
	return gk.Div().W(gk.Px(360)).Child(editor.BaseTextViewMarkdown("surface", editorSurfaceMD).MarkdownExtensions(ext))
}

func editorHeading(a *App, cx *gk.Cx) gk.Element {
	levels := []gk.StyleRefinement{
		gk.NewStyleRefinementDefault(),
		gk.NewStyleRefinementDefault().TextColor(theme.Danger),
		gk.NewStyleRefinementDefault().TextColor(theme.Success),
	}
	style := editor.NewBaseTextViewStyleDefault().WithHeading(levels)
	return gk.Div().W(gk.Px(360)).Child(editor.BaseTextViewMarkdown("heading", editorHeadingMD).Style(style)).
		Child(gk.Div(s.OverflowHidden).H(gk.Px(0)).Child(editorPlugin(a, cx)))
}

func editorBadgeRow(cx *gk.Cx, n gk.MarkdownNodeRead) gk.Element {
	return kit.Label(fmt.Sprintf("[%s]", n.AsText(cx)))
}

func editorCodeRow(cx *gk.Cx, n gk.MarkdownNodeRead) gk.Element {
	return kit.Label(fmt.Sprintf("<%s>", n.AsText(cx)))
}

func editorPlugin(a *App, cx *gk.Cx) gk.Element {
	p := &a.editor
	ext := editor.NewMarkdownExtensionsDefault().Frontmatter().Plugin(editor.NewFrontmatterPlugin())
	return gk.Div().W(gk.Px(420)).Child(editor.BaseTextViewMarkdown("plugin", editorPluginMD).MarkdownExtensions(ext).
		Plugin(gk.MarkdownPlugin{Name: "badge", Match: gk.MarkdownMatchCode, Lang: "badge", Rows: p.blocks}).
		Plugin(gk.MarkdownPlugin{Name: "code", Match: gk.MarkdownMatchInlineCode, Rows: p.inlines, Baseline: 12}))
}

func editorTipRow(cx *gk.Cx, url string) gk.Element {
	return kit.Label(fmt.Sprintf("open %s", url))
}

func editorTooltip(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(360)).Child(editor.BaseTextViewMarkdown("tooltip", editorTooltipMD).LinkTooltip(a.editor.tips))
}

func editorHost(a *App, cx *gk.Cx) gk.Element {
	var bs []editor.BlockNode
	bs = append(bs, editor.BlockNodeHeading{Level: 2, Children: editor.NewParagraphDefault().Push(editor.NewInlineNode("Host")).WithLeafKey(1)})
	bs = append(bs, editor.BlockNodeParagraph{Value: editor.NewParagraphDefault().
		Push(editor.NewInlineNode("Hello ").Marks([]editor.TupleRangeTextMark{{V0: gk.Range{Lo: 0, Hi: 5}, V1: editor.NewTextMarkDefault().Bold()}})).
		Push(editor.NewInlineNode("world")).WithLeafKey(2)})
	one := []editor.BlockNode{editor.BlockNodeParagraph{Value: editor.NewParagraphDefault().Push(editor.NewInlineNode("first")).WithLeafKey(3)}}
	two := []editor.BlockNode{editor.BlockNodeParagraph{Value: editor.NewParagraphDefault().Push(editor.NewInlineNode("second"))}}
	items := []editor.BlockNode{
		editor.BlockNodeListItem{Children: one},
		editor.BlockNodeListItem{Children: two, Checked: gk.Some(true)},
	}
	bs = append(bs, editor.BlockNodeList{Children: items})
	bs = append(bs, editor.BlockNodeCodeBlock{Value: editor.NewBaseCodeBlockFromCode("x := 1", gk.Some("demo")).WithLeafKey(4)})
	for _, h := range a.editor.held {
		bs = append(bs, h)
	}
	decs := []editor.LeafDecoration{{Key: 2, Ranges: []gk.TupleRangeColor{{V0: gk.Range{Lo: 0, Hi: 5}, V1: theme.Selection}}}}
	return gk.Div().W(gk.Px(420)).Child(editor.BlockHost("editor6-host", bs, nil, gk.Option[editor.RenderContext]{}, decs, nil))
}

func editorNumber(a *App, cx *gk.Cx) gk.Element {
	p := &a.editor
	return gk.Div().W(gk.Px(200)).Child(editor.BaseNumberInput(p.number).
		IncrementButton(gk.Decorate{Style: gk.Some(gk.NewStyleRefinementDefault().Bg(theme.Primary)), Content: p.plus}).
		DecrementButton(gk.Decorate{Content: p.minus}))
}

func editorScroller(a *App, cx *gk.Cx) gk.Element {
	p := &a.editor
	return gk.Div().W(gk.Px(300)).H(gk.Px(200)).Child(kit.MessageScroller("scroller", p.scroller, p.messages).Scrollbar(false).
		WithJumpButtonRenderer(gk.Decorate{Style: gk.Some(gk.NewStyleRefinementDefault().Bg(theme.Danger)), Content: p.jump}))
}

func editorHighlight(cx *gk.Cx, key gk.TupleStringCompletionItem) []gk.Range {
	n := len(key.V1.Label)
	return []gk.Range{{Lo: min(1, n), Hi: min(3, n)}}
}

func editorLsp(a *App, cx *gk.Cx) gk.Element {
	complete := func(cx *gk.Cx, _ gk.ClickEvent) {
		items := []gk.CompletionItem{{Label: "first", Kind: 3, Detail: gk.Some("fn first()")}, {Label: "fifth", Kind: 6}}
		a.editor.lsp.PresentCompletionItems(cx, 2, "fi", items)
	}
	return gk.VFlex(s.Gap2).Child(kit.Button("complete").Label("Complete").OnClick(complete)).
		Child(gk.Div().W(gk.Px(420)).Child(editor.CodeEditor(a.editor.lsp, editor.CodeEditorOptions{Height: 160})))
}

func editorDiagnostic(a *App, cx *gk.Cx) gk.Element {
	diagnose := func(cx *gk.Cx, _ gk.ClickEvent) {
		d := editor.NewDiagnostic([2]gk.LspPosition{{Line: 0, Character: 4}, {Line: 0, Character: 5}}, "unused x").
			WithSeverity(editor.DiagnosticSeverityWarning)
		a.editor.diag.PresentDiagnostic(cx, gk.Range{Lo: 4, Hi: 5}, d)
	}
	return gk.VFlex(s.Gap2).Child(kit.Button("diagnose").Label("Diagnose").OnClick(diagnose)).
		Child(gk.Div().W(gk.Px(420)).Child(editor.CodeEditor(a.editor.diag, editor.CodeEditorOptions{Height: 160})))
}
