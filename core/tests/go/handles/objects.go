package main

import (
	"fmt"
	"time"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/dock"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/table"
)

func init() {
	// Register owned-object, global-singleton, given-away, and kit-column tests.
	modes["obj-objects"] = func(a *App, cx *gk.Cx) {
		if a.clicks == 1 {
			objects(a, cx)
		}
	}
	modes["obj-global"] = func(a *App, cx *gk.Cx) {
		if a.clicks == 1 {
			global(a, cx)
		}
	}
	modes["obj-given-away"] = func(a *App, cx *gk.Cx) {
		if a.clicks == 1 {
			givenAway(a, cx)
		}
	}
	modes["obj-columns"] = columns
	views["obj-global"] = func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
		// Verify global theme handles can be read inside view callbacks without leaking.
		return body.Child(gk.Text(fmt.Sprintf("dark %v", gk.ThemeGlobal(cx).IsDark(cx))))
	}
	views["obj-columns"] = func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
		if a.table == (table.State{}) {
			return body
		}
		return body.Child(gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(table.DataTable(a.table)))
	}
}

// objects proves permanent HK_OBJ handles (Dock, DiffLinePosition, SearchMatcher)
// support field getters/setters and increment live object counts from 0 to 3
// with 0 temporaries before explicit release.
func objects(a *App, cx *gk.Cx) {
	counts(cx, "before")
	d := dock.NewDock(cx, 240)
	fmt.Printf("handles dock open=%v size=%.0f\n", d.IsOpen(cx), d.Size(cx))
	d.SetOpen(cx, false)
	d.SetSize(cx, 300)
	d.SetResizing(cx, true)
	fmt.Printf("handles dock written open=%v size=%.0f resizing=%v\n", d.IsOpen(cx), d.Size(cx), d.IsResizing(cx))
	p := editor.NewDiffLinePosition(cx, "src/a.rs", editor.DiffSideModified, 7)
	fmt.Printf("handles position path=%s side=%s line=%d\n", p.Path(cx), diffSide(p.Side(cx)), p.Line(cx))
	m := editor.NewSearchMatcher(cx)
	m.UpdateQuery(cx, "needle", true)
	fmt.Printf("handles matcher len=%d empty=%v label=%q\n", m.Len(cx), m.IsEmpty(cx), m.Label(cx))
	counts(cx, "made")
	gk.Release(cx, d)
	gk.Release(cx, p)
	gk.Release(cx, m)
}

func diffSide(s editor.DiffSide) string {
	switch s {
	case editor.DiffSideModified:
		return "Modified"
	case editor.DiffSideOriginal:
		return "Original"
	}
	return fmt.Sprint(int(s))
}

// global proves ThemeGlobal and ThemeGlobalMut return the same GPUI Global
// handle identity (same=true) and that global singletons count as permanent
// live handles (temps=0, live=2 for Theme + ThemeRegistry).
func global(a *App, cx *gk.Cx) {
	th := gk.ThemeGlobal(cx)
	again := gk.ThemeGlobalMut(cx)
	fmt.Printf("handles theme same=%v dark=%v name=%q radius_full=%.0f\n", th == again, th.IsDark(cx), th.ThemeName(cx), th.RadiusFull(cx))
	reg := gk.ThemeRegistryGlobal(cx)
	fmt.Printf("handles registry ok=%v\n", reg != gk.ThemeRegistry{})
	counts(cx, "globals")
}

// givenAway proves a clonable HK_OBJ value (TooltipDefaults) can be passed by
// value twice (Install twice) without consuming or invalidating its handle
// (temps=0, live=1).
func givenAway(a *App, cx *gk.Cx) {
	t := overlay.NewTooltipDefaults(cx).WithShowDelay(250 * time.Millisecond).Build()
	t.Install(cx)
	t.Install(cx)
	counts(cx, "installed twice")
	gk.Release(cx, t)
}

func (a *App) cell(cx *gk.Cx, at table.CellAt) gk.Element {
	return gk.Text(fmt.Sprintf("%d.%d", at.Row, at.Col))
}

// columns proves SetTableKitColumns installs kit-typed Column descriptors
// (Name width 140, Size width 60) and supports live replacement with 3 columns.
func columns(a *App, cx *gk.Cx) {
	switch a.clicks {
	case 1:
		a.tsrc = table.NewSource(cx, 40, nil, a.cell)
		table.SetTableKitColumns(cx, a.tsrc, []table.Column{table.NewColumn("name", "Name").Width(140).Sortable(), table.NewColumn("size", "Size").Width(60).TextRight()})
		a.table = table.NewState(cx, a.tsrc).Build()
	case 2:
		hs := a.table.Headers(cx)
		w0, _ := a.table.ColWidth(cx, 0).Get()
		first, second := "", ""
		if len(hs) > 0 {
			first = hs[0]
		}
		if len(hs) > 1 {
			second = hs[1]
		}
		fmt.Printf("handles columns n=%d first=%s second=%s width0=%.0f\n", len(hs), first, second, w0)
		table.SetTableKitColumns(cx, a.tsrc, []table.Column{table.NewColumn("name", "Name"), table.NewColumn("size", "Size"), table.NewColumn("kind", "Kind")}, a.table)
	case 3:
		fmt.Printf("handles columns replaced n=%d\n", len(a.table.Headers(cx)))
	}
}
