# Contributing

This guide explains how to build `libgouikit`, regenerate the Rust and Go bindings, run every verification gate, add a new wire op or generator rule, bump upstream `gpui-kit` / `gpui-fast` pins, and assemble release archives.

App developers who only consume a prebuilt release archive do not need Rust, Python, or any of the commands below; see [docs/GETTING-STARTED-GO.md](GETTING-STARTED-GO.md) instead.

---

## 1. Repository Layout and Prerequisites

| Directory | Role |
|---|---|
| `core/` | Rust C ABI library (`gk-capi`), runtime (`rt.rs`), tape decoder (`tape.rs`), protocol shims, `core/wire/ops.txt`, `core/wire/gen.py`, and `core/meter/`. |
| `go/` | The `github.com/ibanks42/goui-kit` Go module (`gk` + sub-packages + `internal/rt` + `internal/goid`). |
| `gen/` | Pure Python 3 code generator (`gkgen.py`, `gk_model.py`, `gk_rust.py`, `gk_go.py`, `gk_split.py`, `gk_apidoc.py`, `extract.sh`, `round.sh`). |
| `api/` | Committed rustdoc JSON extract (`api/rustdoc/`), `overrides.toml`, append-only `opcodes.lock`, `clone.json`, `quarantine.json`, and `coverage.json`. |
| `examples/go/` | Complete Go apps, component gallery (`gallery/`), protocol tests (`proto/`, `port/`), and misuse tests (`misuse*`). |
| `ref/` | Pure-Rust reference apps (`ref-counter`, `ref-gallery`, `ref-notes`, `ref-panels`, `ref-table`) used as ground truth by pixel, memory, and frame-cost gates. |
| `gates/` | Automated test and benchmark scripts, compositor launcher (`display.sh`), and baseline files. |
| `tools/` | Build (`build-lib.sh`, `zigbuild.sh`), packaging (`dist.sh`, `dist-test.sh`), license collection, and status update (`status.py`) scripts. |
| `docs/` | Guides, runnable tutorial programs (`docs/tutorial-go/`), snippet programs (`docs/snippets-go/`), specification (`PROTOCOLS.md`), architecture (`DESIGN.md`), and `STATUS.md`. |

### Toolchain Requirements

- **Go 1.27 or newer** (`go/go.mod`).
- **Rust 1.99.1** (`rust-toolchain.toml`, 2024 edition). Nightly Rust is only needed when re-extracting upstream rustdoc JSON (`gen/extract.sh`).
- **Python 3** (standard library only; no `pip` dependencies).
- **Linux x86-64** for the full gate suite (offscreen `diag` rendering, private headless compositor via `gates/display.sh`, and `bc`).
- **Optional cross and release tools**:
  - `cargo-zigbuild` and `zig` (downloaded automatically by `tools/zigbuild.sh`) for `make lib-release` linking against glibc 2.28.
  - `cargo-xwin` for `make check-windows` (type-checking the Windows library from Linux).
  - **macOS**: Xcode Command Line Tools plus `rustup target add aarch64-apple-darwin x86_64-apple-darwin` for universal archives.
  - **Windows**: MSVC toolchain (with Windows SDK `fxc.exe` for release shaders) plus MinGW-w64 `gcc`, `dlltool`, `windres`, and `ld.lld` for `-tags gkstatic` builds.

---

## 2. Building the Library and Go Examples

### 2.1 Building `libgouikit` (`make lib`)

```sh
make lib
```

`tools/build-lib.sh ship diag` compiles release builds into `target/` and copies the artifacts into:

- `lib/ship/`: shared library (`libgouikit.so` / `libgouikit.dylib` / `gouikit.dll`), static library (`libgouikit_static.a` + platform support libraries), and pure-Rust reference binaries (`ref-*`).
- `lib/diag/`: diagnostic library with `GK_FEAT_DIAG` enabled (`gk_headless_run`, offscreen rendering, `Window::layout_stats()`, and scripted platform dialogs).
- `lib/licences/`: collected third-party license texts.

Useful build variables:

| Variable | Default | Effect |
|---|---|---|
| `ENGINE=gpui` | `gpui-fast` | Build `lib/ship/` against upstream GPUI (`gpui-pre`) instead of `gpui-fast`. |
| `GK_JOBS=N` | CPU count | Limit Cargo parallelism on machines with less RAM. Peak memory during a cold `make lib` can reach 8 to 12 GB (and ~4 GB disk in `target/`). |
| `make lib-release` | host glibc | Links Linux binaries against `x86_64-unknown-linux-gnu.2.28` via `tools/zigbuild.sh` for portable release archives. |
| `make check-windows` | - | Runs `cargo xwin check --locked` on `gk-capi` (both normal and `--features diag`) from Linux. |

### 2.2 Building Go Examples (Dynamic vs. Static)

```sh
# Default dynamic build (CGO_ENABLED=0, purego):
examples/go/build.sh hello
LD_LIBRARY_PATH=lib/ship bin/hello-go   # or GK_LIB=lib/ship bin/hello-go

# Static build (-tags gkstatic, cgo + per-family dead-stripping):
GK_STATIC=1 examples/go/build.sh hello
bin/hello-go
```

`examples/go/build.sh` builds any program under `examples/go/<name>` into `bin/<name>-go` (and in a release archive, into `examples/go/<name>.bin`).

---

## 3. Regenerating Bindings (`make gen`)

Whenever you edit `core/wire/ops.txt`, `api/overrides.toml`, or anything in `gen/`:

```sh
make gen
```

This runs five fast Python/Go steps (a few seconds total):

1. `python3 core/wire/gen.py`: regenerates `core/src/ops.rs`, `core/include/gk_ops.h`, `go/gk/internal/rt/include/gk_ops.h`, and `go/gk/internal/rt/gen_ops.go`, and recomputes `API_HASH` from `core/wire/ops.txt` + `api/opcodes.lock`.
2. `python3 gen/gkgen.py gen`: builds the semantic model from `api/rustdoc/`, appends any new opcodes to `api/opcodes.lock`, emits Rust decoders (`core/src/gen/`) and Go packages (`go/gk/**/gen_*.go`, `go/gk/internal/rt/gen_use.go`), updates the generated index in `docs/API-GO.md`, and writes `api/coverage.json`. Fails immediately with `gen/out/no-rule.md` if any upstream signature matches no rule.
3. `gofmt -w go/gk examples/go`: formats generated Go files.
4. `python3 gates/conformance.py`: regenerates `examples/go/gallery/conf.go`.
5. `python3 gates/accept.py --check`: verifies accepted coverage/status invariants.

After `make gen`, run `gen/round.sh` to type-check the generated Rust quickly (`cargo check --release -p gk-capi`) without a full link, attributing any compiler error via `gen/out/lines.json` and writing `gen/out/report.md`. Run `python3 tools/status.py` (`make status`) to refresh the coverage block in `docs/STATUS.md`, or `python3 tools/status.py --check` (`make coverage`) to verify it is up to date.

---

## 4. Verification Gates

Run `make gates-quick` after everyday changes (a few minutes) and `make gates` before releasing or merging large changes (~20 to 25 minutes). Results are written to `gates/out/summary.md`.

### Complete Gate Catalog

| Gate / Target | Script | What It Verifies | Machine / Display Requirements |
|---|---|---|---|
| `make docs` | `gates/docs-go.sh` | Verifies every ` ```go ` snippet in `GETTING-STARTED-GO.md`, `GO.md`, and `API-GO.md` matches its `<!-- from: ... -->` program, builds & runs every tutorial/snippet/cited app offscreen, runs `docs-tutorial.script`, and checks `API-GO.md`'s index. | Offscreen `diag` library; safe in parallel with other work. |
| README check | `gates/readme-go.sh` | Verifies `README.md`'s Go block is byte-identical to `examples/go/hello/main.go`. | Instant, no display. |
| `make coverage` | `gates/coverage.sh` | Checks `tools/status.py --check` and `gates/coverage-ratchet.py` against `gates/coverage-baseline.json`. | Instant, no display. |
| `make cross` | `gates/cross.sh` | Cross-compiles `examples/go/hello` (`CGO_ENABLED=0`) for macOS (`arm64`, `amd64`) and Windows (`amd64`) from Linux. | Instant, no display. |
| `make misuse` | `gates/go-misuse.sh` | Runs all 92 misuse cases (runtime `*gk.Error` checks and compile-time type rejections across `examples/go/misuse*`). | Offscreen `diag` library. |
| `make proto` | `gates/go-proto.sh`, `core/tests/go/run.sh`, `gates/go-gensample.sh` | Exercises every turn kind, wire shape, handle lifetime tier, and domain protocol family in `docs/PROTOCOLS.md`. | Offscreen `diag` library. |
| `make go-apps` | `gates/go-apps.sh` | Builds and runs `notes`, `todo`, `reader`, `expenses`, `files`, `paint`, `panels`, `writerule`, and the 9 tutorial steps offscreen; compares `notes` pixels against Rust `ref-notes`. | Offscreen `diag` library. |
| `make go-static` | `GK_STATIC=1 gates/go-apps.sh ...` | Builds and runs the Go apps and tutorial statically (`-tags gkstatic`) to verify per-family `gk_use_*` registration and dead-stripping. | Offscreen `diag` library + C toolchain. |
| `make pixels` | `gates/go.sh pixels` / `gates/pixels.sh` | Renders matched Go and Rust scenes offscreen and asserts pixel identity. | Offscreen `diag` library. |
| `make gallery` | `gates/gallery.sh` | Renders every component gallery page in Go and Rust and diffs screenshots. | Offscreen `diag` library. |
| `make rebuild` | `gates/go-rebuild.sh` | Verifies incremental rebuild counts and view retention (`views_built` vs `views_reused`). | Offscreen `diag` library. |
| `make counter` | `gates/counter.sh` | Measures click-every-frame responsiveness and rebuild counts against `ref-counter` under `gates/display.sh`. | **Needs the machine to itself** (private compositor via `gates/display.sh`). |
| `make fps` / `make go-fps` | `gates/fps.sh` | Runs interleaved `rust`, `go`, `rust2` frame-cost rounds at 60/240/1,000 Hz under `gates/display.sh` (`kwin_wayland`, `gamescope`, or `xvfb`), enforcing `GK_BAR=1.03` + Rust noise floor. | **Needs the machine to itself** (timing-sensitive; no competing CPU/GPU load). |
| `make memory` | `gates/memory.sh` | Runs sustained allocation/release/scroll cycles (plus a leak-detector self-test in full `make gates`) and asserts RSS & handle count stability. | **Needs the machine to itself** (memory & timing sensitive; uses `gates/display.sh`). |

### Private Headless Compositor (`gates/display.sh`)

Live-window gates (`fps`, `counter`, `memory`) launch their own headless display server through `gates/display.sh` so they never disturb your desktop session:

- `kwin_wayland` for 60 Hz runs
- `gamescope` for high-refresh runs (240 Hz, 1,000 Hz)
- `GK_DISPLAY=xvfb` as a 60 Hz X11 fallback when Wayland compositors are unavailable

Never run `fps`, `counter`, or `memory` concurrently with heavy builds or other benchmarks, or Rust-vs-Rust baseline jitter (`> 5%`) will discard rounds and fail the gate.

---

## 5. Adding a New Binding or Protocol Feature

### 5.1 Adding a Hand-Written Wire Op

1. Append the new `call`, `node`, `shape`, or `hkind` entry to the appropriate reserved block in `core/wire/ops.txt`. **Never renumber or reuse retired opcodes.**
2. Document the exact byte layout and semantics in `docs/PROTOCOLS.md`.
3. Run `python3 core/wire/gen.py` (or `make gen`) to regenerate `ops.rs`, `gk_ops.h`, `gen_ops.go`, and recompute `API_HASH`.
4. Implement the Rust decoder/handler in `core/src/` and the Go wrapper in `go/gk/internal/rt/` + the appropriate public Go package.
5. Add protocol tests in `examples/go/proto/`, `examples/go/port/`, or `core/tests/go/` (and misuse tests in `examples/go/misuse*` if new misuse cases exist).
6. Run `make lib && make proto && make misuse`.

### 5.2 Adding a New Generator Rule

1. Inspect the unmatched signature in `gen/out/no-rule.md` (produced by `make gen`).
2. Add the type/pattern descriptor in `gen/gk_model.py` (or the relevant family module: `gk_app.py`, `gk_lists.py`, `gk_values.py`, `gk_overlay.py`, `gk_editor.py`, `gk_items.py`, `gk_comp6.py`) or declare an instantiation/override in `api/overrides.toml`.
3. Add matching codegen support in `gen/gk_rust.py` and `gen/gk_go.py` (and `gen/gk_go_kinds.py` / `gk_go_bridges.py` as needed).
4. Run `make gen` (which appends new opcodes to `api/opcodes.lock`), then `gen/round.sh` to verify generated Rust compiles with zero new quarantine entries.
5. Rebuild with `make lib` and run `make gates-quick`.

---

## 6. Bumping Upstream Versions (`gpui-kit` / `gpui-fast`)

When upgrading the pinned upstream crates:

1. Update versions in `Cargo.toml`, `core/Cargo.toml`, and `rust-toolchain.toml` (if the Rust toolchain version changed), plus version strings in `core/src/lib.rs` and `README.md`.
2. Run `gen/extract.sh` (requires nightly rustdoc producing format 61 JSON), then strip and commit the snapshot:
   ```sh
   python3 gen/gkgen.py strip <extract-dir>
   ```
   This refreshes `api/rustdoc/*.json.gz` and `api/rustdoc/docs.json.gz`.
3. Refresh `api/clone.json` (which records which upstream library objects implement `Clone`):
   ```sh
   python3 tools/clone_scan.py
   ```
4. Run `make gen`. Resolve any newly unmatched upstream signatures listed in `gen/out/no-rule.md`.
5. Run `gen/round.sh` to check generated Rust against the new upstream APIs and inspect `gen/out/report.md` for any newly quarantined items.
6. Build `make lib`, run `make gates` (including `pixels` and `gallery` to catch upstream visual changes), and verify all examples.
7. If API coverage increased cleanly, accept the new baseline with `gates/coverage.sh --accept` (updating `gates/coverage-baseline.json`), then refresh `docs/STATUS.md` with `make status` and `python3 gates/status.py`.

---

## 7. Documentation Sync Rules and Release Packaging

### 7.1 Generated and Synced Documentation Regions

- **Never hand-edit generated regions**:
  - The component index in `docs/API-GO.md` (written by `gen/gk_apidoc.py`).
  - `examples/go/gallery/conf.go` (written by `gates/conformance.py`).
  - `<!-- coverage:begin -->..<!-- coverage:end -->` and `<!-- gates:begin -->..<!-- gates:end -->` in `docs/STATUS.md` (written by `tools/status.py` and `gates/status.py`).
- **Keep guide snippets synced**:
  - Every ` ```go ` block in `docs/GETTING-STARTED-GO.md`, `docs/GO.md`, and `docs/API-GO.md` must immediately follow `<!-- from: PATH -->` and match exact lines (tabs included, with `// ...` eliding omitted lines) from that program.
  - `README.md`'s Go block must stay byte-identical to `examples/go/hello/main.go`.
  - Verify with `gates/docs-go.sh` and `gates/readme-go.sh`.

### 7.2 Assembling and Testing Release Archives

```sh
# On Linux:
make lib-release
make dist
tools/dist-test.sh dist/goui-kit-<ver>-linux-x86_64.tar.zst
```

`tools/dist.sh` creates `dist/goui-kit-<ver>-<platform>/` (`.tar.zst` on Linux and macOS, `.zip` on Windows) containing `lib/`, `go/`, `include/`, `examples/go/`, `docs/`, `VERSIONS`, `THIRD_PARTY.md`, and `licences/`. `tools/dist-test.sh` unpacks the archive in isolation and verifies both default purego and `-tags gkstatic` builds plus a live window run. Cross-platform release CI lives in `.github/workflows/platforms.yml`.
