package main

import (
	"fmt"
	"strings"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/theme"
)

// ed holds handles shared by editor-specific test modes (folds, highlighter, styles).
var ed struct {
	folds    gk.RowSource
	foldRows int
	hl       gk.Highlighter
	styles   gk.Ask[string, gk.Option[gk.HighlightStyle]]
}

func init() {
	// history proves History.Retain filters pushed entries ["a","b","c","d"] down
	// to ["b","d"] while preserving order and current selection.
	modes["history"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.hist = gk.NewHistory(cx).Build()
		for _, e := range []string{"a", "b", "c", "d"} {
			a.hist.Push(cx, e)
		}
	}, step: func(a *App, cx *gk.Cx) {
		a.hist.Retain(cx, []string{"b", "d", "x"})
		es := a.hist.Entries(cx)
		cur, _ := a.hist.Current(cx).Get()
		fmt.Printf("editor2 history entries=%s current=%q\n", stringsText(es), cur)
	}, script: "frame\n" + stepScript}
	modes["conflict"] = &mode{step: conflictStep, script: "frame\n" + stepScript}
	modes["fold"] = &mode{init: foldInit, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(420)).Child(editor.CodeEditor(a.ed, editor.CodeEditorOptions{Height: 120}))
	}, step: func(a *App, cx *gk.Cx) {
		fmt.Printf("editor2 fold described=%d\n", ed.foldRows)
	}, script: foldScript()}
	// owner proves SemanticOwner accepts an editor handle via gk.EntityIDOf and renders cleanly.
	modes["owner"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = editor.NewState(cx).DefaultValue("x").Build()
	}, view: func(a *App, cx *gk.Cx) gk.Element {
		body := gk.Div().W(gk.Px(220)).Child(editor.InputBase("ib").SemanticOwner(gk.EntityIDOf(a.ed)).AccessibilityLabel("owned"))
		fmt.Printf("editor2 owner drawn\n")
		return body
	}, script: "frame\nframe\n"}
	// bad-owner proves passing a non-text handle (History) to SemanticOwner fails cleanly.
	modes["bad-owner"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.hist = gk.NewHistory(cx).Build()
	}, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(220)).Child(editor.InputBase("ib").SemanticOwner(gk.EntityID(gk.Handle(a.hist))))
	}, script: "frame\nframe\n"}
	modes["editor-style"] = &mode{init: styleInit, view: editorBox, script: "frame\nadvance 100\nframe\nadvance 100\nframe\nframe\n"}
}

// conflictStep proves ParseConflicts extracts 1 conflict block with "ours"/"theirs"
// labels and no diff3 base section (base=false, base_label=false).
func conflictStep(a *App, cx *gk.Cx) {
	f := editor.ParseConflicts(cx, "a.txt", conflictText)
	file, ok := f.Value.Get()
	if !ok {
		fmt.Printf("editor2 conflict parse failed: %s\n", f.Err)
		return
	}
	cs := file.Conflicts(cx)
	fmt.Printf("editor2 conflict n=%d\n", len(cs))
	for _, c := range cs {
		base, hasBase := c.BaseLines(cx).Get()
		bl, hasBl := c.BaseLabel(cx).Get()
		fmt.Printf("editor2 conflict lines=%v current=%v incoming=%v base=%v/%v current_label=%q incoming_label=%q base_label=%v/%q\n",
			c.Lines(cx), c.CurrentLines(cx), c.IncomingLines(cx), hasBase, base, c.CurrentLabel(cx),
			c.IncomingLabel(cx), hasBl, bl)
	}
}

func (a *App) foldRow(cx *gk.Cx, line int) gk.Element {
	ed.foldRows++
	fmt.Printf("editor2 fold row=%d line=%d\n", line, line)
	return kit.Label("...")
}

// foldInit configures line folding with a fold candidate on lines 0..2 so
// clicking the fold gutter proves SetFoldPlaceholderRenderer invokes foldRow
// with line=0.
func foldInit(a *App, cx *gk.Cx) {
	ed.folds = overlay.Items(cx, a.foldRow)
	a.ed = editor.NewState(cx).DefaultValue(foldText).Build()
	a.ed.SetFolding(cx, true)
	a.ed.SetLineNumber(cx, true)
	a.ed.SetFoldPlaceholderRenderer(cx, gk.Some(ed.folds))
	a.ed.ApplyHighlighterFoldCandidates(cx, []gk.FoldRange{{StartLine: 0, EndLine: 2}})
}

func foldScript() string {
	var b strings.Builder
	b.WriteString("frame\n")
	for _, y := range []int{68, 74} {
		for x := 16; x <= 96; x += 3 {
			fmt.Fprintf(&b, "move %d %d\nclick %d %d\nframe\n", x, y, x, y)
		}
	}
	b.WriteString(stepScript)
	return b.String()
}

func (a *App) keywordRuns(cx *gk.Cx, q gk.HighlightQuery) []gk.HighlightRun {
	var out []gk.HighlightRun
	if i := strings.Index(q.Text, "proc"); i >= 0 {
		out = append(out, gk.HighlightRun{Range: gk.Range{Lo: i, Hi: i + 4}, Name: "keyword"})
	}
	return out
}

func (a *App) styleFor(cx *gk.Cx, name string) gk.Option[gk.HighlightStyle] {
	fmt.Printf("editor2 style asked name=%s\n", name)
	if name == "keyword" {
		return gk.Some(gk.HighlightStyle{Color: gk.Some(theme.Danger)})
	}
	return gk.Option[gk.HighlightStyle]{}
}

// styleInit pairs a custom Highlighter emitting "keyword" spans with an Ask
// style map in SetEditorStyle, proving the editor queries styleFor("keyword").
func styleInit(a *App, cx *gk.Cx) {
	ed.styles = gk.NewAsk(cx, a.styleFor)
	ed.hl = gk.NewHighlighter(cx, a.keywordRuns, nil, "demo")
	a.ed = editor.NewState(cx).Language("demo").DefaultValue("main :: proc() {}\n").Build()
	a.ed.SetHighlighterFactory(cx, ed.hl.Src)
	a.ed.SetEditorStyle(cx, editor.InputEditorStyle{
		Foreground: theme.Foreground, MutedForeground: theme.MutedForeground,
		Background: theme.Background, Border: theme.Border, Selection: theme.Selection,
		Caret: theme.Caret, Diagnostics: editor.DiagnosticColors{Error: theme.Danger, Warning: theme.Warning, Info: theme.Info, Hint: theme.Info},
		HighlightStyles: ed.styles,
	})
}
