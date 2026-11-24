// The lists2 modes of docs/PROTOCOLS.md 9.9 to 9.15 and 11.25 that need more than main.go's names:
//
//	combobox       ComboboxState.Selection
//	will-change    gk.SearchWillChange, gk.SearchRules
//	table, drive   the table's and tree's row sources, the table's texts
//	               and looks (gk.SetTableTexts, gk.SetTableRowLabels, gk.SetTableLook)
//	expr, bad-expr InputStateBuilder.StepBy, NumberStepByValue's expression
//	diff           gk.DiffFileParseCount's gk.Fallible[int]
//	display-map    gk.Font, gk.NewDisplayMap, DisplayMap.SetFont
//	providers      the editor's providers on gk.CodeEditor
//	kept           gk.TestKeptCount
//	ask            gk.Ask, gk.NewAsk, gk.CodeBlock, gk.TestAsks
package main

import (
	"fmt"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/table"
	"gpuikit.dev/gk/theme"
)

func init() {
	for _, m := range []string{"combobox", "will-change", "table", "expr", "bad-expr", "diff", "display-map", "providers", "kept", "ask", "drive"} {
		modeInit[m] = pInit
	}
	for _, m := range []string{"combobox", "will-change", "table", "expr", "diff", "display-map", "kept", "kept-gone", "ask", "drive"} {
		modeStep[m] = pStep
	}
	modeView["ask"] = func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(400)).Child(
			editor.BaseTextViewMarkdown("md", "```demo\nproc x\n```\n\n![pic](pic.png)\n").
				CodeBlockHighlighter(askState.hl).ImageSource(askState.img))
	}
	modeView["providers"] = func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(400)).Child(editor.CodeEditor(a.ed, editor.CodeEditorOptions{Height: 120}))
	}
}

// ask: data closures answered between frames.
var askState struct {
	hl       gk.Ask[gk.CodeBlock, []gk.TupleRangeHighlightStyle]
	img      gk.Ask[string, string]
	hlCalls  int
	imgCalls int
}

var columns = []table.TableColumn{
	{Key: "id", Name: "Id", Width: 60, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "name", Name: "Name", Width: 100, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
	{Key: "a", Name: "A", Width: 60, Resizable: true, Movable: true, Selectable: true, MinWidth: 20, MaxWidth: 400},
}

const diffPatch = "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+2\n--- a/b.txt\n+++ b/b.txt\n@@ -1 +1 @@\n-x\n+y\n"

func (a *App) groupView(cx *gk.Cx) gk.Element {
	fmt.Printf("lists2 group view described\n")
	return kit.Label("[values]")
}

func (a *App) cell(cx *gk.Cx, at table.CellAt) gk.Element {
	return kit.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
}

func (a *App) driveCell(cx *gk.Cx, at table.CellAt) gk.Element {
	a.drawn[fmt.Sprintf("table %d", at.Row)]++
	return kit.Label(fmt.Sprintf("%d:%d", at.Row, at.Col))
}

func (a *App) driveTreeRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	a.drawn[fmt.Sprintf("tree %d", at.Row)]++
	e, _ := table.TreeEntryAt(cx, a.tree, at.Row)
	return table.ListItem(gk.Key("tree", at.Row)).Child(kit.Label(e.Label))
}

func treeNodes(n int) []table.TreeNode {
	nodes := make([]table.TreeNode, n)
	for i := range nodes {
		nodes[i] = table.TreeNode{ID: fmt.Sprintf("n%d", i), Label: fmt.Sprintf("node %d", i)}
	}
	return nodes
}

// ask: the code block's highlights ("proc" in red) and the image's path, each asked once.
func (a *App) askHighlight(cx *gk.Cx, b gk.CodeBlock) []gk.TupleRangeHighlightStyle {
	askState.hlCalls++
	lang, _ := b.Lang.Get()
	fmt.Printf("lists2 ask highlight lang=%s code=%q\n", lang, b.Code)
	return []gk.TupleRangeHighlightStyle{{V0: gk.Range{Lo: 0, Hi: 4}, V1: gk.HighlightStyle{Color: gk.Some(gk.HSLA(0, 0.7, 0.5, 1))}}}
}

func (a *App) askImage(cx *gk.Cx, uri string) string {
	askState.imgCalls++
	fmt.Printf("lists2 ask image uri=%s\n", uri)
	return "/nonexistent/pic.png"
}

// providers: every question of the editor's providers, answered at once.
func (a *App) providerEvent(cx *gk.Cx, e editor.ProviderEvent) {
	name := ""
	switch q := e.Request.(type) {
	case editor.ProviderCompletions:
		name = "completions"
		editor.AnswerCompletions(cx, a.prov, e.ID, editor.CompletionResponse{Items: []gk.CompletionItem{{Label: "alpha"}, {Label: "beta"}}})
	case editor.ProviderHover:
		name = "hover"
		editor.AnswerHover(cx, a.prov, e.ID, gk.Option[string]{})
	case editor.ProviderDefinitions:
		name = "definitions"
		defs := []editor.Definition{{URI: "file:///a.demo", Target: editor.TextRange{EndChar: 5}, Selection: editor.TextRange{EndChar: 5}}}
		editor.AnswerDefinitions(cx, a.prov, e.ID, defs)
	case editor.ProviderColors:
		name = "colors"
		cols := []editor.DocumentColor{{Range: editor.TextRange{EndChar: 1}, RGBA: [4]float32{1, 0, 0, 1}}}
		editor.AnswerColors(cx, a.prov, e.ID, cols)
	case editor.ProviderCodeActions:
		name = "code-actions"
		acts := []editor.CodeAction{{Title: "Fix it", Kind: "quickfix", ID: "fix-1"}}
		editor.AnswerCodeActions(cx, a.prov, e.ID, acts)
	case editor.ProviderPerform:
		name = fmt.Sprintf("perform id=%s", q.ID)
		editor.AnswerPerformed(cx, a.prov, e.ID)
	case editor.ProviderTokens:
		name = "tokens"
		editor.AnswerTokens(cx, a.prov, e.ID, []uint32{0, 0, 1, 0, 0})
	case editor.ProviderInline:
		name = "inline"
		editor.AnswerInline(cx, a.prov, e.ID, gk.Some("lpha"))
	}
	a.asked[name]++
	fmt.Printf("lists2 provider %s id=%d\n", name, e.ID)
}

func pInit(a *App, cx *gk.Cx) {
	switch a.mode {
	case "combobox":
		a.src = gk.NewSearchSource(cx, fruit)
		a.combo = table.NewComboboxState(cx, a.src, []table.BaseIndexPath{table.Row(1)}).Multiple(true).Build()
		a.sub = a.combo.OnComboboxEvent(cx, a.comboEvent)
	case "will-change":
		// The combobox mode's start, with rules (PROTOCOLS.md 11.25): Cherry, which Down and Enter add
		// there, is blocked here; Banana is locked.
		a.src = gk.NewSearchSource(cx, fruit)
		a.combo = table.NewComboboxState(cx, a.src, []table.BaseIndexPath{table.Row(1)}).Multiple(true).Build()
		a.sub = a.combo.OnComboboxEvent(cx, a.comboEvent)
		gk.SearchWillChange(cx, a.src, gk.SearchRules{Locked: []string{"banana"}, Blocked: []string{"cherry"}})
	case "table":
		a.cells = table.NewSource(cx, 20, columns, a.cell, gk.Initial(8))
		a.table = table.NewState(cx, a.cells).Build()
		a.group = gk.NewView(nil, a.groupView)
	case "ask":
		askState.hl = gk.NewAsk(cx, a.askHighlight)
		askState.img = gk.NewAsk(cx, a.askImage)
	case "kept":
		a.kept0 = gk.TestKeptCount()
		a.in1 = editor.NewInputState(cx).Validate(a.noX).Build()
		a.in2 = editor.NewInputState(cx).Build()
	case "drive":
		a.src = table.NewListSource(cx, func() int { return 1000 }, a.driveListRow, gk.Initial(20))
		a.list = table.NewListState(cx, a.src).Build()
		a.cells = table.NewSource(cx, 1000, columns, a.driveCell, gk.Initial(12))
		a.table = table.NewState(cx, a.cells).Build()
		a.tree = table.NewTreeState(cx).Build()
		table.SetTreeItems(cx, a.tree, treeNodes(300))
		a.treeRows = table.NewTreeSource(cx, a.driveTreeRow, gk.Initial(8))
	case "expr":
		// Steps of 1 below 10 and of 10 from there: an expression the library evaluates.
		a.in1 = editor.NewInputState(cx).DefaultValue("9").StepBy("x < 10 ? 1 : 10").Build()
		// The same as NumberStep's ByValue variant, which the binding could not give before.
		a.in2 = editor.NewInputState(cx).DefaultValue("3").Build()
		a.in2.SetStep(cx, gk.Some[editor.NumberStep](editor.NumberStepByValue{Value: "y == 1 ? x : 0.5"})) // Increment doubles, Decrement takes 0.5
	case "bad-expr":
		a.in1 = editor.NewInputState(cx).StepBy("x < 10 ? 1").Build()
	case "providers":
		a.ed = editor.NewState(cx).Language("rust").Build()
		a.prov = editor.Providers(cx, a.ed, a.providerEvent, editor.ProvidersOptions{Triggers: gk.Some("."),
			Which: editor.ProvideCompletions | editor.ProvideHover | editor.ProvideDefinitions | editor.ProvideDocumentColors |
				editor.ProvideCodeActions | editor.ProvideSemanticTokens | editor.ProvideInlineCompletions,
			CodeActionID: "lists2", InlineDebounceMs: 50, TokenTypes: []string{"keyword"}})
	}
}

func pStep(a *App, cx *gk.Cx) {
	switch a.mode {
	case "will-change":
		fmt.Printf("lists2 will-change values=%s\n", strs(a.combo.SelectedValues(cx)))
	case "combobox":
		switch a.step {
		case 1:
			vs := a.combo.SelectedValues(cx)
			sel := a.combo.Selection(cx)
			first, row := "", -1
			if len(sel) > 0 {
				first, row = sel[0].V1.Title, sel[0].V0.Row
			}
			fmt.Printf("lists2 combobox initial values=%s n=%d first=%s row=%d\n", strs(vs), len(sel), first, row)
			added := a.combo.AddSelectedIndex(cx, table.Row(0))
			again := a.combo.AddSelectedIndex(cx, table.Row(0))
			fmt.Printf("lists2 combobox add first=%v again=%v values=%s\n", added, again, strs(a.combo.SelectedValues(cx)))
			removed := a.combo.RemoveSelectedIndex(cx, table.BaseIndexPath{Row: 1})
			fmt.Printf("lists2 combobox remove=%v values=%s\n", removed, strs(a.combo.SelectedValues(cx)))
			a.combo.SetSelectedValues(cx, []string{"apple", "mango"})
			a.combo.SetQuery(cx, "an")
		case 2:
			fmt.Printf("lists2 combobox set values=%s query=%q\n", strs(a.combo.SelectedValues(cx)), a.combo.Query(cx))
			a.combo.ClearSelection(cx)
		case 3:
			_, has := a.combo.SelectedValue(cx).Get()
			fmt.Printf("lists2 combobox cleared n=%d has=%v\n", len(a.combo.SelectedValues(cx)), has)
		}
	case "table":
		switch a.step {
		case 1:
			texts := [][]string{{"0", "zero", "a0"}, {"1", "one", "a1"}}
			table.SetTableTexts(cx, a.table, 0, texts, false)
			table.SetTableRowLabels(cx, a.table, []table.RowLabel{{Row: 1, Label: "the first row"}})
			table.SetTableLook(cx, a.table, table.Look{
				Fit:         []table.Fit{{Col: 1, Width: 150}},
				Groups:      [][]table.Group{{{Label: "Key", Span: 1}, {Label: "Values", Span: 2}}},
				GroupViews:  []table.GroupView{{Label: "Values", View: a.group}},
				HeaderStyle: gk.Some(gk.NewStyleRefinementDefault().Bg(theme.Secondary)),
				RowStyle:    gk.Some(gk.NewStyleRefinementDefault().TextColor(theme.MutedForeground)),
			})
		case 2:
			d := a.table.DumpRange(cx, gk.Range{Lo: 0, Hi: 3})
			rows := make([]string, len(d.V1))
			for i, r := range d.V1 {
				rows[i] = strs(r)
			}
			fmt.Printf("lists2 table dump headers=%s rows=[%s]\n", strs(d.V0), strings.Join(rows, ", "))
			fit := a.table.FitCol(cx, 1)
			nofit := a.table.FitCol(cx, 2)
			back := a.table.Delegate(cx)
			fmt.Printf("lists2 table fit=%v nofit=%v delegate_is_source=%v\n", fit, nofit, back == a.cells)
		}
	case "drive":
		switch a.step {
		case 1:
			// Select and scroll each from code (generated state calls).
			fmt.Printf("lists2 drive before list=%v table=%v tree=%v\n", a.drawnRow("list", 500), a.drawnRow("table", 700), a.drawnRow("tree", 150))
			a.list.SetSelectedIndex(cx, gk.Some(table.Row(500)))
			a.list.ScrollToItem(cx, table.Row(500), gk.ScrollStrategyTop)
			a.table.SetSelectedRow(cx, 700)
			a.table.ScrollToRow(cx, 700)
			a.tree.SetSelectedIndex(cx, gk.Some(150))
			a.tree.ScrollToItem(cx, 150, gk.ScrollStrategyTop)
		case 2:
			li, lok := a.list.SelectedIndex(cx).Get()
			tr, tok := a.table.SelectedRow(cx).Get()
			ti, iok := a.tree.SelectedIndex(cx).Get()
			fmt.Printf("lists2 drive selected list=%d table=%d tree=%d ok=%v\n", li.Row, tr, ti, lok && tok && iok)
			fmt.Printf("lists2 drive scrolled list=%v table=%v tree=%v\n", a.drawnRow("list", 500), a.drawnRow("table", 700), a.drawnRow("tree", 150))
			// The list's scroll handle (an object a read hands out, temporary).
			a.list.ScrollHandle(cx).ScrollToItem(cx, 900, gk.ScrollStrategyTop)
			a.table.ScrollToRow(cx, 0)
		case 3:
			fmt.Printf("lists2 drive handle list900=%v table0=%v\n", a.drawnRow("list", 900), a.drawnRow("table", 0))
		}
	case "kept", "kept-gone":
		switch a.step {
		case 1:
			// Handlers given to calls (a builder's validate, SetValidator) belong to the state.
			a.in2.SetValidator(cx, a.noX)
			a.in2.SetValidator(cx, a.noX)
		case 2:
			a.mode = "kept-gone" // the view stops showing the inputs before they are released
		case 3:
			n := gk.TestKeptCount()
			gk.Release(cx, a.in1)
			gk.Release(cx, a.in2)
			fmt.Printf("lists2 kept added=%d dropped=%v\n", n-a.kept0, gk.TestKeptCount() == a.kept0)
		}
	case "ask":
		gets, asked, answered := gk.TestAsks(cx)
		fmt.Printf("lists2 ask counts asked=%d answered=%d highlight=%d image=%d gets_more=%v\n", asked, answered, askState.hlCalls, askState.imgCalls, gets > asked)
	case "diff":
		// A patch parsed by the kit: the count, each file by its index, a diff state over them;
		// a patch that does not parse gives the kit's error.
		n := editor.DiffFileParseCount(cx, diffPatch)
		count, _ := n.Value.Get()
		bad := editor.DiffFileParseCount(cx, "@@ nonsense\n")
		fmt.Printf("lists2 diff count=%d ok=%v bad_ok=%v bad_err=%q\n", count, n.OK, bad.OK, bad.Err)
		files := make([]editor.DiffFile, count)
		for i := range files {
			files[i] = editor.NewDiffFileParseAt(diffPatch, i)
		}
		st := editor.NewDiffState(cx, files).Build()
		gk.Release(cx, st)
		fmt.Printf("lists2 diff state made\n")
	case "expr":
		fmt.Printf("lists2 expr step_by=%q by_value=%q\n", a.in1.Value(cx), a.in2.Value(cx))
	case "display-map":
		dm := editor.NewDisplayMap(cx, gk.Font{Family: "Inter"}, 14, gk.Option[float32]{})
		dm.SetFoldCandidates(cx, []gk.FoldRange{})
		dm.SetFont(cx, gk.Font{Family: "Inter", Weight: 700}, 16)
		fmt.Printf("lists2 display-map lines=%d rows=%d wraps=%d text=%q folded=%v candidate=%v\n",
			dm.BufferLineCount(cx), dm.DisplayRowCount(cx), dm.WrapRowCount(cx), dm.Text(cx),
			dm.IsFoldedAt(cx, 0), dm.IsFoldCandidate(cx, 0))
		gk.Release(cx, dm)
	}
}
