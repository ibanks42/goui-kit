// editor2: the protocol tests of the editor family (docs/PROTOCOLS.md 9.20 to 9.26 here, 9.30 to 9.37
// in round6.go and 9.38 to 9.40 in round7.go), the port of core/tests/editor2. Offscreen under a
// gk-meter script (the diag library). Each mode prints lines core/tests/go/run-editor2.sh checks, with
// the exit code.
//
//	GK_PROTO=plugin      Markdown plugins: the kit's frontmatter plugin, a data plugin claiming
//	                     ```badge code blocks (a block row) and one claiming inline code (an inline
//	                     row reading the node and the inherited typography); the claimed count
//	tooltip              a text view's link tooltip: the mouse on the link, the row of its URL
//	                     described (a view closure over a value, one row per distinct URL)
//	surface              a block surface: a Div fragment of the app's view around a blockquote
//	history              History::retain over the entries the app keeps (a predicate as a key set)
//	conflict             a working file's merge conflict read back (DiffFile::conflicts: the
//	                     object a read-back value hands out, its seven reads)
//	annotation           a diff annotation's id read back as text
//	fold                 an editor's fold placeholder: the row of the folded line, drawn by an
//	                     element closure upstream calls without a window
//	owner                an InputBase whose semantic owner is an editor (gk.EntityIDOf)
//	bad-owner            gk.EntityID of a handle that is not an input: must stop (exit 2)
//	decorations          an editor's decoration collection: made from decorations with colours (a
//	                     record with a colour in a call op, decoded in place), appended, cleared
//	display-map          an editor's display map read through the borrow (reads of the state)
//	mask-variant         a mask as MaskPattern's Pattern variant (made by its constructor), typed into
//	route                an action routed to the editor's overlays (an action in a call op): none open
//	editor-style         an editor's style set by the app: its colours, and its highlight resolver an
//	                     ask the editor consults for the highlighter's names (a one-method delegate)
//
// Some modes are in files of their own (text.go, editor.go, host.go, round6.go, round7.go), which
// register them from init.
package main

import (
	"fmt"
	"os"
	"strings"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	mode string
	root gk.View
	step int
	ed   gk.EditorState
	inp  gk.InputState
	hist gk.History
	coll gk.TextDecorationCollection
	r6   Round6 // round6.go (round7's state is in round7.go)
}

// mode is what a mode adds to the app: its init (in the root view's init), its body (below the step
// button), what the step button does, and its gk-meter script after "size 480 360\nframe\n" (a final
// "frame\n" follows). Some modes register from their files' init.
type mode struct {
	init   func(a *App, cx *gk.Cx)
	view   func(a *App, cx *gk.Cx) gk.Element
	step   func(a *App, cx *gk.Cx)
	script string
}

var modes = map[string]*mode{}

const (
	conflictText = "a\n<<<<<<< ours\nb\n=======\nc\n>>>>>>> theirs\nd\n"
	foldText     = "top {\nhidden\n}\nlast\n"
)

// The step button at (36, 30); the body from (16, 56).
const stepScript = "click 36 30\nframe\n"

func init() {
	editorView := func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(420)).H(gk.Px(120)).Child(gk.BaseEditor(a.ed))
	}
	modes["annotation"] = &mode{step: annotationStep, script: "frame\n" + stepScript}
	modes["decorations"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = gk.NewEditorState(cx).DefaultValue("hello world\n").Build()
	}, view: editorView, step: decorationsStep, script: stepScript + stepScript + stepScript}
	modes["route"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = gk.NewEditorState(cx).DefaultValue("hello world\n").Build()
	}, view: editorView, step: routeStep, script: "frame\n" + stepScript}
	modes["display-map"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = gk.NewEditorState(cx).DefaultValue(foldText).Build()
	}, view: editorView, step: displayMapStep, script: "frame\n" + stepScript}
	modes["mask-variant"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.inp = gk.NewInputState(cx).Build()
		a.inp.SetMaskPattern(cx, gk.MaskPatternPattern{Pattern: "999-999"})
	}, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(220)).Child(gk.Input(a.inp))
	}, step: func(a *App, cx *gk.Cx) {
		fmt.Printf("editor2 mask-variant value=%q\n", a.inp.Value(cx))
	}, script: "click 100 72\ntype 123456\nframe\n" + stepScript}
}

// odinRanges prints ranges as Odin's %v does: [Range{lo = 0, hi = 5}, ...].
func odinRanges(rs []gk.Range) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = fmt.Sprintf("Range{lo = %d, hi = %d}", r.Lo, r.Hi)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// odinStrings prints strings as Odin's %v does: ["b", "d"].
func odinStrings(ss []string) string {
	parts := make([]string, len(ss))
	for i, x := range ss {
		parts[i] = fmt.Sprintf("%q", x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func annotationStep(a *App, cx *gk.Cx) {
	an := gk.NewDiffAnnotationFile(cx, "ann-1", "src/a.rs")
	fmt.Printf("editor2 annotation id=%q path=%q\n", an.ID(cx), an.Path(cx))
}

func routeStep(a *App, cx *gk.Cx) {
	act := gk.NewAction(cx, "editor2.nothing")
	fmt.Printf("editor2 route handled=%v\n", a.ed.RouteOverlayAction(cx, act))
}

func displayMapStep(a *App, cx *gk.Cx) {
	folds := a.ed.DisplayMapFoldedRanges(cx)
	fmt.Printf("editor2 display-map lines=%d text=%q rows=%d folded=%v hidden=%v row1=%d folds=%d\n",
		a.ed.DisplayMapBufferLineCount(cx), a.ed.DisplayMapText(cx), a.ed.DisplayMapDisplayRowCount(cx),
		a.ed.DisplayMapIsFoldedAt(cx, 0), a.ed.DisplayMapIsBufferLineHidden(cx, 1),
		a.ed.DisplayMapBufferLineToDisplayRow(cx, 1), len(folds))
}

func decorationsStep(a *App, cx *gk.Cx) {
	switch a.step {
	case 1:
		first := []gk.TextDecoration{{Range: gk.Range{Lo: 0, Hi: 5}, Style: gk.HighlightStyle{Color: gk.Some(theme.Danger),
			FontWeight: gk.Some(gk.FontWeight{V0: 700})}}}
		// a call's object is temporary (until the next frame): kept across the steps
		a.coll = a.ed.CreateDecorationsCollection(cx, first)
		gk.Retain(cx, a.coll)
		fmt.Printf("editor2 decorations made ranges=%s\n", odinRanges(a.coll.GetRanges(cx)))
		more := []gk.TextDecoration{{Range: gk.Range{Lo: 6, Hi: 11}, Style: gk.HighlightStyle{BackgroundColor: gk.Some(theme.Accent)}}}
		a.coll.Append(cx, more)
	case 2:
		fmt.Printf("editor2 decorations appended ranges=%s\n", odinRanges(a.coll.GetRanges(cx)))
		a.coll.Clear(cx)
	case 3:
		fmt.Printf("editor2 decorations cleared ranges=%s\n", odinRanges(a.coll.GetRanges(cx)))
	}
}

func (a *App) init(cx *gk.Cx) {
	if m := modes[a.mode]; m.init != nil {
		m.init(a, cx)
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	var body gk.Element = gk.Label("-")
	if m := modes[a.mode]; m.view != nil {
		body = m.view(a, cx)
	}
	return gk.VFlex(s.SizeFull, s.P4, s.Gap2).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2).Child(gk.Button("step").Label("step").OnClick(a.stepped))).
		Child(body)
}

func (a *App) stepped(cx *gk.Cx, _ gk.ClickEvent) {
	a.step++
	if m := modes[a.mode]; m.step != nil {
		m.step(a, cx)
	}
}

func (a *App) start(cx *gk.Cx) {
	a.root = gk.NewView(a.init, a.view)
	gk.OpenWindow(cx, "editor2", 480, 360, a.root)
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	m, ok := modes[a.mode]
	if !ok {
		fmt.Printf("editor2: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	gk.RunHeadless("size 480 360\nframe\n"+m.script+"frame\n", a.start)
	fmt.Printf("editor2 done mode=%s\n", a.mode)
}
