package gk

// The editor's hand-written pieces: the kit's styled code
// editor (and, below, the providers an editor asks; docs/PROTOCOLS.md 9.6 and 9.11).

// CodeEditorEl is the kit's styled code editor (gpui_kit::component::input::Editor): what the
// generated base editor is plus the border, the size and the menus of the providers. Upstream
// declares it in a doc(hidden) module, so the generator has no item for it; this is its
// hand-written node (core/src/shim_input.rs).
type CodeEditorEl struct{ n *node }

func (e CodeEditorEl) node() *node { return e.n }

// Style adds style tokens (s.SizeFull, s.H64).
func (e CodeEditorEl) Style(st ...Style) CodeEditorEl { tokens(e.n, st); return e }

// W sets the width.
func (e CodeEditorEl) W(l Length) CodeEditorEl { tStyledW(e.n, l); return e }

// H sets the height.
func (e CodeEditorEl) H(l Length) CodeEditorEl { tStyledH(e.n, l); return e }

// CodeEditorOptions are a code editor's looks; the zero value is upstream's (bordered, with the
// kit's appearance, enabled, editable, the height left to the layout).
type CodeEditorOptions struct {
	Height       float32 // 0 leaves the height to the layout
	NoBorder     bool
	NoAppearance bool
	Disabled     bool
	ReadOnly     bool
}

// CodeEditor shows an EditorState as the kit's code editor.
//
//	gk.CodeEditor(a.editor, gk.CodeEditorOptions{Height: 320}, s.WFull)
func CodeEditor(state EditorState, opts CodeEditorOptions, st ...Style) CodeEditorEl {
	n := newNode(nInputEditor)
	o := nodeArgs(n)
	o.u64(state.h)
	o.f32(opts.Height)
	var flags uint8
	for i, b := range []bool{!opts.NoBorder, !opts.NoAppearance, opts.Disabled, opts.ReadOnly} {
		if b {
			flags |= 1 << i
		}
	}
	o.u8(flags)
	tokens(n, st)
	return CodeEditorEl{n}
}

// --- the editor's providers (docs/PROTOCOLS.md 9.6, 9.11) ----------------------------------------
//
// What an app implements upstream (completion, hover, definitions, document colors, code actions,
// semantic tokens, inline completions) is asked of the app as questions through a row source's
// request channel; the app answers each with the gk.Answer* of its kind, at once or later.
//
//	a.providers = gk.EditorProviders(cx, a.editor, a.ask, gk.ProvidersOptions{Which: gk.ProvideCompletions | gk.ProvideHover})
//	func (a *App) ask(cx *gk.Cx, e gk.ProviderEvent) {
//		switch q := e.Request.(type) {
//		case gk.ProviderCompletions:
//			gk.AnswerCompletions(cx, a.providers, e.ID, gk.CompletionResponse{Items: a.complete(q.Text, q.Offset)})
//		case gk.ProviderHover:
//			gk.AnswerHover(cx, a.providers, e.ID, gk.Some("**doc**"))
//		}
//	}

// ProviderRequest is one of the questions: ProviderCompletions, ProviderHover,
// ProviderDefinitions, ProviderColors, ProviderCodeActions, ProviderPerform, ProviderTokens,
// ProviderInline.
type ProviderRequest interface{ isProviderRequest() }

// ProviderCompletions asks for the completions at Offset (bytes) of Text, the editor's whole text.
// TriggerKind is LSP's: 1 invoked, 2 a trigger character, 3 an incomplete list asked again.
// Answer with gk.AnswerCompletions.
type ProviderCompletions struct {
	Text        string
	Offset      int
	TriggerKind uint8
	Trigger     Option[string]
}

// ProviderHover asks for the hover at Offset of Text. Answer with gk.AnswerHover.
type ProviderHover struct {
	Text   string
	Offset int
}

// ProviderDefinitions asks where the symbol at Offset is defined. Answer with gk.AnswerDefinitions.
type ProviderDefinitions struct {
	Text   string
	Offset int
}

// ProviderColors asks for the colors written in Text. Answer with gk.AnswerColors.
type ProviderColors struct{ Text string }

// ProviderCodeActions asks for the code actions of bytes Start to End. Answer with
// gk.AnswerCodeActions.
type ProviderCodeActions struct{ Start, End int }

// ProviderPerform asks the app to perform a code action it offered (its ID); the app edits the
// editor with its generated writes and answers gk.AnswerPerformed when done.
type ProviderPerform struct {
	Title, ID     string
	PushToHistory bool
}

// ProviderTokens asks for the semantic tokens of bytes Start to End. Answer with
// gk.AnswerTokens: LSP's encoded tokens, five numbers each, named by the legend given to
// gk.EditorProviders.
type ProviderTokens struct {
	Text       string
	Start, End int
}

// ProviderInline asks for an inline completion at Offset. Answer with gk.AnswerInline.
type ProviderInline struct {
	Text      string
	Offset    int
	Automatic bool // typed (true) or asked for (false)
}

func (ProviderCompletions) isProviderRequest() {}
func (ProviderHover) isProviderRequest()       {}
func (ProviderDefinitions) isProviderRequest() {}
func (ProviderColors) isProviderRequest()      {}
func (ProviderCodeActions) isProviderRequest() {}
func (ProviderPerform) isProviderRequest()     {}
func (ProviderTokens) isProviderRequest()      {}
func (ProviderInline) isProviderRequest()      {}

// ProviderEvent is what a provider handler receives: the request id to answer and the question.
type ProviderEvent struct {
	ID      uint64
	Request ProviderRequest
}

func rdProviderRequest(r *reader) ProviderRequest {
	switch n := r.u16(); n {
	case 0:
		q := ProviderCompletions{Text: r.str(), Offset: int(r.u64()), TriggerKind: r.u8()}
		q.Trigger = rdOpt(r, func(r *reader) string { return r.str() })
		return q
	case 1:
		return ProviderHover{r.str(), int(r.u64())}
	case 2:
		return ProviderDefinitions{r.str(), int(r.u64())}
	case 3:
		return ProviderColors{r.str()}
	case 4:
		return ProviderCodeActions{int(r.u64()), int(r.u64())}
	case 5:
		return ProviderPerform{r.str(), r.str(), r.bool()}
	case 6:
		return ProviderTokens{r.str(), int(r.u64()), int(r.u64())}
	case 7:
		return ProviderInline{r.str(), int(r.u64()), r.bool()}
	default:
		stop("a provider request arrived with variant %d of 8: the library and this package disagree", n)
	}
	return nil
}

func init() {
	registerShape(shapeProvider, shapeInfo{"func(cx *Cx, e ProviderEvent)", func(h any, cx *Cx, r *reader, ret *tape) bool {
		f, ok := h.(func(*Cx, ProviderEvent))
		if ok {
			id := r.u64()
			f(cx, ProviderEvent{id, rdProviderRequest(r)})
		}
		return ok
	}})
}

// EditorProvider is a set of the providers gk.EditorProviders installs.
type EditorProvider uint32

const (
	ProvideCompletions       EditorProvider = 1
	ProvideHover             EditorProvider = 2
	ProvideDefinitions       EditorProvider = 4
	ProvideDocumentColors    EditorProvider = 8
	ProvideCodeActions       EditorProvider = 16
	ProvideSemanticTokens    EditorProvider = 32
	ProvideInlineCompletions EditorProvider = 64
)

// ProvidersOptions are what gk.EditorProviders installs; the zero value is completions and hover,
// "." as the trigger, "gk" as the code action provider's id, upstream's inline debounce, no legend.
type ProvidersOptions struct {
	Which            EditorProvider
	Triggers         Option[string] // the characters that ask for completions (default ".")
	CodeActionID     string         // default "gk"
	InlineDebounceMs int            // 0: upstream's 300 ms
	TokenTypes       []string       // the semantic tokens' legend
	TokenModifiers   []string
}

// EditorProviders installs the providers of an EditorState: f receives every question (a queued
// event, after the editor's own update) and answers it with the gk.Answer* of its kind. Returns
// the row source that carries the questions; releasing it ends them. Needs a window: init or a
// handler.
func EditorProviders(cx *Cx, editor EditorState, f func(cx *Cx, e ProviderEvent), opts ProvidersOptions) RowSource {
	if f == nil {
		stop("gk.EditorProviders needs a handler")
	}
	src := NewRowSource(cx, 0, func(cx *Cx, at RowAt) Element { return Text("") }, sourceEvents(f, shapeProvider),
		Initial(0), Capacity(16))
	which := opts.Which
	if which == 0 {
		which = ProvideCompletions | ProvideHover
	}
	triggers := "."
	if t, ok := opts.Triggers.Get(); ok {
		triggers = t
	}
	id := opts.CodeActionID
	if id == "" {
		id = "gk"
	}
	o := newReq(cEditorProviders)
	o.u64(editor.h)
	o.u64(src.h)
	o.u32(uint32(which))
	o.str(triggers)
	o.str(id)
	o.u32(uint32(max(opts.InlineDebounceMs, 0)))
	for _, list := range [2][]string{opts.TokenTypes, opts.TokenModifiers} {
		o.u32(uint32(len(list)))
		for _, s := range list {
			o.str(s)
		}
	}
	stateSet(cx, o)
	return src
}

// answerBody answers request id of src with an encoded body (buffered).
func answerBody(cx *Cx, src RowSource, id uint64, body tape) {
	t := checkCx(cx)
	o := newReq(cSourceAnswer)
	o.u64(src.h)
	o.u64(id)
	o.bytes(body)
	t.deferOp(*o)
}

func putOptString2(o *tape, s Option[string]) {
	if v, ok := s.Get(); ok {
		o.u8(1)
		o.str(v)
		return
	}
	o.u8(0)
}

// CompletionResponse is the answer to a ProviderCompletions.
type CompletionResponse struct {
	Items      []CompletionItem
	Incomplete bool
}

// AnswerCompletions answers a ProviderCompletions.
func AnswerCompletions(cx *Cx, src RowSource, id uint64, c CompletionResponse) {
	var b tape
	b.u32(uint32(len(c.Items)))
	for _, it := range c.Items {
		putCompletionItem(&b, it)
	}
	b.bool(c.Incomplete)
	answerBody(cx, src, id, b)
}

// AnswerHover answers a ProviderHover with its markdown, or none for no hover.
func AnswerHover(cx *Cx, src RowSource, id uint64, markdown Option[string]) {
	var b tape
	putOptString2(&b, markdown)
	answerBody(cx, src, id, b)
}

// TextRange is an LSP range: lines and UTF-16 characters from 0.
type TextRange struct{ StartLine, StartChar, EndLine, EndChar uint32 }

func putTextRange(o *tape, r TextRange) {
	o.u32(r.StartLine)
	o.u32(r.StartChar)
	o.u32(r.EndLine)
	o.u32(r.EndChar)
}

// Definition is one place a symbol is defined.
type Definition struct {
	URI       string
	Target    TextRange // the whole definition
	Selection TextRange // its name
}

// AnswerDefinitions answers a ProviderDefinitions.
func AnswerDefinitions(cx *Cx, src RowSource, id uint64, defs []Definition) {
	var b tape
	b.u32(uint32(len(defs)))
	for _, d := range defs {
		b.str(d.URI)
		putTextRange(&b, d.Target)
		putTextRange(&b, d.Selection)
	}
	answerBody(cx, src, id, b)
}

// DocumentColor is a color written in the text, red green blue alpha from 0 to 1.
type DocumentColor struct {
	Range TextRange
	RGBA  [4]float32
}

// AnswerColors answers a ProviderColors.
func AnswerColors(cx *Cx, src RowSource, id uint64, colors []DocumentColor) {
	var b tape
	b.u32(uint32(len(colors)))
	for _, c := range colors {
		putTextRange(&b, c.Range)
		for _, x := range c.RGBA {
			b.f32(x)
		}
	}
	answerBody(cx, src, id, b)
}

// CodeAction is one action offered: its title, its LSP kind ("quickfix", "refactor", or empty)
// and an ID that comes back in ProviderPerform.
type CodeAction struct{ Title, Kind, ID string }

// AnswerCodeActions answers a ProviderCodeActions.
func AnswerCodeActions(cx *Cx, src RowSource, id uint64, actions []CodeAction) {
	var b tape
	b.u32(uint32(len(actions)))
	for _, a := range actions {
		b.str(a.Title)
		b.str(a.Kind)
		b.str(a.ID)
	}
	answerBody(cx, src, id, b)
}

// AnswerPerformed answers a ProviderPerform once the app has edited.
func AnswerPerformed(cx *Cx, src RowSource, id uint64) {
	var b tape
	b.bool(true)
	answerBody(cx, src, id, b)
}

// AnswerTokens answers a ProviderTokens with LSP's encoded tokens, five numbers each.
func AnswerTokens(cx *Cx, src RowSource, id uint64, data []uint32) {
	var b tape
	b.u32(uint32(len(data)))
	for _, x := range data {
		b.u32(x)
	}
	answerBody(cx, src, id, b)
}

// AnswerInline answers a ProviderInline with the text to insert, or none.
func AnswerInline(cx *Cx, src RowSource, id uint64, text Option[string]) {
	var b tape
	putOptString2(&b, text)
	answerBody(cx, src, id, b)
}
