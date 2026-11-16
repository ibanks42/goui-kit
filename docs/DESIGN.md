# Design: Odin and Go bindings for GPUI-kit

This document explains how the bindings work and why they are built the way they are. It is
for people who want to understand or change the binding itself. If you only want to write an
app, start with [GETTING-STARTED.md](GETTING-STARTED.md) (Odin) or
[GETTING-STARTED-GO.md](GETTING-STARTED-GO.md) (Go) instead.

The exact runtime contract (every turn, message and rule, as built) is
[PROTOCOLS.md](PROTOCOLS.md). The Go front end has its own design notes in [GO.md](GO.md).
What is bound today is counted in [STATUS.md](STATUS.md).

Pinned upstream: gpui-kit 0.9.1 (commit 0400b34), gpui-fast 0.1.14 (commit 8200882),
gpui-pre 0.3.12, all built from Zed commit d2a4fb6; rustc 1.99.1; rustdoc JSON format 61.

A few words used throughout:

- **Tape**: the compact byte buffer an app's view procedure writes to describe its elements.
  The Rust library turns a tape back into real GPUI-kit elements (section 6).
- **Turn**: one call from the library into the app, on the UI thread, between frames. All app
  code runs inside turns (section 7).
- **Handle**: a 64-bit number that stands for an object living in the Rust library (a kit
  state, a window, a focus handle). The app holds the number; the object stays in Rust.
- **Crossing**: one call across the C boundary between the app and the library.

## 0. Scope and design choices

These choices shape everything below. Where a later section seems to disagree, this one wins.

1. **Styles are tokens.** `gk.v_flex(.Size_Full, .Gap_2)`: GPUI's styling methods that take
   no arguments are members of one enum, not thousands of separate methods. Styling methods
   with arguments stay methods (`->w(gk.px(120))`).
2. **A handler marks its view for redraw automatically.** `gk.quiet(handler)` opts out. A view
   whose description did not change costs no redraw.
3. **One frame of placeholder rows after a far scrollbar jump is acceptable**, so that app
   code never runs in the middle of a frame.
4. **The default library leaves out tree-sitter syntax grammars, speech and `decimal`.** It is
   built with the gpui-kit features `gpui-fast` and `syntect` (`syntect` always; `gpui-fast` by
   default, see choice 9).
5. **One pinned Odin compiler.** The Odin package is tested against one Odin build
   (dev-2026-11-nightly:9a69916); `tools/install-odin.sh` and the CI workflow name it.
6. **One Rust library, one C interface.** The description of the interface names no
   language. Odin and Go are both generated from it; neither may bend the C interface for its
   own sake.
7. **Three platforms: Linux x86-64, macOS (one universal library for Apple silicon and
   Intel) and Windows x86-64.** Nothing in the C interface, the wire format or either
   package's public API is platform-specific. What differs per system sits in one file per
   platform (`odin/gk/rt_platform_{linux,darwin,windows}.odin` for the guard pages,
   `go/gk/rt_thread_{linux,darwin,windows}.go` for the thread id and how messages name the
   library). All three libraries are built by CI (section 16). The frame-cost and pixel
   gates run on Linux, where choice 8 is judged; the other platforms run the same code built
   for their target.
8. **Frame rate is the pass mark for everything.** A bound app must cost the same as a pure
   Rust GPUI-kit app, within noise: at most 1.03 times Rust's main-thread CPU per frame plus
   the measured Rust-against-Rust difference of the same run, for every scene, including
   scenes that re-describe views on every frame. This is measured by the standing gates
   (section 18), never assumed. A change whose gate fails is not done.
9. **gpui-fast is the default engine; upstream GPUI is supported.** The library's `gpui-fast`
   feature is on by default. `cargo build -p gk-capi --no-default-features` builds the same
   library on upstream GPUI (gpui-pre 0.3.12) instead. `gk_info` reports which engine a library
   uses (the `GK_FEAT_GPUI_FAST` bit and its version string), and the packages print it in
   their diagnostics; neither refuses a library built without gpui-fast. The diagnostic variant
   needs gpui-fast, because the retention counters it reads exist only there. The frame-cost
   pass mark (choice 8) is judged on gpui-fast, which is what the published libraries use.
10. **Go is a first-class front end** on the same library and C interface, generated from the
    same description and held to the same usability bar as Odin. What Go needs that the
    interface lacks is added in a language-neutral form, and Odin keeps passing. Go's frame
    cost and rebuild time are measured and reported. Its design is [GO.md](GO.md).

**How progress is counted.** Nothing is hand-written that a rule can generate, nothing is
tested by eye that a machine can test, and progress is the table in [STATUS.md](STATUS.md):
per component, the functions in total, bound, bridged, skipped, failed, not attempted and
verified. The generator rewrites it on every run.

## 1. The approach in short

The binding generates its API from the Rust API, and runs app code only between frames.

- The generator reads rustdoc's JSON output, classifies every public item by rule (plus one
  small override file), and lets rustc judge the result: a generated item that does not
  compile is quarantined with its error.
- A view is described as a typed tape, which Rust decodes recursively into real GPUI-kit
  elements, one decode function per type.
- Handlers fire synchronously with the real `&mut Window` and `&mut App` behind a context
  token, so they can return values and read true state.
- Kit state is read with direct calls (generated from the `&self` methods), not mirrored in
  the app.
- State builders are chains generated from the `self -> Self` methods.
- App code never runs inside a draw (not in render, layout, prepaint or paint).
- A re-described view whose tape is byte-identical to the last one is dropped before
  anything is notified.
- Rows come from a row source that the library asks for by range, between frames.
- Every closure parameter has one of five resolutions (section 12), every public item is
  either bound or skipped with a reason from a fixed list, and wire numbers are append-only.

Five details matter and are easy to get wrong:

1. **When views are described.** Describing views inside `Render::render` would put app code
   inside the draw; describing them at the end of every event would run them many times per
   frame under a fast background stream. Instead there is one *pre-frame turn* per window per
   frame, run from `Window::on_next_frame`. In gpui-fast those callbacks run before the frame
   decides whether to draw, in the same frame request (`crates/gpui/src/window.rs` 2031 to
   2075), and upstream's own benchmark `gpui_perf/examples/views_frames.rs` updates its panels
   the same way. That is one crossing per frame that has dirty views, and none inside the draw.
2. **A value the app read from kit state inside a view.** Replaying a cached tape when that
   value changes would show stale content, because the app computed that content. Instead the
   runtime observes the entity and marks the view dirty, so it is described again.
3. **Events raised inside an entity's own update.** Delegate methods receive
   `cx: &mut Context<TableState<Self>>`: the table entity is leased while they run, and
   reading or updating a leased entity panics in GPUI (`entity_map.rs` 183 to 197, 317). Such
   events are queued and delivered once the update has finished. Writes from every handler are
   applied after the outermost update ends. A read that would hit a leased entity returns an
   error instead of aborting.
4. **Counters and the profiler.** gpui-fast's retention counters (`LayoutStats.views_built`,
   `views_reused`, `Window::layout_stats`) exist only with the `test-support` feature, and
   `gpui-fps` switches on gpui-fast's `profiler` feature, which upstream keeps opt-in. So the
   shipped library links neither, and a second, diagnostic variant has both (section 3).
5. **The inventory depends on the feature set.** Items behind `tree-sitter`, `syntect`,
   `speech`, `decimal`, `inspector`, `test-support` and `profiler` are absent from rustdoc
   JSON unless those features are on, and so are `#[doc(hidden)]` items. The library's
   feature list is therefore part of the pin, and the API is extracted with exactly that list.

## 2. Requirements and how each is met

| requirement | how |
|---|---|
| GPUI-kit on gpui-fast by default, upstream GPUI on request | the library's default feature `gpui-fast` enables `gpui-kit/gpui-fast`; `gk_info` reports the engine (section 3) |
| Odin and Go, both first-class | one language-neutral interface description; one generator with an emitter per language |
| frame rate equal to a pure Rust GPUI-kit app, measured | 0 crossings on frames without app-state change; views are separate retained entities; the frame-cost gate against a Rust reference built from the same lock file (section 18) |
| Linux x86-64, macOS universal and Windows x86-64 | one download per system with the same contents; all three built by CI and published as GitHub releases (section 16) |
| no Rust toolchain on the app developer's machine | the developer receives a prebuilt shared library and generated source; the generator needs only Python and is never needed by an app developer (section 16) |
| a new upstream version is mostly a generator re-run | section 17 |
| the whole interface, not a hand-picked subset | every public item of the pinned feature set is bound, bridged, skipped with a reason, or quarantined with a compiler error; the report lists all four (section 13) |

## 3. Selecting the engine, and the library variants

GPUI-kit runs on either upstream GPUI (gpui-pre) or gpui-fast, chosen by one Cargo feature of
gpui-kit that is off by default upstream. This library turns it on by default. There is no
`[patch]`, no renamed crate and no direct GPUI dependency. From the manifests at 0400b34:

- the workspace `Cargo.toml` has `gpui = { package = "gpui-pre", version = "=0.3.12" }`
  (always) and `gpui-fast = { package = "gpui-fast", version = "0.1.14", features =
  ["windows-manifest-provided-by-host"] }` (optional), plus `gpui-fast-platform` and
  `gpui-fast-web`;
- `crates/kit/Cargo.toml` has `gpui-fast = ["dep:gpui-fast", "dep:gpui-fast-platform",
  "dep:gpui-fast-web", "gpui-base/gpui-fast", "gpui-component?/gpui-fast",
  "gpui-kit-assets?/gpui-fast"]`; the default features are `component` and `assets`;
- every kit crate starts with `#[cfg(feature = "gpui-fast")] extern crate gpui_fast as gpui;`
  and the facade re-exports `::gpui_fast::*` in place of `::gpui::*`;
- proc macros come from `gpui-pre-macros` in both cases and emit `gpui::` paths, which the
  facade resolves with `pub use crate as gpui`;
- `gpui_kit::GPUI_FAST: bool` reports the choice.

The library crate (`core/`, package `gk-capi`, library `libgpuikit`):

```toml
[lib]
name = "gpuikit"
crate-type = ["cdylib", "staticlib"]

[dependencies]
gpui-kit = { version = "=0.9.1", features = ["syntect"] }
gpui-fps = { version = "=0.9.1", features = ["gpui-fast"], optional = true }

[features]
default = ["gpui-fast"]
gpui-fast = ["gpui-kit/gpui-fast", "gk-meter/gpui-fast"]
diag = ["gpui-fast", "gpui-kit/test-support", "dep:gpui-fps", "gk-meter/diag"]

[profile.release]
strip = "symbols"
panic = "unwind"
```

(simplified; `core/Cargo.toml` has the full list) with `extern crate gpui_kit as gpui;` at the
crate root. `gpui_kit::GPUI_FAST` tells the library which engine it was built on, and `gk_info`
passes that on in its feature bits and version string, so a program, a bug report or a gate can
always tell which engine is running.

Two variants are built from the same generated code, both on gpui-fast:

| variant | features | used for |
|---|---|---|
| ship | gpui-fast, syntect | apps, and the frame-cost gates |
| diag | ship plus test-support and gpui-fps (profiler) | offscreen rendering, scripted input, retention counters, the FPS overlay, the pixel and protocol tests |

Both are published; an app ships with the ship variant. A library built with
`--no-default-features` is a ship variant on upstream GPUI; it is not published, and the gates
that compare frame cost against gpui-fast's retention do not apply to it. The ship library is about 58 MB
stripped on Linux.

## 4. Architecture

```
 app (Odin or Go)                      | libgpuikit (Rust, prebuilt)
 --------------------------------------+------------------------------------------
 state: plain structs                  | GkView entities: one per app view, hold a tape
 view procedures -> tape (bytes)   --->| decode tape -> real gpui-kit elements
 handler tables (ids)              <---| closures that carry (view, generation, handler id)
 handles (u64)                     --->| handle table: kit state entities, windows, focus,
 row sources: fill(range)          <-->|   scroll handles, subscriptions, tasks, row stores
 post queue (any thread)           --->| one wake on the main thread
```

The boundary is seven C functions and one callback. Everything else is data in two generated
opcode tables: *element ops*, written into tapes and replayed into elements, and *call ops*,
executed at once (or deferred) under a context token.

## 5. C interface

```c
typedef struct gk_info {
    uint32_t size, magic;
    uint16_t abi_major, abi_minor;
    uint64_t api_hash;              /* hash of the wire-relevant interface description */
    uint32_t features;              /* GK_FEAT_GPUI_FAST, GK_FEAT_DIAG, ... */
    const char *versions; size_t versions_len;   /* kit, gpui-fast, rustc, build number */
} gk_info;

typedef struct gk_cb gk_cb;         /* valid only inside the callback that received it */
typedef int32_t (*gk_callback)(void *user, gk_cb *cb, uint32_t kind,
                               const uint8_t *req, size_t len);
typedef struct gk_app { uint32_t size; uint16_t abi_major, abi_minor; uint64_t api_hash;
                        gk_callback callback; void *user; uint32_t flags; } gk_app;

const gk_info *gk_info_get(void);                                   /* any thread */
int32_t gk_run(const gk_app *app);                                  /* blocks; caller becomes the UI thread */
int32_t gk_call(const uint8_t *req, size_t len,
                const uint8_t **res, size_t *res_len);              /* UI thread, inside a callback */
int32_t gk_reply(gk_cb *cb, const uint8_t *out, size_t len);        /* copies; inside that callback */
int32_t gk_post(uint64_t token);                                    /* any thread */
const char *gk_last_error(size_t *len);                             /* thread-local */
int32_t gk_headless_run(const gk_app *app, const uint8_t *script, size_t len);  /* diag variant only */
```

Callback kinds: `INIT`, `WINDOW_INIT`, `FRAME`, `VIEWS`, `EVENT`, `ROWS`, `WAKE`, `DONE`,
`QUIT`.

Conventions:

- **Memory.** Neither side keeps or frees the other's memory. `req` belongs to the library
  and is valid during the callback. `gk_reply` copies before it returns. `*res` from
  `gk_call` belongs to the library and is valid until the next `gk_call`; both packages copy
  it out at once. This satisfies cgo's pointer rules without pinning.
- **Strings.** UTF-8; on the wire a u32 length then the bytes, never NUL-terminated.
- **Enums.** Numbers assigned by the generator and fixed in `api/opcodes.lock`, never Rust
  discriminants. Decoders have a catch-all that returns `GK_ERR_BAD_ENUM`.
- **Records.** Fields in declaration order; an `Option` is a presence byte; an enum with data
  is a u16 tag followed by its fields.
- **Handles.** A u64 made of a kind (8 bits), a generation (24) and an index (32); 0 is nil.
  A stale or wrong-kind handle gives `GK_ERR_STALE_HANDLE`, never undefined behaviour.
  Release is explicit; with `GK_DEBUG=1` the library prints live handles by kind at exit.
- **Errors.** Every export returns an i32 status; the message comes from `gk_last_error`.
  The packages turn a non-zero status into a panic with the message and the app's call site,
  except for operations upstream declares fallible, which return an error value.
- **Panics.** Every export and every Rust closure that enters the app runs inside
  `catch_unwind`. A caught Rust panic poisons the library (`GK_ERR_POISONED` from then on),
  and the package prints it and exits 1. Nothing unwinds across C: the Go callback recovers,
  prints the stack and exits 2; Odin traps.
- **Threads.** The thread that calls `gk_run` is the UI thread. `gk_call` and `gk_reply`
  check the thread and that a callback is active (`GK_ERR_WRONG_THREAD`,
  `GK_ERR_NO_CONTEXT`). The callback is only ever invoked on the UI thread. Nested callbacks
  are allowed and counted; each level has its own context generation.
- **Versions.** The package embeds `abi_major`, `abi_minor` and `api_hash` (the hash of
  `core/wire/ops.txt` and `api/opcodes.lock`). Ops are append-only within a major version, so
  `gk_run` accepts an app built against an older minor and refuses a different major or a
  newer minor, naming both versions.
- **Naming.** C: `gk_`, `GK_OP_<OWNER>_<METHOD>`, `GK_ENUM_<TYPE>_<VARIANT>`. Odin: package
  `gk`, the Rust snake_case names unchanged, types in `Ada_Case`. Go: package `gk`, the Odin
  names in CamelCase by fixed rules ([GO.md](GO.md) section 2). The generator applies three
  more rules to every emitted name:
  1. *Keywords.* A name that is a keyword of the target language gets a trailing underscore
     (`gk.switch_` for the kit's `Switch` constructor in Odin). The keyword list is part of
     each emitter, and the rename is recorded in the item's `names`.
  2. *Unnamed parameters.* A parameter upstream writes as `_` (or a pattern) is named
     `arg<N>` by its position among the method's own parameters, counted from 0
     (`fn secondary_selected(self, _: bool)` takes `arg0`).
  3. *Collisions after case conversion.* When two upstream names map to one target name
     (`bottom_neg_1_2` and `bottom_neg_12` would both give `BottomNeg12` in Go), digit groups
     that were separated by an underscore keep a separator, and any collision left over fails
     generation until a `[rename]` entry settles it.

## 6. Tape

A tape is one byte buffer per view build: a header (view id, generation, node count, handler
count, dependency list, root node), a node table (type, first op offset) and the op stream.
An op is `u16 opcode, u32 offset of the next op of the same node, arguments`. On the app
side an element is a node index plus the build's generation; a builder call appends one op
and patches the previous op's link. Attaching a child writes a `CHILD` op holding the child's
node index. Nothing is copied and nothing is allocated per call.

Rust decodes it typed and recursively: `decode_button(node)` holds a real `Button` in a local
and applies one match arm per op; a `CHILD` op recurses into the child's decode function and
gets an `AnyElement`. There is no enum of all element types. A builder trait's ops are one
generic function inlined into each implementor's decode loop, and the style-only traits run
on a borrowed `StyleRefinement`, so an element is never moved in and out of a function per op.

Rules of the format:

- A node is attached once; both packages check it and the decoder marks nodes consumed.
- **Methods that change the type** (`div().id(..) -> Stateful<Div>`,
  `overflow_x_scrollbar(self) -> Scrollable<Self>`, `with_animation`) are constructors of a
  node of the new type that take the old node as an argument. Generic wrappers are
  instantiated for the types listed under `[instantiate]` in `api/overrides.toml`; each
  instantiation gets its own type (`gk.div()->id("x")` gives a `Stateful_Div`).
- **Styles.** `Styled` has one required method, `style(&mut self) -> &mut StyleRefinement`,
  and about 3,200 provided ones. The ones with no arguments (about 3,150) are *style tokens*:
  one `STYLE` op carries a list of u16 token numbers that a single non-generic Rust function
  applies, so the table exists once in the library instead of once per styled type. Styled
  methods with arguments are ordinary ops routed through the same function. Style closures
  such as `hover(|style| ..)` take a described style: a `Style_Refinement` value built with
  the same tokens and builders as an element (`gk.style_refinement_default()->bg(..)`), which
  the library applies straight onto the style GPUI hands the closure.
- **Colours** are an HSLA literal, a theme token or a palette id. Theme tokens are resolved
  in Rust at replay, so a theme switch replays cached tapes without asking the app.
- **Fragments** are sub-tapes that a Rust closure replays each time upstream calls it
  (section 12).
- **Dependencies.** The header lists the kit entities the view procedure read through
  handles (section 9).
- A value that is not an element (a style, a column) may be given to any number of builders
  in one view build; an element still attaches once.

## 7. Frame lifecycle and turns

All app code runs in *turns*: one callback, on the UI thread, between frames.

```
 input event ---------> EVENT turn (synchronous, inside GPUI's dispatch)
 any thread: gk_post -> WAKE turn (a foreground task on the main loop)
 timer, completion ---> EVENT / DONE turn
 delegate event ------> queued; EVENT turn when the entity's update has ended
 frame request -------> pre-frame hook: FRAME turn (only if the app asked), then
                        VIEWS turn (only if a mounted view is dirty), then gpui-fast draws
 after the draw ------> ROWS turn (only if a row source has a range missing)
```

A turn's reply carries the return value (for handlers that have one), deferred call ops and
dirty marks. The first dirty mark for a window registers that window's pre-frame hook with
`Window::on_next_frame`, which also schedules a frame.

The pre-frame hook, per window:

1. If the app registered a frame handler (`cx->on_frame`), one `FRAME` turn. This is the only
   way app code runs every frame, and it is opt-in.
2. If any mounted view of the window is dirty, one `VIEWS` turn. The package runs the view
   procedure of each dirty mounted view, plus any newly slotted view that has no tape yet,
   compares each new tape with the view's current one, and replies with the tapes that differ.
3. For each changed tape Rust does `view.update(cx, |v, cx| { v.set_tape(t); cx.notify() })`.
   An identical tape notifies nothing.
4. gpui-fast then draws in the same frame request: it renders exactly the notified views
   (`GkView::render` only decodes) and reuses every other view from the last frame.

What follows from this:

- A frame with no app-state change costs 0 crossings and 0 decoding, and gpui-fast's
  retention is untouched.
- A frame with dirty views costs 1 crossing, however many views changed and however many
  events or posts arrived since the last frame.
- The result of a click is on screen in the same frame a Rust app would show it.
- A dirty view that is not mounted stays dirty and is described when it is mounted.
- App code never runs during a draw, so the two things gpui-fast's documentation warns about
  (notifying during prepaint or paint, notifying every frame without a change) cannot happen
  through the binding.

Offscreen (the diag variant on `HeadlessAppContext`, which has no platform frame loop), the
headless driver calls `Window::simulate_next_frame` before each capture, so the same hook
runs.

**Handler tables.** A handler id is valid for the tape generation it was registered in and
the one before. The `VIEWS` request tells the package which generations are dead; only then
is that generation's table retired. An event for an older generation is dropped and counted
(printed with `GK_DEBUG=1`).

**An identical tape still replaces the handler table.** Handler data (`gk.on(select, row_id)`)
is not in the tape, so two builds can produce the same bytes with different data behind the
same handler ids. When a re-described view's tape is identical to the current one, the tape is
dropped and nothing is notified, but the package replaces that generation's handler table in
place with the new build's: same ids, same generation, new procedures and data. The misuse
gate has a case for this (the handler must see the data of the latest build).

**When a write lands.** A write to kit state is visible to every later step of the same frame:

1. Writes buffered in a turn are applied when that turn's reply is processed, as soon as no
   entity is leased: for an `EVENT` turn, from the deferred callback that runs when the
   outermost update has ended; for `INIT`, `WINDOW_INIT`, `WAKE`, `DONE` and `FRAME` turns,
   which run outside any update, immediately after the callback returns.
2. In the pre-frame hook the order is therefore: `FRAME` turn, apply its writes, mark dirty
   every mounted view whose dependency list names an entity that was just written (the
   runtime does this itself from the tape headers rather than waiting for GPUI's observers,
   which would fire after the hook), then the `VIEWS` turn, then the draw. A write from a
   per-frame handler is on screen in the frame that handler ran in, as in Rust.
3. A view procedure may not write: a write or a notify during a `VIEWS` turn is an error at
   the call site. So nothing is ever applied between the description and the draw.
4. A write made while its target is leased (a queued delegate event) is applied from
   `Window::defer`, before the next draw; reading the target back in that handler returns
   `GK_ERR_BUSY`.

Rule 2 has a gate: a per-frame handler writes an input's value, a view echoes it, and the
echoed text must be in the same frame's pixels (`gates/pixels.sh writerule`, plus the
`write-rule`, `frames` and `view-write` modes of `core/tests`).

## 8. Mapping GPUI's app model

The app never defines a Rust type. A small fixed set of Rust types in the library stands in.

- **Application.** `gk_run` calls `gpui_kit::application().run(|cx| { gpui_kit::init(cx);
  ... })`, sets up the session and makes the `INIT` turn.
- **Views.** `GkView { id, tape, previous_tape, generation, focus_handle, deps }` implements
  `Render` and `Focusable`. On the app side a view is a state pointer, an optional init
  procedure and a view procedure: `gk.view(&state, init, view)`. `child_view(v)` embeds a
  view in another's tape; a child's change does not alter the parent's tape, so the parent
  stays retained. Splitting a window into views is the main performance tool.
- **Windows.** `gk.open_window(cx, options, root_view)` calls `gpui_kit::open_window`, so the
  root sits inside the kit's `Root` and dialogs, sheets, notifications and menus work. Inside
  the build closure, with the window in hand, the runtime makes a `WINDOW_INIT` turn (the
  view's init procedure, where kit state that needs a window is created: `InputState::new`
  and `TableState::new` take `&mut Window`) and then describes the root view and everything
  it slots, so the first frame is complete.
- **Contexts.** `&mut App`, `&mut Window` and `&mut Context<T>` never cross the boundary.
  During a callback the runtime keeps them in a scoped thread-local, and the app receives a
  `gk.Cx` stamped with the callback's generation. A `Cx` kept after its callback fails its
  check loudly. Each call op declares whether it needs a window; a turn without one (`INIT`,
  `WAKE`) reaches a window through its handle.
- **Kit state.** `InputState`, `TableState<D>`, `SliderState` and the other state types are
  real entities in the handle table. Their `self -> Self` methods form a construction builder
  (`gk.input_state(cx)->placeholder("Name")->build()`, one crossing for the chain); their
  `&self` methods are reads; their `&mut self, .., cx: &mut Context<Self>` methods are
  generated as `entity.update(cx, |s, cx| s.method(args, window, cx))`. Types generic over a
  trait the app would implement are instantiated with the binding's bridge type
  (`TableState<GkTable>`), listed in `api/overrides.toml`. Upstream's `InputBaseState<M>` is
  instantiated by the generator as `InputState`, `EditorState` and `TextareaState`.
- **Events from kit state** (the `EventEmitter` types): `gk.on_event(state, cx, handler)` is
  `cx.subscribe`; the `Subscription` lives in the handle table until the state is released.
- **Focus.** `cx->focus_handle()` returns a handle; `track_focus(h)` is a generated builder;
  `focus(h)` is a call op; every `GkView` has its own `FocusHandle`.
- **Actions and keys.** No macros. `gk.action("people::AddRow")` returns an id; in Rust all
  app actions are one type, `GkAction { id }`, whose `partial_eq` compares ids.
  `gk.bind_keys` builds `KeyBinding::new(keys, GkAction { id }, context)`.
  `on_action(a, handler)` installs one `GkAction` listener per element that fires the handler
  when the id matches and propagates otherwise. Kit and GPUI built-in actions are generated
  name constants built with `App::build_action(name, json)`. Because
  `App::is_action_available` goes by type, menu availability for app actions is coarse.
- **Globals.** The app's globals are ordinary app variables. The kit's `Theme` global is read
  and changed through generated get, modify and set call ops.

## 9. Handlers, state and dependencies

**Handlers are integer ids.** While a view is built, each handler is stored in that
generation's handler table and the tape carries its index. The generated Rust closure for
`on_click` is `move |ev, window, cx| rt::fire(view, generation, handler, SHAPE_CLICK, ev,
window, cx)`. There is one trampoline per closure shape and one encoder per event payload
type.

**Element-listener handlers fire synchronously.** `fire` puts `&mut Window` and `&mut App` in
the scoped context, makes an `EVENT` turn and reads the reply. So handlers with a return value
work (`Fn(..) -> bool`, `on_paste`), and so do `stop_propagation` and `prevent_default`.

**Queued events.** Events raised by a bridge from inside an entity's update (delegate methods:
`perform_sort`, `load_more`, `confirm`, `set_selected_index`, `move_column`, `context_menu`,
`perform_search`, `visible_rows_changed`) are queued and delivered from `Window::defer`,
which runs when the outermost update has ended and before the next draw. Subscription events
already arrive outside the emitter's update and fire directly.

**Writes are deferred, reads are direct.**

- Call ops that return nothing (`set_value`, `scroll_to_row`, `focus`, row batches, `notify`)
  are buffered and travel in the turn's reply. Rust applies them from one deferred callback
  when the outermost update has ended, in order, before the next draw. No write from app code
  can meet a leased entity, and a typical handler costs 1 crossing.
- A call op that returns a value goes through `gk_call` at once. It first flushes buffered
  writes so order is preserved. Before touching an entity, a flush or read probes it with
  `Entity::read` inside `catch_unwind`: a leased entity panics there before anything is
  changed, and the probe turns that into `GK_ERR_BUSY` ("this state is being updated by the
  component that called the handler; write to it without reading it back, or do it in
  `cx->defer`"). The same situation is a panic in Rust.
- With `GK_DEBUG=1` every op is sent at once, so an error points at its own line.

**What a handler may do:** change app state; read and change kit state through handles; call
any context method (focus, open a dialog or sheet, push a notification, dispatch an action,
use the clipboard, open or close windows, quit, switch theme); create and release handles;
mark views dirty; start background work. It may not use an element built in a view or keep
the `Cx` (both are generation-checked).

**Dirty marks.** The view that registered the handler is marked dirty when the handler
returns, which is what Rust GPUI apps do anyway with `cx.listener` and `cx.notify()`.
`gk.quiet(handler)` opts out; `view->notify()` marks any view; `gk.refresh(cx)` marks every
view and every row a list shows. An unnecessary mark costs one view procedure and a memory
compare, because an identical tape is dropped. A notify during a view build is an error at the
call site.

**App state** lives in the app's language and Rust never sees it. This is gpui-fast's
"untracked state" case, so the binding does the bookkeeping:

- `gk.Model(T)` in Odin and `gk.Model[T]` in Go hold a value and the set of views that read
  it; `get` in a view build records the view, `set` or `update` marks those views dirty. No
  crossing. It is optional: small apps need only the automatic mark.
- A view procedure that reads kit state through a handle (`gk.value(s.name, cx)` inside a view)
  records the handle in the tape's dependency list. When the tape is accepted, the runtime
  observes those entities (`cx.observe`) and marks the view dirty when one notifies. That is
  what a Rust view that reads `input.read(cx).value()` pays: one re-render per change.
- Values read on the Rust side during replay (theme tokens, a kit entity an element is built
  from, as in `Input::new(&entity)`) are tracked by gpui-fast itself.

Typing in an input, dragging a slider, scrolling a table, hover and press are handled inside
the kit's entities in Rust. No app code runs unless the app subscribed.

Scroll positions, list state, resizable sizes, dock layouts and focus are kit or GPUI state
behind handles, never mirrored in the app. Dock layouts and similar serde types cross as JSON
text.

A view that still shows kit state the app has released stops the program with a message
naming both, rather than drawing garbage.

## 10. Delegates and virtual lists

Rule: the library never asks the app anything during layout, and never once per row.

`TableDelegate`, `ListDelegate`, `SearchableListDelegate`, tree rows, `uniform_list`, `list`
and `v_virtual_list` are each implemented once in Rust over *row sources*. The app supplies a
row source: a count and a procedure that describes rows. Each row is a view of its own,
pulled while the list draws and described before the next frame (PROTOCOLS.md section 9).

- The first rows travel with the list's creation, so the first frame is complete.
- When the list needs rows it does not have, it records the wanted range; after the frame, one
  `ROWS` turn asks for that range plus overscan, and only that list's entity is notified.
  Steady scrolling finds rows already present. A far jump (a dragged scrollbar) shows
  placeholder rows for one frame (choice 3).
- Live data: the app tells the source its count changed, or that a range is stale.
- A table delegate answers from data: cell texts, row labels, fit widths and groups are values
  the app gives, not callbacks.

Scrolling a list of any size costs 0 crossings per frame and at most one `ROWS` turn now and
then; the elements are the kit's own, so gpui-fast's scroll layers and retained rows apply
unchanged. Charts take their data as a table too: with the row type fixed to the bridge row,
`.x(|d| ..)` and `.y(|d| ..)` become column names.

`perform_search -> Task<()>` returns a task that completes when the app has pushed the
filtered rows.

## 11. Background work and async

Every export except `gk_post` belongs to the UI thread, and the library enforces it.

- **Wake.** Each package keeps a mutex-protected queue in its own language. `gk.post` pushes
  an item and calls `gk_post` only when the queue goes from empty to non-empty. Rust holds a
  `SendForegroundExecutor` (`ForegroundExecutor::to_send()`, which is Send, Sync and Clone)
  and spawns a main-thread task that makes one `WAKE` turn, in which the package drains its
  whole queue. No payload crosses C. A thread that posts 10,000 times a second causes at most
  one `WAKE` per turn of the main loop, and the views it dirties are described once per frame.
- **Go:** `gk.Post(func(cx *gk.Cx) { ... })` from any goroutine, or `gk.Go(cx, work, done)`
  as the convenience form. The package locks the main goroutine to the main OS thread in
  `init`, and `gk.Run` must be called from `main`.
- **Odin:** `gk.post(proc(s: ^App, cx: ^gk.Cx, msg: Tick), tick)` from any thread. The message
  is copied into the queue and passes the same type walk as handler data (section 14).
- **Timers:** `cx->after(d, handler)` and `cx->every(d, handler)` are foreground tasks that fire
  as handlers; releasing the handle drops the task, which cancels it.
- **Requests that answer later** (`prompt_for_paths`, `Window::prompt`, file dialogs, and
  upstream methods that return a `Task` or a oneshot receiver): the call op returns a
  completion id; the runtime awaits on the main thread and delivers a `DONE` turn. Releasing
  the id cancels.
- **The kit asking the app for something asynchronous** (`CompletionProvider::completions`
  returns `Task<Result<..>>`): the bridge fires an event with a request id and returns a task
  backed by a oneshot; the app answers with `cx->complete(id, value)` whenever it likes.
- `spawn` of app futures does not cross; goroutines and threads replace it.

## 12. Closures and the remaining families

Every closure parameter gets exactly one resolution. The generator refuses a closure with none.

| resolution | what crosses | used for |
|---|---|---|
| `handler` | a handler id; fires as an `EVENT` turn | `on_*` listeners, subscriptions, actions |
| `build` | a sub-tape of builder ops, applied once | `FnOnce(X) -> X`, style closures, menu builders |
| `fragment` | a sub-tape that Rust replays whenever upstream calls it | dialog, sheet, popover, tooltip and empty-state bodies; render closures without an item argument |
| `data` | values the app computed, shipped ahead | chart accessors (column names), row and cell content |
| `queued` | a handler id; the event is delivered after the entity's update | delegate and provider methods that are really events |

Content that must change while open (a dialog body) is a child view inside the fragment, so it
has its own dirty flag. Render closures that take an item argument (a calendar's days, a tree's
entries, a popover's content) are typed procedures the app writes and gives with `gk.items`;
they run between frames, not while the component draws (PROTOCOLS.md 9.41). Closures that only
map numbers are expressions, and animations are data, not closures (PROTOCOLS.md 13.30).

Other families:

- **Ambient context only:** call ops under the `Cx`, generated.
- **Context<T> mutators:** generated `entity.update` wrappers.
- **Borrowed results:** copied out; `&mut` plain data is get, modify, set.
- **Stateful handles** (`FocusHandle`, `ScrollHandle`, `UniformListScrollHandle`,
  `ListState`, `Subscription`, `Task`): the handle table; methods generated.
- **Library objects that are not entities** (scales, menus, values a read hands out): kept
  behind handles, temporary or kept, described in PROTOCOLS.md section 13.
- **App-implemented traits.** `Render`, `View`, `Focusable`: `GkView`. `RenderOnce`,
  `IntoElement`: not needed, since a component in the app's language is a procedure that
  returns an element. `Action`: `GkAction`. Table, list, searchable and tree delegates: row
  bridges. `Panel`: `GkPanel`, a `GkView` plus data and fragments, so dock panels are app
  views. Editor providers: request ids, LSP types as JSON. Custom drawing: a plot or paint
  list the app describes for known bounds. `EntityInputHandler`, `InputHandler`: not exposed;
  the kit's Input, Textarea and Editor cover text entry. `AssetSource`: bytes or a directory
  registered at start, no callback.
- **Type-erased values.** `dyn Action` is the action value; a drag payload is the library's
  drag value with a fragment as preview, and `on_drop` filters by tag.
- **Platform and backend traits**, test-support items and third-party types are skipped by
  role. chrono values are records, `serde_json` and lsp-types are JSON text, `Rope` is copied
  text or a ranged read, bitflags are integers.
- **Macros:** actions are made at run time; the style macros' output is already in the JSON
  as `Styled` methods.
- **gpui-fast-only API** (window composition, dynamic textures, scene layers): tagged
  `fast_only`.
- **Name collisions** between gpui, gpui-base and gpui-component: the priority is component,
  then base, then gpui; the shadowed item gets a prefix by rule or a `[rename]` entry; an
  unresolved collision fails generation.

## 13. Generator

Repository layout:

```
api/            the interface description, language-neutral: rustdoc/ (the extracted API,
                committed), overrides.toml (the one hand-edited input), opcodes.lock
                (append-only wire numbers), gk.api.json.gz (the classified API),
                coverage.json, quarantine.json, clone.json, accesskit-role.json
gen/            the generator: gkgen.py (classify, number, emit), rdj.py (rustdoc JSON
                reader), gk_rust.py, gk_odin.py, gk_go.py and the family modules;
                extract.sh (stage 1), check.sh and round.sh (the compile check)
core/           the Rust library, package gk-capi, built as libgpuikit: src/ (the protocol
                runtime by hand; src/gen/ generated), include/ (C headers), wire/ (the
                runtime's own opcode table), meter/ (crate gk-meter: measuring and driving,
                linked into the library and the references), tests/ (protocol tests)
odin/gk/        the Odin package: rt_*.odin and kit_*.odin by hand, gen_*.odin generated
go/             the Go module gpuikit.dev, package gk: rt_*.go by hand, gen_*.go generated
examples/       example apps in both languages, and the misuse cases
ref/            pure Rust reference apps for the gates (crate gk-ref)
gates/          the checks; all.sh runs them and prints one summary table
tools/          building the library, the download and the release; status.py
docs/           the guides, this design, PROTOCOLS.md, STATUS.md
```

Stages. Extraction and the compile check need Rust; everything else needs only Python and the
committed files.

1. **Extract** (`gen/extract.sh`). `cargo doc` with rustdoc's JSON output (`RUSTC_BOOTSTRAP=1`)
   for gpui-fast, gpui-base, gpui-component and gpui-kit, with exactly the library's features,
   plus a second pass with `--document-hidden-items`. The result is committed under
   `api/rustdoc/`. The API is extracted from a debug build, so items that exist only in debug
   builds (`DivInspector`, `ToggleInspector`, `Styled::debug`) are skipped as `feature-off`.
   rustdoc records each item's cfg; the generator keeps the target part and writes
   `#[cfg(..)]` on decoders of platform-only types (the Wayland layer shell exists on Linux
   only).
2. **Classify.** Rules plus `api/overrides.toml` plus `api/quarantine.json` give
   `gk.api.json`: every type, enum, record, element, state type, method, closure shape and
   bridge, with its class, the rule that bound it or the reason it is skipped, wire types,
   names in C, Odin and Go, and an opcode from `opcodes.lock` (append-only: a removed item's
   number is retired, never reused).
3. **Emit.** `make gen` writes the Rust decoders into `core/src/gen/`, the Odin package into
   `odin/gk/gen_*.odin`, the Go package into `go/gk/gen_*.go`, the opcode tables, a sample
   program that calls every generated procedure, the coverage table, STATUS.md and the
   gallery's generated conformance pages. It takes about 2 seconds and needs no Rust.
4. **Check.** rustc judges the result when the library is built. `gen/round.sh` runs the
   quarantine loop: `cargo check` with JSON diagnostics, every generated arm sits between
   `// @item <id>` markers, an error is mapped to its item, the item goes into the quarantine
   file with the diagnostic, and emission re-runs.

An item record in `gk.api.json`:

```json
{"id": "gpui_component::button::button::Button::on_click",
 "rust": "fn on_click(self, handler: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static) -> Self",
 "sig_hash": "…", "owner": "Button", "owner_role": "element", "kind": "builder",
 "class": "HAND", "family": "closure", "resolution": "handler", "shape": "S017",
 "table": "element", "op": 1881,
 "args": [{"name": "handler", "wire": "handler", "event": "ClickEvent", "returns": "unit"}],
 "names": {"c": "GK_OP_BUTTON_ON_CLICK", "odin": "on_click", "go": "OnClick"},
 "status": "bound"}
```

Wire types (a closed list): `u8 u16 u32 u64 i32 i64 f32 f64 bool str bytes enum:<T> rec:<T>
opt<..> list<..> node handle:<kind> handler:<shape> fragment style color length pixels
element_id action json`.

Rules that bind without an override: a plain builder returning `Self`; a plain call; `impl
Into<T>` and `AsRef` with T's closed set taken from the `From` impls; `Option`; `Vec`, slices
and iterators; fieldless enums; enums with data; public-field or Copy structs as records;
`Result`; element arguments; borrowed plain results; ambient context; state mutators; state
reads; constructors taking `&Entity<State>`; type-changing methods (section 6); handler, build
and fragment closures by shape; chart accessors once the row type is fixed; generic types
instantiated for the argument lists they are used with.

`api/overrides.toml` is the only hand-edited input. Its sections include `[skip]`,
`[instantiate]`, `[bridge]`, `[into.primary]`, `[closure]` and `[rename]`. Every key must
match an item, or generation fails.

Checks at generation time (they fail before anything is built):

- an unknown rustdoc JSON format version;
- an override key that matches nothing;
- a public item with no status: every item must be `bound`, `bridged`, `skipped` (with a reason
  from a fixed list: platform-trait, test-only, third-party-type, hidden, feature-off,
  superseded-by-<protocol>, rust-only-extension, item-render-closure) or `quarantined`;
- a closure without a resolution, an event payload without a record, a handle kind without a
  creator, an opaque argument type without a constructor the binding can drive;
- a new name collision without a `[rename]`;
- a wire number changed or reused;
- coverage falling against `gates/coverage-baseline.json` without `--accept`.

A few parts are hand-written because no rule fits them: the bridges in `core/src/hand.rs` and
`odin/gk/kit_bridges.odin` (a uniform list over rows kept in the library, and `gk.text`, a
string as an element), and the shims for the input, editor, select and rows families
(`core/src/shim_*.rs`, `odin/gk/kit_*.odin`).

Compile cost is designed for:

- Rust: the decoder is one module per upstream module, and styles are decoded once, not per
  type.
- Go: no generics in the fluent API, so the package compiles once into Go's cache.
- Odin: there is no build cache, so method tables are reachable only from each type's
  constructor and style tokens are one enum. A gate caps the rebuild time after a one-line
  edit.

## 14. Odin API and safety

The same screen as a Rust GPUI-kit app, in Odin. Method names are upstream's
(`Button::new(..).primary().label(..).on_click(..)`, `InputState::placeholder`,
`Column::new(..).width(..)`, `TableState::row_selectable`, `v_flex`, `scroll_to_row`,
`set_value`). The snippet shows the shape of the API; the examples and the guides hold
programs that are built and tested.

```odin
package people

import "core:fmt"
import gk "gpuikit:gk"

Person :: struct { id: u64, name: string, score: f64 }

App :: struct {
	name:  gk.Input_State, // handles to kit entities that live in the library
	table: gk.Table_State,
	rows:  [dynamic]Person, // the app's own data, ordinary Odin memory
	added: int,
}

init :: proc(s: ^App, cx: ^gk.Cx) { // once, when the window builds its root view
	for i in 0 ..< 100_000 do append(&s.rows, Person{u64(i), fmt.aprintf("Person %d", i), f64(i % 977)})
	s.name = gk.input_state(cx)->placeholder("Name")->build()
	// ... the table state over a row source of s.rows
}

view :: proc(s: ^App, cx: ^gk.Cx) -> gk.Element { // runs only when this view is dirty
	return gk.v_flex(.Size_Full, .Gap_3, .P_4)->
		child(gk.h_flex(.Gap_2, .Items_Center)->
			child(gk.div(.W_64)->child(gk.input(s.name)->cleanable(true)))->
			child(gk.button("add")->primary()->label("Add row")->on_click(gk.on(add_row)))->
			child(gk.label(fmt.tprintf("%d rows, %d added", len(s.rows), s.added))))->
		child(gk.div(.Flex_1)->child(gk.data_table(s.table)->stripe(true)))
}

add_row :: proc(s: ^App, cx: ^gk.Cx) {
	name := gk.value(s.name, cx) // gk.Temp_String, valid until this handler returns
	if len(name) == 0 do return
	append(&s.rows, Person{u64(len(s.rows)), gk.keep(name), 0}) // a Temp_String does not assign to a string
	s.added += 1
	gk.set_value(s.name, cx, "")
} // the view that registered this handler is marked dirty on return

main :: proc() {
	app: App
	gk.run(&app, proc(s: ^App, cx: ^gk.Cx) {
		gk.open_window(cx, "People", {1024, 700}, gk.view(s, init, view))
	})
}
```

How the package is shaped:

- Every element and builder is a small value `{using el: Element, using vt: ^Button_VT}`;
  `x->f(a)` calls the procedure in field `f` with `x` first. A chain continued on the next line
  ends its lines with `->`. A kit state or library object handle is `{using state: State}` with
  plain procedures named after its type, `gk.input_state_value(s, cx)`, and a procedure group
  per method name, `gk.value(s, cx)`, where that name is free in the package: a method table
  would link the component family of every method of every handle a program could get
  (section 16).
- Styles: the no-argument `Styled` methods are members of one enum, `gk.Style_Flag`, written
  with implicit selectors in constructors and `->style(..)`. Styled methods with arguments are
  normal methods (`->bg(c)`, `->w(gk.px(120))`).
- Constructors and free procedures are procedure groups, so `gk.button("add")` and
  `gk.button(gk.id("row", i))` both exist. Every entry takes `loc := #caller_location`.
- A handler is a procedure plus at most one value: `gk.on(add_row)`,
  `gk.on(select_row, row_id)`, or with the event
  `gk.on(proc(s: ^App, cx: ^gk.Cx, e: ^gk.Click_Event, id: Row_Id) {..}, id)`. The state type
  is checked against the view's, and the event type against the builder's shape, when the
  handler is registered, with the caller's location.
- One `foreign import` block holds the seven functions; `#load_hash` of the opcode table makes
  a stale generated package fail the build.

**Safety.** The bug this design sets out to make impossible: a string made during a view is
given to a handler, the handler stores it in app state, the memory is reused two views later,
and the app quietly shows another row's content, with exit status 0 and the address sanitizer
silent.

1. **Handler data is owned by the package.** `gk.on(procedure, data)` copies `data` at
   registration, after walking its type information once per type: plain fields are copied,
   every `string` inside is copied; pointers, slices, maps and dynamic arrays are rejected with
   a panic that names the type, the field and the caller's line, unless wrapped in
   `gk.unchecked(..)`. The copy is freed with the handler table that holds it; the strings of a
   value whose handler ran are kept, so a handler that stored one holds memory that is never
   reused. `gk.post` applies the same walk, and a posted message belongs to the procedure: its
   strings arrive in the app's allocator, to keep or delete. The package warns on stderr when
   what it holds for the app passes 16 MB, and at every doubling.
2. **Nothing lent by the package has type `string`.** Getter results and event text are
   `gk.Temp_String :: distinct string`. Storing one in a `string` field does not compile;
   `gk.keep(x)` clones it with the app's allocator. Passing one to a builder is fine, because
   builders copy into the tape.
3. **Turn memory dies visibly.** A turn's tapes are copied by `gk_reply` before the callback
   returns, so nothing a view allocated is needed afterwards. `context.temp_allocator` during a
   turn is a per-turn arena taken from a ring of reserved address ranges. In debug builds, and
   in release builds with `-define:GK_GUARD=true`, retiring it makes the pages inaccessible
   (`mprotect(PROT_NONE)` on Linux, a fresh mapping on macOS, decommit on Windows), so a stale
   read (even through an explicit `string(x)` cast) faults on the line that reads, and the fault
   handler prints which turn the memory belonged to.
4. **Generation checks in every build.** An element used outside its build or attached twice, a
   `Cx` kept past its turn, a released or wrong-kind handle: one integer compare and a panic
   naming both source locations.
5. **Thread check** on every entry except `gk.post`.
6. **Paranoid mode** (`-define:GK_PARANOID=true`): every deferred op is sent at once, and each
   view procedure runs twice per build with the tapes compared, which exposes a view that
   depends on hidden state or stale memory.
7. **Proof.** `examples/odin/negative*/`, run by `gates/misuse.sh`, hold the stored view string
   and its variants (a temporary string as handler data, event text stored without `keep`, an
   element kept across builds, a stored `Cx`, a UI call from another thread). Each must behave
   correctly, fail to compile, or stop with the expected message. Exit 0 with wrong content
   fails the suite.

What stays the developer's job: the lifetime of app state, and pointers wrapped in
`gk.unchecked`.

## 15. Go API

The Go front end is described in [GO.md](GO.md). In short, compared with Odin:

- Handlers are closures or method values; the garbage collector keeps captures alive, so there
  is no handler-data rule. Each handler builder takes the event:
  `OnClick(func(*gk.Cx, gk.ClickEvent))`.
- Methods are real methods on concrete types, with no generics in the fluent API.
- Style tokens are typed constants in a small package (`s.Gap3`), because Go has no implicit
  enum selectors. Theme colours are in package `theme`.
- There is no overloading: a parameter upstream declares as `impl Into<T>` takes the primary
  member of T's closed set, and other members get a suffixed variant.
- Strings returned to the app are Go strings and always safe to keep.
- An element handle is a one-pointer struct into a node block allocated per view build and left
  to the collector; a kept handle keeps its generation and is always detected.
- Background work is goroutines with `gk.Post` and `gk.Go`.
- The package uses cgo over the seven functions and one exported callback, so a build needs a C
  compiler. A panic in app code inside a turn prints the Go stack and exits 2. Go's wasm targets
  have no cgo and are out of scope.

## 16. Distribution: what lands on the developer's disk

Rust is compiled only to build the library: by CI for releases, or by a contributor building
from source. An app developer never needs Rust, a cargo cache or a `target/` directory.

The developer downloads one archive per platform from the repository's GitHub releases:
`gpuikit-VERSION-linux-x86_64.tar.zst`, `gpuikit-VERSION-macos-universal.tar.zst` or
`gpuikit-VERSION-windows-x86_64.zip`, plus `SHA256SUMS`. All three hold the same files except the
library. The diagnostic library (offscreen rendering, scripted input) is not shipped: it is for
this repository's own tests.

| file | what |
|---|---|
| `lib/libgpuikit_static.a` | the library as a static archive, which apps link into themselves: on Linux built against glibc 2.28, so programs run on any Linux with glibc 2.28 or newer; universal (arm64 and x86_64, minimum macOS 11.0) on macOS; MinGW's format on Windows, for Go, beside `gpuikit_static.lib`, MSVC's format, for Odin |
| `lib/libgpuikit_sys.so`, `libgpuikit_sys.a` | Linux: a linker script naming the system libraries the archive needs by their run-time names; Windows: the import library of the Windows API it links |
| `windows/gpuikit.manifest`, `gpuikit.rc` | Windows: a program must embed or sit beside this manifest (per-monitor DPI v2, common controls 6), or it exits before `main` |
| `include/` | the C headers |
| `odin/gk/` | the Odin package |
| `go/` | the Go module |
| `examples/`, `docs/` | the example apps in both languages and the guides |
| `VERSIONS`, `THIRD_PARTY.md`, licence texts | versions, hashes, what the library needs (glibc, minimum macOS, imported DLLs), and the licences of every crate linked in |

A download is mostly the library: on Linux about 66 MB packed and 390 MB unpacked (the static
archive keeps every component family; a program keeps only the ones it uses).

Also on disk because of the developer's own toolchain: Go's build cache holds the compiled `gk`
package; Odin has no cache. For comparison, a Rust developer of the same app holds a target
directory of about 2 GB per checkout, a registry of 0.5 to 1.5 GB and a toolchain.

Tools the developer needs: Odin with its linker, or Go with a C compiler. Runtime libraries on
Linux: libxkbcommon, libxkbcommon-x11, libxcb, libxcb-xkb, libXau, libgcc_s, libm, libc;
Vulkan and Wayland are loaded at run time.

Delivery:

- Odin: unpack anywhere, build with `-collection:gpuikit=<dir>/odin`.
- Go: the app's `go.mod` requires `gpuikit.dev` and replaces it with the download's `go/`
  folder.
- Linking: the static library, into the program, in both languages. The library's dispatchers are
  split by component family, and each generated procedure (Odin) or function (Go) links the family
  it uses (`gen/gk_fam.py`), so a program keeps only the families it calls into: one file, about
  50 to 60 MB for a small app, where a program plus the shared library was 76 MB. Odin's package
  carries the linker flags itself (`@(extra_linker_flags)` on its `foreign import`); Go can't, so a
  Go build passes `-ldflags=-extldflags=-Wl,--gc-sections` (`-Wl,-dead_strip` on macOS; on
  Windows `-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres`, since GNU ld keeps every
  function with unwind data there; package `gk` embeds the manifest as a .syso). The shared library
  is still built, for the repository's tests (`-define:GK_SHARED=true`, `-tags gkshared`), but not
  shipped (GO.md section 8).

How the libraries are built: CI (`.github/workflows/platforms.yml`) runs `tools/build-lib.sh` on
each system. Linux is linked by zig against glibc 2.28 (`tools/zigbuild.sh`, with a pinned zig
and cargo-zigbuild). Windows is built natively with MSVC, because gpui-fast's build script
compiles the release shaders with `fxc.exe` only on a Windows host. macOS builds both slices on
one Apple silicon runner and joins them with `lipo`. `tools/dist.sh` assembles the download,
`tools/dist-test.sh` builds the hello example in both languages from it and runs it
offscreen, and a pushed tag `vVERSION` publishes a release once all three platforms pass
(`tools/release-assets.sh` checks the archives and writes `SHA256SUMS`). A release is never
replaced: a fix is a new version.

Licences: gpui-kit, gpui-fast and gpui-pre are Apache-2.0; the icon set is under the Lucide
licence; `tools/licences.py` collects the licence texts of every crate linked in for each
target.

## 17. Upstream updates

A new upstream version is mostly a re-run: change the pins in `Cargo.toml`, extract the API
again (`gen/extract.sh`), run `make gen`, build the library (rustc judges the generated code;
`gen/round.sh` quarantines what no longer compiles), run the gates, and read the coverage
report.

The library, the headers, both packages and the interface hash change together.

What fails loudly before any build: the checks of section 13. What fails at the Rust compile: a
generated arm whose rule no longer fits is quarantined with its error, and the release goes on
without it; a hand-written bridge that no longer implements its trait is a hard error. What
fails on the developer's machine: app code that used a removed method does not compile; a
library and a package from different generations refuse to start, naming both versions.

What needs a person: a new protocol family (a new delegate trait, a new kind of closure);
behaviour that changed under an unchanged signature (caught by the conformance pixels and the
examples); a rustdoc JSON format change (the reader is a few hundred lines); platform items.

How much moves between versions: between gpui-pre 0.3.12 and gpui-fast 0.1.14, 5,014
signatures were identical, none changed, none were removed and 58 were added.

## 18. Verification and the frame-rate gate

Correctness, all runnable without a display:

1. **Codecs:** every wire type and record is encoded by the package, decoded in Rust, and
   checked by the generated protocol tests (`core/tests/gen`: every handler shape fired once,
   every state type made, read, written, subscribed and released).
2. **The sample:** `gates/gensample.sh` builds a program that calls every generated Odin
   procedure and checks that the runtime accepts the tapes it writes.
3. **Conformance by generation:** for every bound element type a generated page calls its
   builders with fixed arguments, natively in Rust (`ref/`) and through the package. The diag
   library renders both offscreen (`HeadlessAppContext`, the real wgpu renderer and text
   system) and the images must be byte-identical. Rendering is exact on one machine and driver
   but not across machines, so reference and candidate are always rendered in the same run.
   The gallery (`gates/gallery.sh`) adds hand-written pages; `gates/verified.json` records
   which functions an identical page shows.
4. **Scripted interaction:** a script drives each example app and its Rust reference (click,
   press, type, drag); the pixels after each step must match (`gates/pixels.sh`).
5. **Protocol tests and attacks:** `core/tests` runs the runtime's protocols, and attacks on the
   C interface by a raw client (truncated, unknown and forged requests, stale handles, calls
   from another thread).
6. **Misuse:** stale handles, stale `Cx`, elements used twice, wrong thread, a panic on each
   side, a read of a leased entity, and the Odin and Go misuse cases (`gates/misuse.sh`,
   `gates/go-misuse.sh`).

**Frame rate.** Two builds that both keep up with the display both show its rate, so the pass
mark is defined on cost (main-thread CPU per frame), with presented frames as a floor.

- `gates/display.sh --hz N CMD` runs CMD in a private headless compositor with its own runtime
  directory and a D-Bus session without service files: `kwin_wayland --virtual` (60 Hz) or
  `gamescope --backend headless -r N` for other rates (240 gives a 4.166 ms frame interval,
  1,000 gives 1.000 ms). On a machine where no compositor can reach the GPU,
  `GK_DISPLAY=xvfb` uses a private Xvfb, which paces at 60 Hz only.
- Windows are opened with `inactive_frame_interval: None`, because the default caps an
  unfocused window at 30 frames a second.
- `gates/fps.sh [--hz N] SCENE ARGS` is the gate. Per scene it runs the Rust reference and the
  Odin app (at `-o:minimal` and `-o:speed`), driven by the same Rust driver module (`gk-meter`,
  linked into the library and into the reference), which injects input with
  `Window::dispatch_event`. It runs interleaved rounds, takes medians and p95, and runs the
  Rust reference a second time in every round to measure noise. A round whose two Rust runs
  differ by more than 5 % was disturbed by another job and is left out.
- `gk-meter` records main-thread CPU time per frame (`CLOCK_THREAD_CPUTIME_ID` on Linux,
  `GetThreadTimes` on Windows), frame intervals, crossings and time inside callbacks, tape bytes
  and decode time, and process memory. The diag build adds `views_built`, `views_reused` and
  build, prepaint and paint times (`Window::layout_stats`).
- **Pass mark** (choice 8), for every scene at every rate run:
  (a) no more missed frames than the Rust reference (a frame interval over 1.5 periods; 1 % of
  frames tolerated);
  (b) median and p95 main-thread CPU per frame at most 1.03 times Rust's, plus the
  Rust-against-Rust difference (or the standard error of its median, whichever is larger)
  measured in the same run;
  (c) the build judged is the Odin app at `-o:speed`, which is what an app ships; the
  `-o:minimal` build is measured and reported beside it;
  (d) 0 crossings on frames with no app-state change;
  (e) `views_built` per frame equal to the Rust reference's (diag build).
- Scenes: 60 panels of 64 labels with 0 and with 2 changing per frame (the shape of upstream's
  `views_frames`); the counter with a click on every frame; a 100,000-row table scrolled every
  frame. The Go front end is measured the same way (`make go-fps`).
- Memory: `gates/memory.sh` checks over long offscreen runs that the app's resident memory
  grows no more than the Rust reference's (2 MB allowance), and proves with `--selftest` that a
  small leak per view build is caught.
- Rebuild time: `gates/rebuild.sh` measures an Odin rebuild after a one-line edit, scaled
  against a fixed baseline measured in the same run, so it can be judged on any machine.

## 19. Limits and known risks

What the binding does not offer at Rust's level:

- custom elements beyond a described plot or paint list; IME for custom text views (the kit's
  inputs and editor handle text entry);
- web targets (Go's wasm has no cgo);
- a Rust panic ends the process: it is reported, not survived.

Known risks and open points:

1. **Synchronous handlers and leases.** The lease probe turns the known case into an error; a
   kit component that calls an app closure from inside its own update is found only by testing
   each component.
2. **Actions through one Rust type:** menu availability and key hints for app actions are
   coarse.
3. **Rows:** a far scrollbar jump shows placeholder rows for one frame.
4. **rustdoc JSON** is unstable and obtained with `RUSTC_BOOTSTRAP=1`; this is isolated behind
   the committed extract and a pinned rustc.
5. **macOS minimum.** The library says macOS 11.0, but gpui-fast's Metal shaders are compiled
   with a 12.0 minimum, so drawing on macOS 11 is untested; Go 1.27 needs macOS 13 or newer.
6. **High refresh rates** are measured under gamescope's headless output, not on a real panel.
   A run there sometimes receives no frame callback at all; the gates repeat such runs.
7. **Odin rebuild time** grows with the generated package, because Odin has no build cache.

What is bound, what is not yet, and why, is listed per component in [STATUS.md](STATUS.md) and
`api/coverage.json`.
