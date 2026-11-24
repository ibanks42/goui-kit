// Command editor2 runs headless protocol tests for markdown extensions, editor
// host blocks, decorations, display maps, input edits, LSP overlays, and diagnostics.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

// App holds shared root view and editor/input handles for the active GK_PROTO mode.
type App struct {
	mode string
	root gk.View
	step int
	ed   editor.State
	inp  editor.InputState
	hist gk.History
	coll editor.TextDecorationCollection
	r6   Round6
}

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

// stepScript clicks the top-left "step" button to trigger mode.step.
const stepScript = "click 36 30\nframe\n"

func init() {
	editorView := func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(420)).H(gk.Px(120)).Child(editor.BaseEditor(a.ed))
	}
	modes["annotation"] = &mode{step: annotationStep, script: "frame\n" + stepScript}
	modes["decorations"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = editor.NewState(cx).DefaultValue("hello world\n").Build()
	}, view: editorView, step: decorationsStep, script: stepScript + stepScript + stepScript}
	modes["route"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = editor.NewState(cx).DefaultValue("hello world\n").Build()
	}, view: editorView, step: routeStep, script: "frame\n" + stepScript}
	modes["display-map"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.ed = editor.NewState(cx).DefaultValue(foldText).Build()
	}, view: editorView, step: displayMapStep, script: "frame\n" + stepScript}
	modes["mask-variant"] = &mode{init: func(a *App, cx *gk.Cx) {
		a.inp = editor.NewInputState(cx).Build()
		a.inp.SetMaskPattern(cx, editor.MaskPatternPattern{Pattern: "999-999"})
	}, view: func(a *App, cx *gk.Cx) gk.Element {
		return gk.Div().W(gk.Px(220)).Child(editor.Input(a.inp))
	}, step: func(a *App, cx *gk.Cx) {
		fmt.Printf("editor2 mask-variant value=%q\n", a.inp.Value(cx))
	}, script: "click 100 72\ntype 123456\nframe\n" + stepScript}
}

func rangesText(rs []gk.Range) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = fmt.Sprintf("Range{lo = %d, hi = %d}", r.Lo, r.Hi)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func stringsText(ss []string) string {
	parts := make([]string, len(ss))
	for i, x := range ss {
		parts[i] = fmt.Sprintf("%q", x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// annotationStep proves NewDiffAnnotationFile preserves the id ("ann-1") and
// file path ("src/a.rs") through readback getters.
func annotationStep(a *App, cx *gk.Cx) {
	an := editor.NewDiffAnnotationFile(cx, "ann-1", "src/a.rs")
	fmt.Printf("editor2 annotation id=%q path=%q\n", an.ID(cx), an.Path(cx))
}

// routeStep proves RouteOverlayAction returns false when no editor overlay is
// active to intercept the action.
func routeStep(a *App, cx *gk.Cx) {
	act := gk.NewAction(cx, "editor2.nothing")
	fmt.Printf("editor2 route handled=%v\n", a.ed.RouteOverlayAction(cx, act))
}

// displayMapStep proves DisplayMap getters expose exact buffer line count, full
// buffer text, display row count, and initial unfolded state (folded=false, folds=0).
func displayMapStep(a *App, cx *gk.Cx) {
	folds := a.ed.DisplayMapFoldedRanges(cx)
	fmt.Printf("editor2 display-map lines=%d text=%q rows=%d folded=%v hidden=%v row1=%d folds=%d\n",
		a.ed.DisplayMapBufferLineCount(cx), a.ed.DisplayMapText(cx), a.ed.DisplayMapDisplayRowCount(cx),
		a.ed.DisplayMapIsFoldedAt(cx, 0), a.ed.DisplayMapIsBufferLineHidden(cx, 1),
		a.ed.DisplayMapBufferLineToDisplayRow(cx, 1), len(folds))
}

// decorationsStep proves a retained TextDecorationCollection holds initial range
// [0, 5], appends [6, 11] across steps, and empties cleanly on Clear.
func decorationsStep(a *App, cx *gk.Cx) {
	switch a.step {
	case 1:
		first := []editor.TextDecoration{{Range: gk.Range{Lo: 0, Hi: 5}, Style: gk.HighlightStyle{Color: gk.Some(theme.Danger),
			FontWeight: gk.Some(gk.FontWeight{V0: 700})}}}

		a.coll = a.ed.CreateDecorationsCollection(cx, first)
		gk.Retain(cx, a.coll)
		fmt.Printf("editor2 decorations made ranges=%s\n", rangesText(a.coll.GetRanges(cx)))
		more := []editor.TextDecoration{{Range: gk.Range{Lo: 6, Hi: 11}, Style: gk.HighlightStyle{BackgroundColor: gk.Some(theme.Accent)}}}
		a.coll.Append(cx, more)
	case 2:
		fmt.Printf("editor2 decorations appended ranges=%s\n", rangesText(a.coll.GetRanges(cx)))
		a.coll.Clear(cx)
	case 3:
		fmt.Printf("editor2 decorations cleared ranges=%s\n", rangesText(a.coll.GetRanges(cx)))
	}
}

func (a *App) init(cx *gk.Cx) {
	if m := modes[a.mode]; m.init != nil {
		m.init(a, cx)
	}
}

func (a *App) view(cx *gk.Cx) gk.Element {
	var body gk.Element = kit.Label("-")
	if m := modes[a.mode]; m.view != nil {
		body = m.view(a, cx)
	}
	return gk.VFlex(s.SizeFull, s.P4, s.Gap2).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2).Child(kit.Button("step").Label("step").OnClick(a.stepped))).
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
	if err := gk.RunHeadless("size 480 360\nframe\n"+m.script+"frame\n", a.start); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("editor2 done mode=%s\n", a.mode)
}
