# Getting Started with GPUI-kit in Go

This guide takes you from an unpacked release archive (or a local checkout of this repository) to a working desktop application in Go. It assumes you know Go and have never used GPUI or GPUI-kit before.

---

## 1. Requirements

- **Go 1.27 or newer** (`go/go.mod` targets Go 1.27).
- **No Rust toolchain** is needed if you use a prebuilt download.
- **No C compiler** is needed for default builds. By default, Go binaries load the prebuilt shared library at startup through `github.com/ebitengine/purego` with `CGO_ENABLED=0`. The first build fetches `purego` (pinned in `go/go.sum`) from your `GOPROXY`; no other third-party Go modules are required.
- On **Linux**, opening a real window requires an active X11 (`DISPLAY`) or Wayland (`WAYLAND_DISPLAY`) session. On **macOS** (11.0+, universal `arm64`/`x86_64`) and **Windows** (10+, `x86_64` with Direct3D 11), the native window server is always available.

---

## 2. Setting Up a Project

A prebuilt download unpacks into a folder such as `goui-kit-0.0.2-linux-x86_64/` containing `lib/`, `go/`, `include/`, `examples/go/`, and `docs/`. If you are working directly inside a git checkout, run `make lib` first so `lib/ship/` exists.

### Running the Bundled Examples

From an unpacked release archive:

```sh
examples/go/build.sh hello && examples/go/hello.bin
```

On Windows (in Git Bash or MSYS2), the output binary ends in `.exe`. From a git checkout, `examples/go/build.sh hello` writes `bin/hello-go`, which you run with:

```sh
LD_LIBRARY_PATH=lib/ship bin/hello-go
# or, identically on every OS:
GK_LIB=lib/ship bin/hello-go
```

### Creating Your Own Module

Create a new directory, initialize a Go module, and point `github.com/ibanks42/goui-kit` at the unpacked `go/` directory using a `replace` directive:

```go.mod
module myapp

go 1.27

require github.com/ibanks42/goui-kit v0.0.0

replace github.com/ibanks42/goui-kit => ../goui-kit-0.0.2-linux-x86_64/go
```

Then run `go mod tidy` and `go build`.

### Finding the Shared Library at Runtime

Default builds (`CGO_ENABLED=0`) search for `libgouikit.so` (Linux), `libgouikit.dylib` (macOS), or `gouikit.dll` (Windows) in this order:

1. **`GK_LIB`**: a direct path to the library file or the directory containing it. If `GK_LIB` is set and the file is missing, startup fails immediately without falling back.
2. **Next to the executable**: `<exe-dir>`, `<exe-dir>/lib`, `<exe-dir>/../lib`, and on macOS `<exe-dir>/../Frameworks` (for `.app` bundles).
3. **The system loader path**: `LD_LIBRARY_PATH` on Linux, `DYLD_LIBRARY_PATH` on macOS, or `PATH` on Windows.

To distribute an app built in default mode, place the shared library beside your executable (or in `Contents/Frameworks` inside a macOS `.app` bundle). Because cgo is off, you can cross-compile from any OS (`GOOS=windows go build` or `GOOS=darwin GOARCH=arm64 go build`) and ship the resulting binary alongside that platform's shared library.

### Optional Single-File Static Builds (`-tags gkstatic`)

If you prefer a single executable with no shared library next to it, build with `-tags gkstatic` and `CGO_ENABLED=1`. Static builds link `libgouikit_static.a` and register only the component families your Go code actually imports, so the linker drops unused GPUI-kit families from the binary:

- **Linux** (needs `gcc`, or `clang` with `CC=clang`):
  ```sh
  CGO_ENABLED=1 CGO_LDFLAGS="-L/path/to/lib" \
    go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,--gc-sections" -o myapp .
  ```
- **macOS** (needs Xcode Command Line Tools):
  ```sh
  CGO_ENABLED=1 CGO_LDFLAGS="-L/path/to/lib" \
    go build -tags gkstatic -ldflags="-s -w -extldflags=-Wl,-dead_strip" -o myapp .
  ```
- **Windows** (needs MinGW-w64 `gcc` and `ld.lld` on `PATH` to strip unused components):
  ```sh
  CGO_ENABLED=1 CGO_LDFLAGS="-L/path/to/lib" \
    go build -tags gkstatic -ldflags="-s -w '-extldflags=-fuse-ld=lld -Wl,--gc-sections,-Xlink=-force:multipleres'" -o myapp.exe .
  ```

If you pass `-tags gkstatic` with `CGO_ENABLED=0` (or without a C compiler on `PATH`), compilation stops on a descriptive identifier reminding you to enable cgo or drop `-tags gkstatic`.

---

## 3. Tutorial: Building a Groceries App

The nine runnable programs in `docs/tutorial-go/` build a complete groceries list app one feature at a time:

| Step | Directory | Concepts Introduced |
|---:|---|---|
| 1 | `01-window` | `gk.Run`, `gk.OpenWindow`, `gk.NewView`, flex layout, style tokens (`s.*`), semantic theme colors (`theme.*`) |
| 2 | `02-state` | Struct-held Go state, element IDs, `.OnClickDo`, automatic view re-description |
| 3 | `03-list` | `init` callbacks for window-backed states, virtualized `table.ListState`, `table.NewListSource`, `gk.Key`, `gk.Refresh`, `s.Flex1`/`s.MinH0` |
| 4 | `04-input` | `editor.InputState`, `.OnInputEvent`, reading/writing input text, `table.SelectRow` + `table.ScrollIntoView` |
| 5 | `05-rows` | Checkboxes and delete buttons inside list rows, `gk.StopPropagation`, conditional styling |
| 6 | `06-dialog` | `table.ListEventConfirm`, modal `overlay.OpenDialog`, `.OnOk` validation, programmatic focus |
| 7 | `07-shortcuts` | `gk.Shortcut`, `kit.NewPopupMenu`, `kit.DropdownMenu`, shared action handlers |
| 8 | `08-theme` | Live light/dark switching with `gk.ThemeChange`, tooltips, reactive toolbar icons |
| 9 | `09-background` | Off-thread work with `gk.Go`, `gk.Post`, toast alerts via `overlay.PushNotification` |

Every Go code block below is excerpted directly from those runnable programs and verified automatically by `gates/docs-go.sh`.

---

### Step 1: Open a Window

A GPUI-kit app enters the UI loop through `gk.Run`. `gk.Run` locks the calling goroutine to its OS thread (GPUI requires all window and UI operations on one thread) and invokes your start callback once in the `INIT` turn.

Inside the start callback, you create a `gk.View` with `gk.NewView(initFn, viewFn)` and open a window with `gk.OpenWindow`. Whenever GPUI needs the view's UI tree, it calls `view(cx)` and encodes the returned element tree into a compact byte buffer (a tape) that Rust decodes into real GPUI elements.

Notice how styling works:

- Layout containers (`gk.VFlex`, `gk.HFlex`, `gk.Div`) and text controls (`kit.Label`) accept nullary style tokens from `github.com/ibanks42/goui-kit/gk/s` (`s.SizeFull`, `s.P4`, `s.Gap3`, `s.TextXl`, `s.FontBold`) as variadic arguments.
- Semantic colors come from `github.com/ibanks42/goui-kit/gk/theme` (`theme.Background`, `theme.Foreground`). They stay symbolic on the tape and resolve against the active theme whenever GPUI draws.

<!-- from: docs/tutorial-go/01-window/main.go -->
```go
package main

import (
	"log"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/kit"
	"github.com/ibanks42/goui-kit/gk/s"
	"github.com/ibanks42/goui-kit/gk/theme"
)

// view describes the window's root element tree.
// Style tokens (package s) are passed variadically to layout containers and labels;
// semantic colors (package theme) resolve dynamically against the active theme.
func view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Label("Nothing to buy yet"))
}

func main() {
	// gk.Run locks this goroutine to the UI OS thread and enters GPUI's event loop.
	// The start callback runs once in the INIT turn to open the initial window.
	if err := gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Groceries", 520, 560, gk.NewView(nil, view))
	}); err != nil {
		log.Fatal(err)
	}
}
```

Key rules to keep in mind from day one:

- **`*gk.Cx` is valid only for the duration of the callback that received it.** Never store `*gk.Cx` in a struct field or use it from another goroutine; the runtime catches both and stops cleanly with `*gk.Error`.
- **Elements are single-use.** Each element created in `view` belongs to that single build and can be attached to at most one parent.

---

### Step 2: Hold State and Handle Clicks

Application data lives in ordinary Go structs rather than framework-managed containers. Here we store `items []Item` on `*App` and add a primary button wired to `a.addMilk`:

- **Element IDs**: Every interactive element (`kit.Button("add-milk")`) takes a stable `gk.ID` so GPUI can track hover, pressed, and focus states across redraws.
- **`.OnClickDo` vs. `.OnClick`**: Use `.OnClickDo(func(*gk.Cx))` when you do not need click coordinates or modifier keys, and `.OnClick(func(*gk.Cx, gk.ClickEvent))` when you do (for instance, checking `e.ClickCount == 2`).
- **Automatic redraws**: Because `a.addMilk` is registered on this view, the runtime automatically marks the view dirty when `a.addMilk` returns and re-runs `a.view` before the next frame.

<!-- from: docs/tutorial-go/02-state/main.go -->
```go
type Item struct {
	Name   string
	Amount int
}

// App holds ordinary Go state; GPUI-kit redraws from it whenever the view is marked dirty.
type App struct {
	items []Item
}

func (a *App) view(cx *gk.Cx) gk.Element {
	// Interactive elements take an ID ("add-milk") so GPUI tracks hover, active,
	// and focus state across redraws. OnClickDo omits the ClickEvent argument.
	col := gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Button("add-milk").Primary().Label("Add milk").OnClickDo(a.addMilk)).
		Child(kit.Label(fmt.Sprintf("%d items", len(a.items))))
	for _, it := range a.items {
		col = col.Child(kit.Label(fmt.Sprintf("%s ×%d", it.Name, it.Amount)))
	}
	return col
}

// addMilk mutates Go state. Because OnClickDo runs inside this view's turn,
// the runtime marks the view dirty automatically when addMilk returns.
func (a *App) addMilk(cx *gk.Cx) {
	a.items = append(a.items, Item{Name: "Milk", Amount: 1})
}
```

---

### Step 3: Virtualized Lists and `init`

Appending every item directly into a `VFlex` works for ten rows, not for scrollable lists with keyboard navigation, selection, or thousands of entries. Instead, switch to `table.ListState` backed by a row source (`table.NewListSource`).

Two new concepts appear in Step 3:

1. **Window-backed kit state and `init`**: Components whose state lives inside GPUI (`table.ListState`, `editor.InputState`, `table.SelectState`, `dock.Area`, and so on) require an open window when they are constructed. Pass `a.init` as the first argument to `gk.NewView(a.init, a.view)`: the runtime calls `a.init` once in the `WINDOW_INIT` turn right after `OpenWindow` creates the window and before the first `a.view` call.
2. **Row sources and `gk.Refresh(cx)`**: `table.NewListSource` takes a count callback (`func() int`) and a row builder (`a.itemRow`). Only visible rows (plus overscan) are bound and described; each row is cached as its own retained view. Whenever `a.items` changes, call `gk.Refresh(cx)` so the row source re-reads the count function and re-describes visible rows.

<!-- from: docs/tutorial-go/03-list/main.go -->
```go
type App struct {
	items []Item
	list  table.ListState
}

// init runs once when OpenWindow mounts the root view, which is when window-backed
// GPUI-kit states (such as ListState) can be constructed.
func (a *App) init(cx *gk.Cx) {
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.items = append(a.items, Item{"Milk", 2}, Item{"Bread", 1}, Item{"Apples", 6})
	// Refresh notifies the view and re-syncs row source counts after changing a.items.
	gk.Refresh(cx)
}
```

Inside `itemRow`, use `gk.Key("item", at.Row)` to produce a unique `gk.ID` per row without string formatting allocations. In `view`, wrap `table.List(a.list)` in `gk.Div(s.Flex1, s.MinH0)`: `s.Flex1` lets the container take remaining vertical space, and `s.MinH0` overrides flexbox's default `min-height: auto` so the list shrinks to fit the window and scrolls properly.

<!-- from: docs/tutorial-go/03-list/main.go -->
```go
// itemRow describes one visible row on demand as its own retained view.
// gk.Key("item", at.Row) gives each row a stable, unique element ID.
func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	it := a.items[at.Row]
	return table.ListItem(gk.Key("item", at.Row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(gk.Div(s.Flex1).Child(kit.Label(it.Name))).
		Child(kit.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))))
}

func (a *App) view(cx *gk.Cx) gk.Element {
	// s.Flex1 and s.MinH0 constrain the list's height inside the flex column so it scrolls
	// instead of expanding off-screen.
	return gk.VFlex(s.SizeFull, s.P4, s.Gap3).Bg(theme.Background).TextColor(theme.Foreground).
		Child(kit.Label("Groceries", s.TextXl, s.FontBold)).
		Child(kit.Button("add-milk").Primary().Label("Add milk").OnClickDo(a.addMilk)).
		Child(gk.Div(s.Flex1, s.MinH0).Child(table.List(a.list))).
		Child(kit.Label(fmt.Sprintf("%d items", len(a.items)), s.TextSm).TextColor(theme.MutedForeground))
}

func (a *App) addMilk(cx *gk.Cx) {
	a.items = append(a.items, Item{"Milk", 1})
	// Refresh is required here so the row source re-reads len(a.items).
	gk.Refresh(cx)
}
```

---

### Step 4: Text Input and Subscriptions

Instead of a hardcoded "Add milk" button, Step 4 adds an `editor.InputState` text box so the user can type any item and press Enter.

- Create `editor.NewInputState(cx).Placeholder(...).Build()` inside `init` and render it in `view` with `editor.Input(a.newName)`.
- Subscribe to input events with `a.newName.OnInputEvent(cx, a.newNameEvent)`. Subscriptions are owned by the view whose `init` created them and are cleaned up automatically if the view is released.
- Type-assert the `editor.InputEvent` interface (`editor.InputEventPressEnter`, `editor.InputEventChange`, `editor.InputEventFocus`, `editor.InputEventBlur`) to filter for Enter.
- Read the current text with `a.newName.Value(cx)`, clear it with `a.newName.SetValue(cx, "")`, and highlight + scroll to the newly added row with `table.SelectRow(cx, a.list, len(a.items)-1, table.ScrollIntoView)`.

<!-- from: docs/tutorial-go/04-input/main.go -->
```go
// init builds both window-backed states and subscribes to input events.
func (a *App) init(cx *gk.Cx) {
	a.newName = editor.NewInputState(cx).Placeholder("Add an item and press Enter").Build()
	a.newName.OnInputEvent(cx, a.newNameEvent)
	a.list = table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()
	a.items = append(a.items, Item{"Milk", 2}, Item{"Bread", 1}, Item{"Apples", 6})
	gk.Refresh(cx)
}
// ...
// newNameEvent receives every InputEvent (change, enter, focus, blur) and adds
// a new grocery item when the user presses Enter.
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
	// Select the newly appended row and scroll it into view if the list is full.
	table.SelectRow(cx, a.list, len(a.items)-1, table.ScrollIntoView)
	gk.Refresh(cx)
}
```

---

### Step 5: Interactive Controls Inside Rows

List rows are full element trees: you can place checkboxes, tags, icons, and action buttons inside each `table.ListItem`.

Two details matter whenever you put interactive controls inside a row:

1. **Unique IDs per row**: Every interactive child needs its own row-scoped ID (`gk.Key("bought", row)`, `gk.Key("delete", row)`). Reusing `"delete"` across rows would cause GPUI to collide hover and click state between rows.
2. **Stop event bubbling**: Clicking a checkbox or delete button inside a `table.ListItem` would normally bubble up to the row and select or confirm it too. Calling `gk.StopPropagation(cx)` inside the child control's click handler prevents the click from reaching the enclosing row.

<!-- from: docs/tutorial-go/05-rows/main.go -->
```go
// itemRow embeds interactive controls inside each list row.
// Every control uses gk.Key(..., row) for a unique ID, and calls gk.StopPropagation(cx)
// so clicking the checkbox or delete button does not also select or confirm the row.
func (a *App) itemRow(cx *gk.Cx, at gk.RowAt) table.ListItemEl {
	row, it := at.Row, a.items[at.Row]
	name := kit.Label(it.Name)
	if it.Bought {
		name = name.Style(s.LineThrough).TextColor(theme.MutedForeground)
	}
	return table.ListItem(gk.Key("item", row)).Child(gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).
		Child(kit.Checkbox(gk.Key("bought", row)).Checked(it.Bought).OnClick(func(cx *gk.Cx, bought bool) {
			gk.StopPropagation(cx)
			a.items[row].Bought = bought
			gk.Refresh(cx)
		})).
		Child(gk.Div(s.Flex1).Child(name)).
		Child(kit.TagSecondary(s.TextXs).Text(fmt.Sprintf("×%d", it.Amount))).
		Child(kit.Button(gk.Key("delete", row)).Ghost().Xsmall().Icon(gk.IconDelete).OnClickDo(func(cx *gk.Cx) {
			gk.StopPropagation(cx)
			a.items = slices.Delete(a.items, row, row+1)
			gk.Refresh(cx)
		})))
}
```

---

### Step 6: Modal Dialogs

To edit an existing item's name and quantity, subscribe to `a.list.OnListEvent(cx, a.listEvent)` and watch for `table.ListEventConfirm` (fired on Enter or double-click).

Open a modal dialog with `overlay.OpenDialog(cx, a.editView)`:

- `a.editView` returns an `overlay.DialogEl` rather than a generic `gk.Element`. Passing the method directly to `overlay.OpenDialog` automatically allocates and reuses one backing overlay view for that call site; clicking the `+`/`-` buttons inside the dialog marks that overlay view dirty and updates the displayed amount live while the dialog stays open.
- Wrap the Cancel button in `overlay.DialogClose().Trigger(...)` and the primary Save button in `overlay.DialogAction().Child(...)`.
- `.OnOk(a.saveEdit)` receives the confirmation click and returns a `bool`: return `false` to reject invalid input and keep the dialog open, or `true` to commit and close it.
- Call `a.editName.Focus(cx)` immediately after `overlay.OpenDialog` so the user can start typing right away.

<!-- from: docs/tutorial-go/06-dialog/main.go -->
```go
// listEvent opens the edit dialog when the user confirms a row (Enter or double-click).
func (a *App) listEvent(cx *gk.Cx, e table.ListEvent) {
	if c, ok := e.(table.ListEventConfirm); ok {
		a.openEditor(cx, c.Row)
	}
}

// editView describes the modal dialog whenever it opens or updates.
// Passing a.editView directly to overlay.OpenDialog reuses one view automatically.
func (a *App) editView(cx *gk.Cx) overlay.DialogEl {
	amountBy := func(by int) func(*gk.Cx) {
		return func(*gk.Cx) { a.editAmount = max(1, a.editAmount+by) }
	}
	return overlay.Dialog().Title(kit.Label("Edit item")).
		Child(gk.VFlex(s.Gap3).
			Child(editor.Input(a.editName)).
			Child(gk.HFlex(s.Gap2, s.ItemsCenter).
				Child(kit.Label("Amount")).
				Child(kit.Button("less").Small().Icon(gk.IconMinus).OnClickDo(amountBy(-1))).
				Child(kit.Label(fmt.Sprint(a.editAmount))).
				Child(kit.Button("more").Small().Icon(gk.IconPlus).OnClickDo(amountBy(1))))).
		Footer(overlay.DialogFooter().
			Child(overlay.DialogClose().Trigger(kit.Button("cancel").Label("Cancel"))).
			Child(overlay.DialogAction().Child(kit.Button("save").Primary().Label("Save")))).
		OnOk(a.saveEdit)
}

// openEditor loads the chosen item into the dialog's working fields and focuses the input.
func (a *App) openEditor(cx *gk.Cx, row int) {
	a.editing = row
	a.editAmount = a.items[row].Amount
	a.editName.SetValue(cx, a.items[row].Name)
	overlay.OpenDialog(cx, a.editView)
	a.editName.Focus(cx)
}

// saveEdit validates the edited name; returning false keeps the dialog open.
func (a *App) saveEdit(cx *gk.Cx, _ gk.ClickEvent) bool {
	name := strings.TrimSpace(a.editName.Value(cx))
	if name == "" {
		return false
	}
	it := &a.items[a.editing]
	it.Name = name
	it.Amount = a.editAmount
	gk.Refresh(cx)
	return true
}
```

---

### Step 7: Keyboard Shortcuts and Menus

`gk.Shortcut` registers an action, binds its keystroke, and installs an app-wide listener in one call (add `gk.WhenIn("Context")` if you want a shortcut restricted to a focused subtree carrying `.KeyContext("Context")`).

Pass the returned `gk.Action` straight into `kit.NewPopupMenu().Menu(...)`: the menu automatically renders the keystroke hint (`Ctrl+N`, `Ctrl+Shift+D`) next to the label and dispatches the same handler when clicked. Attach the menu to any button with `kit.DropdownMenu(triggerButton, menu)`.

<!-- from: docs/tutorial-go/07-shortcuts/main.go -->
```go
	// gk.Shortcut creates an action, binds its keystroke, and installs an app-wide listener.
	// The returned gk.Action lets menus display the shortcut and dispatch the same handler.
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
```

---

### Step 8: Switching Themes

Because every `theme.*` color (`theme.Background`, `theme.Foreground`, `theme.MutedForeground`, `theme.Border`) travels as a symbolic token on the tape and is resolved against the active theme at draw time, calling `gk.ThemeChange(cx, mode)` switches the entire window between light and dark themes instantly without manually re-coloring individual elements.

<!-- from: docs/tutorial-go/08-theme/main.go -->
```go
// switchTheme flips between light and dark mode. Because theme.* tokens resolve
// when GPUI draws, every semantic color in the window updates immediately.
func (a *App) switchTheme(cx *gk.Cx) {
	a.dark = !a.dark
	mode := gk.ThemeModeLight
	if a.dark {
		mode = gk.ThemeModeDark
	}
	gk.ThemeChange(cx, mode)
	gk.Refresh(cx)
}
```

---

### Step 9: Background Work and Notifications

Never perform slow I/O, network requests, or heavy computation directly on the UI thread, and never call any `gk` UI function (or touch `*gk.Cx`) from another goroutine: the runtime detects off-thread UI calls immediately and stops with `*gk.Error`.

Use one of two primitives depending on the lifetime of your background work:

- **`gk.Go(cx, work, done)`**: For one-shot tasks. Runs `work() T` on a new goroutine and delivers its return value to `done(cx *gk.Cx, v T)` on the UI thread with a fresh `*gk.Cx`. If `work` panics, the panic is caught cleanly and re-panicked from `gk.Run` as `*gk.PanicError`.
- **`gk.Post(func(cx *gk.Cx) { ... })`**: For long-running background goroutines, tickers, or streams. Safe to call from any goroutine; rapid posts coalesce into a single `WAKE` turn on the UI thread.

Call `overlay.PushNotification(cx, message)` from the UI-thread callback to display a transient toast notification.

<!-- from: docs/tutorial-go/09-background/main.go -->
```go
// lookUpPrice runs slow work off the UI thread via gk.Go and applies the result back
// on the UI thread with a fresh *gk.Cx, optionally showing a toast notification.
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

---

## 4. Forms, Validation, and Data-Driven Charts

Once you are comfortable with lists, inputs, and dialogs, explore [examples/go/expenses/main.go](../examples/go/expenses/main.go) for a compact app combining a multi-column `kit.Form`, a `table.Select` dropdown, inline validation messages via `kit.Field().DescriptionFn(...)`, and a live `chart.BarChart`.

Charts accept any Go slice of structs wrapped in `chart.NewData(slice)`: exported struct field names (`"Category"`, `"Total"`) become columnar table names referenced by chart builders such as `.Band("Category").Value("Total")`. Naming an unknown field stops with a clear `*gk.Error`.

<!-- from: examples/go/expenses/main.go -->
```go
func (a *App) form() gk.Element {
	return kit.Form().Columns(3).
		Child(field("What", editor.Input(a.what), a.whatErr)).
		Child(field("Amount", editor.Input(a.amount), a.amountErr)).
		Child(field("Category", table.Select(a.category).Placeholder("Choose one"), a.categoryErr)).
		Footer(kit.Button("add").Primary().Label("Add expense").OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.add(cx) }))
}

func field(label string, input gk.Element, message string) kit.FieldEl {
	f := kit.Field().Label(label).Child(input)
	if message != "" {
		f = f.DescriptionFn(kit.Label(message, s.TextXs).TextColor(theme.Danger))
	}
	return f
}

type total struct {
	Category string
	Total    float64
}

// chart aggregates totals per category and feeds the struct slice directly to chart.NewData.
// Exported struct field names ("Category", "Total") are validated against the chart builders.
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
```

---

## 5. Where to Go Next

- Read [docs/GO.md](GO.md) for the full Go front-end reference: package map, `gk.Model[T]` reactivity, handle lifetimes (`Release`, ownership, GC cleanups, `Held`, `dock.KeepWhenClosed`), linking modes, and debugging flags (`GK_DEBUG`, `GK_INFO`).
- Browse [docs/API-GO.md](API-GO.md) for the rules translating upstream GPUI-kit documentation to Go and the generated index of every component, state, and value type.
- Run `go doc github.com/ibanks42/goui-kit/gk/kit ButtonEl` (or any package or type) inside any module that requires `github.com/ibanks42/goui-kit`.
- Inspect the complete example apps under `examples/go/`:
  - `hello`: minimal counter and live input echo
  - `counter`: child-view isolation and background `gk.Post` stress testing
  - `notes`: skinned `dock.Area` panels with left dock and center editor
  - `todo`: multi-project task manager with JSON persistence, `base.HResizable`, reusable `gk.Built` dialogs, and background sync loop
  - `reader`: Markdown feed reader with `editor.TextViewMarkdown`, `kit.TabBar`, `base.SliderState`, `motion.NewEffectTransition`, and `gk.Go` loading
  - `expenses`: multi-column `kit.Form`, validation, `table.Select`, and `chart.BarChart`
  - `files`: native app menus (`gk.SetMenus`), `kit.AppMenuBar`, async file open/save dialogs, prompts, and `gk.OpenURL`
  - `paint`: sRGB/Oklab linear gradients and custom `chart.Plot` geometry
  - `table`: virtualized 100,000-row by 6-column `table.DataTable`
  - `panels`: retained-view multi-panel grid driven by `gk.OnFrame`
