package rt

import (
	"fmt"
	"runtime"
	"time"
)

// Node represents one element or kit value recorded in a tape build.
// Builder operations applied to a node are stored as a singly linked list of
// byte offsets inside build.ops, threaded from first through last.
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

type madeAt struct {
	file string
	line int
}

// madeMap records creation file:line for nodes when GK_DEBUG=1 is set.
var madeMap map[*Node]madeAt

// Element is implemented by every element handle returned by constructors and
// builders in gk and its component sub-packages.
type Element interface{ node() *Node }

type value interface{ valueNode() *Node }

// Core wraps the underlying tape Node pointer embedded in ElCore and ValCore.
type Core struct{ n *Node }

// ElCore is embedded by generated element types to satisfy Element.
type ElCore struct{ Core }

func (e ElCore) node() *Node { return e.n }

// ValCore is embedded by generated kit value types so they can be passed into
// element builders or encoded as value tapes.
type ValCore struct{ Core }

func (e ValCore) valueNode() *Node { return e.n }

// HCore embeds an opaque 64-bit Rust handle along with a strong reference to
// its GC life token so keeping any handle value keeps the underlying Rust
// object alive.
type HCore struct {
	h uint64
	l *life
}

func (s HCore) handle() uint64 { return s.h }

// N extracts the underlying *Node from a Core.
func N(c Core) *Node { return c.n }

// H extracts the raw 64-bit Rust handle from an HCore.
func H(c HCore) uint64 { return c.h }

// MkH constructs an HCore for raw handle h, attaching its live *life token if tracked.
func MkH(h uint64) HCore { return HCore{h, lifeOf(h)} }

// MkElCore wraps n in an ElCore.
func MkElCore(n *Node) ElCore { return ElCore{Core{n}} }

// MkValCore wraps n in a ValCore.
func MkValCore(n *Node) ValCore { return ValCore{Core{n}} }

// NodeOf extracts the *Node from an Element.
func NodeOf(e Element) *Node { return e.node() }

// ValueOf extracts the *Node from a kit value type.
func ValueOf(v interface{ valueNode() *Node }) *Node { return v.valueNode() }

// HandleOf extracts the raw 64-bit handle from any Handled type.
func HandleOf(x interface{ handle() uint64 }) uint64 { return x.handle() }

// build accumulates nodes, opcodes, event handlers, state dependencies, and
// child view slots for either a view description (Vb) or a stand-alone value
// tree built outside a view function (valb).
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
	calledLib bool
	Cur       uint32
	isValue   bool
	turn      uint64
}

// Vb is the active view-description builder; valb builds transient kit values
// constructed inside init or event handlers (valid for the enclosing top-level turn).
var (
	Vb          build
	valb        build
	buildSerial uint32
	curBuild    *build
)

// Little-endian "GKT1" header magic written at the start of every encoded tape.
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
	// Reserve offsets 0..3 as a sentinel so offset 0 always means "no next op".
	b.ops = append(b.ops[:0], 0, 0, 0, 0)
	b.nodes = b.nodes[:0]
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
		s += madeHint()
	}
	return s
}

func madeHint() string {
	if debug {
		return ""
	}
	return "\n    Run the app with GK_DEBUG=1 to see the line that made the element."
}

const staleElement = "an element was used after the view function that made it returned: an element lives for one call of the view function. Make it again in each call (a function that returns the element is how a piece of a view is reused)"

// NewNode allocates a new element node of type typ in the active view build.
func NewNode(typ uint16) *Node {
	if !Vb.Active {
		Stop("an element was made outside a view function: kit.Button, gk.Div, kit.Label and the rest describe what a view shows, in the function given to gk.NewView. A handler changes the app's state; the view function then describes it again")
	}
	return Vb.alloc(typ)
}

// NewValue allocates a new kit value node in Vb when inside a view function, or
// in valb when called from init or an event handler.
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

// NodeArgs records the current end of the op buffer as the start of n's
// constructor arguments and returns the buffer for appending them.
func NodeArgs(n *Node) *Tape {
	b := n.build()
	n.args = uint32(len(b.ops))
	return &b.ops
}

// Op appends a builder operation with opcode code to n's linked op chain and
// returns the tape positioned immediately after the 6-byte (opcode, next) header.
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
	// Each op record starts with [u16 code, u32 nextOffset] followed by op arguments.
	at := uint32(len(b.ops))
	b.ops = append(b.ops, byte(code), byte(code>>8), 0, 0, 0, 0)
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

// ElemNode validates that e is non-nil and constructed, returning its *Node.
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

// Attach appends an EChild op attaching child to parent node n, enforcing
// single-parent ownership and rejecting self-attachment.
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

// Take consumes element e as an argument to a builder and returns its node index.
func Take(e Element) uint32 { return TakeNode(ElemNode(e), false) }

// TakeNode consumes node n, verifying liveness, single use, and (unless
// receiver is true) that n is not being passed to its own builder.
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

// TakeValue consumes value node n for use inside a builder in the same build
// (Vb or valb) and returns its node index.
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

// PutVtape serializes a stand-alone value tree rooted at n from valb into o as
// a length-prefixed mini-tape for a C ABI call (such as ActionOf or state calls).
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

// finish finalizes the active view build Vb into out using the wire tape layout:
// magic, node count, root index, handler count, deps, child slots, ops bytes,
// and the fixed-size 10-byte node table.
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

// Style is a nullary GPUI style token opcode (defined in package github.com/ibanks42/goui-kit/gk/s).
type Style uint16

// Tokens emits an EStyle op applying a slice of nullary style tokens to n.
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

// PutText emits an EText op appending string s as text content on n.
func PutText(n *Node, s string) { Op(n, EText).Str(s) }

// SlotView embeds child view v into parent node n, records v in Vb.slots, and
// queues v for description if it has not yet been described.
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

// Color represents either a theme/palette token (when theme != 0) or an
// explicit HSLA color.
type Color struct {
	theme      uint16
	h, s, l, a float32
}

// HSLA creates an explicit Color from hue, saturation, lightness, and alpha components.
func HSLA(h, s, l, a float32) Color { return Color{0, h, s, l, a} }

// Theme returns the theme or palette token ID, or 0 for an explicit HSLA color.
func (c Color) Theme() uint16 { return c.theme }

// Hsla returns the raw HSLA components of c.
func (c Color) Hsla() Hsla { return Hsla{c.h, c.s, c.l, c.a} }

// ThemeColor constructs a Color backed by theme token ID n.
func ThemeColor(n uint16) Color { return Color{theme: n} }

// PaletteColor constructs a Color backed by palette token ID n.
func PaletteColor(n uint16) Color { return Color{theme: n} }

// Hsla holds explicit hue, saturation, lightness, and alpha color channels.
type Hsla struct{ H, S, L, A float32 }

// Color converts c to a Color.
func (c Hsla) Color() Color { return HSLA(c.H, c.S, c.L, c.A) }

// RdHsla decodes four float32 channels from r into an Hsla value.
func RdHsla(r *Reader) Hsla { return Hsla{r.F32(), r.F32(), r.F32(), r.F32()} }

// RdColorField decodes a Color stored as a theme token ID followed by HSLA channels.
func RdColorField(r *Reader) Color {
	c := Color{theme: r.U16()}
	c.h, c.s, c.l, c.a = r.F32(), r.F32(), r.F32(), r.F32()
	return c
}

// PutColor encodes c onto o as either a theme token or four HSLA floats.
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

// Fill is implemented by solid Color values and LinearGradient fills accepted
// by background builders.
type Fill interface{ isFill() }

func (Color) isFill() {}

// NewLinearColorStop creates a gradient color stop at percentage (0 to 1).
func NewLinearColorStop(c Color, percentage float32) LinearColorStop {
	return LinearColorStop{Color: c, Percentage: percentage}
}

// LinearGradient defines a two-stop linear gradient background fill.
type LinearGradient struct {
	Angle    float32
	From, To LinearColorStop
	Oklab    bool
}

func (LinearGradient) isFill() {}

// NewLinearGradient constructs a LinearGradient at angle degrees between from and to.
func NewLinearGradient(angle float32, from, to LinearColorStop) LinearGradient {
	return LinearGradient{Angle: angle, From: from, To: to}
}

// InOklab returns a copy of g interpolated in the Oklab color space.
func (g LinearGradient) InOklab() LinearGradient { g.Oklab = true; return g }

// PutFill encodes a solid Color or LinearGradient onto o.
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
		Stop("an unknown gk.Fill was given to a builder: give a gk.Color or a gk.LinearGradient")
	}
}

// Length represents a GPUI CSS-like length in pixels, rems, relative fraction,
// or auto.
type Length struct {
	kind uint8
	v    float32
}

// Px returns an absolute length in logical pixels.
func Px(v float32) Length { return Length{0, v} }

// Rems returns an absolute length in root-em units.
func Rems(v float32) Length { return Length{1, v} }

// Relative returns a fraction (0 to 1) of the parent's dimension.
func Relative(v float32) Length { return Length{2, v} }

// Auto returns an automatic layout length.
func Auto() Length { return Length{3, 0} }

// PutLength encodes any Length onto o.
func PutLength(o *Tape, l Length) {
	o.U8(l.kind)
	o.F32(l.v)
}

// PutDefiniteLength encodes a non-Auto Length onto o, stopping if l is Auto.
func PutDefiniteLength(o *Tape, l Length) {
	if l.kind > 2 {
		Stop("gk.Auto() was given to a builder that takes a definite length: give gk.Px, gk.Rems or gk.Relative%s", origin(nil, false))
	}
	PutLength(o, l)
}

// PutAbsoluteLength encodes a Px or Rems Length onto o, stopping if l is
// Relative or Auto.
func PutAbsoluteLength(o *Tape, l Length) {
	if l.kind > 1 {
		Stop("gk.Relative or gk.Auto was given to a builder that takes an absolute length: give gk.Px or gk.Rems%s", origin(nil, false))
	}
	PutLength(o, l)
}

// ID accepts an element identifier: a string, int, int64, uint64, or NamedID.
type ID interface{}

// NamedID pairs a string prefix with an integer index as a composite element ID.
type NamedID struct {
	Name string
	N    int
}

// Key constructs a NamedID from name and index n.
func Key(name string, n int) NamedID { return NamedID{name, n} }

// PutID encodes an element ID onto o.
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

// Range is a half-open non-negative integer range [Lo, Hi).
type Range struct{ Lo, Hi int }

// PutRange validates that r.Lo and r.Hi are non-negative and encodes both as uint64.
func PutRange(o *Tape, r Range) {
	if r.Lo < 0 || r.Hi < 0 {
		Stop("a negative number was given in a range (%d to %d) where the kit takes byte offsets or indices: give 0 or more%s", r.Lo, r.Hi, origin(nil, false))
	}
	o.U64(uint64(r.Lo))
	o.U64(uint64(r.Hi))
}

// PutUsize validates that v is non-negative and encodes it as uint64.
func PutUsize(o *Tape, v int) {
	if v < 0 {
		Stop("a negative number (%d) was given where the kit takes a count, size or index%s", v, origin(nil, false))
	}
	o.U64(uint64(v))
}

// PutNonzero validates that v != 0 and encodes it with width size (1, 2, 4, or 8 bytes).
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

// PutMs encodes duration d rounded to the nearest non-negative millisecond as uint32.
func PutMs(o *Tape, d time.Duration) {
	ms := (max(int64(d), 0) + 500_000) / 1_000_000
	o.U32(uint32(min(ms, int64(^uint32(0)))))
}

// PutObj encodes an object handle onto o and, during a view build, pins the
// object to the current view's build number on the Rust side so it stays live
// while the frame renders.
func PutObj(o *Tape, h uint64) {
	o.U64(Held(h))
	if Vb.Active && Vb.view != nil && h != 0 {
		req := NewReq(CObjPin)
		req.U64(Vb.view.h)
		req.U32(Vb.view.buildNo)
		req.U64(h)
		call(*req)
	}
}

// PutViewArg encodes a View handle argument onto o and, during a view build,
// records it in Vb.slots and schedules it for initial description if needed.
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
