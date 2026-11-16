package gk

import "base:runtime"
import "core:fmt"

// The editor family (docs/PROTOCOLS.md sections 9.11 and 9.12):
// values the generator's rules carry that are not upstream records (a font), the editor's
// providers that answer from data or between frames, and the data closures.

// Font is gpui's Font as the binding carries it: a family, a weight (0: normal, 400) and a
// style; its features and fallbacks are gpui's defaults. Used by DisplayMap and prewrap.
Font :: struct {
	family: string,
	weight: f32,
	style:  Font_Style,
}

// font is a font of `family` at the normal weight and style.
font :: proc(family: string, weight: f32 = 0, style := Font_Style.Normal) -> Font {
	return {family = family, weight = weight, style = style}
}

// put_kept_handler writes a handler given to a call op (a state's or an object's method, or a builder of
// its constructor): it is kept, as a subscription's handler is, and the library fires it
// whenever upstream calls the closure (between frames: on input, on a click).
@(private)
put_kept_handler :: proc(o: ^Tape, h: Handler, shape: u16, loc := #caller_location) {
	if cur == nil {
		stop(loc, "a handler can only be given to a call in a turn (init or a handler)")
	}
	slot, gen := keep_handler(&cur.cx, h, shape, loc)
	put_u32(o, slot)
	put_u32(o, gen)
	if kept_pending == nil {
		kept_pending = make([dynamic]u32, 0, 4, runtime.heap_allocator())
	}
	append(&kept_pending, slot)
}

// Handlers given to calls belong to the state or object the call is made on (or makes): they
// are dropped when it is released (gk.release), not kept until the app ends (PROTOCOLS.md 9.13).
// kept_pending holds the slots of the call being written; kept_attach gives them to its handle.
@(private)
kept_pending: [dynamic]u32
@(private)
kept_by: map[u64][dynamic]u32

// kept_attach gives the handlers kept for the call being made to `h` (state_make: the new
// handle; state_set and state_call: the handle the request names after its op).
@(private)
kept_attach :: proc(h: u64) {
	if len(kept_pending) == 0 {
		return
	}
	defer clear(&kept_pending)
	if h == 0 {
		return
	}
	if kept_by == nil {
		kept_by = make(map[u64][dynamic]u32, 8, runtime.heap_allocator())
	}
	list := kept_by[h]
	if list == nil {
		list = make([dynamic]u32, 0, 2, runtime.heap_allocator())
	}
	append(&list, ..kept_pending[:])
	kept_by[h] = list
}

// kept_attach_req is kept_attach for a request that names its handle after the op.
@(private)
kept_attach_req :: proc(req: []u8) {
	if len(kept_pending) == 0 {
		return
	}
	h: u64
	if len(req) >= 10 {
		r := Reader{req, 2}
		h = rd_u64(&r)
	}
	kept_attach(h)
}

// kept_release drops the handlers given to calls on `h` (gk.release of a state or object).
@(private)
kept_release :: proc(h: u64) {
	list, ok := kept_by[h]
	if !ok {
		return
	}
	for slot in list {
		drop_handler(slot)
	}
	delete(list)
	delete_key(&kept_by, h)
}

// test_kept_count is the number of kept handlers alive (subscriptions, handlers given to
// calls, timers): the tests' check that releasing a state drops its handlers.
test_kept_count :: proc() -> int {
	return len(kept) - len(kept_free)
}

// put_expr writes an expression closure (docs/PROTOCOLS.md 9.12): a closure that maps numbers
// to a number, given as an expression the library evaluates (`"x < 10 ? 1 : 10"`), checked
// here: its syntax, its functions and its arguments (x y z or a0 a1 .., at most `nargs`).
@(private)
put_expr :: proc(o: ^Tape, text: string, nargs: int, loc := #caller_location) {
	if msg := expr_check(text, nargs); msg != "" {
		stop(loc, "the expression %q %s", text, msg)
	}
	put_str(o, text)
}

@(private = "file")
Expr_P :: struct {
	s:     string,
	i:     int,
	nargs: int,
	err:   string,
}

@(private = "file")
EXPR_FUNCS := [?]struct {
	name: string,
	n:    int,
}{{"min", 2}, {"max", 2}, {"abs", 1}, {"floor", 1}, {"ceil", 1}, {"round", 1}, {"sqrt", 1}, {"pow", 2}, {"sin", 1}, {"cos", 1}, {"exp", 1}, {"ln", 1}, {"clamp", 3}}

// expr_check returns "" when `text` is an expression of at most nargs arguments, else what is wrong.
expr_check :: proc(text: string, nargs: int) -> string {
	p := Expr_P{s = text, nargs = nargs}
	ex_expr(&p)
	ex_ws(&p)
	if p.err == "" && p.i < len(p.s) {
		p.err = fmt.tprintf("has %q left over at byte %d", p.s[p.i:], p.i)
	}
	return p.err
}

@(private = "file")
ex_ws :: proc(p: ^Expr_P) {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t' || p.s[p.i] == '\n') {
		p.i += 1
	}
}

@(private = "file")
ex_eat :: proc(p: ^Expr_P, t: string) -> bool {
	ex_ws(p)
	if p.i + len(t) <= len(p.s) && p.s[p.i:p.i + len(t)] == t {
		p.i += len(t)
		return true
	}
	return false
}

@(private = "file")
ex_fail :: proc(p: ^Expr_P, what: string) {
	if p.err == "" {
		p.err = fmt.tprintf("does not parse: %s at byte %d", what, p.i)
	}
}

@(private = "file")
ex_expr :: proc(p: ^Expr_P) {
	ex_bin(p, 0)
	if ex_eat(p, "?") {
		ex_expr(p)
		if !ex_eat(p, ":") {
			ex_fail(p, "a ? without its :")
			return
		}
		ex_expr(p)
	}
}

// Binary levels: || && comparisons + - * / %, each a chain of the next.
@(private = "file")
ex_bin :: proc(p: ^Expr_P, level: int) {
	OPS := [5][]string{{"||"}, {"&&"}, {"<=", ">=", "==", "!=", "<", ">"}, {"+", "-"}, {"*", "/", "%"}}
	if level == len(OPS) {
		ex_unary(p)
		return
	}
	ex_bin(p, level + 1)
	for p.err == "" {
		found := false
		for t in OPS[level] {
			if ex_eat(p, t) {
				found = true
				break
			}
		}
		if !found {
			return
		}
		ex_bin(p, level + 1)
		if level == 2 {
			return // one comparison, as the library reads it
		}
	}
}

@(private = "file")
ex_unary :: proc(p: ^Expr_P) {
	ex_ws(p)
	if p.i < len(p.s) && (p.s[p.i] == '-' || (p.s[p.i] == '!' && (p.i + 1 >= len(p.s) || p.s[p.i + 1] != '='))) {
		p.i += 1
		ex_unary(p)
		return
	}
	ex_atom(p)
	if ex_eat(p, "^") {
		ex_unary(p)
	}
}

@(private = "file")
ex_atom :: proc(p: ^Expr_P) {
	ex_ws(p)
	if p.err != "" {
		return
	}
	if p.i >= len(p.s) {
		ex_fail(p, "it ends where a number, an argument or a function was due")
		return
	}
	c := p.s[p.i]
	switch {
	case c == '(':
		p.i += 1
		ex_expr(p)
		if !ex_eat(p, ")") {
			ex_fail(p, "a ( without its )")
		}
	case (c >= '0' && c <= '9') || c == '.':
		for p.i < len(p.s) && ((p.s[p.i] >= '0' && p.s[p.i] <= '9') || p.s[p.i] == '.') {
			p.i += 1
		}
	case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		st := p.i
		for p.i < len(p.s) && ((p.s[p.i] >= 'a' && p.s[p.i] <= 'z') || (p.s[p.i] >= 'A' && p.s[p.i] <= 'Z') || (p.s[p.i] >= '0' && p.s[p.i] <= '9') || p.s[p.i] == '_') {
			p.i += 1
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
		case:
			if len(name) == 2 && name[0] == 'a' && name[1] >= '0' && name[1] <= '9' {
				arg = int(name[1] - '0')
			}
		}
		if arg >= 0 {
			if arg >= p.nargs {
				p.err = fmt.tprintf("uses %s, but the closure is handed %d argument(s)", name, p.nargs)
			}
			return
		}
		for f in EXPR_FUNCS {
			if f.name == name {
				if !ex_eat(p, "(") {
					ex_fail(p, "a function without (")
					return
				}
				n := 0
				if !ex_eat(p, ")") {
					for p.err == "" {
						ex_expr(p)
						n += 1
						if ex_eat(p, ")") {
							break
						}
						if !ex_eat(p, ",") {
							ex_fail(p, "arguments not separated by ,")
							return
						}
					}
				}
				if n != f.n && p.err == "" {
					p.err = fmt.tprintf("calls %s with %d argument(s); it takes %d", name, n, f.n)
				}
				return
			}
		}
		p.err = fmt.tprintf("names %q, which is neither an argument (x y z a0..a9) nor a function", name)
	case:
		ex_fail(p, fmt.tprintf("unexpected %q", p.s[p.i:p.i + 1]))
	}
}

@(private)
put_nonzero :: proc(o: ^Tape, v: u64, size: int, loc := #caller_location) {
	if v == 0 {
		stop(loc, "a count that must not be 0 was given 0")
	}
	switch size {
	case 1:
		put_u8(o, u8(v))
	case 2:
		put_u16(o, u16(v))
	case 4:
		put_u32(o, u32(v))
	case:
		put_u64(o, v)
	}
}

// Objects lent by kit state (docs/PROTOCOLS.md 9.17). A state method that hands out its
// own object to change (`diagnostics_mut`) gives a temporary handle to a copy; the app writes to
// it with the turn's buffered writes, and the library gives the copy back to the state when the
// turn's writes have landed (the state redraws). The handle is released then.
//
//   if d, ok := a.editor->diagnostics_mut(cx).?; ok {
//       d->push(cx, gk.diagnostic(gk.Lsp_Position{0, 4}, gk.Lsp_Position{0, 9}, "unused")->..)
//   }

// test_lends: the library's counts of objects lent and given back (the tests').
test_lends :: proc(cx: ^Cx, loc := #caller_location) -> (lent, given_back: int) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_LEND_TEST)
	r := Reader{must_call(req[:], loc), 0}
	return int(rd_u64(&r)), int(rd_u64(&r))
}

// Delegates answered between frames (docs/PROTOCOLS.md 9.18). A trait upstream calls
// while it draws or edits (the editor's highlighter, the language service) is implemented once by
// the library over procedures of the app, each an ask (9.14): the library answers from a cache,
// shows upstream's default for a frame or two for a question it has not got the answer to, and
// asks the app between frames.

// Highlight_Query is what a highlighter is asked: the editor's language, its whole text, and the
// byte range to style.
Highlight_Query :: struct {
	language: Temp_String,
	text:     Temp_String,
	range:    Range,
}

// Highlight_Run styles a byte range by a semantic name ("keyword", "string", "comment", ..), which
// the editor's resolver turns into a style (the kit's editor: the theme's syntax colours). Text no
// run covers is unstyled.
Highlight_Run :: struct {
	range: Range,
	name:  string,
}

// Fold_Query is what a highlighter's folds procedure is asked: the language and the whole text.
Fold_Query :: struct {
	language: Temp_String,
	text:     Temp_String,
}

// Highlighter is a syntax highlighter made of the app's procedures. Give `src` where upstream takes
// an InputHighlighterFactory (`editor->set_highlighter_factory(cx, hl.src)`).
Highlighter :: struct {
	src:    Row_Source,
	styles: Ask(Highlight_Query, []Highlight_Run),
	folds:  Ask(Fold_Query, []Fold_Range),
}

// highlighter makes a highlighter for editors of `languages` (none: every language) whose runs
// `styles` gives, and whose fold ranges `folds` gives (nil: none). In init or a handler; release
// it with gk.release(cx, hl.src) once no editor uses it.
highlighter :: proc(
	cx: ^Cx,
	state: ^$S,
	styles: proc(s: ^S, cx: ^Cx, q: Highlight_Query) -> []Highlight_Run,
	folds: proc(s: ^S, cx: ^Cx, q: Fold_Query) -> []Fold_Range = nil,
	languages: []string = nil,
	loc := #caller_location,
) -> (h: Highlighter) {
	h.styles = ask(cx, state, styles, loc)
	if folds != nil {
		h.folds = ask(cx, state, folds, loc)
	}
	h.src = h.styles.src
	req := make(Tape, 0, 32, context.temp_allocator)
	put_u16(&req, C_HIGHLIGHTER_NEW)
	put_u64(&req, h.styles.src.h)
	put_u64(&req, h.folds.src.h)
	put_u32(&req, u32(len(languages)))
	for l in languages {
		put_str(&req, l)
	}
	must_call(req[:], loc)
	return
}

// Syntax_Query is what a language provider's syntax procedure is asked: where in the syntax an
// offset of the text sits (for pairing quotes and brackets, indenting).
Syntax_Query :: struct {
	language: Temp_String,
	text:     Temp_String,
	offset:   int,
}

// Language_Alias names a language by another name ("rs" for "rust").
Language_Alias :: struct {
	alias, name: string,
}

// language_provider makes the app's language service: aliases of language names, and where in the
// syntax an offset sits (`syntax`, nil: always code). Give the result where upstream takes an
// Rc<dyn LanguageProvider> (`gk.base_set_language_provider(cx, provider)`). In init or a handler.
language_provider :: proc(
	cx: ^Cx,
	state: ^$S,
	aliases: []Language_Alias = nil,
	syntax: proc(s: ^S, cx: ^Cx, q: Syntax_Query) -> Syntax_Context = nil,
	loc := #caller_location,
) -> Row_Source {
	src: Row_Source
	if syntax != nil {
		src = ask(cx, state, syntax, loc).src
	} else {
		none :: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element {
			return {}
		}
		src = row_source(cx, state, 0, none, initial = 0, capacity = 16, loc = loc)
	}
	req := make(Tape, 0, 32, context.temp_allocator)
	put_u16(&req, C_LANGUAGES_NEW)
	put_u64(&req, src.h)
	put_bool(&req, syntax != nil)
	put_u32(&req, u32(len(aliases)))
	for a in aliases {
		put_str(&req, a.alias)
		put_str(&req, a.name)
	}
	must_call(req[:], loc)
	return src
}
