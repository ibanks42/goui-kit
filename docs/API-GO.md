# API guide for Go: from GPUI-kit's documentation to the Go call

GPUI-kit is documented for Rust, at gpui-kit.com/docs and in its rustdoc. The Go package is
generated from that same API by fixed rules, so the Rust documentation is also the Go
documentation once you know the rules. This page gives the rules, then an index of every component
the package has, generated from the package itself so it is always current. If this is your first
app, read [GETTING-STARTED-GO.md](GETTING-STARTED-GO.md) first. The Odin package has the same API
under Odin's names ([API.md](API.md)); [GO.md](GO.md) explains how the Go package works.

Every Go call on this page also appears in `docs/snippets-go/api/main.go`, a program that the
project's tests compile, so none of them is out of date.
<!-- spans: docs/snippets-go/api/main.go -->

## Where things are

- `import "gpuikit.dev/gk"` is the API, with three small packages beside it:
  `gpuikit.dev/gk/s` (the style tokens, `s.Gap3`), `gpuikit.dev/gk/theme` (the theme's colours,
  `theme.Background`) and `gpuikit.dev/gk/palette` (fixed colours, `palette.Blue500`).
- `go/gk/gen_component_NAME.go` holds the types of upstream's `gpui_kit::component::NAME` module,
  `gen_base_*.go` the unstyled primitives of `gpui_kit::base`, `gen_state.go` every kit state type
  (handles), `gen_types.go` the enums, records and events. Every type and method has a doc comment
  naming its upstream path (`// ButtonEl is gpui_kit::component::button::Button.`), so searching the
  package (or `go doc gpuikit.dev/gk ButtonEl`) for the upstream name finds it.
- `rt_*.go` is the hand-written runtime: `gk.Run`, `gk.NewView`, `gk.Refresh`, `gk.Post`, `gk.Go`,
  `gk.Shortcut`, `gk.NewListSource` and the other functions that have no single upstream
  counterpart.
- Not every upstream function is bound yet. Each one that is missing is listed, with the reason,
  in [STATUS.md](STATUS.md); the Go package also lacks a few that Odin has (see "Not in Go yet"
  below).

## Names

The Go names are the Odin names under fixed rules (docs/GO.md section 2), so this table also
reads against [API.md](API.md).

| Rust (gpui-kit.com/docs) | Go | rule |
|---|---|---|
| `Button::new("ok").primary().label("OK")` | `gk.Button("ok").Primary().Label("OK")` | `Type::new` is a function named after the type; methods are CamelCase and chain |
| `Button` (the type) | `gk.ButtonEl` | an element's Go type has `El` after the name (Go has one namespace for types and functions) |
| `Tag::secondary()` | `gk.TagSecondary()` | any other constructor: type name, then the constructor's name |
| `InputState::new(..)`, `Column::new(..)` | `gk.NewInputState(cx)`, `gk.NewColumn(..)` | the constructor of a state or a value is `New` + its type name; the type keeps the name (`gk.InputState`) |
| `v_flex()`, `h_flex()`, `div()` | `gk.VFlex()`, `gk.HFlex()`, `gk.Div()` | free functions are CamelCase |
| `IconName::StarFill`, `ButtonVariant::Primary` | `gk.IconStarFill`, `gk.ButtonVariantPrimary` | an enum value is its type's name and the variant (a few shortened, as `Icon`) |
| `gpui_kit::base::Checkbox` beside `gpui_kit::component::Checkbox` | `gk.BaseCheckbox("raw")` | when two crates have a type of one name, the component keeps it; base gets `Base`, gpui `Gpui`, the kit's root `Kit` |
| `InputEvent::PressEnter` | `gk.InputEventPressEnter` | an enum with data is an interface: an event's variants are struct types, taken apart with a type switch or `e.(gk.InputEventPressEnter)` |
| `ButtonVariant::Danger`, `Size::Large`, `ButtonVariant::Custom(..)` | `gk.ButtonVariantDanger`, `gk.SizeLarge`, `gk.ButtonVariantCustom{Value: ..}` | in an enum with data that you give to the kit, a variant without data is a constant (of `gk.ButtonVariantKind`), as a plain enum's value; a variant with data is a struct |
| `ListEvent::Confirm(IndexPath)` | `c.Row` on a `gk.ListEventConfirm` | a variant's single record is embedded, so its fields are reached directly; a tuple variant's one value is `Value`, more are `V0`, `V1`, ... |
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
| `.font_bold()` | `gk.Label("bold", s.FontBold)` |

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
- `impl Into<ElementId>` is `gk.ID`: a string (`gk.Button("ok")`), an int, or a name and a number
  (`gk.Key("row", 3)`).
- `Option<T>` is `gk.Option[T]`: `gk.Some(..)` of a value, or the zero value for none. A read that may have
  nothing returns one; `.Get()` takes it apart (`if at, chosen := a.list.SelectedIndex(cx).Get(); chosen`).
- `usize` (counts, indices) is `int`; a negative one stops the app with a message instead of
  wrapping.
- `Duration` is `time.Duration`; `Pixels` and lengths are `gk.Length` (`gk.Px(240)`).
- A struct of options with Rust's `Default` is a Go struct whose zero value is the default:
  `gk.OpenWindowWith(cx, gk.WindowConfig{Width: 320, Height: 240, Title: "More", MinWidth: 200}, gk.NewView(nil, a.columns))`
  sets four fields and leaves the rest.
- An `IndexPath` (a row of a list, a select or a combobox) is `gk.Row(n)`, or the full
  `gk.BaseIndexPath{Section: 1, Row: 4}` for a list with sections; an `Option<IndexPath>` is
  `gk.Some(gk.Row(4))` or `gk.NoRow`: `a.list.SetSelectedIndex(cx, gk.Some(gk.Row(4)))`,
  `a.list.ScrollToItem(cx, gk.Row(4), gk.ScrollStrategyTop)`, `a.list.SetSelectedIndex(cx, gk.NoRow)`.
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
| `cx.new(\|cx\| InputState::new(window, cx).placeholder("Name"))` | `gk.NewInputState(cx).Placeholder("Name").Build()`, in `init`, kept in your struct |
| `Input::new(&self.name)` | `gk.Input(a.name)` |
| `self.name.read(cx).value()` | `a.name.Value(cx)` (a Go string, the app's to keep) |
| `self.name.update(cx, \|s, cx\| s.set_value("", window, cx))` | `a.name.SetValue(cx, "")` (applied when the handler returns) |
| `cx.subscribe(&self.name, \|this, _, e: &InputEvent, cx\| ..)` | `a.name.OnInputEvent(cx, a.nameEvent)` |
| `.on_click(cx.listener(\|this, e: &ClickEvent, window, cx\| ..))` | `.OnClick(a.clicked)` with `func (a *App) clicked(cx *gk.Cx, e gk.ClickEvent)` |
| a closure that captures `ix` | a Go closure: `.OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.remove(cx, 3) })` |
| `cx.stop_propagation()` | `gk.StopPropagation(cx)` |
| `cx.notify()` | nothing: a handler redraws its own view; after a change other views show, `gk.Refresh(cx)` |
| `Entity<T>` read by several views | `gk.Model[T]`: `a.count.Get()` in a view records it, `a.count.Set(cx, a.count.Peek()+1)` redraws exactly those views |
| `actions!(app, [Save])`, `KeyBinding::new("ctrl-s", Save, None)`, `.on_action(..)` | `gk.Shortcut(cx, "ctrl-s", a.saveNow)`, which returns the `gk.Action` a menu item takes |
| `ListDelegate` (a trait the app implements for a List) | `gk.NewListState(cx, gk.NewListSource(cx, func() int { return len(a.items) }, a.itemRow)).Build()` |
| `window.open_dialog(cx, \|dialog, window, cx\| ..)` | `gk.OpenAlertDialog(cx, a.ask)` over `a.ask = gk.NewBuilt(cx, a.confirm)` made in `init` (or `gk.OpenDialog` for a `gk.DialogEl`) |
| `window.push_notification("Saved", cx)` | `gk.PushNotification(cx, "Saved")` |
| `Theme::change(ThemeMode::Dark, ..)` | `gk.ThemeChange(cx, gk.ThemeModeDark)` |
| `cx.spawn(async ..)` / a background executor | `gk.Go(cx, ..)` with two functions: the first runs on a goroutine, the second gets its result on the UI thread; or any goroutine calling `gk.Post(func(cx *gk.Cx) { .. })` |
| `cx.on_next_frame`, a `Timer` | `gk.After(cx, 500*time.Millisecond, a.tick)`, `gk.Every`, `gk.OnFrame` |
| `WindowOptions { app_id: Some("notes".into()), icon: Some(..), .. }` | `gk.OpenWindowWith(cx, gk.WindowConfig{Title: "Notes", AppID: "notes", Icon: pngBytes}, root)` |
| `window.on_window_should_close(cx, \|window, cx\| ..)` returning false | `gk.OnWindowShouldClose(cx, a.win, func(cx *gk.Cx) bool { .. })`; close it later with `gk.CloseWindow` |
| `cx.on_app_quit(\|cx\| ..)` | `gk.OnAppQuit(cx, a.quitting)` |
| `cx.reduce_motion()` | `gk.ReduceMotion(cx)` (`gk.SetReduceMotion` writes it) |
| `.bg(linear_gradient(90., linear_color_stop(cyan, 0.), linear_color_stop(violet, 1.)))` | `.Bg(gk.NewLinearGradient(90, gk.NewLinearColorStop(cyan, 0), gk.NewLinearColorStop(violet, 1)))` (`.InOklab()` for `.color_space(ColorSpace::Oklab)`) |
| `impl Plot` (or a `canvas`) whose paint calls `Grid::paint`, `PathBuilder` and `paint_path`, `paint_quad` | `gk.Plot(gk.NewGrid().., gk.NewPlotArea(points, fill), gk.NewPlotLine(points, width, fill), gk.NewPlotQuad(x, y, w, h, fill, gk.QuadStyle{..}))`, points in pixels from the plot's top left |
| `.on_drag(value, \|v, _, _, cx\| cx.new(\|_\| Preview))`, `.drag_over::<T>(\|s, _, _, _\| s.bg(..))`, `.on_drop(\|v: &T, _, cx\| ..)` | `.OnDrag(gk.DragValue{ID: 7, Text: "seven"}, a.preview)`, `.DragOver(gk.NewStyleRefinementDefault().Bg(..))`, `.CanDrop(gk.DropAccept{IDs: []uint64{7}})`, `.OnDrop(func(cx *gk.Cx, d gk.DragValue) { .. })`; one value type, kinds told apart by ID |
| `.tooltip(\|window, cx\| Tooltip::new("Saved").build(window, cx))` | `.Tooltip(gk.NewKitTooltip("Saved"))`, `.Tooltip(gk.NewKitTooltipElement(el))`, or `.Tooltip(a.tipView)` (a `gk.View`) |
| `cx.set_menus(vec![Menu { name: "App".into(), items: vec![MenuItem::action("Quit", Quit)] }])` | `gk.SetMenus(cx, gk.Menu{Name: "App", Items: []gk.MenuItem{gk.MenuAction{Name: "Quit", Action: quit}}})` |
| `img(ImageSource::Render(..))`, `img(Arc::new(Image::from_bytes(ImageFormat::Png, bytes)))` | `gk.Img(gk.ImageSourceRender{Value: img})`, `img` made once by `gk.NewRenderImage(cx, gk.PixelImage{Width: w, Height: h, RGBA: px})` and given back with `gk.Release`, `gk.Img(gk.ImageSourceImage{Value: gk.ImageBytes{Format: gk.ImageFormatPng, Bytes: b}})` |

A handler is any Go function of the builder's shape: a method value (`a.clicked`) or a closure.
Its first parameter is `cx`, then the event if the builder hands one (`gk.ClickEvent`, `bool` for a
checkbox or a switch, `gk.InputEvent`, ...). The shape is the builder's parameter type, so a handler
that does not fit does not compile, and the compiler's message names the type to write. A handler
returning a value (a dialog's `OnOk`, `func(cx *gk.Cx, e gk.ClickEvent) bool`) is allowed where
upstream's closure returns one. `gk.Shortcut` takes a `func(cx *gk.Cx)` or a click handler, so one
method serves a button and its keys.

## Lifetimes: what is safe to keep

| what | valid | to keep it |
|---|---|---|
| your state struct | as long as you keep it | the binding keeps only the functions you gave it (method values hold the pointer); Go's collector keeps it alive |
| elements and values (`gk.Label(..)`, a menu, a column) | one call of the view function that made them | make them again on each call; a function that returns one is how parts are reused |
| handles: `gk.InputState`, `gk.ListState` and every kit state type, `gk.View`, `gk.Action`, `gk.Window`, `gk.Timer`, row sources | until `gk.Release(cx, a.list)` | keep them in your struct |
| strings a read or an event hands you (`a.name.Value(cx)`) | always: a Go string is a copy | assign it anywhere |
| `*gk.Cx` | the call it was given to | never; every function the binding calls gets one |
| what a closure captures | as long as the closure is kept | nothing to do: the collector keeps it alive (Odin's copy rule has no Go counterpart) |
| a library object a read or an event hands out (a temporary handle) | until the next frame | `gk.Retain` it |

Only `gk.Post` may be called from another goroutine. Breaking a rule stops the app with a message
that names it and the line; see "When the app stops" in [GETTING-STARTED-GO.md](GETTING-STARTED-GO.md).

## Not in Go yet

The Go package has 7,133 of the 7,599 functions the Odin package binds. What it lacks, by family
(`gen/out/go-report.md` in the source repository lists each item and why):

- Table sources: Odin's `gk.table_source` (the kit's DataTable over the app's rows and columns)
  has no Go counterpart yet, so `gk.NewTableState` takes a `gk.RowSource` made by hand; the index
  below shows that parameter as `delegate`.
- The dock's saved layouts, its pane tree and panel styles: Go has panels, a skinned dock area and
  its tabs (`gk.NewDockAreaSkinned`, `gk.NewPanel`, `gk.DockView`).
- Of drag and drop, `on_drag_move` and `external_drag_payload` (drags, drops, drag-over styles and
  `CanDrop` are there, as are an element's tooltip, image sources and the menu bar: see
  [GO.md](GO.md) section 7).
- Callbacks the kit makes to draw or answer per item: a calendar's custom days and its date
  arithmetic, a hover card's content, Markdown plugins and block renderers, an editor's
  highlighters, language providers and completion, an input's tokens.
- A few values: setting fields over `bool` and `f64`, `Styled::font`, image caches, scroll anchors.

A builder whose parameter type Go cannot carry yet is left out of the package, so app code cannot
call it; the few kept for a record's other fields stop with "... is not carried by the Go package
yet: leave out the builder that takes it".

## Component index

Every element, kit state and value type of the kit's `component` and `base` modules, by upstream
module: the Go type, the upstream type, how to make one, and the builders it has beyond the
styling every element shares. For a state type: how it is made (`gk.NewX(cx, ..)….Build()` in
`init`), the element that shows it, its reads, writes and events. The full list of each type's
methods, with parameter types, is in its generated file and in `go doc`.

<!-- index:begin (written by gen/gk_apidoc.py from go/gk/gen_*.go; do not edit) -->

253 element types, 141 kit state types and 91 value types, in 76 upstream modules. Styling builders every element has (`.Bg`, `.W`, `.P`, `.BorderColor`, … and the style tokens) are not repeated per type.

### component

- **`gk.IconEl`** (Icon, element): `gk.IconDefault()` or `gk.IconEmpty()` or `gk.Icon(icon)`; `.Small`, `.Large`, `.Data`, `.Path`, `.Rotate`, `.WithSize`, `.Xsmall`
- **`gk.TitleBarEl`** (TitleBar, element): `gk.TitleBar()`; `.OnCloseWindow`
- **`gk.WindowBorderEl`** (WindowBorder, element): `gk.KitWindowBorder()` or `gk.WindowBorder()` or `gk.WindowBorderDefault()`; `.ResizeHitSize`, `.ShadowSize`
- **`gk.Theme`** (Theme, kit state): `gk.KitTheme(cx)` or `gk.ThemeGlobal(cx)` or `gk.ThemeGlobalMut(cx)`; reads `.ApplySemanticConfig`, `.ColorTokens`, `.Colors`, `.CursorBlink`, `.DarkTheme`, `.FocusRing`, `.FontFamily`, `.FontSize` and 30 more; writes `.ApplyConfig`, `.ApplySemanticTokens`, `.SetColors`, `.SetCursorBlink`, `.SetDarkTheme`, `.SetFocusRing`, `.SetFontFamily`, `.SetFontSize` and 14 more
- **`gk.ThemeRegistry`** (ThemeRegistry, kit state): `gk.ThemeRegistryGlobal(cx)` or `gk.ThemeRegistryGlobalMut(cx)`; reads `.DefaultDarkTheme`, `.DefaultLightTheme`, `.DefaultThemes`, `.LoadThemesFromStr`, `.SortedThemes`, `.Themes`

### component::accordion

- **`gk.AccordionEl`** (Accordion, element): `gk.Accordion(id)`; `.Small`, `.Large`, `.Disabled`, `.Bordered`, `.Item`, `.Multiple`, `.OnToggleClick`, `.WithSize` and 1 more
- **`gk.AccordionItemEl`** (AccordionItem, element): `gk.AccordionItem()`; `.Title`, `.Icon`, `.Small`, `.Large`, `.Disabled`, `.ContentStyle`, `.Open`, `.TitleStyle` and 2 more

### component::alert

- **`gk.AlertEl`** (Alert, element): `gk.Alert(id, message)` or `gk.AlertError(id, message)` or `gk.AlertInfo(id, message)` (and 2 more); `.Title`, `.Icon`, `.Small`, `.Large`, `.Banner`, `.OnClose`, `.Visible`, `.WithSize` and 2 more

### component::attachment

- **`gk.AttachmentActionsEl`** (AttachmentActions, element): `gk.AttachmentActions()` or `gk.AttachmentActionsDefault()`
- **`gk.AttachmentContentEl`** (AttachmentContent, element): `gk.AttachmentContent()` or `gk.AttachmentContentDefault()`; `.Title`, `.Description`
- **`gk.AttachmentDescriptionEl`** (AttachmentDescription, element): `gk.AttachmentDescription(text)`; `.Status`
- **`gk.AttachmentEl`** (Attachment, element): `gk.Attachment()` or `gk.AttachmentDefault()`; `.OnClick`, `.Tooltip`, `.Content`, `.Small`, `.Large`, `.Actions`, `.Axis`, `.Media` and 6 more
- **`gk.AttachmentGroupEl`** (AttachmentGroup, element): `gk.AttachmentGroup(id)`; `.WithEdgeFade`
- **`gk.AttachmentMediaEl`** (AttachmentMedia, element): `gk.AttachmentMedia()` or `gk.AttachmentMediaDefault()`; `.Small`, `.Large`, `.Overlay`, `.Src`, `.WithSize`, `.Xsmall`
- **`gk.AttachmentTitleEl`** (AttachmentTitle, element): `gk.AttachmentTitle(text)`; `.Status`, `.WithShimmerStyle`

### component::avatar

- **`gk.AvatarEl`** (Avatar, element): `gk.Avatar()`; `.Placeholder`, `.Small`, `.Large`, `.Name`, `.Src`, `.WithSize`, `.Xsmall`
- **`gk.AvatarGroupEl`** (AvatarGroup, element): `gk.AvatarGroup()`; `.Child`, `.Children`, `.Small`, `.Large`, `.Ellipsis`, `.Limit`, `.WithSize`, `.Xsmall`

### component::badge

- **`gk.BadgeEl`** (Badge, element): `gk.Badge()`; `.Icon`, `.Small`, `.Large`, `.Color`, `.Count`, `.Dot`, `.Max`, `.WithSize` and 1 more

### component::breadcrumb

- **`gk.BreadcrumbEl`** (Breadcrumb, element): `gk.Breadcrumb()`; `.Child`, `.Children`
- **`gk.BreadcrumbItemEl`** (BreadcrumbItem, element): `gk.BreadcrumbItem(label)`; `.OnClick`, `.Disabled`

### component::bubble

- **`gk.BubbleContentEl`** (BubbleContent, element): `gk.BubbleContent()` or `gk.BubbleContentDefault()`
- **`gk.BubbleEl`** (Bubble, element): `gk.Bubble()` or `gk.BubbleDefault()`; `.Content`, `.Alignment`, `.Reactions`, `.WithVariant`
- **`gk.BubbleGroupEl`** (BubbleGroup, element): `gk.BubbleGroup()` or `gk.BubbleGroupDefault()`
- **`gk.BubbleReactionsEl`** (BubbleReactions, element): `gk.BubbleReactions()` or `gk.BubbleReactionsDefault()`; `.Action`, `.Alignment`, `.Side`

### component::button

- **`gk.ButtonEl`** (Button, element): `gk.Button(id)`; `.Label`, `.Selected`, `.Icon`, `.OnClick`, `.Tooltip`, `.Primary`, `.Ghost`, `.Outline` and 22 more
- **`gk.ButtonGroupEl`** (ButtonGroup, element): `gk.ButtonGroup(id)`; `.Child`, `.Children`, `.OnClick`, `.Primary`, `.Ghost`, `.Outline`, `.Small`, `.Large` and 14 more
- **`gk.DropdownButtonEl`** (DropdownButton, element): `gk.DropdownButton(id)`; `.Selected`, `.Primary`, `.Ghost`, `.Outline`, `.Small`, `.Large`, `.Disabled`, `.Button` and 15 more
- **`gk.ToggleEl`** (Toggle, element): `gk.Toggle(id)`; `.Label`, `.Checked`, `.Icon`, `.OnClick`, `.Tooltip`, `.Ghost`, `.Outline`, `.Small` and 5 more
- **`gk.ToggleGroupEl`** (ToggleGroup, element): `gk.ToggleGroup(id)`; `.Child`, `.Children`, `.OnClick`, `.Ghost`, `.Outline`, `.Small`, `.Large`, `.Disabled` and 5 more
- **`gk.ButtonCustomVariant`** (ButtonCustomVariant, value): `gk.NewButtonCustomVariant()`; `.Active`, `.Color`, `.Foreground`, `.Hover`, `.Shadow`

### component::calendar

- **`gk.CalendarEl`** (Calendar, element): `gk.Calendar(state)`; `.Small`, `.Large`, `.NumberOfMonths`, `.WithSize`, `.Xsmall`

### component::carousel

- **`gk.CarouselContentEl`** (CarouselContent, element): `gk.CarouselContent(state)`; `.TrackStyle`
- **`gk.CarouselEl`** (Carousel, element): `gk.Carousel(id, state)`
- **`gk.CarouselItemEl`** (CarouselItem, element): `gk.CarouselItem(id, index, state)`
- **`gk.CarouselNextEl`** (CarouselNext, element): `gk.CarouselNext(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.CarouselPaginationEl`** (CarouselPagination, element): `gk.CarouselPagination()`
- **`gk.CarouselPaginationItemEl`** (CarouselPaginationItem, element): `gk.CarouselPaginationItem(id, index, state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.CarouselPreviousEl`** (CarouselPrevious, element): `gk.CarouselPrevious(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.CarouselState`** (CarouselState, kit state): `gk.NewCarouselState(cx, itemCount)….Build()`; shown by `gk.Carousel(state)`, `gk.CarouselContent(state)`, `gk.CarouselItem(state)`, `gk.CarouselNext(state)`, `gk.CarouselPaginationItem(state)`, `gk.CarouselPrevious(state)`; built with `.WithAxis`, `.WithLooping`, `.WithSelectedIndex`; reads `.Axis`, `.HasNext`, `.HasPrevious`, `.IsLooping`, `.ItemCount`, `.SelectFirst`, `.SelectIndex`, `.SelectLast` and 3 more; writes `.SetAxis`, `.SetItemCount`, `.SetLooping`, `.SetSelectedIndex`; events `.OnCarouselEvent(cx, f)`

### component::chart

- **`gk.AreaChartEl`** (AreaChart<Datum, element): `gk.AreaChart(data)`; `.Appear`, `.AppearKey`, `.Fill`, `.Grid`, `.GridColumns`, `.GridDashed`, `.Interactive`, `.Linear` and 21 more
- **`gk.BarChartEl`** (BarChart<Datum, element): `gk.BarChart(data)`; `.Label`, `.Value`, `.Alignment`, `.Appear`, `.AppearKey`, `.Band`, `.BandCount`, `.BandTickCount` and 20 more
- **`gk.CandlestickChartEl`** (CandlestickChart<Datum, element): `gk.CandlestickChart(data)`; `.Appear`, `.AppearKey`, `.Bearish`, `.BodyWidthRatio`, `.Bullish`, `.Close`, `.Grid`, `.High` and 11 more
- **`gk.LineChartEl`** (LineChart<Datum, element): `gk.LineChart(data)`; `.Appear`, `.AppearKey`, `.Dot`, `.Grid`, `.GridColumns`, `.GridDashed`, `.Interactive`, `.Linear` and 21 more
- **`gk.PieChartEl`** (PieChart<Datum>, element): `gk.PieChart(data)`; `.Label`, `.Value`, `.Appear`, `.AppearKey`, `.Color`, `.InnerRadius`, `.InnerRadiusFn`, `.InnerRadiusRatio` and 10 more
- **`gk.RadarChartEl`** (RadarChart<Datum, element): `gk.RadarChart(data)`; `.Label`, `.Value`, `.Appear`, `.AppearKey`, `.Dot`, `.Fill`, `.Grid`, `.GridLevels` and 11 more
- **`gk.SankeyChartEl`** (SankeyChart<Datum>, element): `gk.SankeyChart(nodes, links)`; `.Appear`, `.AppearKey`, `.Interactive`, `.Iterations`, `.LabelGap`, `.LinkOpacity`, `.MinLinkWidth`, `.NodeAlign` and 9 more
- **`gk.SankeyLabel`** (SankeyLabel, value): `gk.NewSankeyLabel(text)`; `.Color`, `.FontSize`

### component::checkbox

- **`gk.CheckboxEl`** (Checkbox, element): `gk.Checkbox(id)`; `.Label`, `.Checked`, `.Selected`, `.OnClick`, `.OnChange`, `.Tooltip`, `.Small`, `.Large` and 5 more

### component::clipboard

- **`gk.ClipboardEl`** (Clipboard, element): `gk.Clipboard(id)`; `.Value`, `.Tooltip`, `.Small`, `.Large`, `.OnCopied`, `.ValueFn`, `.WithSize`, `.Xsmall`

### component::collapsible

- **`gk.CollapsibleEl`** (Collapsible, element): `gk.Collapsible()`; `.Content`, `.MotionID`, `.Open`

### component::color_picker

- **`gk.ColorPickerEl`** (ColorPicker, element): `gk.ColorPicker(state)`; `.Label`, `.Icon`, `.Small`, `.Large`, `.Anchor`, `.FeaturedColors`, `.WithSize`, `.Xsmall`
- **`gk.ColorSelectEl`** (ColorSelect, element): `gk.ColorSelect(state)`; `.Placeholder`, `.Small`, `.Large`, `.FeaturedColors`, `.WithSize`, `.Xsmall`

### component::combobox

- **`gk.CaretEl`** (Caret, element): `gk.Caret(size)`
- **`gk.ComboboxEl`** (Combobox<GkSearchDelegate>, element): `gk.Combobox(state)`; `.Icon`, `.Placeholder`, `.Footer`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.CheckIcon` and 7 more
- **`gk.ComboboxState`** (ComboboxState<GkSearchDelegate>, kit state): `gk.NewComboboxState(cx, gk.NewSearchSource(cx, items), selectedIndices)….Build()`; shown by `gk.Combobox(state)`; built with `.Multiple`, `.Searchable`; reads `.AddSelectedIndex`, `.Query`, `.RemoveSelectedIndex`, `.SelectedValue`, `.SelectedValues`; writes `.ClearSelection`, `.Focus`, `.SetItems`, `.SetQuery`, `.SetSelectedIndices`, `.SetSelectedValues`; events `.OnComboboxEvent(cx, f)`

### component::command

- **`gk.CommandEl`** (Command, element): `gk.Command(state)`; `.Placeholder`, `.OnSelect`, `.Bordered`, `.Filterable`, `.Item`, `.Items`, `.OnCancel`, `.OnConfirm` and 4 more
- **`gk.CommandState`** (CommandState, kit state): `gk.NewCommandState(cx)`; shown by `gk.Command(state)`; reads `.IsLoading`, `.MatchedCount`, `.Query`, `.SelectedIndex`; writes `.Focus`, `.SetLoading`, `.SetQuery`, `.SetSelectedIndex`
- **`gk.CommandGroup`** (CommandGroup, value): `gk.NewCommandGroup()`; `.Label`, `.Item`, `.Items`
- **`gk.CommandItem`** (CommandItem, value): `gk.NewCommandItem()` or `gk.NewCommandItemDefault()`; `.Label`, `.Child`, `.Checked`, `.Icon`, `.Disabled`, `.Action`, `.Keywords`

### component::date_picker

- **`gk.DatePickerEl`** (DatePicker, element): `gk.DatePicker(state)`; `.Placeholder`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.Cleanable`, `.NumberOfMonths`, `.Presets` and 2 more
- **`gk.DatePickerState`** (DatePickerState, kit state): `gk.NewDatePickerState(cx)….Build()` or `gk.NewDatePickerStateRange(cx)….Build()`; shown by `gk.DatePicker(state)`; built with `.DateFormat`, `.DisabledMatcher`, `.HourCycle`, `.NumberOfMonths`, `.TimePrecision`; reads `.Date`, `.DateTime`; writes `.SetDate`, `.SetDateTime`, `.SetYearRange`; events `.OnDatePickerEvent(cx, f)`
- **`gk.DateRangePreset`** (DateRangePreset, value): `gk.NewDateRangePresetDateTime(label, value)`

### component::description_list

- **`gk.DescriptionListEl`** (DescriptionList, element): `gk.DescriptionList()` or `gk.DescriptionListHorizontal()` or `gk.DescriptionListVertical()`; `.Child`, `.Children`, `.Small`, `.Large`, `.Bordered`, `.Columns`, `.Item`, `.LabelWidth` and 4 more

### component::dialog

- **`gk.AlertDialogEl`** (AlertDialog, element): `gk.AlertDialog()`; `.Title`, `.Icon`, `.Description`, `.Content`, `.Footer`, `.ButtonProps`, `.CancelText`, `.CancelVariant` and 14 more
- **`gk.DialogActionEl`** (DialogAction, element): `gk.DialogAction()`
- **`gk.DialogCloseEl`** (DialogClose, element): `gk.DialogClose()`; `.Trigger`
- **`gk.DialogContentEl`** (DialogContent, element): `gk.DialogContent()`
- **`gk.DialogDescriptionEl`** (DialogDescription, element): `gk.DialogDescription()`
- **`gk.DialogEl`** (Dialog, element): `gk.Dialog()`; `.Title`, `.Content`, `.Footer`, `.ButtonProps`, `.Centered`, `.CloseButton`, `.Entrance`, `.Keyboard` and 8 more
- **`gk.DialogFooterEl`** (DialogFooter, element): `gk.DialogFooter()`
- **`gk.DialogHeaderEl`** (DialogHeader, element): `gk.DialogHeader()`
- **`gk.DialogTitleEl`** (DialogTitle, element): `gk.DialogTitle()`
- **`gk.DialogButtonProps`** (DialogButtonProps, value): `gk.NewDialogButtonPropsDefault()`; `.CancelText`, `.CancelVariant`, `.OkText`, `.OkVariant`, `.OnCancel`, `.OnOk`, `.ShowCancel`

### component::diff

- **`gk.DiffEl`** (Diff, element): `gk.Diff(state)`; `.Annotations`, `.ChangeBackground`, `.ChangeIndicator`, `.Decorations`, `.GutterMarkers`, `.HeaderVisible`, `.HoverHighlight`, `.HunkSeparator` and 6 more
- **`gk.DiffAnnotation`** (DiffAnnotation, kit state): `gk.NewDiffAnnotationFile(cx, id, path)` or `gk.NewDiffAnnotationLine(cx, id, position)`; reads `.ID`, `.Path`, `.Position`
- **`gk.DiffConflict`** (DiffConflict, kit state): handed out by the kit (no constructor); reads `.BaseLabel`, `.BaseLines`, `.CurrentLabel`, `.CurrentLines`, `.IncomingLabel`, `.IncomingLines`, `.Lines`
- **`gk.DiffFileRead`** (DiffFile, kit state): handed out by the kit (no constructor); reads `.Additions`, `.Conflicts`, `.Deletions`, `.ExtendedHeaders`, `.HasChanges`, `.IsBinary`, `.IsTruncated`, `.Language` and 4 more
- **`gk.DiffGutterMarkerRead`** (DiffGutterMarker, kit state): handed out by the kit (no constructor); reads `.Label`, `.ID`, `.Position`
- **`gk.DiffLinePosition`** (DiffLinePosition, kit state): `gk.NewDiffLinePosition(cx, path, side, line)`; shown by `gk.NewDiffGutterMarker(state)`; reads `.Line`, `.Path`, `.Side`
- **`gk.DiffLineRangeRead`** (DiffLineRange, kit state): handed out by the kit (no constructor); reads `.End`, `.EndSide`, `.Path`, `.Side`, `.Start`
- **`gk.DiffParseError`** (DiffParseError, kit state): handed out by the kit (no constructor); reads `.Line`, `.Message`
- **`gk.DiffState`** (DiffState, kit state): `gk.NewDiffState(cx, files)….Build()`; shown by `gk.Diff(state)`; built with `.WithContextLines`, `.WithExpansionLines`, `.WithInlineMaxLineLength`, `.WithInlineUnit`, `.WithMinCollapsedLines`, `.WithMode`, `.WithSyntaxMaxLineLength`; reads `.ConflictResolution`, `.ContextLines`, `.ExpansionLines`, `.Files`, `.InlineMaxLineLength`, `.InlineUnit`, `.IsFileCollapsed`, `.MinCollapsedLines` and 5 more; writes `.ClearSyntaxHighlighter`, `.CollapseUnchanged`, `.ExpandUnchanged`, `.NextChange`, `.PreviousChange`, `.ResolveConflict`, `.ScrollToFile`, `.ScrollToLine` and 10 more; events `.OnDiffEvent(cx, f)`
- **`gk.DiffDecoration`** (DiffDecoration, value): `gk.NewDiffDecoration(range_)`; `.WithBackground`
- **`gk.DiffFile`** (DiffFile, value): `gk.NewDiffFileParseAt(patch, index)` or `gk.NewDiffFileUnchanged(path, text)` or `gk.NewDiffFileParseWithAt(patch, options, index)`; `.WithLanguage`
- **`gk.DiffGutterMarker`** (DiffGutterMarker, value): `gk.NewDiffGutterMarker(id, position)`; `.WithLabel`
- **`gk.DiffLineRange`** (DiffLineRange, value): `gk.NewDiffLineRange(path, side, start, end)`; `.WithEndSide`
- **`gk.DiffParseOptions`** (DiffParseOptions, value): `gk.NewDiffParseOptions()` or `gk.NewDiffParseOptionsDefault()`; `.TruncatedHunks`

### component::dock

- **`gk.PanelHandle`** (PanelHandle, kit state): `gk.NewPanelHandle(cx, panel)` or `gk.NewPanelHandleFromView(cx, panel)`

### component::empty

- **`gk.EmptyContentEl`** (EmptyContent, element): `gk.EmptyContent()` or `gk.EmptyContentDefault()`
- **`gk.EmptyDescriptionEl`** (EmptyDescription, element): `gk.EmptyDescription()` or `gk.EmptyDescriptionDefault()`
- **`gk.EmptyEl`** (Empty, element): `gk.Empty()` or `gk.EmptyDefault()`; `.Content`, `.Header`
- **`gk.EmptyHeaderEl`** (EmptyHeader, element): `gk.EmptyHeader()` or `gk.EmptyHeaderDefault()`; `.Title`, `.Description`, `.Media`
- **`gk.EmptyMediaEl`** (EmptyMedia, element): `gk.EmptyMedia()` or `gk.EmptyMediaDefault()`; `.WithVariant`
- **`gk.EmptyTitleEl`** (EmptyTitle, element): `gk.EmptyTitle()` or `gk.EmptyTitleDefault()`

### component::form

- **`gk.FieldEl`** (Field, element): `gk.Field()` or `gk.KitField()`; `.Label`, `.Description`, `.DescriptionFn`, `.ItemsCenter`, `.ItemsEnd`, `.ItemsStart`, `.LabelFn`, `.LabelIndent` and 2 more
- **`gk.FormEl`** (Form, element): `gk.Form()` or `gk.FormDefault()` or `gk.FormHorizontal()` (and 3 more); `.Child`, `.Children`, `.Footer`, `.Small`, `.Large`, `.Columns`, `.LabelLayout`, `.LabelTextSize` and 4 more

### component::group_box

- **`gk.GroupBoxEl`** (GroupBox, element): `gk.GroupBox()`; `.Title`, `.Footer`, `.Outline`, `.ContentStyle`, `.Fill`, `.Normal`, `.TitleStyle`, `.WithVariant`

### component::highlighter

- **`gk.StatusColors`** (StatusColors, kit state): `gk.NewStatusColorsDefault(cx)`; reads `.Created`, `.Deleted`, `.Error`, `.ErrorBackground`, `.ErrorBorder`, `.Hint`, `.HintBackground`, `.HintBorder` and 10 more
- **`gk.SyntaxColors`** (SyntaxColors, kit state): `gk.NewSyntaxColorsDefault(cx)`; reads `.Style`, `.StyleForIndex`
- **`gk.SyntaxHighlighter`** (SyntaxHighlighter, kit state): `gk.NewSyntaxHighlighter(cx, language)`; reads `.Text`, `.Language`

### component::hover_card

- **`gk.HoverCardEl`** (HoverCard, element): `gk.HoverCard(id)`; `.Anchor`, `.Appearance`, `.CloseDelay`, `.OnOpenChange`, `.OpenDelay`, `.Trigger`, `.TriggerStyle`

### component::input

- **`gk.InputEl`** (Input, element): `gk.Input(state)`; `.Selected`, `.Prefix`, `.Suffix`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.Bordered` and 13 more
- **`gk.InputGroupAddonEl`** (InputGroupAddon, element): `gk.InputGroupAddon(id)`; `.Align`
- **`gk.InputGroupButtonEl`** (InputGroupButton, element): `gk.InputGroupButton(id)`; `.Label`, `.Selected`, `.Icon`, `.OnClick`, `.Tooltip`, `.Primary`, `.Ghost`, `.Outline` and 18 more
- **`gk.InputGroupEl`** (InputGroup, element): `gk.InputGroup(id)`; `.Small`, `.Large`, `.Disabled`, `.Addon`, `.Input`, `.Invalid`, `.Readonly`, `.WithSize` and 1 more
- **`gk.InputGroupTextEl`** (InputGroupText, element): `gk.InputGroupText()` or `gk.InputGroupTextDefault()`
- **`gk.InputTokenEl`** (InputToken, element): `gk.InputToken(context)`; `.Icon`
- **`gk.NumberInputEl`** (NumberInput, element): `gk.NumberInput(state)`; `.Placeholder`, `.Prefix`, `.Suffix`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.WithSize` and 1 more
- **`gk.OtpInputEl`** (OtpInput, element): `gk.OtpInput(state)`; `.Small`, `.Large`, `.Disabled`, `.Groups`, `.WithSize`, `.Xsmall`
- **`gk.TextareaEl`** (Textarea, element): `gk.Textarea(state)`; `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.Bordered`, `.OnPaste`, `.OnTokenClick`, `.OnTokenHover` and 3 more

### component::kbd

- **`gk.KbdEl`** (Kbd, element): `gk.Kbd(stroke)`; `.Outline`, `.Appearance`

### component::label

- **`gk.LabelEl`** (Label, element): `gk.Label(label)`; `.Highlights`, `.Masked`, `.Secondary`

### component::link

- **`gk.LinkEl`** (Link, element): `gk.Link(id)`; `.OnClick`, `.Disabled`, `.Href`

### component::list

- **`gk.ListEl`** (List<GkListDelegate>, element): `gk.List(state)`; `.Small`, `.Large`, `.ScrollBounce`, `.ScrollbarVisible`, `.SearchPlaceholder`, `.WithSize`, `.Xsmall`
- **`gk.ListItemEl`** (ListItem, element): `gk.ListItem(id)`; `.Selected`, `.OnClick`, `.Suffix`, `.Disabled`, `.CheckIcon`, `.Confirmed`, `.OnMouseEnter`, `.Open` and 2 more
- **`gk.ListState`** (ListState<GkListDelegate>, kit state): `gk.NewListState(cx, gk.NewListSource(cx, count, row))….Build()`; shown by `gk.List(state)`; built with `.Searchable`, `.Selectable`; reads `.Delegate`, `.DelegateMut`, `.RightClickedIndex`, `.ScrollHandle`, `.SelectedIndex`; writes `.Focus`, `.ScrollToItem`, `.ScrollToSelectedItem`, `.SetItemToMeasureIndex`, `.SetQuery`, `.SetRightClickedIndex`, `.SetSearchable`, `.SetSelectable` and 1 more; events `.OnListEvent(cx, f)`

### component::marker

- **`gk.MarkerContentEl`** (MarkerContent, element): `gk.MarkerContent()` or `gk.MarkerContentDefault()`; `.Text`
- **`gk.MarkerEl`** (Marker, element): `gk.Marker()` or `gk.MarkerDefault()`; `.Icon`, `.Content`, `.Alignment`, `.Loading`, `.SeparatorStyle`, `.WithLoadingStyle`, `.WithShimmerStyle`, `.WithVariant`
- **`gk.MarkerIconEl`** (MarkerIcon, element): `gk.MarkerIcon()` or `gk.MarkerIconDefault()`

### component::menu

- **`gk.AppMenuBar`** (AppMenuBar, kit state): handed out by the kit (no constructor); writes `.Reload`
- **`gk.PopupMenu`** (PopupMenu, value): `gk.NewPopupMenu()`; `.Label`, `.ActionContext`, `.CheckSide`, `.ExternalLinkIcon`, `.Item`, `.Link`, `.LinkWithDisabled`, `.LinkWithIcon` and 18 more
- **`gk.PopupMenuItem`** (PopupMenuItem, value): `gk.NewPopupMenuItemSeparator()` or `gk.NewPopupMenuItem(label)` or `gk.NewPopupMenuItemElement(builder)` (and 2 more); `.Checked`, `.Icon`, `.OnClick`, `.Disabled`, `.Action`

### component::message

- **`gk.MessageAvatarEl`** (MessageAvatar, element): `gk.MessageAvatar()` or `gk.MessageAvatarDefault()`
- **`gk.MessageContentEl`** (MessageContent, element): `gk.MessageContent()` or `gk.MessageContentDefault()`; `.Bubble`
- **`gk.MessageEl`** (Message, element): `gk.Message()` or `gk.MessageDefault()`; `.Content`, `.Footer`, `.Alignment`, `.Avatar`, `.AvatarSlot`, `.Header`, `.WithStackStyle`
- **`gk.MessageFooterEl`** (MessageFooter, element): `gk.MessageFooter()` or `gk.MessageFooterDefault()`; `.ContentInset`
- **`gk.MessageGroupEl`** (MessageGroup, element): `gk.MessageGroup()` or `gk.MessageGroupDefault()`
- **`gk.MessageHeaderEl`** (MessageHeader, element): `gk.MessageHeader()` or `gk.MessageHeaderDefault()`; `.ContentInset`

### component::message_scroller

- **`gk.MessageScrollerState`** (MessageScrollerState, kit state): `gk.NewMessageScrollerState(cx, itemCount)`; reads `.Append`, `.IsFollowingTail`, `.IsScrolledUp`, `.ItemCount`, `.LogicalScrollTop`, `.Prepend`, `.RemeasureItems`, `.ScrollToItem` and 1 more; writes `.Remeasure`, `.Reset`, `.ScrollToEnd`

### component::native_menu

- **`gk.NativeMenu`** (NativeMenu, value): `gk.NewNativeMenu()` or `gk.NewNativeMenuDefault()`; `.Menu`, `.MenuWithCheck`, `.MenuWithDisabled`, `.MenuWithIcon`, `.MenuWithIconAndDisabled`, `.MenuWithIconDisabled`, `.Separator`, `.Submenu`

### component::notification

- **`gk.Notification`** (Notification, kit state): `gk.NewNotification(cx)….Build()` or `gk.NewNotificationError(cx, message)….Build()` or `gk.NewNotificationInfo(cx, message)….Build()` (and 2 more); built with `.Title`, `.Icon`, `.OnClick`, `.Autohide`, `.Delivery`, `.InAppAndSystem`, `.Message`, `.OnClose` and 3 more; writes `.Dismiss`
- **`gk.NotificationList`** (NotificationList, kit state): `gk.NewNotificationList(cx)`; reads `.Notifications`; writes `.Clear`, `.Push`

### component::pagination

- **`gk.PaginationEl`** (Pagination, element): `gk.Pagination(id)`; `.OnClick`, `.Small`, `.Large`, `.Disabled`, `.Compact`, `.CurrentPage`, `.TotalPages`, `.VisiblePages` and 2 more

### component::plot

- **`gk.CrossLineEl`** (CrossLine, element): `gk.CrossLine(point)`; `.Band`, `.Both`, `.HSpan`, `.Height`, `.Horizontal`, `.Span`, `.Width`
- **`gk.DotEl`** (Dot, element): `gk.Dot(point)`; `.Fill`, `.Halo`, `.Stroke`
- **`gk.TooltipEl`** (Tooltip, element): `gk.Tooltip(cursor, within)`; `.Title`, `.Appearance`, `.CrossLine`, `.Dots`, `.Glide`, `.PlainRow`, `.Progress`, `.Row` and 1 more

### component::popover

- **`gk.PopoverEl`** (Popover, element): `gk.Popover(id)`; `.Anchor`, `.Appearance`, `.Arrow`, `.DefaultOpen`, `.MouseButton`, `.Offset`, `.OnOpenChange`, `.Open` and 3 more

### component::progress

- **`gk.ProgressCircleEl`** (ProgressCircle, element): `gk.ProgressCircle(id)`; `.Value`, `.Small`, `.Large`, `.Color`, `.Loading`, `.WithSize`, `.Xsmall`
- **`gk.ProgressEl`** (Progress, element): `gk.Progress(id)`; `.Value`, `.Small`, `.Large`, `.Color`, `.Loading`, `.WithSize`, `.Xsmall`

### component::questionnaire

- **`gk.QuestionnaireActionsEl`** (QuestionnaireActions, element): `gk.QuestionnaireActions(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireChoiceDescriptionEl`** (QuestionnaireChoiceDescription, element): `gk.QuestionnaireChoiceDescription()` or `gk.QuestionnaireChoiceDescriptionDefault()`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireChoiceEl`** (QuestionnaireChoice, element): `gk.QuestionnaireChoice(state, item, value)`; `.Small`, `.Large`, `.ContentStyle`, `.IndicatorStyle`, `.ShortcutStyle`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireChoicesEl`** (QuestionnaireChoices, element): `gk.QuestionnaireChoices(state, item)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireDescriptionEl`** (QuestionnaireDescription, element): `gk.QuestionnaireDescription(state, item)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireEl`** (Questionnaire, element): `gk.Questionnaire(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireErrorEl`** (QuestionnaireError, element): `gk.QuestionnaireError(state, item)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireInputEl`** (QuestionnaireInput, element): `gk.QuestionnaireInput(state, item)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireItemEl`** (QuestionnaireItem, element): `gk.QuestionnaireItem(state, item)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireNextEl`** (QuestionnaireNext, element): `gk.QuestionnaireNext(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnairePreviousEl`** (QuestionnairePrevious, element): `gk.QuestionnairePrevious(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireProgressEl`** (QuestionnaireProgress, element): `gk.QuestionnaireProgress(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireSkipEl`** (QuestionnaireSkip, element): `gk.QuestionnaireSkip(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireSubmitEl`** (QuestionnaireSubmit, element): `gk.QuestionnaireSubmit(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.QuestionnaireTitleEl`** (QuestionnaireTitle, element): `gk.QuestionnaireTitle(state, item)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`

### component::radio

- **`gk.RadioEl`** (Radio, element): `gk.Radio(id)`; `.Label`, `.Checked`, `.OnClick`, `.OnChange`, `.Tooltip`, `.Small`, `.Large`, `.Disabled` and 2 more
- **`gk.RadioGroupEl`** (RadioGroup, element): `gk.RadioGroup(id)` or `gk.RadioGroupHorizontal(id)` or `gk.RadioGroupVertical(id)`; `.Child`, `.Children`, `.OnClick`, `.OnChange`, `.Disabled`, `.Layout`, `.SelectedIndex`

### component::rating

- **`gk.RatingEl`** (Rating, element): `gk.Rating(id)`; `.Value`, `.OnClick`, `.Small`, `.Large`, `.Disabled`, `.Color`, `.Max`, `.WithSize` and 1 more

### component::searchable_list

- **`gk.SearchableListItemElementEl`** (SearchableListItemElement, element): `gk.SearchableListItemElement(ix)`; `.Checked`, `.Selected`, `.Small`, `.Large`, `.Disabled`, `.CheckIcon`, `.Open`, `.SecondarySelected` and 2 more

### component::select

- **`gk.SelectEl`** (Select<GkSearchDelegate>, element): `gk.Select(state)`; `.Icon`, `.Placeholder`, `.Small`, `.Large`, `.Disabled`, `.Appearance`, `.Cleanable`, `.Empty` and 7 more
- **`gk.SelectState`** (SelectState<GkSearchDelegate>, kit state): `gk.NewSelectState(cx, gk.NewSearchSource(cx, items), selectedIndex)….Build()`; shown by `gk.Select(state)`; built with `.Searchable`; reads `.SelectedIndex`, `.SelectedValue`; writes `.Focus`, `.SetItems`, `.SetSelectedIndex`, `.SetSelectedValue`; events `.OnSelectEvent(cx, f)`

### component::separator

- **`gk.SeparatorEl`** (Separator, element): `gk.SeparatorHorizontal()` or `gk.SeparatorHorizontalDashed()` or `gk.SeparatorVertical()` (and 1 more); `.Label`, `.Color`, `.Dashed`

### component::setting

- **`gk.SettingsEl`** (Settings, element): `gk.Settings(id)`; `.Small`, `.Large`, `.DefaultSelectedIndex`, `.HeaderStyle`, `.Page`, `.Pages`, `.SidebarSizeRange`, `.SidebarStyle` and 4 more
- **`gk.SettingFieldText`** (SettingField<SharedString>, value): `gk.NewSettingFieldTextElement(element)` or `gk.NewSettingFieldTextRender(elementRender)`; `.DefaultValue`, `.Style`
- **`gk.SettingGroup`** (SettingGroup, value): `gk.NewSettingGroup()`; `.Title`, `.Description`, `.Footer`, `.Item`, `.Items`, `.Style`, `.Variant`
- **`gk.SettingItem`** (SettingItem, value): `gk.NewSettingItemRender(render)`; `.Description`, `.Disabled`, `.Keywords`, `.Layout`
- **`gk.SettingPage`** (SettingPage, value): `gk.NewSettingPage(title)`; `.Title`, `.Icon`, `.Description`, `.DefaultOpen`, `.Group`, `.Groups`, `.HeaderStyle`, `.Resettable` and 1 more

### component::sheet

- **`gk.SheetEl`** (Sheet, element): `gk.Sheet()`; `.Title`, `.Footer`, `.OnClose`, `.Overlay`, `.OverlayClosable`, `.Resizable`

### component::shimmer

- **`gk.ShimmerTextEl`** (ShimmerText, element): `gk.ShimmerText(text)`; `.Duration`, `.HighlightColor`, `.Once`, `.Reverse`, `.Spread`, `.WithShimmerStyle`
- **`gk.ShimmerStyle`** (ShimmerStyle, value): `gk.NewShimmerStyle()` or `gk.NewShimmerStyleDefault()`; `.Duration`, `.HighlightColor`, `.Once`, `.Reverse`, `.Spread`

### component::sidebar

- **`gk.GroupedSidebarEl`** (Sidebar<SidebarGroup<SidebarMenu>>, element): `gk.GroupedSidebar(id)`; `.Child`, `.Children`, `.Footer`, `.Collapsed`, `.Collapsible`, `.Header`, `.Side`
- **`gk.SidebarEl`** (Sidebar<SidebarMenu>, element): `gk.Sidebar(id)`; `.Child`, `.Children`, `.Footer`, `.Collapsed`, `.Collapsible`, `.Header`, `.Side`
- **`gk.SidebarFooterEl`** (SidebarFooter, element): `gk.SidebarFooter()`; `.Selected`, `.Collapsed`, `.Open`, `.SecondarySelected`
- **`gk.SidebarHeaderEl`** (SidebarHeader, element): `gk.SidebarHeader()` or `gk.SidebarHeaderDefault()`; `.Selected`, `.Collapsed`, `.Open`, `.SecondarySelected`
- **`gk.SidebarToggleButtonEl`** (SidebarToggleButton, element): `gk.SidebarToggleButton()`; `.OnClick`, `.Collapsed`, `.Side`
- **`gk.SidebarGroup`** (SidebarGroup<SidebarMenu>, value): `gk.NewSidebarGroup(label)`; `.Child`, `.Children`, `.Collapsed`
- **`gk.SidebarMenu`** (SidebarMenu, value): `gk.NewSidebarMenu()`; `.Child`, `.Children`, `.Collapsed`, `.Style`
- **`gk.SidebarMenuItem`** (SidebarMenuItem, value): `gk.NewSidebarMenuItem(label)`; `.Children`, `.Icon`, `.OnClick`, `.Suffix`, `.AccessibilityLabel`, `.Active`, `.ClickToOpen`, `.ClickToToggle` and 6 more

### component::skeleton

- **`gk.SkeletonEl`** (Skeleton, element): `gk.Skeleton()`; `.Secondary`

### component::slider

- **`gk.SliderEl`** (Slider, element): `gk.Slider(state)`; `.Disabled`, `.Horizontal`, `.Reverse`, `.Vertical`

### component::speech

- **`gk.SpeechButtonEl`** (SpeechButton, element): `gk.SpeechButton(state)`; `.Small`, `.Large`, `.Disabled`, `.ShowWhenUnsupported`, `.WithSize`, `.Xsmall`
- **`gk.SpeechIndicatorEl`** (SpeechIndicator, element): `gk.SpeechIndicator(state)`; `.Small`, `.Large`, `.WithSize`, `.Xsmall`
- **`gk.SpeechWaveformEl`** (SpeechWaveform, element): `gk.SpeechWaveform(state)`; `.Small`, `.Large`, `.Liquid`, `.WithSize`, `.Xsmall`
- **`gk.AudioFormat`** (AudioFormat, kit state): `gk.NewAudioFormat(cx, sampleRate, channels)`; reads `.Channels`, `.SampleRate`
- **`gk.SpeechState`** (SpeechState, kit state): `gk.NewSpeechState(cx)….Build()`; shown by `gk.SpeechButton(state)`, `gk.SpeechIndicator(state)`, `gk.SpeechWaveform(state)`; built with `.StopTimeout`, `.SystemFallback`; reads `.HasRecognizer`, `.IsAvailable`, `.Levels`, `.Status`, `.Transcript`; writes `.Cancel`, `.Start`, `.Stop`, `.Toggle`; events `.OnSpeechEvent(cx, f)`

### component::spinner

- **`gk.SpinnerEl`** (Spinner, element): `gk.Spinner()`; `.Icon`, `.Small`, `.Large`, `.Color`, `.WithSize`, `.Xsmall`

### component::status_bar

- **`gk.StatusBarEl`** (StatusBar, element): `gk.StatusBar()`

### component::stepper

- **`gk.StepperEl`** (Stepper, element): `gk.Stepper(id)`; `.OnClick`, `.Small`, `.Large`, `.Disabled`, `.Item`, `.Items`, `.Layout`, `.SelectedIndex` and 4 more
- **`gk.StepperItemEl`** (StepperItem, element): `gk.StepperItem()`; `.Icon`, `.Small`, `.Large`, `.Disabled`, `.WithSize`, `.Xsmall`

### component::switch

- **`gk.SwitchEl`** (Switch, element): `gk.Switch(id)`; `.Label`, `.Checked`, `.OnClick`, `.OnChange`, `.Tooltip`, `.Small`, `.Large`, `.Disabled` and 3 more

### component::tab

- **`gk.TabBarEl`** (TabBar, element): `gk.TabBar(id)`; `.Child`, `.Children`, `.OnClick`, `.Prefix`, `.Suffix`, `.Outline`, `.Small`, `.Large` and 11 more
- **`gk.TabEl`** (Tab, element): `gk.Tab()` or `gk.TabDefault()`; `.Label`, `.Selected`, `.Icon`, `.OnClick`, `.Prefix`, `.Suffix`, `.Outline`, `.Small` and 11 more

### component::table

- **`gk.DataTableEl`** (DataTable<GkTableDelegate>, element): `gk.DataTable(state)`; `.Small`, `.Large`, `.Bordered`, `.FixedColsBorder`, `.HeaderRowHeight`, `.ScrollBounce`, `.ScrollbarVisible`, `.Stripe` and 2 more
- **`gk.TableBodyEl`** (TableBody, element): `gk.TableBody()`; `.Child`, `.Children`, `.Small`, `.Large`, `.WithIx`, `.WithSize`, `.Xsmall`
- **`gk.TableCaptionEl`** (TableCaption, element): `gk.TableCaption()`; `.Small`, `.Large`, `.WithIx`, `.WithSize`, `.Xsmall`
- **`gk.TableCellEl`** (TableCell, element): `gk.TableCell()`; `.Small`, `.Large`, `.TextCenter`, `.TextRight`, `.WithIx`, `.WithSize`, `.Xsmall`
- **`gk.TableEl`** (Table, element): `gk.Table()`; `.Child`, `.Children`, `.Small`, `.Large`, `.WithIx`, `.WithSize`, `.Xsmall`
- **`gk.TableFooterEl`** (TableFooter, element): `gk.TableFooter()`; `.Child`, `.Children`, `.Small`, `.Large`, `.WithIx`, `.WithSize`, `.Xsmall`
- **`gk.TableHeadEl`** (TableHead, element): `gk.TableHead()`; `.Small`, `.Large`, `.TextCenter`, `.TextRight`, `.WithIx`, `.WithSize`, `.Xsmall`
- **`gk.TableHeaderEl`** (TableHeader, element): `gk.TableHeader()`; `.Child`, `.Children`, `.Small`, `.Large`, `.WithIx`, `.WithSize`, `.Xsmall`
- **`gk.TableRowEl`** (TableRow, element): `gk.TableRow()`; `.Child`, `.Children`, `.Small`, `.Large`, `.WithIx`, `.WithSize`, `.Xsmall`
- **`gk.TableState`** (TableState<GkTableDelegate>, kit state): `gk.NewTableState(cx, delegate)….Build()`; shown by `gk.DataTable(state)`; built with `.CellSelectable`, `.ColMovable`, `.ColResizable`, `.ColSelectable`, `.LoopSelection`, `.RowHeader`, `.RowSelectable`, `.SelectOnModifierClick` and 1 more; reads `.ColWidth`, `.Delegate`, `.DelegateMut`, `.Dump`, `.DumpRange`, `.FitCol`, `.Headers`, `.RightClickedCell` and 7 more; writes `.ClearSelection`, `.Refresh`, `.RefreshHeaderLayout`, `.ScrollToCol`, `.ScrollToRow`, `.SetRightClickedRow`, `.SetSelectedCell`, `.SetSelectedCol` and 2 more; events `.OnTableEvent(cx, f)`
- **`gk.TableVisibleRange`** (TableVisibleRange, kit state): `gk.NewTableVisibleRangeDefault(cx)`; reads `.Cols`, `.Rows`
- **`gk.Column`** (Column, value): `gk.NewColumnDefault()` or `gk.NewColumn(key, name)`; `.Ascending`, `.Descending`, `.Fixed`, `.FixedLeft`, `.FixedRight`, `.MaxWidth`, `.MinWidth`, `.Movable` and 9 more

### component::tag

- **`gk.TagEl`** (Tag, element): `gk.Tag()` or `gk.TagDanger()` or `gk.TagInfo()` (and 6 more); `.Outline`, `.Small`, `.Large`, `.RoundedFull`, `.WithSize`, `.WithVariant`, `.Xsmall`

### component::text

- **`gk.TextViewEl`** (TextView, element): `gk.HTML(source)` or `gk.Markdown(source)` or `gk.TextView(state)` (and 2 more); `.BlockquoteStyle`, `.InlineCodeStyle`, `.ListStyle`, `.MarkdownExtensions`, `.MarkdownMdx`, `.MaxLines`, `.Motion`, `.OnLinkClick` and 6 more
- **`gk.FrontmatterPlugin`** (FrontmatterPlugin, value): `gk.NewFrontmatterPlugin()` or `gk.NewFrontmatterPluginDefault()`; `.Setup`
- **`gk.TextViewStyle`** (TextViewStyle, value): `gk.NewTextViewStyleDefault()`; `.CodeBlock`, `.InlineCode`, `.ParagraphGap`, `.Table`, `.TableCell`, `.TableHead`

### component::time_field

- **`gk.TimeFieldEl`** (TimeField, element): `gk.TimeField(state)`; `.Small`, `.Large`, `.Disabled`, `.Invalid`, `.WithSize`, `.Xsmall`

### component::toolbar

- **`gk.ToolbarEl`** (Toolbar, element): `gk.Toolbar(id)`; `.Child`, `.Children`, `.Content`, `.Small`, `.Large`, `.Disabled`, `.Contents`, `.WithSize` and 1 more
- **`gk.ToolbarGroupEl`** (ToolbarGroup, element): `gk.ToolbarGroup(id)`; `.Label`, `.Child`, `.Children`, `.Content`, `.Small`, `.Large`, `.WithSize`, `.Xsmall`

### component::tooltip

- **`gk.KitTooltip`** (Tooltip, value): `gk.NewKitTooltip(text)` or `gk.NewKitTooltipElement(builder)`; `.Action`, `.KeyBinding`, `.Style`

### component::tree

- **`gk.TreeEl`** (Tree, element): `gk.KitTree(state, renderItem)` or `gk.Tree(state, renderItem)`; `.ScrollBounce`

### base

- **`gk.AccordionHeaderEl`** (AccordionHeader, element): `gk.AccordionHeader(trigger)`; `.Level`
- **`gk.AccordionPanelEl`** (AccordionPanel, element): `gk.AccordionPanel()`; `.KeepMounted`, `.Open`
- **`gk.AccordionTriggerEl`** (AccordionTrigger, element): `gk.AccordionTrigger(id)`; `.OnChange`, `.Disabled`, `.Open`
- **`gk.AlertDialogActionEl`** (AlertDialogAction, element): `gk.AlertDialogAction()` or `gk.AlertDialogActionDefault()`
- **`gk.AlertDialogBackdropEl`** (AlertDialogBackdrop, element): `gk.AlertDialogBackdrop()` or `gk.AlertDialogBackdropDefault()`
- **`gk.AlertDialogCancelEl`** (AlertDialogCancel, element): `gk.AlertDialogCancel()` or `gk.AlertDialogCancelDefault()`
- **`gk.AlertDialogCloseEl`** (AlertDialogClose, element): `gk.AlertDialogClose()` or `gk.AlertDialogCloseDefault()`
- **`gk.AlertDialogDescriptionEl`** (AlertDialogDescription, element): `gk.AlertDialogDescription()` or `gk.AlertDialogDescriptionDefault()`
- **`gk.AlertDialogPopupEl`** (AlertDialogPopup, element): `gk.AlertDialogPopup()` or `gk.AlertDialogPopupDefault()`
- **`gk.AlertDialogTitleEl`** (AlertDialogTitle, element): `gk.AlertDialogTitle()` or `gk.AlertDialogTitleDefault()`
- **`gk.AlertDialogTriggerEl`** (AlertDialogTrigger, element): `gk.AlertDialogTrigger(trigger)`; `.Handle`, `.OnOpen`
- **`gk.AvatarFallbackEl`** (AvatarFallback, element): `gk.AvatarFallback()` or `gk.AvatarFallbackDefault()`
- **`gk.AvatarImageEl`** (AvatarImage, element): `gk.AvatarImage(source)`
- **`gk.BaseAccordionEl`** (Accordion, element): `gk.BaseAccordion(id)`
- **`gk.BaseAccordionItemEl`** (AccordionItem, element): `gk.BaseAccordionItem()`; `.Disabled`, `.Header`, `.Open`, `.Panel`
- **`gk.BaseAlertDialogEl`** (AlertDialog, element): `gk.BaseAlertDialog()`; `.Backdrop`, `.CloseOnEscape`, `.DismissBelowY`, `.Handle`, `.OnCancel`, `.OnClose`, `.OnOk`, `.OnOpenChange` and 2 more
- **`gk.BaseAvatarEl`** (Avatar, element): `gk.BaseAvatar()` or `gk.BaseAvatarDefault()`; `.Fallback`, `.Image`
- **`gk.BaseButtonEl`** (Button, element): `gk.BaseButton(id)`; `.Selected`, `.OnClick`, `.Disabled`, `.Open`, `.SecondarySelected`, `.Styles`
- **`gk.BaseCalendarEl`** (Calendar, element): `gk.BaseCalendar(id, state)`; `.NumberOfMonths`
- **`gk.BaseCheckboxEl`** (Checkbox, element): `gk.BaseCheckbox(id)`; `.Checked`, `.OnChange`, `.Disabled`, `.Indeterminate`, `.State`, `.Styles`
- **`gk.BaseCollapsibleEl`** (Collapsible, element): `gk.BaseCollapsible()` or `gk.BaseCollapsibleDefault()`; `.Content`, `.Open`, `.Reveal`
- **`gk.BaseColorPickerEl`** (ColorPicker, element): `gk.BaseColorPicker(id)`; `.Disabled`, `.OnOpenChange`, `.Open`
- **`gk.BaseComboboxEl`** (Combobox, element): `gk.BaseCombobox(id)`; `.Disabled`, `.ContentFocusHandle`, `.FocusHandle`, `.OnConfirm`, `.OnDismiss`, `.OnOpenChange`, `.Open`
- **`gk.BaseDatePickerEl`** (DatePicker, element): `gk.BaseDatePicker(id, focusHandle)`; `.Disabled`, `.OnOpenChange`, `.Open`
- **`gk.BaseDialogCloseEl`** (DialogClose, element): `gk.BaseDialogClose()` or `gk.BaseDialogCloseDefault()`; `.Trigger`
- **`gk.BaseDialogDescriptionEl`** (DialogDescription, element): `gk.BaseDialogDescription()` or `gk.BaseDialogDescriptionDefault()`
- **`gk.BaseDialogEl`** (Dialog, element): `gk.BaseDialog()`; `.Backdrop`, `.CloseOnBackdropPress`, `.CloseOnEscape`, `.DismissBelowY`, `.Handle`, `.OnCancel`, `.OnClose`, `.OnOk` and 3 more
- **`gk.BaseDialogTitleEl`** (DialogTitle, element): `gk.BaseDialogTitle()` or `gk.BaseDialogTitleDefault()`
- **`gk.BaseHoverCardEl`** (HoverCard, element): `gk.BaseHoverCard(id)`; `.Anchor`, `.CloseDelay`, `.OnOpenChange`, `.OpenDelay`, `.Trigger`
- **`gk.BaseInputEl`** (Input, element): `gk.BaseInput(state)`; `.OnTokenClick`, `.OnTokenHover`
- **`gk.BaseLinkEl`** (Link, element): `gk.BaseLink(id)`; `.Disabled`, `.Href`, `.OnActivate`, `.OpenWith`, `.Styles`
- **`gk.BaseNumberInputEl`** (NumberInput, element): `gk.BaseNumberInput(state)`; `.Disabled`, `.ControlsRight`, `.Input`, `.OnStep`
- **`gk.BaseOtpInputEl`** (OtpInput, element): `gk.BaseOtpInput(state)`; `.Disabled`
- **`gk.BasePaginationEl`** (Pagination, element): `gk.BasePagination(id, state)`
- **`gk.BasePopoverEl`** (Popover, element): `gk.BasePopover(id)`; `.Anchor`, `.DefaultOpen`, `.MouseButton`, `.Offset`, `.OnOpenChange`, `.OnPosition`, `.Open`, `.OverlayClosable` and 1 more
- **`gk.BaseProgressEl`** (Progress, element): `gk.BaseProgress(id)`; `.Value`, `.Indeterminate`
- **`gk.BaseRadioEl`** (Radio, element): `gk.BaseRadio(id)`; `.Checked`, `.OnChange`, `.Disabled`, `.SetPosition`, `.Styles`
- **`gk.BaseRadioGroupEl`** (RadioGroup, element): `gk.BaseRadioGroup(id)`; `.Axis`
- **`gk.BaseSelectEl`** (Select, element): `gk.BaseSelect(id)`; `.Disabled`, `.ContentFocusHandle`, `.FocusHandle`, `.OnConfirm`, `.OnDismiss`, `.OnOpenChange`, `.Open`
- **`gk.BaseSheetEl`** (Sheet, element): `gk.BaseSheet()`; `.OnClose`, `.Overlay`, `.OverlayClosable`, `.Surface`
- **`gk.BaseSliderEl`** (Slider, element): `gk.BaseSlider(state)`; `.Disabled`, `.Axis`, `.Horizontal`, `.Vertical`
- **`gk.BaseSwitchEl`** (Switch, element): `gk.BaseSwitch(id)`; `.Checked`, `.OnChange`, `.Disabled`, `.Styles`
- **`gk.BaseTabEl`** (Tab, element): `gk.BaseTab(id)`; `.Selected`, `.OnClick`, `.Disabled`, `.SetPosition`, `.Styles`
- **`gk.BaseTableBodyEl`** (TableBody, element): `gk.BaseTableBody(id)`
- **`gk.BaseTableCaptionEl`** (TableCaption, element): `gk.BaseTableCaption(id)`
- **`gk.BaseTableCellEl`** (TableCell, element): `gk.BaseTableCell(id, columnIndex)`
- **`gk.BaseTableEl`** (Table, element): `gk.BaseTable(id)`; `.ColumnCount`, `.RowCount`
- **`gk.BaseTableHeadEl`** (TableHead, element): `gk.BaseTableHead(id, columnIndex)`
- **`gk.BaseTableHeaderEl`** (TableHeader, element): `gk.BaseTableHeader(id)`
- **`gk.BaseTableRowEl`** (TableRow, element): `gk.BaseTableRow(id, rowIndex)`
- **`gk.BaseTextViewEl`** (TextView, element): `gk.BaseHTML(source)` or `gk.BaseMarkdown(source)` or `gk.BaseTextView(state)` (and 2 more); `.BlockquoteStyle`, `.InlineCodeStyle`, `.ListStyle`, `.MarkdownExtensions`, `.MarkdownMdx`, `.MaxLines`, `.Motion`, `.OnLinkClick` and 5 more
- **`gk.BaseTextareaEl`** (Textarea, element): `gk.BaseTextarea(state)`; `.OnTokenClick`, `.OnTokenHover`
- **`gk.BaseTimeFieldEl`** (TimeField, element): `gk.BaseTimeField(id, state)`; `.Disabled`, `.Separator`
- **`gk.BaseToggleEl`** (Toggle, element): `gk.BaseToggle(id)`; `.OnChange`, `.Disabled`, `.Pressed`, `.Styles`
- **`gk.BaseToggleGroupEl`** (ToggleGroup, element): `gk.BaseToggleGroup(id)`; `.Axis`
- **`gk.BaseToolbarEl`** (Toolbar, element): `gk.BaseToolbar(id)`; `.Disabled`, `.Axis`
- **`gk.BaseToolbarGroupEl`** (ToolbarGroup, element): `gk.BaseToolbarGroup(id)`; `.Label`
- **`gk.BaseTooltipEl`** (Tooltip, element): `gk.BaseTooltip(id)`
- **`gk.BaseTreeEl`** (Tree, element): `gk.BaseTree(state)`; `.ListStyle`
- **`gk.CalendarItemEl`** (CalendarItem, element): `gk.CalendarItemHanded(index)`; `.ClearChildren`
- **`gk.CheckboxIndicatorEl`** (CheckboxIndicator, element): `gk.CheckboxIndicator()` or `gk.CheckboxIndicatorDefault()`; `.Checked`, `.Disabled`, `.State`, `.Styles`
- **`gk.ColorSwatchEl`** (ColorSwatch, element): `gk.ColorSwatch(id, color)`; `.Selected`, `.OnClick`, `.Disabled`
- **`gk.DialogBackdropEl`** (DialogBackdrop, element): `gk.DialogBackdrop()` or `gk.DialogBackdropDefault()`
- **`gk.DialogPopupEl`** (DialogPopup, element): `gk.DialogPopup()` or `gk.DialogPopupDefault()`
- **`gk.DialogTriggerEl`** (DialogTrigger, element): `gk.DialogTrigger(trigger)`; `.Handle`, `.OnOpen`
- **`gk.EditorEl`** (Editor, element): `gk.Editor(state)`
- **`gk.GridLayoutEl`** (GridLayout, element): `gk.GridLayout(id, state)` or `gk.KitGridLayout(id, state)`; `.Placeholder`, `.DraggingItem`, `.FocusedItem`, `.Item`, `.Items`, `.Reflow`, `.ResizeHandle`, `.RowHeight` and 2 more
- **`gk.InputBaseEl`** (InputBase, element): `gk.InputBase(id)`; `.Disabled`, `.Focused`, `.Styles`
- **`gk.MotionRevealEl`** (MotionReveal, element): `gk.MotionReveal(id, progress, child)`
- **`gk.NavPageEl`** (NavPage, element): `gk.NavPageHanded(index)`
- **`gk.NavStackEl`** (NavStack, element): `gk.NavStack(state)`; `.Transition`
- **`gk.NumberInputTextEl`** (NumberInputText, element): `gk.NumberInputText()` or `gk.NumberInputTextDefault()`
- **`gk.PopupEl`** (Popup, element): `gk.Popup(id, trigger)`; `.Content`, `.Anchor`, `.Offset`, `.OnPosition`
- **`gk.PositionerEl`** (Positioner, element): `gk.PositionerSide(triggerBounds)` or `gk.PositionerCorner(anchor, position)`; `.Align`, `.Margin`, `.Offset`, `.OnPosition`, `.Placement`, `.Position`
- **`gk.ProgressIndicatorEl`** (ProgressIndicator, element): `gk.ProgressIndicator()`
- **`gk.ProgressTrackEl`** (ProgressTrack, element): `gk.ProgressTrack()`
- **`gk.ResizablePanelEl`** (ResizablePanel, element): `gk.ResizablePanel()`; `.Fixed`, `.SizeRange`, `.Visible`
- **`gk.ResizablePanelGroupEl`** (ResizablePanelGroup, element): `gk.HResizable(id)` or `gk.KitHResizable(id)` or `gk.KitVResizable(id)` (and 2 more); `.Child`, `.Children`, `.Axis`, `.OnResize`, `.WithState`
- **`gk.ResizeHandleEl`** (ResizeHandle<Drag, element): `gk.KitResizeHandle(id, axis)` or `gk.ResizeHandle(id, axis)`; `.Inside`, `.LockScrollAxis`, `.OnDoubleClick`
- **`gk.RoundedFrameCoverEl`** (RoundedFrameCover, element): `gk.RoundedFrameCoverUniform(radius)`; `.Backdrop`, `.FrameBorder`
- **`gk.ScrollBounceEl`** (ScrollBounce, element): `gk.ScrollBounce(id, handle, child)`; `.Enabled`, `.Motion`, `.ScrollWheel`
- **`gk.ScrollableMaskEl`** (ScrollableMask<ScrollHandle>, element): `gk.ScrollableMaskVerticalWheel(scrollHandle)` or `gk.ScrollableMask(axis, scrollHandle)`; `.Debug`
- **`gk.ScrollbarEl`** (Scrollbar, element): `gk.Scrollbar(scrollHandle)` or `gk.ScrollbarHorizontal(scrollHandle)` or `gk.ScrollbarVertical(scrollHandle)`; `.Axis`, `.Mode`, `.ScrollSize`, `.Styles`, `.ViewportBounds`, `.ViewportFromLayout`
- **`gk.SelectableTextEl`** (SelectableText, element): `gk.SelectableText(id, text)` or `gk.SelectableTextWithHandle(id, handle, text)`; `.DocumentOrder`, `.SelectionColor`, `.TextStyle`
- **`gk.SliderIndicatorEl`** (SliderIndicator, element): `gk.SliderIndicator(state)`
- **`gk.SliderThumbEl`** (SliderThumb, element): `gk.SliderThumb(state)`; `.Disabled`, `.Axis`, `.Start`
- **`gk.SliderTrackEl`** (SliderTrack, element): `gk.SliderTrack(state)`; `.Disabled`, `.Axis`
- **`gk.SwitchThumbEl`** (SwitchThumb, element): `gk.SwitchThumb(checked)`; `.Disabled`, `.Styles`
- **`gk.SwitchTrackEl`** (SwitchTrack, element): `gk.SwitchTrack(id)`; `.Checked`, `.Disabled`, `.Styles`
- **`gk.TabsEl`** (Tabs, element): `gk.Tabs(id)`
- **`gk.TimeFieldSegmentEl`** (TimeFieldSegment, element): `gk.TimeFieldSegmentHanded(index)`; `.ClearChildren`
- **`gk.ToastEl`** (Toast, element): `gk.Toast(id)`; `.TransitionStatus`
- **`gk.ToastStackEl`** (ToastStack, element): `gk.ToastStack(id, state)`; `.FocusHandle`, `.Item`, `.Motion`, `.Placement`
- **`gk.TooltipPositionerEl`** (TooltipPositioner, element): `gk.TooltipPositioner(triggerBounds)`; `.Placement`
- **`gk.AutoScroll`** (AutoScroll, kit state): `gk.NewAutoScrollDefault(cx)`; reads `.Delta`, `.IsActive`; writes `.Stop`
- **`gk.BaseTheme`** (Theme, kit state): `gk.BaseThemeGlobalMut(cx)` or `gk.NewBaseThemeGlobal(cx)`; shown by `gk.NewBaseTextViewStyleFromTheme(state)`; reads `.Appearance`, `.CursorBlink`, `.Plot`, `.Resizable`, `.Scrollbar`, `.Tokens`; writes `.SetAppearance`, `.SetCursorBlink`, `.SetPlot`, `.SetResizable`, `.SetScrollbar`, `.SetTokens`
- **`gk.BaseTreeEntry`** (TreeEntry, kit state): `gk.NewBaseTreeEntry(cx, item, depth)`; reads `.Depth`, `.IsDisabled`, `.IsExpanded`, `.IsFolder`, `.IsRoot`, `.Item`
- **`gk.CalendarState`** (CalendarState, kit state): `gk.NewCalendarState(cx)….Build()`; shown by `gk.BaseCalendar(state)`, `gk.Calendar(state)`; built with `.DisabledMatcher`, `.YearRange`; reads `.ApplyDate`, `.CurrentMonth`, `.CurrentYear`, `.Date`, `.DisabledMatcherRef`, `.HasNextYearPage`, `.HasPrevYearPage`, `.NextYearPage` and 5 more; writes `.NextMonth`, `.PrevMonth`, `.SelectMonth`, `.SelectYear`, `.SetDate`, `.SetDisabledMatcher`, `.SetNumberOfMonths`, `.SetView` and 1 more; events `.OnCalendarEvent(cx, f)`
- **`gk.ColorPickerState`** (ColorPickerState, kit state): `gk.NewColorPickerState(cx)….Build()`; shown by `gk.ColorPicker(state)`, `gk.ColorSelect(state)`; built with `.DefaultValue`; reads `.Value`, `.ActiveTab`, `.CommitHex`, `.DisplayedColor`, `.HexInput`, `.IsOpen`, `.Preview`, `.PreviewHex` and 2 more; writes `.ClearPreview`, `.ClearValue`, `.PreviewColor`, `.SelectColor`, `.SetActiveTab`, `.SetOpen`, `.SetValue`, `.SyncPendingValue` and 2 more; events `.OnColorPickerEvent(cx, f)`
- **`gk.DialogHandle`** (DialogHandle, kit state): `gk.NewDialogHandle(cx, open)`; reads `.IsOpen`; writes `.Close`, `.Open`
- **`gk.Discrete`** (Discrete<f32>, kit state): `gk.NewDiscrete(cx, from, to)`; reads `.Sample`
- **`gk.GlobalState`** (GlobalState, kit state): `gk.GlobalStateGlobal(cx)` or `gk.GlobalStateGlobalMut(cx)`
- **`gk.GridLayoutState`** (GridLayoutState, kit state): `gk.NewGridLayoutState(cx)….Build()`; shown by `gk.GridLayout(state)`, `gk.KitGridLayout(state)`; built with `.Cols`, `.Compaction`, `.DefaultAreas`, `.Draggable`, `.PreventCollision`, `.Resizable`; reads `.ActiveItem`, `.Area`, `.Areas`, `.ColumnCount`, `.CompactionAxis`, `.IsCollisionPrevented`, `.IsDraggable`, `.IsResizable` and 3 more; writes `.SetAreas`, `.SetCols`, `.SetCompaction`, `.SetDraggable`, `.SetPreventCollision`, `.SetResizable`, `.SetSelectedItem`; events `.OnGridLayoutEvent(cx, f)`
- **`gk.History`** (History<SharedString>, kit state): `gk.NewHistory(cx)….Build()`; built with `.MaxEntries`; reads `.Back`, `.CanBack`, `.CanForward`, `.Current`, `.Entries`, `.Forward`, `.ForwardEntries`, `.RemoveCurrent`; writes `.Clear`, `.Push`, `.ReplaceCurrent`
- **`gk.HorizontalScrollClaim`** (HorizontalScrollClaim, kit state): handed out by the kit (no constructor); reads `.DeltaX`
- **`gk.HslaSliders`** (HslaSliders, kit state): handed out by the kit (no constructor); reads `.Alpha`, `.Hue`, `.Lightness`, `.Saturation`
- **`gk.InlineRenderContext`** (InlineRenderContext, kit state): handed out by the kit (no constructor); reads `.FontSize`, `.LineHeight`, `.RemSize`, `.TextStyle`
- **`gk.Keyframes`** (Keyframes<f32>, kit state): handed out by the kit (no constructor); reads `.IsEmpty`, `.Len`, `.Sample`
- **`gk.MarkdownNodeRead`** (MarkdownNode, kit state): handed out by the kit (no constructor); reads `.AccessibilityName`, `.AsMarkdown`, `.AsText`, `.Name`, `.SourceRange`
- **`gk.NavStackState`** (NavStackState, kit state): `gk.NewNavStackState(cx)`; shown by `gk.NavStack(state)`; reads `.Depth`, `.IsEmpty`; writes `.Clear`; events `.OnNavStackEvent(cx, f)`
- **`gk.OtpState`** (OtpState, kit state): `gk.NewOtpState(cx, length)….Build()`; shown by `gk.BaseOtpInput(state)`, `gk.OtpInput(state)`; built with `.DefaultValue`, `.Masked`; reads `.Value`, `.CursorVisible`, `.IsEmpty`, `.IsMasked`, `.Len`, `.TryValue`; writes `.Focus`, `.SetMasked`, `.SetValue`; events `.OnOtpEvent(cx, f)`
- **`gk.PlotThemeRead`** (PlotTheme, kit state): handed out by the kit (no constructor); reads `.Motion`
- **`gk.PopoverState`** (PopoverState, kit state): `gk.NewPopoverState(cx, defaultOpen)`; reads `.IsOpen`; writes `.Dismiss`, `.Show`
- **`gk.RangeHighlight`** (RangeHighlight, kit state): `gk.NewRangeHighlight(cx, range_, background)`; reads `.Background`, `.Range`
- **`gk.RenderedText`** (RenderedText, kit state): handed out by the kit (no constructor); reads `.AsStr`, `.IsEmpty`, `.Len`, `.RangeForSource`, `.Source`
- **`gk.ResizableState`** (ResizableState, kit state): `gk.NewResizableState(cx)`; reads `.ContainerSize`, `.Sizes`; writes `.Clear`, `.InsertPanel`, `.RemovePanel`, `.ResetPanel`, `.ResizePanel`; events `.OnResizablePanelEvent(cx, f)`
- **`gk.ScrollbarMotionRead`** (ScrollbarMotion, kit state): handed out by the kit (no constructor); reads `.Enter`, `.Entrance`, `.Exit`, `.Expand`, `.Idle`, `.ThumbHoverEntrance`
- **`gk.ScrollbarThemeRead`** (ScrollbarTheme, kit state): handed out by the kit (no constructor); reads `.Mode`, `.Motion`
- **`gk.Sequence`** (Sequence<f32>, kit state): `gk.NewSequence(cx, id, from)….Build()`; built with `.WithStep`, `.WithSteps`; reads `.From`, `.Sample`, `.Steps`
- **`gk.SequenceSample`** (SequenceSample<f32>, kit state): handed out by the kit (no constructor); reads `.Value`, `.IntoValue`, `.IsFinished`, `.Status`, `.Step`
- **`gk.SequenceStep`** (SequenceStep<f32>, kit state): `gk.NewSequenceStep(cx, target, transition)`; reads `.Target`
- **`gk.Stagger`** (Stagger, kit state): `gk.NewStagger(cx, interval, origin)`; reads `.Delay`
- **`gk.TaskToggleEvent`** (TaskToggleEvent, kit state): handed out by the kit (no constructor); reads `.Checked`, `.ItemRange`, `.MarkerRange`
- **`gk.TextSelectionEndpoint`** (TextSelectionEndpoint, kit state): handed out by the kit (no constructor); reads `.ContentPoint`
- **`gk.TextSelectionHandle`** (TextSelectionHandle, kit state): `gk.NewTextSelectionHandle(cx, fallbackCopyText)`; shown by `gk.SelectableTextWithHandle(state)`; reads `.HasLocalSelection`, `.Snapshot`; writes `.PaintTouchHandles`, `.SetFallbackCopyText`, `.SetLocalSelection`
- **`gk.TextSelectionProjection`** (TextSelectionProjection, kit state): `gk.NewTextSelectionProjectionDefault(cx)`
- **`gk.TextSelectionSnapshot`** (TextSelectionSnapshot, kit state): handed out by the kit (no constructor); reads `.Anchor`, `.Coverage`, `.Cursor`, `.IsSelecting`, `.WindowPoints`
- **`gk.TextSelectionWindowPoints`** (TextSelectionWindowPoints, kit state): handed out by the kit (no constructor); reads `.Anchor`, `.Cursor`
- **`gk.TextViewDefaults`** (TextViewDefaults, kit state): `gk.NewTextViewDefaults(cx)….Build()` or `gk.NewTextViewDefaultsGlobal(cx)….Build()`; built with `.WithInheritTextColor`, `.WithStyle`; reads `.HasCodeBlockHighlighter`, `.InheritTextColor`; writes `.Install`
- **`gk.TextViewState`** (TextViewState, kit state): `gk.NewTextViewStateHTML(cx, text)….Build()` or `gk.NewTextViewStateMarkdown(cx, text)….Build()`; shown by `gk.BaseTextView(state)`, `gk.TextView(state)`; built with `.Motion`, `.Scrollable`, `.Selectable`, `.SelectionFormat`; reads `.IsClamped`, `.RenderedText`, `.RevealRange`, `.SelectedSourceRange`, `.SelectedText`, `.SetRangeHighlights`; writes `.ClearRangeHighlights`, `.ClearSelection`, `.InvalidateInlineLayout`, `.PushStr`, `.SelectAll`, `.SetMotion`, `.SetScrollable`, `.SetSelectable` and 2 more
- **`gk.TimeFieldState`** (TimeFieldState, kit state): `gk.NewTimeFieldState(cx)….Build()`; shown by `gk.BaseTimeField(state)`, `gk.TimeField(state)`; built with `.HourCycle`, `.Precision`; reads `.SelectedSegment`; writes `.Focus`, `.SetHourCycle`, `.SetPrecision`; events `.OnTimeFieldEvent(cx, f)`
- **`gk.ToastManager`** (ToastManager<u64, kit state): `gk.NewToastManager(cx, motion)`; reads `.Get`, `.IsEmpty`, `.Iter`, `.Len`, `.Visible`
- **`gk.ToastStackState`** (ToastStackState, kit state): `gk.NewToastStackStateDefault(cx)`; shown by `gk.ToastStack(state)`; reads `.IsExpanded`
- **`gk.TooltipDefaults`** (TooltipDefaults, kit state): `gk.NewTooltipDefaults(cx)….Build()` or `gk.NewTooltipDefaultsGlobal(cx)….Build()`; built with `.WithGracePeriod`, `.WithShowDelay`; reads `.GracePeriod`, `.ShowDelay`; writes `.Install`
- **`gk.TooltipOverlay`** (TooltipOverlay, kit state): `gk.NewTooltipOverlay(cx)….Build()`; writes `.Hide`, `.RequestHide`, `.RequestShow`
- **`gk.TouchSelectionSnapshot`** (TouchSelectionSnapshot, kit state): handed out by the kit (no constructor); reads `.Bounds`, `.Dragging`, `.Edge`, `.End`, `.IsEdgeVisible`, `.IsEmpty`, `.IsMenuOpen`, `.Start`
- **`gk.TransitionState`** (TransitionState<f32>, kit state): `gk.NewTransitionStateDefault(cx)`; reads `.Sample`
- **`gk.TreeState`** (TreeState, kit state): `gk.NewTreeState(cx)….Build()`; shown by `gk.BaseTree(state)`, `gk.KitTree(state)`, `gk.Tree(state)`; built with `.Items`; reads `.Entry`, `.IndexOf`, `.SelectedEntry`, `.SelectedIndex`, `.SelectedItem`; writes `.Focus`, `.RevealItem`, `.ScrollToItem`, `.SetItems`, `.SetSelectedIndex`, `.SetSelectedItem`; events `.OnTreeEvent(cx, f)`
- **`gk.UndoHistory`** (UndoHistory<SharedString>, kit state): `gk.NewUndoHistory(cx)….Build()`; built with `.GroupInterval`, `.MaxUndos`; reads `.CanRedo`, `.CanUndo`, `.IsIgnoring`, `.Redo`, `.Undo`; writes `.Clear`, `.EndGrouping`, `.Push`, `.SetIgnoring`, `.StartGrouping`
- **`gk.VirtualListScrollHandle`** (VirtualListScrollHandle, kit state): `gk.NewVirtualListScrollHandle(cx)`; reads `.BaseHandle`; writes `.ScrollToBottom`, `.ScrollToItem`
- **`gk.BaseTextViewStyle`** (TextViewStyle, value): `gk.NewBaseTextViewStyleDefault()` or `gk.NewBaseTextViewStyleFromTheme(theme)`; `.WithBlockquoteStyle`, `.WithBorder`, `.WithCodeBackground`, `.WithCodeBlock`, `.WithDark`, `.WithForeground`, `.WithInlineCode`, `.WithInlineCodeStyle` and 9 more
- **`gk.ButtonStyles`** (ButtonStyles, value): `gk.NewButtonStylesDefault()`; `.Selected`, `.Disabled`
- **`gk.CheckboxIndicatorStyles`** (CheckboxIndicatorStyles, value): `gk.NewCheckboxIndicatorStylesDefault()`; `.Checked`, `.Disabled`, `.Indeterminate`
- **`gk.CheckboxStyles`** (CheckboxStyles, value): `gk.NewCheckboxStylesDefault()`; `.Checked`, `.Disabled`, `.Indeterminate`
- **`gk.GridArea`** (GridArea, value): `gk.NewGridArea(id)`; `.Col`, `.ColSpan`, `.Locked`, `.MaxSpan`, `.MinSpan`, `.Row`, `.RowSpan`
- **`gk.GridItem`** (GridItem, value): `gk.NewGridItem(id)`; `.AccessibilityLabel`, `.DragHandleOnly`, `.Style`
- **`gk.InputStyles`** (InputStyles, value): `gk.NewInputStylesDefault()`; `.Disabled`, `.Focused`
- **`gk.LinkStyles`** (LinkStyles, value): `gk.NewLinkStylesDefault()`; `.Disabled`
- **`gk.MarkdownExtensions`** (MarkdownExtensions, value): `gk.NewMarkdownExtensionsDefault()`; `.Frontmatter`, `.Mdx`, `.ParserRevision`
- **`gk.PaginationState`** (PaginationState, value): `gk.NewPaginationState(currentPage, totalPages)`; `.OnChange`, `.Disabled`, `.VisiblePages`
- **`gk.PlotMotion`** (PlotMotion, value): `gk.NewPlotMotionDefault()`; `.WithAppear`, `.WithEnter`, `.WithExit`, `.WithPointer`
- **`gk.PlotTheme`** (PlotTheme, value): `gk.NewPlotTheme()` or `gk.NewPlotThemeDefault()`; `.WithMotion`
- **`gk.Presence`** (Presence, value): `gk.NewPresence(id, present)`; `.Transition`
- **`gk.RadioStyles`** (RadioStyles, value): `gk.NewRadioStylesDefault()`; `.Checked`, `.Disabled`
- **`gk.ScrollBounceMotion`** (ScrollBounceMotion, value): `gk.NewScrollBounceMotionDefault()`; `.WithMaxStretch`, `.WithResponse`, `.WithTracking`
- **`gk.ScrollbarMotion`** (ScrollbarMotion, value): `gk.NewScrollbarMotionDefault()`; `.WithEnter`, `.WithEntrance`, `.WithExit`, `.WithExpand`, `.WithIdle`, `.WithThumbHoverEntrance`
- **`gk.ScrollbarStyles`** (ScrollbarStyles, value): `gk.NewScrollbarStylesDefault()`; `.Thumb`, `.ThumbActive`, `.ThumbHover`, `.Track`, `.TrackActive`, `.TrackHover`
- **`gk.ScrollbarTheme`** (ScrollbarTheme, value): `gk.NewScrollbarTheme()` or `gk.NewScrollbarThemeDefault()`; `.WithMode`, `.WithMotion`, `.WithStyles`
- **`gk.ScrollbarThumbStyle`** (ScrollbarThumbStyle, value): `gk.NewScrollbarThumbStyleDefault()`; `.Bg`, `.Inset`, `.MinLength`, `.Radius`, `.Width`
- **`gk.ScrollbarTrackStyle`** (ScrollbarTrackStyle, value): `gk.NewScrollbarTrackStyleDefault()`; `.Bg`, `.BorderColor`, `.Width`
- **`gk.Spring`** (Spring, value): `gk.NewPointerSpring()` or `gk.NewSpring(response)`; `.WithDamping`, `.WithEpsilon`, `.WithTravel`
- **`gk.StateStyle`** (StateStyle, value): `gk.NewStateStyleDefault()`; `.Style`
- **`gk.SwitchStyles`** (SwitchStyles, value): `gk.NewSwitchStylesDefault()`; `.Checked`, `.Disabled`
- **`gk.SwitchThumbStyles`** (SwitchThumbStyles, value): `gk.NewSwitchThumbStylesDefault()`; `.Checked`, `.Disabled`
- **`gk.SwitchTrackStyles`** (SwitchTrackStyles, value): `gk.NewSwitchTrackStylesDefault()`; `.Checked`, `.Disabled`
- **`gk.TabStyles`** (TabStyles, value): `gk.NewTabStylesDefault()`; `.Selected`, `.Disabled`
- **`gk.TextViewMotion`** (TextViewMotion, value): `gk.NewTextViewMotionDefault()`; `.WithStreamFade`, `.WithStreamFadeEasing`, `.WithStreamFadeStagger`
- **`gk.Timing`** (Timing, value): `gk.NewTiming(duration)`; `.Delay`, `.Direction`, `.Ease`, `.Iterations`
- **`gk.ToggleStyles`** (ToggleStyles, value): `gk.NewToggleStylesDefault()`; `.Disabled`, `.Pressed`
- **`gk.TooltipRequest`** (TooltipRequest, value): `gk.NewTooltipRequest(triggerBounds, build)`; `.WithPlacement`, `.WithShowDelay`
- **`gk.Transition`** (Transition, value): `gk.NewTransition(duration)`; `.Delay`, `.Easing`, `.ReducedMotion`
- **`gk.TreeItem`** (TreeItem, value): `gk.NewTreeItem(id, label)`; `.Child`, `.Children`, `.Disabled`, `.Expanded`

### base::animation

- **`gk.EffectTransition`** (EffectTransition, value): `gk.NewEffectTransition(duration)`; `.Fade`, `.Height`, `.SlideX`, `.SlideY`, `.Width`

### base::dock

- **`gk.Dock`** (Dock, kit state): `gk.NewDock(cx, size)`; reads `.IsCollapsible`, `.IsOpen`, `.IsResizing`, `.Size`; writes `.SetCollapsible`, `.SetOpen`, `.SetResizing`, `.SetSize`
- **`gk.DockArea`** (DockArea, kit state): `gk.NewDockArea(cx, id, version)….Build()`; reads `.Bounds`, `.DockSize`, `.HasDock`, `.ID`, `.IsDockCollapsible`, `.IsDockOpen`, `.IsEmpty`, `.IsLocked` and 3 more; writes `.AddPanel`, `.AddPanelView`, `.RemoveDock`, `.RemovePanel`, `.SetCenter`, `.SetDock`, `.SetDockCollapsible`, `.SetDockSize` and 4 more; events `.OnDockEvent(cx, f)`
- **`gk.DockContext`** (DockContext, kit state): handed out by the kit (no constructor); reads `.IsCollapsible`, `.IsOpen`, `.Placement`, `.Size`; writes `.EndResize`, `.ResizeTo`, `.Toggle`
- **`gk.DockState`** (DockState, kit state): handed out by the kit (no constructor); reads `.Open`, `.Placement`, `.Size`
- **`gk.DragPanel`** (DragPanel, kit state): handed out by the kit (no constructor); reads `.DragOffset`, `.DragSessionID`, `.PreviewSize`; writes `.SetDragOffset`, `.SetPreviewSize`
- **`gk.DropIndicator`** (DropIndicator, kit state): handed out by the kit (no constructor); reads `.Bounds`, `.DragSessionID`, `.Epoch`, `.From`, `.Placement`, `.To`
- **`gk.DropPlaceholderBounds`** (DropPlaceholderBounds, kit state): `gk.NewDropPlaceholderBoundsForPlacement(cx, bounds, placement)`; reads `.Origin`, `.Size`
- **`gk.DropTarget`** (DropTarget, kit state): handed out by the kit (no constructor); reads `.Placement`
- **`gk.EditResult`** (EditResult, kit state): `gk.NewEditResultDefault(cx)`; reads `.Changed`
- **`gk.PaneTree`** (PaneTree, kit state): `gk.NewPaneTree(cx, rootKind)`; reads `.Root`, `.RootKind`; writes `.Normalize`
- **`gk.PanelRegistry`** (PanelRegistry, kit state): `gk.NewPanelRegistry(cx)` or `gk.PanelRegistryGlobal(cx)` or `gk.PanelRegistryGlobalMut(cx)`
- **`gk.TabGroup`** (TabGroup, kit state): handed out by the kit (no constructor); reads `.ActiveIx`, `.Context`, `.IsClosable`, `.IsCollapsed`, `.IsZoomed`; writes `.SelectTab`, `.ToggleZoom`; events `.OnTabGroupEvent(cx, f)`
- **`gk.TabGroupContext`** (TabGroupContext, kit state): handed out by the kit (no constructor); reads `.ActiveIx`, `.DragPanel`, `.DropIndicator`, `.IsClosable`, `.IsCollapsed`, `.IsDraggable`, `.IsDroppable`, `.IsLocked` and 1 more; writes `.DropItem`, `.DropPanel`, `.SelectTab`, `.ToggleZoom`
- **`gk.DockLayout`** (DockLayout, value): `gk.NewDockLayoutHSplit()` or `gk.NewDockLayoutTabs()` or `gk.NewDockLayoutVSplit()`; `.Child`, `.ActiveIndex`, `.Panel`, `.PanelView`
- **`gk.DockSizing`** (DockSizing, value): `gk.NewDockSizing(placement)`; `.WithAreaBounds`, `.WithAreaHeight`, `.WithAreaWidth`, `.WithOppositeDockSize`

### base::input

- **`gk.CodeActionMenuState`** (CodeActionMenuState, kit state): `gk.NewCodeActionMenuStateDefault(cx)`; reads `.Revision`
- **`gk.CompletionMenuState`** (CompletionMenuState, kit state): `gk.NewCompletionMenuStateDefault(cx)`; reads `.Revision`
- **`gk.CursorBlinkRead`** (CursorBlink, kit state): handed out by the kit (no constructor); reads `.IdleTimeout`, `.Interval`, `.IsSteady`
- **`gk.DiagnosticSet`** (DiagnosticSet, kit state): handed out by the kit (no constructor); reads `.ForOffset`, `.IsEmpty`, `.Iter`, `.Len`, `.Range`; writes `.Clear`, `.Extend`, `.Push`
- **`gk.DisplayMap`** (DisplayMap, kit state): handed out by the kit (no constructor); reads `.Text`, `.BufferLineCount`, `.BufferLineToDisplayRow`, `.BufferLineToDisplayRowRange`, `.BufferPosToDisplayPos`, `.DisplayPosToBufferPos`, `.DisplayRowCount`, `.DisplayRowToBufferLine` and 9 more; writes `.ClearFolds`, `.OnLayoutChanged`, `.SetFoldCandidates`, `.SetFolded`, `.SetWrappingIndent`, `.ToggleFold`
- **`gk.EditorScrollbarContext`** (EditorScrollbarContext, kit state): handed out by the kit (no constructor); reads `.Axis`, `.ScrollViewportSize`
- **`gk.EditorState`** (EditorState, kit state): `gk.NewEditorState(cx)….Build()`; shown by `gk.Editor(state)`; built with `.Placeholder`, `.Autocapitalize`, `.Autocorrect`, `.CleanOnEscape`, `.ContextMenu`, `.CursorSurroundingLines`, `.DefaultValue`, `.GutterDecorationsWidth` and 6 more; reads `.Text`, `.Value`, `.ContextMenuCapabilities`, `.CreateDecorationsCollection`, `.CreateGutterDecorationsCollection`, `.CreateOverviewRulerDecorationsCollection`, `.CreateRangeDecorationsCollection`, `.Cursor` and 52 more; writes `.AddSelection`, `.ApplyHighlighterFoldCandidates`, `.BeginEdgeDrag`, `.Clean`, `.ClearDiagnosticPopover`, `.ClearHoverState`, `.CloseEditMenu`, `.CloseSearch` and 54 more; events `.OnInputEditsEvent(cx, f)`, `.OnInputEvent(cx, f)`
- **`gk.GutterDecorationCollection`** (GutterDecorationCollection, kit state): `gk.NewGutterDecorationCollectionDefault(cx)`; reads `.GetRanges`; writes `.Append`, `.Clear`, `.Dispose`, `.Set`
- **`gk.InlineTokenClickEvent`** (InlineTokenClickEvent, kit state): handed out by the kit (no constructor); reads `.Bounds`, `.Click`, `.Range`, `.Token`
- **`gk.InlineTokenContext`** (InlineTokenContext, kit state): handed out by the kit (no constructor); shown by `gk.InputToken(state)`; reads `.AvailableWidth`, `.IsDisabled`, `.IsReadonly`, `.IsSelected`, `.LineHeight`, `.Range`, `.Token`
- **`gk.InlineTokenHoverEvent`** (InlineTokenHoverEvent, kit state): handed out by the kit (no constructor); reads `.Bounds`, `.IsHovered`, `.Range`, `.RangeUtf16`, `.Token`
- **`gk.InlineTokenRead`** (InlineToken, kit state): handed out by the kit (no constructor); reads `.Label`, `.Text`, `.ID`
- **`gk.InlineTokenSpan`** (InlineTokenSpan, kit state): handed out by the kit (no constructor); reads `.Range`, `.Token`
- **`gk.InputContent`** (InputContent, kit state): `gk.NewInputContent(cx, text)`; reads `.Text`, `.Tokens`
- **`gk.InputContextMenuCapabilitiesRead`** (InputContextMenuCapabilities, kit state): handed out by the kit (no constructor); reads `.HasCodeActions`, `.HasDefinition`, `.HasSelection`, `.IsCodeEditor`, `.IsCopyable`, `.IsDisabled`, `.IsEditable`, `.IsMasked` and 1 more
- **`gk.InputPresentation`** (InputPresentation, kit state): handed out by the kit (no constructor); reads `.Placeholder`, `.FocusHandle`, `.IsCodeEditor`, `.IsDisabled`, `.IsEditable`, `.IsLoading`, `.IsMasked`, `.IsMultiLine` and 3 more
- **`gk.InputState`** (InputState, kit state): `gk.NewInputState(cx)….Build()`; shown by `gk.BaseInput(state)`, `gk.BaseNumberInput(state)`, `gk.Input(state)`, `gk.NewQuestionnaireInputDefinition(state)`, `gk.NumberInput(state)`; built with `.Placeholder`, `.Autocapitalize`, `.Autocorrect`, `.CleanOnEscape`, `.ContextMenu`, `.CursorSurroundingLines`, `.DefaultValue`, `.InputAction` and 7 more; reads `.Text`, `.Value`, `.Content`, `.ContextMenuCapabilities`, `.Cursor`, `.DiagnosticPopover`, `.Diagnostics`, `.DisplayMapBufferLineCount` and 47 more; writes `.AddSelection`, `.ApplyHighlighterFoldCandidates`, `.BeginEdgeDrag`, `.Clean`, `.CloseEditMenu`, `.CloseSearch`, `.EndEdgeDrag`, `.EnsureHighlighterFactory` and 41 more; events `.OnInputEditsEvent(cx, f)`, `.OnInputEvent(cx, f)`
- **`gk.OverviewRulerDecorationCollection`** (OverviewRulerDecorationCollection, kit state): `gk.NewOverviewRulerDecorationCollectionDefault(cx)`; reads `.GetRanges`; writes `.Append`, `.Clear`, `.Dispose`, `.Set`
- **`gk.RangeDecorationCollection`** (RangeDecorationCollection, kit state): handed out by the kit (no constructor); reads `.GetRanges`; writes `.Append`, `.Clear`, `.Dispose`, `.Set`
- **`gk.SearchMatcher`** (SearchMatcher, kit state): `gk.NewSearchMatcher(cx)`; reads `.Label`, `.Current`, `.CurrentMatchIndex`, `.IsEmpty`, `.Len`, `.MatchedRanges`; writes `.UpdateCursorByOffset`, `.UpdateQuery`
- **`gk.SearchSession`** (SearchSession, kit state): `gk.NewSearchSessionDefault(cx)`; reads `.IsActive`
- **`gk.TextDecorationCollection`** (TextDecorationCollection, kit state): handed out by the kit (no constructor); reads `.GetRanges`; writes `.Append`, `.Clear`, `.Set`
- **`gk.TextareaState`** (TextareaState, kit state): `gk.NewTextareaState(cx)….Build()`; shown by `gk.BaseTextarea(state)`, `gk.Textarea(state)`; built with `.Placeholder`, `.AutoGrow`, `.Autocapitalize`, `.Autocorrect`, `.CleanOnEscape`, `.ContextMenu`, `.CursorSurroundingLines`, `.DefaultValue` and 5 more; reads `.Text`, `.Value`, `.Content`, `.ContextMenuCapabilities`, `.Cursor`, `.DiagnosticPopover`, `.Diagnostics`, `.DisplayMapBufferLineCount` and 47 more; writes `.AddSelection`, `.ApplyHighlighterFoldCandidates`, `.BeginEdgeDrag`, `.Clean`, `.CloseEditMenu`, `.CloseSearch`, `.EndEdgeDrag`, `.EnsureHighlighterFactory` and 42 more; events `.OnInputEditsEvent(cx, f)`, `.OnInputEvent(cx, f)`
- **`gk.AutoClosingPair`** (AutoClosingPair, value): `gk.NewAutoClosingPair(open, close_)`; `.NotIn`
- **`gk.BracketPair`** (BracketPair, value): `gk.NewBracketPair(open, close_)`
- **`gk.CursorBlink`** (CursorBlink, value): `gk.NewCursorBlinkDefault()` or `gk.NewCursorBlinkSteady()` or `gk.NewCursorBlinkBlinking(interval)`; `.WithIdleTimeout`
- **`gk.Diagnostic`** (Diagnostic, value): `gk.NewDiagnosticDefault()`; `.WithCode`, `.WithSeverity`, `.WithSource`
- **`gk.GutterDecoration`** (GutterDecoration, value): `gk.NewGutterDecorationDefault()` or `gk.NewGutterDecoration(range_)`; `.WithColor`, `.WithFoldedColor`
- **`gk.InlineToken`** (InlineToken, value): `gk.NewInlineToken(id, text)`; `.WithLabel`
- **`gk.InputAccessibility`** (InputAccessibility, value): `gk.NewInputAccessibility(role)`; `.WithAccessibilityID`, `.WithLabel`, `.WithPlaceholder`, `.WithValueExposed`
- **`gk.LanguageConfig`** (LanguageConfig, value): `gk.NewLanguageConfigDefault()`; `.AutoCloseBefore`, `.AutoClosingPairs`, `.Brackets`, `.IndentationRules`
- **`gk.OverviewRulerDecoration`** (OverviewRulerDecoration, value): `gk.NewOverviewRulerDecorationDefault()` or `gk.NewOverviewRulerDecoration(range_)`; `.WithColor`, `.WithLane`
- **`gk.RangeDecoration`** (RangeDecoration, value): `gk.NewRangeDecoration(range_)`; `.WithColor`, `.WithStyle`

### base::plot

- **`gk.PlotAppearScopeEl`** (PlotAppearScope, element): `gk.PlotAppearScope(id, child)`
- **`gk.PathCache`** (PathCache, kit state): `gk.NewPathCacheDefault(cx)`; reads `.IsWarm`
- **`gk.PathCaches`** (PathCaches, kit state): `gk.NewPathCachesDefault(cx)`
- **`gk.Pie`** (Pie<Datum>, kit state): `gk.NewPie(cx)….Build()`; built with `.Value`, `.EndAngle`, `.PadAngle`, `.StartAngle`
- **`gk.PlotAppear`** (PlotAppear, kit state): `gk.NewPlotAppearComplete(cx)`; reads `.IsAppearing`, `.Progress`, `.Staggered`
- **`gk.PlotLabel`** (PlotLabel, kit state): `gk.NewPlotLabel(cx, items)`; writes `.Paint`
- **`gk.SankeyGraph`** (SankeyGraph, kit state): `gk.NewSankeyGraphDefault(cx)` or `gk.BaseSankeyLayoutFrom(cx, value, graph)`; reads `.LayerCount`
- **`gk.ScaleBand`** (ScaleBand<SharedString>, kit state): `gk.NewScaleBand(cx, domain, range_)….Build()`; built with `.BandCount`, `.MaxBandWidth`, `.PaddingInner`, `.PaddingOuter`; reads `.BandWidth`, `.NearestIndex`, `.Step`, `.Tick`
- **`gk.ScaleLinear`** (ScaleLinear<f64>, kit state): `gk.NewScaleLinear(cx, domain, range_)`; reads `.NearestIndex`, `.Tick`
- **`gk.ScaleOrdinal`** (ScaleOrdinal<SharedString, kit state): `gk.NewScaleOrdinal(cx, domain, range_)….Build()`; built with `.Domain`, `.Range`, `.Unknown`; reads `.Map`
- **`gk.ScalePoint`** (ScalePoint<SharedString>, kit state): `gk.NewScalePoint(cx, domain, range_)`; reads `.NearestIndex`, `.Tick`, `.TickAt`
- **`gk.ShapeKey`** (ShapeKey, kit state): `gk.NewShapeKey(cx, extra)`; reads `.Finish`
- **`gk.Stack`** (Stack<Datum>, kit state): `gk.NewStack(cx)….Build()`; built with `.Value`, `.Data`, `.Keys`
- **`gk.AreaShape`** (Area<Datum>, value): `gk.NewAreaShape()` or `gk.NewAreaShapeDefault()`; `.Curve`, `.Data`, `.Fill`, `.Stroke`, `.X`, `.Y0`, `.Y1`
- **`gk.AxisText`** (AxisText, value): `gk.NewAxisText(text, tick, color)`; `.Align`, `.FontSize`
- **`gk.BarShape`** (Bar<Datum>, value): `gk.NewBarShape()` or `gk.NewBarShapeDefault()`; `.Value`, `.Alignment`, `.BandWidth`, `.Base`, `.CornerRadii`, `.Cross`, `.Data`, `.Fill`
- **`gk.BaseText`** (Text, value): `gk.NewBaseText(text, origin, color)`; `.Align`, `.FontSize`, `.FontWeight`
- **`gk.Grid`** (Grid, value): `gk.NewGrid()` or `gk.NewGridDefault()`; `.DashArray`, `.Stroke`, `.X`, `.Y`
- **`gk.LineShape`** (Line<Datum>, value): `gk.NewLineShape()` or `gk.NewLineShapeDefault()`; `.Curve`, `.Data`, `.Dot`, `.DotFill`, `.DotSize`, `.DotStroke`, `.Stroke`, `.StrokeWidth` and 2 more
- **`gk.PlotAxis`** (PlotAxis, value): `gk.NewPlotAxis()` or `gk.NewPlotAxisDefault()`; `.Stroke`, `.X`, `.XAxis`, `.XLabel`, `.XLabelSide`, `.Y`, `.YAxis`, `.YLabel` and 1 more
- **`gk.RadialLineShape`** (RadialLine<Datum>, value): `gk.NewRadialLineShape()` or `gk.NewRadialLineShapeDefault()`; `.Angle`, `.Closed`, `.Data`, `.Dot`, `.DotFill`, `.DotSize`, `.DotStroke`, `.Fill` and 3 more
- **`gk.Sankey`** (Sankey, value): `gk.NewSankey()` or `gk.NewSankeyDefault()`; `.Extent`, `.Iterations`, `.NodeAlign`, `.NodePadding`, `.NodeWidth`, `.Size`, `.ValueScale`
- **`gk.SankeyLink`** (SankeyLink, value): `gk.NewSankeyLink(source, target, value)`

### base::questionnaire

- **`gk.QuestionnaireAnswerChange`** (QuestionnaireAnswerChange, kit state): handed out by the kit (no constructor); reads `.Answer`, `.Item`, `.Status`
- **`gk.QuestionnaireAnswerRead`** (QuestionnaireAnswer, kit state): handed out by the kit (no constructor); reads `.Choices`, `.Freeform`, `.IsEmpty`
- **`gk.QuestionnaireAnswers`** (QuestionnaireAnswers, kit state): `gk.NewQuestionnaireAnswersDefault(cx)`; reads `.Get`, `.IsEmpty`, `.Iter`, `.Len`
- **`gk.QuestionnaireChoiceDefinitionRead`** (QuestionnaireChoiceDefinition, kit state): handed out by the kit (no constructor); reads `.Value`, `.Description`, `.AccessibilityLabel`, `.IsDefaultSelected`, `.IsDisabled`
- **`gk.QuestionnaireChoiceState`** (QuestionnaireChoiceState, kit state): handed out by the kit (no constructor); reads `.Value`, `.IsDisabled`, `.IsInvalid`, `.IsSelected`, `.Shortcut`
- **`gk.QuestionnaireInputDefinitionRead`** (QuestionnaireInputDefinition, kit state): handed out by the kit (no constructor); reads `.AccessibilityLabel`, `.IsDisabled`, `.State`
- **`gk.QuestionnaireItemDefinitionRead`** (QuestionnaireItemDefinition, kit state): handed out by the kit (no constructor); reads `.Description`, `.AccessibilityLabel`, `.Choices`, `.Input`, `.IsDisabled`, `.IsMultiple`, `.IsRequired`, `.Name`
- **`gk.QuestionnaireItemState`** (QuestionnaireItemState, kit state): handed out by the kit (no constructor); reads `.HasInput`, `.IsDisabled`, `.IsInvalid`, `.IsMultiple`, `.IsRequired`, `.Name`, `.Status`
- **`gk.QuestionnaireNavigationState`** (QuestionnaireNavigationState, kit state): `gk.NewQuestionnaireNavigationStateDefault(cx)`; reads `.IsConfirmable`, `.IsNextVisible`, `.IsPreviousVisible`, `.IsSkipVisible`, `.IsSubmitVisible`
- **`gk.QuestionnaireProgressState`** (QuestionnaireProgressState, kit state): handed out by the kit (no constructor); reads `.Current`, `.Total`
- **`gk.QuestionnaireState`** (QuestionnaireState, kit state): `gk.NewQuestionnaireState(cx, items)….Build()`; shown by `gk.Questionnaire(state)`, `gk.QuestionnaireActions(state)`, `gk.QuestionnaireChoice(state)`, `gk.QuestionnaireChoices(state)`, `gk.QuestionnaireDescription(state)`, `gk.QuestionnaireError(state)`, `gk.QuestionnaireInput(state)`, `gk.QuestionnaireItem(state)`, `gk.QuestionnaireNext(state)`, `gk.QuestionnairePrevious(state)`, `gk.QuestionnaireProgress(state)`, `gk.QuestionnaireSkip(state)`, `gk.QuestionnaireSubmit(state)`, `gk.QuestionnaireTitle(state)`; built with `.WithCurrentItem`, `.WithShortcuts`; reads `.ActivateChoice`, `.ActivateShortcut`, `.Answer`, `.Answers`, `.ChoiceDefinition`, `.ChoiceFocusHandle`, `.ChoiceForShortcut`, `.ChoicePosition` and 38 more; writes `.Reset`; events `.OnQuestionnaireEvent(cx, f)`
- **`gk.QuestionnaireSubmission`** (QuestionnaireSubmission, kit state): `gk.NewQuestionnaireSubmissionDefault(cx)`; reads `.Answer`, `.Items`
- **`gk.QuestionnaireSubmissionItem`** (QuestionnaireSubmissionItem, kit state): handed out by the kit (no constructor); reads `.Answer`, `.Name`, `.Status`
- **`gk.QuestionnaireAnswer`** (QuestionnaireAnswer, value): `gk.NewQuestionnaireAnswer()` or `gk.NewQuestionnaireAnswerDefault()`; `.WithChoices`, `.WithFreeform`
- **`gk.QuestionnaireChoiceDefinition`** (QuestionnaireChoiceDefinition, value): `gk.NewQuestionnaireChoiceDefinition(value, accessibilityLabel)`; `.WithDefaultSelected`, `.WithDescription`, `.WithDisabled`
- **`gk.QuestionnaireInputDefinition`** (QuestionnaireInputDefinition, value): `gk.NewQuestionnaireInputDefinition(state, accessibilityLabel)`; `.WithDisabled`
- **`gk.QuestionnaireItemDefinition`** (QuestionnaireItemDefinition, value): `gk.NewQuestionnaireItemDefinition(name, accessibilityLabel)`; `.WithChoice`, `.WithChoices`, `.WithDescription`, `.WithDisabled`, `.WithInput`, `.WithMultiple`, `.WithRequired`

### base::slider

- **`gk.SliderState`** (SliderState, kit state): `gk.NewSliderState(cx)….Build()`; shown by `gk.BaseSlider(state)`, `gk.Slider(state)`, `gk.SliderIndicator(state)`, `gk.SliderThumb(state)`, `gk.SliderTrack(state)`; built with `.DefaultValue`, `.Max`, `.Min`, `.Scale`, `.Step`; reads `.Value`, `.AccessibilityStep`, `.MaxValue`, `.MinValue`, `.StepValue`, `.TryValue`; writes `.SetValue`; events `.OnSliderEvent(cx, f)`

### base::text

- **`gk.BaseTable2`** (Table, kit state): `gk.NewBaseTable2(cx, rows, columnAligns)`; reads `.ColumnAligns`, `.Rows`
- **`gk.ImageNode`** (ImageNode, kit state): `gk.NewImageNodeDefault(cx)`; shown by `gk.NewInlineNodeImage(state)`; reads `.Title`
- **`gk.InlineNodeRead`** (InlineNode, kit state): handed out by the kit (no constructor); reads `.Text`, `.AsCustom`, `.AsImage`, `.MarkRuns`
- **`gk.LeafGeometry`** (LeafGeometry, kit state): handed out by the kit (no constructor); reads `.AtomAt`, `.Bounds`, `.CaretForPosition`, `.CaretHeight`, `.IndexForPosition`, `.IsEmpty`, `.Len`, `.LineHeight` and 2 more
- **`gk.ParagraphRead`** (Paragraph, kit state): handed out by the kit (no constructor); reads `.Children`
- **`gk.BaseCodeBlock`** (CodeBlock, value): `gk.NewBaseCodeBlockFromCode(code, lang)` or `gk.NewBaseCodeBlock(code, lang, span)`; `.WithLeafKey`, `.WithMeta`
- **`gk.FlowPosition`** (FlowPosition, value): `gk.NewFlowPositionAt(blocks, ix, isLast)`; `.First`, `.Last`
- **`gk.InlineNode`** (InlineNode, value): `gk.NewInlineNodeDefault()` or `gk.NewInlineNode(text)` or `gk.NewInlineNodeImage(image)`; `.Marks`
- **`gk.Paragraph`** (Paragraph, value): `gk.NewParagraphDefault()`; `.Push`, `.WithLeafKey`
- **`gk.RenderContext`** (RenderContext, value): `gk.NewRenderContextFromApp()` or `gk.NewRenderContext(style)`; `.OnLinkClick`, `.WithBodyFontSize`, `.WithForegroundRanges`, `.WithLinkReference`, `.WithMarkdownExtensions`, `.WithStyle`
- **`gk.TextMark`** (TextMark, value): `gk.NewTextMarkDefault()`; `.Bold`, `.Code`, `.Highlight`, `.Italic`, `.Link`, `.Merge`, `.Strikethrough`, `.Underline`

<!-- index:end -->
