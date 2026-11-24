# gpui-kit-bindings

Write desktop apps in **Go** with [GPUI](https://www.gpui.rs) and
[GPUI-kit](https://gpui-kit.com), the GPU-accelerated UI framework from the Zed editor and the
component library built on it.

GPUI and GPUI-kit are Rust. This project wraps them in one prebuilt library with a small C
interface (`libgpuikit`) and puts native Go packages on top: `gpuikit.dev/gk` and one package per area. The packages are
generated from GPUI-kit's own API, so you get
the kit's whole component set (buttons, inputs, lists, tables, dialogs, menus, docks, charts,
editors and more) under names that follow fixed rules from the Rust documentation.

**You don't need Rust.** You download the library, point your compiler at it, and write ordinary
Go.

The prebuilt library runs GPUI-kit on gpui-fast, the alternative GPUI engine the kit supports
through a Cargo feature. If you'd rather use upstream GPUI, you can build the library from source
with either engine; [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) explains how. Your app code is the
same either way.

Supported systems:

| system | download |
|---|---|
| Linux x86-64, glibc 2.28 or newer | `gpuikit-VERSION-linux-x86_64.tar.zst` |
| macOS 11 or newer, Apple silicon and Intel (one universal library) | `gpuikit-VERSION-macos-universal.tar.zst` |
| Windows 10 or newer, x86-64 | `gpuikit-VERSION-windows-x86_64.zip` |

## Start here

The guide installs what you need, runs an example, and then walks you through
building a small app in nine short steps (about 15 minutes).

| | |
|---|---|
| Getting started and tutorial | [docs/GETTING-STARTED-GO.md](docs/GETTING-STARTED-GO.md) |
| From GPUI-kit's Rust docs to your call, and every component | [docs/API-GO.md](docs/API-GO.md) |
| How the Go package works | [docs/GO.md](docs/GO.md) |

## Hello, world

A window with a button, a counter and a text input:

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
	name  editor.InputState // a handle to the kit's InputState, which lives in the library
	root  gk.View
}

func (a *App) init(cx *gk.Cx) { a.name = editor.NewInputState(cx).Placeholder("Your name").Build() }

func (a *App) add(cx *gk.Cx) { a.count++ } // marks its view for redraw

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).
		Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(kit.Button("add").Primary().Label("Add").OnClickDo(a.add)).
			Child(kit.Label(fmt.Sprintf("clicked %d times", a.count)))).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))).
		Child(kit.Label("hello " + a.name.Value(cx))) // a read: redrawn when it changes
}

func main() {
	a := &App{}
	if err := gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "Hello", 480, 320, a.root)
	}); err != nil {
		log.Fatal(err) // a mistake in app code: the message names the file and line
	}
}
```

A few things to notice:

- **Your state is your own struct.** The binding never copies it or owns it.
- **A view function describes the window** from that state. It runs again whenever the view is
  redrawn, so it only describes; it never changes anything.
- **A handler is a plain function** that changes your state. When it returns, the view it belongs
  to is redrawn. There is no `notify` to remember.
- **Styles are tokens**: `s.Gap3`. Everything else keeps GPUI-kit's name, in Go's capitalization
  (`Button`, `Primary`, `Label`, `OnClick`).
- **Kit state** (an input's text, a list's selection) lives in the library; your struct keeps a
  handle to it, made once in `init`.

## Get the library

Download the archive for your system from the
[releases page](https://github.com/ibanks42/gpui-kit-bindings/releases). Each release also has a
`SHA256SUMS` file to check the download with (`sha256sum -c --ignore-missing SHA256SUMS`, or
`shasum -a 256 -c --ignore-missing SHA256SUMS` on macOS).

Inside the archive:

| path | what |
|---|---|
| `lib/` | the library, which your app loads when it starts (`libgpuikit.so`, `libgpuikit.dylib` or `gpuikit.dll`), and the same as a static library, `libgpuikit_static.a`, for an app that links it into itself |
| `go/` | the Go module `gpuikit.dev`: package `gk` (the core) and one package per area (`kit`, `editor`, `table`, `chart`, `overlay`, `dock`, `motion`, `base`), and `s`, `theme`, `palette` for styles and colors |
| `include/` | the C header, if you want to bind another language |
| `windows/` | Windows only: the manifest every program needs (see below) |
| `examples/go/` | six example apps, with a `build.sh` |
| `docs/` | the guides and the tutorial programs |
| `VERSIONS` | what the library was built from and what it needs |
| `THIRD_PARTY.md`, `licences/` | the licences of everything compiled into the library |

## Build and run

Unpack the archive and try an example:

```sh
tar --zstd -xf gpuikit-0.0.2-linux-x86_64.tar.zst
cd gpuikit-0.0.2-linux-x86_64

examples/go/build.sh notes && examples/go/notes.bin
```

To build your own app, put its folder next to the unpacked download.

**Go** (you need Go 1.27 or newer; no C compiler). Your `go.mod` points `gpuikit.dev` at the
download; the only module fetched from the network is purego, once:

```
require gpuikit.dev v0.0.0

replace gpuikit.dev => ../gpuikit-0.0.2-linux-x86_64/go
```

```sh
go mod tidy && go build -ldflags="-s -w" -o myapp.bin .
ln -s ../gpuikit-0.0.2-linux-x86_64/lib/libgpuikit.so .    # the app loads it from beside itself
```

Your app loads the library when it starts, through [purego](https://github.com/ebitengine/purego),
so `go build` needs no C compiler and cross-compiles (`GOOS=windows go build`). It finds the
library in `GK_LIB`, beside itself, in its `lib/` or `../lib/` folder, or where the system loader
looks; ship the two together.

To ship one file instead, build with `-tags gkstatic`: the static library is linked into the
program through cgo (this needs a C compiler), and **only the components it uses** end up in it:
call `kit.Button` and the button code is linked in, never call `chart.LineChart` and the chart
code is left out. There is nothing to list or import for it:

```sh
CGO_LDFLAGS="-L$PWD/../gpuikit-0.0.2-linux-x86_64/lib" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,--gc-sections" -o myapp.bin .
```

The `-ldflags` part tells the linker to drop what isn't used (Go doesn't let a package ask for
that itself); without it the app still works, it just carries every component (about 100 MB
instead of about 50 MB for a small app). [docs/GO.md](docs/GO.md) section 8 explains both ways.

Things to know per system (the guides cover each in detail):

- **macOS**: the library is `libgpuikit.dylib`, universal, so you can build for either kind of
  Mac (`GOARCH=amd64` or `arm64`) and join the two with `lipo`; in an app bundle it goes in
  `Contents/Frameworks`. For `-tags gkstatic` the flag is `-extldflags=-Wl,-dead_strip`.
- **Windows**: put `gpuikit.dll` beside the `.exe`. The `.exe` has the Windows manifest inside it
  (without it the program would exit before `main`: the library uses common controls 6, which
  Windows only loads when a manifest asks for it). `-tags gkstatic` needs MinGW-w64 gcc with LLVM's
  linker, lld, and the flag `-ldflags="-s -w '-extldflags=-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres'"`.
- The library and the package check each other's version at start. If you mix files from two
  downloads, the app stops with a message saying so instead of misbehaving.

## Bigger examples

The download includes these apps:

- `hello`: the program above.
- `counter`: a button, an input echoed into a label, and a list of 10,000 rows.
- `notes`: the kit's dock with two panels, a list of notes and an editor, a File menu with
  keyboard shortcuts, a confirmation dialog, and a light/dark theme switch.
- `todo`: projects, a task list with custom rows, filters, search, a menu, themes, saving to a
  JSON file, and a background thread.
- `reader`: 3,000 articles in a virtual list, Markdown in tabs, an icon toolbar with tooltips, a
  settings dialog, and a loading thread.
- `expenses`: a form with validated fields, a select, and a bar chart.

## Working on the bindings

The rest of this repository is the machinery that produces the download: a generator (Python) that
reads GPUI-kit's API and writes the Rust and Go code, the Rust library itself, and tests that
compare every example against a pure-Rust twin image by image. If you want to build the library from
source, regenerate the packages, or contribute, read [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md).

For how it works inside: [docs/DESIGN.md](docs/DESIGN.md) (the architecture) and
[docs/PROTOCOLS.md](docs/PROTOCOLS.md) (the C interface and the wire format). What is and isn't
bound yet is counted in [docs/STATUS.md](docs/STATUS.md).
