# API guide: from GPUI-kit's documentation to the Odin call

GPUI-kit is documented for Rust, at gpui-kit.com/docs and in its rustdoc. The Odin package is
generated from that same API by fixed rules, so the Rust documentation is also the Odin
documentation once you know the rules. This page gives the rules, then an index of every component
the package has, generated from the package itself so it is always current. If this is your first
app, read [GETTING-STARTED.md](GETTING-STARTED.md) first.

Every Odin call on this page also appears in `docs/snippets/api/main.odin`, a program that the
project's tests compile, so none of them is out of date.
<!-- spans: docs/snippets/api/main.odin -->

## Where things are

- `import gk "gpuikit:gk"` is the whole API. One package; the app needs no other import from the
  binding.
- `odin/gk/gen_component_NAME.odin` holds the types of upstream's `gpui_kit::component::NAME`
  module, `gen_base_*.odin` the unstyled primitives of `gpui_kit::base`, `gen_state.odin` every kit
  state type (handles), `gen_types.odin` the enums and records, `gen_style.odin` the style tokens,
  theme colours and palette. Each type starts with a comment naming its upstream path
  (`// Button is gpui_kit::component::button::Button.`), followed by its builder table: one line
  per method, with its parameter types. Searching the package for the upstream name finds it.
- `rt_*.odin` and `kit_*.odin` are the hand-written runtime: `gk.run`, `gk.view`, `gk.on`,
  `gk.refresh`, `gk.post`, `gk.shortcut`, `gk.list_source` and the other procedures that have no
  single upstream counterpart.
- Not every upstream function is bound yet (about 84 % are). Each one that is missing is listed,
  with the reason, in [STATUS.md](STATUS.md) and `api/coverage.json` of the source repository.

## Names

| Rust (gpui-kit.com/docs) | Odin | rule |
|---|---|---|
| `Button::new("ok").primary().label("OK")` | `gk.button("ok")->primary()->label("OK")` | `Type::new` is the type's name in snake case; methods keep their names and chain with `->` |
| `Tag::secondary()` | `gk.tag_secondary()` | any other constructor: type, underscore, constructor name |
| `v_flex()`, `h_flex()`, `div()` | `gk.v_flex()`, `gk.h_flex()`, `gk.div()` | free functions keep their names |
| `InputState`, `ListEvent` | `gk.Input_State`, `gk.List_Event` | types are Ada_Case |
| `IconName::StarFill` | `.Star_Fill` (`gk.Icon_Name`) | enum variants are Ada_Case; Odin infers the enum from the parameter |
| `Switch::new("wifi")` | `gk.switch_("wifi")` | a name that is an Odin keyword gets a trailing underscore (`switch_`, `len_`, `type_`) |
| `gpui_kit::base::Checkbox` beside `gpui_kit::component::Checkbox` | `gk.Base_Checkbox`, `gk.base_checkbox("raw")` | when two crates have a type of one name, the component keeps it; base gets `Base_`, gpui `Gpui_`, the kit's root `Kit_` |
| `InputEvent::PressEnter` | `gk.Input_Event_Press_Enter` | an enum with data is a union of one struct per variant: `#partial switch v in e` or `e.(gk.Input_Event_Press_Enter)` |
| `ListEvent::Confirm(IndexPath)` | `c.row` on a `gk.List_Event_Confirm` | a tuple variant's fields are `v0`, `v1`, ...; a single record field is reachable directly (`using`) |
| `impl Into<ThemeMode>` | `gk.Theme_Mode.Dark` | a parameter that takes several types (`Theme_Mode_Into`) needs the value's type written out |

## Styles are tokens

GPUI's style methods without arguments (`size_full()`, `gap_2()`, `items_center()`, `text_sm()`,
`line_through()`) are members of one enum, `gk.Style_Flag`, given to any constructor or to
`->style(..)`:

| Rust | Odin |
|---|---|
| `div().size_full().gap_2().items_center()` | `gk.div(.Size_Full, .Gap_2)->style(.Items_Center)` |
| `.w(px(240.)).h(rems(2.))` | `->w(gk.px(240))->h(gk.rems(2))` |
| `.w(relative(0.5))` | `->w(gk.relative(0.5))` |
| `.bg(cx.theme().muted)` | `->bg(gk.theme(.Muted))` |
| `.text_color(cx.theme().warning)` | `->text_color(gk.theme(.Warning))` |
| `.border_color(blue_500())` | `->border_color(gk.palette(.Blue_500))` |
| `.bg(hsla(0.6, 0.5, 0.5, 1.))` | `->bg(gk.hsla(0.6, 0.5, 0.5, 1))` |
| `.font_bold()` | `gk.label("bold", .Font_Bold)` |

Methods with arguments stay methods. A length is `gk.px`, `gk.rems`, `gk.relative` or `gk.auto()`.
A theme colour is resolved by the library, so a view using `gk.theme(..)` follows
`gk.theme_change`. The kit's own style helpers without arguments (`StyledExt`: `font_bold`,
`font_semibold`, `h_flex`, `debug_red`, ...) are tokens too (`.Font_Bold`); their method form still
compiles.

Rust's `.when(cond, |this| this.bg(..))` is an ordinary `if` in Odin, since a chain is a value:

<!-- from: docs/snippets/api/main.odin -->
```odin
	if a.dark {
		row = row->bg(gk.theme(.Muted))
	}
```

## Options and defaults

- Every builder method is optional; leaving it out leaves upstream's default, exactly as in Rust.
- `impl Into<SharedString>` and `&str` are `string`; the binding copies it.
- `impl Into<ElementId>` is `gk.Id`: a string (`gk.button("ok")`) or a name and a number
  (`gk.button(gk.id("row", 3))`).
- `Option<T>` is `Maybe(T)`: pass a value or `nil`.
- `usize` (counts, indices) is `int`; a negative one stops the app with a message instead of
  wrapping.
- `Duration` is `time.Duration`; `Pixels` and lengths are `gk.Length` (`gk.px(240)`).
- A struct of options with Rust's `Default` is an Odin struct whose zero value is the default:
  `gk.open_window_with(cx, {width = 320, height = 240, title = "More", min_width = 200}, gk.view(a, nil, columns))`
  sets four fields and leaves the rest.
- An `IndexPath` (a row of a list, a select or a combobox) is the row alone where a call takes one
  (`a.list->set_selected_index(cx, 4)`, `a.list->scroll_to_item(cx, 4, .Top)`), or the full
  `gk.Base_Index_Path{section = 1, row = 4}`; an `Option<IndexPath>` also takes `nil`, a list of
  them `[]int{0, 2}`. Reads and events hand back the full record (`c.row`).
- A closure parameter is a handler (below) or, where the kit draws a part of a component per item
  (a calendar's day, a tree's row), an item procedure given with `gk.items`; the builder's line
  in the generated file names the procedure to write.

## State, handles and handlers

| Rust | Odin |
|---|---|
| `struct App { .. }` with `impl Render` | your struct, and a view procedure `proc(a: ^App, cx: ^gk.Cx) -> gk.Element` |
| `cx.new(\|cx\| App { .. })` | `gk.view(a, init, view)`: the state, an optional `init`, the view procedure |
| `cx.open_window(options, \|window, cx\| ..)` | `gk.open_window(cx, "API", {640, 480}, gk.view(a, init, view))`; every other option: `gk.open_window_with` |
| `cx.new(\|cx\| InputState::new(window, cx).placeholder("Name"))` | `gk.input_state(cx)->placeholder("Name")->build()`, in `init`, kept in your struct |
| `Input::new(&self.name)` | `gk.input(a.name)` |
| `self.name.read(cx).value()` | `a.name->value(cx)` (a `gk.Temp_String`, lent until the handler returns) |
| `self.name.update(cx, \|s, cx\| s.set_value("", window, cx))` | `a.name->set_value(cx, "")` (applied when the handler returns) |
| `cx.subscribe(&self.name, \|this, _, e: &InputEvent, cx\| ..)` | `a.name->on_input_event(cx, gk.on(name_event))` |
| `.on_click(cx.listener(\|this, e: &ClickEvent, window, cx\| ..))` | `->on_click(gk.on(clicked))` with `clicked :: proc(a: ^App, cx: ^gk.Cx, e: gk.Click_Event)` |
| a closure that captures `ix` | `gk.on(remove, 3)`: the value is copied and handed after `cx` (or after the event) |
| `cx.stop_propagation()` | `gk.stop_propagation(cx)` |
| `cx.notify()` | nothing: a handler redraws its own view; after a change other views show, `gk.refresh(cx)` |
| `actions!(app, [Save])`, `KeyBinding::new("ctrl-s", Save, None)`, `.on_action(..)` | `gk.shortcut(cx, "ctrl-s", gk.on(save))`, which returns the `gk.Action` a menu item takes |
| `ListDelegate` (a trait the app implements for a List) | `gk.list_state(cx, gk.list_source(cx, a, item_count, item_row))->build()` |
| `window.open_dialog(cx, \|dialog, window, cx\| ..)` | `gk.open_alert_dialog(cx, gk.built(cx, a, confirm))` (or `gk.open_dialog` for a `gk.Dialog`) |
| `window.push_notification("Saved", cx)` | `gk.push_notification(cx, "Saved")` |
| `cx.set_global(Theme..)`, `Theme::change(ThemeMode::Dark, ..)` | `gk.theme_change(cx, gk.Theme_Mode.Dark)` |
| `cx.spawn(async ..)` / a background executor | a `core:thread` thread that calls `gk.post(loaded, Loaded{names})`; `loaded` runs on the UI thread |
| `cx.on_next_frame`, a `Timer` | `gk.after(cx, 500, gk.on(tick))`, `gk.every`, `gk.on_frame` |
| `WindowOptions { app_id: Some("notes".into()), icon: Some(..), .. }` | `gk.open_window_with(cx, {title = "Notes", app_id = "notes", icon = png}, root)` (the icon a PNG's bytes) |
| `window.on_window_should_close(cx, \|window, cx\| ..)` returning false | `gk.on_window_should_close(cx, w, gk.on(ask_close))` with `ask_close :: proc(a: ^App, cx: ^gk.Cx) -> bool`; close it later with `gk.close_window` |
| `cx.on_app_quit(\|cx\| ..)` | `gk.on_app_quit(cx, gk.on(quitting))` |
| `cx.reduce_motion()` | `gk.reduce_motion(cx)` (`gk.set_reduce_motion` writes it) |
| `.bg(linear_gradient(90., linear_color_stop(cyan, 0.), linear_color_stop(violet, 1.)))` | `->bg(gk.linear_gradient(90, gk.linear_color_stop(cyan, 0), gk.linear_color_stop(violet, 1)))` (`oklab = true` for `.color_space(ColorSpace::Oklab)`) |
| `impl Plot` (or a `canvas`) whose paint calls `Grid::paint`, `PathBuilder` and `paint_path`, `paint_quad` | `gk.plot(gk.grid()->y(ys), gk.plot_area(points, fill), gk.plot_line(points, 2.5, fill), gk.plot_quad({x, y, w, h}, fill))`, points in pixels from the plot's top left |

A handler's first parameter is the state of the view it is given in (for a shortcut, a timer or a
post: the view whose `init` or handler made it, or the state given to `gk.run`). Its second is
`cx`. Then, in order, the event (if the builder hands one and you want it) and the value given to
`gk.on`. A handler that does not fit the builder stops the app at the `gk.on` line, naming the
procedure to write. A handler returning a value (a dialog's `on_ok`, `proc(..) -> bool`) is allowed
where upstream's closure returns one.

## Lifetimes: what is safe to keep

| what | valid | to keep it |
|---|---|---|
| your state struct | as long as you keep it | the binding holds only the pointer you gave `gk.view`/`gk.run`; keep it alive (a local in `main` before `gk.run` is fine) |
| elements and values (`gk.label(..)`, a menu, a column) | one call of the view procedure that made them | make them again on each call; a procedure that returns one is how parts are reused |
| handles: `gk.Input_State`, `gk.List_State` and every kit state type, `gk.View`, `gk.Action`, `gk.Window`, `gk.Timer`, row sources | until `gk.release(cx, a.list)` | keep them in your struct |
| `gk.Temp_String` (a read like `a.name->value(cx)`, event text) | until the handler, `init` or view procedure returns | `gk.keep(text)` or `strings.clone(string(text))`; it does not assign to a `string` by itself |
| `^gk.Cx` | the call it was given to | never; every procedure the binding calls gets one |
| `context.temp_allocator` | the call it was used in | the binding frees it after each call |
| a value given to `gk.on(remove, 3)` | copied, strings too; pointers are refused | it is yours in the handler |
| a posted message | copied when posted (strings, slices, dynamic arrays) | it is the posted procedure's to keep or delete |
| a library object a read or an event hands out (a temporary handle) | until the next frame | `gk.retain` it |

Breaking a rule stops the app with a message that names it and the line; in a `-debug` build the
binding also stops at a read of kept lent memory, with a message saying what was kept. See "When the app stops" in
[GETTING-STARTED.md](GETTING-STARTED.md).

## Component index

Every element, kit state and value type of the kit's `component` and `base` modules, by upstream
module: the Odin type, the upstream type, how to make one, and the builders it has beyond the
styling every element shares. For a state type: how it is made (`->…->build()` in `init`), the
element that shows it, its reads, writes and events. The full table of each type, with parameter
types, is in its generated file.

<!-- index:begin (written by gen/gk_apidoc.py from odin/gk/gen_*.odin; do not edit) -->

254 element types, 139 kit state types and 93 value types, in 76 upstream modules. Styling builders every element has (`->bg`, `->w`, `->p`, `->border_color`, … and the style tokens) are not repeated per type.

### component

- **`gk.Icon`** (Icon, element): `gk.icon_default()` or `gk.icon_empty()` or `gk.icon(icon)`; `->small`, `->large`, `->data`, `->path`, `->rotate`, `->transform`, `->with_size`, `->xsmall`
- **`gk.Title_Bar`** (TitleBar, element): `gk.title_bar()`; `->on_close_window`
- **`gk.Window_Border`** (WindowBorder, element): `gk.kit_window_border()` or `gk.window_border()` or `gk.window_border_default()`; `->resize_hit_size`, `->shadow_size`
- **`gk.Theme`** (Theme, kit state): handed out by the kit (no constructor); reads `->apply_semantic_config`, `->apply_semantic_config_str`, `->color_tokens`, `->colors`, `->cursor_blink`, `->dark_theme`, `->focus_ring`, `->font_family` and 31 more; writes `->apply_config`, `->apply_semantic_tokens`, `->set_colors`, `->set_cursor_blink`, `->set_dark_theme`, `->set_focus_ring`, `->set_font_family`, `->set_font_size` and 14 more
- **`gk.Theme_Registry`** (ThemeRegistry, kit state): handed out by the kit (no constructor); reads `->default_dark_theme`, `->default_light_theme`, `->default_themes`, `->load_themes_from_str`, `->sorted_themes`, `->themes`

### component::accordion

- **`gk.Accordion`** (Accordion, element): `gk.accordion(id)`; `->small`, `->large`, `->disabled`, `->bordered`, `->item`, `->multiple`, `->on_toggle_click`, `->with_size` and 1 more
- **`gk.Accordion_Item`** (AccordionItem, element): `gk.accordion_item()`; `->title`, `->icon`, `->small`, `->large`, `->disabled`, `->content_style`, `->open`, `->title_style` and 2 more

### component::alert

- **`gk.Alert`** (Alert, element): `gk.alert(id, message)` or `gk.alert_error(id, message)` or `gk.alert_info(id, message)` (and 2 more); `->title`, `->icon`, `->small`, `->large`, `->banner`, `->on_close`, `->visible`, `->with_size` and 2 more

### component::attachment

- **`gk.Attachment`** (Attachment, element): `gk.attachment()` or `gk.attachment_default()`; `->on_click`, `->tooltip`, `->content`, `->small`, `->large`, `->actions`, `->axis`, `->media` and 6 more
- **`gk.Attachment_Actions`** (AttachmentActions, element): `gk.attachment_actions()` or `gk.attachment_actions_default()`
- **`gk.Attachment_Content`** (AttachmentContent, element): `gk.attachment_content()` or `gk.attachment_content_default()`; `->title`, `->description`
- **`gk.Attachment_Description`** (AttachmentDescription, element): `gk.attachment_description(text)`; `->status`
- **`gk.Attachment_Group`** (AttachmentGroup, element): `gk.attachment_group(id)`; `->with_edge_fade`
- **`gk.Attachment_Media`** (AttachmentMedia, element): `gk.attachment_media()` or `gk.attachment_media_default()`; `->small`, `->large`, `->overlay`, `->src`, `->with_size`, `->xsmall`
- **`gk.Attachment_Title`** (AttachmentTitle, element): `gk.attachment_title(text)`; `->status`, `->with_shimmer_style`

### component::avatar

- **`gk.Avatar`** (Avatar, element): `gk.avatar()`; `->placeholder`, `->small`, `->large`, `->name`, `->src`, `->with_size`, `->xsmall`
- **`gk.Avatar_Group`** (AvatarGroup, element): `gk.avatar_group()`; `->child`, `->children`, `->small`, `->large`, `->ellipsis`, `->limit`, `->with_size`, `->xsmall`

### component::badge

- **`gk.Badge`** (Badge, element): `gk.badge()`; `->icon`, `->small`, `->large`, `->color`, `->count`, `->dot`, `->max`, `->with_size` and 1 more

### component::breadcrumb

- **`gk.Breadcrumb`** (Breadcrumb, element): `gk.breadcrumb()`; `->child`, `->children`
- **`gk.Breadcrumb_Item`** (BreadcrumbItem, element): `gk.breadcrumb_item(label)`; `->on_click`, `->disabled`

### component::bubble

- **`gk.Bubble`** (Bubble, element): `gk.bubble()` or `gk.bubble_default()`; `->content`, `->alignment`, `->reactions`, `->with_variant`
- **`gk.Bubble_Content`** (BubbleContent, element): `gk.bubble_content()` or `gk.bubble_content_default()`
- **`gk.Bubble_Group`** (BubbleGroup, element): `gk.bubble_group()` or `gk.bubble_group_default()`
- **`gk.Bubble_Reactions`** (BubbleReactions, element): `gk.bubble_reactions()` or `gk.bubble_reactions_default()`; `->action`, `->alignment`, `->side`

### component::button

- **`gk.Button`** (Button, element): `gk.button(id)`; `->label`, `->child`, `->children`, `->selected`, `->icon`, `->on_click`, `->tooltip`, `->primary` and 24 more
- **`gk.Button_Group`** (ButtonGroup, element): `gk.button_group(id)`; `->child`, `->children`, `->on_click`, `->primary`, `->ghost`, `->outline`, `->small`, `->large` and 14 more
- **`gk.Dropdown_Button`** (DropdownButton, element): `gk.dropdown_button(id)`; `->selected`, `->primary`, `->ghost`, `->outline`, `->small`, `->large`, `->disabled`, `->button` and 15 more
- **`gk.Toggle`** (Toggle, element): `gk.toggle(id)`; `->label`, `->checked`, `->icon`, `->on_click`, `->tooltip`, `->ghost`, `->outline`, `->small` and 5 more
- **`gk.Toggle_Group`** (ToggleGroup, element): `gk.toggle_group(id)`; `->child`, `->children`, `->on_click`, `->ghost`, `->outline`, `->small`, `->large`, `->disabled` and 5 more
- **`gk.Button_Custom_Variant`** (ButtonCustomVariant, value): `gk.button_custom_variant()`; `->active`, `->color`, `->foreground`, `->hover`, `->shadow`

### component::calendar

- **`gk.Calendar`** (Calendar, element): `gk.calendar(state)`; `->small`, `->large`, `->first_day_of_week`, `->number_of_months`, `->with_size`, `->xsmall`

### component::carousel

- **`gk.Carousel`** (Carousel, element): `gk.carousel(id, state)`
- **`gk.Carousel_Content`** (CarouselContent, element): `gk.carousel_content(state)`; `->track_style`
- **`gk.Carousel_Item`** (CarouselItem, element): `gk.carousel_item(id, index, state)`
- **`gk.Carousel_Next`** (CarouselNext, element): `gk.carousel_next(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Carousel_Pagination`** (CarouselPagination, element): `gk.carousel_pagination()`
- **`gk.Carousel_Pagination_Item`** (CarouselPaginationItem, element): `gk.carousel_pagination_item(id, index, state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Carousel_Previous`** (CarouselPrevious, element): `gk.carousel_previous(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Carousel_State`** (CarouselState, kit state): `gk.carousel_state(cx, item_count)->…->build()`; shown by `gk.carousel(state)`, `gk.carousel_content(state)`, `gk.carousel_item(state)`, `gk.carousel_next(state)`, `gk.carousel_pagination_item(state)`, `gk.carousel_previous(state)`; built with `->with_axis`, `->with_looping`, `->with_selected_index`; reads `->axis`, `->has_next`, `->has_previous`, `->is_looping`, `->item_count`, `->select_first`, `->select_index`, `->select_last` and 3 more; writes `->set_axis`, `->set_item_count`, `->set_looping`, `->set_selected_index`; events `->on_carousel_event(cx, gk.on(p))`

### component::chart

- **`gk.Area_Chart`** (AreaChart<Datum, element): `gk.area_chart(data)`; `->appear`, `->appear_key`, `->fill`, `->grid`, `->grid_columns`, `->grid_dashed`, `->interactive`, `->linear` and 21 more
- **`gk.Bar_Chart`** (BarChart<Datum, element): `gk.bar_chart(data)`; `->label`, `->value`, `->alignment`, `->appear`, `->appear_key`, `->band`, `->band_count`, `->band_tick_count` and 21 more
- **`gk.Candlestick_Chart`** (CandlestickChart<Datum, element): `gk.candlestick_chart(data)`; `->appear`, `->appear_key`, `->bearish`, `->body_width_ratio`, `->bullish`, `->close`, `->grid`, `->high` and 11 more
- **`gk.Line_Chart`** (LineChart<Datum, element): `gk.line_chart(data)`; `->appear`, `->appear_key`, `->dot`, `->grid`, `->grid_columns`, `->grid_dashed`, `->interactive`, `->linear` and 21 more
- **`gk.Pie_Chart`** (PieChart<Datum>, element): `gk.pie_chart(data)`; `->label`, `->value`, `->appear`, `->appear_key`, `->color`, `->inner_radius`, `->inner_radius_fn`, `->inner_radius_ratio` and 10 more
- **`gk.Radar_Chart`** (RadarChart<Datum, element): `gk.radar_chart(data)`; `->label`, `->value`, `->appear`, `->appear_key`, `->dot`, `->fill`, `->grid`, `->grid_levels` and 11 more
- **`gk.Sankey_Chart`** (SankeyChart<Datum>, element): `gk.sankey_chart(nodes, links)`; `->appear`, `->appear_key`, `->interactive`, `->iterations`, `->label_gap`, `->labels`, `->link_opacity`, `->min_link_width` and 10 more
- **`gk.Sankey_Label`** (SankeyLabel, value): `gk.sankey_label(text)`; `->color`, `->font_size`

### component::checkbox

- **`gk.Checkbox`** (Checkbox, element): `gk.checkbox(id)`; `->label`, `->checked`, `->selected`, `->on_click`, `->on_change`, `->tooltip`, `->small`, `->large` and 5 more

### component::clipboard

- **`gk.Clipboard`** (Clipboard, element): `gk.clipboard(id)`; `->value`, `->tooltip`, `->small`, `->large`, `->on_copied`, `->value_fn`, `->with_size`, `->xsmall`

### component::collapsible

- **`gk.Collapsible`** (Collapsible, element): `gk.collapsible()`; `->content`, `->motion_id`, `->open`

### component::color_picker

- **`gk.Color_Picker`** (ColorPicker, element): `gk.color_picker(state)`; `->label`, `->icon`, `->small`, `->large`, `->anchor`, `->featured_colors`, `->with_size`, `->xsmall`
- **`gk.Color_Select`** (ColorSelect, element): `gk.color_select(state)`; `->placeholder`, `->small`, `->large`, `->featured_colors`, `->with_size`, `->xsmall`

### component::combobox

- **`gk.Caret`** (Caret, element): `gk.caret(size)`
- **`gk.Combobox`** (Combobox<GkSearchDelegate>, element): `gk.combobox(state)`; `->icon`, `->placeholder`, `->footer`, `->small`, `->large`, `->disabled`, `->appearance`, `->check_icon` and 7 more
- **`gk.Combobox_State`** (ComboboxState<GkSearchDelegate>, kit state): `gk.combobox_state(cx, gk.search_source(cx, a, items), selected_indices)->…->build()`; shown by `gk.combobox(state)`; built with `->multiple`, `->searchable`; reads `->add_selected_index`, `->query`, `->remove_selected_index`, `->selected_value`, `->selected_values`, `->selection`; writes `->clear_selection`, `->focus`, `->set_items`, `->set_query`, `->set_selected_indices`, `->set_selected_values`; events `->on_combobox_event(cx, gk.on(p))`

### component::command

- **`gk.Command`** (Command, element): `gk.command(state)`; `->placeholder`, `->on_select`, `->bordered`, `->filterable`, `->item`, `->items`, `->on_cancel`, `->on_confirm` and 4 more
- **`gk.Command_State`** (CommandState, kit state): handed out by the kit (no constructor); shown by `gk.command(state)`; reads `->is_loading`, `->matched_count`, `->query`, `->selected_index`; writes `->focus`, `->set_loading`, `->set_query`, `->set_selected_index`
- **`gk.Command_Group`** (CommandGroup, value): `gk.command_group()`; `->label`, `->item`, `->items`
- **`gk.Command_Item`** (CommandItem, value): `gk.command_item()` or `gk.command_item_default()`; `->label`, `->child`, `->checked`, `->icon`, `->disabled`, `->action`, `->keywords`

### component::date_picker

- **`gk.Date_Picker`** (DatePicker, element): `gk.date_picker(state)`; `->placeholder`, `->small`, `->large`, `->disabled`, `->appearance`, `->cleanable`, `->number_of_months`, `->presets` and 2 more
- **`gk.Date_Picker_State`** (DatePickerState, kit state): `gk.date_picker_state(cx)->…->build()` or `gk.date_picker_state_range(cx)->…->build()`; shown by `gk.date_picker(state)`; built with `->date_format`, `->default_time`, `->disabled_matcher`, `->first_day_of_week`, `->hour_cycle`, `->number_of_months`, `->time_precision`; reads `->date`, `->date_time`; writes `->set_date`, `->set_date_time`, `->set_year_range`; events `->on_date_picker_event(cx, gk.on(p))`
- **`gk.Date_Range_Preset`** (DateRangePreset, value): `gk.date_range_preset_date_time(label, value)` or `gk.date_range_preset_single(label, date)` or `gk.date_range_preset_range(label, start, end)`

### component::description_list

- **`gk.Description_List`** (DescriptionList, element): `gk.description_list()` or `gk.description_list_horizontal()` or `gk.description_list_vertical()`; `->child`, `->children`, `->small`, `->large`, `->bordered`, `->columns`, `->item`, `->label_width` and 4 more

### component::dialog

- **`gk.Alert_Dialog`** (AlertDialog, element): `gk.alert_dialog()`; `->title`, `->icon`, `->description`, `->content`, `->footer`, `->button_props`, `->cancel_text`, `->cancel_variant` and 14 more
- **`gk.Dialog`** (Dialog, element): `gk.dialog()`; `->title`, `->content`, `->footer`, `->button_props`, `->centered`, `->close_button`, `->entrance`, `->keyboard` and 8 more
- **`gk.Dialog_Action`** (DialogAction, element): `gk.dialog_action()`
- **`gk.Dialog_Close`** (DialogClose, element): `gk.dialog_close()`; `->trigger`
- **`gk.Dialog_Content`** (DialogContent, element): `gk.dialog_content()`
- **`gk.Dialog_Description`** (DialogDescription, element): `gk.dialog_description()`
- **`gk.Dialog_Footer`** (DialogFooter, element): `gk.dialog_footer()`
- **`gk.Dialog_Header`** (DialogHeader, element): `gk.dialog_header()`
- **`gk.Dialog_Title`** (DialogTitle, element): `gk.dialog_title()`
- **`gk.Dialog_Button_Props`** (DialogButtonProps, value): `gk.dialog_button_props_default()`; `->cancel_text`, `->cancel_variant`, `->ok_text`, `->ok_variant`, `->on_cancel`, `->on_ok`, `->show_cancel`

### component::diff

- **`gk.Diff`** (Diff, element): `gk.diff(state)`; `->annotations`, `->change_background`, `->change_indicator`, `->decorations`, `->gutter_markers`, `->header_visible`, `->hover_highlight`, `->hunk_separator` and 6 more
- **`gk.Diff_Annotation`** (DiffAnnotation, kit state): handed out by the kit (no constructor); reads `->id`, `->path`, `->position`
- **`gk.Diff_Conflict`** (DiffConflict, kit state): handed out by the kit (no constructor); reads `->base_label`, `->base_lines`, `->current_label`, `->current_lines`, `->incoming_label`, `->incoming_lines`, `->lines`
- **`gk.Diff_File_Read`** (DiffFile, kit state): handed out by the kit (no constructor); reads `->additions`, `->conflicts`, `->deletions`, `->extended_headers`, `->has_changes`, `->is_binary`, `->is_truncated`, `->language` and 4 more
- **`gk.Diff_Gutter_Marker_Read`** (DiffGutterMarker, kit state): handed out by the kit (no constructor); reads `->label`, `->id`, `->position`
- **`gk.Diff_Line_Position`** (DiffLinePosition, kit state): handed out by the kit (no constructor); reads `->line`, `->path`, `->side`
- **`gk.Diff_Line_Range_Read`** (DiffLineRange, kit state): handed out by the kit (no constructor); reads `->end`, `->end_side`, `->path`, `->side`, `->start`
- **`gk.Diff_Parse_Error`** (DiffParseError, kit state): handed out by the kit (no constructor); reads `->line`, `->message`
- **`gk.Diff_State`** (DiffState, kit state): `gk.diff_state(cx, files)->…->build()`; shown by `gk.diff(state)`; built with `->with_context_lines`, `->with_expansion_lines`, `->with_inline_max_line_length`, `->with_inline_unit`, `->with_min_collapsed_lines`, `->with_mode`, `->with_syntax_highlighter`, `->with_syntax_max_line_length`; reads `->conflict_resolution`, `->context_lines`, `->expansion_lines`, `->files`, `->inline_max_line_length`, `->inline_unit`, `->is_file_collapsed`, `->min_collapsed_lines` and 5 more; writes `->clear_syntax_highlighter`, `->collapse_unchanged`, `->expand_unchanged`, `->next_change`, `->previous_change`, `->resolve_conflict`, `->scroll_to_file`, `->scroll_to_line` and 11 more; events `->on_diff_event(cx, gk.on(p))`
- **`gk.Diff_Decoration`** (DiffDecoration, value): `gk.diff_decoration(range)`; `->with_background`
- **`gk.Diff_File`** (DiffFile, value): `gk.diff_file_parse_at(patch, index)` or `gk.diff_file_unchanged(path, text)` or `gk.diff_file_parse_with_at(patch, options, index)`; `->with_language`
- **`gk.Diff_Gutter_Marker`** (DiffGutterMarker, value): `gk.diff_gutter_marker(id, position)`; `->with_label`
- **`gk.Diff_Line_Range`** (DiffLineRange, value): `gk.diff_line_range(path, side, start, end)`; `->with_end_side`
- **`gk.Diff_Parse_Options`** (DiffParseOptions, value): `gk.diff_parse_options()` or `gk.diff_parse_options_default()`; `->truncated_hunks`

### component::dock

- **`gk.Panel_Handle`** (PanelHandle, kit state): handed out by the kit (no constructor); reads `->panel`

### component::empty

- **`gk.Empty`** (Empty, element): `gk.empty()` or `gk.empty_default()`; `->content`, `->header`
- **`gk.Empty_Content`** (EmptyContent, element): `gk.empty_content()` or `gk.empty_content_default()`
- **`gk.Empty_Description`** (EmptyDescription, element): `gk.empty_description()` or `gk.empty_description_default()`
- **`gk.Empty_Header`** (EmptyHeader, element): `gk.empty_header()` or `gk.empty_header_default()`; `->title`, `->description`, `->media`
- **`gk.Empty_Media`** (EmptyMedia, element): `gk.empty_media()` or `gk.empty_media_default()`; `->with_variant`
- **`gk.Empty_Title`** (EmptyTitle, element): `gk.empty_title()` or `gk.empty_title_default()`

### component::form

- **`gk.Field`** (Field, element): `gk.field()` or `gk.kit_field()`; `->label`, `->description`, `->description_fn`, `->items_center`, `->items_end`, `->items_start`, `->label_fn`, `->label_indent` and 2 more
- **`gk.Form`** (Form, element): `gk.form()` or `gk.form_default()` or `gk.form_horizontal()` (and 3 more); `->child`, `->children`, `->footer`, `->small`, `->large`, `->columns`, `->label_layout`, `->label_text_size` and 4 more

### component::group_box

- **`gk.Group_Box`** (GroupBox, element): `gk.group_box()`; `->title`, `->footer`, `->outline`, `->content_style`, `->fill`, `->normal`, `->title_style`, `->with_variant`

### component::highlighter

- **`gk.Status_Colors`** (StatusColors, kit state): handed out by the kit (no constructor); reads `->created`, `->deleted`, `->error`, `->error_background`, `->error_border`, `->hint`, `->hint_background`, `->hint_border` and 10 more
- **`gk.Syntax_Colors`** (SyntaxColors, kit state): handed out by the kit (no constructor); reads `->style`, `->style_for_index`
- **`gk.Syntax_Highlighter`** (SyntaxHighlighter, kit state): handed out by the kit (no constructor); reads `->text`, `->language`

### component::hover_card

- **`gk.Hover_Card`** (HoverCard, element): `gk.hover_card(id)`; `->anchor`, `->appearance`, `->close_delay`, `->on_open_change`, `->open_delay`, `->trigger`, `->trigger_style`

### component::input

- **`gk.Input`** (Input, element): `gk.input(state)`; `->selected`, `->prefix`, `->suffix`, `->small`, `->large`, `->disabled`, `->appearance`, `->bordered` and 13 more
- **`gk.Input_Group`** (InputGroup, element): `gk.input_group(id)`; `->small`, `->large`, `->disabled`, `->addon`, `->input`, `->invalid`, `->readonly`, `->with_size` and 1 more
- **`gk.Input_Group_Addon`** (InputGroupAddon, element): `gk.input_group_addon(id)`; `->align`
- **`gk.Input_Group_Button`** (InputGroupButton, element): `gk.input_group_button(id)`; `->label`, `->child`, `->children`, `->selected`, `->icon`, `->on_click`, `->tooltip`, `->primary` and 20 more
- **`gk.Input_Group_Text`** (InputGroupText, element): `gk.input_group_text()` or `gk.input_group_text_default()`
- **`gk.Input_Token`** (InputToken, element): `gk.input_token(context_)`; `->icon`
- **`gk.Number_Input`** (NumberInput, element): `gk.number_input(state)`; `->placeholder`, `->prefix`, `->suffix`, `->small`, `->large`, `->disabled`, `->appearance`, `->with_size` and 1 more
- **`gk.Otp_Input`** (OtpInput, element): `gk.otp_input(state)`; `->small`, `->large`, `->disabled`, `->groups`, `->with_size`, `->xsmall`
- **`gk.Textarea`** (Textarea, element): `gk.textarea(state)`; `->small`, `->large`, `->disabled`, `->appearance`, `->bordered`, `->on_paste`, `->on_token_click`, `->on_token_hover` and 3 more

### component::kbd

- **`gk.Kbd`** (Kbd, element): `gk.kbd(stroke)`; `->outline`, `->appearance`

### component::label

- **`gk.Label`** (Label, element): `gk.label(label)`; `->highlights`, `->masked`, `->secondary`

### component::link

- **`gk.Link`** (Link, element): `gk.link(id)`; `->on_click`, `->disabled`, `->href`

### component::list

- **`gk.List`** (List<GkListDelegate>, element): `gk.list(state)`; `->small`, `->large`, `->scroll_bounce`, `->scrollbar_visible`, `->search_placeholder`, `->with_size`, `->xsmall`
- **`gk.List_Item`** (ListItem, element): `gk.list_item(id)`; `->selected`, `->on_click`, `->tooltip`, `->suffix`, `->disabled`, `->check_icon`, `->confirmed`, `->on_mouse_enter` and 3 more
- **`gk.List_State`** (ListState<GkListDelegate>, kit state): `gk.list_state(cx, gk.list_source(cx, a, count, row))->…->build()`; shown by `gk.list(state)`; built with `->searchable`, `->selectable`; reads `->delegate`, `->delegate_mut`, `->right_clicked_index`, `->scroll_handle`, `->selected_index`; writes `->focus`, `->scroll_to_item`, `->scroll_to_selected_item`, `->set_item_to_measure_index`, `->set_query`, `->set_right_clicked_index`, `->set_searchable`, `->set_selectable` and 1 more; events `->on_list_event(cx, gk.on(p))`

### component::marker

- **`gk.Marker`** (Marker, element): `gk.marker()` or `gk.marker_default()`; `->icon`, `->content`, `->alignment`, `->loading`, `->separator_style`, `->with_loading_style`, `->with_shimmer_style`, `->with_variant`
- **`gk.Marker_Content`** (MarkerContent, element): `gk.marker_content()` or `gk.marker_content_default()`; `->text`, `->child`, `->children`
- **`gk.Marker_Icon`** (MarkerIcon, element): `gk.marker_icon()` or `gk.marker_icon_default()`

### component::menu

- **`gk.App_Menu_Bar`** (AppMenuBar, kit state): handed out by the kit (no constructor); writes `->reload`
- **`gk.Popup_Menu`** (PopupMenu, value): `gk.popup_menu()`; `->label`, `->action_context`, `->check_side`, `->external_link_icon`, `->item`, `->link`, `->link_with_disabled`, `->link_with_icon` and 18 more
- **`gk.Popup_Menu_Item`** (PopupMenuItem, value): `gk.popup_menu_item_separator()` or `gk.popup_menu_item(label)` or `gk.popup_menu_item_element(builder)` (and 2 more); `->checked`, `->icon`, `->on_click`, `->disabled`, `->action`

### component::message

- **`gk.Message`** (Message, element): `gk.message()` or `gk.message_default()`; `->content`, `->footer`, `->alignment`, `->avatar`, `->avatar_slot`, `->header`, `->with_stack_style`
- **`gk.Message_Avatar`** (MessageAvatar, element): `gk.message_avatar()` or `gk.message_avatar_default()`
- **`gk.Message_Content`** (MessageContent, element): `gk.message_content()` or `gk.message_content_default()`; `->bubble`
- **`gk.Message_Footer`** (MessageFooter, element): `gk.message_footer()` or `gk.message_footer_default()`; `->content_inset`
- **`gk.Message_Group`** (MessageGroup, element): `gk.message_group()` or `gk.message_group_default()`
- **`gk.Message_Header`** (MessageHeader, element): `gk.message_header()` or `gk.message_header_default()`; `->content_inset`

### component::message_scroller

- **`gk.Message_Scroller`** (MessageScroller, element): `gk.message_scroller(id, state, renderer)`; `->jump_button`, `->with_bottom_fade`, `->with_content_style`, `->with_jump_button_label`, `->with_jump_button_renderer`, `->with_jump_button_style`, `->with_jump_button_transition`, `->with_list_style` and 1 more
- **`gk.Message_Scroller_State`** (MessageScrollerState, kit state): handed out by the kit (no constructor); shown by `gk.message_scroller(state)`; reads `->append`, `->is_following_tail`, `->is_scrolled_up`, `->item_count`, `->logical_scroll_top`, `->prepend`, `->remeasure_items`, `->scroll_to_item` and 1 more; writes `->remeasure`, `->reset`, `->scroll_to_end`

### component::native_menu

- **`gk.Native_Menu`** (NativeMenu, value): `gk.native_menu()` or `gk.native_menu_default()`; `->menu`, `->menu_with_check`, `->menu_with_disabled`, `->menu_with_icon`, `->menu_with_icon_and_disabled`, `->menu_with_icon_disabled`, `->separator`, `->submenu`

### component::notification

- **`gk.Notification`** (Notification, kit state): `gk.notification(cx)->…->build()` or `gk.notification_error(cx, message)->…->build()` or `gk.notification_info(cx, message)->…->build()` (and 2 more); built with `->title`, `->icon`, `->on_click`, `->autohide`, `->delivery`, `->in_app_and_system`, `->message`, `->on_close` and 3 more; writes `->dismiss`
- **`gk.Notification_List`** (NotificationList, kit state): handed out by the kit (no constructor); reads `->notifications`; writes `->clear`, `->push`

### component::pagination

- **`gk.Pagination`** (Pagination, element): `gk.pagination(id)`; `->on_click`, `->small`, `->large`, `->disabled`, `->compact`, `->current_page`, `->total_pages`, `->visible_pages` and 2 more

### component::plot

- **`gk.Cross_Line`** (CrossLine, element): `gk.cross_line(point)`; `->band`, `->both`, `->h_span`, `->height`, `->horizontal`, `->span`, `->width`
- **`gk.Dot`** (Dot, element): `gk.dot(point)`; `->fill`, `->halo`, `->stroke`
- **`gk.Tooltip`** (Tooltip, element): `gk.tooltip(cursor, within)`; `->title`, `->appearance`, `->cross_line`, `->dots`, `->glide`, `->plain_row`, `->progress`, `->row` and 1 more

### component::popover

- **`gk.Popover`** (Popover, element): `gk.popover(id)`; `->anchor`, `->appearance`, `->arrow`, `->default_open`, `->mouse_button`, `->offset`, `->on_open_change`, `->open` and 3 more

### component::progress

- **`gk.Progress`** (Progress, element): `gk.progress(id)`; `->value`, `->small`, `->large`, `->color`, `->loading`, `->with_size`, `->xsmall`
- **`gk.Progress_Circle`** (ProgressCircle, element): `gk.progress_circle(id)`; `->value`, `->small`, `->large`, `->color`, `->loading`, `->with_size`, `->xsmall`

### component::questionnaire

- **`gk.Questionnaire`** (Questionnaire, element): `gk.questionnaire(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Actions`** (QuestionnaireActions, element): `gk.questionnaire_actions(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Choice`** (QuestionnaireChoice, element): `gk.questionnaire_choice(state, item, value)`; `->small`, `->large`, `->content_style`, `->indicator_style`, `->shortcut_style`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Choice_Description`** (QuestionnaireChoiceDescription, element): `gk.questionnaire_choice_description()` or `gk.questionnaire_choice_description_default()`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Choices`** (QuestionnaireChoices, element): `gk.questionnaire_choices(state, item)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Description`** (QuestionnaireDescription, element): `gk.questionnaire_description(state, item)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Error`** (QuestionnaireError, element): `gk.questionnaire_error(state, item)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Input`** (QuestionnaireInput, element): `gk.questionnaire_input(state, item)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Item`** (QuestionnaireItem, element): `gk.questionnaire_item(state, item)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Next`** (QuestionnaireNext, element): `gk.questionnaire_next(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Previous`** (QuestionnairePrevious, element): `gk.questionnaire_previous(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Progress`** (QuestionnaireProgress, element): `gk.questionnaire_progress(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Skip`** (QuestionnaireSkip, element): `gk.questionnaire_skip(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Submit`** (QuestionnaireSubmit, element): `gk.questionnaire_submit(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Questionnaire_Title`** (QuestionnaireTitle, element): `gk.questionnaire_title(state, item)`; `->small`, `->large`, `->with_size`, `->xsmall`

### component::radio

- **`gk.Radio`** (Radio, element): `gk.radio(id)`; `->label`, `->checked`, `->on_click`, `->on_change`, `->tooltip`, `->small`, `->large`, `->disabled` and 2 more
- **`gk.Radio_Group`** (RadioGroup, element): `gk.radio_group(id)` or `gk.radio_group_horizontal(id)` or `gk.radio_group_vertical(id)`; `->child`, `->children`, `->on_click`, `->on_change`, `->disabled`, `->layout`, `->selected_index`

### component::rating

- **`gk.Rating`** (Rating, element): `gk.rating(id)`; `->value`, `->on_click`, `->small`, `->large`, `->disabled`, `->color`, `->max`, `->with_size` and 1 more

### component::searchable_list

- **`gk.Searchable_List_Item_Element`** (SearchableListItemElement, element): `gk.searchable_list_item_element(ix)`; `->checked`, `->selected`, `->small`, `->large`, `->disabled`, `->check_icon`, `->open`, `->secondary_selected` and 2 more

### component::select

- **`gk.Select`** (Select<GkSearchDelegate>, element): `gk.select(state)`; `->icon`, `->placeholder`, `->small`, `->large`, `->disabled`, `->appearance`, `->cleanable`, `->empty` and 7 more
- **`gk.Select_State`** (SelectState<GkSearchDelegate>, kit state): `gk.select_state(cx, gk.search_source(cx, a, items), selected_index)->…->build()`; shown by `gk.select(state)`; built with `->searchable`; reads `->selected_index`, `->selected_value`; writes `->focus`, `->set_items`, `->set_selected_index`, `->set_selected_value`; events `->on_select_event(cx, gk.on(p))`

### component::separator

- **`gk.Separator`** (Separator, element): `gk.separator_horizontal()` or `gk.separator_horizontal_dashed()` or `gk.separator_vertical()` (and 1 more); `->label`, `->color`, `->dashed`

### component::setting

- **`gk.Settings`** (Settings, element): `gk.settings(id)`; `->small`, `->large`, `->default_selected_index`, `->header_style`, `->page`, `->pages`, `->sidebar_size_range`, `->sidebar_style` and 4 more
- **`gk.Setting_Field_Bool`** (SettingField<bool>, value): `gk.setting_field_bool_checkbox(value, set_value)` or `gk.setting_field_bool_switch(value, set_value)`; `->default_value`, `->on_reset`
- **`gk.Setting_Field_Number`** (SettingField<f64>, value): `gk.setting_field_number_number_input(options, value, set_value)`; `->default_value`, `->on_reset`
- **`gk.Setting_Field_Text`** (SettingField<SharedString>, value): `gk.setting_field_text_element(element)` or `gk.setting_field_text_render(element_render)` or `gk.setting_field_text_input(value, set_value)` (and 2 more); `->default_value`, `->on_reset`
- **`gk.Setting_Group`** (SettingGroup, value): `gk.setting_group()`; `->title`, `->description`, `->footer`, `->item`, `->items`, `->variant`
- **`gk.Setting_Item`** (SettingItem, value): `gk.setting_item_render(render)` or `gk.setting_item(title, field)`; `->description`, `->disabled`, `->keywords`, `->layout`, `->on_reset`
- **`gk.Setting_Page`** (SettingPage, value): `gk.setting_page(title)`; `->title`, `->icon`, `->description`, `->default_open`, `->group`, `->groups`, `->header_style`, `->resettable` and 1 more

### component::sheet

- **`gk.Sheet`** (Sheet, element): `gk.sheet()`; `->title`, `->footer`, `->on_close`, `->overlay`, `->overlay_closable`, `->resizable`

### component::shimmer

- **`gk.Shimmer_Text`** (ShimmerText, element): `gk.shimmer_text(text)`; `->duration`, `->highlight_color`, `->once`, `->reverse`, `->spread`, `->with_shimmer_style`
- **`gk.Shimmer_Style`** (ShimmerStyle, value): `gk.shimmer_style()` or `gk.shimmer_style_default()`; `->duration`, `->highlight_color`, `->once`, `->reverse`, `->spread`

### component::sidebar

- **`gk.Grouped_Sidebar`** (Sidebar<SidebarGroup<SidebarMenu>>, element): `gk.grouped_sidebar(id)`; `->child`, `->children`, `->footer`, `->collapsed`, `->collapsible`, `->header`, `->side`
- **`gk.Sidebar`** (Sidebar<SidebarMenu>, element): `gk.sidebar(id)`; `->child`, `->children`, `->footer`, `->collapsed`, `->collapsible`, `->header`, `->side`
- **`gk.Sidebar_Footer`** (SidebarFooter, element): `gk.sidebar_footer()`; `->selected`, `->collapsed`, `->open`, `->secondary_selected`
- **`gk.Sidebar_Header`** (SidebarHeader, element): `gk.sidebar_header()` or `gk.sidebar_header_default()`; `->selected`, `->collapsed`, `->open`, `->secondary_selected`
- **`gk.Sidebar_Toggle_Button`** (SidebarToggleButton, element): `gk.sidebar_toggle_button()`; `->on_click`, `->collapsed`, `->side`
- **`gk.Sidebar_Group`** (SidebarGroup<SidebarMenu>, value): `gk.sidebar_group(label)`; `->child`, `->children`, `->collapsed`
- **`gk.Sidebar_Menu`** (SidebarMenu, value): `gk.sidebar_menu()`; `->child`, `->children`, `->collapsed`
- **`gk.Sidebar_Menu_Item`** (SidebarMenuItem, value): `gk.sidebar_menu_item(label)`; `->children`, `->icon`, `->on_click`, `->suffix`, `->accessibility_label`, `->active`, `->click_to_open`, `->click_to_toggle` and 5 more

### component::skeleton

- **`gk.Skeleton`** (Skeleton, element): `gk.skeleton()`; `->secondary`

### component::slider

- **`gk.Slider`** (Slider, element): `gk.slider(state)`; `->disabled`, `->horizontal`, `->reverse`, `->vertical`

### component::speech

- **`gk.Speech_Button`** (SpeechButton, element): `gk.speech_button(state)`; `->small`, `->large`, `->disabled`, `->show_when_unsupported`, `->with_size`, `->xsmall`
- **`gk.Speech_Indicator`** (SpeechIndicator, element): `gk.speech_indicator(state)`; `->small`, `->large`, `->with_size`, `->xsmall`
- **`gk.Speech_Waveform`** (SpeechWaveform, element): `gk.speech_waveform(state)`; `->small`, `->large`, `->liquid`, `->with_size`, `->xsmall`
- **`gk.Audio_Format`** (AudioFormat, kit state): handed out by the kit (no constructor); reads `->channels`, `->sample_rate`
- **`gk.Speech_State`** (SpeechState, kit state): `gk.speech_state(cx)->…->build()`; shown by `gk.speech_button(state)`, `gk.speech_indicator(state)`, `gk.speech_waveform(state)`; built with `->stop_timeout`, `->system_fallback`; reads `->has_recognizer`, `->is_available`, `->levels`, `->status`, `->transcript`; writes `->cancel`, `->start`, `->stop`, `->toggle`; events `->on_speech_event(cx, gk.on(p))`

### component::spinner

- **`gk.Spinner`** (Spinner, element): `gk.spinner()`; `->icon`, `->small`, `->large`, `->color`, `->ease`, `->with_size`, `->xsmall`

### component::status_bar

- **`gk.Status_Bar`** (StatusBar, element): `gk.status_bar()`

### component::stepper

- **`gk.Stepper`** (Stepper, element): `gk.stepper(id)`; `->on_click`, `->small`, `->large`, `->disabled`, `->item`, `->items`, `->layout`, `->selected_index` and 4 more
- **`gk.Stepper_Item`** (StepperItem, element): `gk.stepper_item()`; `->icon`, `->small`, `->large`, `->disabled`, `->with_size`, `->xsmall`

### component::switch

- **`gk.Switch`** (Switch, element): `gk.switch_(id)`; `->label`, `->checked`, `->on_click`, `->on_change`, `->tooltip`, `->small`, `->large`, `->disabled` and 3 more

### component::tab

- **`gk.Tab`** (Tab, element): `gk.tab()` or `gk.tab_default()`; `->label`, `->selected`, `->icon`, `->on_click`, `->tooltip`, `->prefix`, `->suffix`, `->outline` and 12 more
- **`gk.Tab_Bar`** (TabBar, element): `gk.tab_bar(id)`; `->child`, `->children`, `->on_click`, `->prefix`, `->suffix`, `->outline`, `->small`, `->large` and 11 more

### component::table

- **`gk.Data_Table`** (DataTable<GkTableDelegate>, element): `gk.data_table(state)`; `->small`, `->large`, `->bordered`, `->fixed_cols_border`, `->header_row_height`, `->scroll_bounce`, `->scrollbar_visible`, `->stripe` and 2 more
- **`gk.Table`** (Table, element): `gk.table()`; `->child`, `->children`, `->small`, `->large`, `->with_ix`, `->with_size`, `->xsmall`
- **`gk.Table_Body`** (TableBody, element): `gk.table_body()`; `->child`, `->children`, `->small`, `->large`, `->with_ix`, `->with_size`, `->xsmall`
- **`gk.Table_Caption`** (TableCaption, element): `gk.table_caption()`; `->small`, `->large`, `->with_ix`, `->with_size`, `->xsmall`
- **`gk.Table_Cell`** (TableCell, element): `gk.table_cell()`; `->small`, `->large`, `->text_center`, `->text_right`, `->with_ix`, `->with_size`, `->xsmall`
- **`gk.Table_Footer`** (TableFooter, element): `gk.table_footer()`; `->child`, `->children`, `->small`, `->large`, `->with_ix`, `->with_size`, `->xsmall`
- **`gk.Table_Head`** (TableHead, element): `gk.table_head()`; `->small`, `->large`, `->text_center`, `->text_right`, `->with_ix`, `->with_size`, `->xsmall`
- **`gk.Table_Header`** (TableHeader, element): `gk.table_header()`; `->child`, `->children`, `->small`, `->large`, `->with_ix`, `->with_size`, `->xsmall`
- **`gk.Table_Row`** (TableRow, element): `gk.table_row()`; `->child`, `->children`, `->small`, `->large`, `->with_ix`, `->with_size`, `->xsmall`
- **`gk.Table_State`** (TableState<GkTableDelegate>, kit state): `gk.table_state(cx, gk.table_source(cx, a, rows, columns, cell))->…->build()`; shown by `gk.data_table(state)`; built with `->cell_selectable`, `->col_movable`, `->col_resizable`, `->col_selectable`, `->loop_selection`, `->row_header`, `->row_selectable`, `->select_on_modifier_click` and 1 more; reads `->col_width`, `->delegate`, `->delegate_mut`, `->dump`, `->dump_range`, `->fit_col`, `->headers`, `->right_clicked_cell` and 7 more; writes `->clear_selection`, `->refresh`, `->refresh_header_layout`, `->scroll_to_col`, `->scroll_to_row`, `->set_right_clicked_row`, `->set_selected_cell`, `->set_selected_col` and 2 more; events `->on_table_event(cx, gk.on(p))`
- **`gk.Table_Visible_Range`** (TableVisibleRange, kit state): handed out by the kit (no constructor); reads `->cols`, `->rows`
- **`gk.Column`** (Column, value): `gk.column_default()` or `gk.column(key, name)`; `->ascending`, `->descending`, `->fixed`, `->fixed_left`, `->fixed_right`, `->max_width`, `->min_width`, `->movable` and 9 more

### component::tag

- **`gk.Tag`** (Tag, element): `gk.tag()` or `gk.tag_danger()` or `gk.tag_info()` (and 6 more); `->outline`, `->small`, `->large`, `->rounded_full`, `->with_size`, `->with_variant`, `->xsmall`

### component::text

- **`gk.Text_View`** (TextView, element): `gk.html(source)` or `gk.markdown(source)` or `gk.text_view(state)` (and 2 more); `->blockquote_style`, `->image_source`, `->inline_code_style`, `->link_tooltip`, `->list_style`, `->markdown_extensions`, `->markdown_mdx`, `->max_lines` and 9 more
- **`gk.Frontmatter_Plugin`** (FrontmatterPlugin, value): `gk.frontmatter_plugin()` or `gk.frontmatter_plugin_default()`; `->setup`
- **`gk.Text_View_Style`** (TextViewStyle, value): `gk.text_view_style_default()`; `->code_block`, `->heading_font_size`, `->inline_code`, `->paragraph_gap`, `->table`, `->table_cell`, `->table_head`

### component::time_field

- **`gk.Time_Field`** (TimeField, element): `gk.time_field(state)`; `->small`, `->large`, `->disabled`, `->invalid`, `->with_size`, `->xsmall`

### component::toolbar

- **`gk.Toolbar`** (Toolbar, element): `gk.toolbar(id)`; `->child`, `->children`, `->content`, `->small`, `->large`, `->disabled`, `->contents`, `->with_size` and 1 more
- **`gk.Toolbar_Group`** (ToolbarGroup, element): `gk.toolbar_group(id)`; `->label`, `->child`, `->children`, `->content`, `->small`, `->large`, `->with_size`, `->xsmall`

### component::tooltip

- **`gk.Kit_Tooltip`** (Tooltip, value): `gk.kit_tooltip(text)` or `gk.kit_tooltip_element(builder)`; `->action`, `->key_binding`

### component::tree

- **`gk.Tree`** (Tree, element): `gk.kit_tree(state, render_item)` or `gk.tree(state, render_item)`; `->scroll_bounce`

### base

- **`gk.Accordion_Header`** (AccordionHeader, element): `gk.accordion_header(trigger)`; `->level`
- **`gk.Accordion_Panel`** (AccordionPanel, element): `gk.accordion_panel()`; `->keep_mounted`, `->open`
- **`gk.Accordion_Trigger`** (AccordionTrigger, element): `gk.accordion_trigger(id)`; `->on_change`, `->disabled`, `->open`
- **`gk.Alert_Dialog_Action`** (AlertDialogAction, element): `gk.alert_dialog_action()` or `gk.alert_dialog_action_default()`
- **`gk.Alert_Dialog_Backdrop`** (AlertDialogBackdrop, element): `gk.alert_dialog_backdrop()` or `gk.alert_dialog_backdrop_default()`
- **`gk.Alert_Dialog_Cancel`** (AlertDialogCancel, element): `gk.alert_dialog_cancel()` or `gk.alert_dialog_cancel_default()`
- **`gk.Alert_Dialog_Close`** (AlertDialogClose, element): `gk.alert_dialog_close()` or `gk.alert_dialog_close_default()`
- **`gk.Alert_Dialog_Description`** (AlertDialogDescription, element): `gk.alert_dialog_description()` or `gk.alert_dialog_description_default()`
- **`gk.Alert_Dialog_Popup`** (AlertDialogPopup, element): `gk.alert_dialog_popup()` or `gk.alert_dialog_popup_default()`
- **`gk.Alert_Dialog_Title`** (AlertDialogTitle, element): `gk.alert_dialog_title()` or `gk.alert_dialog_title_default()`
- **`gk.Alert_Dialog_Trigger`** (AlertDialogTrigger, element): `gk.alert_dialog_trigger(trigger)`; `->handle`, `->on_open`
- **`gk.Avatar_Fallback`** (AvatarFallback, element): `gk.avatar_fallback()` or `gk.avatar_fallback_default()`
- **`gk.Avatar_Image`** (AvatarImage, element): `gk.avatar_image(source)`
- **`gk.Base_Accordion`** (Accordion, element): `gk.base_accordion(id)`
- **`gk.Base_Accordion_Item`** (AccordionItem, element): `gk.base_accordion_item()`; `->disabled`, `->header`, `->open`, `->panel`
- **`gk.Base_Alert_Dialog`** (AlertDialog, element): `gk.base_alert_dialog()`; `->backdrop`, `->close_on_escape`, `->dismiss_below_y`, `->handle`, `->on_cancel`, `->on_close`, `->on_ok`, `->on_open_change` and 2 more
- **`gk.Base_Avatar`** (Avatar, element): `gk.base_avatar()` or `gk.base_avatar_default()`; `->fallback`, `->image`
- **`gk.Base_Button`** (Button, element): `gk.base_button(id)`; `->selected`, `->on_click`, `->tooltip`, `->disabled`, `->open`, `->secondary_selected`, `->styles`
- **`gk.Base_Calendar`** (Calendar, element): `gk.base_calendar(id, state)`; `->label`, `->first_day_of_week`, `->number_of_months`
- **`gk.Base_Checkbox`** (Checkbox, element): `gk.base_checkbox(id)`; `->checked`, `->on_click`, `->on_change`, `->tooltip`, `->disabled`, `->indeterminate`, `->state`, `->styles`
- **`gk.Base_Collapsible`** (Collapsible, element): `gk.base_collapsible()` or `gk.base_collapsible_default()`; `->content`, `->open`, `->reveal`
- **`gk.Base_Color_Picker`** (ColorPicker, element): `gk.base_color_picker(id)`; `->on_click`, `->tooltip`, `->disabled`, `->on_open_change`, `->open`
- **`gk.Base_Combobox`** (Combobox, element): `gk.base_combobox(id)`; `->disabled`, `->content_focus_handle`, `->focus_handle`, `->on_confirm`, `->on_dismiss`, `->on_open_change`, `->open`
- **`gk.Base_Date_Picker`** (DatePicker, element): `gk.base_date_picker(id, focus_handle)`; `->disabled`, `->on_open_change`, `->open`
- **`gk.Base_Dialog`** (Dialog, element): `gk.base_dialog()`; `->backdrop`, `->close_on_backdrop_press`, `->close_on_escape`, `->dismiss_below_y`, `->handle`, `->on_cancel`, `->on_close`, `->on_ok` and 3 more
- **`gk.Base_Dialog_Close`** (DialogClose, element): `gk.base_dialog_close()` or `gk.base_dialog_close_default()`; `->trigger`
- **`gk.Base_Dialog_Description`** (DialogDescription, element): `gk.base_dialog_description()` or `gk.base_dialog_description_default()`
- **`gk.Base_Dialog_Title`** (DialogTitle, element): `gk.base_dialog_title()` or `gk.base_dialog_title_default()`
- **`gk.Base_Hover_Card`** (HoverCard, element): `gk.base_hover_card(id)`; `->anchor`, `->close_delay`, `->on_open_change`, `->open_delay`, `->trigger`
- **`gk.Base_Input`** (Input, element): `gk.base_input(state)`; `->on_token_click`, `->on_token_hover`
- **`gk.Base_Link`** (Link, element): `gk.base_link(id)`; `->disabled`, `->href`, `->on_activate`, `->open_with`, `->styles`
- **`gk.Base_Number_Input`** (NumberInput, element): `gk.base_number_input(state)`; `->disabled`, `->controls_right`, `->decrement_button`, `->increment_button`, `->input`, `->on_step`
- **`gk.Base_Otp_Input`** (OtpInput, element): `gk.base_otp_input(state)`; `->disabled`
- **`gk.Base_Pagination`** (Pagination, element): `gk.base_pagination(id, state)`
- **`gk.Base_Popover`** (Popover, element): `gk.base_popover(id)`; `->anchor`, `->default_open`, `->mouse_button`, `->offset`, `->on_open_change`, `->on_position`, `->open`, `->overlay_closable` and 1 more
- **`gk.Base_Progress`** (Progress, element): `gk.base_progress(id)`; `->value`, `->indeterminate`
- **`gk.Base_Radio`** (Radio, element): `gk.base_radio(id)`; `->checked`, `->on_change`, `->disabled`, `->set_position`, `->styles`
- **`gk.Base_Radio_Group`** (RadioGroup, element): `gk.base_radio_group(id)`; `->axis`
- **`gk.Base_Select`** (Select, element): `gk.base_select(id)`; `->disabled`, `->content_focus_handle`, `->focus_handle`, `->on_confirm`, `->on_dismiss`, `->on_open_change`, `->open`
- **`gk.Base_Sheet`** (Sheet, element): `gk.base_sheet()`; `->on_close`, `->overlay`, `->overlay_closable`, `->surface`
- **`gk.Base_Slider`** (Slider, element): `gk.base_slider(state)`; `->disabled`, `->axis`, `->horizontal`, `->vertical`
- **`gk.Base_Switch`** (Switch, element): `gk.base_switch(id)`; `->checked`, `->on_change`, `->disabled`, `->styles`
- **`gk.Base_Tab`** (Tab, element): `gk.base_tab(id)`; `->selected`, `->on_click`, `->tooltip`, `->disabled`, `->set_position`, `->styles`
- **`gk.Base_Table`** (Table, element): `gk.base_table(id)`; `->column_count`, `->row_count`
- **`gk.Base_Table_Body`** (TableBody, element): `gk.base_table_body(id)`
- **`gk.Base_Table_Caption`** (TableCaption, element): `gk.base_table_caption(id)`
- **`gk.Base_Table_Cell`** (TableCell, element): `gk.base_table_cell(id, column_index)`
- **`gk.Base_Table_Head`** (TableHead, element): `gk.base_table_head(id, column_index)`
- **`gk.Base_Table_Header`** (TableHeader, element): `gk.base_table_header(id)`
- **`gk.Base_Table_Row`** (TableRow, element): `gk.base_table_row(id, row_index)`
- **`gk.Base_Text_View`** (TextView, element): `gk.base_html(source)` or `gk.base_markdown(source)` or `gk.base_text_view(state)` (and 2 more); `->blockquote_style`, `->code_block_highlighter`, `->image_source`, `->inline_code_style`, `->link_tooltip`, `->list_style`, `->markdown_extensions`, `->markdown_mdx` and 9 more
- **`gk.Base_Textarea`** (Textarea, element): `gk.base_textarea(state)`; `->on_token_click`, `->on_token_hover`
- **`gk.Base_Time_Field`** (TimeField, element): `gk.base_time_field(id, state)`; `->disabled`, `->separator`
- **`gk.Base_Toggle`** (Toggle, element): `gk.base_toggle(id)`; `->on_change`, `->disabled`, `->pressed`, `->styles`
- **`gk.Base_Toggle_Group`** (ToggleGroup, element): `gk.base_toggle_group(id)`; `->axis`
- **`gk.Base_Toolbar`** (Toolbar, element): `gk.base_toolbar(id)`; `->disabled`, `->axis`
- **`gk.Base_Toolbar_Group`** (ToolbarGroup, element): `gk.base_toolbar_group(id)`; `->label`
- **`gk.Base_Tooltip`** (Tooltip, element): `gk.base_tooltip(id)`
- **`gk.Base_Tree`** (Tree, element): `gk.base_tree(state)`; `->list_style`
- **`gk.Calendar_Item`** (CalendarItem, element): `gk.calendar_item_handed(index)`; `->clear_children`
- **`gk.Checkbox_Indicator`** (CheckboxIndicator, element): `gk.checkbox_indicator()` or `gk.checkbox_indicator_default()`; `->checked`, `->disabled`, `->state`, `->styles`
- **`gk.Color_Swatch`** (ColorSwatch, element): `gk.color_swatch(id, color)`; `->selected`, `->on_click`, `->tooltip`, `->disabled`
- **`gk.Dialog_Backdrop`** (DialogBackdrop, element): `gk.dialog_backdrop()` or `gk.dialog_backdrop_default()`
- **`gk.Dialog_Popup`** (DialogPopup, element): `gk.dialog_popup()` or `gk.dialog_popup_default()`
- **`gk.Dialog_Trigger`** (DialogTrigger, element): `gk.dialog_trigger(trigger)`; `->handle`, `->on_open`
- **`gk.Editor`** (Editor, element): `gk.editor(state)`
- **`gk.Grid_Layout`** (GridLayout, element): `gk.grid_layout(id, state)` or `gk.kit_grid_layout(id, state)`; `->placeholder`, `->dragging_item`, `->focused_item`, `->item`, `->items`, `->reflow`, `->resize_handle`, `->row_height` and 2 more
- **`gk.Input_Base`** (InputBase, element): `gk.input_base(id)`; `->on_click`, `->tooltip`, `->disabled`, `->focused`, `->semantic_owner`, `->styles`
- **`gk.Motion_Reveal`** (MotionReveal, element): `gk.motion_reveal(id, progress, child)`
- **`gk.Nav_Page`** (NavPage, element): `gk.nav_page_handed(index)`
- **`gk.Nav_Stack`** (NavStack, element): `gk.nav_stack(state)`; `->transition`
- **`gk.Number_Input_Text`** (NumberInputText, element): `gk.number_input_text()` or `gk.number_input_text_default()`
- **`gk.Popup`** (Popup, element): `gk.popup(id, trigger)`; `->content`, `->anchor`, `->offset`, `->on_position`
- **`gk.Positioner`** (Positioner, element): `gk.positioner_side(trigger_bounds)` or `gk.positioner_corner(anchor, position)`; `->align`, `->margin`, `->offset`, `->on_position`, `->placement`, `->position`
- **`gk.Progress_Indicator`** (ProgressIndicator, element): `gk.progress_indicator()`
- **`gk.Progress_Track`** (ProgressTrack, element): `gk.progress_track()`
- **`gk.Resizable_Panel`** (ResizablePanel, element): `gk.resizable_panel()`; `->fixed`, `->size_range`, `->visible`
- **`gk.Resizable_Panel_Group`** (ResizablePanelGroup, element): `gk.h_resizable(id)` or `gk.kit_h_resizable(id)` or `gk.kit_v_resizable(id)` (and 2 more); `->child`, `->children`, `->axis`, `->on_resize`, `->with_handle_appearance`, `->with_state`
- **`gk.Resize_Handle`** (ResizeHandle<Drag, element): `gk.kit_resize_handle(id, axis)` or `gk.resize_handle(id, axis)`; `->on_click`, `->tooltip`, `->inside`, `->lock_scroll_axis`, `->on_double_click`, `->with_appearance`
- **`gk.Rounded_Frame_Cover`** (RoundedFrameCover, element): `gk.rounded_frame_cover_uniform(radius)`; `->backdrop`, `->frame_border`
- **`gk.Scroll_Bounce`** (ScrollBounce, element): `gk.scroll_bounce(id, handle, child)`; `->enabled`, `->motion`, `->scroll_wheel`
- **`gk.Scrollable_Mask`** (ScrollableMask<ScrollHandle>, element): `gk.scrollable_mask_vertical_wheel(scroll_handle)` or `gk.scrollable_mask(axis, scroll_handle)`; `->debug`
- **`gk.Scrollbar`** (Scrollbar, element): `gk.scrollbar(scroll_handle)` or `gk.scrollbar_horizontal(scroll_handle)` or `gk.scrollbar_vertical(scroll_handle)`; `->axis`, `->mode`, `->scroll_size`, `->styles`, `->viewport_bounds`, `->viewport_from_layout`
- **`gk.Selectable_Text`** (SelectableText, element): `gk.selectable_text(id, text)` or `gk.selectable_text_with_handle(id, handle, text)`; `->document_order`, `->selection_color`, `->text_style`
- **`gk.Slider_Indicator`** (SliderIndicator, element): `gk.slider_indicator(state)`
- **`gk.Slider_Thumb`** (SliderThumb, element): `gk.slider_thumb(state)`; `->disabled`, `->axis`, `->start`
- **`gk.Slider_Track`** (SliderTrack, element): `gk.slider_track(state)`; `->disabled`, `->axis`
- **`gk.Switch_Thumb`** (SwitchThumb, element): `gk.switch_thumb(checked)`; `->disabled`, `->styles`
- **`gk.Switch_Track`** (SwitchTrack, element): `gk.switch_track(id)`; `->checked`, `->disabled`, `->styles`
- **`gk.Tabs`** (Tabs, element): `gk.tabs(id)`
- **`gk.Time_Field_Segment`** (TimeFieldSegment, element): `gk.time_field_segment_handed(index)`; `->clear_children`
- **`gk.Toast`** (Toast, element): `gk.toast(id)`; `->transition_status`
- **`gk.Toast_Stack`** (ToastStack, element): `gk.toast_stack(id, state)`; `->focus_handle`, `->item`, `->motion`, `->placement`
- **`gk.Tooltip_Positioner`** (TooltipPositioner, element): `gk.tooltip_positioner(trigger_bounds)`; `->placement`
- **`gk.Auto_Scroll`** (AutoScroll, kit state): handed out by the kit (no constructor); reads `->delta`, `->is_active`; writes `->stop`
- **`gk.Base_Theme`** (Theme, kit state): handed out by the kit (no constructor); reads `->appearance`, `->cursor_blink`, `->plot`, `->resizable`, `->scrollbar`, `->tokens`; writes `->set_appearance`, `->set_cursor_blink`, `->set_plot`, `->set_resizable`, `->set_scrollbar`, `->set_tokens`
- **`gk.Base_Tree_Entry`** (TreeEntry, kit state): handed out by the kit (no constructor); reads `->depth`, `->is_disabled`, `->is_expanded`, `->is_folder`, `->is_root`, `->item`
- **`gk.Calendar_State`** (CalendarState, kit state): `gk.calendar_state(cx)->…->build()`; shown by `gk.base_calendar(state)`, `gk.calendar(state)`; built with `->disabled_matcher`, `->year_range`; reads `->activate_date`, `->apply_date`, `->current_month`, `->current_year`, `->date`, `->days`, `->disabled_matcher_ref`, `->has_next_year_page` and 10 more; writes `->next_month`, `->prev_month`, `->select_month`, `->select_year`, `->set_date`, `->set_disabled_matcher`, `->set_number_of_months`, `->set_view` and 1 more; events `->on_calendar_event(cx, gk.on(p))`
- **`gk.Color_Picker_State`** (ColorPickerState, kit state): `gk.color_picker_state(cx)->…->build()`; shown by `gk.color_picker(state)`, `gk.color_select(state)`; built with `->default_value`; reads `->value`, `->active_tab`, `->commit_hex`, `->displayed_color`, `->hex_input`, `->is_open`, `->preview`, `->preview_hex` and 2 more; writes `->clear_preview`, `->clear_value`, `->preview_color`, `->select_color`, `->set_active_tab`, `->set_open`, `->set_value`, `->sync_pending_value` and 2 more; events `->on_color_picker_event(cx, gk.on(p))`
- **`gk.Dialog_Handle`** (DialogHandle, kit state): handed out by the kit (no constructor); reads `->is_open`; writes `->close`, `->open`
- **`gk.Discrete`** (Discrete<f32>, kit state): handed out by the kit (no constructor); reads `->sample`, `->switch_at`
- **`gk.Grid_Layout_State`** (GridLayoutState, kit state): `gk.grid_layout_state(cx)->…->build()`; shown by `gk.grid_layout(state)`, `gk.kit_grid_layout(state)`; built with `->cols`, `->compaction`, `->default_areas`, `->draggable`, `->prevent_collision`, `->resizable`; reads `->active_item`, `->area`, `->areas`, `->column_count`, `->compaction_axis`, `->is_collision_prevented`, `->is_draggable`, `->is_resizable` and 3 more; writes `->set_areas`, `->set_cols`, `->set_compaction`, `->set_draggable`, `->set_prevent_collision`, `->set_resizable`, `->set_selected_item`; events `->on_grid_layout_event(cx, gk.on(p))`
- **`gk.History`** (History<SharedString>, kit state): `gk.history(cx)->…->build()`; built with `->max_entries`; reads `->back`, `->can_back`, `->can_forward`, `->current`, `->entries`, `->forward`, `->forward_entries`, `->remove_current`; writes `->clear`, `->push`, `->replace_current`, `->retain`
- **`gk.Horizontal_Scroll_Claim`** (HorizontalScrollClaim, kit state): handed out by the kit (no constructor); reads `->at`, `->delta_x`
- **`gk.Hsla_Sliders`** (HslaSliders, kit state): handed out by the kit (no constructor); reads `->alpha`, `->hue`, `->lightness`, `->saturation`
- **`gk.Inline_Render_Context`** (InlineRenderContext, kit state): handed out by the kit (no constructor); reads `->font_size`, `->line_height`, `->rem_size`, `->text_style`
- **`gk.Keyframes`** (Keyframes<f32>, kit state): handed out by the kit (no constructor); reads `->is_empty`, `->len_`, `->sample`
- **`gk.Markdown_Node_Read`** (MarkdownNode, kit state): handed out by the kit (no constructor); reads `->accessibility_name`, `->as_markdown`, `->as_text`, `->name`, `->source_range`
- **`gk.Nav_Stack_State`** (NavStackState, kit state): handed out by the kit (no constructor); shown by `gk.nav_stack(state)`; reads `->current`, `->depth`, `->forward`, `->forward_views`, `->is_empty`, `->pop`, `->pop_to_root`, `->replace` and 1 more; writes `->clear`, `->push`; events `->on_nav_stack_event(cx, gk.on(p))`
- **`gk.Otp_State`** (OtpState, kit state): `gk.otp_state(cx, length)->…->build()`; shown by `gk.base_otp_input(state)`, `gk.otp_input(state)`; built with `->default_value`, `->masked`; reads `->value`, `->cursor_visible`, `->is_empty`, `->is_masked`, `->len_`, `->try_value`; writes `->focus`, `->set_masked`, `->set_value`; events `->on_otp_event(cx, gk.on(p))`
- **`gk.Plot_Theme_Read`** (PlotTheme, kit state): handed out by the kit (no constructor); reads `->motion`
- **`gk.Popover_State`** (PopoverState, kit state): handed out by the kit (no constructor); reads `->is_open`; writes `->dismiss`, `->show`
- **`gk.Range_Highlight`** (RangeHighlight, kit state): handed out by the kit (no constructor); reads `->background`, `->range`
- **`gk.Rendered_Text`** (RenderedText, kit state): handed out by the kit (no constructor); reads `->as_str`, `->is_empty`, `->len_`, `->range_for_source`, `->source`
- **`gk.Resizable_State`** (ResizableState, kit state): handed out by the kit (no constructor); reads `->container_size`, `->sizes`; writes `->clear`, `->insert_panel`, `->remove_panel`, `->reset_panel`, `->resize_panel`; events `->on_resizable_panel_event(cx, gk.on(p))`
- **`gk.Root`** (Root, kit state): handed out by the kit (no constructor); reads `->view`
- **`gk.Scrollbar_Motion_Read`** (ScrollbarMotion, kit state): handed out by the kit (no constructor); reads `->enter`, `->entrance`, `->exit`, `->expand`, `->idle`, `->thumb_hover_entrance`
- **`gk.Scrollbar_Theme_Read`** (ScrollbarTheme, kit state): handed out by the kit (no constructor); reads `->mode`, `->motion`
- **`gk.Sequence`** (Sequence<f32>, kit state): `gk.sequence(cx, id, from)->…->build()`; built with `->with_step`, `->with_steps`; reads `->from`, `->sample`, `->steps`
- **`gk.Sequence_Sample`** (SequenceSample<f32>, kit state): handed out by the kit (no constructor); reads `->value`, `->into_value`, `->is_finished`, `->status`, `->step`, `->try_value`
- **`gk.Sequence_Step`** (SequenceStep<f32>, kit state): handed out by the kit (no constructor); reads `->target`
- **`gk.Stagger`** (Stagger, kit state): handed out by the kit (no constructor); reads `->delay`
- **`gk.Task_Toggle_Event`** (TaskToggleEvent, kit state): handed out by the kit (no constructor); reads `->checked`, `->item_range`, `->marker_range`
- **`gk.Text_Selection_Endpoint`** (TextSelectionEndpoint, kit state): handed out by the kit (no constructor); reads `->content_point`
- **`gk.Text_Selection_Handle`** (TextSelectionHandle, kit state): handed out by the kit (no constructor); reads `->has_local_selection`, `->snapshot`; writes `->paint_touch_handles`, `->set_fallback_copy_text`, `->set_local_selection`
- **`gk.Text_Selection_Snapshot`** (TextSelectionSnapshot, kit state): handed out by the kit (no constructor); reads `->anchor`, `->coverage`, `->cursor`, `->is_selecting`, `->window_points`
- **`gk.Text_Selection_Window_Points`** (TextSelectionWindowPoints, kit state): handed out by the kit (no constructor); reads `->anchor`, `->cursor`
- **`gk.Text_View_Defaults`** (TextViewDefaults, kit state): `gk.text_view_defaults(cx)->…->build()` or `gk.text_view_defaults_global(cx)->…->build()`; built with `->with_code_block_highlighter`, `->with_inherit_text_color`, `->with_style`; reads `->has_code_block_highlighter`, `->inherit_text_color`; writes `->install`
- **`gk.Text_View_State`** (TextViewState, kit state): `gk.text_view_state_html(cx, text)->…->build()` or `gk.text_view_state_markdown(cx, text)->…->build()`; shown by `gk.base_text_view(state)`, `gk.text_view(state)`; built with `->motion`, `->scrollable`, `->selectable`, `->selection_format`; reads `->is_clamped`, `->rendered_text`, `->reveal_range`, `->selected_source_range`, `->selected_text`, `->set_range_highlights`; writes `->clear_range_highlights`, `->clear_selection`, `->invalidate_inline_layout`, `->push_str`, `->select_all`, `->set_motion`, `->set_scrollable`, `->set_selectable` and 2 more
- **`gk.Time_Field_State`** (TimeFieldState, kit state): `gk.time_field_state(cx)->…->build()`; shown by `gk.base_time_field(state)`, `gk.time_field(state)`; built with `->hour_cycle`, `->precision`; reads `->selected_segment`, `->time`; writes `->focus`, `->set_hour_cycle`, `->set_precision`, `->set_time`; events `->on_time_field_event(cx, gk.on(p))`
- **`gk.Toast_Manager`** (ToastManager<u64, kit state): handed out by the kit (no constructor); reads `->advance`, `->dismiss`, `->dismiss_all`, `->get`, `->is_empty`, `->iter`, `->len_`, `->push` and 1 more
- **`gk.Toast_Stack_State`** (ToastStackState, kit state): handed out by the kit (no constructor); shown by `gk.toast_stack(state)`; reads `->is_expanded`
- **`gk.Tooltip_Defaults`** (TooltipDefaults, kit state): `gk.tooltip_defaults(cx)->…->build()` or `gk.tooltip_defaults_global(cx)->…->build()`; built with `->with_grace_period`, `->with_show_delay`; reads `->grace_period`, `->show_delay`; writes `->install`
- **`gk.Tooltip_Overlay`** (TooltipOverlay, kit state): `gk.tooltip_overlay(cx)->…->build()`; writes `->hide`, `->request_hide`, `->request_show`
- **`gk.Touch_Selection_Snapshot`** (TouchSelectionSnapshot, kit state): handed out by the kit (no constructor); reads `->bounds`, `->dragging`, `->edge`, `->end`, `->is_edge_visible`, `->is_empty`, `->is_menu_open`, `->start`
- **`gk.Transition_State`** (TransitionState<f32>, kit state): handed out by the kit (no constructor); reads `->sample`
- **`gk.Tree_State`** (TreeState, kit state): `gk.tree_state(cx)->…->build()`; shown by `gk.base_tree(state)`, `gk.kit_tree(state)`, `gk.tree(state)`; built with `->items`; reads `->entry`, `->index_of`, `->scroll_handle`, `->selected_entry`, `->selected_index`, `->selected_item`; writes `->focus`, `->reveal_item`, `->scroll_to_item`, `->set_items`, `->set_selected_index`, `->set_selected_item`; events `->on_tree_event(cx, gk.on(p))`
- **`gk.Undo_History`** (UndoHistory<SharedString>, kit state): `gk.undo_history(cx)->…->build()`; built with `->group_interval`, `->max_undos`; reads `->can_redo`, `->can_undo`, `->is_ignoring`, `->redo`, `->undo`; writes `->clear`, `->end_grouping`, `->push`, `->set_ignoring`, `->start_grouping`
- **`gk.Virtual_List_Scroll_Handle`** (VirtualListScrollHandle, kit state): handed out by the kit (no constructor); reads `->base_handle`; writes `->scroll_to_bottom`, `->scroll_to_item`
- **`gk.Base_Text_View_Style`** (TextViewStyle, value): `gk.base_text_view_style_default()` or `gk.base_text_view_style_from_theme(theme)`; `->with_blockquote_style`, `->with_border`, `->with_code_background`, `->with_code_block`, `->with_dark`, `->with_foreground`, `->with_heading`, `->with_inline_code` and 10 more
- **`gk.Button_Styles`** (ButtonStyles, value): `gk.button_styles_default()`; `->selected`, `->disabled`
- **`gk.Checkbox_Indicator_Styles`** (CheckboxIndicatorStyles, value): `gk.checkbox_indicator_styles_default()`; `->checked`, `->disabled`, `->indeterminate`
- **`gk.Checkbox_Styles`** (CheckboxStyles, value): `gk.checkbox_styles_default()`; `->checked`, `->disabled`, `->indeterminate`
- **`gk.Grid_Area`** (GridArea, value): `gk.grid_area(id)`; `->col`, `->col_span`, `->locked`, `->max_span`, `->min_span`, `->row`, `->row_span`
- **`gk.Grid_Item`** (GridItem, value): `gk.grid_item(id)`; `->accessibility_label`, `->drag_handle_only`
- **`gk.Input_Styles`** (InputStyles, value): `gk.input_styles_default()`; `->disabled`, `->focused`
- **`gk.Link_Styles`** (LinkStyles, value): `gk.link_styles_default()`; `->disabled`
- **`gk.Markdown_Extensions`** (MarkdownExtensions, value): `gk.markdown_extensions_default()`; `->block_surface`, `->frontmatter`, `->mdx`, `->parser_revision`, `->plugin`
- **`gk.Pagination_State`** (PaginationState, value): `gk.pagination_state(current_page, total_pages)`; `->on_change`, `->disabled`, `->visible_pages`
- **`gk.Plot_Motion`** (PlotMotion, value): `gk.plot_motion_default()`; `->with_appear`, `->with_enter`, `->with_exit`, `->with_pointer`
- **`gk.Plot_Theme`** (PlotTheme, value): `gk.plot_theme()` or `gk.plot_theme_default()`; `->with_motion`
- **`gk.Presence`** (Presence, value): `gk.presence(id, present)`; `->transition`
- **`gk.Radio_Styles`** (RadioStyles, value): `gk.radio_styles_default()`; `->checked`, `->disabled`
- **`gk.Scroll_Bounce_Motion`** (ScrollBounceMotion, value): `gk.scroll_bounce_motion_default()`; `->with_max_stretch`, `->with_response`, `->with_tracking`
- **`gk.Scrollbar_Motion`** (ScrollbarMotion, value): `gk.scrollbar_motion_default()`; `->with_enter`, `->with_entrance`, `->with_exit`, `->with_expand`, `->with_idle`, `->with_thumb_hover_entrance`
- **`gk.Scrollbar_Styles`** (ScrollbarStyles, value): `gk.scrollbar_styles_default()`; `->thumb`, `->thumb_active`, `->thumb_hover`, `->track`, `->track_active`, `->track_hover`
- **`gk.Scrollbar_Theme`** (ScrollbarTheme, value): `gk.scrollbar_theme()` or `gk.scrollbar_theme_default()`; `->with_mode`, `->with_motion`, `->with_styles`
- **`gk.Scrollbar_Thumb_Style`** (ScrollbarThumbStyle, value): `gk.scrollbar_thumb_style_default()`; `->bg`, `->inset`, `->min_length`, `->radius`, `->width`
- **`gk.Scrollbar_Track_Style`** (ScrollbarTrackStyle, value): `gk.scrollbar_track_style_default()`; `->bg`, `->border_color`, `->width`
- **`gk.Spring`** (Spring, value): `gk.pointer_spring()` or `gk.spring(response)`; `->with_damping`, `->with_epsilon`, `->with_travel`
- **`gk.Switch_Styles`** (SwitchStyles, value): `gk.switch_styles_default()`; `->checked`, `->disabled`
- **`gk.Switch_Thumb_Styles`** (SwitchThumbStyles, value): `gk.switch_thumb_styles_default()`; `->checked`, `->disabled`
- **`gk.Switch_Track_Styles`** (SwitchTrackStyles, value): `gk.switch_track_styles_default()`; `->checked`, `->disabled`
- **`gk.Tab_Styles`** (TabStyles, value): `gk.tab_styles_default()`; `->selected`, `->disabled`
- **`gk.Text_View_Motion`** (TextViewMotion, value): `gk.text_view_motion_default()`; `->with_stream_fade`, `->with_stream_fade_easing`, `->with_stream_fade_stagger`
- **`gk.Timing`** (Timing, value): `gk.timing(duration)`; `->delay`, `->direction`, `->ease`, `->iterations`
- **`gk.Toggle_Styles`** (ToggleStyles, value): `gk.toggle_styles_default()`; `->disabled`, `->pressed`
- **`gk.Tooltip_Request`** (TooltipRequest, value): `gk.tooltip_request(trigger_bounds, build)`; `->with_placement`, `->with_show_delay`
- **`gk.Transition`** (Transition, value): `gk.transition(duration)`; `->delay`, `->ease`, `->easing`, `->reduced_motion`
- **`gk.Tree_Item`** (TreeItem, value): `gk.tree_item(id, label)`; `->child`, `->children`, `->disabled`, `->expanded`

### base::animation

- **`gk.Effect_Transition`** (EffectTransition, value): `gk.effect_transition(duration)`; `->ease`, `->fade`, `->height`, `->slide_x`, `->slide_y`, `->width`

### base::dock

- **`gk.Dock`** (Dock, kit state): handed out by the kit (no constructor); reads `->is_collapsible`, `->is_open`, `->is_resizing`, `->size`; writes `->set_collapsible`, `->set_open`, `->set_resizing`, `->set_size`
- **`gk.Dock_Area`** (DockArea, kit state): `gk.dock_area(cx, id, version)->…->build()`; built with `->with_renderer`; reads `->bounds`, `->dock_size`, `->dump`, `->has_dock`, `->id`, `->is_dock_collapsible`, `->is_dock_open`, `->is_empty` and 7 more; writes `->add_panel`, `->add_panel_view`, `->move_panel`, `->remove_dock`, `->remove_panel`, `->select_panel`, `->set_center`, `->set_dock` and 9 more; events `->on_dock_event(cx, gk.on(p))`
- **`gk.Dock_Context`** (DockContext, kit state): handed out by the kit (no constructor); reads `->is_collapsible`, `->is_open`, `->placement`, `->size`; writes `->end_resize`, `->resize_to`, `->toggle`
- **`gk.Dock_State`** (DockState, kit state): handed out by the kit (no constructor); reads `->open`, `->panel`, `->placement`, `->size`
- **`gk.Drag_Panel`** (DragPanel, kit state): handed out by the kit (no constructor); reads `->drag_offset`, `->drag_session_id`, `->panel`, `->preview_size`, `->source`; writes `->set_drag_offset`, `->set_preview_size`
- **`gk.Drop_Indicator`** (DropIndicator, kit state): handed out by the kit (no constructor); reads `->bounds`, `->drag_session_id`, `->epoch`, `->from`, `->placement`, `->to`
- **`gk.Drop_Placeholder_Bounds`** (DropPlaceholderBounds, kit state): handed out by the kit (no constructor); reads `->origin`, `->size`
- **`gk.Drop_Target`** (DropTarget, kit state): handed out by the kit (no constructor); reads `->node`, `->placement`
- **`gk.Edit_Result`** (EditResult, kit state): handed out by the kit (no constructor); reads `->changed`
- **`gk.Pane_Node`** (PaneNode, kit state): handed out by the kit (no constructor); reads `->id`
- **`gk.Pane_Tree`** (PaneTree, kit state): handed out by the kit (no constructor); reads `->contains_panel`, `->find_node`, `->find_panel_node`, `->insert_panel`, `->move_panel`, `->node_ids`, `->panels`, `->remove_panel` and 5 more; writes `->normalize`
- **`gk.Tab_Group`** (TabGroup, kit state): handed out by the kit (no constructor); built with `->with_renderer`; reads `->active_ix`, `->active_panel`, `->context_`, `->is_closable`, `->is_collapsed`, `->is_zoomed`, `->node`, `->panels`; writes `->close_panel`, `->select_tab`, `->toggle_zoom`; events `->on_tab_group_event(cx, gk.on(p))`
- **`gk.Tab_Group_Context`** (TabGroupContext, kit state): handed out by the kit (no constructor); reads `->active_ix`, `->active_panel`, `->drag_panel`, `->drop_indicator`, `->is_closable`, `->is_collapsed`, `->is_draggable`, `->is_droppable` and 5 more; writes `->close`, `->drop_item`, `->drop_panel`, `->select_tab`, `->toggle_zoom`
- **`gk.Dock_Layout`** (DockLayout, value): `gk.dock_layout_h_split()` or `gk.dock_layout_tabs()` or `gk.dock_layout_v_split()`; `->child`, `->active_index`, `->panel`, `->panel_view`
- **`gk.Dock_Sizing`** (DockSizing, value): `gk.dock_sizing(placement)`; `->with_area_bounds`, `->with_area_height`, `->with_area_width`, `->with_opposite_dock_size`

### base::input

- **`gk.Code_Action_Menu_State`** (CodeActionMenuState, kit state): handed out by the kit (no constructor); reads `->revision`
- **`gk.Completion_Menu_State`** (CompletionMenuState, kit state): handed out by the kit (no constructor); reads `->revision`
- **`gk.Cursor_Blink_Read`** (CursorBlink, kit state): handed out by the kit (no constructor); reads `->idle_timeout`, `->interval`, `->is_steady`
- **`gk.Diagnostic_Set`** (DiagnosticSet, kit state): handed out by the kit (no constructor); reads `->for_offset`, `->is_empty`, `->iter`, `->len_`, `->range`; writes `->clear`, `->extend`, `->push`
- **`gk.Display_Map`** (DisplayMap, kit state): handed out by the kit (no constructor); reads `->text`, `->buffer_line_count`, `->buffer_line_to_display_row`, `->buffer_line_to_display_row_range`, `->buffer_pos_to_display_pos`, `->display_pos_to_buffer_pos`, `->display_row_count`, `->display_row_to_buffer_line` and 9 more; writes `->clear_folds`, `->on_layout_changed`, `->set_fold_candidates`, `->set_folded`, `->set_font`, `->set_wrapping_indent`, `->toggle_fold`
- **`gk.Editor_Scrollbar_Context`** (EditorScrollbarContext, kit state): handed out by the kit (no constructor); reads `->axis`, `->scroll_viewport_size`
- **`gk.Editor_State`** (EditorState, kit state): `gk.editor_state(cx)->…->build()`; shown by `gk.editor(state)`; built with `->placeholder`, `->autocapitalize`, `->autocorrect`, `->clean_on_escape`, `->context_menu`, `->cursor_surrounding_lines`, `->default_value`, `->gutter_decorations_width` and 6 more; reads `->text`, `->value`, `->context_menu_capabilities`, `->create_decorations_collection`, `->create_gutter_decorations_collection`, `->create_overview_ruler_decorations_collection`, `->create_range_decorations_collection`, `->cursor` and 55 more; writes `->add_selection`, `->apply_highlighter_fold_candidates`, `->begin_edge_drag`, `->clean`, `->clear_diagnostic_popover`, `->clear_hover_state`, `->close_edit_menu`, `->close_search` and 61 more; events `->on_input_event(cx, gk.on(p))`, `->on_input_edits_event(cx, gk.on(p))`
- **`gk.Gutter_Decoration_Collection`** (GutterDecorationCollection, kit state): handed out by the kit (no constructor); reads `->get_ranges`; writes `->append`, `->clear`, `->dispose`, `->set`
- **`gk.Inline_Token_Click_Event`** (InlineTokenClickEvent, kit state): handed out by the kit (no constructor); reads `->bounds`, `->click`, `->range`, `->token`
- **`gk.Inline_Token_Context`** (InlineTokenContext, kit state): handed out by the kit (no constructor); reads `->available_width`, `->is_disabled`, `->is_readonly`, `->is_selected`, `->line_height`, `->range`, `->token`
- **`gk.Inline_Token_Hover_Event`** (InlineTokenHoverEvent, kit state): handed out by the kit (no constructor); reads `->bounds`, `->is_hovered`, `->range`, `->range_utf16`, `->token`
- **`gk.Inline_Token_Read`** (InlineToken, kit state): handed out by the kit (no constructor); reads `->label`, `->text`, `->id`
- **`gk.Inline_Token_Span`** (InlineTokenSpan, kit state): handed out by the kit (no constructor); reads `->range`, `->token`
- **`gk.Input_Content`** (InputContent, kit state): handed out by the kit (no constructor); reads `->text`, `->tokens`, `->with_token`
- **`gk.Input_Context_Menu_Capabilities_Read`** (InputContextMenuCapabilities, kit state): handed out by the kit (no constructor); reads `->has_code_actions`, `->has_definition`, `->has_selection`, `->is_code_editor`, `->is_copyable`, `->is_disabled`, `->is_editable`, `->is_masked` and 1 more
- **`gk.Input_Presentation`** (InputPresentation, kit state): handed out by the kit (no constructor); reads `->placeholder`, `->focus_handle`, `->is_code_editor`, `->is_disabled`, `->is_editable`, `->is_loading`, `->is_masked`, `->is_multi_line` and 3 more
- **`gk.Input_State`** (InputState, kit state): `gk.input_state(cx)->…->build()`; shown by `gk.base_input(state)`, `gk.base_number_input(state)`, `gk.input(state)`, `gk.number_input(state)`, `gk.questionnaire_input_definition(state)`; built with `->placeholder`, `->autocapitalize`, `->autocorrect`, `->clean_on_escape`, `->context_menu`, `->cursor_surrounding_lines`, `->default_value`, `->input_action` and 8 more; reads `->text`, `->value`, `->content`, `->context_menu_capabilities`, `->cursor`, `->cursor_position`, `->diagnostic_popover`, `->diagnostics` and 49 more; writes `->add_selection`, `->apply_highlighter_fold_candidates`, `->begin_edge_drag`, `->clean`, `->close_edit_menu`, `->close_search`, `->end_edge_drag`, `->ensure_highlighter_factory` and 45 more; events `->on_input_event(cx, gk.on(p))`, `->on_input_edits_event(cx, gk.on(p))`
- **`gk.Overview_Ruler_Decoration_Collection`** (OverviewRulerDecorationCollection, kit state): handed out by the kit (no constructor); reads `->get_ranges`; writes `->append`, `->clear`, `->dispose`, `->set`
- **`gk.Range_Decoration_Collection`** (RangeDecorationCollection, kit state): handed out by the kit (no constructor); reads `->get_ranges`; writes `->append`, `->clear`, `->dispose`, `->set`
- **`gk.Search_Matcher`** (SearchMatcher, kit state): handed out by the kit (no constructor); reads `->label`, `->current`, `->current_match_index`, `->is_empty`, `->len_`, `->matched_ranges`; writes `->update_cursor_by_offset`, `->update_query`
- **`gk.Search_Session`** (SearchSession, kit state): handed out by the kit (no constructor); reads `->is_active`
- **`gk.Text_Decoration_Collection`** (TextDecorationCollection, kit state): handed out by the kit (no constructor); reads `->get_ranges`; writes `->append`, `->clear`, `->set`
- **`gk.Textarea_State`** (TextareaState, kit state): `gk.textarea_state(cx)->…->build()`; shown by `gk.base_textarea(state)`, `gk.textarea(state)`; built with `->placeholder`, `->auto_grow`, `->autocapitalize`, `->autocorrect`, `->clean_on_escape`, `->context_menu`, `->cursor_surrounding_lines`, `->default_value` and 5 more; reads `->text`, `->value`, `->content`, `->context_menu_capabilities`, `->cursor`, `->cursor_position`, `->diagnostic_popover`, `->diagnostics` and 49 more; writes `->add_selection`, `->apply_highlighter_fold_candidates`, `->begin_edge_drag`, `->clean`, `->close_edit_menu`, `->close_search`, `->end_edge_drag`, `->ensure_highlighter_factory` and 46 more; events `->on_input_event(cx, gk.on(p))`, `->on_input_edits_event(cx, gk.on(p))`
- **`gk.Auto_Closing_Pair`** (AutoClosingPair, value): `gk.auto_closing_pair(open, close)`; `->not_in_`
- **`gk.Bracket_Pair`** (BracketPair, value): `gk.bracket_pair(open, close)`
- **`gk.Cursor_Blink`** (CursorBlink, value): `gk.cursor_blink_default()` or `gk.cursor_blink_steady()` or `gk.cursor_blink_blinking(interval)`; `->with_idle_timeout`
- **`gk.Diagnostic`** (Diagnostic, value): `gk.diagnostic_default()` or `gk.diagnostic(range, message)`; `->with_code`, `->with_severity`, `->with_source`
- **`gk.Gutter_Decoration`** (GutterDecoration, value): `gk.gutter_decoration_default()` or `gk.gutter_decoration(range)`; `->with_color`, `->with_folded_color`
- **`gk.Inline_Token`** (InlineToken, value): `gk.inline_token(id, text)`; `->with_label`
- **`gk.Input_Accessibility`** (InputAccessibility, value): `gk.input_accessibility(role)`; `->with_accessibility_id`, `->with_label`, `->with_placeholder`, `->with_value_exposed`
- **`gk.Language_Config`** (LanguageConfig, value): `gk.language_config_default()`; `->auto_close_before`, `->auto_closing_pairs`, `->brackets`, `->indentation_rules`
- **`gk.Lsp`** (Lsp, value): `gk.lsp_default()`; `->with_completion_highlighter`
- **`gk.Overview_Ruler_Decoration`** (OverviewRulerDecoration, value): `gk.overview_ruler_decoration_default()` or `gk.overview_ruler_decoration(range)`; `->with_color`, `->with_lane`
- **`gk.Range_Decoration`** (RangeDecoration, value): `gk.range_decoration(range)`; `->with_color`, `->with_style`

### base::plot

- **`gk.Plot_Appear_Scope`** (PlotAppearScope, element): `gk.plot_appear_scope(id, child)`
- **`gk.Path_Cache`** (PathCache, kit state): handed out by the kit (no constructor); reads `->is_warm`
- **`gk.Pie`** (Pie<Datum>, kit state): `gk.pie(cx)->…->build()`; built with `->value`, `->end_angle`, `->pad_angle`, `->start_angle`; reads `->arcs`
- **`gk.Plot_Appear`** (PlotAppear, kit state): handed out by the kit (no constructor); reads `->is_appearing`, `->progress`, `->staggered`
- **`gk.Plot_Label`** (PlotLabel, kit state): handed out by the kit (no constructor); writes `->paint`
- **`gk.Sankey_Graph`** (SankeyGraph, kit state): handed out by the kit (no constructor); reads `->layer_count`
- **`gk.Scale_Band`** (ScaleBand<SharedString>, kit state): `gk.scale_band(cx, domain, range)->…->build()`; built with `->band_count`, `->max_band_width`, `->padding_inner`, `->padding_outer`; reads `->band_width`, `->nearest_index`, `->step`, `->tick`
- **`gk.Scale_Linear`** (ScaleLinear<f64>, kit state): handed out by the kit (no constructor); reads `->nearest_index`, `->tick`
- **`gk.Scale_Ordinal`** (ScaleOrdinal<SharedString, kit state): `gk.scale_ordinal(cx, domain, range)->…->build()`; built with `->domain`, `->range`, `->unknown`; reads `->map_`
- **`gk.Scale_Point`** (ScalePoint<SharedString>, kit state): handed out by the kit (no constructor); reads `->nearest_index`, `->tick`, `->tick_at`
- **`gk.Shape_Key`** (ShapeKey, kit state): handed out by the kit (no constructor); reads `->finish`
- **`gk.Stack`** (Stack<Datum>, kit state): `gk.stack(cx)->…->build()`; built with `->value`, `->data`, `->keys`; reads `->series`
- **`gk.Area_Shape`** (Area<Datum>, value): `gk.area_shape()` or `gk.area_shape_default()`; `->curve`, `->data`, `->fill`, `->stroke`, `->x`, `->y0`, `->y1`
- **`gk.Axis_Text`** (AxisText, value): `gk.axis_text(text, tick, color)`; `->align`, `->font_size`
- **`gk.Bar_Shape`** (Bar<Datum>, value): `gk.bar_shape()` or `gk.bar_shape_default()`; `->label`, `->value`, `->alignment`, `->band_width`, `->base`, `->corner_radii`, `->cross`, `->data` and 1 more
- **`gk.Grid`** (Grid, value): `gk.grid()` or `gk.grid_default()`; `->dash_array`, `->stroke`, `->x`, `->y`
- **`gk.Line_Shape`** (Line<Datum>, value): `gk.line_shape()` or `gk.line_shape_default()`; `->curve`, `->data`, `->dot`, `->dot_fill`, `->dot_size`, `->dot_stroke`, `->stroke`, `->stroke_width` and 2 more
- **`gk.Plot_Axis`** (PlotAxis, value): `gk.plot_axis()` or `gk.plot_axis_default()`; `->stroke`, `->x`, `->x_axis`, `->x_label`, `->x_label_side`, `->y`, `->y_axis`, `->y_label` and 1 more
- **`gk.Radial_Line_Shape`** (RadialLine<Datum>, value): `gk.radial_line_shape()` or `gk.radial_line_shape_default()`; `->angle`, `->closed`, `->data`, `->dot`, `->dot_fill`, `->dot_size`, `->dot_stroke`, `->fill` and 3 more
- **`gk.Sankey`** (Sankey, value): `gk.sankey()` or `gk.sankey_default()`; `->extent`, `->iterations`, `->node_align`, `->node_padding`, `->node_width`, `->size`, `->value_scale`
- **`gk.Sankey_Link`** (SankeyLink, value): `gk.sankey_link(source, target, value)`
- **`gk.Text`** (Text, value): `gk.base_text(text, origin, color)`; `->align`, `->font_size`, `->font_weight`

### base::questionnaire

- **`gk.Questionnaire_Answer_Change`** (QuestionnaireAnswerChange, kit state): handed out by the kit (no constructor); reads `->answer`, `->item`, `->status`
- **`gk.Questionnaire_Answer_Read`** (QuestionnaireAnswer, kit state): handed out by the kit (no constructor); reads `->choices`, `->freeform`, `->is_empty`
- **`gk.Questionnaire_Answers`** (QuestionnaireAnswers, kit state): handed out by the kit (no constructor); reads `->get`, `->is_empty`, `->iter`, `->len_`
- **`gk.Questionnaire_Choice_Definition_Read`** (QuestionnaireChoiceDefinition, kit state): handed out by the kit (no constructor); reads `->value`, `->description`, `->accessibility_label`, `->is_default_selected`, `->is_disabled`, `->try_value`
- **`gk.Questionnaire_Choice_State`** (QuestionnaireChoiceState, kit state): handed out by the kit (no constructor); reads `->value`, `->is_disabled`, `->is_invalid`, `->is_selected`, `->shortcut`, `->try_value`
- **`gk.Questionnaire_Input_Definition_Read`** (QuestionnaireInputDefinition, kit state): handed out by the kit (no constructor); reads `->accessibility_label`, `->is_disabled`, `->state_`
- **`gk.Questionnaire_Item_Definition_Read`** (QuestionnaireItemDefinition, kit state): handed out by the kit (no constructor); reads `->description`, `->accessibility_label`, `->choices`, `->input`, `->is_disabled`, `->is_multiple`, `->is_required`, `->name` and 1 more
- **`gk.Questionnaire_Item_State`** (QuestionnaireItemState, kit state): handed out by the kit (no constructor); reads `->has_input`, `->is_disabled`, `->is_invalid`, `->is_multiple`, `->is_required`, `->name`, `->status`
- **`gk.Questionnaire_Navigation_State`** (QuestionnaireNavigationState, kit state): handed out by the kit (no constructor); reads `->is_confirmable`, `->is_next_visible`, `->is_previous_visible`, `->is_skip_visible`, `->is_submit_visible`
- **`gk.Questionnaire_Progress_State`** (QuestionnaireProgressState, kit state): handed out by the kit (no constructor); reads `->current`, `->total`
- **`gk.Questionnaire_State`** (QuestionnaireState, kit state): `gk.questionnaire_state(cx, items)->…->build()`; shown by `gk.questionnaire(state)`, `gk.questionnaire_actions(state)`, `gk.questionnaire_choice(state)`, `gk.questionnaire_choices(state)`, `gk.questionnaire_description(state)`, `gk.questionnaire_error(state)`, `gk.questionnaire_input(state)`, `gk.questionnaire_item(state)`, `gk.questionnaire_next(state)`, `gk.questionnaire_previous(state)`, `gk.questionnaire_progress(state)`, `gk.questionnaire_skip(state)`, `gk.questionnaire_submit(state)`, `gk.questionnaire_title(state)`; built with `->with_current_item`, `->with_shortcuts`; reads `->activate_choice`, `->activate_shortcut`, `->answer`, `->answers`, `->choice_definition`, `->choice_focus_handle`, `->choice_for_shortcut`, `->choice_position` and 38 more; writes `->reset`; events `->on_questionnaire_event(cx, gk.on(p))`
- **`gk.Questionnaire_Submission`** (QuestionnaireSubmission, kit state): handed out by the kit (no constructor); reads `->answer`, `->items`
- **`gk.Questionnaire_Submission_Item`** (QuestionnaireSubmissionItem, kit state): handed out by the kit (no constructor); reads `->answer`, `->name`, `->status`
- **`gk.Questionnaire_Answer`** (QuestionnaireAnswer, value): `gk.questionnaire_answer()` or `gk.questionnaire_answer_default()`; `->with_choices`, `->with_freeform`
- **`gk.Questionnaire_Choice_Definition`** (QuestionnaireChoiceDefinition, value): `gk.questionnaire_choice_definition(value, accessibility_label)`; `->with_default_selected`, `->with_description`, `->with_disabled`
- **`gk.Questionnaire_Input_Definition`** (QuestionnaireInputDefinition, value): `gk.questionnaire_input_definition(state, accessibility_label)`; `->with_disabled`
- **`gk.Questionnaire_Item_Definition`** (QuestionnaireItemDefinition, value): `gk.questionnaire_item_definition(name, accessibility_label)`; `->with_choice`, `->with_choices`, `->with_description`, `->with_disabled`, `->with_input`, `->with_multiple`, `->with_required`, `->with_validator`

### base::slider

- **`gk.Slider_State`** (SliderState, kit state): `gk.slider_state(cx)->…->build()`; shown by `gk.base_slider(state)`, `gk.slider(state)`, `gk.slider_indicator(state)`, `gk.slider_thumb(state)`, `gk.slider_track(state)`; built with `->default_value`, `->max`, `->min`, `->scale`, `->step`; reads `->value`, `->accessibility_step`, `->max_value`, `->min_value`, `->step_value`, `->try_value`; writes `->set_value`; events `->on_slider_event(cx, gk.on(p))`

### base::text

- **`gk.Base_Table_2`** (Table, kit state): handed out by the kit (no constructor); reads `->column_aligns`, `->rows`
- **`gk.Image_Node`** (ImageNode, kit state): handed out by the kit (no constructor); reads `->title`
- **`gk.Inline_Node_Read`** (InlineNode, kit state): handed out by the kit (no constructor); reads `->text`, `->as_custom`, `->as_image`, `->mark_runs`
- **`gk.Leaf_Geometry`** (LeafGeometry, kit state): handed out by the kit (no constructor); reads `->atom_at`, `->bounds`, `->caret_for_position`, `->caret_height`, `->index_for_position`, `->is_empty`, `->len_`, `->line_height` and 2 more
- **`gk.Paragraph_Read`** (Paragraph, kit state): handed out by the kit (no constructor); reads `->children`
- **`gk.Base_Code_Block`** (CodeBlock, value): `gk.base_code_block_from_code(code, lang)` or `gk.base_code_block(code, lang, span)`; `->with_leaf_key`, `->with_meta`
- **`gk.Flow_Position`** (FlowPosition, value): `gk.flow_position_at(blocks, ix, is_last)`; `->first`, `->last`
- **`gk.Inline_Node`** (InlineNode, value): `gk.inline_node_default()` or `gk.inline_node(text)` or `gk.inline_node_image(image)`; `->marks`
- **`gk.Paragraph`** (Paragraph, value): `gk.paragraph_default()`; `->push`, `->with_leaf_key`
- **`gk.Render_Context`** (RenderContext, value): `gk.render_context_from_app()` or `gk.render_context(style)`; `->on_link_click`, `->with_body_font_size`, `->with_code_block_highlighter`, `->with_foreground_ranges`, `->with_image_source`, `->with_link_reference`, `->with_markdown_extensions`, `->with_style`
- **`gk.Text_Mark`** (TextMark, value): `gk.text_mark_default()`; `->bold`, `->code`, `->highlight`, `->italic`, `->link`, `->merge`, `->strikethrough`, `->underline`

<!-- index:end -->
