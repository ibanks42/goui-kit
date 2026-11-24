package rt

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
	RegAsk(func(r *Reader) HighlightQuery { return HighlightQuery{r.Str(), r.Str(), RdRange(r)} },
		func(o *Tape, runs []HighlightRun) {
			o.U32(uint32(len(runs)))
			for _, x := range runs {
				o.U64(uint64(x.Range.Lo))
				o.U64(uint64(x.Range.Hi))
				o.Str(x.Name)
			}
		})
	RegAsk(func(r *Reader) FoldQuery { return FoldQuery{r.Str(), r.Str()} },
		func(o *Tape, folds []FoldRange) {
			o.U32(uint32(len(folds)))
			for _, f := range folds {
				o.U64(uint64(f.StartLine))
				o.U64(uint64(f.EndLine))
			}
		})
	RegAsk(func(r *Reader) SyntaxQuery { return SyntaxQuery{r.Str(), r.Str(), int(r.U64())} },
		func(o *Tape, c SyntaxContext) { o.U16(uint16(c)) })
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
	o := NewReq(CHighlighterNew)
	o.U64(h.Styles.Src.h)
	o.U64(h.Folds.Src.h)
	o.U32(uint32(len(languages)))
	for _, l := range languages {
		o.Str(l)
	}
	MustCall(*o)
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
	o := NewReq(CLanguagesNew)
	o.U64(src.h)
	o.Bool(syntax != nil)
	o.U32(uint32(len(aliases)))
	for _, a := range aliases {
		o.Str(a.Alias)
		o.Str(a.Name)
	}
	MustCall(*o)
	return src
}
