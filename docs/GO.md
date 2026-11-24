# How the Go package works

The Go package (`gpuikit.dev/gk`) is the front end on the library. It is generated from a
description of GPUI-kit's API, and talks to the library through a small C interface (seven C
functions and one callback). Nothing in that interface is specific to Go.

This page is for when you want to know how the Go side fits together: where things live, how the
names are made, and what rules the package follows. To write your first app, start with
[GETTING-STARTED-GO.md](GETTING-STARTED-GO.md) instead.

## 1. Layout

The binding is one module with several packages, so that `gk.` completes to the core and not to
every component, pkg.go.dev shows a page per area, and a program compiles only the packages it
imports. A package imports only the packages before it in this list, so there is no cycle:

| package | what it holds |
|---|---|
| `gpuikit.dev/gk` | the core: `gk.Run`, `gk.Cx`, views, windows, models, timers, actions, handles, events, gpui's elements (`gk.Div`, `gk.Img`, `gk.SVG`) and styles, the theme (`gk.ThemeGlobal`, `gk.Theme`), and the kit's vocabulary every package shares (`gk.Size`, `gk.IconName`, `gk.Placement`, `gk.ButtonVariant`, ...) |
| `gpuikit.dev/gk/motion` | animations, transitions, springs, motion values (`motion.WithAnimation`, `motion.Value`) |
| `gpuikit.dev/gk/table` | tables, lists, trees, selects, comboboxes and the row sources behind them (`table.NewListSource`, `table.State`) |
| `gpuikit.dev/gk/editor` | inputs and their state, the code editor, text views, highlighters, diffs (`editor.NewInputState`, `editor.Input`) |
| `gpuikit.dev/gk/base` | the unstyled primitives of `gpui_kit::base` that are in none of the packages above (`base.Checkbox`, `base.ButtonEl`) |
| `gpuikit.dev/gk/kit` | the kit's other components: buttons, labels, checkboxes, forms, menus, tabs, sidebars, ... (`kit.Button`, `kit.Label`) |
| `gpuikit.dev/gk/chart` | charts and plots (`chart.LineChart`, `chart.Plot`) |
| `gpuikit.dev/gk/overlay` | dialogs, sheets, popovers, hover cards, tooltips, toasts, notifications (`overlay.OpenDialog`) |
| `gpuikit.dev/gk/dock` | docks and their panels (`dock.NewAreaSkinned`, `dock.NewPanel`) |
| `gpuikit.dev/gk/s`, `gk/theme`, `gk/palette` | the style tokens (`s.Gap3`), the theme's colors (`theme.Background`), the kit's fixed colors (`palette.Blue500`) |
| `gpuikit.dev/gk/internal/rt` | the runtime every package calls into; apps cannot import it |

```
go/go.mod                     module gpuikit.dev (Go 1.27; requires github.com/ebitengine/purego)
go/gk/*/gen_*.go              generated (gen/gk_go.py writes one flat package, gen/gk_split.py
                              places each declaration, gen/gosplit rewrites the names)
go/gk/rt_api.go               the runtime's names apps use, re-exported by gk (aliases and calls)
go/gk/<package>/rt_*.go       each package's hand-written pieces (dock/rt_dock.go, table/rt_table.go, ...)
go/gk/internal/rt/rt_*.go     the runtime, written by hand; rt_thread_{linux,darwin,windows}.go
                              hold what differs per system
go/gk/internal/goid/          cgo-free current-goroutine identity for fast UI-thread checks on
                              amd64 and arm64
go/gk/internal/rt/gen_bases.go  the bases every generated element, value and handle embeds
go/gk/internal/rt/rt_lib_dyn*.go  the default way to the library: found and loaded at start-up,
                              called through purego (no cgo)
go/gk/internal/rt/rt_lib_static.go, capi.c  -tags gkstatic: the static library linked in through
                              cgo, and the C side of the one exported callback
go/gk/internal/rt/cgo_required.go  built only for -tags gkstatic without cgo: the first build
                              error says to install gcc
examples/go/                  module gpuikit.dev/examples: hello, counter, notes, todo, reader,
                              expenses
docs/tutorial-go/             the tutorial's nine steps (module gpuikit.dev/tutorial)
```

To build: `go build ./hello`. No C compiler is needed (cgo may be off), and `GOOS=windows` or
`GOOS=darwin` cross-compiles: the program loads the shared library when it starts (section 8).
For a single-file program with the library linked in, `-tags gkstatic` (section 8).

### Where an item lives

Each item's place follows from fixed rules (gen/gk_split.py), so it is found the same way every
time; [API-GO.md](API-GO.md) lists every element, state and value type with its package.

1. **Its family's package.** Every upstream module (`gpui_kit::component::dock`,
   `gpui_kit::base::input`, ...) is one family, and every family belongs to one package: the
   domains above take both the component and the base modules of their area (`gpui_kit::base::dock`
   and `gpui_kit::component::dock` are both `dock`); a module of no domain goes to `base` when it is
   `gpui_kit::base`'s and to `kit` when it is `gpui_kit::component`'s; gpui's own items are `gk`'s.
2. **A type goes down.** When a type of an earlier package names a type (as a field, or as a
   parameter or result of a method), that type lives in the earliest such package, since a package
   cannot import a later one. The theme's settings are why `gk.NotificationSettings`,
   `gk.SheetSettings` and `gk.Easing` are in `gk`, and a color picker's hex input is why
   `editor` comes before `base`. A few types of the kit's vocabulary that many packages name live
   in `gk` outright (`gk.ButtonVariant`).
3. **A function goes up.** A constructor or another free function lives in the latest package
   among its family's and those of every type it takes or returns: `editor.NewInputState` stays in
   `editor`; `overlay.Items` takes the item functions of every package.
4. **A method that makes an element of a later package becomes a function there,** named after the
   receiver and the method: gpui's `Div::context_menu` is `kit.DivContextMenu(div, menu)` and
   `overflow_y_scrollbar` is `kit.DivOverflowYScrollbar(div)`.
5. **Names are made per package** (section 2).

Go types the runtime itself names, and the interfaces whose members are in several packages
(`gk.AnimatableElement`, `gk.SizableNode`, `gk.PlotPart`, ...), are declared in `internal/rt`
and named by `gk` (aliases). Every generated element embeds a base from `internal/rt` that carries
its node and the markers of those interfaces, so an element is still one pointer and giving it
to `Child` does not allocate.

## 2. Names

Every Go name is made from the upstream (Rust) name by fixed rules, so GPUI-kit's documentation
translates mechanically.

| upstream | Go | rule |
|---|---|---|
| type `InputState`, `base::IndexPath` | `editor.InputState`, `table.BaseIndexPath` | CamelCase; a digit group after a digit group keeps one `_` (`Neg1_2`, but `Neg12`) |
| method `on_click`, `text_color`, `scroll_to_item` | method `OnClick`, `TextColor`, `ScrollToItem` | CamelCase of snake_case |
| initialisms `id`, `url`, `uri`, `json`, `html`, `http`, `svg`, `css`, `api`, `ascii`, `utf8` | `ID`, `URL`, ... | Go's initialisms (`element_id` -> `ElementID`) |
| element constructor `Button::new("x")`, `v_flex(..)` | `kit.Button("x")`, `gk.VFlex(..)` | the type of the element gets `El`: `kit.ButtonEl` (Go has one namespace for types and functions) |
| value or state constructor `Column::new(..)`, `InputState::new(cx)` | `table.NewColumn(..)`, `editor.NewInputState(cx)` | `New` + type name; the type keeps its name |
| enum value `ButtonVariant::Primary` | `gk.ButtonVariantPrimary` | type name + variant; `[go] enum_prefix` in `api/overrides.toml` shortens a few (`IconName` -> `Icon`: `gk.IconStarFill`) |
| enum with data (`ButtonVariant`, `Size`, `InputEvent`) | an interface: a struct type per variant with data (`gk.ButtonVariantCustom{Value: ..}`); the variants without data of a union the app gives to the kit are constants of one `Kind` type (`gk.ButtonVariantDanger`, `gk.SizeLarge` of `gk.SizeKind`) | an event's variants stay types, for a type switch (`case editor.InputEventChange:`) |
| style token `gap_3()` | `s.Gap3` | package `s` |
| theme color `background`, palette color `blue_500` | `theme.Background`, `palette.Blue500` | packages `theme` and `palette`, values of type `gk.Color` |
| record field `row`, `is_today` | `Row`, `IsToday` | exported CamelCase |
| parameter that is a Go keyword (`type`, `range`, `func`) | `type_`, `range_` | trailing underscore |

Names are made per package, with two more rules:

- **A name that starts with its package's name loses it** (or loses it after `New`), as Go
  names do (`bytes.Buffer`, not `bytes.BytesBuffer`): `dock.DockArea` is `dock.Area`,
  `dock.NewDockLayoutTabs` is `dock.NewLayoutTabs`, `table.TableState` is `table.State`,
  `editor.EditorState` is `editor.State`, `motion.MotionValue` is `motion.Value`, and an enum's
  values follow their type (`dock.PlacementLeft`, `table.EventSelectColumn`). It keeps the name
  when what is left would be `El` (`table.TableEl`, made by `table.Table`) or when the shorter name
  is taken in the package (`table.TableColumn`, since `table.Column` is another type). The
  hand-written names follow the same rule (`dock.NewAreaSkinned`, `table.NewSource`,
  `editor.Providers`).
- **A crate prefix only where it still tells two items of one package apart.** When two crates
  have a type of one name in one package, the component keeps it and the other gets a crate
  prefix (`Base`, `Gpui`, `Kit`): `kit.Checkbox` is the kit's, and in `editor`
  `editor.TextViewMarkdown` and `editor.BaseTextViewMarkdown` are the component's and the base's.
  Where the two now live in different packages the prefix is gone: `base.Checkbox`, `base.ButtonEl`
  (the unstyled primitives) beside `kit.Checkbox`, `kit.ButtonEl`. If a name still collides, it gets
  a number. The generator reports every such case (gen/out/go-report.md), so none is silent.

## 3. What app code looks like

<!-- from: docs/snippets-go/gomd/main.go -->
```go
type App struct {
	count int
	name  editor.InputState // a handle: the kit's InputState lives in the library
	root  gk.View
}

func (a *App) init(cx *gk.Cx) { a.name = editor.NewInputState(cx).Placeholder("Your name").Build() }

func (a *App) add(cx *gk.Cx) { a.count++ } // marks its view for redraw on return

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(gk.HFlex(s.Gap2, s.ItemsCenter).
			Child(kit.Button("add").Primary().Label("Add").OnClickDo(a.add)).
			Child(kit.Label(fmt.Sprintf("clicked %d times", a.count)))).
		Child(gk.Div().W(gk.Px(240)).Child(editor.Input(a.name))).
		Child(kit.Label("hello " + a.name.Value(cx))) // a read in a view: redrawn when it changes
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

- **Elements** are values with methods; a builder returns the element so calls chain. An
  element is one pointer into a node block of the view build that made it (so giving it to
  `Child` as a `gk.Element` interface does not allocate); a kept one is caught.
- **Style tokens** are typed constants in package `s` given to a constructor or to
  `.Style(...)`. Styled methods with arguments stay methods (`.W(gk.Px(240))`).
- **Options and defaults**: upstream's `self -> Self` chain is the Go chain; plain records are
  structs whose zero value is upstream's `Default` where the generator knows it. Functional
  options are used where a runtime call has optional parts (`gk.WhenIn("Tasks")`).
- **Enums** are typed constants. **Enums with data** are a Go interface with one struct per
  variant (`editor.InputEventPressEnter{Secondary: true}`); events arrive as one and are taken
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
- **Handlers that ignore the event**: every builder whose one handler takes a `gk.ClickEvent`, a
  `gk.MouseEvent` or a `gk.MouseExitEvent` and returns nothing (a click, a press or release, the
  pointer leaving) has a twin named with `Do` whose handler takes only the cx:
  `.OnClickDo(a.save)` with `func (a *App) save(cx *gk.Cx)`, `.OnMouseDownDo(gk.MouseButtonLeft, f)`.
  The generator makes them uniformly (gen/gk_go.py, `DO_EVENTS`); handlers whose payload says what
  happened (a key, a scroll, a bool, an index, a kit state's event) have the one form.
- **Overlays**: `overlay.OpenDialog`, `OpenAlertDialog`, `OpenSheet` and `OpenSheetAt` take the
  function that describes the overlay, given right there (`overlay.OpenDialog(cx, a.editView)`
  or a closure), or a `gk.Built[T]` made once with `gk.NewBuilt` (both are a `gk.Builder[T]`). A
  function gets one view per place in the code that opens it and per view whose handler opens
  it, released with that view. A popover's content is a row source (`overlay.Items`), not a
  built view, so it needs neither.
- **Lifetimes**: `gk.Release(cx, h)` gives a handle back at once. A view owns what is made in its
  turns (`init` for a window's root, its handlers, its frame handler): kit state, library
  objects, focus handles, row sources, views, overlays opened with a function; releasing the
  view releases them, the newest first. A dock panel owns its view, and a panel the dock removed
  (the user closed its tab) and did not add back is released with its view and all that made
  (`dock.KeepWhenClosed()` keeps it for the app to add again). A kit state, library object or
  focus handle the app can no longer reach is released after the collector finds it
  (`runtime.AddCleanup`; the release runs on the UI thread in a `WAKE` turn, never from the
  cleanup's goroutine); one handed to the library outside a view (an argument of a call, or of a
  value given to one, such as a panel given to a dock) is kept until it or its owner is released.
  Releasing a handle twice stops with the library's message; releasing one its view released
  already stops at the app's line; a stale handle stops where it is used.
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
- **Lists**: `table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)` and
  `table.NewListState(cx, src).Build()`; the count is asked again at every `gk.Refresh`, so a
  handler that changed the slice calls `gk.Refresh(cx)` and nothing else.
  `table.SelectRow(cx, list, row)` (with `table.ScrollIntoView`) selects without the list's event.
  `gk.NewRowSource(cx, n, row)` is the low-level source the kit's tables and trees take. Row views
  are described between frames, never in the middle of one.
- **Selects and comboboxes**: `gk.NewSearchSource(cx, []gk.SearchItem{{Value: "a", Title: "A"}})`,
  then `table.NewSelectState(cx, src, table.NoRow)` (or `gk.Some(table.Row(0))`); an empty Title is the Value.
- **Menus**: `kit.DropdownMenu(kit.Button("file").Label("File"), kit.NewPopupMenu().Menu("New", a.newAction))`.
- **Docks**: `dock.NewAreaSkinned(cx, id)`, `dock.NewPanel(cx, view, dock.PanelData{Name: .., Title: ..})`,
  `dock.SetCenter(cx, dock.NewLayoutTabs().PanelView(p))`, `dock.View(dock)` in a view.
- **Saved layouts and the rest of the dock** (go/gk/dock/rt_dock.go): `dock.Dump(cx)`
  gives a `dock.AreaState`, `dock.Load(cx, saved)` rebuilds panels through `dock.RegisterPanel(cx,
  name, f)` (f gets a `dock.PanelBuild` and answers with `dock.PanelFill`); `dock.PanelGroup`, `dock.PanelID`,
  `dock.PaneNodeKind`, `dock.PaneNodeWalk`, `dock.PaneTreeToState`, `dock.PaneTreeFromState`, `dock.BuildPanel`;
  the skin's settings `dock.SkinPanelStyle`, `dock.SetDockSkinToggleButtonVisible`,
  `dock.SetDockSkinCollapsedTabBarVisible`, `dock.SetDockSkinSplitDividerLineVisible`, ...; renderers
  `dock.NewRenderer(cx, dock.Hooks(dock.HookDock), f)`, `dock.RenderAnswer`, `dock.Slot()`;
  `dock.OnDockToggle`.
- **Tables and trees** (go/gk/table/rt_table.go): `table.NewSource(cx, rows,
  []table.TableColumn{table.NewTableColumn("name", "Name")}, cell, table.Events(f))`, `table.SetTableRows`,
  `table.InvalidateTable`, `table.SetTableColumns`, `table.SetTableTexts`, `table.SetTableRowLabels`,
  `table.SetTableLook`, `kit.RowMenu(el, menu)`; `table.SetTreeItems`, `table.TreeEntryAt`, `table.NewTreeSource`;
  `gk.SearchWillChange(cx, src, gk.SearchRules{..})` for selects.
- **Motion** (go/gk/motion/rt_motion.go): `motion.WithAnimation(el, id, motion.Animation{..}, motion.Track{..})`,
  `motion.Motion(el, motion.Value{Prop, Src: motion.ToTransition{..}})`, `motion.FocusTrap`,
  `motion.TextSelectionScopeOf(el, motion.NewTextSelectionScopeID(cx))`.
- **Editors** (go/gk/editor/rt_editor.go): `editor.CodeEditor(state, editor.CodeEditorOptions{..})`;
  `editor.Providers(cx, editor, f, editor.ProvidersOptions{..})` asks f a `editor.ProviderEvent`, answered
  with `editor.AnswerCompletions`, `editor.AnswerHover`, `editor.AnswerDefinitions`, ... Also `kit.KbdForAction`,
  `kit.NewAppMenuBar`/`kit.AppMenuBarView`, `kit.ClockNow`, `gk.VVirtualList`/`gk.HVirtualList`.
- **Charts over the app's rows**: `chart.BarChart(chart.NewData(rows)).Band("Month").Value("Total")`:
  `chart.NewData` takes a slice of structs; each exported field is a column named as the field is.
- **Effects**: `kit.Effect(el, motion.NewEffectTransition(d).Fade(0, 1), key)`; el is a
  `gk.AnimatableElement` (checked by the compiler).
- **Items** (a calendar's days, a tree's entries): a typed Go function given with the builder
  that takes it, run between frames.
- **Requests that answer later** (file dialogs, prompts): a call takes a `func(cx, answer)` that
  runs on the UI thread once the answer is there; releasing the returned handle cancels.
- **Windows**: `gk.OpenWindow(cx, title, w, h, root)`; `gk.OpenWindowWith(cx, gk.WindowConfig{..}, root)`
  (with `AppID` and a PNG `Icon`); `gk.OnWindowShouldClose(cx, w, func(cx) bool)` asks before the
  user closes a window, `gk.OnAppQuit(cx, f)` runs as the app quits, `gk.ReduceMotion(cx)` (section 7).
- **Drawing**: `chart.Plot(parts...)` over the kit's grid, axes and shapes and the binding's own
  `chart.NewPlotLine`, `chart.NewPlotArea`, `chart.NewPlotQuad`; `.Bg` takes a `gk.Fill`, a colour or a
  `gk.NewLinearGradient(..)` (section 7).
- **Drag and drop, tooltips, menus, images**: `.OnDrag(gk.DragValue{..}, preview)`, `.OnDrop`,
  `.DragOver`, `.CanDrop`; `.Tooltip(view or overlay.NewKitTooltip..)`; `gk.SetMenus`; `kit.Img` of
  `gk.ImageSourceImage`, `gk.ImageSourceRender` of a `gk.RenderImage` made once (section 7).

## 5. Errors and safety

- **Nothing exits the process.** `gk.Run` (and `gk.RunHeadless`) returns an error; main decides:
  `if err := gk.Run(start); err != nil { log.Fatal(err) }`. Deferred functions run, and an app
  can run under `go test` (core/tests/go/runerr).
- **Misuse stops**: an element used after its build, given twice, a `Cx` kept past the call it was given to, a
  released handle, a write in a view, a UI call from another goroutine: `gk` panics with a
  `*gk.Error` naming the app's line; the callback boundary recovers it, records it, asks the
  library to quit, and no app code runs in the turns that come before it has; `gk.Run` returns
  the `*gk.Error` (its text: `gk: <message>\n    at file.go:line (func)`). A mistake the library
  finds while drawing (a view showing a released handle) or when it applies a buffered write
  comes back the same way. Nothing unwinds across C.
- **A panic in app code** (in a handler, a view, `init`, a posted function or `gk.Go`'s work
  function) is recovered, the app shuts down the same way, and `gk.Run` panics again with a
  `*gk.PanicError`: `Value` is the app's panic value (`Unwrap` gives it when it is an error, so
  `errors.As` finds a `runtime.Error`), `Stack` the stack where it panicked, and its text starts
  `gk: panic in app code: ...` / `at main.go:86 (main.(*App).store)` (for `gk.Go`, the `gk.Go`
  call and line). Unrecovered, Go prints it and exits 2, as for any panic. A UI call from
  another goroutine, where no boundary is below, is recorded the same way and that goroutine
  ends (`runtime.Goexit`; its deferred calls run). A panic inside the library is a
  `*gk.LibraryError` from `gk.Run`, with the app's call when it came inside one.
- **No display** (Linux): `gk.Run` with neither `DISPLAY` nor `WAYLAND_DISPLAY` set (and no
  `GK_SCRIPT`) returns an error saying so instead of letting GPUI wait. macOS and Windows always have
  their window server, so the check is skipped there. Messages name the Go calls as an app
  writes them, package and all (`gk.NewBuilt`, `editor.NewInputState`, `table.NewSource`; `make
  gen` fails on a message naming a `gk.` function package gk does not have), and every stop says
  what to do.
- **Fallible upstream calls** return `error` (Go's way); misuse is not an error value.
- **Memory across the C interface**: the library never keeps Go memory past a call. Requests
  are Go byte slices passed for the duration of one C call (`gk_call`); the library's buffers
  are copied out at once (`gk_call`'s result is valid until the next call). A turn's reply is
  handed back either via `GK_FEAT_INLINE_REPLY` (by default on purego builds when the library
  advertises it, writing the reply pointer and length into `gk_cb` with no second crossing and
  no copy; the library parses it before the callback frame unwinds) or via `gk_reply`, which
  copies it; with `-tags gkstatic` the reply is written into C
  memory and the C half of the callback (`go/gk/internal/rt/capi.c`) hands it over after the Go
  callback has returned, as cgo's pointer rules require. Handlers, views and states are numbers on
  the C side (handler ids, handles); the Go side keeps the functions in its own tables. `gk.Post`
  passes no payload through C.
- **Threads**: package `gk` locks the main goroutine to the main OS thread in `init` (and in
  `Run`/`RunHeadless`); `gk.Run` must be called from `main`. Every call taking a `Cx` checks
  that it is on that locked UI goroutine (`go/gk/internal/goid` compares the current goroutine
  pointer on amd64 and arm64 with no syscall, falling back to OS thread IDs on other targets:
  Linux `gettid`, macOS `pthread_self`/`pthread_threadid_np`, Windows `GetCurrentThreadId`) and
  that the `Cx` belongs to the call in progress; only
  `gk.Post` may be called from elsewhere. The library calls the app back (every turn) on that same
  thread, inside `gk_run`: under purego the callback is one `purego.NewCallback` made when the
  library is loaded (purego allows about 2,000 per program and frees none, so one is fine), and it
  runs on the thread that called into C, as a cgo callback does.
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
| a described plot | `chart.Plot(parts ...gk.PlotPart)`, `chart.PlotWith(chart.PlotOptions{ID, NotInteractive, Appear}, parts...)` |
| a stroked polyline, a filled polygon, a quad in a plot | `chart.NewPlotLine(points [][2]float32, width float32, fill gk.Fill)`, `chart.NewPlotArea(points, fill)`, `chart.NewPlotQuad(x, y, w, h float32, fill gk.Fill, chart.QuadStyle{CornerRadius, BorderWidth, BorderColor})` |
| a gradient where the kit takes a Fill (`.Bg`, plot paths and quads) | `gk.NewLinearGradient(angle float32, from, to gk.LinearColorStop) gk.LinearGradient`, `.InOklab()`, `gk.NewLinearColorStop(c gk.Color, at float32)`; `gk.Fill` is a `gk.Color` or a `gk.LinearGradient` |
| drag and drop | `.OnDrag(gk.DragValue{ID uint64; Text string}, preview gk.View)`, `.OnDrop(func(cx *gk.Cx, d gk.DragValue))`, `.DragOver(gk.StyleRefinement)`, `.CanDrop(gk.DropAccept{Values bool; IDs []uint64; Paths bool})` |
| an element's tooltip | `.Tooltip(gk.ViewOrTooltip)`: a `gk.View`, `overlay.NewKitTooltip(text)` or `overlay.NewKitTooltipElement(el)`; `.HoverableTooltip` the same |
| images from bytes and pixels | `kit.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format, Bytes}})`; `img := gk.NewRenderImage(cx, gk.PixelImage{Width, Height, RGBA})` once (in init or a handler), then `gk.ImageSourceRender{Value: img}` or `gk.ImageSourceCustom{Value: img}` on every frame, `gk.Release(cx, img)` when no view shows it (gpui keeps one texture per image until then) |
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

chart.Plot(chart.NewGrid().X(xs).Y(ys).Stroke(theme.Border),
	chart.NewPlotArea(area, gk.HSLA(0.6, 0.8, 0.7, 0.12)),
	chart.NewPlotLine(curve, 2.5, gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1))))

gk.OnWindowShouldClose(cx, a.win, func(cx *gk.Cx) bool {
	if !a.dirty { return true }
	overlay.OpenAlertDialog(cx, a.askClose) // its OK calls gk.CloseWindow(cx, a.win), which does not ask again
	return false
})
```

## 8. How a program reaches the library

There are two ways, chosen when the program is built.

**The default: the shared library, loaded at start-up.** A plain `go build` makes a program
that loads `libgpuikit.so` (`libgpuikit.dylib` on macOS, `gpuikit.dll` on Windows) when it starts
the app and calls its seven functions through [purego](https://github.com/ebitengine/purego)
(`github.com/ebitengine/purego`, pinned in `go/go.mod`), with no cgo. So the build needs no C
compiler, works with `CGO_ENABLED=0`, and cross-compiles: `GOOS=windows go build` or
`GOOS=darwin GOARCH=arm64 go build` on any system. The program looks for the library, in order:

1. `GK_LIB`: the library file, or the folder that holds it (when it is set, nothing else is tried);
2. the program's folder, its `lib/` folder, and `../lib/` beside it (on macOS also
   `../Frameworks/`, so an app bundle keeps it in `Contents/Frameworks`);
3. the system loader's own search by name (`LD_LIBRARY_PATH` and the system's folders on Linux,
   `DYLD_LIBRARY_PATH` and `DYLD_FALLBACK_LIBRARY_PATH` on macOS; on Windows the program's folder,
   the system folders and `PATH`).

When it is in none of them, `gk.Run` stops with a message listing every place it looked. A file
that is found but does not load (a library for another system, a missing system library) stops
with the loader's own message. Ship the library beside the program. The shared library holds every
component family, so the program itself is small (the counter example: 2.3 MB stripped, beside
the 80 MB `libgpuikit.so`).

**`-tags gkstatic`: the static library, linked in.** A program built with `-tags gkstatic` links
`libgpuikit_static.a` through cgo, so it is one file with nothing to ship beside it, and only the
component families the program calls into end up in it. It needs a C compiler for cgo (gcc or
clang, the Xcode command-line tools on macOS, MinGW-w64 gcc on Windows) and builds for the system
it runs on. Without cgo (`CGO_ENABLED=0`, or no C compiler) the build stops at
`gk_static_needs_cgo__install_gcc_or_clang_and_build_with_CGO_ENABLED_1_or_drop_tag_gkstatic`.

There is nothing to list. Every function of the packages (`gk`, `kit`, `editor`, ...) that makes an
element or calls into the library also links its family, whatever package it lives in: `kit.Button` links the button family, `editor.NewInputState` the input
family, `chart.LineChart` the chart family. Go's linker already leaves out the functions a program
never calls, so a program that never calls `chart.LineChart` never links the chart family either.

What it needs from you is the tag and one flag on `go build`:

```sh
CGO_LDFLAGS="-L<lib dir>" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,--gc-sections" .   # Linux
CGO_LDFLAGS="-L<lib dir>" go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,-dead_strip" .     # macOS
CGO_LDFLAGS="-L<lib dir>" go build -tags gkstatic -ldflags="-s -w '-extldflags=-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres'" .   # Windows
```

`-Wl,--gc-sections` (Apple's linker calls it `-dead_strip`) tells the C linker to drop the code
that nothing uses. On Windows it needs LLVM's linker, lld: MinGW's GNU ld keeps every function
that has unwind data there, which is all of them. `-Xlink=-force:multipleres` lets the manifest
the runtime embeds (`gk/internal/rt/gpuikit_windows_amd64.syso`) replace the default one MinGW's gcc adds. Go doesn't let a package
ask for that flag itself, which is why it goes on the command line (or in a script or Makefile).
Without it the program still builds and runs, it just carries every family. `-s -w` leave out the
symbol table and debug information (the library's symbol table alone is over 20 MB); Go's own
stack traces still work without them. For the counter example, stripped:

| build | size |
|---|---|
| default: the program, plus `libgpuikit.so` beside it | 2.3 MB + 80 MB |
| static, without the flag: every family | 106 MB |
| static, with the flag: the four families it uses (Linux) | 52 MB |
| the same on Windows, with lld | 51 MB |
| the same on macOS (x86_64) | 32 MB |
| the same app written in Rust (Linux) | 46 MB |

The remaining 46 MB or so is GPUI itself (the renderer, text, fonts, the platform layer), which
every app needs. Each family the app uses adds what it really costs, mostly a few hundred kilobytes.

How it works, for the curious: the library's dispatch tables are split by family, and a family's
code is reachable only through its registration function, `gk_use_<family>` (generated by
`gen/gk_fam.py`). Each generated Go function, in whichever package it lives, calls the `rt.UseX()`
of its family (go/gk/internal/rt/gen_use.go; empty without gkstatic, gen_use_dyn.go), which calls that registration once, through its address (`C.gk_use_<family>`, not a call), so the reference to it
lives in Go's own code. Go's linker leaves out the functions a program never calls, and with them
the reference; the C linker then drops the families nothing refers to. On Windows the static
library is built for Rust's MinGW target with one section per function, since cgo links with
MinGW-w64 there.

Other things to know:

- `gk.LinkAll()` links and registers every family, for a static program that wants them all (the
  shared library registers them all when it is loaded).
- The repository's own tests use the default build, so one program runs against either library
  variant (`lib/ship` or `lib/diag`, chosen with `LD_LIBRARY_PATH`); `gates/go-apps.sh` with
  `GK_STATIC=1` builds the apps the static way and checks their sizes.
- What a turn costs: the library calls the Go callback once per turn either way, and when
  `GK_FEAT_INLINE_REPLY` is advertised the default purego build returns its reply inline through
  `gk_cb` with no second crossing (falling back to `gk_reply` otherwise). purego enters a callback
  through reflection, a few microseconds a turn when the caches are cold between frames, so the
  package calls the turn callback directly instead (go/gk/internal/rt/rt_lib_dyn_fastcb.go, on
  Linux and macOS for amd64 and arm64, checked by a probe call when the library loads;
  `GK_PUREGO_REFLECT=1` turns it off, to compare). Measured on 2026-11-23 with gates/fps.sh at
  60 Hz (Linux, 10 rounds): callback time per frame, cgo against purego direct against purego
  through reflection, was 14.6, 16.0 and 20.3 us in the counter clicked on every frame (p50 frame
  CPU 1.05 to 1.06, 1.04 and 1.07 x Rust), and 20.4, 23.5 and 29.0 us in panels 60 64 2 (1.025 to
  1.037, 1.039 and 1.05 x Rust): within the runs' own noise for the direct call.
- If the library ever reports that a family is "used but not linked", a generated function is
  missing its `useX()` call. That's a bug in the bindings; `gk.LinkAll()` works around it.

