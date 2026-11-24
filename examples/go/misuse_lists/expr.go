// The misuse_lists modes of an expression closure given to an input's step
// (gk.NewInputState(cx).StepBy("x * 2"), upstream's NumberStep::ByValue).
//
//	GK_NEG=expr-syntax an expression closure that does not parse: must stop
//	GK_NEG=expr-arg    an expression that uses an argument the closure is not handed: must stop
//	GK_NEG=expr-func   an expression that calls a function with the wrong arguments: must stop
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
