package rt

import (
	"fmt"
	"os"
	"reflect"
)

// KeyMods is a bitmask of keyboard modifier keys held during an input event.
type KeyMods uint8

// Modifier bit flags for KeyMods.
const (
	ModControl  KeyMods = 1
	ModAlt      KeyMods = 2
	ModShift    KeyMods = 4
	ModPlatform KeyMods = 8
	ModFunction KeyMods = 16
)

// String formats the active modifier flags in brace-delimited form.
func (m KeyMods) String() string {
	s := "{"
	for i, n := range []string{"Control", "Alt", "Shift", "Platform", "Function"} {
		if m&(1<<i) != 0 {
			if len(s) > 1 {
				s += ", "
			}
			s += n
		}
	}
	return s + "}"
}

// PointerButton identifies a mouse or pointer button.
type PointerButton uint8

// Pointer button constants matching the wire protocol.
const (
	ButtonLeft    PointerButton = 0
	ButtonRight   PointerButton = 1
	ButtonMiddle  PointerButton = 2
	ButtonBack    PointerButton = 3
	ButtonForward PointerButton = 4
	ButtonNone    PointerButton = 255
)

// String returns the human-readable name of pointer button b.
func (b PointerButton) String() string {
	switch b {
	case ButtonLeft:
		return "Left"
	case ButtonRight:
		return "Right"
	case ButtonMiddle:
		return "Middle"
	case ButtonBack:
		return "Back"
	case ButtonForward:
		return "Forward"
	case ButtonNone:
		return "None"
	}
	return fmt.Sprintf("PointerButton(%d)", uint8(b))
}

// ClickKind distinguishes mouse, keyboard, and touch click activations.
type ClickKind uint8

// Click activation sources for ClickEvent.Kind.
const (
	ClickMouse    ClickKind = 0
	ClickKeyboard ClickKind = 1
	ClickTouch    ClickKind = 2
)

// String returns the name of click kind k.
func (k ClickKind) String() string {
	return [...]string{"Mouse", "Keyboard", "Touch", "?"}[min(int(k), 3)]
}

// ClickEvent carries pointer coordinates, button, modifiers, click count, and
// activation kind for element click handlers.
type ClickEvent struct {
	Position   [2]float32
	Button     PointerButton
	Modifiers  KeyMods
	ClickCount uint32
	Kind       ClickKind
}

// KeyEvent carries key name, optional typed character, modifiers, and repeat
// state for keyboard handlers.
type KeyEvent struct {
	Key       string
	KeyChar   Option[string]
	Modifiers KeyMods
	IsHeld    bool
}

// MouseEvent carries pointer coordinates, button, modifiers, and click count
// for low-level mouse handlers.
type MouseEvent struct {
	Position   [2]float32
	Button     PointerButton
	Modifiers  KeyMods
	ClickCount uint32
}

// TestPayload is a diagnostic event payload used by protocol tests.
type TestPayload struct {
	Text string
	N    uint32
}

// RdClickEvent decodes a ClickEvent from r.
func RdClickEvent(r *Reader) ClickEvent {
	return ClickEvent{[2]float32{r.F32(), r.F32()}, PointerButton(r.U8()), KeyMods(r.U8()), r.U32(), ClickKind(r.U8())}
}

// RdKeyEvent decodes a KeyEvent from r.
func RdKeyEvent(r *Reader) KeyEvent {
	return KeyEvent{r.Str(), RdOpt(r, (*Reader).Str), KeyMods(r.U8()), r.Bool()}
}

// RdMouseEvent decodes a MouseEvent from r.
func RdMouseEvent(r *Reader) MouseEvent {
	return MouseEvent{[2]float32{r.F32(), r.F32()}, PointerButton(r.U8()), KeyMods(r.U8()), r.U32()}
}

// ShapeInfo describes how to decode and invoke a handler closure for one wire
// event shape ID.
type ShapeInfo struct {
	Name string
	Call func(h any, cx *Cx, r *Reader, ret *Tape) bool
}

// Shapes maps event shape opcodes to their Go dispatch adapters. Hand-written
// core shapes register in init below; generated packages register theirs on init.
var Shapes [1024]ShapeInfo

// RegisterShape installs info for shape ID n if no adapter is already registered.
func RegisterShape(n int, info ShapeInfo) {
	if Shapes[n].Call == nil {
		Shapes[n] = info
	}
}

func init() {
	RegisterShape(ShapeClick, ShapeInfo{"func(cx *Cx, e ClickEvent)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, ClickEvent))
		if ok {
			f(cx, RdClickEvent(r))
		}
		return ok
	}})
	RegisterShape(ShapeUnitBool, ShapeInfo{"func(cx *Cx) bool", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx) bool)
		if ok {
			ret.Bool(f(cx))
		}
		return ok
	}})
	RegisterShape(ShapeUnit, ShapeInfo{"func(cx *Cx)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx))
		if ok {
			f(cx)
		}
		return ok
	}})
	RegisterShape(ShapeUsize, ShapeInfo{"func(cx *Cx, e int)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, int))
		if ok {
			f(cx, int(r.U64()))
		}
		return ok
	}})
	RegisterShape(ShapeTest, ShapeInfo{"func(cx *Cx, e TestPayload) string", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, TestPayload) string)
		if ok {
			ret.Str(f(cx, TestPayload{r.Str(), r.U32()}))
		}
		return ok
	}})
	RegisterShape(ShapeSourceTest, ShapeInfo{"func(cx *Cx, e SourceTest)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, SourceTest))
		if ok {
			f(cx, SourceTest{r.U64(), r.Str()})
		}
		return ok
	}})
	RegisterShape(ShapeAction, ShapeInfo{"func(cx *Cx, e Action)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, Action))
		if ok {
			f(cx, Action{r.U32()})
		}
		return ok
	}})
	RegisterShape(ShapeWindow, ShapeInfo{"func(cx *Cx, e Window)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, Window))
		if ok {
			f(cx, Window{MkH(r.U64())})
		}
		return ok
	}})
	RegisterShape(ShapePaths, ShapeInfo{"func(cx *Cx, e PathsDone)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, PathsDone))
		if ok {
			f(cx, PathsDone{DoneStatus(r.U8()), RdList(r, (*Reader).Str), r.Str()})
		}
		return ok
	}})
	RegisterShape(ShapePrompt, ShapeInfo{"func(cx *Cx, e PromptDone)", func(h any, cx *Cx, r *Reader, ret *Tape) bool {
		f, ok := h.(func(*Cx, PromptDone))
		if ok {
			f(cx, PromptDone{DoneStatus(r.U8()), r.U32()})
		}
		return ok
	}})
}

type handlerRec struct {
	fn    any
	shape uint16
}

// isNilFunc detects both untyped nil and typed nil function values, checking
// common callback types via a type switch before falling back to reflect.
func isNilFunc(h any) bool {
	if h == nil {
		return true
	}
	switch f := h.(type) {
	case func(*Cx, ClickEvent):
		return f == nil
	case func(*Cx):
		return f == nil
	case func(*Cx, bool):
		return f == nil
	case func(*Cx, int):
		return f == nil
	case func(*Cx, string):
		return f == nil
	case func(*Cx, MouseEvent):
		return f == nil
	case func(*Cx, KeyEvent):
		return f == nil
	}
	v := reflect.ValueOf(h)
	return v.Kind() == reflect.Func && v.IsNil()
}

func checkShape(h any, shape uint16) {
	if int(shape) >= len(Shapes) || Shapes[shape].Call == nil {
		Stop("this builder's handler kind (%d) is unknown to the package: the package's generated files are incomplete or mixed from two downloads; use one download's go/ folder", shape)
	}
	if isNilFunc(h) {
		Stop("a nil function was given as a handler: give a function (a method value like a.save, or a func literal)")
	}
}

// Register appends an element event handler to the active view build's table
// and returns its per-generation handler index.
func Register(h any, shape uint16) uint32 {
	if !Vb.Active {
		Stop("a handler for an element was given outside a view function: give it to an element's builder (.OnClick) while the view function describes it")
	}
	checkShape(h, shape)
	Vb.handlers = append(Vb.handlers, handlerRec{h, shape})
	return uint32(len(Vb.handlers) - 1)
}

// kept stores a long-lived handler (timers, global actions, subscriptions,
// completions, and state callbacks) in a generational slot table.
type kept struct {
	h       handlerRec
	gen     uint32
	live    bool
	owner   uint64
	retired bool
}

var (
	keptTab  []kept
	keptFree []uint32
)

func keptLive() int { return len(keptTab) - len(keptFree) }

// KeepHandler stores a long-lived handler in keptTab, tagging it with the
// current turn's owning view so releasing that view retires the handler.
func KeepHandler(cx *Cx, h any, shape uint16) (slot, gen uint32) {
	t := CheckCx(cx)
	checkShape(h, shape)
	k := kept{h: handlerRec{h, shape}, live: true}
	if t.owner != nil {
		k.owner = t.owner.h
	}
	if n := len(keptFree); n > 0 {
		slot = keptFree[n-1]
		keptFree = keptFree[:n-1]
		k.gen = keptTab[slot].gen
		keptTab[slot] = k
	} else {
		k.gen = 1
		keptTab = append(keptTab, k)
		slot = uint32(len(keptTab) - 1)
	}
	return slot, k.gen
}

// dropHandler invalidates slot, bumps its generation counter so stale events
// cannot match a reused slot, and returns slot to keptFree.
func dropHandler(slot uint32) {
	if int(slot) >= len(keptTab) {
		return
	}
	k := &keptTab[slot]
	if !k.live {
		if k.retired {
			k.retired = false
			keptFree = append(keptFree, slot)
		}
		return
	}
	k.h = handlerRec{}
	k.live = false
	k.gen++
	keptFree = append(keptFree, slot)
}

// dropKeptOf retires every kept handler owned by view when view is released.
// Non-timer, non-completion slots are marked retired rather than freed immediately
// until their explicit owner handle releases them via dropHandler.
func dropKeptOf(t *turn, view uint64) {
	for i := range keptTab {
		k := &keptTab[i]
		if !k.live || k.owner != view {
			continue
		}
		slot := uint32(i)
		if th, ok := timerOfSlot(slot); ok {
			releaseTimer(t, Timer{th, slot})
			continue
		}
		if completionDropKept(t, slot) {
			continue
		}
		k.h = handlerRec{}
		k.live = false
		k.gen++
		k.retired = true
	}
}

// PutKeptHandler allocates a kept handler slot, writes (slot, gen) onto o, and
// records the slot in keptPending until StateMake/StateCall/StateSet binds it
// to the target state handle.
func PutKeptHandler(o *Tape, h any, shape uint16) {
	if cur == nil {
		Stop("a handler was given outside the app's functions: give it in init, a handler or a view function, after gk.Run has started the app")
	}
	slot, gen := KeepHandler(cur.cx, h, shape)
	o.U32(slot)
	o.U32(gen)
	keptPending = append(keptPending, slot)
}

var (
	keptPending []uint32
	keptBy      = map[uint64][]uint32{}
)

func keptAttach(h uint64) {
	if len(keptPending) == 0 {
		return
	}
	if h != 0 {
		keptBy[h] = append(keptBy[h], keptPending...)
	}
	keptPending = keptPending[:0]
}

// keptAttachReq binds any pending kept handler slots to the state handle at
// bytes [2:10] of a state call/set request tape.
func keptAttachReq(req []byte) {
	if len(keptPending) == 0 {
		return
	}
	if len(req) >= 10 {
		keptAttach(le.Uint64(req[2:]))
	} else {
		keptPending = keptPending[:0]
	}
}

func keptRelease(h uint64) {
	for _, s := range keptBy[h] {
		dropHandler(s)
	}
	delete(keptBy, h)
}

// event decodes a KEvent or KDone header, resolves the handler from either
// keptTab (view == 0) or the view's current/old generation table, invokes it,
// and marks the owning view dirty unless Quiet was called.
func event(cx *Cx, r *Reader) {
	t := cx.t
	view := r.U64()
	gen := r.U32()
	id := r.U32()
	shape := r.U16()
	var h handlerRec
	found := false
	var owner uint64
	if view == 0 {
		if int(id) < len(keptTab) && keptTab[id].live && keptTab[id].gen == gen {
			k := keptTab[id]
			h, found, owner = k.h, true, k.owner
			if owner != 0 {
				t.owner = views.get(owner)
			}
		}
	} else if v := views.get(view); v != nil {
		table, ok := v.cur, gen == v.gen
		if !ok {
			table, ok = v.oldTableOf(gen)
		}
		if ok && int(id) < len(table) {
			h, found, owner = table[id], true, view
			t.owner = v
		} else if libDiag {
			panic(&Error{Msg: fmt.Sprintf("an event came for handler %d of description %d of view %#x, whose handler table is gone (the view's description is %d): the library and the package disagree about which descriptions are alive. This is a bug in the binding; please report it", id, gen, view, v.gen)})
		}
	}
	if !found {
		dropped++
		if debug {
			fmt.Fprintf(os.Stderr, "gk: dropped event for view %#x generation %d handler %d\n", view, gen, id)
		}
		return
	}
	// Fast path for the two most frequent shapes before consulting Shapes.
	if shape == ShapeClick {
		if f, ok := h.fn.(func(*Cx, ClickEvent)); ok {
			f(cx, RdClickEvent(r))
			goto ran
		}
	} else if shape == ShapeUnit {
		if f, ok := h.fn.(func(*Cx)); ok {
			f(cx)
			goto ran
		}
	}
	{
		info := Shapes[shape]
		if info.Call == nil || !info.Call(h.fn, cx, r, &t.ret) {
			// Accept a zero-argument handler (created by *Do wrappers) for any event shape.
			if f, ok := h.fn.(func(*Cx)); ok {
				f(cx)
				goto ran
			}
			Stop("an event of kind %d arrived for a handler of type %T (%s expected): the library and the package disagree. Build the app against the go/ folder of the download its libgpuikit.so came from", shape, h.fn, info.Name)
		}
	}
ran:
	if view == 0 {
		timerFired(id)
	}
	if owner != 0 && !t.quiet {
		t.markDirty(owner)
	}
	t.quiet = false
}

// NewHandler stores a bool-returning diagnostic handler in keptTab and returns
// its slot index.
func NewHandler(f func(cx *Cx) bool) uint32 {
	if cur == nil {
		Stop("gk.NewHandler was called before the app started: call it in the function given to gk.Run, in init or in a handler")
	}
	slot, _ := KeepHandler(cur.cx, f, ShapeUnitBool)
	return slot
}
