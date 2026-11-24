# Wire and Runtime Protocols

This document is the complete specification for `libgouikit` (`core/`), the C ABI library that exposes GPUI and GPUI-kit to foreign front ends. A front-end author in any language (Go, Zig, C, Swift, Python, and so on) should be able to implement a conformant binding from this specification, `core/include/gouikit.h`, `core/wire/ops.txt`, and `api/opcodes.lock` without reading the Rust source.

The specification is organized from the lowest layer (C exports and byte primitives) up through turns, tapes, handles, and each domain protocol family, ending with error handling and a front-end implementation checklist.

---

## 1. Architecture, Versioning, and Opcode Numbering

### 1.1 Split Between Hand-Written and Generated Interfaces

The library consists of two halves that share one binary wire format:

1. **Hand-written core (`core/wire/ops.txt`)**: defines the ABI version, feature flags, turn kinds, error codes, handle kinds, built-in handler shapes, universal element ops, color encodings, and hand-written call ops / node types for runtime mechanisms that cannot be generated mechanically (row sources, editor providers, docks, plots, motion, overlays, platform completions, value cells).
2. **Generated bindings (`api/opcodes.lock`)**: generated from the upstream rustdoc JSON extract (`api/rustdoc/`) by `gen/gkgen.py`. Covers every generated element constructor (`node`), per-type builder method (`op`), builder trait method (`trait` + `trait-op`), state/object/ambient call (`call`), state constructor builder option (`state-op`), closure payload shape (`shape`), nullary style token (`style`), semantic theme color (`theme`), fixed palette color (`palette`), and enum discriminant (`variant`).

`core/wire/gen.py` reads `core/wire/ops.txt` and `api/opcodes.lock` and emits:

- `core/src/ops.rs` (Rust constants)
- `core/include/gk_ops.h` (C `#define` constants)
- `go/gk/internal/rt/gen_ops.go` (Go constants)

### 1.2 Opcode Numbering Ranges

All opcode tables are **append-only**. Retired numbers are never reused.

| Table | Hand-written range (`ops.txt`) | Generated range (`opcodes.lock`) | Wire width | Notes |
|---|---|---|---|---|
| Turn kind (`K_*`) | `1..=10` | none | `u32` (C parameter) | Fixed by C callback signature |
| Error code (`GK_ERR_*`) | `0..=10` | none | `i32` (C return) | `0` is success |
| Handle kind (`HK_*`) | `1..=9` | none | `u8` (bits 63..56 of `u64`) | Reserved slots: `10..=13` for future tracks |
| Handler shape (`SHAPE_*`) | `1..=63` | `64..` | `u16` | Built-in shapes use `1..=46` |
| Node type (`N_*`) | `1..=63` | `256..` | `u16` | `64..=255` reserved |
| Element op (`OP_*`) | `1..=255` (universal) | `256..=0x3fff` (per-type), `0x4000..=0xffff` (trait ops) | `u16` | Universal ops (`1..=4`) work on every node decoder |
| Call op (`C_*`) | `1..=255` | `256..` | `u16` | Hand-written blocks listed below |
| State builder op | none | `1..` (scoped per state type) | `u16` | Passed inside state constructor chains |
| Style token | none | `1..` | `u16` | Nullary methods on `Styled` and `StyledExt` |
| Theme color token | none | `1..=0x7fff` | `u16` | Bit 15 clear (`< 0x8000`) |
| Palette color token | none | `0x8000 | palette_num` | `u16` | Bit 15 set (`>= 0x8000`) |
| Enum variant | none | `0..` (scoped per enum) | `u16` | Some built-in protocol enums use `u8` where specified |

#### Reserved Hand-Written Blocks in `core/wire/ops.txt`

| Family | `call` range | `shape` range | `node` range | `hkind` |
|---|---|---|---|---|
| Core runtime & hand bridges | `1..=31` | `1..=13` | `7..=9` | `1..=7` |
| Input & editor providers | `32..=47` | `14..=15` | `10..=11` | - |
| Rows, lists, tables, trees | `48..=63` | `16..=19` | `12..=15` (`12..=14` retired) | - |
| App, windows, actions, platform | `64..=111` | `20..=27` | `16..=23` | `9..=10` |
| Library objects & handles | `112..=127` | `28..=31` | `24..=27` | `8` |
| Data tables & described plots | `128..=143` | `32..=35` | `28..=31` | - |
| Overlays, menus, docks | `144..=175` | `36..=43` | `32..=39` | `11` |
| Searchable lists, asks, editor host | `176..=207` | `44..=51` | `40..=47` | `12` |
| Value cells, pinning, motion | `208..=239` | `52..=59` | `48..=55` | `13` |

#### Trait Element Op Encoding

A builder trait (such as `Styled`, `ParentElement`, `InteractiveElement`, `StatefulInteractiveElement`, `StyledExt`, `Sizable`, `Disableable`, `Selectable`) is assigned a small positive `trait_num` (`1..=22` currently) in `api/opcodes.lock`, and each method of that trait is assigned a 0-based `method_idx` (`0..=127`). Its wire `u16` element opcode is:

```text
trait_elop = 0x4000 | (trait_num << 7) | method_idx
```

Wait: note that `0x4000 + (trait_num << 7) + method_idx` is identical since `method_idx < 128` and `trait_num < 384`. Decoders check `op >= 0x4000` (or match exact constants) and dispatch to the generic trait applier.

#### `api/opcodes.lock` File Format and `API_HASH`

`api/opcodes.lock` is a plain-text, tab-separated file (LF line endings, `#` comments ignored) with four columns per entry:

```text
<kind>\t<scope>\t<name>\t<number>
```

where `<scope>` is empty for global tables (`node`, `call`, `shape`, `style`, `theme`, `palette`, `trait`) or the Rust type/trait path for scoped tables (`op`, `trait-op`, `state-op`, `variant`).

`API_HASH` is the 64-bit FNV-1a hash (offset basis `0xcbf29ce484222325`, prime `0x00000100000001b3`) computed over the exact bytes of `core/wire/ops.txt` followed immediately by the exact bytes of `api/opcodes.lock`. Both the compiled library and every front end embed `API_HASH` and verify exact equality during startup.

---

## 2. C ABI and Handshake

The complete C interface is declared in `core/include/gouikit.h`. It contains 7 exported C functions (plus `gk_use_all` and per-family `gk_use_<family>` registration symbols).

### 2.1 Struct Layouts (`#[repr(C)]`)

All structs follow the platform C ABI with natural alignment. On 64-bit platforms (`LP64` and `LLP64`), both `gk_info` and `gk_app` are 40 bytes.

#### `gk_info`

| Offset (64-bit) | Size | C Field | Type | Description |
|---:|---:|---|---|---|
| 0 | 4 | `size` | `uint32_t` | `sizeof(gk_info)` (`48` on 64-bit). Checked for forward compatibility. |
| 4 | 4 | `magic` | `uint32_t` | `GK_MAGIC = 0x314B5047` (`"GPK1"` in little-endian bytes `47 50 4B 31`). |
| 8 | 2 | `abi_major` | `uint16_t` | Breaking ABI version (`0`). Must match exactly. |
| 10 | 2 | `abi_minor` | `uint16_t` | Additive ABI version (`6`). Library `abi_minor` must be `>=` front-end `abi_minor`. |
| 12 | 4 | *(padding)* | - | Alignment padding for `uint64_t`. |
| 16 | 8 | `api_hash` | `uint64_t` | FNV-1a hash of `ops.txt` + `opcodes.lock`. Must match exactly. |
| 24 | 4 | `features` | `uint32_t` | Bitmask of `GK_FEAT_*` flags. |
| 28 | 4 | *(padding)* | - | Alignment padding for pointer. |
| 32 | 8 | `versions` | `const char *` | Static UTF-8 build string (not NUL-terminated), e.g., `"gk 0.0.2; gpui-kit 0.11.0; gpui-fast 0.1.19"`. |
| 40 | 8 | `versions_len` | `size_t` | Byte length of `versions`. |

Total size on 64-bit platforms is 48 bytes. Front ends must always compute `sizeof(gk_info)` and `sizeof(gk_app)` from the target C struct layout rather than assuming 64-bit offsets on 32-bit architectures.

#### `GK_FEAT_*` Feature Bits

| Bit | Value | Constant | Meaning |
|---:|---:|---|---|
| 0 | `1` | `GK_FEAT_GPUI_FAST` | Built on `gpui-fast` (retained view caching). Clear when built on upstream `gpui-pre`. |
| 1 | `2` | `GK_FEAT_DIAG` | Diagnostic build: `gk_headless_run` is enabled and offscreen/scripted helpers are active. |
| 2 | `4` | `GK_FEAT_ALL` | Full dynamic library (`cdylib`) containing all component families. |
| 3 | `8` | `GK_FEAT_INLINE_REPLY` | `gk_cb` exposes `(inline_ptr, inline_len)` as its first two pointer-sized words so callbacks can reply without calling `gk_reply`. |

#### `gk_app`

| Offset (64-bit) | Size | C Field | Type | Description |
|---:|---:|---|---|---|
| 0 | 4 | `size` | `uint32_t` | Must equal `sizeof(gk_app)` (`40` on 64-bit). |
| 4 | 2 | `abi_major` | `uint16_t` | Front-end `GK_ABI_MAJOR`. Must equal library `abi_major`. |
| 6 | 2 | `abi_minor` | `uint16_t` | Front-end `GK_ABI_MINOR`. Must be `<=` library `abi_minor`. |
| 8 | 8 | `api_hash` | `uint64_t` | Front-end `GK_API_HASH`. Must equal library `api_hash`. |
| 16 | 8 | `callback` | `gk_callback` | Non-NULL function pointer invoked for every turn. |
| 24 | 8 | `user` | `void *` | Opaque context pointer passed back unchanged to `callback`. |
| 32 | 4 | `flags` | `uint32_t` | Reserved; must be `0`. |
| 36 | 4 | *(padding)* | - | Trailing struct alignment padding. |

#### Callback Signature and `gk_cb`

```c
typedef struct gk_cb gk_cb;
typedef int32_t (*gk_callback)(
    void          *user,
    gk_cb         *cb,
    uint32_t       kind,
    const uint8_t *req,
    size_t         len
);
```

The callback returns `0` (`GK_ERR_OK`) on normal completion. A nonzero return value is logged to stderr by `rt.rs` (`gk: callback kind <kind> returned <rc>`); it does not abort the process. To abort cleanly after a front-end panic or contract violation, the front end calls `C_QUIT` via `gk_call` and returns `0`.

When `GK_FEAT_INLINE_REPLY` (`8`) is advertised in `gk_info.features`, the first two words of `gk_cb` are guaranteed by the ABI to be:

```c
struct gk_cb_prefix {
    const uint8_t *inline_ptr;
    size_t         inline_len;
};
```

Instead of invoking `gk_reply(cb, buf, len)`, the callback may write `(buf, len)` directly into `cb->inline_ptr` and `cb->inline_len`. The pointed-to buffer must remain valid and immutable until the callback is next entered or `gk_run` returns (a single reusable front-end reply buffer satisfies this if re-entrant nested turns use a stack of buffers or copy out before issuing `gk_call`). If the callback also calls `gk_reply`, `gk_reply` clears `inline_ptr`/`inline_len` and replaces the reply with a copied buffer.

### 2.2 Exported C Functions

| Function | Signature | Threading & Re-entrancy Rules |
|---|---|---|
| `gk_info_get` | `const gk_info *gk_info_get(void)` | Safe from any thread, before or after `gk_run`. Returned pointer is static for the life of the process. |
| `gk_run` | `int32_t gk_run(const gk_app *app)` | Blocks until the application exits. The calling OS thread becomes the **UI thread** (lock it to one OS thread in green-threaded runtimes like Go). Fails with `GK_ERR_BUSY` if another run is active, `GK_ERR_VERSION` if handshake fails. |
| `gk_headless_run` | `int32_t gk_headless_run(const gk_app *app, const uint8_t *script, size_t len)` | Same contract as `gk_run`, using the offscreen test platform and executing `script` (UTF-8 lines). Available only when `GK_FEAT_DIAG` is set; returns `GK_ERR_UNSUPPORTED` in `ship` builds. Automatically enables scripted platform dialogs (`PLATFORM_ANSWER`). |
| `gk_call` | `int32_t gk_call(const uint8_t *req, size_t len, const uint8_t **res, size_t *res_len)` | UI thread only, and only while inside an active `gk_callback` (except during a `K_PANIC` turn). Writes result bytes to a thread-local buffer and stores its pointer and length in `*res` / `*res_len`; valid until the next `gk_call` on the UI thread. When the session is poisoned, refuses every call except `C_QUIT`. |
| `gk_reply` | `int32_t gk_reply(gk_cb *cb, const uint8_t *out, size_t len)` | UI thread only, and only with the exact `cb` pointer of the innermost currently active `gk_callback`. Passing a stale or outer `cb` returns `GK_ERR_NO_CONTEXT`. Copies `out[0..len]` into `cb`. |
| `gk_post` | `int32_t gk_post(uint64_t token)` | Safe from **any thread** while `gk_run` / `gk_headless_run` is running. Returns `GK_ERR_NO_CONTEXT` before `gk_run` starts or after it ends. Coalesces concurrent calls via an atomic `WAKE_PENDING` flag into a single `K_WAKE` turn on the UI thread (`token` is currently ignored on the wire; front ends keep their own thread-safe work queue). |
| `gk_last_error` | `const char *gk_last_error(size_t *len)` | Thread-local UTF-8 error message from the most recent failing C export on the calling thread. **Not NUL-terminated**; `*len` receives the exact byte length (`0` if none). Valid until the next failing export on the same thread. |
| `gk_use_all` | `void gk_use_all(void)` | Dynamic-library (`cdylib`) entry point that registers every generated component family into the dispatch tables. Dynamic loaders must call `gk_use_all()` once before `gk_run`. Static builds call per-family `gk_use_<family>()` functions instead so linkers dead-strip unused families. |

### 2.3 Startup Handshake Sequence

1. Load the library (or resolve static symbols).
2. If dynamically loaded (`GK_FEAT_ALL`), call `gk_use_all()` once.
3. Call `info = gk_info_get()`. Reject if:
   - `info == NULL`
   - `info->size < sizeof(gk_info)`
   - `info->magic != 0x314B5047` (`GK_MAGIC`)
   - `info->abi_major != EXPECTED_ABI_MAJOR`
   - `info->abi_minor < EXPECTED_ABI_MINOR`
   - `info->api_hash != EXPECTED_API_HASH`
4. Populate `gk_app` with `size = sizeof(gk_app)`, `abi_major`, `abi_minor`, `api_hash`, `callback`, `user`, `flags = 0`.
5. Lock the current OS thread (if applicable) and call `gk_run(&app)` (or `gk_headless_run(&app, script, len)`).
6. Never unwind a foreign exception or panic across `gk_callback` into Rust. Catch panics at the callback boundary, issue `C_QUIT` via `gk_call` (or return an empty reply after recording the panic), let `gk_run` exit cleanly, and re-raise or return the error in the host language.

---

## 3. Wire Primitives and Value Encodings

All multi-byte integers and IEEE-754 floats on the wire are **little-endian**, **packed** (no alignment padding), and **unaligned-safe** (read via byte copies, never cast pointers).

### 3.1 Scalar and Container Primitives

| Wire Type | Size (bytes) | Encoding |
|---|---:|---|
| `u8` / `i8` | 1 | Raw byte / two's complement |
| `bool` | 1 | `0` = false, any nonzero byte (`1` canonical) = true |
| `u16` / `i16` | 2 | Little-endian |
| `u32` / `i32` | 4 | Little-endian |
| `u64` / `i64` | 8 | Little-endian |
| `usize` / `isize` | 8 | Always widened to `u64` / `i64` on the wire (even on 32-bit hosts) |
| `f32` | 4 | IEEE-754 binary32, little-endian |
| `f64` | 8 | IEEE-754 binary64, little-endian |
| `char` | 4 | `u32` Unicode scalar value (`0` if invalid on decode) |
| `str` | `4 + N` | `u32` byte length `N` followed by `N` UTF-8 bytes (no NUL terminator). **Contract**: all strings sent to the library must be valid UTF-8; invalid UTF-8 is a protocol violation (front ends should validate before sending). |
| `bytes` | `4 + N` | `u32` byte length `N` followed by `N` raw bytes (not required to be UTF-8) |
| `opt<T>` | `1` or `1 + |T|` | `u8 0` for `None`; `u8 1` followed immediately by `T` for `Some(T)` |
| `list<T>` | `4 + sum(|T|)` | `u32` element count followed by elements back to back |
| `tuple(A, B, ...)` | `|A| + |B| + ...` | Fields concatenated in order with no framing |
| `Range<usize>` | 16 | `u64 start`, `u64 end` |
| `HashMap<K, V>` | `4 + sum(|K|+|V|)` | `u32` entry count, then `(K, V)` pairs sorted lexicographically by encoded key bytes for determinism |
| `Fallible<T>` | `2 + [|T|] + |str|` | `u8 ok` (`1` = Ok, `0` = Err), `u8 has_val` (`1` if `T` follows, `0` if omitted), optional `T`, `str error_message` (empty when `ok == 1`) |

### 3.2 Built-In Domain Value Types

#### Colors and Fills

Used wherever an element op or value accepts `Hsla`, `Color`, or `Fill`:

| Tag (`u8`) | Constant | Payload | Meaning |
|---:|---|---|---|
| `0` | `COLOR_HSLA` | `f32 h, f32 s, f32 l, f32 a` | Direct HSLA color (components in `0.0..=1.0`) |
| `1` | `COLOR_THEME` | `u16 token` | If `token < 0x8000`: field of active `ThemeColor` resolved at decode time. If `token >= 0x8000`: fixed palette color `(token & 0x7fff)` from `api/opcodes.lock` `palette` table. |
| `2` | `COLOR_GRADIENT` | `f32 angle_deg, u8 color_space, color stop0, f32 pct0, color stop1, f32 pct1` | Linear gradient (`angle_deg`: 0 = up, clockwise; `color_space`: `0` = sRGB, `1` = Oklab; each `stop` is a `COLOR_HSLA` or `COLOR_THEME`; `pct` in `0.0..=1.0`). Valid wherever GPUI accepts a `Fill` (e.g., `Styled::bg`) and in `PLOT_PATH` / `PLOT_QUAD`. |

*Note on record color fields*: When a color is read back as a standalone `Hsla` value (e.g., from a theme read), it is written as 4 `f32` (`h, s, l, a`). When a color is a field inside a bidirectional record whose host type carries a theme-token slot (`genapi::put_color_field`), the library writes `u16 0` followed by `f32 h, s, l, a`.

#### `Background` Read-Back

When reading back a GPUI `Background` (`core/src/values.rs`):

| Field | Wire Type | Description |
|---|---|---|
| `kind` | `u8` | `0` = Solid, `1` = LinearGradient, `2` = PatternSlash, `3` = PatternCheckerboard |
| `color_space` | `u8` | `0` = sRGB, `1` = Oklab |
| `solid` | `4 * f32` | Solid `Hsla` (`h, s, l, a`) |
| `angle_or_height` | `f32` | Gradient angle (degrees) or pattern height (px) |
| `stop0` | `4 * f32, f32` | Stop 0 `Hsla` + percentage |
| `stop1` | `4 * f32, f32` | Stop 1 `Hsla` + percentage |

#### Lengths and Dimensions

- `Pixels`, `Rems`: single `f32`.
- `Length` / `DefiniteLength`: `u8 kind` followed by `f32 value`:
  - `0` = pixels (`px(v)`)
  - `1` = rems (`rems(v)`)
  - `2` = relative fraction (`relative(v)`, where `1.0` = 100%)
  - `3` = `Auto` (`value` ignored, canonical `0.0`)

#### `ElementId`

| Tag (`u8`) | Payload | Meaning |
|---:|---|---|
| `0` | `str name` | Named ID (`ElementId::Name`) |
| `1` | `u64 n` | Integer ID (`ElementId::Integer`) |
| `2` | `str name, u64 n` | Keyed ID (`ElementId::NamedInteger`, Go `gk.Key(name, n)`) |

#### Time, Dates, and Clock

| Type | Wire Layout | Notes |
|---|---|---|
| `Duration` (outbound & `TimedTransformation`) | `i64 nanos` | Signed nanoseconds; negative clamped to `0` on input |
| `Duration` (in `TIMER_NEW` & `ANIMATED`) | `u32 millis` | Milliseconds |
| `Instant` | `i64 nanos` | Nanoseconds relative to the library's monotonic `OnceLock<Instant>` epoch (queried via `C_CLOCK_NOW`) |
| `jiff::civil::Date` | `i32 year, u32 month, u32 day` | Month `1..=12`, day `1..=31` |
| `jiff::civil::Time` | `u32 hour, u32 minute, u32 second, u32 nanosecond` | 24-hour clock |
| `jiff::civil::DateTime` | `Date` followed by `Time` | 28 bytes total |
| `jiff::civil::Weekday` | `u8` | `0` = Monday .. `6` = Sunday |

#### Fonts, Drag Values, Transformations, Expressions, and Format Templates

- **`Font`**: `str family, f32 weight` (`<= 0` means default weight 400), `u16 style` (`0` = Normal, `1` = Italic, `2` = Oblique).
- **`Drag`**: `u64 id, str text`. Fixed payload type backing GPUI's generic drag-and-drop API.
- **`Transformation`** (SVG): `f32 scale_x, f32 scale_y, f32 translate_x_px, f32 translate_y_px, f32 rotation_rad, f32 alpha`.
- **`TimedTransformation`** (SVG): `i64 period_ns`, `Transformation from`, `Transformation to`, `u8 scale_easing`, `u8 translate_easing`, `u8 rotate_easing`, `u8 alpha_easing` (`0` = Linear, `1` = Quadratic, `2` = EaseInOut, `3` = EaseOutQuint, `4` = Heartbeat), `u8 alternate`, `f32 max_fps` (`0` = unlimited).
- **`Expr`** (expression closures, Section 9.7): UTF-8 string evaluated by `core/src/shim_editor.rs`. Supports variables `x`, `y`, `z`, `a0..a9`, binary `+ - * / % == != < <= > >= && ||`, unary `- !`, ternary `cond ? a : b`, and 13 functions (`abs`, `min`, `max`, `clamp`, `floor`, `ceil`, `round`, `sqrt`, `pow`, `sin`, `cos`, `ln`, `exp`).
- **`Format`** (chart/plot format templates, Section 13.2): UTF-8 string with `{}` (first argument), `{0}`/`{1}` (positional argument), `{col}` (column of current `Datum`), format specs `{:,}` (thousands separator) and `{:.N}` (fixed decimal places), and `{{` / `}}` literal braces.

---

## 4. Turns and the Reply Envelope

All foreign code executes inside **turns** initiated by the library calling `gk_callback(user, cb, kind, req, len)` on the UI thread.

### 4.1 Common Reply Envelope

A callback may return an **empty reply** (`0` bytes, or no `gk_reply` call) when it has no return value, no dirty views, no deferred calls, and no tapes. Otherwise, the reply buffer has four sequential sections:

| Section | Wire Layout | Allowed Turn Kinds |
|---|---|---|
| 1. Return value | `u32 ret_len`, `ret_len` bytes | `EVENT` (shapes with non-unit return values such as `UNIT_BOOL`, `CLICK_BOOL`) |
| 2. Dirty views | `u32 n_dirty`, `n_dirty * u64 view_handle` | Any turn except `PANIC` |
| 3. Deferred calls | `u32 n_deferred`, `n_deferred * (u32 op_len, op_len bytes)` | `INIT`, `WINDOW_INIT`, `FRAME`, `EVENT`, `WAKE`, `DONE`. **Forbidden** in `VIEWS` and `ROWS` (causes `app_stop`). |
| 4. View tapes | `u32 n_tapes`, `n_tapes * (u64 view_handle, u32 gen, u32 tape_len, tape_len bytes)` | `VIEWS`, `ROWS`, and inline at the end of outermost `EVENT` (when `flags & 1 != 0`) or outermost `FRAME` (when `n_deferred == 0`). |

#### Deferred Call Scheduling Semantics

Deferred calls are packed call-op requests (exact same format as `gk_call`: `u16 op` + arguments) whose results are discarded:

- In `INIT`, `WINDOW_INIT`, `FRAME`, `WAKE`, and `DONE`: applied immediately after the callback returns.
- In `EVENT`: scheduled via `cx.defer` to run right after the event handler unwinds GPUI's entity borrow, **except** as a fast-path optimization when every deferred op is `C_ROWS_PUSH` and the defer queue is empty, in which case they apply immediately.
- If the front end executes a synchronous `gk_call` (for example, a read) during a turn that has pending buffered writes, the front end **must flush** its buffered writes via `gk_call` first so the read sees prior writes in program order.

#### Inline View Description (Single-Crossing Fast Path)

When an outermost `EVENT` arrives with `flags & 1 != 0` (meaning `wh != 0` and no outer turn is on the stack) or an outermost `FRAME` finishes with `n_deferred == 0`:

1. The front end may immediately run view functions for any dirty views that belong to `wh` and did not need to call into `gk_call` on their previous build (and have no un-flushed deferred writes).
2. The resulting tapes are placed directly into Section 4 (`n_tapes`) of the `EVENT` or `FRAME` reply, and those views are omitted from Section 2 (`n_dirty`).
3. The library installs those tapes directly into each `GkView` entity and calls `cx.notify()`, eliminating a separate `K_VIEWS` round trip on the same frame.

### 4.2 Complete Catalog of Turn Kinds (`K_*`)

| Code | Constant | Request Wire Layout | Reply Behavior & Notes |
|---:|---|---|---|
| `1` | `K_INIT` | *(empty, 0 bytes)* | Fired once when GPUI's app loop starts. Front end runs app startup code (typically creating root views and opening windows). `gk_post` wakeups queued during `INIT` drain afterwards. |
| `2` | `K_WINDOW_INIT` | `u64 wh, u64 root_view` | Fired once per opened window right after the GPUI window exists and before the first `K_VIEWS` for that window. Front end runs `root_view`'s `init` callback with the window in scope (required to create window-backed kit states like `InputState`, `ListState`, `DockArea`). |
| `3` | `K_FRAME` | `u64 wh, u64 frame_no, u32 n_pre_dirty, n_pre_dirty * u64 view, u32 n_retired, n_retired * (u64 view, u32 gen)` | Fired once per frame in the pre-frame hook when `C_ON_FRAME` is enabled. Front end drops retired handler tables, runs the app's frame hook, and may inline-describe dirty views if `n_deferred == 0`. |
| `4` | `K_VIEWS` | `u64 wh, u32 n_views, n_views * u64 view, u32 n_retired, n_retired * (u64 view, u32 gen)` | Fired before painting a frame whenever one or more mounted views are dirty or newly slotted without a tape. Front end drops retired handler tables, runs each requested view (plus any views dirtied during the turn? No: views must not write state), and returns changed tapes in Section 4. Unchanged tapes may be omitted from Section 4 (the library retains the previous tape). |
| `5` | `K_EVENT` | `u64 wh, u8 flags, u32 n_retired, n_retired * (u64 view, u32 gen), u64 view, u32 gen, u32 handler_id, u16 shape, <payload bytes>` | Fires a view handler (`view != 0`) or a kept handler (`view == 0`, where `handler_id` is the slot and `gen` is the slot generation). `flags` bit 0 (`1`) indicates inline view description is allowed. If the front end no longer holds `(view, gen, handler_id)`, it replies empty (ignored stale event). Return value (if `shape` expects one) goes in Section 1. |
| `6` | `K_ROWS` | `u64 wh, u32 n_sources, [u64 src, u32 n_bind, n_bind * (u64 key, u64 view), u32 n_evict, n_evict * u64 view] * n_sources, u32 n_pre_dirty, n_pre_dirty * u64 view, u32 n_retired, n_retired * (u64 view, u32 gen)` | Fired in the pre-frame hook when any row source needs rows bound or evicted. Front end drops retired handler tables, removes evicted row views, creates/binds `(key, view)` pairs, describes the newly bound row views and `pre_dirty` views, and returns their tapes in Section 4. Deferred calls are forbidden. |
| `7` | `K_WAKE` | *(empty, 0 bytes)* | Fired on the UI thread after one or more background `gk_post` calls. Runs with the first open window in scope (if any). Front end drains its posted closures and GC-queued handle releases. |
| `8` | `K_DONE` | `u64 view (always 0), u32 gen, u32 slot, u16 shape, <payload bytes>` | Delivers an asynchronous platform completion (`SHAPE_PATHS = 22` or `SHAPE_PROMPT = 23`). Front end invokes the kept completion handler once, removes it from its table, and releases the `HK_TASK` handle (buffered `C_RELEASE` in Section 3). |
| `9` | `K_QUIT` | *(reserved, never sent)* | Reserved constant in `ops.txt`. App shutdown is signaled via `C_ON_APP_QUIT` (which fires a `K_EVENT` of `SHAPE_UNIT`) and initiated via `C_QUIT`. |
| `10` | `K_PANIC` | `str message` *(or `str message, u8 is_app_stop`)* | Notification that the library caught a Rust panic (`is_app_stop` absent or `0`) or triggered an `app_stop` violation (`is_app_stop == 1`, e.g., drawing a released handle inside a tape). No reply is expected; `gk_call` is forbidden. The library schedules shutdown immediately after `K_PANIC` returns. |

### 4.3 Nested Turns and Pre-Frame Execution Order

Some synchronous `gk_call` operations can cause GPUI to emit an event synchronously before `gk_call` returns (for example, changing focus, submitting a form, or programmatically dispatching an action). This invokes `gk_callback` recursively while an outer turn is still on the stack.

Front ends **must** maintain a stack of active turns:

- Each nested turn receives a distinct turn serial (`Cx` validity token) and its own reply buffer and deferred-op buffer.
- `flags & 1` (inline description) is `0` on nested `K_EVENT` turns.
- When `GK_FEAT_INLINE_REPLY` is used, a nested turn must not overwrite the outer turn's pending reply slice (only the innermost active turn returns its reply slice, and the library consumes it before returning to the outer turn's `gk_call`).

#### Pre-Frame Hook State Machine (`core/src/rt.rs`)

Whenever a window has dirty views, enabled `C_ON_FRAME`, pending row-source binds/evictions, or pending temporary/pinned object sweeps, `rt::ensure_hook` installs a GPUI pre-frame hook on that window (`window.on_next_frame`). When GPUI prepares the next frame, the pre-frame hook runs strictly in this order **before** any `Render::render` or paint runs:

1. **Sweep expired objects**: releases every temporary `HK_OBJ` allocated in prior frames (`objects::sweep_temps`) and every `OBJ_PIN` handle dropped by a view's newer build or view release (`values::sweep_pins`).
2. **`K_FRAME` turn** (only if `C_ON_FRAME` is enabled): sends `wh`, monotonic `frame_no`, `pre_dirty` views, and `retired` handler generations. If the reply has `n_deferred == 0`, inline tapes in Section 4 are installed immediately. Any deferred ops are applied immediately.
3. **`K_ROWS` turn** (only if any row source attached to `wh` has `wanted` or `ahead` keys or needs eviction): runs `Source::plan()` for each source (see Section 9.1), allocates new `HK_VIEW` entities for newly bound keys, sends `K_ROWS` with `bind`, `evict`, `pre_dirty`, and `retired`, installs returned tapes, releases evicted row views, and notifies every watcher entity (`source::notify_watchers`) if `missed` was set.
4. **`K_VIEWS` turn** (only if any mounted views on `wh` remain dirty or newly slotted without a tape): sends `K_VIEWS`, installs returned tapes, and updates `deps` subscriptions and `slots` mounts.
5. **Re-arm hook if needed**: if `K_VIEWS` newly slotted a child view that has never been described, or if a row source still has remaining pure-prefetch rows (`source.has_ahead()`), another turn or next-frame hook is scheduled so no view is left blank.
6. **GPUI layout and paint**: GPUI calls `GkView::render` and delegate shims. No foreign code runs during this phase.

#### `GkView` Rendering and `FastTree` Caching

When GPUI calls `GkView::render` on a mounted view:

- If `tape` is `None`, it renders `div()` (empty placeholder).
- If the tape has `handler_count == 0`, `render_count >= 1` (second render of the same unchanged tape), and every node in the tape is eligible for fast rendering (basic layout containers `div`/`h_flex`/`v_flex`, `N_TEXT`, `OP_CHILD`, `OP_CHILD_VIEW`, `OP_TEXT`, `OP_STYLE`, and common styled properties without dynamic state reads), `GkView` compiles a compact `FastTree` and renders from it on subsequent frames.
- Whenever `C_RELEASE` runs, the global `RELEASE_EPOCH` bumps; `GkView` stores the `RELEASE_EPOCH` at which its `FastTree` was built and invalidates the cache if the epoch changed (ensuring a released child view or resource is never silently reused).

---

## 5. The Tape Format (`GKT1`)

A **tape** is a self-contained, little-endian byte buffer encoding a tree of UI elements (or a single value tree). Front ends emit tapes in a single forward pass with zero heap allocations per node; Rust decodes a tape into real GPUI elements when `GkView::render` runs.

### 5.1 Tape Header and Sections

Minimum header size is 28 bytes (for `n_deps = 0, n_slots = 0, ops_len = 0, node_count = 0`). Every valid tape has `node_count > 0` and `root < node_count`.

| Section | Wire Layout | Description |
|---|---|---|
| Magic | `u32` = `0x31544B47` (`"GKT1"` LE: `47 4B 54 31`) | Checked by `Tape::parse`; mismatch returns `GK_ERR_BAD_TAPE`. |
| `node_count` | `u32` | Number of node descriptors in the trailing node table. |
| `root` | `u32` | 0-based index into the node table of the root element (or value). |
| `handler_count` | `u32` | Number of view-scoped handlers registered for this tape generation (`0` if none). When `> 0`, Rust attaches an `Arc<GenPin>` to the tape so the front end is notified via `retired` once all closures drop. |
| Dependencies (`deps`) | `u16 n_deps`, `n_deps * u64 state_handle` | State handles read during the view build. Rust subscribes `cx.observe` on each entity so mutations automatically mark this view dirty. |
| Child slots (`slots`) | `u16 n_slots`, `n_slots * u64 view_handle` | Child view handles embedded via `OP_CHILD_VIEW`. Rust mounts them into the window and moves any unmounted-dirty views onto the window's dirty list. |
| Ops & args stream (`ops`) | `u32 ops_len`, `ops_len` bytes | Contiguous byte arena holding constructor arguments and linked builder-op records. **Offset `0` is the null sentinel** (meaning "no op / empty args"): front ends should reserve offset `0..4` as zeros (`[0, 0, 0, 0]`) so all real offsets are `>= 4`. |
| Node table | `node_count * 10` bytes | Packed array of 10-byte node records (see below). |

#### Node Table Entry (10 bytes each, indexed `0..node_count`)

| Offset | Size | Type | Field | Description |
|---:|---:|---|---|---|
| 0 | 2 | `u16` | `typ` | Node type opcode (`N_*`: `< 64` hand-written, `>= 256` generated constructor). |
| 2 | 4 | `u32` | `args_off` | Byte offset into `ops` where this node's constructor arguments begin (`0` if constructor takes no arguments). |
| 6 | 4 | `u32` | `first_op_off` | Byte offset into `ops` of the first builder op attached to this node (`0` if none). |

#### Linked Op Record Inside `ops`

Each builder method called on a node appends a record to `ops` and links it into the node's singly-linked list (appending at the tail preserves call order):

| Offset | Size | Type | Field | Description |
|---:|---:|---|---|---|
| 0 | 2 | `u16` | `opcode` | Universal op (`1..=4`), per-type op (`256..=0x3fff`), or trait op (`0x4000..=0xffff`). |
| 2 | 4 | `u32` | `next_op_off` | Byte offset of the next op on the same node, or `0` for the tail. |
| 6 | var | bytes | `args` | Packed little-endian operands for `opcode`. |

#### Handler Reference Operand

Whenever an element op or constructor takes a view-scoped event handler, the tape stores a single `u32 handler_id` (0-based index into this view generation's handler table). The library combines `(view_handle, tape_gen, handler_id)` into a `HandlerRef` and clones the tape's `Arc<GenPin>` into the GPUI closure.

### 5.2 Universal Element Ops (`elop`)

Every generated and hand-written element decoder supports these four opcodes:

| Opcode | Constant | Operands | Meaning |
|---:|---|---|---|
| `1` | `OP_CHILD` | `u32 child_node` | Attach node `child_node` (from the same tape's node table) as a child element (`ParentElement::child`). |
| `2` | `OP_CHILD_VIEW` | `u64 view_handle` | Embed another `GkView` as a child (`AnyView`). `view_handle` must also appear in the tape's `slots` header list so it is mounted and described before paint. |
| `3` | `OP_TEXT` | `str text` | Attach a text child (`ParentElement::child(SharedString)`). |
| `4` | `OP_STYLE` | `u16 count`, `count * u16 style_token` | Apply `count` nullary style tokens (from `api/opcodes.lock` `style` table) directly to the element's `StyleRefinement`. Batching tokens into one `OP_STYLE` avoids repeating the 6-byte op header per token. |

### 5.3 Value Tapes (`PutVtape`)

Some call ops accept complex builder values outside a view (for example `C_TABLE_COLUMN_VALUES`, `C_TABLE_LOOK` header/row styles, `C_VALUE_ACTION`, and generated calls taking builder structs such as `StyleRefinement` or `Column`). These are encoded as:

```text
u32 byte_len, <byte_len bytes of a complete GKT1 tape>
```

Value tapes set `handler_count = 0`, `n_deps = 0`, `n_slots = 0`, and point `root` at the value node.

### 5.4 Tape Runtime Optimizations (`FastTree` and `TableCell`)

Two transparent optimizations live in `core/src/tape.rs`; front ends do not emit special bytes for them, but knowing their invariants helps avoid defeating them:

1. **`FastTree`**: When a view tape has `handler_count == 0` and consists purely of `div` / `h_flex` / `v_flex` / `TEXT` / `OP_CHILD` / `OP_STYLE` / common styled ops without dynamic state reads, `GkView` lazily compiles the tape into a pre-parsed `FastTree` on its **second** render (avoiding compilation cost for one-shot tapes). `FastTree` is invalidated automatically if `RELEASE_EPOCH` changes or a new tape arrives.
2. **`TableCell` pre-parse**: When a tape's root node is `N_TABLE_CELLS` (`15`), `Tape::parse` immediately pre-parses each column cell into a fast enum (`Label`, `Text`, or generic `Node`) so `DataTable` renders hundreds of visible cells per frame without re-scanning the tape per column.

### 5.5 Worked Byte Examples

#### Example A: View Tape for `v_flex(s.P4).child(text("Hi"))`

Assume `N_V_FLEX = 257` (`0x0101`), `N_TEXT = 8` (`0x0008`), `style_token(p_4) = 120` (`0x0078`).

- Node 0 (`N_TEXT`): `args_off = 4` (`str "Hi"` = `02 00 00 00 48 69`, 6 bytes, ends at offset 10), `first_op_off = 0`.
- Node 1 (`N_V_FLEX`, root): `args_off = 0`, `first_op_off = 10`.
  - Op at offset 10: `OP_STYLE (4)`, `next = 20`, `count = 1`, `token = 120` -> `04 00  14 00 00 00  01 00  78 00` (10 bytes, ends at 20).
  - Op at offset 20: `OP_CHILD (1)`, `next = 0`, `child_node = 0` -> `01 00  00 00 00 00  00 00 00 00` (10 bytes, ends at 30).

Complete tape bytes (64 bytes total):

```text
Offset  Hex Bytes                                      Meaning
------  ---------------------------------------------  ----------------------------------------
0x00    47 4B 54 31                                    magic = "GKT1" (0x31544B47 LE)
0x04    02 00 00 00                                    node_count = 2
0x08    01 00 00 00                                    root = 1
0x0C    00 00 00 00                                    handler_count = 0
0x10    00 00                                          n_deps = 0
0x12    00 00                                          n_slots = 0
0x14    1E 00 00 00                                    ops_len = 30 (0x1E)
0x18    00 00 00 00                                    ops[0..4]: null offset sentinel
0x1C    02 00 00 00 48 69                              ops[4..10]: str "Hi" (len=2, 'H','i')
0x22    04 00 14 00 00 00 01 00 78 00                  ops[10..20]: OP_STYLE(4), next=20, n=1, tok=120
0x2C    01 00 00 00 00 00 00 00 00 00                  ops[20..30]: OP_CHILD(1), next=0, node=0
0x36    08 00 04 00 00 00 00 00 00 00                  node[0]: typ=8 (TEXT), args=4, first_op=0
0x40    01 01 00 00 00 00 0A 00 00 00                  node[1]: typ=257 (V_FLEX), args=0, first_op=10
```

#### Example B: `K_EVENT` Request and Inline-Describe Reply

Suppose the user clicks button handler `0` of view `0x0100_0001_0000_0001` (kind `1`, gen `1`, slot `1`), tape gen `3`, inside window `0x0200_0001_0000_0001`, at `(100.0, 20.0)` with left button, no modifiers, single click, mouse click.

Request (`kind = 5`, 55 bytes):

```text
Offset  Hex Bytes                       Meaning
------  ------------------------------  ----------------------------------------
0x00    01 00 00 00 01 00 00 02         wh = 0x0200_0001_0000_0001
0x08    01                              flags = 1 (inline describe allowed)
0x09    00 00 00 00                     n_retired = 0
0x0D    01 00 00 00 01 00 00 01         view = 0x0100_0001_0000_0001
0x15    03 00 00 00                     gen = 3
0x19    00 00 00 00                     handler_id = 0
0x1D    01 00                           shape = 1 (SHAPE_CLICK)
0x1F    00 00 C8 42  00 00 A0 41        position = (100.0f32, 20.0f32)
0x27    00                              button = 0 (Left)
0x28    00                              modifiers = 0
0x29    01 00 00 00                     click_count = 1
0x2D    00                              kind = 0 (Mouse)
```

Reply with inline re-description (say new tape gen `4`, 64 bytes):

```text
00 00 00 00                             ret_len = 0 (SHAPE_CLICK returns unit)
00 00 00 00                             n_dirty = 0 (already described inline)
00 00 00 00                             n_deferred = 0
01 00 00 00                             n_tapes = 1
01 00 00 00 01 00 00 01                 view = 0x0100_0001_0000_0001
04 00 00 00                             gen = 4
40 00 00 00                             tape_len = 64
<64 bytes of GKT1 tape>
```

#### Example C: `K_ROWS` Request (Bind 2 Rows, Evict 1 Row)

```text
01 00 00 00 01 00 00 02                 wh
01 00 00 00                             n_sources = 1
01 00 00 00 01 00 00 07                 src = 0x0700_0001_0000_0001 (HK_SOURCE)
02 00 00 00                             n_bind = 2
00 00 00 00 00 00 00 00                 key = (section=0, row=0)
05 00 00 00 01 00 00 01                 view handle for row 0
01 00 00 00 00 00 00 00                 key = (section=0, row=1)
06 00 00 00 01 00 00 01                 view handle for row 1
01 00 00 00                             n_evict = 1
02 00 00 00 01 00 00 01                 evicted view handle
00 00 00 00                             n_pre_dirty = 0
00 00 00 00                             n_retired = 0
```

The front end binds keys `0` and `1` to views `5` and `6`, drops view `2`, describes views `5` and `6`, and returns both tapes in Section 4 of the reply.

#### Example D: `StateBuilder` Constructor Call (`InputState::new().placeholder("Name")`)

Suppose `InputState::new` has call op `C = 310` (`0x0136`), takes no direct arguments (only `(window, cx)` on the Rust side), and its `state-op` `placeholder` is `3` (`0x0003`) taking `str`:

```text
Offset  Hex Bytes                       Meaning
------  ------------------------------  ----------------------------------------
0x00    36 01                           call op = 310 (InputState::new)
0x02    01 00                           n_state_ops = 1
0x04    03 00                           state_op = 3 (placeholder)
0x06    04 00 00 00 4E 61 6D 65         str "Name" (len=4)
```

Response (`*res`, 8 bytes):

```text
03 00 00 00 01 00 00 03                 HK_STATE handle (kind=3, gen=1, slot=3)
```

#### Example E: Columnar `Datum` Table Inside a Chart Node

Two rows with columns `("Category": TEXT, "Total": NUM)`: `("Food", 12.5)` and `("Bus", 3.0)`:

```text
Offset  Hex Bytes                                   Meaning
------  ------------------------------------------  ---------------------------------
0x00    02 00                                       n_cols = 2
0x02    01                                          col[0].kind = 1 (TEXT)
0x03    08 00 00 00 43 61 74 65 67 6F 72 79         col[0].name = "Category"
0x0F    00                                          col[1].kind = 0 (NUM / f64)
0x10    05 00 00 00 54 6F 74 61 6C                  col[1].name = "Total"
0x19    02 00 00 00                                 n_rows = 2
0x1D    04 00 00 00 46 6F 6F 64                     row[0][0] = "Food"
0x25    00 00 00 00 00 00 29 40                     row[0][1] = 12.5f64
0x2D    03 00 00 00 42 75 73                        row[1][0] = "Bus"
0x34    00 00 00 00 00 00 08 40                     row[1][1] = 3.0f64
```

---

## 6. Handles and Lifetimes

### 6.1 64-Bit Handle Bit Layout

Every handle handed out by the library is an opaque non-zero `u64` (`0` always means "none / unset"):

```text
Bit:  63       56 55                     32 31                             0
     +-----------+-------------------------+--------------------------------+
     | kind (8b) |    generation (24b)     |         slot index (32b)       |
     +-----------+-------------------------+--------------------------------+
```

- **`kind` (`(h >> 56) & 0xFF`)**: identifies the resource category.
- **`generation` (`(h >> 32) & 0x00FF_FFFF`)**: starts at `1`, increments on every slot release, wraps `0x00FF_FFFF -> 1` (never `0`). Prevents ABA reuse of stale handles.
- **`index` (`h & 0xFFFF_FFFF`)**: 1-based slot index (`0` reserved).

### 6.2 Handle Kinds (`HK_*`)

| Value | Constant | Underlying Rust Resource | Created By |
|---:|---|---|---|
| `1` | `HK_VIEW` | `Entity<GkView>` | `C_VIEW_NEW`, `K_ROWS` binds |
| `2` | `HK_WINDOW` | `AnyWindowHandle` | `C_OPEN_WINDOW`, `C_WINDOW_OPEN`, `C_WINDOWS` |
| `3` | `HK_STATE` | Any GPUI `Entity<T>` (kit states, `GkPanel`, `DockArea`, `AppMenuBar`, `Root`) | Generated state constructors, `C_INPUT_STATE_NEW`, `C_PANEL_NEW`, `C_DOCK_AREA_SKINNED`, `C_APP_MENU_BAR_NEW`, entity read-backs (`dock::entity_handle`) |
| `4` | `HK_ROWS` | Legacy `Rows` uniform-list store | `C_ROWS_NEW` |
| `5` | `HK_SUB` | `gpui::Subscription` | Generated `*_subscribe`, `C_INPUT_STATE_SUBSCRIBE`, `C_ON_ACTION`, `C_ON_WINDOW_CLOSED`, `C_ON_WINDOW_SHOULD_CLOSE`, `C_ON_APP_QUIT`, `C_DOCK_TOGGLE_SUBSCRIBE` |
| `6` | `HK_FOCUS` | `gpui::FocusHandle` | `C_FOCUS_HANDLE_NEW`, `FocusHandle` read-back |
| `7` | `HK_SOURCE` | `RowSource` (also backs `Ask`, editor providers, highlighters) | `C_SOURCE_NEW` |
| `8` | `HK_OBJ` | Non-entity Rust object (`Rc<RefCell<T>>` or `Global`) | Generated object constructors, read/call/event returns, `C_IMAGE_CACHE_NEW`, `C_RENDER_IMAGE_NEW`, `C_SCROLL_ANCHOR_NEW`, `C_TEXT_SCOPE_NEW`, `C_DOCK_RENDERER_NEW`, `C_CELL_NEW` |
| `9` | `HK_TASK` | Running timer or pending platform completion | `C_TIMER_NEW`, `C_PROMPT_PATHS`, `C_PROMPT_NEW_PATH`, `C_WINDOW_PROMPT` |

### 6.3 Object Handle Lifetimes (`HK_OBJ`)

Non-entity Rust objects behind `HK_OBJ` have three lifetime tiers:

1. **Kept objects**: returned by constructors (e.g. `ScrollHandle::new`, `C_CELL_NEW`, `C_RENDER_IMAGE_NEW`, `C_DOCK_RENDERER_NEW`). They remain live until explicitly freed with `C_RELEASE`.
2. **Temporary objects (`TEMPS`)**: returned by reads, calls, or event payloads (including `Clone` objects and lent values). They live until the next frame's pre-frame hook sweeps `TEMPS`, **unless**:
   - The app promotes the handle to kept with `C_OBJ_RETAIN (112)`, or
   - A view embeds the temporary handle in its tape and pins it with `C_OBJ_PIN (209)` (`u64 view, u32 build_no, u64 obj`). A pinned handle survives across frames as long as the view's latest build names it; when a subsequent build omits it (or the view is released), it moves to the next frame's sweep queue (`sweep_pins`).
3. **Global objects**: GPUI globals (`App::global::<T>()`) are interned once per `TypeId` and never swept.

### 6.4 Release, `RELEASE_EPOCH`, and `GenPin` Retirement

- **`C_RELEASE (12)`**: `u64 handle`. Releases the slot, increments its generation, drops the underlying Rust resource, and increments the global `RELEASE_EPOCH` counter (which invalidates any `FastTree` caches that may have referenced it).
  - Releasing a `HK_VIEW` also moves its `OBJ_PIN` entries to the sweep list and unmounts it.
  - Releasing a `HK_TASK` cancels the timer or marks a pending platform completion as discarded.
  - Releasing a `RenderImage` (`HK_OBJ`) calls `Window::drop_image` on every open window so GPU atlas textures are freed.
  - Calling `C_RELEASE` on an already-released or unknown handle returns `GK_ERR_STALE_HANDLE`.
- **Drawing a released handle**: If a tape references a handle that was already released (`genapi::shown`, `shown_focus`, `cell_shown`, `panel_view_shown`, `row_source` count), the library calls `rt::app_stop(...)` (sending `K_PANIC` with `is_app_stop = 1` and quitting) rather than silently substituting defaults.
- **Handler generation retirement (`GenPin`)**: Each tape with `handler_count > 0` creates an `Arc<GenPin { view, gen }>` cloned into every GPUI closure produced from that tape. When the tape is replaced and GPUI drops all closures from the old generation, `GenPin::drop` records `(view, gen)` in a thread-local retired list. That list is sent to the front end in the `retired` header of the next `VIEWS`, `FRAME`, `EVENT`, or `ROWS` turn so the front end can free the old handler closures.

---

## 7. Handler Shapes and Event Payloads

Every event handler is identified by `(view: u64, gen: u32, handler_id: u32)` and a `u16 shape`:

- **View handlers (`view != 0`)**: created during a view build; `gen` is the tape generation; `handler_id` is `0..handler_count`. Automatically retired via `GenPin`.
- **Kept handlers (`view == 0`)**: registered outside views (subscriptions, actions, timers, window hooks, row source events, dock panels, renderers, completions). Passed on the wire as `(u32 slot, u32 generation)` and kept by the front end until the owning resource is released (or until `K_DONE` fires once).

### 7.1 Built-In Event Shapes (`1..=46`)

| Shape | Constant | Request Payload (Library -> Front End) | Return Payload (Section 1) |
|---:|---|---|---|
| `1` | `SHAPE_CLICK` | `ClickEvent`: `f32 x, f32 y, u8 button` (`0` L, `1` R, `2` M), `u8 mods` (`1` ctrl, `2` alt, `4` shift, `8` platform, `16` fn), `u32 click_count, u8 kind` (`0` mouse, `1` keyboard, `2` touch) | *(empty)* |
| `2` | `SHAPE_UNIT_BOOL` | *(empty)* | `bool` (`u8`) |
| `3` | `SHAPE_UNIT` | *(empty)* | *(empty)* |
| `4` | `SHAPE_BOOL` | `bool` (`u8`) | *(empty)* |
| `5` | `SHAPE_USIZE` | `u64` | *(empty)* |
| `6` | `SHAPE_STR` | `str` | *(empty)* |
| `7` | `SHAPE_KEY` | `KeyEvent`: `str key, opt<str> key_char, u8 mods, bool is_held` (`false` on key-up) | *(empty)* |
| `8` | `SHAPE_CLICK_BOOL` | `ClickEvent` (same as `SHAPE_CLICK`) | `bool` (`u8`) |
| `9` | `SHAPE_MOUSE` | `MouseEvent`: `f32 x, f32 y, u8 button` (`0` L, `1` R, `2` M, `3` back, `4` fwd, `255` none on move), `u8 mods, u32 click_count` | *(empty)* |
| `10` | `SHAPE_F64` | `f64` | *(empty)* |
| `11` | `SHAPE_INPUT_EVENT` | Legacy hand-written input event (`u8 kind`, `0` change, `1` enter, `2` focus, `3` blur) | *(empty)* |
| `12` | `SHAPE_TEST` | `u64 a, str b` (protocol self-test) | `str` (optional) |
| `13` | `SHAPE_SOURCE_TEST` | `u64 req_id, str query` (row source request test) | *(empty)* |
| `14` | `SHAPE_PROVIDER` | `u64 req_id, u16 variant, ...` (Section 10.2) | *(empty)* |
| `16` | `SHAPE_LIST_EVENT` | `u64 req_id, u16 kind, opt<(u64 sec, u64 row, u64 col)>, bool secondary, str query` (Section 9.3) | *(empty)* |
| `17` | `SHAPE_TABLE_EVENT` | `u64 req_id, u16 kind, u64 row, u64 col, u64 to, u16 sort` (Section 9.4) | *(empty)* |
| `20` | `SHAPE_ACTION` | `u32 action_id` (bit 31 set for built-in kit actions) | *(empty)* |
| `21` | `SHAPE_WINDOW` | `u64 window_handle` | *(empty)* |
| `22` | `SHAPE_PATHS` | `u8 status` (`0` ok, `1` cancelled, `2` error), `list<str> paths, str error` (`K_DONE`) | *(empty)* |
| `23` | `SHAPE_PROMPT` | `u8 status` (`0` ok, `1` cancelled, `2` error), `u32 button_index` (`K_DONE`) | *(empty)* |
| `36` | `SHAPE_PANEL_EVENT` | `u8 kind` (`0` active, `1` zoomed, `2` added, `3` removed), `bool flag` | *(empty)* |
| `37` | `SHAPE_PANEL_BUILD` | `u64 panel, str name, PanelState, u64 dock_area` | *(empty)* |
| `38` | `SHAPE_DOCK_RENDER` | `u64 renderer, u8 hook, u64 key, u64 node, u16 placement, u16 axis, u8 flags, f32 size, u64 dock_ctx, u64 group_ctx, u64 panel, str panel_name` | *(empty)* |
| `39` | `SHAPE_DOCK_TOGGLE` | `u16 placement, u8 open` | *(empty)* |
| `44` | `SHAPE_SEARCH_EVENT` | `u64 req_id, str query` | *(empty)* |
| `45` | `SHAPE_ASK_EVENT` | `u64 req_id, u64 ask_source, <key bytes>` | *(empty)* |
| `46` | `SHAPE_LEAF_GEOMETRY` | `u32 n, n * (u64 leaf_key, u64 LeafGeometry_handle)` | *(empty)* |

### 7.2 Generated Event Shapes (`64..`)

Every generated closure parameter whose signature is not covered by a built-in shape gets an entry in `api/opcodes.lock` under `shape` (starting at `64`). Its payload is the concatenation of the closure's wire-encoded arguments (`Wire::put`), and its return value (if non-unit) is decoded via `Unwire::get` from Section 1 of the reply.

---

## 8. Core Call Ops and Generated State Calls

Every call op request begins with `u16 op` followed by op-specific arguments. Immediate calls go through `gk_call` and write their result into `*res`. Buffered calls return no value and may be batched in Section 3 of a turn reply (or sent immediately via `gk_call` when flushing or in debug mode).

### 8.1 Core Runtime Call Ops (`1..=26`)

| Op | Constant | Mode | Request (after `u16 op`) | Response | Description |
|---:|---|---|---|---|---|
| `1` | `C_VIEW_NEW` | Immediate | *(empty)* | `u64 view` (`HK_VIEW`) | Allocates a new `GkView` entity. |
| `2` | `C_OPEN_WINDOW` | Immediate | `f32 w, f32 h, u64 root_view, str title` | `u64 wh` (`HK_WINDOW`) | Opens a standard window (`WINDOW_OPEN` is the full-featured form). |
| `3` | `C_INPUT_STATE_NEW` | Immediate | `str placeholder` | `u64 state` (`HK_STATE`) | Legacy hand-written input state bridge (prefer generated `InputState`). |
| `4` | `C_INPUT_STATE_VALUE` | Immediate | `u64 state` | `str value` | Legacy input read. |
| `5` | `C_INPUT_STATE_SET_VALUE` | Buffered | `u64 state, str value` | *(empty)* | Legacy input write. |
| `6` | `C_ON_FRAME` | Buffered | `u8 enable` (`0`/`1`) | *(empty)* | Enables or disables `K_FRAME` turns in the pre-frame hook. |
| `7` | `C_QUIT` | Immediate | *(empty)* | *(empty)* | Requests clean GPUI shutdown (`cx.quit()`). Allowed even when poisoned. |
| `8` | `C_ROWS_NEW` | Immediate | *(empty)* | `u64 rows` (`HK_ROWS`) | Legacy string-row store for `N_UNIFORM_LIST`. |
| `9` | `C_ROWS_PUSH` | Buffered | `u64 rows, u32 n, n * str` | *(empty)* | Appends strings to a legacy `HK_ROWS` store. |
| `12` | `C_RELEASE` | Buffered or Immediate | `u64 handle` | *(empty)* | Releases any handle (`HK_*`), bumping slot generation and `RELEASE_EPOCH`. |
| `13` | `C_INPUT_STATE_SUBSCRIBE` | Immediate | `u64 state, u32 slot, u32 gen` | `u64 sub` (`HK_SUB`) | Legacy input subscription (`SHAPE_INPUT_EVENT`). |
| `16` | `C_FOCUS_HANDLE_NEW` | Immediate | *(empty)* | `u64 focus` (`HK_FOCUS`) | Creates a GPUI `FocusHandle`. |
| `17` | `C_FOCUS` | Buffered | `u64 focus` | *(empty)* | Focuses `focus` in the turn's window. |
| `19` | `C_INPUT_STATE_FOCUS` | Buffered | `u64 state` | *(empty)* | Legacy input focus. |
| `20` | `C_SOURCE_NEW` | Immediate | `u32 slot, u32 gen` (`0, 0` if no events) | `u64 src` (`HK_SOURCE`) | Creates a row source (Section 9.1). |
| `21` | `C_SOURCE_SET_COUNT` | Buffered | `u64 src, u32 n_sections, n_sections * u32 count, u32 overscan` | *(empty)* | Sets section counts and overscan; marks existing bound rows dirty if needed. |
| `22` | `C_SOURCE_SCROLL_TO` | Buffered | `u64 src, u32 row` | *(empty)* | Scrolls a `N_SOURCE_LIST` to `row`. |
| `23` | `C_SOURCE_ANSWER` | Immediate | `u64 req_id, u32 len, len bytes` | *(empty)* | Delivers an async answer for a pending source/provider/ask request. |
| `10, 11, 14, 15, 18, 24, 25, 26` | `C_TEST_*` | Immediate | Test-specific | Test-specific | Conformance and protocol self-test ops (`TEST_FIRE`, `TEST_FIRE_IN_UPDATE`, `TEST_PANIC`, `TEST_FIRE_VALUE`, `TEST_FIRE_QUEUED`, `TEST_SOURCE_REQUEST`, `TEST_SOURCE_TAKE`, `TEST_FIRE_SHAPE`). |

### 8.2 Generated Call Ops (`>= 256`) and Generated Codec Conventions

Generated call ops in `core/src/gen/calls.rs` and generated element decoders in `core/src/gen/nodes.rs` / `codecs.rs` follow deterministic wire rules emitted by `gen/gk_rust.py`. A front end can decode or encode any generated item solely from `api/rustdoc/`, `api/overrides.toml`, and `api/opcodes.lock` using these rules:

#### Call Op Patterns

1. **State / Object Constructors (`StateBuilder`)**:
   - Request: `<direct constructor args>, u16 n_state_ops, [u16 state_op, <op args>] * n_state_ops`
   - Response: `u64 handle` (`HK_STATE` for GPUI entities, kept `HK_OBJ` for plain Rust structs).
   - `state_op` numbers are scoped per state type in `api/opcodes.lock` under `state-op`.
   - Constructors that return `Result<Self, E>` upstream wrap their call in `overlay::Fallible`: on `Err(e)`, the library invokes `rt::app_stop` rather than returning a broken handle.
2. **State / Object Reads** (allowed inside views):
   - Request: `u64 handle, <method args>`
   - Response: `Wire::put(result)`.
   - Front ends **must** record `handle` in the active view's `deps` list whenever a `HK_STATE` read executes during a view build so GPUI `cx.observe` subscriptions re-trigger `K_VIEWS` on entity mutations.
   - Reads that return a GPUI `Entity<U>` emit `dock::entity_handle(&entity)` (`HK_STATE`), deduplicated in `HANDED` so repeated reads of the same live entity return the same handle.
   - Reads that return a `Clone` library object (`!records`) emit `genapi::obj_temp(v.clone())` (temporary `HK_OBJ` swept in the next pre-frame hook unless retained or pinned).
   - Reads that return `FocusHandle` allocate a new kept `HK_FOCUS` handle on each read.
3. **State / Object Writes**:
   - Request: `u64 handle, <method args>`
   - Response: empty (buffered in Section 3 of reply) or `Wire::put(result)` (immediate when the method returns a non-unit value).
   - Forbidden inside view builds (`K_VIEWS`, `K_ROWS`, inline describe).
   - Generated `state_update` / `state_update_cx` acquire the entity, run the closure, and call `cx.notify()`.
4. **State Subscriptions (`On*Event`)**:
   - Request: `u64 handle, u32 slot, u32 gen` (kept handler)
   - Response: `u64 sub` (`HK_SUB`).
   - Events fire as `K_EVENT` (`view = 0`, `gen`, `handler_id = slot`, generated `shape`).
5. **Ambient App / Window / Global Calls (`gen/gk_app.py`)**:
   - Static functions on `App`, `Window`, `Theme`, `Root::read(window, cx)` (`values::window_root`), or GPUI `Global` types. Take `<args>` directly and return `Wire::put(result)` (or empty when buffered).

#### Generated Data Type Wire Rules (`core/src/gen/codecs.rs`)

| Rust Category | Wire Encoding | Notes |
|---|---|---|
| Plain C-like enum (`enums`) | `u16 variant_num` | Discriminant from `api/opcodes.lock` `variant` table (0-based). Unknown discriminant returns `GK_ERR_BAD_ENUM`. |
| Data-carrying enum (`denums`) | `u16 variant_num`, followed by variant fields in declaration order | Unit variants have 0 trailing bytes; tuple/struct variants pack fields back to back. |
| Record / plain struct (`records`) | Fields packed sequentially in declaration order | No struct header or length prefix. |
| `impl Into<T>` union (`into_*`) | `u8 arm_index`, followed by the chosen arm's wire encoding | Arm order is deterministic (sorted by generator rule). |
| Builder value passed to a call op | `ValueTape` (`u32 len` + `GKT1` tape) | Decoded via `genapi::with_tape`. |
| Builder value passed to an element op | `u32 value_node` (index into the same tape's node table) | Decoded recursively via `build_<type>(dx, node)`. |
| Action listener (`on_action<A>`) | `u32 action_id, u32 handler_id` | Filters `GkAction` by `action_id`; unmatched actions bubble. |
| Boxed action listener (`Fn(&dyn Action, ...)`) | `u32 handler_id` (`SHAPE_ACTION = 20`) | Receives `u32 action_id` on fire. |
| Tooltip view closure (`Fn(&mut Window, &mut App) -> AnyView`) | `u64 view_handle` | Mounted and returned as `AnyView`. |
| Drag-and-drop (`on_drag`, `can_drop`, `drag_over`, `on_drop`) | `on_drag`: `Drag(u64 id, str text), u64 preview_view (0 = empty)`. `can_drop`: `DropAccept` (`list<u64> ids, u8 accept_external_files`). `drag_over`: `u32 StyleRefinement_node`. `on_drop`: `u32 handler_id` (payload: `Drag`). | Preview view `0` uses a shared lazy `EMPTY_DRAG` `GkView`. |
| `Result<(), E>` read-back (`overlay::outcome`) | `opt<str>` (`u8 0` = Ok, `u8 1 + str debug_err` = Err) | Used for unit-returning fallible calls. |
| `Result<T, E>` read-back (`Fallible<T>`) | `u8 ok, u8 has_val, [T], str err` | Used for value-returning fallible calls. |

### 8.3 Protocol Self-Test Call Ops (`core/src/calls.rs`, `core/src/hand.rs`)

These call ops are exposed in both `diag` and `ship` builds so front ends can run the conformance and protocol test suites (`examples/go/proto/`, `core/tests/go/`):

| Op | Constant | Request | Response | Purpose |
|---:|---|---|---|---|
| `10` | `C_TEST_FIRE` | `u64 view, u32 gen, u32 handler` | `i32` (`0`/`1`) | Fires handler synchronously with `SHAPE_UNIT_BOOL` and returns its bool result. |
| `11` | `C_TEST_FIRE_IN_UPDATE` | `u64 input_state, u64 view, u32 gen, u32 handler` | `i32` (`0`/`1`) | Fires `SHAPE_UNIT_BOOL` from inside `InputState::update` (verifying nested turn / borrow safety). |
| `14` | `C_TEST_PANIC` | *(empty)* | *(returns `GK_ERR_POISONED`)* | Triggers a deliberate Rust panic inside `gk_call` to verify session poisoning. |
| `15` | `C_TEST_FIRE_VALUE` | `u64 view, u32 gen, u32 handler, str text, u32 n` | `str` | Fires `SHAPE_TEST` (`(str, u32) -> str`) and returns the handler's string reply. |
| `18` | `C_TEST_FIRE_QUEUED` | `u64 input_state, u64 view, u32 gen, u32 handler, u64 row` | *(empty)* | Queues `SHAPE_USIZE` from inside `InputState::update` (fires after update unwinds). |
| `24` | `C_TEST_SOURCE_REQUEST` | `u64 src, str query` | *(empty)* | Starts a synthetic `SHAPE_SOURCE_TEST` request on `src` and stores its `oneshot::Receiver`. |
| `25` | `C_TEST_SOURCE_TAKE` | `u64 src` | `u8 ready, [bytes answer]` | Polls the synthetic `oneshot::Receiver` stored by `C_TEST_SOURCE_REQUEST`. |
| `26` | `C_TEST_FIRE_SHAPE` | `u64 view, u32 gen, u32 handler, u16 shape, bytes payload` | `u8 typed, bytes return_val` | Decodes `payload` through generated Rust types (if typed codec exists, `typed = 1`) and fires `(view, gen, handler)` with `shape`. |

---

## 9. Row Sources, Lists, Tables, Trees, Selects, and Closures

GPUI calls row-rendering closures (`render_item`, `render_td`, `UniformList` range builder) synchronously during layout/paint, when calling back into foreign code is forbidden. **Row sources** solve this by binding visible rows into real `GkView` entities **before** the frame paints (`K_ROWS` turn).

### 9.1 Row Source Lifecycle (`HK_SOURCE`, `core/src/source.rs`)

- **Row key encoding**: `key = ((section as u64) << 32) | (row as u64)`. For single-section lists, tables, and trees, `section = 0`, so `key == row`.
- **Maximum rows per section**: `MAX_ROWS = 0xffff_fffe`. The two highest 32-bit row numbers in any section are reserved for `GkListDelegate` section headers and footers:
  - `HEADER_ROW = 0xffff_fffe`
  - `FOOTER_ROW = 0xffff_ffff`
  - Header and footer keys are bound individually when requested and never expanded into an overscan range. Passing a count `> MAX_ROWS` returns `GK_ERR_BAD_OP`.
- **Overscan, Prefetch, and Eviction Algorithm (`Source::plan()`)**:
  1. Each source maintains `counts: Vec<usize>`, `rows: IdMap<(u64 view, Option<Rc<Tape>>)>`, `wanted: BTreeSet<u64>` (keys that rendered as placeholders during paint), `ahead: BTreeSet<u64>` (keys requested proactively via `table_bind_ahead`), `ahead_anchor: u64`, and `capacity: usize` (maximum simultaneously retained row views).
  2. When `wanted` is empty and `ahead.len() > 28` (`PREFETCH_PER_FRAME + 4`, where `PREFETCH_PER_FRAME = 24`), `plan()` sorts `ahead` by distance `k.abs_diff(ahead_anchor)` and takes only the nearest 24 keys so pure prefetch spreads smoothly across frames without spiking frame time. Otherwise all of `ahead` is taken at once.
  3. For each section present in `wanted`, let `[lo, hi]` be the min/max wanted row and `n` the number of wanted rows in that section. `plan()` computes `over = clamp(n, 8, max(capacity / 4, 8))` and expands the target set to `[lo.saturating_sub(over), min(hi + over + 1, count))`.
  4. `to_bind` is every target key not already in `rows`.
  5. If `rows.len() + to_bind.len() > capacity`, let `mid` be the median key of the target set. Existing keys not in the target set are sorted by descending `k.abs_diff(mid)` and the farthest `excess` keys are removed from `rows` and emitted in `to_evict`.
- **Watcher notification & `missed` flag**:
  - When a shim draws a row and hits a miss (`Row::Missing`), it sets `source.missed = true`, bumps `gk_meter` (`rows_missing`, and `placeholder_frames` once per frame), and arms the pre-frame hook.
  - When `K_ROWS` completes, the library calls `cx.notify(entity_id)` on every registered watcher (`source::watch`) **only if `missed` was true** (`source::take_missed`). Pure prefetch binds (`ahead`) that never drew a placeholder do not force redundant redraws.
- **Decoded vs. View-Mounted Sources (`source.decoded`)**:
  - Core `N_SOURCE_LIST` mounts each row's `GkView` directly as an `AnyView` child.
  - Delegate shims (`GkListDelegate`, `GkTableDelegate`, `tree_item`, `GkSearchDelegate`, `items::row`) set `source.decoded = true` and decode each row's `Rc<Tape>` inline inside the delegate closure (`source::row_tape`). Because those row entities are never rendered directly by GPUI, `rt::accept` stores the accepted tape straight into `Source::rows` without notifying the row entity.
- **Global `ROWS_EPOCH`**: Bumped whenever any source binds, evicts, or changes row count, allowing `GkTableDelegate` to skip redundant `rt::with` lookups while the bound set is unchanged.

### 9.2 Core List Nodes (`N_UNIFORM_LIST`, `N_SOURCE_LIST`, `N_VIRTUAL_LIST`, `N_TABLE_CELLS`, `N_ROW_MENU`)

| Node | Op | Args Layout | Description |
|---|---:|---|---|
| `N_UNIFORM_LIST` | `7` | `str id, u64 rows_handle` | Legacy uniform list over `HK_ROWS` string list; style ops follow. |
| `N_TEXT` | `8` | `str text` | Standalone text leaf node. |
| `N_SOURCE_LIST` | `9` | `str id, u64 source_handle, f32 row_height_px` | Core uniform list drawing each row's `GkView` (or empty `div().h(px(row_height))` on miss) with scroll tracked by `source.scroll`; style ops follow. |
| `N_VIRTUAL_LIST` | `16` | `str id, u64 source, u8 axis` (`0` vertical, `1` horizontal), `u32 n, n * (f32 width, f32 height)` | GPUI-kit `v_virtual_list` / `h_virtual_list` with per-item sizes; style ops follow. |
| `N_TABLE_CELLS` | `15` | `u32 n_cols, n_cols * u32 child_node` | Root node of a table row view holding every column's cell element in column order. Pre-parsed by `Tape::parse` into `TableCell`. |
| `N_ROW_MENU` | `38` | `u32 child_node, u32 menu_node` | Wraps a table, list, or tree row element (`child_node`) with a described `PopupMenu` (`menu_node`). During row draw, `menu_child` unwraps `child_node`; when `context_menu` opens, `row_menu` replays `menu_node`'s ops onto the upstream `PopupMenu`. |

### 9.3 Lists (`GkListDelegate`)

- Constructed via generated `ListState` constructor taking a `HK_SOURCE` (`[instantiate]` in `api/overrides.toml`). Reading back `ListState::delegate()` or `delegate_mut()` writes `u64 src` (`Wire for GkListDelegate`).
- **Extra delegate settings (`C_ROWS_DATA = 50`, buffered)**:
  - `u64 state` (list or table state)
  - `u8 flags`: bit 0 = `loading`, bit 1 = `has_more`, bit 2 = `visible_events`, bit 3 = `section_headers`, bit 4 = `section_footers`
  - `u64 load_more_threshold` (default `20`)
  - `u64 empty_view` (`0` = upstream default inbox icon; nonzero views are mounted into the source's window and marked dirty if undescribed)
  - `u64 loading_view` (`0` = upstream default 3-row skeleton)
  - `u64 initial_view` (`0` = none)
  - `f32 placeholder_height`
- **Events (`SHAPE_LIST_EVENT = 16`)**:
  - Payload: `u64 req_id, u16 kind, opt<(u64 section, u64 row, u64 col)>, bool secondary, str query`
  - Kinds: `0` = Select, `1` = RightClick, `2` = Confirm, `3` = Cancel, `4` = LoadMore, `5` = Search.
  - For `kind == 5` (Search), `req_id != 0`: `perform_search` returns a GPUI `Task<()>` that awaits `C_SOURCE_ANSWER(req_id, empty)` after the app updates counts.

### 9.4 Data Tables (`GkTableDelegate`)

A table's row source produces `N_TABLE_CELLS` tapes (one row view contains all columns' cells). Columns have stable app-assigned IDs (`0..n`) tracked in `order: Vec<usize>` (mapping visual position -> column ID) so reordering columns never scrambles cell lookup or `TABLE_TEXTS`. Reading back `TableState::delegate()` writes `u64 src`.

#### `ColumnSpec` Wire Layout (`C_TABLE_SOURCE_COLUMNS = 55`, `C_TABLE_SET_COLUMNS = 51`)

Both ops take `u64 handle` (source handle for `55`, table state handle for `51`), `u32 n_cols`, and `n_cols * ColumnSpec`:

| Field | Wire Type | Values / Notes |
|---|---|---|
| `key` | `str` | Column key (`SharedString`) |
| `name` | `str` | Header display name (`SharedString`) |
| `align` | `u16` | `0` = Left, `1` = Center, `2` = Right |
| `sort` | `opt<u16>` | `0` = Default, `1` = Ascending, `2` = Descending |
| `paddings` | `opt<(f32, f32, f32, f32)>` | `(top, right, bottom, left)` in px |
| `width` | `f32` | Initial width in px |
| `fixed` | `opt<u16>` | `0` = Left, `1` = Right |
| `resizable` | `bool` (`u8`) | Whether column width can be dragged |
| `movable` | `bool` (`u8`) | Whether column can be reordered |
| `selectable` | `bool` (`u8`) | Whether cells/column can be selected |
| `min_width` | `f32` | Minimum width in px |
| `max_width` | `f32` | Maximum width in px |

- **Full `Column` value tapes (`C_TABLE_COLUMN_VALUES = 114`)**: `u64 source, u64 table_state` (`0` = immediate before creation; nonzero = buffered update), `u32 n, n * ValueTape(Column)`. Allows passing any generated `Column` builder option not in `ColumnSpec`.
- **Cell text & accessibility labels (`C_TABLE_TEXTS = 177`, buffered)**:
  - `u64 state, u8 replace, u64 first_row, u32 rows, u32 cols, (rows * cols) * str` (row-major by column ID; empty string removes entry). Feeds `cell_text` (copying) and `cell_label`.
- **Row screen-reader labels (`C_TABLE_ROW_LABELS = 178`, buffered)**:
  - `u64 state, u32 n, n * (u64 row, str label)` (empty restores default).
- **Table visual overrides (`C_TABLE_LOOK = 179`, buffered)**:
  - `u64 state`
  - `u32 n_fit, n_fit * (u32 col_id, f32 fit_width)` (`fit_column` on resize-handle double click)
  - `u8 has_groups, u32 n_levels, n_levels * (u32 n, n * (str label, u64 span))`
  - `u32 n_group_views, n_group_views * (str label, u64 view_handle)` (`render_group_th`)
  - `u64 last_col_view` (`0` = upstream 12 px filler)
  - `ValueTape(StyleRefinement) header_style` (`u32 len = 0` for none)
  - `ValueTape(StyleRefinement) row_style` (`u32 len = 0` for none)
- **Events (`SHAPE_TABLE_EVENT = 17`)**:
  - Payload: `u64 req_id, u16 kind, u64 row, u64 col, u64 to, u16 sort` (`0` Default, `1` Ascending, `2` Descending).
  - Kinds: `0` = Sort, `1` = MoveColumn (`col` -> `to`), `2` = VisibleRows (`row..to`), `3` = VisibleColumns (`col..to`), `4` = LoadMore.
- **Row-tape caching**: `GkTableDelegate` caches the active row's `Rc<Tape>` in `CurRow` (`Empty | Missing(row) | Found(row, tape, view)`) cleared in `render_tr`, so columns `1..N` avoid repeated session lookups.

### 9.5 Trees

- **`C_TREE_SET_ITEMS (52)` (buffered)**: `u64 tree_state, u32 n, n * (str id, str label, u32 depth, u8 flags)` in depth-first preorder.
- **`C_TREE_ENTRY (53)` (immediate, allowed in views)**: `u64 tree_state, u64 index` -> `u8 present` (if `1`: `str id, str label, u64 depth, u8 flags` where bit 0 = folder, bit 1 = expanded, bit 2 = disabled). Reading `TREE_ENTRY` inside a row view records `tree_state` as a dependency so expanding/collapsing redraws affected rows.
- **`Tree::context_menu`**: Backed by `shim_rows::tree_menu(src)` reading `N_ROW_MENU` from the row's tape.

### 9.6 Searchable Lists, Selects, and Comboboxes (`core/src/shim_select.rs`)

`SelectState<GkSearchDelegate>`, `ComboboxState<GkSearchDelegate>`, `Select`, and `Combobox` are generated over `GkSearchDelegate`:

- **`GkItem` wire layout** (used in `C_SEARCH_ITEMS` and as a bidirectional generated value `libval`):
  - `str value` (unique identifier)
  - `str title` (shown in menu and trigger)
  - `opt<str> display` (if present, replaces `title` in the trigger)
  - `str keywords` (additional searchable text)
  - `u8 disabled` (`0`/`1`; shown but not selectable)
- **`C_SEARCH_ITEMS (176)`** (buffered, or immediate before state creation):
  - `u64 source, u8 flags` (bit 0 = row views draw items instead of default title, bit 1 = searches fire async `SHAPE_SEARCH_EVENT` requests), `u32 n_groups`, each group: `opt<str> title, u32 n_items, n_items * GkItem`.
  - Filtering runs locally in Rust (matching `title` and `keywords` like upstream `SearchableVec`). When `flags & 1 != 0`, each item's row key is `(section << 32) | original_item_index`, so a row keeps its view handle across filter changes.
- **`C_SEARCH_RULES (164)` (buffered)**:
  - `u64 source, u32 min_sel, u32 max_sel, list<str> locked, list<str> blocked`. Implements `SearchableListDelegate::on_will_change` synchronously inside Rust without crossing into foreign code.
- **`SHAPE_SEARCH_EVENT (44)`**: `u64 req_id, str query`. Front end calls `C_SEARCH_ITEMS` with filtered results and then `C_SOURCE_ANSWER(req_id, empty)`.

### 9.7 Data Closures (`Ask`), Context Closures (`CTX_GET`), Lent Objects, and Item Procedures

1. **Asks (`C_ASK_NEW = 180`, `SHAPE_ASK_EVENT = 45`, `core/src/shim_text.rs`)**:
   - Turns a `HK_SOURCE` into a thread-safe cached key-value channel (`ASKS: Mutex<HashMap<u64, Arc<Mutex<Cache>>>>`) fed by an unbounded `PUMP` channel on the main thread.
   - When upstream invokes a synchronous data closure `Fn(K) -> V` during draw, `Ask::get` looks up the wire-encoded `key: Vec<u8>` in `Cache::answers`.
   - On cache hit: clones and returns the decoded value immediately (zero crossings).
   - On cache miss: records `key` in `Cache::asked`, stores the type-specific decoder closure, pushes `(src, key)` onto `PUMP`, and returns `fallback()` for the current frame.
   - The main-thread pump task calls `source::request(src, SHAPE_ASK_EVENT, &(src, key_bytes))` between frames, emitting `SHAPE_ASK_EVENT`: `u64 req_id, u64 ask_source, <raw key bytes>`.
   - Front end computes the answer and calls `C_SOURCE_ANSWER(req_id, <encoded value bytes>)`.
   - The pump task decodes the answer inside `with_dx` (using a 34-byte minimal empty `GKT1` tape and the first open window so theme colors resolve), caches it under `key`, and refreshes all windows so upstream asks again and hits the cache.
   - `C_ASK_TEST (181)` -> `u64 gets, u64 keys_asked, u64 answers_cached`.
2. **Context closures (`C_CTX_GET = 182`, immediate, allowed in views, `core/src/source.rs`)**:
   - Used when upstream invokes a render closure carrying extra context values (for example, a calendar day's metadata or a time-field segment).
   - `source::ctx_key(src, explicit_key, fingerprint_bytes, writer_fn, window, cx)` assigns each call a row key (explicit `by` if provided, else monotonic `ctx_next` reset each frame) and compares `fingerprint_bytes` against the previous frame's stored fingerprint in `Source::ctxs`.
   - If the fingerprint changed, the library replaces the stored writer and marks that row view dirty before the next frame.
   - Inside the row view build, the front end calls `C_CTX_GET(u64 src, u32 section, u32 row)` -> the library invokes `writer_fn` and returns the encoded context values in their wire types.
3. **Lent objects (`C_LEND_TEST = 183`, `core/src/shim_editor.rs`)**:
   - When a kit state method takes `impl FnOnce(&mut Inner)`, Rust moves or clones `Inner`, hands it to the callback as a temporary `HK_OBJ` handle, and writes it back into the entity's slot when the callback returns (or applies `LentEdit` mutations). `C_LEND_TEST` -> `u64 lent, u64 returned`.
4. **Item procedures (`core/src/items.rs`, `N_KIT_VIEW = 41`)**:
   - Combines `source::ctx_key` with a thread-local `HANDED: RefCell<Vec<Option<Box<dyn Any>>>>` stack for upstream closures that hand the app a pre-built element (`CalendarItem`, `TimeFieldSegment`, `NavPage`, dialog close `Button`) or an `AnyView` (tooltip content) to decorate and return.
   - On the first frame before the row has a tape, `items::row` invokes `fallback(handed)` (returning upstream's element untouched).
   - Once the row has a tape, `items::row` installs `handed` into `HANDED` and decodes the row tape:
     - A generated node constructor `handed(u8 index)` takes `HANDED[index].take()` and downcasts it to `T`. Using the same handed element twice, or outside its own row, calls `rt::app_stop`.
     - `N_KIT_VIEW (41)` with args `u8 index` takes `HANDED[index]` as `AnyView` and embeds it as an `AnyElement`.
   - Context objects handed to item procedures are serialized via the wrapper `items::Reads<'a, T>` (never implementing `Wire` directly on the object type) so library objects keep their normal `HK_OBJ` handle encoding everywhere else.

---

## 10. Editor and Text Protocols

### 10.1 Styled Code Editor (`N_INPUT_EDITOR = 10`)

Upstream's styled `gpui_kit::component::input::Editor` (which renders completion, hover, and code-action popups) lives in a `doc(hidden)` module, so it is bridged by `N_INPUT_EDITOR`:

- **Args**: `u64 editor_state` (`HK_STATE`), `f32 height` (`0` = none), `u8 flags` (bit 0 = bordered, bit 1 = appearance, bit 2 = disabled, bit 3 = readonly). Followed by standard `Styled` ops.

### 10.2 LSP Providers (`C_EDITOR_PROVIDERS = 32`, `SHAPE_PROVIDER = 14`)

- **`C_EDITOR_PROVIDERS (32)` (buffered)**:
  - `u64 editor_state, u64 source` (`HK_SOURCE` carrying provider requests), `u32 which` bitmask:
    - `1` = completions
    - `2` = hover
    - `4` = go-to-definition
    - `8` = document colors
    - `16` = code actions
    - `32` = semantic tokens
    - `64` = inline completions
  - `str completion_trigger_chars`
  - `str code_action_provider_id`
  - `u32 inline_debounce_ms`
  - `list<str> semantic_token_types`
  - `list<str> semantic_token_modifiers`

- **`SHAPE_PROVIDER (14)` request payload**: `u64 req_id, u16 variant, ...`

| Variant | Name | Request Fields (after `u16 variant`) | Expected `C_SOURCE_ANSWER` Bytes |
|---:|---|---|---|
| `0` | Completions | `str text, u64 offset, u8 trigger_kind, opt<str> trigger_char` | `u32 n, n * (str label, u8 lsp_kind, opt<str> detail, opt<str> insert_text), u8 is_incomplete` |
| `1` | Hover | `str text, u64 offset` | `u8 has_hover` (if `1`: `str markdown, opt<(u64 start, u64 end)> range`) |
| `2` | Definitions | `str text, u64 offset` | `u32 n, n * (str uri, u32 start_line, u32 start_char, u32 end_line, u32 end_char)` |
| `3` | DocumentColors | `str text` | `u32 n, n * (u32 start_line, u32 start_char, u32 end_line, u32 end_char, f32 r, f32 g, f32 b, f32 a)` |
| `4` | CodeActions | `u64 start_offset, u64 end_offset` | `u32 n, n * (str title, opt<str> kind, str command_id)` |
| `5` | PerformCodeAction | `str title, str command_id, bool push_to_history` | *(empty; app applies edits via editor state calls)* |
| `6` | SemanticTokens | `str text, u64 start_offset, u64 end_offset` | `u32 n, n * u32` (LSP delta-encoded 5-tuples: `delta_line, delta_start, length, token_type, token_mods`) |
| `7` | InlineCompletion | `str text, u64 offset, bool automatic` | `opt<str> insert_text` |

### 10.3 Highlighters and Language Providers (`core/src/shim_editor.rs`)

- **`C_HIGHLIGHTER_NEW (184)` (immediate)**: `u64 styles_ask, u64 folds_ask` (`0` = none), `list<str> languages` (empty = matches any language, case-insensitive) -> registers a highlighter factory under `styles_ask`.
  - When an editor requests highlights for a byte range, `GkHighlighter` queries `styles_ask` with key `(str lang, str full_text, Range<usize>)`; uncached ranges draw unstyled until the app answers via `C_SOURCE_ANSWER`, then every window redraws.
  - Fold queries use `folds_ask` with key `(str lang, str full_text)`.
- **`C_LANGUAGES_NEW (185)` (immediate)**: `u64 handle` (syntax `Ask` or `HK_SOURCE`), `u8 has_syntax, list<(str alias, str canonical_name)>` -> registers a `LanguageProvider`. Aliases are normalized to lowercase; if `has_syntax != 0`, `syntax_context_provider` answers syntax-context queries (`(str lang, str full_text, u64 offset)`) through the `Ask` cache.

### 10.4 Markdown Plugins, Typed Fragments, Lazy Rows, and `EntityId` (`core/src/editor_text.rs`)

Upstream's Markdown and code-editor APIs require several specialized runtime shims:

1. **Typed fragments (`MainThread<Frag>`)**: Some upstream hooks (such as `MarkdownExtensions::block_surface`) require `Send + Sync` closures even though GPUI only calls them on the UI thread during draw. `MainThread<T>` stores the creating `ThreadId` and verifies `std::thread::current().id() == self.thread` on every access (aborting with exit 70 if ever called off-thread). Passing a fragment whose root node type does not match the expected type calls `rt::app_stop`.
2. **Windowless rows (`lazy_row`)**: When upstream calls a row render closure without a `&mut Window` (for example, `FoldPlaceholderRenderer`), the shim returns a custom `LazyRow` element that defers calling `source::ctx_row` until GPUI's `request_layout` phase, where `&mut Window` is available.
3. **View-returning closures (`ctx_view`)**: For closures returning `AnyView` per distinct value (such as `TextView::link_tooltip`), the shim binds one row view per distinct serialized value on a row source.
4. **Markdown plugins (`GkPlugin`)**: Because upstream parses Markdown on a background thread, app code cannot run during parsing. `GkPlugin` either wraps a built-in kit plugin (e.g., `FrontmatterPlugin`) or acts as a `DataPlugin` that claims Markdown nodes declaratively by kind (fenced code block language or JSX element name) during parsing, then draws each claimed node on the main thread as a row of a row source (inline nodes wrapped in `InlineElement`). The row view reads the node (and `InlineRenderContext` for inline nodes) via `C_CTX_GET`.
5. **Small values (`entity_id`, `Wire for ElementId`)**: Where an editor API takes a `gpui::EntityId`, the wire carries the `u64` state handle of the underlying `InputState` / `Textarea` / `EditorState` and extracts `entity.entity_id()`. Reading back an `ElementId` writes its display string (`str`).

### 10.5 Block Editor Host (`N_EDITOR_HOST = 40`, `SHAPE_LEAF_GEOMETRY = 46`, `core/src/editor_text.rs`)

- **`N_EDITOR_HOST (40)` args**:
  - `str id` (`ElementId::Name`)
  - `u32 n_blocks`, `n_blocks * BlockNode` (generated value codec, capped at 4096 prealloc)
  - `u32 n_flows`, `n_flows * opt<u32 FlowPosition_node>`
  - `opt<u32 RenderContext_node>`
  - `u32 n_decorations`, `n_decorations * (u64 leaf_key, <foreground ranges>)`
  - `opt<u32 handler_id>` (view handler of `SHAPE_LEAF_GEOMETRY = 46`; keeps `dx.pin()`)
- **`SHAPE_LEAF_GEOMETRY (46)`**: fired after `BlockHost` prepaints, carrying `u32 n, n * (u64 leaf_key, u64 LeafGeometry_handle)` (temporary `HK_OBJ` handles swept on the next frame unless retained).
- **Test ops**:
  - `C_EDITOR_TEST (186)` -> `u64 md_plugin_claims, u64 view_rows`
  - `C_EDITOR_HOST_TEST (187)` -> `u64 hosts_decoded, u64 leaves_collected, u64 leaf_events, u64 decorators_run, u64 lent_edited`.

---

## 11. Overlays, Actions, Menus, Timers, Windows, and Platform Services

### 11.1 View-Built Overlays and Described Values (`core/src/overlay.rs`)

GPUI-kit dialogs, alert dialogs, sheets, popovers, hover cards, context menus, and dropdown menus accept builder closures `Fn(T, &mut Window, &mut App) -> T` invoked lazily when the overlay opens or redraws.

- **View-built closures (`ViewBuild`)**:
  - Call ops such as `open_dialog` / `open_alert_dialog` / `open_sheet` take a `u64 view_handle` (`HK_VIEW`).
  - `ViewBuild::take` verifies the handle, mounts it into the turn's window (or the first open window if applied deferred), and marks it dirty so its first tape arrives before paint.
  - Each time GPUI calls the overlay builder closure, `ViewBuild::apply` reads the view entity (`entity.read(cx)`, which automatically records a GPUI dependency so re-describing the view redraws the open overlay) and checks that the root node's `typ` is in the expected set of node types for `T`. A wrong root type calls `rt::app_stop`; before the first tape arrives, upstream's unmodified `v` is returned for at most one frame.
- **Described values (`PopupMenu`)**:
  - A described value has no standalone constructor (`overlay::described<T>()` calls `app_stop` if used outside its owning dropdown/context/submenu op).
  - Instead, the front end emits a node for the menu in the tape; `dx.frag(menu_node)` captures the tape `Rc` + `GenPin` + node index, and replays `apply_component_popup_menu(dx, menu, first_op)` whenever GPUI opens the menu.
  - Submenus (`overlay::submenu`) construct child `PopupMenu::build` instances eagerly inside the parent menu build and attach them as `PopupMenuItem::submenu`.
- **Overlay nodes**:
  - `N_DROPDOWN_BUTTON (32)`: `u32 button_node, u32 menu_node` (rebuilds `PopupMenu` from `menu_node` each time the dropdown opens).
  - `N_DROPDOWN_ANCHOR (34)`: `u32 button_node, u32 menu_node, u16 anchor`. `anchor` enum: `0` = TopLeft, `1` = TopRight, `2` = BottomLeft, `3` = BottomRight, `4` = TopCenter, `5` = BottomCenter, `6` = LeftCenter, `7` = RightCenter.
  - `N_KBD_FOR_ACTION (35)`: `u32 action_id, u8 how` (`0` = `binding_for_action`, `1` = `binding_for_action_in`, `2` = `global_binding_for_action`), `opt<str> context, u64 focus_handle`. Renders `Empty` if the action has no binding.
  - `N_APP_MENU_BAR (36)`: `u64 app_menu_bar_state`.
  - `C_APP_MENU_BAR_NEW (155)` (immediate): *(empty)* -> `u64 state` (`HK_STATE`).
- **Tooltip view closures**: Any `Fn(&mut Window, &mut App) -> AnyView` parameter takes a `u64 view_handle` directly (`["viewany"]` descriptor).

### 11.2 Actions, Key Bindings, and Event Propagation (`core/src/app.rs`)

Actions are identified by a `u32 action_id`. App-defined actions have bit 31 clear (`0..0x7fff_ffff`); built-in GPUI-kit action values registered via `C_VALUE_ACTION (208)` have bit 31 set (`0x8000_0000 | idx`).

| Op | Constant | Mode | Request | Response | Notes |
|---:|---|---|---|---|---|
| `64` | `C_ACTION_NEW` | Immediate | `str name, str json_data` (`""` = none) | `u32 action_id` | Registers a named app action (`GkAction`). |
| `65` | `C_BIND_KEYS` | Buffered | `u32 n, n * (str keystrokes, u32 action_id, str context)` | *(empty)* | Binds GPUI keystrokes (e.g., `"ctrl-n"`, `"cmd-s"`); empty `context` means global. |
| `66` | `C_ON_ACTION` | Immediate | `u32 action_id, u32 slot, u32 gen` | `u64 sub` (`HK_SUB`) | Installs an app-wide action listener (`SHAPE_ACTION` or `SHAPE_UNIT`). |
| `67` | `C_DISPATCH_ACTION` | Buffered | `u32 action_id` | *(empty)* | Dispatches to focused element in turn's window, or app-wide. |
| `68` | `C_ACTION_AVAILABLE` | Immediate | `u32 action_id` | `u8` (`0`/`1`) | Checks if any listener on focus path or app-wide handles `action_id`. |
| `69` | `C_ACTION_KEYSTROKES` | Immediate | `u32 action_id, str context` (`""` = any) | `opt<str>` | Highest-precedence keystroke binding string. |
| `70` | `C_CLEAR_KEY_BINDINGS` | Buffered | *(empty)* | *(empty)* | Clears all key bindings. |
| `95` | `C_STOP_PROPAGATION` | Immediate | *(empty)* | *(empty)* | Calls `App::stop_propagation()`; valid inside an event handler turn. |
| `208` | `C_VALUE_ACTION` | Immediate | `ValueTape(Action)` | `u32 action_id` | Converts a GPUI-kit `Action` value node into an `action_id` (bit 31 set). |

*Element-scoped action listeners*: `on_action` / `capture_action` element ops encode `(u32 action_id, u32 handler_id)` on the tape; Rust filters `GkAction` by `action_id` and lets unmatched actions continue bubbling.

### 11.3 Timers and Clock

| Op | Constant | Mode | Request | Response | Notes |
|---:|---|---|---|---|---|
| `71` | `C_TIMER_NEW` | Immediate | `u32 millis, u8 repeat` (`0` once, `1` repeating), `u32 slot, u32 gen` | `u64 task` (`HK_TASK`) | Fires kept handler with `SHAPE_UNIT`. Releasing `task` cancels the timer. |
| `159` | `C_CLOCK_NOW` | Immediate | *(empty)* | `i64 nanos` | Monotonic nanoseconds since the library's start epoch. |

### 11.4 Windows, Quit Hooks, Clipboard, and Reduced Motion

| Op | Constant | Mode | Request | Response | Notes |
|---:|---|---|---|---|---|
| `72` | `C_WINDOW_OPEN` | Immediate | `f32 w, f32 h, u64 root_view, str title, u8 flags` (`1` not resizable, `2` not minimizable, `4` not movable, `8` no focus, `16` hidden, `32` kit title bar), `f32 min_w, f32 min_h` (`0` none), `u8 kind` (`0` normal, `1` popup, `2` floating), `u8 has_pos, f32 x, f32 y, str app_id, bytes icon_png` | `u64 wh` (`HK_WINDOW`) | Opens a configured window. `app_id` sets Wayland `app_id` / X11 `WM_CLASS`; `icon_png` is raw PNG bytes (`len 0` = none). |
| `73` | `C_WINDOW_CLOSE` | Buffered | `u64 wh` | *(empty)* | Closes the window. |
| `74` | `C_WINDOW_SET_TITLE` | Buffered | `u64 wh, str title` | *(empty)* | Updates OS window title. |
| `75` | `C_WINDOW_ACTIVATE` | Buffered | `u64 wh` | *(empty)* | Brings window to foreground and focuses it. |
| `76` | `C_ON_WINDOW_CLOSED` | Immediate | `u64 wh, u32 slot, u32 gen` | `u64 sub` (`HK_SUB`) | Fires `SHAPE_WINDOW` once after `wh` closes. |
| `77` | `C_WINDOW_INFO` | Immediate | `u64 wh` | `str title, f32 x, f32 y, f32 w, f32 h, u8 active` | Reads window geometry and active state. |
| `80` | `C_WINDOWS` | Immediate | *(empty)* | `list<u64 wh>` | All open window handles in open order. |
| `81` | `C_QUIT_ON_LAST_WINDOW` | Buffered | `u8 enable` (default `1`) | *(empty)* | Controls whether closing the last window exits `gk_run`. |
| `96` | `C_ON_WINDOW_SHOULD_CLOSE` | Immediate | `u64 wh` (`0` = turn's window), `u32 slot, u32 gen` | `u64 sub` (`HK_SUB`) | Fires `SHAPE_UNIT_BOOL` when user clicks close; returning `false` vetoes close. |
| `97` | `C_ON_APP_QUIT` | Immediate | `u32 slot, u32 gen` | `u64 sub` (`HK_SUB`) | Fires `SHAPE_UNIT` once with no window as the app quits. |
| `78` | `C_CLIPBOARD_WRITE` | Buffered | `str text` | *(empty)* | Copies plain UTF-8 text to the OS clipboard. |
| `79` | `C_CLIPBOARD_READ` | Immediate | *(empty)* | `opt<str>` | Reads plain UTF-8 text from the OS clipboard. |
| `98` | `C_REDUCE_MOTION` | Immediate (view-safe) | *(empty)* | `u8` (`0`/`1`) | Reads OS / app reduced-motion preference. |
| `99` | `C_SET_REDUCE_MOTION` | Buffered | `u8 reduce` | *(empty)* | Overrides reduced-motion preference and redraws every window. |

### 11.5 Native File Dialogs, Prompts, App Menus, URLs, and Platform Assets (`core/src/platform.rs`)

Async dialogs return a `HK_TASK` completion handle and deliver their result later via a `K_DONE` turn. Releasing the `HK_TASK` handle before completion discards the result cleanly.

| Op | Constant | Mode | Request | Response |
|---:|---|---|---|---|
| `82` | `C_PROMPT_PATHS` | Immediate | `u8 files, u8 dirs, u8 multiple, opt<str> prompt, u32 slot, u32 gen` | `u64 task` (`HK_TASK`, delivers `SHAPE_PATHS`) |
| `83` | `C_PROMPT_NEW_PATH` | Immediate | `str dir, opt<str> suggested_name, u32 slot, u32 gen` | `u64 task` (`HK_TASK`, delivers `SHAPE_PATHS`) |
| `84` | `C_WINDOW_PROMPT` | Immediate | `u64 wh` (`0` = turn's), `u8 level` (`0` info, `1` warn, `2` critical), `str msg, opt<str> detail, u32 n_btns, n_btns * (u8 kind [0 other, 1 ok, 2 cancel], str label), u32 slot, u32 gen` | `u64 task` (`HK_TASK`, delivers `SHAPE_PROMPT`) |
| `85` | `C_OPEN_URL` | Buffered | `str url` | *(empty; recorded in scripted mode)* |
| `86` | `C_SET_MENUS` | Buffered | `u32 n, n * Menu` (recursive layout below) | *(empty; updates both OS menu bar and `AppMenuBar`)* |
| `87` | `C_MENUS_GET` | Immediate | *(empty)* | `u32 n, n * Menu` |
| `88` | `C_PLATFORM_ANSWER` | Immediate | `u8 kind` (`1` paths, `2` new_path, `3` prompt), `u8 status`, then `(list<str> paths, str err)` or `u32 answer` | `u8 delivered_immediately` (`0`/`1`) |
| `89` | `C_PLATFORM_LOG` | Immediate | *(empty)* | `list<str> urls, u64 requests, u64 delivered, u64 discarded, u32 waiting, u8 scripted` |
| `90` | `C_IMAGE_CACHE_NEW` | Immediate | *(empty)* | `u64 obj` (`HK_OBJ`, `RetainAllImageCache`) |
| `100` | `C_RENDER_IMAGE_NEW` | Immediate | `u32 width, u32 height, bytes rgba_pixels` | `u64 obj` (`HK_OBJ`, `RenderImage`, converted RGBA -> BGRA) |
| `91` | `C_SCROLL_ANCHOR_NEW` | Immediate | `u64 scroll_handle` (`HK_OBJ`) | `u64 obj` (`HK_OBJ`, `ScrollAnchor`) |
| `92` | `C_SCROLL_ANCHOR_TO` | Immediate | `u64 scroll_anchor` | *(empty; scrolls in turn's window)* |
| `93` | `C_TEXT_SCOPE_NEW` | Immediate | *(empty)* | `u64 obj` (`HK_OBJ`, `TextSelectionScopeId`) |
| `94` | `C_PLATFORM_TEST` | Immediate | `u8 what, <args>` | Test-specific |

#### `Menu` Wire Layout (`C_SET_MENUS` / `C_MENUS_GET`)

- `Menu`: `str name, u8 disabled, u32 n_items, n_items * MenuItem`
- `MenuItem`: `u8 kind`, followed by:
  - `0` (Separator): *(no payload)*
  - `1` (Action): `str name, u32 action_id, u8 os_action` (`0` none, `1` cut, `2` copy, `3` paste, `4` select_all, `5` undo, `6` redo), `u8 checked, u8 disabled`
  - `2` (Submenu): recursive `Menu`
  - `3` (OS Services menu): `str name`

#### Scripted Platform & Headless Harness (`gk_headless_run`, `core/meter/src/headless.rs`)

When running under `gk_headless_run` (requires `GK_FEAT_DIAG`, or `GK_SCRIPTED_PLATFORM=1` in a `diag` build):

- Native file dialogs (`C_PROMPT_PATHS`, `C_PROMPT_NEW_PATH`) and alert prompts (`C_WINDOW_PROMPT`) do not block on OS UI; they wait on per-kind FIFO queues fed by `C_PLATFORM_ANSWER (88)` or by `answer ...` lines in the headless script.
- `C_OPEN_URL (85)` does not spawn a browser; it appends the URL to the platform log returned by `C_PLATFORM_LOG (89)`.
- `C_RENDER_IMAGE_NEW (100)` converts caller-supplied RGBA8 rows into GPUI's BGRA8 `RenderImage` format (`width * height * 4` bytes). Calling `C_RELEASE` on that `HK_OBJ` invokes `Window::drop_image` on every open window so GPU texture atlas memory is reclaimed immediately.
- If a script command closes the driven window while another window remains open, the harness automatically switches to open window `0` and logs a notice; if no window remains, the script terminates cleanly.

#### Headless Script Commands (One Command Per Line)

| Command | Effect |
|---|---|
| `size W H` | Sets offscreen window dimensions in pixels (must appear first; default `640 480`). |
| `frame` | Runs pending foreground tasks, pre-frame hooks, and draws one frame. |
| `shot FILE.png` | Writes the last drawn frame to `FILE.png` (exact wgpu render output). |
| `click X Y` | Injects a left click at `(X, Y)` followed by a frame. |
| `dblclick X Y` | Injects a double click at `(X, Y)` followed by a frame. |
| `rclick X Y` | Moves, presses right button, and releases in separate updates (matching a real window), then draws a frame. |
| `move X Y` | Moves pointer to `(X, Y)` (hover), then draws a frame. |
| `scroll X Y DY` | Injects vertical wheel scroll of `DY` pixels at `(X, Y)` (negative scrolls content up), then draws a frame. |
| `drag X1 Y1 X2 Y2` | Presses at `(X1, Y1)`, performs 4 intermediate moves (one frame each) to `(X2, Y2)`, releases, then draws a frame. |
| `type TEXT` | Sends one keystroke per character to the focused element, drawing a frame after each. |
| `key NAME` | Sends one named GPUI keystroke (`enter`, `escape`, `ctrl-a`, `tab`, etc.), then draws a frame. |
| `sleep MS` | Sleeps `MS` milliseconds so background threads (`gk_post`) can complete work. |
| `answer WHAT` | Answers the oldest waiting scripted platform dialog (`paths P..`, `new-path P`, `prompt N`, `cancel paths|new-path|prompt`), then draws a frame. |
| `stat` | Prints `gk-meter` boundary counters (`callbacks`, `calls`, `tapes`, `rows_missing`, etc.). |

---

## 12. Docks (`core/src/dock.rs`)

### 12.1 Panels and Skinned Dock Areas

A dock panel (`GkPanel`) wraps an app `GkView` inside a GPUI entity (`HK_STATE`) implementing GPUI-kit's `Panel` and `PanelView` traits.

- **`PanelData` wire layout** (used in `C_PANEL_NEW`, `C_PANEL_SET`, `C_PANEL_FILL`):
  - `str name` (interned `&'static str` in Rust; identifies panel type for saved layouts)
  - `opt<str> tab_name`
  - `str title`
  - `u8 flags`: bit 0 = not closable, bit 1 = not zoomable, bit 2 = hidden, bit 3 = no inner padding, bit 4 = no title bar
  - `u64 suffix_view` (`0` = none)
  - `u64 toolbar_view` (`0` = none)
  - `u64 menu_view` (`0` = none; root describes a `PopupMenu`)
  - `str info` (arbitrary JSON string serialized into `PanelInfo::Panel`)

| Op | Constant | Mode | Request | Response |
|---:|---|---|---|---|
| `144` | `C_PANEL_NEW` | Immediate | `u64 view, PanelData, u32 slot, u32 gen` (`0, 0` = no events) | `u64 panel` (`HK_STATE`) |
| `145` | `C_PANEL_SET` | Buffered | `u64 panel, PanelData` | *(empty)* |
| `146` | `C_PANEL_EMIT` | Buffered | `u64 panel, u8 event` (`0` ZoomIn, `1` ZoomOut, `2` LayoutChanged) | *(empty)* |
| `147` | `C_DOCK_AREA_SKINNED` | Immediate | `str id, u8 has_version, u64 version` | `u64 dock_area` (`HK_STATE`, pairs `DockArea` with its `DockSkin`) |
| `148` | `C_PANEL_GROUP` | Immediate | `u64 panel` | `u64 tab_group` (`HK_STATE`, or `0` if not in a group) |
| `149` | `C_PANEL_ID` | Immediate | `u64 panel` | `u64 panel_id` (raw `EntityId`) |
| `158` | `C_DOCK_SKIN` | Immediate/Buffered | `u64 dock_area, u8 what, [arg]` | See table below |
| `163` | `C_DOCK_TOGGLE_SUBSCRIBE` | Immediate | `u64 dock_area, u32 slot, u32 gen` | `u64 sub` (`HK_SUB`, fires `SHAPE_DOCK_TOGGLE`) |

#### `C_DOCK_SKIN (158)` Sub-Operations (`u8 what`)

| `what` | Arg | Response | Meaning |
|---:|---|---|---|
| `0` | none | `u16` (`0` Auto, `1` TabBar) | Read `DockSkin::panel_style` |
| `1` | `u16 style` | none | Set `DockSkin::set_panel_style` |
| `2` | none | `u8` | Read `is_toggle_button_visible` |
| `3` | `u8 visible` | none | Set `set_toggle_button_visible` |
| `4` | `u8 visible` | none | Set `set_close_button_visible` |
| `5` | none | `u8` | Read `is_collapsed_tab_bar_visible` |
| `6` | `u8 visible` | none | Set `set_collapsed_tab_bar_visible` |
| `7` | none | `u8` | Read `is_split_divider_line_visible` |
| `8` | `u8 visible` | none | Set `set_split_divider_line_visible` |

#### Dock Elements

- **`N_DOCK_VIEW (33)`**: `u64 dock_area` -> renders the `DockArea` entity.
- **`N_DOCK_SLOT (37)`**: *(no args)* -> inside a custom dock renderer view, renders the upstream child element handed to the hook.

### 12.2 Saved Layouts, `PanelRegistry`, Entity Deduplication, and Pane Trees

- **`PanelInfo` wire layout**:
  - `u16 0` (Stack): `list<f32> sizes_px, u64 axis`
  - `u16 1` (Tabs): `u64 active_index`
  - `u16 2` (Panel): `str json` (empty = JSON `null`; non-JSON strings fall back to `serde_json::Value::String`)
- **`PanelState` wire layout**: `str panel_name, list<PanelState> children, PanelInfo info`.
- **`DockAreaState` wire layout**: `opt<u64> version, PanelState center, opt<DockState> left_dock, opt<DockState> right_dock, opt<DockState> bottom_dock` (where `DockState` encodes placement, open/size state, and its `PanelState`).
- **`NODE_IDS` table**: GPUI-kit's `NodeId` has no public constructor. `core/src/dock.rs` maps every `NodeId` returned by dock reads to a `u64` in a thread-local `NODE_IDS` map. Passing an unknown `NodeId` in a call op returns `GK_ERR_BAD_OP`, and in a tape triggers `rt::app_stop`.
- **`HANDED` entity deduplication & `VIEWS` reverse map**:
  - Whenever Rust hands a GPUI `Entity<T>` back to the front end (such as a `TabGroup`, `DockArea`, `GkPanel`, or `AppMenuBar`), `dock::entity_handle(&entity)` checks `HANDED: HashMap<EntityId, u64>`. If the entity already has a live `HK_STATE` handle, the exact same handle is returned rather than creating duplicates.
  - Reading an `AnyView` back checks `VIEWS: HashMap<EntityId, u64>` and returns its `HK_VIEW` handle (or `0` for an internal kit placeholder).

| Op | Constant | Mode | Request | Response |
|---:|---|---|---|---|
| `150` | `C_PANEL_REGISTER` | Immediate | `str name, u32 slot, u32 gen` | *(empty; registers `SHAPE_PANEL_BUILD` callback for `name`)* |
| `151` | `C_PANEL_FILL` | Buffered | `u64 panel, u64 view, PanelData, u32 slot, u32 gen` | *(empty; fills a placeholder panel requested by `SHAPE_PANEL_BUILD`)* |
| `152` | `C_PANEL_BUILD_NOW` | Immediate | `u64 dock_area, PanelState` | `opt<u64 panel>` |
| `153` | `C_PANE_TREE_TO_STATE` | Immediate | `u64 pane_tree, u64 dock_area` | `PanelState` |
| `154` | `C_PANE_TREE_FROM_STATE` | Immediate | `u64 dock_area, PanelState, u16 root_kind` | `u64 pane_tree` |
| `156` | `C_PANE_NODE_KIND` | Immediate | `u64 pane_node` | `PaneNodeKind` value |
| `157` | `C_PANE_NODE_WALK` | Immediate | `u64 pane_node` | `list<u64 pane_node>` |

### 12.3 Custom Dock Renderers (`GkRenderer`)

- **`C_DOCK_RENDERER_NEW (160)` (immediate)**: `u32 hooks_mask` (bit `h` = hook `h` answered by app), `u32 slot, u32 gen` (kept handler of `SHAPE_DOCK_RENDER = 38`) -> `u64 renderer` (`HK_OBJ`).
- **Hook indices (`0..=11`)**: `0` Frame, `1` CenterFrame, `2` SplitFrame, `3` SplitHandle, `4` Dock, `5` Placeholder, `6` TabFrame, `7` TabContentFrame, `8` TabBar, `9` TabActivePanel, `10` TabDropIndicator, `11` TabEmpty.
- **`RenderBook` deduplication & `SLOT`**:
  - When GPUI invokes an enabled hook, `GkRenderer` computes a context snapshot hash for `(renderer, hook, key)`.
  - If an answer view is stored in `RenderBook::answers[(renderer, hook, key)]`, `GkRenderer` places upstream's child element into the thread-local `SLOT` cell, renders the answer view (which consumes `SLOT` via `N_DOCK_SLOT = 37`), and clears `SLOT`.
  - If the context snapshot differs from `RenderBook::asked[(renderer, hook, key)]`, `GkRenderer` releases any prior context objects for that key, queues one `SHAPE_DOCK_RENDER` event with fresh `dock_ctx` / `group_ctx` handles, and records the new snapshot.
- **`C_DOCK_RENDER_ANSWER (161)` (buffered)**: `u64 renderer, u8 hook, u64 key, u64 view` (`0` restores upstream default).
- **`C_DOCK_RENDER_TEST (162)` (immediate)**: `u64 renderer` -> `u32 requests_sent, u32 answers_kept, u32 hooks_drawn_from_answer`.

---

## 13. Data Tables and Described Plots

### 13.1 Columnar `Datum` Table Wire Format (`core/src/data.rs`)

Generic GPUI-kit charts (`LineChart`, `AreaChart`, `BarChart`, `PieChart`, `CandlestickChart`, `RadarChart`, `SankeyChart`) and scales are instantiated in Rust over `data::Datum`, a lightweight `(Rc<Table>, u32 row)` handle. A data table is encoded inline inside a tape node or value argument as:

```text
u16 n_cols
n_cols * (u8 col_kind, str col_name)
u32 n_rows
n_rows * [value for each col 0..n_cols in order]
```

| `col_kind` | Constant | Cell Wire Encoding | Notes |
|---:|---|---|---|
| `0` | `NUM` | `f64` | General numeric column |
| `1` | `TEXT` | `str` | String label / category column |
| `2` | `COLOR` | `Color` (`COLOR_HSLA` or `COLOR_THEME`) | Resolved against active theme during decode (transparent in plain/no-context constructor chains) |
| `3` | `OPT` | `u8 has, f64 val` | Optional float (`has == 0` -> `None`; `val` still occupies 8 bytes) |
| `4` | `NUM32` | `f32` | 32-bit float (formatted as `f32`) |

### 13.2 `Field` and `Format` Operands

- **`Field`**: encoded simply as `str column_name`.
  - An empty name `""` resolves against `Datum::key` (set by `Datum::keyed(key)` for multi-series closures like `Stack::value(Fn(&T, &str))`).
  - Column index lookup is cached on first access per `Field`; naming a column that does not exist in the table triggers `rt::app_stop` with the available column names.
  - `Datum::index()` returns the 0-based row index (used when correlating chart rows with a row source or selection).
- **`Format`**: encoded as `str template`:
  - `{}` formats the first numeric argument passed by upstream (e.g., axis tick value).
  - `{0}`, `{1}`, ... format positional arguments passed by upstream.
  - `{col}` formats column `col` of the current `Datum` row (numeric columns format as `f64`/`f32`, text columns verbatim).
  - Format specifiers after `:` support `,` (ASCII thousands grouping, e.g., `{:,}`) and `.P` (fixed decimal precision `P`, e.g., `{:.2}` or `{:,.2}`).
  - `{{` and `}}` emit literal `{` and `}`.

### 13.3 Described Plots (`core/src/plot.rs`)

| Node | Op | Args Layout | Description |
|---|---:|---|---|
| `N_PLOT` | `28` | `u32 n_parts, n_parts * u32 value_node` | Composes an arbitrary list of paintable plot parts (generated chart shapes, grids, axes, or `N_PLOT_PATH` / `N_PLOT_QUAD`), painted directly in Rust without callbacks during paint. |
| `N_PLOT_PATH` | `29` | `u8 kind` (`0` stroked polyline, `1` filled closed polygon), `f32 stroke_width, u32 n_pts, n_pts * (f32 x_px, f32 y_px), Fill` | Paints a path in pixel coordinates relative to the plot's top-left corner (`COLOR_HSLA`, `COLOR_THEME`, or `COLOR_GRADIENT`). |
| `N_PLOT_QUAD` | `30` | `f32 x, f32 y, f32 w, f32 h, f32 corner_radius, Fill, f32 border_width, Color border` | Paints a rounded rectangle quad in plot-local pixel coordinates. |

#### Worked Byte Example: `N_PLOT` Containing One `N_PLOT_QUAD`

A `100 x 40` rounded quad at `(10, 20)` with radius `6`, filled with `theme` token `12` (`0x000C`), 1 px border of `theme` token `18` (`0x0012`):

```text
Node 0 (N_PLOT_QUAD = 30, args at ops[4..39]):
  00 00 20 41       x = 10.0f32
  00 00 A0 41       y = 20.0f32
  00 00 C8 42       w = 100.0f32
  00 00 20 42       h = 40.0f32
  00 00 C0 40       corner_radius = 6.0f32
  01  0C 00         Fill = COLOR_THEME(1), token = 12
  00 00 80 3F       border_width = 1.0f32
  01  12 00         border = COLOR_THEME(1), token = 18

Node 1 (N_PLOT = 28, root, args at ops[39..47]):
  01 00 00 00       n_parts = 1
  00 00 00 00       part[0] = node 0
```

---

## 14. Motion, Value Cells, and Declarative Rules

### 14.1 `N_ANIMATED` (`48`, `core/src/motion.rs`)

Wraps a child element with GPUI animations, motion values, a focus trap, or a text selection scope.

**Header**: `u32 child_node, u8 kind`, followed by kind-specific fields:

| `kind` | Name | Fields After `u8 kind` |
|---:|---|---|
| `1` | `Anim` | `str id, u32 n_anims, n_anims * (u32 duration_ms, u8 flags [1 repeat, 2 repeat_synced], Easing, f32 max_fps), u32 n_tracks, n_tracks * (u16 anim_idx [0xffff = all], u8 prop, Val from, Val to)` |
| `2` | `Effect` | `str id, u32 EffectTransition_node` |
| `3` | `Values` | `u32 n_vals, n_vals * (u8 prop, MotionSource)` |
| `4` | `FocusTrap` | `str id, u64 focus_handle` (wraps a parent + styled element) |
| `5` | `TextSelectionScope` | `u64 text_scope_handle` (wraps a parent element) |

#### Animatable Properties (`u8 prop`)

| Code | Property | Value Type (`Val`) |
|---:|---|---|
| `0` | `Opacity` | `f32` |
| `1` | `Top` | `f32` (px) |
| `2` | `Left` | `f32` (px) |
| `3` | `Right` | `f32` (px) |
| `4` | `Bottom` | `f32` (px) |
| `5` | `Width` | `f32` (px) |
| `6` | `Height` | `f32` (px) |
| `7` | `Bg` | `Color` (`COLOR_HSLA` or `COLOR_THEME`) |
| `8` | `TextColor` | `Color` |
| `9` | `BorderColor` | `Color` |
| `10` | `Size` | `f32` (px, sets both width and height) |

#### Easings (`u8 easing`)

`0` = Linear, `1` = Quadratic, `2` = EaseInOut, `3` = EaseOutQuint, `4` = EaseOutCubic, `5` = EaseInCubic, `6` = EaseInOutCubic, `7` = CubicBezier (`4 * f32`), `8` = Bounce (followed by one non-bounce `Easing`; bounce-of-bounce is refused), `9` = PulsatingBetween (`f32 lo, f32 hi`).

#### Motion Sources (`u8 src`, for `kind == 3`)

| `src` | Name | Fields After `u8 src` | Restrictions |
|---:|---|---|---|
| `1` | Transition | `str id, Val target, u32 Transition_node` | Numeric or color properties |
| `2` | Spring | `str id, f32 target, u32 Spring_node` | Numeric properties only |
| `3` | Keyframes | `str id, u64 Keyframes_f32_obj, u32 Timing_node` | Numeric properties only |
| `4` | Presence | `u32 Presence_node, Val at_0, Val at_1` | Numeric or color properties |
| `5` | TransitionState | `u64 TransitionState_f32_obj, f32 target, u32 Transition_node` | Numeric properties only |

### 14.2 Value Cells, Calendar Matchers, Settings Fields, and Questionnaire Rules (`core/src/values.rs`)

- **Value cells** (back `Fn(&App) -> T` read closures used by settings fields while drawing):
  - `C_CELL_NEW (211)` (immediate): `bytes value` -> `u64 cell` (`HK_OBJ`, kept).
  - `C_CELL_SET (212)` (buffered): `u64 cell, bytes value` (updates stored bytes and redraws every window).
  - `C_CELL_GET (213)` (immediate, view-safe): `u64 cell` -> `bytes value`.
  - When a generated closure reads a cell (`values::cell_read`), Rust clones the stored bytes, appends 64 zero bytes of safety padding, and decodes via `R::new(&b)`, so a cell holding a mismatched shorter type reads zeros rather than panicking out of bounds.
- **Calendar `Matcher::Custom`**: encoded as `list<Date>` (a value set); Rust builds a membership predicate and remembers the encoded bytes in `VALUE_SETS` keyed by the boxed closure pointer so reading the matcher back round-trips the exact list (`u32 0` if a predicate was not created by the binding).
- **`AnySettingField`**: generated setting field nodes wrap their concrete `AnySettingField` in `values::AnyField` (`Rc<dyn AnySettingField>`) so upstream `as_any()` and `type_id()` downcasts see the concrete field type.
- **`QuestionnaireRules`** (declarative synchronous validation for `QuestionnaireItemDefinition::with_validator`):
  - `u32 min_choices, u32 max_choices, u32 min_length, u32 max_length` (`0` = no limit; lengths count Unicode scalar characters `chars().count()` on the freeform answer)
  - `list<str> one_of, list<str> not_one_of` (exact match against freeform text; empty = unrestricted)
  - `str same_as, str differs_from` (`""` = none; compares against another item's answer in `ctx.answers()`)
  - `str message` (`""` = default per-rule message; `"{item}"` is substituted with `ctx.item()`)
  - Reading back `QuestionnaireItemDefinition::validator` (`Option<&QuestionnaireValidator>`) writes `u8 0` or `u8 1` with zero payload bytes (a closure has no readable state beyond presence).
- **Test ops**:
  - `C_OBJ_TEST_TEMPS (113)` -> `u32 temps, u32 live`
  - `C_VALUES_TEST (210)` -> `u32 pinned, u32 pending_sweep`
  - `C_HANDLES_TEST (214)` -> `u64 values_h, u64 dock_h, u64 again_h, u64 count` (verifies `Wire for Entity` and `dock::entity_handle` share one deduplicated `HANDED` table).

#### Worked Byte Example: `N_ANIMATED` (Kind 1: Pulse Opacity From 0.3 to 1.0 Over 800 ms, Repeating)

Assume child element is node `0`, `N_ANIMATED = 48` (`0x0030`), id `"pulse"`, 1 animation (`800 ms`, repeat bit `1`, easing `2` = `EaseInOut`, `max_fps = 0.0`), 1 track (`anim_idx = 0`, `prop = 0` = `Opacity`, `from = 0.3f32`, `to = 1.0f32`):

```text
Offset  Hex Bytes                       Meaning
------  ------------------------------  ----------------------------------------
0x00    00 00 00 00                     child_node = 0
0x04    01                              kind = 1 (Anim)
0x05    05 00 00 00 70 75 6C 73 65      id = "pulse" (len=5)
0x0E    01 00 00 00                     n_anims = 1
0x12    20 03 00 00                     duration_ms = 800 (0x0320)
0x16    01                              flags = 1 (repeat)
0x17    02                              easing = 2 (EaseInOut)
0x18    00 00 00 00                     max_fps = 0.0 (unlimited)
0x1C    01 00 00 00                     n_tracks = 1
0x20    00 00                           anim_idx = 0
0x22    00                              prop = 0 (Opacity)
0x23    9A 99 99 3E                     from = 0.3f32
0x27    00 00 80 3F                     to = 1.0f32
```

#### Worked Byte Example: `C_WINDOW_OPEN` (Open 640x480 Window With Kit Title Bar and `app_id`)

```text
Offset  Hex Bytes                       Meaning
------  ------------------------------  ----------------------------------------
0x00    48 00                           call op = 72 (C_WINDOW_OPEN)
0x02    00 00 20 44                     width = 640.0f32
0x06    00 00 F0 43                     height = 480.0f32
0x0A    01 00 00 00 01 00 00 01         root_view = HK_VIEW (slot 1, gen 1)
0x12    05 00 00 00 4E 6F 74 65 73      title = "Notes"
0x1B    20                              flags = 32 (kit title bar)
0x1C    00 00 48 43                     min_w = 200.0f32
0x20    00 00 16 43                     min_h = 150.0f32
0x24    00                              kind = 0 (Normal)
0x25    00                              has_pos = 0 (OS / centered)
0x26    00 00 00 00  00 00 00 00        x = 0.0, y = 0.0 (ignored)
0x2E    05 00 00 00 6E 6F 74 65 73      app_id = "notes"
0x37    00 00 00 00                     icon_png len = 0 (none)
```

---

## 15. Complete Wire Type Mapping for Generated Descriptors

Front-end code generators targeting `libgouikit` consume the intermediate model produced by `gen/gk_model.py`. Every wire descriptor in that model maps deterministically to the following byte encoding on both sides of the boundary:

| Model Descriptor | In Tape / Call Args (Front End -> Library) | In Call Results / Events (Library -> Front End) |
|---|---|---|
| `bool`, `u8`..`u64`, `i8`..`i64`, `f32`, `f64`, `usize`, `isize`, `char` | Fixed-width LE primitive (`usize`/`isize` widened to 8 bytes) | Identical |
| `str` / `SharedString` / `String` | `u32 len` + UTF-8 bytes | Identical |
| `bytes` / `Vec<u8>` | `u32 len` + raw bytes | Identical |
| `px` / `rems` | `f32` | `f32` |
| `length` / `def_length` | `u8 kind (0..3)` + `f32 value` | `u8 kind` + `f32 value` |
| `color` | `u8 0 + 4*f32` or `u8 1 + u16 token` | `4*f32` (`Hsla`) or `u16 0 + 4*f32` (color field in bidirectional record) |
| `fill` | `color` or `u8 2 + f32 angle + u8 oklab + color + f32 + color + f32` | `Background` read-back (Section 3.2) |
| `elid` (`ElementId`) | `u8 0+str` \| `u8 1+u64` \| `u8 2+str+u64` | `str` (display text) |
| `range` (`Range<usize>`) | `u64 start, u64 end` | `u64 start, u64 end` |
| `duration` | `i64 nanos` (or `u32 ms` in `TIMER_NEW`/`ANIMATED`) | `i64 nanos` |
| `instant` | `i64 nanos` since library clock epoch | `i64 nanos` |
| `date` / `time` / `datetime` / `weekday` | Civil date/time structs (Section 3.2) | Identical |
| `font` | `str family, f32 weight, u16 style` | Identical |
| `drag` | `u64 id, str text` | Identical |
| `opt<T>` | `u8 0` or `u8 1 + T` | Identical |
| `list<T>` | `u32 n + n * T` | Identical |
| `map<K, V>` | `u32 n + n * (K, V)` | `u32 n + n * (K, V)` sorted by encoded `K` bytes |
| `enum` | `u16 variant_num` | `u16 variant_num` |
| `denum` | `u16 variant_num + variant fields` | Identical |
| `record` | Declared fields concatenated | Identical |
| `into` | `u8 arm_idx + chosen arm` | N/A (input-only) |
| `state` (`Entity<T>`) | `u64` (`HK_STATE`) | `u64` (`HK_STATE`, deduplicated via `HANDED`) |
| `obj` (`Clone` / non-entity object) | `u64` (`HK_OBJ`) | `u64` (`HK_OBJ`: kept from ctor, temporary from read/call/event) |
| `view` / `viewany` | `u64` (`HK_VIEW`) | `u64` (`HK_VIEW` or `0` if internal) |
| `focus` | `u64` (`HK_FOCUS`) | `u64` (`HK_FOCUS`, new handle per read) |
| `source` | `u64` (`HK_SOURCE`) | `u64` (`HK_SOURCE`) |
| `action` | `u32 action_id` | `u32 action_id` |
| `handler` (view-scoped) | `u32 handler_id` | Fired via `K_EVENT(view, gen, handler_id, shape, payload)` |
| `kept_handler` | `u32 slot, u32 gen` | Fired via `K_EVENT(0, gen, slot, shape, payload)` |
| `node` (child element or value in tape) | `u32 node_index` | N/A |
| `vtape` (builder value in call op) | `u32 byte_len + GKT1 tape` | N/A |
| `datum_table` | Columnar `Datum` table (Section 13.1) | N/A |
| `field` / `format` / `expr` | `str` | N/A |
| `fallible<T>` | N/A | `u8 ok, u8 has_val, [T], str err` |
| `outcome` (`Result<(), E>`) | N/A | `opt<str>` |
| `viewbuild` (`ViewBuild`) | `u64 view_handle` (`HK_VIEW`) | N/A |
| `cell` (`ValueCell`) | `u64 cell_handle` (`HK_OBJ`) | N/A |
| `value_set<K>` | `list<K>` | `list<K>` |
| `questionnaire_rules` | `QuestionnaireRules` (Section 14.2) | `u8` presence (`0`/`1`) |

---

## 16. Error Codes and Front-End Responsibilities

Every C function returning `int32_t` uses these codes from `core/wire/ops.txt`:

| Code | Constant | Meaning | Required Front-End Action |
|---:|---|---|---|
| `0` | `GK_ERR_OK` | Success. | Proceed normally. |
| `1` | `GK_ERR_BAD_OP` | Unknown call opcode, invalid sub-op, or malformed call arguments (such as an unknown dock `NodeId`). | Treat as a binding/caller bug: read `gk_last_error`, stop the app cleanly (`C_QUIT`), and surface the error with the caller's source location. |
| `2` | `GK_ERR_STALE_HANDLE` | Handle was already released, never existed, or has the wrong kind (`HK_*`). | Report as a use-after-release / misuse error, request `C_QUIT`, and return an error from `Run`. |
| `3` | `GK_ERR_BUSY` | `gk_run` / `gk_headless_run` called while another run is already active in this process. | Return an error immediately without disturbing the active run. |
| `4` | `GK_ERR_NO_CONTEXT` | `gk_call` called outside a callback, `gk_reply` called with a non-current `cb`, `gk_post` called outside `gk_run`, or a window-requiring op called when no window is open. | For `gk_post`: drop or return false (app is shutting down). For `gk_call`/`gk_reply`: treat as a misuse error (stale `Cx` or wrong turn). |
| `5` | `GK_ERR_WRONG_THREAD` | `gk_call` or `gk_reply` called from a thread other than the UI thread that entered `gk_run`. | Report an off-thread UI call misuse error. Front ends should also check thread/goroutine identity in host code before calling C. |
| `6` | `GK_ERR_POISONED` | The Rust library previously caught a panic and marked the session poisoned. Only `C_QUIT` is accepted. | Stop issuing further calls, call `C_QUIT` if not already quitting, and return a library-error type from `Run` once `gk_run` exits. |
| `7` | `GK_ERR_BAD_TAPE` | Tape failed `Tape::parse` (wrong magic, truncated buffer, `root >= node_count`, out-of-bounds offset). | Indicates a front-end tape encoder bug; stop and surface `gk_last_error`. |
| `8` | `GK_ERR_VERSION` | `gk_app` failed size, `abi_major`, `abi_minor`, `api_hash`, or non-NULL callback validation. | Refuse startup and tell the user to match the library and front-end package versions. |
| `9` | `GK_ERR_UNSUPPORTED` | Feature not compiled into this library build (for example, `gk_headless_run` on a `ship` build without `GK_FEAT_DIAG`). | Return an unsupported-build error telling the user to use a `diag` library build. |
| `10` | `GK_ERR_BAD_ENUM` | An enum discriminant on the wire did not match any known variant of that enum. | Treat as a caller/binding bug, read `gk_last_error`, stop cleanly, and surface the error. |

### 16.1 Session State Machine and Fault Recovery

A front end interacts with `libgouikit` through five lifecycle states:

1. **Unstarted**: Only `gk_info_get()` and `gk_use_all()` (or `gk_use_<family>()`) are valid. `gk_call`, `gk_reply`, and `gk_post` return `GK_ERR_NO_CONTEXT`.
2. **Starting (`gk_run` / `gk_headless_run` handshake)**: Validates `gk_app` (`size`, `abi_major`, `abi_minor`, `api_hash`, non-NULL `callback`). Any mismatch returns `GK_ERR_VERSION` immediately without entering GPUI. Re-entrant `gk_run` returns `GK_ERR_BUSY`.
3. **Running**: UI thread enters GPUI's run loop, fires `K_INIT`, then dispatches turns (`WINDOW_INIT`, `FRAME`, `ROWS`, `VIEWS`, `EVENT`, `WAKE`, `DONE`). Background threads may call `gk_post`.
4. **Stopping / Poisoned**:
   - **Normal stop**: App calls `C_QUIT` (or last window closes with `QUIT_ON_LAST_WINDOW == 1`). `C_ON_APP_QUIT` fires `K_EVENT(SHAPE_UNIT)` with `wh == 0`, then `gk_run` returns `GK_ERR_OK`.
   - **App stop (`app_stop`)**: Library detects a drawing-time misuse (such as rendering a released handle or mismatched `ViewBuild` root), fires `K_PANIC` with trailing `u8 1`, calls `quit_soon()`, and unwinds the offending draw via `AppStop`. Front end records a caller error (`*gk.Error`), returns `0`, lets `gk_run` finish, and returns that error to `main`.
   - **Library fault (`POISONED`)**: A Rust panic inside `gk_call` sets `POISONED = true` and returns `GK_ERR_POISONED`; a Rust panic outside `gk_call` sets `POISONED = true`, fires `K_PANIC` with no trailing `u8 1`, and calls `quit_soon()`. While poisoned, every `gk_call` except `C_QUIT` returns `GK_ERR_POISONED`. Front end records a library fault (`*gk.LibraryError`), issues `C_QUIT` if needed, and returns it once `gk_run` exits.
5. **Exited**: Session is cleared and `gk_post` returns `GK_ERR_NO_CONTEXT`.

### 16.2 New Front-End Implementation Checklist

To bring up a new language front end against `libgouikit`:

1. **Generate constants** from `core/wire/ops.txt` and `api/opcodes.lock` (matching `API_HASH` with FNV-1a 64-bit).
2. **Load and verify**: resolve the 7 C functions (+ `gk_use_all` for dynamic builds), check `gk_info_get()` magic/ABI/hash, lock the UI OS thread, and enter `gk_run`.
3. **Implement the turn stack**: dispatch all 10 `K_*` kinds, support nested turns, honor `GK_FEAT_INLINE_REPLY`, flush deferred writes before any synchronous `gk_call`, and implement inline view description for outermost `EVENT` (`flags & 1`) and `FRAME` (`n_deferred == 0`).
4. **Implement the tape builder**: reserve `ops[0..4] = 0`, emit linked op records and 10-byte node table entries, track `deps` during view reads and `slots` for `OP_CHILD_VIEW`, diff completed tapes against each view's previous bytes, and free retired `(view, gen)` handler tables.
5. **Enforce safety invariants**: reject stale turn contexts, off-thread UI calls, writes inside view builds, duplicate/cross-build element attachment, invalid UTF-8, and double/stale handle releases; catch host panics/exceptions at the callback boundary and translate them to `C_QUIT`.
6. **Verify against the conformance suite**: pass `gates/proto.sh` (every turn kind, shape, handle tier, and protocol family) and `gates/go-misuse.sh` equivalents before claiming parity.

---

## 17. End-to-End Wire Trace Reference

This section walks through complete multi-turn byte exchanges so a front-end author can unit-test their serializer and turn dispatcher against concrete byte sequences.

### 17.1 Startup Sequence (`K_INIT` -> `K_WINDOW_INIT` -> `K_VIEWS`)

1. **Turn 1: `K_INIT` (`kind = 1`, `len = 0`)**
   - Front end calls `gk_call` with `C_VIEW_NEW (1)`:
     - Request (2 bytes): `01 00`
     - Response (8 bytes): `01 00 00 00 01 00 00 01` (`root_view = 0x0100_0001_0000_0001`)
   - Front end calls `gk_call` with `C_OPEN_WINDOW (2)` for `"App"` (`480 x 320`):
     - Request (25 bytes):
       ```text
       02 00                           op = C_OPEN_WINDOW (2)
       00 00 F0 43                     width = 480.0f32
       00 00 A0 43                     height = 320.0f32
       01 00 00 00 01 00 00 01         root_view
       03 00 00 00 41 70 70            title = "App"
       ```
     - Response (8 bytes): `01 00 00 00 01 00 00 02` (`wh = 0x0200_0001_0000_0001`)
   - Callback returns `0` with empty reply (`0` bytes).

2. **Turn 2: `K_WINDOW_INIT` (`kind = 2`, `len = 16`)**
   - Request:
     ```text
     01 00 00 00 01 00 00 02           wh
     01 00 00 00 01 00 00 01           root_view
     ```
   - Front end runs `root_view`'s `init` callback (creating window-backed states via `gk_call`), then returns `0` with any deferred setup writes in Section 3.

3. **Turn 3: `K_VIEWS` (`kind = 4`, `len = 24`)**
   - Request:
     ```text
     01 00 00 00 01 00 00 02           wh
     01 00 00 00                       n_views = 1
     01 00 00 00 01 00 00 01           view[0] = root_view
     00 00 00 00                       n_retired = 0
     ```
   - Front end runs `root_view`'s view function, emits a `GKT1` tape (generation `1`), and returns Section 4 with `n_tapes = 1`, `(root_view, gen=1, tape_len, tape_bytes)`.

### 17.2 Async Provider Request & Answer (`SHAPE_PROVIDER` -> `C_SOURCE_ANSWER`)

1. **Incoming `K_EVENT` (`kind = 5`, `SHAPE_PROVIDER = 14`, Hover variant `1` at offset `4` of `"let x"`)**:
   ```text
   01 00 00 00 01 00 00 02             wh
   00                                  flags = 0 (queued event)
   00 00 00 00                         n_retired = 0
   00 00 00 00 00 00 00 00             view = 0 (kept handler)
   01 00 00 00                         gen = 1
   02 00 00 00                         slot = 2
   0E 00                               shape = 14 (SHAPE_PROVIDER)
   07 00 00 00 00 00 00 00             req_id = 7
   01 00                               variant = 1 (Hover)
   05 00 00 00 6C 65 74 20 78          text = "let x"
   04 00 00 00 00 00 00 00             offset = 4
   ```
2. **Immediate `gk_call` with `C_SOURCE_ANSWER (23)`** answering `req_id = 7` with Markdown `"i32"` and range `4..5`:
   ```text
   17 00                               op = C_SOURCE_ANSWER (23)
   07 00 00 00 00 00 00 00             req_id = 7
   19 00 00 00                         answer_len = 25 bytes
   01                                  has_hover = 1
   03 00 00 00 69 33 32                markdown = "i32"
   01                                  has_range = 1
   04 00 00 00 00 00 00 00             start = 4
   05 00 00 00 00 00 00 00             end = 5
   ```

### 17.3 Async File Dialog Completion (`C_PROMPT_PATHS` -> `K_DONE` -> `C_RELEASE`)

1. **Request `C_PROMPT_PATHS (82)`** (files only, single selection, button `"Open"`, kept slot `5`, gen `1`):
   ```text
   52 00                               op = C_PROMPT_PATHS (82)
   01 00 00                            files=1, dirs=0, multiple=0
   01 04 00 00 00 4F 70 65 6E          opt<str> = Some("Open")
   05 00 00 00                         slot = 5
   01 00 00 00                         gen = 1
   ```
   Response: `u64 task_handle` (`HK_TASK = 9`).
2. **Later `K_DONE` (`kind = 8`, `SHAPE_PATHS = 22`)** when user selects `"/a.txt"`:
   ```text
   00 00 00 00 00 00 00 00             view = 0
   01 00 00 00                         gen = 1
   05 00 00 00                         slot = 5
   16 00                               shape = 22 (SHAPE_PATHS)
   00                                  status = 0 (DoneAnswered)
   01 00 00 00                         n_paths = 1
   06 00 00 00 2F 61 2E 74 78 74       path[0] = "/a.txt"
   00 00 00 00                         error = ""
   ```
3. **Front-end reply to `K_DONE`**: removes kept slot `5`, marks dirtied views in Section 2, and appends a buffered `C_RELEASE (12)` for `task_handle` (`0C 00` + 8-byte `task_handle`, `op_len = 10`) in Section 3.

### 17.4 `SHAPE_LIST_EVENT` and `SHAPE_TABLE_EVENT` Byte Layouts

- **`SHAPE_LIST_EVENT (16)`** confirming row `3` in section `0` (not secondary):
  ```text
  00 00 00 00 00 00 00 00             req_id = 0 (non-request event)
  02 00                               kind = 2 (LE_CONFIRM)
  01                                  has_index = 1
  00 00 00 00 00 00 00 00             section = 0
  03 00 00 00 00 00 00 00             row = 3
  00 00 00 00 00 00 00 00             col = 0
  00                                  secondary = false
  00 00 00 00                         query = ""
  ```
- **`SHAPE_TABLE_EVENT (17)`** sorting column ID `2` ascending:
  ```text
  00 00 00 00 00 00 00 00             req_id = 0
  00 00                               kind = 0 (TE_SORT)
  00 00 00 00 00 00 00 00             row = 0
  02 00 00 00 00 00 00 00             col = 2
  00 00 00 00 00 00 00 00             to = 0
  01 00                               sort = 1 (Ascending)
  ```

