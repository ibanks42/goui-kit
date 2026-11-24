# Getting started

This guide takes an Odin programmer who has never used GPUI from the download to a working desktop
app. Part 1 installs what you need and runs the example that comes with the download. Part 2 is a
15-minute tutorial: a shopping list app, built in nine steps, each a complete program you can run.
After it, [API.md](API.md) explains how to read GPUI-kit's own documentation
(gpui-kit.com/docs) and know the Odin call, and lists every component.

Every snippet in this guide is cut from the tutorial's programs in `docs/tutorial/`, and those
programs are built and run by the project's tests, so the code you read here compiles against the
package you download.

## Part 1: install and run

### What you need

- One of the three systems there is a download for:
  - **Linux** on x86_64 with glibc 2.28 or newer, and the libraries `libxkbcommon`,
    `libxkbcommon-x11` and `libxcb` (installed on any desktop Linux);
  - **macOS** 11 or newer, on Apple silicon or Intel: one universal library (arm64 and x86_64);
  - **Windows** 10 or newer on x86_64, with Direct3D 11 (any Windows 10 machine).
- The Odin compiler, and the linker it uses: `clang` on Linux, the Xcode command-line tools on
  macOS (`xcode-select --install`), Visual Studio's C++ build tools on Windows (Odin calls
  `link.exe`; the free Build Tools are enough). Install Odin as
  [odin-lang.org/docs/install](https://odin-lang.org/docs/install/) describes and put `odin` on
  your `PATH`. The package is tested with the `dev-2026-11` release on all three systems. It uses
  language features of late-2026 Odin, so an older compiler may not build it; `odin version` tells
  you which one you have.
- On Windows, a shell that runs `sh` scripts for `examples/build.sh`: Git Bash (it comes with Git
  for Windows) or MSYS2. The `odin build` command it runs is shown below, for PowerShell or cmd.
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
| `lib/libgpuikit_static.a` | the library (GPUI-kit 0.11.0 on the gpui-fast engine), which your program links into itself; on macOS one universal file for Apple silicon and Intel |
| `lib/libgpuikit_sys.so` | Linux: a small linker script naming the system libraries the library uses |
| `lib/gpuikit_static.lib`, `lib/gpuikit_sys.lib` | Windows: the library in Visual Studio's format, which Odin links, and the import library of the Windows API it uses |
| `windows/` | Windows: the manifest a program needs, and `gpuikit.rc`, which embeds it |
| `odin/gk/` | the Odin package your app imports as `gpuikit:gk` |
| `README.md` | where to start: this guide |
| `examples/` | `hello`, `counter`, `notes`, `todo`, `reader`, `expenses` and `build.sh`, which builds them |
| `docs/` | this guide, [API.md](API.md), and the tutorial's programs in `docs/tutorial/` |
| `VERSIONS` | what the library was built from, and what it needs (glibc, macOS, Windows) |
| `THIRD_PARTY.md`, `licences/` | the licences of everything inside the library |

### Build and run hello

```sh
examples/build.sh hello      # about a second; writes examples/hello.bin (hello.exe on Windows)
examples/hello.bin           # a window with a button, a label and an input
```

`build.sh` is one `odin build` command, and builds any app folder too
(`examples/build.sh ../groceries`); the command itself is below. If
the window opens, everything works: the compiler, the package and the library. The other examples
are built the same way (`examples/build.sh todo`, `reader`, `notes`, `counter`, `expenses`).

If it does not start, the first line it prints starts with `gk:` and says why. The usual ones:

- `the library and this package are from different versions`: the app was built against the
  `odin/` folder of one download and the `lib/` of another. Build against the `odin/` folder
  that came with the library.
- `unable to find library -lgpuikit_static` (or `gpuikit_static.lib` on Windows) when building:
  `-extra-linker-flags` does not name the download's `lib/` folder.

### Your own app

An app is a folder with one or more `.odin` files of one package. Put it beside the unpacked
download and build it with:

```sh
G=gpuikit-0.0.2-linux-x86_64
odin build groceries -o:minimal -collection:gpuikit=$G/odin "-extra-linker-flags:-L$G/lib" -out:groceries.bin
./groceries.bin
```

`-collection:gpuikit=...` lets the app write `import gk "gpuikit:gk"`, and `-L` tells the linker
where the library is. Use `-o:minimal` while you edit (a rebuild takes a few seconds) and
`-o:speed` for what you ship.

The library is linked into your program, and only the components your app actually calls end up in
it: use `gk.button` and the button code comes along; never touch the charts and they stay out. You
don't list or import anything for this, and you don't need any extra flag: package `gk` tells the
linker to drop what isn't used (and, unless you build with `-debug`, to leave out the symbol
table). The program is one file, so shipping it means copying that file. [DESIGN.md](DESIGN.md)
section 16 explains how it works.

On macOS the same command works. Odin builds for the Mac it runs on; the library is universal, so the same command with `-target:darwin_amd64`
builds for Intel on Apple silicon (`-target:darwin_arm64` the other way round), and `lipo -create
-output groceries.bin groceries-arm64.bin groceries-x86_64.bin` makes one program for both.
`examples/build.sh` passes what follows the app's name on to `odin build`
(`examples/build.sh hello -target:darwin_amd64`). On Windows the
linker is `lld-link`, which takes the library folder as `/LIBPATH:` (in PowerShell):

```powershell
$G = "gpuikit-0.0.2-windows-x86_64"
odin build groceries -o:minimal -linker:lld -collection:gpuikit=$G/odin "-extra-linker-flags:/LIBPATH:$G/lib" -out:groceries.exe
copy $G/windows/gpuikit.manifest groceries.exe.manifest
.\groceries.exe
```

A Windows program also needs the manifest in `windows/gpuikit.manifest`, or it does not start:
the library uses common controls 6 (which Windows only loads for a program whose manifest asks for
it), and the manifest makes the window per-monitor DPI aware (without it Windows scales the window
up and it looks blurred). Either put a copy beside the program as `APP.exe.manifest` (the build
scripts do), or embed it: `rc windows\gpuikit.rc` makes `gpuikit.res`, which goes into
`-extra-linker-flags` beside `/LIBPATH:`.

## Part 2: the tutorial

You will build a shopping list: items in a list with checkboxes and delete buttons, an input to add
items, a dialog to edit one, keyboard shortcuts and a menu, a dark theme, and prices that a
background thread looks up. Each step below is a complete program in `docs/tutorial/` of the
download. Build and run one from the download's folder with
`examples/build.sh docs/tutorial/03-list && docs/tutorial/03-list.bin` (`build.sh` takes an
example's name or any app folder), or with the command above.

Five ideas carry the whole binding, and the steps introduce them one at a time:

1. **Your state is an ordinary Odin struct** (`App`). The binding never copies or owns it.
2. **A view procedure describes what the window shows** from that state, by calling element
   constructors such as `gk.button` and `gk.label` and chaining builders with `->`. It runs again
   whenever the view needs redrawing, so it only describes: it never changes anything.
3. **A handler is an ordinary procedure** that changes your state, given to an element with
   `gk.on(procedure)`. After it returns, the view it was given in is redrawn.
4. **Kit state** (an input's text, a list's selection and scroll position) lives in the library;
   your struct keeps a handle to it (`gk.Input_State`, `gk.List_State`), made once, in an `init`
   procedure.
5. **`cx: ^gk.Cx`** is the context of the call you are in (a view, a handler, `init`). Pass it on,
   never store it.

### Step 1: a window

<!-- from: docs/tutorial/01-window/main.odin -->
```odin
// Step 1 of docs/GETTING-STARTED.md: a window with a view.
package groceries

import gk "gpuikit:gk"

App :: struct {}

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full, .P_4, .Gap_3)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.label("Groceries", .Text_Xl, .Font_Bold))->
		child(gk.label("Nothing to buy yet"))
}

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Groceries", {520, 560}, gk.view(a, nil, root_view))
}

main :: proc() {
	app: App
	gk.run(&app, start)
}
```

`gk.run` starts the app: it calls `start` once, which opens a window titled "Groceries", 520 by
560 pixels, whose content is a view (`gk.open_window_with` takes the other options: a minimum size,
a position, ...).
`gk.view(a, nil, root_view)` makes that view from your state (`a`), an optional `init` procedure
(none yet) and the view procedure.

`root_view` returns one element tree. `gk.v_flex` is a column; the arguments after it are **style
tokens**: `.Size_Full` (fill the window), `.P_4` (padding), `.Gap_3` (space between children). They
are GPUI's style methods without arguments, written as enum members, and any constructor takes
them; so are the kit's own (`.Font_Bold`, `.Font_Semibold`, `.H_Flex`), and `->style(..)` adds
more to an element later. Methods with arguments stay methods: `->bg(..)`, `->text_color(..)`,
`->w(gk.px(240))`.
`gk.theme(.Background)` is a colour of the kit's theme, so the view follows light and dark themes.

A chain continued on the next line ends its line with `->`.

### Step 2: state and a handler

<!-- from: docs/tutorial/02-state/main.odin -->
```odin
Item :: struct {
	name:   string,
	amount: int,
}
// ...
App :: struct {
	items: [dynamic]Item,
}
// ...
root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	col := gk.v_flex(.Size_Full, .P_4, .Gap_3)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(gk.label("Groceries", .Text_Xl, .Font_Bold))->
		child(gk.button("add-milk")->primary()->label("Add milk")->on_click(gk.on(add_milk)))->
		child(gk.label(fmt.tprintf("%d items", len(a.items))))
	for it in a.items {
		col = col->child(gk.label(fmt.tprintf("%s ×%d", it.name, it.amount)))
	}
	return col
}
// ...
add_milk :: proc(a: ^App, cx: ^gk.Cx) {
	append(&a.items, Item{name = "Milk", amount = 1})
	// nothing else: a handler redraws the view it was given in
}
```

`gk.button("add-milk")` takes an id: a name that is unique among its siblings, which GPUI uses to
keep track of the element between redraws (`gk.id("item", 3)` makes one from a name and a number).
`->on_click(gk.on(add_milk))` gives it a handler. `add_milk` takes your state and a `cx`, appends,
and returns: the view it was given in is then redrawn, so the label shows the new count. There is
no `notify` to call and no state to copy into the view.

The view procedure builds a new tree on every call, with a plain `for` loop where the data has a
variable length. That is cheap: a view whose description did not change is not redrawn.

### Step 3: a list with your own rows

A `for` loop of labels is fine for a few items. For a list the user scrolls, selects in and that may
hold thousands of items, use the kit's List, which asks you only for the rows it shows:

<!-- from: docs/tutorial/03-list/main.odin -->
```odin
App :: struct {
	items: [dynamic]Item,
	list:  gk.List_State, // kit state: it lives in the library; the app keeps the handle
}
// ...
init :: proc(a: ^App, cx: ^gk.Cx) {
	a.list = gk.list_state(cx, gk.list_source(cx, a, item_count, item_row))->build()
	append(&a.items, Item{name = "Milk", amount = 2}, Item{name = "Bread", amount = 1}, Item{name = "Apples", amount = 6})
	gk.refresh(cx)
}
// ...
item_count :: proc(a: ^App) -> int {return len(a.items)}
// ...
item_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	it := a.items[at.row]
	return gk.list_item(gk.id("item", at.row))->child(gk.h_flex(.W_Full, .Gap_2, .Items_Center)->
		child(gk.div(.Flex_1)->child(gk.label(it.name)))->
		child(gk.tag_secondary(.Text_Xs)->text(fmt.tprintf("×%d", it.amount))))
}
```

and in the view:

<!-- from: docs/tutorial/03-list/main.odin -->
```odin
		child(gk.div(.Flex_1, .Min_H_0)->child(gk.list(a.list)))->
```

- `init` runs once, when the window first shows the view (give it as the second argument of
  `gk.view`). Kit state is made there: `gk.list_state(cx, source)->build()` makes the list's state
  in the library and returns the handle you keep in `App`.
- `gk.list_source(cx, a, item_count, item_row)` says where rows come from: a procedure that counts
  them and one that draws row `at.row`. `item_row` returns a `gk.List_Item` with whatever you put
  in it; here a label and a tag.
- `gk.refresh(cx)`, at the end of a handler (or of `init`) that changed what rows show, has every
  view and every visible row described again before the next frame, and the list recounts its rows
  with `item_count`. Unchanged rows cost a comparison.
- `.Min_H_0` and `.Flex_1` on the list's parent let the list take the remaining height and scroll
  inside it, as in CSS flexbox. A list has no height of its own: in a row (`gk.h_flex`, which
  centres its children vertically) give the list's parent `.H_Full` and the row `.Items_Start`, or
  the list is drawn zero pixels high and shows nothing.

### Step 4: an input

<!-- from: docs/tutorial/04-input/main.odin -->
```odin
App :: struct {
	items:    [dynamic]Item,
	new_name: gk.Input_State,
	list:     gk.List_State,
}
// ...
init :: proc(a: ^App, cx: ^gk.Cx) {
	a.new_name = gk.input_state(cx)->placeholder("Add an item and press Enter")->build()
	gk.on_input_event(a.new_name, cx, gk.on(new_name_event))
	a.list = gk.list_state(cx, gk.list_source(cx, a, item_count, item_row))->build()
	add_item(a, "Milk", 2)
	add_item(a, "Bread", 1)
	add_item(a, "Apples", 6)
	gk.refresh(cx)
}
// ...
add_item :: proc(a: ^App, name: string, amount: int) {
	append(&a.items, Item{name = strings.clone(name), amount = amount})
}
// ...
new_name_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	if _, enter := e.(gk.Input_Event_Press_Enter); !enter {
		return
	}
	name := strings.trim_space(string(gk.value(a.new_name, cx))) // lent text: add_item clones it
	if name == "" {
		return
	}
	add_item(a, name, 1)
	gk.set_value(a.new_name, cx, "")
	gk.select_row(cx, a.list, len(a.items) - 1, scroll = true)
	gk.refresh(cx)
}
```

The input's state is kit state like the list's: made in `init`, shown with `gk.input(a.new_name)`
in the view. `gk.on_input_event(a.new_name, cx, gk.on(new_name_event))` subscribes to its events; the handler
gets a `gk.Input_Event`, a union with one variant per event upstream has (`Input_Event_Change`,
`Input_Event_Press_Enter`, …).

`gk.value(a.new_name, cx)` reads the text. It is a `gk.Temp_String`: text the library lends you until
the handler returns. It does not convert to `string` by itself, so you cannot store it by mistake;
`string(..)` reads it, and `strings.clone` (or `gk.keep`) copies it to keep. `add_item` clones,
because `Item.name` outlives the handler. Writes such as `set_value` are applied after the handler
returns, in order.

`gk.select_row(cx, a.list, row, scroll = true)` selects a row of the list and scrolls it into view.

### Step 5: handlers in a row

<!-- from: docs/tutorial/05-rows/main.odin -->
```odin
item_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	it := a.items[at.row]
	name := gk.label(it.name)
	if it.bought {
		name = name->style(.Line_Through)->text_color(gk.theme(.Muted_Foreground))
	}
	return gk.list_item(gk.id("item", at.row))->child(gk.h_flex(.W_Full, .Gap_2, .Items_Center)->
		child(gk.checkbox(gk.id("bought", at.row))->checked(it.bought)->on_click(gk.on(toggle_bought, at.row)))->
		child(gk.div(.Flex_1)->child(name))->
		child(gk.tag_secondary(.Text_Xs)->text(fmt.tprintf("×%d", it.amount)))->
		child(gk.button(gk.id("delete", at.row))->ghost()->xsmall()->icon(.Delete)->on_click(gk.on(delete_item, at.row))))
}
// ...
toggle_bought :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	gk.stop_propagation(cx) // the row under the checkbox is not clicked as well
	a.items[row].bought = !a.items[row].bought
	gk.refresh(cx)
}
// ...
delete_item :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	gk.stop_propagation(cx)
	delete(a.items[row].name)
	ordered_remove(&a.items, row)
	gk.refresh(cx)
}
```

`gk.on(toggle_bought, at.row)` gives the handler a value as well: `toggle_bought` takes it after
`cx`. The value is copied when the row is drawn (strings too), so it is safe to give anything plain.
A handler with an event takes the event first: `proc(a: ^App, cx: ^gk.Cx, e: gk.Click_Event, row: int)`.

A click on the checkbox would also be a click on the row under it; `gk.stop_propagation(cx)` keeps
it for the checkbox. `->icon(.Delete)` is one of the kit's icons (`gk.Icon_Name`).

### Step 6: a dialog

A click on a row (or Enter on the selected row) is the list's `Confirm` event; it opens a dialog
that edits the item:

<!-- from: docs/tutorial/06-dialog/main.odin -->
```odin
list_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Event) {
	if c, ok := e.(gk.List_Event_Confirm); ok { // a click on a row, or Enter
		open_editor(a, cx, c.row)
	}
}
// ...
edit_dialog :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	return gk.dialog()->title(gk.label("Edit item"))->
		child(gk.v_flex(.Gap_3)->
			child(gk.input(a.edit_name))->
			child(gk.h_flex(.Gap_2, .Items_Center)->
				child(gk.label("Amount"))->
				child(gk.button("less")->small()->icon(.Minus)->on_click(gk.on(change_amount, -1)))->
				child(gk.label(fmt.tprintf("%d", a.edit_amount)))->
				child(gk.button("more")->small()->icon(.Plus)->on_click(gk.on(change_amount, 1)))))->
		footer(gk.dialog_footer()->
			child(gk.dialog_close()->trigger(gk.button("cancel")->label("Cancel")))->
			child(gk.dialog_action()->child(gk.button("save")->primary()->label("Save"))))->
		on_ok(gk.on(save_edit))
}
// ...
open_editor :: proc(a: ^App, cx: ^gk.Cx, row: int) {
	a.editing = row
	a.edit_amount = a.items[row].amount
	gk.set_value(a.edit_name, cx, a.items[row].name)
	gk.open_dialog(cx, gk.built(cx, a, edit_dialog))
	gk.input_state_focus(a.edit_name, cx)
}
// ...
change_amount :: proc(a: ^App, cx: ^gk.Cx, by: int) {
	a.edit_amount = max(1, a.edit_amount + by)
}
// ...
save_edit :: proc(a: ^App, cx: ^gk.Cx) -> bool {
	name := strings.trim_space(string(gk.value(a.edit_name, cx)))
	if name == "" {
		return false // keep the dialog open
	}
	it := &a.items[a.editing]
	delete(it.name)
	it.name = strings.clone(name)
	it.amount = a.edit_amount
	gk.refresh(cx)
	return true // close the dialog
}
```

- A dialog is described by a procedure like a view's, which returns a `gk.Dialog`.
  `gk.built(cx, a, edit_dialog)` makes it a small view of its own, and `gk.open_dialog` shows it.
  It is redrawn like any view: `change_amount` changes `a.edit_amount` and the dialog shows the new
  number.
- `->footer(..)` holds the buttons: `gk.dialog_close()->trigger(button)` closes the dialog, and the
  button inside `gk.dialog_action()` runs `on_ok`. `on_ok`'s handler returns a `bool`: `true`
  closes the dialog, `false` keeps it open (an empty name here). Escape closes it too.
- The dialog's input is kit state made in `init` like the others (`a.edit_name`).

### Step 7: shortcuts and a menu

<!-- from: docs/tutorial/07-shortcuts/main.odin -->
```odin
init :: proc(a: ^App, cx: ^gk.Cx) {
	a.new_name = gk.input_state(cx)->placeholder("Add an item and press Enter")->build()
	gk.on_input_event(a.new_name, cx, gk.on(new_name_event))
	a.edit_name = gk.input_state(cx)->build()
	a.list = gk.list_state(cx, gk.list_source(cx, a, item_count, item_row))->build()
	gk.on_list_event(a.list, cx, gk.on(list_event))

	a.new_item = gk.shortcut(cx, "ctrl-n", gk.on(focus_new_name))
	a.clear_bought = gk.shortcut(cx, "ctrl-shift-d", gk.on(clear_bought))
	add_item(a, "Milk", 2)
	add_item(a, "Bread", 1)
	add_item(a, "Apples", 6)
	gk.refresh(cx)
}
// ...
toolbar :: proc(a: ^App) -> gk.Element {
	menu := gk.popup_menu()->
		menu("New item", a.new_item)->
		menu("Remove bought items", a.clear_bought)
	return gk.h_flex(.Gap_2, .Items_Center)->
		child(gk.label("Groceries", .Text_Xl, .Font_Bold))->
		child(gk.div(.Flex_1))->
		child(gk.dropdown_menu(gk.button("menu")->ghost()->icon(.Menu), menu))
}
// ...
focus_new_name :: proc(a: ^App, cx: ^gk.Cx) {
	gk.input_state_focus(a.new_name, cx)
}
// ...
clear_bought :: proc(a: ^App, cx: ^gk.Cx) {
	#reverse for it, i in a.items {
		if it.bought {
			delete(it.name)
			ordered_remove(&a.items, i)
		}
	}
	gk.refresh(cx)
}
```

`gk.shortcut(cx, "ctrl-n", gk.on(focus_new_name))` binds keys to a handler, app-wide: it works
wherever the focus is. It returns a `gk.Action`, which a menu item takes, and the menu then shows
the keys next to the item. `gk.dropdown_menu(button, menu)` opens the menu under the button. To
limit a shortcut to a part of the window, give `when_in = "Items"` and mark that part with
`->key_context("Items")`.

`toolbar` is an ordinary procedure that returns an element. That is how a view is split into parts:
call it from the view procedure; it takes whatever it needs.

### Step 8: a dark theme

<!-- from: docs/tutorial/08-theme/main.odin -->
```odin
toolbar :: proc(a: ^App) -> gk.Element {
	menu := gk.popup_menu()->
		menu("New item", a.new_item)->
		menu("Remove bought items", a.clear_bought)->
		separator()->
		menu("Dark theme" if !a.dark else "Light theme", a.toggle_theme)
	return gk.h_flex(.Gap_2, .Items_Center)->
		child(gk.label("Groceries", .Text_Xl, .Font_Bold))->
		child(gk.div(.Flex_1))->
		child(gk.dropdown_menu(gk.button("menu")->ghost()->icon(.Menu), menu))->
		child(gk.button("theme")->ghost()->icon(.Sun if a.dark else .Moon)->tooltip("Toggle the theme")->on_click(gk.on(toggle_theme)))
}
// ...
toggle_theme :: proc(a: ^App, cx: ^gk.Cx) {
	a.dark = !a.dark
	gk.theme_change(cx, gk.Theme_Mode.Dark if a.dark else gk.Theme_Mode.Light)
	gk.refresh(cx)
}
```

`gk.theme_change(cx, ..)` switches every colour of the kit's theme at once; elements that use
`gk.theme(..)` colours (as `root_view` has since step 1) follow. The parameter takes a mode or a
window appearance, so the mode is written with its type, `gk.Theme_Mode.Dark`. `->tooltip(..)`
shows a tip when the pointer rests on the button.

`gk.refresh(cx)` matters here because the theme is shown in two places: the button belongs to the
view, but `toggle_theme` also runs from the shortcut, which is not given in a view.

### Step 9: work on a thread

Looking up prices is slow, so a thread does it, and posts the result back:

<!-- from: docs/tutorial/09-background/main.odin -->
```odin
Lookup :: struct {
	name:     string,
	announce: bool,
}
// ...
Priced :: struct {
	name:     string,
	price:    f64,
	announce: bool,
}
// ...
look_up_price :: proc(name: string, announce: bool) {
	// the thread gets its own copy of the name: the app may delete the item meanwhile
	thread.create_and_start_with_poly_data(Lookup{strings.clone(name), announce}, price_thread, self_cleanup = true)
}
// ...
price_thread :: proc(l: Lookup) {
	time.sleep(300 * time.Millisecond) // a slow price service
	price := 0.49 + f64(len(l.name)) * 0.35
	gk.post(price_arrived, Priced{l.name, price, l.announce}) // copied, strings and all
	delete(l.name)
}
// ...
price_arrived :: proc(a: ^App, cx: ^gk.Cx, m: Priced) {
	for &it in a.items {
		if it.name == m.name && it.price == 0 {
			it.price = m.price
		}
	}
	if m.announce {
		gk.push_notification(cx, fmt.tprintf("%s costs %.2f", m.name, m.price))
	}
	delete(m.name) // the message is this procedure's
	gk.refresh(cx)
}
```

- Only `gk.post` may be called from another thread. It copies the message (strings, slices and
  dynamic arrays included) and runs `price_arrived` on the UI thread, with a `cx`, before the next
  frame. The copy belongs to the procedure: keep it or delete it.
- A posted procedure takes the state given to `gk.run`, and redraws nothing by itself:
  `gk.refresh(cx)` shows the price.
- `gk.push_notification(cx, text)` shows a toast in the corner of the window.

The whole program, `docs/tutorial/09-background/main.odin`, is 253 lines. That is the tutorial.

## Part 3: what to know next

### Who redraws what

| after | redrawn |
|---|---|
| a handler given to an element in a view | that view |
| a handler given in a row | that row |
| a handler given in a view's `init` (an input's events, a shortcut, a timer) | that view |
| a posted procedure; a timer or shortcut made outside a view's `init` | nothing |
| `gk.refresh(cx)` anywhere | every view and every visible row (unchanged ones cost a comparison) |

When in doubt, end the handler with `gk.refresh(cx)`.

### What you may keep

| thing | lives | keep it? |
|---|---|---|
| your `App` and everything in it | as long as you keep it | yes |
| `gk.Input_State`, `gk.List_State`, `gk.View`, `gk.Action` and other handles | until `gk.release` | yes, in your struct |
| elements (`gk.label(..)`, `gk.button(..)`) | one call of the view procedure | no: make them on each call |
| `gk.Temp_String` (`gk.value(s, cx)`, event text) | until the handler returns | copy with `strings.clone` or `gk.keep` |
| `^gk.Cx` | the call it was given to | never |
| a handler's value (`gk.on(p, value)`), a posted message | copied by the binding | yes: it is yours when it arrives |

### When the app stops with a `gk:` message

A mistake in using the binding stops the app with exit code 2 and one message that says what was
wrong, what to do, and the line of your code (`at main.odin:52:3 in root_view`). For example, making
kit state in a view procedure:

```
gk: a view procedure may not create kit state (it runs on every redraw): create it once in init or in a handler and keep the handle in your state struct, as in a.search = gk.input_state(cx)->build()
    at .../groceries/main.odin:61:12 in root_view
```

Two kinds of mistake are found later than the line that made them, and the message says so:

- An element used twice, or after its view procedure returned, is found inside a builder, which
  has no line of its own. Run the app with `GK_DEBUG=1` and the message names the line that made
  the element.
- A change to kit state (`->set_value`, `->focus`) is applied after the handler returns. With
  `GK_DEBUG=1` every change is applied at once, and the message points at its line.

Build with `-debug` while developing: the binding then also catches lent memory kept past its call
(a `gk.Temp_String`, `context.temp_allocator` memory): the app stops at the read that uses it (a
debugger shows the line) with a message saying what was kept.

### Forms, selects and charts

`examples/expenses` (200 lines) adds expenses through a form with checked fields, and shows a bar
chart of the totals per category:

<!-- from: examples/odin/expenses/main.odin -->
```odin
	items: [len(Category)]gk.Search_Item
	for c in Category {
		items[c] = gk.search_item(fmt.tprint(c))
	}
	a.category = gk.select_state(cx, gk.search_source(cx, a, items[:]), nil)->build()
	gk.on_select_event(a.category, cx, gk.on(revalidate))
// ...
entry_form :: proc(a: ^App) -> gk.Element {
	what := gk.field()->label("What")->child(gk.input(a.what))
	if a.what_error != "" {
		what = what->description_fn(error_text(a.what_error))
	}
// ...
	return gk.form()->columns(3)->child(what)->child(amount)->child(category)->
		footer(gk.button("add")->primary()->label("Add expense")->on_click(gk.on(add_clicked)))
}
// ...
chart :: proc(a: ^App) -> gk.Element {
	totals: [len(Category)]Total
	for c in Category {
		totals[c].category = fmt.tprint(c)
	}
	for e in a.expenses {
		totals[e.category].total += e.amount
	}
	return gk.v_flex(.Gap_2)->w(gk.px(280))->
		child(gk.label("By category", .Font_Semibold))->
		child(gk.div()->h(gk.px(220))->child(gk.bar_chart(gk.data(totals[:]))->band("category")->value("total")))
}
// ...
	a.category_error = "Choose a category"
	if at, chosen := gk.selected_index(a.category, cx).?; chosen {
		category = Category(at.row)
		a.category_error = ""
	}
```

- A select's items are data: `gk.search_source(cx, a, items)` over `gk.Search_Item`s (a value
  and the title shown), and `gk.select_state` is made over that source. `gk.selected_index(a.category, cx)` is
  the chosen row (a `Maybe`, `nil` until something is chosen); `gk.selected_value(a.category, cx)` is its
  value as text. The last argument of `gk.select_state` preselects a row (`0` for the first),
  or is `nil` for none. Wherever a call takes a row of a list (an "index path"), the row number
  alone does: `gk.set_selected_index(a.list, cx, 4)`, `gk.scroll_to_item(a.list, cx, 4, .Top)`; the full
  `gk.Base_Index_Path{section = 1, row = 4}` is for a list with sections.
- A form is fields: `gk.field()->label(..)->child(any element)`, and `->description(..)` (text) or
  `->description_fn(..)` (an element: here a red message) under it. Checking is ordinary code: the
  handler reads the values, sets a message in your state, and the view shows it.
- A chart takes your rows as they are: `gk.data(rows[:])` over a slice of any struct, whose
  fields are the columns the builders name (`->band("category")->value("total")`, checked when the
  view runs). The rows are copied when the chart is made, so a local array of the view procedure
  is fine. A chart has no size of its own: put it in a sized `div`.

### Checking that it starts, in CI

`GK_FRAMES=5,30 ./app` draws 30 frames and exits with status 0, so a CI job can check that the
program starts and draws. On Linux without a display, run it under a virtual one:
`xvfb-run -a env GK_FRAMES=5,30 ./app` (packages `xvfb`, and Mesa's Vulkan driver
`mesa-vulkan-drivers` when the machine has no GPU). macOS and Windows CI machines have a desktop,
so the program runs there as it is.

### Where next

- [API.md](API.md): how the Odin names map from GPUI-kit's Rust documentation, and an index of every
  component with its constructor and builders.
- The examples: `expenses` (a form with checks, a select, a bar chart), `todo` (projects, a task
  list, filters, search, a JSON file) and `reader` (3,000 articles in a virtual list, tabs of
  Markdown views, a settings dialog with a slider and selects) are whole apps written this way;
  `notes` adds the kit's dock with panels.
- The generated files themselves, `odin/gk/gen_component_*.odin`: one per upstream module, each type
  with its constructor and the table of its builders, whose parameter types say what to pass.
