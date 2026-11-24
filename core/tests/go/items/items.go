// The modes of gk.Items (an item function as the row source a builder takes) and
// the builders that take it.
package main

import (
	"fmt"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/base"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/table"
	"gpuikit.dev/gk/theme"
)

func init() {
	views["items-calendar"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(base.Calendar("cal", a.cal).Item(overlay.Items(cx, a.day)))
	}
	views["items-wrong-kind"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(base.Calendar("cal", a.cal).Item(overlay.Items(cx, a.segment))) // stops: a time field's function
	}
	views["items-time-field"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(base.TimeField("tf", a.tf).RenderSegment(overlay.Items(cx, a.segment)))
	}
	views["items-tree"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		tree := table.BaseTree(a.tree, s.SizeFull).ListStyle(gk.NewStyleRefinementDefault(s.SizeFull))
		return root.Child(gk.Div().H(gk.Px(200)).Child(tree.Item(overlay.Items(cx, a.entry))))
	}
	views["items-popover"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		pop := overlay.BasePopover("pop").Open(true).Trigger(kit.Button("t").Label("Open"))
		return root.Child(pop.Content(overlay.Items(cx, a.popContent)))
	}
	views["items-command"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(table.Command(a.cmd).Header(overlay.Items(cx, a.cmdHeader)).Footer(overlay.Items(cx, a.cmdHeader)))
	}
	views["items-hover"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		card := overlay.BaseHoverCard("card").OpenDelay(0).Trigger(gk.Div(s.W20, s.H10).Bg(theme.Primary))
		return root.Child(card.Content(overlay.Items(cx, a.hoverContent)))
	}
	views["items-combobox"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(table.Combobox(a.combo).Placeholder("Fruit").RenderTrigger(overlay.Items(cx, a.trigger)))
	}
	views["items-nav"] = func(a *App, cx *gk.Cx, root gk.DivEl) gk.Element {
		return root.Child(base.NavStack(a.nav, s.SizeFull).Item(overlay.Items(cx, a.navPage)))
	}
	modeInit["items-nav"] = func(a *App, cx *gk.Cx) {
		a.nav.Push(cx, a.page, base.NavMotionImmediate)
	}
}

// --- the functions ---------------------------------------------------------------------------------

func (a *App) day(cx *gk.Cx, item base.CalendarItemEl, st base.CalendarItemState) gk.Element {
	a.described++
	a.kinds[st.Kind]++
	if st.IsActive {
		a.active++
	}
	if a.frames >= 3 {
		a.after++
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

func (a *App) segment(cx *gk.Cx, seg base.TimeFieldSegmentEl, st base.TimeFieldSegmentState) gk.Element {
	a.described++
	a.segments[st.Segment]++
	fmt.Printf("items segment=%s value=%d selected=%v\n", name(segmentNames[:], int(st.Segment)), st.Value, st.IsSelected)
	return gk.Div(s.Px1).Bg(theme.Muted).Child(seg)
}

func (a *App) entry(cx *gk.Cx, ix int, e base.TreeEntryReads, st base.TreeEntryState) gk.Element {
	a.described++
	fmt.Printf("items tree row ix=%d label=%s depth=%d folder=%v selected=%v\n", ix, e.Item.Label, e.Depth, e.IsFolder,
		st.IsSelected)
	return gk.Div(s.H6, s.Px2).Child(gk.Text(fmt.Sprintf("%s (%d)", e.Item.Label, ix)))
}

func (a *App) popContent(cx *gk.Cx, st base.PopoverStateReads) gk.Element {
	a.described++
	fmt.Printf("items popover content is_open=%v\n", st.IsOpen)
	return gk.Div(s.P2).Child(gk.Text("inside the popover"))
}

func (a *App) cmdHeader(cx *gk.Cx, st kit.CommandStateReads) gk.Element {
	a.described++
	_, has := st.SelectedIndex.Get()
	fmt.Printf("items command header matched=%d loading=%v selected=%v\n", st.MatchedCount, st.IsLoading, has)
	return gk.Text(fmt.Sprintf("%d matches", st.MatchedCount))
}

func (a *App) hoverContent(cx *gk.Cx, st base.HoverCardStateReads) gk.StatefulDivEl {
	a.described++
	fmt.Printf("items hover content is_open=%v\n", st.IsOpen)
	return gk.Div(s.P2).ID("card").Child(gk.Text("card"))
}

func (a *App) trigger(cx *gk.Cx, t kit.ComboboxTriggerContext) gk.Element {
	a.described++
	ph, _ := t.Placeholder.Get()
	first := ""
	if len(t.Selection) > 0 {
		first = t.Selection[0].V1.Title
	}
	fmt.Printf("items combobox trigger selected=%d first=%s placeholder=%s open=%v\n", len(t.Selection), first, ph, t.IsOpen)
	return gk.Div(s.P1).Child(gk.Text(fmt.Sprintf("%d chosen", len(t.Selection))))
}

func (a *App) navPage(cx *gk.Cx, pg base.NavPageEl, r base.NavPageReads) gk.Element {
	a.described++
	fmt.Printf("items nav page index=%d phase=%s progress=%.1f\n", r.Index, name(phaseNames[:], int(r.Phase)), r.Progress)
	return pg.Opacity(r.Progress)
}
