# How the Go package works

The Go package (`gpuikit.dev/gk`) is the front end on the library. It is generated from a
description of GPUI-kit's API, and talks to the library through a small C interface (seven C
functions and one callback). Nothing in that interface is specific to Go.

This page is for when you want to know how the Go side fits together: where things live, how the
names are made, and what rules the package follows. To write your first app, start with
[GETTING-STARTED-GO.md](GETTING-STARTED-GO.md) instead.

## 1. Layout

```
go/go.mod                     module gpuikit.dev (Go 1.27, cgo)
go/gk/                        package gk: everything an app imports
go/gk/rt_*.go                 the runtime, written by hand;
                              rt_thread_{linux,darwin,windows}.go hold what differs per system
go/gk/capi.c                  the C side of the one exported callback (cgo)
go/gk/cgo_required.go         built only without cgo: the first build error says to install gcc
go/gk/gen_*.go                generated
go/gk/s/gen_s.go              package s: the style tokens (s.Gap3, s.SizeFull)
go/gk/theme/gen_theme.go      package theme: the theme's colours (theme.Background)
go/gk/palette/gen_palette.go  package palette: the kit's fixed colours (palette.Blue500)
examples/go/                  module gpuikit.dev/examples: hello, counter, notes, todo, reader,
                              expenses
docs/tutorial-go/             the tutorial's nine steps (module gpuikit.dev/tutorial)
```

To build: `CGO_LDFLAGS="-L<lib dir>" go build -ldflags="-s -w -extldflags=-Wl,--gc-sections" ./hello`
on Linux and Windows, and the same with `-Wl,-dead_strip` in place of `-Wl,--gc-sections` on macOS
(section 8 explains the flag). The package names the library itself (libgpuikit_static.a, and the
system libraries it needs), so the only other thing you need is a C compiler for cgo: gcc or clang,
the Xcode command-line tools on macOS, or MinGW-w64 gcc on Windows.

## 2. Names

Every Go name is made from the upstream (Rust) name by fixed rules, so GPUI-kit's documentation
translates mechanically.

| upstream | Go | rule |
|---|---|---|
| type `InputState`, `base::IndexPath` | `InputState`, `BaseIndexPath` | CamelCase; a digit group after a digit group keeps one `_` (`Neg1_2`, but `Neg12`) |
| method `on_click`, `text_color`, `scroll_to_item` | method `OnClick`, `TextColor`, `ScrollToItem` | CamelCase of snake_case |
| initialisms `id`, `url`, `uri`, `json`, `html`, `http`, `svg`, `css`, `api`, `ascii`, `utf8` | `ID`, `URL`, ... | Go's initialisms (`element_id` -> `ElementID`) |
| element constructor `Button::new("x")`, `v_flex(..)` | `gk.Button("x")`, `gk.VFlex(..)` | the type of the element gets `El`: `gk.ButtonEl` (Go has one namespace for types and functions) |
| value or state constructor `Column::new(..)`, `InputState::new(cx)` | `gk.NewColumn(..)`, `gk.NewInputState(cx)` | `New` + type name; the type keeps its name |
| enum value `ButtonVariant::Primary` | `gk.ButtonVariantPrimary` | type name + variant; `[go] enum_prefix` in `api/overrides.toml` shortens a few (`IconName` -> `Icon`: `gk.IconStarFill`) |
| enum with data (`ButtonVariant`, `Size`, `InputEvent`) | an interface: a struct type per variant with data (`gk.ButtonVariantCustom{Value: ..}`); the variants without data of a union the app gives to the kit are constants of one `Kind` type (`gk.ButtonVariantDanger`, `gk.SizeLarge` of `gk.SizeKind`) | an event's variants stay types, for a type switch (`case gk.InputEventChange:`) |
| style token `gap_3()` | `s.Gap3` | package `s` |
| theme color `background`, palette color `blue_500` | `theme.Background`, `palette.Blue500` | packages `theme` and `palette`, values of type `gk.Color` |
| record field `row`, `is_today` | `Row`, `IsToday` | exported CamelCase |
| parameter that is a Go keyword (`type`, `range`, `func`) | `type_`, `range_` | trailing underscore |

If two upstream items still map to the same name after these rules, they get a crate prefix
(`Base`, `Gpui`, `Kit`) and then a number. The generator reports every
such case, so none of them is silent.

## 3. What app code looks like

<!-- from: docs/snippets-go/gomd/main.go -->
```go
type App struct {
	count int
	name  gk.InputState // a handle: the kit's InputState lives in the library
	root  gk.View
}

func (a *App) init(cx *gk.Cx) { a.name = gk.NewInputState(cx).Placeholder("Your name").Build() }

func (a *App) add(cx *gk.Cx, _ gk.ClickEvent) { a.count++ } // marks its view for redraw on return

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(gk.Button("add").Primary().Label("Add").OnClick(a.add)).
			Child(gk.Label(fmt.Sprintf("clicked %d times", a.count)))).
		Child(gk.Div().W(gk.Px(240)).Child(gk.Input(a.name))).
		Child(gk.Label("hello " + a.name.Value(cx))) // a read in a view: redrawn when it changes
}

func main() {
	a := &App{}
	gk.Run(func(cx *gk.Cx) {
		a.root = gk.NewView(a.init, a.view)
		gk.OpenWindow(cx, "Hello", 480, 320, a.root)
	})
}
```

- **Elements** are values with methods; a builder returns the element so calls chain. An
  element is one pointer into a node block of the view build that made it (so giving it to
  `Child` as a `gk.Element` interface does not allocate); a kept one is caught.
- **Style tokens** are typed constants in package `s` given to a constructor or to
  `.Style(...)`. Styled methods with arguments stay methods (`.W(gk.Px(240))`).
- **Options and defaults**: upstream's `self -> Self` chain is the Go chain; plain records are
  structs whose zero value is upstream's `Default` where the generator knows it. Functional
  options are used where a runtime call has optional parts (`gk.WhenIn("Tasks")`).
- **Enums** are typed constants. **Enums with data** are a Go interface with one struct per
  variant (`gk.InputEventPressEnter{Secondary: true}`); events arrive as one and are taken
  apart with a type switch. **Into sets** (`impl Into<X>`) are a named interface type
  documented with its members; a value of another type stops with a message.
- **Optional values** are `gk.Option[T]` (`gk.Some(v)`; the zero value is none); reads that
  may have nothing return `(T, bool)`.
- **Handles** (kit state, focus handles, row sources, timers, subscriptions, windows) are small
  structs holding a generation-checked 64-bit handle; a released one stops with a message.

## 4. Handlers, state and background work

- A **handler** is any Go function of the builder's shape: `OnClick(func(cx *gk.Cx, e gk.ClickEvent))`,
  a method value (`a.add`) or a closure. Closures capture what they need; the collector keeps it
  alive, so Go needs no handler-data rule. The view that registered a handler is marked dirty when it returns;
  `gk.Quiet(cx)` inside the handler opts out.
- **Typed state**: `gk.Model[T]` holds a value and the views that read it: `m.Get()` in a view
  records the view, `m.Set(cx, v)` / `m.Update(cx, func(*T))` mark exactly those views,
  `m.Observe(cx, func(*gk.Cx, T))` runs on every change.
- **Views**: `gk.NewView(init, render)` with `init func(*gk.Cx)` (nil for none) and
  `render func(*gk.Cx) gk.Element`; `.ChildView(v)` embeds one; `gk.Notify(cx, v)` marks one;
  `gk.Refresh(cx)` marks everything.
- **Background work**: `gk.Post(func(cx *gk.Cx) {...})` from any goroutine; the closure runs on
  the UI thread before the next frame. `gk.Go(cx, work, done)` runs `work` on a goroutine and `done`
  with its result on the UI thread. The queue is the binding's; one `gk_post` wakes the library
  when the queue goes from empty to non-empty.
- **Timers**: `gk.After(cx, d, f)`, `gk.Every(cx, d, f)` with `time.Duration`; `gk.Release(cx, t)`.
- **Actions and keys**: `gk.NewAction(cx, "app::Save")`, `gk.BindKeys`, `.OnAction(gk.Act(save, f))`,
  `gk.Shortcut(cx, "ctrl-n", f, gk.WhenIn("Tasks"))`, where f is a `func(cx)` or a click handler
  (one method serves a button and its keys: `gk.Shortcut(cx, "ctrl-t", a.toggleTheme)` and
  `.OnClick(a.toggleTheme)`).
- **Lists**: `gk.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)` and
  `gk.NewListState(cx, src).Build()`; the count is asked again at every `gk.Refresh`, so a
  handler that changed the slice calls `gk.Refresh(cx)` and nothing else.
  `gk.SelectRow(cx, list, row)` (with `gk.ScrollIntoView`) selects without the list's event.
  `gk.NewRowSource(cx, n, row)` is the low-level source the kit's tables and trees take. Row views
  are described between frames, never in the middle of one.
- **Selects and comboboxes**: `gk.NewSearchSource(cx, []gk.SearchItem{{Value: "a", Title: "A"}})`,
  then `gk.NewSelectState(cx, src, gk.NoRow)` (or `gk.Some(gk.Row(0))`); an empty Title is the Value.
- **Menus**: `gk.DropdownMenu(gk.Button("file").Label("File"), gk.NewPopupMenu().Menu("New", a.newAction))`.
- **Docks**: `gk.NewDockAreaSkinned(cx, id)`, `gk.NewPanel(cx, view, gk.PanelData{Name: .., Title: ..})`,
  `dock.SetCenter(cx, gk.NewDockLayoutTabs().PanelView(p))`, `gk.DockView(dock)` in a view.
- **Saved layouts and the rest of the dock** (go/gk/rt_dock.go): `dock.Dump(cx)`
  gives a `gk.DockAreaState`, `dock.Load(cx, saved)` rebuilds panels through `gk.RegisterPanel(cx,
  name, f)` (f gets a `gk.PanelBuild` and answers with `gk.PanelFill`); `gk.PanelGroup`, `gk.PanelID`,
  `gk.PaneNodeKind`, `gk.PaneNodeWalk`, `gk.PaneTreeToState`, `gk.PaneTreeFromState`, `gk.BuildPanel`;
  the skin's settings `gk.DockSkinPanelStyle`, `gk.SetDockSkinToggleButtonVisible`,
  `gk.SetDockSkinCollapsedTabBarVisible`, `gk.SetDockSkinSplitDividerLineVisible`, ...; renderers
  `gk.NewDockRenderer(cx, gk.DockHooks(gk.HookDock), f)`, `gk.DockRenderAnswer`, `gk.DockSlot()`;
  `gk.OnDockToggle`.
- **Tables and trees** (go/gk/rt_table.go): `gk.NewTableSource(cx, rows,
  []gk.TableColumn{gk.NewTableColumn("name", "Name")}, cell, gk.TableEvents(f))`, `gk.SetTableRows`,
  `gk.InvalidateTable`, `gk.SetTableColumns`, `gk.SetTableTexts`, `gk.SetTableRowLabels`,
  `gk.SetTableLook`, `gk.RowMenu(el, menu)`; `gk.SetTreeItems`, `gk.TreeEntryAt`, `gk.NewTreeSource`;
  `gk.SearchWillChange(cx, src, gk.SearchRules{..})` for selects.
- **Motion** (go/gk/rt_motion.go): `gk.WithAnimation(el, id, gk.Animation{..}, gk.Track{..})`,
  `gk.Motion(el, gk.MotionValue{Prop, Src: gk.ToTransition{..}})`, `gk.FocusTrap`,
  `gk.TextSelectionScopeOf(el, gk.NewTextSelectionScopeID(cx))`.
- **Editors** (go/gk/rt_editor.go): `gk.CodeEditor(state, gk.CodeEditorOptions{..})`;
  `gk.EditorProviders(cx, editor, f, gk.ProvidersOptions{..})` asks f a `gk.ProviderEvent`, answered
  with `gk.AnswerCompletions`, `gk.AnswerHover`, `gk.AnswerDefinitions`, ... Also `gk.KbdForAction`,
  `gk.NewAppMenuBar`/`gk.AppMenuBarView`, `gk.ClockNow`, `gk.VVirtualList`/`gk.HVirtualList`.
- **Charts over the app's rows**: `gk.BarChart(gk.NewData(rows)).Band("Month").Value("Total")`:
  `gk.NewData` takes a slice of structs; each exported field is a column named as the field is.
- **Effects**: `gk.Effect(el, gk.NewEffectTransition(d).Fade(0, 1), key)`; el is a
  `gk.AnimatableElement` (checked by the compiler).
- **Items** (a calendar's days, a tree's entries): a typed Go function given with the builder
  that takes it, run between frames.
- **Requests that answer later** (file dialogs, prompts): a call takes a `func(cx, answer)` that
  runs on the UI thread once the answer is there; releasing the returned handle cancels.
- **Windows**: `gk.OpenWindow(cx, title, w, h, root)`; `gk.OpenWindowWith(cx, gk.WindowConfig{..}, root)`
  (with `AppID` and a PNG `Icon`); `gk.OnWindowShouldClose(cx, w, func(cx) bool)` asks before the
  user closes a window, `gk.OnAppQuit(cx, f)` runs as the app quits, `gk.ReduceMotion(cx)` (section 7).
- **Drawing**: `gk.Plot(parts...)` over the kit's grid, axes and shapes and the binding's own
  `gk.NewPlotLine`, `gk.NewPlotArea`, `gk.NewPlotQuad`; `.Bg` takes a `gk.Fill`, a colour or a
  `gk.NewLinearGradient(..)` (section 7).
- **Drag and drop, tooltips, menus, images**: `.OnDrag(gk.DragValue{..}, preview)`, `.OnDrop`,
  `.DragOver`, `.CanDrop`; `.Tooltip(view or gk.NewKitTooltip..)`; `gk.SetMenus`; `gk.Img` of
  `gk.ImageSourceImage`, `gk.ImageSourceRender` of a `gk.RenderImage` made once (section 7).

## 5. Errors and safety

- **Misuse stops**: an element used after its build, given twice, a `Cx` kept past the call it was given to, a
  released handle, a write in a view, a UI call from another goroutine: `gk` panics with a
  `*gk.Error` naming the app's line; the callback boundary recovers it, prints
  `gk: <message>\n    at file.go:line (func)` and exits 2. Nothing unwinds across C.
- **A panic in app code** (in a handler, a view, `init` or a posted function) is recovered at the callback and printed with the app's
  line first (`gk: panic in app code: ...` / `at main.go:86 (main.(*App).store)`), then its stack;
  exit 2. A panic in `gk.Go`'s work function names the `gk.Go` call and the line. A UI call from
  another goroutine, where no boundary is below to recover, prints its stop the same way and exits
  2. A panic inside the library exits 1 with a message saying what to do.
- **No display** (Linux): `gk.Run` with neither `DISPLAY` nor `WAYLAND_DISPLAY` set (and no
  `GK_SCRIPT`) stops with a message instead of letting GPUI wait. macOS and Windows always have
  their window server, so the check is skipped there. Messages the library returns name the Go
  calls (`gk.NewBuilt`, not `gk.built`), and every stop says what to do.
- **Fallible upstream calls** return `error` (Go's way); misuse is not an error value.
- **cgo pointer rules**: the library never keeps Go memory. Requests are Go byte slices passed
  for the duration of one C call (`gk_call`); a turn's reply is written into C memory the package
  keeps, and the C half of the callback (`go/gk/capi.c`) hands it to `gk_reply`, which copies it,
  after the Go callback has returned, so a turn costs one crossing into Go and none back out; the
  library's buffers are copied out at once (`gk_call`'s result is valid until the next call). The `gk_app` struct and the script are C memory. Handlers, views and states are
  numbers on the C side (handler ids, handles); the Go side keeps the functions in its own
  tables. `gk.Post` passes no payload through C.
- **Threads**: package `gk` locks the main goroutine to the main OS thread in `init`; `gk.Run`
  must be called from `main`. Every call taking a `Cx` checks the OS thread (Linux: `gettid`;
  macOS: `pthread_threadid_np`; Windows: `GetCurrentThreadId`, both through cgo) and that the `Cx` belongs to the call in progress; only
  `gk.Post` may be called from elsewhere.
- **GK_DEBUG=1**: every buffered write is sent at once (an error points at its line), each
  node records where it was made, and the dropped-event count is printed at exit.
- **GK_FRAMES=5,30**: the app draws 30 frames (after 5 to warm up) and exits, for a CI check that
  it starts. **GK_SCRIPT=file** runs it offscreen under a script of clicks and keys; that needs
  the diagnostic library, which only this repository's tests build (CONTRIBUTING.md).
- **Version check**: the package carries `ABI_MAJOR`, `ABI_MINOR` and `API_HASH` from the
  generated tables and refuses a library with another hash before `gk_run`.

## 6. What the Go package covers

The generator (`gen/gk_go.py`) writes the Go files. [STATUS.md](STATUS.md) lists every upstream
item that is not bound yet, with the reason, and counts coverage per family.

## 7. Drawing, drag and drop, and windows

These features came later than the rest and are worth a table of their own:

| what | Go |
|---|---|
| a described plot | `gk.Plot(parts ...gk.PlotPart)`, `gk.PlotWith(gk.PlotOptions{ID, NotInteractive, Appear}, parts...)` |
| a stroked polyline, a filled polygon, a quad in a plot | `gk.NewPlotLine(points [][2]float32, width float32, fill gk.Fill)`, `gk.NewPlotArea(points, fill)`, `gk.NewPlotQuad(x, y, w, h float32, fill gk.Fill, gk.QuadStyle{CornerRadius, BorderWidth, BorderColor})` |
| a gradient where the kit takes a Fill (`.Bg`, plot paths and quads) | `gk.NewLinearGradient(angle float32, from, to gk.LinearColorStop) gk.LinearGradient`, `.InOklab()`, `gk.NewLinearColorStop(c gk.Color, at float32)`; `gk.Fill` is a `gk.Color` or a `gk.LinearGradient` |
| drag and drop | `.OnDrag(gk.DragValue{ID uint64; Text string}, preview gk.View)`, `.OnDrop(func(cx *gk.Cx, d gk.DragValue))`, `.DragOver(gk.StyleRefinement)`, `.CanDrop(gk.DropAccept{Values bool; IDs []uint64; Paths bool})` |
| an element's tooltip | `.Tooltip(gk.ViewOrTooltip)`: a `gk.View`, `gk.NewKitTooltip(text)` or `gk.NewKitTooltipElement(el)`; `.HoverableTooltip` the same |
| images from bytes and pixels | `gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format, Bytes}})`; `img := gk.NewRenderImage(cx, gk.PixelImage{Width, Height, RGBA})` once (in init or a handler), then `gk.ImageSourceRender{Value: img}` or `gk.ImageSourceCustom{Value: img}` on every frame, `gk.Release(cx, img)` when no view shows it (gpui keeps one texture per image until then) |
| the menu bar | `gk.SetMenus(cx, menus ...gk.Menu)`, `gk.Menus(cx) []gk.Menu`; items `gk.MenuAction{Name, Action, OsAction, Checked, Disabled}`, `gk.MenuSeparator{}`, `gk.MenuSubmenu{Menu}`, `gk.MenuServices{Name}` |
| ask before a window closes | `gk.OnWindowShouldClose(cx, w gk.Window, f func(cx *gk.Cx) bool) gk.ActionListener` |
| run code as the app quits | `gk.OnAppQuit(cx, f func(cx *gk.Cx)) gk.ActionListener` |
| reduced motion | `gk.ReduceMotion(cx) bool`, `gk.SetReduceMotion(cx, on bool)` |
| the window's icon and app id | `gk.WindowConfig{.., AppID: "notes", Icon: pngBytes}` |
| tests' hooks | `gk.TestShouldClose(cx, w)`, `gk.TestRenderImage`, `gk.TestImagesShown`, `gk.TestDropAccept` |

For example, a draggable band, a plot with a gradient line, and asking before a window closes:

<!-- from: docs/snippets-go/gomd/main.go -->
```go
gk.Div().ID(gk.Key("band", i)).
	OnDrag(gk.DragValue{ID: uint64(i), Text: name}, a.bandPreview).   // a.bandPreview: a gk.View made in init
	CanDrop(gk.DropAccept{IDs: a.bandIDs()}).
	DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent)).
	OnDrop(func(cx *gk.Cx, d gk.DragValue) { a.moveBand(int(d.ID), i) })

gk.Plot(gk.NewGrid().X(xs).Y(ys).Stroke(theme.Border),
	gk.NewPlotArea(area, gk.HSLA(0.6, 0.8, 0.7, 0.12)),
	gk.NewPlotLine(curve, 2.5, gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1))))

gk.OnWindowShouldClose(cx, a.win, func(cx *gk.Cx) bool {
	if !a.dirty { return true }
	gk.OpenAlertDialog(cx, a.askClose) // its OK calls gk.CloseWindow(cx, a.win), which does not ask again
	return false
})
```

## 8. Linking only the components you use

GPUI-kit has about 130 component families: buttons, inputs, tables, charts, docks, dialogs, the
code editor and so on. The shared library has to contain all of them, because it can't know which
ones your program will use. A Go program links the static library instead
(`libgpuikit_static.a`), and only the families the program calls into end up in it.

There is nothing to list. Every function of package `gk` that makes an element or calls into the
library also links its family: `gk.Button` links the button family, `gk.NewInputState` the input
family, `gk.LineChart` the chart family. Go's linker already leaves out the functions a program
never calls, so a program that never calls `gk.LineChart` never links the chart family either.

What it needs from you is one flag on `go build`:

```sh
CGO_LDFLAGS="-L<lib dir>" go build -ldflags="-s -w -extldflags=-Wl,--gc-sections" .   # Linux
CGO_LDFLAGS="-L<lib dir>" go build -ldflags="-s -w -extldflags=-Wl,-dead_strip" .     # macOS
CGO_LDFLAGS="-L<lib dir>" go build -ldflags="-s -w '-extldflags=-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres'" .   # Windows
```

`-Wl,--gc-sections` (Apple's linker calls it `-dead_strip`) tells the C linker to drop the code
that nothing uses. On Windows it needs LLVM's linker, lld: MinGW's GNU ld keeps every function
that has unwind data there, which is all of them. `-Xlink=-force:multipleres` lets the manifest
package `gk` embeds (`gpuikit_windows_amd64.syso`) replace the default one MinGW's gcc adds. Go doesn't let a package
ask for that flag itself, which is why it goes on the command line (or in a script or Makefile).
Without it the program still builds and runs, it just carries every family. `-s -w` leave out the
symbol table and debug information (the library's symbol table alone is over 20 MB); Go's own
stack traces still work without them. For the counter example, stripped:

| build | size |
|---|---|
| shared library (`-tags gkshared`): program plus `libgpuikit.so` | 76 MB |
| static, without the flag: every family | 106 MB |
| static, with the flag: the four families it uses (Linux) | 52 MB |
| the same on Windows, with lld | 51 MB |
| the same on macOS (x86_64) | 32 MB |
| the same app written in Rust (Linux) | 46 MB |

The remaining 46 MB or so is GPUI itself (the renderer, text, fonts, the platform layer), which
every app needs. Each family the app uses adds what it really costs, mostly a few hundred kilobytes.

How it works, for the curious: the library's dispatch tables are split by family, and a family's
code is reachable only through its registration function, `gk_use_<family>` (generated by
`gen/gk_fam.py`). Each generated Go function calls the `useX()` of its family, which calls that
registration once, through its address (`C.gk_use_<family>`, not a call), so the reference to it
lives in Go's own code. Go's linker leaves out the functions a program never calls, and with them
the reference; the C linker then drops the families nothing refers to. On Windows the static
library is built for Rust's MinGW target with one section per function, since cgo links with
MinGW-w64 there.

Other things to know:

- `gk.LinkAll()` links and registers every family, for a program that wants them all.
- `-tags gkshared` links the shared library instead (`libgpuikit.so`, `libgpuikit.dylib` or
  `gpuikit.dll`, which the program then loads at run time). Only a build from source has it (the
  download ships the static library); the repository's own tests use it, so one build can run
  against either library variant.
- If the library ever reports that a family is "used but not linked", a generated function is
  missing its `useX()` call. That's a bug in the bindings; `gk.LinkAll()` works around it.

