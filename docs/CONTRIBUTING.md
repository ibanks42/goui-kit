# Working on the bindings

This guide is for people who want to change the bindings themselves: build the library from
source, regenerate the Odin and Go packages, and run the tests. If you only want to write an app,
you don't need any of this. Grab a prebuilt download and follow
[GETTING-STARTED.md](GETTING-STARTED.md) (Odin) or [GETTING-STARTED-GO.md](GETTING-STARTED-GO.md)
(Go).

## How the pieces fit

- **The library** (`core/`) is Rust. It wraps GPUI-kit and exposes a small C interface
  (`core/include/gpuikit.h`). It compiles to `libgpuikit.so`, `libgpuikit.dylib` or `gpuikit.dll`.
- **The generator** (`gen/`) is Python. It reads GPUI-kit's API (extracted from rustdoc, committed
  in `api/rustdoc/`) and writes the Rust decoders in `core/src/gen/`, the Odin package's
  `odin/gk/gen_*.odin` and the Go package's `go/gk/gen_*.go`. The `rt_*` and `kit_*` files next to
  them are written by hand.
- **The reference apps** (`ref/`) are the example apps written in plain Rust. The tests draw the
  Odin and Go apps and these Rust apps and compare the pixels.
- **The tests** (`gates/` and `core/tests/`) are shell and Python scripts.

Most changes touch only the generator, the hand-written runtime files or the examples, and don't
need a Rust build at all, as long as you have a library in `lib/`.

## What you need

| for | you need |
|---|---|
| regenerating the bindings | Python 3 (standard library only); `gofmt` if you want the Go output formatted |
| building the library | Rust (`rustup` picks up the version in `rust-toolchain.toml`), a C compiler, and your platform's GUI development packages (below) |
| the Odin side | the Odin compiler (`make odin` downloads the tested release) and clang |
| the Go side | Go 1.27 or newer and a C compiler (the bindings use cgo) |
| the tests | Linux, plus `gamescope` or `kwin_wayland` (or `Xvfb`) for the private display the tests draw in, and `bc` |

On Debian or Ubuntu, the development packages for the library are:

```sh
sudo apt install pkg-config libfreetype-dev libfontconfig-dev libxkbcommon-dev \
  libxkbcommon-x11-dev libxcb1-dev libxcb-xkb-dev libx11-dev libwayland-dev clang
```

On macOS you need Xcode (for the Metal compiler) and both Rust targets:
`rustup target add aarch64-apple-darwin x86_64-apple-darwin`. On Windows you need Visual Studio's
C++ tools and the Windows SDK; run the scripts from Git Bash.

## Building the library

```sh
make lib
```

This runs `cargo build --release` in the normal `target/` folder and copies the results to `lib/`:

- `lib/ship/` is the library apps link against. It also holds `libgpuikit_static.a`, the same
  library as a static archive (Go apps link it and keep only the component families they use;
  [GO.md](GO.md) section 8), with, on Linux, `libgpuikit_sys.so`, a linker script naming the system
  libraries the archive needs, and on Windows `libgpuikit_sys.a`, the import library of the Windows
  API it links. On Windows the archive is built for Rust's MinGW target (Go's cgo links with
  MinGW-w64), with one section per function (`-Zfunction-sections`, so a Go app's linker can drop
  what it doesn't use): that needs the Rust toolchain hosted on MinGW (`rustup toolchain install
  1.99.1-x86_64-pc-windows-gnu`) and MinGW-w64's gcc, dlltool and windres on `PATH`. Go apps embed
  the Windows manifest through `go/gk/gpuikit_windows_amd64.syso`; after editing
  `core/windows/gpuikit.manifest`, regenerate it with
  `windres -O coff -i core/windows/gpuikit.rc -o go/gk/gpuikit_windows_amd64.syso`.
- `lib/diag/` is a diagnostic build with offscreen rendering and scripted input. The tests use it.
- `lib/licences/` holds the licence text of every crate linked into the library.
- On Linux, the Rust reference apps (`ref-counter`, `ref-gallery` and so on) go next to each library.

Expect 5 to 15 minutes for the first build, about 4 GB of disk in `target/`, and up to 12 GB of
memory while cargo is busiest. If your machine has less memory, limit the parallel jobs:
`GK_JOBS=4 make lib`. Rebuilds after a small change take a minute or two.

A few options:

- `make lib ENGINE=gpui` builds the shipped library on upstream GPUI instead of gpui-fast (the
  default). The diagnostic library always uses gpui-fast, because the counters the tests read
  exist only there.
- `make lib-release` (Linux) links the library against glibc 2.28 using zig, so it also runs on
  older distributions. This is how the published downloads are built. It downloads a pinned zig
  into `target/toolchain/` the first time and needs `cargo-zigbuild` on your PATH
  (`tools/zigbuild.sh fetch-cargo-zigbuild DIR` fetches the pinned version).
- `make check-windows` type-checks the library for Windows from Linux. It needs `cargo-xwin`.

`tools/build-lib.sh` does the actual work and documents the environment variables it reads at
the top of the file.

## Regenerating the bindings

```sh
make gen
```

This takes a few seconds and needs only Python. It rewrites everything generated from
`api/rustdoc/`: the Rust decoders, the Odin and Go packages, the opcode tables, the coverage
numbers in `api/coverage.json` and the table in `docs/STATUS.md`. It fails if an upstream function
has a signature none of the generator's rules handle.

The Rust compiler is the final judge of the generated Rust. `gen/round.sh` runs `cargo check` and,
for each compile error, sets the offending API item aside and regenerates, until the library
compiles. You only need it after changing the generator's rules or the upstream version.

When the pinned GPUI-kit version changes, extract its API again first:

```sh
gen/extract.sh                          # rustdoc JSON into gen-json/, a few minutes
python3 gen/gkgen.py strip gen-json/public
make gen
```

## Running the tests

The tests run on Linux. They draw every frame in a private, headless display, so nothing appears
on your screen, and they need `lib/ship` and `lib/diag` from `make lib`.

```sh
make gates-quick     # the set to run after a change: a few minutes
make gates           # everything, about 25 minutes
```

Both print one summary table at the end (also written to `gates/out/summary.md`), with the full
log in `gates/out/logs/gates.log`. A few known failures are listed in `gates/expected.txt` with the
reason; they show as XFAIL and don't fail the run.

The tests are also available one at a time:

| command | what it checks | time |
|---|---|---|
| `make proto` | the runtime's protocol tests, one per behaviour described in `docs/PROTOCOLS.md` | about 20 s |
| `make examples` | the Odin examples build | a minute |
| `make pixels` | the Odin apps draw exactly the same pixels as their Rust twins | a minute |
| `make gallery` | every page of the component gallery, Odin against Rust | a few minutes |
| `make misuse` | mistakes in app code are caught, with a clear message, at compile or run time | a minute |
| `make docs` | every code snippet in the guides is part of a program that builds, and both tutorials run | a few minutes |
| `make counter` | a click on every frame costs no more than in Rust | a minute |
| `make fps` | frame cost against Rust for one scene (`HZ=240 SCENE='panels 60 64 2'`) | a few minutes |
| `make memory` | memory stays flat over long runs | a few minutes |
| `make rebuild` | an app rebuilds quickly after a one-line edit | under a minute |
| `make go-test` | the Go examples, protocol tests, misuse cases and pixels | about a minute |
| `make go-apps` | the Go apps and tutorial draw the same images as their Odin twins | a few minutes |
| `make go-static` | the Go and Odin apps built the way an app is (static, only the families each one uses) | a few minutes |
| `make go-fps` | frame cost of Go against Rust and Odin | about ten minutes |
| `make cross` | both front ends compile for macOS and Windows (checked from Linux) | under a minute |

The tests link the shared library (Go: the build tag `gkshared`, set in `go/env.sh`; Odin:
`-define:GK_SHARED=true` in the test scripts), so one program can run against either `lib/ship` or
`lib/diag`. `make go-static` builds the Go and Odin apps the way a user's app is built, statically
against `lib/diag`, and runs their scripted sessions: if a generated function forgot to link its
component family, the app stops and the check fails.

Each test needs about 3 GB of memory at most. They run one after another, never in parallel.

The private display uses `gamescope` (or `kwin_wayland` at 60 Hz). On a machine where neither can
reach the GPU, or that has no GPU, set `GK_DISPLAY=xvfb` to use Xvfb instead; it only paces at
60 Hz, so also set `GK_RATES=60`. Scratch files go to `$TMPDIR/gk-gates` (set `GK_SCRATCH` to move
them) and are deleted when each test ends.

The scripts find Odin as `odin` on your PATH; set `ODIN=/path/to/odin` to use another one.

## Making a download

```sh
make dist
```

This assembles what an app developer downloads (the library, the Odin package, the Go module,
the examples, the guides and the licences) into `dist/`, for the platform you're on, from the
library in `lib/`. The published releases are built by CI (`.github/workflows/platforms.yml`) on
Linux, macOS and Windows and attached to a GitHub release when a `vVERSION` tag is pushed.

## Cleaning up

`make clean` deletes `bin/` and `gates/out/`. `lib/` stays, and so does `target/`; delete
`target/` yourself (or run `cargo clean`) when you want the disk space back.
