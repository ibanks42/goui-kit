package rt

import (
	"fmt"
	"os"
	"reflect"
)

// KeyMods is a set of modifier keys.
type KeyMods uint8

const (
	ModControl  KeyMods = 1
	ModAlt      KeyMods = 2
	ModShift    KeyMods = 4
	ModPlatform KeyMods = 8
	ModFunction KeyMods = 16
)

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

// PointerButton is a mouse button.
type PointerButton uint8

const (
	ButtonLeft    PointerButton = 0
	ButtonRight   PointerButton = 1
	ButtonMiddle  PointerButton = 2
	ButtonBack    PointerButton = 3
	ButtonForward PointerButton = 4
	ButtonNone    PointerButton = 255
)

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

// ClickKind is what made a click.
type ClickKind uint8

const (
	ClickMouse    ClickKind = 0
	ClickKeyboard ClickKind = 1
	ClickTouch    ClickKind = 2
)

func (k ClickKind) String() string {
	return [...]string{"Mouse", "Keyboard", "Touch", "?"}[min(int(k), 3)]
}

// ClickEvent is GPUI's ClickEvent.
type ClickEvent struct {
	Position   [2]float32
	Button     PointerButton
	Modifiers  KeyMods
	ClickCount uint32
	Kind       ClickKind
}

// KeyEvent is GPUI's KeyDownEvent or KeyUpEvent.
type KeyEvent struct {
	Key       string
	KeyChar   Option[string]
	Modifiers KeyMods
	IsHeld    bool
}

// MouseEvent is GPUI's MouseDownEvent, MouseUpEvent or MouseMoveEvent.
type MouseEvent struct {
	Position   [2]float32
	Button     PointerButton
	Modifiers  KeyMods
	ClickCount uint32
}

// TestPayload is the payload of the runtime's own test shape.
type TestPayload struct {
	Text string
	N    uint32
}

func RdClickEvent(r *Reader) ClickEvent {
	return ClickEvent{[2]float32{r.F32(), r.F32()}, PointerButton(r.U8()), KeyMods(r.U8()), r.U32(), ClickKind(r.U8())}
}

func RdKeyEvent(r *Reader) KeyEvent {
	return KeyEvent{r.Str(), RdOpt(r, (*Reader).Str), KeyMods(r.U8()), r.Bool()}
}

func RdMouseEvent(r *Reader) MouseEvent {
	return MouseEvent{[2]float32{r.F32(), r.F32()}, PointerButton(r.U8()), KeyMods(r.U8()), r.U32()}
}

// shapeInfo calls a handler of one shape: decodes the payload, calls, encodes the return value.
// call is false when the handler is not of the shape's Go type.
type ShapeInfo struct {
	Name string
	Call func(h any, cx *Cx, r *Reader, ret *Tape) bool
}

var Shapes [1024]ShapeInfo

func RegisterShape(n int, info ShapeInfo) {
	if Shapes[n].Call == nil {
		Shapes[n] = info
	}
}

func init() {
	// The runtime's own shapes (the generated ones are in gen_types.go).
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

// register puts a handler into the table of the view being built and returns its id, which the
// builder writes into the op.
func Register(h any, shape uint16) uint32 {
	if !Vb.Active {
		Stop("a handler for an element was given outside a view function: give it to an element's builder (.OnClick) while the view function describes it")
	}
	checkShape(h, shape)
	Vb.handlers = append(Vb.handlers, handlerRec{h, shape})
	return uint32(len(Vb.handlers) - 1)
}

type kept struct {
	h       handlerRec
	gen     uint32
	live    bool
	owner   uint64 // the view marked dirty when the handler returns; 0 for none
	retired bool
}

var (
	keptTab  []kept
	keptFree []uint32
)

func keptLive() int { return len(keptTab) - len(keptFree) }

// keepHandler stores a handler until dropHandler: for subscriptions and anything else the library
// calls back outside a view's tape. It belongs to the view whose init or handler is running.
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

// dropKeptOf drops the kept handlers of a view being released (its timers, subscriptions,
// listeners, requests), so none runs again for a view that is gone.
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

// putKeptHandler writes a handler given to a call op (a state's method, a builder of its
// constructor): kept, and dropped with the state when it is released (PROTOCOLS.md 9.12, 9.13).
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

// keptAttach gives the handlers kept for the call being made to handle h.
func keptAttach(h uint64) {
	if len(keptPending) == 0 {
		return
	}
	if h != 0 {
		keptBy[h] = append(keptBy[h], keptPending...)
	}
	keptPending = keptPending[:0]
}

// keptAttachReq: the handle a state's write or call names right after its op.
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
			// Every description the library holds keeps its table (retireTable), so an event
			// for a live view always finds its handler: one that does not is a bug in the
			// binding, which would otherwise drop a click without a word.
			panic(&Error{Msg: fmt.Sprintf("an event came for handler %d of description %d of view %#x, whose handler table is gone (the view's description is %d): the library and the package disagree about which descriptions are alive. This is a bug in the binding; please report it", id, gen, view, v.gen)})
		}
	}
	if !found {
		// The view was released, or the subscription or timer that named this handler. The
		// library falls back to the default return value.
		dropped++
		if debug {
			fmt.Fprintf(os.Stderr, "gk: dropped event for view %#x generation %d handler %d\n", view, gen, id)
		}
		return
	}
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
			// A handler that takes no event fits every shape (a kept handler given where another
			// shape fires): it runs, and the library gets the default return value.
			if f, ok := h.fn.(func(*Cx)); ok {
				f(cx)
				goto ran
			}
			Stop("an event of kind %d arrived for a handler of type %T (%s expected): the library and the package disagree. Build the app against the go/ folder of the download its libgpuikit.so came from", shape, h.fn, info.Name)
		}
	}
ran:
	if view == 0 {
		timerFired(id) // a one-shot timer's handler is forgotten once it has run
	}
	// The view that registered the handler is marked dirty when it returns; an unchanged tape
	// costs one view function and a compare.
	if owner != 0 && !t.quiet {
		t.markDirty(owner)
	}
	t.quiet = false
}

// NewHandler keeps a handler that returns a bool and belongs to no tape, and returns its slot
// (the runtime's tests fire it with TestFire).
func NewHandler(f func(cx *Cx) bool) uint32 {
	if cur == nil {
		Stop("gk.NewHandler was called before the app started: call it in the function given to gk.Run, in init or in a handler")
	}
	slot, _ := KeepHandler(cur.cx, f, ShapeUnitBool)
	return slot
}
