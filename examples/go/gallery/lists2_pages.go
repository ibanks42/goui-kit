// The lists and editor families' gallery pages, the Go twin of ref/src/bin/gallery/lists2_pages.rs
// (and of examples/odin/gallery/lists2_pages.odin), call for call: selects and a combobox over
// the library's search delegate (items as data, rows and group headers as row views) and a
// table whose looks are data (docs/PROTOCOLS.md sections 9.9 and 9.10), a Markdown code block
// highlighted by the app (an ask, 9.14), a patch parsed by the kit (DiffFile::parse, 9.15), an
// input's token drawn by an item function (9.16), a diff's header suffixes, and an editor whose
// diagnostics are lent and whose highlighter is the app's (9.17, 9.18). The steps of
// ref/gallery/pages.txt open the menus, type and press keys.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/theme"
)

type lists2Pages struct {
	select_ gk.SelectState
	search  gk.SelectState
	rows    gk.SelectState
	combo   gk.ComboboxState
	table   gk.TableState
	cells   gk.RowSource
	values  gk.View // the "Values" group header cell
	// docs/PROTOCOLS.md 9.14: a code block highlighter the app answers between frames
	hl   gk.Ask[gk.CodeBlock, []gk.TupleRangeHighlightStyle]
	diff gk.DiffState // a patch parsed by DiffFile::parse (PROTOCOLS.md 9.15)
	// docs/PROTOCOLS.md 9.16 to 9.18
	token    gk.InputState  // an input with a token drawn by the app's item function (9.16)
	editor   gk.EditorState // diagnostics lent (9.17), a highlighter of the app's (9.18)
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

// lists2HeaderRow is the row a search source's header row is asked at (Odin's gk.HEADER).
const lists2HeaderRow = 0xffff_fffe

func lists2PagesInit(a *App, cx *gk.Cx) {
	p := &a.lists2
	p.hl = gk.NewAsk(cx, textHighlightSpans)
	// A patch parsed by the kit: how many files it holds, then each file by its index.
	n, _ := gk.DiffFileParseCount(cx, diffViewPatch).Value.Get()
	files := make([]gk.DiffFile, n)
	for i := range files {
		files[i] = gk.NewDiffFileParseAt(diffViewPatch, i)
	}
	p.diff = gk.NewDiffState(cx, files).Build()
	// a token drawn by an item function of the app's (9.16), a diff's header suffixes, an editor
	// whose diagnostics are lent and given back (9.17) and whose highlighter is the app's (9.18).
	p.token = gk.NewInputState(cx).DefaultValue("Hello world").Build()
	_ = p.token.ReplaceRangeWithToken(cx, gk.Range{Lo: 6, Hi: 11}, gk.NewInlineToken("t1", "world").WithLabel("World"))
	p.keywords = gk.NewHighlighter(cx, editorKeywords, nil)
	p.editor = gk.NewEditorState(cx).Language("odin").DefaultValue(editorDiagText).Build()
	p.editor.SetHighlighterFactory(cx, p.keywords.Src)
	if d, ok := p.editor.DiagnosticsMut(cx).Get(); ok {
		d.Push(cx, gk.NewDiagnostic([2]gk.LspPosition{{Line: 1, Character: 4}, {Line: 1, Character: 10}}, "never called"))
	}
	src := gk.NewSearchSource(cx, lists2Fruit)
	p.select_ = gk.NewSelectState(cx, src, gk.Some(gk.Row(4))).Build()
	groups := []gk.SearchGroup{{Title: "Fruit", Items: lists2Fruit}, {Title: "Veg", Items: lists2Veg}}
	p.search = gk.NewSelectState(cx, gk.NewSearchSourceGroups(cx, groups), gk.NoRow).Searchable(true).Build()
	p.rows = gk.NewSelectState(cx, gk.NewSearchSourceGroups(cx, groups, gk.SearchRow(lists2Row), gk.SearchHeaders()), gk.NoRow).Build()
	p.combo = gk.NewComboboxState(cx, gk.NewSearchSource(cx, lists2Fruit), []gk.BaseIndexPath{gk.Row(1)}).Multiple(true).Build()
	col := func(key, name string, w float32) gk.TableColumn {
		c := gk.NewTableColumn(key, name)
		c.Width = w
		return c
	}
	columns := []gk.TableColumn{col("id", "Id", 60), col("name", "Name", 100), col("a", "A", 60)}
	p.cells = gk.NewTableSource(cx, 6, columns, func(cx *gk.Cx, at gk.CellAt) gk.Element {
		return gk.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
	})
	p.table = gk.NewTableState(cx, p.cells).Build()
	p.values = gk.NewView(nil, func(cx *gk.Cx) gk.Element { return gk.Label("[values]") })
	level := []gk.TableGroup{{Label: "Key", Span: 1}, {Label: "Values", Span: 2}}
	gk.SetTableLook(cx, p.table, gk.TableLook{
		Groups:      [][]gk.TableGroup{level},
		GroupViews:  []gk.TableGroupView{{Label: "Values", View: p.values}},
		HeaderStyle: gk.Some(gk.NewStyleRefinementDefault().Bg(theme.Secondary)),
		RowStyle:    gk.Some(gk.NewStyleRefinementDefault().TextColor(theme.MutedForeground)),
	})
}

func lists2Row(cx *gk.Cx, at gk.RowAt) gk.Element {
	if at.Row == lists2HeaderRow {
		if at.Section == 0 {
			return gk.Label("== fruit ==")
		}
		return gk.Label("== veg ==")
	}
	items := lists2Veg
	if at.Section == 0 {
		items = lists2Fruit
	}
	return gk.Label(fmt.Sprintf("> %s", items[at.Row].Title))
}

func lists2Select(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(220)).Child(gk.Select(a.lists2.select_).Placeholder("Fruit"))
}

func lists2SelectSearch(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(220)).Child(gk.Select(a.lists2.search).Placeholder("Food").SearchPlaceholder("Find"))
}

func lists2SelectRows(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(220)).Child(gk.Select(a.lists2.rows).Placeholder("Food"))
}

func lists2Combobox(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(260)).Child(gk.Combobox(a.lists2.combo).Placeholder("Fruit"))
}

func lists2TableStatic(a *App, cx *gk.Cx) gk.Element {
	fruit := []string{"Apple", "Banana", "Cherry"}
	rows := make([]gk.ChildElementNode, len(fruit))
	for n, f := range fruit {
		rows[n] = gk.TableRow().Child(gk.TableCell().Child(gk.Text(f))).Child(gk.TableCell().Child(gk.Text(fmt.Sprintf("%d", n*3))))
	}
	return gk.Div().W(gk.Px(360)).Child(
		gk.Table().
			Child(gk.TableHeader().Child(gk.TableRow().Child(gk.TableHead().Child(gk.Text("Fruit"))).Child(gk.TableHead().Child(gk.Text("Count"))))).
			Child(gk.TableBody().Children(rows)))
}

func lists2TableLook(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(440)).H(gk.Px(240)).Child(gk.DataTable(a.lists2.table).ScrollbarVisible(false, false))
}

// text-highlight: a Markdown code block whose highlights the app gives (an ask, PROTOCOLS.md
// 9.14): "proc" and "return" in red, as the Rust page's closure does.
var textHighlightWords = []string{"proc", "return"}

const textHighlightMD = "# Code\n\n```odin\nmain :: proc() {\n    return\n}\n```\n"

// keywordRuns finds the keywords of textHighlightWords in text: one range per keyword.
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
	return gk.Div().W(gk.Px(360)).Child(gk.BaseTextViewMarkdown("hl", textHighlightMD).CodeBlockHighlighter(a.lists2.hl))
}

// diff-view: a patch parsed by the kit (DiffFile::parse) shown by its Diff.
const diffViewPatch = "diff --git a/hello.odin b/hello.odin\n--- a/hello.odin\n+++ b/hello.odin\n@@ -1,4 +1,4 @@\n package hello\n \n-main :: proc() {}\n+main :: proc() { run() }\n // end\n"

func diffView(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(440)).H(gk.Px(200)).Child(gk.Diff(a.lists2.diff))
}

// text-token: the input's token drawn by an item function of the app's: the kit's InputToken over
// the token's context, which the kit hands it (9.16, given with gk.Items since 9.41).
func textToken(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(260)).Child(gk.Input(a.lists2.token).Token(gk.Items(cx, func(cx *gk.Cx, t gk.InlineTokenContext) gk.Element {
		return gk.InputToken(t)
	})))
}

// diff-header: each file's header ends with its path in brackets, an item function over the file.
func diffHeader(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(440)).H(gk.Px(200)).Child(gk.Diff(a.lists2.diff).RenderHeaderSuffix(gk.Items(cx, func(cx *gk.Cx, f gk.DiffFileRead) gk.Element {
		return gk.Label(fmt.Sprintf("[%s]", f.Path(cx)))
	})))
}

// editor-diagnostics: "proc" and "return" are keyword runs (the editor's resolver gives their
// color), as the Rust page's highlighter answers.
const editorDiagText = "main :: proc() {\n    helper()\n    return\n}\n"

func editorKeywords(cx *gk.Cx, q gk.HighlightQuery) []gk.HighlightRun {
	var out []gk.HighlightRun
	for _, r := range keywordRuns(q.Text) {
		out = append(out, gk.HighlightRun{Range: r, Name: "keyword"})
	}
	return out
}

func editorDiagnostics(a *App, cx *gk.Cx) gk.Element {
	return gk.Div().W(gk.Px(480)).Child(gk.CodeEditor(a.lists2.editor, gk.CodeEditorOptions{Height: 160}))
}
