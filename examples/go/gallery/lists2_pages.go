// Searchable lists, tables, text highlighting, and diff gallery pages: Select, Combobox,
// custom table styling and static tables, Markdown code-block highlighters, inline tokens, and diffs.
package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
	gkeditor "github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	gktable "github.com/ibanks42/goui-kit/gk/table"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type lists2Pages struct {
	select_  gktable.SelectState
	search   gktable.SelectState
	rows     gktable.SelectState
	combo    gktable.ComboboxState
	table    gktable.State
	cells    gk.RowSource
	values   gk.View
	hl       gk.Ask[gk.CodeBlock, []gk.TupleRangeHighlightStyle]
	diff     gkeditor.DiffState
	token    gkeditor.InputState
	editor   gkeditor.State
	keywords gk.Highlighter
}

var lists2PageTable = []Page{
	{"lists2-select", lists2Select},
	{"lists2-select-search", lists2SelectSearch},
	{"lists2-select-rows", lists2SelectRows},
	{"lists2-combobox", lists2Combobox},
	{"lists2-table-look", lists2TableLook},
	{"lists2-table-static", lists2TableStatic},
	{"text-highlight", textHighlight},
	{"diff-view", diffView},
	{"text-token", textToken},
	{"diff-header", diffHeader},
	{"editor-diagnostics", editorDiagnostics},
}

var lists2Fruit = []gk.SearchItem{
	{Value: "apple", Title: "Apple"},
	{Value: "banana", Title: "Banana"},
	{Value: "cherry", Title: "Cherry"},
	{Value: "grape", Title: "Grape", Disabled: true},
	{Value: "mango", Title: "Mango", Display: gk.Some("Mango (ripe)")},
}

var lists2Veg = []gk.SearchItem{{Value: "kale", Title: "Kale"}, {Value: "leek", Title: "Leek"}}

const lists2HeaderRow = 0xffff_fffe

func lists2PagesInit(a *App, cx *gk.Cx) {
	p := &a.lists2
	p.hl = gk.NewAsk(cx, textHighlightSpans)
	n, _ := gkeditor.DiffFileParseCount(cx, diffViewPatch).Value.Get()
	files := make([]gkeditor.DiffFile, n)
	for i := range files {
		files[i] = gkeditor.NewDiffFileParseAt(diffViewPatch, i)
	}
	p.diff = gkeditor.NewDiffState(cx, files).Build()
	p.token = gkeditor.NewInputState(cx).DefaultValue("Hello world").Build()
	_ = p.token.ReplaceRangeWithToken(cx, gk.Range{Lo: 6, Hi: 11}, gkeditor.NewInlineToken("t1", "world").WithLabel("World"))
	p.keywords = gk.NewHighlighter(cx, editorKeywords, nil)
	p.editor = gkeditor.NewState(cx).Language("demo").DefaultValue(editorDiagText).Build()
	p.editor.SetHighlighterFactory(cx, p.keywords.Src)
	if d, ok := p.editor.DiagnosticsMut(cx).Get(); ok {
		d.Push(cx, gkeditor.NewDiagnostic([2]gk.LspPosition{{Line: 1, Character: 4}, {Line: 1, Character: 10}}, "never called"))
	}
	src := gk.NewSearchSource(cx, lists2Fruit)
	p.select_ = gktable.NewSelectState(cx, src, gk.Some(gktable.Row(4))).Build()
	groups := []gk.SearchGroup{{Title: "Fruit", Items: lists2Fruit}, {Title: "Veg", Items: lists2Veg}}
	p.search = gktable.NewSelectState(cx, gk.NewSearchSourceGroups(cx, groups), gktable.NoRow).Searchable(true).Build()
	p.rows = gktable.NewSelectState(cx, gk.NewSearchSourceGroups(cx, groups, gk.SearchRow(lists2Row), gk.SearchHeaders()), gktable.NoRow).Build()
	p.combo = gktable.NewComboboxState(cx, gk.NewSearchSource(cx, lists2Fruit), []gktable.BaseIndexPath{gktable.Row(1)}).Multiple(true).Build()
	col := func(key, name string, w float32) gktable.TableColumn {
		c := gktable.NewTableColumn(key, name)
		c.Width = w
		return c
	}
	columns := []gktable.TableColumn{col("id", "Id", 60), col("name", "Name", 100), col("a", "A", 60)}
	p.cells = gktable.NewSource(cx, 6, columns, func(cx *gk.Cx, at gktable.CellAt) gk.Element {
		return kit.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
	})
	p.table = gktable.NewState(cx, p.cells).Build()
	p.values = gk.NewView(nil, func(cx *gk.Cx) gk.Element { return kit.Label("[values]") })
	level := []gktable.Group{{Label: "Key", Span: 1}, {Label: "Values", Span: 2}}
	gktable.SetTableLook(cx, p.table, gktable.Look{
		Groups:      [][]gktable.Group{level},
		GroupViews:  []gktable.GroupView{{Label: "Values", View: p.values}},
		HeaderStyle: gk.Some(gk.NewStyleRefinementDefault().Bg(theme.Secondary)),
		RowStyle:    gk.Some(gk.NewStyleRefinementDefault().TextColor(theme.MutedForeground)),
	})
}

func lists2Row(cx *gk.Cx, at gk.RowAt) gk.Element {
	if at.Row == lists2HeaderRow {
		if at.Section == 0 {
			return kit.Label("== fruit ==")
		}
		return kit.Label("== veg ==")
	}
	items := lists2Veg
	if at.Section == 0 {
		items = lists2Fruit
	}
	return kit.Label(fmt.Sprintf("> %s", items[at.Row].Title))
}

func lists2Select(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(220)).Child(gktable.Select(a.lists2.select_).Placeholder("Fruit"))
}

func lists2SelectSearch(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(220)).Child(gktable.Select(a.lists2.search).Placeholder("Food").SearchPlaceholder("Find"))
}

func lists2SelectRows(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(220)).Child(gktable.Select(a.lists2.rows).Placeholder("Food"))
}

func lists2Combobox(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(260)).Child(gktable.Combobox(a.lists2.combo).Placeholder("Fruit"))
}

func lists2TableStatic(a *App, cx *gk.Cx) gk.Element {
	fruit := []string{"Apple", "Banana", "Cherry"}
	rows := make([]gk.ChildElementNode, len(fruit))
	for n, f := range fruit {
		rows[n] = gktable.TableRow().Child(gktable.Cell().Child(gk.Text(f))).Child(gktable.Cell().Child(gk.Text(fmt.Sprintf("%d", n*3))))
	}
	return gk.Div().W(gk.Px(360)).Child(
		gktable.Table().
			Child(gktable.Header().Child(gktable.TableRow().Child(gktable.Head().Child(gk.Text("Fruit"))).Child(gktable.Head().Child(gk.Text("Count"))))).
			Child(gktable.Body().Children(rows)))
}

func lists2TableLook(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(440)).H(gk.Px(240)).Child(gktable.DataTable(a.lists2.table).ScrollbarVisible(false, false))
}

var textHighlightWords = []string{"proc", "return"}

const textHighlightMD = "# Code\n\n```demo\nmain :: proc() {\n    return\n}\n```\n"

func keywordRuns(text string) []gk.Range {
	var out []gk.Range
	for i := 0; i < len(text); {
		n := 0
		for _, kw := range textHighlightWords {
			if len(text)-i >= len(kw) && text[i:i+len(kw)] == kw {
				n = len(kw)
			}
		}
		if n > 0 {
			out = append(out, gk.Range{Lo: i, Hi: i + n})
			i += n
		} else {
			i++
		}
	}
	return out
}

func textHighlightSpans(cx *gk.Cx, b gk.CodeBlock) []gk.TupleRangeHighlightStyle {
	var out []gk.TupleRangeHighlightStyle
	for _, r := range keywordRuns(b.Code) {
		out = append(out, gk.TupleRangeHighlightStyle{V0: r, V1: gk.HighlightStyle{Color: gk.Some(gk.HSLA(0, 0.7, 0.5, 1))}})
	}
	return out
}

func textHighlight(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(360)).Child(gkeditor.BaseTextViewMarkdown("hl", textHighlightMD).CodeBlockHighlighter(a.lists2.hl))
}

const diffViewPatch = "diff --git a/hello.demo b/hello.demo\n--- a/hello.demo\n+++ b/hello.demo\n@@ -1,4 +1,4 @@\n package hello\n \n-main :: proc() {}\n+main :: proc() { run() }\n // end\n"

func diffView(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(440)).H(gk.Px(200)).Child(gkeditor.Diff(a.lists2.diff))
}

func textToken(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(260)).Child(gkeditor.Input(a.lists2.token).Token(overlay.Items(cx, func(cx *gk.Cx, t gkeditor.InlineTokenContext) gk.Element {
		return gkeditor.InputToken(t)
	})))
}

func diffHeader(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(440)).H(gk.Px(200)).Child(gkeditor.Diff(a.lists2.diff).RenderHeaderSuffix(overlay.Items(cx, func(cx *gk.Cx, f gkeditor.DiffFileRead) gk.Element {
		return kit.Label(fmt.Sprintf("[%s]", f.Path(cx)))
	})))
}

const editorDiagText = "main :: proc() {\n    helper()\n    return\n}\n"

func editorKeywords(cx *gk.Cx, q gk.HighlightQuery) []gk.HighlightRun {
	var out []gk.HighlightRun
	for _, r := range keywordRuns(q.Text) {
		out = append(out, gk.HighlightRun{Range: r, Name: "keyword"})
	}
	return out
}

func editorDiagnostics(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(480)).Child(gkeditor.CodeEditor(a.lists2.editor, gkeditor.CodeEditorOptions{Height: 160}))
}
