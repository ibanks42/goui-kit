// Command values runs headless protocol tests for value types, scales, themes,
// cells, motion curves, dates/matchers, scroll handles, drag-and-drop,
// settings, questionnaires, backgrounds, and resize-handle renderers.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/base"
	"github.com/ibanks42/goui-kit/gk/chart"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/table"
	"github.com/ibanks42/goui-kit/gk/theme"
)

// App holds shared view, state, and counter fields for the active GK_PROTO mode.
type App struct {
	mode   string
	root   gk.View
	child  gk.View
	clicks int

	diff   editor.DiffState
	ann    editor.DiffAnnotation
	builds int

	scroll gk.ScrollHandle
	quest  base.QuestionnaireState
	picker base.ColorPickerState
	events int
	tip    gk.View
	tips   int

	preview gk.View
	drops   int
	shadow  gk.BoxShadow
	answers int
	submits int
}

type mode struct {
	click func(a *App, cx *gk.Cx)
	view  func(a *App, cx *gk.Cx) (gk.Element, bool)
	body  func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl
	start func(a *App, cx *gk.Cx)
	init  func(a *App, cx *gk.Cx)
	after func(a *App, cx *gk.Cx)

	script string
}

var modes = map[string]*mode{}

func once(f func(a *App, cx *gk.Cx)) func(a *App, cx *gk.Cx) {
	return func(a *App, cx *gk.Cx) {
		if a.clicks == 1 {
			f(a, cx)
		}
	}
}

func init() {
	modes["val-scales"] = &mode{click: once(scales)}
	modes["val-self-arg"] = &mode{click: once(selfArg)}
	modes["val-clipboard"] = &mode{click: clipboardClick, body: clipboardBody,

		script: "size 640 480\nframe\nclick 10 10\nframe\nframe\nclick 600 400\nframe\nframe\nstat\n"}
	modes["val-scroll"] = &mode{click: scroll, body: scrollBody, start: func(a *App, cx *gk.Cx) { a.scroll = gk.NewScrollHandle(cx) }}
	modes["val-readback"] = &mode{click: once(readback)}
	modes["val-handed"] = &mode{click: once(handed)}
	modes["val-ambient"] = &mode{click: once(ambient)}
	modes["val-tooltip"] = &mode{start: func(a *App, cx *gk.Cx) { a.tip = gk.NewView(nil, a.tipView) }, body: tooltipBody,
		script: "size 640 480\nframe\nmove 20 20\nframe\nadvance 1500\nframe\nframe\nmove 30 25\nadvance 1500\nframe\nstat\n"}
	modes["val-box-shadow"] = &mode{click: once(boxShadow), body: boxShadowBody}
	modes["val-drag"] = &mode{start: func(a *App, cx *gk.Cx) { a.preview = gk.NewView(nil, a.previewView) }, view: dragView,
		script: "size 640 480\nframe\ndrag 50 30 350 60\nframe\nframe\nstat\n"}
	modes["val-quest-events"] = &mode{view: questPage, after: questEventsStart,

		script: "size 640 480\nframe\nframe\nclick 100 74\nframe\nframe\nclick 100 132\nframe\nframe\nstat\n"}
}

// scales proves Band, Linear, Point, and Ordinal scales compute accurate
// domain->range mappings (0->0, 50->100, 100->200; blue->hue 0.66), report
// missing/out-of-range keys cleanly, and resolve nearest band index 2 for "c".
func scales(a *App, cx *gk.Cx) {
	band := gk.NewScaleBand(cx, []string{"a", "b", "c"}, [2]float32{0, 300}).PaddingInner(0.1).Build()
	bw, st := band.BandWidth(cx), band.Step(cx)
	ta, _ := band.Tick(cx, "a").Get()
	tc, _ := band.Tick(cx, "c").Get()
	_, missing := band.Tick(cx, "zz").Get()
	fmt.Printf("values band width_ok=%v step_ok=%v order=%v missing=%v nearest=%d\n",
		bw > 0 && bw < 100, st >= bw, ta < tc, !missing, band.NearestIndex(cx, tc))
	lin := chart.NewScaleLinear(cx, []float64{0, 100}, [2]float32{0, 200})
	t0, _ := lin.Tick(cx, 0).Get()
	t50, _ := lin.Tick(cx, 50).Get()
	t100, _ := lin.Tick(cx, 100).Get()
	fmt.Printf("values linear t0=%.0f t50=%.0f t100=%.0f\n", t0, t50, t100)
	pt := gk.NewScalePoint(cx, []string{"x", "y", "z"}, [2]float32{0, 100})
	p0, _ := pt.TickAt(cx, 0).Get()
	p2, _ := pt.TickAt(cx, 2).Get()
	_, none := pt.TickAt(cx, 9).Get()
	fmt.Printf("values point first=%.0f last=%.0f out_of_range=%v\n", p0, p2, !none)
	ord := gk.NewScaleOrdinal(cx, []string{"red", "blue"}, []gk.Color{gk.HSLA(0, 1, 0.5, 1), gk.HSLA(0.66, 1, 0.5, 1)}).Build()
	c, ok := ord.Map(cx, "blue").Get()
	fmt.Printf("values ordinal found=%v hue=%.2f\n", ok, c.H)
	gk.Release(cx, band)
	gk.Release(cx, lin)
	gk.Release(cx, pt)
	gk.Release(cx, ord)
}

// selfArg proves passing a transient value builder handle as an argument to
// itself (`t.Child(t)`) is caught by the cycle guard before executing the next line.
func selfArg(a *App, cx *gk.Cx) {
	t := table.NewTreeItem("a", "A")
	fmt.Println("values giving a value to its own builder")
	t = t.Child(t)
	fmt.Println("values not stopped")
}

// copyValue proves kit.Clipboard's lazy ValueFn runs only when the copy button
// is clicked and writes "copied by the app" into the system clipboard.
func (a *App) copyValue(cx *gk.Cx) string {
	fmt.Println("values clipboard value asked")
	return "copied by the app"
}

func clipboardClick(a *App, cx *gk.Cx) {
	text, ok := gk.ClipboardRead(cx)
	fmt.Printf("values clipboard click=%d ok=%v text=%q\n", a.clicks, ok, text)
}

func clipboardBody(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
	return body.Child(kit.Clipboard("copy").ValueFn(a.copyValue))
}

// scroll proves ScrollHandle reports content taller than its 256px viewport
// (40 rows * 40px) with initial offset 0, and persists programmatic SetOffset(-200).
func scroll(a *App, cx *gk.Cx) {
	h := a.scroll
	switch a.clicks {
	case 1:
		c := h.ContentSize(cx)
		v := h.ViewportBounds(cx)
		fmt.Printf("values scroll content_taller=%v viewport_h=%.0f offset=%.0f\n", c.Height > v.Size.Height, v.Size.Height, h.Offset(cx).Y)
		h.SetOffset(cx, gk.GpuiPoint{X: 0, Y: -200})
	case 2:
		fmt.Printf("values scroll offset=%.0f\n", h.Offset(cx).Y)
	}
}

func scrollBody(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
	list := gk.Div(s.WFull, s.H64).ID("list").OverflowYScroll().TrackScroll(a.scroll)
	for i := 0; i < 40; i++ {
		list = list.Child(gk.Div(s.H10).Child(gk.Text(fmt.Sprintf("row %d", i))))
	}
	return body.Child(list).Child(gk.ScrollbarVertical(a.scroll))
}

// readback proves SetAnswer writes choice "blue" without error, Answer/Answers
// read back the exact chosen slice (len=1, first=blue, empty=false), and
// ItemDefinition preserves required=true and both choice values.
func readback(a *App, cx *gk.Cx) {
	color := base.NewQuestionnaireItemDefinition("color", "Favourite colour").WithRequired(true).
		WithChoice(base.NewQuestionnaireChoiceDefinition("red", "Red")).WithChoice(base.NewQuestionnaireChoiceDefinition("blue", "Blue"))
	a.quest = base.NewQuestionnaireState(cx, []base.QuestionnaireItemDefinition{color}).Build()
	_ = a.quest.OnQuestionnaireEvent(cx, a.questEvent)
	q := a.quest
	err := q.SetAnswer(cx, "color", base.NewQuestionnaireAnswer().WithChoices([]string{"blue"}))
	e := ""
	if err != nil {
		e = err.Error()
	}
	ans, ok := q.Answer(cx, "color").Get()
	var ch []string
	empty := true
	if ok {
		ch = ans.Choices(cx)
		empty = ans.IsEmpty(cx)
	}
	first := ""
	if len(ch) > 0 {
		first = ch[0]
	}
	fmt.Printf("values answer set_failed=%v ok=%v choices=%d first=%s empty=%v err=%s\n", err != nil, ok, len(ch), first, empty, e)
	if def, dok := q.ItemDefinition(cx, "color").Get(); dok {
		cs := def.Choices(cx)
		second := ""
		if len(cs) > 1 {
			second = cs[1].Value(cx)
		}
		fmt.Printf("values definition name=%s required=%v choices=%d second=%s\n", def.Name(cx), def.IsRequired(cx), len(cs), second)
	}
	all := q.Answers(cx)
	got, gok := all.Get(cx, "color").Get()
	gfirst := ""
	if gok {
		gfirst = got.Choices(cx)[0]
	}
	fmt.Printf("values answers len=%d get=%v first=%s\n", all.Len(cx), gok, gfirst)
}

func (a *App) questEvent(cx *gk.Cx, ev base.QuestionnaireEvent) {
	switch e := ev.(type) {
	case base.QuestionnaireEventAnswerChanged:
		a.events++
		ch := e.Value.Answer(cx).Choices(cx)
		fmt.Printf("values event answer_changed item=%s choices=%d\n", e.Value.Item(cx), len(ch))
	}
}

// handed proves repeated getters for a nested GPUI entity (ColorPicker.Sliders.Hue)
// return the same deduplicated handle (`same_handle=true`) and read live slider bounds.
func handed(a *App, cx *gk.Cx) {
	a.picker = base.NewColorPickerState(cx).Build()
	sl := a.picker.Sliders(cx)
	hue := sl.Hue(cx)
	again := sl.Hue(cx)
	v := hue.Value(cx)
	fmt.Printf("values handed hue_max=%.0f same_handle=%v value_kind=%v\n", hue.MaxValue(cx), gk.Handle(hue) == gk.Handle(again), v)
	gk.Release(cx, hue)
}

// ambient proves gk.Locale returns a valid locale identifier and gk.KitTheme
// reports a positive font size.
func ambient(a *App, cx *gk.Cx) {
	th := gk.KitTheme(cx)
	fmt.Printf("values ambient locale=%s theme_font=%v\n", gk.Locale(cx), th.FontSize(cx) > 0)
}

// tipView proves hovering an element with Tooltip(view) describes the tooltip
// view once after the hover delay elapses.
func (a *App) tipView(cx *gk.Cx) gk.Element {
	a.tips++
	fmt.Printf("values tooltip view described n=%d\n", a.tips)
	return gk.Div().Child(kit.Label("a tooltip from Go"))
}

func tooltipBody(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
	return body.Child(gk.Div().W(gk.Px(120)).H(gk.Px(40)).ID("tip").Tooltip(a.tip).Child(gk.Text("hover me")))
}

// boxShadow proves BaseBoxShadow encodes and round-trips theme=0, HSLA (0.5, 1, 0.25, 1),
// offset (1, 2), blur=3, and spread=4 into the Shadow element property.
func boxShadow(a *App, cx *gk.Cx) {
	a.shadow = gk.BaseBoxShadow(cx, 1, 2, 3, 4, gk.HSLA(0.5, 1, 0.25, 1))
	bs := a.shadow
	c := bs.Color.Hsla()
	fmt.Printf("values box shadow theme=%d h=%.2f l=%.2f x=%.0f y=%.0f blur=%.0f spread=%.0f\n", bs.Color.Theme(), c.H, c.L,
		bs.Offset.X, bs.Offset.Y, bs.BlurRadius, bs.SpreadRadius)
}

func boxShadowBody(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
	if a.clicks == 0 {
		return body
	}
	return body.Child(gk.Div().W(gk.Px(80)).H(gk.Px(40)).Bg(theme.Background).Shadow([]gk.BoxShadow{a.shadow}))
}

func (a *App) dropped(cx *gk.Cx, d gk.DragValue) {
	a.drops++
	fmt.Printf("values drop id=%d text=%s\n", d.ID, d.Text)
}

func (a *App) previewView(cx *gk.Cx) gk.Element {
	fmt.Println("values drag preview described")
	return gk.Div().W(gk.Px(60)).H(gk.Px(20)).Bg(theme.Primary)
}

// dragView proves dragging from src (x=50) onto dst (x=350) delivers the
// DragValue payload (id=7, text="row 7") to OnDrop with a custom preview view.
func dragView(a *App, cx *gk.Cx) (gk.Element, bool) {
	src := gk.Div(s.Absolute).Left(gk.Px(10)).Top(gk.Px(10)).W(gk.Px(100)).H(gk.Px(40)).Bg(theme.Muted).
		ID("src").OnDrag(gk.DragValue{ID: 7, Text: "row 7"}, a.preview).Child(gk.Text("drag me"))
	dst := gk.Div(s.Absolute).Left(gk.Px(300)).Top(gk.Px(10)).W(gk.Px(120)).H(gk.Px(120)).Bg(theme.Secondary).
		DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent)).OnDrop(a.dropped)
	return gk.Div(s.SizeFull, s.Relative).Child(src).Child(dst), true
}

// questEventsStart builds a live Questionnaire widget so clicking choice "Blue"
// then "Submit" proves QuestionnaireEventAnswerChanged ("blue") and
// QuestionnaireEventSubmit fire in order.
func questEventsStart(a *App, cx *gk.Cx) {
	color := base.NewQuestionnaireItemDefinition("color", "Favourite colour").
		WithChoice(base.NewQuestionnaireChoiceDefinition("red", "Red")).WithChoice(base.NewQuestionnaireChoiceDefinition("blue", "Blue"))
	a.quest = base.NewQuestionnaireState(cx, []base.QuestionnaireItemDefinition{color}).Build()
	_ = a.quest.OnQuestionnaireEvent(cx, a.questEvent2)
	gk.Notify(cx, a.root)
}

func (a *App) questEvent2(cx *gk.Cx, ev base.QuestionnaireEvent) {
	switch e := ev.(type) {
	case base.QuestionnaireEventAnswerChanged:
		a.answers++
		ch := e.Value.Answer(cx).Choices(cx)
		first := ""
		if len(ch) > 0 {
			first = ch[0]
		}
		fmt.Printf("values quest answer_changed item=%s choice=%s\n", e.Value.Item(cx), first)
	case base.QuestionnaireEventSubmit:
		a.submits++
		fmt.Printf("values quest submit\n")
	}
}

func questPage(a *App, cx *gk.Cx) (gk.Element, bool) {
	q := a.quest
	if gk.Handle(q) == 0 {
		return gk.Div(), true
	}
	choices := kit.QuestionnaireChoices(q, "color").
		Child(kit.QuestionnaireChoice(q, "color", "red").W(gk.Px(200)).H(gk.Px(40)).Child(gk.Text("Red"))).
		Child(kit.QuestionnaireChoice(q, "color", "blue").W(gk.Px(200)).H(gk.Px(40)).Child(gk.Text("Blue")))
	return gk.Div(s.SizeFull).Child(kit.Questionnaire(q).Child(kit.QuestionnaireItem(q, "color").Child(choices)).
		Child(kit.QuestionnaireSubmit(q).W(gk.Px(200)).H(gk.Px(40)).Child(gk.Text("Submit")))), true
}

func (a *App) clicked(cx *gk.Cx, _ gk.ClickEvent) {
	a.clicks++
	if m := modes[a.mode]; m != nil && m.click != nil {
		m.click(a, cx)
	}
}

func (a *App) rootView(cx *gk.Cx) gk.Element {
	m := modes[a.mode]
	if m.view != nil {
		if el, ok := m.view(a, cx); ok {
			return el
		}
	}
	body := gk.Div(s.SizeFull).ID("all").OnClick(a.clicked)
	if m.body != nil {
		body = m.body(a, cx, body)
	}
	return body
}

func (a *App) init(cx *gk.Cx) {
	if m := modes[a.mode]; m.init != nil {
		m.init(a, cx)
	}
}

func (a *App) start(cx *gk.Cx) {
	m := modes[a.mode]
	if m.start != nil {
		m.start(a, cx)
	}
	a.root = gk.NewView(a.init, a.rootView)
	gk.OpenWindow(cx, "values", 640, 480, a.root)
	if m.after != nil {
		m.after(a, cx)
	}
}

func main() {
	a := &App{mode: os.Getenv("GK_PROTO")}
	m, ok := modes[a.mode]
	if !ok {
		fmt.Printf("values: mode %q is unknown\n", a.mode)
		os.Exit(3)
	}
	script := "size 640 480\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nclick 600 400\nframe\nframe\nstat\n"
	if m.script != "" {
		script = m.script
	}
	if s, ok := os.LookupEnv("GK_VALUES_SCRIPT"); ok {
		script = s
	}
	if err := gk.RunHeadless(script, a.start); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("values done mode=%s clicks=%d builds=%d\n", a.mode, a.clicks, a.builds)
}
