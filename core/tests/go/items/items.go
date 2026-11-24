// The modes of gk.Items (an item function as the row source a builder takes) and
// the builders that take it.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

func init() {
	views["items-calendar"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(gk.BaseCalendar("cal", a.cal).Item(gk.Items(cx, a.day)))
	}
	views["items-wrong-kind"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(gk.BaseCalendar("cal", a.cal).Item(gk.Items(cx, a.segment))) // stops: a time field's function
	}
	views["items-time-field"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(gk.BaseTimeField("tf", a.tf).RenderSegment(gk.Items(cx, a.segment)))
	}
	views["items-tree"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		tree := gk.BaseTree(a.tree, s.SizeFull).ListStyle(gk.NewStyleRefinementDefault(s.SizeFull))
		return root.Child(gk.Div().H(gk.Px(200)).Child(tree.Item(gk.Items(cx, a.entry))))
	}
	views["items-popover"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		pop := gk.BasePopover("pop").Open(true).Trigger(gk.Button("t").Label("Open"))
		return root.Child(pop.Content(gk.Items(cx, a.popContent)))
	}
	views["items-command"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(gk.Command(a.cmd).Header(gk.Items(cx, a.cmdHeader)).Footer(gk.Items(cx, a.cmdHeader)))
	}
	views["items-hover"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		card := gk.BaseHoverCard("card").OpenDelay(0).Trigger(gk.Div(s.W20, s.H10).Bg(theme.Primary))
		return root.Child(card.Content(gk.Items(cx, a.hoverContent)))
	}
	views["items-combobox"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(gk.Combobox(a.combo).Placeholder("Fruit").RenderTrigger(gk.Items(cx, a.trigger)))
	}
	views["items-nav"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(gk.NavStack(a.nav, s.SizeFull).Item(gk.Items(cx, a.navPage)))
	}
	modeInit["items-nav"] = func(a *App, cx *gk.Cx) {
		a.nav.Push(cx, a.page, gk.NavMotionImmediate)
	}
}

// --- the functions ---------------------------------------------------------------------------------

func (a *App) day(cx *gk.Cx, item gk.CalendarItemEl, st gk.BaseCalendarItemState) gk.Element {
	a.described++
	a.kinds[st.Kind]++
	if st.IsActive {
		a.active++
	}
	if a.frames >= 3 {
		a.after++
	}
	if st.Kind != gk.CalendarItemKindDay {
		return item
	}
	bg := theme.Background
	if st.IsToday {
		bg = theme.Primary
	}
	return item.Style(s.RoundedFull).Bg(bg)
}

func (a *App) segment(cx *gk.Cx, seg gk.TimeFieldSegmentEl, st gk.TimeFieldSegmentState) gk.Element {
	a.described++
	a.segments[st.Segment]++
	fmt.Printf("items segment=%s value=%d selected=%v\n", name(segmentNames[:], int(st.Segment)), st.Value, st.IsSelected)
	return gk.Div(s.Px1).Bg(theme.Muted).Child(seg)
}

func (a *App) entry(cx *gk.Cx, ix int, e gk.TreeEntryReads, st gk.BaseTreeEntryState) gk.Element {
	a.described++
	fmt.Printf("items tree row ix=%d label=%s depth=%d folder=%v selected=%v\n", ix, e.Item.Label, e.Depth, e.IsFolder,
		st.IsSelected)
	return gk.Div(s.H6, s.Px2).Child(gk.Text(fmt.Sprintf("%s (%d)", e.Item.Label, ix)))
}

func (a *App) popContent(cx *gk.Cx, st gk.PopoverStateReads) gk.Element {
	a.described++
	fmt.Printf("items popover content is_open=%v\n", st.IsOpen)
	return gk.Div(s.P2).Child(gk.Text("inside the popover"))
}

func (a *App) cmdHeader(cx *gk.Cx, st gk.CommandStateReads) gk.Element {
	a.described++
	_, has := st.SelectedIndex.Get()
	fmt.Printf("items command header matched=%d loading=%v selected=%v\n", st.MatchedCount, st.IsLoading, has)
	return gk.Text(fmt.Sprintf("%d matches", st.MatchedCount))
}

func (a *App) hoverContent(cx *gk.Cx, st gk.HoverCardStateReads) gk.StatefulDivEl {
	a.described++
	fmt.Printf("items hover content is_open=%v\n", st.IsOpen)
	return gk.Div(s.P2).ID("card").Child(gk.Text("card"))
}

func (a *App) trigger(cx *gk.Cx, t gk.ComboboxTriggerContext) gk.Element {
	a.described++
	ph, _ := t.Placeholder.Get()
	first := ""
	if len(t.Selection) > 0 {
		first = t.Selection[0].V1.Title
	}
	fmt.Printf("items combobox trigger selected=%d first=%s placeholder=%s open=%v\n", len(t.Selection), first, ph, t.IsOpen)
	return gk.Div(s.P1).Child(gk.Text(fmt.Sprintf("%d chosen", len(t.Selection))))
}

func (a *App) navPage(cx *gk.Cx, pg gk.NavPageEl, r gk.NavPageReads) gk.Element {
	a.described++
	fmt.Printf("items nav page index=%d phase=%s progress=%.1f\n", r.Index, name(phaseNames[:], int(r.Phase)), r.Progress)
	return pg.Opacity(r.Progress)
}
