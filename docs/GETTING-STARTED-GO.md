# Getting started with Go

This guide takes a Go programmer who has never used GPUI from the download to a working desktop
app. Part 1 installs what you need and runs the example that comes with the download. Part 2 is a
15-minute tutorial: a shopping list app, built in nine steps, each a complete program you can run.
After it, [API-GO.md](API-GO.md) explains how to read GPUI-kit's own documentation
(gpui-kit.com/docs) and know the Go call, and lists every component.

Every snippet in this guide is cut from the tutorial's programs in `docs/tutorial-go/`, and those
programs are built and run by the project's tests, so the code you read here compiles against the
package you download.

## Part 1: install and run

### What you need

- One of the three systems there is a download for:
  - **Linux** on x86_64 with glibc 2.28 or newer, and the libraries `libxkbcommon`,
    `libxkbcommon-x11` and `libxcb` (installed on any desktop Linux);
  - **macOS** 13 Ventura or newer (Go 1.27, which this package needs, runs on nothing older), on
    Apple silicon or Intel: the library is universal (arm64 and x86_64, minimum macOS 11.0);
  - **Windows** 10 or newer on x86_64, with Direct3D 11 (any Windows 10 machine).
- Go 1.27 or newer ([go.dev/doc/install](https://go.dev/doc/install)); the package was tested with
  Go 1.27.3. `go version` tells you which one you have.
- A C compiler, because the package calls the library through cgo. Without one, Go turns cgo off
  by itself and the build stops with
  `undefined: gk_needs_cgo__install_gcc_or_clang_and_build_with_CGO_ENABLED_1`.
  - Linux: **gcc**, which Go uses by default (`sudo dnf install gcc` on Fedora, `sudo apt install
    gcc` on Debian and Ubuntu), or clang with `CC=clang` set.
  - macOS: the Xcode command-line tools (`xcode-select --install`), whose `clang` Go finds as `gcc`.
  - Windows: MinGW-w64 **gcc** on `PATH` (Go's cgo does not use Visual Studio's compiler): MSYS2's
    `pacman -S mingw-w64-ucrt-x86_64-gcc`, then `C:\msys64\ucrt64\bin` on `PATH`, or w64devkit.
    `examples/go/build.sh` is an `sh` script: run it in Git Bash or MSYS2 (the `go build` it runs
    is shown below, for PowerShell or cmd).
- No Rust. The Rust half (GPUI, GPUI-kit and the binding's runtime) is one prebuilt library in the
  download. (If you want to build that library yourself, see [CONTRIBUTING.md](CONTRIBUTING.md).)

### Download and unpack

The download is one file per system: `gpuikit-0.0.2-linux-x86_64.tar.zst` (about 66 MB),
`gpuikit-0.0.2-macos-universal.tar.zst` or `gpuikit-0.0.2-windows-x86_64.zip`, from release `v0.0.2`
of the bindings' GitHub releases
([github.com/ibanks42/gpui-kit-bindings/releases](https://github.com/ibanks42/gpui-kit-bindings/releases)).
`SHA256SUMS` beside them checks what you downloaded: `sha256sum -c --ignore-missing SHA256SUMS`
(on macOS `shasum -a 256 -c --ignore-missing SHA256SUMS`). Unpack it anywhere:

```sh
tar --zstd -xf gpuikit-0.0.2-linux-x86_64.tar.zst      # or: zstd -dc gpuikit-0.0.2-linux-x86_64.tar.zst | tar -x
cd gpuikit-0.0.2-linux-x86_64
```

On macOS the same with `macos-universal` (`zstd` is in Homebrew); on Windows unpack the `.zip` with
Explorer or `tar -xf gpuikit-0.0.2-windows-x86_64.zip`. The three hold the same files except the
library's:

| path | what |
|---|---|
| `lib/libgpuikit_static.a` | the library (GPUI-kit 0.11.0 on the gpui-fast engine), which your program links into itself; on macOS one universal file for Apple silicon and Intel, on Windows in MinGW's format |
| `lib/libgpuikit_sys.so`, `lib/libgpuikit_sys.a` | Linux: a small linker script naming the system libraries it needs; Windows: the import library of the Windows API it uses |
| `windows/` | Windows: the manifest a program embeds, and `gpuikit.rc`, which embeds it |
| `go/` | the Go module `gpuikit.dev`: the package `gpuikit.dev/gk` your app imports, and `s`, `theme`, `palette` beside it |
| `README.md` | where to start |
| `examples/go/` | `hello`, `counter`, `notes`, `todo`, `reader`, `expenses` in Go, and `build.sh`, which builds them |
| `docs/` | this guide, [API-GO.md](API-GO.md), [GO.md](GO.md), the tutorial's programs in `docs/tutorial-go/` |
| `VERSIONS` | what the library was built from, and what it needs (glibc, macOS, Windows) |
| `THIRD_PARTY.md`, `licences/` | the licences of everything inside the library |

### Build and run hello

```sh
examples/go/build.sh hello      # the first build compiles the package, about 15 s; after an edit, half a second
examples/go/hello.bin           # a window with a button, a label and an input (hello.exe on Windows)
```

`build.sh` is one `go build` command, and builds any app folder too
(`examples/go/build.sh ../groceries`); the command itself is below. If the window opens,
everything works: Go, cgo, the package and the library. The other examples are built the same way
(`examples/go/build.sh todo`, `reader`, `notes`, `counter`, `expenses`). The first build compiles
package `gk` (a few megabytes of generated Go and its cgo part) into Go's build cache (about 200 MB):
about 15 s with an empty cache, then about 1.5 s for the next example and half a second for a
rebuild after a one-line edit. Every build after the first reuses it, as long as `CGO_LDFLAGS` is
the same: the link flags are part of cgo's cache key, so an app built with other flags compiles the
package once more (about 12 s) for itself.

If it does not start, the first line it prints starts with `gk:` and says why. The usual ones:

- `the library and this package are from different versions`: the app was built against the `go/`
  folder of one download and the `lib/` of another. Build against the `go/` folder that came with
  the library.
- `cannot find -lgpuikit_static` when building: `CGO_LDFLAGS` does not name the download's `lib/`
  folder.
- `the app has no display to open its window on` (Linux): neither `DISPLAY` nor `WAYLAND_DISPLAY` is set
  (an ssh session, a container). Run it in a desktop session, or offscreen (Part 3, "Testing
  without a display").

### Your own app

An app is a Go module that requires `gpuikit.dev` and points it at the download's `go/` folder.
Put the app's folder beside the unpacked download, with this `go.mod`:

```
module groceries

go 1.27

require gpuikit.dev v0.0.0

replace gpuikit.dev => ../gpuikit-0.0.2-linux-x86_64/go
```

and build it with:

```sh
G=$PWD/../gpuikit-0.0.2-linux-x86_64
CGO_LDFLAGS="-L$G/lib" go build -ldflags="-s -w -extldflags=-Wl,--gc-sections" -o groceries.bin .
./groceries.bin
```

- `require` and `replace` are the two lines that matter: `gpuikit.dev` is not fetched from the
  network, the `replace` names a folder (relative to `go.mod`, or absolute), and no `go.sum` entry
  is needed for it. `go build` works offline.
- To keep the package in the app's repository instead (a vendored copy), copy the download's
  `go/` folder into it, say as `third_party/gpuikit`, and write
  `replace gpuikit.dev => ./third_party/gpuikit`. Copy the whole folder: package `gk` includes its
  C header (`gk/include/`), which `go mod vendor` would leave out.
- `CGO_LDFLAGS` tells the linker where the library is. Use the same value for every build (a
  script or a Makefile): the first build of the app compiles package `gk` for those flags, about
  12 s, and every rebuild after an edit then takes half a second.
- The library is linked into the program, and only the components your app actually calls end
  up in it. Use `kit.Button` and the button code comes along; never touch the charts and
  they stay out. You don't list or import anything for this. The `-ldflags` part is what tells the
  linker to drop the unused parts: without it the app still works but carries every component
  (about 100 MB instead of about 50 MB for a small app). The program is one file, so shipping it
  means copying that file. [GO.md](GO.md) section 8 explains the details.
- The app imports `"gpuikit.dev/gk"` (the app, views, windows, `gk.Div` and the styles), the
  package of each area it uses (`"gpuikit.dev/gk/kit"` for buttons and labels,
  `"gpuikit.dev/gk/editor"` for inputs, `"gpuikit.dev/gk/table"` for lists and tables, and
  `motion`, `chart`, `overlay`, `dock`, `base`; [GO.md](GO.md) section 1 says what is where), and
  `"gpuikit.dev/gk/s"`, `"gpuikit.dev/gk/theme"` or `"gpuikit.dev/gk/palette"` when it uses style
  tokens or colors. A program compiles only the packages it imports.

**macOS.** The same, with Apple's spelling of the flag:
`go build -ldflags="-s -w -extldflags=-Wl,-dead_strip"`. The program has the library inside it,
so an app bundle needs nothing in `Contents/Frameworks` for it. `go build` builds for the Mac it
runs on; the library is universal, so the same `CGO_LDFLAGS` build for the other
kind too: `GOARCH=amd64 CC='clang -arch x86_64' go build -o groceries-x86_64.bin .` on Apple
silicon (`GOARCH=arm64 CC='clang -arch arm64'` on Intel), and `lipo -create -output groceries.bin
groceries-arm64.bin groceries-x86_64.bin` makes one program for both. `examples/go/build.sh` passes
`GOARCH` and `CC` on. `gk.Run` must be
called from `main` on every system, and on macOS it may not return when the app quits (GPUI can
end the process inside it there), so put nothing after it that must run.

**Windows.** You need MinGW-w64's gcc and LLVM's linker, lld, on `PATH`: MSYS2's UCRT64
environment with the `mingw-w64-ucrt-x86_64-gcc` and `mingw-w64-ucrt-x86_64-lld` packages,
llvm-mingw, or WinLibs' build "with LLVM". In PowerShell:

```powershell
$G = "$PWD/../gpuikit-0.0.2-windows-x86_64"
$env:CGO_ENABLED = "1"; $env:CGO_LDFLAGS = "-L$G/lib"
go build -ldflags="-s -w '-extldflags=-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres'" -o groceries.exe .
.\groceries.exe
```

The `.exe` has the library and the Windows manifest inside it; there is nothing to ship next to it.
`-fuse-ld=lld` is what lets the linker drop the components you don't use: MinGW's own linker (GNU
ld) can't on Windows, so without lld the program still works but carries all of them (about 72 MB
instead of about 51 for a small app). `-Xlink=-force:multipleres` lets the gpuikit manifest replace
the default one MinGW's gcc adds to every program; lld prints a warning about it, which is
expected.

- The manifest (in `windows/gpuikit.manifest`, embedded by package `gk`) asks for common controls 6,
  which the library needs: without it Windows refuses to start the program. It also makes the
  program per-monitor DPI aware (without it Windows draws the window at 96 dpi and scales it up,
  blurred). To give the window an icon, compile a `.rc` with only the icon into a `.syso` in your
  main package (`windres -O coff -i app.rc -o rsrc_windows_amd64.syso`); `go build` links every
  `.syso` there by itself. `windows/gpuikit.rc` shows the line.
- `-ldflags "-H windowsgui"` builds a program without a console window.

## Part 2: the tutorial

You will build a shopping list: items in a list with checkboxes and delete buttons, an input to add
items, a dialog to edit one, keyboard shortcuts and a menu, a dark theme, and prices that a
goroutine looks up. Each step below is a complete program in `docs/tutorial-go/` of the download
(one module, `gpuikit.dev/tutorial`, whose `go.mod` has the `replace` above). Build and run one
from the download's folder with
`examples/go/build.sh docs/tutorial-go/03-list && docs/tutorial-go/03-list.bin`.

Five ideas carry the whole binding, and the steps introduce them one at a time:

1. **Your state is an ordinary Go struct** (`App`). The binding never copies or owns it; its
   methods are the view and the handlers.
2. **A view function describes what the window shows** from that state, by calling element
   constructors such as `kit.Button` and `kit.Label` and chaining builders. It runs again whenever
   the view needs redrawing, so it only describes: it never changes anything.
3. **A handler is an ordinary Go function** that changes your state: a method value (`a.addMilk`)
   or a closure, given to an element's builder (`.OnClick`). After it returns, the view it was
   given in is redrawn.
4. **Kit state** (an input's text, a list's selection and scroll position) lives in the library;
   your struct keeps a handle to it (`editor.InputState`, `table.ListState`), made once, in an `init`
   function.
5. **`cx *gk.Cx`** is the context of the call you are in (a view, a handler, `init`). Pass it on,
   never store it.

### Step 1: a window

<!-- from: docs/tutorial-go/01-window/main.go -->
```go
// Step 1 of the Go tutorial: a window with a view.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/kit"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

func view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Label("Nothing to buy yet"))
}

func main() {
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(nil, view))
	})
}
```

`gk.Run` starts the app: it calls the function you give it once, which opens a window titled
"Groceries", 520 by 560 pixels, whose content is a view (`gk.OpenWindowWith` takes the other
options: a minimum size, a position, ...). `gk.NewView(nil, view)` makes that view from an
optional `init` function (none yet) and the view function. Call `gk.Run` from `main`: the main
goroutine becomes the UI thread.

`view` returns one element tree. `gk.VFlex` is a column; the arguments after it are **style
tokens** from package `s`: `s.SizeFull` (fill the window), `s.P4` (padding), `s.Gap3` (space
between children). They are GPUI's style methods without arguments, and any constructor takes
them; so are the kit's own (`s.FontBold`, `s.FontSemibold`, `s.HFlex`), and `.Style(..)` adds more
to an element later. Methods with arguments stay methods: `.Bg(..)`, `.TextColor(..)`,
`.W(gk.Px(240))`. `theme.Background` is a colour of the kit's theme, so the view follows light and
dark themes.

A chain continued on the next line ends its line with `.`, as Go requires.

### Step 2: state and a handler

<!-- from: docs/tutorial-go/02-state/main.go -->
```go
type Item struct {
	Name   string
	Amount int
}

type App struct {
	items []Item
}

func (a *App) view(cx *gk.Cx) gk.Element {
	col := gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Button("add-milk").Primary().Label("Add milk").OnClick(a.addMilk)).
		Child(kit.Label(fmt.Sprintf("%d items", len(a.items))))
	for _, it := range a.items {
		col = col.Child(kit.Label(fmt.Sprintf("%s ×%d", it.Name, it.Amount)))
	}
	return col
}

func (a *App) addMilk(cx *gk.Cx, _ gk.ClickEvent) {
	a.items = append(a.items, Item{Name: "Milk", Amount: 1})
	// nothing else: a handler redraws the view it was given in
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(nil, a.view))
	})
}
```

`kit.Button("add-milk")` takes an id: a name that is unique among its siblings, which GPUI uses to
keep track of the element between redraws (`gk.Key("item", 3)` makes one from a name and a
number). `.OnClick(a.addMilk)` gives it a handler: a method value, whose type is the builder's,
`func(cx *gk.Cx, e gk.ClickEvent)`; a handler that does not fit does not compile. `addMilk`
appends and returns: the view it was given in is then redrawn, so the label shows the new count.
There is no `notify` to call and no state to copy into the view.

The view function builds a new tree on every call, with a plain `for` loop where the data has a
variable length. That is cheap: a view whose description did not change is not redrawn.

### Step 3: a list with your own rows

A `for` loop of labels is fine for a few items. For a list the user scrolls, selects in and that may
hold thousands of items, use the kit's List, which asks you only for the rows it shows:

<!-- from: docs/tutorial-go/03-list/main.go -->
```go
type App struct {
	items []Item
	list  table.ListState // kit state: it lives in the library; the app keeps the handle
}

func (a *App) init(cx *gk.Cx) {
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.items = append(a.items, Item{"Milk", 2}, Item{"Bread", 1}, Item{"Apples", 6})
	gk.Refresh(cx)
}

func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	it := a.items[at.Row]
	return table.ListItem(gk.Key("item", at.Row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(gk.Div(s.Flex1).Child(kit.Label(it.Name))).
		Child(kit.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))))
}
```

and in the view, and the handler and `main`:

<!-- from: docs/tutorial-go/03-list/main.go -->
```go
		Child(gk.Div(s.Flex1, s.MinH0).Child(table.List(a.list))).
// ...
func (a *App) addMilk(cx *gk.Cx, _ gk.ClickEvent) {
	a.items = append(a.items, Item{"Milk", 1})
	gk.Refresh(cx) // the rows show data this handler changed: describe them again
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(a.init, a.view))
	})
}
```

- `init` runs once, when the window first shows the view (the first argument of `gk.NewView`).
  Kit state is made there: `table.NewListState(cx, source).Build()` makes the list's state in the
  library and returns the handle you keep in `App`.
- `table.NewListSource(cx, count, row)` says where rows come from: a function that counts them and
  one that draws row `at.Row`. `itemRow` returns a `table.ListItemEl` with whatever you put in it;
  here a label and a tag.
- `gk.Refresh(cx)`, at the end of a handler (or of `init`) that changed what rows show, has every
  view and every visible row described again before the next frame, and the list recounts its rows
  with the count function. Unchanged rows cost a comparison.
- `s.MinH0` and `s.Flex1` on the list's parent let the list take the remaining height and scroll
  inside it, as in CSS flexbox. A list has no height of its own: in a row (`gk.HFlex`, which
  centres its children vertically) give the list's parent `s.HFull` and the row `s.ItemsStart`, or
  the list is drawn zero pixels high and shows nothing.

### Step 4: an input

<!-- from: docs/tutorial-go/04-input/main.go -->
```go
type App struct {
	items   []Item
	newName editor.InputState
	list    table.ListState
}

func (a *App) init(cx *gk.Cx) {
	a.newName = editor.NewInputState(cx).Placeholder("Add an item and press Enter").Build()
	a.newName.OnInputEvent(cx, a.newNameEvent)
// ...
func (a *App) newNameEvent(cx *gk.Cx, e editor.InputEvent) {
	if _, enter := e.(editor.InputEventPressEnter); !enter {
		return
	}
	name := strings.TrimSpace(a.newName.Value(cx))
	if name == "" {
		return
	}
	a.items = append(a.items, Item{name, 1})
	a.newName.SetValue(cx, "")
	table.SelectRow(cx, a.list, len(a.items)-1, table.ScrollIntoView)
	gk.Refresh(cx)
}
```

The input's state is kit state like the list's: made in `init`, shown with `editor.Input(a.newName)`
in the view. `.OnInputEvent(cx, a.newNameEvent)` subscribes to its events; the handler gets a
`editor.InputEvent`, an interface with one struct per event upstream has (`editor.InputEventChange`,
`editor.InputEventPressEnter`, …), taken apart with a type assertion or a type switch.

`a.newName.Value(cx)` reads the text: an ordinary Go string, yours to keep. Writes such as
`SetValue` are applied after the handler returns, in order.

`table.SelectRow(cx, a.list, row, table.ScrollIntoView)` selects a row of the list and scrolls it into
view.

### Step 5: handlers in a row

<!-- from: docs/tutorial-go/05-rows/main.go -->
```go
func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	row, it := at.Row, a.items[at.Row]
	name := kit.Label(it.Name)
	if it.Bought {
		name = name.Style(s.LineThrough).TextColor(theme.MutedForeground)
	}
	return table.ListItem(gk.Key("item", row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(kit.Checkbox(gk.Key("bought", row)).Checked(it.Bought).OnClick(func(cx *gk.Cx, bought bool) {
			gk.StopPropagation(cx) // the row under the checkbox is not clicked as well
			a.items[row].Bought = bought
			gk.Refresh(cx)
		})).
		Child(gk.Div(s.Flex1).Child(name)).
		Child(kit.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))).
		Child(kit.Button(gk.Key("delete", row)).Ghost().Xsmall().Icon(gk.IconDelete).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
			gk.StopPropagation(cx)
			a.items = slices.Delete(a.items, row, row+1)
			gk.Refresh(cx)
		})))
}
```

A handler in a row is a closure that captures what it needs, here the row's number. A checkbox's
`OnClick` hands the new state (`bought bool`) where a button's hands a `gk.ClickEvent`; the types
say which, and the compiler checks them.

A click on the checkbox would also be a click on the row under it; `gk.StopPropagation(cx)` keeps
it for the checkbox. `.Icon(gk.IconDelete)` is one of the kit's icons (`gk.IconName`).

### Step 6: a dialog

A click on a row (or Enter on the selected row) is the list's `Confirm` event; it opens a dialog
that edits the item:

<!-- from: docs/tutorial-go/06-dialog/main.go -->
```go
	a.list.OnListEvent(cx, a.listEvent)
	a.editDialog = gk.NewBuilt(cx, a.editView)
// ...
func (a *App) listEvent(cx *gk.Cx, e table.ListEvent) {
	if c, ok := e.(table.ListEventConfirm); ok { // a click on a row, or Enter
		a.openEditor(cx, c.Row)
	}
}

func (a *App) editView(cx *gk.Cx) overlay.DialogEl {
	amountBy := func(by int) func(*gk.Cx, gk.ClickEvent) {
		return func(*gk.Cx, gk.ClickEvent) { a.editAmount = max(1, a.editAmount+by) }
	}
	return overlay.Dialog().Title(kit.Label("Edit item")).
		Child(gk.VFlex(s.Gap3).
			Child(editor.Input(a.editName)).
			Child(gk.HFlex(s.Gap2, s.ItemsCenter).
				Child(kit.Label("Amount")).
				Child(kit.Button("less").Small().Icon(gk.IconMinus).OnClick(amountBy(-1))).
				Child(kit.Label(fmt.Sprint(a.editAmount))).
				Child(kit.Button("more").Small().Icon(gk.IconPlus).OnClick(amountBy(1))))).
		Footer(overlay.DialogFooter().
			Child(overlay.DialogClose().Trigger(kit.Button("cancel").Label("Cancel"))).
			Child(overlay.DialogAction().Child(kit.Button("save").Primary().Label("Save")))).
		OnOk(a.saveEdit)
}

func (a *App) openEditor(cx *gk.Cx, row int) {
	a.editing = row
	a.editAmount = a.items[row].Amount
	a.editName.SetValue(cx, a.items[row].Name)
	overlay.OpenDialog(cx, a.editDialog)
	a.editName.Focus(cx)
}

func (a *App) saveEdit(cx *gk.Cx, _ gk.ClickEvent) bool {
	name := strings.TrimSpace(a.editName.Value(cx))
	if name == "" {
		return false // keep the dialog open
	}
	it := &a.items[a.editing]
	it.Name = name
	it.Amount = a.editAmount
	gk.Refresh(cx)
	return true // close the dialog
}
```

- A dialog is described by a function like a view's, which returns a `overlay.DialogEl`.
  `gk.NewBuilt(cx, a.editView)`, made once in `init` and kept as a `gk.Built[overlay.DialogEl]`, makes
  it a small view of its own, and `overlay.OpenDialog` shows it. It is redrawn like any view: the `+`
  and `-` buttons change `a.editAmount` and the dialog shows the new number.
- `.Footer(..)` holds the buttons: `overlay.DialogClose().Trigger(button)` closes the dialog, and the
  button inside `overlay.DialogAction()` runs `OnOk`. `OnOk`'s handler returns a `bool`: `true` closes
  the dialog, `false` keeps it open (an empty name here). Escape closes it too.
- `amountBy` is a function that makes handlers: a closure per step size. The dialog's input is
  kit state made in `init` like the others (`a.editName`).

### Step 7: shortcuts and a menu

<!-- from: docs/tutorial-go/07-shortcuts/main.go -->
```go
	newItem, clearBought gk.Action // shortcuts, also shown in the menu
}
// ...
	a.newItem = gk.Shortcut(cx, "ctrl-n", a.focusNewName)
	a.clearBought = gk.Shortcut(cx, "ctrl-shift-d", a.removeBought)
// ...
func (a *App) toolbar() gk.Element {
	menu := kit.NewPopupMenu().
		Menu("New item", a.newItem).
		Menu("Remove bought items", a.clearBought)
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(gk.Div(s.Flex1)).
		Child(kit.DropdownMenu(kit.Button("menu").Ghost().Icon(gk.IconMenu), menu))
}
// ...
func (a *App) focusNewName(cx *gk.Cx) { a.newName.Focus(cx) }

func (a *App) removeBought(cx *gk.Cx) {
	a.items = slices.DeleteFunc(a.items, func(it Item) bool { return it.Bought })
	gk.Refresh(cx)
}
```

`gk.Shortcut(cx, "ctrl-n", a.focusNewName)` binds keys to a handler, app-wide: it works wherever
the focus is. The handler is a `func(cx *gk.Cx)` or a click handler, so one method can serve a
button and its keys (step 8 does that). It returns a `gk.Action`, which a menu item takes, and the
menu then shows the keys next to the item. `kit.DropdownMenu(button, menu)` opens the menu under
the button. To limit a shortcut to a part of the window, add `gk.WhenIn("Items")` and mark that
part with `.KeyContext("Items")`.

`toolbar` is an ordinary method that returns an element. That is how a view is split into parts:
call it from the view function; it takes whatever it needs.

### Step 8: a dark theme

<!-- from: docs/tutorial-go/08-theme/main.go -->
```go
func (a *App) toolbar() gk.Element {
	themeLabel, icon := "Dark theme", gk.IconMoon
	if a.dark {
		themeLabel, icon = "Light theme", gk.IconSun
	}
	menu := kit.NewPopupMenu().
		Menu("New item", a.newItem).
		Menu("Remove bought items", a.clearBought).
		Separator().
		Menu(themeLabel, a.toggleTheme)
	return gk.HFlex(s.Gap2, s.ItemsCenter).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(gk.Div(s.Flex1)).
		Child(kit.DropdownMenu(kit.Button("menu").Ghost().Icon(gk.IconMenu), menu)).
		Child(kit.Button("theme").Ghost().Icon(icon).Tooltip("Toggle the theme").OnClick(a.switchTheme))
}
// ...
func (a *App) switchTheme(cx *gk.Cx, _ gk.ClickEvent) {
	a.dark = !a.dark
	mode := gk.ThemeModeLight
	if a.dark {
		mode = gk.ThemeModeDark
	}
	gk.ThemeChange(cx, mode)
	gk.Refresh(cx)
}
```

`gk.ThemeChange(cx, ..)` switches every colour of the kit's theme at once; elements that use
`theme.` colours (as the view has since step 1) follow. `.Tooltip(..)` shows a tip when the
pointer rests on the button. `switchTheme` is a click handler, given both to the button and, in
`init`, to `gk.Shortcut(cx, "ctrl-t", a.switchTheme)`.

`gk.Refresh(cx)` matters here because the theme is shown in two places: the button belongs to the
view, but `switchTheme` also runs from the shortcut, which is not given in a view.

### Step 9: work on a goroutine

Looking up prices is slow, so a goroutine does it, and the result comes back on the UI thread:

<!-- from: docs/tutorial-go/09-background/main.go -->
```go
// lookUpPrice asks a slow price service on a goroutine; the answer comes back on the UI thread.
func (a *App) lookUpPrice(cx *gk.Cx, name string, announce bool) {
	gk.Go(cx, func() float64 {
		time.Sleep(300 * time.Millisecond)
		return 0.49 + float64(len(name))*0.35
	}, func(cx *gk.Cx, price float64) {
		for i := range a.items {
			if it := &a.items[i]; it.Name == name && it.Price == 0 {
				it.Price = price
			}
		}
		if announce {
			overlay.PushNotification(cx, fmt.Sprintf("%s costs %.2f", name, price))
		}
		gk.Refresh(cx)
	})
}
```

- `gk.Go(cx, work, done)` runs `work` on a new goroutine and then `done` with its result on the UI
  thread, with a `cx`, before the next frame. The type of the result (`float64` here) is checked
  by the compiler: `gk.Go` is generic.
- For a goroutine of your own (a stream, a ticker, a watcher), call `gk.Post(func(cx *gk.Cx) { .. })`
  from it: the function runs on the UI thread. `gk.Post` is the only call of the package any
  goroutine may make; anything else from another goroutine stops the app, naming the line.
- A posted function, or `done`, redraws nothing by itself: `gk.Refresh(cx)` shows the price.
- `overlay.PushNotification(cx, text)` shows a toast in the corner of the window.

The whole program, `docs/tutorial-go/09-background/main.go`, is 232 lines. That is the tutorial.

## Part 3: what to know next

### Who redraws what

| after | redrawn |
|---|---|
| a handler given to an element in a view | that view |
| a handler given in a row | that row |
| a handler given in a view's `init` (an input's events, a shortcut, a timer) | that view |
| a posted function, `gk.Go`'s `done`; a timer or shortcut made outside a view's `init` | nothing |
| `gk.Refresh(cx)` anywhere | every view and every visible row (unchanged ones cost a comparison) |
| `m.Set(cx, v)` of a `gk.Model[T]` | the views that read `m.Get()` |

When in doubt, end the handler with `gk.Refresh(cx)`. `gk.Quiet(cx)` in a handler keeps its view
from being redrawn.

### What you may keep

| thing | lives | keep it? |
|---|---|---|
| your `App` and everything in it | as long as you keep it | yes |
| `editor.InputState`, `table.ListState`, `gk.View`, `gk.Action`, `gk.Built[T]` and other handles | until `gk.Release` | yes, in your struct |
| elements (`kit.Label(..)`, `kit.Button(..)`) | one call of the view function | no: make them on each call |
| strings from reads and events (`a.newName.Value(cx)`) | always (Go strings are copies) | yes |
| `*gk.Cx` | the call it was given to | never |
| what a handler closure captures | as long as the closure | yes: the collector keeps it alive |

### When the app stops with a `gk:` message

A mistake in using the binding stops the app with exit code 2 and one message that says what was
wrong, what to do, and the line of your code. For example, making kit state in a view function:

```
gk: a view function may not create kit state (it runs on every redraw): create it once in init or in a handler and keep the handle in your state struct, as in a.search = editor.NewInputState(cx).Build()
    at /home/me/groceries/main.go:61 (main.(*App).view)
```

The binding panics with a `*gk.Error`; the boundary between the library and your code recovers it,
prints it and exits, so nothing unwinds into the library. A panic of your own code (an index out of
range, a nil map) in a handler, a view, `init` or a posted function is reported the same way, with
`gk: panic in app code:`, the line it came from and Go's stack. A mistake the compiler can catch
(a handler of the wrong type, a style token where a colour goes, an element where none fits) does
not compile at all.

Two kinds of mistake are found later than the line that made them, and the message says so:

- An element used twice, or after its view function returned, is found inside a builder or when
  the view returns. The message names the view (`while the view function of the gk.NewView made at
  main.go:158 ran`); run the app with `GK_DEBUG=1` and it names the line that made the element.
- A change to kit state (`SetValue`, `Focus`) is applied after the handler returns. With
  `GK_DEBUG=1` every change is applied at once, and the message points at its line.

### Forms, selects and charts

`examples/go/expenses` (200 lines) adds expenses through a form with checked fields, and shows a
bar chart of the totals per category:

<!-- from: examples/go/expenses/main.go -->
```go
	items := make([]gk.SearchItem, len(categories))
	for i, c := range categories {
		items[i] = gk.SearchItem{Value: c.String()}
	}
	a.category = table.NewSelectState(cx, gk.NewSearchSource(cx, items), table.NoRow).Build()
	a.category.OnSelectEvent(cx, func(cx *gk.Cx, _ table.SelectEvent) { a.revalidate(cx) })
// ...
// field is a form field with its message under it while it has one.
func field(label string, input gk.Element, message string) kit.FieldEl {
	f := kit.Field().Label(label).Child(input)
	if message != "" {
		f = f.DescriptionFn(kit.Label(message, s.TextXs).TextColor(theme.Danger))
	}
	return f
}

// total is one bar of the chart.
type total struct {
	Category string
	Total    float64
}

func (a *App) chart() gk.Element {
	totals := make([]total, len(categories))
	for i, c := range categories {
		totals[i].Category = c.String()
	}
	for _, e := range a.expenses {
		totals[e.Category].Total += e.Amount
	}
	return gk.VFlex(s.Gap2).W(gk.Px(280)).
		Child(kit.Label("By category", s.FontSemibold)).
		Child(gk.Div().H(gk.Px(220)).Child(chart.BarChart(chart.NewData(totals)).Band("Category").Value("Total")))
}
// ...
	a.categoryErr = "Choose a category"
	if at, chosen := a.category.SelectedIndex(cx).Get(); chosen {
		e.Category, a.categoryErr = Category(at.Row), ""
	}
```

- A select's items are data: `gk.NewSearchSource(cx, items)` over `gk.SearchItem`s (a value and
  the title shown; an empty title shows the value), and `table.NewSelectState` is made over that
  source. `.SelectedIndex(cx)` is the chosen row (a `gk.Option`, empty until something is chosen;
  `.Get()` takes it apart); `.SelectedValue(cx)` is its value as text. The last argument of
  `table.NewSelectState` preselects a row (`gk.Some(table.Row(0))` for the first), or is `table.NoRow`.
  Wherever a call takes a row of a list (an "index path"), `table.Row(n)` gives one.
- A form is fields: `kit.Field().Label(..).Child(any element)`, and `.Description(..)` (text) or
  `.DescriptionFn(..)` (an element: here a red message) under it. Checking is ordinary code: the
  handler reads the values, sets a message in your state, and the view shows it.
- A chart takes your rows as they are: `chart.NewData(rows)` over a slice of any struct, whose
  exported fields are the columns the builders name (`.Band("Category").Value("Total")`, checked
  when the view runs; a type with a `String` method, like `Category`, is text). The rows are read
  when the chart is made. A chart has no size of its own: put it in a sized `gk.Div`.

### Checking that it starts, in CI

`GK_FRAMES=5,30 ./app.bin` draws 30 frames and exits with status 0, so a CI job can check that the
program starts and draws. On Linux without a display, run it under a virtual one:
`xvfb-run -a env GK_FRAMES=5,30 ./app.bin` (packages `xvfb`, and Mesa's Vulkan driver
`mesa-vulkan-drivers` when the machine has no GPU). macOS and Windows CI machines have a desktop,
so the program runs there as it is.

### Where next

- [API-GO.md](API-GO.md): how the Go names map from GPUI-kit's Rust documentation, and an index of
  every component with its constructor and builders.
- [GO.md](GO.md): how the Go front end works (handlers, `gk.Model[T]`, goroutines, cgo rules).
- The examples in `examples/go/`: `expenses` (a form with checks, a select, a bar chart), `todo`
  (projects, a task list, filters, search, a JSON file, a goroutine that syncs) and `reader` (3,000
  articles in a virtual list, tabs of Markdown views, a settings dialog with a slider and selects)
  are whole apps written this way; `notes` adds the kit's dock with panels.
- `go doc gpuikit.dev/gk/kit ButtonEl` (from a module that requires the package), or the generated
  files themselves, `go/gk/<package>/gen_component_*.go`: one per upstream module, each type with its
  constructor and its methods, whose parameter types say what to pass.
