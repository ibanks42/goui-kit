package gk

// The emitter's interface: everything generated Go (gen_*.go) may call (docs/PROTOCOLS.md
// section 7, the Go column of docs/GO.md). Generated code uses:
//
//   elements:  node  Element  newNode  newValue  nodeArgs  op  attach  take  takeNode  takeValue
//              putVtape  tokens  register  slotView  putText
//   state:     newReq  stateMake  stateGet  stateTry  stateSet  stateCall  objGet  ambientCall
//              keepHandler  putKeptHandler  newStateBuilder  Subscription
//   wire:      tape (u8 .. str), reader (u8 .. str), rdOpt  rdList  rdRange  rdDuration  rdResult
//              rdHsla  rdClickEvent  rdKeyEvent  rdMouseEvent
//   values:    putColor  putLength  putDefiniteLength  putAbsoluteLength  putID  putMs  putRange
//              putUsize  putNonzero  putObj  putAction  putActionHandler  putViewArg  stop

import (
	"fmt"
	"runtime"
	"time"
)

// node is one element or value being described: a node of a view build (or of the value build of
// a turn, for values made outside a view). Element types hold a pointer to theirs, so an element
// is one word and giving it to Child does not allocate. A node of an earlier build is caught by
// its serial.
type node struct {
	b                 *build
	serial            uint32
	idx               uint32
	typ               uint16
	args, first, last uint32
	attached          bool
	file              string // GK_DEBUG=1: where it was made
	line              int
}

// Element is an element being described (gk.Div(), gk.Button("ok"), ...).
type Element interface{ node() *node }

// value is a builder value that is not an element (a column, a style refinement, a menu item).
type value interface{ valueNode() *node }

// build is the tape of one view build (or the value build of a turn): an op stream and a node
// table. A builder call appends one op and patches the previous op's link.
type build struct {
	ops      tape
	nodes    []*node
	slab     []node
	handlers []handlerRec
	deps     []uint64
	slots    []uint64
	view     *viewRec
	serial   uint32
	active   bool
	cur      uint32
	isValue  bool
	turn     uint64 // the value build: the outermost turn it belongs to
}

var (
	vb          build // the view build in progress
	valb        build // the value build of the outermost turn
	buildSerial uint32
	curBuild    *build // the build the last op or node went to
)

const tapeMagic = 0x31544b47

func init() { valb.isValue = true }

func (b *build) live(n *node) bool {
	if n.serial != b.serial {
		return false
	}
	if b.isValue {
		return b.serial != 0 && b.turn == outerTurn()
	}
	return b.active
}

func (b *build) reset(serial uint32) {
	b.ops = append(b.ops[:0], 0, 0, 0, 0) // offset 0 means "no op"
	b.nodes = b.nodes[:0]
	b.slab = nil // a fresh block: nodes of the last build stay what they were, and stale
	b.handlers = b.handlers[:0]
	b.deps = b.deps[:0]
	b.slots = b.slots[:0]
	b.serial = serial
}

func (b *build) alloc(typ uint16) *node {
	if len(b.slab) == cap(b.slab) {
		b.slab = make([]node, 0, 64)
	}
	b.slab = append(b.slab, node{b: b, serial: b.serial, idx: uint32(len(b.nodes)), typ: typ, args: uint32(len(b.ops))})
	n := &b.slab[len(b.slab)-1]
	b.nodes = append(b.nodes, n)
	b.cur = n.idx
	curBuild = b
	if debug {
		_, n.file, n.line, _ = runtime.Caller(3)
		if f, l, _ := appFrame(3); l != 0 {
			n.file, n.line = f, l
		}
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
func origin(n *node, thisBuild bool) string {
	if n != nil && thisBuild && n.line != 0 {
		return fmt.Sprintf("\n    the element was made at %s:%d", n.file, n.line)
	}
	s := ""
	if vb.active && vb.view != nil {
		v := vb.view
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
func newNode(typ uint16) *node {
	if !vb.active {
		stop("an element was made outside a view function: gk.Button, gk.Div, gk.Label and the rest describe what a view shows, in the function given to gk.NewView. A handler changes the app's state; the view function then describes it again")
	}
	return vb.alloc(typ)
}

// newValue starts a value node: in the view build, or in the turn's value build outside a view
// (a call op then carries it as a tape of its own, PROTOCOLS.md 13.6).
func newValue(typ uint16) *node {
	if vb.active {
		return vb.alloc(typ)
	}
	if cur == nil {
		stop("a kit value was made outside the app's functions: make it in init, a handler or a view function (after gk.Run has started the app)")
	}
	if outer := outerTurn(); valb.turn != outer || valb.serial == 0 {
		buildSerial++
		valb.reset(buildSerial)
		valb.turn = outer
	}
	return valb.alloc(typ)
}

// nodeArgs is where the node just made keeps its constructor's arguments.
func nodeArgs(n *node) *tape {
	n.args = uint32(len(n.b.ops))
	return &n.b.ops
}

// op starts an op of this node and returns the op stream to append its arguments to.
func op(n *node, code uint16) *tape {
	if n == nil {
		stop("an element that was never made (a zero gk element value) was given a builder: make it with its constructor")
	}
	b := n.b
	if !b.live(n) {
		stop("%s%s", staleElement, origin(n, false))
	}
	if n.attached {
		stop("an element was changed after it was given to its parent (.Child or another builder): call its builders first, then give it, as in row.Child(gk.Label(..).TextColor(..))%s", origin(n, true))
	}
	at := uint32(len(b.ops))
	b.ops = append(b.ops, byte(code), byte(code>>8), 0, 0, 0, 0) // u16 opcode, u32 link to the node's next op
	if n.last == 0 {
		n.first = at
	} else {
		le.PutUint32(b.ops[n.last+2:], at)
	}
	n.last = at
	b.cur = n.idx
	curBuild = b
	return &b.ops
}

func elemNode(e Element) *node {
	if e == nil {
		stop("a nil element was given to a builder: give an element (gk.Div(), gk.Label(..))")
	}
	n := e.node()
	if n == nil {
		stop("an element that was never made (a zero gk element value, as in var b gk.ButtonEl) was given to a builder: make it with its constructor (gk.Button(..)) in the view function")
	}
	return n
}

// attach makes child a child of n (the CHILD op). A node is attached once.
func attach(n *node, child Element) {
	c := elemNode(child)
	if !c.b.live(c) {
		stop("%s%s", staleElement, origin(c, false))
	}
	if c.attached {
		stop("an element was given to a second parent: an element has one place in a view. Make another one for the second place (a function that returns it, called twice)%s", origin(c, true))
	}
	if c == n {
		stop("an element was made a child of itself (x.Child(x))%s", origin(n, true))
	}
	o := op(n, eChild)
	c.attached = true
	o.u32(c.idx)
}

// take uses an element as an argument (a builder's child or fragment): once, in its build.
func take(e Element) uint32 { return takeNode(elemNode(e), false) }

func takeNode(n *node, receiver bool) uint32 {
	if n == nil {
		stop("an element that was never made (a zero gk element value, as in var b gk.ButtonEl) was given to a builder: make it with its constructor (gk.Button(..)) in the view function")
	}
	if !n.b.live(n) {
		stop("%s%s", staleElement, origin(n, false))
	}
	if n.attached {
		stop("an element was given to a second builder: an element has one place in a view. Make another one for the second place%s", origin(n, true))
	}
	if n.idx == n.b.cur && n.b == curBuild && !receiver {
		stop("an element was given as an argument to its own builder (x.Child(x) or the like)%s", origin(n, true))
	}
	n.attached = true
	return n.idx
}

// takeValue uses a value (a column, a style) as an argument of another node of its build. A
// value may be given to any number of builders of the build that made it.
func takeValue(n *node) uint32 {
	if n == nil {
		stop("a kit value that was never made (a zero value) was given to a builder: make it with its constructor")
	}
	if !n.b.live(n) {
		stop("a kit value (a column, a menu item, a style) was used after the function that made it returned: values live for one call. Make it again in each call%s", origin(n, false))
	}
	if n.idx == n.b.cur && n.b == curBuild {
		stop("a value was given as an argument to its own builder%s", origin(n, true))
	}
	if curBuild != nil && n.b != curBuild {
		stop("a kit value made outside this view function was given to one of its elements (or the other way round): make it where it is used")
	}
	n.attached = true
	return n.idx
}

// putVtape writes a value as a call op's argument: u32 length, then a tape whose root is the
// value (the value build so far, without handlers, dependencies or slots).
func putVtape(o *tape, n *node) {
	if n == nil {
		stop("a kit value that was never made (a zero value) was given to a call: make it with its constructor where it is used")
	}
	if !n.b.isValue {
		stop("a value made in a view function was given to a call: make it in the handler that calls")
	}
	if !n.b.live(n) {
		stop("a kit value was used after the function that made it returned: values live for one call of init, a handler or a view function. Make it again where it is used")
	}
	n.attached = true
	b := n.b
	o.u32(uint32(4*4 + 2 + 2 + 4 + len(b.ops) + len(b.nodes)*10))
	o.u32(tapeMagic)
	o.u32(uint32(len(b.nodes)))
	o.u32(n.idx)
	o.u32(0)
	o.u16(0)
	o.u16(0)
	o.u32(uint32(len(b.ops)))
	*o = append(*o, b.ops...)
	for _, nd := range b.nodes {
		o.u16(nd.typ)
		o.u32(nd.args)
		o.u32(nd.first)
	}
}

// finish writes the tape of the view build: header, ops, node table.
func finish(root Element, out tape) tape {
	if root == nil {
		stop("a view function returned nil: return the element it built%s", origin(nil, false))
	}
	r := root.node()
	if r == nil || r.b != &vb || r.serial != vb.serial || !vb.active {
		stop("a view function returned an element it did not make in this call (one kept from an earlier call, or made by another view): return an element the function built just now%s%s", origin(nil, false), madeHint())
	}
	b := &vb
	b.active = false
	out = out[:0]
	out.u32(tapeMagic)
	out.u32(uint32(len(b.nodes)))
	out.u32(r.idx)
	out.u32(uint32(len(b.handlers)))
	out.u16(uint16(len(b.deps)))
	for _, d := range b.deps {
		out.u64(d)
	}
	out.u16(uint16(len(b.slots)))
	for _, s := range b.slots {
		out.u64(s)
	}
	out.u32(uint32(len(b.ops)))
	out = append(out, b.ops...)
	for _, n := range b.nodes {
		out.u16(n.typ)
		out.u32(n.args)
		out.u32(n.first)
	}
	return out
}

// Style is one style token (package s: s.Gap3, s.SizeFull).
type Style uint16

// tokens writes style tokens (the STYLE op).
func tokens(n *node, st []Style) {
	if len(st) == 0 {
		return
	}
	o := op(n, eStyle)
	o.u16(uint16(len(st)))
	for _, s := range st {
		o.u16(uint16(s))
	}
}

func putText(n *node, s string) { op(n, eText).str(s) }

// slotView embeds a view in n (the CHILD_VIEW op).
func slotView(n *node, v View) {
	op(n, eChildView).u64(v.h)
	vb.slots = append(vb.slots, v.h)
	if rec := views[v.h]; rec != nil && !rec.described && cur != nil {
		cur.pending = append(cur.pending, rec)
	}
}

// --- colours, lengths, ids, ranges ------------------------------------------------------------

// Color is an HSLA colour or a colour of the kit's theme or palette, resolved by the library each
// time it draws (package theme: theme.Background; package palette: palette.Blue500).
type Color struct {
	theme      uint16
	h, s, l, a float32
}

// HSLA is a colour by hue, saturation, lightness and alpha, each 0 to 1.
func HSLA(h, s, l, a float32) Color { return Color{0, h, s, l, a} }

// ThemeColor is the theme's colour number n (package theme names them all).
func ThemeColor(n uint16) Color { return Color{theme: n} }

// PaletteColor is the kit's fixed colour number n (package palette names them all).
func PaletteColor(n uint16) Color { return Color{theme: n} }

// Hsla is a colour the library hands back (in an event, a read's result).
type Hsla struct{ H, S, L, A float32 }

// Color turns a colour read back into one a builder takes.
func (c Hsla) Color() Color { return HSLA(c.H, c.S, c.L, c.A) }

func rdHsla(r *reader) Hsla { return Hsla{r.f32(), r.f32(), r.f32(), r.f32()} }

func putColor(o *tape, c Color) {
	if c.theme != 0 {
		o.u8(colorTheme)
		o.u16(c.theme)
		return
	}
	o.u8(colorHsla)
	o.f32(c.h)
	o.f32(c.s)
	o.f32(c.l)
	o.f32(c.a)
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

func putFill(o *tape, f Fill) {
	switch x := f.(type) {
	case Color:
		putColor(o, x)
	case LinearGradient:
		o.u8(colorGradient)
		o.f32(x.Angle)
		o.bool(x.Oklab)
		putColor(o, x.From.Color)
		o.f32(x.From.Percentage)
		putColor(o, x.To.Color)
		o.f32(x.To.Percentage)
	case nil:
		stop("a nil gk.Fill was given to a builder: give a colour (theme.Background, gk.HSLA(..)) or a gk.LinearGradient")
	default:
		// Unreachable from outside the package (isFill is unexported). No %T here: passing f to stop
		// would make every .Bg argument escape to the heap, an allocation per call in every view.
		stop("an unknown gk.Fill was given to a builder: give a gk.Color or a gk.LinearGradient")
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

func putLength(o *tape, l Length) {
	o.u8(l.kind)
	o.f32(l.v)
}

func putDefiniteLength(o *tape, l Length) {
	if l.kind > 2 {
		stop("gk.Auto() was given to a builder that takes a definite length: give gk.Px, gk.Rems or gk.Relative%s", origin(nil, false))
	}
	putLength(o, l)
}

func putAbsoluteLength(o *tape, l Length) {
	if l.kind > 1 {
		stop("gk.Relative or gk.Auto was given to a builder that takes an absolute length: give gk.Px or gk.Rems%s", origin(nil, false))
	}
	putLength(o, l)
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

func putID(o *tape, v ID) {
	switch x := v.(type) {
	case string:
		o.u8(0)
		o.str(x)
	case int:
		o.u8(1)
		o.u64(uint64(x))
	case int64:
		o.u8(1)
		o.u64(uint64(x))
	case uint64:
		o.u8(1)
		o.u64(x)
	case NamedID:
		o.u8(2)
		o.str(x.Name)
		o.u64(uint64(x.N))
	default:
		stop("a %T was given as an element id: give a string, an int or gk.Key(name, n)%s", v, origin(nil, false))
	}
}

// Range is a range of indices or byte offsets, Lo included, Hi not.
type Range struct{ Lo, Hi int }

func putRange(o *tape, r Range) {
	if r.Lo < 0 || r.Hi < 0 {
		stop("a negative number was given in a range (%d to %d) where the kit takes byte offsets or indices: give 0 or more%s", r.Lo, r.Hi, origin(nil, false))
	}
	o.u64(uint64(r.Lo))
	o.u64(uint64(r.Hi))
}

func putUsize(o *tape, v int) {
	if v < 0 {
		stop("a negative number (%d) was given where the kit takes a count, size or index%s", v, origin(nil, false))
	}
	o.u64(uint64(v))
}

func putNonzero(o *tape, v uint64, size int) {
	if v == 0 {
		stop("0 was given where the kit takes a number that is never 0: give 1 or more%s", origin(nil, false))
	}
	switch size {
	case 1:
		o.u8(uint8(v))
	case 2:
		o.u16(uint16(v))
	case 4:
		o.u32(uint32(v))
	default:
		o.u64(v)
	}
}

// putMs writes a duration as whole milliseconds (rounded; negative is 0, at most 49 days).
func putMs(o *tape, d time.Duration) {
	ms := (max(int64(d), 0) + 500_000) / 1_000_000
	o.u32(uint32(min(ms, int64(^uint32(0)))))
}

// putObj writes a library object's handle as an element's argument and pins it to the view that
// shows it (PROTOCOLS.md 13.12).
func putObj(o *tape, h uint64) {
	o.u64(h)
	if vb.active && vb.view != nil && h != 0 {
		req := newReq(cObjPin)
		req.u64(vb.view.h)
		req.u32(vb.view.buildNo)
		req.u64(h)
		call(*req) // a stale handle is reported where the element is drawn
	}
}

// putViewArg writes a view as a closure's answer (a view-returning closure, PROTOCOLS.md 13.18).
func putViewArg(o *tape, v View) {
	o.u64(v.h)
	if vb.active && v.h != 0 {
		vb.slots = append(vb.slots, v.h)
		if rec := views[v.h]; rec != nil && !rec.described && cur != nil {
			cur.pending = append(cur.pending, rec)
		}
	}
}
