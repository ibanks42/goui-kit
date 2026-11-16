// negative_compile: must NOT compile. Text lent by the binding is a gk.Temp_String, which does
// not assign to a `string`; gk.keep clones it. measure/odin-negative.sh checks that
// `odin check` refuses this file.
package negative_compile

import gk "gpuikit:gk"

App :: struct {
	name: gk.Input_State,
	kept: string,
}

store :: proc(a: ^App, cx: ^gk.Cx) {
	a.kept = gk.value(a.name, cx) // error: cannot assign Temp_String to string
}

main :: proc() {
	app: App
	_ = app
	_ = store
}
