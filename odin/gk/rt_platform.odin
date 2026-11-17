package gk

// Requests that answer later and the platform's services (docs/PROTOCOLS.md 11.30): file dialogs,
// a window's prompt, the native menu bar, opening a URL. The library side is core/src/platform.rs.
// Nothing here is Linux-only; rt_platform_linux.odin holds what is.
//
//   gk.prompt_for_paths(cx, {files = true, multiple = true}, gk.on(opened))   // -> gk.Completion
//   opened :: proc(a: ^App, cx: ^gk.Cx, d: gk.Paths_Done) { if d.status == .Answered { .. d.paths .. } }
//   gk.prompt_for_new_path(cx, "/home/me", gk.on(save_to), "notes.txt")
//   gk.prompt(cx, .Warning, "Delete the note?", {{label = "Delete"}, {.Cancel, "Keep"}}, gk.on(answered))
//   gk.set_menus(cx, {name = "File", items = {gk.Menu_Action{name = "Open", action = open}}})
//   gk.open_url(cx, "https://example.com")
//
// A request returns a Completion at once; the answer comes later as one DONE turn to the handler,
// between frames, with the window the request was made in. The view that made the request is
// marked for redraw after it, as after any of its handlers. gk.release(cx, completion) before the
// answer cancels it (no turn comes); after the answer it does nothing. A completion started in a
// view's init or handler goes with that view: releasing the view cancels it.
//
// Offscreen (the diag library's driver), or with GK_SCRIPTED_PLATFORM=1, the platform is not asked:
// a test answers with gk.platform_answer_paths / gk.platform_answer_prompt, and open_url is recorded.

import "base:runtime"

// Done_Status says what happened to a request.
Done_Status :: enum u8 {
	Answered  = 0, // the user chose (paths, a button)
	Cancelled = 1, // the user dismissed it
	Failed    = 2, // the platform could not show it: error says why
}

// Paths_Done is what a file dialog answers: the chosen paths (none when cancelled), lent for the turn.
Paths_Done :: struct {
	status: Done_Status,
	paths:  []string,
	error:  string,
}

// Prompt_Done is what a prompt answers: the index of the button chosen.
Prompt_Done :: struct {
	status: Done_Status,
	answer: u32,
}

// Completion is a request that answers later. gk.release cancels it while it waits.
Completion :: struct {
	h:    u64,
	slot: u32,
}

@(private)
Completion_State :: enum u8 {
	Live,
	Done,
	Released,
}

@(private)
completions: map[u64]Completion_State
@(private)
completion_of: map[u32]u64 // kept slot -> live completion

@(init, private)
platform_shapes :: proc "contextless" () {
	register_shape(SHAPE_PATHS, Paths_Done, nil)
	register_shape(SHAPE_PROMPT, Prompt_Done, nil)
}

@(private)
completion_begin :: proc(cx: ^Cx, op: u16, h: Handler, shape: u16, loc: runtime.Source_Code_Location) -> (req: ^Tape, slot, gen: u32) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not start a request: start it in init or in a handler")
	}
	flush(t, loc)
	slot, gen = keep_handler(cx, h, shape, loc)
	b := new(Tape, context.temp_allocator)
	b^ = make(Tape, 0, 64, context.temp_allocator)
	put_u16(b, op)
	return b, slot, gen
}

@(private)
completion_end :: proc(req: ^Tape, slot, gen: u32, loc: runtime.Source_Code_Location) -> Completion {
	put_u32(req, slot)
	put_u32(req, gen)
	r := Reader{must_call(req[:], loc), 0}
	c := Completion{rd_u64(&r), slot}
	if completions == nil {
		completions = make(map[u64]Completion_State, 8, runtime.heap_allocator())
		completion_of = make(map[u32]u64, 8, runtime.heap_allocator())
	}
	completions[c.h] = .Live
	completion_of[slot] = c.h
	return c
}

@(private)
put_opt_str :: proc(b: ^Tape, s: string) {
	if s == "" {
		put_u8(b, 0)
	} else {
		put_u8(b, 1)
		put_str(b, s)
	}
}

// Path_Prompt is what a file dialog may choose (gpui's PathPromptOptions). prompt: the text of the
// dialog's button, "" for the platform's.
Path_Prompt :: struct {
	files:       bool,
	directories: bool,
	multiple:    bool,
	prompt:      string,
}

// prompt_for_paths shows the platform's dialog for opening files or directories. The handler gets
// a Paths_Done.
prompt_for_paths :: proc(cx: ^Cx, o: Path_Prompt, h: Handler, loc := #caller_location) -> Completion {
	if !o.files && !o.directories {
		stop(loc, "prompt_for_paths: a dialog that may choose neither files nor directories (set files or directories)")
	}
	req, slot, gen := completion_begin(cx, C_PROMPT_PATHS, h, SHAPE_PATHS, loc)
	put_bool(req, o.files)
	put_bool(req, o.directories)
	put_bool(req, o.multiple)
	put_opt_str(req, o.prompt)
	return completion_end(req, slot, gen, loc)
}

// prompt_for_new_path shows the platform's dialog for saving a new file, starting in directory. The
// handler gets a Paths_Done with one path, or none when cancelled.
prompt_for_new_path :: proc(cx: ^Cx, directory: string, h: Handler, suggested_name := "", loc := #caller_location) -> Completion {
	req, slot, gen := completion_begin(cx, C_PROMPT_NEW_PATH, h, SHAPE_PATHS, loc)
	put_str(req, directory)
	put_opt_str(req, suggested_name)
	return completion_end(req, slot, gen, loc)
}

Prompt_Level :: enum u8 {
	Info     = 0,
	Warning  = 1,
	Critical = 2,
}

Prompt_Button_Kind :: enum u8 {
	Other  = 0,
	Ok     = 1,
	Cancel = 2,
}

// Prompt_Button is one answer of a prompt (gpui's PromptButton).
Prompt_Button :: struct {
	kind:  Prompt_Button_Kind,
	label: string,
}

// prompt asks a question in the turn's window (or `window`), with the platform's dialog where it
// has one and gpui's own otherwise (Window::prompt). The handler gets a Prompt_Done whose answer is
// the index of the button.
prompt :: proc(cx: ^Cx, level: Prompt_Level, message: string, answers: []Prompt_Button, h: Handler,
               detail := "", window: Window = {}, loc := #caller_location) -> Completion {
	if len(answers) == 0 {
		stop(loc, "prompt: give at least one answer")
	}
	req, slot, gen := completion_begin(cx, C_WINDOW_PROMPT, h, SHAPE_PROMPT, loc)
	put_u64(req, window.h)
	put_u8(req, u8(level))
	put_str(req, message)
	put_opt_str(req, detail)
	put_u32(req, u32(len(answers)))
	for a in answers {
		put_u8(req, u8(a.kind))
		put_str(req, a.label)
	}
	return completion_end(req, slot, gen, loc)
}

// release_completion: gk.release of a Completion (rt_state.odin).
@(private)
release_completion :: proc(t: ^Turn, c: Completion, loc: runtime.Source_Code_Location) {
	state, ok := completions[c.h]
	if !ok {
		stop(loc, "this completion is not one the app holds: released twice, or not made by a request (gk.prompt_for_paths, gk.prompt)")
	}
	switch state {
	case .Live:
		completions[c.h] = .Released
		delete_key(&completion_of, c.slot)
		drop_handler(c.slot)
		release_handle(t, c.h, loc)
	case .Done:
		delete_key(&completions, c.h) // answered already: nothing to cancel
	case .Released:
		stop(loc, "this completion was released already (a request is released once)")
	}
}

// completion_drop_kept: drop_kept_of (rt_handler.odin) found a kept handler of a view being
// released; when it waits for an answer, the request is cancelled with it.
@(private)
completion_drop_kept :: proc(t: ^Turn, slot: u32, loc: runtime.Source_Code_Location) -> bool {
	h, ok := completion_of[slot]
	if !ok {
		return false
	}
	release_completion(t, Completion{h, slot}, loc)
	return true
}

// done_turn delivers an answer (a DONE turn): the kept handler runs as for an event, then the
// completion is done and its handle given back.
@(private)
done_turn :: proc(cx: ^Cx, r: ^Reader) {
	peek := r^
	_ = rd_u64(&peek)
	_ = rd_u32(&peek)
	slot := rd_u32(&peek)
	event(cx, r)
	if h, ok := completion_of[slot]; ok {
		delete_key(&completion_of, slot)
		completions[h] = .Done
		drop_handler(slot)
		release_handle(cx.turn, h, {})
	}
}

// open_url has the platform open a URL with the user's browser (App::open_url). It runs after the
// handler.
open_url :: proc(cx: ^Cx, url: string, loc := #caller_location) {
	req := make(Tape, 0, 8 + len(url), context.temp_allocator)
	put_u16(&req, C_OPEN_URL)
	put_str(&req, url)
	defer_op(check_cx(cx, loc), req[:], loc)
}

// --- the menu bar ---------------------------------------------------------------------------------

Os_Action :: enum u8 {
	None       = 0,
	Cut        = 1,
	Copy       = 2,
	Paste      = 3,
	Select_All = 4,
	Undo       = 5,
	Redo       = 6,
}

// Menu is a menu of the menu bar, or a submenu.
Menu :: struct {
	name:     string,
	items:    []Menu_Item,
	disabled: bool,
}

Menu_Separator :: struct {}

// Menu_Action is an item that dispatches an action (gk.action); the menu shows its key binding.
Menu_Action :: struct {
	name:      string,
	action:    Action,
	os_action: Os_Action,
	checked:   bool,
	disabled:  bool,
}

// Menu_Submenu is an item that opens another menu.
Menu_Submenu :: struct {
	menu: Menu,
}

// Menu_Services is the operating system's Services menu (macOS; elsewhere nothing).
Menu_Services :: struct {
	name: string,
}

Menu_Item :: union {
	Menu_Separator,
	Menu_Action,
	Menu_Submenu,
	Menu_Services,
}

@(private)
put_menu :: proc(b: ^Tape, m: Menu, depth: int, loc: runtime.Source_Code_Location) {
	if depth > 16 {
		stop(loc, "set_menus: menus nested more than 16 deep")
	}
	put_str(b, m.name)
	put_bool(b, m.disabled)
	put_u32(b, u32(len(m.items)))
	for it in m.items {
		switch x in it {
		case Menu_Separator:
			put_u8(b, 0)
		case Menu_Action:
			if x.action.id == 0 {
				stop(loc, "set_menus: the item %q has no action (make one with gk.action)", x.name)
			}
			put_u8(b, 1)
			put_str(b, x.name)
			put_u32(b, x.action.id)
			put_u8(b, u8(x.os_action))
			put_bool(b, x.checked)
			put_bool(b, x.disabled)
		case Menu_Submenu:
			put_u8(b, 2)
			put_menu(b, x.menu, depth + 1, loc)
		case Menu_Services:
			put_u8(b, 3)
			put_str(b, x.name)
		case:
			stop(loc, "set_menus: an empty menu item (give a gk.Menu_Separator{} for a separator)")
		}
	}
}

// set_menus sets the app's menu bar (App::set_menus): the platform's where it has one (macOS), and
// always the kit's AppMenuBar (gk.app_menu_bar draws it; reload it after a change). Items dispatch
// their action to the focused element, as its key binding would. It runs after the handler.
set_menus :: proc(cx: ^Cx, menus: ..Menu, loc := #caller_location) {
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_SET_MENUS)
	put_u32(&req, u32(len(menus)))
	for m in menus {
		put_menu(&req, m, 0, loc)
	}
	defer_op(check_cx(cx, loc), req[:], loc)
}

@(private)
rd_menu :: proc(r: ^Reader) -> Menu {
	m: Menu
	m.name = string(rd_temp(r))
	m.disabled = rd_bool(r)
	n := int(rd_u32(r))
	items := make([]Menu_Item, n, context.temp_allocator)
	for i in 0 ..< n {
		switch rd_u8(r) {
		case 0:
			items[i] = Menu_Separator{}
		case 1:
			a: Menu_Action
			a.name = string(rd_temp(r))
			a.action = Action{rd_u32(r)}
			a.os_action = Os_Action(rd_u8(r))
			a.checked = rd_bool(r)
			a.disabled = rd_bool(r)
			items[i] = a
		case 2:
			items[i] = Menu_Submenu{rd_menu(r)}
		case 3:
			items[i] = Menu_Services{string(rd_temp(r))}
		}
	}
	m.items = items
	return m
}

// menus gives the menu bar as the platform holds it (App::get_menus), lent for the turn.
menus :: proc(cx: ^Cx, loc := #caller_location) -> []Menu {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_MENUS_GET)
	r := Reader{must_call(req[:], loc), 0}
	n := int(rd_u32(&r))
	out := make([]Menu, n, context.temp_allocator)
	for i in 0 ..< n {
		out[i] = rd_menu(&r)
	}
	return out
}

// --- scripted answers (tests, offscreen) ------------------------------------------------------------

// platform_answer_paths answers the oldest waiting file dialog (new_path: a save dialog), or the
// next one asked when none waits. Only offscreen or with GK_SCRIPTED_PLATFORM=1. Returns whether a
// waiting request took it (its DONE turn comes after this turn).
platform_answer_paths :: proc(cx: ^Cx, status: Done_Status, paths: []string = nil, new_path := false, error := "", loc := #caller_location) -> bool {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_PLATFORM_ANSWER)
	put_u8(&req, new_path ? 2 : 1)
	put_u8(&req, u8(status))
	put_u32(&req, u32(len(paths)))
	for p in paths {
		put_str(&req, p)
	}
	put_str(&req, error)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u8(&r) != 0
}

// platform_answer_prompt answers the oldest waiting prompt with the index of a button.
platform_answer_prompt :: proc(cx: ^Cx, status: Done_Status, answer: int, loc := #caller_location) -> bool {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_PLATFORM_ANSWER)
	put_u8(&req, 3)
	put_u8(&req, u8(status))
	put_u32(&req, u32(answer))
	r := Reader{must_call(req[:], loc), 0}
	return rd_u8(&r) != 0
}

// Platform_Log is what scripted mode recorded: URLs opened, requests made, answers delivered and
// discarded (their completion released first), requests waiting.
Platform_Log :: struct {
	urls:                          []string,
	requests, delivered, discarded: u64,
	waiting:                       u32,
	scripted:                      bool,
}

platform_log :: proc(cx: ^Cx, loc := #caller_location) -> Platform_Log {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_PLATFORM_LOG)
	r := Reader{must_call(req[:], loc), 0}
	l: Platform_Log
	n := int(rd_u32(&r))
	l.urls = make([]string, n, context.temp_allocator)
	for i in 0 ..< n {
		l.urls[i] = string(rd_temp(&r))
	}
	l.requests = rd_u64(&r)
	l.delivered = rd_u64(&r)
	l.discarded = rd_u64(&r)
	l.waiting = rd_u32(&r)
	l.scripted = rd_bool(&r)
	return l
}

// --- gpui's element leftovers (docs/PROTOCOLS.md 13.31) -----------------------------------
//
// Closures upstream calls while it draws or while a drag moves take data instead (app code never runs
// mid-frame); the image caches, scroll anchors and text selection scopes are objects the library
// makes and the app holds until gk.release. The library side is core/src/platform.rs.
//
//   gk.div()->id("bin")->on_drop(gk.on(dropped))->can_drop({values = true, ids = {7}})
//   gk.div()->id("file")->on_drag(v, preview)->external_drag_payload({files = {{path = "/tmp/a.txt"}}})
//   gk.div()->on_drag_move(gk.on(moved))          // moved :: proc(a: ^App, cx: ^gk.Cx, e: gk.Drag_Move_Event)
//   gk.div()->group_drag_over("row", gk.style()->bg(..))
//   gk.div()->with_dynamic_prepaint_order({2, 0, 1})
//   gk.div()->id("chart")->a11y_synthetic_children({{"q1", .List_Item, "Q1: 12"}})
//   cache := gk.image_cache(cx); gk.div()->image_cache(cache); gk.img(src)->image_cache(cache)
//   anchor := gk.scroll_anchor(cx, handle); gk.div()->id("x")->anchor_scroll(anchor); gk.scroll_anchor_to(cx, anchor)
//   scope := gk.text_selection_scope_id(cx); gk.text_selection_scope(gk.div()->child(..), scope)

// Drop_Accept is what an element's can_drop takes, as data: the library's drag values (every one, or
// only those whose id is in ids) and files dragged in from outside the app.
Drop_Accept :: struct {
	values: bool,
	ids:    []u64,
	paths:  bool,
}

@(private)
put_drop_accept :: proc(o: ^Tape, a: Drop_Accept) {
	put_u8(o, u8(a.values || len(a.ids) > 0))
	put_u32(o, u32(len(a.ids)))
	for id in a.ids {
		put_u64(o, id)
	}
	put_u8(o, u8(a.paths))
}

// External_File is a file (or a directory) handed to the platform when a drag leaves the window.
External_File :: struct {
	path:      string,
	directory: bool,
}

// External_Drag_Payload is what external_drag_payload takes: the files a drag of the app's value
// becomes outside the window (none: the drag stays inside the app).
External_Drag_Payload :: struct {
	files: []External_File,
}

@(private)
put_external_drag_payload :: proc(o: ^Tape, p: External_Drag_Payload) {
	put_u32(o, u32(len(p.files)))
	for f in p.files {
		put_str(o, f.path)
		put_u8(o, u8(f.directory))
	}
}

// A11y_Child is one synthetic accessibility node a11y_synthetic_children adds under the element: its
// key (its id is derived from it and the element's, as upstream does), its role and its label.
A11y_Child :: struct {
	key:   string,
	role:  Role,
	label: string,
}

// Drag_Move_Event is gpui's DragMoveEvent of the library's drag value: where the pointer is (as a
// Mouse_Event), the bounds of the element listening, and the value being dragged (its text lent for
// the turn).
Drag_Move_Event :: struct {
	using event: Mouse_Event,
	bounds:      Bounds,
	value:       Drag_Value,
}

// Image_Cache is an image cache the library keeps (gpui's RetainAllImageCache): the images drawn under
// an element given it stay loaded until it is released. gk.release frees it.
Image_Cache :: struct {
	h: u64,
}

// Retain_All_Images is gpui's retain-all cache kept by the element id given (retain_all(id)).
Retain_All_Images :: struct {
	id: string,
}

// Image_Cache_Provider is what Div's image_cache takes: a cache the app made, or one kept by id.
Image_Cache_Provider :: union {
	Image_Cache,
	Retain_All_Images,
}

@(private)
put_image_cache_provider :: proc(o: ^Tape, p: Image_Cache_Provider) {
	switch x in p {
	case Image_Cache:
		put_u8(o, 0)
		put_u64(o, x.h)
	case Retain_All_Images:
		put_u8(o, 1)
		put_str(o, x.id)
	case:
		put_u8(o, 1)
		put_str(o, "images")
	}
}

// image_cache makes an image cache (gpui's RetainAllImageCache::new). Keep it in the app's state.
image_cache :: proc(cx: ^Cx, loc := #caller_location) -> Image_Cache {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not make an image cache: make it in init or in a handler")
	}
	flush(t, loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_IMAGE_CACHE_NEW)
	r := Reader{must_call(req[:], loc), 0}
	return {rd_u64(&r)}
}

// Scroll_Anchor keeps an element in view of a scroll handle (gpui's ScrollAnchor): an element given it
// with anchor_scroll is scrolled to by gk.scroll_anchor_to. gk.release frees it.
Scroll_Anchor :: struct {
	h: u64,
}

// scroll_anchor makes an anchor for a scroll handle (ScrollAnchor::for_handle).
scroll_anchor :: proc(cx: ^Cx, handle: Scroll_Handle, loc := #caller_location) -> Scroll_Anchor {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not make a scroll anchor: make it in init or in a handler")
	}
	flush(t, loc)
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_SCROLL_ANCHOR_NEW)
	put_u64(&req, handle.h)
	r := Reader{must_call(req[:], loc), 0}
	return {rd_u64(&r)}
}

// scroll_anchor_to scrolls the anchor's handle so the element given the anchor is in view
// (ScrollAnchor::scroll_to), with the turn's window.
scroll_anchor_to :: proc(cx: ^Cx, a: Scroll_Anchor, loc := #caller_location) {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not scroll: scroll in a handler")
	}
	flush(t, loc)
	req := make(Tape, 0, 10, context.temp_allocator)
	put_u16(&req, C_SCROLL_ANCHOR_TO)
	put_u64(&req, a.h)
	_ = must_call(req[:], loc)
}

// Text_Selection_Scope is a text-selection scope id (TextSelectionScopeId::new): text selected under an
// element wrapped with it stays inside it. Make it once and keep it; gk.release frees it.
Text_Selection_Scope :: struct {
	h: u64,
}

text_selection_scope_id :: proc(cx: ^Cx, loc := #caller_location) -> Text_Selection_Scope {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not make a text selection scope: make it once, in init")
	}
	flush(t, loc)
	req := make(Tape, 0, 2, context.temp_allocator)
	put_u16(&req, C_TEXT_SCOPE_NEW)
	r := Reader{must_call(req[:], loc), 0}
	return {rd_u64(&r)}
}

// Image_Bytes is an encoded image (gpui's Image: a PNG, a JPEG, ..) for ImageSource's Image variant.
Image_Bytes :: struct {
	format: Image_Format,
	bytes:  []u8,
}

// Pixel_Image is an image as pixels, for gk.render_image: width x height pixels of 4 bytes each, red,
// green, blue, alpha, row by row.
Pixel_Image :: struct {
	width, height: u32,
	rgba:          []u8,
}

// Render_Image is an image the library made from pixels (gpui's RenderImage), shown by ImageSource's
// Render and Custom variants. Make it once, with gk.render_image, and keep it while a view shows it:
// gpui keeps each image's texture on the graphics card until the image is released, so pixels made
// into a new image on every frame would be a new texture on every frame. gk.release frees it in every
// window; a view that names it after that stops with a message. New pixels are a new image: make it,
// show it, release the old one.
Render_Image :: struct {
	h: u64,
}

@(private)
put_bytes :: proc(o: ^Tape, b: []u8) {
	put_u32(o, u32(len(b)))
	append(o, ..b)
}

@(private)
put_render_image :: proc(o: ^Tape, i: Render_Image) {
	if i.h == 0 {
		stop(#location(), "a zero gk.Render_Image: make one with gk.render_image(cx, gk.Pixel_Image{..}) and keep it")
	}
	put_u64(o, i.h)
}

// render_image makes an image of p's pixels, converted once to the format gpui draws. Make it in init
// or in a handler, not in a view procedure; gk.release frees it.
render_image :: proc(cx: ^Cx, p: Pixel_Image, loc := #caller_location) -> Render_Image {
	t := check_cx(cx, loc)
	if build.active {
		stop(loc, "a view procedure may not make an image: make it once, in init or in a handler, and keep it")
	}
	if int(p.width) * int(p.height) * 4 != len(p.rgba) {
		stop(loc, "a gk.Pixel_Image of %dx%d pixels holds %d bytes: it needs width x height x 4 (red, green, blue, alpha per pixel)", p.width, p.height, len(p.rgba))
	}
	flush(t, loc)
	req := make(Tape, 0, 16 + len(p.rgba), context.temp_allocator)
	put_u16(&req, C_RENDER_IMAGE_NEW)
	put_u32(&req, p.width)
	put_u32(&req, p.height)
	put_bytes(&req, p.rgba)
	r := Reader{must_call(req[:], loc), 0}
	return {rd_u64(&r)}
}

// --- tests (core/tests/platform, mode elements): the decoders generated code uses, on the library side

test_drop_accept :: proc(cx: ^Cx, a: Drop_Accept, id: u64, loc := #caller_location) -> (takes_value, takes_files: bool) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_PLATFORM_TEST)
	put_u8(&req, 1)
	put_drop_accept(&req, a)
	put_u64(&req, id)
	r := Reader{must_call(req[:], loc), 0}
	return rd_bool(&r), rd_bool(&r)
}

test_drag_payload :: proc(cx: ^Cx, p: External_Drag_Payload, loc := #caller_location) -> []External_File {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_PLATFORM_TEST)
	put_u8(&req, 2)
	put_external_drag_payload(&req, p)
	r := Reader{must_call(req[:], loc), 0}
	out := make([]External_File, int(rd_u32(&r)), context.temp_allocator)
	for &f in out {
		f.path = string(rd_temp(&r))
		f.directory = rd_bool(&r)
	}
	return out
}

test_prepaint_order :: proc(cx: ^Cx, order: []int, loc := #caller_location) -> []int {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 64, context.temp_allocator)
	put_u16(&req, C_PLATFORM_TEST)
	put_u8(&req, 3)
	put_u32(&req, u32(len(order)))
	for x in order {
		put_u64(&req, u64(x))
	}
	r := Reader{must_call(req[:], loc), 0}
	out := make([]int, int(rd_u32(&r)), context.temp_allocator)
	for &x in out {
		x = int(rd_u64(&r))
	}
	return out
}

// test_render_image: the library's image behind i (custom: through ImageSource::Custom's loader): its
// size, its first pixel as gpui keeps it (BGRA, little-endian u32) and gpui's id of it, the same for as
// long as i is kept.
test_render_image :: proc(cx: ^Cx, i: Render_Image, custom := false, loc := #caller_location) -> (w, h, first: u32, id: u64) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 16, context.temp_allocator)
	put_u16(&req, C_PLATFORM_TEST)
	put_u8(&req, 4)
	put_u8(&req, u8(custom))
	put_render_image(&req, i)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u32(&r), rd_u32(&r), rd_u32(&r), rd_u64(&r)
}

// test_images_shown: how many times views have handed gpui an image (Image_Source_Render or
// Image_Source_Custom), how many distinct images (gpui's ids, one texture each) those were, and how
// many times a release dropped an image's texture from a window. The diagnostic library only.
test_images_shown :: proc(cx: ^Cx, loc := #caller_location) -> (times: u64, images: u32, dropped: u64) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 4, context.temp_allocator)
	put_u16(&req, C_PLATFORM_TEST)
	put_u8(&req, 7)
	r := Reader{must_call(req[:], loc), 0}
	return rd_u64(&r), rd_u32(&r), rd_u64(&r)
}

// test_file_drop_entered: gpui's FileDropEvent::Entered as the library writes a handler's payload (its
// paths read back).
test_file_drop_entered :: proc(cx: ^Cx, loc := #caller_location) -> (ev: File_Drop_Event) {
	flush(check_cx(cx, loc), loc)
	req := make(Tape, 0, 4, context.temp_allocator)
	put_u16(&req, C_PLATFORM_TEST)
	put_u8(&req, 5)
	r := Reader{must_call(req[:], loc), 0}
	rd_into(&r, &ev)
	return
}
