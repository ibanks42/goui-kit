package rt

// HighlightQuery holds the language name, full text, and byte range passed to
// a custom editor syntax highlighter.
type HighlightQuery struct {
	Language string
	Text     string
	Range    Range
}

// HighlightRun associates a byte range with a highlight capture or style name.
type HighlightRun struct {
	Range Range
	Name  string
}

// FoldQuery holds the language name and text passed to a custom code-folding callback.
type FoldQuery struct {
	Language string
	Text     string
}

// SyntaxQuery holds the language name, text, and cursor byte offset passed to a
// syntax-context callback.
type SyntaxQuery struct {
	Language string
	Text     string
	Offset   int
}

// LanguageAlias maps an alias (such as a file extension or short name) to a
// canonical language name.
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

// Highlighter bundles the row-source handle and request channels for a custom
// editor syntax highlighter and optional folder.
type Highlighter struct {
	Src    RowSource
	Styles Ask[HighlightQuery, []HighlightRun]
	Folds  Ask[FoldQuery, []FoldRange]
}

// NewHighlighter registers a custom highlighter for the named languages using
// styles and optional folds callbacks.
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

// NewLanguageProvider creates a language provider source with the given aliases
// and optional syntax-context callback.
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
