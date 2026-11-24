package gk

// Syntax highlighters and language providers made of the app's functions, ported from
// odin/gk/kit_editor.odin (docs/PROTOCOLS.md 9.18). The library asks them between frames (asks,
// PROTOCOLS.md 9.14); an editor draws unstyled text until the answer for its text is in.

// HighlightQuery is what a highlighter is asked: the editor's language, its whole text, and the
// byte range to style.
type HighlightQuery struct {
	Language string
	Text     string
	Range    Range
}

// HighlightRun styles a byte range by a semantic name ("keyword", "string", "comment", ..), which
// the editor's resolver turns into a style (the kit's editor: the theme's syntax colors). Text no
// run covers is unstyled.
type HighlightRun struct {
	Range Range
	Name  string
}

// FoldQuery is what a highlighter's folds function is asked: the language and the whole text.
type FoldQuery struct {
	Language string
	Text     string
}

// SyntaxQuery is what a language provider's syntax function is asked: where in the syntax an
// offset of the text sits (for pairing quotes and brackets, indenting).
type SyntaxQuery struct {
	Language string
	Text     string
	Offset   int
}

// LanguageAlias names a language by another name ("rs" for "rust").
type LanguageAlias struct{ Alias, Name string }

func init() {
	regAsk(func(r *reader) HighlightQuery { return HighlightQuery{r.str(), r.str(), rdRange(r)} },
		func(o *tape, runs []HighlightRun) {
			o.u32(uint32(len(runs)))
			for _, x := range runs {
				o.u64(uint64(x.Range.Lo))
				o.u64(uint64(x.Range.Hi))
				o.str(x.Name)
			}
		})
	regAsk(func(r *reader) FoldQuery { return FoldQuery{r.str(), r.str()} },
		func(o *tape, folds []FoldRange) {
			o.u32(uint32(len(folds)))
			for _, f := range folds {
				o.u64(uint64(f.StartLine))
				o.u64(uint64(f.EndLine))
			}
		})
	regAsk(func(r *reader) SyntaxQuery { return SyntaxQuery{r.str(), r.str(), int(r.u64())} },
		func(o *tape, c SyntaxContext) { o.u16(uint16(c)) })
}

// Highlighter is a syntax highlighter made of the app's functions. Give Src where upstream takes
// an InputHighlighterFactory (editor.SetHighlighterFactory(cx, hl.Src)).
type Highlighter struct {
	Src    RowSource
	Styles Ask[HighlightQuery, []HighlightRun]
	Folds  Ask[FoldQuery, []FoldRange]
}

// NewHighlighter makes a highlighter for editors of languages (none: every language) whose runs
// styles gives, and whose fold ranges folds gives (nil: none). In init or a handler; release it
// with gk.Release(cx, hl.Src) once no editor uses it.
func NewHighlighter(cx *Cx, styles func(cx *Cx, q HighlightQuery) []HighlightRun, folds func(cx *Cx, q FoldQuery) []FoldRange,
	languages ...string) Highlighter {
	var h Highlighter
	h.Styles = NewAsk(cx, styles)
	if folds != nil {
		h.Folds = NewAsk(cx, folds)
	}
	h.Src = h.Styles.Src
	o := newReq(cHighlighterNew)
	o.u64(h.Styles.Src.h)
	o.u64(h.Folds.Src.h)
	o.u32(uint32(len(languages)))
	for _, l := range languages {
		o.str(l)
	}
	mustCall(*o)
	return h
}

// NewLanguageProvider makes the app's language service: aliases of language names, and where in
// the syntax an offset sits (syntax; nil: always code). Give the result where upstream takes an
// Rc<dyn LanguageProvider>. In init or a handler.
func NewLanguageProvider(cx *Cx, aliases []LanguageAlias, syntax func(cx *Cx, q SyntaxQuery) SyntaxContext) RowSource {
	var src RowSource
	if syntax != nil {
		src = NewAsk(cx, syntax).Src
	} else {
		src = NewRowSource(cx, 0, func(cx *Cx, at RowAt) Element { return Text("") }, Initial(0), Capacity(16))
	}
	o := newReq(cLanguagesNew)
	o.u64(src.h)
	o.bool(syntax != nil)
	o.u32(uint32(len(aliases)))
	for _, a := range aliases {
		o.str(a.Alias)
		o.str(a.Name)
	}
	mustCall(*o)
	return src
}
