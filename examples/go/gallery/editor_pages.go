// The editor family's gallery pages (docs/PROTOCOLS.md 9.20 to 9.40), the Go twin of
// ref/src/bin/gallery/editor_pages.rs and examples/odin/gallery/editor_pages.odin, call for call:
//
//	editor5-surface   a block surface: the app's Div around a blockquote's blocks (a typed fragment)
//	editor5-heading   a text style's headings by level (a table the app gives for Fn(u8) -> style)
//	editor5-plugin    Markdown plugins: the kit's frontmatter plugin, a data plugin drawing ```badge
//	                  code blocks as rows of the app's, another drawing inline code inside the line
//	editor5-tooltip   a link's tooltip: the row of the link's URL (an item function over a value)
//	editor6-host      the editor host (9.30): the app's blocks (a heading, a paragraph of inline
//	                  nodes, a list of items, a code block) and parsed ones (held values) drawn by
//	                  gk.BlockHost, a selection painted behind a keyed leaf
//	editor7-number    decorators kept for later (9.32, 9.40): a number input's buttons given a style
//	                  and a row each (gk.Decorate), the state made in init
//	editor7-scroller  a message scroller scrolled up: its jump button decorated (a style, a row); no
//	                  scrollbar (its fade after a scroll follows the wall clock, so no two runs agree)
//	editor7-lsp       the Lsp's completion highlighter (9.33, an ask): the menu the app presents on a
//	                  click, each label highlighted where the app says
//	editor7-diagnostic  a diagnostic presented on a click (9.34): the editor's popover
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type editorPages struct {
	blocks  gk.RowSource // editor5-plugin: the badge blocks
	inlines gk.RowSource // editor5-plugin: the inline code
	tips    gk.RowSource // editor5-tooltip: one row per URL
	held    []gk.BlockNodeHeld
	// the editor7-* pages
	number   gk.InputState // editor7-number
	plus     gk.RowSource
	minus    gk.RowSource
	scroller gk.MessageScrollerState // editor7-scroller
	messages gk.RowSource
	jump     gk.RowSource
	lsp      gk.EditorState // editor7-lsp
	hl       gk.Ask[gk.TupleStringCompletionItem, []gk.Range]
	diag     gk.EditorState // editor7-diagnostic
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
	return func(cx *gk.Cx, at gk.RowAt) gk.Element { return gk.Label(text) }
}

func editorPagesInit(a *App, cx *gk.Cx) {
	p := &a.editor
	p.blocks = gk.Items(cx, editorBadgeRow)
	p.inlines = gk.Items(cx, editorCodeRow)
	p.tips = gk.Items(cx, editorTipRow)
	parsed, _ := gk.BaseParseMarkdown(cx, editorHostMD, gk.NewMarkdownExtensionsArc(cx, gk.NewMarkdownExtensionsDefault())).Value.Get()
	for _, b := range parsed {
		gk.Retain(cx, b)
		p.held = append(p.held, b)
	}
	// the editor7-* pages
	p.number = gk.NewInputState(cx).DefaultValue("4").Build()
	p.plus = gk.NewRowSource(cx, 1, editorRowLabel("+"), gk.Initial(1))
	p.minus = gk.NewRowSource(cx, 1, editorRowLabel("-"), gk.Initial(1))
	p.scroller = gk.NewMessageScrollerState(cx, editorMessages)
	p.messages = gk.NewRowSource(cx, editorMessages, func(cx *gk.Cx, at gk.RowAt) gk.Element {
		return gk.Label(fmt.Sprintf("message %d", at.Row))
	}, gk.Initial(editorMessages))
	p.jump = gk.NewRowSource(cx, 1, editorRowLabel("latest"), gk.Initial(1))
	p.hl = gk.NewAsk(cx, editorHighlight)
	p.lsp = gk.NewEditorState(cx).DefaultValue("fi").Build()
	p.lsp.LspMut(cx, gk.NewLspDefault().WithCompletionHighlighter(p.hl))
	p.diag = gk.NewEditorState(cx).DefaultValue("let x = 1\n").Build()
}

func editorSurface(a *App, cx *gk.Cx) gk.Element {
	ext := gk.NewMarkdownExtensionsDefault().BlockSurface("note", gk.Div(s.P2).Bg(theme.Secondary).Child(gk.Label("Note")))
	return gk.Div().W(gk.Px(360)).Child(gk.BaseTextViewMarkdown("surface", editorSurfaceMD).MarkdownExtensions(ext))
}

func editorHeading(a *App, cx *gk.Cx) gk.Element {
	// By level: 0 is never asked, 1 and 2 are colored, the rest (past the end) upstream's default.
	levels := []gk.StyleRefinement{
		gk.NewStyleRefinementDefault(),
		gk.NewStyleRefinementDefault().TextColor(theme.Danger),
		gk.NewStyleRefinementDefault().TextColor(theme.Success),
	}
	style := gk.NewBaseTextViewStyleDefault().WithHeading(levels)
	// The next page's text view drawn here first: a plugin's rows are described the frame after
	// its nodes are parsed, and a page's first shot is taken on its first frame. Laid out with no
	// height, so this page does not show it.
	return gk.Div().W(gk.Px(360)).Child(gk.BaseTextViewMarkdown("heading", editorHeadingMD).Style(style)).
		Child(gk.Div(s.OverflowHidden).H(gk.Px(0)).Child(editorPlugin(a, cx)))
}

func editorBadgeRow(cx *gk.Cx, n gk.MarkdownNodeRead) gk.Element {
	return gk.Label(fmt.Sprintf("[%s]", n.AsText(cx)))
}

// editorCodeRow is an inline plugin's row: the node is the first value the kit hands it (then the
// inline render context, which this row does not read).
func editorCodeRow(cx *gk.Cx, n gk.MarkdownNodeRead) gk.Element {
	return gk.Label(fmt.Sprintf("<%s>", n.AsText(cx)))
}

func editorPlugin(a *App, cx *gk.Cx) gk.Element {
	p := &a.editor
	ext := gk.NewMarkdownExtensionsDefault().Frontmatter().Plugin(gk.NewFrontmatterPlugin())
	return gk.Div().W(gk.Px(420)).Child(gk.BaseTextViewMarkdown("plugin", editorPluginMD).MarkdownExtensions(ext).
		Plugin(gk.MarkdownPlugin{Name: "badge", Match: gk.MarkdownMatchCode, Lang: "badge", Rows: p.blocks}).
		Plugin(gk.MarkdownPlugin{Name: "code", Match: gk.MarkdownMatchInlineCode, Rows: p.inlines, Baseline: 12}))
}

func editorTipRow(cx *gk.Cx, url string) gk.Element {
	return gk.Label(fmt.Sprintf("open %s", url))
}

func editorTooltip(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(360)).Child(gk.BaseTextViewMarkdown("tooltip", editorTooltipMD).LinkTooltip(a.editor.tips))
}

func editorHost(a *App, cx *gk.Cx) gk.Element {
	var bs []gk.BlockNode
	bs = append(bs, gk.BlockNodeHeading{Level: 2, Children: gk.NewParagraphDefault().Push(gk.NewInlineNode("Host")).WithLeafKey(1)})
	bs = append(bs, gk.BlockNodeParagraph{Value: gk.NewParagraphDefault().
		Push(gk.NewInlineNode("Hello ").Marks([]gk.TupleRangeTextMark{{V0: gk.Range{Lo: 0, Hi: 5}, V1: gk.NewTextMarkDefault().Bold()}})).
		Push(gk.NewInlineNode("world")).WithLeafKey(2)})
	one := []gk.BlockNode{gk.BlockNodeParagraph{Value: gk.NewParagraphDefault().Push(gk.NewInlineNode("first")).WithLeafKey(3)}}
	two := []gk.BlockNode{gk.BlockNodeParagraph{Value: gk.NewParagraphDefault().Push(gk.NewInlineNode("second"))}}
	items := []gk.BlockNode{
		gk.BlockNodeListItem{Children: one},
		gk.BlockNodeListItem{Children: two, Checked: gk.Some(true)},
	}
	bs = append(bs, gk.BlockNodeList{Children: items})
	bs = append(bs, gk.BlockNodeCodeBlock{Value: gk.NewBaseCodeBlockFromCode("x := 1", gk.Some("odin")).WithLeafKey(4)})
	for _, h := range a.editor.held {
		bs = append(bs, h)
	}
	decs := []gk.LeafDecoration{{Key: 2, Ranges: []gk.TupleRangeColor{{V0: gk.Range{Lo: 0, Hi: 5}, V1: theme.Selection}}}}
	return gk.Div().W(gk.Px(420)).Child(gk.BlockHost("editor6-host", bs, nil, gk.Option[gk.RenderContext]{}, decs, nil))
}

// --- docs/PROTOCOLS.md 9.38 to 9.40

func editorNumber(a *App, cx *gk.Cx) gk.Element {
	p := &a.editor
	return gk.Div().W(gk.Px(200)).Child(gk.BaseNumberInput(p.number).
		IncrementButton(gk.Decorate{Style: gk.Some(gk.NewStyleRefinementDefault().Bg(theme.Primary)), Content: p.plus}).
		DecrementButton(gk.Decorate{Content: p.minus}))
}

func editorScroller(a *App, cx *gk.Cx) gk.Element {
	p := &a.editor
	return gk.Div().W(gk.Px(300)).H(gk.Px(200)).Child(gk.MessageScroller("scroller", p.scroller, p.messages).Scrollbar(false).
		WithJumpButtonRenderer(gk.Decorate{Style: gk.Some(gk.NewStyleRefinementDefault().Bg(theme.Danger)), Content: p.jump}))
}

// editorHighlight: the completion menu's highlighter, the second and third character of each label.
func editorHighlight(cx *gk.Cx, key gk.TupleStringCompletionItem) []gk.Range {
	n := len(key.V1.Label)
	return []gk.Range{{Lo: min(1, n), Hi: min(3, n)}}
}

func editorLsp(a *App, cx *gk.Cx) gk.Element {
	complete := func(cx *gk.Cx, _ gk.ClickEvent) {
		items := []gk.CompletionItem{{Label: "first", Kind: 3, Detail: gk.Some("fn first()")}, {Label: "fifth", Kind: 6}}
		a.editor.lsp.PresentCompletionItems(cx, 2, "fi", items)
	}
	return gk.VFlex(s.Gap2).Child(gk.Button("complete").Label("Complete").OnClick(complete)).
		Child(gk.Div().W(gk.Px(420)).Child(gk.CodeEditor(a.editor.lsp, gk.CodeEditorOptions{Height: 160})))
}

func editorDiagnostic(a *App, cx *gk.Cx) gk.Element {
	diagnose := func(cx *gk.Cx, _ gk.ClickEvent) {
		d := gk.NewDiagnostic([2]gk.LspPosition{{Line: 0, Character: 4}, {Line: 0, Character: 5}}, "unused x").
			WithSeverity(gk.DiagnosticSeverityWarning)
		a.editor.diag.PresentDiagnostic(cx, gk.Range{Lo: 4, Hi: 5}, d)
	}
	return gk.VFlex(s.Gap2).Child(gk.Button("diagnose").Label("Diagnose").OnClick(diagnose)).
		Child(gk.Div().W(gk.Px(420)).Child(gk.CodeEditor(a.editor.diag, gk.CodeEditorOptions{Height: 160})))
}
