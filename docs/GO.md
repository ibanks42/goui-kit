# The Go Front End (`github.com/ibanks42/goui-kit`)

The `github.com/ibanks42/goui-kit` Go module exposes a strongly typed, idiomatic Go API over the prebuilt `libgouikit` Rust library. This guide covers package organization, upstream-to-Go naming rules, execution turns and view re-description, state and handlers, handle lifetimes, specialized subsystems (lists, tables, trees, selects, overlays, docks, charts, motion, theming), dynamic vs. static linking, and runtime environment variables.

For a hands-on walkthrough that builds an app from scratch, see [docs/GETTING-STARTED-GO.md](GETTING-STARTED-GO.md). For the generated index of every component, state, and value type, see [docs/API-GO.md](API-GO.md).

---

## 1. Package Layout

The code generator (`gen/gk_go.py`) first emits a flat IR and then `gen/gk_split.py` partitions declarations across a layered acyclic package graph. Every public package imports only earlier packages in the table below (plus `internal/rt`), so imports never form cycles:

| Package | Role |
|---|---|
| `github.com/ibanks42/goui-kit/gk` | Core runtime API (`Run`, `RunHeadless`, `NewView`, `OpenWindow`, `OpenWindowWith`, `Model`, `Go`, `Post`, `Shortcut`, `After`, `Every`, `Release`, `Retain`, `Refresh`, `Notify`), GPUI primitives (`Div`, `HFlex`, `VFlex`, `Img`, `SVG`, `Text`), lengths, colors, gradients, icons, drag-and-drop, clipboard, file dialogs, menus, and theme control. |
| `github.com/ibanks42/goui-kit/gk/motion` | Animations (`WithAnimation`, `WithAnimations`), effect transitions, springs, keyframes, presence, focus traps (`FocusTrap`), and text selection scopes (`TextSelectionScopeOf`). |
| `github.com/ibanks42/goui-kit/gk/table` | Virtualized `List`, `DataTable`, `Tree`, `Select`, `Combobox`, `Command`, row sources (`NewListSource`, `NewSource`, `NewTreeSource`), and selection helpers (`SelectRow`, `Row`, `NoRow`). |
| `github.com/ibanks42/goui-kit/gk/editor` | Single-line `Input`, `Textarea`, `CodeEditor`, `NumberInput`, `OtpInput`, Markdown `TextView`, `BlockHost`, syntax highlighters, language providers, and `Diff`. |
| `github.com/ibanks42/goui-kit/gk/base` | Unstyled primitives from `gpui_kit::base` not placed in a specialized package (`base.Button`, `base.Checkbox`, `base.HResizable`, `base.Calendar`, `base.Questionnaire`). |
| `github.com/ibanks42/goui-kit/gk/kit` | Styled GPUI-kit components (`kit.Button`, `kit.Label`, `kit.Checkbox`, `kit.Switch`, `kit.Form`, `kit.Field`, `kit.TabBar`, `kit.Sidebar`, `kit.Settings`, `kit.TagSecondary`, `kit.Badge`, `kit.Alert`, `kit.NewPopupMenu`, `kit.DropdownMenu`). |
| `github.com/ibanks42/goui-kit/gk/chart` | Generic data-driven charts (`LineChart`, `AreaChart`, `BarChart`, `PieChart`, `CandlestickChart`, `RadarChart`, `SankeyChart`), `NewData`, scales, grids, and described plots (`Plot`, `NewPlotLine`, `NewPlotArea`, `NewPlotQuad`). |
| `github.com/ibanks42/goui-kit/gk/overlay` | Dialogs (`Dialog`, `OpenDialog`), alert dialogs (`AlertDialog`, `OpenAlertDialog`), sheets (`Sheet`, `OpenSheet`), popovers, hover cards, tooltips (`NewKitTooltip`, `NewKitTooltipElement`), notifications (`PushNotification`), and `overlay.Items`. |
| `github.com/ibanks42/goui-kit/gk/dock` | Dock areas (`NewAreaSkinned`, `NewArea`), panels (`NewPanel`, `KeepWhenClosed`), saved layouts (`Dump`, `Load`, `RegisterPanel`), pane trees, and custom dock renderers (`NewRenderer`, `Slot`). |
| `github.com/ibanks42/goui-kit/gk/s` | Nullary style tokens covering GPUI's `Styled` and GPUI-kit's `StyledExt` methods (`s.SizeFull`, `s.P4`, `s.Gap3`, `s.Flex1`, `s.ItemsCenter`, `s.TextXl`, `s.FontBold`). |
| `github.com/ibanks42/goui-kit/gk/theme` | Semantic theme color tokens (`theme.Background`, `theme.Foreground`, `theme.Primary`, `theme.MutedForeground`, `theme.Border`, `theme.Danger`), resolved dynamically against the active theme. |
| `github.com/ibanks42/goui-kit/gk/palette` | Fixed color palette tokens (`palette.Blue500`, `palette.Amber500`, `palette.Red500`). |
| `github.com/ibanks42/goui-kit/gk/internal/rt` | Shared internal runtime: turn stack, tape builder, handle registry, GC cleanups, purego/cgo transport, scheduler keep-alive, and panic boundary. Not importable by user apps. |
| `github.com/ibanks42/goui-kit/gk/internal/goid` | Cgo-free current-goroutine identity lookup (direct `g` register read in assembly on `amd64`/`arm64`, `runtime.Stack` fallback elsewhere) powering `*gk.Cx` UI-thread enforcement. |

---

## 2. Package Split and Naming Conventions

`gen/gk_split.py` applies five deterministic rules when placing generated declarations:

1. **Family placement**: Each upstream module belongs to a target package (`component/dialog` -> `overlay`, `component/table` -> `table`, `gpui` primitives -> `gk`, remaining `base/*` -> `base`, remaining `component/*` -> `kit`).
2. **Types move down**: If a type in an earlier package references a type from a later package in a field or method signature, the referenced type moves to the earliest package that needs it.
3. **Functions move up**: Constructors and free functions live in the latest package among their own family and the types in their parameters and return values.
4. **Cross-package builders become package functions**: When a method on an earlier-package element produces a later-package element (for instance, `Div::context_menu` producing `ContextMenuDiv`), it is emitted as a function in the later package: `kit.DivContextMenu(div, menu)`.
5. **Package-prefix trimming**: In sub-packages, leading prefixes matching the package name are stripped when the remainder is unambiguous and not bare `El` (for example, `dock.Area`, `table.State`, `table.DataTable`).

### Upstream Rust to Go Spelling Map

| Upstream Rust | Go | Notes |
|---|---|---|
| Element type `Button` | `kit.ButtonEl` | Element types append `El`; the constructor takes the bare name `kit.Button(id)`. |
| Builder value `StyleRefinement` | `gk.StyleRefinement` | Non-element builder values prefix constructors with `New` (`gk.NewStyleRefinementDefault`). |
| Kit state entity `InputState` | `editor.InputState` | Built through a state builder (`editor.NewInputState(cx).Placeholder("...").Build()`). |
| Snake-case methods & fields | Exported `CamelCase` | `text_color` -> `.TextColor(...)`; `is_today` -> `.IsToday`. |
| `Option<T>` | `gk.Option[T]` | Construct with `gk.Some(v)` or zero value / `gk.None[T]()`; inspect with `v, ok := opt.Get()`. |
| `Result<T, E>` | `gk.Fallible[T]` | Call `.Result()` for idiomatic `(T, error)`. |
| `impl Into<ElementId>` | `gk.ID` | Accepts `string`, `int`, `int64`, `uint64`, or `gk.Key(name, n)`. |
| `impl Into<Fill>` | `gk.Fill` | Accepts `gk.Color` or `gk.LinearGradient` (only where gradients are valid, such as `.Bg`). |
| `impl Into<Length>` | `gk.Length` | `gk.Px(n)`, `gk.Rems(n)`, `gk.Relative(f)`, `gk.Auto()`. |

A minimal complete view combining `init`, state reads, style tokens, and click handlers:

<!-- from: docs/snippets-go/gomd/main.go -->
```go
type App struct {
	count int
	name  editor.InputState
	root  gk.View
}

func (a *App) init(cx *gk.Cx) { a.name = editor.NewInputState(cx).Placeholder("Your name").Build() }

func (a *App) add(cx *gk.Cx) { a.count++ }

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(kit.Button("add").Primary().Label("Add").OnClickDo(a.add)).
			Child(kit.Label(fmt.Sprintf("clicked %d times", a.count)))).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))).
		Child(kit.Label("hello " + a.name.Value(cx)))
}
```

---

## 3. Running an Application, Errors, and Panics

### `gk.Run` and `gk.RunHeadless`

- **`gk.Run(start func(cx *gk.Cx)) error`**: locks the calling goroutine to its OS thread (`runtime.LockOSThread`), verifies the loaded `libgouikit` ABI and `API_HASH`, and enters the GPUI event loop. `start` runs in the `INIT` turn (create your root view and call `gk.OpenWindow` or `gk.OpenWindowWith` there). When `GK_SCRIPT=file` is set in the environment (and a `diag` library is loaded), `gk.Run` automatically delegates to `gk.RunHeadless`.
- **`gk.RunHeadless(script string, start func(cx *gk.Cx)) error`**: runs the application offscreen using the `diag` library's `HeadlessAppContext` and executes `script` (see `docs/PROTOCOLS.md` Section 11.5). Returns an error if loaded against a `ship` library without `GK_FEAT_DIAG`.

### Error and Panic Contract

Nothing in `github.com/ibanks42/goui-kit` ever calls `os.Exit`, and no Go panic ever unwinds across C into Rust. Deferred functions in `main` always run.

| Outcome | Type / Behavior | When It Happens |
|---|---|---|
| Clean quit | `nil` | User closed the last window (when `QuitOnLastWindow` is enabled) or app called `gk.Quit(cx)`. |
| App contract violation | `*gk.Error` returned by `gk.Run` | Stale `*gk.Cx`, off-thread UI call, state write inside a view function, element reused across builds or attached twice, released handle used or drawn, invalid UTF-8, unknown chart column, or invalid animation description. The error message names the offending Go file, line, and function. |
| App panic | `*gk.PanicError` re-panicked from `gk.Run` | Any Go panic inside `start`, `init`, a view function, an event handler, a posted closure, or a `gk.Go` worker is recovered at the callback boundary, requests clean shutdown, and re-panics from `gk.Run` with `Value` (accessible via `Unwrap()` when it is an `error`) and `Stack`. |
| Rust library fault | `*gk.LibraryError` returned by `gk.Run` | An internal panic inside `libgouikit` poisoned the session (`GK_ERR_POISONED` or a `PANIC` turn). |
| Startup failure | `error` returned by `gk.Run` | Shared library missing, ABI/hash mismatch, missing display on Linux, or `gk.Run` called re-entrantly (`GK_ERR_BUSY`). |

Inspecting failures in `main` uses standard Go `errors.As`:

- `var ge *gk.Error; errors.As(err, &ge)` detects an app-code binding mistake (`ge.Error()` prints the offending Go file and line).
- `var le *gk.LibraryError; errors.As(err, &le)` detects an internal Rust panic inside `libgouikit`.
- Recovering a panic around `gk.Run` yields `*gk.PanicError`, where `pe.Value` is the original panic value (`errors.Is` / `errors.As` work automatically through `pe.Unwrap()`) and `pe.Stack` holds the formatted Go stack captured at the original panic site before the library unwound.

When diagnosing element or buffered-write errors, run with `GK_DEBUG=1`: write buffering turns off so every state write fails at its own call site instead of at turn end, and every element node captures its creation stack frame.

---

## 4. `*gk.Cx`, Thread Affinity, Views, and Re-Description

### `*gk.Cx` Validity Rules

Every UI operation takes a `*gk.Cx` representing the currently active turn on the UI thread:

- **UI thread only**: Calling any `gk` function that accepts `*gk.Cx` from another goroutine stops immediately with `*gk.Error`.
- **Single-turn scope**: Never store `*gk.Cx` in a struct field or close over it for later use; using a `*gk.Cx` after its callback returns (or from an outer turn while a nested turn is active) stops with `*gk.Error`.
- **View purity**: Inside a view function (`view(cx)`), reads (`a.name.Value(cx)`, `a.count.Get()`, `gk.ReduceMotion(cx)`) are allowed, while writes (`SetValue`, `Model.Set`, `OpenWindow`, `Release`) stop with `*gk.Error`.

### Views, Tape Diffing, and Reactivity

- **`gk.NewView(initFn, viewFn)`**: Creates a `gk.View`. Pass `initFn` (or `nil`) for one-time initialization once a window exists (required for window-backed states such as `editor.InputState` and `table.ListState`). Embed child views with `.ChildView(child)` so changing a child view does not rebuild parent views.
- **Automatic dirtying & `gk.Quiet`**:
  - When a handler attached to a view runs (e.g., `.OnClickDo(a.add)`), the runtime automatically marks that view dirty when the handler returns unless the handler calls `gk.Quiet(cx)`.
  - Reading any kit state during `view(cx)` (e.g., `a.name.Value(cx)`) records that state in the tape's `deps` list, so upstream mutations (like typing into an input) automatically re-describe that view.
  - After mutating plain Go slices or fields from an input subscription, timer, or `gk.Go` completion, call `gk.Refresh(cx)` (marks the active view and its row sources dirty) or `gk.Notify(cx, view)` (marks one specific view dirty).
- **Tape diffing**: When `view(cx)` finishes, `internal/rt` compares the emitted tape bytes against the view's previous accepted tape (`bytes.Equal`). Identical tapes are not sent to Rust (the Go closure table is still refreshed in place so closures see updated captured variables).
- **`gk.Model[T]`**: Wraps a Go value with automatic view-level dependency tracking. Calling `m.Get()` inside a view subscribes that view; `m.Set(cx, v)`, `m.Update(cx, fn)`, and `m.Touch(cx)` dirty only views that read `m` and fire any `.Observe` callbacks. Use `m.Peek()` inside event handlers to read without registering a view dependency.

### Handlers, `...Do` Forms, Subscriptions, and Shortcuts

- **Full vs. `...Do` handlers**: Interactive methods that pass an event struct provide a convenience `...Do` sibling when you do not need the event payload (for example, `.OnClick(func(cx *gk.Cx, e gk.ClickEvent))` vs. `.OnClickDo(func(cx *gk.Cx))`).
- **State reads, writes, `Try` reads, and subscriptions**:
  - Generated kit states expose constructors (`New...State(cx)...Build()`), reads (allowed inside views, automatically recording view dependencies), writes (buffered until the turn ends unless a read flushes them), and `On...Event(cx, fn)` subscriptions returning a releasable `gk.Subscription`.
  - Where an upstream read can fail or return `Option<T>`, Go exposes `gk.Option[T]` (`.Get()`) or `gk.Fallible[T]` (`.Result()`), and `Try` variants return `(T, bool)` rather than stopping on an absent value.
  - Event enums arrive as Go interfaces (e.g., `editor.InputEvent`, `table.ListEvent`, `table.SelectEvent`); switch or type-assert on concrete variant structs (`editor.InputEventPressEnter`, `table.ListEventConfirm`, `table.SelectEventConfirm`).
- **Event bubbling**: Call `gk.StopPropagation(cx)` inside a click, mouse, or key handler on a nested control (such as a checkbox or delete button inside a list row) to prevent the enclosing row from also handling the click.
- **Shortcuts and actions**: `gk.Shortcut(cx, "ctrl-n", a.addTask)` (optionally scoped with `gk.WhenIn("Editor")`) creates an app action, binds the keystroke, installs an app-wide listener, and returns a `gk.Action` you can pass straight to `kit.NewPopupMenu().Menu("New task", action)` or `gk.SetMenus` so menus display the keystroke automatically. Both `func(*gk.Cx)` and `func(*gk.Cx, gk.ClickEvent)` satisfy `gk.ShortcutHandler`. For built-in GPUI-kit action structs (such as `gk.NewCancelDefault()`), wrap them with `gk.ActionOf(cx, v)`.
- **Timers**: `gk.After(cx, d, fn)` fires once and `gk.Every(cx, d, fn)` repeats between frames; call `gk.Release(cx, timer)` to cancel.
- **Background work**: `gk.Post(func(cx *gk.Cx) { ... })` is the **only** function safe to call from background goroutines; rapid posts coalesce into a single `WAKE` turn. `gk.Go(cx, work, done)` spawns a goroutine for `work()` and delivers its return value to `done(cx, v)` on the UI thread.

---

## 5. Handle Lifetimes and Resource Management

Handles (`gk.View`, kit states like `editor.InputState`, `gk.RowSource`, `gk.FocusHandle`, `gk.Timer`, `gk.Subscription`, `gk.Completion`, and library objects) wrap a generation-checked 64-bit ID. Five layered rules manage their lifetimes:

1. **Explicit `gk.Release(cx, handle)`**: Frees the Rust resource immediately, bumps its generation, and invalidates the handle. Releasing a handle twice or using it afterwards stops cleanly with `*gk.Error`.
2. **View ownership**: States, child views, row sources, subscriptions, timers, and `Built` overlays created during a view's `init` or handlers are owned by that view. Releasing the parent view releases all owned children in reverse creation order.
3. **GC fallback and pinning (`Held`)**: Tracked kit states, library objects, and focus handles register a `runtime.AddCleanup` on a shared `*life` token (observed via `weak.Pointer`). When no Go reference remains reachable, the cleanup enqueues the handle and wakes the UI thread (`gk_post`) to release it on the next `WAKE` turn. Handles transferred to Rust outside a view description (such as a dock panel inserted into a `DockArea`, or a handle explicitly pinned via `Held`) stay anchored in Go so GC will not prematurely release them while Rust holds them.
4. **Temporary library objects (`gk.Retain`)**: Non-entity Rust objects returned by reads, calls, or events (not constructors) live until the next frame's pre-frame hook sweeps them unless embedded in a view (auto-pinned via `C_OBJ_PIN`) or promoted with `gk.Retain(cx, obj)`.
5. **Dock panels (`dock.KeepWhenClosed`)**: By default, a `dock.Panel` owns its content `gk.View` and releases it when the user closes the panel. Pass `dock.KeepWhenClosed` if you plan to re-open or re-attach the same view later.

### When to Release Explicitly vs. Relying on Ownership

- **Normal app lifetime**: States created inside the root view's `init` live for the window's lifetime; you do not need to call `gk.Release` manually.
- **Dynamic sub-views, tabs, or dialogs**: Creating states inside a child view's `init` ties them to that child view; releasing the child view cleans up all of its states automatically.
- **Short-lived timers, subscriptions, or custom `RenderImage` textures**: Release them explicitly with `gk.Release(cx, handle)` as soon as you no longer need them so GPU atlas textures or repeating timers stop immediately instead of waiting for GC.

---

## 6. Subsystems Overview

### Lists, Tables, Trees, Selects, and Row Sources (`table`)

- Virtualized collections never invoke Go during paint. Back them with a row source: `table.NewListSource(cx, countFn, rowFn)`, `table.NewSource` (data tables returning `table.Cells(...)`), or `table.NewTreeSource`.
- Always give row elements unique IDs via `gk.Key("row", at.Row)`.
- Wrap scrollable lists and tables in a flex container with `s.Flex1, s.MinH0` so flex layout constrains the viewport height (without `s.MinH0`, a flex child defaults to `min-height: auto` and expands off-screen instead of scrolling).
- After mutating backing Go slices, call `gk.Refresh(cx)` so the source updates its row count and re-describes visible rows.
- `table.Select` and `table.Combobox` accept structured `gk.SearchItem` slices (filtered locally in Rust, with optional async search callbacks and declarative `SearchRules`).
- Use `table.SelectRow(cx, list, row, table.ScrollIntoView)` to update selection and scroll programmatically, and `table.NoRow` to clear selection.

### Text Inputs, Code Editors, and Markdown (`editor`)

- Single-line inputs (`editor.NewInputState(cx)`), multiline textareas, number/OTP inputs, and styled code editors (`editor.CodeEditor`) all live as GPUI entities behind state handles.
- Render Markdown natively with `editor.TextViewMarkdown(id, markdownString).Scrollable(true)`.
- Attach custom syntax highlighting via `gk.NewHighlighter` and language aliases/syntax context via `gk.NewLanguageProvider`; queries run asynchronously between frames via the `Ask` cache so typing never stalls.

### Overlays (`overlay`) and `Builder[T]` / `Built[T]`

- `overlay.OpenDialog`, `overlay.OpenAlertDialog`, and `overlay.OpenSheet` accept a `gk.Builder[T]`, which is either:
  - A reusable `gk.Built[T]` constructed once in `init` via `gk.NewBuilt(cx, fn)` (release with `gk.Release(cx, b.View)`), or
  - A function passed directly at the open call (`overlay.OpenDialog(cx, a.editView)`), which automatically allocates one view per call site and releases it with the calling view.
- `.OnOk` callbacks return `bool`: return `true` to close the overlay or `false` to keep it open (for example, on validation errors).
- Toast notifications arrive via `overlay.PushNotification(cx, message)`.
- Tooltips accept plain strings (`overlay.NewKitTooltip("Saved")`), static elements (`overlay.NewKitTooltipElement(el)`), or live `gk.View` handles.

### Docks (`dock`)

- Create a styled dock area with `dock.NewAreaSkinned(cx, "main")` and render it via `dock.View(area)`.
- Wrap views in panels with `dock.NewPanel(cx, view, dock.PanelData{Name: "notes", Title: "Notes"})`.
- Arrange left/right/bottom docks and center splits with `dock.NewLayoutTabs().PanelView(panel)`, `area.SetDock(...)`, and `area.SetCenter(...)`.
- Persist and restore layouts with `area.Dump(cx)` and `area.Load(cx, state)` after registering panel factories via `dock.RegisterPanel(cx, "notes", buildFn)`.
- Customize chrome without mid-frame callbacks via `dock.NewRenderer` and `dock.Slot()`.

### Native Menus, File Dialogs, Prompts, and Windows (`gk`)

- **`gk.OpenWindowWith(cx, gk.WindowConfig{...}, root)`**: configures title, size, minimum dimensions, initial position, `AppID` (Wayland `app_id` / X11 `WM_CLASS`), window icon PNG bytes, and title bar style.
- **`gk.SetMenus(cx, menus...)`** updates both the OS native menu bar and `kit.AppMenuBarView(kit.NewAppMenuBar(cx))`.
- **`gk.PromptForPaths`**, **`gk.PromptForNewPath`**, and **`gk.Prompt`** open native file dialogs and alerts asynchronously and return a cancelable `gk.Completion`; their callbacks run on the UI thread with `gk.PathsDone` or `gk.PromptDone` (`DoneAnswered`, `DoneCancelled`, `DoneFailed`).
- **`gk.ClipboardRead` / `gk.ClipboardWrite`** and **`gk.OpenURL`** access the OS clipboard and default browser.

### Charts, Described Plots (`chart`), Drag-and-Drop, and Window Hooks

- Generic charts (`chart.BarChart`, `chart.LineChart`, `chart.AreaChart`, `chart.PieChart`, `chart.CandlestickChart`, `chart.RadarChart`, `chart.SankeyChart`) take `chart.NewData(sliceOfStructs)`. Exported struct field names become the column names checked by `.Band("Category").Value("Total")`.
- Described plots (`chart.Plot`), drag-and-drop (`.OnDrag`, `.CanDrop`, `.DragOver`, `.OnDrop`), and close-veto hooks (`gk.OnWindowShouldClose`) execute paint and hit-testing natively in Rust:

<!-- from: docs/snippets-go/gomd/main.go -->
```go
gk.Div().ID(gk.Key("band", i)).
	OnDrag(gk.DragValue{ID: uint64(i), Text: name}, a.bandPreview).
	CanDrop(gk.DropAccept{IDs: a.bandIDs()}).
	DragOver(gk.NewStyleRefinementDefault().Bg(theme.Accent)).
	OnDrop(func(cx *gk.Cx, d gk.DragValue) { a.moveBand(int(d.ID), i) })

chart.Plot(chart.NewGrid().X(xs).Y(ys).Stroke(theme.Border),
	chart.NewPlotArea(area, gk.HSLA(0.6, 0.8, 0.7, 0.12)),
	chart.NewPlotLine(curve, 2.5, gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1))))

gk.OnWindowShouldClose(cx, a.win, func(cx *gk.Cx) bool {
	if !a.dirty { return true }
	overlay.OpenAlertDialog(cx, a.askClose)
	return false
})
```

### Motion (`motion`) and Theming (`theme`, `palette`, `s`)

- `motion.WithAnimation`, `motion.WithAnimations`, `motion.NewEffectTransition` (`kit.Effect`), springs, keyframes, presence, `FocusTrap`, and `TextSelectionScopeOf` encode declarative animation tracks onto the tape (`N_ANIMATED`) so 60 to 240 Hz interpolation runs inside GPUI with zero Go crossings.
- `theme.*` tokens resolve symbolically at decode time, so `gk.ThemeChange(cx, gk.ThemeModeDark)` updates every semantic color across all open windows immediately. `gk.Fill` accepts both `gk.Color` (`gk.HSLA`, `theme.*`, `palette.*`) and `gk.LinearGradient` (in sRGB or `.InOklab()`).
- Check `gk.ReduceMotion(cx)` inside views to respect OS reduced-motion preferences.

---

## 7. Dynamic (`purego`) vs. Static (`-tags gkstatic`) Builds

Both modes verify `GK_MAGIC`, ABI `0.6`, and `API_HASH` from `gk_info_get()` at startup before opening any window.

### Default Dynamic Mode (`!gkstatic`, `CGO_ENABLED=0`)

- Loads `libgouikit.so` (Linux), `libgouikit.dylib` (macOS), or `gouikit.dll` (Windows) via `github.com/ebitengine/purego` and calls `gk_use_all()` once on load.
- Requires **no C compiler** and supports effortless `GOOS`/`GOARCH` cross-compilation (`GOOS=windows go build`, `GOOS=darwin GOARCH=arm64 go build`).
- Uses the reflection-free `purego` callback trampoline on Linux and macOS (`amd64`/`arm64`) and `GK_FEAT_INLINE_REPLY` to avoid a second FFI call per turn.

#### Shared Library Search Order

1. **`GK_LIB`**: explicit path to the library file or a directory containing it. If set and not found, startup fails immediately without falling back.
2. **Relative to the executable**: `<exe-dir>`, `<exe-dir>/lib`, `<exe-dir>/../lib`, and on macOS `<exe-dir>/../Frameworks` (for `.app` bundles).
3. **OS loader path**: `LD_LIBRARY_PATH` (Linux), `DYLD_LIBRARY_PATH` (macOS), or `PATH` (Windows).

### Opt-In Static Mode (`-tags gkstatic`, `CGO_ENABLED=1`)

- Links `libgouikit_static.a` (plus `libgouikit_sys.so` on Linux or `libgouikit_sys.a` on Windows) via cgo and `internal/rt/capi.c`.
- `internal/rt/gen_use.go` registers only the component families referenced by your Go imports; linker dead-stripping drops unused Rust decoders and GPUI-kit families:
  - **Linux**: `CGO_ENABLED=1 CGO_LDFLAGS="-L/path/to/lib" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,--gc-sections" .`
  - **macOS**: `CGO_ENABLED=1 CGO_LDFLAGS="-L/path/to/lib" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,-dead_strip" .`
  - **Windows**: `CGO_ENABLED=1 CGO_LDFLAGS="-L/path/to/lib" go build -tags gkstatic -ldflags="-s -w '-extldflags=-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres'" .`
- Call `gk.LinkAll()` if you want every family registered unconditionally.
- On Windows, `internal/rt/gouikit_windows_amd64.syso` embeds the Common Controls 6 and DPI-awareness manifest.
- Building with `-tags gkstatic` while `CGO_ENABLED=0` fails at compile time with a clear identifier reminding you to enable cgo or drop `-tags gkstatic`.

---

## 8. Environment Variables

| Variable | Effect |
|---|---|
| `GK_LIB` | Path to the shared library file or its directory (disables fallback search when set). |
| `GK_DEBUG=1` | Disables write buffering (every call runs immediately at its Go line) and records creation stacks on every element node for richer `*gk.Error` reports. |
| `GK_INFO=1` | Prints the loaded library's ABI version, `API_HASH`, engine (`gpui-fast` vs. upstream GPUI), and version string on startup. |
| `GK_SCRIPT=path` | Causes `gk.Run` to execute offscreen via `gk.RunHeadless` using the script file at `path` (requires a `diag` library). |
| `GK_SCRIPTED_PLATFORM=1` | Routes native file dialogs, prompts, and `OpenURL` through the scripted test queue even in a live window (`diag` library). |
| `GK_TRACE_TURNS=1` | Logs every turn (`kind`, window presence) and headless script step to stderr. |
| `GK_PUREGO_REFLECT=1` | Disables the `rt_lib_dyn_fastcb.go` direct callback trampoline and falls back to `purego`'s reflection dispatcher (for A/B benchmarking). |
| `GK_NO_KEEPALIVE=1` | Disables the slow-cadence `P` scheduler keep-alive (`rt_keepalive.go`) used to avoid `sysmon` `SIGURG` preemptions at <= 60 Hz. |
| `GK_KEEPALIVE_STATS=1` | Prints keep-alive wake and preemption statistics on exit. |
