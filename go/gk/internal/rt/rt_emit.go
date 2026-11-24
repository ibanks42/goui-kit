package rt

import (
	"fmt"
	"runtime"
	"time"
)

// node is one element or value being described: a node of a view build (or of the value build of
// a turn, for values made outside a view). Element types hold a pointer to theirs, so an element
// is one word and giving it to Child does not allocate. A node of an earlier build is caught by
// its serial. Pointer-free so the slab is noscan for the Go GC.
type Node struct {
	serial            uint32
	idx               uint32
	args, first, last uint32
	typ               uint16
	attached          bool
	valBuild          bool
}

func (n *Node) build() *build {
	if n.valBuild {
		return &valb
	}
	return &Vb
}

// madeAt is where an element was made (recorded with GK_DEBUG=1 only, in a side map so Node stays pointer-free).
type madeAt struct {
	file string
	line int
}

var madeMap map[*Node]madeAt

// Element is an element being described (gk.Div(), kit.Button("ok"), ...).
type Element interface{ node() *Node }

// value is a builder value that is not an element (a column, a style refinement, a menu item).
type value interface{ valueNode() *Node }

// Core is the node a generated element or value holds. Every generated node type of every package
// embeds one of the bases (gen_bases.go), which embed ElCore (an element) or ValCore (a value):
// the struct stays one pointer, so giving it to an interface does not allocate, and the bases
// carry the unexported methods of the interfaces their types are (Element, AnimatableElement, ...).
type Core struct{ n *Node }

// ElCore is an element's core: its node() makes the type an Element.
type ElCore struct{ Core }

func (e ElCore) node() *Node { return e.n }

// ValCore is a value's core (a builder value that is no element).
type ValCore struct{ Core }

func (e ValCore) valueNode() *Node { return e.n }

// HCore is a handle's core: kit state, library objects and the runtime's handles embed it. l is
// shared by every copy of a handle the collector may release (rt_life.go); nil for the others.
type HCore struct {
	h uint64
	l *life
}

func (s HCore) handle() uint64 { return s.h }

// N is the node of a generated element or value (x.Core).
func N(c Core) *Node { return c.n }

// H is the handle number of a handle (x.HCore).
func H(c HCore) uint64 { return c.h }

// MkH makes a handle's core from its number.
func MkH(h uint64) HCore { return HCore{h, lifeOf(h)} }

// MkElCore and MkValCore make the cores of a hand-written element or value.
func MkElCore(n *Node) ElCore { return ElCore{Core{n}} }

// MkValCore: see MkElCore.
func MkValCore(n *Node) ValCore { return ValCore{Core{n}} }

// NodeOf is an element's node, for the packages' hand-written code.
func NodeOf(e Element) *Node { return e.node() }

// ValueOf is a value's node, for the packages' hand-written code.
func ValueOf(v interface{ valueNode() *Node }) *Node { return v.valueNode() }

// HandleOf is a handle's number, for the packages' hand-written code.
func HandleOf(x interface{ handle() uint64 }) uint64 { return x.handle() }

// build is the tape of one view build (or the value build of a turn): an op stream and a node
// table. A builder call appends one op and patches the previous op's link.
type build struct {
	ops       Tape
	nodes     []*Node
	slab      []Node
	handlers  []handlerRec
	deps      []uint64
	slots     []uint64
	view      *viewRec
	Serial    uint32
	Active    bool
	calledLib bool // a gk_call ran during this view build
	Cur       uint32
	isValue   bool
	turn      uint64 // the value build: the outermost turn it belongs to
}

var (
	Vb          build // the view build in progress
	valb        build // the value build of the outermost turn
	buildSerial uint32
	curBuild    *build // the build the last op or node went to
)

const tapeMagic = 0x31544b47

func init() { valb.isValue = true }

func (b *build) live(n *Node) bool {
	if n.serial != b.Serial {
		return false
	}
	if b.isValue {
		return b.Serial != 0 && b.turn == outerTurn()
	}
	return b.Active
}

func (b *build) reset(serial uint32) {
	b.ops = append(b.ops[:0], 0, 0, 0, 0) // offset 0 means "no op"
	b.nodes = b.nodes[:0]
	// The slab is not reused: the next build's nodes go after this one's (a new block when it is
	// full), so a node of an earlier build keeps its serial and is caught as stale. Starting a
	// fresh block per build cost a 4 KB allocation for every row of a table.
	b.handlers = b.handlers[:0]
	b.deps = b.deps[:0]
	b.slots = b.slots[:0]
	b.calledLib = false
	b.Serial = serial
	if debug && len(madeMap) > 0 && !b.isValue {
		clear(madeMap)
	}
}

func (b *build) alloc(typ uint16) *Node {
	if len(b.slab) == cap(b.slab) {
		b.slab = make([]Node, 0, 256)
	}
	b.slab = append(b.slab, Node{serial: b.Serial, idx: uint32(len(b.nodes)), typ: typ, args: uint32(len(b.ops)), valBuild: b.isValue})
	n := &b.slab[len(b.slab)-1]
	b.nodes = append(b.nodes, n)
	b.Cur = n.idx
	curBuild = b
	if debug {
		var m madeAt
		_, m.file, m.line, _ = runtime.Caller(3)
		if f, l, _ := appFrame(3); l != 0 {
			m.file, m.line = f, l
		}
		if madeMap == nil {
			madeMap = map[*Node]madeAt{}
		}
		madeMap[n] = m
	}
	return n
}

func outerTurn() uint64 {
	t := cur
	if t == nil {
		return 0
	}
	for t.parent != nil {
		t = t.parent
	}
	return t.serial
}

// origin is what the runtime can say about where an element came from, for a stop.
func origin(n *Node, thisBuild bool) string {
	if n != nil && thisBuild && debug {
		if m, ok := madeMap[n]; ok && m.line != 0 {
			return fmt.Sprintf("\n    the element was made at %s:%d", m.file, m.line)
		}
	}
	s := ""
	if Vb.Active && Vb.view != nil {
		v := Vb.view
		if v.row != nil {
			s = fmt.Sprintf("\n    while the row function described row %d of a list", v.rowAt.Row)
		} else if v.line != 0 {
			s = fmt.Sprintf("\n    while the view function of the gk.NewView made at %s:%d ran", v.file, v.line)
		}
	}
	if n != nil {
		// Only a stop about an element: a stop about a value given to a builder names that
		// builder's line itself.
		s += madeHint()
	}
	return s
}

// madeHint says how to learn where an element was made (GK_DEBUG records it).
func madeHint() string {
	if debug {
		return ""
	}
	return "\n    Run the app with GK_DEBUG=1 to see the line that made the element."
}

const staleElement = "an element was used after the view function that made it returned: an element lives for one call of the view function. Make it again in each call (a function that returns the element is how a piece of a view is reused)"

// newNode starts an element of a node type in the view build in progress.
func NewNode(typ uint16) *Node {
	if !Vb.Active {
		Stop("an element was made outside a view function: kit.Button, gk.Div, kit.Label and the rest describe what a view shows, in the function given to gk.NewView. A handler changes the app's state; the view function then describes it again")
	}
	return Vb.alloc(typ)
}

// newValue starts a value node: in the view build, or in the turn's value build outside a view
// (a call op then carries it as a tape of its own, PROTOCOLS.md 13.6).
func NewValue(typ uint16) *Node {
	if Vb.Active {
		return Vb.alloc(typ)
	}
	if cur == nil {
		Stop("a kit value was made outside the app's functions: make it in init, a handler or a view function (after gk.Run has started the app)")
	}
	if outer := outerTurn(); valb.turn != outer || valb.Serial == 0 {
		buildSerial++
		valb.reset(buildSerial)
		valb.turn = outer
	}
	return valb.alloc(typ)
}

// nodeArgs is where the node just made keeps its constructor's arguments.
func NodeArgs(n *Node) *Tape {
	b := n.build()
	n.args = uint32(len(b.ops))
	return &b.ops
}

// op starts an op of this node and returns the op stream to append its arguments to.
func Op(n *Node, code uint16) *Tape {
	if n == nil {
		Stop("an element that was never made (a zero gk element value) was given a builder: make it with its constructor")
	}
	b := n.build()
	if !b.live(n) {
		Stop("%s%s", staleElement, origin(n, false))
	}
	if n.attached {
		Stop("an element was changed after it was given to its parent (.Child or another builder): call its builders first, then give it, as in row.Child(kit.Label(..).TextColor(..))%s", origin(n, true))
	}
	at := uint32(len(b.ops))
	b.ops = append(b.ops, byte(code), byte(code>>8), 0, 0, 0, 0) // u16 opcode, u32 link to the node's next op
	if n.last == 0 {
		n.first = at
	} else {
		le.PutUint32(b.ops[n.last+2:], at)
	}
	n.last = at
	b.Cur = n.idx
	curBuild = b
	return &b.ops
}

func ElemNode(e Element) *Node {
	if e == nil {
		Stop("a nil element was given to a builder: give an element (gk.Div(), kit.Label(..))")
	}
	n := e.node()
	if n == nil {
		Stop("an element that was never made (a zero gk element value, as in var b kit.ButtonEl) was given to a builder: make it with its constructor (kit.Button(..)) in the view function")
	}
	return n
}

// attach makes child a child of n (the CHILD op). A node is attached once.
func Attach(n *Node, child Element) {
	c := ElemNode(child)
	cb := c.build()
	if !cb.live(c) {
		Stop("%s%s", staleElement, origin(c, false))
	}
	if c.attached {
		Stop("an element was given to a second parent: an element has one place in a view. Make another one for the second place (a function that returns it, called twice)%s", origin(c, true))
	}
	if c == n {
		Stop("an element was made a child of itself (x.Child(x))%s", origin(n, true))
	}
	o := Op(n, EChild)
	c.attached = true
	*o = le.AppendUint32(*o, c.idx)
}

// take uses an element as an argument (a builder's child or fragment): once, in its build.
func Take(e Element) uint32 { return TakeNode(ElemNode(e), false) }

func TakeNode(n *Node, receiver bool) uint32 {
	if n == nil {
		Stop("an element that was never made (a zero gk element value, as in var b kit.ButtonEl) was given to a builder: make it with its constructor (kit.Button(..)) in the view function")
	}
	b := n.build()
	if !b.live(n) {
		Stop("%s%s", staleElement, origin(n, false))
	}
	if n.attached {
		Stop("an element was given to a second builder: an element has one place in a view. Make another one for the second place%s", origin(n, true))
	}
	if n.idx == b.Cur && b == curBuild && !receiver {
		Stop("an element was given as an argument to its own builder (x.Child(x) or the like)%s", origin(n, true))
	}
	n.attached = true
	return n.idx
}

// takeValue uses a value (a column, a style) as an argument of another node of its build. A
// value may be given to any number of builders of the build that made it.
func TakeValue(n *Node) uint32 {
	if n == nil {
		Stop("a kit value that was never made (a zero value) was given to a builder: make it with its constructor")
	}
	b := n.build()
	if !b.live(n) {
		Stop("a kit value (a column, a menu item, a style) was used after the function that made it returned: values live for one call. Make it again in each call%s", origin(n, false))
	}
	if n.idx == b.Cur && b == curBuild {
		Stop("a value was given as an argument to its own builder%s", origin(n, true))
	}
	if curBuild != nil && b != curBuild {
		Stop("a kit value made outside this view function was given to one of its elements (or the other way round): make it where it is used")
	}
	n.attached = true
	return n.idx
}

// putVtape writes a value as a call op's argument: u32 length, then a tape whose root is the
// value (the value build so far, without handlers, dependencies or slots).
func PutVtape(o *Tape, n *Node) {
	if n == nil {
		Stop("a kit value that was never made (a zero value) was given to a call: make it with its constructor where it is used")
	}
	b := n.build()
	if !b.isValue {
		Stop("a value made in a view function was given to a call: make it in the handler that calls")
	}
	if !b.live(n) {
		Stop("a kit value was used after the function that made it returned: values live for one call of init, a handler or a view function. Make it again where it is used")
	}
	n.attached = true
	o.U32(uint32(4*4 + 2 + 2 + 4 + len(b.ops) + len(b.nodes)*10))
	o.U32(tapeMagic)
	o.U32(uint32(len(b.nodes)))
	o.U32(n.idx)
	o.U32(0)
	o.U16(0)
	o.U16(0)
	o.U32(uint32(len(b.ops)))
	*o = append(*o, b.ops...)
	for _, nd := range b.nodes {
		o.U16(nd.typ)
		o.U32(nd.args)
		o.U32(nd.first)
	}
}

// finish writes the tape of the view build: header, ops, node table.
func finish(root Element, out Tape) Tape {
	if root == nil {
		Stop("a view function returned nil: return the element it built%s", origin(nil, false))
	}
	r := root.node()
	if r == nil || r.valBuild || r.serial != Vb.Serial || !Vb.Active {
		Stop("a view function returned an element it did not make in this call (one kept from an earlier call, or made by another view): return an element the function built just now%s%s", origin(nil, false), madeHint())
	}
	b := &Vb
	b.Active = false
	need := 24 + len(b.deps)*8 + len(b.slots)*8 + len(b.ops) + len(b.nodes)*10
	if cap(out) < need {
		out = make(Tape, 0, max(need, 2*cap(out), 512))
	} else {
		out = out[:0]
	}
	out = le.AppendUint32(out, tapeMagic)
	out = le.AppendUint32(out, uint32(len(b.nodes)))
	out = le.AppendUint32(out, r.idx)
	out = le.AppendUint32(out, uint32(len(b.handlers)))
	out = le.AppendUint16(out, uint16(len(b.deps)))
	for _, d := range b.deps {
		out = le.AppendUint64(out, d)
	}
	out = le.AppendUint16(out, uint16(len(b.slots)))
	for _, s := range b.slots {
		out = le.AppendUint64(out, s)
	}
	out = le.AppendUint32(out, uint32(len(b.ops)))
	out = append(out, b.ops...)
	for _, n := range b.nodes {
		out = le.AppendUint16(out, n.typ)
		out = le.AppendUint32(out, n.args)
		out = le.AppendUint32(out, n.first)
	}
	return out
}

// Style is one style token (package s: s.Gap3, s.SizeFull).
type Style uint16

// tokens writes style tokens (the STYLE op).
func Tokens(n *Node, st []Style) {
	if len(st) == 0 {
		return
	}
	o := Op(n, EStyle)
	b := le.AppendUint16(*o, uint16(len(st)))
	for _, s := range st {
		b = le.AppendUint16(b, uint16(s))
	}
	*o = b
}

func PutText(n *Node, s string) { Op(n, EText).Str(s) }

// slotView embeds a view in n (the CHILD_VIEW op).
func SlotView(n *Node, v View) {
	Op(n, EChildView).U64(v.h)
	Vb.slots = append(Vb.slots, v.h)
	if rec := views.get(v.h); rec != nil && cur != nil {
		if cur.window != 0 {
			rec.window = cur.window
		}
		if !rec.described {
			cur.pending = append(cur.pending, rec)
		}
	}
}

// Color is an HSLA colour or a colour of the kit's theme or palette, resolved by the library each
// time it draws (package theme: theme.Background; package palette: palette.Blue500).
type Color struct {
	theme      uint16
	h, s, l, a float32
}

// HSLA is a colour by hue, saturation, lightness and alpha, each 0 to 1.
func HSLA(h, s, l, a float32) Color { return Color{0, h, s, l, a} }

// Theme is the theme colour number c names (package theme), 0 for a colour given by its parts.
func (c Color) Theme() uint16 { return c.theme }

// Hsla is c's hue, saturation, lightness and alpha (zero for a theme colour, which the theme
// resolves where it is drawn).
func (c Color) Hsla() Hsla { return Hsla{c.h, c.s, c.l, c.a} }

// ThemeColor is the theme's colour number n (package theme names them all).
func ThemeColor(n uint16) Color { return Color{theme: n} }

// PaletteColor is the kit's fixed colour number n (package palette names them all).
func PaletteColor(n uint16) Color { return Color{theme: n} }

// Hsla is a colour the library hands back (in an event, a read's result).
type Hsla struct{ H, S, L, A float32 }

// Color turns a colour read back into one a builder takes.
func (c Hsla) Color() Color { return HSLA(c.H, c.S, c.L, c.A) }

func RdHsla(r *Reader) Hsla { return Hsla{r.F32(), r.F32(), r.F32(), r.F32()} }

// rdColorField reads a colour field of a record the app also writes, which the library writes
// in Color's own layout (a theme u16, then h, s, l, a; gk_values.rust_field_put).
func RdColorField(r *Reader) Color {
	c := Color{theme: r.U16()}
	c.h, c.s, c.l, c.a = r.F32(), r.F32(), r.F32(), r.F32()
	return c
}

func PutColor(o *Tape, c Color) {
	if c.theme != 0 {
		o.U8(ColorTheme)
		o.U16(c.theme)
		return
	}
	o.U8(ColorHsla)
	o.F32(c.h)
	o.F32(c.s)
	o.F32(c.l)
	o.F32(c.a)
}

// Fill is what .Bg and the plot's paths and quads take (gpui's Fill and Background): a Color, or
// a LinearGradient. PROTOCOLS.md 11.38.
//
//	.Bg(theme.Background)
//	.Bg(gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1)))
type Fill interface{ isFill() }

func (Color) isFill() {}

// NewLinearColorStop is gpui's linear_color_stop: c at percentage (0 to 1) of a gradient
// (gk.LinearColorStop is the kit's record of it).
func NewLinearColorStop(c Color, percentage float32) LinearColorStop {
	return LinearColorStop{Color: c, Percentage: percentage}
}

// LinearGradient is gpui's linear_gradient: from one stop to the other along Angle (degrees; 0 is
// up, 90 left to right, increasing clockwise), mixed in sRGB or, with Oklab, in the Oklab space.
type LinearGradient struct {
	Angle    float32
	From, To LinearColorStop
	Oklab    bool
}

func (LinearGradient) isFill() {}

// NewLinearGradient is gpui's linear_gradient(angle, from, to).
func NewLinearGradient(angle float32, from, to LinearColorStop) LinearGradient {
	return LinearGradient{Angle: angle, From: from, To: to}
}

// InOklab is the gradient mixed in the Oklab colour space (gpui's Background::color_space).
func (g LinearGradient) InOklab() LinearGradient { g.Oklab = true; return g }

func PutFill(o *Tape, f Fill) {
	switch x := f.(type) {
	case Color:
		PutColor(o, x)
	case LinearGradient:
		o.U8(ColorGradient)
		o.F32(x.Angle)
		o.Bool(x.Oklab)
		PutColor(o, x.From.Color)
		o.F32(x.From.Percentage)
		PutColor(o, x.To.Color)
		o.F32(x.To.Percentage)
	case nil:
		Stop("a nil gk.Fill was given to a builder: give a colour (theme.Background, gk.HSLA(..)) or a gk.LinearGradient")
	default:
		// Unreachable from outside the package (isFill is unexported). No %T here: passing f to stop
		// would make every .Bg argument escape to the heap, an allocation per call in every view.
		Stop("an unknown gk.Fill was given to a builder: give a gk.Color or a gk.LinearGradient")
	}
}

// Length is what `impl Into<Length>`, DefiniteLength and AbsoluteLength accept.
type Length struct {
	kind uint8
	v    float32
}

// Px is a length in logical pixels.
func Px(v float32) Length { return Length{0, v} }

// Rems is a length in rems (the root font size).
func Rems(v float32) Length { return Length{1, v} }

// Relative is a fraction of the parent's size (0.5 is half).
func Relative(v float32) Length { return Length{2, v} }

// Auto is the automatic length.
func Auto() Length { return Length{3, 0} }

func PutLength(o *Tape, l Length) {
	o.U8(l.kind)
	o.F32(l.v)
}

func PutDefiniteLength(o *Tape, l Length) {
	if l.kind > 2 {
		Stop("gk.Auto() was given to a builder that takes a definite length: give gk.Px, gk.Rems or gk.Relative%s", origin(nil, false))
	}
	PutLength(o, l)
}

func PutAbsoluteLength(o *Tape, l Length) {
	if l.kind > 1 {
		Stop("gk.Relative or gk.Auto was given to a builder that takes an absolute length: give gk.Px or gk.Rems%s", origin(nil, false))
	}
	PutLength(o, l)
}

// ID is what an element id (`impl Into<ElementId>`) accepts: a string, an int, or gk.Key(name, n).
type ID interface{}

// NamedID is an id made of a name and a number: gk.Key("row", i).
type NamedID struct {
	Name string
	N    int
}

// Key is an element id of a name and a number, for elements made in a loop: gk.Key("row", i).
func Key(name string, n int) NamedID { return NamedID{name, n} }

func PutID(o *Tape, v ID) {
	switch x := v.(type) {
	case string:
		o.U8(0)
		o.Str(x)
	case int:
		o.U8(1)
		o.U64(uint64(x))
	case int64:
		o.U8(1)
		o.U64(uint64(x))
	case uint64:
		o.U8(1)
		o.U64(x)
	case NamedID:
		o.U8(2)
		o.Str(x.Name)
		o.U64(uint64(x.N))
	default:
		Stop("a %T was given as an element id: give a string, an int or gk.Key(name, n)%s", v, origin(nil, false))
	}
}

// Range is a range of indices or byte offsets, Lo included, Hi not.
type Range struct{ Lo, Hi int }

func PutRange(o *Tape, r Range) {
	if r.Lo < 0 || r.Hi < 0 {
		Stop("a negative number was given in a range (%d to %d) where the kit takes byte offsets or indices: give 0 or more%s", r.Lo, r.Hi, origin(nil, false))
	}
	o.U64(uint64(r.Lo))
	o.U64(uint64(r.Hi))
}

func PutUsize(o *Tape, v int) {
	if v < 0 {
		Stop("a negative number (%d) was given where the kit takes a count, size or index%s", v, origin(nil, false))
	}
	o.U64(uint64(v))
}

func PutNonzero(o *Tape, v uint64, size int) {
	if v == 0 {
		Stop("0 was given where the kit takes a number that is never 0: give 1 or more%s", origin(nil, false))
	}
	switch size {
	case 1:
		o.U8(uint8(v))
	case 2:
		o.U16(uint16(v))
	case 4:
		o.U32(uint32(v))
	default:
		o.U64(v)
	}
}

// putMs writes a duration as whole milliseconds (rounded; negative is 0, at most 49 days).
func PutMs(o *Tape, d time.Duration) {
	ms := (max(int64(d), 0) + 500_000) / 1_000_000
	o.U32(uint32(min(ms, int64(^uint32(0)))))
}

// putObj writes a library object's handle as an element's argument and pins it to the view that
// shows it (PROTOCOLS.md 13.12).
func PutObj(o *Tape, h uint64) {
	o.U64(Held(h))
	if Vb.Active && Vb.view != nil && h != 0 {
		req := NewReq(CObjPin)
		req.U64(Vb.view.h)
		req.U32(Vb.view.buildNo)
		req.U64(h)
		call(*req) // a stale handle is reported where the element is drawn
	}
}

// putViewArg writes a view as a closure's answer (a view-returning closure, PROTOCOLS.md 13.18).
func PutViewArg(o *Tape, v View) {
	o.U64(v.h)
	if Vb.Active && v.h != 0 {
		Vb.slots = append(Vb.slots, v.h)
		if rec := views.get(v.h); rec != nil && cur != nil {
			if cur.window != 0 {
				rec.window = cur.window
			}
			if !rec.described {
				cur.pending = append(cur.pending, rec)
			}
		}
	}
}
