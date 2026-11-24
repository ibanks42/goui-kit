// The modes that count handles (gk.TestObjCounts) and a table's
// columns as the kit's Column values (gk.SetTableKitColumns).
package main

import (
	"fmt"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/dock"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/overlay"
	"gpuikit.dev/gk/table"
)

func init() {
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
		// a read of a global in a view function: a read, allowed
		return body.Child(gk.Text(fmt.Sprintf("dark %v", gk.ThemeGlobal(cx).IsDark(cx))))
	}
	views["obj-columns"] = func(a *App, cx *gk.Cx, body gk.StatefulDivEl) gk.StatefulDivEl {
		if a.table == (table.State{}) {
			return body
		}
		return body.Child(gk.Div().W(gk.Px(300)).H(gk.Px(240)).Child(table.DataTable(a.table)))
	}
}

// Owned objects: made by a constructor (kept), read at once, written (buffered: the next read
// sees it), a call that returns a value; released.
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

// diffSide prints a DiffSide as the checks expect (the variant's name).
func diffSide(s editor.DiffSide) string {
	switch s {
	case editor.DiffSideModified:
		return "Modified"
	case editor.DiffSideOriginal:
		return "Original"
	}
	return fmt.Sprint(int(s))
}

// The kit's theme global: one handle, reads in place; a read in a view function is allowed.
func global(a *App, cx *gk.Cx) {
	th := gk.ThemeGlobal(cx)
	again := gk.ThemeGlobalMut(cx)
	fmt.Printf("handles theme same=%v dark=%v name=%q radius_full=%.0f\n", th == again, th.IsDark(cx), th.ThemeName(cx), th.RadiusFull(cx))
	reg := gk.ThemeRegistryGlobal(cx)
	fmt.Printf("handles registry ok=%v\n", reg != gk.ThemeRegistry{})
	counts(cx, "globals")
}

// An object a method takes by value and cannot copy is given away: its handle is released.
func givenAway(a *App, cx *gk.Cx) {
	t := overlay.NewTooltipDefaults(cx).WithShowDelay(250 * time.Millisecond).Build()
	t.Install(cx) // TooltipDefaults::install(self, cx): copied (it is Clone), the handle stays
	t.Install(cx)
	counts(cx, "installed twice")
	gk.Release(cx, t)
}

func (a *App) cell(cx *gk.Cx, at table.CellAt) gk.Element {
	return gk.Text(fmt.Sprintf("%d.%d", at.Row, at.Col))
}

// A table's columns as the kit's own Column values (gk.NewColumn(..)...) instead of gk.TableColumn.
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
