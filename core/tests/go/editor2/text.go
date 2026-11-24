// plugin, tooltip and surface: the Markdown text bridges: data plugins
// (gk.MarkdownPlugin) whose rows are item functions (gk.Items over the node, and over the node and
// the inherited typography inline), a link's tooltip, a block surface, and the library's counts
// (gk.TestEditorText).
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

const (
	pluginMD  = "---\ntitle: Notes\nowner: me\n---\n\n# Plugins\n\n```badge\nready\n```\n\nSome `inline` text.\n"
	tooltipMD = "[docs](https://example.com/docs) and more"
	surfaceMD = "> [!note]\n> Inside the surface.\n\nAfter it.\n"
)

// The rows of the plugin and tooltip modes.
var text struct {
	blocks, inlines, tips gk.RowSource
	blockRows, inlineRows int
	tipRows               int
	rows                  map[string]int // the rows described, numbered by first description
}

func init() {
	text.rows = map[string]int{}
	modes["plugin"] = &mode{init: func(a *App, cx *gk.Cx) {
		text.blocks = gk.Items(cx, a.blockRow)
		text.inlines = gk.Items(cx, a.inlineRow)
	}, view: pluginView, step: func(a *App, cx *gk.Cx) {
		parsed, _ := gk.TestEditorText(cx)
		fmt.Printf("editor2 plugin parsed=%d blocks=%d inlines=%d\n", parsed, text.blockRows, text.inlineRows)
	}, script: "frame\nadvance 100\nframe\nframe\nframe\n" + stepScript}
	modes["tooltip"] = &mode{init: func(a *App, cx *gk.Cx) {
		text.tips = gk.Items(cx, a.tipRow)
	}, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(420)).Child(gk.BaseTextViewMarkdown("md", tooltipMD).LinkTooltip(text.tips))
	}, step: func(a *App, cx *gk.Cx) {
		_, rows := gk.TestEditorText(cx)
		fmt.Printf("editor2 tooltip view_rows=%d described=%d\n", rows, text.tipRows)
	},
		// On the link "docs" (the first word of the text), then over it again: one row.
		script: "frame\nmove 30 66\nframe\nadvance 600\nframe\nframe\nmove 32 67\nframe\nframe\n" + stepScript}
	modes["surface"] = &mode{view: func(a *App, cx *gk.Cx) gk.Element {
		ext := gk.NewMarkdownExtensionsDefault().BlockSurface("note",
			gk.Div(s.P2).Bg(theme.Secondary).Child(gk.Label("Note")))
		return gk.Div().W(gk.Px(420)).Child(gk.BaseTextViewMarkdown("md", surfaceMD).MarkdownExtensions(ext))
	}, script: "frame\nframe\n"}
}

// Go: an item function is handed its values, not its row: the rows are numbered here in the order
// they are first described.
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
	return gk.Label(fmt.Sprintf("[%s]", n.AsText(cx)))
}

func (a *App) inlineRow(cx *gk.Cx, node gk.MarkdownNodeRead, t gk.InlineRenderContext) gk.Element {
	text.inlineRows++
	fmt.Printf("editor2 inline row=%d name=%s text=%q font_ok=%v line_ok=%v rem_ok=%v\n", rowOf(text.rows, "i"+node.AsMarkdown(cx)), node.Name(cx),
		node.AsText(cx), t.FontSize(cx) > 0, t.LineHeight(cx) >= t.FontSize(cx), t.RemSize(cx) > 0)
	return gk.Label(fmt.Sprintf("<%s>", node.AsText(cx)))
}

func (a *App) tipRow(cx *gk.Cx, url string) gk.Element {
	text.tipRows++
	fmt.Printf("editor2 tooltip row=%d url=%s\n", rowOf(text.rows, url), url)
	return gk.Label(fmt.Sprintf("open %s", url))
}

func pluginView(a *App, cx *gk.Cx) gk.Element {
	ext := gk.NewMarkdownExtensionsDefault().Frontmatter().Plugin(gk.NewFrontmatterPlugin())
	return gk.Div().W(gk.Px(420)).Child(gk.BaseTextViewMarkdown("md", pluginMD).MarkdownExtensions(ext).
		Plugin(gk.MarkdownPlugin{Name: "badge", Match: gk.MarkdownMatchCode, Lang: "badge", Rows: text.blocks}).
		Plugin(gk.MarkdownPlugin{Name: "code", Match: gk.MarkdownMatchInlineCode, Rows: text.inlines, Baseline: 12}))
}
