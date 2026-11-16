package gk

// The input family of the delegate protocol (docs/PROTOCOLS.md section 9.6). The state
// types (Input_State, Editor_State, Textarea_State) are generated; what is here
// is what an app implements upstream: the editor's completion and hover providers, as
// questions the library asks through a row source's request channel (rt_source.odin) and the
// app answers with gk.answer. The Rust side is core/src/shim_input.rs.

// Provider_Completions asks for the completions at `offset` (bytes) of `text`, the editor's
// whole text. trigger_kind is LSP's: 1 invoked, 2 a trigger character, 3 an incomplete list
// asked again. Answer it with a Completion_Response.
Provider_Completions :: struct {
	text:         Temp_String,
	offset:       int,
	trigger_kind: u8,
	trigger:      Maybe(Temp_String),
}

// Provider_Hover asks for the hover at `offset` of `text`. Answer it with a Hover_Response.
Provider_Hover :: struct {
	text:   Temp_String,
	offset: int,
}

// The lists and editor families' providers (docs/PROTOCOLS.md 9.11), variants 2 to 7.

// Provider_Definitions asks where the symbol at `offset` is defined. Answer []Definition.
Provider_Definitions :: struct {
	text:   Temp_String,
	offset: int,
}

// Provider_Colors asks for the colours written in `text`. Answer []Document_Color.
Provider_Colors :: struct {
	text: Temp_String,
}

// Provider_Code_Actions asks for the code actions of bytes start ..< end. Answer []Code_Action.
Provider_Code_Actions :: struct {
	start, end: int,
}

// Provider_Perform asks the app to perform a code action it offered (its id); the app edits
// the editor with its generated writes and answers true when done.
Provider_Perform :: struct {
	title:           Temp_String,
	id:              Temp_String,
	push_to_history: bool,
}

// Provider_Tokens asks for the semantic tokens of bytes start ..< end. Answer []u32: LSP's
// encoded tokens, five numbers each (delta line, delta start, length, type, modifier bits),
// the types and modifiers named by the legend given to gk.editor_providers.
Provider_Tokens :: struct {
	text:       Temp_String,
	start, end: int,
}

// Provider_Inline asks for an inline completion at `offset`. Answer Maybe(string).
Provider_Inline :: struct {
	text:      Temp_String,
	offset:    int,
	automatic: bool, // typed (true) or asked for (false)
}

Provider_Request :: union #no_nil {
	Provider_Completions,
	Provider_Hover,
	Provider_Definitions,
	Provider_Colors,
	Provider_Code_Actions,
	Provider_Perform,
	Provider_Tokens,
	Provider_Inline,
}

// Text_Range is an LSP range: lines and UTF-16 characters from 0.
Text_Range :: struct {
	start_line, start_char, end_line, end_char: u32,
}

// Definition is one place a symbol is defined.
Definition :: struct {
	uri:       string,
	target:    Text_Range, // the whole definition
	selection: Text_Range, // its name
}

// Document_Color is a colour written in the text, red green blue alpha from 0 to 1.
Document_Color :: struct {
	range: Text_Range,
	rgba:  [4]f32,
}

// Code_Action is one action offered: its title, its LSP kind ("quickfix", "refactor", or
// empty) and an id that comes back in Provider_Perform.
Code_Action :: struct {
	title, kind, id: string,
}

// Provider_Event is what a provider handler receives: the request id to answer and the
// question. Its strings are lent for the turn.
Provider_Event :: struct {
	id:      u64,
	request: Provider_Request,
}

// Completion_Item is one entry of the completion menu: kind 0 for none, else LSP's
// CompletionItemKind (3 function, 6 variable, 14 keyword ...). insert_text defaults to label.
Completion_Item :: struct {
	label:       string,
	kind:        u8,
	detail:      Maybe(string),
	insert_text: Maybe(string),
}

Completion_Response :: struct {
	items:      []Completion_Item,
	incomplete: bool,
}

// Hover_Response is the hover's markdown, or nil for no hover.
Hover_Response :: struct {
	markdown: Maybe(string),
}

@(init, private)
input_shapes :: proc "contextless" () {
	register_shape(SHAPE_PROVIDER, Provider_Event, nil)
}

Editor_Provider :: enum {
	Completions,
	Hover,
	Definitions,
	Document_Colors,
	Code_Actions,
	Semantic_Tokens,
	Inline_Completions,
}

// editor_providers installs the completion and hover providers of an Editor_State: `fn`
// receives every question (a queued event, after the editor's own update) and answers it
// with gk.answer(cx, source, e.id, Completion_Response{..} or Hover_Response{..}), at once or
// later. A completion is asked when a typed character is one of `triggers`. Returns the row
// source that carries the questions; releasing it ends them (the editor's menu then stays
// empty). Needs a window: call it in init or in a handler.
editor_providers :: proc(
	cx: ^Cx,
	editor: Editor_State,
	state: ^$S,
	fn: proc(s: ^S, cx: ^Cx, e: Provider_Event),
	triggers := ".",
	which := bit_set[Editor_Provider]{.Completions, .Hover},
	code_action_id := "gk",
	inline_debounce_ms := 0, // 0: upstream's 300 ms
	token_types: []string = nil, // the semantic tokens' legend
	token_modifiers: []string = nil,
	loc := #caller_location,
) -> Row_Source {
	no_rows :: proc(s: ^S, cx: ^Cx, at: Row_At) -> Element {
		return text("")
	}
	src := row_source(cx, state, 0, no_rows, on_event(fn, loc), SHAPE_PROVIDER, initial = 0, capacity = 16, loc = loc)
	bits: u32
	for p in which {
		bits |= 1 << u32(p)
	}
	req := make(Tape, 0, 32 + len(triggers), context.temp_allocator)
	put_u16(&req, C_EDITOR_PROVIDERS)
	put_u64(&req, editor.h)
	put_u64(&req, src.h)
	put_u32(&req, bits)
	put_str(&req, triggers)
	put_str(&req, code_action_id)
	put_u32(&req, u32(max(inline_debounce_ms, 0)))
	put_u32(&req, u32(len(token_types)))
	for t in token_types {
		put_str(&req, t)
	}
	put_u32(&req, u32(len(token_modifiers)))
	for m in token_modifiers {
		put_str(&req, m)
	}
	state_set(cx, req[:], loc)
	return src
}

// Code_Editor is the kit's styled code editor (gpui_kit::component::input::Editor): what
// gk.editor (the generated base::Editor) is plus the border, the size and the menus of the
// providers. Upstream declares it in a doc(hidden) module, so the generator has no item for
// it; this is its hand-written node (core/src/shim_input.rs).
Code_Editor :: struct {
	using el: Element,
	using vt: ^Code_Editor_VT,
}

Code_Editor_VT :: struct {
	using t_styled: ^Styled_Trait(Code_Editor),
}

// code_editor shows an Editor_State: height 0 leaves the height to the layout.
code_editor :: proc(
	state: Editor_State,
	height: f32 = 0,
	bordered := true,
	appearance := true,
	disabled := false,
	readonly := false,
	flags: ..Style_Flag,
	loc := #caller_location,
) -> Code_Editor {
	e := Code_Editor{new_node(N_INPUT_EDITOR, loc), &code_editor_vt}
	o := node_args()
	put_u64(o, state.h)
	put_f32(o, height)
	put_u8(o, u8(bordered) | u8(appearance) << 1 | u8(disabled) << 2 | u8(readonly) << 3)
	tokens(e.el, flags)
	return e
}

@(private)
code_editor_vt := Code_Editor_VT {
	t_styled = cast(^Styled_Trait(Code_Editor))&styled_any,
}

// --- sliders --------------------------------------------------------------------------------------

// slider_number is the number a slider's event or value carries: a single slider's value, or a
// range slider's start (upstream's SliderValue::start, a value helper the generator leaves to Odin).
// font_changed :: proc(a: ^App, cx: ^gk.Cx, e: gk.Slider_Event) {a.font_size = gk.slider_number(e)}
slider_number :: proc {
	slider_value_number,
	slider_event_number,
}

slider_value_number :: proc(v: Slider_Value) -> f32 {
	switch x in v {
	case Slider_Value_Single:
		return x.v0
	case Slider_Value_Range:
		return x.v0
	}
	return 0
}

slider_event_number :: proc(e: Slider_Event) -> f32 {
	switch x in e {
	case Slider_Event_Change:
		return slider_value_number(x.v0)
	case Slider_Event_Release:
		return slider_value_number(x.v0)
	}
	return 0
}
