# API guide for Go: from GPUI-kit's documentation to the Go call

GPUI-kit is documented for Rust, at gpui-kit.com/docs and in its rustdoc. The Go package is
generated from that same API by fixed rules, so the Rust documentation is also the Go
documentation once you know the rules. This page gives the rules, then an index of every component
the package has, generated from the package itself so it is always current. If this is your first
app, read [GETTING-STARTED-GO.md](GETTING-STARTED-GO.md) first. [GO.md](GO.md) explains how the Go
package works.

Every Go call on this page also appears in `docs/snippets-go/api/main.go`, a program that the
project's tests compile, so none of them is out of date.
<!-- spans: docs/snippets-go/api/main.go -->

## Where things are

- `import "gpuikit.dev/gk"` is the core (the app, views, windows, gpui's elements, the theme), and
  each area has its package: `gk/motion`, `gk/table`, `gk/editor`, `gk/base` (the unstyled
  primitives), `gk/kit` (the other components), `gk/chart`, `gk/overlay` and `gk/dock`
  ([GO.md](GO.md) section 1 says what goes where), with three small packages beside them:
  `gpuikit.dev/gk/s` (the style tokens, `s.Gap3`), `gpuikit.dev/gk/theme` (the theme's colors,
  `theme.Background`) and `gpuikit.dev/gk/palette` (fixed colors, `palette.Blue500`). The index
  below names every type with its package.
- `go/gk/<package>/gen_component_NAME.go` holds the types of upstream's `gpui_kit::component::NAME` module,
  `gen_base_*.go` the unstyled primitives of `gpui_kit::base`, `gen_state.go` every kit state type
  (handles), `gen_types.go` the enums, records and events. Every type and method has a doc comment
  naming its upstream path (`// ButtonEl is gpui_kit::component::button::Button.`), so searching the
  package (or `go doc gpuikit.dev/gk/kit ButtonEl`) for the upstream name finds it.
- `rt_*.go` (in `go/gk` and each package; the runtime itself in `go/gk/internal/rt`) is the hand-written part: `gk.Run`, `gk.NewView`, `gk.Refresh`, `gk.Post`, `gk.Go`,
  `gk.Shortcut`, `table.NewListSource` and the other functions that have no single upstream
  counterpart.
- Not every upstream function is bound yet. Each one that is missing is listed, with the reason,
  in [STATUS.md](STATUS.md).

## Names

The Go names follow fixed rules (docs/GO.md section 2).

| Rust (gpui-kit.com/docs) | Go | rule |
|---|---|---|
| `Button::new("ok").primary().label("OK")` | `kit.Button("ok").Primary().Label("OK")` | `Type::new` is a function named after the type; methods are CamelCase and chain |
| `Button` (the type) | `kit.ButtonEl` | an element's Go type has `El` after the name (Go has one namespace for types and functions) |
| `Tag::secondary()` | `kit.TagSecondary()` | any other constructor: type name, then the constructor's name |
| `InputState::new(..)`, `Column::new(..)` | `editor.NewInputState(cx)`, `table.NewColumn(..)` | the constructor of a state or a value is `New` + its type name; the type keeps the name (`editor.InputState`) |
| `v_flex()`, `h_flex()`, `div()` | `gk.VFlex()`, `gk.HFlex()`, `gk.Div()` | free functions are CamelCase |
| `IconName::StarFill`, `ButtonVariant::Primary` | `gk.IconStarFill`, `gk.ButtonVariantPrimary` | an enum value is its type's name and the variant (a few shortened, as `Icon`) |
| `gpui_kit::base::Checkbox` beside `gpui_kit::component::Checkbox` | `base.Checkbox("raw")` beside `kit.Checkbox(..)` | the unstyled primitives of `gpui_kit::base` are package `base`'s (or their area's); where a base and a component type of one name share a package, the component keeps it and the base one gets `Base` (`editor.BaseTextViewMarkdown`) |
| `DockArea`, `TableState`, `MotionValue` | `dock.Area`, `table.State`, `motion.Value` | a name that starts with its package's name loses it (unless only `El` would be left, or the short name is taken) |
| `InputEvent::PressEnter` | `editor.InputEventPressEnter` | an enum with data is an interface: an event's variants are struct types, taken apart with a type switch or `e.(editor.InputEventPressEnter)` |
| `ButtonVariant::Danger`, `Size::Large`, `ButtonVariant::Custom(..)` | `gk.ButtonVariantDanger`, `gk.SizeLarge`, `gk.ButtonVariantCustom{Value: ..}` | in an enum with data that you give to the kit, a variant without data is a constant (of `gk.ButtonVariantKind`), as a plain enum's value; a variant with data is a struct |
| `ListEvent::Confirm(IndexPath)` | `c.Row` on a `table.ListEventConfirm` | a variant's single record is embedded, so its fields are reached directly; a tuple variant's one value is `Value`, more are `V0`, `V1`, ... |
| `element_id`, `url` | `ElementID`, `URL` | Go's initialisms |
| a parameter named `type`, `range` | `type_`, `range_` | a Go keyword gets a trailing underscore |

## Styles are tokens

GPUI's style methods without arguments (`size_full()`, `gap_2()`, `items_center()`, `text_sm()`,
`line_through()`) are constants of package `s`, given to any constructor or to `.Style(..)`:

| Rust | Go |
|---|---|
| `div().size_full().gap_2().items_center()` | `gk.Div(s.SizeFull, s.Gap2).Style(s.ItemsCenter)` |
| `.w(px(240.)).h(rems(2.))` | `.W(gk.Px(240)).H(gk.Rems(2))` |
| `.w(relative(0.5))` | `.W(gk.Relative(0.5))` |
| `.bg(cx.theme().muted)` | `.Bg(theme.Muted)` |
| `.text_color(cx.theme().warning)` | `.TextColor(theme.Warning)` |
| `.border_color(blue_500())` | `.BorderColor(palette.Blue500)` |
| `.bg(hsla(0.6, 0.5, 0.5, 1.))` | `.Bg(gk.HSLA(0.6, 0.5, 0.5, 1))` |
| `.font_bold()` | `kit.Label("bold", s.FontBold)` |

Methods with arguments stay methods. A length is `gk.Px`, `gk.Rems`, `gk.Relative` or `gk.Auto()`.
A label's line height comes with the kit's text sizes (`s.TextSm`, `s.TextXl`); a label given its own
size, `.TextSize(gk.Px(64))`, keeps the old line height and spills onto its neighbours, so give it
`.LineHeight(gk.Px(80))` as well.
A theme colour is resolved by the library, so a view using `theme.` colours follows
`gk.ThemeChange`. The kit's own style helpers without arguments (`StyledExt`: `font_bold`,
`font_semibold`, `h_flex`, ...) are tokens too (`s.FontBold`); their method form still compiles.

Rust's `.when(cond, |this| this.bg(..))` is an ordinary `if` in Go, since a chain is a value:

<!-- from: docs/snippets-go/api/main.go -->
```go
	if a.dark {
		row = row.Bg(theme.Muted)
	}
```

## Options and defaults

- Every builder method is optional; leaving it out leaves upstream's default, exactly as in Rust.
- `impl Into<SharedString>` and `&str` are `string`; the binding copies it.
- `impl Into<ElementId>` is `gk.ID`: a string (`kit.Button("ok")`), an int, or a name and a number
  (`gk.Key("row", 3)`).
- `Option<T>` is `gk.Option[T]`: `gk.Some(..)` of a value, or the zero value for none. A read that may have
  nothing returns one; `.Get()` takes it apart (`if at, chosen := a.list.SelectedIndex(cx).Get(); chosen`).
- `usize` (counts, indices) is `int`; a negative one stops the app with a message instead of
  wrapping.
- `Duration` is `time.Duration`; `Pixels` and lengths are `gk.Length` (`gk.Px(240)`).
- A struct of options with Rust's `Default` is a Go struct whose zero value is the default:
  `gk.OpenWindowWith(cx, gk.WindowConfig{Width: 320, Height: 240, Title: "More", MinWidth: 200}, gk.NewView(nil, a.columns))`
  sets four fields and leaves the rest.
- An `IndexPath` (a row of a list, a select or a combobox) is `table.Row(n)`, or the full
  `table.BaseIndexPath{Section: 1, Row: 4}` for a list with sections; an `Option<IndexPath>` is
  `gk.Some(table.Row(4))` or `table.NoRow`: `a.list.SetSelectedIndex(cx, gk.Some(table.Row(4)))`,
  `a.list.ScrollToItem(cx, table.Row(4), gk.ScrollStrategyTop)`, `a.list.SetSelectedIndex(cx, table.NoRow)`.
- `impl Into<ThemeMode>` and the other parameters that take several types are an interface
  (`gk.ThemeModeInto`) that each of those types satisfies: `gk.ThemeChange(cx, gk.ThemeModeDark)`.
  A value of another type stops with a message naming the ones it takes.
- An upstream call that returns `Result` returns a Go `error` (a `*gk.KitError` with upstream's
  text): `if err := a.name.ReplaceWithToken(cx, tok); err != nil { .. }`. A mistake in using the
  binding is not an error value: it stops the app (below).

## State, handles and handlers

| Rust | Go |
|---|---|
| `struct App { .. }` with `impl Render` | your struct, and a view method `func (a *App) view(cx *gk.Cx) gk.Element` |
| `cx.new(\|cx\| App { .. })` | `gk.NewView(a.init, a.view)`: an optional `init` (or `nil`), the view function |
| `cx.open_window(options, \|window, cx\| ..)` | `gk.OpenWindow(cx, "API", 640, 480, gk.NewView(a.init, a.view))`; every other option: `gk.OpenWindowWith` |
| `cx.new(\|cx\| InputState::new(window, cx).placeholder("Name"))` | `editor.NewInputState(cx).Placeholder("Name").Build()`, in `init`, kept in your struct |
| `Input::new(&self.name)` | `editor.Input(a.name)` |
| `self.name.read(cx).value()` | `a.name.Value(cx)` (a Go string, the app's to keep) |
| `self.name.update(cx, \|s, cx\| s.set_value("", window, cx))` | `a.name.SetValue(cx, "")` (applied when the handler returns) |
| `cx.subscribe(&self.name, \|this, _, e: &InputEvent, cx\| ..)` | `a.name.OnInputEvent(cx, a.nameEvent)` |
| `.on_click(cx.listener(\|this, e: &ClickEvent, window, cx\| ..))` | `.OnClick(a.clicked)` with `func (a *App) clicked(cx *gk.Cx, e gk.ClickEvent)` |
| a closure that captures `ix` | a Go closure: `.OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.remove(cx, 3) })` |
| `cx.stop_propagation()` | `gk.StopPropagation(cx)` |
| `cx.notify()` | nothing: a handler redraws its own view; after a change other views show, `gk.Refresh(cx)` |
| `Entity<T>` read by several views | `gk.Model[T]`: `a.count.Get()` in a view records it, `a.count.Set(cx, a.count.Peek()+1)` redraws exactly those views |
| `actions!(app, [Save])`, `KeyBinding::new("ctrl-s", Save, None)`, `.on_action(..)` | `gk.Shortcut(cx, "ctrl-s", a.saveNow)`, which returns the `gk.Action` a menu item takes |
| `ListDelegate` (a trait the app implements for a List) | `table.NewListState(cx, table.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()` |
| `window.open_dialog(cx, \|dialog, window, cx\| ..)` | `overlay.OpenAlertDialog(cx, a.ask)` over `a.ask = gk.NewBuilt(cx, a.confirm)` made in `init` (or `overlay.OpenDialog` for a `overlay.DialogEl`) |
| `window.push_notification("Saved", cx)` | `overlay.PushNotification(cx, "Saved")` |
| `Theme::change(ThemeMode::Dark, ..)` | `gk.ThemeChange(cx, gk.ThemeModeDark)` |
| `cx.spawn(async ..)` / a background executor | `gk.Go(cx, ..)` with two functions: the first runs on a goroutine, the second gets its result on the UI thread; or any goroutine calling `gk.Post(func(cx *gk.Cx) { .. })` |
| `cx.on_next_frame`, a `Timer` | `gk.After(cx, 500*time.Millisecond, a.tick)`, `gk.Every`, `gk.OnFrame` |
| `WindowOptions { app_id: Some("notes".into()), icon: Some(..), .. }` | `gk.OpenWindowWith(cx, gk.WindowConfig{Title: "Notes", AppID: "notes", Icon: pngBytes}, root)` |
| `window.on_window_should_close(cx, \|window, cx\| ..)` returning false | `gk.OnWindowShouldClose(cx, a.win, func(cx *gk.Cx) bool { .. })`; close it later with `gk.CloseWindow` |
| `cx.on_app_quit(\|cx\| ..)` | `gk.OnAppQuit(cx, a.quitting)` |
| `cx.reduce_motion()` | `gk.ReduceMotion(cx)` (`gk.SetReduceMotion` writes it) |
| `.bg(linear_gradient(90., linear_color_stop(cyan, 0.), linear_color_stop(violet, 1.)))` | `.Bg(gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1)))` (`.InOklab()` for `.color_space(ColorSpace::Oklab)`) |
| `impl Plot` (or a `canvas`) whose paint calls `Grid::paint`, `PathBuilder` and `paint_path`, `paint_quad` | `chart.Plot(chart.NewGrid().., chart.NewPlotArea(points, fill), chart.NewPlotLine(points, width, fill), chart.NewPlotQuad(x, y, w, h, fill, chart.QuadStyle{..}))`, points in pixels from the plot's top left |
| `.on_drag(value, \|v, _, _, cx\| cx.new(\|_\| Preview))`, `.drag_over::<T>(\|s, _, _, _\| s.bg(..))`, `.on_drop(\|v: &T, _, cx\| ..)` | `.OnDrag(gk.DragValue{ID: 7, Text: "seven"}, a.preview)`, `.DragOver(gk.NewStyleRefinementDefault().Bg(..))`, `.CanDrop(gk.DropAccept{IDs: []uint64{7}})`, `.OnDrop(func(cx *gk.Cx, d gk.DragValue) { .. })`; one value type, kinds told apart by ID |
| `.tooltip(\|window, cx\| Tooltip::new("Saved").build(window, cx))` | `.Tooltip(overlay.NewKitTooltip("Saved"))`, `.Tooltip(overlay.NewKitTooltipElement(el))`, or `.Tooltip(a.tipView)` (a `gk.View`) |
| `cx.set_menus(vec![Menu { name: "App".into(), items: vec![MenuItem::action("Quit", Quit)] }])` | `gk.SetMenus(cx, gk.Menu{Name: "App", Items: []gk.MenuItem{gk.MenuAction{Name: "Quit", Action: quit}}})` |
| `img(ImageSource::Render(..))`, `img(Arc::new(Image::from_bytes(ImageFormat::Png, bytes)))` | `kit.Img(gk.ImageSourceRender{Value: img})`, `img` made once by `gk.NewRenderImage(cx, gk.PixelImage{Width: w, Height: h, RGBA: px})` and given back with `gk.Release`, `kit.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: b}})` |

A handler is any Go function of the builder's shape: a method value (`a.clicked`) or a closure.
Its first parameter is `cx`, then the event if the builder hands one (`gk.ClickEvent`, `bool` for a
checkbox or a switch, `editor.InputEvent`, ...). The shape is the builder's parameter type, so a handler
that does not fit does not compile, and the compiler's message names the type to write. A handler
returning a value (a dialog's `OnOk`, `func(cx *gk.Cx, e gk.ClickEvent) bool`) is allowed where
upstream's closure returns one. `gk.Shortcut` takes a `func(cx *gk.Cx)` or a click handler, so one
method serves a button and its keys.

## Lifetimes: what is safe to keep

| what | valid | to keep it |
|---|---|---|
| your state struct | as long as you keep it | the binding keeps only the functions you gave it (method values hold the pointer); Go's collector keeps it alive |
| elements and values (`kit.Label(..)`, a menu, a column) | one call of the view function that made them | make them again on each call; a function that returns one is how parts are reused |
| handles: `editor.InputState`, `table.ListState` and every kit state type, `gk.View`, `gk.Action`, `gk.Window`, `gk.Timer`, row sources | until `gk.Release(cx, a.list)` | keep them in your struct |
| strings a read or an event hands you (`a.name.Value(cx)`) | always: a Go string is a copy | assign it anywhere |
| `*gk.Cx` | the call it was given to | never; every function the binding calls gets one |
| what a closure captures | as long as the closure is kept | nothing to do: the collector keeps it alive |
| a library object a read or an event hands out (a temporary handle) | until the next frame | `gk.Retain` it |

Only `gk.Post` may be called from another goroutine. Breaking a rule stops the app with a message
that names it and the line; see "When the app stops" in [GETTING-STARTED-GO.md](GETTING-STARTED-GO.md).

## Not in Go yet

Every upstream function that is not bound yet is listed, with the reason, in
[STATUS.md](STATUS.md).

A builder whose parameter type Go cannot carry yet is left out of the package, so app code cannot
call it; the few kept for a record's other fields stop with "... is not carried by the Go package
yet: leave out the builder that takes it".

## Component index

Every element, kit state and value type of the kit's `component` and `base` modules, by upstream
module: the Go type, the upstream type, how to make one, and the builders it has beyond the
styling every element shares. For a state type: how it is made (`gk.NewX(cx, ..)….Build()` in
`init`), the element that shows it, its reads, writes and events. The full list of each type's
methods, with parameter types, is in its generated file and in `go doc`.

<!-- index:begin (written by gen/gk_apidoc.py from the generated Go packages; do not edit) -->

254 element types, 144 kit state types, and 97 value types across 76 upstream modules. Shared styling builders (`.Bg`, `.W`, `.P`, `.BorderColor`, and style tokens) are omitted from per-type rows.

### component

- **`gk.IconEl`** (Icon, element): `gk.IconDefault()` or `gk.IconEmpty()` or `gk.Icon(icon)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.Data`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 15 more
- **`kit.TitleBarEl`** (TitleBar, element): `kit.TitleBar()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 9 more
- **`kit.WindowBorderEl`** (WindowBorder, element): `kit.KitWindowBorder()` or `kit.WindowBorder()` or `kit.WindowBorderDefault()`; `.ResizeHitSize`, `.ShadowSize`
- **`gk.Theme`** (Theme, kit state): `gk.KitTheme(cx)` or `gk.ThemeGlobal(cx)` or `gk.ThemeGlobalMut(cx)`; reads `.ApplySemanticConfig`, `.ApplySemanticConfigStr`, `.ColorTokens`, `.Colors`, `.CursorBlink`, `.DarkTheme`, `.FocusRing`, `.FontFamily` and 31 more; writes `.ApplyConfig`, `.ApplySemanticTokens`, `.SetColors`, `.SetCursorBlink`, `.SetDarkTheme`, `.SetFocusRing`, `.SetFontFamily`, `.SetFontSize` and 14 more
- **`gk.ThemeRegistry`** (ThemeRegistry, kit state): `gk.ThemeRegistryGlobal(cx)` or `gk.ThemeRegistryGlobalMut(cx)`; reads `.DefaultDarkTheme`, `.DefaultLightTheme`, `.DefaultThemes`, `.LoadThemesFromStr`, `.SortedThemes`, `.Themes`

### component::accordion

- **`kit.AccordionEl`** (Accordion, element): `kit.Accordion(id)`; `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.Bordered`, `.FlexRowDir`, `.InsetE` and 16 more
- **`kit.AccordionItemEl`** (AccordionItem, element): `kit.AccordionItem()`; `.Title`, `.Icon`, `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.ContentStyle` and 17 more

### component::alert

- **`kit.AlertEl`** (Alert, element): `kit.Alert(id, message)` or `kit.AlertError(id, message)` or `kit.AlertInfo(id, message)` (and 2 more); `.Title`, `.Icon`, `.Small`, `.Large`, `.Banner`, `.BorderE`, `.BorderS`, `.FlexRowDir` and 18 more

### component::attachment

- **`kit.AttachmentActionsEl`** (AttachmentActions, element): `kit.AttachmentActions()` or `kit.AttachmentActionsDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.AttachmentContentEl`** (AttachmentContent, element): `kit.AttachmentContent()` or `kit.AttachmentContentDefault()`; `.Title`, `.Description`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more
- **`kit.AttachmentDescriptionEl`** (AttachmentDescription, element): `kit.AttachmentDescription(text)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more
- **`kit.AttachmentEl`** (Attachment, element): `kit.Attachment()` or `kit.AttachmentDefault()`; `.OnClick`, `.Tooltip`, `.Content`, `.Small`, `.Large`, `.Actions`, `.Axis`, `.BorderE` and 23 more
- **`kit.AttachmentGroupEl`** (AttachmentGroup, element): `kit.AttachmentGroup(id)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more
- **`kit.AttachmentMediaEl`** (AttachmentMedia, element): `kit.AttachmentMedia()` or `kit.AttachmentMediaDefault()`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 13 more
- **`kit.AttachmentTitleEl`** (AttachmentTitle, element): `kit.AttachmentTitle(text)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 9 more

### component::avatar

- **`kit.AvatarEl`** (Avatar, element): `kit.Avatar()`; `.Placeholder`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 14 more
- **`kit.AvatarGroupEl`** (AvatarGroup, element): `kit.AvatarGroup()`; `.Child`, `.Children`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.Ellipsis`, `.FlexRowDir` and 15 more

### component::badge

- **`kit.BadgeEl`** (Badge, element): `kit.Badge()`; `.Icon`, `.Small`, `.Large`, `.Color`, `.Count`, `.Dot`, `.Max`, `.WithSize` and 1 more

### component::breadcrumb

- **`kit.BreadcrumbEl`** (Breadcrumb, element): `kit.Breadcrumb()`; `.Child`, `.Children`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more
- **`kit.BreadcrumbItemEl`** (BreadcrumbItem, element): `kit.BreadcrumbItem(label)`; `.OnClick`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more

### component::bubble

- **`kit.BubbleContentEl`** (BubbleContent, element): `kit.BubbleContent()` or `kit.BubbleContentDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.BubbleEl`** (Bubble, element): `kit.Bubble()` or `kit.BubbleDefault()`; `.Content`, `.Alignment`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.BubbleGroupEl`** (BubbleGroup, element): `kit.BubbleGroup()` or `kit.BubbleGroupDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.BubbleReactionsEl`** (BubbleReactions, element): `kit.BubbleReactions()` or `kit.BubbleReactionsDefault()`; `.Action`, `.Alignment`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 10 more

### component::button

- **`kit.ButtonEl`** (Button, element): `kit.Button(id)`; `.Label`, `.Selected`, `.Icon`, `.OnClick`, `.Tooltip`, `.Primary`, `.Ghost`, `.Outline` and 37 more
- **`kit.ButtonGroupEl`** (ButtonGroup, element): `kit.ButtonGroup(id)`; `.Child`, `.Children`, `.OnClick`, `.Primary`, `.Ghost`, `.Outline`, `.Small`, `.Large` and 29 more
- **`kit.DropdownButtonEl`** (DropdownButton, element): `kit.DropdownButton(id)`; `.Selected`, `.Primary`, `.Ghost`, `.Outline`, `.Small`, `.Large`, `.Disabled`, `.BorderE` and 30 more
- **`kit.ToggleEl`** (Toggle, element): `kit.Toggle(id)`; `.Label`, `.Checked`, `.Icon`, `.OnClick`, `.Tooltip`, `.Ghost`, `.Outline`, `.Small` and 20 more
- **`kit.ToggleGroupEl`** (ToggleGroup, element): `kit.ToggleGroup(id)`; `.Child`, `.Children`, `.OnClick`, `.Ghost`, `.Outline`, `.Small`, `.Large`, `.Disabled` and 20 more
- **`gk.ButtonCustomVariant`** (ButtonCustomVariant, value): `kit.NewButtonCustomVariant()`; `.Active`, `.Color`, `.Foreground`, `.Hover`, `.Shadow`

### component::calendar

- **`kit.CalendarEl`** (Calendar, element): `kit.Calendar(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FirstDayOfWeek`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 13 more

### component::carousel

- **`kit.CarouselContentEl`** (CarouselContent, element): `kit.CarouselContent(state)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more
- **`kit.CarouselEl`** (Carousel, element): `kit.Carousel(id, state)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.CarouselItemEl`** (CarouselItem, element): `kit.CarouselItem(id, index, state)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.CarouselNextEl`** (CarouselNext, element): `kit.CarouselNext(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.CarouselPaginationEl`** (CarouselPagination, element): `kit.CarouselPagination()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.CarouselPaginationItemEl`** (CarouselPaginationItem, element): `kit.CarouselPaginationItem(id, index, state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.CarouselPreviousEl`** (CarouselPrevious, element): `kit.CarouselPrevious(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.CarouselState`** (CarouselState, kit state): `kit.NewCarouselState(cx, itemCount)….Build()`; shown by `kit.Carousel(state)`, `kit.CarouselContent(state)`, `kit.CarouselItem(state)`, `kit.CarouselNext(state)`, `kit.CarouselPaginationItem(state)`, `kit.CarouselPrevious(state)`; built with `.WithAxis`, `.WithLooping`, `.WithSelectedIndex`; reads `.Axis`, `.HasNext`, `.HasPrevious`, `.IsLooping`, `.ItemCount`, `.SelectFirst`, `.SelectIndex`, `.SelectLast` and 3 more; writes `.SetAxis`, `.SetItemCount`, `.SetLooping`, `.SetSelectedIndex`; events `.OnCarouselEvent(cx, f)`

### component::chart

- **`chart.AreaChartEl`** (AreaChart<Datum, element): `chart.AreaChart(data)`; `.Appear`, `.AppearKey`, `.Fill`, `.Grid`, `.GridColumns`, `.GridDashed`, `.Interactive`, `.Linear` and 21 more
- **`chart.BarChartEl`** (BarChart<Datum, element): `chart.BarChart(data)`; `.Label`, `.Value`, `.Alignment`, `.Appear`, `.AppearKey`, `.Band`, `.BandCount`, `.BandTickCount` and 21 more
- **`chart.CandlestickChartEl`** (CandlestickChart<Datum, element): `chart.CandlestickChart(data)`; `.Appear`, `.AppearKey`, `.Bearish`, `.BodyWidthRatio`, `.Bullish`, `.Close`, `.Grid`, `.High` and 11 more
- **`chart.LineChartEl`** (LineChart<Datum, element): `chart.LineChart(data)`; `.Appear`, `.AppearKey`, `.Dot`, `.Grid`, `.GridColumns`, `.GridDashed`, `.Interactive`, `.Linear` and 21 more
- **`chart.PieChartEl`** (PieChart<Datum>, element): `chart.PieChart(data)`; `.Label`, `.Value`, `.Appear`, `.AppearKey`, `.Color`, `.InnerRadius`, `.InnerRadiusFn`, `.InnerRadiusRatio` and 10 more
- **`chart.RadarChartEl`** (RadarChart<Datum, element): `chart.RadarChart(data)`; `.Label`, `.Value`, `.Appear`, `.AppearKey`, `.Dot`, `.Fill`, `.Grid`, `.GridLevels` and 11 more
- **`chart.SankeyChartEl`** (SankeyChart<Datum>, element): `chart.SankeyChart(nodes, links)`; `.Appear`, `.AppearKey`, `.Interactive`, `.Iterations`, `.LabelGap`, `.Labels`, `.LinkOpacity`, `.MinLinkWidth` and 10 more
- **`chart.SankeyLabel`** (SankeyLabel, value): `chart.NewSankeyLabel(text)`; `.Color`, `.FontSize`

### component::checkbox

- **`kit.CheckboxEl`** (Checkbox, element): `kit.Checkbox(id)`; `.Label`, `.Checked`, `.Selected`, `.OnClick`, `.OnChange`, `.Tooltip`, `.Small`, `.Large` and 21 more

### component::clipboard

- **`kit.ClipboardEl`** (Clipboard, element): `kit.Clipboard(id)`; `.Label`, `.Value`, `.Tooltip`, `.Small`, `.Large`, `.OnCopied`, `.ValueFn`, `.WithSize` and 1 more

### component::collapsible

- **`kit.CollapsibleEl`** (Collapsible, element): `kit.Collapsible()`; `.Content`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more

### component::color_picker

- **`kit.ColorPickerEl`** (ColorPicker, element): `kit.ColorPicker(state)`; `.Label`, `.Icon`, `.Small`, `.Large`, `.Anchor`, `.BorderE`, `.BorderS`, `.FeaturedColors` and 15 more
- **`kit.ColorSelectEl`** (ColorSelect, element): `kit.ColorSelect(state)`; `.Placeholder`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FeaturedColors`, `.FlexRowDir`, `.InsetE` and 13 more

### component::combobox

- **`table.CaretEl`** (Caret, element): `table.Caret(size)`
- **`table.ComboboxEl`** (Combobox<GkSearchDelegate>, element): `table.Combobox(state)`; `.Icon`, `.Placeholder`, `.Footer`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.BorderE` and 24 more
- **`table.ComboboxState`** (ComboboxState<GkSearchDelegate>, kit state): `table.NewComboboxState(cx, gk.NewSearchSource(cx, items), selectedIndices)….Build()`; shown by `table.Combobox(state)`; built with `.Multiple`, `.Searchable`; reads `.AddSelectedIndex`, `.Query`, `.RemoveSelectedIndex`, `.SelectedValue`, `.SelectedValues`, `.Selection`; writes `.ClearSelection`, `.Focus`, `.SetItems`, `.SetQuery`, `.SetSelectedIndices`, `.SetSelectedValues`; events `.OnComboboxEvent(cx, f)`

### component::command

- **`table.CommandEl`** (Command, element): `table.Command(state)`; `.Placeholder`, `.OnSelect`, `.Footer`, `.BorderE`, `.BorderS`, `.Bordered`, `.Empty`, `.Filterable` and 23 more
- **`table.CommandState`** (CommandState, kit state): `table.NewCommandState(cx)`; shown by `table.Command(state)`; reads `.IsLoading`, `.MatchedCount`, `.Query`, `.SelectedIndex`; writes `.Focus`, `.SetLoading`, `.SetQuery`, `.SetSelectedIndex`
- **`table.CommandGroup`** (CommandGroup, value): `table.NewCommandGroup()`; `.Label`, `.Item`, `.Items`
- **`table.CommandItem`** (CommandItem, value): `table.NewCommandItem()` or `table.NewCommandItemDefault()`; `.Label`, `.Child`, `.Checked`, `.Icon`, `.Disabled`, `.Action`, `.Keywords`

### component::date_picker

- **`kit.DatePickerEl`** (DatePicker, element): `kit.DatePicker(state)`; `.Placeholder`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.BorderE`, `.BorderS`, `.Cleanable` and 17 more
- **`kit.DatePickerState`** (DatePickerState, kit state): `kit.NewDatePickerState(cx)….Build()` or `kit.NewDatePickerStateRange(cx)….Build()`; shown by `kit.DatePicker(state)`; built with `.DateFormat`, `.DefaultTime`, `.DisabledMatcher`, `.FirstDayOfWeek`, `.HourCycle`, `.NumberOfMonths`, `.TimePrecision`; reads `.Date`, `.DateTime`; writes `.SetDate`, `.SetDateTime`, `.SetYearRange`; events `.OnDatePickerEvent(cx, f)`
- **`kit.DateRangePreset`** (DateRangePreset, value): `kit.NewDateRangePresetDateTime(label, value)` or `kit.NewDateRangePresetSingle(label, date)` or `kit.NewDateRangePresetRange(label, start, end)`

### component::description_list

- **`kit.DescriptionListEl`** (DescriptionList, element): `kit.DescriptionList()` or `kit.DescriptionListHorizontal()` or `kit.DescriptionListVertical()`; `.Child`, `.Children`, `.Small`, `.Large`, `.Bordered`, `.Columns`, `.Item`, `.LabelWidth` and 4 more

### component::dialog

- **`overlay.AlertDialogEl`** (AlertDialog, element): `overlay.AlertDialog()`; `.Title`, `.Icon`, `.Description`, `.Content`, `.Footer`, `.BorderE`, `.BorderS`, `.ButtonProps` and 30 more
- **`overlay.DialogActionEl`** (DialogAction, element): `overlay.DialogAction()`
- **`overlay.DialogCloseEl`** (DialogClose, element): `overlay.DialogClose()`; `.Trigger`
- **`overlay.DialogContentEl`** (DialogContent, element): `overlay.DialogContent()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.DialogDescriptionEl`** (DialogDescription, element): `overlay.DialogDescription()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.DialogEl`** (Dialog, element): `overlay.Dialog()`; `.Title`, `.Content`, `.Footer`, `.BorderE`, `.BorderS`, `.ButtonProps`, `.Centered`, `.CloseButton` and 24 more
- **`overlay.DialogFooterEl`** (DialogFooter, element): `overlay.DialogFooter()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.DialogHeaderEl`** (DialogHeader, element): `overlay.DialogHeader()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.DialogTitleEl`** (DialogTitle, element): `overlay.DialogTitle()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.DialogButtonProps`** (DialogButtonProps, value): `overlay.NewDialogButtonPropsDefault()`; `.CancelText`, `.CancelVariant`, `.OkText`, `.OkVariant`, `.OnCancel`, `.OnOk`, `.ShowCancel`

### component::diff

- **`editor.DiffEl`** (Diff, element): `editor.Diff(state)`; `.AnnotationContent`, `.Annotations`, `.BorderE`, `.BorderS`, `.ChangeBackground`, `.ChangeIndicator`, `.Decorations`, `.FlexRowDir` and 37 more
- **`editor.DiffAnnotation`** (DiffAnnotation, kit state): `editor.NewDiffAnnotationFile(cx, id, path)` or `editor.NewDiffAnnotationLine(cx, id, position)`; reads `.ID`, `.Path`, `.Position`
- **`editor.DiffConflict`** (DiffConflict, kit state): handed out by the kit (no constructor); reads `.BaseLabel`, `.BaseLines`, `.CurrentLabel`, `.CurrentLines`, `.IncomingLabel`, `.IncomingLines`, `.Lines`
- **`editor.DiffFileRead`** (DiffFile, kit state): handed out by the kit (no constructor); reads `.Additions`, `.Conflicts`, `.Deletions`, `.ExtendedHeaders`, `.HasChanges`, `.IsBinary`, `.IsTruncated`, `.Language` and 4 more
- **`editor.DiffGutterMarkerRead`** (DiffGutterMarker, kit state): handed out by the kit (no constructor); reads `.Label`, `.ID`, `.Position`
- **`editor.DiffLinePosition`** (DiffLinePosition, kit state): `editor.NewDiffLinePosition(cx, path, side, line)`; shown by `editor.NewDiffGutterMarker(state)`, `editor.NewDiffSpanHighlight(state)`; reads `.Line`, `.Path`, `.Side`
- **`editor.DiffLineRangeRead`** (DiffLineRange, kit state): handed out by the kit (no constructor); reads `.End`, `.EndSide`, `.Path`, `.Side`, `.Start`
- **`editor.DiffParseError`** (DiffParseError, kit state): handed out by the kit (no constructor); reads `.Line`, `.Message`
- **`editor.DiffState`** (DiffState, kit state): `editor.NewDiffState(cx, files)….Build()`; shown by `editor.Diff(state)`; built with `.WithContextLines`, `.WithExpansionLines`, `.WithInlineMaxLineLength`, `.WithInlineUnit`, `.WithMinCollapsedLines`, `.WithMode`, `.WithSyntaxHighlighter`, `.WithSyntaxMaxLineLength`; reads `.ConflictResolution`, `.ContextLines`, `.ExpansionLines`, `.Files`, `.HasLine`, `.InlineMaxLineLength`, `.InlineUnit`, `.IsFileCollapsed` and 6 more; writes `.ClearSyntaxHighlighter`, `.CollapseUnchanged`, `.ExpandUnchanged`, `.NextChange`, `.PreviousChange`, `.ResolveConflict`, `.ScrollToFile`, `.ScrollToLine` and 13 more; events `.OnDiffEvent(cx, f)`
- **`editor.DiffTextPosition`** (DiffTextPosition, kit state): handed out by the kit (no constructor); reads `.Text`, `.Offset`, `.Position`
- **`editor.DiffDecoration`** (DiffDecoration, value): `editor.NewDiffDecoration(range_)`; `.WithBackground`
- **`editor.DiffFile`** (DiffFile, value): `editor.NewDiffFileExcerpts(path, parts)` or `editor.NewDiffFileParseAt(patch, index)` or `editor.NewDiffFileUnchanged(path, text)` (and 1 more); `.WithLanguage`
- **`editor.DiffGutterMarker`** (DiffGutterMarker, value): `editor.NewDiffGutterMarker(id, position)`; `.WithLabel`
- **`editor.DiffLineRange`** (DiffLineRange, value): `editor.NewDiffLineRange(path, side, start, end)`; `.WithEndSide`
- **`editor.DiffParseOptions`** (DiffParseOptions, value): `editor.NewDiffParseOptions()` or `editor.NewDiffParseOptionsDefault()`; `.TruncatedHunks`
- **`editor.DiffSpanHighlight`** (DiffSpanHighlight, value): `editor.NewDiffSpanHighlight(position, span)`; `.WithBackground`, `.WithForeground`, `.WithUnderline`

### component::dock

- **`dock.PanelHandle`** (PanelHandle, kit state): `dock.NewPanelHandle(cx, panel)` or `dock.NewPanelHandleFromView(cx, panel)`; reads `.Panel`

### component::empty

- **`kit.EmptyContentEl`** (EmptyContent, element): `kit.EmptyContent()` or `kit.EmptyContentDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.EmptyDescriptionEl`** (EmptyDescription, element): `kit.EmptyDescription()` or `kit.EmptyDescriptionDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.EmptyEl`** (Empty, element): `kit.Empty()` or `kit.EmptyDefault()`; `.Content`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Header`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more
- **`kit.EmptyHeaderEl`** (EmptyHeader, element): `kit.EmptyHeader()` or `kit.EmptyHeaderDefault()`; `.Title`, `.Description`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 10 more
- **`kit.EmptyMediaEl`** (EmptyMedia, element): `kit.EmptyMedia()` or `kit.EmptyMediaDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more
- **`kit.EmptyTitleEl`** (EmptyTitle, element): `kit.EmptyTitle()` or `kit.EmptyTitleDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more

### component::form

- **`kit.FieldEl`** (Field, element): `kit.Field()` or `kit.KitField()`; `.Label`, `.Description`, `.BorderE`, `.BorderS`, `.DescriptionFn`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 17 more
- **`kit.FormEl`** (Form, element): `kit.Form()` or `kit.FormDefault()` or `kit.FormHorizontal()` (and 3 more); `.Child`, `.Children`, `.Footer`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.Columns` and 19 more

### component::group_box

- **`kit.GroupBoxEl`** (GroupBox, element): `kit.GroupBox()`; `.Title`, `.Footer`, `.Outline`, `.BorderE`, `.BorderS`, `.ContentStyle`, `.Fill`, `.FlexRowDir` and 15 more

### component::highlighter

- **`editor.StatusColors`** (StatusColors, kit state): `editor.NewStatusColorsDefault(cx)`; reads `.Created`, `.Deleted`, `.Error`, `.ErrorBackground`, `.ErrorBorder`, `.Hint`, `.HintBackground`, `.HintBorder` and 10 more
- **`editor.SyntaxColors`** (SyntaxColors, kit state): `editor.NewSyntaxColorsDefault(cx)`; reads `.Style`, `.StyleForIndex`
- **`editor.SyntaxHighlighter`** (SyntaxHighlighter, kit state): `editor.NewSyntaxHighlighter(cx, language)`; reads `.Text`, `.Language`

### component::hover_card

- **`overlay.HoverCardEl`** (HoverCard, element): `overlay.HoverCard(id)`; `.Content`, `.Anchor`, `.Appearance`, `.BorderE`, `.BorderS`, `.CloseDelay`, `.FlexRowDir`, `.InsetE` and 15 more

### component::input

- **`editor.EditorEl`** (Editor, element): `editor.Editor(state)`; `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.BorderE`, `.BorderS`, `.Bordered`, `.FlexRowDir` and 17 more
- **`editor.InputEl`** (Input, element): `editor.Input(state)`; `.Selected`, `.Prefix`, `.Suffix`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.BorderE` and 30 more
- **`editor.InputGroupAddonEl`** (InputGroupAddon, element): `editor.InputGroupAddon(id)`; `.Align`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`editor.InputGroupButtonEl`** (InputGroupButton, element): `editor.InputGroupButton(id)`; `.Label`, `.Selected`, `.Icon`, `.OnClick`, `.Tooltip`, `.Primary`, `.Ghost`, `.Outline` and 33 more
- **`editor.InputGroupEl`** (InputGroup, element): `editor.InputGroup(id)`; `.Small`, `.Large`, `.Disabled`, `.Addon`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Input` and 16 more
- **`editor.InputGroupTextEl`** (InputGroupText, element): `editor.InputGroupText()` or `editor.InputGroupTextDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`editor.InputTokenEl`** (InputToken, element): `editor.InputToken(context)`; `.Icon`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`editor.NumberInputEl`** (NumberInput, element): `editor.NumberInput(state)`; `.Placeholder`, `.Prefix`, `.Suffix`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.BorderE` and 17 more
- **`editor.OtpInputEl`** (OtpInput, element): `editor.OtpInput(state)`; `.Small`, `.Large`, `.Disabled`, `.Groups`, `.WithSize`, `.Xsmall`
- **`editor.TextareaEl`** (Textarea, element): `editor.Textarea(state)`; `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.BorderE`, `.BorderS`, `.Bordered`, `.FlexRowDir` and 19 more
- **`editor.GutterStyle`** (GutterStyle, value): `editor.NewGutterStyleDefault()`; `.WithActiveLineNumberColor`, `.WithGap`, `.WithLineNumberColor`, `.WithLineNumberFontFamily`, `.WithLineNumberFontSize`

### component::kbd

- **`kit.KbdEl`** (Kbd, element): `kit.Kbd(stroke)`; `.Outline`, `.Appearance`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more

### component::label

- **`kit.LabelEl`** (Label, element): `kit.Label(label)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Highlights`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more

### component::link

- **`kit.LinkEl`** (Link, element): `kit.Link(id)`; `.OnClick`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Href`, `.InsetE`, `.InsetS` and 10 more

### component::list

- **`table.ListEl`** (List<GkListDelegate>, element): `table.List(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 14 more
- **`table.ListItemEl`** (ListItem, element): `table.ListItem(id)`; `.Selected`, `.OnClick`, `.Suffix`, `.Disabled`, `.BorderE`, `.BorderS`, `.CheckIcon`, `.Confirmed` and 18 more
- **`table.ListState`** (ListState<GkListDelegate>, kit state): `table.NewListState(cx, table.NewListSource(cx, count, row))….Build()`; shown by `table.List(state)`; built with `.Searchable`, `.Selectable`; reads `.Delegate`, `.DelegateMut`, `.RightClickedIndex`, `.ScrollHandle`, `.SelectedIndex`; writes `.Focus`, `.ScrollToItem`, `.ScrollToSelectedItem`, `.SetItemToMeasureIndex`, `.SetQuery`, `.SetRightClickedIndex`, `.SetSearchable`, `.SetSelectable` and 1 more; events `.OnListEvent(cx, f)`

### component::marker

- **`kit.MarkerContentEl`** (MarkerContent, element): `kit.MarkerContent()` or `kit.MarkerContentDefault()`; `.Text`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`kit.MarkerEl`** (Marker, element): `kit.Marker()` or `kit.MarkerDefault()`; `.Icon`, `.Content`, `.Alignment`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 15 more
- **`kit.MarkerIconEl`** (MarkerIcon, element): `kit.MarkerIcon()` or `kit.MarkerIconDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more

### component::menu

- **`kit.AppMenuBar`** (AppMenuBar, kit state): handed out by the kit (no constructor); writes `.Reload`
- **`kit.PopupMenu`** (PopupMenu, value): `kit.NewPopupMenu()`; `.Label`, `.ActionContext`, `.CheckSide`, `.ExternalLinkIcon`, `.Item`, `.Link`, `.LinkWithDisabled`, `.LinkWithIcon` and 18 more
- **`kit.PopupMenuItem`** (PopupMenuItem, value): `kit.NewPopupMenuItemSeparator()` or `kit.NewPopupMenuItem(label)` or `kit.NewPopupMenuItemElement(builder)` (and 2 more); `.Checked`, `.Icon`, `.OnClick`, `.Disabled`, `.Action`, `.OnClickDo`

### component::message

- **`kit.MessageAvatarEl`** (MessageAvatar, element): `kit.MessageAvatar()` or `kit.MessageAvatarDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.MessageContentEl`** (MessageContent, element): `kit.MessageContent()` or `kit.MessageContentDefault()`; `.BorderE`, `.BorderS`, `.Bubble`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`kit.MessageEl`** (Message, element): `kit.Message()` or `kit.MessageDefault()`; `.Content`, `.Footer`, `.Alignment`, `.Avatar`, `.AvatarSlot`, `.BorderE`, `.BorderS`, `.FlexRowDir` and 14 more
- **`kit.MessageFooterEl`** (MessageFooter, element): `kit.MessageFooter()` or `kit.MessageFooterDefault()`; `.BorderE`, `.BorderS`, `.ContentInset`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`kit.MessageGroupEl`** (MessageGroup, element): `kit.MessageGroup()` or `kit.MessageGroupDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`kit.MessageHeaderEl`** (MessageHeader, element): `kit.MessageHeader()` or `kit.MessageHeaderDefault()`; `.BorderE`, `.BorderS`, `.ContentInset`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more

### component::message_scroller

- **`kit.MessageScrollerEl`** (MessageScroller, element): `kit.MessageScroller(id, state, renderer)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JumpButton`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 16 more
- **`kit.MessageScrollerState`** (MessageScrollerState, kit state): `kit.NewMessageScrollerState(cx, itemCount)`; shown by `kit.MessageScroller(state)`; reads `.Append`, `.IsFollowingTail`, `.IsScrolledUp`, `.ItemCount`, `.LogicalScrollTop`, `.Prepend`, `.RemeasureItems`, `.ScrollToItem` and 2 more; writes `.Remeasure`, `.Reset`, `.ScrollToEnd`

### component::native_menu

- **`editor.NativeMenu`** (NativeMenu, value): `editor.NewNativeMenu()` or `editor.NewNativeMenuDefault()`; `.Menu`, `.MenuWithCheck`, `.MenuWithDisabled`, `.MenuWithIcon`, `.MenuWithIconAndDisabled`, `.MenuWithIconDisabled`, `.Separator`, `.Submenu`

### component::notification

- **`overlay.Notification`** (Notification, kit state): `overlay.NewNotification(cx)….Build()` or `overlay.NewNotificationError(cx, message)….Build()` or `overlay.NewNotificationInfo(cx, message)….Build()` (and 2 more); built with `.Title`, `.Icon`, `.OnClick`, `.Content`, `.Action`, `.Autohide`, `.Delivery`, `.InAppAndSystem` and 5 more; writes `.Dismiss`
- **`overlay.NotificationList`** (NotificationList, kit state): `overlay.NewNotificationList(cx)`; reads `.Notifications`; writes `.Clear`, `.Push`

### component::pagination

- **`kit.PaginationEl`** (Pagination, element): `kit.Pagination(id)`; `.OnClick`, `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.Compact`, `.CurrentPage` and 17 more

### component::plot

- **`chart.CrossLineEl`** (CrossLine, element): `chart.CrossLine(point)`; `.Band`, `.Both`, `.HSpan`, `.Height`, `.Horizontal`, `.Span`, `.Width`
- **`chart.DotEl`** (Dot, element): `chart.Dot(point)`; `.Fill`, `.Halo`, `.Stroke`
- **`chart.TooltipEl`** (Tooltip, element): `chart.Tooltip(cursor, within)`; `.Title`, `.Appearance`, `.BorderE`, `.BorderS`, `.CrossLine`, `.Dots`, `.FlexRowDir`, `.Glide` and 16 more

### component::popover

- **`overlay.PopoverEl`** (Popover, element): `overlay.Popover(id)`; `.Content`, `.Anchor`, `.Appearance`, `.Arrow`, `.BorderE`, `.BorderS`, `.DefaultOpen`, `.FlexRowDir` and 19 more

### component::progress

- **`kit.ProgressCircleEl`** (ProgressCircle, element): `kit.ProgressCircle(id)`; `.Value`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.Color`, `.FlexRowDir`, `.InsetE` and 14 more
- **`kit.ProgressEl`** (Progress, element): `kit.Progress(id)`; `.Value`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.Color`, `.FlexRowDir`, `.InsetE` and 14 more

### component::questionnaire

- **`kit.QuestionnaireActionsEl`** (QuestionnaireActions, element): `kit.QuestionnaireActions(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireChoiceDescriptionEl`** (QuestionnaireChoiceDescription, element): `kit.QuestionnaireChoiceDescription()` or `kit.QuestionnaireChoiceDescriptionDefault()`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireChoiceEl`** (QuestionnaireChoice, element): `kit.QuestionnaireChoice(state, item, value)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.ContentStyle`, `.FlexRowDir`, `.IndicatorStyle`, `.InsetE` and 16 more
- **`kit.QuestionnaireChoicesEl`** (QuestionnaireChoices, element): `kit.QuestionnaireChoices(state, item)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireDescriptionEl`** (QuestionnaireDescription, element): `kit.QuestionnaireDescription(state, item)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireEl`** (Questionnaire, element): `kit.Questionnaire(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireErrorEl`** (QuestionnaireError, element): `kit.QuestionnaireError(state, item)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireInputEl`** (QuestionnaireInput, element): `kit.QuestionnaireInput(state, item)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireItemEl`** (QuestionnaireItem, element): `kit.QuestionnaireItem(state, item)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireNextEl`** (QuestionnaireNext, element): `kit.QuestionnaireNext(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnairePreviousEl`** (QuestionnairePrevious, element): `kit.QuestionnairePrevious(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireProgressEl`** (QuestionnaireProgress, element): `kit.QuestionnaireProgress(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireSkipEl`** (QuestionnaireSkip, element): `kit.QuestionnaireSkip(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireSubmitEl`** (QuestionnaireSubmit, element): `kit.QuestionnaireSubmit(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.QuestionnaireTitleEl`** (QuestionnaireTitle, element): `kit.QuestionnaireTitle(state, item)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more

### component::radio

- **`kit.RadioEl`** (Radio, element): `kit.Radio(id)`; `.Label`, `.Checked`, `.OnClick`, `.OnChange`, `.Tooltip`, `.Small`, `.Large`, `.Disabled` and 17 more
- **`kit.RadioGroupEl`** (RadioGroup, element): `kit.RadioGroup(id)` or `kit.RadioGroupHorizontal(id)` or `kit.RadioGroupVertical(id)`; `.Child`, `.Children`, `.OnClick`, `.OnChange`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir` and 14 more

### component::rating

- **`kit.RatingEl`** (Rating, element): `kit.Rating(id)`; `.Value`, `.OnClick`, `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.Color` and 16 more

### component::searchable_list

- **`table.SearchableListItemElementEl`** (SearchableListItemElement, element): `table.SearchableListItemElement(ix)`; `.Checked`, `.Selected`, `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.CheckIcon` and 17 more

### component::select

- **`table.SelectEl`** (Select<GkSearchDelegate>, element): `table.Select(state)`; `.Icon`, `.Placeholder`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.BorderE`, `.BorderS` and 23 more
- **`table.SelectState`** (SelectState<GkSearchDelegate>, kit state): `table.NewSelectState(cx, gk.NewSearchSource(cx, items), selectedIndex)….Build()`; shown by `table.Select(state)`; built with `.Searchable`; reads `.SelectedIndex`, `.SelectedValue`; writes `.Focus`, `.SetItems`, `.SetSelectedIndex`, `.SetSelectedValue`; events `.OnSelectEvent(cx, f)`

### component::separator

- **`kit.SeparatorEl`** (Separator, element): `kit.SeparatorHorizontal()` or `kit.SeparatorHorizontalDashed()` or `kit.SeparatorVertical()` (and 1 more); `.Label`, `.BorderE`, `.BorderS`, `.Color`, `.Dashed`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 10 more

### component::setting

- **`kit.SettingsEl`** (Settings, element): `kit.Settings(id)`; `.Small`, `.Large`, `.DefaultSelectedIndex`, `.HeaderStyle`, `.Page`, `.Pages`, `.SidebarSizeRange`, `.SidebarStyle` and 4 more
- **`kit.SettingFieldBool`** (SettingField<bool>, value): `kit.NewSettingFieldBoolCheckbox(value, setValue)` or `kit.NewSettingFieldBoolSwitch(value, setValue)`; `.BorderE`, `.BorderS`, `.DefaultValue`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more
- **`kit.SettingFieldNumber`** (SettingField<f64>, value): `kit.NewSettingFieldNumberNumberInput(options, value, setValue)`; `.BorderE`, `.BorderS`, `.DefaultValue`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more
- **`kit.SettingFieldText`** (SettingField<SharedString>, value): `kit.NewSettingFieldTextElement(element)` or `kit.NewSettingFieldTextRender(elementRender)` or `kit.NewSettingFieldTextInput(value, setValue)` (and 2 more); `.BorderE`, `.BorderS`, `.DefaultValue`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more
- **`kit.SettingGroup`** (SettingGroup, value): `kit.NewSettingGroup()`; `.Title`, `.Description`, `.Footer`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 14 more
- **`kit.SettingItem`** (SettingItem, value): `kit.NewSettingItemRender(render)` or `kit.NewSettingItem(title, field)`; `.Description`, `.Disabled`, `.Keywords`, `.Layout`, `.OnReset`
- **`kit.SettingPage`** (SettingPage, value): `kit.NewSettingPage(title)`; `.Title`, `.Icon`, `.Description`, `.DefaultOpen`, `.Group`, `.Groups`, `.HeaderStyle`, `.Resettable` and 1 more

### component::sheet

- **`overlay.SheetEl`** (Sheet, element): `overlay.Sheet()`; `.Title`, `.Footer`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 14 more

### component::shimmer

- **`kit.ShimmerTextEl`** (ShimmerText, element): `kit.ShimmerText(text)`; `.BorderE`, `.BorderS`, `.Duration`, `.FlexRowDir`, `.HighlightColor`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 13 more
- **`kit.ShimmerStyle`** (ShimmerStyle, value): `kit.NewShimmerStyle()` or `kit.NewShimmerStyleDefault()`; `.Duration`, `.HighlightColor`, `.Once`, `.Reverse`, `.Spread`

### component::sidebar

- **`kit.GroupedSidebarEl`** (Sidebar<SidebarGroup<SidebarMenu>>, element): `kit.GroupedSidebar(id)`; `.Child`, `.Children`, `.Footer`, `.BorderE`, `.BorderS`, `.Collapsed`, `.Collapsible`, `.FlexRowDir` and 14 more
- **`kit.SidebarEl`** (Sidebar<SidebarMenu>, element): `kit.Sidebar(id)`; `.Child`, `.Children`, `.Footer`, `.BorderE`, `.BorderS`, `.Collapsed`, `.Collapsible`, `.FlexRowDir` and 14 more
- **`kit.SidebarFooterEl`** (SidebarFooter, element): `kit.SidebarFooter()`; `.Selected`, `.BorderE`, `.BorderS`, `.Collapsed`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.SidebarHeaderEl`** (SidebarHeader, element): `kit.SidebarHeader()` or `kit.SidebarHeaderDefault()`; `.Selected`, `.BorderE`, `.BorderS`, `.Collapsed`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`kit.SidebarToggleButtonEl`** (SidebarToggleButton, element): `kit.SidebarToggleButton()`; `.OnClick`, `.Collapsed`, `.Side`
- **`kit.SidebarGroup`** (SidebarGroup<SidebarMenu>, value): `kit.NewSidebarGroup(label)`; `.Child`, `.Children`, `.Suffix`, `.Collapsed`
- **`kit.SidebarMenu`** (SidebarMenu, value): `kit.NewSidebarMenu()`; `.Child`, `.Children`, `.BorderE`, `.BorderS`, `.Collapsed`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 11 more
- **`kit.SidebarMenuItem`** (SidebarMenuItem, value): `kit.NewSidebarMenuItem(label)`; `.Children`, `.Icon`, `.OnClick`, `.Suffix`, `.AccessibilityLabel`, `.Active`, `.BorderE`, `.BorderS` and 22 more

### component::skeleton

- **`kit.SkeletonEl`** (Skeleton, element): `kit.Skeleton()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more

### component::slider

- **`kit.SliderEl`** (Slider, element): `kit.Slider(state)`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Horizontal`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more

### component::speech

- **`kit.SpeechButtonEl`** (SpeechButton, element): `kit.SpeechButton(state)`; `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 13 more
- **`kit.SpeechIndicatorEl`** (SpeechIndicator, element): `kit.SpeechIndicator(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`kit.SpeechWaveformEl`** (SpeechWaveform, element): `kit.SpeechWaveform(state)`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 12 more
- **`kit.AudioFormat`** (AudioFormat, kit state): `kit.NewAudioFormat(cx, sampleRate, channels)`; reads `.Channels`, `.SampleRate`
- **`kit.SpeechState`** (SpeechState, kit state): `kit.NewSpeechState(cx)….Build()`; shown by `kit.SpeechButton(state)`, `kit.SpeechIndicator(state)`, `kit.SpeechWaveform(state)`; built with `.StopTimeout`, `.SystemFallback`; reads `.HasRecognizer`, `.IsAvailable`, `.Levels`, `.Status`, `.Transcript`; writes `.Cancel`, `.Start`, `.Stop`, `.Toggle`; events `.OnSpeechEvent(cx, f)`

### component::spinner

- **`kit.SpinnerEl`** (Spinner, element): `kit.Spinner()`; `.Icon`, `.Small`, `.Large`, `.Color`, `.Ease`, `.WithSize`, `.Xsmall`

### component::status_bar

- **`kit.StatusBarEl`** (StatusBar, element): `kit.StatusBar()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more

### component::stepper

- **`kit.StepperEl`** (Stepper, element): `kit.Stepper(id)`; `.OnClick`, `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE` and 19 more
- **`kit.StepperItemEl`** (StepperItem, element): `kit.StepperItem()`; `.Icon`, `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE` and 13 more

### component::switch

- **`kit.SwitchEl`** (Switch, element): `kit.Switch(id)`; `.Label`, `.Checked`, `.OnClick`, `.OnChange`, `.Tooltip`, `.Small`, `.Large`, `.Disabled` and 18 more

### component::tab

- **`kit.TabBarEl`** (TabBar, element): `kit.TabBar(id)`; `.Child`, `.Children`, `.OnClick`, `.Prefix`, `.Suffix`, `.Outline`, `.Small`, `.Large` and 26 more
- **`kit.TabEl`** (Tab, element): `kit.Tab()` or `kit.TabDefault()`; `.Label`, `.Selected`, `.Icon`, `.OnClick`, `.Prefix`, `.Suffix`, `.Outline`, `.Small` and 26 more

### component::table

- **`table.DataTableEl`** (DataTable<GkTableDelegate>, element): `table.DataTable(state)`; `.Small`, `.Large`, `.Bordered`, `.FixedColsBorder`, `.HeaderRowHeight`, `.ScrollBounce`, `.ScrollbarMode`, `.ScrollbarVisible` and 3 more
- **`table.BodyEl`** (TableBody, element): `table.Body()`; `.Child`, `.Children`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE` and 14 more
- **`table.CaptionEl`** (TableCaption, element): `table.Caption()`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 12 more
- **`table.CellEl`** (TableCell, element): `table.Cell()`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 14 more
- **`table.TableEl`** (Table, element): `table.Table()`; `.Child`, `.Children`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE` and 14 more
- **`table.FooterEl`** (TableFooter, element): `table.Footer()`; `.Child`, `.Children`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE` and 14 more
- **`table.HeadEl`** (TableHead, element): `table.Head()`; `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 14 more
- **`table.HeaderEl`** (TableHeader, element): `table.Header()`; `.Child`, `.Children`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE` and 14 more
- **`table.RowEl`** (TableRow, element): `table.TableRow()`; `.Child`, `.Children`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE` and 14 more
- **`table.State`** (TableState<GkTableDelegate>, kit state): `table.NewState(cx, delegate)….Build()`; shown by `table.DataTable(state)`; built with `.CellSelectable`, `.ColMovable`, `.ColResizable`, `.ColSelectable`, `.LoopSelection`, `.RowHeader`, `.RowSelectable`, `.SelectOnModifierClick` and 1 more; reads `.ColWidth`, `.Delegate`, `.DelegateMut`, `.Dump`, `.DumpRange`, `.FitCol`, `.Headers`, `.RightClickedCell` and 7 more; writes `.ClearSelection`, `.Refresh`, `.RefreshHeaderLayout`, `.ScrollToCol`, `.ScrollToRow`, `.SetRightClickedRow`, `.SetSelectedCell`, `.SetSelectedCol` and 2 more; events `.OnTableEvent(cx, f)`
- **`table.VisibleRange`** (TableVisibleRange, kit state): `table.NewVisibleRangeDefault(cx)`; reads `.Cols`, `.Rows`
- **`table.Column`** (Column, value): `table.NewColumnDefault()` or `table.NewColumn(key, name)`; `.Ascending`, `.Descending`, `.Fixed`, `.FixedLeft`, `.FixedRight`, `.MaxWidth`, `.MinWidth`, `.Movable` and 9 more

### component::tag

- **`kit.TagEl`** (Tag, element): `kit.Tag()` or `kit.TagDanger()` or `kit.TagInfo()` (and 6 more); `.Outline`, `.Small`, `.Large`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 14 more

### component::text

- **`editor.TextViewEl`** (TextView, element): `editor.HTML(source)` or `editor.Markdown(source)` or `editor.TextView(state)` (and 2 more); `.BlockquoteStyle`, `.BorderE`, `.BorderS`, `.CodeBlockActions`, `.FlexRowDir`, `.ImageSource`, `.InlineCodeStyle`, `.InsetE` and 27 more
- **`editor.FrontmatterPlugin`** (FrontmatterPlugin, value): `editor.NewFrontmatterPlugin()` or `editor.NewFrontmatterPluginDefault()`; `.Setup`
- **`editor.TextViewStyle`** (TextViewStyle, value): `editor.NewTextViewStyleDefault()`; `.CodeBlock`, `.HeadingFontSize`, `.InlineCode`, `.ParagraphGap`, `.Table`, `.TableCell`, `.TableHead`

### component::time_field

- **`kit.TimeFieldEl`** (TimeField, element): `kit.TimeField(state)`; `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 13 more

### component::toolbar

- **`kit.ToolbarEl`** (Toolbar, element): `kit.Toolbar(id)`; `.Child`, `.Children`, `.Content`, `.Small`, `.Large`, `.Disabled`, `.BorderE`, `.BorderS` and 16 more
- **`kit.ToolbarGroupEl`** (ToolbarGroup, element): `kit.ToolbarGroup(id)`; `.Label`, `.Child`, `.Children`, `.Content`, `.Small`, `.Large`, `.BorderE`, `.BorderS` and 15 more

### component::tooltip

- **`overlay.KitTooltip`** (Tooltip, value): `overlay.NewKitTooltip(text)` or `overlay.NewKitTooltipElement(builder)`; `.Action`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more

### component::tree

- **`table.TreeEl`** (Tree, element): `table.KitTree(state, renderItem)` or `table.Tree(state, renderItem)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more

### base

- **`base.AccordionHeaderEl`** (AccordionHeader, element): `base.AccordionHeader(trigger)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Level` and 8 more
- **`base.AccordionPanelEl`** (AccordionPanel, element): `base.AccordionPanel()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.KeepMounted` and 9 more
- **`base.AccordionTriggerEl`** (AccordionTrigger, element): `base.AccordionTrigger(id)`; `.OnChange`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 10 more
- **`overlay.AlertDialogActionEl`** (AlertDialogAction, element): `overlay.AlertDialogAction()` or `overlay.AlertDialogActionDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.AlertDialogBackdropEl`** (AlertDialogBackdrop, element): `overlay.AlertDialogBackdrop()` or `overlay.AlertDialogBackdropDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.AlertDialogCancelEl`** (AlertDialogCancel, element): `overlay.AlertDialogCancel()` or `overlay.AlertDialogCancelDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.AlertDialogCloseEl`** (AlertDialogClose, element): `overlay.AlertDialogClose()` or `overlay.AlertDialogCloseDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.AlertDialogDescriptionEl`** (AlertDialogDescription, element): `overlay.AlertDialogDescription()` or `overlay.AlertDialogDescriptionDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.AlertDialogPopupEl`** (AlertDialogPopup, element): `overlay.AlertDialogPopup()` or `overlay.AlertDialogPopupDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.AlertDialogTitleEl`** (AlertDialogTitle, element): `overlay.AlertDialogTitle()` or `overlay.AlertDialogTitleDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.AlertDialogTriggerEl`** (AlertDialogTrigger, element): `overlay.AlertDialogTrigger(trigger)`; `.Handle`, `.OnOpen`
- **`base.AvatarFallbackEl`** (AvatarFallback, element): `base.AvatarFallback()` or `base.AvatarFallbackDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`base.AvatarImageEl`** (AvatarImage, element): `base.AvatarImage(source)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`base.AccordionEl`** (Accordion, element): `base.Accordion(id)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`base.AccordionItemEl`** (AccordionItem, element): `base.AccordionItem()`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Header`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`overlay.BaseAlertDialogEl`** (AlertDialog, element): `overlay.BaseAlertDialog()`; `.Backdrop`, `.BorderE`, `.BorderS`, `.CloseOnEscape`, `.DismissBelowY`, `.FlexRowDir`, `.Handle`, `.InsetE` and 18 more
- **`base.AvatarEl`** (Avatar, element): `base.Avatar()` or `base.AvatarDefault()`; `.BorderE`, `.BorderS`, `.Fallback`, `.FlexRowDir`, `.Image`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more
- **`base.ButtonEl`** (Button, element): `base.Button(id)`; `.Selected`, `.OnClick`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 13 more
- **`base.CalendarEl`** (Calendar, element): `base.Calendar(id, state)`; `.Label`, `.BorderE`, `.BorderS`, `.FirstDayOfWeek`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.Item` and 11 more
- **`base.CheckboxEl`** (Checkbox, element): `base.Checkbox(id)`; `.Checked`, `.OnChange`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Indeterminate`, `.InsetE` and 13 more
- **`base.CollapsibleEl`** (Collapsible, element): `base.Collapsible()` or `base.CollapsibleDefault()`; `.Content`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more
- **`base.ColorPickerEl`** (ColorPicker, element): `base.ColorPicker(id)`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more
- **`table.BaseComboboxEl`** (Combobox, element): `table.BaseCombobox(id)`; `.Disabled`, `.BorderE`, `.BorderS`, `.ContentFocusHandle`, `.FlexRowDir`, `.FocusHandle`, `.InsetE`, `.InsetS` and 14 more
- **`base.DatePickerEl`** (DatePicker, element): `base.DatePicker(id, focusHandle)`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more
- **`overlay.BaseDialogCloseEl`** (DialogClose, element): `overlay.BaseDialogClose()` or `overlay.BaseDialogCloseDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more
- **`overlay.BaseDialogDescriptionEl`** (DialogDescription, element): `overlay.BaseDialogDescription()` or `overlay.BaseDialogDescriptionDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.BaseDialogEl`** (Dialog, element): `overlay.BaseDialog()`; `.Backdrop`, `.BorderE`, `.BorderS`, `.CloseOnBackdropPress`, `.CloseOnEscape`, `.DismissBelowY`, `.FlexRowDir`, `.Handle` and 19 more
- **`overlay.BaseDialogTitleEl`** (DialogTitle, element): `overlay.BaseDialogTitle()` or `overlay.BaseDialogTitleDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.BaseHoverCardEl`** (HoverCard, element): `overlay.BaseHoverCard(id)`; `.Content`, `.Anchor`, `.BorderE`, `.BorderS`, `.CloseDelay`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 13 more
- **`editor.BaseInputEl`** (Input, element): `editor.BaseInput(state)`; `.OnTokenClick`, `.OnTokenHover`, `.Token`
- **`base.LinkEl`** (Link, element): `base.Link(id)`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Href`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 13 more
- **`editor.BaseNumberInputEl`** (NumberInput, element): `editor.BaseNumberInput(state)`; `.Disabled`, `.BorderE`, `.BorderS`, `.ControlsRight`, `.DecrementButton`, `.FlexRowDir`, `.IncrementButton`, `.Input` and 14 more
- **`editor.BaseOtpInputEl`** (OtpInput, element): `editor.BaseOtpInput(state)`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`base.PaginationEl`** (Pagination, element): `base.Pagination(id, state)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.BasePopoverEl`** (Popover, element): `overlay.BasePopover(id)`; `.Content`, `.Anchor`, `.BorderE`, `.BorderS`, `.DefaultOpen`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 17 more
- **`base.ProgressEl`** (Progress, element): `base.Progress(id)`; `.Value`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Indeterminate`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more
- **`base.RadioEl`** (Radio, element): `base.Radio(id)`; `.Checked`, `.OnChange`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 12 more
- **`base.RadioGroupEl`** (RadioGroup, element): `base.RadioGroup(id)`; `.Axis`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`table.BaseSelectEl`** (Select, element): `table.BaseSelect(id)`; `.Disabled`, `.BorderE`, `.BorderS`, `.ContentFocusHandle`, `.FlexRowDir`, `.FocusHandle`, `.InsetE`, `.InsetS` and 14 more
- **`overlay.BaseSheetEl`** (Sheet, element): `overlay.BaseSheet()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 12 more
- **`base.SliderEl`** (Slider, element): `base.Slider(state)`; `.Disabled`, `.Axis`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Horizontal`, `.InsetE`, `.InsetS` and 11 more
- **`base.SwitchEl`** (Switch, element): `base.Switch(id)`; `.Checked`, `.OnChange`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 11 more
- **`base.TabEl`** (Tab, element): `base.Tab(id)`; `.Selected`, `.OnClick`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 12 more
- **`table.BaseTableBodyEl`** (TableBody, element): `table.BaseTableBody(id)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`table.BaseTableCaptionEl`** (TableCaption, element): `table.BaseTableCaption(id)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`table.BaseTableCellEl`** (TableCell, element): `table.BaseTableCell(id, columnIndex)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`table.BaseTableEl`** (Table, element): `table.BaseTable(id)`; `.BorderE`, `.BorderS`, `.ColumnCount`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 9 more
- **`table.BaseTableHeadEl`** (TableHead, element): `table.BaseTableHead(id, columnIndex)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`table.BaseTableHeaderEl`** (TableHeader, element): `table.BaseTableHeader(id)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`table.BaseTableRowEl`** (TableRow, element): `table.BaseTableRow(id, rowIndex)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`editor.BaseTextViewEl`** (TextView, element): `editor.BaseHTML(source)` or `editor.BaseMarkdown(source)` or `editor.BaseTextView(state)` (and 2 more); `.BlockquoteStyle`, `.BorderE`, `.BorderS`, `.CodeBlockActions`, `.CodeBlockHighlighter`, `.FlexRowDir`, `.ImageSource`, `.InlineCodeStyle` and 28 more
- **`editor.BaseTextareaEl`** (Textarea, element): `editor.BaseTextarea(state)`; `.OnTokenClick`, `.OnTokenHover`, `.Token`
- **`base.TimeFieldEl`** (TimeField, element): `base.TimeField(id, state)`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more
- **`base.ToggleEl`** (Toggle, element): `base.Toggle(id)`; `.OnChange`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`base.ToggleGroupEl`** (ToggleGroup, element): `base.ToggleGroup(id)`; `.Axis`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`base.ToolbarEl`** (Toolbar, element): `base.Toolbar(id)`; `.Disabled`, `.Axis`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more
- **`base.ToolbarGroupEl`** (ToolbarGroup, element): `base.ToolbarGroup(id)`; `.Label`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`overlay.BaseTooltipEl`** (Tooltip, element): `overlay.BaseTooltip(id)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`table.BaseTreeEl`** (Tree, element): `table.BaseTree(state)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.Item`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 9 more
- **`base.CalendarItemEl`** (CalendarItem, element): `base.CalendarItemHanded(index)`; `.BorderE`, `.BorderS`, `.ClearChildren`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`base.CheckboxIndicatorEl`** (CheckboxIndicator, element): `base.CheckboxIndicator()` or `base.CheckboxIndicatorDefault()`; `.Checked`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`base.ColorSwatchEl`** (ColorSwatch, element): `base.ColorSwatch(id, color)`; `.Selected`, `.OnClick`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS` and 10 more
- **`overlay.DialogBackdropEl`** (DialogBackdrop, element): `overlay.DialogBackdrop()` or `overlay.DialogBackdropDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.DialogPopupEl`** (DialogPopup, element): `overlay.DialogPopup()` or `overlay.DialogPopupDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.DialogTriggerEl`** (DialogTrigger, element): `overlay.DialogTrigger(trigger)`; `.Handle`, `.OnOpen`
- **`base.GridLayoutEl`** (GridLayout, element): `base.GridLayout(id, state)` or `base.KitGridLayout(id, state)`; `.Placeholder`, `.BorderE`, `.BorderS`, `.DraggingItem`, `.FlexRowDir`, `.FocusedItem`, `.InsetE`, `.InsetS` and 17 more
- **`editor.InputBaseEl`** (InputBase, element): `editor.InputBase(id)`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.Focused`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`motion.RevealEl`** (MotionReveal, element): `motion.Reveal(id, progress, child)`
- **`base.NavPageEl`** (NavPage, element): `base.NavPageHanded(index)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`base.NavStackEl`** (NavStack, element): `base.NavStack(state)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.Item`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 9 more
- **`editor.NumberInputTextEl`** (NumberInputText, element): `editor.NumberInputText()` or `editor.NumberInputTextDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`overlay.PopupEl`** (Popup, element): `overlay.Popup(id, trigger)`; `.Content`, `.Anchor`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 11 more
- **`overlay.PositionerEl`** (Positioner, element): `overlay.PositionerSide(triggerBounds)` or `overlay.PositionerCorner(anchor, position)`; `.Align`, `.Margin`, `.Offset`, `.OnPosition`, `.Placement`, `.Position`
- **`base.ProgressIndicatorEl`** (ProgressIndicator, element): `base.ProgressIndicator()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`base.ProgressTrackEl`** (ProgressTrack, element): `base.ProgressTrack()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`base.ResizablePanelEl`** (ResizablePanel, element): `base.ResizablePanel()`; `.BorderE`, `.BorderS`, `.Fixed`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 10 more
- **`base.ResizablePanelGroupEl`** (ResizablePanelGroup, element): `base.HResizable(id)` or `base.KitHResizable(id)` or `base.KitVResizable(id)` (and 2 more); `.Child`, `.Children`, `.Axis`, `.OnResize`, `.WithHandleAppearance`, `.WithState`
- **`base.ResizeHandleEl`** (ResizeHandle<Drag, element): `base.KitResizeHandle(id, axis)` or `base.ResizeHandle(id, axis)`; `.Inside`, `.LockScrollAxis`, `.OnDoubleClick`, `.WithAppearance`
- **`base.RoundedFrameCoverEl`** (RoundedFrameCover, element): `base.RoundedFrameCoverUniform(radius)`; `.Backdrop`, `.FrameBorder`
- **`motion.ScrollBounceEl`** (ScrollBounce, element): `motion.ScrollBounce(id, handle, child)`; `.Enabled`, `.Motion`, `.ScrollWheel`
- **`base.ScrollableMaskEl`** (ScrollableMask<ScrollHandle>, element): `base.ScrollableMaskVerticalWheel(scrollHandle)` or `base.ScrollableMask(axis, scrollHandle)`; `.Debug`
- **`gk.ScrollbarEl`** (Scrollbar, element): `gk.Scrollbar(scrollHandle)` or `gk.ScrollbarHorizontal(scrollHandle)` or `gk.ScrollbarVertical(scrollHandle)`; `.Axis`, `.Mode`, `.Placement`, `.ScrollSize`, `.Styles`, `.ViewportBounds`, `.ViewportFromLayout`
- **`editor.SelectableTextEl`** (SelectableText, element): `editor.SelectableText(id, text)` or `editor.SelectableTextWithHandle(id, handle, text)`; `.DocumentOrder`, `.Highlights`, `.OnReveal`, `.RevealRange`, `.SelectionColor`, `.TextStyle`
- **`base.SliderIndicatorEl`** (SliderIndicator, element): `base.SliderIndicator(state)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 7 more
- **`base.SliderThumbEl`** (SliderThumb, element): `base.SliderThumb(state)`; `.Disabled`, `.Axis`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 10 more
- **`base.SliderTrackEl`** (SliderTrack, element): `base.SliderTrack(state)`; `.Disabled`, `.Axis`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 9 more
- **`base.SwitchThumbEl`** (SwitchThumb, element): `base.SwitchThumb(checked)`; `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 9 more
- **`base.SwitchTrackEl`** (SwitchTrack, element): `base.SwitchTrack(id)`; `.Checked`, `.Disabled`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 10 more
- **`base.TabsEl`** (Tabs, element): `base.Tabs(id)`; `.Axis`, `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`base.TimeFieldSegmentEl`** (TimeFieldSegment, element): `base.TimeFieldSegmentHanded(index)`; `.BorderE`, `.BorderS`, `.ClearChildren`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart` and 8 more
- **`overlay.ToastEl`** (Toast, element): `overlay.Toast(id)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more
- **`overlay.ToastStackEl`** (ToastStack, element): `overlay.ToastStack(id, state)`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.FocusHandle`, `.InsetE`, `.InsetS`, `.Item`, `.JustifyFlexEnd` and 11 more
- **`overlay.TooltipPositionerEl`** (TooltipPositioner, element): `overlay.TooltipPositioner(triggerBounds)`; `.Offset`, `.Placement`
- **`base.AutoScroll`** (AutoScroll, kit state): `base.NewAutoScrollDefault(cx)`; reads `.Delta`, `.IsActive`; writes `.Stop`
- **`gk.BaseTheme`** (Theme, kit state): `gk.BaseThemeGlobalMut(cx)` or `gk.NewBaseThemeGlobal(cx)`; shown by `editor.NewBaseTextViewStyleFromTheme(state)`; reads `.Appearance`, `.CursorBlink`, `.Plot`, `.Resizable`, `.Scrollbar`, `.Tokens`; writes `.SetAppearance`, `.SetCursorBlink`, `.SetPlot`, `.SetResizable`, `.SetScrollbar`, `.SetTokens`
- **`table.BaseTreeEntry`** (TreeEntry, kit state): `table.NewBaseTreeEntry(cx, item, depth)`; reads `.Depth`, `.IsDisabled`, `.IsExpanded`, `.IsFolder`, `.IsRoot`, `.Item`
- **`base.CalendarState`** (CalendarState, kit state): `base.NewCalendarState(cx)….Build()`; shown by `base.Calendar(state)`, `kit.Calendar(state)`; built with `.DisabledMatcher`, `.YearRange`; reads `.ActivateDate`, `.ApplyDate`, `.CurrentMonth`, `.CurrentYear`, `.Date`, `.Days`, `.DisabledMatcherRef`, `.HasNextYearPage` and 10 more; writes `.NextMonth`, `.PrevMonth`, `.SelectMonth`, `.SelectYear`, `.SetDate`, `.SetDisabledMatcher`, `.SetNumberOfMonths`, `.SetView` and 1 more; events `.OnCalendarEvent(cx, f)`
- **`base.ColorPickerState`** (ColorPickerState, kit state): `base.NewColorPickerState(cx)….Build()`; shown by `kit.ColorPicker(state)`, `kit.ColorSelect(state)`; built with `.DefaultValue`; reads `.Value`, `.ActiveTab`, `.CommitHex`, `.DisplayedColor`, `.HexInput`, `.IsOpen`, `.Preview`, `.PreviewHex` and 2 more; writes `.ClearPreview`, `.ClearValue`, `.PreviewColor`, `.SelectColor`, `.SetActiveTab`, `.SetOpen`, `.SetValue`, `.SyncPendingValue` and 2 more; events `.OnColorPickerEvent(cx, f)`
- **`overlay.DialogHandle`** (DialogHandle, kit state): `overlay.NewDialogHandle(cx, open)`; reads `.IsOpen`; writes `.Close`, `.Open`
- **`motion.Discrete`** (Discrete<f32>, kit state): `motion.NewDiscrete(cx, from, to)`; reads `.Sample`, `.SwitchAt`
- **`gk.GlobalState`** (GlobalState, kit state): `gk.GlobalStateGlobal(cx)` or `gk.GlobalStateGlobalMut(cx)`
- **`base.GridLayoutState`** (GridLayoutState, kit state): `base.NewGridLayoutState(cx)….Build()`; shown by `base.GridLayout(state)`, `base.KitGridLayout(state)`; built with `.Cols`, `.Compaction`, `.DefaultAreas`, `.Draggable`, `.PreventCollision`, `.Resizable`; reads `.ActiveItem`, `.Area`, `.Areas`, `.ColumnCount`, `.CompactionAxis`, `.IsCollisionPrevented`, `.IsDraggable`, `.IsResizable` and 3 more; writes `.SetAreas`, `.SetCols`, `.SetCompaction`, `.SetDraggable`, `.SetPreventCollision`, `.SetResizable`, `.SetSelectedItem`; events `.OnGridLayoutEvent(cx, f)`
- **`gk.History`** (History<SharedString>, kit state): `gk.NewHistory(cx)….Build()`; built with `.MaxEntries`; reads `.Back`, `.CanBack`, `.CanForward`, `.Current`, `.Entries`, `.Forward`, `.ForwardEntries`, `.RemoveCurrent`; writes `.Clear`, `.Push`, `.ReplaceCurrent`, `.Retain`
- **`base.HorizontalScrollClaim`** (HorizontalScrollClaim, kit state): handed out by the kit (no constructor); reads `.At`, `.DeltaX`
- **`base.HslaSliders`** (HslaSliders, kit state): handed out by the kit (no constructor); reads `.Alpha`, `.Hue`, `.Lightness`, `.Saturation`
- **`editor.InlineRenderContext`** (InlineRenderContext, kit state): handed out by the kit (no constructor); reads `.FontSize`, `.LineHeight`, `.RemSize`, `.TextStyle`
- **`motion.Keyframes`** (Keyframes<f32>, kit state): handed out by the kit (no constructor); reads `.IsEmpty`, `.Len`, `.Sample`
- **`gk.MarkdownNodeRead`** (MarkdownNode, kit state): handed out by the kit (no constructor); reads `.AccessibilityName`, `.AsMarkdown`, `.AsText`, `.Name`, `.SourceRange`
- **`base.NavStackState`** (NavStackState, kit state): `base.NewNavStackState(cx)`; shown by `base.NavStack(state)`; reads `.Current`, `.Depth`, `.Forward`, `.ForwardViews`, `.IsEmpty`, `.Pop`, `.PopToRoot`, `.Replace` and 1 more; writes `.Clear`, `.Push`; events `.OnNavStackEvent(cx, f)`
- **`editor.OtpState`** (OtpState, kit state): `editor.NewOtpState(cx, length)….Build()`; shown by `editor.BaseOtpInput(state)`, `editor.OtpInput(state)`; built with `.DefaultValue`, `.Masked`; reads `.Value`, `.CursorVisible`, `.IsEmpty`, `.IsMasked`, `.Len`, `.TryValue`; writes `.Focus`, `.SetMasked`, `.SetValue`; events `.OnOtpEvent(cx, f)`
- **`gk.PlotThemeRead`** (PlotTheme, kit state): handed out by the kit (no constructor); reads `.Motion`
- **`overlay.PopoverState`** (PopoverState, kit state): `overlay.NewPopoverState(cx, defaultOpen)`; reads `.IsOpen`; writes `.Dismiss`, `.Show`
- **`editor.RangeHighlight`** (RangeHighlight, kit state): `editor.NewRangeHighlight(cx, range_, background)`; reads `.Background`, `.Range`
- **`editor.RenderedText`** (RenderedText, kit state): handed out by the kit (no constructor); reads `.AsStr`, `.IsEmpty`, `.Len`, `.RangeForSource`, `.Source`
- **`base.ResizableState`** (ResizableState, kit state): `base.NewResizableState(cx)`; reads `.ContainerSize`, `.IsResizing`, `.Sizes`; writes `.Clear`, `.InsertPanel`, `.RemovePanel`, `.ResetPanel`, `.ResizePanel`; events `.OnResizablePanelEvent(cx, f)`
- **`gk.Root`** (Root, kit state): handed out by the kit (no constructor); reads `.View`
- **`gk.ScrollbarMotionRead`** (ScrollbarMotion, kit state): handed out by the kit (no constructor); reads `.Enter`, `.Entrance`, `.Exit`, `.Expand`, `.Idle`, `.ThumbHoverEntrance`
- **`gk.ScrollbarThemeRead`** (ScrollbarTheme, kit state): handed out by the kit (no constructor); reads `.Mode`, `.Motion`
- **`motion.Sequence`** (Sequence<f32>, kit state): `motion.NewSequence(cx, id, from)….Build()`; built with `.WithStep`, `.WithSteps`; reads `.From`, `.Sample`, `.Steps`
- **`motion.SequenceSample`** (SequenceSample<f32>, kit state): handed out by the kit (no constructor); reads `.Value`, `.IntoValue`, `.IsFinished`, `.Status`, `.Step`
- **`motion.SequenceStep`** (SequenceStep<f32>, kit state): `motion.NewSequenceStep(cx, target, transition)`; reads `.Target`
- **`motion.Stagger`** (Stagger, kit state): `motion.NewStagger(cx, interval, origin)`; reads `.Delay`
- **`editor.TaskToggleEvent`** (TaskToggleEvent, kit state): handed out by the kit (no constructor); reads `.Checked`, `.ItemRange`, `.MarkerRange`
- **`editor.TextSelectionEndpoint`** (TextSelectionEndpoint, kit state): handed out by the kit (no constructor); reads `.ContentPoint`
- **`editor.TextSelectionHandle`** (TextSelectionHandle, kit state): `editor.NewTextSelectionHandle(cx, fallbackCopyText)`; shown by `editor.SelectableTextWithHandle(state)`; reads `.HasLocalSelection`, `.Snapshot`; writes `.PaintTouchHandles`, `.SetFallbackCopyText`, `.SetLocalSelection`
- **`editor.TextSelectionProjection`** (TextSelectionProjection, kit state): `editor.NewTextSelectionProjectionDefault(cx)`
- **`editor.TextSelectionSnapshot`** (TextSelectionSnapshot, kit state): handed out by the kit (no constructor); reads `.Anchor`, `.Coverage`, `.Cursor`, `.IsSelecting`, `.WindowPoints`
- **`editor.TextSelectionWindowPoints`** (TextSelectionWindowPoints, kit state): handed out by the kit (no constructor); reads `.Anchor`, `.Cursor`
- **`editor.TextViewDefaults`** (TextViewDefaults, kit state): `editor.NewTextViewDefaults(cx)….Build()` or `editor.NewTextViewDefaultsGlobal(cx)….Build()`; built with `.WithCodeBlockHighlighter`, `.WithInheritTextColor`, `.WithStyle`; reads `.HasCodeBlockHighlighter`, `.InheritTextColor`; writes `.Install`
- **`editor.TextViewState`** (TextViewState, kit state): `editor.NewTextViewStateHTML(cx, text)….Build()` or `editor.NewTextViewStateMarkdown(cx, text)….Build()`; shown by `editor.BaseTextView(state)`, `editor.TextView(state)`; built with `.Motion`, `.Scrollable`, `.Selectable`, `.SelectionFormat`; reads `.IsClamped`, `.RenderedText`, `.RevealRange`, `.SelectedSourceRange`, `.SelectedText`, `.SetRangeHighlights`; writes `.ClearRangeHighlights`, `.ClearSelection`, `.InvalidateInlineLayout`, `.PushStr`, `.SelectAll`, `.SetMotion`, `.SetScrollable`, `.SetSelectable` and 2 more
- **`base.TimeFieldState`** (TimeFieldState, kit state): `base.NewTimeFieldState(cx)….Build()`; shown by `base.TimeField(state)`, `kit.TimeField(state)`; built with `.HourCycle`, `.Precision`; reads `.SelectedSegment`, `.Time`; writes `.Focus`, `.SetHourCycle`, `.SetPrecision`, `.SetTime`; events `.OnTimeFieldEvent(cx, f)`
- **`overlay.ToastManager`** (ToastManager<u64, kit state): `overlay.NewToastManager(cx, motion)`; reads `.Advance`, `.Dismiss`, `.DismissAll`, `.Get`, `.IsEmpty`, `.Iter`, `.Len`, `.Push` and 1 more
- **`overlay.ToastStackState`** (ToastStackState, kit state): `overlay.NewToastStackStateDefault(cx)`; shown by `overlay.ToastStack(state)`; reads `.IsExpanded`
- **`overlay.TooltipDefaults`** (TooltipDefaults, kit state): `overlay.NewTooltipDefaults(cx)….Build()` or `overlay.NewTooltipDefaultsGlobal(cx)….Build()`; built with `.WithGracePeriod`, `.WithShowDelay`; reads `.GracePeriod`, `.ShowDelay`; writes `.Install`
- **`overlay.TooltipOverlay`** (TooltipOverlay, kit state): `overlay.NewTooltipOverlay(cx)….Build()`; built with `.RenderWith`; writes `.Hide`, `.RequestHide`, `.RequestShow`
- **`editor.TouchSelectionSnapshot`** (TouchSelectionSnapshot, kit state): handed out by the kit (no constructor); reads `.Bounds`, `.Dragging`, `.Edge`, `.End`, `.IsEdgeVisible`, `.IsEmpty`, `.IsMenuOpen`, `.Start`
- **`motion.TransitionState`** (TransitionState<f32>, kit state): `motion.NewTransitionStateDefault(cx)`; reads `.Sample`
- **`table.TreeState`** (TreeState, kit state): `table.NewTreeState(cx)….Build()`; shown by `table.BaseTree(state)`, `table.KitTree(state)`, `table.Tree(state)`; built with `.Items`; reads `.Entry`, `.IndexOf`, `.ScrollHandle`, `.SelectedEntry`, `.SelectedIndex`, `.SelectedItem`; writes `.Focus`, `.RevealItem`, `.ScrollToItem`, `.SetItems`, `.SetSelectedIndex`, `.SetSelectedItem`; events `.OnTreeEvent(cx, f)`
- **`gk.UndoHistory`** (UndoHistory<SharedString>, kit state): `gk.NewUndoHistory(cx)….Build()`; built with `.GroupInterval`, `.MaxUndos`; reads `.CanRedo`, `.CanUndo`, `.IsIgnoring`, `.Redo`, `.Undo`; writes `.Clear`, `.EndGrouping`, `.Push`, `.SetIgnoring`, `.StartGrouping`
- **`table.VirtualListScrollHandle`** (VirtualListScrollHandle, kit state): `table.NewVirtualListScrollHandle(cx)`; reads `.BaseHandle`; writes `.ScrollToBottom`, `.ScrollToItem`
- **`editor.BaseTextViewStyle`** (TextViewStyle, value): `editor.NewBaseTextViewStyleDefault()` or `editor.NewBaseTextViewStyleFromTheme(theme)`; `.WithBlockquoteStyle`, `.WithBorder`, `.WithCodeBackground`, `.WithCodeBlock`, `.WithDark`, `.WithForeground`, `.WithHeading`, `.WithInlineCode` and 10 more
- **`base.ButtonStyles`** (ButtonStyles, value): `base.NewButtonStylesDefault()`; `.Selected`, `.Disabled`
- **`base.CheckboxIndicatorStyles`** (CheckboxIndicatorStyles, value): `base.NewCheckboxIndicatorStylesDefault()`; `.Checked`, `.Disabled`, `.Indeterminate`
- **`base.CheckboxStyles`** (CheckboxStyles, value): `base.NewCheckboxStylesDefault()`; `.Checked`, `.Disabled`, `.Indeterminate`
- **`base.GridArea`** (GridArea, value): `base.NewGridArea(id)`; `.Col`, `.ColSpan`, `.Locked`, `.MaxSpan`, `.MinSpan`, `.Row`, `.RowSpan`
- **`base.GridItem`** (GridItem, value): `base.NewGridItem(id)`; `.AccessibilityLabel`, `.BorderE`, `.BorderS`, `.DragHandleOnly`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd` and 10 more
- **`editor.InputStyles`** (InputStyles, value): `editor.NewInputStylesDefault()`; `.Disabled`, `.Focused`
- **`base.LinkStyles`** (LinkStyles, value): `base.NewLinkStylesDefault()`; `.Disabled`
- **`editor.MarkdownExtensions`** (MarkdownExtensions, value): `editor.NewMarkdownExtensionsDefault()`; `.BlockRenderer`, `.BlockSurface`, `.Frontmatter`, `.Mdx`, `.ParserRevision`, `.Plugin`, `.RawHTML`
- **`base.PaginationState`** (PaginationState, value): `base.NewPaginationState(currentPage, totalPages)`; `.OnChange`, `.Disabled`, `.VisiblePages`
- **`gk.PlotMotion`** (PlotMotion, value): `chart.NewPlotMotionDefault()`; `.WithAppear`, `.WithEnter`, `.WithExit`, `.WithPointer`
- **`gk.PlotTheme`** (PlotTheme, value): `gk.NewPlotTheme()` or `gk.NewPlotThemeDefault()`; `.WithMotion`
- **`motion.Presence`** (Presence, value): `motion.NewPresence(id, present)`; `.Transition`
- **`base.RadioStyles`** (RadioStyles, value): `base.NewRadioStylesDefault()`; `.Checked`, `.Disabled`
- **`motion.ScrollBounceMotion`** (ScrollBounceMotion, value): `motion.NewScrollBounceMotionDefault()`; `.WithMaxStretch`, `.WithResponse`, `.WithTracking`
- **`gk.ScrollbarMotion`** (ScrollbarMotion, value): `gk.NewScrollbarMotionDefault()`; `.WithEnter`, `.WithEntrance`, `.WithExit`, `.WithExpand`, `.WithIdle`, `.WithThumbHoverEntrance`
- **`gk.ScrollbarStyles`** (ScrollbarStyles, value): `gk.NewScrollbarStylesDefault()`; `.Thumb`, `.ThumbActive`, `.ThumbHover`, `.Track`, `.TrackActive`, `.TrackHover`
- **`gk.ScrollbarTheme`** (ScrollbarTheme, value): `gk.NewScrollbarTheme()` or `gk.NewScrollbarThemeDefault()`; `.WithMode`, `.WithMotion`, `.WithStyles`
- **`gk.ScrollbarThumbStyle`** (ScrollbarThumbStyle, value): `gk.NewScrollbarThumbStyleDefault()`; `.Bg`, `.Inset`, `.MinLength`, `.Radius`, `.Width`
- **`gk.ScrollbarTrackStyle`** (ScrollbarTrackStyle, value): `gk.NewScrollbarTrackStyleDefault()`; `.Bg`, `.BorderColor`, `.Width`
- **`gk.Spring`** (Spring, value): `motion.NewPointerSpring()` or `motion.NewSpring(response)`; `.WithDamping`, `.WithEpsilon`, `.WithTravel`
- **`gk.StateStyle`** (StateStyle, value): `gk.NewStateStyleDefault()`; `.BorderE`, `.BorderS`, `.FlexRowDir`, `.InsetE`, `.InsetS`, `.JustifyFlexEnd`, `.JustifyFlexStart`, `.Me` and 8 more
- **`base.SwitchStyles`** (SwitchStyles, value): `base.NewSwitchStylesDefault()`; `.Checked`, `.Disabled`
- **`base.SwitchThumbStyles`** (SwitchThumbStyles, value): `base.NewSwitchThumbStylesDefault()`; `.Checked`, `.Disabled`
- **`base.SwitchTrackStyles`** (SwitchTrackStyles, value): `base.NewSwitchTrackStylesDefault()`; `.Checked`, `.Disabled`
- **`base.TabStyles`** (TabStyles, value): `base.NewTabStylesDefault()`; `.Selected`, `.Disabled`
- **`editor.TextViewMotion`** (TextViewMotion, value): `editor.NewTextViewMotionDefault()`; `.WithStreamFade`, `.WithStreamFadeEasing`, `.WithStreamFadeStagger`
- **`motion.Timing`** (Timing, value): `motion.NewTiming(duration)`; `.Delay`, `.Direction`, `.Ease`, `.Iterations`
- **`base.ToggleStyles`** (ToggleStyles, value): `base.NewToggleStylesDefault()`; `.Disabled`, `.Pressed`
- **`overlay.TooltipRequest`** (TooltipRequest, value): `overlay.NewTooltipRequest(triggerBounds, build)`; `.WithOffset`, `.WithPlacement`, `.WithShowDelay`
- **`gk.Transition`** (Transition, value): `motion.NewTransition(duration)`; `.Delay`, `.Ease`, `.Easing`, `.ReducedMotion`
- **`table.TreeItem`** (TreeItem, value): `table.NewTreeItem(id, label)`; `.Child`, `.Children`, `.Disabled`, `.Expanded`

### base::animation

- **`motion.EffectTransition`** (EffectTransition, value): `motion.NewEffectTransition(duration)`; `.Ease`, `.Fade`, `.Height`, `.SlideX`, `.SlideY`, `.Width`

### base::dock

- **`dock.Dock`** (Dock, kit state): `dock.NewDock(cx, size)`; reads `.IsCollapsible`, `.IsOpen`, `.IsResizing`, `.Size`; writes `.SetCollapsible`, `.SetOpen`, `.SetResizing`, `.SetSize`
- **`dock.Area`** (DockArea, kit state): `dock.NewArea(cx, id, version)….Build()`; built with `.WithRenderer`; reads `.Bounds`, `.DockSize`, `.Dump`, `.HasDock`, `.ID`, `.IsDockCollapsible`, `.IsDockOpen`, `.IsEmpty` and 7 more; writes `.AddPanel`, `.AddPanelView`, `.MovePanel`, `.RemoveDock`, `.RemovePanel`, `.SelectPanel`, `.SetCenter`, `.SetDock` and 9 more; events `.OnDockEvent(cx, f)`
- **`dock.Context`** (DockContext, kit state): handed out by the kit (no constructor); reads `.IsCollapsible`, `.IsOpen`, `.Placement`, `.Size`; writes `.EndResize`, `.ResizeTo`, `.Toggle`
- **`dock.State`** (DockState, kit state): `dock.NewState(cx, panel, placement, size, open)`; reads `.Open`, `.Panel`, `.Placement`, `.Size`
- **`dock.DragPanel`** (DragPanel, kit state): `dock.NewDragPanel(cx, panel, source)`; reads `.DragOffset`, `.DragSessionID`, `.Panel`, `.PreviewSize`, `.Source`; writes `.SetDragOffset`, `.SetPreviewSize`
- **`dock.DropIndicator`** (DropIndicator, kit state): handed out by the kit (no constructor); reads `.Bounds`, `.DragSessionID`, `.Epoch`, `.From`, `.Placement`, `.To`
- **`dock.DropPlaceholderBounds`** (DropPlaceholderBounds, kit state): `dock.NewDropPlaceholderBoundsForPlacement(cx, bounds, placement)`; reads `.Origin`, `.Size`
- **`dock.DropTarget`** (DropTarget, kit state): handed out by the kit (no constructor); reads `.Node`, `.Placement`
- **`dock.EditResult`** (EditResult, kit state): `dock.NewEditResultDefault(cx)`; reads `.Changed`
- **`dock.PaneNode`** (PaneNode, kit state): handed out by the kit (no constructor); reads `.ID`
- **`dock.PaneTree`** (PaneTree, kit state): `dock.NewPaneTree(cx, rootKind)`; reads `.ContainsPanel`, `.FindNode`, `.FindPanelNode`, `.InsertPanel`, `.MovePanel`, `.NodeIDs`, `.Panels`, `.RemovePanel` and 5 more; writes `.Normalize`
- **`dock.PanelRegistry`** (PanelRegistry, kit state): `dock.NewPanelRegistry(cx)` or `dock.PanelRegistryGlobal(cx)` or `dock.PanelRegistryGlobalMut(cx)`
- **`dock.TabGroup`** (TabGroup, kit state): handed out by the kit (no constructor); built with `.WithRenderer`; reads `.ActiveIx`, `.ActivePanel`, `.Context`, `.IsClosable`, `.IsCollapsed`, `.IsZoomed`, `.Node`, `.Panels`; writes `.ClosePanel`, `.SelectTab`, `.ToggleZoom`; events `.OnTabGroupEvent(cx, f)`
- **`dock.TabGroupContext`** (TabGroupContext, kit state): handed out by the kit (no constructor); reads `.ActiveIx`, `.ActivePanel`, `.DragPanel`, `.DropIndicator`, `.IsClosable`, `.IsCollapsed`, `.IsDraggable`, `.IsDroppable` and 5 more; writes `.Close`, `.DropItem`, `.DropPanel`, `.SelectTab`, `.ToggleZoom`
- **`dock.Layout`** (DockLayout, value): `dock.NewLayoutHSplit()` or `dock.NewLayoutTabs()` or `dock.NewLayoutVSplit()`; `.Child`, `.ActiveIndex`, `.Panel`, `.PanelView`
- **`dock.Sizing`** (DockSizing, value): `dock.NewSizing(placement)`; `.WithAreaBounds`, `.WithAreaHeight`, `.WithAreaWidth`, `.WithOppositeDockSize`

### base::input

- **`editor.CodeActionMenuState`** (CodeActionMenuState, kit state): `editor.NewCodeActionMenuStateDefault(cx)`; reads `.Revision`
- **`editor.CompletionMenuState`** (CompletionMenuState, kit state): `editor.NewCompletionMenuStateDefault(cx)`; reads `.Revision`
- **`gk.CursorBlinkRead`** (CursorBlink, kit state): handed out by the kit (no constructor); reads `.IdleTimeout`, `.Interval`, `.IsSteady`
- **`editor.DiagnosticSet`** (DiagnosticSet, kit state): handed out by the kit (no constructor); reads `.ForOffset`, `.IsEmpty`, `.Iter`, `.Len`, `.Range`; writes `.Clear`, `.Extend`, `.Push`
- **`editor.DisplayMap`** (DisplayMap, kit state): `editor.NewDisplayMap(cx, font, fontSize, wrapWidth)`; reads `.Text`, `.BufferLineCount`, `.BufferLineToDisplayRow`, `.BufferLineToDisplayRowRange`, `.BufferPosToDisplayPos`, `.DisplayPosToBufferPos`, `.DisplayRowCount`, `.DisplayRowToBufferLine` and 9 more; writes `.ClearFolds`, `.OnLayoutChanged`, `.SetFoldCandidates`, `.SetFolded`, `.SetFont`, `.SetWrappingIndent`, `.ToggleFold`
- **`editor.ScrollbarContext`** (EditorScrollbarContext, kit state): handed out by the kit (no constructor); reads `.Axis`, `.ContentHeightF64`, `.Placement`, `.ScrollViewportSize`, `.ScrollYF64`
- **`editor.State`** (EditorState, kit state): `editor.NewState(cx)….Build()`; shown by `editor.Editor(state)`; built with `.Placeholder`, `.Autocapitalize`, `.Autocorrect`, `.CleanOnEscape`, `.ContextMenu`, `.CursorSurroundingLines`, `.DefaultValue`, `.GutterDecorationsWidth` and 7 more; reads `.Text`, `.Value`, `.ContentHeightF64`, `.ContextMenuCapabilities`, `.ContextMenuOffset`, `.CreateDecorationsCollection`, `.CreateGutterDecorationsCollection`, `.CreateOverviewRulerDecorationsCollection` and 61 more; writes `.AddSelection`, `.ApplyHighlighterFoldCandidates`, `.BeginEdgeDrag`, `.Clean`, `.ClearDiagnosticPopover`, `.ClearHoverState`, `.CloseEditMenu`, `.CloseSearch` and 68 more; events `.OnInputEditsEvent(cx, f)`, `.OnInputEvent(cx, f)`
- **`editor.GutterDecorationCollection`** (GutterDecorationCollection, kit state): `editor.NewGutterDecorationCollectionDefault(cx)`; reads `.GetRanges`; writes `.Append`, `.Clear`, `.Dispose`, `.Set`
- **`editor.InlineTokenClickEvent`** (InlineTokenClickEvent, kit state): handed out by the kit (no constructor); reads `.Bounds`, `.Click`, `.Range`, `.Token`
- **`editor.InlineTokenContext`** (InlineTokenContext, kit state): handed out by the kit (no constructor); shown by `editor.InputToken(state)`; reads `.AvailableWidth`, `.IsDisabled`, `.IsReadonly`, `.IsSelected`, `.LineHeight`, `.Range`, `.Token`
- **`editor.InlineTokenHoverEvent`** (InlineTokenHoverEvent, kit state): handed out by the kit (no constructor); reads `.Bounds`, `.IsHovered`, `.Range`, `.RangeUtf16`, `.Token`
- **`editor.InlineTokenRead`** (InlineToken, kit state): handed out by the kit (no constructor); reads `.Label`, `.Text`, `.ID`
- **`editor.InlineTokenSpan`** (InlineTokenSpan, kit state): handed out by the kit (no constructor); reads `.Range`, `.Token`
- **`editor.InputContent`** (InputContent, kit state): `editor.NewInputContent(cx, text)`; reads `.Text`, `.Tokens`, `.WithToken`
- **`editor.InputContextMenuCapabilitiesRead`** (InputContextMenuCapabilities, kit state): handed out by the kit (no constructor); reads `.HasCodeActions`, `.HasDefinition`, `.HasSelection`, `.IsCodeEditor`, `.IsCopyable`, `.IsDisabled`, `.IsEditable`, `.IsMasked` and 1 more
- **`editor.InputPresentation`** (InputPresentation, kit state): handed out by the kit (no constructor); reads `.Placeholder`, `.FocusHandle`, `.IsCodeEditor`, `.IsDisabled`, `.IsEditable`, `.IsLoading`, `.IsMasked`, `.IsMultiLine` and 3 more
- **`editor.InputState`** (InputState, kit state): `editor.NewInputState(cx)….Build()`; shown by `editor.BaseInput(state)`, `editor.BaseNumberInput(state)`, `editor.Input(state)`, `base.NewQuestionnaireInputDefinition(state)`, `editor.NumberInput(state)`; built with `.Placeholder`, `.Autocapitalize`, `.Autocorrect`, `.CleanOnEscape`, `.ContextMenu`, `.CursorSurroundingLines`, `.DefaultValue`, `.InputAction` and 8 more; reads `.Text`, `.Value`, `.Content`, `.ContentHeightF64`, `.ContextMenuCapabilities`, `.ContextMenuOffset`, `.Cursor`, `.CursorPosition` and 53 more; writes `.AddSelection`, `.ApplyHighlighterFoldCandidates`, `.BeginEdgeDrag`, `.Clean`, `.CloseEditMenu`, `.CloseSearch`, `.EndEdgeDrag`, `.EnsureHighlighterFactory` and 47 more; events `.OnInputEditsEvent(cx, f)`, `.OnInputEvent(cx, f)`
- **`editor.OverviewRulerDecorationCollection`** (OverviewRulerDecorationCollection, kit state): `editor.NewOverviewRulerDecorationCollectionDefault(cx)`; reads `.GetRanges`; writes `.Append`, `.Clear`, `.Dispose`, `.Set`
- **`editor.RangeDecorationCollection`** (RangeDecorationCollection, kit state): handed out by the kit (no constructor); reads `.GetRanges`; writes `.Append`, `.Clear`, `.Dispose`, `.Set`
- **`editor.SearchMatcher`** (SearchMatcher, kit state): `editor.NewSearchMatcher(cx)`; reads `.Label`, `.Current`, `.CurrentMatchIndex`, `.IsEmpty`, `.Len`, `.MatchedRanges`; writes `.UpdateCursorByOffset`, `.UpdateQuery`
- **`editor.SearchSession`** (SearchSession, kit state): `editor.NewSearchSessionDefault(cx)`; reads `.IsActive`
- **`editor.TextDecorationCollection`** (TextDecorationCollection, kit state): handed out by the kit (no constructor); reads `.GetRanges`; writes `.Append`, `.Clear`, `.Set`
- **`editor.TextareaState`** (TextareaState, kit state): `editor.NewTextareaState(cx)….Build()`; shown by `editor.BaseTextarea(state)`, `editor.Textarea(state)`; built with `.Placeholder`, `.AutoGrow`, `.Autocapitalize`, `.Autocorrect`, `.CleanOnEscape`, `.ContextMenu`, `.CursorSurroundingLines`, `.DefaultValue` and 6 more; reads `.Text`, `.Value`, `.Content`, `.ContentHeightF64`, `.ContextMenuCapabilities`, `.ContextMenuOffset`, `.Cursor`, `.CursorPosition` and 54 more; writes `.AddSelection`, `.ApplyHighlighterFoldCandidates`, `.BeginEdgeDrag`, `.Clean`, `.CloseEditMenu`, `.CloseSearch`, `.EndEdgeDrag`, `.EnsureHighlighterFactory` and 50 more; events `.OnInputEditsEvent(cx, f)`, `.OnInputEvent(cx, f)`
- **`editor.AutoClosingPair`** (AutoClosingPair, value): `editor.NewAutoClosingPair(open, close_)`; `.NotIn`
- **`editor.BracketPair`** (BracketPair, value): `editor.NewBracketPair(open, close_)`
- **`gk.CursorBlink`** (CursorBlink, value): `editor.NewCursorBlinkDefault()` or `editor.NewCursorBlinkSteady()` or `editor.NewCursorBlinkBlinking(interval)`; `.WithIdleTimeout`
- **`editor.Diagnostic`** (Diagnostic, value): `editor.NewDiagnosticDefault()` or `editor.NewDiagnostic(range_, message)`; `.WithCode`, `.WithSeverity`, `.WithSource`
- **`editor.GutterDecoration`** (GutterDecoration, value): `editor.NewGutterDecorationDefault()` or `editor.NewGutterDecoration(range_)`; `.WithColor`, `.WithFoldedColor`
- **`editor.InlineToken`** (InlineToken, value): `editor.NewInlineToken(id, text)`; `.WithLabel`
- **`editor.InputAccessibility`** (InputAccessibility, value): `editor.NewInputAccessibility(role)`; `.WithAccessibilityID`, `.WithLabel`, `.WithPlaceholder`, `.WithValueExposed`
- **`editor.LanguageConfig`** (LanguageConfig, value): `editor.NewLanguageConfigDefault()`; `.AutoCloseBefore`, `.AutoClosingPairs`, `.Brackets`, `.IndentationRules`
- **`editor.Lsp`** (Lsp, value): `editor.NewLspDefault()`; `.WithCompletionHighlighter`
- **`editor.OverviewRulerDecoration`** (OverviewRulerDecoration, value): `editor.NewOverviewRulerDecorationDefault()` or `editor.NewOverviewRulerDecoration(range_)`; `.WithColor`, `.WithLane`
- **`editor.RangeDecoration`** (RangeDecoration, value): `editor.NewRangeDecoration(range_)`; `.WithColor`, `.WithStyle`

### base::plot

- **`chart.PlotAppearScopeEl`** (PlotAppearScope, element): `chart.PlotAppearScope(id, child)`
- **`chart.PathCache`** (PathCache, kit state): `chart.NewPathCacheDefault(cx)`; reads `.IsWarm`
- **`chart.PathCaches`** (PathCaches, kit state): `chart.NewPathCachesDefault(cx)`
- **`chart.Pie`** (Pie<Datum>, kit state): `chart.NewPie(cx)….Build()`; built with `.Value`, `.EndAngle`, `.PadAngle`, `.StartAngle`; reads `.Arcs`
- **`chart.PlotAppear`** (PlotAppear, kit state): `chart.NewPlotAppearComplete(cx)`; reads `.IsAppearing`, `.Progress`, `.Staggered`
- **`chart.PlotLabel`** (PlotLabel, kit state): `chart.NewPlotLabel(cx, items)`; writes `.Paint`
- **`chart.SankeyGraph`** (SankeyGraph, kit state): `chart.NewSankeyGraphDefault(cx)` or `chart.BaseSankeyLayoutFrom(cx, value, graph)`; reads `.LayerCount`
- **`gk.ScaleBand`** (ScaleBand<SharedString>, kit state): `gk.NewScaleBand(cx, domain, range_)….Build()`; built with `.BandCount`, `.MaxBandWidth`, `.PaddingInner`, `.PaddingOuter`; reads `.BandWidth`, `.NearestIndex`, `.Step`, `.Tick`
- **`chart.ScaleLinear`** (ScaleLinear<f64>, kit state): `chart.NewScaleLinear(cx, domain, range_)`; reads `.NearestIndex`, `.Tick`
- **`gk.ScaleOrdinal`** (ScaleOrdinal<SharedString, kit state): `gk.NewScaleOrdinal(cx, domain, range_)….Build()`; built with `.Domain`, `.Range`, `.Unknown`; reads `.Map`
- **`gk.ScalePoint`** (ScalePoint<SharedString>, kit state): `gk.NewScalePoint(cx, domain, range_)`; reads `.NearestIndex`, `.Tick`, `.TickAt`
- **`chart.ShapeKey`** (ShapeKey, kit state): `chart.NewShapeKey(cx, extra)`; reads `.Finish`
- **`chart.Stack`** (Stack<Datum>, kit state): `chart.NewStack(cx)….Build()`; built with `.Value`, `.Data`, `.Keys`; reads `.Series`
- **`chart.AreaShape`** (Area<Datum>, value): `chart.NewAreaShape()` or `chart.NewAreaShapeDefault()`; `.Curve`, `.Data`, `.Fill`, `.Stroke`, `.X`, `.Y0`, `.Y1`
- **`chart.AxisText`** (AxisText, value): `chart.NewAxisText(text, tick, color)`; `.Align`, `.FontSize`
- **`chart.BarShape`** (Bar<Datum>, value): `chart.NewBarShape()` or `chart.NewBarShapeDefault()`; `.Label`, `.Value`, `.Alignment`, `.BandWidth`, `.Base`, `.CornerRadii`, `.Cross`, `.Data` and 1 more
- **`chart.Text`** (Text, value): `chart.NewBaseText(text, origin, color)`; `.Align`, `.FontSize`, `.FontWeight`
- **`chart.Grid`** (Grid, value): `chart.NewGrid()` or `chart.NewGridDefault()`; `.DashArray`, `.Stroke`, `.X`, `.Y`
- **`chart.LineShape`** (Line<Datum>, value): `chart.NewLineShape()` or `chart.NewLineShapeDefault()`; `.Curve`, `.Data`, `.Dot`, `.DotFill`, `.DotSize`, `.DotStroke`, `.Stroke`, `.StrokeWidth` and 2 more
- **`chart.PlotAxis`** (PlotAxis, value): `chart.NewPlotAxis()` or `chart.NewPlotAxisDefault()`; `.Stroke`, `.X`, `.XAxis`, `.XLabel`, `.XLabelSide`, `.Y`, `.YAxis`, `.YLabel` and 1 more
- **`chart.RadialLineShape`** (RadialLine<Datum>, value): `chart.NewRadialLineShape()` or `chart.NewRadialLineShapeDefault()`; `.Angle`, `.Closed`, `.Data`, `.Dot`, `.DotFill`, `.DotSize`, `.DotStroke`, `.Fill` and 3 more
- **`chart.Sankey`** (Sankey, value): `chart.NewSankey()` or `chart.NewSankeyDefault()`; `.Extent`, `.Iterations`, `.NodeAlign`, `.NodePadding`, `.NodeWidth`, `.Size`, `.ValueScale`
- **`chart.SankeyLink`** (SankeyLink, value): `chart.NewSankeyLink(source, target, value)`

### base::questionnaire

- **`base.QuestionnaireAnswerChange`** (QuestionnaireAnswerChange, kit state): handed out by the kit (no constructor); reads `.Answer`, `.Item`, `.Status`
- **`base.QuestionnaireAnswerRead`** (QuestionnaireAnswer, kit state): handed out by the kit (no constructor); reads `.Choices`, `.Freeform`, `.IsEmpty`
- **`base.QuestionnaireAnswers`** (QuestionnaireAnswers, kit state): `base.NewQuestionnaireAnswersDefault(cx)`; reads `.Get`, `.IsEmpty`, `.Iter`, `.Len`
- **`base.QuestionnaireChoiceDefinitionRead`** (QuestionnaireChoiceDefinition, kit state): handed out by the kit (no constructor); reads `.Value`, `.Description`, `.AccessibilityLabel`, `.IsDefaultSelected`, `.IsDisabled`
- **`base.QuestionnaireChoiceState`** (QuestionnaireChoiceState, kit state): handed out by the kit (no constructor); reads `.Value`, `.IsDisabled`, `.IsInvalid`, `.IsSelected`, `.Shortcut`
- **`base.QuestionnaireInputDefinitionRead`** (QuestionnaireInputDefinition, kit state): handed out by the kit (no constructor); reads `.AccessibilityLabel`, `.IsDisabled`, `.State`
- **`base.QuestionnaireItemDefinitionRead`** (QuestionnaireItemDefinition, kit state): handed out by the kit (no constructor); reads `.Description`, `.AccessibilityLabel`, `.Choices`, `.Input`, `.IsDisabled`, `.IsMultiple`, `.IsRequired`, `.Name` and 1 more
- **`base.QuestionnaireItemState`** (QuestionnaireItemState, kit state): handed out by the kit (no constructor); reads `.HasInput`, `.IsDisabled`, `.IsInvalid`, `.IsMultiple`, `.IsRequired`, `.Name`, `.Status`
- **`base.QuestionnaireNavigationState`** (QuestionnaireNavigationState, kit state): `base.NewQuestionnaireNavigationStateDefault(cx)`; reads `.IsConfirmable`, `.IsNextVisible`, `.IsPreviousVisible`, `.IsSkipVisible`, `.IsSubmitVisible`
- **`base.QuestionnaireProgressState`** (QuestionnaireProgressState, kit state): handed out by the kit (no constructor); reads `.Current`, `.Total`
- **`base.QuestionnaireState`** (QuestionnaireState, kit state): `base.NewQuestionnaireState(cx, items)….Build()`; shown by `kit.Questionnaire(state)`, `kit.QuestionnaireActions(state)`, `kit.QuestionnaireChoice(state)`, `kit.QuestionnaireChoices(state)`, `kit.QuestionnaireDescription(state)`, `kit.QuestionnaireError(state)`, `kit.QuestionnaireInput(state)`, `kit.QuestionnaireItem(state)`, `kit.QuestionnaireNext(state)`, `kit.QuestionnairePrevious(state)`, `kit.QuestionnaireProgress(state)`, `kit.QuestionnaireSkip(state)`, `kit.QuestionnaireSubmit(state)`, `kit.QuestionnaireTitle(state)`; built with `.WithCurrentItem`, `.WithShortcuts`; reads `.ActivateChoice`, `.ActivateShortcut`, `.Answer`, `.Answers`, `.ChoiceDefinition`, `.ChoiceFocusHandle`, `.ChoiceForShortcut`, `.ChoicePosition` and 38 more; writes `.Reset`; events `.OnQuestionnaireEvent(cx, f)`
- **`base.QuestionnaireSubmission`** (QuestionnaireSubmission, kit state): `base.NewQuestionnaireSubmissionDefault(cx)`; reads `.Answer`, `.Items`
- **`base.QuestionnaireSubmissionItem`** (QuestionnaireSubmissionItem, kit state): handed out by the kit (no constructor); reads `.Answer`, `.Name`, `.Status`
- **`base.QuestionnaireAnswer`** (QuestionnaireAnswer, value): `base.NewQuestionnaireAnswer()` or `base.NewQuestionnaireAnswerDefault()`; `.WithChoices`, `.WithFreeform`
- **`base.QuestionnaireChoiceDefinition`** (QuestionnaireChoiceDefinition, value): `base.NewQuestionnaireChoiceDefinition(value, accessibilityLabel)`; `.WithDefaultSelected`, `.WithDescription`, `.WithDisabled`
- **`base.QuestionnaireInputDefinition`** (QuestionnaireInputDefinition, value): `base.NewQuestionnaireInputDefinition(state, accessibilityLabel)`; `.WithDisabled`
- **`base.QuestionnaireItemDefinition`** (QuestionnaireItemDefinition, value): `base.NewQuestionnaireItemDefinition(name, accessibilityLabel)`; `.WithChoice`, `.WithChoices`, `.WithDescription`, `.WithDisabled`, `.WithInput`, `.WithMultiple`, `.WithRequired`, `.WithValidator`

### base::slider

- **`base.SliderState`** (SliderState, kit state): `base.NewSliderState(cx)….Build()`; shown by `base.Slider(state)`, `kit.Slider(state)`, `base.SliderIndicator(state)`, `base.SliderThumb(state)`, `base.SliderTrack(state)`; built with `.DefaultValue`, `.Max`, `.Min`, `.Scale`, `.Step`; reads `.Value`, `.AccessibilityStep`, `.MaxValue`, `.MinValue`, `.StepValue`, `.TryValue`; writes `.SetValue`; events `.OnSliderEvent(cx, f)`

### base::text

- **`editor.BaseTable2`** (Table, kit state): `editor.NewBaseTable2(cx, rows, columnAligns)`; reads `.ColumnAligns`, `.Rows`
- **`editor.ImageNode`** (ImageNode, kit state): `editor.NewImageNodeDefault(cx)`; shown by `editor.NewInlineNodeImage(state)`; reads `.Title`
- **`editor.InlineNodeRead`** (InlineNode, kit state): handed out by the kit (no constructor); reads `.Text`, `.AsCustom`, `.AsImage`, `.MarkRuns`
- **`editor.LeafGeometry`** (LeafGeometry, kit state): handed out by the kit (no constructor); reads `.AtomAt`, `.Bounds`, `.CaretForPosition`, `.CaretHeight`, `.IndexForPosition`, `.IsEmpty`, `.Len`, `.LineHeight` and 2 more
- **`editor.ParagraphRead`** (Paragraph, kit state): handed out by the kit (no constructor); reads `.Children`
- **`editor.BaseCodeBlock`** (CodeBlock, value): `editor.NewBaseCodeBlockFromCode(code, lang)` or `editor.NewBaseCodeBlock(code, lang, span)`; `.WithLeafKey`, `.WithMeta`
- **`editor.BaseLink2`** (Link, value): `editor.NewBaseLink2(url)`; `.Title`, `.URL`
- **`editor.FlowPosition`** (FlowPosition, value): `editor.NewFlowPositionAt(blocks, ix, isLast)`; `.First`, `.Last`
- **`editor.InlineNode`** (InlineNode, value): `editor.NewInlineNodeDefault()` or `editor.NewInlineNode(text)` or `editor.NewInlineNodeImage(image)`; `.Marks`
- **`editor.Paragraph`** (Paragraph, value): `editor.NewParagraphDefault()`; `.Push`, `.WithLeafKey`
- **`editor.RenderContext`** (RenderContext, value): `editor.NewRenderContextFromApp()` or `editor.NewRenderContext(style)`; `.OnLinkClick`, `.WithBodyFontSize`, `.WithCodeBlockHighlighter`, `.WithForegroundRanges`, `.WithImageSource`, `.WithLinkReference`, `.WithMarkdownExtensions`, `.WithStyle`
- **`editor.TextMark`** (TextMark, value): `editor.NewTextMarkDefault()`; `.Bold`, `.Code`, `.Highlight`, `.Italic`, `.Link`, `.Merge`, `.Strikethrough`, `.Underline`

<!-- index:end -->
