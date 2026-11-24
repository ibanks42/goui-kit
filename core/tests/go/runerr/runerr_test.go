// Package runerr verifies how gk.RunHeadless surfaces Go API misuse (`*gk.Error`)
// and Go runtime panics (`*gk.PanicError`), ensuring deferred functions execute
// and no further handlers run after the failure.
package runerr

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
)

var (
	mode     = os.Getenv("GK_PROTO")
	runErr   error
	deferred bool
	panicked any
	after    int
	failedAt bool
)

// script clicks twice so the tests can confirm the second click never reaches
// the handler after the first click triggers a misuse error or panic.
const script = "size 200 100\nframe\nclick 100 50\nframe\nclick 100 50\nframe\nframe\n"

func start(cx *gk.Cx) {
	kept := cx
	var root gk.View
	root = gk.NewView(nil, func(cx *gk.Cx) gk.Element {
		return gk.Div(s.SizeFull).ID("all").OnClickDo(func(cx *gk.Cx) {
			if failedAt {
				after++
				return
			}
			failedAt = true
			switch mode {
			case "gotest-misuse":
				_ = kept.Window()
			case "gotest-panic":
				var m map[string]int
				m["x"] = 1
			}
		})
	})
	gk.OpenWindow(cx, "runerr", 200, 100, root)
}

func TestMain(m *testing.M) {
	func() {
		defer func() { deferred = true }()
		defer func() { panicked = recover() }()
		runErr = gk.RunHeadless(script, start)
	}()
	os.Exit(m.Run())
}

// TestMisuse proves calling a gk API with a stale *gk.Cx (saved from start and
// reused inside OnClickDo) unwinds RunHeadless into a *gk.Error with a nonzero
// Go source line, executes deferred cleanup, and suppresses the second click.
func TestMisuse(t *testing.T) {
	if mode != "gotest-misuse" {
		t.Skip("GK_PROTO=gotest-misuse")
	}
	var e *gk.Error
	if !errors.As(runErr, &e) {
		t.Fatalf("RunHeadless returned %v (%T), not a *gk.Error", runErr, runErr)
	}
	fmt.Printf("runerr misuse error=%q line=%v deferred=%v after=%d\n", e.Msg[:39], e.Line != 0, deferred, after)
	if !deferred || after != 0 || e.Line == 0 {
		t.Fatal("the deferred function did not run, a handler ran after the stop, or no line")
	}
}

// TestPanic proves a Go runtime panic (writing to a nil map) inside an event
// handler is captured as a *gk.PanicError wrapping the runtime.Error with stack
// and location metadata, runs deferred cleanup, and blocks the second click.
func TestPanic(t *testing.T) {
	if mode != "gotest-panic" {
		t.Skip("GK_PROTO=gotest-panic")
	}
	p, ok := panicked.(*gk.PanicError)
	if !ok {
		t.Fatalf("RunHeadless panicked with %v (%T), not a *gk.PanicError (returned %v)", panicked, panicked, runErr)
	}
	var re runtime.Error
	fmt.Printf("runerr panic value=%q runtime_error=%v where=%q stack=%v deferred=%v after=%d\n",
		fmt.Sprint(p.Value), errors.As(p, &re), p.Where, len(p.Stack) > 0, deferred, after)
	if !deferred || after != 0 || !errors.As(p, &re) {
		t.Fatal("the deferred function did not run, a handler ran after the panic, or the value is lost")
	}
}
