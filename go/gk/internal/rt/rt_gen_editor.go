package rt

import (
	"fmt"
)

// Font is gpui's Font as the binding carries it: a family, a weight (0: normal, 400) and a style;
// its features and fallbacks are gpui's defaults. DisplayMap and Prewrap take one.
type Font struct {
	Family string
	Weight float32
	Style  FontStyle
}

// NewFont is a font of family at the normal weight and style.
func NewFont(family string) Font { return Font{Family: family} }

func PutFont(o *Tape, f Font) {
	o.Str(f.Family)
	o.F32(f.Weight)
	o.U16(uint16(f.Style))
}

// putExpr writes an expression closure (PROTOCOLS.md 9.12): a closure that maps numbers to a
// number, given as an expression the library evaluates ("x < 10 ? 1 : 10"), checked here: its
// syntax, its functions and its arguments (x y z or a0 a1 .., at most nargs).
func PutExpr(o *Tape, text string, nargs int) {
	if msg := ExprCheck(text, nargs); msg != "" {
		Stop("the expression %q %s", text, msg)
	}
	o.Str(text)
}

type exprP struct {
	s     string
	i     int
	nargs int
	err   string
}

var exprFuncs = []struct {
	name string
	n    int
}{{"min", 2}, {"max", 2}, {"abs", 1}, {"floor", 1}, {"ceil", 1}, {"round", 1}, {"sqrt", 1}, {"pow", 2},
	{"sin", 1}, {"cos", 1}, {"exp", 1}, {"ln", 1}, {"clamp", 3}}

// ExprCheck returns "" when text is an expression of at most nargs arguments, else what is wrong.
func ExprCheck(text string, nargs int) string {
	p := &exprP{s: text, nargs: nargs}
	p.expr()
	p.ws()
	if p.err == "" && p.i < len(p.s) {
		p.err = fmt.Sprintf("has %q left over at byte %d", p.s[p.i:], p.i)
	}
	return p.err
}

func (p *exprP) ws() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t' || p.s[p.i] == '\n') {
		p.i++
	}
}

func (p *exprP) eat(t string) bool {
	p.ws()
	if p.i+len(t) <= len(p.s) && p.s[p.i:p.i+len(t)] == t {
		p.i += len(t)
		return true
	}
	return false
}

func (p *exprP) fail(what string) {
	if p.err == "" {
		p.err = fmt.Sprintf("does not parse: %s at byte %d", what, p.i)
	}
}

func (p *exprP) expr() {
	p.bin(0)
	if p.eat("?") {
		p.expr()
		if !p.eat(":") {
			p.fail("a ? without its :")
			return
		}
		p.expr()
	}
}

var exprOps = [5][]string{{"||"}, {"&&"}, {"<=", ">=", "==", "!=", "<", ">"}, {"+", "-"}, {"*", "/", "%"}}

// bin: binary levels || && comparisons + - * / %, each a chain of the next.
func (p *exprP) bin(level int) {
	if level == len(exprOps) {
		p.unary()
		return
	}
	p.bin(level + 1)
	for p.err == "" {
		found := false
		for _, t := range exprOps[level] {
			if p.eat(t) {
				found = true
				break
			}
		}
		if !found {
			return
		}
		p.bin(level + 1)
		if level == 2 {
			return // one comparison, as the library reads it
		}
	}
}

func (p *exprP) unary() {
	p.ws()
	if p.i < len(p.s) && (p.s[p.i] == '-' || (p.s[p.i] == '!' && (p.i+1 >= len(p.s) || p.s[p.i+1] != '='))) {
		p.i++
		p.unary()
		return
	}
	p.atom()
	if p.eat("^") {
		p.unary()
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func (p *exprP) atom() {
	p.ws()
	if p.err != "" {
		return
	}
	if p.i >= len(p.s) {
		p.fail("it ends where a number, an argument or a function was due")
		return
	}
	c := p.s[p.i]
	switch {
	case c == '(':
		p.i++
		p.expr()
		if !p.eat(")") {
			p.fail("a ( without its )")
		}
	case isDigit(c) || c == '.':
		for p.i < len(p.s) && (isDigit(p.s[p.i]) || p.s[p.i] == '.') {
			p.i++
		}
	case isAlpha(c):
		st := p.i
		for p.i < len(p.s) && (isAlpha(p.s[p.i]) || isDigit(p.s[p.i]) || p.s[p.i] == '_') {
			p.i++
		}
		name := p.s[st:p.i]
		arg := -1
		switch name {
		case "x":
			arg = 0
		case "y":
			arg = 1
		case "z":
			arg = 2
		default:
			if len(name) == 2 && name[0] == 'a' && isDigit(name[1]) {
				arg = int(name[1] - '0')
			}
		}
		if arg >= 0 {
			if arg >= p.nargs {
				p.err = fmt.Sprintf("uses %s, but the closure is handed %d argument(s)", name, p.nargs)
			}
			return
		}
		for _, f := range exprFuncs {
			if f.name != name {
				continue
			}
			if !p.eat("(") {
				p.fail("a function without (")
				return
			}
			n := 0
			if !p.eat(")") {
				for p.err == "" {
					p.expr()
					n++
					if p.eat(")") {
						break
					}
					if !p.eat(",") {
						p.fail("arguments not separated by ,")
						return
					}
				}
			}
			if n != f.n && p.err == "" {
				p.err = fmt.Sprintf("calls %s with %d argument(s); it takes %d", name, n, f.n)
			}
			return
		}
		p.err = fmt.Sprintf("names %q, which is neither an argument (x y z a0..a9) nor a function", name)
	default:
		p.fail(fmt.Sprintf("unexpected %q", p.s[p.i:p.i+1]))
	}
}

// MarkdownMatch is the kind of Markdown node a data plugin claims. The first six are blocks, the
// rest inline. Code and the JSX elements also match on Lang: a fenced code block's language, an
// element's name ("" matches every one).
type MarkdownMatch uint8

const (
	MarkdownMatchCode MarkdownMatch = iota
	MarkdownMatchMath
	MarkdownMatchYaml
	MarkdownMatchToml
	MarkdownMatchHTML
	MarkdownMatchMdxElement
	MarkdownMatchInlineCode
	MarkdownMatchInlineMath
	MarkdownMatchInlineHTML
	MarkdownMatchMdxInlineElement
)

// MarkdownPlugin is a data plugin: its name (the node's name), the nodes it claims, the row source
// that draws them, and an inline node's baseline in pixels from its top (0: upstream's). The library
// claims the nodes of the rule while upstream parses and draws each node as a row of Rows: a block's
// row reads a gk.MarkdownNodeRead, an inline node's a gk.MarkdownNodeRead and a
// gk.InlineRenderContext (make Rows with gk.Items).
type MarkdownPlugin struct {
	Name     string
	Match    MarkdownMatch
	Lang     string
	Rows     RowSource
	Baseline float32
}

// MarkdownPluginAny is what Plugin takes: one of the kit's plugins (gk.NewFrontmatterPlugin()) or
// a data plugin (gk.MarkdownPlugin).
type MarkdownPluginAny interface{ isMarkdownPluginAny() }

func (MarkdownPlugin) isMarkdownPluginAny() {}

func PutMarkdownPlugin(o *Tape, p MarkdownPlugin) {
	o.Str(p.Name)
	o.U8(uint8(p.Match))
	o.Str(p.Lang)
	o.U64(p.Rows.h)
	o.F32(p.Baseline)
}

// EntityID is the entity behind an input, textarea or editor handle, where upstream takes a
// gpui::EntityId (InputBase's SemanticOwner): gk.EntityIDOf(a.editor).
type EntityID uint64

// EntityIDOf is the entity of a state handle.
func EntityIDOf(state Handled) EntityID { return EntityID(state.handle()) }

// PutDecorate writes a gk.Decorate (package gk): its style refinement, if any, and its content.
func PutDecorate[V value](o *Tape, style Option[V], content RowSource) {
	if s, ok := style.Get(); ok {
		o.U8(1)
		o.U32(TakeValue(s.valueNode()))
	} else {
		o.U8(0)
	}
	o.U64(content.h)
}
