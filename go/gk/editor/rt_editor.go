// Hand-written code editor element and LSP-style provider request/response
// channel (completions, hover, definitions, document colors, code actions,
// semantic tokens, and inline ghost-text completions).

package editor

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/internal/rt"
)

// CodeEditorEl is the element handle returned by CodeEditor.
type CodeEditorEl struct{ rt.ElCore }

// Style applies nullary style tokens to e.
func (e CodeEditorEl) Style(st ...gk.Style) CodeEditorEl { rt.Tokens(rt.N(e.Core), st); return e }

// W sets the width of e.
func (e CodeEditorEl) W(l gk.Length) CodeEditorEl { tStyledW(rt.N(e.Core), l); return e }

// H sets the height of e.
func (e CodeEditorEl) H(l gk.Length) CodeEditorEl { tStyledH(rt.N(e.Core), l); return e }

// CodeEditorOptions configures height, chrome, disabled, and read-only flags
// for CodeEditor.
type CodeEditorOptions struct {
	Height       float32
	NoBorder     bool
	NoAppearance bool
	Disabled     bool
	ReadOnly     bool
}

// CodeEditor renders a code editor element bound to editor state.
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

// ProviderRequest is implemented by all LSP-style request structs delivered in
// ProviderEvent.Request.
type ProviderRequest interface{ isProviderRequest() }

// ProviderCompletions requests autocomplete items at byte Offset in Text.
type ProviderCompletions struct {
	Text        string
	Offset      int
	TriggerKind uint8
	Trigger     gk.Option[string]
}

// ProviderHover requests hover markdown documentation at byte Offset in Text.
type ProviderHover struct {
	Text   string
	Offset int
}

// ProviderDefinitions requests go-to-definition targets at byte Offset in Text.
type ProviderDefinitions struct {
	Text   string
	Offset int
}

// ProviderColors requests document color swatches for Text.
type ProviderColors struct{ Text string }

// ProviderCodeActions requests available code actions for byte range [Start, End).
type ProviderCodeActions struct{ Start, End int }

// ProviderPerform requests execution of a previously offered code action.
type ProviderPerform struct {
	Title, ID     string
	PushToHistory bool
}

// ProviderTokens requests semantic token data for byte range [Start, End) of Text.
type ProviderTokens struct {
	Text       string
	Start, End int
}

// ProviderInline requests inline ghost-text completion at byte Offset in Text.
type ProviderInline struct {
	Text      string
	Offset    int
	Automatic bool
}

func (ProviderCompletions) isProviderRequest() {}

func (ProviderHover) isProviderRequest() {}

func (ProviderDefinitions) isProviderRequest() {}

func (ProviderColors) isProviderRequest() {}

func (ProviderCodeActions) isProviderRequest() {}

func (ProviderPerform) isProviderRequest() {}

func (ProviderTokens) isProviderRequest() {}

func (ProviderInline) isProviderRequest() {}

// ProviderEvent pairs a correlation ID with an incoming editor ProviderRequest.
// Handlers reply using the matching Answer* function with e.ID.
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

// Provider is a bitmask selecting which language-intelligence providers are
// enabled on an editor State.
type Provider uint32

// Provider capability flags combined in ProvidersOptions.Which.
const (
	ProvideCompletions       Provider = 1
	ProvideHover             Provider = 2
	ProvideDefinitions       Provider = 4
	ProvideDocumentColors    Provider = 8
	ProvideCodeActions       Provider = 16
	ProvideSemanticTokens    Provider = 32
	ProvideInlineCompletions Provider = 64
)

// ProvidersOptions configures enabled provider kinds, completion trigger
// characters, code-action provider ID, inline completion debounce, and semantic
// token legends.
type ProvidersOptions struct {
	Which            Provider
	Triggers         gk.Option[string]
	CodeActionID     string
	InlineDebounceMs int
	TokenTypes       []string
	TokenModifiers   []string
}

// Providers attaches an LSP-style provider handler f to editor and returns the
// backing RowSource handle passed to Answer* replies.
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

// CompletionResponse holds the completion items and incomplete flag returned
// by AnswerCompletions.
type CompletionResponse struct {
	Items      []gk.CompletionItem
	Incomplete bool
}

// AnswerCompletions replies to a ProviderCompletions request.
func AnswerCompletions(cx *gk.Cx, src gk.RowSource, id uint64, c CompletionResponse) {
	var b rt.Tape
	b.U32(uint32(len(c.Items)))
	for _, it := range c.Items {
		rt.PutCompletionItem(&b, it)
	}
	b.Bool(c.Incomplete)
	answerBody(cx, src, id, b)
}

// AnswerHover replies to a ProviderHover request with optional markdown content.
func AnswerHover(cx *gk.Cx, src gk.RowSource, id uint64, markdown gk.Option[string]) {
	var b rt.Tape
	putOptString2(&b, markdown)
	answerBody(cx, src, id, b)
}

// TextRange is a zero-based (line, character) span used by LSP provider replies.
type TextRange struct{ StartLine, StartChar, EndLine, EndChar uint32 }

func putTextRange(o *rt.Tape, r TextRange) {
	o.U32(r.StartLine)
	o.U32(r.StartChar)
	o.U32(r.EndLine)
	o.U32(r.EndChar)
}

// Definition identifies a go-to-definition target URI and ranges.
type Definition struct {
	URI       string
	Target    TextRange
	Selection TextRange
}

// AnswerDefinitions replies to a ProviderDefinitions request.
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

// DocumentColor associates a document TextRange with an RGBA color (0..1 channels).
type DocumentColor struct {
	Range TextRange
	RGBA  [4]float32
}

// AnswerColors replies to a ProviderColors request.
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

// CodeAction describes one actionable quick-fix or refactoring entry.
type CodeAction struct{ Title, Kind, ID string }

// AnswerCodeActions replies to a ProviderCodeActions request.
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

// AnswerPerformed acknowledges completion of a ProviderPerform request.
func AnswerPerformed(cx *gk.Cx, src gk.RowSource, id uint64) {
	var b rt.Tape
	b.Bool(true)
	answerBody(cx, src, id, b)
}

// AnswerTokens replies to a ProviderTokens request with LSP-encoded semantic
// token integers.
func AnswerTokens(cx *gk.Cx, src gk.RowSource, id uint64, data []uint32) {
	var b rt.Tape
	b.U32(uint32(len(data)))
	for _, x := range data {
		b.U32(x)
	}
	answerBody(cx, src, id, b)
}

// AnswerInline replies to a ProviderInline request with optional ghost text.
func AnswerInline(cx *gk.Cx, src gk.RowSource, id uint64, text gk.Option[string]) {
	var b rt.Tape
	putOptString2(&b, text)
	answerBody(cx, src, id, b)
}
