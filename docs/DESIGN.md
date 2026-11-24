# Architecture and Design

This document explains the architectural decisions behind `goui-kit`, the alternatives that were evaluated and rejected, the per-frame performance model (including how parity with pure Rust is measured by `gk-meter` and `gates/fps.sh`), the safety and lifetime invariants across the FFI boundary, and the end-to-end code generation pipeline.

---

## 1. Core Architectural Goals

The binding exposes the full GPUI + GPUI-kit desktop UI stack (over 600 element constructors, 1,600 builder ops, 250 builder trait ops, 1,500 state/object calls, and 3,100 style tokens) to Go applications under four non-negotiable constraints:

1. **Zero Rust toolchain and zero C compiler** required for default Go application builds.
2. **Frame-rate and memory parity** with an equivalent pure-Rust GPUI-kit application (verified within a tight margin by automated gates on real compositors).
3. **Complete API coverage** derived mechanically from upstream rustdoc metadata so new GPUI-kit releases do not require hand-porting hundreds of widgets.
4. **Hard safety guarantees** so any misuse in Go produces a clean Go error (`*gk.Error` or `*gk.PanicError`) with source location, never undefined behavior, silent misrendering, or an unhandled Rust panic unwinding across FFI.

Every major design choice in the repository follows directly from these four constraints.

---

## 2. Major Design Decisions and Rejected Alternatives

### 2.1 Seven C Functions + Binary Tape vs. Per-Widget FFI

**Decision**: Expose a fixed C ABI of 7 functions (`gk_info_get`, `gk_run`, `gk_headless_run`, `gk_call`, `gk_reply`, `gk_post`, `gk_last_error`) and one callback, and encode element trees into compact, little-endian byte buffers (**tapes**, format `GKT1`).

**Rejected alternatives**:

- **One C function per GPUI widget and builder method**: At GPUI-kit's API scale (thousands of constructors and style methods), building a typical 200-element view would require 500 to 1,500 FFI calls per redraw. Even with a fast C trampoline, crossing the boundary hundreds of times per frame adds tens of microseconds of overhead, balloons the exported symbol table, and breaks ABI compatibility whenever upstream adds or changes a method. A tape builds the entire view in pure Go memory (zero FFI calls during view construction) and crosses the boundary once as a flat byte slice.
- **JSON, Protobuf, or out-of-process IPC (pipes / sockets)**: Text parsing or schema framing per frame cannot match pure Rust at 240 Hz or 1,000 Hz. Out-of-process IPC also prevents synchronous in-turn state reads (`InputState::value`, `TableState::selected_row`) and native window/focus handle sharing.
- **Retained foreign-language mirror of the widget tree**: Mirroring GPUI's element hierarchy in Go would duplicate ownership, force fine-grained attribute diffing across FFI, and fight GPUI's own immediate-mode `Render::render` model. Instead, Rust owns the real GPUI entities (`GkView`, `InputState`, `ListState`, `TableState`, `DockArea`), and Go views simply re-describe their element tree when dirty.

### 2.2 Pre-Frame Turns and Declarative Protocols vs. Mid-Frame Callbacks

**Decision**: Foreign code **never** executes while GPUI is inside `Render::render`, layout, prepaint, or paint. All foreign code runs in discrete **turns** (`INIT`, `WINDOW_INIT`, `FRAME`, `VIEWS`, `EVENT`, `ROWS`, `WAKE`, `DONE`) executed either between frames in the pre-frame hook or in response to input events.

**Rejected alternatives**:

- **Calling back into Go synchronously from `ListDelegate::render_item`, `TableDelegate::render_td`, `Canvas::paint`, or `PopupMenu` builders**:
  1. **Re-entrant entity borrow panics**: During render and paint, GPUI holds mutable borrows on entities and window state. Invoking foreign code that might call `gk_call` back into GPUI can easily trigger fatal entity-lease or `RefCell` panics.
  2. **Per-cell FFI multiplication**: A 10-column by 40-row visible table viewport would cross the FFI boundary 400 times on every scroll frame.
  3. **Breaking view retention**: `gpui-fast` skips rebuilding unchanged views across frames; calling Go during paint would defeat view caching.

Instead, every upstream synchronous-draw callback maps onto one of five zero-mid-frame-callback protocols (specified in full in `docs/PROTOCOLS.md`):

| Upstream synchronous callback pattern | Binding protocol | Mid-frame crossings |
|---|---|---:|
| Lists, tables, trees, virtual lists (`render_item`, `render_td`) | **Row sources (`K_ROWS`)**: visible + overscan rows are bound as retained `GkView` entities in the pre-frame hook (`N_TABLE_CELLS` packs all cells of a table row into one tape). Far scrollbar jumps show placeholders for at most one frame. | 0 |
| Generic charts, scales, custom canvas plots | **Columnar `Datum` tables + described `N_PLOT` / `N_PLOT_PATH` / `N_PLOT_QUAD`**: row columns, `Field` names, `Format` templates, and paint primitives travel on the tape once per view build; Rust paints directly. | 0 |
| Overlays (`Dialog`, `AlertDialog`, `Sheet`, `PopupMenu`, `Tooltip`) | **View-built closures (`ViewBuild`) and described values**: upstream's closure replays ops from a retained `GkView` or tape fragment when the overlay opens or redraws. | 0 |
| Asynchronous or expensive draw lookups (syntax highlighters, LSP providers, `Ask`) | **Cached `Ask` channel (`SHAPE_ASK_EVENT`)**: cache misses paint a fallback for one frame and dispatch a queued request between frames; answers populate the cache and trigger a redraw. | 0 |
| Synchronous validators and predicates (`Questionnaire`, `Calendar`, `SearchableList`) | **Declarative rules & value cells**: `QuestionnaireRules`, `SEARCH_RULES`, calendar date sets, `Expr` strings, and `ValueCell` evaluate natively in Rust. | 0 |

### 2.3 View-Level Retained Tapes + Byte Diffing

**Decision**: Each `gk.View` maps 1:1 to a GPUI `Entity<GkView>` that stores an `Option<Rc<Tape>>`. Go diffs each newly encoded tape byte-for-byte (`bytes.Equal`) against the view's previous accepted tape before sending it to Rust.

**Why this matters**:

- **Zero-cost no-op redraws**: If an event handler marks a view dirty (or an observed state changes) without altering the resulting UI description, Go refreshes its local closure table in place (so closures capture updated values) and sends **zero bytes** to Rust, leaving GPUI's retained view untouched.
- **Fine-grained retention**: Apps can split dynamic areas into child views (`gk.NewView` embedded with `.ChildView(v)`). When only a child view is dirtied (via `gk.Notify` or a `gk.Model[T]`), parent views do not re-run in Go and stay retained in `gpui-fast`.

### 2.4 Default `purego` Dynamic Loading vs. Cgo-Only Builds

**Decision**: Default builds (`!gkstatic`) set `CGO_ENABLED=0` and load `libgouikit.so` / `libgouikit.dylib` / `gouikit.dll` at startup via `github.com/ebitengine/purego`. An opt-in `-tags gkstatic` build links `libgouikit_static.a` through cgo with linker dead-stripping (`--gc-sections` / `-dead_strip` / `ld.lld`).

**Rejected alternatives**:

- **Cgo-only**: Forces every Go user to install a C toolchain (and MinGW-w64 + `ld.lld` on Windows), breaks effortless `GOOS`/`GOARCH` cross-compilation, and slows iterative `go build` / `go test` loops.
- **Dynamic-only**: Prevents single-binary distribution and forces small utilities to ship every GPUI-kit component family (charts, code editors, docks). With `-tags gkstatic`, `go/gk/internal/rt/gen_use.go` calls `gk_use_<family>()` only for component families referenced by the Go binary, so the linker strips unused Rust decoders and components out of the executable.

---

## 3. Performance Model

### 3.1 Per-Frame Crossing Budget

The binding is structured so common UI workloads cross the Go<->Rust boundary zero or one time per frame:

| Workload | Crossings per frame | Mechanism |
|---|---:|---|
| Idle window (no state change) | **0** | `gpui-fast` retains every `GkView`; no pre-frame hook is scheduled. |
| Cursor blink, hover style, `N_ANIMATED` transitions, or scroll within bound overscan rows | **0** | Executed entirely inside GPUI / Rust. |
| Pure-Go click updating a view (for example, counter increment) | **1** | `K_EVENT` arrives with `flags & 1` (inline describe allowed). Go runs the click handler, runs `view`, writes the new tape into Section 4 of the same reply, and returns via `GK_FEAT_INLINE_REPLY` without calling `gk_reply`. |
| Steady scrolling of a large list or table past overscan | **1** (`K_ROWS`) | Pre-frame hook binds a batch (`PREFETCH_PER_FRAME = 24` for pure prefetch, or viewport + overscan on miss) and collects all new row tapes in a single `K_ROWS` turn. |
| Continuous Go-driven animation (`OnFrame`) | **1** (`K_FRAME`) | `K_FRAME` runs the Go callback and inline-describes dirty views in the same reply when `n_deferred == 0`. |

### 3.2 Go-Side Allocation and Tape Budget

Inside `go/gk/internal/rt/rt_emit.go` and `rt_turn.go`:

- **Reusable builder slab**: Each view build reuses a single `rt.Vb` (one `[]byte` ops arena + one `[]Node` slice).
- **Zero-pointer `Node` (`noscan`)**: `rt.Node` stores only integer offsets, opcodes, and build serials (no Go pointers), so the Go garbage collector marks the slab `noscan` and never traverses element trees during GC marks.
- **Zero-allocation interface boxing**: Every public element type (`gk.DivEl`, `kit.ButtonEl`, etc.) is a single-field struct wrapping `*rt.Node`. Converting it to `gk.Element` stores the pointer directly in the interface data word with zero heap allocation.
- **Write buffering**: State writes returning no value append to `t.DeferOp` and piggyback in Section 3 of the turn reply instead of making immediate `gk_call` trips. Whenever a subsequent synchronous read occurs in the same turn, `t.Flush` flushes pending writes first so program order is always preserved.

### 3.3 FFI Boundary Optimizations

Three low-level mechanisms close the remaining microsecond gap between pure Rust and Go:

1. **Inline reply (`GK_FEAT_INLINE_REPLY`, bit 3)**: Instead of calling `gk_reply(cb, ptr, len)` (a second FFI call plus buffer copy), the Go callback writes `(inline_ptr, inline_len)` directly into the first two pointer-sized words of `gk_cb`, pointing into Go's rooted reply buffer (`replyMem`). Rust reads that slice immediately after `callback` returns.
2. **Reflection-free `purego` callback trampoline (`rt_lib_dyn_fastcb.go`)**: By default, `purego.NewCallback` dispatches C->Go callbacks through `reflect.Value.Call`, allocating and reflecting on every turn. On Linux and macOS (`amd64` and `arm64`), `rt_lib_dyn_fastcb.go` probes a sentinel callback (`cbMarker`) at startup and, once verified, hooks `purego.callbackWrap_call` via `//go:linkname` to dispatch the fixed 5-argument `gk_callback` signature directly. Setting `GK_PUREGO_REFLECT=1` disables this bypass for benchmarking.
3. **Slow-cadence Go scheduler keep-alive (`rt_keepalive.go`)**: At 60 Hz or slower (inter-frame gaps between 10 ms and 250 ms), the OS-thread-locked UI goroutine spends more than Go's 10 ms `sysmon` threshold inside C (`gk_run`). Go's `sysmon` retakes its `P` and pushes it onto `sched.pidle`. When the next frame callback re-enters Go, `exitsyscallTryGetP` pops that `P` without incrementing `P.schedtick`; ~6 us later, `sysmon` wakes, observes `schedwhen + 10ms <= now`, and sends `SIGURG` (`preemptone`), forcing `stoplockedm()` (`futex_wait`, adding +10 to 12 us and voluntary context switches on every frame). `rt_keepalive.go` tracks turn intervals and, during steady 10 to 250 ms cadences, wakes a lightweight background goroutine ~550 us before the next expected frame (nudged 2 ms earlier when `GOMAXPROCS > 1`) to call `runtime.Gosched()`, advancing `P.schedtick` and leaving a fresh `P` at the top of `sched.pidle`. Main-thread overhead is ~20 ns with zero extra main-thread wakeups; at 120 Hz+ or while idle, the goroutine sleeps completely. Disable with `GK_NO_KEEPALIVE=1`; print statistics with `GK_KEEPALIVE_STATS=1`.

### 3.4 Rust-Side Decode Optimizations

- **`FastTree` (`core/src/tape.rs`)**: Static, handlerless view tapes that render a second time compile lazily into a pre-parsed `FastTree`, avoiding per-frame opcode dispatch on unchanged views while automatically invalidating whenever `RELEASE_EPOCH` changes or a new tape arrives.
- **`TableCell` pre-parse and `CurRow` cache (`core/src/tape.rs`, `core/src/shim_rows.rs`)**: `N_TABLE_CELLS` tapes pre-parse every column cell during `Tape::parse` into `Label` / `Text` / generic `Node` entries, and `GkTableDelegate` caches the active row across columns `1..N` (`CurRow`) alongside `ROWS_EPOCH` so multi-column tables do not re-borrow the session per cell.
- **Direct `C_ROWS_PUSH` fast path (`core/src/rt.rs`)**: If an `EVENT` reply contains only `C_ROWS_PUSH` deferred ops and GPUI's defer queue is empty, the library applies them immediately without scheduling `cx.defer`.

### 3.5 Anatomy of Three Representative Frames

Tracing the exact work on both sides of the boundary clarifies where time goes and why each optimization exists:

1. **Idle frame (60 Hz to 1,000 Hz, unchanged scene)**:
   - Rust: no pre-frame hook is armed; `gpui-fast` reuses every retained `GkView` (`views_built = 0, views_reused = N`).
   - Go: **0 crossings, 0 allocations, 0 CPU time**.
2. **Interactive click frame (`examples/go/counter`, 1 click updating 1 child view)**:
   - Rust: GPUI invokes the button's click closure -> `rt::fire` enters `gk_callback` with `K_EVENT` (`flags = 1`, inline describe enabled).
   - Go: `fastWrap` dispatches without reflection; handler increments `a.count` and buffers `C_ROWS_PUSH` (if any); because `flags & 1 != 0` and the controls view did not call `gk_call` on its prior build, Go immediately runs `controlsView` into its reusable `rt.Vb` slab (zero heap allocations for `Node`s), checks `bytes.Equal` against the prior tape, writes the new tape into Section 4 of `replyMem`, stores `(inline_ptr, inline_len)` into `gk_cb`, and returns `0`.
   - Rust: installs the new tape directly into `GkView` (marking only `controls` dirty), applies buffered `C_ROWS_PUSH` directly on the fast path, and GPUI rebuilds only the `controls` view while `root`, `echo`, and `list` remain retained. Total boundary crossings for the entire click-to-pixel cycle: **1**.
3. **Steady 100,000-row table scroll frame (`examples/go/table`)**:
   - Rust (during paint of frame `N`): `GkTableDelegate` reads already-bound `N_TABLE_CELLS` row tapes from `Source::rows`, caching `CurRow` across all 6 columns and using pre-parsed `TableCell::Label` entries. Any rows approaching the viewport edge are queued in `Source::ahead` via `table_bind_ahead`.
   - Rust (pre-frame hook of frame `N+1`): `Source::plan()` selects up to `PREFETCH_PER_FRAME = 24` keys nearest `ahead_anchor` (and evicts farthest keys past `capacity`), emitting one `K_ROWS` turn.
   - Go: binds all 24 rows, encodes all 24 `N_TABLE_CELLS` tapes into one reply buffer, and returns via `GK_FEAT_INLINE_REPLY`. Because none of those 24 rows drew a placeholder, `source.missed` is `false` and no extra redraw is forced. Total crossings: **1**, with **0** placeholder frames during steady scrolling.

### 3.6 Measuring Parity (`gk-meter`, `gates/fps.sh`, `gates/memory.sh`)

Performance is verified by automated gates rather than ad-hoc runs:

- **`core/meter/` (`gk-meter`)**: Linked identically into both `libgouikit` and the pure-Rust reference apps (`ref/`). Measures per-frame main-thread CPU time (`CLOCK_THREAD_CPUTIME_ID` on Unix, `GetThreadTimes` on Windows), inter-frame wall time, boundary counters (`callbacks`, `calls`, `callback_ns`, `decode_ns`, `tapes`, `tape_bytes`, per-turn-kind counts, `deferred_ops`, `rows_missing`, `placeholder_frames`), and in `diag` builds `gpui-fast`'s `Window::layout_stats()` (`views_built`, `views_reused`, `build_ns`, `prepaint_ns`, `paint_ns`). Also drives deterministic synthetic input (`GK_FRAMES`, `GK_CLICK`, `GK_SCROLL`, `GK_SIZE`).
- **`gates/fps.sh` + `gates/fps-summary.py`**: Runs matched Go and Rust binaries in interleaved rounds (`rust`, `go`, `rust2`) under a private headless compositor (`gates/display.sh`: `kwin_wayland` at 60 Hz, `gamescope` up to 1,000 Hz, or `xvfb` at 60 Hz). Any round where the two Rust baselines drift by more than 5% (`|rust2/rust - 1| > 0.05`) is dropped as host jitter. The gate passes when Go's median frame CPU time is within `GK_BAR` (default `1.03`, i.e., 3% of Rust) plus the measured Rust-vs-Rust noise floor (`max(|median - 1|, 1.2533 * MAD_sigma / sqrt(n))`).
- **`gates/counter.sh`**: Measures click-every-frame responsiveness and retained-view rebuild counts against the Rust counter reference.
- **`gates/memory.sh`**: Drives sustained open/close, scroll, and allocation/release loops and checks that RSS and live handle counts plateau without leaks (including a leak-detector self-test in full `make gates`).

---

## 4. Safety and Lifetime Model

A foundational rule of the binding is: **misuse in Go must always surface as a clean Go error with source location, never undefined behavior, silent misrendering, or a Rust abort.**

### 4.1 Thread Affinity and Context Validity (`*gk.Cx`)

- `gk.Run` locks its goroutine to the OS thread (`runtime.LockOSThread`) and records its goroutine identity (`uiG`) via `internal/goid` (direct `g` register read in assembly on `amd64`/`arm64`, `runtime.Stack` fallback elsewhere).
- Every public function taking `*gk.Cx` checks:
  1. `curG() == uiG` (stops immediately if called from another goroutine).
  2. `cx != nil && cx.serial == activeTurn.serial` (stops immediately if a stored `*gk.Cx` is reused after its callback returned or across nested turns).
  3. Turn-kind legality: state writes, `gk.Model.Set`, `gk.OpenWindow`, and `gk.Release` are rejected inside view builds (`K_VIEWS`, `K_ROWS`, inline describe).

### 4.2 Element Single-Build and Single-Parent Rules (`rt.Vb`)

Each `*rt.Node` stores the build serial of the `rt.Vb` that created it and a `used` bit:

- Reusing an element from a previous build or another view stops with `*gk.Error`.
- Attaching an element to two parents, attaching it to itself, or mutating it after attaching stops with `*gk.Error`.
- Setting `GK_DEBUG=1` disables write buffering (so every call executes at its own Go line) and records `runtime.Caller` stacks on every node creation for pinpointing element errors.

### 4.3 Four-Tier Handle Lifetime Model

Every handle wraps a 64-bit generation-checked identifier (`kind | 24-bit generation | 32-bit index` in `rt.HCore`):

1. **Explicit `gk.Release(cx, handle)`**: Immediately issues `C_RELEASE`, bumping the Rust slot generation and `RELEASE_EPOCH`, and marks the Go handle dead. Double release or use-after-release stops cleanly with `*gk.Error` (and if a released handle is somehow referenced in a tape, Rust's `shown` / `released_shown` triggers `rt::app_stop` -> `K_PANIC` with `is_app_stop = 1`).
2. **View ownership**: Resources created during a view's `init`, event handlers, or frame hooks belong to that view. Releasing the view releases its owned states, child views, sources, subscriptions, and overlays in reverse creation order. Dock panels own their content view unless `dock.KeepWhenClosed` is set.
3. **GC fallback (`runtime.AddCleanup` + `weak.Pointer`)**: Tracked kit states, library objects, and focus handles share a `*life` token. When Go's GC determines no copy of the handle remains reachable, the cleanup callback pushes the handle onto a concurrent queue and calls `gk_post`, releasing it on the UI thread during the next `K_WAKE` turn. Handles handed to Rust outside a view description (such as a panel inserted into a `DockArea`, or a handle pinned with `Held`) stay anchored in a Go map until Rust no longer holds them.
4. **Temporary objects, `OBJ_PIN`, and `GenPin`**:
   - Temporary `HK_OBJ` handles returned by reads, calls, or events are swept in the next frame's pre-frame hook unless retained (`gk.Retain` -> `C_OBJ_RETAIN`) or pinned to a view build (`C_OBJ_PIN`).
   - View handler closures stay alive in Go's generation table until Rust's `Arc<GenPin>` drops and reports `(view, gen)` in a subsequent turn's `retired` header.

### 4.4 Panic Boundary and Error Types

Neither Go panics nor Rust panics ever unwind across the `extern "C"` boundary:

- **Go panics**: Recovered at the callback wrapper, wrapped in `*gk.PanicError` (preserving original `Value`, `Unwrap()` for `error` values, and stack trace), followed by `C_QUIT` via `gk_call`. Once `gk_run` exits cleanly, `gk.Run` re-panics with `*gk.PanicError`.
- **Rust panics**: Every C export and GPUI closure runs inside `std::panic::catch_unwind` (`rt::guard`). A panic inside `gk_call` poisons the session and returns `GK_ERR_POISONED`; a panic outside `gk_call` poisons the session, delivers `K_PANIC` (`is_app_stop = 0`), and calls `quit_soon()`. Go returns `*gk.LibraryError` from `gk.Run`.
- **App contract violations**: Return `*gk.Error` from `gk.Run` with the offending Go file, line, and function. Nothing in `github.com/ibanks42/goui-kit` calls `os.Exit`, so deferred cleanup in `main` always runs.

All 92 misuse behaviors (runtime `*gk.Error` cases and compile-time type rejections) are verified by `gates/go-misuse.sh` and `examples/go/misuse*`.

---

## 5. End-to-End Code Generation Pipeline

Rather than hand-maintaining bindings for hundreds of upstream types, `gen/` (pure Python 3 standard library) drives both the Rust library and the Go module from a committed rustdoc JSON snapshot of the upstream crates.

```text
upstream crates (gpui-pre / gpui-fast, gpui-kit)
        │
        ▼  gen/extract.sh  (cargo +nightly rustdoc --output-format json)
        ▼  gen/gkgen.py strip
api/rustdoc/*.json.gz + docs.json.gz  (committed snapshot)
        │
        ├── api/overrides.toml    (generic [instantiate], [rows], [shims], exclusions)
        ├── api/opcodes.lock      (append-only wire numbers -> API_HASH)
        ├── api/clone.json        (tools/clone_scan.py: Clone library objects)
        └── api/quarantine.json   (items temporarily held back by gen/round.sh)
        │
        ▼  gen/gkgen.py gen
        ├── gen/gk_model.py + family plugins (gk_app, gk_lists, gk_values, gk_overlay, gk_editor, gk_items, gk_comp6)
        ├── gen/gk_rust.py + gk_fam.py ──► core/src/gen/{mod,codecs,nodes,calls,traits,...}.rs + gen/out/lines.json
        ├── gen/gk_go.py + gk_split.py ──► go/gk/{,base,kit,editor,table,overlay,dock,chart,motion,s,theme,palette}/gen_*.go
        │                                  + go/gk/internal/rt/gen_use.go
        └── gen/gk_apidoc.py           ──► docs/API-GO.md (generated index)
        │
        ▼  core/wire/gen.py (core/wire/ops.txt + api/opcodes.lock)
        ├── core/src/ops.rs
        ├── core/include/gk_ops.h + go/gk/internal/rt/include/gk_ops.h
        └── go/gk/internal/rt/gen_ops.go
        │
        ▼  gates/conformance.py + gates/accept.py --check
        ├── examples/go/gallery/conf.go (generated conformance gallery pages)
        └── api/coverage.json + docs/STATUS.md
```

### 5.1 Key Pipeline Properties

1. **Deterministic, fail-closed classification**: Every public upstream item must either match a generator rule, appear in `api/overrides.toml`, or be explicitly excluded with a documented reason. Unmatched signatures fail `make gen` immediately and write `gen/out/no-rule.md`.
2. **One-line-per-item Rust emission & automatic quarantine**: `gk_rust.py` puts every generated item on a single line and records `(file, line) -> item_id` in `gen/out/lines.json`. `gen/round.sh` runs `cargo check --release -p gk-capi` and automatically quarantines any item whose generated Rust fails to compile into `api/quarantine.json`, generating `gen/out/report.md` so upstream bumps remain incremental and diagnosable.
3. **Acyclic Go package partitioning (`gen/gk_split.py`)**: Places declarations into family packages (`gk`, `motion`, `table`, `editor`, `base`, `kit`, `chart`, `overlay`, `dock`, plus `s`, `theme`, `palette`), moving referenced types downward and constructors upward so imports form a strict DAG with zero cycles, and emitting `internal/rt/gen_use.go` per-family registration calls for static linker dead-stripping.
4. **Append-only ABI integrity**: `api/opcodes.lock` never reuses retired numbers, and `API_HASH` (64-bit FNV-1a over `ops.txt` + `opcodes.lock`) guarantees that a mismatched `libgouikit` and Go module reject each other on the very first handshake call (`GK_ERR_VERSION`).
### 5.2 Static Per-Family Dead-Stripping Architecture

Static Go binaries (`-tags gkstatic`) must not pay the binary-size cost of unused GPUI-kit subsystems (such as charts, docks, or the code editor) when an application only draws a few buttons and labels.

`gen/gk_fam.py` partitions generated Rust decoders and call ops into independent component families, each registering its dispatch hooks inside a dedicated `#[no_mangle] pub extern "C" fn gk_use_<family>()` function instead of one monolithic `match` table:

- **Dynamic library (`cdylib`)**: calls `gk_use_all()` once on load, activating every family.
- **Static library (`staticlib`)**: never calls `gk_use_all()` unless the app explicitly calls `gk.LinkAll()`. Instead, `gen/gk_split.py` generates `go/gk/internal/rt/gen_use.go` with per-family registration calls referenced only from the Go packages that use each family. Combined with `-Wl,--gc-sections` (Linux), `-Wl,-dead_strip` (macOS), or `-fuse-ld=lld -Wl,--gc-sections` (Windows), the system linker discards unreferenced Rust families completely.

### 5.3 Generator Rule Anatomy and `api/overrides.toml`

The generator stays predictable by keeping all domain-specific knowledge in small, auditable modules rather than scattering special cases:

- **`api/overrides.toml`**: declares generic instantiations (`[instantiate]` such as `ListState<GkListDelegate>`, `TableState<GkTableDelegate>`, `SelectState<GkSearchDelegate>`, `LineChart<Datum>`), delegate/tree shim hooks (`[rows]`), trait-to-bridge mappings (`[shims]`), and explicit exclusions for Rust-only APIs (raw pointers, allocator hooks, internal macros) with written reasons.
- **Family plugins (`gen/gk_app.py`, `gk_lists.py`, `gk_values.py`, `gk_overlay.py`, `gk_editor.py`, `gk_items.py`, `gk_comp6.py`)**: each plugin handles one protocol family's closure shapes and type descriptors, keeping `gk_model.py` focused on general Rust type classification.
- **Conformance gallery generation (`gates/conformance.py`)**: synthesizes `examples/go/gallery/conf.go` from the generated model so newly bound components automatically enter visual and runtime verification.

---

## 6. Verification Architecture

Every architectural claim in this document is backed by an automated gate in `gates/`:

| Claim | Enforcing Gate | How It Proves the Claim |
|---|---|---|
| Visual fidelity against pure Rust GPUI-kit | `gates/pixels.sh`, `gates/gallery.sh`, `gates/go-apps.sh` | Runs matched Go and pure-Rust (`ref/`) binaries offscreen through the identical `gk-meter` `HeadlessAppContext` driver and compares output PNGs byte-for-byte (or within strict pixel tolerances where subpixel rendering varies). |
| Complete protocol correctness | `gates/go-proto.sh`, `core/tests/go/run.sh`, `gates/go-gensample.sh` | Drives every turn kind, built-in and generated handler shape (`C_TEST_FIRE_SHAPE`), handle lifetime tier (`OBJ_TEST_TEMPS`, `VALUES_TEST`, `HANDLES_TEST`), row source bind/evict cycle, dock renderer, editor provider, and platform completion. |
| Hard safety on Go misuse | `gates/go-misuse.sh` | Executes 92 distinct misuse cases across `examples/go/misuse*` (stale `Cx`, off-thread calls, writes inside views, duplicate element attachment, released handles, bad UTF-8, bad chart columns, bad animations, compile-time type mismatches) and verifies exact `*gk.Error` diagnostics or compiler rejections. |
| Frame-cost parity with Rust | `gates/fps.sh`, `gates/counter.sh`, `gates/go-rebuild.sh` | Measures main-thread CPU time (`CLOCK_THREAD_CPUTIME_ID`) across interleaved `rust`, `go`, `rust2` rounds under `gates/display.sh` at 60/240/1,000 Hz, plus `views_built` vs `views_reused` retention counts. |
| No memory or handle leaks | `gates/memory.sh` | Runs sustained open/close, scroll, and create/release workloads and verifies RSS and live handle counts plateau, alongside a leak-detector self-test. |
| Portable builds & static stripping | `gates/cross.sh`, `make go-static`, `tools/dist-test.sh` | Cross-compiles `CGO_ENABLED=0` binaries for macOS and Windows from Linux, builds all major Go apps with `-tags gkstatic` to confirm per-family `gk_use_*` registration, and tests unpacked release archives end to end. |
| Documentation stays true | `gates/docs-go.sh`, `gates/readme-go.sh`, `gates/coverage.sh` | Verifies every Go code block in `README.md`, `GETTING-STARTED-GO.md`, `GO.md`, and `API-GO.md` matches a compiled, executed program line-for-line, and ratchets API coverage against `gates/coverage-baseline.json`. |
