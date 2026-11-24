// Expression-parser misuse cases registered into misuse_lists: syntax errors,
// out-of-range argument names, and wrong function arity.
package main

import (
	"gpuikit.dev/gk"
	"gpuikit.dev/gk/editor"
)

func init() {
	pendingInit["expr-syntax"] = func(a *App, cx *gk.Cx) { a.inp = editor.NewInputState(cx).StepBy("x <").Build() }
	pendingInit["expr-arg"] = func(a *App, cx *gk.Cx) { a.inp = editor.NewInputState(cx).StepBy("z * 2").Build() }
	pendingInit["expr-func"] = func(a *App, cx *gk.Cx) { a.inp = editor.NewInputState(cx).StepBy("clamp(x, 1)").Build() }
}
