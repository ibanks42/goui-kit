package gallery

// The input family's gallery pages, the Odin twin of ref/src/bin/gallery/input_pages.rs, call
// for call: an input edited by keys, a multi-line textarea, the code editor without grammars, a
// number input, an OTP input, a masked input and the editor's completion menu filled by the
// app's provider (gk.editor_providers). Each page has its own state, made once in init, so a
// page shows only what its own steps did.

import gk "gpuikit:gk"

Input_Pages :: struct {
	edit:   gk.Input_State,
	text:   gk.Textarea_State,
	editor: gk.Editor_State,
	number: gk.Input_State,
	otp:    gk.Otp_State,
	masked: gk.Input_State,
	off:    gk.Input_State,
	completion: gk.Editor_State,
	asks:   gk.Row_Source, // the completion editor's questions
	writes: gk.Input_State,
}

// The input-writes page's click handlers: writes to the state, buffered and applied after the
// handler, as the Rust page's update closures do them at once.
w_select :: proc(a: ^App, cx: ^gk.Cx) {gk.set_selected_range(a.input.writes, cx, {0, 5})}
w_replace :: proc(a: ^App, cx: ^gk.Cx) {gk.replace(a.input.writes, cx, "HELLO")}
w_insert :: proc(a: ^App, cx: ^gk.Cx) {gk.insert(a.input.writes, cx, "!")}
w_set :: proc(a: ^App, cx: ^gk.Cx) {gk.set_value(a.input.writes, cx, "reset")}

EDITOR_TEXT :: "fn main() {\n    let x = 40 + 2;\n    println!(\"{x}\");\n}\n"

// completions answers the editor-completion page's provider: the two items the Rust page's
// CompletionProvider returns.
completions :: proc(a: ^App, cx: ^gk.Cx, e: gk.Provider_Event) {
	if _, ok := e.request.(gk.Provider_Completions); ok {
		items := []gk.Completion_Item{{label = "first", kind = 3, detail = "fn first()"}, {label = "second", kind = 6}}
		gk.answer(cx, a.input.asks, e.id, gk.Completion_Response{items = items})
	}
}

input_pages_init :: proc(a: ^App, cx: ^gk.Cx) {
	p := &a.input
	p.edit = gk.input_state(cx)->placeholder("Type here")->build()
	p.text = gk.textarea_state(cx)->auto_grow(2, 4)->placeholder("Notes")->build()
	p.editor = gk.editor_state(cx)->language("rust")->default_value(EDITOR_TEXT)->build()
	gk.set_line_number(p.editor, cx, true)
	p.number = gk.input_state(cx)->min(0)->max(10)->step(f64(2))->default_value("4")->build()
	p.otp = gk.otp_state(cx, 4)->build()
	p.masked = gk.input_state(cx)->masked(true)->default_value("secret")->build()
	p.off = gk.input_state(cx)->default_value("disabled")->build()
	p.writes = gk.input_state(cx)->default_value("hello world")->build()
	p.completion = gk.editor_state(cx)->language("rust")->build()
	p.asks = gk.editor_providers(cx, p.completion, a, completions, triggers = ".", which = {.Completions})
}

INPUT_PAGES := []Page {
	{"input-edit", proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
			return gk.div()->w(gk.px(240))->child(gk.input(a.input.edit))
		}},
	{"textarea", proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
			return gk.div()->w(gk.px(240))->child(gk.textarea(a.input.text))
		}},
	{"editor", proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
			return gk.div()->w(gk.px(480))->h(gk.px(160))->child(gk.base_editor(a.input.editor))
		}},
	{"number-input", proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
			return gk.div()->w(gk.px(200))->child(gk.number_input(a.input.number))
		}},
	{"otp-input", proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
			return gk.otp_input(a.input.otp)
		}},
	{"editor-completion", proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
			return gk.div()->w(gk.px(480))->child(gk.code_editor(a.input.completion, height = 160))
		}},
	{"input-writes", proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
			button :: proc(id, label: string, h: gk.Handler) -> gk.Button {
				return gk.button(id)->label(label)->w(gk.px(80))->on_click(h)
			}
			return gk.v_flex(.Gap_2)->
				child(gk.div()->w(gk.px(240))->child(gk.input(a.input.writes)))->
				child(gk.h_flex(.Gap_2)->
					child(button("w-select", "Select", gk.on(w_select)))->
					child(button("w-replace", "Replace", gk.on(w_replace)))->
					child(button("w-insert", "Insert", gk.on(w_insert)))->
					child(button("w-set", "Set", gk.on(w_set))))
		}},
	{"input-masked", proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
			return gk.v_flex(.Gap_2)->
				child(gk.div()->w(gk.px(240))->child(gk.input(a.input.masked)->mask_toggle()))->
				child(gk.div()->w(gk.px(240))->child(gk.input(a.input.off)->disabled(true)))
		}},
}
