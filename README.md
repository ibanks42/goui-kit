# gpui-kit-bindings

Go bindings for the Rust desktop UI stack [GPUI-kit](https://gpui-kit.com) (0.11.0), running on `gpui-fast` (0.1.19) by default and also buildable on upstream GPUI (`gpui-pre` 0.3.14).

The Rust side compiles into a single prebuilt library (`libgpuikit`) behind seven C functions and one callback. Go applications build against the `gpuikit.dev` module without installing Rust or a C compiler.

## Quick Example

This is the complete program in [examples/go/hello/main.go](examples/go/hello/main.go):

```go
package main

import (
	"fmt"
	"log"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

type App struct {
	count int
	name  editor.InputState
	root  gk.View
}

// init runs once when the window mounts the root view, which is when window-backed
// kit state (such as InputState) can be created.
func (a *App) init(cx *gk.Cx) { a.name = editor.NewInputState(cx).Placeholder("Your name").Build() }

// add is an event handler. After it returns, the view is marked dirty and re-described.
func (a *App) add(cx *gk.Cx) { a.count++ }

// view describes the current UI tree. Reading a.name.Value records a dependency so
// typing into the input automatically triggers a redraw of this view.
func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(kit.Button("add").Primary().Label("Add").OnClickDo(a.add)).
			Child(kit.Label(fmt.Sprintf("clicked %d times", a.count)))).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))).
		Child(kit.Label("hello " + a.name.Value(cx)))
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "Hello", 480, 320, a.root)
	}); err != nil {
		log.Fatal(err)
	}
}
```

## How It Works

Go code never calls into GPUI on every rendered pixel or inside the middle of a frame. Instead, the binding separates application logic from frame rendering:

- **Views write byte tapes.** When a view is dirty, Go runs its view function once and encodes the resulting element tree into a compact little-endian byte buffer (a tape). If the new tape matches the previous one byte-for-byte, the binding skips notifying the library.
- **Rust owns the real UI tree.** The library stores each view's tape on a GPUI entity (`GkView`) and decodes it into real GPUI-kit elements when GPUI builds that view. Unchanged views stay retained by `gpui-fast`.
- **All Go code runs in turns on the UI thread.** Events, timers, frame hooks, and background completions arrive through a single callback. Background goroutines hand work back to the UI thread with `gk.Go` or `gk.Post`.
- **Two linking modes.** By default, Go binaries load `libgpuikit.so`, `libgpuikit.dylib`, or `gpuikit.dll` at startup using `purego` with `CGO_ENABLED=0`, so no C compiler is required and standard `GOOS`/`GOARCH` cross-compilation works. Building with `-tags gkstatic` links `libgpuikit_static.a` via cgo and lets the linker discard unused component families.

## Building From This Repository

App developers using a release archive only need Go 1.27 or newer. To build and test directly from a git checkout on Linux:

1. **Build the Rust library and reference apps** (needs Rust 1.99.1, defined in `rust-toolchain.toml`):

   ```sh
   make lib
   ```

   This writes `lib/ship/` (the library apps link or load), `lib/diag/` (the diagnostic build with offscreen rendering and scripted input), the pure-Rust reference apps (`ref-*`), and third-party license texts (`lib/licences/`).

2. **Build and run the Hello example**:

   ```sh
   examples/go/build.sh hello
   LD_LIBRARY_PATH=lib/ship bin/hello-go
   ```

3. **Run the quick verification suite**:

   ```sh
   make gates-quick
   ```

   Run `make help` for the full list of generation, build, and test targets.

## Repository Layout

| Path | Contents |
|---|---|
| `core/` | Rust C ABI library (`gk-capi`), tape decoder, runtime shims, wire tables (`core/wire/ops.txt`), and frame meter (`core/meter/`). |
| `go/` | The `gpuikit.dev` Go module: `gk` plus sub-packages (`kit`, `editor`, `table`, `overlay`, `chart`, `dock`, `motion`, `base`, `s`, `theme`, `palette`) and `internal/rt`. |
| `gen/` | Python 3 code generator that turns the rustdoc JSON extract in `api/rustdoc/` into Rust decoders, Go packages, and API indexes. |
| `api/` | Committed rustdoc extract, `overrides.toml`, append-only `opcodes.lock`, `quarantine.json`, and machine-generated `coverage.json`. |
| `examples/go/` | Example applications (`hello`, `counter`, `notes`, `todo`, `reader`, `expenses`, `files`, `paint`, `panels`, `table`, `writerule`), the component gallery, protocol tests, and misuse tests. |
| `ref/` | Pure-Rust reference applications used by the gates to verify pixel identity, memory stability, and frame cost. |
| `gates/` | Automated test harnesses for pixels, protocol behavior, misuse cases, documentation snippets, cross-compilation, memory, and frame cost. |
| `tools/` | Build, packaging (`dist.sh`), license collection, and status update scripts. |
| `docs/` | Guides, tutorial programs, architecture notes, protocol specification, and status reports. |

## Documentation

- [docs/GETTING-STARTED-GO.md](docs/GETTING-STARTED-GO.md): installation, building apps, and a nine-step Go tutorial.
- [docs/API-GO.md](docs/API-GO.md): mapping upstream GPUI-kit documentation to Go, plus the generated component index.
- [docs/GO.md](docs/GO.md): package layout, naming rules, handle lifetimes, error model, and dynamic vs. static linking.
- [docs/DESIGN.md](docs/DESIGN.md): architecture of the C ABI, tape format, turns, generator pipeline, and verification gates.
- [docs/PROTOCOLS.md](docs/PROTOCOLS.md): wire encoding and runtime contracts between hand-written and generated code.
- [docs/STATUS.md](docs/STATUS.md): machine-generated API coverage counts and latest gate results.
- [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md): working on the library, generator, upstream pins, and gates.
