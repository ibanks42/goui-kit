package editor

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
)

// CodeEditorEl is the kit's styled code editor (gpui_kit::component::input::Editor): what the
// generated base editor is plus the border, the size and the menus of the providers. Upstream
// declares it in a doc(hidden) module, so the generator has no item for it; this is its
// hand-written node (core/src/shim_input.rs).
type CodeEditorEl struct{ rt.ElCore }

// Style adds style tokens (s.SizeFull, s.H64).
func (e CodeEditorEl) Style(st ...gk.Style) CodeEditorEl { rt.Tokens(rt.N(e.Core), st); return e }

// W sets the width.
func (e CodeEditorEl) W(l gk.Length) CodeEditorEl { tStyledW(rt.N(e.Core), l); return e }

// H sets the height.
func (e CodeEditorEl) H(l gk.Length) CodeEditorEl { tStyledH(rt.N(e.Core), l); return e }

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
func CodeEditor(state State, opts CodeEditorOptions, st ...gk.Style) CodeEditorEl {
	n := rt.NewNode(rt.NInputEditor)
	o := rt.NodeArgs(n)
	o.U64(rt.H(state.HCore))
	o.F32(opts.Height)
	var flags uint8
	for i, b := range []bool{!opts.NoBorder, !opts.NoAppearance, opts.Disabled, opts.ReadOnly} {
		if b {
			flags |= 1 << i
		}
	}
	o.U8(flags)
	rt.Tokens(n, st)
	return CodeEditorEl{ElCore: rt.MkElCore(n)}
}

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
	Trigger     gk.Option[string]
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
// editor.Providers.
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

func (ProviderHover) isProviderRequest() {}

func (ProviderDefinitions) isProviderRequest() {}

func (ProviderColors) isProviderRequest() {}

func (ProviderCodeActions) isProviderRequest() {}

func (ProviderPerform) isProviderRequest() {}

func (ProviderTokens) isProviderRequest() {}

func (ProviderInline) isProviderRequest() {}

// ProviderEvent is what a provider handler receives: the request id to answer and the question.
type ProviderEvent struct {
	ID      uint64
	Request ProviderRequest
}

func rdProviderRequest(r *rt.Reader) ProviderRequest {
	switch n := r.U16(); n {
	case 0:
		q := ProviderCompletions{Text: r.Str(), Offset: int(r.U64()), TriggerKind: r.U8()}
		q.Trigger = rt.RdOpt(r, func(r *rt.Reader) string { return r.Str() })
		return q
	case 1:
		return ProviderHover{r.Str(), int(r.U64())}
	case 2:
		return ProviderDefinitions{r.Str(), int(r.U64())}
	case 3:
		return ProviderColors{r.Str()}
	case 4:
		return ProviderCodeActions{int(r.U64()), int(r.U64())}
	case 5:
		return ProviderPerform{r.Str(), r.Str(), r.Bool()}
	case 6:
		return ProviderTokens{r.Str(), int(r.U64()), int(r.U64())}
	case 7:
		return ProviderInline{r.Str(), int(r.U64()), r.Bool()}
	default:
		rt.Stop("a provider request arrived with variant %d of 8: the library and this package disagree", n)
	}
	return nil
}

func init() {
	rt.RegisterShape(rt.ShapeProvider, rt.ShapeInfo{Name: "func(cx *Cx, e ProviderEvent)", Call: func(h any, cx *gk.Cx, r *rt.Reader, ret *rt.Tape) bool {
		f, ok := h.(func(*gk.Cx, ProviderEvent))
		if ok {
			id := r.U64()
			f(cx, ProviderEvent{id, rdProviderRequest(r)})
		}
		return ok
	}})
}

// EditorProvider is a set of the providers editor.Providers installs.
type Provider uint32

const (
	ProvideCompletions       Provider = 1
	ProvideHover             Provider = 2
	ProvideDefinitions       Provider = 4
	ProvideDocumentColors    Provider = 8
	ProvideCodeActions       Provider = 16
	ProvideSemanticTokens    Provider = 32
	ProvideInlineCompletions Provider = 64
)

// ProvidersOptions are what editor.Providers installs; the zero value is completions and hover,
// "." as the trigger, "gk" as the code action provider's id, upstream's inline debounce, no legend.
type ProvidersOptions struct {
	Which            Provider
	Triggers         gk.Option[string] // the characters that ask for completions (default ".")
	CodeActionID     string            // default "gk"
	InlineDebounceMs int               // 0: upstream's 300 ms
	TokenTypes       []string          // the semantic tokens' legend
	TokenModifiers   []string
}

// EditorProviders installs the providers of an EditorState: f receives every question (a queued
// event, after the editor's own update) and answers it with the gk.Answer* of its kind. Returns
// the row source that carries the questions; releasing it ends them. Needs a window: init or a
// handler.
func Providers(cx *gk.Cx, editor State, f func(cx *gk.Cx, e ProviderEvent), opts ProvidersOptions) gk.RowSource {
	if f == nil {
		rt.Stop("editor.Providers needs a handler")
	}
	src := gk.NewRowSource(cx, 0, func(cx *gk.Cx, at gk.RowAt) gk.Element { return gk.Text("") }, rt.SourceEvents(f, rt.ShapeProvider),
		gk.Initial(0), gk.Capacity(16))
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
	o := rt.NewReq(rt.CEditorProviders)
	o.U64(rt.H(editor.HCore))
	o.U64(rt.H(src.HCore))
	o.U32(uint32(which))
	o.Str(triggers)
	o.Str(id)
	o.U32(uint32(max(opts.InlineDebounceMs, 0)))
	for _, list := range [2][]string{opts.TokenTypes, opts.TokenModifiers} {
		o.U32(uint32(len(list)))
		for _, s := range list {
			o.Str(s)
		}
	}
	rt.StateSet(cx, o)
	return src
}

// answerBody answers request id of src with an encoded body (buffered).
func answerBody(cx *gk.Cx, src gk.RowSource, id uint64, body rt.Tape) {
	t := rt.CheckCx(cx)
	o := rt.NewReq(rt.CSourceAnswer)
	o.U64(rt.H(src.HCore))
	o.U64(id)
	o.Bytes(body)
	t.DeferOp(*o)
}

func putOptString2(o *rt.Tape, s gk.Option[string]) {
	if v, ok := s.Get(); ok {
		o.U8(1)
		o.Str(v)
		return
	}
	o.U8(0)
}

// CompletionResponse is the answer to a ProviderCompletions.
type CompletionResponse struct {
	Items      []gk.CompletionItem
	Incomplete bool
}

// AnswerCompletions answers a ProviderCompletions.
func AnswerCompletions(cx *gk.Cx, src gk.RowSource, id uint64, c CompletionResponse) {
	var b rt.Tape
	b.U32(uint32(len(c.Items)))
	for _, it := range c.Items {
		rt.PutCompletionItem(&b, it)
	}
	b.Bool(c.Incomplete)
	answerBody(cx, src, id, b)
}

// AnswerHover answers a ProviderHover with its markdown, or none for no hover.
func AnswerHover(cx *gk.Cx, src gk.RowSource, id uint64, markdown gk.Option[string]) {
	var b rt.Tape
	putOptString2(&b, markdown)
	answerBody(cx, src, id, b)
}

// TextRange is an LSP range: lines and UTF-16 characters from 0.
type TextRange struct{ StartLine, StartChar, EndLine, EndChar uint32 }

func putTextRange(o *rt.Tape, r TextRange) {
	o.U32(r.StartLine)
	o.U32(r.StartChar)
	o.U32(r.EndLine)
	o.U32(r.EndChar)
}

// Definition is one place a symbol is defined.
type Definition struct {
	URI       string
	Target    TextRange // the whole definition
	Selection TextRange // its name
}

// AnswerDefinitions answers a ProviderDefinitions.
func AnswerDefinitions(cx *gk.Cx, src gk.RowSource, id uint64, defs []Definition) {
	var b rt.Tape
	b.U32(uint32(len(defs)))
	for _, d := range defs {
		b.Str(d.URI)
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
func AnswerColors(cx *gk.Cx, src gk.RowSource, id uint64, colors []DocumentColor) {
	var b rt.Tape
	b.U32(uint32(len(colors)))
	for _, c := range colors {
		putTextRange(&b, c.Range)
		for _, x := range c.RGBA {
			b.F32(x)
		}
	}
	answerBody(cx, src, id, b)
}

// CodeAction is one action offered: its title, its LSP kind ("quickfix", "refactor", or empty)
// and an ID that comes back in ProviderPerform.
type CodeAction struct{ Title, Kind, ID string }

// AnswerCodeActions answers a ProviderCodeActions.
func AnswerCodeActions(cx *gk.Cx, src gk.RowSource, id uint64, actions []CodeAction) {
	var b rt.Tape
	b.U32(uint32(len(actions)))
	for _, a := range actions {
		b.Str(a.Title)
		b.Str(a.Kind)
		b.Str(a.ID)
	}
	answerBody(cx, src, id, b)
}

// AnswerPerformed answers a ProviderPerform once the app has edited.
func AnswerPerformed(cx *gk.Cx, src gk.RowSource, id uint64) {
	var b rt.Tape
	b.Bool(true)
	answerBody(cx, src, id, b)
}

// AnswerTokens answers a ProviderTokens with LSP's encoded tokens, five numbers each.
func AnswerTokens(cx *gk.Cx, src gk.RowSource, id uint64, data []uint32) {
	var b rt.Tape
	b.U32(uint32(len(data)))
	for _, x := range data {
		b.U32(x)
	}
	answerBody(cx, src, id, b)
}

// AnswerInline answers a ProviderInline with the text to insert, or none.
func AnswerInline(cx *gk.Cx, src gk.RowSource, id uint64, text gk.Option[string]) {
	var b rt.Tape
	putOptString2(&b, text)
	answerBody(cx, src, id, b)
}
