// Runtime misuse cases for the editor and Markdown host protocols (GK_NEG):
// invalid semantic owners, released plugin/decorator row sources, stale held blocks,
// expired temporary leaf geometry handles, and view-time mutations.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/editor"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/overlay"
	"github.com/ibanks42/goui-kit/gk/s"
)

type App struct {
	mode   string
	root   gk.View
	rows   gk.RowSource
	ed     editor.State
	hist   gk.History
	drawn  int
	rowed  int
	held   []editor.BlockNodeHeld
	leaf   gk.Option[editor.LeafGeometry]
	leaves int
	inp    editor.InputState
	plus   gk.RowSource
	ext    editor.MarkdownExtensionsArc
	custom gk.Option[editor.BlockNodeHeld]
}

const md = "> [!note]\n> inside\n\n```badge\nok\n```\n"

func (a *App) badge(cx *gk.Cx, n gk.MarkdownNodeRead) gk.Element {
	a.rowed++
	return kit.Label(n.AsText(cx))
}

func (a *App) init(cx *gk.Cx) {
	a.rows = overlay.Items(cx, a.badge)
	a.ed = editor.NewState(cx).DefaultValue("x").Build()
	a.hist = gk.NewHistory(cx).Build()
	switch a.mode {
	case "ed-owner-released":
		gk.Release(cx, a.ed)
	case "ed-plugin-freed":
		gk.Release(cx, a.rows)
	case "ed6-ok", "ed6-held-stale", "ed6-leaf-stale", "ed6-decorate-freed", "ed6-lent-view":
		ext := editor.NewMarkdownExtensionsArc(cx, editor.NewMarkdownExtensionsDefault())
		held, _ := editor.BaseParseMarkdown(cx, "# Kept\n\ntext\n", ext).Value.Get()
		for _, h := range held {
			gk.Retain(cx, h)
		}
		if a.mode == "ed6-held-stale" && len(held) > 0 {
			gk.Release(cx, held[0])
		}
		a.held = held
		a.inp = editor.NewInputState(cx).DefaultValue("3").Build()
		a.plus = gk.NewRowSource(cx, 0, a.plusRow, gk.Initial(1))
		if a.mode == "ed6-decorate-freed" {
			gk.Release(cx, a.plus)
		}
	case "ed7-ok", "ed7-custom-unretained":
		a.ext = editor.NewMarkdownExtensionsArc(cx, editor.NewMarkdownExtensionsDefault().Frontmatter().Plugin(editor.NewFrontmatterPlugin()))
		held, _ := editor.BaseParseMarkdown(cx, "---\ntitle: kept\n---\n\ntext\n", a.ext).Value.Get()
		if len(held) > 0 {
			if a.mode == "ed7-ok" {
				gk.Retain(cx, held[0])
			}
			a.custom = gk.Some(held[0])
		}
	}
}

func (a *App) view7(cx *gk.Cx) gk.Element {
	blocks := []editor.BlockNode{editor.BlockNodeParagraph{Value: editor.NewParagraphDefault().Push(editor.NewInlineNode("mine")).WithLeafKey(7)}}
	if c, ok := a.custom.Get(); ok && (a.mode == "ed7-ok" || a.drawn >= 2) {
		blocks = append(blocks, c)
	}
	flows := make([]gk.Option[editor.FlowPosition], len(blocks))
	for i := range blocks {
		flows[i] = gk.Some(editor.NewFlowPositionAt(blocks, i, i == len(blocks)-1))
	}
	rc := editor.NewRenderContextFromApp().WithMarkdownExtensions(a.ext)
	return gk.VFlex(s.SizeFull, s.P4).Child(editor.BlockHost("doc", blocks, flows, gk.Some(rc), nil, a.leavesLaid))
}

func (a *App) plusRow(cx *gk.Cx, at gk.RowAt) gk.Element {
	a.rowed++
	return kit.Label("+")
}

func (a *App) leavesLaid(cx *gk.Cx, e editor.LeafGeometryEvent) {
	for _, l := range e.Leaves {
		a.leaves += l.Geometry.Len(cx)
		if a.mode == "ed6-leaf-stale" {
			a.leaf = gk.Some(l.Geometry)
		}
	}
}

func (a *App) view6(cx *gk.Cx) gk.Element {
	if g, ok := a.leaf.Get(); ok {
		_ = g.Len(cx)
	}
	if a.mode == "ed6-lent-view" {
		a.ed.LspMut(cx, editor.NewLspDefault())
	}
	blocks := []editor.BlockNode{editor.BlockNodeParagraph{Value: editor.NewParagraphDefault().Push(editor.NewInlineNode("mine")).WithLeafKey(7)}}
	for _, h := range a.held {
		blocks = append(blocks, h)
	}
	return gk.VFlex(s.SizeFull, s.P4).Child(editor.BlockHost("doc", blocks, nil, gk.Option[editor.RenderContext]{}, nil, a.leavesLaid)).
		Child(gk.Div().W(gk.Px(160)).Child(editor.BaseNumberInput(a.inp).IncrementButton(gk.Decorate{Content: a.plus})))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	a.drawn++
	if strings.HasPrefix(a.mode, "ed6-") {
		return a.view6(cx)
	}
	if strings.HasPrefix(a.mode, "ed7-") {
		return a.view7(cx)
	}
	body := gk.VFlex(s.SizeFull, s.P4)
	ext := editor.NewMarkdownExtensionsDefault().BlockSurface("note", gk.Div(s.P2).Child(kit.Label("Note")))
	text := editor.BaseTextViewMarkdown("md", md).MarkdownExtensions(ext).
		Plugin(gk.MarkdownPlugin{Name: "badge", Match: gk.MarkdownMatchCode, Lang: "badge", Rows: a.rows})
	switch a.mode {
	case "ed-ok", "ed-plugin-freed":
		body = body.Child(text).Child(editor.InputBase("ib").SemanticOwner(gk.EntityIDOf(a.ed)))
	case "ed-owner":
		body = body.Child(editor.InputBase("ib").SemanticOwner(gk.EntityIDOf(a.hist)))
	case "ed-owner-released":
		body = body.Child(editor.InputBase("ib").SemanticOwner(gk.EntityIDOf(a.ed)))
	}
	return body
}

func main() {
	a := &App{mode: os.Getenv("GK_NEG")}
	if err := gk.RunHeadless("size 400 300\nframe\nadvance 100\nframe\nframe\nframe\n", func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "misuse_editor", 400, 300, a.root)
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gk-neg done mode=%s drawn=%d rows=%d leaves=%d\n", a.mode, a.drawn, a.rowed, a.leaves)
}
