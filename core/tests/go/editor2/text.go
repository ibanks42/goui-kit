package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

const (
	pluginMD  = "---\ntitle: Notes\nowner: me\n---\n\n# Plugins\n\n```badge\nready\n```\n\nSome `inline` text.\n"
	tooltipMD = "[docs](https://example.com/docs) and more"
	surfaceMD = "> [!note]\n> Inside the surface.\n\nAfter it.\n"
)

// text tracks RowSource handles and invocation counts for markdown block/inline
// plugin renderers and link tooltips.
var text struct {
	blocks, inlines, tips gk.RowSource
	blockRows, inlineRows int
	tipRows               int
	rows                  map[string]int
}

func init() {
	text.rows = map[string]int{}
	// plugin proves Markdown block ("badge" fence) and inline (`inline` code)
	// plugins both receive parsed text/markdown and valid typography metrics.
	modes["plugin"] = &mode{init: func(a *App, cx *gk.Cx) {
		text.blocks = overlay.Items(cx, a.blockRow)
		text.inlines = overlay.Items(cx, a.inlineRow)
	}, view: pluginView, step: func(a *App, cx *gk.Cx) {
		parsed, _ := gk.TestEditorText(cx)
		fmt.Printf("editor2 plugin parsed=%d blocks=%d inlines=%d\n", parsed, text.blockRows, text.inlineRows)
	}, script: "frame\nadvance 100\nframe\nframe\nframe\n" + stepScript}
	// tooltip proves hovering a Markdown link invokes the LinkTooltip row source
	// with the target URL ("https://example.com/docs").
	modes["tooltip"] = &mode{init: func(a *App, cx *gk.Cx) {
		text.tips = overlay.Items(cx, a.tipRow)
	}, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(420)).Child(editor.BaseTextViewMarkdown("md", tooltipMD).LinkTooltip(text.tips))
	}, step: func(a *App, cx *gk.Cx) {
		_, rows := gk.TestEditorText(cx)
		fmt.Printf("editor2 tooltip view_rows=%d described=%d\n", rows, text.tipRows)
	},

		script: "frame\nmove 30 66\nframe\nadvance 600\nframe\nframe\nmove 32 67\nframe\nframe\n" + stepScript}
	// surface proves BlockSurface("note", ...) renders a custom admonition container cleanly.
	modes["surface"] = &mode{view: func(a *App, cx *gk.Cx) gk.Element {
		ext := editor.NewMarkdownExtensionsDefault().BlockSurface("note",
			gk.Div(s.P2).Bg(theme.Secondary).Child(kit.Label("Note")))
		return gk.Div().W(gk.Px(420)).Child(editor.BaseTextViewMarkdown("md", surfaceMD).MarkdownExtensions(ext))
	}, script: "frame\nframe\n"}
}

func rowOf(rows map[string]int, key string) int {
	n, ok := rows[key]
	if !ok {
		n = len(rows)
		rows[key] = n
	}
	return n
}

func (a *App) blockRow(cx *gk.Cx, n gk.MarkdownNodeRead) gk.Element {
	text.blockRows++
	fmt.Printf("editor2 block row=%d name=%s text=%q markdown=%q\n", rowOf(text.rows, "b"+n.AsMarkdown(cx)), n.Name(cx), n.AsText(cx), n.AsMarkdown(cx))
	return kit.Label(fmt.Sprintf("[%s]", n.AsText(cx)))
}

func (a *App) inlineRow(cx *gk.Cx, node gk.MarkdownNodeRead, t editor.InlineRenderContext) gk.Element {
	text.inlineRows++
	fmt.Printf("editor2 inline row=%d name=%s text=%q font_ok=%v line_ok=%v rem_ok=%v\n", rowOf(text.rows, "i"+node.AsMarkdown(cx)), node.Name(cx),
		node.AsText(cx), t.FontSize(cx) > 0, t.LineHeight(cx) >= t.FontSize(cx), t.RemSize(cx) > 0)
	return kit.Label(fmt.Sprintf("<%s>", node.AsText(cx)))
}

func (a *App) tipRow(cx *gk.Cx, url string) gk.Element {
	text.tipRows++
	fmt.Printf("editor2 tooltip row=%d url=%s\n", rowOf(text.rows, url), url)
	return kit.Label(fmt.Sprintf("open %s", url))
}

func pluginView(a *App, cx *gk.Cx) gk.Element {
	ext := editor.NewMarkdownExtensionsDefault().Frontmatter().Plugin(editor.NewFrontmatterPlugin())
	return gk.Div().W(gk.Px(420)).Child(editor.BaseTextViewMarkdown("md", pluginMD).MarkdownExtensions(ext).
		Plugin(gk.MarkdownPlugin{Name: "badge", Match: gk.MarkdownMatchCode, Lang: "badge", Rows: text.blocks}).
		Plugin(gk.MarkdownPlugin{Name: "code", Match: gk.MarkdownMatchInlineCode, Rows: text.inlines, Baseline: 12}))
}
