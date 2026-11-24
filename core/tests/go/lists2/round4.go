// The lists and editor families' modes of docs/PROTOCOLS.md 9.16 to 9.19:
//
//	ctx-token   an input's token drawn by a row of the app's (an element closure over a context):
//	            the row reads the token's context, and is described again when it is selected
//	            (an item function over gk.InlineTokenContext, gk.Items, given to InputEl.Token)
//	lend        an editor's diagnostics lent (EditorState.DiagnosticsMut: lent), written
//	            (gk.NewDiagnostic, gk.LspPosition: libval), and given back (gk.TestLends)
//	highlight   an editor whose highlighter is the app's: the styles and folds functions asked
//	            between frames with the language, the text and a range (gk.NewHighlighter,
//	            gk.HighlightQuery, gk.HighlightRun, gk.FoldQuery, gk.TestAsks)
//	languages   the app's language service: an alias, and the syntax function asked while typing
//	            (gk.NewLanguageProvider, gk.LanguageAlias, gk.SyntaxQuery)
//	bad-lend    a lent object used after the turn that lent it (exit 2)
//	small       9.19: a patch's error as the kit's value, a tree's scroll handle as gpui's
//	            ScrollHandle (TreeState.ScrollHandle: uscroll; the tree's row source), the
//	            completion highlighters over an LSP item (gk.BaseHighlightPrefix, gk.CompletionItem)
package main

import (
	"fmt"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

var r4 struct {
	tokens                               gk.RowSource
	tokenRows                            int
	lent                                 gk.DiagnosticSet
	hl                                   gk.Highlighter
	stylesAsked, foldsAsked, syntaxAsked int
	prov                                 gk.RowSource
}

func init() {
	for _, m := range []string{"ctx-token", "lend", "highlight", "languages", "bad-lend", "small"} {
		modeInit[m] = r4Init
		modeStep[m] = r4Step
		modeView[m] = r4View
		modeScript[m] = r4Script
	}
}

// rng prints a range in the form the checks expect: Range{lo = 6, hi = 11}.
func rng(r gk.Range) string { return fmt.Sprintf("Range{lo = %d, hi = %d}", r.Lo, r.Hi) }

// Go: an item function is handed the token's context, not its row: the input has one token, row 0.
func (a *App) r4TokenRow(cx *gk.Cx, t gk.InlineTokenContext) gk.Element {
	r4.tokenRows++
	fmt.Printf("lists2 ctx token row=%d range=%s selected=%v disabled=%v readonly=%v label=%q height_ok=%v\n",
		0, rng(t.Range(cx)), t.IsSelected(cx), t.IsDisabled(cx), t.IsReadonly(cx), t.Token(cx).Label(cx),
		t.LineHeight(cx) > 0)
	return gk.InputToken(t)
}

func (a *App) r4Styles(cx *gk.Cx, q gk.HighlightQuery) []gk.HighlightRun {
	r4.stylesAsked++
	fmt.Printf("lists2 highlight lang=%s range=%s text=%q\n", q.Language, rng(q.Range), q.Text)
	var out []gk.HighlightRun
	if i := strings.Index(q.Text, "proc"); i >= 0 {
		out = append(out, gk.HighlightRun{Range: gk.Range{Lo: i, Hi: i + 4}, Name: "keyword"})
	}
	return out
}

func (a *App) r4Folds(cx *gk.Cx, q gk.FoldQuery) []gk.FoldRange {
	r4.foldsAsked++
	fmt.Printf("lists2 highlight folds lang=%s lines=%d\n", q.Language, strings.Count(q.Text, "\n"))
	return []gk.FoldRange{{StartLine: 0, EndLine: 2}}
}

func (a *App) r4Syntax(cx *gk.Cx, q gk.SyntaxQuery) gk.SyntaxContext {
	r4.syntaxAsked++
	fmt.Printf("lists2 syntax asked lang=%s offset=%d\n", q.Language, q.Offset)
	return gk.SyntaxContextString
}

func r4Init(a *App, cx *gk.Cx) {
	switch a.mode {
	case "ctx-token":
		a.in1 = gk.NewInputState(cx).DefaultValue("Hello world").Build()
		_ = a.in1.ReplaceRangeWithToken(cx, gk.Range{Lo: 6, Hi: 11}, gk.NewInlineToken("t1", "world").WithLabel("World"))
		r4.tokens = gk.Items(cx, a.r4TokenRow)
	case "lend", "bad-lend":
		a.ed = gk.NewEditorState(cx).Language("demo").DefaultValue("main :: proc() {}\n").Build()
	case "highlight":
		r4.hl = gk.NewHighlighter(cx, a.r4Styles, a.r4Folds, "demo")
		a.ed = gk.NewEditorState(cx).Language("demo").DefaultValue("main :: proc() {\n\n}\n").Build()
		a.ed.SetHighlighterFactory(cx, r4.hl.Src)
	case "languages":
		r4.prov = gk.NewLanguageProvider(cx, []gk.LanguageAlias{{Alias: "od", Name: "demo"}}, a.r4Syntax)
		gk.BaseSetLanguageProvider(cx, r4.prov)
		a.ed = gk.NewEditorState(cx).Language("demo").DefaultValue("x := \n").Build()
	case "small":
		a.tree = gk.NewTreeState(cx).Build()
		gk.SetTreeItems(cx, a.tree, treeNodes(300))
		a.treeRows = gk.NewTreeSource(cx, a.driveTreeRow, gk.Initial(8))
	}
}

func r4View(a *App, cx *gk.Cx) gk.Element {
	switch a.mode {
	case "ctx-token":
		return gk.Div().W(gk.Px(260)).Child(gk.Input(a.in1).Token(r4.tokens))
	case "lend", "bad-lend", "highlight", "languages":
		return gk.Div().W(gk.Px(400)).Child(gk.CodeEditor(a.ed, gk.CodeEditorOptions{Height: 120}))
	case "small":
		return gk.Div().W(gk.Px(120)).H(gk.Px(200)).Child(gk.Tree(a.tree, a.treeRows, s.SizeFull))
	}
	return gk.Label("-")
}

func r4Step(a *App, cx *gk.Cx) {
	switch a.mode {
	case "ctx-token":
		a.in1.SetSelectedRange(cx, gk.Range{Lo: 0, Hi: 11}) // the token is selected now: its row is described again
	case "lend", "bad-lend":
		switch a.step {
		case 1:
			d, ok := a.ed.DiagnosticsMut(cx).Get()
			fmt.Printf("lists2 lend got=%v empty=%v\n", ok, ok && d.IsEmpty(cx))
			if ok {
				d.Push(cx, gk.NewDiagnostic([2]gk.LspPosition{{Line: 0, Character: 0}, {Line: 0, Character: 4}}, "unused"))
				d.Push(cx, gk.NewDiagnostic([2]gk.LspPosition{{Line: 0, Character: 8}, {Line: 0, Character: 12}}, "twice"))
				r4.lent = d
			}
		case 2:
			if a.mode == "bad-lend" {
				fmt.Printf("lists2 bad-lend using the lent handle\n")
				_ = r4.lent.Len(cx) // given back at the end of the turn that lent it: stops
				return
			}
			n := -1
			if got, ok := a.ed.Diagnostics(cx).Get(); ok {
				n = got.Len(cx)
			}
			lent, back := gk.TestLends(cx)
			fmt.Printf("lists2 lend after len=%d lent=%d given_back=%d\n", n, lent, back)
		}
	case "highlight":
		gets, asked, answered := gk.TestAsks(cx)
		fmt.Printf("lists2 highlight counts styles=%v folds=%v asked=%d answered=%d gets_more=%v\n",
			r4.stylesAsked > 0, r4.foldsAsked > 0, asked, answered, gets > asked)
	case "languages":
		fmt.Printf("lists2 languages syntax_asked=%v\n", r4.syntaxAsked > 0)
	case "small":
		switch a.step {
		case 1:
			e, ok := gk.DiffFileParseError(cx, "@@ nonsense\n").Get()
			_, none := gk.DiffFileParseError(cx, diffPatch).Get()
			line, message := -1, ""
			if ok {
				line, message = e.Line(cx), e.Message(cx)
			}
			fmt.Printf("lists2 small parse-error ok=%v line=%d message=%q good_has_error=%v\n", ok, line, message, none)
			ranges := gk.BaseHighlightPrefix(cx, "ma", gk.CompletionItem{Label: "main"})
			rs := make([]string, len(ranges))
			for i, r := range ranges {
				rs[i] = rng(r)
			}
			fmt.Printf("lists2 small highlight_prefix=[%s]\n", strings.Join(rs, ", "))
			sh := a.tree.ScrollHandle(cx)
			fmt.Printf("lists2 small scroll before y=%v\n", sh.Offset(cx).Y)
			sh.SetOffset(cx, gk.GpuiPoint{X: 0, Y: -4000})
		case 2:
			far := false
			for row := 100; row < 300; row++ {
				if a.drawnRow("tree", row) {
					far = true
				}
			}
			fmt.Printf("lists2 small scroll after y=%v drawn_far=%v\n", a.tree.ScrollHandle(cx).Offset(cx).Y, far)
		}
	}
}

func r4Script(a *App, sb *strings.Builder) {
	switch a.mode {
	case "ctx-token":
		sb.WriteString("frame\nframe\nframe\n")
		sb.WriteString(stepClick)
		sb.WriteString("frame\nframe\nframe\n")
	case "lend", "bad-lend":
		sb.WriteString("frame\n")
		sb.WriteString(stepClick)
		sb.WriteString("frame\nframe\n")
		sb.WriteString(stepClick)
	case "highlight":
		sb.WriteString("frame\nadvance 100\nframe\nadvance 100\nframe\nframe\n")
		sb.WriteString(stepClick)
	case "languages":
		// The caret after "x := ", then a quote: the editor asks where in the syntax it is.
		sb.WriteString("click 300 80\nframe\nkey end\ntype \"\nframe\nadvance 100\nframe\ntype \"\nframe\nframe\n")
		sb.WriteString(stepClick)
	case "small":
		sb.WriteString("frame\n")
		sb.WriteString(stepClick)
		sb.WriteString("frame\nframe\nframe\n")
		sb.WriteString(stepClick)
	}
}
