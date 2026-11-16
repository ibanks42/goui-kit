# Protocols: what the runtime gives the generator

This file is the contract between the hand-written runtime and the generated code. It says
what bytes go over the wire, which Rust module generated Rust may call (`core/src/genapi.rs`),
and which procedures generated Odin and Go may call (`odin/gk/rt_emit.odin`,
`go/gk/rt_emit.go`). It is written for people working on the binding itself; you don't need
it to write an app. For the reasoning behind the design, see [DESIGN.md](DESIGN.md),
sections 5 to 9 and 11.

Section numbers are stable: source comments cite them ("PROTOCOLS.md 9.41"), so a section is
edited in place and never renumbered. Some numbers (10, 11.29, 12.10 to 12.13, 13.22 to
13.29) were never used. Section 8 says what is tested. Anything still marked "not built"
already has its wire format fixed here.

The library's backend is gpui-fast by default; it also builds against upstream gpui. Nothing
in this file depends on which one is used, except where a passage names gpui-fast
explicitly.

## 1. Numbers

Protocol numbers are in `core/wire/ops.txt` (classes `abi`, `feat`, `kind`, `err`, `hkind`,
`shape`, and the runtime's own `call` and `elop` rows). `core/wire/gen.py` writes
`core/src/ops.rs`, `core/include/gk_ops.h` and `odin/gk/ops.odin` from it.

Reserved ranges, so the generator's `api/opcodes.lock` never collides with the runtime:

| class | runtime and hand sample | generator |
|---|---|---|
| `elop` (element ops) | 1 to 63 | 256 and up |
| `call` (call ops) | 1 to 63 | 256 and up |
| `node` (node types) | 1 to 63 | 256 and up |
| `shape` (closure shapes) | 1 to 63 | 64 to 1023 |
| `style`, `color`, `theme` | the sample rows, until the generator moves them | all, afterwards |
| `kind`, `err`, `hkind`, `feat`, `abi` | all | none |

Element ops every decoder understands (the runtime's): `CHILD 1` (u32 node), `CHILD_VIEW 2`
(u64 view handle), `TEXT 3` (str), `STYLE 4` (u16 count, u16 token each).

## 2. Scalars and records on the wire

Little-endian, no padding, no alignment.

| wire type | bytes | Rust | Odin |
|---|---|---|---|
| `u8 u16 u32 u64 i32 i64 f32 f64` | as named | the same | the same |
| `bool` | 1 | `bool` | `bool` |
| `usize` / `isize` | 8 | `usize` | `int` (as i64), or `u64` |
| `str` | u32 length, UTF-8 | `&str`, `String`, `SharedString` | `string` written; `gk.Temp_String` read |
| `opt<T>` | u8 presence, then T | `Option<T>` | `Maybe(T)` |
| `list<T>` | u32 count, then each | `Vec<T>`, `&[T]` | `[]T` (read: in the turn's arena) |
| `enum:<T>` (no data) | the Odin enum's backing integer, u8 unless the generator says u16 | a number from `opcodes.lock`, never a Rust discriminant | `enum u8` |
| `rec:<T>` | fields in declaration order | struct | struct with the fields in the same order |
| enum with data | u16 variant number from 0, then that variant's fields | enum | `union` whose variants are structs, in the same order |
| `handle` | u64 | `u64` | a struct whose first field is `h: u64` |
| `handler:<shape>` | u32 handler id | `genapi::HandlerRef` (read with `Dx::handler`) | `gk.Handler` (written with `register`) |
| fixed array `[N]T` | N times T | `[T; N]`, `Point` as `[2]f32` | `[N]T` |
| flags | the backing integer | bit tests | `bit_set[E; u8]` |

The Odin runtime reads event payloads and writes handler return values **by reflection over
the Odin type**, following exactly this table. So an event record needs no decoder in Odin:
the generator emits a plain struct with the fields in wire order and one
`register_shape` line (section 4). In Rust each payload type implements `genapi::Wire`
(`fn put(&self, out: &mut Vec<u8>)`) and each return type `genapi::Unwire`.

Built-in records (Rust impls in `genapi.rs`, Odin types in `rt_handler.odin`):

- `Click_Event`: `[2]f32 position`, `u8 button` (0 left, 1 right, 2 middle, 3 back,
  4 forward), `u8 modifiers` (bits: 1 control, 2 alt, 4 shift, 8 platform, 16 function),
  `u32 click_count`, `u8 kind` (0 mouse, 1 keyboard, 2 touch).
- `Key_Event`: `str key`, `opt<str> key_char`, `u8 modifiers`, `bool is_held`.
- `Mouse_Event` (down, up, move): `[2]f32 position`, `u8 button` (255 = none),
  `u8 modifiers`, `u32 click_count`.

## 3. Turns

One callback, `gk_callback(user, cb, kind, req, len)`, on the UI thread.

| kind | request | when |
|---|---|---|
| `INIT 1` | empty | once, inside `gk_run` |
| `WINDOW_INIT 2` | u64 window, u64 root view | inside `open_window`, window in hand |
| `FRAME 3` | u64 window, u64 frame number | pre-frame hook, only if the app asked (`on_frame`) |
| `VIEWS 4` | u64 window, u32 n, u64 view each | pre-frame hook, only if a mounted view is dirty |
| `EVENT 5` | u64 view, u32 generation, u32 handler, u16 shape, payload | a listener, a subscription, a queued event |
| `WAKE 7` | empty | after `gk_post`, at most one per turn of the main loop |
| `PANIC 10` | str message | the library caught a Rust panic; the binding prints it and exits 1 |

`ROWS 6` is section 9's and `DONE 8` section 11.30's. `QUIT 9` keeps its number and is not sent.

An `EVENT` with view 0 names a *kept handler* (a subscription, a global): `handler` is its
slot and `generation` the slot's generation.

The reply (`gk_reply`, copied before it returns). An empty reply means all zero. `gk_reply`
accepts only the `gk_cb` of the callback running now (`GK_ERR_NO_CONTEXT` otherwise: one kept
from an earlier callback points into a stack frame that has returned). A request shorter than
its op number, or a null one, is `GK_ERR_BAD_OP`; a request or reply whose lengths run past its
bytes, or a string that is not UTF-8, stops the library (poisoned, the message in
`gk_last_error`). Tests: `core/tests/attack` (`run-attack.sh`).

```
u32 ret_len, bytes                         the handler's return value in the shape's wire type; 0 = Default
u32 n, u64 view each                       views to mark dirty
u32 n, (u32 len, call op bytes) each       buffered writes
u32 n, (u64 view, u32 generation, u32 len, tape) each     changed tapes (VIEWS and WINDOW_INIT only)
```

When buffered writes are applied (DESIGN.md section 7, "When a write lands"):

- `EVENT`: from one deferred callback when the outermost entity update has ended, in order,
  before the next draw.
- `INIT`, `WINDOW_INIT`, `WAKE`, `FRAME`: immediately after the callback returns.
- In the pre-frame hook the order is `FRAME` turn, its writes, then every mounted view
  whose dependency list names a state that was just written is marked dirty by the runtime
  itself, then the `VIEWS` turn, then the draw. A per-frame handler's write is in the pixels
  of the frame it ran in.
- `VIEWS`: a view procedure may not write. The Odin runtime stops at the call site
  ("a view procedure may not write"); the library also refuses writes in a `VIEWS` reply.
- A write that meets a state on lease is put back and applied when the update has ended;
  a read of a state on lease returns `GK_ERR_BUSY`.

## 4. Handlers (closure parameters with the `handler` resolution)

A *shape* is one closure signature seen from the app: the payload arguments in order (the
`&mut Window`, `&mut App` or `&mut Context<T>` tail is not payload) and the return type.
Two closures with the same payload types and return type are the same shape.

Built-in shapes (`core/wire/ops.txt`):

| shape | payload | returns | upstream closures |
|---|---|---|---|
| `CLICK 1` | `Click_Event` | nothing | `Fn(&ClickEvent, &mut Window, &mut App)` |
| `UNIT_BOOL 2` | none | bool | `Fn(&mut Window, &mut App) -> bool` |
| `UNIT 3` | none | nothing | `Fn(&mut Window, &mut App)`, `Fn(&mut App)` (focus, blur, close, dismiss) |
| `BOOL 4` | bool | nothing | `Fn(&bool, ..)`, `Fn(bool, ..)` (toggle, change, hover, open) |
| `USIZE 5` | usize | nothing | `Fn(&usize, ..)` (select by index) |
| `STR 6` | str | nothing | `Fn(&str, ..)`, `Fn(&SharedString, ..)` (input, change of text) |
| `KEY 7` | `Key_Event` | nothing | `Fn(&KeyDownEvent, ..)`, `Fn(&KeyUpEvent, ..)` |
| `CLICK_BOOL 8` | `Click_Event` | bool | `Fn(&ClickEvent, ..) -> bool` (dialog ok, cancel) |
| `MOUSE 9` | `Mouse_Event` | nothing | `Fn(&MouseDownEvent, ..)`, up, move |
| `F64 10` | f64 | nothing | `Fn(&f64, ..)`, `Fn(f32, ..)` widened (slider, number) |
| `INPUT_EVENT 11` | `Input_Event` (the sample's enum with data) | nothing | `EventEmitter<InputEvent>` of `InputState` |
| `TEST 12` | `Test_Payload` (str, u32) | str | fired only by the runtime's tests |

For any other signature the generator takes the next number from 64 in `opcodes.lock`,
emits an Odin struct for the payload (one field per payload argument, in order; a single
payload argument is its own type, without a wrapping struct) and registers it.

**Element op.** A handler argument is a u32 handler id in the op's arguments.

**Generated Rust**, one line per closure parameter, any shape:

```rust
E_BUTTON_ON_CLICK => {
    let h = dx.handler(&mut r);                     // (view, tape generation, id)
    el = el.on_click(move |ev, window, cx| genapi::fire(h, SHAPE_CLICK, ev, window, cx));
}
// two payload arguments: a tuple of references
el = el.on_toggle(move |on, ev, window, cx| genapi::fire(h, SHAPE_BOOL_CLICK, &(on, ev), window, cx));
// a return value: fire is generic over it (Unwire + Default)
el = el.on_ok(move |ev, window, cx| -> bool { genapi::fire(h, SHAPE_CLICK_BOOL, ev, window, cx) });
// no window in the closure
el = el.on_close(move |cx| genapi::fire_app(h, SHAPE_UNIT, &(), cx));
// called from inside an entity's own update (delegate methods): delivered after it
genapi::fire_queued(h, SHAPE_USIZE, &row, cx);
```

`fire` makes one synchronous `EVENT` turn with the real window and app in scope, applies the
reply and returns the handler's return value. A handler that was registered without a return
value, or an event whose handler table is gone, gives `Default::default()`.

**Generated Odin**, per closure parameter:

```odin
on_click = proc(e: Button, h: Handler) -> Button {
	put_u32(op(e, E_BUTTON_ON_CLICK), register(h, SHAPE_CLICK))
	return e
},
```

and once per generated shape, in a `gen_*.odin` file:

```odin
@(init, private)
gen_shapes :: proc "contextless" () {
	register_shape(SHAPE_BOOL_CLICK, Bool_Click, nil) // shape, payload type, return type (nil = none)
}
```

**What the app writes.** `gk.on` takes a procedure in one of eight forms and, with the data
forms, one value that the binding copies and owns:

```odin
gk.on(proc(s: ^App, cx: ^gk.Cx) {..})                                   // fits every shape
gk.on(proc(s: ^App, cx: ^gk.Cx, e: gk.Click_Event) {..})                // the payload
gk.on(proc(s: ^App, cx: ^gk.Cx, id: Row_Id) {..}, row_id)               // one value of data
gk.on(proc(s: ^App, cx: ^gk.Cx, e: gk.Click_Event, id: Row_Id) {..}, row_id)
// each of the four also with a return value, for shapes that have one:
gk.on(proc(s: ^App, cx: ^gk.Cx, e: gk.Click_Event) -> bool {..})
```

`register` checks, with the line of the `gk.on` call: the state type against the view's,
the payload type against the shape's (a handler without a payload parameter fits any
shape), the return type against the shape's. When the handler returns, the view that
registered it is marked dirty (`gk.quiet(h)` opts out).

## 5. Kit state behind handles

A handle is u64: kind (8 bits), generation (24), index (32); 0 is nil. Kinds: `VIEW 1`,
`WINDOW 2`, `STATE 3` (any kit entity; the library also checks the Rust type), `ROWS 4`,
`SUB 5` (a subscription), `FOCUS 6` (a focus handle). A released, stale or wrong-type
handle is `GK_ERR_STALE_HANDLE` with a message naming both types; never undefined
behaviour. `C_RELEASE` (call op 12: u64 handle) releases any handle; the slot's generation
goes up and the index is reused.

**Generated Rust** for a state type `T`, in call-op arms (`c: &mut genapi::Call`, which
holds `cx`, the window if the turn has one, `out`, and whether the op is deferred):

```rust
C_INPUT_STATE_NEW => {                       // constructor chain: one crossing
    let window = c.window()?;                // GK_ERR_NO_CONTEXT if the turn has none; not a borrow of `c`
    let entity = c.cx.new(|cx| InputState::new(window, cx) /* then the self -> Self ops */);
    let h = genapi::state_new(entity, c);    // remembers the window; returns the handle
    c.out.put_u64(h);
}
C_INPUT_STATE_VALUE => {                     // a read (&self): GK_ERR_BUSY if on lease
    let v = genapi::state_read::<InputState, _>(r.u64(), c, |s, _cx| s.value())?;
    c.out.put_str(&v);
}
C_INPUT_STATE_SET_VALUE => {                 // a write (&mut self, .., window, cx)
    let h = r.u64();
    let v = SharedString::from(r.str());
    genapi::state_update::<InputState>(h, c, move |s, window, cx| s.set_value(v, window, cx))?;
}
C_INPUT_STATE_SUBSCRIBE => {                 // EventEmitter<E>: u64 state, u32 slot, u32 generation
    let (h, slot, generation) = (r.u64(), r.u32(), r.u32());
    let sub = genapi::state_subscribe::<InputState, InputEvent>(h, slot, generation, SHAPE_INPUT_EVENT, c)?;
    c.out.put_u64(sub);
}
```

`state_update` applies the closure with the state's own window, or, if the state is on
lease, puts the op back to run when the update has ended. It also tells the runtime the
state was written (the write rule of section 3). `state_update_cx` is the same for a
mutator that takes no window (`&mut self, .., cx: &mut Context<Self>`).

A constructor that takes `&Entity<State>` in a decode arm: `genapi::state::<T>(handle)?`
gives the `Entity<T>`.

**Generated Odin** (all in `rt_emit.odin`): a state type is
`struct {using state: State, using vt: ^T_VT}`; methods are built from

```odin
state_get(s.h, cx, req[:], loc) -> Reader         // a read: flushes buffered writes first; inside a view
                                                  // build it records the dependency (the view is
                                                  // described again when the state notifies)
state_try(s.h, cx, req[:], loc) -> (Reader, bool) // the same, false when the state is busy
state_set(cx, req[:], loc)                        // a buffered write
state_make(cx, req[:], loc) -> u64                // a constructor call
keep_handler(cx, h, shape, loc) -> (slot, generation: u32)   // for subscriptions
dep(s.h)                                          // only if a method reads without state_get
```

A method in a `_VT` struct may end its parameter list with `loc := #caller_location` (Odin
allows the default in a procedure type): `s->value(cx)` then reports the app's line when it
stops. The sample's state methods do; element builders do not need it (the element's
constructor and `gk.on` already carry the line).

## 6. App state, background posts, panics, versions

These need nothing from the generator; they are listed so the Odin API is in one place.

- `gk.Model(T)`: `gk.get(&m)` in a view records that view; `gk.set(cx, &m, v)` and
  `gk.update(cx, &m, proc)` mark the views that read it in their latest build;
  `gk.observe(cx, &m, handler)` runs a handler on every change. `gk.notify(cx, view)` marks
  one view. No crossing: marks travel in the turn's reply.
- `gk.post(procedure[, value])` from any thread: the value is copied and owned like handler
  data; the first post into an empty queue calls `gk_post`, the library makes one `WAKE`
  turn per turn of the main loop, and views marked in it are described once, in the next
  pre-frame hook.
- A Rust panic never unwinds into the app: every export and every closure the library
  hands to GPUI catches it, the library is poisoned (`GK_ERR_POISONED` from every later
  export), and the binding prints the message and exits 1 (through the `PANIC` turn when
  the panic was not inside a `gk_call`). A panic or failed assertion in app code prints the
  message and the source line and exits 2; Odin does not unwind.
- Versions: `gk_info_get()` is checked by the binding before `gk_run` (struct size, magic,
  `abi_major` equal, the library's `abi_minor` at least the binding's, `api_hash` equal;
  the `feat` bits say which backend the library was built on, gpui-fast or upstream gpui,
  and either is accepted), and `gk_run` checks the `gk_app` the same way from its side.
  While `abi_major` is 0 the hash must be equal: a library and a package from different
  generations refuse each other with both versions in the message.

## 7. What generated code may use

**Rust: `crate::genapi` only.**

```rust
pub use crate::ops::*;                         // the numbers
pub struct R<'a>;                              // reader: u8 u16 u32 u64 i32 i64 f32 f64 str bytes
pub trait Put;                                 // writer on Vec<u8>: put_u8 .. put_str
pub struct GkErr(pub i32, pub String);  pub type GkResult<T>;  pub fn err<T>(code, msg) -> GkResult<T>;

// decoding a tape
pub struct Dx<'a> { pub window: &'a mut Window, pub cx: &'a mut App, .. }
impl Dx { fn child(&mut self, node: u32) -> AnyElement;        // decodes a child node (CHILD)
          fn child_view(&mut self, handle: u64) -> Option<AnyView>;
          fn args(&self, at: u32) -> R;                        // a node's constructor arguments
          fn op(&self, at: &mut u32) -> Option<(u16, R)>;      // next op of a node; advances
          fn handler(&self, r: &mut R) -> HandlerRef;
          fn node(&self, n: u32) -> (u16, u32, u32);           // type, args, first op of any node
          fn frag(&self, node: u32) -> Frag; }                 // keeps the tape and a node for a closure
impl Frag { fn with<T>(&self, window: &mut Window, cx: &mut App, f: impl FnOnce(&mut Dx, u32) -> T) -> T; }   // Clone, 'static
pub fn style_op(style: &mut StyleRefinement, op: u16, r: &mut R, cx: &App) -> bool;
pub fn color(r: &mut R, cx: &App) -> Hsla;

// handlers
pub struct HandlerRef;                         // Copy
pub trait Wire { fn put(&self, out: &mut Vec<u8>); }
pub trait Unwire: Sized { fn get(r: &mut R) -> Self; }
pub fn fire<P: Wire + ?Sized, T: Unwire + Default>(h: HandlerRef, shape: u16, payload: &P, window: &mut Window, cx: &mut App) -> T;
pub fn fire_app<P: Wire + ?Sized, T: Unwire + Default>(h: HandlerRef, shape: u16, payload: &P, cx: &mut App) -> T;
pub fn fire_queued<P: Wire + ?Sized>(h: HandlerRef, shape: u16, payload: &P, cx: &mut App);
pub fn focus(handle: u64) -> GkResult<FocusHandle>;             // a handle of kind FOCUS
pub fn action(r: &mut R) -> GkResult<Box<dyn Action>>;          // u32 action id (section 11.1)

// call ops and state
pub struct Call<'a> { pub cx: &'a mut App, pub out: &'a mut Vec<u8>, .. }
impl<'a> Call<'a> { fn window(&mut self) -> GkResult<&'a mut Window>;   // the turn's window; not a borrow of the Call
                    fn window_handle(&self) -> Option<AnyWindowHandle>; }
// public fields: cx, out, deferred, req
pub fn state_new<T: 'static>(entity: Entity<T>, c: &mut Call) -> u64;
pub fn state<T: 'static>(handle: u64) -> GkResult<Entity<T>>;
pub fn state_read<T: 'static, V>(handle: u64, c: &mut Call, f: impl FnOnce(&T, &App) -> V) -> GkResult<V>;
pub fn state_update<T: 'static>(handle: u64, c: &mut Call, f: impl FnOnce(&mut T, &mut Window, &mut Context<T>) + 'static) -> GkResult<()>;
pub fn state_update_cx<T: 'static>(handle: u64, c: &mut Call, f: impl FnOnce(&mut T, &mut Context<T>)) -> GkResult<()>;
pub fn state_subscribe<T: EventEmitter<E> + 'static, E: Wire + 'static>(handle: u64, slot: u32, generation: u32, shape: u16, c: &mut Call) -> GkResult<u64>;
pub fn state_subscribe_with<T: EventEmitter<E> + 'static, E: 'static>(handle: u64, slot: u32, generation: u32, shape: u16, c: &mut Call, encode: fn(&E, &mut Vec<u8>)) -> GkResult<u64>;
pub fn style_token(style: &mut StyleRefinement, token: u16);    // one nullary Styled method
pub struct StyleOf<'a>(pub &'a mut StyleRefinement);            // implements Styled over a borrowed refinement
```

The generated module plugs in through three functions in `core/src/gen/mod.rs`. It is
always compiled, declared in `lib.rs` as `#[path = "gen/mod.rs"] mod generated;` because
`gen` is a reserved word in the 2024 edition. The runtime asks it first for every node type,
call op and style token, and falls back to the hand-written decoders of `core/src/hand.rs`
for what it returns `None` or `false`:

```rust
pub fn node(dx: &mut genapi::Dx, ty: u16, args: u32, first: u32) -> Option<AnyElement>;   // None: not mine
pub fn call(op: u16, r: &mut genapi::R, c: &mut genapi::Call) -> Option<genapi::GkResult<()>>;
pub fn style_token(style: &mut StyleRefinement, token: u16) -> bool;
```

`core/src/hand.rs` (a text node, the uniform list over rows, the test call ops) uses only
`genapi` and is the pattern to copy for hand-written decoders.

**Odin: `odin/gk/rt_emit.odin` only** (same package, so the names are package-private):
`Element`, `new_node`, `op`, `attach`, `slot_view`, `tokens`, `register`,
`register_shape`, `put_u8 .. put_str`, `put_color`, `Reader`, `rd_*`, `rd_temp`, `rd_into`,
`State`, `state_get`, `state_try`, `state_set`, `state_make`, `dep`, `keep_handler`,
`focus_new`, `focus_set`, `Handler`, `Temp_String`, `Cx`, `Subscription`, `Color`, `hsla`,
`Hsla`, and the built-in events `Click_Event`, `Key_Event`, `Mouse_Event` with their own
`Pointer_Button`, `Key_Mods` and `Click_Kind`. The header of `rt_emit.odin` lists the
names the runtime leaves to the generated package. `odin/gk/kit_bridges.odin` holds the few
hand-written bridges that sit beside the generated types.

## 8. Status

Everything in sections 1 to 7 is built. The test is `core/tests/run.sh` (`make proto`): one test
app, `core/tests/proto`, run offscreen with the diag library in 19 modes; each mode checks
an exit code and the lines the app printed.

| protocol | test mode | what it shows |
|---|---|---|
| handlers of every shape | `handlers` | click (`Click_Event`), toggle (bool, a Switch), hover (bool, `quiet`), click with owned data, mouse down (`Mouse_Event`), key down (`Key_Event`, twice, one with shift), bool returns, `(str, u32) -> str` with and without data, default return |
| registration checks | `shape`, `state-type` | a handler whose event or state type does not fit stops at the `gk.on` line, exit 2 |
| kit state behind handles | `state` | constructor chain, a read in a view as a dependency (described again per keystroke), a subscription (`Input_Event`), read-your-writes, unsubscribe |
| handle checks | `stale`, `stale-write`, `wrong-type` | a read through a released handle or one of another kind stops at its line; a buffered write to a released handle stops when it is applied, after its handler, with the hint to run with `GK_DEBUG=1` for the line; exit 2 each |
| leases and queued events | `lease` | inside the state's own update a read is busy and a write lands after; a queued event runs after the update |
| the write rule | `write-rule`, `frames`, `view-write` | a per-frame write is described in the same frame; a frame handler alone runs every frame; a view procedure that writes stops |
| app state | `model` | `gk.Model`: only the view that read it is described again; observers run |
| background posts | `post` | 20,001 posts from another thread: all delivered, 5 `WAKE` turns, 7 descriptions over 11 frames |
| panics | `panic-odin`, `panic-call`, `panic-later`, `thread` | app panic exit 2 with its line; library panic exit 1 inside a call and outside one (the `PANIC` turn); a UI call from another thread stops |
| versions | `version`, `version-lib` | the library refuses five wrong `gk_app`s; the binding refuses a library with another hash |
| every handler shape, generated (`core/tests/gen`, `run-gen.sh`) | `shapes` | each of the 45 shapes the package registers fired once through the library (call op `TEST_FIRE_SHAPE 26`) with a payload `gen/gk_tests.py` writes from the wire model: 36 decoded into the Rust payload types and fired through their own `Wire` impls (typed), 9 fired as bytes (raw: a `ClickEvent`, `KeyDownEvent` or `MouseDownEvent` in the payload, or no payload); the handler, registered with `gk.on` for the registered type, gets back exactly the bytes sent, and its return value arrives |
| every generated state type | `states` | 25 of 27 state types made through their builder chains, every read, write and call once with fixed arguments, every event subscribed (13), read again after the writes were applied, released; `List_State` and `Table_State` take a row source and are the rows tests' |

The `ROWS` turn is section 9's, `DONE` is 11.30's, and actions, key bindings and timers are
11.1 and 11.2, each with its own tests. The `QUIT` turn kind keeps its number and is not
sent. Focus events of the kit's Input (`Input_Focus`, `Input_Blur`) are encoded but don't
fire offscreen, where the window is never active, so they are untested.

## 9. Delegates and row sources

The kit asks the app things through traits the app implements: the list,
table and searchable-list delegates, the tree's render closure, and the input's completion,
hover, code-action, definition and document-colour providers. Upstream calls those methods
while it lays out and draws. The binding never does: **app code never runs in the middle of
a frame** (user decision 3). Every trait is implemented once in Rust by a *shim* that answers
from data the app pushed, draws rows the app described earlier, and turns everything else
into events or requests delivered between frames. Handlers and kit state follow sections 4
and 5 unchanged. Sections 9.1 to 9.4 are the shared core, built and tested
(`core/src/source.rs`, `odin/gk/rt_source.odin`, test mode `rows`); 9.5 to 9.8 are the two delegate
families built on it.

### 9.1 Row sources

A *row source* (handle kind `SOURCE 7`) holds the rows of one list, table or tree. A row is
named by a key, `u64 section << 32 | row` (section 0 when there are none). **Each row is a
view**: a `GkView` with its own tape, described by the app's row procedure for that key,
retained by gpui-fast like any view, and described again only when it is marked dirty (by
`gk.invalidate`, by its own handlers, by kit state or a `gk.Model` it read). Its handlers,
their data and their events are exactly those of a view (section 4: `EVENT` with the row
view's handle).

- **Pull by range.** While GPUI draws, a shim asks `source::row(src, key, window, cx)`. It
  gets `Row::View(AnyView)` for a row whose view has a tape, or `Row::Missing`: the shim
  draws a placeholder of the row's height (the kit's skeleton where the kit has one), the
  key is recorded as wanted and the window's pre-frame hook is armed. Nothing calls the app.
- **The ROWS turn** (`kind 6`), in the pre-frame hook after `FRAME` and before `VIEWS`, for
  every source of that window with wanted keys. The library extends the wanted rows of each
  section by as many again on each side (clamped to 8 ..= capacity / 4), makes a view for
  every extended key that has none, evicts the farthest bound rows beyond the source's
  capacity, and sends

  ```
  u64 window, u32 n_sources, per source: u64 source,
      u32 n_new, (u64 key, u64 view) each, u32 n_evicted, (u64 view) each
  ```

  The binding binds each new view to its key (a view record whose procedure is the source's
  row procedure for that key) and forgets the evicted ones. No app code runs in it; the reply
  is empty, and a write in it is refused as in `VIEWS`. The library then releases the
  evicted views and marks the new ones dirty, so the `VIEWS` turn right after describes
  them, and when their tapes are accepted it notifies every entity that drew the source
  (`source::watch`), so **the same frame shows them**. A far jump therefore shows
  placeholders for exactly one frame; scrolling within the overscan shows none.
- **The first rows** come with the source: `C_SOURCE_NEW` binds rows `0 ..< initial` at once
  and marks them dirty, so the first frame that draws the source is complete.
- **Counts and changes.** `gk.set_count(cx, src, n, section)` drops rows at and past `n`
  (the binding forgets them at once; the library releases their views when the write is
  applied). `gk.invalidate(cx, src, lo, hi, section)` marks bound rows dirty; unbound rows
  are described when they are drawn. Releasing the source releases all its rows. A section
  holds at most `MAX_ROWS` (4,294,967,294) rows, a table at most that many cells (rows times
  columns): a row's number is 32 bits and the two highest are the header and footer. A larger
  count stops in the binding (and is `GK_ERR_BAD_OP` in the library).

Cost: 0 crossings per frame while scrolling through bound rows; one ROWS turn and one VIEWS
turn per frame that drew placeholders; each row is described once per change.

### 9.2 Events and requests

- **Events** (selection, confirm, cancel, sort, column moved, load more, visible range): the
  shim calls `source::event(src, SHAPE_X, &payload, cx)`. It is delivered to the handler given
  to the source, as a *queued* event (section 4, `fire_queued`): after the kit's own update
  has ended and before the next draw. Each family has its own shape and union (9.5, 9.6); a
  source without a handler drops its events.
- **Requests** (a trait method that returns a `Task`): `source::request(src, SHAPE_X,
  &payload, cx)` fires the event with a new request id first, `(u64 id, payload..)`, and
  returns a `oneshot::Receiver<Vec<u8>>`. The app answers whenever it likes with
  `gk.answer(cx, src, id, value)`, a buffered write (`C_SOURCE_ANSWER`) whose bytes are
  `value` in the wire type the family fixed (section 2, written by reflection); the shim's
  task decodes them with `Unwire`. A request answered twice, or never asked, is
  `GK_ERR_BAD_OP`; an answer whose receiver has gone is dropped. Tasks are spawned with
  `cx.foreground_executor().spawn(async move { rx.await ... })` or returned as upstream's
  `Task` type directly.
- **Data the kit asks for synchronously** (counts, column definitions, `loading`, `has_more`,
  trigger characters, a provider's `id`): pushed by the app with buffered writes and kept in
  the shim; never a turn.

### 9.3 Wire

| number | what |
|---|---|
| `kind ROWS 6` | the ROWS turn above |
| `hkind SOURCE 7` | a row source |
| `call SOURCE_NEW 20` | `u64 count, u32 initial, u32 capacity, u8 has_events, u32 slot, u32 generation` (a kept handler, section 5) -> `u64 source, u32 n, (u64 key, u64 view) each`. Needs the turn's window |
| `call SOURCE_SET_COUNT 21` | `u64 source, u32 section, u64 count` (buffered) |
| `call SOURCE_SCROLL_TO 22` | `u64 source, u64 row` (buffered): the core list's scroll handle |
| `call SOURCE_ANSWER 23` | `u64 source, u64 id, u32 len, bytes` (buffered) |
| `call TEST_SOURCE_REQUEST 24`, `TEST_SOURCE_TAKE 25` | the runtime's tests |
| `node SOURCE_LIST 9` | the core's own list element: `str id, u64 source, f32 row height`, styled ops |
| `shape SOURCE_TEST 13` | `Source_Test {u64 id, str text}`, the tests' request |

Reserved for the two delegate families, so each can add numbers without touching the
other's lines of `core/wire/ops.txt` (each appends its block at the end of the class):

| class | input family | rows family |
|---|---|---|
| `call` | 32 to 47 | 48 to 63 |
| `shape` | 14, 15 | 16 to 19 |
| `node` | 10, 11 | 12 to 15 |

`ABI_MINOR` is 4 from this section on; a family that adds a turn kind or changes a reply
raises it again.

### 9.4 The shared core's API

Rust (`core/src/source.rs`; shims are hand-written library code, not generated, and may use
it directly):

```rust
pub fn key(section: u32, row: u32) -> u64;
pub enum Row { View(AnyView), Missing }
pub fn row(src: u64, key: u64, window: &mut Window, cx: &mut App) -> Row;    // never calls the app
pub fn count(src: u64, section: u32) -> GkResult<usize>;
pub fn watch(src: u64, entity: EntityId);                 // notified when rows it drew arrive
pub fn event<P: Wire + ?Sized>(src: u64, shape: u16, payload: &P, cx: &mut App);
pub fn request<P: Wire + ?Sized>(src: u64, shape: u16, payload: &P, cx: &mut App)
    -> GkResult<oneshot::Receiver<Vec<u8>>>;
pub fn scroll_handle(src: u64) -> GkResult<UniformListScrollHandle>;     // the core list's
// genapi: released_shown(msg, what) -> !    a released source a view still draws stops (exit 2)
```

Odin (`odin/gk/rt_source.odin`):

```odin
Row_At :: struct {section, row: int}
Row_Source :: struct {h: u64}
row_source :: proc(cx, state: ^$S, count: int, row: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element,
                   events: Handler = {}, events_shape: u16 = 0, initial := 64, capacity := 512) -> Row_Source
set_count(cx, src, count, section := 0)    invalidate(cx, src, lo, hi, section := 0)
answer(cx, src, id: u64, value: $T)        release(cx, src)
source_list(id, src, row_height, ..flags) -> Uniform_List      scroll_to(cx, src, row)
```

A row procedure is a view procedure with the key: it may read kit state and models (the row
is described again when they change), may not write, and registers handlers whose state is
`^S`. `row_source` needs a window (init or a handler), like kit state.

### 9.5 Rows family: lists, tables, searchable lists, trees

- **`ListDelegate`** → `GkListDelegate { src: u64 }` with `type Item = GkRow`, a small element
  that wraps `Row::View` in the kit's `ListItem` (so selection styling stays the kit's) or a
  skeleton placeholder, and implements `Selectable`. `sections_count` and `items_count` read
  the source's counts; `render_item(ix)` is `source::row(src, key(ix.section, ix.row))`;
  section headers and footers are rows of their own with the key `section << 32 |
  0xffff_fffe` and `0xffff_ffff` (the row procedure is told by `Row_At.row` being
  `gk.HEADER` or `gk.FOOTER`); `render_empty`, `render_loading` and `render_initial` show a
  `gk.View` the app set (data), or upstream's default when none. `set_selected_index`,
  `set_right_clicked_index`, `confirm`, `cancel`, `load_more` are `List_Event` variants
  (shape 16). `perform_search(query)` is a request (shape 16, `List_Search{id, query}`); its
  task completes when the app answers, after it has set the new counts. `loading`,
  `has_more`, `load_more_threshold` are data.
- **`TableDelegate`** → `GkTableDelegate { src }`: `columns_count` and `column(ix)` are data
  (the kit's `Column` is plain data: `gk.set_columns(cx, table, []gk.Column)`), `rows_count`
  is the source's count. A row's view describes the whole row: its root is a
  `gk.table_row()` node whose children are the cells in column order; `render_tr(row)` gives
  the row view and `render_td(row, col)` decodes child `col` of that row's tape with the row
  view's handle (so cell handlers are the row's). `cell_text`/`cell_label`/`row_label` are
  data (an optional text column the app sets) or empty. `perform_sort`, `move_column`,
  `visible_rows_changed`, `visible_columns_changed`, `load_more` are `Table_Event` variants
  (shape 17); `render_th` shows the column's name (upstream default) unless a header view is
  set; `context_menu` is a menu the app describes as data, with the row in its events.
- **`SearchableListDelegate`** → `GkSearchableDelegate`: as the list delegate, with
  `perform_search` as a request; `SearchableListItem` values are rows (title as data for
  `matches`, the rendering as the row's view).
- **Tree**: `tree(&state, render_item)` gets a render closure over a source whose keys are
  the tree's flat indices; the row procedure reads the entry it draws through the
  `TreeState` handle (`tree->entry(cx, at.row)`: a state read in a view build, so every
  bound row is described again when the tree state changes, e.g. on expand).
- **States.** `ListState<GkListDelegate>`, `TableState<GkTableDelegate>`: instantiated by the
  generator rule of 9.6 (`"ListState<crate::shim_rows::GkListDelegate> as ListState"`), so
  their 17 + 34 methods (scroll, select, column widths) are generated. Odin: `gk.list_state(cx, state, count,
  row, events) -> List_State` holds the state handle and the source handle;
  `gk.list(list_state)` is the element.

### 9.6 Input family: InputState, EditorState, providers

- **Instantiating the generic state.** Upstream declares `InputBaseState<M>`, with `type
  InputState = InputBaseState<InputMode>` and `type EditorState = InputBaseState<EditorMode>`
  (155 functions, "state types generic over a delegate or mode"). Decision: the generator
  instantiates a generic entity state for a fixed argument list, named by a new list in
  `api/overrides.toml`:

  ```toml
  [instantiate]
  types = ["Stateful<Div>"]
  # Generic entity states, one state type per line: "Generic<Arg> as Name". Arg is an upstream
  # type or a shim type of the library (crate::...). Owned by the family that adds the line.
  states = ["InputBaseState<InputMode> as InputState", "InputBaseState<EditorMode> as EditorState"]
  ```

  Each line makes one state type (`Input_State`, `Editor_State` in Odin, the alias's name):
  every method of a generic impl (`impl<M: InputModeKind> InputBaseState<M>`) whose bounds
  the argument meets, every method of the impl for exactly that argument
  (`impl InputBaseState<EditorMode>`), `Self` and `M` substituted; a method whose
  where-clause the argument does not meet is skipped with the reason `generic-bound`.
  Constructors and the `self -> Self` builder chain are one constructor op, as section 5
  says. The rule lives in `gen/gk_model.py` (`classify_state`, `add_node` for states) and
  the emitters; the rows family only adds its lines.
  The Rust type is `gpui_kit::base::input::InputBaseState<gpui_kit::base::input::InputMode>`,
  the same `TypeId` as the alias, so handles stay compatible, and the generated names give
  the calls (`gk.input_state(cx)->placeholder(..)->build()`, `->value(cx)`, `->set_value`,
  `->focus`, `->on_event`) working.
- **Providers.** The `Lsp` of an input holds `Rc<dyn CompletionProvider>` and the like. Each
  provider is a shim over a row source used only for its request channel (count 0):
  `GkCompletionProvider { src }`, `GkHoverProvider`, `GkCodeActionProvider`,
  `GkDefinitionProvider`, `GkDocumentColorProvider`. Installing one is a call op of the
  family: `editor->set_completions(cx, src)` sets `lsp_mut().completion_provider`.
  - `completions(text, offset, trigger)` is a request (shape 14, `Provider_Request` union,
    variant `Completions{id, text: str, offset: usize, trigger_kind: u8, trigger: opt<str>}`;
    the text of the rope is copied, DESIGN.md section 12). The answer is
    `Completion_Response{items: []Completion_Item, incomplete: bool}`, the LSP fields as
    plain records and JSON text where LSP has open shapes. `hover`, `code_actions`,
    `perform_code_action`, `definitions`, `document_colors` the same, one variant each.
  - `is_completion_trigger(offset, new_text)` is data: the trigger characters the app set,
    plus word characters when the app asked for them. `inline_completion_debounce` and `id`
    are data. `resolve_completions` and `inline_completion` keep upstream's defaults until
    asked for.
  - The `Task` returned is `cx.foreground_executor().spawn(async move { decode(rx.await) })`;
    a request the app never answers leaves the menu empty, which is what upstream does for a
    provider that returns nothing.

### 9.7 Where each family's code lives

The shared row-source core is `core/src/source.rs`, the ROWS hook in `core/src/rt.rs`, and
`odin/gk/rt_source.odin`, with the numbers of 9.3 and the test mode `rows`. The library's
dispatch already asks both families (`calls.rs` asks `shim_input::call` and
`shim_rows::call`, `hand.rs` asks their `node`), so adding to a family doesn't mean editing a
dispatch file.

| | input family | rows family |
|---|---|---|
| Rust | `core/src/shim_input.rs` | `core/src/shim_rows.rs` |
| Odin | `odin/gk/kit_input.odin` | `odin/gk/kit_rows.odin` |
| generator | the `[instantiate] states` rule in `gen/gk_model.py`, `gen/gk_rust.py`, `gen/gk_odin.py` | its lines in `states` and its `[traits.role]` lines in `api/overrides.toml` |
| numbers (`core/wire/ops.txt`) | call 32-47, shape 14-15, node 10-11 | call 48-63, shape 16-19, node 12-15 |
| tests | `core/tests/input/`, `core/tests/run-input.sh` | `core/tests/rows/`, `core/tests/run-rows.sh` |
| example | the editor pages of `examples/odin/gallery` | `examples/odin/table` |

`core/tests/run.sh` sources every `core/tests/run-*.sh`: a family script builds its app
with `build NAME DIR` and checks it with `GK_PROTO_BIN=NAME run MODE EXIT PATTERN...`.
`api/opcodes.lock` is append-only; when two branches both add numbers, a merge keeps both
sides.

### 9.8 What a delegate family is tested for

- Gallery pages that are byte-identical to the same page drawn by the pure Rust kit (a list,
  a table of 10,000 rows, a tree, an input and an editor with the providers' menus open).
- Protocol tests: the first frame is complete; a far jump shows one placeholder frame
  (`placeholder_frames` in the meter's `stat`); every delegate event is delivered after the
  kit's update; a request is answered and its menu drawn; a released source or provider
  that a view still draws stops the program (exit 2).
- A frame-cost scene: a large table scrolled every frame, measured against the same table on
  a Rust delegate, within 1.03 of Rust plus the Rust-to-Rust noise of the run (DESIGN.md
  section 0, decision 8), with 0 crossings per frame while the rows are bound.

### 9.9 Searchable lists: Select and Combobox over a search delegate

Upstream's `SelectState<D>`, `ComboboxState<D>`, `Select<D>` and
`Combobox<D>` are generic over a `SearchableListDelegate` whose `Item: SearchableListItem` the app
implements. The library implements both once (`core/src/shim_select.rs`):

- `GkSearchDelegate` over a row source, instantiated by the generator (`[instantiate] types` and
  `states`: `Select<crate::shim_select::GkSearchDelegate> as Select`, `SelectState<..>`,
  `Combobox<..>`, `ComboboxState<..>`); its `from_source` takes the items the app pushed for the
  source. Every method of the four types is generated (the states' reads, writes, calls, builders
  and events, the elements' builders).
- `GkItem` (`gk.Search_Item` in Odin: `value`, `title`, `display`, `keywords`, `disabled`), the
  item, a **library value** that travels both ways (`selection()`, events): the generator's
  `libval` descriptor, `[lists] values` in `api/overrides.toml`; Rust reads and writes it with its
  `Unwire` and `Wire` impls, Odin by reflection. `Value` is `SharedString`, so a selected value is
  a string.
- Associated types of a shim (`D::Item`, `<D::Item as SearchableListItem>::Value`) are read from
  `[lists] assoc` (`qualified` in `gen/gk_lists.py`, hooked into `desc`, `rtype` and `borrowed`).
- The items are data: `gk.search_source(cx, state, items, row = nil, requests = {})` or
  `gk.search_source_groups(cx, state, groups, row = nil, headers = false, requests = {})` (call op
  `SEARCH_ITEMS 176`: `u64 source, u8 flags (1 rows, 2 requests, 4 headers), u32 n, (opt<str>
  group title, u32 n, item each) each`), then `gk.select_state(cx, src, nil)->build()`;
  `gk.search_items` / `gk.search_groups` replace them (buffered) and the state's generated
  `set_items(cx, src)` takes the new ones.
- Searching filters in the library exactly as upstream's `SearchableVec` does (the title or the
  keywords contain the query, ignoring case; a group stays while one of its items or its title
  matches). A source made with `requests` asks the app instead: `Search_Request {id, query}`
  (shape `SEARCH_EVENT 44`); the app pushes what it found, sets it, and answers with `gk.answer`;
  the kit's search task ends then.
- Rows: with a row procedure, item `(group, index)` is row `section << 32 | index` of the source
  (its index among all of the group's items, so its view stays while the filter changes) and the
  kit draws that view instead of the title; `headers` makes row `HEADER` of each group its
  header. A row without a view yet is drawn as upstream's row for one frame.
- `display_title` is the item's `display`; `is_item_enabled` is `!disabled`; `is_item_checked`
  compares values; `section` (upstream's default header) is the group's title.
- Not bound: `SearchableListDelegate::on_confirm` and `on_will_change` (called without a context:
  the states' own `Confirm` and `Change` events carry the same), `Combobox::render_trigger` (an
  element closure over a context, skipped as `item-render-closure`, so `ComboboxTriggerContext`'s 5
  reads are reachable by nothing), `SearchableListState` (the core a new searchable component is
  built on in Rust, in the parent's `Context<P>`) and `SearchableVec` (a delegate for Rust apps),
  skipped as `rust-only-extension`.
- An element closure that gets `&App` instead of `&mut App` and draws an element
  (`Select::empty(Fn(&mut Window, &App) -> E)`) takes a `gk.View` (`viewfn`): a fragment cannot be
  decoded without `&mut App`.

### 9.10 A table delegate's texts and looks

`GkTableDelegate` (9.5) answers the rest of `TableDelegate` from data the app pushes (buffered
writes; columns are the app's column ids):

| upstream | binding | call op |
|---|---|---|
| `cell_text`, `cell_label` | `gk.table_texts(cx, t, first_row, [][]string, replace)` | `TABLE_TEXTS 177` |
| `row_label` | `gk.table_row_labels(cx, t, []Table_Row_Label)`, else upstream's join of the cells' labels | `TABLE_ROW_LABELS 178` |
| `fit_column`, `group_headers`, `render_group_th`, `render_header`, `render_tr`, `render_last_empty_col` | `gk.table_look(cx, t, Table_Look{fit, groups, group_views, last_column, header_style, row_style})`: widths, group header levels, a view per group label, a view for the filler, described styles (13.6 values) over upstream's header and body rows | `TABLE_LOOK 179` |

A static table's parts take `impl ChildElement` (`Table::child`, `TableBody::children`): one of
the trait's node types, `gk.Table_Child` in Odin (a union the kit file declares, `[lists] unions`
in `api/overrides.toml`); the library picks the decoder by the child node's type (`rust_arm`).

`TableState::dump_range` reads the texts back (it calls `cell_text`). `ListState::delegate`,
`delegate_mut` and `TableState::delegate`, `delegate_mut` hand out the delegate's row source
(`Row_Source`; the shims' `Wire` impls write its handle). `TableDelegate::context_menu` waits for
the menus of 11.6 as a `PopupMenu` the delegate is handed.

### 9.11 The editor: fonts, texts, objects in events

- `gpui::Font` is `gk.Font {family, weight, style}` (`gk.font("Inter", 700)`; the descriptor
  `font`, Rust `shim_editor::font` and a `Wire` impl): `DisplayMap::new` and `set_font` and
  `InputBaseState::prewrap` take one, so `DisplayMap` is made by the app and its 24 reads and writes
  are the object rule's (13).
- An editor's text read back (`text() -> &Rope`, `selected_text() -> RopeSlice`) is a string
  (`Wire` impls for ropey's types in `shim_editor.rs`).
- An event payload that is a library object and Clone (`InlineTokenClickEvent`,
  `InlineTokenHoverEvent`) is a copy behind a temporary handle, as a read's result is (13.2), so the
  handler can call its reads (`payload_object`).
- `History<SharedString>` and `UndoHistory<SharedString>` are objects (the
  `[values] objects` rule): the app keeps its entries as strings. An iterator read back
  (`History::entries`, `impl Iterator<Item = X>`) is a list.
- The editor's other providers ask the app through the input shims' request channel (9.6):
  `Provider_Definitions`, `Provider_Colors`, `Provider_Code_Actions`, `Provider_Perform`,
  `Provider_Tokens`, `Provider_Inline` (variants 2 to 7 of `Provider_Request`), installed with
  `gk.editor_providers(..., which = {.Definitions, .Document_Colors, .Code_Actions,
  .Semantic_Tokens, .Inline_Completions}, code_action_id, inline_debounce_ms, token_types,
  token_modifiers)`; the code action provider's id, the inline debounce and the semantic tokens'
  legend are data.
- `InputModeKind` and `InputExtras` are the hooks a new input mode implements in Rust; the binding
  instantiates the kit's three modes, whose implementations are upstream's: skipped as
  `rust-only-extension`.

### 9.12 Options the binding could not give, and handlers given to calls

- A `char` travels as its u32 (`rune` in Odin): `MaskPattern::Number { separator: Option<char> }`
  and every other char argument or field.
- `impl From<&str> for T` (not `&'static str`) is a member of T's Into set: `set_mask_pattern`
  takes upstream's string pattern (`"999-999"`, upstream's `Pattern` variant, whose tokens are
  private) as well as `None` and `Number`.
- A handler given to a call op (a state's or an object's method, or a builder of its constructor:
  `InputState::validate`, `set_validator`) is a **kept handler**, as a subscription's is
  (`put_kept_handler` in `kit_editor.odin`, `shim_editor::kept`): the library fires it whenever upstream
  calls the closure, between frames (on input, on a click), with the window when the closure has
  one. It lives until the app ends; giving the state another one keeps both.

Wire numbers: call 176-207, shape 44-51, node 40-47, hkind 12 (`core/wire/ops.txt`).
Tests: `core/tests/lists2` (`run-lists2.sh`; `core/tests/lists2/alone.sh lists2` runs them alone),
gallery pages `lists2-*` (`core/tests/lists2/gallery-pages.sh lists2-` compares them alone).
### 9.13 Objects made by Default, and handlers given to calls dropped with their state

- A library object (13.1) with no constructor of its own that implements `Default` gets a
  synthetic constructor `<Type as Default>::default` (`gk_lists.default_ctor`, hooked at the end of
  `classify_object`): a kept handle, `gk.status_colors_default(cx)`, not counted as an upstream
  function. Its reads, writes and calls become reachable (13.4): `StatusColors` (18 reads, each a
  theme colour unless the theme set its own), `SyntaxColors` (`style`, `style_for_index`),
  `TableVisibleRange`, `ImageNode` and 15 more across the kit. The generated `states` test makes,
  reads and releases each one.
- A handler given to a call op (9.12: `validate`, `set_validator`, a notification's `on_click`)
  belongs to the handle the call is made on, or makes: `put_kept_handler` (kit_editor.odin) notes
  its slot; `state_make` gives the noted slots to the new handle, `state_set` and `state_call` to
  the handle the request names after its op; `gk.release` of that handle drops them
  (`kept_release`). `gk.test_kept_count()` counts
  the kept handlers alive (test mode `kept`).

### 9.14 Data closures answered between frames: asks

A closure upstream calls while it lays out or draws and that returns a value which is not a
number or a bool (9.12 covers numbers), with arguments the app can read and a result the app can
write, is an **ask** (descriptor `["ask", key, result, wrap, ptypes, params, rust type]`,
`gk_lists.ask_closure`, hooked into `closure_desc` before "a closure that returns a value"):

- Odin: `a.hl = gk.ask(cx, state, procedure)` in init or a handler, where `procedure :: proc(s: ^S,
  cx: ^gk.Cx, key: K) -> V`; the builder takes `gk.Ask(K, V)`, so a procedure of other types does not
  compile. K is the closure's argument (several: a `Tuple_`), read by reflection; V is written by
  reflection (`put_value`, which writes a `Color` as `put_color` does). `gk.release(cx, a.hl.src)`.
- Wire: the ask is a row source used for its request channel (9.2), made an ask by call
  `ASK_NEW 180` (`u64 source`, immediate: the library's cache and, once, its main-thread task).
  An element op or a call op carries its `u64` handle. The request is shape `ASK_EVENT 45`:
  `u64 id, u64 ask, key bytes` (the closure's arguments in their wire form); the answer is
  `C_SOURCE_ANSWER` with V's bytes. `ASK_TEST 181` returns the tests' counts (gets, keys asked,
  answers cached).
- Rust (`core/src/shim_text.rs`): the closure looks the key (`shim_text::key`, the arguments'
  `Wire` bytes) up in the ask's cache. A key not there returns V's default (an image source: an
  empty path) and is sent once to the task, which, between frames, asks the app
  (`source::request`, a queued event); when the answer arrives it is decoded with the app context
  and the first window (`genapi::with_root` over an empty tape, so colours resolve against the
  theme), cached, and every window is redrawn (`refresh_windows`: the tapes are decoded again and
  upstream gets a new closure, now answered from the cache). One or two frames show the default.
- Arguments: values with no handles (a key is their bytes): strings, numbers, records, enums,
  `&SharedUri` (a string; `Wire` in shim_text.rs), `&CodeBlock` (`gk.Code_Block {lang, code,
  meta}`, a library value). Results: values with no nodes or handles, and `ImageSource` as a path
  or a URL (`"https://..."`; `shim_text::image_source`).
- Bound: `TextView::code_block_highlighter` (base), `TextViewDefaults::with_code_block_highlighter`,
  `RenderContext::with_code_block_highlighter` and `with_image_source`, `TextView::image_source`
  (base and component), `DiffState::with_syntax_highlighter` and `set_syntax_highlighter`,
  `Calendar::label`, and the ambient `set_icon_resolver`.
- Not bound: closures whose result is an element or view (`link_tooltip -> AnyView`,
  `block_surface -> Div`), a style (`with_heading -> StyleRefinement`), a menu (`Tree::context_menu`),
  or that take a library object as their argument (a key must be bytes).

### 9.15 Values a static makes several of

A static of a value type (a node that is not an element) that returns `Vec<Self>` or
`Result<Vec<Self>, E>` (`DiffFile::parse(patch)`, `parse_with(patch, options)`) is bound as two
synthetic functions (`gk_lists.list_ctor`, hooked before "a static function that is not a
constructor"): `gk.diff_file_parse_count(cx, patch) -> gk.Fallible(int)` (an ambient call, 11.5:
how many values it made, or the error's text) and the constructor `gk.diff_file_parse_at(patch,
index)`, whose value is the static's value at that index (the library runs the static where the
value is decoded; an index past the count, or a failed static, stops the app, exit 2). The static
itself is also bound as an ambient call whose values come back as read-only `X_Read` objects (13.15: `gk.parse(cx, patch)` gives `[]gk.Diff_File_Read`); only the `_at` values can
be given to the kit (`gk.diff_state(cx, files)`). When no other rule binds it, the upstream
function counts as bridged by this one. A diff from a patch:

```odin
n := gk.diff_file_parse_count(cx, patch)
files := make([]gk.Diff_File, n.value.? or_else 0, context.temp_allocator)
for &f, i in files { f = gk.diff_file_parse_at(patch, i) }
a.diff = gk.diff_state(cx, files)->build()
```

Tests: modes `drive`, `kept`, `status-colors`, `ask`, `diff` of `core/tests/lists2`; gallery
pages `text-highlight` and `diff-view`.
### 9.16 Element closures over a context

Upstream draws parts of some elements through a closure it hands values it made: a token's
context (`Input::token(Fn(&InlineTokenContext, &mut Window, &mut App) -> R)`), a diff file
(`Diff::render_header_suffix(Fn(&DiffFile, ..) -> E)`), an editor's scrollbar geometry
(`set_scrollbar_renderer(Option<Rc<dyn Fn(EditorScrollbarContext, ..) -> AnyElement>>)`, a type
alias the rule reads through), a scroller's item index, a questionnaire choice's state. They were
skipped as `item-render-closure`, and the contexts' reads were not attempted (no bound function
handed one out). General rule (`gk_lists.ctx_rows`, hooked into `closure_desc` before that skip):
an element closure that gets the window, the app and one to three values the app can read
(plain values, or library objects that are Clone) is `["ctxrows", wrap, ptypes, params, descs]`.

- Odin: the app passes a `gk.Row_Source` (made in init or a handler, count 0); its row procedure
  reads the values with `gk.row_context(cx, src, at, T)` (T the closure's value, or a struct of
  its values in order). A library object comes as a copy behind a temporary handle, valid while
  the row is described, so the row can call its reads (`t->is_selected(cx)`) or give it to an
  element (`gk.input_token(t)`: pinned to the row's build, 13.12).
- Rows: each call of the closure in a frame is a row, in the order of the calls (the count starts
  again at the next frame: `window.on_next_frame`), or the index itself when the closure's only
  value is an integer. The library keeps, per row, the identity of the values it was handed (the
  plain values' bytes; an object's reads that take nothing and return plain values, generated
  per type) and a writer of the values. A bound row whose values changed is marked dirty: the next
  frame describes it again, and this frame shows the old row. A row without a view is drawn as
  nothing for one frame, as any row (9.1). Nothing calls the app while upstream draws.
- Objects handed to such a closure are reachable (`gk_lists.handed_out`, hooked into
  `reach_objects`), so their reads and the functions that take them are bound by the object rule
  (13): `InlineTokenContext` (7), `EditorScrollbarContext` (2), `InputToken::new(&InlineTokenContext)`.
- Wire: call `CTX_GET 182` (`u64 source, u32 section, u32 row` -> the values; immediate, allowed in
  a view procedure). A row no closure drew stops: "row N of this row source was never drawn by an
  element closure over a context (gk.row_context)", exit 2.

### 9.17 Objects lent by kit state

A state method `fn x_mut(&mut self) -> &mut T` or `-> Option<&mut T>`, T a library object that is
Clone (`InputBaseState::diagnostics_mut`), was "an update that returns a value" (a borrow). It is
now a **lend** (`gk_lists.lend`, hooked into `state_method`; descriptor `["lent", T, state, opt]`):
a call (run at once, after the turn's buffered writes have been sent) that returns a temporary
handle to a copy of the object, of T's own Odin type, so every generated method of T works on it.
The app writes to it with the turn's buffered writes; the library gives the copy back to the state
through the same method when the turn's writes have landed (`shim_editor::lend`: deferred twice,
after `rt::process_reply`'s deferral of the writes), notifies the state (its views redraw), and
releases the handle. Using it after its turn stops ("was released", exit 2). The copy is taken when
the call runs, after the writes buffered before it, so they are in it. Call `LEND_TEST 183` (tests:
objects lent, given back). Not lent: `lsp_mut` (`Lsp` is a builder value, not an object the app can
hold) and `display_map` (`&DisplayMap`, which is neither Clone nor lent through `&mut`).

### 9.18 Delegates answered between frames

A trait upstream calls while it draws or edits and the app implements is implemented once by the
library over procedures of the app, each an ask (9.14): asked between frames for a question the
cache has not got, upstream's default (or nothing) meanwhile, the answer cached, every window
redrawn. `[lists] delegates` in `api/overrides.toml` names the library function that makes the
upstream value from the handle the app passes, by type alias or by `"dyn Trait"` for `Rc`, `Arc` or
`Box` of it (`gk_lists.alias`): the argument is the descriptor `rows` (a `gk.Row_Source`), which a
call op can decode (`gk_lists.CALL_SAFE`). The trait methods count as bridged through `[shims]`.

| upstream | Odin | the app answers |
|---|---|---|
| `InputHighlighterFactory` (`set_highlighter_factory`, `ensure_highlighter_factory`) | `hl := gk.highlighter(cx, a, styles, folds = nil, languages = nil)`, then `ed->set_highlighter_factory(cx, hl.src)` | `styles(q: gk.Highlight_Query{language, text, range}) -> []gk.Highlight_Run{range, name}`: named runs, resolved by the editor's resolver (the kit's editor: the theme's syntax colours), the gaps unstyled; `folds(gk.Fold_Query) -> []gk.Fold_Range` |
| `Rc<dyn LanguageProvider>` (`set_language_provider`) | `p := gk.language_provider(cx, a, aliases, syntax = nil)`, `gk.base_set_language_provider(cx, p)` | aliases as data (`language_name`); `syntax(gk.Syntax_Query{language, text, offset}) -> gk.Syntax_Context` (`SyntaxContextProvider::context_at`, Code until answered); `config` is upstream's default (an app's configs go through `set_language_config`, which takes precedence) |

`InputHighlighter::update` keeps the editor's text and drops the answers about the old text;
`update_batch` and `fold_ranges_for_edit` are upstream's defaults over `update` and `fold_ranges`.
Calls `HIGHLIGHTER_NEW 184` (`u64 styles ask, u64 folds ask (0 none), u32 n, str language each`) and
`LANGUAGES_NEW 185` (`u64 handle, u8 has syntax, u32 n, (str alias, str name) each`), immediate.

### 9.19 Small rules of

- A static that makes several values and can fail (`DiffFile::parse`, 9.15) also gets
  `<type>_<name>_error`, an ambient call that returns the error as the kit's own value when the
  error type is a library object that is Clone (`gk.diff_file_parse_error(cx, patch) ->
  Maybe(gk.Diff_Parse_Error)`, with `line` and `message`).
- gpui's `UniformListScrollHandle` read back (`TreeState::scroll_handle`) is the gpui ScrollHandle it
  scrolls by, the scroll-handle object (13.14): `tree->scroll_handle(cx)->set_offset(cx, ..)`.
- Third-party types these rules carry (`gk_lists.THIRD_PARTY`, hooked into `rtype`): ropey's
  `Rope` inside a tuple (an editor's text, 9.11), and LSP's `CompletionItem` going in as
  `gk.Completion_Item` (label, kind, detail, insert text; `Unwire` in shim_input.rs), so the
  completion highlighters are ambient calls: `gk.base_highlight_prefix(cx, "ma", item) -> []gk.Range`
  (and `highlight_contains`, `highlight_characters`).

Tests: modes `ctx-token`, `lend`, `highlight`, `languages`, `bad-lend`, `bad-ctx` of
`core/tests/lists2`; gallery pages `text-token`, `diff-header`, `editor-diagnostics`.
### 9.20 Typed fragments

A closure upstream calls while it draws that gets the window and the app, no values, and returns
one element type rather than `impl IntoElement` (`MarkdownExtensions::block_surface(name,
Fn(&mut Window, &mut App) -> Div + Send + Sync)`) is a fragment of the app's view, as any render
closure without an item is (DESIGN.md 12), whose root is decoded as that type: descriptor
`["tfrag", node, params, wrap, ptypes]` (`gen/gk_editor.py`). The app passes an element of the type
(`gk.div(.P_2)->bg(..)->child(..)`, Odin's type is the node's, so another element does not compile);
the library replays the fragment whenever upstream calls the closure and decodes its root with the
type's generated `make_` function. The fragment holds the view's tape in an `Rc`, so the closure is
wrapped in `editor_text::MainThread`, which refuses any thread but the main one (upstream calls it
while it draws). A root of another type (only a hand-written tape can make one) stops (exit 2).

### 9.21 Predicates as key sets, tables by index

Two data closures with no window and no app, which upstream calls synchronously, are data the app
gives once:

- `FnMut(&T) -> bool` over a string or an integer (`History::retain`): `["keyset", key]`, Odin
  `[]T`: the keys for which the predicate holds (`h->retain(cx, []string{"b", "d"})`).
- `Fn(u8) -> V` (`TextViewStyle::with_heading`, a heading level): `["table", V, .., rust V]`, Odin
  `[]V`: the value at index k; an index past the end is V's default (`StyleRefinement::default()`).

### 9.22 Element and view closures over values without a window

Two kinds of closures over values the kit hands them, beside the element closures over a context of
9.16, served by the same row sources (`gk.row_context` reads the values):

- **Without a window** (`ctxlazy`): upstream gives the closure no window and no app
  (`FoldPlaceholderRenderer = Rc<dyn Fn(usize) -> AnyElement>`, `FoldIconRenderer = Rc<dyn Fn(usize,
  bool) -> AnyElement>`; through their type aliases, `gk_editor.alias`). The closure returns a library
  element (`editor_text::lazy_row`) that becomes the row of the source when it is laid out
  (`request_layout`, where the window and the app are), by `source::ctx_row`; rows are keyed as in
  9.16 (the value itself when it is the only one and an integer: a fold's line).
- **Views** (`viewrows`): a closure over plain values that returns an `AnyView` with the window and
  the app (`TextView::link_tooltip(Fn(&SharedString, &mut Window, &mut App) -> AnyView)`): the row
  view of one row per distinct value (`editor_text::ctx_view` keeps, per source, the row of each
  value's bytes), so the same link shows the same view; a row not described yet is an empty view for
  one frame.

Odin: both take a `gk.Row_Source`; the row procedure reads `int` (a fold's line), a `Temp_String` (a
URL) or a struct of the values in order.

### 9.23 Markdown plugins

`MarkdownExtensions::plugin<P: MarkdownPlugin>` and `TextView::plugin<P: TextViewPlugin>` (base and
component; TextViewPlugin is implemented for every MarkdownPlugin) take `gk.Markdown_Plugin_Any`, a
union of the kit's plugins (node types that implement MarkdownPlugin: `gk.frontmatter_plugin()`) and
the library's **data plugin** `gk.Markdown_Plugin {name, match, lang, rows, baseline}`
(`["mdplug", "crate::editor_text::DataPlugin", ["node", kit plugin]...]`; the generator stops when the
kit's set of plugins changes, so the library's `GkPlugin` enum is kept in step). Upstream parses on a
background task and must not call the app (decision 3), so the data plugin claims nodes by a rule
given as data:

| `gk.Markdown_Match` | the Markdown node | `lang` matches |
|---|---|---|
| `Code`, `Math`, `Yaml`, `Toml`, `Html`, `Mdx_Element` (blocks) | fenced code, `$$` math, front matter, raw HTML, a JSX element | Code: the fence's language; Mdx: the element's name; "" any |
| `Inline_Code`, `Inline_Math`, `Inline_Html`, `Mdx_Inline_Element` (inline) | the same in a line | as above |

A claimed node is the kit's `MarkdownNode` with the plugin's name, the node's text (its value: the
code, the math, the YAML; for a JSX element its source) and its Markdown source. Drawing it is a row of
`rows`, described between frames like any row (one frame of nothing at first; again when the node
changes): a block's row reads the node (`gk.row_context(cx, rows, at, gk.Markdown_Node_Read)`: name,
text, Markdown, source range); an inline node's row reads `gk.Markdown_Inline {node, typography}`, `typography`
being the kit's `InlineRenderContext` (the inherited text style, font size, line height, rem
size; reachable through this rule, `gk_editor.handed_out`), and is placed in the line as an
`InlineElement` with `baseline` pixels from its top (0: upstream's). The kit's plugins run as
upstream wrote them. Not bound: a parser written in Rust (`MarkdownParseContext`, `MarkdownNode::new`
with typed data, `InlineElement`), skipped as `rust-only-extension`.

### 9.24 Small values

- An `ElementId` read back is its text (`DiffAnnotation::id`; `Wire for ElementId` in
  `core/src/editor_text.rs`).
- A `gpui::EntityId` argument (`InputBase::semantic_owner`) is the entity behind an input, textarea or
  editor handle: `gk.entity_id(a.editor)` (Odin `Entity_Id`). Any other handle stops (exit 2: "is not an
  input, a textarea or an editor"); a released one stops with the release's own message.

### 9.25 Objects a read-back value hands out

An object returned by a `&self` read of a value that is read back (13.15) is reachable: the
read-back value's reads are classified after the reachability pass
(`readback_reads`), which therefore found nothing handing the object out. `gk_editor.before_reach`
keeps every object's lists and statuses, and `gk_editor.revive` gives back those of an object such a
read returns (by value, a list or an option of it, Clone) before the reads are classified:
`DiffFile::conflicts() -> &[DiffConflict]` makes `DiffConflict`'s seven reads bound (`gk.parse_conflicts(cx,
path, text)`, then `file->conflicts(cx)`).

### 9.26 Records with colours in call ops; one-method data delegates

- **Records with colours.** A call op has the app context, but the records' `_c` codecs do not (DESIGN
  13, `call_bad`), so a record holding a colour at depth was refused in call ops. Now such an argument
  (`Vec<TextDecoration>`: a range and a `HighlightStyle` with colours; `InputEditorStyle`) is decoded in
  place, field by field, in the op itself, every record inside it too, its colours resolved against the
  theme (`gk_editor.call_bad` before the general check, `rust_inline_rec`). Not inside a builder chain
  (no app context there). Binds `create_decorations_collection` (its `TextDecorationCollection` is an
  object the app keeps: `set`, `append`, `clear`, `get_ranges`) and `set_editor_style` of the three input
  states.
- **One-method data delegates.** `Arc<dyn Trait>` (also through a type alias,
  `SharedHighlightStyleResolver`) of a trait whose one method maps a plain value to a value is served by
  an ask (9.14): `["askdyn", key, result, shim, trait]`, Odin `gk.Ask(K, V)`; the shim per trait is one
  bridge (`gk_editor.ASK_TRAITS`): `HighlightStyleResolver::style(name) -> Option<HighlightStyle>` is
  `editor_text::GkStyles`, the app's procedure asked once per name, unstyled until answered. Note that
  the kit's component inputs set their own editor style on every render; `set_editor_style` is for the
  base elements (`gk.editor`, `gk.base_input`, `gk.base_textarea`).

- **Actions in call ops.** A `Box<dyn Action>` argument (`EditorState::route_overlay_action`) is the
  action's id (`gk.action(cx, name)`, `gk.action_of(cx, value)`), which a call op reads as a tape does
  (`genapi::action`); not inside a builder chain.

### 9.27 Reads through a borrow

A state read `fn x(&self) -> &T` where T is a library object that cannot be copied out of the borrow
(not Clone: `InputState::display_map() -> &DisplayMap`) was refused. Its object's reads become reads
of the state, each a synthetic state read that calls the object's read on the borrow in place
(`gk_editor.project`): `ed->display_map_folded_ranges(cx)`, `display_map_buffer_line_to_display_row(cx,
line)`, `display_map_text(cx)`, 17 per state; a read that returns a borrow returns an owned copy
(`ToOwned`). The upstream function counts as bridged by them (also on the generic state's row). The
same rule reaches `TableState::visible_range() -> &TableVisibleRange` (its `rows`, `cols`).

### 9.28 What the editor family still waits for

22 functions of the family are not attempted (`gen/gk_editor.py` `LEFT` gives each its reason), and
five value types (21 functions) are taken by nothing (`gates/accept.py`):

- **The editor host** (14 functions; `CodeBlock`, `FlowPosition`, `InlineNode`, `RenderContext`
  unaccepted). `gpui_kit::base::text::editor_host` is for an editor written outside the kit: it renders a
  `BlockNode` tree (`BlockNode::render`, skipped as a data value's method) and, in the host's own
  element, wraps the blocks' prepaint in `collect_leaf_geometry`, which returns a `LeafGeometry` per keyed
  text leaf (10 reads: hit testing, carets, IME bounds). Plan: block nodes as library objects
  (`parse_markdown` by the count-and-index rule of 9.15, or built by the app through `Paragraph::push`,
  today a builder-mutator); a library element `gk.block_host(id, blocks, render_context, decorations,
  rows)` that renders them and collects the geometry in its prepaint, then hands the app `(key,
  LeafGeometry)` pairs after the frame (a queued event; the objects temporary handles, as 13.2). App
  code still never runs mid-frame; the geometry is one frame old, as any measurement the app reads.
- **Builder chains of call ops** (2: `TextViewDefaults::with_style`, `TextViewState::motion`): a value
  node inside a state's or an object's builder chain needs the app context, which the chain decoder
  (`sb_*`) has not got; passing the call's context into it would bind every such builder.
- **Build closures kept for later** (2: `NumberInput::decrement_button`, `increment_button`): a
  `'static FnOnce(Button) -> Button` NumberInput calls while it renders, with no window: the fragment
  would have to be applied inside the element's own layout (as 9.22's rows are).
- **An edit of a builder value lent by `&mut`** (2: `EditorState::lsp_mut`, `Lsp::with_completion_highlighter`):
  the app's builder ops applied to the value in place (the ops of a value tape without its constructor).
- **A record holding a value node in a call op** (1: `present_diagnostic(DiagnosticEntry)`), **an answer
  needed at once** (1: `set_overlay_action_handler`, whether the app handled an overlay's action; data
  as for 9.21's key sets would do), **an event payload that is neither state nor object** (1:
  `InputEditsEvent::edits`).

### 9.29 Variants made by a constructor

A variant of an enum with data that the binding cannot build field by field, because its other
fields are private types (`MaskPattern::Pattern { pattern, tokens: Vec<MaskToken> }`), is made by the
enum's constructor whose parameters are named as some of its fields, when `[editor] variants` in
`api/overrides.toml` names the pair (`"MaskPattern::Pattern" = "new"`; `gk_editor.variant_ctors`): the
variant's fields are the constructor's parameters (Odin `gk.Mask_Pattern_Pattern{pattern = "999-999"}`),
and the decoder calls the constructor (`<MaskPattern>::new(&*pattern)`). Every variant of `MaskPattern`
is now reachable (`set_mask_pattern`, `mask_pattern` of the three input states: None, Pattern,
Number; before, Pattern only through the `&str` member of the Into set), and the function leaves the
partial-options table of STATUS.md.

Wire numbers: call `EDITOR_TEST 186` (tests: nodes the data plugins claimed, view rows made).
Tests: `core/tests/editor2` (`run-editor2.sh`; `core/tests/lists2/alone.sh editor2` runs them alone),
misuse `examples/odin/negative_editor` (4 cases), gallery pages `editor5-*` (`core/tests/lists2/gallery-pages.sh
editor5-`).
### 9.30 The editor host

`gpui_kit::base::text::editor_host` serves an editor written outside the kit: it renders a `BlockNode`
tree and, around the blocks' prepaint, collects each keyed text leaf's geometry. The library's element
does both, as one protocol:

```odin
gk.block_host(id, blocks: []gk.Block_Node, flows: []Maybe(gk.Flow_Position) = nil,
              render_context: Maybe(gk.Render_Context) = nil, decorations: []gk.Leaf_Decoration = nil,
              on_geometry: gk.Handler = {})
```

- **Rendering.** Each block is drawn as TextView draws it (`BlockNode::render`) inside a `div` with the
  host's id: with its flow when the app gives one (`gk.flow_position_at(blocks, ix, last)->first(true)`, a
  chunk of a long document), else its place among the blocks (`FlowPosition::at`); with the render context
  when the app gives one, else the one TextView would start from (`RenderContext::from_app`).
- **Geometry.** The host runs `collect_leaf_geometry(decorations, || prepaint)`: the decorations (a leaf
  key and byte ranges with colours: the app's selection, find matches) are painted behind the keyed
  leaves, and the leaves' geometry is handed to `on_geometry` **after the frame** (app code never runs
  mid-frame), as `gk.Leaf_Geometry_Event{leaves: []gk.Leaf_Entry{key, geometry}}`; `geometry` is a
  temporary library object (13.2; `gk.retain` keeps it) whose ten reads are upstream's: `bounds`,
  `len_`, `is_empty`, `line_height`, `caret_height`, `position_for_index`, `index_for_position`,
  `caret_for_position`, `position_for_caret`, `atom_at`. The event comes again only when the leaves' keys,
  bounds, lengths or heights change (a fingerprint per view and handler), so describing the view again
  does not keep the app drawing; the geometry is one frame old, as any measurement the app reads.
- **Values the app builds.** Block nodes are the generated `gk.Block_Node` union. Variants that hold
  blocks (`Root`, `Blockquote`, `Surface`, `List`, `ListItem`) are built like any variant: a recursive data
  enum (`gk_editor.recursive`) is its own decoder's argument, Odin's union holds a slice of itself. A
  paragraph's inline nodes are added in place (`gk.paragraph_default()->push(gk.inline_node("x"))`:
  `fn push(&mut self, x)` of a value the host takes is an op applied in place, `gk_editor.mutator`;
  also `TextMark::merge`).
- **Held values.** A data enum the app builds that a bound function also returns, whose variants cannot
  all be read back (`parse_markdown -> Result<Vec<BlockNode>, SharedString>`; a parsed `Custom` node holds
  the parser's `MarkdownNode`): read back, each value is a library object (`["held", rt]`, Odin
  `gk.Block_Node_Held`, a temporary handle, `gk.retain` keeps it), and the enum's union gets one member
  more, the held value (variant `0xfffe`, copied out of its handle where it is decoded, pinned to the view
  that places it as any object, 13.12). `gk.base_parse_markdown(cx, source, gk.markdown_extensions_arc(cx,
  ext)) -> gk.Fallible([]gk.Block_Node_Held)`; the blocks go into `block_host` beside the app's own.
- **Late objects.** `RenderContext::with_markdown_extensions(Arc<MarkdownExtensions>)` and `parse_markdown`
  take an `Arc` object that the values rules make constructible after the reachability pass
  (`gk_values.pointer_ctors`); `gk_editor.after_values` gives such items back their status. An iterator of
  iterators read back is collected at each level (`editor_host::Table::rows -> [][]gk.Paragraph_Read`).

Wire: node `EDITOR_HOST 40` (str id; u32 n, BlockNode each; u32 n, (u8 has, u32 FlowPosition value node)
each; u8 has, u32 RenderContext value node; u32 n, (u64 key, ranges) each; u8 has, u32 handler), shape
`LEAF_GEOMETRY 46` (u32 n, (u64 key, u64 LeafGeometry) each), call `EDITOR_HOST_TEST 187` (tests: hosts
decoded, leaves collected, events delivered, decorators run, lent values edited). The element's procedure
and its decoders are generated from one list of descriptors (`gk_editor.host_args`: `gen_editor_host.odin`,
`core/src/gen/n_editor_host.rs`); the element is `core/src/editor_text.rs` `BlockHost`. Binds
`collect_leaf_geometry` (bridged by the element), LeafGeometry's 10 reads, `parse_markdown`,
`RenderContext::with_markdown_extensions` and `Table::rows`, and accepts CodeBlock, FlowPosition,
InlineNode and RenderContext.

### 9.31 Values in a builder chain of a call op

A builder of a state's or an object's constructor chain whose argument the chain decoder (`sb_*`, no
context) cannot decode but a call op can (a value node the app builds, 13.6: `TextViewState::motion(
TextViewMotion)`, `TextViewDefaults::with_style(TextViewStyle)`) is bound (`gk_editor.chain_cx`): the
constructor's call op decodes the whole chain first into closures (`sbp_*`, with the call's context and
window), then applies them to the new value. `gk.text_view_state_markdown(cx, md)->motion(
gk.text_view_motion_default()->with_stream_fade(200 * time.Millisecond))->build()`. The rule is general and binds such builders in every family (9.38), for example
`ColorPickerState::default_value`.

### 9.32 Decorators kept for later

`impl FnOnce(X) -> X + 'static` with no window, X an element that is Styled and a ParentElement
(`NumberInput::increment_button(Button)`, `decrement_button`, `MessageScroller::with_jump_button_renderer`):
upstream keeps the closure and calls it while it renders, where no app code may run and no decode context
is left. The app gives as data what it would add (`gk.Decorate{style, content}`): `style`, a
`gk.Style_Refinement` decoded where the closure is decoded (its colours resolved then), refines the
element's own style, and `content`, a row source (zero: none), adds its row 0 as the element's child, laid
out where the window is (`editor_text::lazy_row`, as the fold placeholder's rows, 9.22). Descriptor
`["decorate", node, wrap, ptypes]` (`gk_editor.decorate`, hooked where the model refused a build closure
without a window).

### 9.33 Edits of a value lent by &mut

`fn x(&mut self) -> &mut V` of kit state, V a value the app builds that has a Default
(`EditorState::lsp_mut -> &mut Lsp`): a state write that takes a value of V (a tape of its own, 13.6) and
applies its builder ops, not its constructor, to the state's own V in place (`editor_text::LentEdit`:
the value is taken, the ops applied with the update's window and app, the result put back):
`ed->lsp_mut(cx, gk.lsp_default()->with_completion_highlighter(ask))`. `Rc<CompletionHighlighter>`, an alias
of `dyn Fn(&str, &CompletionItem) -> Vec<Range<usize>>`, is the closure behind the pointer (`gk_editor.alias`),
answered by an ask (9.14) keyed by the query and the item (`Wire for CompletionItem`, shim_input.rs).

### 9.34 A record holding a value node in a call op

A state call whose argument is a struct with public fields, one of them a value node the app builds
(`present_diagnostic(DiagnosticEntry { range, diagnostic: Diagnostic })`), was refused as an opaque object
(the struct is a record only read back elsewhere). Its fields become the call's arguments and the callee
builds the struct (`gk_editor.flat_records`): `ed->present_diagnostic(cx, {0, 3}, gk.diagnostic(range,
"unused")->with_severity(.Warning))`.

### 9.35 Event payloads made of their reads

`EventEmitter<E>` of kit state where E has no public field and is no object the binding holds, but has
`&self` reads that take nothing (`InputEditsEvent::edits() -> &[(InputEdit, Rope)]`): the payload is a
record of the reads' results, copied when the event is raised (`gk.Input_Edits_Event{edits = []{edit,
text after it}}`, subscribed with `inp->on_input_edits_event(cx, gk.on(..))`); the reads count as bridged
by it (`gk_editor.event_reads`, its Wire impl calls the reads). This applies to every event (9.38).

### 9.36 Answers given in advance

`set_x(handler: impl Fn(E, Box<dyn Action>, &mut Window, &mut Context<..>) -> bool)` with E an enum the
binding reads (`EditorState::set_overlay_action_handler(InputOverlayKind, action)`): upstream needs the
answer at once, while it dispatches a key, so the app gives it in advance: the pairs (E, action name) it
takes (`ed->set_overlay_action_handler(cx, {{.Completion, "input::Tab"}})`). The handler answers true for
those, so the overlay leaves them alone (and closes for Enter and Escape); the app handles them with its own
key bindings (`gk_editor.answer_sets`).

### 9.37 Actions read back

`Box<dyn Action>` read back (`NativeMenuItem::Action { label, disabled, action }`, the context menu upstream
would show, handed to `on_context_menu`) is the action's id as the app knows actions (`gk.Action`, 11.1; a
kit action gets an id the first time it is seen, `app::id_of`), compared with `gk.action(cx, "input::Copy")`;
`on_context_menu` of the three input states leaves the partial-options table. Upstream hands the handler an
empty `NativeMenu::new()` (the app builds its items with `NativeMenu::menu(label, gk.Action)`), so the
read-back is the type's completeness, not a menu upstream filled; under the offscreen driver the handler did
not fire after a right click (round6.odin mode `context-menu`, not in run-editor2.sh). The rule is general
(9.38).

Tests: modes `host`, `chain`, `decorate`, `lsp`, `diagnostic`, `overlay-keys`, `edits` of
`core/tests/editor2` (`round6.odin`); misuse `ed6-*` of `examples/odin/negative_editor`
(held values and leaves kept past their frame, a decorator's freed row source, a lent edit in a view);
gallery page `editor6-host` (Rust twin in `ref/src/bin/gallery/editor_pages.rs`).
### 9.38 The rules of 9.31 to 9.37 serve every family

The rules of 9.31 to 9.37 are general: they bind any function whose shape they match, in any family,
with no per-family test and no list of admitted functions:

- **Builders of a constructor's chain with the call's context (9.31, 11.27)** bind every builder the chain
  decoder cannot decode but a call op can; an `impl Into<Hsla>` argument is named `Hsla` there (what
  `gk_comp6.ADMIT` did for `ColorPickerState::default_value`).
- **Records holding a value node in a call op (9.34)** and **answers given in advance (9.36)** apply to any
  kit state.
- **Event payloads made of their reads (9.35)** apply to any event. A payload whose event no state the
  binding keeps raises loses its record and its shape (`gk_editor.event_read_statuses`): the kit's
  `DockArea` raises `DockToggleEvent`, which the dock runtime already delivers (`gk.on_dock_toggle`,
  `Dock_Toggle`), so no `gk.Dock_Toggle_Event` type and no handler shape is generated that nothing hands
  the app (without this the wider rule made both).
- **Actions read back (9.37)** apply to any type holding a `Box<dyn Action>` (`gk_editor.ACTION_OUT` is
  gone). A value that holds an action and also goes into the library is decoded in a call op by the
  action's id, nested in records and variants too (`gk_editor.call_bad`; before, only at the top level):
  gpui's `OwnedMenuItem` gains its `Action` variant (`gk.Owned_Menu_Item_Action{name, action, os_action,
  checked, disabled}`) in both directions. `GlobalState::set_app_menus` and `app_menus` stay replaced by
  the platform runtime's calls (11.30), which carry every variant, so no generated function takes or
  returns `OwnedMenu` and the new variant's codecs are compiled but run by no test.

### 9.39 Held values reach every variant; a parsed Custom block

`BlockNode::Custom { node: MarkdownNode, state }` cannot be built by the app (`MarkdownNode`'s builders
are skipped: it wraps a parser plugin's typed data), but it is a held value (9.30): `gk.base_parse_markdown`
with an extension whose block parser claims a node (the kit's `FrontmatterPlugin` claims the front matter)
returns it as a `gk.Block_Node_Held`, and every function that takes a `BlockNode` takes a held one (the
union's member `Block_Node_Held`, variant `0xfffe`). So `gk_values.partials` counts a data enum read back as
held values as fully given in the in direction when a usable function returns it held (`m._held_from`):
`FlowPosition::at` and `collect_leaf_geometry` leave the partial-options table (2 -> 0).

Test: mode `custom` of `core/tests/editor2` (round7.odin) parses front matter, places the held Custom block
between two keyed paragraphs of the app in `gk.block_host`, with flows from `gk.flow_position_at` over that
list, and reads the leaves' geometry: the paragraph after it lies lower than in a host without it. Misuse:
`ed7-ok` (kept by `gk.retain`) and `ed7-custom-unretained` (not kept, placed a frame later: stops).

### 9.40 Tests that run the bindings of 9.31 to 9.37

These bindings are exercised by running them, not only by checking that the calls are taken:

- **Decorators (9.32)**: gallery pages `editor7-number` (a number input's increment button with a style and
  a row, its decrement button with a row; the state made in init, the Rust twin's in its view's first
  render, where the window is) and `editor7-scroller` (a message scroller scrolled up: its jump button with
  a style and a row, after its 200 ms entrance).
- **The Lsp's completion highlighter (9.33)**: protocol mode `lsp-ask` (the menu the app presents asks the
  app's highlighter for each label, through the ask) and gallery page `editor7-lsp` (the labels highlighted
  where the app says, the menu presented by a click). The ask is answered between frames (9.14): the frame
  that first draws the menu shows no highlight, the next one does; a gallery step is followed by a second
  frame before its shot.
- **Answers given in advance (9.36)**, fired by a key: protocol modes `overlay-fire` and `overlay-control`.
  The completion menu is open (the app presents it with `present_completion_items` from its
  InputEditsEvent handler, so the editor has the focus) and Enter is pressed: with `{.Completion,
  "input::Enter"}` the overlay closes and the editor does nothing with the key (the action is the app's;
  text `"abx"`); without the key set Enter is the editor's (a new line, `"abx\n"`). Upstream offers the
  handler only Enter and Escape (and what `route_overlay_action` routes): Tab is the editor's
  `IndentInline`, never offered. **The key set is for an editor drawn with the base `gk.editor`**: the kit's
  code editor (`gk.code_editor`, `component::input::Editor`) installs its own overlay handler each time it
  renders (`install_action_handler`: its completion menu takes the menu's actions, Enter accepts the item),
  replacing the app's. Measured: under `gk.code_editor` both modes gave `"abxfirst\n"` and
  `has_overlay_action_handler` true.
- **A presented diagnostic (9.34)**: protocol mode `diagnostic-shown` (read back as the editor's popover,
  then cleared) and gallery page `editor7-diagnostic` (the popover after a click).

### 9.41 Item procedures: closures that draw one item, given as typed procedures

33 functions were skipped as `item-render-closure`: closures upstream calls while it draws one item of
a component, handing it something 9.16 could not read. They are bound by one rule (`gen/gk_items.py`,
hooked into `closure_desc` where 9.16's rule refuses a closure with values, and where no other rule took
a closure that returns a value); the runtime is `core/src/items.rs` and `odin/gk/rt_items.odin`.

**What the app writes.** One procedure per kind of item, whose parameters are the values upstream hands,
typed, given with `gk.items(cx, state, procedure)`:

```odin
day :: proc(a: ^App, cx: ^gk.Cx, item: gk.Calendar_Item, st: gk.Base_Calendar_Item_State) -> gk.Element {
	return item->style(.Rounded_Full)->bg(gk.theme(.Primary) if st.is_today else gk.theme(.Background))
}
gk.base_calendar(gk.Id("cal"), a.cal)->item(gk.items(cx, a, day))         // in the view procedure
```

`gk.items` is a procedure group (`gen_items.odin`, one member per kind: 22, the 9.16 closures included),
so a procedure that fits no closure does not compile. It makes the row source of a (procedure, state)
pair on first use and gives the same one after (`items_source`), so it may be called in a view
procedure (a source of no rows made there binds no row and runs no app code). The builder still takes a
`Row_Source`, so `gk.row_source` plus `gk.row_context` (9.16) keep working; a source `gk.items` made
for one kind and given to a closure of another stops at the builder (`items_check`: "gk.items was given
a procedure of the kind .., but this closure calls one of the kind .."). Each such builder's table
entry ends with a comment naming the procedure to write.

**Rows.** As 9.16: each call of the closure in a frame is a row, keyed by its order in the frame, or by
the first value when it is an integer (a tree entry's index: `ctx_key`); the row is described between
frames by the procedure (its trampoline reads the values with `row_context` and passes them), described
again when upstream hands it other values, and the frame that first draws it shows upstream's own
drawing (the handed element as it came, or the type's plain value). Nothing calls the app while
upstream draws.

**The values** (descriptor `["items", wrap, ptypes, params, vals, out]`, one entry of `vals` per value):

| entry | what | Odin parameter |
|---|---|---|
| `["val", d]` | what 9.16 reads: plain values, a library object (a temporary handle) | its type |
| `["reads", rt]` | a record of the value's reads, for a value 9.16 could not read: a Clone value whose methods are reads (`fn x(self)` or `fn x(&self)`), constructors and builders; kit state handed by reference (`&mut PopoverState`, `&CommandState`, `&mut Self` of a closure over the value itself); a generic context with the closure's type argument (`ComboboxTriggerContext<'_, GkSearchDelegate>`). Reads that take arguments or return an object, an element or a view are left out; a data enum whose read would change its Odin type is left out (`ComboboxTriggerContext::size`, `gk.Size`); a state with no read left passes nothing | `gk.Base_Calendar_Item_State`, `gk.Time_Field_Segment_State`, `gk.Base_Tree_Entry_State`, `gk.Tree_Entry_Reads`, `gk.Popover_State_Reads`, `gk.Hover_Card_State_Reads`, `gk.Command_State_Reads`, `gk.Combobox_Trigger_Context`, `gk.Nav_Page_Reads` (named `X_Reads` when X is already a state or an object the app holds) |
| `["handed", node, rt]` | an element upstream built for the item, to decorate (`CalendarItem`, `TimeFieldSegment`, `NavPage`) | the node (`gk.Calendar_Item`), then the record of its own reads when it has some not given otherwise (`gk.Nav_Page_Reads{index, phase, operation, progress}`) |
| `["view"]` | a view upstream hands (a tooltip's content) | `gk.Kit_View`, a node to place |

The record's `Wire` impl calls the reads (`gk_items.field_put`; a `self` read on a copy), so its bytes are
the row's identity and what the procedure reads. The reads count bridged ("a field of the record ..").

**Handed elements.** The procedure gets the element as a node of the row's own tape: the node's synthetic,
private constructor `handed` (`calendar_item_handed(0)`, one per handed node type, numbered in
`opcodes.lock`; not counted, not in the sample or the gallery) carries the value's position; the
`Kit_View` node is the library's `KIT_VIEW 41` (u8 position). The procedure styles it, gives it children
and handlers, returns it or places it inside its own elements. Rust: such a row is decoded inside
upstream's closure (`items::row`: the source is a decoded one, `source::row_tape` and `genapi::with_tape`,
as a delegate shim decodes its rows; the drawing view watches the source), with the handed values in a
thread-local slot that the `handed` constructors take from (`items::handed::<T>(i)`); placing one twice,
or outside its row, stops: "the .. an item procedure is handed was placed twice, or outside its own row"
(exit 2). Rows without a handed value are the row's view, as 9.16 (`source::ctx_row`).

**Typed results.** A closure that returns one element type (`Notification::action -> Button`, the base
`HoverCard::content -> Stateful<Div>`) takes a procedure returning that type; the row's root is decoded
as it (`build_<node>`), and before its first description the closure returns the type's plain value
(`Button::new("gk-item")`, `div().id("gk-item")`; `gk_items._fallback`, no rule binds a type without one).

**FnOnce(X) -> E without a window**, X an element node (`DialogClose::trigger(FnOnce(Button) -> E)`, both
crates): the build closure of DESIGN.md 12 with E taken as X: `gk.base_dialog_close()->trigger(
gk.base_button(gk.Id("x"))->child(gk.text("Close")))` (the ops are applied to the button upstream makes).

Bound: `base::Calendar::item`, `TimeField::render_segment`, `NavStack::item`, `Tree::item`,
`TooltipOverlay::render_with` (a state builder: `gk.tooltip_overlay(cx)->render_with(..)->build()`),
`Popover::content` and `HoverCard::content` (both crates), `DialogClose::trigger` (both),
`component::Combobox::render_trigger`, `Command::empty`, `header`, `footer`, `Notification::content` and
`action`; the reads of `ComboboxTriggerContext` (but `size`, skipped as builder-getter),
`TimeFieldSegmentState`, `HoverCardState`, `CalendarItemState`, `TreeEntryState` and `NavPage` (`view`
stays skipped: the page draws its view itself). `CalendarItemState::new` and its five setters are value
helpers: no kit function takes a `CalendarItemState`, the app reads the record.

Tests: `core/tests/items` (`run-items.sh`): `items-calendar` (a handed day decorated, rows described in
the first frames and none after), `items-time-field` (a handed segment placed inside the app's element),
`items-tree` (an index key, a record of an object's reads, a record of a value's `self` reads),
`items-popover` (state by reference with its own context), `items-command` (with the app's context),
`items-hover` (a typed result, opened by the pointer), `items-close` (FnOnce(Button) -> E), misuse
`items-wrong-kind`. Gallery pages `text-token` and `diff-header` now give their 9.16 closures with
`gk.items`.



## 11. App protocols: actions, key bindings, timers, windows, the clipboard

Runtime: `core/src/app.rs`, `odin/gk/rt_app.odin`;
generator rules: `gen/gk_app.py` (hooked into `gen/gk_model.py` at two points); numbers: the
app block of `core/wire/ops.txt` (call 64-111, shape 20-27, node 16-23, hkind 9-10); tests:
`core/tests/app`, `core/tests/run-app.sh`.

### 11.1 Actions

An action is a u32 id. `C_ACTION_NEW` (`str name, str data`) names one, immediately:

- a name GPUI's action registry knows (`ui::Cancel`, `dock::ToggleZoom`, any kit or gpui
  action) is that **built-in** action, built with `App::build_action(name, data)` (`data` is
  JSON, empty for none); its id has bit 31 set (`gk.ACTION_BUILTIN`, `gk.is_builtin`);
- any other name is an **app action**: `GkAction { id }`, one Rust type for every app action,
  whose `partial_eq` compares ids and whose `name()` is the name the app gave.

The same name and data always give the same id. An action that travels to the app (a menu's,
a listener's payload) is its id (`impl Wire for dyn Action`): a built-in action the app never
named is entered in the table under its own name then. `genapi::action(r)` reads an id and gives
the boxed action; generated code takes actions anywhere upstream takes `Box<dyn Action>` or
`&dyn Action` (menus, tooltips, `Kbd`, key bindings).

| call | wire | when |
|---|---|---|
| `ACTION_NEW 64` | `str name, str data` -> `u32 id` | immediate |
| `BIND_KEYS 65` | `u32 n, (str keystrokes, u32 action, str context)` | buffered; `KeyBinding::load` with the platform's keystroke parser; a keystroke or a context predicate that does not parse is `GK_ERR_BAD_OP` |
| `ON_ACTION 66` | `u32 action, u32 slot, u32 generation` -> `u64 listener` (kind SUB) | an app-wide listener (`App::on_action::<GkAction>`, installed once, at the end of the bubble phase); app actions only; release ends it |
| `DISPATCH_ACTION 67` | `u32 action` | buffered; `Window::dispatch_action` on the turn's window (to the focused element, as a key binding would), else `App::dispatch_action` |
| `ACTION_AVAILABLE 68` | `u32 action` -> `u8` | GPUI's availability: by type, so for app actions it is true when any app-action listener is on the path (DESIGN.md section 8) |
| `ACTION_KEYSTROKES 69` | `u32 action, str context` -> `opt<str>` | the highest-precedence binding's keystrokes as GPUI displays them |
| `CLEAR_KEY_BINDINGS 70` | | buffered |

Shape `ACTION 20`: payload `Action` (u32 id). Every action listener has it; a handler may take
the `gk.Action` or nothing.

**Element listeners, by generator rule** (`gen/gk_app.py`, `action_listener`):

- a method generic over the action type whose closure takes it, `fn on_action<A: Action>(self,
  listener: impl Fn(&A, &mut Window, &mut App))` and `capture_action`: A is `GkAction`; the op
  takes `u32 action, u32 handler` (descriptor `["action", "listener"]`; Odin `Action_Handler`,
  made with `gk.act(action, handler)`); the library's listener (`genapi::action_listener`)
  fires the handler for that action and calls `cx.propagate()` for every other app action, so
  several listeners on one element each get their own. The Odin runtime refuses a built-in
  action here at the app's line.
- a listener over a boxed action, `on_boxed_action(&dyn Action, Fn(&dyn Action, ..))`: an
  ordinary handler of shape ACTION. GPUI registers it by the action's type, so it is the way to
  listen to a built-in action on an element.
- `key_context<C: TryInto<KeyContext, Error = E>>` takes a string (`bounded`), and a generic
  parameter named only as another bound's associated type (`E`) does not block a method
  (`bound_only`).

Odin:

```odin
save := gk.action(cx, "editor::Save")
gk.bind_keys(cx, {"ctrl-s", save, "Editor"}, {"ctrl-o", open, ""})
gk.div()->key_context("Editor")->track_focus(f)->on_action(gk.act(save, gk.on(do_save)))
listener := gk.on_app_action(cx, open, gk.on(do_open))     // gk.release(cx, listener)
gk.dispatch_action(cx, save)
keys, ok := gk.keystrokes(cx, save)                        // "ctrl-s"
```

### 11.2 Timers

`TIMER_NEW 71` (`u32 ms, u8 repeat, u32 slot, u32 generation` -> `u64 timer`, kind `TASK 9`): a
foreground task that awaits `BackgroundExecutor::timer` and fires the kept handler as an EVENT
turn (shape UNIT) on the main loop, so never during a draw; the turn's window, if it had one, is
in scope when it is free. Releasing the handle drops the task, which cancels it. A one-shot timer
releases itself after it has fired, and the Odin runtime forgets its handler then (`timer_fired`
in `event`); releasing it afterwards does nothing. Odin: `gk.after(cx, ms, h)`,
`gk.every(cx, ms, h)` -> `gk.Timer`; a handler marks its view dirty as any handler does.
Offscreen the dispatcher's clock is simulated: the gk-meter script command `advance MS` moves it
on (timers due fire), `sleep` does not.

A handler kept in a view's init or handlers (a timer, an
app-wide or window listener, a subscription, panel events) goes with that view: `gk.release` of
the view cancels its timers and drops the rest (`drop_kept_of`, `rt_handler.odin`), so none runs
again with the state of a view that is gone (test `timer-after-view`, `core/tests/run-review3.sh`).
The app may still release those handles afterwards; that does nothing more.

### 11.3 Windows

| call | wire |
|---|---|
| `WINDOW_OPEN 72` | `f32 w, f32 h, u64 root, str title, u8 flags, f32 min_w, f32 min_h, u8 kind, u8 has_pos, f32 x, f32 y, str app_id, u32 n, n bytes icon` -> `u64 window` (flags: 1 not resizable, 2 not minimizable, 4 not movable, 8 no focus, 16 hidden, 32 the kit's title bar; kind: 0 normal, 1 popup, 2 floating; app_id "" none, icon a PNG, 0 bytes none: 11.38) |
| `WINDOW_CLOSE 73`, `WINDOW_SET_TITLE 74`, `WINDOW_ACTIVATE 75` | `u64 window` (0: the turn's), title; buffered |
| `ON_WINDOW_CLOSED 76` | `u64 window, u32 slot, u32 generation` -> `u64 listener`; shape `WINDOW 21` (payload the window's handle) |
| `WINDOW_INFO 77` | `u64 window` -> `str title, f32 x, y, w, h, u8 active` |
| `WINDOWS 80` | -> `u32 n, u64 window` each, in the order opened |
| `QUIT_ON_LAST_WINDOW 81` | `u8` (default 1); buffered |
| `ON_WINDOW_SHOULD_CLOSE 96`, `ON_APP_QUIT 97`, `REDUCE_MOTION 98`, `SET_REDUCE_MOTION 99` | a window's close asked of the app, the app's quit, reduced motion: 11.38 |

`WINDOW_OPEN` builds on the same options as `OPEN_WINDOW` (offscreen, the meter's size wins) and
the same `WINDOW_INIT` turn. When a window closes (`App::on_window_closed`, installed at start)
its listeners fire, its handle is released (stale from then on) and, when it was the last window
and the app did not say otherwise, the app quits. Odin: `gk.open_window_with(cx,
gk.Window_Config{..}, view)`, `gk.close_window`, `gk.set_window_title`, `gk.activate_window`,
`gk.on_window_closed`, `gk.window_info`, `gk.windows`, `gk.quit_on_last_window`. The gk-meter
script drives several windows with `window N` (the N-th open window from then on) and counts
them with `windows`.

A window op given 0 (`gk.close_window(cx)`: the turn's window) and every other buffered write of an
EVENT turn is applied with the window its turn had (`rt::apply_deferred`), so `close_window(cx)` in a
key or click handler closes that handler's window (test `close-own-window`).

### 11.4 The clipboard

`CLIPBOARD_WRITE 78` (`str`, buffered) and `CLIPBOARD_READ 79` (-> `opt<str>`): GPUI's
`write_to_clipboard` / `read_from_clipboard` with text. Odin `gk.clipboard_write`,
`gk.clipboard_read`.

### 11.5 Ambient calls (generator rule)

A function of the ambient context is one call op, generated (`gen/gk_app.py`, `ambient`, hooked
into `classify_free` and the `[traits.role]` branch of `classify_trait`):

- a free function of the kit that builds no element: the palette scales (`amber(500)`), `hsl`,
  `oklch`, `set_locale`, `try_parse_color`, `window_paddings`, the easing curves, `step_value`;
- a method of a trait the kit implements on `Window` whose role in `api/overrides.toml` is
  `calls (ambient context: methods on Window and App)`: `WindowExt::close_dialog`,
  `close_all_dialogs`, `close_sheet`, `has_active_dialog`, `has_active_sheet`,
  `has_focused_input`, `push_notification`, `clear_notifications`.

Rules: every argument is one a call op can carry (`call_bad` of the state rule); `&mut Window`,
`&Window`, `&mut App`, `&App` are the turn's (`c.window()?`, `c.cx`); the return value, if any,
travels back in its descriptor's wire type; a generic parameter the caller would choose stops
the rule; `init` (run by the library at start) is left out. Rust: `fn c_N(r, c)` beside the
state calls, `<gpui_kit::Window as WindowExt>::close_dialog(window, cx)`. Odin
(`gen_ambient.odin`): `gk.<name>(cx, args..) -> value`, run at once through `ambient_call`
(`rt_app.odin`); one that returns nothing may not run in a view procedure, one that returns a
value may (`gk.blue(cx, 500)` in a view). base's names that collide get `base_`. The test is
generated with them: `core/tests/app/gen_ambient.odin` (`gk_app.odin_test`, called from
`gen/gkgen.py`) calls every ambient function once with the conformance sample's arguments,
in the app test's mode `ambient`.

Not ambient (left for the next round, with their reasons kept): `open_dialog`, `open_sheet`,
`open_sheet_at`, `open_alert_dialog` (their argument is a build closure that outlives the call:
it needs the view-built closure of section 11.6, not built yet), `notifications` and
`focused_input` (results the binding cannot carry), `remove_notification<T>` (a type the caller
chooses), `ActiveTheme::theme` (the theme global, an object by the rule of section 13).

### 11.6 View-built closures, described values, fallible calls

Runtime: `core/src/overlay.rs`, `odin/gk/rt_overlay.odin`; rules: `gen/gk_overlay.py` (hooked into
`gen/gk_model.py` at `scan_types`, after classification, `closure_desc`, `resolved`, `state_method`
and `call_bad`, and into the two emitters' `dec`/`put_ret`, `otype`/`put`); numbers: the overlays
block of `core/wire/ops.txt` (call 144-175, shape 36-43, node 32-39, hkind 11); tests:
`core/tests/overlay`, `core/tests/run-overlay.sh`; gallery: `overlay_pages.odin` / `.rs`.

**View-built closures.** A closure upstream keeps and calls later, `Fn(T, &mut Window, &mut App) ->
T + 'static`, given to a **call op** is view-built (descriptor `viewbuild`; in an element op the
same closure stays a `build` over a fragment of the view's own tape, section 12 of DESIGN.md):

- Odin: `gk.built(cx, state, proc(s: ^S, cx: ^Cx) -> T) -> gk.Built(T)` (`Built(T) :: struct {using
  view: View}`). The runtime makes one view per (procedure, state) pair, kept until
  `gk.release(cx, b.view)`; the same pair gives the same view, and every `gk.built` marks it for
  redraw. Its root is the node the procedure returns (`gk.dialog()->title(..)`). The call takes
  it typed: `gk.open_dialog(cx, gk.built(cx, a, confirm))` will not compile with a procedure
  returning another type. Not in a view procedure (it may make a view).
- Wire: the view's `u64` handle. Rust: `crate::overlay::ViewBuild::take(view, c, &[ctor node
  types], "T")?` checks the handle, mounts the view in the turn's window (the first open window for
  a write applied later) and marks it dirty, so the frame that first calls the closure has its
  tape. `vb.apply(v, window, cx, apply_<node>)` reads the view entity's current tape (so the drawing
  view, the kit's dialog layer, depends on it and draws again when it is described again), checks
  the root's node type (another type stops: "the procedure given to gk.built for a Dialog returned a
  node of type N, not a Dialog", exit 2) and applies the root's ops to the value upstream hands in
  with the generated `apply_<node>` (`genapi::with_root`). Before the first description it returns
  upstream's value unchanged (one frame at most). A released view stops at the next build with
  `genapi::released_shown` (exit 2).
- The view is an ordinary view: its handlers (`on_ok`, `on_cancel`, children's `on_click`) run
  with its state between frames and mark it dirty, so the dialog's content changes while open.
- Bound by rule: `WindowExt::open_dialog`, `open_alert_dialog`, `open_sheet`, `open_sheet_at` (the
  kit's `Sheet` is a value node, `[node] types` in `api/overrides.toml`, so `Sheet::title`,
  `footer` and `on_close` bind as its ops).

**Build closures with the entity's context.** `Fn(PopupMenu, &mut Window, &mut Context<PopupMenu>)
-> PopupMenu` in an element op is a `build` closure like one with `&mut App` (the context derefs to
it). This binds `ContextMenuExt::context_menu` on `Div` (the wrapper `ContextMenu<Div>` instantiated
for one element type by its full path, `gk_overlay.OVERLAY_INSTANCES`; the blanket trait entered in
Div's tables), `SidebarMenuItem::context_menu` and the other menus' builders.

**Described values.** An entity type upstream builds itself and hands to such a closure is a value
node (`[node] types = ["Sheet", "PopupMenu"]`): its `self -> Self` methods are its ops (`menu(label,
action)` with an action of 11.1, `separator`, `label`, `link`, `menu_with_check`, `menu_element`
with a fragment, `item`, `min_w`..). It has no public constructor: Odin starts it with
`gk.popup_menu()`, a constructor the library never runs on its own (`overlay::described`, not
counted). `PopupMenu::submenu(label, window, cx: &mut Context<Self>, f)` is
`overlay::submenu(menu, label, window, cx: &mut App, f)`: the child is built at once with
`PopupMenu::build` and attached with `item(PopupMenuItem::submenu(..))`, which upstream wires to its
parent when the menu draws. `PopupMenu::rebuild` (needs the menu entity) is skipped as a builder
mutator, `is_empty` as a builder read.

**The dropdown button.** `DropdownMenu::dropdown_menu` on the kit's Button returns
`DropdownMenuPopover<Button>`, which has no public path, so generated Rust cannot name it: node
`DROPDOWN_BUTTON 32` of the library (`u32 button node, u32 menu node`), Odin
`gk.dropdown_menu(gk.button(..)->label(..), gk.popup_menu()->menu(..))`; counted bridged by
`gk_overlay.LIBRARY_NODES`.

**Fallible calls.** `Result<(), E>` as a call's result is `opt<str>` on the wire (Odin
`Maybe(Temp_String)`: the error's `Debug` text, `nil` for Ok; `overlay::outcome`). A state
constructor or builder that returns `Result<Self, E>` is bound like one that returns `Self`; an Err
stops the app with the error's text (`overlay::Fallible(..).callN`, exit 2): a schema the app wrote
wrongly. A `&self` method that takes the window is a call op run at once with the turn's window
(`state_call`), like an update that returns a value. This binds `QuestionnaireState` (constructor,
`with_current_item`, `choose`, `set_answer`, `go_next`, the focus calls) and the other states' calls
that return `Result<(), E>`.

Not bound yet, with their reasons: `PopupMenu::build` as a call op (it returns the menu entity;
nothing takes one), the `*_with_icon` items (`Icon` is entity state: "a state object by value"),
`Notification` (an `id::<T>()` the caller chooses, closures that return a value),
`ToastManager<T>` (generic), `Tooltip::new` / `key_binding` (a node given to a static call),
`Kbd::binding_for_action` (statics returning an element type).

### 11.7 Dock panels as Odin views

Runtime: `core/src/dock.rs`, `odin/gk/rt_dock.odin`; rules: `gen/gk_overlay.py` (`panel_entity`,
`panel_view`, `mark_panel_traits`, hooked into `Model.resolved` and the emitters); tests: mode
`dock` of `core/tests/overlay`; gallery: page `dock-panels`.

`GkPanel { view: Entity<GkView>, focus, data, events }` implements gpui-base's `Panel` and the
component layer's `Panel` once, an entity behind a state handle (Odin `gk.Dock_Panel`):

- its render is the Odin view (`gk.panel(cx, view, gk.Panel_Data{..}, events)`; the view is mounted
  in the turn's window and described like any view, its handlers and dependencies its own);
- `panel_name`, `tab_name`, `title`, `closable`, `zoomable`, `visible`, `inner_padding`,
  `title_bar` answer from `Panel_Data` the app pushed (`gk.panel_set`, buffered; the panel entity
  is notified so the dock draws the new data), never by calling the app while the dock draws;
  names are interned (`panel_name` is `&'static str` upstream);
- `set_active`, `set_zoomed`, `on_added_to`, `on_removed` reach the app's handler as a queued
  `gk.Panel_Event{kind, flag}` (shape `PANEL_EVENT 36`); `gk.panel_emit` emits upstream's
  `PanelEvent` (zoom in, zoom out, layout changed). The kit 0.9.1 zooms tab groups, never a panel,
  so a panel's ZoomIn is delivered and ignored (tested);
- **by rule**: a generic `P: Panel` is GkPanel (descriptor `entity crate::dock::GkPanel`, Odin
  `gk.Dock_Panel`), and `Arc<dyn PanelView>` given to the kit is made from a panel handle
  (descriptor `panelview base|kit`: gpui-base and the component layer each have a `PanelView`).
  This binds `DockArea::add_panel`, `add_panel_view`, `remove_panel`, `DockLayout::panel`,
  `panel_view`, `PanelHandle::new`, `from_view`; the trait methods of both `Panel`s and both
  `PanelView`s count bridged (GkPanel's own, or upstream's blanket impl over `Entity<GkPanel>`).
- `gk.dock_area_skinned(cx, id)` is the kit's styled dock (`DockSkin::dock_area`, whose result
  pairs the area with its skin, which a call cannot carry), call `DOCK_AREA_SKINNED 147`;
  `gk.dock_view(dock)` draws a dock area in a view (node `DOCK_VIEW 33`).

| number | what |
|---|---|
| `call PANEL_NEW 144` | `u64 view, str name, opt<str> tab name, str title, u8 flags (1 not closable, 2 not zoomable, 4 hidden, 8 no padding, 16 no title bar), u32 slot, u32 generation` -> `u64 panel`; needs a window |
| `call PANEL_SET 145` | `u64 panel`, the same data; buffered |
| `call PANEL_EMIT 146` | `u64 panel, u8 event`; buffered |
| `call DOCK_AREA_SKINNED 147` | `str id, u8 has version, u64 version` -> `u64 dock area` |
| `node DOCK_VIEW 33` | `u64 dock area` |
| `shape PANEL_EVENT 36` | `Panel_Event {u8 kind (0 active, 1 zoomed, 2 added, 3 removed), bool flag}` |

Not built yet: restoring a saved layout (`DockArea::load`, `PanelRegistry`, `PanelSource`: a
recursive `PanelState` and a request to the app per panel through 9.2), `Panel::dropdown_menu`,
`title_suffix` and `toolbar_buttons` (upstream's defaults: a view-built menu and elements per
panel would be the next step), `DockAreaRenderer` / `TabGroupRenderer` (the skin is the kit's),
the objects the dock hands to renderers (`TabGroupContext`, `DockContext`, `DropIndicator`,
`DragPanel`), `NodeId`-addressed calls (`split_at`, `set_split_sizes`, `set_zoomed_in`), and
`ResizableState` / `NavStackState`.

**A real window.** Mode `window-real` runs the test app with the real platform
on a display `gates/display.sh` provides (a private Xvfb or compositor): the size the app gave comes back through
`gk.window_info` (420 x 300), and the X server holds the title the app gave and the one it set
later (`xwininfo` while the app runs). `gk.window_info`'s title is empty on Linux: gpui-fast reads
a window's title back on macOS only (`PlatformWindow::get_title`).

### 11.8 What the dock hands out, saved layouts, a panel's own title bar

Runtime: `core/src/dock.rs`, `core/src/overlay.rs`, `odin/gk/rt_dock.odin`,
`odin/gk/rt_overlay.odin`; rules: `gen/gk_overlay.py` (`resolved`, `mark_handed`, `LIBRARY_CALLS`,
hooked into `Model.resolved`, the settle step's state filter, `uses_protocol`, and the two emitters'
`dec`, `put_ret`, `otype`, `put` and state emission at one line each); numbers: the dock lines of
the overlays block of `core/wire/ops.txt`; tests: modes `dock-ids`, `dock-bad-node`, `dock-layout`,
`nav` of `core/tests/overlay`; gallery pages `dock-split`, `dock-hooks`, `dock-restore`, `menu-kbd`,
`menu-dropdown-anchor`.

**One descriptor family for what the kit hands out and takes back**, `["dock", what, ..]`, call-safe,
`bridged` (each needs the library's tables):

| what | upstream | wire | Odin |
|---|---|---|---|
| `node_id` | gpui-base's `NodeId` (no public constructor) | u64 | `gk.Node_Id` (distinct u64) |
| `panel_id` | `PanelId` | u64 | `gk.Dock_Panel_Id` (`gk.panel_id(cx, panel)`) |
| `view` | gpui's `AnyView`, `impl Into<AnyView>` | u64 view handle | `gk.View` |
| `panel` (read back) | `Arc<dyn PanelView>` (base or kit) | u64 panel handle, 0 for the kit's own placeholders | `gk.Dock_Panel` |
| `entity` (read back) | `Entity<T>`, `WeakEntity<T>`, T a state type | u64 state handle | the state's type |
| `ms` (read back) | `Duration` | i64 nanoseconds | `time.Duration` |
| `area_state`, `panel_state`, `panel_info` | `DockAreaState`, `PanelState`, `PanelInfo` | a tree, below | `gk.Dock_Area_State`, `gk.Panel_State`, `gk.Panel_Info` |

- A `NodeId` the library writes is remembered (`crate::dock::node_id`); one it never handed out is
  `GK_ERR_BAD_OP` in a call op ("node id N was never handed out by the dock"), which stops the app at
  the buffered write as any failed write does (mode `dock-bad-node`, exit 2). Node ids come from the
  reads: `dock->zoomed_group`, `tree->node_ids`, `tree->find_panel_node`, `node->id`, `group->node`,
  `ctx->node`, `drag->source`. They make `split_at`, `set_split_sizes`, `set_zoomed_in`,
  `PaneTree::split`, `set_active`, `set_sizes` and `InsertTarget` (`move_panel`, `insert_panel`) usable.
- An app view taken by a call op is drawn in the turn's window and marked dirty (as 11.6's views
  are); read back, a view the library was given is its handle, any other 0 (`NavStackState::push`,
  `current`, `views`, `pop`, `Root::new`, `Root::view`, `FieldBuilder::View`).
- An entity read back gets one state handle per entity while it lives (`crate::dock::entity_handle`),
  with the app's first window as its window.
- **States the library hands out** (`gk_overlay.HANDED`): an entity state with no public constructor
  whose methods are the state rule's once something hands one out: `TabGroup` (`gk.panel_group(cx,
  panel)`: the group the dock told the panel it was added to, call `PANEL_GROUP 148`) and
  `AppMenuBar` (`gk.app_menu_bar(cx)`, call `APP_MENU_BAR_NEW 155`, drawn by `gk.app_menu_bar_view`,
  node `APP_MENU_BAR 36`). `TabGroup::context` then hands out `TabGroupContext`, whose reads and calls
  the object rule binds (13), and through it `DropIndicator` and `DragPanel`.
- **Default constructors** (`gk_overlay.DEFAULT_CTORS`): a state with only `impl Default`
  (`ResizableState`) is made by `T::default()` (`gk.resizable_state(cx)`; not an upstream function of
  its own, no coverage row).

**Saved layouts.** `DockArea::dump` and `load` are generated calls over the tree:

```
PanelState:  str panel_name, u32 n, PanelState each, PanelInfo
PanelInfo:   u16 variant: 0 Stack {u32 n, f32 each, u64 axis}, 1 Tabs {u64 active index}, 2 Panel {str JSON, "" for null}
DockAreaState: opt<u64> version, PanelState center, then left, right, bottom: u8 present, PanelState, u16 placement, f32 size, u8 open
```

Odin reads and writes them by reflection (`Panel_Info` is a union of `Panel_Info_Stack`, `_Tabs`,
`_Panel`). A load rebuilds panels through upstream's registry: `gk.register_panel(cx, name,
handler)` (call `PANEL_REGISTER 150`: `str name, u32 slot, u32 generation`) registers a builder that
makes a GkPanel with no view yet and asks the app with a queued `gk.Panel_Build {panel, name, state,
dock_handle}` (shape `PANEL_BUILD 37`), delivered after the load's update and before the next frame;
the app answers with `gk.panel_fill(cx, panel, view, data, events)` (call `PANEL_FILL 151`, buffered).
A restored panel therefore draws nothing for at most one frame. `Panel_Data.info` is the panel's own
JSON, written back by `Panel::dump` (a GkPanel's dump is its name and that info). Also bridged by
library calls: `gk.build_panel(cx, dock, state)` (`PanelRegistry::build_panel` with a
`PanelBuildContext` made of the dock and the state, call `PANEL_BUILD_NOW 152`),
`gk.pane_tree_to_state(cx, tree, dock)` (`PaneTree::to_state` over a `PanelSource` the library fills
from the dock's panels before the call: upstream's methods get no context, call 153),
`gk.pane_tree_from_state(cx, dock, state, root)` (`PaneTree::from_state` with a `PanelBuilder` over
the registry, call 154; the built panels are kept alive by the library), and
`gk.pane_node_kind` / `gk.pane_node_walk` (`PaneRef` as a `gk.Pane_Kind` value, the walk's nodes as
a list; calls 156, 157). `PanelBuildContext`'s reads are the `Panel_Build` request's fields.

**A panel's own title bar** (`Panel::title_suffix`, `toolbar_buttons`, `dropdown_menu`): three more
fields of `gk.Panel_Data`, pushed as all of its data is: `suffix` (a view drawn at the end of the title
bar), `toolbar` (a view whose root's `gk.button` children are the toolbar's buttons: the library
decodes them from the view's tape each time the bar draws) and `menu` (a `gk.Built(gk.Popup_Menu)`,
applied to the bar's menu as 11.6's view-built closures are). The wire of `PANEL_NEW`, `PANEL_SET` and
`PANEL_FILL` ends with `u64 suffix, u64 toolbar, u64 menu view, str info`.

**Menus.** Node `KBD_FOR_ACTION 35` (`u32 action, u8 how, opt<str> context, u64 focus`) is the kit's
Kbd for an action's binding, or nothing when it has none: `gk.kbd_for_action(action, context)`,
`gk.kbd_for_action_in(action, focus)`, `gk.kbd_global_for_action(action)` (the three `Kbd` statics,
which return `Option<Kbd>`: an element no call op can return). Node `DROPDOWN_ANCHOR 34`
(`gk.dropdown_menu_anchored(button, anchor, menu)`) is 11.6's dropdown button with
`dropdown_menu_with_anchor`.

Not built (reasons in `docs/STATUS.md`): the renderers (`DockAreaRenderer`, `TabGroupRenderer`, and
`DockContext`, which only they receive), `DockSkin` (its result pairs the area with an `Rc<Self>`),
`DragPanel` by value (`drop_panel`, `drag_panel`: a type that renders, classified as state),
`AnyDrag`, `DockToggleEvent` (emitted by the component crate's impl on DockArea, which the extract
does not list), the toast manager and tooltip families.

### 11.9 Dock renderers over app views keyed by node

Runtime: `core/src/dock.rs` (`GkRenderer`), `odin/gk/rt_dock.odin`; rules: `gen/gk_overlay.py`
(`resolved4`, `after_classify4`, `LIBRARY_HANDOUTS`); tests: mode `render` of `core/tests/overlay`;
gallery page `dock-renderer`.

A Rust app gives a dock its look by implementing `DockAreaRenderer` and `TabGroupRenderer`, whose
hooks upstream calls while it lays the dock out. App code never runs then (decision 3), so the
binding implements both traits once, `GkRenderer` (a library object, `gk.Dock_Renderer`), and
answers each hook from an app view the app gave for that hook and a key, between frames:

- `gk.dock_renderer(cx, hooks, handler)` makes it (call `DOCK_RENDERER_NEW 160`: `u32 hooks, u32
  slot, u32 generation` -> `u64`, kind OBJ); `hooks` is a `bit_set[gk.Dock_Hook]` of the hooks the app
  answers (the others keep upstream's defaults). `Rc<dyn DockAreaRenderer>` and `Rc<dyn
  TabGroupRenderer>` given to the kit (`DockArea::with_renderer`, `TabGroup::with_renderer`) are this
  object by rule (descriptor `["dock", "renderer"]`), so `gk.dock_area(cx, id, nil)->with_renderer(r)->
  build()`. `tab_group_renderer` hands upstream the same renderer.
- A hook in `hooks` looks its (hook, key) up. The key is the hook's subject: a dock's placement, a
  split's or a tab group's node id (`Node_Id`), a divider's axis + 2 * dragging, 0 for the area's
  frames and the drop indicator. With no answer for the context upstream draws with (a hash of the
  context's reads: open, size, panels, active tab, zoom..), the hook sends one queued request, shape
  `DOCK_RENDER 38`, `gk.Dock_Render {renderer, hook, key, node, placement, axis, flags, size, dock,
  group, panel, panel_name}`, and draws upstream's default (or the older answer) this frame. The
  app answers with `gk.dock_render_answer(cx, r, hook, key, view)` (call `DOCK_RENDER_ANSWER 161`,
  buffered: the view is mounted and every window drawn again; view 0 gives upstream's default back).
  One frame of the default at most, as for a restored panel (11.8).
- The contexts upstream hands the hooks reach the app in the request: `dock: gk.Dock_Context` (the
  dock hook) and `group: gk.Tab_Group_Context` (the tab group hooks), library objects whose reads and
  calls are bound by the object rule (13), `dock->toggle(cx)`, `group->select_tab(cx, i)`. They are kept
  by the library until the next request for the same key (so a click handler of the chrome can call
  them), then released. Because the library hands them out, `DockContext` is reachable
  (`gk_overlay.LIBRARY_HANDOUTS`, a synthetic hand-out item that is not counted).
- Drawing an answer: an element hook (`render_dock`, `render_tab_bar`, `render_active_panel`,
  `render_split_handle`, `render_empty`) decodes the app view's current tape where upstream wants the
  element (`genapi::root`), so its handlers are the view's own and it is drawn again when the view is
  described again. The element upstream hands the hook (a dock's content, the active panel's view) is
  drawn where the app's view put `gk.dock_slot()` (node `DOCK_SLOT 37`; nothing when the view has no
  slot). A frame hook (`frame`, `center_frame`, `split_frame`, the tab group's `frame` and
  `content_frame`, which return `Stateful<Div>`) takes the app view's root, which must be a `gk.div()`,
  `gk.h_flex()` or `gk.v_flex()` (another node stops the app, exit 2), and gives it upstream's id: that div
  with its styles, handlers and children is upstream's frame. The drop indicator's view is drawn in
  the rectangle the drop would fill (`DropIndicator::to`).
- `build_placeholder` (a panel no registered builder makes, in `hooks` as `.Placeholder`) makes an
  empty panel and asks the app with the request's `panel` and `panel_name`; the app fills it with
  `gk.panel_fill` (11.8).
- A dock upstream does not draw (closed) calls no hook, so it asks nothing until it is drawn again.
- `gk.test_dock_render(cx, r)` (call `DOCK_RENDER_TEST 162`): requests sent, answers kept, hooks drawn
  from an answer.

Counted bridged: the 13 methods of the two renderer traits (`GkRenderer`), `DockArea::with_renderer`,
`TabGroup::with_renderer`; bound by the object rule now that something hands one out: the 7 methods of
`DockContext`.

### 11.10 The library clock; the toast manager

`std::time::Instant` has no number of its own. Both ways it is the time since the library's clock
began (the first time anything read it), i64 nanoseconds on the wire, `time.Duration` in Odin
(descriptor `["dock", "instant"]`; `crate::overlay::instant`, `put_instant`); `gk.clock_now(cx)` reads
the clock (call `CLOCK_NOW 159`). The offscreen runner's simulated clock does not move it: it is the
process's monotonic clock.

`ToastManager<I, T>`, the kit's bookkeeping of a toast stack (which toasts are entering, present,
ending, and when a timeout ends one), is a generic calculator the app calls, so it is a library
object instantiated by `[values] objects` (13.8) as `ToastManager<u64, u64>`: ids and values are the
app's own numbers (an index into its toasts, a view handle). Its 10 methods are bound: `new` from a
`gk.Toast_Motion`, `push(id, value, gk.Toast_Options, now)` (the replaced value back), `dismiss`,
`dismiss_all`, `advance(now, paused)` (a `gk.Toast_Advance_U64_U64`: presented, ending, removed),
`iter` and `visible` (lists of `(id, value, status)`), `get`, `len`, `is_empty`. The app draws its toasts
itself (a timer calls `advance`). Test: mode `toast`.

Also bound by the clock: `HorizontalScrollClaim::at` (values family).

### 11.11 Generic elements bounded by a kit trait: Popover::trigger

`Popover::trigger<T: Selectable + IntoElement>` (the kit's and gpui-base's) takes an element that
also shows a selected state. A kit trait whose implementors are node types is one of them (`[lists]
unions`, the rule of 9.12): `"Selectable" = "Selectable_Element"`, a union of the kit's
eleven Selectable elements declared in `rt_overlay.odin` (`gk.button(..)`, `gk.tab(..)`,
`gk.list_item(..)`, ..). Upstream's popover sets the trigger's selected state from its own.

### 11.12 Popovers, dialogs and menus driven from code

- A popover is open while the app says so: `gk.popover(id)->open(a.open)->on_open_change(h)` (upstream's
  controlled popover). The app opens and closes it from any handler by changing its state; a click
  outside or escape reports through `on_open_change`. Test: mode `popover`.
- Dialogs, alerts and sheets open and close from handlers through the view-built closures of 11.6
  (modes `dialog`, `alert`, `sheet`); menus open from their trigger (11.6, 11.8).

### 11.13 A dock the user toggled

The kit's skin reports a dock the user opened or closed with its buttons as `DockToggleEvent` (an
`EventEmitter` impl of the component crate that the extract does not list). `gk.on_dock_toggle(cx,
dock, handler)` subscribes (call `DOCK_TOGGLE_SUBSCRIBE 163` -> a subscription), the handler gets
`gk.Dock_Toggle {placement, open}` (shape `DOCK_TOGGLE 39`): `DockToggleEvent::placement` and
`is_open` are its fields (counted bridged). The app's own `toggle_dock` and a renderer's
`DockContext::toggle` report nothing, as upstream (tested in mode `render`).

### 11.14 Observations: closures upstream calls while it lays out

A closure that gets values and no window, app or entity context and returns nothing, `impl Fn(..)`
(`Popover::on_position(Fn(ResolvedPosition, Bounds<Pixels>))`, `Popup::on_position`,
`Positioner::on_position(Fn(ResolvedPosition))`), is called by upstream while it lays out a frame,
where app code may not run (DESIGN.md section 0, decision 3). It is a handler of the payload's shape
(the app passes `gk.on(proc(a, cx, p: gk.Resolved_Position))`, two values as `gk.Event_117`), and the
library's closure only records: `crate::views_after::observe` encodes the values as that handler's
EVENT request and keeps it when they differ from the last values the same view's handler id was given.
One main-thread task (spawned through the app's `AsyncApp`, kept when the app starts, because no
context is in scope where upstream calls the closure) hands the kept requests to the queue of events
raised outside an update (`rt::queue_event`): each is delivered after the frame with its view's window.
The handler marks its view for redraw; the view lays out the same values again, which are not delivered
again, so an observation does not keep the app drawing. The comparison is per view and handler id, not
per tape generation. Generator rule: `gk_overlay.observed` (a hook in `Model.closure_desc`) and
`gk_overlay.rust_observe` (in `Rust.handler`). Tested by core/tests/run-overlay.sh mode `observe`.

### 11.15 Value calls: computations over a value the app builds

A `&self` method of a value node (not an element) that takes arguments besides `self` and returns an
owned value (`DockSizing::clamp(size)`, `size_from_pointer(point)`, `Sankey::layout(node_count,
links) -> Result<SankeyGraph, SankeyError>`, `topology`, `layout_from`, `Timing::sample(elapsed)`)
computes something from the value: it is an ambient call (11.5) whose first argument is the value,
which travels as a tape of its own (13.6) and is built in the call op by its constructor and
builders: `gk.base_dock_sizing_clamp(cx, gk.dock_sizing(.Left)->with_area_width(1000), 5000)`. The
Odin name is the type's and the method's (`base_` for the base crate). A method without arguments
reads back what the app wrote and stays skipped (builder-getter); one with a type parameter, a
context argument or a result that borrows the value is not a value call. Generator rule:
`gk_overlay.value_read` (a hook in `Model.classify_method`). The generated ambient test
calls every value call it can sample (core/tests/app/gen_ambient.odin); core/tests/run-overlay.sh mode
`values` checks results (a clamp, a pointer's size, a sankey's layers, a refused layout's error).

### 11.16 Skips, described menu ops, menu items as values, generic sidebars

- **Skips with a reason** (`gk_overlay.SKIPS5`, merged into `[skip]` so the model's own rule applies
  them and a key that matches nothing stops generation): a Rust plot's paint-time caches, paths, hover
  and appear hooks (PathCaches, PathCache, ShapeKey, polygon, origin_point, sankey_link_path, PlotHover,
  Plot::appear/hover/prepaint/tooltip/tooltip_state, PlotElement::new, ArcData::new, shape::Arc,
  TooltipState::new); objects handed only to closures skipped as item-render-closure
  (ComboboxTriggerContext, TimeFieldSegmentState, HoverCardState, CalendarItemState, base
  HoverCard::content, Notification::action); values upstream never takes (AnyChildElement,
  ListSeparatorItem, ContextMenuState, RenderOptions, TabGroupConstraints, PanelId::from_u64,
  SankeyNodeLayout); Rust's own typing or contexts (AnyDrag, Notification::id/id1, DockSkin::new,
  AutoScroll::set, PopupMenu::build, ContextMenu::new, CalendarState::set_disabled_matcher_shared,
  DialogFooterButton, a trait nothing implements); IndexPath's builders (value-helper: the record
  `gk.Base_Index_Path` is what calls take).
- **A described menu's op made without the menu's context**: `PopupMenu::submenu_with_icon` as
  `crate::views_menu::submenu_with_icon` (as `submenu` is, 11.6); a context op's `Option<Icon>` is
  the `Icon` its Into set makes from an `IconName` (`Maybe(gk.Icon_Name)` in Odin).
- **Menu items as values**: `PopupMenuItem` is an enum built like a builder (`[values] enum_nodes`,
  13.19), so `PopupMenu::item` takes one: `gk.popup_menu_item("Copy")->icon(.Copy)->action(a)`,
  `popup_menu_item_separator`, `_label`, `_element` (a fragment), `_link`, `checked`, `disabled`,
  `on_click`. `PopupMenuItem::submenu` takes a menu entity only `PopupMenu::build` makes (skipped).
- **Generic sidebars**: `SidebarGroup<SidebarMenu> as SidebarGroup` and
  `Sidebar<SidebarGroup<SidebarMenu>> as GroupedSidebar` (`gk.grouped_sidebar`) in `[instantiate]
  types`. Sidebar has two instances, so its conformance pages are named `component::Sidebar.*`
  and draw the grouped one; `gk.sidebar` is unchanged.
- **Bridged by the library's own code**: `SearchableListDelegate::on_confirm` keeps upstream's default
  in `GkSearchDelegate` (the same final selection reaches the app as the state's `Confirm` event, which
  upstream emits beside the call); `DockSkin::dock_area` (gk.dock_area_skinned) is kept by the pruning
  (its upstream arguments are on no wire).

### 11.17 Tooltip values in view closures

A closure that returns a view (`Fn(&mut Window, &mut App) -> AnyView`, `["viewany"]`, 13.18:
`StatefulInteractiveElement::tooltip`, `hoverable_tooltip`, `TextView::link_tooltip`) also takes the
kit's own Tooltip (`gpui_kit::component::tooltip::Tooltip`), which is a value node (`[node]
types`): its constructors `new(impl Into<Text>)` and `element(Fn(&mut Window, &mut App) -> E)` (a
fragment of the view) and its builders `key_binding(Option<Kbd>)`, `action(&dyn Action, Option<&str>)`
and the Styled ones are ops of the view's tape, decoded in the view build. Odin: the argument is
`gk.View_Or_Tooltip :: union {View, Kit_Tooltip}` (a `gk.View` converts implicitly, so existing calls
compile): `gk.button("save")->tooltip(gk.kit_tooltip_new("Save")->key_binding(gk.kbd(..)))`. Wire:
`u8 kind`, then `u64 view` (kind 0; 0 = an empty view) or `u32 node` (kind 1). Rust
(`crate::views_after::tooltip_view`): the node is kept with the view's tape (`Frag`) and built each
time upstream asks for the tooltip (the pointer rests on the element), then `Tooltip::build(window,
cx)`, as a Rust app does in its closure. `Tooltip::build` itself stays skipped (rust-only-extension).
Generator: `gk_comp6.viewany_tooltips` (the descriptor becomes `["viewany", ["node", key]]`, so
gates/accept.py sees the Tooltip taken). Gallery page `comp6-tooltip`.

### 11.18 Snapshots: an object copied out as the values of its reads

An object upstream hands a closure that cannot be copied (not Clone, so 13.4's temporary handle cannot
hold it) and whose public methods are all `&self` reads that take nothing (`ResizeHandleContext`: `axis`,
`edge`, `is_active`, `state`) is copied out where the closure is called as a **record of its reads**:
one field per read, named as the read, in the read's wire type (`gk.Resize_Handle_Context {axis,
edge: Maybe(Handle_Edge), is_active, state}`). The record is out only; its generated `Wire` impl calls
the reads (`gk_comp6.snap_field_put`), so the record is exactly what the reads return at the call.
The reads count bridged ("a field of the snapshot record"). Rule: `gk_comp6.snapshot`
(gk_values.resolved's hook, only while a closure's payload is read). A row of 9.16 compares the
record's bytes, so a row is described again when a read changes (the handle hovered, pressed,
dragged).

### 11.19 Optional element closures over a context

`Fn(values.., &mut Window, &mut App) -> Option<AnyElement>` (and `Option<impl IntoElement>`) is 9.16's
row source (`["ctxrows", .., "opt"]`): the closure returns `Some(the row)`; a row with no view yet is
drawn as nothing for one frame, as any row. Returning `None` (upstream's own drawing, where upstream
has one) is the kit's function of 11.20, not a row.

### 11.20 Closure types the kit also makes

A type alias of a closure (`ResizeHandleRenderer = Rc<dyn Fn(&ResizeHandleContext, &mut Window, &mut
App) -> Option<AnyElement>>`) that a kit function with no arguments returns (`resize_handle_appearance()`,
the kit's divider) is either the app's rows (9.16, 11.19) or that function: `["kitfn", alias, name,
[functions], rows]`, Odin `gk.Resize_Handle_Renderer :: union {Row_Source,
Resize_Handle_Renderer_Kit}` with `Resize_Handle_Renderer_Kit :: enum u16 {Resize_Handle_Appearance}`
(generated, gen_values.odin). Wire: `u8 kind` (0 rows: `u64 source`; 1 the kit's: `u16` function).
The library calls the function where the value is decoded, so it counts bridged. Binds
`ResizablePanelGroup::with_handle_appearance`, `ResizeHandle::with_appearance`,
`resize_handle_appearance` (`gk_comp6.kitfn`, gk_lists.alias's hook). The app's row procedure reads
`gk.row_context(cx, src, at, gk.Resize_Handle_Context)` (11.18) and draws the visible part of the
handle; the hit area, cursor and drag stay upstream's.

### 11.21 Sets of values; a calendar matcher's parts

- `Fn(&X) -> bool` with no window or app, X a plain value (a string, a number, a record, a chrono date),
  is the **set** of values for which it holds (`["valset", X, wrap, ptypes]`, Odin `[]X`): the closure
  answers whether its argument is in the set. Call-safe (no app context needed). In `Box<dyn Fn(&X) ->
  bool + Send + Sync>` the library's closure keeps the set's bytes (`crate::values::value_set`), so a
  value holding it reads back as the set (`impl Wire for Box<dyn Fn(&K) -> bool + ..>`). A variant of an
  enum with data whose field is such a set is buildable even when the enum was first met read back
  (`gk_comp6.set_variants`): `Matcher::Custom` is `gk.Matcher_Custom{v0 = []gk.Naive_Date}`.
- `Matcher::Interval(IntervalMatcher)` and `Range(RangeMatcher)`: their parts have private fields and
  no reads; they are library values both ways (`gk.Interval_Matcher {before, after: Maybe(Naive_Date)}`,
  `gk.Range_Matcher {from, to}`, rt_values.odin), made by the enum's own constructors
  (`Matcher::interval`, `range`) and destructured, and the library keeps what each was made of, keyed by
  its 8 bytes, to write it back (a matcher's days cannot be recovered through `Matcher::matched`: the
  days it leaves out may be a single day anywhere). With this every variant of Matcher is reachable both
  ways: `CalendarState::disabled_matcher`, `set_disabled_matcher`, `disabled_matcher_ref` and
  `DatePickerState::disabled_matcher` leave the partial-options table. chrono's NaiveDate is nameable in
  closure types (`gk_lists.THIRD_PARTY`).

### 11.22 Boxed values

`Box<T>` of a value node or element in a variant (`Text::TextView(Box<TextView>)`) is T, boxed where it
is decoded (`["boxed", T]`). The `Text::TextView` variant is reachable: `gk.Kit_Text_Text_View{v0 =
gk.text_view(..)}` for `Alert::new`, `info`, `success`, `warning`, `error`, `Checkbox::label`,
`Radio::label`, `Switch::label`, `SettingItem::description` and `DescriptionList::child`, `children`,
`item`, which leave the partial-options table. Not in a call op (a node there would need a value tape).

### 11.23 gpui's Background read back

`Background` as a result is the library value `gk.Background_Value {kind, color_space, solid: Hsla,
angle_or_height, stops: [2]Background_Stop{color: Hsla, percentage}}` (rt_values.odin; `impl Wire for
gpui_kit::Background` in core/src/values.rs: its fields are private, so the kind, colour space, angle
and percentages come from its serde form and the colours, at full precision, from its Debug form). This
binds `try_parse_background(text) -> gk.Fallible(gk.Background_Value)` (an ambient call, 11.5): a
colour (`"#ff0000"`) is a solid background, `"linear-gradient(90deg, #f00 0%, #00f 100%)"` a gradient.

### 11.24 Per-row menus described ahead of the right-click

`TableDelegate::context_menu(row, menu, ..) -> PopupMenu` and `Tree::context_menu(Fn(usize,
&TreeEntry, PopupMenu, ..) -> PopupMenu)` build a row's menu when the row is right-clicked. App code
does not run then, so the menu is described with the row, ahead of the click: the row (a table's
cell in any column; a tree's or a list's item) wraps its element in `gk.row_menu(el, gk.popup_menu()->
menu(..)..)`, library node `ROW_MENU 38` (`u32 child node, u32 menu node`), drawn as its child. The
library (`crate::shim_rows::row_menu`) finds the first of the row's views whose root is a ROW_MENU and
applies the menu node's ops to the menu upstream hands in (as 11.6's view-built menus are applied), so
the menu's handlers are the row view's own. A row with no view yet, or no menu, opens nothing
(upstream's empty menu). `GkTableDelegate::context_menu` counts bridged (`[shims]`); `Tree::context_menu`
takes the tree's row source (`[rows]`: `crate::shim_rows::tree_menu`, Odin
`gk.tree(..)->context_menu(rows)`). A tree's or list's row is a `gk.List_Item`: `gk.row_menu(item,
menu)` returns one (call it last). Gallery pages `comp6-table-menu`, `comp6-tree-menu`.

### 11.25 Rules for a selection change

`SearchableListDelegate::on_will_change(&mut self, selection: &mut Vec<..>, changes)` is called
synchronously while the list handles a click, with no context: the library's delegate
(`GkSearchDelegate`) applies upstream's default (each change in order) and then the rules the app gave
the source in advance: `gk.search_will_change(cx, src, gk.Search_Rules{min, max, locked, blocked})`
(call `SEARCH_RULES 164`, buffered: `u64 source, u32 min, u32 max, u32 n, str locked each, u32 n, str
blocked each`). `blocked` values are never added, `locked` values never taken out once selected, and a
change that would leave fewer than `min` or more than `max` values (0: no limit) is refused whole
(upstream's veto: the selection is left as it was). Counted bridged (`[shims]`). Test: mode
`will-change` of core/tests/lists2.

### 11.26 A questionnaire item's validator as rules

`QuestionnaireItemDefinition::with_validator(Fn(&QuestionnaireValidationContext) -> Result<(),
SharedString>)` is called while the questionnaire updates (an answer set, Next, Submit). It takes
`gk.Questionnaire_Rules {min_choices, max_choices, min_length, max_length, one_of, not_one_of, same_as,
differs_from, message}` (rt_values.odin, written by reflection; `["qrules"]`), checked by the library
(`crate::values::QuestionnaireRules`): limits of 0 are none, a length counts the free answer's
characters, `one_of`/`not_one_of` the free answer, `same_as`/`differs_from` name another item whose
answer this one must equal or differ from, `message` replaces the error (`{item}`: the item's name).
The library's check calls `QuestionnaireValidationContext::item`, `answer` and `answers`, which count
bridged. The read of a definition's validator (`QuestionnaireItemDefinition_Read::validator`) is
`Maybe(gk.Questionnaire_Validator)`: present when the item has one (a closure has nothing else to
read). Test: mode `q-rules` of core/tests/values.

### 11.27 Values and colours in a constructor's builder chain

`ColorPickerState::default_value(impl Into<Hsla>)`, `GridLayoutState::default_areas(GridArea..)`,
`TreeState::items(Vec<TreeItem>)` and `ScaleOrdinal<SharedString, Hsla>::range` and `unknown` are
builders of a constructor's chain whose argument (a colour, a value node) the chain's decoder (`sb_*`)
cannot decode: it has no app context. They take the rule of 9.31 (`gk_editor.chain_cx`
and `chain_body`: the constructor's call op decodes the whole chain first, with the call's context,
into closures `sbp_*`, a value node as a tape of its own (13.6), a colour against the theme, then
applies them to the new value), the one shared helper (see 9.38). Their states have a builder form:
`gk.color_picker_state(cx)->default_value(c)->build()`, `gk.tree_state(cx)->items({..})->build()`. Test: mode
`c6-chains` of core/tests/values (each value read back from the state or object its chain made).

### 11.28 Left unbound in the components

- `read: ScrollbarTheme::styles` (the read of a read-back ScrollbarTheme): skipped as builder-getter.
  ScrollbarStyles has six private styles and no public read; a copy would show the app nothing.

Wire numbers: node `ROW_MENU 38`, call `SEARCH_RULES 164` (the overlays block of
core/wire/ops.txt). Tests: core/tests/values (modes `c6-background`, `c6-matcher`, `c6-q-rules`, `c6-resize`,
`c6-chains`; core/tests/values/comp6.odin), core/tests/lists2 (mode `will-change`); misuse
examples/odin/negative_values (cases `c6-rules-source`, `c6-rules-limits`, `c6-tooltip-kept`); gallery
pages `comp6-*` (values_pages and chart_pages, .odin and .rs).

### 11.30 Requests that answer later; file dialogs, prompts, the menu bar, URLs

DESIGN.md section 11's "requests that answer later", built. Runtime: `core/src/platform.rs`,
`odin/gk/rt_platform.odin` (platform-neutral; nothing Linux-only crosses the C interface); numbers:
the end of `core/wire/ops.txt` (call 82-89, shape 22-23 of the app block); tests:
`core/tests/platform`, `core/tests/run-platform.sh`.

**Completions (general).** A request op takes a kept handler (`u32 slot, u32 generation`, as a timer
does) and returns at once a handle of kind `TASK`: the *completion*. The library awaits the answer on
the main thread (a foreground task over upstream's oneshot receiver or `Task`) and delivers it as one
`DONE 8` turn: the request bytes of an event (`u64 view 0, u32 generation, u32 slot, u16 shape`, the
payload), with the window the request was made in, never inside a draw. Its buffered writes are
applied when the turn returns (a `DONE` turn runs outside every update, section 3). The Odin runtime
runs the handler as for an event (the view that made the request is marked for redraw), then marks the
completion done, forgets the handler and sends `RELEASE` for it itself. Rules:

- the library never releases a completion: only the app's runtime does, after the `DONE` turn or when
  the app releases it. So an answer racing the app's release cannot make that release fail (test
  `race`: released and answered in the same turn; the answer arrives first, the handler, dropped with
  the release, does not run, and the release lands);
- `gk.release(cx, c)` before the answer cancels: the task is dropped (a platform dialog left open
  answers into nothing), a scripted request leaves the queue, a late answer is discarded (counted);
  after the answer it does nothing; a second release before the answer stops (`release-twice`);
- a completion started in a view's init or handlers goes with the view (`drop_kept_of`), as a timer
  does: releasing the view cancels it (test `view-released`);
- a view procedure may not start a request (`request-in-view`).

Any upstream call returning a oneshot receiver or a `Task` is bound this way: one call op that starts
it and a payload shape for its answer.

| call | wire | upstream |
|---|---|---|
| `PROMPT_PATHS 82` | `u8 files, u8 directories, u8 multiple, opt<str> prompt, slot, gen` -> `u64` | `App::prompt_for_paths(PathPromptOptions)` |
| `PROMPT_NEW_PATH 83` | `str directory, opt<str> suggested name, slot, gen` -> `u64` | `App::prompt_for_new_path` |
| `WINDOW_PROMPT 84` | `u64 window (0: the turn's), u8 level, str message, opt<str> detail, u32 n, (u8 kind, str label) each, slot, gen` -> `u64` | `Window::prompt` (the platform's dialog, else gpui's own) |
| `OPEN_URL 85` | `str url`, buffered | `App::open_url` |
| `SET_MENUS 86` | `u32 n, menus`, buffered (a menu: `str name, u8 disabled, u32 n, items`; an item: `u8 kind`: 0 separator, 1 action `str name, u32 action, u8 os action, u8 checked, u8 disabled`, 2 submenu, 3 the OS's Services menu `str name`) | `App::set_menus`, and the kit's `GlobalState::set_app_menus` so its `AppMenuBar` draws the same menus where the platform has no menu bar (Linux) |
| `MENUS_GET 87` | -> `u32 n, menus` | the menus as `AppMenuBar` draws them |
| `PLATFORM_ANSWER 88` | `u8 kind (1 paths, 2 new path, 3 prompt), u8 status, paths or u32 answer` -> `u8 taken` | scripted answers |
| `PLATFORM_LOG 89` | -> URLs opened, requests, delivered, discarded, waiting, scripted | scripted mode's record |

Shapes: `PATHS 22` (`Paths_Done {status: Done_Status, paths: []string, error: string}`; a save dialog
answers one path or none), `PROMPT 23` (`Prompt_Done {status, answer: u32}`, the button's index).
`Done_Status`: `.Answered`, `.Cancelled` (the user dismissed it), `.Failed` (the platform could not
show it; `error` says why).

**Scripted answers.** Under the offscreen driver (`gk_headless_run`, the diag library) or with
`GK_SCRIPTED_PLATFORM=1`, the platform is not asked: a request waits for `gk.platform_answer_paths` /
`gk.platform_answer_prompt` (oldest request of the kind first) or takes an answer given before it;
`open_url` is recorded (`gk.platform_log`). Otherwise `PLATFORM_ANSWER` is refused
(`GK_ERR_UNSUPPORTED`). The app's code is the same either way.

Odin:

```odin
gk.prompt_for_paths(cx, {files = true, multiple = true}, gk.on(opened))   // opened :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done)
gk.prompt_for_new_path(cx, dir, gk.on(save_to), "notes.txt")
gk.prompt(cx, .Warning, "Replace it?", {{label = "Replace"}, {.Cancel, "Keep"}}, gk.on(answered), detail = "It exists")
gk.set_menus(cx, {name = "File", items = {gk.Menu_Action{name = "Open", action = open}, gk.Menu_Separator{}}})
gk.open_url(cx, "https://example.com")
gk.release(cx, completion)                                                 // cancels while it waits
```

Tests (`core/tests/run-platform.sh`): `dialogs` (a dialog answered later, a save dialog answered
before it was asked, a prompt, a dismissed dialog: four DONE turns with their payloads, the view
redrawn), `cancel`, `race`, `view-released`, `menus` (set from app actions and read back, open_url
recorded), and the misuse modes `release-twice`, `request-in-view`, `wrong-payload`, `menu-no-action`.

### 11.31 Requests from a turn without a window; dialogs answered by the offscreen script

- **A request's window.** A request made in a turn that has no window (an app-wide action from the menu
  bar or a key binding: `gk.on_app_action`; an app timer) belongs to the app's active window, else its
  first (`platform::request_window`). Its `DONE` turn runs with that window, and `gk.prompt(cx, ..)` asks in
  it, so `gk.prompt` works from a menu item's action. A prompt for another window than the turn's, or from a turn without one, is shown once the turn is
  over (the dispatching window is in use while an app-wide action runs).
- **Frames.** The change a `DONE` turn's handler makes is drawn in the frame after its answer, as in a Rust
  app (11.32).
- **`answer WHAT` in a gk-meter script** (`core/meter/src/headless.rs`, `on_answer`): offscreen, the
  script answers the oldest waiting request of a kind, `answer paths P..`, `answer new-path P`, `answer
  prompt N`, `answer cancel paths|new-path|prompt` (then a frame). The library registers its scripted
  platform (`platform::answer_line`, the same queue as `PLATFORM_ANSWER`); a Rust twin registers its own
  (`ref/src/bin/files.rs`). So an app's dialogs are part of its pixel gate: `gates/pixels.sh files`
  (`examples/odin/files` against `ref-files`, `gates/files.script`: the kit's AppMenuBar and its menus,
  Open… answered with a file, Save as… with a new path, Quit's prompt answered Stay, Help > Website's
  open_url recorded, ctrl-o's dialog dismissed, and the status line's entrance animation).

### 11.32 The real platform; a DONE turn's frame; a right click

- **A `DONE` turn's change is in the frame after its answer, as in Rust.** `HeadlessAppContext::update`
  runs its closure without ending an app update, so the offscreen driver's `answer` line runs inside an update of the driven window
  (`core/meter/src/headless.rs`), as a platform answer arrives in an update of the app (the request's
  foreground task, `cx.update(|cx| cx.defer(deliver))`): the `DONE` turn runs when that update ends, and
  its change is described by the next pre-frame hook and drawn in that frame. No app code runs mid-frame.
- **The real platform (Linux, gpui-fast-linux 0.1.14; modes `real-*`, `GK_SCRIPTED_PLATFORM` unset).**
  `bin/platform-real` runs the test app on a real window inside `gates/display.sh` (a private display and
  a private D-Bus session bus with no service files, so no portal answers), with a fake `xdg-open` first
  on `PATH`.
  - `prompt_for_paths`, `prompt_for_new_path`: gpui asks the xdg-desktop-portal FileChooser through ashpd;
    with no portal on the bus the request fails at once and both `DONE` turns come with `.Failed` and
    gpui's reason (`Portal request failed: org.freedesktop.zbus.Error: The name
    org.freedesktop.portal.Desktop was not provided by any .service files`; where ashpd finds no portal
    implementation gpui says "Couldn't open file picker due to missing xdg-desktop-portal
    implementation."). A dialog the user dismisses answers `.Cancelled`. Nothing hangs (both in time).
  - `open_url`: gpui runs the `open` crate's openers on its background executor, `xdg-open` first (then
    gio, gnome-open, kde-open, wslview), and the OpenURI portal only if none succeeded. The fake opener got
    the URL as its one argument; nothing is recorded in `PLATFORM_LOG` (that is the scripted mode's).
  - `set_menus`: the Linux platform keeps the menus (gpui's `get_menus` returns them) and shows no menu
    bar; the kit's `AppMenuBar` draws them from its `GlobalState`: laid out in a real window's frame
    (more than 100 by 10 px) with the two menus set.
  Nothing Linux-only crosses the C interface: no op or shape was added.
- **A right click under the offscreen driver.** `rclick X Y` is a move, a press and a release, each in its
  own update of the window with pending tasks run between, as a real window delivers them (gpui's
  `on_input`: one `handle.update` per platform input). Each update's end runs what it deferred and
  gpui's test-support flush draws a window it dirtied, as a real window draws frames during a click.
  Note that `InputState::on_context_menu` (the base state's
  handler, upstream hands it an empty `NativeMenu`, the capabilities and the position) fires when the
  state is drawn by the base element `gk.base_input`, and never when it is drawn by the kit's `gk.input`,
  because gpui-component's `Input::render` installs its own handler on the state each time it draws (its
  Cut/Copy/Paste/Select All menu, or `Input::context_menu`'s). A handler set between two draws is
  replaced at the next, which comes between a press and its release. Rust behaves the same. The app's own
  menu on a kit input is `gk.input(s)->context_menu(menu)` (data).

Tests (`core/tests/run-platform.sh`): `real-dialogs`, `real-url`, `real-menus` (through
`bin/platform-real`), `context-menu` (gk.base_input: fired, items 0, at the click, editable, the cursor
moved) and `context-menu-kit` (gk.input: not fired, the cursor moved). `GK_PLATFORM_SCRIPT=FILE` runs the
test app under another script (shots of what a mode draws).

### 11.33 Stopping an event; shortcuts; refreshing everything

What writing `examples/odin/todo` needed.

- **`gk.stop_propagation(cx)`**, call op `STOP_PROPAGATION` (95, app block): immediate, with no
  arguments and no answer; the library calls GPUI's `App::stop_propagation`, so an element further
  out than the one whose listener runs gets no listener call for this event (a click on a checkbox in
  a list row does not also confirm the row). Called in a handler of a mouse or key event it does what
  `cx.stop_propagation()` does in Rust; in any other turn it does nothing. It is a sync call (the
  buffered writes before it are sent first), so it acts before the listener returns to GPUI.
- **`gk.shortcut(cx, keys, handler, when_in = "")`** (runtime only, rt_app.odin): `gk.action` with a
  name of the procedure's own (`gk::shortcut::<address>`), `gk.bind_keys` and `gk.on_app_action`
  (11.4) in one call. It returns the action, for menus. One action per procedure: a second call with
  the same procedure only binds more keys. Being app-wide, the listener hears the keys wherever the
  focus is (after a dialog closed with nothing focused, too); an element's `on_action` still takes the
  action first when it is on the focus path.
- **An app-wide action listener (`gk.on_app_action`, so `gk.shortcut`) runs with a window.** GPUI calls
  global action listeners inside the window's dispatch, where `&mut Window` is taken, so the handler ran
  in a turn without one. So the library queues the event (section 4's `fire_queued`): it runs when
  the dispatch has ended and before the next draw, in an update of the window of the view whose init
  registered the listener (the app's first window for one registered elsewhere), and its change is in
  the same frame.
- **A posted procedure runs with a window.** The library runs the `WAKE` turn (section 6) inside an
  update of the app's first window (the window a queued event without a view gets), so a call that needs
  a window, such as `gk.push_notification`, works in a procedure a thread posted. Before any window is
  open, the turn runs without one.
- **`gk.refresh(cx)`** (runtime only, rt_source.odin): marks every live view dirty, and every bound row
  of every row source (the same marks `gk.notify` and `gk.invalidate` make; nothing new crosses), after
  setting the count of each source made with a count procedure whose procedure now answers another
  number. `gk.list_source` is a procedure group: the count is an `int` or a `proc(s: ^S) -> int`. A
  view not drawn in a window is described when it is drawn next (5.2's rule), so a closed dialog's view
  is not described by a refresh.
- `gk.select_row(cx, list, row)` (kit_rows.odin) is `List_State::set_selected_index` with a row, or
  none for a negative row.
- Generated types: a tuple variant of a data enum whose one field is a record declares it `using v0`
  (gen/gk_odin.py), so its fields are the variant's (`e.row` of `List_Event_Select`). No wire change.

### 11.34 Icons, posted lists, early posts

What writing `examples/odin/reader` needed.

- **`[into] stand_in`** (api/overrides.toml, `Model.stand_in` in gen/gk_model.py): an `impl Into<X>`
  whose X has no public path in the extract (a type in a private module of the kit) is bound as `impl
  Into<Y>` for the public type Y named there, which converts into X upstream. One entry:
  `ButtonIcon = "gpui_kit::component::Icon"`, so `Button::icon` takes a `gk.Icon_Name` (op 276, decoded
  as `Icon::from(IconName)`, which is Into<ButtonIcon> through ButtonIconVariant). Library change: one
  decoder arm.
- **`Icon` is a node type** (`[node] types`): `gk.icon(name, flags..)` (node 887), `gk.icon_empty()`,
  `gk.icon_default()`, with the styled builders and `path`, `data`, `rotate`, `transform`; it is no
  not kit state (the call ops it once had keep their opcodes in the lock). Into sets: a set made of an element and the one enum it is made from keeps the enum alone
  (`Into<Icon>`: `gk.Icon_Name`), because Odin infers `.Folder` for an enum parameter and never for a
  union; a larger set keeps the element as a member (`Tab_Into` gained `Icon`). An `Option<Icon>`
  takes `Maybe(gk.Icon)` (`PopupMenu::submenu_with_icon`).
- **A posted message may hold slices and dynamic arrays** (rt_handler.odin `own(.., lists = true)`,
  rt_post.odin): `gk.post` copies them with their elements into the binding's memory, so the posting
  thread keeps (and frees) what it allocated; when the procedure runs, they are copied once more into
  the app's allocator (context.allocator of `gk.run`), with the strings, and are the procedure's.
  Pointers, maps, unions, `any` and procedures are still refused. Handler data (`gk.on(p, value)`)
  keeps section 4's rule (no slices).
- **The INIT turn runs only the posts made before it** (rt_turn.odin, rt_post.odin): the posts queued
  when the turn starts (made before `gk.run`, when no wake could be asked for) run after the start
  procedure as before; a post made while it runs (a thread started by a view's init that posts at once)
  waits for its WAKE turn, which has the app's first window (11.33). If the INIT turn leaves posts in
  the queue, it asks for a wake.
- `gk.select_row(cx, list, row, scroll = true)` also calls `List_State::scroll_to_item(row, Nearest)`;
  `gk.slider_number(e)` (kit_input.odin) is the number of a `Slider_Event` or `Slider_Value` (a range's
  start), upstream's `SliderValue::start`.
- Not bound yet: pushing a notification built with `gk.notification(cx)`. The way
  proposed is an Into set member made of a kit state's builder chain, decoded into a value where the
  kit takes one (`push_notification`, `NotificationList::push`).

### 11.35 GK_SCRIPT in gk.run; where a stop inside a builder points

- `gk.run(state, start)` reads `GK_SCRIPT` first: when it is set, the file it names is read and the
  app runs as `gk.run_headless(state, script, start)` would (the diagnostic library only; with the
  ship library the start fails with the library's message and a line saying to put the diagnostic
  `lib/` first). An unreadable script exits 1 before the library is entered.
- A stop inside a generated builder (an element taken, attached or changed out of turn, a negative
  `usize`, a length of the wrong kind, a kept value) has no caller location. It ends with the context
  the runtime holds (`element_origin`, rt_capi.odin): the line of the `gk.view` whose procedure was
  running, or the row a list's row procedure was drawing, and with `GK_DEBUG=1` the line that made
  the element: `Build.locs` records each node's constructor location while `GK_DEBUG` is set (nothing
  is recorded otherwise). `View_Rec.loc` is where `gk.view` made the view.
- A call the library refuses stops with its message and one line of advice by error kind
  (`stop_call`): a zero handle ("never made", `ERR_STALE_HANDLE` whose message says so), another
  released handle, no window (`ERR_NO_CONTEXT`), another thread, and for a bare refusal of an op or
  tape a package and library from different downloads. The error number is not printed.
- The library: a zero handle is "a zero handle (one that was never made)"; a view that shows one
  stops with advice to make it in init instead of the advice about releasing; the type a view shows
  is printed without module paths (`genapi::app_type_name`); a buffered write that fails is "a change
  a handler asked for could not be made".

### 11.36 A record as one field; the kit's style helpers as tokens; a window's title

- **A record taken as one field.** `[shorthand]` of `api/overrides.toml` maps a record to one of its
  integer fields (`"gpui_kit::base::IndexPath" = "row"`); the model checks the record is one an app
  writes and the field an integer, and the description carries it as `records[path].shorthand`. An
  emitter offers the field alone wherever a parameter is the record, an option of it or a list of it,
  and writes the record (the field set, the others zero), so the wire is the record's and the
  library is unchanged. Odin: `Index_Path :: union #no_nil {int, Base_Index_Path}`,
  `Maybe_Index_Path :: union {int, Base_Index_Path}` (nil: none), `Index_Path_List :: union {[]int,
  []Base_Index_Path}` (nil: empty), named after upstream's type (the name its builder node claimed is
  free when that node is never emitted); the parameter is converted to the record (`index_rec`) before
  the record's own writer runs. Reads and events still hand back `Base_Index_Path`. 11 calls take it
  (`List_State`'s `set_selected_index`, `scroll_to_item`, `set_right_clicked_index` and
  `set_item_to_measure_index`; `select_state` and `Select_State.set_selected_index`;
  `combobox_state`, `add_selected_index`, `remove_selected_index` and `set_selected_indices`;
  `Command_State.set_selected_index`).
- **Style helpers as tokens.** A nullary method (`fn m(self) -> Self`) of a trait in `[traits]
  via_style` other than `Styled`, with no extra bound, is also a style token: a second entry in the
  token table (`style` numbers 3133 to 3148 in `api/opcodes.lock`: `StyledExt`'s `debug_*`, `font_*`,
  `h_flex`, `v_flex`), decoded in `style_token` as `gpui_kit::base::StyledExt::font_bold(s)` on the
  same `W(style)` as gpui's (`core/src/gen/style.rs`). The method keeps its trait op, so
  `->font_bold()` still compiles; the description gives the method its token number (`token`).
  `ThemeStyled`'s methods take `cx` and stay methods. This is a library change: `API_HASH` moves.
- **A window's title.** `gk.open_window(cx, title, size: [2]f32, root)` opens through `WINDOW_OPEN 72`
  (`open_window_with` with the title and size set, the rest zero); `OPEN_WINDOW 2` stays in the
  library but the Odin package doesn't use it.

### 11.37 The Go front end: the same protocols

No change to the wire or the library. The Go package (`go/gk`, docs/GO.md) speaks every protocol of
this file through the same seven C functions; its emitter interface is `go/gk/rt_emit.go` (the Go
column of section 7). Differences in the binding only:

- **Handlers** are Go functions of the shape's type, stored in the view's handler table as values;
  the shape table (`shapes[n]`, generated in `gen_types.go`) decodes the payload with generated
  readers (no reflection) and calls the function. A kept handler that takes no event (`func(*gk.Cx)`)
  fits every shape, as Odin's does. An index in a payload (Rust `usize`) is a Go `int`.
- **Requests and replies** are Go byte slices lent for one C call; `gk_call`'s result is copied out
  at once; the `gk_app` and the offscreen script are C memory. The library keeps no Go pointer.
- **View-built closures** (11.6): `gk.NewBuilt(cx, render)` makes the view once; the generated call
  that opens the overlay marks it dirty, so it is described again before the kit builds it.
- **`ref-panels --script FILE`**: the Rust panels reference runs offscreen under a gk-meter script (the
  diag build), for the pixel comparison in `gates/go.sh` (`gates/panels.script`).

### 11.38 Asking before a window closes, the app's quit, reduced motion, a window's icon, gradients; the Go ports

What porting a real desktop app from Rust to Go needed from both front ends. These are wire additions
(`core/wire/ops.txt`, so the interface hash covers them); the C interface itself is unchanged.

**A window's close asked of the app** (`Window::on_window_should_close`). `ON_WINDOW_SHOULD_CLOSE 96`: `u64
window` (0: the turn's), a kept handler of shape `UNIT_BOOL` -> `u64 listener` (kind SUB). The library
installs one gpui callback per window the first time (`core/src/app.rs`); when the user closes the window
(its close button, Alt-F4, the taskbar, Cmd-W) it asks the app's handlers in the order given, each in a
synchronous EVENT turn with the window in scope, and the first that returns false keeps the window open. A
handler whose listener was released, or that returns nothing (a reply without a value), does not hold the
window. The app closes it later with `WINDOW_CLOSE`, which does not ask. Odin `gk.on_window_should_close(cx,
w, gk.on(ask))` with `ask :: proc(a: ^App, cx: ^gk.Cx) -> bool`; Go `gk.OnWindowShouldClose(cx, w,
func(cx *gk.Cx) bool)`; `gk.release` ends it. Offscreen there is no close button: `PLATFORM_TEST` 6 (`u64
window` -> `u8`) asks the handlers as the platform does, through the same function (`ask_should_close`).

**The app's quit** (`App::on_app_quit`). `ON_APP_QUIT 97`: a kept handler of shape `UNIT` -> `u64 listener`.
The library registers one quit observer at start; as the app shuts down (`gk.quit`, the last window closed,
the platform's quit) it fires each hook once with no window, before it lets go of the app's state. gpui
gives quit observers a moment (its shutdown timeout) and then exits: a hook starts what must outlive the app
(an updater, say) and returns. Offscreen the hook runs when the driver's context is dropped at
the end of the script, inside `gk.run_headless`. Odin `gk.on_app_quit(cx, gk.on(quitting))`, Go
`gk.OnAppQuit(cx, f)`.

**Reduced motion.** `REDUCE_MOTION 98` -> `u8` (immediate; a view may read it): `App::reduce_motion`, which
the kit's `apply_system_reduce_motion` (bound: `gk.base_apply_system_reduce_motion`) keeps in step with the
system's setting. `SET_REDUCE_MOTION 99`: `u8` (buffered): `App::set_reduce_motion`. Odin
`gk.reduce_motion(cx)`, `gk.set_reduce_motion(cx, on)`; Go `gk.ReduceMotion(cx)`, `gk.SetReduceMotion`.

**A window's icon and app id.** `WINDOW_OPEN 72` ends with `str app_id` and `u32 n, n bytes`, a PNG decoded
by the library (the `image` crate's PNG decoder, which gpui already links) into `WindowOptions::icon`; the
app id is `WindowOptions::app_id` (Wayland's app_id, X11's WM_CLASS: the desktop matches the window to the
app's desktop entry and its icon by it). Bytes that are not a PNG stop the call with the decoder's reason.
Odin `gk.Window_Config{.., app_id = "myapp", icon = png}`, Go `gk.WindowConfig{.., AppID: "myapp", Icon:
png}`. gpui shows the icon on X11 (and takes it on Windows); Wayland takes the desktop entry's.

**Gradients** (a `Fill`). `Styled::bg` takes `impl Into<Fill>`, which gpui's `Background` (a colour, a
linear gradient, a pattern) is; the binding gave it a colour only. Colour encoding `GRADIENT 2`: `f32 angle`
(degrees; 0 up, 90 left to right, clockwise), `u8` colour space (0 sRGB, 1 Oklab), then two stops, each a
colour (`HSLA` or `THEME`, so a theme colour in a gradient follows the theme) and `f32 percentage`.
`genapi::background` reads a colour or a gradient into a `Background` and serves every Fill the generator
meets (`gk_model.is_fill`: today `Styled::bg`, so every styled element and `StyleRefinement`, hover and
drag-over styles included) and the plot's paths and quads (12.17). The front ends type it so a gradient fits
only there: Odin `gk.Fill :: union #no_nil {Color, Linear_Gradient}` (a `gk.Color` converts by itself, so
`->bg(gk.theme(.Background))` compiles as before), made with `gk.linear_gradient(angle,
gk.linear_color_stop(c, 0), gk.linear_color_stop(d, 1), oklab = true)`; Go `gk.Fill`, an interface that
`gk.Color` and `gk.LinearGradient` satisfy: `gk.NewLinearGradient(angle, gk.NewLinearColorStop(c, 0),
gk.NewLinearColorStop(d, 1)).InOklab()`. A gradient given where only a colour goes does not compile (Go
misuse case `gradient_color`); should one reach `genapi::color` anyway, the library reads past it, draws its
first stop and says so.

**The Go ports of Odin's hand-written families** (no wire change; the Go emitter, `gen/gk_go.py`, and
`go/gk/rt_platform.go`, `rt_data.go`):

| family | Odin | Go |
|---|---|---|
| described plots (12.6, 12.14) | `gk.plot(parts..)`, `gk.plot_with(opts, parts..)`, `gk.Plot_Part` | `gk.Plot(parts...)`, `gk.PlotWith(gk.PlotOptions{ID: "x", NotInteractive: true, Appear: 2}, parts...)`; `gk.PlotPart`, generated in `gen_plot.go` from the same list as Odin's union |
| drag and drop (13.20) | `->on_drag(gk.Drag_Value{id, text}, preview)`, `->on_drop(gk.on(dropped))`, `->drag_over(style)`, `->can_drop(gk.Drop_Accept{..})` | `.OnDrag(gk.DragValue{ID: 7, Text: ".."}, preview)`, `.OnDrop(func(cx *gk.Cx, d gk.DragValue))`, `.DragOver(gk.NewStyleRefinementDefault().Bg(..))`, `.CanDrop(gk.DropAccept{IDs: []uint64{7}})` (descriptors `libval` `Drag_Value`, `dragview`, `dragstyle`, `dropaccept`) |
| an element's tooltip (11.17) | `->tooltip(view)` or `->tooltip(gk.kit_tooltip_new(..))` (`gk.View_Or_Tooltip`) | `.Tooltip(v)` with `gk.ViewOrTooltip`: a `gk.View`, `gk.NewKitTooltip(text)` or `gk.NewKitTooltipElement(el)` (descriptor `viewany`) |
| image sources (13.31) | `gk.Image_Bytes{format, bytes}`, `gk.Pixel_Image{width, height, rgba}` | `gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: b}}`, `gk.ImageSourceRender{Value: gk.PixelImage{..}}`, `gk.ImageSourceCustom`, `gk.ResourceURI`, `gk.ResourcePath` (descriptors `imagebytes`, `pixelimage`, `customimage`, `uri`, `arcpath`); a `gk.PixelImage` whose bytes are not width x height x 4 stops |
| the menu bar (11.30) | `gk.set_menus(cx, menus..)`, `gk.menus(cx)` | `gk.SetMenus(cx, gk.Menu{Name: .., Items: []gk.MenuItem{gk.MenuAction{..}, gk.MenuSeparator{}, gk.MenuSubmenu{..}, gk.MenuServices{..}}})`, `gk.Menus(cx)` |

Not ported to Go yet (listed in `gen/out/go-report.md`): `on_drag_move` (its `gk.Drag_Move_Event`),
`external_drag_payload`, image caches, scroll anchors, a prepaint order and synthetic accessibility children.
Tests: `core/tests/run-app.sh` modes `close-veto`, `quit-hook`, `reduce-motion`, `window-icon`, `bad-icon`
(Odin); `gates/go-proto.sh` runs the same five and `drag`, `menus`, `menu-no-action`, `images`, `bad-pixels`,
`tooltip`, `plot`, `fill-nil` of `examples/go/port` (Go); `gates/pixels.sh paint` (gradients and plot parts,
Rust against Odin and Go, 12.17); `gates/go-misuse.sh` cases `fill_token` and `gradient_color`.


## 12. Data tables, generic types and described plots

Upstream's charts, plot shapes and many
other types are generic over the type of the app's data (`LineChart<T, X, Y>`,
`Bar<T>`), and they take closures over a datum that upstream calls while it draws
(`x(|d| d.month.clone())`). App code never runs in the middle of a frame (decision 3), so the
binding instantiates each generic type once, for a row of a table of the app's own data that
the library keeps, and answers every such closure from that row. Code: `core/src/data.rs`,
`core/src/plot.rs`, `odin/gk/rt_data.odin`; generator rules in `gen/gk_model.py`
(`instantiate_many`, `data_closure`, `datum_arg`, `ret_kind`, `paints`), `gen/gk_rust.py`
(`data_closure`, `paint_part`), `gen/gk_odin.py` (`data`, `field`, `format`, `Plot_Part`).

### 12.1 Generic types instantiated for an argument list

`[instantiate] types` in `api/overrides.toml` takes `Base<A, B, ..> as Name` with any number of
arguments. An argument is a library type (`crate::...`), a primitive (`f64`, `usize`, ..), a
gpui value type (`SharedString`, `Hsla`, `Pixels`) or an upstream type by name. The line makes
one node type `Name` (`Line_Chart` in Odin) whose methods are every method of an impl that
holds for the arguments (an impl for concrete arguments only when they are the line's), `Self`
and every type parameter substituted. Constructors, builders and type-changing methods are then
classified exactly as for any node type. The one-argument forms of section 9.5 are unchanged.
Lines today: the seven charts, `LineChart<crate::data::Datum, SharedString,
f64>`, `AreaChart<..same..>`, `BarChart<..same..>`, `CandlestickChart<..same..>`,
`PieChart<crate::data::Datum>`, `RadarChart<crate::data::Datum, f64>`,
`SankeyChart<crate::data::Datum>`, and the four plot shapes `Line`, `Area`, `Bar`, `RadialLine`
over `crate::data::Datum` as `LineShape`, `AreaShape`, `BarShape`, `RadialLineShape`, and
`Sidebar<SidebarMenu>` (an element generic over its children's type). A one-argument line with
`as Name` and an upstream type takes this rule too (taking the first type of that name instead
would pick the wrong `Sidebar`, which is not the generic one). Labels
are `SharedString` and values `f64` because those are what an app's columns hold; upstream's
`PlotValue` is `f32`, `f64` (and `Decimal`, a feature the library does not build).

A parameter named only in the bounds of another parameter that is named (`fill<F, B>(f: F)
where F: Fn(&T, ..) -> B`) is chosen by the closure's return type and counts as named.

### 12.2 Rows of the app's data

`[instantiate] data` names the library types that stand for one row: `crate::data::Datum`. A
parameter that takes rows (`impl IntoIterator<Item = T>`, `Vec<T>`, `&[T]`) is the descriptor
`data`. In Odin it is a `gk.Data`, made by `gk.data(rows[:])` from a slice of any struct (or of
plain numbers: one column named `value`); each field is a column:

| Odin field | column kind | wire |
|---|---|---|
| any integer, `bool` (0 or 1), `f64` | `Num` 0 | f64 |
| `f32`, `f16` | `Num32` 4 | f32 (shown as text as an f32 prints) |
| `string`, an enum (by name) | `Text` 1 | str |
| `gk.Color` (a theme colour is resolved when the view is decoded), `gk.Hsla` | `Color` 2 | colour (section 2) |
| `Maybe` of a number | `Opt` 3 | u8 presence, f64 |

Wire: `u16 columns, (u8 kind, str name) each, u32 rows`, then the rows one after the other, each
column in its kind. A field of any other type stops at the `gk.data` line. The library keeps the
rows in columns (`data::Table`) and hands upstream one `Datum` per row (the table and the row's
index). The rows are copied into the tape each time the view is described, as a Rust app clones
its rows into the chart it builds in `render`.

### 12.3 Closures over a row: `field` and `format`

A closure parameter that gets no window or app, takes a row (`&T`, or a borrowed wrapper with a
public field `&T`, `&ArcData<T>` through `data`) and numbers, and returns text, a number or a
colour, is answered from the row (`data_closure`):

- **`field`**: a column. `Fn(&T) -> X` for any of those results, and a colour or number from a
  row with other arguments too (`tooltip_value_color(Fn(&T, usize, f64) -> H)`,
  `Bar::fill(Fn(&T, Bounds<f32>, BarAlignment) -> B)`), whose other arguments are not used: the
  app writes into the row what it would have computed from them. Odin: a `string`, the column's
  name (`->x("month")`); wire: str. Text from a number column is the number as upstream prints a
  raw value (`format!("{}")`); a number from an `Opt` column without a value is 0, and
  `Option<f32>` results get `None`.
- **`format`**: a template, for a closure that returns text from numbers it is handed
  (`y_tick_format(Fn(f64) -> S)`, `tooltip_value(Fn(&T, f64) -> SharedString)`, `Fn(&T, f32,
  f32)`): `{}` the next number, `{1}` the second, `{:.2}` two decimals, `{:,}` thousands
  separated by commas, `{month}` a column of the row (when the closure has one), `{{` and `}}`
  braces. Each number keeps its type (an f32 prints as an f32). Odin: a `string`; wire: str.

The return type is upstream's or the one its bound names (`S: Into<SharedString>` is
`SharedString`, `H: Into<Hsla>` is `Hsla`, `Bg: Into<Background>` is `Background`, `L:
Into<RadarLabel>` is `RadarLabel` through `From<SharedString>`); the generated Rust closure
names it, so upstream's own type parameter is inferred from it.

Checks, at the builder's line in Odin (`put_field`, `put_format`), when the node was given its
rows first (every chart: they are the constructor's argument): the column exists, its kind fits
(text from anything but a colour, a number from `Num`, `Num32` or `Opt`, a colour from `Color`),
a template's braces close, its numbers are ones the closure is handed and its columns exist.
Each stops with the column names of the row type. A plot shape given its rows after a field
builder is checked only by the library, which answers a missing column with an empty value.

### 12.4 Element closures over a row: a row source keyed by the row

`Fn(&T, &mut Window, &mut App) -> impl IntoElement` over a row (a chart's `tooltip_content`) is
the descriptor `datum_rows`: the app passes a `gk.Row_Source` (section 9.1) whose rows are the
data's rows, and the closure draws the source's view for the row's index (`data::row_element`):
the row is bound and described between frames like any row, and the first frame it is asked for
draws nothing in its place. The view drawing it is told when the row arrives.

### 12.5 Keys only hashed

`impl Hash` with no other bound (`appear_key`: draw in again when the key changes) is a string.

### 12.6 Described plots

A Rust app draws a custom plot by implementing the kit's `Plot` trait, whose `paint` calls the
`paint(&self, &Bounds<Pixels>, &mut Window[, &mut App])` of an axis, a grid and shapes. The
binding implements `Plot` once (`plot::GkPlot`) for a plot the app describes as a list of parts,
`gk.plot(parts: ..gk.Plot_Part)`, painted in that order in the plot's bounds (it fills its
parent). A part is any value type with such a `paint` (`paints` in the model): `Plot_Axis`,
`Grid`, `Line_Shape`, `Area_Shape`, `Bar_Shape`, `Radial_Line_Shape` today; the generator lists
them in `Plot_Part` (Odin) and `generated::paint_part` (Rust) and counts their `paint` as
bridged. The shapes' position closures (`Fn(&T) -> Option<f32>`) are fields holding pixels in
the plot's bounds: the app computes them, as upstream's `paint` computes them from its scales.

| number | what |
|---|---|
| `node PLOT 28` | `u32 n, u32 value node each`; no ops |
| reserved | call 128-143, shape 32-35, node 28-31 |

### 12.7 What generated code may use, beyond section 7

Rust: `crate::data::{rows, Field, Format, Arg, Datum, row_element}` and
`generated::{PaintFn, paint_part}` (written by the generator, used by `core/src/plot.rs`).
Odin: `Data`, `put_data`, `put_field`, `put_format`, `Data_Kind`, `put_plot_part` (generated).

### 12.8 Not bound, and why

- `SankeyChart::labels(Fn(&T, f64) -> Vec<SankeyLabel>)`: a closure that returns a list of
  builder values per row; a field cannot hold one. `Bar::label(Fn(&T, Point<Pixels>) ->
  Vec<Text>)` the same.
- `BarChart::fill_gradient`: the closure is handed `&dyn Fn(f32) -> f32`, a remap the app would
  call; no data closure can.
- `Pie<T>`, `Stack<T>`, `ArcData`, `Arc::paint`, the scales (`ScaleLinear`, `ScaleBand`,
  `ScalePoint`, `ScaleOrdinal`): calculators an app calls from its `paint` and whose results it
  reads (`arcs`, `series`, `tick`); they need a value behind a handle that the app can call
  between frames (the protocol of section 13) and an instantiation from 12.1.
- Upstream's `PlotHover`, `PlotAppear`, `PlotLabel`, `TooltipState` and the other `Plot`
  methods: `GkPlot` keeps upstream's defaults (no id, so no hover and no appear motion).

### 12.9 Tests

- Gallery pages, each byte-identical to a Rust page that writes the same chart with its own row
  types and closures (`ref/src/bin/gallery/chart_pages.rs`, `examples/odin/gallery/chart_pages.odin`):
  `chart-line`, `chart-line-more`, `chart-area`, `chart-bar`, `chart-candlestick`, `chart-pie`,
  `chart-pie-fn`, `chart-radar`, `chart-sankey`, the hover pages `chart-tooltip-line`,
  `chart-tooltip-bar` (the tooltip content over a row source) and `chart-tooltip-pie`,
  `plot-custom` (a `Plot` impl against `gk.plot` with an axis, a grid and the four shapes), and
  `generic-sidebar` (`Sidebar<SidebarMenu>`).
- The generated sample (`core/tests/gensample`) calls every chart and shape builder with rows of
  its own (`Sample_Row`: a text, a number, a gap and a colour column).
- Misuse cases (`examples/odin/negative_data`, in `gates/misuse.sh`): a column the rows do not
  have, a column of the wrong kind, a template that asks for a number the closure is not handed,
  a template with an unclosed brace, a field of a type a data table cannot carry.

### 12.14 Plot options: the Plot trait answered from data

`gk.plot_with(opts, parts..)` is `gk.plot` with the answers the library's `GkPlot` gives the kit's
`Plot` trait (core/src/plot.rs): `id` (`Plot::id`: keeps the plot's state across frames and opts in
to appear motion and hover), `not_interactive` (`Plot::interactive`), `appear` (`Plot::appear_generation`,
0 none). Node `PLOT 28` gets them after its parts: u8 has id, the id; u8 interactive; u64 appear
generation. `gk.plot(parts..)` sends the trait's defaults. The trait's `prepaint`, `hover`,
`appear`, `tooltip_state` and `tooltip` stay not attempted: upstream calls them while it lays out
and paints the frame, and app code never runs mid-frame (decision 3); answering them from data
would need a description of the tooltip and of the hover targets, which no part has yet.

### 12.15 The pie and stack layouts over the app's rows

`Pie<T>` and `Stack<T>` are calculators a Rust app calls from its own plot code. They are library
objects instantiated by `[values] objects` (13.8) for a row of a data table (12.2), `Pie<Datum>` and
`Stack<Datum>`, and their closures over a row are answered from the row (12.3):

- Rows and row closures in a call op: a `gk.Data` argument (`Pie::arcs(&[T])`, `Stack::data`) and
  `field` / `format` closures are call-safe (gk_overlay.view_built). In a builder chain of an
  object's constructor, which has no app context, the rows are read without one (`data::rows_plain`:
  a theme colour in them is transparent). The Odin checks of a column (`put_field`) run in a view only.
- A field closure handed a `&str` key (`Stack::value(Fn(&T, &str) -> Option<f32>)`) reads the column
  the key names when the field is `""` (`Datum::keyed`); a named field reads that column for every key.
- Results that hold rows come back as library values with each row as its index in the rows the app
  passed: `[]gk.Arc_Data {row, index, value, start_angle, end_angle, pad_angle}` (`Pie::arcs`, whose
  `'a` borrows the argument, not the object: `gk_overlay.borrows_arg`) and `[]gk.Stack_Series {key,
  index, points: []gk.Stack_Point {y0, y1, row}}` (`Stack::series`). Wire impls in `core/src/data.rs`.
- `Stack::keys<I, S>` is bound now that a parameter named in the where clause of a named one counts as
  named in the object and state classifiers too (`gk_overlay.named_text`), as it did for node methods.

```odin
arcs := gk.pie(cx)->value("visits")->build()->arcs(cx, gk.data(rows[:]))   // one Arc_Data per row with a value
st := gk.stack(cx)->data(gk.data(rows[:]))->keys({"apples", "pears"})->value("")->build()
```

Not bound: `ArcData::new` (an ArcData borrows its row; the app has `Arc_Data` values from `arcs`), so
`shape::Arc::paint` stays without an accepter. Tests: modes `pie`, `stack` of `core/tests/charts`.

### 12.16 Lists per row: closures over a row that return a list

A closure over a row of a data table (12.2) with no window or app that returns a list or a fixed array
of values the binding builds, not a number, text or a colour that 12.3's `field` answers:
`SankeyChart::labels(Fn(&T, f64) -> Vec<SankeyLabel>)`, `Bar::label(Fn(&T, Point<Pixels>) ->
Vec<Text>)`, `BarChart::fill_gradient(Fn(&T, RangeInclusive<f32>, &dyn Fn(f32) -> f32) ->
[LinearColorStop; 2])`. The app gives the list for each row in advance, row i's at index i (Odin
`[][]gk.Sankey_Label`, `[][]gk.Text`, `[][2]gk.Linear_Color_Stop`; `["rowvals", out, wrap, ptypes,
accessor, at]`), decoded where the chart is (value nodes in the view's tape, colours against the
theme); the closure answers with a clone of the row's list (`crate::plot::row_value`; past the end, the
type's default). The closure's other arguments (the node's throughput, the bar's position, the value
range and its remap) are not used: the app writes into each row's list what it would have computed
from them. Rules: `gk_comp6.rowvals` (gk_values.closure's hook) and, for a closure whose other
parameters name a type no closure rule can (`&dyn Fn(f32) -> f32`), `gk_comp6.bounded` (gk_lists'
bounded hook), whose generated closure names that parameter's type itself. SankeyLabel is accepted now
(gates/accept.py). Gallery pages `comp6-sankey-labels`, `comp6-bar-labels`, `comp6-bar-gradient`.

### 12.17 The binding's own plot parts: paths and quads

A Rust plot's `paint` calls the kit's parts and also gpui's own drawing: `PathBuilder::stroke(px(w))` or
`PathBuilder::fill()`, `move_to`, `line_to`, `close`, `window.paint_path(path, background)`, and
`window.paint_quad(quad(bounds, radius, background, border, border_color, BorderStyle::Solid))`, with a
colour or a gradient (gpui's `Background`). An app that draws curves, filled areas or glowing lines in a
gradient needs these, because the kit's shapes cannot take a gradient (their strokes are an `Hsla`). Two value nodes of the binding's own
are plot parts beside the kit's (`Plot_Part` in Odin and `PlotPart` in Go list them with the generated
ones; `core/src/plot.rs` `part` decodes them before `generated::paint_part`):

| node | wire | Odin | Go |
|---|---|---|---|
| `PLOT_PATH 29` | `u8 kind` (0 a stroked polyline, 1 a filled polygon, closed), `f32 width`, `u32 n`, `(f32 x, f32 y)` each, a fill (11.38) | `gk.plot_line(points, width, fill)`, `gk.plot_area(points, fill)` -> `gk.Plot_Path` | `gk.NewPlotLine(points, width, fill)`, `gk.NewPlotArea(points, fill)` -> `gk.PlotPath` |
| `PLOT_QUAD 30` | `f32 x, y, w, h, f32 corner radius`, a fill, `f32 border width`, a colour | `gk.plot_quad({x, y, w, h}, fill, gk.Quad_Style{..})` -> `gk.Plot_Quad` | `gk.NewPlotQuad(x, y, w, h, fill, gk.QuadStyle{..})` -> `gk.PlotQuad` |

Points are `[][2]f32` (Go `[][2]float32`), pixels from the plot's top left; the library adds the plot's
`bounds.origin`, as a Rust paint does. A path of fewer than two points paints nothing. The colours (theme
colours too) are resolved when the view's tape is decoded, so a theme switch needs no new description.
The parts paint in the order given, interleaved with the kit's (a grid under an area under a line). Test:
`gates/pixels.sh paint` draws `ref-paint` (a `Plot` impl that paints a grid, an area, three gradient
strokes and two quads), `examples/odin/paint` and `examples/go/paint` under `gates/paint.script` and
compares the images byte for byte; mode `plot` of `examples/go/port` (options, theme colours, a one-point
line).

## 13. Library objects behind handles, and values made outside a view

Two general protocols that the generator binds whole families
through: kit values that are not entities, held by the library behind a handle (13.1 to 13.5),
and builder values made in a handler or in init, given to a call op (13.6). Runtime:
`core/src/objects.rs`, `odin/gk/rt_handles.odin`; generator: `classify_object`,
`object_method`, `reach_objects`, `call_desc` in `gen/gk_model.py`, `obj_calls` in
`gen/gk_rust.py`, `globals_statics` in `gen/gk_odin.py`. Tests: `core/tests/handles`
(`run-handles.sh`), and every object type with a constructor is made, read, written and
called by the generated `states` test (`core/tests/gen`).

### 13.1 What an object is

A struct of the kit crates that is not an entity, not an element or builder (a node type),
not a plain record and not generic, has no lifetime parameter, and has reads or calls
(`&self`, `&mut self`) or is a kit global (`impl Global`) (`Model.is_obj`). Examples:
`DialogHandle`, `VirtualListScrollHandle`, `SearchMatcher`, `DisplayMap`, `DiagnosticSet`,
`PaneTree`, `TreeEntry`, `InputPresentation`, `Theme` and `ThemeRegistry` (globals). In Odin an
object type is `struct {using state: State, using vt: ^T_VT}`, like kit state: `x->len(cx)`.

### 13.2 Handles and lifetimes

| how the app gets one | lifetime | example |
|---|---|---|
| a constructor (`new`, any static returning `Self`), with its `self -> Self` chain as builder ops, one crossing | **kept**: until `gk.release` | `gk.dialog_handle(cx, false)`, `gk.tooltip_defaults(cx)->with_show_delay(d)->build()` |
| a read, a call or an event that hands one out (by value, or a copy of a borrowed one) | **temporary**: until the next frame begins (the pre-frame hook sweeps it), after the buffered writes of the turn that got it; `gk.retain(cx, x)` keeps it | `list->scroll_handle(cx)`, `input->presentation(cx)`, `tree->entry(cx, i)` |
| a kit global's accessor (`fn global(cx) -> &Self`, `global_mut`) | one interned handle per type, reads and updates the global in place (`cx.global`, `cx.update_global`); never temporary | `gk.theme_global(cx)` |

A temporary handle used after its frame stops with the library's message ("... was released
(a library object a read, a call or an event hands out is temporary ...)") and the app's line,
exit 2. An object does not notify: a view that shows an object's value is described again when
something marks it (a handler marks its own view), not when the object changes.

### 13.3 Calls

| upstream method | kind | wire | when |
|---|---|---|---|
| `fn new(..) -> Self` and other statics returning `Self` | `obj-ctor` | call op -> u64 handle | at once (flushes writes first); not in a view procedure |
| `fn x(self, ..) -> Self` | `obj-builder` | a builder op of the constructor | in the constructor's crossing |
| `fn global(cx: &App) -> &Self` | `obj-global` | call op -> u64 handle | at once; allowed in a view procedure |
| `&self` -> value, no `&mut` context | `obj-read` | call op u64 handle, args -> result | at once; allowed in a view procedure |
| `&mut self`, or `&self` with `&mut App`/`&mut Window`, or `self`, returning `()` | `obj-write` | call op, buffered | with the turn's writes |
| the same returning a value | `obj-call` | call op -> result | at once; not in a view procedure |
| a static returning `()` / a value | `obj-static` / `obj-scall` | call op without a handle | buffered / at once |

A method that needs the window gets the turn's, or the app's first open window for a write
applied after its turn (`genapi::with_window`); none open is `GK_ERR_NO_CONTEXT`. An owned
object is an `Rc<RefCell<T>>` cloned out of the handle table before kit code runs; a method that
re-enters the binding and touches the same object meets a borrowed cell: `GK_ERR_BUSY`, never a
panic. `self` by value: a copy when the type is Clone (the handle stays good), else the object
itself, and the handle is released.

### 13.4 Objects as arguments and results

- `&T` in a call op is borrowed in place (`genapi::obj_rc`), `&mut T` likewise (`refmut`); `T`
  by value is a copy (Clone) or given away. In an element's tape an object argument is copied
  out of its handle each time the tape is decoded, so only a Clone type can be one there
  (`Dialog::handle(DialogHandle)`), and a released handle there stops (`genapi::obj_shown`).
- Not yet: an object inside a record, an enum variant or an `Into` set (their codecs have no
  call context), and `Option<&Object>`.
- A result by value is moved into a new temporary handle (`genapi::Owned`); a borrowed one, or
  one inside an option or a list, is copied, so its type must be Clone. Which kit types are
  Clone is not in the reduced extract (it drops std-trait impls): `api/clone.json`, read from the
  pinned sources by `tools/clone_scan.py` (a wrong entry would be refused by rustc). A result
  that borrows from its object (`PaneRef<'_>`) is not bound.
- In Odin a value of an object type read by reflection (a result inside a `Maybe` or a slice, an
  event payload) gets its method table from `register_obj` (`rt_handles.odin`, hooked into
  `rd_value`).
- Unreachable objects: an object type that no bound constructor, global accessor, result or
  event hands out is not emitted, and functions that take one are `not yet` with the reason
  "protocol: object (no constructor, global or bound function hands one out)" (`reach_objects`).
  Most of these are the contexts the kit passes to render closures and delegates
  (`TabGroupContext`, `InlineTokenContext`, `DockContext`, `ResizeHandleContext`): they become
  reachable with the dock and closure protocols, without a change here.

### 13.5 Wire

| number | what |
|---|---|
| `hkind OBJ 8` | a library object |
| `call OBJ_RETAIN 112` | `u64 object` (immediate): a temporary handle becomes kept |
| `call OBJ_TEST_TEMPS 113` | tests: -> `u32` temporary handles waiting, `u32` live objects |
| `call TABLE_COLUMN_VALUES 114` | `u64 source, u64 table state` (0: the state about to be made over the source), `u32 n`, n values as tapes (13.6): a table's columns as the kit's own `Column` values; immediate for 0, buffered otherwise |
| generated call ops | numbered from `api/opcodes.lock` like state call ops: `u64 handle` (none for globals and statics), the arguments; results as in section 2, an object as `u64` |

Reserved: call 112-127, shape 28-31, node 24-27.

### 13.6 Builder values made outside a view

A builder value that is not an element (a grid area, a tree item, an editor decoration, a diff
file) could only be made inside a view build, so no call op could take one: the review counted
at least 172 usable functions building values nothing accepted. Now `new_value` outside a view
build puts the value into the turn's **value build** (`vbuild`, `rt_handles.odin`), and a call
op argument of a value type is written by `put_vtape`: `u32 length`, then a tape of its own
(header as section 6 of DESIGN.md, root = the value, no handlers, dependencies or slots). The
library decodes it with the value's generated decoder (`genapi::value_arg`, with the turn's or
the first window). A value lives for the outermost turn that made it; one kept longer stops
where it is used ("a value can only be used in the turn that made it"). A value made in a view
procedure cannot be given to a call (a view may not write anyway). A handler inside a value
made outside a view stops at `gk.on`'s registration, as before.

A builder type that is also a plain record with public fields (`IndexPath`) travels as the
record instead (`call_desc`): `list->set_selected_index(cx, gk.Base_Index_Path{row = 120})`.
This is what makes a list selectable and scrollable from code (`set_selected_index`,
`scroll_to_item`, `scroll_handle()->scroll_to_item`).

A table's columns can be the kit's own `Column` values (its 18 builders and `new`), which until
now nothing took (the table delegate's shim takes `gk.Table_Column` records):
`gk.table_columns(cx, src, {gk.column("size", "Size")->width(60)->text_right()}, t = {})`, before
the table state is made or replacing the columns of state `t` (`kit_rows.odin`,
`shim_rows::source_columns` and `state_columns`).

### 13.7 What generated code may use (added to section 7)

Rust (`genapi`, re-exported from `objects.rs`): `obj_new`, `obj_temp`, `obj_global`,
`obj_rc`, `obj_clone`, `obj_take`, `obj_shown`, `obj_by_value`, `obj_read`, `obj_read_global`,
`obj_update`, `obj_update_global`, `obj_update_w`, `obj_update_w_global`, `with_window`,
`value_arg`, `Owned`. Odin: `obj_get` (a read that records no dependency), `register_obj`,
`put_vtape`, and `value_new`, `value_op`, `value_take`, `value_args` behind `new_value`, `op`,
`take_value` and `node_args`.

### 13.8 Generic objects instantiated for an argument list

Code: `gen/gk_values.py` (hooked into `gen/gk_model.py`,
`gk_rust.py`, `gk_odin.py` and `gkgen.py` at one line each), `core/src/values.rs`,
`odin/gk/rt_values.odin`; numbers: the values block of `core/wire/ops.txt` (call 208-239, shape
52-59, node 48-55, hkind 13); tests: `core/tests/values` (`run-values.sh`), the gallery's
`values-*` pages, `examples/odin/negative_values`.

A generic kit type an app *calls* rather than describes (a calculator: the plot scales, the
motion values) is a library object (13.1) once its parameters are fixed. `[values] objects` in
`api/overrides.toml` takes `Base<A, B, ..> as Name` (arguments as in 12.1: a primitive, a gpui
value type, an upstream type by name, a library type); each line is one object type `Name`
whose methods are those of every inherent impl generic over the parameters, classified as in
13.3 with `Self` and the parameters substituted. A generic trait the type implements for its own
parameter (`impl<T> Scale<T> for ScaleBand<T>`) gives its methods to the instance, called
through the trait (`<ScaleBand<SharedString> as Scale<SharedString>>::tick`); the trait's row
counts them as bridged. A method of one instance may hand out another (`Sequence::sample` gives
a `SequenceSample`): every line's type is known before any is classified. A static that makes
its own type fallibly (`Keyframes::try_new -> Result<Self, E>`) makes the object reachable.

Lines today: `ScaleBand<SharedString>`, `ScalePoint<SharedString>`, `ScaleLinear<f64>`,
`ScaleOrdinal<SharedString, Hsla>`, and over `f32` `Sequence`, `SequenceStep`, `SequenceSample`,
`Keyframes`, `Discrete`. Odin: `gk.scale_band(cx, {"a", "b"}, {0, 300})->padding_inner(0.1)->build()`,
`band->tick(cx, "a")` (a `Maybe(f32)`), `gk.keyframes_try_new(cx, frames)` (a `gk.Fallible`).

### 13.9 Results read back: Result<T, E>, maps

| upstream result | descriptor | wire | Odin |
|---|---|---|---|
| `Result<T, E>`, T not `()` | `["fallible", T]` | u8 ok, opt<T>, str error (its Debug form) | `gk.Fallible(T)`: `ok`, `value: Maybe(T)`, `err` |
| `Result<(), E>` | `["result", "unit"]` (section 11.6) | opt<str> | `Maybe(Temp_String)`, the error |
| `HashMap<K, V>` | `["list", ["tuple", [K, V]], "map"]` | u32 n, (K, V) each, in the order of their bytes | `[]Tuple_K_V` |

`crate::values::Fallible` writes the first; a `Result<Self, E>` of an object moves the value
into a new handle (`.map(genapi::Owned::new)`). The error's text is its Debug form because some
kit errors (`DiscreteError`) have no Display. A map's order is fixed by sorting the encoded
pairs, so the same map always reads the same.

### 13.10 Arguments: fixed arrays, Option<&mut Window>, Rc and Arc of a kit struct

- `[T; N]` (N up to 16) is `["array", T, N]`: N values of T both ways; Odin `[N]T`
  (`range: [2]f32` of the scales).
- `Option<&mut Window>` (`Theme::change(mode, Option<&mut Window>, cx)`) is `["ctx",
  "window_opt"]`: the turn's window, or the app's first one for a write applied after its turn.
- `Rc<X>` / `Arc<X>` of a non-generic kit struct is a library object of its own, named `X_Rc` /
  `X_Arc` in Odin (`Theme_Config_Rc`, `Theme_Color_Arc`), always copied (a clone of the pointer):
  handed out by results (`ThemeRegistry::default_light_theme`, `ThemeColor::dark()`), taken by
  reference (`Theme::apply_config(&Rc<ThemeConfig>)`), and made from a value the app built
  (`gk.theme_config_rc(cx, gk.theme_config_default())`, a synthetic `Rc::new`, kept).
- A static function of a node type that returns a value or nothing (`ThemeColor::light()`,
  `Img::extensions()`, `Popup::resolved_corner`) is an ambient call (11.5) of that type.

### 13.11 Values the package accepted nowhere (item e)

A value type (a node type that is not an element, with a usable constructor or builder) is
*accepted* when a usable function takes it at any depth (through records, enums with data and
Into sets), a usable conversion returns it, or a runtime procedure takes it. `gates/accept.py`
counts the rest after every `make gen` and fails `--check` when one has no stated reason (its
`REASONS` table). Rules added for it:

- **Kit action values** (`gk.cancel_default()`, `gk.close_panel_default()`, every type that
  implements Action): `gk.action_of(cx, value) -> gk.Action` (call op `VALUE_ACTION 208`: the
  value's tape, decoded by the generated `value_action` dispatcher, boxed, entered in the action
  table of 11.1, its id returned). The same value always gives the same id. Not in a view.
- **Generic From impls in Into sets**: `impl<T: IconNamed> From<T> for Icon` makes the kit's own
  implementors of the trait members (`impl Into<Icon>` takes every `IconName`): 10 functions.
- **Self-generic builder traits**: `trait StyleSized<T: Styled>`, implemented for every `T:
  Styled`, is a builder trait bound as `E: StyleSized<E> + Styled`: 13 functions.
- **Public fields of a kit global** (the two `Theme`s): a read and a write each, synthetic items
  (not upstream functions, not counted in the coverage table): `th->font_size(cx)`,
  `th->set_font_size(cx, 17)`, `th->set_list(cx, gk.list_settings_default()->..)`,
  `th->colors(cx)`. A write runs with the turn's writes and redraws every window
  (`cx.refresh_windows()`); like any write it may not run in a view procedure.

Count (gates/accept.py, the same definition before and after): 138 types with 251 usable
functions at 23f6cb1 -> 53 types with 147 functions, every one with a stated reason.

### 13.12 Objects inside records, variants and Into sets; Option<&Object>; pinned temporaries

- A record, a variant of an enum with data or an Into set an app writes may hold a library
  object (by value or by reference, of a Clone type): it is copied out of its handle where the
  value is decoded (a call op's codec, a builder chain, a tape). A record read back keeps the
  older reading (its own fields).
- `Option<&Object>` in a call op is lent in place (`a.as_ref().map(|x| x.borrow()).as_deref()`).
- **Temporary handles placed in a view**: an object argument of an element written by a view
  procedure goes through `put_obj` (rt_values.odin), which pins the handle to that view's build
  (call op `OBJ_PIN 209`: u64 view, u32 build number, u64 handle; immediate, allowed in a view).
  A pinned temporary is not swept when the next frame begins: it lives while a build of
  the view names it, so a redraw of the same tape frames later (a theme change redraws every
  window without describing the view again) finds it. The handles a view's earlier build pinned
  are released at the first sweep after a newer build pinned its own. A kept handle or a global
  is left alone. `gk.test_value_pins` (call `VALUES_TEST 210`) counts both for the tests.
- **A value given to its own builder** (`t->child(t)`) stops at once, in a view build (as before)
  and now in the value build of a handler too ("a value was given as an argument to its own
  builder"); the library would otherwise decode the value inside itself until its stack ran out.

Not bound here, and why: `Theme::update(cx, FnOnce(&mut Theme) -> R)` (a closure over the
theme; the field writes above do the same), `ThemeRegistry::watch_dir` (a kept handler in a call
op: no call op carries a handler yet), `Sequence::with_step(T, Transition)` and
`ScaleOrdinal::range/unknown` (a node or a colour in a builder chain: `sb_*` has no Call),
`SequenceStep::transition` (a node read back), `Pie<T>`, `Stack<T>`, `ArcData`, `Arc::paint`
(they borrow the rows: `arcs(&self, &'a [T]) -> Vec<ArcData<'a, T>>`), `SettingField<T>` (its
value closures are called while the settings page draws: app code never runs mid-frame), the
motion free functions (`transition<T>`, `spring<T>`, `animate_keyframes<T>`: T chosen by the
caller).

### 13.13 Calendar values, durations read back

chrono's `NaiveDate`, `NaiveTime`, `NaiveDateTime` and `Weekday` are plain records both ways
(`["chrono", Name]`, gen/gk_values.py): Odin `gk.Naive_Date{year, month, day}`,
`gk.Naive_Time{hour, minute, second, nanosecond}`, `gk.Naive_Date_Time{date, time}`,
`gk.Weekday` (Mon = 0); wire: i32 year, u32 month, u32 day; u32 hour, minute, second, nanosecond;
a weekday a u8. Odin's writers (`put_naive_date` and the rest, rt_values.odin) stop on a date or
time that does not exist ("the date 2026-02-30 does not exist"); the library's `Unwire` impls
(core/src/values.rs) take what a raw C client sends anyway as the first day of the era or
midnight, never a panic. This makes upstream's `Date` and `DateTime` enums buildable: a calendar's,
a date picker's and a time field's dates and times are written and read
(`cal->set_date(cx, gk.Date_Single{gk.Naive_Date{2024, 2, 29}})`, `cal->days(cx)`,
`picker->date_time(cx)`, `field->time(cx)`, `gk.date_range_preset_date_time`). core/Cargo.toml
names chrono at the version gpui-base already links (no new crate).

A `std::time::Duration` read back (a result or a payload) is i64 nanoseconds, Odin's
`time.Duration` (it was written as milliseconds in a u32 only).

### 13.14 gpui's ScrollHandle as a library object; a generic bounded by a kit trait

`gpui::ScrollHandle` is a library object (`gk.Scroll_Handle`, made by `gk.scroll_handle(cx)`, a
kept handle, always copied: it is an Rc inside, and every copy names the same scroll position).
Taken by every `&ScrollHandle` argument (`div->id("l")->overflow_y_scroll()->track_scroll(h)`,
`TabBar::track_scroll`, `AttachmentGroup::track_scroll`) and, as the one implementor the binding
holds, by a generic parameter bounded by the kit trait `ScrollbarHandle` (`Scrollbar::new<H>`,
`ScrollableElement::scrollbar<H>`, `ScrollBounce::new<H>`; `GPUI_OBJECTS` in gen/gk_values.py,
the rule in gk_app.bounded). The trait's methods (`offset`, `set_offset`, `content_size`,
`viewport_bounds`, `start_drag`, `end_drag`) are the object's reads and writes, called through the
trait; the trait's row counts them as bridged. gpui's own methods of ScrollHandle are not in the
coverage table and not bound. A released handle a view still tracks stops where it is drawn.

### 13.15 Kit values read back with their reads; objects inside a state's events

A kit value type the app builds (a node that is not an element) and that has no public fields
cannot travel out as a record. Read back (`QuestionnaireState::answer -> Option<QuestionnaireAnswer>`,
`item_definition -> Option<&QuestionnaireItemDefinition>`), it is a copy behind a temporary handle,
a library object of its own named `X_Read` (`gk.Questionnaire_Answer_Read`), whose methods are
the type's `&self` reads (`answer->choices(cx)`, `def->choices(cx)` handing out more read-back
values). Only a Clone type can be one. The reads are synthetic items: upstream's functions keep
their row (a read of a builder the app is writing, skipped).

An object inside a state's event (`QuestionnaireEvent::AnswerChanged(QuestionnaireAnswerChange)`,
`Submit(QuestionnaireSubmission)`) is handed out by that event: the reachability of objects now
follows enums with data, records and Into sets (`gk_values.walk_tables`), so these objects and
their reads are bound.

### 13.16 Kit state a read hands out

A read, a call or an event that hands out `Entity<S>` (`HslaSliders::hue -> &Entity<SliderState>`,
`NotificationList::notifications`) gives the app a state handle of S, one per entity: the same
entity read twice gives the same handle (core/src/values.rs `entity_handle`). It is kept like a
handle the app made (gk.release frees it; a later read makes a new one); its writes run with the
window of the call that makes them. Only for state types the binding holds (a constructor of S is
bound); the others stay not attempted. Odin reads one by reflection: the generated
`gen_values.odin` registers those state types.

### 13.17 Ambient calls

- A trait implemented for App (`ActiveTheme`) gives its `&self` methods as ambient reads:
  `gk.kit_theme(cx)` is `cx.theme()`, a copy of the theme (a temporary handle).
- `impl Deref<Target = str>` read back is its text (`gk.locale(cx)`).
- `box_shadow` stays not attempted: its BoxShadow read back would need an Hsla where Odin's record
  holds a gk.Color (what an app writes).
- `init` of each crate is skipped: the library runs it at start, and a second call registers
  everything twice. Rust's reflection, profiling and plugin points are skipped with their reason in
  `api/overrides.toml` (`measure`, `AnySettingField::type_id`, `Root::register_plugin`, ..).

### 13.18 Closures that return a view

`Fn(&mut Window, &mut App) -> AnyView` (`StatefulInteractiveElement::tooltip`, `hoverable_tooltip`,
`TextView::link_tooltip`, `TooltipRequest::new`) is `["viewany"]` (gk_app.view_closure): the app
passes a `gk.View`, described like any child view, and the closure hands it back each time upstream
asks (when the pointer rests on the element); a zero View draws an empty one. Element decoders only
(a call op has no view build to mount the view in): `DEC_ONLY` in gen/gk_values.py.

### 13.19 Value cells and settings fields

A closure upstream calls **while it draws** to read a value, `Fn(&App) -> T` (a settings field's
`value`, its `is_dirty` test), cannot call the app (decision 3). It is a **value cell** instead
(`["cell", T]`, gen/gk_values.py `closure`): a library object (kind OBJ, kept until `gk.release`)
holding the value's bytes in its wire type. The generated closure decodes the cell's current bytes
each time upstream calls it; the app writes the cell between frames.

| call | wire | when |
|---|---|---|
| `CELL_NEW 211` | `u32 n, bytes` -> `u64 cell` | immediate; not in a view procedure |
| `CELL_SET 212` | `u64 cell, u32 n, bytes` | buffered (with the turn's writes); every window redraws |
| `CELL_GET 213` | `u64 cell` -> `u32 n, bytes` | immediate; allowed in a view procedure |

Odin (`rt_values.odin`): `gk.Cell(T) :: struct {h: u64}`; `c := gk.cell(cx, false)`,
`gk.cell_set(cx, c, true)`, `gk.cell_get(cx, c)`. T is the type the closure returns (`bool`,
`string` for SharedString, `f64`); the bytes are written by `put_value` and read by the generated
decoder, so both sides follow section 2. A released cell named in a tape stops where it is drawn.

**Settings fields.** `SettingField<T>` is instantiated three times (`[instantiate] types`:
`SettingFieldBool`, `SettingFieldText`, `SettingFieldNumber`): a builder instantiated more than once
is classified per instance (`gk_values.multi_inst`, one coverage row per upstream function, bound
when an instance binds it). A field's `value` is a cell, its `set_value` an ordinary handler
(`Fn(bool, &mut App)` is shape BOOL through `fire_app`, `SharedString` STR, `f64` F64), so the field
draws the cell and the app's handler writes it:

```odin
dark := gk.cell(cx, false)                                   // in init
set_dark :: proc(s: ^App, cx: ^gk.Cx, v: bool) {gk.cell_set(cx, s.dark, v)}
gk.setting_item("Dark", gk.setting_field_bool_switch(dark, gk.on(set_dark))->default_value(false))
```

`on_reset(is_dirty, reset)` takes a cell and a handler. `SettingField::render`, `element` and
`SettingItem::render` take a **fragment** of the view (an `Element`); their `&RenderOptions` payload
is not passed on (`FRAGMENT_PAYLOADS`): the app wrote the options it would read. A generic parameter
bounded by `SettingFieldElement` is that fragment closure. `SettingItem` is an enum built by
constructors and builders: a value node (`[values] enum_nodes`). `SettingItem::new(title, F:
AnySettingField)` takes one of the field instances (`["traitval", trait, node keys]`: Odin
`gk.Any_Setting_Field`, a union generated in gen_values.odin; Rust
`generated::traitval_any_setting_field` builds the node and wraps it in `crate::values::AnyField`,
which implements AnySettingField by handing every method to the field, `as_any` and `type_id`
included, so upstream's downcasts see the field itself). The trait's methods upstream calls
(`field_type`, `style`, `is_resettable`, `reset`) and `SettingFieldElement::render_field` count as
bridged (`[shims]`). `Settings::sidebar_size_range` takes `Range<Pixels>` (`["rangeof", px]`, two
f32).

Tests: modes `val-cells` (made, read, written, read in the next turn, released) of
`core/tests/values`; gallery page `values-settings` (a switch, a checkbox, an input, a dropdown, a
number input and a rendered item over cells, against a Rust app keeping the same values).

### 13.20 Drag and drop, parameters the binding fixes, colours read back

**Parameters fixed by rule** (`gk_values.fn_ctx`, hooked into `Model.fn_ctx`):

- gpui's drag methods are generic over the dragged value (`on_drag<T, W>`, `on_drop<T>`,
  `drag_over<S>`, `on_drag_move<T>`, the kit's `resize_handle<T, E>`): T is the library's
  `crate::values::Drag` (Odin `gk.Drag_Value{id: u64, text: string}`, a plain value both ways,
  `[lists] values`), the view type W or E the library's view (`crate::rt::GkView`). The app passes
  the value and a `gk.View` for the preview: `div->id("row")->on_drag(gk.Drag_Value{7, "row 7"},
  preview_view)`. `on_drop` is a handler whose payload is the value; `drag_over` takes a style
  (`gk.style_refinement_default()->bg(..)`), applied while a dragged value is over the element
  (`["dragstyle", style node]`: the described style refined over upstream's, never app code while
  drawing); the preview closure hands back the app's view (`["dragview"]`, an empty library view when
  the app gave none). `gk_app.bound_only` treats the fixed parameters as not chosen by the caller.
- a parameter bounded only by `Into<X>` and derive traits (`L: Into<DefiniteLength> + Clone +
  Default + Debug + PartialEq` of `StyledExt::margins`, `paddings`) is X; a length is Pixels, the type
  the wrapper's `From` impls are for (`Edges<Pixels>: From<f32>`).

**Wrappers for one element type** (`VALUES_INSTANCES`, as for ContextMenu<Div>):
`Scrollable<Div>` (`div->overflow_y_scrollbar()` and the x and both forms, then the wrapper's own
`id`, `scroll_bounce`), with ScrollableElement entered in Div's tables (`VALUES_BLANKETS`: its impl
for gpui's Div is in the kit crate). `ResizeHandle<Drag, GkView>` is instantiated
(`[instantiate] types`; a generic type given an instantiated argument list is that node,
`gk_values.resolved`), and `ResizablePanelGroup`, an element that also implements EventEmitter, is a
node (`[node] types`), so `h_resizable`, `v_resizable` (base and component) and the group's `child`,
`children`, `size`, `axis`, `with_state` are element builders.

**Toolbar children** (`Toolbar::child(impl Sizable + IntoElement)`): one of the element nodes
implementing Sizable, through the union rule (`[lists] unions`, `"Sizable" =
"Sizable_Element"`); the union is generated in gen_values.odin (`GENERATED_UNIONS`).

**Colours read back in a record the app also writes** (`box_shadow -> BoxShadow`): the record's
Odin field is a `gk.Color`, so the library writes that layout (u16 theme 0, then h, s, l, a:
`crate::values::put_color_field`, `gk_values.rust_field_put`); a record only read keeps Hsla. The
ambient rule accepts such a result (`gk_values.ret_readable`). `gk.base_box_shadow(cx, 4, 6, 12, 2,
gk.hsla(..))` may run in a view procedure (it returns a value) and its result goes to `shadow()`.

**One table of handed-out entities.** Entities read back by value reads (13.16, `Wire for Entity<T>`)
and by the dock (`crate::dock::entity_handle`, which also holds the panels and dock areas the app made)
share one table: `values::entity_handle` hands every entity to the dock's table, so an entity read back
through both paths gets one handle. Test: call op `HANDLES_TEST 214`
(an entity read back through both paths and again: one distinct handle), mode `val-one-handle`.

Skipped with their reason: `AxisExt::is_horizontal`, `is_vertical` (value helpers on gpui's Axis,
`skip:value-helper`: compare with `.Horizontal`). Not attempted, and why: `FocusTrapElement::focus_trap`
(returns `FocusTrapContainer<Self>`, which has no public path), `ElementExt::text_selection_scope`
(returns an opaque `impl IntoElement`), `group_drag_over<S>` (S named by no argument: the callee
needs a turbofish the emitters do not write), `can_drop` and `external_drag_payload` (closures over
`&dyn Any` and a gpui payload called while dragging), `on_drag_move` (`DragMoveEvent<T>` has no
encodable fields), `ResizeHandleContext` and `with_appearance` (a renderer closure upstream calls while
it draws, with a context object).

Tests: modes `val-drag` (a drag from a source onto a target runs the drop handler with the value),
`val-box-shadow`, `val-one-handle` of `core/tests/values`; gallery pages `values-drag`,
`values-shadow`, `values-resizable`, `values-scrollbar-div`, `values-toolbar`.

### 13.21 The window's own state, focus handles and aliased states read back, the kit's title bar

- **The window's own Root.** `Root::read(window, cx) -> &Root` hands out the state handle of the
  kit's Root the library makes around the app's view in every window (`WINDOW_ROOTS` in
  gen/gk_values.py; Rust `crate::values::window_root::<Root>`, an ambient call): Odin
  `gk.root_read(cx) -> Maybe(gk.Root)`, nil when the window's root is not one. Its reads follow
  (`root->view(cx)`, the app's root view back as a `gk.View`). `Root::update` (a closure given the
  Root by `&mut`) and `Theme::update` (the theme by `&mut`) are skipped as Rust-only: the app changes
  them through the state's calls and the theme's fields.
- **Focus handles read back** (`QuestionnaireState::focus_handle`, `item_focus_handle`,
  `choice_focus_handle`): a handle of kind FOCUS (`Wire for FocusHandle` in core/src/values.rs), kept
  until `gk.release` like one the app made with `gk.focus_handle`; each read makes a new handle.
- **Kit state an alias names** (`ColorPickerState::hex_input -> &Entity<InputState>`, where
  `InputState = InputBaseState<InputMode>` is an `[instantiate] states` line): the state handle of
  13.16, whatever order the generator met the types in.
- **The kit's title bar.** `TitleBar::window_options()` is what `gk.Window_Config{kit_title_bar =
  true}` opens the window with (flag 32 of `WINDOW_OPEN`, section 11.3: the kit's titlebar, its
  decorations and title-bar drag over the meter's bounds; the app's title is kept). Counted bridged
  (`RUNTIME_ITEMS`). Test: the second window of `core/tests/app` mode `windows`.

### 13.30 Animations and motion values; accessibility roles; type-changing wrappers

Runtime: `core/src/motion.rs`, `odin/gk/rt_motion.odin`; generator: `gen/gk_values.py`
(`animatable`, `motion_wrappers`, `MOTION_ITEMS`, `access_role`); node `ANIMATED 48`.

App code never runs while a frame is drawn (DESIGN.md decision 3), and upstream's animators are
closures it calls while it lays out. So an animation is **described as data** and the library
evaluates it while it draws: no turn per frame, no crossing per frame. A moving value asks gpui for
the next frame itself, which re-renders only the view that drew it (its tape is decoded again).

**Type-changing wrappers, by rule.** `with_animation` returns `AnimationElement<E>`, `focus_trap`
`FocusTrapContainer<E>`, `EffectTransition::apply` `AnimationElement<E>`: types the tape cannot name.
The generator writes one arm per element type (`generated::motion_wrap`, `motion_trap`: the element
node's constructors, then the wrapper, then `into_any_element`) and the Odin unions
`Animatable_Element` (every styled element node) and `Focus_Trap_Element` (parents that are a gpui
`Element`); the Odin procedures take any of them (`where intrinsics.type_is_variant_of`) and return an
`Element`, which `child()` takes. The same rule serves any wrapper whose result only becomes a child.

Node `ANIMATED`: `u32 child node, u8 kind`, then

- 1 **animations** (gpui's `with_animations`; `with_animation` is a chain of one): `str id, u32 n, n x
  (u32 ms, u8 flags (1 repeat, 2 repeat synced), easing, f32 max fps)`, `u32 n, n x track (u16
  animation (0xffff every one), u8 property, value from, value to)`. The animator sets each track's
  property to `from + (to - from) * delta` (colours: the kit's `Lerp for Hsla`). An easing is `u8`:
  0 linear, 1 quadratic, 2 ease in-out, 3 ease-out quint, 8 bounce of another (gpui's), 4 ease-out
  cubic, 5 ease-in cubic, 6 ease in-out cubic, 7 cubic Bézier `f32 x1, y1, x2, y2` (the kit's
  `base::animation`), 9 pulsating between `f32 min, max` (gpui's);
- 2 **effect**: `str id, u32 node of the kit's EffectTransition` (`EffectTransition::apply`);
- 3 **motion values**: `u32 n, n x (u8 property, u8 source, ...)`, each sampled while drawing:
  1 `transition(str id, target, u32 Transition)`, 2 `spring(str id, f32 target, u32 Spring)`,
  3 `animate_keyframes(str id, u64 Keyframes<f32>, u32 Timing)`, 4 `Presence::sample(u32 Presence,
  value at 0, value at 1)` (its progress between two values), 5 `TransitionState<f32>::sample(u64
  state, f32 target, u32 Transition)`;
- 4 **focus trap**: `str id, u64 focus handle` (`FocusTrapElement::focus_trap`).

Properties (`gk.Motion_Prop`): opacity, top, left, right, bottom, width, height, size (numbers, pixels)
and bg, text colour, border colour (colours: a value is a `gk.Color`, theme colours resolved as the tape
is decoded). The Odin package refuses at the app's line a colour for a number, a number for a colour, a
spring or keyframes on a colour, a bounce of a bounce, an animation of 0 ms, a track's step past the
chain.

```odin
gk.with_animation(gk.div()->relative()->size(gk.px(40))->bg(c), "slide", {ms = 400, ease = {kind = .Ease_In_Out}},
    {prop = .Left, from = f32(0), to = f32(160)}, {prop = .Opacity, from = f32(0.2), to = f32(1)})
gk.effect(el, gk.effect_transition(300 * time.Millisecond)->fade(0, 1)->slide_x(-20, 0), "enter")
gk.motion(el, {.Height, gk.To_Transition{"h", f32(a.open ? 120 : 40), gk.transition(300 * time.Millisecond)}},
              {.Left, gk.To_Spring{"x", f32(a.open ? 120 : 0), gk.spring(250 * time.Millisecond)->with_damping(0.6)}})
gk.focus_trap(gk.div()->child(..), "trap", a.focus)
```

Counted bridged where the runtime calls upstream (`MOTION_ITEMS`): `transition`, `spring`,
`animate_keyframes`, `Presence::sample`, `TransitionState::sample` (also a call of its object, made by
`Default`), `EffectTransition::apply`, `base::animation::cubic_bezier`, `FocusTrapElement::focus_trap`.
So `Presence`, `Timing`, `EffectTransition` and `TransitionState<f32>` are accepted. Not bound:
`transition_with_status` (its status would have to reach app code while drawing; `transition` gives the
value).

**Accessibility roles.** accesskit's `Role` (gpui re-exports it, `RoleOverride::Role` holds one) is a
third-party enum outside the API extract: its 182 variants are read from the crate source Cargo.lock
pins (`api/accesskit-role.json`, with the source's sha256) and it is an ordinary enum both ways
(`gk.Role`), so every `role()` takes every role: `gk.Role_Override_Role{.Button}`.

Tests: the gallery pages `motion-*` (motion_pages.odin against ref/src/bin/gallery/motion_pages.rs,
fixed-time frames: the offscreen clock moves only on `advance`), byte-identical to Rust.

### 13.31 gpui's element leftovers: closures given as data, objects the library makes, image sources as bytes

Runtime: `core/src/platform.rs`, `core/src/source.rs` (`virtual_list`), `core/src/motion.rs`
(kind 5); Odin: `odin/gk/rt_platform.odin`, `rt_source.odin`, `rt_motion.odin`; generator: `gen/gk_app.py`
(hooked from gk_model `closure_desc`, gk_values `resolved` and `runtime_items`, gk_rust
`dec`, the handler payload and trait ops, gk_odin `otype` and `put`); wire: the end of `core/wire/ops.txt`
(call 90-94, node 16 of the app block); tests: `core/tests/platform` modes `elements`, `drag`, `vlist`,
`cache-in-view`, `scope-zero`; gallery pages `elem-drop`, `elem-image`, `elem-scope`, `elem-vlist`
(`motion_pages.odin` against `motion_pages.rs`).

**Closures upstream calls while it draws or drags become data the app gives beforehand** (decision 3).
The rule, general: a closure whose answer depends only on what the app knew when it described the element
is that answer, decoded once and handed back each time upstream asks. Bound this way, by signature
(`gk_app.platform_closure`):

| closure | descriptor | Odin | what the library answers |
|---|---|---|---|
| `Fn(&dyn Any, &mut Window, &mut App) -> bool` (`can_drop`) | `dropaccept` | `gk.Drop_Accept{values, ids, paths}` | true for the library's drag value (every one, or those whose id is in `ids`), and for files dragged in from outside when `paths` |
| `Fn(&Drag, &mut Window, &mut App) -> Option<ExternalDragPayload>` (`external_drag_payload`) | `dragpayload` | `gk.External_Drag_Payload{files = {{path, directory}}}` | `Files(FileDragPaths)` of the files, none when empty |
| `Fn(&mut Window, &mut App) -> SmallVec<[usize; N]>` (`with_dynamic_prepaint_order`) | `prepaintorder` | `[]int` | the list |
| `FnOnce(&mut A11ySubtreeBuilder)` (`a11y_synthetic_children`) | `a11ychildren` | `[]gk.A11y_Child{key, role, label}` | one synthetic leaf per child, its id from the key (`synthetic_node_id`), `accesskit::Node::new(role)` with the label |

**The drag protocol, completed** (13.20's Drag is the one dragged type): `group_drag_over<S>` and
`external_drag_payload<T>` have their parameter fixed to `crate::values::Drag` (`gk_values.DRAG_FNS`); a fixed
parameter no argument names is written as a turbofish (`gk_app.turbofish`). `on_drag_move<T>`'s
`DragMoveEvent<Drag>` is an event leaf (`EVENT_LEAF`) whose payload the library copies before the handler fires
(`platform::DragMove::of`: `drag(cx)` borrows the app): `gk.Drag_Move_Event{using event: Mouse_Event, bounds,
value: Drag_Value}`. The dock's `DragPanel` is held as a **library object, not an entity** (`[state] objects`
in overrides.toml: a Render type that is a value passed and returned by value; base renders it as Empty), so
`TabGroupContext::drag_panel` hands out a temporary `gk.Drag_Panel`, `drop_panel` takes one by value, and its
`&self` setters `set_drag_offset` / `set_preview_size` are object writes (Rc<Cell> inside: every copy sees them).
The dock renderers of 11.9 get it from the TabGroupContext they are handed (11.9).

**Objects the library makes** (call ops of the app block; each a kept object handle until `gk.release`):

| call | Odin | upstream | taken by |
|---|---|---|---|
| `IMAGE_CACHE_NEW 90` | `gk.image_cache(cx) -> gk.Image_Cache` | `RetainAllImageCache::new` | `Img::image_cache(&Entity<I: ImageCache>)` (`imagecacheref`), `Div::image_cache(impl ImageCacheProvider)` (`imagecache`: the closed set `gk.Image_Cache_Provider = union{gk.Image_Cache, gk.Retain_All_Images{id}}`, the latter gpui's `retain_all(id)`) |
| `SCROLL_ANCHOR_NEW 91` | `gk.scroll_anchor(cx, scroll_handle) -> gk.Scroll_Anchor` | `ScrollAnchor::for_handle` | `anchor_scroll(Option<ScrollAnchor>)` (`scrollanchor`) |
| `SCROLL_ANCHOR_TO 92` | `gk.scroll_anchor_to(cx, anchor)` | `ScrollAnchor::scroll_to` (immediate, the turn's window) | |
| `TEXT_SCOPE_NEW 93` | `gk.text_selection_scope_id(cx) -> gk.Text_Selection_Scope` | `TextSelectionScopeId::new` | `gk.text_selection_scope(el, scope)` |
| `PLATFORM_TEST 94` | `gk.test_drop_accept`, `test_drag_payload`, `test_prepaint_order`, `test_pixel_image` | the decoders above run on the package's bytes | tests only |

A view procedure may not make one (`cache-in-view`). `ElementExt::text_selection_scope` (a type-changing
wrapper over a ParentElement) is node `ANIMATED` kind 5 (`u64 scope`), one generated arm per parent element
type (`generated::motion_scope`, the rule of 13.30), Odin over `gk.Focus_Trap_Element`.

**Virtual lists** (node `VIRTUAL_LIST 16`: `str id, u64 source, u8 axis, u32 n, (f32 w, f32 h) each`, then
style ops): the rows protocol of section 9 with a size per row. `gk.v_virtual_list(id, src, sizes)` (each row
`sizes[i].y` high) and `gk.h_virtual_list` (`sizes[i].x` wide) call upstream's `v_virtual_list` /
`h_virtual_list` with the drawing view's own entity; the closure upstream calls while it lays out hands back
each row's view, or a gap of the row's size for a row without one yet (asked for before the next frame, so app
code never runs mid-frame; one frame of gaps after a far jump). Sizes past the source's count are dropped.

**Image sources as bytes and pixels** (`gk_app.resolved`): every variant of `ImageSource` and `Resource` is
built, so `img`, `Avatar::src`, `AvatarImage::new` and `AttachmentMedia::src` take all of them:
`Resource::Uri` (`uri`: a string), `Resource::Path` (`arcpath`), `ImageSource::Image` (`imagebytes`:
`gk.Image_Bytes{format, bytes}`, gpui's `Image::from_bytes`), `ImageSource::Render` (`pixelimage`:
`gk.Pixel_Image{width, height, rgba}`, turned into gpui's BGRA frame) and `ImageSource::Custom` (`customimage`:
the loader upstream calls while it draws is data: the same `gk.Pixel_Image`, handed back each time).
`FileDropEvent::Entered`'s `ExternalPaths` is read back as `[]string` (`Wire for ExternalPaths`).

**Menus.** `GlobalState::set_app_menus` and `GlobalState::app_menus` are bound through the runtime's
`gk.set_menus` / `gk.menus` (calls 86, 87 of 11.30, which call them with every `OwnedMenuItem` variant:
separator, action with its OS action and flags, submenu, the OS's Services menu); the generated calls that
reached only two variants are left out (`gk_app.PLATFORM_REPLACES`).

**Skipped with their upstream reason** (`builder-getter`): `SequenceStep<f32>::transition` and the reads
`PlotMotion::pointer`, `enter`, `exit`, `appear` (also of a PlotMotion read back): Transition and Spring have
no public reads (their getters are `pub(crate)`), so the value read back is the one the app gave.

## 14. Conventions the generator and the runtime share

Rules that don't belong to one section but that every emitter and the runtime follow.

**Numbers** (`api/opcodes.lock`, append-only). Node types start at 256, one per constructor. A
type's own element ops start at 256 and are numbered per type, so an op number only means
something for the node type it is applied to. Builder-trait ops are
`0x4000 + (trait number << 7) + index`, the same number for every type that implements the
trait. Generated shapes start at 64; the built-in shapes 1 to 10 are used as section 4 lists
them (an `f32` payload gets a generated shape rather than being widened to `F64 10`). Style
tokens, enum variants, theme colours and palette colours each count from their own start.

**The runtime's four element ops** (section 1) are decoded by generated arms, not by a
function of the runtime: every node type that implements `ParentElement` decodes `CHILD`,
`CHILD_VIEW` and `TEXT`, and every `Styled` type decodes `STYLE` through the generated
`style_tokens`.

**Integers and enums.** Integers are as wide as upstream declares them (`usize` and `isize`
are 8 bytes, Odin `int`). The generator gives every enum without data a `u16` (Odin
`enum u16`), numbered from `opcodes.lock`. An enum with data that only the app builds is a
union of one `<T>_Kind` enum (the variants without data) and one struct per variant with
data, written by a generated `put_<T>`. One that an event carries has the form of section 2,
one struct per variant in number order, with gaps filled by `<T>_Reserved_n`. An event enum
with a single variant (`ColorPickerEvent`, `CarouselEvent`) is a struct
`{variant: u16, using value: <the variant>}` in Odin, because the variant number is on the
wire.

**`Wire` impls.** Generated code implements `genapi::Wire` for every record and enum an event
carries. The runtime implements it for exactly `u8 u16 u32 u64 i8 i16 i32 i64 usize isize
f32 f64 bool () str String SharedString Pixels Hsla`, `&T`, `Option<T>`, `[T]`, `Vec<T>`,
tuples of 2 to 4, and the six built-in event types (`ClickEvent`, `KeyDownEvent`,
`KeyUpEvent`, `MouseDownEvent`, `MouseUpEvent`, `MouseMoveEvent`). An impl on both sides for
the same type is a duplicate-impl error at compile time, which is how a clash shows up. A
payload of several arguments is `&(&a, &b)`. `Unwire` covers the scalars, `String`,
`SharedString`, `Option` and `Vec`.

**Names.** A kit type keeps its name in `Ada_Case` (Odin) or CamelCase (Go, docs/GO.md
section 2). Where gpui-base and gpui-component both have a type of the same name (`Button`,
`Checkbox`), the component's keeps the name and the base type is `Base_<Name>`. A name the
runtime already owns gets `Kit_` (`Kit_Tooltip`). A constructor is `snake(type)` for `new`
and `snake(type)_<name>` otherwise; a free function keeps its name. A builder trait is one
parametric table `<Trait>_Trait($T)` that each node type's `_VT` embeds with `using`, so
`x->with_size(..)` works for every implementor and the package has one procedure per
upstream method. Each generated type starts with a comment naming its upstream path
(`// Button is gpui_kit::component::button::Button.`); tools that pair Odin procedures with
upstream functions (`gates/conformance.py`) rely on it.

**Stops.** A buffered write that fails with `GK_ERR_STALE_HANDLE` or `GK_ERR_BAD_OP` stops the
program (exit 2) with the message and a hint to run with `GK_DEBUG=1`, which sends every
write at once and so stops at the line that made it. Other failures of a buffered write (a
window that has closed) are printed and the program goes on. A generated decoder that meets a
released kit-state or focus handle reads it with `genapi::shown` / `genapi::shown_focus` and
stops the program (`genapi::released_shown`) instead of drawing nothing.

**Handler values and lengths.** A `Handler` made with a value belongs to one builder or
subscription: giving it to a second one, or after the turn or view build that made it has
ended, stops at its line. Length arguments are checked against the builder's type
(`gk.auto()` where upstream takes a `DefiniteLength`; `gk.relative` or `gk.auto` where it
takes an `AbsoluteLength` stops). A duration travels as whole milliseconds, rounded, with a
negative one sent as 0 (`put_ms`).

**Posted messages** belong to the procedure they are posted to. Their strings (and, per
11.34, slices and dynamic arrays) are copied when posted and handed to the procedure in the
app's allocator (`context.allocator` of `gk.run`); the binding keeps nothing after the call.
Handler values are freed with their handler table. The binding warns on stderr when it holds
more than 16 MB for the app; `gk.test_owned_bytes()` says how much it holds.

**Turn arenas.** Each turn gets a slot of the arena ring (`rt_arena.odin`); a turn nested
inside another skips slots that are still live rather than stopping, so many synchronous turns
inside one handler are fine as long as few are nested at once.

**Rows drawn by the kit.** A table's cells are views, keyed `row * columns + id` with `id` the
column's index in the columns the app gave (it stays the same when the user moves a column).
The runtime keeps each row view's tape in its session (`Session::row_tapes`), so a shim never
reads a row's entity while the kit draws. That matters: in gpui-fast an entity read during a
draw makes the drawing view depend on that entity, and a table would come to depend on every
cell it had drawn.

**Tape writers.** The `put_*` procedures and `op` of `rt_wire.odin` write through one pointer
per call (`grow`). Generated code should call them; appending to the tape directly
(`append(o, ..)`) gives the same bytes, just more slowly.

**Conformance pages.** `gates/conformance.py` writes one gallery page per bound function for
both languages, with the Rust side calling the kit directly, from the description
`api/gk.api.json.gz` (its keys stay; new ones are only added). `gates/verified.json`, written
by `gates/gallery.sh --record` from a passing run, lists the pages that are byte-identical to
Rust and the functions they show; `gates/coverage-ratchet.py` refuses a later drop in that
count. The Rust reference `ref-gallery` must be rebuilt whenever `gates/conformance.py`
changes `ref/src/bin/gallery/conf.rs`; until then `gates/conformance.py --check` reports
"behind the description" without failing.
