// Protocol tests for extended descriptor kinds: jiff civil dates and times, value cells,
// fallible Result values, expression validation, item procedures, Ask data closures,
// SVG transforms, image caches, scroll anchors, and synthetic accessibility nodes.
package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/base"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/table"
	"github.com/ibanks42/goui-kit/gk/theme"
)

type kindsApp struct {
	mode                   string
	root                   gk.View
	cal                    base.CalendarState
	tree                   table.TreeState
	label, img             gk.Ask[string, string]
	calLabel               gk.Ask[base.TupleCalendarItemKindInt32, string]
	hl                     gk.Ask[gk.CodeBlock, []gk.TupleRangeHighlightStyle]
	cache                  gk.ImageCache
	anchor                 gk.ScrollAnchor
	described, days, wdays int
	active, labels         int
	hlCalls, imgCalls      int
	frames                 int
}

func kindsMain(mode string) bool {
	if !strings.HasPrefix(mode, "kinds-") {
		return false
	}
	a := &kindsApp{mode: mode}
	script := "size 640 480\nframe\nframe\nframe\n"
	if mode == "kinds-elements" {
		script += "frame\nadvance 100\nframe\nadvance 100\nframe\nframe\n"
	}
	script += "frame\nframe\nframe\nstat\n"
	if err := gk.RunHeadless(script, func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "kinds", 640, 480, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	switch mode {
	case "kinds-items", "kinds-label":
		fmt.Printf("kinds calendar described=%d days=%d weekdays=%d active=%d labels=%d\n", a.described, a.days, a.wdays, a.active, a.labels)
	case "kinds-elements":
		fmt.Printf("kinds asks highlight=%d image=%d\n", a.hlCalls, a.imgCalls)
	}
	fmt.Printf("kinds done mode=%s described=%d frames=%d\n", mode, a.described, a.frames)
	return true
}

func (a *kindsApp) dates(cx *gk.Cx) {
	cal := base.NewCalendarState(cx).Build()
	cal.SetDate(cx, base.DateSingle{Value: gk.Some(gk.CivilDate{Year: 2024, Month: 2, Day: 29})})
	d, _ := cal.Date(cx).(base.DateSingle).Value.Get()
	today := cal.Today(cx)
	weeks := cal.Days(cx)
	wl := 0
	if len(weeks) > 0 {
		wl = len(weeks[0])
	}
	fmt.Printf("kinds calendar date=%d-%02d-%02d today_ok=%v weeks=%d week_len=%d\n", d.Year, d.Month, d.Day,
		today.Year >= 2024 && today.Month >= 1 && today.Month <= 12, len(weeks), wl)
	ok := cal.ApplyDate(cx, base.DateRange{V0: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 5}), V1: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 9})})
	r, _ := cal.Date(cx).(base.DateRange)
	st, _ := r.V0.Get()
	en, _ := r.V1.Get()
	fmt.Printf("kinds calendar range applied=%v start=%d-%02d-%02d end=%d\n", ok, st.Year, st.Month, st.Day, en.Day)
	cal2 := base.NewCalendarState(cx).Build()
	sel := cal2.SelectDate(cx, gk.CivilDate{Year: 2026, Month: 3, Day: 14})
	if one, isOne := cal2.Date(cx).(base.DateSingle); isOne {
		d, _ = one.Value.Get()
	}
	gk.Release(cx, cal2)
	fmt.Printf("kinds calendar select=%v date=%d-%02d-%02d\n", sel, d.Year, d.Month, d.Day)
	tf := base.NewTimeFieldState(cx).Build()
	tf.SetTime(cx, gk.CivilTime{Hour: 23, Minute: 59})
	t := tf.Time(cx)
	fmt.Printf("kinds time %02d:%02d:%02d\n", t.Hour, t.Minute, t.Second)
	gk.Release(cx, cal)
	gk.Release(cx, tf)
}

func (a *kindsApp) cells(cx *gk.Cx) {
	b := gk.NewCell(cx, true)
	str := gk.NewCell(cx, "first")
	f := gk.NewCell(cx, 1.5)
	fmt.Printf("kinds cells made b=%v s=%s f=%g\n", gk.CellGet(cx, b), gk.CellGet(cx, str), gk.CellGet(cx, f))
	gk.CellSet(cx, b, false)
	gk.CellSet(cx, str, "second")
	gk.CellSet(cx, f, 2.25)
	fmt.Printf("kinds cells again b=%v s=%s f=%g\n", gk.CellGet(cx, b), gk.CellGet(cx, str), gk.CellGet(cx, f))
	gk.Release(cx, b)
	gk.Release(cx, str)
	gk.Release(cx, f)
}

func (a *kindsApp) results(cx *gk.Cx) {
	bg := gk.TryParseBackground(cx, "#808080")
	v, _ := bg.Value.Get()
	fmt.Printf("kinds background solid ok=%v kind=%d l=%.2f a=%.2f\n", bg.OK, v.Kind, v.Solid.L, v.Solid.A)
	bad := gk.TryParseBackground(cx, "not a colour")
	fmt.Printf("kinds background bad ok=%v has=%v err=%v\n", bad.OK, bad.Value.Valid(), bad.Err != "")
	c, err := gk.TryParseColor(cx, "#ff0000").Result()
	fmt.Printf("kinds color h=%.2f s=%.2f l=%.2f err=%v\n", c.H, c.S, c.L, err)
	_, err = gk.TryParseColor(cx, "nope").Result()
	fmt.Printf("kinds color bad err=%v\n", err != nil)
	patch := "--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-old\n+new\n--- a/y.txt\n+++ b/y.txt\n@@ -1 +1 @@\n-a\n+b\n"
	n, err := editor.DiffFileParseCount(cx, patch).Result()
	fmt.Printf("kinds diff count=%d err=%v\n", n, err)
	fmt.Printf("kinds expr ok=%q bad=%q args=%q\n", gk.ExprCheck("x < 10 ? 1 : min(x, 2)", 1), gk.ExprCheck("x +", 1), gk.ExprCheck("y", 1))
}

func (a *kindsApp) day(cx *gk.Cx, item base.CalendarItemEl, st base.CalendarItemState) gk.Element {
	a.described++
	switch st.Kind {
	case base.CalendarItemKindDay:
		a.days++
	case base.CalendarItemKindWeekday:
		a.wdays++
	}
	if st.IsActive {
		a.active++
	}
	if st.Kind != base.CalendarItemKindDay {
		return item
	}
	bg := theme.Background
	if st.IsToday {
		bg = theme.Primary
	}
	return item.Style(s.RoundedFull).Bg(bg)
}

func (a *kindsApp) entry(cx *gk.Cx, ix int, e base.TreeEntryReads, st base.TreeEntryState) gk.Element {
	a.described++
	fmt.Printf("kinds tree row ix=%d label=%s depth=%d folder=%v selected=%v\n", ix, e.Item.Label, e.Depth, e.IsFolder, st.IsSelected)
	return gk.Div(s.H6, s.Px2).Child(gk.Text(fmt.Sprintf("%s (%d)", e.Item.Label, ix)))
}

func (a *kindsApp) dayLabel(cx *gk.Cx, k base.TupleCalendarItemKindInt32) string {
	a.labels++
	return fmt.Sprintf("L%d", k.V1)
}

func (a *kindsApp) highlight(cx *gk.Cx, b gk.CodeBlock) []gk.TupleRangeHighlightStyle {
	a.hlCalls++
	lang, _ := b.Lang.Get()
	fmt.Printf("kinds ask highlight lang=%s code=%q\n", lang, b.Code)
	return []gk.TupleRangeHighlightStyle{{V0: gk.Range{Lo: 0, Hi: 4}, V1: gk.HighlightStyle{Color: gk.Some(gk.HSLA(0, 0.7, 0.5, 1))}}}
}

func (a *kindsApp) image(cx *gk.Cx, uri string) string {
	a.imgCalls++
	fmt.Printf("kinds ask image uri=%s\n", uri)
	return "/nonexistent/pic.png"
}

func (a *kindsApp) init(cx *gk.Cx) {
	gk.OnFrame(cx, func(cx *gk.Cx) {
		a.frames++
		if a.frames == 8 && a.mode == "kinds-elements" {
			a.askCounts(cx)
		}
	})
	switch a.mode {
	case "kinds-values":
		a.dates(cx)
		a.cells(cx)
		a.results(cx)
	case "kinds-bad-date":
		cal := base.NewCalendarState(cx).Build()
		fmt.Println("kinds writing February 30")
		cal.SetDate(cx, base.DateSingle{Value: gk.Some(gk.CivilDate{Year: 2026, Month: 2, Day: 30})})
		fmt.Println("kinds not stopped")
	case "kinds-items", "kinds-label", "kinds-wrong-kind":
		a.cal = base.NewCalendarState(cx).Build()
		a.calLabel = gk.NewAsk(cx, a.dayLabel)
	case "kinds-tree":
		a.tree = table.NewTreeState(cx).Items([]table.TreeItem{table.NewTreeItem("a", "Alpha"), table.NewTreeItem("b", "Beta")}).Build()
	case "kinds-elements":
		a.hl = gk.NewAsk(cx, a.highlight)
		a.img = gk.NewAsk(cx, a.image)
		a.cache = gk.NewImageCache(cx)
		a.anchor = gk.NewScrollAnchor(cx, gk.NewScrollHandle(cx))
	}
}

func (a *kindsApp) view(cx *gk.Cx) gk.Element {
	root := gk.Div(s.SizeFull, s.P2).Bg(theme.Background).TextColor(theme.Foreground)
	switch a.mode {
	case "kinds-items", "kinds-label":
		cal := base.Calendar("cal", a.cal).FirstDayOfWeek(gk.WeekdaySun).Item(overlay.Items(cx, a.day))
		if a.mode == "kinds-label" {
			cal = cal.Label(a.calLabel)
		}
		return root.Child(cal)
	case "kinds-wrong-kind":
		return root.Child(base.Calendar("cal", a.cal).Item(overlay.Items(cx, a.entry)))
	case "kinds-tree":
		tree := table.BaseTree(a.tree, s.SizeFull).ListStyle(gk.NewStyleRefinementDefault(s.SizeFull))
		return root.Child(gk.Div().H(gk.Px(200)).Child(tree.Item(overlay.Items(cx, a.entry))))
	case "kinds-bad-expr":
		return root.Child(kit.Spinner().Ease("x +"))
	case "kinds-elements":
		t := gk.NewTransformation().Scaled(0.5, 0.5).Translated(2, 3).Rotated(0.25).WithAlpha(0.5)
		return root.
			Child(gk.SVG().Path("icons/loader.svg").W(gk.Px(16)).H(gk.Px(16)).WithTransformation(t)).
			Child(gk.SVG().Path("icons/loader.svg").W(gk.Px(16)).H(gk.Px(16)).
				WithTimedTransformation(gk.NewTimedRotation(time.Second, gk.NewTransformation()))).
			Child(gk.Icon(gk.IconStarFill).Transform(gk.NewTransformation().Rotated(0.5))).
			Child(gk.Div().ImageCache(a.cache).WithDynamicPrepaintOrder([]int{1, 0}).
				Child(gk.Text("one")).Child(gk.Text("two")).
				OnDragMove(func(cx *gk.Cx, e gk.DragMoveEvent) {})).
			Child(gk.Div().ImageCache(gk.RetainAllImages{ID: "kept"}).ID("anchored").AnchorScroll(gk.Some(a.anchor)).
				A11ySyntheticChildren([]gk.A11yChild{{Key: "q1", Role: gk.RoleListItem, Label: "Q1: 12"}}).
				ExternalDragPayload(gk.ExternalDragPayload{Files: []gk.ExternalFile{{Path: "/nonexistent/a.txt"}}}).
				Child(gk.Text("anchored"))).
			Child(gk.Div().W(gk.Px(400)).Child(editor.BaseTextViewMarkdown("md", "```go\nfunc x\n```\n\n![pic](pic.png)\n").
				CodeBlockHighlighter(a.hl).ImageSource(a.img)))
	}
	return root
}

func (a *kindsApp) askCounts(cx *gk.Cx) {
	gets, asked, answered := gk.TestAsks(cx)
	fmt.Printf("kinds ask counts asked=%d answered=%d gets_more=%v\n", asked, answered, gets > asked)
}
